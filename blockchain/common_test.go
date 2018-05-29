// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	mrand "math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ffldb"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

const (
	// testDbType is the database backend type to use for the tests.
	testDbType = "ffldb"

	// testDbRoot is the root directory used to create all test databases.
	testDbRoot = "testdbs"

	// blockDataNet is the expected network in the test block data.
	blockDataNet = wire.MainNet
)

// filesExists returns whether or not the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// isSupportedDbType returns whether or not the passed database type is
// currently supported.
func isSupportedDbType(dbType string) bool {
	supportedDrivers := database.SupportedDrivers()
	for _, driver := range supportedDrivers {
		if dbType == driver {
			return true
		}
	}

	return false
}

// chainSetup is used to create a new db and chain instance with the genesis
// block already inserted.  In addition to the new chain instance, it returns
// a teardown function the caller should invoke when done testing to clean up.
func chainSetup(dbName string, params *chaincfg.Params) (*BlockChain, func(), error) {
	if !isSupportedDbType(testDbType) {
		return nil, nil, fmt.Errorf("unsupported db type %v", testDbType)
	}

	// Handle memory database specially since it doesn't need the disk
	// specific handling.
	var db database.DB
	var teardown func()
	if testDbType == "memdb" {
		ndb, err := database.Create(testDbType)
		if err != nil {
			return nil, nil, fmt.Errorf("error creating db: %v", err)
		}
		db = ndb

		// Setup a teardown function for cleaning up.  This function is
		// returned to the caller to be invoked when it is done testing.
		teardown = func() {
			db.Close()
		}
	} else {
		// Create the root directory for test databases.
		if !fileExists(testDbRoot) {
			if err := os.MkdirAll(testDbRoot, 0700); err != nil {
				err := fmt.Errorf("unable to create test db "+
					"root: %v", err)
				return nil, nil, err
			}
		}

		// Create a new database to store the accepted blocks into.
		dbPath := filepath.Join(testDbRoot, dbName)
		_ = os.RemoveAll(dbPath)
		ndb, err := database.Create(testDbType, dbPath, blockDataNet)
		if err != nil {
			return nil, nil, fmt.Errorf("error creating db: %v", err)
		}
		db = ndb

		// Setup a teardown function for cleaning up.  This function is
		// returned to the caller to be invoked when it is done testing.
		teardown = func() {
			db.Close()
			os.RemoveAll(dbPath)
			os.RemoveAll(testDbRoot)
		}
	}

	// Copy the chain params to ensure any modifications the tests do to
	// the chain parameters do not affect the global instance.
	paramsCopy := *params

	// Create the main chain instance.
	chain, err := New(&Config{
		DB:          db,
		ChainParams: &paramsCopy,
		TimeSource:  NewMedianTime(),
		SigCache:    txscript.NewSigCache(1000),
	})

	if err != nil {
		teardown()
		err := fmt.Errorf("failed to create chain instance: %v", err)
		return nil, nil, err
	}

	return chain, teardown, nil
}

// newFakeChain returns a chain that is usable for syntetic tests.  It is
// important to note that this chain has no database associated with it, so
// it is not usable with all functions and the tests must take care when making
// use of it.
func newFakeChain(params *chaincfg.Params) *BlockChain {
	// Create a genesis block node and block index populated with it for use
	// when creating the fake chain below.
	node := newBlockNode(&params.GenesisBlock.Header, nil)
	node.inMainChain = true
	index := newBlockIndex(nil, params)
	index.AddNode(node)
	mainNodesByHeight := make(map[int64]*blockNode)
	mainNodesByHeight[node.height] = node

	return &BlockChain{
		chainParams:                   params,
		deploymentCaches:              newThresholdCaches(params),
		bestNode:                      node,
		index:                         index,
		mainNodesByHeight:             mainNodesByHeight,
		isVoterMajorityVersionCache:   make(map[[stakeMajorityCacheKeySize]byte]bool),
		isStakeMajorityVersionCache:   make(map[[stakeMajorityCacheKeySize]byte]bool),
		calcPriorStakeVersionCache:    make(map[[chainhash.HashSize]byte]uint32),
		calcVoterVersionIntervalCache: make(map[[chainhash.HashSize]byte]uint32),
		calcStakeVersionCache:         make(map[[chainhash.HashSize]byte]uint32),
	}
}

// testNoncePrng provides a deterministic prng for the nonce in generated fake
// nodes.  The ensures that the nodes have unique hashes.
var testNoncePrng = mrand.New(mrand.NewSource(0))

// newFakeNode creates a block node connected to the passed parent with the
// provided fields populated and fake values for the other fields.
func newFakeNode(parent *blockNode, blockVersion int32, stakeVersion uint32, bits uint32, timestamp time.Time) *blockNode {
	// Make up a header and create a block node from it.
	header := &wire.BlockHeader{
		Version:      blockVersion,
		PrevBlock:    parent.hash,
		VoteBits:     0x01,
		Bits:         bits,
		Height:       uint32(parent.height) + 1,
		Timestamp:    timestamp,
		Nonce:        testNoncePrng.Uint32(),
		StakeVersion: stakeVersion,
	}
	return newBlockNode(header, parent)
}

// chainedFakeNodes returns the specified number of nodes constructed such that
// each subsequent node points to the previous one to create a chain.  The first
// node will point to the passed parent which can be nil if desired.
func chainedFakeNodes(parent *blockNode, numNodes int) []*blockNode {
	nodes := make([]*blockNode, numNodes)
	tip := parent
	blockTime := time.Now()
	if tip != nil {
		blockTime = time.Unix(tip.timestamp, 0)
	}
	for i := 0; i < numNodes; i++ {
		blockTime = blockTime.Add(time.Second)
		node := newFakeNode(tip, 1, 1, 0, blockTime)
		tip = node

		nodes[i] = node
	}
	return nodes
}

// branchTip is a convenience function to grab the tip of a chain of block nodes
// created via chainedFakeNodes.
func branchTip(nodes []*blockNode) *blockNode {
	return nodes[len(nodes)-1]
}

// appendFakeVotes appends the passed number of votes to the node with the
// provided version and vote bits.
func appendFakeVotes(node *blockNode, numVotes uint16, voteVersion uint32, voteBits uint16) {
	for i := uint16(0); i < numVotes; i++ {
		node.votes = append(node.votes, stake.VoteVersionTuple{
			Version: voteVersion,
			Bits:    voteBits,
		})
	}
}
