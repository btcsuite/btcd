// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txsort

import (
	"bytes"
	"sort"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// sortableInputSlice implements sort.Interface to allow a slice of transaction
// inputs to be sorted according to the description in the package
// documentation.
type sortableInputSlice []*wire.TxIn

// Len returns the number of transaction inputs in the slice.  It is part of the
// sort.Interface implementation.
func (s sortableInputSlice) Len() int {
	return len(s)
}

// Swap swaps the transaction intputs at the passed indices.  It is part of the
// sort.Interface implementation.
func (s sortableInputSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less returns whether the transaction input with index i should sort before
// the transaction input with index j.  It is part of the sort.Interface
// implementation.
//
// First sort based on input tree, then input hash (reversed / big-endian), then
// index.
func (s sortableInputSlice) Less(i, j int) bool {
	// Sort by tree first.
	if s[i].PreviousOutPoint.Tree != s[j].PreviousOutPoint.Tree {
		return s[i].PreviousOutPoint.Tree < s[j].PreviousOutPoint.Tree
	}

	// Input hashes are the same, so compare the index.
	ihash := s[i].PreviousOutPoint.Hash
	jhash := s[j].PreviousOutPoint.Hash
	if ihash == jhash {
		return s[i].PreviousOutPoint.Index < s[j].PreviousOutPoint.Index
	}

	// At this point, the hashes are not equal, so reverse them to
	// big endian and return the result of the comparison.
	const hashSize = chainhash.HashSize
	for b := 0; b < hashSize/2; b++ {
		ihash[b], ihash[hashSize-1-b] = ihash[hashSize-1-b], ihash[b]
		jhash[b], jhash[hashSize-1-b] = jhash[hashSize-1-b], jhash[b]
	}
	return bytes.Compare(ihash[:], jhash[:]) == -1
}

// sortableOutputSlice implements sort.Interface to allow a slice of transaction
// outputs to be sorted according to the description in the package
// documentation.
type sortableOutputSlice []*wire.TxOut

// Len returns the number of transaction outputs in the slice.  It is part of
// the sort.Interface implementation.
func (s sortableOutputSlice) Len() int {
	return len(s)
}

// Swap swaps the transaction outputs at the passed indices.  It is part of the
// sort.Interface implementation.
func (s sortableOutputSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less returns whether the transaction output with index i should sort before
// the transaction output with index j.  It is part of the sort.Interface
// implementation.
//
// First sort based on amount (smallest first), then by script version, then by
// script.
func (s sortableOutputSlice) Less(i, j int) bool {
	// Sort by amount first.
	if s[i].Value != s[j].Value {
		return s[i].Value < s[j].Value
	}

	// At this point, the amounts are the same, so sort by the script version.
	if s[i].Version != s[j].Version {
		return s[i].Version < s[j].Version
	}

	// At this point, the script versions are the same, so sort by the script.
	return bytes.Compare(s[i].PkScript, s[j].PkScript) < 0
}

// InPlaceSort modifies the passed transaction inputs and outputs to be sorted
// according to the description in the package documentation.
//
// WARNING: This function must NOT be called with published transactions since
// it will mutate the transaction if it's not already sorted.  This can cause
// issues if you mutate a tx in a block, for example, which would invalidate the
// block.  It could also cause cached hashes, such as in a dcrutil.Tx to become
// invalidated.
//
// The function should only be used if the caller is creating the transaction or
// is otherwise 100% positive mutating will not cause adverse affects due to
// other dependencies.
func InPlaceSort(tx *wire.MsgTx) {
	sort.Sort(sortableInputSlice(tx.TxIn))
	sort.Sort(sortableOutputSlice(tx.TxOut))
}

// Sort returns a new transaction with the inputs and outputs sorted according
// to the description in the package documentation.  The passed transaction is
// not modified and the new transaction might have a different hash if any
// sorting was done.
func Sort(tx *wire.MsgTx) *wire.MsgTx {
	txCopy := tx.Copy()
	sort.Sort(sortableInputSlice(txCopy.TxIn))
	sort.Sort(sortableOutputSlice(txCopy.TxOut))
	return txCopy
}

// IsSorted checks whether tx has inputs and outputs sorted according to the
// description in the package documentation.
func IsSorted(tx *wire.MsgTx) bool {
	if !sort.IsSorted(sortableInputSlice(tx.TxIn)) {
		return false
	}
	if !sort.IsSorted(sortableOutputSlice(tx.TxOut)) {
		return false
	}
	return true
}
