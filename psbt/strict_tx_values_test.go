package psbt

import (
	"bytes"
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
