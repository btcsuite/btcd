package miniscript

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// resourceThreshold constructs a half-of-n threshold with distinct symbolic
// keys. Half is the worst case for algorithms that enumerate signer subsets.
func resourceThreshold(n int) string {
	var expression strings.Builder
	fmt.Fprintf(&expression, "thresh(%d,pk(K0)", n/2)
	for i := 1; i < n; i++ {
		fmt.Fprintf(&expression, ",s:pk(K%d)", i)
	}
	expression.WriteByte(')')
	return expression.String()
}

// TestThresholdResourceSeries checks deterministic output bounds as thresholds
// grow, without imposing machine-dependent timing or allocation-count limits.
func TestThresholdResourceSeries(t *testing.T) {
	t.Parallel()
	for _, n := range []int{8, 16, 32, 64, 128} {
		// Taproot permits these widths without the legacy opcode limit.
		// Dummy signatures isolate assembly and sizing from
		// cryptography, which the independent spending tests exercise
		// separately.
		node, err := Parse(resourceThreshold(n), P2TR)
		require.NoError(t, err)
		require.NoError(
			t, node.ApplyVars(func(name string) ([]byte, error) {
				return testKey(name), nil
			}),
		)
		script, err := node.Script()
		require.NoError(t, err)
		require.Equal(t, len(script), node.ScriptLen())
		witness, err := node.Satisfy(&Satisfier{
			Sign: func([]byte) ([]byte, bool) {
				return bytes.Repeat([]byte{1}, 64), true
			},
		})
		require.NoError(t, err)
		require.Len(t, witness, n)
		require.Equal(t, n, node.maxWitnessSize())

		// Exactly half the items must be signatures, and the others
		// canonical empty dissatisfactions. Extra available signatures
		// must not inflate a threshold satisfaction to n signatures.
		signatures := 0
		for _, item := range witness {
			if len(item) != 0 {
				require.Len(t, item, 64)
				signatures++
			}
		}
		require.Equal(t, n/2, signatures)
		bound, err := node.MaxSatisfactionSize()
		require.NoError(t, err)
		require.LessOrEqual(t, witnessSize(witness), bound)
	}
}

// BenchmarkResourceParse reports time and bytes allocated across increasing
// threshold width, nesting depth and late syntax errors. Compare size series
// with benchstat; a wall-clock pass/fail budget would be host dependent.
func BenchmarkResourceParse(b *testing.B) {
	for _, n := range []int{8, 16, 32, 64, 128} {
		for _, kind := range []string{"wide", "deep", "malformed"} {
			expression := resourceThreshold(n)
			switch kind {
			case "deep":
				expression = nestedWrappers(n)

			case "malformed":
				expression = strings.TrimSuffix(expression, ")")
			}
			b.Run(
				fmt.Sprintf("%s/%d", kind, n),
				func(b *testing.B) {
					// Validate the scenario outside the
					// timed loop so an unexpectedly
					// rejected valid input cannot look
					// fast.
					_, err := Parse(expression, P2TR)
					require.Equal(
						b, kind == "malformed",
						err != nil,
					)
					b.ReportAllocs()
					b.ResetTimer()
					for range b.N {
						_, err := Parse(
							expression, P2TR,
						)
						if (err != nil) != (kind == "malformed") {
							require.FailNow(
								b,
								"parse outcome changed",
							)
						}
					}
				},
			)
		}
	}
}
