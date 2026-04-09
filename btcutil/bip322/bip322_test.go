package bip322

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
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

// TestParseTxWitnessTooManyItems asserts that ParseTxWitness rejects witness
// stacks whose declared item count exceeds maxWitnessItems, bounding the
// upfront slice-header allocation.
func TestParseTxWitnessTooManyItems(t *testing.T) {
	// Just claim maxWitnessItems+1 items — no item data follows. If the
	// parser allocated first, it would commit ~24 MB from a 5-byte input.
	var buf bytes.Buffer
	require.NoError(t, wire.WriteVarInt(&buf, 0, uint64(maxWitnessItems+1)))

	_, err := ParseTxWitness(buf.Bytes())
	require.Error(t, err)
	require.Contains(t, err.Error(), "too many witness items")
}

// TestParseTxWitnessItemTooLarge asserts that ParseTxWitness rejects a
// witness stack containing a single item whose declared length exceeds
// maxWitnessItemBytes.
func TestParseTxWitnessItemTooLarge(t *testing.T) {
	// 1 item, declared length = maxWitnessItemBytes + 1, no body.
	var buf bytes.Buffer
	require.NoError(t, wire.WriteVarInt(&buf, 0, 1))
	require.NoError(
		t, wire.WriteVarInt(&buf, 0, uint64(maxWitnessItems+1)),
	)

	_, err := ParseTxWitness(buf.Bytes())
	require.Error(t, err)
	require.Contains(t, err.Error(), "witness item too large")
}

// TestParseTxWitnessItemExceedsRemaining asserts that ParseTxWitness rejects
// a witness item whose declared length is larger than the remaining input,
// BEFORE committing a potentially large allocation.
func TestParseTxWitnessItemExceedsRemaining(t *testing.T) {
	// 1 item, declared length = 100_000, but only 1 body byte available.
	var buf bytes.Buffer
	require.NoError(t, wire.WriteVarInt(&buf, 0, 1))
	require.NoError(t, wire.WriteVarInt(&buf, 0, 100_000))
	buf.WriteByte(0xaa)

	_, err := ParseTxWitness(buf.Bytes())
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds remaining input")
}

// TestParseTxWitnessRawTooLarge asserts that ParseTxWitness rejects raw
// inputs larger than maxSignatureBytes up front.
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
// maxSignatureBytes.
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
// signatures whose base64-decoded size exceeds maxSignatureBytes.
func TestVerifyMessageSignatureTooLarge(t *testing.T) {
	addr, err := btcutil.DecodeAddress(
		"bc1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l",
		&chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	huge := make([]byte, maxWitnessItems+1)
	sig := base64.StdEncoding.EncodeToString(huge)

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
	addr, err := btcutil.DecodeAddress(
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
