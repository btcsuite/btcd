// Copyright (c) 2015-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// blockNode represents a block within the block chain and is primarily used to
// aid in selecting the best chain to be the main chain.  The main chain is
// stored into the block database.
type blockNode struct {
	// parent is the parent block for this node.
	parent *blockNode

	// children contains the child nodes for this node.  Typically there
	// will only be one, but sometimes there can be more than one and that
	// is when the best chain selection algorithm is used.
	children []*blockNode

	// hash is the double sha 256 of the block.
	hash chainhash.Hash

	// parentHash is the double sha 256 of the parent block.  This is kept
	// here over simply relying on parent.hash directly since block nodes
	// are sparse and the parent node might not be in memory when its hash
	// is needed.
	parentHash chainhash.Hash

	// height is the position in the block chain.
	height int32

	// workSum is the total amount of work in the chain up to and including
	// this node.
	workSum *big.Int

	// inMainChain denotes whether the block node is currently on the
	// the main chain or not.  This is used to help find the common
	// ancestor when switching chains.
	inMainChain bool

	// Some fields from block headers to aid in best chain selection and
	// reconstructing headers from memory.  These must be treated as
	// immutable and are intentionally ordered to avoid padding on 64-bit
	// platforms.
	version    int32
	bits       uint32
	nonce      uint32
	timestamp  int64
	merkleRoot chainhash.Hash
}

// newBlockNode returns a new block node for the given block header.  It is
// completely disconnected from the chain and the workSum value is just the work
// for the passed block.  The work sum is updated accordingly when the node is
// inserted into a chain.
func newBlockNode(blockHeader *wire.BlockHeader, blockHash *chainhash.Hash, height int32) *blockNode {
	node := blockNode{
		hash:       *blockHash,
		parentHash: blockHeader.PrevBlock,
		workSum:    CalcWork(blockHeader.Bits),
		height:     height,
		version:    blockHeader.Version,
		bits:       blockHeader.Bits,
		nonce:      blockHeader.Nonce,
		timestamp:  blockHeader.Timestamp.Unix(),
		merkleRoot: blockHeader.MerkleRoot,
	}
	return &node
}

// Header constructs a block header from the node and returns it.
//
// This function is safe for concurrent access.
func (node *blockNode) Header() wire.BlockHeader {
	// No lock is needed because all accessed fields are immutable.
	return wire.BlockHeader{
		Version:    node.version,
		PrevBlock:  node.parentHash,
		MerkleRoot: node.merkleRoot,
		Timestamp:  time.Unix(node.timestamp, 0),
		Bits:       node.bits,
		Nonce:      node.nonce,
	}
}

// removeChildNode deletes node from the provided slice of child block
// nodes.  It ensures the final pointer reference is set to nil to prevent
// potential memory leaks.  The original slice is returned unmodified if node
// is invalid or not in the slice.
//
// This function MUST be called with the block index lock held (for writes).
func removeChildNode(children []*blockNode, node *blockNode) []*blockNode {
	if node == nil {
		return children
	}

	// An indexing for loop is intentionally used over a range here as range
	// does not reevaluate the slice on each iteration nor does it adjust
	// the index for the modified slice.
	for i := 0; i < len(children); i++ {
		if children[i].hash.IsEqual(&node.hash) {
			copy(children[i:], children[i+1:])
			children[len(children)-1] = nil
			return children[:len(children)-1]
		}
	}
	return children
}

// blockIndex provides facilities for keeping track of an in-memory index of the
// block chain.  Although the name block chain suggest a single chain of blocks,
// it is actually a tree-shaped structure where any node can have multiple
// children.  However, there can only be one active branch which does indeed
// form a chain from the tip all the way back to the genesis block.
type blockIndex struct {
	// The following fields are set when the instance is created and can't
	// be changed afterwards, so there is no need to protect them with a
	// separate mutex.
	db          database.DB
	chainParams *chaincfg.Params

	sync.RWMutex
	index    map[chainhash.Hash]*blockNode
	depNodes map[chainhash.Hash][]*blockNode
}

// newBlockIndex returns a new empty instance of a block index.  The index will
// be dynamically populated as block nodes are loaded from the database and
// manually added.
func newBlockIndex(db database.DB, chainParams *chaincfg.Params) *blockIndex {
	return &blockIndex{
		db:          db,
		chainParams: chainParams,
		index:       make(map[chainhash.Hash]*blockNode),
		depNodes:    make(map[chainhash.Hash][]*blockNode),
	}
}

// HaveBlock returns whether or not the block index contains the provided hash.
//
// This function is safe for concurrent access.
func (bi *blockIndex) HaveBlock(hash *chainhash.Hash) bool {
	bi.RLock()
	_, hasBlock := bi.index[*hash]
	bi.RUnlock()
	return hasBlock
}

// loadBlockNode loads the block identified by hash from the block database,
// creates a block node from it, and updates the block index accordingly.  It is
// used mainly to dynamically load previous blocks from the database as they are
// needed to avoid needing to put the entire block index in memory.
//
// This function MUST be called with the block index lock held (for writes).
// The database transaction may be read-only.
func (bi *blockIndex) loadBlockNode(dbTx database.Tx, hash *chainhash.Hash) (*blockNode, error) {
	// Load the block header and height from the db.
	blockHeader, err := dbFetchHeaderByHash(dbTx, hash)
	if err != nil {
		return nil, err
	}
	blockHeight, err := dbFetchHeightByHash(dbTx, hash)
	if err != nil {
		return nil, err
	}

	// Create the new block node for the block and set the work.
	node := newBlockNode(blockHeader, hash, blockHeight)
	node.inMainChain = true

	// Add the node to the chain.
	// There are a few possibilities here:
	//  1) This node is a child of an existing block node
	//  2) This node is the parent of one or more nodes
	//  3) Neither 1 or 2 is true which implies it's an orphan block and
	//     therefore is an error to insert into the chain
	prevHash := &blockHeader.PrevBlock
	if parentNode, ok := bi.index[*prevHash]; ok {
		// Case 1 -- This node is a child of an existing block node.
		// Update the node's work sum with the sum of the parent node's
		// work sum and this node's work, append the node as a child of
		// the parent node and set this node's parent to the parent
		// node.
		node.workSum = node.workSum.Add(parentNode.workSum, node.workSum)
		parentNode.children = append(parentNode.children, node)
		node.parent = parentNode

	} else if childNodes, ok := bi.depNodes[*hash]; ok {
		// Case 2 -- This node is the parent of one or more nodes.
		// Update the node's work sum by subtracting this node's work
		// from the sum of its first child, and connect the node to all
		// of its children.
		node.workSum.Sub(childNodes[0].workSum, node.workSum)
		for _, childNode := range childNodes {
			childNode.parent = node
			node.children = append(node.children, childNode)
		}

	} else {
		// Case 3 -- The node doesn't have a parent and is not the
		// parent of another node.  This means an arbitrary orphan block
		// is trying to be loaded which is not allowed.
		str := "loadBlockNode: attempt to insert orphan block %v"
		return nil, AssertError(fmt.Sprintf(str, hash))
	}

	// Add the new node to the indices for faster lookups.
	bi.index[*hash] = node
	bi.depNodes[*prevHash] = append(bi.depNodes[*prevHash], node)

	return node, nil
}

// PrevNodeFromBlock returns a block node for the block previous to the passed
// block (the passed block's parent).  When it is already in the memory block
// chain, it simply returns it.  Otherwise, it loads the previous block header
// from the block database, creates a new block node from it, and returns it.
// The returned node will be nil if the genesis block is passed.
//
// This function is safe for concurrent access.
func (bi *blockIndex) PrevNodeFromBlock(block *btcutil.Block) (*blockNode, error) {
	// Genesis block.
	prevHash := &block.MsgBlock().Header.PrevBlock
	if prevHash.IsEqual(zeroHash) {
		return nil, nil
	}

	bi.Lock()
	defer bi.Unlock()

	// Return the existing previous block node if it's already there.
	if bn, ok := bi.index[*prevHash]; ok {
		return bn, nil
	}

	// Dynamically load the previous block from the block database, create
	// a new block node for it, and update the memory chain accordingly.
	var prevBlockNode *blockNode
	err := bi.db.View(func(dbTx database.Tx) error {
		var err error
		prevBlockNode, err = bi.loadBlockNode(dbTx, prevHash)
		return err
	})
	return prevBlockNode, err
}

// prevNodeFromNode returns a block node for the block previous to the
// passed block node (the passed block node's parent).  When the node is already
// connected to a parent, it simply returns it.  Otherwise, it loads the
// associated block from the database to obtain the previous hash and uses that
// to dynamically create a new block node and return it.  The memory block
// chain is updated accordingly.  The returned node will be nil if the genesis
// block is passed.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) prevNodeFromNode(node *blockNode) (*blockNode, error) {
	// Return the existing previous block node if it's already there.
	if node.parent != nil {
		return node.parent, nil
	}

	// Genesis block.
	if node.hash.IsEqual(bi.chainParams.GenesisHash) {
		return nil, nil
	}

	// Dynamically load the previous block from the block database, create
	// a new block node for it, and update the memory chain accordingly.
	var prevBlockNode *blockNode
	err := bi.db.View(func(dbTx database.Tx) error {
		var err error
		prevBlockNode, err = bi.loadBlockNode(dbTx, &node.parentHash)
		return err
	})
	return prevBlockNode, err
}

// PrevNodeFromNode returns a block node for the block previous to the
// passed block node (the passed block node's parent).  When the node is already
// connected to a parent, it simply returns it.  Otherwise, it loads the
// associated block from the database to obtain the previous hash and uses that
// to dynamically create a new block node and return it.  The memory block
// chain is updated accordingly.  The returned node will be nil if the genesis
// block is passed.
//
// This function is safe for concurrent access.
func (bi *blockIndex) PrevNodeFromNode(node *blockNode) (*blockNode, error) {
	bi.Lock()
	node, err := bi.prevNodeFromNode(node)
	bi.Unlock()
	return node, err
}

// RelativeNode returns the ancestor block a relative 'distance' blocks before
// the passed anchor block.  While iterating backwards through the chain, any
// block nodes which aren't in the memory chain are loaded in dynamically.
//
// This function is safe for concurrent access.
func (bi *blockIndex) RelativeNode(anchor *blockNode, distance uint32) (*blockNode, error) {
	bi.Lock()
	defer bi.Unlock()

	iterNode := anchor
	err := bi.db.View(func(dbTx database.Tx) error {
		// Walk backwards in the chian until we've gone 'distance'
		// steps back.
		var err error
		for i := distance; i > 0; i-- {
			switch {
			// If the parent of this node has already been loaded
			// into memory, then we can follow the link without
			// hitting the database.
			case iterNode.parent != nil:
				iterNode = iterNode.parent

			// If this node is the genesis block, then we can't go
			// back any further, so we exit immediately.
			case iterNode.hash.IsEqual(bi.chainParams.GenesisHash):
				return nil

			// Otherwise, load the block node from the database,
			// pulling it into the memory cache in the processes.
			default:
				iterNode, err = bi.loadBlockNode(dbTx,
					&iterNode.parentHash)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return iterNode, nil
}

// AncestorNode returns the ancestor block node at the provided height by
// following the chain backwards from the given node while dynamically loading
// any pruned nodes from the database and updating the memory block chain as
// needed.  The returned block will be nil when a height is requested that is
// after the height of the passed node or is less than zero.
//
// This function is safe for concurrent access.
func (bi *blockIndex) AncestorNode(node *blockNode, height int32) (*blockNode, error) {
	// Nothing to do if the requested height is outside of the valid range.
	if height > node.height || height < 0 {
		return nil, nil
	}

	// Iterate backwards until the requested height is reached.
	bi.Lock()
	iterNode := node
	for iterNode != nil && iterNode.height > height {
		var err error
		iterNode, err = bi.prevNodeFromNode(iterNode)
		if err != nil {
			break
		}
	}
	bi.Unlock()

	return iterNode, nil
}

// AddNode adds the provided node to the block index.  Duplicate entries are not
// checked so it is up to caller to avoid adding them.
//
// This function is safe for concurrent access.
func (bi *blockIndex) AddNode(node *blockNode) {
	bi.Lock()
	bi.index[node.hash] = node
	if prevHash := &node.parentHash; *prevHash != *zeroHash {
		bi.depNodes[*prevHash] = append(bi.depNodes[*prevHash], node)
	}
	bi.Unlock()
}

// LookupNode returns the block node identified by the provided hash.  It will
// return nil if there is no entry for the hash.
//
// This function is safe for concurrent access.
func (bi *blockIndex) LookupNode(hash *chainhash.Hash) *blockNode {
	bi.RLock()
	node := bi.index[*hash]
	bi.RUnlock()
	return node
}

// CalcPastMedianTime calculates the median time of the previous few blocks
// prior to, and including, the passed block node.
//
// This function is safe for concurrent access.
func (bi *blockIndex) CalcPastMedianTime(startNode *blockNode) (time.Time, error) {
	// Genesis block.
	if startNode == nil {
		return bi.chainParams.GenesisBlock.Header.Timestamp, nil
	}

	// Create a slice of the previous few block timestamps used to calculate
	// the median per the number defined by the constant medianTimeBlocks.
	timestamps := make([]int64, medianTimeBlocks)
	numNodes := 0
	iterNode := startNode
	bi.Lock()
	for i := 0; i < medianTimeBlocks && iterNode != nil; i++ {
		timestamps[i] = iterNode.timestamp
		numNodes++

		// Get the previous block node.  This function is used over
		// simply accessing iterNode.parent directly as it will
		// dynamically create previous block nodes as needed.  This
		// helps allow only the pieces of the chain that are needed
		// to remain in memory.
		var err error
		iterNode, err = bi.prevNodeFromNode(iterNode)
		if err != nil {
			bi.Unlock()
			log.Errorf("prevNodeFromNode: %v", err)
			return time.Time{}, err
		}
	}
	bi.Unlock()

	// Prune the slice to the actual number of available timestamps which
	// will be fewer than desired near the beginning of the block chain
	// and sort them.
	timestamps = timestamps[:numNodes]
	sort.Sort(timeSorter(timestamps))

	// NOTE: The consensus rules incorrectly calculate the median for even
	// numbers of blocks.  A true median averages the middle two elements
	// for a set with an even number of elements in it.   Since the constant
	// for the previous number of blocks to be used is odd, this is only an
	// issue for a few blocks near the beginning of the chain.  I suspect
	// this is an optimization even though the result is slightly wrong for
	// a few of the first blocks since after the first few blocks, there
	// will always be an odd number of blocks in the set per the constant.
	//
	// This code follows suit to ensure the same rules are used, however, be
	// aware that should the medianTimeBlocks constant ever be changed to an
	// even number, this code will be wrong.
	medianTimestamp := timestamps[numNodes/2]
	return time.Unix(medianTimestamp, 0), nil
}
