package miniscript

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// nestedWrappers returns an expression of n `n:` wrappers around a pk, which
// nests n+1 levels deep and is the cheapest way to nest deeply: one byte of
// input per level.
func nestedWrappers(n int) string {
	return strings.Repeat("n", n) + ":pk(A)"
}

// nestedAndV returns an expression of depth nested and_v fragments, which nests
// 2*depth+1 levels deep (the and_v and the v: wrapper of each level, plus the
// innermost pk). Every level uses a distinct key identifier, since a miniscript
// must not repeat a key.
func nestedAndV(depth int) string {
	var b strings.Builder
	for i := 0; i < depth; i++ {
		fmt.Fprintf(&b, "and_v(v:pk(A%d),", i)
	}
	b.WriteString("pk(B)")
	b.WriteString(strings.Repeat(")", depth))

	return b.String()
}

// TestNestingDepthLimit checks that expressions nesting deeper than
// maxNestingDepth are rejected at parse time. Without the limit, the recursive
// tree passes (here Parse and Script) grow the goroutine stack until the
// runtime throws a fatal stack overflow, which recover() cannot catch: a ~1 MB
// input was enough to kill the process.
func TestNestingDepthLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		expr      string
		ctx       Context
		expectErr bool
	}{{
		// 401 wrappers plus the pk they wrap are exactly at the limit.
		name: "wrappers at limit",
		expr: nestedWrappers(maxNestingDepth - 1),
		ctx:  P2TR,
	}, {
		name:      "wrappers one over limit",
		expr:      nestedWrappers(maxNestingDepth),
		ctx:       P2TR,
		expectErr: true,
	}, {
		// The input from the report: one byte per level, so a megabyte
		// of input nests a million levels deep.
		name:      "one megabyte of wrappers",
		expr:      strings.Repeat("n", 1_000_000) + ":1",
		ctx:       P2WSH,
		expectErr: true,
	}, {
		name: "nested and_v below limit",
		expr: nestedAndV(200),
		ctx:  P2TR,
	}, {
		name:      "nested and_v over limit",
		expr:      nestedAndV(500),
		ctx:       P2TR,
		expectErr: true,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			node, err := Parse(tc.expr, tc.ctx)
			if tc.expectErr {
				require.ErrorContains(t, err, "nesting depth")
				return
			}

			require.NoError(t, err)

			// An expression at the limit must remain fully usable,
			// i.e. the recursive passes that run after Parse have
			// to cope with the deepest tree Parse accepts.
			require.NoError(t, node.ApplyVars(
				func(identifier string) ([]byte, error) {
					return testKey(identifier), nil
				},
			))
			_, err = node.Script()
			require.NoError(t, err)
		})
	}
}

// testKey returns a deterministic x-only public key for a key identifier used
// in the depth tests. The value does not matter, only its length.
func testKey(identifier string) []byte {
	key := make([]byte, xOnlyPubKeyLen)
	copy(key, identifier)

	return key
}
