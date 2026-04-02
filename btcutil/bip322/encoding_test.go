package bip322

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

func TestBIP322EncodeDecodeSimple(t *testing.T) {
	original := wire.TxWitness{
		{0x01, 0x02},
		{0x03, 0x04, 0x05},
	}

	encoded, err := EncodeSimple(original)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded, err := DecodeSimple(encoded)
	require.NoError(t, err)
	require.Equal(t, len(original), len(decoded))
	for i := range original {
		require.Equal(t, original[i], decoded[i])
	}
}

func TestBIP322EncodeDecodeFull(t *testing.T) {
	tx := wire.NewMsgTx(0)
	prevHash := wire.OutPoint{Index: 0xffffffff}
	txIn := wire.NewTxIn(&prevHash, nil, nil)
	txIn.Sequence = 0
	tx.AddTxIn(txIn)
	tx.AddTxOut(&wire.TxOut{Value: 0, PkScript: []byte{0x51}})
	tx.LockTime = 0

	encoded, err := EncodeFull(tx)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded, err := DecodeFull(encoded)
	require.NoError(t, err)
	require.Equal(t, tx.TxHash(), decoded.TxHash())
}

func TestBIP322DetectFormat(t *testing.T) {
	t.Run("compact 65 bytes rejected", func(t *testing.T) {
		payload := make([]byte, 65)
		payload[0] = 0x1f
		sig := base64.StdEncoding.EncodeToString(payload)
		_, err := DetectFormat(sig)
		require.Error(t, err)
	})

	t.Run("simple witness", func(t *testing.T) {
		witness := wire.TxWitness{{0x01, 0x02, 0x03}}
		encoded, err := EncodeSimple(witness)
		require.NoError(t, err)
		format, err := DetectFormat(encoded)
		require.NoError(t, err)
		require.Equal(t, FormatSimple, format)
	})

	t.Run("full transaction", func(t *testing.T) {
		tx := wire.NewMsgTx(wire.TxVersion)
		prevHash := wire.OutPoint{Index: 0xffffffff}
		txIn := wire.NewTxIn(&prevHash, nil, nil)
		tx.AddTxIn(txIn)
		tx.AddTxOut(&wire.TxOut{Value: 0, PkScript: []byte{0x51}})

		var buf bytes.Buffer
		require.NoError(t, tx.Serialize(&buf))
		encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

		format, err := DetectFormat(encoded)
		require.NoError(t, err)
		require.Equal(t, FormatFull, format)
	})

	t.Run("invalid base64", func(t *testing.T) {
		_, err := DetectFormat("not-valid-base64!!!")
		require.Error(t, err)
	})

	t.Run("valid base64 invalid content", func(t *testing.T) {
		payload := []byte{0xff, 0xfe, 0xfd, 0x00, 0x01}
		encoded := base64.StdEncoding.EncodeToString(payload)
		_, err := DetectFormat(encoded)
		require.Error(t, err)
	})
}

func TestBIP322EmptyWitness(t *testing.T) {
	_, err := EncodeSimple(nil)
	require.ErrorIs(t, err, ErrEmptyWitness)

	_, err = EncodeSimple(wire.TxWitness{})
	require.ErrorIs(t, err, ErrEmptyWitness)
}
