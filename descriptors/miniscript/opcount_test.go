package miniscript

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMaxInt checks the and/or algebra of the maxInt helper used by the
// op-count, stack-size and execution-stack passes. "and" sums two valid values
// (and is invalid if either operand is), while "or" keeps the larger valid
// value and ignores an invalid operand.
func TestMaxInt(t *testing.T) {
	t.Parallel()

	valid := func(v int) maxInt {
		return maxInt{valid: true, value: v}
	}
	invalid := maxInt{}

	tests := []struct {
		name    string
		a, b    maxInt
		wantAnd maxInt
		wantOr  maxInt
	}{{
		name:    "both valid, or picks larger right",
		a:       valid(3),
		b:       valid(5),
		wantAnd: valid(8),
		wantOr:  valid(5),
	}, {
		name:    "both valid, or picks larger left",
		a:       valid(9),
		b:       valid(2),
		wantAnd: valid(11),
		wantOr:  valid(9),
	}, {
		name:    "equal values",
		a:       valid(4),
		b:       valid(4),
		wantAnd: valid(8),
		wantOr:  valid(4),
	}, {
		name:    "left invalid",
		a:       invalid,
		b:       valid(4),
		wantAnd: invalid,
		wantOr:  valid(4),
	}, {
		name:    "right invalid",
		a:       valid(7),
		b:       invalid,
		wantAnd: invalid,
		wantOr:  valid(7),
	}, {
		name:    "both invalid",
		a:       invalid,
		b:       invalid,
		wantAnd: invalid,
		wantOr:  invalid,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.wantAnd, tc.a.and(tc.b))
			require.Equal(t, tc.wantOr, tc.a.or(tc.b))
		})
	}
}
