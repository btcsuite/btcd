package btcnet_test

import (
	"testing"

	. "github.com/conformal/btcnet"
)

// Define some of the required parameters for a user-registered
// network.  This is necessary to test the registration of and
// lookup of encoding magics from the network.
var mockNetParams = Params{
	Name:             "mocknet",
	Net:              1<<32 - 1,
	PubKeyHashAddrID: 0x9f,
	ScriptHashAddrID: 0xf9,
}

func TestRegister(t *testing.T) {
	type registerTest struct {
		name   string
		params *Params
		err    error
	}
	type magicTest struct {
		magic byte
		valid bool
	}

	tests := []struct {
		name        string
		register    []registerTest
		p2pkhMagics []magicTest
		p2shMagics  []magicTest
	}{
		{
			name: "default networks",
			register: []registerTest{
				{
					name:   "duplicate mainnet",
					params: &MainNetParams,
					err:    ErrDuplicateNet,
				},
				{
					name:   "duplicate regtest",
					params: &RegressionNetParams,
					err:    ErrDuplicateNet,
				},
				{
					name:   "duplicate testnet3",
					params: &TestNet3Params,
					err:    ErrDuplicateNet,
				},
				{
					name:   "duplicate simnet",
					params: &SimNetParams,
					err:    ErrDuplicateNet,
				},
			},
			p2pkhMagics: []magicTest{
				{
					magic: MainNetParams.PubKeyHashAddrID,
					valid: true,
				},
				{
					magic: TestNet3Params.PubKeyHashAddrID,
					valid: true,
				},
				{
					magic: RegressionNetParams.PubKeyHashAddrID,
					valid: true,
				},
				{
					magic: SimNetParams.PubKeyHashAddrID,
					valid: true,
				},
				{
					magic: mockNetParams.PubKeyHashAddrID,
					valid: false,
				},
				{
					magic: 0xFF,
					valid: false,
				},
			},
			p2shMagics: []magicTest{
				{
					magic: MainNetParams.ScriptHashAddrID,
					valid: true,
				},
				{
					magic: TestNet3Params.ScriptHashAddrID,
					valid: true,
				},
				{
					magic: RegressionNetParams.ScriptHashAddrID,
					valid: true,
				},
				{
					magic: SimNetParams.ScriptHashAddrID,
					valid: true,
				},
				{
					magic: mockNetParams.ScriptHashAddrID,
					valid: false,
				},
				{
					magic: 0xFF,
					valid: false,
				},
			},
		},
		{
			name: "register mocknet",
			register: []registerTest{
				{
					name:   "mocknet",
					params: &mockNetParams,
					err:    nil,
				},
			},
			p2pkhMagics: []magicTest{
				{
					magic: MainNetParams.PubKeyHashAddrID,
					valid: true,
				},
				{
					magic: TestNet3Params.PubKeyHashAddrID,
					valid: true,
				},
				{
					magic: RegressionNetParams.PubKeyHashAddrID,
					valid: true,
				},
				{
					magic: SimNetParams.PubKeyHashAddrID,
					valid: true,
				},
				{
					magic: mockNetParams.PubKeyHashAddrID,
					valid: true,
				},
				{
					magic: 0xFF,
					valid: false,
				},
			},
			p2shMagics: []magicTest{
				{
					magic: MainNetParams.ScriptHashAddrID,
					valid: true,
				},
				{
					magic: TestNet3Params.ScriptHashAddrID,
					valid: true,
				},
				{
					magic: RegressionNetParams.ScriptHashAddrID,
					valid: true,
				},
				{
					magic: SimNetParams.ScriptHashAddrID,
					valid: true,
				},
				{
					magic: mockNetParams.ScriptHashAddrID,
					valid: true,
				},
				{
					magic: 0xFF,
					valid: false,
				},
			},
		},
		{
			name: "more duplicates",
			register: []registerTest{
				{
					name:   "duplicate mainnet",
					params: &MainNetParams,
					err:    ErrDuplicateNet,
				},
				{
					name:   "duplicate regtest",
					params: &RegressionNetParams,
					err:    ErrDuplicateNet,
				},
				{
					name:   "duplicate testnet3",
					params: &TestNet3Params,
					err:    ErrDuplicateNet,
				},
				{
					name:   "duplicate simnet",
					params: &SimNetParams,
					err:    ErrDuplicateNet,
				},
				{
					name:   "duplicate mocknet",
					params: &mockNetParams,
					err:    ErrDuplicateNet,
				},
			},
			p2pkhMagics: []magicTest{
				{
					magic: MainNetParams.PubKeyHashAddrID,
					valid: true,
				},
				{
					magic: TestNet3Params.PubKeyHashAddrID,
					valid: true,
				},
				{
					magic: RegressionNetParams.PubKeyHashAddrID,
					valid: true,
				},
				{
					magic: SimNetParams.PubKeyHashAddrID,
					valid: true,
				},
				{
					magic: mockNetParams.PubKeyHashAddrID,
					valid: true,
				},
				{
					magic: 0xFF,
					valid: false,
				},
			},
			p2shMagics: []magicTest{
				{
					magic: MainNetParams.ScriptHashAddrID,
					valid: true,
				},
				{
					magic: TestNet3Params.ScriptHashAddrID,
					valid: true,
				},
				{
					magic: RegressionNetParams.ScriptHashAddrID,
					valid: true,
				},
				{
					magic: SimNetParams.ScriptHashAddrID,
					valid: true,
				},
				{
					magic: mockNetParams.ScriptHashAddrID,
					valid: true,
				},
				{
					magic: 0xFF,
					valid: false,
				},
			},
		},
	}

	for _, test := range tests {
		for _, regTest := range test.register {
			err := Register(regTest.params)
			if err != regTest.err {
				t.Errorf("%s:%s: Registered network with unexpected error: got %v expected %v",
					test.name, regTest.name, err, regTest.err)
			}
		}
		for i, magTest := range test.p2pkhMagics {
			valid := IsPubKeyHashAddrID(magTest.magic)
			if valid != magTest.valid {
				t.Errorf("%s: P2PKH magic %d valid mismatch: got %v expected %v",
					test.name, i, valid, magTest.valid)
			}
		}
		for i, magTest := range test.p2shMagics {
			valid := IsScriptHashAddrID(magTest.magic)
			if valid != magTest.valid {
				t.Errorf("%s: P2SH magic %d valid mismatch: got %v expected %v",
					test.name, i, valid, magTest.valid)
			}
		}
	}
}
