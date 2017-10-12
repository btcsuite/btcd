// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
)

// TestHeaders tests the MsgHeaders API.
func TestHeaders(t *testing.T) {
	pver := uint32(1)

	// Ensure the command is expected value.
	wantCmd := "headers"
	msg := NewMsgHeaders()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgHeaders: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Num headers (varInt) + max allowed headers (header length + 1 byte
	// for the number of transactions which is always 0).
	wantPayload := uint32(362009)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Ensure headers are added properly.
	bh := &testBlock.Header
	msg.AddBlockHeader(bh)
	if !reflect.DeepEqual(msg.Headers[0], bh) {
		t.Errorf("AddHeader: wrong header - got %v, want %v",
			spew.Sdump(msg.Headers),
			spew.Sdump(bh))
	}

	// Ensure adding more than the max allowed headers per message returns
	// error.
	var err error
	for i := 0; i < MaxBlockHeadersPerMsg+1; i++ {
		err = msg.AddBlockHeader(bh)
	}
	if reflect.TypeOf(err) != reflect.TypeOf(&MessageError{}) {
		t.Errorf("AddBlockHeader: expected error on too many headers " +
			"not received")
	}
}

// TestHeadersWire tests the MsgHeaders wire encode and decode for various
// numbers of headers and protocol versions.
func TestHeadersWire(t *testing.T) {
	bh := NewBlockHeader(
		testBlock.Header.Version,                    // Version
		&mainNetGenesisHash,                         // PrevHash
		&testBlock.Header.MerkleRoot,                // MerkleRootHash
		&testBlock.Header.StakeRoot,                 // StakeRoot
		uint16(0x0000),                              // VoteBits
		[6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // FinalState
		uint16(0x0000),                              // Voters
		uint8(0x00),                                 // FreshStake
		uint8(0x00),                                 // Revocations
		uint32(0),                                   // Poolsize
		uint32(0x1d00ffff),                          // Bits
		int64(0x0000000000000000),                   // Sbits
		uint32(0),                                   // Height
		uint32(0),                                   // Size
		uint32(0x01010101),                          // Nonce
		[32]byte{},                                  // ExtraData
		uint32(0xba5eba11),                          //StakeVersion
	)
	bh.Timestamp = time.Unix(0x4966bc61, 0)

	// Empty headers message.
	noHeaders := NewMsgHeaders()
	noHeadersEncoded := []byte{
		0x00, // Varint for number of headers
	}

	// Headers message with one header.
	oneHeader := NewMsgHeaders()
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
		0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
		0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
		0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
		0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e, // StakeRoot
		0x00, 0x00, // VoteBits
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // FinalState
		0x00, 0x00, // Voters
		0x00,                   // FreshStake
		0x00,                   // Revocations
		0x00, 0x00, 0x00, 0x00, // Poolsize
		0xff, 0xff, 0x00, 0x1d, // Bits
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SBits
		0x00, 0x00, 0x00, 0x00, // Height
		0x00, 0x00, 0x00, 0x00, // Size
		0x61, 0xbc, 0x66, 0x49, // Timestamp
		0x01, 0x01, 0x01, 0x01, // Nonce
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ExtraData
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x11, 0xba, 0x5e, 0xba, // StakeVersion
		0x00, // TxnCount (0 for headers message)
	}

	tests := []struct {
		in   *MsgHeaders // Message to encode
		out  *MsgHeaders // Expected decoded message
		buf  []byte      // Wire encoding
		pver uint32      // Protocol version for wire encoding
	}{
		// Latest protocol version with no headers.
		{
			noHeaders,
			noHeaders,
			noHeadersEncoded,
			ProtocolVersion,
		},

		// Latest protocol version with one header.
		{
			oneHeader,
			oneHeader,
			oneHeaderEncoded,
			ProtocolVersion,
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
		var msg MsgHeaders
		rbuf := bytes.NewReader(test.buf)
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

// TestHeadersWireErrors performs negative tests against wire encode and decode
// of MsgHeaders to confirm error paths work correctly.
func TestHeadersWireErrors(t *testing.T) {
	pver := ProtocolVersion
	wireErr := &MessageError{}

	hash := mainNetGenesisHash
	merkleHash := testBlock.Header.MerkleRoot
	bits := uint32(0x1d00ffff)
	nonce := uint32(0x9962e301)
	bh := NewBlockHeader(
		int32(pver),                                 // Verision
		&hash,                                       // PrevHash
		&merkleHash,                                 // MerkleRootHash
		&merkleHash,                                 // StakeRoot
		uint16(0x0000),                              // VoteBits
		[6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // FinalState
		uint16(0x0000),                              // Voters
		uint8(0x00),                                 // FreshStake
		uint8(0x00),                                 // Revocations
		uint32(0),                                   // Poolsize
		bits,                                        // Bits
		int64(0x0000000000000000), // Sbits
		uint32(1),                 // Height
		uint32(0),                 // Size
		nonce,                     // Nonce
		[32]byte{},                // ExtraData
		uint32(0xca55e77e),        //StakeVersion
	)

	bh.Version = testBlock.Header.Version
	bh.Timestamp = testBlock.Header.Timestamp

	// Headers message with one header.
	oneHeader := NewMsgHeaders()
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
		0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
		0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
		0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
		0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e, // StakeRoot
		0x00, 0x00, // VoteBits
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // FinalState
		0x00, 0x00, // Voters
		0x00,                   // FreshStake
		0x00,                   // Revocations
		0x00, 0x00, 0x00, 0x00, // Poolsize
		0x61, 0xbc, 0x66, 0x49, // Timestamp
		0xff, 0xff, 0x00, 0x1d, // Bits
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SBits
		0x01, 0xe3, 0x62, 0x99, // Nonce
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ExtraData
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x7e, 0xe7, 0x55, 0xca, // StakeVersion
		0x00, // TxnCount (0 for headers message)
	}

	// Message that forces an error by having more than the max allowed
	// headers.
	maxHeaders := NewMsgHeaders()
	for i := 0; i < MaxBlockHeadersPerMsg; i++ {
		maxHeaders.AddBlockHeader(bh)
	}
	maxHeaders.Headers = append(maxHeaders.Headers, bh)
	maxHeadersEncoded := []byte{
		0xfd, 0xd1, 0x07, // Varint for number of addresses (2001)7D1
	}

	// Intentionally invalid block header that has a transaction count used
	// to force errors.
	bhTrans := NewBlockHeader(
		int32(0),                                    // Verision
		&hash,                                       // PrevHash
		&merkleHash,                                 // MerkleRootHash
		&merkleHash,                                 // StakeRoot
		uint16(0x0000),                              // VoteBits
		[6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // FinalState
		uint16(0x0000),                              // Voters
		uint8(0x00),                                 // FreshStake
		uint8(0x00),                                 // Revocations
		uint32(0),                                   // Poolsize
		bits,                                        // Bits
		int64(0x0000000000000000), // Sbits
		uint32(1),                 // Height
		uint32(0),                 // Size
		nonce,                     // Nonce
		[32]byte{},                // ExtraData
		uint32(0xf01dab1e),        // StakeVersion
	)
	bhTrans.Version = testBlock.Header.Version
	bhTrans.Timestamp = testBlock.Header.Timestamp

	transHeader := NewMsgHeaders()
	transHeader.AddBlockHeader(bhTrans)
	transHeaderEncoded := []byte{
		0x01,                   // VarInt for number of headers.
		0x01, 0x00, 0x00, 0x00, // Version 1
		0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
		0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
		0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
		0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, // PrevBlock
		0x3b, 0xa3, 0xed, 0xfd, 0x7a, 0x7b, 0x12, 0xb2,
		0x7a, 0xc7, 0x2c, 0x3e, 0x67, 0x76, 0x8f, 0x61,
		0x7f, 0xc8, 0x1b, 0xc3, 0x88, 0x8a, 0x51, 0x32,
		0x3a, 0x9f, 0xb8, 0xaa, 0x4b, 0x1e, 0x5e, 0x4a, // MerkleRoot
		0x3b, 0xa3, 0xed, 0xfd, 0x7a, 0x7b, 0x12, 0xb2,
		0x7a, 0xc7, 0x2c, 0x3e, 0x67, 0x76, 0x8f, 0x61,
		0x7f, 0xc8, 0x1b, 0xc3, 0x88, 0x8a, 0x51, 0x32,
		0x3a, 0x9f, 0xb8, 0xaa, 0x4b, 0x1e, 0x5e, 0x4a, // StakeRoot
		0x00, 0x00, // VoteBits
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // FinalState
		0x00, 0x00, // Voters
		0x00,                   // FreshStake
		0x00,                   // Revocations
		0x00, 0x00, 0x00, 0x00, // Poolsize
		0x61, 0xbc, 0x66, 0x49, // Timestamp
		0xff, 0xff, 0x00, 0x1d, // Bits
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SBits
		0x01, 0xe3, 0x62, 0x99, // Nonce
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ExtraData
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x1e, 0xab, 0x1d, 0xf0, // StakeVersion
		0x01, // TxnCount (should be 0 for headers message, but 1 to force error)
	}

	tests := []struct {
		in       *MsgHeaders // Value to encode
		buf      []byte      // Wire encoding
		pver     uint32      // Protocol version for wire encoding
		max      int         // Max size of fixed buffer to induce errors
		writeErr error       // Expected write error
		readErr  error       // Expected read error
	}{
		// Latest protocol version with intentional read/write errors.
		// Force error in header count.
		{oneHeader, oneHeaderEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in block header.
		{oneHeader, oneHeaderEncoded, pver, 5, io.ErrShortWrite, io.EOF},
		// Force error with greater than max headers.
		{maxHeaders, maxHeadersEncoded, pver, 3, wireErr, wireErr},
		// Force error with number of transactions.
		{transHeader, transHeaderEncoded, pver, 181, io.ErrShortWrite, io.EOF},
		// Force error with included transactions.
		{transHeader, transHeaderEncoded, pver, len(transHeaderEncoded), io.ErrShortWrite, io.ErrUnexpectedEOF},
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
		var msg MsgHeaders
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.readErr) {
			spew.Dump(test)
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
