package descriptors

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

// testCompressedKeys returns n deterministic compressed-key encodings as hex
// strings. For n <= 255, these are distinct valid keys derived from scalars
// 1..n. Larger counts wrap the one-byte scalar, including zero, and are only
// suitable for tests that expect descriptor rejection.
func testCompressedKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		privBytes := make([]byte, 32)
		privBytes[31] = byte(i + 1)
		_, pub := btcec.PrivKeyFromBytes(privBytes)
		keys[i] = hex.EncodeToString(pub.SerializeCompressed())
	}

	return keys
}

// shMulti returns an sh(multi(1, ...)) descriptor with the given number of
// keys. Its redeem script is 3 bytes (the two thresholds and OP_CHECKMULTISIG)
// plus 34 bytes per key.
func shMulti(kind string, numKeys int) string {
	return "sh(" + kind + "(1," +
		strings.Join(testCompressedKeys(numKeys), ",") + "))"
}

// shPkChain returns an sh(and_v(v:pk(K1),and_v(v:pk(K2),...pk(Kn))))
// descriptor, a miniscript inner whose redeem script is 35 bytes per key (the
// key push plus its OP_CHECKSIG, which the v: wrapper collapses into
// OP_CHECKSIGVERIFY).
func shPkChain(numKeys int) string {
	keys := testCompressedKeys(numKeys)

	var b strings.Builder
	b.WriteString("sh(")
	for _, key := range keys[:numKeys-1] {
		fmt.Fprintf(&b, "and_v(v:pk(%s),", key)
	}
	fmt.Fprintf(&b, "pk(%s)", keys[numKeys-1])
	b.WriteString(strings.Repeat(")", numKeys-1) + ")")

	return b.String()
}

// TestShRedeemScriptLimit checks that an sh() descriptor whose redeem script
// exceeds the 520-byte consensus limit on the size of a script element is
// rejected. The redeem script is pushed as a single element in the spending
// scriptSig, so coins sent to the address of a larger one can never be spent -
// and the package used to derive that address happily, and even report a
// satisfaction weight for it.
func TestShRedeemScriptLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		desc string

		// wantScriptSize is the size of the redeem script if the
		// descriptor is valid.
		wantScriptSize int

		// errStr is the expected error substring if it is not.
		errStr string
	}{{
		// 15 keys are 513 bytes, which is also the largest multi BIP383
		// allows inside an sh().
		name:           "multi with 15 keys",
		desc:           shMulti("multi", 15),
		wantScriptSize: 513,
	}, {
		name:   "multi with 16 keys",
		desc:   shMulti("multi", 16),
		errStr: "547 bytes, which is larger than the maximum redeem",
	}, {
		name:           "sortedmulti with 15 keys",
		desc:           shMulti("sortedmulti", 15),
		wantScriptSize: 513,
	}, {
		name:   "sortedmulti with 16 keys",
		desc:   shMulti("sortedmulti", 16),
		errStr: "547 bytes, which is larger than the maximum redeem",
	}, {
		// A miniscript inner is bound by the same limit through the
		// Legacy script context. Each key of the chain is 35 script
		// bytes, so 14 keys are 490 bytes and 15 are 525.
		name:           "miniscript below the limit",
		desc:           shPkChain(14),
		wantScriptSize: 490,
	}, {
		name: "miniscript over the limit",
		desc: shPkChain(15),
		errStr: "larger than the maximum script size of 520 in the " +
			"Legacy",
	}, {
		// The P2SH-wrapped segwit forms commit to a fixed-size witness
		// program and are unaffected.
		name: "sh-wpkh",
		desc: "sh(wpkh(" + testCompressedKeys(1)[0] + "))",
		// ScriptCodeAt of an sh(wpkh()) returns the P2PKH script code
		// its sighash is computed over, not the redeem script.
		wantScriptSize: 25,
	}, {
		name: "sh-wsh",
		desc: "sh(wsh(pk(" + testCompressedKeys(1)[0] + ")))",
		// ScriptCodeAt of an sh(wsh()) returns the witness script, not
		// the redeem script.
		wantScriptSize: 35,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d, err := NewDescriptor(tc.desc)
			if tc.errStr != "" {
				require.ErrorContains(t, err, tc.errStr)
				return
			}

			require.NoError(t, err)

			script, err := d.ScriptCodeAt(0, 0)
			require.NoError(t, err)
			require.Len(t, script, tc.wantScriptSize)

			// Anything that parses has to have a satisfaction
			// weight, i.e. the plan APIs must not report a spend
			// for an output that cannot be spent.
			_, err = d.MaxWeightToSatisfy()
			require.NoError(t, err)
		})
	}
}

// TestShScriptSigLimit checks that the scriptSig a legacy P2SH spend needs
// stays within the standardness limit for every sh() descriptor that is
// accepted. The redeem script limit of 520 bytes bounds the satisfaction so
// tightly that the 1650-byte scriptSig limit cannot be reached, which this test
// pins down: the largest satisfaction an accepted sh() descriptor can need is a
// 15-of-15 multisig.
func TestShScriptSigLimit(t *testing.T) {
	t.Parallel()

	keys := testCompressedKeys(15)
	desc := "sh(multi(15," + strings.Join(keys, ",") + "))"

	d, err := NewDescriptor(desc)
	require.NoError(t, err)

	// 15 signatures of 73 bytes, the dummy element, and the push of the
	// 513-byte redeem script.
	weight, err := d.MaxWeightToSatisfy()
	require.NoError(t, err)

	scriptSig := 1 + 15*ecdsaSigSize + 2 + 513
	require.Equal(
		t, uint64(4*(scriptSig+varintLen(uint64(scriptSig)))), weight,
	)
	require.Less(t, scriptSig, maxScriptSigSize)
}

// TestLegacyMalleableFragments checks that the fragments which are malleable in
// a pre-segwit script are rejected inside an sh(). Outside of segwit, the
// argument of an OP_IF is not required to be minimally encoded, so a third
// party can replace the branch selector of an or_i or d: satisfaction with any
// other non-zero value and change the transaction id. rust-miniscript rejects
// both in its Legacy context; this package used to derive a usable P2SH address
// for them.
func TestLegacyMalleableFragments(t *testing.T) {
	t.Parallel()

	keys := testCompressedKeys(2)

	tests := []struct {
		name   string
		inner  string
		errStr string
	}{{
		name:   "or_i",
		inner:  "or_i(pk(" + keys[0] + "),pk(" + keys[1] + "))",
		errStr: "or_i",
	}, {
		// or_d and or_c are not malleable in the same way: their branch
		// selector is the dissatisfaction of the first branch, not a
		// free-form OP_IF argument.
		name:  "or_d is fine",
		inner: "or_d(pk(" + keys[0] + "),pk(" + keys[1] + "))",
	}, {
		name:   "d wrapper",
		inner:  "and_v(v:pk(" + keys[0] + "),dv:older(144))",
		errStr: "d: wrapper",
	}, {
		// u: and l: are defined in terms of or_i, so they are rejected
		// as well.
		name:   "u wrapper",
		inner:  "and_v(v:pk(" + keys[0] + "),u:pk(" + keys[1] + "))",
		errStr: "or_i",
	}, {
		name:   "l wrapper",
		inner:  "and_v(v:pk(" + keys[0] + "),l:pk(" + keys[1] + "))",
		errStr: "or_i",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewDescriptor("sh(" + tc.inner + ")")
			if tc.errStr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.errStr)
			}

			// The same expression is unproblematic in a witness
			// script, where the OP_IF argument has to be minimally
			// encoded.
			_, err = NewDescriptor("wsh(" + tc.inner + ")")
			require.NoError(t, err)
		})
	}
}

// TestNestingRules checks that every descriptor expression is only accepted in
// the positions its BIP allows, so that an invalid nesting is rejected at parse
// time instead of failing at address derivation time (or, worse, deriving an
// address for a script that cannot be built).
func TestNestingRules(t *testing.T) {
	t.Parallel()

	key := testCompressedKeys(1)[0]
	xOnly := key[2:]

	tests := []struct {
		name   string
		desc   string
		errStr string
	}{{
		// BIP381 lists this as an invalid descriptor: sh() is top level
		// only.
		name:   "sh in sh",
		desc:   "sh(sh(pkh(" + key + ")))",
		errStr: "sh() cannot be used inside an sh() descriptor",
	}, {
		name:   "sh in wsh",
		desc:   "wsh(sh(pkh(" + key + ")))",
		errStr: "sh() cannot be used inside a wsh() descriptor",
	}, {
		// BIP386: tr() is top level only. This used to parse and only
		// fail at AddressAt with "cannot build script for node".
		name:   "tr in sh",
		desc:   "sh(tr(" + xOnly + "))",
		errStr: "tr() cannot be used inside an sh() descriptor",
	}, {
		name:   "tr in wsh",
		desc:   "wsh(tr(" + xOnly + "))",
		errStr: "tr() cannot be used inside a wsh() descriptor",
	}, {
		// BIP382 lists these as invalid descriptors.
		name:   "wsh in wsh",
		desc:   "wsh(wsh(pkh(" + key + ")))",
		errStr: "wsh() cannot be used inside a wsh() descriptor",
	}, {
		name:   "wsh in wsh in sh",
		desc:   "sh(wsh(wsh(pkh(" + key + "))))",
		errStr: "wsh() cannot be used inside a wsh() descriptor",
	}, {
		name:   "wpkh in wsh",
		desc:   "wsh(wpkh(" + key + "))",
		errStr: "wpkh() cannot be used inside a wsh() descriptor",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewDescriptor(tc.desc)
			require.ErrorContains(t, err, tc.errStr)
		})
	}

	// The nestings the BIPs do allow have to keep working.
	for _, desc := range []string{
		"pk(" + key + ")",
		"pkh(" + key + ")",
		"wpkh(" + key + ")",
		"sh(wpkh(" + key + "))",
		"sh(wsh(pk(" + key + ")))",
		"sh(pkh(" + key + "))",
		"sh(multi(1," + key + "))",
		"wsh(pk(" + key + "))",
		"wsh(pkh(" + key + "))",
		"wsh(multi(1," + key + "))",
		"tr(" + xOnly + ")",
		"tr(" + xOnly + ",pk(" + xOnly + "))",
	} {

		_, err := NewDescriptor(desc)
		require.NoErrorf(t, err, "descriptor %s", desc)
	}
}

// TestMultiKeyCountLimits checks the key count limits of multi() and
// sortedmulti(). The descriptor-level multisig does not go through the
// miniscript parser, whose argCheck caps the key count, so it used to accept
// any number of keys: a 21-key multi produced a perfectly ordinary-looking
// address whose OP_CHECKMULTISIG the interpreter rejects at spend time, burning
// the coins sent to it.
func TestMultiKeyCountLimits(t *testing.T) {
	t.Parallel()

	multi := func(kind, wrap string, numKeys int) string {
		inner := kind + "(1," +
			strings.Join(testCompressedKeys(numKeys), ",") + ")"
		if wrap == "" {
			return inner
		}

		return wrap + "(" + inner + ")"
	}

	for _, kind := range []string{"multi", "sortedmulti"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			// A witness script may use the full 20 keys
			// OP_CHECKMULTISIG accepts, but not more.
			_, err := NewDescriptor(multi(kind, "wsh", 20))
			require.NoError(t, err)

			_, err = NewDescriptor(multi(kind, "wsh", 21))
			require.ErrorContains(
				t, err,
				"more than the 20 keys OP_CHECKMULTISIG "+
					"accepts",
			)

			// A bare multisig is only standard with up to three
			// keys.
			_, err = NewDescriptor(multi(kind, "", 3))
			require.NoError(t, err)

			_, err = NewDescriptor(multi(kind, "", 4))
			require.ErrorContains(
				t, err, "more than the 3 keys a bare multisig",
			)

			// Inside an sh() the redeem script size is what limits
			// the key count, which BIP383 puts at 15 keys.
			_, err = NewDescriptor(multi(kind, "sh", 15))
			require.NoError(t, err)

			_, err = NewDescriptor(multi(kind, "sh", 16))
			require.ErrorContains(
				t, err, "larger than the maximum redeem script",
			)
		})
	}
}
