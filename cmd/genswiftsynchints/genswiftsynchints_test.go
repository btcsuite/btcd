package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/cockroachdb/pebble/v2"
	"github.com/stretchr/testify/require"
)

// openTestDB opens a Pebble database in a fresh temp directory that is closed
// and removed when the test finishes.
func openTestDB(t *testing.T) *pebble.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "testdb")
	db, err := pebble.Open(dbPath, &pebble.Options{})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	return db
}

func TestWriteHintsBlock(t *testing.T) {
	tests := []struct {
		name    string
		indices []uint32
		wantHex string
	}{
		{
			name:    "empty",
			indices: nil,
			wantHex: "00",
		},
		{
			name:    "consecutive from zero",
			indices: []uint32{0, 1, 2, 3},
			wantHex: "0400000000",
		},
		{
			name: "regular gaps",
			indices: []uint32{
				13, 16, 19, 22, 25, 28, 31, 34, 37, 40,
			},
			wantHex: "0a0d020202020202020202",
		},
		{
			name:    "single multi-byte index",
			indices: []uint32{300},
			wantHex: "03fd2c01",
		},
		{
			name:    "mixed gap widths",
			indices: []uint32{5, 500},
			wantHex: "0405fdee01",
		},
		{
			name:    "max index",
			indices: []uint32{math.MaxUint32},
			wantHex: "05feffffffff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := writeHintsBlock(&buf, tt.indices)
			require.NoError(t, err)
			require.Equal(t, tt.wantHex, hex.EncodeToString(buf.Bytes()))
		})
	}
}

func TestWriteHintsBlockRejectsNonIncreasing(t *testing.T) {
	for _, indices := range [][]uint32{
		{1, 3, 3}, // duplicate
		{3, 1},    // out of order
	} {
		var buf bytes.Buffer
		err := writeHintsBlock(&buf, indices)
		require.Error(t, err)
		require.Empty(t, buf.Bytes())
	}
}

func TestWriteSwiftSyncHintsHeader(t *testing.T) {
	var buf bytes.Buffer
	err := writeSwiftSyncHintsHeader(&buf, 285205)
	require.NoError(t, err)

	require.Equal(t, []byte{
		0x55, 0x54, 0x58, 0x4f, // magic "UTXO"
		0x01,                   // version
		0x15, 0x5a, 0x04, 0x00, // height 285205, little-endian
	}, buf.Bytes())
}

func TestHintsFileMatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, swiftSyncHintsFileName)

	// Missing file: matches=false, err=nil.
	matches, err := hintsFileMatches(path, 285205)
	require.NoError(t, err)
	require.False(t, matches)

	writeHeader := func(magic [4]byte, version byte, height uint32) {
		f, err := os.Create(path)
		require.NoError(t, err)
		zw := gzip.NewWriter(f)
		_, err = zw.Write(magic[:])
		require.NoError(t, err)
		_, err = zw.Write([]byte{version})
		require.NoError(t, err)
		var hb [4]byte
		binary.LittleEndian.PutUint32(hb[:], height)
		_, err = zw.Write(hb[:])
		require.NoError(t, err)
		require.NoError(t, zw.Close())
		require.NoError(t, f.Close())
	}

	// Correct header for the requested height.
	writeHeader(swiftSyncHintsMagic, swiftSyncHintsVersion, 285205)
	matches, err = hintsFileMatches(path, 285205)
	require.NoError(t, err)
	require.True(t, matches)

	// Same file, different requested height: must not match.
	matches, err = hintsFileMatches(path, 285206)
	require.NoError(t, err)
	require.False(t, matches)

	// Wrong magic.
	writeHeader([4]byte{'B', 'A', 'D', '!'}, swiftSyncHintsVersion, 285205)
	matches, err = hintsFileMatches(path, 285205)
	require.NoError(t, err)
	require.False(t, matches)

	// Wrong version.
	writeHeader(swiftSyncHintsMagic, 0xff, 285205)
	matches, err = hintsFileMatches(path, 285205)
	require.NoError(t, err)
	require.False(t, matches)

	// Truncated, non-gzip file.
	require.NoError(t, os.WriteFile(path, []byte{0x55, 0x54}, 0o644))
	matches, err = hintsFileMatches(path, 285205)
	require.NoError(t, err)
	require.False(t, matches)
}

func TestSaveLoadSpentBitmap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, spentBitmapFileName)

	// Missing file: zeroed bitset, height 0.
	spent, height, err := loadSpentBitmap(path, 8, 100)
	require.NoError(t, err)
	require.Equal(t, int32(0), height)
	require.Equal(t, make([]byte, 8), spent)

	// Round-trip at the same target height.
	saved := []byte{0xff, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x01}
	require.NoError(t, saveSpentBitmap(path, saved, 100))
	require.NoFileExists(t, path+".tmp")

	// The on-disk layout is pinned: magic, version, marked height
	// (uint32 LE), payload length (uint64 LE), payload.
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, append([]byte{
		0x53, 0x53, 0x42, 0x4d,
		0x01,
		0x64, 0x00, 0x00, 0x00,
		0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}, saved...), raw)

	spent, height, err = loadSpentBitmap(path, 8, 100)
	require.NoError(t, err)
	require.Equal(t, int32(100), height)
	require.Equal(t, saved, spent)

	// Higher target: the bitset grows and the tail stays zeroed.
	spent, height, err = loadSpentBitmap(path, 12, 150)
	require.NoError(t, err)
	require.Equal(t, int32(100), height)
	require.Equal(t, append(append([]byte{}, saved...), 0, 0, 0, 0), spent)

	// Target below the marked height: rebuild from scratch.
	spent, height, err = loadSpentBitmap(path, 8, 99)
	require.NoError(t, err)
	require.Equal(t, int32(0), height)
	require.Equal(t, make([]byte, 8), spent)

	// Truncated payload: rebuild.
	require.NoError(t, os.WriteFile(path, raw[:len(raw)-3], 0o644))
	spent, height, err = loadSpentBitmap(path, 8, 100)
	require.NoError(t, err)
	require.Equal(t, int32(0), height)
	require.Equal(t, make([]byte, 8), spent)

	// Payload missing entirely (header only): rebuild.
	require.NoError(t, os.WriteFile(path, raw[:17], 0o644))
	spent, height, err = loadSpentBitmap(path, 8, 100)
	require.NoError(t, err)
	require.Equal(t, int32(0), height)
	require.Equal(t, make([]byte, 8), spent)

	// Smaller bitset with an all-zero leftover payload: the leftover is
	// ignored.
	saved = []byte{0xaa, 0xbb, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	require.NoError(t, saveSpentBitmap(path, saved, 50))

	spent, height, err = loadSpentBitmap(path, 2, 60)
	require.NoError(t, err)
	require.Equal(t, int32(50), height)
	require.Equal(t, []byte{0xaa, 0xbb}, spent)

	// Smaller bitset with a set bit in the leftover payload: rebuild.
	spent, height, err = loadSpentBitmap(path, 1, 60)
	require.NoError(t, err)
	require.Equal(t, int32(0), height)
	require.Equal(t, make([]byte, 1), spent)

	// Truncated header: rebuild.
	require.NoError(t, os.WriteFile(path, []byte{0x53, 0x53}, 0o644))
	spent, height, err = loadSpentBitmap(path, 8, 100)
	require.NoError(t, err)
	require.Equal(t, int32(0), height)
	require.Equal(t, make([]byte, 8), spent)

	// Wrong magic: rebuild.
	bad := append([]byte{}, raw...)
	copy(bad[0:4], "BAD!")
	require.NoError(t, os.WriteFile(path, bad, 0o644))
	spent, height, err = loadSpentBitmap(path, 8, 100)
	require.NoError(t, err)
	require.Equal(t, int32(0), height)
	require.Equal(t, make([]byte, 8), spent)

	// Wrong version: rebuild.
	bad = append([]byte{}, raw...)
	bad[4] = 0xff
	require.NoError(t, os.WriteFile(path, bad, 0o644))
	spent, height, err = loadSpentBitmap(path, 8, 100)
	require.NoError(t, err)
	require.Equal(t, int32(0), height)
	require.Equal(t, make([]byte, 8), spent)
}

func TestTxStartIndexCache(t *testing.T) {
	cache := newTxStartIndexCache(3)

	hash1 := chainhash.Hash{0x01}
	hash2 := chainhash.Hash{0x02}
	hash3 := chainhash.Hash{0x03}
	hash4 := chainhash.Hash{0x04}

	// Cache should be empty initially.
	_, ok := cache.get(hash1)
	require.False(t, ok, "cache should be empty initially")

	// Add entries.
	cache.put(hash1, 100)
	cache.put(hash2, 200)
	cache.put(hash3, 300)

	// Verify entries.
	val, ok := cache.get(hash1)
	require.True(t, ok)
	require.Equal(t, uint64(100), val)

	val, ok = cache.get(hash2)
	require.True(t, ok)
	require.Equal(t, uint64(200), val)

	val, ok = cache.get(hash3)
	require.True(t, ok)
	require.Equal(t, uint64(300), val)

	// Adding 4th entry should clear the cache and add only hash4.
	cache.put(hash4, 400)

	// Previous entries should be gone (cache was cleared).
	_, ok = cache.get(hash1)
	require.False(t, ok, "hash1 should have been cleared")
	_, ok = cache.get(hash2)
	require.False(t, ok, "hash2 should have been cleared")
	_, ok = cache.get(hash3)
	require.False(t, ok, "hash3 should have been cleared")

	// New entry should be present.
	val, ok = cache.get(hash4)
	require.True(t, ok)
	require.Equal(t, uint64(400), val)
}

func TestPutAndGetTxStartIndex(t *testing.T) {
	db := openTestDB(t)

	batch := db.NewBatch()
	defer batch.Close()

	// Create test transaction hashes.
	hash1 := &chainhash.Hash{0x01, 0x02, 0x03}
	hash2 := &chainhash.Hash{0xaa, 0xbb, 0xcc}

	// Store start indices.
	require.NoError(t, putTxStartIndex(batch, hash1, 0))
	require.NoError(t, putTxStartIndex(batch, hash2, 100))

	err := batch.Commit(syncWriteOptions)
	require.NoError(t, err)

	// Create cache and lookup.
	cache := newTxStartIndexCache(100)

	// Test lookups for different output indices.
	tests := []struct {
		hash     chainhash.Hash
		outIdx   uint32
		expected uint64
	}{
		{*hash1, 0, 0},
		{*hash1, 1, 1},
		{*hash1, 5, 5},
		{*hash1, 99, 99},
		{*hash2, 0, 100},
		{*hash2, 3, 103},
		{*hash2, 100, 200},
	}

	for _, tt := range tests {
		op := wire.OutPoint{Hash: tt.hash, Index: tt.outIdx}
		result, err := getOutpointBitIndex(db, cache, op)
		require.NoError(t, err)
		require.Equal(t, tt.expected, result, "getOutpointBitIndex(%v, %d)", tt.hash, tt.outIdx)
	}

	// Verify cache was populated.
	_, ok := cache.get(*hash1)
	require.True(t, ok, "hash1 should be in cache")
	_, ok = cache.get(*hash2)
	require.True(t, ok, "hash2 should be in cache")
}

func TestReadTxStartIndexTreatsInvalidLengthAsUnmapped(t *testing.T) {
	db := openTestDB(t)

	hash := &chainhash.Hash{0x01, 0x02, 0x03}
	err := db.Set(hash[:], []byte{0x01, 0x02}, syncWriteOptions)
	require.NoError(t, err)

	startIndex, mapped, err := readTxStartIndex(db, hash)
	require.NoError(t, err)
	require.False(t, mapped)
	require.Zero(t, startIndex)

	cache := newTxStartIndexCache(100)
	_, err = getOutpointBitIndex(db, cache, wire.OutPoint{Hash: *hash})
	require.ErrorContains(t, err, "txhash mapping missing or invalid")
}

func TestLookupOutpointsBatch(t *testing.T) {
	db := openTestDB(t)

	// Populate database with test data.
	batch := db.NewBatch()
	defer batch.Close()
	hashes := make([]chainhash.Hash, 5)
	for i := 0; i < 5; i++ {
		hashes[i] = chainhash.Hash{byte(i + 1)}
		require.NoError(t, putTxStartIndex(batch, &hashes[i], uint64(i*10)))
	}
	err := batch.Commit(syncWriteOptions)
	require.NoError(t, err)

	// Create outpoints in random order.
	outpoints := []wire.OutPoint{
		{Hash: hashes[3], Index: 2}, // expected: 30 + 2 = 32
		{Hash: hashes[0], Index: 0}, // expected: 0 + 0 = 0
		{Hash: hashes[3], Index: 5}, // expected: 30 + 5 = 35 (same tx as first)
		{Hash: hashes[1], Index: 1}, // expected: 10 + 1 = 11
		{Hash: hashes[4], Index: 3}, // expected: 40 + 3 = 43
		{Hash: hashes[4], Index: 9}, // expected: 40 + 9 = 49 (same tx as hashes[4])
		{Hash: hashes[0], Index: 9}, // expected: 0 + 9 = 9
		{Hash: hashes[1], Index: 0}, // expected: 10 + 0 = 10
	}

	expected := []uint64{32, 0, 35, 11, 43, 49, 9, 10}

	cache := newTxStartIndexCache(100)
	results, err := lookupOutpointsBatch(db, cache, outpoints)
	require.NoError(t, err)
	require.Equal(t, expected, results)

	// Verify cache was populated for hashes that were actually looked up.
	usedHashes := []chainhash.Hash{hashes[0], hashes[1], hashes[3], hashes[4]}
	for _, h := range usedHashes {
		_, ok := cache.get(h)
		require.True(t, ok, "hash %v should be in cache", h)
	}
}

func TestLookupOutpointsBatchWithCacheHits(t *testing.T) {
	db := openTestDB(t)

	// Populate database with only hash2.
	hash1 := chainhash.Hash{0x01}
	hash2 := chainhash.Hash{0x02}
	batch := db.NewBatch()
	defer batch.Close()
	require.NoError(t, putTxStartIndex(batch, &hash2, 200))
	err := batch.Commit(syncWriteOptions)
	require.NoError(t, err)

	// Pre-populate cache with hash1 (not in DB).
	// If cache isn't used, lookup will fail.
	cache := newTxStartIndexCache(2)
	cache.put(hash1, 100)

	outpoints := []wire.OutPoint{
		{Hash: hash1, Index: 5}, // must hit cache (not in DB)
		{Hash: hash2, Index: 3}, // must hit db (not in cache)
	}

	results, err := lookupOutpointsBatch(db, cache, outpoints)
	require.NoError(t, err)
	require.Equal(t, uint64(105), results[0])
	require.Equal(t, uint64(203), results[1])
}

func TestMetaKey(t *testing.T) {
	key := metaKey("test_key")
	require.Len(t, key, 1+len("test_key"))
	require.Equal(t, byte(metaPrefix), key[0])
	require.Equal(t, "test_key", string(key[1:]))
}

func TestMetaUint64(t *testing.T) {
	db := openTestDB(t)

	// Test setMetaUint64 with batch.
	batch := db.NewBatch()
	defer batch.Close()
	require.NoError(t, setMetaUint64(batch, "test_value", 12345678901234))
	err := batch.Commit(syncWriteOptions)
	require.NoError(t, err)

	// Test getMetaUint64.
	val, err := getMetaUint64(db, "test_value")
	require.NoError(t, err)
	require.Equal(t, uint64(12345678901234), val)

	// Test non-existent key returns 0.
	val, err = getMetaUint64(db, "nonexistent")
	require.NoError(t, err)
	require.Equal(t, uint64(0), val)

	// Test setMetaUint64Direct.
	err = setMetaUint64Direct(db, "direct_value", 9999)
	require.NoError(t, err)

	val, err = getMetaUint64(db, "direct_value")
	require.NoError(t, err)
	require.Equal(t, uint64(9999), val)
}

func TestLookupOutpointsBatchEmpty(t *testing.T) {
	db := openTestDB(t)

	cache := newTxStartIndexCache(100)
	results, err := lookupOutpointsBatch(db, cache, []wire.OutPoint{})
	require.NoError(t, err)
	require.Nil(t, results)
}

func TestDeleteMetaKey(t *testing.T) {
	db := openTestDB(t)

	// Set a value.
	err := setMetaUint64Direct(db, "to_delete", 12345)
	require.NoError(t, err)

	// Verify it exists.
	val, err := getMetaUint64(db, "to_delete")
	require.NoError(t, err)
	require.Equal(t, uint64(12345), val)

	// Delete it.
	err = deleteMetaKey(db, "to_delete")
	require.NoError(t, err)

	// Verify it's gone (should return 0).
	val, err = getMetaUint64(db, "to_delete")
	require.NoError(t, err)
	require.Equal(t, uint64(0), val)
}

func TestScanPartialHints(t *testing.T) {
	const height = 100

	// build returns a header followed by the given block entries, plus the byte
	// offset just past each (offs[0] ends the header, offs[i] ends block i).
	build := func(blocks [][]uint32) ([]byte, []int64) {
		var buf bytes.Buffer
		require.NoError(t, writeSwiftSyncHintsHeader(&buf, height))
		offs := []int64{int64(buf.Len())}
		for _, b := range blocks {
			require.NoError(t, writeHintsBlock(&buf, b))
			offs = append(offs, int64(buf.Len()))
		}
		return buf.Bytes(), offs
	}

	header, _ := build(nil)
	full, offs := build([][]uint32{{0, 1, 2}, {5}, {0, 4, 9, 14}})
	wrongMagic := append([]byte(nil), full...)
	wrongMagic[0] = 'X'

	tests := []struct {
		name       string
		data       []byte // file contents to write
		absent     bool   // leave the file uncreated instead
		height     uint32
		wantBlocks int32
		wantOff    int64
		wantOK     bool
	}{
		{
			name:   "missing file",
			absent: true,
			height: height,
		},
		{
			name:    "header only",
			data:    header,
			height:  height,
			wantOff: swiftSyncHintsHeaderSize,
			wantOK:  true,
		},
		{
			name:       "three complete blocks",
			data:       full,
			height:     height,
			wantBlocks: 3,
			wantOff:    int64(len(full)),
			wantOK:     true,
		},
		{
			// Last block's payload truncated: only the two complete blocks
			// count, and the offset stops at the end of block 2.
			name:       "torn final entry",
			data:       full[:len(full)-2],
			height:     height,
			wantBlocks: 2,
			wantOff:    offs[2],
			wantOK:     true,
		},
		{
			name:   "wrong height",
			data:   full,
			height: height + 1,
		},
		{
			name:   "wrong magic",
			data:   wrongMagic,
			height: height,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), swiftSyncHintsFileName+".raw")
			if !tt.absent {
				require.NoError(t, os.WriteFile(path, tt.data, 0o644))
			}

			blocks, off, ok, err := scanPartialHints(path, tt.height)
			require.NoError(t, err)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantBlocks, blocks)
			require.Equal(t, tt.wantOff, off)
		})
	}
}

func TestGzipFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "raw")
	dst := filepath.Join(dir, swiftSyncHintsFileName)

	payload := bytes.Repeat([]byte("swift sync hints payload "), 4096)
	require.NoError(t, os.WriteFile(src, payload, 0o644))

	require.NoError(t, gzipFile(src, dst))
	require.NoFileExists(t, dst+".tmp")

	f, err := os.Open(dst)
	require.NoError(t, err)
	defer f.Close()
	zr, err := gzip.NewReader(f)
	require.NoError(t, err)
	got, err := io.ReadAll(zr)
	require.NoError(t, err)
	require.Equal(t, payload, got)
}

// gunzipFile returns the decompressed contents of a gzipped file.
func gunzipFile(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	zr, err := gzip.NewReader(f)
	require.NoError(t, err)
	got, err := io.ReadAll(zr)
	require.NoError(t, err)
	return got
}

// loadFirstBlocks reads the first count+1 blocks (genesis through height count)
// from a btcd block fixture, each framed as a 4-byte magic, 4-byte length, then
// the serialized block.
func loadFirstBlocks(t *testing.T, path string, count int) []*btcutil.Block {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	var blocks []*btcutil.Block
	for len(blocks) <= count {
		var magic uint32
		if err := binary.Read(f, binary.LittleEndian, &magic); err != nil {
			break // end of file
		}
		require.Equal(t, uint32(wire.MainNet), magic)

		var blockLen uint32
		require.NoError(t, binary.Read(f, binary.LittleEndian, &blockLen))
		raw := make([]byte, blockLen)
		_, err := io.ReadFull(f, raw)
		require.NoError(t, err)

		blk, err := btcutil.NewBlockFromBytes(raw)
		require.NoError(t, err)
		blocks = append(blocks, blk)
	}
	require.GreaterOrEqual(t, len(blocks), count+1)
	return blocks
}

// TestGenHintsFileResume checks that resuming genHintsFile from a partial .raw
// working file produces output byte-identical to an uninterrupted run.
func TestGenHintsFileResume(t *testing.T) {
	// Past the first non-coinbase spend (mainnet block 170), so the resumed
	// region has blocks with real spends, not just coinbase outputs.
	const target = 200

	dir := t.TempDir()

	// The pipeline reads these globals. Restore them for other tests.
	origCfg, origNet := cfg, activeNetParams
	t.Cleanup(func() { cfg, activeNetParams = origCfg, origNet })
	cfg = &config{DataDir: dir}
	activeNetParams = &chaincfg.MainNetParams

	db, err := database.Create(blockDbType,
		filepath.Join(dir, blockDbNamePrefix+"_"+blockDbType), wire.MainNet)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	chain, err := blockchain.New(&blockchain.Config{
		DB:          db,
		ChainParams: activeNetParams,
		TimeSource:  blockchain.NewMedianTime(),
	})
	require.NoError(t, err)

	// Connect the first `target` mainnet blocks. The relative path resolves
	// because `go test` runs from the package directory.
	blocks := loadFirstBlocks(t,
		"../../blockchain/testdata/blk_0_to_14131.dat", target)
	for _, blk := range blocks[1:] {
		_, _, err := chain.ProcessBlock(blk, blockchain.BFNone)
		require.NoError(t, err)
	}
	targetHash := blocks[target].Hash()

	pdb, err := newPebbleDB(filepath.Join(dir, opMappingDbName))
	require.NoError(t, err)
	t.Cleanup(func() { pdb.Close() })

	// Build the mapping and spent bitmap, then the reference hintsfile. A
	// complete run leaves no .raw behind.
	ctx := context.Background()
	require.NoError(t, genOPMapping(ctx, pdb, chain, targetHash))
	require.NoError(t, genSpentBitmap(ctx, pdb, chain, targetHash))
	require.NoError(t, genHintsFile(ctx, pdb, chain, targetHash))

	hintsPath := filepath.Join(dir, swiftSyncHintsFileName)
	reference := gunzipFile(t, hintsPath)
	require.NoFileExists(t, hintsPath+".raw")

	// The reference is exactly the uncompressed .raw bytes, so a prefix ending
	// after k whole entries is the working file a clean interrupt would leave.
	offsetAfterBlocks := func(data []byte, k int) int {
		r := bytes.NewReader(data[swiftSyncHintsHeaderSize:])
		for i := 0; i < k; i++ {
			byteLen, err := wire.ReadVarInt(r, 0)
			require.NoError(t, err)
			_, err = r.Seek(int64(byteLen), io.SeekCurrent)
			require.NoError(t, err)
		}
		return swiftSyncHintsHeaderSize + int(r.Size()) - r.Len()
	}
	cut := offsetAfterBlocks(reference, target/2)

	// Seed the working file, rerun, and require byte-identical output. Remove
	// the final file first so genHintsFile resumes instead of skipping.
	resume := func(t *testing.T, partial []byte) {
		require.NoError(t, os.Remove(hintsPath))
		require.NoError(t, os.WriteFile(hintsPath+".raw", partial, 0o644))

		require.NoError(t, genHintsFile(ctx, pdb, chain, targetHash))

		require.Equal(t, reference, gunzipFile(t, hintsPath))
		require.NoFileExists(t, hintsPath+".raw")
	}

	// Clean interrupt: working file ends on a block boundary.
	t.Run("clean block boundary", func(t *testing.T) {
		resume(t, reference[:cut])
	})

	// Torn entry: a length prefix claiming five payload bytes with none
	// present, which scanPartialHints must discard and rewrite.
	t.Run("torn trailing entry", func(t *testing.T) {
		torn := append(append([]byte(nil), reference[:cut]...), 0x05, 0x00)
		resume(t, torn)
	})
}
