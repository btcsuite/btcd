package bip322

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"
)

const (
	testPrivKeyWIF = "L3VFeEujGtevx9w18HD1fhRbCH67Az2dpCymeRE1SoPK6XQtaN2k"
	testP2WPKHAddr = "bc1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l"
)

func decodeTestKey(t *testing.T) (*btcutil.WIF, btcutil.Address) {
	t.Helper()
	wif, err := btcutil.DecodeWIF(testPrivKeyWIF)
	require.NoError(t, err)
	addr, err := btcutil.DecodeAddress(testP2WPKHAddr, &chaincfg.MainNetParams)
	require.NoError(t, err)
	return wif, addr
}

func TestBIP322P2WPKHRoundTrip(t *testing.T) {
	wif, addr := decodeTestKey(t)

	for _, msg := range []string{"Hello World", ""} {
		sig, err := SignP2WPKH(wif.PrivKey, addr, msg)
		require.NoError(t, err)
		require.NotEmpty(t, sig)

		valid, err := VerifyP2WPKH(addr, msg, sig)
		require.NoError(t, err)
		require.True(t, valid, "expected valid signature for message %q", msg)
	}
}

func TestBIP322P2WPKHBIPVectors(t *testing.T) {
	addr, err := btcutil.DecodeAddress(testP2WPKHAddr, &chaincfg.MainNetParams)
	require.NoError(t, err)

	vectors := []struct {
		message   string
		signature string
	}{
		{
			message:   "",
			signature: "AkcwRAIgM2gBAQqvZX15ZiysmKmQpDrG83avLIT492QBzLnQIxYCIBaTpOaD20qRlEylyxFSeEA2ba9YOixpX8z46TSDtS40ASECx/EgAxlkQpQ9hYjgGu6EBCPMVPwVIVJqO4XCsMvViHI=",
		},
		{
			message: "",
			signature: "AkgwRQIhAPkJ1Q4oYS0htvyuSFHLxRQpFAY56b70UvE7Dxazen0ZAiAtZfFz1S6T6I23MWI2lK" +
				"/pcNTWncuyL8UL+oMdydVgzAEhAsfxIAMZZEKUPYWI4BruhAQjzFT8FSFSajuFwrDL1Yhy",
		},
		{
			message:   "Hello World",
			signature: "AkcwRAIgZRfIY3p7/DoVTty6YZbWS71bc5Vct9p9Fia83eRmw2QCICK/ENGfwLtptFluMGs2KsqoNSk89pO7F29zJLUx9a/sASECx/EgAxlkQpQ9hYjgGu6EBCPMVPwVIVJqO4XCsMvViHI=",
		},
		{
			message: "Hello World",
			signature: "AkgwRQIhAOzyynlqt93lOKJr+wmmxIens//zPzl9tqIOua93wO6MAiBi5n5EyAcPScOjf1lAqIUI" +
				"Qtr3zKNeavYabHyR8eGhowEhAsfxIAMZZEKUPYWI4BruhAQjzFT8FSFSajuFwrDL1Yhy",
		},
	}

	for _, v := range vectors {
		valid, err := VerifyP2WPKH(addr, v.message, v.signature)
		require.NoError(t, err)
		require.True(t, valid, "BIP spec vector failed for message %q", v.message)
	}
}
