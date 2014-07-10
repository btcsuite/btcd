// Copyright (c) 2013, 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bloom

import (
	"github.com/conformal/btcchain"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// merkleBlock is used to house intermediate information needed to generate a
// btcwire.MsgMerkleBlock according to a filter.
type merkleBlock struct {
	numTx       uint32
	allHashes   []*btcwire.ShaHash
	finalHashes []*btcwire.ShaHash
	matchedBits []byte
	bits        []byte
}

// calcTreeWidth calculates and returns the the number of nodes (width) or a
// merkle tree at the given depth-first height.
func (m *merkleBlock) calcTreeWidth(height uint32) uint32 {
	return (m.numTx + (1 << height) - 1) >> height
}

// calcHash returns the hash for a sub-tree given a depth-first height and
// node position.
func (m *merkleBlock) calcHash(height, pos uint32) *btcwire.ShaHash {
	if height == 0 {
		return m.allHashes[pos]
	}

	var right *btcwire.ShaHash
	left := m.calcHash(height-1, pos*2)
	if pos*2+1 < m.calcTreeWidth(height-1) {
		right = m.calcHash(height-1, pos*2+1)
	} else {
		right = left
	}
	return btcchain.HashMerkleBranches(left, right)
}

// traverseAndBuild builds a partial merkle tree using a recursive depth-first
// approach.  As it calculates the hashes, it also saves whether or not each
// node is a parent node and a list of final hashes to be included in the
// merkle block.
func (m *merkleBlock) traverseAndBuild(height, pos uint32) {
	// Determine whether this node is a parent of a matched node.
	var isParent byte
	for i := pos << height; i < (pos+1)<<height && i < m.numTx; i++ {
		isParent |= m.matchedBits[i]
	}
	m.bits = append(m.bits, isParent)

	// When the node is a leaf node or not a parent of a matched node,
	// append the hash to the list that will be part of the final merkle
	// block.
	if height == 0 || isParent == 0x00 {
		m.finalHashes = append(m.finalHashes, m.calcHash(height, pos))
		return
	}

	// At this point, the node is an internal node and it is the parent of
	// of an included leaf node.

	// Descend into the left child and process its sub-tree.
	m.traverseAndBuild(height-1, pos*2)

	// Descend into the right child and process its sub-tree if
	// there is one.
	if pos*2+1 < m.calcTreeWidth(height-1) {
		m.traverseAndBuild(height-1, pos*2+1)
	}
}

// NewMerkleBlock returns a new *btcwire.MsgMerkleBlock and an array of the matched
// transaction hashes based on the passed block and filter.
func NewMerkleBlock(block *btcutil.Block, filter *Filter) (*btcwire.MsgMerkleBlock, []*btcwire.ShaHash) {
	numTx := uint32(len(block.Transactions()))
	mBlock := merkleBlock{
		numTx:       numTx,
		allHashes:   make([]*btcwire.ShaHash, 0, numTx),
		matchedBits: make([]byte, 0, numTx),
	}

	// Find and keep track of any transactions that match the filter.
	var matchedHashes []*btcwire.ShaHash
	for _, tx := range block.Transactions() {
		if filter.MatchTxAndUpdate(tx) {
			mBlock.matchedBits = append(mBlock.matchedBits, 0x01)
			matchedHashes = append(matchedHashes, tx.Sha())
		} else {
			mBlock.matchedBits = append(mBlock.matchedBits, 0x00)
		}
		mBlock.allHashes = append(mBlock.allHashes, tx.Sha())
	}

	// Calculate the number of merkle branches (height) in the tree.
	height := uint32(0)
	for mBlock.calcTreeWidth(height) > 1 {
		height++
	}

	// Build the depth-first partial merkle tree.
	mBlock.traverseAndBuild(height, 0)

	// Create and return the merkle block.
	msgMerkleBlock := btcwire.MsgMerkleBlock{
		Header:       block.MsgBlock().Header,
		Transactions: uint32(mBlock.numTx),
		Hashes:       make([]*btcwire.ShaHash, 0, len(mBlock.finalHashes)),
		Flags:        make([]byte, (len(mBlock.bits)+7)/8),
	}
	for _, sha := range mBlock.finalHashes {
		msgMerkleBlock.AddTxHash(sha)
	}
	for i := uint32(0); i < uint32(len(mBlock.bits)); i++ {
		msgMerkleBlock.Flags[i/8] |= mBlock.bits[i] << (i % 8)
	}
	return &msgMerkleBlock, matchedHashes
}
