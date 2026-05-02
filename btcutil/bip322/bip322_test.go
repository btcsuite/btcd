package bip322

import (
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/stretchr/testify/require"
)

// TestBIP322MessageHash tests the MessageHash function against official BIP-322 test vectors.
// See https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki#test-vectors
func TestBIP322MessageHash(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "empty message",
			message:  "",
			expected: "c90c269c4f8fcbe6880f72a721ddfbf1914268a794cbb21cfafee13770ae19f1",
		},
		{
			name:     "hello world",
			message:  "Hello World",
			expected: "f0eb03b1a75ac6d9847f55c624a99169b5dccba2a31f5b23bea77ba270de0a7a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := *chainhash.TaggedHash(
				bip322MsgTag, []byte(tt.message),
			)
			require.Equal(t, tt.expected, hex.EncodeToString(hash[:]))
		})
	}
}
