package silentpayments

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAddresses tests the generation of silent payment addresses.
func TestAddresses(t *testing.T) {
	vectors, err := ReadTestVectors()
	require.NoError(t, err)

	for _, vector := range vectors {
		vector := vector
		t.Run(vector.Comment, func(tt *testing.T) {
			runAddressesTest(tt, vector)
		})
	}
}

// runAddressesTest tests the generation and parsing of silent payment
// addresses.
func runAddressesTest(t *testing.T, vector *TestVector) {
	// We first check that we can create the receiving address correctly.
	for _, receiving := range vector.Receiving {
		mat := receiving.Given.KeyMaterial

		scanPrivKey, spendPrivKey, err := mat.Parse()
		require.NoError(t, err)

		scanPubKey := scanPrivKey.PubKey()
		spendPubKey := spendPrivKey.PubKey()

		addrNoLabel := NewAddress(
			MainNetHRP, *scanPubKey, *spendPubKey, nil,
		)

		require.Equal(
			t, receiving.Expected.Addresses[0],
			addrNoLabel.EncodeAddress(),
		)

		for idx, label := range receiving.Given.Labels {
			tweak := LabelTweak(scanPrivKey, label)

			addrWithLabel := NewAddress(
				MainNetHRP, *scanPubKey, *spendPubKey, tweak,
			)

			require.Equalf(
				t, receiving.Expected.Addresses[idx+1],
				addrWithLabel.EncodeAddress(),
				"with label %d", label,
			)
		}
	}

	// We then also check that we can successfully parse all sending
	// addresses.
	for _, sending := range vector.Sending {
		for _, recipient := range sending.Given.Recipients {
			addr, err := DecodeAddress(recipient)
			require.NoError(t, err)

			require.Equal(t, MainNetHRP, addr.Hrp)
		}
	}
}
