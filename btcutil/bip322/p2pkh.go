package bip322

import (
	"errors"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// ErrInvalidToSignStructure is returned when a decoded to_sign transaction
// does not conform to the BIP-322 structure requirements.
var ErrInvalidToSignStructure = errors.New(
	"bip322: invalid to_sign transaction structure",
)

// SignP2PKH signs a BIP-322 message using a P2PKH address and returns the
// signature encoded in BIP-322 Full format (base64-encoded complete to_sign
// transaction). P2PKH uses Full format because it lacks a witness field.
//
// The private key must correspond to the public key hash in addr.
func SignP2PKH(privKey *btcec.PrivateKey, addr btcutil.Address,
	message string) (string, error) {

	scriptPubKey, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return "", err
	}

	toSpend, err := BuildToSpendTx(message, scriptPubKey)
	if err != nil {
		return "", err
	}

	toSign, err := BuildToSignTx(toSpend.TxHash(), scriptPubKey)
	if err != nil {
		return "", err
	}

	scriptSig, err := txscript.SignatureScript(
		toSign, 0, scriptPubKey, txscript.SigHashAll, privKey, true,
	)
	if err != nil {
		return "", err
	}

	toSign.TxIn[0].SignatureScript = scriptSig

	return EncodeFull(toSign)
}

// VerifyP2PKH verifies a BIP-322 Full format signature against a P2PKH
// address and message.
//
// Returns (true, nil) on valid signature, (false, nil) on invalid signature,
// and (false, err) on malformed input.
func VerifyP2PKH(addr btcutil.Address, message string,
	signature string) (bool, error) {

	toSign, err := DecodeFull(signature)
	if err != nil {
		return false, err
	}

	if err := validateToSignStructure(toSign); err != nil {
		return false, err
	}

	scriptPubKey, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return false, err
	}

	toSpend, err := BuildToSpendTx(message, scriptPubKey)
	if err != nil {
		return false, err
	}

	expectedTxID := toSpend.TxHash()
	prevOut := toSign.TxIn[0].PreviousOutPoint
	if prevOut.Hash != expectedTxID || prevOut.Index != 0 {
		return false, nil
	}

	prevFetcher := txscript.NewCannedPrevOutputFetcher(scriptPubKey, 0)
	hashCache := txscript.NewTxSigHashes(toSign, prevFetcher)

	vm, err := txscript.NewEngine(
		scriptPubKey, toSign, 0, bip322VerifyFlags, nil,
		hashCache, 0, prevFetcher,
	)
	if err != nil {
		return false, err
	}

	if err := vm.Execute(); err != nil {
		return false, nil
	}

	return true, nil
}

// validateToSignStructure checks that the decoded to_sign transaction conforms
// to the BIP-322 structure: version 0, one input, one output with value 0,
// and locktime 0.
func validateToSignStructure(tx *wire.MsgTx) error {
	if tx.Version != 0 {
		return ErrInvalidToSignStructure
	}
	if len(tx.TxIn) != 1 {
		return ErrInvalidToSignStructure
	}
	if len(tx.TxOut) != 1 {
		return ErrInvalidToSignStructure
	}
	if tx.TxOut[0].Value != 0 {
		return ErrInvalidToSignStructure
	}
	if tx.LockTime != 0 {
		return ErrInvalidToSignStructure
	}
	return nil
}
