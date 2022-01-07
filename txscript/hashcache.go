// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/binary"
	"math"
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

// PrevOutputFetcher is an interface used to supply the sighash cache with the
// previous output information needed to calculate the pre-computed sighash
// midstate for taproot transactions.
type PrevOutputFetcher interface {
	// FetchPrevOutput attempts to fetch the previous output referenced by
	// the passed outpoint. A nil value will be returned if the passed
	// outpoint doesn't exist.
	FetchPrevOutput(wire.OutPoint) *wire.TxOut
}

// CannedPrevOutputFetcher is an implementation of PrevOutputFetcher that only
// is able to return information for a single previous output.
type CannedPrevOutputFetcher struct {
	pkScript []byte
	amt      int64
}

// NewCannedPrevOutputFetcher returns an instance of a CannedPrevOutputFetcher
// that can only return the TxOut defined by the passed script and amount.
func NewCannedPrevOutputFetcher(script []byte, amt int64) *CannedPrevOutputFetcher {
	return &CannedPrevOutputFetcher{
		pkScript: script,
		amt:      amt,
	}
}

// FetchPrevOutput attempts to fetch the previous output referenced by the
// passed outpoint.
//
// NOTE: This is a part of the PrevOutputFetcher interface.
func (c *CannedPrevOutputFetcher) FetchPrevOutput(wire.OutPoint) *wire.TxOut {
	return &wire.TxOut{
		PkScript: c.pkScript,
		Value:    c.amt,
	}
}

// A compile-time assertion to ensure that CannedPrevOutputFetcher matches the
// PrevOutputFetcher interface.
var _ PrevOutputFetcher = (*CannedPrevOutputFetcher)(nil)

// MultiPrevOutFetcher is a custom implementation of the PrevOutputFetcher
// backed by a key-value map of prevouts to outputs.
type MultiPrevOutFetcher struct {
	prevOuts map[wire.OutPoint]*wire.TxOut
}

// NewMultiPrevOutFetcher returns an instance of a PrevOutputFetcher that's
// backed by an optional map which is used as an input source. The
func NewMultiPrevOutFetcher(prevOuts map[wire.OutPoint]*wire.TxOut) *MultiPrevOutFetcher {
	if prevOuts == nil {
		prevOuts = make(map[wire.OutPoint]*wire.TxOut)
	}

	return &MultiPrevOutFetcher{
		prevOuts: prevOuts,
	}
}

// FetchPrevOutput attempts to fetch the previous output referenced by the
// passed outpoint.
//
// NOTE: This is a part of the CannedPrevOutputFetcher interface.
func (m *MultiPrevOutFetcher) FetchPrevOutput(op wire.OutPoint) *wire.TxOut {
	return m.prevOuts[op]
}

// AddPrevOut adds a new prev out, tx out pair to the backing map.
func (m *MultiPrevOutFetcher) AddPrevOut(op wire.OutPoint, txOut *wire.TxOut) {
	m.prevOuts[op] = txOut
}

// Merge merges two instances of a MultiPrevOutFetcher into a single source.
func (m *MultiPrevOutFetcher) Merge(other *MultiPrevOutFetcher) {
	for k, v := range other.prevOuts {
		m.prevOuts[k] = v
	}
}

// A compile-time assertion to ensure that MultiPrevOutFetcher matches the
// PrevOutputFetcher interface.
var _ PrevOutputFetcher = (*MultiPrevOutFetcher)(nil)

// calcHashInputAmounts computes a hash digest of the input amounts of all
// inputs referenced in the passed transaction. This hash pre computation is only
// used for validating taproot inputs.
func calcHashInputAmounts(tx *wire.MsgTx, inputFetcher PrevOutputFetcher) chainhash.Hash {
	var b bytes.Buffer
	for _, txIn := range tx.TxIn {
		prevOut := inputFetcher.FetchPrevOutput(txIn.PreviousOutPoint)

		_ = binary.Write(&b, binary.LittleEndian, prevOut.Value)
	}

	return chainhash.HashH(b.Bytes())
}

// calcHashInputAmts computes the hash digest of all the previous input scripts
// referenced by the passed transaction. This hash pre computation is only used
// for validating taproot inputs.
func calcHashInputScripts(tx *wire.MsgTx, inputFetcher PrevOutputFetcher) chainhash.Hash {
	var b bytes.Buffer
	for _, txIn := range tx.TxIn {
		prevOut := inputFetcher.FetchPrevOutput(txIn.PreviousOutPoint)

		_ = wire.WriteVarBytes(&b, 0, prevOut.PkScript)
	}

	return chainhash.HashH(b.Bytes())
}

// SegwitSigHashMidstate is the sighash midstate used in the base segwit
// sighash calculation as defined in BIP 143.
type SegwitSigHashMidstate struct {
	HashPrevOutsV0 chainhash.Hash
	HashSequenceV0 chainhash.Hash
	HashOutputsV0  chainhash.Hash
}

// TaprootSigHashMidState is the sighash midstate used to compute taproot and
// tapscript signatures as defined in BIP 341.
type TaprootSigHashMidState struct {
	HashPrevOutsV1     chainhash.Hash
	HashSequenceV1     chainhash.Hash
	HashOutputsV1      chainhash.Hash
	HashInputScriptsV1 chainhash.Hash
	HashInputAmountsV1 chainhash.Hash
}

// TxSigHashes houses the partial set of sighashes introduced within BIP0143.
// This partial set of sighashes may be re-used within each input across a
// transaction when validating all inputs. As a result, validation complexity
// for SigHashAll can be reduced by a polynomial factor.
type TxSigHashes struct {
	SegwitSigHashMidstate

	TaprootSigHashMidState
}

// NewTxSigHashes computes, and returns the cached sighashes of the given
// transaction.
func NewTxSigHashes(tx *wire.MsgTx,
	inputFetcher PrevOutputFetcher) *TxSigHashes {

	var (
		sigHashes TxSigHashes
		zeroHash  chainhash.Hash
	)

	// Base segwit (witness version v0), and taproot (witness version v1)
	// differ in how the set of pre-computed cached sighash midstate is
	// computed. For taproot, the prevouts, sequence, and outputs are
	// computed as normal, but a single sha256 hash invocation is used. In
	// addition, the hashes of all the previous input amounts and scripts
	// are included as well.
	//
	// Based on the above distinction, we'll run through all the referenced
	// inputs to determine what we need to compute.
	var hasV0Inputs, hasV1Inputs bool
	for _, txIn := range tx.TxIn {
		// If this is a coinbase input, then we know that we only need
		// the v0 midstate (though it won't be used) in this instance.
		outpoint := txIn.PreviousOutPoint
		if outpoint.Index == math.MaxUint32 && outpoint.Hash == zeroHash {
			hasV0Inputs = true
			continue
		}

		prevOut := inputFetcher.FetchPrevOutput(outpoint)

		// If this is spending a script that looks like a taproot output,
		// then we'll need to pre-compute the extra taproot data.
		if IsPayToTaproot(prevOut.PkScript) {
			hasV1Inputs = true
		} else {
			// Otherwise, we'll assume we need the v0 sighash midstate.
			hasV0Inputs = true
		}

		// If the transaction has _both_ v0 and v1 inputs, then we can stop
		// here.
		if hasV0Inputs && hasV1Inputs {
			break
		}
	}

	// Now that we know which cached midstate we need to calculate, we can
	// go ahead and do so.
	//
	// First, we can calculate the information that both segwit v0 and v1
	// need: the prevout, sequence and output hashes. For v1 the only
	// difference is that this is a single instead of a double hash.
	//
	// Both v0 and v1 share this base data computed using a sha256 single
	// hash.
	sigHashes.HashPrevOutsV1 = calcHashPrevOuts(tx)
	sigHashes.HashSequenceV1 = calcHashSequence(tx)
	sigHashes.HashOutputsV1 = calcHashOutputs(tx)

	// The v0 data is the same as the v1 (newer data) but it uses a double
	// hash instead.
	if hasV0Inputs {
		sigHashes.HashPrevOutsV0 = chainhash.HashH(
			sigHashes.HashPrevOutsV1[:],
		)
		sigHashes.HashSequenceV0 = chainhash.HashH(
			sigHashes.HashSequenceV1[:],
		)
		sigHashes.HashOutputsV0 = chainhash.HashH(
			sigHashes.HashOutputsV1[:],
		)
	}

	// Finally, we'll compute the taproot specific data if needed.
	if hasV1Inputs {
		sigHashes.HashInputAmountsV1 = calcHashInputAmounts(
			tx, inputFetcher,
		)
		sigHashes.HashInputScriptsV1 = calcHashInputScripts(
			tx, inputFetcher,
		)
	}

	return &sigHashes
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
func (h *HashCache) AddSigHashes(tx *wire.MsgTx,
	inputFetcher PrevOutputFetcher) {

	h.Lock()
	h.sigHashes[tx.TxHash()] = NewTxSigHashes(tx, inputFetcher)
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
