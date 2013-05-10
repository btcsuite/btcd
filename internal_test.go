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

// TestReadResultCmd tests that readResultCmd can properly unmarshall the
// returned []byte that contains a json reply for both known and unknown
// messages.
func TestReadResultCmd(t *testing.T) {
	// Generate a fake message to make sure we can encode and decode it and
	// get the same thing back.
	msg := []byte(`{"result":226790,"error":{"code":1,"message":"No Error"},"id":"btcd"}`)
	result, err := readResultCmd("getblockcount", msg)
	if err != nil {
		t.Errorf("Reading json reply to struct failed. %v", err)
	}
	msg2, err := json.Marshal(result)
	if err != nil {
		t.Errorf("Converting struct back to json bytes failed. %v", err)
	}
	if bytes.Compare(msg, msg2) != 0 {
		t.Errorf("json byte arrays differ.")
	}
	// Generate a fake message to make sure we don't make a command from it.
	msg = []byte(`{"result":"test","id":1}`)
	_, err = readResultCmd("anycommand", msg)
	if err == nil {
		t.Errorf("Incorrect json accepted.")
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
