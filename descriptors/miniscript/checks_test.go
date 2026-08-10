package miniscript

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValueArgumentWrappers checks that a wrapper on an argument which is a
// value rather than a sub expression is rejected. Wrappers are only defined for
// sub expressions, so the tree passes never look at the wrappers of a key, hash
// or number argument, and accepting them would silently drop them from the
// compiled script: older(v:100) would compile to the script of older(100).
func TestValueArgumentWrappers(t *testing.T) {
	t.Parallel()

	const hash = "6c60f404f8167a38fc70eaf8aa17ac351023bef86bcb9d1086a1" +
		"9afe95bd5333"

	for _, expr := range []string{
		"older(v:100)",
		"after(xyz:500000000)",
		"multi(j:1,key1)",
		"multi(1,v:key1)",
		"thresh(n:1,pk(key1))",
		"pk(v:key1)",
		"pk_k(d:key1)",
		"pkh(a:key1)",
		"pk_h(s:key1)",
		"sha256(v:" + hash + ")",
		"hash160(c:" + hash[:40] + ")",
	} {

		t.Run(expr, func(t *testing.T) {
			t.Parallel()

			_, err := ParseInsane(expr, P2WSH)
			require.ErrorContains(t, err, "must not have wrappers")
		})
	}

	// The same expressions without the wrapper are accepted, so it is
	// really only the wrapper that is rejected.
	for _, expr := range []string{
		"older(100)", "after(500000000)", "multi(1,key1)",
		"thresh(1,pk(key1))", "pk(key1)", "pk_k(key1)", "pkh(key1)",
		"pk_h(key1)", "sha256(" + hash + ")",
		"hash160(" + hash[:40] + ")",
	} {

		t.Run(expr, func(t *testing.T) {
			t.Parallel()

			_, err := ParseInsane(expr, P2WSH)
			require.NoError(t, err)
		})
	}
}
