package bip322

import (
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/psbt/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// codeSeparatorScript builds the minimal `OP_CODESEPARATOR OP_TRUE` script used
// by the OP_CODESEPARATOR rejection probes.
func codeSeparatorScript(t *testing.T) []byte {
	t.Helper()

	script, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_CODESEPARATOR).
		AddOp(txscript.OP_TRUE).
		Script()
	require.NoError(t, err)

	return script
}

// witnessScriptChallenge return the pkScript for a p2wsh challenge.
func witnessScriptChallenge(t *testing.T, witnessScript []byte,
	versionOP byte) []byte {

	scriptHash := sha256.Sum256(witnessScript)
	pkScript, err := txscript.NewScriptBuilder().
		AddOp(versionOP).
		AddData(scriptHash[:]).
		Script()
	require.NoError(t, err)

	return pkScript
}

// opTrueChallenge creates an OP_TRUE pkScript challenge and witness stack for
// spending it.
func opTrueChallenge(t *testing.T) ([]byte, wire.TxWitness, []byte) {
	t.Helper()

	witnessScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_TRUE).
		Script()
	require.NoError(t, err)

	pkScript := witnessScriptChallenge(t, witnessScript, txscript.OP_0)
	witness := wire.TxWitness{witnessScript}
	witnessBytes, err := SerializeTxWitness(witness)
	require.NoError(t, err)

	return pkScript, witness, witnessBytes
}

func makeHtlcScript(t *testing.T, paymentHash [32]byte, taproot bool) ([]byte,
	*btcec.PrivateKey) {

	receiverKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	refundKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	receiverKeyBytes := receiverKey.PubKey().SerializeCompressed()
	refundKeyBytes := refundKey.PubKey().SerializeCompressed()
	if taproot {
		receiverKeyBytes = schnorr.SerializePubKey(receiverKey.PubKey())
		refundKeyBytes = schnorr.SerializePubKey(refundKey.PubKey())
	}

	htlcScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_IF).
		AddOp(txscript.OP_SHA256).
		AddData(paymentHash[:]).
		AddOp(txscript.OP_EQUALVERIFY).
		AddData(receiverKeyBytes).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_ELSE).
		AddInt64(500).
		AddOp(txscript.OP_CHECKLOCKTIMEVERIFY).
		AddOp(txscript.OP_DROP).
		AddData(refundKeyBytes).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_ENDIF).
		Script()
	require.NoError(t, err)

	return htlcScript, receiverKey
}

func taprootWitness(t *testing.T, script []byte) ([]byte, []byte) {
	internalKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	leaf := txscript.NewBaseTapLeaf(script)
	tree := txscript.AssembleTaprootScriptTree(leaf)
	rootHash := tree.RootNode.TapHash()
	outputKey := txscript.ComputeTaprootOutputKey(
		internalKey.PubKey(), rootHash[:],
	)
	pkScript, err := txscript.PayToTaprootScript(outputKey)
	require.NoError(t, err)

	controlBlock := tree.LeafMerkleProofs[0].ToControlBlock(
		internalKey.PubKey(),
	)
	controlBlockBytes, err := controlBlock.ToBytes()
	require.NoError(t, err)

	return pkScript, controlBlockBytes
}

func taprootHtlcWitness(t *testing.T, packet *psbt.Packet, htlcScript, preimage,
	controlBlockBytes []byte, sigHashType txscript.SigHashType,
	receiverKey *btcec.PrivateKey) wire.TxWitness {

	finalTx := packet.UnsignedTx.Copy()
	utxo := packet.Inputs[0].WitnessUtxo
	prevOutFetcher := txscript.NewCannedPrevOutputFetcher(
		utxo.PkScript, utxo.Value,
	)
	sigHashes := txscript.NewTxSigHashes(finalTx, prevOutFetcher)

	receiverSig, err := txscript.RawTxInTapscriptSignature(
		finalTx, sigHashes, 0, utxo.Value, utxo.PkScript,
		txscript.NewBaseTapLeaf(htlcScript), sigHashType, receiverKey,
	)
	require.NoError(t, err)

	return wire.TxWitness{
		receiverSig,
		preimage,
		[]byte{1}, // Select the preimage (hashlock) branch.
		htlcScript,
		controlBlockBytes,
	}
}

// TestVerifyMessageSimpleRejectsCodeSeparator asserts that the BIP-322 required
// rule forbidding OP_CODESEPARATOR is enforced for segwit witness scripts and
// taproot leaf scripts. The script engine, even with StandardVerifyFlags, only
// rejects OP_CODESEPARATOR in non-segwit scripts, so without the dedicated
// inspection these probes would (incorrectly) verify as valid.
func TestVerifyMessageSimpleRejectsCodeSeparator(t *testing.T) {
	t.Parallel()

	message := []byte("probe")

	// P2WSH witness script: OP_CODESEPARATOR OP_TRUE.
	t.Run("p2wsh", func(t *testing.T) {
		script := codeSeparatorScript(t)

		pkScript := witnessScriptChallenge(t, script, txscript.OP_0)
		witness := wire.TxWitness{script}

		valid, _, err := VerifyMessageSimple(message, pkScript, witness)
		require.ErrorIs(t, err, ErrCodeSeparator)
		require.False(t, valid)
	})

	// P2TR tapscript: OP_CODESEPARATOR OP_TRUE, revealed via a script-path
	// spend.
	t.Run("p2tr", func(t *testing.T) {
		leafScript := codeSeparatorScript(t)
		pkScript, controlBlockBytes := taprootWitness(t, leafScript)

		witness := wire.TxWitness{leafScript, controlBlockBytes}

		valid, _, err := VerifyMessageSimple(message, pkScript, witness)
		require.ErrorIs(t, err, ErrCodeSeparator)
		require.False(t, valid)
	})
}

// TestVerifyMessageSimpleAcceptsTaprootScriptPathData asserts that a valid P2TR
// script-path spend is not rejected just because one of its (non-signature)
// stack inputs happens to look like a Schnorr signature with an explicit
// sighash byte.
//
// The probe uses an HTLC-style tapscript whose hashlock branch consumes a
// 65-byte preimage whose final byte is SigHashNone, so it structurally
// resembles a 65-byte Schnorr signature carrying a disallowed sighash flag.
// Because the restricted-sighash policy is enforced by the script engine, only
// signatures actually consumed by a signature opcode are checked; the preimage
// is consumed by OP_SHA256, not OP_CHECKSIG, so it is never subject to the
// rule. The real signature in the witness is a valid 64-byte SIGHASH_DEFAULT
// Schnorr signature, so BIP-322 gives no reason to reject the message.
func TestVerifyMessageSimpleAcceptsTaprootScriptPathData(t *testing.T) {
	t.Parallel()

	message := []byte("probe")

	// Build a 65-byte preimage whose trailing byte is a (disallowed)
	// explicit sighash flag, so it structurally resembles a 65-byte Schnorr
	// signature.
	preimage := make([]byte, 65)
	for i := 0; i < len(preimage)-1; i++ {
		preimage[i] = byte(i + 1)
	}
	preimage[len(preimage)-1] = byte(txscript.SigHashNone)
	paymentHash := sha256.Sum256(preimage)

	htlcScript, receiverKey := makeHtlcScript(t, paymentHash, true)
	pkScript, controlBlockBytes := taprootWitness(t, htlcScript)

	packet, err := BuildToSignPacketSimple(message, pkScript)
	require.NoError(t, err)

	finalTx := packet.UnsignedTx.Copy()
	utxo := packet.Inputs[0].WitnessUtxo
	prevOutFetcher := txscript.NewCannedPrevOutputFetcher(
		utxo.PkScript, utxo.Value,
	)
	sigHashes := txscript.NewTxSigHashes(finalTx, prevOutFetcher)

	witness := taprootHtlcWitness(
		t, packet, htlcScript, preimage, controlBlockBytes,
		txscript.SigHashDefault, receiverKey,
	)
	finalTx.TxIn[0].Witness = witness

	// Sanity check: the witness is valid under raw consensus rules.
	sigHashes = txscript.NewTxSigHashes(finalTx, prevOutFetcher)
	vm, err := txscript.NewEngine(
		utxo.PkScript, finalTx, 0, txscript.StandardVerifyFlags, nil,
		sigHashes, utxo.Value, prevOutFetcher,
	)
	require.NoError(t, err)
	require.NoError(t, vm.Execute())

	// BIP-322 verification must reach the same conclusion: valid.
	valid, _, err := VerifyMessageSimple(message, pkScript, witness)
	require.NoError(t, err)
	require.True(t, valid)
}

// TestVerifyTaprootKeyPathRejectsBadSigHash asserts that the restricted-sighash
// policy is enforced for a taproot key-spend: a P2TR key-path spend whose lone
// witness item is a 65-byte Schnorr signature with an explicit, disallowed
// sighash flag must be rejected by the script engine.
func TestVerifyTaprootKeyPathRejectsBadSigHash(t *testing.T) {
	t.Parallel()

	message := []byte("probe")

	internalKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	outputKey := txscript.ComputeTaprootKeyNoScript(internalKey.PubKey())
	pkScript, err := txscript.PayToTaprootScript(outputKey)
	require.NoError(t, err)

	packet, err := BuildToSignPacketSimple(message, pkScript)
	require.NoError(t, err)

	finalTx := packet.UnsignedTx.Copy()
	utxo := packet.Inputs[0].WitnessUtxo
	prevOutFetcher := txscript.NewCannedPrevOutputFetcher(
		utxo.PkScript, utxo.Value,
	)
	sigHashes := txscript.NewTxSigHashes(finalTx, prevOutFetcher)

	// Sign the key-path spend with an explicit, non-default sighash flag.
	// This produces a 65-byte signature that BIP-322 does not allow.
	witness, err := txscript.TaprootWitnessSignature(
		finalTx, sigHashes, 0, utxo.Value, utxo.PkScript,
		txscript.SigHashSingle, internalKey,
	)
	require.NoError(t, err)
	require.Len(t, witness[0], 65)

	valid, _, err := VerifyMessageSimple(message, pkScript, witness)
	require.ErrorIs(t, err, ErrInvalidSigHashFlag)
	require.False(t, valid)
}

// TestVerifyMessageSimpleAcceptsP2WSHScriptData is the ECDSA/segwit-v0
// analog of TestVerifyMessageSimpleAcceptsTaprootScriptPathData: a valid
// P2WSH script-path spend must not be rejected just because one of its
// (non-signature) stack inputs happens to look like a DER-encoded ECDSA
// signature with a disallowed sighash byte.
//
// The probe uses an HTLC-style witness script whose hashlock branch consumes a
// 32-byte preimage crafted to resemble a DER-encoded ECDSA signature: it starts
// with 0x30, its second byte equals len-3, and its final byte is SigHashNone.
// Because the restricted-sighash policy is enforced by the script engine, the
// preimage is never checked (it is consumed by OP_SHA256, not OP_CHECKSIG). The
// real signature in the witness uses SIGHASH_ALL, so BIP-322 gives no reason to
// reject the message.
func TestVerifyMessageSimpleAcceptsP2WSHScriptData(t *testing.T) {
	t.Parallel()

	message := []byte("probe")

	// Build a 32-byte preimage shaped like a DER ECDSA signature whose
	// trailing byte is a (disallowed) explicit sighash flag.
	preimage := make([]byte, 32)
	preimage[0] = 0x30
	preimage[1] = byte(len(preimage) - 3)
	preimage[len(preimage)-1] = byte(txscript.SigHashNone)
	paymentHash := sha256.Sum256(preimage)

	htlcScript, receiverKey := makeHtlcScript(t, paymentHash, false)
	pkScript := witnessScriptChallenge(t, htlcScript, txscript.OP_0)
	packet, err := BuildToSignPacketSimple(message, pkScript)
	require.NoError(t, err)

	finalTx := packet.UnsignedTx.Copy()
	utxo := packet.Inputs[0].WitnessUtxo
	prevOutFetcher := txscript.NewCannedPrevOutputFetcher(
		utxo.PkScript, utxo.Value,
	)
	sigHashes := txscript.NewTxSigHashes(finalTx, prevOutFetcher)
	receiverSigWithSighashAll, err := txscript.RawTxInWitnessSignature(
		finalTx, sigHashes, 0, utxo.Value, htlcScript,
		txscript.SigHashAll, receiverKey,
	)
	require.NoError(t, err)

	witness := wire.TxWitness{
		receiverSigWithSighashAll,
		preimage,
		[]byte{1}, // Select the preimage (hashlock) branch.
		htlcScript,
	}
	finalTx.TxIn[0].Witness = witness

	// Sanity check: the witness is valid under raw consensus rules.
	sigHashes = txscript.NewTxSigHashes(finalTx, prevOutFetcher)
	vm, err := txscript.NewEngine(
		utxo.PkScript, finalTx, 0, txscript.StandardVerifyFlags, nil,
		sigHashes, utxo.Value, prevOutFetcher,
	)
	require.NoError(t, err)
	require.NoError(t, vm.Execute())

	// BIP-322 verification must reach the same conclusion: valid.
	valid, _, err := VerifyMessageSimple(message, pkScript, witness)
	require.NoError(t, err)
	require.True(t, valid)
}

// TestVerifyP2WPKHRejectsBadSigHash is the ECDSA analog of
// TestVerifyTaprootKeyPathRejectsBadSigHash: a P2WPKH spend whose signature
// uses a disallowed (non-SIGHASH_ALL) sighash flag must be rejected by the
// script engine.
func TestVerifyP2WPKHRejectsBadSigHash(t *testing.T) {
	t.Parallel()

	message := []byte("probe")

	key, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	pubKeyHash := address.Hash160(key.PubKey().SerializeCompressed())
	pkScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(pubKeyHash).
		Script()
	require.NoError(t, err)

	packet, err := BuildToSignPacketSimple(message, pkScript)
	require.NoError(t, err)

	finalTx := packet.UnsignedTx.Copy()
	utxo := packet.Inputs[0].WitnessUtxo
	prevOutFetcher := txscript.NewCannedPrevOutputFetcher(
		utxo.PkScript, utxo.Value,
	)
	sigHashes := txscript.NewTxSigHashes(finalTx, prevOutFetcher)

	// Sign with a disallowed (non-SIGHASH_ALL) sighash flag.
	sig, err := txscript.RawTxInWitnessSignature(
		finalTx, sigHashes, 0, utxo.Value, utxo.PkScript,
		txscript.SigHashNone, key,
	)
	require.NoError(t, err)

	witness := wire.TxWitness{sig, key.PubKey().SerializeCompressed()}

	valid, _, err := VerifyMessageSimple(message, pkScript, witness)
	require.ErrorIs(t, err, ErrInvalidSigHashFlag)
	require.False(t, valid)
}

// TestVerifyTaprootScriptPathRejectsBadSigHash exercises the capability that a
// static witness inspection cannot provide: rejecting a disallowed sighash flag
// on a signature that is consumed by a CHECKSIG inside a custom tapscript. The
// signature is a real, otherwise-valid SIGHASH_SINGLE Schnorr signature on the
// hashlock branch of an HTLC; only the engine, which observes the actual
// signature opcode, can soundly reject it.
func TestVerifyTaprootScriptPathRejectsBadSigHash(t *testing.T) {
	t.Parallel()

	message := []byte("probe")

	preimage := make([]byte, 32)
	for i := range preimage {
		preimage[i] = byte(i + 1)
	}
	paymentHash := sha256.Sum256(preimage)

	htlcScript, receiverKey := makeHtlcScript(t, paymentHash, true)
	pkScript, controlBlockBytes := taprootWitness(t, htlcScript)

	packet, err := BuildToSignPacketSimple(message, pkScript)
	require.NoError(t, err)

	// Sign the hashlock branch with a disallowed sighash flag. This is a
	// genuine, cryptographically valid signature; only its sighash type is
	// disallowed by BIP-322.
	witness := taprootHtlcWitness(
		t, packet, htlcScript, preimage, controlBlockBytes,
		txscript.SigHashSingle, receiverKey,
	)

	valid, _, err := VerifyMessageSimple(message, pkScript, witness)
	require.ErrorIs(t, err, ErrInvalidSigHashFlag)
	require.False(t, valid)
}

// TestVerifyP2WSHMultisigRejectsBadSigHash asserts that the sighash rule is
// enforced for every signature consumed by an OP_CHECKMULTISIG: a 2-of-2 P2WSH
// multisig whose second signature uses a disallowed sighash flag must be
// rejected, even though the first signature uses SIGHASH_ALL.
func TestVerifyP2WSHMultisigRejectsBadSigHash(t *testing.T) {
	t.Parallel()

	message := []byte("probe")

	key1, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	key2, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	witnessScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_2).
		AddData(key1.PubKey().SerializeCompressed()).
		AddData(key2.PubKey().SerializeCompressed()).
		AddOp(txscript.OP_2).
		AddOp(txscript.OP_CHECKMULTISIG).
		Script()
	require.NoError(t, err)

	pkScript := witnessScriptChallenge(t, witnessScript, txscript.OP_0)
	packet, err := BuildToSignPacketSimple(message, pkScript)
	require.NoError(t, err)

	finalTx := packet.UnsignedTx.Copy()
	utxo := packet.Inputs[0].WitnessUtxo
	prevOutFetcher := txscript.NewCannedPrevOutputFetcher(
		utxo.PkScript, utxo.Value,
	)
	sigHashes := txscript.NewTxSigHashes(finalTx, prevOutFetcher)

	// The first signer uses SIGHASH_ALL (allowed); the second uses
	// SIGHASH_SINGLE (disallowed). The engine inspects every signature
	// consumed by the multisig, so the message must be rejected.
	sig1, err := txscript.RawTxInWitnessSignature(
		finalTx, sigHashes, 0, utxo.Value, witnessScript,
		txscript.SigHashAll, key1,
	)
	require.NoError(t, err)
	sig2, err := txscript.RawTxInWitnessSignature(
		finalTx, sigHashes, 0, utxo.Value, witnessScript,
		txscript.SigHashSingle, key2,
	)
	require.NoError(t, err)

	// The leading empty item is the well-known OP_CHECKMULTISIG dummy.
	witness := wire.TxWitness{nil, sig1, sig2, witnessScript}

	valid, _, err := VerifyMessageSimple(message, pkScript, witness)
	require.ErrorIs(t, err, ErrInvalidSigHashFlag)
	require.False(t, valid)
}

// TestVerifyMessageSimpleReservedNOPIsInconclusive tests that using a reserved
// OP_NOPx returns the ErrInconclusive error.
func TestVerifyMessageSimpleReservedNOPIsInconclusive(t *testing.T) {
	t.Parallel()

	witnessScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_NOP5).
		AddOp(txscript.OP_TRUE).
		Script()
	require.NoError(t, err)

	pkScript := witnessScriptChallenge(t, witnessScript, txscript.OP_0)
	valid, _, err := VerifyMessageSimple(
		[]byte("probe"), pkScript, wire.TxWitness{witnessScript},
	)
	require.False(t, valid)
	require.ErrorIs(t, err, ErrInconclusive)
	require.False(t, errors.Is(err, ErrInvalidSignature))
}

// TestVerifyMessageFullFutureWitnessVersionIsInconclusive tests that future
// witness versions are returned as inconclusive.
func TestVerifyMessageFullFutureWitnessVersionIsInconclusive(t *testing.T) {
	t.Parallel()

	witnessScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_TRUE).
		Script()
	require.NoError(t, err)

	pkScript := witnessScriptChallenge(t, witnessScript, txscript.OP_2)
	valid, _, err := VerifyMessageFull(
		[]byte("probe"), pkScript, nil, wire.TxWitness{witnessScript},
		0, 0, 0,
	)
	require.False(t, valid)
	require.ErrorIs(t, err, ErrInconclusive)
	require.False(t, errors.Is(err, ErrInvalidSignature))
}

// TestVerifyMessageTxVersionIsInconclusive tests that future transaction
// versions (or the version 1) are not allowed and are validated as
// inconclusive.
func TestVerifyMessageTxVersionIsInconclusive(t *testing.T) {
	t.Parallel()

	const (
		txVersion1 = 1
		txVersion3 = 3
	)

	pkScript, witness, witnessBytes := opTrueChallenge(t)
	valid, _, err := VerifyMessageFull(
		[]byte("probe"), pkScript, nil, witness, txVersion1, 0, 0,
	)
	require.ErrorIs(t, err, ErrInconclusive)
	require.False(t, valid)

	packet := BuildToSignPacketFull(
		[]byte("probe"), pkScript, txVersion3, 0, 0,
	)
	packet.Inputs[0].FinalScriptWitness = witnessBytes
	valid, _, err = VerifyMessagePoF([]byte("probe"), pkScript, packet)
	require.ErrorIs(t, err, ErrInconclusive)
	require.False(t, valid)
}

// TestVerifyMessagePoFWitnessUtxoHandling tests that the (non-) witness UTXO
// fields are verified correctly in a proof-of-funds transaction.
func TestVerifyMessagePoFWitnessUtxoHandling(t *testing.T) {
	message := []byte("probe")

	// Use a native segwit OP_TRUE challenge for the first BIP-322 input so
	// the test stays focused on the additional proof-of-funds input.
	pkScript, _, witnessBytes := opTrueChallenge(t)

	packet := BuildToSignPacketFull(message, pkScript, 0, 0, 0)
	packet.Inputs[0].FinalScriptWitness = witnessBytes

	// Add a legacy P2PKH PoF input. For this input type, the PSBT must
	// carry a NonWitnessUtxo (or find it via the BIP-322 same-tx
	// optimization), not just a bare WitnessUtxo.
	legacyKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	pubKey := legacyKey.PubKey().SerializeCompressed()
	pubKeyHash := address.Hash160(pubKey)
	legacyAddr, err := address.NewAddressPubKeyHash(
		pubKeyHash, &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	legacyPkScript, err := txscript.PayToAddrScript(legacyAddr)
	require.NoError(t, err)

	legacyPrevHash := chainhash.HashH([]byte("legacy prevout"))
	packet.UnsignedTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  legacyPrevHash,
			Index: 0,
		},
	})

	packet.Inputs = append(packet.Inputs, psbt.PInput{
		WitnessUtxo: &wire.TxOut{
			Value:    1000,
			PkScript: legacyPkScript,
		},
	})
	require.Nil(t, packet.Inputs[1].NonWitnessUtxo)

	legacySigScript, err := txscript.SignatureScript(
		packet.UnsignedTx, 1, legacyPkScript, txscript.SigHashAll,
		legacyKey, true,
	)
	require.NoError(t, err)
	packet.Inputs[1].FinalScriptSig = legacySigScript

	signature, err := SerializeSignature(packet)
	require.NoError(t, err)

	valid, _, err := verifyMessageForChallenge(message, pkScript, signature)

	require.ErrorContains(
		t, err, "only witness utxo present for non-witness",
	)
	require.False(t, valid)

	// Additional P2WSH OP_TRUE input claims claimedHash:0, but supplies a
	// different NonWitnessUtxo. The current verifier verifies against
	// prevTx.TxOut[0] anyway.
	prevTx := wire.NewMsgTx(2)
	prevTx.AddTxIn(&wire.TxIn{})
	prevTx.AddTxOut(&wire.TxOut{Value: 1, PkScript: pkScript})

	var claimedHash chainhash.Hash
	claimedHash[0] = 0x42
	require.NotEqual(t, prevTx.TxHash(), claimedHash)

	// Update the second (PoF) input to a new one.
	packet.UnsignedTx.TxIn[1].PreviousOutPoint = wire.OutPoint{
		Hash:  claimedHash,
		Index: 0,
	}
	packet.Inputs[1] = psbt.PInput{
		NonWitnessUtxo:     prevTx,
		FinalScriptWitness: witnessBytes,
	}

	signature, err = SerializeSignature(packet)
	require.NoError(t, err)
	valid, _, err = verifyMessageForChallenge(message, pkScript, signature)
	require.ErrorContains(
		t, err, "non witness utxo does not match input prevout",
	)
	require.False(t, valid)
}

// TestVerifyMessagePoFRejectsDuplicateAdditionalInputs tests that a
// proof-of-funds transaction rejects duplicate additional inputs.
func TestVerifyMessagePoFRejectsDuplicateAdditionalInputs(t *testing.T) {
	message := []byte("probe")
	pkScript, _, witnessBytes := opTrueChallenge(t)

	packet := BuildToSignPacketFull(message, pkScript, 0, 0, 0)
	packet.Inputs[0].FinalScriptWitness = witnessBytes

	prevTx := wire.NewMsgTx(2)
	prevTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 0xffffffff},
	})
	prevTx.AddTxOut(&wire.TxOut{
		Value:    1,
		PkScript: pkScript,
	})

	duplicatePrevOut := wire.OutPoint{
		Hash:  prevTx.TxHash(),
		Index: 0,
	}
	for i := 0; i < 2; i++ {
		packet.UnsignedTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: duplicatePrevOut,
		})
		packet.Inputs = append(packet.Inputs, psbt.PInput{
			WitnessUtxo: &wire.TxOut{
				Value:    1,
				PkScript: pkScript,
			},
			FinalScriptWitness: witnessBytes,
		})
	}

	signature, err := SerializeSignature(packet)
	require.NoError(t, err)

	valid, _, err := verifyMessageForChallenge(
		message, pkScript, signature,
	)

	require.Error(t, err)
	require.False(t, valid)
}

// TestVerifyMessagePoFRejectsNullAdditionalInput tests that a proof-of-funds
// transaction rejects null outpoint additional inputs.
func TestVerifyMessagePoFRejectsNullAdditionalInput(t *testing.T) {
	message := []byte("probe")
	pkScript, _, witnessBytes := opTrueChallenge(t)

	packet := BuildToSignPacketFull(message, pkScript, 0, 0, 0)
	packet.Inputs[0].FinalScriptWitness = witnessBytes

	packet.UnsignedTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 0xffffffff},
	})
	packet.Inputs = append(packet.Inputs, psbt.PInput{
		WitnessUtxo: &wire.TxOut{
			Value:    1,
			PkScript: pkScript,
		},
		FinalScriptWitness: witnessBytes,
	})

	signature, err := SerializeSignature(packet)
	require.NoError(t, err)

	valid, _, err := verifyMessageForChallenge(
		message, pkScript, signature,
	)

	require.Error(t, err)
	require.False(t, valid)
}

// TestVerifyMessagePoFRejectsOversizeTransaction makes sure too large
// transactions (more than 1 MB of stripped size) are rejected.
func TestVerifyMessagePoFRejectsOversizeTransaction(t *testing.T) {
	message := []byte("probe")

	pkScriptOpTrue, _, witnessBytesOpTrue := opTrueChallenge(t)

	packet := BuildToSignPacketFull(message, pkScriptOpTrue, 0, 0, 0)
	packet.Inputs[0].FinalScriptWitness = witnessBytesOpTrue

	const numInputs = 30_000
	for i := 1; i < numInputs; i++ {
		var h chainhash.Hash
		h[0] = byte(i)
		h[1] = byte(i >> 8)
		h[2] = byte(i >> 16)
		h[3] = byte(i >> 24)
		packet.UnsignedTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  h,
				Index: 0,
			},
		})
		packet.Inputs = append(packet.Inputs, psbt.PInput{
			WitnessUtxo: &wire.TxOut{
				Value:    1,
				PkScript: pkScriptOpTrue,
			},
			FinalScriptWitness: witnessBytesOpTrue,
		})
	}

	require.Greater(t, packet.UnsignedTx.SerializeSizeStripped(), numInputs)

	valid, _, err := VerifyMessagePoF(message, pkScriptOpTrue, packet)
	require.ErrorIs(t, err, ErrInvalidToSign)
	require.False(t, valid)
}

// TestProbeVerifyMessagePoFDirectRejectsMalformedToSignOutput tests that the
// VerifyMessagePoF function properly rejects malformed transactions.
func TestProbeVerifyMessagePoFDirectRejectsMalformedToSignOutput(t *testing.T) {
	message := []byte("probe")
	pkScript, _, witnessBytes := opTrueChallenge(t)

	packet := BuildToSignPacketFull(message, pkScript, 0, 0, 0)
	packet.Inputs[0].FinalScriptWitness = witnessBytes
	packet.UnsignedTx.TxOut[0].PkScript = []byte{txscript.OP_TRUE}

	valid, _, err := VerifyMessagePoF(message, pkScript, packet)

	require.ErrorIs(t, err, ErrInvalidToSign)
	require.False(t, valid)
}
