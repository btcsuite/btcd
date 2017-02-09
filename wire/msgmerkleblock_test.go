// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"crypto/rand"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/chaincfg/chainhash"
)

// TestMerkleBlock tests the MsgMerkleBlock API.
func TestMerkleBlock(t *testing.T) {
	pver := ProtocolVersion

	// Test block header.
	bh := NewBlockHeader(
		int32(pver),
		&testBlock.Header.PrevBlock,                 // PrevHash
		&testBlock.Header.MerkleRoot,                // MerkleRootHash
		&testBlock.Header.StakeRoot,                 // StakeRoot
		uint16(0x0000),                              // VoteBits
		[6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // FinalState
		uint16(0x0000),                              // Voters
		uint8(0x00),                                 // FreshStake
		uint8(0x00),                                 // Revocations
		uint32(0),                                   // Poolsize
		testBlock.Header.Bits,                       // Bits
		int64(0x0000000000000000),                   // Sbits
		uint32(1),                                   // Height
		uint32(0),                                   // Size
		testBlock.Header.Nonce,                      // Nonce
		[32]byte{},                                  // ExtraData
		uint32(0x7e1eca57),                          // StakeVersion
	)

	// Ensure the command is expected value.
	wantCmd := "merkleblock"
	msg := NewMsgMerkleBlock(bh)
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgBlock: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Num addresses (varInt) + max allowed addresses.
	wantPayload := uint32(1310720)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Ensure max payload is expected value for protocol version 3.
	wantPayload = uint32(1000000)
	maxPayload = msg.MaxPayloadLength(3)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", 3,
			maxPayload, wantPayload)
	}

	// Load maxTxPerBlock hashes
	data := make([]byte, 32)
	for i := uint64(0); i < MaxTxPerTxTree(pver); i++ {
		rand.Read(data)
		hash, err := chainhash.NewHash(data)
		if err != nil {
			t.Errorf("NewHash failed: %v\n", err)
			return
		}

		if err = msg.AddTxHash(hash); err != nil {
			t.Errorf("AddTxHash failed: %v\n", err)
			return
		}
		if err = msg.AddSTxHash(hash); err != nil {
			t.Errorf("AddSTxHash failed: %v\n", err)
			return
		}
	}

	// Add one more Tx to test failure.
	rand.Read(data)
	hash, err := chainhash.NewHash(data)
	if err != nil {
		t.Errorf("NewHash failed: %v\n", err)
		return
	}

	if err = msg.AddTxHash(hash); err == nil {
		t.Errorf("AddTxHash succeeded when it should have failed")
		return
	}

	// Add one more STx to test failure.
	rand.Read(data)
	hash, err = chainhash.NewHash(data)
	if err != nil {
		t.Errorf("NewHash failed: %v\n", err)
		return
	}

	if err = msg.AddSTxHash(hash); err == nil {
		t.Errorf("AddTxHash succeeded when it should have failed")
		return
	}

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err = msg.BtcEncode(&buf, pver)
	if err != nil {
		t.Errorf("encode of MsgMerkleBlock failed %v err <%v>", msg, err)
	}

	// Test decode with latest protocol version.
	readmsg := MsgMerkleBlock{}
	err = readmsg.BtcDecode(&buf, pver)
	if err != nil {
		t.Errorf("decode of MsgMerkleBlock failed [%v] err <%v>", buf, err)
	}

	// Force extra hash to test maxTxPerBlock.
	msg.Hashes = append(msg.Hashes, hash)
	err = msg.BtcEncode(&buf, pver)
	if err == nil {
		t.Errorf("encode of MsgMerkleBlock succeeded with too many " +
			"tx hashes when it should have failed")
		return
	}

	// Force too many flag bytes to test maxFlagsPerMerkleBlock.
	// Reset the number of hashes back to a valid value.
	msg.Hashes = msg.Hashes[len(msg.Hashes)-1:]
	msg.Flags = make([]byte, maxFlagsPerMerkleBlock(pver)+1)
	err = msg.BtcEncode(&buf, pver)
	if err == nil {
		t.Errorf("encode of MsgMerkleBlock succeeded with too many " +
			"flag bytes when it should have failed")
		return
	}
}

// TestMerkleBlockWire tests the MsgMerkleBlock wire encode and decode for
// various numbers of transaction hashes and protocol versions.
func TestMerkleBlockWire(t *testing.T) {
	tests := []struct {
		in   *MsgMerkleBlock // Message to encode
		out  *MsgMerkleBlock // Expected decoded message
		buf  []byte          // Wire encoding
		pver uint32          // Protocol version for wire encoding
	}{
		// Latest protocol version.
		{
			&testMerkleBlock, &testMerkleBlock,
			testMerkleBlockBytes, ProtocolVersion,
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
		var msg MsgMerkleBlock
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

// TestMerkleBlockWireErrors performs negative tests against wire encode and
// decode of MsgBlock to confirm error paths work correctly.
func TestMerkleBlockWireErrors(t *testing.T) {
	// Use protocol version 70001 specifically here instead of the latest
	// because the test data is using bytes encoded with that protocol
	// version.
	pver := uint32(1)

	tests := []struct {
		in       *MsgMerkleBlock // Value to encode
		buf      []byte          // Wire encoding
		pver     uint32          // Protocol version for wire encoding
		max      int             // Max size of fixed buffer to induce errors
		writeErr error           // Expected write error
		readErr  error           // Expected read error
	}{
		// Force error in version. [0]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 0,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in prev block hash. [1]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 4,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in merkle root.  [2]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 36,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in stake merkle root. [3]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 68,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in VoteBits. [4]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 100,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in FinalState. [5]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 102,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in Voters. [6]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 108,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in FreshStake. [7]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 110,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in Revocations. [8]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 111,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in poolsize. [9]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 112,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in difficulty bits. [10]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 116,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in stake difficulty bits. [11]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 120,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in height. [12]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 128,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in size. [13]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 132,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in timestamp. [14]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 136,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in header nonce. [15]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 140,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in transaction count. [16]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 180,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in num hashes. [17]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 184,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in hashes. [18]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 185,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in num flag bytes. [19]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 254,
			io.ErrShortWrite, io.EOF,
		},
		// Force error in flag bytes. [20]
		{
			&testMerkleBlock, testMerkleBlockBytes, pver, 255,
			io.ErrShortWrite, io.EOF,
		},
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
		var msg MsgMerkleBlock
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

// TestMerkleBlockOverflowErrors performs tests to ensure encoding and decoding
// merkle blocks that are intentionally crafted to use large values for the
// number of hashes and flags are handled properly.  This could otherwise
// potentially be used as an attack vector.
func TestMerkleBlockOverflowErrors(t *testing.T) {
	// Use protocol version 1 specifically here instead of the latest
	// protocol version because the test data is using bytes encoded with
	// that version.
	pver := uint32(1)

	// Create bytes for a merkle block that claims to have more than the max
	// allowed tx hashes.
	var buf bytes.Buffer
	WriteVarInt(&buf, pver, MaxTxPerTxTree(pver)+1)
	numHashesOffset := 140
	exceedMaxHashes := make([]byte, numHashesOffset)
	copy(exceedMaxHashes, testMerkleBlockBytes[:numHashesOffset])
	exceedMaxHashes = append(exceedMaxHashes, buf.Bytes()...)

	// Create bytes for a merkle block that claims to have more than the max
	// allowed flag bytes.
	buf.Reset()
	WriteVarInt(&buf, pver, uint64(maxFlagsPerMerkleBlock(pver)+1))
	numFlagBytesOffset := 210
	exceedMaxFlagBytes := make([]byte, numFlagBytesOffset)
	copy(exceedMaxFlagBytes, testMerkleBlockBytes[:numFlagBytesOffset])
	exceedMaxFlagBytes = append(exceedMaxFlagBytes, buf.Bytes()...)

	tests := []struct {
		buf  []byte // Wire encoding
		pver uint32 // Protocol version for wire encoding
		err  error  // Expected error
	}{
		// Block that claims to have more than max allowed hashes.
		{exceedMaxHashes, pver, io.ErrUnexpectedEOF},
		// Block that claims to have more than max allowed flag bytes.
		{exceedMaxFlagBytes, pver, io.ErrUnexpectedEOF},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire format.
		var msg MsgMerkleBlock
		r := bytes.NewReader(test.buf)
		err := msg.BtcDecode(r, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, reflect.TypeOf(test.err))
			continue
		}
	}
}

// testMerkleBlock is a basic normative merkle block that is used throughout the
// tests.
var testMerkleBlock = MsgMerkleBlock{
	Header: BlockHeader{
		Version: 1,
		PrevBlock: chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
			0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
			0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
			0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
			0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00,
		}),
		MerkleRoot: chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
			0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
			0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
			0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
			0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e,
		}),
		StakeRoot: chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
			0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
			0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
			0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
			0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e,
		}),
		VoteBits:     uint16(0x0000),
		FinalState:   [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Voters:       uint16(0x0000),
		FreshStake:   uint8(0x00),
		Revocations:  uint8(0x00),
		Timestamp:    time.Unix(0x4966bc61, 0), // 2009-01-08 20:54:25 -0600 CST
		Bits:         0x1d00ffff,               // 486604799
		SBits:        int64(0x0000000000000000),
		Nonce:        0x9962e301, // 2573394689
		ExtraData:    [32]byte{},
		StakeVersion: uint32(0x7e1eca57),
	},
	Transactions: 1,
	Hashes: []*chainhash.Hash{
		(*chainhash.Hash)(&[chainhash.HashSize]byte{ // Make go vet happy.
			0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
			0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
			0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
			0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e,
		}),
	},
	STransactions: 1,
	SHashes: []*chainhash.Hash{
		(*chainhash.Hash)(&[chainhash.HashSize]byte{ // Make go vet happy.
			0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
			0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
			0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
			0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e,
		}),
	},
	Flags: []byte{0x80},
}

// testMerkleBlockBytes is the serialized bytes for the above test merkle block.
var testMerkleBlockBytes = []byte{
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
	0x01, 0xe3, 0x62, 0x99, // Nonce
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ExtraData
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x57, 0xca, 0x1e, 0x7e, // StakeVersion
	0x01, 0x00, 0x00, 0x00, // TxnCount (regular) [180]
	0x01, // Num hashes (regular) [184]
	0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
	0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
	0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
	0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e, // Hash [185]
	0x01, 0x00, 0x00, 0x00, // TxnCount (stake) [217]
	0x01, // Num hashes (stake) [221]
	0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
	0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
	0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
	0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e, // Hash [222]
	0x01, // Num flag bytes [254]
	0x80, // Flags [255]
}
