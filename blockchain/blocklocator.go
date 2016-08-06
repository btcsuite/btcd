// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
)

// BlockLocator is used to help locate a specific block.  The algorithm for
// building the block locator is to add the hashes in reverse order until
// the genesis block is reached.  In order to keep the list of locator hashes
// to a reasonable number of entries, first the most recent previous 10 block
// hashes are added, then the step is doubled each loop iteration to
// exponentially decrease the number of hashes as a function of the distance
// from the block being located.
//
// For example, assume you have a block chain with a side chain as depicted
// below:
// 	genesis -> 1 -> 2 -> ... -> 15 -> 16  -> 17  -> 18
// 	                              \-> 16a -> 17a
//
// The block locator for block 17a would be the hashes of blocks:
// [17a 16a 15 14 13 12 11 10 9 8 6 2 genesis]
type BlockLocator []*chainhash.Hash

// blockLocatorFromHash returns a block locator for the passed block hash.
// See BlockLocator for details on the algotirhm used to create a block locator.
//
// In addition to the general algorithm referenced above, there are a couple of
// special cases which are handled:
//
//  - If the genesis hash is passed, there are no previous hashes to add and
//    therefore the block locator will only consist of the genesis hash
//  - If the passed hash is not currently known, the block locator will only
//    consist of the passed hash
//
// This function MUST be called with the chain state lock held (for reads).
func (b *BlockChain) blockLocatorFromHash(hash *chainhash.Hash) BlockLocator {
	// The locator contains the requested hash at the very least.
	locator := make(BlockLocator, 0, wire.MaxBlockLocatorsPerMsg)
	locator = append(locator, hash)

	// Nothing more to do if a locator for the genesis hash was requested.
	if hash.IsEqual(b.chainParams.GenesisHash) {
		return locator
	}

	// Attempt to find the height of the block that corresponds to the
	// passed hash, and if it's on a side chain, also find the height at
	// which it forks from the main chain.
	blockHeight := int32(-1)
	forkHeight := int32(-1)
	node, exists := b.index[*hash]
	if !exists {
		// Try to look up the height for passed block hash.  Assume an
		// error means it doesn't exist and just return the locator for
		// the block itself.
		var height int32
		err := b.db.View(func(dbTx database.Tx) error {
			var err error
			height, err = dbFetchHeightByHash(dbTx, hash)
			return err
		})
		if err != nil {
			return locator
		}

		blockHeight = height
	} else {
		blockHeight = node.height

		// Find the height at which this node forks from the main chain
		// if the node is on a side chain.
		if !node.inMainChain {
			for n := node; n.parent != nil; n = n.parent {
				if n.inMainChain {
					forkHeight = n.height
					break
				}
			}
		}
	}

	// Generate the block locators according to the algorithm described in
	// in the BlockLocator comment and make sure to leave room for the final
	// genesis hash.
	//
	// The error is intentionally ignored here since the only way the code
	// could fail is if there is something wrong with the database which
	// will be caught in short order anyways and it's also safe to ignore
	// block locators.
	_ = b.db.View(func(dbTx database.Tx) error {
		iterNode := node
		increment := int32(1)
		for len(locator) < wire.MaxBlockLocatorsPerMsg-1 {
			// Once there are 10 locators, exponentially increase
			// the distance between each block locator.
			if len(locator) > 10 {
				increment *= 2
			}
			blockHeight -= increment
			if blockHeight < 1 {
				break
			}

			// As long as this is still on the side chain, walk
			// backwards along the side chain nodes to each block
			// height.
			if forkHeight != -1 && blockHeight > forkHeight {
				// Intentionally use parent field instead of the
				// getPrevNodeFromNode function since we don't
				// want to dynamically load nodes when building
				// block locators.  Side chain blocks should
				// always be in memory already, and if they
				// aren't for some reason it's ok to skip them.
				for iterNode != nil && blockHeight > iterNode.height {
					iterNode = iterNode.parent
				}
				if iterNode != nil && iterNode.height == blockHeight {
					locator = append(locator, iterNode.hash)
				}
				continue
			}

			// The desired block height is in the main chain, so
			// look it up from the main chain database.
			h, err := dbFetchHashByHeight(dbTx, blockHeight)
			if err != nil {
				// This shouldn't happen and it's ok to ignore
				// block locators, so just continue to the next
				// one.
				log.Warnf("Lookup of known valid height failed %v",
					blockHeight)
				continue
			}
			locator = append(locator, h)
		}

		return nil
	})

	// Append the appropriate genesis block.
	locator = append(locator, b.chainParams.GenesisHash)
	return locator
}

// BlockLocatorFromHash returns a block locator for the passed block hash.
// See BlockLocator for details on the algorithm used to create a block locator.
//
// In addition to the general algorithm referenced above, there are a couple of
// special cases which are handled:
//
//  - If the genesis hash is passed, there are no previous hashes to add and
//    therefore the block locator will only consist of the genesis hash
//  - If the passed hash is not currently known, the block locator will only
//    consist of the passed hash
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockLocatorFromHash(hash *chainhash.Hash) BlockLocator {
	b.chainLock.RLock()
	locator := b.blockLocatorFromHash(hash)
	b.chainLock.RUnlock()
	return locator
}

// LatestBlockLocator returns a block locator for the latest known tip of the
// main (best) chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) LatestBlockLocator() (BlockLocator, error) {
	b.chainLock.RLock()
	locator := b.blockLocatorFromHash(b.bestNode.hash)
	b.chainLock.RUnlock()
	return locator, nil
}
