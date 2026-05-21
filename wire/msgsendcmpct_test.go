// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// TestSendCmpct tests the MsgSendCmpct API against the latest protocol
// version.
func TestSendCmpct(t *testing.T) {
	pver := ProtocolVersion
	enc := BaseEncoding
	announce := uint8(1)
	version := uint64(2)

	msg := NewMsgSendCmpct(announce, version)
	if msg.Announce != announce {
		t.Errorf("NewMsgSendCmpct: wrong announce - got %v, want %v",
			msg.Announce, announce)
	}
	if msg.Version != version {
		t.Errorf("NewMsgSendCmpct: wrong version - got %v, want %v",
			msg.Version, version)
	}

	// Ensure the command is the expected value.
	wantCmd := "sendcmpct"
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgSendCmpct: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is the expected value.
	wantPayload := uint32(9)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Test encode with the latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver, enc)
	if err != nil {
		t.Errorf("encode of MsgSendCmpct failed %v err <%v>", msg, err)
	}

	// Test decode with the latest protocol version.
	readMsg := NewMsgSendCmpct(0, 0)
	err = readMsg.BtcDecode(&buf, pver, enc)
	if err != nil {
		t.Errorf("decode of MsgSendCmpct failed [%v] err <%v>", buf,
			err)
	}

	if !reflect.DeepEqual(msg, readMsg) {
		t.Errorf("mismatch\n got: %s want: %s",
			spew.Sdump(readMsg), spew.Sdump(msg))
	}
}

// TestSendCmpctProtocolVersion tests the MsgSendCmpct API against a protocol
// version prior to ShortIdsBlocksVersion.
func TestSendCmpctProtocolVersion(t *testing.T) {
	pver := ShortIdsBlocksVersion - 1
	enc := BaseEncoding
	msg := NewMsgSendCmpct(1, 1)

	// Test encode with an old protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver, enc)
	if err == nil {
		t.Errorf("encode of MsgSendCmpct succeeded when it should have " +
			"failed")
	}

	// Test decode with an old protocol version.
	readMsg := NewMsgSendCmpct(0, 0)
	err = readMsg.BtcDecode(&buf, pver, enc)
	if err == nil {
		t.Errorf("decode of MsgSendCmpct succeeded when it should have " +
			"failed")
	}
}

// TestSendCmpctCrossProtocol tests the MsgSendCmpct API when encoding with the
// latest protocol version and decoding with ShortIdsBlocksVersion.
func TestSendCmpctCrossProtocol(t *testing.T) {
	enc := BaseEncoding
	msg := NewMsgSendCmpct(1, 2)

	// Encode with the latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, ProtocolVersion, enc)
	if err != nil {
		t.Errorf("encode of MsgSendCmpct failed %v err <%v>", msg, err)
	}

	// Decode with the oldest supported protocol version.
	readMsg := NewMsgSendCmpct(0, 0)
	err = readMsg.BtcDecode(&buf, ShortIdsBlocksVersion, enc)
	if err != nil {
		t.Errorf("decode of MsgSendCmpct failed [%v] err <%v>", buf,
			err)
	}

	if !reflect.DeepEqual(msg, readMsg) {
		t.Errorf("encode/decode mismatch\n got: %s want: %s",
			spew.Sdump(readMsg), spew.Sdump(msg))
	}
}

// TestSendCmpctWire tests the MsgSendCmpct wire encode and decode for various
// protocol versions.
func TestSendCmpctWire(t *testing.T) {
	tests := []struct {
		in   *MsgSendCmpct   // Message to encode
		out  *MsgSendCmpct   // Expected decoded message
		buf  []byte          // Wire encoding
		pver uint32          // Protocol version for wire encoding
		enc  MessageEncoding // Message encoding format
	}{
		// Latest protocol version.
		{
			NewMsgSendCmpct(1, 2),
			NewMsgSendCmpct(1, 2),
			[]byte{
				0x01,
				0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
			ProtocolVersion,
			BaseEncoding,
		},

		// Protocol version ShortIdsBlocksVersion+1.
		{
			NewMsgSendCmpct(0, 1),
			NewMsgSendCmpct(0, 1),
			[]byte{
				0x00,
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
			ShortIdsBlocksVersion + 1,
			BaseEncoding,
		},

		// Protocol version ShortIdsBlocksVersion.
		{
			NewMsgSendCmpct(1, 1),
			NewMsgSendCmpct(1, 1),
			[]byte{
				0x01,
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
			ShortIdsBlocksVersion,
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
		var msg MsgSendCmpct
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

// TestSendCmpctWireErrors performs negative tests against wire encode and
// decode of MsgSendCmpct to confirm error paths work correctly.
func TestSendCmpctWireErrors(t *testing.T) {
	pver := ProtocolVersion
	pverNoSendCmpct := ShortIdsBlocksVersion - 1
	wireErr := &MessageError{}

	baseSendCmpct := NewMsgSendCmpct(1, 2)
	baseSendCmpctEncoded := []byte{
		0x01,
		0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	invalidSendCmpct := NewMsgSendCmpct(2, 1)
	invalidSendCmpctEncoded := []byte{
		0x02,
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	tests := []struct {
		in       *MsgSendCmpct // Value to encode
		buf      []byte        // Wire encoding
		pver     uint32        // Protocol version for wire encoding
		max      int           // Max size of fixed buffer to induce errors
		writeErr error         // Expected write error
		readErr  error         // Expected read error
	}{
		// Latest protocol version with intentional read/write errors.
		{baseSendCmpct, baseSendCmpctEncoded, pver, 2,
			io.ErrShortWrite, io.ErrUnexpectedEOF},

		// Force errors due to an unsupported protocol version.
		{baseSendCmpct, baseSendCmpctEncoded, pverNoSendCmpct, 9,
			wireErr, wireErr},

		// Force errors due to an invalid announce field.
		{invalidSendCmpct, invalidSendCmpctEncoded, pver, 9,
			wireErr, wireErr},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := test.in.BtcEncode(w, test.pver, BaseEncoding)
		if reflect.TypeOf(err) != reflect.TypeOf(test.writeErr) {
			t.Errorf("BtcEncode #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		if _, ok := err.(*MessageError); !ok && err != test.writeErr {
			t.Errorf("BtcEncode #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// Decode from wire format.
		var msg MsgSendCmpct
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver, BaseEncoding)
		if reflect.TypeOf(err) != reflect.TypeOf(test.readErr) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		if _, ok := err.(*MessageError); !ok && err != test.readErr {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}
