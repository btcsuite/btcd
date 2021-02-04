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
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

func testGetTxOutSetInfo(r *rpctest.Harness, t *testing.T) {

	p2pkhScript := func() []byte {
		address, err := r.NewAddress()
		if err != nil {
			t.Fatalf("Unable to generate address: %v", err)
		}

		script, err := txscript.PayToAddrScript(address)

		if err != nil {
			t.Fatalf("Unable to generate PKScript: %v", err)
		}

		return script
	}

	const txQuantity int = 20
	testTxs := make([]*btcutil.Tx, txQuantity)

	// Generate transaction outputs to be added to the utxo.
	for i := 0; i < txQuantity; i++ {
		// Each transaction has 2 outputs + change.
		txOuts := []*wire.TxOut{
			{
				Value:    int64(2000000000),
				PkScript: p2pkhScript(),
			},
			{
				Value:    int64(2000000000),
				PkScript: p2pkhScript(),
			},
		}

		tx, err := r.CreateTransaction(txOuts, 0, true)
		if err != nil {
			fmt.Println(i)
			t.Fatalf("Unable to generate transaction: %v", err)
		}
		testTxs[i] = btcutil.NewTx(tx)
	}

	tests := []struct {
		name         string
		txs          []*btcutil.Tx
		height       int64
		transactions int64
		txOuts       int64
		bogoSize     int64
		totalAmount  int64
	}{
		{
			name:         "starting utxo set",
			txs:          []*btcutil.Tx{},
			height:       126, // The harness starts with 125 blocks. The test adds 1 more.
			transactions: 126,
			txOuts:       126,
			bogoSize:     126 * (50 + 25), // Each p2pkh script has 25 bytes in length.
			totalAmount:  126 * 5000000000,
		},
		{
			name:         "add 40 utxos",
			txs:          testTxs,
			height:       127,
			transactions: 127,
			txOuts:       167,
			bogoSize:     167 * (50 + 25),
			totalAmount:  127 * 5000000000,
		},
	}
	for _, test := range tests {
		block, err := r.GenerateAndSubmitBlock(test.txs, -1, time.Time{})
		if err != nil {
			t.Fatalf("Unable to generate block: %v", err)
		}

		txOutSetInfo, err := r.Node.GetTxOutSetInfo()
		if err != nil {
			t.Fatalf("Call to `gettxoutsetinfo` failed in test %v", test.name)
		}
		if txOutSetInfo.Height != test.height {
			t.Errorf("Unexpected block height in test %v, got: %v want %v", test.name, txOutSetInfo.Height, test.height)
		}
		if txOutSetInfo.BestBlock != *block.Hash() {
			t.Errorf("Unexpected block hash in test %v, got: %v want %v", test.name, txOutSetInfo.BestBlock, *block.Hash())
		}
		if txOutSetInfo.Transactions != test.transactions {
			t.Errorf("Unexpected transactions in test %v, got: %v want %v", test.name, txOutSetInfo.Transactions, test.transactions)
		}
		if txOutSetInfo.TxOuts != test.txOuts {
			t.Errorf("Unexpected transaction outputs in test %v, got: %v want %v", test.name, txOutSetInfo.TxOuts, test.txOuts)
		}
		if txOutSetInfo.BogoSize != test.bogoSize {
			t.Errorf("Unexpected bogosize in test %v, got: %v want %v", test.name, txOutSetInfo.BogoSize, test.bogoSize)
		}
		if int64(txOutSetInfo.TotalAmount) != test.totalAmount {
			t.Errorf("Unexpected total amount in test %v, got: %v want %v", test.name, txOutSetInfo.TotalAmount, test.totalAmount)
		}
	}
}

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

var rpcTestCases = []rpctest.HarnessTestCase{
	testGetTxOutSetInfo, // This test must be run first, otherwise the utxo set may change.
	testGetBestBlock,
	testGetBlockCount,
	testGetBlockHash,
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
