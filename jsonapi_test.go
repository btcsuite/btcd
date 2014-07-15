// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	"github.com/conformal/btcjson"
)

// cmdtests is a table of all the possible commands and a list of inputs,
// some of which should work, some of which should not (indicated by the
// pass variable).  This mainly checks the type and number of the arguments,
// it does not actually check to make sure the values are correct (i.e., that
// addresses are reasonable) as the bitcoin client must be able to deal with
// that.
var cmdtests = []struct {
	cmd  string
	args []interface{}
	pass bool
}{
	{"createmultisig", nil, false},
	{"createmultisig", []interface{}{1}, false},
	{"getinfo", nil, true},
	{"getinfo", []interface{}{1}, false},
	{"listaccounts", nil, true},
	{"listaccounts", []interface{}{1}, true},
	{"listaccounts", []interface{}{"test"}, false},
	{"listaccounts", []interface{}{1, 2}, false},
	{"estimatefee", nil, false},
	{"estimatefee", []interface{}{1}, true},
	{"estimatefee", []interface{}{1, 2}, false},
	{"estimatefee", []interface{}{1.3}, false},
	{"estimatepriority", nil, false},
	{"estimatepriority", []interface{}{1}, true},
	{"estimatepriority", []interface{}{1, 2}, false},
	{"estimatepriority", []interface{}{1.3}, false},
	{"getblockhash", nil, false},
	{"getblockhash", []interface{}{1}, true},
	{"getblockhash", []interface{}{1, 2}, false},
	{"getblockhash", []interface{}{1.1}, false},
	{"settxfee", nil, false},
	{"settxfee", []interface{}{1.0}, true},
	{"settxfee", []interface{}{1.0, 2.0}, false},
	{"settxfee", []interface{}{1}, false},
	{"getmemorypool", nil, true},
	{"getmemorypool", []interface{}{"test"}, true},
	{"getmemorypool", []interface{}{1}, false},
	{"getmemorypool", []interface{}{"test", 2}, false},
	{"backupwallet", nil, false},
	{"backupwallet", []interface{}{1, 2}, false},
	{"backupwallet", []interface{}{1}, false},
	{"backupwallet", []interface{}{"testpath"}, true},
	{"setaccount", nil, false},
	{"setaccount", []interface{}{1}, false},
	{"setaccount", []interface{}{1, 2, 3}, false},
	{"setaccount", []interface{}{1, "test"}, false},
	{"setaccount", []interface{}{"test", "test"}, true},
	{"verifymessage", nil, false},
	{"verifymessage", []interface{}{1}, false},
	{"verifymessage", []interface{}{1, 2}, false},
	{"verifymessage", []interface{}{1, 2, 3, 4}, false},
	{"verifymessage", []interface{}{"test", "test", "test"}, true},
	{"verifymessage", []interface{}{"test", "test", 1}, false},
	{"getaddednodeinfo", nil, false},
	{"getaddednodeinfo", []interface{}{1}, false},
	{"getaddednodeinfo", []interface{}{true}, true},
	{"getaddednodeinfo", []interface{}{true, 1}, false},
	{"getaddednodeinfo", []interface{}{true, "test"}, true},
	{"setgenerate", nil, false},
	{"setgenerate", []interface{}{1, 2, 3}, false},
	{"setgenerate", []interface{}{true}, true},
	{"setgenerate", []interface{}{true, 1}, true},
	{"setgenerate", []interface{}{true, 1.1}, false},
	{"setgenerate", []interface{}{"true", 1}, false},
	{"getbalance", nil, true},
	{"getbalance", []interface{}{"test"}, true},
	{"getbalance", []interface{}{"test", 1}, true},
	{"getbalance", []interface{}{"test", 1.0}, false},
	{"getbalance", []interface{}{1, 1}, false},
	{"getbalance", []interface{}{"test", 1, 2}, false},
	{"getbalance", []interface{}{1}, false},
	{"addnode", nil, false},
	{"addnode", []interface{}{1, 2, 3}, false},
	{"addnode", []interface{}{"test", "test"}, true},
	{"addnode", []interface{}{1}, false},
	{"addnode", []interface{}{"test", 1}, false},
	{"addnode", []interface{}{"test", 1.0}, false},
	{"listreceivedbyaccount", nil, true},
	{"listreceivedbyaccount", []interface{}{1, 2, 3}, false},
	{"listreceivedbyaccount", []interface{}{1}, true},
	{"listreceivedbyaccount", []interface{}{1.0}, false},
	{"listreceivedbyaccount", []interface{}{1, false}, true},
	{"listreceivedbyaccount", []interface{}{1, "false"}, false},
	{"listtransactions", nil, true},
	{"listtransactions", []interface{}{"test"}, true},
	{"listtransactions", []interface{}{"test", 1}, true},
	{"listtransactions", []interface{}{"test", 1, 2}, true},
	{"listtransactions", []interface{}{"test", 1, 2, 3}, false},
	{"listtransactions", []interface{}{1}, false},
	{"listtransactions", []interface{}{"test", 1.0}, false},
	{"listtransactions", []interface{}{"test", 1, "test"}, false},
	{"importprivkey", nil, false},
	{"importprivkey", []interface{}{"test"}, true},
	{"importprivkey", []interface{}{1}, false},
	{"importprivkey", []interface{}{"test", "test"}, true},
	{"importprivkey", []interface{}{"test", "test", true}, true},
	{"importprivkey", []interface{}{"test", "test", true, 1}, false},
	{"importprivkey", []interface{}{"test", 1.0, true}, false},
	{"importprivkey", []interface{}{"test", "test", "true"}, false},
	{"listunspent", nil, true},
	{"listunspent", []interface{}{1}, true},
	{"listunspent", []interface{}{1, 2}, true},
	{"listunspent", []interface{}{1, 2, 3}, false},
	{"listunspent", []interface{}{1.0}, false},
	{"listunspent", []interface{}{1, 2.0}, false},
	{"sendfrom", nil, false},
	{"sendfrom", []interface{}{"test"}, false},
	{"sendfrom", []interface{}{"test", "test"}, false},
	{"sendfrom", []interface{}{"test", "test", 1.0}, true},
	{"sendfrom", []interface{}{"test", 1, 1.0}, false},
	{"sendfrom", []interface{}{1, "test", 1.0}, false},
	{"sendfrom", []interface{}{"test", "test", 1}, false},
	{"sendfrom", []interface{}{"test", "test", 1.0, 1}, true},
	{"sendfrom", []interface{}{"test", "test", 1.0, 1, "test"}, true},
	{"sendfrom", []interface{}{"test", "test", 1.0, 1, "test", "test"}, true},
	{"move", nil, false},
	{"move", []interface{}{1, 2, 3, 4, 5, 6}, false},
	{"move", []interface{}{1, 2}, false},
	{"move", []interface{}{"test", "test", 1.0}, true},
	{"move", []interface{}{"test", "test", 1.0, 1, "test"}, true},
	{"move", []interface{}{"test", "test", 1.0, 1}, true},
	{"move", []interface{}{1, "test", 1.0}, false},
	{"move", []interface{}{"test", 1, 1.0}, false},
	{"move", []interface{}{"test", "test", 1}, false},
	{"move", []interface{}{"test", "test", 1.0, 1.0, "test"}, false},
	{"move", []interface{}{"test", "test", 1.0, 1, true}, false},
	{"sendtoaddress", nil, false},
	{"sendtoaddress", []interface{}{"test"}, false},
	{"sendtoaddress", []interface{}{"test", 1.0}, true},
	{"sendtoaddress", []interface{}{"test", 1.0, "test"}, true},
	{"sendtoaddress", []interface{}{"test", 1.0, "test", "test"}, true},
	{"sendtoaddress", []interface{}{1, 1.0, "test", "test"}, false},
	{"sendtoaddress", []interface{}{"test", 1, "test", "test"}, false},
	{"sendtoaddress", []interface{}{"test", 1.0, 1.0, "test"}, false},
	{"sendtoaddress", []interface{}{"test", 1.0, "test", 1.0}, false},
	{"sendtoaddress", []interface{}{"test", 1.0, "test", "test", 1}, false},
	{"addmultisignaddress", []interface{}{1, "test", "test"}, true},
	{"addmultisignaddress", []interface{}{1, "test"}, false},
	{"addmultisignaddress", []interface{}{1, 1.0, "test"}, false},
	{"addmultisignaddress", []interface{}{1, "test", "test", "test"}, true},
	{"createrawtransaction", []interface{}{"in1", uint32(0), "a1", 1.0}, true},
	{"createrawtransaction", []interface{}{"in1", "out1", "a1", 1.0, "test"}, false},
	{"createrawtransaction", []interface{}{}, false},
	{"createrawtransaction", []interface{}{"in1", 1.0, "a1", 1.0}, false},
	{"sendmany", []interface{}{"in1", "out1", 1.0, 1, "comment"}, true},
	{"sendmany", []interface{}{"in1", "out1", 1.0, "comment"}, true},
	{"sendmany", []interface{}{"in1", "out1"}, false},
	{"sendmany", []interface{}{true, "out1", 1.0, 1, "comment"}, false},
	{"sendmany", []interface{}{"in1", "out1", "test", 1, "comment"}, false},
	{"lockunspent", []interface{}{true, "something"}, true},
	{"lockunspent", []interface{}{true}, false},
	{"lockunspent", []interface{}{1.0, "something"}, false},
	{"signrawtransaction", []interface{}{"hexstring", "test", uint32(1), "test"}, true},
	{"signrawtransaction", []interface{}{"hexstring", "test", "test2", "test3", "test4"}, false},
	{"signrawtransaction", []interface{}{"hexstring", "test", "test2", "test3"}, false},
	{"signrawtransaction", []interface{}{1.2, "test", "test2", "test3", "test4"}, false},
	{"signrawtransaction", []interface{}{"hexstring", 1, "test2", "test3", "test4"}, false},
	{"signrawtransaction", []interface{}{"hexstring", "test", "test2", 3, "test4"}, false},
	{"listsinceblock", []interface{}{"test", "test"}, true},
	{"listsinceblock", []interface{}{"test", "test", "test"}, false},
	{"listsinceblock", []interface{}{"test"}, true},
	{"listsinceblock", []interface{}{}, true},
	{"listsinceblock", []interface{}{1, "test"}, false},
	{"walletpassphrase", []interface{}{"test", 1}, true},
	{"walletpassphrase", []interface{}{"test"}, false},
	{"walletpassphrase", []interface{}{"test", "test"}, false},
	{"getrawchangeaddress", []interface{}{}, true},
	{"getrawchangeaddress", []interface{}{"something"}, true},
	{"getrawchangeaddress", []interface{}{"something", "test"}, false},
	{"getbestblockhash", []interface{}{}, true},
	{"getbestblockhash", []interface{}{"something"}, false},
	{"getblockchaininfo", []interface{}{}, true},
	{"getblockchaininfo", []interface{}{"something"}, false},
	{"getnetworkinfo", []interface{}{}, true},
	{"getnetworkinfo", []interface{}{"something"}, false},
	{"submitblock", []interface{}{}, false},
	{"submitblock", []interface{}{"something"}, true},
	{"submitblock", []interface{}{"something", "something else"}, true},
	{"submitblock", []interface{}{"something", "something else", "more"}, false},
	{"submitblock", []interface{}{"something", 1}, false},
	{"fakecommand", nil, false},
}

// TestRpcCreateMessage tests CreateMessage using the table of messages
// in cmdtests.
func TestRpcCreateMessage(t *testing.T) {
	var err error
	for i, tt := range cmdtests {
		if tt.args == nil {
			_, err = btcjson.CreateMessage(tt.cmd)
		} else {
			_, err = btcjson.CreateMessage(tt.cmd, tt.args...)
		}
		if tt.pass {
			if err != nil {
				t.Errorf("Could not create command %d: %s %v.", i, tt.cmd, err)
			}
		} else {
			if err == nil {
				t.Errorf("Should create command. %d: %s", i, tt.cmd)
			}
		}
	}
	return
}

// TestRpcCommand tests RpcCommand by generating some commands and
// trying to send them off.
func TestRpcCommand(t *testing.T) {
	user := "something"
	pass := "something"
	server := "invalid"
	var msg []byte
	_, err := btcjson.RpcCommand(user, pass, server, msg)
	if err == nil {
		t.Errorf("Should fail.")
	}
	msg, err = btcjson.CreateMessage("getinfo")
	if err != nil {
		t.Errorf("Cannot create valid json message")
	}
	_, err = btcjson.RpcCommand(user, pass, server, msg)
	if err == nil {
		t.Errorf("Should not connect to server.")
	}

	badMsg := []byte("{\"jsonrpc\":\"1.0\",\"id\":\"btcd\",\"method\":\"\"}")
	_, err = btcjson.RpcCommand(user, pass, server, badMsg)
	if err == nil {
		t.Errorf("Cannot have no method in msg..")
	}
	return
}

// FailingReadClose is a type used for testing so we can get something that
// fails past Go's type system.
type FailingReadCloser struct{}

func (f *FailingReadCloser) Close() error {
	return io.ErrUnexpectedEOF
}

func (f *FailingReadCloser) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

// TestRpcReply tests JsonGetRaw by sending both a good and a bad buffer
// to it.
func TestRpcReply(t *testing.T) {
	buffer := new(bytes.Buffer)
	buffer2 := ioutil.NopCloser(buffer)
	_, err := btcjson.GetRaw(buffer2)
	if err != nil {
		t.Errorf("Error reading rpc reply.")
	}
	failBuf := &FailingReadCloser{}
	_, err = btcjson.GetRaw(failBuf)
	if err == nil {
		t.Errorf("Error, this should fail.")
	}
	return
}

var idtests = []struct {
	testID []interface{}
	pass   bool
}{
	{[]interface{}{"string test"}, true},
	{[]interface{}{1}, true},
	{[]interface{}{1.0}, true},
	{[]interface{}{nil}, true},
	{[]interface{}{make(chan int)}, false},
}

// TestIsValidIdType tests that IsValidIdType allows (and disallows the correct
// types).
func TestIsValidIdType(t *testing.T) {
	for _, tt := range idtests {
		res := btcjson.IsValidIdType(tt.testID[0])
		if res != tt.pass {
			t.Errorf("Incorrect type result %v.", tt)
		}
	}
	return
}

var floattests = []struct {
	in   float64
	out  int64
	pass bool
}{
	{1.0, 100000000, true},
	{-1.0, -100000000, true},
	{0.0, 0, true},
	{0.00000001, 1, true},
	{-0.00000001, -1, true},
	{-1.0e307, 0, false},
	{1.0e307, 0, false},
}

// TestJSONtoAmount tests that JSONtoAmount returns the proper values.
func TestJSONtoAmount(t *testing.T) {
	for _, tt := range floattests {
		res, err := btcjson.JSONToAmount(tt.in)
		if tt.pass {
			if res != tt.out || err != nil {
				t.Errorf("Should not fail: %v", tt.in)
			}
		} else {
			if err == nil {
				t.Errorf("Should not pass: %v", tt.in)
			}
		}
	}
	return
}

// TestErrorInterface tests that the Error type satisifies the builtin
// error interface and tests that the error string is created in the form
// "code: message".
func TestErrorInterface(t *testing.T) {
	codes := []int{
		-1,
		0,
		1,
	}
	messages := []string{
		"parse error",
		"error getting field",
		"method not found",
	}

	// Create an Error and check that both Error and *Error can be used
	// as an error.
	var jsonError btcjson.Error
	var iface interface{} = jsonError
	var ifacep interface{} = &jsonError
	if _, ok := iface.(error); !ok {
		t.Error("cannot type assert Error as error")
		return
	}
	if _, ok := ifacep.(error); !ok {
		t.Error("cannot type assert *Error as error")
		return
	}

	// Verify jsonError is converted to the expected string using a few
	// combinations of codes and messages.
	for _, code := range codes {
		for _, message := range messages {
			// Create Error
			jsonError := btcjson.Error{
				Code:    code,
				Message: message,
			}

			exp := fmt.Sprintf("%d: %s", jsonError.Code, jsonError.Message)
			res := fmt.Sprintf("%v", jsonError)
			if exp != res {
				t.Errorf("error string '%s' differs from expected '%v'", res, exp)
			}
		}
	}
}
