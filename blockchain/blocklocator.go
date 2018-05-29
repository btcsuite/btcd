// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"github.com/decred/dcrd/chaincfg/chainhash"
)

// log2FloorMasks defines the masks to use when quickly calculating
// floor(log2(x)) in a constant log2(32) = 5 steps, where x is a uint32, using
// shifts.  They are derived from (2^(2^x) - 1) * (2^(2^x)), for x in 4..0.
var log2FloorMasks = []uint32{0xffff0000, 0xff00, 0xf0, 0xc, 0x2}

// fastLog2Floor calculates and returns floor(log2(x)) in a constant 5 steps.
func fastLog2Floor(n uint32) uint8 {
	rv := uint8(0)
	exponent := uint8(16)
	for i := 0; i < 5; i++ {
		if n&log2FloorMasks[i] != 0 {
			rv += exponent
			n >>= exponent
		}
		exponent >>= 1
	}
	return rv
}

// BlockLocator is used to help locate a specific block.  The algorithm for
// building the block locator is to add the hashes in reverse order until
// the genesis block is reached.  In order to keep the list of locator hashes
// to a reasonable number of entries, first the most recent 12 block hashes are
// added, then the step is doubled each loop iteration to exponentially decrease
// the number of hashes as a function of the distance from the block being
// located.
//
// For example, assume you have a block chain with a side chain as depicted
// below:
// 	genesis -> 1 -> 2 -> ... -> 15 -> 16  -> 17  -> 18
// 	                              \-> 16a -> 17a
//
// The block locator for block 17a would be the hashes of blocks:
// [17a 16a 15 14 13 12 11 10 9 8 7 6 4 genesis]
type BlockLocator []*chainhash.Hash

// blockLocator returns a block locator for the passed block node.
//
// See BlockLocator for details on the algorithm used to create a block locator.
//
// This function MUST be called with the block index lock held (for reads).
func blockLocator(node *blockNode) BlockLocator {
	if node == nil {
		return nil
	}

	// Calculate the max number of entries that will ultimately be in the
	// block locator.  See the description of the algorithm for how these
	// numbers are derived.
	var maxEntries uint8
	if node.height <= 12 {
		maxEntries = uint8(node.height) + 1
	} else {
		// Requested hash itself + previous 10 entries + genesis block.
		// Then floor(log2(height-10)) entries for the skip portion.
		adjustedHeight := uint32(node.height) - 10
		maxEntries = 12 + fastLog2Floor(adjustedHeight)
	}
	locator := make(BlockLocator, 0, maxEntries)

	step := int64(1)
	for node != nil {
		locator = append(locator, &node.hash)

		// Nothing more to add once the genesis block has been added.
		if node.height == 0 {
			break
		}

		// Calculate height of previous node to include ensuring the
		// final node is the genesis block.
		height := node.height - step
		if height < 0 {
			height = 0
		}

		// Walk backwards through the nodes to the correct ancestor.
		node = node.Ancestor(height)

		// Once 11 entries have been included, start doubling the
		// distance between included hashes.
		if len(locator) > 10 {
			step *= 2
		}
	}

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
//  - If the passed hash is not currently known, the block locator will be for
//    the latest known tip of the main (best) chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockLocatorFromHash(hash *chainhash.Hash) BlockLocator {
	b.chainLock.RLock()
	b.index.RLock()
	node, exists := b.index.index[*hash]
	if !exists {
		node = b.bestNode
	}
	locator := blockLocator(node)
	b.index.RUnlock()
	b.chainLock.RUnlock()
	return locator
}

// LatestBlockLocator returns a block locator for the latest known tip of the
// main (best) chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) LatestBlockLocator() (BlockLocator, error) {
	b.chainLock.RLock()
	b.index.RLock()
	locator := blockLocator(b.bestNode)
	b.index.RUnlock()
	b.chainLock.RUnlock()
	return locator, nil
}
