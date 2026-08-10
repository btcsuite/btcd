package descriptors

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPlanConcurrentCompletion checks retry and ownership guarantees when one
// immutable plan is completed with separately owned satisfiers concurrently.
func TestPlanConcurrentCompletion(t *testing.T) {
	t.Parallel()
	for _, wrapper := range []string{"wsh(%s)", "sh(wsh(%s))", "sh(%s)"} {
		t.Run(wrapper, func(t *testing.T) {
			// A threshold requires several callback results and
			// literal witness elements. Changing signature lengths
			// on completion must not change which two keys the plan
			// selected.
			keys := testCompressedKeys(3)
			expression := fmt.Sprintf("multi(2,%s,%s,%s)", keys[0], keys[1], keys[2])
			d, err := NewDescriptor(fmt.Sprintf(
				wrapper, expression,
			))
			require.NoError(t, err)
			plan, err := d.PlanAt(0, 0, Assets{
				LookupEcdsaSig: func(key string) bool {
					return key == keys[0] || key == keys[2]
				},
			})
			require.NoError(t, err)
			makeSigner := func(signature []byte,
				extra bool) *Satisfier {

				return &Satisfier{
					LookupEcdsaSig: func(
						key string) ([]byte, bool) {

						// A newly available, cheaper
						// signature must not replace
						// either originally selected
						// key.
						if key == keys[1] && extra {
							return bytes.Repeat(
								[]byte{
									8,
								}, 65,
							), true
						}
						return signature, key == keys[0] || key == keys[2]
					},
				}
			}
			want, err := plan.Satisfy(makeSigner(
				bytes.Repeat([]byte{
					7,
				}, 71), false,
			))
			require.NoError(t, err)

			// A failed completion must not consume or partially
			// replace the template. Callback buffers must not alias
			// its outputs.
			_, err = plan.Satisfy(&Satisfier{})
			require.Error(t, err)
			_, err = plan.Satisfy(&Satisfier{
				LookupEcdsaSig: func(key string) ([]byte,
					bool) {

					return bytes.Repeat([]byte{
						9,
					}, 71), key == keys[0]
				},
			})
			require.Error(t, err)
			buffer := bytes.Repeat([]byte{7}, 71)
			got, err := plan.Satisfy(makeSigner(buffer, false))
			require.NoError(t, err)
			clear(buffer)
			require.Equal(t, want, got)

			// Workers own both their callback buffers and result
			// slots. Assertions stay in the test goroutine, after
			// synchronization.
			const workers = 16
			results := make([]*SatisfyResult, workers)
			errors := make([]error, workers)
			var wg sync.WaitGroup
			for i := range workers {
				wg.Go(func() {
					results[i], errors[i] = plan.Satisfy(
						makeSigner(bytes.Repeat([]byte{
							7,
						}, 71), true),
					)
				})
			}
			wg.Wait()
			for i := range workers {
				require.NoError(t, errors[i])
				require.Equal(t, want, results[i])
			}
		})
	}
}
