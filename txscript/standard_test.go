// Copyright (c) 2013-2020 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

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

// newAddressTaproot returns a new btcutil.AddressTaproot from the
// provided hash.  It panics if an error occurs.  This is only used in the tests
// as a helper since the only way it can fail is if there is an error in the
// test source code.
func newAddressTaproot(scriptHash []byte) btcutil.Address {
	addr, err := btcutil.NewAddressTaproot(scriptHash,
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
		addrs   []btcutil.Address
		reqSigs int
		class   ScriptClass
	}{
		{
			name: "standard p2pk with compressed pubkey (0x02)",
			script: hexToBytes("2102192d74d0cb94344c9569c2e779015" +
				"73d8d7903c3ebec3a957724895dca52c6b4ac"),
			addrs: []btcutil.Address{
				newAddressPubKey(hexToBytes("02192d74d0cb9434" +
					"4c9569c2e77901573d8d7903c3ebec3a9577" +
					"24895dca52c6b4")),
			},
			reqSigs: 1,
			class:   PubKeyTy,
		},
		{
			name: "standard p2pk with uncompressed pubkey (0x04)",
			script: hexToBytes("410411db93e1dcdb8a016b49840f8c53b" +
				"c1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddf" +
				"b84ccf9744464f82e160bfa9b8b64f9d4c03f999b864" +
				"3f656b412a3ac"),
			addrs: []btcutil.Address{
				newAddressPubKey(hexToBytes("0411db93e1dcdb8a" +
					"016b49840f8c53bc1eb68a382e97b1482eca" +
					"d7b148a6909a5cb2e0eaddfb84ccf9744464" +
					"f82e160bfa9b8b64f9d4c03f999b8643f656" +
					"b412a3")),
			},
			reqSigs: 1,
			class:   PubKeyTy,
		},
		{
			name: "standard p2pk with hybrid pubkey (0x06)",
			script: hexToBytes("4106192d74d0cb94344c9569c2e779015" +
				"73d8d7903c3ebec3a957724895dca52c6b40d4526483" +
				"8c0bd96852662ce6a847b197376830160c6d2eb5e6a4" +
				"c44d33f453eac"),
			addrs: []btcutil.Address{
				newAddressPubKey(hexToBytes("06192d74d0cb9434" +
					"4c9569c2e77901573d8d7903c3ebec3a9577" +
					"24895dca52c6b40d45264838c0bd96852662" +
					"ce6a847b197376830160c6d2eb5e6a4c44d3" +
					"3f453e")),
			},
			reqSigs: 1,
			class:   PubKeyTy,
		},
		{
			name: "standard p2pk with compressed pubkey (0x03)",
			script: hexToBytes("2103b0bd634234abbb1ba1e986e884185" +
				"c61cf43e001f9137f23c2c409273eb16e65ac"),
			addrs: []btcutil.Address{
				newAddressPubKey(hexToBytes("03b0bd634234abbb" +
					"1ba1e986e884185c61cf43e001f9137f23c2" +
					"c409273eb16e65")),
			},
			reqSigs: 1,
			class:   PubKeyTy,
		},
		{
			name: "2nd standard p2pk with uncompressed pubkey (0x04)",
			script: hexToBytes("4104b0bd634234abbb1ba1e986e884185" +
				"c61cf43e001f9137f23c2c409273eb16e6537a576782" +
				"eba668a7ef8bd3b3cfb1edb7117ab65129b8a2e681f3" +
				"c1e0908ef7bac"),
			addrs: []btcutil.Address{
				newAddressPubKey(hexToBytes("04b0bd634234abbb" +
					"1ba1e986e884185c61cf43e001f9137f23c2" +
					"c409273eb16e6537a576782eba668a7ef8bd" +
					"3b3cfb1edb7117ab65129b8a2e681f3c1e09" +
					"08ef7b")),
			},
			reqSigs: 1,
			class:   PubKeyTy,
		},
		{
			name: "standard p2pk with hybrid pubkey (0x07)",
			script: hexToBytes("4107b0bd634234abbb1ba1e986e884185" +
				"c61cf43e001f9137f23c2c409273eb16e6537a576782" +
				"eba668a7ef8bd3b3cfb1edb7117ab65129b8a2e681f3" +
				"c1e0908ef7bac"),
			addrs: []btcutil.Address{
				newAddressPubKey(hexToBytes("07b0bd634234abbb" +
					"1ba1e986e884185c61cf43e001f9137f23c2" +
					"c409273eb16e6537a576782eba668a7ef8bd" +
					"3b3cfb1edb7117ab65129b8a2e681f3c1e09" +
					"08ef7b")),
			},
			reqSigs: 1,
			class:   PubKeyTy,
		},
		{
			name: "standard p2pkh",
			script: hexToBytes("76a914ad06dd6ddee55cbca9a9e3713bd" +
				"7587509a3056488ac"),
			addrs: []btcutil.Address{
				newAddressPubKeyHash(hexToBytes("ad06dd6ddee5" +
					"5cbca9a9e3713bd7587509a30564")),
			},
			reqSigs: 1,
			class:   PubKeyHashTy,
		},
		{
			name: "standard p2sh",
			script: hexToBytes("a91463bcc565f9e68ee0189dd5cc67f1b" +
				"0e5f02f45cb87"),
			addrs: []btcutil.Address{
				newAddressScriptHash(hexToBytes("63bcc565f9e6" +
					"8ee0189dd5cc67f1b0e5f02f45cb")),
			},
			reqSigs: 1,
			class:   ScriptHashTy,
		},
		// from real tx 60a20bd93aa49ab4b28d514ec10b06e1829ce6818ec06cd3aabd013ebcdc4bb1, vout 0
		{
			name: "standard 1 of 2 multisig",
			script: hexToBytes("514104cc71eb30d653c0c3163990c47b9" +
				"76f3fb3f37cccdcbedb169a1dfef58bbfbfaff7d8a47" +
				"3e7e2e6d317b87bafe8bde97e3cf8f065dec022b51d1" +
				"1fcdd0d348ac4410461cbdcc5409fb4b4d42b51d3338" +
				"1354d80e550078cb532a34bfa2fcfdeb7d76519aecc6" +
				"2770f5b0e4ef8551946d8a540911abe3e7854a26f39f" +
				"58b25c15342af52ae"),
			addrs: []btcutil.Address{
				newAddressPubKey(hexToBytes("04cc71eb30d653c0" +
					"c3163990c47b976f3fb3f37cccdcbedb169a" +
					"1dfef58bbfbfaff7d8a473e7e2e6d317b87b" +
					"afe8bde97e3cf8f065dec022b51d11fcdd0d" +
					"348ac4")),
				newAddressPubKey(hexToBytes("0461cbdcc5409fb4" +
					"b4d42b51d33381354d80e550078cb532a34b" +
					"fa2fcfdeb7d76519aecc62770f5b0e4ef855" +
					"1946d8a540911abe3e7854a26f39f58b25c1" +
					"5342af")),
			},
			reqSigs: 1,
			class:   MultiSigTy,
		},
		// from real tx d646f82bd5fbdb94a36872ce460f97662b80c3050ad3209bef9d1e398ea277ab, vin 1
		{
			name: "standard 2 of 3 multisig",
			script: hexToBytes("524104cb9c3c222c5f7a7d3b9bd152f36" +
				"3a0b6d54c9eb312c4d4f9af1e8551b6c421a6a4ab0e2" +
				"9105f24de20ff463c1c91fcf3bf662cdde4783d4799f" +
				"787cb7c08869b4104ccc588420deeebea22a7e900cc8" +
				"b68620d2212c374604e3487ca08f1ff3ae12bdc63951" +
				"4d0ec8612a2d3c519f084d9a00cbbe3b53d071e9b09e" +
				"71e610b036aa24104ab47ad1939edcb3db65f7fedea6" +
				"2bbf781c5410d3f22a7a3a56ffefb2238af8627363bd" +
				"f2ed97c1f89784a1aecdb43384f11d2acc64443c7fc2" +
				"99cef0400421a53ae"),
			addrs: []btcutil.Address{
				newAddressPubKey(hexToBytes("04cb9c3c222c5f7a" +
					"7d3b9bd152f363a0b6d54c9eb312c4d4f9af" +
					"1e8551b6c421a6a4ab0e29105f24de20ff46" +
					"3c1c91fcf3bf662cdde4783d4799f787cb7c" +
					"08869b")),
				newAddressPubKey(hexToBytes("04ccc588420deeeb" +
					"ea22a7e900cc8b68620d2212c374604e3487" +
					"ca08f1ff3ae12bdc639514d0ec8612a2d3c5" +
					"19f084d9a00cbbe3b53d071e9b09e71e610b" +
					"036aa2")),
				newAddressPubKey(hexToBytes("04ab47ad1939edcb" +
					"3db65f7fedea62bbf781c5410d3f22a7a3a5" +
					"6ffefb2238af8627363bdf2ed97c1f89784a" +
					"1aecdb43384f11d2acc64443c7fc299cef04" +
					"00421a")),
			},
			reqSigs: 2,
			class:   MultiSigTy,
		},

		// The below are nonstandard script due to things such as
		// invalid pubkeys, failure to parse, and not being of a
		// standard form.

		{
			name: "p2pk with uncompressed pk missing OP_CHECKSIG",
			script: hexToBytes("410411db93e1dcdb8a016b49840f8c53b" +
				"c1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddf" +
				"b84ccf9744464f82e160bfa9b8b64f9d4c03f999b864" +
				"3f656b412a3"),
			addrs:   nil,
			reqSigs: 0,
			class:   NonStandardTy,
		},
		{
			name: "valid signature from a sigscript - no addresses",
			script: hexToBytes("47304402204e45e16932b8af514961a1d" +
				"3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd41022" +
				"0181522ec8eca07de4860a4acdd12909d831cc56cbba" +
				"c4622082221a8768d1d0901"),
			addrs:   nil,
			reqSigs: 0,
			class:   NonStandardTy,
		},
		// Note the technically the pubkey is the second item on the
		// stack, but since the address extraction intentionally only
		// works with standard PkScripts, this should not return any
		// addresses.
		{
			name: "valid sigscript to reedeem p2pk - no addresses",
			script: hexToBytes("493046022100ddc69738bf2336318e4e0" +
				"41a5a77f305da87428ab1606f023260017854350ddc0" +
				"22100817af09d2eec36862d16009852b7e3a0f6dd765" +
				"98290b7834e1453660367e07a014104cd4240c198e12" +
				"523b6f9cb9f5bed06de1ba37e96a1bbd13745fcf9d11" +
				"c25b1dff9a519675d198804ba9962d3eca2d5937d58e" +
				"5a75a71042d40388a4d307f887d"),
			addrs:   nil,
			reqSigs: 0,
			class:   NonStandardTy,
		},
		// from real tx 691dd277dc0e90a462a3d652a1171686de49cf19067cd33c7df0392833fb986a, vout 0
		// invalid public keys
		{
			name: "1 of 3 multisig with invalid pubkeys",
			script: hexToBytes("51411c2200007353455857696b696c656" +
				"16b73204361626c6567617465204261636b75700a0a6" +
				"361626c65676174652d3230313031323034313831312" +
				"e377a0a0a446f41776e6c6f61642074686520666f6c6" +
				"c6f77696e67207472616e73616374696f6e732077697" +
				"468205361746f736869204e616b616d6f746f2773206" +
				"46f776e6c6f61416420746f6f6c2077686963680a636" +
				"16e20626520666f756e6420696e207472616e7361637" +
				"4696f6e2036633533636439383731313965663739376" +
				"435616463636453ae"),
			addrs:   []btcutil.Address{},
			reqSigs: 1,
			class:   MultiSigTy,
		},
		{
			name: "v1 p2tr witness-script-hash",
			script: hexToBytes("51201a82f7457a9ba6ab1074e9f50" +
				"053eefc637f8b046e389b636766bdc7d1f676f8"),
			addrs: []btcutil.Address{newAddressTaproot(
				hexToBytes("1a82f7457a9ba6ab1074e9f50053eefc6" +
					"37f8b046e389b636766bdc7d1f676f8"))},
			reqSigs: 1,
			class:   WitnessV1TaprootTy,
		},
		{
			name: "1 of 3 multisig with invalid pubkeys 2",
			script: hexToBytes("514134633365633235396337346461636" +
				"536666430383862343463656638630a6336366263313" +
				"93936633862393461333831316233363536313866653" +
				"16539623162354136636163636539393361333938386" +
				"134363966636336643664616266640a3236363363666" +
				"13963663463303363363039633539336333653931666" +
				"56465373032392131323364643432643235363339643" +
				"338613663663530616234636434340a00000053ae"),
			addrs:   []btcutil.Address{},
			reqSigs: 1,
			class:   MultiSigTy,
		},
		{
			name:    "empty script",
			script:  []byte{},
			addrs:   nil,
			reqSigs: 0,
			class:   NonStandardTy,
		},
		{
			name:    "script that does not parse",
			script:  []byte{OP_DATA_45},
			addrs:   nil,
			reqSigs: 0,
			class:   NonStandardTy,
		},
	}

	t.Logf("Running %d tests.", len(tests))
	for i, test := range tests {
		class, addrs, reqSigs, err := ExtractPkScriptAddrs(
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

// TestCalcScriptInfo ensures the CalcScriptInfo provides the expected results
// for various valid and invalid script pairs.
func TestCalcScriptInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sigScript string
		pkScript  string
		witness   []string

		bip16  bool
		segwit bool

		scriptInfo    ScriptInfo
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
			scriptInfoErr: scriptError(ErrMalformedPush, ""),
		},
		{
			name: "sigScript doesn't parse",
			// Truncated version of p2sh script below.
			sigScript: "1 81 DATA_8 2DUP EQUAL NOT VERIFY ABS " +
				"SWAP ABS",
			pkScript: "HASH160 DATA_20 0xfe441065b6532231de2fac56" +
				"3152205ec4f59c74 EQUAL",
			bip16:         true,
			scriptInfoErr: scriptError(ErrMalformedPush, ""),
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
			scriptInfo: ScriptInfo{
				PkScriptClass:  ScriptHashTy,
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
			scriptInfo: ScriptInfo{
				PkScriptClass:  ScriptHashTy,
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
			scriptInfo: ScriptInfo{
				PkScriptClass:  MultiSigTy,
				NumInputs:      4,
				ExpectedInputs: 4,
				SigOps:         3,
			},
		},
		{
			// A v0 p2wkh spend.
			name:     "p2wkh script",
			pkScript: "OP_0 DATA_20 0x365ab47888e150ff46f8d51bce36dcd680f1283f",
			witness: []string{
				"3045022100ee9fe8f9487afa977" +
					"6647ebcf0883ce0cd37454d7ce19889d34ba2c9" +
					"9ce5a9f402200341cb469d0efd3955acb9e46" +
					"f568d7e2cc10f9084aaff94ced6dc50a59134ad01",
				"03f0000d0639a22bfaf217e4c9428" +
					"9c2b0cc7fa1036f7fd5d9f61a9d6ec153100e",
			},
			segwit: true,
			scriptInfo: ScriptInfo{
				PkScriptClass:  WitnessV0PubKeyHashTy,
				NumInputs:      2,
				ExpectedInputs: 2,
				SigOps:         1,
			},
		},
		{
			// Nested p2sh v0
			name: "p2wkh nested inside p2sh",
			pkScript: "HASH160 DATA_20 " +
				"0xb3a84b564602a9d68b4c9f19c2ea61458ff7826c EQUAL",
			sigScript: "DATA_22 0x0014ad0ffa2e387f07e7ead14dc56d5a97dbd6ff5a23",
			witness: []string{
				"3045022100cb1c2ac1ff1d57d" +
					"db98f7bdead905f8bf5bcc8641b029ce8eef25" +
					"c75a9e22a4702203be621b5c86b771288706be5" +
					"a7eee1db4fceabf9afb7583c1cc6ee3f8297b21201",
				"03f0000d0639a22bfaf217e4c9" +
					"4289c2b0cc7fa1036f7fd5d9f61a9d6ec153100e",
			},
			segwit: true,
			bip16:  true,
			scriptInfo: ScriptInfo{
				PkScriptClass:  ScriptHashTy,
				NumInputs:      3,
				ExpectedInputs: 3,
				SigOps:         1,
			},
		},
		{
			// A v0 p2wsh spend.
			name: "p2wsh spend of a p2wkh witness script",
			pkScript: "0 DATA_32 0xe112b88a0cd87ba387f44" +
				"9d443ee2596eb353beb1f0351ab2cba8909d875db23",
			witness: []string{
				"3045022100cb1c2ac1ff1d57d" +
					"db98f7bdead905f8bf5bcc8641b029ce8eef25" +
					"c75a9e22a4702203be621b5c86b771288706be5" +
					"a7eee1db4fceabf9afb7583c1cc6ee3f8297b21201",
				"03f0000d0639a22bfaf217e4c9" +
					"4289c2b0cc7fa1036f7fd5d9f61a9d6ec153100e",
				"76a914064977cb7b4a2e0c9680df0ef696e9e0e296b39988ac",
			},
			segwit: true,
			scriptInfo: ScriptInfo{
				PkScriptClass:  WitnessV0ScriptHashTy,
				NumInputs:      3,
				ExpectedInputs: 3,
				SigOps:         1,
			},
		},
	}

	for _, test := range tests {
		sigScript := mustParseShortForm(test.sigScript)
		pkScript := mustParseShortForm(test.pkScript)

		var witness wire.TxWitness

		for _, witElement := range test.witness {
			wit, err := hex.DecodeString(witElement)
			if err != nil {
				t.Fatalf("unable to decode witness "+
					"element: %v", err)
			}

			witness = append(witness, wit)
		}

		si, err := CalcScriptInfo(sigScript, pkScript, witness,
			test.bip16, test.segwit)
		if e := tstCheckScriptError(err, test.scriptInfoErr); e != nil {
			t.Errorf("scriptinfo test %q: %v", test.name, e)
			continue
		}
		if err != nil {
			continue
		}

		if *si != test.scriptInfo {
			t.Errorf("%s: scriptinfo doesn't match expected. "+
				"got: %q expected %q", test.name, *si,
				test.scriptInfo)
			continue
		}
	}
}

// bogusAddress implements the btcutil.Address interface so the tests can ensure
// unsupported address types are handled properly.
type bogusAddress struct{}

// EncodeAddress simply returns an empty string.  It exists to satisfy the
// btcutil.Address interface.
func (b *bogusAddress) EncodeAddress() string {
	return ""
}

// ScriptAddress simply returns an empty byte slice.  It exists to satisfy the
// btcutil.Address interface.
func (b *bogusAddress) ScriptAddress() []byte {
	return nil
}

// IsForNet lies blatantly to satisfy the btcutil.Address interface.
func (b *bogusAddress) IsForNet(chainParams *chaincfg.Params) bool {
	return true // why not?
}

// String simply returns an empty string.  It exists to satisfy the
// btcutil.Address interface.
func (b *bogusAddress) String() string {
	return ""
}

// TestPayToAddrScript ensures the PayToAddrScript function generates the
// correct scripts for the various types of addresses.
func TestPayToAddrScript(t *testing.T) {
	t.Parallel()

	// 1MirQ9bwyQcGVJPwKUgapu5ouK2E2Ey4gX
	p2pkhMain, err := btcutil.NewAddressPubKeyHash(hexToBytes("e34cce70c86"+
		"373273efcc54ce7d2a491bb4a0e84"), &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Unable to create public key hash address: %v", err)
	}

	// Taken from transaction:
	// b0539a45de13b3e0403909b8bd1a555b8cbe45fd4e3f3fda76f3a5f52835c29d
	p2shMain, _ := btcutil.NewAddressScriptHashFromHash(hexToBytes("e8c300"+
		"c87986efa84c37c0519929019ef86eb5b4"), &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Unable to create script hash address: %v", err)
	}

	//  mainnet p2pk 13CG6SJ3yHUXo4Cr2RY4THLLJrNFuG3gUg
	p2pkCompressedMain, err := btcutil.NewAddressPubKey(hexToBytes("02192d"+
		"74d0cb94344c9569c2e77901573d8d7903c3ebec3a957724895dca52c6b4"),
		&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Unable to create pubkey address (compressed): %v",
			err)
	}
	p2pkCompressed2Main, err := btcutil.NewAddressPubKey(hexToBytes("03b0b"+
		"d634234abbb1ba1e986e884185c61cf43e001f9137f23c2c409273eb16e65"),
		&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Unable to create pubkey address (compressed 2): %v",
			err)
	}

	p2pkUncompressedMain, err := btcutil.NewAddressPubKey(hexToBytes("0411"+
		"db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5"+
		"cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b4"+
		"12a3"), &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Unable to create pubkey address (uncompressed): %v",
			err)
	}

	p2wsh, err := btcutil.NewAddressWitnessScriptHash(hexToBytes("e981bd992a43650657"+
		"d705ef7a30b2adc75a927ed42a4cf6b3da0f865a475fb4"), &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Unable to create p2wsh address: %v",
			err)
	}

	p2tr, err := btcutil.NewAddressTaproot(hexToBytes("3a8e170b546c3b122ab9c175e"+
		"ff36fb344db2684fe96497eb51b440e75232709"), &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Unable to create p2tr address: %v",
			err)
	}

	p2wpkh, err := btcutil.NewAddressWitnessPubKeyHash(hexToBytes("748e50366adb8"+
		"ae4b0255e406a28f99d24b73cbc"), &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Unable to create p2wpkh address: %v",
			err)
	}

	// Errors used in the tests below defined here for convenience and to
	// keep the horizontal test size shorter.
	errUnsupportedAddress := scriptError(ErrUnsupportedAddress, "")

	tests := []struct {
		in       btcutil.Address
		expected string
		err      error
	}{
		// pay-to-pubkey-hash address on mainnet
		{
			p2pkhMain,
			"DUP HASH160 DATA_20 0xe34cce70c86373273efcc54ce7d2a4" +
				"91bb4a0e8488 CHECKSIG",
			nil,
		},
		// pay-to-script-hash address on mainnet
		{
			p2shMain,
			"HASH160 DATA_20 0xe8c300c87986efa84c37c0519929019ef8" +
				"6eb5b4 EQUAL",
			nil,
		},
		// pay-to-pubkey address on mainnet. compressed key.
		{
			p2pkCompressedMain,
			"DATA_33 0x02192d74d0cb94344c9569c2e77901573d8d7903c3" +
				"ebec3a957724895dca52c6b4 CHECKSIG",
			nil,
		},
		// pay-to-pubkey address on mainnet. compressed key (other way).
		{
			p2pkCompressed2Main,
			"DATA_33 0x03b0bd634234abbb1ba1e986e884185c61cf43e001" +
				"f9137f23c2c409273eb16e65 CHECKSIG",
			nil,
		},
		// pay-to-pubkey address on mainnet. uncompressed key.
		{
			p2pkUncompressedMain,
			"DATA_65 0x0411db93e1dcdb8a016b49840f8c53bc1eb68a382e" +
				"97b1482ecad7b148a6909a5cb2e0eaddfb84ccf97444" +
				"64f82e160bfa9b8b64f9d4c03f999b8643f656b412a3 " +
				"CHECKSIG",
			nil,
		},
		// pay-to-witness-script-hash address on mainnet.
		{
			p2wsh,
			"OP_0 DATA_32 0xe981bd992a43650657d705ef7a30b2adc75a927ed" +
				"42a4cf6b3da0f865a475fb4",
			nil,
		},
		// pay-to-taproot address on mainnet.
		{
			p2tr,
			"OP_1 DATA_32 0x3a8e170b546c3b122ab9c175eff36fb344db2684" +
				"fe96497eb51b440e75232709",
			nil,
		},
		// pay-to-witness-pubkey-hash address on mainnet.
		{
			p2wpkh,
			"OP_0 DATA_20 0x748e50366adb8ae4b0255e406a28f99d24b73cbc",
			nil,
		},

		// Supported address types with nil pointers.
		{(*btcutil.AddressPubKeyHash)(nil), "", errUnsupportedAddress},
		{(*btcutil.AddressScriptHash)(nil), "", errUnsupportedAddress},
		{(*btcutil.AddressPubKey)(nil), "", errUnsupportedAddress},
		{(*btcutil.AddressWitnessPubKeyHash)(nil), "", errUnsupportedAddress},
		{(*btcutil.AddressWitnessScriptHash)(nil), "", errUnsupportedAddress},
		{(*btcutil.AddressTaproot)(nil), "", errUnsupportedAddress},

		// Unsupported address type.
		{&bogusAddress{}, "", errUnsupportedAddress},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		pkScript, err := PayToAddrScript(test.in)
		if e := tstCheckScriptError(err, test.err); e != nil {
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
	p2pkCompressedMain, err := btcutil.NewAddressPubKey(hexToBytes("02192d"+
		"74d0cb94344c9569c2e77901573d8d7903c3ebec3a957724895dca52c6b4"),
		&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Unable to create pubkey address (compressed): %v",
			err)
	}
	p2pkCompressed2Main, err := btcutil.NewAddressPubKey(hexToBytes("03b0b"+
		"d634234abbb1ba1e986e884185c61cf43e001f9137f23c2c409273eb16e65"),
		&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Unable to create pubkey address (compressed 2): %v",
			err)
	}

	p2pkUncompressedMain, err := btcutil.NewAddressPubKey(hexToBytes("0411"+
		"db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5"+
		"cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b4"+
		"12a3"), &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Unable to create pubkey address (uncompressed): %v",
			err)
	}

	tests := []struct {
		keys      []*btcutil.AddressPubKey
		nrequired int
		expected  string
		err       error
	}{
		{
			[]*btcutil.AddressPubKey{
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
			[]*btcutil.AddressPubKey{
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
			[]*btcutil.AddressPubKey{
				p2pkCompressedMain,
				p2pkCompressed2Main,
			},
			3,
			"",
			scriptError(ErrTooManyRequiredSigs, ""),
		},
		{
			[]*btcutil.AddressPubKey{
				p2pkUncompressedMain,
			},
			1,
			"1 DATA_65 0x0411db93e1dcdb8a016b49840f8c53bc1eb68a382" +
				"e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf97444" +
				"64f82e160bfa9b8b64f9d4c03f999b8643f656b412a3 " +
				"1 CHECKMULTISIG",
			nil,
		},
		{
			[]*btcutil.AddressPubKey{
				p2pkUncompressedMain,
			},
			2,
			"",
			scriptError(ErrTooManyRequiredSigs, ""),
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		script, err := MultiSigScript(test.keys, test.nrequired)
		if e := tstCheckScriptError(err, test.err); e != nil {
			t.Errorf("MultiSigScript #%d: %v", i, e)
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
			err: scriptError(ErrNotMultisigScript, ""),
		},
		{
			name: "stack underflow",
			script: "RETURN DATA_41 0x046708afdb0fe5548271967f1a" +
				"67130b7105cd6a828e03909a67962e0ea1f61deb649f6" +
				"bc3f4cef308",
			err: scriptError(ErrNotMultisigScript, ""),
		},
		{
			name: "multisig script",
			script: "1 DATA_33 0x0232abdc893e7f0631364d7fd01cb33d24da45329a0" +
				"0357b3a7886211ab414d55a 1 CHECKMULTISIG",
			err: nil,
		},
	}

	for i, test := range tests {
		script := mustParseShortForm(test.script)
		_, _, err := CalcMultiSigStats(script)
		if e := tstCheckScriptError(err, test.err); e != nil {
			t.Errorf("CalcMultiSigStats #%d (%s): %v", i, test.name,
				e)
			continue
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
	class  ScriptClass
}{
	{
		name: "Pay Pubkey",
		script: "DATA_65 0x0411db93e1dcdb8a016b49840f8c53bc1eb68a382e" +
			"97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e16" +
			"0bfa9b8b64f9d4c03f999b8643f656b412a3 CHECKSIG",
		class: PubKeyTy,
	},
	// tx 599e47a8114fe098103663029548811d2651991b62397e057f0c863c2bc9f9ea
	{
		name: "Pay PubkeyHash",
		script: "DUP HASH160 DATA_20 0x660d4ef3a743e3e696ad990364e555" +
			"c271ad504b EQUALVERIFY CHECKSIG",
		class: PubKeyHashTy,
	},
	// part of tx 6d36bc17e947ce00bb6f12f8e7a56a1585c5a36188ffa2b05e10b4743273a74b
	// codeseparator parts have been elided. (bitcoin core's checks for
	// multisig type doesn't have codesep either).
	{
		name: "multisig",
		script: "1 DATA_33 0x0232abdc893e7f0631364d7fd01cb33d24da4" +
			"5329a00357b3a7886211ab414d55a 1 CHECKMULTISIG",
		class: MultiSigTy,
	},
	// tx e5779b9e78f9650debc2893fd9636d827b26b4ddfa6a8172fe8708c924f5c39d
	{
		name: "P2SH",
		script: "HASH160 DATA_20 0x433ec2ac1ffa1b7b7d027f564529c57197f" +
			"9ae88 EQUAL",
		class: ScriptHashTy,
	},

	{
		// Nulldata with no data at all.
		name:   "nulldata no data",
		script: "RETURN",
		class:  NullDataTy,
	},
	{
		// Nulldata with single zero push.
		name:   "nulldata zero",
		script: "RETURN 0",
		class:  NullDataTy,
	},
	{
		// Nulldata with small integer push.
		name:   "nulldata small int",
		script: "RETURN 1",
		class:  NullDataTy,
	},
	{
		// Nulldata with max small integer push.
		name:   "nulldata max small int",
		script: "RETURN 16",
		class:  NullDataTy,
	},
	{
		// Nulldata with small data push.
		name:   "nulldata small data",
		script: "RETURN DATA_8 0x046708afdb0fe554",
		class:  NullDataTy,
	},
	{
		// Canonical nulldata with 60-byte data push.
		name: "canonical nulldata 60-byte push",
		script: "RETURN 0x3c 0x046708afdb0fe5548271967f1a67130b7105cd" +
			"6a828e03909a67962e0ea1f61deb649f6bc3f4cef3046708afdb" +
			"0fe5548271967f1a67130b7105cd6a",
		class: NullDataTy,
	},
	{
		// Non-canonical nulldata with 60-byte data push.
		name: "non-canonical nulldata 60-byte push",
		script: "RETURN PUSHDATA1 0x3c 0x046708afdb0fe5548271967f1a67" +
			"130b7105cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef3" +
			"046708afdb0fe5548271967f1a67130b7105cd6a",
		class: NullDataTy,
	},
	{
		// Nulldata with max allowed data to be considered standard.
		name: "nulldata max standard push",
		script: "RETURN PUSHDATA1 0x50 0x046708afdb0fe5548271967f1a67" +
			"130b7105cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef3" +
			"046708afdb0fe5548271967f1a67130b7105cd6a828e03909a67" +
			"962e0ea1f61deb649f6bc3f4cef3",
		class: NullDataTy,
	},
	{
		// Nulldata with more than max allowed data to be considered
		// standard (so therefore nonstandard)
		name: "nulldata exceed max standard push",
		script: "RETURN PUSHDATA1 0x51 0x046708afdb0fe5548271967f1a67" +
			"130b7105cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef3" +
			"046708afdb0fe5548271967f1a67130b7105cd6a828e03909a67" +
			"962e0ea1f61deb649f6bc3f4cef308",
		class: NonStandardTy,
	},
	{
		// Almost nulldata, but add an additional opcode after the data
		// to make it nonstandard.
		name:   "almost nulldata",
		script: "RETURN 4 TRUE",
		class:  NonStandardTy,
	},

	// The next few are almost multisig (it is the more complex script type)
	// but with various changes to make it fail.
	{
		// Multisig but invalid nsigs.
		name: "strange 1",
		script: "DUP DATA_33 0x0232abdc893e7f0631364d7fd01cb33d24da45" +
			"329a00357b3a7886211ab414d55a 1 CHECKMULTISIG",
		class: NonStandardTy,
	},
	{
		// Multisig but invalid pubkey.
		name:   "strange 2",
		script: "1 1 1 CHECKMULTISIG",
		class:  NonStandardTy,
	},
	{
		// Multisig but no matching npubkeys opcode.
		name: "strange 3",
		script: "1 DATA_33 0x0232abdc893e7f0631364d7fd01cb33d24da4532" +
			"9a00357b3a7886211ab414d55a DATA_33 0x0232abdc893e7f0" +
			"631364d7fd01cb33d24da45329a00357b3a7886211ab414d55a " +
			"CHECKMULTISIG",
		class: NonStandardTy,
	},
	{
		// Multisig but with multisigverify.
		name: "strange 4",
		script: "1 DATA_33 0x0232abdc893e7f0631364d7fd01cb33d24da4532" +
			"9a00357b3a7886211ab414d55a 1 CHECKMULTISIGVERIFY",
		class: NonStandardTy,
	},
	{
		// Multisig but wrong length.
		name:   "strange 5",
		script: "1 CHECKMULTISIG",
		class:  NonStandardTy,
	},
	{
		name:   "doesn't parse",
		script: "DATA_5 0x01020304",
		class:  NonStandardTy,
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
		class: NonStandardTy,
	},

	// New standard segwit script templates.
	{
		// A pay to witness pub key hash pk script.
		name:   "Pay To Witness PubkeyHash",
		script: "0 DATA_20 0x1d0f172a0ecb48aee1be1f2687d2963ae33f71a1",
		class:  WitnessV0PubKeyHashTy,
	},
	{
		// A pay to witness scripthash pk script.
		name:   "Pay To Witness Scripthash",
		script: "0 DATA_32 0x9f96ade4b41d5433f4eda31e1738ec2b36f6e7d1420d94a6af99801a88f7f7ff",
		class:  WitnessV0ScriptHashTy,
	},
}

// TestScriptClass ensures all the scripts in scriptClassTests have the expected
// class.
func TestScriptClass(t *testing.T) {
	t.Parallel()

	for _, test := range scriptClassTests {
		script := mustParseShortForm(test.script)
		class := GetScriptClass(script)
		if class != test.class {
			t.Errorf("%s: expected %s got %s (script %x)", test.name,
				test.class, class, script)
			continue
		}
	}
}

// TestStringifyClass ensures the script class string returns the expected
// string for each script class.
func TestStringifyClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		class    ScriptClass
		stringed string
	}{
		{
			name:     "nonstandardty",
			class:    NonStandardTy,
			stringed: "nonstandard",
		},
		{
			name:     "pubkey",
			class:    PubKeyTy,
			stringed: "pubkey",
		},
		{
			name:     "pubkeyhash",
			class:    PubKeyHashTy,
			stringed: "pubkeyhash",
		},
		{
			name:     "witnesspubkeyhash",
			class:    WitnessV0PubKeyHashTy,
			stringed: "witness_v0_keyhash",
		},
		{
			name:     "scripthash",
			class:    ScriptHashTy,
			stringed: "scripthash",
		},
		{
			name:     "witnessscripthash",
			class:    WitnessV0ScriptHashTy,
			stringed: "witness_v0_scripthash",
		},
		{
			name:     "multisigty",
			class:    MultiSigTy,
			stringed: "multisig",
		},
		{
			name:     "nulldataty",
			class:    NullDataTy,
			stringed: "nulldata",
		},
		{
			name:     "broken",
			class:    ScriptClass(255),
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

// TestNullDataScript tests whether NullDataScript returns a valid script.
func TestNullDataScript(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected []byte
		err      error
		class    ScriptClass
	}{
		{
			name:     "small int",
			data:     hexToBytes("01"),
			expected: mustParseShortForm("RETURN 1"),
			err:      nil,
			class:    NullDataTy,
		},
		{
			name:     "max small int",
			data:     hexToBytes("10"),
			expected: mustParseShortForm("RETURN 16"),
			err:      nil,
			class:    NullDataTy,
		},
		{
			name: "data of size before OP_PUSHDATA1 is needed",
			data: hexToBytes("0102030405060708090a0b0c0d0e0f10111" +
				"2131415161718"),
			expected: mustParseShortForm("RETURN 0x18 0x01020304" +
				"05060708090a0b0c0d0e0f101112131415161718"),
			err:   nil,
			class: NullDataTy,
		},
		{
			name: "just right",
			data: hexToBytes("000102030405060708090a0b0c0d0e0f101" +
				"112131415161718191a1b1c1d1e1f202122232425262" +
				"728292a2b2c2d2e2f303132333435363738393a3b3c3" +
				"d3e3f404142434445464748494a4b4c4d4e4f"),
			expected: mustParseShortForm("RETURN PUSHDATA1 0x50 " +
				"0x000102030405060708090a0b0c0d0e0f101112131" +
				"415161718191a1b1c1d1e1f20212223242526272829" +
				"2a2b2c2d2e2f303132333435363738393a3b3c3d3e3" +
				"f404142434445464748494a4b4c4d4e4f"),
			err:   nil,
			class: NullDataTy,
		},
		{
			name: "too big",
			data: hexToBytes("000102030405060708090a0b0c0d0e0f101" +
				"112131415161718191a1b1c1d1e1f202122232425262" +
				"728292a2b2c2d2e2f303132333435363738393a3b3c3" +
				"d3e3f404142434445464748494a4b4c4d4e4f50"),
			expected: nil,
			err:      scriptError(ErrTooMuchNullData, ""),
			class:    NonStandardTy,
		},
	}

	for i, test := range tests {
		script, err := NullDataScript(test.data)
		if e := tstCheckScriptError(err, test.err); e != nil {
			t.Errorf("NullDataScript: #%d (%s): %v", i, test.name,
				e)
			continue

		}

		// Check that the expected result was returned.
		if !bytes.Equal(script, test.expected) {
			t.Errorf("NullDataScript: #%d (%s) wrong result\n"+
				"got: %x\nwant: %x", i, test.name, script,
				test.expected)
			continue
		}

		// Check that the script has the correct type.
		scriptType := GetScriptClass(script)
		if scriptType != test.class {
			t.Errorf("GetScriptClass: #%d (%s) wrong result -- "+
				"got: %v, want: %v", i, test.name, scriptType,
				test.class)
			continue
		}
	}
}

// TestNewScriptClass tests whether NewScriptClass returns a valid ScriptClass.
func TestNewScriptClass(t *testing.T) {
	tests := []struct {
		name       string
		scriptName string
		want       *ScriptClass
		wantErr    error
	}{
		{
			name:       "NewScriptClass - ok",
			scriptName: NullDataTy.String(),
			want: func() *ScriptClass {
				s := NullDataTy
				return &s
			}(),
		},
		{
			name:       "NewScriptClass - invalid",
			scriptName: "foo",
			wantErr:    ErrUnsupportedScriptType,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewScriptClass(tt.scriptName)
			if err != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("NewScriptClass() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewScriptClass() got = %v, want %v", got, tt.want)
			}
		})
	}
}
