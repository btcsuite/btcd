// Copyright 2013-2026 The btcsuite developers

package musig2

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

// nonCanonicalInfinity returns a 33-byte encoding with a 0x00 prefix followed
// by non-zero bytes. BIP 327 rejects it both as a public nonce (cpoint) and as
// an aggregate nonce (cpoint_ext only accepts 33 zero bytes for infinity).
func nonCanonicalInfinity() []byte {
	b := make([]byte, btcec.PubKeyBytesLenCompressed)
	for i := 1; i < len(b); i++ {
		b[i] = 0xff
	}
	return b
}

// setHalf returns a copy of nonce with its first or second 33-byte half
// replaced by half.
func setHalf(nonce [PubNonceSize]byte, second bool,
	half []byte) [PubNonceSize]byte {

	offset := 0
	if second {
		offset = btcec.PubKeyBytesLenCompressed
	}
	copy(nonce[offset:offset+btcec.PubKeyBytesLenCompressed], half)
	return nonce
}

type nonceTestSigner struct {
	priv   *btcec.PrivateKey
	nonces *Nonces
}

func newNonceTestSigners(t *testing.T, n int) ([]nonceTestSigner,
	[]*btcec.PublicKey) {

	signers := make([]nonceTestSigner, n)
	keys := make([]*btcec.PublicKey, n)
	for i := range signers {
		priv, err := btcec.NewPrivateKey()
		require.NoError(t, err)

		nonces, err := GenNonces(WithPublicKey(priv.PubKey()))
		require.NoError(t, err)

		signers[i] = nonceTestSigner{priv: priv, nonces: nonces}
		keys[i] = priv.PubKey()
	}
	return signers, keys
}

// TestAggregateNoncesRejectsInfinityPubNonce makes sure a public nonce that
// encodes the point at infinity, canonically or not, is rejected instead of
// being silently treated as infinity.
func TestAggregateNoncesRejectsInfinityPubNonce(t *testing.T) {
	t.Parallel()

	signers, _ := newNonceTestSigners(t, 2)
	honest := signers[0].nonces.PubNonce

	zeros := make([]byte, btcec.PubKeyBytesLenCompressed)
	for _, second := range []bool{false, true} {
		for _, half := range [][]byte{nonCanonicalInfinity(), zeros} {
			bad := setHalf(signers[1].nonces.PubNonce, second, half)

			_, err := AggregateNonces(
				[][PubNonceSize]byte{honest, bad},
			)
			require.Error(t, err, "second=%v half=%x", second,
				half)
		}
	}
}

// TestSignVerifyRejectsNonCanonicalAggNonce makes sure that Sign and Verify
// only accept the 33 zero byte encoding of infinity in the aggregate nonce,
// and that Verify rejects a public nonce that encodes infinity.
func TestSignVerifyRejectsNonCanonicalAggNonce(t *testing.T) {
	t.Parallel()

	signers, keys := newNonceTestSigners(t, 2)
	combined, err := AggregateNonces([][PubNonceSize]byte{
		signers[0].nonces.PubNonce, signers[1].nonces.PubNonce,
	})
	require.NoError(t, err)

	var msg [32]byte
	copy(msg[:], "musig2 aggregate nonce encoding")

	s := signers[0]
	sig, err := Sign(s.nonces.SecNonce, s.priv, combined, keys, msg)
	require.NoError(t, err)
	require.True(t, sig.Verify(
		s.nonces.PubNonce, combined, keys, s.priv.PubKey(), msg,
	))

	for _, second := range []bool{false, true} {
		bad := setHalf(combined, second, nonCanonicalInfinity())

		_, err := Sign(s.nonces.SecNonce, s.priv, bad, keys, msg)
		require.Error(t, err, "second=%v", second)

		require.False(t, sig.Verify(
			s.nonces.PubNonce, bad, keys, s.priv.PubKey(), msg,
		), "second=%v", second)

		badPubNonce := setHalf(
			s.nonces.PubNonce, second, nonCanonicalInfinity(),
		)
		require.False(t, sig.Verify(
			badPubNonce, combined, keys, s.priv.PubKey(), msg,
		), "second=%v", second)
	}

	// The canonical encoding of infinity (33 zero bytes) is still a valid
	// aggregate nonce half, as in BIP 327's cpoint_ext.
	zeros := make([]byte, btcec.PubKeyBytesLenCompressed)
	canonical := setHalf(combined, false, zeros)
	_, err = Sign(s.nonces.SecNonce, s.priv, canonical, keys, msg)
	require.NoError(t, err)
}
