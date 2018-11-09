// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package simpleregtest

import (
	"testing"

	"github.com/btcsuite/btcd/integration/harness"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

func TestMemWalletLockedOutputs(t *testing.T) {
	// Skip tests when running with -short
	if testing.Short() {
		t.Skip("Skipping RPC h tests in short mode")
	}
	r := ObtainHarness(mainHarnessName)
	// Obtain the initial balance of the wallet at this point.
	startingBalance := r.Wallet.ConfirmedBalance()

	// First, create a signed transaction spending some outputs.
	addr, err := r.Wallet.NewAddress(nil)
	if err != nil {
		t.Fatalf("unable to generate new address: %v", err)
	}
	pkScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		t.Fatalf("unable to create script: %v", err)
	}
	outputAmt := btcutil.Amount(50 * btcutil.SatoshiPerBitcoin)
	output := wire.NewTxOut(int64(outputAmt), pkScript)
	ctargs := &harness.CreateTransactionArgs{
		Outputs: []*wire.TxOut{output},
		FeeRate: 10,
		Change:  true,
	}
	tx, err := r.Wallet.CreateTransaction(ctargs)
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// The current wallet balance should now be at least 50 BTC less
	// (accounting for fees) than the period balance
	currentBalance := r.Wallet.ConfirmedBalance()
	if !(currentBalance <= startingBalance-outputAmt) {
		t.Fatalf("spent outputs not locked: previous balance %v, "+
			"current balance %v", startingBalance, currentBalance)
	}

	// Now unlocked all the spent inputs within the unbroadcast signed
	// transaction. The current balance should now be exactly that of the
	// starting balance.
	r.Wallet.UnlockOutputs(tx.TxIn)
	currentBalance = r.Wallet.ConfirmedBalance()
	if currentBalance != startingBalance {
		t.Fatalf("current and starting balance should now match: "+
			"expected %v, got %v", startingBalance, currentBalance)
	}
}
