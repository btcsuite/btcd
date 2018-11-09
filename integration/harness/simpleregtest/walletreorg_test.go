// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package simpleregtest

import (
	"testing"

	"github.com/btcsuite/btcd/integration/harness"
	"github.com/btcsuite/btcutil"
)

func TestMemWalletReorg(t *testing.T) {
	// Skip tests when running with -short
	if testing.Short() {
		t.Skip("Skipping RPC h tests in short mode")
	}
	r := ObtainHarness(mainHarnessName)

	// Create a fresh h, we'll be using the main h to force a
	// re-org on this local h.
	h := testSetup.Regnet5.NewInstance(t.Name() + ".4").(*harness.Harness)
	defer testSetup.Regnet5.Dispose(h)
	h.Wallet.Sync()

	// The internal wallet of this h should now have 250 BTC.
	expectedBalance := btcutil.Amount(250 * btcutil.SatoshiPerBitcoin)
	walletBalance := h.Wallet.ConfirmedBalance()
	if expectedBalance != walletBalance {
		t.Fatalf("wallet balance incorrect: expected %v, got %v",
			expectedBalance, walletBalance)
	}

	// Now connect this local h to the main h then wait for
	// their chains to synchronize.
	if err := ConnectNode(h, r); err != nil {
		t.Fatalf("unable to connect harnesses: %v", err)
	}
	nodeSlice := []*harness.Harness{r, h}
	if err := JoinNodes(nodeSlice, Blocks); err != nil {
		t.Fatalf("unable to join node on blocks: %v", err)
	}

	// The original wallet should now have a balance of 0 BTC as its entire
	// chain should have been decimated in favor of the main h'
	// chain.
	expectedBalance = btcutil.Amount(0)
	walletBalance = h.Wallet.ConfirmedBalance()
	if expectedBalance != walletBalance {
		t.Fatalf("wallet balance incorrect: expected %v, got %v",
			expectedBalance, walletBalance)
	}
}
