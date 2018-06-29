// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// SigHashType represents hash type bits at the end of a signature.
type SigHashType byte

// Hash type bits from the end of a signature.
const (
	SigHashAll          SigHashType = 0x1
	SigHashNone         SigHashType = 0x2
	SigHashSingle       SigHashType = 0x3
	SigHashAnyOneCanPay SigHashType = 0x80

	// sigHashMask defines the number of bits of the hash type which is used
	// to identify which outputs are signed.
	sigHashMask = 0x1f
)

// SigHashSerType represents the serialization type used when calculating
// signature hashes.
//
// NOTE: These values were originally a part of transaction serialization which
// is why there is a gap and they are not zero based.  The logic for calculating
// signature hashes has since been decoupled from transaction serialization
// logic, but these specific values are still required by consensus, so they
// must remain unchanged.
type SigHashSerType uint16

const (
	// SigHashSerializePrefix indicates the serialization does not include
	// any witness data.
	SigHashSerializePrefix = 1

	// SigHashSerializeWitness indicates the serialization only contains
	// witness data.
	SigHashSerializeWitness = 3
)

// -----------------------------------------------------------------------------
// A variable length integer (varint) is an encoding for integers up to a max
// value of 2^64-1 that uses a variable number of bytes depending on the value
// being encoded.  It produces fewer bytes for smaller numbers as opposed to a
// fixed-size encoding and is used within the signature hash algorithm to
// specify the number of items or bytes that follow it.
//
// The encoding is as follows:
//
//   Value                   Len   Format
//   -----                   ---   ------
//   < 0xfd                  1     val as uint8
//   <= 0xffff               3     0xfd followed by val as little-endian uint16
//   <= 0xffffffff           5     0xfe followed by val as little-endian uint32
//   <= 0xffffffffffffffff   9     0xff followed by val as little-endian uint64
//
// Example encodings:
//            0 -> [0x00]
//          252 -> [0xfc]                  * Max 1-byte encoded value
//          253 -> [0xfdfd00]              * Min 3-byte encoded value
//          254 -> [0xfdfe00]
//          256 -> [0xfd0001]
//        65535 -> [0xfdffff]              * Max 3-byte encoded value
//        65536 -> [0xfe00000100]          * Min 5-byte encoded value
//       131071 -> [0xfeffff0100]
//   4294967295 -> [0xfeffffffff]          * Max 5-byte encoded value
//   4294967296 -> [0xff0000000001000000]  * Min 9-byte encoded value
//       2^64-1 -> [0xffffffffffffffffff]  * Max allowed value
// -----------------------------------------------------------------------------

// varIntSerializeSize returns the number of bytes it would take to serialize
// the provided value as a variable length integer according to the format
// described above.
func varIntSerializeSize(val uint64) int {
	// The value is small enough to be represented by itself.
	if val < 0xfd {
		return 1
	}

	// Discriminant 1 byte plus 2 bytes for the uint16.
	if val <= math.MaxUint16 {
		return 3
	}

	// Discriminant 1 byte plus 4 bytes for the uint32.
	if val <= math.MaxUint32 {
		return 5
	}

	// Discriminant 1 byte plus 8 bytes for the uint64.
	return 9
}

// putVarInt serializes the provided number to a variable-length integer and
// according to the format described above returns the number of bytes of the
// encoded value.  The result is placed directly into the passed byte slice
// which must be at least large enough to handle the number of bytes returned by
// the varIntSerializeSize function or it will panic.
func putVarInt(buf []byte, val uint64) int {
	if val < 0xfd {
		buf[0] = uint8(val)
		return 1
	}

	if val <= math.MaxUint16 {
		buf[0] = 0xfd
		binary.LittleEndian.PutUint16(buf[1:], uint16(val))
		return 3
	}

	if val <= math.MaxUint32 {
		buf[0] = 0xfe
		binary.LittleEndian.PutUint32(buf[1:], uint32(val))
		return 5
	}

	buf[0] = 0xff
	binary.LittleEndian.PutUint64(buf[1:], val)
	return 9
}

// putByte writes the passed byte to the provided slice and returns 1 to signify
// the number of bytes written.  The target byte slice must be at least large
// enough to handle the write or it will panic.
func putByte(buf []byte, val byte) int {
	buf[0] = val
	return 1
}

// putUint16LE writes the provided uint16 as little endian to the provided slice
// and returns 2 to signify the number of bytes written.  The target byte slice
// must be at least large enough to handle the write or it will panic.
func putUint16LE(buf []byte, val uint16) int {
	binary.LittleEndian.PutUint16(buf, val)
	return 2
}

// putUint32LE writes the provided uint32 as little endian to the provided slice
// and returns 4 to signify the number of bytes written.  The target byte slice
// must be at least large enough to handle the write or it will panic.
func putUint32LE(buf []byte, val uint32) int {
	binary.LittleEndian.PutUint32(buf, val)
	return 4
}

// putUint64LE writes the provided uint64 as little endian to the provided slice
// and returns 8 to signify the number of bytes written.  The target byte slice
// must be at least large enough to handle the write or it will panic.
func putUint64LE(buf []byte, val uint64) int {
	binary.LittleEndian.PutUint64(buf, val)
	return 8
}

// sigHashPrefixSerializeSize returns the number of bytes the passed parameters
// would take when encoded with the format used by the prefix hash portion of
// the overall signature hash.
func sigHashPrefixSerializeSize(hashType SigHashType, txIns []*wire.TxIn, txOuts []*wire.TxOut, signIdx int) int {
	// 1) 4 bytes version/serialization type
	// 2) number of inputs varint
	// 3) per input:
	//    a) 32 bytes prevout hash
	//    b) 4 bytes prevout index
	//    c) 1 byte prevout tree
	//    d) 4 bytes sequence
	// 4) number of outputs varint
	// 5) per output:
	//    a) 8 bytes amount
	//    b) 2 bytes script version
	//    c) pkscript len varint (1 byte if not SigHashSingle output)
	//    d) N bytes pkscript (0 bytes if not SigHashSingle output)
	// 6) 4 bytes lock time
	// 7) 4 bytes expiry
	numTxIns := len(txIns)
	numTxOuts := len(txOuts)
	size := 4 + varIntSerializeSize(uint64(numTxIns)) +
		numTxIns*(chainhash.HashSize+4+1+4) +
		varIntSerializeSize(uint64(numTxOuts)) +
		numTxOuts*(8+2) + 4 + 4
	for txOutIdx, txOut := range txOuts {
		pkScript := txOut.PkScript
		if hashType&sigHashMask == SigHashSingle && txOutIdx != signIdx {
			pkScript = nil
		}
		size += varIntSerializeSize(uint64(len(pkScript)))
		size += len(pkScript)
	}
	return size
}

// sigHashWitnessSerializeSize returns the number of bytes the passed parameters
// would take when encoded with the format used by the witness hash portion of
// the overall signature hash.
func sigHashWitnessSerializeSize(hashType SigHashType, txIns []*wire.TxIn, signScript []byte) int {
	// 1) 4 bytes version/serialization type
	// 2) number of inputs varint
	// 3) per input:
	//    a) prevout pkscript varint (1 byte if not input being signed)
	//    b) N bytes prevout pkscript (0 bytes if not input being signed)
	//
	// NOTE: The prevout pkscript is replaced by nil for all inputs except
	// the input being signed.  Thus, all other inputs (aka numTxIns-1) commit
	// to a nil script which gets encoded as a single 0x00 byte.  This is
	// because encoding 0 as a varint results in 0x00 and there is no script
	// to write.  So, rather than looping through all inputs and manually
	// calculating the size per input, use (numTxIns - 1) as an
	// optimization.
	numTxIns := len(txIns)
	return 4 + varIntSerializeSize(uint64(numTxIns)) + (numTxIns - 1) +
		varIntSerializeSize(uint64(len(signScript))) +
		len(signScript)
}

// calcSignatureHash computes the signature hash for the specified input of
// the target transaction observing the desired signature hash type.  The
// cached prefix parameter allows the caller to optimize the calculation by
// providing the prefix hash to be reused in the case of SigHashAll without the
// SigHashAnyOneCanPay flag set.
func calcSignatureHash(prevOutScript []parsedOpcode, hashType SigHashType, tx *wire.MsgTx, idx int, cachedPrefix *chainhash.Hash) ([]byte, error) {
	// The SigHashSingle signature type signs only the corresponding input
	// and output (the output with the same index number as the input).
	//
	// Since transactions can have more inputs than outputs, this means it
	// is improper to use SigHashSingle on input indices that don't have a
	// corresponding output.
	if hashType&sigHashMask == SigHashSingle && idx >= len(tx.TxOut) {
		str := fmt.Sprintf("attempt to sign single input at index %d "+
			">= %d outputs", idx, len(tx.TxOut))
		return nil, scriptError(ErrInvalidSigHashSingleIndex, str)
	}

	// Remove all instances of OP_CODESEPARATOR from the script.
	//
	// The call to unparseScript cannot fail here because removeOpcode
	// only returns a valid script.
	prevOutScript = removeOpcode(prevOutScript, OP_CODESEPARATOR)
	signScript, _ := unparseScript(prevOutScript)

	// Choose the inputs that will be committed to based on the signature
	// hash type.
	//
	// The SigHashAnyOneCanPay flag specifies that the signature will only
	// commit to the input being signed.  Otherwise, it will commit to all
	// inputs.
	txIns := tx.TxIn
	signTxInIdx := idx
	if hashType&SigHashAnyOneCanPay != 0 {
		txIns = tx.TxIn[idx : idx+1]
		signTxInIdx = 0
	}

	// The prefix hash commits to the non-witness data depending on the
	// signature hash type.  In particular, the specific inputs and output
	// semantics which are committed to are modified depending on the
	// signature hash type as follows:
	//
	// SigHashAll (and undefined signature hash types):
	//   Commits to all outputs.
	// SigHashNone:
	//   Commits to no outputs with all input sequences except the input
	//   being signed replaced with 0.
	// SigHashSingle:
	//   Commits to a single output at the same index as the input being
	//   signed.  All outputs before that index are cleared by setting the
	//   value to -1 and pkscript to nil and all outputs after that index
	//   are removed.  Like SigHashNone, all input sequences except the
	//   input being signed are replaced by 0.
	// SigHashAnyOneCanPay:
	//   Commits to only the input being signed.  Bit flag that can be
	//   combined with the other signature hash types.  Without this flag
	//   set, commits to all inputs.
	//
	// With the relevant inputs and outputs selected and the aforementioned
	// substitions, the prefix hash consists of the hash of the
	// serialization of the following fields:
	//
	// 1) txversion|(SigHashSerializePrefix<<16) (as little-endian uint32)
	// 2) number of inputs (as varint)
	// 3) per input:
	//    a) prevout hash (as little-endian uint256)
	//    b) prevout index (as little-endian uint32)
	//    c) prevout tree (as single byte)
	//    d) sequence (as little-endian uint32)
	// 4) number of outputs (as varint)
	// 5) per output:
	//    a) output amount (as little-endian uint64)
	//    b) pkscript version (as little-endian uint16)
	//    c) pkscript length (as varint)
	//    d) pkscript (as unmodified bytes)
	// 6) transaction lock time (as little-endian uint32)
	// 7) transaction expiry (as little-endian uint32)
	//
	// In addition, an optimization for SigHashAll is provided when the
	// SigHashAnyOneCanPay flag is not set.  In that case, the prefix hash
	// can be reused because only the witness data has been modified, so
	// the wasteful extra O(N^2) hash can be avoided.
	var prefixHash chainhash.Hash
	if chaincfg.SigHashOptimization && cachedPrefix != nil &&
		hashType&sigHashMask == SigHashAll &&
		hashType&SigHashAnyOneCanPay == 0 {

		prefixHash = *cachedPrefix
	} else {
		// Choose the outputs to commit to based on the signature hash
		// type.
		//
		// As the names imply, SigHashNone commits to no outputs and
		// SigHashSingle commits to the single output that corresponds
		// to the input being signed.  However, SigHashSingle is also a
		// bit special in that it commits to cleared out variants of all
		// outputs prior to the one being signed.  This is required by
		// consensus due to legacy reasons.
		//
		// All other signature hash types, such as SighHashAll commit to
		// all outputs.  Note that this includes undefined hash types as well.
		txOuts := tx.TxOut
		switch hashType & sigHashMask {
		case SigHashNone:
			txOuts = nil
		case SigHashSingle:
			txOuts = tx.TxOut[:idx+1]
		default:
			fallthrough
		case SigHashAll:
			// Nothing special here.
		}

		size := sigHashPrefixSerializeSize(hashType, txIns, txOuts, idx)
		prefixBuf := make([]byte, size)

		// Commit to the version and hash serialization type.
		version := uint32(tx.Version) | uint32(SigHashSerializePrefix)<<16
		offset := putUint32LE(prefixBuf, version)

		// Commit to the relevant transaction inputs.
		offset += putVarInt(prefixBuf[offset:], uint64(len(txIns)))
		for txInIdx, txIn := range txIns {
			// Commit to the outpoint being spent.
			prevOut := &txIn.PreviousOutPoint
			offset += copy(prefixBuf[offset:], prevOut.Hash[:])
			offset += putUint32LE(prefixBuf[offset:], prevOut.Index)
			offset += putByte(prefixBuf[offset:], byte(prevOut.Tree))

			// Commit to the sequence.  In the case of SigHashNone
			// and SigHashSingle, commit to 0 for everything that is
			// not the input being signed instead.
			sequence := txIn.Sequence
			if (hashType&sigHashMask == SigHashNone ||
				hashType&sigHashMask == SigHashSingle) &&
				txInIdx != signTxInIdx {

				sequence = 0
			}
			offset += putUint32LE(prefixBuf[offset:], sequence)
		}

		// Commit to the relevant transaction outputs.
		offset += putVarInt(prefixBuf[offset:], uint64(len(txOuts)))
		for txOutIdx, txOut := range txOuts {
			// Commit to the output amount, script version, and
			// public key script.  In the case of SigHashSingle,
			// commit to an output amount of -1 and a nil public
			// key script for everything that is not the output
			// corresponding to the input being signed instead.
			value := txOut.Value
			pkScript := txOut.PkScript
			if hashType&sigHashMask == SigHashSingle && txOutIdx != idx {
				value = -1
				pkScript = nil
			}
			offset += putUint64LE(prefixBuf[offset:], uint64(value))
			offset += putUint16LE(prefixBuf[offset:], txOut.Version)
			offset += putVarInt(prefixBuf[offset:], uint64(len(pkScript)))
			offset += copy(prefixBuf[offset:], pkScript)
		}

		// Commit to the lock time and expiry.
		offset += putUint32LE(prefixBuf[offset:], tx.LockTime)
		putUint32LE(prefixBuf[offset:], tx.Expiry)

		prefixHash = chainhash.HashH(prefixBuf)
	}

	// The witness hash commits to the input witness data depending on
	// whether or not the signature hash type has the SigHashAnyOneCanPay
	// flag set.  In particular the semantics are as follows:
	//
	// SigHashAnyOneCanPay:
	//   Commits to only the input being signed.  Without this flag set,
	//   commits to all inputs with the reference scripts cleared by setting
	//   them to nil.
	//
	// With the relevant inputs selected, and the aforementioned substitutions,
	// the witness hash consists of the hash of the serialization of the
	// following fields:
	//
	// 1) txversion|(SigHashSerializeWitness<<16) (as little-endian uint32)
	// 2) number of inputs (as varint)
	// 3) per input:
	//    a) length of prevout pkscript (as varint)
	//    b) prevout pkscript (as unmodified bytes)

	size := sigHashWitnessSerializeSize(hashType, txIns, signScript)
	witnessBuf := make([]byte, size)

	// Commit to the version and hash serialization type.
	version := uint32(tx.Version) | uint32(SigHashSerializeWitness)<<16
	offset := putUint32LE(witnessBuf, version)

	// Commit to the relevant transaction inputs.
	offset += putVarInt(witnessBuf[offset:], uint64(len(txIns)))
	for txInIdx := range txIns {
		// Commit to the input script at the index corresponding to the
		// input index being signed.  Otherwise, commit to a nil script
		// instead.
		commitScript := signScript
		if txInIdx != signTxInIdx {
			commitScript = nil
		}
		offset += putVarInt(witnessBuf[offset:], uint64(len(commitScript)))
		offset += copy(witnessBuf[offset:], commitScript)
	}

	witnessHash := chainhash.HashH(witnessBuf)

	// The final signature hash (message to sign) is the hash of the
	// serialization of the following fields:
	//
	// 1) the hash type (as little-endian uint32)
	// 2) prefix hash (as produced by hash function)
	// 3) witness hash (as produced by hash function)
	sigHashBuf := make([]byte, chainhash.HashSize*2+4)
	offset = putUint32LE(sigHashBuf, uint32(hashType))
	offset += copy(sigHashBuf[offset:], prefixHash[:])
	copy(sigHashBuf[offset:], witnessHash[:])
	return chainhash.HashB(sigHashBuf), nil
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
