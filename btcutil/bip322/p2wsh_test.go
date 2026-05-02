package bip322

import (
	"crypto/sha256"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

const (
	testP2WSHPrivKeyWIF = "L3VFeEujGtevx9w18HD1fhRbCH67Az2dpCymeRE1SoPK6XQtaN2k"
)

func TestBIP322P2WSHVerify(t *testing.T) {
	// Decode the test private key
	wif, err := btcutil.DecodeWIF(testP2WSHPrivKeyWIF)
	require.NoError(t, err)

	// Create witness script.
	// OP_DUP OP_HASH160 <pubKeyHash> OP_EQUALVERIFY OP_CHECKSIG
	pubKey := wif.PrivKey.PubKey()
	pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())

	witnessScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_DUP).
		AddOp(txscript.OP_HASH160).
		AddData(pubKeyHash).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_CHECKSIG).
		Script()
	require.NoError(t, err)

	// Create P2WSH address from witness script hash
	witnessScriptHash := sha256.Sum256(witnessScript)
	addr, err := btcutil.NewAddressWitnessScriptHash(witnessScriptHash[:], &chaincfg.MainNetParams)
	require.NoError(t, err)

	message := "Hello World"

	// Build scriptPubKey from address
	scriptPubKey, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	// Build BIP-322 transactions
	toSpend, err := BuildToSpendTx(message, scriptPubKey)
	require.NoError(t, err)

	toSign, err := BuildToSignTx(toSpend.TxHash(), scriptPubKey)
	require.NoError(t, err)

	// Sign the P2WSH input
	prevFetcher := txscript.NewCannedPrevOutputFetcher(scriptPubKey, 0)
	sigHashes := txscript.NewTxSigHashes(toSign, prevFetcher)

	sig, err := txscript.RawTxInWitnessSignature(
		toSign, sigHashes, 0, 0, witnessScript, txscript.SigHashAll, wif.PrivKey,
	)
	require.NoError(t, err)

	// For P2WSH with P2PKH-style witness script, witness is [sig, pubKey, witnessScript]
	pubKeyBytes := pubKey.SerializeCompressed()
	witness := wire.TxWitness{sig, pubKeyBytes, witnessScript}

	// Encode witness in Simple format
	encoded, err := EncodeSimple(witness)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	// Verify the signature
	valid, err := VerifyP2WSH(addr, message, encoded)
	require.NoError(t, err)
	require.True(t, valid, "expected valid P2WSH signature")
}
