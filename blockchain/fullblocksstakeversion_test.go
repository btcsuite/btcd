// Copyright (c) 2016-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"fmt"
	"testing"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/chaingen"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
)

// TestStakeVersion ensures that the stake version field in the block header is
// enforced properly.
func TestStakeVersion(t *testing.T) {
	// Create a test generator instance initialized with the genesis block
	// as the tip as well as some cached payment scripts to be used
	// throughout the tests.
	params := &chaincfg.SimNetParams
	g, err := chaingen.MakeGenerator(params)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("stakeversiontest", params)
	if err != nil {
		t.Fatalf("Failed to setup chain instance: %v", err)
	}
	defer teardownFunc()

	// Define some convenience helper functions to process the current tip
	// block associated with the generator.
	//
	// accepted expects the block to be accepted to the main chain.
	//
	// rejected expects the block to be rejected with the provided error
	// code.
	accepted := func() {
		msgBlock := g.Tip()
		blockHeight := msgBlock.Header.Height
		block := dcrutil.NewBlock(msgBlock)
		t.Logf("Testing block %s (hash %s, height %d)",
			g.TipName(), block.Hash(), blockHeight)

		isMainChain, isOrphan, err := chain.ProcessBlock(block,
			blockchain.BFNone)
		if err != nil {
			t.Fatalf("block %q (hash %s, height %d) should "+
				"have been accepted: %v", g.TipName(),
				block.Hash(), blockHeight, err)
		}

		// Ensure the main chain and orphan flags match the values
		// specified in the test.
		if !isMainChain {
			t.Fatalf("block %q (hash %s, height %d) unexpected main "+
				"chain flag -- got %v, want true", g.TipName(),
				block.Hash(), blockHeight, isMainChain)
		}
		if isOrphan {
			t.Fatalf("block %q (hash %s, height %d) unexpected "+
				"orphan flag -- got %v, want false", g.TipName(),
				block.Hash(), blockHeight, isOrphan)
		}
	}
	rejected := func(code blockchain.ErrorCode) {
		msgBlock := g.Tip()
		blockHeight := msgBlock.Header.Height
		block := dcrutil.NewBlock(msgBlock)
		t.Logf("Testing block %s (hash %s, height %d)", g.TipName(),
			block.Hash(), blockHeight)

		_, _, err := chain.ProcessBlock(block, blockchain.BFNone)
		if err == nil {
			t.Fatalf("block %q (hash %s, height %d) should not "+
				"have been accepted", g.TipName(), block.Hash(),
				blockHeight)
		}

		// Ensure the error code is of the expected type and the reject
		// code matches the value specified in the test instance.
		rerr, ok := err.(blockchain.RuleError)
		if !ok {
			t.Fatalf("block %q (hash %s, height %d) returned "+
				"unexpected error type -- got %T, want "+
				"blockchain.RuleError", g.TipName(),
				block.Hash(), blockHeight, err)
		}
		if rerr.ErrorCode != code {
			t.Fatalf("block %q (hash %s, height %d) does not have "+
				"expected reject code -- got %v, want %v",
				g.TipName(), block.Hash(), blockHeight,
				rerr.ErrorCode, code)
		}
	}

	// Shorter versions of useful params for convenience.
	ticketsPerBlock := params.TicketsPerBlock
	coinbaseMaturity := params.CoinbaseMaturity
	stakeEnabledHeight := params.StakeEnabledHeight
	stakeValidationHeight := params.StakeValidationHeight
	stakeVerInterval := params.StakeVersionInterval
	stakeMajorityMul := int64(params.StakeMajorityMultiplier)
	stakeMajorityDiv := int64(params.StakeMajorityDivisor)

	// ---------------------------------------------------------------------
	// Premine.
	// ---------------------------------------------------------------------

	// Add the required premine block.
	//
	//   genesis -> bp
	g.CreatePremineBlock("bp", 0)
	g.AssertTipHeight(1)
	accepted()

	// ---------------------------------------------------------------------
	// Generate enough blocks to have mature coinbase outputs to work with.
	//
	//   genesis -> bp -> bm0 -> bm1 -> ... -> bm#
	// ---------------------------------------------------------------------

	for i := uint16(0); i < coinbaseMaturity; i++ {
		blockName := fmt.Sprintf("bm%d", i)
		g.NextBlock(blockName, nil, nil)
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(coinbaseMaturity) + 1)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake enabled height while
	// creating ticket purchases that spend from the coinbases matured
	// above.  This will also populate the pool of immature tickets.
	//
	//   ... -> bm# ... -> bse0 -> bse1 -> ... -> bse#
	// ---------------------------------------------------------------------

	var ticketsPurchased int
	for i := int64(0); int64(g.Tip().Header.Height) < stakeEnabledHeight; i++ {
		outs := g.OldestCoinbaseOuts()
		ticketOuts := outs[1:]
		ticketsPurchased += len(ticketOuts)
		blockName := fmt.Sprintf("bse%d", i)
		g.NextBlock(blockName, nil, ticketOuts)
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeEnabledHeight))

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake validation height while
	// continuing to purchase tickets using the coinbases matured above and
	// allowing the immature tickets to mature and thus become live.
	// ---------------------------------------------------------------------

	targetPoolSize := g.Params().TicketPoolSize * ticketsPerBlock
	for i := int64(0); int64(g.Tip().Header.Height) < stakeValidationHeight; i++ {
		// Only purchase tickets until the target ticket pool size is
		// reached.
		outs := g.OldestCoinbaseOuts()
		ticketOuts := outs[1:]
		if ticketsPurchased+len(ticketOuts) > int(targetPoolSize) {
			ticketsNeeded := int(targetPoolSize) - ticketsPurchased
			if ticketsNeeded > 0 {
				ticketOuts = ticketOuts[1 : ticketsNeeded+1]
			} else {
				ticketOuts = nil
			}
		}
		ticketsPurchased += len(ticketOuts)

		blockName := fmt.Sprintf("bsv%d", i)
		g.NextBlock(blockName, nil, ticketOuts)
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight))

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next stake
	// version interval with block version 2, stake version 0, and vote
	// version 3.
	//
	// This will result in a majority of blocks with a version prior to
	// version 3 where stake version enforcement begins and thus it must not
	// be enforced.
	// ---------------------------------------------------------------------

	for i := int64(0); i < stakeVerInterval-1; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtA%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(2),
			chaingen.ReplaceStakeVersion(0),
			chaingen.ReplaceVoteVersions(3))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + stakeVerInterval - 1))
	g.AssertBlockVersion(2)
	g.AssertStakeVersion(0)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 42, and
	// vote version 41.
	//
	// This block must be accepted because even though it is a version 3
	// block with an invalid stake version, there have not yet been a
	// majority of version 3 blocks which is required to trigger stake
	// version enforcement.
	// ---------------------------------------------------------------------

	outs := g.OldestCoinbaseOuts()
	g.NextBlock("bsvtB0", nil, outs[1:],
		chaingen.ReplaceBlockVersion(3),
		chaingen.ReplaceStakeVersion(42),
		chaingen.ReplaceVoteVersions(41))
	g.SaveTipCoinbaseOuts()
	accepted()
	g.AssertTipHeight(uint32(stakeValidationHeight + stakeVerInterval))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(42) // expected bogus

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next stake
	// version interval with block version 3, stake version 0, and vote
	// version 2.
	//
	// This will result in a majority of version 3 blocks which will trigger
	// enforcement of the stake version.  It also results in a majority of
	// version 2 votes, however, since enforcement is not yet active in this
	// interval, they will not actually count toward establishing a
	// majority.
	// ---------------------------------------------------------------------

	for i := int64(0); i < stakeVerInterval-1; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtB%d", i+1)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(3),
			chaingen.ReplaceStakeVersion(0),
			chaingen.ReplaceVoteVersions(2))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + 2*stakeVerInterval - 1))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(0)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 2, and
	// vote version 3.
	//
	// This block must be rejected because even though the majority stake
	// version per voters was 2 in the previous period, stake version
	// enforcement had not yet been achieved and thus the required stake
	// version is still 0.
	// ---------------------------------------------------------------------

	g.NextBlock("bsvtCbad0", nil, nil,
		chaingen.ReplaceBlockVersion(3),
		chaingen.ReplaceStakeVersion(2),
		chaingen.ReplaceVoteVersions(3))
	g.AssertTipHeight(uint32(stakeValidationHeight + 2*stakeVerInterval))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(2)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 1, and
	// vote version 3.
	//
	// This block must be rejected because even though the majority stake
	// version per voters was 2 in the previous period, stake version
	// enforcement had not yet been achieved and thus the required stake
	// version is still 0.
	// ---------------------------------------------------------------------

	g.SetTip(fmt.Sprintf("bsvtB%d", stakeVerInterval-1))
	g.NextBlock("bsvtCbad1", nil, nil,
		chaingen.ReplaceBlockVersion(3),
		chaingen.ReplaceStakeVersion(1),
		chaingen.ReplaceVoteVersions(3))
	g.AssertTipHeight(uint32(stakeValidationHeight + 2*stakeVerInterval))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(1)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next stake
	// version interval with block version 3, stake version 0, and vote
	// version 3.
	//
	// This will result in a majority of version 3 votes which will trigger
	// enforcement of a bump in the stake version to 3.
	// ---------------------------------------------------------------------

	g.SetTip(fmt.Sprintf("bsvtB%d", stakeVerInterval-1))
	for i := int64(0); i < stakeVerInterval; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtC%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(3),
			chaingen.ReplaceStakeVersion(0),
			chaingen.ReplaceVoteVersions(3))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + 3*stakeVerInterval - 1))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(0)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 2, and
	// vote version 2.
	//
	// This block must be rejected because the majority stake version per
	// voters is now 3 and stake version enforcement has been achieved.
	// ---------------------------------------------------------------------

	g.NextBlock("bsvtDbad0", nil, nil,
		chaingen.ReplaceBlockVersion(3),
		chaingen.ReplaceStakeVersion(2),
		chaingen.ReplaceVoteVersions(2))
	g.AssertTipHeight(uint32(stakeValidationHeight + 3*stakeVerInterval))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(2)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 4, and
	// vote version 2.
	//
	// This block must be rejected because the majority stake version per
	// voters is now 3 and stake version enforcement has been achieved.
	// ---------------------------------------------------------------------

	g.SetTip(fmt.Sprintf("bsvtC%d", stakeVerInterval-1))
	g.NextBlock("bsvtDbad1", nil, nil,
		chaingen.ReplaceBlockVersion(3),
		chaingen.ReplaceStakeVersion(4),
		chaingen.ReplaceVoteVersions(2))
	g.AssertTipHeight(uint32(stakeValidationHeight + 3*stakeVerInterval))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(4)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next stake
	// version interval with block version 3, stake version 3, and vote
	// version 2.
	//
	// This will result in a majority of version 2 votes, but since version
	// 3 has already been achieved, the stake version must not regress.
	// ---------------------------------------------------------------------

	g.SetTip(fmt.Sprintf("bsvtC%d", stakeVerInterval-1))
	for i := int64(0); i < stakeVerInterval; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtD%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(3),
			chaingen.ReplaceStakeVersion(3),
			chaingen.ReplaceVoteVersions(2))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + 4*stakeVerInterval - 1))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(3)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 2, and
	// vote version 2.
	//
	// This block must be rejected because even though the majority stake
	// version per voters in the previous interval was 2, the majority stake
	// version is not allowed to regress.
	// ---------------------------------------------------------------------

	g.NextBlock("bsvtEbad0", nil, nil,
		chaingen.ReplaceBlockVersion(3),
		chaingen.ReplaceStakeVersion(2),
		chaingen.ReplaceVoteVersions(2))
	g.AssertTipHeight(uint32(stakeValidationHeight + 4*stakeVerInterval))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(2)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 4, and
	// vote version 3.
	//
	// This block must be rejected because the majority stake version is
	// still 3.
	// ---------------------------------------------------------------------

	g.SetTip(fmt.Sprintf("bsvtD%d", stakeVerInterval-1))
	g.NextBlock("bsvtEbad1", nil, nil,
		chaingen.ReplaceBlockVersion(3),
		chaingen.ReplaceStakeVersion(4),
		chaingen.ReplaceVoteVersions(3))
	g.AssertTipHeight(uint32(stakeValidationHeight + 4*stakeVerInterval))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(4)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next stake
	// version interval with block version 3, stake version 3, and a mix of
	// 3 and 4 for the vote version such that a super majority is *NOT*
	// achieved.
	//
	// This will result in an unchanged required stake version.
	// ---------------------------------------------------------------------

	g.SetTip(fmt.Sprintf("bsvtD%d", stakeVerInterval-1))
	votesPerInterval := stakeVerInterval * int64(ticketsPerBlock)
	targetVotes := (votesPerInterval * stakeMajorityMul) / stakeMajorityDiv
	targetBlocks := targetVotes / int64(ticketsPerBlock)
	for i := int64(0); i < targetBlocks-1; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtE%da", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(3),
			chaingen.ReplaceStakeVersion(3),
			chaingen.ReplaceVoteVersions(4))
		g.SaveTipCoinbaseOuts()
		g.AssertBlockVersion(3)
		g.AssertStakeVersion(3)
		accepted()
	}
	for i := int64(0); i < stakeVerInterval-(targetBlocks-1); i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtE%db", targetBlocks-1+i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(3),
			chaingen.ReplaceStakeVersion(3),
			chaingen.ReplaceVoteVersions(3))
		g.SaveTipCoinbaseOuts()
		g.AssertBlockVersion(3)
		g.AssertStakeVersion(3)
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + 5*stakeVerInterval - 1))

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 4, and
	// vote version 3.
	//
	// This block must be rejected because the majority stake version is
	// still 3 due to failing to achieve enough votes in the previous
	// period.
	// ---------------------------------------------------------------------

	g.NextBlock("bsvtFbad0", nil, nil,
		chaingen.ReplaceBlockVersion(3),
		chaingen.ReplaceStakeVersion(4),
		chaingen.ReplaceVoteVersions(3))
	g.AssertTipHeight(uint32(stakeValidationHeight + 5*stakeVerInterval))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(4)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next stake
	// version interval with block version 3, stake version 3, and a mix of
	// 3 and 4 for the vote version such that a super majority of version 4
	// is achieved.
	//
	// This will result in a majority stake version of 4.
	// ---------------------------------------------------------------------

	g.SetTip(fmt.Sprintf("bsvtE%db", stakeVerInterval-1))
	for i := int64(0); i < targetBlocks; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtF%da", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(3),
			chaingen.ReplaceStakeVersion(3),
			chaingen.ReplaceVoteVersions(4))
		g.SaveTipCoinbaseOuts()
		g.AssertBlockVersion(3)
		g.AssertStakeVersion(3)
		accepted()
	}
	for i := int64(0); i < stakeVerInterval-targetBlocks; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtF%db", targetBlocks+i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(3),
			chaingen.ReplaceStakeVersion(3),
			chaingen.ReplaceVoteVersions(3))
		g.SaveTipCoinbaseOuts()
		g.AssertBlockVersion(3)
		g.AssertStakeVersion(3)
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + 6*stakeVerInterval - 1))

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 3, and
	// vote version 3.
	//
	// This block must be rejected because the majority stake version is
	// now 4 due to achieving a majority of votes in the previous period.
	// ---------------------------------------------------------------------

	g.NextBlock("bsvtGbad0", nil, nil,
		chaingen.ReplaceBlockVersion(3),
		chaingen.ReplaceStakeVersion(3),
		chaingen.ReplaceVoteVersions(3))
	g.AssertTipHeight(uint32(stakeValidationHeight + 6*stakeVerInterval))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(3)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 4, and
	// vote version 3.
	//
	// This block must be accepted because the majority stake version is
	// now 4 due to achieving a majority of votes in the previous period.
	// ---------------------------------------------------------------------

	g.SetTip(fmt.Sprintf("bsvtF%db", stakeVerInterval-1))
	g.NextBlock("bsvtG0", nil, nil,
		chaingen.ReplaceBlockVersion(3),
		chaingen.ReplaceStakeVersion(4),
		chaingen.ReplaceVoteVersions(3))
	g.AssertTipHeight(uint32(stakeValidationHeight + 6*stakeVerInterval))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(4)
	accepted()
}
