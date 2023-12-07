// Copyright (c) 2015-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
)

// blockStatus is a bit field representing the validation state of the block.
type blockStatus byte

const (
	// statusDataStored indicates that the block's payload is stored on disk.
	statusDataStored blockStatus = 1 << iota

	// statusValid indicates that the block has been fully validated.
	statusValid

	// statusValidateFailed indicates that the block has failed validation.
	statusValidateFailed

	// statusInvalidAncestor indicates that one of the block's ancestors has
	// has failed validation, thus the block is also invalid.
	statusInvalidAncestor

	// statusNone indicates that the block has no validation state flags set.
	//
	// NOTE: This must be defined last in order to avoid influencing iota.
	statusNone blockStatus = 0
)

// HaveData returns whether the full block data is stored in the database. This
// will return false for a block node where only the header is downloaded or
// kept.
func (status blockStatus) HaveData() bool {
	return status&statusDataStored != 0
}

// KnownValid returns whether the block is known to be valid. This will return
// false for a valid block that has not been fully validated yet.
func (status blockStatus) KnownValid() bool {
	return status&statusValid != 0
}

// KnownInvalid returns whether the block is known to be invalid. This may be
// because the block itself failed validation or any of its ancestors is
// invalid. This will return false for invalid blocks that have not been proven
// invalid yet.
func (status blockStatus) KnownInvalid() bool {
	return status&(statusValidateFailed|statusInvalidAncestor) != 0
}

// blockNode represents a block within the block chain and is primarily used to
// aid in selecting the best chain to be the main chain.  The main chain is
// stored into the block database.
type blockNode struct {
	// NOTE: Additions, deletions, or modifications to the order of the
	// definitions in this struct should not be changed without considering
	// how it affects alignment on 64-bit platforms.  The current order is
	// specifically crafted to result in minimal padding.  There will be
	// hundreds of thousands of these in memory, so a few extra bytes of
	// padding adds up.

	// parent is the parent block for this node.
	parent *blockNode

	// ancestor is a block that is more than one block back from this node.
	ancestor *blockNode

	// hash is the double sha 256 of the block.
	hash chainhash.Hash

	// workSum is the total amount of work in the chain up to and including
	// this node.
	workSum *big.Int

	// height is the position in the block chain.
	height int32

	// Some fields from block headers to aid in best chain selection and
	// reconstructing headers from memory.  These must be treated as
	// immutable and are intentionally ordered to avoid padding on 64-bit
	// platforms.
	version    int32
	bits       uint32
	nonce      uint32
	timestamp  int64
	merkleRoot chainhash.Hash

	// status is a bitfield representing the validation state of the block. The
	// status field, unlike the other fields, may be written to and so should
	// only be accessed using the concurrent-safe NodeStatus method on
	// blockIndex once the node has been added to the global index.
	status blockStatus
}

// initBlockNode initializes a block node from the given header and parent node,
// calculating the height and workSum from the respective fields on the parent.
// This function is NOT safe for concurrent access.  It must only be called when
// initially creating a node.
func initBlockNode(node *blockNode, blockHeader *wire.BlockHeader, parent *blockNode) {
	*node = blockNode{
		hash:       blockHeader.BlockHash(),
		workSum:    CalcWork(blockHeader.Bits),
		version:    blockHeader.Version,
		bits:       blockHeader.Bits,
		nonce:      blockHeader.Nonce,
		timestamp:  blockHeader.Timestamp.Unix(),
		merkleRoot: blockHeader.MerkleRoot,
	}
	if parent != nil {
		node.parent = parent
		node.height = parent.height + 1
		node.workSum = node.workSum.Add(parent.workSum, node.workSum)
		node.buildAncestor()
	}
}

// newBlockNode returns a new block node for the given block header and parent
// node, calculating the height and workSum from the respective fields on the
// parent. This function is NOT safe for concurrent access.
func newBlockNode(blockHeader *wire.BlockHeader, parent *blockNode) *blockNode {
	var node blockNode
	initBlockNode(&node, blockHeader, parent)
	return &node
}

// Header constructs a block header from the node and returns it.
//
// This function is safe for concurrent access.
func (node *blockNode) Header() wire.BlockHeader {
	// No lock is needed because all accessed fields are immutable.
	prevHash := &zeroHash
	if node.parent != nil {
		prevHash = &node.parent.hash
	}
	return wire.BlockHeader{
		Version:    node.version,
		PrevBlock:  *prevHash,
		MerkleRoot: node.merkleRoot,
		Timestamp:  time.Unix(node.timestamp, 0),
		Bits:       node.bits,
		Nonce:      node.nonce,
	}
}

// invertLowestOne turns the lowest 1 bit in the binary representation of a number into a 0.
func invertLowestOne(n int32) int32 {
	return n & (n - 1)
}

// getAncestorHeight returns a suitable ancestor for the node at the given height.
func getAncestorHeight(height int32) int32 {
	// We pop off two 1 bits of the height.
	// This results in a maximum of 330 steps to go back to an ancestor
	// from height 1<<29.
	return invertLowestOne(invertLowestOne(height))
}

// buildAncestor sets an ancestor for the given blocknode.
func (node *blockNode) buildAncestor() {
	if node.parent != nil {
		node.ancestor = node.parent.Ancestor(getAncestorHeight(node.height))
	}
}

// Ancestor returns the ancestor block node at the provided height by following
// the chain backwards from this node.  The returned block will be nil when a
// height is requested that is after the height of the passed node or is less
// than zero.
//
// This function is safe for concurrent access.
func (node *blockNode) Ancestor(height int32) *blockNode {
	if height < 0 || height > node.height {
		return nil
	}

	// Traverse back until we find the desired node.
	n := node
	for n != nil && n.height != height {
		// If there's an ancestor available, use it. Otherwise, just
		// follow the parent.
		if n.ancestor != nil {
			// Calculate the height for this ancestor and
			// check if we can take the ancestor skip.
			if getAncestorHeight(n.height) >= height {
				n = n.ancestor
				continue
			}
		}

		// We couldn't take the ancestor skip so traverse back to the parent.
		n = n.parent
	}

	return n
}

// Height returns the blockNode's height in the chain.
//
// NOTE: Part of the HeaderCtx interface.
func (node *blockNode) Height() int32 {
	return node.height
}

// Bits returns the blockNode's nBits.
//
// NOTE: Part of the HeaderCtx interface.
func (node *blockNode) Bits() uint32 {
	return node.bits
}

// Timestamp returns the blockNode's timestamp.
//
// NOTE: Part of the HeaderCtx interface.
func (node *blockNode) Timestamp() int64 {
	return node.timestamp
}

// Parent returns the blockNode's parent.
//
// NOTE: Part of the HeaderCtx interface.
func (node *blockNode) Parent() HeaderCtx {
	if node.parent == nil {
		// This is required since node.parent is a *blockNode and if we
		// do not explicitly return nil here, the caller may fail when
		// nil-checking this.
		return nil
	}

	return node.parent
}

// RelativeAncestorCtx returns the blockNode's ancestor that is distance blocks
// before it in the chain. This is equivalent to the RelativeAncestor function
// below except that the return type is different.
//
// This function is safe for concurrent access.
//
// NOTE: Part of the HeaderCtx interface.
func (node *blockNode) RelativeAncestorCtx(distance int32) HeaderCtx {
	ancestor := node.RelativeAncestor(distance)
	if ancestor == nil {
		// This is required since RelativeAncestor returns a *blockNode
		// and if we do not explicitly return nil here, the caller may
		// fail when nil-checking this.
		return nil
	}

	return ancestor
}

// RelativeAncestor returns the ancestor block node a relative 'distance' blocks
// before this node.  This is equivalent to calling Ancestor with the node's
// height minus provided distance.
//
// This function is safe for concurrent access.
func (node *blockNode) RelativeAncestor(distance int32) *blockNode {
	return node.Ancestor(node.height - distance)
}

// CalcPastMedianTime calculates the median time of the previous few blocks
// prior to, and including, the block node.
//
// This function is safe for concurrent access.
func CalcPastMedianTime(node HeaderCtx) time.Time {
	// Create a slice of the previous few block timestamps used to calculate
	// the median per the number defined by the constant medianTimeBlocks.
	timestamps := make([]int64, medianTimeBlocks)
	numNodes := 0
	iterNode := node
	for i := 0; i < medianTimeBlocks && iterNode != nil; i++ {
		timestamps[i] = iterNode.Timestamp()
		numNodes++

		iterNode = iterNode.Parent()
	}

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
	return time.Unix(medianTimestamp, 0)
}

// A compile-time assertion to ensure blockNode implements the HeaderCtx
// interface.
var _ HeaderCtx = (*blockNode)(nil)

// blockIndex provides facilities for keeping track of an in-memory index of the
// block chain.  Although the name block chain suggests a single chain of
// blocks, it is actually a tree-shaped structure where any node can have
// multiple children.  However, there can only be one active branch which does
// indeed form a chain from the tip all the way back to the genesis block.
type blockIndex struct {
	// The following fields are set when the instance is created and can't
	// be changed afterwards, so there is no need to protect them with a
	// separate mutex.
	db          database.DB
	chainParams *chaincfg.Params

	sync.RWMutex
	index map[chainhash.Hash]*blockNode
	dirty map[*blockNode]struct{}
}

// newBlockIndex returns a new empty instance of a block index.  The index will
// be dynamically populated as block nodes are loaded from the database and
// manually added.
func newBlockIndex(db database.DB, chainParams *chaincfg.Params) *blockIndex {
	return &blockIndex{
		db:          db,
		chainParams: chainParams,
		index:       make(map[chainhash.Hash]*blockNode),
		dirty:       make(map[*blockNode]struct{}),
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

// AddNode adds the provided node to the block index and marks it as dirty.
// Duplicate entries are not checked so it is up to caller to avoid adding them.
//
// This function is safe for concurrent access.
func (bi *blockIndex) AddNode(node *blockNode) {
	bi.Lock()
	bi.addNode(node)
	bi.dirty[node] = struct{}{}
	bi.Unlock()
}

// addNode adds the provided node to the block index, but does not mark it as
// dirty. This can be used while initializing the block index.
//
// This function is NOT safe for concurrent access.
func (bi *blockIndex) addNode(node *blockNode) {
	bi.index[node.hash] = node
}

// NodeStatus provides concurrent-safe access to the status field of a node.
//
// This function is safe for concurrent access.
func (bi *blockIndex) NodeStatus(node *blockNode) blockStatus {
	bi.RLock()
	status := node.status
	bi.RUnlock()
	return status
}

// SetStatusFlags flips the provided status flags on the block node to on,
// regardless of whether they were on or off previously. This does not unset any
// flags currently on.
//
// This function is safe for concurrent access.
func (bi *blockIndex) SetStatusFlags(node *blockNode, flags blockStatus) {
	bi.Lock()
	node.status |= flags
	bi.dirty[node] = struct{}{}
	bi.Unlock()
}

// UnsetStatusFlags flips the provided status flags on the block node to off,
// regardless of whether they were on or off previously.
//
// This function is safe for concurrent access.
func (bi *blockIndex) UnsetStatusFlags(node *blockNode, flags blockStatus) {
	bi.Lock()
	node.status &^= flags
	bi.dirty[node] = struct{}{}
	bi.Unlock()
}

// InactiveTips returns all the block nodes that aren't in the best chain.
//
// This function is safe for concurrent access.
func (bi *blockIndex) InactiveTips(bestChain *chainView) []*blockNode {
	bi.RLock()
	defer bi.RUnlock()

	// Look through the entire blockindex and look for nodes that aren't in
	// the best chain. We're gonna keep track of all the orphans and the parents
	// of the orphans.
	orphans := make(map[chainhash.Hash]*blockNode)
	orphanParent := make(map[chainhash.Hash]*blockNode)
	for hash, node := range bi.index {
		found := bestChain.Contains(node)
		if !found {
			orphans[hash] = node
			orphanParent[node.parent.hash] = node.parent
		}
	}

	// If an orphan isn't pointed to by another orphan, it is a chain tip.
	//
	// We can check this by looking for the orphan in the orphan parent map.
	// If the orphan exists in the orphan parent map, it means that another
	// orphan is pointing to it.
	tips := make([]*blockNode, 0, len(orphans))
	for hash, orphan := range orphans {
		_, found := orphanParent[hash]
		if !found {
			tips = append(tips, orphan)
		}

		delete(orphanParent, hash)
	}

	return tips
}

// flushToDB writes all dirty block nodes to the database. If all writes
// succeed, this clears the dirty set.
func (bi *blockIndex) flushToDB() error {
	bi.Lock()
	if len(bi.dirty) == 0 {
		bi.Unlock()
		return nil
	}

	err := bi.db.Update(func(dbTx database.Tx) error {
		for node := range bi.dirty {
			err := dbStoreBlockNode(dbTx, node)
			if err != nil {
				return err
			}
		}
		return nil
	})

	// If write was successful, clear the dirty set.
	if err == nil {
		bi.dirty = make(map[*blockNode]struct{})
	}

	bi.Unlock()
	return err
}
