package descriptors

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWitnessV0Script checks that a version-0 witness program is wrapped as
// OP_0 followed by the pushed program, for both the 20-byte (P2WPKH) and
// 32-byte (P2WSH) program sizes.
func TestWitnessV0Script(t *testing.T) {
	t.Parallel()

	program20 := bytes.Repeat([]byte{0xab}, 20)
	script, err := witnessV0Script(program20)
	require.NoError(t, err)
	require.Equal(t, append([]byte{0x00, 0x14}, program20...), script)

	program32 := bytes.Repeat([]byte{0xcd}, 32)
	script, err = witnessV0Script(program32)
	require.NoError(t, err)
	require.Equal(t, append([]byte{0x00, 0x20}, program32...), script)
}
