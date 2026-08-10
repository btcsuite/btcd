package miniscript

import (
	"math/rand"
	"slices"
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
			// Each pending branch owns its indices, so extending
			// one branch cannot overwrite a sibling's backing
			// array.
			newSubset := slices.Clone(current.subset)
			newSubset = append(newSubset, i)
			stack = append(stack, stackItem{
				subset: newSubset,
				start:  i + 1,
			})
		}
	}

	return subsets
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

// bruteForceMaxSum computes the maximum summed cost over all ways of satisfying
// exactly k of the sub expressions by enumerating every subset, mirroring what
// the analysis passes did before threshSelection replaced the enumeration.
func bruteForceMaxSum(k int, sat, dsat []maxInt) maxInt {
	result := maxInt{}
	for _, subset := range subsets(len(sat), k) {
		candidate := maxInt{valid: true}
		for i := range sat {
			value := dsat[i]
			if slices.Contains(subset, i) {
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
			if slices.Contains(subset, i) && !sat[i].valid {
				valid = false
			}
			if !slices.Contains(subset, i) && !dsat[i].valid {
				valid = false
			}
		}
		if !valid {
			continue
		}

		for i := range sat {
			if slices.Contains(subset, i) {
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
			for range 300 {
				sat := make([]maxInt, n)
				dsat := make([]maxInt, n)
				for i := range n {
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
				for i := range n {
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
