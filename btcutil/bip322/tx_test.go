package bip322

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/stretchr/testify/require"
)

func TestBIP322BuildToSpendTx(t *testing.T) {
	addr, err := btcutil.DecodeAddress(
		"bc1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l",
		&chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	scriptPubKey, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	tests := []struct {
		message      string
		expectedTXID string
	}{
		{
			message:      "",
			expectedTXID: "c5680aa69bb8d860bf82d4e9cd3504b55dde018de765a91bb566283c545a99a7",
		},
		{
			message:      "Hello World",
			expectedTXID: "b79d196740ad5217771c1098fc4a4b51e0535c32236c71f1ea4d61a2d603352b",
		},
	}

	for _, tc := range tests {
		tx, err := BuildToSpendTx(tc.message, scriptPubKey)
		require.NoError(t, err)

		require.Equal(t, int32(0), tx.Version)
		require.Equal(t, uint32(0), tx.LockTime)
		require.Len(t, tx.TxIn, 1)
		require.Len(t, tx.TxOut, 1)

		require.Equal(t, tc.expectedTXID, tx.TxHash().String())
	}
}

func TestBIP322BuildToSignTx(t *testing.T) {
	addr, err := btcutil.DecodeAddress(
		"bc1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l",
		&chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	scriptPubKey, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	tests := []struct {
		message          string
		expectedToSignID string
	}{
		{
			message:          "",
			expectedToSignID: "1e9654e951a5ba44c8604c4de6c67fd78a27e81dcadcfe1edf638ba3aaebaed6",
		},
		{
			message:          "Hello World",
			expectedToSignID: "88737ae86f2077145f93cc4b153ae9a1cb8d56afa511988c149c5c8c9d93bddf",
		},
	}

	for _, tc := range tests {
		toSpendTx, err := BuildToSpendTx(tc.message, scriptPubKey)
		require.NoError(t, err)

		toSignTx, err := BuildToSignTx(toSpendTx.TxHash(), scriptPubKey)
		require.NoError(t, err)

		require.Equal(t, int32(0), toSignTx.Version)
		require.Len(t, toSignTx.TxIn, 1)
		require.Equal(t, uint32(0), toSignTx.TxIn[0].PreviousOutPoint.Index)
		require.Equal(t, uint32(0), toSignTx.TxIn[0].Sequence)
		require.Len(t, toSignTx.TxOut, 1)
		require.Equal(t, int64(0), toSignTx.TxOut[0].Value)
		require.Equal(t, uint32(0), toSignTx.LockTime)

		scriptClass := txscript.GetScriptClass(toSignTx.TxOut[0].PkScript)
		require.Equal(t, txscript.NullDataTy, scriptClass)

		require.Equal(t, toSpendTx.TxHash(), toSignTx.TxIn[0].PreviousOutPoint.Hash)
		require.Equal(t, tc.expectedToSignID, toSignTx.TxHash().String())
	}
}
