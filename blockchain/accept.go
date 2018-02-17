// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
)

// checkCoinbaseUniqueHeight checks to ensure that for all blocks height > 1
// that the coinbase contains the height encoding to make coinbase hash collisions
// impossible.
func checkCoinbaseUniqueHeight(blockHeight int64, block *dcrutil.Block) error {
	// Coinbase TxOut[0] is always tax, TxOut[1] is always
	// height + extranonce, so at least two outputs must
	// exist.
	if len(block.MsgBlock().Transactions[0].TxOut) < 2 {
		str := fmt.Sprintf("block %v is missing necessary coinbase "+
			"outputs", block.Hash())
		return ruleError(ErrFirstTxNotCoinbase, str)
	}

	// The first 4 bytes of the NullData output must be the
	// encoded height of the block, so that every coinbase
	// created has a unique transaction hash.
	nullData, err := txscript.GetNullDataContent(
		block.MsgBlock().Transactions[0].TxOut[1].Version,
		block.MsgBlock().Transactions[0].TxOut[1].PkScript)
	if err != nil {
		str := fmt.Sprintf("block %v txOut 1 has wrong pkScript "+
			"type", block.Hash())
		return ruleError(ErrFirstTxNotCoinbase, str)
	}

	if len(nullData) < 4 {
		str := fmt.Sprintf("block %v txOut 1 has too short nullData "+
			"push to contain height", block.Hash())
		return ruleError(ErrFirstTxNotCoinbase, str)
	}

	// Check the height and ensure it is correct.
	cbHeight := binary.LittleEndian.Uint32(nullData[0:4])
	if cbHeight != uint32(blockHeight) {
		prevBlock := block.MsgBlock().Header.PrevBlock
		str := fmt.Sprintf("block %v txOut 1 has wrong height in "+
			"coinbase; want %v, got %v; prevBlock %v, header height %v",
			block.Hash(), blockHeight, cbHeight, prevBlock,
			block.MsgBlock().Header.Height)
		return ruleError(ErrCoinbaseHeight, str)
	}

	return nil
}

// IsFinalizedTransaction determines whether or not a transaction is finalized.
func IsFinalizedTransaction(tx *dcrutil.Tx, blockHeight int64, blockTime time.Time) bool {
	// Lock time of zero means the transaction is finalized.
	msgTx := tx.MsgTx()
	lockTime := msgTx.LockTime
	if lockTime == 0 {
		return true
	}

	// The lock time field of a transaction is either a block height at
	// which the transaction is finalized or a timestamp depending on if the
	// value is before the txscript.LockTimeThreshold.  When it is under the
	// threshold it is a block height.
	var blockTimeOrHeight int64
	if lockTime < txscript.LockTimeThreshold {
		blockTimeOrHeight = blockHeight
	} else {
		blockTimeOrHeight = blockTime.Unix()
	}
	if int64(lockTime) < blockTimeOrHeight {
		return true
	}

	// At this point, the transaction's lock time hasn't occurred yet, but
	// the transaction might still be finalized if the sequence number
	// for all transaction inputs is maxed out.
	for _, txIn := range msgTx.TxIn {
		if txIn.Sequence != math.MaxUint32 {
			return false
		}
	}
	return true
}

// maybeAcceptBlock potentially accepts a block into the block chain and, if
// accepted, returns whether or not it is on the main chain.  It performs
// several validation checks which depend on its position within the block chain
// before adding it.  The block is expected to have already gone through
// ProcessBlock before calling this function with it.
//
// The flags modify the behavior of this function as follows:
//  - BFDryRun: The memory chain index will not be pruned and no accept
//    notification will be sent since the block is not being accepted.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) maybeAcceptBlock(block *dcrutil.Block, flags BehaviorFlags) (bool, error) {
	dryRun := flags&BFDryRun == BFDryRun

	// Get a block node for the block previous to this one.  Will be nil
	// if this is the genesis block.
	prevNode, err := b.index.PrevNodeFromBlock(block)
	if err != nil {
		log.Debugf("PrevNodeFromBlock: %v", err)
		return false, err
	}

	// This function should never be called with orphan blocks or the
	// genesis block.
	if prevNode == nil {
		prevHash := &block.MsgBlock().Header.PrevBlock
		str := fmt.Sprintf("previous block %s is not known", prevHash)
		return false, ruleError(ErrMissingParent, str)
	}

	// There is no need to validate the block if an ancestor is already
	// known to be invalid.
	if b.index.NodeStatus(prevNode).KnownInvalid() {
		prevHash := &block.MsgBlock().Header.PrevBlock
		str := fmt.Sprintf("previous block %s is known to be invalid",
			prevHash)
		return false, ruleError(ErrInvalidAncestorBlock, str)
	}

	// The block must pass all of the validation rules which depend on the
	// position of the block within the block chain.
	err = b.checkBlockContext(block, prevNode, flags)
	if err != nil {
		return false, err
	}

	// Prune stake nodes which are no longer needed before creating a new
	// node.
	if !dryRun {
		b.pruner.pruneChainIfNeeded()
	}

	// Create a new block node for the block and add it to the block index.
	// The block could either be on a side chain or the main chain, but it
	// starts off as a side chain regardless.
	blockHeader := &block.MsgBlock().Header
	newNode := newBlockNode(blockHeader, prevNode)
	newNode.populateTicketInfo(stake.FindSpentTicketsInBlock(block.MsgBlock()))
	newNode.status = statusDataStored
	b.index.AddNode(newNode)

	// Insert the block into the database if it's not already there.  Even
	// though it is possible the block will ultimately fail to connect, it
	// has already passed all proof-of-work and validity tests which means
	// it would be prohibitively expensive for an attacker to fill up the
	// disk with a bunch of blocks that fail to connect.  This is necessary
	// since it allows block download to be decoupled from the much more
	// expensive connection logic.  It also has some other nice properties
	// such as making blocks that never become part of the main chain or
	// blocks that fail to connect available for further analysis.
	//
	// Also, store the associated block index entry when not running in dry
	// run mode.
	err = b.db.Update(func(dbTx database.Tx) error {
		if err := dbMaybeStoreBlock(dbTx, block); err != nil {
			return err
		}

		if !dryRun {
			if err := dbPutBlockNode(dbTx, newNode); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return false, err
	}

	// Remove the node from the block index and disconnect it from the
	// parent node when running in dry run mode.
	if dryRun {
		defer func() {
			b.index.RemoveNode(newNode)
		}()
	}

	// Fetching a stake node could enable a new DoS vector, so restrict
	// this only to blocks that are recent in history.
	if newNode.height < b.bestNode.height-minMemoryNodes {
		newNode.stakeNode, err = b.fetchStakeNode(newNode)
		if err != nil {
			return false, err
		}
		newNode.stakeUndoData = newNode.stakeNode.UndoData()
	}

	// Grab the parent block since it is required throughout the block
	// connection process.
	parent, err := b.fetchBlockByHash(&newNode.parentHash)
	if err != nil {
		return false, err
	}

	// Connect the passed block to the chain while respecting proper chain
	// selection according to the chain with the most proof of work.  This
	// also handles validation of the transaction scripts.
	isMainChain, err := b.connectBestChain(newNode, block, parent, flags)
	if err != nil {
		return false, err
	}

	// Notify the caller that the new block was accepted into the block
	// chain.  The caller would typically want to react by relaying the
	// inventory to other peers.
	if !dryRun {
		b.chainLock.Unlock()
		b.sendNotification(NTBlockAccepted,
			&BlockAcceptedNtfnsData{isMainChain, block})
		b.chainLock.Lock()
	}

	return isMainChain, nil
}
