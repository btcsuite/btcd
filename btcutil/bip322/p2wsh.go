package bip322

import (
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
)

// VerifyP2WSH verifies a BIP-322 signature for a P2WSH address.
// The signature must be in Simple format (base64-encoded witness stack).
// P2WSH signing is not supported; use VerifyP2WSH with externally produced witnesses.
//
// Returns (true, nil) on valid signature, (false, nil) on invalid signature, and
// (false, err) on malformed input.
func VerifyP2WSH(addr btcutil.Address, message string, signature string) (bool, error) {
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
		scriptPubKey, toSign, 0, bip322VerifyFlags,
		nil, hashCache, 0, prevFetcher,
	)
	if err != nil {
		return false, err
	}

	if err := vm.Execute(); err != nil {
		// Script execution failure means the signature is invalid, not an error.
		return false, nil
	}

	return true, nil
}
