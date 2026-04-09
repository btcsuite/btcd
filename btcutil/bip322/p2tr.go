package bip322

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
)

// SignP2TR signs a BIP-322 message using a P2TR address (key-path spend) and
// returns the signature encoded in BIP-322 Simple format (base64-encoded
// witness stack containing a 64-byte Schnorr signature).
//
// The private key must correspond to the taproot output key derived via
// txscript.ComputeTaprootKeyNoScript(privKey.PubKey()), which must match
// the key committed to in addr. SigHashDefault (0x00) is used, producing a
// compact 64-byte Schnorr signature per BIP-341.
func SignP2TR(privKey *btcec.PrivateKey, addr btcutil.Address, message string) (string, error) {
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
	// SigHashDefault produces a compact 64-byte Schnorr signature (no sighash byte appended).
	witness, err := txscript.TaprootWitnessSignature(
		toSign, sigHashes, 0, 0, scriptPubKey, txscript.SigHashDefault, privKey,
	)
	if err != nil {
		return "", err
	}

	toSign.TxIn[0].Witness = witness

	return EncodeSimple(witness)
}

// VerifyP2TR verifies a BIP-322 Simple format signature against a P2TR address
// and message (key-path spend).
//
// Returns (true, nil) on valid signature, (false, nil) on invalid signature, and
// (false, err) on malformed input.
func VerifyP2TR(addr btcutil.Address, message string, signature string) (bool, error) {
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
