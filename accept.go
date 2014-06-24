// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcchain

import (
	"fmt"
	"github.com/conformal/btcutil"
)

// maybeAcceptBlock potentially accepts a block into the memory block chain.
// It performs several validation checks which depend on its position within
// the block chain before adding it.  The block is expected to have already gone
// through ProcessBlock before calling this function with it.
// The fastAdd argument modifies the behavior of the function by avoiding the
// somewhat expensive operation: BIP34 validation, it also passes the argument
// down to connectBestChain()
func (b *BlockChain) maybeAcceptBlock(block *btcutil.Block, fastAdd bool) error {
	// Get a block node for the block previous to this one.  Will be nil
	// if this is the genesis block.
	prevNode, err := b.getPrevNodeFromBlock(block)
	if err != nil {
		log.Errorf("getPrevNodeFromBlock: %v", err)
		return err
	}

	// The height of this block is one more than the referenced previous
	// block.
	blockHeight := int64(0)
	if prevNode != nil {
		blockHeight = prevNode.height + 1
	}
	block.SetHeight(blockHeight)

	blockHeader := &block.MsgBlock().Header
	if !fastAdd {
		// Ensure the difficulty specified in the block header matches
		// the calculated difficulty based on the previous block and
		// difficulty retarget rules.
		expectedDifficulty, err := b.calcNextRequiredDifficulty(prevNode,
			block.MsgBlock().Header.Timestamp)
		if err != nil {
			return err
		}
		blockDifficulty := blockHeader.Bits
		if blockDifficulty != expectedDifficulty {
			str := "block difficulty of %d is not the expected value of %d"
			str = fmt.Sprintf(str, blockDifficulty, expectedDifficulty)
			return ruleError(ErrUnexpectedDifficulty, str)
		}

		// Ensure the timestamp for the block header is after the
		// median time of the last several blocks (medianTimeBlocks).
		medianTime, err := b.calcPastMedianTime(prevNode)
		if err != nil {
			log.Errorf("calcPastMedianTime: %v", err)
			return err
		}
		if !blockHeader.Timestamp.After(medianTime) {
			str := "block timestamp of %v is not after expected %v"
			str = fmt.Sprintf(str, blockHeader.Timestamp,
				medianTime)
			return ruleError(ErrTimeTooOld, str)
		}

		// Ensure all transactions in the block are finalized.
		for _, tx := range block.Transactions() {
			if !IsFinalizedTransaction(tx, blockHeight,
				blockHeader.Timestamp) {
				str := fmt.Sprintf("block contains "+
					"unfinalized transaction %v", tx.Sha())
				return ruleError(ErrUnfinalizedTx, str)
			}
		}

	}

	// Ensure chain matches up to predetermined checkpoints.
	// It's safe to ignore the error on Sha since it's already cached.
	blockHash, _ := block.Sha()
	if !b.verifyCheckpoint(blockHeight, blockHash) {
		// TODO(davec): This should probably be a distinct error type
		// (maybe CheckpointError).  Since this error shouldn't happen
		// unless the peer is connected to a rogue network serving up an
		// alternate chain, the caller would likely need to react by
		// disconnecting peers and rolling back the chain to the last
		// known good point.
		str := fmt.Sprintf("block at height %d does not match "+
			"checkpoint hash", blockHeight)
		return ruleError(ErrBadCheckpoint, str)
	}

	// Find the previous checkpoint and prevent blocks which fork the main
	// chain before it.  This prevents storage of new, otherwise valid,
	// blocks which build off of old blocks that are likely at a much easier
	// difficulty and therefore could be used to waste cache and disk space.
	checkpointBlock, err := b.findPreviousCheckpoint()
	if err != nil {
		return err
	}
	if checkpointBlock != nil && blockHeight < checkpointBlock.Height() {
		str := fmt.Sprintf("block at height %d forks the main chain "+
			"before the previous checkpoint at height %d",
			blockHeight, checkpointBlock.Height())
		return ruleError(ErrForkTooOld, str)
	}

	if !fastAdd {
		// Reject version 1 blocks once a majority of the network has
		// upgraded.  This is part of BIP0034.
		if blockHeader.Version == 1 {
			if b.isMajorityVersion(2, prevNode,
				b.netParams.BlockV1RejectNumRequired,
				b.netParams.BlockV1RejectNumToCheck) {

				str := "new blocks with version %d are no " +
					"longer valid"
				str = fmt.Sprintf(str, blockHeader.Version)
				return ruleError(ErrBlockVersionTooOld, str)
			}
		}

		// Ensure coinbase starts with serialized block heights for
		// blocks whose version is the serializedHeightVersion or
		// newer once a majority of the network has upgraded.  This is
		// part of BIP0034.
		if blockHeader.Version >= serializedHeightVersion {
			if b.isMajorityVersion(serializedHeightVersion,
				prevNode,
				b.netParams.CoinbaseBlockHeightNumRequired,
				b.netParams.CoinbaseBlockHeightNumToCheck) {

				expectedHeight := int64(0)
				if prevNode != nil {
					expectedHeight = prevNode.height + 1
				}
				coinbaseTx := block.Transactions()[0]
				err := checkSerializedHeight(coinbaseTx,
					expectedHeight)
				if err != nil {
					return err
				}
			}
		}
	}

	// Prune block nodes which are no longer needed before creating
	// a new node.
	err = b.pruneBlockNodes()
	if err != nil {
		return err
	}

	// Create a new block node for the block and add it to the in-memory
	// block chain (could be either a side chain or the main chain).
	newNode := newBlockNode(blockHeader, blockHash, blockHeight)
	if prevNode != nil {
		newNode.parent = prevNode
		newNode.height = blockHeight
		newNode.workSum.Add(prevNode.workSum, newNode.workSum)
	}

	// Connect the passed block to the chain while respecting proper chain
	// selection according to the chain with the most proof of work.  This
	// also handles validation of the transaction scripts.
	err = b.connectBestChain(newNode, block, fastAdd)
	if err != nil {
		return err
	}

	// Notify the caller that the new block was accepted into the block
	// chain.  The caller would typically want to react by relaying the
	// inventory to other peers.
	b.sendNotification(NTBlockAccepted, block)

	return nil
}
