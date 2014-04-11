// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire_test

import (
	"bytes"
	"github.com/conformal/btcwire"
	"io"
	"testing"
)

// TestFilterCLearLatest tests the MsgFilterAdd API against the latest protocol version.
func TestFilterAddLatest(t *testing.T) {
	pver := btcwire.ProtocolVersion

	data := []byte{0x01, 0x02}
	msg := btcwire.NewMsgFilterAdd(data)

	// Ensure the command is expected value.
	wantCmd := "filteradd"
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgFilterAdd: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	wantPayload := uint32(529)
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
		t.Errorf("encode of MsgFilterAdd failed %v err <%v>", msg, err)
	}

	// Test decode with latest protocol version.
	readmsg := btcwire.MsgFilterAdd{}
	err = readmsg.BtcDecode(&buf, pver)
	if err != nil {
		t.Errorf("decode of MsgFilterAdd failed [%v] err <%v>", buf, err)
	}

	return
}

// TestFilterAddCrossProtocol tests the MsgFilterAdd API when encoding with the latest
// protocol version and decoded with BIP0031Version.
func TestFilterAddCrossProtocol(t *testing.T) {
	data := []byte{0x01, 0x02}
	msg := btcwire.NewMsgFilterAdd(data)

	// Encode with old protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, btcwire.BIP0037Version-1)
	if err == nil {
		t.Errorf("encode of MsgFilterAdd succeeded when it shouldn't have %v",
			msg)
	}

	// Decode with old protocol version.
	readmsg := btcwire.MsgFilterAdd{}
	err = readmsg.BtcDecode(&buf, btcwire.BIP0031Version)
	if err == nil {
		t.Errorf("decode of MsgFilterAdd succeeded when it shouldn't have %v",
			msg)
	}
}

// TestFilterAddMaxDataSize tests the MsgFilterAdd API maximum data size.
func TestFilterAddMaxDataSize(t *testing.T) {
	data := bytes.Repeat([]byte{0xff}, 521)
	msg := btcwire.NewMsgFilterAdd(data)

	// Encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, btcwire.ProtocolVersion)
	if err == nil {
		t.Errorf("encode of MsgFilterAdd succeeded when it shouldn't have %v",
			msg)
	}

	// Decode with latest protocol version.
	readbuf := bytes.NewReader(data)
	err = msg.BtcDecode(readbuf, btcwire.ProtocolVersion)
	if err == nil {
		t.Errorf("decode of MsgFilterAdd succeeded when it shouldn't have %v",
			msg)
	}
}

// TestFilterAddWireErrors performs negative tests against wire encode and decode
// of MsgFilterAdd to confirm error paths work correctly.
func TestFilterAddWireErrors(t *testing.T) {
	pver := btcwire.ProtocolVersion

	tests := []struct {
		in       *btcwire.MsgFilterAdd // Value to encode
		buf      []byte                // Wire encoding
		pver     uint32                // Protocol version for wire encoding
		max      int                   // Max size of fixed buffer to induce errors
		writeErr error                 // Expected write error
		readErr  error                 // Expected read error
	}{
		// Latest protocol version with intentional read/write errors.
		{
			&btcwire.MsgFilterAdd{Data: []byte{0x01, 0x02, 0x03, 0x04}},
			[]byte{
				0x05,             // Varint for size of data
				0x02, 0x03, 0x04, // Data
			},
			pver,
			2,
			io.ErrShortWrite,
			io.ErrUnexpectedEOF,
		},
		{
			&btcwire.MsgFilterAdd{Data: []byte{0x01, 0x02, 0x03, 0x04}},
			[]byte{
				0x05,             // Varint for size of data
				0x02, 0x03, 0x04, // Data
			},
			pver,
			0,
			io.ErrShortWrite,
			io.EOF,
		},
		{
			&btcwire.MsgFilterAdd{Data: []byte{0x01, 0x02, 0x03, 0x04}},
			[]byte{
				0x05,             // Varint for size of data
				0x02, 0x03, 0x04, // Data
			},
			pver,
			1,
			io.ErrShortWrite,
			io.EOF,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := test.in.BtcEncode(w, test.pver)
		if err != test.writeErr {
			t.Errorf("BtcEncode #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// Decode from wire format.
		var msg btcwire.MsgFilterAdd
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if err != test.readErr {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}
