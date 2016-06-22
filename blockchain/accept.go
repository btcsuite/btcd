// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import "github.com/btcsuite/btcutil"

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
// The flags are also passed to checkBlockContext and connectBestChain.  See
// their documentation for how the flags modify their behavior.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) maybeAcceptBlock(block *btcutil.Block, flags BehaviorFlags) (bool, error) {
	dryRun := flags&BFDryRun == BFDryRun

	// Get a block node for the block previous to this one.  Will be nil
	// if this is the genesis block.
	prevNode, err := b.getPrevNodeFromBlock(block)
	if err != nil {
		log.Errorf("getPrevNodeFromBlock: %v", err)
		return false, err
	}

	// The height of this block is one more than the referenced previous
	// block.
	blockHeight := int32(0)
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

	// Create a new block node for the block and add it to the in-memory
	// block chain (could be either a side chain or the main chain).
	blockHeader := &block.MsgBlock().Header
	newNode := newBlockNode(blockHeader, block.Hash(), blockHeight)
	if prevNode != nil {
		newNode.parent = prevNode
		newNode.height = blockHeight
		newNode.workSum.Add(prevNode.workSum, newNode.workSum)
	}

	// Connect the passed block to the chain while respecting proper chain
	// selection according to the chain with the most proof of work.  This
	// also handles validation of the transaction scripts.
	isMainChain, err := b.connectBestChain(newNode, block, flags)
	if err != nil {
		return false, err
	}

	// Notify the caller that the new block was accepted into the block
	// chain.  The caller would typically want to react by relaying the
	// inventory to other peers.
	if !dryRun {
		b.chainLock.Unlock()
		b.sendNotification(NTBlockAccepted, block)
		b.chainLock.Lock()
	}

	return isMainChain, nil
}
