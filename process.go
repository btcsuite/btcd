// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcchain

import (
	"fmt"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// RuleError identifies a rule violation.  It is used to indicate that
// processing of a block or transaction failed due to one of the many validation
// rules.  The caller can use type assertions to determine if a failure was
// specifically due to a rule violation.
type RuleError string

// Error satisfies the error interface to print human-readable errors.
func (e RuleError) Error() string {
	return string(e)
}

// blockExists determines whether a block with the given hash exists either in
// the main chain or any side chains.
func (b *BlockChain) blockExists(hash *btcwire.ShaHash) bool {
	// Check memory chain first (could be main chain or side chain blocks).
	if _, ok := b.index[*hash]; ok {
		return true
	}

	// Check in database (rest of main chain not in memory).
	return b.db.ExistsSha(hash)
}

// processOrphans determines if there are any orphans which depend on the passed
// block hash (they are no longer orphans if true) and potentially accepts them.
// It repeats the process for the newly accepted blocks (to detect further
// orphans which may no longer be orphans) until there are no more.
func (b *BlockChain) processOrphans(hash *btcwire.ShaHash) error {
	processHashes := []*btcwire.ShaHash{hash}
	for len(processHashes) > 0 {
		// Pop the first hash to process from the slice.
		processHash := processHashes[0]
		processHashes = processHashes[1:]

		// Look up all orphans that are parented by the block we just
		// accepted.  This will typically only be one, but it could
		// be multiple if multiple blocks are mined and broadcast
		// around the same time.  The one with the most proof of work
		// will eventually win out.
		for _, orphan := range b.prevOrphans[*processHash] {
			// Remove the orphan from the orphan pool.
			// It's safe to ignore the error on Sha since the hash
			// is already cached.
			orphanHash, _ := orphan.block.Sha()
			b.removeOrphanBlock(orphan)

			// Potentially accept the block into the block chain.
			err := b.maybeAcceptBlock(orphan.block)
			if err != nil {
				return err
			}

			// Add this block to the list of blocks to process so
			// any orphan blocks that depend on this block are
			// handled too.
			processHashes = append(processHashes, orphanHash)
		}
	}
	return nil
}

// ProcessBlock is the main workhorse for handling insertion of new blocks into
// the block chain.  It includes functionality such as rejecting duplicate
// blocks, ensuring blocks follow all rules, orphan handling, and insertion into
// the block chain along with best chain selection and reorganization.
func (b *BlockChain) ProcessBlock(block *btcutil.Block) error {
	blockHash, err := block.Sha()
	if err != nil {
		return err
	}
	log.Debugf("Processing block %v", blockHash)

	// The block must not already exist in the main chain or side chains.
	if b.blockExists(blockHash) {
		str := fmt.Sprintf("already have block %v", blockHash)
		return RuleError(str)
	}

	// The block must not already exist as an orphan.
	if _, exists := b.orphans[*blockHash]; exists {
		str := fmt.Sprintf("already have block (orphan) %v", blockHash)
		return RuleError(str)
	}

	// Perform preliminary sanity checks on the block and its transactions.
	err = b.checkBlockSanity(block)
	if err != nil {
		return err
	}

	// Find the latest known checkpoint and perform some additional checks
	// based on the checkpoint.  This provides a few nice properties such as
	// preventing forks from blocks before the last checkpoint, rejecting
	// easy to mine, but otherwise bogus, blocks that could be used to eat
	// memory, and ensuring expected (versus claimed) proof of work
	// requirements since the last checkpoint are met.
	blockHeader := block.MsgBlock().Header
	checkpointBlock, err := b.findLatestKnownCheckpoint()
	if err != nil {
		return err
	}
	if checkpointBlock != nil {
		// Ensure the block timestamp is after the checkpoint timestamp.
		checkpointHeader := checkpointBlock.MsgBlock().Header
		checkpointTime := checkpointHeader.Timestamp
		if blockHeader.Timestamp.Before(checkpointTime) {
			str := fmt.Sprintf("block %v has timestamp %v before "+
				"last checkpoint timestamp %v", blockHash,
				blockHeader.Timestamp, checkpointTime)
			return RuleError(str)
		}

		// Even though the checks prior to now have already ensured the
		// proof of work exceeds the claimed amount, the claimed amount
		// is a field in the block header which could be forged.  This
		// check ensures the proof of work is at least the minimum
		// expected based on elapsed time since the last checkpoint and
		// maximum adjustment allowed by the retarget rules.
		duration := blockHeader.Timestamp.Sub(checkpointTime)
		requiredTarget := CompactToBig(b.calcEasiestDifficulty(
			checkpointHeader.Bits, duration))
		currentTarget := CompactToBig(blockHeader.Bits)
		if currentTarget.Cmp(requiredTarget) > 0 {
			str := fmt.Sprintf("block target difficulty of %064x "+
				"is too low when compared to the previous "+
				"checkpoint", currentTarget)
			return RuleError(str)
		}
	}

	// Handle orphan blocks.
	prevHash := &blockHeader.PrevBlock
	if !prevHash.IsEqual(zeroHash) && !b.blockExists(prevHash) {
		// Add the orphan block to the orphan pool.
		log.Infof("Adding orphan block %v with parent %v", blockHash,
			prevHash)
		b.addOrphanBlock(block)

		// Get the hash for the head of the orphaned block chain for
		// this block and notify the caller so it can request missing
		// blocks.
		orphanRoot := b.GetOrphanRoot(blockHash)
		b.sendNotification(NTOrphanBlock, orphanRoot)
		return nil
	}

	// The block has passed all context independent checks and appears sane
	// enough to potentially accept it into the block chain.
	err = b.maybeAcceptBlock(block)
	if err != nil {
		return err
	}

	// Accept any orphan blocks that depend on this block (they are no
	// longer orphans) and repeat for those accepted blocks until there are
	// no more.
	err = b.processOrphans(blockHash)
	if err != nil {
		return err
	}

	log.Debugf("Accepted block %v", blockHash)
	return nil
}
