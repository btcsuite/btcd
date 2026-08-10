package miniscript

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chainhash/v2"
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

// checkMiniscript makes sure the passed miniscript is top level, has the
// expected type and script length.
func checkMiniscript(miniscript, expectedType string, opCodes int) error {
	// The corpora include expressions that are valid but not sane
	// (malleable ones, and ones that mix time lock kinds), so the sanity
	// checks that Parse runs are not wanted here.
	node, err := ParseInsane(miniscript, P2WSH)
	if err != nil {
		return err
	}
	if err := node.IsValidTopLevel(); err != nil {
		return err
	}

	// Property letters are a set, so compare canonical orders without
	// requiring the upstream corpus to use our diagnostic presentation.
	sortString := func(s string) string {
		r := []rune(s)
		slices.Sort(r)

		return string(r)
	}
	if sortString(expectedType) != sortString(node.formattedType()) {
		return fmt.Errorf("expected type %s, got %s",
			sortString(expectedType),
			sortString(node.formattedType()))
	}

	err = node.ApplyVars(func(identifier string) ([]byte, error) {
		if len(identifier) == 64 {
			return nil, nil
		}

		// Return an arbitrary unique 33 bytes.
		return append(
			chainhash.HashB([]byte(identifier)), 0,
		), nil
	})
	if err != nil {
		return err
	}

	script, err := node.Script()
	if err != nil {
		return err
	}

	if len(script) != node.scriptLen {
		return fmt.Errorf("expected script length %d but got %d for "+
			"script %s", node.scriptLen, len(script),
			node.DrawTree())
	}

	if opCodes != 0 && opCodes != node.maxOpCount() {
		return fmt.Errorf("expected %d opcodes but got %d for "+
			"miniscript %s", opCodes, node.maxOpCount(),
			miniscript)
	}

	return nil
}

// TestVectors asserts all test vectors in the test data text files pass.
func TestVectors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		fileName    string
		valid       bool
		withOpCodes bool
	}{
		{
			// Invalid expressions (failed type check).
			fileName: "testdata/invalid.txt",
			valid:    false,
		},
		{
			// Valid miniscript expressions including the expected
			// type.
			fileName: "testdata/valid_8f1e8_from_alloy.txt",
			valid:    true,
		},
		{
			// Valid miniscript expressions including the expected
			// type.
			fileName: "testdata/valid_from_alloy.txt",
			valid:    true,
		},
		{
			// Valid expressions but do not contain the `m` type
			// property: non-malleable satisfaction is not
			// guaranteed.
			fileName: "testdata/malleable_from_alloy.txt",
			valid:    true,
		},
		{
			// miniscripts with time lock mixing in `after` (same
			// expression contains both time-based and block-based
			// time locks). This unit test is not testing this
			// currently, see
			// https://github.com/rust-bitcoin/rust-miniscript/issues/514.
			fileName: "testdata/conflict_from_alloy.txt",
			valid:    true,
		},
		{
			// miniscripts with number of opcodes.
			fileName:    "testdata/opcodes.txt",
			valid:       true,
			withOpCodes: true,
		},
	}

	for _, tc := range testCases {
		content, err := os.ReadFile(tc.fileName)
		require.NoError(t, err)

		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if line == "" {
				continue
			}

			if !tc.valid {
				_, err := ParseInsane(line, P2WSH)
				require.Errorf(
					t, err, "failure on line %d: %s", i,
					line,
				)

				continue
			}

			parts := strings.Split(line, " ")

			var opCodes int
			if tc.withOpCodes {
				require.Lenf(
					t, parts, 3, "malformed test on line "+
						"%d: %s", i, line,
				)
				opCodes, err = strconv.Atoi(parts[2])
				require.NoError(t, err)
			} else {
				require.Lenf(
					t, parts, 2, "malformed test on line "+
						"%d: %s", i, line,
				)
			}

			miniscript, expectedType := parts[0], parts[1]
			require.NoError(
				t, checkMiniscript(
					miniscript, expectedType, opCodes,
				), "failure on line %d: %s", i, line,
			)
		}
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
