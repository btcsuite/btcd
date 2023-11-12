package miniscript

import (
	"testing"

	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/stretchr/testify/require"
)

// applyDummyKeys resolves single-letter key placeholders to arbitrary but
// distinct 33-byte values and leaves hex hash values untouched, so that a
// parsed miniscript can be turned into a concrete script for inspection.
func applyDummyKeys(t *testing.T, node *AST) {
	t.Helper()
	err := node.ApplyVars(func(id string) ([]byte, error) {
		if len(id) == 64 || len(id) == 40 {
			return nil, nil
		}
		return append(chainhash.HashB([]byte(id)), 0), nil
	})
	require.NoError(t, err)
}

// TestThreshUnitProperty verifies the `o` (one) type property of thresh, which
// was previously always computed as false. rust-miniscript computes it as true
// for a single-sub threshold whose sub reads exactly one stack element.
func TestThreshUnitProperty(t *testing.T) {
	t.Parallel()

	// thresh(1,pk(A)): the single sub c:pk_k reads one element, so the
	// threshold has the `o` property.
	single, err := Parse("thresh(1,pk(A))", P2WSH)
	require.NoError(t, err)
	require.True(t, single.props.o, "thresh(1,pk(A)) must have the o property")

	// A multi-sub threshold never has the o property.
	multi, err := Parse("thresh(1,pk(A),s:pk(B))", P2WSH)
	require.NoError(t, err)
	require.False(t, multi.props.o)

	// Because thresh(1,pk(B)) has o, the s: wrapper accepts it, so this
	// composes. Before the fix this was wrongly rejected.
	_, err = Parse("and_b(pk(A),s:thresh(1,pk(B)))", P2WSH)
	require.NoError(t, err, "and_b(pk(A),s:thresh(1,pk(B))) must parse")

	// pkh reads more than one element (input Any), so thresh(1,pkh(B)) does
	// not have o and the s: wrapper must reject it (matching rust).
	_, err = Parse("and_b(pk(A),s:thresh(1,pkh(B)))", P2WSH)
	require.Error(t, err)
}

// TestVerifyWrapperCollapse verifies that the v: wrapper only collapses the
// single final opcode of its child into a VERIFY variant, and never rewrites
// collapsible opcodes that appear in non-final positions (which would produce
// an invalid script).
func TestVerifyWrapperCollapse(t *testing.T) {
	t.Parallel()

	h := "926a54995ca48600920a19bf7bc502ca5f2f7d07e6f804c4f00ebf0325084dbc"

	// v: wraps or_d, whose final opcode is OP_ENDIF (not collapsible), so a
	// separate OP_VERIFY is appended. The inner OP_CHECKSIG (from c:pk_h)
	// and OP_CHECKMULTISIG (from multi) must NOT be collapsed to their
	// VERIFY variants, since they are followed by more script.
	node, err := Parse("and_v(v:or_d(c:pk_h(A),multi(2,B,C,D)),sha256("+
		h+"))", P2WSH)
	require.NoError(t, err)
	applyDummyKeys(t, node)

	script, err := node.Script()
	require.NoError(t, err)
	disasm, err := txscript.DisasmString(script)
	require.NoError(t, err)

	require.Contains(t, disasm, "OP_CHECKSIG OP_IFDUP",
		"inner checksig must stay non-VERIFY")
	require.Contains(t, disasm, "OP_CHECKMULTISIG OP_ENDIF OP_VERIFY",
		"inner checkmultisig must stay non-VERIFY; the v: adds OP_VERIFY "+
			"after the ENDIF")
	require.NotContains(t, disasm, "OP_CHECKSIGVERIFY")
	require.NotContains(t, disasm, "OP_CHECKMULTISIGVERIFY")

	// By contrast, v: directly on multi collapses the final CHECKMULTISIG.
	node2, err := Parse("and_v(v:multi(2,A,B,C),pk(D))", P2WSH)
	require.NoError(t, err)
	applyDummyKeys(t, node2)
	script2, err := node2.Script()
	require.NoError(t, err)
	disasm2, err := txscript.DisasmString(script2)
	require.NoError(t, err)
	require.Contains(t, disasm2, "OP_CHECKMULTISIGVERIFY",
		"v:multi collapses the final CHECKMULTISIG")
}

// TestThreshScriptLen verifies that the computed script length of a threshold
// matches the actual generated script length for thresholds where the number of
// sub expressions differs from k+1 (the only case the old formula handled).
func TestThreshScriptLen(t *testing.T) {
	t.Parallel()

	cases := []string{
		"thresh(1,pk(A),s:pk(B),s:pk(C))",
		"thresh(2,pk(A),s:pk(B),s:pk(C))",
		"thresh(3,pk(A),s:pk(B),s:pk(C))",
		"thresh(2,pk(A),s:pk(B),s:pk(C),s:pk(D))",
		"thresh(4,pk(A),s:pk(B),s:pk(C),s:pk(D))",
	}
	for _, ms := range cases {
		node, err := Parse(ms, P2WSH)
		require.NoError(t, err)
		applyDummyKeys(t, node)
		script, err := node.Script()
		require.NoError(t, err)
		require.Equalf(t, len(script), node.scriptLen,
			"computed script length mismatch for %s", ms)
	}
}

// TestParseRejections asserts that a set of expressions rejected by
// rust-miniscript are also rejected by this implementation.
func TestParseRejections(t *testing.T) {
	t.Parallel()

	rejects := []string{
		// s: requires its child to have the o property; pkh does not.
		"and_b(pk(A),s:thresh(1,pkh(B)))",

		// t: (and_v(X,1)) requires a V-type first argument.
		"t:pk(A)",

		// j: cannot wrap a K-type fragment.
		"j:pk_k(A)",

		// n: cannot wrap a K-type fragment.
		"n:pk_k(A)",
	}
	for _, ms := range rejects {
		_, err := Parse(ms, P2WSH)
		require.Errorf(t, err, "%s must be rejected", ms)
	}
}
