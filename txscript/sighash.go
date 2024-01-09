// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// SigHashType represents hash type bits at the end of a signature.
type SigHashType uint32

// Hash type bits from the end of a signature.
const (
	SigHashDefault      SigHashType = 0x00
	SigHashOld          SigHashType = 0x0
	SigHashAll          SigHashType = 0x1
	SigHashNone         SigHashType = 0x2
	SigHashSingle       SigHashType = 0x3
	SigHashAnyOneCanPay SigHashType = 0x80

	// sigHashMask defines the number of bits of the hash type which is used
	// to identify which outputs are signed.
	sigHashMask = 0x1f
)

const (
	// blankCodeSepValue is the value of the code separator position in the
	// tapscript sighash when no code separator was found in the script.
	blankCodeSepValue = math.MaxUint32
)

// shallowCopyTx creates a shallow copy of the transaction for use when
// calculating the signature hash.  It is used over the Copy method on the
// transaction itself since that is a deep copy and therefore does more work and
// allocates much more space than needed.
func shallowCopyTx(tx *wire.MsgTx) wire.MsgTx {
	// As an additional memory optimization, use contiguous backing arrays
	// for the copied inputs and outputs and point the final slice of
	// pointers into the contiguous arrays.  This avoids a lot of small
	// allocations.
	txCopy := wire.MsgTx{
		Version:  tx.Version,
		TxIn:     make([]*wire.TxIn, len(tx.TxIn)),
		TxOut:    make([]*wire.TxOut, len(tx.TxOut)),
		LockTime: tx.LockTime,
	}
	txIns := make([]wire.TxIn, len(tx.TxIn))
	for i, oldTxIn := range tx.TxIn {
		txIns[i] = *oldTxIn
		txCopy.TxIn[i] = &txIns[i]
	}
	txOuts := make([]wire.TxOut, len(tx.TxOut))
	for i, oldTxOut := range tx.TxOut {
		txOuts[i] = *oldTxOut
		txCopy.TxOut[i] = &txOuts[i]
	}
	return txCopy
}

// CalcSignatureHash will, given a script and hash type for the current script
// engine instance, calculate the signature hash to be used for signing and
// verification.
//
// NOTE: This function is only valid for version 0 scripts. Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func CalcSignatureHash(script []byte, hashType SigHashType, tx *wire.MsgTx, idx int) ([]byte, error) {
	const scriptVersion = 0
	if err := checkScriptParses(scriptVersion, script); err != nil {
		return nil, err
	}

	return calcSignatureHash(script, hashType, tx, idx), nil
}

// calcSignatureHash computes the signature hash for the specified input of the
// target transaction observing the desired signature hash type.
func calcSignatureHash(sigScript []byte, hashType SigHashType, tx *wire.MsgTx, idx int) []byte {
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
	if hashType&sigHashMask == SigHashSingle && idx >= len(tx.TxOut) {
		var hash chainhash.Hash
		hash[0] = 0x01
		return hash[:]
	}

	// Remove all instances of OP_CODESEPARATOR from the script.
	sigScript = removeOpcodeRaw(sigScript, OP_CODESEPARATOR)

	// Make a shallow copy of the transaction, zeroing out the script for
	// all inputs that are not currently being processed.
	txCopy := shallowCopyTx(tx)
	for i := range txCopy.TxIn {
		if i == idx {
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
	case SigHashAll:
		// Nothing special here.
	}
	if hashType&SigHashAnyOneCanPay != 0 {
		txCopy.TxIn = txCopy.TxIn[idx : idx+1]
	}

	// The final hash is the double sha256 of both the serialized modified
	// transaction and the hash type (encoded as a 4-byte little-endian
	// value) appended.
	sigHashBytes := chainhash.DoubleHashRaw(func(w io.Writer) error {
		if err := txCopy.SerializeNoWitness(w); err != nil {
			return err
		}
		err := binary.Write(w, binary.LittleEndian, hashType)
		if err != nil {
			return err
		}
		return nil
	})

	return sigHashBytes[:]
}

// calcWitnessSignatureHashRaw computes the sighash digest of a transaction's
// segwit input using the new, optimized digest calculation algorithm defined
// in BIP0143: https://github.com/bitcoin/bips/blob/master/bip-0143.mediawiki.
// This function makes use of pre-calculated sighash fragments stored within
// the passed HashCache to eliminate duplicate hashing computations when
// calculating the final digest, reducing the complexity from O(N^2) to O(N).
// Additionally, signatures now cover the input value of the referenced unspent
// output. This allows offline, or hardware wallets to compute the exact amount
// being spent, in addition to the final transaction fee. In the case the
// wallet if fed an invalid input amount, the real sighash will differ causing
// the produced signature to be invalid.
func calcWitnessSignatureHashRaw(subScript []byte, sigHashes *TxSigHashes,
	hashType SigHashType, tx *wire.MsgTx, idx int, amt int64) ([]byte, error) {

	// As a sanity check, ensure the passed input index for the transaction
	// is valid.
	//
	// TODO(roasbeef): check needs to be lifted elsewhere?
	if idx > len(tx.TxIn)-1 {
		return nil, fmt.Errorf("idx %d but %d txins", idx, len(tx.TxIn))
	}

	sigHashBytes := chainhash.DoubleHashRaw(func(w io.Writer) error {
		var scratch [8]byte

		// First write out, then encode the transaction's version
		// number.
		binary.LittleEndian.PutUint32(scratch[:], uint32(tx.Version))
		w.Write(scratch[:4])

		// Next write out the possibly pre-calculated hashes for the
		// sequence numbers of all inputs, and the hashes of the
		// previous outs for all outputs.
		var zeroHash chainhash.Hash

		// If anyone can pay isn't active, then we can use the cached
		// hashPrevOuts, otherwise we just write zeroes for the prev
		// outs.
		if hashType&SigHashAnyOneCanPay == 0 {
			w.Write(sigHashes.HashPrevOutsV0[:])
		} else {
			w.Write(zeroHash[:])
		}

		// If the sighash isn't anyone can pay, single, or none, the
		// use the cached hash sequences, otherwise write all zeroes
		// for the hashSequence.
		if hashType&SigHashAnyOneCanPay == 0 &&
			hashType&sigHashMask != SigHashSingle &&
			hashType&sigHashMask != SigHashNone {

			w.Write(sigHashes.HashSequenceV0[:])
		} else {
			w.Write(zeroHash[:])
		}

		txIn := tx.TxIn[idx]

		// Next, write the outpoint being spent.
		w.Write(txIn.PreviousOutPoint.Hash[:])
		var bIndex [4]byte
		binary.LittleEndian.PutUint32(
			bIndex[:], txIn.PreviousOutPoint.Index,
		)
		w.Write(bIndex[:])

		if isWitnessPubKeyHashScript(subScript) {
			// The script code for a p2wkh is a length prefix
			// varint for the next 25 bytes, followed by a
			// re-creation of the original p2pkh pk script.
			w.Write([]byte{0x19})
			w.Write([]byte{OP_DUP})
			w.Write([]byte{OP_HASH160})
			w.Write([]byte{OP_DATA_20})
			w.Write(extractWitnessPubKeyHash(subScript))
			w.Write([]byte{OP_EQUALVERIFY})
			w.Write([]byte{OP_CHECKSIG})
		} else {
			// For p2wsh outputs, and future outputs, the script
			// code is the original script, with all code
			// separators removed, serialized with a var int length
			// prefix.
			wire.WriteVarBytes(w, 0, subScript)
		}

		// Next, add the input amount, and sequence number of the input
		// being signed.
		binary.LittleEndian.PutUint64(scratch[:], uint64(amt))
		w.Write(scratch[:])
		binary.LittleEndian.PutUint32(scratch[:], txIn.Sequence)
		w.Write(scratch[:4])

		// If the current signature mode isn't single, or none, then we
		// can re-use the pre-generated hashoutputs sighash fragment.
		// Otherwise, we'll serialize and add only the target output
		// index to the signature pre-image.
		if hashType&sigHashMask != SigHashSingle &&
			hashType&sigHashMask != SigHashNone {

			w.Write(sigHashes.HashOutputsV0[:])
		} else if hashType&sigHashMask == SigHashSingle &&
			idx < len(tx.TxOut) {

			h := chainhash.DoubleHashRaw(func(tw io.Writer) error {
				wire.WriteTxOut(tw, 0, 0, tx.TxOut[idx])
				return nil
			})
			w.Write(h[:])
		} else {
			w.Write(zeroHash[:])
		}

		// Finally, write out the transaction's locktime, and the sig
		// hash type.
		binary.LittleEndian.PutUint32(scratch[:], tx.LockTime)
		w.Write(scratch[:4])
		binary.LittleEndian.PutUint32(scratch[:], uint32(hashType))
		w.Write(scratch[:4])

		return nil
	})

	return sigHashBytes[:], nil
}

// CalcWitnessSigHash computes the sighash digest for the specified input of
// the target transaction observing the desired sig hash type.
func CalcWitnessSigHash(script []byte, sigHashes *TxSigHashes, hType SigHashType,
	tx *wire.MsgTx, idx int, amt int64) ([]byte, error) {

	const scriptVersion = 0
	if err := checkScriptParses(scriptVersion, script); err != nil {
		return nil, err
	}

	return calcWitnessSignatureHashRaw(script, sigHashes, hType, tx, idx, amt)
}

// sigHashExtFlag represents the sig hash extension flag as defined in BIP 341.
// Extensions to the base sighash algorithm will be appended to the base
// sighash digest.
type sigHashExtFlag uint8

const (
	// baseSigHashExtFlag is the base extension flag. This adds no changes
	// to the sighash digest message. This is used for segwit v1 spends,
	// a.k.a the tapscript keyspend path.
	baseSigHashExtFlag sigHashExtFlag = 0

	// tapscriptSighashExtFlag is the extension flag defined by tapscript
	// base leaf version spend define din BIP 342. This augments the base
	// sighash by including the tapscript leaf hash, the key version, and
	// the code separator position.
	tapscriptSighashExtFlag sigHashExtFlag = 1
)

// taprootSigHashOptions houses a set of functional options that may optionally
// modify how the taproot/script sighash digest algorithm is implemented.
type taprootSigHashOptions struct {
	// extFlag denotes the current message digest extension being used. For
	// top-level script spends use a value of zero, while each tapscript
	// version can define its own values as well.
	extFlag sigHashExtFlag

	// annexHash is the sha256 hash of the annex with a compact size length
	// prefix: sha256(sizeOf(annex) || annex).
	annexHash []byte

	// tapLeafHash is the hash of the tapscript leaf as defined in BIP 341.
	// This should be h_tapleaf(version || compactSizeOf(script) || script).
	tapLeafHash []byte

	// keyVersion is the key version as defined in BIP 341. This is always
	// 0x00 for all currently defined leaf versions.
	keyVersion byte

	// codeSepPos is the op code position of the last code separator. This
	// is used for the BIP 342 sighash message extension.
	codeSepPos uint32
}

// writeDigestExtensions writes out the sighash message extension defined by the
// current active sigHashExtFlags.
func (t *taprootSigHashOptions) writeDigestExtensions(w io.Writer) error {
	switch t.extFlag {
	// The base extension, used for tapscript keypath spends doesn't modify
	// the digest at all.
	case baseSigHashExtFlag:
		return nil

	// The tapscript base leaf version extension adds the leaf hash, key
	// version, and code separator position to the final digest.
	case tapscriptSighashExtFlag:
		if _, err := w.Write(t.tapLeafHash); err != nil {
			return err
		}
		if _, err := w.Write([]byte{t.keyVersion}); err != nil {
			return err
		}
		err := binary.Write(w, binary.LittleEndian, t.codeSepPos)
		if err != nil {
			return err
		}
	}

	return nil
}

// defaultTaprootSighashOptions returns the set of default sighash options for
// taproot execution.
func defaultTaprootSighashOptions() *taprootSigHashOptions {
	return &taprootSigHashOptions{}
}

// TaprootSigHashOption defines a set of functional param options that can be
// used to modify the base sighash message with optional extensions.
type TaprootSigHashOption func(*taprootSigHashOptions)

// WithAnnex is a functional option that allows the caller to specify the
// existence of an annex in the final witness stack for the taproot/tapscript
// spends.
func WithAnnex(annex []byte) TaprootSigHashOption {
	return func(o *taprootSigHashOptions) {
		// It's just a bytes.Buffer which never returns an error on
		// write.
		var b bytes.Buffer
		_ = wire.WriteVarBytes(&b, 0, annex)

		o.annexHash = chainhash.HashB(b.Bytes())
	}
}

// WithBaseTapscriptVersion is a functional option that specifies that the
// sighash digest should include the extra information included as part of the
// base tapscript version.
func WithBaseTapscriptVersion(codeSepPos uint32,
	tapLeafHash []byte) TaprootSigHashOption {

	return func(o *taprootSigHashOptions) {
		o.extFlag = tapscriptSighashExtFlag
		o.tapLeafHash = tapLeafHash
		o.keyVersion = 0
		o.codeSepPos = codeSepPos
	}
}

// isValidTaprootSigHash returns true if the passed sighash is a valid taproot
// sighash.
func isValidTaprootSigHash(hashType SigHashType) bool {
	switch hashType {
	case SigHashDefault, SigHashAll, SigHashNone, SigHashSingle:
		fallthrough
	case 0x81, 0x82, 0x83:
		return true

	default:
		return false
	}
}

// calcTaprootSignatureHashRaw computes the sighash as specified in BIP 143.
// If an invalid sighash type is passed in, an error is returned.
func calcTaprootSignatureHashRaw(sigHashes *TxSigHashes, hType SigHashType,
	tx *wire.MsgTx, idx int,
	prevOutFetcher PrevOutputFetcher,
	sigHashOpts ...TaprootSigHashOption) ([]byte, error) {

	opts := defaultTaprootSighashOptions()
	for _, sigHashOpt := range sigHashOpts {
		sigHashOpt(opts)
	}

	// If a valid sighash type isn't passed in, then we'll exit early.
	if !isValidTaprootSigHash(hType) {
		// TODO(roasbeef): use actual errr here
		return nil, fmt.Errorf("invalid taproot sighash type: %v", hType)
	}

	// As a sanity check, ensure the passed input index for the transaction
	// is valid.
	if idx > len(tx.TxIn)-1 {
		return nil, fmt.Errorf("idx %d but %d txins", idx, len(tx.TxIn))
	}

	// We'll utilize this buffer throughout to incrementally calculate
	// the signature hash for this transaction.
	var sigMsg bytes.Buffer

	// The final sighash always has a value of 0x00 prepended to it, which
	// is called the sighash epoch.
	sigMsg.WriteByte(0x00)

	// First, we write the hash type encoded as a single byte.
	if err := sigMsg.WriteByte(byte(hType)); err != nil {
		return nil, err
	}

	// Next we'll write out the transaction specific data which binds the
	// outer context of the sighash.
	err := binary.Write(&sigMsg, binary.LittleEndian, tx.Version)
	if err != nil {
		return nil, err
	}
	err = binary.Write(&sigMsg, binary.LittleEndian, tx.LockTime)
	if err != nil {
		return nil, err
	}

	// If sighash isn't anyone can pay, then we'll include all the
	// pre-computed midstate digests in the sighash.
	if hType&SigHashAnyOneCanPay != SigHashAnyOneCanPay {
		sigMsg.Write(sigHashes.HashPrevOutsV1[:])
		sigMsg.Write(sigHashes.HashInputAmountsV1[:])
		sigMsg.Write(sigHashes.HashInputScriptsV1[:])
		sigMsg.Write(sigHashes.HashSequenceV1[:])
	}

	// If this is sighash all, or its taproot alias (sighash default),
	// then we'll also include the pre-computed digest of all the outputs
	// of the transaction.
	if hType&SigHashSingle != SigHashSingle &&
		hType&SigHashSingle != SigHashNone {

		sigMsg.Write(sigHashes.HashOutputsV1[:])
	}

	// Next, we'll write out the relevant information for this specific
	// input.
	//
	// The spend type is computed as the (ext_flag*2) + annex_present. We
	// use this to bind the extension flag (that BIP 342 uses), as well as
	// the annex if its present.
	input := tx.TxIn[idx]
	witnessHasAnnex := opts.annexHash != nil
	spendType := byte(opts.extFlag) * 2
	if witnessHasAnnex {
		spendType += 1
	}

	if err := sigMsg.WriteByte(spendType); err != nil {
		return nil, err
	}

	// If anyone can pay is active, then we'll write out just the specific
	// information about this input, given we skipped writing all the
	// information of all the inputs above.
	if hType&SigHashAnyOneCanPay == SigHashAnyOneCanPay {
		// We'll start out with writing this input specific information by
		// first writing the entire previous output.
		err = wire.WriteOutPoint(&sigMsg, 0, 0, &input.PreviousOutPoint)
		if err != nil {
			return nil, err
		}

		// Next, we'll write out the previous output (amt+script) being
		// spent itself.
		prevOut := prevOutFetcher.FetchPrevOutput(input.PreviousOutPoint)
		if err := wire.WriteTxOut(&sigMsg, 0, 0, prevOut); err != nil {
			return nil, err
		}

		// Finally, we'll write out the input sequence itself.
		err = binary.Write(&sigMsg, binary.LittleEndian, input.Sequence)
		if err != nil {
			return nil, err
		}
	} else {
		err := binary.Write(&sigMsg, binary.LittleEndian, uint32(idx))
		if err != nil {
			return nil, err
		}
	}

	// Now that we have the input specific information written, we'll
	// include the anex, if we have it.
	if witnessHasAnnex {
		sigMsg.Write(opts.annexHash)
	}

	// Finally, if this is sighash single, then we'll write out the
	// information for this given output.
	if hType&sigHashMask == SigHashSingle {
		// If this output doesn't exist, then we'll return with an error
		// here as this is an invalid sighash type for this input.
		if idx >= len(tx.TxOut) {
			// TODO(roasbeef): real error here
			return nil, fmt.Errorf("invalid sighash type for input")
		}

		// Now that we know this is a valid sighash input combination,
		// we'll write out the information specific to this input.
		// We'll write the wire serialization of the output and compute
		// the sha256 in a single step.
		shaWriter := sha256.New()
		txOut := tx.TxOut[idx]
		if err := wire.WriteTxOut(shaWriter, 0, 0, txOut); err != nil {
			return nil, err
		}

		// With the digest obtained, we'll write this out into our
		// signature message.
		if _, err := sigMsg.Write(shaWriter.Sum(nil)); err != nil {
			return nil, err
		}
	}

	// Now that we've written out all the base information, we'll write any
	// message extensions (if they exist).
	if err := opts.writeDigestExtensions(&sigMsg); err != nil {
		return nil, err
	}

	// The final sighash is computed as: hash_TagSigHash(0x00 || sigMsg).
	// We wrote the 0x00 above so we don't need to append here and incur
	// extra allocations.
	sigHash := chainhash.TaggedHash(chainhash.TagTapSighash, sigMsg.Bytes())
	return sigHash[:], nil
}

// CalcTaprootSignatureHash computes the sighash digest of a transaction's
// taproot-spending input using the new sighash digest algorithm described in
// BIP 341. As the new digest algorithms may require the digest to commit to the
// entire prev output, a PrevOutputFetcher argument is required to obtain the
// needed information. The TxSigHashes pre-computed sighash midstate MUST be
// specified.
func CalcTaprootSignatureHash(sigHashes *TxSigHashes, hType SigHashType,
	tx *wire.MsgTx, idx int,
	prevOutFetcher PrevOutputFetcher) ([]byte, error) {

	return calcTaprootSignatureHashRaw(
		sigHashes, hType, tx, idx, prevOutFetcher,
	)
}

// CalcTaprootSignatureHash is similar to CalcTaprootSignatureHash but for
// _tapscript_ spends instead. A proper TapLeaf instance (the script leaf being
// signed) must be passed in. The functional options can be used to specify an
// annex if the signature was bound to that context.
//
// NOTE: This function is able to compute the sighash of scripts that contain a
// code separator if the caller passes in an instance of
// WithBaseTapscriptVersion with the valid position.
func CalcTapscriptSignaturehash(sigHashes *TxSigHashes, hType SigHashType,
	tx *wire.MsgTx, idx int, prevOutFetcher PrevOutputFetcher,
	tapLeaf TapLeaf,
	sigHashOpts ...TaprootSigHashOption) ([]byte, error) {

	tapLeafHash := tapLeaf.TapHash()

	var opts []TaprootSigHashOption
	opts = append(
		opts, WithBaseTapscriptVersion(blankCodeSepValue, tapLeafHash[:]),
	)
	opts = append(opts, sigHashOpts...)

	return calcTaprootSignatureHashRaw(
		sigHashes, hType, tx, idx, prevOutFetcher, opts...,
	)
}
