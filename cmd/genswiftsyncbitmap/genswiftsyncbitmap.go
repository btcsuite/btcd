package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"syscall"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
	"github.com/cockroachdb/pebble"
)

const (
	blockDbNamePrefix       = "blocks"
	swiftSyncBitmapFileName = "ssbitmap.dat"
	headerSize              = 8

	// PebbleDB key prefix for metadata entries.
	metaPrefix           = 'm'
	metaLastHeight       = "last_height"
	metaNextBitIndex     = "next_bit_index"
	metaBitmapLastHeight = "bitmap_last_height"
	opMappingDbName      = "optoindex"
	progressInterval     = 1000 // log progress every N blocks

	// maxBatchSize is the maximum size of a pebble batch before we commit.
	// Pebble panics if batch size exceeds 4GB, so we use 3GB to be safe.
	maxBatchSize = 3 << 30 // 3GB

	// txCacheSize is the number of txhash -> startIndex entries to cache.
	// Each entry is ~40 bytes, so 1M entries ≈ 40MB.
	txCacheSize = 1_000_000

	// bitmapBatchBlocks is the number of blocks to process before flushing
	// spent bits to the bitmap file and doing sorted iterator lookups.
	bitmapBatchBlocks = 5000
)

var (
	cfg *config
)

// loadBlockDB opens the block database and returns a handle to it.
func loadBlockDB() (database.DB, error) {
	// The database name is based on the database type.
	dbName := blockDbNamePrefix + "_" + cfg.DbType
	dbPath := filepath.Join(cfg.DataDir, dbName)
	log.Infof("Loading block database from '%s'", dbPath)
	db, err := database.Open(cfg.DbType, dbPath, activeNetParams.Net)
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

// metaKey returns the PebbleDB key for a metadata entry.
func metaKey(name string) []byte {
	key := make([]byte, 1+len(name))
	key[0] = metaPrefix
	copy(key[1:], name)
	return key
}

// getMetaUint64 retrieves a uint64 metadata value from PebbleDB.
func getMetaUint64(db *pebble.DB, name string) (uint64, error) {
	val, closer, err := db.Get(metaKey(name))
	if err == pebble.ErrNotFound {
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

// setMetaUint64 stores a uint64 metadata value in a PebbleDB batch.
func setMetaUint64(batch *pebble.Batch, name string, value uint64) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, value)
	batch.Set(metaKey(name), buf, pebble.NoSync)
}

// putTxStartIndex stores a transaction's starting bit index in a batch.
// The key is just the txhash, and the value is the starting bit index.
// To get an outpoint's bit index: start_index + output_index.
func putTxStartIndex(batch *pebble.Batch, txHash *chainhash.Hash, startBitIndex uint64) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, startBitIndex)
	batch.Set(txHash[:], buf, pebble.NoSync)
}

// txStartIndexCache is a simple cache for txhash -> starting bit index lookups.
// This significantly speeds up bitmap generation since many inputs reference
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

	val, closer, err := db.Get(op.Hash[:])
	if err != nil {
		return 0, err
	}
	defer closer.Close()

	if len(val) != 8 {
		return 0, fmt.Errorf("invalid value length for tx %v", op.Hash)
	}
	startIndex := binary.LittleEndian.Uint64(val)

	// Cache the result.
	cache.put(op.Hash, startIndex)

	return startIndex + uint64(op.Index), nil
}

// allocateSwiftSyncBitmapBits reserves bitCount new bits at the end of
// ssbitmap.dat, ensuring the 8-byte header tracks the total allocated bits and
// the file is zero-extended as needed. It returns the starting bit index for the
// allocated range.
func allocateSwiftSyncBitmapBits(file *os.File, bitCount uint64) (uint64, error) {
	if bitCount == 0 {
		return 0, nil
	}

	stat, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("unable to stat %s: %w", swiftSyncBitmapFileName, err)
	}

	if stat.Size() < headerSize {
		if err := file.Truncate(headerSize); err != nil {
			return 0, fmt.Errorf("unable to initialize bitmap header: %w", err)
		}
		stat, err = file.Stat()
		if err != nil {
			return 0, fmt.Errorf("unable to restat bitmap file: %w", err)
		}
	}

	header := make([]byte, headerSize)
	if _, err := file.ReadAt(header, 0); err != nil {
		return 0, fmt.Errorf("unable to read bitmap header: %w", err)
	}

	currentBits := binary.LittleEndian.Uint64(header)
	startBit := currentBits
	newTotal := currentBits + bitCount

	requiredBytes := (newTotal + 7) / 8
	requiredSize := int64(headerSize) + int64(requiredBytes)
	if stat.Size() < requiredSize {
		if err := file.Truncate(requiredSize); err != nil {
			return 0, fmt.Errorf("unable to extend bitmap file: %w", err)
		}
	}

	binary.LittleEndian.PutUint64(header, newTotal)
	if _, err := file.WriteAt(header, 0); err != nil {
		return 0, fmt.Errorf("unable to update bitmap header: %w", err)
	}

	return startBit, nil
}

// genOPMapping generates the outpoint -> bit index mapping by iterating through
// all blocks. It is resumable: progress is stored in PebbleDB and the function
// can be stopped via context cancellation.
func genOPMapping(ctx context.Context, pdb *pebble.DB, chain *blockchain.BlockChain, lastHash *chainhash.Hash) error {
	lastHeight, err := chain.BlockHeightByHash(lastHash)
	if err != nil {
		return fmt.Errorf("unable to get height for hash %v: %w", lastHash, err)
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

	// Determine starting height. We start from block 1 (skip genesis coinbase
	// which is unspendable) or resume from the next unprocessed block.
	startHeight := int32(1)
	if lastProcessedHeight > 0 {
		startHeight = int32(lastProcessedHeight) + 1
		log.Infof("Resuming from block %d (next bit index: %d)", startHeight, nextBitIndex)
	} else {
		log.Infof("Starting fresh from block %d", startHeight)
	}

	if startHeight > lastHeight {
		log.Infof("Already up to date, nothing to do")
		return nil
	}

	log.Infof("Processing blocks %d to %d", startHeight, lastHeight)

	batch := pdb.NewBatch()
	defer batch.Close()

	for height := startHeight; height <= lastHeight; height++ {
		// Check for cancellation.
		select {
		case <-ctx.Done():
			log.Infof("Interrupted at block %d, saving progress...", height)
			// Save progress up to the last fully processed block.
			if height > startHeight {
				setMetaUint64(batch, metaLastHeight, uint64(height-1))
				setMetaUint64(batch, metaNextBitIndex, nextBitIndex)
				if err := batch.Commit(pebble.Sync); err != nil {
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
				putTxStartIndex(batch, txHash, nextBitIndex)
				nextBitIndex += uint64(numOutputs)
			}
		}

		// Commit batch periodically to limit memory usage, provide
		// resumability checkpoints, and avoid exceeding pebble's 4GB batch limit.
		batchSize := batch.Len()
		if height == lastHeight || batchSize >= maxBatchSize {
			setMetaUint64(batch, metaLastHeight, uint64(height))
			setMetaUint64(batch, metaNextBitIndex, nextBitIndex)
			if err := batch.Commit(pebble.Sync); err != nil {
				return fmt.Errorf("unable to commit batch at height %d: %w", height, err)
			}
			batch.Reset()
		}

		// Log progress periodically.
		if height%progressInterval == 0 {
			log.Infof("Processed block %d / %d (bit index: %d)",
				height, lastHeight, nextBitIndex)
		}
	}

	log.Infof("Mapping complete. Total outputs: %d", nextBitIndex)
	return nil
}

// genBitmap generates the spent outpoint bitmap by iterating through all blocks
// and marking spent outpoints. It is resumable: progress is stored in PebbleDB.
func genBitmap(ctx context.Context, pdb *pebble.DB, chain *blockchain.BlockChain, lastHash *chainhash.Hash) error {
	lastHeight, err := chain.BlockHeightByHash(lastHash)
	if err != nil {
		return fmt.Errorf("unable to get height for hash %v: %w", lastHash, err)
	}

	// Get the total number of bits needed from the mapping phase.
	totalBits, err := getMetaUint64(pdb, metaNextBitIndex)
	if err != nil {
		return fmt.Errorf("unable to read total bit count: %w", err)
	}
	if totalBits == 0 {
		return fmt.Errorf("no outpoint mapping found; run mapping phase first")
	}

	// Open/create the bitmap file.
	filePath := filepath.Join(cfg.DataDir, swiftSyncBitmapFileName)
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("unable to open %s: %w", swiftSyncBitmapFileName, err)
	}
	defer file.Close()

	// Allocate space for all bits if not already done.
	_, err = allocateSwiftSyncBitmapBits(file, totalBits)
	if err != nil {
		return fmt.Errorf("unable to allocate bitmap: %w", err)
	}

	// Load progress state.
	lastProcessedHeight, err := getMetaUint64(pdb, metaBitmapLastHeight)
	if err != nil {
		return fmt.Errorf("unable to read bitmap last height: %w", err)
	}

	// Determine starting height.
	startHeight := int32(1)
	if lastProcessedHeight > 0 {
		startHeight = int32(lastProcessedHeight) + 1
		log.Infof("Resuming bitmap generation from block %d", startHeight)
	} else {
		log.Infof("Starting bitmap generation from block %d", startHeight)
	}

	if startHeight > lastHeight {
		log.Infof("Bitmap already up to date, nothing to do")
		return nil
	}

	log.Infof("Generating bitmap for blocks %d to %d", startHeight, lastHeight)

	// Cache for txhash -> starting bit index lookups.
	cache := newTxStartIndexCache(txCacheSize)

	// Process blocks in batches for efficient sorted iterator lookups.
	for batchStart := startHeight; batchStart <= lastHeight; batchStart += bitmapBatchBlocks {
		// Check for cancellation.
		select {
		case <-ctx.Done():
			log.Infof("Interrupted at block %d, saving progress...", batchStart)
			if batchStart > startHeight {
				if err := setMetaUint64Direct(pdb, metaBitmapLastHeight, uint64(batchStart-1)); err != nil {
					return fmt.Errorf("unable to save progress: %w", err)
				}
			}
			return ctx.Err()
		default:
		}

		// Collect all outpoints from this batch of blocks.
		outpoints := make([]wire.OutPoint, 0, 50000)
		batchEnd := min(batchStart+bitmapBatchBlocks-1, lastHeight)
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

		// Look up all bit indices using sorted iterator.
		bitIndices, err := lookupOutpointsBatch(pdb, cache, outpoints)
		if err != nil {
			return fmt.Errorf("unable to lookup outpoints at blocks %d-%d: %w",
				batchStart, batchEnd, err)
		}

		// Mark all bits as spent.
		if len(bitIndices) > 0 {
			if err := markBitsSpent(file, bitIndices); err != nil {
				return fmt.Errorf("unable to write bits at blocks %d-%d: %w",
					batchStart, batchEnd, err)
			}
		}

		// Save progress.
		if err := setMetaUint64Direct(pdb, metaBitmapLastHeight, uint64(batchEnd)); err != nil {
			return fmt.Errorf("unable to save bitmap progress: %w", err)
		}

		log.Infof("Bitmap progress: block %d / %d", batchEnd, lastHeight)
	}

	log.Infof("Bitmap generation complete")
	return nil
}

// markBitsSpent sets the specified bits to 1 in the bitmap file.
func markBitsSpent(file *os.File, bitIndices []uint64) error {
	if len(bitIndices) == 0 {
		return nil
	}

	// Find min and max byte indices to determine the range to read.
	minByte, maxByte := bitIndices[0]/8, bitIndices[0]/8
	for _, bitIdx := range bitIndices[1:] {
		byteIdx := bitIdx / 8
		if byteIdx < minByte {
			minByte = byteIdx
		}
		if byteIdx > maxByte {
			maxByte = byteIdx
		}
	}

	// Read the entire byte range at once.
	rangeSize := maxByte - minByte + 1
	buf := make([]byte, rangeSize)
	readOffset := int64(headerSize) + int64(minByte)
	if _, err := file.ReadAt(buf, readOffset); err != nil && err != io.EOF {
		return fmt.Errorf("unable to read bytes at offset %d: %w", readOffset, err)
	}

	// Set all the bits in memory.
	for _, bitIdx := range bitIndices {
		byteIdx := bitIdx / 8
		bitPos := bitIdx % 8
		buf[byteIdx-minByte] |= 1 << bitPos
	}

	// Write the entire range back at once.
	if _, err := file.WriteAt(buf, readOffset); err != nil {
		return fmt.Errorf("unable to write bytes at offset %d: %w", readOffset, err)
	}

	return nil
}

// setMetaUint64Direct stores a uint64 metadata value directly (not in a batch).
func setMetaUint64Direct(db *pebble.DB, name string, value uint64) error {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, value)
	return db.Set(metaKey(name), buf, pebble.Sync)
}

// deleteMetaKey deletes a metadata key from PebbleDB.
func deleteMetaKey(db *pebble.DB, name string) error {
	return db.Delete(metaKey(name), pebble.Sync)
}

// deleteBitmap removes the bitmap file and its progress metadata.
func deleteBitmap(pdb *pebble.DB) error {
	// Delete the bitmap file.
	filePath := filepath.Join(cfg.DataDir, swiftSyncBitmapFileName)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("unable to delete bitmap file: %w", err)
	}
	log.Infof("Deleted bitmap file: %s", filePath)

	// Delete the bitmap progress metadata.
	if err := deleteMetaKey(pdb, metaBitmapLastHeight); err != nil && err != pebble.ErrNotFound {
		return fmt.Errorf("unable to delete bitmap metadata: %w", err)
	}
	log.Infof("Deleted bitmap progress metadata")

	return nil
}

// deleteMapping removes the outpoint mapping database and its metadata.
func deleteMapping(pdb *pebble.DB) error {
	// Delete the mapping metadata first (while db is still open).
	if err := deleteMetaKey(pdb, metaLastHeight); err != nil && err != pebble.ErrNotFound {
		return fmt.Errorf("unable to delete mapping last_height metadata: %w", err)
	}
	if err := deleteMetaKey(pdb, metaNextBitIndex); err != nil && err != pebble.ErrNotFound {
		return fmt.Errorf("unable to delete mapping next_bit_index metadata: %w", err)
	}
	log.Infof("Deleted mapping progress metadata")

	// Close the pebble db before deleting its directory.
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

	// Open the PebbleDB for storing the outpoint -> index mapping.
	dbPath := filepath.Join(cfg.DataDir, opMappingDbName)
	pdb, err := newPebbleDB(dbPath)
	if err != nil {
		log.Errorf("Failed to open mapping database: %v", err)
		return
	}

	// Handle deletion flags.
	if cfg.DeleteBitmap {
		if err := deleteBitmap(pdb); err != nil {
			log.Errorf("Failed to delete bitmap: %v", err)
		} else {
			log.Infof("Bitmap deletion complete")
		}
		pdb.Close()
		return
	}

	if cfg.DeleteMapping {
		// deleteBitmap first since it depends on pdb being open for metadata.
		// Also delete bitmap since it's useless without the mapping.
		if err := deleteBitmap(pdb); err != nil {
			log.Errorf("Failed to delete bitmap: %v", err)
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

	// Generate the outpoint -> bit index mapping.
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

	// Generate the bitmap marking spent outpoints.
	err = genBitmap(ctx, pdb, chain, hash)
	if err != nil {
		if err == context.Canceled {
			log.Infof("Bitmap generation stopped. Run again to resume.")
		} else {
			log.Errorf("Failed to generate bitmap: %v", err)
		}
		return
	}

	log.Infof("Bitmap generation complete!")

	// Write memory profile if requested.
	if cfg.MemProfile != "" {
		f, err := os.Create(cfg.MemProfile)
		if err != nil {
			log.Errorf("Failed to create memory profile: %v", err)
			return
		}
		defer f.Close()
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Errorf("Failed to write memory profile: %v", err)
			return
		}
	}
}
