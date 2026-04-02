package bip322

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/stretchr/testify/require"
)

func testP2SHP2WPKHAddress(t *testing.T) (*btcutil.WIF, btcutil.Address) {
	t.Helper()
	wif, err := btcutil.DecodeWIF(testPrivKeyWIF)
	require.NoError(t, err)

	pubKeyBytes := wif.PrivKey.PubKey().SerializeCompressed()
	redeemScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(btcutil.Hash160(pubKeyBytes)).
		Script()
	require.NoError(t, err)

	addr, err := btcutil.NewAddressScriptHash(redeemScript, &chaincfg.MainNetParams)
	require.NoError(t, err)

	return wif, addr
}

func TestBIP322P2SHP2WPKHRoundTrip(t *testing.T) {
	wif, addr := testP2SHP2WPKHAddress(t)

	for _, msg := range []string{"Hello World", ""} {
		sig, err := SignP2SHP2WPKH(wif.PrivKey, addr, msg)
		require.NoError(t, err)
		require.NotEmpty(t, sig)

		valid, err := VerifyP2SHP2WPKH(addr, msg, sig)
		require.NoError(t, err)
		require.True(t, valid, "expected valid signature for message %q", msg)
	}
}
