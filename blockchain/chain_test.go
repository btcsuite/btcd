// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"compress/bzip2"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/blockchain/chaingen"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
)

// cloneParams returns a deep copy of the provided parameters so the caller is
// free to modify them without worrying about interfering with other tests.
func cloneParams(params *chaincfg.Params) *chaincfg.Params {
	// Encode via gob.
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	enc.Encode(params)

	// Decode via gob to make a deep copy.
	var paramsCopy chaincfg.Params
	dec := gob.NewDecoder(buf)
	dec.Decode(&paramsCopy)
	return &paramsCopy
}

// TestBlockchainFunction tests the various blockchain API to ensure proper
// functionality.
func TestBlockchainFunctions(t *testing.T) {
	// Update simnet parameters to reflect what is expected by the legacy
	// data.
	params := cloneParams(&chaincfg.SimNetParams)
	params.GenesisBlock.Header.MerkleRoot = *mustParseHash("a216ea043f0d481a072424af646787794c32bcefd3ed181a090319bbf8a37105")
	genesisHash := params.GenesisBlock.BlockHash()
	params.GenesisHash = &genesisHash

	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("validateunittests", params)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	// Load up the rest of the blocks up to HEAD~1.
	filename := filepath.Join("testdata/", "blocks0to168.bz2")
	fi, err := os.Open(filename)
	if err != nil {
		t.Errorf("Unable to open %s: %v", filename, err)
	}
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file.
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data.
	bcDecoder := gob.NewDecoder(bcBuf)
	blockChain := make(map[int64][]byte)

	// Decode the blockchain into the map.
	if err := bcDecoder.Decode(&blockChain); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	// Insert blocks 1 to 168 and perform various tests.
	for i := 1; i <= 168; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Errorf("NewBlockFromBytes error: %v", err.Error())
		}

		_, _, err = chain.ProcessBlock(bl, BFNone)
		if err != nil {
			t.Fatalf("ProcessBlock error at height %v: %v", i, err.Error())
		}
	}

	val, err := chain.TicketPoolValue()
	if err != nil {
		t.Errorf("Failed to get ticket pool value: %v", err)
	}
	expectedVal := dcrutil.Amount(3495091704)
	if val != expectedVal {
		t.Errorf("Failed to get correct result for ticket pool value; "+
			"want %v, got %v", expectedVal, val)
	}

	a, _ := dcrutil.DecodeAddress("SsbKpMkPnadDcZFFZqRPY8nvdFagrktKuzB")
	hs, err := chain.TicketsWithAddress(a)
	if err != nil {
		t.Errorf("Failed to do TicketsWithAddress: %v", err)
	}
	expectedLen := 223
	if len(hs) != expectedLen {
		t.Errorf("Failed to get correct number of tickets for "+
			"TicketsWithAddress; want %v, got %v", expectedLen, len(hs))
	}

	totalSubsidy := chain.TotalSubsidy()
	expectedSubsidy := int64(35783267326630)
	if expectedSubsidy != totalSubsidy {
		t.Errorf("Failed to get correct total subsidy for "+
			"TotalSubsidy; want %v, got %v", expectedSubsidy,
			totalSubsidy)
	}
}

// TestForceHeadReorg ensures forcing header reorganization works as expected.
func TestForceHeadReorg(t *testing.T) {
	// Create a test generator instance initialized with the genesis block
	// as the tip as well as some cached payment scripts to be used
	// throughout the tests.
	params := &chaincfg.SimNetParams
	g, err := chaingen.MakeGenerator(params)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("forceheadreorgtest", params)
	if err != nil {
		t.Fatalf("Failed to setup chain instance: %v", err)
	}
	defer teardownFunc()

	// Define some convenience helper functions to process the current tip
	// block associated with the generator.
	//
	// accepted expects the block to be accepted to the main chain.
	//
	// expectTip expects the provided block to be the current tip of the
	// main chain.
	//
	// acceptedToSideChainWithExpectedTip expects the block to be accepted
	// to a side chain, but the current best chain tip to be the provided
	// value.
	//
	// forceTipReorg forces the chain instance to reorganize the current tip
	// of the main chain from the given block to the given block.  An error
	// will result if the provided from block is not actually the current
	// tip.
	accepted := func() {
		msgBlock := g.Tip()
		blockHeight := msgBlock.Header.Height
		block := dcrutil.NewBlock(msgBlock)
		t.Logf("Testing block %s (hash %s, height %d)",
			g.TipName(), block.Hash(), blockHeight)

		forkLen, isOrphan, err := chain.ProcessBlock(block, BFNone)
		if err != nil {
			t.Fatalf("block %q (hash %s, height %d) should "+
				"have been accepted: %v", g.TipName(),
				block.Hash(), blockHeight, err)
		}

		// Ensure the main chain and orphan flags match the values
		// specified in the test.
		isMainChain := !isOrphan && forkLen == 0
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
	expectTip := func(tipName string) {
		// Ensure hash and height match.
		wantTip := g.BlockByName(tipName)
		best := chain.BestSnapshot()
		if best.Hash != wantTip.BlockHash() ||
			best.Height != int64(wantTip.Header.Height) {
			t.Fatalf("block %q (hash %s, height %d) should be "+
				"the current tip -- got (hash %s, height %d)",
				tipName, wantTip.BlockHash(),
				wantTip.Header.Height, best.Hash, best.Height)
		}
	}
	acceptedToSideChainWithExpectedTip := func(tipName string) {
		msgBlock := g.Tip()
		blockHeight := msgBlock.Header.Height
		block := dcrutil.NewBlock(msgBlock)
		t.Logf("Testing block %s (hash %s, height %d)",
			g.TipName(), block.Hash(), blockHeight)

		forkLen, isOrphan, err := chain.ProcessBlock(block, BFNone)
		if err != nil {
			t.Fatalf("block %q (hash %s, height %d) should "+
				"have been accepted: %v", g.TipName(),
				block.Hash(), blockHeight, err)
		}

		// Ensure the main chain and orphan flags match the values
		// specified in the test.
		isMainChain := !isOrphan && forkLen == 0
		if isMainChain {
			t.Fatalf("block %q (hash %s, height %d) unexpected main "+
				"chain flag -- got %v, want false", g.TipName(),
				block.Hash(), blockHeight, isMainChain)
		}
		if isOrphan {
			t.Fatalf("block %q (hash %s, height %d) unexpected "+
				"orphan flag -- got %v, want false", g.TipName(),
				block.Hash(), blockHeight, isOrphan)
		}

		expectTip(tipName)
	}
	forceTipReorg := func(fromTipName, toTipName string) {
		from := g.BlockByName(fromTipName)
		to := g.BlockByName(toTipName)
		err = chain.ForceHeadReorganization(from.BlockHash(), to.BlockHash())
		if err != nil {
			t.Fatalf("failed to force header reorg from block %q "+
				"(hash %s, height %d) to block %q (hash %s, "+
				"height %d): %v", fromTipName, from.BlockHash(),
				from.Header.Height, toTipName, to.BlockHash(),
				to.Header.Height, err)
		}
	}

	// Shorter versions of useful params for convenience.
	ticketsPerBlock := params.TicketsPerBlock
	coinbaseMaturity := params.CoinbaseMaturity
	stakeEnabledHeight := params.StakeEnabledHeight
	stakeValidationHeight := params.StakeValidationHeight

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
	// Generate enough blocks to have a known distance to the first mature
	// coinbase outputs for all tests that follow.  These blocks continue
	// to purchase tickets to avoid running out of votes.
	//
	//   ... -> bsv# -> bbm0 -> bbm1 -> ... -> bbm#
	// ---------------------------------------------------------------------

	for i := uint16(0); i < coinbaseMaturity; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bbm%d", i)
		g.NextBlock(blockName, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight) + uint32(coinbaseMaturity))

	// Collect spendable outputs into two different slices.  The outs slice
	// is intended to be used for regular transactions that spend from the
	// output, while the ticketOuts slice is intended to be used for stake
	// ticket purchases.
	var outs []*chaingen.SpendableOut
	var ticketOuts [][]chaingen.SpendableOut
	for i := uint16(0); i < coinbaseMaturity; i++ {
		coinbaseOuts := g.OldestCoinbaseOuts()
		outs = append(outs, &coinbaseOuts[0])
		ticketOuts = append(ticketOuts, coinbaseOuts[1:])
	}

	// ---------------------------------------------------------------------
	// Forced header reorganization test.
	// ---------------------------------------------------------------------

	// Start by building a couple of blocks at current tip (value in parens
	// is which output is spent):
	//
	//   ... -> b1(0) -> b2(1)
	g.NextBlock("b1", outs[0], ticketOuts[0])
	accepted()

	g.NextBlock("b2", outs[1], ticketOuts[1])
	accepted()

	// Create some forks from b1.  There should not be a reorg since b2 was
	// seen first.
	//
	//   ... -> b1(0) -> b2(1)
	//               \-> b3(1)
	//               \-> b4(1)
	//               \-> b5(1)
	g.SetTip("b1")
	g.NextBlock("b3", outs[1], ticketOuts[1])
	acceptedToSideChainWithExpectedTip("b2")

	g.SetTip("b1")
	g.NextBlock("b4", outs[1], ticketOuts[1])
	acceptedToSideChainWithExpectedTip("b2")

	g.SetTip("b1")
	g.NextBlock("b5", outs[1], ticketOuts[1])
	acceptedToSideChainWithExpectedTip("b2")

	// Force tip reorganization to b3.
	//
	//   ... -> b1(0) -> b3(1)
	//               \-> b2(1)
	//               \-> b4(1)
	//               \-> b5(1)
	forceTipReorg("b2", "b3")
	expectTip("b3")

	// Force tip reorganization to b4.
	//
	//   ... -> b1(0) -> b4(1)
	//               \-> b2(1)
	//               \-> b3(1)
	//               \-> b5(1)
	forceTipReorg("b3", "b4")
	expectTip("b4")

	// Force tip reorganization to b5.
	//
	//   ... -> b1(0) -> b5(1)
	//               \-> b2(1)
	//               \-> b3(1)
	//               \-> b4(1)
	forceTipReorg("b4", "b5")
	expectTip("b5")
}
