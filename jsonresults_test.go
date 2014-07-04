// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/conformal/btcjson"
)

var resulttests = []struct {
	cmd  string
	msg  []byte
	comp bool
	pass bool
}{
	// Generate a fake message to make sure we can encode and decode it and
	// get the same thing back.
	{"getblockcount",
		[]byte(`{"result":226790,"error":{"code":1,"message":"No Error"},"id":"btcd"}`),
		true, true},
	// Generate a fake message to make sure we don't make a command from it.
	{"anycommand", []byte(`{"result":"test","id":1}`), false, false},
	{"anycommand", []byte(`{some junk}`), false, false},
	{"anycommand", []byte(`{"error":null,"result":null,"id":"test"}`), false, true},
	{"createmultisig", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"createmultisig", []byte(`{"error":null,"id":1,"result":{"address":"something","redeemScript":"else"}}`), false, true},
	{"decodescript", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"decodescript", []byte(`{"error":null,"id":1,"result":{"Asm":"something"}}`), false, true},
	{"getinfo", []byte(`{"error":null,"result":null,"id":"test"}`), false, true},
	{"getinfo", []byte(`{"error":null,"result":null}`), false, false},
	{"getinfo", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"getblock", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"getblock", []byte(`{"result":{"hash":"000000","confirmations":16007,"size":325648},"error":null,"id":1}`), false, true},
	{"getblockchaininfo", []byte(`{"result":{"chain":"hex","blocks":250000,"bestblockhash":"hash"},"error":null,"id":1}`), false, true},
	{"getblockchaininfo", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"getnetworkinfo", []byte(`{"result":{"version":100,"protocolversion":70002},"error":null,"id":1}`), false, true},
	{"getnetworkinfo", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"getrawtransaction", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"getrawtransaction", []byte(`{"error":null,"id":1,"result":{"hex":"somejunk","version":1}}`), false, true},
	{"gettransaction", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"gettransaction", []byte(`{"error":null,"id":1,"result":{"Amount":0.0}}`), false, true},
	{"decoderawtransaction", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"decoderawtransaction", []byte(`{"error":null,"id":1,"result":{"Txid":"something"}}`), false, true},
	{"getaddressesbyaccount", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"getaddressesbyaccount", []byte(`{"error":null,"id":1,"result":["test"]}`), false, true},
	{"getmininginfo", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"getmininginfo", []byte(`{"error":null,"id":1,"result":{"generate":true}}`), false, true},
	{"gettxout", []byte(`{"error":null,"id":1,"result":{"bestblock":"a","value":1.0}}`), false, true},
	{"listreceivedbyaddress", []byte(`{"error":null,"id":1,"result":[{"a"}]}`), false, false},
	{"listreceivedbyaddress", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, true},
	{"listsinceblock", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"listsinceblock", []byte(`{"error":null,"id":1,"result":{"lastblock":"something"}}`), false, true},
	{"validateaddress", []byte(`{"error":null,"id":1,"result":{"isvalid":false}}`), false, true},
	{"validateaddress", []byte(`{"error":null,"id":1,"result":{false}}`), false, false},
	{"signrawtransaction", []byte(`{"error":null,"id":1,"result":{"hex":"something","complete":false}}`), false, true},
	{"signrawtransaction", []byte(`{"error":null,"id":1,"result":{false}}`), false, false},
	{"listunspent", []byte(`{"error":null,"id":1,"result":[{"txid":"something"}]}`), false, true},
	{"listunspent", []byte(`{"error":null,"id":1,"result":[{"txid"}]}`), false, false},
}

// TestReadResultCmd tests that readResultCmd can properly unmarshall the
// returned []byte that contains a json reply for both known and unknown
// messages.
func TestReadResultCmd(t *testing.T) {
	for i, tt := range resulttests {
		result, err := btcjson.ReadResultCmd(tt.cmd, tt.msg)
		if tt.pass {
			if err != nil {
				t.Errorf("Should read result: %d %v", i, err)
			}
			// Due to the pointer for the Error and other structs,
			// we can't always guarantee byte for byte comparison.
			if tt.comp {
				msg2, err := json.Marshal(result)
				if err != nil {
					t.Errorf("Should unmarshal result: %d %v", i, err)
				}
				if !bytes.Equal(tt.msg, msg2) {
					t.Errorf("json byte arrays differ. %d %v %v", i, tt.msg, msg2)
				}
			}

		} else {
			if err == nil {
				t.Errorf("Should fail: %d, %s", i, tt.msg)
			}
		}
	}

	return
}
