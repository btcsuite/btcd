package miniscript

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExecStack checks the execution stack size computation against hand
// computed values: the peak number of stack elements, including the initial
// witness elements. This is the property that feeds the total stack size limit
// of both segwit contexts. It is not compared against rust-miniscript, whose
// value is a looser, order-dependent estimate.
func TestExecStack(t *testing.T) {
	t.Parallel()

	cases := []struct {
		miniscript string
		ctx        Context
		exec       int
	}{{
		// The signature, then the pushed public key.
		miniscript: "pk(A)",
		ctx:        P2WSH,
		exec:       2,
	}, {
		// OP_CHECKMULTISIG has the dummy zero, the two signatures, <k>,
		// all n keys and <n> on the stack.
		miniscript: "multi(2,A,B,C)",
		ctx:        P2WSH,
		exec:       8,
	}, {
		// Dissatisfying X starts from two elements (the signature for B
		// and the empty one for A) and pushes the key of A on top.
		miniscript: "or_d(pk(A),pk(B))",
		ctx:        P2WSH,
		exec:       3,
	}, {
		// The Z branch starts from the four elements of both witnesses,
		// of which the dissatisfaction of X is consumed before the
		// multi pushes its five elements on top of the other three.
		miniscript: "or_d(pk(A),multi(2,B,C,D))",
		ctx:        P2WSH,
		exec:       8,
	}, {
		// Both signatures, then the key of A on top. The result of A
		// then stays on the stack while B pushes its key.
		miniscript: "and_b(pk(A),s:pk(B))",
		ctx:        P2WSH,
		exec:       3,
	}, {
		// The signature and the branch selector, which OP_IF consumes
		// before the key is pushed.
		miniscript: "or_i(pk(A),pk(B))",
		ctx:        P2WSH,
		exec:       2,
	}, {
		// Three witness elements and the first key.
		miniscript: "thresh(2,pk(A),s:pk(B),s:pk(C))",
		ctx:        P2WSH,
		exec:       4,
	}, {
		// Three witness elements and the first key.
		miniscript: "multi_a(2,A,B,C)",
		ctx:        P2TR,
		exec:       4,
	}, {
		// Two signatures and the first key.
		miniscript: "and_v(v:pk(A),pk(B))",
		ctx:        P2TR,
		exec:       3,
	}, {
		miniscript: "thresh(2,pk(A),s:pk(B),s:pk(C))",
		ctx:        P2TR,
		exec:       4,
	}, {
		// Core's test vector for the same expression: two witness
		// elements and the first key.
		miniscript: "and_v(v:multi_a(2,A,B),after(1231488000))",
		ctx:        P2TR,
		exec:       3,
	}}

	for _, tc := range cases {
		node, err := Parse(tc.miniscript, tc.ctx)
		require.NoErrorf(t, err, "parsing %s", tc.miniscript)
		require.Equalf(t, tc.exec, node.maxExecStackSize(), "exec "+
			"stack size for %s", tc.miniscript)
	}
}
