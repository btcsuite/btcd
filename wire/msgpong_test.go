// Copyright (c) 2013-2016 The btcsuite developers
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

// TestPongLatest tests the MsgPong API against the latest protocol version.
func TestPongLatest(t *testing.T) {
	pver := ProtocolVersion

	nonce, err := RandomUint64()
	if err != nil {
		t.Errorf("RandomUint64: error generating nonce: %v", err)
	}
	msg := NewMsgPong(nonce)
	if msg.Nonce != nonce {
		t.Errorf("NewMsgPong: wrong nonce - got %v, want %v",
			msg.Nonce, nonce)
	}

	// Ensure the command is expected value.
	wantCmd := "pong"
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgPong: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	wantPayload := uint32(8)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err = msg.BtcEncode(&buf, pver)
	if err != nil {
		t.Errorf("encode of MsgPong failed %v err <%v>", msg, err)
	}

	// Test decode with latest protocol version.
	readmsg := NewMsgPong(0)
	err = readmsg.BtcDecode(&buf, pver)
	if err != nil {
		t.Errorf("decode of MsgPong failed [%v] err <%v>", buf, err)
	}

	// Ensure nonce is the same.
	if msg.Nonce != readmsg.Nonce {
		t.Errorf("Should get same nonce for protocol version %d", pver)
	}

	return
}

// TestPongBIP0031 tests the MsgPong API against the protocol version
// BIP0031Version.
func TestPongBIP0031(t *testing.T) {
	// Use the protocol version just prior to BIP0031Version changes.
	pver := BIP0031Version

	nonce, err := RandomUint64()
	if err != nil {
		t.Errorf("Error generating nonce: %v", err)
	}
	msg := NewMsgPong(nonce)
	if msg.Nonce != nonce {
		t.Errorf("Should get same nonce back out.")
	}

	// Ensure max payload is expected value for old protocol version.
	size := msg.MaxPayloadLength(pver)
	if size != 0 {
		t.Errorf("Max length should be 0 for pong protocol version %d.",
			pver)
	}

	// Test encode with old protocol version.
	var buf bytes.Buffer
	err = msg.BtcEncode(&buf, pver)
	if err == nil {
		t.Errorf("encode of MsgPong succeeded when it shouldn't have %v",
			msg)
	}

	// Test decode with old protocol version.
	readmsg := NewMsgPong(0)
	err = readmsg.BtcDecode(&buf, pver)
	if err == nil {
		t.Errorf("decode of MsgPong succeeded when it shouldn't have %v",
			spew.Sdump(buf))
	}

	// Since this protocol version doesn't support pong, make sure the
	// nonce didn't get encoded and decoded back out.
	if msg.Nonce == readmsg.Nonce {
		t.Errorf("Should not get same nonce for protocol version %d", pver)
	}

	return
}

// TestPongCrossProtocol tests the MsgPong API when encoding with the latest
// protocol version and decoding with BIP0031Version.
func TestPongCrossProtocol(t *testing.T) {
	nonce, err := RandomUint64()
	if err != nil {
		t.Errorf("Error generating nonce: %v", err)
	}
	msg := NewMsgPong(nonce)
	if msg.Nonce != nonce {
		t.Errorf("Should get same nonce back out.")
	}

	// Encode with latest protocol version.
	var buf bytes.Buffer
	err = msg.BtcEncode(&buf, ProtocolVersion)
	if err != nil {
		t.Errorf("encode of MsgPong failed %v err <%v>", msg, err)
	}

	// Decode with old protocol version.
	readmsg := NewMsgPong(0)
	err = readmsg.BtcDecode(&buf, BIP0031Version)
	if err == nil {
		t.Errorf("encode of MsgPong succeeded when it shouldn't have %v",
			msg)
	}

	// Since one of the protocol versions doesn't support the pong message,
	// make sure the nonce didn't get encoded and decoded back out.
	if msg.Nonce == readmsg.Nonce {
		t.Error("Should not get same nonce for cross protocol")
	}
}

// TestPongWire tests the MsgPong wire encode and decode for various protocol
// versions.
func TestPongWire(t *testing.T) {
	tests := []struct {
		in   MsgPong // Message to encode
		out  MsgPong // Expected decoded message
		buf  []byte  // Wire encoding
		pver uint32  // Protocol version for wire encoding
	}{
		// Latest protocol version.
		{
			MsgPong{Nonce: 123123}, // 0x1e0f3
			MsgPong{Nonce: 123123}, // 0x1e0f3
			[]byte{0xf3, 0xe0, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00},
			ProtocolVersion,
		},

		// Protocol version BIP0031Version+1
		{
			MsgPong{Nonce: 456456}, // 0x6f708
			MsgPong{Nonce: 456456}, // 0x6f708
			[]byte{0x08, 0xf7, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00},
			BIP0031Version + 1,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode the message to wire format.
		var buf bytes.Buffer
		err := test.in.BtcEncode(&buf, test.pver)
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
		var msg MsgPong
		rbuf := bytes.NewReader(test.buf)
		err = msg.BtcDecode(rbuf, test.pver)
		if err != nil {
			t.Errorf("BtcDecode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(msg, test.out) {
			t.Errorf("BtcDecode #%d\n got: %s want: %s", i,
				spew.Sdump(msg), spew.Sdump(test.out))
			continue
		}
	}
}

// TestPongWireErrors performs negative tests against wire encode and decode
// of MsgPong to confirm error paths work correctly.
func TestPongWireErrors(t *testing.T) {
	pver := ProtocolVersion
	pverNoPong := BIP0031Version
	wireErr := &MessageError{}

	basePong := NewMsgPong(123123) // 0x1e0f3
	basePongEncoded := []byte{
		0xf3, 0xe0, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	tests := []struct {
		in       *MsgPong // Value to encode
		buf      []byte   // Wire encoding
		pver     uint32   // Protocol version for wire encoding
		max      int      // Max size of fixed buffer to induce errors
		writeErr error    // Expected write error
		readErr  error    // Expected read error
	}{
		// Latest protocol version with intentional read/write errors.
		// Force error in nonce.
		{basePong, basePongEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error due to unsupported protocol version.
		{basePong, basePongEncoded, pverNoPong, 4, wireErr, wireErr},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := test.in.BtcEncode(w, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.writeErr) {
			t.Errorf("BtcEncode #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		if _, ok := err.(*MessageError); !ok {
			if err != test.writeErr {
				t.Errorf("BtcEncode #%d wrong error got: %v, "+
					"want: %v", i, err, test.writeErr)
				continue
			}
		}

		// Decode from wire format.
		var msg MsgPong
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.readErr) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		if _, ok := err.(*MessageError); !ok {
			if err != test.readErr {
				t.Errorf("BtcDecode #%d wrong error got: %v, "+
					"want: %v", i, err, test.readErr)
				continue
			}
		}

	}
}
