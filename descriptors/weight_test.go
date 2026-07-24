package descriptors

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVarintLen checks the Bitcoin variable-length-integer byte-length
// boundaries.
func TestVarintLen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		n    uint64
		want int
	}{
		{0, 1},
		{252, 1},
		{253, 3},
		{0xffff, 3},
		{0x10000, 5},
		{0xffffffff, 5},
		{0x100000000, 9},
	}

	for _, tc := range tests {
		require.Equalf(t, tc.want, varintLen(tc.n),
			"varintLen(%d)", tc.n)
	}
}

// TestPushOpcodeSize checks the byte count of the opcode that pushes a script
// of a given size, at the PUSHDATA boundaries.
func TestPushOpcodeSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		size int
		want int
	}{
		{0, 1},
		{75, 1},
		{76, 2},
		{255, 2},
		{256, 3},
		{0xffff, 3},
		{0x10000, 5},
	}

	for _, tc := range tests {
		require.Equalf(t, tc.want, pushOpcodeSize(tc.size),
			"pushOpcodeSize(%d)", tc.size)
	}
}

// TestWeightVB checks the virtual-byte to weight-unit conversion.
func TestWeightVB(t *testing.T) {
	t.Parallel()

	require.Equal(t, uint64(0), weightVB(0))
	require.Equal(t, uint64(4), weightVB(1))
	require.Equal(t, uint64(140), weightVB(35))
}

// TestMultiNumCost checks the byte cost of the two threshold pushes in a
// multisig script: one byte each while k and n fit in a minimal push, growing
// when either exceeds 16.
func TestMultiNumCost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		k, n int
		want int
	}{
		{1, 1, 2},
		{2, 3, 2},
		{16, 16, 2},
		{17, 16, 3},
		{2, 17, 3},
		{17, 17, 4},
	}

	for _, tc := range tests {
		require.Equalf(t, tc.want, multiNumCost(tc.k, tc.n),
			"multiNumCost(%d, %d)", tc.k, tc.n)
	}
}
