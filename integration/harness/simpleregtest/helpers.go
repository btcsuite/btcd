// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package simpleregtest

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/btcsuite/btcd/integration"
	"github.com/btcsuite/btcd/integration/harness"
	"github.com/btcsuite/btcd/rpcclient"
)

// JoinType is an enum representing a particular type of "node join". A node
// join is a synchronization tool used to wait until a subset of nodes have a
// consistent state with respect to an attribute.
type JoinType uint8

const (
	// Blocks is a JoinType which waits until all nodes share the same
	// block height.
	Blocks JoinType = iota

	// Mempools is a JoinType which blocks until all nodes have identical
	// mempool.
	Mempools
)

// JoinNodes is a synchronization tool used to block until all passed nodes are
// fully synced with respect to an attribute. This function will block for a
// period of time, finally returning once all nodes are synced according to the
// passed JoinType. This function be used to to ensure all active test
// harnesses are at a consistent state before proceeding to an assertion or
// check within rpc tests.
func JoinNodes(nodes []*harness.Harness, joinType JoinType) error {
	switch joinType {
	case Blocks:
		return syncBlocks(nodes)
	case Mempools:
		return syncMempools(nodes)
	}
	return nil
}

// syncMempools blocks until all nodes have identical mempools.
func syncMempools(nodes []*harness.Harness) error {
	poolsMatch := false

	for !poolsMatch {
	retry:
		firstPool, err := nodes[0].NodeRPCClient().GetRawMempool()
		if err != nil {
			return err
		}

		// If all nodes have an identical mempool with respect to the
		// first node, then we're done. Otherwise, drop back to the top
		// of the loop and retry after a short wait period.
		for _, node := range nodes[1:] {
			nodePool, err := node.NodeRPCClient().GetRawMempool()
			if err != nil {
				return err
			}

			if !reflect.DeepEqual(firstPool, nodePool) {
				time.Sleep(time.Millisecond * 100)
				goto retry
			}
		}

		poolsMatch = true
	}

	return nil
}

// syncBlocks blocks until all nodes report the same block height.
func syncBlocks(nodes []*harness.Harness) error {
	blocksMatch := false

	for !blocksMatch {
	retry:
		blockHeights := make(map[int64]struct{})

		for _, node := range nodes {
			blockHeight, err := node.NodeRPCClient().GetBlockCount()
			if err != nil {
				return err
			}

			blockHeights[blockHeight] = struct{}{}
			if len(blockHeights) > 1 {
				time.Sleep(time.Millisecond * 100)
				goto retry
			}
		}

		blocksMatch = true
	}

	return nil
}

// ConnectNode establishes a new peer-to-peer connection between the "from"
// harness and the "to" harness.  The connection made is flagged as persistent,
// therefore in the case of disconnects, "from" will attempt to reestablish a
// connection to the "to" harness.
func ConnectNode(from *harness.Harness, to *harness.Harness) error {
	peerInfo, err := from.NodeRPCClient().GetPeerInfo()
	if err != nil {
		return err
	}
	numPeers := len(peerInfo)

	targetAddr := to.P2PAddress()
	if err := from.NodeRPCClient().AddNode(targetAddr, rpcclient.ANAdd); err != nil {
		return err
	}

	// Block until a new connection has been established.
	for attempts := 5; attempts > 0; attempts-- {
		peerInfo, err = from.NodeRPCClient().GetPeerInfo()
		if err != nil {
			return err
		}
		if len(peerInfo) > numPeers {
			return nil
		}
		integration.Sleep(1000)
	}

	return fmt.Errorf("failed to connet node")
}

// Create a test chain with the desired number of mature coinbase outputs
func generateTestChain(numToGenerate uint32, node *rpcclient.Client) error {
	fmt.Printf("Generating %v blocks...\n", numToGenerate)
	_, err := node.Generate(numToGenerate)
	if err != nil {
		return err
	}
	fmt.Println("Block generation complete.")
	return nil
}

func assertConnectedTo(t *testing.T, nodeA *harness.Harness, nodeB *harness.Harness) {
	nodeAPeers, err := nodeA.NodeRPCClient().GetPeerInfo()
	if err != nil {
		t.Fatalf("unable to get nodeA's peer info")
	}

	nodeAddr := nodeB.P2PAddress()
	addrFound := false
	for _, peerInfo := range nodeAPeers {
		if peerInfo.Addr == nodeAddr {
			addrFound = true
			break
		}
	}

	if !addrFound {
		t.Fatal("nodeA not connected to nodeB")
	}
}

// Waits for wallet to sync to the target height
func syncWalletTo(rpcClient *rpcclient.Client, desiredHeight int64) (int64, error) {
	var count int64
	var err error
	for count != desiredHeight {
		//rpctest.Sleep(100)
		count, err = rpcClient.GetBlockCount()
		if err != nil {
			return -1, err
		}
		fmt.Println("   sync to: " + strconv.FormatInt(count, 10))
	}
	return count, nil
}
