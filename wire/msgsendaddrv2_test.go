// Copyright (c) 2024 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// TestSendAddrV2 tests the MsgSendAddrV2 API against the latest protocol
// version.
func TestSendAddrV2(t *testing.T) {
	pver := ProtocolVersion
	enc := BaseEncoding

	// Ensure the command is expected value.
	wantCmd := "sendaddrv2"
	msg := NewMsgSendAddrV2()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgSendAddrV2: wrong command - got %v want %v",
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
		t.Errorf("encode of MsgSendAddrV2 failed %v err <%v>", msg,
			err)
	}

	// Older protocol versions should fail encode since message didn't
	// exist yet.
	oldPver := SendAddrV2Version - 1
	err = msg.BtcEncode(&buf, oldPver, enc)
	if err == nil {
		s := "encode of MsgSendAddrV2 passed for old protocol " +
			"version %v err <%v>"
		t.Errorf(s, msg, err)
	}

	// Test decode with latest protocol version.
	readmsg := NewMsgSendAddrV2()
	err = readmsg.BtcDecode(&buf, pver, enc)
	if err != nil {
		t.Errorf("decode of MsgSendAddrV2 failed [%v] err <%v>", buf,
			err)
	}

	// Older protocol versions should fail decode since message didn't
	// exist yet.
	err = readmsg.BtcDecode(&buf, oldPver, enc)
	if err == nil {
		s := "decode of MsgSendAddrV2 passed for old protocol " +
			"version %v err <%v>"
		t.Errorf(s, msg, err)
	}
}

// TestSendAddrV2BIP0130 tests the MsgSendAddrV2 API against the protocol
// prior to version SendAddrV2Version.
func TestSendAddrV2BIP0130(t *testing.T) {
	// Use the protocol version just prior to SendAddrV2Version changes.
	pver := SendAddrV2Version - 1
	enc := BaseEncoding

	msg := NewMsgSendAddrV2()

	// Test encode with old protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver, enc)
	if err == nil {
		t.Errorf("encode of MsgSendAddrV2 succeeded when it should " +
			"have failed")
	}

	// Test decode with old protocol version.
	readmsg := NewMsgSendAddrV2()
	err = readmsg.BtcDecode(&buf, pver, enc)
	if err == nil {
		t.Errorf("decode of MsgSendAddrV2 succeeded when it should " +
			"have failed")
	}
}

// TestSendAddrV2CrossProtocol tests the MsgSendAddrV2 API when encoding with
// the latest protocol version and decoding with SendAddrV2Version.
func TestSendAddrV2CrossProtocol(t *testing.T) {
	enc := BaseEncoding
	msg := NewMsgSendAddrV2()

	// Encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, ProtocolVersion, enc)
	if err != nil {
		t.Errorf("encode of MsgSendAddrV2 failed %v err <%v>", msg,
			err)
	}

	// Decode with old protocol version.
	readmsg := NewMsgSendAddrV2()
	err = readmsg.BtcDecode(&buf, SendAddrV2Version, enc)
	if err != nil {
		t.Errorf("decode of MsgSendAddrV2 failed [%v] err <%v>", buf,
			err)
	}
}

// TestSendAddrV2Wire tests the MsgSendAddrV2 wire encode and decode for
// various protocol versions.
func TestSendAddrV2Wire(t *testing.T) {
	msgSendAddrV2 := NewMsgSendAddrV2()
	msgSendAddrV2Encoded := []byte{}

	tests := []struct {
		in   *MsgSendAddrV2  // Message to encode
		out  *MsgSendAddrV2  // Expected decoded message
		buf  []byte          // Wire encoding
		pver uint32          // Protocol version for wire encoding
		enc  MessageEncoding // Message encoding format
	}{
		// Latest protocol version.
		{
			msgSendAddrV2,
			msgSendAddrV2,
			msgSendAddrV2Encoded,
			ProtocolVersion,
			BaseEncoding,
		},

		// Protocol version SendAddrV2Version+1
		{
			msgSendAddrV2,
			msgSendAddrV2,
			msgSendAddrV2Encoded,
			SendAddrV2Version + 1,
			BaseEncoding,
		},

		// Protocol version SendAddrV2Version
		{
			msgSendAddrV2,
			msgSendAddrV2,
			msgSendAddrV2Encoded,
			SendAddrV2Version,
			BaseEncoding,
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
		var msg MsgSendAddrV2
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
