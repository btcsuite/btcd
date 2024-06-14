// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

// The Finalizer requires provision of a single PSBT input
// in which all necessary signatures are encoded, and
// uses it to construct valid final sigScript and scriptWitness
// fields.
// NOTE that p2sh (legacy) and p2wsh currently support only
// multisig and no other custom script.

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"slices"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

// bip328SyntheticChainCode is the fixed chain code that BIP-328 attaches to the
// synthetic extended public key of a MuSig2 plain aggregate public key. It is
// the SHA256 of the text "MuSig2MuSig2MuSig2".
var bip328SyntheticChainCode = [32]byte{
	0x86, 0x80, 0x87, 0xca, 0x02, 0xa6, 0xf9, 0x74,
	0xc4, 0x59, 0x89, 0x24, 0xc3, 0x6b, 0x57, 0x76,
	0x2d, 0x32, 0xcb, 0x45, 0x71, 0x71, 0x67, 0xe3,
	0x00, 0x62, 0x2c, 0x71, 0x67, 0xe3, 0x89, 0x65,
}

// isFinalized considers this input finalized if it contains at least one of
// the FinalScriptSig or FinalScriptWitness are filled (which only occurs in a
// successful call to Finalize*).
func isFinalized(p *Packet, inIndex int) bool {
	input := p.Inputs[inIndex]
	return input.FinalScriptSig != nil || input.FinalScriptWitness != nil
}

// isFinalizableWitnessInput returns true if the target input is a witness UTXO
// that can be finalized.
func isFinalizableWitnessInput(pInput *PInput) bool {
	pkScript := pInput.WitnessUtxo.PkScript

	switch {
	// If this is a native witness output, then we require both
	// the witness script, but not a redeem script.
	case txscript.IsWitnessProgram(pkScript):
		switch {
		case txscript.IsPayToWitnessScriptHash(pkScript):
			if pInput.WitnessScript == nil ||
				pInput.RedeemScript != nil {

				return false
			}

		case txscript.IsPayToTaproot(pkScript):
			if pInput.TaprootKeySpendSig == nil &&
				pInput.TaprootScriptSpendSig == nil &&
				pInput.MuSig2PartialSigs == nil {

				return false
			}

			// MuSig2 partial sigs are only useful once at least one
			// aggregate key and spend path has a matching nonce and
			// partial signature for every participant. The
			// pre-aggregated TaprootKeySpendSig /
			// TaprootScriptSpendSig branch above is preferred when
			// either is also present.
			if len(pInput.MuSig2PartialSigs) > 0 &&
				pInput.TaprootKeySpendSig == nil &&
				pInput.TaprootScriptSpendSig == nil {

				return musig2InputReady(pInput)
			}

			// For each of the script spend signatures we need a
			// corresponding tap script leaf with the control block.
			for _, sig := range pInput.TaprootScriptSpendSig {
				if sig == nil {
					return false
				}

				_, err := FindLeafScript(pInput, sig.LeafHash)
				if err != nil {
					return false
				}
			}

		default:
			// A P2WKH output on the other hand doesn't need
			// neither a witnessScript or redeemScript.
			if pInput.WitnessScript != nil ||
				pInput.RedeemScript != nil {

				return false
			}
		}

	// For nested P2SH inputs, we verify that a witness script is known.
	case txscript.IsPayToScriptHash(pkScript):
		if pInput.RedeemScript == nil {
			return false
		}

		// If this is a nested P2SH input, then it must also have a
		// witness script, while we don't need one for P2WKH.
		if txscript.IsPayToWitnessScriptHash(pInput.RedeemScript) {
			if pInput.WitnessScript == nil {
				return false
			}
		} else if txscript.IsPayToWitnessPubKeyHash(pInput.RedeemScript) {
			if pInput.WitnessScript != nil {
				return false
			}
		} else {
			// unrecognized type
			return false
		}

	// If this isn't a nested P2SH output or a native witness output, then
	// we can't finalize this input as we don't understand it.
	default:
		return false
	}

	return true
}

// isFinalizableLegacyInput returns true of the passed input a legacy input
// (non-witness) that can be finalized.
func isFinalizableLegacyInput(p *Packet, pInput *PInput, inIndex int) bool {
	// If the input has a witness, then it's invalid.
	if pInput.WitnessScript != nil {
		return false
	}

	// Otherwise, we'll verify that we only have a RedeemScript if the prev
	// output script is P2SH.
	outIndex := p.UnsignedTx.TxIn[inIndex].PreviousOutPoint.Index
	if txscript.IsPayToScriptHash(pInput.NonWitnessUtxo.TxOut[outIndex].PkScript) {
		if pInput.RedeemScript == nil {
			return false
		}
	} else {
		if pInput.RedeemScript != nil {
			return false
		}
	}

	return true
}

// isFinalizable checks whether the structure of the entry for the input of the
// psbt.Packet at index inIndex contains sufficient information to finalize
// this input.
func isFinalizable(p *Packet, inIndex int) bool {
	pInput := p.Inputs[inIndex]

	// The input cannot be finalized without any signatures.
	if pInput.PartialSigs == nil && pInput.TaprootKeySpendSig == nil &&
		pInput.TaprootScriptSpendSig == nil &&
		pInput.MuSig2PartialSigs == nil {

		return false
	}

	// For an input to be finalized, we'll one of two possible top-level
	// UTXOs present. Each UTXO type has a distinct set of requirements to
	// be considered finalized.
	switch {

	// A witness input must be either native P2WSH or nested P2SH with all
	// relevant sigScript or witness data populated.
	case pInput.WitnessUtxo != nil:
		if !isFinalizableWitnessInput(&pInput) {
			return false
		}

	case pInput.NonWitnessUtxo != nil:
		if !isFinalizableLegacyInput(p, &pInput, inIndex) {
			return false
		}

	// If neither a known UTXO type isn't present at all, then we'll
	// return false as we need one of them.
	default:
		return false
	}

	return true
}

// MaybeFinalize attempts to finalize the input at index inIndex in the PSBT p,
// returning true with no error if it succeeds, OR if the input has already
// been finalized.
func MaybeFinalize(p *Packet, inIndex int) (bool, error) {
	if isFinalized(p, inIndex) {
		return true, nil
	}

	if !isFinalizable(p, inIndex) {
		return false, ErrNotFinalizable
	}

	if err := Finalize(p, inIndex); err != nil {
		return false, err
	}

	return true, nil
}

// MaybeFinalizeAll attempts to finalize all inputs of the psbt.Packet that are
// not already finalized, and returns an error if it fails to do so.
func MaybeFinalizeAll(p *Packet) error {
	for i := range p.UnsignedTx.TxIn {
		success, err := MaybeFinalize(p, i)
		if err != nil || !success {
			return err
		}
	}

	return nil
}

// Finalize assumes that the provided psbt.Packet struct has all partial
// signatures and redeem scripts/witness scripts already prepared for the
// specified input, and so removes all temporary data and replaces them with
// completed sigScript and witness fields, which are stored in key-types 07 and
// 08. The witness/non-witness utxo fields in the inputs (key-types 00 and 01)
// are left intact as they may be needed for validation (?).  If there is any
// invalid or incomplete data, an error is returned.
func Finalize(p *Packet, inIndex int) error {
	pInput := p.Inputs[inIndex]

	// Depending on the UTXO type, we either attempt to finalize it as a
	// witness or legacy UTXO.
	switch {
	case pInput.WitnessUtxo != nil:
		pkScript := pInput.WitnessUtxo.PkScript

		switch {
		case txscript.IsPayToTaproot(pkScript):
			if err := finalizeTaprootInput(p, inIndex); err != nil {
				return err
			}

		default:
			if err := finalizeWitnessInput(p, inIndex); err != nil {
				return err
			}
		}

	case pInput.NonWitnessUtxo != nil:
		if err := finalizeNonWitnessInput(p, inIndex); err != nil {
			return err
		}

	default:
		return ErrInvalidPsbtFormat
	}

	// Before returning we sanity check the PSBT to ensure we don't extract
	// an invalid transaction or produce an invalid intermediate state.
	if err := p.SanityCheck(); err != nil {
		return err
	}

	return nil
}

// checkFinalScriptSigWitness checks whether a given input in the psbt.Packet
// struct already has the fields 07 (FinalInScriptSig) or 08 (FinalInWitness).
// If so, it returns true. It does not modify the Psbt.
func checkFinalScriptSigWitness(p *Packet, inIndex int) bool {
	pInput := p.Inputs[inIndex]

	if pInput.FinalScriptSig != nil {
		return true
	}

	if pInput.FinalScriptWitness != nil {
		return true
	}

	return false
}

// finalizeNonWitnessInput attempts to create a PsbtInFinalScriptSig field for
// the input at index inIndex, and removes all other fields except for the UTXO
// field, for an input of type non-witness, or returns an error.
func finalizeNonWitnessInput(p *Packet, inIndex int) error {
	// If this input has already been finalized, then we'll return an error
	// as we can't proceed.
	if checkFinalScriptSigWitness(p, inIndex) {
		return ErrInputAlreadyFinalized
	}

	// Our goal here is to construct a sigScript given the pubkey,
	// signature (keytype 02), of which there might be multiple, and the
	// redeem script field (keytype 04) if present (note, it is not present
	// for p2pkh type inputs).
	var sigScript []byte

	pInput := p.Inputs[inIndex]
	containsRedeemScript := pInput.RedeemScript != nil

	var (
		pubKeys [][]byte
		sigs    [][]byte
	)
	for _, ps := range pInput.PartialSigs {
		pubKeys = append(pubKeys, ps.PubKey)

		sigOK := checkSigHashFlags(ps.Signature, &pInput)
		if !sigOK {
			return ErrInvalidSigHashFlags
		}

		sigs = append(sigs, ps.Signature)
	}

	// We have failed to identify at least 1 (sig, pub) pair in the PSBT,
	// which indicates it was not ready to be finalized. As a result, we
	// can't proceed.
	if len(sigs) < 1 || len(pubKeys) < 1 {
		return ErrNotFinalizable
	}

	// If this input doesn't need a redeem script (P2PKH), then we'll
	// construct a simple sigScript that's just the signature then the
	// pubkey (OP_CHECKSIG).
	var err error
	if !containsRedeemScript {
		// At this point, we should only have a single signature and
		// pubkey.
		if len(sigs) != 1 || len(pubKeys) != 1 {
			return ErrNotFinalizable
		}

		// In this case, our sigScript is just: <sig> <pubkey>.
		builder := txscript.NewScriptBuilder()
		builder.AddData(sigs[0]).AddData(pubKeys[0])
		sigScript, err = builder.Script()
		if err != nil {
			return err
		}
	} else {
		// This is assumed p2sh multisig Given redeemScript and pubKeys
		// we can decide in what order signatures must be appended.
		orderedSigs, err := extractKeyOrderFromScript(
			pInput.RedeemScript, pubKeys, sigs,
		)
		if err != nil {
			return err
		}

		// At this point, we assume that this is a mult-sig input, so
		// we construct our sigScript which looks something like this
		// (mind the extra element for the extra multi-sig pop):
		//  * <nil> <sigs...> <redeemScript>
		//
		// TODO(waxwing): the below is specific to the multisig case.
		builder := txscript.NewScriptBuilder()
		builder.AddOp(txscript.OP_FALSE)
		for _, os := range orderedSigs {
			builder.AddData(os)
		}
		builder.AddData(pInput.RedeemScript)
		sigScript, err = builder.Script()
		if err != nil {
			return err
		}
	}

	// At this point, a sigScript has been constructed.  Remove all fields
	// other than non-witness utxo (00) and finaliscriptsig (07)
	newInput := NewPsbtInput(pInput.NonWitnessUtxo, nil)
	newInput.FinalScriptSig = sigScript

	// Overwrite the entry in the input list at the correct index. Note
	// that this removes all the other entries in the list for this input
	// index.
	p.Inputs[inIndex] = *newInput

	return nil
}

// finalizeWitnessInput attempts to create PsbtInFinalScriptSig field and
// PsbtInFinalScriptWitness field for input at index inIndex, and removes all
// other fields except for the utxo field, for an input of type witness, or
// returns an error.
func finalizeWitnessInput(p *Packet, inIndex int) error {
	// If this input has already been finalized, then we'll return an error
	// as we can't proceed.
	if checkFinalScriptSigWitness(p, inIndex) {
		return ErrInputAlreadyFinalized
	}

	// Depending on the actual output type, we'll either populate a
	// serializedWitness or a witness as well asa sigScript.
	var (
		sigScript         []byte
		serializedWitness []byte
	)

	pInput := p.Inputs[inIndex]

	// First we'll validate and collect the pubkey+sig pairs from the set
	// of partial signatures.
	var (
		pubKeys [][]byte
		sigs    [][]byte
	)
	for _, ps := range pInput.PartialSigs {
		pubKeys = append(pubKeys, ps.PubKey)

		sigOK := checkSigHashFlags(ps.Signature, &pInput)
		if !sigOK {
			return ErrInvalidSigHashFlags

		}

		sigs = append(sigs, ps.Signature)
	}

	// If at this point, we don't have any pubkey+sig pairs, then we bail
	// as we can't proceed.
	if len(sigs) == 0 || len(pubKeys) == 0 {
		return ErrNotFinalizable
	}

	containsRedeemScript := pInput.RedeemScript != nil
	containsWitnessScript := pInput.WitnessScript != nil

	// If there's no redeem script, then we assume that this is native
	// segwit input.
	var err error
	if !containsRedeemScript {
		// If we have only a sigley pubkey+sig pair, and no witness
		// script, then we assume this is a P2WKH input.
		if len(pubKeys) == 1 && len(sigs) == 1 &&
			!containsWitnessScript {

			serializedWitness, err = writePKHWitness(
				sigs[0], pubKeys[0],
			)
			if err != nil {
				return err
			}
		} else {
			// Otherwise, we must have a witnessScript field, so
			// we'll generate a valid multi-sig witness.
			//
			// NOTE: We tacitly assume multisig.
			//
			// TODO(roasbeef): need to add custom finalize for
			// non-multisig P2WSH outputs (HTLCs, delay outputs,
			// etc).
			if !containsWitnessScript {
				return ErrNotFinalizable
			}

			serializedWitness, err = getMultisigScriptWitness(
				pInput.WitnessScript, pubKeys, sigs,
			)
			if err != nil {
				return err
			}
		}
	} else {
		// Otherwise, we assume that this is a p2wsh multi-sig output,
		// which is nested in a p2sh, or a p2wkh nested in a p2sh.
		//
		// In this case, we'll take the redeem script (the witness
		// program in this case), and push it on the stack within the
		// sigScript.
		builder := txscript.NewScriptBuilder()
		builder.AddData(pInput.RedeemScript)
		sigScript, err = builder.Script()
		if err != nil {
			return err
		}

		// If don't have a witness script, then we assume this is a
		// nested p2wkh output.
		if !containsWitnessScript {
			// Assumed p2sh-p2wkh Here the witness is just (sig,
			// pub) as for p2pkh case
			if len(sigs) != 1 || len(pubKeys) != 1 {
				return ErrNotFinalizable
			}

			serializedWitness, err = writePKHWitness(
				sigs[0], pubKeys[0],
			)
			if err != nil {
				return err
			}

		} else {
			// Otherwise, we assume that this is a p2wsh multi-sig,
			// so we generate the proper witness.
			serializedWitness, err = getMultisigScriptWitness(
				pInput.WitnessScript, pubKeys, sigs,
			)
			if err != nil {
				return err
			}
		}
	}

	// At this point, a witness has been constructed, and a sigScript (if
	// nested; else it's []). Remove all fields other than witness utxo
	// (01) and finalscriptsig (07), finalscriptwitness (08).
	newInput := NewPsbtInput(nil, pInput.WitnessUtxo)
	if len(sigScript) > 0 {
		newInput.FinalScriptSig = sigScript
	}

	newInput.FinalScriptWitness = serializedWitness

	// Finally, we overwrite the entry in the input list at the correct
	// index.
	p.Inputs[inIndex] = *newInput
	return nil
}

// finalizeTaprootInput attempts to create PsbtInFinalScriptWitness field for
// input at index inIndex, and removes all other fields except for the utxo
// field, for an input of type p2tr, or returns an error.
func finalizeTaprootInput(p *Packet, inIndex int) error {
	// If this input has already been finalized, then we'll return an error
	// as we can't proceed.
	if checkFinalScriptSigWitness(p, inIndex) {
		return ErrInputAlreadyFinalized
	}

	// Any p2tr input will only have a witness script, no sig script.
	var (
		serializedWitness []byte
		err               error
		pInput            = &p.Inputs[inIndex]
	)

	for idx, scriptSpendSig := range pInput.TaprootScriptSpendSig {
		if scriptSpendSig == nil {
			return fmt.Errorf("nil taproot script spend signature "+
				"at index %d: %w", idx, ErrInvalidPsbtFormat)
		}
	}

	// What spend path did we take?
	switch {
	// Key spend path.
	case len(pInput.TaprootKeySpendSig) > 0:
		sig := pInput.TaprootKeySpendSig

		// Make sure TaprootKeySpendSig is equal to size of signature,
		// if not, we assume that sighash flag was appended to the
		// signature.
		if len(pInput.TaprootKeySpendSig) == schnorr.SignatureSize {
			// Append to the signature if flag is not equal to the
			// default sighash (that can be omitted).
			if pInput.SighashType != txscript.SigHashDefault {
				sigHashType := byte(pInput.SighashType)
				sig = append(sig, sigHashType)
			}
		}
		serializedWitness, err = writeWitness(sig)

	// Script spend path.
	case len(pInput.TaprootScriptSpendSig) > 0:
		var witnessStack wire.TxWitness

		// If there are multiple script spend signatures, we assume they
		// are from multiple signing participants for the same leaf
		// script that uses OP_CHECKSIGADD for multi-sig. Signing
		// multiple possible execution paths at the same time is
		// currently not supported by this library.
		targetLeafHash := pInput.TaprootScriptSpendSig[0].LeafHash
		leafScript, err := FindLeafScript(pInput, targetLeafHash)
		if err != nil {
			return fmt.Errorf("control block for script spend "+
				"signature not found: %w", err)
		}

		// The witness stack will contain all signatures, followed by
		// the script itself and then the control block.
		for idx, scriptSpendSig := range pInput.TaprootScriptSpendSig {
			// Make sure that if there are indeed multiple
			// signatures, they all reference the same leaf hash.
			if !bytes.Equal(
				scriptSpendSig.LeafHash, targetLeafHash,
			) {

				return fmt.Errorf("script spend signature %d "+
					"references different target leaf "+
					"hash than first signature; only one "+
					"script path is supported", idx)
			}

			sig := append([]byte{}, scriptSpendSig.Signature...)
			if scriptSpendSig.SigHash != txscript.SigHashDefault {
				sig = append(sig, byte(scriptSpendSig.SigHash))
			}
			witnessStack = append(witnessStack, sig)
		}

		// Complete the witness stack with the executed script and the
		// serialized control block.
		witnessStack = append(witnessStack, leafScript.Script)
		witnessStack = append(witnessStack, leafScript.ControlBlock)

		serializedWitness, err = writeWitness(witnessStack...)

	// MuSig2 spend path. An input may carry several independent signing
	// contexts, so the spend path is derived from whichever of them turns
	// out to be complete and combinable rather than from any single field.
	case len(pInput.MuSig2PartialSigs) > 0:
		serializedWitness, err = finalizeMuSig2Input(p, inIndex)

	default:
		return ErrInvalidPsbtFormat
	}
	if err != nil {
		return err
	}

	// At this point, a witness has been constructed. Remove all fields
	// other than witness utxo (01) and finalscriptsig (07),
	// finalscriptwitness (08).
	newInput := NewPsbtInput(nil, pInput.WitnessUtxo)
	newInput.FinalScriptWitness = serializedWitness

	// Finally, we overwrite the entry in the input list at the correct
	// index.
	p.Inputs[inIndex] = *newInput
	return nil
}

// finalizeMuSig2Input builds the witness for an input whose signatures come
// from a MuSig2 session.
//
// Each complete signing context on the input is tried in turn, and the first
// one that combines into a signature valid under the key it will be checked
// against wins. Trying them is safe precisely because combineMuSig2Sig verifies
// its result: a context that belongs to a spend path this input cannot take
// fails there instead of producing a witness that the script engine rejects.
func finalizeMuSig2Input(p *Packet, inIndex int) ([]byte, error) {
	pInput := &p.Inputs[inIndex]

	contexts := muSig2SigningContexts(pInput)
	if len(contexts) == 0 {
		return nil, fmt.Errorf("no complete MuSig2 signing context " +
			"on input: every aggregate key and tap leaf hash is " +
			"missing either a nonce or a partial signature")
	}

	var lastErr error
	for _, set := range contexts {
		var (
			serializedWitness []byte
			err               error
		)
		if len(set.tapLeafHash) > 0 {
			serializedWitness, err = finalizeMuSig2ScriptSpend(
				p, inIndex, set,
			)
		} else {
			serializedWitness, err = finalizeMuSig2KeySpend(
				p, inIndex, set,
			)
		}
		if err != nil {
			lastErr = err
			continue
		}

		return serializedWitness, nil
	}

	return nil, lastErr
}

// finalizeMuSig2KeySpend handles BIP-373 test vector cases 1, 2 and 4: a
// top-level taproot key spend where the aggregate MuSig2 key is the output key
// directly (no tweak), the internal key (BIP-86 or merkle-root taproot tweak),
// or a parent the internal key was BIP-32 derived from. Returns the serialized
// witness containing the aggregated BIP-340 Schnorr signature.
func finalizeMuSig2KeySpend(p *Packet, inIndex int,
	set *muSig2SigningSet) ([]byte, error) {

	pInput := &p.Inputs[inIndex]

	// The signature will be checked against the taproot output key of the
	// UTXO we are spending, so that is what we have to verify it against.
	outputKey, err := taprootOutputKey(pInput.WitnessUtxo.PkScript)
	if err != nil {
		return nil, err
	}

	prevOutFetcher, err := PrevOutputFetcher(p)
	if err != nil {
		return nil, fmt.Errorf("error making prev out fetcher: %w", err)
	}

	sigHashes := txscript.NewTxSigHashes(p.UnsignedTx, prevOutFetcher)
	sigHash, err := txscript.CalcTaprootSignatureHash(
		sigHashes, pInput.SighashType, p.UnsignedTx, inIndex,
		prevOutFetcher,
	)
	if err != nil {
		return nil, fmt.Errorf("error calculating signature hash: %w",
			err)
	}

	var sigHashMsg [32]byte
	copy(sigHashMsg[:], sigHash)

	orderedSet, keyAggOpts, combineOpts, err := selectMuSig2Tweaks(
		p, pInput, set, sigHashMsg, muSig2TweaksForKeySpend,
	)
	if err != nil {
		return nil, err
	}

	schnorrSig, err := combineMuSig2Sig(
		sigHashMsg, orderedSet, keyAggOpts, combineOpts, outputKey,
	)
	if err != nil {
		return nil, err
	}

	sig := appendSighashType(schnorrSig.Serialize(), pInput.SighashType)
	return writeWitness(sig)
}

// taprootOutputKey returns the 32-byte x-only taproot output key committed to
// by the given P2TR pkScript.
func taprootOutputKey(pkScript []byte) ([]byte, error) {
	if !txscript.IsPayToTaproot(pkScript) {
		return nil, fmt.Errorf("prev output script is not a taproot "+
			"output: %x", pkScript)
	}

	// A P2TR script is OP_1 followed by a 32 byte data push.
	return pkScript[2:], nil
}

// muSig2TweakSelector infers the tweaks that were applied at sign time for one
// candidate plain aggregate key, given the signing set ordered the way that
// candidate's participants record requires. See muSig2TweaksForKeySpend and
// muSig2TweaksForScriptSpend for the two implementations.
type muSig2TweakSelector func(p *Packet, pInput *PInput,
	bareAggregate *btcec.PublicKey, set *muSig2SigningSet,
	sigHashMsg [32]byte) ([]musig2.KeyAggOption, []musig2.CombineOption,
	error)

// selectMuSig2Tweaks decides which (if any) tweaks the finalizer must apply
// when combining an input's MuSig2 partial signatures, and returns the signing
// set in the participant order those tweaks were chosen for.
//
// The aggregate key in the partial signature keydata is the key found in the
// script, which may be the result of tweaking or deriving one of the plain
// aggregate keys recorded in PSBT_IN_MUSIG2_PARTICIPANT_PUBKEYS. Since that
// field is keyed by the aggregate pubkey, an input may carry a record for more
// than one aggregate key, and the order of the records says nothing about which
// one the signatures belong to. Each record is therefore tried in turn, both as
// the bare aggregate and as the participant order to aggregate in.
//
// A record's tweaks are only accepted once they are shown to actually reproduce
// the key in the script, so a record for an unrelated aggregate key (or in an
// order the signers did not use) is skipped rather than silently producing a
// signature that cannot be verified.
func selectMuSig2Tweaks(p *Packet, pInput *PInput, set *muSig2SigningSet,
	sigHashMsg [32]byte, tweaksFor muSig2TweakSelector) (*muSig2SigningSet,
	[]musig2.KeyAggOption, []musig2.CombineOption, error) {

	// muSig2Candidate pairs a plain aggregate key recorded on the input
	// with the signing set ordered the way that record requires.
	type muSig2Candidate struct {
		bareAggregate *btcec.PublicKey
		set           *muSig2SigningSet
	}

	var (
		candidates []muSig2Candidate
		lastErr    error
	)
	for _, participants := range pInput.MuSig2Participants {
		ordered, err := set.orderedBy(participants.Keys)
		if err != nil {
			lastErr = err
			continue
		}

		candidates = append(candidates, muSig2Candidate{
			bareAggregate: participants.AggregateKey,
			set:           ordered,
		})
	}

	// If MuSig2Participants is missing entirely we have neither a bare
	// aggregate nor an authoritative participant order. We then fall back
	// to treating the partial sig aggregate as the bare aggregate and to
	// the order the nonces appear in with sorting enabled, which is what a
	// session using KeySort would have produced. This mirrors the behavior
	// of a PSBT in which an updater only set the partial sigs.
	if len(pInput.MuSig2Participants) == 0 {
		candidates = append(candidates, muSig2Candidate{
			bareAggregate: set.aggregateKey,
			set:           set,
		})
	}

	// Keep the first candidate whose tweak chain actually reproduces the
	// key in the script. If none of them do, we report the error of the
	// last one, which is the most specific diagnostic we have for the
	// common case of a single participants record.
	for _, candidate := range candidates {
		keyAggOpts, combineOpts, err := tweaksFor(
			p, pInput, candidate.bareAggregate, candidate.set,
			sigHashMsg,
		)
		if err != nil {
			lastErr = err
			continue
		}

		// Confirm the tweaks turn the participant keys into the key the
		// signers actually signed for. This rules out a participants
		// record belonging to a different aggregate key, a record whose
		// order the signers did not aggregate in, and a tweak chain we
		// inferred incorrectly, all before we hand out a signature that
		// cannot be verified.
		match, err := muSig2TweaksMatch(candidate.set, keyAggOpts)
		if err != nil {
			lastErr = err
			continue
		}
		if !match {
			lastErr = fmt.Errorf("MuSig2 finalize: aggregate key "+
				"%x does not produce the key in the script "+
				"(%x) under the inferred tweaks",
				candidate.bareAggregate.SerializeCompressed(),
				set.aggregateKey.SerializeCompressed())
			continue
		}

		return candidate.set, keyAggOpts, combineOpts, nil
	}

	return nil, nil, nil, lastErr
}

// muSig2TweaksMatch reports whether aggregating the signing set's participant
// keys under the given tweaks yields the key found in the script. Only the x
// coordinate is compared, as that is all a taproot script commits to.
func muSig2TweaksMatch(set *muSig2SigningSet,
	keyAggOpts []musig2.KeyAggOption) (bool, error) {

	aggKey, _, _, err := musig2.AggregateKeys(
		set.keysForAggregation(), set.sortKeys, keyAggOpts...,
	)
	if err != nil {
		return false, fmt.Errorf("error aggregating keys: %w", err)
	}

	return bytes.Equal(
		schnorr.SerializePubKey(aggKey.FinalKey),
		schnorr.SerializePubKey(set.aggregateKey),
	), nil
}

// muSig2TweaksForKeySpend returns the key aggregation and combine options
// needed to turn the given plain aggregate key into the taproot output key of a
// key spend, comparing the candidate against the key in the script:
//
//   - Equal → no tweak was applied at sign time (BIP-373 test vector case 1:
//     the output key IS the aggregate).
//   - Differ + TaprootInternalKey == bare aggregate → BIP-86 tweak (or taproot
//     tweak with the merkle root, if present). BIP-373 test vector case 2.
//   - Differ + TaprootInternalKey ≠ bare aggregate → the internal key was
//     derived from the aggregate via BIP-32, so the derivation tweaks come
//     first and the taproot tweak is computed over the derived internal key.
//     BIP-373 test vector case 4.
func muSig2TweaksForKeySpend(p *Packet, pInput *PInput,
	bareAggregate *btcec.PublicKey, set *muSig2SigningSet,
	sigHashMsg [32]byte) ([]musig2.KeyAggOption, []musig2.CombineOption,
	error) {

	// Case 1: no tweak. The signers signed against the bare aggregate.
	if bareAggregate.IsEqual(set.aggregateKey) {
		return nil, []musig2.CombineOption{
			musig2.WithTweakedCombine(
				sigHashMsg, set.keysForAggregation(), nil,
				set.sortKeys,
			),
		}, nil
	}

	// A tweak was applied at sign time. We can only recover the right
	// tweak when the PSBT pins down the internal key.
	if pInput.TaprootInternalKey == nil {
		return nil, nil, fmt.Errorf("MuSig2 finalize: tweaked " +
			"aggregate without PSBT_IN_TAP_INTERNAL_KEY is not " +
			"supported")
	}

	// Case 4: TaprootInternalKey was derived from the bare aggregate via
	// BIP-32. Walk the path recorded for the internal key to compute the
	// per-step BIP-32 tweaks, then add the taproot tweak on top.
	if !bytes.Equal(
		schnorr.SerializePubKey(bareAggregate),
		pInput.TaprootInternalKey,
	) {

		tweaks, internalKey, err := muSig2DerivationTweaks(
			p, pInput, bareAggregate, pInput.TaprootInternalKey,
		)
		if err != nil {
			return nil, nil, err
		}

		tweaks = append(tweaks, taprootKeyTweak(
			internalKey, pInput.TaprootMerkleRoot,
		))

		keyAggOpts, combineOpts := muSig2TweakOptions(
			set, tweaks, sigHashMsg,
		)
		return keyAggOpts, combineOpts, nil
	}

	// Case 2: TaprootInternalKey is the bare aggregate; the output key is
	// either BIP-86 tweaked (no script tree) or taproot-tweaked with a
	// known merkle root.
	if pInput.TaprootMerkleRoot != nil {
		return []musig2.KeyAggOption{
				musig2.WithTaprootKeyTweak(
					pInput.TaprootMerkleRoot,
				),
			}, []musig2.CombineOption{
				musig2.WithTaprootTweakedCombine(
					sigHashMsg, set.keysForAggregation(),
					pInput.TaprootMerkleRoot, set.sortKeys,
				),
			}, nil
	}

	return []musig2.KeyAggOption{
			musig2.WithBIP86KeyTweak(),
		}, []musig2.CombineOption{
			musig2.WithBip86TweakedCombine(
				sigHashMsg, set.keysForAggregation(),
				set.sortKeys,
			),
		}, nil
}

// muSig2TweaksForScriptSpend returns the key aggregation and combine options
// needed to turn the given plain aggregate key into the key found in a
// tapscript leaf. BIP-373 permits that key to be a BIP-32 child of a parent
// MuSig2 aggregate, in which case the derivation tweaks must be applied here
// too.
//
// Unlike a key spend there is never a taproot tweak: the leaf script commits to
// the key directly via its CHECKSIG opcode, and the taproot tweak of the output
// key is not part of what the participants signed.
func muSig2TweaksForScriptSpend(p *Packet, pInput *PInput,
	bareAggregate *btcec.PublicKey, set *muSig2SigningSet,
	sigHashMsg [32]byte) ([]musig2.KeyAggOption, []musig2.CombineOption,
	error) {

	// BIP-373 test vector case 3: the key in the leaf script is the bare
	// aggregate itself, so no tweak was applied at sign time.
	if bareAggregate.IsEqual(set.aggregateKey) {
		return nil, []musig2.CombineOption{
			musig2.WithTweakedCombine(
				sigHashMsg, set.keysForAggregation(), nil,
				set.sortKeys,
			),
		}, nil
	}

	// The key in the leaf script was derived from the bare aggregate, so
	// the signers applied the BIP-32 derivation tweaks and nothing else.
	tweaks, _, err := muSig2DerivationTweaks(
		p, pInput, bareAggregate,
		schnorr.SerializePubKey(set.aggregateKey),
	)
	if err != nil {
		return nil, nil, err
	}

	keyAggOpts, combineOpts := muSig2TweakOptions(set, tweaks, sigHashMsg)
	return keyAggOpts, combineOpts, nil
}

// muSig2TweakOptions returns the key aggregation and combine options for an
// explicit list of tweaks.
func muSig2TweakOptions(set *muSig2SigningSet, tweaks []musig2.KeyTweakDesc,
	sigHashMsg [32]byte) ([]musig2.KeyAggOption, []musig2.CombineOption) {

	return []musig2.KeyAggOption{
			musig2.WithKeyTweaks(tweaks...),
		}, []musig2.CombineOption{
			musig2.WithTweakedCombine(
				sigHashMsg, set.keysForAggregation(), tweaks,
				set.sortKeys,
			),
		}
}

// taprootKeyTweak returns the x-only tweak that turns the given taproot
// internal key into the output key: the BIP-86 tweak when there is no script
// tree, or the tagged hash over the merkle root otherwise.
func taprootKeyTweak(internalKey *btcec.PublicKey,
	merkleRoot []byte) musig2.KeyTweakDesc {

	tapTweakHash := chainhash.TaggedHash(
		chainhash.TagTapTweak,
		schnorr.SerializePubKey(internalKey), merkleRoot,
	)

	return musig2.KeyTweakDesc{
		Tweak:   *tapTweakHash,
		IsXOnly: true,
	}
}

// muSig2DerivationTweaks walks the BIP-32 derivation path that the input
// records for the given target key, starting from the extended key of the bare
// MuSig2 aggregate, and returns one non-x-only KeyTweakDesc per derivation step
// along with the derived key they produce.
//
// Per BIP-328, the IL value computed in CKDpub is the tweak used at each step,
// with a tweak mode of plain.
func muSig2DerivationTweaks(p *Packet, pInput *PInput,
	bareAggregate *btcec.PublicKey, targetXOnly []byte) (
	[]musig2.KeyTweakDesc, *btcec.PublicKey, error) {

	xpub, err := muSig2AggregateXpub(p, bareAggregate)
	if err != nil {
		return nil, nil, err
	}

	path, err := taprootDerivationPath(pInput, targetXOnly)
	if err != nil {
		return nil, nil, err
	}

	tweaks, derivedXpub, err := bip32TweaksForPath(xpub, path)
	if err != nil {
		return nil, nil, err
	}

	// Sanity-check: the derived key must be the key we were aiming for
	// (compared as x-only). If it doesn't match, the path on the input
	// doesn't correspond to the extended key we derived from and the
	// partial signatures were produced for a different signing context.
	derivedKey, err := derivedXpub.ECPubKey()
	if err != nil {
		return nil, nil, err
	}
	derivedXOnly := schnorr.SerializePubKey(derivedKey)
	if !bytes.Equal(derivedXOnly, targetXOnly) {
		return nil, nil, fmt.Errorf("MuSig2 finalize: BIP-32 derived "+
			"key %x does not match the expected key %x",
			derivedXOnly, targetXOnly)
	}

	return tweaks, derivedKey, nil
}

// muSig2AggregateXpub returns the extended public key to perform BIP-32
// derivation from the given plain MuSig2 aggregate key with.
//
// A PSBT_GLOBAL_XPUB for the aggregate is preferred, as it pins down the chain
// code explicitly. BIP-373 states that derivation from the aggregate pubkey can
// be assumed to follow BIP-328 if the packet carries no such xpub, so we fall
// back to the synthetic extended key that BIP-328 defines.
func muSig2AggregateXpub(p *Packet,
	aggregate *btcec.PublicKey) (*hdkeychain.ExtendedKey, error) {

	xpub, err := findAggregateXpub(p, aggregate)
	if err != nil {
		return nil, err
	}
	if xpub != nil {
		return xpub, nil
	}

	return bip328SyntheticXpub(aggregate), nil
}

// bip328SyntheticXpub returns the BIP-328 synthetic extended public key of a
// plain MuSig2 aggregate public key: the aggregate key with the depth and child
// number set to zero and the fixed BIP-328 chain code attached. Only unhardened
// derivation is possible from it, as there is no aggregate private key.
//
// The version bytes and the parent fingerprint are irrelevant for our purposes,
// as the returned key is only ever derived from, never serialized.
func bip328SyntheticXpub(aggregate *btcec.PublicKey) *hdkeychain.ExtendedKey {
	return hdkeychain.NewExtendedKey(
		chaincfg.MainNetParams.HDPublicKeyID[:],
		aggregate.SerializeCompressed(),
		bip328SyntheticChainCode[:], []byte{0, 0, 0, 0}, 0, 0, false,
	)
}

// findAggregateXpub returns the PSBT_GLOBAL_XPUB whose serialized public key
// matches the given bare MuSig2 aggregate, or nil if no such xpub is present.
func findAggregateXpub(p *Packet,
	aggregate *btcec.PublicKey) (*hdkeychain.ExtendedKey, error) {

	want := aggregate.SerializeCompressed()
	for _, x := range p.XPubs {
		ext, err := DecodeExtendedKey(x.ExtendedKey)
		if err != nil {
			return nil, err
		}

		pub, err := ext.ECPubKey()
		if err != nil {
			return nil, err
		}

		if bytes.Equal(pub.SerializeCompressed(), want) {
			return ext, nil
		}
	}

	return nil, nil
}

// internalKeyDerivationPath returns the BIP-32 path recorded on the
// taproot internal key's PSBT_IN_TAP_BIP32_DERIVATION entry. Returns an
// error if no derivation entry matches the internal key.
func taprootDerivationPath(pInput *PInput, xOnlyKey []byte) ([]uint32, error) {
	if len(xOnlyKey) == 0 {
		return nil, fmt.Errorf("no public key to look up a " +
			"derivation path for")
	}

	for _, d := range pInput.TaprootBip32Derivation {
		if bytes.Equal(d.XOnlyPubKey, xOnlyKey) {
			return d.Bip32Path, nil
		}
	}

	return nil, fmt.Errorf("no PSBT_IN_TAP_BIP32_DERIVATION entry for "+
		"public key %x", xOnlyKey)
}

// bip32TweaksForPath walks the unhardened BIP-32 derivation path on the given
// parent extended key. For each step it computes the per-step scalar tweak (the
// IL half of HMAC-SHA512) and returns it as a non-x-only KeyTweakDesc. The
// fully derived child xpub is also returned so callers can compute follow-up
// tweaks (e.g. the taproot tweak) over its public key.
func bip32TweaksForPath(parent *hdkeychain.ExtendedKey,
	path []uint32) ([]musig2.KeyTweakDesc, *hdkeychain.ExtendedKey,
	error) {

	tweaks := make([]musig2.KeyTweakDesc, 0, len(path))
	current := parent

	for _, idx := range path {
		if idx >= hdkeychain.HardenedKeyStart {
			return nil, nil, fmt.Errorf("hardened derivation step "+
				"%d not supported with public-only xpub", idx)
		}

		parentPub, err := current.ECPubKey()
		if err != nil {
			return nil, nil, err
		}

		// I = HMAC-SHA512(parent.ChainCode,
		//                 parent.SerializedCompressed || idx_be).
		var idxBytes [4]byte
		binary.BigEndian.PutUint32(idxBytes[:], idx)

		h := hmac.New(sha512.New, current.ChainCode())
		h.Write(parentPub.SerializeCompressed())
		h.Write(idxBytes[:])
		ilr := h.Sum(nil)

		var tweak [32]byte
		copy(tweak[:], ilr[:32])
		tweaks = append(tweaks, musig2.KeyTweakDesc{
			Tweak:   tweak,
			IsXOnly: false,
		})

		next, err := current.Derive(idx)
		if err != nil {
			return nil, nil, err
		}
		current = next
	}

	return tweaks, current, nil
}

// finalizeMuSig2ScriptSpend handles a tapscript leaf spend where the key in the
// leaf script is a MuSig2 aggregate key (BIP-373 test vector case 3) or a
// BIP-32 child of one. Returns the serialized witness as
// [aggregatedSig, leafScript, controlBlock], mirroring the regular taproot
// script-spend witness shape.
func finalizeMuSig2ScriptSpend(p *Packet, inIndex int,
	set *muSig2SigningSet) ([]byte, error) {

	pInput := &p.Inputs[inIndex]

	if len(set.tapLeafHash) == 0 {
		return nil, fmt.Errorf("script spend MuSig2 signing requires " +
			"a tap leaf hash on partial signatures")
	}

	leaf, err := FindLeafScript(pInput, set.tapLeafHash)
	if err != nil {
		return nil, fmt.Errorf("leaf script for tap leaf hash %x not "+
			"found: %w", set.tapLeafHash, err)
	}

	// The leaf's CHECKSIG will verify the signature against the aggregate
	// key that BIP-373 says is the key found in the script, so confirm the
	// script really contains it before we trust that declaration.
	spendKey := schnorr.SerializePubKey(set.aggregateKey)
	if err := assertLeafContainsKey(leaf.Script, spendKey); err != nil {
		return nil, err
	}

	prevOutFetcher, err := PrevOutputFetcher(p)
	if err != nil {
		return nil, fmt.Errorf("error making prev out fetcher: %w", err)
	}

	sigHashes := txscript.NewTxSigHashes(p.UnsignedTx, prevOutFetcher)
	sigHash, err := txscript.CalcTapscriptSignaturehash(
		sigHashes, pInput.SighashType, p.UnsignedTx, inIndex,
		prevOutFetcher, txscript.TapLeaf{
			LeafVersion: leaf.LeafVersion,
			Script:      leaf.Script,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("error calculating tapscript signature "+
			"hash: %w", err)
	}

	var sigHashMsg [32]byte
	copy(sigHashMsg[:], sigHash)

	orderedSet, keyAggOpts, combineOpts, err := selectMuSig2Tweaks(
		p, pInput, set, sigHashMsg, muSig2TweaksForScriptSpend,
	)
	if err != nil {
		return nil, err
	}

	schnorrSig, err := combineMuSig2Sig(
		sigHashMsg, orderedSet, keyAggOpts, combineOpts, spendKey,
	)
	if err != nil {
		return nil, err
	}

	sig := appendSighashType(schnorrSig.Serialize(), pInput.SighashType)
	return writeWitness(sig, leaf.Script, leaf.ControlBlock)
}

// assertLeafContainsKey returns an error if the given x-only public key is not
// pushed anywhere by the leaf script.
func assertLeafContainsKey(leafScript []byte, xOnlyKey []byte) error {
	tokenizer := txscript.MakeScriptTokenizer(0, leafScript)
	for tokenizer.Next() {
		if bytes.Equal(tokenizer.Data(), xOnlyKey) {
			return nil
		}
	}
	if err := tokenizer.Err(); err != nil {
		return fmt.Errorf("error parsing tap leaf script: %w", err)
	}

	return fmt.Errorf("tap leaf script does not contain the MuSig2 "+
		"aggregate key %x", xOnlyKey)
}

// musig2InputReady reports whether a taproot input carries at least one
// complete MuSig2 signing context, meaning a set of nonces and partial
// signatures for one aggregate key and spend path that the finalizer can
// attempt to combine.
func musig2InputReady(pInput *PInput) bool {
	return len(muSig2SigningContexts(pInput)) > 0
}

// muSig2SigningSet is the set of MuSig2 fields required to combine the partial
// signatures of an input into a single BIP-340 Schnorr signature. The keys,
// pubNonces and partialSigs slices are parallel: index i of each of them refers
// to the same participant.
type muSig2SigningSet struct {
	// keys, pubNonces and partialSigs are the participants' public keys,
	// public nonces and partial signatures, in matching order.
	keys        []*btcec.PublicKey
	pubNonces   [][musig2.PubNonceSize]byte
	partialSigs []*musig2.PartialSignature

	// sortKeys is true if the keys must be sorted with KeySort before they
	// are aggregated. It is only set for a set whose participant order is
	// unknown, since a set ordered by a PSBT_IN_MUSIG2_PARTICIPANT_PUBKEYS
	// record is already in the order aggregation requires.
	sortKeys bool

	// aggregateKey is the plain (non-tweaked) aggregate key that every
	// nonce and partial signature on the input agrees on.
	aggregateKey *btcec.PublicKey

	// tapLeafHash is the optional tap leaf hash that every nonce and
	// partial signature on the input agrees on. It is empty for a key
	// spend.
	tapLeafHash []byte
}

// keysForAggregation returns a copy of the participant keys, to be handed to
// the btcec MuSig2 package.
//
// NOTE: musig2.AggregateKeys sorts the slice it is given in place when sorting
// is requested, so passing the set's own slice would silently leave keys,
// pubNonces and partialSigs out of sync with one another.
func (m *muSig2SigningSet) keysForAggregation() []*btcec.PublicKey {
	keys := make([]*btcec.PublicKey, len(m.keys))
	copy(keys, m.keys)

	return keys
}

// orderedBy returns a copy of the signing set with its participants rearranged
// into the order of the given participant key list.
//
// BIP-373 stores the keys of a PSBT_IN_MUSIG2_PARTICIPANT_PUBKEYS record "in
// the order required for aggregation", and BIP-327 makes sorting optional, so
// that recorded order is authoritative and the returned set must be aggregated
// without sorting. A session that did sort recorded its keys already sorted, so
// this handles sorted and explicitly unsorted sessions alike.
//
// An error is returned if the record and the input do not describe the exact
// same set of participants.
func (m *muSig2SigningSet) orderedBy(
	participantKeys []*btcec.PublicKey) (*muSig2SigningSet, error) {

	if len(participantKeys) != len(m.keys) {
		return nil, fmt.Errorf("participants record has %d keys but "+
			"input has nonces and partial signatures for %d",
			len(participantKeys), len(m.keys))
	}

	ordered := &muSig2SigningSet{
		keys: make([]*btcec.PublicKey, len(participantKeys)),
		pubNonces: make(
			[][musig2.PubNonceSize]byte, len(participantKeys),
		),
		partialSigs: make(
			[]*musig2.PartialSignature, len(participantKeys),
		),
		sortKeys:     false,
		aggregateKey: m.aggregateKey,
		tapLeafHash:  m.tapLeafHash,
	}

	// Each participant of the record must be matched to exactly one nonce
	// and partial signature pair, so that a record listing the same key
	// twice cannot silently re-use one participant's nonce.
	used := make([]bool, len(m.keys))
	for idx, key := range participantKeys {
		pos := -1
		for i, have := range m.keys {
			if !used[i] && have.IsEqual(key) {
				pos = i
				break
			}
		}
		if pos < 0 {
			return nil, fmt.Errorf("no MuSig2 nonce and partial "+
				"signature for participant key %x",
				key.SerializeCompressed())
		}

		used[pos] = true
		ordered.keys[idx] = m.keys[pos]
		ordered.pubNonces[idx] = m.pubNonces[pos]
		ordered.partialSigs[idx] = m.partialSigs[pos]
	}

	return ordered, nil
}

// muSig2SigningContexts groups the MuSig2 nonces and partial signatures of an
// input into independent signing contexts and returns the complete ones.
//
// BIP-373 keys PSBT_IN_MUSIG2_PUB_NONCE and PSBT_IN_MUSIG2_PARTIAL_SIG by the
// participant key, the aggregate key and an optional tap leaf hash, so a single
// input may carry several sessions at once: different aggregate keys, or the
// same aggregate key on different spend paths. Every (aggregate key, tap leaf
// hash) pair is one such context, and a context is complete when each
// participant that contributed a nonce also contributed a partial signature,
// and no partial signature is left over.
//
// Incomplete contexts are skipped rather than reported as an error: a session
// that is still missing signatures for one spend path must not stop us from
// finalizing a complete session for another.
//
// The returned contexts are ordered deterministically, with key path contexts
// (those without a tap leaf hash) first, since a key path spend produces the
// cheaper witness.
func muSig2SigningContexts(pInput *PInput) []*muSig2SigningSet {
	// contextKey identifies one signing context. The keys are strings
	// rather than byte slices so they can be used as map keys.
	type contextKey struct {
		aggregateKey string
		tapLeafHash  string
	}

	// muSig2Context collects the fields belonging to one signing context.
	type muSig2Context struct {
		aggregateKey *btcec.PublicKey
		tapLeafHash  []byte
		nonces       []*MuSig2PubNonce
		partialSigs  []*MuSig2PartialSig
	}

	var (
		order    []contextKey
		contexts = make(map[contextKey]*muSig2Context)
	)
	contextFor := func(aggregateKey *btcec.PublicKey,
		tapLeafHash []byte) *muSig2Context {

		key := contextKey{
			aggregateKey: string(
				aggregateKey.SerializeCompressed(),
			),
			tapLeafHash: string(tapLeafHash),
		}
		if context, ok := contexts[key]; ok {
			return context
		}

		context := &muSig2Context{
			aggregateKey: aggregateKey,
			tapLeafHash:  tapLeafHash,
		}
		contexts[key] = context
		order = append(order, key)

		return context
	}

	for _, nonce := range pInput.MuSig2PubNonces {
		if nonce == nil || nonce.PubKey == nil ||
			nonce.AggregateKey == nil {

			continue
		}

		context := contextFor(nonce.AggregateKey, nonce.TapLeafHash)
		context.nonces = append(context.nonces, nonce)
	}
	for _, partialSig := range pInput.MuSig2PartialSigs {
		if partialSig == nil || partialSig.PubKey == nil ||
			partialSig.AggregateKey == nil {

			continue
		}

		context := contextFor(
			partialSig.AggregateKey, partialSig.TapLeafHash,
		)
		context.partialSigs = append(context.partialSigs, partialSig)
	}

	sets := make([]*muSig2SigningSet, 0, len(order))
	for _, key := range order {
		context := contexts[key]

		set := muSig2SetFromContext(
			context.aggregateKey, context.tapLeafHash,
			context.nonces, context.partialSigs,
		)
		if set == nil {
			continue
		}

		sets = append(sets, set)
	}

	slices.SortStableFunc(sets, func(a, b *muSig2SigningSet) int {
		// Key path contexts have no tap leaf hash and come first.
		switch {
		case len(a.tapLeafHash) == 0 && len(b.tapLeafHash) != 0:
			return -1
		case len(a.tapLeafHash) != 0 && len(b.tapLeafHash) == 0:
			return 1
		}

		if cmp := bytes.Compare(
			a.aggregateKey.SerializeCompressed(),
			b.aggregateKey.SerializeCompressed(),
		); cmp != 0 {

			return cmp
		}

		return bytes.Compare(a.tapLeafHash, b.tapLeafHash)
	})

	return sets
}

// muSig2SetFromContext pairs the nonces and partial signatures of a single
// signing context up by participant key, so the resulting parallel slices are
// aligned regardless of the order the fields appear on the input. It returns
// nil if the context is not complete.
//
// The participants end up in the order their nonces appear on the input, which
// carries no meaning for key aggregation, so the returned set is marked as
// needing a sort. Use orderedBy to put it into the order a
// PSBT_IN_MUSIG2_PARTICIPANT_PUBKEYS record requires.
func muSig2SetFromContext(aggregateKey *btcec.PublicKey, tapLeafHash []byte,
	nonces []*MuSig2PubNonce,
	partialSigs []*MuSig2PartialSig) *muSig2SigningSet {

	if len(nonces) == 0 || len(nonces) != len(partialSigs) {
		return nil
	}

	set := &muSig2SigningSet{
		keys:         make([]*btcec.PublicKey, len(nonces)),
		pubNonces:    make([][musig2.PubNonceSize]byte, len(nonces)),
		partialSigs:  make([]*musig2.PartialSignature, len(nonces)),
		sortKeys:     true,
		aggregateKey: aggregateKey,
		tapLeafHash:  tapLeafHash,
	}

	for idx, nonce := range nonces {
		var partialSig *musig2.PartialSignature
		for _, candidate := range partialSigs {
			if candidate.PubKey.IsEqual(nonce.PubKey) {
				partialSig = &candidate.PartialSig
				break
			}
		}

		// A nonce without a matching partial signature means the
		// session is still in progress.
		if partialSig == nil {
			return nil
		}

		set.keys[idx] = nonce.PubKey
		set.pubNonces[idx] = nonce.PubNonce
		set.partialSigs[idx] = partialSig
	}

	return set
}

// combineMuSig2Sig aggregates the keys and nonces of the given signing set,
// combines its partial signatures into a single BIP-340 Schnorr signature and
// verifies that signature against spendKey, the x-only key the script engine
// will check it with. The keyAggOpts and combineOpts must describe the same
// tweak chain: tweaks applied during key aggregation must match the tweaks
// accumulated by the combine option.
//
// The verification is not optional. Finalization replaces the input's MuSig2
// fields with the witness it builds, so an invalid combined signature that
// slips through here leaves a transaction the script engine rejects and no
// partial signatures left to retry from. PartialSigAgg cannot detect a partial
// signature that was tampered with or produced for a different message, which
// is exactly what this catches.
func combineMuSig2Sig(sigHashMsg [32]byte, set *muSig2SigningSet,
	keyAggOpts []musig2.KeyAggOption, combineOpts []musig2.CombineOption,
	spendKey []byte) (*schnorr.Signature, error) {

	aggKey, _, _, err := musig2.AggregateKeys(
		set.keysForAggregation(), set.sortKeys, keyAggOpts...,
	)
	if err != nil {
		return nil, fmt.Errorf("error aggregating keys: %w", err)
	}

	aggregateNonce, err := musig2.AggregateNonces(set.pubNonces)
	if err != nil {
		return nil, fmt.Errorf("error aggregating pub nonces: %w", err)
	}

	// The final nonce cannot be taken from the partial signatures the way
	// musig2.CombineSigs is normally called with partialSigs[0].R: only the
	// S value of a partial signature is serialized in a PSBT, so R is
	// always nil for a signature we read out of a packet. We therefore have
	// to re-derive the final nonce from the participants' public nonces.
	nonceJ, _, err := musig2.ComputeSigningNonce(
		aggregateNonce, aggKey.FinalKey, sigHashMsg,
	)
	if err != nil {
		return nil, fmt.Errorf("error computing signing nonce: %w", err)
	}
	nonceJ.ToAffine()

	sig := musig2.CombineSigs(
		btcec.NewPublicKey(&nonceJ.X, &nonceJ.Y), set.partialSigs,
		combineOpts...,
	)

	verifyKey, err := schnorr.ParsePubKey(spendKey)
	if err != nil {
		return nil, fmt.Errorf("error parsing spend key %x: %w",
			spendKey, err)
	}
	if !sig.Verify(sigHashMsg[:], verifyKey) {
		return nil, fmt.Errorf("combined MuSig2 signature for "+
			"aggregate key %x does not verify under spend key %x",
			set.aggregateKey.SerializeCompressed(), spendKey)
	}

	return sig, nil
}

// appendSighashType appends a one-byte sighash type to the signature if it
// differs from the default sighash (which is omitted on the wire).
func appendSighashType(sig []byte, sht txscript.SigHashType) []byte {
	if sht == txscript.SigHashDefault {
		return sig
	}
	return append(sig, byte(sht))
}
