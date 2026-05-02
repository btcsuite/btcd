package bip322

import (
	"crypto/sha256"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"
)

func TestBIP322UnifiedSignDispatch(t *testing.T) {
	wif, p2wpkhAddr := decodeTestKey(t)

	p2wpkhSig, err := Sign(wif.PrivKey, p2wpkhAddr, "Hello World")
	require.NoError(t, err)
	format, err := DetectFormat(p2wpkhSig)
	require.NoError(t, err)
	require.Equal(t, FormatSimple, format)

	p2trAddr, _ := deriveTaprootAddr(t, testPrivKeyWIF)
	p2trSig, err := Sign(wif.PrivKey, p2trAddr, "Hello World")
	require.NoError(t, err)
	format, err = DetectFormat(p2trSig)
	require.NoError(t, err)
	require.Equal(t, FormatSimple, format)

	_, p2pkhAddr := makeP2PKHAddr(t)
	p2pkhSig, err := Sign(wif.PrivKey, p2pkhAddr, "Hello World")
	require.NoError(t, err)
	format, err = DetectFormat(p2pkhSig)
	require.NoError(t, err)
	require.Equal(t, FormatFull, format)

	pubKeyBytes := wif.PrivKey.PubKey().SerializeCompressed()
	redeemScriptHash := btcutil.Hash160(pubKeyBytes)
	p2wpkhScript := append([]byte{0x00, 0x14}, redeemScriptHash...)
	scriptHash := sha256.Sum256(p2wpkhScript)
	p2wshAddr, err := btcutil.NewAddressWitnessScriptHash(
		scriptHash[:], &chaincfg.MainNetParams,
	)
	require.NoError(t, err)
	_, err = Sign(wif.PrivKey, p2wshAddr, "Hello World")
	require.ErrorIs(t, err, ErrUnsupportedAddressType)
}

func TestBIP322UnifiedVerifyRoundTrip(t *testing.T) {
	wif, p2wpkhAddr := decodeTestKey(t)

	p2wpkhSig, err := Sign(wif.PrivKey, p2wpkhAddr, "Hello World")
	require.NoError(t, err)
	valid, err := Verify(p2wpkhAddr, "Hello World", p2wpkhSig)
	require.NoError(t, err)
	require.True(t, valid)

	p2trAddr, _ := deriveTaprootAddr(t, testPrivKeyWIF)
	p2trSig, err := Sign(wif.PrivKey, p2trAddr, "Hello World")
	require.NoError(t, err)
	valid, err = Verify(p2trAddr, "Hello World", p2trSig)
	require.NoError(t, err)
	require.True(t, valid)

	_, p2pkhAddr := makeP2PKHAddr(t)
	p2pkhSig, err := Sign(wif.PrivKey, p2pkhAddr, "Hello World")
	require.NoError(t, err)
	valid, err = Verify(p2pkhAddr, "Hello World", p2pkhSig)
	require.NoError(t, err)
	require.True(t, valid)

	_, p2shAddr := testP2SHP2WPKHAddress(t)
	p2shSig, err := Sign(wif.PrivKey, p2shAddr, "Hello World")
	require.NoError(t, err)
	valid, err = Verify(p2shAddr, "Hello World", p2shSig)
	require.NoError(t, err)
	require.True(t, valid)
}
