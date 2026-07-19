// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/database"
)

// TestColdDirectStoreWhenHeadersAhead verifies that when headers tip already
// places a block past the witness buffer, ProcessBlock stores the body
// cold-direct (IsColdBlock true immediately) instead of writing hot first.
func TestColdDirectStoreWhenHeadersAhead(t *testing.T) {
	chain, tearDown, err := chainSetup("TestColdDirectStoreWhenHeadersAhead",
		&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("chainSetup: %v", err)
	}
	defer tearDown()

	const buffer int32 = 5
	chain.witnessBuffer = buffer

	blocks, err := loadBlocks("blk_0_to_14131.dat")
	if err != nil {
		t.Fatalf("loadBlocks: %v", err)
	}
	if len(blocks) < 20 {
		t.Fatalf("need at least 20 blocks, got %d", len(blocks))
	}

	// Headers-first: tip at height 19 before any body past genesis.
	for i := 1; i < 20; i++ {
		header := &blocks[i].MsgBlock().Header
		_, err := chain.ProcessBlockHeader(header, BFNone, false)
		if err != nil {
			t.Fatalf("ProcessBlockHeader %d: %v", i, err)
		}
	}
	_, headerHeight := chain.BestHeader()
	if headerHeight != 19 {
		t.Fatalf("BestHeader height = %d, want 19", headerHeight)
	}

	// Bodies: heights 1..14 are past buffer relative to headers tip 19.
	for i := 1; i < 20; i++ {
		_, _, err := chain.ProcessBlock(blocks[i], BFNone)
		if err != nil {
			t.Fatalf("ProcessBlock %d: %v", i, err)
		}
	}

	for height := int32(1); height <= 19; height++ {
		hash := blocks[height].Hash()
		var cold bool
		err := chain.db.View(func(dbTx database.Tx) error {
			cc, ok := dbTx.(database.ColdCompactor)
			if !ok {
				t.Fatal("db does not implement ColdCompactor")
			}
			var err error
			cold, err = cc.IsColdBlock(hash)
			return err
		})
		if err != nil {
			t.Fatalf("IsColdBlock height %d: %v", height, err)
		}
		wantCold := height <= headerHeight-buffer
		if cold != wantCold {
			t.Errorf("height %d: IsColdBlock = %v, want %v "+
				"(headerHeight=%d buffer=%d)",
				height, cold, wantCold, headerHeight, buffer)
		}
	}
}
