// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Provides functions for sorting tx inputs and outputs according to BIP LI01
// (https://github.com/kristovatlas/rfc/blob/master/bips/bip-li01.mediawiki)

package txsort

import (
	"bytes"
	"sort"

	"github.com/btcsuite/btcd/wire"
)

// Sort returns a new transaction with the inputs and outputs sorted based on
// BIP LI01.  The passed transaction is not modified and the new transaction
// might have a different hash if any sorting was done.
func Sort(tx *wire.MsgTx) *wire.MsgTx {
	txCopy := tx.Copy()
	sort.Sort(sortableInputSlice(txCopy.TxIn))
	sort.Sort(sortableOutputSlice(txCopy.TxOut))
	return txCopy
}

// IsSorted checks whether tx has inputs and outputs sorted according to BIP
// LI01.
func IsSorted(tx *wire.MsgTx) bool {
	if !sort.IsSorted(sortableInputSlice(tx.TxIn)) {
		return false
	}
	if !sort.IsSorted(sortableOutputSlice(tx.TxOut)) {
		return false
	}
	return true
}

type sortableInputSlice []*wire.TxIn
type sortableOutputSlice []*wire.TxOut

// For SortableInputSlice and SortableOutputSlice, three functions are needed
// to make it sortable with sort.Sort() -- Len, Less, and Swap
// Len and Swap are trivial.  Less is BIP LI01 specific.
func (s sortableInputSlice) Len() int       { return len(s) }
func (s sortableOutputSlice) Len() int      { return len(s) }
func (s sortableOutputSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s sortableInputSlice) Swap(i, j int)  { s[i], s[j] = s[j], s[i] }

// Input comparison function.
// First sort based on input hash (reversed / rpc-style), then index.
func (s sortableInputSlice) Less(i, j int) bool {
	// Input hashes are the same, so compare the index.
	ihash := s[i].PreviousOutPoint.Hash
	jhash := s[j].PreviousOutPoint.Hash
	if ihash == jhash {
		return s[i].PreviousOutPoint.Index < s[j].PreviousOutPoint.Index
	}

	// At this point, the hashes are not equal, so reverse them to
	// big-endian and return the result of the comparison.
	const hashSize = wire.HashSize
	for b := 0; b < hashSize/2; b++ {
		ihash[b], ihash[hashSize-1-b] = ihash[hashSize-1-b], ihash[b]
		jhash[b], jhash[hashSize-1-b] = jhash[hashSize-1-b], jhash[b]
	}
	return bytes.Compare(ihash[:], jhash[:]) == -1
}

// Output comparison function.
// First sort based on amount (smallest first), then PkScript.
func (s sortableOutputSlice) Less(i, j int) bool {
	if s[i].Value == s[j].Value {
		return bytes.Compare(s[i].PkScript, s[j].PkScript) < 0
	}
	return s[i].Value < s[j].Value
}
