// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// TestCalcMinRequiredTxRelayFee tests the calcMinRequiredTxRelayFee API.
func TestCalcMinRequiredTxRelayFee(t *testing.T) {
	tests := []struct {
		size     int64          // Transaction size in bytes.
		relayFee btcutil.Amount // minimum relay transaction fee.
		want     int64          // Expected fee.
	}{
		{
			0,
			defaultMinRelayTxFee,
			int64(defaultMinRelayTxFee),
		},
		{
			100,
			defaultMinRelayTxFee,
			int64(defaultMinRelayTxFee),
		},
		{
			maxStandardTxSize,
			defaultMinRelayTxFee,
			101000,
		},
		{
			maxStandardTxSize,
			btcutil.MaxSatoshi,
			btcutil.MaxSatoshi,
		},
	}

	for x, test := range tests {
		got := calcMinRequiredTxRelayFee(test.size, test.relayFee)
		if got != test.want {
			t.Errorf("TestCalcMinRequiredTxRelayFee test #%d "+
				"failed: got %v want %v", x, got, test.want)
			continue
		}
	}
}

// TestDust tests the isDust API.
func TestDust(t *testing.T) {
	pkScript := []byte{0x76, 0xa9, 0x21, 0x03, 0x2f, 0x7e, 0x43,
		0x0a, 0xa4, 0xc9, 0xd1, 0x59, 0x43, 0x7e, 0x84, 0xb9,
		0x75, 0xdc, 0x76, 0xd9, 0x00, 0x3b, 0xf0, 0x92, 0x2c,
		0xf3, 0xaa, 0x45, 0x28, 0x46, 0x4b, 0xab, 0x78, 0x0d,
		0xba, 0x5e, 0x88, 0xac}

	tests := []struct {
		txOut  wire.TxOut
		isDust bool
	}{
		{
			// Zero value is dust.
			wire.TxOut{0, pkScript},
			true,
		},
		{
			wire.TxOut{584, pkScript},
			true,
		},
		{
			wire.TxOut{585, pkScript},
			false,
		},
		{
			// Maximum allowed value,
			wire.TxOut{btcutil.MaxSatoshi, pkScript},
			false,
		},
		{
			// Maximum int64 value causes overflow.
			wire.TxOut{1<<63 - 1, pkScript},
			true,
		},
		{
			// Unspendable pkScript due to an invalid public key
			// script.
			wire.TxOut{5000, []byte{0x01}},
			true,
		},
	}
	for x, test := range tests {
		res := isDust(&test.txOut, defaultMinRelayTxFee)
		if res != test.isDust {
			t.Fatalf("Dust test %d with amount %d failed: "+
				"want %v got %v", x, test.txOut.Value,
				test.isDust, res)
			continue
		}
	}
}
