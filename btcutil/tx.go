// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil

import (
	"bytes"
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// TxIndexUnknown is the value returned for a transaction index that is unknown.
// This is typically because the transaction has not been inserted into a block
// yet.
const TxIndexUnknown = -1

// Tx defines a bitcoin transaction that provides easier and more efficient
// manipulation of raw transactions.  It also memoizes the hash for the
// transaction on its first access so subsequent accesses don't have to repeat
// the relatively expensive hashing operations.
type Tx struct {
	msgTx         *wire.MsgTx     // Underlying MsgTx
	txHash        *chainhash.Hash // Cached transaction hash
	txHashWitness *chainhash.Hash // Cached transaction witness hash
	txHasWitness  *bool           // If the transaction has witness data
	txIndex       int             // Position within a block or TxIndexUnknown
	rawBytes      []byte          // Raw bytes for the tx in the raw block.
}

// MsgTx returns the underlying wire.MsgTx for the transaction.
func (t *Tx) MsgTx() *wire.MsgTx {
	// Return the cached transaction.
	return t.msgTx
}

// Hash returns the hash of the transaction.  This is equivalent to calling
// TxHash on the underlying wire.MsgTx, however it caches the result so
// subsequent calls are more efficient.  If the Tx has the raw bytes of the tx
// cached, it will use that and skip serialization.
func (t *Tx) Hash() *chainhash.Hash {
	// Return the cached hash if it has already been generated.
	if t.txHash != nil {
		return t.txHash
	}

	// If the rawBytes aren't available, call msgtx.TxHash.
	if t.rawBytes == nil {
		hash := t.msgTx.TxHash()
		t.txHash = &hash
		return &hash
	}

	// If we have the raw bytes, then don't call msgTx.TxHash as that has
	// the overhead of serialization. Instead, we can take the existing
	// serialized bytes and hash them to speed things up.
	var hash chainhash.Hash
	if t.HasWitness() {
		// If the raw bytes contain the witness, we must strip it out
		// before calculating the hash.
		baseSize := t.msgTx.SerializeSizeStripped()
		nonWitnessBytes := make([]byte, 0, baseSize)

		// Append the version bytes.
		offset := 4
		nonWitnessBytes = append(
			nonWitnessBytes, t.rawBytes[:offset]...,
		)

		// Append the input and output bytes.  -8 to account for the
		// version bytes and the locktime bytes.
		//
		// Skip the 2 bytes for the witness encoding.
		offset += 2
		nonWitnessBytes = append(
			nonWitnessBytes,
			t.rawBytes[offset:offset+baseSize-8]...,
		)

		// Append the last 4 bytes which are the locktime bytes.
		nonWitnessBytes = append(
			nonWitnessBytes, t.rawBytes[len(t.rawBytes)-4:]...,
		)

		// We purposely call doublehashh here instead of doublehashraw
		// as we don't have the serialization overhead and avoiding the
		// 1 alloc is better in this case.
		hash = chainhash.DoubleHashRaw(func(w io.Writer) error {
			_, err := w.Write(nonWitnessBytes)
			return err
		})
	} else {
		// If the raw bytes don't have the witness, we can use it
		// directly.
		//
		// We purposely call doublehashh here instead of doublehashraw
		// as we don't have the serialization overhead and avoiding the
		// 1 alloc is better in this case.
		hash = chainhash.DoubleHashRaw(func(w io.Writer) error {
			_, err := w.Write(t.rawBytes)
			return err
		})
	}

	t.txHash = &hash
	return &hash
}

// WitnessHash returns the witness hash (wtxid) of the transaction.  This is
// equivalent to calling WitnessHash on the underlying wire.MsgTx, however it
// caches the result so subsequent calls are more efficient.  If the Tx has the
// raw bytes of the tx cached, it will use that and skip serialization.
func (t *Tx) WitnessHash() *chainhash.Hash {
	// Return the cached hash if it has already been generated.
	if t.txHashWitness != nil {
		return t.txHashWitness
	}

	// Cache the hash and return it.
	var hash chainhash.Hash
	if len(t.rawBytes) > 0 {
		hash = chainhash.DoubleHashH(t.rawBytes)
	} else {
		hash = t.msgTx.WitnessHash()
	}

	t.txHashWitness = &hash
	return &hash
}

// HasWitness returns false if none of the inputs within the transaction
// contain witness data, true false otherwise. This equivalent to calling
// HasWitness on the underlying wire.MsgTx, however it caches the result so
// subsequent calls are more efficient.
func (t *Tx) HasWitness() bool {
	if t.txHasWitness != nil {
		return *t.txHasWitness
	}

	hasWitness := t.msgTx.HasWitness()
	t.txHasWitness = &hasWitness
	return hasWitness
}

// Index returns the saved index of the transaction within a block.  This value
// will be TxIndexUnknown if it hasn't already explicitly been set.
func (t *Tx) Index() int {
	return t.txIndex
}

// SetIndex sets the index of the transaction in within a block.
func (t *Tx) SetIndex(index int) {
	t.txIndex = index
}

// NewTx returns a new instance of a bitcoin transaction given an underlying
// wire.MsgTx.  See Tx.
func NewTx(msgTx *wire.MsgTx) *Tx {
	return &Tx{
		msgTx:   msgTx,
		txIndex: TxIndexUnknown,
	}
}

// setBytes sets the raw bytes of the tx.
func (t *Tx) setBytes(bytes []byte) {
	t.rawBytes = bytes
}

// NewTxFromBytes returns a new instance of a bitcoin transaction given the
// serialized bytes.  See Tx.
func NewTxFromBytes(serializedTx []byte) (*Tx, error) {
	br := bytes.NewReader(serializedTx)
	return NewTxFromReader(br)
}

// NewTxFromReader returns a new instance of a bitcoin transaction given a
// Reader to deserialize the transaction.  See Tx.
func NewTxFromReader(r io.Reader) (*Tx, error) {
	// Deserialize the bytes into a MsgTx.
	var msgTx wire.MsgTx
	err := msgTx.Deserialize(r)
	if err != nil {
		return nil, err
	}

	t := Tx{
		msgTx:   &msgTx,
		txIndex: TxIndexUnknown,
	}
	return &t, nil
}
