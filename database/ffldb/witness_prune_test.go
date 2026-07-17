// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is part of the ffldb package (whitebox testing) so it can access
// internal types like coldFlag, deserializeBlockLoc, and blockStore.

package ffldb

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire/v2"
)

// TestWitnessPruningEndToEnd is the definitive end-to-end test for the
// witness-separated storage design. It exercises the full lifecycle with real
// post-SegWit mainnet blocks that contain witness data:
//
//  1. Store a block hot (full, with witness) via StoreBlock
//  2. Verify FetchBlock returns the FULL block (with witness) before compaction
//  3. Verify the block has witness data (stripped is shorter than full)
//  4. Compact the block to cold via CompactBlockToCold
//  5. Verify FetchBlock returns the STRIPPED block (without witness) after compaction
//  6. Verify the stripped block deserializes via both Deserialize and DeserializeNoWitness
//  7. Verify HasWitness() returns false on the deserialized cold block (served as legacy)
//  8. Verify the cold file on disk is smaller than the hot file
//  9. Reclaim hot space
//
// 10. Verify the block is still readable from cold after reclaim
// 11. Verify the block hash is unchanged (stripped block has same hash as full block)
func TestWitnessPruningEndToEnd(t *testing.T) {
	fixtures := loadColdFixtures(t)
	if len(fixtures) == 0 {
		t.Skip("no post-SegWit fixtures")
	}

	for fi, raw := range fixtures {
		t.Run("fixture_"+itoa(fi), func(t *testing.T) {
			testWitnessPruningLifecycle(t, raw)
		})
	}
}

func testWitnessPruningLifecycle(t *testing.T, raw []byte) {
	dbPath := t.TempDir()
	pdb, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer pdb.Close()

	block, err := btcutil.NewBlockFromBytes(raw)
	if err != nil {
		t.Fatalf("NewBlockFromBytes: %v", err)
	}
	hash := block.Hash()

	// Compute the stripped serialization.
	stripped := strippedSerialization(t, raw)

	// --- Step 2: Verify the fixture actually has witness data ---
	if len(stripped) >= len(raw) {
		t.Fatalf("fixture has no witness data: stripped (%d) >= full (%d)",
			len(stripped), len(raw))
	}
	witnessBytes := len(raw) - len(stripped)
	witnessPct := 100.0 * float64(witnessBytes) / float64(len(raw))
	t.Logf("full=%d stripped=%d witness=%d (%.1f%%)",
		len(raw), len(stripped), witnessBytes, witnessPct)

	// --- Step 1: Store the block hot ---
	if err := pdb.Update(func(tx database.Tx) error {
		return tx.StoreBlock(block)
	}); err != nil {
		t.Fatalf("StoreBlock: %v", err)
	}

	// --- Step 2: FetchBlock returns FULL block before compaction ---
	if err := pdb.View(func(tx database.Tx) error {
		got, err := tx.FetchBlock(hash)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, raw) {
			t.Errorf("pre-compact: FetchBlock != full block (got %d, want %d)",
				len(got), len(raw))
		}
		return nil
	}); err != nil {
		t.Fatalf("pre-compact FetchBlock: %v", err)
	}

	// --- Step 4: Compact to cold ---
	if err := pdb.Update(func(tx database.Tx) error {
		cc, ok := tx.(database.ColdCompactor)
		if !ok {
			t.Fatal("tx does not implement ColdCompactor")
		}
		return cc.CompactBlockToCold(hash)
	}); err != nil {
		t.Fatalf("CompactBlockToCold: %v", err)
	}

	// --- Step 5: FetchBlock returns STRIPPED block after compaction ---
	if err := pdb.View(func(tx database.Tx) error {
		got, err := tx.FetchBlock(hash)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, stripped) {
			t.Errorf("post-compact: FetchBlock != stripped (got %d, want %d)",
				len(got), len(stripped))
		}
		if len(got) >= len(raw) {
			t.Errorf("post-compact: stripped (%d) not smaller than full (%d)",
				len(got), len(raw))
		}
		return nil
	}); err != nil {
		t.Fatalf("post-compact FetchBlock: %v", err)
	}

	// --- Step 6 & 7: Deserialized cold block has no witness ---
	if err := pdb.View(func(tx database.Tx) error {
		got, err := tx.FetchBlock(hash)
		if err != nil {
			return err
		}

		// Deserialize with full Deserialize (as pushBlockMsg does).
		var blk wire.MsgBlock
		if err := blk.Deserialize(bytes.NewReader(got)); err != nil {
			t.Errorf("Deserialize (full): %v", err)
			return nil
		}
		// A cold (stripped) block has no witness data on any transaction.
		hasWitness := false
		for _, tx := range blk.Transactions {
			if tx.HasWitness() {
				hasWitness = true
				break
			}
		}
		if hasWitness {
			t.Error("cold block deserialized with Deserialize has witness — " +
				"should be false (served as legacy to peers)")
		}

		// Deserialize with DeserializeNoWitness (as indexers might).
		var blk2 wire.MsgBlock
		if err := blk2.DeserializeNoWitness(bytes.NewReader(got)); err != nil {
			t.Errorf("DeserializeNoWitness: %v", err)
		}

		// Block hash must be unchanged.
		gotHash := blk.BlockHash()
		if !gotHash.IsEqual(hash) {
			t.Errorf("cold block hash mismatch: got %v, want %v",
				gotHash, hash)
		}
		return nil
	}); err != nil {
		t.Fatalf("post-compact deserialization: %v", err)
	}

	// --- Step 8: Cold file is smaller than hot file ---
	hotSize := dirFileSize(t, dbPath, ".fdb")
	coldSize := dirFileSize(t, filepath.Join(dbPath, "cold"), "")
	t.Logf("hot files: %d bytes, cold files: %d bytes (%.1f%% reduction)",
		hotSize, coldSize,
		100.0*(1-float64(coldSize)/float64(max(hotSize, 1))))
	if coldSize >= hotSize {
		t.Errorf("cold files (%d) not smaller than hot files (%d)",
			coldSize, hotSize)
	}

	// --- Step 9: Reclaim hot space ---
	var reclaimed uint64
	if err := pdb.Update(func(tx database.Tx) error {
		var err error
		reclaimed, err = tx.(database.ColdCompactor).ReclaimHotSpace()
		return err
	}); err != nil {
		t.Fatalf("ReclaimHotSpace: %v", err)
	}
	if reclaimed == 0 {
		t.Error("ReclaimHotSpace returned 0 bytes reclaimed")
	}
	t.Logf("reclaimed %d bytes from hot tier", reclaimed)

	// --- Step 10: Block still readable from cold after reclaim ---
	if err := pdb.View(func(tx database.Tx) error {
		got, err := tx.FetchBlock(hash)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, stripped) {
			t.Errorf("post-reclaim: FetchBlock != stripped (got %d, want %d)",
				len(got), len(stripped))
		}
		return nil
	}); err != nil {
		t.Fatalf("post-reclaim FetchBlock: %v", err)
	}

	// Hot file should be gone after reclaim.
	hotSizeAfter := dirFileSize(t, dbPath, ".fdb")
	if hotSizeAfter > 0 {
		t.Errorf("hot files still exist after reclaim (%d bytes)", hotSizeAfter)
	}
}

// TestWitnessPruningMultipleBlocks exercises the lifecycle with multiple blocks
// compacted and reclaimed together, verifying that the hot window is preserved
// while older blocks are cold.
func TestWitnessPruningMultipleBlocks(t *testing.T) {
	fixtures := loadColdFixtures(t)
	if len(fixtures) < 2 {
		t.Skip("need >= 2 fixtures")
	}

	dbPath := t.TempDir()
	pdb, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer pdb.Close()

	// Store all fixtures hot.
	type blockInfo struct {
		hash     *chainhash.Hash
		raw      []byte
		stripped []byte
	}
	var blocks []blockInfo
	for _, raw := range fixtures {
		block, err := btcutil.NewBlockFromBytes(raw)
		if err != nil {
			t.Fatalf("NewBlockFromBytes: %v", err)
		}
		if err := pdb.Update(func(tx database.Tx) error {
			return tx.StoreBlock(block)
		}); err != nil {
			t.Fatalf("StoreBlock: %v", err)
		}
		blocks = append(blocks, blockInfo{
			hash:     block.Hash(),
			raw:      raw,
			stripped: strippedSerialization(t, raw),
		})
	}

	// Compact all but the last block to cold (last stays hot).
	for i := 0; i < len(blocks)-1; i++ {
		if err := pdb.Update(func(tx database.Tx) error {
			return tx.(database.ColdCompactor).CompactBlockToCold(blocks[i].hash)
		}); err != nil {
			t.Fatalf("CompactBlockToCold %d: %v", i, err)
		}
	}

	// Verify: compacted blocks return stripped, last block returns full.
	for i, b := range blocks {
		if err := pdb.View(func(tx database.Tx) error {
			got, err := tx.FetchBlock(b.hash)
			if err != nil {
				return err
			}
			if i < len(blocks)-1 {
				// Cold: should be stripped.
				if !bytes.Equal(got, b.stripped) {
					t.Errorf("block %d (cold): got len %d, want stripped len %d",
						i, len(got), len(b.stripped))
				}
			} else {
				// Hot: should be full.
				if !bytes.Equal(got, b.raw) {
					t.Errorf("block %d (hot): got len %d, want full len %d",
						i, len(got), len(b.raw))
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("FetchBlock %d: %v", i, err)
		}
	}

	// Reclaim hot space. Hot file should still exist (last block is hot).
	if err := pdb.Update(func(tx database.Tx) error {
		_, err := tx.(database.ColdCompactor).ReclaimHotSpace()
		return err
	}); err != nil {
		t.Fatalf("ReclaimHotSpace: %v", err)
	}

	// All blocks should still be readable.
	for i, b := range blocks {
		if err := pdb.View(func(tx database.Tx) error {
			got, err := tx.FetchBlock(b.hash)
			if err != nil {
				return err
			}
			if i < len(blocks)-1 {
				if !bytes.Equal(got, b.stripped) {
					t.Errorf("post-reclaim block %d (cold): mismatch", i)
				}
			} else {
				if !bytes.Equal(got, b.raw) {
					t.Errorf("post-reclaim block %d (hot): mismatch", i)
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("post-reclaim FetchBlock %d: %v", i, err)
		}
	}
}

// --- helpers ---

// dirFileSize returns the total size of all regular files in a directory.
// If ext is non-empty, only files with that extension are counted.
func dirFileSize(t *testing.T, dir string, ext string) int64 {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var total int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if ext != "" && filepath.Ext(e.Name()) != ext {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total
}

// itoa is a simple int->string to avoid strconv import.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
