package silentpayments

import (
	"bytes"
	"testing"

	"github.com/btcsuite/btcd/address/v2/bech32"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

// TestAddresses tests the generation of silent payment addresses.
func TestAddresses(t *testing.T) {
	vectors, err := ReadTestVectors()
	require.NoError(t, err)

	for _, vector := range vectors {
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
		require.True(t, IsValidAddress(receiving.Expected.Addresses[0]))

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
			addr, err := DecodeAddress(recipient.Address)
			require.NoError(t, err)

			require.True(t, IsValidAddress(recipient.Address))
			require.Equal(t, MainNetHRP, addr.Hrp)
		}
	}
}

// FuzzDecodeAddress ensures that DecodeAddress never panics on arbitrary input
// and that every address it accepts round-trips: re-encoding a decoded address
// and decoding the result must yield an equal address with a stable string
// representation.
func FuzzDecodeAddress(f *testing.F) {
	// seedAddr builds a valid silent payment address string for the given
	// HRP, so the fuzzer starts from inputs that exercise the success path.
	seedAddr := func(hrp string) string {
		var scanBytes, spendBytes [32]byte
		scanBytes[31] = 1
		spendBytes[31] = 2

		scanPriv, _ := btcec.PrivKeyFromBytes(scanBytes[:])
		spendPriv, _ := btcec.PrivKeyFromBytes(spendBytes[:])

		addr := NewAddress(
			hrp, *scanPriv.PubKey(), *spendPriv.PubKey(), nil,
		)

		return addr.String()
	}

	// Seed the corpus with valid mainnet and testnet addresses, plus a few
	// malformed strings that hit the various early-exit error paths.
	f.Add(seedAddr(MainNetHRP))
	f.Add(seedAddr(TestNetHRP))
	f.Add("")
	f.Add("sp1")
	f.Add("not a silent payment address")

	f.Fuzz(func(t *testing.T, addr string) {
		// The core property is that decoding untrusted input must never
		// panic; it may only return an address or an error.
		decoded, err := DecodeAddress(addr)

		// On any error, no address must be returned.
		if err != nil {
			require.Nil(t, decoded)

			return
		}

		require.NotNil(t, decoded)

		// A successfully decoded address must re-encode to a non-empty
		// string that decodes back to an equal address.
		encoded := decoded.String()
		require.NotEmpty(t, encoded)

		reDecoded, err := DecodeAddress(encoded)
		require.NoError(t, err)
		require.True(t, decoded.Equal(reDecoded))

		// The canonical string form must be stable across re-encoding.
		require.Equal(t, encoded, reDecoded.String())
	})
}

// TestGroupByScanKey tests that GroupByScanKey buckets addresses by their scan
// key, preserves the relative order of addresses that share a scan key, orders
// the groups themselves lexicographically by scan key, and never splits a
// single scan key across multiple groups.
func TestGroupByScanKey(t *testing.T) {
	t.Parallel()

	// addr builds an address with the given scan and spend key, derived
	// deterministically from single-byte private keys.
	addr := func(scan, spend byte) Address {
		return *NewAddress(
			MainNetHRP, *pubKeyFromByte(t, scan),
			*pubKeyFromByte(t, spend), nil,
		)
	}

	// Addresses sharing scan key 1, made unique (and orderable) through
	// distinct spend keys.
	a1, a2, a3 := addr(1, 10), addr(1, 11), addr(1, 12)

	// Addresses sharing scan key 2.
	b1, b2 := addr(2, 20), addr(2, 21)

	// A lone address with scan key 3.
	c1 := addr(3, 30)

	testCases := []struct {
		name     string
		input    []Address
		expected [][]Address
	}{{
		name:     "empty input",
		input:    nil,
		expected: nil,
	}, {
		name:     "single address",
		input:    []Address{a1},
		expected: [][]Address{{a1}},
	}, {
		name:     "all distinct scan keys",
		input:    []Address{a1, b1, c1},
		expected: [][]Address{{a1}, {b1}, {c1}},
	}, {
		// Interleaved input must still group by scan key while keeping
		// the within-group order from the input.
		name:  "multiple per group, order preserved",
		input: []Address{a1, b1, a2, c1, b2, a3},
		expected: [][]Address{
			{a1, a2, a3}, {b1, b2}, {c1},
		},
	}, {
		// Duplicate addresses aren't de-duplicated; they land in the
		// same group in input order.
		name:     "duplicate addresses preserved",
		input:    []Address{a1, a1},
		expected: [][]Address{{a1, a1}},
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := GroupByScanKey(tc.input)

			// The number of groups must match.
			require.Len(t, result, len(tc.expected))

			// The groups must be ordered lexicographically by their
			// scan key.
			for i := 1; i < len(result); i++ {
				prev := result[i-1][0].ScanKey.
					SerializeCompressed()
				curr := result[i][0].ScanKey.
					SerializeCompressed()

				require.Negativef(
					t, bytes.Compare(prev, curr),
					"groups not ordered by scan key at "+
						"index %d", i,
				)
			}

			// Grouping and membership are independent of the group
			// order, so we index both sides by scan key to compare.
			actual := indexByScanKey(t, result)
			expected := indexByScanKey(t, tc.expected)
			require.Len(t, actual, len(expected))

			for key, expGroup := range expected {
				actGroup, ok := actual[key]
				require.True(t, ok, "missing scan key group")

				// The within-group order must be preserved.
				requireAddressesEqual(t, expGroup, actGroup)
			}
		})
	}
}

// indexByScanKey indexes address groups by their shared scan key. It asserts
// that every group is non-empty, that all addresses within a group share the
// same scan key, and that no scan key is spread across multiple groups.
func indexByScanKey(t *testing.T,
	groups [][]Address) map[[33]byte][]Address {

	t.Helper()

	index := make(map[[33]byte][]Address)
	for _, group := range groups {
		require.NotEmpty(t, group)

		var key [33]byte
		copy(key[:], group[0].ScanKey.SerializeCompressed())

		// A scan key must map to exactly one group.
		_, exists := index[key]
		require.False(t, exists, "scan key split across groups")

		// Every address in the group must carry the group's scan key.
		for _, a := range group {
			var aKey [33]byte
			copy(aKey[:], a.ScanKey.SerializeCompressed())
			require.Equal(t, key, aKey)
		}

		index[key] = group
	}

	return index
}

// requireAddressesEqual asserts that two address slices are equal element by
// element and in the same order.
func requireAddressesEqual(t *testing.T, expected, actual []Address) {
	t.Helper()

	require.Len(t, actual, len(expected))
	for i := range expected {
		require.Truef(
			t, expected[i].Equal(&actual[i]),
			"address at index %d differs", i,
		)
	}
}

// pubKeyFromByte returns a deterministic public key derived from a single-byte
// private key, which is enough to produce distinct, valid keys for the test.
func pubKeyFromByte(t *testing.T, b byte) *btcec.PublicKey {
	t.Helper()

	var privKeyBytes [32]byte
	privKeyBytes[31] = b
	privKey, _ := btcec.PrivKeyFromBytes(privKeyBytes[:])

	return privKey.PubKey()
}

// TestDecodeAddressVersions exercises DecodeAddress's version handling (v0
// exact length, v1-30 forward-compat reading the first 66 bytes, v31
// rejection) and HRP validation, which the v0-only official vectors miss.
func TestDecodeAddressVersions(t *testing.T) {
	t.Parallel()

	scanPub := pubKeyFromByte(t, 1).SerializeCompressed()
	spendPub := pubKeyFromByte(t, 2).SerializeCompressed()

	// A valid v0 payload is exactly scan key || spend key (66 bytes).
	payload := append(append([]byte{}, scanPub...), spendPub...)

	// A payload with extra trailing data, for the v1-30 forward-compat
	// path.
	payloadExtra := append(
		append([]byte{}, payload...), bytes.Repeat([]byte{0xab}, 10)...,
	)

	// A payload that is one byte too short.
	payloadShort := payload[:len(payload)-1]

	testCases := []struct {
		name      string
		addr      string
		expectErr bool
	}{{
		name: "v0 exact 66 bytes",
		addr: encodeSPAddress(t, MainNetHRP, 0, payload),
	}, {
		name:      "v0 wrong length",
		addr:      encodeSPAddress(t, MainNetHRP, 0, payloadShort),
		expectErr: true,
	}, {
		name: "v1 exact 66 bytes",
		addr: encodeSPAddress(t, MainNetHRP, 1, payload),
	}, {
		name: "v1 extra trailing data ignored",
		addr: encodeSPAddress(t, MainNetHRP, 1, payloadExtra),
	}, {
		name:      "v1 too short",
		addr:      encodeSPAddress(t, MainNetHRP, 1, payloadShort),
		expectErr: true,
	}, {
		name: "v30 exact 66 bytes",
		addr: encodeSPAddress(t, TestNetHRP, 30, payload),
	}, {
		name:      "v31 rejected",
		addr:      encodeSPAddress(t, MainNetHRP, 31, payload),
		expectErr: true,
	}, {
		name:      "invalid HRP rejected",
		addr:      encodeSPAddress(t, "bc", 0, payload),
		expectErr: true,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			addr, err := DecodeAddress(tc.addr)
			if tc.expectErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			// Regardless of version, the scan and spend keys come
			// from the first 66 bytes of the payload.
			gotScan := addr.ScanKey.SerializeCompressed()
			gotSpend := addr.SpendKey.SerializeCompressed()
			require.Equal(t, scanPub, gotScan)
			require.Equal(t, spendPub, gotSpend)
		})
	}
}

// encodeSPAddress builds a silent payment address string with an explicit
// version byte and raw payload, bypassing EncodeAddress (which always uses
// version 0). This lets the test exercise DecodeAddress's version branches.
func encodeSPAddress(t *testing.T, hrp string, version byte,
	payload []byte) string {

	t.Helper()

	converted, err := bech32.ConvertBits(payload, 8, 5, true)
	require.NoError(t, err)

	combined := append([]byte{version}, converted...)
	addr, err := bech32.EncodeM(hrp, combined)
	require.NoError(t, err)

	return addr
}
