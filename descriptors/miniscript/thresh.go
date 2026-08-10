package miniscript

import (
	"cmp"
	"slices"
)

// threshSelection describes the ways a thresh(k, X1, ..., Xn) fragment can be
// satisfied, without materializing them: a satisfaction satisfies exactly k of
// the n sub expressions and dissatisfies the remaining ones.
//
// A sub expression that cannot be dissatisfied has to be satisfied by every
// selection, one that cannot be satisfied has to be dissatisfied by every
// selection, and the remaining ones are free to go either way. That is enough
// to answer the two questions the analysis passes ask - the maximum sum of the
// per-sub-expression costs (op count, witness element count) and whether a
// given sub expression can be satisfied or dissatisfied at all (execution stack
// peak) - in O(n log n) time and O(n) space instead of enumerating all C(n, k)
// selections, which grows exponentially and made Parse abort the process with
// an out-of-memory error for n as small as 28.
type threshSelection struct {
	// k is the number of sub expressions a satisfaction has to satisfy.
	k int

	// sat and dsat hold the satisfaction and dissatisfaction value of each
	// sub expression, in order.
	sat, dsat []maxInt

	// numMandatory is the number of sub expressions that cannot be
	// dissatisfied, and which every selection therefore has to satisfy.
	numMandatory int

	// numFree is the number of sub expressions that can be both satisfied
	// and dissatisfied.
	numFree int

	// possible is false if there is no valid selection at all, i.e. the
	// thresh cannot be satisfied.
	possible bool
}

// newThreshSelection classifies the sub expressions of a thresh(k, X1, ..., Xn)
// fragment by their satisfaction and dissatisfaction validity. sat and dsat
// hold the values of the sub expressions in order and must have the same
// length.
func newThreshSelection(k int, sat, dsat []maxInt) threshSelection {
	s := threshSelection{k: k, sat: sat, dsat: dsat, possible: true}
	for i := range sat {
		switch {
		case !sat[i].valid && !dsat[i].valid:
			// The sub expression can neither be satisfied nor
			// dissatisfied, so no selection is valid.
			s.possible = false

		case !dsat[i].valid:
			// Cannot be dissatisfied, so it has to be satisfied.
			s.numMandatory++

		case sat[i].valid:
			// Both choices exist, so this child can fill any
			// remaining satisfaction slot after the mandatory
			// children.
			s.numFree++
		}
	}

	// The sub expressions that have to be satisfied must not exceed k, and
	// the free ones have to be able to make up the difference.
	if s.numMandatory > k || k-s.numMandatory > s.numFree {
		s.possible = false
	}

	return s
}

// maxSum returns the maximum, over every valid selection, of the summed
// satisfaction and dissatisfaction values of the sub expressions, or an invalid
// value if there is no valid selection.
//
// The sum is separable: starting from "dissatisfy every sub expression that can
// be dissatisfied", satisfying a free sub expression instead changes the total
// by sat-dsat, independently of the other choices. The maximum is therefore
// reached by satisfying the mandatory sub expressions plus the free ones with
// the largest differences, which only needs a sort.
func (s threshSelection) maxSum() maxInt {
	if !s.possible {
		return maxInt{}
	}

	total := 0
	deltas := make([]int, 0, s.numFree)
	for i := range s.sat {
		switch {
		case !s.dsat[i].valid:
			// Mandatory: satisfied by every selection.
			total += s.sat[i].value

		case !s.sat[i].valid:
			// Forbidden: dissatisfied by every selection.
			total += s.dsat[i].value

		default:
			// Free: dissatisfied unless picked below.
			total += s.dsat[i].value
			deltas = append(deltas, s.sat[i].value-s.dsat[i].value)
		}
	}

	// Satisfy the free sub expressions that increase the total the most,
	// until exactly k sub expressions are satisfied. Note that a negative
	// difference is still picked if it is needed to reach k, since every
	// selection satisfies exactly k sub expressions.
	slices.SortFunc(deltas, func(a, b int) int {
		return cmp.Compare(b, a)
	})
	for _, delta := range deltas[:s.k-s.numMandatory] {
		total += delta
	}

	return maxInt{valid: true, value: total}
}

// canSatisfy returns whether some valid selection satisfies the sub expression
// at index i.
func (s threshSelection) canSatisfy(i int) bool {
	if !s.possible || !s.sat[i].valid {
		return false
	}

	// A mandatory sub expression is satisfied by every selection.
	if !s.dsat[i].valid {
		return true
	}

	// A free sub expression can be satisfied as long as the mandatory ones
	// do not already account for all k satisfactions. The other k-1
	// satisfactions can always be assigned, because the selection is
	// possible at all.
	return s.numMandatory < s.k
}

// canDissatisfy returns whether some valid selection dissatisfies the sub
// expression at index i.
func (s threshSelection) canDissatisfy(i int) bool {
	if !s.possible || !s.dsat[i].valid {
		return false
	}

	// A forbidden sub expression is dissatisfied by every selection.
	if !s.sat[i].valid {
		return true
	}

	// A free sub expression can be dissatisfied if the satisfactions that
	// are still to be assigned fit in the remaining free sub expressions.
	return s.k-s.numMandatory <= s.numFree-1
}
