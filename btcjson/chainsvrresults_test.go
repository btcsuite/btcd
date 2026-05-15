// Copyright (c) 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson_test

import (
	"bytes"
	"encoding/json"
	"net/url"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/davecgh/go-spew/spew"
)

// TestChainSvrCustomResults ensures any results that have custom marshalling
// work as intended.
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
		{
			name: "zmq notification",
			result: &btcjson.GetZmqNotificationResult{{
				Type: "pubrawblock",
				Address: func() *url.URL {
					u, err := url.Parse("tcp://127.0.0.1:1238")
					if err != nil {
						panic(err)
					}
					return u
				}(),
				HighWaterMark: 1337,
			}},
			expected: `[{"address":"tcp://127.0.0.1:1238","hwm":1337,"type":"pubrawblock"}]`,
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

// TestStringOrArrayMarshalJSON ensures StringOrArray.MarshalJSON encodes the
// underlying slice as a JSON array without recursing into itself. Prior to the
// fix, calling MarshalJSON would dispatch back through the json.Marshaler
// interface and recurse until the goroutine stack overflowed.
func TestStringOrArrayMarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    btcjson.StringOrArray
		expected []byte
	}{
		{
			name:     "nil slice marshals as null",
			input:    nil,
			expected: []byte(`null`),
		},
		{
			name:     "empty slice marshals as empty array",
			input:    btcjson.StringOrArray{},
			expected: []byte(`[]`),
		},
		{
			name:     "single element",
			input:    btcjson.StringOrArray{"test"},
			expected: []byte(`["test"]`),
		},
		{
			name:     "multiple elements",
			input:    btcjson.StringOrArray{"test", "test1"},
			expected: []byte(`["test","test1"]`),
		},
		{
			name:     "element requiring escaping",
			input:    btcjson.StringOrArray{`a "quoted" value`},
			expected: []byte(`["a \"quoted\" value"]`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Call MarshalJSON directly to exercise the method
			// that previously recursed. Using json.Marshal at the
			// top level would also work, but the direct call makes
			// the regression target explicit.
			got, err := test.input.MarshalJSON()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, test.expected) {
				t.Fatalf("unexpected marshalled data - got %s, "+
					"want %s", got, test.expected)
			}

			// Also exercise the path through json.Marshal so we
			// cover the case where StringOrArray is reached via
			// the json.Marshaler interface dispatch.
			got, err = json.Marshal(test.input)
			if err != nil {
				t.Fatalf("json.Marshal returned error: %v", err)
			}
			if !bytes.Equal(got, test.expected) {
				t.Fatalf("unexpected json.Marshal data - got "+
					"%s, want %s", got, test.expected)
			}
		})
	}
}

// TestStringOrArrayRoundTrip ensures values produced by MarshalJSON can be
// round-tripped back through UnmarshalJSON to the original slice.
func TestStringOrArrayRoundTrip(t *testing.T) {
	t.Parallel()

	inputs := []btcjson.StringOrArray{
		{},
		{"warning"},
		{"a", "b", "c"},
	}

	for _, in := range inputs {
		encoded, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal failed for %v: %v", in, err)
		}

		var out btcjson.StringOrArray
		if err := json.Unmarshal(encoded, &out); err != nil {
			t.Fatalf("unmarshal failed for %s: %v", encoded, err)
		}

		// Treat nil and empty as equivalent since json.Unmarshal of
		// an empty array yields a non-nil empty slice.
		if len(in) == 0 && len(out) == 0 {
			continue
		}
		if !reflect.DeepEqual(in, out) {
			t.Fatalf("round-trip mismatch: got %v, want %v",
				out, in)
		}
	}
}

// TestGetBlockChainInfoWarnings tests that the warnings field in
// GetBlockChainInfoResult can be unmarshalled from both the legacy single
// string form and the post-bitcoin#29845 array form.
func TestGetBlockChainInfoWarnings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		result   string
		expected btcjson.StringOrArray
	}{
		{
			name:   "blockchain info with single warning string",
			result: `{"warnings": "this is a warning"}`,
			expected: btcjson.StringOrArray{
				"this is a warning",
			},
		},
		{
			name:   "blockchain info with array of warnings",
			result: `{"warnings": ["a", "or", "b"]}`,
			expected: btcjson.StringOrArray{
				"a", "or", "b",
			},
		},
		{
			name:     "blockchain info with empty warnings array",
			result:   `{"warnings": []}`,
			expected: btcjson.StringOrArray{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var info btcjson.GetBlockChainInfoResult
			if err := json.Unmarshal([]byte(test.result), &info); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(info.Warnings, test.expected) {
				t.Fatalf("unexpected warnings - got %+v, "+
					"want %+v", info.Warnings, test.expected)
			}
		})
	}
}

// TestGetNetworkInfoWarnings tests that we can use both a single string value
// and an array of string values for the warnings field in GetNetworkInfoResult.
func TestGetNetworkInfoWarnings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		result   string
		expected btcjson.GetNetworkInfoResult
	}{
		{
			name:   "network info with single warning",
			result: `{"warnings": "this is a warning"}`,
			expected: btcjson.GetNetworkInfoResult{
				Warnings: btcjson.StringOrArray{
					"this is a warning",
				},
			},
		},
		{
			name:   "network info with array of warnings",
			result: `{"warnings": ["a", "or", "b"]}`,
			expected: btcjson.GetNetworkInfoResult{
				Warnings: btcjson.StringOrArray{
					"a", "or", "b",
				},
			},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		var infoResult btcjson.GetNetworkInfoResult
		err := json.Unmarshal([]byte(test.result), &infoResult)
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}
		if !reflect.DeepEqual(infoResult, test.expected) {
			t.Errorf("Test #%d (%s) unexpected marhsalled data - "+
				"got %+v, want %+v", i, test.name, infoResult,
				test.expected)
			continue
		}
	}
}
