package bip322

import (
	"errors"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
)

var (
	// ErrInconclusive is returned when verification cannot be completed
	// because the address uses an unknown script type (e.g., witness version > 1).
	ErrInconclusive = errors.New("bip322: verification inconclusive")

	// ErrUnsupportedAddressType is returned when Sign is called with an
	// address type that is not supported for signing.
	ErrUnsupportedAddressType = errors.New("bip322: unsupported address type for signing")

	// ErrMalformedSignature is returned when the signature cannot be decoded.
	ErrMalformedSignature = errors.New("bip322: malformed signature")
)

// Sign produces a BIP-322 signature for the given message using the provided
// private key, dispatching to the appropriate signing function based on the
// address type:
//   - P2WPKH      -> BIP-322 Simple format
//   - P2TR        -> BIP-322 Simple format (Schnorr key-path)
//   - P2SH-P2WPKH -> BIP-322 Simple format (nested segwit)
//   - P2PKH       -> BIP-322 Full format
//
// Any other address type returns ErrUnsupportedAddressType.
func Sign(privKey *btcec.PrivateKey, addr btcutil.Address, message string) (string, error) {
	switch addr.(type) {
	case *btcutil.AddressWitnessPubKeyHash:
		return SignP2WPKH(privKey, addr, message)
	case *btcutil.AddressTaproot:
		return SignP2TR(privKey, addr, message)
	case *btcutil.AddressScriptHash:
		return SignP2SHP2WPKH(privKey, addr, message)
	case *btcutil.AddressPubKeyHash:
		return SignP2PKH(privKey, addr, message)
	default:
		return "", ErrUnsupportedAddressType
	}
}

// Verify verifies a BIP-322 signature against an address and message.
// The format is auto-detected from the signature bytes:
//   - Valid witness stack encoding -> BIP-322 Simple format
//   - Valid serialized transaction -> BIP-322 Full format
func Verify(addr btcutil.Address, message string, signature string) (bool, error) {
	format, err := DetectFormat(signature)
	if err != nil {
		return false, ErrMalformedSignature
	}

	switch format {
	case FormatSimple:
		return verifySimple(addr, message, signature)
	case FormatFull:
		return verifyFull(addr, message, signature)
	default:
		return false, ErrMalformedSignature
	}
}

// verifySimple verifies a BIP-322 Simple format signature by dispatching to
// the appropriate address-type-specific verifier.
func verifySimple(addr btcutil.Address, message string, signature string) (bool, error) {
	switch addr.(type) {
	case *btcutil.AddressWitnessPubKeyHash:
		return VerifyP2WPKH(addr, message, signature)
	case *btcutil.AddressTaproot:
		return VerifyP2TR(addr, message, signature)
	case *btcutil.AddressScriptHash:
		return VerifyP2SHP2WPKH(addr, message, signature)
	case *btcutil.AddressWitnessScriptHash:
		return VerifyP2WSH(addr, message, signature)
	default:
		return false, ErrInconclusive
	}
}

// verifyFull verifies a BIP-322 Full format signature by decoding the complete
// to_sign transaction, validating its structure, and executing the script engine
// against the expected to_spend transaction.
func verifyFull(addr btcutil.Address, message string, signature string) (bool, error) {
	toSign, err := DecodeFull(signature)
	if err != nil {
		return false, ErrMalformedSignature
	}

	if err := validateToSignStructure(toSign); err != nil {
		return false, nil
	}

	scriptPubKey, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return false, err
	}
	toSpend, err := BuildToSpendTx(message, scriptPubKey)
	if err != nil {
		return false, err
	}
	if toSign.TxIn[0].PreviousOutPoint.Hash != toSpend.TxHash() {
		return false, nil
	}
	if toSign.TxIn[0].PreviousOutPoint.Index != 0 {
		return false, nil
	}

	prevFetcher := txscript.NewCannedPrevOutputFetcher(scriptPubKey, 0)
	hashCache := txscript.NewTxSigHashes(toSign, prevFetcher)
	vm, err := txscript.NewEngine(scriptPubKey, toSign, 0, bip322VerifyFlags, nil, hashCache, 0, prevFetcher)
	if err != nil {
		return false, err
	}
	if err := vm.Execute(); err != nil {
		return false, nil
	}
	return true, nil
}
