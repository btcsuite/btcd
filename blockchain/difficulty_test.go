// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math/big"
	"runtime"
	"testing"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/wire"
)

func TestBigToCompact(t *testing.T) {
	tests := []struct {
		in  int64
		out uint32
	}{
		{0, 0},
		{-1, 25231360},
	}

	for x, test := range tests {
		n := big.NewInt(test.in)
		r := BigToCompact(n)
		if r != test.out {
			t.Errorf("TestBigToCompact test #%d failed: got %d want %d\n",
				x, r, test.out)
			return
		}
	}
}

func TestCompactToBig(t *testing.T) {
	tests := []struct {
		in  uint32
		out int64
	}{
		{10000000, 0},
	}

	for x, test := range tests {
		n := CompactToBig(test.in)
		want := big.NewInt(test.out)
		if n.Cmp(want) != 0 {
			t.Errorf("TestCompactToBig test #%d failed: got %d want %d\n",
				x, n.Int64(), want.Int64())
			return
		}
	}
}

func TestCalcWork(t *testing.T) {
	tests := []struct {
		in  uint32
		out int64
	}{
		{10000000, 0},
	}

	for x, test := range tests {
		bits := uint32(test.in)

		r := CalcWork(bits)
		if r.Int64() != test.out {
			t.Errorf("TestCalcWork test #%d failed: got %v want %d\n",
				x, r.Int64(), test.out)
			return
		}
	}
}

// TestEstimateSupply ensures the supply estimation function used in the stake
// difficulty algorithm defined by DCP0001 works as expected.
func TestEstimateSupply(t *testing.T) {
	t.Parallel()

	// The parameters used for the supply estimation.
	params := &chaincfg.MainNetParams
	baseSubsidy := params.BaseSubsidy
	reduxInterval := params.SubsidyReductionInterval
	blockOneSubsidy := params.BlockOneSubsidy()

	// intervalSubsidy is a helper function to return the full block subsidy
	// for the given reduction interval.
	intervalSubsidy := func(interval int) int64 {
		subsidy := baseSubsidy
		for i := 0; i < interval; i++ {
			subsidy *= params.MulSubsidy
			subsidy /= params.DivSubsidy
		}
		return subsidy
	}

	// Useful calculations for the tests below.
	intervalOneSubsidy := intervalSubsidy(1)
	intervalTwoSubsidy := intervalSubsidy(2)
	reduxIntervalMinusOneSupply := blockOneSubsidy + (baseSubsidy * (reduxInterval - 2))
	reduxIntervalTwoMinusOneSupply := reduxIntervalMinusOneSupply + (intervalOneSubsidy * reduxInterval)

	tests := []struct {
		height   int64
		expected int64
	}{
		{height: -1, expected: 0},
		{height: 0, expected: 0},
		{height: 1, expected: blockOneSubsidy},
		{height: 2, expected: blockOneSubsidy + baseSubsidy},
		{height: 3, expected: blockOneSubsidy + baseSubsidy*2},
		{height: reduxInterval - 1, expected: reduxIntervalMinusOneSupply},
		{height: reduxInterval, expected: reduxIntervalMinusOneSupply + intervalOneSubsidy},
		{height: reduxInterval + 1, expected: reduxIntervalMinusOneSupply + intervalOneSubsidy*2},
		{height: reduxInterval*2 - 1, expected: reduxIntervalTwoMinusOneSupply},
		{height: reduxInterval * 2, expected: reduxIntervalTwoMinusOneSupply + intervalTwoSubsidy},
		{height: reduxInterval*2 + 1, expected: reduxIntervalTwoMinusOneSupply + intervalTwoSubsidy*2},
	}

	for _, test := range tests {
		// Ensure the function to calculate the estimated supply is
		// working properly.
		gotSupply := estimateSupply(params, test.height)
		if gotSupply != test.expected {
			t.Errorf("estimateSupply (height %d): did not get "+
				"expected supply - got %d, want %d", test.height,
				gotSupply, test.expected)
			continue
		}
	}
}

// assertStakeDiffParams ensure the passed params have the values used in the
// tests related to stake difficulty calculation.
func assertStakeDiffParams(t *testing.T, params *chaincfg.Params) {
	if params.MinimumStakeDiff != 200000000 {
		_, file, line, _ := runtime.Caller(1)
		t.Fatalf("%s:%d -- expect params with minimum stake diff of "+
			"%d, got %d", file, line, 200000000,
			params.MinimumStakeDiff)
	}
	if params.TicketMaturity != 256 {
		_, file, line, _ := runtime.Caller(1)
		t.Fatalf("%s:%d -- expect params with ticket maturity of "+
			"%d, got %d", file, line, 256, params.TicketMaturity)
	}
	if params.StakeValidationHeight != 4096 {
		_, file, line, _ := runtime.Caller(1)
		t.Fatalf("%s:%d -- expect params with stake val height of %d, "+
			"got %d", file, line, 4096, params.StakeValidationHeight)
	}
	if params.StakeDiffWindowSize != 144 {
		_, file, line, _ := runtime.Caller(1)
		t.Fatalf("%s:%d -- expect params with stake diff interval of "+
			"%d, got %d", file, line, 144, params.StakeDiffWindowSize)
	}
	if params.TicketsPerBlock != 5 {
		_, file, line, _ := runtime.Caller(1)
		t.Fatalf("%s:%d -- expect params with tickets per block of "+
			"%d, got %d", file, line, 5, params.TicketsPerBlock)
	}
}

// TestCalcNextRequiredStakeDiffV2 ensure the stake diff calculation function
// for the algorithm defined by DCP0001 works as expected.
func TestCalcNextRequiredStakeDiffV2(t *testing.T) {
	t.Parallel()

	// ticketInfo is used to control the tests by specifying the details
	// about how many fake blocks to create with the specified number of
	// ticket and stake difficulty.
	type ticketInfo struct {
		numNodes   uint32
		newTickets uint8
		stakeDiff  int64
	}

	// Specify the params used in the tests and assert the values directly
	// used by the tests are the expected ones.  All of the test values will
	// need to be updated if these parameters change since they are manually
	// calculated based on them.
	params := &chaincfg.MainNetParams
	assertStakeDiffParams(t, params)
	minStakeDiff := params.MinimumStakeDiff
	ticketMaturity := uint32(params.TicketMaturity)
	stakeValidationHeight := params.StakeValidationHeight

	tests := []struct {
		name         string
		ticketInfo   []ticketInfo
		expectedDiff int64
	}{
		{
			// Next retarget is at 144.  Prior to coinbase maturity,
			// so will always be the minimum.
			name:         "genesis block",
			ticketInfo:   []ticketInfo{{0, 0, minStakeDiff}},
			expectedDiff: minStakeDiff,
		},
		{
			// Next retarget is at 144.  Prior to coinbase maturity,
			// so will always be the minimum.
			name:         "1st retarget, before coinbase",
			ticketInfo:   []ticketInfo{{143, 0, minStakeDiff}},
			expectedDiff: minStakeDiff,
		},
		{
			// Next retarget is at 288.
			//
			// Tickets could not possibly have been bought yet, but
			// ensure the algorithm handles it properly.
			name:         "coinbase maturity with impossible num tickets",
			ticketInfo:   []ticketInfo{{255, 20, minStakeDiff}},
			expectedDiff: minStakeDiff,
		},
		{
			// Next retarget is at 288.
			//
			// Block 0 has no spendable outputs, so tickets could
			// not have possibly been bought yet.
			name: "coinbase maturity + 1",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},
			},
			expectedDiff: minStakeDiff,
		},
		{
			// Next retarget is at 288.
			name: "2nd retarget interval - 1, 100% demand",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff}, // 256
				{30, 20, minStakeDiff}, // 286
			},
			expectedDiff: minStakeDiff,
		},
		{
			// Next retarget is at 288.
			name: "2nd retarget interval, 100% demand",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff}, // 256
				{31, 20, minStakeDiff}, // 287
			},
			expectedDiff: minStakeDiff,
		},
		{
			// Next retarget is at 432.
			name: "3rd retarget interval, 100% demand",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},  // 256
				{175, 20, minStakeDiff}, // 431
			},
			expectedDiff: minStakeDiff,
		},
		{
			// Next retarget is at 2304.
			name: "16th retarget interval - 1, 100% demand",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},   // 256
				{2046, 20, minStakeDiff}, // 2302
			},
			expectedDiff: minStakeDiff,
		},
		{
			// Next retarget is at 2304.
			name: "16th retarget interval, 100% demand",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},   // 256
				{2047, 20, minStakeDiff}, // 2303
			},
			expectedDiff: 208418769,
		},
		{
			// Next retarget is at 2448.
			name: "17th retarget interval - 1, 100% demand",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},   // 256
				{2047, 20, minStakeDiff}, // 2303
				{143, 20, 208418769},     // 2446
			},
			expectedDiff: 208418769,
		},
		{
			// Next retarget is at 2448.
			name: "17th retarget interval, 100% demand",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},   // 256
				{2047, 20, minStakeDiff}, // 2303
				{144, 20, 208418769},     // 2447
			},
			expectedDiff: 231326567,
		},
		{
			// Next retarget is at 2592.
			name: "17th retarget interval+1, 100% demand",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},   // 256
				{2047, 20, minStakeDiff}, // 2303
				{144, 20, 208418769},     // 2447
				{1, 20, 231326567},       // 2448
			},
			expectedDiff: 231326567,
		},
		{
			// Next retarget is at 3456.
			name: "24th retarget interval, varying demand",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},  // 256
				{31, 20, minStakeDiff},  // 287
				{144, 10, minStakeDiff}, // 431
				{144, 20, minStakeDiff}, // 575
				{144, 10, minStakeDiff}, // 719
				{144, 20, minStakeDiff}, // 863
				{144, 10, minStakeDiff}, // 1007
				{144, 20, minStakeDiff}, // 1151
				{144, 10, minStakeDiff}, // 1295
				{144, 20, minStakeDiff}, // 1439
				{144, 10, minStakeDiff}, // 1583
				{144, 20, minStakeDiff}, // 1727
				{144, 10, minStakeDiff}, // 1871
				{144, 20, minStakeDiff}, // 2015
				{144, 10, minStakeDiff}, // 2159
				{144, 20, minStakeDiff}, // 2303
				{144, 10, minStakeDiff}, // 2447
				{144, 20, minStakeDiff}, // 2591
				{144, 10, minStakeDiff}, // 2735
				{144, 20, minStakeDiff}, // 2879
				{144, 9, 201743368},     // 3023
				{144, 20, 201093236},    // 3167
				{144, 8, 222625877},     // 3311
				{144, 20, 242331291},    // 3455
			},
			expectedDiff: 291317641,
		},
		{
			// Next retarget is at 4176.  Post stake validation
			// height.
			name: "29th retarget interval, 100% demand",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},   // 256
				{2047, 20, minStakeDiff}, // 2303
				{144, 20, 208418769},     // 2447
				{144, 20, 231326567},     // 2591
				{144, 20, 272451490},     // 2735
				{144, 20, 339388424},     // 2879
				{144, 20, 445827839},     // 3023
				{144, 20, 615949254},     // 3167
				{144, 20, 892862990},     // 3311
				{144, 20, 1354989669},    // 3455
				{144, 20, 2148473276},    // 3599
				{144, 20, 3552797658},    // 3743
				{144, 20, 6116808441},    // 3887
				{144, 20, 10947547379},   // 4031
				{144, 20, 20338554623},   // 4175
			},
			expectedDiff: 22097687698,
		},
		{
			// Next retarget is at 4176.  Post stake validation
			// height.
			name: "29th retarget interval, 50% demand",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},   // 256
				{3919, 10, minStakeDiff}, // 4175
			},
			expectedDiff: minStakeDiff,
		},
		{
			// Next retarget is at 4464.  Post stake validation
			// height.
			name: "31st retarget interval, waning demand",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},   // 256
				{2047, 20, minStakeDiff}, // 2303
				{144, 20, 208418769},     // 2447
				{144, 20, 231326567},     // 2591
				{144, 20, 272451490},     // 2735
				{144, 20, 339388424},     // 2879
				{144, 20, 445827839},     // 3023
				{144, 20, 615949254},     // 3167
				{144, 20, 892862990},     // 3311
				{144, 20, 1354989669},    // 3455
				{144, 20, 2148473276},    // 3599
				{144, 20, 3552797658},    // 3743
				{144, 13, 6116808441},    // 3887
				{144, 0, 10645659768},    // 4031
				{144, 0, 18046712136},    // 4175
				{144, 0, 22097687698},    // 4319
				{144, 0, 22152524112},    // 4463
			},
			expectedDiff: 22207360526,
		},
	}

nextTest:
	for _, test := range tests {
		bc := newFakeChain(params)

		// immatureTickets tracks which height the purchased tickets
		// will mature and thus be eligible for admission to the live
		// ticket pool.
		immatureTickets := make(map[uint32]uint8)
		var poolSize uint32
		for _, ticketInfo := range test.ticketInfo {
			// Ensure the test data isn't faking ticket purchases at
			// an incorrect difficulty.
			gotDiff, err := bc.calcNextRequiredStakeDifficultyV2(bc.bestNode)
			if err != nil {
				t.Errorf("calcNextRequiredStakeDifficultyV2 (%s): "+
					"unexpected error: %v", test.name, err)
				continue nextTest
			}
			if gotDiff != ticketInfo.stakeDiff {
				t.Errorf("calcNextRequiredStakeDifficultyV2 (%s): "+
					"did not get expected stake difficulty -- got "+
					"%d, want %d", test.name, gotDiff,
					ticketInfo.stakeDiff)
				continue nextTest
			}

			for i := uint32(0); i < ticketInfo.numNodes; i++ {
				// Make up a header.
				nextHeight := uint32(bc.bestNode.height) + 1
				header := &wire.BlockHeader{
					Version:    4,
					SBits:      ticketInfo.stakeDiff,
					Height:     nextHeight,
					FreshStake: ticketInfo.newTickets,
					PoolSize:   poolSize,
				}
				node := newBlockNode(header, bc.bestNode)

				// Update the pool size for the next header.
				// Notice how tickets that mature for this block
				// do not show up in the pool size until the
				// next block.  This is correct behavior.
				poolSize += uint32(immatureTickets[nextHeight])
				delete(immatureTickets, nextHeight)
				if int64(nextHeight) >= stakeValidationHeight {
					poolSize -= uint32(params.TicketsPerBlock)
				}

				// Track maturity height for new ticket
				// purchases.
				maturityHeight := nextHeight + ticketMaturity
				immatureTickets[maturityHeight] = ticketInfo.newTickets

				// Update the chain to use the new fake node as
				// the new best node.
				bc.bestNode = node
			}
		}

		// Ensure the calculated difficulty matches the expected value.
		gotDiff, err := bc.calcNextRequiredStakeDifficultyV2(bc.bestNode)
		if err != nil {
			t.Errorf("calcNextRequiredStakeDifficultyV2 (%s): "+
				"unexpected error: %v", test.name, err)
			continue
		}
		if gotDiff != test.expectedDiff {
			t.Errorf("calcNextRequiredStakeDifficultyV2 (%s): "+
				"did not get expected stake difficulty -- got "+
				"%d, want %d", test.name, gotDiff,
				test.expectedDiff)
			continue
		}
	}
}

// TestEstimateNextStakeDiffV2 ensures the function that estimates the stake
// diff calculation for the algorithm defined by DCP0001 works as expected.
func TestEstimateNextStakeDiffV2(t *testing.T) {
	t.Parallel()

	// ticketInfo is used to control the tests by specifying the details
	// about how many fake blocks to create with the specified number of
	// ticket and stake difficulty.
	type ticketInfo struct {
		numNodes   uint32
		newTickets uint8
		stakeDiff  int64
	}

	// Specify the params used in the tests and assert the values directly
	// used by the tests are the expected ones.  All of the test values will
	// need to be updated if these parameters change since they are manually
	// calculated based on them.
	params := &chaincfg.MainNetParams
	assertStakeDiffParams(t, params)
	minStakeDiff := params.MinimumStakeDiff
	ticketMaturity := uint32(params.TicketMaturity)
	stakeValidationHeight := params.StakeValidationHeight

	tests := []struct {
		name          string
		ticketInfo    []ticketInfo
		newTickets    int64
		useMaxTickets bool
		expectedDiff  int64
	}{
		{
			// Regardless of claiming tickets will be purchased, the
			// resulting stake difficulty should be the minimum
			// because the first retarget is before the start
			// height.
			name:          "genesis block",
			ticketInfo:    []ticketInfo{{0, 0, minStakeDiff}},
			newTickets:    2860,
			useMaxTickets: false,
			expectedDiff:  minStakeDiff,
		},
		{
			// Next retarget is 144.  Resulting stake difficulty
			// should be the minimum regardless of claimed ticket
			// purchases because the previous pool size is still 0.
			name:          "during retarget, but before coinbase",
			ticketInfo:    []ticketInfo{{140, 0, minStakeDiff}},
			newTickets:    20 * 3, // blocks 141, 142, and 143.
			useMaxTickets: true,
			expectedDiff:  minStakeDiff,
		},
		{
			// Next retarget is at 288.  Regardless of claiming
			// tickets will be purchased, the resulting stake
			// difficulty should be the min because the previous
			// pool size is still 0.
			name:          "at coinbase maturity",
			ticketInfo:    []ticketInfo{{256, 0, minStakeDiff}},
			useMaxTickets: true,
			expectedDiff:  minStakeDiff,
		},
		{
			// Next retarget is at 288.  Regardless of actually
			// purchasing tickets and claiming more tickets will be
			// purchased, the resulting stake difficulty should be
			// the min because the previous pool size is still 0.
			name: "2nd retarget interval - 2, 100% demand",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff}, // 256
				{30, 20, minStakeDiff}, // 286
			},
			useMaxTickets: true,
			expectedDiff:  minStakeDiff,
		},
		{
			// Next retarget is at 288.  Still expect minimum stake
			// difficulty since the raw result would be lower.
			name: "2nd retarget interval - 1, 100% demand",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff}, // 256
				{31, 20, minStakeDiff}, // 287
			},
			useMaxTickets: true,
			expectedDiff:  minStakeDiff,
		},
		{
			// Next retarget is at 432.
			name: "3rd retarget interval, 100% demand, 1st block",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff}, // 256
				{32, 20, minStakeDiff}, // 288
			},
			useMaxTickets: true,
			expectedDiff:  minStakeDiff,
		},
		{
			// Next retarget is at 2304.
			name: "16th retarget interval, 100% demand, 1st block",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},   // 256
				{1904, 20, minStakeDiff}, // 2160
			},
			useMaxTickets: true,
			expectedDiff:  208418769,
		},
		{
			// Next retarget is at 2304.
			name: "16th retarget interval, 100% demand, 2nd block",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},   // 256
				{1905, 20, minStakeDiff}, // 2161
			},
			useMaxTickets: true,
			expectedDiff:  208418769,
		},
		{
			// Next retarget is at 2304.
			name: "16th retarget interval, 100% demand, final block",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},   // 256
				{2047, 20, minStakeDiff}, // 2303
			},
			useMaxTickets: true,
			expectedDiff:  208418769,
		},
		{
			// Next retarget is at 3456.
			name: "24th retarget interval, varying demand, 5th block",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},  // 256
				{31, 20, minStakeDiff},  // 287
				{144, 10, minStakeDiff}, // 431
				{144, 20, minStakeDiff}, // 575
				{144, 10, minStakeDiff}, // 719
				{144, 20, minStakeDiff}, // 863
				{144, 10, minStakeDiff}, // 1007
				{144, 20, minStakeDiff}, // 1151
				{144, 10, minStakeDiff}, // 1295
				{144, 20, minStakeDiff}, // 1439
				{144, 10, minStakeDiff}, // 1583
				{144, 20, minStakeDiff}, // 1727
				{144, 10, minStakeDiff}, // 1871
				{144, 20, minStakeDiff}, // 2015
				{144, 10, minStakeDiff}, // 2159
				{144, 20, minStakeDiff}, // 2303
				{144, 10, minStakeDiff}, // 2447
				{144, 20, minStakeDiff}, // 2591
				{144, 10, minStakeDiff}, // 2735
				{144, 20, minStakeDiff}, // 2879
				{144, 9, 201743368},     // 3023
				{144, 20, 201093236},    // 3167
				{144, 8, 222625877},     // 3311
				{5, 20, 242331291},      // 3316
			},
			useMaxTickets: true,
			expectedDiff:  291317641,
		},
		{
			// Next retarget is at 4176.  Post stake validation
			// height.
			name: "29th retarget interval, 100% demand, 10th block",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},   // 256
				{2047, 20, minStakeDiff}, // 2303
				{144, 20, 208418769},     // 2447
				{144, 20, 231326567},     // 2591
				{144, 20, 272451490},     // 2735
				{144, 20, 339388424},     // 2879
				{144, 20, 445827839},     // 3023
				{144, 20, 615949254},     // 3167
				{144, 20, 892862990},     // 3311
				{144, 20, 1354989669},    // 3455
				{144, 20, 2148473276},    // 3599
				{144, 20, 3552797658},    // 3743
				{144, 20, 6116808441},    // 3887
				{144, 20, 10947547379},   // 4031
				{10, 20, 20338554623},    // 4041
			},
			useMaxTickets: true,
			expectedDiff:  22097687698,
		},
		{
			// Next retarget is at 4176.  Post stake validation
			// height.
			name: "29th retarget interval, 50% demand, 23rd block",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},   // 256
				{3775, 10, minStakeDiff}, // 4031
				{23, 10, minStakeDiff},   // 4054
			},
			newTickets:    1210, // 121 * 10
			useMaxTickets: false,
			expectedDiff:  minStakeDiff,
		},
		{
			// Next retarget is at 4464.  Post stake validation
			// height.
			name: "31st retarget interval, waning demand, 117th block",
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiff},   // 256
				{2047, 20, minStakeDiff}, // 2303
				{144, 20, 208418769},     // 2447
				{144, 20, 231326567},     // 2591
				{144, 20, 272451490},     // 2735
				{144, 20, 339388424},     // 2879
				{144, 20, 445827839},     // 3023
				{144, 20, 615949254},     // 3167
				{144, 20, 892862990},     // 3311
				{144, 20, 1354989669},    // 3455
				{144, 20, 2148473276},    // 3599
				{144, 20, 3552797658},    // 3743
				{144, 13, 6116808441},    // 3887
				{144, 0, 10645659768},    // 4031
				{144, 0, 18046712136},    // 4175
				{144, 0, 22097687698},    // 4319
				{117, 0, 22152524112},    // 4436
			},
			useMaxTickets: false,
			newTickets:    0,
			expectedDiff:  22207360526,
		},
	}

nextTest:
	for _, test := range tests {
		bc := newFakeChain(params)

		// immatureTickets track which height the purchased tickets will
		// mature and thus be eligible for admission to the live ticket
		// pool.
		immatureTickets := make(map[uint32]uint8)
		var poolSize uint32
		for _, ticketInfo := range test.ticketInfo {
			// Ensure the test data isn't faking ticket purchases at
			// an incorrect difficulty.
			reqDiff, err := bc.calcNextRequiredStakeDifficultyV2(bc.bestNode)
			if err != nil {
				t.Errorf("calcNextRequiredStakeDifficultyV2 (%s): "+
					"unexpected error: %v", test.name, err)
				continue nextTest
			}
			if ticketInfo.stakeDiff != reqDiff {
				t.Errorf("calcNextRequiredStakeDifficultyV2 (%s): "+
					"test data has incorrect stake difficulty: "+
					"has %d, requires %d", test.name,
					ticketInfo.stakeDiff, reqDiff)
				continue nextTest
			}

			for i := uint32(0); i < ticketInfo.numNodes; i++ {
				// Make up a header.
				nextHeight := uint32(bc.bestNode.height) + 1
				header := &wire.BlockHeader{
					Version:    4,
					SBits:      ticketInfo.stakeDiff,
					Height:     nextHeight,
					FreshStake: ticketInfo.newTickets,
					PoolSize:   poolSize,
				}
				node := newBlockNode(header, bc.bestNode)

				// Update the pool size for the next header.
				// Notice how tickets that mature for this block
				// do not show up in the pool size until the
				// next block.  This is correct behavior.
				poolSize += uint32(immatureTickets[nextHeight])
				delete(immatureTickets, nextHeight)
				if int64(nextHeight) >= stakeValidationHeight {
					poolSize -= uint32(params.TicketsPerBlock)
				}

				// Track maturity height for new ticket
				// purchases.
				maturityHeight := nextHeight + ticketMaturity
				immatureTickets[maturityHeight] = ticketInfo.newTickets

				// Update the chain to use the new fake node as
				// the new best node.
				bc.bestNode = node
			}
		}

		// Ensure the calculated difficulty matches the expected value.
		gotDiff, err := bc.estimateNextStakeDifficultyV2(bc.bestNode,
			test.newTickets, test.useMaxTickets)
		if err != nil {
			t.Errorf("estimateNextStakeDifficultyV2 (%s): "+
				"unexpected error: %v", test.name, err)
			continue
		}
		if gotDiff != test.expectedDiff {
			t.Errorf("estimateNextStakeDifficultyV2 (%s): did not "+
				"get expected stake difficulty -- got %d, "+
				"want %d", test.name, gotDiff, test.expectedDiff)
			continue
		}
	}
}
