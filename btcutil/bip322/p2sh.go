package bip322

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// buildP2WPKHRedeemScript constructs the P2WPKH redeemScript for a compressed
// public key: OP_0 PUSH20[Hash160(pubKey)].
func buildP2WPKHRedeemScript(pubKeyBytes []byte) ([]byte, error) {
	return txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(btcutil.Hash160(pubKeyBytes)).
		Script()
}

// buildP2PKHScriptCode constructs the P2PKH scriptCode used as the signing
// subscript for P2SH-P2WPKH witness signatures:
// OP_DUP OP_HASH160 PUSH20[Hash160(pubKey)] OP_EQUALVERIFY OP_CHECKSIG.
func buildP2PKHScriptCode(pubKeyBytes []byte) ([]byte, error) {
	return txscript.NewScriptBuilder().
		AddOp(txscript.OP_DUP).
		AddOp(txscript.OP_HASH160).
		AddData(btcutil.Hash160(pubKeyBytes)).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_CHECKSIG).
		Script()
}

// buildRedeemScriptSig constructs the scriptSig that pushes redeemScript as
// data, satisfying a P2SH input.
func buildRedeemScriptSig(redeemScript []byte) ([]byte, error) {
	return txscript.NewScriptBuilder().AddData(redeemScript).Script()
}

// SignP2SHP2WPKH signs a BIP-322 message using a P2SH-P2WPKH address (nested
// segwit, starts with "3") and returns the signature in BIP-322 Simple format
// (base64-encoded witness stack).
//
// P2SH-P2WPKH requires the to_sign input to carry both:
//   - scriptSig: pushes the P2WPKH redeemScript
//   - witness: [sig, compressedPubKey] signed over the P2PKH scriptCode
//
// The private key must correspond to the public key whose Hash160 is embedded
// in the P2SH address.
func SignP2SHP2WPKH(privKey *btcec.PrivateKey, addr btcutil.Address, message string) (string, error) {
	pubKeyBytes := privKey.PubKey().SerializeCompressed()

	redeemScript, err := buildP2WPKHRedeemScript(pubKeyBytes)
	if err != nil {
		return "", err
	}

	p2shScriptPubKey, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return "", err
	}

	toSpend, err := BuildToSpendTx(message, p2shScriptPubKey)
	if err != nil {
		return "", err
	}

	toSign, err := BuildToSignTx(toSpend.TxHash(), p2shScriptPubKey)
	if err != nil {
		return "", err
	}

	// The witness signature is computed over the P2PKH scriptCode, not the
	// P2SH scriptPubKey.
	scriptCode, err := buildP2PKHScriptCode(pubKeyBytes)
	if err != nil {
		return "", err
	}

	prevFetcher := txscript.NewCannedPrevOutputFetcher(p2shScriptPubKey, 0)
	sigHashes := txscript.NewTxSigHashes(toSign, prevFetcher)

	// BIP-322 to_spend output has value 0, so amt=0.
	sig, err := txscript.RawTxInWitnessSignature(
		toSign, sigHashes, 0, 0, scriptCode, txscript.SigHashAll, privKey,
	)
	if err != nil {
		return "", err
	}

	witness := wire.TxWitness{sig, pubKeyBytes}

	scriptSig, err := buildRedeemScriptSig(redeemScript)
	if err != nil {
		return "", err
	}

	toSign.TxIn[0].SignatureScript = scriptSig
	toSign.TxIn[0].Witness = witness

	return EncodeSimple(witness)
}

// VerifyP2SHP2WPKH verifies a BIP-322 Simple format signature against a
// P2SH-P2WPKH address and message.
//
// Returns (true, nil) on valid signature, (false, nil) on invalid signature, and
// (false, err) on malformed input.
func VerifyP2SHP2WPKH(addr btcutil.Address, message string, signature string) (bool, error) {
	witness, err := DecodeSimple(signature)
	if err != nil {
		return false, err
	}

	if len(witness) != 2 {
		return false, nil
	}

	pubKeyBytes := witness[1]

	redeemScript, err := buildP2WPKHRedeemScript(pubKeyBytes)
	if err != nil {
		return false, err
	}

	p2shScriptPubKey, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return false, err
	}

	toSpend, err := BuildToSpendTx(message, p2shScriptPubKey)
	if err != nil {
		return false, err
	}

	toSign, err := BuildToSignTx(toSpend.TxHash(), p2shScriptPubKey)
	if err != nil {
		return false, err
	}

	scriptSig, err := buildRedeemScriptSig(redeemScript)
	if err != nil {
		return false, err
	}

	toSign.TxIn[0].SignatureScript = scriptSig
	toSign.TxIn[0].Witness = witness

	prevFetcher := txscript.NewCannedPrevOutputFetcher(p2shScriptPubKey, 0)
	hashCache := txscript.NewTxSigHashes(toSign, prevFetcher)

	vm, err := txscript.NewEngine(
		p2shScriptPubKey, toSign, 0, bip322VerifyFlags, nil, hashCache, 0, prevFetcher,
	)
	if err != nil {
		return false, err
	}

	if err := vm.Execute(); err != nil {
		return false, nil
	}

	return true, nil
}
