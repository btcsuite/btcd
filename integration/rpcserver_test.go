// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is ignored during the regular tests due to the following build tag.
// +build rpctest

package integration

import (
	"bytes"
	"fmt"
	"os"
	"runtime/debug"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcutil"
)

func testGetBestBlock(r *rpctest.Harness, t *testing.T) {
	_, prevbestHeight, err := r.Node.GetBestBlock()
	if err != nil {
		t.Fatalf("Call to `getbestblock` failed: %v", err)
	}

	// Create a new block connecting to the current tip.
	generatedBlockHashes, err := r.Node.Generate(1)
	if err != nil {
		t.Fatalf("Unable to generate block: %v", err)
	}

	bestHash, bestHeight, err := r.Node.GetBestBlock()
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
	currentCount, err := r.Node.GetBlockCount()
	if err != nil {
		t.Fatalf("Unable to get block count: %v", err)
	}

	if _, err := r.Node.Generate(1); err != nil {
		t.Fatalf("Unable to generate block: %v", err)
	}

	// Count should have increased by one.
	newCount, err := r.Node.GetBlockCount()
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
	generatedBlockHashes, err := r.Node.Generate(1)
	if err != nil {
		t.Fatalf("Unable to generate block: %v", err)
	}

	info, err := r.Node.GetInfo()
	if err != nil {
		t.Fatalf("call to getinfo cailed: %v", err)
	}

	blockHash, err := r.Node.GetBlockHash(int64(info.Blocks))
	if err != nil {
		t.Fatalf("Call to `getblockhash` failed: %v", err)
	}

	// Block hashes should match newly created block.
	if !bytes.Equal(generatedBlockHashes[0][:], blockHash[:]) {
		t.Fatalf("Block hashes do not match. Returned hash %v, wanted "+
			"hash %v", blockHash, generatedBlockHashes[0][:])
	}
}

func testMiningAddrCalls(r *rpctest.Harness, t *testing.T) {
	var err error
	const numAddrs = 5

	// First generate a few fresh addresses from the harness.
	addrs := make([]btcutil.Address, numAddrs)
	for i := 0; i < numAddrs; i++ {
		addrs[i], err = r.NewAddress()
		if err != nil {
			t.Fatalf("unable to generate new address: %v", err)
		}
	}

	// Next, we use the addminingaddr call to add the above generated
	// addresses to btcd's set of mining addresses.
	for _, addr := range addrs {
		if err := r.Node.AddMiningAddr(addr); err != nil {
			t.Fatalf("unable to add mining address: %v", err)
		}
	}

	// Attempting to add the same address twice should be rejected due to
	// duplicates.
	if err := r.Node.AddMiningAddr(addrs[0]); err == nil {
		t.Fatalf("duplicate mining address wasn't rejected")
	}

	// Next, try to delete a single address, then immediately try to delete
	// the same address. The second call should result in an error as the
	// address is no longer within the set of mining addresses.
	if err := r.Node.DelMiningAddr(addrs[2]); err != nil {
		t.Fatalf("unable to delete mining address: %v", err)
	}
	if err := r.Node.DelMiningAddr(addrs[2]); err == nil {
		t.Fatalf("deletion of non-existent address should fail")
	}

	// Query for the list of mining addresses, there should be exactly 5
	// addresses: one from the harness' initialization, and four of the
	// remaining addresses we added.
	miningAddrs, err := r.Node.ListMiningAddrs()
	if err != nil {
		t.Fatalf("unable to list mining addrs: %v", err)
	}
	if len(miningAddrs) != 5 {
		t.Fatalf("expected %v mining addrs after deletion got %v", 5,
			len(miningAddrs))
	}

	// Next, remove all the remaining address we generated from btcd's set
	// of mining addresses.
	for _, addr := range append(addrs[:2], addrs[3:]...) {
		if err := r.Node.DelMiningAddr(addr); err != nil {
			t.Fatalf("unable to delete mining addr: %v", err)
		}
	}

	// Only a single address should remain at this point: the harness'
	// original mining address.
	miningAddrs, err = r.Node.ListMiningAddrs()
	if err != nil {
		t.Fatalf("unable to list mining addrs: %v", err)
	}
	if len(miningAddrs) != 1 {
		t.Fatalf("expected %v mining addrs after deletion got %v", 1,
			len(miningAddrs))
	}
}

var rpcTestCases = []rpctest.HarnessTestCase{
	testGetBestBlock,
	testGetBlockCount,
	testGetBlockHash,
	testMiningAddrCalls,
}

var primaryHarness *rpctest.Harness

func TestMain(m *testing.M) {
	var err error

	// In order to properly test scenarios on as if we were on mainnet,
	// ensure that non-standard transactions aren't accepted into the
	// mempool or relayed.
	btcdCfg := []string{"--rejectnonstd"}
	primaryHarness, err = rpctest.New(&chaincfg.SimNetParams, nil, btcdCfg)
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
