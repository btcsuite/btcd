package descriptors

import (
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/stretchr/testify/require"
)

// TestDeepNestingRejected checks that the deeply nested descriptors from the
// report are rejected instead of exhausting the goroutine stack. parseNode
// recursed once per sh()/wsh() level with no bound, and Go's stack limit turns
// that into a fatal stack overflow that recover() cannot catch, so the process
// died rather than the request: 16 MB of nested sh( still parsed, 32 MB killed
// the process.
func TestDeepNestingRejected(t *testing.T) {
	t.Parallel()

	key := testCompressedKeys(1)[0]

	// nested wraps a pkh() in the given number of sh() or wsh() levels.
	nested := func(wrapper string, levels int) string {
		return strings.Repeat(wrapper+"(", levels) + "pkh(" + key +
			")" + strings.Repeat(")", levels)
	}

	tests := []struct {
		name   string
		desc   string
		errStr string
	}{{
		name:   "nested sh",
		desc:   nested("sh", 10_000),
		errStr: "sh() cannot be used inside an sh() descriptor",
	}, {
		name:   "nested wsh",
		desc:   nested("wsh", 10_000),
		errStr: "wsh() cannot be used inside a wsh() descriptor",
	}, {
		// A deeply nested tap tree is bounded by the tap tree depth
		// limit.
		name:   "nested tap tree",
		desc:   degenerateTapTree(10_000),
		errStr: "taproot script tree is deeper than",
	}, {
		// A deeply nested miniscript is bounded by the miniscript
		// package's nesting limit.
		name: "nested miniscript",
		desc: "wsh(" + strings.Repeat("n", 100_000) + ":pk(" + key +
			"))",
		errStr: "nesting depth",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewDescriptor(tc.desc)
			require.ErrorContains(t, err, tc.errStr)
		})
	}
}

// TestMaxDepthDescriptorUsable checks that the deepest descriptor that is
// accepted stays fully usable: every recursive walk of the parsed tree (address
// derivation, script code, weight estimation, planning and lifting) has to cope
// with it, since a bound that only holds during parsing would just move the
// crash to a later call.
func TestMaxDepthDescriptorUsable(t *testing.T) {
	t.Parallel()

	// A tap tree at the maximum depth is the deepest tree the package
	// accepts, and its leaves are the ones with the longest merkle path.
	d, err := NewDescriptor(degenerateTapTree(maxTapTreeDepth))
	require.NoError(t, err)

	_, err = d.AddressAt(&chaincfg.MainNetParams, 0, 0)
	require.NoError(t, err)

	_, err = d.MaxWeightToSatisfy()
	require.NoError(t, err)

	_, err = d.Lift()
	require.NoError(t, err)

	// Planning builds each leaf's merkle proof while walking the tree.
	plan, err := d.PlanAt(0, 0, Assets{
		LookupTapLeafScriptSig: func(pk, leafHash string) (uint32,
			bool) {

			return 64, true
		},
	})
	require.NoError(t, err)
	require.NotZero(t, plan.SatisfactionWeight())

	// The deepest sh()/wsh() nesting the position rules allow.
	shWsh, err := NewDescriptor(
		"sh(wsh(pk(" + testCompressedKeys(1)[0] + ")))",
	)
	require.NoError(t, err)

	_, err = shWsh.AddressAt(&chaincfg.MainNetParams, 0, 0)
	require.NoError(t, err)
}
