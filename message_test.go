// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire_test

import (
	"bytes"
	"encoding/binary"
	"github.com/conformal/btcwire"
	"github.com/davecgh/go-spew/spew"
	"io"
	"net"
	"reflect"
	"testing"
	"time"
)

// makeHeader is a convenience function to make a message header in the form of
// a byte slice.  It is used to force errors when reading messages.
func makeHeader(btcnet btcwire.BitcoinNet, command string,
	payloadLen uint32, checksum uint32) []byte {

	// The length of a bitcoin message header is 24 bytes.
	// 4 byte magic number of the bitcoin network + 12 byte command + 4 byte
	// payload length + 4 byte checksum.
	buf := make([]byte, 24)
	binary.LittleEndian.PutUint32(buf, uint32(btcnet))
	copy(buf[4:], []byte(command))
	binary.LittleEndian.PutUint32(buf[16:], payloadLen)
	binary.LittleEndian.PutUint32(buf[20:], checksum)
	return buf
}

// TestMessage tests the Read/WriteMessage API.
func TestMessage(t *testing.T) {
	pver := btcwire.ProtocolVersion

	// Create the various types of messages to test.

	// MsgVersion.
	addrYou := &net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8333}
	you, err := btcwire.NewNetAddress(addrYou, btcwire.SFNodeNetwork)
	if err != nil {
		t.Errorf("NewNetAddress: %v", err)
	}
	you.Timestamp = time.Time{} // Version message has zero value timestamp.
	addrMe := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333}
	me, err := btcwire.NewNetAddress(addrMe, btcwire.SFNodeNetwork)
	if err != nil {
		t.Errorf("NewNetAddress: %v", err)
	}
	me.Timestamp = time.Time{} // Version message has zero value timestamp.
	msgVersion := btcwire.NewMsgVersion(me, you, 123123, "/test:0.0.1/", 0)

	msgVerack := btcwire.NewMsgVerAck()
	msgGetAddr := btcwire.NewMsgGetAddr()
	msgAddr := btcwire.NewMsgAddr()
	msgGetBlocks := btcwire.NewMsgGetBlocks(&btcwire.ShaHash{})
	msgBlock := &blockOne
	msgInv := btcwire.NewMsgInv()
	msgGetData := btcwire.NewMsgGetData()
	msgNotFound := btcwire.NewMsgNotFound()
	msgTx := btcwire.NewMsgTx()
	msgPing := btcwire.NewMsgPing(123123)
	msgPong := btcwire.NewMsgPong(123123)
	msgGetHeaders := btcwire.NewMsgGetHeaders()
	msgHeaders := btcwire.NewMsgHeaders()
	msgAlert := btcwire.NewMsgAlert("payload", "signature")
	msgMemPool := btcwire.NewMsgMemPool()

	tests := []struct {
		in     btcwire.Message    // Value to encode
		out    btcwire.Message    // Expected decoded value
		pver   uint32             // Protocol version for wire encoding
		btcnet btcwire.BitcoinNet // Network to use for wire encoding
	}{
		{msgVersion, msgVersion, pver, btcwire.MainNet},
		{msgVerack, msgVerack, pver, btcwire.MainNet},
		{msgGetAddr, msgGetAddr, pver, btcwire.MainNet},
		{msgAddr, msgAddr, pver, btcwire.MainNet},
		{msgGetBlocks, msgGetBlocks, pver, btcwire.MainNet},
		{msgBlock, msgBlock, pver, btcwire.MainNet},
		{msgInv, msgInv, pver, btcwire.MainNet},
		{msgGetData, msgGetData, pver, btcwire.MainNet},
		{msgNotFound, msgNotFound, pver, btcwire.MainNet},
		{msgTx, msgTx, pver, btcwire.MainNet},
		{msgPing, msgPing, pver, btcwire.MainNet},
		{msgPong, msgPong, pver, btcwire.MainNet},
		{msgGetHeaders, msgGetHeaders, pver, btcwire.MainNet},
		{msgHeaders, msgHeaders, pver, btcwire.MainNet},
		{msgAlert, msgAlert, pver, btcwire.MainNet},
		{msgMemPool, msgMemPool, pver, btcwire.MainNet},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		var buf bytes.Buffer
		err := btcwire.WriteMessage(&buf, test.in, test.pver, test.btcnet)
		if err != nil {
			t.Errorf("WriteMessage #%d error %v", i, err)
			continue
		}

		// Decode from wire format.
		rbuf := bytes.NewBuffer(buf.Bytes())
		msg, _, err := btcwire.ReadMessage(rbuf, test.pver, test.btcnet)
		if err != nil {
			t.Errorf("ReadMessage #%d error %v, msg %v", i, err,
				spew.Sdump(msg))
			continue
		}
		if !reflect.DeepEqual(msg, test.out) {
			t.Errorf("ReadMessage #%d\n got: %v want: %v", i,
				spew.Sdump(msg), spew.Sdump(test.out))
			continue
		}
	}
}

// TestReadMessageWireErrors performs negative tests against wire decoding into
// concrete messages to confirm error paths work correctly.
func TestReadMessageWireErrors(t *testing.T) {
	pver := btcwire.ProtocolVersion
	btcnet := btcwire.MainNet

	// Ensure message errors are as expected with no function specified.
	wantErr := "something bad happened"
	testErr := btcwire.MessageError{Description: wantErr}
	if testErr.Error() != wantErr {
		t.Errorf("MessageError: wrong error - got %v, want %v",
			testErr.Error(), wantErr)
	}

	// Ensure message errors are as expected with a function specified.
	wantFunc := "foo"
	testErr = btcwire.MessageError{Func: wantFunc, Description: wantErr}
	if testErr.Error() != wantFunc+": "+wantErr {
		t.Errorf("MessageError: wrong error - got %v, want %v",
			testErr.Error(), wantErr)
	}

	// Wire encoded bytes for main and testnet3 networks magic identifiers.
	testNet3Bytes := makeHeader(btcwire.TestNet3, "", 0, 0)

	// Wire encoded bytes for a message that exceeds max overall message
	// length.
	mpl := btcwire.MaxMessagePayload
	exceedMaxPayloadBytes := makeHeader(btcnet, "getaddr", mpl+1, 0)

	// Wire encoded bytes for a command which is invalid utf-8.
	badCommandBytes := makeHeader(btcnet, "bogus", 0, 0)
	badCommandBytes[4] = 0x81

	// Wire encoded bytes for a command which is valid, but not supported.
	unsupportedCommandBytes := makeHeader(btcnet, "bogus", 0, 0)

	// Wire encoded bytes for a message which exceeds the max payload for
	// a specific message type.
	exceedTypePayloadBytes := makeHeader(btcnet, "getaddr", 1, 0)

	// Wire encoded bytes for a message which does not deliver the full
	// payload according to the header length.
	shortPayloadBytes := makeHeader(btcnet, "version", 115, 0)

	// Wire encoded bytes for a message with a bad checksum.
	badChecksumBytes := makeHeader(btcnet, "version", 2, 0xbeef)
	badChecksumBytes = append(badChecksumBytes, []byte{0x0, 0x0}...)

	// Wire encoded bytes for a message which has a valid header, but is
	// the wrong format.  An addr starts with a varint of the number of
	// contained in the message.  Claim there is two, but don't provide
	// them.  At the same time, forge the header fields so the message is
	// otherwise accurate.
	badMessageBytes := makeHeader(btcnet, "addr", 1, 0xeaadc31c)
	badMessageBytes = append(badMessageBytes, 0x2)

	tests := []struct {
		buf     []byte             // Wire encoding
		pver    uint32             // Protocol version for wire encoding
		btcnet  btcwire.BitcoinNet // Bitcoin network for wire encoding
		max     int                // Max size of fixed buffer to induce errors
		readErr error              // Expected read error
	}{
		// Latest protocol version with intentional read errors.

		// Short header.
		{
			[]byte{},
			pver,
			btcnet,
			0,
			io.EOF,
		},

		// Wrong network.  Want MainNet, but giving TestNet3.
		{
			testNet3Bytes,
			pver,
			btcnet,
			len(testNet3Bytes),
			&btcwire.MessageError{},
		},

		// Exceed max overall message payload length.
		{
			exceedMaxPayloadBytes,
			pver,
			btcnet,
			len(exceedMaxPayloadBytes),
			&btcwire.MessageError{},
		},

		// Invalid UTF-8 command.
		{
			badCommandBytes,
			pver,
			btcnet,
			len(badCommandBytes),
			&btcwire.MessageError{},
		},

		// Valid, but unsupported command.
		{
			unsupportedCommandBytes,
			pver,
			btcnet,
			len(unsupportedCommandBytes),
			&btcwire.MessageError{},
		},

		// Exceed max allowed payload for a message of a specific type.
		{
			exceedTypePayloadBytes,
			pver,
			btcnet,
			len(exceedTypePayloadBytes),
			&btcwire.MessageError{},
		},

		// Message with a payload shorter than the header indicates.
		{
			shortPayloadBytes,
			pver,
			btcnet,
			len(shortPayloadBytes),
			io.EOF,
		},

		// Message with a bad checksum.
		{
			badChecksumBytes,
			pver,
			btcnet,
			len(badChecksumBytes),
			&btcwire.MessageError{},
		},

		// Message with a valid header, but wrong format.
		{
			badMessageBytes,
			pver,
			btcnet,
			len(badMessageBytes),
			io.EOF,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire format.
		r := newFixedReader(test.max, test.buf)
		_, _, err := btcwire.ReadMessage(r, test.pver, test.btcnet)
		if reflect.TypeOf(err) != reflect.TypeOf(test.readErr) {
			t.Errorf("ReadMessage #%d wrong error got: %v <%T>, "+
				"want: %T", i, err, err, test.readErr)
			continue
		}

		// For errors which are not of type btcwire.MessageError, check
		// them for equality.
		if _, ok := err.(*btcwire.MessageError); !ok {
			if err != test.readErr {
				t.Errorf("ReadMessage #%d wrong error got: %v <%T>, "+
					"want: %v <%T>", i, err, err,
					test.readErr, test.readErr)
				continue
			}
		}
	}
}
