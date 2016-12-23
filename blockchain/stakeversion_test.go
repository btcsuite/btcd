// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

// genesisBlockNode creates a fake chain of blockNodes.  It is used for testing
// the mechanical properties of the version code.
func genesisBlockNode(params *chaincfg.Params) *blockNode {
	// Create a new node from the genesis block.
	genesisBlock := dcrutil.NewBlock(params.GenesisBlock)
	header := &genesisBlock.MsgBlock().Header
	node := newBlockNode(header, genesisBlock.Hash(), 0, []chainhash.Hash{},
		[]chainhash.Hash{}, []uint32{})
	node.inMainChain = true

	return node
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
			name:       "testnet params",
			skip:       chaincfg.TestNetParams.StakeValidationHeight,
			interval:   chaincfg.TestNetParams.StakeVersionInterval,
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

func newFakeNode(blockVersion int32, height int64, currentNode *blockNode) *blockNode {
	// Make up a header.
	header := &wire.BlockHeader{
		Version: blockVersion,
		Height:  uint32(height),
		Nonce:   0,
	}
	node := newBlockNode(header, &chainhash.Hash{}, 0,
		[]chainhash.Hash{}, []chainhash.Hash{},
		[]uint32{})
	node.height = height
	node.parent = currentNode

	return node
}

func TestCalcStakeVersionCorners(t *testing.T) {
	params := &chaincfg.SimNetParams
	currentNode := genesisBlockNode(params)

	bc := &BlockChain{
		chainParams: params,
	}

	svh := params.StakeValidationHeight
	interval := params.StakeVersionInterval

	height := int64(0)
	for i := int64(1); i <= svh; i++ {
		node := newFakeNode(0, i, currentNode)

		// Don't set stake versions.

		currentNode = node
		bc.bestNode = currentNode
		height = i
	}
	if height != svh {
		t.Fatalf("invalid height got %v expected %v", height,
			params.StakeValidationHeight)
	}

	// Generate 3 intervals with v2 votes and calculate StakeVersion.
	runCount := interval * 3
	for i := int64(0); i < runCount; i++ {
		node := newFakeNode(3, height+i, currentNode)

		// Set stake versions.
		for x := uint16(0); x < params.TicketsPerBlock; x++ {
			node.voterVersions = append(node.voterVersions, 2)
		}

		sv, err := bc.calcStakeVersionByNode(currentNode)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}
		node.header.StakeVersion = sv

		currentNode = node
		bc.bestNode = currentNode

	}
	height += runCount

	if !bc.isStakeMajorityVersion(0, currentNode) {
		t.Fatalf("invalid StakeVersion expected 0 -> true")
	}
	if !bc.isStakeMajorityVersion(2, currentNode) {
		t.Fatalf("invalid StakeVersion expected 2 -> true")
	}
	if bc.isStakeMajorityVersion(4, currentNode) {
		t.Fatalf("invalid StakeVersion expected 4 -> false")
	}

	// Generate 3 intervals with v4 votes and calculate StakeVersion.
	runCount = interval * 3
	for i := int64(0); i < runCount; i++ {
		node := newFakeNode(3, height+i, currentNode)

		// Set stake versions.
		for x := uint16(0); x < params.TicketsPerBlock; x++ {
			node.voterVersions = append(node.voterVersions, 4)
		}

		sv, err := bc.calcStakeVersionByNode(currentNode)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}
		node.header.StakeVersion = sv

		currentNode = node
		bc.bestNode = currentNode
	}
	height += runCount

	if !bc.isStakeMajorityVersion(0, currentNode) {
		t.Fatalf("invalid StakeVersion expected 0 -> true")
	}
	if !bc.isStakeMajorityVersion(2, currentNode) {
		t.Fatalf("invalid StakeVersion expected 2 -> true")
	}
	if !bc.isStakeMajorityVersion(4, currentNode) {
		t.Fatalf("invalid StakeVersion expected 4 -> true")
	}
	if bc.isStakeMajorityVersion(5, currentNode) {
		t.Fatalf("invalid StakeVersion expected 5 -> false")
	}

	// Generate 3 intervals with v2 votes and calculate StakeVersion.
	runCount = interval * 3
	for i := int64(0); i < runCount; i++ {
		node := newFakeNode(3, height+i, currentNode)

		// Set stake versions.
		for x := uint16(0); x < params.TicketsPerBlock; x++ {
			node.voterVersions = append(node.voterVersions, 2)
		}

		sv, err := bc.calcStakeVersionByNode(currentNode)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}
		node.header.StakeVersion = sv

		currentNode = node
		bc.bestNode = currentNode

	}
	height += runCount

	if !bc.isStakeMajorityVersion(0, currentNode) {
		t.Fatalf("invalid StakeVersion expected 0 -> true")
	}
	if !bc.isStakeMajorityVersion(2, currentNode) {
		t.Fatalf("invalid StakeVersion expected 2 -> true")
	}
	if !bc.isStakeMajorityVersion(4, currentNode) {
		t.Fatalf("invalid StakeVersion expected 4 -> true")
	}
	if bc.isStakeMajorityVersion(5, currentNode) {
		t.Fatalf("invalid StakeVersion expected 5 -> false")
	}

	// Generate 2 interval with v5 votes
	runCount = interval * 2
	for i := int64(0); i < runCount; i++ {
		node := newFakeNode(3, height+i, currentNode)

		// Set stake versions.
		for x := uint16(0); x < params.TicketsPerBlock; x++ {
			node.voterVersions = append(node.voterVersions, 5)
		}

		sv, err := bc.calcStakeVersionByNode(currentNode)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}
		node.header.StakeVersion = sv

		currentNode = node
		bc.bestNode = currentNode

	}
	height += runCount

	if !bc.isStakeMajorityVersion(0, currentNode) {
		t.Fatalf("invalid StakeVersion expected 0 -> true")
	}
	if !bc.isStakeMajorityVersion(2, currentNode) {
		t.Fatalf("invalid StakeVersion expected 2 -> true")
	}
	if !bc.isStakeMajorityVersion(4, currentNode) {
		t.Fatalf("invalid StakeVersion expected 4 -> true")
	}
	if !bc.isStakeMajorityVersion(5, currentNode) {
		t.Fatalf("invalid StakeVersion expected 5 -> true")
	}
	if bc.isStakeMajorityVersion(6, currentNode) {
		t.Fatalf("invalid StakeVersion expected 6 -> false")
	}

	// Generate 1 interval with v4 votes, to test the edge condition
	runCount = interval
	for i := int64(0); i < runCount; i++ {
		node := newFakeNode(3, height+i, currentNode)

		// Set stake versions.
		for x := uint16(0); x < params.TicketsPerBlock; x++ {
			node.voterVersions = append(node.voterVersions, 4)
		}

		sv, err := bc.calcStakeVersionByNode(currentNode)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}
		node.header.StakeVersion = sv

		currentNode = node
		bc.bestNode = currentNode

	}
	height += runCount

	if !bc.isStakeMajorityVersion(0, currentNode) {
		t.Fatalf("invalid StakeVersion expected 0 -> true")
	}
	if !bc.isStakeMajorityVersion(2, currentNode) {
		t.Fatalf("invalid StakeVersion expected 2 -> true")
	}
	if !bc.isStakeMajorityVersion(4, currentNode) {
		t.Fatalf("invalid StakeVersion expected 4 -> true")
	}
	if !bc.isStakeMajorityVersion(5, currentNode) {
		t.Fatalf("invalid StakeVersion expected 5 -> true")
	}
	if bc.isStakeMajorityVersion(6, currentNode) {
		t.Fatalf("invalid StakeVersion expected 6 -> false")
	}

	// Generate 1 interval with v4 votes.
	runCount = interval
	for i := int64(0); i < runCount; i++ {
		node := newFakeNode(3, height+i, currentNode)

		// Set stake versions.
		for x := uint16(0); x < params.TicketsPerBlock; x++ {
			node.voterVersions = append(node.voterVersions, 4)
		}

		sv, err := bc.calcStakeVersionByNode(currentNode)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}
		node.header.StakeVersion = sv

		currentNode = node
		bc.bestNode = currentNode

	}
	height += runCount

	if !bc.isStakeMajorityVersion(0, currentNode) {
		t.Fatalf("invalid StakeVersion expected 0 -> true")
	}
	if !bc.isStakeMajorityVersion(2, currentNode) {
		t.Fatalf("invalid StakeVersion expected 2 -> true")
	}
	if !bc.isStakeMajorityVersion(4, currentNode) {
		t.Fatalf("invalid StakeVersion expected 4 -> true")
	}
	if !bc.isStakeMajorityVersion(5, currentNode) {
		t.Fatalf("invalid StakeVersion expected 5 -> true")
	}
	if bc.isStakeMajorityVersion(6, currentNode) {
		t.Fatalf("invalid StakeVersion expected 6 -> false")
	}
}

func TestCalcStakeVersionByNode(t *testing.T) {
	params := &chaincfg.SimNetParams

	tests := []struct {
		name          string
		numNodes      int64
		expectVersion uint32
		set           func(*blockNode)
	}{
		{
			name:          "headerStake 2 votes 3",
			numNodes:      params.StakeValidationHeight + params.StakeVersionInterval*3,
			expectVersion: 3,
			set: func(b *blockNode) {
				if int64(b.header.Height) > params.StakeValidationHeight {
					// set voter versions
					for x := 0; x < int(params.TicketsPerBlock); x++ {
						b.voterVersions = append(b.voterVersions, 3)
					}

					// set header stake version
					b.header.StakeVersion = 2
					// set enforcement version
					b.header.Version = 3
				}
			},
		},
		{
			name:          "headerStake 3 votes 2",
			numNodes:      params.StakeValidationHeight + params.StakeVersionInterval*3,
			expectVersion: 3,
			set: func(b *blockNode) {
				if int64(b.header.Height) > params.StakeValidationHeight {
					// set voter versions
					for x := 0; x < int(params.TicketsPerBlock); x++ {
						b.voterVersions = append(b.voterVersions, 2)
					}

					// set header stake version
					b.header.StakeVersion = 3
					// set enforcement version
					b.header.Version = 3
				}
			},
		},
	}

	bc := &BlockChain{
		chainParams: params,
	}
	for _, test := range tests {
		currentNode := genesisBlockNode(params)

		t.Logf("running: \"%v\"\n", test.name)
		for i := int64(1); i <= test.numNodes; i++ {
			// Make up a header.
			header := &wire.BlockHeader{
				Version: 1,
				Height:  uint32(i),
				Nonce:   uint32(0),
			}
			node := newBlockNode(header, &chainhash.Hash{}, 0,
				[]chainhash.Hash{}, []chainhash.Hash{},
				[]uint32{})
			node.height = i
			node.parent = currentNode

			test.set(node)

			currentNode = node
			bc.bestNode = currentNode
		}

		version, err := bc.calcStakeVersionByNode(bc.bestNode)
		t.Logf("name \"%v\" version %v err %v", test.name, version, err)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}
		if version != test.expectVersion {
			t.Fatalf("version mismatch: got %v expected %v",
				version, test.expectVersion)
		}
	}
}

func TestIsStakeMajorityVersion(t *testing.T) {
	params := &chaincfg.MainNetParams

	// Calculate super majority for 5 and 3 ticket maxes.
	maxTickets5 := int32(params.StakeVersionInterval) * int32(params.TicketsPerBlock)
	sm5 := maxTickets5 * params.StakeMajorityMultiplier / params.StakeMajorityDivisor
	maxTickets3 := int32(params.StakeVersionInterval) * int32(params.TicketsPerBlock-2)
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
			numNodes:             params.StakeValidationHeight + params.StakeVersionInterval - 1,
			startStakeVersion:    1,
			expectedStakeVersion: 1,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:                 "just enough",
			numNodes:             params.StakeValidationHeight + params.StakeVersionInterval,
			startStakeVersion:    1,
			expectedStakeVersion: 1,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:                 "odd",
			numNodes:             params.StakeValidationHeight + params.StakeVersionInterval + 1,
			startStakeVersion:    1,
			expectedStakeVersion: 1,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:     "100%",
			numNodes: params.StakeValidationHeight + params.StakeVersionInterval,
			set: func(b *blockNode) {
				if int64(b.header.Height) > params.StakeValidationHeight {
					for x := 0; x < int(params.TicketsPerBlock); x++ {
						b.voterVersions = append(b.voterVersions, 2)
					}
				}
			},
			startStakeVersion:    1,
			expectedStakeVersion: 2,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:     "50%",
			numNodes: params.StakeValidationHeight + (params.StakeVersionInterval * 2),
			set: func(b *blockNode) {
				if int64(b.header.Height) <= params.StakeValidationHeight {
					return
				}

				if int64(b.header.Height) < params.StakeValidationHeight+params.StakeVersionInterval {
					for x := 0; x < int(params.TicketsPerBlock); x++ {
						b.voterVersions = append(b.voterVersions, uint32(1))
					}
					return
				}

				threshold := maxTickets5 / 2

				v := uint32(1)
				for x := 0; x < int(params.TicketsPerBlock); x++ {
					if ticketCount >= threshold {
						v = 2
					}
					b.voterVersions = append(b.voterVersions, v)
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
			numNodes: params.StakeValidationHeight + (params.StakeVersionInterval * 2),
			set: func(b *blockNode) {
				if int64(b.header.Height) < params.StakeValidationHeight {
					return
				}

				if int64(b.header.Height) < params.StakeValidationHeight+params.StakeVersionInterval {
					for x := 0; x < int(params.TicketsPerBlock); x++ {
						b.voterVersions = append(b.voterVersions, uint32(1))
					}
					return
				}

				threshold := maxTickets5 - sm5 + 1

				v := uint32(1)
				for x := 0; x < int(params.TicketsPerBlock); x++ {
					if ticketCount >= threshold {
						v = 2
					}
					b.voterVersions = append(b.voterVersions, v)
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
			numNodes: params.StakeValidationHeight + (params.StakeVersionInterval * 2),
			set: func(b *blockNode) {
				if int64(b.header.Height) <= params.StakeValidationHeight {
					return
				}

				if int64(b.header.Height) < params.StakeValidationHeight+params.StakeVersionInterval {
					for x := 0; x < int(params.TicketsPerBlock); x++ {
						b.voterVersions = append(b.voterVersions, uint32(1))
					}
					return
				}

				threshold := maxTickets5 - sm5

				v := uint32(1)
				for x := 0; x < int(params.TicketsPerBlock); x++ {
					if ticketCount >= threshold {
						v = 2
					}
					b.voterVersions = append(b.voterVersions, v)
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
			numNodes: params.StakeValidationHeight + (params.StakeVersionInterval * 222),
			set: func(b *blockNode) {
				if int64(b.header.Height) <= params.StakeValidationHeight {
					return
				}

				if int64(b.header.Height) < params.StakeValidationHeight+params.StakeVersionInterval {
					for x := 0; x < int(params.TicketsPerBlock); x++ {
						b.voterVersions = append(b.voterVersions, uint32(1))
					}
					return
				}

				for x := 0; x < int(params.TicketsPerBlock); x++ {
					b.voterVersions = append(b.voterVersions, uint32(x)%5)
				}
			},
			startStakeVersion:    1,
			expectedStakeVersion: 1,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:     "no majority ever",
			numNodes: params.StakeValidationHeight + (params.StakeVersionInterval * 8),
			set: func(b *blockNode) {
				if int64(b.header.Height) <= params.StakeValidationHeight {
					return
				}

				for x := 0; x < int(params.TicketsPerBlock); x++ {
					b.voterVersions = append(b.voterVersions, uint32(x)%5)
				}
			},
			startStakeVersion:    1,
			expectedStakeVersion: 1,
			expectedCalcVersion:  0,
			result:               true,
		},
		{
			name:     "75%-1 with 3 votes",
			numNodes: params.StakeValidationHeight + (params.StakeVersionInterval * 2),
			set: func(b *blockNode) {
				if int64(b.header.Height) < params.StakeValidationHeight {
					return
				}

				if int64(b.header.Height) < params.StakeValidationHeight+params.StakeVersionInterval {
					for x := 0; x < int(params.TicketsPerBlock-2); x++ {
						b.voterVersions = append(b.voterVersions, uint32(1))
					}
					return
				}

				threshold := maxTickets3 - sm3 + 1

				v := uint32(1)
				for x := 0; x < int(params.TicketsPerBlock-2); x++ {
					if ticketCount >= threshold {
						v = 2
					}
					b.voterVersions = append(b.voterVersions, v)
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
			numNodes: params.StakeValidationHeight + (params.StakeVersionInterval * 2),
			set: func(b *blockNode) {
				if int64(b.header.Height) <= params.StakeValidationHeight {
					return
				}

				if int64(b.header.Height) < params.StakeValidationHeight+params.StakeVersionInterval {
					for x := 0; x < int(params.TicketsPerBlock-2); x++ {
						b.voterVersions = append(b.voterVersions, uint32(1))
					}
					return
				}

				threshold := maxTickets3 - sm3

				v := uint32(1)
				for x := 0; x < int(params.TicketsPerBlock-2); x++ {
					if ticketCount >= threshold {
						v = 2
					}
					b.voterVersions = append(b.voterVersions, v)
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
			numNodes: params.StakeValidationHeight + (params.StakeVersionInterval * 2),
			set: func(b *blockNode) {
				if int64(b.header.Height) <= params.StakeValidationHeight {
					return
				}

				if int64(b.header.Height) < params.StakeValidationHeight+params.StakeVersionInterval {
					for x := 0; x < int(params.TicketsPerBlock-2); x++ {
						b.voterVersions = append(b.voterVersions, uint32(1))
					}
					return
				}

				threshold := maxTickets3 - sm3

				v := uint32(1)
				for x := 0; x < int(params.TicketsPerBlock-2); x++ {
					if ticketCount >= threshold {
						v = 2
					}
					b.voterVersions = append(b.voterVersions, v)
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
			numNodes: params.StakeValidationHeight + (params.StakeVersionInterval * 2),
			set: func(b *blockNode) {
				if int64(b.header.Height) < params.StakeValidationHeight {
					return
				}

				if int64(b.header.Height) < params.StakeValidationHeight+params.StakeVersionInterval {
					for x := 0; x < int(params.TicketsPerBlock-2); x++ {
						b.voterVersions = append(b.voterVersions, uint32(1))
					}
					return
				}

				threshold := maxTickets3 - sm3 + 1

				v := uint32(1)
				for x := 0; x < int(params.TicketsPerBlock-2); x++ {
					if ticketCount >= threshold {
						v = 2
					}
					b.voterVersions = append(b.voterVersions, v)
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

	bc := &BlockChain{
		chainParams: params,
	}
	for _, test := range tests {
		ticketCount = 0

		genesisNode := genesisBlockNode(params)
		genesisNode.header.StakeVersion = test.startStakeVersion

		t.Logf("running: %v\n", test.name)
		var currentNode *blockNode
		currentNode = genesisNode
		for i := int64(1); i <= test.numNodes; i++ {
			// Make up a header.
			header := &wire.BlockHeader{
				Version:      test.blockVersion,
				Height:       uint32(i),
				Nonce:        uint32(0),
				StakeVersion: test.startStakeVersion,
			}
			node := newBlockNode(header, &chainhash.Hash{}, 0,
				[]chainhash.Hash{}, []chainhash.Hash{},
				[]uint32{})
			node.height = i
			node.parent = currentNode

			// Override version.
			if test.set != nil {
				test.set(node)
			} else {
				for x := 0; x < int(params.TicketsPerBlock); x++ {
					node.voterVersions = append(node.voterVersions, test.startStakeVersion)
				}
			}

			currentNode = node
			bc.bestNode = currentNode
		}

		res := bc.isVoterMajorityVersion(test.expectedStakeVersion, currentNode)
		if res != test.result {
			t.Fatalf("%v isVoterMajorityVersion", test.name)
		}

		// validate calcStakeVersion
		version, err := bc.calcStakeVersionByNode(currentNode)
		if err != nil {
			t.Fatalf("calcStakeVersionByNode: unexpected error: %v", err)
		}
		if version != test.expectedCalcVersion {
			t.Fatalf("%v calcStakeVersionByNode got %v expected %v",
				test.name, version, test.expectedCalcVersion)
		}
	}
}
