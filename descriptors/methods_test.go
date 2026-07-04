package descriptors

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testXpub1 = "[e81a5744/48'/0'/0'/2']xpub6Duv8Gj9gZeA3sUo5nUMPEv6" +
		"FZ81GHn3feyaUej5KqcjPKsYLww4xBX4MmYZUPX5NqzaVJWYdYZwGLECtg" +
		"QruG4FkZMh566RkfUT2pbzsEg/<0;1>/*"
	testXpub2 = "[3c157b79/48'/0'/0'/2']xpub6DdSN9RNZi3eDjhZWA8PJ5mS" +
		"uWgfmPdBduXWzSP91Y3GxKWNwkjyc5mF9FcpTFymUh9C4Bar45b6rWv6Y5" +
		"kSbi9yJDjuJUDzQSWUh3ijzXP/<0;1>/*"

	testTr = "tr(" + testXpub1 + ",and_v(v:pk(" + testXpub2 +
		"),older(65535)))#lg9nqqhr"
)

// TestKeys checks that Keys returns the descriptor's keys in order.
func TestKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc     string
		expected []string
	}{{
		desc:     testTr,
		expected: []string{testXpub1, testXpub2},
	}, {
		desc: "wpkh(xpub6BzikmgQmvoYG3ShFhXU1LFKaUeU832dHoYL6ka9JpC" +
			"qKXr7PTHQHaoSMbGU36CZNcoryVPsFBjt9aYyCQHtYi6BQTo6VfR" +
			"v9xVRuSNNteB)",
		expected: []string{
			"xpub6BzikmgQmvoYG3ShFhXU1LFKaUeU832dHoYL6ka9JpCqKXr7" +
				"PTHQHaoSMbGU36CZNcoryVPsFBjt9aYyCQHtYi6BQTo6" +
				"VfRv9xVRuSNNteB",
		},
	}}

	for _, test := range tests {
		descriptor, err := NewDescriptor(test.desc)
		require.NoError(t, err)
		require.Equal(t, test.expected, descriptor.Keys())
	}
}

// TestDescType checks the descriptor type classification.
func TestDescType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc     string
		expected DescType
	}{{
		desc:     testTr,
		expected: DescTypeTr,
	}, {
		desc: "wpkh(xpub6BzikmgQmvoYG3ShFhXU1LFKaUeU832dHoYL6ka9JpC" +
			"qKXr7PTHQHaoSMbGU36CZNcoryVPsFBjt9aYyCQHtYi6BQTo6VfR" +
			"v9xVRuSNNteB)",
		expected: DescTypeWpkh,
	}}

	for _, test := range tests {
		descriptor, err := NewDescriptor(test.desc)
		require.NoError(t, err)
		require.Equal(t, test.expected, descriptor.DescType())
	}
}

// TestMaxWeightToSatisfy checks the satisfaction weight bound, including the
// impossible-to-satisfy error case, against the descriptors-go reference
// values.
func TestMaxWeightToSatisfy(t *testing.T) {
	t.Parallel()

	descriptor, err := NewDescriptor(
		"wpkh(xpub6BzikmgQmvoYG3ShFhXU1LFKaUeU832dHoYL6ka9JpCqKXr7PTH" +
			"QHaoSMbGU36CZNcoryVPsFBjt9aYyCQHtYi6BQTo6VfRv9xVRuSN" +
			"NteB/*)",
	)
	require.NoError(t, err)
	weight, err := descriptor.MaxWeightToSatisfy()
	require.NoError(t, err)
	require.Equal(t, uint64(107), weight)

	// A descriptor whose script is a bare OP_FALSE can never be satisfied.
	descriptor, err = NewDescriptor("wsh(0)")
	require.NoError(t, err)
	_, err = descriptor.MaxWeightToSatisfy()
	require.Error(t, err)
}

// TestScriptCodeAt checks the script code derived for a P2WSH sorted-multisig.
func TestScriptCodeAt(t *testing.T) {
	t.Parallel()

	descriptor, err := NewDescriptor(
		"wsh(sortedmulti(2," + testXpub1 + "," + testXpub2 +
			"))#jx2cv4q8",
	)
	require.NoError(t, err)

	expected := "5221020b44e43e2f276697d23c2248f80bb09e84f702ddae399d" +
		"194f5132f472bf8713210326547ceb5352bd238ca7e1da004e9d6625ba" +
		"f3324feda4ead69436042a53510452ae"
	script, err := descriptor.ScriptCodeAt(0, 0)
	require.NoError(t, err)
	require.Equal(t, expected, hex.EncodeToString(script))
}
