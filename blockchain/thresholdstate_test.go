// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/chaingen"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
)

const (
	// vbPrevBlockValid defines the vote bit necessary to vote yes to the
	// previous block being valid.
	vbPrevBlockValid = 0x01

	// invalidChoice is the value returns by thresholdState when not in a
	// state where the choice can be valid.
	invalidChoice = 0xffffffff

	// testDummy1ID is the human-readable ID for the first test dummy voting
	// agenda.
	testDummy1ID = "testdummy1"

	// testDummy1YesIndex is the offset in the choices slice of the first
	// test dummy agenda for the yes choice.
	testDummy1YesIndex = 2

	// testDummy2NoIndex is the offset in the choices slice of the second
	// test dummy agenda for the no choice.
	testDummy2NoIndex = 1

	// vbTestDummy1No defines the vote bits necessary to vote no on the first
	// test dummy agenda as well as yes to the previous block being valid.
	vbTestDummy1No = 0x02

	// vbTestDummy1Yes defines the vote bits necessary to vote yes on the
	// first test dummy agenda as well as yes to the previous block being
	// valid.
	vbTestDummy1Yes = 0x04

	// testDummy2ID is the human-readable ID for the second test dummy
	// voting agenda.
	testDummy2ID = "testdummy2"

	// vbTestDummy2No defines the vote bits necessary to vote no on the
	// second test dummy agenda as well as yes to the previous block being
	// valid.
	vbTestDummy2No = 0x08

	// vbTestDummy2Yes defines the vote bits necessary to vote yes on the
	// second test dummy agenda as well as yes to the previous block being
	// valid.
	vbTestDummy2Yes = 0x10
)

var (
	// testDummy1 is a voting agenda used throughout these tests.
	testDummy1 = chaincfg.Vote{
		Id:          testDummy1ID,
		Description: "",
		Mask:        0x6, // 0b0110
		Choices: []chaincfg.Choice{{
			Id:          "abstain",
			Description: "abstain voting for change",
			Bits:        0x0000,
			IsAbstain:   true,
			IsNo:        false,
		}, {
			Id:          "no",
			Description: "vote no",
			Bits:        0x0002, // Bit 1
			IsAbstain:   false,
			IsNo:        true,
		}, {
			Id:          "yes",
			Description: "vote yes",
			Bits:        0x0004, // Bit 2
			IsAbstain:   false,
			IsNo:        false,
		}},
	}

	// testDummy2 is a voting agenda used throughout these tests.
	testDummy2 = chaincfg.Vote{
		Id:          testDummy2ID,
		Description: "",
		Mask:        0x18, // 0b11000
		Choices: []chaincfg.Choice{{
			Id:          "abstain",
			Description: "abstain voting for change",
			Bits:        0x0000,
			IsAbstain:   true,
			IsNo:        false,
		}, {
			Id:          "no",
			Description: "vote no",
			Bits:        0x0008, // Bit 3
			IsAbstain:   false,
			IsNo:        true,
		}, {
			Id:          "yes",
			Description: "vote yes",
			Bits:        0x0010, // Bit 4
			IsAbstain:   false,
			IsNo:        false,
		}},
	}
)

// TestThresholdState ensures that the threshold state function progresses
// through the states correctly.
func TestThresholdState(t *testing.T) {
	// Create chain params based on simnet params, but add a specific test
	// dummy deployment and set the proof-of-work difficulty readjustment
	// size to a really large number so that the test chain can be generated
	// more quickly.
	posVersion := uint32(4)
	params := chaincfg.SimNetParams
	params.WorkDiffWindowSize = 200000
	params.WorkDiffWindows = 1
	params.TargetTimespan = params.TargetTimePerBlock *
		time.Duration(params.WorkDiffWindowSize)
	if params.Deployments == nil {
		params.Deployments = make(map[uint32][]chaincfg.ConsensusDeployment)
	}
	params.Deployments[posVersion] = append(params.Deployments[posVersion],
		chaincfg.ConsensusDeployment{
			Vote:       testDummy1,
			StartTime:  0,
			ExpireTime: math.MaxUint64,
		})
	params.Deployments[posVersion] = append(params.Deployments[posVersion],
		chaincfg.ConsensusDeployment{
			Vote:       testDummy2,
			StartTime:  0,
			ExpireTime: math.MaxUint64,
		})

	// Create a test generator instance initialized with the genesis block
	// as the tip.
	g, err := chaingen.MakeGenerator(&params)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("thresholdstatetest", &params)
	if err != nil {
		t.Fatalf("Failed to setup chain instance: %v", err)
	}
	defer teardownFunc()

	// accepted processes the current tip block associated with the
	// generator and expects it to be accepted to the main chain.
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

	// testThresholdState queries the threshold state from the current
	// tip block associated with the generator and expects the returned
	// state and choice to match the provided values.
	testThresholdState := func(id string, state blockchain.ThresholdState, choice uint32) {
		tipHash := g.Tip().BlockHash()
		s, err := chain.ThresholdState(&tipHash, posVersion, id)
		if err != nil {
			t.Fatalf("block %q (hash %s, height %d) unexpected "+
				"error when retrieving threshold state: %v",
				g.TipName(), tipHash, g.Tip().Header.Height,
				err)
		}

		if s.State != state {
			t.Fatalf("block %q (hash %s, height %d) unexpected "+
				"threshold state for %s -- got %v, want %v",
				g.TipName(), tipHash, g.Tip().Header.Height,
				id, s.State, state)
		}
		if s.Choice != choice {
			t.Fatalf("block %q (hash %s, height %d) unexpected "+
				"choice for %s -- got %v, want %v",
				g.TipName(), tipHash, g.Tip().Header.Height,
				id, s.Choice, choice)
		}
	}

	// Shorter versions of useful params for convenience.
	ticketsPerBlock := int64(params.TicketsPerBlock)
	coinbaseMaturity := params.CoinbaseMaturity
	stakeEnabledHeight := params.StakeEnabledHeight
	stakeValidationHeight := params.StakeValidationHeight
	stakeVerInterval := params.StakeVersionInterval
	ruleChangeInterval := int64(params.RuleChangeActivationInterval)
	powNumToCheck := int64(params.BlockUpgradeNumToCheck)
	ruleChangeQuorum := int64(params.RuleChangeActivationQuorum)
	ruleChangeMult := int64(params.RuleChangeActivationMultiplier)
	ruleChangeDiv := int64(params.RuleChangeActivationDivisor)

	// ---------------------------------------------------------------------
	// Premine.
	// ---------------------------------------------------------------------

	// Add the required premine block.
	//
	//   genesis -> bp
	g.CreatePremineBlock("bp", 0)
	g.AssertTipHeight(1)
	accepted()
	testThresholdState(testDummy1ID, blockchain.ThresholdDefined, invalidChoice)
	testThresholdState(testDummy2ID, blockchain.ThresholdDefined, invalidChoice)

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
	testThresholdState(testDummy1ID, blockchain.ThresholdDefined, invalidChoice)
	testThresholdState(testDummy2ID, blockchain.ThresholdDefined, invalidChoice)

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
	testThresholdState(testDummy1ID, blockchain.ThresholdDefined, invalidChoice)
	testThresholdState(testDummy2ID, blockchain.ThresholdDefined, invalidChoice)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake validation height while
	// continuing to purchase tickets using the coinbases matured above and
	// allowing the immature tickets to mature and thus become live.
	//
	// The blocks are also generated with version 3 to ensure stake version
	// enforcement is reached.
	// ---------------------------------------------------------------------

	targetPoolSize := int64(g.Params().TicketPoolSize) * ticketsPerBlock
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
		g.NextBlock(blockName, nil, ticketOuts,
			chaingen.ReplaceBlockVersion(3))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight))
	testThresholdState(testDummy1ID, blockchain.ThresholdDefined, invalidChoice)
	testThresholdState(testDummy2ID, blockchain.ThresholdDefined, invalidChoice)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next stake
	// version interval with block version 3, stake version 0, and vote
	// version 3.
	//
	// This will result in triggering enforcement of the stake version and
	// that the stake version is 3.  The treshold state for the test dummy
	// deployments must still be defined since a v4 majority proof-of-work
	// and proof-of-stake upgrade are required before moving to started.
	// ---------------------------------------------------------------------

	blocksNeeded := stakeValidationHeight + stakeVerInterval - 1 -
		int64(g.Tip().Header.Height)
	for i := int64(0); i < blocksNeeded; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtA%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(3),
			chaingen.ReplaceStakeVersion(0),
			chaingen.ReplaceVoteVersions(3))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + stakeVerInterval - 1))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(0)
	testThresholdState(testDummy1ID, blockchain.ThresholdDefined, invalidChoice)
	testThresholdState(testDummy2ID, blockchain.ThresholdDefined, invalidChoice)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next rule change
	// interval with block version 3, stake version 3, and vote version 3.
	//
	// The threshold state for the dummy deployments must still be defined
	// since it can only change on a rule change boundary and it requires a
	// v4 majority proof-of-work and proof-of-stake upgrade before moving to
	// started.
	// ---------------------------------------------------------------------

	blocksNeeded = stakeValidationHeight + ruleChangeInterval - 2 -
		int64(g.Tip().Header.Height)
	for i := int64(0); i < blocksNeeded; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtB%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(3),
			chaingen.ReplaceStakeVersion(3),
			chaingen.ReplaceVoteVersions(3))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + ruleChangeInterval - 2))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(3)
	testThresholdState(testDummy1ID, blockchain.ThresholdDefined, invalidChoice)
	testThresholdState(testDummy2ID, blockchain.ThresholdDefined, invalidChoice)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next stake
	// version interval with block version 3, stake version 3, and vote
	// version 4.
	//
	// This will result in achieving stake version 4 enforcement.
	//
	// The treshold state for the dummy deployments must still be defined
	// since it can only change on a rule change boundary and it still
	// requires a v4 majority proof-of-work upgrade before moving to
	// started.
	// ---------------------------------------------------------------------

	blocksNeeded = stakeValidationHeight + stakeVerInterval*4 - 1 -
		int64(g.Tip().Header.Height)
	for i := int64(0); i < blocksNeeded; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtC%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(3),
			chaingen.ReplaceStakeVersion(3),
			chaingen.ReplaceVoteVersions(4))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + stakeVerInterval*4 - 1))
	g.AssertBlockVersion(3)
	g.AssertStakeVersion(3)
	testThresholdState(testDummy1ID, blockchain.ThresholdDefined, invalidChoice)
	testThresholdState(testDummy2ID, blockchain.ThresholdDefined, invalidChoice)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the next rule change interval with
	// block version 3 majority, stake version 4, and vote version 4.  Set
	// the final two blocks to block version 4 so that majority version 4
	// is not achieved, but the final block in the interval is version 4.
	//
	// The treshold state for the dummy deployments must still be defined
	// since it still requires a v4 majority proof-of-work upgrade before
	// moving to started.
	// ---------------------------------------------------------------------

	blocksNeeded = stakeValidationHeight + ruleChangeInterval*2 - 1 -
		int64(g.Tip().Header.Height)
	for i := int64(0); i < blocksNeeded; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtD%d", i)
		blockVersion := int32(3)
		if i >= blocksNeeded-2 {
			blockVersion = 4
		}
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(blockVersion),
			chaingen.ReplaceStakeVersion(4),
			chaingen.ReplaceVoteVersions(4))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + ruleChangeInterval*2 - 1))
	g.AssertBlockVersion(4)
	g.AssertStakeVersion(4)
	testThresholdState(testDummy1ID, blockchain.ThresholdDefined, invalidChoice)
	testThresholdState(testDummy2ID, blockchain.ThresholdDefined, invalidChoice)

	// ---------------------------------------------------------------------
	// Generate enough blocks to achieve proof-of-work block version lockin
	// with block version 4, stake version 4, and vote version 4.  Also, set
	// the vote bits to include yes votes for the first test dummy agenda
	// and no for the second test dummy agenda for an upcoming test.
	//
	// Since v4 majority proof-of-stake upgrade has been already been
	// achieved and this will achieve v4 majority proof-of-work upgrade,
	// voting can begin at the next rule change interval.
	//
	// The treshold state for the dummy deployments must still be defined
	// since even though all required upgrade conditions are met, the state
	// change must not happen until the start of the next rule change
	// interval.
	// ---------------------------------------------------------------------

	for i := int64(0); i < powNumToCheck; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtE%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(4),
			chaingen.ReplaceStakeVersion(4),
			chaingen.ReplaceVotes(vbPrevBlockValid|vbTestDummy1Yes|
				vbTestDummy2No, 4))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + ruleChangeInterval*2 -
		1 + powNumToCheck))
	g.AssertBlockVersion(4)
	g.AssertStakeVersion(4)
	testThresholdState(testDummy1ID, blockchain.ThresholdDefined, invalidChoice)
	testThresholdState(testDummy2ID, blockchain.ThresholdDefined, invalidChoice)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the next rule change interval with
	// block version 4, stake version 4, and vote version 4.  Also, set the
	// vote bits to include yes votes for the first test dummy agenda and
	// no for the second test dummy agenda to ensure they aren't counted.
	//
	// The treshold state for the dummy deployments must move to started.
	// Even though the majority of the votes have already been voting yes
	// for the first test dummy agenda, and no for the second one, they must
	// not count, otherwise it would move straight to lockedin or failed,
	// respectively.
	// ---------------------------------------------------------------------

	blocksNeeded = stakeValidationHeight + ruleChangeInterval*3 - 1 -
		int64(g.Tip().Header.Height)
	for i := int64(0); i < blocksNeeded; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtF%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(4),
			chaingen.ReplaceStakeVersion(4),
			chaingen.ReplaceVotes(vbPrevBlockValid|vbTestDummy1Yes|
				vbTestDummy2No, 4))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + ruleChangeInterval*3 - 1))
	g.AssertBlockVersion(4)
	g.AssertStakeVersion(4)
	testThresholdState(testDummy1ID, blockchain.ThresholdStarted, invalidChoice)
	testThresholdState(testDummy2ID, blockchain.ThresholdStarted, invalidChoice)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the next rule change interval with
	// block version 4, stake version 4, and vote version 3.  Also, set the
	// vote bits to include yes votes for the first test dummy agenda and
	// no for the second test dummy agenda to ensure they aren't counted.
	//
	// The treshold state for the dummy deployments must remain in started
	// because the votes are an old version and thus have a different
	// definition and don't apply to version 4.
	// ---------------------------------------------------------------------

	blocksNeeded = stakeValidationHeight + ruleChangeInterval*4 - 1 -
		int64(g.Tip().Header.Height)
	for i := int64(0); i < blocksNeeded; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtG%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(4),
			chaingen.ReplaceStakeVersion(4),
			chaingen.ReplaceVotes(vbPrevBlockValid|vbTestDummy1Yes|
				vbTestDummy2No, 3))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + ruleChangeInterval*4 - 1))
	g.AssertBlockVersion(4)
	g.AssertStakeVersion(4)
	testThresholdState(testDummy1ID, blockchain.ThresholdStarted, invalidChoice)
	testThresholdState(testDummy2ID, blockchain.ThresholdStarted, invalidChoice)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the next rule change interval with
	// block version 4, stake version 4, and vote version 4.  Set the vote
	// bits such that quorum is not reached, but there is a majority yes
	// votes for the first test dummy agenda and a majority no for the
	// second test dummy agenda.
	//
	// The treshold state for the dummy deployments must remain in started
	// because quorum was not reached.
	// ---------------------------------------------------------------------

	var totalVotes int64
	blocksNeeded = stakeValidationHeight + ruleChangeInterval*5 - 1 -
		int64(g.Tip().Header.Height)
	for i := int64(0); i < blocksNeeded; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtH%d", i)
		voteBits := uint16(vbPrevBlockValid) // Abstain both test dummy
		if totalVotes+ticketsPerBlock < ruleChangeQuorum {
			voteBits = vbPrevBlockValid | vbTestDummy1Yes |
				vbTestDummy2No
		}
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(4),
			chaingen.ReplaceStakeVersion(4),
			chaingen.ReplaceVotes(voteBits, 4))
		totalVotes += ticketsPerBlock
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + ruleChangeInterval*5 - 1))
	g.AssertBlockVersion(4)
	g.AssertStakeVersion(4)
	testThresholdState(testDummy1ID, blockchain.ThresholdStarted, invalidChoice)
	testThresholdState(testDummy2ID, blockchain.ThresholdStarted, invalidChoice)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the next rule change interval with
	// block version 4, stake version 4, and vote version 4.  Set the vote
	// bits such that quorum is reached, but there are a few votes shy of a
	// majority yes for the first test dummy agenda and a few votes shy of a
	// majority no for the second test dummy agenda.
	//
	// The treshold state for the dummy deployments must remain in started
	// because even though quorum was reached, a required majority was not.
	// ---------------------------------------------------------------------

	blocksNeeded = stakeValidationHeight + ruleChangeInterval*6 - 1 -
		int64(g.Tip().Header.Height)
	totalVotes = 0
	numActiveNeeded := ruleChangeQuorum * 2
	numMinorityNeeded := numActiveNeeded*ruleChangeMult/ruleChangeDiv - 1
	if numActiveNeeded > ticketsPerBlock*blocksNeeded {
		numActiveNeeded = ticketsPerBlock * blocksNeeded
	}
	for i := int64(0); i < blocksNeeded; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtI%d", i)
		voteBits := uint16(vbPrevBlockValid) // Abstain both test dummy
		if totalVotes+ticketsPerBlock < numMinorityNeeded {
			voteBits = vbPrevBlockValid | vbTestDummy1Yes |
				vbTestDummy2No
		} else if totalVotes+ticketsPerBlock <= numActiveNeeded {
			voteBits = vbPrevBlockValid | vbTestDummy1No |
				vbTestDummy2Yes
		}
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(4),
			chaingen.ReplaceStakeVersion(4),
			chaingen.ReplaceVotes(voteBits, 4))
		totalVotes += int64(ticketsPerBlock)
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + ruleChangeInterval*6 - 1))
	g.AssertBlockVersion(4)
	g.AssertStakeVersion(4)
	testThresholdState(testDummy1ID, blockchain.ThresholdStarted, invalidChoice)
	testThresholdState(testDummy2ID, blockchain.ThresholdStarted, invalidChoice)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the next rule change interval with
	// block version 4, stake version 4, and vote version 4.  Also, set the
	// vote bits to yes for the first test dummy agenda and no to the second
	// one.
	//
	// The treshold state for the first dummy deployment must move to
	// lockedin since a majority yes vote was achieved while the second
	// dummy deployment must move to failed since a majority no vote was
	// achieved.
	// ---------------------------------------------------------------------

	blocksNeeded = stakeValidationHeight + ruleChangeInterval*7 - 1 -
		int64(g.Tip().Header.Height)
	for i := int64(0); i < blocksNeeded; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtJ%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(4),
			chaingen.ReplaceStakeVersion(4),
			chaingen.ReplaceVotes(vbPrevBlockValid|vbTestDummy1Yes|
				vbTestDummy2No, 4))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + ruleChangeInterval*7 - 1))
	g.AssertBlockVersion(4)
	g.AssertStakeVersion(4)
	testThresholdState(testDummy1ID, blockchain.ThresholdLockedIn, testDummy1YesIndex)
	testThresholdState(testDummy2ID, blockchain.ThresholdFailed, testDummy2NoIndex)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the next rule change interval with
	// block version 4, stake version 4, and vote version 4.  Also, set the
	// vote bits to include no votes for the first test dummy agenda and
	// yes votes for the second one.
	//
	// The treshold state for the first dummy deployment must move to active
	// since even though the interval had a majority no votes, lockedin
	// status has already been achieved and can't be undone without a new
	// agenda.  Similarly, the second one must remain in failed even though
	// the interval had a majority yes votes since a failed state can't be
	// undone.
	// ---------------------------------------------------------------------

	blocksNeeded = stakeValidationHeight + ruleChangeInterval*8 - 1 -
		int64(g.Tip().Header.Height)
	for i := int64(0); i < blocksNeeded; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtK%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(4),
			chaingen.ReplaceStakeVersion(4),
			chaingen.ReplaceVotes(vbPrevBlockValid|vbTestDummy1No|
				vbTestDummy2Yes, 4))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + ruleChangeInterval*8 - 1))
	g.AssertBlockVersion(4)
	g.AssertStakeVersion(4)
	testThresholdState(testDummy1ID, blockchain.ThresholdActive, testDummy1YesIndex)
	testThresholdState(testDummy2ID, blockchain.ThresholdFailed, testDummy2NoIndex)
}
