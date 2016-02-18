// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"container/list"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

const (
	// maxOrphanBlocks is the maximum number of orphan blocks that can be
	// queued.
	maxOrphanBlocks = 500

	// minMemoryNodesLocal is the minimum number of consecutive nodes needed
	// in memory in order to perform all necessary validation.  It is used
	// to determine when it's safe to prune nodes from memory without
	// causing constant dynamic reloading.
	minMemoryNodesLocal = 4096

	// searchDepth is the distance in blocks to search down the blockchain
	// to find some parent.
	searchDepth = 2048
)

// ErrIndexAlreadyInitialized describes an error that indicates the block index
// is already initialized.
var ErrIndexAlreadyInitialized = errors.New("the block index can only be " +
	"initialized before it has been modified")

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
	hash *chainhash.Hash

	// parentHash is the double sha 256 of the parent block.  This is kept
	// here over simply relying on parent.hash directly since block nodes
	// are sparse and the parent node might not be in memory when its hash
	// is needed.
	parentHash *chainhash.Hash

	// height is the position in the block chain.
	height int64

	// workSum is the total amount of work in the chain up to and including
	// this node.
	workSum *big.Int

	// inMainChain denotes whether the block node is currently on the
	// the main chain or not.  This is used to help find the common
	// ancestor when switching chains.
	inMainChain bool

	// outputAmtsTotal is amount of fees in the tx tree regular of the parent plus
	// the value of the coinbase, which may or may not be given to the child node
	// depending on the voters. Doesn't get set until you actually attempt to
	// connect the block and calculate the fees/reward for it.
	// DECRED TODO: Is this actually used anywhere? If not prune it.
	outputAmtsTotal int64

	// Decred: Keep the full block header.
	header wire.BlockHeader

	// VoteBits for the stake voters.
	voteBits []uint16

	// Some fields from block headers to aid in best chain selection.
	version   int32
	bits      uint32
	timestamp time.Time
}

// newBlockNode returns a new block node for the given block header.  It is
// completely disconnected from the chain and the workSum value is just the work
// for the passed block.  The work sum is updated accordingly when the node is
// inserted into a chain.
func newBlockNode(blockHeader *wire.BlockHeader, blockSha *chainhash.Hash,
	height int64, voteBits []uint16) *blockNode {
	// Make a copy of the hash so the node doesn't keep a reference to part
	// of the full block/block header preventing it from being garbage
	// collected.
	prevHash := blockHeader.PrevBlock
	node := blockNode{
		hash:       blockSha,
		parentHash: &prevHash,
		workSum:    CalcWork(blockHeader.Bits),
		height:     height,
		version:    blockHeader.Version,
		bits:       blockHeader.Bits,
		timestamp:  blockHeader.Timestamp,
		header:     *blockHeader,
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

// addChildrenWork adds the passed work amount to all children all the way
// down the chain.  It is used primarily to allow a new node to be dynamically
// inserted from the database into the memory chain prior to nodes we already
// have and update their work values accordingly.
func addChildrenWork(node *blockNode, work *big.Int) {
	for _, childNode := range node.children {
		childNode.workSum.Add(childNode.workSum, work)
		addChildrenWork(childNode, work)
	}
}

// removeChildNode deletes node from the provided slice of child block
// nodes.  It ensures the final pointer reference is set to nil to prevent
// potential memory leaks.  The original slice is returned unmodified if node
// is invalid or not in the slice.
func removeChildNode(children []*blockNode, node *blockNode) []*blockNode {
	if node == nil {
		return children
	}

	// An indexing for loop is intentionally used over a range here as range
	// does not reevaluate the slice on each iteration nor does it adjust
	// the index for the modified slice.
	for i := 0; i < len(children); i++ {
		if children[i].hash.IsEqual(node.hash) {
			copy(children[i:], children[i+1:])
			children[len(children)-1] = nil
			return children[:len(children)-1]
		}
	}
	return children
}

// BlockChain provides functions for working with the decred block chain.
// It includes functionality such as rejecting duplicate blocks, ensuring blocks
// follow all rules, orphan handling, checkpoint handling, and best chain
// selection with reorganization.
type BlockChain struct {
	db                    database.Db
	tmdb                  *stake.TicketDB
	chainParams           *chaincfg.Params
	checkpointsByHeight   map[int64]*chaincfg.Checkpoint
	notifications         NotificationCallback
	minMemoryNodes        int64
	blocksPerRetarget     int64
	stakeValidationHeight int64
	root                  *blockNode
	bestChain             *blockNode
	index                 map[chainhash.Hash]*blockNode
	depNodes              map[chainhash.Hash][]*blockNode
	orphans               map[chainhash.Hash]*orphanBlock
	prevOrphans           map[chainhash.Hash][]*orphanBlock
	oldestOrphan          *orphanBlock
	orphanLock            sync.RWMutex
	blockCache            map[chainhash.Hash]*dcrutil.Block
	blockCacheLock        sync.RWMutex
	noVerify              bool
	noCheckpoints         bool
	nextCheckpoint        *chaincfg.Checkpoint
	checkpointBlock       *dcrutil.Block
}

// StakeValidationHeight returns the height at which proof of stake validation
// begins for proof of work block headers.
func (b *BlockChain) StakeValidationHeight() int64 {
	return b.stakeValidationHeight
}

// DisableVerify provides a mechanism to disable transaction script validation
// which you DO NOT want to do in production as it could allow double spends
// and othe undesirable things.  It is provided only for debug purposes since
// script validation is extremely intensive and when debugging it is sometimes
// nice to quickly get the chain.
func (b *BlockChain) DisableVerify(disable bool) {
	b.noVerify = disable
}

// MissedTickets returns all currently missed tickets from the stake database.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) MissedTickets() (stake.SStxMemMap, error) {
	missed, err := b.tmdb.DumpMissedTickets()
	if err != nil {
		return nil, err
	}

	return missed, nil
}

// TicketsWithAddress returns a slice of ticket hashes that are currently live
// corresponding to the given address.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) TicketsWithAddress(address dcrutil.Address) ([]chainhash.Hash,
	error) {
	return b.tmdb.GetLiveTicketsForAddress(address)
}

// CheckLiveTicket returns whether or not a ticket exists in the live ticket
// map of the stake database.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) CheckLiveTicket(hash *chainhash.Hash) (bool, error) {
	return b.tmdb.CheckLiveTicket(*hash)
}

// HaveBlock returns whether or not the chain instance has the block represented
// by the passed hash.  This includes checking the various places a block can
// be like part of the main chain, on a side chain, or in the orphan pool.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) HaveBlock(hash *chainhash.Hash) (bool, error) {
	exists, err := b.blockExists(hash)
	if err != nil {
		return false, err
	}
	return b.IsKnownOrphan(hash) || exists, nil
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
	defer b.orphanLock.RUnlock()

	if _, exists := b.orphans[*hash]; exists {
		return true
	}

	return false
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
	orphanHash := orphan.block.Sha()
	delete(b.orphans, *orphanHash)

	// Remove the reference from the previous orphan index too.  An indexing
	// for loop is intentionally used over a range here as range does not
	// reevaluate the slice on each iteration nor does it adjust the index
	// for the modified slice.
	prevHash := &orphan.block.MsgBlock().Header.PrevBlock
	orphans := b.prevOrphans[*prevHash]
	for i := 0; i < len(orphans); i++ {
		hash := orphans[i].block.Sha()
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
	b.orphans[*block.Sha()] = oBlock

	// Add to previous hash lookup index for faster dependency lookups.
	prevHash := &block.MsgBlock().Header.PrevBlock
	b.prevOrphans[*prevHash] = append(b.prevOrphans[*prevHash], oBlock)

	return
}

// getGeneration gets a generation of blocks who all have the same parent by
// taking a hash as input, locating its parent node, and then returning all
// children for that parent node including the hash passed. This can then be
// used by the mempool downstream to locate all potential block template
// parents.
func (b *BlockChain) getGeneration(h chainhash.Hash) ([]chainhash.Hash, error) {
	node, err := b.findNode(&h)

	// This typically happens because the main chain has recently
	// reorganized and the block the miner is looking at is on
	// a fork. Usually it corrects itself after failure.
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
		allChildren[i] = *p.children[i].hash
	}

	return allChildren, nil
}

// GetGeneration is the exported version of getGeneration.
func (b *BlockChain) GetGeneration(hash chainhash.Hash) ([]chainhash.Hash, error) {
	return b.getGeneration(hash)
}

// GenerateInitialIndex is an optional function which generates the required
// number of initial block nodes in an optimized fashion.  This is optional
// because the memory block index is sparse and previous nodes are dynamically
// loaded as needed.  However, during initial startup (when there are no nodes
// in memory yet), dynamically loading all of the required nodes on the fly in
// the usual way is much slower than preloading them.
//
// This function can only be called once and it must be called before any nodes
// are added to the block index.  ErrIndexAlreadyInitialized is returned if
// the former is not the case.  In practice, this means the function should be
// called directly after New.
func (b *BlockChain) GenerateInitialIndex() error {
	// Return an error if the has already been modified.
	if b.root != nil {
		return ErrIndexAlreadyInitialized
	}

	// Grab the latest block height for the main chain from the database.
	_, endHeight, err := b.db.NewestSha()
	if err != nil {
		return err
	}

	// Calculate the starting height based on the minimum number of nodes
	// needed in memory.
	startHeight := endHeight - b.minMemoryNodes
	if startHeight < 0 {
		startHeight = 0
	}

	// Loop forwards through each block loading the node into the index for
	// the block.
	//
	// Due to a bug in the SQLite dcrdb driver, the FetchBlockBySha call is
	// limited to a maximum number of hashes per invocation.  Since SQLite
	// is going to be nuked eventually, the bug isn't being fixed in the
	// driver.  In the mean time, work around the issue by calling
	// FetchBlockBySha multiple times with the appropriate indices as needed.
	for start := startHeight; start <= endHeight; {
		hashList, err := b.db.FetchHeightRange(start, endHeight+1)
		if err != nil {
			return err
		}

		// The database did not return any further hashes.  Break out of
		// the loop now.
		if len(hashList) == 0 {
			break
		}

		// Loop forwards through each block loading the node into the
		// index for the block.
		for _, hash := range hashList {
			// Make a copy of the hash to make sure there are no
			// references into the list so it can be freed.
			hashCopy := hash
			node, err := b.loadBlockNode(&hashCopy)
			if err != nil {
				return err
			}

			// This node is now the end of the best chain.
			b.bestChain = node
		}

		// Start at the next block after the latest one on the next loop
		// iteration.
		start += int64(len(hashList))
	}

	return nil
}

// loadBlockNode loads the block identified by hash from the block database,
// creates a block node from it, and updates the memory block chain accordingly.
// It is used mainly to dynamically load previous blocks from database as they
// are needed to avoid needing to put the entire block chain in memory.
func (b *BlockChain) loadBlockNode(hash *chainhash.Hash) (*blockNode, error) {
	block, err := b.db.FetchBlockBySha(hash)
	if err != nil {
		return nil, err
	}

	// Create the new block node for the block and set the work.
	var voteBitsStake []uint16
	for _, stx := range block.STransactions() {
		if is, _ := stake.IsSSGen(stx); is {
			vb := stake.GetSSGenVoteBits(stx)
			voteBitsStake = append(voteBitsStake, vb)
		}
	}
	node := newBlockNode(&block.MsgBlock().Header, hash,
		int64(block.MsgBlock().Header.Height), voteBitsStake)
	node.inMainChain = true
	prevHash := &block.MsgBlock().Header.PrevBlock

	// Add the node to the chain.
	// There are several possibilities here:
	//  1) This node is a child of an existing block node
	//  2) This node is the parent of one or more nodes
	//  3) Neither 1 or 2 is true, and this is not the first node being
	//     added to the tree which implies it's an orphan block and
	//     therefore is an error to insert into the chain
	//  4) Neither 1 or 2 is true, but this is the first node being added
	//     to the tree, so it's the root.
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
		// Connect this block node to all of its children and update
		// all of the children (and their children) with the new work
		// sums.
		for _, childNode := range childNodes {
			childNode.parent = node
			node.children = append(node.children, childNode)
			addChildrenWork(childNode, node.workSum)
			b.root = node
		}
	} else {
		// Case 3 -- The node doesn't have a parent and is not the parent
		// of another node.  This is only acceptable for the first node
		// inserted into the chain.  Otherwise it means an arbitrary
		// orphan block is trying to be loaded which is not allowed.
		if b.root != nil {
			str := "loadBlockNode: attempt to insert orphan block %v"
			return nil, fmt.Errorf(str, hash)
		}

		// Case 4 -- This is the root since it's the first and only node.
		b.root = node
	}

	// Add the new node to the indices for faster lookups.
	b.index[*hash] = node
	b.depNodes[*prevHash] = append(b.depNodes[*prevHash], node)

	return node, nil
}

// findNode finds the node scaling backwards from best chain or return an
// error.
func (b *BlockChain) findNode(nodeHash *chainhash.Hash) (*blockNode, error) {
	var node *blockNode

	// Most common case; we're checking a block that wants to be connected
	// on top of the current main chain.
	distance := 0
	if nodeHash.IsEqual(b.bestChain.hash) {
		node = b.bestChain
	} else {
		// Look backwards in our blockchain and try to find it in the
		// parents of blocks.
		foundPrev := b.bestChain
		notFound := false
		for !foundPrev.hash.IsEqual(b.chainParams.GenesisHash) {
			if distance >= searchDepth {
				notFound = true
				break
			}

			if foundPrev.hash.IsEqual(b.chainParams.GenesisHash) {
				notFound = true
				break
			}

			if foundPrev.hash.IsEqual(nodeHash) {
				break
			}

			foundPrev = foundPrev.parent
			if foundPrev == nil {
				parent, err := b.loadBlockNode(&foundPrev.header.PrevBlock)
				if err != nil {
					return nil, err
				}

				foundPrev = parent
			}

			distance++
		}

		if notFound {
			return nil, fmt.Errorf("couldn't find node %v in best chain",
				nodeHash)
		}

		node = foundPrev
	}

	return node, nil
}

// getPrevNodeFromBlock returns a block node for the block previous to the
// passed block (the passed block's parent).  When it is already in the memory
// block chain, it simply returns it.  Otherwise, it loads the previous block
// from the block database, creates a new block node from it, and returns it.
// The returned node will be nil if the genesis block is passed.
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
	prevBlockNode, err := b.findNode(prevHash)
	if err != nil {
		return nil, err
	}
	return prevBlockNode, nil
}

// getPrevNodeFromNode returns a block node for the block previous to the
// passed block node (the passed block node's parent).  When the node is already
// connected to a parent, it simply returns it.  Otherwise, it loads the
// associated block from the database to obtain the previous hash and uses that
// to dynamically create a new block node and return it.  The memory block
// chain is updated accordingly.  The returned node will be nil if the genesis
// block is passed.
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
	prevBlockNode, err := b.findNode(node.parentHash)
	if err != nil {
		return nil, err
	}

	return prevBlockNode, nil
}

// GetNodeAtHeightFromTopNode goes backwards through a node until it a reaches
// the node with a desired block height; it returns this block. The benefit is
// this works for both the main chain and the side chain.
func (b *BlockChain) getNodeAtHeightFromTopNode(node *blockNode,
	toTraverse int64) (*blockNode, error) {
	oldNode := node
	var err error

	for i := 0; i < int(toTraverse); i++ {
		// Get the previous block node.
		oldNode, err = b.getPrevNodeFromNode(oldNode)
		if err != nil {
			return nil, err
		}

		if oldNode == nil {
			return nil, fmt.Errorf("unable to obtain previous node; " +
				"ancestor is genesis block")
		}
	}

	return oldNode, nil
}

// getBlockFromHash searches the internal chain block stores and the database in
// an attempt to find the block. If it finds the block, it returns it.
func (b *BlockChain) getBlockFromHash(hash *chainhash.Hash) (*dcrutil.Block,
	error) {
	// Check block cache
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
	blockMainchain, errFetchMainchain := b.db.FetchBlockBySha(hash)
	existsMainchain := (errFetchMainchain == nil) || (blockMainchain != nil)
	if existsMainchain {
		return blockMainchain, nil
	}

	// Implicit !existsMainchain && !existsSidechain && !existsOrphans
	return nil, fmt.Errorf("unable to find block %v in "+
		"side chain cache, orphan cache, and main chain db", hash)
}

// GetBlockFromHash is the generalized and exported version of getBlockFromHash.
func (b *BlockChain) GetBlockFromHash(hash *chainhash.Hash) (*dcrutil.Block,
	error) {
	return b.getBlockFromHash(hash)
}

// GetTopBlock returns the current block at HEAD on the blockchain. Needed
// for mining in the daemon.
func (b *BlockChain) GetTopBlock() (dcrutil.Block, error) {
	block, err := b.getBlockFromHash(b.bestChain.hash)
	return *block, err
}

// removeBlockNode removes the passed block node from the memory chain by
// unlinking all of its children and removing it from the the node and
// dependency indices.
func (b *BlockChain) removeBlockNode(node *blockNode) error {
	if node.parent != nil {
		return fmt.Errorf("removeBlockNode must be called with a "+
			" node at the front of the chain - node %v", node.hash)
	}

	// Remove the node from the node index.
	delete(b.index, *node.hash)

	// Unlink all of the node's children.
	for _, child := range node.children {
		child.parent = nil
	}
	node.children = nil

	// Remove the reference from the dependency index.
	prevHash := node.parentHash
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
func (b *BlockChain) pruneBlockNodes() error {
	// Nothing to do if there is not a best chain selected yet.
	if b.bestChain == nil {
		return nil
	}

	// Walk the chain backwards to find what should be the new root node.
	// Intentionally use node.parent instead of getPrevNodeFromNode since
	// the latter loads the node and the goal is to find nodes still in
	// memory that can be pruned.
	newRootNode := b.bestChain
	for i := int64(0); i < b.minMemoryNodes-1 && newRootNode != nil; i++ {
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
		err := b.removeBlockNode(node)
		if err != nil {
			return err
		}
	}

	// Set the new root node.
	b.root = newRootNode

	return nil
}

// GetCurrentBlockHeader returns the block header of the block at HEAD.
// This function is NOT safe for concurrent access.
func (b *BlockChain) GetCurrentBlockHeader() *wire.BlockHeader {
	return &b.bestChain.header
}

// isMajorityVersion determines if a previous number of blocks in the chain
// starting with startNode are at least the minimum passed version.
func (b *BlockChain) isMajorityVersion(minVer int32, startNode *blockNode,
	numRequired int32) bool {

	numFound := int32(0)
	iterNode := startNode
	for i := int32(0); i < b.chainParams.CurrentBlockVersion &&
		numFound < numRequired && iterNode != nil; i++ {
		// This node has a version that is at least the minimum version.
		if iterNode.version >= minVer {
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
		timestamps[i] = iterNode.timestamp
		numNodes++

		// Get the previous block node.  This function is used over
		// simply accessing iterNode.parent directly as it will
		// dynamically create previous block nodes as needed.  This
		// helps allow only the pieces of the chain that are needed
		// to remain in memory.
		var err error
		iterNode, err = b.getPrevNodeFromNode(iterNode)
		if err != nil {
			log.Errorf("getPrevNodeFromNode: %v", err)
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

// CalcPastMedianTime calculates the median time of the previous few blocks
// prior to, and including, the end of the current best chain.  It is primarily
// used to ensure new blocks have sane timestamps.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) CalcPastMedianTime() (time.Time, error) {
	return b.calcPastMedianTime(b.bestChain)
}

// getReorganizeNodes finds the fork point between the main chain and the passed
// node and returns a list of block nodes that would need to be detached from
// the main chain and a list of block nodes that would need to be attached to
// the fork point (which will be the end of the main chain after detaching the
// returned list of block nodes) in order to reorganize the chain such that the
// passed node is the new end of the main chain.  The lists will be empty if the
// passed node is not on a side chain.
func (b *BlockChain) getReorganizeNodes(node *blockNode) (*list.List, *list.List,
	error) {
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
	for ancestor.parent != nil {
		if ancestor.inMainChain {
			break
		}
		attachNodes.PushFront(ancestor)

		var err error
		ancestor, err = b.getPrevNodeFromNode(ancestor)
		if err != nil {
			return nil, nil, err
		}
	}

	// Start from the end of the main chain and work backwards until the
	// common ancestor adding each block to the list of nodes to detach from
	// the main chain.
	for n := b.bestChain; n != nil && n.parent != nil; {
		if n.hash.IsEqual(ancestor.hash) {
			break
		}
		detachNodes.PushBack(n)

		var err error
		n, err = b.getPrevNodeFromNode(n)
		if err != nil {
			return nil, nil, err
		}
	}

	return detachNodes, attachNodes, nil
}

// connectBlock handles connecting the passed node/block to the end of the main
// (best) chain.
func (b *BlockChain) connectBlock(node *blockNode, block *dcrutil.Block) error {
	// Make sure it's extending the end of the best chain.
	prevHash := &block.MsgBlock().Header.PrevBlock
	if b.bestChain != nil && !prevHash.IsEqual(b.bestChain.hash) {
		return fmt.Errorf("connectBlock must be called with a block " +
			"that extends the main chain")
	}

	var err error

	// Insert block into ticket database if we're the point where tickets begin to
	// mature. Note that if the block is inserted into tmdb and then insertion
	// into DB fails, the two database will be on different HEADs. This needs
	// to be handled correctly in the near future.
	if node.height >= b.chainParams.StakeEnabledHeight {
		spentAndMissedTickets, newTickets, _, err :=
			b.tmdb.InsertBlock(block)
		if err != nil {
			return err
		}

		nextStakeDiff, err := b.calcNextRequiredStakeDifficulty(node)
		if err != nil {
			return err
		}

		// Notify of spent and missed tickets
		b.sendNotification(NTSpentAndMissedTickets,
			&TicketNotificationsData{*node.hash,
				node.height,
				nextStakeDiff,
				spentAndMissedTickets})
		// Notify of new tickets
		b.sendNotification(NTNewTickets,
			&TicketNotificationsData{*node.hash,
				node.height,
				nextStakeDiff,
				newTickets})
	}

	// Insert the block into the database which houses the main chain.
	_, err = b.db.InsertBlock(block)
	if err != nil {
		// Attempt to restore TicketDb if this fails.
		_, _, _, errRemove := b.tmdb.RemoveBlockToHeight(node.height - 1)
		if errRemove != nil {
			return errRemove
		}

		return err
	}

	// Add the new node to the memory main chain indices for faster
	// lookups.
	node.inMainChain = true
	b.index[*node.hash] = node
	b.depNodes[*prevHash] = append(b.depNodes[*prevHash], node)

	// This node is now the end of the best chain.
	b.bestChain = node

	// Get the parent block.
	parent, err := b.getBlockFromHash(node.parent.hash)
	if err != nil {
		return err
	}

	// Assemble the current block and the parent into a slice.
	blockAndParent := []*dcrutil.Block{block, parent}

	// Notify the caller that the block was connected to the main chain.
	// The caller would typically want to react with actions such as
	// updating wallets.
	b.sendNotification(NTBlockConnected, blockAndParent)

	return nil
}

// disconnectBlock handles disconnecting the passed node/block from the end of
// the main (best) chain.
func (b *BlockChain) disconnectBlock(node *blockNode, block *dcrutil.Block) error {
	// Make sure the node being disconnected is the end of the best chain.
	if b.bestChain == nil || !node.hash.IsEqual(b.bestChain.hash) {
		return fmt.Errorf("disconnectBlock must be called with the " +
			"block at the end of the main chain")
	}

	// Insert block into ticket database if we're the point where tickets begin to
	// mature.
	maturityHeight := int64(b.chainParams.TicketMaturity) +
		int64(b.chainParams.CoinbaseMaturity)

	// Remove from ticket database.
	if node.height-1 >= maturityHeight {
		_, _, _, err := b.tmdb.RemoveBlockToHeight(node.height - 1)
		if err != nil {
			return err
		}
	}

	// Remove the block from the database which houses the main chain.
	// if we're above the point in which the stake db is enabled.
	prevNode, err := b.getPrevNodeFromNode(node)
	if err != nil {
		// Attempt to restore TicketDb if this fails.
		_, _, _, errReinsert := b.tmdb.InsertBlock(block)
		if errReinsert != nil {
			return errReinsert
		}

		return err
	}
	err = b.db.DropAfterBlockBySha(prevNode.hash)
	if err != nil {
		// Attempt to restore TicketDb if this fails.
		_, _, _, errReinsert := b.tmdb.InsertBlock(block)
		if errReinsert != nil {
			return errReinsert
		}

		return err
	}

	// Put block in the side chain cache.
	node.inMainChain = false
	b.blockCacheLock.Lock()
	b.blockCache[*node.hash] = block
	b.blockCacheLock.Unlock()

	// This node's parent is now the end of the best chain.
	b.bestChain = node.parent

	// Get the parent block.
	parent, err := b.getBlockFromHash(node.parent.hash)
	if err != nil {
		return err
	}

	// Assemble the current block and the parent into a slice.
	blockAndParent := []*dcrutil.Block{block, parent}

	// Notify the caller that the block was disconnected from the main
	// chain.  The caller would typically want to react with actions such as
	// updating wallets.
	b.sendNotification(NTBlockDisconnected, blockAndParent)

	return nil
}

// reorganizeChain reorganizes the block chain by disconnecting the nodes in the
// detachNodes list and connecting the nodes in the attach list.  It expects
// that the lists are already in the correct order and are in sync with the
// end of the current best chain.  Specifically, nodes that are being
// disconnected must be in reverse order (think of popping them off
// the end of the chain) and nodes the are being attached must be in forwards
// order (think pushing them onto the end of the chain).
//
// The flags modify the behavior of this function as follows:
//  - BFDryRun: Only the checks which ensure the reorganize can be completed
//    successfully are performed.  The chain is not reorganized.
// Decred: TODO Implement and debug reorg behaviour with ticket database.
func (b *BlockChain) reorganizeChain(detachNodes, attachNodes *list.List,
	flags BehaviorFlags) error {
	oldHash := b.bestChain.hash
	oldHeight := b.bestChain.height

	// Ensure all of the needed side chain blocks are in the cache.
	for e := attachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		b.blockCacheLock.RLock()
		_, exists := b.blockCache[*n.hash]
		b.blockCacheLock.RUnlock()
		if !exists {
			return fmt.Errorf("block %v is missing from the side "+
				"chain block cache", n.hash)
		}
	}

	// Perform several checks to verify each block that needs to be attached
	// to the main chain can be connected without violating any rules and
	// without actually connecting the block.
	//
	// NOTE: bitcoind does these checks directly when it connects a block.
	// The downside to that approach is that if any of these checks fail
	// after disconnecting some blocks or attaching others, all of the
	// operations have to be rolled back to get the chain back into the
	// state it was before the rule violation (or other failure).  There are
	// at least a couple of ways accomplish that rollback, but both involve
	// tweaking the chain.  This approach catches these issues before ever
	// modifying the chain.
	var topBlock *blockNode
	for e := attachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		b.blockCacheLock.RLock()
		block := b.blockCache[*n.hash]
		b.blockCacheLock.RUnlock()
		log.Debugf("Evaluating block %v (height %v) for correctness",
			block.Sha(), block.Height())
		err := b.checkConnectBlock(n, block)
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
		*oldHash,
		oldHeight,
		*newHash,
		newHeight,
	}
	b.sendNotification(NTReorganization, reorgData)

	// Disconnect blocks from the main chain.
	for e := detachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		block, err := b.db.FetchBlockBySha(n.hash)
		if err != nil {
			return err
		}
		err = b.disconnectBlock(n, block)
		if err != nil {
			return err
		}
	}

	// Connect the new best chain blocks.
	for e := attachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		b.blockCacheLock.RLock()
		block := b.blockCache[*n.hash]
		b.blockCacheLock.RUnlock()
		err := b.connectBlock(n, block)
		if err != nil {
			return err
		}
		b.blockCacheLock.Lock()
		delete(b.blockCache, *n.hash)
		b.blockCacheLock.Unlock()
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
		oldHash,
		oldHeight)
	log.Infof("REORGANIZE: New best chain head is %v, height %v",
		lastAttachNode.hash,
		lastAttachNode.height)

	return nil
}

// forceReorganizationToBlock forces a reorganization of the block chain to the
// block hash requested, so long as it matches up with the current organization
// of the best chain.
func (b *BlockChain) forceHeadReorganization(formerBest chainhash.Hash,
	newBest chainhash.Hash,
	timeSource MedianTimeSource) error {
	if formerBest.IsEqual(&newBest) {
		return fmt.Errorf("can't reorganize to the same block")
	}

	formerBestNode := b.bestChain

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
		ruleError(ErrForceReorgMissingChild, "missing child of common parent "+
			"for forced reorg")
	}

	newBestBlock, err := b.getBlockFromHash(&newBest)

	// Check to make sure our forced-in node validates correctly.
	err = checkBlockSanity(newBestBlock,
		timeSource,
		BFNone,
		b.chainParams)

	err = b.checkConnectBlock(newBestNode, newBestBlock)
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
func (b *BlockChain) ForceHeadReorganization(formerBest chainhash.Hash,
	newBest chainhash.Hash, timeSource MedianTimeSource) error {
	return b.forceHeadReorganization(formerBest, newBest, timeSource)
}

// connectBestChain handles connecting the passed block to the chain while
// respecting proper chain selection according to the chain with the most
// proof of work.  In the typical case, the new block simply extends the main
// chain.  However, it may also be extending (or creating) a side chain (fork)
// which may or may not end up becoming the main chain depending on which fork
// cumulatively has the most proof of work.
// Returns a boolean that indicates where the block passed was on the main
// chain or a sidechain (true = main chain).
//
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: Avoids the call to checkConnectBlock which does several
//    expensive transaction validation operations.
//  - BFDryRun: Prevents the block from being connected and avoids modifying the
//    state of the memory chain index.  Also, any log messages related to
//    modifying the state are avoided.
func (b *BlockChain) connectBestChain(node *blockNode, block *dcrutil.Block,
	flags BehaviorFlags) (bool, error) {
	fastAdd := flags&BFFastAdd == BFFastAdd
	dryRun := flags&BFDryRun == BFDryRun

	// We haven't selected a best chain yet or we are extending the main
	// (best) chain with a new block.  This is the most common case.
	if b.bestChain == nil || node.parent.hash.IsEqual(b.bestChain.hash) {
		// Perform several checks to verify the block can be connected
		// to the main chain (including whatever reorganization might
		// be necessary to get this node to the main chain) without
		// violating any rules and without actually connecting the
		// block.
		if !fastAdd {
			err := b.checkConnectBlock(node, block)
			if err != nil {
				return false, err
			}
		}

		// Don't connect the block if performing a dry run.
		if dryRun {
			return true, nil
		}

		// Connect the block to the main chain.
		err := b.connectBlock(node, block)
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
			"%v the previous block",
			node.hash,
			node.height,
			validateStr)

		return true, nil
	}
	if fastAdd {
		log.Warnf("fastAdd set in the side chain case? %v\n",
			block.Sha())
	}

	// We're extending (or creating) a side chain which may or may not
	// become the main chain, but in either case we need the block stored
	// for future processing, so add the block to the side chain holding
	// cache.
	if !dryRun {
		log.Debugf("Adding block %v to side chain cache", node.hash)
	}
	b.blockCacheLock.Lock()
	b.blockCache[*node.hash] = block
	b.blockCacheLock.Unlock()
	b.index[*node.hash] = node

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

			delete(b.index, *node.hash)
			b.blockCacheLock.Lock()
			delete(b.blockCache, *node.hash)
			b.blockCacheLock.Unlock()
		}()
	}

	// We're extending (or creating) a side chain, but the cumulative
	// work for this new side chain is not enough to make it the new chain.
	if node.workSum.Cmp(b.bestChain.workSum) <= 0 {
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
		if fork.hash.IsEqual(node.parent.hash) {
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
		return false, nil
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

// IsCurrent returns whether or not the chain believes it is current.  Several
// factors are used to guess, but the key factors that allow the chain to
// believe it is current are:
//  - Latest block height is after the latest checkpoint (if enabled)
//  - Latest block has a timestamp newer than 24 hours ago
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) IsCurrent(timeSource MedianTimeSource) bool {
	// Not current if there isn't a main (best) chain yet.
	if b.bestChain == nil {
		return false
	}

	// Not current if the latest main (best) chain height is before the
	// latest known good checkpoint (when checkpoints are enabled).
	checkpoint := b.LatestCheckpoint()
	if checkpoint != nil && b.bestChain.height < checkpoint.Height {
		return false
	}

	// Not current if the latest best block has a timestamp before 24 hours
	// ago and is on mainnet.
	minus24Hours := timeSource.AdjustedTime().Add(-24 * time.Hour)
	if b.bestChain.timestamp.Before(minus24Hours) &&
		b.chainParams.Name == "mainnet" {
		return false
	}

	// The chain appears to be current if the above checks did not report
	// otherwise.
	return true
}

// maxInt64 returns the maximum of two 64-bit integers.
func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// New returns a BlockChain instance for the passed decred network using the
// provided backing database.  It accepts a callback on which notifications
// will be sent when various events take place.  See the documentation for
// Notification and NotificationType for details on the types and contents of
// notifications.  The provided callback can be nil if the caller is not
// interested in receiving notifications.
func New(db database.Db, tmdb *stake.TicketDB, params *chaincfg.Params,
	c NotificationCallback) *BlockChain {
	// Generate a checkpoint by height map from the provided checkpoints.
	var checkpointsByHeight map[int64]*chaincfg.Checkpoint
	if len(params.Checkpoints) > 0 {
		checkpointsByHeight = make(map[int64]*chaincfg.Checkpoint)
		for i := range params.Checkpoints {
			checkpoint := &params.Checkpoints[i]
			checkpointsByHeight[checkpoint.Height] = checkpoint
		}
	}

	// BlocksPerRetarget is the number of blocks between each difficulty
	// retarget.  It is calculated based on the retargeting window sizes
	// in blocks for both PoW and PoS.
	blocksPerRetargetPoW := int64(params.WorkDiffWindowSize *
		params.WorkDiffWindows)
	blocksPerRetargetPoS := int64(params.StakeDiffWindowSize *
		params.StakeDiffWindows)
	blocksPerRetarget := maxInt64(blocksPerRetargetPoW, blocksPerRetargetPoS)

	b := BlockChain{
		db:                    db,
		tmdb:                  tmdb,
		chainParams:           params,
		checkpointsByHeight:   checkpointsByHeight,
		notifications:         c,
		blocksPerRetarget:     blocksPerRetarget,
		minMemoryNodes:        minMemoryNodesLocal,
		stakeValidationHeight: params.StakeValidationHeight,
		root:        nil,
		bestChain:   nil,
		index:       make(map[chainhash.Hash]*blockNode),
		depNodes:    make(map[chainhash.Hash][]*blockNode),
		orphans:     make(map[chainhash.Hash]*orphanBlock),
		prevOrphans: make(map[chainhash.Hash][]*orphanBlock),
		blockCache:  make(map[chainhash.Hash]*dcrutil.Block),
	}

	return &b
}
