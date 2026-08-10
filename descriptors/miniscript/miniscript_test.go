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
