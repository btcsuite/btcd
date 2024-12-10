// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/database"
)

// maybeAcceptBlock potentially accepts a block into the block chain and, if
// accepted, returns whether or not it is on the main chain.  It performs
// several validation checks which depend on its position within the block chain
// before adding it.  The block is expected to have already gone through
// ProcessBlock before calling this function with it.
//
// The flags are also passed to checkBlockContext and connectBestChain.  See
// their documentation for how the flags modify their behavior.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) maybeAcceptBlock(block *btcutil.Block, flags BehaviorFlags) (bool, error) {
	// The height of this block is one more than the referenced previous
	// block.
	prevHash := &block.MsgBlock().Header.PrevBlock
	prevNode := b.index.LookupNode(prevHash)
	if prevNode == nil {
		str := fmt.Sprintf("previous block %s is unknown", prevHash)
		return false, ruleError(ErrPreviousBlockUnknown, str)
	} else if b.index.NodeStatus(prevNode).KnownInvalid() {
		str := fmt.Sprintf("previous block %s is known to be invalid", prevHash)
		return false, ruleError(ErrInvalidAncestorBlock, str)
	}

	blockHeight := prevNode.height + 1
	block.SetHeight(blockHeight)

	// The block must pass all of the validation rules which depend on the
	// position of the block within the block chain.
	err := b.checkBlockContext(block, prevNode, flags)
	if err != nil {
		return false, err
	}

	// Store the block in parallel if we're in headers first mode.  The
	// headers were already checked and this block is under the checkpoint
	// so it's safe to just add it to the database while the block
	// validation is happening.
	var wg sync.WaitGroup
	var dbStoreError error
	if flags&BFFastAdd == BFFastAdd {
		go func() {
			wg.Add(1)
			defer wg.Done()
			// Insert the block into the database if it's not already there.  Even
			// though it is possible the block will ultimately fail to connect, it
			// has already passed all proof-of-work and validity tests which means
			// it would be prohibitively expensive for an attacker to fill up the
			// disk with a bunch of blocks that fail to connect.  This is necessary
			// since it allows block download to be decoupled from the much more
			// expensive connection logic.  It also has some other nice properties
			// such as making blocks that never become part of the main chain or
			// blocks that fail to connect available for further analysis.
			dbStoreError = b.db.Update(func(dbTx database.Tx) error {
				return dbTx.StoreBlock(block)
			})
		}()
	} else {
		err = b.db.Update(func(dbTx database.Tx) error {
			return dbStoreBlock(dbTx, block)
		})
		if err != nil {
			return false, err
		}
	}

	// Create a new block node for the block and add it to the node index. Even
	// if the block ultimately gets connected to the main chain, it starts out
	// on a side chain.
	blockHeader := &block.MsgBlock().Header
	newNode := newBlockNode(blockHeader, prevNode)
	newNode.status = statusDataStored

	b.index.AddNode(newNode)
	err = b.index.flushToDB()
	if err != nil {
		return false, err
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
	func() {
		b.chainLock.Unlock()
		defer b.chainLock.Lock()
		b.sendNotification(NTBlockAccepted, block)
	}()

	// Wait until the block is saved.  If there was a db error, then unset
	// the data stored flag and flush the block index.
	wg.Wait()
	if dbStoreError != nil {
		b.index.UnsetStatusFlags(newNode, statusDataStored)
		err = b.index.flushToDB()
		if err != nil {
			return false, fmt.Errorf("%v. %v", err, dbStoreError)
		}
		return false, dbStoreError
	}

	return isMainChain, nil
}
