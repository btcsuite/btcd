package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/cockroachdb/pebble"
	"github.com/stretchr/testify/require"
)

func TestAllocateSwiftSyncBitmapBits(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_bitmap.dat")

	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0o644)
	require.NoError(t, err)
	defer file.Close()

	// First allocation should start at 0.
	startBit, err := allocateSwiftSyncBitmapBits(file, 100)
	require.NoError(t, err)
	require.Equal(t, uint64(0), startBit)

	// Check file size: header (8) + ceil(100/8) = 8 + 13 = 21 bytes.
	stat, err := file.Stat()
	require.NoError(t, err)
	require.Equal(t, int64(headerSize+13), stat.Size())

	// Second allocation should start at 100.
	startBit, err = allocateSwiftSyncBitmapBits(file, 50)
	require.NoError(t, err)
	require.Equal(t, uint64(100), startBit)

	// Check file size: header (8) + ceil(150/8) = 8 + 19 = 27 bytes.
	stat, err = file.Stat()
	require.NoError(t, err)
	require.Equal(t, int64(headerSize+19), stat.Size())

	// Zero allocation should return current position without changing anything.
	startBit, err = allocateSwiftSyncBitmapBits(file, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(0), startBit)
}

func TestMarkBitsSpent(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_bitmap.dat")

	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0o644)
	require.NoError(t, err)
	defer file.Close()

	// Allocate 64 bits.
	_, err = allocateSwiftSyncBitmapBits(file, 64)
	require.NoError(t, err)

	// Mark some bits as spent.
	spentBits := []uint64{0, 7, 8, 15, 63}
	err = markBitsSpent(file, spentBits)
	require.NoError(t, err)

	// Read the bitmap and verify.
	bitmap := make([]byte, 8)
	_, err = file.ReadAt(bitmap, headerSize)
	require.NoError(t, err)

	// Check specific bits.
	expectedSet := []uint64{0, 7, 8, 15, 63}
	expectedUnset := []uint64{1, 2, 9, 62}

	for _, bitIdx := range expectedSet {
		byteIdx := bitIdx / 8
		bitPos := bitIdx % 8
		isSet := (bitmap[byteIdx] & (1 << bitPos)) != 0
		require.True(t, isSet, "bit %d should be set", bitIdx)
	}

	for _, bitIdx := range expectedUnset {
		byteIdx := bitIdx / 8
		bitPos := bitIdx % 8
		isSet := (bitmap[byteIdx] & (1 << bitPos)) != 0
		require.False(t, isSet, "bit %d should not be set", bitIdx)
	}
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
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "testdb")

	db, err := pebble.Open(dbPath, &pebble.Options{})
	require.NoError(t, err)
	defer db.Close()

	batch := db.NewBatch()
	defer batch.Close()

	// Create test transaction hashes.
	hash1 := &chainhash.Hash{0x01, 0x02, 0x03}
	hash2 := &chainhash.Hash{0xaa, 0xbb, 0xcc}

	// Store start indices.
	putTxStartIndex(batch, hash1, 0)
	putTxStartIndex(batch, hash2, 100)

	err = batch.Commit(pebble.Sync)
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

func TestLookupOutpointsBatch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "testdb")

	db, err := pebble.Open(dbPath, &pebble.Options{})
	require.NoError(t, err)
	defer db.Close()

	// Populate database with test data.
	batch := db.NewBatch()
	hashes := make([]chainhash.Hash, 5)
	for i := 0; i < 5; i++ {
		hashes[i] = chainhash.Hash{byte(i + 1)}
		putTxStartIndex(batch, &hashes[i], uint64(i*10))
	}
	err = batch.Commit(pebble.Sync)
	require.NoError(t, err)
	batch.Close()

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
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "testdb")

	db, err := pebble.Open(dbPath, &pebble.Options{})
	require.NoError(t, err)
	defer db.Close()

	// Populate database with only hash2.
	hash1 := chainhash.Hash{0x01}
	hash2 := chainhash.Hash{0x02}
	batch := db.NewBatch()
	putTxStartIndex(batch, &hash2, 200)
	err = batch.Commit(pebble.Sync)
	require.NoError(t, err)
	batch.Close()

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
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "testdb")

	db, err := pebble.Open(dbPath, &pebble.Options{})
	require.NoError(t, err)
	defer db.Close()

	// Test setMetaUint64 with batch.
	batch := db.NewBatch()
	setMetaUint64(batch, "test_value", 12345678901234)
	err = batch.Commit(pebble.Sync)
	require.NoError(t, err)
	batch.Close()

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

func TestMarkBitsSpentEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_bitmap.dat")

	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0o644)
	require.NoError(t, err)
	defer file.Close()

	// Empty slice should not error.
	err = markBitsSpent(file, []uint64{})
	require.NoError(t, err)
}

func TestLookupOutpointsBatchEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "testdb")

	db, err := pebble.Open(dbPath, &pebble.Options{})
	require.NoError(t, err)
	defer db.Close()

	cache := newTxStartIndexCache(100)
	results, err := lookupOutpointsBatch(db, cache, []wire.OutPoint{})
	require.NoError(t, err)
	require.Nil(t, results)
}

func TestDeleteMetaKey(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "testdb")

	db, err := pebble.Open(dbPath, &pebble.Options{})
	require.NoError(t, err)
	defer db.Close()

	// Set a value.
	err = setMetaUint64Direct(db, "to_delete", 12345)
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
