// Copyright (c) 2026 The btcsuite developers
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

// TestBlockTxn tests the MsgBlockTxn API against the latest protocol version.
func TestBlockTxn(t *testing.T) {
	pver := ProtocolVersion
	enc := BaseEncoding
	blockHash := mainNetGenesisHash
	msg := NewMsgBlockTxn(&blockHash)

	if !reflect.DeepEqual(&msg.BlockHash, &blockHash) {
		t.Errorf("NewMsgBlockTxn: wrong block hash - got %v, want %v",
			msg.BlockHash, blockHash)
	}

	wantCmd := "blocktxn"
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgBlockTxn: wrong command - got %v, want %v",
			cmd, wantCmd)
	}

	wantPayload := uint32(4000041)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	tx := blockOne.Transactions[0]
	if err := msg.AddTransaction(tx); err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}
	if !reflect.DeepEqual(msg.Transactions, []*MsgTx{tx}) {
		t.Errorf("AddTransaction: wrong transactions - got %v, want %v",
			msg.Transactions, []*MsgTx{tx})
	}

	// Test encode with the latest protocol version.
	var buf bytes.Buffer
	if err := blockTxn.BtcEncode(&buf, pver, enc); err != nil {
		t.Errorf("encode of MsgBlockTxn failed %v err <%v>", blockTxn,
			err)
	}

	// Test decode with the latest protocol version.
	var readMsg MsgBlockTxn
	if err := readMsg.BtcDecode(&buf, pver, enc); err != nil {
		t.Errorf("decode of MsgBlockTxn failed [%v] err <%v>", buf, err)
	}
	if !reflect.DeepEqual(&blockTxn, &readMsg) {
		t.Errorf("encode/decode mismatch\n got: %s want: %s",
			spew.Sdump(&readMsg), spew.Sdump(&blockTxn))
	}

	msg.Transactions = make([]*MsgTx, maxTxPerBlock)
	if err := msg.AddTransaction(tx); err == nil {
		t.Error("AddTransaction succeeded with too many transactions")
	}
}

// TestBlockTxnProtocolVersion tests the MsgBlockTxn API against a protocol
// version prior to ShortIdsBlocksVersion.
func TestBlockTxnProtocolVersion(t *testing.T) {
	pver := ShortIdsBlocksVersion - 1
	enc := BaseEncoding
	msg := blockTxn

	// Test encode with an old protocol version.
	var buf bytes.Buffer
	if err := msg.BtcEncode(&buf, pver, enc); err == nil {
		t.Error("encode of MsgBlockTxn succeeded when it should have failed")
	}

	// Test decode with an old protocol version.
	var readMsg MsgBlockTxn
	if err := readMsg.BtcDecode(&buf, pver, enc); err == nil {
		t.Error("decode of MsgBlockTxn succeeded when it should have failed")
	}
}

// TestBlockTxnCrossProtocol tests the MsgBlockTxn API when encoding with the
// latest protocol version and decoding with ShortIdsBlocksVersion.
func TestBlockTxnCrossProtocol(t *testing.T) {
	msg := blockTxn

	// Encode with the latest protocol version.
	var buf bytes.Buffer
	if err := msg.BtcEncode(&buf, ProtocolVersion, BaseEncoding); err != nil {
		t.Errorf("encode of MsgBlockTxn failed %v err <%v>", msg, err)
	}

	// Decode with the oldest supported protocol version.
	var readMsg MsgBlockTxn
	if err := readMsg.BtcDecode(
		&buf, ShortIdsBlocksVersion, BaseEncoding,
	); err != nil {
		t.Errorf("decode of MsgBlockTxn failed [%v] err <%v>", buf, err)
	}
	if !reflect.DeepEqual(&msg, &readMsg) {
		t.Errorf("encode/decode mismatch\n got: %s want: %s",
			spew.Sdump(&readMsg), spew.Sdump(&msg))
	}
}

// TestBlockTxnWire tests the MsgBlockTxn wire encode and decode for various
// protocol versions and encoding formats.
func TestBlockTxnWire(t *testing.T) {
	tests := []struct {
		in   *MsgBlockTxn    // Message to encode
		out  *MsgBlockTxn    // Expected decoded message
		buf  []byte          // Wire encoding
		pver uint32          // Protocol version for wire encoding
		enc  MessageEncoding // Message encoding format
	}{
		// Latest protocol version.
		{
			&blockTxn, &blockTxn, blockTxnBytes,
			ProtocolVersion, BaseEncoding,
		},

		// Protocol version ShortIdsBlocksVersion.
		{
			&blockTxn, &blockTxn, blockTxnBytes,
			ShortIdsBlocksVersion, BaseEncoding,
		},

		// Latest protocol version with witness encoding.
		{
			&blockTxn, &blockTxn, blockTxnBytes,
			ProtocolVersion, WitnessEncoding,
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
		var msg MsgBlockTxn
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

// TestBlockTxnWireErrors performs negative tests against wire encode and
// decode of MsgBlockTxn to confirm error paths work correctly.
func TestBlockTxnWireErrors(t *testing.T) {
	pver := ProtocolVersion
	pverNoBlockTxn := ShortIdsBlocksVersion - 1
	wireErr := &MessageError{}

	tests := []struct {
		in       *MsgBlockTxn // Value to encode
		buf      []byte       // Wire encoding
		pver     uint32       // Protocol version for wire encoding
		max      int          // Max size of fixed buffer to induce errors
		writeErr error        // Expected write error
		readErr  error        // Expected read error
	}{
		// Force error in block hash.
		{&blockTxn, blockTxnBytes, pver, 0,
			io.ErrShortWrite, io.EOF},

		// Force error in transaction count.
		{&blockTxn, blockTxnBytes, pver, 32,
			io.ErrShortWrite, io.EOF},

		// Force error in transaction.
		{&blockTxn, blockTxnBytes, pver, 33,
			io.ErrShortWrite, io.EOF},

		// Force errors due to an unsupported protocol version.
		{&blockTxn, blockTxnBytes, pverNoBlockTxn, len(blockTxnBytes),
			wireErr, wireErr},
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
		var msg MsgBlockTxn
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

// TestBlockTxnOverflowErrors performs tests to ensure encoding and decoding
// messages that are intentionally crafted to use too many transactions are
// handled properly.
func TestBlockTxnOverflowErrors(t *testing.T) {
	pver := ProtocolVersion
	wireErr := &MessageError{}

	tooManyTransactions := blockTxn
	tooManyTransactions.Transactions = make([]*MsgTx, maxTxPerBlock+1)

	nilTransaction := blockTxn
	nilTransaction.Transactions = []*MsgTx{nil}

	// Create bytes for a message that claims to have more than the maximum
	// allowed number of transactions.
	var buf bytes.Buffer
	buf.Write(blockTxnBytes[:32])
	if err := WriteVarInt(&buf, pver, maxTxPerBlock+1); err != nil {
		t.Fatalf("WriteVarInt: %v", err)
	}
	exceedMaxTransactions := append([]byte(nil), buf.Bytes()...)

	encodeTests := []struct {
		in  *MsgBlockTxn // Message to encode
		err error        // Expected error
	}{
		// Message with more than the maximum allowed transactions.
		{&tooManyTransactions, wireErr},

		// Message with a nil transaction.
		{&nilTransaction, wireErr},
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
		// Message that claims to have more than the maximum allowed
		// transactions.
		{exceedMaxTransactions, wireErr},
	}

	t.Logf("Running %d decode tests", len(decodeTests))
	for i, test := range decodeTests {
		var msg MsgBlockTxn
		err := msg.BtcDecode(bytes.NewReader(test.buf), pver, BaseEncoding)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, reflect.TypeOf(test.err))
		}
	}
}

// blockTxn is a blocktxn message with the coinbase transaction from block one.
var blockTxn = MsgBlockTxn{
	BlockHash:    mainNetGenesisHash,
	Transactions: []*MsgTx{blockOne.Transactions[0]},
}

// blockTxnBytes is the wire encoded blockTxn message.
var blockTxnBytes = append([]byte{
	0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
	0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
	0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
	0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, // Block hash.
	0x01, // Transaction count.
}, blockOneBytes[81:]...)
