// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is ignored during the regular tests due to the following build tag.
//go:build rpctest
// +build rpctest

package integration

import (
	"bytes"
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"testing"
	"time"

	"github.com/lbryio/lbcd/chaincfg"
	"github.com/lbryio/lbcd/chaincfg/chainhash"
	"github.com/lbryio/lbcd/integration/rpctest"
	"github.com/lbryio/lbcd/rpcclient"
)

func testGetBestBlock(r *rpctest.Harness, t *testing.T) {
	_, prevbestHeight, err := r.Client.GetBestBlock()
	if err != nil {
		t.Fatalf("Call to `getbestblock` failed: %v", err)
	}

	// Create a new block connecting to the current tip.
	generatedBlockHashes, err := r.Client.Generate(1)
	if err != nil {
		t.Fatalf("Unable to generate block: %v", err)
	}

	bestHash, bestHeight, err := r.Client.GetBestBlock()
	if err != nil {
		t.Fatalf("Call to `getbestblock` failed: %v", err)
	}

	// Hash should be the same as the newly submitted block.
	if !bytes.Equal(bestHash[:], generatedBlockHashes[0][:]) {
		t.Fatalf("Block hashes do not match. Returned hash %v, wanted "+
			"hash %v", bestHash, generatedBlockHashes[0][:])
	}

	// Block height should now reflect newest height.
	if bestHeight != prevbestHeight+1 {
		t.Fatalf("Block heights do not match. Got %v, wanted %v",
			bestHeight, prevbestHeight+1)
	}
}

func testGetBlockCount(r *rpctest.Harness, t *testing.T) {
	// Save the current count.
	currentCount, err := r.Client.GetBlockCount()
	if err != nil {
		t.Fatalf("Unable to get block count: %v", err)
	}

	if _, err := r.Client.Generate(1); err != nil {
		t.Fatalf("Unable to generate block: %v", err)
	}

	// Count should have increased by one.
	newCount, err := r.Client.GetBlockCount()
	if err != nil {
		t.Fatalf("Unable to get block count: %v", err)
	}
	if newCount != currentCount+1 {
		t.Fatalf("Block count incorrect. Got %v should be %v",
			newCount, currentCount+1)
	}
}

func testGetBlockHash(r *rpctest.Harness, t *testing.T) {
	// Create a new block connecting to the current tip.
	generatedBlockHashes, err := r.Client.Generate(1)
	if err != nil {
		t.Fatalf("Unable to generate block: %v", err)
	}

	info, err := r.Client.GetInfo()
	if err != nil {
		t.Fatalf("call to getinfo cailed: %v", err)
	}

	blockHash, err := r.Client.GetBlockHash(int64(info.Blocks))
	if err != nil {
		t.Fatalf("Call to `getblockhash` failed: %v", err)
	}

	// Block hashes should match newly created block.
	if !bytes.Equal(generatedBlockHashes[0][:], blockHash[:]) {
		t.Fatalf("Block hashes do not match. Returned hash %v, wanted "+
			"hash %v", blockHash, generatedBlockHashes[0][:])
	}
}

func testBulkClient(r *rpctest.Harness, t *testing.T) {
	// Create a new block connecting to the current tip.
	generatedBlockHashes, err := r.Client.Generate(20)
	if err != nil {
		t.Fatalf("Unable to generate block: %v", err)
	}

	var futureBlockResults []rpcclient.FutureGetBlockResult
	for _, hash := range generatedBlockHashes {
		futureBlockResults = append(futureBlockResults, r.BatchClient.GetBlockAsync(hash))
	}

	err = r.BatchClient.Send()
	if err != nil {
		t.Fatal(err)
	}

	isKnownBlockHash := func(blockHash chainhash.Hash) bool {
		for _, hash := range generatedBlockHashes {
			if blockHash.IsEqual(hash) {
				return true
			}
		}
		return false
	}

	for _, block := range futureBlockResults {
		msgBlock, err := block.Receive()
		if err != nil {
			t.Fatal(err)
		}
		blockHash := msgBlock.Header.BlockHash()
		if !isKnownBlockHash(blockHash) {
			t.Fatalf("expected hash %s  to be in generated hash list", blockHash)
		}
	}
}

func testGetBlockStats(r *rpctest.Harness, t *testing.T) {
	t.Parallel()

	baseFeeRate := int64(10)
	txValue := int64(50000000)
	txQuantity := 10
	txs := make([]*btcutil.Tx, txQuantity)
	fees := make([]int64, txQuantity)
	sizes := make([]int64, txQuantity)
	feeRates := make([]int64, txQuantity)
	var outputCount int

	// Generate test sample.
	for i := 0; i < txQuantity; i++ {
		address, err := r.NewAddress()
		if err != nil {
			t.Fatalf("Unable to generate address: %v", err)
		}

		pkScript, err := txscript.PayToAddrScript(address)
		if err != nil {
			t.Fatalf("Unable to generate PKScript: %v", err)
		}

		// This feerate is not the actual feerate. See comment below.
		feeRate := baseFeeRate * int64(i)

		tx, err := r.CreateTransaction([]*wire.TxOut{wire.NewTxOut(txValue, pkScript)}, btcutil.Amount(feeRate), true)
		if err != nil {
			t.Fatalf("Unable to generate segwit transaction: %v", err)
		}

		txs[i] = btcutil.NewTx(tx)
		sizes[i] = int64(tx.SerializeSize())

		// memWallet.fundTx makes some assumptions when calculating fees.
		// For instance, it assumes the signature script has exactly 108 bytes
		// and it does not account for the size of the change output.
		// This needs to be taken into account when getting the true feerate.
		scriptSigOffset := 108 - len(tx.TxIn[0].SignatureScript)
		changeOutputSize := tx.TxOut[len(tx.TxOut)-1].SerializeSize()
		fees[i] = (sizes[i] + int64(scriptSigOffset) - int64(changeOutputSize)) * feeRate
		feeRates[i] = fees[i] / sizes[i]

		outputCount += len(tx.TxOut)
	}

	stats := func(slice []int64) (int64, int64, int64, int64, int64) {
		var total, average, min, max, median int64
		min = slice[0]
		length := len(slice)
		for _, item := range slice {
			if min > item {
				min = item
			}
			if max < item {
				max = item
			}
			total += item
		}
		average = total / int64(length)
		sort.Slice(slice, func(i, j int) bool { return slice[i] < slice[j] })
		if length == 0 {
			median = 0
		} else if length%2 == 0 {
			median = (slice[length/2-1] + slice[length/2]) / 2
		} else {
			median = slice[length/2]
		}
		return total, average, min, max, median
	}

	totalFee, avgFee, minFee, maxFee, medianFee := stats(fees)
	totalSize, avgSize, minSize, maxSize, medianSize := stats(sizes)
	_, avgFeeRate, minFeeRate, maxFeeRate, _ := stats(feeRates)

	tests := []struct {
		name            string
		txs             []*btcutil.Tx
		stats           []string
		expectedResults map[string]interface{}
	}{
		{
			name:  "empty block",
			txs:   []*btcutil.Tx{},
			stats: []string{},
			expectedResults: map[string]interface{}{
				"avgfee":              int64(0),
				"avgfeerate":          int64(0),
				"avgtxsize":           int64(0),
				"feerate_percentiles": []int64{0, 0, 0, 0, 0},
				"ins":                 int64(0),
				"maxfee":              int64(0),
				"maxfeerate":          int64(0),
				"maxtxsize":           int64(0),
				"medianfee":           int64(0),
				"mediantxsize":        int64(0),
				"minfee":              int64(0),
				"mintxsize":           int64(0),
				"outs":                int64(1),
				"swtotal_size":        int64(0),
				"swtotal_weight":      int64(0),
				"swtxs":               int64(0),
				"total_out":           int64(0),
				"total_size":          int64(0),
				"total_weight":        int64(0),
				"txs":                 int64(1),
				"utxo_increase":       int64(1),
			},
		},
		{
			name: "block with 10 transactions + coinbase",
			txs:  txs,
			stats: []string{"avgfee", "avgfeerate", "avgtxsize", "feerate_percentiles",
				"ins", "maxfee", "maxfeerate", "maxtxsize", "medianfee", "mediantxsize",
				"minfee", "minfeerate", "mintxsize", "outs", "subsidy", "swtxs",
				"total_size", "total_weight", "totalfee", "txs", "utxo_increase"},
			expectedResults: map[string]interface{}{
				"avgfee":     avgFee,
				"avgfeerate": avgFeeRate,
				"avgtxsize":  avgSize,
				"feerate_percentiles": []int64{feeRates[0], feeRates[2],
					feeRates[4], feeRates[7], feeRates[8]},
				"ins":            int64(txQuantity),
				"maxfee":         maxFee,
				"maxfeerate":     maxFeeRate,
				"maxtxsize":      maxSize,
				"medianfee":      medianFee,
				"mediantxsize":   medianSize,
				"minfee":         minFee,
				"minfeerate":     minFeeRate,
				"mintxsize":      minSize,
				"outs":           int64(outputCount + 1), // Coinbase output also counts.
				"subsidy":        int64(5000000000),
				"swtotal_weight": nil, // This stat was not selected, so it should be nil.
				"swtxs":          int64(0),
				"total_size":     totalSize,
				"total_weight":   totalSize * 4,
				"totalfee":       totalFee,
				"txs":            int64(txQuantity + 1), // Coinbase transaction also counts.
				"utxo_increase":  int64(outputCount + 1 - txQuantity),
				"utxo_size_inc":  nil,
			},
		},
	}
	for _, test := range tests {
		// Submit a new block with the provided transactions.
		block, err := r.GenerateAndSubmitBlock(test.txs, -1, time.Time{})
		if err != nil {
			t.Fatalf("Unable to generate block: %v from test %s", err, test.name)
		}

		blockStats, err := r.Node.GetBlockStats(block.Hash(), &test.stats)
		if err != nil {
			t.Fatalf("Call to `getblockstats` on test %s failed: %v", test.name, err)
		}

		if blockStats.Height != (*int64)(nil) && *blockStats.Height != int64(block.Height()) {
			t.Fatalf("Unexpected result in test %s, stat: %v, expected: %v, got: %v", test.name, "height", block.Height(), *blockStats.Height)
		}

		for stat, value := range test.expectedResults {
			var result interface{}
			switch stat {
			case "avgfee":
				result = blockStats.AverageFee
			case "avgfeerate":
				result = blockStats.AverageFeeRate
			case "avgtxsize":
				result = blockStats.AverageTxSize
			case "feerate_percentiles":
				result = blockStats.FeeratePercentiles
			case "blockhash":
				result = blockStats.Hash
			case "height":
				result = blockStats.Height
			case "ins":
				result = blockStats.Ins
			case "maxfee":
				result = blockStats.MaxFee
			case "maxfeerate":
				result = blockStats.MaxFeeRate
			case "maxtxsize":
				result = blockStats.MaxTxSize
			case "medianfee":
				result = blockStats.MedianFee
			case "mediantime":
				result = blockStats.MedianTime
			case "mediantxsize":
				result = blockStats.MedianTxSize
			case "minfee":
				result = blockStats.MinFee
			case "minfeerate":
				result = blockStats.MinFeeRate
			case "mintxsize":
				result = blockStats.MinTxSize
			case "outs":
				result = blockStats.Outs
			case "swtotal_size":
				result = blockStats.SegWitTotalSize
			case "swtotal_weight":
				result = blockStats.SegWitTotalWeight
			case "swtxs":
				result = blockStats.SegWitTxs
			case "subsidy":
				result = blockStats.Subsidy
			case "time":
				result = blockStats.Time
			case "total_out":
				result = blockStats.TotalOut
			case "total_size":
				result = blockStats.TotalSize
			case "total_weight":
				result = blockStats.TotalWeight
			case "totalfee":
				result = blockStats.TotalFee
			case "txs":
				result = blockStats.Txs
			case "utxo_increase":
				result = blockStats.UTXOIncrease
			case "utxo_size_inc":
				result = blockStats.UTXOSizeIncrease
			}

			var equality bool

			// Check for nil equality.
			if value == nil && result == (*int64)(nil) {
				equality = true
				break
			} else if result == nil || value == nil {
				equality = false
			}

			var resultValue interface{}
			switch v := value.(type) {
			case int64:
				resultValue = *result.(*int64)
				equality = v == resultValue
			case string:
				resultValue = *result.(*string)
				equality = v == resultValue
			case []int64:
				resultValue = *result.(*[]int64)
				resultSlice := resultValue.([]int64)
				equality = true
				for i, item := range resultSlice {
					if item != v[i] {
						equality = false
						break
					}
				}
			}
			if !equality {
				if result != nil {
					t.Fatalf("Unexpected result in test %s, stat: %v, expected: %v, got: %v", test.name, stat, value, resultValue)
				} else {
					t.Fatalf("Unexpected result in test %s, stat: %v, expected: %v, got: %v", test.name, stat, value, "<nil>")
				}
			}

		}
	}
}

var rpcTestCases = []rpctest.HarnessTestCase{
	testGetBestBlock,
	testGetBlockCount,
	testGetBlockHash,
	testGetBlockStats,
	testBulkClient,
}

var primaryHarness *rpctest.Harness

func TestMain(m *testing.M) {
	var err error

	// In order to properly test scenarios on as if we were on mainnet,
	// ensure that non-standard transactions aren't accepted into the
	// mempool or relayed.
	// Enable transaction index to be able to fully test GetBlockStats
	btcdCfg := []string{"--rejectnonstd", "--txindex"}
	primaryHarness, err = rpctest.New(
		&chaincfg.SimNetParams, nil, btcdCfg, "",
	)
	if err != nil {
		fmt.Println("unable to create primary harness: ", err)
		os.Exit(1)
	}

	// Initialize the primary mining node with a chain of length 125,
	// providing 25 mature coinbases to allow spending from for testing
	// purposes.
	if err := primaryHarness.SetUp(true, 25); err != nil {
		fmt.Println("unable to setup test chain: ", err)

		// Even though the harness was not fully setup, it still needs
		// to be torn down to ensure all resources such as temp
		// directories are cleaned up.  The error is intentionally
		// ignored since this is already an error path and nothing else
		// could be done about it anyways.
		_ = primaryHarness.TearDown()
		os.Exit(1)
	}

	exitCode := m.Run()

	// Clean up any active harnesses that are still currently running.This
	// includes removing all temporary directories, and shutting down any
	// created processes.
	if err := rpctest.TearDownAll(); err != nil {
		fmt.Println("unable to tear down all harnesses: ", err)
		os.Exit(1)
	}

	os.Exit(exitCode)
}

func TestRpcServer(t *testing.T) {
	var currentTestNum int
	defer func() {
		// If one of the integration tests caused a panic within the main
		// goroutine, then tear down all the harnesses in order to avoid
		// any leaked btcd processes.
		if r := recover(); r != nil {
			fmt.Println("recovering from test panic: ", r)
			if err := rpctest.TearDownAll(); err != nil {
				fmt.Println("unable to tear down all harnesses: ", err)
			}
			t.Fatalf("test #%v panicked: %s", currentTestNum, debug.Stack())
		}
	}()

	for _, testCase := range rpcTestCases {
		testCase(primaryHarness, t)

		currentTestNum++
	}
}
