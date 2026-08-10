package miniscript

import (
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
