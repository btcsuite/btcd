// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
package txscript_test

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcutil"
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
		&chaincfg.MainNetParams)
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
	addr, err := btcutil.NewAddressPubKeyHash(pkHash, &chaincfg.MainNetParams)
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
		&chaincfg.MainNetParams)
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
		class   txscript.ScriptClass
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
			class:   txscript.PubKeyTy,
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
			class:   txscript.PubKeyTy,
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
			class:   txscript.PubKeyTy,
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
			class:   txscript.PubKeyTy,
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
			class:   txscript.PubKeyTy,
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
			class:   txscript.PubKeyTy,
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
			class:   txscript.PubKeyHashTy,
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
			class:   txscript.ScriptHashTy,
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
			class:   txscript.MultiSigTy,
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
			class:   txscript.MultiSigTy,
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
			class:   txscript.NonStandardTy,
		},
		{
			name: "valid signature from a sigscript - no addresses",
			script: decodeHex("47304402204e45e16932b8af514961a1d3" +
				"a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220" +
				"181522ec8eca07de4860a4acdd12909d831cc56cbbac" +
				"4622082221a8768d1d0901"),
			addrs:   nil,
			reqSigs: 0,
			class:   txscript.NonStandardTy,
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
			class:   txscript.NonStandardTy,
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
			class:   txscript.MultiSigTy,
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
			class:   txscript.MultiSigTy,
		},
		{
			name:    "empty script",
			script:  []byte{},
			addrs:   nil,
			reqSigs: 0,
			class:   txscript.NonStandardTy,
		},
		{
			name:    "script that does not parse",
			script:  []byte{txscript.OP_DATA_45},
			addrs:   nil,
			reqSigs: 0,
			class:   txscript.NonStandardTy,
		},
	}

	t.Logf("Running %d tests.", len(tests))
	for i, test := range tests {
		class, addrs, reqSigs, err := txscript.ExtractPkScriptAddrs(
			test.script, &chaincfg.MainNetParams)
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

type scriptInfoTest struct {
	name          string
	sigScript     []byte
	pkScript      []byte
	bip16         bool
	scriptInfo    txscript.ScriptInfo
	scriptInfoErr error
}

func TestScriptInfo(t *testing.T) {
	t.Parallel()

	for _, test := range txTests {
		si, err := txscript.CalcScriptInfo(
			test.tx.TxIn[test.idx].SignatureScript,
			test.pkScript, test.bip16)
		if err != nil {
			if err != test.scriptInfoErr {
				t.Errorf("scriptinfo test \"%s\": got \"%v\""+
					"expected \"%v\"", test.name, err,
					test.scriptInfoErr)
			}
			continue
		}
		if test.scriptInfoErr != nil {
			t.Errorf("%s: succeeded when expecting \"%v\"",
				test.name, test.scriptInfoErr)
			continue
		}
		if *si != test.scriptInfo {
			t.Errorf("%s: scriptinfo doesn't match expected. "+
				"got: \"%v\" expected \"%v\"", test.name,
				*si, test.scriptInfo)
			continue
		}
	}

	extraTests := []scriptInfoTest{
		{
			// Invented scripts, the hashes do not match
			name: "pkscript doesn't parse",
			sigScript: []byte{txscript.OP_TRUE,
				txscript.OP_DATA_1, 81,
				txscript.OP_DATA_8,
				txscript.OP_2DUP, txscript.OP_EQUAL,
				txscript.OP_NOT, txscript.OP_VERIFY,
				txscript.OP_ABS, txscript.OP_SWAP,
				txscript.OP_ABS, txscript.OP_EQUAL,
			},
			// truncated version of test below:
			pkScript: []byte{txscript.OP_HASH160,
				txscript.OP_DATA_20,
				0xfe, 0x44, 0x10, 0x65, 0xb6, 0x53, 0x22, 0x31,
				0xde, 0x2f, 0xac, 0x56, 0x31, 0x52, 0x20, 0x5e,
				0xc4, 0xf5, 0x9c,
			},
			bip16:         true,
			scriptInfoErr: txscript.ErrStackShortScript,
		},
		{
			name: "sigScript doesn't parse",
			// Truncated version of p2sh script below.
			sigScript: []byte{txscript.OP_TRUE,
				txscript.OP_DATA_1, 81,
				txscript.OP_DATA_8,
				txscript.OP_2DUP, txscript.OP_EQUAL,
				txscript.OP_NOT, txscript.OP_VERIFY,
				txscript.OP_ABS, txscript.OP_SWAP,
				txscript.OP_ABS,
			},
			pkScript: []byte{txscript.OP_HASH160,
				txscript.OP_DATA_20,
				0xfe, 0x44, 0x10, 0x65, 0xb6, 0x53, 0x22, 0x31,
				0xde, 0x2f, 0xac, 0x56, 0x31, 0x52, 0x20, 0x5e,
				0xc4, 0xf5, 0x9c, 0x74, txscript.OP_EQUAL,
			},
			bip16:         true,
			scriptInfoErr: txscript.ErrStackShortScript,
		},
		{
			// Invented scripts, the hashes do not match
			name: "p2sh standard script",
			sigScript: []byte{txscript.OP_TRUE,
				txscript.OP_DATA_1, 81,
				txscript.OP_DATA_25,
				txscript.OP_DUP, txscript.OP_HASH160,
				txscript.OP_DATA_20, 0x1, 0x2, 0x3, 0x4, 0x5,
				0x6, 0x7, 0x8, 0x9, 0xa, 0xb, 0xc, 0xd, 0xe,
				0xf, 0x10, 0x11, 0x12, 0x13, 0x14,
				txscript.OP_EQUALVERIFY, txscript.OP_CHECKSIG,
			},
			pkScript: []byte{txscript.OP_HASH160,
				txscript.OP_DATA_20,
				0xfe, 0x44, 0x10, 0x65, 0xb6, 0x53, 0x22, 0x31,
				0xde, 0x2f, 0xac, 0x56, 0x31, 0x52, 0x20, 0x5e,
				0xc4, 0xf5, 0x9c, 0x74, txscript.OP_EQUAL,
			},
			bip16: true,
			scriptInfo: txscript.ScriptInfo{
				PkScriptClass:  txscript.ScriptHashTy,
				NumInputs:      3,
				ExpectedInputs: 3, // nonstandard p2sh.
				SigOps:         1,
			},
		},
		{
			// from 567a53d1ce19ce3d07711885168484439965501536d0d0294c5d46d46c10e53b
			// from the blockchain.
			name: "p2sh nonstandard script",
			sigScript: []byte{txscript.OP_TRUE,
				txscript.OP_DATA_1, 81,
				txscript.OP_DATA_8,
				txscript.OP_2DUP, txscript.OP_EQUAL,
				txscript.OP_NOT, txscript.OP_VERIFY,
				txscript.OP_ABS, txscript.OP_SWAP,
				txscript.OP_ABS, txscript.OP_EQUAL,
			},
			pkScript: []byte{txscript.OP_HASH160,
				txscript.OP_DATA_20,
				0xfe, 0x44, 0x10, 0x65, 0xb6, 0x53, 0x22, 0x31,
				0xde, 0x2f, 0xac, 0x56, 0x31, 0x52, 0x20, 0x5e,
				0xc4, 0xf5, 0x9c, 0x74, txscript.OP_EQUAL,
			},
			bip16: true,
			scriptInfo: txscript.ScriptInfo{
				PkScriptClass:  txscript.ScriptHashTy,
				NumInputs:      3,
				ExpectedInputs: -1, // nonstandard p2sh.
				SigOps:         0,
			},
		},
		{
			// Script is invented, numbers all fake.
			name: "multisig script",
			sigScript: []byte{txscript.OP_TRUE,
				txscript.OP_TRUE, txscript.OP_TRUE,
				txscript.OP_0, // Extra arg for OP_CHECKMULTISIG bug
			},
			pkScript: []byte{
				txscript.OP_3, txscript.OP_DATA_33,
				0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9,
				0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10, 0x11, 0x12,
				0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a,
				0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20, 0x21,
				txscript.OP_DATA_33,
				0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9,
				0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10, 0x11, 0x12,
				0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a,
				0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20, 0x21,
				txscript.OP_DATA_33,
				0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9,
				0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10, 0x11, 0x12,
				0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a,
				0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20, 0x21,
				txscript.OP_3, txscript.OP_CHECKMULTISIG,
			},
			bip16: true,
			scriptInfo: txscript.ScriptInfo{
				PkScriptClass:  txscript.MultiSigTy,
				NumInputs:      4,
				ExpectedInputs: 4,
				SigOps:         3,
			},
		},
	}

	for _, test := range extraTests {
		si, err := txscript.CalcScriptInfo(test.sigScript,
			test.pkScript, test.bip16)
		if err != nil {
			if err != test.scriptInfoErr {
				t.Errorf("scriptinfo test \"%s\": got \"%v\""+
					"expected \"%v\"", test.name, err,
					test.scriptInfoErr)
			}
			continue
		}
		if test.scriptInfoErr != nil {
			t.Errorf("%s: succeeded when expecting \"%v\"",
				test.name, test.scriptInfoErr)
			continue
		}
		if *si != test.scriptInfo {
			t.Errorf("%s: scriptinfo doesn't match expected. "+
				"got: \"%v\" expected \"%v\"", test.name,
				*si, test.scriptInfo)
			continue
		}
	}

}

// bogusAddress implements the btcutil.Address interface so the tests can ensure
// unsupported address types are handled properly.
type bogusAddress struct{}

// EncodeAddress simply returns an empty string.  It exists to satsify the
// btcutil.Address interface.
func (b *bogusAddress) EncodeAddress() string {
	return ""
}

// ScriptAddress simply returns an empty byte slice.  It exists to satsify the
// btcutil.Address interface.
func (b *bogusAddress) ScriptAddress() []byte {
	return []byte{}
}

// IsForNet lies blatantly to satisfy the btcutil.Address interface.
func (b *bogusAddress) IsForNet(chainParams *chaincfg.Params) bool {
	return true // why not?
}

// String simply returns an empty string.  It exists to satsify the
// btcutil.Address interface.
func (b *bogusAddress) String() string {
	return ""
}

func TestPayToAddrScript(t *testing.T) {
	t.Parallel()

	// 1MirQ9bwyQcGVJPwKUgapu5ouK2E2Ey4gX
	p2pkhMain, err := btcutil.NewAddressPubKeyHash([]byte{
		0xe3, 0x4c, 0xce, 0x70, 0xc8, 0x63, 0x73, 0x27, 0x3e, 0xfc,
		0xc5, 0x4c, 0xe7, 0xd2, 0xa4, 0x91, 0xbb, 0x4a, 0x0e, 0x84,
	}, &chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create public key hash address: %v", err)
		return
	}

	// Taken from transaction:
	// b0539a45de13b3e0403909b8bd1a555b8cbe45fd4e3f3fda76f3a5f52835c29d
	p2shMain, _ := btcutil.NewAddressScriptHashFromHash([]byte{
		0xe8, 0xc3, 0x00, 0xc8, 0x79, 0x86, 0xef, 0xa8, 0x4c, 0x37,
		0xc0, 0x51, 0x99, 0x29, 0x01, 0x9e, 0xf8, 0x6e, 0xb5, 0xb4,
	}, &chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create script hash address: %v", err)
		return
	}

	//  mainnet p2pk 13CG6SJ3yHUXo4Cr2RY4THLLJrNFuG3gUg
	p2pkCompressedMain, err := btcutil.NewAddressPubKey([]byte{
		0x02, 0x19, 0x2d, 0x74, 0xd0, 0xcb, 0x94, 0x34, 0x4c, 0x95,
		0x69, 0xc2, 0xe7, 0x79, 0x01, 0x57, 0x3d, 0x8d, 0x79, 0x03,
		0xc3, 0xeb, 0xec, 0x3a, 0x95, 0x77, 0x24, 0x89, 0x5d, 0xca,
		0x52, 0xc6, 0xb4}, &chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create pubkey address (compressed): %v",
			err)
		return
	}
	p2pkCompressed2Main, err := btcutil.NewAddressPubKey([]byte{
		0x03, 0xb0, 0xbd, 0x63, 0x42, 0x34, 0xab, 0xbb, 0x1b, 0xa1,
		0xe9, 0x86, 0xe8, 0x84, 0x18, 0x5c, 0x61, 0xcf, 0x43, 0xe0,
		0x01, 0xf9, 0x13, 0x7f, 0x23, 0xc2, 0xc4, 0x09, 0x27, 0x3e,
		0xb1, 0x6e, 0x65}, &chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create pubkey address (compressed 2): %v",
			err)
		return
	}

	p2pkUncompressedMain, err := btcutil.NewAddressPubKey([]byte{
		0x04, 0x11, 0xdb, 0x93, 0xe1, 0xdc, 0xdb, 0x8a, 0x01, 0x6b,
		0x49, 0x84, 0x0f, 0x8c, 0x53, 0xbc, 0x1e, 0xb6, 0x8a, 0x38,
		0x2e, 0x97, 0xb1, 0x48, 0x2e, 0xca, 0xd7, 0xb1, 0x48, 0xa6,
		0x90, 0x9a, 0x5c, 0xb2, 0xe0, 0xea, 0xdd, 0xfb, 0x84, 0xcc,
		0xf9, 0x74, 0x44, 0x64, 0xf8, 0x2e, 0x16, 0x0b, 0xfa, 0x9b,
		0x8b, 0x64, 0xf9, 0xd4, 0xc0, 0x3f, 0x99, 0x9b, 0x86, 0x43,
		0xf6, 0x56, 0xb4, 0x12, 0xa3}, &chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create pubkey address (uncompressed): %v",
			err)
		return
	}

	tests := []struct {
		in       btcutil.Address
		expected []byte
		err      error
	}{
		// pay-to-pubkey-hash address on mainnet
		{
			p2pkhMain,
			[]byte{
				0x76, 0xa9, 0x14, 0xe3, 0x4c, 0xce, 0x70, 0xc8,
				0x63, 0x73, 0x27, 0x3e, 0xfc, 0xc5, 0x4c, 0xe7,
				0xd2, 0xa4, 0x91, 0xbb, 0x4a, 0x0e, 0x84, 0x88,
				0xac,
			},
			nil,
		},
		// pay-to-script-hash address on mainnet
		{
			p2shMain,
			[]byte{
				0xa9, 0x14, 0xe8, 0xc3, 0x00, 0xc8, 0x79, 0x86,
				0xef, 0xa8, 0x4c, 0x37, 0xc0, 0x51, 0x99, 0x29,
				0x01, 0x9e, 0xf8, 0x6e, 0xb5, 0xb4, 0x87,
			},
			nil,
		},
		// pay-to-pubkey address on mainnet. compressed key.
		{
			p2pkCompressedMain,
			[]byte{
				txscript.OP_DATA_33,
				0x02, 0x19, 0x2d, 0x74, 0xd0, 0xcb, 0x94, 0x34,
				0x4c, 0x95, 0x69, 0xc2, 0xe7, 0x79, 0x01, 0x57,
				0x3d, 0x8d, 0x79, 0x03, 0xc3, 0xeb, 0xec, 0x3a,
				0x95, 0x77, 0x24, 0x89, 0x5d, 0xca, 0x52, 0xc6,
				0xb4, txscript.OP_CHECKSIG,
			},
			nil,
		},
		// pay-to-pubkey address on mainnet. compressed key (other way).
		{
			p2pkCompressed2Main,
			[]byte{
				txscript.OP_DATA_33,
				0x03, 0xb0, 0xbd, 0x63, 0x42, 0x34, 0xab, 0xbb,
				0x1b, 0xa1, 0xe9, 0x86, 0xe8, 0x84, 0x18, 0x5c,
				0x61, 0xcf, 0x43, 0xe0, 0x01, 0xf9, 0x13, 0x7f,
				0x23, 0xc2, 0xc4, 0x09, 0x27, 0x3e, 0xb1, 0x6e,
				0x65, txscript.OP_CHECKSIG,
			},
			nil,
		},
		// pay-to-pubkey address on mainnet. uncompressed key.
		{
			p2pkUncompressedMain,
			[]byte{
				txscript.OP_DATA_65,
				0x04, 0x11, 0xdb, 0x93, 0xe1, 0xdc, 0xdb, 0x8a,
				0x01, 0x6b, 0x49, 0x84, 0x0f, 0x8c, 0x53, 0xbc,
				0x1e, 0xb6, 0x8a, 0x38, 0x2e, 0x97, 0xb1, 0x48,
				0x2e, 0xca, 0xd7, 0xb1, 0x48, 0xa6, 0x90, 0x9a,
				0x5c, 0xb2, 0xe0, 0xea, 0xdd, 0xfb, 0x84, 0xcc,
				0xf9, 0x74, 0x44, 0x64, 0xf8, 0x2e, 0x16, 0x0b,
				0xfa, 0x9b, 0x8b, 0x64, 0xf9, 0xd4, 0xc0, 0x3f,
				0x99, 0x9b, 0x86, 0x43, 0xf6, 0x56, 0xb4, 0x12,
				0xa3, txscript.OP_CHECKSIG,
			},
			nil,
		},

		// Supported address types with nil pointers.
		{(*btcutil.AddressPubKeyHash)(nil), []byte{}, txscript.ErrUnsupportedAddress},
		{(*btcutil.AddressScriptHash)(nil), []byte{}, txscript.ErrUnsupportedAddress},
		{(*btcutil.AddressPubKey)(nil), []byte{}, txscript.ErrUnsupportedAddress},

		// Unsupported address type.
		{&bogusAddress{}, []byte{}, txscript.ErrUnsupportedAddress},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		pkScript, err := txscript.PayToAddrScript(test.in)
		if err != test.err {
			t.Errorf("PayToAddrScript #%d unexpected error - "+
				"got %v, want %v", i, err, test.err)
			continue
		}

		if !bytes.Equal(pkScript, test.expected) {
			t.Errorf("PayToAddrScript #%d got: %x\nwant: %x",
				i, pkScript, test.expected)
			continue
		}
	}
}

func TestMultiSigScript(t *testing.T) {
	t.Parallel()

	//  mainnet p2pk 13CG6SJ3yHUXo4Cr2RY4THLLJrNFuG3gUg
	p2pkCompressedMain, err := btcutil.NewAddressPubKey([]byte{
		0x02, 0x19, 0x2d, 0x74, 0xd0, 0xcb, 0x94, 0x34, 0x4c, 0x95,
		0x69, 0xc2, 0xe7, 0x79, 0x01, 0x57, 0x3d, 0x8d, 0x79, 0x03,
		0xc3, 0xeb, 0xec, 0x3a, 0x95, 0x77, 0x24, 0x89, 0x5d, 0xca,
		0x52, 0xc6, 0xb4}, &chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create pubkey address (compressed): %v",
			err)
		return
	}
	p2pkCompressed2Main, err := btcutil.NewAddressPubKey([]byte{
		0x03, 0xb0, 0xbd, 0x63, 0x42, 0x34, 0xab, 0xbb, 0x1b, 0xa1,
		0xe9, 0x86, 0xe8, 0x84, 0x18, 0x5c, 0x61, 0xcf, 0x43, 0xe0,
		0x01, 0xf9, 0x13, 0x7f, 0x23, 0xc2, 0xc4, 0x09, 0x27, 0x3e,
		0xb1, 0x6e, 0x65}, &chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create pubkey address (compressed 2): %v",
			err)
		return
	}

	p2pkUncompressedMain, err := btcutil.NewAddressPubKey([]byte{
		0x04, 0x11, 0xdb, 0x93, 0xe1, 0xdc, 0xdb, 0x8a, 0x01, 0x6b,
		0x49, 0x84, 0x0f, 0x8c, 0x53, 0xbc, 0x1e, 0xb6, 0x8a, 0x38,
		0x2e, 0x97, 0xb1, 0x48, 0x2e, 0xca, 0xd7, 0xb1, 0x48, 0xa6,
		0x90, 0x9a, 0x5c, 0xb2, 0xe0, 0xea, 0xdd, 0xfb, 0x84, 0xcc,
		0xf9, 0x74, 0x44, 0x64, 0xf8, 0x2e, 0x16, 0x0b, 0xfa, 0x9b,
		0x8b, 0x64, 0xf9, 0xd4, 0xc0, 0x3f, 0x99, 0x9b, 0x86, 0x43,
		0xf6, 0x56, 0xb4, 0x12, 0xa3}, &chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create pubkey address (uncompressed): %v",
			err)
		return
	}

	tests := []struct {
		keys      []*btcutil.AddressPubKey
		nrequired int
		expected  []byte
		err       error
	}{
		{
			[]*btcutil.AddressPubKey{
				p2pkCompressedMain,
				p2pkCompressed2Main,
			},
			1,
			[]byte{
				txscript.OP_1,
				txscript.OP_DATA_33,
				0x02, 0x19, 0x2d, 0x74, 0xd0, 0xcb, 0x94, 0x34,
				0x4c, 0x95, 0x69, 0xc2, 0xe7, 0x79, 0x01, 0x57,
				0x3d, 0x8d, 0x79, 0x03, 0xc3, 0xeb, 0xec, 0x3a,
				0x95, 0x77, 0x24, 0x89, 0x5d, 0xca, 0x52, 0xc6,
				0xb4,
				txscript.OP_DATA_33,
				0x03, 0xb0, 0xbd, 0x63, 0x42, 0x34, 0xab, 0xbb,
				0x1b, 0xa1, 0xe9, 0x86, 0xe8, 0x84, 0x18, 0x5c,
				0x61, 0xcf, 0x43, 0xe0, 0x01, 0xf9, 0x13, 0x7f,
				0x23, 0xc2, 0xc4, 0x09, 0x27, 0x3e, 0xb1, 0x6e,
				0x65,
				txscript.OP_2, txscript.OP_CHECKMULTISIG,
			},
			nil,
		},
		{
			[]*btcutil.AddressPubKey{
				p2pkCompressedMain,
				p2pkCompressed2Main,
			},
			2,
			[]byte{
				txscript.OP_2,
				txscript.OP_DATA_33,
				0x02, 0x19, 0x2d, 0x74, 0xd0, 0xcb, 0x94, 0x34,
				0x4c, 0x95, 0x69, 0xc2, 0xe7, 0x79, 0x01, 0x57,
				0x3d, 0x8d, 0x79, 0x03, 0xc3, 0xeb, 0xec, 0x3a,
				0x95, 0x77, 0x24, 0x89, 0x5d, 0xca, 0x52, 0xc6,
				0xb4,
				txscript.OP_DATA_33,
				0x03, 0xb0, 0xbd, 0x63, 0x42, 0x34, 0xab, 0xbb,
				0x1b, 0xa1, 0xe9, 0x86, 0xe8, 0x84, 0x18, 0x5c,
				0x61, 0xcf, 0x43, 0xe0, 0x01, 0xf9, 0x13, 0x7f,
				0x23, 0xc2, 0xc4, 0x09, 0x27, 0x3e, 0xb1, 0x6e,
				0x65,
				txscript.OP_2, txscript.OP_CHECKMULTISIG,
			},
			nil,
		},
		{
			[]*btcutil.AddressPubKey{
				p2pkCompressedMain,
				p2pkCompressed2Main,
			},
			3,
			[]byte{},
			txscript.ErrBadNumRequired,
		},
		{
			[]*btcutil.AddressPubKey{
				p2pkUncompressedMain,
			},
			1,
			[]byte{
				txscript.OP_1, txscript.OP_DATA_65,
				0x04, 0x11, 0xdb, 0x93, 0xe1, 0xdc, 0xdb, 0x8a,
				0x01, 0x6b, 0x49, 0x84, 0x0f, 0x8c, 0x53, 0xbc,
				0x1e, 0xb6, 0x8a, 0x38, 0x2e, 0x97, 0xb1, 0x48,
				0x2e, 0xca, 0xd7, 0xb1, 0x48, 0xa6, 0x90, 0x9a,
				0x5c, 0xb2, 0xe0, 0xea, 0xdd, 0xfb, 0x84, 0xcc,
				0xf9, 0x74, 0x44, 0x64, 0xf8, 0x2e, 0x16, 0x0b,
				0xfa, 0x9b, 0x8b, 0x64, 0xf9, 0xd4, 0xc0, 0x3f,
				0x99, 0x9b, 0x86, 0x43, 0xf6, 0x56, 0xb4, 0x12,
				0xa3,
				txscript.OP_1, txscript.OP_CHECKMULTISIG,
			},
			nil,
		},
		{
			[]*btcutil.AddressPubKey{
				p2pkUncompressedMain,
			},
			2,
			[]byte{},
			txscript.ErrBadNumRequired,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		script, err := txscript.MultiSigScript(test.keys,
			test.nrequired)
		if err != test.err {
			t.Errorf("MultiSigScript #%d unexpected error - "+
				"got %v, want %v", i, err, test.err)
			continue
		}

		if !bytes.Equal(script, test.expected) {
			t.Errorf("MultiSigScript #%d got: %x\nwant: %x",
				i, script, test.expected)
			continue
		}
	}
}

func TestCalcMultiSigStats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		script   []byte
		expected error
	}{
		{
			name: "short script",
			script: []byte{
				0x04, 0x67, 0x08, 0xaf, 0xdb, 0x0f, 0xe5, 0x54,
				0x82, 0x71, 0x96, 0x7f, 0x1a, 0x67, 0x13, 0x0b,
				0x71, 0x05, 0xcd, 0x6a, 0x82, 0x8e, 0x03, 0x90,
				0x9a, 0x67, 0x96, 0x2e, 0x0e, 0xa1, 0xf6, 0x1d,
			},
			expected: txscript.ErrStackShortScript,
		},
		{
			name: "stack underflow",
			script: []byte{
				txscript.OP_RETURN,
				txscript.OP_PUSHDATA1,
				0x29,
				0x04, 0x67, 0x08, 0xaf, 0xdb, 0x0f, 0xe5, 0x54,
				0x82, 0x71, 0x96, 0x7f, 0x1a, 0x67, 0x13, 0x0b,
				0x71, 0x05, 0xcd, 0x6a, 0x82, 0x8e, 0x03, 0x90,
				0x9a, 0x67, 0x96, 0x2e, 0x0e, 0xa1, 0xf6, 0x1d,
				0xeb, 0x64, 0x9f, 0x6b, 0xc3, 0xf4, 0xce, 0xf3,
				0x08,
			},
			expected: txscript.ErrStackUnderflow,
		},
		{
			name: "multisig script",
			script: []uint8{
				txscript.OP_FALSE,
				txscript.OP_DATA_72,
				0x30, 0x45, 0x02, 0x20, 0x10,
				0x6a, 0x3e, 0x4e, 0xf0, 0xb5,
				0x1b, 0x76, 0x4a, 0x28, 0x87,
				0x22, 0x62, 0xff, 0xef, 0x55,
				0x84, 0x65, 0x14, 0xda, 0xcb,
				0xdc, 0xbb, 0xdd, 0x65, 0x2c,
				0x84, 0x9d, 0x39, 0x5b, 0x43,
				0x84, 0x02, 0x21, 0x00, 0xe0,
				0x3a, 0xe5, 0x54, 0xc3, 0xcb,
				0xb4, 0x06, 0x00, 0xd3, 0x1d,
				0xd4, 0x6f, 0xc3, 0x3f, 0x25,
				0xe4, 0x7b, 0xf8, 0x52, 0x5b,
				0x1f, 0xe0, 0x72, 0x82, 0xe3,
				0xb6, 0xec, 0xb5, 0xf3, 0xbb,
				0x28, 0x01,
				txscript.OP_CODESEPARATOR,
				txscript.OP_TRUE,
				txscript.OP_DATA_33,
				0x02, 0x32, 0xab, 0xdc, 0x89,
				0x3e, 0x7f, 0x06, 0x31, 0x36,
				0x4d, 0x7f, 0xd0, 0x1c, 0xb3,
				0x3d, 0x24, 0xda, 0x45, 0x32,
				0x9a, 0x00, 0x35, 0x7b, 0x3a,
				0x78, 0x86, 0x21, 0x1a, 0xb4,
				0x14, 0xd5, 0x5a,
				txscript.OP_TRUE,
				txscript.OP_CHECKMULTISIG,
			},
			expected: nil,
		},
	}

	for i, test := range tests {
		if _, _, err := txscript.CalcMultiSigStats(test.script); err != test.expected {
			t.Errorf("CalcMultiSigStats #%d (%s) wrong result\n"+
				"got: %v\nwant: %v", i, test.name, err,
				test.expected)
		}
	}
}

type scriptTypeTest struct {
	name       string
	script     []byte
	scripttype txscript.ScriptClass
}

var scriptTypeTests = []scriptTypeTest{
	// tx 0437cd7f8525ceed2324359c2d0ba26006d92d85.
	{
		name: "Pay Pubkey",
		script: []byte{
			txscript.OP_DATA_65,
			0x04, 0x11, 0xdb, 0x93, 0xe1, 0xdc, 0xdb, 0x8a, 0x01,
			0x6b, 0x49, 0x84, 0x0f, 0x8c, 0x53, 0xbc, 0x1e, 0xb6,
			0x8a, 0x38, 0x2e, 0x97, 0xb1, 0x48, 0x2e, 0xca, 0xd7,
			0xb1, 0x48, 0xa6, 0x90, 0x9a, 0x5c, 0xb2, 0xe0, 0xea,
			0xdd, 0xfb, 0x84, 0xcc, 0xf9, 0x74, 0x44, 0x64, 0xf8,
			0x2e, 0x16, 0x0b, 0xfa, 0x9b, 0x8b, 0x64, 0xf9, 0xd4,
			0xc0, 0x3f, 0x99, 0x9b, 0x86, 0x43, 0xf6, 0x56, 0xb4,
			0x12, 0xa3,
			txscript.OP_CHECKSIG,
		},
		scripttype: txscript.PubKeyTy,
	},
	// tx 599e47a8114fe098103663029548811d2651991b62397e057f0c863c2bc9f9ea
	{
		name: "Pay PubkeyHash",
		script: []byte{
			txscript.OP_DUP,
			txscript.OP_HASH160,
			txscript.OP_DATA_20,
			0x66, 0x0d, 0x4e, 0xf3, 0xa7, 0x43, 0xe3, 0xe6, 0x96,
			0xad, 0x99, 0x03, 0x64, 0xe5, 0x55, 0xc2, 0x71, 0xad,
			0x50, 0x4b,
			txscript.OP_EQUALVERIFY,
			txscript.OP_CHECKSIG,
		},
		scripttype: txscript.PubKeyHashTy,
	},
	// part of tx 6d36bc17e947ce00bb6f12f8e7a56a1585c5a36188ffa2b05e10b4743273a74b
	// codeseparator parts have been elided. (bitcoind's checks for multisig
	// type doesn't have codesep etc either.
	{
		name: "multisig",
		script: []byte{
			txscript.OP_TRUE,
			txscript.OP_DATA_33,
			0x02, 0x32, 0xab, 0xdc, 0x89, 0x3e, 0x7f, 0x06, 0x31,
			0x36, 0x4d, 0x7f, 0xd0, 0x1c, 0xb3, 0x3d, 0x24, 0xda,
			0x45, 0x32, 0x9a, 0x00, 0x35, 0x7b, 0x3a, 0x78, 0x86,
			0x21, 0x1a, 0xb4, 0x14, 0xd5, 0x5a,
			txscript.OP_TRUE,
			txscript.OP_CHECKMULTISIG,
		},
		scripttype: txscript.MultiSigTy,
	},
	// tx e5779b9e78f9650debc2893fd9636d827b26b4ddfa6a8172fe8708c924f5c39d
	// P2SH
	{
		name: "P2SH",
		script: []byte{
			txscript.OP_HASH160,
			txscript.OP_DATA_20,
			0x43, 0x3e, 0xc2, 0xac, 0x1f, 0xfa, 0x1b, 0x7b, 0x7d,
			0x02, 0x7f, 0x56, 0x45, 0x29, 0xc5, 0x71, 0x97, 0xf9,
			0xae, 0x88,
			txscript.OP_EQUAL,
		},
		scripttype: txscript.ScriptHashTy,
	},
	// Nulldata with no data at all.
	{
		name: "nulldata",
		script: []byte{
			txscript.OP_RETURN,
		},
		scripttype: txscript.NullDataTy,
	},
	// Nulldata with small data.
	{
		name: "nulldata2",
		script: []byte{
			txscript.OP_RETURN,
			txscript.OP_DATA_8,
			0x04, 0x67, 0x08, 0xaf, 0xdb, 0x0f, 0xe5, 0x54,
		},
		scripttype: txscript.NullDataTy,
	},
	// Nulldata with max allowed data.
	{
		name: "nulldata3",
		script: []byte{
			txscript.OP_RETURN,
			txscript.OP_PUSHDATA1,
			0x50, // 80
			0x04, 0x67, 0x08, 0xaf, 0xdb, 0x0f, 0xe5, 0x54,
			0x82, 0x71, 0x96, 0x7f, 0x1a, 0x67, 0x13, 0x0b,
			0x71, 0x05, 0xcd, 0x6a, 0x82, 0x8e, 0x03, 0x90,
			0x9a, 0x67, 0x96, 0x2e, 0x0e, 0xa1, 0xf6, 0x1d,
			0xeb, 0x64, 0x9f, 0x6b, 0xc3, 0xf4, 0xce, 0xf3,
			0x04, 0x67, 0x08, 0xaf, 0xdb, 0x0f, 0xe5, 0x54,
			0x82, 0x71, 0x96, 0x7f, 0x1a, 0x67, 0x13, 0x0b,
			0x71, 0x05, 0xcd, 0x6a, 0x82, 0x8e, 0x03, 0x90,
			0x9a, 0x67, 0x96, 0x2e, 0x0e, 0xa1, 0xf6, 0x1d,
			0xeb, 0x64, 0x9f, 0x6b, 0xc3, 0xf4, 0xce, 0xf3,
		},
		scripttype: txscript.NullDataTy,
	},
	// Nulldata with more than max allowed data (so therefore nonstandard)
	{
		name: "nulldata4",
		script: []byte{
			txscript.OP_RETURN,
			txscript.OP_PUSHDATA1,
			0x51, // 81
			0x04, 0x67, 0x08, 0xaf, 0xdb, 0x0f, 0xe5, 0x54,
			0x82, 0x71, 0x96, 0x7f, 0x1a, 0x67, 0x13, 0x0b,
			0x71, 0x05, 0xcd, 0x6a, 0x82, 0x8e, 0x03, 0x90,
			0x9a, 0x67, 0x96, 0x2e, 0x0e, 0xa1, 0xf6, 0x1d,
			0xeb, 0x64, 0x9f, 0x6b, 0xc3, 0xf4, 0xce, 0xf3,
			0x04, 0x67, 0x08, 0xaf, 0xdb, 0x0f, 0xe5, 0x54,
			0x82, 0x71, 0x96, 0x7f, 0x1a, 0x67, 0x13, 0x0b,
			0x71, 0x05, 0xcd, 0x6a, 0x82, 0x8e, 0x03, 0x90,
			0x9a, 0x67, 0x96, 0x2e, 0x0e, 0xa1, 0xf6, 0x1d,
			0xeb, 0x64, 0x9f, 0x6b, 0xc3, 0xf4, 0xce, 0xf3,
			0x08,
		},
		scripttype: txscript.NonStandardTy,
	},
	// Almost nulldata, but add an additional opcode after the data to make
	// it nonstandard.
	{
		name: "nulldata5",
		script: []byte{
			txscript.OP_RETURN,
			txscript.OP_DATA_1,
			0x04,
			txscript.OP_TRUE,
		},
		scripttype: txscript.NonStandardTy,
	}, // The next few are almost multisig (it is the more complex script type)
	// but with various changes to make it fail.
	{
		// multisig but funny nsigs..
		name: "strange 1",
		script: []byte{
			txscript.OP_DUP,
			txscript.OP_DATA_33,
			0x02, 0x32, 0xab, 0xdc, 0x89, 0x3e, 0x7f, 0x06, 0x31,
			0x36, 0x4d, 0x7f, 0xd0, 0x1c, 0xb3, 0x3d, 0x24, 0xda,
			0x45, 0x32, 0x9a, 0x00, 0x35, 0x7b, 0x3a, 0x78, 0x86,
			0x21, 0x1a, 0xb4, 0x14, 0xd5, 0x5a,
			txscript.OP_TRUE,
			txscript.OP_CHECKMULTISIG,
		},
		scripttype: txscript.NonStandardTy,
	},
	{
		name: "strange 2",
		// multisig but funny pubkey.
		script: []byte{
			txscript.OP_TRUE,
			txscript.OP_TRUE,
			txscript.OP_TRUE,
			txscript.OP_CHECKMULTISIG,
		},
		scripttype: txscript.NonStandardTy,
	},
	{
		name: "strange 3",
		// multisig but no matching npubkeys opcode.
		script: []byte{
			txscript.OP_TRUE,
			txscript.OP_DATA_33,
			0x02, 0x32, 0xab, 0xdc, 0x89, 0x3e, 0x7f, 0x06, 0x31,
			0x36, 0x4d, 0x7f, 0xd0, 0x1c, 0xb3, 0x3d, 0x24, 0xda,
			0x45, 0x32, 0x9a, 0x00, 0x35, 0x7b, 0x3a, 0x78, 0x86,
			0x21, 0x1a, 0xb4, 0x14, 0xd5, 0x5a,
			txscript.OP_DATA_33,
			0x02, 0x32, 0xab, 0xdc, 0x89, 0x3e, 0x7f, 0x06, 0x31,
			0x36, 0x4d, 0x7f, 0xd0, 0x1c, 0xb3, 0x3d, 0x24, 0xda,
			0x45, 0x32, 0x9a, 0x00, 0x35, 0x7b, 0x3a, 0x78, 0x86,
			0x21, 0x1a, 0xb4, 0x14, 0xd5, 0x5a,
			// No number.
			txscript.OP_CHECKMULTISIG,
		},
		scripttype: txscript.NonStandardTy,
	},
	{
		name: "strange 4",
		// multisig but with multisigverify
		script: []byte{
			txscript.OP_TRUE,
			txscript.OP_DATA_33,
			0x02, 0x32, 0xab, 0xdc, 0x89, 0x3e, 0x7f, 0x06, 0x31,
			0x36, 0x4d, 0x7f, 0xd0, 0x1c, 0xb3, 0x3d, 0x24, 0xda,
			0x45, 0x32, 0x9a, 0x00, 0x35, 0x7b, 0x3a, 0x78, 0x86,
			0x21, 0x1a, 0xb4, 0x14, 0xd5, 0x5a,
			txscript.OP_TRUE,
			txscript.OP_CHECKMULTISIGVERIFY,
		},
		scripttype: txscript.NonStandardTy,
	},
	{
		name: "strange 5",
		// multisig but wrong length.
		script: []byte{
			txscript.OP_TRUE,
			txscript.OP_CHECKMULTISIG,
		},
		scripttype: txscript.NonStandardTy,
	},
	{
		name: "doesn't parse",
		script: []byte{
			txscript.OP_DATA_5, 0x1, 0x2, 0x3, 0x4,
		},
		scripttype: txscript.NonStandardTy,
	},
}

func testScriptType(t *testing.T, test *scriptTypeTest) {
	scripttype := txscript.GetScriptClass(test.script)
	if scripttype != test.scripttype {
		t.Errorf("%s: expected %s got %s", test.name, test.scripttype,
			scripttype)
	}
}

func TestScriptTypes(t *testing.T) {
	t.Parallel()

	for i := range scriptTypeTests {
		testScriptType(t, &scriptTypeTests[i])
	}
}

func TestStringifyClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		scriptclass txscript.ScriptClass
		stringed    string
	}{
		{
			name:        "nonstandardty",
			scriptclass: txscript.NonStandardTy,
			stringed:    "nonstandard",
		},
		{
			name:        "pubkey",
			scriptclass: txscript.PubKeyTy,
			stringed:    "pubkey",
		},
		{
			name:        "pubkeyhash",
			scriptclass: txscript.PubKeyHashTy,
			stringed:    "pubkeyhash",
		},
		{
			name:        "scripthash",
			scriptclass: txscript.ScriptHashTy,
			stringed:    "scripthash",
		},
		{
			name:        "multisigty",
			scriptclass: txscript.MultiSigTy,
			stringed:    "multisig",
		},
		{
			name:        "nulldataty",
			scriptclass: txscript.NullDataTy,
			stringed:    "nulldata",
		},
		{
			name:        "broken",
			scriptclass: txscript.ScriptClass(255),
			stringed:    "Invalid",
		},
	}

	for _, test := range tests {
		typeString := test.scriptclass.String()
		if typeString != test.stringed {
			t.Errorf("%s: got \"%s\" expected \"%s\"", test.name,
				typeString, test.stringed)
		}
	}
}
