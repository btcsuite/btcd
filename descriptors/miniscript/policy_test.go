package miniscript

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPolicyNormalize checks the semantic-policy normalization, which flattens
// nested and/or thresholds, drops trivial and unsatisfiable branches, and
// collapses degenerate thresholds. It operates purely on a hand-built policy
// tree, without parsing a miniscript.
func TestPolicyNormalize(t *testing.T) {
	t.Parallel()

	key := func(k string) *Policy {
		return &Policy{Type: PolicyKey, Key: k}
	}
	thresh := func(k int, subs ...*Policy) *Policy {
		return &Policy{Type: PolicyThresh, K: k, Subs: subs}
	}
	trivial := &Policy{Type: PolicyTrivial}
	unsat := &Policy{Type: PolicyUnsatisfiable}

	tests := []struct {
		name  string
		input *Policy
		want  *Policy
	}{{
		name:  "non-thresh passes through",
		input: key("A"),
		want:  key("A"),
	}, {
		name:  "nested AND flattens",
		input: thresh(2, key("A"), thresh(2, key("B"), key("C"))),
		want:  thresh(3, key("A"), key("B"), key("C")),
	}, {
		name:  "nested OR flattens",
		input: thresh(1, key("A"), thresh(1, key("B"), key("C"))),
		want:  thresh(1, key("A"), key("B"), key("C")),
	}, {
		name:  "trivial reduces the threshold",
		input: thresh(2, key("A"), key("B"), trivial),
		want:  thresh(1, key("A"), key("B")),
	}, {
		name:  "unsatisfiable is dropped",
		input: thresh(2, key("A"), key("B"), unsat),
		want:  thresh(2, key("A"), key("B")),
	}, {
		name:  "single sub collapses",
		input: thresh(1, key("A")),
		want:  key("A"),
	}, {
		name:  "all-trivial becomes trivial",
		input: thresh(1, trivial),
		want:  trivial,
	}, {
		name:  "too few survivors becomes unsatisfiable",
		input: thresh(2, key("A"), unsat, unsat),
		want:  unsat,
	}, {
		name:  "general m-of-n is preserved",
		input: thresh(2, key("A"), key("B"), key("C")),
		want:  thresh(2, key("A"), key("B"), key("C")),
	}, {
		name:  "inner thresh inside or is preserved",
		input: thresh(1, key("A"), thresh(2, key("B"), key("C"))),
		want:  thresh(1, key("A"), thresh(2, key("B"), key("C"))),
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.input.Normalize())
		})
	}
}

// TestLiftHash checks literal and substituted commitments independently of key
// derivation, and rejects unresolved hashes instead of silently dropping them.
func TestLiftHash(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		fragment string
		kind     PolicyType
		size     int
	}{
		{"sha256", PolicySha256, 32},
		{"hash256", PolicyHash256, 32},
		{"ripemd160", PolicyRipemd160, 20},
		{"hash160", PolicyHash160, 20},
	} {

		t.Run(tc.fragment, func(t *testing.T) {
			// A literal digest is meaningful before ApplyVars; a
			// symbolic digest is not. Both must yield the same
			// policy once resolved.
			hash := bytes.Repeat([]byte{0x42}, tc.size)
			for _, id := range []string{
				hex.EncodeToString(hash), "H",
			} {

				ast, err := ParseInsane(
					fmt.Sprintf("%s(%s)", tc.fragment, id),
					P2WSH,
				)
				require.NoError(t, err)
				policy, err := ast.Lift()
				if id == "H" {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, hash, policy.Hash)
				}

				require.NoError(
					t, ast.ApplyVars(func(string) ([]byte,
						error) {

						return hash, nil
					}),
				)
				policy, err = ast.Lift()
				require.NoError(t, err)
				require.Equal(t, &Policy{
					Type: tc.kind,
					Hash: hash,
				}, policy)
			}

			// Hex decoding alone must not bless a truncated
			// commitment.
			ast, err := ParseInsane(tc.fragment+"(00)", P2WSH)
			require.NoError(t, err)
			_, err = ast.Lift()
			require.ErrorContains(t, err, "expected")
		})
	}
}
