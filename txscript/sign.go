// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// RawTxInWitnessSignature returns the serialized ECDA signature for the input
// idx of the given transaction, with the hashType appended to it. This
// function is identical to RawTxInSignature, however the signature generated
// signs a new sighash digest defined in BIP0143.
func RawTxInWitnessSignature(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	amt int64, subScript []byte, hashType SigHashType,
	key *btcec.PrivateKey) ([]byte, error) {

	parsedScript, err := parseScript(subScript)
	if err != nil {
		return nil, fmt.Errorf("cannot parse output script: %v", err)
	}

	hash, err := calcWitnessSignatureHash(parsedScript, sigHashes, hashType, tx,
		idx, amt)
	if err != nil {
		return nil, err
	}

	signature, err := key.Sign(hash)
	if err != nil {
		return nil, fmt.Errorf("cannot sign tx input: %s", err)
	}

	return append(signature.Serialize(), byte(hashType)), nil
}

// WitnessSignature creates an input witness stack for tx to spend BTC sent
// from a previous output to the owner of privKey using the p2wkh script
// template. The passed transaction must contain all the inputs and outputs as
// dictated by the passed hashType. The signature generated observes the new
// transaction digest algorithm defined within BIP0143.
func WitnessSignature(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int, amt int64,
	subscript []byte, hashType SigHashType,
	privKey *btcec.PrivateKey, compress bool) (wire.TxWitness, error) {

	stack, err := signP2pkh(tx, sigHashes, idx, amt, subscript, 1,
		hashType, privKey, compress)
	if err != nil {
		return nil, err
	}
	// A witness is actually a stack, so we return an array of byte
	// slices here, rather than a single byte slice.
	return wire.TxWitness(stack), nil
}

// RawTxInSignature returns the serialized ECDSA signature for the input idx of
// the given transaction, with hashType appended to it.
func RawTxInSignature(tx *wire.MsgTx, idx int, subScript []byte,
	hashType SigHashType, key *btcec.PrivateKey) ([]byte, error) {

	hash, err := CalcSignatureHash(subScript, hashType, tx, idx)
	if err != nil {
		return nil, err
	}
	signature, err := key.Sign(hash)
	if err != nil {
		return nil, fmt.Errorf("cannot sign tx input: %s", err)
	}

	return append(signature.Serialize(), byte(hashType)), nil
}

// SignatureScript creates a non-witness input signature script for tx to spend BTC
// sent from a previous output to the owner of privKey. tx must include all
// transaction inputs and outputs, however txin scripts are allowed to be filled
// or empty. The returned script is calculated to be used as the idx'th txin
// sigscript for tx. subscript is the PkScript of the previous output being used
// as the idx'th input. privKey is serialized in either a compressed or
// uncompressed format based on compress. This format must match the same format
// used to generate the payment address, or the script validation will fail.
func SignatureScript(tx *wire.MsgTx, idx int, subscript []byte, hashType SigHashType, privKey *btcec.PrivateKey, compress bool) ([]byte, error) {
	stack, err := signP2pkh(tx, nil, idx, 0, subscript, 0,
		hashType, privKey, compress)
	if err != nil {
		return nil, err
	}
	builder := NewScriptBuilder()
	for i := 0; i < len(stack); i++ {
		builder.AddData(stack[i])
	}
	return builder.Script()
}

// makeSignature calls the appropriate signing function based on sigVersion.
// Since sign accepts more script types than SignTxOutput can handle, we
// check that sigHashes is provided if a witness signature is requested.
// This informs of using the wrong scripts with SignTxOutput, and prevents
// a nil pointer dereference. We require compress here to ensure segwit public
// key encoding policy is followed
func makeSignature(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	amt int64, subscript []byte, sigVersion int, hashType SigHashType,
	privKey *btcec.PrivateKey, compress bool) ([]byte, error) {
	if sigVersion == 1 {
		if !compress {
			return nil, errors.New("cannot use uncompressed keys with segwit")
		}
		if sigHashes == nil {
			return nil, errors.New("cannot sign segwit outputs without TxSigHashes")
		}
		return RawTxInWitnessSignature(tx, sigHashes, idx, amt, subscript, hashType, privKey)
	}
	return RawTxInSignature(tx, idx, subscript, hashType, privKey)
}

// signP2pkh produces a stack for a pay-to-pubkey-hash script, or an error on failure
func signP2pkh(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	amt int64, subscript []byte, sigVersion int, hashType SigHashType,
	privKey *btcec.PrivateKey, compress bool) ([][]byte, error) {
	sig, err := makeSignature(tx, sigHashes, idx, amt, subscript, sigVersion,
		hashType, privKey, compress)
	if err != nil {
		return nil, err
	}
	pk := (*btcec.PublicKey)(&privKey.PublicKey)
	stack := make([][]byte, 2)
	stack[0] = sig
	if compress {
		stack[1] = pk.SerializeCompressed()
	} else {
		stack[1] = pk.SerializeUncompressed()
	}

	return stack, nil
}

// signP2pk produces a stack for a pay-to-pubkey script, or an error on failure
func signP2pk(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	amt int64, subScript []byte, sigVersion int, hashType SigHashType,
	privKey *btcec.PrivateKey, compress bool) ([][]byte, error) {
	sig, err := makeSignature(tx, sigHashes, idx, amt, subScript, sigVersion,
		hashType, privKey, compress)
	if err != nil {
		return nil, err
	}

	stack := make([][]byte, 1)
	stack[0] = sig
	return stack, nil
}

// signMultiSig signs as many of the outputs in the provided multisig script as
// possible. It returns the generated script and a boolean if the script fulfils
// the contract (i.e. nrequired signatures are provided).  Since it is arguably
// legal to not be able to sign any of the outputs, no error is returned.
func signMultiSig(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	amt int64, subScript []byte, sigVersion int, hashType SigHashType,
	addresses []btcutil.Address, nRequired int, kdb KeyDB) ([][]byte, bool, error) {
	// We start with a single zero length push (OP_FALSE) to work around the
	// (now standard) but in the reference implementation that causes a spurious
	// pop at the end of OP_CHECKMULTISIG.
	signed := 0
	stack := make([][]byte, 1)
	stack[0] = nil
	for _, addr := range addresses {
		key, compressed, err := kdb.GetKey(addr)
		if err != nil {
			continue
		}
		sig, err := makeSignature(tx, sigHashes, idx, amt, subScript, sigVersion,
			hashType, key, compressed)
		if err != nil {
			return nil, false, err
		}

		stack = append(stack, sig)
		signed++
		if signed == nRequired {
			break
		}
	}

	return stack, signed == nRequired, nil
}

// sign supports returning the scripts for script-hash types, as well
// as returning a signature stack if the script was P2PK|P2PKH|multisig
// P2WPKH is not directly signed here, the caller must check for that
// and call again requesting the P2PKH signature with segwit sigVersion
// Care must be taken by the caller not to allow incorrectly nesting script types.
func sign(chainParams *chaincfg.Params, tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	amt int64, subScript []byte, sigVersion int, hashType SigHashType,
	kdb KeyDB, sdb ScriptDB) ([][]byte, ScriptClass, []btcutil.Address, int, error) {

	class, addresses, nrequired, err := ExtractPkScriptAddrs(subScript,
		chainParams)
	if err != nil {
		return nil, NonStandardTy, nil, 0, err
	}

	switch class {
	case PubKeyTy:
		// look up key for address
		key, compressed, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		stack, err := signP2pk(tx, sigHashes, idx, amt, subScript, sigVersion,
			hashType, key, compressed)
		if err != nil {
			return nil, class, nil, 0, err
		}

		return stack, class, addresses, nrequired, nil
	case PubKeyHashTy:
		// look up key for address
		key, compressed, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		stack, err := signP2pkh(tx, sigHashes, idx, amt, subScript, sigVersion,
			hashType, key, compressed)
		if err != nil {
			return nil, class, nil, 0, err
		}

		return stack, class, addresses, nrequired, nil
	case WitnessV0PubKeyHashTy:
		return [][]byte{addresses[0].ScriptAddress()}, class, addresses, nrequired, nil
	case ScriptHashTy, WitnessV0ScriptHashTy:
		script, err := sdb.GetScript(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		return [][]byte{script}, class, addresses, nrequired, nil
	case MultiSigTy:
		stack, _, err := signMultiSig(tx, sigHashes, idx, amt, subScript, sigVersion,
			hashType, addresses, nrequired, kdb)
		if err != nil {
			return nil, class, nil, 0, err
		}
		return stack, class, addresses, nrequired, nil
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
func mergeScripts(tx *wire.MsgTx, sigHashes *TxSigHashes, sigVersion int, idx int,
	amt int64, pkScript []byte, class ScriptClass, addresses []btcutil.Address,
	nRequired int, stack [][]byte, prevStack [][]byte) ([][]byte, error) {
	switch class {
	case ScriptHashTy:
		return nil, errors.New("cannot merge " + ScriptHashTy.String() + " type")
	case WitnessV0ScriptHashTy:
		return nil, errors.New("cannot merge " + WitnessV0ScriptHashTy.String() + " type")
	case MultiSigTy:
		return mergeMultiSig(tx, sigHashes, sigVersion, idx, amt, pkScript, addresses, nRequired,
			stack, prevStack)

	// It doesn't actually make sense to merge anything other than multiig
	// and scripthash (because it could contain multisig). Everything else
	// has either zero signature, can't be spent, or has a single signature
	// which is either present or not. The other two cases are handled
	// above. In the conflict case here we just assume the longest is
	// correct (this matches behaviour of the reference implementation).
	default:
		if len(stack) > len(prevStack) {
			return stack, nil
		}
		return prevStack, nil
	}
}

// mergeMultiSig combines the two signature stacks stack and prevStack
// that both provide signatures for pkScript in output idx of tx. addresses
// and nRequired should be the results from extracting the addresses from
// pkScript. Since this function is internal only we assume that the arguments
// have come from other functions internally and thus are all consistent with
// each other, behaviour is undefined if this contract is broken.
func mergeMultiSig(tx *wire.MsgTx, sigHashes *TxSigHashes, sigVersion int, idx int,
	amt int64, pkScript []byte, addresses []btcutil.Address,
	nRequired int, stack [][]byte, prevStack [][]byte) ([][]byte, error) {
	// This is an internal only function and we already parsed this script
	// as ok for multisig (this is how we got here), so if this fails then
	// all assumptions are broken and who knows which way is up?
	pkPops, _ := parseScript(pkScript)

	possibleSigs := make([][]byte, 0, len(stack)+len(prevStack))
	possibleSigs = append(possibleSigs, stack...)
	possibleSigs = append(possibleSigs, prevStack...)

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

		pSig, err := btcec.ParseDERSignature(tSig, btcec.S256())
		if err != nil {
			continue
		}

		// We have to do this each round since hash types may vary
		// between signatures and so the hash will vary. We can,
		// however, assume no sigs etc are in the script since that
		// would make the transaction nonstandard and thus not
		// MultiSigTy, so we just need to hash the full thing.
		var hash []byte
		if sigVersion == 1 {
			var err error
			hash, err = calcWitnessSignatureHash(pkPops, sigHashes, hashType, tx, idx, amt)
			if err != nil {
				return nil, err
			}
		} else {
			hash = calcSignatureHash(pkPops, hashType, tx, idx)
		}

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
	mergedStack := make([][]byte, 0)
	mergedStack = append(mergedStack, []byte{OP_FALSE})
	doneSigs := 0
	// This assumes that addresses are in the same order as in the script.
	for _, addr := range addresses {
		sig, ok := addrToSig[addr.EncodeAddress()]
		if !ok {
			continue
		}
		mergedStack = append(mergedStack, sig)
		doneSigs++
		if doneSigs == nRequired {
			break
		}
	}

	// padding for missing ones.
	for i := doneSigs; i < nRequired; i++ {
		mergedStack = append(mergedStack, []byte{OP_0})
	}

	return mergedStack, nil
}

// KeyDB is an interface type provided to SignTxOutput, it encapsulates
// any user state required to get the private keys for an address.
type KeyDB interface {
	GetKey(btcutil.Address) (*btcec.PrivateKey, bool, error)
}

// KeyClosure implements KeyDB with a closure.
type KeyClosure func(btcutil.Address) (*btcec.PrivateKey, bool, error)

// GetKey implements KeyDB by returning the result of calling the closure.
func (kc KeyClosure) GetKey(address btcutil.Address) (*btcec.PrivateKey,
	bool, error) {
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
// signature script. Because input amt is unknown this function is not compatible
// with segwit outputs.
func SignTxOutput(chainParams *chaincfg.Params, tx *wire.MsgTx, idx int,
	pkScript []byte, hashType SigHashType, kdb KeyDB, sdb ScriptDB,
	previousScript []byte) ([]byte, error) {
	script, _, err := SignTxWitness(chainParams, tx, nil, idx, pkScript, 0,
		hashType, kdb, sdb, previousScript, nil)
	return script, err
}

// SignTxWitness signs output idx of the given tx to resolve the script
// given in pkScript with a signature type of hashType. Any keys required will
// be looked up by calling getKey() with the string of the given address.
// Any pay-to-*-script-hash signatures will be similarly looked up by calling
// getScript. If previousScript / previousWitness is provided then the results
// will be merged in a type-dependent manner with the newly generated.
// signature script & witness
func SignTxWitness(chainParams *chaincfg.Params, tx *wire.MsgTx, sigHashes *TxSigHashes,
	idx int, pkScript []byte, amt int64, hashType SigHashType, kdb KeyDB, sdb ScriptDB,
	previousScript []byte, previousWitness wire.TxWitness) ([]byte, wire.TxWitness, error) {
	sigVersion := 0
	var redeemScript []byte
	var witnessScript []byte
	prevStack, err := readPushOnlyData(previousScript)
	if err != nil {
		return nil, nil, err
	}

	stack, class, addresses, nrequired, err := sign(chainParams, tx, sigHashes,
		idx, amt, pkScript, sigVersion, hashType, kdb, sdb)
	if err != nil {
		return nil, nil, err
	}

	if class == ScriptHashTy {
		redeemScript = stack[0]
		pkScript = stack[0]
		stack, class, addresses, nrequired, err = sign(chainParams, tx, sigHashes,
			idx, amt, redeemScript, sigVersion, hashType, kdb, sdb)
		if err != nil {
			return nil, nil, err
		}
		if class == ScriptHashTy {
			return nil, nil, errors.New("cannot nest P2SH inside P2SH")
		}
		if len(prevStack) > 0 {
			// strip redeemScript from prevStack if present. our stack won't have this
			if bytes.Equal(prevStack[len(prevStack)-1], redeemScript) {
				prevStack = prevStack[:len(prevStack)-1]
			}
		}
	}

	if class == WitnessV0ScriptHashTy {
		sigVersion = 1
		witnessScript = stack[0]
		pkScript = stack[0]
		stack, class, addresses, nrequired, err = sign(chainParams, tx, sigHashes,
			idx, amt, witnessScript, sigVersion, hashType, kdb, sdb)
		if err != nil {
			return nil, nil, err
		}
		if class == ScriptHashTy || class == WitnessV0ScriptHashTy {
			return nil, nil, errors.New("cannot nest P2SH or P2WSH inside P2WSH")
		} else if class == WitnessV0PubKeyHashTy {
			return nil, nil, errors.New("cannot use P2WPKH inside P2WSH")
		}
		if len(previousWitness) > 0 {
			if bytes.Equal(previousWitness[len(previousWitness)-1], witnessScript) {
				// strip witnessScript from prevStack if present. our stack won't have this
				previousWitness = previousWitness[:len(previousWitness)-1]
			}
		}
	} else if class == WitnessV0PubKeyHashTy {
		sigVersion = 1
		key, compressed, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, nil, err
		}
		// call by hand as sign would search using AddressPubKeyHash, and
		// not AddressWitnessPubKeyHash which we desire
		stack, err = signP2pkh(tx, sigHashes, idx, amt, pkScript, sigVersion,
			hashType, key, compressed)
		if err != nil {
			return nil, nil, err
		}
	}

	// Merge scripts. with any previous data, if any.
	var scriptStack [][]byte
	var witness wire.TxWitness
	if sigVersion == 1 {
		witness, err = mergeScripts(tx, sigHashes, sigVersion, idx, amt, pkScript, class,
			addresses, nrequired, stack, previousWitness)
	} else {
		scriptStack, err = mergeScripts(tx, sigHashes, sigVersion, idx, amt, pkScript, class,
			addresses, nrequired, stack, prevStack)
	}
	if err != nil {
		return nil, nil, err
	}

	// Build final scriptSig and witness
	builder := NewScriptBuilder()
	for i := 0; i < len(scriptStack); i++ {
		builder.AddData(scriptStack[i])
	}
	if redeemScript != nil {
		builder.AddData(redeemScript)
	}
	scriptSig, err := builder.Script()
	if err != nil {
		return nil, nil, err
	}
	if witnessScript != nil {
		witness = append(witness, witnessScript)
	}

	return scriptSig, witness, nil
}

func readPushOnlyData(s []byte) ([][]byte, error) {
	sigPops, err := parseScript(s)
	if err != nil {
		return nil, err
	}

	// Push only sigScript makes little sense.
	// Can't have a signature script that doesn't just push data.
	if !isPushOnly(sigPops) {
		return nil, scriptError(ErrNotPushOnly,
			"signature script is not push only")
	}

	data := make([][]byte, len(sigPops))
	for i, pop := range sigPops {
		data[i] = pop.data
	}
	return data, nil
}
