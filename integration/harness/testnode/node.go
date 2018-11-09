// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package testnode

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/integration"
	"github.com/btcsuite/btcd/integration/commandline"
	"github.com/btcsuite/btcd/integration/harness"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcutil"
)

// DefaultTestNode launches a new btcd instance using command-line call.
// Implements harness.TestNode.
type DefaultTestNode struct {
	// NodeExecutablePathProvider returns path to the btcd executable
	NodeExecutablePathProvider commandline.ExecutablePathProvider

	rpcUser    string
	rpcPass    string
	p2pAddress string
	rpcListen  string
	rpcConnect string
	profile    string
	debugLevel string
	appDir     string
	endpoint   string

	externalProcess *commandline.ExternalProcess

	rPCClient *harness.RPCConnection

	miningAddress btcutil.Address

	network *chaincfg.Params
}

// RPCConnectionConfig produces a new connection config instance for RPC client
func (node *DefaultTestNode) RPCConnectionConfig() *rpcclient.ConnConfig {
	file := node.CertFile()
	fmt.Println("reading: " + file)
	cert, err := ioutil.ReadFile(file)
	integration.CheckTestSetupMalfunction(err)

	return &rpcclient.ConnConfig{
		Host:                 node.rpcListen,
		Endpoint:             node.endpoint,
		User:                 node.rpcUser,
		Pass:                 node.rpcPass,
		Certificates:         cert,
		DisableAutoReconnect: true,
		HTTPPostMode:         false,
	}
}

// FullConsoleCommand returns the full console command used to
// launch external process of the node
func (node *DefaultTestNode) FullConsoleCommand() string {
	return node.externalProcess.FullConsoleCommand()
}

// P2PAddress returns node p2p address
func (node *DefaultTestNode) P2PAddress() string {
	return node.p2pAddress
}

// RPCClient returns node RPCConnection
func (node *DefaultTestNode) RPCClient() *harness.RPCConnection {
	return node.rPCClient
}

// CertFile returns file path of the .cert-file of the node
func (node *DefaultTestNode) CertFile() string {
	return filepath.Join(node.appDir, "rpc.cert")
}

// KeyFile returns file path of the .key-file of the node
func (node *DefaultTestNode) KeyFile() string {
	return filepath.Join(node.appDir, "rpc.key")
}

// Network returns current network of the node
func (node *DefaultTestNode) Network() *chaincfg.Params {
	return node.network
}

// IsRunning returns true if DefaultTestNode is running external btcd process
func (node *DefaultTestNode) IsRunning() bool {
	return node.externalProcess.IsRunning()
}

// Start node process. Deploys working dir, launches btcd using command-line,
// connects RPC client to the node.
func (node *DefaultTestNode) Start(args *harness.TestNodeStartArgs) {
	if node.IsRunning() {
		integration.ReportTestSetupMalfunction(fmt.Errorf("DefaultTestNode is already running"))
	}
	fmt.Println("Start node process...")
	integration.MakeDirs(node.appDir)

	node.miningAddress = args.MiningAddress

	exec := node.NodeExecutablePathProvider.Executable()
	node.externalProcess.CommandName = exec
	node.externalProcess.Arguments = commandline.ArgumentsToStringArray(
		node.cookArguments(args.ExtraArguments),
	)
	node.externalProcess.Launch(args.DebugOutput)
	// Node RPC instance will create a cert file when it is ready for incoming calls
	integration.WaitForFile(node.CertFile(), 7)

	fmt.Println("Connect to node RPC...")
	cfg := node.RPCConnectionConfig()
	node.rPCClient.Connect(cfg, nil)
	fmt.Println("node RPC client connected.")
}

// Stop interrupts the running node process.
// Disconnects RPC client from the node, removes cert-files produced by the btcd,
// stops btcd process.
func (node *DefaultTestNode) Stop() {
	if !node.IsRunning() {
		integration.ReportTestSetupMalfunction(fmt.Errorf("node is not running"))
	}

	if node.rPCClient.IsConnected() {
		fmt.Println("Disconnect from node RPC...")
		node.rPCClient.Disconnect()
	}

	fmt.Println("Stop node process...")
	err := node.externalProcess.Stop()
	integration.CheckTestSetupMalfunction(err)

	// Delete files, RPC servers will recreate them on the next launch sequence
	integration.DeleteFile(node.CertFile())
	integration.DeleteFile(node.KeyFile())
}

// Dispose simply stops the node process if running
func (node *DefaultTestNode) Dispose() error {
	if node.IsRunning() {
		node.Stop()
	}
	return nil
}

// cookArguments prepares arguments for the command-line call
func (node *DefaultTestNode) cookArguments(extraArguments map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	result["txindex"] = commandline.NoArgumentValue
	result["addrindex"] = commandline.NoArgumentValue
	result["rpcuser"] = node.rpcUser
	result["rpcpass"] = node.rpcPass
	result["rpcconnect"] = node.rpcConnect
	result["rpclisten"] = node.rpcListen
	result["listen"] = node.p2pAddress
	result["datadir"] = node.appDir
	result["debuglevel"] = node.debugLevel
	result["profile"] = node.profile
	result["rpccert"] = node.CertFile()
	result["rpckey"] = node.KeyFile()
	if node.miningAddress != nil {
		result["miningaddr"] = node.miningAddress.String()
	}
	result[networkFor(node.network)] = commandline.NoArgumentValue

	commandline.ArgumentsCopyTo(extraArguments, result)
	return result
}

// networkFor resolves network argument for node and wallet console commands
func networkFor(net *chaincfg.Params) string {
	if net == &chaincfg.SimNetParams {
		return "simnet"
	}
	if net == &chaincfg.TestNet3Params {
		return "testnet"
	}
	if net == &chaincfg.RegressionNetParams {
		return "regtest"
	}
	if net == &chaincfg.MainNetParams {
		// no argument needed for the MainNet
		return commandline.NoArgument
	}

	// should never reach this line, report violation
	integration.ReportTestSetupMalfunction(fmt.Errorf("unknown network: %v ", net))
	return ""
}

// GenerateAndSubmitBlock creates a block whose contents include the passed
// transactions and submits it to the running simnet node. For generating
// blocks with only a coinbase tx, callers can simply pass nil instead of
// transactions to be mined. Additionally, a custom block version can be set by
// the caller. An uninitialized time.Time should be used for the
// blockTime parameter if one doesn't wish to set a custom time.
func (node *DefaultTestNode) GenerateAndSubmitBlock(args harness.GenerateBlockArgs) (*btcutil.Block, error) {
	integration.AssertTrue("args.MineTo is empty", len(args.MineTo) == 0)
	return node.GenerateAndSubmitBlockWithCustomCoinbaseOutputs(args)
}

// GenerateAndSubmitBlockWithCustomCoinbaseOutputs creates a block whose
// contents include the passed coinbase outputs and transactions and submits
// it to the running simnet node. For generating blocks with only a coinbase tx,
// callers can simply pass nil instead of transactions to be mined.
// Additionally, a custom block version can be set by the caller. A blockVersion
// of -1 indicates that the current default block version should be used. An
// uninitialized time.Time should be used for the blockTime parameter if one
// doesn't wish to set a custom time. The mineTo list of outputs will be added
// to the coinbase; this is not checked for correctness until the block is
// submitted; thus, it is the caller's responsibility to ensure that the outputs
// are correct. If the list is empty, the coinbase reward goes to the wallet
// managed by the Harness.
func (node *DefaultTestNode) GenerateAndSubmitBlockWithCustomCoinbaseOutputs(args harness.GenerateBlockArgs) (*btcutil.Block, error) {
	txns := args.Txns
	blockVersion := args.BlockVersion
	integration.AssertTrue(fmt.Sprintf("Incorrect blockVersion(%v)", blockVersion), blockVersion > 0)
	blockTime := args.BlockTime
	mineTo := args.MineTo

	integration.AssertTrue("blockVersion != -1", blockVersion != -1)

	prevBlockHash, prevBlockHeight, err := node.RPCClient().Connection().GetBestBlock()
	if err != nil {
		return nil, err
	}
	mBlock, err := node.RPCClient().Connection().GetBlock(prevBlockHash)
	if err != nil {
		return nil, err
	}
	prevBlock := btcutil.NewBlock(mBlock)
	prevBlock.SetHeight(prevBlockHeight)

	// Create a new block including the specified transactions
	newBlock, err := CreateBlock(prevBlock, txns, blockVersion,
		blockTime, node.miningAddress, mineTo, node.network)
	if err != nil {
		return nil, err
	}

	// Submit the block to the simnet node.
	if err := node.RPCClient().Connection().SubmitBlock(newBlock, nil); err != nil {
		return nil, err
	}

	return newBlock, nil
}
