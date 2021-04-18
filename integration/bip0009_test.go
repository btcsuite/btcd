// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is ignored during the regular tests due to the following build tag.
// +build rpctest

package integration

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/integration/rpctest"
)

const (
	// vbLegacyBlockVersion is the highest legacy block version before the
	// version bits scheme became active.
	vbLegacyBlockVersion = 4

	// vbTopBits defines the bits to set in the version to signal that the
	// version bits scheme is being used.
	vbTopBits = 0x20000000
)

// assertVersionBit gets the passed block hash from the given test harness and
// ensures its version either has the provided bit set or unset per the set
// flag.
func assertVersionBit(r *rpctest.Harness, t *testing.T, hash *chainhash.Hash, bit uint8, set bool) {
	block, err := r.Client.GetBlock(hash)
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

// assertChainHeight retrieves the current chain height from the given test
// harness and ensures it matches the provided expected height.
func assertChainHeight(r *rpctest.Harness, t *testing.T, expectedHeight uint32) {
	height, err := r.Client.GetBlockCount()
	if err != nil {
		t.Fatalf("failed to retrieve block height: %v", err)
	}
	if uint32(height) != expectedHeight {
		_, _, line, _ := runtime.Caller(1)
		t.Fatalf("assertion failed at line %d: block height of %d "+
			"is not the expected %d", line, height, expectedHeight)
	}
}

// thresholdStateToStatus converts the passed threshold state to the equivalent
// status string returned in the getblockchaininfo RPC.
func thresholdStateToStatus(state blockchain.ThresholdState) (string, error) {
	switch state {
	case blockchain.ThresholdDefined:
		return "defined", nil
	case blockchain.ThresholdStarted:
		return "started", nil
	case blockchain.ThresholdLockedIn:
		return "lockedin", nil
	case blockchain.ThresholdActive:
		return "active", nil
	case blockchain.ThresholdFailed:
		return "failed", nil
	}

	return "", fmt.Errorf("unrecognized threshold state: %v", state)
}

// assertSoftForkStatus retrieves the current blockchain info from the given
// test harness and ensures the provided soft fork key is both available and its
// status is the equivalent of the passed state.
func assertSoftForkStatus(r *rpctest.Harness, t *testing.T, forkKey string, state blockchain.ThresholdState) {
	// Convert the expected threshold state into the equivalent
	// getblockchaininfo RPC status string.
	status, err := thresholdStateToStatus(state)
	if err != nil {
		_, _, line, _ := runtime.Caller(1)
		t.Fatalf("assertion failed at line %d: unable to convert "+
			"threshold state %v to string", line, state)
	}

	info, err := r.Client.GetBlockChainInfo()
	if err != nil {
		t.Fatalf("failed to retrieve chain info: %v", err)
	}

	// Ensure the key is available.
	desc, ok := info.SoftForks.Bip9SoftForks[forkKey]
	if !ok {
		_, _, line, _ := runtime.Caller(1)
		t.Fatalf("assertion failed at line %d: softfork status for %q "+
			"is not in getblockchaininfo results", line, forkKey)
	}

	// Ensure the status it the expected value.
	if desc.Status != status {
		_, _, line, _ := runtime.Caller(1)
		t.Fatalf("assertion failed at line %d: softfork status for %q "+
			"is %v instead of expected %v", line, forkKey,
			desc.Status, status)
	}
}

// testBIP0009 ensures the BIP0009 soft fork mechanism follows the state
// transition rules set forth by the BIP for the provided soft fork key.  It
// uses the regression test network to signal support and advance through the
// various threshold states including failure to achieve locked in status.
//
// See TestBIP0009 for an overview of what is tested.
//
// NOTE: This only differs from the exported version in that it accepts the
// specific soft fork deployment to test.
func testBIP0009(t *testing.T, forkKey string, deploymentID uint32) {
	// Initialize the primary mining node with only the genesis block.
	r, err := rpctest.New(&chaincfg.RegressionNetParams, nil, nil, "")
	if err != nil {
		t.Fatalf("unable to create primary harness: %v", err)
	}
	if err := r.SetUp(false, 0); err != nil {
		t.Fatalf("unable to setup test chain: %v", err)
	}
	defer r.TearDown()

	// *** ThresholdDefined ***
	//
	// Assert the chain height is the expected value and the soft fork
	// status starts out as defined.
	assertChainHeight(r, t, 0)
	assertSoftForkStatus(r, t, forkKey, blockchain.ThresholdDefined)

	// *** ThresholdDefined part 2 - 1 block prior to ThresholdStarted ***
	//
	// Generate enough blocks to reach the height just before the first
	// state transition without signalling support since the state should
	// move to started once the start time has been reached regardless of
	// support signalling.
	//
	// NOTE: This is two blocks before the confirmation window because the
	// getblockchaininfo RPC reports the status for the block AFTER the
	// current one.  All of the heights below are thus offset by one to
	// compensate.
	//
	// Assert the chain height is the expected value and soft fork status is
	// still defined and did NOT move to started.
	confirmationWindow := r.ActiveNet.MinerConfirmationWindow
	for i := uint32(0); i < confirmationWindow-2; i++ {
		_, err := r.GenerateAndSubmitBlock(nil, vbLegacyBlockVersion,
			time.Time{})
		if err != nil {
			t.Fatalf("failed to generated block %d: %v", i, err)
		}
	}
	assertChainHeight(r, t, confirmationWindow-2)
	assertSoftForkStatus(r, t, forkKey, blockchain.ThresholdDefined)

	// *** ThresholdStarted ***
	//
	// Generate another block to reach the next window.
	//
	// Assert the chain height is the expected value and the soft fork
	// status is started.
	_, err = r.GenerateAndSubmitBlock(nil, vbLegacyBlockVersion, time.Time{})
	if err != nil {
		t.Fatalf("failed to generated block: %v", err)
	}
	assertChainHeight(r, t, confirmationWindow-1)
	assertSoftForkStatus(r, t, forkKey, blockchain.ThresholdStarted)

	// *** ThresholdStarted part 2 - Fail to achieve ThresholdLockedIn ***
	//
	// Generate enough blocks to reach the next window in such a way that
	// the number blocks with the version bit set to signal support is 1
	// less than required to achieve locked in status.
	//
	// Assert the chain height is the expected value and the soft fork
	// status is still started and did NOT move to locked in.
	if deploymentID > uint32(len(r.ActiveNet.Deployments)) {
		t.Fatalf("deployment ID %d does not exist", deploymentID)
	}
	deployment := &r.ActiveNet.Deployments[deploymentID]
	activationThreshold := r.ActiveNet.RuleChangeActivationThreshold
	signalForkVersion := int32(1<<deployment.BitNumber) | vbTopBits
	for i := uint32(0); i < activationThreshold-1; i++ {
		_, err := r.GenerateAndSubmitBlock(nil, signalForkVersion,
			time.Time{})
		if err != nil {
			t.Fatalf("failed to generated block %d: %v", i, err)
		}
	}
	for i := uint32(0); i < confirmationWindow-(activationThreshold-1); i++ {
		_, err := r.GenerateAndSubmitBlock(nil, vbLegacyBlockVersion,
			time.Time{})
		if err != nil {
			t.Fatalf("failed to generated block %d: %v", i, err)
		}
	}
	assertChainHeight(r, t, (confirmationWindow*2)-1)
	assertSoftForkStatus(r, t, forkKey, blockchain.ThresholdStarted)

	// *** ThresholdLockedIn ***
	//
	// Generate enough blocks to reach the next window in such a way that
	// the number blocks with the version bit set to signal support is
	// exactly the number required to achieve locked in status.
	//
	// Assert the chain height is the expected value and the soft fork
	// status moved to locked in.
	for i := uint32(0); i < activationThreshold; i++ {
		_, err := r.GenerateAndSubmitBlock(nil, signalForkVersion,
			time.Time{})
		if err != nil {
			t.Fatalf("failed to generated block %d: %v", i, err)
		}
	}
	for i := uint32(0); i < confirmationWindow-activationThreshold; i++ {
		_, err := r.GenerateAndSubmitBlock(nil, vbLegacyBlockVersion,
			time.Time{})
		if err != nil {
			t.Fatalf("failed to generated block %d: %v", i, err)
		}
	}
	assertChainHeight(r, t, (confirmationWindow*3)-1)
	assertSoftForkStatus(r, t, forkKey, blockchain.ThresholdLockedIn)

	// *** ThresholdLockedIn part 2 -- 1 block prior to ThresholdActive ***
	//
	// Generate enough blocks to reach the height just before the next
	// window without continuing to signal support since it is already
	// locked in.
	//
	// Assert the chain height is the expected value and the soft fork
	// status is still locked in and did NOT move to active.
	for i := uint32(0); i < confirmationWindow-1; i++ {
		_, err := r.GenerateAndSubmitBlock(nil, vbLegacyBlockVersion,
			time.Time{})
		if err != nil {
			t.Fatalf("failed to generated block %d: %v", i, err)
		}
	}
	assertChainHeight(r, t, (confirmationWindow*4)-2)
	assertSoftForkStatus(r, t, forkKey, blockchain.ThresholdLockedIn)

	// *** ThresholdActive ***
	//
	// Generate another block to reach the next window without continuing to
	// signal support since it is already locked in.
	//
	// Assert the chain height is the expected value and the soft fork
	// status moved to active.
	_, err = r.GenerateAndSubmitBlock(nil, vbLegacyBlockVersion, time.Time{})
	if err != nil {
		t.Fatalf("failed to generated block: %v", err)
	}
	assertChainHeight(r, t, (confirmationWindow*4)-1)
	assertSoftForkStatus(r, t, forkKey, blockchain.ThresholdActive)
}

// TestBIP0009 ensures the BIP0009 soft fork mechanism follows the state
// transition rules set forth by the BIP for all soft forks.  It uses the
// regression test network to signal support and advance through the various
// threshold states including failure to achieve locked in status.
//
// Overview:
// - Assert the chain height is 0 and the state is ThresholdDefined
// - Generate 1 fewer blocks than needed to reach the first state transition
//   - Assert chain height is expected and state is still ThresholdDefined
// - Generate 1 more block to reach the first state transition
//   - Assert chain height is expected and state moved to ThresholdStarted
// - Generate enough blocks to reach the next state transition window, but only
//   signal support in 1 fewer than the required number to achieve
//   ThresholdLockedIn
//   - Assert chain height is expected and state is still ThresholdStarted
// - Generate enough blocks to reach the next state transition window with only
//   the exact number of blocks required to achieve locked in status signalling
//   support.
//   - Assert chain height is expected and state moved to ThresholdLockedIn
// - Generate 1 fewer blocks than needed to reach the next state transition
//   - Assert chain height is expected and state is still ThresholdLockedIn
// - Generate 1 more block to reach the next state transition
//   - Assert chain height is expected and state moved to ThresholdActive
func TestBIP0009(t *testing.T) {
	t.Parallel()

	testBIP0009(t, "dummy", chaincfg.DeploymentTestDummy)
	testBIP0009(t, "segwit", chaincfg.DeploymentSegwit)
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
	r, err := rpctest.New(&chaincfg.SimNetParams, nil, nil, "")
	if err != nil {
		t.Fatalf("unable to create primary harness: %v", err)
	}
	if err := r.SetUp(true, 0); err != nil {
		t.Fatalf("unable to setup test chain: %v", err)
	}
	defer r.TearDown()

	// Assert the chain only consists of the gensis block.
	assertChainHeight(r, t, 0)

	// *** ThresholdDefined ***
	//
	// Generate a block that extends the genesis block.  It should not have
	// the test dummy bit set in the version since the first window is
	// in the defined threshold state.
	deployment := &r.ActiveNet.Deployments[chaincfg.DeploymentTestDummy]
	testDummyBitNum := deployment.BitNumber
	hashes, err := r.Client.Generate(1)
	if err != nil {
		t.Fatalf("unable to generate blocks: %v", err)
	}
	assertChainHeight(r, t, 1)
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
	hashes, err = r.Client.Generate(numNeeded)
	if err != nil {
		t.Fatalf("failed to generated %d blocks: %v", numNeeded, err)
	}
	assertChainHeight(r, t, confirmationWindow)
	assertVersionBit(r, t, hashes[len(hashes)-2], testDummyBitNum, false)
	assertVersionBit(r, t, hashes[len(hashes)-1], testDummyBitNum, true)

	// *** ThresholdLockedIn ***
	//
	// Generate enough blocks to reach the next state transition.
	//
	// The last generated block should still have the test bit set in the
	// version since the btcd mining code will have recognized the test
	// dummy deployment as locked in.
	hashes, err = r.Client.Generate(confirmationWindow)
	if err != nil {
		t.Fatalf("failed to generated %d blocks: %v", confirmationWindow,
			err)
	}
	assertChainHeight(r, t, confirmationWindow*2)
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
	hashes, err = r.Client.Generate(confirmationWindow)
	if err != nil {
		t.Fatalf("failed to generated %d blocks: %v", confirmationWindow,
			err)
	}
	assertChainHeight(r, t, confirmationWindow*3)
	assertVersionBit(r, t, hashes[len(hashes)-2], testDummyBitNum, true)
	assertVersionBit(r, t, hashes[len(hashes)-1], testDummyBitNum, false)
}
