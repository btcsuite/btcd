package descriptors

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// resourceTapTree constructs either balanced or maximally skewed trees with
// distinct leaves. The first key is internal, and all remaining keys are
// leaves.
func resourceTapTree(leaves int, skewed bool) string {
	keys := testCompressedKeys(leaves + 1)
	var tree func([]string) string
	tree = func(keys []string) string {
		if len(keys) == 1 {
			return "pk(" + keys[0][2:] + ")"
		}

		// Skewed trees stress proof depth; balanced trees stress the
		// number of candidates the planner must compare.
		split := len(keys) / 2
		if skewed {
			split = 1
		}
		return "{" + tree(keys[:split]) + "," + tree(keys[split:]) + "}"
	}
	return "tr(" + keys[0][2:] + "," + tree(keys[1:]) + ")"
}

// resourceTapAssets offers a correctly sized signature for every script-path
// candidate, forcing the planner to consider the entire tree, not a key spend.
func resourceTapAssets() Assets {
	return Assets{
		LookupTapLeafScriptSig: func(string, string) (uint32, bool) {
			return 64, true
		},
	}
}

// resourceTapSigner supplies dummy Schnorr-sized values for assembly
// benchmarks; script execution with real signatures is covered by spending
// properties.
func resourceTapSigner() *Satisfier {
	return &Satisfier{
		LookupTapLeafScriptSig: func(string, string) ([]byte, bool) {
			return bytes.Repeat([]byte{1}, 64), true
		},
	}
}

// TestTapTreeResourceSeries checks planning at increasing tree sizes and at the
// BIP341 proof-depth boundary. Sizing must include the selected control block.
func TestTapTreeResourceSeries(t *testing.T) {
	t.Parallel()
	for _, skewed := range []bool{false, true} {
		for _, leaves := range []int{8, 32, 129} {
			d, err := NewDescriptor(resourceTapTree(leaves, skewed))
			require.NoError(t, err)
			plan, err := d.PlanAt(0, 0, resourceTapAssets())
			require.NoError(t, err)
			result, err := plan.Satisfy(resourceTapSigner())
			require.NoError(t, err)
			require.Len(t, result.Witness, 3)
			require.Equal(
				t,
				uint64(wire.TxWitness(result.Witness).SerializeSize()),
				plan.WitnessSize(),
			)

			// Each leaf has the same signing cost. A skewed tree
			// must choose its depth-one leaf, whose proof has one
			// hash. This is an independent minimum-cost assertion,
			// not just a comparison of two production size
			// calculations.
			if skewed {
				require.Len(t, result.Witness[2], 33+32)
			}
		}
	}

	// One additional level cannot be represented by a valid control block.
	// Reject at construction, before callers can derive an unspendable
	// output.
	_, err := NewDescriptor(resourceTapTree(130, true))
	require.Error(t, err)
}

// BenchmarkPlanningResources separates parsing, path selection and completion
// over increasing balanced and skewed trees. Completion should depend on the
// selected path, not reconsider every leaf as the tree grows.
func BenchmarkPlanningResources(b *testing.B) {
	for _, skewed := range []bool{false, true} {
		for _, leaves := range []int{8, 32, 128} {
			expression := resourceTapTree(leaves, skewed)
			d, err := NewDescriptor(expression)
			require.NoError(b, err)
			assets, signer := resourceTapAssets(), resourceTapSigner()
			plan, err := d.PlanAt(0, 0, assets)
			require.NoError(b, err)
			for _, stage := range []string{
				"parse", "plan", "satisfy",
			} {

				b.Run(
					fmt.Sprintf("skewed=%v/%d/%s", skewed, leaves, stage),
					func(b *testing.B) {
						// Reuse setup objects so the
						// stages have distinct costs.
						// Check errors without timing
						// assertions on the normal
						// successful path.
						b.ReportAllocs()
						for range b.N {
							var err error
							switch stage {
							case "parse":
								_, err = NewDescriptor(
									expression,
								)

							case "plan":
								_, err = d.PlanAt(
									0, 0, assets,
								)

							case "satisfy":
								_, err = plan.Satisfy(
									signer,
								)
							}
							if err != nil {
								require.NoError(
									b, err,
								)
							}
						}
					},
				)
			}
		}
	}
}
