// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

// signer encapsulates the role 'Signer' as specified in BIP174; it controls
// the insertion of signatures; the Sign() function will attempt to insert
// signatures using Updater.addPartialSignature, after first ensuring the Psbt
// is in the correct state.

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcd/txscript/v2"
)

// SignOutcome is a enum-like value that expresses the outcome of a call to the
// Sign method.
type SignOutcome int

const (
	// SignSuccesful indicates that the partial signature was successfully
	// attached.
	SignSuccesful = 0

	// SignFinalized  indicates that this input is already finalized, so the
	// provided signature was *not* attached
	SignFinalized = 1

	// SignInvalid indicates that the provided signature data was not valid.
	// In this case an error will also be returned.
	SignInvalid = -1
)

// Sign allows the caller to sign a PSBT at a particular input; they
// must provide a signature and a pubkey, both as byte slices; they can also
// optionally provide both witnessScript and/or redeemScript, otherwise these
// arguments must be set as nil (and in that case, they must already be present
// in the PSBT if required for signing to succeed).
//
// This serves as a wrapper around Updater.addPartialSignature; it ensures that
// the redeemScript and witnessScript are updated as needed (note that the
// Updater is allowed to add redeemScripts and witnessScripts independently,
// before signing), and ensures that the right form of utxo field
// (NonWitnessUtxo or WitnessUtxo) is included in the input so that signature
// insertion (and then finalization) can take place.
func (u *Updater) Sign(inIndex int, sig []byte, pubKey []byte,
	redeemScript []byte, witnessScript []byte) (SignOutcome, error) {

	if isFinalized(u.Upsbt, inIndex) {
		return SignFinalized, nil
	}

	// Add the witnessScript to the PSBT in preparation.  If it already
	// exists, it will be overwritten.
	if witnessScript != nil {
		err := u.AddInWitnessScript(witnessScript, inIndex)
		if err != nil {
			return SignInvalid, err
		}
	}

	// Add the redeemScript to the PSBT in preparation.  If it already
	// exists, it will be overwritten.
	if redeemScript != nil {
		err := u.AddInRedeemScript(redeemScript, inIndex)
		if err != nil {
			return SignInvalid, err
		}
	}

	// At this point, the PSBT must have the requisite witnessScript or
	// redeemScript fields for signing to succeed.
	//
	// Case 1: if witnessScript is present, it must be of type witness;
	// if not, signature insertion will of course fail.
	pInput := u.Upsbt.Inputs[inIndex]
	switch {
	case pInput.WitnessScript != nil:
		if pInput.WitnessUtxo == nil {
			err := nonWitnessToWitness(u.Upsbt, inIndex)
			if err != nil {
				return SignInvalid, err
			}
		}

		err := u.addPartialSignature(inIndex, sig, pubKey)
		if err != nil {
			return SignInvalid, err
		}

	// Case 2: no witness script, only redeem script; can be legacy p2sh or
	// p2sh-wrapped p2wkh.
	case pInput.RedeemScript != nil:
		// We only need to decide if the input is witness, and we don't
		// rely on the witnessutxo/nonwitnessutxo in the PSBT, instead
		// we check the redeemScript content.
		if txscript.IsWitnessProgram(redeemScript) {
			if pInput.WitnessUtxo == nil {
				err := nonWitnessToWitness(u.Upsbt, inIndex)
				if err != nil {
					return SignInvalid, err
				}
			}
		}

		// If it is not a valid witness program, we here assume that
		// the provided WitnessUtxo/NonWitnessUtxo field was correct.
		err := u.addPartialSignature(inIndex, sig, pubKey)
		if err != nil {
			return SignInvalid, err
		}

	// Case 3: Neither provided only works for native p2wkh, or non-segwit
	// non-p2sh. To check if it's segwit, check the scriptPubKey of the
	// output.
	default:
		if pInput.WitnessUtxo == nil {
			txIn := u.Upsbt.UnsignedTx.TxIn[inIndex]
			outIndex := txIn.PreviousOutPoint.Index
			script := pInput.NonWitnessUtxo.TxOut[outIndex].PkScript

			if txscript.IsWitnessProgram(script) {
				err := nonWitnessToWitness(u.Upsbt, inIndex)
				if err != nil {
					return SignInvalid, err
				}
			}
		}

		err := u.addPartialSignature(inIndex, sig, pubKey)
		if err != nil {
			return SignInvalid, err
		}
	}

	return SignSuccesful, nil
}

// SignMuSig2 attaches a MuSig2 partial signature to the input at index
// inIndex, following the BIP-174 Signer role for the MuSig2 fields defined by
// BIP-373.
//
// Before appending, SignMuSig2 enforces the invariants a finalizer will later
// rely on:
//
//   - The input must not be finalized.
//   - The input must carry a witness UTXO (every BIP-373 MuSig2 spend is
//     segwit v1).
//   - The participant pubkey on the partial sig must appear in at least one
//     PSBT_IN_MUSIG2_PARTICIPANT_PUBKEYS record on the input. The aggregate
//     key recorded on the partial sig itself does not need to equal the
//     bare aggregate in the participants record: BIP-373 case 4 deliberately
//     records the BIP-32-derived aggregate on the partial sigs while the
//     participants record carries the parent (bare) aggregate, and the
//     finalizer reconciles the two via PSBT_GLOBAL_XPUB. Enforcing equality
//     here would reject the legitimate derived-aggregate case.
//   - A PSBT_IN_MUSIG2_PUB_NONCE field with the same key data prefix
//     (participant pubkey || aggregate pubkey || optional tap leaf hash) must
//     already be present — partial sigs cannot be combined without their
//     matching nonces.
//   - If TapLeafHash is set, it must resolve to a leaf script recorded on
//     the input (i.e. the script the partial sig commits to is actually
//     part of this spend).
//
// On any of these checks failing the input is left untouched and SignInvalid
// is returned together with the underlying error. Shape validation
// (compressed pubkeys, 32-byte partial sig) and duplicate-key detection are
// handled by AddInMuSig2PartialSig, which SignMuSig2 delegates to once the
// Signer-role checks pass.
//
// SignMuSig2 does not itself compute the partial signature; callers are
// expected to feed in the output of musig2.Sign (held in the
// MuSig2PartialSig.PartialSig field). This mirrors the existing Sign()
// helper, which accepts a pre-computed ECDSA signature rather than driving
// the signing key directly.
func (u *Updater) SignMuSig2(inIndex int,
	partialSig *MuSig2PartialSig) (SignOutcome, error) {

	if inIndex < 0 || inIndex >= len(u.Upsbt.Inputs) {
		return SignInvalid, ErrInvalidPsbtFormat
	}

	if isFinalized(u.Upsbt, inIndex) {
		return SignFinalized, nil
	}

	if partialSig == nil || partialSig.PubKey == nil ||
		partialSig.AggregateKey == nil {

		return SignInvalid, ErrInvalidPsbtFormat
	}

	pInput := &u.Upsbt.Inputs[inIndex]

	// BIP-373 inputs are taproot (segwit v1); a witness UTXO is required
	// for the finalizer to recompute the sighash.
	if pInput.WitnessUtxo == nil {
		return SignInvalid, ErrInvalidPsbtFormat
	}

	// Make sure the participant is actually a member of a known aggregate
	// on this input. Without a PSBT_IN_MUSIG2_PARTICIPANT_PUBKEYS record
	// the finalizer cannot reproduce the aggregate, so the partial sig is
	// not usable.
	if !musig2ParticipantRegistered(pInput, partialSig) {
		return SignInvalid, fmt.Errorf("%w: participant pubkey not "+
			"found in any MuSig2 participants record matching the "+
			"supplied aggregate key", ErrInvalidSignatureForInput)
	}

	// A matching pub nonce (same participant pubkey || aggregate key ||
	// optional tap leaf hash) must exist; nonces are the precondition for
	// signing per BIP-373 §Signer.
	keyData := partialSig.KeyData()
	if !musig2HasMatchingPubNonce(pInput, keyData) {
		return SignInvalid, fmt.Errorf("%w: no matching "+
			"PSBT_IN_MUSIG2_PUB_NONCE found for partial signature",
			ErrInvalidSignatureForInput)
	}

	// If the partial sig commits to a tap leaf, the leaf script must
	// actually be present on the input, otherwise the finalizer will not
	// be able to assemble the script-spend witness.
	if len(partialSig.TapLeafHash) > 0 {
		if _, err := FindLeafScript(
			pInput, partialSig.TapLeafHash,
		); err != nil {

			return SignInvalid, fmt.Errorf("%w: tap leaf hash %x "+
				"on partial signature does not match any leaf "+
				"script on input: %v",
				ErrInvalidSignatureForInput,
				partialSig.TapLeafHash, err)
		}
	}

	// Shape validation, duplicate-key detection and the actual append are
	// done by the existing low-level updater helper.
	if err := u.AddInMuSig2PartialSig(inIndex, partialSig); err != nil {
		return SignInvalid, err
	}

	return SignSuccesful, nil
}

// musig2ParticipantRegistered reports whether the partial sig's participant
// pubkey appears in any PSBT_IN_MUSIG2_PARTICIPANT_PUBKEYS record on the
// input. The aggregate key on the partial sig is intentionally not compared
// against the record's aggregate; in BIP-373 case 4 the partial sig records
// the BIP-32 derived aggregate while the record carries the bare aggregate.
func musig2ParticipantRegistered(pInput *PInput,
	partialSig *MuSig2PartialSig) bool {

	for _, participants := range pInput.MuSig2Participants {
		for _, key := range participants.Keys {
			if key.IsEqual(partialSig.PubKey) {
				return true
			}
		}
	}

	return false
}

// musig2HasMatchingPubNonce reports whether the input carries a
// PSBT_IN_MUSIG2_PUB_NONCE whose key data matches the partial signature's
// key data (participant pubkey || aggregate key || optional tap leaf hash).
func musig2HasMatchingPubNonce(pInput *PInput, partialSigKeyData []byte) bool {
	for _, n := range pInput.MuSig2PubNonces {
		if bytes.Equal(n.KeyData(), partialSigKeyData) {
			return true
		}
	}

	return false
}

// nonWitnessToWitness extracts the TxOut from the existing NonWitnessUtxo
// field in the given PSBT input and sets it as type witness by replacing the
// NonWitnessUtxo field with a WitnessUtxo field. See
// https://github.com/bitcoin/bitcoin/pull/14197.
func nonWitnessToWitness(p *Packet, inIndex int) error {
	outIndex := p.UnsignedTx.TxIn[inIndex].PreviousOutPoint.Index
	txout := p.Inputs[inIndex].NonWitnessUtxo.TxOut[outIndex]

	// TODO(guggero): For segwit v1, we'll want to remove the NonWitnessUtxo
	// from the packet. For segwit v0 it is unsafe to only rely on the
	// witness UTXO. See https://github.com/bitcoin/bitcoin/pull/19215.
	// p.Inputs[inIndex].NonWitnessUtxo = nil

	u := Updater{
		Upsbt: p,
	}

	return u.AddInWitnessUtxo(txout, inIndex)
}
