// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"math/rand"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// TestFeeFilterLatest tests the MsgFeeFilter API against the latest protocol version.
func TestFeeFilterLatest(t *testing.T) {
	pver := ProtocolVersion

	minfee := rand.Int63()
	msg := NewMsgFeeFilter(minfee)
	if msg.MinFee != minfee {
		t.Errorf("NewMsgFeeFilter: wrong minfee - got %v, want %v",
			msg.MinFee, minfee)
	}

	// Ensure the command is expected value.
	wantCmd := "feefilter"
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgFeeFilter: wrong command - got %v want %v",
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
	err := msg.BtcEncode(&buf, pver, BaseEncoding)
	if err != nil {
		t.Errorf("encode of MsgFeeFilter failed %v err <%v>", msg, err)
	}

	// Test decode with latest protocol version.
	readmsg := NewMsgFeeFilter(0)
	err = readmsg.BtcDecode(&buf, pver, BaseEncoding)
	if err != nil {
		t.Errorf("decode of MsgFeeFilter failed [%v] err <%v>", buf, err)
	}

	// Ensure minfee is the same.
	if msg.MinFee != readmsg.MinFee {
		t.Errorf("Should get same minfee for protocol version %d", pver)
	}
}

// TestFeeFilterWire tests the MsgFeeFilter wire encode and decode for various protocol
// versions.
func TestFeeFilterWire(t *testing.T) {
	tests := []struct {
		in   MsgFeeFilter // Message to encode
		out  MsgFeeFilter // Expected decoded message
		buf  []byte       // Wire encoding
		pver uint32       // Protocol version for wire encoding
	}{
		// Latest protocol version.
		{
			MsgFeeFilter{MinFee: 123123}, // 0x1e0f3
			MsgFeeFilter{MinFee: 123123}, // 0x1e0f3
			[]byte{0xf3, 0xe0, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00},
			ProtocolVersion,
		},

		// Protocol version FeeFilterVersion
		{
			MsgFeeFilter{MinFee: 456456}, // 0x6f708
			MsgFeeFilter{MinFee: 456456}, // 0x6f708
			[]byte{0x08, 0xf7, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00},
			FeeFilterVersion,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode the message to wire format.
		var buf bytes.Buffer
		err := test.in.BtcEncode(&buf, test.pver, BaseEncoding)
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
		var msg MsgFeeFilter
		rbuf := bytes.NewReader(test.buf)
		err = msg.BtcDecode(rbuf, test.pver, BaseEncoding)
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

// TestFeeFilterWireErrors performs negative tests against wire encode and decode
// of MsgFeeFilter to confirm error paths work correctly.
func TestFeeFilterWireErrors(t *testing.T) {
	pver := ProtocolVersion
	pverNoFeeFilter := FeeFilterVersion - 1
	wireErr := &MessageError{}

	baseFeeFilter := NewMsgFeeFilter(123123) // 0x1e0f3
	baseFeeFilterEncoded := []byte{
		0xf3, 0xe0, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	tests := []struct {
		in       *MsgFeeFilter // Value to encode
		buf      []byte        // Wire encoding
		pver     uint32        // Protocol version for wire encoding
		max      int           // Max size of fixed buffer to induce errors
		writeErr error         // Expected write error
		readErr  error         // Expected read error
	}{
		// Latest protocol version with intentional read/write errors.
		// Force error in minfee.
		{baseFeeFilter, baseFeeFilterEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error due to unsupported protocol version.
		{baseFeeFilter, baseFeeFilterEncoded, pverNoFeeFilter, 4, wireErr, wireErr},
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
		if _, ok := err.(*MessageError); !ok {
			if err != test.writeErr {
				t.Errorf("BtcEncode #%d wrong error got: %v, "+
					"want: %v", i, err, test.writeErr)
				continue
			}
		}

		// Decode from wire format.
		var msg MsgFeeFilter
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver, BaseEncoding)
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
