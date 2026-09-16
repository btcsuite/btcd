package bip322

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/psbt/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

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

// TestVerifyMessageFullLegacyMinimalIf tests that BIP-322 applies MINIMALIF to
// legacy P2SH redeem scripts for both conditional opcodes and both permitted
// boolean encodings. Negative cases are covered by the negative test vectors.
func TestVerifyMessageFullLegacyMinimalIf(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		condition   byte
		conditional byte
	}{
		{
			name:        "if false",
			condition:   txscript.OP_0,
			conditional: txscript.OP_IF,
		},
		{
			name:        "if true",
			condition:   txscript.OP_1,
			conditional: txscript.OP_IF,
		},
		{
			name:        "notif false",
			condition:   txscript.OP_0,
			conditional: txscript.OP_NOTIF,
		},
		{
			name:        "notif true",
			condition:   txscript.OP_1,
			conditional: txscript.OP_NOTIF,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Both branches succeed so the condition encoding is
			// the only property that determines whether
			// verification passes.
			redeemScript, err := txscript.NewScriptBuilder().
				AddOp(tc.conditional).
				AddOp(txscript.OP_TRUE).
				AddOp(txscript.OP_ELSE).
				AddOp(txscript.OP_TRUE).
				AddOp(txscript.OP_ENDIF).
				Script()
			require.NoError(t, err)

			pkScript, err := txscript.NewScriptBuilder().
				AddOp(txscript.OP_HASH160).
				AddData(address.Hash160(redeemScript)).
				AddOp(txscript.OP_EQUAL).
				Script()
			require.NoError(t, err)

			// The condition is pushed minimally even in the
			// rejected cases. MINIMALIF restricts the value itself
			// to an empty vector or 0x01, independently of
			// minimal-push encoding.
			sigScript, err := txscript.NewScriptBuilder().
				AddOp(tc.condition).
				AddData(redeemScript).
				Script()
			require.NoError(t, err)

			valid, _, err := VerifyMessageFull(
				[]byte("probe"), pkScript, sigScript, nil, 0, 0,
				0,
			)
			require.NoError(t, err)
			require.True(t, valid)
		})
	}
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

// TestValidateUtxoCorrectness tests that validateUtxoCorrectness enforces the
// BIP-174 rule that an input spending a non-segwit output must convey it via
// a non-witness UTXO. Only a bare witness UTXO on a non-segwit input is
// rejected; segwit inputs (native or P2SH-nested) and inputs that also carry a
// non-witness UTXO are accepted.
func TestValidateUtxoCorrectness(t *testing.T) {
	t.Parallel()

	key, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	// Build one output script of each relevant type.
	p2wpkh, err := payToWitnessPubKeyHashScript(key)
	require.NoError(t, err)

	p2wsh := witnessScriptChallenge(
		t, []byte{txscript.OP_TRUE}, txscript.OP_0,
	)

	p2tr, err := payToTaprootScript(key)
	require.NoError(t, err)

	p2pkh, err := payToPubKeyHashScript(key)
	require.NoError(t, err)

	// A bare pay-to-pubkey output, another legacy (non-segwit) type.
	p2pk, err := txscript.NewScriptBuilder().
		AddData(key.PubKey().SerializeCompressed()).
		AddOp(txscript.OP_CHECKSIG).Script()
	require.NoError(t, err)

	// A nested P2SH-P2WPKH output plus the finalized scriptSig that reveals
	// its witness redeem script (a single push of the witness program).
	nestedP2sh, witProg, err := payToNestedWitnessPubKeyHashScript(key)
	require.NoError(t, err)
	nestedSig, err := txscript.NewScriptBuilder().
		AddData(witProg).Script()
	require.NoError(t, err)

	// A P2SH output that wraps a *non*-witness redeem script, plus the
	// scriptSig revealing it. This is a legacy P2SH spend, not segwit.
	legacyRedeem := []byte{txscript.OP_TRUE}
	p2shLegacy, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_HASH160).
		AddData(address.Hash160(legacyRedeem)).
		AddOp(txscript.OP_EQUAL).
		Script()
	require.NoError(t, err)
	p2shLegacySig, err := txscript.NewScriptBuilder().
		AddData(legacyRedeem).
		Script()
	require.NoError(t, err)

	// txOut builds a UTXO output for the given script.
	txOut := func(pkScript []byte) *wire.TxOut {
		return &wire.TxOut{Value: 1000, PkScript: pkScript}
	}

	// prevTx builds a stand-in previous transaction with a single output of
	// the given script, for use as a non-witness UTXO.
	prevTx := func(pkScript []byte) *wire.MsgTx {
		tx := wire.NewMsgTx(2)
		tx.AddTxIn(&wire.TxIn{})
		tx.AddTxOut(txOut(pkScript))
		return tx
	}

	testCases := []struct {
		name      string
		inputs    []psbt.PInput
		expectErr string
	}{{
		name:   "native p2wpkh witness utxo",
		inputs: []psbt.PInput{{WitnessUtxo: txOut(p2wpkh)}},
	}, {
		name:   "native p2wsh witness utxo",
		inputs: []psbt.PInput{{WitnessUtxo: txOut(p2wsh)}},
	}, {
		name:   "native p2tr witness utxo",
		inputs: []psbt.PInput{{WitnessUtxo: txOut(p2tr)}},
	}, {
		name: "nested p2sh-p2wpkh witness utxo",
		inputs: []psbt.PInput{{
			WitnessUtxo:    txOut(nestedP2sh),
			FinalScriptSig: nestedSig,
		}},
	}, {
		name:   "legacy p2pkh non-witness utxo only",
		inputs: []psbt.PInput{{NonWitnessUtxo: prevTx(p2pkh)}},
	}, {
		name:   "segwit p2wpkh non-witness utxo only",
		inputs: []psbt.PInput{{NonWitnessUtxo: prevTx(p2wpkh)}},
	}, {
		name: "legacy p2pkh with both utxo fields",
		inputs: []psbt.PInput{{
			WitnessUtxo:    txOut(p2pkh),
			NonWitnessUtxo: prevTx(p2pkh),
		}},
		expectErr: "input 0",
	}, {
		name:   "neither utxo field set",
		inputs: []psbt.PInput{{}},
	}, {
		name:      "legacy p2pkh witness utxo only",
		inputs:    []psbt.PInput{{WitnessUtxo: txOut(p2pkh)}},
		expectErr: "input 0",
	}, {
		name:      "bare p2pk witness utxo only",
		inputs:    []psbt.PInput{{WitnessUtxo: txOut(p2pk)}},
		expectErr: "input 0",
	}, {
		name: "legacy p2sh witness utxo only",
		inputs: []psbt.PInput{{
			WitnessUtxo:    txOut(p2shLegacy),
			FinalScriptSig: p2shLegacySig,
		}},
		expectErr: "input 0",
	}, {
		name:      "nested p2sh witness utxo without redeem",
		inputs:    []psbt.PInput{{WitnessUtxo: txOut(nestedP2sh)}},
		expectErr: "input 0",
	}, {
		name: "valid segwit followed by bare legacy",
		inputs: []psbt.PInput{
			{WitnessUtxo: txOut(p2wpkh)},
			{WitnessUtxo: txOut(p2pkh)},
		},
		expectErr: "input 1",
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			packet := &psbt.Packet{Inputs: tc.inputs}
			err := validateUtxoCorrectness(packet)
			if tc.expectErr != "" {
				require.ErrorContains(t, err, tc.expectErr)
				require.ErrorContains(
					t, err, "non-segwit script",
				)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestVerifyMessagePoFMatchingUtxos tests that a proof-of-funds input carrying
// both UTXO representations is accepted only when their amounts and scripts
// match exactly.
func TestVerifyMessagePoFMatchingUtxos(t *testing.T) {
	message := []byte("probe")
	pkScript, _, witnessBytes := opTrueChallenge(t)

	testCases := []struct {
		name        string
		witnessUtxo func(*wire.TxOut) *wire.TxOut
	}{
		{
			name: "matching",
			witnessUtxo: func(prevOut *wire.TxOut) *wire.TxOut {
				return &wire.TxOut{
					Value:    prevOut.Value,
					PkScript: bytes.Clone(prevOut.PkScript),
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// The proof's additional input spends a real output and
			// provides both allowed PSBT representations of it.
			prevTx := wire.NewMsgTx(2)
			prevTx.AddTxIn(&wire.TxIn{})
			prevTx.AddTxOut(&wire.TxOut{
				Value:    1,
				PkScript: pkScript,
			})

			packet, err := BuildToSignPacketFull(
				message, pkScript, 0, 0, 0,
			)
			require.NoError(t, err)
			packet.Inputs[0].FinalScriptWitness = witnessBytes
			packet.UnsignedTx.AddTxIn(&wire.TxIn{
				PreviousOutPoint: wire.OutPoint{
					Hash:  prevTx.TxHash(),
					Index: 0,
				},
			})
			packet.Inputs = append(packet.Inputs, psbt.PInput{
				NonWitnessUtxo: prevTx,
				WitnessUtxo: tc.witnessUtxo(
					prevTx.TxOut[0],
				),
				FinalScriptWitness: witnessBytes,
			})

			valid, _, err := VerifyMessagePoF(
				message, pkScript, packet,
			)
			require.NoError(t, err)
			require.True(t, valid)
		})
	}
}

// TestVerifyMessagePoFRejectsOversizeTransaction makes sure too large
// transactions (more than 1 MB of stripped size) are rejected.
func TestVerifyMessagePoFRejectsOversizeTransaction(t *testing.T) {
	message := []byte("probe")

	pkScriptOpTrue, _, witnessBytesOpTrue := opTrueChallenge(t)

	packet, err := BuildToSignPacketFull(message, pkScriptOpTrue, 0, 0, 0)
	require.NoError(t, err)

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

// TestVerifyMessagePoFTransactionWeight tests that a proof-of-funds
// transaction is accepted at the consensus weight limit and rejected one
// weight unit above it.
func TestVerifyMessagePoFTransactionWeight(t *testing.T) {
	const (
		numAdditionalInputs = 392
		maxAnnexSize        = 10_000
	)

	message := []byte("probe")
	pkScript, _, witnessBytes := opTrueChallenge(t)
	packet, err := BuildToSignPacketFull(message, pkScript, 0, 0, 0)
	require.NoError(t, err)
	packet.Inputs[0].FinalScriptWitness = witnessBytes

	// Taproot annexes contribute witness weight without affecting script
	// execution. Enough independent inputs make the transaction overweight
	// while every individual witness item remains within its size limit.
	tapScript, controlBlock := taprootWitness(
		t, []byte{txscript.OP_TRUE},
	)
	annex := make([]byte, maxAnnexSize)
	annex[0] = txscript.TaprootAnnexTag
	inputWitness, err := SerializeTxWitness(wire.TxWitness{
		{txscript.OP_TRUE}, controlBlock, annex,
	})
	require.NoError(t, err)
	for i := 1; i <= numAdditionalInputs; i++ {
		var hash chainhash.Hash
		hash[0] = byte(i)
		hash[1] = byte(i >> 8)

		packet.UnsignedTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{Hash: hash},
		})

		packet.Inputs = append(packet.Inputs, psbt.PInput{
			WitnessUtxo: &wire.TxOut{
				Value:    1,
				PkScript: tapScript,
			},
			FinalScriptWitness: inputWitness,
		})
	}

	// Shorten the final annex by the measured excess. Since witness bytes
	// each contribute one weight unit, this places the extracted
	// transaction exactly at the limit without duplicating serialization
	// size constants in the test.
	finalTx, err := psbt.Extract(packet)
	require.NoError(t, err)
	weight := finalTx.SerializeSizeStripped()*3 + finalTx.SerializeSize()
	excessWeight := weight - maxWitnessItems
	require.Positive(t, excessWeight)
	require.Less(t, excessWeight, len(annex))

	annex = annex[:len(annex)-excessWeight]
	lastInput := &packet.Inputs[len(packet.Inputs)-1]
	lastInput.FinalScriptWitness, err = SerializeTxWitness(wire.TxWitness{
		{txscript.OP_TRUE}, controlBlock, annex,
	})
	require.NoError(t, err)

	finalTx, err = psbt.Extract(packet)
	require.NoError(t, err)
	weight = finalTx.SerializeSizeStripped()*3 + finalTx.SerializeSize()
	require.Equal(t, maxWitnessItems, weight)

	valid, _, err := VerifyMessagePoF(message, pkScript, packet)
	require.NoError(t, err)
	require.True(t, valid)

	// Growing the annex by one byte crosses the limit without changing the
	// transaction's base serialization or script validity.
	annex = append(annex, 0)
	lastInput.FinalScriptWitness, err = SerializeTxWitness(wire.TxWitness{
		{txscript.OP_TRUE}, controlBlock, annex,
	})
	require.NoError(t, err)

	finalTx, err = psbt.Extract(packet)
	require.NoError(t, err)
	weight = finalTx.SerializeSizeStripped()*3 + finalTx.SerializeSize()
	require.Equal(t, maxWitnessItems+1, weight)

	valid, _, err = VerifyMessagePoF(message, pkScript, packet)
	require.ErrorContains(t, err, "weight exceeds maximum")
	require.False(t, valid)
}
