package descriptors

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
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

// TestLift checks that a descriptor lifts to the expected semantic policy,
// including the nested-threshold structure and JSON encoding, against the
// descriptors-go reference values.
func TestLift(t *testing.T) {
	t.Parallel()

	descriptor, err := NewDescriptor(testTr)
	require.NoError(t, err)

	policy, err := descriptor.Lift()
	require.NoError(t, err)

	threshold1 := uint(1)
	threshold2 := uint(2)
	key1 := testXpub1
	key2 := testXpub2
	lockTime := uint32(65535)
	require.Equal(t, &SemanticPolicy{
		Type:      SemanticPolicyTypeThresh,
		Threshold: &threshold1,
		Policies: []*SemanticPolicy{{
			Type: SemanticPolicyTypeKey,
			Key:  &key1,
		}, {
			Type:      SemanticPolicyTypeThresh,
			Threshold: &threshold2,
			Policies: []*SemanticPolicy{{
				Type: SemanticPolicyTypeKey,
				Key:  &key2,
			}, {
				Type:     SemanticPolicyTypeOlder,
				LockTime: &lockTime,
			}},
		}},
	}, policy)

	jsonPolicy, err := json.Marshal(policy)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"type": "thresh",
		"threshold": 1,
		"policies": [
			{"type": "key", "key": "`+testXpub1+`"},
			{
				"type": "thresh",
				"threshold": 2,
				"policies": [
					{"type": "key", "key": "`+testXpub2+`"},
					{"type": "older", "lockTime": 65535}
				]
			}
		]
	}`, string(jsonPolicy))
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

// TestBareScriptCode checks that a bare descriptor exposes its output script.
// Only pk() and pkh() used to be handled, so the bare multisigs of BIP383
// parsed and derived a satisfaction weight, but their script - the very thing
// the output pays to - could not be obtained at all.
func TestBareScriptCode(t *testing.T) {
	t.Parallel()

	const (
		key1 = "03a34b99f22c790c4e36b2b3c2c35a36db06226e41c692fc82b8b" +
			"56ac1c540c5bd"
		key2 = "0260b2003c386519fc9eadf2b5cf124dd8eea4c4e68d5e154050a" +
			"9346ea98ce600"
	)

	tests := []struct {
		name string
		desc string

		// wantScript is the output script of the bare descriptor, which
		// is also the script code its sighash is computed over.
		wantScript string
	}{{
		// The BIP383 test vector, with the keys in the order the
		// descriptor writes them.
		name:       "bare multi",
		desc:       "multi(1," + key1 + "," + key2 + ")",
		wantScript: "5121" + key1 + "21" + key2 + "52ae",
	}, {
		// sortedmulti sorts the keys, and key2 (0260...) sorts before
		// key1 (03a3...).
		name:       "bare sortedmulti",
		desc:       "sortedmulti(1," + key1 + "," + key2 + ")",
		wantScript: "5121" + key2 + "21" + key1 + "52ae",
	}, {
		name:       "bare pk",
		desc:       "pk(" + key1 + ")",
		wantScript: "21" + key1 + "ac",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d, err := NewDescriptor(tc.desc)
			require.NoError(t, err)
			require.Equal(t, DescTypeBare, d.DescType())

			script, err := d.ScriptCodeAt(0, 0)
			require.NoError(t, err)
			require.Equal(
				t, tc.wantScript, hex.EncodeToString(script),
			)

			// A bare descriptor has no address.
			_, err = d.AddressAt(&chaincfg.MainNetParams, 0, 0)
			require.ErrorContains(t, err, "has no address")
		})
	}
}
