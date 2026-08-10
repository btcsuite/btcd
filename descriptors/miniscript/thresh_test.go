package miniscript

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// subsets returns all subsets of the set {0, ..., n-1} of length k.
//
// This is the brute-force oracle the threshSelection implementation is checked
// against below. It used to be part of the threshold analysis passes, but
// materializing all C(n, k) subsets needs exponential space, which made Parse
// abort the process with an out-of-memory error for inputs as small as two
// kilobytes, so the passes now use threshSelection instead.
// - Written by ChatGPT.
func subsets(n int, k int) [][]int {
	type stackItem struct {
		subset []int
		start  int
	}

	var subsets [][]int
	stack := []stackItem{{
		subset: []int{},
		start:  0,
	}}

	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if len(current.subset) == k {
			subsets = append(subsets, current.subset)
			continue
		}

		for i := current.start; i < n; i++ {
			newSubset := append([]int{}, current.subset...)
			newSubset = append(newSubset, i)
			stack = append(stack, stackItem{
				subset: newSubset,
				start:  i + 1,
			})
		}
	}

	return subsets
}

// containsInt returns whether the given integer is part of the slice.
func containsInt(ints []int, i int) bool {
	for _, el := range ints {
		if el == i {
			return true
		}
	}
	return false
}

// TestSubsets checks the brute-force subset enumeration used as the oracle for
// the threshold selection: it yields every strictly-increasing subset of
// {0,...,n-1} of length k, with no duplicates.
func TestSubsets(t *testing.T) {
	t.Parallel()

	// Sizes: choosing k from n yields C(n, k) subsets.
	require.Len(t, subsets(4, 2), 6)
	require.Len(t, subsets(5, 0), 1)
	require.Len(t, subsets(3, 3), 1)
	require.Empty(t, subsets(2, 3))

	// The single length-0 subset is empty.
	require.Empty(t, subsets(5, 0)[0])

	// Every emitted subset has length k, holds strictly increasing indices
	// in range, and is unique.
	seen := make(map[string]bool)
	for _, subset := range subsets(4, 2) {
		require.Len(t, subset, 2)
		for i, idx := range subset {
			require.GreaterOrEqual(t, idx, 0)
			require.Less(t, idx, 4)
			if i > 0 {
				require.Greater(t, idx, subset[i-1])
			}
		}
		key := containsIntKey(subset)
		require.False(t, seen[key], "duplicate subset")
		seen[key] = true
	}
}

// containsIntKey renders a subset as a stable map key for the uniqueness check.
func containsIntKey(subset []int) string {
	key := ""
	for _, idx := range subset {
		key += string(rune('a' + idx))
	}
	return key
}

// TestContainsInt checks the integer membership helper.
func TestContainsInt(t *testing.T) {
	t.Parallel()

	require.True(t, containsInt([]int{1, 3, 5}, 3))
	require.False(t, containsInt([]int{1, 3, 5}, 4))
	require.False(t, containsInt(nil, 0))
}

// bruteForceMaxSum computes the maximum summed cost over all ways of satisfying
// exactly k of the sub expressions by enumerating every subset, mirroring what
// the analysis passes did before threshSelection replaced the enumeration.
func bruteForceMaxSum(k int, sat, dsat []maxInt) maxInt {
	result := maxInt{}
	for _, subset := range subsets(len(sat), k) {
		candidate := maxInt{valid: true}
		for i := range sat {
			value := dsat[i]
			if containsInt(subset, i) {
				value = sat[i]
			}
			candidate = candidate.and(value)
		}
		result = result.or(candidate)
	}

	return result
}

// bruteForceReachable returns, for every sub expression, whether some subset of
// exactly k satisfied sub expressions satisfies resp. dissatisfies it, with all
// required (dis)satisfactions available.
func bruteForceReachable(k int, sat, dsat []maxInt) (canSat, canDsat []bool) {
	n := len(sat)
	canSat, canDsat = make([]bool, n), make([]bool, n)
	for _, subset := range subsets(n, k) {
		// Skip selections that require an unavailable
		// (dis)satisfaction.
		valid := true
		for i := range sat {
			if containsInt(subset, i) && !sat[i].valid {
				valid = false
			}
			if !containsInt(subset, i) && !dsat[i].valid {
				valid = false
			}
		}
		if !valid {
			continue
		}

		for i := range sat {
			if containsInt(subset, i) {
				canSat[i] = true
			} else {
				canDsat[i] = true
			}
		}
	}

	return canSat, canDsat
}

// TestThreshSelection checks the threshold selection against the brute-force
// enumeration it replaced, over randomized availability and cost patterns: the
// maximum summed cost and the per-sub-expression reachability must agree
// exactly, including the cases where no valid selection exists at all.
func TestThreshSelection(t *testing.T) {
	t.Parallel()

	// A fixed seed keeps the test deterministic while still covering a wide
	// range of availability patterns.
	rng := rand.New(rand.NewSource(42))

	for n := 1; n <= 7; n++ {
		for k := 1; k <= n; k++ {
			for round := 0; round < 300; round++ {
				sat := make([]maxInt, n)
				dsat := make([]maxInt, n)
				for i := 0; i < n; i++ {
					// Draw both availability and cost, so
					// that mandatory (no dissatisfaction),
					// forbidden (no satisfaction) and
					// impossible (neither) sub expressions
					// all occur.
					sat[i] = maxInt{
						valid: rng.Intn(4) != 0,
						value: rng.Intn(10) - 3,
					}
					dsat[i] = maxInt{
						valid: rng.Intn(4) != 0,
						value: rng.Intn(10) - 3,
					}
				}

				sel := newThreshSelection(k, sat, dsat)

				wantSum := bruteForceMaxSum(k, sat, dsat)
				require.Equal(
					t, wantSum, sel.maxSum(),
					"n=%d k=%d sat=%v dsat=%v", n, k, sat,
					dsat,
				)

				wantSat, wantDsat := bruteForceReachable(
					k, sat, dsat,
				)
				for i := 0; i < n; i++ {
					require.Equal(
						t, wantSat[i],
						sel.canSatisfy(i),
						"canSatisfy(%d) n=%d k=%d "+
							"sat=%v dsat=%v",
						i, n, k, sat, dsat,
					)
					require.Equal(
						t, wantDsat[i],
						sel.canDissatisfy(i),
						"canDissatisfy(%d) n=%d k=%d "+
							"sat=%v dsat=%v", i, n,
						k, sat, dsat,
					)
				}
			}
		}
	}
}

// TestThreshLargeParse is the regression test for the threshold analysis passes
// being exponential in space: a thresh with 28 sub expressions is a perfectly
// valid miniscript, about two kilobytes long with concrete keys and well under
// every script limit, but enumerating its C(28, 14) selections allocated around
// five gigabytes and aborted the process with a fatal out-of-memory error.
// Parsing it must simply work, and produce the same values as the brute-force
// oracle does for a size it can still handle.
func TestThreshLargeParse(t *testing.T) {
	t.Parallel()

	// buildThresh assembles thresh(k, pk(K1), s:pk(K2), ..., s:pk(Kn)). All
	// sub expressions but the first need the s: wrapper to be of type W.
	buildThresh := func(k, n int) string {
		var b strings.Builder
		fmt.Fprintf(&b, "thresh(%d,pk(key1)", k)
		for i := 2; i <= n; i++ {
			fmt.Fprintf(&b, ",s:pk(key%d)", i)
		}
		b.WriteByte(')')
		return b.String()
	}

	// The number of sub expressions that used to be fatal, and a threshold
	// of half of them, which maximizes the number of selections.
	const (
		n = 28
		k = 14
	)

	node, err := Parse(buildThresh(k, n), P2WSH)
	require.NoError(t, err)

	// Every sub expression needs exactly one witness element, whether it is
	// satisfied (a signature) or dissatisfied (an empty element), so the
	// satisfaction needs one element per sub expression.
	require.True(t, node.stackSize.sat.valid)
	require.Equal(t, n, node.stackSize.sat.value)
	require.True(t, node.stackSize.dsat.valid)
	require.Equal(t, n, node.stackSize.dsat.value)

	// None of the sub expressions is an OP_CHECKMULTISIG, so no additional
	// ops are needed to satisfy the script beyond the ones it contains.
	require.True(t, node.opCount.sat.valid)
	require.Equal(t, 0, node.opCount.sat.value)

	// The same expression at a size the brute-force oracle can still handle
	// must produce identical values, which ties the fast path back to the
	// enumeration for a real, parsed expression.
	small, err := Parse(buildThresh(6, 12), P2WSH)
	require.NoError(t, err)

	subs := small.args[1:]
	sat := make([]maxInt, len(subs))
	dsat := make([]maxInt, len(subs))
	for i, sub := range subs {
		sat[i], dsat[i] = sub.stackSize.sat, sub.stackSize.dsat
	}
	require.Equal(
		t, bruteForceMaxSum(6, sat, dsat), small.stackSize.sat,
	)

	for i, sub := range subs {
		sat[i], dsat[i] = sub.opCount.sat, sub.opCount.dsat
	}
	require.Equal(t, bruteForceMaxSum(6, sat, dsat), small.opCount.sat)
}
