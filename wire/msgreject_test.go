// Copyright (c) 2014-2016 The btcsuite developers
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

// TestRejectCodeStringer tests the stringized output for the reject code type.
func TestRejectCodeStringer(t *testing.T) {
	tests := []struct {
		in   RejectCode
		want string
	}{
		{RejectMalformed, "REJECT_MALFORMED"},
		{RejectInvalid, "REJECT_INVALID"},
		{RejectObsolete, "REJECT_OBSOLETE"},
		{RejectDuplicate, "REJECT_DUPLICATE"},
		{RejectNonstandard, "REJECT_NONSTANDARD"},
		{RejectDust, "REJECT_DUST"},
		{RejectInsufficientFee, "REJECT_INSUFFICIENTFEE"},
		{RejectCheckpoint, "REJECT_CHECKPOINT"},
		{0xff, "Unknown RejectCode (255)"},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.String()
		if result != test.want {
			t.Errorf("String #%d\n got: %s want: %s", i, result,
				test.want)
			continue
		}
	}

}

// TestRejectLatest tests the MsgPong API against the latest protocol version.
func TestRejectLatest(t *testing.T) {
	pver := ProtocolVersion

	// Create reject message data.
	rejCommand := (&MsgBlock{}).Command()
	rejCode := RejectDuplicate
	rejReason := "duplicate block"
	rejHash := mainNetGenesisHash

	// Ensure we get the correct data back out.
	msg := NewMsgReject(rejCommand, rejCode, rejReason)
	msg.Hash = rejHash
	if msg.Cmd != rejCommand {
		t.Errorf("NewMsgReject: wrong rejected command - got %v, "+
			"want %v", msg.Cmd, rejCommand)
	}
	if msg.Code != rejCode {
		t.Errorf("NewMsgReject: wrong rejected code - got %v, "+
			"want %v", msg.Code, rejCode)
	}
	if msg.Reason != rejReason {
		t.Errorf("NewMsgReject: wrong rejected reason - got %v, "+
			"want %v", msg.Reason, rejReason)
	}

	// Ensure the command is expected value.
	wantCmd := "reject"
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgReject: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	wantPayload := uint32(MaxMessagePayload)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver)
	if err != nil {
		t.Errorf("encode of MsgReject failed %v err <%v>", msg, err)
	}

	// Test decode with latest protocol version.
	readMsg := MsgReject{}
	err = readMsg.BtcDecode(&buf, pver)
	if err != nil {
		t.Errorf("decode of MsgReject failed %v err <%v>", buf.Bytes(),
			err)
	}

	// Ensure decoded data is the same.
	if msg.Cmd != readMsg.Cmd {
		t.Errorf("Should get same reject command - got %v, want %v",
			readMsg.Cmd, msg.Cmd)
	}
	if msg.Code != readMsg.Code {
		t.Errorf("Should get same reject code - got %v, want %v",
			readMsg.Code, msg.Code)
	}
	if msg.Reason != readMsg.Reason {
		t.Errorf("Should get same reject reason - got %v, want %v",
			readMsg.Reason, msg.Reason)
	}
	if msg.Hash != readMsg.Hash {
		t.Errorf("Should get same reject hash - got %v, want %v",
			readMsg.Hash, msg.Hash)
	}
}

// TestRejectBeforeAdded tests the MsgReject API against a protocol version
// before the version which introduced it (RejectVersion).
func TestRejectBeforeAdded(t *testing.T) {
	// Use the protocol version just prior to RejectVersion.
	pver := RejectVersion - 1

	// Create reject message data.
	rejCommand := (&MsgBlock{}).Command()
	rejCode := RejectDuplicate
	rejReason := "duplicate block"
	rejHash := mainNetGenesisHash

	msg := NewMsgReject(rejCommand, rejCode, rejReason)
	msg.Hash = rejHash

	// Ensure max payload is expected value for old protocol version.
	size := msg.MaxPayloadLength(pver)
	if size != 0 {
		t.Errorf("Max length should be 0 for reject protocol version %d.",
			pver)
	}

	// Test encode with old protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver)
	if err == nil {
		t.Errorf("encode of MsgReject succeeded when it shouldn't "+
			"have %v", msg)
	}

	//	// Test decode with old protocol version.
	readMsg := MsgReject{}
	err = readMsg.BtcDecode(&buf, pver)
	if err == nil {
		t.Errorf("decode of MsgReject succeeded when it shouldn't "+
			"have %v", spew.Sdump(buf.Bytes()))
	}

	// Since this protocol version doesn't support reject, make sure various
	// fields didn't get encoded and decoded back out.
	if msg.Cmd == readMsg.Cmd {
		t.Errorf("Should not get same reject command for protocol "+
			"version %d", pver)
	}
	if msg.Code == readMsg.Code {
		t.Errorf("Should not get same reject code for protocol "+
			"version %d", pver)
	}
	if msg.Reason == readMsg.Reason {
		t.Errorf("Should not get same reject reason for protocol "+
			"version %d", pver)
	}
	if msg.Hash == readMsg.Hash {
		t.Errorf("Should not get same reject hash for protocol "+
			"version %d", pver)
	}
}

// TestRejectCrossProtocol tests the MsgReject API when encoding with the latest
// protocol version and decoded with a version before the version which
// introduced it (RejectVersion).
func TestRejectCrossProtocol(t *testing.T) {
	// Create reject message data.
	rejCommand := (&MsgBlock{}).Command()
	rejCode := RejectDuplicate
	rejReason := "duplicate block"
	rejHash := mainNetGenesisHash

	msg := NewMsgReject(rejCommand, rejCode, rejReason)
	msg.Hash = rejHash

	// Encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, ProtocolVersion)
	if err != nil {
		t.Errorf("encode of MsgReject failed %v err <%v>", msg, err)
	}

	// Decode with old protocol version.
	readMsg := MsgReject{}
	err = readMsg.BtcDecode(&buf, RejectVersion-1)
	if err == nil {
		t.Errorf("encode of MsgReject succeeded when it shouldn't "+
			"have %v", msg)
	}

	// Since one of the protocol versions doesn't support the reject
	// message, make sure the various fields didn't get encoded and decoded
	// back out.
	if msg.Cmd == readMsg.Cmd {
		t.Errorf("Should not get same reject command for cross protocol")
	}
	if msg.Code == readMsg.Code {
		t.Errorf("Should not get same reject code for cross protocol")
	}
	if msg.Reason == readMsg.Reason {
		t.Errorf("Should not get same reject reason for cross protocol")
	}
	if msg.Hash == readMsg.Hash {
		t.Errorf("Should not get same reject hash for cross protocol")
	}
}

// TestRejectWire tests the MsgReject wire encode and decode for various
// protocol versions.
func TestRejectWire(t *testing.T) {
	tests := []struct {
		msg  MsgReject // Message to encode
		buf  []byte    // Wire encoding
		pver uint32    // Protocol version for wire encoding
	}{
		// Latest protocol version rejected command version (no hash).
		{
			MsgReject{
				Cmd:    "version",
				Code:   RejectDuplicate,
				Reason: "duplicate version",
			},
			[]byte{
				0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, // "version"
				0x12, // RejectDuplicate
				0x11, 0x64, 0x75, 0x70, 0x6c, 0x69, 0x63, 0x61,
				0x74, 0x65, 0x20, 0x76, 0x65, 0x72, 0x73, 0x69,
				0x6f, 0x6e, // "duplicate version"
			},
			ProtocolVersion,
		},
		// Latest protocol version rejected command block (has hash).
		{
			MsgReject{
				Cmd:    "block",
				Code:   RejectDuplicate,
				Reason: "duplicate block",
				Hash:   mainNetGenesisHash,
			},
			[]byte{
				0x05, 0x62, 0x6c, 0x6f, 0x63, 0x6b, // "block"
				0x12, // RejectDuplicate
				0x0f, 0x64, 0x75, 0x70, 0x6c, 0x69, 0x63, 0x61,
				0x74, 0x65, 0x20, 0x62, 0x6c, 0x6f, 0x63, 0x6b, // "duplicate block"
				0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
				0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
				0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
				0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, // mainNetGenesisHash
			},
			ProtocolVersion,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode the message to wire format.
		var buf bytes.Buffer
		err := test.msg.BtcEncode(&buf, test.pver)
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
		var msg MsgReject
		rbuf := bytes.NewReader(test.buf)
		err = msg.BtcDecode(rbuf, test.pver)
		if err != nil {
			t.Errorf("BtcDecode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(msg, test.msg) {
			t.Errorf("BtcDecode #%d\n got: %s want: %s", i,
				spew.Sdump(msg), spew.Sdump(test.msg))
			continue
		}
	}
}

// TestRejectWireErrors performs negative tests against wire encode and decode
// of MsgReject to confirm error paths work correctly.
func TestRejectWireErrors(t *testing.T) {
	pver := ProtocolVersion
	pverNoReject := RejectVersion - 1
	wireErr := &MessageError{}

	baseReject := NewMsgReject("block", RejectDuplicate, "duplicate block")
	baseReject.Hash = mainNetGenesisHash
	baseRejectEncoded := []byte{
		0x05, 0x62, 0x6c, 0x6f, 0x63, 0x6b, // "block"
		0x12, // RejectDuplicate
		0x0f, 0x64, 0x75, 0x70, 0x6c, 0x69, 0x63, 0x61,
		0x74, 0x65, 0x20, 0x62, 0x6c, 0x6f, 0x63, 0x6b, // "duplicate block"
		0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
		0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
		0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
		0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, // mainNetGenesisHash
	}

	tests := []struct {
		in       *MsgReject // Value to encode
		buf      []byte     // Wire encoding
		pver     uint32     // Protocol version for wire encoding
		max      int        // Max size of fixed buffer to induce errors
		writeErr error      // Expected write error
		readErr  error      // Expected read error
	}{
		// Latest protocol version with intentional read/write errors.
		// Force error in reject command.
		{baseReject, baseRejectEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in reject code.
		{baseReject, baseRejectEncoded, pver, 6, io.ErrShortWrite, io.EOF},
		// Force error in reject reason.
		{baseReject, baseRejectEncoded, pver, 7, io.ErrShortWrite, io.EOF},
		// Force error in reject hash.
		{baseReject, baseRejectEncoded, pver, 23, io.ErrShortWrite, io.EOF},
		// Force error due to unsupported protocol version.
		{baseReject, baseRejectEncoded, pverNoReject, 6, wireErr, wireErr},
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
		var msg MsgReject
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
