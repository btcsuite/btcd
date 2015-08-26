// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrutil"
)

// checkCoinbaseUniqueHeight checks to ensure that for all blocks height > 1
// that the coinbase contains the height encoding to make coinbase hash collisions
// impossible.
func checkCoinbaseUniqueHeight(blockHeight int64, block *dcrutil.Block) error {
	if !(len(block.MsgBlock().Transactions) > 0) {
		str := fmt.Sprintf("block %v has no coinbase", block.Sha())
		return ruleError(ErrNoTransactions, str)
	}

	// Coinbase TxOut[0] is always tax, TxOut[1] is always
	// height + extranonce, so at least two outputs must
	// exist.
	if !(len(block.MsgBlock().Transactions[0].TxOut) > 1) {
		str := fmt.Sprintf("block %v is missing necessary coinbase "+
			"outputs", block.Sha())
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
			"type", block.Sha())
		return ruleError(ErrFirstTxNotCoinbase, str)
	}

	if len(nullData) < 4 {
		str := fmt.Sprintf("block %v txOut 1 has too short nullData "+
			"push to contain height", block.Sha())
		return ruleError(ErrFirstTxNotCoinbase, str)
	}

	// Check the height and ensure it is correct.
	cbHeight := binary.LittleEndian.Uint32(nullData[0:4])
	if cbHeight != uint32(blockHeight) {
		prevBlock := block.MsgBlock().Header.PrevBlock
		str := fmt.Sprintf("block %v txOut 1 has wrong height in "+
			"coinbase; want %v, got %v; prevBlock %v, header height %v",
			block.Sha(), blockHeight, cbHeight, prevBlock,
			block.MsgBlock().Header.Height)
		return ruleError(ErrCoinbaseHeight, str)
	}

	return nil
}

// IsFinalizedTransaction determines whether or not a transaction is finalized.
func IsFinalizedTransaction(tx *dcrutil.Tx, blockHeight int64,
	blockTime time.Time) bool {
	msgTx := tx.MsgTx()

	// Lock time of zero means the transaction is finalized.
	lockTime := msgTx.LockTime
	if lockTime == 0 {
		return true
	}

	// The lock time field of a transaction is either a block height at
	// which the transaction is finalized or a timestamp depending on if the
	// value is before the txscript.LockTimeThreshold.  When it is under the
	// threshold it is a block height.
	blockTimeOrHeight := int64(0)
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

// checkBlockContext peforms several validation checks on the block which depend
// on its position within the block chain.
//
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: The transaction are not checked to see if they are finalized
//    and the somewhat expensive duplication transaction check is not performed.
//
// The flags are also passed to checkBlockHeaderContext.  See its documentation
// for how the flags modify its behavior.
func (b *BlockChain) checkBlockContext(block *dcrutil.Block, prevNode *blockNode,
	flags BehaviorFlags) error {
	// The genesis block is valid by definition.
	if prevNode == nil {
		return nil
	}

	// Perform all block header related validation checks.
	header := &block.MsgBlock().Header
	err := b.checkBlockHeaderContext(header, prevNode, flags)
	if err != nil {
		return err
	}

	fastAdd := flags&BFFastAdd == BFFastAdd
	if !fastAdd {
		// The height of this block is one more than the referenced
		// previous block.
		blockHeight := prevNode.height + 1

		// Ensure all transactions in the block are finalized.
		for _, tx := range block.Transactions() {
			if !IsFinalizedTransaction(tx, blockHeight,
				header.Timestamp) {

				str := fmt.Sprintf("block contains unfinalized regular "+
					"transaction %v", tx.Sha())
				return ruleError(ErrUnfinalizedTx, str)
			}
		}
		for _, stx := range block.STransactions() {
			if !IsFinalizedTransaction(stx, blockHeight,
				header.Timestamp) {

				str := fmt.Sprintf("block contains unfinalized stake "+
					"transaction %v", stx.Sha())
				return ruleError(ErrUnfinalizedTx, str)
			}
		}

		// Check that the node is at the correct height in the blockchain,
		// as specified in the block header.
		if blockHeight != int64(block.MsgBlock().Header.Height) {
			errStr := fmt.Sprintf("Block header height invalid; expected %v"+
				" but %v was found", blockHeight, header.Height)
			return ruleError(ErrBadBlockHeight, errStr)
		}

		// Check that the coinbase contains at minimum the block
		// height in output 1.
		if blockHeight > 1 {
			err := checkCoinbaseUniqueHeight(blockHeight, block)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// maybeAcceptBlock potentially accepts a block into the memory block chain.
// It performs several validation checks which depend on its position within
// the block chain before adding it.  The block is expected to have already gone
// through ProcessBlock before calling this function with it.
//
// The flags modify the behavior of this function as follows:
//  - BFDryRun: The memory chain index will not be pruned and no accept
//    notification will be sent since the block is not being accepted.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) maybeAcceptBlock(block *dcrutil.Block,
	flags BehaviorFlags) (bool, error) {
	dryRun := flags&BFDryRun == BFDryRun

	// Get a block node for the block previous to this one.  Will be nil
	// if this is the genesis block.
	prevNode, err := b.getPrevNodeFromBlock(block)
	if err != nil {
		log.Debugf("getPrevNodeFromBlock: %v", err)
		return false, err
	}

	// The height of this block is one more than the referenced previous
	// block.
	blockHeight := int64(0)
	if prevNode != nil {
		blockHeight = prevNode.height + 1
	}
	block.SetHeight(blockHeight)

	// The block must pass all of the validation rules which depend on the
	// position of the block within the block chain.
	err = b.checkBlockContext(block, prevNode, flags)
	if err != nil {
		return false, err
	}

	// Prune block nodes which are no longer needed before creating
	// a new node.
	if !dryRun {
		err = b.pruneBlockNodes()
		if err != nil {
			return false, err
		}
	}

	// Create a new block node for the block and add it to the in-memory
	// block chain (could be either a side chain or the main chain).
	blockHeader := &block.MsgBlock().Header
	var voteBitsStake []uint16
	for _, stx := range block.STransactions() {
		if is, _ := stake.IsSSGen(stx); is {
			vb := stake.SSGenVoteBits(stx)
			voteBitsStake = append(voteBitsStake, vb)
		}
	}

	newNode := newBlockNode(blockHeader, block.Sha(), blockHeight, voteBitsStake)
	if prevNode != nil {
		newNode.parent = prevNode
		newNode.height = blockHeight
		newNode.workSum.Add(prevNode.workSum, newNode.workSum)
	}

	// Connect the passed block to the chain while respecting proper chain
	// selection according to the chain with the most proof of work.  This
	// also handles validation of the transaction scripts.
	var onMainChain bool
	onMainChain, err = b.connectBestChain(newNode, block, flags)
	if err != nil {
		return false, err
	}

	// Notify the caller that the new block was accepted into the block
	// chain.  The caller would typically want to react by relaying the
	// inventory to other peers.
	if !dryRun {
		b.chainLock.Unlock()
		b.sendNotification(NTBlockAccepted,
			&BlockAcceptedNtfnsData{onMainChain, block})
		b.chainLock.Lock()
	}

	return onMainChain, nil
}
