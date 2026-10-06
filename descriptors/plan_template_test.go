package descriptors

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPlanKeepsChosenPath verifies that a completed plan never selects a new
// branch or key subset, and owns its literal bytes independently of callers.
func TestPlanKeepsChosenPath(t *testing.T) {
	t.Parallel()
	keys := testCompressedKeys(3)
	for _, kind := range []string{"sh", "wsh", "sh-wsh", "tr"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			a, b := keys[0], keys[1]
			if kind == "tr" {
				a, b = a[2:], b[2:]
			}
			expression := fmt.Sprintf("or_d(pk(%s),pk(%s))", a, b)
			switch kind {
			case "sh", "wsh":
				expression = kind + "(" + expression + ")"

			case "sh-wsh":
				expression = "sh(wsh(" + expression + "))"

			case "tr":
				expression = "tr(" + keys[2][2:] + "," +
					expression + ")"
			}
			d, err := NewDescriptor(expression)
			require.NoError(t, err)
			available := a
			plan, err := d.PlanAt(0, 0, Assets{
				LookupEcdsaSig: func(key string) bool {
					return key == available
				},
				LookupTapLeafScriptSig: func(
					key, _ string) (uint32, bool) {

					return 64, key == available
				},
			})
			require.NoError(t, err)

			// Changing the caller's asset lookup after planning
			// must not affect the plan or trigger another asset
			// query.
			available = b
			signer := func(key string, extra bool) *Satisfier {
				return &Satisfier{
					LookupEcdsaSig: func(
						request string) ([]byte, bool) {

						return bytes.Repeat([]byte{
							7,
						}, 72), request == key || extra
					},
					LookupTapLeafScriptSig: func(
						request, _ string) ([]byte,
						bool) {

						return bytes.Repeat([]byte{
							7,
						}, 64), request == key || extra
					},
				}
			}
			want, err := plan.Satisfy(signer(a, false))
			require.NoError(t, err)
			_, err = plan.Satisfy(signer(b, false))
			require.ErrorIs(t, err, errCouldNotSatisfy)
			got, err := plan.Satisfy(signer(a, true))
			require.NoError(t, err)
			require.Equal(t, want, got)
			for _, item := range got.Witness {
				for i := range item {
					item[i] ^= 0xff
				}
			}
			for i := range got.ScriptSig {
				got.ScriptSig[i] ^= 0xff
			}
			again, err := plan.Satisfy(signer(a, false))
			require.NoError(t, err)
			require.Equal(t, want, again)
		})
	}
}
