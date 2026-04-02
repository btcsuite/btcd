package bip322

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
)

// SignP2WPKH signs a BIP-322 message using a P2WPKH address and returns
// the signature encoded in BIP-322 Simple format (base64-encoded witness stack).
//
// The private key must correspond to the public key hash in addr.
func SignP2WPKH(privKey *btcec.PrivateKey, addr btcutil.Address, message string) (string, error) {
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

	prevFetcher := txscript.NewCannedPrevOutputFetcher(scriptPubKey, 0)
	sigHashes := txscript.NewTxSigHashes(toSign, prevFetcher)

	// BIP-322 to_spend output has value 0, so amt=0.
	witness, err := txscript.WitnessSignature(
		toSign, sigHashes, 0, 0, scriptPubKey, txscript.SigHashAll, privKey, true,
	)
	if err != nil {
		return "", err
	}

	toSign.TxIn[0].Witness = witness

	return EncodeSimple(witness)
}

// VerifyP2WPKH verifies a BIP-322 Simple format signature against a P2WPKH
// address and message.
//
// Returns (true, nil) on valid signature, (false, nil) on invalid signature, and
// (false, err) on malformed input.
func VerifyP2WPKH(addr btcutil.Address, message string, signature string) (bool, error) {
	scriptPubKey, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return false, err
	}

	witness, err := DecodeSimple(signature)
	if err != nil {
		return false, err
	}

	toSpend, err := BuildToSpendTx(message, scriptPubKey)
	if err != nil {
		return false, err
	}

	toSign, err := BuildToSignTx(toSpend.TxHash(), scriptPubKey)
	if err != nil {
		return false, err
	}

	toSign.TxIn[0].Witness = witness

	prevFetcher := txscript.NewCannedPrevOutputFetcher(scriptPubKey, 0)
	hashCache := txscript.NewTxSigHashes(toSign, prevFetcher)

	vm, err := txscript.NewEngine(
		scriptPubKey, toSign, 0, bip322VerifyFlags, nil, hashCache, 0, prevFetcher,
	)
	if err != nil {
		return false, err
	}

	// Script execution failure means the signature is invalid, not an error.
	if err := vm.Execute(); err != nil {
		return false, nil
	}

	return true, nil
}
