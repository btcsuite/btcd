// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package simpleregtest

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/integration"
	"github.com/btcsuite/btcd/integration/harness"
	"github.com/btcsuite/btcutil"
)

// DeploySimpleChain defines harness setup sequence for this package:
// 1. obtains a new mining wallet address
// 2. restart harness node and wallet with the new mining address
// 3. builds a new chain with the target number of mature outputs
// receiving the mining reward to the test wallet
// 4. syncs wallet to the tip of the chain
func DeploySimpleChain(testSetup *ChainWithMatureOutputsSpawner, h *harness.Harness) {
	integration.AssertNotEmpty("harness name", h.Name)
	fmt.Println("Deploying Harness[" + h.Name + "]")

	// launch a fresh h (assumes h working dir is empty)
	{
		args := &launchArguments{
			DebugNodeOutput:    testSetup.DebugNodeOutput,
			DebugWalletOutput:  testSetup.DebugWalletOutput,
			NodeExtraArguments: testSetup.NodeStartExtraArguments,
		}
		launchHarnessSequence(h, args)
	}

	// Get a new address from the WalletTestServer
	// to be set with node --miningaddr
	{
		address, err := h.Wallet.NewAddress(nil)
		integration.CheckTestSetupMalfunction(err)
		h.MiningAddress = address

		integration.AssertNotNil("MiningAddress", h.MiningAddress)
		integration.AssertNotEmpty("MiningAddress", h.MiningAddress.String())

		fmt.Println("Mining address: " + h.MiningAddress.String())
	}

	// restart the h with the new argument
	{
		shutdownHarnessSequence(h)

		args := &launchArguments{
			DebugNodeOutput:    testSetup.DebugNodeOutput,
			DebugWalletOutput:  testSetup.DebugWalletOutput,
			NodeExtraArguments: testSetup.NodeStartExtraArguments,
		}
		launchHarnessSequence(h, args)
	}

	{
		if testSetup.NumMatureOutputs > 0 {
			numToGenerate := uint32(testSetup.ActiveNet.CoinbaseMaturity) + testSetup.NumMatureOutputs
			err := generateTestChain(numToGenerate, h.NodeRPCClient())
			integration.CheckTestSetupMalfunction(err)
		}
		// wait for the WalletTestServer to sync up to the current height
		h.Wallet.Sync()
	}
	fmt.Println("Harness[" + h.Name + "] is ready")
}

// local struct to bundle launchHarnessSequence function arguments
type launchArguments struct {
	DebugNodeOutput    bool
	DebugWalletOutput  bool
	MiningAddress      *btcutil.Address
	NodeExtraArguments map[string]interface{}
}

// launchHarnessSequence
func launchHarnessSequence(h *harness.Harness, args *launchArguments) {
	node := h.Node
	wallet := h.Wallet

	nodeLaunchArguments := &harness.TestNodeStartArgs{
		DebugOutput:    args.DebugNodeOutput,
		MiningAddress:  h.MiningAddress,
		ExtraArguments: args.NodeExtraArguments,
	}
	node.Start(nodeLaunchArguments)

	rpcConfig := node.RPCConnectionConfig()

	walletLaunchArguments := &harness.TestWalletStartArgs{
		NodeRPCCertFile:          node.CertFile(),
		DebugWalletOutput:        args.DebugWalletOutput,
		MaxSecondsToWaitOnLaunch: 90,
		NodeRPCConfig:            rpcConfig,
	}
	wallet.Start(walletLaunchArguments)

}

// shutdownHarnessSequence reverses the launchHarnessSequence
func shutdownHarnessSequence(harness *harness.Harness) {
	harness.Wallet.Stop()
	harness.Node.Stop()
}

// extractSeedSaltFromHarnessName tries to split harness name string
// at `.`-character and parse the second part as a uint32 number.
// Otherwise returns default value.
func extractSeedSaltFromHarnessName(harnessName string) uint32 {
	parts := strings.Split(harnessName, ".")
	if len(parts) != 2 {
		// no salt specified, return default value
		return 0
	}
	seedString := parts[1]
	tmp, err := strconv.Atoi(seedString)
	seedNonce := uint32(tmp)
	integration.CheckTestSetupMalfunction(err)
	return seedNonce
}
