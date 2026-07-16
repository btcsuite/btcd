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

// TestCompactAlreadyCold verifies compacting a block that is already cold fails.
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
	// Compact again -> error.
	err = pdb.Update(func(tx database.Tx) error {
		return tx.(database.ColdCompactor).CompactBlockToCold(hash)
	})
	if err == nil {
		t.Fatal("second compact succeeded; expected error (already cold)")
	}
}
