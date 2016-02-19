// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
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
	database "github.com/decred/dcrd/database2"
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
	// causing constant dynamic reloading.
	minMemoryNodes = 4096

	// searchDepth is the distance in blocks to search down the blockchain
	// to find some parent.
	searchDepth = 2048
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
	hash *chainhash.Hash

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
	node := blockNode{
		hash:    blockSha,
		workSum: CalcWork(blockHeader.Bits),
		height:  height,
		header:  *blockHeader,
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
		if children[i].hash.IsEqual(node.hash) {
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
	TotalSubsidy int64           // The total subsidy for the chain.
}

// newBestState returns a new best stats instance for the given parameters.
func newBestState(node *blockNode, blockSize, numTxns, totalTxns uint64, totalSubsidy int64) *BestState {
	return &BestState{
		Hash:         node.hash,
		Height:       node.height,
		Bits:         node.header.Bits,
		BlockSize:    blockSize,
		NumTxns:      numTxns,
		TotalTxns:    totalTxns,
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
	tmdb                *stake.TicketDB
	chainParams         *chaincfg.Params
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
	root     *blockNode
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
	b.chainLock.Lock()
	ts := b.BestSnapshot().TotalSubsidy
	b.chainLock.Unlock()

	return ts
}

// FetchSubsidyCache returns the current subsidy cache from the blockchain.
//
// This function is safe for concurrent access.
func (b *BlockChain) FetchSubsidyCache() *SubsidyCache {
	return b.subsidyCache
}

// LiveTickets returns all currently live tickets from the stake database.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) LiveTickets() ([]*chainhash.Hash, error) {
	live, err := b.tmdb.DumpAllLiveTicketHashes()
	if err != nil {
		return nil, err
	}

	return live, nil
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
	tickets, err := b.tmdb.DumpAllLiveTicketHashes()
	if err != nil {
		return nil, err
	}

	var ticketsWithAddr []chainhash.Hash
	err = b.db.View(func(dbTx database.Tx) error {
		var err error
		for _, hash := range tickets {
			utxo, err := dbFetchUtxoEntry(dbTx, hash)
			if err != nil {
				return err
			}

			_, addrs, _, err :=
				txscript.ExtractPkScriptAddrs(txscript.DefaultScriptVersion,
					utxo.PkScriptByIndex(0), b.chainParams)
			if addrs[0].EncodeAddress() == address.EncodeAddress() {
				ticketsWithAddr = append(ticketsWithAddr, *hash)
			}
		}
		return err
	})
	if err != nil {
		return nil, err
	}

	return ticketsWithAddr, nil
}

// CheckLiveTicket returns whether or not a ticket exists in the live ticket
// map of the stake database.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) CheckLiveTicket(hash *chainhash.Hash) (bool, error) {
	return b.tmdb.CheckLiveTicket(*hash)
}

// CheckLiveTickets returns whether or not a slice of tickets exist in the live
// ticket map of the stake database.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) CheckLiveTickets(hashes []*chainhash.Hash) ([]bool, error) {
	var err error
	existsSlice := make([]bool, len(hashes))
	for i, hash := range hashes {
		existsSlice[i], err = b.tmdb.CheckLiveTicket(*hash)
		if err != nil {
			return nil, err
		}
	}

	return existsSlice, nil
}

// TicketPoolValueExact returns the current value of all the locked funds in the
// ticket pool.
//
// This function is safe for concurrent access. All live tickets are at least
// 256 blocks deep on mainnet, so the UTXO set should generally always have
// the asked for transactions.
func (b *BlockChain) TicketPoolValue() (dcrutil.Amount, error) {
	tickets, err := b.tmdb.DumpAllLiveTicketHashes()
	if err != nil {
		return 0, err
	}

	var amt int64
	err = b.db.View(func(dbTx database.Tx) error {
		var err error
		for _, hash := range tickets {
			utxo, err := dbFetchUtxoEntry(dbTx, hash)
			if err != nil {
				return err
			}

			amt += utxo.sparseOutputs[0].amount
		}
		return err
	})
	if err != nil {
		return 0, err
	}
	return dcrutil.Amount(amt), nil
}

// HaveBlock returns whether or not the chain instance has the block represented
// by the passed hash.  This includes checking the various places a block can
// be like part of the main chain, on a side chain, or in the orphan pool.
//
// This function is safe for concurrent access.
func (b *BlockChain) HaveBlock(hash *chainhash.Hash) (bool, error) {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

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

// loadBlockNode loads the block identified by hash from the block database,
// creates a block node from it, and updates the memory block chain accordingly.
// It is used mainly to dynamically load previous blocks from the database as
// they are needed to avoid needing to put the entire block chain in memory.
//
// This function MUST be called with the chain state lock held (for writes).
// The database transaction may be read-only.
func (b *BlockChain) loadBlockNode(dbTx database.Tx,
	hash *chainhash.Hash) (*blockNode, error) {
	blockHeader, err := dbFetchHeaderByHash(dbTx, hash)
	if err != nil {
		return nil, err
	}

	var voteBitsStake []uint16
	node := newBlockNode(blockHeader, hash,
		int64(blockHeader.Height), voteBitsStake)
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
			b.root = node
		}
	} else {
		// Case 3 -- The node doesn't have a parent and is not the
		// parent of another node.  This means an arbitrary orphan block
		// is trying to be loaded which is not allowed.
		str := "loadBlockNode: attempt to insert orphan block %v"
		return nil, AssertError(fmt.Sprintf(str, hash))
	}

	// Add the new node to the indices for faster lookups.
	b.index[*hash] = node
	b.depNodes[*prevHash] = append(b.depNodes[*prevHash], node)

	return node, nil
}

// findNode finds the node scaling backwards from best chain or return an
// error.
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) findNode(nodeHash *chainhash.Hash) (*blockNode, error) {
	var node *blockNode
	err := b.db.View(func(dbTx database.Tx) error {
		// Most common case; we're checking a block that wants to be connected
		// on top of the current main chain.
		distance := 0
		if nodeHash.IsEqual(b.bestNode.hash) {
			node = b.bestNode
		} else {
			// Look backwards in our blockchain and try to find it in the
			// parents of blocks.
			foundPrev := b.bestNode
			notFound := true
			for !foundPrev.hash.IsEqual(b.chainParams.GenesisHash) {
				if distance >= searchDepth {
					break
				}

				if foundPrev.hash.IsEqual(nodeHash) {
					notFound = false
					break
				}

				foundPrev = foundPrev.parent
				if foundPrev == nil {
					parent, err := b.loadBlockNode(dbTx,
						&foundPrev.header.PrevBlock)
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
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) getBlockFromHash(hash *chainhash.Hash) (*dcrutil.Block,
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
	if ok {
		b.mainchainBlockCacheLock.RUnlock()
		return block, nil
	}
	b.mainchainBlockCacheLock.RUnlock()

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

// GetBlockFromHash is the generalized and exported version of getBlockFromHash.
func (b *BlockChain) GetBlockFromHash(hash *chainhash.Hash) (*dcrutil.Block,
	error) {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()
	return b.getBlockFromHash(hash)
}

// GetTopBlock returns the current block at HEAD on the blockchain. Needed
// for mining in the daemon.
func (b *BlockChain) GetTopBlock() (dcrutil.Block, error) {
	block, err := b.getBlockFromHash(b.bestNode.hash)
	return *block, err
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
	delete(b.index, *node.hash)

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
	return &b.bestNode.header
}

// isMajorityVersion determines if a previous number of blocks in the chain
// starting with startNode are at least the minimum passed version.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isMajorityVersion(minVer int32, startNode *blockNode,
	numRequired int32) bool {
	numFound := int32(0)
	iterNode := startNode
	for i := int32(0); i < b.chainParams.CurrentBlockVersion &&
		numFound < int32(numRequired) && iterNode != nil; i++ {
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

// CalcPastMedianTime calculates the median time of the previous few blocks
// prior to, and including, the end of the current best chain.  It is primarily
// used to ensure new blocks have sane timestamps.
//
// This function is safe for concurrent access.
func (b *BlockChain) CalcPastMedianTime() (time.Time, error) {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()

	return b.calcPastMedianTime(b.bestNode)
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
func (b *BlockChain) getReorganizeNodes(node *blockNode) (*list.List, *list.List) {
	// Nothing to detach or attach if there is no node.
	attachNodes := list.New()
	detachNodes := list.New()
	if node == nil {
		return detachNodes, attachNodes
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
	for n := b.bestNode; n != nil && n.parent != nil; n = n.parent {
		if n.hash.IsEqual(ancestor.hash) {
			break
		}
		detachNodes.PushBack(n)
	}

	return detachNodes, attachNodes
}

// pushMainChainBlockCache pushes a block onto the main chain block cache,
// and removes any old blocks from the cache that might be present.
func (b *BlockChain) pushMainChainBlockCache(block *dcrutil.Block) {
	curHeight := block.Height()
	curHash := block.Sha()
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
	hasBlock, err := dbTx.HasBlock(block.Sha())
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
func (b *BlockChain) connectBlock(node *blockNode, block *dcrutil.Block,
	view *UtxoViewpoint, stxos []spentTxOut) error {
	// Make sure it's extending the end of the best chain.
	prevHash := &block.MsgBlock().Header.PrevBlock
	if !prevHash.IsEqual(b.bestNode.hash) {
		return AssertError("connectBlock must be called with a block " +
			"that extends the main chain")
	}

	// Sanity check the correct number of stxos are provided.
	parent, err := b.getBlockFromHash(node.parent.hash)
	if err != nil {
		return err
	}
	if len(stxos) != countSpentOutputs(block, parent) {
		return AssertError("connectBlock called with inconsistent " +
			"spent transaction out information")
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
		curTotalSubsidy+subsidy)

	// Insert block into ticket database if we're the point where tickets begin to
	// mature. Note that if the block is inserted into tmdb and then insertion
	// into DB fails, the two database will be on different HEADs. This needs
	// to be handled correctly in the near future.
	if node.height >= b.chainParams.StakeEnabledHeight {
		spentAndMissedTickets, newTickets, _, err :=
			b.tmdb.InsertBlock(block, parent)
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

	// Atomically insert info into the database.
	err = b.db.Update(func(dbTx database.Tx) error {
		// Update best block state.
		err := dbPutBestState(dbTx, state, node.workSum)
		if err != nil {
			return err
		}

		// Add the block hash and height to the block index which tracks
		// the main chain.
		err = dbPutBlockIndex(dbTx, block.Sha(), node.height)
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
		err = dbPutSpendJournalEntry(dbTx, block.Sha(), stxos)
		if err != nil {
			// Attempt to restore TicketDb if this fails.
			_, _, _, errRemove := b.tmdb.RemoveBlockToHeight(node.height - 1)
			if errRemove != nil {
				return errRemove
			}

			return err
		}

		// Insert the block into the database if it's not already there.
		err = dbMaybeStoreBlock(dbTx, block)
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
		log.Errorf("Failed to insert block %v: %s", node.hash, err.Error())

		// Attempt to restore TicketDb if this fails.
		if node.height >= b.chainParams.StakeEnabledHeight {
			_, _, _, errRemove := b.tmdb.RemoveBlockToHeight(node.height - 1)
			if errRemove != nil {
				return errRemove
			}
		}

		return err
	}

	// Prune fully spent entries and mark all entries in the view unmodified
	// now that the modifications have been committed to the database.
	view.commit()

	// Add the new node to the memory main chain indices for faster
	// lookups.
	node.inMainChain = true
	b.index[*node.hash] = node
	b.depNodes[*prevHash] = append(b.depNodes[*prevHash], node)

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

	// Assemble the current block and the parent into a slice.
	blockAndParent := []*dcrutil.Block{block, parent}

	// Notify the caller that the block was connected to the main chain.
	// The caller would typically want to react with actions such as
	// updating wallets.
	b.chainLock.Unlock()
	b.sendNotification(NTBlockConnected, blockAndParent)
	b.chainLock.Lock()

	b.pushMainChainBlockCache(block)

	return nil
}

// dropMainChainBlockCache drops a block from the main chain block cache.
func (b *BlockChain) dropMainChainBlockCache(block *dcrutil.Block) {
	curHash := block.Sha()
	b.mainchainBlockCacheLock.Lock()
	delete(b.mainchainBlockCache, *curHash)
	b.mainchainBlockCacheLock.Unlock()
}

// disconnectBlock handles disconnecting the passed node/block from the end of
// the main (best) chain.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) disconnectBlock(node *blockNode, block *dcrutil.Block,
	view *UtxoViewpoint) error {
	// Make sure the node being disconnected is the end of the best chain.
	if !node.hash.IsEqual(b.bestNode.hash) {
		return AssertError("disconnectBlock must be called with the " +
			"block at the end of the main chain")
	}

	// Get the previous block node.  This function is used over simply
	// accessing node.parent directly as it will dynamically create previous
	// block nodes as needed.  This helps allow only the pieces of the chain
	// that are needed to remain in memory.

	// Remove from ticket database.
	maturityHeight := int64(b.chainParams.TicketMaturity) +
		int64(b.chainParams.CoinbaseMaturity)
	if node.height-1 >= maturityHeight {
		_, _, _, err := b.tmdb.RemoveBlockToHeight(node.height - 1)
		if err != nil {
			return err
		}
	}

	// if we're above the point in which the stake db is enabled.
	prevNode, err := b.getPrevNodeFromNode(node)
	if err != nil {
		return err
	}

	// Load the previous block since some details for it are needed below.
	parent, err := b.getBlockFromHash(prevNode.hash)
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
		newTotalSubsidy)

	err = b.db.Update(func(dbTx database.Tx) error {
		// Update best block state.
		err := dbPutBestState(dbTx, state, node.workSum)
		if err != nil {
			return err
		}

		// Remove the block hash and height from the block index which
		// tracks the main chain.
		err = dbRemoveBlockIndex(dbTx, block.Sha(), node.height)
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
		err = dbRemoveSpendJournalEntry(dbTx, block.Sha())
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
	b.blockCache[*node.hash] = block
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
	for _, stx := range block.STransactions() {
		txType := stake.DetermineTxType(stx)
		if txType == stake.TxTypeSSGen || txType == stake.TxTypeSSRtx {
			numSpent++
			continue
		}
		numSpent += len(stx.MsgTx().TxIn)
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
		b.blockCacheLock.RLock()
		n := e.Value.(*blockNode)
		if _, exists := b.blockCache[*n.hash]; !exists {
			return AssertError(fmt.Sprintf("block %v is missing "+
				"from the side chain block cache", n.hash))
		}
		b.blockCacheLock.RUnlock()
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
	view.SetBestHash(formerBestHash)
	view.SetStakeViewpoint(ViewpointPrevValidInitial)
	i := 0
	for e := detachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		var block *dcrutil.Block
		var parent *dcrutil.Block
		var err error
		block, err = b.getBlockFromHash(n.hash)
		if err != nil {
			return err
		}
		parent, err = b.getBlockFromHash(&n.header.PrevBlock)
		if err != nil {
			return err
		}

		// Load all of the spent txos for the block from the spend
		// journal.
		var stxos []spentTxOut
		err = b.db.View(func(dbTx database.Tx) error {
			stxos, err = dbFetchSpendJournalEntry(dbTx, block, parent, view)
			return err
		})
		if err != nil {
			return err
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
		block := b.blockCache[*n.hash]
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
		*formerBestHash,
		formerBestHeight,
		*newHash,
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
	view.SetBestHash(formerBestHash)
	view.SetStakeViewpoint(ViewpointPrevValidInitial)

	// Disconnect blocks from the main chain.
	for i, e := 0, detachNodes.Front(); e != nil; i, e = i+1, e.Next() {
		n := e.Value.(*blockNode)
		block := detachBlocks[i]
		parent, err := b.getBlockFromHash(&n.header.PrevBlock)
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
		block := b.blockCache[*n.hash]
		b.blockCacheLock.RUnlock()

		parent, err := b.getBlockFromHash(&n.header.PrevBlock)
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
		delete(b.blockCache, *n.hash)
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
func (b *BlockChain) forceHeadReorganization(formerBest chainhash.Hash,
	newBest chainhash.Hash, timeSource MedianTimeSource) error {
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

	newBestBlock, err := b.getBlockFromHash(&newBest)

	// Check to make sure our forced-in node validates correctly.
	view := NewUtxoViewpoint()
	view.SetBestHash(&b.bestNode.header.PrevBlock)
	view.SetStakeViewpoint(ViewpointPrevValidInitial)
	err = checkBlockSanity(newBestBlock,
		timeSource,
		BFNone,
		b.chainParams)

	err = b.checkConnectBlock(newBestNode, newBestBlock, view, nil)
	if err != nil {
		return err
	}

	attach, detach := b.getReorganizeNodes(newBestNode)
	if err != nil {
		return err
	}

	return b.reorganizeChain(attach, detach, BFNone)
}

// ForceHeadReorganization is the exported version of forceHeadReorganization.
func (b *BlockChain) ForceHeadReorganization(formerBest chainhash.Hash,
	newBest chainhash.Hash, timeSource MedianTimeSource) error {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()
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
//  - BFFastAdd: Avoids several expensive transaction validation operations.
//    This is useful when using checkpoints.
//  - BFDryRun: Prevents the block from being connected and avoids modifying the
//    state of the memory chain index.  Also, any log messages related to
//    modifying the state are avoided.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) connectBestChain(node *blockNode, block *dcrutil.Block,
	flags BehaviorFlags) (bool, error) {
	fastAdd := flags&BFFastAdd == BFFastAdd
	dryRun := flags&BFDryRun == BFDryRun

	// We are extending the main (best) chain with a new block.  This is the
	// most common case.
	if node.header.PrevBlock.IsEqual(b.bestNode.hash) {
		// Fetch the best block, now the parent, to be able to
		// connect the txTreeRegular if needed.
		// TODO optimize by not fetching if not needed?
		parent, err := b.getBlockFromHash(&node.header.PrevBlock)
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
	detachNodes, attachNodes := b.getReorganizeNodes(node)

	// Reorganize the chain.
	if !dryRun {
		log.Infof("REORGANIZE: Block %v is causing a reorganize.",
			node.hash)
	}
	err := b.reorganizeChain(detachNodes, attachNodes, flags)
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
// This function is safe for concurrent access.
func (b *BlockChain) IsCurrent(timeSource MedianTimeSource) bool {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	// Not current if the latest main (best) chain height is before the
	// latest known good checkpoint (when checkpoints are enabled).
	checkpoint := b.latestCheckpoint()
	if checkpoint != nil && b.bestNode.height < checkpoint.Height {
		return false
	}

	// Not current if the latest best block has a timestamp before 24 hours
	// ago.
	minus24Hours := timeSource.AdjustedTime().Add(-24 * time.Hour)
	if b.bestNode.header.Timestamp.Before(minus24Hours) {
		return false
	}

	// The chain appears to be current if the above checks did not report
	// otherwise.
	return true
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

	// tmdb
	TMDB *stake.TicketDB

	// ChainParams identifies which chain parameters the chain is associated
	// with.
	//
	// This field is required.
	ChainParams *chaincfg.Params

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
		checkpointsByHeight:     checkpointsByHeight,
		db:                      config.DB,
		tmdb:                    config.TMDB,
		chainParams:             params,
		notifications:           config.Notifications,
		sigCache:                config.SigCache,
		indexManager:            config.IndexManager,
		root:                    nil,
		bestNode:                nil,
		index:                   make(map[chainhash.Hash]*blockNode),
		depNodes:                make(map[chainhash.Hash][]*blockNode),
		orphans:                 make(map[chainhash.Hash]*orphanBlock),
		prevOrphans:             make(map[chainhash.Hash][]*orphanBlock),
		blockCache:              make(map[chainhash.Hash]*dcrutil.Block),
		mainchainBlockCache:     make(map[chainhash.Hash]*dcrutil.Block),
		mainchainBlockCacheSize: int(params.CoinbaseMaturity) + 1,
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

	b.subsidyCache = NewSubsidyCache(b.bestNode.height, b.chainParams)

	log.Infof("Blockchain database version %v loaded successfully",
		b.dbInfo.version)

	log.Infof("Chain state (height %d, hash %v, total transactions %d, work %v)",
		b.bestNode.height, b.bestNode.hash, b.stateSnapshot.TotalTxns,
		b.bestNode.workSum)

	return &b, nil
}
