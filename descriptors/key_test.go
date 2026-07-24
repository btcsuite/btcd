package descriptors

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// basicTestXpub is a valid extended public key reused across the descriptor
// unit tests in this package.
const basicTestXpub = "xpub6BzikmgQmvoYG3ShFhXU1LFKaUeU832dHoYL6ka9JpC" +
	"qKXr7PTHQHaoSMbGU36CZNcoryVPsFBjt9aYyCQHtYi6BQTo6V" +
	"fRv9xVRuSNNteB"

// TestParseDescKey checks that individual key expressions parse into the right
// structure: raw keys keep their bytes and reject a path, extended keys keep
// their derivation steps, key origins are stripped, and multipath and wildcard
// steps are recognized.
func TestParseDescKey(t *testing.T) {
	t.Parallel()

	t.Run("compressed raw key", func(t *testing.T) {
		t.Parallel()

		k, err := parseDescKey("02" + strings.Repeat("ab", 32))
		require.NoError(t, err)
		require.Nil(t, k.xpub)
		require.Len(t, k.rawKey, 33)
		require.Empty(t, k.steps)
	})

	t.Run("x-only raw key", func(t *testing.T) {
		t.Parallel()

		k, err := parseDescKey(strings.Repeat("ab", 32))
		require.NoError(t, err)
		require.Len(t, k.rawKey, 32)
	})

	t.Run("raw key rejects a path", func(t *testing.T) {
		t.Parallel()

		_, err := parseDescKey(strings.Repeat("ab", 32) + "/0")
		require.Error(t, err)
	})

	t.Run("xpub with fixed path", func(t *testing.T) {
		t.Parallel()

		k, err := parseDescKey(basicTestXpub + "/0/5")
		require.NoError(t, err)
		require.NotNil(t, k.xpub)
		require.Nil(t, k.rawKey)
		require.Len(t, k.steps, 2)
		require.Equal(t, stepNum, k.steps[0].kind)
		require.Equal(t, uint32(0), k.steps[0].num)
		require.Equal(t, uint32(5), k.steps[1].num)
		require.False(t, k.isWildcard())
		require.Equal(t, 1, k.multipathLen())
	})

	t.Run("origin, multipath and wildcard", func(t *testing.T) {
		t.Parallel()

		k, err := parseDescKey(
			"[e81a5744/48'/0'/0'/2']" + basicTestXpub + "/<0;1>/*",
		)
		require.NoError(t, err)
		require.NotNil(t, k.xpub)
		require.Len(t, k.steps, 2)
		require.Equal(t, stepMultipath, k.steps[0].kind)
		require.Equal(t, []uint32{0, 1}, k.steps[0].multipath)
		require.Equal(t, stepWildcard, k.steps[1].kind)
		require.True(t, k.isWildcard())
		require.Equal(t, 2, k.multipathLen())
	})

	t.Run("invalid key rejected", func(t *testing.T) {
		t.Parallel()

		_, err := parseDescKey("abcd")
		require.Error(t, err)
	})
}

// TestIsPubKeyLen checks the public-key length classification: 32 (x-only), 33
// (compressed) and 65 (uncompressed) are valid, everything else is not.
func TestIsPubKeyLen(t *testing.T) {
	t.Parallel()

	for _, n := range []int{32, 33, 65} {
		require.Truef(t, isPubKeyLen(n), "length %d", n)
	}
	for _, n := range []int{0, 20, 31, 34, 64, 66} {
		require.Falsef(t, isPubKeyLen(n), "length %d", n)
	}
}

// TestDescKeyDerive checks non-hardened derivation from an extended key: the
// x-only form is the compressed key without its parity byte, distinct
// derivation indices give distinct keys, and the multipath element selects a
// different child per multipath index.
func TestDescKeyDerive(t *testing.T) {
	t.Parallel()

	k, err := parseDescKey(basicTestXpub + "/*")
	require.NoError(t, err)

	compressed, err := k.derive(0, 0, false)
	require.NoError(t, err)
	require.Len(t, compressed, 33)

	xOnly, err := k.derive(0, 0, true)
	require.NoError(t, err)
	require.Len(t, xOnly, 32)
	require.Equal(t, compressed[1:], xOnly)

	other, err := k.derive(0, 1, false)
	require.NoError(t, err)
	require.NotEqual(t, compressed, other)

	// The multipath element selects a different child per multipath index.
	mk, err := parseDescKey(basicTestXpub + "/<0;1>/*")
	require.NoError(t, err)

	mp0, err := mk.derive(0, 0, false)
	require.NoError(t, err)
	mp1, err := mk.derive(1, 0, false)
	require.NoError(t, err)
	require.NotEqual(t, mp0, mp1)
}
