// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/conformal/btcjson"
)

// TestMarshallAndSend tests the MarshallAndSend function to make sure it can
// create a json message to write to the io.Writerr and to make sure
// it fails properly in cases where it cannot generate json.
func TestMarshallAndSend(t *testing.T) {
	jsonError := btcjson.Error{
		Code:    -32700,
		Message: "Parse error",
	}
	// json.Marshal cannot handle channels so this is a good way to get a
	// marshal failure.
	badRes := make(chan interface{})
	rawReply := btcjson.Reply{
		Result: badRes,
		Error:  &jsonError,
		Id:     nil,
	}
	var w bytes.Buffer

	msg, err := btcjson.MarshallAndSend(rawReply, &w)
	if fmt.Sprintf("%s", err) != "json: unsupported type: chan interface {}" {
		t.Error("Should not be able to unmarshall channel")
	}

	// Use something simple so we can compare the reply.
	rawReply = btcjson.Reply{
		Result: nil,
		Error:  nil,
		Id:     nil,
	}

	msg, err = btcjson.MarshallAndSend(rawReply, &w)
	if msg != "[RPCS] reply: {<nil> <nil> <nil>}" {
		t.Error("Incorrect reply:", msg)
	}

	expBuf := "{\"result\":null,\"error\":null,\"id\":null}\n"

	if w.String() != expBuf {
		t.Error("Incorrect data in buffer:", w.String())
	}
	return
}
