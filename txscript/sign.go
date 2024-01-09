// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"errors"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"

	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

// RawTxInWitnessSignature returns the serialized ECDA signature for the input
// idx of the given transaction, with the hashType appended to it. This
// function is identical to RawTxInSignature, however the signature generated
// signs a new sighash digest defined in BIP0143.
func RawTxInWitnessSignature(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	amt int64, subScript []byte, hashType SigHashType,
	key *btcec.PrivateKey) ([]byte, error) {

	hash, err := calcWitnessSignatureHashRaw(subScript, sigHashes, hashType, tx,
		idx, amt)
	if err != nil {
		return nil, err
	}

	signature := ecdsa.Sign(key, hash)

	return append(signature.Serialize(), byte(hashType)), nil
}

// WitnessSignature creates an input witness stack for tx to spend BTC sent
// from a previous output to the owner of privKey using the p2wkh script
// template. The passed transaction must contain all the inputs and outputs as
// dictated by the passed hashType. The signature generated observes the new
// transaction digest algorithm defined within BIP0143.
func WitnessSignature(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int, amt int64,
	subscript []byte, hashType SigHashType, privKey *btcec.PrivateKey,
	compress bool) (wire.TxWitness, error) {

	sig, err := RawTxInWitnessSignature(tx, sigHashes, idx, amt, subscript,
		hashType, privKey)
	if err != nil {
		return nil, err
	}

	pk := privKey.PubKey()
	var pkData []byte
	if compress {
		pkData = pk.SerializeCompressed()
	} else {
		pkData = pk.SerializeUncompressed()
	}

	// A witness script is actually a stack, so we return an array of byte
	// slices here, rather than a single byte slice.
	return wire.TxWitness{sig, pkData}, nil
}

// RawTxInTaprootSignature returns a valid schnorr signature required to
// perform a taproot key-spend of the specified input. If SigHashDefault was
// specified, then the returned signature is 64-byte in length, as it omits the
// additional byte to denote the sighash type.
func RawTxInTaprootSignature(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	amt int64, pkScript []byte, tapScriptRootHash []byte, hashType SigHashType,
	key *btcec.PrivateKey) ([]byte, error) {

	// First, we'll start by compute the top-level taproot sighash.
	sigHash, err := calcTaprootSignatureHashRaw(
		sigHashes, hashType, tx, idx,
		NewCannedPrevOutputFetcher(pkScript, amt),
	)
	if err != nil {
		return nil, err
	}

	// Before we sign the sighash, we'll need to apply the taptweak to the
	// private key based on the tapScriptRootHash.
	privKeyTweak := TweakTaprootPrivKey(*key, tapScriptRootHash)

	// With the sighash constructed, we can sign it with the specified
	// private key.
	signature, err := schnorr.Sign(privKeyTweak, sigHash)
	if err != nil {
		return nil, err
	}

	sig := signature.Serialize()

	// If this is sighash default, then we can just return the signature
	// directly.
	if hashType == SigHashDefault {
		return sig, nil
	}

	// Otherwise, append the sighash type to the final sig.
	return append(sig, byte(hashType)), nil
}

// TaprootWitnessSignature returns a valid witness stack that can be used to
// spend the key-spend path of a taproot input as specified in BIP 342 and BIP
// 86. This method assumes that the public key included in pkScript was
// generated using ComputeTaprootKeyNoScript that commits to a fake root
// tapscript hash. If not, then RawTxInTaprootSignature should be used with the
// actual committed contents.
//
// TODO(roasbeef): add support for annex even tho it's non-standard?
func TaprootWitnessSignature(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	amt int64, pkScript []byte, hashType SigHashType,
	key *btcec.PrivateKey) (wire.TxWitness, error) {

	// As we're assuming this was a BIP 86 key, we use an empty root hash
	// which means output key commits to just the public key.
	fakeTapscriptRootHash := []byte{}

	sig, err := RawTxInTaprootSignature(
		tx, sigHashes, idx, amt, pkScript, fakeTapscriptRootHash,
		hashType, key,
	)
	if err != nil {
		return nil, err
	}

	// The witness script to spend a taproot input using the key-spend path
	// is just the signature itself, given the public key is
	// embedded in the previous output script.
	return wire.TxWitness{sig}, nil
}

// RawTxInTapscriptSignature computes a raw schnorr signature for a signature
// generated from a tapscript leaf. This differs from the
// RawTxInTaprootSignature which is used to generate signatures for top-level
// taproot key spends.
//
// TODO(roasbeef): actually add code-sep to interface? not really used
// anywhere....
func RawTxInTapscriptSignature(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	amt int64, pkScript []byte, tapLeaf TapLeaf, hashType SigHashType,
	privKey *btcec.PrivateKey) ([]byte, error) {

	// First, we'll start by compute the top-level taproot sighash.
	tapLeafHash := tapLeaf.TapHash()
	sigHash, err := calcTaprootSignatureHashRaw(
		sigHashes, hashType, tx, idx,
		NewCannedPrevOutputFetcher(pkScript, amt),
		WithBaseTapscriptVersion(blankCodeSepValue, tapLeafHash[:]),
	)
	if err != nil {
		return nil, err
	}

	// With the sighash constructed, we can sign it with the specified
	// private key.
	signature, err := schnorr.Sign(privKey, sigHash)
	if err != nil {
		return nil, err
	}

	// Finally, append the sighash type to the final sig if it's not the
	// default sighash value (in which case appending it is disallowed).
	if hashType != SigHashDefault {
		return append(signature.Serialize(), byte(hashType)), nil
	}

	// The default sighash case where we'll return _just_ the signature.
	return signature.Serialize(), nil
}

// RawTxInSignature returns the serialized ECDSA signature for the input idx of
// the given transaction, with hashType appended to it.
func RawTxInSignature(tx *wire.MsgTx, idx int, subScript []byte,
	hashType SigHashType, key *btcec.PrivateKey) ([]byte, error) {

	hash, err := CalcSignatureHash(subScript, hashType, tx, idx)
	if err != nil {
		return nil, err
	}
	signature := ecdsa.Sign(key, hash)

	return append(signature.Serialize(), byte(hashType)), nil
}

// SignatureScript creates an input signature script for tx to spend BTC sent
// from a previous output to the owner of privKey. tx must include all
// transaction inputs and outputs, however txin scripts are allowed to be filled
// or empty. The returned script is calculated to be used as the idx'th txin
// sigscript for tx. subscript is the PkScript of the previous output being used
// as the idx'th input. privKey is serialized in either a compressed or
// uncompressed format based on compress. This format must match the same format
// used to generate the payment address, or the script validation will fail.
func SignatureScript(tx *wire.MsgTx, idx int, subscript []byte, hashType SigHashType, privKey *btcec.PrivateKey, compress bool) ([]byte, error) {
	sig, err := RawTxInSignature(tx, idx, subscript, hashType, privKey)
	if err != nil {
		return nil, err
	}

	pk := privKey.PubKey()
	var pkData []byte
	if compress {
		pkData = pk.SerializeCompressed()
	} else {
		pkData = pk.SerializeUncompressed()
	}

	return NewScriptBuilder().AddData(sig).AddData(pkData).Script()
}

func p2pkSignatureScript(tx *wire.MsgTx, idx int, subScript []byte, hashType SigHashType, privKey *btcec.PrivateKey) ([]byte, error) {
	sig, err := RawTxInSignature(tx, idx, subScript, hashType, privKey)
	if err != nil {
		return nil, err
	}

	return NewScriptBuilder().AddData(sig).Script()
}

// signMultiSig signs as many of the outputs in the provided multisig script as
// possible. It returns the generated script and a boolean if the script fulfils
// the contract (i.e. nrequired signatures are provided).  Since it is arguably
// legal to not be able to sign any of the outputs, no error is returned.
func signMultiSig(tx *wire.MsgTx, idx int, subScript []byte, hashType SigHashType,
	addresses []btcutil.Address, nRequired int, kdb KeyDB) ([]byte, bool) {
	// We start with a single OP_FALSE to work around the (now standard)
	// but in the reference implementation that causes a spurious pop at
	// the end of OP_CHECKMULTISIG.
	builder := NewScriptBuilder().AddOp(OP_FALSE)
	signed := 0
	for _, addr := range addresses {
		key, _, err := kdb.GetKey(addr)
		if err != nil {
			continue
		}
		sig, err := RawTxInSignature(tx, idx, subScript, hashType, key)
		if err != nil {
			continue
		}

		builder.AddData(sig)
		signed++
		if signed == nRequired {
			break
		}

	}

	script, _ := builder.Script()
	return script, signed == nRequired
}

func sign(chainParams *chaincfg.Params, tx *wire.MsgTx, idx int,
	subScript []byte, hashType SigHashType, kdb KeyDB, sdb ScriptDB) ([]byte,
	ScriptClass, []btcutil.Address, int, error) {

	class, addresses, nrequired, err := ExtractPkScriptAddrs(subScript,
		chainParams)
	if err != nil {
		return nil, NonStandardTy, nil, 0, err
	}

	switch class {
	case PubKeyTy:
		// look up key for address
		key, _, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		script, err := p2pkSignatureScript(tx, idx, subScript, hashType,
			key)
		if err != nil {
			return nil, class, nil, 0, err
		}

		return script, class, addresses, nrequired, nil
	case PubKeyHashTy:
		// look up key for address
		key, compressed, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		script, err := SignatureScript(tx, idx, subScript, hashType,
			key, compressed)
		if err != nil {
			return nil, class, nil, 0, err
		}

		return script, class, addresses, nrequired, nil
	case ScriptHashTy:
		script, err := sdb.GetScript(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		return script, class, addresses, nrequired, nil
	case MultiSigTy:
		script, _ := signMultiSig(tx, idx, subScript, hashType,
			addresses, nrequired, kdb)
		return script, class, addresses, nrequired, nil
	case NullDataTy:
		return nil, class, nil, 0,
			errors.New("can't sign NULLDATA transactions")
	default:
		return nil, class, nil, 0,
			errors.New("can't sign unknown transactions")
	}
}

// mergeMultiSig combines the two signature scripts sigScript and prevScript
// that both provide signatures for pkScript in output idx of tx. addresses
// and nRequired should be the results from extracting the addresses from
// pkScript. Since this function is internal only we assume that the arguments
// have come from other functions internally and thus are all consistent with
// each other, behaviour is undefined if this contract is broken.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func mergeMultiSig(tx *wire.MsgTx, idx int, addresses []btcutil.Address,
	nRequired int, pkScript, sigScript, prevScript []byte) []byte {

	// Nothing to merge if either the new or previous signature scripts are
	// empty.
	if len(sigScript) == 0 {
		return prevScript
	}
	if len(prevScript) == 0 {
		return sigScript
	}

	// Convenience function to avoid duplication.
	var possibleSigs [][]byte
	extractSigs := func(script []byte) error {
		const scriptVersion = 0
		tokenizer := MakeScriptTokenizer(scriptVersion, script)
		for tokenizer.Next() {
			if data := tokenizer.Data(); len(data) != 0 {
				possibleSigs = append(possibleSigs, data)
			}
		}
		return tokenizer.Err()
	}

	// Attempt to extract signatures from the two scripts.  Return the other
	// script that is intended to be merged in the case signature extraction
	// fails for some reason.
	if err := extractSigs(sigScript); err != nil {
		return prevScript
	}
	if err := extractSigs(prevScript); err != nil {
		return sigScript
	}

	// Now we need to match the signatures to pubkeys, the only real way to
	// do that is to try to verify them all and match it to the pubkey
	// that verifies it. we then can go through the addresses in order
	// to build our script. Anything that doesn't parse or doesn't verify we
	// throw away.
	addrToSig := make(map[string][]byte)
sigLoop:
	for _, sig := range possibleSigs {

		// can't have a valid signature that doesn't at least have a
		// hashtype, in practise it is even longer than this. but
		// that'll be checked next.
		if len(sig) < 1 {
			continue
		}
		tSig := sig[:len(sig)-1]
		hashType := SigHashType(sig[len(sig)-1])

		pSig, err := ecdsa.ParseDERSignature(tSig)
		if err != nil {
			continue
		}

		// We have to do this each round since hash types may vary
		// between signatures and so the hash will vary. We can,
		// however, assume no sigs etc are in the script since that
		// would make the transaction nonstandard and thus not
		// MultiSigTy, so we just need to hash the full thing.
		hash := calcSignatureHash(pkScript, hashType, tx, idx)

		for _, addr := range addresses {
			// All multisig addresses should be pubkey addresses
			// it is an error to call this internal function with
			// bad input.
			pkaddr := addr.(*btcutil.AddressPubKey)

			pubKey := pkaddr.PubKey()

			// If it matches we put it in the map. We only
			// can take one signature per public key so if we
			// already have one, we can throw this away.
			if pSig.Verify(hash, pubKey) {
				aStr := addr.EncodeAddress()
				if _, ok := addrToSig[aStr]; !ok {
					addrToSig[aStr] = sig
				}
				continue sigLoop
			}
		}
	}

	// Extra opcode to handle the extra arg consumed (due to previous bugs
	// in the reference implementation).
	builder := NewScriptBuilder().AddOp(OP_FALSE)
	doneSigs := 0
	// This assumes that addresses are in the same order as in the script.
	for _, addr := range addresses {
		sig, ok := addrToSig[addr.EncodeAddress()]
		if !ok {
			continue
		}
		builder.AddData(sig)
		doneSigs++
		if doneSigs == nRequired {
			break
		}
	}

	// padding for missing ones.
	for i := doneSigs; i < nRequired; i++ {
		builder.AddOp(OP_0)
	}

	script, _ := builder.Script()
	return script
}

// mergeScripts merges sigScript and prevScript assuming they are both
// partial solutions for pkScript spending output idx of tx. class, addresses
// and nrequired are the result of extracting the addresses from pkscript.
// The return value is the best effort merging of the two scripts. Calling this
// function with addresses, class and nrequired that do not match pkScript is
// an error and results in undefined behaviour.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func mergeScripts(chainParams *chaincfg.Params, tx *wire.MsgTx, idx int,
	pkScript []byte, class ScriptClass, addresses []btcutil.Address,
	nRequired int, sigScript, prevScript []byte) []byte {

	// TODO(oga) the scripthash and multisig paths here are overly
	// inefficient in that they will recompute already known data.
	// some internal refactoring could probably make this avoid needless
	// extra calculations.
	const scriptVersion = 0
	switch class {
	case ScriptHashTy:
		// Nothing to merge if either the new or previous signature
		// scripts are empty or fail to parse.
		if len(sigScript) == 0 ||
			checkScriptParses(scriptVersion, sigScript) != nil {

			return prevScript
		}
		if len(prevScript) == 0 ||
			checkScriptParses(scriptVersion, prevScript) != nil {

			return sigScript
		}

		// Remove the last push in the script and then recurse.
		// this could be a lot less inefficient.
		//
		// Assume that final script is the correct one since it was just
		// made and it is a pay-to-script-hash.
		script := finalOpcodeData(scriptVersion, sigScript)

		// We already know this information somewhere up the stack,
		// therefore the error is ignored.
		class, addresses, nrequired, _ :=
			ExtractPkScriptAddrs(script, chainParams)

		// Merge
		mergedScript := mergeScripts(chainParams, tx, idx, script,
			class, addresses, nrequired, sigScript, prevScript)

		// Reappend the script and return the result.
		builder := NewScriptBuilder()
		builder.AddOps(mergedScript)
		builder.AddData(script)
		finalScript, _ := builder.Script()
		return finalScript

	case MultiSigTy:
		return mergeMultiSig(tx, idx, addresses, nRequired, pkScript,
			sigScript, prevScript)

	// It doesn't actually make sense to merge anything other than multiig
	// and scripthash (because it could contain multisig). Everything else
	// has either zero signature, can't be spent, or has a single signature
	// which is either present or not. The other two cases are handled
	// above. In the conflict case here we just assume the longest is
	// correct (this matches behaviour of the reference implementation).
	default:
		if len(sigScript) > len(prevScript) {
			return sigScript
		}
		return prevScript
	}
}

// KeyDB is an interface type provided to SignTxOutput, it encapsulates
// any user state required to get the private keys for an address.
type KeyDB interface {
	GetKey(btcutil.Address) (*btcec.PrivateKey, bool, error)
}

// KeyClosure implements KeyDB with a closure.
type KeyClosure func(btcutil.Address) (*btcec.PrivateKey, bool, error)

// GetKey implements KeyDB by returning the result of calling the closure.
func (kc KeyClosure) GetKey(address btcutil.Address) (*btcec.PrivateKey, bool, error) {
	return kc(address)
}

// ScriptDB is an interface type provided to SignTxOutput, it encapsulates any
// user state required to get the scripts for an pay-to-script-hash address.
type ScriptDB interface {
	GetScript(btcutil.Address) ([]byte, error)
}

// ScriptClosure implements ScriptDB with a closure.
type ScriptClosure func(btcutil.Address) ([]byte, error)

// GetScript implements ScriptDB by returning the result of calling the closure.
func (sc ScriptClosure) GetScript(address btcutil.Address) ([]byte, error) {
	return sc(address)
}

// SignTxOutput signs output idx of the given tx to resolve the script given in
// pkScript with a signature type of hashType. Any keys required will be
// looked up by calling getKey() with the string of the given address.
// Any pay-to-script-hash signatures will be similarly looked up by calling
// getScript. If previousScript is provided then the results in previousScript
// will be merged in a type-dependent manner with the newly generated.
// signature script.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func SignTxOutput(chainParams *chaincfg.Params, tx *wire.MsgTx, idx int,
	pkScript []byte, hashType SigHashType, kdb KeyDB, sdb ScriptDB,
	previousScript []byte) ([]byte, error) {

	sigScript, class, addresses, nrequired, err := sign(chainParams, tx,
		idx, pkScript, hashType, kdb, sdb)
	if err != nil {
		return nil, err
	}

	if class == ScriptHashTy {
		// TODO keep the sub addressed and pass down to merge.
		realSigScript, _, _, _, err := sign(chainParams, tx, idx,
			sigScript, hashType, kdb, sdb)
		if err != nil {
			return nil, err
		}

		// Append the p2sh script as the last push in the script.
		builder := NewScriptBuilder()
		builder.AddOps(realSigScript)
		builder.AddData(sigScript)

		sigScript, _ = builder.Script()
		// TODO keep a copy of the script for merging.
	}

	// Merge scripts. with any previous data, if any.
	mergedScript := mergeScripts(chainParams, tx, idx, pkScript, class,
		addresses, nrequired, sigScript, previousScript)
	return mergedScript, nil
}
