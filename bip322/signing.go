package bip322

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/psbt/v2"
	"github.com/btcsuite/btcd/txscript/v2"
)

var (
	// errPrivateKeyNil is returned if a nil private key is passed to one of
	// the signing functions.
	errPrivateKeyNil = errors.New("private key cannot be nil")
)

// PsbtPrevOutputFetcher returns a txscript.PrevOutFetcher built from the UTXO
// information in a PSBT packet.
func PsbtPrevOutputFetcher(packet *psbt.Packet) *txscript.MultiPrevOutFetcher {
	fetcher := txscript.NewMultiPrevOutFetcher(nil)
	for idx, txIn := range packet.UnsignedTx.TxIn {
		in := packet.Inputs[idx]

		// Skip any input that has no UTXO.
		if in.WitnessUtxo == nil && in.NonWitnessUtxo == nil {
			continue
		}

		if in.NonWitnessUtxo != nil {
			prevIndex := txIn.PreviousOutPoint.Index

			if prevIndex >= uint32(len(in.NonWitnessUtxo.TxOut)) {
				continue
			}

			fetcher.AddPrevOut(
				txIn.PreviousOutPoint,
				in.NonWitnessUtxo.TxOut[prevIndex],
			)

			continue
		}

		// Fall back to witness UTXO only for older wallets.
		if in.WitnessUtxo != nil {
			fetcher.AddPrevOut(
				txIn.PreviousOutPoint, in.WitnessUtxo,
			)
		}
	}

	return fetcher
}

// addPartialSignature adds a signature to the given input's PartialSigs and
// checks for duplicate pubkeys.
func addPartialSignature(in *psbt.PInput, sig []byte, pubKey []byte) error {
	for _, existingSig := range in.PartialSigs {
		if bytes.Equal(existingSig.PubKey, pubKey) {
			return fmt.Errorf("duplicate signature for pubkey %x",
				pubKey)
		}
	}

	in.PartialSigs = append(in.PartialSigs, &psbt.PartialSig{
		Signature: sig,
		PubKey:    pubKey,
	})

	return nil
}

// signInputTaprootKeySpend signs the P2TR input at the given index in the PSBT
// packet using the given private key.
func signInputTaprootKeySpend(packet *psbt.Packet, idx int,
	privateKey *btcec.PrivateKey) error {

	if idx >= len(packet.Inputs) {
		return fmt.Errorf("invalid input index %d", idx)
	}

	in := &packet.Inputs[idx]
	utxo := in.WitnessUtxo
	if utxo == nil {
		return fmt.Errorf("input %d has no witness UTXO", idx)
	}

	prevOutFetcher := PsbtPrevOutputFetcher(packet)
	sigHashes := txscript.NewTxSigHashes(packet.UnsignedTx, prevOutFetcher)
	sig, err := txscript.RawTxInTaprootSignature(
		packet.UnsignedTx, sigHashes, idx, utxo.Value, utxo.PkScript,
		[]byte{}, txscript.SigHashDefault, privateKey,
	)
	if err != nil {
		return fmt.Errorf("error signing: %w", err)
	}

	in.TaprootKeySpendSig = sig

	return nil
}

// signInputWitness signs a SegWit v0 input at the given index in the PSBT
// packet using the given private key. The script must be the UTXO's pkScript
// for P2WPKH, the redeem script for P2SH, or the witness program for P2WSH.
func signInputWitness(packet *psbt.Packet, idx int, script []byte,
	privateKey *btcec.PrivateKey) error {

	if idx >= len(packet.Inputs) {
		return fmt.Errorf("invalid input index %d", idx)
	}

	in := &packet.Inputs[idx]
	txIn := packet.UnsignedTx.TxIn[idx]
	utxo := in.WitnessUtxo
	if utxo == nil {
		prevTx := in.NonWitnessUtxo
		if prevTx == nil {
			return fmt.Errorf("input %d has no UTXO", idx)
		}

		if txIn.PreviousOutPoint.Index >= uint32(len(prevTx.TxOut)) {
			return fmt.Errorf("input %d has no UTXO", idx)
		}

		if prevTx.TxHash() != txIn.PreviousOutPoint.Hash {
			return fmt.Errorf("non witness utxo does not match " +
				"input prevout")
		}

		utxo = prevTx.TxOut[txIn.PreviousOutPoint.Index]

		// Since we know this is a witness input, we should set the
		// WitnessUtxo field to avoid issues with finalizing the packet.
		in.WitnessUtxo = utxo
	}

	prevOutFetcher := PsbtPrevOutputFetcher(packet)
	sigHashes := txscript.NewTxSigHashes(packet.UnsignedTx, prevOutFetcher)
	sig, err := txscript.RawTxInWitnessSignature(
		packet.UnsignedTx, sigHashes, idx, utxo.Value, script,
		txscript.SigHashAll, privateKey,
	)
	if err != nil {
		return fmt.Errorf("error signing: %w", err)
	}

	return addPartialSignature(
		in, sig, privateKey.PubKey().SerializeCompressed(),
	)
}

// signInputLegacy signs a legacy input at the given index in the PSBT packet
// using the given private key. The script must be the UTXO's pkScript for
// P2PKH or the redeem script for P2SH.
func signInputLegacy(packet *psbt.Packet, idx int, script []byte,
	privateKey *btcec.PrivateKey) error {

	if idx >= len(packet.Inputs) {
		return fmt.Errorf("invalid input index %d", idx)
	}

	in := &packet.Inputs[idx]

	sig, err := txscript.RawTxInSignature(
		packet.UnsignedTx, idx, script, txscript.SigHashAll, privateKey,
	)
	if err != nil {
		return fmt.Errorf("error signing: %w", err)
	}

	return addPartialSignature(
		in, sig, privateKey.PubKey().SerializeCompressed(),
	)
}

// payToTaprootScript creates a new script to pay to a version 1 Taproot key
// spend address.
func payToTaprootScript(privateKey *btcec.PrivateKey) ([]byte, error) {
	trKey := txscript.ComputeTaprootKeyNoScript(privateKey.PubKey())
	return txscript.PayToTaprootScript(trKey)
}

// SignP2TR signs a message using the given private key using the P2TR address
// that corresponds to the given key. The message MUST be valid UTF-8.
func SignP2TR(message string, privateKey *btcec.PrivateKey) (string, error) {
	if privateKey == nil {
		return "", errPrivateKeyNil
	}

	pkScript, err := payToTaprootScript(privateKey)
	if err != nil {
		return "", fmt.Errorf("error creating pkScript: %w", err)
	}

	toSign, err := BuildToSignPacketSimple([]byte(message), pkScript)
	if err != nil {
		return "", fmt.Errorf("error creating toSign packet: %w", err)
	}

	err = signInputTaprootKeySpend(toSign, 0, privateKey)
	if err != nil {
		return "", fmt.Errorf("error signing: %w", err)
	}

	if err := psbt.MaybeFinalizeAll(toSign); err != nil {
		return "", fmt.Errorf("error finalizing packet: %w", err)
	}

	return SerializeSignature(toSign)
}

// payToWitnessPubKeyHashScript creates a new script to pay to a version 0
// pubkey hash witness program. The passed private key is expected to be valid.
func payToWitnessPubKeyHashScript(
	privateKey *btcec.PrivateKey) ([]byte, error) {

	pubKeyHash := address.Hash160(privateKey.PubKey().SerializeCompressed())
	return txscript.NewScriptBuilder().AddOp(txscript.OP_0).
		AddData(pubKeyHash).Script()
}

// SignP2WPKH signs a message using the given private key using the P2WPKH
// address that corresponds to the given key. The message MUST be valid UTF-8.
func SignP2WPKH(message string, privateKey *btcec.PrivateKey) (string, error) {
	if privateKey == nil {
		return "", errPrivateKeyNil
	}

	pkScript, err := payToWitnessPubKeyHashScript(privateKey)
	if err != nil {
		return "", fmt.Errorf("error creating pkScript: %w", err)
	}

	toSign, err := BuildToSignPacketSimple([]byte(message), pkScript)
	if err != nil {
		return "", fmt.Errorf("error creating toSign packet: %w", err)
	}

	err = signInputWitness(toSign, 0, pkScript, privateKey)
	if err != nil {
		return "", fmt.Errorf("error signing: %w", err)
	}

	if err := psbt.MaybeFinalizeAll(toSign); err != nil {
		return "", fmt.Errorf("error finalizing packet: %w", err)
	}

	return SerializeSignature(toSign)
}

// payToNestedWitnessPubKeyHashScript creates a new script to pay to a nested
// version 0 pubkey hash witness program. The passed private key is expected to
// be valid.
func payToNestedWitnessPubKeyHashScript(
	privateKey *btcec.PrivateKey) ([]byte, []byte, error) {

	witnessProgram, err := payToWitnessPubKeyHashScript(privateKey)
	if err != nil {
		return nil, nil, err
	}

	scriptHash := address.Hash160(witnessProgram)
	pkScript, err := txscript.NewScriptBuilder().AddOp(txscript.OP_HASH160).
		AddData(scriptHash).AddOp(txscript.OP_EQUAL).Script()
	if err != nil {
		return nil, nil, err
	}

	return pkScript, witnessProgram, nil
}

// SignNestedP2WPKH signs a message using the given private key using the
// NestedP2WKH (sometimes also called NP2WKH or P2SH-P2WPKH) address that
// corresponds to the given key. The message MUST be valid UTF-8.
func SignNestedP2WPKH(message string, privateKey *btcec.PrivateKey) (string,
	error) {

	if privateKey == nil {
		return "", errPrivateKeyNil
	}

	pkScript, witnessScript, err := payToNestedWitnessPubKeyHashScript(
		privateKey,
	)
	if err != nil {
		return "", fmt.Errorf("error creating pkScript: %w", err)
	}

	toSign, err := BuildToSignPacketFull([]byte(message), pkScript, 0, 0, 0)
	if err != nil {
		return "", fmt.Errorf("error building packet: %w", err)
	}

	toSign.Inputs[0].RedeemScript = witnessScript
	err = signInputWitness(toSign, 0, witnessScript, privateKey)
	if err != nil {
		return "", fmt.Errorf("error signing: %w", err)
	}

	if err := psbt.MaybeFinalizeAll(toSign); err != nil {
		return "", fmt.Errorf("error finalizing packet: %w", err)
	}

	return SerializeSignature(toSign)
}

// payToPubKeyHashScript creates a new script to pay a transaction
// output to a 20-byte pubkey hash. It is expected that the input is a valid
// hash.
func payToPubKeyHashScript(privateKey *btcec.PrivateKey) ([]byte, error) {
	pubKeyHash := address.Hash160(privateKey.PubKey().SerializeCompressed())
	return txscript.NewScriptBuilder().AddOp(txscript.OP_DUP).
		AddOp(txscript.OP_HASH160).AddData(pubKeyHash).
		AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG).
		Script()
}

// SignP2PKH signs a message using the given private key using the P2PKH
// address that corresponds to the given key. The message MUST be valid UTF-8.
func SignP2PKH(message string, privateKey *btcec.PrivateKey) (string, error) {
	if privateKey == nil {
		return "", errPrivateKeyNil
	}

	pkScript, err := payToPubKeyHashScript(privateKey)
	if err != nil {
		return "", fmt.Errorf("error creating pkScript: %w", err)
	}

	toSign, err := BuildToSignPacketFull([]byte(message), pkScript, 0, 0, 0)
	if err != nil {
		return "", fmt.Errorf("error building packet: %w", err)
	}

	err = signInputLegacy(toSign, 0, pkScript, privateKey)
	if err != nil {
		return "", fmt.Errorf("error signing: %w", err)
	}

	if err := psbt.MaybeFinalizeAll(toSign); err != nil {
		return "", fmt.Errorf("error finalizing packet: %w", err)
	}

	return SerializeSignature(toSign)
}
