// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/btcsuite/btcd/blockchain/internal/testhelper"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/database"
)

// TestInvalidatePastWitnessBufferRefused verifies that invalidateblock is
// refused when the resulting tip would fall outside the witness buffer (a cold
// tip would lack retained witness and tip metrics / reconsider would be wrong).
func TestInvalidatePastWitnessBufferRefused(t *testing.T) {
	chain, params, tearDown := utxoCacheTestChain("TestInvalidatePastWitnessBuffer")
	defer tearDown()

	const buffer int32 = 5
	chain.witnessBuffer = buffer

	tip := btcutil.NewBlock(params.GenesisBlock)
	tip.SetHeight(0)
	if _, _, err := addBlocks(15, chain, tip, []*testhelper.SpendableOut{}); err != nil {
		t.Fatalf("addBlocks: %v", err)
	}

	// Tip is height 15. Invalidating height 5 would set tip to 4;
	// 15-4=11 >= buffer → must refuse with ErrWitnessExcised.
	deep, err := chain.BlockByHeight(5)
	if err != nil {
		t.Fatalf("BlockByHeight(5): %v", err)
	}
	err = chain.InvalidateBlock(deep.Hash())
	if err == nil {
		t.Fatal("expected InvalidateBlock past witness buffer to fail")
	}
	rerr, ok := err.(RuleError)
	if !ok {
		t.Fatalf("expected RuleError, got %T: %v", err, err)
	}
	if rerr.ErrorCode != ErrWitnessExcised {
		t.Fatalf("ErrorCode=%v, want ErrWitnessExcised", rerr.ErrorCode)
	}

	// Shallow invalidate (height 14 → tip 13): 15-13=2 < buffer → allowed.
	shallow, err := chain.BlockByHeight(14)
	if err != nil {
		t.Fatalf("BlockByHeight(14): %v", err)
	}
	if err := chain.InvalidateBlock(shallow.Hash()); err != nil {
		t.Fatalf("shallow InvalidateBlock: %v", err)
	}
	if got := chain.BestSnapshot().Height; got != 13 {
		t.Fatalf("tip height=%d, want 13", got)
	}
}

// TestStaleSideChainBodyDropped verifies that age-out deletes bodies of
// alternate forks once they fall past the witness buffer, while recent side
// tips keep their data and headers remain in the block index.
//
// "Side chain" means an alternate fork in the block index — not a separate
// consensus network.
func TestStaleSideChainBodyDropped(t *testing.T) {
	chain, params, tearDown := utxoCacheTestChain("TestStaleSideChainBodyDropped")
	defer tearDown()

	const buffer int32 = 5
	chain.witnessBuffer = buffer

	genesis := btcutil.NewBlock(params.GenesisBlock)
	genesis.SetHeight(0)

	// Main tip to height 3, then fork a short side branch off height 1.
	_, spendable, err := addBlocks(3, chain, genesis, []*testhelper.SpendableOut{})
	if err != nil {
		t.Fatalf("addBlocks early main: %v", err)
	}
	b1, err := chain.BlockByHeight(1)
	if err != nil {
		t.Fatalf("BlockByHeight(1): %v", err)
	}
	staleHashes, _, err := addBlocks(2, chain, b1, spendable[0])
	if err != nil {
		t.Fatalf("addBlocks stale side: %v", err)
	}
	if len(staleHashes) == 0 {
		t.Fatal("expected stale side-chain blocks")
	}
	staleHash := staleHashes[len(staleHashes)-1]

	// Extend main past the buffer so age-out drops the early fork bodies.
	// Tip becomes 3+9=12; dropHeight=7; stale side heights 2–3 are dropped.
	mainAt3, err := chain.BlockByHeight(3)
	if err != nil {
		t.Fatalf("BlockByHeight(3): %v", err)
	}
	_, spendableMore, err := addBlocks(9, chain, mainAt3, spendable[len(spendable)-1])
	if err != nil {
		t.Fatalf("addBlocks late main: %v", err)
	}
	allSpendable := append(spendable, spendableMore...)

	if got := chain.BestSnapshot().Height; got != 12 {
		t.Fatalf("best height=%d, want 12", got)
	}

	// Recent fork off height 10 — above dropHeight, must stay hot.
	b10, err := chain.BlockByHeight(10)
	if err != nil {
		t.Fatalf("BlockByHeight(10): %v", err)
	}
	// spendable index: blocks after genesis are 0..11 for heights 1..12.
	recentHashes, _, err := addBlocks(1, chain, b10, allSpendable[9])
	if err != nil {
		t.Fatalf("addBlocks recent side: %v", err)
	}
	if len(recentHashes) == 0 {
		t.Fatal("expected recent side-chain block")
	}
	recentHash := recentHashes[0]

	if chain.bestChain.Contains(chain.index.LookupNode(staleHash)) {
		t.Fatal("stale side tip unexpectedly on best chain")
	}
	if chain.bestChain.Contains(chain.index.LookupNode(recentHash)) {
		t.Fatal("recent side tip unexpectedly on best chain")
	}
	// Stale body dropped; header/node retained.
	staleNode := chain.index.LookupNode(staleHash)
	if staleNode == nil {
		t.Fatal("stale side node missing from block index")
	}
	if staleNode.status.HaveData() {
		t.Fatal("stale side node still HaveData after age-out")
	}
	have, err := chain.HaveBlock(staleHash)
	if err != nil {
		t.Fatalf("HaveBlock stale: %v", err)
	}
	if have {
		t.Fatal("HaveBlock true for dropped stale side body")
	}
	err = chain.db.View(func(dbTx database.Tx) error {
		_, err := dbTx.FetchBlock(staleHash)
		if err == nil {
			t.Fatal("FetchBlock succeeded for dropped stale side body")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("View stale: %v", err)
	}

	// Recent side tip still has its body.
	recentNode := chain.index.LookupNode(recentHash)
	if recentNode == nil || !recentNode.status.HaveData() {
		t.Fatal("recent side tip missing HaveData")
	}
	have, err = chain.HaveBlock(recentHash)
	if err != nil {
		t.Fatalf("HaveBlock recent: %v", err)
	}
	if !have {
		t.Fatal("HaveBlock false for recent side tip")
	}
	err = chain.db.View(func(dbTx database.Tx) error {
		got, err := dbTx.FetchBlock(recentHash)
		if err != nil {
			return err
		}
		if len(got) == 0 {
			t.Fatal("empty FetchBlock for recent side tip")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("View recent: %v", err)
	}
}

// TestReconsiderColdAttachPreservesInvalidStatus verifies that ReconsiderBlock
// refuses a cold attach path with ErrWitnessExcised WITHOUT clearing
// statusValidateFailed. Clearing first made a refused reconsider look like a
// successful status reset in memory.
func TestReconsiderColdAttachPreservesInvalidStatus(t *testing.T) {
	chain, params, tearDown := utxoCacheTestChain("TestReconsiderColdAttach")
	defer tearDown()

	tip := btcutil.NewBlock(params.GenesisBlock)
	tip.SetHeight(0)
	_, spendableOuts, err := addBlocks(6, chain, tip, []*testhelper.SpendableOut{})
	if err != nil {
		t.Fatalf("addBlocks: %v", err)
	}

	nextSpends, _ := randomSelect(spendableOuts[len(spendableOuts)-1])
	nextSpends[0].Amount += testhelper.LowFee

	bestBlock, err := chain.BlockByHash(&chain.BestSnapshot().Hash)
	if err != nil {
		t.Fatalf("BlockByHash: %v", err)
	}
	invalidBlock, _, err := newBlock(chain, bestBlock, nextSpends)
	if err != nil {
		t.Fatalf("newBlock: %v", err)
	}
	invalidHash := invalidBlock.Hash()

	if _, _, err := chain.ProcessBlock(invalidBlock, BFNone); err == nil {
		t.Fatal("expected ProcessBlock of invalid block to fail")
	}

	node := chain.index.LookupNode(invalidHash)
	if node == nil || !node.status.KnownInvalid() {
		t.Fatal("expected invalid block to be KnownInvalid after ProcessBlock")
	}

	err = chain.db.Update(func(dbTx database.Tx) error {
		return dbTx.(database.ColdCompactor).CompactBlockToCold(invalidHash)
	})
	if err != nil {
		t.Fatalf("CompactBlockToCold: %v", err)
	}

	err = chain.ReconsiderBlock(invalidHash)
	if err == nil {
		t.Fatal("expected ReconsiderBlock of cold invalid tip to fail")
	}
	rerr, ok := err.(RuleError)
	if !ok {
		t.Fatalf("expected RuleError, got %T: %v", err, err)
	}
	if rerr.ErrorCode != ErrWitnessExcised {
		t.Fatalf("ErrorCode=%v, want ErrWitnessExcised", rerr.ErrorCode)
	}

	node = chain.index.LookupNode(invalidHash)
	if node == nil || !node.status.KnownInvalid() {
		t.Fatal("KnownInvalid cleared despite cold-attach refusal; " +
			"status must be preserved until reconsider can proceed")
	}
}



