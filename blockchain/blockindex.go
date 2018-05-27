// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/wire"
)

// blockStatus is a bit field representing the validation state of the block.
type blockStatus byte

// The following constants specify possible status bit flags for a block.
//
// NOTE: This section specifically does not use iota since the block status is
// serialized and must be stable for long-term storage.
const (
	// statusNone indicates that the block has no validation state flags set.
	statusNone blockStatus = 0

	// statusDataStored indicates that the block's payload is stored on disk.
	statusDataStored blockStatus = 1 << 0

	// statusValid indicates that the block has been fully validated.
	statusValid blockStatus = 1 << 1

	// statusValidateFailed indicates that the block has failed validation.
	statusValidateFailed blockStatus = 1 << 2

	// statusInvalidAncestor indicates that one of the ancestors of the block
	// has failed validation, thus the block is also invalid.
	statusInvalidAncestor = 1 << 3
)

// HaveData returns whether the full block data is stored in the database.  This
// will return false for a block node where only the header is downloaded or
// stored.
func (status blockStatus) HaveData() bool {
	return status&statusDataStored != 0
}

// KnownValid returns whether the block is known to be valid.  This will return
// false for a valid block that has not been fully validated yet.
func (status blockStatus) KnownValid() bool {
	return status&statusValid != 0
}

// KnownInvalid returns whether the block is known to be invalid.  This will
// return false for invalid blocks that have not been proven invalid yet.
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

	// hash is the hash of the block this node represents.
	hash chainhash.Hash

	// workSum is the total amount of work in the chain up to and including
	// this node.
	workSum *big.Int

	// Some fields from block headers to aid in best chain selection and
	// reconstructing headers from memory.  These must be treated as
	// immutable and are intentionally ordered to avoid padding on 64-bit
	// platforms.
	height       int64
	voteBits     uint16
	finalState   [6]byte
	blockVersion int32
	voters       uint16
	freshStake   uint8
	revocations  uint8
	poolSize     uint32
	bits         uint32
	sbits        int64
	timestamp    int64
	merkleRoot   chainhash.Hash
	stakeRoot    chainhash.Hash
	blockSize    uint32
	nonce        uint32
	extraData    [32]byte
	stakeVersion uint32

	// status is a bitfield representing the validation state of the block.
	// This field, unlike the other fields, may be changed after the block
	// node is created, so it must only be accessed or updated using the
	// concurrent-safe NodeStatus, SetStatusFlags, and UnsetStatusFlags
	// methods on blockIndex once the node has been added to the index.
	status blockStatus

	// inMainChain denotes whether the block node is currently on the
	// the main chain or not.  This is used to help find the common
	// ancestor when switching chains.
	inMainChain bool

	// stakeNode contains all the consensus information required for the
	// staking system.  The node also caches information required to add or
	// remove stake nodes, so that the stake node itself may be pruneable
	// to save memory while maintaining high throughput efficiency for the
	// evaluation of sidechains.
	stakeNode      *stake.Node
	newTickets     []chainhash.Hash
	stakeUndoData  stake.UndoTicketDataSlice
	ticketsVoted   []chainhash.Hash
	ticketsRevoked []chainhash.Hash

	// Keep track of all vote version and bits in this block.
	votes []stake.VoteVersionTuple
}

// initBlockNode initializes a block node from the given header, initialization
// vector for the ticket lottery, and parent node.  The workSum is calculated
// based on the parent, or, in the case no parent is provided, it will just be
// the work for the passed block.
//
// This function is NOT safe for concurrent access.  It must only be called when
// initially creating a node.
func initBlockNode(node *blockNode, blockHeader *wire.BlockHeader, parent *blockNode) {
	*node = blockNode{
		hash:         blockHeader.BlockHash(),
		workSum:      CalcWork(blockHeader.Bits),
		height:       int64(blockHeader.Height),
		blockVersion: blockHeader.Version,
		voteBits:     blockHeader.VoteBits,
		finalState:   blockHeader.FinalState,
		voters:       blockHeader.Voters,
		freshStake:   blockHeader.FreshStake,
		poolSize:     blockHeader.PoolSize,
		bits:         blockHeader.Bits,
		sbits:        blockHeader.SBits,
		timestamp:    blockHeader.Timestamp.Unix(),
		merkleRoot:   blockHeader.MerkleRoot,
		stakeRoot:    blockHeader.StakeRoot,
		revocations:  blockHeader.Revocations,
		blockSize:    blockHeader.Size,
		nonce:        blockHeader.Nonce,
		extraData:    blockHeader.ExtraData,
		stakeVersion: blockHeader.StakeVersion,
	}
	if parent != nil {
		node.parent = parent
		node.workSum = node.workSum.Add(parent.workSum, node.workSum)
	}
}

// newBlockNode returns a new block node for the given block header and parent
// node.  The workSum is calculated based on the parent, or, in the case no
// parent is provided, it will just be the work for the passed block.
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
	prevHash := zeroHash
	if node.parent != nil {
		prevHash = &node.parent.hash
	}
	return wire.BlockHeader{
		Version:      node.blockVersion,
		PrevBlock:    *prevHash,
		MerkleRoot:   node.merkleRoot,
		StakeRoot:    node.stakeRoot,
		VoteBits:     node.voteBits,
		FinalState:   node.finalState,
		Voters:       node.voters,
		FreshStake:   node.freshStake,
		Revocations:  node.revocations,
		PoolSize:     node.poolSize,
		Bits:         node.bits,
		SBits:        node.sbits,
		Height:       uint32(node.height),
		Size:         node.blockSize,
		Timestamp:    time.Unix(node.timestamp, 0),
		Nonce:        node.nonce,
		ExtraData:    node.extraData,
		StakeVersion: node.stakeVersion,
	}
}

// lotteryIV returns the initialization vector for the deterministic PRNG used
// to determine winning tickets.
//
// This function is safe for concurrent access.
func (node *blockNode) lotteryIV() chainhash.Hash {
	// Serialize the block header for use in calculating the initialization
	// vector for the ticket lottery.  The only way this can fail is if the
	// process is out of memory in which case it would panic anyways, so
	// although panics are generally frowned upon in package code, it is
	// acceptable here.
	buf := bytes.NewBuffer(make([]byte, 0, wire.MaxBlockHeaderPayload))
	header := node.Header()
	if err := header.Serialize(buf); err != nil {
		panic(err)
	}

	return stake.CalcHash256PRNGIV(buf.Bytes())
}

// populateTicketInfo sets prunable ticket information in the provided block
// node.
//
// This function is NOT safe for concurrent access.  It must only be called when
// initially creating a node.
func (node *blockNode) populateTicketInfo(spentTickets *stake.SpentTicketsInBlock) {
	node.ticketsVoted = spentTickets.VotedTickets
	node.ticketsRevoked = spentTickets.RevokedTickets
	node.votes = spentTickets.Votes
}

// Ancestor returns the ancestor block node at the provided height by following
// the chain backwards from this node.  The returned block will be nil when a
// height is requested that is after the height of the passed node or is less
// than zero.
//
// This function is safe for concurrent access.
func (node *blockNode) Ancestor(height int64) *blockNode {
	if height < 0 || height > node.height {
		return nil
	}

	n := node
	for ; n != nil && n.height != height; n = n.parent {
		// Intentionally left blank
	}

	return n
}

// RelativeAncestor returns the ancestor block node a relative 'distance' blocks
// before this node.  This is equivalent to calling Ancestor with the node's
// height minus provided distance.
//
// This function is safe for concurrent access.
func (node *blockNode) RelativeAncestor(distance int64) *blockNode {
	return node.Ancestor(node.height - distance)
}

// CalcPastMedianTime calculates the median time of the previous few blocks
// prior to, and including, the block node.
//
// This function is safe for concurrent access.
func (node *blockNode) CalcPastMedianTime() time.Time {
	// Create a slice of the previous few block timestamps used to calculate
	// the median per the number defined by the constant medianTimeBlocks.
	timestamps := make([]int64, medianTimeBlocks)
	numNodes := 0
	iterNode := node
	for i := 0; i < medianTimeBlocks && iterNode != nil; i++ {
		timestamps[i] = iterNode.timestamp
		numNodes++

		iterNode = iterNode.parent
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
	index     map[chainhash.Hash]*blockNode
	chainTips map[int64][]*blockNode
}

// newBlockIndex returns a new empty instance of a block index.  The index will
// be dynamically populated as block nodes are loaded from the database and
// manually added.
func newBlockIndex(db database.DB, chainParams *chaincfg.Params) *blockIndex {
	return &blockIndex{
		db:          db,
		chainParams: chainParams,
		index:       make(map[chainhash.Hash]*blockNode),
		chainTips:   make(map[int64][]*blockNode),
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

// addNode adds the provided node to the block index.  Duplicate entries are not
// checked so it is up to caller to avoid adding them.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) addNode(node *blockNode) {
	bi.index[node.hash] = node

	// Since the block index does not support nodes that do not connect to
	// an existing node (except the genesis block), all new nodes are either
	// extending an existing chain or are on a side chain, but in either
	// case, are a new chain tip.  In the case the node is extending a
	// chain, the parent is no longer a tip.
	bi.addChainTip(node)
	if node.parent != nil {
		bi.removeChainTip(node.parent)
	}
}

// AddNode adds the provided node to the block index.  Duplicate entries are not
// checked so it is up to caller to avoid adding them.
//
// This function is safe for concurrent access.
func (bi *blockIndex) AddNode(node *blockNode) {
	bi.Lock()
	bi.addNode(node)
	bi.Unlock()
}

// addChainTip adds the passed block node as a new chain tip.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) addChainTip(tip *blockNode) {
	bi.chainTips[tip.height] = append(bi.chainTips[tip.height], tip)
}

// removeChainTip removes the passed block node from the available chain tips.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) removeChainTip(tip *blockNode) {
	nodes := bi.chainTips[tip.height]
	for i, n := range nodes {
		if n == tip {
			copy(nodes[i:], nodes[i+1:])
			nodes[len(nodes)-1] = nil
			nodes = nodes[:len(nodes)-1]
			break
		}
	}

	// Either update the map entry for the height with the remaining nodes
	// or remove it altogether if there are no more nodes left.
	if len(nodes) == 0 {
		delete(bi.chainTips, tip.height)
	} else {
		bi.chainTips[tip.height] = nodes
	}
}

// lookupNode returns the block node identified by the provided hash.  It will
// return nil if there is no entry for the hash.
//
// This function MUST be called with the block index lock held (for reads).
func (bi *blockIndex) lookupNode(hash *chainhash.Hash) *blockNode {
	return bi.index[*hash]
}

// LookupNode returns the block node identified by the provided hash.  It will
// return nil if there is no entry for the hash.
//
// This function is safe for concurrent access.
func (bi *blockIndex) LookupNode(hash *chainhash.Hash) *blockNode {
	bi.RLock()
	node := bi.lookupNode(hash)
	bi.RUnlock()
	return node
}

// NodeStatus returns the status associated with the provided node.
//
// This function is safe for concurrent access.
func (bi *blockIndex) NodeStatus(node *blockNode) blockStatus {
	bi.RLock()
	status := node.status
	bi.RUnlock()
	return status
}

// SetStatusFlags sets the provided status flags for the given block node
// regardless of their previous state.  It does not unset any flags.
//
// This function is safe for concurrent access.
func (bi *blockIndex) SetStatusFlags(node *blockNode, flags blockStatus) {
	bi.Lock()
	node.status |= flags
	bi.Unlock()
}

// UnsetStatusFlags unsets the provided status flags for the given block node
// regardless of their previous state.
//
// This function is safe for concurrent access.
func (bi *blockIndex) UnsetStatusFlags(node *blockNode, flags blockStatus) {
	bi.Lock()
	node.status &^= flags
	bi.Unlock()
}
