// Copyright (c) 2015-2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math/rand"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
)

// countingDB wraps a database.DB and counts the number of Update calls.
type countingDB struct {
	database.DB
	updates int
}

// Update increments the updates counter on a call.
func (c *countingDB) Update(fn func(tx database.Tx) error) error {
	c.updates++
	return c.DB.Update(fn)
}

// TestFlushToDB tests that flushToDB only opens a write transaction when at
// least one dirty node has block data and skips the transaction when all dirty
// nodes are header-only.
func TestFlushToDB(t *testing.T) {
	tests := []struct {
		name string

		// statuses defines the dirty nodes to create for this test
		// case. Each entry's status determines whether the node is
		// header-only or has block data. A nil slice means no nodes
		// are added (empty dirty set).
		statuses []blockStatus

		// wantUpdates is the expected number of DB Update calls.
		wantUpdates int
	}{
		{
			name:        "empty dirty set",
			statuses:    nil,
			wantUpdates: 0,
		},
		{
			name:        "single header-only node",
			statuses:    []blockStatus{statusHeaderStored},
			wantUpdates: 0,
		},
		{
			name: "multiple header-only nodes",
			statuses: []blockStatus{
				statusHeaderStored,
				statusHeaderStored,
				statusHeaderStored,
			},
			wantUpdates: 0,
		},
		{
			name:        "single data node",
			statuses:    []blockStatus{statusDataStored | statusHeaderStored},
			wantUpdates: 1,
		},
		{
			name: "header-only and data nodes mixed",
			statuses: []blockStatus{
				statusHeaderStored,
				statusDataStored | statusHeaderStored,
			},
			wantUpdates: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chain, teardown, err := chainSetup(
				"flushtodbtest", &chaincfg.SimNetParams,
			)
			if err != nil {
				t.Fatalf("failed to setup chain: %v", err)
			}
			defer teardown()

			bi := chain.index
			cdb := &countingDB{DB: bi.db}
			bi.db = cdb

			// Create the dirty nodes for this test case, chaining
			// each off the genesis tip.
			tip := chain.bestChain.Tip()
			var nodes []*blockNode
			for i, status := range test.statuses {
				node := newBlockNode(&wire.BlockHeader{
					PrevBlock: tip.hash,
					Nonce:     uint32(i),
				}, tip)
				node.status = status
				bi.AddNode(node)
				nodes = append(nodes, node)
				tip = node
			}

			err = bi.flushToDB()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cdb.updates != test.wantUpdates {
				t.Fatalf("expected %d Update calls, got %d",
					test.wantUpdates, cdb.updates)
			}

			bi.RLock()
			dirtyLen := len(bi.dirty)
			bi.RUnlock()

			if dirtyLen != 0 {
				t.Fatalf("expected dirty set to be empty, got %d",
					dirtyLen)
			}

			// Nodes with block data should be in the DB;
			// header-only nodes should not.
			for i, node := range nodes {
				var found bool
				err := bi.db.View(func(dbTx database.Tx) error {
					bucket := dbTx.Metadata().Bucket(blockIndexBucketName)
					key := blockIndexKey(&node.hash, uint32(node.height))
					found = bucket.Get(key) != nil
					return nil
				})
				if err != nil {
					t.Fatalf("node %d: View failed: %v", i, err)
				}

				wantInDB := node.status.HaveData()
				if found != wantInDB {
					t.Fatalf("node %d: in database = %v, want %v",
						i, found, wantInDB)
				}
			}
		})
	}
}

func TestAncestor(t *testing.T) {
	height := 500_000
	blockNodes := chainedNodes(nil, height)

	for i, blockNode := range blockNodes {
		// Grab a random node that's a child of this node
		// and try to fetch the current blockNode with Ancestor.
		randNode := blockNodes[rand.Intn(height-i)+i]
		got := randNode.Ancestor(blockNode.height)

		// See if we got the right one.
		if got.hash != blockNode.hash {
			t.Fatalf("expected ancestor at height %d "+
				"but got a node at height %d",
				blockNode.height, got.height)
		}

		// Gensis doesn't have ancestors so skip the check below.
		if blockNode.height == 0 {
			continue
		}

		// The ancestors are deterministic so check that this node's
		// ancestor is the correct one.
		if blockNode.ancestor.height != getAncestorHeight(blockNode.height) {
			t.Fatalf("expected anestor at height %d, but it was at %d",
				getAncestorHeight(blockNode.height),
				blockNode.ancestor.height)
		}
	}
}
