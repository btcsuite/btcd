package descriptors

import (
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/stretchr/testify/require"
)

const (
	// basicTestXpub is a valid extended public key reused across the
	// descriptor unit tests in this package.
	basicTestXpub = "xpub6BzikmgQmvoYG3ShFhXU1LFKaUeU832dHoYL6ka9JpCqKXr7" +
		"PTHQHaoSMbGU36CZNcoryVPsFBjt9aYyCQHtYi6BQTo6VfRv9xVRuSNNteB"
)

// TestParseDescKey checks that individual key expressions parse into the right
// structure: raw keys keep their bytes and reject a path, extended keys keep
// their derivation steps, key origins are stripped, and multipath and wildcard
// steps are recognized.
func TestParseDescKey(t *testing.T) {
	t.Parallel()

	t.Run("compressed raw key", func(t *testing.T) {
		t.Parallel()

		k, err := parseDescKey(
			"02"+strings.Repeat("ab", 32), keyFormLegacy,
		)
		require.NoError(t, err)
		require.Nil(t, k.xpub)
		require.Len(t, k.rawKey, 33)
		require.Empty(t, k.steps)
	})

	t.Run("uncompressed raw key", func(t *testing.T) {
		t.Parallel()

		k, err := parseDescKey(
			"04"+strings.Repeat("ab", 64), keyFormLegacy,
		)
		require.NoError(t, err)
		require.Len(t, k.rawKey, 65)
	})

	t.Run("x-only raw key", func(t *testing.T) {
		t.Parallel()

		k, err := parseDescKey(strings.Repeat("ab", 32), keyFormXOnly)
		require.NoError(t, err)
		require.Len(t, k.rawKey, 32)
	})

	t.Run("raw key rejects a path", func(t *testing.T) {
		t.Parallel()

		_, err := parseDescKey(
			strings.Repeat("ab", 32)+"/0", keyFormXOnly,
		)
		require.Error(t, err)
	})

	t.Run("xpub with fixed path", func(t *testing.T) {
		t.Parallel()

		k, err := parseDescKey(basicTestXpub+"/0/5", keyFormCompressed)
		require.NoError(t, err)
		require.NotNil(t, k.xpub)
		require.Nil(t, k.rawKey)
		require.Len(t, k.steps, 2)
		require.Equal(t, stepNum, k.steps[0].kind)
		require.Equal(t, pathIndex{num: 0}, k.steps[0].index)
		require.Equal(t, pathIndex{num: 5}, k.steps[1].index)
		require.False(t, k.isWildcard())
		require.Equal(t, 1, k.multipathLen())
	})

	t.Run("origin, multipath and wildcard", func(t *testing.T) {
		t.Parallel()

		k, err := parseDescKey(
			"[e81a5744/48'/0'/0'/2']"+basicTestXpub+"/<0;1>/*",
			keyFormCompressed,
		)
		require.NoError(t, err)
		require.NotNil(t, k.xpub)
		require.Len(t, k.steps, 2)
		require.Equal(t, stepMultipath, k.steps[0].kind)
		require.Equal(
			t, []pathIndex{{num: 0}, {num: 1}},
			k.steps[0].multipath,
		)
		require.Equal(t, stepWildcard, k.steps[1].kind)
		require.True(t, k.isWildcard())
		require.Equal(t, 2, k.multipathLen())
	})

	t.Run("invalid key rejected", func(t *testing.T) {
		t.Parallel()

		_, err := parseDescKey("abcd", keyFormCompressed)
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

	k, err := parseDescKey(basicTestXpub+"/*", keyFormCompressed)
	require.NoError(t, err)

	compressed, err := k.derive(0, 0)
	require.NoError(t, err)
	require.Len(t, compressed, 33)

	// The same key in a taproot position is the compressed key without its
	// parity byte.
	trKey, err := parseDescKey(basicTestXpub+"/*", keyFormXOnly)
	require.NoError(t, err)

	xOnly, err := trKey.derive(0, 0)
	require.NoError(t, err)
	require.Len(t, xOnly, 32)
	require.Equal(t, compressed[1:], xOnly)

	// A key derived from an extended key is compressed even in a legacy
	// position, which BIP380 requires.
	legacyKey, err := parseDescKey(basicTestXpub+"/*", keyFormLegacy)
	require.NoError(t, err)

	derived, err := legacyKey.derive(0, 0)
	require.NoError(t, err)
	require.Equal(t, compressed, derived)
	require.Equal(t, 34, legacyKey.pushLen())

	other, err := k.derive(0, 1)
	require.NoError(t, err)
	require.NotEqual(t, compressed, other)

	// The multipath element selects a different child per multipath index.
	mk, err := parseDescKey(basicTestXpub+"/<0;1>/*", keyFormCompressed)
	require.NoError(t, err)

	mp0, err := mk.derive(0, 0)
	require.NoError(t, err)
	mp1, err := mk.derive(1, 0)
	require.NoError(t, err)
	require.NotEqual(t, mp0, mp1)
}

// TestKeyOriginValidation checks that a key origin is validated, using the test
// vectors of BIP380. The content between the brackets used to be skipped
// entirely, so descriptors that every other implementation rejects were
// accepted and echoed back by String().
func TestKeyOriginValidation(t *testing.T) {
	t.Parallel()

	const key = "0260b2003c386519fc9eadf2b5cf124dd8eea4c4e68d5e154050a934" +
		"6ea98ce600"

	tests := []struct {
		name   string
		origin string
		valid  bool
	}{{
		name:   "fingerprint only",
		origin: "[deadbeef]",
		valid:  true,
	}, {
		name:   "hardened with h",
		origin: "[deadbeef/0h/0h/0h]",
		valid:  true,
	}, {
		name:   "hardened with apostrophe",
		origin: "[deadbeef/0'/0'/0']",
		valid:  true,
	}, {
		name:   "mixed hardened indicators",
		origin: "[deadbeef/0'/0h/0']",
		valid:  true,
	}, {
		name:   "unhardened steps",
		origin: "[deadbeef/1/2/3]",
		valid:  true,
	}, {
		name:   "children indicator",
		origin: "[deadbeef/0h/0h/0h/*]",
	}, {
		name:   "trailing slash",
		origin: "[deadbeef/0h/0h/0h/]",
	}, {
		name:   "too short fingerprint",
		origin: "[deadbef/0h/0h/0h]",
	}, {
		name:   "too long fingerprint",
		origin: "[deadbeeef/0h/0h/0h]",
	}, {
		name:   "non hex fingerprint",
		origin: "[gaaaaaaa]",
	}, {
		name:   "invalid hardened indicator f",
		origin: "[deadbeef/0f/0f/0f]",
	}, {
		name:   "negative index",
		origin: "[deadbeef/-0/-0/-0]",
	}, {
		name:   "uppercase hardened indicator",
		origin: "[deadbeef/0H/0H/0H]",
	}, {
		name:   "multipath element",
		origin: "[deadbeef/<0;1>]",
	}, {
		name:   "index out of range",
		origin: "[deadbeef/2147483648]",
	}, {
		name:   "unterminated",
		origin: "[deadbeef/0h",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseDescKey(tc.origin+key, keyFormLegacy)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestHardenedDerivation checks the handling of hardened derivation steps,
// which BIP380 and BIP389 allow anywhere in a path, including as the wildcard
// and inside a multipath element. Deriving one needs the private extended key:
// the descriptor is valid either way, so the distinction belongs at derivation
// time rather than at parse time, where a hardened wildcard used to be rejected
// outright.
func TestHardenedDerivation(t *testing.T) {
	t.Parallel()

	const (
		xpub = "xpub6ERApfZwUNrhLCkDtcHTcxd75RbzS1ed54G1LkBUHQVHQKqhM" +
			"khgbmJbZRkrgZw4koxb5JaHWkY4ALHY2grBGRjaDMzQLcgJvLJuZ" +
			"ZvRcEL"
		xprv = "xprvA1RpRA33e1JQ7ifknakTFpgNXPmW2YvmhqLQYMmrj4xJXXWYp" +
			"DPS3xz7iAxn8L39njGVyuoseXzU6rcxFLJ8HFsTjSyQbLYnMpCqE" +
			"2VbFWc"
	)

	// A hardened path is derivable from a private extended key.
	priv, err := parseDescKey(xprv+"/3h/4'/5h/*h", keyFormCompressed)
	require.NoError(t, err)

	derived, err := priv.derive(0, 7)
	require.NoError(t, err)
	require.Len(t, derived, 33)

	// The definite key string keeps the hardened indicators, with the
	// wildcard resolved to the derivation index.
	require.Equal(t, xprv+"/3'/4'/5'/7'", priv.definiteString(0, 7))

	// The same expression is valid with a public extended key, but cannot
	// be derived from it.
	pub, err := parseDescKey(xpub+"/3h/4h/5h/*h", keyFormCompressed)
	require.NoError(t, err)

	_, err = pub.derive(0, 7)
	require.ErrorContains(t, err, "cannot derive the hardened child")

	// An unhardened wildcard on the same key works.
	unhardened, err := parseDescKey(xpub+"/*", keyFormCompressed)
	require.NoError(t, err)

	_, err = unhardened.derive(0, 7)
	require.NoError(t, err)

	// A derivation index in the hardened range must not silently turn an
	// unhardened wildcard into a hardened derivation.
	_, err = unhardened.derive(0, hdkeychain.HardenedKeyStart)
	require.ErrorContains(t, err, "in the hardened range")

	// A multipath element may hold hardened indices (BIP389), which select
	// hardened children of the private key.
	multi, err := parseDescKey(xprv+"/<2147483647h;0>/0", keyFormCompressed)
	require.NoError(t, err)
	require.Equal(t, 2, multi.multipathLen())

	hardenedPath, err := multi.derive(0, 0)
	require.NoError(t, err)
	unhardenedPath, err := multi.derive(1, 0)
	require.NoError(t, err)
	require.NotEqual(t, hardenedPath, unhardenedPath)

	require.Equal(t, xprv+"/2147483647'/0", multi.definiteString(0, 0))
	require.Equal(t, xprv+"/0/0", multi.definiteString(1, 0))

	// A multipath element must not repeat an index (BIP389).
	_, err = parseDescKey(xpub+"/<0;0>/*", keyFormCompressed)
	require.ErrorContains(t, err, "more than once")

	_, err = parseDescKey(xpub+"/<0;1;0>/*", keyFormCompressed)
	require.ErrorContains(t, err, "more than once")

	// The two hardened indicators spell the same child index, so they
	// collide as well.
	_, err = parseDescKey(xprv+"/<1h;1'>/*", keyFormCompressed)
	require.ErrorContains(t, err, "more than once")
}
