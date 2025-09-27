// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"bytes"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/mock"
)

// TestCalcMinRequiredTxRelayFee tests the calcMinRequiredTxRelayFee API.
func TestCalcMinRequiredTxRelayFee(t *testing.T) {
	tests := []struct {
		name     string         // test description.
		size     int64          // Transaction size in bytes.
		relayFee btcutil.Amount // minimum relay transaction fee.
		want     int64          // Expected fee.
	}{
		{
			// Ensure combination of size and fee that are less than 1000
			// produce a non-zero fee.
			"250 bytes with relay fee of 3",
			250,
			3,
			3,
		},
		{
			"100 bytes with default minimum relay fee",
			100,
			DefaultMinRelayTxFee,
			100,
		},
		{
			"max standard tx size with default minimum relay fee",
			maxStandardTxWeight / 4,
			DefaultMinRelayTxFee,
			100000,
		},
		{
			"max standard tx size with max satoshi relay fee",
			maxStandardTxWeight / 4,
			btcutil.MaxSatoshi,
			btcutil.MaxSatoshi,
		},
		{
			"1500 bytes with 5000 relay fee",
			1500,
			5000,
			7500,
		},
		{
			"1500 bytes with 3000 relay fee",
			1500,
			3000,
			4500,
		},
		{
			"782 bytes with 5000 relay fee",
			782,
			5000,
			3910,
		},
		{
			"782 bytes with 3000 relay fee",
			782,
			3000,
			2346,
		},
		{
			"782 bytes with 2550 relay fee",
			782,
			2550,
			1994,
		},
	}

	for _, test := range tests {
		got := calcMinRequiredTxRelayFee(test.size, test.relayFee)
		if got != test.want {
			t.Errorf("TestCalcMinRequiredTxRelayFee test '%s' "+
				"failed: got %v want %v", test.name, got,
				test.want)
			continue
		}
	}
}

// TestCheckPkScriptStandard tests the checkPkScriptStandard API.
func TestCheckPkScriptStandard(t *testing.T) {
	var pubKeys [][]byte
	for i := 0; i < 4; i++ {
		pk, err := btcec.NewPrivateKey()
		if err != nil {
			t.Fatalf("TestCheckPkScriptStandard NewPrivateKey failed: %v",
				err)
			return
		}
		pubKeys = append(pubKeys, pk.PubKey().SerializeCompressed())
	}

	tests := []struct {
		name       string // test description.
		script     *txscript.ScriptBuilder
		isStandard bool
	}{
		{
			"key1 and key2",
			txscript.NewScriptBuilder().AddOp(txscript.OP_2).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_2).AddOp(txscript.OP_CHECKMULTISIG),
			true,
		},
		{
			"key1 or key2",
			txscript.NewScriptBuilder().AddOp(txscript.OP_1).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_2).AddOp(txscript.OP_CHECKMULTISIG),
			true,
		},
		{
			"escrow",
			txscript.NewScriptBuilder().AddOp(txscript.OP_2).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddData(pubKeys[2]).
				AddOp(txscript.OP_3).AddOp(txscript.OP_CHECKMULTISIG),
			true,
		},
		{
			"one of four",
			txscript.NewScriptBuilder().AddOp(txscript.OP_1).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddData(pubKeys[2]).AddData(pubKeys[3]).
				AddOp(txscript.OP_4).AddOp(txscript.OP_CHECKMULTISIG),
			false,
		},
		{
			"malformed1",
			txscript.NewScriptBuilder().AddOp(txscript.OP_3).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_2).AddOp(txscript.OP_CHECKMULTISIG),
			false,
		},
		{
			"malformed2",
			txscript.NewScriptBuilder().AddOp(txscript.OP_2).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_3).AddOp(txscript.OP_CHECKMULTISIG),
			false,
		},
		{
			"malformed3",
			txscript.NewScriptBuilder().AddOp(txscript.OP_0).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_2).AddOp(txscript.OP_CHECKMULTISIG),
			false,
		},
		{
			"malformed4",
			txscript.NewScriptBuilder().AddOp(txscript.OP_1).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_0).AddOp(txscript.OP_CHECKMULTISIG),
			false,
		},
		{
			"malformed5",
			txscript.NewScriptBuilder().AddOp(txscript.OP_1).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_CHECKMULTISIG),
			false,
		},
		{
			"malformed6",
			txscript.NewScriptBuilder().AddOp(txscript.OP_1).
				AddData(pubKeys[0]).AddData(pubKeys[1]),
			false,
		},
	}

	for _, test := range tests {
		script, err := test.script.Script()
		if err != nil {
			t.Fatalf("TestCheckPkScriptStandard test '%s' "+
				"failed: %v", test.name, err)
		}
		scriptClass := txscript.GetScriptClass(script)
		got := checkPkScriptStandard(script, scriptClass)
		if (test.isStandard && got != nil) ||
			(!test.isStandard && got == nil) {

			t.Fatalf("TestCheckPkScriptStandard test '%s' failed",
				test.name)
			return
		}
	}
}

// TestDust tests the IsDust API.
func TestDust(t *testing.T) {
	pkScript := []byte{0x76, 0xa9, 0x21, 0x03, 0x2f, 0x7e, 0x43,
		0x0a, 0xa4, 0xc9, 0xd1, 0x59, 0x43, 0x7e, 0x84, 0xb9,
		0x75, 0xdc, 0x76, 0xd9, 0x00, 0x3b, 0xf0, 0x92, 0x2c,
		0xf3, 0xaa, 0x45, 0x28, 0x46, 0x4b, 0xab, 0x78, 0x0d,
		0xba, 0x5e, 0x88, 0xac}

	tests := []struct {
		name     string // test description
		txOut    wire.TxOut
		relayFee btcutil.Amount // minimum relay transaction fee.
		isDust   bool
	}{
		{
			// Any value is allowed with a zero relay fee.
			"zero value with zero relay fee",
			wire.TxOut{Value: 0, PkScript: pkScript},
			0,
			false,
		},
		{
			// Zero value is dust with any relay fee"
			"zero value with very small tx fee",
			wire.TxOut{Value: 0, PkScript: pkScript},
			1,
			true,
		},
		{
			"38 byte public key script with value 584",
			wire.TxOut{Value: 584, PkScript: pkScript},
			1000,
			true,
		},
		{
			"38 byte public key script with value 585",
			wire.TxOut{Value: 585, PkScript: pkScript},
			1000,
			false,
		},
		{
			// Maximum allowed value is never dust.
			"max satoshi amount is never dust",
			wire.TxOut{Value: btcutil.MaxSatoshi, PkScript: pkScript},
			btcutil.MaxSatoshi,
			false,
		},
		{
			// Maximum int64 value causes overflow.
			"maximum int64 value",
			wire.TxOut{Value: 1<<63 - 1, PkScript: pkScript},
			1<<63 - 1,
			true,
		},
		{
			// Unspendable pkScript due to an invalid public key
			// script.
			"unspendable pkScript",
			wire.TxOut{Value: 5000, PkScript: []byte{0x01}},
			0, // no relay fee
			true,
		},
		// P2A (Pay-to-Anchor) tests
		{
			"P2A with 239 sats (dust)",
			wire.TxOut{
				Value:    239,
				PkScript: txscript.PayToAnchorScript,
			},
			1000,
			true,
		},
		{
			"P2A with 240 sats (not dust)",
			wire.TxOut{
				Value:    240,
				PkScript: txscript.PayToAnchorScript,
			},
			1000,
			false,
		},
		{
			"P2A with 241 sats (not dust)",
			wire.TxOut{
				Value:    241,
				PkScript: txscript.PayToAnchorScript,
			},
			1000,
			false,
		},
		{
			"P2A with 240 sats and zero relay fee (not dust)",
			wire.TxOut{
				Value:    240,
				PkScript: txscript.PayToAnchorScript,
			},
			0,
			false,
		},
		{
			"P2A with 239 sats and zero relay fee (dust)",
			wire.TxOut{
				Value:    239,
				PkScript: txscript.PayToAnchorScript,
			},
			0,
			true,
		},
		{
			"P2A with 240 sats and high relay fee (not dust)",
			wire.TxOut{
				Value:    240,
				PkScript: txscript.PayToAnchorScript,
			},
			100000,
			false,
		},
		{
			"P2A with 239 sats and high relay fee (dust)",
			wire.TxOut{
				Value:    239,
				PkScript: txscript.PayToAnchorScript,
			},
			100000,
			true,
		},
	}
	for _, test := range tests {
		res := IsDust(&test.txOut, test.relayFee)
		if res != test.isDust {
			t.Fatalf("Dust test '%s' failed: want %v got %v",
				test.name, test.isDust, res)
		}
	}
}

// TestCheckTransactionStandard tests the CheckTransactionStandard API.
func TestCheckTransactionStandard(t *testing.T) {
	// Create some dummy, but otherwise standard, data for transactions.
	prevOutHash, err := chainhash.NewHashFromStr("01")
	if err != nil {
		t.Fatalf("NewShaHashFromStr: unexpected error: %v", err)
	}
	dummyPrevOut := wire.OutPoint{Hash: *prevOutHash, Index: 1}
	dummySigScript := bytes.Repeat([]byte{0x00}, 65)
	dummyTxIn := wire.TxIn{
		PreviousOutPoint: dummyPrevOut,
		SignatureScript:  dummySigScript,
		Sequence:         wire.MaxTxInSequenceNum,
	}
	addrHash := [20]byte{0x01}
	addr, err := btcutil.NewAddressPubKeyHash(addrHash[:],
		&chaincfg.TestNet3Params)
	if err != nil {
		t.Fatalf("NewAddressPubKeyHash: unexpected error: %v", err)
	}
	dummyPkScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		t.Fatalf("PayToAddrScript: unexpected error: %v", err)
	}
	dummyTxOut := wire.TxOut{
		Value:    100000000, // 1 BTC
		PkScript: dummyPkScript,
	}

	tests := []struct {
		name       string
		tx         wire.MsgTx
		height     int32
		isStandard bool
		code       wire.RejectCode
	}{
		{
			name: "Typical pay-to-pubkey-hash transaction",
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{&dummyTxIn},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
		{
			name: "Version 2 transaction",
			tx: wire.MsgTx{
				Version:  2,
				TxIn:     []*wire.TxIn{&dummyTxIn},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
		{
			name: "Version 3 transaction",
			tx: wire.MsgTx{
				Version:  3,
				TxIn:     []*wire.TxIn{&dummyTxIn},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
		{
			name: "Transaction version too high",
			tx: wire.MsgTx{
				Version:  MaxStandardTxVersion + 1,
				TxIn:     []*wire.TxIn{&dummyTxIn},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectNonstandard,
		},
		{
			name: "Transaction is not finalized",
			tx: wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					SignatureScript:  dummySigScript,
					Sequence:         0,
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 300001,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectNonstandard,
		},
		{
			name: "Transaction size is too large",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value: 0,
					PkScript: bytes.Repeat([]byte{0x00},
						(maxStandardTxWeight/4)+1),
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectNonstandard,
		},
		{
			name: "Signature script size is too large",
			tx: wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					SignatureScript: bytes.Repeat([]byte{0x00},
						maxStandardSigScriptSize+1),
					Sequence: wire.MaxTxInSequenceNum,
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectNonstandard,
		},
		{
			name: "Signature script that does more than push data",
			tx: wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					SignatureScript: []byte{
						txscript.OP_CHECKSIGVERIFY},
					Sequence: wire.MaxTxInSequenceNum,
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectNonstandard,
		},
		{
			name: "Valid but non standard public key script",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    100000000,
					PkScript: []byte{txscript.OP_TRUE},
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectNonstandard,
		},
		{
			name: "More than one nulldata output",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}, {
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectNonstandard,
		},
		{
			name: "Dust output",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    0,
					PkScript: dummyPkScript,
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectDust,
		},
		{
			name: "One nulldata output with 0 amount (standard)",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
		{
			name: "P2A output with 240 sats (standard)",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    240,
					PkScript: txscript.PayToAnchorScript,
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
		{
			name: "P2A output with 239 sats (dust)",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    239,
					PkScript: txscript.PayToAnchorScript,
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectDust,
		},
		{
			name: "P2A output with 1000 sats (standard)",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    1000,
					PkScript: txscript.PayToAnchorScript,
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
		{
			name: "Multiple P2A outputs (standard)",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{
					{
						Value:    250,
						PkScript: txscript.PayToAnchorScript,
					},
					{
						Value:    300,
						PkScript: txscript.PayToAnchorScript,
					},
					{
						Value:    500,
						PkScript: txscript.PayToAnchorScript,
					},
				},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
		{
			name: "P2A mixed with regular outputs (standard)",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{
					&dummyTxOut,
					{
						Value:    250,
						PkScript: txscript.PayToAnchorScript,
					},
				},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
	}

	pastMedianTime := time.Now()
	for _, test := range tests {
		// Ensure standardness is as expected.
		err := CheckTransactionStandard(
			btcutil.NewTx(&test.tx), test.height, pastMedianTime,
			DefaultMinRelayTxFee,
			MaxStandardTxVersion,
		)
		if err == nil && test.isStandard {
			// Test passes since function returned standard for a
			// transaction which is intended to be standard.
			continue
		}
		if err == nil && !test.isStandard {
			t.Errorf("CheckTransactionStandard (%s): standard when "+
				"it should not be", test.name)
			continue
		}
		if err != nil && test.isStandard {
			t.Errorf("CheckTransactionStandard (%s): nonstandard "+
				"when it should not be: %v", test.name, err)
			continue
		}

		// Ensure error type is a TxRuleError inside of a RuleError.
		rerr, ok := err.(RuleError)
		if !ok {
			t.Errorf("CheckTransactionStandard (%s): unexpected "+
				"error type - got %T", test.name, err)
			continue
		}
		txrerr, ok := rerr.Err.(TxRuleError)
		if !ok {
			t.Errorf("CheckTransactionStandard (%s): unexpected "+
				"error type - got %T", test.name, rerr.Err)
			continue
		}

		// Ensure the reject code is the expected one.
		if txrerr.RejectCode != test.code {
			t.Errorf("CheckTransactionStandard (%s): unexpected "+
				"error code - got %v, want %v", test.name,
				txrerr.RejectCode, test.code)
			continue
		}
	}
}

// mockUtxoEntry mocks the utxoEntry interface using testify/mock.
type mockUtxoEntry struct {
	mock.Mock
}

// PkScript returns the public key script.
func (m *mockUtxoEntry) PkScript() []byte {
	args := m.Called()
	return args.Get(0).([]byte)
}

// mockUtxoView mocks the utxoView interface using testify/mock.
type mockUtxoView struct {
	mock.Mock
}

// LookupEntry returns the entry for a given outpoint.
func (m *mockUtxoView) LookupEntry(op wire.OutPoint) utxoEntry {
	args := m.Called(op)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(utxoEntry)
}

// TestP2ASpendingStandardness tests that P2A outputs require empty witness
// and empty signature script to be considered standard.
func TestP2ASpendingStandardness(t *testing.T) {
	// Create a previous transaction with a P2A output.
	prevTxHash, _ := chainhash.NewHashFromStr("0101010101010101010101010101010101010101010101010101010101010101")
	prevOut := wire.OutPoint{Hash: *prevTxHash, Index: 0}

	// Create mocked UTXO entry and view.
	mockEntry := new(mockUtxoEntry)
	mockEntry.On("PkScript").Return(txscript.PayToAnchorScript)

	mockView := new(mockUtxoView)
	mockView.On("LookupEntry", prevOut).Return(mockEntry)

	tests := []struct {
		name       string
		sigScript  []byte
		witness    wire.TxWitness
		shouldFail bool
	}{
		{
			name:       "P2A with empty witness and empty sigscript (standard)",
			sigScript:  []byte{},
			witness:    wire.TxWitness{},
			shouldFail: false,
		},
		{
			name:       "P2A with empty sigscript and non-empty witness (not standard)",
			sigScript:  []byte{},
			witness:    wire.TxWitness{[]byte{0x01}},
			shouldFail: true,
		},
		{
			name:       "P2A with non-empty sigscript (not standard)",
			sigScript:  []byte{0x01, 0x02},
			witness:    wire.TxWitness{},
			shouldFail: true,
		},
		{
			name:       "P2A with both non-empty (not standard)",
			sigScript:  []byte{0x01},
			witness:    wire.TxWitness{[]byte{0x02}},
			shouldFail: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create a transaction spending the P2A output.
			tx := wire.NewMsgTx(2)
			tx.AddTxIn(&wire.TxIn{
				PreviousOutPoint: prevOut,
				SignatureScript:  test.sigScript,
				Witness:          test.witness,
			})
			// Add a dummy output.
			dummyPkScript := []byte{
				txscript.OP_DUP,
				txscript.OP_HASH160,
				txscript.OP_DATA_20,
			}
			dummyPkScript = append(dummyPkScript, make([]byte, 20)...)
			dummyPkScript = append(dummyPkScript,
				txscript.OP_EQUALVERIFY, txscript.OP_CHECKSIG)

			tx.AddTxOut(&wire.TxOut{
				Value:    900,
				PkScript: dummyPkScript,
			})

			btcTx := btcutil.NewTx(tx)
			err := checkInputsStandardWithView(btcTx, mockView)

			if test.shouldFail {
				if err == nil {
					t.Errorf("Expected error for P2A with non-empty witness/sigscript, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for valid P2A spend: %v", err)
				}
			}

			// Verify that the mock was called.
			mockView.AssertExpectations(t)
			mockEntry.AssertExpectations(t)
		})
	}
}
