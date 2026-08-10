package descriptors

import (
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
