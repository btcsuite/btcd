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

// testCmpctBlock returns a compact block and its expected wire encoding.
func testCmpctBlock() (*MsgCmpctBlock, []byte) {
	msg := NewMsgCmpctBlock(&blockOne.Header, 0x0807060504030201)
	msg.ShortIDs = append(msg.ShortIDs,
		ShortID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
		ShortID{0x11, 0x12, 0x13, 0x14, 0x15, 0x16},
	)
	msg.PrefilledTxns = append(msg.PrefilledTxns,
		&PrefilledTxn{Index: 0, Tx: blockOne.Transactions[0]},
		&PrefilledTxn{Index: 2, Tx: blockOne.Transactions[0]},
	)

	// The prefilled indexes 0 and 2 are encoded as the differential indexes
	// 0 and 1 respectively.
	wireBytes := make([]byte, 0, 371)
	wireBytes = append(wireBytes, blockOneBytes[:80]...)
	wireBytes = append(wireBytes,
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, // Nonce.
		0x02,                               // Short ID count.
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, // Short ID 0.
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, // Short ID 1.
		0x02, // Prefilled transaction count.
		0x00, // Absolute index 0.
	)
	wireBytes = append(wireBytes, blockOneBytes[81:]...)
	wireBytes = append(wireBytes, 0x01) // Index 2 relative to index 0.
	wireBytes = append(wireBytes, blockOneBytes[81:]...)

	return msg, wireBytes
}

// TestCmpctBlock tests the MsgCmpctBlock API against the latest protocol
// version.
func TestCmpctBlock(t *testing.T) {
	pver := ProtocolVersion
	enc := BaseEncoding
	msg := NewMsgCmpctBlock(&blockOne.Header, 123)

	if !reflect.DeepEqual(&msg.Header, &blockOne.Header) {
		t.Fatalf("NewMsgCmpctBlock: wrong header\n got: %s want: %s",
			spew.Sdump(&msg.Header), spew.Sdump(&blockOne.Header))
	}
	if msg.Nonce != 123 {
		t.Fatalf("NewMsgCmpctBlock: wrong nonce - got %d, want %d",
			msg.Nonce, 123)
	}

	if cmd := msg.Command(); cmd != CmdCmpctBlock {
		t.Errorf("NewMsgCmpctBlock: wrong command - got %v, want %v",
			cmd, CmdCmpctBlock)
	}

	wantPayload := uint32(10000121)
	if maxPayload := msg.MaxPayloadLength(pver); maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for protocol "+
			"version %d - got %v, want %v", pver, maxPayload,
			wantPayload)
	}

	shortID := ShortID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	if err := msg.AddShortID(shortID); err != nil {
		t.Fatalf("AddShortID: %v", err)
	}
	if !reflect.DeepEqual(msg.ShortIDs, []ShortID{shortID}) {
		t.Errorf("AddShortID: wrong short IDs - got %v, want %v",
			msg.ShortIDs, []ShortID{shortID})
	}

	prefilledTxn := &PrefilledTxn{Index: 0, Tx: blockOne.Transactions[0]}
	if err := msg.AddPrefilledTxn(prefilledTxn); err != nil {
		t.Fatalf("AddPrefilledTxn: %v", err)
	}
	if !reflect.DeepEqual(msg.PrefilledTxns, []*PrefilledTxn{prefilledTxn}) {
		t.Errorf("AddPrefilledTxn: wrong transactions - got %v, want %v",
			msg.PrefilledTxns, []*PrefilledTxn{prefilledTxn})
	}

	// Test encode with the latest protocol version.
	baseMsg, _ := testCmpctBlock()
	var buf bytes.Buffer
	if err := baseMsg.BtcEncode(&buf, pver, enc); err != nil {
		t.Errorf("encode of MsgCmpctBlock failed %v err <%v>", baseMsg,
			err)
	}

	// Test decode with the latest protocol version.
	var readMsg MsgCmpctBlock
	if err := readMsg.BtcDecode(&buf, pver, enc); err != nil {
		t.Errorf("decode of MsgCmpctBlock failed [%v] err <%v>", buf,
			err)
	}
	if !reflect.DeepEqual(baseMsg, &readMsg) {
		t.Errorf("encode/decode mismatch\n got: %s want: %s",
			spew.Sdump(&readMsg), spew.Sdump(baseMsg))
	}

	msg.ShortIDs = make([]ShortID, maxTxPerBlock)
	if err := msg.AddShortID(shortID); err == nil {
		t.Error("AddShortID succeeded with too many short IDs")
	}

	msg.PrefilledTxns = make([]*PrefilledTxn, maxTxPerBlock)
	if err := msg.AddPrefilledTxn(prefilledTxn); err == nil {
		t.Error("AddPrefilledTxn succeeded with too many transactions")
	}
}

// TestCmpctBlockProtocolVersion tests the MsgCmpctBlock API against a protocol
// version prior to ShortIdsBlocksVersion.
func TestCmpctBlockProtocolVersion(t *testing.T) {
	msg, _ := testCmpctBlock()
	var buf bytes.Buffer

	// Test encode with an old protocol version.
	if err := msg.BtcEncode(&buf, ShortIdsBlocksVersion-1,
		BaseEncoding); err == nil {
		t.Error("BtcEncode succeeded with an unsupported protocol version")
	}

	// Test decode with an old protocol version.
	if err := new(MsgCmpctBlock).BtcDecode(
		&buf, ShortIdsBlocksVersion-1, BaseEncoding,
	); err == nil {
		t.Error("BtcDecode succeeded with an unsupported protocol version")
	}
}

// TestCmpctBlockCrossProtocol tests the MsgCmpctBlock API when encoding with
// the latest protocol version and decoding with ShortIdsBlocksVersion.
func TestCmpctBlockCrossProtocol(t *testing.T) {
	msg, _ := testCmpctBlock()
	var buf bytes.Buffer

	// Encode with the latest protocol version.
	if err := msg.BtcEncode(&buf, ProtocolVersion, BaseEncoding); err != nil {
		t.Fatalf("BtcEncode: %v", err)
	}

	// Decode with the oldest supported protocol version.
	var decoded MsgCmpctBlock
	if err := decoded.BtcDecode(
		&buf, ShortIdsBlocksVersion, BaseEncoding,
	); err != nil {
		t.Fatalf("BtcDecode at ShortIdsBlocksVersion: %v", err)
	}
	if !reflect.DeepEqual(&decoded, msg) {
		t.Errorf("encode/decode mismatch\n got: %s want: %s",
			spew.Sdump(&decoded), spew.Sdump(msg))
	}
}

// TestCmpctBlockWire tests MsgCmpctBlock wire encoding and decoding.
func TestCmpctBlockWire(t *testing.T) {
	msg, wireBytes := testCmpctBlock()
	tests := []struct {
		in   *MsgCmpctBlock  // Message to encode
		out  *MsgCmpctBlock  // Expected decoded message
		buf  []byte          // Wire encoding
		pver uint32          // Protocol version for wire encoding
		enc  MessageEncoding // Message encoding format
	}{
		// Latest protocol version.
		{msg, msg, wireBytes, ProtocolVersion, BaseEncoding},

		// Protocol version ShortIdsBlocksVersion.
		{msg, msg, wireBytes, ShortIdsBlocksVersion, BaseEncoding},

		// Latest protocol version with witness encoding.
		{msg, msg, wireBytes, ProtocolVersion, WitnessEncoding},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode the message to wire format.
		var buf bytes.Buffer
		if err := test.in.BtcEncode(&buf, test.pver, test.enc); err != nil {
			t.Errorf("BtcEncode #%d: %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("BtcEncode #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode the message from wire format.
		var decoded MsgCmpctBlock
		if err := decoded.BtcDecode(
			bytes.NewReader(test.buf), test.pver, test.enc,
		); err != nil {
			t.Errorf("BtcDecode #%d: %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&decoded, test.out) {
			t.Errorf("BtcDecode #%d\n got: %s want: %s", i,
				spew.Sdump(&decoded), spew.Sdump(test.out))
		}
	}
}

// TestCmpctBlockWireErrors performs negative tests against wire encoding and
// decoding of MsgCmpctBlock to confirm error paths work correctly.
func TestCmpctBlockWireErrors(t *testing.T) {
	pver := ProtocolVersion
	pverNoCmpctBlock := ShortIdsBlocksVersion - 1
	wireErr := &MessageError{}
	msg, wireBytes := testCmpctBlock()
	tests := []struct {
		in       *MsgCmpctBlock  // Value to encode
		buf      []byte          // Wire encoding
		pver     uint32          // Protocol version for wire encoding
		enc      MessageEncoding // Message encoding format
		max      int             // Max size of fixed buffer to induce errors
		writeErr error           // Expected write error
		readErr  error           // Expected read error
	}{
		// Force error in header version.
		{msg, wireBytes, pver, BaseEncoding, 0, io.ErrShortWrite, io.EOF},
		// Force error in previous block hash.
		{msg, wireBytes, pver, BaseEncoding, 4, io.ErrShortWrite, io.EOF},
		// Force error in merkle root.
		{msg, wireBytes, pver, BaseEncoding, 36, io.ErrShortWrite, io.EOF},
		// Force error in timestamp.
		{msg, wireBytes, pver, BaseEncoding, 68, io.ErrShortWrite, io.EOF},
		// Force error in difficulty bits.
		{msg, wireBytes, pver, BaseEncoding, 72, io.ErrShortWrite, io.EOF},
		// Force error in header nonce.
		{msg, wireBytes, pver, BaseEncoding, 76, io.ErrShortWrite, io.EOF},
		// Force error in compact block nonce.
		{msg, wireBytes, pver, BaseEncoding, 80, io.ErrShortWrite, io.EOF},
		// Force error in short ID count.
		{msg, wireBytes, pver, BaseEncoding, 88, io.ErrShortWrite, io.EOF},
		// Force error in short ID.
		{msg, wireBytes, pver, BaseEncoding, 89, io.ErrShortWrite, io.EOF},
		// Force error in prefilled transaction count.
		{msg, wireBytes, pver, BaseEncoding, 101, io.ErrShortWrite, io.EOF},
		// Force error in prefilled transaction index.
		{msg, wireBytes, pver, BaseEncoding, 102, io.ErrShortWrite, io.EOF},
		// Force error in prefilled transaction.
		{msg, wireBytes, pver, BaseEncoding, 103, io.ErrShortWrite, io.EOF},
		// Force errors due to an unsupported protocol version.
		{msg, wireBytes, pverNoCmpctBlock, BaseEncoding, len(wireBytes),
			wireErr, wireErr},
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
		if _, ok := err.(*MessageError); !ok && err != test.writeErr {
			t.Errorf("BtcEncode #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// Decode from wire format.
		var decoded MsgCmpctBlock
		r := newFixedReader(test.max, test.buf)
		err = decoded.BtcDecode(r, test.pver, test.enc)
		if reflect.TypeOf(err) != reflect.TypeOf(test.readErr) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
		if _, ok := err.(*MessageError); !ok && err != test.readErr {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}

// TestCmpctBlockOverflowErrors performs tests to ensure encoding and decoding
// compact blocks that are intentionally crafted to use large values are
// handled properly.  This could otherwise potentially be used as an attack
// vector.
func TestCmpctBlockOverflowErrors(t *testing.T) {
	wireErr := &MessageError{}
	assertMessageError := func(t *testing.T, err error) {
		t.Helper()
		if reflect.TypeOf(err) != reflect.TypeOf(wireErr) {
			t.Fatalf("wrong error type - got %T (%v), want %T", err, err,
				wireErr)
		}
	}

	msg, wireBytes := testCmpctBlock()
	msg.ShortIDs = make([]ShortID, maxTxPerBlock+1)
	assertMessageError(t, msg.BtcEncode(io.Discard, ProtocolVersion,
		BaseEncoding))

	msg, _ = testCmpctBlock()
	msg.PrefilledTxns = make([]*PrefilledTxn, maxTxPerBlock+1)
	assertMessageError(t, msg.BtcEncode(io.Discard, ProtocolVersion,
		BaseEncoding))

	msg, _ = testCmpctBlock()
	msg.PrefilledTxns = []*PrefilledTxn{nil}
	assertMessageError(t, msg.BtcEncode(io.Discard, ProtocolVersion,
		BaseEncoding))

	msg.PrefilledTxns = []*PrefilledTxn{{Index: 0, Tx: nil}}
	assertMessageError(t, msg.BtcEncode(io.Discard, ProtocolVersion,
		BaseEncoding))

	msg.PrefilledTxns = []*PrefilledTxn{
		{Index: 1, Tx: blockOne.Transactions[0]},
		{Index: 1, Tx: blockOne.Transactions[0]},
	}
	assertMessageError(t, msg.BtcEncode(io.Discard, ProtocolVersion,
		BaseEncoding))

	var tooManyShortIDs bytes.Buffer
	tooManyShortIDs.Write(wireBytes[:88])
	if err := WriteVarInt(
		&tooManyShortIDs, ProtocolVersion, maxTxPerBlock+1,
	); err != nil {
		t.Fatalf("WriteVarInt: %v", err)
	}
	assertMessageError(t, new(MsgCmpctBlock).BtcDecode(
		&tooManyShortIDs, ProtocolVersion, BaseEncoding,
	))

	var tooManyPrefilled bytes.Buffer
	tooManyPrefilled.Write(wireBytes[:101])
	if err := WriteVarInt(
		&tooManyPrefilled, ProtocolVersion, maxTxPerBlock+1,
	); err != nil {
		t.Fatalf("WriteVarInt: %v", err)
	}
	assertMessageError(t, new(MsgCmpctBlock).BtcDecode(
		&tooManyPrefilled, ProtocolVersion, BaseEncoding,
	))

	// A differential index greater than uint32's maximum overflows the
	// absolute index.
	var overflowingIndex bytes.Buffer
	overflowingIndex.Write(wireBytes[:101])
	if err := WriteVarInt(&overflowingIndex, ProtocolVersion, 1); err != nil {
		t.Fatalf("WriteVarInt: %v", err)
	}
	if err := WriteVarInt(
		&overflowingIndex, ProtocolVersion, uint64(^uint32(0))+1,
	); err != nil {
		t.Fatalf("WriteVarInt: %v", err)
	}
	assertMessageError(t, new(MsgCmpctBlock).BtcDecode(
		&overflowingIndex, ProtocolVersion, BaseEncoding,
	))
}

// TestCmpctBlockDifferentialOverflow ensures prefilled transaction indexes
// that wrap uint64 are rejected.
func TestCmpctBlockDifferentialOverflow(t *testing.T) {
	_, wireBytes := testCmpctBlock()
	var payload bytes.Buffer
	payload.Write(wireBytes[:102])
	if err := WriteVarInt(&payload, ProtocolVersion, 5); err != nil {
		t.Fatalf("WriteVarInt: %v", err)
	}
	payload.Write(blockOneBytes[81:])
	if err := WriteVarInt(&payload, ProtocolVersion, math.MaxUint64); err != nil {
		t.Fatalf("WriteVarInt: %v", err)
	}

	msg := new(MsgCmpctBlock)
	err := msg.BtcDecode(&payload, ProtocolVersion,
		BaseEncoding)
	if _, ok := err.(*MessageError); !ok {
		t.Fatalf("BtcDecode: got %T (%v), want MessageError", err, err)
	}
}

// TestCmpctBlockMaximumUint32DoesNotResetEncoder ensures the maximum uint32
// index does not reset the encoder's strictly increasing index check.
func TestCmpctBlockMaximumUint32DoesNotResetEncoder(t *testing.T) {
	msg, _ := testCmpctBlock()
	msg.PrefilledTxns = []*PrefilledTxn{
		{Index: 5, Tx: blockOne.Transactions[0]},
		{Index: math.MaxUint32, Tx: blockOne.Transactions[0]},
		{Index: 3, Tx: blockOne.Transactions[0]},
	}

	err := msg.BtcEncode(io.Discard, ProtocolVersion, BaseEncoding)
	if _, ok := err.(*MessageError); !ok {
		t.Fatalf("BtcEncode: got %T (%v), want MessageError", err, err)
	}
}

// TestCmpctBlockMaximumUint32IsNotSentinel ensures the maximum uint32 index
// is not treated as the first index sentinel.
func TestCmpctBlockMaximumUint32IsNotSentinel(t *testing.T) {
	_, wireBytes := testCmpctBlock()
	var payload bytes.Buffer
	payload.Write(wireBytes[:102])
	if err := WriteVarInt(&payload, ProtocolVersion, math.MaxUint32); err != nil {
		t.Fatalf("WriteVarInt: %v", err)
	}
	payload.Write(blockOneBytes[81:])
	if err := WriteVarInt(&payload, ProtocolVersion, 0); err != nil {
		t.Fatalf("WriteVarInt: %v", err)
	}
	payload.Write(blockOneBytes[81:])

	err := new(MsgCmpctBlock).BtcDecode(&payload, ProtocolVersion,
		BaseEncoding)
	if _, ok := err.(*MessageError); !ok {
		t.Fatalf("BtcDecode: got %T (%v), want MessageError", err, err)
	}
}
