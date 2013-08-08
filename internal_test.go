// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

import (
	"bytes"
	"encoding/json"
	"testing"
)

/*
This test file is part of the btcjson package rather than than the
btcjson_test package so it can bridge access to the internals to properly test
cases which are either not possible or can't reliably be tested via the public
interface. The functions are only exported while the tests are being run.
*/

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
	{"getinfo", []byte(`{"error":null,"result":null,"id":"test"}`), false, true},
	{"getinfo", []byte(`{"error":null,"result":null}`), false, false},
	{"getinfo", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"getblock", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"getblock", []byte(`{"result":{"hash":"000000","confirmations":16007,"size":325648},"error":null,"id":1}`), false, true},
	{"getrawtransaction", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"getrawtransaction", []byte(`{"error":null,"id":1,"result":{"hex":"somejunk","version":1}}`), false, true},
	{"decoderawtransaction", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"decoderawtransaction", []byte(`{"error":null,"id":1,"result":{"Txid":"something"}}`), false, true},
	{"getaddressesbyaccount", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"getaddressesbyaccount", []byte(`{"error":null,"id":1,"result":["test"]}`), false, true},
	{"getmininginfo", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"getmininginfo", []byte(`{"error":null,"id":1,"result":{"generate":true}}`), false, true},
	{"getrawmempool", []byte(`{"error":null,"id":1,"result":[{"a":"b"}]}`), false, false},
	{"getrawmempool", []byte(`{"error":null,"id":1,"result":["test"]}`), false, true},
	{"validateaddress", []byte(`{"error":null,"id":1,"result":{"isvalid":false}}`), false, true},
	{"validateaddress", []byte(`{"error":null,"id":1,"result":{false}}`), false, false},
	{"signrawtransaction", []byte(`{"error":null,"id":1,"result":{"hex":"something","complete":false}}`), false, true},
	{"signrawtransaction", []byte(`{"error":null,"id":1,"result":{false}}`), false, false},
}

// TestReadResultCmd tests that readResultCmd can properly unmarshall the
// returned []byte that contains a json reply for both known and unknown
// messages.
func TestReadResultCmd(t *testing.T) {
	for i, tt := range resulttests {
		result, err := readResultCmd(tt.cmd, tt.msg)
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
				if bytes.Compare(tt.msg, msg2) != 0 {
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

// TestJsonWIthArgs tests jsonWithArgs to ensure that it can generate a json
// command as well as sending it something that cannot be marshalled to json
// (a channel) to test the failure paths.
func TestJsonWithArgs(t *testing.T) {
	cmd := "list"
	var args interface{}
	_, err := jsonWithArgs(cmd, args)
	if err != nil {
		t.Errorf("Could not make json with no args. %v", err)
	}

	channel := make(chan int)
	_, err = jsonWithArgs(cmd, channel)
	if _, ok := err.(*json.UnsupportedTypeError); !ok {
		t.Errorf("Message with channel should fail. %v", err)
	}

	var comp complex128
	_, err = jsonWithArgs(cmd, comp)
	if _, ok := err.(*json.UnsupportedTypeError); !ok {
		t.Errorf("Message with complex part should fail. %v", err)
	}

	return
}

// TestJsonRpcSend tests jsonRpcSend which actually send the rpc command.
// This currently a negative test only until we setup a fake http server to
// test the actually connection code.
func TestJsonRpcSend(t *testing.T) {
	// Just negative test right now.
	user := "something"
	password := "something"
	server := "invalid"
	var message []byte
	_, err := jsonRpcSend(user, password, server, message)
	if err == nil {
		t.Errorf("Should fail when it cannot connect.")
	}
	return
}
