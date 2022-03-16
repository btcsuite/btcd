// Copyright (c) 2013-2022 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// signatureVerifier is an abstract interface that allows the op code execution
// to abstract over the _type_ of signature validation being executed. At this
// point in Bitcoin's history, there're four possible sig validation contexts:
// pre-segwit, segwit v0, segwit v1 (taproot key spend validation), and the
// base tapscript verification.
type signatureVerifier interface {
	// Verify returns true if the signature verifier context deems the
	// signature to be valid for the given context.
	Verify() bool
}

// baseSigVerifier is used to verify signatures for the _base_ system, meaning
// ECDSA signatures encoded in DER or BER encoding.
type baseSigVerifier struct {
	vm *Engine

	pubKey *btcec.PublicKey

	sig *ecdsa.Signature

	fullSigBytes []byte

	sigBytes []byte
	pkBytes  []byte

	subScript []byte

	hashType SigHashType
}

// parseBaseSigAndPubkey attempts to parse a signature and public key according
// to the base consensus rules, which expect an 33-byte public key and DER or
// BER encoded signature.
func parseBaseSigAndPubkey(pkBytes, fullSigBytes []byte,
	vm *Engine) (*btcec.PublicKey, *ecdsa.Signature, SigHashType, error) {

	strictEncoding := vm.hasFlag(ScriptVerifyStrictEncoding) ||
		vm.hasFlag(ScriptVerifyDERSignatures)

	// Trim off hashtype from the signature string and check if the
	// signature and pubkey conform to the strict encoding requirements
	// depending on the flags.
	//
	// NOTE: When the strict encoding flags are set, any errors in the
	// signature or public encoding here result in an immediate script error
	// (and thus no result bool is pushed to the data stack).  This differs
	// from the logic below where any errors in parsing the signature is
	// treated as the signature failure resulting in false being pushed to
	// the data stack.  This is required because the more general script
	// validation consensus rules do not have the new strict encoding
	// requirements enabled by the flags.
	hashType := SigHashType(fullSigBytes[len(fullSigBytes)-1])
	sigBytes := fullSigBytes[:len(fullSigBytes)-1]
	if err := vm.checkHashTypeEncoding(hashType); err != nil {
		return nil, nil, 0, err
	}
	if err := vm.checkSignatureEncoding(sigBytes); err != nil {
		return nil, nil, 0, err
	}
	if err := vm.checkPubKeyEncoding(pkBytes); err != nil {
		return nil, nil, 0, err
	}

	// First, parse the public key, which we expect to be in the proper
	// encoding.
	pubKey, err := btcec.ParsePubKey(pkBytes)
	if err != nil {
		return nil, nil, 0, err
	}

	// Next, parse the signature which should be in DER or BER depending on
	// the active script flags.
	var signature *ecdsa.Signature
	if strictEncoding {
		signature, err = ecdsa.ParseDERSignature(sigBytes)
	} else {
		signature, err = ecdsa.ParseSignature(sigBytes)
	}
	if err != nil {
		return nil, nil, 0, err
	}

	return pubKey, signature, hashType, nil
}

// newBaseSigVerifier returns a new instance of the base signature verifier. An
// error is returned if the signature, sighash, or public key aren't correctly
// encoded.
func newBaseSigVerifier(pkBytes, fullSigBytes []byte,
	vm *Engine) (*baseSigVerifier, error) {

	pubKey, sig, hashType, err := parseBaseSigAndPubkey(
		pkBytes, fullSigBytes, vm,
	)
	if err != nil {
		return nil, err
	}

	// Get script starting from the most recent OP_CODESEPARATOR.
	subScript := vm.subScript()

	return &baseSigVerifier{
		vm:           vm,
		pubKey:       pubKey,
		pkBytes:      pkBytes,
		sig:          sig,
		sigBytes:     fullSigBytes[:len(fullSigBytes)-1],
		subScript:    subScript,
		hashType:     hashType,
		fullSigBytes: fullSigBytes,
	}, nil
}

// verifySig attempts to verify the signature given the computed sighash. A nil
// error is returned if the signature is valid.
func (b *baseSigVerifier) verifySig(sigHash []byte) bool {
	var valid bool
	if b.vm.sigCache != nil {
		var sigHashBytes chainhash.Hash
		copy(sigHashBytes[:], sigHash[:])

		valid = b.vm.sigCache.Exists(sigHashBytes, b.sigBytes, b.pkBytes)
		if !valid && b.sig.Verify(sigHash, b.pubKey) {
			b.vm.sigCache.Add(sigHashBytes, b.sigBytes, b.pkBytes)
			valid = true
		}
	} else {
		valid = b.sig.Verify(sigHash, b.pubKey)
	}

	return valid
}

// Verify returns true if the signature verifier context deems the signature to
// be valid for the given context.
//
// NOTE: This is part of the baseSigVerifier interface.
func (b *baseSigVerifier) Verify() bool {
	// Remove the signature since there is no way for a signature
	// to sign itself.
	subScript := removeOpcodeByData(b.subScript, b.fullSigBytes)

	sigHash := calcSignatureHash(
		subScript, b.hashType, &b.vm.tx, b.vm.txIdx,
	)

	return b.verifySig(sigHash)
}

// A compile-time assertion to ensure baseSigVerifier implements the
// signatureVerifier interface.
var _ signatureVerifier = (*baseSigVerifier)(nil)

// baseSegwitSigVerifier implements signature verification for segwit v0. The
// only difference between this and the baseSigVerifier is how the sighash is
// computed.
type baseSegwitSigVerifier struct {
	*baseSigVerifier
}

// newBaseSegwitSigVerifier returns a new instance of the base segwit verifier.
func newBaseSegwitSigVerifier(pkBytes, fullSigBytes []byte,
	vm *Engine) (*baseSegwitSigVerifier, error) {

	sigVerifier, err := newBaseSigVerifier(pkBytes, fullSigBytes, vm)
	if err != nil {
		return nil, err
	}

	return &baseSegwitSigVerifier{
		baseSigVerifier: sigVerifier,
	}, nil
}

// Verify returns true if the signature verifier context deems the signature to
// be valid for the given context.
//
// NOTE: This is part of the baseSigVerifier interface.
func (s *baseSegwitSigVerifier) Verify() bool {
	var sigHashes *TxSigHashes
	if s.vm.hashCache != nil {
		sigHashes = s.vm.hashCache
	} else {
		sigHashes = NewTxSigHashes(&s.vm.tx, s.vm.prevOutFetcher)
	}

	sigHash, err := calcWitnessSignatureHashRaw(
		s.subScript, sigHashes, s.hashType, &s.vm.tx, s.vm.txIdx,
		s.vm.inputAmount,
	)
	if err != nil {
		// TODO(roasbeef): this doesn't need to return an error, should
		// instead be further up the stack? this only returns an error
		// if the input index is greater than the number of inputs
		return false
	}

	return s.verifySig(sigHash)
}

// A compile-time assertion to ensure baseSegwitSigVerifier implements the
// signatureVerifier interface.
var _ signatureVerifier = (*baseSegwitSigVerifier)(nil)

// taprootSigVerifier verifies signatures according to the segwit v1 rules,
// which are described in BIP 341.
type taprootSigVerifier struct {
	pubKey  *btcec.PublicKey
	pkBytes []byte

	fullSigBytes []byte
	sig          *schnorr.Signature

	hashType SigHashType

	sigCache  *SigCache
	hashCache *TxSigHashes

	tx *wire.MsgTx

	inputIndex int

	annex []byte

	prevOuts PrevOutputFetcher
}

// parseTaprootSigAndPubKey attempts to parse the public key and signature for
// a taproot spend that may be a keyspend or script path spend. This function
// returns an error if the pubkey is invalid, or the sig is.
func parseTaprootSigAndPubKey(pkBytes, rawSig []byte,
) (*btcec.PublicKey, *schnorr.Signature, SigHashType, error) {

	// Now that we have the raw key, we'll parse it into a schnorr public
	// key we can work with.
	pubKey, err := schnorr.ParsePubKey(pkBytes)
	if err != nil {
		return nil, nil, 0, err
	}

	// Next, we'll parse the signature, which may or may not be appended
	// with the desired sighash flag.
	var (
		sig         *schnorr.Signature
		sigHashType SigHashType
	)
	switch {
	// If the signature is exactly 64 bytes, then we know we're using the
	// implicit SIGHASH_DEFAULT sighash type.
	case len(rawSig) == schnorr.SignatureSize:
		// First, parse out the signature which is just the raw sig itself.
		sig, err = schnorr.ParseSignature(rawSig)
		if err != nil {
			return nil, nil, 0, err
		}

		// If the sig is 64 bytes, then we'll assume that it's the
		// default sighash type, which is actually an alias for
		// SIGHASH_ALL.
		sigHashType = SigHashDefault

	// Otherwise, if this is a signature, with a sighash looking byte
	// appended that isn't all zero, then we'll extract the sighash from
	// the end of the signature.
	case len(rawSig) == schnorr.SignatureSize+1 && rawSig[64] != 0:
		// Extract the sighash type, then snip off the last byte so we can
		// parse the signature.
		sigHashType = SigHashType(rawSig[schnorr.SignatureSize])

		rawSig = rawSig[:schnorr.SignatureSize]
		sig, err = schnorr.ParseSignature(rawSig)
		if err != nil {
			return nil, nil, 0, err
		}

	// Otherwise, this is an invalid signature, so we need to bail out.
	default:
		str := fmt.Sprintf("invalid sig len: %v", len(rawSig))
		return nil, nil, 0, scriptError(ErrInvalidTaprootSigLen, str)
	}

	return pubKey, sig, sigHashType, nil
}

// newTaprootSigVerifier returns a new instance of a taproot sig verifier given
// the necessary contextual information.
func newTaprootSigVerifier(pkBytes []byte, fullSigBytes []byte,
	tx *wire.MsgTx, inputIndex int, prevOuts PrevOutputFetcher,
	sigCache *SigCache, hashCache *TxSigHashes,
	annex []byte) (*taprootSigVerifier, error) {

	pubKey, sig, sigHashType, err := parseTaprootSigAndPubKey(
		pkBytes, fullSigBytes,
	)
	if err != nil {
		return nil, err
	}

	return &taprootSigVerifier{
		pubKey:       pubKey,
		pkBytes:      pkBytes,
		sig:          sig,
		fullSigBytes: fullSigBytes,
		hashType:     sigHashType,
		tx:           tx,
		inputIndex:   inputIndex,
		prevOuts:     prevOuts,
		sigCache:     sigCache,
		hashCache:    hashCache,
		annex:        annex,
	}, nil
}

// verifySig attempts to verify a BIP 340 signature using the internal public
// key and signature, and the passed sigHash as the message digest.
func (t *taprootSigVerifier) verifySig(sigHash []byte) bool {
	// At this point, we can check to see if this signature is already
	// included in the sigCcahe and is valid or not (if one was passed in).
	cacheKey, _ := chainhash.NewHash(sigHash)
	if t.sigCache != nil {
		if t.sigCache.Exists(*cacheKey, t.fullSigBytes, t.pkBytes) {
			return true
		}
	}

	// If we didn't find the entry in the cache, then we'll perform full
	// verification as normal, adding the entry to the cache if it's found
	// to be valid.
	sigValid := t.sig.Verify(sigHash, t.pubKey)
	if sigValid {
		if t.sigCache != nil {
			// The sig is valid, so we'll add it to the cache.
			t.sigCache.Add(*cacheKey, t.fullSigBytes, t.pkBytes)
		}

		return true
	}

	// Otherwise the sig is invalid if we get to this point.
	return false
}

// Verify returns true if the signature verifier context deems the signature to
// be valid for the given context.
//
// NOTE: This is part of the baseSigVerifier interface.
func (t *taprootSigVerifier) Verify() bool {
	var opts []TaprootSigHashOption
	if t.annex != nil {
		opts = append(opts, WithAnnex(t.annex))
	}

	// Before we attempt to verify the signature, we'll need to first
	// compute the sighash based on the input and tx information.
	sigHash, err := calcTaprootSignatureHashRaw(
		t.hashCache, t.hashType, t.tx, t.inputIndex, t.prevOuts,
		opts...,
	)
	if err != nil {
		// TODO(roasbeef): propagate the error here?
		return false
	}

	return t.verifySig(sigHash)
}

// A compile-time assertion to ensure taprootSigVerifier implements the
// signatureVerifier interface.
var _ signatureVerifier = (*taprootSigVerifier)(nil)

// baseTapscriptSigVerifier verifies a signature for an input spending a
// tapscript leaf from the prevoous output.
type baseTapscriptSigVerifier struct {
	*taprootSigVerifier

	vm *Engine
}

// newBaseTapscriptSigVerifier returns a new sig verifier for tapscript input
// spends. If the public key or signature aren't correctly formatted, an error
// is returned.
func newBaseTapscriptSigVerifier(pkBytes, rawSig []byte,
	vm *Engine) (*baseTapscriptSigVerifier, error) {

	switch len(pkBytes) {
	// If the public key is zero bytes, then this is invalid, and will fail
	// immediately.
	case 0:
		return nil, scriptError(ErrTaprootPubkeyIsEmpty, "")

	// If the public key is 32 byte as we expect, then we'll parse things
	// as normal.
	case 32:
		baseTaprootVerifier, err := newTaprootSigVerifier(
			pkBytes, rawSig, &vm.tx, vm.txIdx, vm.prevOutFetcher,
			vm.sigCache, vm.hashCache, vm.taprootCtx.annex,
		)
		if err != nil {
			return nil, err
		}

		return &baseTapscriptSigVerifier{
			taprootSigVerifier: baseTaprootVerifier,
			vm:                 vm,
		}, nil

	// Otherwise, we consider this to be an unknown public key, which means
	// that we'll just assume the sig to be valid.
	default:
		// However, if the flag preventing usage of unknown key types
		// is active, then we'll return that error.
		if vm.hasFlag(ScriptVerifyDiscourageUpgradeablePubkeyType) {
			str := fmt.Sprintf("pubkey of length %v was used",
				len(pkBytes))
			return nil, scriptError(
				ErrDiscourageUpgradeablePubKeyType, str,
			)
		}

		return &baseTapscriptSigVerifier{
			taprootSigVerifier: &taprootSigVerifier{},
		}, nil
	}
}

// Verify returns true if the signature verifier context deems the signature to
// be valid for the given context.
//
// NOTE: This is part of the baseSigVerifier interface.
func (b *baseTapscriptSigVerifier) Verify() bool {
	// If the public key is blank, then that means it wasn't 0 or 32 bytes,
	// so we'll treat this as an unknown public key version and return
	// true.
	if b.pubKey == nil {
		return true
	}

	var opts []TaprootSigHashOption
	opts = append(opts, WithBaseTapscriptVersion(
		b.vm.taprootCtx.codeSepPos, b.vm.taprootCtx.tapLeafHash[:],
	))

	if b.vm.taprootCtx.annex != nil {
		opts = append(opts, WithAnnex(b.vm.taprootCtx.annex))
	}

	// Otherwise, we'll compute the sighash using the tapscript message
	// extensions and return the outcome.
	sigHash, err := calcTaprootSignatureHashRaw(
		b.hashCache, b.hashType, b.tx, b.inputIndex, b.prevOuts,
		opts...,
	)
	if err != nil {
		// TODO(roasbeef): propagate the error here?
		return false
	}

	return b.verifySig(sigHash)
}

// A compile-time assertion to ensure baseTapscriptSigVerifier implements the
// signatureVerifier interface.
var _ signatureVerifier = (*baseTapscriptSigVerifier)(nil)
