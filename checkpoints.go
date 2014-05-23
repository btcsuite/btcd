// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcchain

import (
	"fmt"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// CheckpointConfirmations is the number of blocks before the end of the current
// best block chain that a good checkpoint candidate must be.
const CheckpointConfirmations = 2016

// Checkpoint identifies a known good point in the block chain.  Using
// checkpoints allows a few optimizations for old blocks during initial download
// and also prevents forks from old blocks.
//
// Each checkpoint is selected by the core developers based upon several
// factors.  See the documentation for IsCheckpointCandidate for details
// on the selection criteria.
//
// As alluded to above, this package provides an IsCheckpointCandidate function
// which programatically identifies a block as a checkpoint candidate.  The idea
// is that candidates are reviewed by a developer to make the final decision and
// then manually added to the list of checkpoints.
type Checkpoint struct {
	Height int64
	Hash   *btcwire.ShaHash
}

// checkpointData groups checkpoints and other pertinent checkpoint data into
// a single type.
type checkpointData struct {
	// Checkpoints ordered from oldest to newest.
	checkpoints []Checkpoint

	// A map that will be automatically generated with the heights from
	// the checkpoints as keys.
	checkpointsByHeight map[int64]*Checkpoint
}

// checkpointDataMainNet contains checkpoint data for the main network.
var checkpointDataMainNet = checkpointData{
	checkpoints: []Checkpoint{
		{11111, newShaHashFromStr("0000000069e244f73d78e8fd29ba2fd2ed618bd6fa2ee92559f542fdb26e7c1d")},
		{33333, newShaHashFromStr("000000002dd5588a74784eaa7ab0507a18ad16a236e7b1ce69f00d7ddfb5d0a6")},
		{74000, newShaHashFromStr("0000000000573993a3c9e41ce34471c079dcf5f52a0e824a81e7f953b8661a20")},
		{105000, newShaHashFromStr("00000000000291ce28027faea320c8d2b054b2e0fe44a773f3eefb151d6bdc97")},
		{134444, newShaHashFromStr("00000000000005b12ffd4cd315cd34ffd4a594f430ac814c91184a0d42d2b0fe")},
		{168000, newShaHashFromStr("000000000000099e61ea72015e79632f216fe6cb33d7899acb35b75c8303b763")},
		{193000, newShaHashFromStr("000000000000059f452a5f7340de6682a977387c17010ff6e6c3bd83ca8b1317")},
		{210000, newShaHashFromStr("000000000000048b95347e83192f69cf0366076336c639f9b7228e9ba171342e")},
		{216116, newShaHashFromStr("00000000000001b4f4b433e81ee46494af945cf96014816a4e2370f11b23df4e")},
		{225430, newShaHashFromStr("00000000000001c108384350f74090433e7fcf79a606b8e797f065b130575932")},
		{250000, newShaHashFromStr("000000000000003887df1f29024b06fc2200b55f8af8f35453d7be294df2d214")},
		{267300, newShaHashFromStr("000000000000000a83fbd660e918f218bf37edd92b748ad940483c7c116179ac")},
		{279000, newShaHashFromStr("0000000000000001ae8c72a0b0c301f67e3afca10e819efa9041e458e9bd7e40")},
		{300255, newShaHashFromStr("0000000000000000162804527c6e9b9f0563a280525f9d08c12041def0a0f3b2")},
	},
	checkpointsByHeight: nil, // Automatically generated in init.
}

// checkpointDataTestNet3 contains checkpoint data for the test network (version
// 3).
var checkpointDataTestNet3 = checkpointData{
	checkpoints: []Checkpoint{
		{546, newShaHashFromStr("000000002a936ca763904c3c35fce2f3556c559c0214345d31b1bcebf76acb70")},
	},
	checkpointsByHeight: nil, // Automatically generated in init.
}

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

// checkpointData returns the appropriate checkpoint data set depending on the
// network configured for the block chain.
func (b *BlockChain) checkpointData() *checkpointData {
	switch b.btcnet {
	case btcwire.TestNet3:
		return &checkpointDataTestNet3
	case btcwire.MainNet:
		return &checkpointDataMainNet
	}
	return nil
}

// Checkpoints returns a slice of checkpoints (regardless of whether they are
// already known).  When checkpoints are disabled or there are no checkpoints
// for the active network, it will return nil.
func (b *BlockChain) Checkpoints() []Checkpoint {
	if b.noCheckpoints || b.checkpointData() == nil {
		return nil
	}

	return b.checkpointData().checkpoints
}

// LatestCheckpoint returns the most recent checkpoint (regardless of whether it
// is already known).  When checkpoints are disabled or there are no checkpoints
// for the active network, it will return nil.
func (b *BlockChain) LatestCheckpoint() *Checkpoint {
	if b.noCheckpoints || b.checkpointData() == nil {
		return nil
	}

	checkpoints := b.checkpointData().checkpoints
	return &checkpoints[len(checkpoints)-1]
}

// verifyCheckpoint returns whether the passed block height and hash combination
// match the hard-coded checkpoint data.  It also returns true if there is no
// checkpoint data for the passed block height.
func (b *BlockChain) verifyCheckpoint(height int64, hash *btcwire.ShaHash) bool {
	if b.noCheckpoints || b.checkpointData() == nil {
		return true
	}

	// Nothing to check if there is no checkpoint data for the block height.
	checkpoint, exists := b.checkpointData().checkpointsByHeight[height]
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
	if b.noCheckpoints || b.checkpointData() == nil {
		return nil, nil
	}

	// No checkpoints.
	checkpoints := b.checkpointData().checkpoints
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
			if b.db.ExistsSha(checkpoints[i].Hash) {
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
	if !b.db.ExistsSha(blockHash) {
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

// init is called on package load.
func init() {
	// Generate the checkpoint by height maps from the checkpoint data
	// when the package loads.
	checkpointInitializeList := []*checkpointData{
		&checkpointDataMainNet,
		&checkpointDataTestNet3,
	}
	for _, data := range checkpointInitializeList {
		data.checkpointsByHeight = make(map[int64]*Checkpoint)
		for i := range data.checkpoints {
			checkpoint := &data.checkpoints[i]
			data.checkpointsByHeight[checkpoint.Height] = checkpoint
		}
	}
}
