// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"math"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// TestGetBlockTxn tests the MsgGetBlockTxn API against the latest protocol
// version.
func TestGetBlockTxn(t *testing.T) {
	pver := ProtocolVersion
	enc := BaseEncoding
	blockHash := mainNetGenesisHash
	msg := NewMsgGetBlockTxn(&blockHash)

	if !reflect.DeepEqual(&msg.BlockHash, &blockHash) {
		t.Errorf("NewMsgGetBlockTxn: wrong block hash - got %v, want %v",
			msg.BlockHash, blockHash)
	}

	wantCmd := "getblocktxn"
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgGetBlockTxn: wrong command - got %v, want %v",
			cmd, wantCmd)
	}

	wantPayload := uint32(3600050)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	index := uint32(1)
	if err := msg.AddIndex(index); err != nil {
		t.Fatalf("AddIndex: %v", err)
	}
	if !reflect.DeepEqual(msg.Indexes, []uint32{index}) {
		t.Errorf("AddIndex: wrong indexes - got %v, want %v",
			msg.Indexes, []uint32{index})
	}

	// Test encode with the latest protocol version.
	var buf bytes.Buffer
	if err := getBlockTxn.BtcEncode(&buf, pver, enc); err != nil {
		t.Errorf("encode of MsgGetBlockTxn failed %v err <%v>",
			getBlockTxn, err)
	}

	// Test decode with the latest protocol version.
	var readMsg MsgGetBlockTxn
	if err := readMsg.BtcDecode(&buf, pver, enc); err != nil {
		t.Errorf("decode of MsgGetBlockTxn failed [%v] err <%v>", buf,
			err)
	}
	if !reflect.DeepEqual(&getBlockTxn, &readMsg) {
		t.Errorf("encode/decode mismatch\n got: %s want: %s",
			spew.Sdump(&readMsg), spew.Sdump(&getBlockTxn))
	}

	msg.Indexes = make([]uint32, maxTxPerBlock)
	if err := msg.AddIndex(index); err == nil {
		t.Error("AddIndex succeeded with too many indexes")
	}
}

// TestGetBlockTxnProtocolVersion tests the MsgGetBlockTxn API against a
// protocol version prior to ShortIdsBlocksVersion.
func TestGetBlockTxnProtocolVersion(t *testing.T) {
	pver := ShortIdsBlocksVersion - 1
	enc := BaseEncoding
	msg := getBlockTxn

	// Test encode with an old protocol version.
	var buf bytes.Buffer
	if err := msg.BtcEncode(&buf, pver, enc); err == nil {
		t.Error("encode of MsgGetBlockTxn succeeded when it should have " +
			"failed")
	}

	// Test decode with an old protocol version.
	var readMsg MsgGetBlockTxn
	if err := readMsg.BtcDecode(&buf, pver, enc); err == nil {
		t.Error("decode of MsgGetBlockTxn succeeded when it should have " +
			"failed")
	}
}

// TestGetBlockTxnCrossProtocol tests the MsgGetBlockTxn API when encoding with
// the latest protocol version and decoding with ShortIdsBlocksVersion.
func TestGetBlockTxnCrossProtocol(t *testing.T) {
	msg := getBlockTxn

	// Encode with the latest protocol version.
	var buf bytes.Buffer
	if err := msg.BtcEncode(&buf, ProtocolVersion, BaseEncoding); err != nil {
		t.Errorf("encode of MsgGetBlockTxn failed %v err <%v>", msg, err)
	}

	// Decode with the oldest supported protocol version.
	var readMsg MsgGetBlockTxn
	if err := readMsg.BtcDecode(
		&buf, ShortIdsBlocksVersion, BaseEncoding,
	); err != nil {
		t.Errorf("decode of MsgGetBlockTxn failed [%v] err <%v>", buf,
			err)
	}
	if !reflect.DeepEqual(&msg, &readMsg) {
		t.Errorf("encode/decode mismatch\n got: %s want: %s",
			spew.Sdump(&readMsg), spew.Sdump(&msg))
	}
}

// TestGetBlockTxnWire tests the MsgGetBlockTxn wire encode and decode for
// various protocol versions.
func TestGetBlockTxnWire(t *testing.T) {
	tests := []struct {
		in   *MsgGetBlockTxn // Message to encode
		out  *MsgGetBlockTxn // Expected decoded message
		buf  []byte          // Wire encoding
		pver uint32          // Protocol version for wire encoding
		enc  MessageEncoding // Message encoding format
	}{
		// Latest protocol version.
		{
			&getBlockTxn, &getBlockTxn, getBlockTxnBytes,
			ProtocolVersion, BaseEncoding,
		},

		// Protocol version ShortIdsBlocksVersion.
		{
			&getBlockTxn, &getBlockTxn, getBlockTxnBytes,
			ShortIdsBlocksVersion, BaseEncoding,
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
		var msg MsgGetBlockTxn
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

// TestGetBlockTxnWireErrors performs negative tests against wire encode and
// decode of MsgGetBlockTxn to confirm error paths work correctly.
func TestGetBlockTxnWireErrors(t *testing.T) {
	pver := ProtocolVersion
	pverNoGetBlockTxn := ShortIdsBlocksVersion - 1
	wireErr := &MessageError{}

	tests := []struct {
		in       *MsgGetBlockTxn // Value to encode
		buf      []byte          // Wire encoding
		pver     uint32          // Protocol version for wire encoding
		max      int             // Max size of fixed buffer to induce errors
		writeErr error           // Expected write error
		readErr  error           // Expected read error
	}{
		// Force error in block hash.
		{&getBlockTxn, getBlockTxnBytes, pver, 0,
			io.ErrShortWrite, io.EOF},

		// Force error in index count.
		{&getBlockTxn, getBlockTxnBytes, pver, 32,
			io.ErrShortWrite, io.EOF},

		// Force error in indexes.
		{&getBlockTxn, getBlockTxnBytes, pver, 33,
			io.ErrShortWrite, io.EOF},

		// Force errors due to an unsupported protocol version.
		{&getBlockTxn, getBlockTxnBytes, pverNoGetBlockTxn,
			len(getBlockTxnBytes), wireErr, wireErr},
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
		var msg MsgGetBlockTxn
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

// TestGetBlockTxnOverflowErrors performs tests to ensure encoding and decoding
// messages that are intentionally crafted to use large or invalid indexes are
// handled properly.
func TestGetBlockTxnOverflowErrors(t *testing.T) {
	pver := ProtocolVersion
	wireErr := &MessageError{}

	tooManyIndexes := getBlockTxn
	tooManyIndexes.Indexes = make([]uint32, maxTxPerBlock+1)

	duplicateIndexes := getBlockTxn
	duplicateIndexes.Indexes = []uint32{1, 1}

	// Create bytes for a message that claims to have more than the maximum
	// allowed number of indexes.
	var buf bytes.Buffer
	buf.Write(getBlockTxnBytes[:32])
	if err := WriteVarInt(&buf, pver, maxTxPerBlock+1); err != nil {
		t.Fatalf("WriteVarInt: %v", err)
	}
	exceedMaxIndexes := append([]byte(nil), buf.Bytes()...)

	// Create bytes with a differential index greater than uint32's maximum.
	buf.Reset()
	buf.Write(getBlockTxnBytes[:32])
	if err := WriteVarInt(&buf, pver, 1); err != nil {
		t.Fatalf("WriteVarInt: %v", err)
	}
	if err := WriteVarInt(&buf, pver, uint64(^uint32(0))+1); err != nil {
		t.Fatalf("WriteVarInt: %v", err)
	}
	overflowingIndex := append([]byte(nil), buf.Bytes()...)

	encodeTests := []struct {
		in  *MsgGetBlockTxn // Message to encode
		err error           // Expected error
	}{
		// Message with more than the maximum allowed indexes.
		{&tooManyIndexes, wireErr},

		// Message with indexes that are not strictly increasing.
		{&duplicateIndexes, wireErr},
	}

	t.Logf("Running %d encode tests", len(encodeTests))
	for i, test := range encodeTests {
		err := test.in.BtcEncode(io.Discard, pver, BaseEncoding)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("BtcEncode #%d wrong error got: %v, want: %v",
				i, err, reflect.TypeOf(test.err))
		}
	}

	decodeTests := []struct {
		buf []byte // Wire encoding
		err error  // Expected error
	}{
		// Message that claims to have more than the maximum allowed indexes.
		{exceedMaxIndexes, wireErr},

		// Message with an index that overflows uint32.
		{overflowingIndex, wireErr},
	}

	t.Logf("Running %d decode tests", len(decodeTests))
	for i, test := range decodeTests {
		var msg MsgGetBlockTxn
		err := msg.BtcDecode(bytes.NewReader(test.buf), pver, BaseEncoding)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, reflect.TypeOf(test.err))
		}
	}
}

// TestGetBlockTxnDifferentialOverflow ensures differential indexes that wrap
// uint64 are rejected.
func TestGetBlockTxnDifferentialOverflow(t *testing.T) {
	var payload bytes.Buffer
	payload.Write(make([]byte, 32))
	for _, value := range []uint64{2, 5, math.MaxUint64} {
		if err := WriteVarInt(&payload, ProtocolVersion, value); err != nil {
			t.Fatalf("WriteVarInt: %v", err)
		}
	}

	err := new(MsgGetBlockTxn).BtcDecode(&payload, ProtocolVersion,
		BaseEncoding)
	if _, ok := err.(*MessageError); !ok {
		t.Fatalf("BtcDecode: got %T (%v), want MessageError", err, err)
	}
}

// TestGetBlockTxnMaximumUint32DoesNotResetEncoder ensures the maximum uint32
// index does not reset the encoder's strictly increasing index check.
func TestGetBlockTxnMaximumUint32DoesNotResetEncoder(t *testing.T) {
	msg := NewMsgGetBlockTxn(&mainNetGenesisHash)
	msg.Indexes = []uint32{5, math.MaxUint32, 3}

	err := msg.BtcEncode(io.Discard, ProtocolVersion, BaseEncoding)
	if _, ok := err.(*MessageError); !ok {
		t.Fatalf("BtcEncode: got %T (%v), want MessageError", err, err)
	}
}

// TestGetBlockTxnMaximumUint32IsNotSentinel ensures the maximum uint32 index
// is not treated as the first index sentinel.
func TestGetBlockTxnMaximumUint32IsNotSentinel(t *testing.T) {
	var payload bytes.Buffer
	payload.Write(make([]byte, 32))
	for _, value := range []uint64{2, math.MaxUint32, 0} {
		if err := WriteVarInt(&payload, ProtocolVersion, value); err != nil {
			t.Fatalf("WriteVarInt: %v", err)
		}
	}

	err := new(MsgGetBlockTxn).BtcDecode(&payload, ProtocolVersion,
		BaseEncoding)
	if _, ok := err.(*MessageError); !ok {
		t.Fatalf("BtcDecode: got %T (%v), want MessageError", err, err)
	}
}

// getBlockTxn is a getblocktxn message that requests four transactions from
// the main network genesis block.
var getBlockTxn = MsgGetBlockTxn{
	BlockHash: mainNetGenesisHash,
	Indexes:   []uint32{0, 1, 3, 300},
}

// getBlockTxnBytes is the wire encoded getBlockTxn message.  The absolute
// indexes 0, 1, 3, and 300 are encoded as 0, 0, 1, and 296 respectively.
var getBlockTxnBytes = []byte{
	0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
	0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
	0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
	0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, // Block hash.
	0x04,             // Index count.
	0x00,             // Absolute index 0.
	0x00,             // Index 1 relative to index 0.
	0x01,             // Index 3 relative to index 1.
	0xfd, 0x28, 0x01, // Index 300 relative to index 3.
}
