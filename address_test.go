// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
package btcscript_test

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/conformal/btcnet"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
)

// decodeHex decodes the passed hex string and returns the resulting bytes.  It
// panics if an error occurs.  This is only used in the tests as a helper since
// the only way it can fail is if there is an error in the test source code.
func decodeHex(hexStr string) []byte {
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		panic("invalid hex string in test source: err " + err.Error() +
			", hex: " + hexStr)
	}

	return b
}

// newAddressPubKey returns a new btcutil.AddressPubKey from the provided
// serialized public key.  It panics if an error occurs.  This is only used in
// the tests as a helper since the only way it can fail is if there is an error
// in the test source code.
func newAddressPubKey(serializedPubKey []byte) btcutil.Address {
	addr, err := btcutil.NewAddressPubKey(serializedPubKey,
		&btcnet.MainNetParams)
	if err != nil {
		panic("invalid public key in test source")
	}

	return addr
}

// newAddressPubKeyHash returns a new btcutil.AddressPubKeyHash from the
// provided hash.  It panics if an error occurs.  This is only used in the tests
// as a helper since the only way it can fail is if there is an error in the
// test source code.
func newAddressPubKeyHash(pkHash []byte) btcutil.Address {
	addr, err := btcutil.NewAddressPubKeyHash(pkHash, &btcnet.MainNetParams)
	if err != nil {
		panic("invalid public key hash in test source")
	}

	return addr
}

// newAddressScriptHash returns a new btcutil.AddressScriptHash from the
// provided hash.  It panics if an error occurs.  This is only used in the tests
// as a helper since the only way it can fail is if there is an error in the
// test source code.
func newAddressScriptHash(scriptHash []byte) btcutil.Address {
	addr, err := btcutil.NewAddressScriptHashFromHash(scriptHash,
		&btcnet.MainNetParams)
	if err != nil {
		panic("invalid script hash in test source")
	}

	return addr
}

// TestExtractPkScriptAddrs ensures that extracting the type, addresses, and
// number of required signatures from PkScripts works as intended.
func TestExtractPkScriptAddrs(t *testing.T) {
	tests := []struct {
		name    string
		script  []byte
		addrs   []btcutil.Address
		reqSigs int
		class   btcscript.ScriptClass
	}{
		{
			name: "standard p2pk with compressed pubkey (0x02)",
			script: decodeHex("2102192d74d0cb94344c9569c2e7790157" +
				"3d8d7903c3ebec3a957724895dca52c6b4ac"),
			addrs: []btcutil.Address{
				newAddressPubKey(decodeHex("02192d74d0cb94344" +
					"c9569c2e77901573d8d7903c3ebec3a95772" +
					"4895dca52c6b4")),
			},
			reqSigs: 1,
			class:   btcscript.PubKeyTy,
		},
		{
			name: "standard p2pk with uncompressed pubkey (0x04)",
			script: decodeHex("410411db93e1dcdb8a016b49840f8c53bc" +
				"1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb" +
				"84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643" +
				"f656b412a3ac"),
			addrs: []btcutil.Address{
				newAddressPubKey(decodeHex("0411db93e1dcdb8a0" +
					"16b49840f8c53bc1eb68a382e97b1482ecad" +
					"7b148a6909a5cb2e0eaddfb84ccf9744464f" +
					"82e160bfa9b8b64f9d4c03f999b8643f656b" +
					"412a3")),
			},
			reqSigs: 1,
			class:   btcscript.PubKeyTy,
		},
		{
			name: "standard p2pk with hybrid pubkey (0x06)",
			script: decodeHex("4106192d74d0cb94344c9569c2e7790157" +
				"3d8d7903c3ebec3a957724895dca52c6b40d45264838" +
				"c0bd96852662ce6a847b197376830160c6d2eb5e6a4c" +
				"44d33f453eac"),
			addrs: []btcutil.Address{
				newAddressPubKey(decodeHex("06192d74d0cb94344" +
					"c9569c2e77901573d8d7903c3ebec3a95772" +
					"4895dca52c6b40d45264838c0bd96852662c" +
					"e6a847b197376830160c6d2eb5e6a4c44d33" +
					"f453e")),
			},
			reqSigs: 1,
			class:   btcscript.PubKeyTy,
		},
		{
			name: "standard p2pk with compressed pubkey (0x03)",
			script: decodeHex("2103b0bd634234abbb1ba1e986e884185c" +
				"61cf43e001f9137f23c2c409273eb16e65ac"),
			addrs: []btcutil.Address{
				newAddressPubKey(decodeHex("03b0bd634234abbb1" +
					"ba1e986e884185c61cf43e001f9137f23c2c" +
					"409273eb16e65")),
			},
			reqSigs: 1,
			class:   btcscript.PubKeyTy,
		},
		{
			name: "2nd standard p2pk with uncompressed pubkey (0x04)",
			script: decodeHex("4104b0bd634234abbb1ba1e986e884185c" +
				"61cf43e001f9137f23c2c409273eb16e6537a576782e" +
				"ba668a7ef8bd3b3cfb1edb7117ab65129b8a2e681f3c" +
				"1e0908ef7bac"),
			addrs: []btcutil.Address{
				newAddressPubKey(decodeHex("04b0bd634234abbb1" +
					"ba1e986e884185c61cf43e001f9137f23c2c" +
					"409273eb16e6537a576782eba668a7ef8bd3" +
					"b3cfb1edb7117ab65129b8a2e681f3c1e090" +
					"8ef7b")),
			},
			reqSigs: 1,
			class:   btcscript.PubKeyTy,
		},
		{
			name: "standard p2pk with hybrid pubkey (0x07)",
			script: decodeHex("4107b0bd634234abbb1ba1e986e884185c" +
				"61cf43e001f9137f23c2c409273eb16e6537a576782e" +
				"ba668a7ef8bd3b3cfb1edb7117ab65129b8a2e681f3c" +
				"1e0908ef7bac"),
			addrs: []btcutil.Address{
				newAddressPubKey(decodeHex("07b0bd634234abbb1" +
					"ba1e986e884185c61cf43e001f9137f23c2c" +
					"409273eb16e6537a576782eba668a7ef8bd3" +
					"b3cfb1edb7117ab65129b8a2e681f3c1e090" +
					"8ef7b")),
			},
			reqSigs: 1,
			class:   btcscript.PubKeyTy,
		},
		{
			name: "standard p2pkh",
			script: decodeHex("76a914ad06dd6ddee55cbca9a9e3713bd7" +
				"587509a3056488ac"),
			addrs: []btcutil.Address{
				newAddressPubKeyHash(decodeHex("ad06dd6ddee55" +
					"cbca9a9e3713bd7587509a30564")),
			},
			reqSigs: 1,
			class:   btcscript.PubKeyHashTy,
		},
		{
			name: "standard p2sh",
			script: decodeHex("a91463bcc565f9e68ee0189dd5cc67f1b0" +
				"e5f02f45cb87"),
			addrs: []btcutil.Address{
				newAddressScriptHash(decodeHex("63bcc565f9e68" +
					"ee0189dd5cc67f1b0e5f02f45cb")),
			},
			reqSigs: 1,
			class:   btcscript.ScriptHashTy,
		},
		// from real tx 60a20bd93aa49ab4b28d514ec10b06e1829ce6818ec06cd3aabd013ebcdc4bb1, vout 0
		{
			name: "standard 1 of 2 multisig",
			script: decodeHex("514104cc71eb30d653c0c3163990c47b97" +
				"6f3fb3f37cccdcbedb169a1dfef58bbfbfaff7d8a473" +
				"e7e2e6d317b87bafe8bde97e3cf8f065dec022b51d11" +
				"fcdd0d348ac4410461cbdcc5409fb4b4d42b51d33381" +
				"354d80e550078cb532a34bfa2fcfdeb7d76519aecc62" +
				"770f5b0e4ef8551946d8a540911abe3e7854a26f39f5" +
				"8b25c15342af52ae"),
			addrs: []btcutil.Address{
				newAddressPubKey(decodeHex("04cc71eb30d653c0c" +
					"3163990c47b976f3fb3f37cccdcbedb169a1" +
					"dfef58bbfbfaff7d8a473e7e2e6d317b87ba" +
					"fe8bde97e3cf8f065dec022b51d11fcdd0d3" +
					"48ac4")),
				newAddressPubKey(decodeHex("0461cbdcc5409fb4b" +
					"4d42b51d33381354d80e550078cb532a34bf" +
					"a2fcfdeb7d76519aecc62770f5b0e4ef8551" +
					"946d8a540911abe3e7854a26f39f58b25c15" +
					"342af")),
			},
			reqSigs: 1,
			class:   btcscript.MultiSigTy,
		},
		// from real tx d646f82bd5fbdb94a36872ce460f97662b80c3050ad3209bef9d1e398ea277ab, vin 1
		{
			name: "standard 2 of 3 multisig",
			script: decodeHex("524104cb9c3c222c5f7a7d3b9bd152f363" +
				"a0b6d54c9eb312c4d4f9af1e8551b6c421a6a4ab0e29" +
				"105f24de20ff463c1c91fcf3bf662cdde4783d4799f7" +
				"87cb7c08869b4104ccc588420deeebea22a7e900cc8b" +
				"68620d2212c374604e3487ca08f1ff3ae12bdc639514" +
				"d0ec8612a2d3c519f084d9a00cbbe3b53d071e9b09e7" +
				"1e610b036aa24104ab47ad1939edcb3db65f7fedea62" +
				"bbf781c5410d3f22a7a3a56ffefb2238af8627363bdf" +
				"2ed97c1f89784a1aecdb43384f11d2acc64443c7fc29" +
				"9cef0400421a53ae"),
			addrs: []btcutil.Address{
				newAddressPubKey(decodeHex("04cb9c3c222c5f7a7" +
					"d3b9bd152f363a0b6d54c9eb312c4d4f9af1" +
					"e8551b6c421a6a4ab0e29105f24de20ff463" +
					"c1c91fcf3bf662cdde4783d4799f787cb7c0" +
					"8869b")),
				newAddressPubKey(decodeHex("04ccc588420deeebe" +
					"a22a7e900cc8b68620d2212c374604e3487c" +
					"a08f1ff3ae12bdc639514d0ec8612a2d3c51" +
					"9f084d9a00cbbe3b53d071e9b09e71e610b0" +
					"36aa2")),
				newAddressPubKey(decodeHex("04ab47ad1939edcb3" +
					"db65f7fedea62bbf781c5410d3f22a7a3a56" +
					"ffefb2238af8627363bdf2ed97c1f89784a1" +
					"aecdb43384f11d2acc64443c7fc299cef040" +
					"0421a")),
			},
			reqSigs: 2,
			class:   btcscript.MultiSigTy,
		},

		// The below are nonstandard script due to things such as
		// invalid pubkeys, failure to parse, and not being of a
		// standard form.

		{
			name: "p2pk with uncompressed pk missing OP_CHECKSIG",
			script: decodeHex("410411db93e1dcdb8a016b49840f8c53bc" +
				"1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb" +
				"84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643" +
				"f656b412a3"),
			addrs:   nil,
			reqSigs: 0,
			class:   btcscript.NonStandardTy,
		},
		{
			name: "valid signature from a sigscript - no addresses",
			script: decodeHex("47304402204e45e16932b8af514961a1d3" +
				"a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220" +
				"181522ec8eca07de4860a4acdd12909d831cc56cbbac" +
				"4622082221a8768d1d0901"),
			addrs:   nil,
			reqSigs: 0,
			class:   btcscript.NonStandardTy,
		},
		// Note the technically the pubkey is the second item on the
		// stack, but since the address extraction intentionally only
		// works with standard PkScripts, this should not return any
		// addresses.
		{
			name: "valid sigscript to reedeem p2pk - no addresses",
			script: decodeHex("493046022100ddc69738bf2336318e4e04" +
				"1a5a77f305da87428ab1606f023260017854350ddc02" +
				"2100817af09d2eec36862d16009852b7e3a0f6dd7659" +
				"8290b7834e1453660367e07a014104cd4240c198e125" +
				"23b6f9cb9f5bed06de1ba37e96a1bbd13745fcf9d11c" +
				"25b1dff9a519675d198804ba9962d3eca2d5937d58e5" +
				"a75a71042d40388a4d307f887d"),
			addrs:   nil,
			reqSigs: 0,
			class:   btcscript.NonStandardTy,
		},
		// from real tx 691dd277dc0e90a462a3d652a1171686de49cf19067cd33c7df0392833fb986a, vout 0
		// invalid public keys
		{
			name: "1 of 3 multisig with invalid pubkeys",
			script: decodeHex("51411c2200007353455857696b696c6561" +
				"6b73204361626c6567617465204261636b75700a0a63" +
				"61626c65676174652d3230313031323034313831312e" +
				"377a0a0a446f41776e6c6f61642074686520666f6c6c" +
				"6f77696e67207472616e73616374696f6e7320776974" +
				"68205361746f736869204e616b616d6f746f27732064" +
				"6f776e6c6f61416420746f6f6c2077686963680a6361" +
				"6e20626520666f756e6420696e207472616e73616374" +
				"696f6e20366335336364393837313139656637393764" +
				"35616463636453ae"),
			addrs:   []btcutil.Address{},
			reqSigs: 1,
			class:   btcscript.MultiSigTy,
		},
		// from real tx: 691dd277dc0e90a462a3d652a1171686de49cf19067cd33c7df0392833fb986a, vout 44
		// invalid public keys
		{
			name: "1 of 3 multisig with invalid pubkeys 2",
			script: decodeHex("5141346333656332353963373464616365" +
				"36666430383862343463656638630a63363662633139" +
				"39366338623934613338313162333635363138666531" +
				"65396231623541366361636365393933613339383861" +
				"34363966636336643664616266640a32363633636661" +
				"39636634633033633630396335393363336539316665" +
				"64653730323921313233646434326432353633396433" +
				"38613663663530616234636434340a00000053ae"),
			addrs:   []btcutil.Address{},
			reqSigs: 1,
			class:   btcscript.MultiSigTy,
		},
		{
			name:    "empty script",
			script:  []byte{},
			addrs:   nil,
			reqSigs: 0,
			class:   btcscript.NonStandardTy,
		},
		{
			name:    "script that does not parse",
			script:  []byte{btcscript.OP_DATA_45},
			addrs:   nil,
			reqSigs: 0,
			class:   btcscript.NonStandardTy,
		},
	}

	t.Logf("Running %d tests.", len(tests))
	for i, test := range tests {
		class, addrs, reqSigs, err := btcscript.ExtractPkScriptAddrs(
			test.script, &btcnet.MainNetParams)
		if err != nil {
		}

		if !reflect.DeepEqual(addrs, test.addrs) {
			t.Errorf("ExtractPkScriptAddrs #%d (%s) unexpected "+
				"addresses\ngot  %v\nwant %v", i, test.name,
				addrs, test.addrs)
			continue
		}

		if reqSigs != test.reqSigs {
			t.Errorf("ExtractPkScriptAddrs #%d (%s) unexpected "+
				"number of required signatures - got %d, "+
				"want %d", i, test.name, reqSigs, test.reqSigs)
			continue
		}

		if class != test.class {
			t.Errorf("ExtractPkScriptAddrs #%d (%s) unexpected "+
				"script type - got %s, want %s", i, test.name,
				class, test.class)
			continue
		}
	}
}
