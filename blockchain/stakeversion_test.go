// Copyright (c) 2016-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// newFakeChain returns a chain that is usable for syntetic tests.  It is
// important to note that this chain has no database associated with it, so
// it is not usable with all functions and the tests must take care when making
// use of it.
func newFakeChain(params *chaincfg.Params) *BlockChain {
	// Create a genesis block node and block index populated with it for use
	// when creating the fake chain below.
	node := newBlockNode(&params.GenesisBlock.Header, nil)
	node.inMainChain = true
	index := newBlockIndex(nil, params)
	index.AddNode(node)

	return &BlockChain{
		chainParams:      params,
		deploymentCaches: newThresholdCaches(params),
		bestNode:         node,
		index:            index,
		isVoterMajorityVersionCache:   make(map[[stakeMajorityCacheKeySize]byte]bool),
		isStakeMajorityVersionCache:   make(map[[stakeMajorityCacheKeySize]byte]bool),
		calcPriorStakeVersionCache:    make(map[[chainhash.HashSize]byte]uint32),
		calcVoterVersionIntervalCache: make(map[[chainhash.HashSize]byte]uint32),
		calcStakeVersionCache:         make(map[[chainhash.HashSize]byte]uint32),
	}
}

// newFakeNode creates a block node connected to the passed parent with the
// provided fields populated and fake values for the other fields.
func newFakeNode(parent *blockNode, blockVersion int32, stakeVersion uint32, bits uint32, timestamp time.Time) *blockNode {
	// Make up a header and create a block node from it.
	header := &wire.BlockHeader{
		Version:      blockVersion,
		PrevBlock:    parent.hash,
		VoteBits:     0x01,
		Bits:         bits,
		Height:       uint32(parent.height) + 1,
		Timestamp:    timestamp,
		StakeVersion: stakeVersion,
	}
	return newBlockNode(header, parent)
}

// appendFakeVotes appends the passed number of votes to the node with the
// provided version and vote bits.
func appendFakeVotes(node *blockNode, numVotes uint16, voteVersion uint32, voteBits uint16) {
	for i := uint16(0); i < numVotes; i++ {
		node.votes = append(node.votes, stake.VoteVersionTuple{
			Version: voteVersion,
			Bits:    voteBits,
		})
	}
}

func TestCalcWantHeight(t *testing.T) {
	// For example, if StakeVersionInterval = 11 and StakeValidationHeight = 13 the
	// windows start at 13 + (11 * 2) 25 and are as follows: 24-34, 35-45, 46-56 ...
	// If height comes in at 35 we use the 24-34 window, up to height 45.
	// If height comes in at 46 we use the 35-45 window, up to height 56 etc.
	tests := []struct {
		name       string
		skip       int64
		interval   int64
		multiplier int64
		negative   int64
	}{
		{
			name:       "13 11 10000",
			skip:       13,
			interval:   11,
			multiplier: 10000,
		},
		{
			name:       "27 33 10000",
			skip:       27,
			interval:   33,
			multiplier: 10000,
		},
		{
			name:       "mainnet params",
			skip:       chaincfg.MainNetParams.StakeValidationHeight,
			interval:   chaincfg.MainNetParams.StakeVersionInterval,
			multiplier: 5000,
		},
		{
			name:       "testnet2 params",
			skip:       chaincfg.TestNet2Params.StakeValidationHeight,
			interval:   chaincfg.TestNet2Params.StakeVersionInterval,
			multiplier: 1000,
		},
		{
			name:       "simnet params",
			skip:       chaincfg.SimNetParams.StakeValidationHeight,
			interval:   chaincfg.SimNetParams.StakeVersionInterval,
			multiplier: 10000,
		},
		{
			name:       "negative mainnet params",
			skip:       chaincfg.MainNetParams.StakeValidationHeight,
			interval:   chaincfg.MainNetParams.StakeVersionInterval,
			multiplier: 1000,
			negative:   1,
		},
	}

	for _, test := range tests {
		t.Logf("running: %v skip: %v interval: %v",
			test.name, test.skip, test.interval)

		start := int64(test.skip + test.interval*2)
		expectedHeight := start - 1 // zero based
		x := int64(0) + test.negative
		for i := start; i < test.multiplier*test.interval; i++ {
			if x%test.interval == 0 && i != start {
				expectedHeight += test.interval
			}
			wantHeight := calcWantHeight(test.skip, test.interval, i)

			if wantHeight != expectedHeight {
				if test.negative == 0 {
					t.Fatalf("%v: i %v x %v -> wantHeight %v expectedHeight %v\n",
						test.name, i, x, wantHeight, expectedHeight)
				}
			}

			x++
		}
	}
}

// TestCalcStakeVersionCorners ensures that stake version calculation works as
// intended under various corner cases such as attempting to go back backwards.
func TestCalcStakeVersionCorners(t *testing.T) {
	params := &chaincfg.SimNetParams
	svh := params.StakeValidationHeight
	svi := params.StakeVersionInterval

	// Generate enough nodes to reach stake validation height with stake
	// versions set to 0.
	bc := newFakeChain(params)
	node := bc.bestNode
	for i := int64(1); i <= svh; i++ {
		node = newFakeNode(node, 0, 0, 0, time.Now())
		bc.bestNode = node
	}
	if node.height != svh {
		t.Fatalf("invalid height got %v expected %v", node.height, svh)
	}

	// Generate 3 intervals with v2 votes and calculated stake version.
	for i := int64(0); i < svi*3; i++ {
		sv, err := bc.calcStakeVersionByNode(node)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}

		// Set vote and stake versions.
		node = newFakeNode(node, 3, sv, 0, time.Now())
		appendFakeVotes(node, params.TicketsPerBlock, 2, 0)
		bc.bestNode = node
	}

	// Versions 0 and 2 should now be considered the majority version, but
	// v4 should not yet be considered majority.
	if !bc.isStakeMajorityVersion(0, node) {
		t.Fatalf("invalid StakeVersion expected 0 -> true")
	}
	if !bc.isStakeMajorityVersion(2, node) {
		t.Fatalf("invalid StakeVersion expected 2 -> true")
	}
	if bc.isStakeMajorityVersion(4, node) {
		t.Fatalf("invalid StakeVersion expected 4 -> false")
	}

	// Generate 3 intervals with v4 votes and calculated stake version.
	for i := int64(0); i < svi*3; i++ {
		sv, err := bc.calcStakeVersionByNode(node)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}

		// Set vote and stake versions.
		node = newFakeNode(node, 3, sv, 0, time.Now())
		appendFakeVotes(node, params.TicketsPerBlock, 4, 0)
		bc.bestNode = node
	}

	// Versions up to and including v4 should now be considered the majority
	// version, but v5 should not yet be considered majority.
	for _, version := range []uint32{0, 2, 4} {
		if !bc.isStakeMajorityVersion(version, node) {
			t.Fatalf("invalid StakeVersion expected %d -> true",
				version)
		}

	}
	if bc.isStakeMajorityVersion(5, node) {
		t.Fatalf("invalid StakeVersion expected 5 -> false")
	}

	// Generate 3 intervals with v2 votes and calculated stake version.
	for i := int64(0); i < svi*3; i++ {
		sv, err := bc.calcStakeVersionByNode(node)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}

		// Set vote and stake versions.
		node = newFakeNode(node, 3, sv, 0, time.Now())
		appendFakeVotes(node, params.TicketsPerBlock, 2, 0)
		bc.bestNode = node
	}

	// Versions up to and including v4 should still be considered the
	// majority version since even though there were multiple intervals with
	// a majority v2 votes, the stake version is not allowed to go
	// backwards.  Version 5 should still not be consider majority.
	for _, version := range []uint32{0, 2, 4} {
		if !bc.isStakeMajorityVersion(version, node) {
			t.Fatalf("invalid StakeVersion expected %d -> true",
				version)
		}

	}
	if bc.isStakeMajorityVersion(5, node) {
		t.Fatalf("invalid StakeVersion expected 5 -> false")
	}

	// Generate 2 intervals with v5 votes and calculated stake version.
	for i := int64(0); i < svi*2; i++ {
		sv, err := bc.calcStakeVersionByNode(node)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}

		// Set vote and stake versions.
		node = newFakeNode(node, 3, sv, 0, time.Now())
		appendFakeVotes(node, params.TicketsPerBlock, 5, 0)
		bc.bestNode = node
	}

	// Versions up to and including v5 should now be considered the majority
	// version, but v6 should not yet be.
	for _, version := range []uint32{0, 2, 4, 5} {
		if !bc.isStakeMajorityVersion(version, node) {
			t.Fatalf("invalid StakeVersion expected %d -> true",
				version)
		}

	}
	if bc.isStakeMajorityVersion(6, node) {
		t.Fatalf("invalid StakeVersion expected 6 -> false")
	}

	// Generate 1 interval with v4 votes to test the edge condition.
	for i := int64(0); i < svi; i++ {
		sv, err := bc.calcStakeVersionByNode(node)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}

		// Set vote and stake versions.
		node = newFakeNode(node, 3, sv, 0, time.Now())
		appendFakeVotes(node, params.TicketsPerBlock, 4, 0)
		bc.bestNode = node

	}

	// Versions up to and including v5 should still be considered the
	// majority version since even though there was an interval with a
	// majority v4 votes, the stake version is not allowed to go backwards.
	// Version 6 should still not be consider majority.
	for _, version := range []uint32{0, 2, 4, 5} {
		if !bc.isStakeMajorityVersion(version, node) {
			t.Fatalf("invalid StakeVersion expected %d -> true",
				version)
		}

	}
	if bc.isStakeMajorityVersion(6, node) {
		t.Fatalf("invalid StakeVersion expected 6 -> false")
	}

	// Generate another interval with v4 votes.
	for i := int64(0); i < svi; i++ {
		sv, err := bc.calcStakeVersionByNode(node)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}

		// Set stake versions.
		node = newFakeNode(node, 3, sv, 0, time.Now())
		appendFakeVotes(node, params.TicketsPerBlock, 4, 0)
		bc.bestNode = node

	}

	// Versions up to and including v5 should still be considered the
	// majority version since even though there was another interval with a
	// majority v4 votes, the stake version is not allowed to go backwards.
	// Version 6 should still not be consider majority.
	for _, version := range []uint32{0, 2, 4, 5} {
		if !bc.isStakeMajorityVersion(version, node) {
			t.Fatalf("invalid StakeVersion expected %d -> true",
				version)
		}

	}
	if bc.isStakeMajorityVersion(6, node) {
		t.Fatalf("invalid StakeVersion expected 6 -> false")
	}
}

// TestCalcStakeVersionByNode ensures that stake version calculation works as
// intended when
func TestCalcStakeVersionByNode(t *testing.T) {
	params := &chaincfg.SimNetParams
	svh := params.StakeValidationHeight
	svi := params.StakeVersionInterval
	tpb := params.TicketsPerBlock

	tests := []struct {
		name          string
		numNodes      int64
		expectVersion uint32
		set           func(*blockNode)
	}{
		{
			name:          "headerStake 2 votes 3",
			numNodes:      svh + svi*3,
			expectVersion: 3,
			set: func(node *blockNode) {
				if int64(node.height) > svh {
					appendFakeVotes(node, tpb, 3, 0)
					node.stakeVersion = 2
					node.blockVersion = 3
				}
			},
		},
		{
			name:          "headerStake 3 votes 2",
			numNodes:      svh + svi*3,
			expectVersion: 3,
			set: func(node *blockNode) {
				if int64(node.height) > svh {
					appendFakeVotes(node, tpb, 2, 0)
					node.stakeVersion = 3
					node.blockVersion = 3
				}
			},
		},
	}

	for _, test := range tests {
		bc := newFakeChain(params)
		node := bc.bestNode

		for i := int64(1); i <= test.numNodes; i++ {
			node = newFakeNode(node, 1, 0, 0, time.Now())
			test.set(node)
			bc.bestNode = node
		}

		version, err := bc.calcStakeVersionByNode(bc.bestNode)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v",
				err)
		}
		if version != test.expectVersion {
			t.Fatalf("version mismatch: got %v expected %v",
				version, test.expectVersion)
		}
	}
}

// TestIsStakeMajorityVersion ensures that determining the current majority
// stake version works as intended under a wide variety of scenarios.
func TestIsStakeMajorityVersion(t *testing.T) {
	params := &chaincfg.MainNetParams
	svh := params.StakeValidationHeight
	svi := params.StakeVersionInterval
	tpb := params.TicketsPerBlock

	// Calculate super majority for 5 and 3 ticket maxes.
	maxTickets5 := int32(svi) * int32(tpb)
	sm5 := maxTickets5 * params.StakeMajorityMultiplier / params.StakeMajorityDivisor
	maxTickets3 := int32(svi) * int32(tpb-2)
	sm3 := maxTickets3 * params.StakeMajorityMultiplier / params.StakeMajorityDivisor

	// Keep track of ticketcount in set.  Must be reset every test.
	ticketCount := int32(0)

	tests := []struct {
		name                 string
		numNodes             int64
		set                  func(*blockNode)
		blockVersion         int32
		startStakeVersion    uint32
		expectedStakeVersion uint32
		expectedCalcVersion  uint32
		result               bool
	}{
		{
			name:                 "too shallow",
			numNodes:             svh + svi - 1,
			startStakeVersion:    1,
			expectedStakeVersion: 1,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:                 "just enough",
			numNodes:             svh + svi,
			startStakeVersion:    1,
			expectedStakeVersion: 1,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:                 "odd",
			numNodes:             svh + svi + 1,
			startStakeVersion:    1,
			expectedStakeVersion: 1,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:     "100%",
			numNodes: svh + svi,
			set: func(node *blockNode) {
				if int64(node.height) > svh {
					appendFakeVotes(node, tpb, 2, 0)
				}
			},
			startStakeVersion:    1,
			expectedStakeVersion: 2,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:     "50%",
			numNodes: svh + (svi * 2),
			set: func(node *blockNode) {
				if int64(node.height) <= svh {
					return
				}

				if int64(node.height) < svh+svi {
					appendFakeVotes(node, tpb, 1, 0)
					return
				}

				threshold := maxTickets5 / 2

				v := uint32(1)
				for i := 0; i < int(tpb); i++ {
					if ticketCount >= threshold {
						v = 2
					}
					appendFakeVotes(node, 1, v, 0)
					ticketCount++
				}
			},
			startStakeVersion:    1,
			expectedStakeVersion: 2,
			expectedCalcVersion:  0,
			result:               false,
		},
		{
			name:     "75%-1",
			numNodes: svh + (svi * 2),
			set: func(node *blockNode) {
				if int64(node.height) < svh {
					return
				}

				if int64(node.height) < svh+svi {
					appendFakeVotes(node, tpb, 1, 0)
					return
				}

				threshold := maxTickets5 - sm5 + 1

				v := uint32(1)
				for i := 0; i < int(tpb); i++ {
					if ticketCount >= threshold {
						v = 2
					}
					appendFakeVotes(node, 1, v, 0)
					ticketCount++
				}
			},
			startStakeVersion:    1,
			expectedStakeVersion: 2,
			expectedCalcVersion:  0,
			result:               false,
		},
		{
			name:     "75%",
			numNodes: svh + (svi * 2),
			set: func(node *blockNode) {
				if int64(node.height) <= svh {
					return
				}

				if int64(node.height) < svh+svi {
					appendFakeVotes(node, tpb, 1, 0)
					return
				}

				threshold := maxTickets5 - sm5

				v := uint32(1)
				for i := 0; i < int(tpb); i++ {
					if ticketCount >= threshold {
						v = 2
					}
					appendFakeVotes(node, 1, v, 0)
					ticketCount++
				}
			},
			startStakeVersion:    1,
			expectedStakeVersion: 2,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:     "100% after several non majority intervals",
			numNodes: svh + (params.StakeVersionInterval * 222),
			set: func(node *blockNode) {
				if int64(node.height) <= svh {
					return
				}

				if int64(node.height) < svh+svi {
					appendFakeVotes(node, tpb, 1, 0)
					return
				}

				for i := uint32(0); i < uint32(tpb); i++ {
					appendFakeVotes(node, 1, i%5, 0)
				}
			},
			startStakeVersion:    1,
			expectedStakeVersion: 1,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:     "no majority ever",
			numNodes: svh + (svi * 8),
			set: func(node *blockNode) {
				if int64(node.height) <= svh {
					return
				}

				for i := uint32(0); i < uint32(tpb); i++ {
					appendFakeVotes(node, 1, i%5, 0)
				}
			},
			startStakeVersion:    1,
			expectedStakeVersion: 1,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:     "75%-1 with 3 votes",
			numNodes: svh + (svi * 2),
			set: func(node *blockNode) {
				if int64(node.height) < svh {
					return
				}

				if int64(node.height) < svh+svi {
					appendFakeVotes(node, tpb-2, 1, 0)
					return
				}

				threshold := maxTickets3 - sm3 + 1

				v := uint32(1)
				for i := 0; i < int(tpb-2); i++ {
					if ticketCount >= threshold {
						v = 2
					}
					appendFakeVotes(node, 1, v, 0)
					ticketCount++
				}
			},
			startStakeVersion:    1,
			expectedStakeVersion: 2,
			expectedCalcVersion:  0,
			result:               false,
		},
		{
			name:     "75% with 3 votes",
			numNodes: svh + (svi * 2),
			set: func(node *blockNode) {
				if int64(node.height) <= svh {
					return
				}

				if int64(node.height) < svh+svi {
					appendFakeVotes(node, tpb-2, 1, 0)
					return
				}

				threshold := maxTickets3 - sm3

				v := uint32(1)
				for i := 0; i < int(tpb-2); i++ {
					if ticketCount >= threshold {
						v = 2
					}
					appendFakeVotes(node, 1, v, 0)
					ticketCount++
				}
			},
			startStakeVersion:    1,
			expectedStakeVersion: 2,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:     "75% with 3 votes blockversion 3",
			numNodes: svh + (svi * 2),
			set: func(node *blockNode) {
				if int64(node.height) <= svh {
					return
				}

				if int64(node.height) < svh+svi {
					appendFakeVotes(node, tpb-2, 1, 0)
					return
				}

				threshold := maxTickets3 - sm3

				v := uint32(1)
				for i := 0; i < int(tpb-2); i++ {
					if ticketCount >= threshold {
						v = 2
					}
					appendFakeVotes(node, 1, v, 0)
					ticketCount++
				}
			},
			blockVersion:         3,
			startStakeVersion:    1,
			expectedStakeVersion: 2,
			expectedCalcVersion:  2,
			result:               true,
		},
		{
			name:     "75%-1 with 3 votes blockversion 3",
			numNodes: svh + (svi * 2),
			set: func(node *blockNode) {
				if int64(node.height) < svh {
					return
				}

				if int64(node.height) < svh+svi {
					appendFakeVotes(node, tpb-2, 1, 0)
					return
				}

				threshold := maxTickets3 - sm3 + 1

				v := uint32(1)
				for i := 0; i < int(tpb-2); i++ {
					if ticketCount >= threshold {
						v = 2
					}
					appendFakeVotes(node, 1, v, 0)
					ticketCount++
				}
			},
			blockVersion:         3,
			startStakeVersion:    1,
			expectedStakeVersion: 2,
			expectedCalcVersion:  1,
			result:               false,
		},
	}

	for _, test := range tests {
		// Create new BlockChain in order to blow away cache.
		bc := newFakeChain(params)
		node := bc.bestNode
		node.stakeVersion = test.startStakeVersion

		ticketCount = 0

		for i := int64(1); i <= test.numNodes; i++ {
			node = newFakeNode(node, test.blockVersion,
				test.startStakeVersion, 0, time.Now())

			// Override version.
			if test.set != nil {
				test.set(node)
			} else {
				appendFakeVotes(node, tpb,
					test.startStakeVersion, 0)
			}

			bc.bestNode = node
		}

		res := bc.isVoterMajorityVersion(test.expectedStakeVersion, node)
		if res != test.result {
			t.Fatalf("%v isVoterMajorityVersion", test.name)
		}

		// validate calcStakeVersion
		version, err := bc.calcStakeVersionByNode(node)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}
		if version != test.expectedCalcVersion {
			t.Fatalf("%v calcStakeVersionByNode got %v expected %v",
				test.name, version, test.expectedCalcVersion)
		}
	}
}

func TestLarge(t *testing.T) {
	params := &chaincfg.MainNetParams

	numRuns := 5
	numBlocks := params.StakeVersionInterval * 100
	numBlocksShallow := params.StakeVersionInterval * 10
	tests := []struct {
		name                 string
		numNodes             int64
		blockVersion         int32
		startStakeVersion    uint32
		expectedStakeVersion uint32
		expectedCalcVersion  uint32
		result               bool
	}{
		{
			name:                 "shallow cache",
			numNodes:             numBlocksShallow,
			startStakeVersion:    1,
			expectedStakeVersion: 1,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:                 "deep cache",
			numNodes:             numBlocks,
			startStakeVersion:    1,
			expectedStakeVersion: 1,
			expectedCalcVersion:  0,
			result:               true,
		},
	}

	for _, test := range tests {
		// Create new BlockChain in order to blow away cache.
		bc := newFakeChain(params)
		node := bc.bestNode
		node.stakeVersion = test.startStakeVersion

		for i := int64(1); i <= test.numNodes; i++ {
			node = newFakeNode(node, test.blockVersion,
				test.startStakeVersion, 0, time.Now())

			// Override version.
			appendFakeVotes(node, params.TicketsPerBlock,
				test.startStakeVersion, 0)
			bc.bestNode = node
		}

		for x := 0; x < numRuns; x++ {
			start := time.Now()
			res := bc.isVoterMajorityVersion(test.expectedStakeVersion, node)
			if res != test.result {
				t.Fatalf("%v isVoterMajorityVersion got %v expected %v", test.name, res, test.result)
			}

			// validate calcStakeVersion
			version, err := bc.calcStakeVersionByNode(node)
			if err != nil {
				t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
			}
			if version != test.expectedCalcVersion {
				t.Fatalf("%v calcStakeVersionByNode got %v expected %v",
					test.name, version, test.expectedCalcVersion)
			}
			end := time.Now()

			setup := "setup 0"
			if x != 0 {
				setup = fmt.Sprintf("run %v", x)
			}

			vkey := stakeMajorityCacheKeySize + 8 // bool on x86_64
			key := chainhash.HashSize + 4         // size of uint32

			cost := len(bc.isVoterMajorityVersionCache) * vkey
			cost += len(bc.isStakeMajorityVersionCache) * vkey
			cost += len(bc.calcPriorStakeVersionCache) * key
			cost += len(bc.calcVoterVersionIntervalCache) * key
			cost += len(bc.calcStakeVersionCache) * key
			memoryCost := fmt.Sprintf("memory cost: %v", cost)

			t.Logf("run time (%v) %v %v", setup, end.Sub(start),
				memoryCost)
		}
	}
}
