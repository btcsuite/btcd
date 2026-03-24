// Copyright (c) 2013-2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/btcsuite/btcd/blockchain/internal/testhelper"
	"github.com/btcsuite/btcd/btcutil"
)

// TestMaybeAcceptBlockReusesHeaderNode ensures that when a block header is
// processed first via ProcessBlockHeader and later the full block arrives via
// ProcessBlock, the existing blockNode pointer is reused rather than replaced.
// Replacing the pointer would orphan the entry held by bestHeader's chainView,
// causing bestHeader.Contains(index.LookupNode(hash)) to return false and
// breaking IsValidHeader and downstream netsync checks.
func TestMaybeAcceptBlockReusesHeaderNode(t *testing.T) {
	chain, params, tearDown := utxoCacheTestChain(
		"TestMaybeAcceptBlockReusesHeaderNode")
	defer tearDown()

	// Build a base chain of 3 blocks.
	//
	// genesis -> 1 -> 2 -> 3
	tip := btcutil.NewBlock(params.GenesisBlock)
	_, _, err := addBlocks(3, chain, tip, []*testhelper.SpendableOut{})
	if err != nil {
		t.Fatalf("failed to build base chain: %v", err)
	}

	// Create block 4 without processing it.
	prevBlock, err := chain.BlockByHeight(3)
	if err != nil {
		t.Fatalf("failed to get block at height 3: %v", err)
	}
	block4, _, err := newBlock(chain, prevBlock, nil)
	if err != nil {
		t.Fatalf("failed to create block 4: %v", err)
	}

	// Process block 4's header first.
	block4Hash := block4.Hash()
	_, err = chain.ProcessBlockHeader(
		&block4.MsgBlock().Header, BFNone, false)
	if err != nil {
		t.Fatalf("ProcessBlockHeader fail: %v", err)
	}

	// Capture the header-only node pointer from the index.
	headerNode := chain.index.LookupNode(block4Hash)
	if headerNode == nil {
		t.Fatal("header node not found in block index")
	}

	// Now process the full block.
	_, _, err = chain.ProcessBlock(block4, BFNone)
	if err != nil {
		t.Fatalf("ProcessBlock fail: %v", err)
	}

	// The index must still hold the same pointer that bestHeader has.
	// Before the fix, maybeAcceptBlock would create a fresh node and
	// overwrite the index entry, orphaning the pointer in bestHeader.
	fullBlockNode := chain.index.LookupNode(block4Hash)
	if fullBlockNode != headerNode {
		t.Fatal("ProcessBlock replaced the header node pointer " +
			"instead of reusing it")
	}
	if !chain.bestHeader.Contains(fullBlockNode) {
		t.Fatal("node no longer in bestHeader after ProcessBlock")
	}
}
