// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcscript_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/conformal/btcec"
	"github.com/conformal/btcnet"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

func TestPushedData(t *testing.T) {
	var tests = []struct {
		in    []byte
		out   [][]byte
		valid bool
	}{
		{
			[]byte{btcscript.OP_0, btcscript.OP_IF, btcscript.OP_0, btcscript.OP_ELSE, btcscript.OP_2, btcscript.OP_ENDIF},
			[][]byte{{}, {}},
			true,
		},
		{
			btcscript.NewScriptBuilder().AddInt64(16777216).AddInt64(10000000).Script(),
			[][]byte{
				{0x00, 0x00, 0x00, 0x01}, // 16777216
				{0x80, 0x96, 0x98, 0x00}, // 10000000
			},
			true,
		},
		{
			btcscript.NewScriptBuilder().AddOp(btcscript.OP_DUP).AddOp(btcscript.OP_HASH160).
				AddData([]byte("17VZNX1SN5NtKa8UQFxwQbFeFc3iqRYhem")).AddOp(btcscript.OP_EQUALVERIFY).
				AddOp(btcscript.OP_CHECKSIG).Script(),
			[][]byte{
				// 17VZNX1SN5NtKa8UQFxwQbFeFc3iqRYhem
				{
					0x31, 0x37, 0x56, 0x5a, 0x4e, 0x58, 0x31, 0x53, 0x4e, 0x35,
					0x4e, 0x74, 0x4b, 0x61, 0x38, 0x55, 0x51, 0x46, 0x78, 0x77,
					0x51, 0x62, 0x46, 0x65, 0x46, 0x63, 0x33, 0x69, 0x71, 0x52,
					0x59, 0x68, 0x65, 0x6d,
				},
			},
			true,
		},
		{
			btcscript.NewScriptBuilder().AddOp(btcscript.OP_PUSHDATA4).AddInt64(1000).
				AddOp(btcscript.OP_EQUAL).Script(),
			[][]byte{},
			false,
		},
	}

	for x, test := range tests {
		pushedData, err := btcscript.PushedData(test.in)
		if test.valid && err != nil {
			t.Errorf("TestPushedData failed test #%d: %v\n", x, err)
			continue
		} else if !test.valid && err == nil {
			t.Errorf("TestPushedData failed test #%d: test should be invalid\n", x)
			continue
		}
		for x, data := range pushedData {
			if !bytes.Equal(data, test.out[x]) {
				t.Errorf("TestPushedData failed test #%d: want: %x got: %x\n",
					x, test.out[x], data)
			}
		}
	}
}

func TestStandardPushes(t *testing.T) {
	for i := 0; i < 1000; i++ {
		builder := btcscript.NewScriptBuilder()
		builder.AddInt64(int64(i))
		if result := btcscript.IsPushOnlyScript(builder.Script()); !result {
			t.Errorf("StandardPushesTests IsPushOnlyScript test #%d failed: %x\n", i, builder.Script())
		}
		if result := btcscript.HasCanonicalPushes(builder.Script()); !result {
			t.Errorf("StandardPushesTests HasCanonicalPushes test #%d failed: %x\n", i, builder.Script())
			continue
		}
	}
	for i := 0; i < 1000; i++ {
		builder := btcscript.NewScriptBuilder()
		builder.AddData(bytes.Repeat([]byte{0x49}, i))
		if result := btcscript.IsPushOnlyScript(builder.Script()); !result {
			t.Errorf("StandardPushesTests IsPushOnlyScript test #%d failed: %x\n", i, builder.Script())
		}
		if result := btcscript.HasCanonicalPushes(builder.Script()); !result {
			t.Errorf("StandardPushesTests HasCanonicalPushes test #%d failed: %x\n", i, builder.Script())
			continue
		}
	}
}

type txTest struct {
	name          string
	tx            *btcwire.MsgTx
	pkScript      []byte               // output script of previous tx
	idx           int                  // tx idx to be run.
	bip16         bool                 // is bip16 active?
	canonicalSigs bool                 // should signatures be validated as canonical?
	parseErr      error                // failure of NewScript
	err           error                // Failure of Executre
	shouldFail    bool                 // Execute should fail with nonspecified error.
	nSigOps       int                  // result of GetPreciseSigOpsCount
	scriptInfo    btcscript.ScriptInfo // result of ScriptInfo
	scriptInfoErr error                // error return of ScriptInfo
}

var txTests = []txTest{
	// tx 0437cd7f8525ceed2324359c2d0ba26006d92d85. the first tx in the
	// blockchain that verifies signatures.
	{
		name: "CheckSig",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0xc9, 0x97, 0xa5, 0xe5,
							0x6e, 0x10, 0x41, 0x02,
							0xfa, 0x20, 0x9c, 0x6a,
							0x85, 0x2d, 0xd9, 0x06,
							0x60, 0xa2, 0x0b, 0x2d,
							0x9c, 0x35, 0x24, 0x23,
							0xed, 0xce, 0x25, 0x85,
							0x7f, 0xcd, 0x37, 0x04,
						}),
						Index: 0,
					},
					SignatureScript: []uint8{
						btcscript.OP_DATA_71,
						0x30, 0x44, 0x02, 0x20, 0x4e,
						0x45, 0xe1, 0x69, 0x32, 0xb8,
						0xaf, 0x51, 0x49, 0x61, 0xa1,
						0xd3, 0xa1, 0xa2, 0x5f, 0xdf,
						0x3f, 0x4f, 0x77, 0x32, 0xe9,
						0xd6, 0x24, 0xc6, 0xc6, 0x15,
						0x48, 0xab, 0x5f, 0xb8, 0xcd,
						0x41, 0x02, 0x20, 0x18, 0x15,
						0x22, 0xec, 0x8e, 0xca, 0x07,
						0xde, 0x48, 0x60, 0xa4, 0xac,
						0xdd, 0x12, 0x90, 0x9d, 0x83,
						0x1c, 0xc5, 0x6c, 0xbb, 0xac,
						0x46, 0x22, 0x08, 0x22, 0x21,
						0xa8, 0x76, 0x8d, 0x1d, 0x09,
						0x01,
					},

					Sequence: 4294967295,
				},
			},
			TxOut: []*btcwire.TxOut{
				{
					Value: 1000000000,
					PkScript: []byte{
						btcscript.OP_DATA_65,
						0x04, 0xae, 0x1a, 0x62, 0xfe,
						0x09, 0xc5, 0xf5, 0x1b, 0x13,
						0x90, 0x5f, 0x07, 0xf0, 0x6b,
						0x99, 0xa2, 0xf7, 0x15, 0x9b,
						0x22, 0x25, 0xf3, 0x74, 0xcd,
						0x37, 0x8d, 0x71, 0x30, 0x2f,
						0xa2, 0x84, 0x14, 0xe7, 0xaa,
						0xb3, 0x73, 0x97, 0xf5, 0x54,
						0xa7, 0xdf, 0x5f, 0x14, 0x2c,
						0x21, 0xc1, 0xb7, 0x30, 0x3b,
						0x8a, 0x06, 0x26, 0xf1, 0xba,
						0xde, 0xd5, 0xc7, 0x2a, 0x70,
						0x4f, 0x7e, 0x6c, 0xd8, 0x4c,
						btcscript.OP_CHECKSIG,
					},
				},
				{
					Value: 4000000000,
					PkScript: []byte{
						btcscript.OP_DATA_65,
						0x04, 0x11, 0xdb, 0x93, 0xe1,
						0xdc, 0xdb, 0x8a, 0x01, 0x6b,
						0x49, 0x84, 0x0f, 0x8c, 0x53,
						0xbc, 0x1e, 0xb6, 0x8a, 0x38,
						0x2e, 0x97, 0xb1, 0x48, 0x2e,
						0xca, 0xd7, 0xb1, 0x48, 0xa6,
						0x90, 0x9a, 0x5c, 0xb2, 0xe0,
						0xea, 0xdd, 0xfb, 0x84, 0xcc,
						0xf9, 0x74, 0x44, 0x64, 0xf8,
						0x2e, 0x16, 0x0b, 0xfa, 0x9b,
						0x8b, 0x64, 0xf9, 0xd4, 0xc0,
						0x3f, 0x99, 0x9b, 0x86, 0x43,
						0xf6, 0x56, 0xb4, 0x12, 0xa3,
						btcscript.OP_CHECKSIG,
					},
				},
			},
			LockTime: 0,
		},
		pkScript: []byte{
			btcscript.OP_DATA_65,
			0x04, 0x11, 0xdb, 0x93, 0xe1, 0xdc, 0xdb, 0x8a, 0x01,
			0x6b, 0x49, 0x84, 0x0f, 0x8c, 0x53, 0xbc, 0x1e, 0xb6,
			0x8a, 0x38, 0x2e, 0x97, 0xb1, 0x48, 0x2e, 0xca, 0xd7,
			0xb1, 0x48, 0xa6, 0x90, 0x9a, 0x5c, 0xb2, 0xe0, 0xea,
			0xdd, 0xfb, 0x84, 0xcc, 0xf9, 0x74, 0x44, 0x64, 0xf8,
			0x2e, 0x16, 0x0b, 0xfa, 0x9b, 0x8b, 0x64, 0xf9, 0xd4,
			0xc0, 0x3f, 0x99, 0x9b, 0x86, 0x43, 0xf6, 0x56, 0xb4,
			0x12, 0xa3, btcscript.OP_CHECKSIG,
		},
		idx:     0,
		nSigOps: 1,
		scriptInfo: btcscript.ScriptInfo{
			PkScriptClass:  btcscript.PubKeyTy,
			NumInputs:      1,
			ExpectedInputs: 1,
			SigOps:         1,
		},
	},
	// Previous test with the value of one output changed.
	{
		name: "CheckSig Failure",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0xc9, 0x97, 0xa5, 0xe5,
							0x6e, 0x10, 0x41, 0x02,
							0xfa, 0x20, 0x9c, 0x6a,
							0x85, 0x2d, 0xd9, 0x06,
							0x60, 0xa2, 0x0b, 0x2d,
							0x9c, 0x35, 0x24, 0x23,
							0xed, 0xce, 0x25, 0x85,
							0x7f, 0xcd, 0x37, 0x04,
						}),
						Index: 0,
					},
					SignatureScript: []uint8{
						btcscript.OP_DATA_71,
						0x30, 0x44, 0x02, 0x20, 0x4e,
						0x45, 0xe1, 0x69, 0x32, 0xb8,
						0xaf, 0x51, 0x49, 0x61, 0xa1,
						0xd3, 0xa1, 0xa2, 0x5f, 0xdf,
						0x3f, 0x4f, 0x77, 0x32, 0xe9,
						0xd6, 0x24, 0xc6, 0xc6, 0x15,
						0x48, 0xab, 0x5f, 0xb8, 0xcd,
						0x41, 0x02, 0x20, 0x18, 0x15,
						0x22, 0xec, 0x8e, 0xca, 0x07,
						0xde, 0x48, 0x60, 0xa4, 0xac,
						0xdd, 0x12, 0x90, 0x9d, 0x83,
						0x1c, 0xc5, 0x6c, 0xbb, 0xac,
						0x46, 0x22, 0x08, 0x22, 0x21,
						0xa8, 0x76, 0x8d, 0x1d, 0x09,
						0x01,
					},

					Sequence: 4294967295,
				},
			},
			TxOut: []*btcwire.TxOut{
				{
					Value: 1000000000,
					PkScript: []byte{
						btcscript.OP_DATA_65,
						0x04, 0xae, 0x1a, 0x62, 0xfe,
						0x09, 0xc5, 0xf5, 0x1b, 0x13,
						0x90, 0x5f, 0x07, 0xf0, 0x6b,
						0x99, 0xa2, 0xf7, 0x15, 0x9b,
						0x22, 0x25, 0xf3, 0x74, 0xcd,
						0x37, 0x8d, 0x71, 0x30, 0x2f,
						0xa2, 0x84, 0x14, 0xe7, 0xaa,
						0xb3, 0x73, 0x97, 0xf5, 0x54,
						0xa7, 0xdf, 0x5f, 0x14, 0x2c,
						0x21, 0xc1, 0xb7, 0x30, 0x3b,
						0x8a, 0x06, 0x26, 0xf1, 0xba,
						0xde, 0xd5, 0xc7, 0x2a, 0x70,
						0x4f, 0x7e, 0x6c, 0xd8, 0x4c,
						btcscript.OP_CHECKSIG,
					},
				},
				{
					Value: 5000000000,
					PkScript: []byte{
						btcscript.OP_DATA_65,
						0x04, 0x11, 0xdb, 0x93, 0xe1,
						0xdc, 0xdb, 0x8a, 0x01, 0x6b,
						0x49, 0x84, 0x0f, 0x8c, 0x53,
						0xbc, 0x1e, 0xb6, 0x8a, 0x38,
						0x2e, 0x97, 0xb1, 0x48, 0x2e,
						0xca, 0xd7, 0xb1, 0x48, 0xa6,
						0x90, 0x9a, 0x5c, 0xb2, 0xe0,
						0xea, 0xdd, 0xfb, 0x84, 0xcc,
						0xf9, 0x74, 0x44, 0x64, 0xf8,
						0x2e, 0x16, 0x0b, 0xfa, 0x9b,
						0x8b, 0x64, 0xf9, 0xd4, 0xc0,
						0x3f, 0x99, 0x9b, 0x86, 0x43,
						0xf6, 0x56, 0xb4, 0x12, 0xa3,
						btcscript.OP_CHECKSIG,
					},
				},
			},
			LockTime: 0,
		},
		pkScript: []byte{
			btcscript.OP_DATA_65,
			0x04, 0x11, 0xdb, 0x93, 0xe1, 0xdc, 0xdb, 0x8a, 0x01,
			0x6b, 0x49, 0x84, 0x0f, 0x8c, 0x53, 0xbc, 0x1e, 0xb6,
			0x8a, 0x38, 0x2e, 0x97, 0xb1, 0x48, 0x2e, 0xca, 0xd7,
			0xb1, 0x48, 0xa6, 0x90, 0x9a, 0x5c, 0xb2, 0xe0, 0xea,
			0xdd, 0xfb, 0x84, 0xcc, 0xf9, 0x74, 0x44, 0x64, 0xf8,
			0x2e, 0x16, 0x0b, 0xfa, 0x9b, 0x8b, 0x64, 0xf9, 0xd4,
			0xc0, 0x3f, 0x99, 0x9b, 0x86, 0x43, 0xf6, 0x56, 0xb4,
			0x12, 0xa3, btcscript.OP_CHECKSIG,
		},
		idx:     0,
		err:     btcscript.StackErrScriptFailed,
		nSigOps: 1,
		scriptInfo: btcscript.ScriptInfo{
			PkScriptClass:  btcscript.PubKeyTy,
			NumInputs:      1,
			ExpectedInputs: 1,
			SigOps:         1,
		},
	},
	{
		name: "CheckSig invalid signature",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0xc9, 0x97, 0xa5, 0xe5,
							0x6e, 0x10, 0x41, 0x02,
							0xfa, 0x20, 0x9c, 0x6a,
							0x85, 0x2d, 0xd9, 0x06,
							0x60, 0xa2, 0x0b, 0x2d,
							0x9c, 0x35, 0x24, 0x23,
							0xed, 0xce, 0x25, 0x85,
							0x7f, 0xcd, 0x37, 0x04,
						}),
						Index: 0,
					},
					// Signature has length fiddled to
					// fail parsing.
					SignatureScript: []uint8{
						btcscript.OP_DATA_71,
						0x30, 0x45, 0x02, 0x20, 0x4e,
						0x45, 0xe1, 0x69, 0x32, 0xb8,
						0xaf, 0x51, 0x49, 0x61, 0xa1,
						0xd3, 0xa1, 0xa2, 0x5f, 0xdf,
						0x3f, 0x4f, 0x77, 0x32, 0xe9,
						0xd6, 0x24, 0xc6, 0xc6, 0x15,
						0x48, 0xab, 0x5f, 0xb8, 0xcd,
						0x41, 0x02, 0x20, 0x18, 0x15,
						0x22, 0xec, 0x8e, 0xca, 0x07,
						0xde, 0x48, 0x60, 0xa4, 0xac,
						0xdd, 0x12, 0x90, 0x9d, 0x83,
						0x1c, 0xc5, 0x6c, 0xbb, 0xac,
						0x46, 0x22, 0x08, 0x22, 0x21,
						0xa8, 0x76, 0x8d, 0x1d, 0x09,
						0x01,
					},

					Sequence: 4294967295,
				},
			},
			TxOut: []*btcwire.TxOut{
				{
					Value: 1000000000,
					PkScript: []byte{
						btcscript.OP_DATA_65,
						0x04, 0xae, 0x1a, 0x62, 0xfe,
						0x09, 0xc5, 0xf5, 0x1b, 0x13,
						0x90, 0x5f, 0x07, 0xf0, 0x6b,
						0x99, 0xa2, 0xf7, 0x15, 0x9b,
						0x22, 0x25, 0xf3, 0x74, 0xcd,
						0x37, 0x8d, 0x71, 0x30, 0x2f,
						0xa2, 0x84, 0x14, 0xe7, 0xaa,
						0xb3, 0x73, 0x97, 0xf5, 0x54,
						0xa7, 0xdf, 0x5f, 0x14, 0x2c,
						0x21, 0xc1, 0xb7, 0x30, 0x3b,
						0x8a, 0x06, 0x26, 0xf1, 0xba,
						0xde, 0xd5, 0xc7, 0x2a, 0x70,
						0x4f, 0x7e, 0x6c, 0xd8, 0x4c,
						btcscript.OP_CHECKSIG,
					},
				},
				{
					Value: 4000000000,
					PkScript: []byte{
						btcscript.OP_DATA_65,
						0x04, 0x11, 0xdb, 0x93, 0xe1,
						0xdc, 0xdb, 0x8a, 0x01, 0x6b,
						0x49, 0x84, 0x0f, 0x8c, 0x53,
						0xbc, 0x1e, 0xb6, 0x8a, 0x38,
						0x2e, 0x97, 0xb1, 0x48, 0x2e,
						0xca, 0xd7, 0xb1, 0x48, 0xa6,
						0x90, 0x9a, 0x5c, 0xb2, 0xe0,
						0xea, 0xdd, 0xfb, 0x84, 0xcc,
						0xf9, 0x74, 0x44, 0x64, 0xf8,
						0x2e, 0x16, 0x0b, 0xfa, 0x9b,
						0x8b, 0x64, 0xf9, 0xd4, 0xc0,
						0x3f, 0x99, 0x9b, 0x86, 0x43,
						0xf6, 0x56, 0xb4, 0x12, 0xa3,
						btcscript.OP_CHECKSIG,
					},
				},
			},
			LockTime: 0,
		},
		pkScript: []byte{
			btcscript.OP_DATA_65,
			0x04, 0x11, 0xdb, 0x93, 0xe1, 0xdc, 0xdb, 0x8a, 0x01,
			0x6b, 0x49, 0x84, 0x0f, 0x8c, 0x53, 0xbc, 0x1e, 0xb6,
			0x8a, 0x38, 0x2e, 0x97, 0xb1, 0x48, 0x2e, 0xca, 0xd7,
			0xb1, 0x48, 0xa6, 0x90, 0x9a, 0x5c, 0xb2, 0xe0, 0xea,
			0xdd, 0xfb, 0x84, 0xcc, 0xf9, 0x74, 0x44, 0x64, 0xf8,
			0x2e, 0x16, 0x0b, 0xfa, 0x9b, 0x8b, 0x64, 0xf9, 0xd4,
			0xc0, 0x3f, 0x99, 0x9b, 0x86, 0x43, 0xf6, 0x56, 0xb4,
			0x12, 0xa3, btcscript.OP_CHECKSIG,
		},
		idx:        0,
		shouldFail: true,
		nSigOps:    1,
		scriptInfo: btcscript.ScriptInfo{
			PkScriptClass:  btcscript.PubKeyTy,
			NumInputs:      1,
			ExpectedInputs: 1,
			SigOps:         1,
		},
	},
	{
		name: "CheckSig invalid pubkey",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0xc9, 0x97, 0xa5, 0xe5,
							0x6e, 0x10, 0x41, 0x02,
							0xfa, 0x20, 0x9c, 0x6a,
							0x85, 0x2d, 0xd9, 0x06,
							0x60, 0xa2, 0x0b, 0x2d,
							0x9c, 0x35, 0x24, 0x23,
							0xed, 0xce, 0x25, 0x85,
							0x7f, 0xcd, 0x37, 0x04,
						}),
						Index: 0,
					},
					SignatureScript: []uint8{
						btcscript.OP_DATA_71,
						0x30, 0x44, 0x02, 0x20, 0x4e,
						0x45, 0xe1, 0x69, 0x32, 0xb8,
						0xaf, 0x51, 0x49, 0x61, 0xa1,
						0xd3, 0xa1, 0xa2, 0x5f, 0xdf,
						0x3f, 0x4f, 0x77, 0x32, 0xe9,
						0xd6, 0x24, 0xc6, 0xc6, 0x15,
						0x48, 0xab, 0x5f, 0xb8, 0xcd,
						0x41, 0x02, 0x20, 0x18, 0x15,
						0x22, 0xec, 0x8e, 0xca, 0x07,
						0xde, 0x48, 0x60, 0xa4, 0xac,
						0xdd, 0x12, 0x90, 0x9d, 0x83,
						0x1c, 0xc5, 0x6c, 0xbb, 0xac,
						0x46, 0x22, 0x08, 0x22, 0x21,
						0xa8, 0x76, 0x8d, 0x1d, 0x09,
						0x01,
					},

					Sequence: 4294967295,
				},
			},
			TxOut: []*btcwire.TxOut{
				{
					Value: 1000000000,
					PkScript: []byte{
						btcscript.OP_DATA_65,
						0x04, 0xae, 0x1a, 0x62, 0xfe,
						0x09, 0xc5, 0xf5, 0x1b, 0x13,
						0x90, 0x5f, 0x07, 0xf0, 0x6b,
						0x99, 0xa2, 0xf7, 0x15, 0x9b,
						0x22, 0x25, 0xf3, 0x74, 0xcd,
						0x37, 0x8d, 0x71, 0x30, 0x2f,
						0xa2, 0x84, 0x14, 0xe7, 0xaa,
						0xb3, 0x73, 0x97, 0xf5, 0x54,
						0xa7, 0xdf, 0x5f, 0x14, 0x2c,
						0x21, 0xc1, 0xb7, 0x30, 0x3b,
						0x8a, 0x06, 0x26, 0xf1, 0xba,
						0xde, 0xd5, 0xc7, 0x2a, 0x70,
						0x4f, 0x7e, 0x6c, 0xd8, 0x4c,
						btcscript.OP_CHECKSIG,
					},
				},
				{
					Value: 4000000000,
					PkScript: []byte{
						btcscript.OP_DATA_65,
						0x04, 0x11, 0xdb, 0x93, 0xe1,
						0xdc, 0xdb, 0x8a, 0x01, 0x6b,
						0x49, 0x84, 0x0f, 0x8c, 0x53,
						0xbc, 0x1e, 0xb6, 0x8a, 0x38,
						0x2e, 0x97, 0xb1, 0x48, 0x2e,
						0xca, 0xd7, 0xb1, 0x48, 0xa6,
						0x90, 0x9a, 0x5c, 0xb2, 0xe0,
						0xea, 0xdd, 0xfb, 0x84, 0xcc,
						0xf9, 0x74, 0x44, 0x64, 0xf8,
						0x2e, 0x16, 0x0b, 0xfa, 0x9b,
						0x8b, 0x64, 0xf9, 0xd4, 0xc0,
						0x3f, 0x99, 0x9b, 0x86, 0x43,
						0xf6, 0x56, 0xb4, 0x12, 0xa3,
						btcscript.OP_CHECKSIG,
					},
				},
			},
			LockTime: 0,
		},
		// pubkey header magic byte has been changed to parse wrong.
		pkScript: []byte{
			btcscript.OP_DATA_65,
			0x02, 0x11, 0xdb, 0x93, 0xe1, 0xdc, 0xdb, 0x8a, 0x01,
			0x6b, 0x49, 0x84, 0x0f, 0x8c, 0x53, 0xbc, 0x1e, 0xb6,
			0x8a, 0x38, 0x2e, 0x97, 0xb1, 0x48, 0x2e, 0xca, 0xd7,
			0xb1, 0x48, 0xa6, 0x90, 0x9a, 0x5c, 0xb2, 0xe0, 0xea,
			0xdd, 0xfb, 0x84, 0xcc, 0xf9, 0x74, 0x44, 0x64, 0xf8,
			0x2e, 0x16, 0x0b, 0xfa, 0x9b, 0x8b, 0x64, 0xf9, 0xd4,
			0xc0, 0x3f, 0x99, 0x9b, 0x86, 0x43, 0xf6, 0x56, 0xb4,
			0x12, 0xa3, btcscript.OP_CHECKSIG,
		},
		idx:        0,
		shouldFail: true,
		nSigOps:    1,
		scriptInfo: btcscript.ScriptInfo{
			PkScriptClass:  btcscript.PubKeyTy,
			NumInputs:      1,
			ExpectedInputs: 1,
			SigOps:         1,
		},
	},
	// tx 599e47a8114fe098103663029548811d2651991b62397e057f0c863c2bc9f9ea
	// uses checksig with SigHashNone.
	{
		name: "CheckSigHashNone",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0x5f, 0x38, 0x6c, 0x8a,
							0x38, 0x42, 0xc9, 0xa9,
							0xdc, 0xfa, 0x9b, 0x78,
							0xbe, 0x78, 0x5a, 0x40,
							0xa7, 0xbd, 0xa0, 0x8b,
							0x64, 0x64, 0x6b, 0xe3,
							0x65, 0x43, 0x01, 0xea,
							0xcc, 0xfc, 0x8d, 0x5e,
						}),
						Index: 1,
					},
					SignatureScript: []byte{
						btcscript.OP_DATA_71,
						0x30, 0x44, 0x02, 0x20, 0xbb,
						0x4f, 0xbc, 0x49, 0x5a, 0xa2,
						0x3b, 0xab, 0xb2, 0xc2, 0xbe,
						0x4e, 0x3f, 0xb4, 0xa5, 0xdf,
						0xfe, 0xfe, 0x20, 0xc8, 0xef,
						0xf5, 0x94, 0x0f, 0x13, 0x56,
						0x49, 0xc3, 0xea, 0x96, 0x44,
						0x4a, 0x02, 0x20, 0x04, 0xaf,
						0xcd, 0xa9, 0x66, 0xc8, 0x07,
						0xbb, 0x97, 0x62, 0x2d, 0x3e,
						0xef, 0xea, 0x82, 0x8f, 0x62,
						0x3a, 0xf3, 0x06, 0xef, 0x2b,
						0x75, 0x67, 0x82, 0xee, 0x6f,
						0x8a, 0x22, 0xa9, 0x59, 0xa2,
						0x02,
						btcscript.OP_DATA_65,
						0x04, 0xf1, 0x93, 0x9a, 0xe6,
						0xb0, 0x1e, 0x84, 0x9b, 0xf0,
						0x5d, 0x0e, 0xd5, 0x1f, 0xd5,
						0xb9, 0x2b, 0x79, 0xa0, 0xe3,
						0x13, 0xe3, 0xf3, 0x89, 0xc7,
						0x26, 0xf1, 0x1f, 0xa3, 0xe1,
						0x44, 0xd9, 0x22, 0x7b, 0x07,
						0xe8, 0xa8, 0x7c, 0x0e, 0xe3,
						0x63, 0x72, 0xe9, 0x67, 0xe0,
						0x90, 0xd1, 0x1b, 0x77, 0x77,
						0x07, 0xaa, 0x73, 0xef, 0xac,
						0xab, 0xff, 0xff, 0xa2, 0x85,
						0xc0, 0x0b, 0x36, 0x22, 0xd6,
					},
					Sequence: 4294967295,
				},
			},
			TxOut: []*btcwire.TxOut{
				{
					Value: 1000000,
					PkScript: []byte{
						btcscript.OP_DUP,
						btcscript.OP_HASH160,
						btcscript.OP_DATA_20,
						0x66, 0x0d, 0x4e, 0xf3, 0xa7,
						0x43, 0xe3, 0xe6, 0x96, 0xad,
						0x99, 0x03, 0x64, 0xe5, 0x55,
						0xc2, 0x71, 0xad, 0x50, 0x4b,
						btcscript.OP_EQUALVERIFY,
						btcscript.OP_CHECKSIG,
					},
				},
				{
					Value: 29913632,
					PkScript: []byte{
						btcscript.OP_DUP,
						btcscript.OP_HASH160,
						btcscript.OP_DATA_20,
						0x21, 0xc4, 0x3c, 0xe4, 0x00,
						0x90, 0x13, 0x12, 0xa6, 0x03,
						0xe4, 0x20, 0x7a, 0xad, 0xfd,
						0x74, 0x2b, 0xe8, 0xe7, 0xda,
						btcscript.OP_EQUALVERIFY,
						btcscript.OP_CHECKSIG,
					},
				},
			},
			LockTime: 0,
		},
		pkScript: []byte{
			btcscript.OP_DUP,
			btcscript.OP_HASH160,
			btcscript.OP_DATA_20,
			0x21, 0xc4, 0x3c, 0xe4, 0x00, 0x90, 0x13, 0x12, 0xa6,
			0x03, 0xe4, 0x20, 0x7a, 0xad, 0xfd, 0x74, 0x2b, 0xe8,
			0xe7, 0xda,
			btcscript.OP_EQUALVERIFY,
			btcscript.OP_CHECKSIG,
		},
		idx:     0,
		bip16:   true, // after threshold
		nSigOps: 1,
		scriptInfo: btcscript.ScriptInfo{
			PkScriptClass:  btcscript.PubKeyHashTy,
			NumInputs:      2,
			ExpectedInputs: 2,
			SigOps:         1,
		},
	},
	{
		name: "Non-canonical signature: R value negative",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0xfe, 0x15, 0x62, 0xc4,
							0x8b, 0x3a, 0xa6, 0x37,
							0x3f, 0x42, 0xe9, 0x61,
							0x51, 0x89, 0xcf, 0x73,
							0x32, 0xd7, 0x33, 0x5c,
							0xbe, 0xa7, 0x80, 0xbe,
							0x69, 0x6a, 0xc6, 0xc6,
							0x50, 0xfd, 0xda, 0x4a,
						}),
						Index: 1,
					},
					SignatureScript: []byte{
						btcscript.OP_DATA_71,
						0x30, 0x44, 0x02, 0x20, 0xa0,
						0x42, 0xde, 0xe5, 0x52, 0x6b,
						0xf2, 0x29, 0x4d, 0x3f, 0x3e,
						0xb9, 0x5a, 0xa7, 0x73, 0x19,
						0xd3, 0xff, 0x56, 0x7b, 0xcf,
						0x36, 0x46, 0x07, 0x0c, 0x81,
						0x12, 0x33, 0x01, 0xca, 0xce,
						0xa9, 0x02, 0x20, 0xea, 0x48,
						0xae, 0x58, 0xf5, 0x54, 0x10,
						0x96, 0x3f, 0xa7, 0x03, 0xdb,
						0x56, 0xf0, 0xba, 0xb2, 0x70,
						0xb1, 0x08, 0x22, 0xc5, 0x1c,
						0x68, 0x02, 0x6a, 0x97, 0x5c,
						0x7d, 0xae, 0x11, 0x2e, 0x4f,
						0x01,
						btcscript.OP_DATA_65,
						0x04, 0x49, 0x45, 0x33, 0x18,
						0xbd, 0x5e, 0xcf, 0xea, 0x5f,
						0x86, 0x32, 0x8c, 0x6d, 0x8e,
						0xd4, 0x12, 0xb4, 0xde, 0x2c,
						0xab, 0xd7, 0xb8, 0x56, 0x51,
						0x2f, 0x8c, 0xb7, 0xfd, 0x25,
						0xf6, 0x03, 0xb0, 0x55, 0xc5,
						0xdf, 0xe6, 0x22, 0x4b, 0xc4,
						0xfd, 0xbb, 0x6a, 0x7a, 0xa0,
						0x58, 0xd7, 0x5d, 0xad, 0x92,
						0x99, 0x45, 0x4f, 0x62, 0x1a,
						0x95, 0xb4, 0xb0, 0x21, 0x0e,
						0xc4, 0x09, 0x2b, 0xe5, 0x27,
					},
					Sequence: 4294967295,
				},
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0x2a, 0xc7, 0xee, 0xf8,
							0xa9, 0x62, 0x2d, 0xda,
							0xec, 0x18, 0x3b, 0xba,
							0xa9, 0x92, 0xb0, 0x7a,
							0x70, 0x3b, 0x48, 0x86,
							0x27, 0x9c, 0x46, 0xac,
							0x25, 0xeb, 0x91, 0xec,
							0x4c, 0x86, 0xd2, 0x9c,
						}),
						Index: 1,
					},
					SignatureScript: []byte{
						btcscript.OP_DATA_71,
						0x30, 0x44, 0x02, 0x20, 0xc3,
						0x02, 0x3b, 0xed, 0x85, 0x0d,
						0x94, 0x27, 0x8e, 0x06, 0xd2,
						0x37, 0x92, 0x21, 0x55, 0x28,
						0xdd, 0xdb, 0x63, 0xa4, 0xb6,
						0x88, 0x33, 0x92, 0x06, 0xdd,
						0xf9, 0xee, 0x72, 0x97, 0xa3,
						0x08, 0x02, 0x20, 0x25, 0x00,
						0x42, 0x8b, 0x26, 0x36, 0x45,
						0x54, 0xcb, 0x11, 0xd3, 0x3e,
						0x85, 0x35, 0x23, 0x49, 0x65,
						0x82, 0x8e, 0x33, 0x6e, 0x1a,
						0x4a, 0x72, 0x73, 0xeb, 0x5b,
						0x8d, 0x1d, 0xd7, 0x02, 0xcc,
						0x01,
						btcscript.OP_DATA_65,
						0x04, 0x49, 0x5c, 0x8f, 0x66,
						0x90, 0x0d, 0xb7, 0x62, 0x69,
						0x0b, 0x54, 0x49, 0xa1, 0xf4,
						0xe7, 0xc2, 0xed, 0x1f, 0x4b,
						0x34, 0x70, 0xfd, 0x42, 0x79,
						0x68, 0xa1, 0x31, 0x76, 0x0c,
						0x25, 0xf9, 0x12, 0x63, 0xad,
						0x51, 0x73, 0x8e, 0x19, 0xb6,
						0x07, 0xf5, 0xcf, 0x5f, 0x94,
						0x27, 0x4a, 0x8b, 0xbc, 0x74,
						0xba, 0x4b, 0x56, 0x0c, 0xe0,
						0xb3, 0x08, 0x8f, 0x7f, 0x5c,
						0x5f, 0xcf, 0xd6, 0xa0, 0x4b,
					},
					Sequence: 4294967295,
				},
			},
			TxOut: []*btcwire.TxOut{
				{
					Value: 630320000,
					PkScript: []byte{
						btcscript.OP_DUP,
						btcscript.OP_HASH160,
						btcscript.OP_DATA_20,
						0x43, 0xdc, 0x32, 0x1b, 0x66,
						0x00, 0x51, 0x1f, 0xe0, 0xa9,
						0x6a, 0x97, 0xc2, 0x59, 0x3a,
						0x90, 0x54, 0x29, 0x74, 0xd6,
						btcscript.OP_EQUALVERIFY,
						btcscript.OP_CHECKSIG,
					},
				},
				{
					Value: 100000181,
					PkScript: []byte{
						btcscript.OP_DUP,
						btcscript.OP_HASH160,
						btcscript.OP_DATA_20,
						0xa4, 0x05, 0xea, 0x18, 0x09,
						0x14, 0xa9, 0x11, 0xd0, 0xb8,
						0x07, 0x99, 0x19, 0x2b, 0x0b,
						0x84, 0xae, 0x80, 0x1e, 0xbd,
						btcscript.OP_EQUALVERIFY,
						btcscript.OP_CHECKSIG,
					},
				},
				{
					Value: 596516343,
					PkScript: []byte{
						btcscript.OP_DUP,
						btcscript.OP_HASH160,
						btcscript.OP_DATA_20,
						0x24, 0x56, 0x76, 0x45, 0x4f,
						0x6f, 0xff, 0x28, 0x88, 0x39,
						0x47, 0xea, 0x70, 0x23, 0x86,
						0x9b, 0x8a, 0x71, 0xa3, 0x05,
						btcscript.OP_EQUALVERIFY,
						btcscript.OP_CHECKSIG,
					},
				},
			},
			LockTime: 0,
		},
		// Test input 0
		pkScript: []byte{
			btcscript.OP_DUP,
			btcscript.OP_HASH160,
			btcscript.OP_DATA_20,
			0xfd, 0xf6, 0xea, 0xe7, 0x10,
			0xa0, 0xc4, 0x49, 0x7a, 0x8d,
			0x0f, 0xd2, 0x9a, 0xf6, 0x6b,
			0xac, 0x94, 0xaf, 0x6c, 0x98,
			btcscript.OP_EQUALVERIFY,
			btcscript.OP_CHECKSIG,
		},
		idx:           0,
		canonicalSigs: true,
		shouldFail:    true,
		nSigOps:       1,
		scriptInfo: btcscript.ScriptInfo{
			PkScriptClass:  btcscript.PubKeyHashTy,
			NumInputs:      2,
			ExpectedInputs: 2,
			SigOps:         1,
		},
	},

	// tx 51bf528ecf3c161e7c021224197dbe84f9a8564212f6207baa014c01a1668e1e
	// first instance of an AnyoneCanPay signature in the blockchain
	{
		name: "CheckSigHashAnyoneCanPay",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0xf6, 0x04, 0x4c, 0x0a,
							0xd4, 0x85, 0xf6, 0x33,
							0xb4, 0x1f, 0x97, 0xd0,
							0xd7, 0x93, 0xeb, 0x28,
							0x37, 0xae, 0x40, 0xf7,
							0x38, 0xff, 0x6d, 0x5f,
							0x50, 0xfd, 0xfd, 0x10,
							0x52, 0x8c, 0x1d, 0x76,
						}),
						Index: 1,
					},
					SignatureScript: []byte{
						btcscript.OP_DATA_72,
						0x30, 0x45, 0x02, 0x20, 0x58,
						0x53, 0xc7, 0xf1, 0x39, 0x57,
						0x85, 0xbf, 0xab, 0xb0, 0x3c,
						0x57, 0xe9, 0x62, 0xeb, 0x07,
						0x6f, 0xf2, 0x4d, 0x8e, 0x4e,
						0x57, 0x3b, 0x04, 0xdb, 0x13,
						0xb4, 0x5e, 0xd3, 0xed, 0x6e,
						0xe2, 0x02, 0x21, 0x00, 0x9d,
						0xc8, 0x2a, 0xe4, 0x3b, 0xe9,
						0xd4, 0xb1, 0xfe, 0x28, 0x47,
						0x75, 0x4e, 0x1d, 0x36, 0xda,
						0xd4, 0x8b, 0xa8, 0x01, 0x81,
						0x7d, 0x48, 0x5d, 0xc5, 0x29,
						0xaf, 0xc5, 0x16, 0xc2, 0xdd,
						0xb4, 0x81,
						btcscript.OP_DATA_33,
						0x03, 0x05, 0x58, 0x49, 0x80,
						0x36, 0x7b, 0x32, 0x1f, 0xad,
						0x7f, 0x1c, 0x1f, 0x4d, 0x5d,
						0x72, 0x3d, 0x0a, 0xc8, 0x0c,
						0x1d, 0x80, 0xc8, 0xba, 0x12,
						0x34, 0x39, 0x65, 0xb4, 0x83,
						0x64, 0x53, 0x7a,
					},
					Sequence: 4294967295,
				},
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0x9c, 0x6a, 0xf0, 0xdf,
							0x66, 0x69, 0xbc, 0xde,
							0xd1, 0x9e, 0x31, 0x7e,
							0x25, 0xbe, 0xbc, 0x8c,
							0x78, 0xe4, 0x8d, 0xf8,
							0xae, 0x1f, 0xe0, 0x2a,
							0x7f, 0x03, 0x08, 0x18,
							0xe7, 0x1e, 0xcd, 0x40,
						}),
						Index: 1,
					},
					SignatureScript: []byte{
						btcscript.OP_DATA_73,
						0x30, 0x46, 0x02, 0x21, 0x00,
						0x82, 0x69, 0xc9, 0xd7, 0xba,
						0x0a, 0x7e, 0x73, 0x0d, 0xd1,
						0x6f, 0x40, 0x82, 0xd2, 0x9e,
						0x36, 0x84, 0xfb, 0x74, 0x63,
						0xba, 0x06, 0x4f, 0xd0, 0x93,
						0xaf, 0xc1, 0x70, 0xad, 0x6e,
						0x03, 0x88, 0x02, 0x21, 0x00,
						0xbc, 0x6d, 0x76, 0x37, 0x39,
						0x16, 0xa3, 0xff, 0x6e, 0xe4,
						0x1b, 0x2c, 0x75, 0x20, 0x01,
						0xfd, 0xa3, 0xc9, 0xe0, 0x48,
						0xbc, 0xff, 0x0d, 0x81, 0xd0,
						0x5b, 0x39, 0xff, 0x0f, 0x42,
						0x17, 0xb2, 0x81,
						btcscript.OP_DATA_33,
						0x03, 0xaa, 0xe3, 0x03, 0xd8,
						0x25, 0x42, 0x15, 0x45, 0xc5,
						0xbc, 0x7c, 0xcd, 0x5a, 0xc8,
						0x7d, 0xd5, 0xad, 0xd3, 0xbc,
						0xc3, 0xa4, 0x32, 0xba, 0x7a,
						0xa2, 0xf2, 0x66, 0x16, 0x99,
						0xf9, 0xf6, 0x59,
					},
					Sequence: 4294967295,
				},
			},
			TxOut: []*btcwire.TxOut{
				{
					Value: 300000,
					PkScript: []byte{
						btcscript.OP_DUP,
						btcscript.OP_HASH160,
						btcscript.OP_DATA_20,
						0x5c, 0x11, 0xf9, 0x17, 0x88,
						0x3b, 0x92, 0x7e, 0xef, 0x77,
						0xdc, 0x57, 0x70, 0x7a, 0xeb,
						0x85, 0x3f, 0x6d, 0x38, 0x94,
						btcscript.OP_EQUALVERIFY,
						btcscript.OP_CHECKSIG,
					},
				},
			},
			LockTime: 0,
		},
		pkScript: []byte{
			btcscript.OP_DUP,
			btcscript.OP_HASH160,
			btcscript.OP_DATA_20,
			0x85, 0x51, 0xe4, 0x8a, 0x53, 0xde, 0xcd, 0x1c, 0xfc,
			0x63, 0x07, 0x9a, 0x45, 0x81, 0xbc, 0xcc, 0xfa, 0xd1,
			0xa9, 0x3c,
			btcscript.OP_EQUALVERIFY,
			btcscript.OP_CHECKSIG,
		},
		idx:     0,
		bip16:   true, // after threshold
		nSigOps: 1,
		scriptInfo: btcscript.ScriptInfo{
			PkScriptClass:  btcscript.PubKeyHashTy,
			NumInputs:      2,
			ExpectedInputs: 2,
			SigOps:         1,
		},
	},
	// tx 6d36bc17e947ce00bb6f12f8e7a56a1585c5a36188ffa2b05e10b4743273a74b
	// Uses OP_CODESEPARATOR and OP_CHECKMULTISIG
	{
		name: "CheckMultiSig",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0x37, 0xb1, 0x7d, 0x76,
							0x38, 0x51, 0xcd, 0x1a,
							0xb0, 0x4a, 0x42, 0x44,
							0x63, 0xd4, 0x13, 0xc4,
							0xee, 0x5c, 0xf6, 0x13,
							0x04, 0xc7, 0xfd, 0x76,
							0x97, 0x7b, 0xea, 0x7f,
							0xce, 0x07, 0x57, 0x05,
						}),
						Index: 0,
					},
					SignatureScript: []byte{
						btcscript.OP_DATA_71,
						0x30, 0x44, 0x02, 0x20, 0x02,
						0xdb, 0xe4, 0xb5, 0xa2, 0xfb,
						0xb5, 0x21, 0xe4, 0xdc, 0x5f,
						0xbe, 0xc7, 0x5f, 0xd9, 0x60,
						0x65, 0x1a, 0x27, 0x54, 0xb0,
						0x3d, 0x08, 0x71, 0xb8, 0xc9,
						0x65, 0x46, 0x9b, 0xe5, 0x0f,
						0xa7, 0x02, 0x20, 0x6d, 0x97,
						0x42, 0x1f, 0xb7, 0xea, 0x93,
						0x59, 0xb6, 0x3e, 0x48, 0xc2,
						0x10, 0x82, 0x23, 0x28, 0x4b,
						0x9a, 0x71, 0x56, 0x0b, 0xd8,
						0x18, 0x24, 0x69, 0xb9, 0x03,
						0x92, 0x28, 0xd7, 0xb3, 0xd7,
						0x01, 0x21, 0x02, 0x95, 0xbf,
						0x72, 0x71, 0x11, 0xac, 0xde,
						0xab, 0x87, 0x78, 0x28, 0x4f,
						0x02, 0xb7, 0x68, 0xd1, 0xe2,
						0x1a, 0xcb, 0xcb, 0xae, 0x42,
					},
					Sequence: 4294967295,
				},
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0x37, 0xb1, 0x7d, 0x76,
							0x38, 0x51, 0xcd, 0x1a,
							0xb0, 0x4a, 0x42, 0x44,
							0x63, 0xd4, 0x13, 0xc4,
							0xee, 0x5c, 0xf6, 0x13,
							0x04, 0xc7, 0xfd, 0x76,
							0x97, 0x7b, 0xea, 0x7f,
							0xce, 0x07, 0x57, 0x05,
						}),
						Index: 1,
					},
					SignatureScript: []uint8{
						btcscript.OP_FALSE,
						btcscript.OP_DATA_72,
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
						btcscript.OP_CODESEPARATOR,
						btcscript.OP_TRUE,
						btcscript.OP_DATA_33,
						0x02, 0x32, 0xab, 0xdc, 0x89,
						0x3e, 0x7f, 0x06, 0x31, 0x36,
						0x4d, 0x7f, 0xd0, 0x1c, 0xb3,
						0x3d, 0x24, 0xda, 0x45, 0x32,
						0x9a, 0x00, 0x35, 0x7b, 0x3a,
						0x78, 0x86, 0x21, 0x1a, 0xb4,
						0x14, 0xd5, 0x5a,
						btcscript.OP_TRUE,
						btcscript.OP_CHECKMULTISIG,
					},
					Sequence: 4294967295,
				},
			},
			TxOut: []*btcwire.TxOut{
				{
					Value: 4800000,
					PkScript: []byte{
						btcscript.OP_DUP,
						btcscript.OP_HASH160,
						btcscript.OP_DATA_20,
						0x0d, 0x77, 0x13, 0x64, 0x9f,
						0x9a, 0x06, 0x78, 0xf4, 0xe8,
						0x80, 0xb4, 0x0f, 0x86, 0xb9,
						0x32, 0x89, 0xd1, 0xbb, 0x27,
						btcscript.OP_EQUALVERIFY,
						btcscript.OP_CHECKSIG,
					},
				},
			},
			LockTime: 0,
		},
		// This is a very weird script...
		pkScript: []byte{
			btcscript.OP_DATA_20,
			0x2a, 0x9b, 0xc5, 0x44, 0x7d, 0x66, 0x4c, 0x1d, 0x01,
			0x41, 0x39, 0x2a, 0x84, 0x2d, 0x23, 0xdb, 0xa4, 0x5c,
			0x4f, 0x13,
			btcscript.OP_NOP2, btcscript.OP_DROP,
		},
		idx:           1,
		bip16:         false,
		nSigOps:       0, // multisig is in the pkScript!
		scriptInfoErr: btcscript.StackErrNonPushOnly,
	},
	// same as previous but with one byte changed to make signature fail
	{
		name: "CheckMultiSig fail",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0x37, 0xb1, 0x7d, 0x76,
							0x38, 0x51, 0xcd, 0x1a,
							0xb0, 0x4a, 0x42, 0x44,
							0x63, 0xd4, 0x13, 0xc4,
							0xee, 0x5c, 0xf6, 0x13,
							0x04, 0xc7, 0xfd, 0x76,
							0x97, 0x7b, 0xea, 0x7f,
							0xce, 0x07, 0x57, 0x05,
						}),
						Index: 0,
					},
					SignatureScript: []byte{
						btcscript.OP_DATA_71,
						0x30, 0x44, 0x02, 0x20, 0x02,
						0xdb, 0xe4, 0xb5, 0xa2, 0xfb,
						0xb5, 0x21, 0xe4, 0xdc, 0x5f,
						0xbe, 0xc7, 0x5f, 0xd9, 0x60,
						0x65, 0x1a, 0x27, 0x54, 0xb0,
						0x3d, 0x08, 0x71, 0xb8, 0xc9,
						0x65, 0x46, 0x9b, 0xe5, 0x0f,
						0xa7, 0x02, 0x20, 0x6d, 0x97,
						0x42, 0x1f, 0xb7, 0xea, 0x93,
						0x59, 0xb6, 0x3e, 0x48, 0xc2,
						0x10, 0x82, 0x23, 0x28, 0x4b,
						0x9a, 0x71, 0x56, 0x0b, 0xd8,
						0x18, 0x24, 0x69, 0xb9, 0x03,
						0x92, 0x28, 0xd7, 0xb3, 0xd7,
						0x01, 0x21, 0x02, 0x95, 0xbf,
						0x72, 0x71, 0x11, 0xac, 0xde,
						0xab, 0x87, 0x78, 0x28, 0x4f,
						0x02, 0xb7, 0x68, 0xd1, 0xe2,
						0x1a, 0xcb, 0xcb, 0xae, 0x42,
					},
					Sequence: 4294967295,
				},
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0x37, 0xb1, 0x7d, 0x76,
							0x38, 0x51, 0xcd, 0x1a,
							0xb0, 0x4a, 0x42, 0x44,
							0x63, 0xd4, 0x13, 0xc4,
							0xee, 0x5c, 0xf6, 0x13,
							0x04, 0xc7, 0xfd, 0x76,
							0x97, 0x7b, 0xea, 0x7f,
							0xce, 0x07, 0x57, 0x05,
						}),
						Index: 1,
					},
					SignatureScript: []uint8{
						btcscript.OP_FALSE,
						btcscript.OP_DATA_72,
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
						btcscript.OP_CODESEPARATOR,
						btcscript.OP_TRUE,
						btcscript.OP_DATA_33,
						0x02, 0x32, 0xab, 0xdc, 0x89,
						0x3e, 0x7f, 0x06, 0x31, 0x36,
						0x4d, 0x7f, 0xd0, 0x1c, 0xb3,
						0x3d, 0x24, 0xda, 0x45, 0x32,
						0x9a, 0x00, 0x35, 0x7b, 0x3a,
						0x78, 0x86, 0x21, 0x1a, 0xb4,
						0x14, 0xd5, 0x5a,
						btcscript.OP_TRUE,
						btcscript.OP_CHECKMULTISIG,
					},
					Sequence: 4294967295,
				},
			},
			TxOut: []*btcwire.TxOut{
				{
					Value: 5800000,
					PkScript: []byte{
						btcscript.OP_DUP,
						btcscript.OP_HASH160,
						btcscript.OP_DATA_20,
						0x0d, 0x77, 0x13, 0x64, 0x9f,
						0x9a, 0x06, 0x78, 0xf4, 0xe8,
						0x80, 0xb4, 0x0f, 0x86, 0xb9,
						0x32, 0x89, 0xd1, 0xbb, 0x27,
						btcscript.OP_EQUALVERIFY,
						btcscript.OP_CHECKSIG,
					},
				},
			},
			LockTime: 0,
		},
		// This is a very weird script...
		pkScript: []byte{
			btcscript.OP_DATA_20,
			0x2a, 0x9b, 0xc5, 0x44, 0x7d, 0x66, 0x4c, 0x1d, 0x01,
			0x41, 0x39, 0x2a, 0x84, 0x2d, 0x23, 0xdb, 0xa4, 0x5c,
			0x4f, 0x13,
			btcscript.OP_NOP2, btcscript.OP_DROP,
		},
		idx:           1,
		bip16:         false,
		err:           btcscript.StackErrScriptFailed,
		nSigOps:       0, // multisig is in the pkScript!
		scriptInfoErr: btcscript.StackErrNonPushOnly,
	},
	// taken from tx b2d93dfd0b2c1a380e55e76a8d9cb3075dec9f4474e9485be008c337fd62c1f7
	// on testnet
	// multisig with zero required signatures
	{
		name: "CheckMultiSig zero required signatures",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0x37, 0xb1, 0x7d, 0x76,
							0x38, 0x51, 0xcd, 0x1a,
							0xb0, 0x4a, 0x42, 0x44,
							0x63, 0xd4, 0x13, 0xc4,
							0xee, 0x5c, 0xf6, 0x13,
							0x04, 0xc7, 0xfd, 0x76,
							0x97, 0x7b, 0xea, 0x7f,
							0xce, 0x07, 0x57, 0x05,
						}),
						Index: 0,
					},
					SignatureScript: []byte{
						btcscript.OP_0,
						btcscript.OP_DATA_37,
						btcscript.OP_0,
						btcscript.OP_DATA_33,
						0x02, 0x4a, 0xb3, 0x3c, 0x3a,
						0x54, 0x7a, 0x37, 0x29, 0x3e,
						0xb8, 0x75, 0xb4, 0xbb, 0xdb,
						0xd4, 0x73, 0xe9, 0xd4, 0xba,
						0xfd, 0xf3, 0x56, 0x87, 0xe7,
						0x97, 0x44, 0xdc, 0xd7, 0x0f,
						0x6e, 0x4d, 0xe2,
						btcscript.OP_1,
						btcscript.OP_CHECKMULTISIG,
					},
					Sequence: 4294967295,
				},
			},
			TxOut:    []*btcwire.TxOut{},
			LockTime: 0,
		},
		pkScript: []byte{
			btcscript.OP_HASH160,
			btcscript.OP_DATA_20,
			0x2c, 0x6b, 0x10, 0x7f, 0xdf, 0x10, 0x6f, 0x22, 0x6f,
			0x3f, 0xa3, 0x27, 0xba, 0x36, 0xd6, 0xe3, 0xca, 0xc7,
			0x3d, 0xf0,
			btcscript.OP_EQUAL,
		},
		idx:     0,
		bip16:   true,
		nSigOps: 1,
		scriptInfo: btcscript.ScriptInfo{
			PkScriptClass:  btcscript.ScriptHashTy,
			NumInputs:      2,
			ExpectedInputs: 2,
			SigOps:         1,
		},
	},
	// tx e5779b9e78f9650debc2893fd9636d827b26b4ddfa6a8172fe8708c924f5c39d
	// First P2SH transaction in the blockchain
	{
		name: "P2SH",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0x6d, 0x58, 0xf8, 0xa3,
							0xaa, 0x43, 0x0b, 0x84,
							0x78, 0x52, 0x3a, 0x65,
							0xc2, 0x03, 0xa2, 0x7b,
							0xb8, 0x81, 0x17, 0x8c,
							0xb1, 0x23, 0x13, 0xaf,
							0xde, 0x29, 0xf9, 0x2e,
							0xd7, 0x56, 0xaa, 0x7e,
						}),
						Index: 0,
					},
					SignatureScript: []byte{
						btcscript.OP_DATA_2,
						// OP_3 OP_7
						0x53, 0x57,
					},
					Sequence: 4294967295,
				},
			},
			TxOut: []*btcwire.TxOut{
				{
					Value: 1000000,
					PkScript: []byte{
						btcscript.OP_DUP,
						btcscript.OP_HASH160,
						btcscript.OP_DATA_20,
						0x5b, 0x69, 0xd8, 0xb9, 0xdf,
						0xa6, 0xe4, 0x12, 0x26, 0x47,
						0xe1, 0x79, 0x4e, 0xaa, 0x3b,
						0xfc, 0x11, 0x1f, 0x70, 0xef,
						btcscript.OP_EQUALVERIFY,
						btcscript.OP_CHECKSIG,
					},
				},
			},
			LockTime: 0,
		},
		pkScript: []byte{
			btcscript.OP_HASH160,
			btcscript.OP_DATA_20,
			0x43, 0x3e, 0xc2, 0xac, 0x1f, 0xfa, 0x1b, 0x7b, 0x7d,
			0x02, 0x7f, 0x56, 0x45, 0x29, 0xc5, 0x71, 0x97, 0xf9,
			0xae, 0x88,
			btcscript.OP_EQUAL,
		},
		idx:     0,
		bip16:   true,
		nSigOps: 0, // no signature ops in the pushed script.
		scriptInfo: btcscript.ScriptInfo{
			PkScriptClass:  btcscript.ScriptHashTy,
			NumInputs:      1,
			ExpectedInputs: -1, // p2sh script is non standard
			SigOps:         0,
		},
	},
	// next few tests are modified versions of previous to hit p2sh error
	// cases.
	{
		// sigscript changed so that pkscript hash will not match.
		name: "P2SH - bad hash",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0x6d, 0x58, 0xf8, 0xa3,
							0xaa, 0x43, 0x0b, 0x84,
							0x78, 0x52, 0x3a, 0x65,
							0xc2, 0x03, 0xa2, 0x7b,
							0xb8, 0x81, 0x17, 0x8c,
							0xb1, 0x23, 0x13, 0xaf,
							0xde, 0x29, 0xf9, 0x2e,
							0xd7, 0x56, 0xaa, 0x7e,
						}),
						Index: 0,
					},
					SignatureScript: []byte{
						btcscript.OP_DATA_2,
						// OP_3 OP_8
						0x53, 0x58,
					},
					Sequence: 4294967295,
				},
			},
			TxOut: []*btcwire.TxOut{
				{
					Value: 1000000,
					PkScript: []byte{
						btcscript.OP_DUP,
						btcscript.OP_HASH160,
						btcscript.OP_DATA_20,
						0x5b, 0x69, 0xd8, 0xb9, 0xdf,
						0xa6, 0xe4, 0x12, 0x26, 0x47,
						0xe1, 0x79, 0x4e, 0xaa, 0x3b,
						0xfc, 0x11, 0x1f, 0x70, 0xef,
						btcscript.OP_EQUALVERIFY,
						btcscript.OP_CHECKSIG,
					},
				},
			},
			LockTime: 0,
		},
		pkScript: []byte{
			btcscript.OP_HASH160,
			btcscript.OP_DATA_20,
			0x43, 0x3e, 0xc2, 0xac, 0x1f, 0xfa, 0x1b, 0x7b, 0x7d,
			0x02, 0x7f, 0x56, 0x45, 0x29, 0xc5, 0x71, 0x97, 0xf9,
			0xae, 0x88,
			btcscript.OP_EQUAL,
		},
		idx:     0,
		err:     btcscript.StackErrScriptFailed,
		bip16:   true,
		nSigOps: 0, // no signature ops in the pushed script.
		scriptInfo: btcscript.ScriptInfo{
			PkScriptClass:  btcscript.ScriptHashTy,
			NumInputs:      1,
			ExpectedInputs: -1, // p2sh script is non standard
			SigOps:         0,
		},
	},
	{
		// sigscript changed so that pkscript hash will not match.
		name: "P2SH - doesn't parse",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0x6d, 0x58, 0xf8, 0xa3,
							0xaa, 0x43, 0x0b, 0x84,
							0x78, 0x52, 0x3a, 0x65,
							0xc2, 0x03, 0xa2, 0x7b,
							0xb8, 0x81, 0x17, 0x8c,
							0xb1, 0x23, 0x13, 0xaf,
							0xde, 0x29, 0xf9, 0x2e,
							0xd7, 0x56, 0xaa, 0x7e,
						}),
						Index: 0,
					},
					SignatureScript: []byte{
						btcscript.OP_DATA_2,
						// pushed script.
						btcscript.OP_DATA_2, 0x1,
					},
					Sequence: 4294967295,
				},
			},
			TxOut: []*btcwire.TxOut{
				{
					Value: 1000000,
					PkScript: []byte{
						btcscript.OP_DUP,
						btcscript.OP_HASH160,
						btcscript.OP_DATA_20,
						0x5b, 0x69, 0xd8, 0xb9, 0xdf,
						0xa6, 0xe4, 0x12, 0x26, 0x47,
						0xe1, 0x79, 0x4e, 0xaa, 0x3b,
						0xfc, 0x11, 0x1f, 0x70, 0xef,
						btcscript.OP_EQUALVERIFY,
						btcscript.OP_CHECKSIG,
					},
				},
			},
			LockTime: 0,
		},
		pkScript: []byte{
			btcscript.OP_HASH160,
			btcscript.OP_DATA_20,
			0xd4, 0x8c, 0xe8, 0x6c, 0x69, 0x8f, 0x24, 0x68, 0x29,
			0x92, 0x1b, 0xa9, 0xfb, 0x2a, 0x84, 0x4a, 0xe2, 0xad,
			0xba, 0x67,
			btcscript.OP_EQUAL,
		},
		idx:           0,
		err:           btcscript.StackErrShortScript,
		bip16:         true,
		scriptInfoErr: btcscript.StackErrShortScript,
	},
	{
		// sigscript changed so to be non pushonly.
		name: "P2SH - non pushonly",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0x6d, 0x58, 0xf8, 0xa3,
							0xaa, 0x43, 0x0b, 0x84,
							0x78, 0x52, 0x3a, 0x65,
							0xc2, 0x03, 0xa2, 0x7b,
							0xb8, 0x81, 0x17, 0x8c,
							0xb1, 0x23, 0x13, 0xaf,
							0xde, 0x29, 0xf9, 0x2e,
							0xd7, 0x56, 0xaa, 0x7e,
						}),
						Index: 0,
					},
					// doesn't have to match signature.
					// will never run.
					SignatureScript: []byte{

						btcscript.OP_DATA_2,
						// pushed script.
						btcscript.OP_DATA_1, 0x1,
						btcscript.OP_DUP,
					},
					Sequence: 4294967295,
				},
			},
			TxOut: []*btcwire.TxOut{
				{
					Value: 1000000,
					PkScript: []byte{
						btcscript.OP_DUP,
						btcscript.OP_HASH160,
						btcscript.OP_DATA_20,
						0x5b, 0x69, 0xd8, 0xb9, 0xdf,
						0xa6, 0xe4, 0x12, 0x26, 0x47,
						0xe1, 0x79, 0x4e, 0xaa, 0x3b,
						0xfc, 0x11, 0x1f, 0x70, 0xef,
						btcscript.OP_EQUALVERIFY,
						btcscript.OP_CHECKSIG,
					},
				},
			},
			LockTime: 0,
		},
		pkScript: []byte{
			btcscript.OP_HASH160,
			btcscript.OP_DATA_20,
			0x43, 0x3e, 0xc2, 0xac, 0x1f, 0xfa, 0x1b, 0x7b, 0x7d,
			0x02, 0x7f, 0x56, 0x45, 0x29, 0xc5, 0x71, 0x97, 0xf9,
			0xae, 0x88,
			btcscript.OP_EQUAL,
		},
		idx:           0,
		parseErr:      btcscript.StackErrP2SHNonPushOnly,
		bip16:         true,
		nSigOps:       0, // no signature ops in the pushed script.
		scriptInfoErr: btcscript.StackErrNonPushOnly,
	},
	{
		// sigscript changed so to be non pushonly.
		name: "empty pkScript",
		tx: &btcwire.MsgTx{
			Version: 1,
			TxIn: []*btcwire.TxIn{
				{
					PreviousOutPoint: btcwire.OutPoint{
						Hash: btcwire.ShaHash([32]byte{
							0x6d, 0x58, 0xf8, 0xa3,
							0xaa, 0x43, 0x0b, 0x84,
							0x78, 0x52, 0x3a, 0x65,
							0xc2, 0x03, 0xa2, 0x7b,
							0xb8, 0x81, 0x17, 0x8c,
							0xb1, 0x23, 0x13, 0xaf,
							0xde, 0x29, 0xf9, 0x2e,
							0xd7, 0x56, 0xaa, 0x7e,
						}),
						Index: 0,
					},
					// doesn't have to match signature.
					// will never run.
					SignatureScript: []byte{
						btcscript.OP_TRUE,
					},
					Sequence: 4294967295,
				},
			},
			TxOut: []*btcwire.TxOut{
				{
					Value: 1000000,
					PkScript: []byte{
						btcscript.OP_DUP,
						btcscript.OP_HASH160,
						btcscript.OP_DATA_20,
						0x5b, 0x69, 0xd8, 0xb9, 0xdf,
						0xa6, 0xe4, 0x12, 0x26, 0x47,
						0xe1, 0x79, 0x4e, 0xaa, 0x3b,
						0xfc, 0x11, 0x1f, 0x70, 0xef,
						btcscript.OP_EQUALVERIFY,
						btcscript.OP_CHECKSIG,
					},
				},
			},
			LockTime: 0,
		},
		pkScript: []byte{},
		idx:      0,
		bip16:    true,
		nSigOps:  0, // no signature ops in the pushed script.
		scriptInfo: btcscript.ScriptInfo{
			PkScriptClass:  btcscript.NonStandardTy,
			NumInputs:      1,
			ExpectedInputs: -1,
			SigOps:         0,
		},
	},
}

// Test a number of tx from the blockchain to test otherwise difficult to test
// opcodes (i.e. those that involve signatures). Being from the blockchain,
// these transactions are known good.
// TODO(oga) For signatures we currently do not test SigHashSingle because
// nothing in the blockchain that we have yet seen uses them, making it hard
// to confirm we implemented the spec correctly.
func testTx(t *testing.T, test txTest) {
	var flags btcscript.ScriptFlags
	if test.bip16 {
		flags |= btcscript.ScriptBip16
	}
	if test.canonicalSigs {
		flags |= btcscript.ScriptCanonicalSignatures
	}
	engine, err := btcscript.NewScript(
		test.tx.TxIn[test.idx].SignatureScript, test.pkScript,
		test.idx, test.tx, flags)
	if err != nil {
		if err != test.parseErr {
			t.Errorf("Failed to parse %s: got \"%v\" expected "+
				"\"%v\"", test.name, err, test.parseErr)
		}
		return
	}
	if test.parseErr != nil {
		t.Errorf("%s: parse succeeded when expecting \"%v\"", test.name,
			test.parseErr)
	}

	err = engine.Execute()
	if err != nil {
		// failed means no specified error
		if test.shouldFail == true {
			return
		}
		if err != test.err {
			t.Errorf("Failed to validate %s tx: %v expected %v",
				test.name, err, test.err)
		}
		return
	}
	if test.err != nil || test.shouldFail == true {
		t.Errorf("%s: expected failure: %v, succeeded", test.name,
			test.err)
	}
}

func TestTX(t *testing.T) {
	for i := range txTests {
		testTx(t, txTests[i])
	}
}

func TestGetPreciseSignOps(t *testing.T) {
	// First we go over the range of tests in testTx and count the sigops in
	// them.
	for _, test := range txTests {
		count := btcscript.GetPreciseSigOpCount(
			test.tx.TxIn[test.idx].SignatureScript, test.pkScript,
			test.bip16)
		if count != test.nSigOps {
			t.Errorf("%s: expected count of %d, got %d", test.name,
				test.nSigOps, count)

		}
	}

	// Now we go over a number of tests to hit the more awkward error
	// conditions in the P2SH cases..

	type psocTest struct {
		name      string
		scriptSig []byte
		nSigOps   int
		err       error
	}
	psocTests := []psocTest{
		{
			name:      "scriptSig doesn't parse",
			scriptSig: []byte{btcscript.OP_PUSHDATA1, 2},
			err:       btcscript.StackErrShortScript,
		},
		{
			name:      "scriptSig isn't push only",
			scriptSig: []byte{btcscript.OP_1, btcscript.OP_DUP},
			nSigOps:   0,
		},
		{
			name:      "scriptSig length 0",
			scriptSig: []byte{},
			nSigOps:   0,
		},
		{
			name: "No script at the end",
			// No script at end but still push only.
			scriptSig: []byte{btcscript.OP_1, btcscript.OP_1},
			nSigOps:   0,
		},
		// pushed script doesn't parse.
		{
			name: "pushed script doesn't parse",
			scriptSig: []byte{btcscript.OP_DATA_2,
				btcscript.OP_PUSHDATA1, 2},
			err: btcscript.StackErrShortScript,
		},
	}
	// The signature in the p2sh script is nonsensical for the tests since
	// this script will never be executed. What matters is that it matches
	// the right pattern.
	pkScript := []byte{
		btcscript.OP_HASH160,
		btcscript.OP_DATA_20,
		0x43, 0x3e, 0xc2, 0xac, 0x1f, 0xfa, 0x1b, 0x7b, 0x7d,
		0x02, 0x7f, 0x56, 0x45, 0x29, 0xc5, 0x71, 0x97, 0xf9,
		0xae, 0x88,
		btcscript.OP_EQUAL,
	}
	for _, test := range psocTests {
		count := btcscript.GetPreciseSigOpCount(
			test.scriptSig, pkScript, true)
		if count != test.nSigOps {
			t.Errorf("%s: expected count of %d, got %d", test.name,
				test.nSigOps, count)

		}
	}
}

type scriptInfoTest struct {
	name          string
	sigScript     []byte
	pkScript      []byte
	bip16         bool
	scriptInfo    btcscript.ScriptInfo
	scriptInfoErr error
}

func TestScriptInfo(t *testing.T) {
	for _, test := range txTests {
		si, err := btcscript.CalcScriptInfo(
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
			sigScript: []byte{btcscript.OP_TRUE,
				btcscript.OP_DATA_1, 81,
				btcscript.OP_DATA_8,
				btcscript.OP_2DUP, btcscript.OP_EQUAL,
				btcscript.OP_NOT, btcscript.OP_VERIFY,
				btcscript.OP_ABS, btcscript.OP_SWAP,
				btcscript.OP_ABS, btcscript.OP_EQUAL,
			},
			// truncated version of test below:
			pkScript: []byte{btcscript.OP_HASH160,
				btcscript.OP_DATA_20,
				0xfe, 0x44, 0x10, 0x65, 0xb6, 0x53, 0x22, 0x31,
				0xde, 0x2f, 0xac, 0x56, 0x31, 0x52, 0x20, 0x5e,
				0xc4, 0xf5, 0x9c,
			},
			bip16:         true,
			scriptInfoErr: btcscript.StackErrShortScript,
		},
		{
			name: "sigScript doesn't parse",
			// Truncated version of p2sh script below.
			sigScript: []byte{btcscript.OP_TRUE,
				btcscript.OP_DATA_1, 81,
				btcscript.OP_DATA_8,
				btcscript.OP_2DUP, btcscript.OP_EQUAL,
				btcscript.OP_NOT, btcscript.OP_VERIFY,
				btcscript.OP_ABS, btcscript.OP_SWAP,
				btcscript.OP_ABS,
			},
			pkScript: []byte{btcscript.OP_HASH160,
				btcscript.OP_DATA_20,
				0xfe, 0x44, 0x10, 0x65, 0xb6, 0x53, 0x22, 0x31,
				0xde, 0x2f, 0xac, 0x56, 0x31, 0x52, 0x20, 0x5e,
				0xc4, 0xf5, 0x9c, 0x74, btcscript.OP_EQUAL,
			},
			bip16:         true,
			scriptInfoErr: btcscript.StackErrShortScript,
		},
		{
			// Invented scripts, the hashes do not match
			name: "p2sh standard script",
			sigScript: []byte{btcscript.OP_TRUE,
				btcscript.OP_DATA_1, 81,
				btcscript.OP_DATA_25,
				btcscript.OP_DUP, btcscript.OP_HASH160,
				btcscript.OP_DATA_20, 0x1, 0x2, 0x3, 0x4, 0x5,
				0x6, 0x7, 0x8, 0x9, 0xa, 0xb, 0xc, 0xd, 0xe,
				0xf, 0x10, 0x11, 0x12, 0x13, 0x14,
				btcscript.OP_EQUALVERIFY, btcscript.OP_CHECKSIG,
			},
			pkScript: []byte{btcscript.OP_HASH160,
				btcscript.OP_DATA_20,
				0xfe, 0x44, 0x10, 0x65, 0xb6, 0x53, 0x22, 0x31,
				0xde, 0x2f, 0xac, 0x56, 0x31, 0x52, 0x20, 0x5e,
				0xc4, 0xf5, 0x9c, 0x74, btcscript.OP_EQUAL,
			},
			bip16: true,
			scriptInfo: btcscript.ScriptInfo{
				PkScriptClass:  btcscript.ScriptHashTy,
				NumInputs:      3,
				ExpectedInputs: 3, // nonstandard p2sh.
				SigOps:         1,
			},
		},
		{
			// from 567a53d1ce19ce3d07711885168484439965501536d0d0294c5d46d46c10e53b
			// from the blockchain.
			name: "p2sh nonstandard script",
			sigScript: []byte{btcscript.OP_TRUE,
				btcscript.OP_DATA_1, 81,
				btcscript.OP_DATA_8,
				btcscript.OP_2DUP, btcscript.OP_EQUAL,
				btcscript.OP_NOT, btcscript.OP_VERIFY,
				btcscript.OP_ABS, btcscript.OP_SWAP,
				btcscript.OP_ABS, btcscript.OP_EQUAL,
			},
			pkScript: []byte{btcscript.OP_HASH160,
				btcscript.OP_DATA_20,
				0xfe, 0x44, 0x10, 0x65, 0xb6, 0x53, 0x22, 0x31,
				0xde, 0x2f, 0xac, 0x56, 0x31, 0x52, 0x20, 0x5e,
				0xc4, 0xf5, 0x9c, 0x74, btcscript.OP_EQUAL,
			},
			bip16: true,
			scriptInfo: btcscript.ScriptInfo{
				PkScriptClass:  btcscript.ScriptHashTy,
				NumInputs:      3,
				ExpectedInputs: -1, // nonstandard p2sh.
				SigOps:         0,
			},
		},
		{
			// Script is invented, numbers all fake.
			name: "multisig script",
			sigScript: []byte{btcscript.OP_TRUE,
				btcscript.OP_TRUE, btcscript.OP_TRUE,
				btcscript.OP_0, // Extra arg for OP_CHECKMULTISIG bug
			},
			pkScript: []byte{
				btcscript.OP_3, btcscript.OP_DATA_33,
				0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9,
				0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10, 0x11, 0x12,
				0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a,
				0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20, 0x21,
				btcscript.OP_DATA_33,
				0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9,
				0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10, 0x11, 0x12,
				0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a,
				0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20, 0x21,
				btcscript.OP_DATA_33,
				0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9,
				0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10, 0x11, 0x12,
				0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a,
				0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20, 0x21,
				btcscript.OP_3, btcscript.OP_CHECKMULTISIG,
			},
			bip16: true,
			scriptInfo: btcscript.ScriptInfo{
				PkScriptClass:  btcscript.MultiSigTy,
				NumInputs:      4,
				ExpectedInputs: 4,
				SigOps:         3,
			},
		},
	}

	for _, test := range extraTests {
		si, err := btcscript.CalcScriptInfo(test.sigScript,
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

type removeOpcodeTest struct {
	name   string
	before []byte
	remove byte
	err    error
	after  []byte
}

var removeOpcodeTests = []removeOpcodeTest{
	// Nothing to remove.
	{
		name:   "nothing to remove",
		before: []byte{btcscript.OP_NOP},
		remove: btcscript.OP_CODESEPARATOR,
		after:  []byte{btcscript.OP_NOP},
	},
	// Test basic opcode removal
	{
		name: "codeseparator 1",
		before: []byte{btcscript.OP_NOP, btcscript.OP_CODESEPARATOR,
			btcscript.OP_TRUE},
		remove: btcscript.OP_CODESEPARATOR,
		after:  []byte{btcscript.OP_NOP, btcscript.OP_TRUE},
	},
	// The opcode in question is actually part of the data in a previous
	// opcode
	{
		name: "codeseparator by coincidence",
		before: []byte{btcscript.OP_NOP, btcscript.OP_DATA_1, btcscript.OP_CODESEPARATOR,
			btcscript.OP_TRUE},
		remove: btcscript.OP_CODESEPARATOR,
		after: []byte{btcscript.OP_NOP, btcscript.OP_DATA_1, btcscript.OP_CODESEPARATOR,
			btcscript.OP_TRUE},
	},
	{
		name:   "invalid opcode",
		before: []byte{btcscript.OP_UNKNOWN186},
		remove: btcscript.OP_CODESEPARATOR,
		after:  []byte{btcscript.OP_UNKNOWN186},
	},
	{
		name:   "invalid length (insruction)",
		before: []byte{btcscript.OP_PUSHDATA1},
		remove: btcscript.OP_CODESEPARATOR,
		err:    btcscript.StackErrShortScript,
	},
	{
		name:   "invalid length (data)",
		before: []byte{btcscript.OP_PUSHDATA1, 255, 254},
		remove: btcscript.OP_CODESEPARATOR,
		err:    btcscript.StackErrShortScript,
	},
}

func testRemoveOpcode(t *testing.T, test *removeOpcodeTest) {
	result, err := btcscript.TstRemoveOpcode(test.before, test.remove)
	if test.err != nil {
		if err != test.err {
			t.Errorf("%s: got unexpected error. exp: \"%v\" "+
				"got: \"%v\"", test.name, test.err, err)
		}
		return
	}
	if err != nil {
		t.Errorf("%s: unexpected failure: \"%v\"", test.name, err)
		return
	}
	if !bytes.Equal(test.after, result) {
		t.Errorf("%s: value does not equal expected: exp: \"%v\""+
			" got: \"%v\"", test.name, test.after, result)
	}
}

func TestRemoveOpcodes(t *testing.T) {
	for i := range removeOpcodeTests {
		testRemoveOpcode(t, &removeOpcodeTests[i])
	}
}

type removeOpcodeByDataTest struct {
	name   string
	before []byte
	remove []byte
	err    error
	after  []byte
}

var removeOpcodeByDataTests = []removeOpcodeByDataTest{
	{
		name:   "nothing to do",
		before: []byte{btcscript.OP_NOP},
		remove: []byte{1, 2, 3, 4},
		after:  []byte{btcscript.OP_NOP},
	},
	{
		name:   "simple case",
		before: []byte{btcscript.OP_DATA_4, 1, 2, 3, 4},
		remove: []byte{1, 2, 3, 4},
		after:  []byte{},
	},
	{
		name:   "simple case (miss)",
		before: []byte{btcscript.OP_DATA_4, 1, 2, 3, 4},
		remove: []byte{1, 2, 3, 5},
		after:  []byte{btcscript.OP_DATA_4, 1, 2, 3, 4},
	},
	{
		// padded to keep it canonical.
		name: "simple case (pushdata1)",
		before: append(append([]byte{btcscript.OP_PUSHDATA1, 76},
			bytes.Repeat([]byte{0}, 72)...), []byte{1, 2, 3, 4}...),
		remove: []byte{1, 2, 3, 4},
		after:  []byte{},
	},
	{
		name: "simple case (pushdata1 miss)",
		before: append(append([]byte{btcscript.OP_PUSHDATA1, 76},
			bytes.Repeat([]byte{0}, 72)...), []byte{1, 2, 3, 4}...),
		remove: []byte{1, 2, 3, 5},
		after: append(append([]byte{btcscript.OP_PUSHDATA1, 76},
			bytes.Repeat([]byte{0}, 72)...), []byte{1, 2, 3, 4}...),
	},
	{
		name:   "simple case (pushdata1 miss noncanonical)",
		before: []byte{btcscript.OP_PUSHDATA1, 4, 1, 2, 3, 4},
		remove: []byte{1, 2, 3, 4},
		after:  []byte{btcscript.OP_PUSHDATA1, 4, 1, 2, 3, 4},
	},
	{
		name: "simple case (pushdata2)",
		before: append(append([]byte{btcscript.OP_PUSHDATA2, 0, 1},
			bytes.Repeat([]byte{0}, 252)...), []byte{1, 2, 3, 4}...),
		remove: []byte{1, 2, 3, 4},
		after:  []byte{},
	},
	{
		name: "simple case (pushdata2 miss)",
		before: append(append([]byte{btcscript.OP_PUSHDATA2, 0, 1},
			bytes.Repeat([]byte{0}, 252)...), []byte{1, 2, 3, 4}...),
		remove: []byte{1, 2, 3, 4, 5},
		after: append(append([]byte{btcscript.OP_PUSHDATA2, 0, 1},
			bytes.Repeat([]byte{0}, 252)...), []byte{1, 2, 3, 4}...),
	},
	{
		name:   "simple case (pushdata2 miss noncanonical)",
		before: []byte{btcscript.OP_PUSHDATA2, 4, 0, 1, 2, 3, 4},
		remove: []byte{1, 2, 3, 4},
		after:  []byte{btcscript.OP_PUSHDATA2, 4, 0, 1, 2, 3, 4},
	},
	{
		// This is padded to make the push canonical.
		name: "simple case (pushdata4)",
		before: append(append([]byte{btcscript.OP_PUSHDATA4, 0, 0, 1, 0},
			bytes.Repeat([]byte{0}, 65532)...), []byte{1, 2, 3, 4}...),
		remove: []byte{1, 2, 3, 4},
		after:  []byte{},
	},
	{
		name:   "simple case (pushdata4 miss noncanonical)",
		before: []byte{btcscript.OP_PUSHDATA4, 4, 0, 0, 0, 1, 2, 3, 4},
		remove: []byte{1, 2, 3, 4},
		after:  []byte{btcscript.OP_PUSHDATA4, 4, 0, 0, 0, 1, 2, 3, 4},
	},
	{
		// This is padded to make the push canonical.
		name: "simple case (pushdata4 miss)",
		before: append(append([]byte{btcscript.OP_PUSHDATA4, 0, 0, 1, 0},
			bytes.Repeat([]byte{0}, 65532)...), []byte{1, 2, 3, 4}...),
		remove: []byte{1, 2, 3, 4, 5},
		after: append(append([]byte{btcscript.OP_PUSHDATA4, 0, 0, 1, 0},
			bytes.Repeat([]byte{0}, 65532)...), []byte{1, 2, 3, 4}...),
	},
	{
		name:   "invalid opcode ",
		before: []byte{btcscript.OP_UNKNOWN187},
		remove: []byte{1, 2, 3, 4},
		after:  []byte{btcscript.OP_UNKNOWN187},
	},
	{
		name:   "invalid length (instruction)",
		before: []byte{btcscript.OP_PUSHDATA1},
		remove: []byte{1, 2, 3, 4},
		err:    btcscript.StackErrShortScript,
	},
	{
		name:   "invalid length (data)",
		before: []byte{btcscript.OP_PUSHDATA1, 255, 254},
		remove: []byte{1, 2, 3, 4},
		err:    btcscript.StackErrShortScript,
	},
}

func testRemoveOpcodeByData(t *testing.T, test *removeOpcodeByDataTest) {
	result, err := btcscript.TstRemoveOpcodeByData(test.before,
		test.remove)
	if test.err != nil {
		if err != test.err {
			t.Errorf("%s: got unexpected error. exp: \"%v\" "+
				"got: \"%v\"", test.name, test.err, err)
		}
		return
	}
	if err != nil {
		t.Errorf("%s: unexpected failure: \"%v\"", test.name, err)
		return
	}
	if !bytes.Equal(test.after, result) {
		t.Errorf("%s: value does not equal expected: exp: \"%v\""+
			" got: \"%v\"", test.name, test.after, result)
	}
}
func TestRemoveOpcodeByDatas(t *testing.T) {
	for i := range removeOpcodeByDataTests {
		testRemoveOpcodeByData(t, &removeOpcodeByDataTests[i])
	}
}

// Tests for the script type logic

type scriptTypeTest struct {
	name       string
	script     []byte
	scripttype btcscript.ScriptClass
}

var scriptTypeTests = []scriptTypeTest{
	// tx 0437cd7f8525ceed2324359c2d0ba26006d92d85.
	{
		name: "Pay Pubkey",
		script: []byte{
			btcscript.OP_DATA_65,
			0x04, 0x11, 0xdb, 0x93, 0xe1, 0xdc, 0xdb, 0x8a, 0x01,
			0x6b, 0x49, 0x84, 0x0f, 0x8c, 0x53, 0xbc, 0x1e, 0xb6,
			0x8a, 0x38, 0x2e, 0x97, 0xb1, 0x48, 0x2e, 0xca, 0xd7,
			0xb1, 0x48, 0xa6, 0x90, 0x9a, 0x5c, 0xb2, 0xe0, 0xea,
			0xdd, 0xfb, 0x84, 0xcc, 0xf9, 0x74, 0x44, 0x64, 0xf8,
			0x2e, 0x16, 0x0b, 0xfa, 0x9b, 0x8b, 0x64, 0xf9, 0xd4,
			0xc0, 0x3f, 0x99, 0x9b, 0x86, 0x43, 0xf6, 0x56, 0xb4,
			0x12, 0xa3,
			btcscript.OP_CHECKSIG,
		},
		scripttype: btcscript.PubKeyTy,
	},
	// tx 599e47a8114fe098103663029548811d2651991b62397e057f0c863c2bc9f9ea
	{
		name: "Pay PubkeyHash",
		script: []byte{
			btcscript.OP_DUP,
			btcscript.OP_HASH160,
			btcscript.OP_DATA_20,
			0x66, 0x0d, 0x4e, 0xf3, 0xa7, 0x43, 0xe3, 0xe6, 0x96,
			0xad, 0x99, 0x03, 0x64, 0xe5, 0x55, 0xc2, 0x71, 0xad,
			0x50, 0x4b,
			btcscript.OP_EQUALVERIFY,
			btcscript.OP_CHECKSIG,
		},
		scripttype: btcscript.PubKeyHashTy,
	},
	// part of tx 6d36bc17e947ce00bb6f12f8e7a56a1585c5a36188ffa2b05e10b4743273a74b
	// codeseparator parts have been elided. (bitcoind's checks for multisig
	// type doesn't have codesep etc either.
	{
		name: "multisig",
		script: []byte{
			btcscript.OP_TRUE,
			btcscript.OP_DATA_33,
			0x02, 0x32, 0xab, 0xdc, 0x89, 0x3e, 0x7f, 0x06, 0x31,
			0x36, 0x4d, 0x7f, 0xd0, 0x1c, 0xb3, 0x3d, 0x24, 0xda,
			0x45, 0x32, 0x9a, 0x00, 0x35, 0x7b, 0x3a, 0x78, 0x86,
			0x21, 0x1a, 0xb4, 0x14, 0xd5, 0x5a,
			btcscript.OP_TRUE,
			btcscript.OP_CHECKMULTISIG,
		},
		scripttype: btcscript.MultiSigTy,
	},
	// tx e5779b9e78f9650debc2893fd9636d827b26b4ddfa6a8172fe8708c924f5c39d
	// P2SH
	{
		name: "P2SH",
		script: []byte{
			btcscript.OP_HASH160,
			btcscript.OP_DATA_20,
			0x43, 0x3e, 0xc2, 0xac, 0x1f, 0xfa, 0x1b, 0x7b, 0x7d,
			0x02, 0x7f, 0x56, 0x45, 0x29, 0xc5, 0x71, 0x97, 0xf9,
			0xae, 0x88,
			btcscript.OP_EQUAL,
		},
		scripttype: btcscript.ScriptHashTy,
	},
	// Nulldata with no data at all.
	{
		name: "nulldata",
		script: []byte{
			btcscript.OP_RETURN,
		},
		scripttype: btcscript.NullDataTy,
	},
	// Nulldata with small data.
	{
		name: "nulldata2",
		script: []byte{
			btcscript.OP_RETURN,
			btcscript.OP_DATA_8,
			0x04, 0x67, 0x08, 0xaf, 0xdb, 0x0f, 0xe5, 0x54,
		},
		scripttype: btcscript.NullDataTy,
	},
	// Nulldata with max allowed data.
	{
		name: "nulldata3",
		script: []byte{
			btcscript.OP_RETURN,
			btcscript.OP_PUSHDATA1,
			0x28, // 40
			0x04, 0x67, 0x08, 0xaf, 0xdb, 0x0f, 0xe5, 0x54,
			0x82, 0x71, 0x96, 0x7f, 0x1a, 0x67, 0x13, 0x0b,
			0x71, 0x05, 0xcd, 0x6a, 0x82, 0x8e, 0x03, 0x90,
			0x9a, 0x67, 0x96, 0x2e, 0x0e, 0xa1, 0xf6, 0x1d,
			0xeb, 0x64, 0x9f, 0x6b, 0xc3, 0xf4, 0xce, 0xf3,
		},
		scripttype: btcscript.NullDataTy,
	},
	// Nulldata with more than max allowed data (so therefore nonstandard)
	{
		name: "nulldata4",
		script: []byte{
			btcscript.OP_RETURN,
			btcscript.OP_PUSHDATA1,
			0x29, // 41
			0x04, 0x67, 0x08, 0xaf, 0xdb, 0x0f, 0xe5, 0x54,
			0x82, 0x71, 0x96, 0x7f, 0x1a, 0x67, 0x13, 0x0b,
			0x71, 0x05, 0xcd, 0x6a, 0x82, 0x8e, 0x03, 0x90,
			0x9a, 0x67, 0x96, 0x2e, 0x0e, 0xa1, 0xf6, 0x1d,
			0xeb, 0x64, 0x9f, 0x6b, 0xc3, 0xf4, 0xce, 0xf3,
			0x08,
		},
		scripttype: btcscript.NonStandardTy,
	},
	// Almost nulldata, but add an additional opcode after the data to make
	// it nonstandard.
	{
		name: "nulldata5",
		script: []byte{
			btcscript.OP_RETURN,
			btcscript.OP_DATA_1,
			0x04,
			btcscript.OP_TRUE,
		},
		scripttype: btcscript.NonStandardTy,
	}, // The next few are almost multisig (it is the more complex script type)
	// but with various changes to make it fail.
	{
		// multisig but funny nsigs..
		name: "strange 1",
		script: []byte{
			btcscript.OP_DUP,
			btcscript.OP_DATA_33,
			0x02, 0x32, 0xab, 0xdc, 0x89, 0x3e, 0x7f, 0x06, 0x31,
			0x36, 0x4d, 0x7f, 0xd0, 0x1c, 0xb3, 0x3d, 0x24, 0xda,
			0x45, 0x32, 0x9a, 0x00, 0x35, 0x7b, 0x3a, 0x78, 0x86,
			0x21, 0x1a, 0xb4, 0x14, 0xd5, 0x5a,
			btcscript.OP_TRUE,
			btcscript.OP_CHECKMULTISIG,
		},
		scripttype: btcscript.NonStandardTy,
	},
	{
		name: "strange 2",
		// multisig but funny pubkey.
		script: []byte{
			btcscript.OP_TRUE,
			btcscript.OP_TRUE,
			btcscript.OP_TRUE,
			btcscript.OP_CHECKMULTISIG,
		},
		scripttype: btcscript.NonStandardTy,
	},
	{
		name: "strange 3",
		// multisig but no matching npubkeys opcode.
		script: []byte{
			btcscript.OP_TRUE,
			btcscript.OP_DATA_33,
			0x02, 0x32, 0xab, 0xdc, 0x89, 0x3e, 0x7f, 0x06, 0x31,
			0x36, 0x4d, 0x7f, 0xd0, 0x1c, 0xb3, 0x3d, 0x24, 0xda,
			0x45, 0x32, 0x9a, 0x00, 0x35, 0x7b, 0x3a, 0x78, 0x86,
			0x21, 0x1a, 0xb4, 0x14, 0xd5, 0x5a,
			btcscript.OP_DATA_33,
			0x02, 0x32, 0xab, 0xdc, 0x89, 0x3e, 0x7f, 0x06, 0x31,
			0x36, 0x4d, 0x7f, 0xd0, 0x1c, 0xb3, 0x3d, 0x24, 0xda,
			0x45, 0x32, 0x9a, 0x00, 0x35, 0x7b, 0x3a, 0x78, 0x86,
			0x21, 0x1a, 0xb4, 0x14, 0xd5, 0x5a,
			// No number.
			btcscript.OP_CHECKMULTISIG,
		},
		scripttype: btcscript.NonStandardTy,
	},
	{
		name: "strange 4",
		// multisig but with multisigverify
		script: []byte{
			btcscript.OP_TRUE,
			btcscript.OP_DATA_33,
			0x02, 0x32, 0xab, 0xdc, 0x89, 0x3e, 0x7f, 0x06, 0x31,
			0x36, 0x4d, 0x7f, 0xd0, 0x1c, 0xb3, 0x3d, 0x24, 0xda,
			0x45, 0x32, 0x9a, 0x00, 0x35, 0x7b, 0x3a, 0x78, 0x86,
			0x21, 0x1a, 0xb4, 0x14, 0xd5, 0x5a,
			btcscript.OP_TRUE,
			btcscript.OP_CHECKMULTISIGVERIFY,
		},
		scripttype: btcscript.NonStandardTy,
	},
	{
		name: "strange 5",
		// multisig but wrong length.
		script: []byte{
			btcscript.OP_TRUE,
			btcscript.OP_CHECKMULTISIG,
		},
		scripttype: btcscript.NonStandardTy,
	},
	{
		name: "doesn't parse",
		script: []byte{
			btcscript.OP_DATA_5, 0x1, 0x2, 0x3, 0x4,
		},
		scripttype: btcscript.NonStandardTy,
	},
}

func testScriptType(t *testing.T, test *scriptTypeTest) {
	scripttype := btcscript.GetScriptClass(test.script)
	if scripttype != test.scripttype {
		t.Errorf("%s: expected %s got %s", test.name, test.scripttype,
			scripttype)
	}
}

func TestScriptTypes(t *testing.T) {
	for i := range scriptTypeTests {
		testScriptType(t, &scriptTypeTests[i])
	}
}

func TestIsPayToScriptHash(t *testing.T) {
	for _, test := range scriptTypeTests {
		shouldBe := (test.scripttype == btcscript.ScriptHashTy)
		p2sh := btcscript.IsPayToScriptHash(test.script)
		if p2sh != shouldBe {
			t.Errorf("%s: epxected p2sh %v, got %v", test.name,
				shouldBe, p2sh)
		}
	}
}

// This test sets the pc to a deliberately bad result then confirms that Step()
//  and Disasm fail correctly.
func TestBadPC(t *testing.T) {
	type pcTest struct {
		script, off int
	}
	pcTests := []pcTest{
		{
			script: 2,
			off:    0,
		},
		{
			script: 0,
			off:    2,
		},
	}
	// tx with almost empty scripts.
	tx := &btcwire.MsgTx{
		Version: 1,
		TxIn: []*btcwire.TxIn{
			{
				PreviousOutPoint: btcwire.OutPoint{
					Hash: btcwire.ShaHash([32]byte{
						0xc9, 0x97, 0xa5, 0xe5,
						0x6e, 0x10, 0x41, 0x02,
						0xfa, 0x20, 0x9c, 0x6a,
						0x85, 0x2d, 0xd9, 0x06,
						0x60, 0xa2, 0x0b, 0x2d,
						0x9c, 0x35, 0x24, 0x23,
						0xed, 0xce, 0x25, 0x85,
						0x7f, 0xcd, 0x37, 0x04,
					}),
					Index: 0,
				},
				SignatureScript: []uint8{btcscript.OP_NOP},
				Sequence:        4294967295,
			},
		},
		TxOut: []*btcwire.TxOut{
			{
				Value:    1000000000,
				PkScript: []byte{},
			},
		},
		LockTime: 0,
	}
	pkScript := []byte{btcscript.OP_NOP}

	for _, test := range pcTests {
		engine, err := btcscript.NewScript(tx.TxIn[0].SignatureScript,
			pkScript, 0, tx, 0)
		if err != nil {
			t.Errorf("Failed to create script: %v", err)
		}

		// set to after all scripts
		engine.TstSetPC(test.script, test.off)

		_, err = engine.Step()
		if err == nil {
			t.Errorf("Step with invalid pc (%v) succeeds!", test)
			continue
		}

		_, err = engine.DisasmPC()
		if err == nil {
			t.Errorf("DisasmPC with invalid pc (%v) succeeds!",
				test)
		}
	}
}

// Most codepaths in CheckErrorCondition() are testd elsewhere, this tests
// the execute early test.
func TestCheckErrorCondition(t *testing.T) {
	// tx with almost empty scripts.
	tx := &btcwire.MsgTx{
		Version: 1,
		TxIn: []*btcwire.TxIn{
			{
				PreviousOutPoint: btcwire.OutPoint{
					Hash: btcwire.ShaHash([32]byte{
						0xc9, 0x97, 0xa5, 0xe5,
						0x6e, 0x10, 0x41, 0x02,
						0xfa, 0x20, 0x9c, 0x6a,
						0x85, 0x2d, 0xd9, 0x06,
						0x60, 0xa2, 0x0b, 0x2d,
						0x9c, 0x35, 0x24, 0x23,
						0xed, 0xce, 0x25, 0x85,
						0x7f, 0xcd, 0x37, 0x04,
					}),
					Index: 0,
				},
				SignatureScript: []uint8{},
				Sequence:        4294967295,
			},
		},
		TxOut: []*btcwire.TxOut{
			{
				Value:    1000000000,
				PkScript: []byte{},
			},
		},
		LockTime: 0,
	}
	pkScript := []byte{
		btcscript.OP_NOP,
		btcscript.OP_NOP,
		btcscript.OP_NOP,
		btcscript.OP_NOP,
		btcscript.OP_NOP,
		btcscript.OP_NOP,
		btcscript.OP_NOP,
		btcscript.OP_NOP,
		btcscript.OP_NOP,
		btcscript.OP_NOP,
		btcscript.OP_TRUE,
	}

	engine, err := btcscript.NewScript(tx.TxIn[0].SignatureScript, pkScript,
		0, tx, 0)
	if err != nil {
		t.Errorf("failed to create script: %v", err)
	}

	for i := 0; i < len(pkScript)-1; i++ {
		done, err := engine.Step()
		if err != nil {
			t.Errorf("failed to step %dth time: %v", i, err)
			return
		}
		if done {
			t.Errorf("finshed early on %dth time", i)
			return
		}

		err = engine.CheckErrorCondition()
		if err != btcscript.StackErrScriptUnfinished {
			t.Errorf("got unexepected error %v on %dth iteration",
				err, i)
			return
		}
	}
	done, err := engine.Step()
	if err != nil {
		t.Errorf("final step failed %v", err)
		return
	}
	if !done {
		t.Errorf("final step isn't done!")
		return
	}

	err = engine.CheckErrorCondition()
	if err != nil {
		t.Errorf("unexpected error %v on final check", err)
	}
}

type TstSigScript struct {
	name               string
	inputs             []TstInput
	hashtype           byte
	compress           bool
	scriptAtWrongIndex bool
}

type TstInput struct {
	txout              *btcwire.TxOut
	sigscriptGenerates bool
	inputValidates     bool
	indexOutOfRange    bool
}

var coinbaseOutPoint = &btcwire.OutPoint{
	Index: (1 << 32) - 1,
}

// Pregenerated private key, with associated public key and pkScripts
// for the uncompressed and compressed hash160.
var (
	privKeyD = []byte{0x6b, 0x0f, 0xd8, 0xda, 0x54, 0x22, 0xd0, 0xb7,
		0xb4, 0xfc, 0x4e, 0x55, 0xd4, 0x88, 0x42, 0xb3, 0xa1, 0x65,
		0xac, 0x70, 0x7f, 0x3d, 0xa4, 0x39, 0x5e, 0xcb, 0x3b, 0xb0,
		0xd6, 0x0e, 0x06, 0x92}
	pubkeyX = []byte{0xb2, 0x52, 0xf0, 0x49, 0x85, 0x78, 0x03, 0x03, 0xc8,
		0x7d, 0xce, 0x51, 0x7f, 0xa8, 0x69, 0x0b, 0x91, 0x95, 0xf4,
		0xf3, 0x5c, 0x26, 0x73, 0x05, 0x05, 0xa2, 0xee, 0xbc, 0x09,
		0x38, 0x34, 0x3a}
	pubkeyY = []byte{0xb7, 0xc6, 0x7d, 0xb2, 0xe1, 0xff, 0xc8, 0x43, 0x1f,
		0x63, 0x32, 0x62, 0xaa, 0x60, 0xc6, 0x83, 0x30, 0xbd, 0x24,
		0x7e, 0xef, 0xdb, 0x6f, 0x2e, 0x8d, 0x56, 0xf0, 0x3c, 0x9f,
		0x6d, 0xb6, 0xf8}
	uncompressedPkScript = []byte{0x76, 0xa9, 0x14, 0xd1, 0x7c, 0xb5,
		0xeb, 0xa4, 0x02, 0xcb, 0x68, 0xe0, 0x69, 0x56, 0xbf, 0x32,
		0x53, 0x90, 0x0e, 0x0a, 0x86, 0xc9, 0xfa, 0x88, 0xac}
	compressedPkScript = []byte{0x76, 0xa9, 0x14, 0x27, 0x4d, 0x9f, 0x7f,
		0x61, 0x7e, 0x7c, 0x7a, 0x1c, 0x1f, 0xb2, 0x75, 0x79, 0x10,
		0x43, 0x65, 0x68, 0x27, 0x9d, 0x86, 0x88, 0xac}
	shortPkScript = []byte{0x76, 0xa9, 0x14, 0xd1, 0x7c, 0xb5,
		0xeb, 0xa4, 0x02, 0xcb, 0x68, 0xe0, 0x69, 0x56, 0xbf, 0x32,
		0x53, 0x90, 0x0e, 0x0a, 0x88, 0xac}
	uncompressedAddrStr = "1L6fd93zGmtzkK6CsZFVVoCwzZV3MUtJ4F"
	compressedAddrStr   = "14apLppt9zTq6cNw8SDfiJhk9PhkZrQtYZ"
)

// Pretend output amounts.
const coinbaseVal = 2500000000
const fee = 5000000

var SigScriptTests = []TstSigScript{
	{
		name: "one input uncompressed",
		inputs: []TstInput{
			{
				txout:              btcwire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashtype:           btcscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "two inputs uncompressed",
		inputs: []TstInput{
			{
				txout:              btcwire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              btcwire.NewTxOut(coinbaseVal+fee, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashtype:           btcscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "one input compressed",
		inputs: []TstInput{
			{
				txout:              btcwire.NewTxOut(coinbaseVal, compressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashtype:           btcscript.SigHashAll,
		compress:           true,
		scriptAtWrongIndex: false,
	},
	{
		name: "two inputs compressed",
		inputs: []TstInput{
			{
				txout:              btcwire.NewTxOut(coinbaseVal, compressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              btcwire.NewTxOut(coinbaseVal+fee, compressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashtype:           btcscript.SigHashAll,
		compress:           true,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashtype SigHashNone",
		inputs: []TstInput{
			{
				txout:              btcwire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashtype:           btcscript.SigHashNone,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashtype SigHashSingle",
		inputs: []TstInput{
			{
				txout:              btcwire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashtype:           btcscript.SigHashSingle,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashtype SigHashAnyoneCanPay",
		inputs: []TstInput{
			{
				txout:              btcwire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashtype:           btcscript.SigHashAnyOneCanPay,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashtype non-standard",
		inputs: []TstInput{
			{
				txout:              btcwire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashtype:           0x04,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "invalid compression",
		inputs: []TstInput{
			{
				txout:              btcwire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     false,
				indexOutOfRange:    false,
			},
		},
		hashtype:           btcscript.SigHashAll,
		compress:           true,
		scriptAtWrongIndex: false,
	},
	{
		name: "short PkScript",
		inputs: []TstInput{
			{
				txout:              btcwire.NewTxOut(coinbaseVal, shortPkScript),
				sigscriptGenerates: false,
				indexOutOfRange:    false,
			},
		},
		hashtype:           btcscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "valid script at wrong index",
		inputs: []TstInput{
			{
				txout:              btcwire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              btcwire.NewTxOut(coinbaseVal+fee, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashtype:           btcscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: true,
	},
	{
		name: "index out of range",
		inputs: []TstInput{
			{
				txout:              btcwire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              btcwire.NewTxOut(coinbaseVal+fee, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashtype:           btcscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: true,
	},
}

// Test the sigscript generation for valid and invalid inputs, all
// hashtypes, and with and without compression.  This test creates
// sigscripts to spend fake coinbase inputs, as sigscripts cannot be
// created for the MsgTxs in txTests, since they come from the blockchain
// and we don't have the private keys.
func TestSignatureScript(t *testing.T) {
	privKey := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: btcec.S256(),
			X:     new(big.Int),
			Y:     new(big.Int),
		},
		D: new(big.Int),
	}
	privKey.D.SetBytes(privKeyD)
	privKey.PublicKey.X.SetBytes(pubkeyX)
	privKey.PublicKey.Y.SetBytes(pubkeyY)

nexttest:
	for i := range SigScriptTests {
		tx := btcwire.NewMsgTx()

		output := btcwire.NewTxOut(500, []byte{btcscript.OP_RETURN})
		tx.AddTxOut(output)

		for _ = range SigScriptTests[i].inputs {
			txin := btcwire.NewTxIn(coinbaseOutPoint, nil)
			tx.AddTxIn(txin)
		}

		var script []byte
		var err error
		for j := range tx.TxIn {
			var idx int
			if SigScriptTests[i].inputs[j].indexOutOfRange {
				t.Errorf("at test %v", SigScriptTests[i].name)
				idx = len(SigScriptTests[i].inputs)
			} else {
				idx = j
			}
			script, err = btcscript.SignatureScript(tx, idx,
				SigScriptTests[i].inputs[j].txout.PkScript,
				SigScriptTests[i].hashtype, privKey,
				SigScriptTests[i].compress)

			if (err == nil) != SigScriptTests[i].inputs[j].sigscriptGenerates {
				if err == nil {
					t.Errorf("passed test '%v' incorrectly",
						SigScriptTests[i].name)
				} else {
					t.Errorf("failed test '%v': %v",
						SigScriptTests[i].name, err)
				}
				continue nexttest
			}
			if !SigScriptTests[i].inputs[j].sigscriptGenerates {
				// done with this test
				continue nexttest
			}

			tx.TxIn[j].SignatureScript = script
		}

		// If testing using a correct sigscript but for an incorrect
		// index, use last input script for first input.  Requires > 0
		// inputs for test.
		if SigScriptTests[i].scriptAtWrongIndex {
			tx.TxIn[0].SignatureScript = script
			SigScriptTests[i].inputs[0].inputValidates = false
		}

		// Validate tx input scripts
		scriptFlags := btcscript.ScriptBip16 | btcscript.ScriptCanonicalSignatures
		for j, txin := range tx.TxIn {
			engine, err := btcscript.NewScript(txin.SignatureScript,
				SigScriptTests[i].inputs[j].txout.PkScript,
				j, tx, scriptFlags)
			if err != nil {
				t.Errorf("cannot create script vm for test %v: %v",
					SigScriptTests[i].name, err)
				continue nexttest
			}
			err = engine.Execute()
			if (err == nil) != SigScriptTests[i].inputs[j].inputValidates {
				if err == nil {
					t.Errorf("passed test '%v' validation incorrectly: %v",
						SigScriptTests[i].name, err)
				} else {
					t.Errorf("failed test '%v' validation: %v",
						SigScriptTests[i].name, err)
				}
				continue nexttest
			}
		}
	}
}

var classStringifyTests = []struct {
	name        string
	scriptclass btcscript.ScriptClass
	stringed    string
}{
	{
		name:        "nonstandardty",
		scriptclass: btcscript.NonStandardTy,
		stringed:    "nonstandard",
	},
	{
		name:        "pubkey",
		scriptclass: btcscript.PubKeyTy,
		stringed:    "pubkey",
	},
	{
		name:        "pubkeyhash",
		scriptclass: btcscript.PubKeyHashTy,
		stringed:    "pubkeyhash",
	},
	{
		name:        "scripthash",
		scriptclass: btcscript.ScriptHashTy,
		stringed:    "scripthash",
	},
	{
		name:        "multisigty",
		scriptclass: btcscript.MultiSigTy,
		stringed:    "multisig",
	},
	{
		name:        "nulldataty",
		scriptclass: btcscript.NullDataTy,
		stringed:    "nulldata",
	},
	{
		name:        "broken",
		scriptclass: btcscript.ScriptClass(255),
		stringed:    "Invalid",
	},
}

func TestStringifyClass(t *testing.T) {
	for _, test := range classStringifyTests {
		typeString := test.scriptclass.String()
		if typeString != test.stringed {
			t.Errorf("%s: got \"%s\" expected \"%s\"", test.name,
				typeString, test.stringed)
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
func (b *bogusAddress) IsForNet(net *btcnet.Params) bool {
	return true // why not?
}

// String simply returns an empty string.  It exists to satsify the
// btcutil.Address interface.
func (b *bogusAddress) String() string {
	return ""
}

func TestPayToAddrScript(t *testing.T) {
	// 1MirQ9bwyQcGVJPwKUgapu5ouK2E2Ey4gX
	p2pkhMain, err := btcutil.NewAddressPubKeyHash([]byte{
		0xe3, 0x4c, 0xce, 0x70, 0xc8, 0x63, 0x73, 0x27, 0x3e, 0xfc,
		0xc5, 0x4c, 0xe7, 0xd2, 0xa4, 0x91, 0xbb, 0x4a, 0x0e, 0x84,
	}, &btcnet.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create public key hash address: %v", err)
		return
	}

	// Taken from transaction:
	// b0539a45de13b3e0403909b8bd1a555b8cbe45fd4e3f3fda76f3a5f52835c29d
	p2shMain, _ := btcutil.NewAddressScriptHashFromHash([]byte{
		0xe8, 0xc3, 0x00, 0xc8, 0x79, 0x86, 0xef, 0xa8, 0x4c, 0x37,
		0xc0, 0x51, 0x99, 0x29, 0x01, 0x9e, 0xf8, 0x6e, 0xb5, 0xb4,
	}, &btcnet.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create script hash address: %v", err)
		return
	}

	//  mainnet p2pk 13CG6SJ3yHUXo4Cr2RY4THLLJrNFuG3gUg
	p2pkCompressedMain, err := btcutil.NewAddressPubKey([]byte{
		0x02, 0x19, 0x2d, 0x74, 0xd0, 0xcb, 0x94, 0x34, 0x4c, 0x95,
		0x69, 0xc2, 0xe7, 0x79, 0x01, 0x57, 0x3d, 0x8d, 0x79, 0x03,
		0xc3, 0xeb, 0xec, 0x3a, 0x95, 0x77, 0x24, 0x89, 0x5d, 0xca,
		0x52, 0xc6, 0xb4}, &btcnet.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create pubkey address (compressed): %v",
			err)
		return
	}
	p2pkCompressed2Main, err := btcutil.NewAddressPubKey([]byte{
		0x03, 0xb0, 0xbd, 0x63, 0x42, 0x34, 0xab, 0xbb, 0x1b, 0xa1,
		0xe9, 0x86, 0xe8, 0x84, 0x18, 0x5c, 0x61, 0xcf, 0x43, 0xe0,
		0x01, 0xf9, 0x13, 0x7f, 0x23, 0xc2, 0xc4, 0x09, 0x27, 0x3e,
		0xb1, 0x6e, 0x65}, &btcnet.MainNetParams)
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
		0xf6, 0x56, 0xb4, 0x12, 0xa3}, &btcnet.MainNetParams)
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
				btcscript.OP_DATA_33,
				0x02, 0x19, 0x2d, 0x74, 0xd0, 0xcb, 0x94, 0x34,
				0x4c, 0x95, 0x69, 0xc2, 0xe7, 0x79, 0x01, 0x57,
				0x3d, 0x8d, 0x79, 0x03, 0xc3, 0xeb, 0xec, 0x3a,
				0x95, 0x77, 0x24, 0x89, 0x5d, 0xca, 0x52, 0xc6,
				0xb4, btcscript.OP_CHECKSIG,
			},
			nil,
		},
		// pay-to-pubkey address on mainnet. compressed key (other way).
		{
			p2pkCompressed2Main,
			[]byte{
				btcscript.OP_DATA_33,
				0x03, 0xb0, 0xbd, 0x63, 0x42, 0x34, 0xab, 0xbb,
				0x1b, 0xa1, 0xe9, 0x86, 0xe8, 0x84, 0x18, 0x5c,
				0x61, 0xcf, 0x43, 0xe0, 0x01, 0xf9, 0x13, 0x7f,
				0x23, 0xc2, 0xc4, 0x09, 0x27, 0x3e, 0xb1, 0x6e,
				0x65, btcscript.OP_CHECKSIG,
			},
			nil,
		},
		// pay-to-pubkey address on mainnet. uncompressed key.
		{
			p2pkUncompressedMain,
			[]byte{
				btcscript.OP_DATA_65,
				0x04, 0x11, 0xdb, 0x93, 0xe1, 0xdc, 0xdb, 0x8a,
				0x01, 0x6b, 0x49, 0x84, 0x0f, 0x8c, 0x53, 0xbc,
				0x1e, 0xb6, 0x8a, 0x38, 0x2e, 0x97, 0xb1, 0x48,
				0x2e, 0xca, 0xd7, 0xb1, 0x48, 0xa6, 0x90, 0x9a,
				0x5c, 0xb2, 0xe0, 0xea, 0xdd, 0xfb, 0x84, 0xcc,
				0xf9, 0x74, 0x44, 0x64, 0xf8, 0x2e, 0x16, 0x0b,
				0xfa, 0x9b, 0x8b, 0x64, 0xf9, 0xd4, 0xc0, 0x3f,
				0x99, 0x9b, 0x86, 0x43, 0xf6, 0x56, 0xb4, 0x12,
				0xa3, btcscript.OP_CHECKSIG,
			},
			nil,
		},

		// Supported address types with nil pointers.
		{(*btcutil.AddressPubKeyHash)(nil), []byte{}, btcscript.ErrUnsupportedAddress},
		{(*btcutil.AddressScriptHash)(nil), []byte{}, btcscript.ErrUnsupportedAddress},
		{(*btcutil.AddressPubKey)(nil), []byte{}, btcscript.ErrUnsupportedAddress},

		// Unsupported address type.
		{&bogusAddress{}, []byte{}, btcscript.ErrUnsupportedAddress},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		pkScript, err := btcscript.PayToAddrScript(test.in)
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
	//  mainnet p2pk 13CG6SJ3yHUXo4Cr2RY4THLLJrNFuG3gUg
	p2pkCompressedMain, err := btcutil.NewAddressPubKey([]byte{
		0x02, 0x19, 0x2d, 0x74, 0xd0, 0xcb, 0x94, 0x34, 0x4c, 0x95,
		0x69, 0xc2, 0xe7, 0x79, 0x01, 0x57, 0x3d, 0x8d, 0x79, 0x03,
		0xc3, 0xeb, 0xec, 0x3a, 0x95, 0x77, 0x24, 0x89, 0x5d, 0xca,
		0x52, 0xc6, 0xb4}, &btcnet.MainNetParams)
	if err != nil {
		t.Errorf("Unable to create pubkey address (compressed): %v",
			err)
		return
	}
	p2pkCompressed2Main, err := btcutil.NewAddressPubKey([]byte{
		0x03, 0xb0, 0xbd, 0x63, 0x42, 0x34, 0xab, 0xbb, 0x1b, 0xa1,
		0xe9, 0x86, 0xe8, 0x84, 0x18, 0x5c, 0x61, 0xcf, 0x43, 0xe0,
		0x01, 0xf9, 0x13, 0x7f, 0x23, 0xc2, 0xc4, 0x09, 0x27, 0x3e,
		0xb1, 0x6e, 0x65}, &btcnet.MainNetParams)
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
		0xf6, 0x56, 0xb4, 0x12, 0xa3}, &btcnet.MainNetParams)
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
				btcscript.OP_1,
				btcscript.OP_DATA_33,
				0x02, 0x19, 0x2d, 0x74, 0xd0, 0xcb, 0x94, 0x34,
				0x4c, 0x95, 0x69, 0xc2, 0xe7, 0x79, 0x01, 0x57,
				0x3d, 0x8d, 0x79, 0x03, 0xc3, 0xeb, 0xec, 0x3a,
				0x95, 0x77, 0x24, 0x89, 0x5d, 0xca, 0x52, 0xc6,
				0xb4,
				btcscript.OP_DATA_33,
				0x03, 0xb0, 0xbd, 0x63, 0x42, 0x34, 0xab, 0xbb,
				0x1b, 0xa1, 0xe9, 0x86, 0xe8, 0x84, 0x18, 0x5c,
				0x61, 0xcf, 0x43, 0xe0, 0x01, 0xf9, 0x13, 0x7f,
				0x23, 0xc2, 0xc4, 0x09, 0x27, 0x3e, 0xb1, 0x6e,
				0x65,
				btcscript.OP_2, btcscript.OP_CHECKMULTISIG,
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
				btcscript.OP_2,
				btcscript.OP_DATA_33,
				0x02, 0x19, 0x2d, 0x74, 0xd0, 0xcb, 0x94, 0x34,
				0x4c, 0x95, 0x69, 0xc2, 0xe7, 0x79, 0x01, 0x57,
				0x3d, 0x8d, 0x79, 0x03, 0xc3, 0xeb, 0xec, 0x3a,
				0x95, 0x77, 0x24, 0x89, 0x5d, 0xca, 0x52, 0xc6,
				0xb4,
				btcscript.OP_DATA_33,
				0x03, 0xb0, 0xbd, 0x63, 0x42, 0x34, 0xab, 0xbb,
				0x1b, 0xa1, 0xe9, 0x86, 0xe8, 0x84, 0x18, 0x5c,
				0x61, 0xcf, 0x43, 0xe0, 0x01, 0xf9, 0x13, 0x7f,
				0x23, 0xc2, 0xc4, 0x09, 0x27, 0x3e, 0xb1, 0x6e,
				0x65,
				btcscript.OP_2, btcscript.OP_CHECKMULTISIG,
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
			btcscript.ErrBadNumRequired,
		},
		{
			[]*btcutil.AddressPubKey{
				p2pkUncompressedMain,
			},
			1,
			[]byte{
				btcscript.OP_1, btcscript.OP_DATA_65,
				0x04, 0x11, 0xdb, 0x93, 0xe1, 0xdc, 0xdb, 0x8a,
				0x01, 0x6b, 0x49, 0x84, 0x0f, 0x8c, 0x53, 0xbc,
				0x1e, 0xb6, 0x8a, 0x38, 0x2e, 0x97, 0xb1, 0x48,
				0x2e, 0xca, 0xd7, 0xb1, 0x48, 0xa6, 0x90, 0x9a,
				0x5c, 0xb2, 0xe0, 0xea, 0xdd, 0xfb, 0x84, 0xcc,
				0xf9, 0x74, 0x44, 0x64, 0xf8, 0x2e, 0x16, 0x0b,
				0xfa, 0x9b, 0x8b, 0x64, 0xf9, 0xd4, 0xc0, 0x3f,
				0x99, 0x9b, 0x86, 0x43, 0xf6, 0x56, 0xb4, 0x12,
				0xa3,
				btcscript.OP_1, btcscript.OP_CHECKMULTISIG,
			},
			nil,
		},
		{
			[]*btcutil.AddressPubKey{
				p2pkUncompressedMain,
			},
			2,
			[]byte{},
			btcscript.ErrBadNumRequired,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		script, err := btcscript.MultiSigScript(test.keys,
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

func signAndCheck(msg string, tx *btcwire.MsgTx, idx int, pkScript []byte,
	hashType byte, kdb btcscript.KeyDB, sdb btcscript.ScriptDB,
	previousScript []byte) error {

	sigScript, err := btcscript.SignTxOutput(
		&btcnet.TestNet3Params, tx, idx, pkScript, hashType,
		kdb, sdb, []byte{})
	if err != nil {
		return fmt.Errorf("failed to sign output %s: %v", msg, err)
	}

	return checkScripts(msg, tx, idx, sigScript, pkScript)
}

func checkScripts(msg string, tx *btcwire.MsgTx, idx int,
	sigScript, pkScript []byte) error {
	engine, err := btcscript.NewScript(sigScript, pkScript, idx, tx,
		btcscript.ScriptBip16|
			btcscript.ScriptCanonicalSignatures)
	if err != nil {
		return fmt.Errorf("failed to make script engine for %s: %v",
			msg, err)
	}

	err = engine.Execute()
	if err != nil {
		return fmt.Errorf("invalid script signature for %s: %v", msg,
			err)
	}

	return nil
}

type addressToKey struct {
	key        *ecdsa.PrivateKey
	compressed bool
}

func mkGetKey(keys map[string]addressToKey) btcscript.KeyDB {
	if keys == nil {
		return btcscript.KeyClosure(func(addr btcutil.Address) (*ecdsa.PrivateKey,
			bool, error) {
			return nil, false, errors.New("nope")
		})
	}
	return btcscript.KeyClosure(func(addr btcutil.Address) (*ecdsa.PrivateKey,
		bool, error) {
		a2k, ok := keys[addr.EncodeAddress()]
		if !ok {
			return nil, false, errors.New("nope")
		}
		return a2k.key, a2k.compressed, nil
	})
}

func mkGetScript(scripts map[string][]byte) btcscript.ScriptDB {
	if scripts == nil {
		return btcscript.ScriptClosure(func(addr btcutil.Address) (
			[]byte, error) {
			return nil, errors.New("nope")
		})
	}
	return btcscript.ScriptClosure(func(addr btcutil.Address) ([]byte,
		error) {
		script, ok := scripts[addr.EncodeAddress()]
		if !ok {
			return nil, errors.New("nope")
		}
		return script, nil
	})
}

func TestSignTxOutput(t *testing.T) {
	// make key
	// make script based on key.
	// sign with magic pixie dust.
	hashTypes := []byte{
		btcscript.SigHashOld, // no longer used but should act like all
		btcscript.SigHashAll,
		btcscript.SigHashNone,
		btcscript.SigHashSingle,
		btcscript.SigHashAll | btcscript.SigHashAnyOneCanPay,
		btcscript.SigHashNone | btcscript.SigHashAnyOneCanPay,
		btcscript.SigHashSingle | btcscript.SigHashAnyOneCanPay,
	}
	tx := &btcwire.MsgTx{
		Version: 1,
		TxIn: []*btcwire.TxIn{
			&btcwire.TxIn{
				PreviousOutPoint: btcwire.OutPoint{
					Hash:  btcwire.ShaHash{},
					Index: 0,
				},
				Sequence: 4294967295,
			},
			&btcwire.TxIn{
				PreviousOutPoint: btcwire.OutPoint{
					Hash:  btcwire.ShaHash{},
					Index: 1,
				},
				Sequence: 4294967295,
			},
			&btcwire.TxIn{
				PreviousOutPoint: btcwire.OutPoint{
					Hash:  btcwire.ShaHash{},
					Index: 2,
				},
				Sequence: 4294967295,
			},
		},
		TxOut: []*btcwire.TxOut{
			&btcwire.TxOut{
				Value: 1,
			},
			&btcwire.TxOut{
				Value: 2,
			},
			&btcwire.TxOut{
				Value: 3,
			},
		},
		LockTime: 0,
	}

	// Pay to Pubkey Hash (uncompressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)
			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(nil), []byte{}); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to Pubkey Hash (uncompressed) (merging with correct)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)
			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			sigScript, err := btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, pkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(nil), []byte{})
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, pkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(nil), sigScript)
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, sigScript, pkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Pay to Pubkey Hash (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(nil), []byte{}); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to Pubkey Hash (compressed) with duplicate merge
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			sigScript, err := btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, pkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(nil), []byte{})
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, pkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(nil), sigScript)
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, sigScript, pkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Pay to PubKey (uncompressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(nil), []byte{}); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to PubKey (uncompressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			sigScript, err := btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, pkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(nil), []byte{})
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, pkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(nil), sigScript)
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, sigScript, pkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Pay to PubKey (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(nil), []byte{}); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to PubKey (compressed) with duplicate merge
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			sigScript, err := btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, pkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(nil), []byte{})
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, pkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(nil), sigScript)
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, sigScript, pkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// As before, but with p2sh now.
	// Pay to Pubkey Hash (uncompressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)
			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
				break
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := btcscript.PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			if err := signAndCheck(msg, tx, i, scriptPkScript,
				hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{}); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to Pubkey Hash (uncompressed) with duplicate merge
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)
			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
				break
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := btcscript.PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			sigScript, err := btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, scriptPkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{})
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, scriptPkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{})
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, sigScript, scriptPkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Pay to Pubkey Hash (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := btcscript.PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			if err := signAndCheck(msg, tx, i, scriptPkScript,
				hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{}); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to Pubkey Hash (compressed) with duplicate merge
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := btcscript.PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			sigScript, err := btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, scriptPkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{})
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, scriptPkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{})
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, sigScript, scriptPkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Pay to PubKey (uncompressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := btcscript.PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			if err := signAndCheck(msg, tx, i, scriptPkScript,
				hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{}); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to PubKey (uncompressed) with duplicate merge
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := btcscript.PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			sigScript, err := btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, scriptPkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{})
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, scriptPkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{})
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, sigScript, scriptPkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Pay to PubKey (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := btcscript.PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			if err := signAndCheck(msg, tx, i, scriptPkScript,
				hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{}); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to PubKey (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := btcscript.PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			sigScript, err := btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, scriptPkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{})
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, scriptPkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{})
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, sigScript, scriptPkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Basic Multisig
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key1, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk1 := (*btcec.PublicKey)(&key1.PublicKey).
				SerializeCompressed()
			address1, err := btcutil.NewAddressPubKey(pk1,
				&btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			key2, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey 2 for %s: %v",
					msg, err)
				break
			}

			pk2 := (*btcec.PublicKey)(&key2.PublicKey).
				SerializeCompressed()
			address2, err := btcutil.NewAddressPubKey(pk2,
				&btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.MultiSigScript(
				[]*btcutil.AddressPubKey{address1, address2},
				2)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := btcscript.PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			if err := signAndCheck(msg, tx, i, scriptPkScript,
				hashType,
				mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
					address2.EncodeAddress(): {key2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{}); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Two part multisig, sign with one key then the other.
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key1, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk1 := (*btcec.PublicKey)(&key1.PublicKey).
				SerializeCompressed()
			address1, err := btcutil.NewAddressPubKey(pk1,
				&btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			key2, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey 2 for %s: %v",
					msg, err)
				break
			}

			pk2 := (*btcec.PublicKey)(&key2.PublicKey).
				SerializeCompressed()
			address2, err := btcutil.NewAddressPubKey(pk2,
				&btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.MultiSigScript(
				[]*btcutil.AddressPubKey{address1, address2},
				2)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := btcscript.PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			sigScript, err := btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, scriptPkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{})
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// Only 1 out of 2 signed, this *should* fail.
			if checkScripts(msg, tx, i, sigScript,
				scriptPkScript) == nil {
				t.Errorf("part signed script valid for %s", msg)
				break
			}

			// Sign with the other key and merge
			sigScript, err = btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, scriptPkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address2.EncodeAddress(): {key2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), sigScript)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, sigScript,
				scriptPkScript)
			if err != nil {
				t.Errorf("fully signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Two part multisig, sign with one key then both, check key dedup
	// correctly.
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key1, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk1 := (*btcec.PublicKey)(&key1.PublicKey).
				SerializeCompressed()
			address1, err := btcutil.NewAddressPubKey(pk1,
				&btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			key2, err := ecdsa.GenerateKey(btcec.S256(),
				rand.Reader)
			if err != nil {
				t.Errorf("failed to make privKey 2 for %s: %v",
					msg, err)
				break
			}

			pk2 := (*btcec.PublicKey)(&key2.PublicKey).
				SerializeCompressed()
			address2, err := btcutil.NewAddressPubKey(pk2,
				&btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v",
					msg, err)
				break
			}

			pkScript, err := btcscript.MultiSigScript(
				[]*btcutil.AddressPubKey{address1, address2},
				2)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &btcnet.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := btcscript.PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			sigScript, err := btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, scriptPkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), []byte{})
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// Only 1 out of 2 signed, this *should* fail.
			if checkScripts(msg, tx, i, sigScript,
				scriptPkScript) == nil {
				t.Errorf("part signed script valid for %s", msg)
				break
			}

			// Sign with the other key and merge
			sigScript, err = btcscript.SignTxOutput(
				&btcnet.TestNet3Params, tx, i, scriptPkScript,
				hashType, mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
					address2.EncodeAddress(): {key2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), sigScript)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg, err)
				break
			}

			// Now we should pass.
			err = checkScripts(msg, tx, i, sigScript,
				scriptPkScript)
			if err != nil {
				t.Errorf("fully signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}
}

func TestCalcMultiSigStats(t *testing.T) {
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
			expected: btcscript.StackErrShortScript,
		},
		{
			name: "stack underflow",
			script: []byte{
				btcscript.OP_RETURN,
				btcscript.OP_PUSHDATA1,
				0x29,
				0x04, 0x67, 0x08, 0xaf, 0xdb, 0x0f, 0xe5, 0x54,
				0x82, 0x71, 0x96, 0x7f, 0x1a, 0x67, 0x13, 0x0b,
				0x71, 0x05, 0xcd, 0x6a, 0x82, 0x8e, 0x03, 0x90,
				0x9a, 0x67, 0x96, 0x2e, 0x0e, 0xa1, 0xf6, 0x1d,
				0xeb, 0x64, 0x9f, 0x6b, 0xc3, 0xf4, 0xce, 0xf3,
				0x08,
			},
			expected: btcscript.StackErrUnderflow,
		},
		{
			name: "multisig script",
			script: []uint8{
				btcscript.OP_FALSE,
				btcscript.OP_DATA_72,
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
				btcscript.OP_CODESEPARATOR,
				btcscript.OP_TRUE,
				btcscript.OP_DATA_33,
				0x02, 0x32, 0xab, 0xdc, 0x89,
				0x3e, 0x7f, 0x06, 0x31, 0x36,
				0x4d, 0x7f, 0xd0, 0x1c, 0xb3,
				0x3d, 0x24, 0xda, 0x45, 0x32,
				0x9a, 0x00, 0x35, 0x7b, 0x3a,
				0x78, 0x86, 0x21, 0x1a, 0xb4,
				0x14, 0xd5, 0x5a,
				btcscript.OP_TRUE,
				btcscript.OP_CHECKMULTISIG,
			},
			expected: nil,
		},
	}

	for i, test := range tests {
		if _, _, err := btcscript.CalcMultiSigStats(test.script); err != test.expected {
			t.Errorf("CalcMultiSigStats #%d (%s) wrong result\n"+
				"got: %x\nwant: %x", i, test.name, err,
				test.expected)
		}
	}
}

func TestHasCanonicalPushes(t *testing.T) {
	tests := []struct {
		name     string
		script   []byte
		expected bool
	}{
		{
			name: "does not parse",
			script: []byte{
				0x04, 0x67, 0x08, 0xaf, 0xdb, 0x0f, 0xe5, 0x54,
				0x82, 0x71, 0x96, 0x7f, 0x1a, 0x67, 0x13, 0x0b,
				0x71, 0x05, 0xcd, 0x6a, 0x82, 0x8e, 0x03, 0x90,
				0x9a, 0x67, 0x96, 0x2e, 0x0e, 0xa1, 0xf6, 0x1d,
			},
			expected: false,
		},
		{
			name:     "non-canonical push",
			script:   []byte{btcscript.OP_PUSHDATA1, 4, 1, 2, 3, 4},
			expected: false,
		},
	}

	for i, test := range tests {
		if btcscript.HasCanonicalPushes(test.script) != test.expected {
			t.Errorf("HasCanonicalPushes #%d (%s) wrong result\n"+
				"got: %x\nwant: %x", i, test.name, true,
				test.expected)
		}
	}
}

func TestIsPushOnlyScript(t *testing.T) {
	test := struct {
		name     string
		script   []byte
		expected bool
	}{
		name: "does not parse",
		script: []byte{
			0x04, 0x67, 0x08, 0xaf, 0xdb, 0x0f, 0xe5, 0x54,
			0x82, 0x71, 0x96, 0x7f, 0x1a, 0x67, 0x13, 0x0b,
			0x71, 0x05, 0xcd, 0x6a, 0x82, 0x8e, 0x03, 0x90,
			0x9a, 0x67, 0x96, 0x2e, 0x0e, 0xa1, 0xf6, 0x1d,
		},
		expected: false,
	}

	if btcscript.IsPushOnlyScript(test.script) != test.expected {
		t.Errorf("IsPushOnlyScript (%s) wrong result\n"+
			"got: %x\nwant: %x", test.name, true,
			test.expected)
	}
}
