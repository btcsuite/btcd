// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"container/list"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

const (
	// maxOrphanBlocks is the maximum number of orphan blocks that can be
	// queued.
	maxOrphanBlocks = 500

	// minMemoryNodes is the minimum number of consecutive nodes needed
	// in memory in order to perform all necessary validation.  It is used
	// to determine when it's safe to prune nodes from memory without
	// causing constant dynamic reloading.  This value should be larger than
	// that for minMemoryStakeNodes.
	minMemoryNodes = 2880

	// minMemoryStakeNodes is the maximum height to keep stake nodes
	// in memory for in their respective nodes.  Beyond this height,
	// they will need to be manually recalculated.  This value should
	// be at least the stake retarget interval.
	minMemoryStakeNodes = 288

	// mainchainBlockCacheSize is the number of mainchain blocks to
	// keep in memory, by height from the tip of the mainchain.
	mainchainBlockCacheSize = 12

	// maxSearchDepth is the distance in block nodes to search down the
	// blockchain to find some parent, loading block nodes from the
	// database if necessary.  Reorganizations longer than this disance may
	// fail.
	maxSearchDepth = 2880
)

// VoteVersionTuple contains the extracted vote bits and version from votes
// (SSGen).
type VoteVersionTuple struct {
	Version uint32
	Bits    uint16
}

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

	// height is the position in the block chain.
	height int64

	// workSum is the total amount of work in the chain up to and including
	// this node.
	workSum *big.Int

	// inMainChain denotes whether the block node is currently on the
	// the main chain or not.  This is used to help find the common
	// ancestor when switching chains.
	inMainChain bool

	// header is the full block header.
	header wire.BlockHeader

	// stakeNode contains all the consensus information required for the
	// staking system.  The node also caches information required to add or
	// remove stake nodes, so that the stake node itself may be pruneable
	// to save memory while maintaining high throughput efficiency for the
	// evaluation of sidechains.
	stakeDataLock  sync.Mutex
	stakeNode      *stake.Node
	newTickets     []chainhash.Hash
	stakeUndoData  stake.UndoTicketDataSlice
	ticketsSpent   []chainhash.Hash
	ticketsRevoked []chainhash.Hash

	// Keep track of all vote version and bits in this block.
	votes []VoteVersionTuple
}

// newBlockNode returns a new block node for the given block header.  It is
// completely disconnected from the chain and the workSum value is just the work
// for the passed block.  The work sum is updated accordingly when the node is
// inserted into a chain.
func newBlockNode(blockHeader *wire.BlockHeader, ticketsSpent []chainhash.Hash, ticketsRevoked []chainhash.Hash, votes []VoteVersionTuple) *blockNode {
	// Make a copy of the hash so the node doesn't keep a reference to part
	// of the full block/block header preventing it from being garbage
	// collected.
	node := blockNode{
		hash:           blockHeader.BlockHash(),
		workSum:        CalcWork(blockHeader.Bits),
		height:         int64(blockHeader.Height),
		header:         *blockHeader,
		ticketsSpent:   ticketsSpent,
		ticketsRevoked: ticketsRevoked,
		votes:          votes,
	}
	return &node
}

// orphanBlock represents a block that we don't yet have the parent for.  It
// is a normal block plus an expiration time to prevent caching the orphan
// forever.
type orphanBlock struct {
	block      *dcrutil.Block
	expiration time.Time
}

// removeChildNode deletes node from the provided slice of child block
// nodes.  It ensures the final pointer reference is set to nil to prevent
// potential memory leaks.  The original slice is returned unmodified if node
// is invalid or not in the slice.
//
// This function MUST be called with the chain state lock held (for writes).
func removeChildNode(children []*blockNode, node *blockNode) []*blockNode {
	if node == nil {
		return children
	}

	// An indexing for loop is intentionally used over a range here as range
	// does not reevaluate the slice on each iteration nor does it adjust
	// the index for the modified slice.
	for i := 0; i < len(children); i++ {
		if children[i].hash == node.hash {
			copy(children[i:], children[i+1:])
			children[len(children)-1] = nil
			return children[:len(children)-1]
		}
	}
	return children
}

// BestState houses information about the current best block and other info
// related to the state of the main chain as it exists from the point of view of
// the current best block.
//
// The BestSnapshot method can be used to obtain access to this information
// in a concurrent safe manner and the data will not be changed out from under
// the caller when chain state changes occur as the function name implies.
// However, the returned snapshot must be treated as immutable since it is
// shared by all callers.
type BestState struct {
	Hash         *chainhash.Hash // The hash of the block.
	Height       int64           // The height of the block.
	Bits         uint32          // The difficulty bits of the block.
	BlockSize    uint64          // The size of the block.
	NumTxns      uint64          // The number of txns in the block.
	TotalTxns    uint64          // The total number of txns in the chain.
	MedianTime   time.Time       // Median time as per calcPastMedianTime.
	TotalSubsidy int64           // The total subsidy for the chain.
}

// newBestState returns a new best stats instance for the given parameters.
func newBestState(node *blockNode, blockSize, numTxns, totalTxns uint64, medianTime time.Time, totalSubsidy int64) *BestState {
	return &BestState{
		Hash:         &node.hash,
		Height:       node.height,
		Bits:         node.header.Bits,
		BlockSize:    blockSize,
		NumTxns:      numTxns,
		TotalTxns:    totalTxns,
		MedianTime:   medianTime,
		TotalSubsidy: totalSubsidy,
	}
}

// BlockChain provides functions for working with the Decred block chain.
// It includes functionality such as rejecting duplicate blocks, ensuring blocks
// follow all rules, orphan handling, checkpoint handling, and best chain
// selection with reorganization.
type BlockChain struct {
	// The following fields are set when the instance is created and can't
	// be changed afterwards, so there is no need to protect them with a
	// separate mutex.
	checkpointsByHeight map[int64]*chaincfg.Checkpoint
	db                  database.DB
	dbInfo              *databaseInfo
	chainParams         *chaincfg.Params
	timeSource          MedianTimeSource
	notifications       NotificationCallback
	sigCache            *txscript.SigCache
	indexManager        IndexManager

	// subsidyCache is the cache that provides quick lookup of subsidy
	// values.
	subsidyCache *SubsidyCache

	// chainLock protects concurrent access to the vast majority of the
	// fields in this struct below this point.
	chainLock sync.RWMutex

	// These fields are configuration parameters that can be toggled at
	// runtime.  They are protected by the chain lock.
	noVerify      bool
	noCheckpoints bool

	// These fields are related to the memory block index.  They are
	// protected by the chain lock.
	bestNode *blockNode
	index    map[chainhash.Hash]*blockNode
	depNodes map[chainhash.Hash][]*blockNode

	// These fields are related to handling of orphan blocks.  They are
	// protected by a combination of the chain lock and the orphan lock.
	orphanLock     sync.RWMutex
	orphans        map[chainhash.Hash]*orphanBlock
	prevOrphans    map[chainhash.Hash][]*orphanBlock
	oldestOrphan   *orphanBlock
	blockCacheLock sync.RWMutex
	blockCache     map[chainhash.Hash]*dcrutil.Block

	// The block cache for mainchain blocks, to facilitate faster
	// reorganizations.
	mainchainBlockCacheLock sync.RWMutex
	mainchainBlockCache     map[chainhash.Hash]*dcrutil.Block
	mainchainBlockCacheSize int

	// These fields are related to checkpoint handling.  They are protected
	// by the chain lock.
	nextCheckpoint  *chaincfg.Checkpoint
	checkpointBlock *dcrutil.Block

	// The state is used as a fairly efficient way to cache information
	// about the current best chain state that is returned to callers when
	// requested.  It operates on the principle of MVCC such that any time a
	// new block becomes the best block, the state pointer is replaced with
	// a new struct and the old state is left untouched.  In this way,
	// multiple callers can be pointing to different best chain states.
	// This is acceptable for most callers because the state is only being
	// queried at a specific point in time.
	//
	// In addition, some of the fields are stored in the database so the
	// chain state can be quickly reconstructed on load.
	stateLock     sync.RWMutex
	stateSnapshot *BestState

	// The following caches are used to efficiently keep track of the
	// current deployment threshold state of each rule change deployment.
	//
	// This information is stored in the database so it can be quickly
	// reconstructed on load.
	//
	// deploymentCaches caches the current deployment threshold state for
	// blocks in each of the actively defined deployments.
	deploymentCaches map[uint32][]thresholdStateCache

	// pruner is the automatic pruner for block nodes and stake nodes,
	// so that the memory may be restored by the garbage collector if
	// it is unlikely to be referenced in the future.
	pruner *chainPruner

	// The following maps are various caches for the stake version/voting
	// system.  The goal of these is to reduce disk access to load blocks
	// from disk.  Measurements indicate that it is slightly more expensive
	// so setup the cache (<10%) vs doing a straight chain walk.  Every
	// other subsequent call is >10x faster.
	isVoterMajorityVersionCache   map[[stakeMajorityCacheKeySize]byte]bool
	isStakeMajorityVersionCache   map[[stakeMajorityCacheKeySize]byte]bool
	calcPriorStakeVersionCache    map[[chainhash.HashSize]byte]uint32
	calcVoterVersionIntervalCache map[[chainhash.HashSize]byte]uint32
	calcStakeVersionCache         map[[chainhash.HashSize]byte]uint32
}

const (
	// stakeMajorityCacheKeySize is comprised of the stake version and the
	// hash size.  The stake version is a little endian uint32, hence we
	// add 4 to the overall size.
	stakeMajorityCacheKeySize = 4 + chainhash.HashSize
)

// StakeVersions is a condensed form of a dcrutil.Block that is used to prevent
// using gigabytes of memory.
type StakeVersions struct {
	Hash         chainhash.Hash
	Height       int64
	BlockVersion int32
	StakeVersion uint32
	Votes        []VoteVersionTuple
}

// GetStakeVersions returns a cooked array of StakeVersions.  We do this in
// order to not bloat memory by returning raw blocks.
func (b *BlockChain) GetStakeVersions(hash *chainhash.Hash, count int32) ([]StakeVersions, error) {
	// Nothing to do if no count requested.
	if count == 0 {
		return nil, nil
	}

	if count < 0 {
		return nil, fmt.Errorf("count must not be less than zero - "+
			"got %d", count)
	}

	b.chainLock.Lock()
	defer b.chainLock.Unlock()

	if count > int32(b.bestNode.height) {
		count = int32(b.bestNode.height)
	}

	startNode, err := b.findNode(hash, 0)
	if err != nil {
		return nil, err
	}

	result := make([]StakeVersions, 0, count)
	prevNode := startNode
	for i := int32(0); prevNode != nil && i < count; i++ {
		sv := StakeVersions{
			Hash:         prevNode.hash,
			Height:       prevNode.height,
			BlockVersion: prevNode.header.Version,
			StakeVersion: prevNode.header.StakeVersion,
			Votes:        prevNode.votes,
		}

		result = append(result, sv)

		prevNode, err = b.getPrevNodeFromNode(prevNode)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

type VoteInfo struct {
	Agendas      []chaincfg.ConsensusDeployment
	AgendaStatus []ThresholdStateTuple
}

// GetVoteInfo returns
func (b *BlockChain) GetVoteInfo(hash *chainhash.Hash, version uint32) (*VoteInfo, error) {
	deployments, ok := b.chainParams.Deployments[version]
	if !ok {
		return nil, VoteVersionError(version)
	}

	if !ok {
		return nil, HashError(hash.String())
	}

	vi := VoteInfo{
		Agendas: make([]chaincfg.ConsensusDeployment,
			0, len(deployments)),
		AgendaStatus: make([]ThresholdStateTuple, 0, len(deployments)),
	}
	for _, deployment := range deployments {
		vi.Agendas = append(vi.Agendas, deployment)
		status, err := b.ThresholdState(hash, version, deployment.Vote.Id)
		if err != nil {
			return nil, err
		}
		vi.AgendaStatus = append(vi.AgendaStatus, status)
	}

	return &vi, nil
}

// DisableVerify provides a mechanism to disable transaction script validation
// which you DO NOT want to do in production as it could allow double spends
// and other undesirable things.  It is provided only for debug purposes since
// script validation is extremely intensive and when debugging it is sometimes
// nice to quickly get the chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) DisableVerify(disable bool) {
	b.chainLock.Lock()
	b.noVerify = disable
	b.chainLock.Unlock()
}

// TotalSubsidy returns the total subsidy mined so far in the best chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) TotalSubsidy() int64 {
	b.chainLock.RLock()
	ts := b.BestSnapshot().TotalSubsidy
	b.chainLock.RUnlock()

	return ts
}

// FetchSubsidyCache returns the current subsidy cache from the blockchain.
//
// This function is safe for concurrent access.
func (b *BlockChain) FetchSubsidyCache() *SubsidyCache {
	return b.subsidyCache
}

// HaveBlock returns whether or not the chain instance has the block represented
// by the passed hash.  This includes checking the various places a block can
// be like part of the main chain, on a side chain, or in the orphan pool.
//
// This function is safe for concurrent access.
func (b *BlockChain) HaveBlock(hash *chainhash.Hash) (bool, error) {
	b.chainLock.RLock()
	exists, err := b.blockExists(hash)
	b.chainLock.RUnlock()

	if err != nil {
		return false, err
	}
	return exists || b.IsKnownOrphan(hash), nil
}

// IsKnownOrphan returns whether the passed hash is currently a known orphan.
// Keep in mind that only a limited number of orphans are held onto for a
// limited amount of time, so this function must not be used as an absolute
// way to test if a block is an orphan block.  A full block (as opposed to just
// its hash) must be passed to ProcessBlock for that purpose.  However, calling
// ProcessBlock with an orphan that already exists results in an error, so this
// function provides a mechanism for a caller to intelligently detect *recent*
// duplicate orphans and react accordingly.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsKnownOrphan(hash *chainhash.Hash) bool {
	// Protect concurrent access.  Using a read lock only so multiple
	// readers can query without blocking each other.
	b.orphanLock.RLock()
	_, exists := b.orphans[*hash]
	b.orphanLock.RUnlock()

	return exists
}

// GetOrphanRoot returns the head of the chain for the provided hash from the
// map of orphan blocks.
//
// This function is safe for concurrent access.
func (b *BlockChain) GetOrphanRoot(hash *chainhash.Hash) *chainhash.Hash {
	// Protect concurrent access.  Using a read lock only so multiple
	// readers can query without blocking each other.
	b.orphanLock.RLock()
	defer b.orphanLock.RUnlock()

	// Keep looping while the parent of each orphaned block is
	// known and is an orphan itself.
	orphanRoot := hash
	prevHash := hash
	for {
		orphan, exists := b.orphans[*prevHash]
		if !exists {
			break
		}
		orphanRoot = prevHash
		prevHash = &orphan.block.MsgBlock().Header.PrevBlock
	}

	return orphanRoot
}

// removeOrphanBlock removes the passed orphan block from the orphan pool and
// previous orphan index.
func (b *BlockChain) removeOrphanBlock(orphan *orphanBlock) {
	// Protect concurrent access.
	b.orphanLock.Lock()
	defer b.orphanLock.Unlock()

	// Remove the orphan block from the orphan pool.
	orphanHash := orphan.block.Hash()
	delete(b.orphans, *orphanHash)

	// Remove the reference from the previous orphan index too.  An indexing
	// for loop is intentionally used over a range here as range does not
	// reevaluate the slice on each iteration nor does it adjust the index
	// for the modified slice.
	prevHash := &orphan.block.MsgBlock().Header.PrevBlock
	orphans := b.prevOrphans[*prevHash]
	for i := 0; i < len(orphans); i++ {
		hash := orphans[i].block.Hash()
		if hash.IsEqual(orphanHash) {
			copy(orphans[i:], orphans[i+1:])
			orphans[len(orphans)-1] = nil
			orphans = orphans[:len(orphans)-1]
			i--
		}
	}
	b.prevOrphans[*prevHash] = orphans

	// Remove the map entry altogether if there are no longer any orphans
	// which depend on the parent hash.
	if len(b.prevOrphans[*prevHash]) == 0 {
		delete(b.prevOrphans, *prevHash)
	}
}

// addOrphanBlock adds the passed block (which is already determined to be
// an orphan prior calling this function) to the orphan pool.  It lazily cleans
// up any expired blocks so a separate cleanup poller doesn't need to be run.
// It also imposes a maximum limit on the number of outstanding orphan
// blocks and will remove the oldest received orphan block if the limit is
// exceeded.
func (b *BlockChain) addOrphanBlock(block *dcrutil.Block) {
	// Remove expired orphan blocks.
	for _, oBlock := range b.orphans {
		if time.Now().After(oBlock.expiration) {
			b.removeOrphanBlock(oBlock)
			continue
		}

		// Update the oldest orphan block pointer so it can be discarded
		// in case the orphan pool fills up.
		if b.oldestOrphan == nil ||
			oBlock.expiration.Before(b.oldestOrphan.expiration) {
			b.oldestOrphan = oBlock
		}
	}

	// Limit orphan blocks to prevent memory exhaustion.
	if len(b.orphans)+1 > maxOrphanBlocks {
		// Remove the oldest orphan to make room for the new one.
		b.removeOrphanBlock(b.oldestOrphan)
		b.oldestOrphan = nil
	}

	// Protect concurrent access.  This is intentionally done here instead
	// of near the top since removeOrphanBlock does its own locking and
	// the range iterator is not invalidated by removing map entries.
	b.orphanLock.Lock()
	defer b.orphanLock.Unlock()

	// Insert the block into the orphan map with an expiration time
	// 1 hour from now.
	expiration := time.Now().Add(time.Hour)
	oBlock := &orphanBlock{
		block:      block,
		expiration: expiration,
	}
	b.orphans[*block.Hash()] = oBlock

	// Add to previous hash lookup index for faster dependency lookups.
	prevHash := &block.MsgBlock().Header.PrevBlock
	b.prevOrphans[*prevHash] = append(b.prevOrphans[*prevHash], oBlock)

	return
}

// getGeneration gets a generation of blocks who all have the same parent by
// taking a hash as input, locating its parent node, and then returning all
// children for that parent node including the hash passed.  This can then be
// used by the mempool downstream to locate all potential block template
// parents.
func (b *BlockChain) getGeneration(h chainhash.Hash) ([]chainhash.Hash, error) {
	node, err := b.findNode(&h, maxSearchDepth)

	// This typically happens because the main chain has recently
	// reorganized and the block the miner is looking at is on
	// a fork.  Usually it corrects itself after failure.
	if err != nil {
		return nil, fmt.Errorf("couldn't find block node in best chain: %v",
			err.Error())
	}

	// Get the parent of this node.
	p, err := b.getPrevNodeFromNode(node)
	if err != nil {
		return nil, fmt.Errorf("block is orphan (parent missing)")
	}
	if p == nil {
		return nil, fmt.Errorf("no need to get children of genesis block")
	}

	// Store all the hashes in a new slice and return them.
	lenChildren := len(p.children)
	allChildren := make([]chainhash.Hash, lenChildren, lenChildren)
	for i := 0; i < lenChildren; i++ {
		allChildren[i] = p.children[i].hash
	}

	return allChildren, nil
}

// GetGeneration is the exported version of getGeneration.
func (b *BlockChain) GetGeneration(hash chainhash.Hash) ([]chainhash.Hash, error) {
	return b.getGeneration(hash)
}

// loadBlockNode loads the block identified by hash from the block database,
// creates a block node from it, and updates the memory block chain accordingly.
// It is used mainly to dynamically load previous blocks from the database as
// they are needed to avoid needing to put the entire block chain in memory.
//
// This function MUST be called with the chain state lock held (for writes).
// The database transaction may be read-only.
func (b *BlockChain) loadBlockNode(dbTx database.Tx, hash *chainhash.Hash) (*blockNode, error) {
	block, err := dbFetchBlockByHash(dbTx, hash)
	if err != nil {
		return nil, err
	}

	blockHeader := block.MsgBlock().Header
	node := newBlockNode(&blockHeader, ticketsSpentInBlock(block),
		ticketsRevokedInBlock(block), voteBitsInBlock(block))
	node.inMainChain = true
	prevHash := &blockHeader.PrevBlock

	// Add the node to the chain.
	// There are a few possibilities here:
	//  1) This node is a child of an existing block node
	//  2) This node is the parent of one or more nodes
	//  3) Neither 1 or 2 is true which implies it's an orphan block and
	//     therefore is an error to insert into the chain
	if parentNode, ok := b.index[*prevHash]; ok {
		// Case 1 -- This node is a child of an existing block node.
		// Update the node's work sum with the sum of the parent node's
		// work sum and this node's work, append the node as a child of
		// the parent node and set this node's parent to the parent
		// node.
		node.workSum = node.workSum.Add(parentNode.workSum, node.workSum)
		parentNode.children = append(parentNode.children, node)
		node.parent = parentNode
	} else if childNodes, ok := b.depNodes[*hash]; ok {
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
		// Case 3 -- The node doesn't have a parent in the node cache
		// and is not the parent of another node.  This means an arbitrary
		// orphan block is trying to be loaded which is not allowed.
		// Before we return, check and make sure there isn't a parent
		// further down the line in the blockchain to which the block
		// could be attached, for example if the node had been pruned from
		// the index.
		foundParent, err := b.findNode(&node.header.PrevBlock, maxSearchDepth)
		if err == nil {
			node.workSum = node.workSum.Add(foundParent.workSum, node.workSum)
			foundParent.children = append(foundParent.children, node)
			node.parent = foundParent
		} else {
			str := "loadBlockNode: attempt to insert orphan block %v"
			return nil, AssertError(fmt.Sprintf(str, hash))
		}
	}

	// Add the new node to the indices for faster lookups.
	b.index[*hash] = node
	b.depNodes[*prevHash] = append(b.depNodes[*prevHash], node)

	return node, nil
}

// findNode finds the node scaling backwards from best chain or return an
// error.  If searchDepth equal zero there is no searchDepth.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) findNode(nodeHash *chainhash.Hash, searchDepth int) (*blockNode, error) {
	var node *blockNode
	err := b.db.View(func(dbTx database.Tx) error {
		// Most common case; we're checking a block that wants to be connected
		// on top of the current main chain.
		distance := 0
		if *nodeHash == b.bestNode.hash {
			node = b.bestNode
		} else {
			// Look backwards in our blockchain and try to find it in the
			// parents of blocks.
			foundPrev := b.bestNode
			notFound := true
			for !foundPrev.hash.IsEqual(b.chainParams.GenesisHash) {
				if searchDepth != 0 && distance >= searchDepth {
					break
				}

				if foundPrev.hash.IsEqual(nodeHash) {
					notFound = false
					break
				}

				last := foundPrev.header.PrevBlock
				foundPrev = foundPrev.parent
				if foundPrev == nil {
					parent, err := b.loadBlockNode(dbTx, &last)
					if err != nil {
						return err
					}

					foundPrev = parent
				}

				distance++
			}

			if notFound {
				return fmt.Errorf("couldn't find node %v in best chain",
					nodeHash)
			}

			node = foundPrev
		}

		return nil
	})

	return node, err
}

// getPrevNodeFromBlock returns a block node for the block previous to the
// passed block (the passed block's parent).  When it is already in the memory
// block chain, it simply returns it.  Otherwise, it loads the previous block
// header from the block database, creates a new block node from it, and returns
// it.  The returned node will be nil if the genesis block is passed.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) getPrevNodeFromBlock(block *dcrutil.Block) (*blockNode,
	error) {
	// Genesis block.
	prevHash := &block.MsgBlock().Header.PrevBlock
	if prevHash.IsEqual(zeroHash) {
		return nil, nil
	}

	// Return the existing previous block node if it's already there.
	if bn, ok := b.index[*prevHash]; ok {
		return bn, nil
	}

	// Dynamically load the previous block from the block database, create
	// a new block node for it, and update the memory chain accordingly.
	var prevBlockNode *blockNode
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		prevBlockNode, err = b.loadBlockNode(dbTx, prevHash)
		return err
	})
	return prevBlockNode, err
}

// getPrevNodeFromNode returns a block node for the block previous to the
// passed block node (the passed block node's parent).  When the node is already
// connected to a parent, it simply returns it.  Otherwise, it loads the
// associated block from the database to obtain the previous hash and uses that
// to dynamically create a new block node and return it.  The memory block
// chain is updated accordingly.  The returned node will be nil if the genesis
// block is passed.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) getPrevNodeFromNode(node *blockNode) (*blockNode, error) {
	// Return the existing previous block node if it's already there.
	if node.parent != nil {
		return node.parent, nil
	}

	// Genesis block.
	if node.hash.IsEqual(b.chainParams.GenesisHash) {
		return nil, nil
	}

	// Dynamically load the previous block from the block database, create
	// a new block node for it, and update the memory chain accordingly.
	var prevBlockNode *blockNode
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		prevBlockNode, err = b.loadBlockNode(dbTx, &node.header.PrevBlock)
		return err
	})
	return prevBlockNode, err
}

// ancestorNode returns the ancestor block node at the provided height by
// following the chain backwards from the given node while dynamically loading
// any pruned nodes from the database and updating the memory block chain as
// needed.  The returned block will be nil when a height is requested that is
// after the height of the passed node or is less than zero.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) ancestorNode(node *blockNode, height int64) (*blockNode, error) {
	// Nothing to do if the requested height is outside of the valid range.
	if height > node.height || height < 0 {
		return nil, nil
	}

	// Iterate backwards until the requested height is reached.
	iterNode := node
	for iterNode != nil && iterNode.height > height {
		// Get the previous block node.  This function is used over
		// simply accessing iterNode.parent directly as it will
		// dynamically create previous block nodes as needed.  This
		// helps allow only the pieces of the chain that are needed
		// to remain in memory.
		var err error
		iterNode, err = b.getPrevNodeFromNode(iterNode)
		if err != nil {
			log.Errorf("getPrevNodeFromNode: %v", err)
			return nil, err
		}
	}

	return iterNode, nil
}

// fetchBlockFromHash searches the internal chain block stores and the database in
// an attempt to find the block.  If it finds the block, it returns it.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) fetchBlockFromHash(hash *chainhash.Hash) (*dcrutil.Block,
	error) {
	// Check side chain block cache
	b.blockCacheLock.RLock()
	blockSidechain, existsSidechain := b.blockCache[*hash]
	b.blockCacheLock.RUnlock()
	if existsSidechain {
		return blockSidechain, nil
	}

	// Check orphan cache
	b.orphanLock.RLock()
	orphan, existsOrphans := b.orphans[*hash]
	b.orphanLock.RUnlock()
	if existsOrphans {
		return orphan.block, nil
	}

	// Check main chain
	b.mainchainBlockCacheLock.RLock()
	block, ok := b.mainchainBlockCache[*hash]
	b.mainchainBlockCacheLock.RUnlock()
	if ok {
		return block, nil
	}

	var blockMainchain *dcrutil.Block
	errFetchMainchain := b.db.View(func(dbTx database.Tx) error {
		var err error
		blockMainchain, err = dbFetchBlockByHash(dbTx, hash)
		return err
	})
	if errFetchMainchain == nil && blockMainchain != nil {
		return blockMainchain, nil
	}

	// Implicit !existsMainchain && !existsSidechain && !existsOrphans
	return nil, fmt.Errorf("unable to find block %v in "+
		"side chain cache, orphan cache, and main chain db", hash)
}

// FetchBlockFromHash is the generalized and exported version of
// fetchBlockFromHash.  It is safe for concurrent access.
func (b *BlockChain) FetchBlockFromHash(hash *chainhash.Hash) (*dcrutil.Block,
	error) {
	return b.fetchBlockFromHash(hash)
}

// GetTopBlock returns the current block at HEAD on the blockchain.  Needed
// for mining in the daemon.
func (b *BlockChain) GetTopBlock() (*dcrutil.Block, error) {
	block, err := b.fetchBlockFromHash(&b.bestNode.hash)
	return block, err
}

// removeBlockNode removes the passed block node from the memory chain by
// unlinking all of its children and removing it from the the node and
// dependency indices.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) removeBlockNode(node *blockNode) error {
	if node.parent != nil {
		return AssertError(fmt.Sprintf("removeBlockNode must be "+
			"called with a node at the front of the chain - node %v",
			node.hash))
	}

	// Remove the node from the node index.
	delete(b.index, node.hash)

	// Unlink all of the node's children.
	for _, child := range node.children {
		child.parent = nil
	}
	node.children = nil

	// Remove the reference from the dependency index.
	prevHash := &node.header.PrevBlock
	if children, ok := b.depNodes[*prevHash]; ok {
		// Find the node amongst the children of the
		// dependencies for the parent hash and remove it.
		b.depNodes[*prevHash] = removeChildNode(children, node)

		// Remove the map entry altogether if there are no
		// longer any nodes which depend on the parent hash.
		if len(b.depNodes[*prevHash]) == 0 {
			delete(b.depNodes, *prevHash)
		}
	}

	return nil
}

// pruneBlockNodes removes references to old block nodes which are no longer
// needed so they may be garbage collected.  In order to validate block rules
// and choose the best chain, only a portion of the nodes which form the block
// chain are needed in memory.  This function walks the chain backwards from the
// current best chain to find any nodes before the first needed block node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) pruneBlockNodes() error {
	// Walk the chain backwards to find what should be the new root node.
	// Intentionally use node.parent instead of getPrevNodeFromNode since
	// the latter loads the node and the goal is to find nodes still in
	// memory that can be pruned.
	newRootNode := b.bestNode
	for i := int64(0); i < minMemoryNodes-1 && newRootNode != nil; i++ {
		newRootNode = newRootNode.parent
	}

	// Nothing to do if there are not enough nodes.
	if newRootNode == nil || newRootNode.parent == nil {
		return nil
	}

	// Push the nodes to delete on a list in reverse order since it's easier
	// to prune them going forwards than it is backwards.  This will
	// typically end up being a single node since pruning is currently done
	// just before each new node is created.  However, that might be tuned
	// later to only prune at intervals, so the code needs to account for
	// the possibility of multiple nodes.
	deleteNodes := list.New()
	for node := newRootNode.parent; node != nil; node = node.parent {
		deleteNodes.PushFront(node)
	}

	// Loop through each node to prune, unlink its children, remove it from
	// the dependency index, and remove it from the node index.
	for e := deleteNodes.Front(); e != nil; e = e.Next() {
		node := e.Value.(*blockNode)
		// Do not attempt to prune if the node should already have been pruned,
		// for example if you're adding an old side chain block.
		if node.height > b.bestNode.height-minMemoryNodes {
			err := b.removeBlockNode(node)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// pruneStakeNodes removes references to old stake nodes which should no
// longer be held in memory so as to keep the maximum memory usage down.
// It proceeds from the bestNode back to the determined minimum height node,
// finds all the relevant children, and then drops the the stake nodes from
// them by assigning nil and allowing the memory to be recovered by GC.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) pruneStakeNodes() {
	// Find the height to prune to.
	pruneToNode := b.bestNode
	for i := int64(0); i < minMemoryStakeNodes-1 && pruneToNode != nil; i++ {
		pruneToNode = pruneToNode.parent
	}

	// Nothing to do if there are not enough nodes.
	if pruneToNode == nil || pruneToNode.parent == nil {
		return
	}

	// Push the nodes to delete on a list in reverse order since it's easier
	// to prune them going forwards than it is backwards.  This will
	// typically end up being a single node since pruning is currently done
	// just before each new node is created.  However, that might be tuned
	// later to only prune at intervals, so the code needs to account for
	// the possibility of multiple nodes.
	deleteNodes := list.New()
	for node := pruneToNode.parent; node != nil; node = node.parent {
		deleteNodes.PushFront(node)
	}

	// Loop through each node to prune, unlink its children, remove it from
	// the dependency index, and remove it from the node index.
	for e := deleteNodes.Front(); e != nil; e = e.Next() {
		node := e.Value.(*blockNode)
		// Do not attempt to prune if the node should already have been pruned,
		// for example if you're adding an old side chain block.
		if node.height > b.bestNode.height-minMemoryNodes {
			node.stakeNode = nil
			node.stakeUndoData = nil
			node.newTickets = nil
			node.ticketsSpent = nil
			node.ticketsRevoked = nil
		}
	}
}

// pruneNodes tranverses the blockchain and prunes nodes and stake data from
// memory so that the memory can be recovered by the garbage collector.  This
// allows the caller of the blockchain to manually handle GC related to the
// blockchain.
//
// This function is NOT safe for concurrent access and must be called with
// the chain lock held for writes.
func (b *BlockChain) pruneNodes() error {
	b.pruneStakeNodes()

	return b.pruneBlockNodes()
}

// BestPrevHash returns the hash of the previous block of the block at HEAD.
//
// This function is safe for concurrent access.
func (b *BlockChain) BestPrevHash() chainhash.Hash {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()

	return b.bestNode.header.PrevBlock
}

// isMajorityVersion determines if a previous number of blocks in the chain
// starting with startNode are at least the minimum passed version.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isMajorityVersion(minVer int32, startNode *blockNode, numRequired uint64) bool {
	numFound := uint64(0)
	iterNode := startNode
	for i := uint64(0); i < b.chainParams.BlockUpgradeNumToCheck &&
		numFound < numRequired && iterNode != nil; i++ {
		// This node has a version that is at least the minimum version.
		if iterNode.header.Version >= minVer {
			numFound++
		}

		// Get the previous block node.  This function is used over
		// simply accessing iterNode.parent directly as it will
		// dynamically create previous block nodes as needed.  This
		// helps allow only the pieces of the chain that are needed
		// to remain in memory.
		var err error
		iterNode, err = b.getPrevNodeFromNode(iterNode)
		if err != nil {
			break
		}
	}

	return numFound >= numRequired
}

// calcPastMedianTime calculates the median time of the previous few blocks
// prior to, and including, the passed block node.  It is primarily used to
// validate new blocks have sane timestamps.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) calcPastMedianTime(startNode *blockNode) (time.Time, error) {
	// Genesis block.
	if startNode == nil {
		return b.chainParams.GenesisBlock.Header.Timestamp, nil
	}

	// Create a slice of the previous few block timestamps used to calculate
	// the median per the number defined by the constant medianTimeBlocks.
	timestamps := make([]time.Time, medianTimeBlocks)
	numNodes := 0
	iterNode := startNode
	for i := 0; i < medianTimeBlocks && iterNode != nil; i++ {
		timestamps[i] = iterNode.header.Timestamp
		numNodes++

		// Get the previous block node.  This function is used over
		// simply accessing iterNode.parent directly as it will
		// dynamically create previous block nodes as needed.  This
		// helps allow only the pieces of the chain that are needed
		// to remain in memory.
		var err error
		iterNode, err = b.getPrevNodeFromNode(iterNode)
		if err != nil {
			log.Errorf("getPrevNodeFromNode failed to find node: %v", err)
			return time.Time{}, err
		}
	}

	// Prune the slice to the actual number of available timestamps which
	// will be fewer than desired near the beginning of the block chain
	// and sort them.
	timestamps = timestamps[:numNodes]
	sort.Sort(timeSorter(timestamps))

	// NOTE: bitcoind incorrectly calculates the median for even numbers of
	// blocks.  A true median averages the middle two elements for a set
	// with an even number of elements in it.   Since the constant for the
	// previous number of blocks to be used is odd, this is only an issue
	// for a few blocks near the beginning of the chain.  I suspect this is
	// an optimization even though the result is slightly wrong for a few
	// of the first blocks since after the first few blocks, there will
	// always be an odd number of blocks in the set per the constant.
	//
	// This code follows suit to ensure the same rules are used as bitcoind
	// however, be aware that should the medianTimeBlocks constant ever be
	// changed to an even number, this code will be wrong.
	medianTimestamp := timestamps[numNodes/2]
	return medianTimestamp, nil
}

// getReorganizeNodes finds the fork point between the main chain and the passed
// node and returns a list of block nodes that would need to be detached from
// the main chain and a list of block nodes that would need to be attached to
// the fork point (which will be the end of the main chain after detaching the
// returned list of block nodes) in order to reorganize the chain such that the
// passed node is the new end of the main chain.  The lists will be empty if the
// passed node is not on a side chain.
//
// This function MUST be called with the chain state lock held (for reads).
func (b *BlockChain) getReorganizeNodes(node *blockNode) (*list.List, *list.List, error) {
	// Nothing to detach or attach if there is no node.
	attachNodes := list.New()
	detachNodes := list.New()
	if node == nil {
		return detachNodes, attachNodes, nil
	}

	// Find the fork point (if any) adding each block to the list of nodes
	// to attach to the main tree.  Push them onto the list in reverse order
	// so they are attached in the appropriate order when iterating the list
	// later.
	ancestor := node
	for ; ancestor.parent != nil; ancestor = ancestor.parent {
		if ancestor.inMainChain {
			break
		}
		attachNodes.PushFront(ancestor)
	}

	// TODO(davec): Use prevNodeFromNode function in case the requested
	// node is further back than the what is in memory.  This shouldn't
	// happen in the normal course of operation, but the ability to fetch
	// input transactions of arbitrary blocks will likely to be exposed at
	// some point and that could lead to an issue here.

	// Start from the end of the main chain and work backwards until the
	// common ancestor adding each block to the list of nodes to detach from
	// the main chain.
	for n := b.bestNode; n != nil; n = n.parent {
		if n.hash == ancestor.hash {
			break
		}
		detachNodes.PushBack(n)

		if n.parent == nil {
			var err error
			n.parent, err = b.findNode(&n.header.PrevBlock, maxSearchDepth)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	return detachNodes, attachNodes, nil
}

// pushMainChainBlockCache pushes a block onto the main chain block cache,
// and removes any old blocks from the cache that might be present.
func (b *BlockChain) pushMainChainBlockCache(block *dcrutil.Block) {
	curHeight := block.Height()
	curHash := block.Hash()
	b.mainchainBlockCacheLock.Lock()
	b.mainchainBlockCache[*curHash] = block
	for hash, bl := range b.mainchainBlockCache {
		if bl.Height() <= curHeight-int64(b.mainchainBlockCacheSize) {
			delete(b.mainchainBlockCache, hash)
		}
	}
	b.mainchainBlockCacheLock.Unlock()
}

// dbMaybeStoreBlock stores the provided block in the database if it's not
// already there.
func dbMaybeStoreBlock(dbTx database.Tx, block *dcrutil.Block) error {
	hasBlock, err := dbTx.HasBlock(block.Hash())
	if err != nil {
		return err
	}
	if hasBlock {
		return nil
	}

	return dbTx.StoreBlock(block)
}

// connectBlock handles connecting the passed node/block to the end of the main
// (best) chain.
//
// This passed utxo view must have all referenced txos the block spends marked
// as spent and all of the new txos the block creates added to it.  In addition,
// the passed stxos slice must be populated with all of the information for the
// spent txos.  This approach is used because the connection validation that
// must happen prior to calling this function requires the same details, so
// it would be inefficient to repeat it.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) connectBlock(node *blockNode, block *dcrutil.Block, view *UtxoViewpoint, stxos []spentTxOut) error {
	// Make sure it's extending the end of the best chain.
	prevHash := block.MsgBlock().Header.PrevBlock
	if prevHash != b.bestNode.hash {
		return AssertError("connectBlock must be called with a block " +
			"that extends the main chain")
	}

	// Sanity check the correct number of stxos are provided.
	parent, err := b.fetchBlockFromHash(&node.parent.hash)
	if err != nil {
		return err
	}
	if len(stxos) != countSpentOutputs(block, parent) {
		return AssertError("connectBlock called with inconsistent " +
			"spent transaction out information")
	}

	// Calculate the median time for the block.
	medianTime, err := b.calcPastMedianTime(node)
	if err != nil {
		return err
	}

	// Generate a new best state snapshot that will be used to update the
	// database and later memory if all database updates are successful.
	b.stateLock.RLock()
	curTotalTxns := b.stateSnapshot.TotalTxns
	curTotalSubsidy := b.stateSnapshot.TotalSubsidy
	b.stateLock.RUnlock()

	// Calculate the number of transactions that would be added by adding
	// this block.
	numTxns := countNumberOfTransactions(block, parent)

	// Calculate the exact subsidy produced by adding the block.
	subsidy := CalculateAddedSubsidy(block, parent)

	blockSize := uint64(block.MsgBlock().Header.Size)
	state := newBestState(node, blockSize, numTxns, curTotalTxns+numTxns,
		medianTime, curTotalSubsidy+subsidy)

	// Get the stake node for this node, filling in any data that
	// may have yet to have been filled in.  In all cases this
	// should simply give a pointer to data already prepared, but
	// run this anyway to be safe.
	stakeNode, err := b.fetchStakeNode(node)
	if err != nil {
		return err
	}

	// Atomically insert info into the database.
	err = b.db.Update(func(dbTx database.Tx) error {
		// Update best block state.
		err := dbPutBestState(dbTx, state, node.workSum)
		if err != nil {
			return err
		}

		// Add the block hash and height to the block index which tracks
		// the main chain.
		err = dbPutBlockIndex(dbTx, block.Hash(), node.height)
		if err != nil {
			return err
		}

		// Update the utxo set using the state of the utxo view.  This
		// entails removing all of the utxos spent and adding the new
		// ones created by the block.
		err = dbPutUtxoView(dbTx, view)
		if err != nil {
			return err
		}

		// Update the transaction spend journal by adding a record for
		// the block that contains all txos spent by it.
		err = dbPutSpendJournalEntry(dbTx, block.Hash(), stxos)
		if err != nil {
			return err
		}

		// Insert the block into the database if it's not already there.
		err = dbMaybeStoreBlock(dbTx, block)
		if err != nil {
			return err
		}

		// Insert the block into the stake database.
		err = stake.WriteConnectedBestNode(dbTx, stakeNode, node.hash)
		if err != nil {
			return err
		}

		// Allow the index manager to call each of the currently active
		// optional indexes with the block being connected so they can
		// update themselves accordingly.
		if b.indexManager != nil {
			err := b.indexManager.ConnectBlock(dbTx, block, parent, view)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Prune fully spent entries and mark all entries in the view unmodified
	// now that the modifications have been committed to the database.
	view.commit()

	// Add the new node to the memory main chain indices for faster
	// lookups.
	node.inMainChain = true
	b.index[node.hash] = node
	b.depNodes[prevHash] = append(b.depNodes[prevHash], node)

	// This node is now the end of the best chain.
	b.bestNode = node

	// Update the state for the best block.  Notice how this replaces the
	// entire struct instead of updating the existing one.  This effectively
	// allows the old version to act as a snapshot which callers can use
	// freely without needing to hold a lock for the duration.  See the
	// comments on the state variable for more details.
	b.stateLock.Lock()
	b.stateSnapshot = state
	b.stateLock.Unlock()

	// Send stake notifications about the new block.
	if node.height >= b.chainParams.StakeEnabledHeight {
		nextStakeDiff, err := b.calcNextRequiredStakeDifficulty(node)
		if err != nil {
			return err
		}

		// Notify of spent and missed tickets
		b.sendNotification(NTSpentAndMissedTickets,
			&TicketNotificationsData{
				Hash:            node.hash,
				Height:          node.height,
				StakeDifficulty: nextStakeDiff,
				TicketsSpent:    node.stakeNode.SpentByBlock(),
				TicketsMissed:   node.stakeNode.MissedByBlock(),
				TicketsNew:      []chainhash.Hash{},
			})
		// Notify of new tickets
		b.sendNotification(NTNewTickets,
			&TicketNotificationsData{
				Hash:            node.hash,
				Height:          node.height,
				StakeDifficulty: nextStakeDiff,
				TicketsSpent:    []chainhash.Hash{},
				TicketsMissed:   []chainhash.Hash{},
				TicketsNew:      node.stakeNode.NewTickets(),
			})
	}

	// Assemble the current block and the parent into a slice.
	blockAndParent := []*dcrutil.Block{block, parent}

	// Notify the caller that the block was connected to the main chain.
	// The caller would typically want to react with actions such as
	// updating wallets.
	b.chainLock.Unlock()
	b.sendNotification(NTBlockConnected, blockAndParent)
	b.chainLock.Lock()

	// Optimization: Before checkpoints, immediately dump the parent's stake
	// node because we no longer need it.
	if node.height < b.chainParams.LatestCheckpointHeight() {
		b.bestNode.parent.stakeNode = nil
		b.bestNode.parent.stakeUndoData = nil
		b.bestNode.parent.newTickets = nil
		b.bestNode.parent.ticketsSpent = nil
		b.bestNode.parent.ticketsRevoked = nil
	}

	b.pushMainChainBlockCache(block)

	return nil
}

// dropMainChainBlockCache drops a block from the main chain block cache.
func (b *BlockChain) dropMainChainBlockCache(block *dcrutil.Block) {
	curHash := block.Hash()
	b.mainchainBlockCacheLock.Lock()
	delete(b.mainchainBlockCache, *curHash)
	b.mainchainBlockCacheLock.Unlock()
}

// disconnectBlock handles disconnecting the passed node/block from the end of
// the main (best) chain.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) disconnectBlock(node *blockNode, block *dcrutil.Block, view *UtxoViewpoint) error {
	// Make sure the node being disconnected is the end of the best chain.
	if node.hash != b.bestNode.hash {
		return AssertError("disconnectBlock must be called with the " +
			"block at the end of the main chain")
	}

	// Get the previous block node.  This function is used over simply
	// accessing node.parent directly as it will dynamically create previous
	// block nodes as needed.  This helps allow only the pieces of the chain
	// that are needed to remain in memory.
	prevNode, err := b.getPrevNodeFromNode(node)
	if err != nil {
		return err
	}

	// Calculate the median time for the previous block.
	medianTime, err := b.calcPastMedianTime(prevNode)
	if err != nil {
		return err
	}

	// Load the previous block since some details for it are needed below.
	parent, err := b.fetchBlockFromHash(&prevNode.hash)
	if err != nil {
		return err
	}

	// Generate a new best state snapshot that will be used to update the
	// database and later memory if all database updates are successful.
	b.stateLock.RLock()
	curTotalTxns := b.stateSnapshot.TotalTxns
	curTotalSubsidy := b.stateSnapshot.TotalSubsidy
	b.stateLock.RUnlock()
	parentBlockSize := uint64(parent.MsgBlock().Header.Size)

	// Calculate the number of transactions that would be added by adding
	// this block.
	numTxns := countNumberOfTransactions(block, parent)
	newTotalTxns := curTotalTxns - numTxns

	// Calculate the exact subsidy produced by adding the block.
	subsidy := CalculateAddedSubsidy(block, parent)
	newTotalSubsidy := curTotalSubsidy - subsidy

	state := newBestState(prevNode, parentBlockSize, numTxns, newTotalTxns,
		medianTime, newTotalSubsidy)

	// Prepare the information required to update the stake database
	// contents.
	childStakeNode, err := b.fetchStakeNode(node)
	if err != nil {
		return err
	}
	parentStakeNode, err := b.fetchStakeNode(node.parent)
	if err != nil {
		return err
	}

	err = b.db.Update(func(dbTx database.Tx) error {
		// Update best block state.
		err := dbPutBestState(dbTx, state, node.workSum)
		if err != nil {
			return err
		}

		// Remove the block hash and height from the block index which
		// tracks the main chain.
		err = dbRemoveBlockIndex(dbTx, block.Hash(), node.height)
		if err != nil {
			return err
		}

		// Update the utxo set using the state of the utxo view.  This
		// entails restoring all of the utxos spent and removing the new
		// ones created by the block.
		err = dbPutUtxoView(dbTx, view)
		if err != nil {
			return err
		}

		// Update the transaction spend journal by removing the record
		// that contains all txos spent by the block .
		err = dbRemoveSpendJournalEntry(dbTx, block.Hash())
		if err != nil {
			return err
		}

		err = stake.WriteDisconnectedBestNode(dbTx, parentStakeNode,
			node.parent.hash, childStakeNode.UndoData())
		if err != nil {
			return err
		}

		// Allow the index manager to call each of the currently active
		// optional indexes with the block being disconnected so they
		// can update themselves accordingly.
		if b.indexManager != nil {
			err := b.indexManager.DisconnectBlock(dbTx, block, parent, view)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Prune fully spent entries and mark all entries in the view unmodified
	// now that the modifications have been committed to the database.
	view.commit()

	// Put block in the side chain cache.
	node.inMainChain = false
	b.blockCacheLock.Lock()
	b.blockCache[node.hash] = block
	b.blockCacheLock.Unlock()

	// This node's parent is now the end of the best chain.
	b.bestNode = node.parent

	// Update the state for the best block.  Notice how this replaces the
	// entire struct instead of updating the existing one.  This effectively
	// allows the old version to act as a snapshot which callers can use
	// freely without needing to hold a lock for the duration.  See the
	// comments on the state variable for more details.
	b.stateLock.Lock()
	b.stateSnapshot = state
	b.stateLock.Unlock()

	// Assemble the current block and the parent into a slice.
	blockAndParent := []*dcrutil.Block{block, parent}

	// Notify the caller that the block was disconnected from the main
	// chain.  The caller would typically want to react with actions such as
	// updating wallets.
	b.chainLock.Unlock()
	b.sendNotification(NTBlockDisconnected, blockAndParent)
	b.chainLock.Lock()

	b.dropMainChainBlockCache(block)

	return nil
}

// countSpentOutputs returns the number of utxos the passed block spends.
func countSpentOutputs(block *dcrutil.Block, parent *dcrutil.Block) int {
	// We need to skip the regular tx tree if it's not valid.
	// We also exclude the coinbase transaction since it can't
	// spend anything.
	regularTxTreeValid := dcrutil.IsFlagSet16(block.MsgBlock().Header.VoteBits,
		dcrutil.BlockValid)
	var numSpent int
	if regularTxTreeValid {
		for _, tx := range parent.Transactions()[1:] {
			numSpent += len(tx.MsgTx().TxIn)
		}
	}
	for _, stx := range block.MsgBlock().STransactions {
		txType := stake.DetermineTxType(stx)
		if txType == stake.TxTypeSSGen || txType == stake.TxTypeSSRtx {
			numSpent++
			continue
		}
		numSpent += len(stx.TxIn)
	}

	return numSpent
}

// countNumberOfTransactions returns the number of transactions inserted by
// adding the block.
func countNumberOfTransactions(block, parent *dcrutil.Block) uint64 {
	var numTxns uint64

	regularTxTreeValid := dcrutil.IsFlagSet16(block.MsgBlock().Header.VoteBits,
		dcrutil.BlockValid)
	if regularTxTreeValid {
		numTxns += uint64(len(parent.Transactions()))
	}
	numTxns += uint64(len(block.STransactions()))

	return numTxns
}

// reorganizeChain reorganizes the block chain by disconnecting the nodes in the
// detachNodes list and connecting the nodes in the attach list.  It expects
// that the lists are already in the correct order and are in sync with the
// end of the current best chain.  Specifically, nodes that are being
// disconnected must be in reverse order (think of popping them off the end of
// the chain) and nodes the are being attached must be in forwards order
// (think pushing them onto the end of the chain).
//
// The flags modify the behavior of this function as follows:
//  - BFDryRun: Only the checks which ensure the reorganize can be completed
//    successfully are performed.  The chain is not reorganized.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) reorganizeChain(detachNodes, attachNodes *list.List,
	flags BehaviorFlags) error {
	formerBestHash := b.bestNode.hash
	formerBestHeight := b.bestNode.height

	// Ensure all of the needed side chain blocks are in the cache.
	for e := attachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)

		b.blockCacheLock.RLock()
		_, exists := b.blockCache[n.hash]
		b.blockCacheLock.RUnlock()
		if !exists {
			return AssertError(fmt.Sprintf("block %v is missing "+
				"from the side chain block cache", n.hash))
		}
	}

	// All of the blocks to detach and related spend journal entries needed
	// to unspend transaction outputs in the blocks being disconnected must
	// be loaded from the database during the reorg check phase below and
	// then they are needed again when doing the actual database updates.
	// Rather than doing two loads, cache the loaded data into these slices.
	detachBlocks := make([]*dcrutil.Block, 0, detachNodes.Len())
	detachSpentTxOuts := make([][]spentTxOut, 0, detachNodes.Len())

	// Disconnect all of the blocks back to the point of the fork.  This
	// entails loading the blocks and their associated spent txos from the
	// database and using that information to unspend all of the spent txos
	// and remove the utxos created by the blocks.
	view := NewUtxoViewpoint()
	view.SetBestHash(&formerBestHash)
	view.SetStakeViewpoint(ViewpointPrevValidInitial)
	i := 0
	for e := detachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		var block *dcrutil.Block
		var parent *dcrutil.Block
		var err error
		block, err = b.fetchBlockFromHash(&n.hash)
		if err != nil {
			return err
		}
		parent, err = b.fetchBlockFromHash(&n.header.PrevBlock)
		if err != nil {
			return err
		}

		// Load all of the spent txos for the block from the spend
		// journal.
		var stxos []spentTxOut
		err = b.db.View(func(dbTx database.Tx) error {
			stxos, err = dbFetchSpendJournalEntry(dbTx, block, parent)
			return err
		})
		if err != nil {
			return err
		}

		// Quick sanity test.
		if len(stxos) != countSpentOutputs(block, parent) {
			return AssertError(fmt.Sprintf("retrieved %v stxos when trying to "+
				"disconnect block %v (height %v), yet counted %v "+
				"many spent utxos", len(stxos), block.Hash(), block.Height(),
				countSpentOutputs(block, parent)))
		}

		// Store the loaded block and spend journal entry for later.
		detachBlocks = append(detachBlocks, block)
		detachSpentTxOuts = append(detachSpentTxOuts, stxos)

		err = b.disconnectTransactions(view, block, parent, stxos)
		if err != nil {
			return err
		}

		i++
	}

	// Perform several checks to verify each block that needs to be attached
	// to the main chain can be connected without violating any rules and
	// without actually connecting the block.
	//
	// NOTE: These checks could be done directly when connecting a block,
	// however the downside to that approach is that if any of these checks
	// fail after disconnecting some blocks or attaching others, all of the
	// operations have to be rolled back to get the chain back into the
	// state it was before the rule violation (or other failure).  There are
	// at least a couple of ways accomplish that rollback, but both involve
	// tweaking the chain and/or database.  This approach catches these
	// issues before ever modifying the chain.
	var topBlock *blockNode
	for e := attachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		b.blockCacheLock.RLock()
		block := b.blockCache[n.hash]
		b.blockCacheLock.RUnlock()

		// Notice the spent txout details are not requested here and
		// thus will not be generated.  This is done because the state
		// is not being immediately written to the database, so it is
		// not needed.
		err := b.checkConnectBlock(n, block, view, nil)
		if err != nil {
			return err
		}
		topBlock = n
	}
	newHash := topBlock.hash
	newHeight := topBlock.height
	log.Debugf("New best chain validation completed successfully, " +
		"commencing with the reorganization.")

	// Skip disconnecting and connecting the blocks when running with the
	// dry run flag set.
	if flags&BFDryRun == BFDryRun {
		return nil
	}

	// Send a notification that a blockchain reorganization is in progress.
	reorgData := &ReorganizationNtfnsData{
		formerBestHash,
		formerBestHeight,
		newHash,
		newHeight,
	}
	b.chainLock.Unlock()
	b.sendNotification(NTReorganization, reorgData)
	b.chainLock.Lock()

	// Reset the view for the actual connection code below.  This is
	// required because the view was previously modified when checking if
	// the reorg would be successful and the connection code requires the
	// view to be valid from the viewpoint of each block being connected or
	// disconnected.
	view = NewUtxoViewpoint()
	view.SetBestHash(&formerBestHash)
	view.SetStakeViewpoint(ViewpointPrevValidInitial)

	// Disconnect blocks from the main chain.
	for i, e := 0, detachNodes.Front(); e != nil; i, e = i+1, e.Next() {
		n := e.Value.(*blockNode)
		block := detachBlocks[i]
		parent, err := b.fetchBlockFromHash(&n.header.PrevBlock)
		if err != nil {
			return err
		}

		// Load all of the utxos referenced by the block that aren't
		// already in the view.
		err = view.fetchInputUtxos(b.db, block, parent)
		if err != nil {
			return err
		}

		// Update the view to unspend all of the spent txos and remove
		// the utxos created by the block.
		err = b.disconnectTransactions(view, block, parent,
			detachSpentTxOuts[i])
		if err != nil {
			return err
		}

		// Update the database and chain state.
		err = b.disconnectBlock(n, block, view)
		if err != nil {
			return err
		}
	}

	// Connect the new best chain blocks.
	for e := attachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		b.blockCacheLock.RLock()
		block := b.blockCache[n.hash]
		b.blockCacheLock.RUnlock()

		parent, err := b.fetchBlockFromHash(&n.header.PrevBlock)
		if err != nil {
			return err
		}

		// Update the view to mark all utxos referenced by the block
		// as spent and add all transactions being created by this block
		// to it.  Also, provide an stxo slice so the spent txout
		// details are generated.
		stxos := make([]spentTxOut, 0, countSpentOutputs(block, parent))
		err = b.connectTransactions(view, block, parent, &stxos)
		if err != nil {
			return err
		}

		// Update the database and chain state.
		err = b.connectBlock(n, block, view, stxos)
		if err != nil {
			return err
		}
		delete(b.blockCache, n.hash)
	}

	// Log the point where the chain forked.
	firstAttachNode := attachNodes.Front().Value.(*blockNode)
	forkNode, err := b.getPrevNodeFromNode(firstAttachNode)
	if err == nil {
		log.Infof("REORGANIZE: Chain forks at %v, height %v",
			forkNode.hash,
			forkNode.height)
	}

	// Log the old and new best chain heads.
	lastAttachNode := attachNodes.Back().Value.(*blockNode)
	log.Infof("REORGANIZE: Old best chain head was %v, height %v",
		formerBestHash,
		formerBestHeight)
	log.Infof("REORGANIZE: New best chain head is %v, height %v",
		lastAttachNode.hash,
		lastAttachNode.height)

	return nil
}

// forceReorganizationToBlock forces a reorganization of the block chain to the
// block hash requested, so long as it matches up with the current organization
// of the best chain.
func (b *BlockChain) forceHeadReorganization(formerBest chainhash.Hash, newBest chainhash.Hash) error {
	if formerBest.IsEqual(&newBest) {
		return fmt.Errorf("can't reorganize to the same block")
	}
	formerBestNode := b.bestNode

	// We can't reorganize the chain unless our head block matches up with
	// b.bestChain.
	if !formerBestNode.hash.IsEqual(&formerBest) {
		return ruleError(ErrForceReorgWrongChain, "tried to force reorg "+
			"on wrong chain")
	}

	var newBestNode *blockNode
	for _, n := range formerBestNode.parent.children {
		if n.hash.IsEqual(&newBest) {
			newBestNode = n
		}
	}

	// Child to reorganize to is missing.
	if newBestNode == nil {
		return ruleError(ErrForceReorgMissingChild, "missing child of "+
			"common parent for forced reorg")
	}

	newBestBlock, err := b.fetchBlockFromHash(&newBest)
	if err != nil {
		return err
	}

	// Check to make sure our forced-in node validates correctly.
	view := NewUtxoViewpoint()
	view.SetBestHash(&b.bestNode.header.PrevBlock)
	view.SetStakeViewpoint(ViewpointPrevValidInitial)

	formerBestBlock, err := b.fetchBlockFromHash(&formerBest)
	if err != nil {
		return err
	}
	commonParentBlock, err := b.fetchBlockFromHash(&formerBestNode.parent.hash)
	if err != nil {
		return err
	}
	var stxos []spentTxOut
	err = b.db.View(func(dbTx database.Tx) error {
		stxos, err = dbFetchSpendJournalEntry(dbTx, formerBestBlock,
			commonParentBlock)
		return err
	})
	if err != nil {
		return err
	}

	// Quick sanity test.
	if len(stxos) != countSpentOutputs(formerBestBlock, commonParentBlock) {
		return AssertError(fmt.Sprintf("retrieved %v stxos when trying to "+
			"disconnect block %v (height %v), yet counted %v "+
			"many spent utxos when trying to force head reorg", len(stxos),
			formerBestBlock.Hash(), formerBestBlock.Height(),
			countSpentOutputs(formerBestBlock, commonParentBlock)))
	}

	err = b.disconnectTransactions(view, formerBestBlock, commonParentBlock,
		stxos)
	if err != nil {
		return err
	}

	err = checkBlockSanity(newBestBlock, b.timeSource, BFNone, b.chainParams)
	if err != nil {
		return err
	}

	err = b.checkConnectBlock(newBestNode, newBestBlock, view, nil)
	if err != nil {
		return err
	}

	attach, detach, err := b.getReorganizeNodes(newBestNode)
	if err != nil {
		return err
	}

	return b.reorganizeChain(attach, detach, BFNone)
}

// ForceHeadReorganization is the exported version of forceHeadReorganization.
func (b *BlockChain) ForceHeadReorganization(formerBest chainhash.Hash, newBest chainhash.Hash) error {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()
	return b.forceHeadReorganization(formerBest, newBest)
}

// connectBestChain handles connecting the passed block to the chain while
// respecting proper chain selection according to the chain with the most
// proof of work.  In the typical case, the new block simply extends the main
// chain.  However, it may also be extending (or creating) a side chain (fork)
// which may or may not end up becoming the main chain depending on which fork
// cumulatively has the most proof of work.  It returns whether or not the block
// ended up on the main chain (either due to extending the main chain or causing
// a reorganization to become the main chain).
//
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: Avoids several expensive transaction validation operations.
//    This is useful when using checkpoints.
//  - BFDryRun: Prevents the block from being connected and avoids modifying the
//    state of the memory chain index.  Also, any log messages related to
//    modifying the state are avoided.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) connectBestChain(node *blockNode, block *dcrutil.Block, flags BehaviorFlags) (bool, error) {
	fastAdd := flags&BFFastAdd == BFFastAdd
	dryRun := flags&BFDryRun == BFDryRun

	// We are extending the main (best) chain with a new block.  This is the
	// most common case.
	if node.header.PrevBlock == b.bestNode.hash {
		// Fetch the best block, now the parent, to be able to
		// connect the txTreeRegular if needed.
		// TODO optimize by not fetching if not needed?
		parent, err := b.fetchBlockFromHash(&node.header.PrevBlock)
		if err != nil {
			return false, err
		}
		// Perform several checks to verify the block can be connected
		// to the main chain without violating any rules and without
		// actually connecting the block.
		view := NewUtxoViewpoint()
		view.SetBestHash(&node.header.PrevBlock)
		view.SetStakeViewpoint(ViewpointPrevValidInitial)
		var stxos []spentTxOut
		if !fastAdd {
			err := b.checkConnectBlock(node, block, view, &stxos)
			if err != nil {
				return false, err
			}
		}

		// Don't connect the block if performing a dry run.
		if dryRun {
			return true, nil
		}

		// In the fast add case the code to check the block connection
		// was skipped, so the utxo view needs to load the referenced
		// utxos, spend them, and add the new utxos being created by
		// this block.
		if fastAdd {
			err := view.fetchInputUtxos(b.db, block, parent)
			if err != nil {
				return false, err
			}
			err = b.connectTransactions(view, block, parent, &stxos)
			if err != nil {
				return false, err
			}
		}

		// Connect the block to the main chain.
		err = b.connectBlock(node, block, view, stxos)
		if err != nil {
			return false, err
		}

		// Connect the parent node to this node.
		if node.parent != nil {
			node.parent.children = append(node.parent.children, node)
		}

		validateStr := "validating"
		txTreeRegularValid := dcrutil.IsFlagSet16(node.header.VoteBits,
			dcrutil.BlockValid)
		if !txTreeRegularValid {
			validateStr = "invalidating"
		}

		log.Debugf("Block %v (height %v) connected to the main chain, "+
			"%v the previous block", node.hash, node.height,
			validateStr)

		return true, nil
	}
	if fastAdd {
		log.Warnf("fastAdd set in the side chain case? %v\n",
			block.Hash())
	}

	// We're extending (or creating) a side chain which may or may not
	// become the main chain, but in either case we need the block stored
	// for future processing, so add the block to the side chain holding
	// cache.
	if !dryRun {
		log.Debugf("Adding block %v to side chain cache", node.hash)
	}
	b.blockCacheLock.Lock()
	b.blockCache[node.hash] = block
	b.blockCacheLock.Unlock()
	b.index[node.hash] = node

	// Connect the parent node to this node.
	node.inMainChain = false
	node.parent.children = append(node.parent.children, node)

	// Remove the block from the side chain cache and disconnect it from the
	// parent node when the function returns when running in dry run mode.
	if dryRun {
		defer func() {
			children := node.parent.children
			children = removeChildNode(children, node)
			node.parent.children = children

			delete(b.index, node.hash)
			b.blockCacheLock.Lock()
			delete(b.blockCache, node.hash)
			b.blockCacheLock.Unlock()
		}()
	}

	// We're extending (or creating) a side chain, but the cumulative
	// work for this new side chain is not enough to make it the new chain.
	if node.workSum.Cmp(b.bestNode.workSum) <= 0 {
		// Skip Logging info when the dry run flag is set.
		if dryRun {
			return false, nil
		}

		// Find the fork point.
		fork := node
		for fork.parent != nil {
			if fork.inMainChain {
				break
			}
			var err error
			fork, err = b.getPrevNodeFromNode(fork)
			if err != nil {
				return false, err
			}
		}

		// Log information about how the block is forking the chain.
		if fork.hash == node.parent.hash {
			log.Infof("FORK: Block %v (height %v) forks the chain at height "+
				"%d/block %v, but does not cause a reorganize",
				node.hash,
				node.height,
				fork.height,
				fork.hash)
		} else {
			log.Infof("EXTEND FORK: Block %v (height %v) extends a side chain "+
				"which forks the chain at height "+
				"%d/block %v",
				node.hash,
				node.height,
				fork.height,
				fork.hash)
		}

		return false, nil
	}

	// We're extending (or creating) a side chain and the cumulative work
	// for this new side chain is more than the old best chain, so this side
	// chain needs to become the main chain.  In order to accomplish that,
	// find the common ancestor of both sides of the fork, disconnect the
	// blocks that form the (now) old fork from the main chain, and attach
	// the blocks that form the new chain to the main chain starting at the
	// common ancenstor (the point where the chain forked).
	detachNodes, attachNodes, err := b.getReorganizeNodes(node)
	if err != nil {
		return false, err
	}

	// Reorganize the chain.
	if !dryRun {
		log.Infof("REORGANIZE: Block %v is causing a reorganize.",
			node.hash)
	}
	err = b.reorganizeChain(detachNodes, attachNodes, flags)
	if err != nil {
		return false, err
	}

	return true, nil
}

// isCurrent returns whether or not the chain believes it is current.  Several
// factors are used to guess, but the key factors that allow the chain to
// believe it is current are:
//  - Latest block height is after the latest checkpoint (if enabled)
//  - Latest block has a timestamp newer than 24 hours ago
//
// This function MUST be called with the chain state lock held (for reads).
func (b *BlockChain) isCurrent() bool {
	// Not current if the latest main (best) chain height is before the
	// latest known good checkpoint (when checkpoints are enabled).
	checkpoint := b.latestCheckpoint()
	if checkpoint != nil && b.bestNode.height < checkpoint.Height {
		return false
	}

	// Not current if the latest best block has a timestamp before 24 hours
	// ago.
	//
	// The chain appears to be current if none of the checks reported
	// otherwise.
	minus24Hours := b.timeSource.AdjustedTime().Add(-24 * time.Hour)
	return !b.bestNode.header.Timestamp.Before(minus24Hours)
}

// IsCurrent returns whether or not the chain believes it is current.  Several
// factors are used to guess, but the key factors that allow the chain to
// believe it is current are:
//  - Latest block height is after the latest checkpoint (if enabled)
//  - Latest block has a timestamp newer than 24 hours ago
//
// This function is safe for concurrent access.
func (b *BlockChain) IsCurrent() bool {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	return b.isCurrent()
}

// BestSnapshot returns information about the current best chain block and
// related state as of the current point in time.  The returned instance must be
// treated as immutable since it is shared by all callers.
//
// This function is safe for concurrent access.
func (b *BlockChain) BestSnapshot() *BestState {
	b.stateLock.RLock()
	snapshot := b.stateSnapshot
	b.stateLock.RUnlock()
	return snapshot
}

// MaximumBlockSize returns the maximum permitted block size for the block
// AFTER the given node.
//
// This function MUST be called with the chain state lock held (for reads).
func (b *BlockChain) maxBlockSize(prevNode *blockNode) (int64, error) {
	// Hard fork voting on block size is only enabled on testnet v1 and
	// simnet.
	if b.chainParams.Net != wire.SimNet {
		return int64(b.chainParams.MaximumBlockSizes[0]), nil
	}

	// Return the larger block size if the version 4 stake vote for the max
	// block size increase agenda is active.
	//
	// NOTE: The choice field of the return threshold state is not examined
	// here because there is only one possible choice that can be active
	// for the agenda, which is yes, so there is no need to check it.
	maxSize := int64(b.chainParams.MaximumBlockSizes[0])
	state, err := b.deploymentState(prevNode, 4, chaincfg.VoteIDMaxBlockSize)
	if err != nil {
		return maxSize, err
	}
	if state.State == ThresholdActive {
		return int64(b.chainParams.MaximumBlockSizes[1]), nil
	}

	// The max block size is not changed in any other cases.
	return maxSize, nil
}

// MaximumBlockSize returns the maximum permitted block size for the block AFTER
// the end of the current best chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) MaxBlockSize() (int64, error) {
	b.chainLock.Lock()
	maxSize, err := b.maxBlockSize(b.bestNode)
	b.chainLock.Unlock()
	return maxSize, err
}

// IndexManager provides a generic interface that the is called when blocks are
// connected and disconnected to and from the tip of the main chain for the
// purpose of supporting optional indexes.
type IndexManager interface {
	// Init is invoked during chain initialize in order to allow the index
	// manager to initialize itself and any indexes it is managing.
	Init(*BlockChain) error

	// ConnectBlock is invoked when a new block has been connected to the
	// main chain.
	ConnectBlock(database.Tx, *dcrutil.Block, *dcrutil.Block, *UtxoViewpoint) error

	// DisconnectBlock is invoked when a block has been disconnected from
	// the main chain.
	DisconnectBlock(database.Tx, *dcrutil.Block, *dcrutil.Block, *UtxoViewpoint) error
}

// Config is a descriptor which specifies the blockchain instance configuration.
type Config struct {
	// DB defines the database which houses the blocks and will be used to
	// store all metadata created by this package such as the utxo set.
	//
	// This field is required.
	DB database.DB

	// ChainParams identifies which chain parameters the chain is associated
	// with.
	//
	// This field is required.
	ChainParams *chaincfg.Params

	// TimeSource defines the median time source to use for things such as
	// block processing and determining whether or not the chain is current.
	//
	// The caller is expected to keep a reference to the time source as well
	// and add time samples from other peers on the network so the local
	// time is adjusted to be in agreement with other peers.
	TimeSource MedianTimeSource

	// Notifications defines a callback to which notifications will be sent
	// when various events take place.  See the documentation for
	// Notification and NotificationType for details on the types and
	// contents of notifications.
	//
	// This field can be nil if the caller is not interested in receiving
	// notifications.
	Notifications NotificationCallback

	// SigCache defines a signature cache to use when when validating
	// signatures.  This is typically most useful when individual
	// transactions are already being validated prior to their inclusion in
	// a block such as what is usually done via a transaction memory pool.
	//
	// This field can be nil if the caller is not interested in using a
	// signature cache.
	SigCache *txscript.SigCache

	// IndexManager defines an index manager to use when initializing the
	// chain and connecting and disconnecting blocks.
	//
	// This field can be nil if the caller does not wish to make use of an
	// index manager.
	IndexManager IndexManager
}

// New returns a BlockChain instance using the provided configuration details.
func New(config *Config) (*BlockChain, error) {
	// Enforce required config fields.
	if config.DB == nil {
		return nil, AssertError("blockchain.New database is nil")
	}
	if config.ChainParams == nil {
		return nil, AssertError("blockchain.New chain parameters nil")
	}

	// Generate a checkpoint by height map from the provided checkpoints.
	params := config.ChainParams
	var checkpointsByHeight map[int64]*chaincfg.Checkpoint
	if len(params.Checkpoints) > 0 {
		checkpointsByHeight = make(map[int64]*chaincfg.Checkpoint)
		for i := range params.Checkpoints {
			checkpoint := &params.Checkpoints[i]
			checkpointsByHeight[checkpoint.Height] = checkpoint
		}
	}

	b := BlockChain{
		checkpointsByHeight:           checkpointsByHeight,
		db:                            config.DB,
		chainParams:                   params,
		timeSource:                    config.TimeSource,
		notifications:                 config.Notifications,
		sigCache:                      config.SigCache,
		indexManager:                  config.IndexManager,
		bestNode:                      nil,
		index:                         make(map[chainhash.Hash]*blockNode),
		depNodes:                      make(map[chainhash.Hash][]*blockNode),
		orphans:                       make(map[chainhash.Hash]*orphanBlock),
		prevOrphans:                   make(map[chainhash.Hash][]*orphanBlock),
		blockCache:                    make(map[chainhash.Hash]*dcrutil.Block),
		mainchainBlockCache:           make(map[chainhash.Hash]*dcrutil.Block),
		mainchainBlockCacheSize:       mainchainBlockCacheSize,
		deploymentCaches:              newThresholdCaches(params),
		isVoterMajorityVersionCache:   make(map[[stakeMajorityCacheKeySize]byte]bool),
		isStakeMajorityVersionCache:   make(map[[stakeMajorityCacheKeySize]byte]bool),
		calcPriorStakeVersionCache:    make(map[[chainhash.HashSize]byte]uint32),
		calcVoterVersionIntervalCache: make(map[[chainhash.HashSize]byte]uint32),
		calcStakeVersionCache:         make(map[[chainhash.HashSize]byte]uint32),
	}

	// Initialize the chain state from the passed database.  When the db
	// does not yet contain any chain state, both it and the chain state
	// will be initialized to contain only the genesis block.
	if err := b.initChainState(); err != nil {
		return nil, err
	}

	// Initialize and catch up all of the currently active optional indexes
	// as needed.
	if config.IndexManager != nil {
		if err := config.IndexManager.Init(&b); err != nil {
			return nil, err
		}
	}

	// Apply any upgrades as needed.
	if err := b.upgrade(); err != nil {
		return nil, err
	}

	b.subsidyCache = NewSubsidyCache(b.bestNode.height, b.chainParams)
	b.pruner = newChainPruner(&b)

	log.Infof("Blockchain database version %v loaded",
		b.dbInfo.version)

	log.Infof("Chain state: height %d, hash %v, total transactions %d, "+
		"work %v, stake version %v", b.bestNode.height, b.bestNode.hash,
		b.stateSnapshot.TotalTxns, b.bestNode.workSum,
		0)

	return &b, nil
}
