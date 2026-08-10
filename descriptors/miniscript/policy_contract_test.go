package miniscript

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// policyTruth evaluates key-only threshold policies directly, without using
// normalization or miniscript analysis to decide which signers suffice.
func policyTruth(p *Policy, keys uint8) bool {
	switch p.Type {
	case PolicyTrivial:
		return true

	case PolicyKey:
		return keys&(1<<(p.Key[0]-'A')) != 0

	case PolicyThresh:
		// A policy threshold means at least K available children, even
		// though a script satisfaction may select exactly K of them.
		count := 0
		for _, sub := range p.Subs {
			if policyTruth(sub, keys) {
				count++
			}
		}
		return count >= p.K

	default:
		return false
	}
}

// TestNormalizeContracts checks meaning, idempotence and non-mutation across
// nested thresholds, including constants that reduce the required threshold.
func TestNormalizeContracts(t *testing.T) {
	t.Parallel()

	// Exhaust both child thresholds and the outer threshold. Mixed AND,
	// OR and general thresholds catch flattening that loses grouping.
	for left := range 4 {
		for right := range 4 {
			for outer := range 5 {
				p := &Policy{Type: PolicyThresh, K: outer, Subs: []*Policy{
					{Type: PolicyThresh, K: left, Subs: []*Policy{
						{Type: PolicyKey, Key: "A"},
						{Type: PolicyKey, Key: "B"},
						{Type: PolicyTrivial},
					}},
					{Type: PolicyThresh, K: right, Subs: []*Policy{
						{Type: PolicyKey, Key: "C"},
						{Type: PolicyKey, Key: "D"},
						{Type: PolicyUnsatisfiable},
					}},
					{Type: PolicyTrivial},
					{Type: PolicyUnsatisfiable},
				}}
				before, err := json.Marshal(p)
				require.NoError(t, err)
				normalized := p.Normalize()
				require.Equal(
					t, normalized, normalized.Normalize(),
				)

				// Compare all signer subsets, including none
				// and all. Serialization snapshots the input
				// without sharing nodes.
				for mask := range uint8(16) {
					require.Equal(
						t, policyTruth(p, mask),
						policyTruth(normalized, mask),
						"left=%d right=%d outer=%d "+
							"mask=%d",
						left, right, outer, mask,
					)
				}
				after, err := json.Marshal(p)
				require.NoError(t, err)
				require.Equal(t, before, after)
			}
		}
	}
}
