// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/davecgh/go-spew/spew"
)

// TestGetHeaders tests the MsgGetHeader API.
func TestGetHeaders(t *testing.T) {
	pver := ProtocolVersion

	// Block 99500 hash.
	hashStr := "000000000002e7ad7b9eef9479e4aabc65cb831269cc20d2632c13684406dee0"
	locatorHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
	}

	// Ensure the command is expected value.
	wantCmd := "getheaders"
	msg := NewMsgGetHeaders()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgGetHeaders: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Protocol version 4 bytes + num hashes (varInt) + max block locator
	// hashes + hash stop.
	wantPayload := uint32(16045)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Ensure block locator hashes are added properly.
	err = msg.AddBlockLocatorHash(locatorHash)
	if err != nil {
		t.Errorf("AddBlockLocatorHash: %v", err)
	}
	if msg.BlockLocatorHashes[0] != locatorHash {
		t.Errorf("AddBlockLocatorHash: wrong block locator added - "+
			"got %v, want %v",
			spew.Sprint(msg.BlockLocatorHashes[0]),
			spew.Sprint(locatorHash))
	}

	// Ensure adding more than the max allowed block locator hashes per
	// message returns an error.
	for i := 0; i < MaxBlockLocatorsPerMsg; i++ {
		err = msg.AddBlockLocatorHash(locatorHash)
	}
	if err == nil {
		t.Errorf("AddBlockLocatorHash: expected error on too many " +
			"block locator hashes not received")
	}
}

// TestGetHeadersWire tests the MsgGetHeaders wire encode and decode for various
// numbers of block locator hashes and protocol versions.
func TestGetHeadersWire(t *testing.T) {
	// Set protocol inside getheaders message.  Use protocol version 60002
	// specifically here instead of the latest because the test data is
	// using bytes encoded with that protocol version.
	pver := uint32(60002)

	// Block 99499 hash.
	hashStr := "2710f40c87ec93d010a6fd95f42c59a2cbacc60b18cf6b7957535"
	hashLocator, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
	}

	// Block 99500 hash.
	hashStr = "2e7ad7b9eef9479e4aabc65cb831269cc20d2632c13684406dee0"
	hashLocator2, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
	}

	// Block 100000 hash.
	hashStr = "3ba27aa200b1cecaad478d2b00432346c3f1f3986da1afd33e506"
	hashStop, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
	}

	// MsgGetHeaders message with no block locators or stop hash.
	noLocators := NewMsgGetHeaders()
	noLocators.ProtocolVersion = pver
	noLocatorsEncoded := []byte{
		0x62, 0xea, 0x00, 0x00, // Protocol version 60002
		0x00, // Varint for number of block locator hashes
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Hash stop
	}

	// MsgGetHeaders message with multiple block locators and a stop hash.
	multiLocators := NewMsgGetHeaders()
	multiLocators.ProtocolVersion = pver
	multiLocators.HashStop = *hashStop
	multiLocators.AddBlockLocatorHash(hashLocator2)
	multiLocators.AddBlockLocatorHash(hashLocator)
	multiLocatorsEncoded := []byte{
		0x62, 0xea, 0x00, 0x00, // Protocol version 60002
		0x02, // Varint for number of block locator hashes
		0xe0, 0xde, 0x06, 0x44, 0x68, 0x13, 0x2c, 0x63,
		0xd2, 0x20, 0xcc, 0x69, 0x12, 0x83, 0xcb, 0x65,
		0xbc, 0xaa, 0xe4, 0x79, 0x94, 0xef, 0x9e, 0x7b,
		0xad, 0xe7, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, // Block 99500 hash
		0x35, 0x75, 0x95, 0xb7, 0xf6, 0x8c, 0xb1, 0x60,
		0xcc, 0xba, 0x2c, 0x9a, 0xc5, 0x42, 0x5f, 0xd9,
		0x6f, 0x0a, 0x01, 0x3d, 0xc9, 0x7e, 0xc8, 0x40,
		0x0f, 0x71, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, // Block 99499 hash
		0x06, 0xe5, 0x33, 0xfd, 0x1a, 0xda, 0x86, 0x39,
		0x1f, 0x3f, 0x6c, 0x34, 0x32, 0x04, 0xb0, 0xd2,
		0x78, 0xd4, 0xaa, 0xec, 0x1c, 0x0b, 0x20, 0xaa,
		0x27, 0xba, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, // Hash stop
	}

	tests := []struct {
		in   *MsgGetHeaders  // Message to encode
		out  *MsgGetHeaders  // Expected decoded message
		buf  []byte          // Wire encoding
		pver uint32          // Protocol version for wire encoding
		enc  MessageEncoding // Message encoding format
	}{
		// Latest protocol version with no block locators.
		{
			noLocators,
			noLocators,
			noLocatorsEncoded,
			ProtocolVersion,
			BaseEncoding,
		},

		// Latest protocol version with multiple block locators.
		{
			multiLocators,
			multiLocators,
			multiLocatorsEncoded,
			ProtocolVersion,
			BaseEncoding,
		},

		// Protocol version BIP0035Version with no block locators.
		{
			noLocators,
			noLocators,
			noLocatorsEncoded,
			BIP0035Version,
			BaseEncoding,
		},

		// Protocol version BIP0035Version with multiple block locators.
		{
			multiLocators,
			multiLocators,
			multiLocatorsEncoded,
			BIP0035Version,
			BaseEncoding,
		},

		// Protocol version BIP0031Version with no block locators.
		{
			noLocators,
			noLocators,
			noLocatorsEncoded,
			BIP0031Version,
			BaseEncoding,
		},

		// Protocol version BIP0031Versionwith multiple block locators.
		{
			multiLocators,
			multiLocators,
			multiLocatorsEncoded,
			BIP0031Version,
			BaseEncoding,
		},

		// Protocol version NetAddressTimeVersion with no block locators.
		{
			noLocators,
			noLocators,
			noLocatorsEncoded,
			NetAddressTimeVersion,
			BaseEncoding,
		},

		// Protocol version NetAddressTimeVersion multiple block locators.
		{
			multiLocators,
			multiLocators,
			multiLocatorsEncoded,
			NetAddressTimeVersion,
			BaseEncoding,
		},

		// Protocol version MultipleAddressVersion with no block locators.
		{
			noLocators,
			noLocators,
			noLocatorsEncoded,
			MultipleAddressVersion,
			BaseEncoding,
		},

		// Protocol version MultipleAddressVersion multiple block locators.
		{
			multiLocators,
			multiLocators,
			multiLocatorsEncoded,
			MultipleAddressVersion,
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
		var msg MsgGetHeaders
		rbuf := bytes.NewReader(test.buf)
		err = msg.BtcDecode(rbuf, test.pver, test.enc)
		if err != nil {
			t.Errorf("BtcDecode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&msg, test.out) {
			t.Errorf("BtcDecode #%d\n got: %s want: %s", i,
				spew.Sdump(&msg), spew.Sdump(test.out))
			continue
		}
	}
}

// TestGetHeadersWireErrors performs negative tests against wire encode and
// decode of MsgGetHeaders to confirm error paths work correctly.
func TestGetHeadersWireErrors(t *testing.T) {
	// Set protocol inside getheaders message.  Use protocol version 60002
	// specifically here instead of the latest because the test data is
	// using bytes encoded with that protocol version.
	pver := uint32(60002)
	wireErr := &MessageError{}

	// Block 99499 hash.
	hashStr := "2710f40c87ec93d010a6fd95f42c59a2cbacc60b18cf6b7957535"
	hashLocator, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
	}

	// Block 99500 hash.
	hashStr = "2e7ad7b9eef9479e4aabc65cb831269cc20d2632c13684406dee0"
	hashLocator2, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
	}

	// Block 100000 hash.
	hashStr = "3ba27aa200b1cecaad478d2b00432346c3f1f3986da1afd33e506"
	hashStop, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
	}

	// MsgGetHeaders message with multiple block locators and a stop hash.
	baseGetHeaders := NewMsgGetHeaders()
	baseGetHeaders.ProtocolVersion = pver
	baseGetHeaders.HashStop = *hashStop
	baseGetHeaders.AddBlockLocatorHash(hashLocator2)
	baseGetHeaders.AddBlockLocatorHash(hashLocator)
	baseGetHeadersEncoded := []byte{
		0x62, 0xea, 0x00, 0x00, // Protocol version 60002
		0x02, // Varint for number of block locator hashes
		0xe0, 0xde, 0x06, 0x44, 0x68, 0x13, 0x2c, 0x63,
		0xd2, 0x20, 0xcc, 0x69, 0x12, 0x83, 0xcb, 0x65,
		0xbc, 0xaa, 0xe4, 0x79, 0x94, 0xef, 0x9e, 0x7b,
		0xad, 0xe7, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, // Block 99500 hash
		0x35, 0x75, 0x95, 0xb7, 0xf6, 0x8c, 0xb1, 0x60,
		0xcc, 0xba, 0x2c, 0x9a, 0xc5, 0x42, 0x5f, 0xd9,
		0x6f, 0x0a, 0x01, 0x3d, 0xc9, 0x7e, 0xc8, 0x40,
		0x0f, 0x71, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, // Block 99499 hash
		0x06, 0xe5, 0x33, 0xfd, 0x1a, 0xda, 0x86, 0x39,
		0x1f, 0x3f, 0x6c, 0x34, 0x32, 0x04, 0xb0, 0xd2,
		0x78, 0xd4, 0xaa, 0xec, 0x1c, 0x0b, 0x20, 0xaa,
		0x27, 0xba, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, // Hash stop
	}

	// Message that forces an error by having more than the max allowed
	// block locator hashes.
	maxGetHeaders := NewMsgGetHeaders()
	for i := 0; i < MaxBlockLocatorsPerMsg; i++ {
		maxGetHeaders.AddBlockLocatorHash(&mainNetGenesisHash)
	}
	maxGetHeaders.BlockLocatorHashes = append(maxGetHeaders.BlockLocatorHashes,
		&mainNetGenesisHash)
	maxGetHeadersEncoded := []byte{
		0x62, 0xea, 0x00, 0x00, // Protocol version 60002
		0xfd, 0xf5, 0x01, // Varint for number of block loc hashes (501)
	}

	tests := []struct {
		in       *MsgGetHeaders  // Value to encode
		buf      []byte          // Wire encoding
		pver     uint32          // Protocol version for wire encoding
		enc      MessageEncoding // Message encoding format
		max      int             // Max size of fixed buffer to induce errors
		writeErr error           // Expected write error
		readErr  error           // Expected read error
	}{
		// Force error in protocol version.
		{baseGetHeaders, baseGetHeadersEncoded, pver, BaseEncoding, 0, io.ErrShortWrite, io.EOF},
		// Force error in block locator hash count.
		{baseGetHeaders, baseGetHeadersEncoded, pver, BaseEncoding, 4, io.ErrShortWrite, io.EOF},
		// Force error in block locator hashes.
		{baseGetHeaders, baseGetHeadersEncoded, pver, BaseEncoding, 5, io.ErrShortWrite, io.EOF},
		// Force error in stop hash.
		{baseGetHeaders, baseGetHeadersEncoded, pver, BaseEncoding, 69, io.ErrShortWrite, io.EOF},
		// Force error with greater than max block locator hashes.
		{maxGetHeaders, maxGetHeadersEncoded, pver, BaseEncoding, 7, wireErr, wireErr},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := test.in.BtcEncode(w, test.pver, test.enc)
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
		var msg MsgGetHeaders
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver, test.enc)
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
