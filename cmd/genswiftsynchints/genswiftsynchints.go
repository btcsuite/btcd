package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"syscall"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/cockroachdb/pebble/v2"
)

const (
	blockDbNamePrefix      = "blocks"
	blockDbType            = "ffldb"
	swiftSyncHintsFileName = "sshints.dat"
	spentBitmapFileName    = "spentbitmap.dat"
	spentBitmapVersion     = 0x01

	// Pebble key prefix for metadata entries.
	metaPrefix       = 'm'
	metaLastHeight   = "last_height"
	metaNextBitIndex = "next_bit_index"
	opMappingDbName  = "optoindex"
	progressInterval = 1000 // log progress every N blocks

	// maxBatchRecords is the maximum number of records to accumulate in a
	// Pebble batch before committing it.  Each tx mapping record carries a
	// 32-byte key and an 8-byte value, so this bounds batch memory to
	// roughly 1 GiB including per-record overhead.
	maxBatchRecords = 20_000_000

	// txCacheSize is the number of txhash -> startIndex entries to cache.
	// Each entry is ~40 bytes, so 1M entries ≈ 40MB.
	txCacheSize = 1_000_000

	// spentBatchBlocks is the number of blocks to process before flushing
	// the collected outpoints through a sorted iterator lookup when marking
	// spent bits.
	spentBatchBlocks = 5000

	// bitmapCheckpointInterval is the minimum time between periodic spent
	// bitmap checkpoints during marking.  Each checkpoint rewrites the
	// whole bitmap file, so checkpoints are rate-limited by time rather
	// than written after every block batch.
	bitmapCheckpointInterval = time.Minute

	swiftSyncHintsVersion = 0x01

	// swiftSyncHintsHeaderSize is the byte length of the hintsfile header:
	// magic(4) + version(1) + height(4).
	swiftSyncHintsHeaderSize = 9
)

var (
	cfg *config

	syncWriteOptions = pebble.Sync

	// swiftSyncHintsMagic is the hintsfile magic, the ASCII bytes "UTXO".
	swiftSyncHintsMagic = [4]byte{0x55, 0x54, 0x58, 0x4f}

	// spentBitmapMagic is the spent bitmap file magic, the ASCII bytes "SSBM".
	spentBitmapMagic = [4]byte{0x53, 0x53, 0x42, 0x4d}
)

// loadBlockDB opens the block database and returns a handle to it.
func loadBlockDB() (database.DB, error) {
	// The database name is based on the database type.
	dbName := blockDbNamePrefix + "_" + blockDbType
	dbPath := filepath.Join(cfg.DataDir, dbName)
	log.Infof("Loading block database from '%s'", dbPath)
	db, err := database.Open(blockDbType, dbPath, activeNetParams.Net)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// newPebbleDB returns a freshly-initialized Pebble instance backed by dbPath.
func newPebbleDB(dbPath string) (*pebble.DB, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("pebble database path cannot be empty")
	}

	if err := os.MkdirAll(dbPath, 0o750); err != nil {
		return nil, fmt.Errorf("unable to create pebble database dir %s: %w", dbPath, err)
	}

	db, err := pebble.Open(dbPath, &pebble.Options{})
	if err != nil {
		return nil, fmt.Errorf("unable to open pebble database at %s: %w", dbPath, err)
	}

	return db, nil
}

// metaKey returns the Pebble key for a metadata entry.
func metaKey(name string) []byte {
	key := make([]byte, 1+len(name))
	key[0] = metaPrefix
	copy(key[1:], name)
	return key
}

// getMetaUint64 retrieves a uint64 metadata value from Pebble.
func getMetaUint64(db *pebble.DB, name string) (uint64, error) {
	val, closer, err := db.Get(metaKey(name))
	if errors.Is(err, pebble.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer closer.Close()

	if len(val) != 8 {
		return 0, fmt.Errorf("invalid metadata value length for %s", name)
	}
	return binary.LittleEndian.Uint64(val), nil
}

// setMetaUint64 stores a uint64 metadata value in a Pebble batch.
func setMetaUint64(batch *pebble.Batch, name string, value uint64) error {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, value)
	return batch.Set(metaKey(name), buf, nil)
}

// putTxStartIndex stores a transaction's starting bit index in a batch.
// The key is just the txhash, and the value is the starting bit index.
// To get an outpoint's bit index: start_index + output_index.
func putTxStartIndex(batch *pebble.Batch, txHash *chainhash.Hash, startBitIndex uint64) error {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, startBitIndex)
	return batch.Set(txHash[:], buf, nil)
}

func readTxStartIndex(db *pebble.DB, txHash *chainhash.Hash) (uint64, bool, error) {
	val, closer, err := db.Get(txHash[:])
	if errors.Is(err, pebble.ErrNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	defer closer.Close()

	if len(val) != 8 {
		return 0, false, nil
	}

	return binary.LittleEndian.Uint64(val), true, nil
}

// txStartIndexCache is a simple cache for txhash -> starting bit index lookups.
// This significantly speeds up hintsfile generation since many inputs reference
// the same transactions. When full, the entire cache is cleared rather than
// using LRU eviction, which avoids the overhead of maintaining an order list.
type txStartIndexCache struct {
	cache map[chainhash.Hash]uint64
	max   int
}

func newTxStartIndexCache(size int) *txStartIndexCache {
	return &txStartIndexCache{
		cache: make(map[chainhash.Hash]uint64, size),
		max:   size,
	}
}

func (c *txStartIndexCache) get(hash chainhash.Hash) (uint64, bool) {
	val, ok := c.cache[hash]
	return val, ok
}

func (c *txStartIndexCache) put(hash chainhash.Hash, startIndex uint64) {
	if len(c.cache) >= c.max {
		// Clear entire cache instead of LRU eviction for better performance.
		c.cache = make(map[chainhash.Hash]uint64, c.max)
	}
	c.cache[hash] = startIndex
}

// getOutpointBitIndex looks up the bit index for an outpoint.
// It finds the transaction's starting bit index and adds the output index.
func getOutpointBitIndex(db *pebble.DB, cache *txStartIndexCache, op wire.OutPoint) (uint64, error) {
	// Check cache first.
	if startIndex, ok := cache.get(op.Hash); ok {
		return startIndex + uint64(op.Index), nil
	}

	startIndex, ok, err := readTxStartIndex(db, &op.Hash)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("txhash mapping missing or invalid: %v", op.Hash)
	}

	// Cache the result.
	cache.put(op.Hash, startIndex)

	return startIndex + uint64(op.Index), nil
}

// writeHintsBlock serializes one block's strictly increasing unspent output
// indices.  The encoding is a CompactSize byte length followed by that many
// bytes of CompactSize values: the first value is the first index, and every
// subsequent value is the gap to the previous index minus one (consecutive
// indices therefore encode as 0x00).
func writeHintsBlock(w io.Writer, indices []uint32) error {
	for i := 1; i < len(indices); i++ {
		if indices[i] <= indices[i-1] {
			return fmt.Errorf("indices must be strictly increasing")
		}
	}

	var payload bytes.Buffer

	// next is the smallest value the upcoming index may take: 0 for the
	// first index, previous index + 1 afterwards.
	var next uint64
	for _, index := range indices {
		if err := wire.WriteVarInt(&payload, 0, uint64(index)-next); err != nil {
			return err
		}
		next = uint64(index) + 1
	}

	if err := wire.WriteVarInt(w, 0, uint64(payload.Len())); err != nil {
		return err
	}
	_, err := w.Write(payload.Bytes())
	return err
}

func writeSwiftSyncHintsHeader(w io.Writer, height uint32) error {
	if _, err := w.Write(swiftSyncHintsMagic[:]); err != nil {
		return err
	}
	if _, err := w.Write([]byte{swiftSyncHintsVersion}); err != nil {
		return err
	}

	var heightBytes [4]byte
	binary.LittleEndian.PutUint32(heightBytes[:], height)
	_, err := w.Write(heightBytes[:])
	return err
}

// hintsFileMatches reports whether the hintsfile at path already exists and
// has a header matching the expected magic, version, and height.  It returns
// (false, nil) when the file does not exist; non-nil errors are reserved for
// unexpected I/O failures.
func hintsFileMatches(path string, expectedHeight uint32) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("unable to open %s: %w", path, err)
	}
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		// Not a gzip stream (or truncated): treat as "not matching" so
		// the caller regenerates rather than aborts.
		return false, nil
	}
	defer zr.Close()

	var hdr [9]byte
	if _, err := io.ReadFull(zr, hdr[:]); err != nil {
		// A short or unreadable file is treated as "not matching" so
		// the caller regenerates rather than aborts.
		return false, nil
	}
	if !bytes.Equal(hdr[0:4], swiftSyncHintsMagic[:]) {
		return false, nil
	}
	if hdr[4] != swiftSyncHintsVersion {
		return false, nil
	}
	return binary.LittleEndian.Uint32(hdr[5:9]) == expectedHeight, nil
}

// mappedBlockOutputEndBit returns the bit index immediately after all outputs
// in the provided block height according to the tx -> start index mapping.
func mappedBlockOutputEndBit(pdb *pebble.DB, chain *blockchain.BlockChain,
	height int32) (uint64, bool, error) {

	if height <= 0 {
		return 0, true, nil
	}

	block, err := chain.BlockByHeight(height)
	if err != nil {
		return 0, false, fmt.Errorf("unable to fetch block at height %d: %w",
			height, err)
	}

	txs := block.Transactions()
	for txIdx := len(txs) - 1; txIdx >= 0; txIdx-- {
		tx := txs[txIdx]
		numOutputs := len(tx.MsgTx().TxOut)
		if numOutputs == 0 {
			continue
		}

		txHash := tx.Hash()
		startIndex, mapped, err := readTxStartIndex(pdb, txHash)
		if err != nil {
			return 0, false, err
		}
		if !mapped {
			return 0, false, nil
		}

		return startIndex + uint64(numOutputs), true, nil
	}

	return 0, true, nil
}

// genOPMapping generates the outpoint -> bit index mapping by iterating through
// all blocks. It is resumable: progress is stored in Pebble and the function
// can be stopped via context cancellation.
func genOPMapping(ctx context.Context, pdb *pebble.DB, chain *blockchain.BlockChain, lastHash *chainhash.Hash) error {
	lastHeight, err := resolveTargetHeight(chain, lastHash)
	if err != nil {
		return err
	}

	// Load progress state.
	lastProcessedHeight, err := getMetaUint64(pdb, metaLastHeight)
	if err != nil {
		return fmt.Errorf("unable to read last height: %w", err)
	}
	nextBitIndex, err := getMetaUint64(pdb, metaNextBitIndex)
	if err != nil {
		return fmt.Errorf("unable to read next bit index: %w", err)
	}
	if lastProcessedHeight > uint64(math.MaxInt32) {
		return fmt.Errorf("mapping height metadata %d exceeds supported height",
			lastProcessedHeight)
	}

	// Determine starting height. We start from block 1 (skip genesis coinbase
	// which is unspendable) or resume from the next unprocessed block.
	startHeight := int32(1)
	if lastProcessedHeight > 0 {
		startHeight = int32(lastProcessedHeight) + 1
		log.Infof("Outpoint mapping resuming from block %d (next bit index: %d)",
			startHeight, nextBitIndex)
	} else {
		log.Infof("Outpoint mapping starting fresh from block %d", startHeight)
	}

	if startHeight > lastHeight {
		log.Infof("Outpoint mapping already up to date, nothing to do")
		return nil
	}

	log.Infof("Outpoint mapping processing blocks %d to %d", startHeight, lastHeight)

	batch := pdb.NewBatch()
	defer batch.Close()

	for height := startHeight; height <= lastHeight; height++ {
		// Check for cancellation.
		select {
		case <-ctx.Done():
			log.Infof("Outpoint mapping interrupted at block %d, saving progress...", height)
			// Save progress up to the last fully processed block.
			if height > startHeight {
				err := setMetaUint64(batch, metaLastHeight, uint64(height-1))
				if err != nil {
					return fmt.Errorf("unable to save progress: %w", err)
				}
				err = setMetaUint64(batch, metaNextBitIndex, nextBitIndex)
				if err != nil {
					return fmt.Errorf("unable to save progress: %w", err)
				}
				if err := batch.Commit(syncWriteOptions); err != nil {
					return fmt.Errorf("unable to save progress: %w", err)
				}
			}
			return ctx.Err()
		default:
		}

		block, err := chain.BlockByHeight(height)
		if err != nil {
			return fmt.Errorf("unable to fetch block at height %d: %w", height, err)
		}

		// Process each transaction. Store only the starting bit index for
		// each transaction to reduce the number of keys (optimization).
		// To get an outpoint's bit index: start_index + output_index.
		for _, tx := range block.Transactions() {
			txHash := tx.Hash()
			numOutputs := len(tx.MsgTx().TxOut)
			if numOutputs > 0 {
				err := putTxStartIndex(batch, txHash, nextBitIndex)
				if err != nil {
					return fmt.Errorf("unable to add tx mapping at height %d: %w",
						height, err)
				}
				nextBitIndex += uint64(numOutputs)
			}
		}

		// Commit batch periodically to limit memory usage, provide
		// resumability checkpoints, and avoid unbounded memory growth.
		// Batch.Count counts records, not bytes.
		if height == lastHeight || batch.Count() >= maxBatchRecords {
			err := setMetaUint64(batch, metaLastHeight, uint64(height))
			if err != nil {
				return fmt.Errorf("unable to set last height at height %d: %w",
					height, err)
			}
			err = setMetaUint64(batch, metaNextBitIndex, nextBitIndex)
			if err != nil {
				return fmt.Errorf("unable to set next bit index at height %d: %w",
					height, err)
			}
			if err := batch.Commit(syncWriteOptions); err != nil {
				return fmt.Errorf("unable to commit batch at height %d: %w", height, err)
			}
			batch.Reset()
		}

		// Log progress periodically.
		if height%progressInterval == 0 {
			log.Infof("Outpoint mapping processed block %d / %d (bit index: %d)",
				height, lastHeight, nextBitIndex)
		}
	}

	log.Infof("Outpoint mapping complete. Total outputs: %d", nextBitIndex)
	return nil
}

// saveSpentBitmap writes the spent bitset along with the height through which
// spends have been marked: a magic tag, a format version, the marked height
// (uint32 LE), the bitset length (uint64 LE, letting loads detect truncated
// files), then the bitset itself.  The file is written to a temp path,
// synced, and renamed into place so an interrupted write never replaces a
// good bitmap with a partial one.
func saveSpentBitmap(path string, spent []byte, height int32) error {
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("unable to create %s: %w", tmpPath, err)
	}

	closed := false
	defer func() {
		if !closed {
			f.Close()
			os.Remove(tmpPath)
		}
	}()

	bw := bufio.NewWriter(f)
	if _, err := bw.Write(spentBitmapMagic[:]); err != nil {
		return err
	}
	if err := bw.WriteByte(spentBitmapVersion); err != nil {
		return err
	}
	var heightBytes [4]byte
	binary.LittleEndian.PutUint32(heightBytes[:], uint32(height))
	if _, err := bw.Write(heightBytes[:]); err != nil {
		return err
	}
	var lenBytes [8]byte
	binary.LittleEndian.PutUint64(lenBytes[:], uint64(len(spent)))
	if _, err := bw.Write(lenBytes[:]); err != nil {
		return err
	}
	if _, err := bw.Write(spent); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	closed = true

	return os.Rename(tmpPath, path)
}

// loadSpentBitmap returns a bitset of bitsetSize bytes seeded from the bitmap
// file at path, along with the height through which spends are already
// marked.  A file that cannot seed the requested target — missing, corrupt,
// shorter than its recorded payload length, marked beyond targetHeight, or
// carrying spent bits past bitsetSize — yields a zeroed bitset and height 0
// so the caller rebuilds from scratch; non-nil errors are reserved for
// unexpected I/O failures.
//
// The payload length is independent of targetHeight: the set bits only
// describe spends by blocks up to the marked height, so a bitset saved while
// targeting one height seeds a run targeting any height at or above the
// marked height.  A shorter payload than bitsetSize leaves the tail of the
// returned bitset zeroed (all unspent).
func loadSpentBitmap(path string, bitsetSize uint64, targetHeight int32) ([]byte, int32, error) {
	log.Infof("Allocating in-memory spent bitset of %d bytes (%d MiB)",
		bitsetSize, bitsetSize/(1<<20))
	spent := make([]byte, bitsetSize)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return spent, 0, nil
		}
		return nil, 0, fmt.Errorf("unable to open %s: %w", path, err)
	}
	defer f.Close()

	var hdr [17]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		log.Warnf("Spent bitmap %s is too short; rebuilding", path)
		return spent, 0, nil
	}
	if !bytes.Equal(hdr[0:4], spentBitmapMagic[:]) || hdr[4] != spentBitmapVersion {
		log.Warnf("Spent bitmap %s has an unrecognized header; rebuilding", path)
		return spent, 0, nil
	}

	markedHeight := binary.LittleEndian.Uint32(hdr[5:9])

	// A payload shorter than its recorded length means spends were lost to
	// truncation; seeding from it would silently emit those outputs as
	// unspent.
	payloadLen := binary.LittleEndian.Uint64(hdr[9:17])
	fi, err := f.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("unable to stat %s: %w", path, err)
	}
	if uint64(fi.Size())-uint64(len(hdr)) != payloadLen {
		log.Warnf("Spent bitmap %s does not match its recorded payload "+
			"length; rebuilding", path)
		return spent, 0, nil
	}

	if int64(markedHeight) > int64(targetHeight) {
		log.Infof("Spent bitmap is marked through block %d, beyond target %d; rebuilding",
			markedHeight, targetHeight)
		return spent, 0, nil
	}

	if _, err := io.ReadFull(f, spent); err != nil &&
		err != io.ErrUnexpectedEOF && err != io.EOF {

		return nil, 0, fmt.Errorf("unable to read %s: %w", path, err)
	}

	// A file longer than bitsetSize was written for a higher target.  Because
	// it is marked only through markedHeight (at or below targetHeight), every
	// spend it records lands within the first bitsetSize bytes.  A set bit past
	// that means the file describes different blocks and cannot seed this
	// target.
	buf := make([]byte, 64*1024)
	for {
		n, err := f.Read(buf)
		for _, b := range buf[:n] {
			if b != 0 {
				log.Warnf("Spent bitmap %s has spent bits beyond target %d; rebuilding",
					path, targetHeight)
				return make([]byte, bitsetSize), 0, nil
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("unable to read %s: %w", path, err)
		}
	}

	return spent, int32(markedHeight), nil
}

// markSpentBits walks every input in blocks startHeight..lastHeight, looks up
// the bit index of the referenced outpoint via the tx -> start_bit_index
// Pebble mapping, and sets the corresponding bit in spent.  Progress is
// checkpointed to the spent bitmap file at bitmapPath — periodically, on
// interruption, and always on completion — so a later run resumes marking
// where this one stopped, or extends the bitmap to a higher target without
// re-marking the blocks already covered.
func markSpentBits(ctx context.Context, pdb *pebble.DB, chain *blockchain.BlockChain,
	startHeight, lastHeight int32, spent []byte, bitmapPath string) error {

	cache := newTxStartIndexCache(txCacheSize)
	markedHeight := startHeight - 1
	lastCheckpoint := time.Now()
	for batchStart := startHeight; batchStart <= lastHeight; batchStart += spentBatchBlocks {
		select {
		case <-ctx.Done():
			if markedHeight >= startHeight {
				log.Infof("Interrupted at block %d, saving spent bitmap...",
					batchStart)
				if err := saveSpentBitmap(bitmapPath, spent, markedHeight); err != nil {
					return fmt.Errorf("unable to save spent bitmap: %w", err)
				}
			}
			return ctx.Err()
		default:
		}

		batchEnd := batchStart + spentBatchBlocks - 1
		if batchEnd > lastHeight {
			batchEnd = lastHeight
		}

		outpoints := make([]wire.OutPoint, 0, 50000)
		for height := batchStart; height <= batchEnd; height++ {
			block, err := chain.BlockByHeight(height)
			if err != nil {
				return fmt.Errorf("unable to fetch block at height %d: %w", height, err)
			}
			for _, tx := range block.Transactions() {
				if blockchain.IsCoinBaseTx(tx.MsgTx()) {
					continue
				}
				for _, txIn := range tx.MsgTx().TxIn {
					outpoints = append(outpoints, txIn.PreviousOutPoint)
				}
			}
		}

		bitIndices, err := lookupOutpointsBatch(pdb, cache, outpoints)
		if err != nil {
			return fmt.Errorf("unable to lookup outpoints at blocks %d-%d: %w",
				batchStart, batchEnd, err)
		}

		for _, bitIdx := range bitIndices {
			spent[bitIdx/8] |= 1 << (bitIdx % 8)
		}

		markedHeight = batchEnd

		if time.Since(lastCheckpoint) >= bitmapCheckpointInterval {
			if err := saveSpentBitmap(bitmapPath, spent, markedHeight); err != nil {
				return fmt.Errorf("unable to save spent bitmap: %w", err)
			}
			lastCheckpoint = time.Now()
			log.Infof("Checkpointed spent bitmap through block %d", markedHeight)
		}

		log.Infof("Spent-marking progress: block %d / %d", batchEnd, lastHeight)
	}

	if err := saveSpentBitmap(bitmapPath, spent, lastHeight); err != nil {
		return fmt.Errorf("unable to save spent bitmap: %w", err)
	}
	log.Infof("Saved spent bitmap through block %d", lastHeight)

	return nil
}

// resolveTargetHeight resolves lastHash to the block height the spent bitmap
// and hintsfile should cover, rejecting heights that do not fit the hintsfile
// format.
func resolveTargetHeight(chain *blockchain.BlockChain,
	lastHash *chainhash.Hash) (int32, error) {

	lastHeight, err := chain.BlockHeightByHash(lastHash)
	if err != nil {
		return 0, fmt.Errorf("unable to get height for hash %v: %w", lastHash, err)
	}
	if lastHeight < 0 {
		return 0, fmt.Errorf("invalid negative swift sync height: %d", lastHeight)
	}
	if uint64(lastHeight) > math.MaxUint32 {
		return 0, fmt.Errorf("swift sync height %d exceeds hintsfile limit", lastHeight)
	}
	return lastHeight, nil
}

// totalOutputBits returns the number of output bits across blocks 1..lastHeight
// from the outpoint mapping, which is the size in bits the spent bitset must
// cover.
func totalOutputBits(pdb *pebble.DB, chain *blockchain.BlockChain,
	lastHeight int32) (uint64, error) {

	totalBits, mapped, err := mappedBlockOutputEndBit(pdb, chain, lastHeight)
	if err != nil {
		return 0, err
	}
	if !mapped || totalBits == 0 {
		return 0, fmt.Errorf("outpoint mapping incomplete at block %d; run mapping phase first",
			lastHeight)
	}
	if totalBits > math.MaxInt64 {
		return 0, fmt.Errorf("bit count %d too large for in-memory bitset", totalBits)
	}
	return totalBits, nil
}

// genSpentBitmap marks every spent output for blocks 1..lastHeight into the
// spent bitmap file (spentbitmap.dat) using the tx -> start_bit_index mapping
// from genOPMapping.  Progress is checkpointed to the bitmap file, so a later
// run resumes where it stopped, or extends a bitmap marked through a lower
// height to a higher target.  A target below the bitmap's marked height
// rebuilds it from block 1, since set bits cannot be taken back.
func genSpentBitmap(ctx context.Context, pdb *pebble.DB, chain *blockchain.BlockChain,
	lastHash *chainhash.Hash) error {

	lastHeight, err := resolveTargetHeight(chain, lastHash)
	if err != nil {
		return err
	}

	totalBits, err := totalOutputBits(pdb, chain, lastHeight)
	if err != nil {
		return err
	}

	bitmapPath := filepath.Join(cfg.DataDir, spentBitmapFileName)
	spent, markedHeight, err := loadSpentBitmap(bitmapPath, (totalBits+7)/8, lastHeight)
	if err != nil {
		return err
	}
	if markedHeight >= lastHeight {
		log.Infof("Spent bitmap already marked through block %d", lastHeight)
		return nil
	}

	log.Infof("Marking spent outputs for blocks %d..%d", markedHeight+1, lastHeight)
	return markSpentBits(ctx, pdb, chain, markedHeight+1, lastHeight, spent, bitmapPath)
}

// genHintsFile writes the swift sync hintsfile at sshints.dat for blocks
// 1..lastHeight from the completed spent bitmap.  It walks each block's outputs
// in order, skips unspendables per the hintsfile rules, and writes one varint
// hints entry per block: an output is recorded as unspent when its spendable
// index is not marked in the bitmap.  The spent bitmap must already be marked
// through lastHeight (see genSpentBitmap).
//
// Per-block entries are written uncompressed to a .raw working file as they are
// produced; the completed file is then gzip-compressed into sshints.dat.  An
// interrupted run leaves the .raw file in place, and a later run resumes from
// the last fully written block rather than restarting.
func genHintsFile(ctx context.Context, pdb *pebble.DB, chain *blockchain.BlockChain,
	lastHash *chainhash.Hash) error {

	lastHeight, err := resolveTargetHeight(chain, lastHash)
	if err != nil {
		return err
	}

	// If a hintsfile already exists for this exact height, there is
	// nothing to do: re-running would just redo the emit pass to produce
	// an identical file.
	hintsPath := filepath.Join(cfg.DataDir, swiftSyncHintsFileName)
	complete, err := hintsFileMatches(hintsPath, uint32(lastHeight))
	if err != nil {
		return err
	}
	if complete {
		log.Infof("Hintsfile already up to date at %s; nothing to do",
			hintsPath)
		// Drop any working file left behind by an interrupted earlier run.
		_ = os.Remove(hintsPath + ".raw")
		return nil
	}

	totalBits, err := totalOutputBits(pdb, chain, lastHeight)
	if err != nil {
		return err
	}

	bitmapPath := filepath.Join(cfg.DataDir, spentBitmapFileName)
	spent, markedHeight, err := loadSpentBitmap(bitmapPath, (totalBits+7)/8, lastHeight)
	if err != nil {
		return err
	}
	if markedHeight < lastHeight {
		return fmt.Errorf("spent bitmap is only marked through block %d; "+
			"run the spent bitmap pass first", markedHeight)
	}

	// Per-block entries are written uncompressed to this working file so an
	// interrupted run can resume from the last fully written block.
	rawPath := hintsPath + ".raw"
	resumeBlocks, resumeOff, resumable, err := scanPartialHints(
		rawPath, uint32(lastHeight))
	if err != nil {
		return err
	}
	if resumeBlocks > lastHeight {
		// A working file claiming more blocks than the target is corrupt, so
		// rebuild from scratch rather than trusting the count.
		resumable = false
	}

	var hintsFile *os.File
	if resumable && resumeBlocks > 0 {
		hintsFile, err = os.OpenFile(rawPath, os.O_RDWR, 0o644)
		if err != nil {
			return fmt.Errorf("unable to open %s: %w", rawPath, err)
		}
		// Drop a torn trailing entry, then append after the last good block.
		if err := hintsFile.Truncate(resumeOff); err != nil {
			hintsFile.Close()
			return fmt.Errorf("unable to truncate %s: %w", rawPath, err)
		}
		if _, err := hintsFile.Seek(resumeOff, io.SeekStart); err != nil {
			hintsFile.Close()
			return fmt.Errorf("unable to seek %s: %w", rawPath, err)
		}
		log.Infof("Resuming hintsfile from block %d / %d",
			resumeBlocks+1, lastHeight)
	} else {
		resumeBlocks = 0
		hintsFile, err = os.Create(rawPath)
		if err != nil {
			return fmt.Errorf("unable to create %s: %w", rawPath, err)
		}
	}

	// On an early return the working file is kept so the next run resumes; it
	// is removed only after it has been compressed into place.
	rawClosed := false
	defer func() {
		if !rawClosed {
			hintsFile.Close()
		}
	}()

	fw := bufio.NewWriter(hintsFile)
	if resumeBlocks == 0 {
		if err := writeSwiftSyncHintsHeader(fw, uint32(lastHeight)); err != nil {
			return fmt.Errorf("unable to write hintsfile header: %w", err)
		}
	}

	// Recompute the spent-bitmap cursor for the first block still to write.
	bitIdx, mapped, err := mappedBlockOutputEndBit(pdb, chain, resumeBlocks)
	if err != nil {
		return err
	}
	if !mapped {
		return fmt.Errorf("outpoint mapping incomplete at block %d", resumeBlocks)
	}

	log.Infof("Writing hintsfile for blocks %d..%d", resumeBlocks+1, lastHeight)
	var unspentIndices []uint32
	net := activeNetParams.Net
	for height := resumeBlocks + 1; height <= lastHeight; height++ {
		select {
		case <-ctx.Done():
			// Persist the blocks written so far so the next run resumes
			// from here, then leave the working file in place.
			if ferr := fw.Flush(); ferr != nil {
				return ferr
			}
			return ctx.Err()
		default:
		}

		block, err := chain.BlockByHeight(height)
		if err != nil {
			return fmt.Errorf("unable to fetch block at height %d: %w", height, err)
		}

		unspentIndices = unspentIndices[:0]
		var spendableIdx uint32
		blockHash := block.Hash()
		for txIndex, tx := range block.Transactions() {
			for _, txOut := range tx.MsgTx().TxOut {
				isSpent := (spent[bitIdx/8] & (1 << (bitIdx % 8))) != 0
				bitIdx++

				if blockchain.IsHintsfileUnspendableOutput(
					net, blockHash, height, txIndex, txOut,
				) {
					continue
				}

				if !isSpent {
					unspentIndices = append(unspentIndices, spendableIdx)
				}

				if spendableIdx == math.MaxUint32 {
					return fmt.Errorf("too many spendable outputs in block %d", height)
				}
				spendableIdx++
			}
		}

		if err := writeHintsBlock(fw, unspentIndices); err != nil {
			return fmt.Errorf("unable to write hints for block %d: %w", height, err)
		}

		if height%progressInterval == 0 {
			// Flush so an unexpected kill loses at most one interval of
			// work for the resume.
			if err := fw.Flush(); err != nil {
				return fmt.Errorf("unable to flush hintsfile: %w", err)
			}
			log.Infof("Hintsfile progress: block %d / %d", height, lastHeight)
		}
	}

	if bitIdx != totalBits {
		return fmt.Errorf("bitset has %d unused trailing bits",
			totalBits-bitIdx)
	}

	if err := fw.Flush(); err != nil {
		return fmt.Errorf("unable to flush hintsfile: %w", err)
	}
	if err := hintsFile.Sync(); err != nil {
		return fmt.Errorf("unable to sync hintsfile: %w", err)
	}
	if err := hintsFile.Close(); err != nil {
		return fmt.Errorf("unable to close hintsfile: %w", err)
	}
	rawClosed = true

	// Compress the completed working file into place, then drop the raw copy.
	if err := gzipFile(rawPath, hintsPath); err != nil {
		return err
	}
	if err := os.Remove(rawPath); err != nil {
		return fmt.Errorf("unable to remove %s: %w", rawPath, err)
	}

	log.Infof("Hintsfile generation complete: %s", hintsPath)
	return nil
}

// scanPartialHints inspects an uncompressed partial hintsfile and reports how
// many complete per-block entries it already holds and the byte offset just
// past them.  ok is false when the file is absent or its header does not match
// expectedHeight, in which case the caller starts a fresh pass.  A torn final
// entry is excluded from the count so the resumed pass rewrites it.
func scanPartialHints(path string, expectedHeight uint32) (int32, int64, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, false, nil
		}
		return 0, 0, false, fmt.Errorf("unable to read %s: %w", path, err)
	}
	if len(data) < swiftSyncHintsHeaderSize ||
		!bytes.Equal(data[0:4], swiftSyncHintsMagic[:]) ||
		data[4] != swiftSyncHintsVersion ||
		binary.LittleEndian.Uint32(data[5:9]) != expectedHeight {

		return 0, 0, false, nil
	}

	r := bytes.NewReader(data[swiftSyncHintsHeaderSize:])
	var blocks int32
	var consumed int64 // body bytes covered by complete entries
	for r.Len() > 0 {
		byteLen, err := wire.ReadVarInt(r, 0)
		if err != nil {
			break // torn length prefix
		}
		if byteLen > uint64(r.Len()) {
			break // torn payload
		}
		if _, err := r.Seek(int64(byteLen), io.SeekCurrent); err != nil {
			break
		}
		consumed = r.Size() - int64(r.Len())
		blocks++
	}

	return blocks, int64(swiftSyncHintsHeaderSize) + consumed, true, nil
}

// gzipFile compresses srcPath into dstPath, writing the gzip stream to a temp
// file that is renamed into place so dstPath only ever appears complete.
func gzipFile(srcPath, dstPath string) error {
	// Open the uncompressed source file.
	in, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("unable to open %s: %w", srcPath, err)
	}
	defer in.Close()

	// Write the compressed output to a temp file.
	tmp := dstPath + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("unable to create %s: %w", tmp, err)
	}
	done := false
	defer func() {
		// Drop the temp file unless the rename below succeeds.
		if !done {
			out.Close()
			os.Remove(tmp)
		}
	}()

	// Bytes flow in -> gzip -> bufio -> out.
	fw := bufio.NewWriter(out)
	zw, err := gzip.NewWriterLevel(fw, gzip.BestCompression)
	// NewWriterLevel only errors on an invalid level.
	if err != nil {
		return err
	}
	// Compress the whole source into the gzip stream.
	if _, err := io.Copy(zw, in); err != nil {
		return fmt.Errorf("unable to compress %s: %w", srcPath, err)
	}
	// Finish the gzip stream, flushing its trailer into the bufio.
	if err := zw.Close(); err != nil {
		return fmt.Errorf("unable to finalize hintsfile gzip stream: %w", err)
	}
	// Push the buffered bytes down to the file.
	if err := fw.Flush(); err != nil {
		return fmt.Errorf("unable to flush %s: %w", tmp, err)
	}
	// Persist the file to disk before the rename.
	if err := out.Sync(); err != nil {
		return fmt.Errorf("unable to sync %s: %w", tmp, err)
	}
	// Close the file so the rename has no open handle.
	if err := out.Close(); err != nil {
		return fmt.Errorf("unable to close %s: %w", tmp, err)
	}
	// Publish atomically so the destination only ever appears complete.
	if err := os.Rename(tmp, dstPath); err != nil {
		return fmt.Errorf("unable to move %s into place: %w", dstPath, err)
	}
	done = true
	return nil
}

// setMetaUint64Direct stores a uint64 metadata value directly (not in a batch).
func setMetaUint64Direct(db *pebble.DB, name string, value uint64) error {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, value)
	return db.Set(metaKey(name), buf, syncWriteOptions)
}

// deleteMetaKey deletes a metadata key from Pebble.
func deleteMetaKey(db *pebble.DB, name string) error {
	return db.Delete(metaKey(name), syncWriteOptions)
}

// deleteDataFile removes the named swift sync file from the data directory
// along with its ".tmp" variant, which an interrupted write may have left
// behind.  A file that does not exist is not an error.  It does not touch the
// outpoint mapping database.
func deleteDataFile(fileName string) error {
	for _, name := range []string{fileName, fileName + ".tmp"} {
		filePath := filepath.Join(cfg.DataDir, name)
		err := os.Remove(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("unable to delete %s: %w", name, err)
		}
		log.Infof("Deleted swift sync file: %s", filePath)
	}
	return nil
}

// deleteMapping removes the outpoint mapping database and its metadata.
func deleteMapping(pdb *pebble.DB) error {
	// Delete the mapping metadata first (while db is still open).
	if err := deleteMetaKey(pdb, metaLastHeight); err != nil {
		return fmt.Errorf("unable to delete mapping last_height metadata: %w", err)
	}
	if err := deleteMetaKey(pdb, metaNextBitIndex); err != nil {
		return fmt.Errorf("unable to delete mapping next_bit_index metadata: %w", err)
	}
	log.Infof("Deleted mapping progress metadata")

	// Close the Pebble instance before deleting its directory.
	if err := pdb.Close(); err != nil {
		return fmt.Errorf("unable to close mapping database: %w", err)
	}

	// Delete the mapping database directory.
	dbPath := filepath.Join(cfg.DataDir, opMappingDbName)
	if err := os.RemoveAll(dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("unable to delete mapping database: %w", err)
	}
	log.Infof("Deleted mapping database: %s", dbPath)

	return nil
}

// lookupOutpointsBatch looks up bit indices for multiple outpoints using a
// sorted iterator for efficient sequential access. It also populates the cache.
func lookupOutpointsBatch(db *pebble.DB, cache *txStartIndexCache, outpoints []wire.OutPoint) ([]uint64, error) {
	if len(outpoints) == 0 {
		return nil, nil
	}

	// First, check cache and collect uncached txhashes.
	bitIndices := make([]uint64, len(outpoints))
	uncachedIndices := make([]int, 0, len(outpoints))
	uncachedOutpoints := make([]wire.OutPoint, 0, len(outpoints))

	for i, op := range outpoints {
		if startIndex, ok := cache.get(op.Hash); ok {
			bitIndices[i] = startIndex + uint64(op.Index)
		} else {
			uncachedIndices = append(uncachedIndices, i)
			uncachedOutpoints = append(uncachedOutpoints, op)
		}
	}

	if len(uncachedOutpoints) == 0 {
		return bitIndices, nil
	}

	// Sort uncached outpoints by txhash for sequential iterator access.
	sortOrder := make([]int, len(uncachedOutpoints))
	for i := range sortOrder {
		sortOrder[i] = i
	}
	sort.Slice(sortOrder, func(i, j int) bool {
		return bytes.Compare(uncachedOutpoints[sortOrder[i]].Hash[:],
			uncachedOutpoints[sortOrder[j]].Hash[:]) < 0
	})

	// Use iterator for sequential lookups.
	iter, err := db.NewIter(nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create iterator: %w", err)
	}
	defer iter.Close()

	var lastHash chainhash.Hash
	var lastStartIndex uint64
	var hasLast bool

	for _, sortIdx := range sortOrder {
		op := uncachedOutpoints[sortIdx]
		origIdx := uncachedIndices[sortIdx]

		// Skip lookup if same txhash as previous (common for multi-input txs).
		if hasLast && op.Hash == lastHash {
			bitIndices[origIdx] = lastStartIndex + uint64(op.Index)
			continue
		}

		// Seek to the txhash.
		if !iter.SeekGE(op.Hash[:]) {
			return nil, fmt.Errorf("txhash not found: %v", op.Hash)
		}

		// Verify we found the exact key.
		key := iter.Key()
		if len(key) != chainhash.HashSize || !bytes.Equal(key, op.Hash[:]) {
			return nil, fmt.Errorf("txhash not found: %v", op.Hash)
		}

		val := iter.Value()
		if len(val) != 8 {
			return nil, fmt.Errorf("invalid value length for tx %v", op.Hash)
		}

		startIndex := binary.LittleEndian.Uint64(val)
		bitIndices[origIdx] = startIndex + uint64(op.Index)

		// Cache and remember for next iteration.
		cache.put(op.Hash, startIndex)
		lastHash = op.Hash
		lastStartIndex = startIndex
		hasLast = true
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("iterator error: %w", err)
	}

	return bitIndices, nil
}

func main() {
	// Load configuration and parse command line.
	tcfg, _, err := loadConfig()
	if err != nil {
		return
	}
	cfg = tcfg

	// Start CPU profiling if requested.
	if cfg.CPUProfile != "" {
		f, err := os.Create(cfg.CPUProfile)
		if err != nil {
			log.Errorf("Failed to create CPU profile: %v", err)
			return
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Errorf("Failed to start CPU profile: %v", err)
			return
		}
		defer pprof.StopCPUProfile()
	}

	// Write a memory profile on exit if requested.
	if cfg.MemProfile != "" {
		f, err := os.Create(cfg.MemProfile)
		if err != nil {
			log.Errorf("Failed to create memory profile: %v", err)
			return
		}
		defer func() {
			runtime.GC()
			if err := pprof.WriteHeapProfile(f); err != nil {
				log.Errorf("Failed to write memory profile: %v", err)
			}
			f.Close()
		}()
	}

	// Set up signal handling for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Infof("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Load the block database.
	db, err := loadBlockDB()
	if err != nil {
		log.Errorf("Failed to load database: %v", err)
		return
	}
	defer db.Close()

	// Open Pebble for storing the outpoint -> index mapping.
	dbPath := filepath.Join(cfg.DataDir, opMappingDbName)
	pdb, err := newPebbleDB(dbPath)
	if err != nil {
		log.Errorf("Failed to open mapping database: %v", err)
		return
	}

	// Handle deletion flags.
	if cfg.DeleteHints {
		if err := deleteDataFile(swiftSyncHintsFileName); err != nil {
			log.Errorf("Failed to delete hintsfile: %v", err)
		} else {
			log.Infof("Hintsfile deletion complete")
		}
		pdb.Close()
		return
	}

	if cfg.DeleteMapping {
		// Also delete the hintsfile and the spent bitmap since they
		// would be stale without the mapping they were derived from.
		if err := deleteDataFile(swiftSyncHintsFileName); err != nil {
			log.Errorf("Failed to delete hintsfile: %v", err)
		}
		if err := deleteDataFile(spentBitmapFileName); err != nil {
			log.Errorf("Failed to delete spent bitmap: %v", err)
		}
		// deleteMapping closes pdb and removes the directory.
		if err := deleteMapping(pdb); err != nil {
			log.Errorf("Failed to delete mapping: %v", err)
		} else {
			log.Infof("Mapping deletion complete")
		}
		return
	}

	defer pdb.Close()

	// Setup chain.  Ignore notifications since they aren't needed for this
	// util.
	chain, err := blockchain.New(&blockchain.Config{
		DB:          db,
		ChainParams: activeNetParams,
		TimeSource:  blockchain.NewMedianTime(),
	})
	if err != nil {
		log.Errorf("Failed to initialize chain: %v", err)
		return
	}

	hash := &chain.BestSnapshot().Hash
	if cfg.GenBlockHash != "" {
		hash, err = chainhash.NewHashFromStr(cfg.GenBlockHash)
		if err != nil {
			log.Errorf("Unable to parse block hash: %v", cfg.GenBlockHash)
			return
		}
	}

	// Phase 1: build the outpoint -> bit index mapping. Resumable.
	err = genOPMapping(ctx, pdb, chain, hash)
	if err != nil {
		if err == context.Canceled {
			log.Infof("Generation stopped. Run again to resume.")
		} else {
			log.Errorf("Failed to generate mapping: %v", err)
		}
		return
	}

	log.Infof("Outpoint mapping generation complete!")

	// Phase 2: mark spent outputs into the spent bitmap. Resumable.
	err = genSpentBitmap(ctx, pdb, chain, hash)
	if err != nil {
		if err == context.Canceled {
			log.Infof("Spent bitmap generation stopped. Run again to resume.")
		} else {
			log.Errorf("Failed to generate spent bitmap: %v", err)
		}
		return
	}

	log.Infof("Spent bitmap generation complete!")

	// Phase 3: write the hintsfile from the spent bitmap.
	err = genHintsFile(ctx, pdb, chain, hash)
	if err != nil {
		if err == context.Canceled {
			log.Infof("Hintsfile generation stopped. Run again to resume.")
		} else {
			log.Errorf("Failed to generate hintsfile: %v", err)
		}
		return
	}

	log.Infof("Hintsfile generation complete!")
}
