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
	"testing"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/rpcclient"
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

func calculateHashesPerSecBetweenBlockHeights(r *rpctest.Harness, t *testing.T, startHeight, endHeight int64) float64 {
	var totalWork int64 = 0
	var minTimestamp, maxTimestamp time.Time

	for curHeight := startHeight; curHeight <= endHeight; curHeight++ {
		hash, err := r.Client.GetBlockHash(curHeight)

		if err != nil {
			t.Fatal(err)
		}

		blockHeader, err := r.Client.GetBlockHeader(hash)

		if err != nil {
			t.Fatal(err)
		}

		if curHeight == startHeight {
			minTimestamp = blockHeader.Timestamp
			continue
		}

		totalWork += blockchain.CalcWork(blockHeader.Bits).Int64()

		if curHeight == endHeight {
			maxTimestamp = blockHeader.Timestamp
		}
	}

	timeDiff := maxTimestamp.Sub(minTimestamp).Seconds()

	if timeDiff == 0 {
		return 0
	}

	return float64(totalWork) / timeDiff
}

func testGetNetworkHashPS(r *rpctest.Harness, t *testing.T) {
	networkHashPS, err := r.Client.GetNetworkHashPS()

	if err != nil {
		t.Fatal(err)
	}

	expectedNetworkHashPS := calculateHashesPerSecBetweenBlockHeights(r, t, 28, 148)

	if networkHashPS != expectedNetworkHashPS {
		t.Fatalf("Network hashes per second should be %f but received: %f", expectedNetworkHashPS, networkHashPS)
	}
}

func testGetNetworkHashPS2(r *rpctest.Harness, t *testing.T) {
	networkHashPS2BlockTests := []struct {
		blocks              int
		expectedStartHeight int64
		expectedEndHeight   int64
	}{
		// Test receiving command for negative blocks
		{blocks: -200, expectedStartHeight: 0, expectedEndHeight: 148},
		// Test receiving command for 0 blocks
		{blocks: 0, expectedStartHeight: 0, expectedEndHeight: 148},
		// Test receiving command for less than total blocks -> expectedStartHeight = 148 - 100 = 48
		{blocks: 100, expectedStartHeight: 48, expectedEndHeight: 148},
		// Test receiving command for exact total blocks -> expectedStartHeight = 148 - 148 = 0
		{blocks: 148, expectedStartHeight: 0, expectedEndHeight: 148},
		// Test receiving command for greater than total blocks
		{blocks: 200, expectedStartHeight: 0, expectedEndHeight: 148},
	}

	for _, networkHashPS2BlockTest := range networkHashPS2BlockTests {
		blocks := networkHashPS2BlockTest.blocks
		expectedStartHeight := networkHashPS2BlockTest.expectedStartHeight
		expectedEndHeight := networkHashPS2BlockTest.expectedEndHeight

		networkHashPS, err := r.Client.GetNetworkHashPS2(blocks)

		if err != nil {
			t.Fatal(err)
		}

		expectedNetworkHashPS := calculateHashesPerSecBetweenBlockHeights(r, t, expectedStartHeight, expectedEndHeight)

		if networkHashPS != expectedNetworkHashPS {
			t.Fatalf("Network hashes per second should be %f but received: %f", expectedNetworkHashPS, networkHashPS)
		}
	}
}

func testGetNetworkHashPS3(r *rpctest.Harness, t *testing.T) {
	networkHashPS3BlockTests := []struct {
		height              int
		blocks              int
		expectedStartHeight int64
		expectedEndHeight   int64
	}{
		// Test receiving command for negative height -> expectedEndHeight force to 148
		// - And negative blocks -> expectedStartHeight = 148 - ((148 % 2016) + 1) =  -1 -> forced to 0
		{height: -200, blocks: -120, expectedStartHeight: 0, expectedEndHeight: 148},
		// - And zero blocks -> expectedStartHeight = 148 - ((148 % 2016) + 1) = -1 -> forced to 0
		{height: -200, blocks: 0, expectedStartHeight: 0, expectedEndHeight: 148},
		// - And positive blocks less than total blocks -> expectedStartHeight = 148 - 100 = 48
		{height: -200, blocks: 100, expectedStartHeight: 48, expectedEndHeight: 148},
		// - And positive blocks equal to total blocks
		{height: -200, blocks: 148, expectedStartHeight: 0, expectedEndHeight: 148},
		// - And positive blocks greater than total blocks
		{height: -200, blocks: 250, expectedStartHeight: 0, expectedEndHeight: 148},

		// Test receiving command for zero height
		// - Should return 0 similar to expected start height and expected end height both being 0
		// (blocks is irrelevant to output)
		{height: 0, blocks: 120, expectedStartHeight: 0, expectedEndHeight: 0},

		// Tests for valid block height -> expectedEndHeight set as height
		// - And negative blocks -> expectedStartHeight = 148 - ((148 % 2016) + 1) = -1 -> forced to 0
		{height: 100, blocks: -120, expectedStartHeight: 0, expectedEndHeight: 100},
		// - And zero blocks -> expectedStartHeight = 148 - ((148 % 2016) + 1) = -1 -> forced to 0
		{height: 100, blocks: 0, expectedStartHeight: 0, expectedEndHeight: 100},
		// - And positive blocks less than command blocks -> expectedStartHeight = 100 - 70 = 30
		{height: 100, blocks: 70, expectedStartHeight: 30, expectedEndHeight: 100},
		// - And positive blocks equal to command blocks -> expectedStartHeight = 100 - 100 = 0
		{height: 100, blocks: 100, expectedStartHeight: 0, expectedEndHeight: 100},
		// - And positive blocks greater than command blocks -> expectedStartHeight = 100 - 200 = -100 -> forced to 0
		{height: 100, blocks: 200, expectedStartHeight: 0, expectedEndHeight: 100},

		// Test receiving command for height greater than block height
		// - Should return 0 similar to expected start height and expected end height both being 0
		// (blocks is irrelevant to output)
		{height: 200, blocks: 120, expectedStartHeight: 0, expectedEndHeight: 0},
	}

	for _, networkHashPS3BlockTest := range networkHashPS3BlockTests {
		blocks := networkHashPS3BlockTest.blocks
		height := networkHashPS3BlockTest.height
		expectedStartHeight := networkHashPS3BlockTest.expectedStartHeight
		expectedEndHeight := networkHashPS3BlockTest.expectedEndHeight

		networkHashPS, err := r.Client.GetNetworkHashPS3(blocks, height)

		if err != nil {
			t.Fatal(err)
		}

		expectedNetworkHashPS := calculateHashesPerSecBetweenBlockHeights(r, t, expectedStartHeight, expectedEndHeight)

		if networkHashPS != expectedNetworkHashPS {
			t.Fatalf("Network hashes per second should be %f but received: %f", expectedNetworkHashPS, networkHashPS)
		}
	}
}

var rpcTestCases = []rpctest.HarnessTestCase{
	testGetBestBlock,
	testGetBlockCount,
	testGetBlockHash,
	testBulkClient,
	testGetNetworkHashPS,
	testGetNetworkHashPS2,
	testGetNetworkHashPS3,
}

var primaryHarness *rpctest.Harness

func TestMain(m *testing.M) {
	var err error

	// In order to properly test scenarios on as if we were on mainnet,
	// ensure that non-standard transactions aren't accepted into the
	// mempool or relayed.
	btcdCfg := []string{"--rejectnonstd"}
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
