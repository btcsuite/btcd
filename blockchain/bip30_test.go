package blockchain

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/stretchr/testify/require"
)

// mustHashFromStr is a helper to parse hashes inside tests.  It fails the
// test immediately when the string is not a valid hash.
func mustHashFromStr(t *testing.T, s string) chainhash.Hash {
	t.Helper()

	h, err := chainhash.NewHashFromStr(s)
	require.NoError(t, err)

	return *h
}

// TestBip0030CheckNeededExceptions makes sure the two historical blocks that
// overwrote earlier coinbases do not trigger the BIP30 check.  This mirrors the
// exceptions Bitcoin Core ships as part of the original BIP30 fix.
func TestBip0030CheckNeededExceptions(t *testing.T) {
	params := chaincfg.MainNetParams

	cases := []struct {
		height int32
		hash   string
	}{{
		height: 91842,
		hash:   "00000000000a4d0a398161ffc163c503763b1f4360639393e0e4c8e300e0caec",
	}, {
		height: 91880,
		hash:   "00000000000743f190a18c5577a3c2d2a1f610ae9601ac046a38084ccb7cd721",
	}}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.hash, func(t *testing.T) {
			node := &blockNode{
				height: tc.height,
				hash:   mustHashFromStr(t, tc.hash),
			}
			require.False(t, bip0030CheckNeeded(node, &params))
		})
	}
}

// TestBip0030CheckNeededBeforeBIP34 ensures we still run the BIP30 check prior
// to the recorded BIP34 activation height.
func TestBip0030CheckNeededBeforeBIP34(t *testing.T) {
	params := chaincfg.MainNetParams

	node := &blockNode{
		height: params.BIP0034Height - 1,
		hash:   mustHashFromStr(t, "0000000000000000000000000000000000000000000000000000000000000001"),
	}

	require.True(t, bip0030CheckNeeded(node, &params))
}

// TestBip0030CheckNeededAfterBIP34 covers the happy-path where we are on a
// chain that contains the recorded BIP34 activation block.  In that case the
// expensive duplicate-coinbase check can be skipped.
func TestBip0030CheckNeededAfterBIP34(t *testing.T) {
	params := chaincfg.MainNetParams

	ancestor := &blockNode{
		height: params.BIP0034Height,
		hash:   mustHashFromStr(t, "000000000000024b89b42a942fe0d9fea3bb44ab7bd1b19115dd6a759c0808b8"),
	}
	parent := &blockNode{
		height: ancestor.height + 1,
		hash:   mustHashFromStr(t, "0000000000000000000000000000000000000000000000000000000000000002"),
		parent: ancestor,
	}
	node := &blockNode{
		height: parent.height + 1,
		hash:   mustHashFromStr(t, "0000000000000000000000000000000000000000000000000000000000000003"),
		parent: parent,
	}

	require.False(t, bip0030CheckNeeded(node, &params))
}
