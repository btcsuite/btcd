package descriptors

import (
	"fmt"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/stretchr/testify/require"
)

// degenerateTapTree returns a tr() descriptor whose script tree is a chain of
// branches nesting the given number of levels deep, i.e. its deepest leaves sit
// at that level: {leaf,{leaf,{leaf,...}}}.
func degenerateTapTree(depth int) string {
	xOnly := testCompressedKeys(depth + 2)

	var b strings.Builder
	fmt.Fprintf(&b, "tr(%s,", xOnly[0][2:])
	for i := range depth {
		fmt.Fprintf(&b, "{pk(%s),", xOnly[i+1][2:])
	}
	fmt.Fprintf(&b, "pk(%s)", xOnly[depth+1][2:])
	b.WriteString(strings.Repeat("}", depth) + ")")

	return b.String()
}

// TestTapTreeDepthLimit checks that the depth of a taproot script tree is
// bounded. Every level adds a 32-byte hash to the merkle path in the control
// block of the leaves below it, and BIP341 allows at most 128, so a deeper tree
// has unspendable leaves.
//
// The bound is also what keeps parsing affordable: parseTapTree used to re-scan
// the entire remaining subtree at every level, so a degenerate tree cost
// O(depth * length) - a 570 KB descriptor took over two seconds, and a ~5 MB
// one minutes, which let a single small request pin a CPU. It also removes the
// unbounded recursion of parseTapTree and of every later walk of the tree
// (address derivation, planning, lifting), which would otherwise end in a fatal
// stack overflow.
func TestTapTreeDepthLimit(t *testing.T) {
	t.Parallel()

	// A tree whose leaves are exactly at the maximum depth is valid and has
	// to stay usable.
	d, err := NewDescriptor(degenerateTapTree(maxTapTreeDepth))
	require.NoError(t, err)

	_, err = d.AddressAt(&chaincfg.MainNetParams, 0, 0)
	require.NoError(t, err)

	// One level deeper, the leaves could not be spent.
	_, err = NewDescriptor(degenerateTapTree(maxTapTreeDepth + 1))
	require.ErrorContains(t, err, "taproot script tree is deeper than 128")

	// The input from the report, which used to parse in seconds rather than
	// being rejected.
	_, err = NewDescriptor(degenerateTapTree(8000))
	require.ErrorContains(t, err, "taproot script tree is deeper than 128")
}

// TestSplitTapBranch checks the branch splitting: a branch has exactly two
// children, and the split stops at the first top-level comma so that parsing a
// tree avoids re-scanning the remaining right subtree.
func TestSplitTapBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inner     string
		wantLeft  string
		wantRight string
		expectErr bool
	}{{
		name:      "two leaves",
		inner:     "pk(A),pk(B)",
		wantLeft:  "pk(A)",
		wantRight: "pk(B)",
	}, {
		name:      "branch on the right",
		inner:     "pk(A),{pk(B),pk(C)}",
		wantLeft:  "pk(A)",
		wantRight: "{pk(B),pk(C)}",
	}, {
		name:      "branch on the left",
		inner:     "{pk(A),pk(B)},pk(C)",
		wantLeft:  "{pk(A),pk(B)}",
		wantRight: "pk(C)",
	}, {
		// A comma inside a fragment, a key origin or a multipath
		// element is not a top-level one.
		name:      "commas inside the children",
		inner:     "multi_a(2,A,B),pk([00000000/1]C)",
		wantLeft:  "multi_a(2,A,B)",
		wantRight: "pk([00000000/1]C)",
	}, {
		name:      "single child",
		inner:     "pk(A)",
		expectErr: true,
	}, {
		name:      "three leaves",
		inner:     "pk(A),pk(B),pk(C)",
		expectErr: true,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			left, right, err := splitTapBranch(tc.inner)
			if tc.expectErr {
				require.ErrorContains(
					t, err, "exactly two children",
				)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.wantLeft, left)
			require.Equal(t, tc.wantRight, right)
		})
	}
}

// TestTopLevelComma checks the top-level comma search that the branch split
// uses.
func TestTopLevelComma(t *testing.T) {
	t.Parallel()

	require.Equal(t, -1, topLevelComma(""))
	require.Equal(t, -1, topLevelComma("pk(A)"))
	require.Equal(t, -1, topLevelComma("multi_a(2,A,B)"))
	require.Equal(t, -1, topLevelComma("{pk(A),pk(B)}"))
	require.Equal(t, -1, topLevelComma("pk([00000000/1]A)"))
	require.Equal(t, -1, topLevelComma("pk(xpub/<0;1>/*)"))
	require.Equal(t, 5, topLevelComma("pk(A),pk(B)"))
	require.Equal(t, 13, topLevelComma("{pk(A),pk(B)},pk(C)"))
}

// BenchmarkTapTreeParse measures parsing a maximum-depth right-skewed tap tree.
func BenchmarkTapTreeParse(b *testing.B) {
	desc := degenerateTapTree(maxTapTreeDepth)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := NewDescriptor(desc); err != nil {
			require.NoError(b, err, "parse")
		}
	}
}

// TestTapLeafFragments checks which expressions may be used as a tap leaf.
// BIP386 notes that "as of 2021-06-27, the only allowed script expression that
// can be used in a tree expression is pk()", and lists tr(K,pkh(K2)) as
// invalid, but BIP379 and BIP387 postdate it and extend tree expressions to any
// miniscript fragment and to multi_a()/sortedmulti_a(). This package follows
// the newer BIPs, which is a deliberate divergence from BIP386's vector list.
func TestTapLeafFragments(t *testing.T) {
	t.Parallel()

	keys := testCompressedKeys(4)
	xOnly := make([]string, len(keys))
	for i, key := range keys {
		xOnly[i] = key[2:]
	}

	for _, leaf := range []string{
		"pk(" + xOnly[1] + ")",
		"pkh(" + xOnly[1] + ")",
		"multi_a(2," + xOnly[1] + "," + xOnly[2] + ")",
		"sortedmulti_a(2," + xOnly[1] + "," + xOnly[2] + ")",
		"and_v(v:pk(" + xOnly[1] + "),older(9))",
		"{pk(" + xOnly[1] + "),pk(" + xOnly[2] + ")}",
	} {

		desc := "tr(" + xOnly[0] + "," + leaf + ")"
		_, err := NewDescriptor(desc)
		require.NoErrorf(t, err, "descriptor %s", desc)
	}

	// Whitespace is not insignificant in a descriptor, so the same
	// expression with a space is rejected.
	_, err := NewDescriptor("tr(" + xOnly[0] + ", pk(" + xOnly[1] + "))")
	require.Error(t, err)
}
