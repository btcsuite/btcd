package bip322

import (
	"crypto/rand"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

// TestSign tests that a valid signature can be created for all supported
// single-key address types.
func TestSign(t *testing.T) {
	for range 100 {
		randBytes := make([]byte, 256)
		_, err := rand.Read(randBytes)
		require.NoError(t, err)

		randPrivateKey, err := btcec.NewPrivateKey()
		require.NoError(t, err)

		// Sign for a P2TR address.
		sigP2TR, err := SignP2TR(string(randBytes), randPrivateKey)
		require.NoError(t, err)
		require.Contains(t, sigP2TR, PrefixSimple)

		scriptP2TR, err := payToTaprootScript(randPrivateKey)
		require.NoError(t, err)

		valid, _, err := verifyMessageForChallenge(
			randBytes, scriptP2TR, sigP2TR,
		)
		require.NoError(t, err)
		require.True(t, valid)

		// Sign for a P2WPKH address.
		sigP2WPKH, err := SignP2WPKH(string(randBytes), randPrivateKey)
		require.NoError(t, err)
		require.Contains(t, sigP2WPKH, PrefixSimple)

		scriptP2WPKH, err := payToWitnessPubKeyHashScript(
			randPrivateKey,
		)
		require.NoError(t, err)

		valid, _, err = verifyMessageForChallenge(
			randBytes, scriptP2WPKH, sigP2WPKH,
		)
		require.NoError(t, err)
		require.True(t, valid)

		// Sign for a P2PKH address.
		sigP2PKH, err := SignP2PKH(string(randBytes), randPrivateKey)
		require.NoError(t, err)
		require.Contains(t, sigP2PKH, PrefixFull)

		scriptP2PKH, err := payToPubKeyHashScript(randPrivateKey)
		require.NoError(t, err)

		valid, _, err = verifyMessageForChallenge(
			randBytes, scriptP2PKH, sigP2PKH,
		)
		require.NoError(t, err)
		require.True(t, valid)
	}
}
