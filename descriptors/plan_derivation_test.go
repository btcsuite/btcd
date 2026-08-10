package descriptors

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPlanTaprootInvalidDerivation rejects key-path plans whose internal key
// cannot be derived, even when the signature provider advertises availability.
func TestPlanTaprootInvalidDerivation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		path  string
		index uint32
	}{
		{name: "index high bit", path: "/*", index: 1 << 31},
		{name: "maximum index", path: "/*", index: ^uint32(0)},
		{name: "hardened step", path: "/0'/*"},
		{name: "hardened wildcard", path: "/*'"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d, err := NewDescriptor(
				"tr(" + basicTestXpub + test.path + ")",
			)
			require.NoError(t, err)
			_, err = d.PlanAt(0, test.index, Assets{
				LookupTapKeySpendSig: func(string) (uint32,
					bool) {

					return 64, true
				},
			})
			require.Error(t, err)
		})
	}
}
