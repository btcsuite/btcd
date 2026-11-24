package psbt

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/stretchr/testify/require"
)

// TestReadBip32Derivation tests the ReadBip32Derivation function to ensure
// it correctly deserializes the BIP32 derivation path and master key
// fingerprint from a byte slice.
func TestReadBip32Derivation(t *testing.T) {
	tests := []struct {
		name              string
		path              []byte
		expectFingerprint uint32
		expectPath        []uint32
		expectErr         error
	}{
		{
			name: "valid path with multiple derivations",
			path: []byte{
				0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00,
				0x03, 0x00, 0x00, 0x00},
			expectFingerprint: 1,
			expectPath:        []uint32{2, 3},
		},
		{
			name: "valid path with single derivation",
			path: []byte{
				0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00,
			},
			expectFingerprint: 1,
			expectPath:        []uint32{2},
		},
		{
			name: "valid path with no derivations",
			path: []byte{
				0x01, 0x00, 0x00, 0x00,
			},
			expectFingerprint: 1,
			expectPath:        nil,
		},
		{
			name: "invalid path length",
			path: []byte{
				0x01, 0x00, 0x00,
			},
			expectErr: ErrInvalidPsbtFormat,
		},
		{
			name: "invalid path length not multiple of 4",
			path: []byte{
				0x01, 0x00, 0x00, 0x00, 0x02,
			},
			expectErr: ErrInvalidPsbtFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp, path, err := ReadBip32Derivation(tt.path)
			if tt.expectErr != nil {
				require.ErrorIs(t, err, tt.expectErr)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectFingerprint, fp)
			require.Equal(t, tt.expectPath, path)
		})
	}
}

// makeTestExtendedKey builds a serialized 78-byte BIP-32 extended public key
// with the requested depth for testing ReadXPub path-length validation.
func makeTestExtendedKey(t *testing.T, depth uint8) []byte {
	t.Helper()

	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	chainCode := bytes.Repeat([]byte{0x11}, 32)
	parentFP := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	childNum := []byte{0x00, 0x00, 0x00, 0x01}

	extKey := hdkeychain.NewExtendedKey(
		chaincfg.MainNetParams.HDPublicKeyID[:],
		privKey.PubKey().SerializeCompressed(),
		chainCode,
		parentFP,
		depth,
		binary.BigEndian.Uint32(childNum),
		false,
	)

	// DecodeExtendedKey expects the 78-byte binary BIP-32 serialization
	// (version + depth + parentFP + childNum + chainCode + 33-byte pubkey).
	raw := make([]byte, 0, 78)
	raw = append(raw, chaincfg.MainNetParams.HDPublicKeyID[:]...)
	raw = append(raw, depth)
	raw = append(raw, parentFP...)
	raw = append(raw, childNum...)
	raw = append(raw, chainCode...)
	raw = append(raw, privKey.PubKey().SerializeCompressed()...)

	parsed, err := DecodeExtendedKey(raw)
	require.NoError(t, err)
	require.Equal(t, extKey.Depth(), parsed.Depth())

	return raw
}

// TestReadXPubDepthOverflow verifies that ReadXPub does not overflow uint8
// when computing expected path length for extended keys with Depth() >= 63.
func TestReadXPubDepthOverflow(t *testing.T) {
	const depth = 63
	keyData := makeTestExtendedKey(t, depth)

	// Positive control: a depth-63 key requires 4 * (63 + 1) = 256 bytes
	// (4-byte master fingerprint plus 63 uint32 path elements).
	validPath := make([]byte, 4*(depth+1))
	xpub, err := ReadXPub(keyData, validPath)
	require.NoError(t, err)
	require.NotNil(t, xpub)
	require.Len(t, xpub.Bip32Path, depth)

	// Regression check: a 0-byte path must be rejected even though
	// uint8(4 * (63 + 1)) wraps around to 0.
	xpub, err = ReadXPub(keyData, []byte{})
	require.ErrorIs(t, err, ErrInvalidPsbtFormat)
	require.Nil(t, xpub)
}
