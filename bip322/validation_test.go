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
// weight unit above it. The weight comes from a single tapscript leaf, as a
// taproot witness item is only limited by the transaction weight and not by
// the script size limit of non-taproot scripts.
func TestVerifyMessagePoFTransactionWeight(t *testing.T) {
	message := []byte("probe")
	pkScript, _, witnessBytes := opTrueChallenge(t)
	packet, err := BuildToSignPacketFull(message, pkScript, 0, 0, 0)
	require.NoError(t, err)
	packet.Inputs[0].FinalScriptWitness = witnessBytes

	packet.UnsignedTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Hash: chainhash.Hash{0x01}},
	})
	packet.Inputs = append(packet.Inputs, psbt.PInput{})

	// setLeafSize lets the additional input spend a taproot output through
	// a leaf script of the given size. OP_NOP has no effect, so the leaf
	// script adds weight without affecting its validity.
	setLeafSize := func(leafSize int) {
		leafScript := append(
			bytes.Repeat([]byte{txscript.OP_NOP}, leafSize-1),
			txscript.OP_TRUE,
		)
		tapScript, controlBlock := taprootWitness(t, leafScript)
		inputWitness, err := SerializeTxWitness(wire.TxWitness{
			leafScript, controlBlock,
		})
		require.NoError(t, err)

		packet.Inputs[1] = psbt.PInput{
			WitnessUtxo: &wire.TxOut{
				Value:    1,
				PkScript: tapScript,
			},
			FinalScriptWitness: inputWitness,
		}
	}

	// txWeight returns the weight of the transaction extracted from the
	// packet.
	txWeight := func() int {
		finalTx, err := psbt.Extract(packet)
		require.NoError(t, err)

		return finalTx.SerializeSizeStripped()*3 +
			finalTx.SerializeSize()
	}

	// Start with a leaf script that is slightly too large, then shorten it
	// by the measured excess. Since witness bytes each contribute one
	// weight unit, this places the extracted transaction exactly at the
	// limit without duplicating serialization size constants in the test.
	setLeafSize(maxWitnessItems - 100)
	excessWeight := txWeight() - maxWitnessItems
	require.Positive(t, excessWeight)

	leafSize := maxWitnessItems - 100 - excessWeight
	require.Greater(t, leafSize, txscript.MaxScriptSize)
	setLeafSize(leafSize)
	require.Equal(t, maxWitnessItems, txWeight())

	valid, _, err := VerifyMessagePoF(message, pkScript, packet)
	require.NoError(t, err)
	require.True(t, valid)

	// Growing the leaf script by one byte crosses the limit without
	// changing the transaction's base serialization or script validity.
	setLeafSize(leafSize + 1)
	require.Equal(t, maxWitnessItems+1, txWeight())

	valid, _, err = VerifyMessagePoF(message, pkScript, packet)
	require.ErrorContains(t, err, "weight exceeds maximum")
	require.False(t, valid)
}

// unexecutedSigOpsScript returns a script that contains the given opcodes in a
// branch that is never executed, followed by OP_TRUE. Signature operations are
// counted statically, so the opcodes add to the signature operation cost of an
// input spending the script without requiring any signatures.
func unexecutedSigOpsScript(t *testing.T, ops ...byte) []byte {
	t.Helper()

	script, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_FALSE).
		AddOp(txscript.OP_IF).
		AddOps(ops).
		AddOp(txscript.OP_ENDIF).
		AddOp(txscript.OP_TRUE).
		Script()
	require.NoError(t, err)

	return script
}

// maxSigOpsRedeemScript returns a P2SH redeem script with the highest
// signature operation count that still executes successfully. Each
// OP_CHECKMULTISIG that isn't preceded by a small integer counts as 20
// signature operations, and two of the allowed non-push opcodes are needed for
// the unexecuted branch. Spending the script therefore costs
// 199 * 20 * witnessScaleFactor = 15,920.
func maxSigOpsRedeemScript(t *testing.T) []byte {
	t.Helper()

	return unexecutedSigOpsScript(t, bytes.Repeat(
		[]byte{txscript.OP_CHECKMULTISIG}, txscript.MaxOpsPerScript-2,
	)...)
}

// p2shScript returns the P2SH pkScript for the given redeem script.
func p2shScript(t *testing.T, redeemScript []byte) []byte {
	t.Helper()

	pkScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_HASH160).
		AddData(address.Hash160(redeemScript)).
		AddOp(txscript.OP_EQUAL).
		Script()
	require.NoError(t, err)

	return pkScript
}

// pushDataScript returns a script that only pushes the given data items.
func pushDataScript(t *testing.T, items ...[]byte) []byte {
	t.Helper()

	builder := txscript.NewScriptBuilder()
	for _, item := range items {
		builder.AddData(item)
	}
	script, err := builder.Script()
	require.NoError(t, err)

	return script
}

// TestTxSigOpCost tests that the signature operation cost of a transaction is
// calculated according to BIP-141 for every input and output type.
func TestTxSigOpCost(t *testing.T) {
	t.Parallel()

	// The signatures and keys are never verified, only their shape
	// matters for counting signature operations.
	sig := bytes.Repeat([]byte{0x30}, 72)
	pubKey := bytes.Repeat([]byte{0x02}, 33)
	pubKeyHash := address.Hash160(pubKey)

	// A 2-of-2 multisig script counts as 2 signature operations when
	// counted precisely and as 20 when counted the legacy way.
	multiSig := bareMultisigScript(t)

	p2pkh, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_DUP).
		AddOp(txscript.OP_HASH160).
		AddData(pubKeyHash).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_CHECKSIG).
		Script()
	require.NoError(t, err)

	p2wpkh, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(pubKeyHash).
		Script()
	require.NoError(t, err)

	p2wsh := witnessScriptChallenge(t, multiSig, txscript.OP_0)

	tapLeaf, err := txscript.NewScriptBuilder().
		AddData(pubKey[1:]).
		AddOp(txscript.OP_CHECKSIG).
		Script()
	require.NoError(t, err)
	p2tr, controlBlock := taprootWitness(t, tapLeaf)

	maxSigOpsRedeem := maxSigOpsRedeemScript(t)

	testCases := []struct {
		name      string
		pkScript  []byte
		sigScript []byte
		witness   wire.TxWitness
		outScript []byte
		expected  int
	}{{
		name:      "p2pkh",
		pkScript:  p2pkh,
		sigScript: pushDataScript(t, sig, pubKey),
		expected:  0,
	}, {
		name:      "bare multisig",
		pkScript:  multiSig,
		sigScript: pushDataScript(t, nil, sig, sig),
		expected:  0,
	}, {
		name:     "non-push signature script",
		pkScript: p2pkh,
		sigScript: []byte{
			txscript.OP_CHECKSIG, txscript.OP_CHECKMULTISIG,
		},
		expected: (1 + 20) * witnessScaleFactor,
	}, {
		name:      "p2sh multisig",
		pkScript:  p2shScript(t, multiSig),
		sigScript: pushDataScript(t, nil, sig, sig, multiSig),
		expected:  2 * witnessScaleFactor,
	}, {
		name:      "p2sh max sigops",
		pkScript:  p2shScript(t, maxSigOpsRedeem),
		sigScript: pushDataScript(t, maxSigOpsRedeem),
		expected:  199 * 20 * witnessScaleFactor,
	}, {
		name:     "p2wpkh",
		pkScript: p2wpkh,
		witness:  wire.TxWitness{sig, pubKey},
		expected: 1,
	}, {
		name:     "p2wsh multisig",
		pkScript: p2wsh,
		witness:  wire.TxWitness{nil, sig, sig, multiSig},
		expected: 2,
	}, {
		name:      "np2wpkh",
		pkScript:  p2shScript(t, p2wpkh),
		sigScript: pushDataScript(t, p2wpkh),
		witness:   wire.TxWitness{sig, pubKey},
		expected:  1,
	}, {
		name:      "np2wsh multisig",
		pkScript:  p2shScript(t, p2wsh),
		sigScript: pushDataScript(t, p2wsh),
		witness:   wire.TxWitness{nil, sig, sig, multiSig},
		expected:  2,
	}, {
		name:     "p2tr key path",
		pkScript: p2tr,
		witness:  wire.TxWitness{sig[:64]},
		expected: 0,
	}, {
		name:     "p2tr script path",
		pkScript: p2tr,
		witness:  wire.TxWitness{sig[:64], tapLeaf, controlBlock},
		expected: 0,
	}, {
		name:      "output with sigops",
		pkScript:  p2wpkh,
		witness:   wire.TxWitness{sig, pubKey},
		outScript: p2pkh,
		expected:  1 + witnessScaleFactor,
	}}

	prevOut := wire.OutPoint{Hash: chainhash.Hash{0x01}}
	newTx := func(sigScript []byte, witness wire.TxWitness,
		outScript []byte) *wire.MsgTx {

		if outScript == nil {
			outScript = []byte{txscript.OP_RETURN}
		}

		tx := wire.NewMsgTx(0)
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: prevOut,
			SignatureScript:  sigScript,
			Witness:          witness,
		})
		tx.AddTxOut(&wire.TxOut{PkScript: outScript})

		return tx
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tx := newTx(tc.sigScript, tc.witness, tc.outScript)
			utxos := map[wire.OutPoint]*wire.TxOut{
				prevOut: {Value: 1, PkScript: tc.pkScript},
			}

			cost, err := txSigOpCost(tx, utxos)
			require.NoError(t, err)
			require.Equal(t, tc.expected, cost)
		})
	}

	t.Run("missing utxo", func(t *testing.T) {
		t.Parallel()

		tx := newTx(nil, wire.TxWitness{sig, pubKey}, nil)
		_, err := txSigOpCost(tx, nil)
		require.ErrorContains(t, err, "missing UTXO for input index 0")
	})
}

// TestVerifyMessagePoFSigOpCost tests that a proof-of-funds transaction is
// accepted at the consensus signature operation cost limit and rejected one
// unit above it.
func TestVerifyMessagePoFSigOpCost(t *testing.T) {
	t.Parallel()

	const numP2SHInputs = 5

	message := []byte("probe")
	pkScript, _, witnessBytes := opTrueChallenge(t)

	// The P2SH inputs bring the cost close to the limit with as few inputs
	// as possible, while every script still executes successfully.
	redeemScript := maxSigOpsRedeemScript(t)
	redeemPkScript := p2shScript(t, redeemScript)
	sigScript := pushDataScript(t, redeemScript)
	p2shCost := numP2SHInputs * (txscript.MaxOpsPerScript - 2) *
		txscript.MaxPubKeysPerMultiSig * witnessScaleFactor
	remainingCost := maxTxSigOpCost - p2shCost
	require.Positive(t, remainingCost)

	prevTx := wire.NewMsgTx(2)
	prevTx.AddTxIn(&wire.TxIn{})
	for range numP2SHInputs {
		prevTx.AddTxOut(&wire.TxOut{Value: 1, PkScript: redeemPkScript})
	}

	// newPacket creates a proof-of-funds packet with the P2SH inputs and a
	// final P2WSH input that adds the given number of unscaled witness
	// signature operations, allowing the cost to be set to an exact value.
	newPacket := func(witnessSigOps int) *psbt.Packet {
		packet, err := BuildToSignPacketFull(message, pkScript, 0, 0, 0)
		require.NoError(t, err)
		packet.Inputs[0].FinalScriptWitness = witnessBytes

		for idx := range uint32(numP2SHInputs) {
			packet.UnsignedTx.AddTxIn(&wire.TxIn{
				PreviousOutPoint: wire.OutPoint{
					Hash:  prevTx.TxHash(),
					Index: idx,
				},
			})
			packet.Inputs = append(packet.Inputs, psbt.PInput{
				NonWitnessUtxo: prevTx,
				FinalScriptSig: sigScript,
			})
		}

		// A precisely counted OP_16 OP_CHECKMULTISIG counts as 16
		// signature operations, an OP_CHECKSIG as one.
		var ops []byte
		for range witnessSigOps / 16 {
			ops = append(
				ops, txscript.OP_16, txscript.OP_CHECKMULTISIG,
			)
		}
		for range witnessSigOps % 16 {
			ops = append(ops, txscript.OP_CHECKSIG)
		}
		witnessScript := unexecutedSigOpsScript(t, ops...)
		witness, err := SerializeTxWitness(
			wire.TxWitness{witnessScript},
		)
		require.NoError(t, err)

		packet.UnsignedTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash: chainhash.Hash{0x01},
			},
		})
		packet.Inputs = append(packet.Inputs, psbt.PInput{
			WitnessUtxo: &wire.TxOut{
				Value: 1,
				PkScript: witnessScriptChallenge(
					t, witnessScript, txscript.OP_0,
				),
			},
			FinalScriptWitness: witness,
		})

		return packet
	}

	valid, _, err := VerifyMessagePoF(
		message, pkScript, newPacket(remainingCost),
	)
	require.NoError(t, err)
	require.True(t, valid)

	valid, _, err = VerifyMessagePoF(
		message, pkScript, newPacket(remainingCost+1),
	)
	require.ErrorContains(
		t, err, "signature operation cost 80001 exceeds maximum of "+
			"80000",
	)
	require.False(t, valid)
}
