// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/wire"
	"github.com/davecgh/go-spew/spew"
)

// TestSendHeaders tests the MsgSendHeaders API against the latest protocol
// version.
func TestSendHeaders(t *testing.T) {
	pver := wire.ProtocolVersion
	enc := wire.BaseEncoding

	// Ensure the command is expected value.
	wantCmd := "sendheaders"
	msg := wire.NewMsgSendHeaders()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgSendHeaders: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value.
	wantPayload := uint32(0)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver, enc)
	if err != nil {
		t.Errorf("encode of MsgSendHeaders failed %v err <%v>", msg,
			err)
	}

	// Older protocol versions should fail encode since message didn't
	// exist yet.
	oldPver := wire.SendHeadersVersion - 1
	err = msg.BtcEncode(&buf, oldPver, enc)
	if err == nil {
		s := "encode of MsgSendHeaders passed for old protocol " +
			"version %v err <%v>"
		t.Errorf(s, msg, err)
	}

	// Test decode with latest protocol version.
	readmsg := wire.NewMsgSendHeaders()
	err = readmsg.BtcDecode(&buf, pver, enc)
	if err != nil {
		t.Errorf("decode of MsgSendHeaders failed [%v] err <%v>", buf,
			err)
	}

	// Older protocol versions should fail decode since message didn't
	// exist yet.
	err = readmsg.BtcDecode(&buf, oldPver, enc)
	if err == nil {
		s := "decode of MsgSendHeaders passed for old protocol " +
			"version %v err <%v>"
		t.Errorf(s, msg, err)
	}

	return
}

// TestSendHeadersBIP0130 tests the MsgSendHeaders API against the protocol
// prior to version SendHeadersVersion.
func TestSendHeadersBIP0130(t *testing.T) {
	// Use the protocol version just prior to SendHeadersVersion changes.
	pver := wire.SendHeadersVersion - 1
	enc := wire.BaseEncoding

	msg := wire.NewMsgSendHeaders()

	// Test encode with old protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver, enc)
	if err == nil {
		t.Errorf("encode of MsgSendHeaders succeeded when it should " +
			"have failed")
	}

	// Test decode with old protocol version.
	readmsg := wire.NewMsgSendHeaders()
	err = readmsg.BtcDecode(&buf, pver, enc)
	if err == nil {
		t.Errorf("decode of MsgSendHeaders succeeded when it should " +
			"have failed")
	}

	return
}

// TestSendHeadersCrossProtocol tests the MsgSendHeaders API when encoding with
// the latest protocol version and decoding with SendHeadersVersion.
func TestSendHeadersCrossProtocol(t *testing.T) {
	enc := wire.BaseEncoding
	msg := wire.NewMsgSendHeaders()

	// Encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, wire.ProtocolVersion, enc)
	if err != nil {
		t.Errorf("encode of MsgSendHeaders failed %v err <%v>", msg,
			err)
	}

	// Decode with old protocol version.
	readmsg := wire.NewMsgSendHeaders()
	err = readmsg.BtcDecode(&buf, wire.SendHeadersVersion, enc)
	if err != nil {
		t.Errorf("decode of MsgSendHeaders failed [%v] err <%v>", buf,
			err)
	}
}

// TestSendHeadersWire tests the MsgSendHeaders wire encode and decode for
// various protocol versions.
func TestSendHeadersWire(t *testing.T) {
	msgSendHeaders := wire.NewMsgSendHeaders()
	msgSendHeadersEncoded := []byte{}

	tests := []struct {
		in   *wire.MsgSendHeaders // Message to encode
		out  *wire.MsgSendHeaders // Expected decoded message
		buf  []byte               // Wire encoding
		pver uint32               // Protocol version for wire encoding
		enc  wire.MessageEncoding // Message encoding format
	}{
		// Latest protocol version.
		{
			msgSendHeaders,
			msgSendHeaders,
			msgSendHeadersEncoded,
			wire.ProtocolVersion,
			wire.BaseEncoding,
		},

		// Protocol version SendHeadersVersion+1
		{
			msgSendHeaders,
			msgSendHeaders,
			msgSendHeadersEncoded,
			wire.SendHeadersVersion + 1,
			wire.BaseEncoding,
		},

		// Protocol version SendHeadersVersion
		{
			msgSendHeaders,
			msgSendHeaders,
			msgSendHeadersEncoded,
			wire.SendHeadersVersion,
			wire.BaseEncoding,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode the message to wire format.
		var buf bytes.Buffer
		err := test.in.BtcEncode(&buf, test.pver, test.enc)
		if err != nil {
			t.Errorf("BtcEncode #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("BtcEncode #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode the message from wire format.
		var msg wire.MsgSendHeaders
		rbuf := bytes.NewReader(test.buf)
		err = msg.BtcDecode(rbuf, test.pver, test.enc)
		if err != nil {
			t.Errorf("BtcDecode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&msg, test.out) {
			t.Errorf("BtcDecode #%d\n got: %s want: %s", i,
				spew.Sdump(msg), spew.Sdump(test.out))
			continue
		}
	}
}
