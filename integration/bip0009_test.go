// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is ignored during the regular tests due to the following build tag.
// +build rpctest

package integration

import (
	"runtime"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpctest"
)

// assertVersionBit gets the passed block hash from the given test harness and
// ensures its version either has the provided bit set or unset per the set
// flag.
func assertVersionBit(r *rpctest.Harness, t *testing.T, hash *chainhash.Hash, bit uint8, set bool) {
	block, err := r.Node.GetBlock(hash)
	if err != nil {
		t.Fatalf("failed to retrieve block %v: %v", hash, err)
	}
	switch {
	case set && block.Header.Version&(1<<bit) == 0:
		_, _, line, _ := runtime.Caller(1)
		t.Fatalf("assertion failed at line %d: block %s, version 0x%x "+
			"does not have bit %d set", line, hash,
			block.Header.Version, bit)
	case !set && block.Header.Version&(1<<bit) != 0:
		_, _, line, _ := runtime.Caller(1)
		t.Fatalf("assertion failed at line %d: block %s, version 0x%x "+
			"has bit %d set", line, hash, block.Header.Version, bit)
	}
}

// TestBIP0009Mining ensures blocks built via btcd's CPU miner follow the rules
// set forth by BIP0009 by using the test dummy deployment.
//
// Overview:
// - Generate block 1
//   - Assert bit is NOT set (ThresholdDefined)
// - Generate enough blocks to reach first state transition
//   - Assert bit is NOT set for block prior to state transition
//   - Assert bit is set for block at state transition (ThresholdStarted)
// - Generate enough blocks to reach second state transition
//   - Assert bit is set for block at state transition (ThresholdLockedIn)
// - Generate enough blocks to reach third state transition
//   - Assert bit is set for block prior to state transition (ThresholdLockedIn)
//   - Assert bit is NOT set for block at state transition (ThresholdActive)
func TestBIP0009Mining(t *testing.T) {
	t.Parallel()

	// Initialize the primary mining node with only the genesis block.
	r, err := rpctest.New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatalf("unable to create primary harness: %v", err)
	}
	if err := r.SetUp(true, 0); err != nil {
		t.Fatalf("unable to setup test chain: %v", err)
	}
	defer r.TearDown()

	// *** ThresholdDefined ***
	//
	// Generate a block that extends the genesis block.  It should not have
	// the test dummy bit set in the version since the first window is
	// in the defined threshold state.
	deployment := &r.ActiveNet.Deployments[chaincfg.DeploymentTestDummy]
	testDummyBitNum := deployment.BitNumber
	hashes, err := r.Node.Generate(1)
	if err != nil {
		t.Fatalf("unable to generate blocks: %v", err)
	}
	assertVersionBit(r, t, hashes[0], testDummyBitNum, false)

	// *** ThresholdStarted ***
	//
	// Generate enough blocks to reach the first state transition.
	//
	// The second to last generated block should not have the test bit set
	// in the version.
	//
	// The last generated block should now have the test bit set in the
	// version since the btcd mining code will have recognized the test
	// dummy deployment as started.
	confirmationWindow := r.ActiveNet.MinerConfirmationWindow
	numNeeded := confirmationWindow - 1
	hashes, err = r.Node.Generate(numNeeded)
	if err != nil {
		t.Fatalf("failed to generated %d blocks: %v", numNeeded, err)
	}
	assertVersionBit(r, t, hashes[len(hashes)-2], testDummyBitNum, false)
	assertVersionBit(r, t, hashes[len(hashes)-1], testDummyBitNum, true)

	// *** ThresholdLockedIn ***
	//
	// Generate enough blocks to reach the next state transition.
	//
	// The last generated block should still have the test bit set in the
	// version since the btcd mining code will have recognized the test
	// dummy deployment as locked in.
	hashes, err = r.Node.Generate(confirmationWindow)
	if err != nil {
		t.Fatalf("failed to generated %d blocks: %v", confirmationWindow,
			err)
	}
	assertVersionBit(r, t, hashes[len(hashes)-1], testDummyBitNum, true)

	// *** ThresholdActivated ***
	//
	// Generate enough blocks to reach the next state transition.
	//
	// The second to last generated block should still have the test bit set
	// in the version since it is still locked in.
	//
	// The last generated block should NOT have the test bit set in the
	// version since the btcd mining code will have recognized the test
	// dummy deployment as activated and thus there is no longer any need
	// to set the bit.
	hashes, err = r.Node.Generate(confirmationWindow)
	if err != nil {
		t.Fatalf("failed to generated %d blocks: %v", confirmationWindow,
			err)
	}
	assertVersionBit(r, t, hashes[len(hashes)-2], testDummyBitNum, true)
	assertVersionBit(r, t, hashes[len(hashes)-1], testDummyBitNum, false)
}
