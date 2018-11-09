// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package simpleregtest

import (
	"testing"

	"github.com/btcsuite/btcd/integration/harness"
)

func TestConnectNode(t *testing.T) {
	// Skip tests when running with -short
	if testing.Short() {
		t.Skip("Skipping RPC harness tests in short mode")
	}
	r := ObtainHarness(mainHarnessName)

	// Create a fresh test harness.
	harness := testSetup.Regnet0.NewInstance(t.Name()).(*harness.Harness)
	defer testSetup.Regnet0.Dispose(harness)

	// Establish a p2p connection from our new local harness to the main
	// harness.
	if err := ConnectNode(harness, r); err != nil {
		t.Fatalf("unable to connect local to main harness: %v", err)
	}

	// The main harness should show up in our local harness' peer's list,
	// and vice verse.
	assertConnectedTo(t, harness, r)
}
