// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcchain

import (
	"fmt"

	"github.com/conformal/btcnet"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// CheckpointConfirmations is the number of blocks before the end of the current
// best block chain that a good checkpoint candidate must be.
const CheckpointConfirmations = 2016

// newShaHashFromStr converts the passed big-endian hex string into a
// btcwire.ShaHash.  It only differs from the one available in btcwire in that
// it ignores the error since it will only (and must only) be called with
// hard-coded, and therefore known good, hashes.
func newShaHashFromStr(hexStr string) *btcwire.ShaHash {
	sha, _ := btcwire.NewShaHashFromStr(hexStr)
	return sha
}

// DisableCheckpoints provides a mechanism to disable validation against
// checkpoints which you DO NOT want to do in production.  It is provided only
// for debug purposes.
func (b *BlockChain) DisableCheckpoints(disable bool) {
	b.noCheckpoints = disable
}

// Checkpoints returns a slice of checkpoints (regardless of whether they are
// already known).  When checkpoints are disabled or there are no checkpoints
// for the active network, it will return nil.
func (b *BlockChain) Checkpoints() []btcnet.Checkpoint {
	if b.noCheckpoints || len(b.netParams.Checkpoints) == 0 {
		return nil
	}

	return b.netParams.Checkpoints
}

// LatestCheckpoint returns the most recent checkpoint (regardless of whether it
// is already known).  When checkpoints are disabled or there are no checkpoints
// for the active network, it will return nil.
func (b *BlockChain) LatestCheckpoint() *btcnet.Checkpoint {
	if b.noCheckpoints || len(b.netParams.Checkpoints) == 0 {
		return nil
	}

	checkpoints := b.netParams.Checkpoints
	return &checkpoints[len(checkpoints)-1]
}

// verifyCheckpoint returns whether the passed block height and hash combination
// match the hard-coded checkpoint data.  It also returns true if there is no
// checkpoint data for the passed block height.
func (b *BlockChain) verifyCheckpoint(height int64, hash *btcwire.ShaHash) bool {
	if b.noCheckpoints || len(b.netParams.Checkpoints) == 0 {
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
// associated block.  It returns nil if a checkpoint can't be found (this should
// really only happen for blocks before the first checkpoint).
func (b *BlockChain) findPreviousCheckpoint() (*btcutil.Block, error) {
	if b.noCheckpoints || len(b.netParams.Checkpoints) == 0 {
		return nil, nil
	}

	// No checkpoints.
	checkpoints := b.netParams.Checkpoints
	numCheckpoints := len(checkpoints)
	if numCheckpoints == 0 {
		return nil, nil
	}

	// Perform the initial search to find and cache the latest known
	// checkpoint if the best chain is not known yet or we haven't already
	// previously searched.
	if b.bestChain == nil || (b.checkpointBlock == nil && b.nextCheckpoint == nil) {
		// Loop backwards through the available checkpoints to find one
		// that we already have.
		checkpointIndex := -1
		for i := numCheckpoints - 1; i >= 0; i-- {
			exists, err := b.db.ExistsSha(checkpoints[i].Hash)
			if err != nil {
				return nil, err
			}

			if exists {
				checkpointIndex = i
				break
			}
		}

		// No known latest checkpoint.  This will only happen on blocks
		// before the first known checkpoint.  So, set the next expected
		// checkpoint to the first checkpoint and return the fact there
		// is no latest known checkpoint block.
		if checkpointIndex == -1 {
			b.nextCheckpoint = &checkpoints[0]
			return nil, nil
		}

		// Cache the latest known checkpoint block for future lookups.
		checkpoint := checkpoints[checkpointIndex]
		block, err := b.db.FetchBlockBySha(checkpoint.Hash)
		if err != nil {
			return nil, err
		}
		b.checkpointBlock = block

		// Set the next expected checkpoint block accordingly.
		b.nextCheckpoint = nil
		if checkpointIndex < numCheckpoints-1 {
			b.nextCheckpoint = &checkpoints[checkpointIndex+1]
		}

		return block, nil
	}

	// At this point we've already searched for the latest known checkpoint,
	// so when there is no next checkpoint, the current checkpoint lockin
	// will always be the latest known checkpoint.
	if b.nextCheckpoint == nil {
		return b.checkpointBlock, nil
	}

	// When there is a next checkpoint and the height of the current best
	// chain does not exceed it, the current checkpoint lockin is still
	// the latest known checkpoint.
	if b.bestChain.height < b.nextCheckpoint.Height {
		return b.checkpointBlock, nil
	}

	// We've reached or exceeded the next checkpoint height.  Note that
	// once a checkpoint lockin has been reached, forks are prevented from
	// any blocks before the checkpoint, so we don't have to worry about the
	// checkpoint going away out from under us due to a chain reorganize.

	// Cache the latest known checkpoint block for future lookups.  Note
	// that if this lookup fails something is very wrong since the chain
	// has already passed the checkpoint which was verified as accurate
	// before inserting it.
	block, err := b.db.FetchBlockBySha(b.nextCheckpoint.Hash)
	if err != nil {
		return nil, err
	}
	b.checkpointBlock = block

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

	return b.checkpointBlock, nil
}

// isNonstandardTransaction determines whether a transaction contains any
// scripts which are not one of the standard types.
func isNonstandardTransaction(tx *btcutil.Tx) bool {
	// TODO(davec): Should there be checks for the input signature scripts?

	// Check all of the output public key scripts for non-standard scripts.
	for _, txOut := range tx.MsgTx().TxOut {
		scriptClass := btcscript.GetScriptClass(txOut.PkScript)
		if scriptClass == btcscript.NonStandardTy {
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
func (b *BlockChain) IsCheckpointCandidate(block *btcutil.Block) (bool, error) {
	// Checkpoints must be enabled.
	if b.noCheckpoints {
		return false, fmt.Errorf("checkpoints are disabled")
	}

	blockHash, err := block.Sha()
	if err != nil {
		return false, err
	}

	// A checkpoint must be in the main chain.
	exists, err := b.db.ExistsSha(blockHash)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	// A checkpoint must be at least CheckpointConfirmations blocks before
	// the end of the main chain.
	blockHeight := block.Height()
	_, mainChainHeight, err := b.db.NewestSha()
	if err != nil {
		return false, err
	}
	if blockHeight > (mainChainHeight - CheckpointConfirmations) {
		return false, nil
	}

	// Get the previous block.
	prevHash := &block.MsgBlock().Header.PrevBlock
	prevBlock, err := b.db.FetchBlockBySha(prevHash)
	if err != nil {
		return false, err
	}

	// Get the next block.
	nextHash, err := b.db.FetchBlockShaByHeight(blockHeight + 1)
	if err != nil {
		return false, err
	}
	nextBlock, err := b.db.FetchBlockBySha(nextHash)
	if err != nil {
		return false, err
	}

	// A checkpoint must have timestamps for the block and the blocks on
	// either side of it in order (due to the median time allowance this is
	// not always the case).
	prevTime := prevBlock.MsgBlock().Header.Timestamp
	curTime := block.MsgBlock().Header.Timestamp
	nextTime := nextBlock.MsgBlock().Header.Timestamp
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

	return true, nil
}
