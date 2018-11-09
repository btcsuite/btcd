// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package harness

import (
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// TestNode wraps optional test node implementations for different test setups
type TestNode interface {
	// Network returns current network of the node
	Network() *chaincfg.Params

	// Start node process
	Start(args *TestNodeStartArgs)

	// Stop node process
	Stop()

	// Dispose releases all resources allocated by the node
	// This action is final (irreversible)
	Dispose() error

	// CertFile returns file path of the .cert-file of the node
	CertFile() string

	// RPCConnectionConfig produces a new connection config instance for RPC client
	RPCConnectionConfig() *rpcclient.ConnConfig

	// RPCClient returns node RPCConnection
	RPCClient() *RPCConnection

	// P2PAddress returns node p2p address
	P2PAddress() string

	GenerateAndSubmitBlock(args GenerateBlockArgs) (*btcutil.Block, error)

	GenerateAndSubmitBlockWithCustomCoinbaseOutputs(args GenerateBlockArgs) (*btcutil.Block, error)
}

// TestNodeFactory produces a new TestNode instance
type TestNodeFactory interface {
	// NewNode is used by harness builder to setup a node component
	NewNode(cfg *TestNodeConfig) TestNode
}

// TestNodeConfig bundles settings required to create a new node instance
type TestNodeConfig struct {
	ActiveNet *chaincfg.Params

	WorkingDir string

	P2PHost string
	P2PPort int

	NodeRPCHost string
	NodeRPCPort int
}

// GenerateBlockArgs bundles GenerateBlock() arguments to minimize diff
// in case a new argument for the function is added
type GenerateBlockArgs struct {
	Txns         []*btcutil.Tx
	BlockVersion int32
	BlockTime    time.Time
	MineTo       []wire.TxOut
}

// TestNodeStartArgs bundles Start() arguments to minimize diff
// in case a new argument for the function is added
type TestNodeStartArgs struct {
	ExtraArguments map[string]interface{}
	DebugOutput    bool
	MiningAddress  btcutil.Address
}
