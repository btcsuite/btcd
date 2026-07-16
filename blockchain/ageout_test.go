// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"os"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

// TestWitnessBufferAgeOut verifies that the age-out driver compacts blocks
// older than the witness buffer to the cold tier: after processing enough
// blocks, FetchBlock on an aged-out block returns the stripped (non-witness)
// serialization, while a block still in the hot window returns the full block.
//
// The test blocks (blk_0_to_14131.dat) are pre-SegWit, so stripped == full for
// them. The test verifies the compaction mechanism (blocks remain readable and
// byte-correct after compaction, hot window blocks are unchanged) rather than
// the witness savings ratio, which is covered by the ffldb cold-tier tests on
// post-SegWit fixtures.
func TestWitnessBufferAgeOut(t *testing.T) {
	chain, tearDown, err := chainSetup("TestWitnessBufferAgeOut",
		&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("chainSetup: %v", err)
	}
	defer tearDown()

	// Enable age-out compaction with a 5-block hot window.
	chain.witnessBuffer = 5

	blocks, err := loadBlocks("blk_0_to_14131.dat")
	if err != nil {
		t.Fatalf("loadBlocks: %v", err)
	}
	if len(blocks) < 20 {
		t.Fatalf("need at least 20 blocks, got %d", len(blocks))
	}

	// Process blocks 1..19 (skipping genesis at index 0).
	for i := 1; i < 20; i++ {
		_, _, err := chain.ProcessBlock(blocks[i], BFNone)
		if err != nil {
			t.Fatalf("ProcessBlock %d: %v", i, err)
		}
	}

	// The tip is now at height 19. connectBlock compacts the block at
	// height (H - witnessBuffer) on each connection at height H. So:
	//   H=6  -> compact height 1
	//   H=7  -> compact height 2
	//   ...
	//   H=19 -> compact height 14
	// Blocks 1..14 should be cold, blocks 15..19 should be hot.
	//
	// For pre-SegWit blocks, stripped == full, so cold and hot blocks both
	// return the same bytes via FetchBlock. The test verifies that all blocks
	// remain readable and byte-correct after the compaction process runs.

	// All blocks 1..19 should be readable and correct.
	for height := 1; height <= 19; height++ {
		hash := blocks[height].Hash()
		var got []byte
		err := chain.db.View(func(dbTx database.Tx) error {
			var err error
			got, err = dbTx.FetchBlock(hash)
			return err
		})
		if err != nil {
			t.Fatalf("FetchBlock height %d: %v", height, err)
		}
		want, err := blocks[height].Bytes()
		if err != nil {
			t.Fatalf("block.Bytes height %d: %v", height, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("height %d: FetchBlock mismatch after compaction "+
				"(got len %d, want len %d)", height, len(got),
				len(want))
		}
	}

	// Verify that cold-tier files were actually created on disk. The ffldb
	// cold store writes to a "cold/" subdirectory under the database path.
	// If the age-out driver ran, this directory must exist and contain files.
	coldDir := "testdbs/TestWitnessBufferAgeOut/cold"
	entries, err := os.ReadDir(coldDir)
	if err != nil {
		t.Fatalf("cold directory not created (age-out driver did not "+
			"run): %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("cold directory is empty (no blocks were compacted)")
	}
	t.Logf("cold tier: %d file(s) created", len(entries))
}

// TestWitnessBufferDisabled verifies that with witnessBuffer=0 (disabled), no
// blocks are compacted to cold — all blocks remain hot and return full bytes.
func TestWitnessBufferDisabled(t *testing.T) {
	chain, tearDown, err := chainSetup("TestWitnessBufferDisabled",
		&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("chainSetup: %v", err)
	}
	defer tearDown()

	// witnessBuffer defaults to 0 (disabled).
	blocks, err := loadBlocks("blk_0_to_14131.dat")
	if err != nil {
		t.Fatalf("loadBlocks: %v", err)
	}
	if len(blocks) < 10 {
		t.Fatalf("need at least 10 blocks, got %d", len(blocks))
	}

	for i := 1; i < 10; i++ {
		_, _, err := chain.ProcessBlock(blocks[i], BFNone)
		if err != nil {
			t.Fatalf("ProcessBlock %d: %v", i, err)
		}
	}

	for height := 1; height < 10; height++ {
		hash := blocks[height].Hash()
		var got []byte
		err := chain.db.View(func(dbTx database.Tx) error {
			var err error
			got, err = dbTx.FetchBlock(hash)
			return err
		})
		if err != nil {
			t.Fatalf("FetchBlock height %d: %v", height, err)
		}
		want, err := blocks[height].Bytes()
		if err != nil {
			t.Fatalf("block.Bytes height %d: %v", height, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("height %d: block mismatch with compaction disabled "+
				"(got len %d, want len %d)", height, len(got),
				len(want))
		}
	}

	// With compaction disabled, no cold directory should exist.
	coldDir := "testdbs/TestWitnessBufferDisabled/cold"
	if _, err := os.Stat(coldDir); err == nil {
		t.Error("cold directory created despite witnessBuffer=0")
	}
}

// TestWitnessBufferConfig verifies that the WitnessBuffer config field is
// wired through to the BlockChain.
func TestWitnessBufferConfig(t *testing.T) {
	dbPath := "testdbs/TestWitnessBufferConfig"
	_ = os.RemoveAll(dbPath)
	defer os.RemoveAll(dbPath)
	defer os.RemoveAll("testdbs")

	db, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer db.Close()

	chain, err := New(&Config{
		DB:            db,
		ChainParams:   &chaincfg.MainNetParams,
		TimeSource:    NewMedianTime(),
		SigCache:      txscript.NewSigCache(1000),
		WitnessBuffer: 2016,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if chain.witnessBuffer != 2016 {
		t.Errorf("witnessBuffer = %d, want 2016", chain.witnessBuffer)
	}
}
