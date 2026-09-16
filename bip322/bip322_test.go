package bip322

import (
	"bytes"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/psbt/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// encodeWitnessHeader writes a (count, items...) witness stack into a buffer
// where each item is described by (length, data). Used for crafting edge-case
// payloads below.
func encodeWitnessHeader(t *testing.T, witCount uint64,
	items ...[]byte) []byte {

	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, wire.WriteVarInt(&buf, 0, witCount))
	for _, item := range items {
		require.NoError(t, wire.WriteVarInt(&buf, 0, uint64(len(item))))
		_, err := buf.Write(item)
		require.NoError(t, err)
	}
	return buf.Bytes()
}

// TestParseTxWitnessRawTooLarge asserts that ParseTxWitness rejects raw
// inputs larger than maxWitnessItems up front.
func TestParseTxWitnessRawTooLarge(t *testing.T) {
	raw := make([]byte, maxWitnessItems+1)
	_, err := ParseTxWitness(raw)
	require.ErrorIs(t, err, errSignatureTooLarge)
}

// TestParseTxWitnessValidSmallStack asserts that ordinary, well-formed witness
// stacks inside the new bounds still parse.
func TestParseTxWitnessValidSmallStack(t *testing.T) {
	sig := bytes.Repeat([]byte{0xaa}, 72)
	pub := bytes.Repeat([]byte{0xbb}, 33)

	raw := encodeWitnessHeader(t, 2, sig, pub)

	stack, err := ParseTxWitness(raw)
	require.NoError(t, err)
	require.Len(t, stack, 2)
	require.Equal(t, sig, stack[0])
	require.Equal(t, pub, stack[1])
}

// TestParseTxTooLarge asserts that ParseTx rejects raw inputs larger than
// maxWitnessItems.
func TestParseTxTooLarge(t *testing.T) {
	raw := make([]byte, maxWitnessItems+1)
	_, err := ParseTx(raw)
	require.ErrorIs(t, err, errSignatureTooLarge)
}

// TestVerifyMessageNilAddress asserts that VerifyMessage returns a clean
// error (not a panic) when the caller passes a nil address.
func TestVerifyMessageNilAddress(t *testing.T) {
	require.NotPanics(t, func() {
		valid, _, err := VerifyMessage("msg", nil, "AA==")
		require.False(t, valid)
		require.ErrorIs(t, err, errNilAddress)
	})
}

// TestVerifyMessageSignatureTooLarge asserts that VerifyMessage rejects
// signatures whose base64-decoded size exceeds maxWitnessItems.
func TestVerifyMessageSignatureTooLarge(t *testing.T) {
	addr, err := address.DecodeAddress(
		"bc1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l",
		&chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	huge := make([]byte, maxWitnessItems+1)
	sig := b64Encode(huge)

	valid, _, err := VerifyMessage("msg", addr, sig)
	require.False(t, valid)
	require.ErrorIs(t, err, errSignatureTooLarge)
}

// bareMultisigScript builds OP_2 <pub1> <pub2> OP_2 OP_CHECKMULTISIG — a
// script that is neither P2PK, P2PKH, P2SH, nor any native SegWit type.
// Under the old exclusion-list gate this was silently accepted; under the
// inclusion-list gate it must be rejected.
func bareMultisigScript(t *testing.T) []byte {
	t.Helper()

	// Two arbitrary 33-byte compressed pubkeys (bytes are not valid
	// curve points, but txscript.PayToAddrScript isn't involved here —
	// we just need the script shape).
	pub1 := bytes.Repeat([]byte{0x02}, 33)
	pub2 := bytes.Repeat([]byte{0x03}, 33)

	b := txscript.NewScriptBuilder()
	b.AddOp(txscript.OP_2)
	b.AddData(pub1)
	b.AddData(pub2)
	b.AddOp(txscript.OP_2)
	b.AddOp(txscript.OP_CHECKMULTISIG)

	script, err := b.Script()
	require.NoError(t, err)
	return script
}

// TestBuildToSignPacketSimpleRejectsNonNativeSegWit asserts that
// BuildToSignPacketSimple rejects every non-native-SegWit script type,
// including ones that slipped through the previous exclusion-list gate.
func TestBuildToSignPacketSimpleRejectsNonNativeSegWit(t *testing.T) {
	cases := []struct {
		name     string
		pkScript []byte
	}{
		{
			name:     "empty pkScript",
			pkScript: nil,
		},
		{
			name:     "OP_RETURN",
			pkScript: []byte{txscript.OP_RETURN},
		},
		{
			name:     "bare multisig",
			pkScript: bareMultisigScript(t),
		},
		{
			// A plausible-looking but non-existent future witness
			// version. We want the gate to fail *closed* on
			// unknown versions.
			name: "future witness version (v2)",
			pkScript: append(
				[]byte{txscript.OP_2, 0x20},
				bytes.Repeat([]byte{0xaa}, 32)...,
			),
		},
		{
			// Random garbage bytes.
			name:     "random bytes",
			pkScript: bytes.Repeat([]byte{0x42}, 20),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildToSignPacketSimple(
				[]byte("msg"), tc.pkScript,
			)
			require.ErrorIs(t, err, errSimpleSegWitOnly)
		})
	}
}

// TestVerifyMessageSimpleRejectsNonNativeSegWit mirrors the BuildToSignPacket
// test for the verification path.
func TestVerifyMessageSimpleRejectsNonNativeSegWit(t *testing.T) {
	scripts := map[string][]byte{
		"OP_RETURN":     {txscript.OP_RETURN},
		"bare multisig": bareMultisigScript(t),
		"random bytes":  bytes.Repeat([]byte{0x42}, 20),
	}

	for name, pkScript := range scripts {
		t.Run(name, func(t *testing.T) {
			valid, _, err := VerifyMessageSimple(
				[]byte("msg"), pkScript, wire.TxWitness{},
			)
			require.False(t, valid)
			require.ErrorIs(t, err, errSimpleSegWitOnly)
		})
	}
}

// TestBuildToSignPacketSimpleAcceptsNativeSegWit asserts that the inclusion
// list still accepts all three native SegWit script types.
func TestBuildToSignPacketSimpleAcceptsNativeSegWit(t *testing.T) {
	// Minimal but well-formed pkScripts for each native SegWit type.
	p2wpkh := append(
		[]byte{txscript.OP_0, 0x14},
		bytes.Repeat([]byte{0xaa}, 20)...,
	)
	p2wsh := append(
		[]byte{txscript.OP_0, 0x20},
		bytes.Repeat([]byte{0xbb}, 32)...,
	)
	p2tr := append(
		[]byte{txscript.OP_1, 0x20},
		bytes.Repeat([]byte{0xcc}, 32)...,
	)

	cases := map[string][]byte{
		"p2wpkh": p2wpkh,
		"p2wsh":  p2wsh,
		"p2tr":   p2tr,
	}
	for name, pkScript := range cases {
		t.Run(name, func(t *testing.T) {
			packet, err := BuildToSignPacketSimple(
				[]byte("msg"), pkScript,
			)
			require.NoError(t, err)
			require.NotNil(t, packet)
		})
	}
}

// TestVerifyMessageDecodesBase64Error asserts the base64 decode path still
// returns a clean error (not a panic or bool=true).
func TestVerifyMessageDecodesBase64Error(t *testing.T) {
	addr, err := address.DecodeAddress(
		"bc1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l",
		&chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	valid, _, err := VerifyMessage("msg", addr, "not valid base64!!!")
	require.False(t, valid)
	require.Error(t, err)
	require.True(
		t, strings.Contains(err.Error(), "base64"),
		"expected base64 error, got %q", err.Error(),
	)
}

// makeUtxoTx returns a fresh transaction whose non-witness content (and thus
// its txid) is determined solely by tag: two calls with the same tag produce
// distinct *wire.MsgTx values that share the same TxHash, while different tags
// produce different TxHashes. When withWitness is set, witness data is attached
// to the input. Because a txid is computed over the non-witness serialization,
// the witness must NOT change the TxHash, so a witness-bearing tx still
// deduplicates against a witness-less tx with the same tag.
func makeUtxoTx(tag int64, withWitness bool) *wire.MsgTx {
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 0xffffffff},
		Sequence:         0xffffffff,
	})
	if withWitness {
		tx.TxIn[0].Witness = wire.TxWitness{{0x01, 0x02, 0x03}}
	}
	tx.AddTxOut(&wire.TxOut{
		Value:    tag,
		PkScript: []byte{txscript.OP_TRUE},
	})

	return tx
}

// dedupInput describes a single PSBT input to construct for the
// deduplicateNonWitnessUtxos test.
type dedupInput struct {
	// noUtxo indicates the input has no NonWitnessUtxo (it is left nil).
	noUtxo bool

	// tag identifies the NonWitnessUtxo content; inputs with the same tag
	// share a txid. Ignored when noUtxo is true.
	tag int64

	// withWitness attaches witness data to the NonWitnessUtxo transaction.
	// Since a txid ignores witness data, such a transaction still
	// deduplicates against a witness-less transaction with the same tag.
	withWitness bool

	// withWitnessUtxo also sets a WitnessUtxo on the input. The
	// deduplication must never touch the WitnessUtxo field.
	withWitnessUtxo bool
}

// TestDeduplicateNonWitnessUtxos exercises deduplicateNonWitnessUtxos across a
// range of inputs. The function must keep the first occurrence of each distinct
// NonWitnessUtxo transaction (identified by txid), set any later duplicates to
// nil, always preserve input 0, skip inputs that have no NonWitnessUtxo, and
// never modify the WitnessUtxo field.
func TestDeduplicateNonWitnessUtxos(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		inputs []dedupInput

		// want[i] is true if input i must retain its NonWitnessUtxo
		// after deduplication, and false if it must be nil.
		want []bool
	}{
		{
			name:   "no inputs",
			inputs: nil,
			want:   nil,
		},
		{
			name:   "single input with utxo is kept",
			inputs: []dedupInput{{tag: 1}},
			want:   []bool{true},
		},
		{
			name:   "single input without utxo stays nil",
			inputs: []dedupInput{{noUtxo: true}},
			want:   []bool{false},
		},
		{
			name:   "two inputs same utxo dedups the second",
			inputs: []dedupInput{{tag: 1}, {tag: 1}},
			want:   []bool{true, false},
		},
		{
			name:   "two inputs distinct utxos both kept",
			inputs: []dedupInput{{tag: 1}, {tag: 2}},
			want:   []bool{true, true},
		},
		{
			name: "three identical utxos keep only the first",
			inputs: []dedupInput{
				{tag: 1}, {tag: 1}, {tag: 1},
			},
			want: []bool{true, false, false},
		},
		{
			name: "duplicate of first input with unique middle",
			inputs: []dedupInput{
				{tag: 1}, {tag: 2}, {tag: 1},
			},
			want: []bool{true, true, false},
		},
		{
			name: "interleaved duplicates A B A B",
			inputs: []dedupInput{
				{tag: 1}, {tag: 2}, {tag: 1}, {tag: 2},
			},
			want: []bool{true, true, false, false},
		},
		{
			name: "first non-nil utxo after a nil input is kept",
			inputs: []dedupInput{
				{noUtxo: true}, {tag: 1}, {tag: 1},
			},
			want: []bool{false, true, false},
		},
		{
			name: "nil input between duplicates",
			inputs: []dedupInput{
				{tag: 1}, {noUtxo: true}, {tag: 1},
			},
			want: []bool{true, false, false},
		},
		{
			name: "all inputs without a utxo",
			inputs: []dedupInput{
				{noUtxo: true}, {noUtxo: true}, {noUtxo: true},
			},
			want: []bool{false, false, false},
		},
		{
			name: "first utxo duplicated repeatedly around a " +
				"unique one",
			inputs: []dedupInput{
				{tag: 1}, {tag: 1}, {tag: 2}, {tag: 1},
			},
			want: []bool{true, false, true, false},
		},
		{
			name: "complex mix of nils and duplicates",
			inputs: []dedupInput{
				{noUtxo: true}, {tag: 1}, {tag: 2},
				{tag: 1}, {noUtxo: true}, {tag: 2},
			},
			want: []bool{false, true, true, false, false, false},
		},
		{
			name: "duplicate detected by txid ignores witness data",
			inputs: []dedupInput{
				{tag: 1}, {tag: 1, withWitness: true},
			},
			want: []bool{true, false},
		},
		{
			name: "witness-bearing utxo as the first occurrence",
			inputs: []dedupInput{
				{tag: 1, withWitness: true}, {tag: 1},
			},
			want: []bool{true, false},
		},
		{
			name: "witness utxo preserved on a deduplicated input",
			inputs: []dedupInput{
				{tag: 1, withWitnessUtxo: true},
				{tag: 1, withWitnessUtxo: true},
			},
			want: []bool{true, false},
		},
		{
			name: "input zero is always kept even if it has a " +
				"later twin",
			inputs: []dedupInput{
				{tag: 7}, {tag: 7},
			},
			want: []bool{true, false},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Len(t, tc.want, len(tc.inputs),
				"test setup: want must match inputs length")

			// Build the PSBT packet from the input specs.
			packet := &psbt.Packet{}
			for _, in := range tc.inputs {
				pin := psbt.PInput{}
				if !in.noUtxo {
					pin.NonWitnessUtxo = makeUtxoTx(
						in.tag, in.withWitness,
					)
				}
				if in.withWitnessUtxo {
					pin.WitnessUtxo = &wire.TxOut{
						Value: 1000,
						PkScript: []byte{
							txscript.OP_TRUE,
						},
					}
				}
				packet.Inputs = append(packet.Inputs, pin)
			}

			deduplicateNonWitnessUtxos(packet)

			// The deduplication must never add or remove inputs.
			require.Len(t, packet.Inputs, len(tc.inputs),
				"input count must not change")

			for i, in := range tc.inputs {
				got := packet.Inputs[i].NonWitnessUtxo

				if tc.want[i] {
					require.NotNilf(t, got, "input %d "+
						"should keep its "+
						"NonWitnessUtxo", i)

					// A retained utxo must be left
					// unchanged.
					want := makeUtxoTx(
						in.tag, in.withWitness,
					)
					require.Equalf(
						t, want.TxHash(), got.TxHash(),
						"input %d retained the wrong "+
							"transaction", i,
					)
				} else {
					require.Nilf(t, got, "input %d should "+
						"be deduplicated to nil", i)
				}

				// The WitnessUtxo field must never be touched.
				if in.withWitnessUtxo {
					require.NotNilf(
						t, packet.Inputs[i].WitnessUtxo,
						"input %d WitnessUtxo must be "+
							"preserved", i,
					)
				}
			}
		})
	}
}

// TestVerifyMessagePoFDoesNotMutatePacket ensures verification is read-only.
func TestVerifyMessagePoFDoesNotMutatePacket(t *testing.T) {
	message := []byte("probe")
	pkScript, _, witnessBytes := opTrueChallenge(t)

	packet, err := BuildToSignPacketFull(message, pkScript, 0, 0, 0)
	require.NoError(t, err)

	packet.Inputs[0].FinalScriptWitness = witnessBytes

	prevTx := wire.NewMsgTx(2)
	prevTx.AddTxIn(&wire.TxIn{})
	prevTx.AddTxOut(&wire.TxOut{Value: 1, PkScript: pkScript})
	prevTx.AddTxOut(&wire.TxOut{Value: 1, PkScript: pkScript})
	prevHash := prevTx.TxHash()

	for idx := range uint32(2) {
		packet.UnsignedTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  prevHash,
				Index: idx,
			},
		})
		packet.Inputs = append(packet.Inputs, psbt.PInput{
			NonWitnessUtxo:     prevTx,
			FinalScriptWitness: witnessBytes,
		})
	}

	require.NotNil(t, packet.Inputs[2].NonWitnessUtxo)
	valid, _, err := VerifyMessagePoF(message, pkScript, packet)
	require.NoError(t, err)
	require.True(t, valid)
	require.NotNil(
		t, packet.Inputs[2].NonWitnessUtxo,
		"VerifyMessagePoF must not mutate the caller-provided PSBT",
	)
}

// TestProbeVerifyMessagePoFDoesNotMutateOutputDerivationOrder tests that
// VerifyMessagePoF doesn't mutate the order of derivation paths in the packet.
func TestProbeVerifyMessagePoFDoesNotMutateOutputDerivationOrder(t *testing.T) {
	message := []byte("probe")
	pkScript, _, witnessBytes := opTrueChallenge(t)
	packet, err := BuildToSignPacketFull(message, pkScript, 0, 0, 0)
	require.NoError(t, err)

	packet.Inputs[0].FinalScriptWitness = witnessBytes

	prevTx := wire.NewMsgTx(2)
	prevTx.AddTxIn(&wire.TxIn{})
	prevTx.AddTxOut(&wire.TxOut{Value: 1, PkScript: pkScript})
	prevHash := prevTx.TxHash()
	packet.UnsignedTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  prevHash,
			Index: 0,
		},
	})
	packet.Inputs = append(packet.Inputs, psbt.PInput{
		WitnessUtxo:        prevTx.TxOut[0],
		FinalScriptWitness: witnessBytes,
	})

	keyA, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	keyB, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	pubA := keyA.PubKey().SerializeCompressed()
	pubB := keyB.PubKey().SerializeCompressed()
	if string(pubA) > string(pubB) {
		pubA, pubB = pubB, pubA
	}
	packet.Outputs[0].Bip32Derivation = []*psbt.Bip32Derivation{
		{PubKey: pubB},
		{PubKey: pubA},
	}

	require.Equal(t, pubB, packet.Outputs[0].Bip32Derivation[0].PubKey)
	valid, _, err := VerifyMessagePoF(message, pkScript, packet)
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(
		t, pubB, packet.Outputs[0].Bip32Derivation[0].PubKey,
		"VerifyMessagePoF must not mutate output metadata order",
	)
}

// TestVerifyMessagePoFReusesEarlierNonWitnessUtxo tests that an additional
// input can omit its NonWitnessUtxo when an earlier input supplied the same
// transaction.
func TestVerifyMessagePoFReusesEarlierNonWitnessUtxo(t *testing.T) {
	message := []byte("probe")
	pkScript, _, witnessBytes := opTrueChallenge(t)
	packet, err := BuildToSignPacketFull(message, pkScript, 0, 0, 0)
	require.NoError(t, err)
	packet.Inputs[0].FinalScriptWitness = witnessBytes

	// The two additional inputs spend distinct outputs from the same
	// transaction. Only the first carries the full transaction, exercising
	// the BIP-322 reuse rule without creating a duplicate outpoint.
	prevTx := wire.NewMsgTx(2)
	prevTx.AddTxIn(&wire.TxIn{})
	prevTx.AddTxOut(&wire.TxOut{Value: 1, PkScript: pkScript})
	prevTx.AddTxOut(&wire.TxOut{Value: 1, PkScript: pkScript})
	prevHash := prevTx.TxHash()

	for idx := range uint32(2) {
		packet.UnsignedTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  prevHash,
				Index: idx,
			},
		})

		input := psbt.PInput{FinalScriptWitness: witnessBytes}
		if idx == 0 {
			input.NonWitnessUtxo = prevTx
		}
		packet.Inputs = append(packet.Inputs, input)
	}

	valid, _, err := VerifyMessagePoF(message, pkScript, packet)
	require.NoError(t, err)
	require.True(t, valid)
}

// TestVerifyMessageNonUtf8 tests that a non-UTF-8 message returns a nice error.
func TestVerifyMessageNonUtf8(t *testing.T) {
	msg := []byte{0xff, 0xfe, 0xfd}
	_, _, err := VerifyMessagePoF(msg, nil, &psbt.Packet{})
	require.ErrorIs(t, err, errInvalidMessage)
}

// TestVerifyMessagePoFRejectsMissingOrMismatchedGenericSignedMessage tests that
// a mismatched PSBT_GLOBAL_GENERIC_SIGNED_MESSAGE field on the PoF PSBT is
// rejected.
func TestVerifyMessagePoFRejectsMissingOrMismatchedGenericSignedMessage(
	t *testing.T) {

	message := []byte("probe")
	pkScript, _, witnessBytes := opTrueChallenge(t)

	makeSig := func(t *testing.T, field *string) string {
		packet, err := BuildToSignPacketFull(message, pkScript, 0, 0, 0)
		require.NoError(t, err)

		packet.Inputs[0].FinalScriptWitness = witnessBytes
		packet.GenericSignedMessage = field

		prevTx := wire.NewMsgTx(2)
		prevTx.AddTxIn(&wire.TxIn{})
		prevTx.AddTxOut(&wire.TxOut{Value: 1, PkScript: pkScript})

		packet.UnsignedTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  prevTx.TxHash(),
				Index: 0,
			},
		})
		packet.Inputs = append(packet.Inputs, psbt.PInput{
			WitnessUtxo:        prevTx.TxOut[0],
			FinalScriptWitness: witnessBytes,
		})

		sig, err := SerializeSignature(packet)
		require.NoError(t, err)

		return sig
	}

	different := "different message"
	tests := []struct {
		name      string
		field     *string
		wantError bool
	}{
		{
			name:      "missing",
			field:     nil,
			wantError: false,
		},
		{
			name:      "mismatched",
			field:     &different,
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sig := makeSig(t, test.field)

			valid, _, err := verifyMessageForChallenge(
				message, pkScript, sig,
			)

			if test.wantError {
				require.Error(t, err)
				require.False(t, valid)
			} else {
				require.NoError(t, err)
				require.True(t, valid)
			}
		})
	}
}

// TestSerializeSignatureWitnessUtxoCorrectness tests that attempting to
// serialize a PoF signature fails if the UTXO information isn't correct for
// the input type.
func TestSerializeSignatureWitnessUtxoCorrectness(t *testing.T) {
	message := []byte("probe")
	pkScript, _, witnessBytes := opTrueChallenge(t)

	packet, err := BuildToSignPacketFull(message, pkScript, 0, 0, 0)
	require.NoError(t, err)

	packet.Inputs[0].FinalScriptWitness = witnessBytes

	key, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	legacyPkScript, err := payToPubKeyHashScript(key)
	require.NoError(t, err)

	prevTx := wire.NewMsgTx(2)
	prevTx.AddTxIn(&wire.TxIn{})
	prevTx.AddTxOut(&wire.TxOut{Value: 1, PkScript: legacyPkScript})
	prevTx.AddTxOut(&wire.TxOut{Value: 2, PkScript: legacyPkScript})
	prevHash := prevTx.TxHash()

	for outIdx := uint32(0); outIdx < 2; outIdx++ {
		packet.UnsignedTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  prevHash,
				Index: outIdx,
			},
		})
		packet.Inputs = append(packet.Inputs, psbt.PInput{
			NonWitnessUtxo: prevTx,
			WitnessUtxo:    prevTx.TxOut[outIdx],
		})
	}

	for inputIdx := 1; inputIdx <= 2; inputIdx++ {
		scriptSig, err := txscript.SignatureScript(
			packet.UnsignedTx, inputIdx, legacyPkScript,
			txscript.SigHashAll, key, true,
		)
		require.NoError(t, err)

		packet.Inputs[inputIdx].FinalScriptSig = scriptSig
	}

	_, err = SerializeSignature(packet)
	require.ErrorContains(
		t, err, "witness UTXO for spending a non-segwit script",
	)
}

// TestSerializeSignatureCheckOrder demonstrates that SerializeSignature runs
// validateUtxoCorrectness *before* IsComplete().
func TestSerializeSignatureCheckOrder(t *testing.T) {
	key, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	// Build a P2SH-P2WPKH pkScript. This puts the challenge input into
	// the P2SH-nested-segwit path, where BuildToSignPacketFull nils
	// WitnessUtxo on input 0 (so input 0 alone can't trigger the bug).
	pkScript, _, err := payToNestedWitnessPubKeyHashScript(key)
	require.NoError(t, err)

	packet, err := BuildToSignPacketFull([]byte("probe"), pkScript, 0, 0, 0)
	require.NoError(t, err)

	// Add a second, P2SH-nested-segwit PoF input that a wallet has
	// pre-populated with both UTXO fields but has NOT signed yet. This is
	// the exact state a wallet's outgoing PSBT is in before being handed
	// off for signing.
	prevTx := wire.NewMsgTx(2)
	prevTx.AddTxIn(&wire.TxIn{})
	prevTx.AddTxOut(&wire.TxOut{Value: 1000, PkScript: pkScript})

	packet.UnsignedTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  prevTx.TxHash(),
			Index: 0,
		},
	})

	// FinalScriptSig deliberately left nil — packet is unfinalized.
	packet.Inputs = append(packet.Inputs, psbt.PInput{
		WitnessUtxo:    prevTx.TxOut[0],
		NonWitnessUtxo: prevTx,
	})

	// The packet is not finalized. Ask SerializeSignature what it thinks.
	_, err = SerializeSignature(packet)
	require.Error(t, err)

	// The honest error would be "packet must be finalized"; today the
	// caller instead gets the misleading BIP-174-shaped error.
	require.ErrorContains(
		t, err, "packet must be finalized",
		"expected the honest finalized-first error; got the "+
			"misleading UTXO-correctness error because "+
			"validateUtxoCorrectness runs before IsComplete "+
			"in SerializeSignature",
	)
}
