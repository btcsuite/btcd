// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package simpleregtest

import (
	"bytes"
	"testing"
)

func TestGetBlockHash(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping RPC harness tests in short mode")
	}
	r := ObtainHarness(mainHarnessName)
	// Create a new block connecting to the current tip.
	generatedBlockHashes, err := r.NodeRPCClient().Generate(1)
	if err != nil {
		t.Fatalf("Unable to generate block: %v", err)
	}

	info, err := r.NodeRPCClient().GetInfo()
	if err != nil {
		t.Fatalf("call to getinfo cailed: %v", err)
	}

	blockHash, err := r.NodeRPCClient().GetBlockHash(int64(info.Blocks))
	if err != nil {
		t.Fatalf("Call to `getblockhash` failed: %v", err)
	}

	// Block hashes should match newly created block.
	if !bytes.Equal(generatedBlockHashes[0][:], blockHash[:]) {
		t.Fatalf("Block hashes do not match. Returned hash %v, wanted "+
			"hash %v", blockHash, generatedBlockHashes[0][:])
	}
}
