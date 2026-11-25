package silentpayments

import (
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/stretchr/testify/require"
)

// TestTransactionOutputKeysForFilterBatch tests that the batched candidate
// derivation produces exactly the same keys as the per-tweak variant, for
// random key material and for every official test vector.
func TestTransactionOutputKeysForFilterBatch(t *testing.T) {
	t.Parallel()

	scanPriv, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	spendPriv, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	scanPub := *scanPriv.PubKey()
	spendPub := *spendPriv.PubKey()
	baseAddr := NewAddress(MainNetHRP, scanPub, spendPub, nil)
	changeAddr := NewAddress(
		MainNetHRP, scanPub, spendPub, LabelTweak(scanPriv, 0),
	)
	addresses := []ScanAddress{
		NewScanAddress(*baseAddr, *scanPriv),
		NewScanAddress(*changeAddr, *scanPriv),
	}

	// Random tweak points, as an index server would serve them.
	tweaks := make([]*btcec.PublicKey, 50)
	for i := range tweaks {
		tweakPriv, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		tweaks[i] = tweakPriv.PubKey()
	}

	batched, err := TransactionOutputKeysForFilterBatch(addresses, tweaks)
	require.NoError(t, err)
	require.Len(t, batched, len(tweaks)*len(addresses))

	for i, tweak := range tweaks {
		single, err := TransactionOutputKeysForFilter(
			*tweak, addresses,
		)
		require.NoError(t, err)
		require.Len(t, single, len(addresses))

		for j, key := range single {
			require.Equal(
				t, schnorr.SerializePubKey(key),
				schnorr.SerializePubKey(
					batched[i*len(addresses)+j],
				),
				"tweak %d address %d", i, j,
			)
		}
	}

	// The batch call requires a single scan key across all addresses.
	otherScan, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	mixed := []ScanAddress{
		addresses[0], NewScanAddress(*baseAddr, *otherScan),
	}
	_, err = TransactionOutputKeysForFilterBatch(mixed, tweaks[:1])
	require.ErrorContains(t, err, "same scan private key")

	// No tweaks means no candidates and no error.
	empty, err := TransactionOutputKeysForFilterBatch(addresses, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}

// TestTransactionOutputKeysForFilterBatchVectors tests the batched
// derivation against every official receiving vector that carries a
// transaction tweak, comparing against the per-tweak variant.
func TestTransactionOutputKeysForFilterBatchVectors(t *testing.T) {
	t.Parallel()

	vectors, err := ReadTestVectors()
	require.NoError(t, err)

	for _, vector := range vectors {
		for _, r := range vector.Receiving {
			if r.Expected.Tweak == "" {
				continue
			}

			tweakBytes, err := hex.DecodeString(r.Expected.Tweak)
			require.NoError(t, err)
			tweak, err := btcec.ParsePubKey(tweakBytes)
			require.NoError(t, err)

			scanKey, spendKey, err := r.Given.KeyMaterial.Parse()
			require.NoError(t, err)

			scanPub := *scanKey.PubKey()
			spendPub := *spendKey.PubKey()
			baseAddr := NewAddress(
				MainNetHRP, scanPub, spendPub, nil,
			)
			changeAddr := NewAddress(
				MainNetHRP, scanPub, spendPub,
				LabelTweak(scanKey, 0),
			)
			addresses := []ScanAddress{
				NewScanAddress(*baseAddr, *scanKey),
				NewScanAddress(*changeAddr, *scanKey),
			}

			batched, err := TransactionOutputKeysForFilterBatch(
				addresses, []*btcec.PublicKey{tweak},
			)
			require.NoError(t, err)

			single, err := TransactionOutputKeysForFilter(
				*tweak, addresses,
			)
			require.NoError(t, err)
			require.Len(t, batched, len(single))

			for i, key := range single {
				require.Equal(
					t, schnorr.SerializePubKey(key),
					schnorr.SerializePubKey(batched[i]),
				)
			}
		}
	}
}

// TestTransactionOutputKeysForFilterBatchInfinity tests that a candidate
// output key at the point at infinity (spend key equal to the negated
// tweak point) yields an error, matching the per-tweak variant.
func TestTransactionOutputKeysForFilterBatchInfinity(t *testing.T) {
	t.Parallel()

	// With a scan key of one, the shared secret equals the tweak point,
	// which makes the output tweak t_0 predictable so the adversarial
	// spend key can be constructed.
	var oneScalar btcec.ModNScalar
	oneScalar.SetInt(1)
	scanPriv := btcec.PrivKeyFromScalar(&oneScalar)

	tweakPriv, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	tweak := tweakPriv.PubKey()

	tweakScalar := testTweakScalar(t, tweak, 0)
	spendKey := Negate(ScalarBaseMult(*tweakScalar))

	scanPub := *scanPriv.PubKey()
	badAddr := NewAddress(MainNetHRP, scanPub, *spendKey, nil)
	addresses := []ScanAddress{NewScanAddress(*badAddr, *scanPriv)}

	_, err = TransactionOutputKeysForFilterBatch(
		addresses, []*btcec.PublicKey{tweak},
	)
	require.ErrorContains(t, err, "infinity")
}
