// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math/big"
	"runtime"
	"testing"
	"time"

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

// assertStakeDiffParamsMainNet ensure the passed params have the values used in
// the tests related to mainnet stake difficulty calculation.
func assertStakeDiffParamsMainNet(t *testing.T, params *chaincfg.Params) {
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

// assertStakeDiffParamsTestNet ensure the passed params have the values used in
// the tests related to testnet stake difficulty calculation.
func assertStakeDiffParamsTestNet(t *testing.T, params *chaincfg.Params) {
	if params.MinimumStakeDiff != 20000000 {
		_, file, line, _ := runtime.Caller(1)
		t.Fatalf("%s:%d -- expect params with minimum stake diff of "+
			"%d, got %d", file, line, 20000000,
			params.MinimumStakeDiff)
	}
	if params.TicketMaturity != 16 {
		_, file, line, _ := runtime.Caller(1)
		t.Fatalf("%s:%d -- expect params with ticket maturity of "+
			"%d, got %d", file, line, 16, params.TicketMaturity)
	}
	if params.StakeValidationHeight != 768 {
		_, file, line, _ := runtime.Caller(1)
		t.Fatalf("%s:%d -- expect params with stake val height of %d, "+
			"got %d", file, line, 768, params.StakeValidationHeight)
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
	assertStakeDiffParamsMainNet(t, params)
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
			tip := bc.bestChain.Tip()
			gotDiff, err := bc.calcNextRequiredStakeDifficultyV2(tip)
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
				nextHeight := uint32(tip.height) + 1
				header := &wire.BlockHeader{
					Version:    4,
					SBits:      ticketInfo.stakeDiff,
					Height:     nextHeight,
					FreshStake: ticketInfo.newTickets,
					PoolSize:   poolSize,
				}
				tip = newBlockNode(header, tip)

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
				bc.bestChain.SetTip(tip)
			}
		}

		// Ensure the calculated difficulty matches the expected value.
		gotDiff, err := bc.calcNextRequiredStakeDifficultyV2(bc.bestChain.Tip())
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
	// tickets and stake difficulty.
	type ticketInfo struct {
		numNodes   uint32
		newTickets uint8
		stakeDiff  int64
	}

	// Assert the param values directly used by the tests are the expected
	// ones.  All of the test values will need to be updated if these
	// parameters change since they are manually calculated based on them.
	mainNetParams := &chaincfg.MainNetParams
	testNetParams := &chaincfg.TestNet3Params
	assertStakeDiffParamsMainNet(t, mainNetParams)
	assertStakeDiffParamsTestNet(t, testNetParams)
	minStakeDiffMainNet := mainNetParams.MinimumStakeDiff
	minStakeDiffTestNet := testNetParams.MinimumStakeDiff

	tests := []struct {
		name          string
		params        *chaincfg.Params
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
			params:        mainNetParams,
			ticketInfo:    []ticketInfo{{0, 0, minStakeDiffMainNet}},
			newTickets:    2860,
			useMaxTickets: false,
			expectedDiff:  minStakeDiffMainNet,
		},
		{
			// Next retarget is 144.  Resulting stake difficulty
			// should be the minimum regardless of claimed ticket
			// purchases because the previous pool size is still 0.
			name:          "during retarget, but before coinbase",
			params:        mainNetParams,
			ticketInfo:    []ticketInfo{{140, 0, minStakeDiffMainNet}},
			newTickets:    20 * 3, // blocks 141, 142, and 143.
			useMaxTickets: true,
			expectedDiff:  minStakeDiffMainNet,
		},
		{
			// Next retarget is at 288.  Regardless of claiming
			// tickets will be purchased, the resulting stake
			// difficulty should be the min because the previous
			// pool size is still 0.
			name:          "at coinbase maturity",
			params:        mainNetParams,
			ticketInfo:    []ticketInfo{{256, 0, minStakeDiffMainNet}},
			useMaxTickets: true,
			expectedDiff:  minStakeDiffMainNet,
		},
		{
			// Next retarget is at 288.  Regardless of actually
			// purchasing tickets and claiming more tickets will be
			// purchased, the resulting stake difficulty should be
			// the min because the previous pool size is still 0.
			name:   "2nd retarget interval - 2, 100% demand",
			params: mainNetParams,
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiffMainNet}, // 256
				{30, 20, minStakeDiffMainNet}, // 286
			},
			useMaxTickets: true,
			expectedDiff:  minStakeDiffMainNet,
		},
		{
			// Next retarget is at 288.  Still expect minimum stake
			// difficulty since the raw result would be lower.
			name:   "2nd retarget interval - 1, 100% demand",
			params: mainNetParams,
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiffMainNet}, // 256
				{31, 20, minStakeDiffMainNet}, // 287
			},
			useMaxTickets: true,
			expectedDiff:  minStakeDiffMainNet,
		},
		{
			// Next retarget is at 432.
			name:   "3rd retarget interval, 100% demand, 1st block",
			params: mainNetParams,
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiffMainNet}, // 256
				{32, 20, minStakeDiffMainNet}, // 288
			},
			useMaxTickets: true,
			expectedDiff:  minStakeDiffMainNet,
		},
		{
			// Next retarget is at 2304.
			name:   "16th retarget interval, 100% demand, 1st block",
			params: mainNetParams,
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiffMainNet},   // 256
				{1904, 20, minStakeDiffMainNet}, // 2160
			},
			useMaxTickets: true,
			expectedDiff:  208418769,
		},
		{
			// Next retarget is at 2304.
			name:   "16th retarget interval, 100% demand, 2nd block",
			params: mainNetParams,
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiffMainNet},   // 256
				{1905, 20, minStakeDiffMainNet}, // 2161
			},
			useMaxTickets: true,
			expectedDiff:  208418769,
		},
		{
			// Next retarget is at 2304.
			name:   "16th retarget interval, 100% demand, final block",
			params: mainNetParams,
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiffMainNet},   // 256
				{2047, 20, minStakeDiffMainNet}, // 2303
			},
			useMaxTickets: true,
			expectedDiff:  208418769,
		},
		{
			// Next retarget is at 3456.
			name:   "24th retarget interval, varying demand, 5th block",
			params: mainNetParams,
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiffMainNet},  // 256
				{31, 20, minStakeDiffMainNet},  // 287
				{144, 10, minStakeDiffMainNet}, // 431
				{144, 20, minStakeDiffMainNet}, // 575
				{144, 10, minStakeDiffMainNet}, // 719
				{144, 20, minStakeDiffMainNet}, // 863
				{144, 10, minStakeDiffMainNet}, // 1007
				{144, 20, minStakeDiffMainNet}, // 1151
				{144, 10, minStakeDiffMainNet}, // 1295
				{144, 20, minStakeDiffMainNet}, // 1439
				{144, 10, minStakeDiffMainNet}, // 1583
				{144, 20, minStakeDiffMainNet}, // 1727
				{144, 10, minStakeDiffMainNet}, // 1871
				{144, 20, minStakeDiffMainNet}, // 2015
				{144, 10, minStakeDiffMainNet}, // 2159
				{144, 20, minStakeDiffMainNet}, // 2303
				{144, 10, minStakeDiffMainNet}, // 2447
				{144, 20, minStakeDiffMainNet}, // 2591
				{144, 10, minStakeDiffMainNet}, // 2735
				{144, 20, minStakeDiffMainNet}, // 2879
				{144, 9, 201743368},            // 3023
				{144, 20, 201093236},           // 3167
				{144, 8, 222625877},            // 3311
				{5, 20, 242331291},             // 3316
			},
			useMaxTickets: true,
			expectedDiff:  291317641,
		},
		{
			// Next retarget is at 4176.  Post stake validation
			// height.
			name:   "29th retarget interval, 100% demand, 10th block",
			params: mainNetParams,
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiffMainNet},   // 256
				{2047, 20, minStakeDiffMainNet}, // 2303
				{144, 20, 208418769},            // 2447
				{144, 20, 231326567},            // 2591
				{144, 20, 272451490},            // 2735
				{144, 20, 339388424},            // 2879
				{144, 20, 445827839},            // 3023
				{144, 20, 615949254},            // 3167
				{144, 20, 892862990},            // 3311
				{144, 20, 1354989669},           // 3455
				{144, 20, 2148473276},           // 3599
				{144, 20, 3552797658},           // 3743
				{144, 20, 6116808441},           // 3887
				{144, 20, 10947547379},          // 4031
				{10, 20, 20338554623},           // 4041
			},
			useMaxTickets: true,
			expectedDiff:  22097687698,
		},
		{
			// Next retarget is at 4176.  Post stake validation
			// height.
			name:   "29th retarget interval, 50% demand, 23rd block",
			params: mainNetParams,
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiffMainNet},   // 256
				{3775, 10, minStakeDiffMainNet}, // 4031
				{23, 10, minStakeDiffMainNet},   // 4054
			},
			newTickets:    1210, // 121 * 10
			useMaxTickets: false,
			expectedDiff:  minStakeDiffMainNet,
		},
		{
			// Next retarget is at 4464.  Post stake validation
			// height.
			name:   "31st retarget interval, waning demand, 117th block",
			params: mainNetParams,
			ticketInfo: []ticketInfo{
				{256, 0, minStakeDiffMainNet},   // 256
				{2047, 20, minStakeDiffMainNet}, // 2303
				{144, 20, 208418769},            // 2447
				{144, 20, 231326567},            // 2591
				{144, 20, 272451490},            // 2735
				{144, 20, 339388424},            // 2879
				{144, 20, 445827839},            // 3023
				{144, 20, 615949254},            // 3167
				{144, 20, 892862990},            // 3311
				{144, 20, 1354989669},           // 3455
				{144, 20, 2148473276},           // 3599
				{144, 20, 3552797658},           // 3743
				{144, 13, 6116808441},           // 3887
				{144, 0, 10645659768},           // 4031
				{144, 0, 18046712136},           // 4175
				{144, 0, 22097687698},           // 4319
				{117, 0, 22152524112},           // 4436
			},
			useMaxTickets: false,
			newTickets:    0,
			expectedDiff:  22207360526,
		},
		// --------------------------
		// TestNet params start here.
		// --------------------------
		{
			// Regardless of claiming tickets will be purchased, the
			// resulting stake difficulty should be the minimum
			// because the first retarget is before the start
			// height.
			name:          "genesis block",
			params:        testNetParams,
			ticketInfo:    []ticketInfo{{0, 0, minStakeDiffTestNet}},
			newTickets:    2860,
			useMaxTickets: false,
			expectedDiff:  minStakeDiffTestNet,
		},
		{
			// Next retarget is at 144.  Regardless of claiming
			// tickets will be purchased, the resulting stake
			// difficulty should be the min because the previous
			// pool size is still 0.
			name:          "at coinbase maturity",
			params:        testNetParams,
			ticketInfo:    []ticketInfo{{16, 0, minStakeDiffTestNet}},
			useMaxTickets: true,
			expectedDiff:  minStakeDiffTestNet,
		},
		{
			// Next retarget is at 144.  Regardless of actually
			// purchasing tickets and claiming more tickets will be
			// purchased, the resulting stake difficulty should be
			// the min because the previous pool size is still 0.
			name:   "1st retarget interval - 2, 100% demand",
			params: testNetParams,
			ticketInfo: []ticketInfo{
				{16, 0, minStakeDiffTestNet},   // 16
				{126, 20, minStakeDiffTestNet}, // 142
			},
			useMaxTickets: true,
			expectedDiff:  minStakeDiffTestNet,
		},
		{
			// Next retarget is at 288.  Still expect minimum stake
			// difficulty since the raw result would be lower.
			name:   "2nd retarget interval - 1, 30% demand",
			params: testNetParams,
			ticketInfo: []ticketInfo{
				{16, 0, minStakeDiffTestNet},  // 16
				{271, 6, minStakeDiffTestNet}, // 287
			},
			useMaxTickets: true,
			expectedDiff:  minStakeDiffTestNet,
		},
		{
			// Next retarget is at 288.  Still expect minimum stake
			// difficulty since the raw result would be lower.
			//
			// Since the ticket maturity is smaller than the
			// retarget interval, this case ensures some of the
			// nodes being estimated will mature during the
			// interval.
			name:   "2nd retarget interval - 23, 30% demand",
			params: testNetParams,
			ticketInfo: []ticketInfo{
				{16, 0, minStakeDiffTestNet},  // 16
				{249, 6, minStakeDiffTestNet}, // 265
			},
			newTickets:    132, // 22 * 6
			useMaxTickets: false,
			expectedDiff:  minStakeDiffTestNet,
		},
		{
			// Next retarget is at 288.  Still expect minimum stake
			// difficulty since the raw result would be lower.
			//
			// None of the nodes being estimated will mature during the
			// interval.
			name:   "2nd retarget interval - 11, 30% demand",
			params: testNetParams,
			ticketInfo: []ticketInfo{
				{16, 0, minStakeDiffTestNet},  // 16
				{261, 6, minStakeDiffTestNet}, // 277
			},
			newTickets:    60, // 10 * 6
			useMaxTickets: false,
			expectedDiff:  minStakeDiffTestNet,
		},
		{
			// Next retarget is at 432.
			name:   "3rd retarget interval, 100% demand, 1st block",
			params: testNetParams,
			ticketInfo: []ticketInfo{
				{16, 0, minStakeDiffTestNet},   // 16
				{256, 20, minStakeDiffTestNet}, // 288
			},
			useMaxTickets: true,
			expectedDiff:  44505494,
		},
		{
			// Next retarget is at 432.
			//
			// None of the nodes being estimated will mature during the
			// interval.
			name:   "3rd retarget interval - 11, 100% demand",
			params: testNetParams,
			ticketInfo: []ticketInfo{
				{16, 0, minStakeDiffTestNet},   // 16
				{271, 20, minStakeDiffTestNet}, // 287
				{134, 20, 44505494},            // 421
			},
			useMaxTickets: true,
			expectedDiff:  108661875,
		},
		{
			// Next retarget is at 576.
			name:   "4th retarget interval, 100% demand, 1st block",
			params: testNetParams,
			ticketInfo: []ticketInfo{
				{16, 0, minStakeDiffTestNet},   // 16
				{271, 20, minStakeDiffTestNet}, // 287
				{144, 20, 44505494},            // 431
				{1, 20, 108661875},             // 432
			},
			useMaxTickets: true,
			expectedDiff:  314319918,
		},
		{
			// Next retarget is at 576.
			name:   "4th retarget interval, 100% demand, 2nd block",
			params: testNetParams,
			ticketInfo: []ticketInfo{
				{16, 0, minStakeDiffTestNet},   // 16
				{271, 20, minStakeDiffTestNet}, // 287
				{144, 20, 44505494},            // 431
				{2, 20, 108661875},             // 433
			},
			useMaxTickets: true,
			expectedDiff:  314319918,
		},
		{
			// Next retarget is at 576.
			name:   "4th retarget interval, 100% demand, final block",
			params: testNetParams,
			ticketInfo: []ticketInfo{
				{16, 0, minStakeDiffTestNet},   // 16
				{271, 20, minStakeDiffTestNet}, // 287
				{144, 20, 44505494},            // 431
				{144, 20, 108661875},           // 575
			},
			useMaxTickets: true,
			expectedDiff:  314319918,
		},
		{
			// Next retarget is at 1152.
			name:   "9th retarget interval, varying demand, 137th block",
			params: testNetParams,
			ticketInfo: []ticketInfo{
				{16, 0, minStakeDiffTestNet},   // 16
				{127, 20, minStakeDiffTestNet}, // 143
				{144, 10, minStakeDiffTestNet}, // 287
				{144, 20, 24055097},            // 431
				{144, 10, 54516186},            // 575
				{144, 20, 105335577},           // 719
				{144, 10, 304330579},           // 863
				{144, 20, 772249463},           // 1007
				{76, 10, 2497324513},           // 1083
				{9, 0, 2497324513},             // 1092
				{1, 10, 2497324513},            // 1093
				{8, 0, 2497324513},             // 1101
				{1, 10, 2497324513},            // 1102
				{12, 0, 2497324513},            // 1114
				{1, 10, 2497324513},            // 1115
				{9, 0, 2497324513},             // 1124
				{1, 10, 2497324513},            // 1125
				{8, 0, 2497324513},             // 1133
				{1, 10, 2497324513},            // 1134
				{10, 0, 2497324513},            // 1144
			},
			useMaxTickets: false,
			newTickets:    10,
			expectedDiff:  6976183842,
		},
		{
			// Next retarget is at 1440.  The estimated number of
			// tickets are such that they span the ticket maturity
			// floor so that the estimation result is slightly
			// different as compared to what it would be if each
			// remaining node only had 10 ticket purchases.  This is
			// because it results in a different number of maturing
			// tickets depending on how they are allocated on each
			// side of the maturity floor.
			name:   "11th retarget interval, 50% demand, 127th block",
			params: testNetParams,
			ticketInfo: []ticketInfo{
				{16, 0, minStakeDiffTestNet},   // 16
				{271, 10, minStakeDiffTestNet}, // 287
				{144, 10, 22252747},            // 431
				{144, 10, 27165468},            // 575
				{144, 10, 39289988},            // 719
				{144, 10, 66729608},            // 863
				{144, 10, 116554208},           // 1007
				{144, 10, 212709675},           // 1151
				{144, 10, 417424410},           // 1295
				{127, 10, 876591473},           // 1422
			},
			useMaxTickets: false,
			newTickets:    170, // 17 * 10
			expectedDiff:  1965171141,
		},
		{
			// Next retarget is at 1440.  This is similar to the
			// last test except all of the estimated tickets are
			// after the ticket maturity floor, so the estimate is
			// the same as if each remaining node only had 10 ticket
			// purchases.
			name:   "11th retarget interval, 50% demand, 128th block",
			params: testNetParams,
			ticketInfo: []ticketInfo{
				{16, 0, minStakeDiffTestNet},   // 16
				{271, 10, minStakeDiffTestNet}, // 287
				{144, 10, 22252747},            // 431
				{144, 10, 27165468},            // 575
				{144, 10, 39289988},            // 719
				{144, 10, 66729608},            // 863
				{144, 10, 116554208},           // 1007
				{144, 10, 212709675},           // 1151
				{144, 10, 417424410},           // 1295
				{128, 10, 876591473},           // 1423
			},
			useMaxTickets: false,
			newTickets:    160, // 16 * 10
			expectedDiff:  1961558695,
		},
	}

nextTest:
	for _, test := range tests {
		stakeValidationHeight := test.params.StakeValidationHeight
		ticketMaturity := uint32(test.params.TicketMaturity)
		ticketsPerBlock := uint32(test.params.TicketsPerBlock)

		bc := newFakeChain(test.params)

		// immatureTickets track which height the purchased tickets will
		// mature and thus be eligible for admission to the live ticket
		// pool.
		immatureTickets := make(map[uint32]uint8)
		var poolSize uint32
		for _, ticketInfo := range test.ticketInfo {
			// Ensure the test data isn't faking ticket purchases at
			// an incorrect difficulty.
			tip := bc.bestChain.Tip()
			reqDiff, err := bc.calcNextRequiredStakeDifficultyV2(tip)
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
				nextHeight := uint32(tip.height) + 1
				header := &wire.BlockHeader{
					Version:    4,
					SBits:      ticketInfo.stakeDiff,
					Height:     nextHeight,
					FreshStake: ticketInfo.newTickets,
					PoolSize:   poolSize,
				}
				tip = newBlockNode(header, tip)

				// Update the pool size for the next header.
				// Notice how tickets that mature for this block
				// do not show up in the pool size until the
				// next block.  This is correct behavior.
				poolSize += uint32(immatureTickets[nextHeight])
				delete(immatureTickets, nextHeight)
				if int64(nextHeight) >= stakeValidationHeight {
					poolSize -= ticketsPerBlock
				}

				// Track maturity height for new ticket
				// purchases.
				maturityHeight := nextHeight + ticketMaturity
				immatureTickets[maturityHeight] = ticketInfo.newTickets

				// Update the chain to use the new fake node as
				// the new best node.
				bc.bestChain.SetTip(tip)
			}
		}

		// Ensure the calculated difficulty matches the expected value.
		gotDiff, err := bc.estimateNextStakeDifficultyV2(bc.bestChain.Tip(),
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

// TestMinDifficultyReduction ensures the code which results in reducing the
// minimum required difficulty, when the network params allow it, works as
// expected.
func TestMinDifficultyReduction(t *testing.T) {
	// Create chain params based on simnet params, but set the fields related to
	// proof-of-work difficulty to specific values expected by the tests.
	params := chaincfg.SimNetParams
	params.ReduceMinDifficulty = true
	params.TargetTimePerBlock = time.Minute * 2
	params.MinDiffReductionTime = time.Minute * 10 // ~99.3% chance to be mined
	params.WorkDiffAlpha = 1
	params.WorkDiffWindowSize = 144
	params.WorkDiffWindows = 20
	params.TargetTimespan = params.TargetTimePerBlock *
		time.Duration(params.WorkDiffWindowSize)
	params.RetargetAdjustmentFactor = 4

	tests := []struct {
		name           string
		timeAdjustment func(i int) time.Duration
		numBlocks      int64
		expectedDiff   func(i int) uint32
	}{
		{
			name:           "genesis block",
			timeAdjustment: func(i int) time.Duration { return time.Second },
			numBlocks:      1,
			expectedDiff:   func(i int) uint32 { return params.PowLimitBits },
		},
		{
			name:           "create difficulty spike - part 1",
			timeAdjustment: func(i int) time.Duration { return time.Second },
			numBlocks:      params.WorkDiffWindowSize - 2,
			expectedDiff:   func(i int) uint32 { return 545259519 },
		},
		{
			name:           "create difficulty spike - part 2",
			timeAdjustment: func(i int) time.Duration { return time.Second },
			numBlocks:      params.WorkDiffWindowSize,
			expectedDiff:   func(i int) uint32 { return 545259519 },
		},
		{
			name:           "create difficulty spike - part 3",
			timeAdjustment: func(i int) time.Duration { return time.Second },
			numBlocks:      params.WorkDiffWindowSize,
			expectedDiff:   func(i int) uint32 { return 541100164 },
		},
		{
			name:           "create difficulty spike - part 4",
			timeAdjustment: func(i int) time.Duration { return time.Second },
			numBlocks:      params.WorkDiffWindowSize,
			expectedDiff:   func(i int) uint32 { return 537954654 },
		},
		{
			name:           "create difficulty spike - part 5",
			timeAdjustment: func(i int) time.Duration { return time.Second },
			numBlocks:      params.WorkDiffWindowSize,
			expectedDiff:   func(i int) uint32 { return 537141847 },
		},
		{
			name:           "create difficulty spike - part 6",
			timeAdjustment: func(i int) time.Duration { return time.Second },
			numBlocks:      params.WorkDiffWindowSize,
			expectedDiff:   func(i int) uint32 { return 536938645 },
		},
		{
			name:           "create difficulty spike - part 7",
			timeAdjustment: func(i int) time.Duration { return time.Second },
			numBlocks:      params.WorkDiffWindowSize,
			expectedDiff:   func(i int) uint32 { return 524428608 },
		},
		{
			name:           "create difficulty spike - part 8",
			timeAdjustment: func(i int) time.Duration { return time.Second },
			numBlocks:      params.WorkDiffWindowSize,
			expectedDiff:   func(i int) uint32 { return 521177424 },
		},
		{
			name:           "create difficulty spike - part 9",
			timeAdjustment: func(i int) time.Duration { return time.Second },
			numBlocks:      params.WorkDiffWindowSize,
			expectedDiff:   func(i int) uint32 { return 520364628 },
		},
		{
			name:           "create difficulty spike - part 10",
			timeAdjustment: func(i int) time.Duration { return time.Second },
			numBlocks:      params.WorkDiffWindowSize,
			expectedDiff:   func(i int) uint32 { return 520161429 },
		},
		{
			name: "alternate min diff blocks",
			timeAdjustment: func(i int) time.Duration {
				if i%2 == 0 {
					return params.MinDiffReductionTime + time.Second
				}
				return params.TargetTimePerBlock
			},
			numBlocks: params.WorkDiffWindowSize,
			expectedDiff: func(i int) uint32 {
				if i%2 == 0 && i != 0 {
					return params.PowLimitBits
				}
				return 507651392
			},
		},
		{
			name: "interval of blocks taking twice the target time - part 1",
			timeAdjustment: func(i int) time.Duration {
				return params.TargetTimePerBlock * 2
			},
			numBlocks:    params.WorkDiffWindowSize,
			expectedDiff: func(i int) uint32 { return 509850141 },
		},
		{
			name: "interval of blocks taking twice the target time - part 2",
			timeAdjustment: func(i int) time.Duration {
				return params.TargetTimePerBlock * 2
			},
			numBlocks:    params.WorkDiffWindowSize,
			expectedDiff: func(i int) uint32 { return 520138451 },
		},
		{
			name: "interval of blocks taking twice the target time - part 3",
			timeAdjustment: func(i int) time.Duration {
				return params.TargetTimePerBlock * 2
			},
			numBlocks:    params.WorkDiffWindowSize,
			expectedDiff: func(i int) uint32 { return 520177692 },
		},
	}

	bc := newFakeChain(&params)
	node := bc.bestChain.Tip()
	blockTime := time.Unix(node.timestamp, 0)
	for _, test := range tests {
		for i := 0; i < int(test.numBlocks); i++ {
			// Update the block time according to the test data and calculate
			// the difficulty for the next block.
			blockTime = blockTime.Add(test.timeAdjustment(i))
			diff, err := bc.calcNextRequiredDifficulty(node, blockTime)
			if err != nil {
				t.Fatalf("calcNextRequiredDifficulty: unexpected err: %v", err)
			}

			// Ensure the calculated difficulty matches the expected value.
			expectedDiff := test.expectedDiff(i)
			if diff != expectedDiff {
				t.Fatalf("calcNextRequiredDifficulty (%s): did not get "+
					"expected difficulty -- got %d, want %d", test.name, diff,
					expectedDiff)
			}

			node = newFakeNode(node, 1, 1, diff, blockTime)
			bc.index.AddNode(node)
			bc.bestChain.SetTip(node)
		}
	}
}
