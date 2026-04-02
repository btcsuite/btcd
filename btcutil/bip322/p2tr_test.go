package bip322

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/stretchr/testify/require"
)

func deriveTaprootAddr(t *testing.T, wifStr string) (*btcutil.AddressTaproot, *btcutil.WIF) {
	t.Helper()
	wif, err := btcutil.DecodeWIF(wifStr)
	require.NoError(t, err)
	taprootKey := txscript.ComputeTaprootKeyNoScript(wif.PrivKey.PubKey())
	addr, err := btcutil.NewAddressTaproot(schnorr.SerializePubKey(taprootKey), &chaincfg.MainNetParams)
	require.NoError(t, err)
	return addr, wif
}

func TestBIP322P2TRRoundTrip(t *testing.T) {
	addr, wif := deriveTaprootAddr(t, testPrivKeyWIF)

	for _, msg := range []string{"Hello World", ""} {
		sig, err := SignP2TR(wif.PrivKey, addr, msg)
		require.NoError(t, err)

		valid, err := VerifyP2TR(addr, msg, sig)
		require.NoError(t, err)
		require.True(t, valid)
	}
}
