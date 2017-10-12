// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"errors"
	"fmt"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainec"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
)

// RawTxInSignature returns the serialized ECDSA signature for the input idx of
// the given transaction, with hashType appended to it.
func RawTxInSignature(tx *wire.MsgTx, idx int, subScript []byte,
	hashType SigHashType, key chainec.PrivateKey) ([]byte, error) {

	parsedScript, err := parseScript(subScript)
	if err != nil {
		return nil, fmt.Errorf("cannot parse output script: %v", err)
	}
	hash, err := calcSignatureHash(parsedScript, hashType, tx, idx, nil)
	if err != nil {
		return nil, err
	}

	r, s, err := chainec.Secp256k1.Sign(key, hash)
	if err != nil {
		return nil, fmt.Errorf("cannot sign tx input: %s", err)
	}
	sig := chainec.Secp256k1.NewSignature(r, s)

	return append(sig.Serialize(), byte(hashType)), nil
}

// RawTxInSignatureAlt returns the serialized ECDSA signature for the input idx of
// the given transaction, with hashType appended to it.
func RawTxInSignatureAlt(tx *wire.MsgTx, idx int, subScript []byte,
	hashType SigHashType, key chainec.PrivateKey, sigType sigTypes) ([]byte,
	error) {

	parsedScript, err := parseScript(subScript)
	if err != nil {
		return nil, fmt.Errorf("cannot parse output script: %v", err)
	}
	hash, err := calcSignatureHash(parsedScript, hashType, tx, idx, nil)
	if err != nil {
		return nil, err
	}

	var sig chainec.Signature
	switch sigType {
	case edwards:
		r, s, err := chainec.Edwards.Sign(key, hash)
		if err != nil {
			return nil, fmt.Errorf("cannot sign tx input: %s", err)
		}
		sig = chainec.Edwards.NewSignature(r, s)
	case secSchnorr:
		r, s, err := chainec.SecSchnorr.Sign(key, hash)
		if err != nil {
			return nil, fmt.Errorf("cannot sign tx input: %s", err)
		}
		sig = chainec.SecSchnorr.NewSignature(r, s)
	default:
		return nil, fmt.Errorf("unknown alt sig type %v", sigType)
	}

	return append(sig.Serialize(), byte(hashType)), nil
}

// SignatureScript creates an input signature script for tx to spend coins sent
// from a previous output to the owner of privKey. tx must include all
// transaction inputs and outputs, however txin scripts are allowed to be filled
// or empty. The returned script is calculated to be used as the idx'th txin
// sigscript for tx. subscript is the PkScript of the previous output being used
// as the idx'th input. privKey is serialized in either a compressed or
// uncompressed format based on compress. This format must match the same format
// used to generate the payment address, or the script validation will fail.
func SignatureScript(tx *wire.MsgTx, idx int, subscript []byte,
	hashType SigHashType, privKey chainec.PrivateKey, compress bool) ([]byte,
	error) {
	sig, err := RawTxInSignature(tx, idx, subscript, hashType, privKey)
	if err != nil {
		return nil, err
	}

	pubx, puby := privKey.Public()
	pub := chainec.Secp256k1.NewPublicKey(pubx, puby)
	var pkData []byte
	if compress {
		pkData = pub.SerializeCompressed()
	} else {
		pkData = pub.SerializeUncompressed()
	}

	return NewScriptBuilder().AddData(sig).AddData(pkData).Script()
}

// SignatureScriptAlt creates an input signature script for tx to spend coins sent
// from a previous output to the owner of privKey. tx must include all
// transaction inputs and outputs, however txin scripts are allowed to be filled
// or empty. The returned script is calculated to be used as the idx'th txin
// sigscript for tx. subscript is the PkScript of the previous output being used
// as the idx'th input. privKey is serialized in the respective format for the
// ECDSA type. This format must match the same format used to generate the payment
// address, or the script validation will fail.
func SignatureScriptAlt(tx *wire.MsgTx, idx int, subscript []byte,
	hashType SigHashType, privKey chainec.PrivateKey, compress bool,
	sigType int) ([]byte,
	error) {
	sig, err := RawTxInSignatureAlt(tx, idx, subscript, hashType, privKey,
		sigTypes(sigType))
	if err != nil {
		return nil, err
	}

	pubx, puby := privKey.Public()
	var pub chainec.PublicKey
	switch sigTypes(sigType) {
	case edwards:
		pub = chainec.Edwards.NewPublicKey(pubx, puby)
	case secSchnorr:
		pub = chainec.SecSchnorr.NewPublicKey(pubx, puby)
	}
	pkData := pub.Serialize()

	return NewScriptBuilder().AddData(sig).AddData(pkData).Script()
}

// p2pkSignatureScript constructs a pay-to-pubkey signature script.
func p2pkSignatureScript(tx *wire.MsgTx, idx int, subScript []byte,
	hashType SigHashType, privKey chainec.PrivateKey) ([]byte, error) {
	sig, err := RawTxInSignature(tx, idx, subScript, hashType, privKey)
	if err != nil {
		return nil, err
	}

	return NewScriptBuilder().AddData(sig).Script()
}

// p2pkSignatureScript constructs a pay-to-pubkey signature script for alternative
// ECDSA types.
func p2pkSignatureScriptAlt(tx *wire.MsgTx, idx int, subScript []byte,
	hashType SigHashType, privKey chainec.PrivateKey, sigType sigTypes) ([]byte,
	error) {
	sig, err := RawTxInSignatureAlt(tx, idx, subScript, hashType, privKey,
		sigType)
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
	addresses []dcrutil.Address, nRequired int, kdb KeyDB) ([]byte, bool) {
	// No need to add dummy in Decred.
	builder := NewScriptBuilder()
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

// handleStakeOutSign is a convenience function for reducing code clutter in
// sign. It handles the signing of stake outputs.
func handleStakeOutSign(chainParams *chaincfg.Params, tx *wire.MsgTx, idx int,
	subScript []byte, hashType SigHashType, kdb KeyDB, sdb ScriptDB,
	addresses []dcrutil.Address, class ScriptClass, subClass ScriptClass,
	nrequired int) ([]byte, ScriptClass, []dcrutil.Address, int, error) {

	// look up key for address
	switch subClass {
	case PubKeyHashTy:
		key, compressed, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}
		txscript, err := SignatureScript(tx, idx, subScript, hashType,
			key, compressed)
		if err != nil {
			return nil, class, nil, 0, err
		}
		return txscript, class, addresses, nrequired, nil
	case ScriptHashTy:
		script, err := sdb.GetScript(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		return script, class, addresses, nrequired, nil
	}

	return nil, class, nil, 0, fmt.Errorf("unknown subclass for stake output " +
		"to sign")
}

// sign is the main signing workhorse. It takes a script, its input transaction,
// its input index, a database of keys, a database of scripts, and information
// about the type of signature and returns a signature, script class, the
// addresses involved, and the number of signatures required.
func sign(chainParams *chaincfg.Params, tx *wire.MsgTx, idx int,
	subScript []byte, hashType SigHashType, kdb KeyDB, sdb ScriptDB,
	sigType sigTypes) ([]byte,
	ScriptClass, []dcrutil.Address, int, error) {

	class, addresses, nrequired, err := ExtractPkScriptAddrs(DefaultScriptVersion,
		subScript, chainParams)
	if err != nil {
		return nil, NonStandardTy, nil, 0, err
	}

	subClass := class
	isStakeType := class == StakeSubmissionTy ||
		class == StakeSubChangeTy ||
		class == StakeGenTy ||
		class == StakeRevocationTy
	if isStakeType {
		subClass, err = GetStakeOutSubclass(subScript)
		if err != nil {
			return nil, 0, nil, 0,
				fmt.Errorf("unknown stake output subclass encountered")
		}
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

	case PubkeyAltTy:
		// look up key for address
		key, _, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		script, err := p2pkSignatureScriptAlt(tx, idx, subScript, hashType,
			key, sigType)
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

	case PubkeyHashAltTy:
		// look up key for address
		key, compressed, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		script, err := SignatureScriptAlt(tx, idx, subScript, hashType,
			key, compressed, int(sigType))
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

	case StakeSubmissionTy:
		return handleStakeOutSign(chainParams, tx, idx, subScript, hashType, kdb,
			sdb, addresses, class, subClass, nrequired)

	case StakeGenTy:
		return handleStakeOutSign(chainParams, tx, idx, subScript, hashType, kdb,
			sdb, addresses, class, subClass, nrequired)

	case StakeRevocationTy:
		return handleStakeOutSign(chainParams, tx, idx, subScript, hashType, kdb,
			sdb, addresses, class, subClass, nrequired)

	case StakeSubChangeTy:
		return handleStakeOutSign(chainParams, tx, idx, subScript, hashType, kdb,
			sdb, addresses, class, subClass, nrequired)

	case NullDataTy:
		return nil, class, nil, 0,
			errors.New("can't sign NULLDATA transactions")

	default:
		return nil, class, nil, 0,
			errors.New("can't sign unknown transactions")
	}
}

// mergeScripts merges sigScript and prevScript assuming they are both
// partial solutions for pkScript spending output idx of tx. class, addresses
// and nrequired are the result of extracting the addresses from pkscript.
// The return value is the best effort merging of the two scripts. Calling this
// function with addresses, class and nrequired that do not match pkScript is
// an error and results in undefined behaviour.
func mergeScripts(chainParams *chaincfg.Params, tx *wire.MsgTx, idx int,
	pkScript []byte, class ScriptClass, addresses []dcrutil.Address,
	nRequired int, sigScript, prevScript []byte) []byte {

	// TODO(oga) the scripthash and multisig paths here are overly
	// inefficient in that they will recompute already known data.
	// some internal refactoring could probably make this avoid needless
	// extra calculations.
	switch class {
	case ScriptHashTy:
		// Remove the last push in the script and then recurse.
		// this could be a lot less inefficient.
		sigPops, err := parseScript(sigScript)
		if err != nil || len(sigPops) == 0 {
			return prevScript
		}
		prevPops, err := parseScript(prevScript)
		if err != nil || len(prevPops) == 0 {
			return sigScript
		}

		// assume that script in sigPops is the correct one, we just
		// made it.
		script := sigPops[len(sigPops)-1].data

		// We already know this information somewhere up the stack,
		// therefore the error is ignored.
		class, addresses, nrequired, _ :=
			ExtractPkScriptAddrs(DefaultScriptVersion, script, chainParams)

		// regenerate scripts.
		sigScript, _ := unparseScript(sigPops)
		prevScript, _ := unparseScript(prevPops)

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

// mergeMultiSig combines the two signature scripts sigScript and prevScript
// that both provide signatures for pkScript in output idx of tx. addresses
// and nRequired should be the results from extracting the addresses from
// pkScript. Since this function is internal only we assume that the arguments
// have come from other functions internally and thus are all consistent with
// each other, behaviour is undefined if this contract is broken.
func mergeMultiSig(tx *wire.MsgTx, idx int, addresses []dcrutil.Address,
	nRequired int, pkScript, sigScript, prevScript []byte) []byte {

	// This is an internal only function and we already parsed this script
	// as ok for multisig (this is how we got here), so if this fails then
	// all assumptions are broken and who knows which way is up?
	pkPops, _ := parseScript(pkScript)

	sigPops, err := parseScript(sigScript)
	if err != nil || len(sigPops) == 0 {
		return prevScript
	}

	prevPops, err := parseScript(prevScript)
	if err != nil || len(prevPops) == 0 {
		return sigScript
	}

	// Convenience function to avoid duplication.
	extractSigs := func(pops []parsedOpcode, sigs [][]byte) [][]byte {
		for _, pop := range pops {
			if len(pop.data) != 0 {
				sigs = append(sigs, pop.data)
			}
		}
		return sigs
	}

	possibleSigs := make([][]byte, 0, len(sigPops)+len(prevPops))
	possibleSigs = extractSigs(sigPops, possibleSigs)
	possibleSigs = extractSigs(prevPops, possibleSigs)

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

		pSig, err := chainec.Secp256k1.ParseDERSignature(tSig)
		if err != nil {
			continue
		}

		// We have to do this each round since hash types may vary
		// between signatures and so the hash will vary. We can,
		// however, assume no sigs etc are in the script since that
		// would make the transaction nonstandard and thus not
		// MultiSigTy, so we just need to hash the full thing.
		hash, err := calcSignatureHash(pkPops, hashType, tx, idx, nil)
		if err != nil {
			// Decred -- is this the right handling for SIGHASH_SINGLE error ?
			// TODO make sure this doesn't break anything.
			continue
		}

		for _, addr := range addresses {
			// All multisig addresses should be pubkey addreses
			// it is an error to call this internal function with
			// bad input.
			pkaddr := addr.(*dcrutil.AddressSecpPubKey)

			pubKey := pkaddr.PubKey()

			// If it matches we put it in the map. We only
			// can take one signature per public key so if we
			// already have one, we can throw this away.
			r := pSig.GetR()
			s := pSig.GetS()
			if chainec.Secp256k1.Verify(pubKey, hash, r, s) {
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
	builder := NewScriptBuilder() //.AddOp(OP_FALSE)
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

// KeyDB is an interface type provided to SignTxOutput, it encapsulates
// any user state required to get the private keys for an address.
type KeyDB interface {
	GetKey(dcrutil.Address) (chainec.PrivateKey, bool, error)
}

// KeyClosure implements KeyDB with a closure.
type KeyClosure func(dcrutil.Address) (chainec.PrivateKey, bool, error)

// GetKey implements KeyDB by returning the result of calling the closure.
func (kc KeyClosure) GetKey(address dcrutil.Address) (chainec.PrivateKey,
	bool, error) {
	return kc(address)
}

// ScriptDB is an interface type provided to SignTxOutput, it encapsulates any
// user state required to get the scripts for an pay-to-script-hash address.
type ScriptDB interface {
	GetScript(dcrutil.Address) ([]byte, error)
}

// ScriptClosure implements ScriptDB with a closure.
type ScriptClosure func(dcrutil.Address) ([]byte, error)

// GetScript implements ScriptDB by returning the result of calling the closure.
func (sc ScriptClosure) GetScript(address dcrutil.Address) ([]byte, error) {
	return sc(address)
}

// SignTxOutput signs output idx of the given tx to resolve the script given in
// pkScript with a signature type of hashType. Any keys required will be
// looked up by calling getKey() with the string of the given address.
// Any pay-to-script-hash signatures will be similarly looked up by calling
// getScript. If previousScript is provided then the results in previousScript
// will be merged in a type-dependent manner with the newly generated.
// signature script.
func SignTxOutput(chainParams *chaincfg.Params, tx *wire.MsgTx, idx int,
	pkScript []byte, hashType SigHashType, kdb KeyDB, sdb ScriptDB,
	previousScript []byte, sigType int) ([]byte, error) {
	sigScript, class, addresses, nrequired, err := sign(chainParams, tx,
		idx, pkScript, hashType, kdb, sdb, sigTypes(sigType))
	if err != nil {
		return nil, err
	}

	isStakeType := class == StakeSubmissionTy ||
		class == StakeSubChangeTy ||
		class == StakeGenTy ||
		class == StakeRevocationTy
	if isStakeType {
		class, err = GetStakeOutSubclass(pkScript)
		if err != nil {
			return nil, fmt.Errorf("unknown stake output subclass encountered")
		}
	}

	if class == ScriptHashTy {
		// TODO keep the sub addressed and pass down to merge.
		realSigScript, _, _, _, err := sign(chainParams, tx, idx,
			sigScript, hashType, kdb, sdb, sigTypes(sigType))
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
