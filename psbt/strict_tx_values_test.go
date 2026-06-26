package psbt

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// strictnessTxPair returns a minimal unsigned transaction and the previous
// transaction provided as its non-witness UTXO.
func strictnessTxPair(t *testing.T) (*wire.MsgTx, *wire.MsgTx) {
	t.Helper()

	prevTx := wire.NewMsgTx(2)
	prevTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: wire.MaxPrevOutIndex},
		Sequence:         wire.MaxTxInSequenceNum,
	})
	prevTx.AddTxOut(&wire.TxOut{
		Value:    12345,
		PkScript: []byte{0x51},
	})

	unsignedTx := wire.NewMsgTx(2)
	unsignedTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  prevTx.TxHash(),
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})
	unsignedTx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x51},
	})

	return unsignedTx, prevTx
}

// serializeTxForStrictness serializes tx in the PSBT form required by the
// field under test.
func serializeTxForStrictness(t *testing.T, tx *wire.MsgTx,
	noWitness bool) []byte {

	t.Helper()

	var buf bytes.Buffer
	var err error
	if noWitness {
		err = tx.SerializeNoWitness(&buf)
	} else {
		err = tx.Serialize(&buf)
	}
	require.NoError(t, err)

	return buf.Bytes()
}

// strictnessPSBT builds a minimal PSBT using the supplied transaction-valued
// fields verbatim.
func strictnessPSBT(t *testing.T, unsignedTx,
	nonWitnessUtxo []byte) []byte {

	t.Helper()

	var buf bytes.Buffer
	_, err := buf.Write(psbtMagic[:])
	require.NoError(t, err)

	require.NoError(t, serializeKVPairWithType(
		&buf, byte(UnsignedTxType), nil, unsignedTx,
	))
	require.NoError(t, buf.WriteByte(0x00))

	require.NoError(t, serializeKVPairWithType(
		&buf, byte(NonWitnessUtxoType), nil, nonWitnessUtxo,
	))
	require.NoError(t, buf.WriteByte(0x00))
	require.NoError(t, buf.WriteByte(0x00))

	return buf.Bytes()
}

// serializeTxOutForStrictness serializes txOut in the PSBT form required by
// the WitnessUtxo field.
func serializeTxOutForStrictness(t *testing.T, txOut *wire.TxOut) []byte {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, wire.WriteTxOut(&buf, 0, 0, txOut))

	return buf.Bytes()
}

// strictnessPSBTWithWitnessUtxo builds a minimal PSBT using the supplied
// WitnessUtxo value verbatim.
func strictnessPSBTWithWitnessUtxo(t *testing.T,
	witnessUtxo []byte) []byte {

	t.Helper()

	unsignedTx, _ := strictnessTxPair(t)

	var buf bytes.Buffer
	_, err := buf.Write(psbtMagic[:])
	require.NoError(t, err)

	require.NoError(t, serializeKVPairWithType(
		&buf, byte(UnsignedTxType), nil,
		serializeTxForStrictness(t, unsignedTx, true),
	))
	require.NoError(t, buf.WriteByte(0x00))

	require.NoError(t, serializeKVPairWithType(
		&buf, byte(WitnessUtxoType), nil, witnessUtxo,
	))
	require.NoError(t, buf.WriteByte(0x00))
	require.NoError(t, buf.WriteByte(0x00))

	return buf.Bytes()
}

// TestRejectsTrailingDataInTransactionValues verifies that PSBT transaction
// values must contain exactly one serialized transaction.
func TestRejectsTrailingDataInTransactionValues(t *testing.T) {
	unsignedTx, prevTx := strictnessTxPair(t)
	unsignedTxBytes := serializeTxForStrictness(t, unsignedTx, true)
	prevTxBytes := serializeTxForStrictness(t, prevTx, false)

	testCases := []struct {
		name           string
		unsignedTx     []byte
		nonWitnessUtxo []byte
	}{{
		name:           "global unsigned tx",
		unsignedTx:     append(unsignedTxBytes, 0x00),
		nonWitnessUtxo: prevTxBytes,
	}, {
		name:           "input non-witness utxo",
		unsignedTx:     unsignedTxBytes,
		nonWitnessUtxo: append(prevTxBytes, 0x00),
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewFromRawBytes(bytes.NewReader(
				strictnessPSBT(
					t, tc.unsignedTx, tc.nonWitnessUtxo,
				),
			), false)
			require.ErrorIs(t, err, ErrInvalidPsbtFormat)
		})
	}
}

// TestRejectsTrailingDataAfterPacket verifies that extra bytes after a valid
// PSBT packet are rejected.
func TestRejectsTrailingDataAfterPacket(t *testing.T) {
	unsignedTx, prevTx := strictnessTxPair(t)
	rawPacket := strictnessPSBT(
		t,
		serializeTxForStrictness(t, unsignedTx, true),
		serializeTxForStrictness(t, prevTx, false),
	)

	_, err := NewFromRawBytes(
		bytes.NewReader(append(rawPacket, 0x00)), false,
	)
	require.ErrorIs(t, err, ErrInvalidPsbtFormat)
}

// TestRejectsNonCanonicalBase64Packet verifies that base64 PSBT input rejects
// whitespace, bad padding, and extra decoded packet bytes.
func TestRejectsNonCanonicalBase64Packet(t *testing.T) {
	unsignedTx, prevTx := strictnessTxPair(t)
	rawPacket := strictnessPSBT(
		t,
		serializeTxForStrictness(t, unsignedTx, true),
		serializeTxForStrictness(t, prevTx, false),
	)
	encoded := base64.StdEncoding.EncodeToString(rawPacket)
	insert := func(idx int, s string) string {
		return encoded[:idx] + s + encoded[idx:]
	}

	testCases := []struct {
		name    string
		encoded string
	}{{
		name:    "trailing LF",
		encoded: encoded + "\n",
	}, {
		name:    "trailing CRLF",
		encoded: encoded + "\r\n",
	}, {
		name:    "LF between groups",
		encoded: insert(4, "\n"),
	}, {
		name:    "LF inside group",
		encoded: insert(5, "\n"),
	}, {
		name:    "space between groups",
		encoded: insert(4, " "),
	}, {
		name:    "tab inside group",
		encoded: insert(5, "\t"),
	}, {
		name:    "padding in middle",
		encoded: insert(len(encoded)-4, "="),
	}, {
		name: "extra decoded bytes",
		encoded: base64.StdEncoding.EncodeToString(
			append(append([]byte{}, rawPacket...), 0x00),
		),
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewFromRawBytes(
				bytes.NewReader([]byte(tc.encoded)), true,
			)
			require.ErrorIs(t, err, ErrInvalidPsbtFormat)
		})
	}
}

// TestParsesWitnessUtxoTxOutStrictly verifies that WitnessUtxo values are
// parsed as exact transaction outputs.
func TestParsesWitnessUtxoTxOutStrictly(t *testing.T) {
	pkScript := bytes.Repeat([]byte{0x51}, 253)
	txOutBytes := serializeTxOutForStrictness(t, &wire.TxOut{
		Value:    1234,
		PkScript: pkScript,
	})

	packet, err := NewFromRawBytes(bytes.NewReader(
		strictnessPSBTWithWitnessUtxo(t, txOutBytes),
	), false)
	require.NoError(t, err)
	require.Equal(t, pkScript, packet.Inputs[0].WitnessUtxo.PkScript)

	malformedTxOut := append(append([]byte{}, txOutBytes...), 0x00)
	_, err = NewFromRawBytes(bytes.NewReader(
		strictnessPSBTWithWitnessUtxo(t, malformedTxOut),
	), false)
	require.ErrorIs(t, err, ErrInvalidPsbtFormat)
}

// TestParsesWitnessUtxoTxOutCompactSizeScriptLength verifies that WitnessUtxo
// scripts with multi-byte CompactSize lengths are parsed without folding the
// length bytes into the script.
func TestParsesWitnessUtxoTxOutCompactSizeScriptLength(t *testing.T) {
	pkScript := bytes.Repeat([]byte{0x51}, 300)
	txOutBytes := serializeTxOutForStrictness(t, &wire.TxOut{
		Value:    1234,
		PkScript: pkScript,
	})
	require.Equal(t, byte(0xfd), txOutBytes[8])

	packet, err := NewFromRawBytes(bytes.NewReader(
		strictnessPSBTWithWitnessUtxo(t, txOutBytes),
	), false)
	require.NoError(t, err)
	require.Equal(t, pkScript, packet.Inputs[0].WitnessUtxo.PkScript)
}
