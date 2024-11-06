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

// TestWTxIdRelay tests the MsgWTxIdRelay API against the latest protocol
// version.
func TestWTxIdRelay(t *testing.T) {
	pver := ProtocolVersion
	enc := BaseEncoding

	// Ensure the command is expected value.
	wantCmd := "wtxidrelay"
	msg := NewMsgWTxIdRelay()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgWTxIdRelay: wrong command - got %v want %v",
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
		t.Errorf("encode of MsgWTxIdRelay failed %v err <%v>", msg,
			err)
	}

	// Older protocol versions should fail encode since message didn't
	// exist yet.
	oldPver := WTxIdRelayVersion - 1
	err = msg.BtcEncode(&buf, oldPver, enc)
	if err == nil {
		s := "encode of MsgWTxIdRelay passed for old protocol " +
			"version %v err <%v>"
		t.Errorf(s, msg, err)
	}

	// Test decode with latest protocol version.
	readmsg := NewMsgWTxIdRelay()
	err = readmsg.BtcDecode(&buf, pver, enc)
	if err != nil {
		t.Errorf("decode of MsgWTxIdRelay failed [%v] err <%v>", buf,
			err)
	}

	// Older protocol versions should fail decode since message didn't
	// exist yet.
	err = readmsg.BtcDecode(&buf, oldPver, enc)
	if err == nil {
		s := "decode of MsgWTxIdRelay passed for old protocol " +
			"version %v err <%v>"
		t.Errorf(s, msg, err)
	}
}

// TestWTxIdRelayBIP0130 tests the MsgWTxIdRelay API against the protocol
// prior to version WTxIdRelayVersion.
func TestWTxIdRelayBIP0130(t *testing.T) {
	// Use the protocol version just prior to WTxIdRelayVersion changes.
	pver := WTxIdRelayVersion - 1
	enc := BaseEncoding

	msg := NewMsgWTxIdRelay()

	// Test encode with old protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver, enc)
	if err == nil {
		t.Errorf("encode of MsgWTxIdRelay succeeded when it should " +
			"have failed")
	}

	// Test decode with old protocol version.
	readmsg := NewMsgWTxIdRelay()
	err = readmsg.BtcDecode(&buf, pver, enc)
	if err == nil {
		t.Errorf("decode of MsgWTxIdRelay succeeded when it should " +
			"have failed")
	}
}

// TestWTxIdRelayCrossProtocol tests the MsgWTxIdRelay API when encoding with
// the latest protocol version and decoding with WTxIdRelayVersion.
func TestWTxIdRelayCrossProtocol(t *testing.T) {
	enc := BaseEncoding
	msg := NewMsgWTxIdRelay()

	// Encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, ProtocolVersion, enc)
	if err != nil {
		t.Errorf("encode of MsgWTxIdRelay failed %v err <%v>", msg,
			err)
	}

	// Decode with old protocol version.
	readmsg := NewMsgWTxIdRelay()
	err = readmsg.BtcDecode(&buf, WTxIdRelayVersion, enc)
	if err != nil {
		t.Errorf("decode of MsgWTxIdRelay failed [%v] err <%v>", buf,
			err)
	}
}

// TestWTxIdRelayWire tests the MsgWTxIdRelay wire encode and decode for
// various protocol versions.
func TestWTxIdRelayWire(t *testing.T) {
	msgWTxIdRelay := NewMsgWTxIdRelay()
	msgWTxIdRelayEncoded := []byte{}

	tests := []struct {
		in   *MsgWTxIdRelay  // Message to encode
		out  *MsgWTxIdRelay  // Expected decoded message
		buf  []byte          // Wire encoding
		pver uint32          // Protocol version for wire encoding
		enc  MessageEncoding // Message encoding format
	}{
		// Latest protocol version.
		{
			msgWTxIdRelay,
			msgWTxIdRelay,
			msgWTxIdRelayEncoded,
			ProtocolVersion,
			BaseEncoding,
		},

		// Protocol version WTxIdRelayVersion+1
		{
			msgWTxIdRelay,
			msgWTxIdRelay,
			msgWTxIdRelayEncoded,
			WTxIdRelayVersion + 1,
			BaseEncoding,
		},

		// Protocol version WTxIdRelayVersion
		{
			msgWTxIdRelay,
			msgWTxIdRelay,
			msgWTxIdRelayEncoded,
			WTxIdRelayVersion,
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
		var msg MsgWTxIdRelay
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
