package silentpayments

import (
	"encoding/binary"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/stretchr/testify/require"
)

// testTweakScalar computes t_k for a shared secret the same way outputKey
// does, so tests can construct adversarial spend keys relative to it.
func testTweakScalar(t *testing.T, sharedSecret *btcec.PublicKey,
	k uint32) *btcec.ModNScalar {

	payload := make([]byte, 33+4)
	copy(payload, sharedSecret.SerializeCompressed())
	binary.BigEndian.PutUint32(payload[33:], k)
	tweakHash := chainhash.TaggedHash(TagBIP0352SharedSecret, payload)

	var tweak btcec.ModNScalar
	overflow := tweak.SetBytes((*[32]byte)(tweakHash))
	require.Zero(t, overflow)

	return &tweak
}

// TestCreateOutputKeyInfinity tests that a spend key equal to the negated
// tweak point, which pushes P = B + t_k*G to the point at infinity, yields
// an error instead of the bogus (0, 0) pseudo key. Honest key derivation
// cannot produce such a spend key, but the function must not return an
// invalid point for any input (the secp256k1 silentpayments module fails
// this case cleanly as well).
func TestCreateOutputKeyInfinity(t *testing.T) {
	sharedPriv, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	sharedSecret := sharedPriv.PubKey()

	var one btcec.ModNScalar
	one.SetInt(1)

	tweak := testTweakScalar(t, sharedSecret, 0)
	spendKey := Negate(ScalarBaseMult(*tweak))

	_, err = CreateOutputKey(*sharedSecret, *spendKey, 0, one)
	require.ErrorContains(t, err, "point at infinity")

	// An honest spend key must still work.
	honestPriv, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	outputKey, err := CreateOutputKey(
		*sharedSecret, *honestPriv.PubKey(), 0, one,
	)
	require.NoError(t, err)
	require.True(t, outputKey.IsOnCurve())
}

// TestCreateOutputKeyZeroLabelTweak pins the behavior for a label tweak
// that is the exact negation of t_k: the labeled spend key is B - t_k*G,
// so the derived output key is B itself and the combined private key tweak
// is zero. The output is still a valid, findable (and with the plain spend
// key, spendable) key — matching the secp256k1 silentpayments module,
// which reports such outputs with a zero tweak instead of failing.
func TestCreateOutputKeyZeroLabelTweak(t *testing.T) {
	sharedPriv, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	sharedSecret := sharedPriv.PubKey()

	spendPriv, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	spendKey := spendPriv.PubKey()

	var one btcec.ModNScalar
	one.SetInt(1)

	labelTweak := testTweakScalar(t, sharedSecret, 0)
	labelTweak.Negate()

	labeledSpend := LabelSpendKey(labelTweak, spendKey)
	outputKey, err := CreateOutputKey(*sharedSecret, *labeledSpend, 0, one)
	require.NoError(t, err)

	// P = (B - t_0*G) + t_0*G = B: found under the plain spend key.
	require.Equal(
		t, schnorr.SerializePubKey(spendKey),
		schnorr.SerializePubKey(outputKey),
	)

	// The combined private key tweak t_0 + label is zero.
	combined := *testTweakScalar(t, sharedSecret, 0)
	combined.Add(labelTweak)
	require.True(t, combined.IsZero())
}
