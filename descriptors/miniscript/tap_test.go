package miniscript

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExecStack checks the execution stack size computation against hand
// computed values. This is the property that feeds the P2TR total stack size
// limit, and is not compared against rust-miniscript (whose value is a looser,
// order-dependent estimate).
func TestExecStack(t *testing.T) {
	t.Parallel()

	cases := []struct {
		miniscript string
		ctx        Context
		exec       int
	}{{
		// c:pk_k pushes the public key.
		miniscript: "pk(A)",
		ctx:        P2WSH,
		exec:       1,
	}, {
		// OP_CHECKMULTISIG has <k>, all n keys and <n> on the stack.
		miniscript: "multi(2,A,B,C)",
		ctx:        P2WSH,
		exec:       5,
	}, {
		// The X branch of or_d leaves one element, which OP_IFDUP
		// duplicates: max(1, 2) = 2.
		miniscript: "or_d(pk(A),pk(B))",
		ctx:        P2WSH,
		exec:       2,
	}, {
		// The Z branch runs on a clean stack, so a larger peak there
		// dominates: max(2, multi(2,B,C,D) = 5) = 5.
		miniscript: "or_d(pk(A),multi(2,B,C,D))",
		ctx:        P2WSH,
		exec:       5,
	}, {
		// The left result stays on the stack while the right runs, so
		// max(1, 1+1) = 2.
		miniscript: "and_b(pk(A),s:pk(B))",
		ctx:        P2WSH,
		exec:       2,
	}, {
		// Only one branch executes.
		miniscript: "or_i(pk(A),pk(B))",
		ctx:        P2WSH,
		exec:       1,
	}, {
		// Two pk checks (1 each) plus the running total: peak 2.
		miniscript: "thresh(2,pk(A),s:pk(B),s:pk(C))",
		ctx:        P2WSH,
		exec:       2,
	}, {
		// The two numbers before OP_NUMEQUAL.
		miniscript: "multi_a(2,A,B,C)",
		ctx:        P2TR,
		exec:       2,
	}, {
		// v:pk leaves nothing, then pk runs: max(1, 1) = 1.
		miniscript: "and_v(v:pk(A),pk(B))",
		ctx:        P2TR,
		exec:       1,
	}, {
		miniscript: "thresh(2,pk(A),s:pk(B),s:pk(C))",
		ctx:        P2TR,
		exec:       2,
	}}

	for _, tc := range cases {
		node, err := Parse(tc.miniscript, tc.ctx)
		require.NoErrorf(t, err, "parsing %s", tc.miniscript)
		require.Equalf(t, tc.exec, node.maxExecStackSize(),
			"exec stack size for %s", tc.miniscript)
	}
}
