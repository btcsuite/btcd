// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire_test

import (
	"bytes"
	"github.com/conformal/btcwire"
	"github.com/davecgh/go-spew/spew"
	"reflect"
	"testing"
)

// TestHeaders tests the MsgHeaders API.
func TestHeaders(t *testing.T) {
	pver := uint32(60002)

	// Ensure the command is expected value.
	wantCmd := "headers"
	msg := btcwire.NewMsgHeaders()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgHeaders: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Num headers (varInt) + max allowed headers.
	wantPayload := uint32(178009)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Ensure headers are added properly.
	bh := &blockOne.Header
	msg.AddBlockHeader(bh)
	if !reflect.DeepEqual(msg.Headers[0], bh) {
		t.Errorf("AddHeader: wrong header - got %v, want %v",
			spew.Sdump(msg.Headers),
			spew.Sdump(bh))
	}

	// Ensure adding more than the max allowed headers per message returns
	// error.
	var err error
	for i := 0; i < btcwire.MaxBlockHeadersPerMsg+1; i++ {
		err = msg.AddBlockHeader(bh)
	}
	// TODO(davec): Check for actual error.
	if err == nil {
		t.Errorf("AddBlockHeader: expected error on too many headers " +
			"not received")
	}

	return
}

// TestHeadersWire tests the MsgHeaders wire encode and decode for various
// numbers of headers and protocol versions.
func TestHeadersWire(t *testing.T) {
	hash := btcwire.GenesisHash
	merkleHash := blockOne.Header.MerkleRoot
	bits := uint32(0x1d00ffff)
	nonce := uint32(0x9962e301)
	bh := btcwire.NewBlockHeader(&hash, &merkleHash, bits, nonce)
	bh.Version = blockOne.Header.Version
	bh.Timestamp = blockOne.Header.Timestamp

	// Empty headers message.
	noHeaders := btcwire.NewMsgHeaders()
	noHeadersEncoded := []byte{
		0x00, // Varint for number of headers
	}

	// Headers message with one header.
	oneHeader := btcwire.NewMsgHeaders()
	oneHeader.AddBlockHeader(bh)
	oneHeaderEncoded := []byte{
		0x01,                   // VarInt for number of headers.
		0x01, 0x00, 0x00, 0x00, // Version 1
		0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
		0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
		0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
		0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, // PrevBlock
		0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
		0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
		0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
		0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e, // MerkleRoot
		0x61, 0xbc, 0x66, 0x49, // Timestamp
		0xff, 0xff, 0x00, 0x1d, // Bits
		0x01, 0xe3, 0x62, 0x99, // Nonce
		0x00, // TxnCount (0 for headers message)
	}

	tests := []struct {
		in   *btcwire.MsgHeaders // Message to encode
		out  *btcwire.MsgHeaders // Expected decoded message
		buf  []byte              // Wire encoding
		pver uint32              // Protocol version for wire encoding
	}{
		// Latest protocol version with no headers.
		{
			noHeaders,
			noHeaders,
			noHeadersEncoded,
			btcwire.ProtocolVersion,
		},

		// Latest protocol version with one header.
		{
			oneHeader,
			oneHeader,
			oneHeaderEncoded,
			btcwire.ProtocolVersion,
		},

		// Protocol version BIP0035Version with no headers.
		{
			noHeaders,
			noHeaders,
			noHeadersEncoded,
			btcwire.BIP0035Version,
		},

		// Protocol version BIP0035Version with one header.
		{
			oneHeader,
			oneHeader,
			oneHeaderEncoded,
			btcwire.BIP0035Version,
		},

		// Protocol version BIP0031Version with no headers.
		{
			noHeaders,
			noHeaders,
			noHeadersEncoded,
			btcwire.BIP0031Version,
		},

		// Protocol version BIP0031Version with one header.
		{
			oneHeader,
			oneHeader,
			oneHeaderEncoded,
			btcwire.BIP0031Version,
		},
		// Protocol version NetAddressTimeVersion with no headers.
		{
			noHeaders,
			noHeaders,
			noHeadersEncoded,
			btcwire.NetAddressTimeVersion,
		},

		// Protocol version NetAddressTimeVersion with one header.
		{
			oneHeader,
			oneHeader,
			oneHeaderEncoded,
			btcwire.NetAddressTimeVersion,
		},

		// Protocol version MultipleAddressVersion with no headers.
		{
			noHeaders,
			noHeaders,
			noHeadersEncoded,
			btcwire.MultipleAddressVersion,
		},

		// Protocol version MultipleAddressVersion with one header.
		{
			oneHeader,
			oneHeader,
			oneHeaderEncoded,
			btcwire.MultipleAddressVersion,
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
		var msg btcwire.MsgHeaders
		rbuf := bytes.NewBuffer(test.buf)
		err = msg.BtcDecode(rbuf, test.pver)
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
