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

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

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

			// MuSig2 partial sigs are only useful when there is a
			// matching nonce per participant AND every partial sig
			// and nonce references the same aggregate key and tap
			// leaf hash. The pre-aggregated TaprootKeySpendSig /
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
			return fmt.Errorf("control block for script spend " +
				"signature not found")
		}

		// The witness stack will contain all signatures, followed by
		// the script itself and then the control block.
		for idx, scriptSpendSig := range pInput.TaprootScriptSpendSig {
			// Make sure that if there are indeed multiple
			// signatures, they all reference the same leaf hash.
			if !bytes.Equal(scriptSpendSig.LeafHash, targetLeafHash) {
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

	// MuSig2 spend path. Dispatch on the presence of a tap leaf hash on
	// the partial signatures: if all partial sigs reference a tap leaf
	// hash, this is a tapscript leaf spend; otherwise it's a top-level
	// keyspend.
	case len(pInput.MuSig2PartialSigs) > 0:
		firstSig := pInput.MuSig2PartialSigs[0]
		if len(firstSig.TapLeafHash) > 0 {
			serializedWitness, err = finalizeMuSig2ScriptSpend(
				p, inIndex,
			)
		} else {
			serializedWitness, err = finalizeMuSig2KeySpend(
				p, inIndex,
			)
		}

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

// finalizeMuSig2KeySpend handles BIP-373 test vector cases 1 and 2: a top-level
// taproot key spend where the aggregate MuSig2 key is either the output key
// directly (no tweak) or the internal key (BIP-86 or merkle-root taproot
// tweak). Returns the serialized witness containing the aggregated BIP-340
// Schnorr signature.
func finalizeMuSig2KeySpend(p *Packet, inIndex int) ([]byte, error) {
	pInput := &p.Inputs[inIndex]

	keys, pubNonces, partialSigs, aggKey, _, err := extractMuSig2SigningSet(
		pInput,
	)
	if err != nil {
		return nil, err
	}

	prevOutFetcher := PrevOutputFetcher(p)
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

	keyAggOpts, combineOpts, err := selectMuSig2KeyAggTweaks(
		p, pInput, aggKey, keys, sigHashMsg,
	)
	if err != nil {
		return nil, err
	}

	schnorrSig, err := combineMuSig2Sig(
		sigHashMsg, keys, pubNonces, partialSigs, keyAggOpts,
		combineOpts,
	)
	if err != nil {
		return nil, err
	}

	sig := appendSighashType(schnorrSig.Serialize(), pInput.SighashType)
	return writeWitness(sig)
}

// selectMuSig2KeyAggTweaks decides which (if any) tweaks the finalizer must
// apply when combining the keyspend MuSig2 partial signatures. It compares
// the aggregate key recorded in the partial sig keydata against the bare
// aggregate from PSBT_IN_MUSIG2_PARTICIPANT_PUBKEYS:
//
//   - Equal → no tweak was applied at sign time (BIP-373 case 1: output key
//     IS the aggregate).
//   - Differ + TaprootInternalKey == bare aggregate → BIP-86 tweak (or
//     taproot tweak with the merkle root, if present). BIP-373 test vector
//     case 2.
//   - Differ + TaprootInternalKey ≠ bare aggregate → BIP-373 test vector case 4
//     (the internal key was derived from the aggregate via BIP-32). Not yet
//     supported; return an explicit error rather than silently producing a
//     wrong signature.
func selectMuSig2KeyAggTweaks(p *Packet, pInput *PInput,
	partialSigAggregate *btcec.PublicKey, keys []*btcec.PublicKey,
	sigHashMsg [32]byte) ([]musig2.KeyAggOption, []musig2.CombineOption,
	error) {

	var bareAggregate *btcec.PublicKey
	if len(pInput.MuSig2Participants) > 0 {
		bareAggregate = pInput.MuSig2Participants[0].AggregateKey
	}

	// If MuSig2Participants is missing, fall back to treating the partial
	// sig aggregate as the bare aggregate. This mirrors the behavior of a
	// PSBT in which an updater only set the partial sigs.
	if bareAggregate == nil {
		bareAggregate = partialSigAggregate
	}

	// Case 1: no tweak. The signers signed against the bare aggregate.
	if bareAggregate.IsEqual(partialSigAggregate) {
		return nil, []musig2.CombineOption{
			musig2.WithTweakedCombine(sigHashMsg, keys, nil, true),
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
	// BIP-32. Walk the path on the synthetic xpub from PSBT_GLOBAL_XPUB
	// to compute the per-step BIP-32 tweaks, then add the taproot tweak.
	if !bytes.Equal(
		schnorr.SerializePubKey(bareAggregate),
		pInput.TaprootInternalKey,
	) {

		return musig2BIP32DerivedTweaks(
			p, pInput, bareAggregate, keys, sigHashMsg,
		)
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
					sigHashMsg, keys,
					pInput.TaprootMerkleRoot, true,
				),
			}, nil
	}

	return []musig2.KeyAggOption{
			musig2.WithBIP86KeyTweak(),
		}, []musig2.CombineOption{
			musig2.WithBip86TweakedCombine(sigHashMsg, keys, true),
		}, nil
}

// musig2BIP32DerivedTweaks handles BIP-373 test vector case 4: the taproot
// internal key is a BIP-32 unhardened child of the bare MuSig2 aggregate. The
// PSBT must include a PSBT_GLOBAL_XPUB whose serialized public key matches the
// bare aggregate; the chain code from that xpub is used to walk the path
// recorded on the internal key's PSBT_IN_TAP_BIP32_DERIVATION entry.
//
// The resulting list of tweaks is:
//   - one non-x-only KeyTweakDesc per BIP-32 derivation step
//   - one x-only KeyTweakDesc carrying the taproot tweak hash, computed
//     against the post-BIP-32 derived internal key (BIP-86 if no merkle
//     root; tagged hash with the merkle root otherwise)
//
// The combined Schnorr signature produced via these tweaks verifies under
// the post-derivation, post-taproot-tweak output key.
func musig2BIP32DerivedTweaks(p *Packet, pInput *PInput,
	bareAggregate *btcec.PublicKey, keys []*btcec.PublicKey,
	sigHashMsg [32]byte) ([]musig2.KeyAggOption, []musig2.CombineOption,
	error) {

	xpub, err := findAggregateXpub(p, bareAggregate)
	if err != nil {
		return nil, nil, err
	}
	if xpub == nil {
		return nil, nil, fmt.Errorf("MuSig2 finalize: aggregate is " +
			"BIP-32 derived but no matching PSBT_GLOBAL_XPUB " +
			"found (BIP-328 fallback is not supported)")
	}

	path, err := internalKeyDerivationPath(pInput)
	if err != nil {
		return nil, nil, err
	}

	bip32Tweaks, derivedXpub, err := bip32TweaksForPath(xpub, path)
	if err != nil {
		return nil, nil, err
	}

	// Sanity-check: the derived xpub must equal the taproot internal key
	// (compared as x-only). If it doesn't match, the path on the input
	// doesn't correspond to the synthetic xpub we used and the partial
	// signatures were produced for a different signing context.
	derivedKey, err := derivedXpub.ECPubKey()
	if err != nil {
		return nil, nil, err
	}
	if !bytes.Equal(
		schnorr.SerializePubKey(derivedKey),
		pInput.TaprootInternalKey,
	) {

		return nil, nil, fmt.Errorf("MuSig2 finalize: BIP-32 derived " +
			"key does not match taproot internal key on input")
	}

	// Compute the taproot tweak: BIP-86 (empty merkle root) or a tagged
	// hash with the merkle root.
	internalKeyXOnly := schnorr.SerializePubKey(derivedKey)
	var merkleRoot []byte
	if pInput.TaprootMerkleRoot != nil {
		merkleRoot = pInput.TaprootMerkleRoot
	}
	tapTweakHash := chainhash.TaggedHash(
		chainhash.TagTapTweak, internalKeyXOnly, merkleRoot,
	)
	allTweaks := append(bip32Tweaks, musig2.KeyTweakDesc{
		Tweak:   *tapTweakHash,
		IsXOnly: true,
	})

	return []musig2.KeyAggOption{
			musig2.WithKeyTweaks(allTweaks...),
		}, []musig2.CombineOption{
			musig2.WithTweakedCombine(
				sigHashMsg, keys, allTweaks, true,
			),
		}, nil
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
func internalKeyDerivationPath(pInput *PInput) ([]uint32, error) {
	if pInput.TaprootInternalKey == nil {
		return nil, fmt.Errorf("input has no taproot internal key")
	}

	for _, d := range pInput.TaprootBip32Derivation {
		if bytes.Equal(d.XOnlyPubKey, pInput.TaprootInternalKey) {
			return d.Bip32Path, nil
		}
	}

	return nil, fmt.Errorf("no PSBT_IN_TAP_BIP32_DERIVATION entry for " +
		"taproot internal key")
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

// finalizeMuSig2ScriptSpend handles BIP-373 test vector case 3: a tapscript
// leaf spend where the aggregate MuSig2 key is the key in the leaf script.
// Returns the serialized witness as [aggregatedSig, leafScript, controlBlock],
// mirroring the regular taproot script-spend witness shape.
func finalizeMuSig2ScriptSpend(p *Packet, inIndex int) ([]byte, error) {
	pInput := &p.Inputs[inIndex]

	keys, pubNonces, partialSigs, _, tapLeafHash, err :=
		extractMuSig2SigningSet(pInput)
	if err != nil {
		return nil, err
	}
	if len(tapLeafHash) == 0 {
		return nil, fmt.Errorf("script spend MuSig2 signing requires " +
			"a tap leaf hash on partial signatures")
	}

	leaf, err := FindLeafScript(pInput, tapLeafHash)
	if err != nil {
		return nil, fmt.Errorf("leaf script for tap leaf hash %x not "+
			"found: %w", tapLeafHash, err)
	}

	prevOutFetcher := PrevOutputFetcher(p)
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

	// No taproot tweak: the aggregate key is the key in the script and is
	// committed to directly via the leaf script's CHECKSIG opcode.
	combineOpts := []musig2.CombineOption{
		musig2.WithTweakedCombine(sigHashMsg, keys, nil, true),
	}

	schnorrSig, err := combineMuSig2Sig(
		sigHashMsg, keys, pubNonces, partialSigs, nil, combineOpts,
	)
	if err != nil {
		return nil, err
	}

	sig := appendSighashType(schnorrSig.Serialize(), pInput.SighashType)
	return writeWitness(sig, leaf.Script, leaf.ControlBlock)
}

// musig2InputReady reports whether a taproot input has a complete and
// internally consistent set of MuSig2 fields ready for finalization: the
// number of nonces matches the number of partial signatures, and every
// nonce and partial signature references the same (AggregateKey,
// TapLeafHash) pair.
func musig2InputReady(pInput *PInput) bool {
	if len(pInput.MuSig2PartialSigs) != len(pInput.MuSig2PubNonces) {
		return false
	}

	first := pInput.MuSig2PartialSigs[0]
	for _, ps := range pInput.MuSig2PartialSigs {
		if !ps.AggregateKey.IsEqual(first.AggregateKey) {
			return false
		}
		if !bytes.Equal(ps.TapLeafHash, first.TapLeafHash) {
			return false
		}
	}
	for _, n := range pInput.MuSig2PubNonces {
		if !n.AggregateKey.IsEqual(first.AggregateKey) {
			return false
		}
		if !bytes.Equal(n.TapLeafHash, first.TapLeafHash) {
			return false
		}
	}

	return true
}

// extractMuSig2SigningSet validates that all MuSig2 nonces and partial
// signatures on the input reference the same aggregate key (and the same
// optional tap leaf hash) and returns parallel slices of participant keys,
// nonces, and partial signatures, paired up by participant pubkey. The
// returned aggregate key and tap leaf hash are taken from the partial
// signatures (and verified to agree with the nonces).
func extractMuSig2SigningSet(pInput *PInput) ([]*btcec.PublicKey,
	[][musig2.PubNonceSize]byte, []*musig2.PartialSignature,
	*btcec.PublicKey, []byte, error) {

	if len(pInput.MuSig2PartialSigs) == 0 {
		return nil, nil, nil, nil, nil, fmt.Errorf("no MuSig2 " +
			"partial signatures on input")
	}
	if len(pInput.MuSig2PubNonces) != len(pInput.MuSig2PartialSigs) {
		return nil, nil, nil, nil, nil, fmt.Errorf("number of MuSig2 " +
			"pub nonces does not match number of partial " +
			"signatures")
	}

	first := pInput.MuSig2PartialSigs[0]
	aggregateKey := first.AggregateKey
	tapLeafHash := first.TapLeafHash

	// All partial sigs must agree on the aggregate key and tap leaf hash.
	for i, ps := range pInput.MuSig2PartialSigs {
		if !ps.AggregateKey.IsEqual(aggregateKey) {
			return nil, nil, nil, nil, nil, fmt.Errorf("partial "+
				"sig %d references different aggregate key "+
				"than first signature", i)
		}
		if !bytes.Equal(ps.TapLeafHash, tapLeafHash) {
			return nil, nil, nil, nil, nil, fmt.Errorf("partial "+
				"sig %d references different tap leaf hash "+
				"than first signature", i)
		}
	}

	// Likewise for the nonces.
	for i, n := range pInput.MuSig2PubNonces {
		if !n.AggregateKey.IsEqual(aggregateKey) {
			return nil, nil, nil, nil, nil, fmt.Errorf("pub nonce "+
				"%d references different aggregate key than "+
				"partial signatures", i)
		}
		if !bytes.Equal(n.TapLeafHash, tapLeafHash) {
			return nil, nil, nil, nil, nil, fmt.Errorf("pub nonce "+
				"%d references different tap leaf hash than "+
				"partial signatures", i)
		}
	}

	// Pair each pub nonce with its partial sig by participant key, so
	// that the parallel slices we return are aligned regardless of the
	// order the fields appear on the input.
	n := len(pInput.MuSig2PubNonces)
	keys := make([]*btcec.PublicKey, n)
	pubNonces := make([][musig2.PubNonceSize]byte, n)
	partialSigs := make([]*musig2.PartialSignature, n)
	for i, nonce := range pInput.MuSig2PubNonces {
		keys[i] = nonce.PubKey
		pubNonces[i] = nonce.PubNonce

		var matched *MuSig2PartialSig
		for _, ps := range pInput.MuSig2PartialSigs {
			if ps.PubKey.IsEqual(nonce.PubKey) {
				matched = ps
				break
			}
		}
		if matched == nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("no MuSig2 "+
				"partial signature found for participant key "+
				"%x", nonce.PubKey.SerializeCompressed())
		}
		partialSigs[i] = &matched.PartialSig
	}

	return keys, pubNonces, partialSigs, aggregateKey, tapLeafHash, nil
}

// combineMuSig2Sig aggregates the keys/nonces and combines the partial
// signatures into a single BIP-340 Schnorr signature. The keyAggOpts and
// combineOpts must describe the same tweak chain: tweaks applied during key
// aggregation must match the tweaks accumulated by the combine option.
func combineMuSig2Sig(sigHashMsg [32]byte, keys []*btcec.PublicKey,
	pubNonces [][musig2.PubNonceSize]byte,
	partialSigs []*musig2.PartialSignature,
	keyAggOpts []musig2.KeyAggOption,
	combineOpts []musig2.CombineOption) (*schnorr.Signature, error) {

	aggKey, _, _, err := musig2.AggregateKeys(keys, true, keyAggOpts...)
	if err != nil {
		return nil, fmt.Errorf("error aggregating keys: %w", err)
	}

	aggregateNonce, err := musig2.AggregateNonces(pubNonces)
	if err != nil {
		return nil, fmt.Errorf("error aggregating pub nonces: %w", err)
	}

	combinedNonce, err := computeSigningNonce(
		aggregateNonce, aggKey.FinalKey, sigHashMsg,
	)
	if err != nil {
		return nil, fmt.Errorf("error computing signing nonce: %w", err)
	}

	return musig2.CombineSigs(
		combinedNonce, partialSigs, combineOpts...,
	), nil
}

// appendSighashType appends a one-byte sighash type to the signature if it
// differs from the default sighash (which is omitted on the wire).
func appendSighashType(sig []byte, sht txscript.SigHashType) []byte {
	if sht == txscript.SigHashDefault {
		return sig
	}
	return append(sig, byte(sht))
}

// computeSigningNonce calculates the final nonce used for signing. This will
// be the R value used in the final signature.
func computeSigningNonce(combinedNonce [musig2.PubNonceSize]byte,
	combinedKey *btcec.PublicKey, msg [32]byte) (*btcec.PublicKey, error) {

	// Next we'll compute the value b, that blinds our second public
	// nonce:
	//  * b = h(tag=NonceBlindTag, combinedNonce || combinedKey || m).
	var (
		nonceMsgBuf  bytes.Buffer
		nonceBlinder btcec.ModNScalar
	)
	nonceMsgBuf.Write(combinedNonce[:])
	nonceMsgBuf.Write(schnorr.SerializePubKey(combinedKey))
	nonceMsgBuf.Write(msg[:])
	nonceBlindHash := chainhash.TaggedHash(
		musig2.NonceBlindTag, nonceMsgBuf.Bytes(),
	)
	nonceBlinder.SetByteSlice(nonceBlindHash[:])

	// Next, we'll parse the public nonces into R1 and R2.
	r1J, err := btcec.ParseJacobian(
		combinedNonce[:btcec.PubKeyBytesLenCompressed],
	)
	if err != nil {
		return nil, err
	}
	r2J, err := btcec.ParseJacobian(
		combinedNonce[btcec.PubKeyBytesLenCompressed:],
	)
	if err != nil {
		return nil, err
	}

	// With our nonce blinding value, we'll now combine both the public
	// nonces, using the blinding factor to tweak the second nonce:
	//  * R = R_1 + b*R_2
	var nonce btcec.JacobianPoint
	btcec.ScalarMultNonConst(&nonceBlinder, &r2J, &r2J)
	btcec.AddNonConst(&r1J, &r2J, &nonce)

	// If the combined nonce is the point at infinity, we'll use the
	// generator point instead.
	var infinityPoint btcec.JacobianPoint
	if nonce == infinityPoint {
		G := btcec.Generator()
		G.AsJacobian(&nonce)
	}

	nonce.ToAffine()

	return btcec.NewPublicKey(&nonce.X, &nonce.Y), nil
}
