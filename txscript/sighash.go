// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/binary"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// SigHashType represents hash type bits at the end of a signature.
type SigHashType byte

// Hash type bits from the end of a signature.
const (
	SigHashOld          SigHashType = 0x0
	SigHashAll          SigHashType = 0x1
	SigHashNone         SigHashType = 0x2
	SigHashSingle       SigHashType = 0x3
	SigHashAllValue     SigHashType = 0x4
	SigHashAnyOneCanPay SigHashType = 0x80

	// sigHashMask defines the number of bits of the hash type which is used
	// to identify which outputs are signed.
	sigHashMask = 0x1f
)

// calcSignatureHash will, given a script and hash type for the current script
// engine instance, calculate the signature hash to be used for signing and
// verification.
func calcSignatureHash(script []parsedOpcode, hashType SigHashType,
	tx *wire.MsgTx, idx int, cachedPrefix *chainhash.Hash) ([]byte, error) {
	// The SigHashSingle signature type signs only the corresponding input
	// and output (the output with the same index number as the input).
	//
	// Since transactions can have more inputs than outputs, this means it
	// is improper to use SigHashSingle on input indices that don't have a
	// corresponding output.
	//
	// A bug in the original Satoshi client implementation means specifying
	// an index that is out of range results in a signature hash of 1 (as a
	// uint256 little endian).  The original intent appeared to be to
	// indicate failure, but unfortunately, it was never checked and thus is
	// treated as the actual signature hash.  This buggy behavior is now
	// part of the consensus and a hard fork would be required to fix it.
	//
	// Due to this, care must be taken by software that creates transactions
	// which make use of SigHashSingle because it can lead to an extremely
	// dangerous situation where the invalid inputs will end up signing a
	// hash of 1.  This in turn presents an opportunity for attackers to
	// cleverly construct transactions which can steal those coins provided
	// they can reuse signatures.
	//
	// Decred mitigates this by actually returning an error instead.
	if hashType&sigHashMask == SigHashSingle && idx >= len(tx.TxOut) {
		return nil, ErrSighashSingleIdx
	}

	// Remove all instances of OP_CODESEPARATOR from the script.
	script = removeOpcode(script, OP_CODESEPARATOR)

	// Make a deep copy of the transaction, zeroing out the script for all
	// inputs that are not currently being processed.
	txCopy := tx.Copy()
	for i := range txCopy.TxIn {
		if i == idx {
			// UnparseScript cannot fail here because removeOpcode
			// above only returns a valid script.
			sigScript, _ := unparseScript(script)
			txCopy.TxIn[idx].SignatureScript = sigScript
		} else {
			txCopy.TxIn[i].SignatureScript = nil
		}
	}

	switch hashType & sigHashMask {
	case SigHashNone:
		txCopy.TxOut = txCopy.TxOut[0:0] // Empty slice.
		for i := range txCopy.TxIn {
			if i != idx {
				txCopy.TxIn[i].Sequence = 0
			}
		}

	case SigHashSingle:
		// Resize output array to up to and including requested index.
		txCopy.TxOut = txCopy.TxOut[:idx+1]

		// All but current output get zeroed out.
		for i := 0; i < idx; i++ {
			txCopy.TxOut[i].Value = -1
			txCopy.TxOut[i].PkScript = nil
		}

		// Sequence on all other inputs is 0, too.
		for i := range txCopy.TxIn {
			if i != idx {
				txCopy.TxIn[i].Sequence = 0
			}
		}

	default:
		// Consensus treats undefined hashtypes like normal SigHashAll
		// for purposes of hash generation.
		fallthrough
	case SigHashOld:
		fallthrough
	case SigHashAllValue:
		fallthrough
	case SigHashAll:
		// Nothing special here.
	}
	if hashType&SigHashAnyOneCanPay != 0 {
		txCopy.TxIn = txCopy.TxIn[idx : idx+1]

		// Future code that may need the index
		// should consider resetting it.
		// idx = 0
	}

	// The final hash (message to sign) is the hash of:
	// 1) the hash type (encoded as a 4-byte little-endian value)
	// 2) hash of the prefix ||
	// 3) hash of the witness for signing ||
	var wbuf bytes.Buffer
	wbuf.Grow(chainhash.HashSize*2 + 4)
	binary.Write(&wbuf, binary.LittleEndian, uint32(hashType))

	// Optimization for SIGHASH_ALL. In this case, the prefix hash is
	// the same as the transaction hash because only the inputs have
	// been modified, so don't bother to do the wasteful O(N^2) extra
	// hash here.
	// The caching only works if the "anyone can pay flag" is also
	// disabled.
	var prefixHash chainhash.Hash
	if cachedPrefix != nil &&
		(hashType&sigHashMask == SigHashAll) &&
		(hashType&SigHashAnyOneCanPay == 0) &&
		chaincfg.SigHashOptimization {
		prefixHash = *cachedPrefix
	} else {
		prefixHash = txCopy.TxHash()
	}

	// If the ValueIn is to be included in what we're signing, sign
	// the witness hash that includes it. Otherwise, just sign the
	// prefix and signature scripts.
	var witnessHash chainhash.Hash
	if hashType&sigHashMask != SigHashAllValue {
		witnessHash = txCopy.TxHashWitnessSigning()
	} else {
		witnessHash = txCopy.TxHashWitnessValueSigning()
	}
	wbuf.Write(prefixHash[:])
	wbuf.Write(witnessHash[:])
	return chainhash.HashB(wbuf.Bytes()), nil
}

// CalcSignatureHash computes the signature hash for the specified input of
// the target transaction observing the desired signature hash type.  The
// cached prefix parameter allows the caller to optimize the calculation by
// providing the prefix hash to be reused in the case of SigHashAll without the
// SigHashAnyOneCanPay flag set.
func CalcSignatureHash(script []byte, hashType SigHashType, tx *wire.MsgTx, idx int, cachedPrefix *chainhash.Hash) ([]byte, error) {
	pops, err := parseScript(script)
	if err != nil {
		return nil, err
	}

	return calcSignatureHash(pops, hashType, tx, idx, cachedPrefix)
}
