// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is part of the ffldb package rather than the ffldb_test package as
// it provides whitebox testing of the cold-tier block storage.

package ffldb

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/blockcompress"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire/v2"
)

// coldTestStore bundles a hot blockStore and a coldStore sharing a temp dir, for
// cold-tier whitebox tests. cleanup removes the temp dir.
type coldTestStore struct {
	hot  *blockStore
	cold *coldStore
	dir  string
}

func newColdTestStore(t *testing.T) (*coldTestStore, func()) {
	t.Helper()
	dir := t.TempDir()
	hot, err := newBlockStore(dir, wire.MainNet)
	if err != nil {
		t.Fatalf("newBlockStore: %v", err)
	}
	cold, err := newColdStore(dir, wire.MainNet, blockcompress.FormatV1)
	if err != nil {
		t.Fatalf("newColdStore: %v", err)
	}
	cleanup := func() {
		cold.Close()
		// hot has no Close; its files are under dir which t.TempDir removes.
		_ = os.RemoveAll(dir)
	}
	return &coldTestStore{hot: hot, cold: cold, dir: dir}, cleanup
}

// loadColdFixtures returns raw serialized mainnet blocks from wire/testdata.
func loadColdFixtures(t *testing.T) [][]byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "wire", "testdata", "block-*.blk"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Skip("no block-*.blk fixtures")
	}
	var out [][]byte
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		out = append(out, b)
	}
	return out
}

// strippedSerialization returns the SerializeNoWitness bytes of a raw block.
func strippedSerialization(t *testing.T, raw []byte) []byte {
	t.Helper()
	var blk wire.MsgBlock
	if err := blk.Deserialize(bytes.NewReader(raw)); err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	var buf bytes.Buffer
	if err := blk.SerializeNoWitness(&buf); err != nil {
		t.Fatalf("serialize nowitness: %v", err)
	}
	return buf.Bytes()
}

// TestColdBlockRoundTrip verifies the core A-2 property: a block written to the
// cold tier reads back as its stripped (non-witness) serialization, byte
// identical, with CRC and zstd frame checks passing.
func TestColdBlockRoundTrip(t *testing.T) {
	ct, cleanup := newColdTestStore(t)
	defer cleanup()

	for i, raw := range loadColdFixtures(t) {
		loc, err := ct.cold.writeColdBlock(raw)
		if err != nil {
			t.Fatalf("fixture %d: writeColdBlock: %v", i, err)
		}
		if loc.blockFileNum&coldFlag == 0 {
			t.Fatalf("fixture %d: cold location missing coldFlag", i)
		}
		// readBlock routes on the cold flag and returns the stripped bytes.
		got, err := ct.hot.readBlock(nil, loc)
		if err != nil {
			t.Fatalf("fixture %d: readBlock: %v", i, err)
		}
		want := strippedSerialization(t, raw)
		if !bytes.Equal(got, want) {
			t.Fatalf("fixture %d: cold round-trip mismatch: got len %d, "+
				"want len %d", i, len(got), len(want))
		}
		// The read-back block must deserialize as a valid stripped block.
		var blk wire.MsgBlock
		if err := blk.DeserializeNoWitness(bytes.NewReader(got)); err != nil {
			t.Fatalf("fixture %d: DeserializeNoWitness: %v", i, err)
		}
	}
}

// TestColdBlockRegion verifies readBlockRegion on a cold block: the 80-byte
// block header (offset 0, len 80) reads back correctly from the decompressed
// stripped block. The header is identical in stripped and full serializations.
func TestColdBlockRegion(t *testing.T) {
	ct, cleanup := newColdTestStore(t)
	defer cleanup()

	for i, raw := range loadColdFixtures(t) {
		loc, err := ct.cold.writeColdBlock(raw)
		if err != nil {
			t.Fatalf("fixture %d: writeColdBlock: %v", i, err)
		}
		hdr, err := ct.hot.readBlockRegion(loc, 0, 80)
		if err != nil {
			t.Fatalf("fixture %d: readBlockRegion header: %v", i, err)
		}
		// The first 80 bytes of the raw block are its header.
		if !bytes.Equal(hdr, raw[:80]) {
			t.Fatalf("fixture %d: cold header region mismatch", i)
		}
	}
}

// TestHotPathUnchanged verifies the hot tier still round-trips a full block with
// witness, unchanged from the original btcd behavior. This is the guard that the
// acceptance write path is genuinely untouched.
func TestHotPathUnchanged(t *testing.T) {
	ct, cleanup := newColdTestStore(t)
	defer cleanup()

	for i, raw := range loadColdFixtures(t) {
		loc, err := ct.hot.writeBlock(raw)
		if err != nil {
			t.Fatalf("fixture %d: writeBlock: %v", i, err)
		}
		if loc.blockFileNum&coldFlag != 0 {
			t.Fatalf("fixture %d: hot location has coldFlag set", i)
		}
		got, err := ct.hot.readBlock(nil, loc)
		if err != nil {
			t.Fatalf("fixture %d: readBlock: %v", i, err)
		}
		if !bytes.Equal(got, raw) {
			t.Fatalf("fixture %d: hot round-trip mismatch: got len %d, "+
				"want len %d", i, len(got), len(raw))
		}
	}
}

// TestMixedHotAndCold verifies a single blockStore can serve both hot (full) and
// cold (stripped) blocks simultaneously — the mixed-format datadir case.
func TestMixedHotAndCold(t *testing.T) {
	ct, cleanup := newColdTestStore(t)
	defer cleanup()

	fixtures := loadColdFixtures(t)
	if len(fixtures) < 2 {
		t.Skip("need >= 2 fixtures")
	}
	// Write fixture 0 hot, fixture 1 cold.
	hotLoc, err := ct.hot.writeBlock(fixtures[0])
	if err != nil {
		t.Fatalf("writeBlock hot: %v", err)
	}
	coldLoc, err := ct.cold.writeColdBlock(fixtures[1])
	if err != nil {
		t.Fatalf("writeColdBlock: %v", err)
	}

	// Read both back through the same blockStore and verify each decodes via
	// the right path.
	hotGot, err := ct.hot.readBlock(nil, hotLoc)
	if err != nil {
		t.Fatalf("readBlock hot: %v", err)
	}
	if !bytes.Equal(hotGot, fixtures[0]) {
		t.Fatalf("hot block mismatch in mixed store")
	}
	coldGot, err := ct.hot.readBlock(nil, coldLoc)
	if err != nil {
		t.Fatalf("readBlock cold: %v", err)
	}
	wantStripped := strippedSerialization(t, fixtures[1])
	if !bytes.Equal(coldGot, wantStripped) {
		t.Fatalf("cold block mismatch in mixed store: got len %d, want %d",
			len(coldGot), len(wantStripped))
	}
}

// TestColdCorruptionDetection verifies that a corrupted cold record is rejected
// rather than silently yielding wrong bytes. This backs the determinism/
// integrity claim for the cold tier.
func TestColdCorruptionDetection(t *testing.T) {
	ct, cleanup := newColdTestStore(t)
	defer cleanup()

	raw := loadColdFixtures(t)[0]
	loc, err := ct.cold.writeColdBlock(raw)
	if err != nil {
		t.Fatalf("writeColdBlock: %v", err)
	}

	// Corrupt a byte in the middle of the on-disk record (inside the
	// compressed payload). Read it directly off the file and flip a byte.
	path := coldBlockFilePath(ct.dir, loc.blockFileNum&^coldFlag)
	f, err := os.OpenFile(path, os.O_RDWR, 0o666)
	if err != nil {
		t.Fatalf("open cold file: %v", err)
	}
	mid := int64(loc.fileOffset) + int64(loc.blockLen)/2
	var one [1]byte
	if _, err := f.ReadAt(one[:], mid); err != nil {
		t.Fatalf("read mid: %v", err)
	}
	one[0] ^= 0xff
	if _, err := f.WriteAt(one[:], mid); err != nil {
		t.Fatalf("write mid: %v", err)
	}
	_ = f.Close()

	// The read must fail (zstd frame check or CRC). Either way, no wrong bytes.
	if _, err := ct.hot.readBlock(nil, loc); err == nil {
		t.Fatalf("corrupted cold block read succeeded; expected error")
	}
}

// TestColdSavingsOnDisk verifies the headline: the cold on-disk record is
// materially smaller than the hot on-disk record for the same block, and
// reports the per-fixture and blended reduction so the number is visible.
func TestColdSavingsOnDisk(t *testing.T) {
	ct, cleanup := newColdTestStore(t)
	defer cleanup()

	var totHot, totCold int
	for i, raw := range loadColdFixtures(t) {
		hotLoc, err := ct.hot.writeBlock(raw)
		if err != nil {
			t.Fatalf("fixture %d: writeBlock: %v", i, err)
		}
		coldLoc, err := ct.cold.writeColdBlock(raw)
		if err != nil {
			t.Fatalf("fixture %d: writeColdBlock: %v", i, err)
		}
		hotOnDisk := int64(hotLoc.blockLen)
		coldOnDisk := int64(coldLoc.blockLen)
		reduction := 100.0 * (1 - float64(coldOnDisk)/float64(hotOnDisk))
		t.Logf("fixture %d: hot=%d cold=%d (%.1f%% reduction)",
			i, hotOnDisk, coldOnDisk, reduction)
		totHot += int(hotOnDisk)
		totCold += int(coldOnDisk)
	}
	blended := 100.0 * (1 - float64(totCold)/float64(totHot))
	t.Logf("BLENDED: hot=%d cold=%d (%.1f%% reduction)", totHot, totCold, blended)
	// The cold tier must be materially smaller than the hot tier. These are
	// the real on-disk record sizes (including the 12-byte overhead each),
	// so the reduction is slightly below the pure-content figure but must
	// still clear a meaningful bar.
	if blended < 30.0 {
		t.Errorf("blended cold reduction %.1f%% below 30%% threshold", blended)
	}
}

// TestCompactBlockToCold verifies the end-to-end age-out compaction primitive via
// the full database stack: store a block hot (StoreBlock), compact it to cold
// (CompactBlockToCold via the ColdCompactor interface), commit, and confirm
// FetchBlock returns the stripped serialization and the block index location
// now points to the cold tier.
func TestCompactBlockToCold(t *testing.T) {
	dbPath := t.TempDir()
	pdb, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer pdb.Close()

	fixtures := loadColdFixtures(t)
	raw := fixtures[0]
	block, err := btcutil.NewBlockFromBytes(raw)
	if err != nil {
		t.Fatalf("NewBlockFromBytes: %v", err)
	}
	hash := block.Hash()
	wantStripped := strippedSerialization(t, raw)

	// Store the block hot.
	if err := pdb.Update(func(tx database.Tx) error {
		return tx.StoreBlock(block)
	}); err != nil {
		t.Fatalf("StoreBlock: %v", err)
	}

	// Confirm it reads back as a full block (hot tier) before compaction.
	if err := pdb.View(func(tx database.Tx) error {
		got, err := tx.FetchBlock(hash)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, raw) {
			t.Errorf("pre-compact: FetchBlock != stored full block")
		}
		return nil
	}); err != nil {
		t.Fatalf("pre-compact FetchBlock: %v", err)
	}

	// Compact it to cold via the ColdCompactor optional interface.
	if err := pdb.Update(func(tx database.Tx) error {
		cc, ok := tx.(database.ColdCompactor)
		if !ok {
			t.Fatal("tx does not implement ColdCompactor")
		}
		return cc.CompactBlockToCold(hash)
	}); err != nil {
		t.Fatalf("CompactBlockToCold: %v", err)
	}

	// After compaction, FetchBlock returns the stripped (non-witness) block.
	if err := pdb.View(func(tx database.Tx) error {
		got, err := tx.FetchBlock(hash)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, wantStripped) {
			t.Errorf("post-compact: FetchBlock != stripped serialization "+
				"(got len %d, want len %d)", len(got), len(wantStripped))
		}
		// The block index location must now carry the cold flag.
		row := tx.(*transaction).blockIdxBucket.Get(hash[:])
		loc := deserializeBlockLoc(row)
		if loc.blockFileNum&coldFlag == 0 {
			t.Errorf("post-compact: block index location is not cold")
		}
		return nil
	}); err != nil {
		t.Fatalf("post-compact FetchBlock: %v", err)
	}
}

// TestCompactBlockToColdRollback verifies that rolling back a compaction
// transaction leaves the block in the hot tier (no orphaned cold data, block
// index unchanged).
func TestCompactBlockToColdRollback(t *testing.T) {
	dbPath := t.TempDir()
	pdb, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer pdb.Close()

	raw := loadColdFixtures(t)[0]
	block, err := btcutil.NewBlockFromBytes(raw)
	if err != nil {
		t.Fatalf("NewBlockFromBytes: %v", err)
	}
	hash := block.Hash()
	if err := pdb.Update(func(tx database.Tx) error {
		return tx.StoreBlock(block)
	}); err != nil {
		t.Fatalf("StoreBlock: %v", err)
	}

	// Schedule a compaction then roll back.
	if err := pdb.Update(func(tx database.Tx) error {
		cc := tx.(database.ColdCompactor)
		if err := cc.CompactBlockToCold(hash); err != nil {
			return err
		}
		return errors.New("test: force rollback") // force rollback
	}); err == nil {
		t.Fatal("expected rollback error")
	}

	// The block must still be hot and return the full block.
	if err := pdb.View(func(tx database.Tx) error {
		got, err := tx.FetchBlock(hash)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, raw) {
			t.Errorf("after rollback: FetchBlock != full block "+
				"(got len %d, want len %d)", len(got), len(raw))
		}
		row := tx.(*transaction).blockIdxBucket.Get(hash[:])
		loc := deserializeBlockLoc(row)
		if loc.blockFileNum&coldFlag != 0 {
			t.Errorf("after rollback: block index location is cold")
		}
		return nil
	}); err != nil {
		t.Fatalf("post-rollback FetchBlock: %v", err)
	}

	// No cold files should exist.
	matches, _ := filepath.Glob(filepath.Join(dbPath, coldFileSubdir, "*"))
	if len(matches) != 0 {
		t.Errorf("after rollback: %d cold files exist", len(matches))
	}
}

// TestCompactAlreadyCold verifies compacting a block that is already cold is
// an idempotent no-op (returns nil). This is required so the blockchain layer
// can call CompactBlockToCold unconditionally and then rewrite offset index
// entries — which is important after a reorg reconnects a cold block, because
// ConnectBlock rebuilds those indexes with witness-relative offsets that must
// be rewritten even though the block is already cold.
func TestCompactAlreadyCold(t *testing.T) {
	dbPath := t.TempDir()
	pdb, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer pdb.Close()

	raw := loadColdFixtures(t)[0]
	block, _ := btcutil.NewBlockFromBytes(raw)
	hash := block.Hash()
	if err := pdb.Update(func(tx database.Tx) error {
		return tx.StoreBlock(block)
	}); err != nil {
		t.Fatalf("StoreBlock: %v", err)
	}
	// Compact once.
	if err := pdb.Update(func(tx database.Tx) error {
		return tx.(database.ColdCompactor).CompactBlockToCold(hash)
	}); err != nil {
		t.Fatalf("first compact: %v", err)
	}
	// Compact again -> idempotent no-op (nil, not an error).
	if err := pdb.Update(func(tx database.Tx) error {
		return tx.(database.ColdCompactor).CompactBlockToCold(hash)
	}); err != nil {
		t.Fatalf("second compact: %v", err)
	}
}

// TestReclaimHotSpace verifies that after compacting blocks to cold, the
// hot-tier files are deleted by ReclaimHotSpace and the cold blocks remain
// readable.
func TestReclaimHotSpace(t *testing.T) {
	dbPath := t.TempDir()
	pdb, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer pdb.Close()

	fixtures := loadColdFixtures(t)
	raw := fixtures[0]
	block, err := btcutil.NewBlockFromBytes(raw)
	if err != nil {
		t.Fatalf("NewBlockFromBytes: %v", err)
	}
	hash := block.Hash()
	wantStripped := strippedSerialization(t, raw)

	// Store the block hot.
	if err := pdb.Update(func(tx database.Tx) error {
		return tx.StoreBlock(block)
	}); err != nil {
		t.Fatalf("StoreBlock: %v", err)
	}

	// Compact it to cold.
	if err := pdb.Update(func(tx database.Tx) error {
		return tx.(database.ColdCompactor).CompactBlockToCold(hash)
	}); err != nil {
		t.Fatalf("CompactBlockToCold: %v", err)
	}

	// Reclaim hot space. The hot file (file 0) should be deleted since all
	// its blocks are now cold.
	var reclaimed uint64
	if err := pdb.Update(func(tx database.Tx) error {
		var err error
		reclaimed, err = tx.(database.ColdCompactor).ReclaimHotSpace()
		return err
	}); err != nil {
		t.Fatalf("ReclaimHotSpace: %v", err)
	}
	t.Logf("reclaimed %d bytes", reclaimed)
	if reclaimed == 0 {
		t.Errorf("expected nonzero reclaim")
	}

	// The block must still be readable from the cold tier.
	if err := pdb.View(func(tx database.Tx) error {
		got, err := tx.FetchBlock(hash)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, wantStripped) {
			t.Errorf("post-reclaim: FetchBlock != stripped (got len %d, want %d)",
				len(got), len(wantStripped))
		}
		return nil
	}); err != nil {
		t.Fatalf("post-reclaim FetchBlock: %v", err)
	}
}

// TestReclaimHotSpacePartial verifies that ReclaimHotSpace does NOT delete a
// hot file that still contains hot (non-compacted) blocks.
func TestReclaimHotSpacePartial(t *testing.T) {
	dbPath := t.TempDir()
	pdb, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer pdb.Close()

	fixtures := loadColdFixtures(t)
	if len(fixtures) < 2 {
		t.Skip("need >= 2 fixtures")
	}

	// Store two blocks hot.
	block0, err := btcutil.NewBlockFromBytes(fixtures[0])
	if err != nil {
		t.Fatalf("NewBlockFromBytes: %v", err)
	}
	block1, err := btcutil.NewBlockFromBytes(fixtures[1])
	if err != nil {
		t.Fatalf("NewBlockFromBytes: %v", err)
	}
	hash0 := block0.Hash()
	hash1 := block1.Hash()

	if err := pdb.Update(func(tx database.Tx) error {
		if err := tx.StoreBlock(block0); err != nil {
			return err
		}
		return tx.StoreBlock(block1)
	}); err != nil {
		t.Fatalf("StoreBlock: %v", err)
	}

	// Compact only block0 to cold. block1 stays hot.
	if err := pdb.Update(func(tx database.Tx) error {
		return tx.(database.ColdCompactor).CompactBlockToCold(hash0)
	}); err != nil {
		t.Fatalf("CompactBlockToCold: %v", err)
	}

	// Reclaim hot space. Both blocks are in file 0, so nothing should be
	// reclaimed (file 0 still has a hot block).
	var reclaimed uint64
	if err := pdb.Update(func(tx database.Tx) error {
		var err error
		reclaimed, err = tx.(database.ColdCompactor).ReclaimHotSpace()
		return err
	}); err != nil {
		t.Fatalf("ReclaimHotSpace: %v", err)
	}
	if reclaimed != 0 {
		t.Errorf("expected 0 reclaim (file 0 still has hot block), got %d",
			reclaimed)
	}

	// Both blocks must still be readable.
	if err := pdb.View(func(tx database.Tx) error {
		got0, err := tx.FetchBlock(hash0)
		if err != nil {
			return err
		}
		if !bytes.Equal(got0, strippedSerialization(t, fixtures[0])) {
			t.Errorf("block0 (cold) mismatch")
		}
		got1, err := tx.FetchBlock(hash1)
		if err != nil {
			return err
		}
		if !bytes.Equal(got1, fixtures[1]) {
			t.Errorf("block1 (hot) mismatch")
		}
		return nil
	}); err != nil {
		t.Fatalf("post-reclaim FetchBlock: %v", err)
	}
}

// TestColdCacheHit verifies that the decompressed-block cache serves repeated
// cold reads from memory: after an initial read populates the cache, a second
// read of the same block returns identical bytes without re-decompressing.
func TestColdCacheHit(t *testing.T) {
	ct, cleanup := newColdTestStore(t)
	defer cleanup()

	raw := loadColdFixtures(t)[0]
	loc, err := ct.cold.writeColdBlock(raw)
	if err != nil {
		t.Fatalf("writeColdBlock: %v", err)
	}
	want := strippedSerialization(t, raw)

	// First read: cache miss, decompresses, populates cache.
	got1, err := ct.hot.readBlock(nil, loc)
	if err != nil {
		t.Fatalf("first readBlock: %v", err)
	}
	if !bytes.Equal(got1, want) {
		t.Fatalf("first read: mismatch")
	}
	if ct.hot.coldCache.len() != 1 {
		t.Fatalf("cache should have 1 entry after first read, got %d",
			ct.hot.coldCache.len())
	}

	// Second read: cache hit, same bytes, no decompression.
	got2, err := ct.hot.readBlock(nil, loc)
	if err != nil {
		t.Fatalf("second readBlock: %v", err)
	}
	if !bytes.Equal(got2, want) {
		t.Fatalf("second read: mismatch")
	}
	// Cache still holds one entry (hit doesn't add a new one).
	if ct.hot.coldCache.len() != 1 {
		t.Fatalf("cache should still have 1 entry, got %d",
			ct.hot.coldCache.len())
	}
}

// TestColdCacheRegion verifies that readBlockRegion on a cold block populates
// the cache and a subsequent readBlock of the same block is a cache hit.
func TestColdCacheRegion(t *testing.T) {
	ct, cleanup := newColdTestStore(t)
	defer cleanup()

	raw := loadColdFixtures(t)[0]
	loc, err := ct.cold.writeColdBlock(raw)
	if err != nil {
		t.Fatalf("writeColdBlock: %v", err)
	}

	// Read the header region (offset 0, 80 bytes) — populates cache.
	hdr, err := ct.hot.readBlockRegion(loc, 0, 80)
	if err != nil {
		t.Fatalf("readBlockRegion: %v", err)
	}
	if !bytes.Equal(hdr, raw[:80]) {
		t.Fatalf("header region mismatch")
	}
	if ct.hot.coldCache.len() != 1 {
		t.Fatalf("cache should have 1 entry after region read, got %d",
			ct.hot.coldCache.len())
	}

	// Now read the full block — should be a cache hit (no file open needed).
	got, err := ct.hot.readBlock(nil, loc)
	if err != nil {
		t.Fatalf("readBlock after region: %v", err)
	}
	want := strippedSerialization(t, raw)
	if !bytes.Equal(got, want) {
		t.Fatalf("full block after region read: mismatch")
	}
}

// TestColdCacheEviction verifies the cache evicts the least recently used entry
// when it exceeds maxColdCacheBlocks.
func TestColdCacheEviction(t *testing.T) {
	ct, cleanup := newColdTestStore(t)
	defer cleanup()

	fixtures := loadColdFixtures(t)
	if len(fixtures) < 2 {
		t.Skip("need >= 2 fixtures")
	}

	// Write more cold blocks than the cache can hold. Cycle the two fixtures
	// to produce distinct locations.
	var locs []blockLocation
	for i := 0; i < maxColdCacheBlocks+2; i++ {
		raw := fixtures[i%len(fixtures)]
		loc, err := ct.cold.writeColdBlock(raw)
		if err != nil {
			t.Fatalf("writeColdBlock %d: %v", i, err)
		}
		locs = append(locs, loc)
	}

	// Read each block once to populate the cache. After this, the cache
	// should be at capacity and the first two entries (locs[0], locs[1])
	// should have been evicted.
	for _, loc := range locs {
		if _, err := ct.hot.readBlock(nil, loc); err != nil {
			t.Fatalf("readBlock: %v", err)
		}
	}

	if ct.hot.coldCache.len() != maxColdCacheBlocks {
		t.Fatalf("cache should have %d entries, got %d",
			maxColdCacheBlocks, ct.hot.coldCache.len())
	}

	// The first two locations should no longer be in the cache (evicted as
	// LRU). Verify by checking the cache directly.
	if ct.hot.coldCache.get(locs[0]) != nil {
		t.Errorf("locs[0] should have been evicted")
	}
	if ct.hot.coldCache.get(locs[1]) != nil {
		t.Errorf("locs[1] should have been evicted")
	}

	// The last two locations should still be cached.
	if ct.hot.coldCache.get(locs[len(locs)-1]) == nil {
		t.Errorf("last loc should still be cached")
	}
	if ct.hot.coldCache.get(locs[len(locs)-2]) == nil {
		t.Errorf("second-to-last loc should still be cached")
	}
}
