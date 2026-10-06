package descriptors

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDescriptorChecksum checks the BIP380 descriptor checksum against
// rust-validated vectors, and that a descriptor containing a character outside
// the allowed input set yields the empty checksum.
func TestDescriptorChecksum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{{
		name: "wpkh xpub wildcard",
		body: "wpkh(" + basicTestXpub + "/*)",
		want: "97qc8vss",
	}, {
		name: "tr xpub multipath wildcard",
		body: "tr(" + basicTestXpub + "/<0;1>/*)",
		want: "rq20sev9",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := descriptorChecksum(tc.body)
			require.Equal(t, tc.want, got)
			require.Len(t, got, checksumLength)
		})
	}

	// A newline is not part of the descriptor input character set, so the
	// checksum is undefined and reported as the empty string.
	require.Empty(t, descriptorChecksum("wpkh(\n)"))
}
