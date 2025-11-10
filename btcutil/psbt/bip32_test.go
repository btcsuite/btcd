package psbt

import (
	"testing"

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
