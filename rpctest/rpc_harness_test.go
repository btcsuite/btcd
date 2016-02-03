// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
package rpctest

import (
	"bytes"
	"math"
	"os"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcwallet/waddrmgr"
)

func TestSetUp(t *testing.T) {
	// Create a new test instance.
	nodeTest, err := New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer nodeTest.TearDown()

	// Initiate setup, generated a chain of length 125.
	if err := nodeTest.SetUp(true, 25); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}

	// Chain length is 125, we should have 26 mature coinbase outputs.
	maxConfs := int32(math.MaxInt32)
	unspentOutputs, err := nodeTest.Wallet.ListUnspent(0, maxConfs, nil)
	if err != nil {
		t.Fatalf("unable to retrieve unspent outputs from the wallet: %v",
			err)
	}
	if len(unspentOutputs) != 26 {
		t.Fatalf("incorrect number of mature coinbases, have %v, should be %v",
			len(unspentOutputs), 26)
	}

	// Current tip should be at height 124.
	nodeInfo, err := nodeTest.Node.GetInfo()
	if err != nil {
		t.Fatalf("unable to execute getinfo on node: %v", err)
	}
	if nodeInfo.Blocks != 125 {
		t.Errorf("Chain height is %v, should be %v",
			nodeInfo.Blocks, 125)
	}

	nodeTest.TearDown()

	// Ensure all test directories have been deleted.
	if _, err := os.Stat(nodeTest.testNodeDir); err == nil {
		t.Errorf("Create test datadir was not deleted.")
	}
}

func TestCoinbaseSpend(t *testing.T) {
	// Create a new test instance.
	nodeTest, err := New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer nodeTest.TearDown()

	// Initiate setup, generated a chain of length 125.
	if err := nodeTest.SetUp(true, 25); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}

	// Grab a fresh address from the wallet.
	addr, err := nodeTest.Wallet.NewAddress(waddrmgr.DefaultAccountNum)
	if err != nil {
		t.Fatalf("unable to get new address: %v", err)
	}

	// Next, send 5 BTC to this address, spending from one of our mature
	// coinbase outputs.
	outputs := map[string]btcutil.Amount{addr.String(): 5e8}
	txid, err := nodeTest.CoinbaseSpend(outputs)
	if err != nil {
		t.Fatalf("coinbase spend failed: %v", err)
	}

	// Generate a single block, the transaction the wallet created should
	// be found in this block.
	blockHashes, err := nodeTest.Node.Generate(1)
	if err != nil {
		t.Fatalf("unable to generate single block: %v", err)
	}

	block, err := nodeTest.Node.GetBlock(blockHashes[0])
	if err != nil {
		t.Fatalf("unable to get block: %v", err)
	}

	minedTx := block.Transactions()[1]
	txSha := minedTx.Sha()
	if !bytes.Equal(txid[:], txSha.Bytes()[:]) {
		t.Fatalf("txid's don't match, %v vs %v", txSha, txid)
	}
}
