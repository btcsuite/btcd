// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript_test

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainec"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
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

// hexToBytes converts the passed hex string into bytes and will panic if there
// is an error.  This is only provided for the hard-coded constants so errors in
// the source code can be detected. It will only (and must only) be called with
// hard-coded values.
func hexToBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return b
}

// mustParseShortForm parses the passed short form script and returns the
// resulting bytes.  It panics if an error occurs.  This is only used in the
// tests as a helper since the only way it can fail is if there is an error in
// the test source code.
func mustParseShortForm(script string) []byte {
	s, err := parseShortForm(script)
	if err != nil {
		panic("invalid short form script in test source: err " +
			err.Error() + ", script: " + script)
	}

	return s
}

// newAddressPubKey returns a new dcrutil.AddressPubKey from the provided
// serialized public key.  It panics if an error occurs.  This is only used in
// the tests as a helper since the only way it can fail is if there is an error
// in the test source code.
func newAddressPubKey(serializedPubKey []byte) dcrutil.Address {
	pubkey, err := chainec.Secp256k1.ParsePubKey(serializedPubKey)
	if err != nil {
		panic("invalid public key in test source")
	}
	addr, err := dcrutil.NewAddressSecpPubKeyCompressed(pubkey,
		&chaincfg.MainNetParams)
	if err != nil {
		panic("invalid public key in test source")
	}

	return addr
}

// newAddressPubKeyHash returns a new dcrutil.AddressPubKeyHash from the
// provided hash.  It panics if an error occurs.  This is only used in the tests
// as a helper since the only way it can fail is if there is an error in the
// test source code.
func newAddressPubKeyHash(pkHash []byte) dcrutil.Address {
	addr, err := dcrutil.NewAddressPubKeyHash(pkHash, &chaincfg.MainNetParams,
		secp)
	if err != nil {
		panic("invalid public key hash in test source")
	}

	return addr
}

// newAddressScriptHash returns a new dcrutil.AddressScriptHash from the
// provided hash.  It panics if an error occurs.  This is only used in the tests
// as a helper since the only way it can fail is if there is an error in the
// test source code.
func newAddressScriptHash(scriptHash []byte) dcrutil.Address {
	addr, err := dcrutil.NewAddressScriptHashFromHash(scriptHash,
		&chaincfg.MainNetParams)
	if err != nil {
		panic("invalid script hash in test source")
	}

	return addr
}

// TestExtractPkScriptAddrs ensures that extracting the type, addresses, and
// number of required signatures from PkScripts works as intended.
func TestExtractPkScriptAddrs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		script  []byte
		addrs   []dcrutil.Address
		reqSigs int
		class   txscript.ScriptClass
		noparse bool
	}{
		{
			name: "standard p2pk with compressed pubkey (0x02)",
			script: decodeHex("2102192d74d0cb94344c9569c2e7790157" +
				"3d8d7903c3ebec3a957724895dca52c6b4ac"),
			addrs: []dcrutil.Address{
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
			addrs: []dcrutil.Address{
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
			addrs: []dcrutil.Address{
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
			addrs: []dcrutil.Address{
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
			addrs: []dcrutil.Address{
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
			addrs: []dcrutil.Address{
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
			addrs: []dcrutil.Address{
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
			addrs: []dcrutil.Address{
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
			addrs: []dcrutil.Address{
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
			addrs: []dcrutil.Address{
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
			addrs:   []dcrutil.Address{},
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
			addrs:   []dcrutil.Address{},
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
			noparse: true,
		},
	}

	t.Logf("Running %d tests.", len(tests))
	for i, test := range tests {
		class, addrs, reqSigs, err := txscript.ExtractPkScriptAddrs(
			txscript.DefaultScriptVersion, test.script,
			&chaincfg.MainNetParams)
		if err != nil && !test.noparse {
			t.Errorf("ExtractPkScriptAddrs #%d (%s): %v", i,
				test.name, err)
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

// TestCalcScriptInfo ensures the CalcScriptInfo provides the expected results
// for various valid and invalid script pairs.
func TestCalcScriptInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		sigScript     string
		pkScript      string
		bip16         bool
		scriptInfo    txscript.ScriptInfo
		scriptInfoErr error
	}{
		{
			// Invented scripts, the hashes do not match
			// Truncated version of test below:
			name: "pkscript doesn't parse",
			sigScript: "1 81 DATA_8 2DUP EQUAL NOT VERIFY ABS " +
				"SWAP ABS EQUAL",
			pkScript: "HASH160 DATA_20 0xfe441065b6532231de2fac56" +
				"3152205ec4f59c",
			bip16:         true,
			scriptInfoErr: txscript.ErrStackShortScript,
		},
		{
			name: "sigScript doesn't parse",
			// Truncated version of p2sh script below.
			sigScript: "1 81 DATA_8 2DUP EQUAL NOT VERIFY ABS " +
				"SWAP ABS",
			pkScript: "HASH160 DATA_20 0xfe441065b6532231de2fac56" +
				"3152205ec4f59c74 EQUAL",
			bip16:         true,
			scriptInfoErr: txscript.ErrStackShortScript,
		},
		{
			// Invented scripts, the hashes do not match
			name: "p2sh standard script",
			sigScript: "1 81 DATA_25 DUP HASH160 DATA_20 0x010203" +
				"0405060708090a0b0c0d0e0f1011121314 EQUALVERIFY " +
				"CHECKSIG",
			pkScript: "HASH160 DATA_20 0xfe441065b6532231de2fac56" +
				"3152205ec4f59c74 EQUAL",
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
			sigScript: "1 81 DATA_8 2DUP EQUAL NOT VERIFY ABS " +
				"SWAP ABS EQUAL",
			pkScript: "HASH160 DATA_20 0xfe441065b6532231de2fac56" +
				"3152205ec4f59c74 EQUAL",
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
			// Extra 0 arg on the end for OP_CHECKMULTISIG bug.
			sigScript: "1 1 1 0",
			pkScript: "3 " +
				"DATA_33 0x0102030405060708090a0b0c0d0e0f1011" +
				"12131415161718191a1b1c1d1e1f2021 DATA_33 " +
				"0x0102030405060708090a0b0c0d0e0f101112131415" +
				"161718191a1b1c1d1e1f2021 DATA_33 0x010203040" +
				"5060708090a0b0c0d0e0f101112131415161718191a1" +
				"b1c1d1e1f2021 3 CHECKMULTISIG",
			bip16: true,
			scriptInfo: txscript.ScriptInfo{
				PkScriptClass:  txscript.MultiSigTy,
				NumInputs:      4,
				ExpectedInputs: 3,
				SigOps:         3,
			},
		},
	}

	for _, test := range tests {
		sigScript := mustParseShortForm(test.sigScript)
		pkScript := mustParseShortForm(test.pkScript)
		si, err := txscript.CalcScriptInfo(sigScript, pkScript,
			test.bip16)
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

// bogusAddress implements the dcrutil.Address interface so the tests can ensure
// unsupported address types are handled properly.
type bogusAddress struct{}

// EncodeAddress simply returns an empty string.  It exists to satisfy the
// dcrutil.Address interface.
func (b *bogusAddress) EncodeAddress() string {
	return ""
}

// ScriptAddress simply returns an empty byte slice.  It exists to satisfy the
// dcrutil.Address interface.
func (b *bogusAddress) ScriptAddress() []byte {
	return nil
}

// Hash160 simply returns an empty byte slice.  It exists to satisfy the
// dcrutil.Address interface.
func (b *bogusAddress) Hash160() *[20]byte {
	return nil
}

// IsForNet lies blatantly to satisfy the dcrutil.Address interface.
func (b *bogusAddress) IsForNet(chainParams *chaincfg.Params) bool {
	return true // why not?
}

// String simply returns an empty string.  It exists to satisfy the
// dcrutil.Address interface.
func (b *bogusAddress) String() string {
	return ""
}

// DSA returns -1.
func (b *bogusAddress) DSA(chainParams *chaincfg.Params) int {
	return -1
}

// Net returns &chaincfg.TestNet2Params.
func (b *bogusAddress) Net() *chaincfg.Params {
	return &chaincfg.TestNet2Params
}

// TestPayToAddrScript ensures the PayToAddrScript function generates the
// correct scripts for the various types of addresses.
func TestPayToAddrScript(t *testing.T) {
	t.Parallel()

	// 1MirQ9bwyQcGVJPwKUgapu5ouK2E2Ey4gX
	p2pkhMain, err := dcrutil.NewAddressPubKeyHash(decodeHex("e34cce70c863"+
		"73273efcc54ce7d2a491bb4a0e84"), &chaincfg.MainNetParams, secp)
	if err != nil {
		t.Errorf("Unable to create public key hash address: %v", err)
		return
	}

	// Taken from transaction:
	// b0539a45de13b3e0403909b8bd1a555b8cbe45fd4e3f3fda76f3a5f52835c29d
	p2shMain, _ := dcrutil.NewAddressScriptHashFromHash(decodeHex("e8c300"+
		"c87986efa84c37c0519929019ef86eb5b4"), &chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create script hash address: %v", err)
		return
	}

	//  mainnet p2pk 13CG6SJ3yHUXo4Cr2RY4THLLJrNFuG3gUg
	p2pkCompressedMain, err := dcrutil.NewAddressSecpPubKey(decodeHex("02192d74"+
		"d0cb94344c9569c2e77901573d8d7903c3ebec3a957724895dca52c6b4"),
		&chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create pubkey address (compressed): %v",
			err)
		return
	}
	p2pkCompressed2Main, err := dcrutil.NewAddressSecpPubKey(decodeHex("03b0bd"+
		"634234abbb1ba1e986e884185c61cf43e001f9137f23c2c409273eb16e65"),
		&chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create pubkey address (compressed 2): %v",
			err)
		return
	}

	p2pkUncompressedMain := newAddressPubKey(decodeHex("0411db" +
		"93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2" +
		"e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3"))

	tests := []struct {
		in       dcrutil.Address
		expected string
		err      error
	}{
		// pay-to-pubkey-hash address on mainnet 0
		{
			p2pkhMain,
			"DUP HASH160 DATA_20 0xe34cce70c86373273efcc54ce7d2a4" +
				"91bb4a0e8488 CHECKSIG",
			nil,
		},
		// pay-to-script-hash address on mainnet 1
		{
			p2shMain,
			"HASH160 DATA_20 0xe8c300c87986efa84c37c0519929019ef8" +
				"6eb5b4 EQUAL",
			nil,
		},
		// pay-to-pubkey address on mainnet. compressed key. 2
		{
			p2pkCompressedMain,
			"DATA_33 0x02192d74d0cb94344c9569c2e77901573d8d7903c3" +
				"ebec3a957724895dca52c6b4 CHECKSIG",
			nil,
		},
		// pay-to-pubkey address on mainnet. compressed key (other way). 3
		{
			p2pkCompressed2Main,
			"DATA_33 0x03b0bd634234abbb1ba1e986e884185c61cf43e001" +
				"f9137f23c2c409273eb16e65 CHECKSIG",
			nil,
		},
		// pay-to-pubkey address on mainnet. for decred this would
		// be uncompressed, but standard for decred is 33 byte
		// compressed public keys.
		{
			p2pkUncompressedMain,
			"DATA_33 0x0311db93e1dcdb8a016b49840f8c53bc1eb68a382e97b" +
				"1482ecad7b148a6909a5cac",
			nil,
		},

		// Supported address types with nil pointers.
		{(*dcrutil.AddressPubKeyHash)(nil), "", txscript.ErrUnsupportedAddress},
		{(*dcrutil.AddressScriptHash)(nil), "", txscript.ErrUnsupportedAddress},
		{(*dcrutil.AddressSecpPubKey)(nil), "", txscript.ErrUnsupportedAddress},

		// Unsupported address type.
		{&bogusAddress{}, "", txscript.ErrUnsupportedAddress},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		pkScript, err := txscript.PayToAddrScript(test.in)
		if err != test.err {
			t.Errorf("PayToAddrScript #%d unexpected error - "+
				"got %v, want %v", i, err, test.err)
			continue
		}

		expected := mustParseShortForm(test.expected)
		if !bytes.Equal(pkScript, expected) {
			t.Errorf("PayToAddrScript #%d got: %x\nwant: %x",
				i, pkScript, expected)
			continue
		}
	}
}

// TestMultiSigScript ensures the MultiSigScript function returns the expected
// scripts and errors.
func TestMultiSigScript(t *testing.T) {
	t.Parallel()

	//  mainnet p2pk 13CG6SJ3yHUXo4Cr2RY4THLLJrNFuG3gUg
	p2pkCompressedMain, err := dcrutil.NewAddressSecpPubKey(decodeHex("02192d7"+
		"4d0cb94344c9569c2e77901573d8d7903c3ebec3a957724895dca52c6b4"),
		&chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create pubkey address (compressed): %v",
			err)
		return
	}
	p2pkCompressed2Main, err := dcrutil.NewAddressSecpPubKey(decodeHex("03b0bd"+
		"634234abbb1ba1e986e884185c61cf43e001f9137f23c2c409273eb16e65"),
		&chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create pubkey address (compressed 2): %v",
			err)
		return
	}

	p2pkUncompressedMain := newAddressPubKey(decodeHex("0411d" +
		"b93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c" +
		"b2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b41" +
		"2a3"))

	tests := []struct {
		keys      []*dcrutil.AddressSecpPubKey
		nrequired int
		expected  string
		err       error
	}{
		{
			[]*dcrutil.AddressSecpPubKey{
				p2pkCompressedMain,
				p2pkCompressed2Main,
			},
			1,
			"1 DATA_33 0x02192d74d0cb94344c9569c2e77901573d8d7903c" +
				"3ebec3a957724895dca52c6b4 DATA_33 0x03b0bd634" +
				"234abbb1ba1e986e884185c61cf43e001f9137f23c2c4" +
				"09273eb16e65 2 CHECKMULTISIG",
			nil,
		},
		{
			[]*dcrutil.AddressSecpPubKey{
				p2pkCompressedMain,
				p2pkCompressed2Main,
			},
			2,
			"2 DATA_33 0x02192d74d0cb94344c9569c2e77901573d8d7903c" +
				"3ebec3a957724895dca52c6b4 DATA_33 0x03b0bd634" +
				"234abbb1ba1e986e884185c61cf43e001f9137f23c2c4" +
				"09273eb16e65 2 CHECKMULTISIG",
			nil,
		},
		{
			[]*dcrutil.AddressSecpPubKey{
				p2pkCompressedMain,
				p2pkCompressed2Main,
			},
			3,
			"",
			txscript.ErrBadNumRequired,
		},
		{
			// By default compressed pubkeys are used in Decred.
			[]*dcrutil.AddressSecpPubKey{
				p2pkUncompressedMain.(*dcrutil.AddressSecpPubKey),
			},
			1,
			"1 DATA_33 0x0311db93e1dcdb8a016b49840f8c53bc1eb68a3" +
				"82e97b1482ecad7b148a6909a5c 1 CHECKMULTISIG",
			nil,
		},
		{
			[]*dcrutil.AddressSecpPubKey{
				p2pkUncompressedMain.(*dcrutil.AddressSecpPubKey),
			},
			2,
			"",
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

		expected := mustParseShortForm(test.expected)
		if !bytes.Equal(script, expected) {
			t.Errorf("MultiSigScript #%d got: %x\nwant: %x",
				i, script, expected)
			continue
		}
	}
}

// TestCalcMultiSigStats ensures the CalcMutliSigStats function returns the
// expected errors.
func TestCalcMultiSigStats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
		err    error
	}{
		{
			name: "short script",
			script: "0x046708afdb0fe5548271967f1a67130b7105cd6a828" +
				"e03909a67962e0ea1f61d",
			err: txscript.ErrStackShortScript,
		},
		{
			name: "stack underflow",
			script: "RETURN DATA_41 0x046708afdb0fe5548271967f1a" +
				"67130b7105cd6a828e03909a67962e0ea1f61deb649f6" +
				"bc3f4cef308",
			err: txscript.ErrStackUnderflow,
		},
		{
			name: "multisig script",
			script: "0 DATA_72 0x30450220106a3e4ef0b51b764a2887226" +
				"2ffef55846514dacbdcbbdd652c849d395b4384022100" +
				"e03ae554c3cbb40600d31dd46fc33f25e47bf8525b1fe" +
				"07282e3b6ecb5f3bb2801 CODESEPARATOR 1 DATA_33 " +
				"0x0232abdc893e7f0631364d7fd01cb33d24da45329a0" +
				"0357b3a7886211ab414d55a 1 CHECKMULTISIG",
			err: nil,
		},
	}

	for i, test := range tests {
		script := mustParseShortForm(test.script)
		if _, _, err := txscript.CalcMultiSigStats(script); err != test.err {
			t.Errorf("CalcMultiSigStats #%d (%s) unexpected "+
				"error\ngot: %v\nwant: %v", i, test.name, err,
				test.err)
		}
	}
}

// scriptClassTests houses several test scripts used to ensure various class
// determination is working as expected.  It's defined as a test global versus
// inside a function scope since this spans both the standard tests and the
// consensus tests (pay-to-script-hash is part of consensus).
var scriptClassTests = []struct {
	name   string
	script string
	class  txscript.ScriptClass
}{
	{
		name: "Pay Pubkey",
		script: "DATA_65 0x0411db93e1dcdb8a016b49840f8c53bc1eb68a382e" +
			"97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e16" +
			"0bfa9b8b64f9d4c03f999b8643f656b412a3 CHECKSIG",
		class: txscript.PubKeyTy,
	},
	// tx 599e47a8114fe098103663029548811d2651991b62397e057f0c863c2bc9f9ea
	{
		name: "Pay PubkeyHash",
		script: "DUP HASH160 DATA_20 0x660d4ef3a743e3e696ad990364e555" +
			"c271ad504b EQUALVERIFY CHECKSIG",
		class: txscript.PubKeyHashTy,
	},
	// part of tx 6d36bc17e947ce00bb6f12f8e7a56a1585c5a36188ffa2b05e10b4743273a74b
	// codeseparator parts have been elided. (bitcoin core's checks for
	// multisig type doesn't have codesep either).
	{
		name: "multisig",
		script: "1 DATA_33 0x0232abdc893e7f0631364d7fd01cb33d24da4" +
			"5329a00357b3a7886211ab414d55a 1 CHECKMULTISIG",
		class: txscript.MultiSigTy,
	},
	// tx e5779b9e78f9650debc2893fd9636d827b26b4ddfa6a8172fe8708c924f5c39d
	{
		name: "P2SH",
		script: "HASH160 DATA_20 0x433ec2ac1ffa1b7b7d027f564529c57197f" +
			"9ae88 EQUAL",
		class: txscript.ScriptHashTy,
	},

	{
		// Nulldata with no data at all.
		name:   "nulldata no data",
		script: "RETURN",
		class:  txscript.NullDataTy,
	},
	{
		// Nulldata with single zero push.
		name:   "nulldata zero",
		script: "RETURN 0",
		class:  txscript.NullDataTy,
	},
	{
		// Nulldata with small integer push.
		name:   "nulldata small int",
		script: "RETURN 1",
		class:  txscript.NullDataTy,
	},
	{
		// Nulldata with max small integer push.
		name:   "nulldata max small int",
		script: "RETURN 16",
		class:  txscript.NullDataTy,
	},
	{
		// Nulldata with small data push.
		name:   "nulldata small data",
		script: "RETURN DATA_8 0x046708afdb0fe554",
		class:  txscript.NullDataTy,
	},
	{
		// Canonical nulldata with 60-byte data push.
		name: "canonical nulldata 60-byte push",
		script: "RETURN 0x3c 0x046708afdb0fe5548271967f1a67130b7105cd" +
			"6a828e03909a67962e0ea1f61deb649f6bc3f4cef3046708afdb" +
			"0fe5548271967f1a67130b7105cd6a",
		class: txscript.NullDataTy,
	},
	{
		// Non-canonical nulldata with 60-byte data push.
		name: "non-canonical nulldata 60-byte push",
		script: "RETURN PUSHDATA1 0x3c 0x046708afdb0fe5548271967f1a67" +
			"130b7105cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef3" +
			"046708afdb0fe5548271967f1a67130b7105cd6a",
		class: txscript.NullDataTy,
	},
	{
		// Nulldata with max allowed data to be considered standard.
		name: "nulldata max standard push",
		script: "RETURN PUSHDATA1 0x50 0x046708afdb0fe5548271967f1a67" +
			"130b7105cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef3" +
			"046708afdb0fe5548271967f1a67130b7105cd6a828e03909a67" +
			"962e0ea1f61deb649f6bc3f4cef3",
		class: txscript.NullDataTy,
	},
	{
		// Nulldata with more than max allowed data to be considered
		// standard (so therefore nonstandard)
		name: "nulldata exceed max standard push",
		script: "RETURN PUSHDATA2 0x1801 0x046708afdb0fe5548271967f1a670" +
			"46708afdb0fe5548271967f1a67046708afdb0fe5548271967f1a670467" +
			"08afdb0fe5548271967f1a67046708afdb0fe5548271967f1a67046708a" +
			"fdb0fe5548271967f1a67046708afdb0fe5548271967f1a67046708afdb" +
			"0fe5548271967f1a67046708afdb0fe5548271967f1a67046708afdb0fe" +
			"5548271967f1a67",
		class: txscript.NonStandardTy,
	},
	{
		// Almost nulldata, but add an additional opcode after the data
		// to make it nonstandard.
		name:   "almost nulldata",
		script: "RETURN 4 TRUE",
		class:  txscript.NonStandardTy,
	},

	// The next few are almost multisig (it is the more complex script type)
	// but with various changes to make it fail.
	{
		// Multisig but invalid nsigs.
		name: "strange 1",
		script: "DUP DATA_33 0x0232abdc893e7f0631364d7fd01cb33d24da45" +
			"329a00357b3a7886211ab414d55a 1 CHECKMULTISIG",
		class: txscript.NonStandardTy,
	},
	{
		// Multisig but invalid pubkey.
		name:   "strange 2",
		script: "1 1 1 CHECKMULTISIG",
		class:  txscript.NonStandardTy,
	},
	{
		// Multisig but no matching npubkeys opcode.
		name: "strange 3",
		script: "1 DATA_33 0x0232abdc893e7f0631364d7fd01cb33d24da4532" +
			"9a00357b3a7886211ab414d55a DATA_33 0x0232abdc893e7f0" +
			"631364d7fd01cb33d24da45329a00357b3a7886211ab414d55a " +
			"CHECKMULTISIG",
		class: txscript.NonStandardTy,
	},
	{
		// Multisig but with multisigverify.
		name: "strange 4",
		script: "1 DATA_33 0x0232abdc893e7f0631364d7fd01cb33d24da4532" +
			"9a00357b3a7886211ab414d55a 1 CHECKMULTISIGVERIFY",
		class: txscript.NonStandardTy,
	},
	{
		// Multisig but wrong length.
		name:   "strange 5",
		script: "1 CHECKMULTISIG",
		class:  txscript.NonStandardTy,
	},
	{
		name:   "doesn't parse",
		script: "DATA_5 0x01020304",
		class:  txscript.NonStandardTy,
	},
	{
		name: "multisig script with wrong number of pubkeys",
		script: "2 " +
			"DATA_33 " +
			"0x027adf5df7c965a2d46203c781bd4dd8" +
			"21f11844136f6673af7cc5a4a05cd29380 " +
			"DATA_33 " +
			"0x02c08f3de8ee2de9be7bd770f4c10eb0" +
			"d6ff1dd81ee96eedd3a9d4aeaf86695e80 " +
			"3 CHECKMULTISIG",
		class: txscript.NonStandardTy,
	},
}

// TestScriptClass ensures all the scripts in scriptClassTests have the expected
// class.
func TestScriptClass(t *testing.T) {
	t.Parallel()

	for _, test := range scriptClassTests {
		script := mustParseShortForm(test.script)
		class := txscript.GetScriptClass(txscript.DefaultScriptVersion, script)
		if class != test.class {
			t.Errorf("%s: expected %s got %s (script %x)", test.name,
				test.class, class, script)
			return
		}
	}
}

// TestStringifyClass ensures the script class string returns the expected
// string for each script class.
func TestStringifyClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		class    txscript.ScriptClass
		stringed string
	}{
		{
			name:     "nonstandardty",
			class:    txscript.NonStandardTy,
			stringed: "nonstandard",
		},
		{
			name:     "pubkey",
			class:    txscript.PubKeyTy,
			stringed: "pubkey",
		},
		{
			name:     "pubkeyhash",
			class:    txscript.PubKeyHashTy,
			stringed: "pubkeyhash",
		},
		{
			name:     "scripthash",
			class:    txscript.ScriptHashTy,
			stringed: "scripthash",
		},
		{
			name:     "multisigty",
			class:    txscript.MultiSigTy,
			stringed: "multisig",
		},
		{
			name:     "nulldataty",
			class:    txscript.NullDataTy,
			stringed: "nulldata",
		},
		{
			name:     "broken",
			class:    txscript.ScriptClass(255),
			stringed: "Invalid",
		},
	}

	for _, test := range tests {
		typeString := test.class.String()
		if typeString != test.stringed {
			t.Errorf("%s: got %#q, want %#q", test.name,
				typeString, test.stringed)
		}
	}
}

// TestGenerateProvablyPruneableOut tests whether GenerateProvablyPruneableOut returns a valid script.
func TestGenerateProvablyPruneableOut(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected []byte
		err      error
		class    txscript.ScriptClass
	}{
		{
			name:     "small int",
			data:     hexToBytes("01"),
			expected: mustParseShortForm("RETURN 1"),
			err:      nil,
			class:    txscript.NullDataTy,
		},
		{
			name:     "max small int",
			data:     hexToBytes("10"),
			expected: mustParseShortForm("RETURN 16"),
			err:      nil,
			class:    txscript.NullDataTy,
		},
		{
			name: "data of size before OP_PUSHDATA1 is needed",
			data: hexToBytes("0102030405060708090a0b0c0d0e0f10111" +
				"2131415161718"),
			expected: mustParseShortForm("RETURN 0x18 0x01020304" +
				"05060708090a0b0c0d0e0f101112131415161718"),
			err:   nil,
			class: txscript.NullDataTy,
		},
		{
			name: "just right",
			data: hexToBytes("000102030405060708090a0b0c0d0e0f1011121" +
				"31415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f3" +
				"03132333435363738393a3b3c3d3e3f404142434445464748494a4b4c4" +
				"d4e4f202122232425262728292a2b2c2d2e2f303132333435363738393" +
				"a3b3c3d3e3f404142434445464748494a4b4c4d4e4f202122232425262" +
				"728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f404142434" +
				"445464748494a4b4c4d4e4f202122232425262728292a2b2c2d2e2f303" +
				"132333435363738393a3b3c3d3e3f404142434445464748494a4b4c4d4" +
				"e4f202122232425262728292a2b2c2d2e2f303132333435363738393a3" +
				"b3c3d3e"),
			expected: mustParseShortForm("RETURN PUSHDATA1 0xFF " +
				"0x000102030405060708090a0b0c0d0e0f101112131415161" +
				"718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f" +
				"303132333435363738393a3b3c3d3e3f40414243444546474" +
				"8494a4b4c4d4e4f202122232425262728292a2b2c2d2e2f30" +
				"3132333435363738393a3b3c3d3e3f4041424344454647484" +
				"94a4b4c4d4e4f202122232425262728292a2b2c2d2e2f3031" +
				"32333435363738393a3b3c3d3e3f404142434445464748494" +
				"a4b4c4d4e4f202122232425262728292a2b2c2d2e2f303132" +
				"333435363738393a3b3c3d3e3f404142434445464748494a4" +
				"b4c4d4e4f202122232425262728292a2b2c2d2e2f30313233" +
				"3435363738393a3b3c3d3e"),
			err:   nil,
			class: txscript.NullDataTy,
		},
		{
			name: "too big",
			data: hexToBytes("000102030405060708090a0b0c0d0e0f10111213141516" +
				"1718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132333435363" +
				"738393a3b3c3d3e3f404142434445464748494a4b4c4d4e4f2021222324252627" +
				"28292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f40414243444546474" +
				"8494a4b4c4d4e4f202122232425262728292a2b2c2d2e2f303132333435363738" +
				"393a3b3c3d3e3f404142434445464748494a4b4c4d4e4f2021222324252627282" +
				"92a2b2c2d2e2f303132333435363738393a3b3c3d3e3f40414243444546474849" +
				"4a4b4c4d4e4f202122232425262728292a2b2c2d2e2f303132333435363738393" +
				"a3b3c3d3e3f3f"),
			expected: nil,
			err:      txscript.ErrStackLongScript,
			class:    txscript.NonStandardTy,
		},
	}

	for i, test := range tests {
		script, err := txscript.GenerateProvablyPruneableOut(test.data)
		if err != test.err {
			t.Errorf("GenerateProvablyPruneableOut: #%d (%s) unexpected error: "+
				"got %v, want %v", i, test.name, err, test.err)
			continue
		}

		// Check that the expected result was returned.
		if !bytes.Equal(script, test.expected) {
			t.Errorf("GenerateProvablyPruneableOut: #%d (%s) wrong result\n"+
				"got: %x\nwant: %x", i, test.name, script,
				test.expected)
			continue
		}

		// Check that the script has the correct type.
		scriptType := txscript.GetScriptClass(txscript.DefaultScriptVersion, script)
		if scriptType != test.class {
			t.Errorf("GetScriptClass: #%d (%s) wrong result -- "+
				"got: %v, want: %v", i, test.name, scriptType,
				test.class)
			continue
		}
	}
}
