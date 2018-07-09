// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
)

// CheckpointConfirmations is the number of blocks before the end of the current
// best block chain that a good checkpoint candidate must be.
const CheckpointConfirmations = 4096

// DisableCheckpoints provides a mechanism to disable validation against
// checkpoints which you DO NOT want to do in production.  It is provided only
// for debug purposes.
//
// This function is safe for concurrent access.
func (b *BlockChain) DisableCheckpoints(disable bool) {
	b.chainLock.Lock()
	b.noCheckpoints = disable
	b.chainLock.Unlock()
}

// Checkpoints returns a slice of checkpoints (regardless of whether they are
// already known).  When checkpoints are disabled or there are no checkpoints
// for the active network, it will return nil.
//
// This function is safe for concurrent access.
func (b *BlockChain) Checkpoints() []chaincfg.Checkpoint {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	if b.noCheckpoints || len(b.chainParams.Checkpoints) == 0 {
		return nil
	}

	return b.chainParams.Checkpoints
}

// latestCheckpoint returns the most recent checkpoint (regardless of whether it
// is already known).  When checkpoints are disabled or there are no checkpoints
// for the active network, it will return nil.
//
// This function MUST be called with the chain state lock held (for reads).
func (b *BlockChain) latestCheckpoint() *chaincfg.Checkpoint {
	if b.noCheckpoints || len(b.chainParams.Checkpoints) == 0 {
		return nil
	}

	checkpoints := b.chainParams.Checkpoints
	return &checkpoints[len(checkpoints)-1]
}

// LatestCheckpoint returns the most recent checkpoint (regardless of whether it
// is already known).  When checkpoints are disabled or there are no checkpoints
// for the active network, it will return nil.
//
// This function is safe for concurrent access.
func (b *BlockChain) LatestCheckpoint() *chaincfg.Checkpoint {
	b.chainLock.RLock()
	checkpoint := b.latestCheckpoint()
	b.chainLock.RUnlock()
	return checkpoint
}

// verifyCheckpoint returns whether the passed block height and hash combination
// match the hard-coded checkpoint data.  It also returns true if there is no
// checkpoint data for the passed block height.
//
// This function MUST be called with the chain lock held (for reads).
func (b *BlockChain) verifyCheckpoint(height int64, hash *chainhash.Hash) bool {
	if b.noCheckpoints || len(b.chainParams.Checkpoints) == 0 {
		return true
	}

	// Nothing to check if there is no checkpoint data for the block height.
	checkpoint, exists := b.checkpointsByHeight[height]
	if !exists {
		return true
	}

	if !checkpoint.Hash.IsEqual(hash) {
		return false
	}

	log.Infof("Verified checkpoint at height %d/block %s", checkpoint.Height,
		checkpoint.Hash)
	return true
}

// findPreviousCheckpoint finds the most recent checkpoint that is already
// available in the downloaded portion of the block chain and returns the
// associated block node.  It returns nil if a checkpoint can't be found (this
// should really only happen for blocks before the first checkpoint).
//
// This function MUST be called with the chain lock held (for reads).
func (b *BlockChain) findPreviousCheckpoint() (*blockNode, error) {
	if b.noCheckpoints || len(b.chainParams.Checkpoints) == 0 {
		return nil, nil
	}

	// Perform the initial search to find and cache the latest known
	// checkpoint if the best chain is not known yet or we haven't already
	// previously searched.
	checkpoints := b.chainParams.Checkpoints
	numCheckpoints := len(checkpoints)
	if b.checkpointNode == nil && b.nextCheckpoint == nil {
		// Loop backwards through the available checkpoints to find one
		// that is already available.
		for i := numCheckpoints - 1; i >= 0; i-- {
			node := b.index.LookupNode(checkpoints[i].Hash)
			if node == nil || !b.bestChain.Contains(node) {
				continue
			}

			// Checkpoint found.  Cache it for future lookups and
			// set the next expected checkpoint accordingly.
			b.checkpointNode = node
			if i < numCheckpoints-1 {
				b.nextCheckpoint = &checkpoints[i+1]
			}
			return b.checkpointNode, nil
		}

		// No known latest checkpoint.  This will only happen on blocks
		// before the first known checkpoint.  So, set the next expected
		// checkpoint to the first checkpoint and return the fact there
		// is no latest known checkpoint block.
		b.nextCheckpoint = &checkpoints[0]
		return nil, nil
	}

	// At this point we've already searched for the latest known checkpoint,
	// so when there is no next checkpoint, the current checkpoint lockin
	// will always be the latest known checkpoint.
	if b.nextCheckpoint == nil {
		return b.checkpointNode, nil
	}

	// When there is a next checkpoint and the height of the current best
	// chain does not exceed it, the current checkpoint lockin is still
	// the latest known checkpoint.
	if b.bestChain.Tip().height < b.nextCheckpoint.Height {
		return b.checkpointNode, nil
	}

	// We've reached or exceeded the next checkpoint height.  Note that
	// once a checkpoint lockin has been reached, forks are prevented from
	// any blocks before the checkpoint, so we don't have to worry about the
	// checkpoint going away out from under us due to a chain reorganize.

	// Cache the latest known checkpoint for future lookups.  Note that if
	// this lookup fails something is very wrong since the chain has already
	// passed the checkpoint which was verified as accurate before inserting
	// it.
	checkpointNode := b.index.LookupNode(b.nextCheckpoint.Hash)
	if checkpointNode == nil {
		return nil, AssertError(fmt.Sprintf("findPreviousCheckpoint "+
			"failed lookup of known good block node %s",
			b.nextCheckpoint.Hash))
	}
	b.checkpointNode = checkpointNode

	// Set the next expected checkpoint.
	checkpointIndex := -1
	for i := numCheckpoints - 1; i >= 0; i-- {
		if checkpoints[i].Hash.IsEqual(b.nextCheckpoint.Hash) {
			checkpointIndex = i
			break
		}
	}
	b.nextCheckpoint = nil
	if checkpointIndex != -1 && checkpointIndex < numCheckpoints-1 {
		b.nextCheckpoint = &checkpoints[checkpointIndex+1]
	}

	return b.checkpointNode, nil
}

// isNonstandardTransaction determines whether a transaction contains any
// scripts which are not one of the standard types.
func isNonstandardTransaction(tx *dcrutil.Tx) bool {
	// Check all of the output public key scripts for non-standard scripts.
	for _, txOut := range tx.MsgTx().TxOut {
		scriptClass := txscript.GetScriptClass(txOut.Version, txOut.PkScript)
		if scriptClass == txscript.NonStandardTy {
			return true
		}
	}
	return false
}

// IsCheckpointCandidate returns whether or not the passed block is a good
// checkpoint candidate.
//
// The factors used to determine a good checkpoint are:
//  - The block must be in the main chain
//  - The block must be at least 'CheckpointConfirmations' blocks prior to the
//    current end of the main chain
//  - The timestamps for the blocks before and after the checkpoint must have
//    timestamps which are also before and after the checkpoint, respectively
//    (due to the median time allowance this is not always the case)
//  - The block must not contain any strange transaction such as those with
//    nonstandard scripts
//
// The intent is that candidates are reviewed by a developer to make the final
// decision and then manually added to the list of checkpoints for a network.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsCheckpointCandidate(block *dcrutil.Block) (bool, error) {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	// Checkpoints must be enabled.
	if b.noCheckpoints {
		return false, fmt.Errorf("checkpoints are disabled")
	}

	// A checkpoint must be in the main chain.
	node := b.index.LookupNode(block.Hash())
	if node == nil || !b.bestChain.Contains(node) {
		return false, nil
	}

	// Ensure the height of the passed block and the entry for the block in
	// the main chain match.  This should always be the case unless the
	// caller provided an invalid block.
	if node.height != block.Height() {
		return false, fmt.Errorf("passed block height of %d does not "+
			"match the main chain height of %d", block.Height(),
			node.height)
	}

	// A checkpoint must be at least CheckpointConfirmations blocks before
	// the end of the main chain.
	if node.height > (b.bestChain.Tip().height - CheckpointConfirmations) {
		return false, nil
	}

	// A checkpoint must be have at least one block after it.
	//
	// This should always succeed since the check above already made sure it
	// is CheckpointConfirmations back, but be safe in case the constant
	// changes.
	nextNode := b.bestChain.Next(node)
	if nextNode == nil {
		return false, nil
	}

	// A checkpoint must be have at least one block before it.
	if node.parent == nil {
		return false, nil
	}

	// A checkpoint must have timestamps for the block and the blocks on
	// either side of it in order (due to the median time allowance this is
	// not always the case).
	prevTime := time.Unix(node.parent.timestamp, 0)
	curTime := block.MsgBlock().Header.Timestamp
	nextTime := time.Unix(nextNode.timestamp, 0)
	if prevTime.After(curTime) || nextTime.Before(curTime) {
		return false, nil
	}

	// A checkpoint must have transactions that only contain standard
	// scripts.
	for _, tx := range block.Transactions() {
		if isNonstandardTransaction(tx) {
			return false, nil
		}
	}

	// All of the checks passed, so the block is a candidate.
	return true, nil
}
