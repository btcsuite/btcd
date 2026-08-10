package descriptors

import (
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/stretchr/testify/require"
)

// The key pair used by the BIP381 and BIP383 test vectors, in its compressed
// and uncompressed serialization, plus the generator point in both forms, whose
// addresses are widely published.
const (
	bipKeyCompressed = "03a34b99f22c790c4e36b2b3c2c35a36db06226e41c692fc8" +
		"2b8b56ac1c540c5bd"
	bipKeyUncompressed = "04a34b99f22c790c4e36b2b3c2c35a36db06226e41c692f" +
		"c82b8b56ac1c540c5bd5b8dec5235a0fa8722476c7709c02559e3aa73aa0" +
		"3918ba2d492eea75abea235"

	genCompressed = "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959" +
		"f2815b16f81798"
	genUncompressed = "0479be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d9" +
		"59f2815b16f81798483ada7726a3c4655da4fbfc0e1108a8fd17b448a685" +
		"54199c47d08ffb10d4b8"
	genXOnly = "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b1" +
		"6f81798"
)

// TestUncompressedKeyScripts checks the scripts derived for the pre-segwit
// positions that BIP380, BIP381 and BIP383 allow uncompressed keys in, against
// the test vectors of those BIPs.
//
// Uncompressed keys used to be silently re-serialized as compressed, which
// produced a different script, and therefore a different address, than every
// BIP380-conformant implementation derives for the same descriptor: a wallet
// importing pkh(<uncompressed key>) from Bitcoin Core or rust-miniscript
// watched a different address than this package, so deposits were missed.
func TestUncompressedKeyScripts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		desc string

		// wantScript is the script code, as given by the BIP test
		// vectors. For an sh() descriptor that is its redeem script.
		wantScript string
	}{{
		// BIP381: pk() keeps the uncompressed key verbatim, pushed with
		// its 65-byte push opcode 0x41.
		name:       "bare pk",
		desc:       "pk(" + bipKeyUncompressed + ")",
		wantScript: "41" + bipKeyUncompressed + "ac",
	}, {
		// BIP381: the key hash commits to the uncompressed
		// serialization.
		name: "bare pkh",
		desc: "pkh(" + bipKeyUncompressed + ")",
		wantScript: "76a914b5bd079c4d57cc7fc28ecf8213a6b791625b818388" +
			"ac",
	}, {
		// BIP383: a multi may mix both serializations, each key pushed
		// as written.
		name: "sh multi with both forms",
		desc: "sh(multi(1," + bipKeyCompressed + "," +
			bipKeyUncompressed + "))",
		wantScript: "5121" + bipKeyCompressed + "41" +
			bipKeyUncompressed + "52ae",
	}, {
		// BIP383: sortedmulti sorts the keys by their serialization, so
		// the compressed key (0x03...) comes before the uncompressed
		// one (0x04...) and the script is the same as the multi above.
		// This only holds if the keys are sorted in the form they are
		// used in.
		name: "sh sortedmulti with both forms",
		desc: "sh(sortedmulti(1," + bipKeyUncompressed + "," +
			bipKeyCompressed + "))",
		wantScript: "5121" + bipKeyCompressed + "41" +
			bipKeyUncompressed + "52ae",
	}, {
		// The same key inside an sh() pk(), which is a legacy position
		// as well.
		name:       "sh pk",
		desc:       "sh(pk(" + bipKeyUncompressed + "))",
		wantScript: "41" + bipKeyUncompressed + "ac",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d, err := NewDescriptor(tc.desc)
			require.NoError(t, err)

			script, err := d.ScriptCodeAt(0, 0)
			require.NoError(t, err)
			require.Equal(t, tc.wantScript, hex.EncodeToString(
				script,
			))
		})
	}
}

// TestUncompressedKeyAddresses checks the addresses derived for a descriptor
// that uses the generator point, whose compressed and uncompressed addresses
// are widely published. The two have to differ: they used to be identical,
// because the uncompressed key was compressed before hashing.
func TestUncompressedKeyAddresses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc     string
		wantAddr string
	}{{
		desc:     "pkh(" + genUncompressed + ")",
		wantAddr: "1EHNa6Q4Jz2uvNExL497mE43ikXhwF6kZm",
	}, {
		desc:     "pkh(" + genCompressed + ")",
		wantAddr: "1BgGZ9tcN4rm9KBzDn7KprQz87SZ26SAMH",
	}}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			d, err := NewDescriptor(tc.desc)
			require.NoError(t, err)

			addr, err := d.AddressAt(&chaincfg.MainNetParams, 0, 0)
			require.NoError(t, err)
			require.Equal(t, tc.wantAddr, addr)
		})
	}
}

// TestKeyFormRejected checks that a key serialization which is invalid in the
// position it appears in is rejected at parse time, rather than silently
// re-serialized into a form the descriptor does not name.
func TestKeyFormRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		desc   string
		errStr string
	}{{
		// BIP382 lists all of these as invalid descriptors: segwit v0
		// only takes compressed keys.
		name:   "wpkh",
		desc:   "wpkh(" + bipKeyUncompressed + ")",
		errStr: "uncompressed public key",
	}, {
		name:   "sh-wpkh",
		desc:   "sh(wpkh(" + bipKeyUncompressed + "))",
		errStr: "uncompressed public key",
	}, {
		name:   "wsh pk",
		desc:   "wsh(pk(" + bipKeyUncompressed + "))",
		errStr: "uncompressed public key",
	}, {
		name:   "wsh pkh",
		desc:   "wsh(pkh(" + bipKeyUncompressed + "))",
		errStr: "uncompressed public key",
	}, {
		name: "wsh multi",
		desc: "wsh(multi(1," + bipKeyCompressed + "," +
			bipKeyUncompressed + "))",
		errStr: "uncompressed public key",
	}, {
		// BIP386: tr() takes x-only and compressed keys, which it
		// converts, but no uncompressed ones.
		name:   "tr internal key",
		desc:   "tr(" + bipKeyUncompressed + ")",
		errStr: "uncompressed public key",
	}, {
		name:   "tr leaf key",
		desc:   "tr(" + genXOnly + ",pk(" + bipKeyUncompressed + "))",
		errStr: "uncompressed public key",
	}, {
		// Miniscript is only defined for the wsh() and tr() contexts
		// (BIP379), so its keys are compressed even in a legacy
		// position.
		name:   "sh miniscript",
		desc:   "sh(and_v(v:pk(" + bipKeyUncompressed + "),older(9)))",
		errStr: "uncompressed public key",
	}, {
		name:   "wsh miniscript",
		desc:   "wsh(and_v(v:pk(" + bipKeyUncompressed + "),older(9)))",
		errStr: "uncompressed public key",
	}, {
		// BIP386 only defines x-only keys inside tr(). Outside, a
		// 32-byte key carries an implicit even-Y assumption that other
		// implementations treat as an error.
		name:   "x-only in pkh",
		desc:   "pkh(" + genXOnly + ")",
		errStr: "x-only public key",
	}, {
		name:   "x-only in wpkh",
		desc:   "wpkh(" + genXOnly + ")",
		errStr: "x-only public key",
	}, {
		name:   "x-only in sh multi",
		desc:   "sh(multi(1," + genXOnly + "))",
		errStr: "x-only public key",
	}, {
		name:   "x-only in wsh miniscript",
		desc:   "wsh(and_v(v:pk(" + genXOnly + "),older(9)))",
		errStr: "x-only public key",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewDescriptor(tc.desc)
			require.ErrorContains(t, err, tc.errStr)
		})
	}

	// The same descriptors with a compressed key, or with the x-only key in
	// a tr(), have to be accepted.
	for _, desc := range []string{
		"wpkh(" + bipKeyCompressed + ")",
		"sh(wpkh(" + bipKeyCompressed + "))",
		"wsh(pk(" + bipKeyCompressed + "))",
		"tr(" + bipKeyCompressed + ")",
		"tr(" + genXOnly + ")",
		"wsh(and_v(v:pk(" + bipKeyCompressed + "),older(9)))",
		"sh(and_v(v:pk(" + bipKeyCompressed + "),older(9)))",
	} {

		_, err := NewDescriptor(desc)
		require.NoErrorf(t, err, "descriptor %s", desc)
	}
}

// TestWIFKeys checks that a WIF encoded private key works as a key expression
// (BIP380): the key it contributes is the public key of that private key, in
// the serialization the WIF asks for, so an uncompressed WIF is only valid
// where an uncompressed public key is.
func TestWIFKeys(t *testing.T) {
	t.Parallel()

	// The two WIF keys of the BIP380 vectors, which encode the same key
	// pair as bipKeyCompressed and bipKeyUncompressed.
	const (
		wifCompressed = "L4rK1yDtCWekvXuE6oXD9jCYfFNV2cWRpVuPLBcCU2z8" +
			"TrisoyY1"
		wifUncompressed = "5KYZdUEo39z3FPrtuX2QbbwGnNP5zTd7yyr2SC1j29" +
			"9sBCnWjss"
	)

	// A WIF key and the hex key it encodes have to produce the same script.
	scriptOf := func(desc string) string {
		d, err := NewDescriptor(desc)
		require.NoErrorf(t, err, "descriptor %s", desc)

		script, err := d.ScriptCodeAt(0, 0)
		require.NoErrorf(t, err, "script of %s", desc)

		return hex.EncodeToString(script)
	}

	require.Equal(t, scriptOf("pk("+bipKeyCompressed+")"), scriptOf(
		"pk("+wifCompressed+")",
	))
	require.Equal(t, scriptOf("pkh("+bipKeyUncompressed+")"), scriptOf(
		"pkh("+wifUncompressed+")",
	))
	require.Equal(t, scriptOf("wsh(pk("+bipKeyCompressed+"))"), scriptOf(
		"wsh(pk("+wifCompressed+"))",
	))

	// A compressed WIF is converted to x-only inside a tr(), like a
	// compressed hex key.
	trWIF, err := NewDescriptor("tr(" + wifCompressed + ")")
	require.NoError(t, err)
	trHex, err := NewDescriptor("tr(" + bipKeyCompressed + ")")
	require.NoError(t, err)

	wifAddr, err := trWIF.AddressAt(&chaincfg.MainNetParams, 0, 0)
	require.NoError(t, err)
	hexAddr, err := trHex.AddressAt(&chaincfg.MainNetParams, 0, 0)
	require.NoError(t, err)
	require.Equal(t, hexAddr, wifAddr)

	// An uncompressed WIF is an uncompressed key, so it is invalid wherever
	// one is (BIP382, BIP386).
	for _, desc := range []string{
		"wpkh(" + wifUncompressed + ")",
		"sh(wpkh(" + wifUncompressed + "))",
		"wsh(pk(" + wifUncompressed + "))",
		"tr(" + wifUncompressed + ")",
	} {

		_, err := NewDescriptor(desc)
		require.ErrorContainsf(
			t, err, "uncompressed public key", "descriptor %s",
			desc,
		)
	}

	// A private key must not have a derivation path (BIP380).
	for _, desc := range []string{
		"pk(" + wifCompressed + "/0)",
		"pk(" + wifCompressed + "/*)",
	} {

		_, err := NewDescriptor(desc)
		require.ErrorContainsf(
			t, err, "must not have a derivation path",
			"descriptor %s", desc,
		)
	}

	// The key is reported as written, and a WIF key is not ranged.
	d, err := NewDescriptor("pkh(" + wifCompressed + ")")
	require.NoError(t, err)
	require.Equal(t, []string{wifCompressed}, d.Keys())
	require.Equal(t, 1, d.MultipathLen())
}
