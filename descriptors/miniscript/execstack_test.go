package miniscript

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSeqExec checks seqExec, which combines the execution-stack peaks of two
// sub expressions running in sequence. The peak is the max of the two, but
// when the first leaves its result on the stack while the second runs
// (keepFirst), the second's peak counts one extra element. The result is
// invalid if either operand is.
func TestSeqExec(t *testing.T) {
	t.Parallel()

	valid := func(v int) maxInt {
		return maxInt{valid: true, value: v}
	}
	invalid := maxInt{}

	tests := []struct {
		name      string
		a, b      maxInt
		keepFirst bool
		want      maxInt
	}{{
		name:      "max of both, second larger",
		a:         valid(3),
		b:         valid(5),
		keepFirst: false,
		want:      valid(5),
	}, {
		name:      "max of both, first larger",
		a:         valid(8),
		b:         valid(5),
		keepFirst: false,
		want:      valid(8),
	}, {
		name:      "keepFirst bumps the second",
		a:         valid(3),
		b:         valid(5),
		keepFirst: true,
		want:      valid(6),
	}, {
		name:      "keepFirst but first still dominates",
		a:         valid(10),
		b:         valid(5),
		keepFirst: true,
		want:      valid(10),
	}, {
		name:      "left invalid",
		a:         invalid,
		b:         valid(5),
		keepFirst: true,
		want:      invalid,
	}, {
		name:      "right invalid",
		a:         valid(3),
		b:         invalid,
		keepFirst: false,
		want:      invalid,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(
				t, tc.want, seqExec(tc.a, tc.b, tc.keepFirst),
			)
		})
	}
}
