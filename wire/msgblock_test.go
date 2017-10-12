// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
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
	"github.com/decred/dcrd/chaincfg/chainhash"
)

// TestBlock tests the MsgBlock API.
func TestBlock(t *testing.T) {
	pver := ProtocolVersion

	// Test block header.
	bh := NewBlockHeader(
		int32(pver),                                 // Version
		&testBlock.Header.PrevBlock,                 // PrevHash
		&testBlock.Header.MerkleRoot,                // MerkleRoot
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
		uint32(1),                                   // Size
		testBlock.Header.Nonce,                      // Nonce
		[32]byte{},                                  // ExtraData
		uint32(0x5ca1ab1e),                          // StakeVersion
	)

	// Ensure the command is expected value.
	wantCmd := "block"
	msg := NewMsgBlock(bh)
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

	// Ensure we get the same block header data back out.
	if !reflect.DeepEqual(&msg.Header, bh) {
		t.Errorf("NewMsgBlock: wrong block header - got %v, want %v",
			spew.Sdump(&msg.Header), spew.Sdump(bh))
	}

	// Ensure transactions are added properly.
	tx := testBlock.Transactions[0].Copy()
	msg.AddTransaction(tx)
	if !reflect.DeepEqual(msg.Transactions, testBlock.Transactions) {
		t.Errorf("AddTransaction: wrong transactions - got %v, want %v",
			spew.Sdump(msg.Transactions),
			spew.Sdump(testBlock.Transactions))
	}

	// Ensure transactions are properly cleared.
	msg.ClearTransactions()
	if len(msg.Transactions) != 0 {
		t.Errorf("ClearTransactions: wrong transactions - got %v, want %v",
			len(msg.Transactions), 0)
	}

	// Ensure stake transactions are added properly.
	stx := testBlock.STransactions[0].Copy()
	msg.AddSTransaction(stx)
	if !reflect.DeepEqual(msg.STransactions, testBlock.STransactions) {
		t.Errorf("AddSTransaction: wrong transactions - got %v, want %v",
			spew.Sdump(msg.STransactions),
			spew.Sdump(testBlock.STransactions))
	}

	// Ensure transactions are properly cleared.
	msg.ClearSTransactions()
	if len(msg.STransactions) != 0 {
		t.Errorf("ClearTransactions: wrong transactions - got %v, want %v",
			len(msg.STransactions), 0)
	}
}

// TestBlockTxHashes tests the ability to generate a slice of all transaction
// hashes from a block accurately.
func TestBlockTxHashes(t *testing.T) {
	// Block 1, transaction 1 hash.
	hashStr := "55a25248c04dd8b6599ca2a708413c00d79ae90ce075c54e8a967a647d7e4bea"
	wantHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
		return
	}

	wantHashes := []chainhash.Hash{*wantHash}
	hashes := testBlock.TxHashes()
	if !reflect.DeepEqual(hashes, wantHashes) {
		t.Errorf("TxHashes: wrong transaction hashes - got %v, want %v",
			spew.Sdump(hashes), spew.Sdump(wantHashes))
	}
}

// TestBlockSTxHashes tests the ability to generate a slice of all stake
// transaction hashes from a block accurately.
func TestBlockSTxHashes(t *testing.T) {
	// Block 1, transaction 1 hash.
	hashStr := "ae208a69f3ee088d0328126e3d9bef7652b108d1904f27b166c5999233a801d4"
	wantHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
		return
	}

	wantHashes := []chainhash.Hash{*wantHash}
	hashes := testBlock.STxHashes()
	if !reflect.DeepEqual(hashes, wantHashes) {
		t.Errorf("STxHashes: wrong transaction hashes - got %v, want %v",
			spew.Sdump(hashes), spew.Sdump(wantHashes))
	}
}

// TestBlockHash tests the ability to generate the hash of a block accurately.
func TestBlockHash(t *testing.T) {
	// Block 1 hash.
	hashStr := "6b73b6f6faebbfd6a541f38820593e43c50ce1abf64602ab8ac7d5502991c37f"
	wantHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
	}

	// Ensure the hash produced is expected.
	blockHash := testBlock.BlockHash()
	if !blockHash.IsEqual(wantHash) {
		t.Errorf("BlockHash: wrong hash - got %v, want %v",
			spew.Sprint(blockHash), spew.Sprint(wantHash))
	}
}

// TestBlockWire tests the MsgBlock wire encode and decode for various numbers
// of transaction inputs and outputs and protocol versions.
func TestBlockWire(t *testing.T) {
	tests := []struct {
		in      *MsgBlock // Message to encode
		out     *MsgBlock // Expected decoded message
		buf     []byte    // Wire encoding
		txLocs  []TxLoc   // Expected transaction locations
		sTxLocs []TxLoc   // Expected stake transaction locations
		pver    uint32    // Protocol version for wire encoding
	}{
		// Latest protocol version.
		{
			&testBlock,
			&testBlock,
			testBlockBytes,
			testBlockTxLocs,
			testBlockSTxLocs,
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
		var msg MsgBlock
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

// TestBlockWireErrors performs negative tests against wire encode and decode
// of MsgBlock to confirm error paths work correctly.
func TestBlockWireErrors(t *testing.T) {
	// Use protocol version 60002 specifically here instead of the latest
	// because the test data is using bytes encoded with that protocol
	// version.
	pver := uint32(60002)

	tests := []struct {
		in       *MsgBlock // Value to encode
		buf      []byte    // Wire encoding
		pver     uint32    // Protocol version for wire encoding
		max      int       // Max size of fixed buffer to induce errors
		writeErr error     // Expected write error
		readErr  error     // Expected read error
	}{ // Force error in version.
		{&testBlock, testBlockBytes, pver, 0, io.ErrShortWrite, io.EOF}, // 0
		// Force error in prev block hash.
		{&testBlock, testBlockBytes, pver, 4, io.ErrShortWrite, io.EOF}, // 1
		// Force error in merkle root.
		{&testBlock, testBlockBytes, pver, 36, io.ErrShortWrite, io.EOF}, // 2
		// Force error in stake root.
		{&testBlock, testBlockBytes, pver, 68, io.ErrShortWrite, io.EOF}, // 3
		// Force error in vote bits.
		{&testBlock, testBlockBytes, pver, 100, io.ErrShortWrite, io.EOF}, // 4
		// Force error in finalState.
		{&testBlock, testBlockBytes, pver, 102, io.ErrShortWrite, io.EOF}, // 5
		// Force error in voters.
		{&testBlock, testBlockBytes, pver, 108, io.ErrShortWrite, io.EOF}, // 6
		// Force error in freshstake.
		{&testBlock, testBlockBytes, pver, 110, io.ErrShortWrite, io.EOF}, // 7
		// Force error in revocations.
		{&testBlock, testBlockBytes, pver, 111, io.ErrShortWrite, io.EOF}, // 8
		// Force error in poolsize.
		{&testBlock, testBlockBytes, pver, 112, io.ErrShortWrite, io.EOF}, // 9
		// Force error in difficulty bits.
		{&testBlock, testBlockBytes, pver, 116, io.ErrShortWrite, io.EOF}, // 10
		// Force error in stake difficulty bits.
		{&testBlock, testBlockBytes, pver, 120, io.ErrShortWrite, io.EOF}, // 11
		// Force error in height.
		{&testBlock, testBlockBytes, pver, 128, io.ErrShortWrite, io.EOF}, // 12
		// Force error in size.
		{&testBlock, testBlockBytes, pver, 132, io.ErrShortWrite, io.EOF}, // 13
		// Force error in timestamp.
		{&testBlock, testBlockBytes, pver, 136, io.ErrShortWrite, io.EOF}, // 14
		// Force error in nonce.
		{&testBlock, testBlockBytes, pver, 140, io.ErrShortWrite, io.EOF}, // 15
		// Force error in tx count.
		{&testBlock, testBlockBytes, pver, 180, io.ErrShortWrite, io.EOF}, // 16
		// Force error in tx.
		{&testBlock, testBlockBytes, pver, 181, io.ErrShortWrite, io.EOF}, // 17
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
		var msg MsgBlock
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if err != test.readErr {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}

// TestBlockSerialize tests MsgBlock serialize and deserialize.
func TestBlockSerialize(t *testing.T) {
	tests := []struct {
		in      *MsgBlock // Message to encode
		out     *MsgBlock // Expected decoded message
		buf     []byte    // Serialized data
		txLocs  []TxLoc   // Expected transaction locations
		sTxLocs []TxLoc   // Expected stake transaction locations
	}{
		{
			&testBlock,
			&testBlock,
			testBlockBytes,
			testBlockTxLocs,
			testBlockSTxLocs,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Serialize the block.
		buf := bytes.NewBuffer(make([]byte, 0, test.in.SerializeSize()))
		err := test.in.Serialize(buf)
		if err != nil {
			t.Errorf("Serialize #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("Serialize #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Deserialize the block.
		var block MsgBlock
		rbuf := bytes.NewReader(test.buf)
		err = block.Deserialize(rbuf)
		if err != nil {
			t.Errorf("Deserialize #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&block, test.out) {
			t.Errorf("Deserialize #%d\n got: %s want: %s", i,
				spew.Sdump(&block), spew.Sdump(test.out))
			continue
		}

		// Deserialize the block while gathering transaction location
		// information.
		var txLocBlock MsgBlock
		br := bytes.NewBuffer(test.buf)
		txLocs, sTxLocs, err := txLocBlock.DeserializeTxLoc(br)
		if err != nil {
			t.Errorf("DeserializeTxLoc #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&txLocBlock, test.out) {
			t.Errorf("DeserializeTxLoc #%d\n got: %s want: %s", i,
				spew.Sdump(&txLocBlock), spew.Sdump(test.out))
			continue
		}
		if !reflect.DeepEqual(txLocs, test.txLocs) {
			t.Errorf("DeserializeTxLoc #%d\n got: %s want: %s", i,
				spew.Sdump(txLocs), spew.Sdump(test.txLocs))
			continue
		}
		if !reflect.DeepEqual(sTxLocs, test.sTxLocs) {
			t.Errorf("DeserializeTxLoc, sTxLocs #%d\n got: %s want: %s", i,
				spew.Sdump(sTxLocs), spew.Sdump(test.sTxLocs))
			continue
		}
	}
}

// TestBlockSerializeErrors performs negative tests against wire encode and
// decode of MsgBlock to confirm error paths work correctly.
func TestBlockSerializeErrors(t *testing.T) {
	tests := []struct {
		in       *MsgBlock // Value to encode
		buf      []byte    // Serialized data
		max      int       // Max size of fixed buffer to induce errors
		writeErr error     // Expected write error
		readErr  error     // Expected read error
	}{
		{&testBlock, testBlockBytes, 0, io.ErrShortWrite, io.EOF}, // 0
		// Force error in prev block hash.
		{&testBlock, testBlockBytes, 4, io.ErrShortWrite, io.EOF}, // 1
		// Force error in merkle root.
		{&testBlock, testBlockBytes, 36, io.ErrShortWrite, io.EOF}, // 2
		// Force error in stake root.
		{&testBlock, testBlockBytes, 68, io.ErrShortWrite, io.EOF}, // 3
		// Force error in vote bits.
		{&testBlock, testBlockBytes, 100, io.ErrShortWrite, io.EOF}, // 4
		// Force error in finalState.
		{&testBlock, testBlockBytes, 102, io.ErrShortWrite, io.EOF}, // 5
		// Force error in voters.
		{&testBlock, testBlockBytes, 108, io.ErrShortWrite, io.EOF}, // 8
		// Force error in freshstake.
		{&testBlock, testBlockBytes, 110, io.ErrShortWrite, io.EOF}, // 9
		// Force error in revocations.
		{&testBlock, testBlockBytes, 111, io.ErrShortWrite, io.EOF}, // 10
		// Force error in poolsize.
		{&testBlock, testBlockBytes, 112, io.ErrShortWrite, io.EOF}, // 11
		// Force error in difficulty bits.
		{&testBlock, testBlockBytes, 116, io.ErrShortWrite, io.EOF}, // 12
		// Force error in stake difficulty bits.
		{&testBlock, testBlockBytes, 120, io.ErrShortWrite, io.EOF}, // 13
		// Force error in height.
		{&testBlock, testBlockBytes, 128, io.ErrShortWrite, io.EOF}, // 14
		// Force error in size.
		{&testBlock, testBlockBytes, 132, io.ErrShortWrite, io.EOF}, // 15
		// Force error in timestamp.
		{&testBlock, testBlockBytes, 136, io.ErrShortWrite, io.EOF}, // 16
		// Force error in nonce.
		{&testBlock, testBlockBytes, 140, io.ErrShortWrite, io.EOF}, // 17
		// Force error in tx count.
		{&testBlock, testBlockBytes, 180, io.ErrShortWrite, io.EOF}, // 18
		// Force error in tx.
		{&testBlock, testBlockBytes, 181, io.ErrShortWrite, io.EOF}, // 19
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Serialize the block.
		w := newFixedWriter(test.max)
		err := test.in.Serialize(w)
		if err != test.writeErr {
			t.Errorf("Serialize #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// Deserialize the block.
		var block MsgBlock
		r := newFixedReader(test.max, test.buf)
		err = block.Deserialize(r)
		if err != test.readErr {
			t.Errorf("Deserialize #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}

		var txLocBlock MsgBlock
		br := bytes.NewBuffer(test.buf[0:test.max])
		_, _, err = txLocBlock.DeserializeTxLoc(br)
		if err != test.readErr {
			t.Errorf("DeserializeTxLoc #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}

// TestBlockOverflowErrors  performs tests to ensure deserializing blocks which
// are intentionally crafted to use large values for the number of transactions
// are handled properly.  This could otherwise potentially be used as an attack
// vector.
func TestBlockOverflowErrors(t *testing.T) {
	// Use protocol version 70001 specifically here instead of the latest
	// protocol version because the test data is using bytes encoded with
	// that version.
	pver := uint32(1)

	tests := []struct {
		buf  []byte // Wire encoding
		pver uint32 // Protocol version for wire encoding
		err  error  // Expected error
	}{
		// Block that claims to have ~uint64(0) transactions.
		{
			[]byte{
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
				0x01, 0x00, 0x00, 0x00, // Height
				0x01, 0x00, 0x00, 0x00, // Size
				0x61, 0xbc, 0x66, 0x49, // Timestamp
				0x01, 0xe3, 0x62, 0x99, // Nonce
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ExtraData
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0x5c, 0xa1, 0xab, 0x1e, //StakeVersion
				0xff, // TxnCount
			}, pver, &MessageError{},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire format.
		var msg MsgBlock
		r := bytes.NewReader(test.buf)
		err := msg.BtcDecode(r, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, reflect.TypeOf(test.err))
			continue
		}

		// Deserialize from wire format.
		r = bytes.NewReader(test.buf)
		err = msg.Deserialize(r)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Deserialize #%d wrong error got: %v, want: %v",
				i, err, reflect.TypeOf(test.err))
			continue
		}

		// Deserialize with transaction location info from wire format.
		br := bytes.NewBuffer(test.buf)
		_, _, err = msg.DeserializeTxLoc(br)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("DeserializeTxLoc #%d wrong error got: %v, "+
				"want: %v", i, err, reflect.TypeOf(test.err))
			continue
		}
	}
}

// TestBlockSerializeSize performs tests to ensure the serialize size for
// various blocks is accurate.
func TestBlockSerializeSize(t *testing.T) {
	// Block with no transactions.
	noTxBlock := NewMsgBlock(&testBlock.Header)

	tests := []struct {
		in   *MsgBlock // Block to encode
		size int       // Expected serialized size
	}{
		// Block with no transactions (header + 2x numtx)
		{noTxBlock, 182},

		// First block in the mainnet block chain.
		{&testBlock, len(testBlockBytes)},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		serializedSize := test.in.SerializeSize()
		if serializedSize != test.size {
			t.Errorf("MsgBlock.SerializeSize: #%d got: %d, want: "+
				"%d", i, serializedSize, test.size)
			continue
		}
	}
}

// testBlock is a basic normative block that is used throughout tests.
var testBlock = MsgBlock{
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
		PoolSize:     uint32(0x00000000), // Poolsize
		Bits:         0x1d00ffff,         // 486604799
		SBits:        int64(0x0000000000000000),
		Height:       uint32(1),
		Size:         uint32(1),
		Timestamp:    time.Unix(0x4966bc61, 0), // 2009-01-08 20:54:25 -0600 CST
		Nonce:        0x9962e301,               // 2573394689
		ExtraData:    [32]byte{},
		StakeVersion: uint32(0x5ca1ab1e),
	},
	Transactions: []*MsgTx{
		{
			SerType: TxSerializeFull,
			Version: 1,
			TxIn: []*TxIn{
				{
					PreviousOutPoint: OutPoint{
						Hash:  chainhash.Hash{},
						Index: 0xffffffff,
						Tree:  TxTreeRegular,
					},
					Sequence:    0xffffffff,
					ValueIn:     0x1616161616161616,
					BlockHeight: 0x17171717,
					BlockIndex:  0x18181818,
					SignatureScript: []byte{
						0xff, 0xff, 0xff, 0xff, 0x01, 0x00, 0xf2,
					},
				},
			},
			TxOut: []*TxOut{
				{
					Value:   0x3333333333333333,
					Version: 0x9898,
					PkScript: []byte{
						0x41, // OP_DATA_65
						0x04, 0x96, 0xb5, 0x38, 0xe8, 0x53, 0x51, 0x9c,
						0x72, 0x6a, 0x2c, 0x91, 0xe6, 0x1e, 0xc1, 0x16,
						0x00, 0xae, 0x13, 0x90, 0x81, 0x3a, 0x62, 0x7c,
						0x66, 0xfb, 0x8b, 0xe7, 0x94, 0x7b, 0xe6, 0x3c,
						0x52, 0xda, 0x75, 0x89, 0x37, 0x95, 0x15, 0xd4,
						0xe0, 0xa6, 0x04, 0xf8, 0x14, 0x17, 0x81, 0xe6,
						0x22, 0x94, 0x72, 0x11, 0x66, 0xbf, 0x62, 0x1e,
						0x73, 0xa8, 0x2c, 0xbf, 0x23, 0x42, 0xc8, 0x58,
						0xee, // 65-byte signature
						0xac, // OP_CHECKSIG
					},
				},
			},
			LockTime: 0x11111111,
			Expiry:   0x22222222,
		},
	},
	STransactions: []*MsgTx{
		{
			SerType: TxSerializeFull,
			Version: 1,
			TxIn: []*TxIn{
				{
					PreviousOutPoint: OutPoint{
						Hash:  chainhash.Hash{},
						Index: 0xffffffff,
						Tree:  TxTreeStake,
					},
					Sequence:    0xffffffff,
					ValueIn:     0x1313131313131313,
					BlockHeight: 0x14141414,
					BlockIndex:  0x15151515,
					SignatureScript: []byte{
						0xff, 0xff, 0xff, 0xff, 0x01, 0x00, 0xf2,
					},
				},
			},
			TxOut: []*TxOut{
				{
					Value:   0x3333333333333333,
					Version: 0x1212,
					PkScript: []byte{
						0x41, // OP_DATA_65
						0x04, 0x96, 0xb5, 0x38, 0xe8, 0x53, 0x51, 0x9c,
						0x72, 0x6a, 0x2c, 0x91, 0xe6, 0x1e, 0xc1, 0x16,
						0x00, 0xae, 0x13, 0x90, 0x81, 0x3a, 0x62, 0x7c,
						0x66, 0xfb, 0x8b, 0xe7, 0x94, 0x7b, 0xe6, 0x3c,
						0x52, 0xda, 0x75, 0x89, 0x37, 0x95, 0x15, 0xd4,
						0xe0, 0xa6, 0x04, 0xf8, 0x14, 0x17, 0x81, 0xe6,
						0x22, 0x94, 0x72, 0x11, 0x66, 0xbf, 0x62, 0x1e,
						0x73, 0xa8, 0x2c, 0xbf, 0x23, 0x42, 0xc8, 0x58,
						0xee, // 65-byte signature
						0xac, // OP_CHECKSIG
					},
				},
			},
			LockTime: 0x11111111,
			Expiry:   0x22222222,
		},
	},
}

// testBlockBytes is the serialized bytes for the above test block (testBlock).
var testBlockBytes = []byte{
	// Begin block header
	0x01, 0x00, 0x00, 0x00, // Version 1 [0]
	0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
	0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
	0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
	0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, // PrevBlock [4]
	0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
	0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
	0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
	0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e, // MerkleRoot [36]
	0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
	0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
	0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
	0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e, // StakeRoot [68]
	0x00, 0x00, // VoteBits [100]
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // FinalState [102]
	0x00, 0x00, // Voters [108]
	0x00,                   // FreshStake [110]
	0x00,                   // Revocations [111]
	0x00, 0x00, 0x00, 0x00, // Poolsize [112]
	0xff, 0xff, 0x00, 0x1d, // Bits [116]
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SBits [120]
	0x01, 0x00, 0x00, 0x00, // Height [128]
	0x01, 0x00, 0x00, 0x00, // Size [132]
	0x61, 0xbc, 0x66, 0x49, // Timestamp [136]
	0x01, 0xe3, 0x62, 0x99, // Nonce [140]
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ExtraData [144]
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x1e, 0xab, 0xa1, 0x5c, // StakeVersion
	// Announce number of txs
	0x01, // TxnCount [180]
	// Begin bogus normal txs
	0x01, 0x00, 0x00, 0x00, // Version [181]
	0x01, // Varint for number of transaction inputs [185]
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Previous output hash [186]
	0xff, 0xff, 0xff, 0xff, // Prevous output index [218]
	0x00,                   // Previous output tree [222]
	0xff, 0xff, 0xff, 0xff, // Sequence [223]
	0x01,                                           // Varint for number of transaction outputs [227]
	0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // Transaction amount [228]
	0x98, 0x98, // Script version
	0x43, // Varint for length of pk script
	0x41, // OP_DATA_65
	0x04, 0x96, 0xb5, 0x38, 0xe8, 0x53, 0x51, 0x9c,
	0x72, 0x6a, 0x2c, 0x91, 0xe6, 0x1e, 0xc1, 0x16,
	0x00, 0xae, 0x13, 0x90, 0x81, 0x3a, 0x62, 0x7c,
	0x66, 0xfb, 0x8b, 0xe7, 0x94, 0x7b, 0xe6, 0x3c,
	0x52, 0xda, 0x75, 0x89, 0x37, 0x95, 0x15, 0xd4,
	0xe0, 0xa6, 0x04, 0xf8, 0x14, 0x17, 0x81, 0xe6,
	0x22, 0x94, 0x72, 0x11, 0x66, 0xbf, 0x62, 0x1e,
	0x73, 0xa8, 0x2c, 0xbf, 0x23, 0x42, 0xc8, 0x58,
	0xee,                   // 65-byte signature
	0xac,                   // OP_CHECKSIG
	0x11, 0x11, 0x11, 0x11, // Lock time
	0x22, 0x22, 0x22, 0x22, // Expiry
	0x01,                                           // Varint for number of signatures
	0x16, 0x16, 0x16, 0x16, 0x16, 0x16, 0x16, 0x16, // ValueIn
	0x17, 0x17, 0x17, 0x17, // BlockHeight
	0x18, 0x18, 0x18, 0x18, // BlockIndex
	0x07,                                     // SigScript length
	0xff, 0xff, 0xff, 0xff, 0x01, 0x00, 0xf2, // Signature script (coinbase)
	// Announce number of stake txs
	0x01, // TxnCount for stake tx
	// Begin bogus stake txs
	0x01, 0x00, 0x00, 0x00, // Version
	0x01, // Varint for number of transaction inputs
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Previous output hash
	0xff, 0xff, 0xff, 0xff, // Prevous output index
	0x01,                   // Previous output tree
	0xff, 0xff, 0xff, 0xff, // Sequence
	0x01,                                           // Varint for number of transaction outputs
	0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // Transaction amount
	0x12, 0x12, // Script version
	0x43, // Varint for length of pk script
	0x41, // OP_DATA_65
	0x04, 0x96, 0xb5, 0x38, 0xe8, 0x53, 0x51, 0x9c,
	0x72, 0x6a, 0x2c, 0x91, 0xe6, 0x1e, 0xc1, 0x16,
	0x00, 0xae, 0x13, 0x90, 0x81, 0x3a, 0x62, 0x7c,
	0x66, 0xfb, 0x8b, 0xe7, 0x94, 0x7b, 0xe6, 0x3c,
	0x52, 0xda, 0x75, 0x89, 0x37, 0x95, 0x15, 0xd4,
	0xe0, 0xa6, 0x04, 0xf8, 0x14, 0x17, 0x81, 0xe6,
	0x22, 0x94, 0x72, 0x11, 0x66, 0xbf, 0x62, 0x1e,
	0x73, 0xa8, 0x2c, 0xbf, 0x23, 0x42, 0xc8, 0x58,
	0xee,                   // 65-byte signature
	0xac,                   // OP_CHECKSIG
	0x11, 0x11, 0x11, 0x11, // Lock time
	0x22, 0x22, 0x22, 0x22, // Expiry
	0x01,                                           // Varint for number of signatures
	0x13, 0x13, 0x13, 0x13, 0x13, 0x13, 0x13, 0x13, // ValueIn
	0x14, 0x14, 0x14, 0x14, // BlockHeight
	0x15, 0x15, 0x15, 0x15, // BlockIndex
	0x07,                                     // SigScript length
	0xff, 0xff, 0xff, 0xff, 0x01, 0x00, 0xf2, // Signature script (coinbase)
}

// Transaction location information for the test block transactions.
var testBlockTxLocs = []TxLoc{
	{TxStart: 181, TxLen: 158},
}

// Transaction location information for the test block stake transactions.
var testBlockSTxLocs = []TxLoc{
	{TxStart: 340, TxLen: 158},
}
