// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcchain

import (
	"fmt"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// maybeAcceptBlock potentially accepts a block into the memory block chain.
// It performs several validation checks which depend on its position within
// the block chain before adding it.  The block is expected to have already gone
// through ProcessBlock before calling this function with it.
func (b *BlockChain) maybeAcceptBlock(block *btcutil.Block) error {
	// Get a block node for the block previous to this one.  Will be nil
	// if this is the genesis block.
	prevNode, err := b.getPrevNodeFromBlock(block)
	if err != nil {
		log.Errorf("getPrevNodeFromBlock: %v", err)
		return err
	}

	// The height of this block one more than the referenced previous block.
	blockHeight := int64(0)
	if prevNode != nil {
		blockHeight = prevNode.height + 1
	}
	block.SetHeight(blockHeight)

	// Ensure the difficulty specified in the block header matches the
	// calculated difficulty based on the previous block and difficulty
	// retarget rules.
	blockHeader := block.MsgBlock().Header
	expectedDifficulty, err := b.calcNextRequiredDifficulty(prevNode, block)
	if err != nil {
		return err
	}
	blockDifficulty := blockHeader.Bits
	if blockDifficulty != expectedDifficulty {
		str := "block difficulty of %d is not the expected value of %d"
		str = fmt.Sprintf(str, blockDifficulty, expectedDifficulty)
		return RuleError(str)
	}

	// Ensure the timestamp for the block header is after the median time of
	// the last several blocks (medianTimeBlocks).
	medianTime, err := b.calcPastMedianTime(prevNode)
	if err != nil {
		log.Errorf("calcPastMedianTime: %v", err)
		return err
	}
	if !blockHeader.Timestamp.After(medianTime) {
		str := "block timestamp of %v is not after expected %v"
		str = fmt.Sprintf(str, blockHeader.Timestamp, medianTime)
		return RuleError(str)
	}

	// Ensure all transactions in the block are finalized.
	for i, tx := range block.MsgBlock().Transactions {
		if !IsFinalizedTransaction(tx, blockHeight, blockHeader.Timestamp) {
			// Use the TxSha function from the block rather
			// than the transaction itself since the block version
			// is cached.  Also, it's safe to ignore the error here
			// since the only reason TxSha can fail is if the index
			// is out of range which is impossible here.
			txSha, _ := block.TxSha(i)
			str := fmt.Sprintf("block contains unfinalized "+
				"transaction %v", txSha)
			return RuleError(str)
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
		return RuleError(str)
	}

	// Reject version 1 blocks once a majority of the network has upgraded.
	// Rules:
	//  95% (950 / 1000) for main network
	//  75% (75 / 100) for the test network
	// This is part of BIP_0034.
	if blockHeader.Version == 1 {
		minRequired := uint64(950)
		numToCheck := uint64(1000)
		if b.btcnet == btcwire.TestNet3 || b.btcnet == btcwire.TestNet {
			minRequired = 75
			numToCheck = 100
		}
		if b.isMajorityVersion(2, prevNode, minRequired, numToCheck) {
			str := "new blocks with version %d are no longer valid"
			str = fmt.Sprintf(str, blockHeader.Version)
			return RuleError(str)
		}
	}

	// Ensure coinbase starts with serialized block heights for blocks
	// whose version is the serializedHeightVersion or newer once a majority
	// of the network has upgraded.
	// Rules:
	//  75% (750 / 1000) for main network
	//  51% (51 / 100) for the test network
	// This is part of BIP_0034.
	if blockHeader.Version >= serializedHeightVersion {
		minRequired := uint64(750)
		numToCheck := uint64(1000)
		if b.btcnet == btcwire.TestNet3 || b.btcnet == btcwire.TestNet {
			minRequired = 51
			numToCheck = 100
		}
		if b.isMajorityVersion(serializedHeightVersion, prevNode,
			minRequired, numToCheck) {

			expectedHeight := int64(0)
			if prevNode != nil {
				expectedHeight = prevNode.height + 1
			}
			coinbaseTx := block.MsgBlock().Transactions[0]
			err := checkSerializedHeight(coinbaseTx, expectedHeight)
			if err != nil {
				return err
			}
		}
	}

	// Prune block nodes which are no longer needed before creating a new
	// node.
	err = b.pruneBlockNodes()
	if err != nil {
		return err
	}

	// Create a new block node for the block and add it to the in-memory
	// block chain (could be either a side chain or the main chain).
	newNode := newBlockNode(block)
	if prevNode != nil {
		newNode.parent = prevNode
		newNode.height = blockHeight
		newNode.workSum.Add(prevNode.workSum, newNode.workSum)
	}

	// Connect the passed block to the chain while respecting proper chain
	// selection according to the chain with the most proof of work.  This
	// also handles validation of the transaction scripts.
	err = b.connectBestChain(newNode, block)
	if err != nil {
		return err
	}

	// Notify the caller that the new block was accepted into the block
	// chain.  The caller would typically want to react by relaying the
	// inventory to other peers.
	b.sendNotification(NTBlockAccepted, block)

	return nil
}
