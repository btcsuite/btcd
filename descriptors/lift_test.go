package descriptors

import (
	"testing"

	"github.com/btcsuite/btcd/descriptors/miniscript"
	"github.com/stretchr/testify/require"
)

// TestSemanticPolicyString checks the canonical rendering of a semantic policy:
// keys as pk(...), locktimes and hashes by their named forms, k=n thresholds as
// and(...), k=1 thresholds as or(...), and other thresholds as thresh(k, ...).
func TestSemanticPolicyString(t *testing.T) {
	t.Parallel()

	key := func(k string) *SemanticPolicy {
		return &SemanticPolicy{Type: SemanticPolicyTypeKey, Key: &k}
	}
	after := func(v uint32) *SemanticPolicy {
		return &SemanticPolicy{
			Type: SemanticPolicyTypeAfter, LockTime: &v,
		}
	}
	older := func(v uint32) *SemanticPolicy {
		return &SemanticPolicy{
			Type: SemanticPolicyTypeOlder, LockTime: &v,
		}
	}
	sha := func(h string) *SemanticPolicy {
		return &SemanticPolicy{Type: SemanticPolicyTypeSha256, Hash: &h}
	}
	thresh := func(k uint, subs ...*SemanticPolicy) *SemanticPolicy {
		return &SemanticPolicy{
			Type:      SemanticPolicyTypeThresh,
			Threshold: &k,
			Policies:  subs,
		}
	}

	tests := []struct {
		name   string
		policy *SemanticPolicy
		want   string
	}{{
		name:   "unsatisfiable",
		policy: &SemanticPolicy{Type: SemanticPolicyTypeUnsatisfiable},
		want:   "UNSATISFIABLE",
	}, {
		name:   "trivial",
		policy: &SemanticPolicy{Type: SemanticPolicyTypeTrivial},
		want:   "TRIVIAL",
	}, {
		name:   "key",
		policy: key("A"),
		want:   "pk(A)",
	}, {
		name:   "after",
		policy: after(100),
		want:   "after(100)",
	}, {
		name:   "older",
		policy: older(65535),
		want:   "older(65535)",
	}, {
		name:   "sha256",
		policy: sha("deadbeef"),
		want:   "sha256(deadbeef)",
	}, {
		name:   "and is a k=n threshold",
		policy: thresh(2, key("A"), key("B")),
		want:   "and(pk(A),pk(B))",
	}, {
		name:   "or is a k=1 threshold",
		policy: thresh(1, key("A"), key("B")),
		want:   "or(pk(A),pk(B))",
	}, {
		name:   "general threshold",
		policy: thresh(2, key("A"), key("B"), key("C")),
		want:   "thresh(2,pk(A),pk(B),pk(C))",
	}, {
		name:   "nested",
		policy: thresh(1, key("A"), thresh(2, key("B"), older(144))),
		want:   "or(pk(A),and(pk(B),older(144)))",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.policy.String())
		})
	}
}

// TestFromMiniscriptPolicy checks the conversion from a miniscript semantic
// policy to the descriptor-level SemanticPolicy: keys, hex-encoded hashes and
// locktimes are carried through, and thresholds recurse while keeping their
// threshold value.
func TestFromMiniscriptPolicy(t *testing.T) {
	t.Parallel()

	// A key maps to a key policy.
	got := fromMiniscriptPolicy(&miniscript.Policy{
		Type: miniscript.PolicyKey, Key: "A",
	})
	require.Equal(t, SemanticPolicyTypeKey, got.Type)
	require.Equal(t, "A", *got.Key)

	// A hash is hex-encoded.
	got = fromMiniscriptPolicy(&miniscript.Policy{
		Type: miniscript.PolicySha256, Hash: []byte{0xde, 0xad},
	})
	require.Equal(t, SemanticPolicyTypeSha256, got.Type)
	require.Equal(t, "dead", *got.Hash)

	// A locktime is carried through.
	got = fromMiniscriptPolicy(&miniscript.Policy{
		Type: miniscript.PolicyOlder, Locktime: 144,
	})
	require.Equal(t, SemanticPolicyTypeOlder, got.Type)
	require.Equal(t, uint32(144), *got.LockTime)

	// Trivial and unsatisfiable map across unchanged.
	require.Equal(t, SemanticPolicyTypeTrivial, fromMiniscriptPolicy(
		&miniscript.Policy{Type: miniscript.PolicyTrivial},
	).Type)
	require.Equal(t, SemanticPolicyTypeUnsatisfiable, fromMiniscriptPolicy(
		&miniscript.Policy{Type: miniscript.PolicyUnsatisfiable},
	).Type)

	// A threshold recurses into its sub policies and keeps its threshold.
	got = fromMiniscriptPolicy(&miniscript.Policy{
		Type: miniscript.PolicyThresh, K: 1,
		Subs: []*miniscript.Policy{
			{Type: miniscript.PolicyKey, Key: "A"},
			{Type: miniscript.PolicyOlder, Locktime: 144},
		},
	})
	require.Equal(t, uint(1), *got.Threshold)
	require.Len(t, got.Policies, 2)
	require.Equal(t, "or(pk(A),older(144))", got.String())
}
