package miniscript

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSplitString tests the splitString function.
func TestSplitString(t *testing.T) {
	separators := func(c rune) bool {
		return c == '(' || c == ')' || c == ','
	}

	testCases := []struct {
		str      string
		expected []string
	}{
		{
			str:      "",
			expected: []string{},
		},
		{
			str:      "0",
			expected: []string{"0"},
		},
		{
			str:      "0)(1(",
			expected: []string{"0", ")", "(", "1", "("},
		},
		{
			str: "or_b(pk(key_1),s:pk(key_2))",
			expected: []string{
				"or_b", "(", "pk", "(", "key_1", ")", ",",
				"s:pk", "(", "key_2", ")", ")",
			},
		},
	}

	for _, tc := range testCases {
		require.Equal(t, tc.expected, splitString(tc.str, separators))
	}
}

// TestComputeOpCount tests that the maxOpCount function returns the correct
// number of operations.
func TestComputeOpCount(t *testing.T) {
	testCases := []struct {
		script     string
		maxOpCount int
	}{
		{
			script: "or_i(multi(2,key1,key2,key3)," +
				"multi(3,key4,key5,key6,key7))",
			maxOpCount: 9,
		},
		{
			script: "thresh(2,or_i(multi(2,key1,key2,key3)," +
				"multi(3,key4,key5,key6,key7))," +
				"s:pk(key8),s:pk(key9))",
			maxOpCount: 16,
		},
		{
			script: "thresh(2,or_d(multi(2,key1,key2,key3)," +
				"multi(3,key4,key5,key6,key7))," +
				"s:pk(key8),s:pk(key9))",
			maxOpCount: 19,
		},
	}

	for _, tc := range testCases {
		node, err := ParseInsane(tc.script, P2WSH)
		require.NoError(t, err)
		require.Equal(t, tc.maxOpCount, node.maxOpCount())
	}
}
