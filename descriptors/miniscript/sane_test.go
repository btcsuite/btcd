package miniscript

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	// testHash is a sha256 hash value used to pad expressions to a given
	// script size or op count.
	testHash = "926a54995ca48600920a19bf7bc502ca5f2f7d07e6f804c4f00ebf032" +
		"5084dbc"
)

// hashChain returns and_v(v:pk(A),and_v(v:sha256(H),...,sha256(H))) with n hash
// fragments, which needs a signature and is non-malleable, so the only thing
// that can make it insane is its size: every hash fragment adds 39 script bytes
// and 4 ops.
func hashChain(n int) string {
	var b strings.Builder
	b.WriteString("and_v(v:pk(A),")
	for range n - 1 {
		b.WriteString("and_v(v:sha256(" + testHash + "),")
	}
	b.WriteString("sha256(" + testHash + ")")
	b.WriteString(strings.Repeat(")", n))

	return b.String()
}

// TestParseRejectsInsane checks that Parse enforces the sanity checks, which
// used to be dead code: IsSane, IsValidTopLevel, isSaneSubexpression and
// validSatisfactions had no callers outside of tests, so Parse accepted (and
// the descriptor package derived addresses for) scripts that are provably
// unspendable, over a consensus limit, malleable by third parties or broken by
// mixed time locks. ParseInsane must still accept all of them, since the
// analysis passes themselves are unaffected.
func TestParseRejectsInsane(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		expr   string
		ctx    Context
		errStr string
	}{{
		// A type-V top level is a provably unspendable script: the
		// VERIFY leaves nothing on the stack.
		name:   "top level V",
		expr:   "v:pk(A)",
		ctx:    P2WSH,
		errStr: "expected to have type B, but is type V",
	}, {
		// A type-K top level leaves a public key on the stack.
		name:   "top level K",
		expr:   "pk_k(A)",
		ctx:    P2WSH,
		errStr: "expected to have type B, but is type K",
	}, {
		name:   "relative timelock mixing",
		expr:   "and_v(v:pk(A),and_v(v:older(10),older(4194305)))",
		ctx:    P2WSH,
		errStr: "combination of height-based and time-based",
	}, {
		name:   "absolute timelock mixing",
		expr:   "and_v(v:pk(A),and_v(v:after(10),after(500000001)))",
		ctx:    P2WSH,
		errStr: "combination of height-based and time-based",
	}, {
		// Either branch alone satisfies the or_b, so a third party can
		// strip the other one and change the witness.
		name:   "malleable",
		expr:   "or_b(pk(A),al:pk(B))",
		ctx:    P2WSH,
		errStr: "malleable",
	}, {
		// Anyone can spend this once the time lock expires.
		name:   "no signature required",
		expr:   "older(144)",
		ctx:    P2WSH,
		errStr: "does not need signature",
	}, {
		// 51 hash fragments need 205 ops, over the P2WSH consensus
		// limit: an output with this witness script is unspendable.
		name:   "over the P2WSH op count limit",
		expr:   hashChain(51),
		ctx:    P2WSH,
		errStr: "larger than the consensus limit of 201",
	}, {
		// 93 hash fragments need 3662 script bytes, over the P2WSH
		// standardness limit.
		name:   "over the P2WSH script size limit",
		expr:   hashChain(93),
		ctx:    P2WSH,
		errStr: "larger than the maximum script size of 3600",
	}, {
		// The two occurrences of the key look like two signatures are
		// needed, but one signature satisfies both, which is what
		// BIP379's analysis of an expression assumes away.
		name:   "duplicate key",
		expr:   "and_v(v:pk(A),pk(A))",
		ctx:    P2WSH,
		errStr: "duplicate key A",
	}, {
		name:   "duplicate key in a multi",
		expr:   "and_v(v:pk(A),multi(2,B,A))",
		ctx:    P2WSH,
		errStr: "duplicate key A",
	}, {
		// A tapscript leaf is subject to the same sanity rules, minus
		// the ones that only apply to P2WSH.
		name:   "tapscript leaf without signature",
		expr:   "older(144)",
		ctx:    P2TR,
		errStr: "does not need signature",
	}, {
		name:   "duplicate key in a tapscript leaf",
		expr:   "and_v(v:pk(A),multi_a(2,B,A))",
		ctx:    P2TR,
		errStr: "duplicate key A",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := Parse(tc.expr, tc.ctx)
			require.ErrorContains(t, err, tc.errStr)

			// The expression itself is well-formed, so the insane
			// parse has to accept it, and its own sanity check has
			// to report the same problem.
			node, err := ParseInsane(tc.expr, tc.ctx)
			require.NoError(t, err)
			require.ErrorContains(t, node.IsSane(), tc.errStr)
		})
	}
}

// TestParseAcceptsSane checks that the sanity checks Parse now runs do not
// reject expressions that are safe to use, including ones that sit exactly at a
// resource limit.
func TestParseAcceptsSane(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		expr string
		ctx  Context
	}{{
		name: "single key",
		expr: "pk(A)",
		ctx:  P2WSH,
	}, {
		name: "timelocked multisig",
		expr: "and_v(v:multi(2,A,B,C),older(144))",
		ctx:  P2WSH,
	}, {
		name: "time locks of the same kind",
		expr: "and_v(v:pk(A),and_v(v:older(10),older(20)))",
		ctx:  P2WSH,
	}, {
		// One hash fragment below the op count limit.
		name: "at the P2WSH op count limit",
		expr: hashChain(50),
		ctx:  P2WSH,
	}, {
		name: "tapscript multi_a",
		expr: "and_v(v:multi_a(2,A,B,C),older(144))",
		ctx:  P2TR,
	}, {
		// Only a repeated key is rejected; distinct keys in the same
		// positions are what every policy looks like.
		name: "distinct keys everywhere",
		expr: "or_d(pk(A),and_v(v:pkh(B),multi(2,C,D,E)))",
		ctx:  P2WSH,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			node, err := Parse(tc.expr, tc.ctx)
			require.NoError(t, err)
			require.NoError(t, node.IsSane())
		})
	}
}
