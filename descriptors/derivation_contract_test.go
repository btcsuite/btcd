package descriptors

import (
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/descriptors/miniscript"
	"github.com/stretchr/testify/require"
)

// TestDerivationContracts compares cached derivation with fresh parsing and
// checks that sorted multisig is independent of the input key permutation.
func TestDerivationContracts(t *testing.T) {
	t.Parallel()

	// Revisiting earlier indices exposes accidental mutation of the cached
	// AST. Each reference descriptor starts with an entirely fresh cache.
	for _, expression := range []string{
		"wsh(and_v(v:pk(" + testXpub1 + "),pk(" + testXpub2 + ")))",
		"wsh(sortedmulti(2," + testXpub1 + "," + testXpub2 + "))",
	} {

		cached, err := NewDescriptor(expression)
		require.NoError(t, err)
		for _, index := range []uint32{0, 7, 1, 0, 7} {
			fresh, err := NewDescriptor(expression)
			require.NoError(t, err)
			want, err := fresh.ScriptCodeAt(0, index)
			require.NoError(t, err)
			got, err := cached.ScriptCodeAt(0, index)
			require.NoError(t, err)
			require.Equal(t, want, got)
		}
	}

	// Distinct keys avoid duplicate-key rejection obscuring the sorting
	// property. Check every permutation against the same canonical script.
	keys := testCompressedKeys(3)
	var want []byte
	for _, order := range [][3]int{
		{0, 1, 2}, {0, 2, 1}, {1, 0, 2},
		{1, 2, 0}, {2, 0, 1}, {2, 1, 0},
	} {

		d, err := NewDescriptor(fmt.Sprintf(
			"wsh(sortedmulti(2,%s,%s,%s))", keys[order[0]],
			keys[order[1]], keys[order[2]],
		))
		require.NoError(t, err)
		got, err := d.ScriptCodeAt(0, 0)
		require.NoError(t, err)
		if want == nil {
			want = got
		}
		require.Equal(t, want, got)
	}
}

// TestCloneOwnsValues checks independence both before and after substitution;
// callers may reuse the original while deriving or modifying a clone.
func TestCloneOwnsValues(t *testing.T) {
	t.Parallel()

	// Separate the callback's ownership from Clone's contract: ApplyVars
	// may retain its input, but Clone must copy every retained value.
	a, err := miniscript.Parse("and_v(v:pk(A),pk(B))", miniscript.P2WSH)
	require.NoError(t, err)
	keys := map[string][]byte{"A": make([]byte, 33), "B": make([]byte, 33)}
	keys["A"][0], keys["B"][0] = 2, 3
	unresolved := a.Clone()
	require.NoError(t, a.ApplyVars(func(name string) ([]byte, error) {
		return keys[name], nil
	}))
	want, err := a.Script()
	require.NoError(t, err)
	resolved := a.Clone()

	// Mutate the original's backing storage, then resolve a different copy.
	// Neither operation may alter the already resolved clone.
	keys["A"][1] = 1
	require.NoError(
		t, unresolved.ApplyVars(func(name string) ([]byte, error) {
			return keys[name], nil
		}),
	)
	got, err := resolved.Script()
	require.NoError(t, err)
	require.Equal(t, want, got)
	changed, err := unresolved.Script()
	require.NoError(t, err)
	require.NotEqual(t, want, changed)
}
