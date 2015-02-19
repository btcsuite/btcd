// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

import (
	"encoding/json"
	"testing"
)

/*
This test file is part of the btcjson package rather than than the
btcjson_test package so it can bridge access to the internals to properly test
cases which are either not possible or can't reliably be tested via the public
interface. The functions are only exported while the tests are being run.
*/

// TestJsonWIthArgs tests jsonWithArgs to ensure that it can generate a json
// command as well as sending it something that cannot be marshalled to json
// (a channel) to test the failure paths.
func TestJsonWithArgs(t *testing.T) {
	cmd := "list"
	var args interface{}
	_, err := jsonWithArgs(cmd, "test", args)
	if err != nil {
		t.Errorf("Could not make json with no args. %v", err)
	}

	channel := make(chan int)
	_, err = jsonWithArgs(cmd, "test", channel)
	if _, ok := err.(*json.UnsupportedTypeError); !ok {
		t.Errorf("Message with channel should fail. %v", err)
	}

	var comp complex128
	_, err = jsonWithArgs(cmd, "test", comp)
	if _, ok := err.(*json.UnsupportedTypeError); !ok {
		t.Errorf("Message with complex part should fail. %v", err)
	}

	return
}

// TestJsonRPCSend tests jsonRPCSend which actually send the rpc command.
// This currently a negative test only until we setup a fake http server to
// test the actually connection code.
func TestJsonRPCSend(t *testing.T) {
	// Just negative test right now.
	user := "something"
	password := "something"
	server := "invalid"
	var message []byte
	_, err := jsonRPCSend(user, password, server, message, false, nil, false)
	if err == nil {
		t.Errorf("Should fail when it cannot connect.")
	}
	return
}
