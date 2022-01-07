// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/binary"
	"sync"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// calcHashPrevOuts calculates a single hash of all the previous outputs
// (txid:index) referenced within the passed transaction. This calculated hash
// can be re-used when validating all inputs spending segwit outputs, with a
// signature hash type of SigHashAll. This allows validation to re-use previous
// hashing computation, reducing the complexity of validating SigHashAll inputs
// from  O(N^2) to O(N).
func calcHashPrevOuts(tx *wire.MsgTx) chainhash.Hash {
	var b bytes.Buffer
	for _, in := range tx.TxIn {
		// First write out the 32-byte transaction ID one of whose
		// outputs are being referenced by this input.
		b.Write(in.PreviousOutPoint.Hash[:])

		// Next, we'll encode the index of the referenced output as a
		// little endian integer.
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], in.PreviousOutPoint.Index)
		b.Write(buf[:])
	}

	return chainhash.HashH(b.Bytes())
}

// calcHashSequence computes an aggregated hash of each of the sequence numbers
// within the inputs of the passed transaction. This single hash can be re-used
// when validating all inputs spending segwit outputs, which include signatures
// using the SigHashAll sighash type. This allows validation to re-use previous
// hashing computation, reducing the complexity of validating SigHashAll inputs
// from O(N^2) to O(N).
func calcHashSequence(tx *wire.MsgTx) chainhash.Hash {
	var b bytes.Buffer
	for _, in := range tx.TxIn {
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], in.Sequence)
		b.Write(buf[:])
	}

	return chainhash.HashH(b.Bytes())
}

// calcHashOutputs computes a hash digest of all outputs created by the
// transaction encoded using the wire format. This single hash can be re-used
// when validating all inputs spending witness programs, which include
// signatures using the SigHashAll sighash type. This allows computation to be
// cached, reducing the total hashing complexity from O(N^2) to O(N).
func calcHashOutputs(tx *wire.MsgTx) chainhash.Hash {
	var b bytes.Buffer
	for _, out := range tx.TxOut {
		wire.WriteTxOut(&b, 0, 0, out)
	}

	return chainhash.HashH(b.Bytes())
}

// TxSigHashes houses the partial set of sighashes introduced within BIP0143.
// This partial set of sighashes may be re-used within each input across a
// transaction when validating all inputs. As a result, validation complexity
// for SigHashAll can be reduced by a polynomial factor.
type TxSigHashes struct {
	HashPrevOuts chainhash.Hash
	HashSequence chainhash.Hash
	HashOutputs  chainhash.Hash
}

// NewTxSigHashes computes, and returns the cached sighashes of the given
// transaction.
func NewTxSigHashes(tx *wire.MsgTx) *TxSigHashes {
	return &TxSigHashes{
		HashPrevOuts: calcHashPrevOuts(tx),
		HashSequence: calcHashSequence(tx),
		HashOutputs:  calcHashOutputs(tx),
	}
}

// HashCache houses a set of partial sighashes keyed by txid. The set of partial
// sighashes are those introduced within BIP0143 by the new more efficient
// sighash digest calculation algorithm. Using this threadsafe shared cache,
// multiple goroutines can safely re-use the pre-computed partial sighashes
// speeding up validation time amongst all inputs found within a block.
type HashCache struct {
	sigHashes map[chainhash.Hash]*TxSigHashes

	sync.RWMutex
}

// NewHashCache returns a new instance of the HashCache given a maximum number
// of entries which may exist within it at anytime.
func NewHashCache(maxSize uint) *HashCache {
	return &HashCache{
		sigHashes: make(map[chainhash.Hash]*TxSigHashes, maxSize),
	}
}

// AddSigHashes computes, then adds the partial sighashes for the passed
// transaction.
func (h *HashCache) AddSigHashes(tx *wire.MsgTx) {
	h.Lock()
	h.sigHashes[tx.TxHash()] = NewTxSigHashes(tx)
	h.Unlock()
}

// ContainsHashes returns true if the partial sighashes for the passed
// transaction currently exist within the HashCache, and false otherwise.
func (h *HashCache) ContainsHashes(txid *chainhash.Hash) bool {
	h.RLock()
	_, found := h.sigHashes[*txid]
	h.RUnlock()

	return found
}

// GetSigHashes possibly returns the previously cached partial sighashes for
// the passed transaction. This function also returns an additional boolean
// value indicating if the sighashes for the passed transaction were found to
// be present within the HashCache.
func (h *HashCache) GetSigHashes(txid *chainhash.Hash) (*TxSigHashes, bool) {
	h.RLock()
	item, found := h.sigHashes[*txid]
	h.RUnlock()

	return item, found
}

// PurgeSigHashes removes all partial sighashes from the HashCache belonging to
// the passed transaction.
func (h *HashCache) PurgeSigHashes(txid *chainhash.Hash) {
	h.Lock()
	delete(h.sigHashes, *txid)
	h.Unlock()
}
