// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package simpleregtest

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/integration/harness"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

func TestGenerateAndSubmitBlockWithCustomCoinbaseOutputs(t *testing.T) {
	// Skip tests when running with -short
	if testing.Short() {
		t.Skip("Skipping RPC harness tests in short mode")
	}
	r := ObtainHarness(mainHarnessName)

	// Generate a few test spend transactions.
	addr, err := r.Wallet.NewAddress(nil)
	if err != nil {
		t.Fatalf("unable to generate new address: %v", err)
	}
	pkScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		t.Fatalf("unable to create script: %v", err)
	}
	output := wire.NewTxOut(btcutil.SatoshiPerBitcoin, pkScript)

	const numTxns = 5
	txns := make([]*btcutil.Tx, 0, numTxns)
	for i := 0; i < numTxns; i++ {
		ctargs := &harness.CreateTransactionArgs{
			Outputs: []*wire.TxOut{output},
			FeeRate: 10,
			Change:  true,
		}
		tx, err := r.Wallet.CreateTransaction(ctargs)
		if err != nil {
			t.Fatalf("unable to create tx: %v", err)
		}

		txns = append(txns, btcutil.NewTx(tx))
	}

	// Now generate a block with the default block version, a zero'd out
	// time, and a burn output.
	newBlockArgs := harness.GenerateBlockArgs{
		Txns:         txns,
		BlockVersion: BlockVersion,
		BlockTime:    time.Time{},
		MineTo: []wire.TxOut{{
			Value:    0,
			PkScript: []byte{},
		}}}
	block, err := r.Node.GenerateAndSubmitBlockWithCustomCoinbaseOutputs(newBlockArgs)
	if err != nil {
		t.Fatalf("unable to generate block: %v", err)
	}

	// Ensure that all created transactions were included, and that the
	// block version was properly set to the default.
	numBlocksTxns := len(block.Transactions())
	if numBlocksTxns != numTxns+1 {
		t.Fatalf("block did not include all transactions: "+
			"expected %v, got %v", numTxns+1, numBlocksTxns)
	}
	blockVersion := block.MsgBlock().Header.Version
	if blockVersion != BlockVersion {
		t.Fatalf("block version is not default: expected %v, got %v",
			BlockVersion, blockVersion)
	}

	// Next generate a block with a "non-standard" block version along with
	// time stamp a minute after the previous block's timestamp.
	timestamp := block.MsgBlock().Header.Timestamp.Add(time.Minute)
	targetBlockVersion := int32(1337)
	newBlockArgs2 := harness.GenerateBlockArgs{
		Txns:         nil,
		BlockVersion: targetBlockVersion,
		BlockTime:    timestamp,
		MineTo: []wire.TxOut{{
			Value:    0,
			PkScript: []byte{},
		}}}
	block, err = r.Node.GenerateAndSubmitBlockWithCustomCoinbaseOutputs(newBlockArgs2)
	if err != nil {
		t.Fatalf("unable to generate block: %v", err)
	}

	// Finally ensure that the desired block version and timestamp were set
	// properly.
	header := block.MsgBlock().Header
	blockVersion = header.Version
	if blockVersion != targetBlockVersion {
		t.Fatalf("block version mismatch: expected %v, got %v",
			targetBlockVersion, blockVersion)
	}
	if !timestamp.Equal(header.Timestamp) {
		t.Fatalf("header time stamp mismatch: expected %v, got %v",
			timestamp, header.Timestamp)
	}
}
