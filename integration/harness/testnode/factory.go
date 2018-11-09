// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package testnode

import (
	"net"
	"path/filepath"
	"strconv"

	"github.com/btcsuite/btcd/integration"
	"github.com/btcsuite/btcd/integration/commandline"
	"github.com/btcsuite/btcd/integration/harness"
)

// NodeFactory produces a new DefaultTestNode-instance upon request
type NodeFactory struct {
	// NodeExecutablePathProvider returns path to the btcd executable
	NodeExecutablePathProvider commandline.ExecutablePathProvider
}

// NewNode creates and returns a fully initialized instance of the DefaultTestNode.
func (factory *NodeFactory) NewNode(config *harness.TestNodeConfig) harness.TestNode {
	exec := factory.NodeExecutablePathProvider

	integration.AssertNotNil("NodeExecutablePathProvider", exec)
	integration.AssertNotNil("WorkingDir", config.WorkingDir)
	integration.AssertNotEmpty("WorkingDir", config.WorkingDir)

	node := &DefaultTestNode{
		p2pAddress:                 net.JoinHostPort(config.P2PHost, strconv.Itoa(config.P2PPort)),
		rpcListen:                  net.JoinHostPort(config.NodeRPCHost, strconv.Itoa(config.NodeRPCPort)),
		rpcUser:                    "user",
		rpcPass:                    "pass",
		appDir:                     filepath.Join(config.WorkingDir, "btcd"),
		endpoint:                   "ws",
		externalProcess:            &commandline.ExternalProcess{CommandName: "btcd"},
		rPCClient:                  &harness.RPCConnection{MaxConnRetries: 20},
		NodeExecutablePathProvider: exec,
		network:                    config.ActiveNet,
	}
	return node
}
