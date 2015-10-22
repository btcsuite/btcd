// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil

import (
	"bytes"
	"sort"

	"github.com/btcsuite/btcd/wire"
)

// TxSort
// Provides functions for sorting tx inputs and outputs according to BIP LI01
// (https://github.com/kristovatlas/rfc/blob/master/bips/bip-li01.mediawiki)

// TxSort sorts the inputs and outputs of a tx based on BIP LI01
// It does not modify the transaction given, but returns a new copy
// which has been sorted and may have a different txid.
func TxSort(tx *wire.MsgTx) *wire.MsgTx {
	txCopy := tx.Copy()
	sort.Sort(sortableInputSlice(txCopy.TxIn))
	sort.Sort(sortableOutputSlice(txCopy.TxOut))
	return txCopy
}

// TxIsSorted checks whether tx has inputs and outputs sorted according
// to BIP LI01.
func TxIsSorted(tx *wire.MsgTx) bool {
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

// for SortableInputSlice and SortableOutputSlice, three functions are needed
// to make it sortable with sort.Sort() -- Len, Less, and Swap
// Len and Swap are trivial.  Less is BIP LI01 specific.
func (ins sortableInputSlice) Len() int {
	return len(ins)
}
func (outs sortableOutputSlice) Len() int {
	return len(outs)
}

func (ins sortableInputSlice) Swap(i, j int) {
	ins[i], ins[j] = ins[j], ins[i]
}
func (outs sortableOutputSlice) Swap(i, j int) {
	outs[i], outs[j] = outs[j], outs[i]
}

// Input comparison function.
// First sort based on input txid (reversed / rpc-style), then index
func (ins sortableInputSlice) Less(i, j int) bool {
	ihash := ins[i].PreviousOutPoint.Hash
	jhash := ins[j].PreviousOutPoint.Hash
	for b := 0; b < wire.HashSize/2; b++ {
		ihash[b], ihash[wire.HashSize-1-b] = ihash[wire.HashSize-1-b], ihash[b]
		jhash[b], jhash[wire.HashSize-1-b] = jhash[wire.HashSize-1-b], jhash[b]
	}
	c := bytes.Compare(ihash[:], jhash[:])
	if c == 0 {
		// input txids are the same, compare index
		return ins[i].PreviousOutPoint.Index < ins[j].PreviousOutPoint.Index
	}
	return c == -1.
}

// Output comparison function.
// First sort based on amount (smallest first), then PkScript
func (outs sortableOutputSlice) Less(i, j int) bool {
	if outs[i].Value == outs[j].Value {
		return bytes.Compare(outs[i].PkScript, outs[j].PkScript) < 0
	}
	return outs[i].Value < outs[j].Value
}
