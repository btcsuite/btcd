package bip322

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"
)

func makeP2PKHAddr(t *testing.T) (*btcutil.WIF, btcutil.Address) {
	t.Helper()

	wif, err := btcutil.DecodeWIF(
		"L3VFeEujGtevx9w18HD1fhRbCH67Az2dpCymeRE1SoPK6XQtaN2k",
	)
	require.NoError(t, err)

	pubKey := wif.PrivKey.PubKey()
	pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())
	addr, err := btcutil.NewAddressPubKeyHash(
		pubKeyHash, &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	return wif, addr
}

func TestBIP322P2PKHRoundTrip(t *testing.T) {
	wif, addr := makeP2PKHAddr(t)

	for _, msg := range []string{"Hello World", ""} {
		sig, err := SignP2PKH(wif.PrivKey, addr, msg)
		require.NoError(t, err)
		require.NotEmpty(t, sig)

		format, err := DetectFormat(sig)
		require.NoError(t, err)
		require.Equal(t, FormatFull, format,
			"P2PKH signature must be Full format")

		valid, err := VerifyP2PKH(addr, msg, sig)
		require.NoError(t, err)
		require.True(t, valid,
			"expected valid signature for message %q", msg)
	}
}
