package miniscript

import (
	"math/rand"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSatInfo checks the union and concatenation of sets of execution traces.
// The union takes the larger netdiff and exec independently. Concatenation adds
// the netdiffs, and the peak is either reached in the second trace or in the
// first, in which case it is the second's netdiff higher. Either is empty if
// an operand is, except that a union with the empty set is the other set.
func TestSatInfo(t *testing.T) {
	t.Parallel()

	none := satInfo{}

	tests := []struct {
		name string
		got  satInfo
		want satInfo
	}{{
		name: "union takes the larger of each field",
		got:  trace(3, 1).or(trace(-1, 4)),
		want: trace(3, 4),
	}, {
		name: "union with the empty set",
		got:  none.or(trace(1, 2)),
		want: trace(1, 2),
	}, {
		name: "union of empty sets",
		got:  none.or(none),
		want: none,
	}, {
		// OP_1 OP_DROP: the peak is right after the push.
		name: "push then drop",
		got:  siPush.then(trace(1, 1)),
		want: trace(0, 1),
	}, {
		// OP_DROP OP_1: the peak is at the start and the end.
		name: "drop then push",
		got:  trace(1, 1).then(siPush),
		want: trace(0, 0),
	}, {
		name: "concatenation with the empty set",
		got:  siPush.then(none),
		want: none,
	}, {
		// <key> OP_CHECKSIG on top of a signature: the start has one
		// element, the peak two and the end one.
		name: "c:pk_k",
		got:  seq(siPush, siCheckSig),
		want: trace(0, 1),
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.got)
		})
	}
}

// bruteForceThreshExec computes the execSize of a thresh by enumerating every
// way of satisfying exactly k (for sat) or none (for dsat) of its sub
// expressions and concatenating their traces with the OP_ADDs between them and
// the final <k> OP_EQUAL.
func bruteForceThreshExec(k int, subs []execSize) execSize {
	run := func(satisfied []int) satInfo {
		result := siEmpty
		for i, sub := range subs {
			part := sub.dsat
			if slices.Contains(satisfied, i) {
				part = sub.sat
			}
			result = result.then(part)
			if i > 0 {
				result = result.then(siBinaryOp)
			}
		}

		return seq(result, siPush, siEqual)
	}

	sat := satInfo{}
	for _, subset := range subsets(len(subs), k) {
		sat = sat.or(run(subset))
	}

	return execSize{dsat: run(nil), sat: sat}
}

// TestThreshExecStack checks the dynamic program of threshExecStack against the
// brute-force enumeration of all selections, over randomized sub expression
// traces, including sub expressions that cannot be satisfied or dissatisfied.
func TestThreshExecStack(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(42))
	randomInfo := func() satInfo {
		if rng.Intn(4) == 0 {
			return satInfo{}
		}

		// Any trace reaches at least its start and end, so exec is at
		// least max(netdiff, 0).
		netdiff := rng.Intn(7) - 3
		return trace(netdiff, max(netdiff, 0)+rng.Intn(4))
	}

	for n := 1; n <= 7; n++ {
		for k := 1; k <= n; k++ {
			for range 200 {
				node := &AST{
					identifier: f_thresh,
					args:       []*AST{{num: uint64(k)}},
				}
				subs := make([]execSize, n)
				for i := range subs {
					subs[i] = execSize{
						dsat: randomInfo(),
						sat:  randomInfo(),
					}
					node.args = append(node.args, &AST{
						execStack: subs[i],
					})
				}

				require.Equalf(
					t, bruteForceThreshExec(k, subs),
					threshExecStack(node),
					"n=%d k=%d subs=%v", n, k, subs,
				)
			}
		}
	}
}
