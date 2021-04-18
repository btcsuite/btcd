// Copyright (c) 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcutil"
	"github.com/davecgh/go-spew/spew"
)

// TestChainSvrCustomResults ensures any results that have custom marshalling
// work as inteded.
// and unmarshal code of results are as expected.
func TestChainSvrCustomResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		result   interface{}
		expected string
	}{
		{
			name: "custom vin marshal with coinbase",
			result: &btcjson.Vin{
				Coinbase: "021234",
				Sequence: 4294967295,
			},
			expected: `{"coinbase":"021234","sequence":4294967295}`,
		},
		{
			name: "custom vin marshal without coinbase",
			result: &btcjson.Vin{
				Txid: "123",
				Vout: 1,
				ScriptSig: &btcjson.ScriptSig{
					Asm: "0",
					Hex: "00",
				},
				Sequence: 4294967295,
			},
			expected: `{"txid":"123","vout":1,"scriptSig":{"asm":"0","hex":"00"},"sequence":4294967295}`,
		},
		{
			name: "custom vinprevout marshal with coinbase",
			result: &btcjson.VinPrevOut{
				Coinbase: "021234",
				Sequence: 4294967295,
			},
			expected: `{"coinbase":"021234","sequence":4294967295}`,
		},
		{
			name: "custom vinprevout marshal without coinbase",
			result: &btcjson.VinPrevOut{
				Txid: "123",
				Vout: 1,
				ScriptSig: &btcjson.ScriptSig{
					Asm: "0",
					Hex: "00",
				},
				PrevOut: &btcjson.PrevOut{
					Addresses: []string{"addr1"},
					Value:     0,
				},
				Sequence: 4294967295,
			},
			expected: `{"txid":"123","vout":1,"scriptSig":{"asm":"0","hex":"00"},"prevOut":{"addresses":["addr1"],"value":0},"sequence":4294967295}`,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		marshalled, err := json.Marshal(test.result)
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}
		if string(marshalled) != test.expected {
			t.Errorf("Test #%d (%s) unexpected marhsalled data - "+
				"got %s, want %s", i, test.name, marshalled,
				test.expected)
			continue
		}
	}
}

// TestGetTxOutSetInfoResult ensures that custom unmarshalling of
// GetTxOutSetInfoResult works as intended.
func TestGetTxOutSetInfoResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result string
		want   btcjson.GetTxOutSetInfoResult
	}{
		{
			name:   "GetTxOutSetInfoResult - not scanning",
			result: `{"height":123,"bestblock":"000000000000005f94116250e2407310463c0a7cf950f1af9ebe935b1c0687ab","transactions":1,"txouts":1,"bogosize":1,"hash_serialized_2":"9a0a561203ff052182993bc5d0cb2c620880bfafdbd80331f65fd9546c3e5c3e","disk_size":1,"total_amount":0.2}`,
			want: btcjson.GetTxOutSetInfoResult{
				Height: 123,
				BestBlock: func() chainhash.Hash {
					h, err := chainhash.NewHashFromStr("000000000000005f94116250e2407310463c0a7cf950f1af9ebe935b1c0687ab")
					if err != nil {
						panic(err)
					}

					return *h
				}(),
				Transactions: 1,
				TxOuts:       1,
				BogoSize:     1,
				HashSerialized: func() chainhash.Hash {
					h, err := chainhash.NewHashFromStr("9a0a561203ff052182993bc5d0cb2c620880bfafdbd80331f65fd9546c3e5c3e")
					if err != nil {
						panic(err)
					}

					return *h
				}(),
				DiskSize: 1,
				TotalAmount: func() btcutil.Amount {
					a, err := btcutil.NewAmount(0.2)
					if err != nil {
						panic(err)
					}

					return a
				}(),
			},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		var out btcjson.GetTxOutSetInfoResult
		err := json.Unmarshal([]byte(test.result), &out)
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !reflect.DeepEqual(out, test.want) {
			t.Errorf("Test #%d (%s) unexpected unmarshalled data - "+
				"got %v, want %v", i, test.name, spew.Sdump(out),
				spew.Sdump(test.want))
			continue
		}
	}
}

// TestChainSvrMiningInfoResults ensures GetMiningInfoResults are unmarshalled correctly
func TestChainSvrMiningInfoResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		result   string
		expected btcjson.GetMiningInfoResult
	}{
		{
			name:   "mining info with integer networkhashps",
			result: `{"networkhashps": 89790618491361}`,
			expected: btcjson.GetMiningInfoResult{
				NetworkHashPS: 89790618491361,
			},
		},
		{
			name:   "mining info with scientific notation networkhashps",
			result: `{"networkhashps": 8.9790618491361e+13}`,
			expected: btcjson.GetMiningInfoResult{
				NetworkHashPS: 89790618491361,
			},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		var miningInfoResult btcjson.GetMiningInfoResult
		err := json.Unmarshal([]byte(test.result), &miningInfoResult)
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}
		if miningInfoResult != test.expected {
			t.Errorf("Test #%d (%s) unexpected marhsalled data - "+
				"got %+v, want %+v", i, test.name, miningInfoResult,
				test.expected)
			continue
		}
	}
}
