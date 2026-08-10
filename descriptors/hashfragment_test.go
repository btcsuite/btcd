package descriptors

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/stretchr/testify/require"
)

// The hash values used by the hash fragment tests, a sha256 and a hash160 (or
// ripemd160) sized one.
const (
	testSha256 = "926a54995ca48600920a19bf7bc502ca5f2f7d07e6f804c4f00ebf0" +
		"325084dbc"
	testHash160 = "5c9e2a8e0dd2c1f8b3a5f8e0d0c7b6a594837261"
)

// TestHashFragmentScripts checks that a descriptor holding a hash fragment can
// be compiled. The lookup that substitutes the descriptor's keys used to error
// on any identifier it did not know, but a miniscript also carries the hash
// values of its hash fragments as variables, so every descriptor with a
// sha256(), hash256(), ripemd160() or hash160() fragment failed at script build
// time with `unknown key "<hash>"` - i.e. it parsed and then had no address.
func TestHashFragmentScripts(t *testing.T) {
	t.Parallel()

	key := testCompressedKeys(1)[0]
	xOnly := key[2:]

	tests := []struct {
		name string
		desc string

		// wantScriptSize is the size of the compiled script, and
		// wantHash the hash value it has to embed.
		wantScriptSize int
		wantHash       string
	}{{
		// 35 bytes for the key push and its OP_CHECKSIGVERIFY, plus 39
		// for the sha256 fragment (OP_SIZE <32> OP_EQUALVERIFY
		// OP_SHA256 <hash> OP_EQUAL).
		name:           "wsh sha256",
		desc:           "wsh(and_v(v:pk(" + key + "),sha256(" + testSha256 + ")))",
		wantScriptSize: 35 + 39,
		wantHash:       testSha256,
	}, {
		// A 20-byte hash fragment is 27 bytes.
		name: "wsh hash160",
		desc: "wsh(and_v(v:pk(" + key + "),hash160(" + testHash160 +
			")))",
		wantScriptSize: 35 + 27,
		wantHash:       testHash160,
	}, {
		name: "wsh ripemd160 in a branch",
		desc: "wsh(or_d(pk(" + key + "),and_v(v:pkh(" +
			testCompressedKeys(2)[1] + "),ripemd160(" +
			testHash160 + "))))",
		wantHash: testHash160,
	}, {
		// A tapscript leaf, where the sha256 fragment is the same size
		// but the key push is one byte shorter.
		name: "tr leaf sha256",
		desc: "tr(" + xOnly + ",and_v(v:pk(" + xOnly + "),sha256(" +
			testSha256 + ")))",
		wantHash: testSha256,
	}, {
		// A legacy P2SH redeem script.
		name:           "sh sha256",
		desc:           "sh(and_v(v:pk(" + key + "),sha256(" + testSha256 + ")))",
		wantScriptSize: 35 + 39,
		wantHash:       testSha256,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d, err := NewDescriptor(tc.desc)
			require.NoError(t, err)

			// Deriving an address needs the script, which is what
			// used to fail.
			addr, err := d.AddressAt(&chaincfg.MainNetParams, 0, 0)
			require.NoError(t, err)
			require.NotEmpty(t, addr)

			// A tr() descriptor has no script code, its leaves are
			// reached through the tree instead.
			if strings.HasPrefix(tc.desc, "tr(") {
				return
			}

			script, err := d.ScriptCodeAt(0, 0)
			require.NoError(t, err)

			if tc.wantScriptSize != 0 {
				require.Len(t, script, tc.wantScriptSize)
			}

			// The script has to commit to the hash value from the
			// descriptor.
			hashBytes, err := hex.DecodeString(tc.wantHash)
			require.NoError(t, err)
			require.Contains(t, string(script), string(hashBytes))
		})
	}
}
