package bip322

import (
	mrand "math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

// randomUTF8String returns a string of n randomly-chosen valid UTF-8 runes.
func randomUTF8String(n int) string {
	runes := make([]rune, n)
	for i := range runes {
		runes[i] = randomRune()
	}
	return string(runes)
}

// randomRune returns a random valid Unicode scalar value. It rejects the
// surrogate range (U+D800–U+DFFF) and anything above U+10FFFF, both of which
// cannot be legally encoded as UTF-8.
func randomRune() rune {
	for {
		// Random value in [0, 0x10FFFF].
		r := rune(mrand.Int64N(utf8.MaxRune + 1))
		if utf8.ValidRune(r) {
			return r
		}
	}
}

// TestSign tests that a valid signature can be created for all supported
// single-key address types.
func TestSign(t *testing.T) {
	for range 100 {
		randString := randomUTF8String(256)

		randPrivateKey, err := btcec.NewPrivateKey()
		require.NoError(t, err)

		// Sign for a P2TR address.
		sigP2TR, err := SignP2TR(randString, randPrivateKey)
		require.NoError(t, err)
		require.Contains(t, sigP2TR, PrefixSimple)
		require.True(t, strings.HasPrefix(sigP2TR, PrefixSimple))

		scriptP2TR, err := payToTaprootScript(randPrivateKey)
		require.NoError(t, err)

		valid, _, err := verifyMessageForChallenge(
			[]byte(randString), scriptP2TR, sigP2TR,
		)
		require.NoError(t, err)
		require.True(t, valid)

		// Sign for a P2WPKH address.
		sigP2WPKH, err := SignP2WPKH(randString, randPrivateKey)
		require.NoError(t, err)
		require.Contains(t, sigP2WPKH, PrefixSimple)
		require.True(t, strings.HasPrefix(sigP2WPKH, PrefixSimple))

		scriptP2WPKH, err := payToWitnessPubKeyHashScript(
			randPrivateKey,
		)
		require.NoError(t, err)

		valid, _, err = verifyMessageForChallenge(
			[]byte(randString), scriptP2WPKH, sigP2WPKH,
		)
		require.NoError(t, err)
		require.True(t, valid)

		// Sign for a NP2WPKH address.
		sigNP2WPKH, err := SignNestedP2WPKH(randString, randPrivateKey)
		require.NoError(t, err)
		require.Contains(t, sigNP2WPKH, PrefixFull)
		require.True(t, strings.HasPrefix(sigNP2WPKH, PrefixFull))

		scriptNP2WPKH, _, err := payToNestedWitnessPubKeyHashScript(
			randPrivateKey,
		)
		require.NoError(t, err)

		valid, _, err = verifyMessageForChallenge(
			[]byte(randString), scriptNP2WPKH, sigNP2WPKH,
		)
		require.NoError(t, err)
		require.True(t, valid)

		// Sign for a P2PKH address.
		sigP2PKH, err := SignP2PKH(randString, randPrivateKey)
		require.NoError(t, err)
		require.Contains(t, sigP2PKH, PrefixFull)
		require.True(t, strings.HasPrefix(sigP2PKH, PrefixFull))

		scriptP2PKH, err := payToPubKeyHashScript(randPrivateKey)
		require.NoError(t, err)

		valid, _, err = verifyMessageForChallenge(
			[]byte(randString), scriptP2PKH, sigP2PKH,
		)
		require.NoError(t, err)
		require.True(t, valid)
	}
}
