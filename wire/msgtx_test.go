// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/chaincfg/chainhash"
)

// TestTx tests the MsgTx API.
func TestTx(t *testing.T) {
	pver := ProtocolVersion

	// Block 100000 hash.
	hashStr := "3ba27aa200b1cecaad478d2b00432346c3f1f3986da1afd33e506"
	hash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
	}

	// Ensure the command is expected value.
	wantCmd := "tx"
	msg := NewMsgTx()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgAddr: wrong command - got %v want %v",
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

	// Ensure we get the same transaction output point data back out.
	// NOTE: This is a block hash and made up index, but we're only
	// testing package functionality.
	prevOutIndex := uint32(1)
	prevOut := NewOutPoint(hash, prevOutIndex, TxTreeRegular)
	if !prevOut.Hash.IsEqual(hash) {
		t.Errorf("NewOutPoint: wrong hash - got %v, want %v",
			spew.Sprint(&prevOut.Hash), spew.Sprint(hash))
	}
	if prevOut.Index != prevOutIndex {
		t.Errorf("NewOutPoint: wrong index - got %v, want %v",
			prevOut.Index, prevOutIndex)
	}
	prevOutStr := fmt.Sprintf("%s:%d", hash.String(), prevOutIndex)
	if s := prevOut.String(); s != prevOutStr {
		t.Errorf("OutPoint.String: unexpected result - got %v, "+
			"want %v", s, prevOutStr)
	}

	// Ensure we get the same transaction input back out.
	sigScript := []byte{0x04, 0x31, 0xdc, 0x00, 0x1b, 0x01, 0x62}
	txIn := NewTxIn(prevOut, sigScript)
	if !reflect.DeepEqual(&txIn.PreviousOutPoint, prevOut) {
		t.Errorf("NewTxIn: wrong prev outpoint - got %v, want %v",
			spew.Sprint(&txIn.PreviousOutPoint),
			spew.Sprint(prevOut))
	}
	if !bytes.Equal(txIn.SignatureScript, sigScript) {
		t.Errorf("NewTxIn: wrong signature script - got %v, want %v",
			spew.Sdump(txIn.SignatureScript),
			spew.Sdump(sigScript))
	}

	// Ensure we get the same transaction output back out.
	txValue := int64(5000000000)
	pkScript := []byte{
		0x41, // OP_DATA_65
		0x04, 0xd6, 0x4b, 0xdf, 0xd0, 0x9e, 0xb1, 0xc5,
		0xfe, 0x29, 0x5a, 0xbd, 0xeb, 0x1d, 0xca, 0x42,
		0x81, 0xbe, 0x98, 0x8e, 0x2d, 0xa0, 0xb6, 0xc1,
		0xc6, 0xa5, 0x9d, 0xc2, 0x26, 0xc2, 0x86, 0x24,
		0xe1, 0x81, 0x75, 0xe8, 0x51, 0xc9, 0x6b, 0x97,
		0x3d, 0x81, 0xb0, 0x1c, 0xc3, 0x1f, 0x04, 0x78,
		0x34, 0xbc, 0x06, 0xd6, 0xd6, 0xed, 0xf6, 0x20,
		0xd1, 0x84, 0x24, 0x1a, 0x6a, 0xed, 0x8b, 0x63,
		0xa6, // 65-byte signature
		0xac, // OP_CHECKSIG
	}
	txOut := NewTxOut(txValue, pkScript)
	if txOut.Value != txValue {
		t.Errorf("NewTxOut: wrong pk script - got %v, want %v",
			txOut.Value, txValue)

	}
	if !bytes.Equal(txOut.PkScript, pkScript) {
		t.Errorf("NewTxOut: wrong pk script - got %v, want %v",
			spew.Sdump(txOut.PkScript),
			spew.Sdump(pkScript))
	}

	// Ensure transaction inputs are added properly.
	msg.AddTxIn(txIn)
	if !reflect.DeepEqual(msg.TxIn[0], txIn) {
		t.Errorf("AddTxIn: wrong transaction input added - got %v, want %v",
			spew.Sprint(msg.TxIn[0]), spew.Sprint(txIn))
	}

	// Ensure transaction outputs are added properly.
	msg.AddTxOut(txOut)
	if !reflect.DeepEqual(msg.TxOut[0], txOut) {
		t.Errorf("AddTxIn: wrong transaction output added - got %v, want %v",
			spew.Sprint(msg.TxOut[0]), spew.Sprint(txOut))
	}

	// Ensure the copy produced an identical transaction message.
	newMsg := msg.Copy()
	if !reflect.DeepEqual(newMsg, msg) {
		t.Errorf("Copy: mismatched tx messages - got %v, want %v",
			spew.Sdump(newMsg), spew.Sdump(msg))
	}
}

// TestTxHash tests the ability to generate the hash of a transaction accurately.
func TestTxHash(t *testing.T) {
	// Hash of first transaction from block 113875.
	hashStr := "4538fc1618badd058ee88fd020984451024858796be0a1ed111877f887e1bd53"
	wantHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
		return
	}

	msgTx := NewMsgTx()
	txIn := TxIn{
		PreviousOutPoint: OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0xffffffff,
			Tree:  TxTreeRegular,
		},
		Sequence:        0xffffffff,
		ValueIn:         5000000000,
		BlockHeight:     0x3F3F3F3F,
		BlockIndex:      0x2E2E2E2E,
		SignatureScript: []byte{0x04, 0x31, 0xdc, 0x00, 0x1b, 0x01, 0x62},
	}
	txOut := TxOut{
		Value:   5000000000,
		Version: 0xf0f0,
		PkScript: []byte{
			0x41, // OP_DATA_65
			0x04, 0xd6, 0x4b, 0xdf, 0xd0, 0x9e, 0xb1, 0xc5,
			0xfe, 0x29, 0x5a, 0xbd, 0xeb, 0x1d, 0xca, 0x42,
			0x81, 0xbe, 0x98, 0x8e, 0x2d, 0xa0, 0xb6, 0xc1,
			0xc6, 0xa5, 0x9d, 0xc2, 0x26, 0xc2, 0x86, 0x24,
			0xe1, 0x81, 0x75, 0xe8, 0x51, 0xc9, 0x6b, 0x97,
			0x3d, 0x81, 0xb0, 0x1c, 0xc3, 0x1f, 0x04, 0x78,
			0x34, 0xbc, 0x06, 0xd6, 0xd6, 0xed, 0xf6, 0x20,
			0xd1, 0x84, 0x24, 0x1a, 0x6a, 0xed, 0x8b, 0x63,
			0xa6, // 65-byte signature
			0xac, // OP_CHECKSIG
		},
	}
	msgTx.AddTxIn(&txIn)
	msgTx.AddTxOut(&txOut)
	msgTx.LockTime = 0
	msgTx.Expiry = 0

	// Ensure the hash produced is expected.
	txHash := msgTx.TxHash()
	if !txHash.IsEqual(wantHash) {
		t.Errorf("TxHash: wrong hash - got %v, want %v",
			spew.Sprint(txHash), spew.Sprint(wantHash))
	}
}

// TestTxWire tests the MsgTx wire encode and decode for various numbers
// of transaction inputs and outputs and protocol versions.
func TestTxWire(t *testing.T) {
	// Empty tx message.
	noTx := NewMsgTx()
	noTx.Version = 1
	noTxEncoded := []byte{
		0x01, 0x00, 0x00, 0x00, // Version
		0x00,                   // Varint for number of input transactions
		0x00,                   // Varint for number of output transactions
		0x00, 0x00, 0x00, 0x00, // Lock time
		0x00, 0x00, 0x00, 0x00, // Expiry
		0x00, // Varint for number of input signatures
	}

	tests := []struct {
		in   *MsgTx // Message to encode
		out  *MsgTx // Expected decoded message
		buf  []byte // Wire encoding
		pver uint32 // Protocol version for wire encoding
	}{
		// Latest protocol version with no transactions.
		{
			noTx,
			noTx,
			noTxEncoded,
			ProtocolVersion,
		},

		// Latest protocol version with multiple transactions.
		{
			multiTx,
			multiTx,
			multiTxEncoded,
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
		var msg MsgTx
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

// TestTxWireErrors performs negative tests against wire encode and decode
// of MsgTx to confirm error paths work correctly.
func TestTxWireErrors(t *testing.T) {
	// Use protocol version 60002 specifically here instead of the latest
	// because the test data is using bytes encoded with that protocol
	// version.
	pver := uint32(60002)

	tests := []struct {
		in       *MsgTx // Value to encode
		buf      []byte // Wire encoding
		pver     uint32 // Protocol version for wire encoding
		max      int    // Max size of fixed buffer to induce errors
		writeErr error  // Expected write error
		readErr  error  // Expected read error
	}{
		// Force error in version.
		{multiTx, multiTxEncoded, pver, 0, io.ErrShortWrite, io.EOF}, // 0
		// Force error in number of transaction inputs.
		{multiTx, multiTxEncoded, pver, 4, io.ErrShortWrite, io.EOF}, // 1
		// Force error in transaction input previous block hash.
		{multiTx, multiTxEncoded, pver, 5, io.ErrShortWrite, io.EOF}, // 2
		// Force error in transaction input previous block output index.
		{multiTx, multiTxEncoded, pver, 37, io.ErrShortWrite, io.EOF}, // 3
		// Force error in transaction input previous block output tree.
		{multiTx, multiTxEncoded, pver, 41, io.ErrShortWrite, io.EOF}, // 4
		// Force error in transaction input sequence.
		{multiTx, multiTxEncoded, pver, 42, io.ErrShortWrite, io.EOF}, // 5
		// Force error in number of transaction outputs.
		{multiTx, multiTxEncoded, pver, 46, io.ErrShortWrite, io.EOF}, // 6
		// Force error in transaction output value.
		{multiTx, multiTxEncoded, pver, 47, io.ErrShortWrite, io.EOF}, // 7
		// Force error in transaction output script version.
		{multiTx, multiTxEncoded, pver, 55, io.ErrShortWrite, io.EOF}, // 8
		// Force error in transaction output pk script length.
		{multiTx, multiTxEncoded, pver, 57, io.ErrShortWrite, io.EOF}, // 9
		// Force error in transaction output pk script.
		{multiTx, multiTxEncoded, pver, 58, io.ErrShortWrite, io.EOF}, // 10
		// Force error in transaction output lock time.
		{multiTx, multiTxEncoded, pver, 203, io.ErrShortWrite, io.EOF}, // 11
		// Force error in transaction output expiry.
		{multiTx, multiTxEncoded, pver, 207, io.ErrShortWrite, io.EOF}, // 12
		// Force error in transaction num sig varint.
		{multiTx, multiTxEncoded, pver, 211, io.ErrShortWrite, io.EOF}, // 13
		// Force error in transaction sig 0 AmountIn.
		{multiTx, multiTxEncoded, pver, 212, io.ErrShortWrite, io.EOF}, // 14
		// Force error in transaction sig 0 BlockHeight.
		{multiTx, multiTxEncoded, pver, 220, io.ErrShortWrite, io.EOF}, // 15
		// Force error in transaction sig 0 BlockIndex.
		{multiTx, multiTxEncoded, pver, 224, io.ErrShortWrite, io.EOF}, // 16
		// Force error in transaction sig 0 length.
		{multiTx, multiTxEncoded, pver, 228, io.ErrShortWrite, io.EOF}, // 17
		// Force error in transaction sig 0 signature script.
		{multiTx, multiTxEncoded, pver, 229, io.ErrShortWrite, io.EOF}, // 18
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
		var msg MsgTx
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if err != test.readErr {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}

// TestTxSerialize tests MsgTx serialize and deserialize.
func TestTxSerialize(t *testing.T) {
	noTx := NewMsgTx()
	noTx.Version = 1
	noTxEncoded := []byte{
		0x01, 0x00, 0x00, 0x00, // Version
		0x00,                   // Varint for number of input transactions
		0x00,                   // Varint for number of output transactions
		0x00, 0x00, 0x00, 0x00, // Lock time
		0x00, 0x00, 0x00, 0x00, // Expiry
		0x00, // Varint for number of input signatures
	}

	tests := []struct {
		in           *MsgTx // Message to encode
		out          *MsgTx // Expected decoded message
		buf          []byte // Serialized data
		pkScriptLocs []int  // Expected output script locations
	}{
		// No transactions.
		{
			noTx,
			noTx,
			noTxEncoded,
			nil,
		},

		// Multiple transactions.
		{
			multiTx,
			multiTx,
			multiTxEncoded,
			multiTxPkScriptLocs,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Serialize the transaction.
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

		// Test SerializeSize.
		sz := test.in.SerializeSize()
		actualSz := len(buf.Bytes())
		if sz != actualSz {
			t.Errorf("Wrong serialize size #%d\n got: %d want: %d", i,
				sz, actualSz)
		}

		// Deserialize the transaction.
		var tx MsgTx
		rbuf := bytes.NewReader(test.buf)
		err = tx.Deserialize(rbuf)
		if err != nil {
			t.Errorf("Deserialize #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&tx, test.out) {
			t.Errorf("Deserialize #%d\n got: %s want: %s", i,
				spew.Sdump(&tx), spew.Sdump(test.out))
			continue
		}

		// Ensure the public key script locations are accurate.
		pkScriptLocs := test.in.PkScriptLocs()
		if !reflect.DeepEqual(pkScriptLocs, test.pkScriptLocs) {
			t.Errorf("PkScriptLocs #%d\n got: %s want: %s", i,
				spew.Sdump(pkScriptLocs),
				spew.Sdump(test.pkScriptLocs))
			continue
		}
		for j, loc := range pkScriptLocs {
			wantPkScript := test.in.TxOut[j].PkScript
			gotPkScript := test.buf[loc : loc+len(wantPkScript)]
			if !bytes.Equal(gotPkScript, wantPkScript) {
				t.Errorf("PkScriptLocs #%d:%d\n unexpected "+
					"script got: %s want: %s", i, j,
					spew.Sdump(gotPkScript),
					spew.Sdump(wantPkScript))
			}
		}
	}
}

// TestTxSerializePrefix tests MsgTx serialize and deserialize.
func TestTxSerializePrefix(t *testing.T) {
	noTx := NewMsgTx()
	noTx.SerType = TxSerializeNoWitness
	noTx.Version = 1
	noTxEncoded := []byte{
		0x01, 0x00, 0x01, 0x00, // Version
		0x00,                   // Varint for number of input transactions
		0x00,                   // Varint for number of output transactions
		0x00, 0x00, 0x00, 0x00, // Lock time
		0x00, 0x00, 0x00, 0x00, // Expiry
	}

	tests := []struct {
		in           *MsgTx // Message to encode
		out          *MsgTx // Expected decoded message
		buf          []byte // Serialized data
		pkScriptLocs []int  // Expected output script locations
	}{
		// No transactions.
		{
			noTx,
			noTx,
			noTxEncoded,
			nil,
		},

		// Multiple transactions.
		{
			multiTxPrefix,
			multiTxPrefix,
			multiTxPrefixEncoded,
			multiTxPkScriptLocs,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Serialize the transaction.
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

		// Test SerializeSize.
		sz := test.in.SerializeSize()
		actualSz := len(buf.Bytes())
		if sz != actualSz {
			t.Errorf("Wrong serialize size #%d\n got: %d want: %d", i,
				sz, actualSz)
		}

		// Deserialize the transaction.
		var tx MsgTx
		rbuf := bytes.NewReader(test.buf)
		err = tx.Deserialize(rbuf)
		if err != nil {
			t.Errorf("Deserialize #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&tx, test.out) {
			t.Errorf("Deserialize #%d\n got: %s want: %s", i,
				spew.Sdump(&tx), spew.Sdump(test.out))
			continue
		}

		// Ensure the public key script locations are accurate.
		pkScriptLocs := test.in.PkScriptLocs()
		if !reflect.DeepEqual(pkScriptLocs, test.pkScriptLocs) {
			t.Errorf("PkScriptLocs #%d\n got: %s want: %s", i,
				spew.Sdump(pkScriptLocs),
				spew.Sdump(test.pkScriptLocs))
			continue
		}
		for j, loc := range pkScriptLocs {
			wantPkScript := test.in.TxOut[j].PkScript
			gotPkScript := test.buf[loc : loc+len(wantPkScript)]
			if !bytes.Equal(gotPkScript, wantPkScript) {
				t.Errorf("PkScriptLocs #%d:%d\n unexpected "+
					"script got: %s want: %s", i, j,
					spew.Sdump(gotPkScript),
					spew.Sdump(wantPkScript))
			}
		}
	}
}

// TestTxSerializeWitness tests MsgTx serialize and deserialize.
func TestTxSerializeWitness(t *testing.T) {
	noTx := NewMsgTx()
	noTx.SerType = TxSerializeOnlyWitness
	noTx.Version = 1
	noTxEncoded := []byte{
		0x01, 0x00, 0x02, 0x00, // Version
		0x00, // Varint for number of input signatures
	}

	tests := []struct {
		in           *MsgTx // Message to encode
		out          *MsgTx // Expected decoded message
		buf          []byte // Serialized data
		pkScriptLocs []int  // Expected output script locations
	}{
		// No transactions.
		{
			noTx,
			noTx,
			noTxEncoded,
			nil,
		},

		// Multiple transactions.
		{
			multiTxWitness,
			multiTxWitness,
			multiTxWitnessEncoded,
			nil,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Serialize the transaction.
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

		// Test SerializeSize.
		sz := test.in.SerializeSize()
		actualSz := len(buf.Bytes())
		if sz != actualSz {
			t.Errorf("Wrong serialize size #%d\n got: %d want: %d", i,
				sz, actualSz)
		}

		// Deserialize the transaction.
		var tx MsgTx
		rbuf := bytes.NewReader(test.buf)
		err = tx.Deserialize(rbuf)
		if err != nil {
			t.Errorf("Deserialize #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&tx, test.out) {
			t.Errorf("Deserialize #%d\n got: %s want: %s", i,
				spew.Sdump(&tx), spew.Sdump(test.out))
			continue
		}

		// Ensure the public key script locations are accurate.
		pkScriptLocs := test.in.PkScriptLocs()
		if !reflect.DeepEqual(pkScriptLocs, test.pkScriptLocs) {
			t.Errorf("PkScriptLocs #%d\n got: %s want: %s", i,
				spew.Sdump(pkScriptLocs),
				spew.Sdump(test.pkScriptLocs))
			continue
		}
		for j, loc := range pkScriptLocs {
			wantPkScript := test.in.TxOut[j].PkScript
			gotPkScript := test.buf[loc : loc+len(wantPkScript)]
			if !bytes.Equal(gotPkScript, wantPkScript) {
				t.Errorf("PkScriptLocs #%d:%d\n unexpected "+
					"script got: %s want: %s", i, j,
					spew.Sdump(gotPkScript),
					spew.Sdump(wantPkScript))
			}
		}
	}
}

// TestTxSerializeWitnessSigning tests MsgTx serialize and deserialize.
func TestTxSerializeWitnessSigning(t *testing.T) {
	noTx := NewMsgTx()
	noTx.SerType = TxSerializeWitnessSigning
	noTx.Version = 1
	noTxEncoded := []byte{
		0x01, 0x00, 0x03, 0x00, // Version
		0x00, // Varint for number of input signatures
	}

	tests := []struct {
		in           *MsgTx // Message to encode
		out          *MsgTx // Expected decoded message
		buf          []byte // Serialized data
		pkScriptLocs []int  // Expected output script locations
	}{
		// No transactions.
		{
			noTx,
			noTx,
			noTxEncoded,
			nil,
		},

		// Multiple transactions.
		{
			multiTxWitnessSigning,
			multiTxWitnessSigning,
			multiTxWitnessSigningEncoded,
			nil,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Serialize the transaction.
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

		// Test SerializeSize.
		sz := test.in.SerializeSize()
		actualSz := len(buf.Bytes())
		if sz != actualSz {
			t.Errorf("Wrong serialize size #%d\n got: %d want: %d", i,
				sz, actualSz)
		}

		// Deserialize the transaction.
		var tx MsgTx
		rbuf := bytes.NewReader(test.buf)
		err = tx.Deserialize(rbuf)
		if err != nil {
			t.Errorf("Deserialize #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&tx, test.out) {
			t.Errorf("Deserialize #%d\n got: %s want: %s", i,
				spew.Sdump(&tx), spew.Sdump(test.out))
			continue
		}

		// Ensure the public key script locations are accurate.
		pkScriptLocs := test.in.PkScriptLocs()
		if !reflect.DeepEqual(pkScriptLocs, test.pkScriptLocs) {
			t.Errorf("PkScriptLocs #%d\n got: %s want: %s", i,
				spew.Sdump(pkScriptLocs),
				spew.Sdump(test.pkScriptLocs))
			continue
		}
		for j, loc := range pkScriptLocs {
			wantPkScript := test.in.TxOut[j].PkScript
			gotPkScript := test.buf[loc : loc+len(wantPkScript)]
			if !bytes.Equal(gotPkScript, wantPkScript) {
				t.Errorf("PkScriptLocs #%d:%d\n unexpected "+
					"script got: %s want: %s", i, j,
					spew.Sdump(gotPkScript),
					spew.Sdump(wantPkScript))
			}
		}
	}
}

// TestTxSerializeWitnessValueSigning tests MsgTx serialize and deserialize.
func TestTxSerializeWitnessValueSigning(t *testing.T) {
	noTx := NewMsgTx()
	noTx.SerType = TxSerializeWitnessValueSigning
	noTx.Version = 1
	noTxEncoded := []byte{
		0x01, 0x00, 0x04, 0x00, // Version
		0x00, // Varint for number of input signatures
	}

	tests := []struct {
		in           *MsgTx // Message to encode
		out          *MsgTx // Expected decoded message
		buf          []byte // Serialized data
		pkScriptLocs []int  // Expected output script locations
	}{
		// No transactions.
		{
			noTx,
			noTx,
			noTxEncoded,
			nil,
		},

		// Multiple transactions.
		{
			multiTxWitnessValueSigning,
			multiTxWitnessValueSigning,
			multiTxWitnessValueSigningEncoded,
			nil,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Serialize the transaction.
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

		// Test SerializeSize.
		sz := test.in.SerializeSize()
		actualSz := len(buf.Bytes())
		if sz != actualSz {
			t.Errorf("Wrong serialize size #%d\n got: %d want: %d", i,
				sz, actualSz)
		}

		// Deserialize the transaction.
		var tx MsgTx
		rbuf := bytes.NewReader(test.buf)
		err = tx.Deserialize(rbuf)
		if err != nil {
			t.Errorf("Deserialize #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&tx, test.out) {
			t.Errorf("Deserialize #%d\n got: %s want: %s", i,
				spew.Sdump(&tx), spew.Sdump(test.out))
			continue
		}

		// Ensure the public key script locations are accurate.
		pkScriptLocs := test.in.PkScriptLocs()
		if !reflect.DeepEqual(pkScriptLocs, test.pkScriptLocs) {
			t.Errorf("PkScriptLocs #%d\n got: %s want: %s", i,
				spew.Sdump(pkScriptLocs),
				spew.Sdump(test.pkScriptLocs))
			continue
		}
		for j, loc := range pkScriptLocs {
			wantPkScript := test.in.TxOut[j].PkScript
			gotPkScript := test.buf[loc : loc+len(wantPkScript)]
			if !bytes.Equal(gotPkScript, wantPkScript) {
				t.Errorf("PkScriptLocs #%d:%d\n unexpected "+
					"script got: %s want: %s", i, j,
					spew.Sdump(gotPkScript),
					spew.Sdump(wantPkScript))
			}
		}
	}
}

// TestTxSerializeErrors performs negative tests against wire encode and decode
// of MsgTx to confirm error paths work correctly.
func TestTxSerializeErrors(t *testing.T) {
	tests := []struct {
		in       *MsgTx // Value to encode
		buf      []byte // Serialized data
		max      int    // Max size of fixed buffer to induce errors
		writeErr error  // Expected write error
		readErr  error  // Expected read error
	}{
		// Force error in version.
		{multiTx, multiTxEncoded, 0, io.ErrShortWrite, io.EOF},
		// Force error in number of transaction inputs.
		{multiTx, multiTxEncoded, 4, io.ErrShortWrite, io.EOF},
		// Force error in transaction input previous block hash.
		{multiTx, multiTxEncoded, 5, io.ErrShortWrite, io.EOF},
		// Force error in transaction input previous block output index.
		{multiTx, multiTxEncoded, 37, io.ErrShortWrite, io.EOF},
		// Force error in transaction input previous block output tree.
		{multiTx, multiTxEncoded, 41, io.ErrShortWrite, io.EOF},
		// Force error in transaction input sequence.
		{multiTx, multiTxEncoded, 42, io.ErrShortWrite, io.EOF},
		// Force error in number of transaction outputs.
		{multiTx, multiTxEncoded, 46, io.ErrShortWrite, io.EOF},
		// Force error in transaction output value.
		{multiTx, multiTxEncoded, 47, io.ErrShortWrite, io.EOF},
		// Force error in transaction output version.
		{multiTx, multiTxEncoded, 55, io.ErrShortWrite, io.EOF},
		// Force error in transaction output pk script length.
		{multiTx, multiTxEncoded, 57, io.ErrShortWrite, io.EOF},
		// Force error in transaction output pk script.
		{multiTx, multiTxEncoded, 58, io.ErrShortWrite, io.EOF},
		// Force error in transaction lock time.
		{multiTx, multiTxEncoded, 203, io.ErrShortWrite, io.EOF},
		// Force error in transaction expiry.
		{multiTx, multiTxEncoded, 207, io.ErrShortWrite, io.EOF},
		// Force error in transaction num sig varint.
		{multiTx, multiTxEncoded, 211, io.ErrShortWrite, io.EOF},
		// Force error in transaction sig 0 ValueIn.
		{multiTx, multiTxEncoded, 212, io.ErrShortWrite, io.EOF},
		// Force error in transaction sig 0 BlockHeight.
		{multiTx, multiTxEncoded, 220, io.ErrShortWrite, io.EOF},
		// Force error in transaction sig 0 BlockIndex.
		{multiTx, multiTxEncoded, 224, io.ErrShortWrite, io.EOF},
		// Force error in transaction sig 0 length.
		{multiTx, multiTxEncoded, 228, io.ErrShortWrite, io.EOF},
		// Force error in transaction sig 0 signature script.
		{multiTx, multiTxEncoded, 229, io.ErrShortWrite, io.EOF},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Serialize the transaction.
		w := newFixedWriter(test.max)
		err := test.in.Serialize(w)
		if err != test.writeErr {
			t.Errorf("Serialize #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// Deserialize the transaction.
		var tx MsgTx
		r := newFixedReader(test.max, test.buf)
		err = tx.Deserialize(r)
		if err != test.readErr {
			t.Errorf("Deserialize #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}

// TestTxOverflowErrors performs tests to ensure deserializing transactions
// which are intentionally crafted to use large values for the variable number
// of inputs and outputs are handled properly.  This could otherwise potentially
// be used as an attack vector.
func TestTxOverflowErrors(t *testing.T) {
	// Use protocol version 1 and transaction version 1 specifically
	// here instead of the latest values because the test data is using
	// bytes encoded with those versions.
	pver := uint32(1)
	txVer := int32(1)

	tests := []struct {
		buf     []byte // Wire encoding
		pver    uint32 // Protocol version for wire encoding
		version int32  // Transaction version
		err     error  // Expected error
	}{
		// Transaction that claims to have ~uint64(0) inputs. [0]
		{
			[]byte{
				0x01, 0x00, 0x00, 0x00, // Version
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, // Varint for number of input transactions
			}, pver, txVer, &MessageError{},
		},

		// Transaction that claims to have ~uint64(0) outputs. [1]
		{
			[]byte{
				0x01, 0x00, 0x00, 0x00, // Version
				0x00, // Varint for number of input transactions
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, // Varint for number of output transactions
			}, pver, txVer, &MessageError{},
		},

		// Transaction that has an input with a signature script that [2]
		// claims to have ~uint64(0) length.
		{
			[]byte{
				0x01, 0x00, 0x00, 0x00, // Version
				0x01, // Varint for number of input transactions
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Previous output hash
				0xff, 0xff, 0xff, 0xff, // Previous output index
				0x00,                   // Previous output tree
				0x00,                   // Varint for length of signature script
				0xff, 0xff, 0xff, 0xff, // Sequence
				0x02,                                           // Varint for number of output transactions
				0x00, 0xf2, 0x05, 0x2a, 0x01, 0x00, 0x00, 0x00, // Transaction amount
				0x43, // Varint for length of pk script
				0x41, // OP_DATA_65
				0x04, 0xd6, 0x4b, 0xdf, 0xd0, 0x9e, 0xb1, 0xc5,
				0xfe, 0x29, 0x5a, 0xbd, 0xeb, 0x1d, 0xca, 0x42,
				0x81, 0xbe, 0x98, 0x8e, 0x2d, 0xa0, 0xb6, 0xc1,
				0xc6, 0xa5, 0x9d, 0xc2, 0x26, 0xc2, 0x86, 0x24,
				0xe1, 0x81, 0x75, 0xe8, 0x51, 0xc9, 0x6b, 0x97,
				0x3d, 0x81, 0xb0, 0x1c, 0xc3, 0x1f, 0x04, 0x78,
				0x34, 0xbc, 0x06, 0xd6, 0xd6, 0xed, 0xf6, 0x20,
				0xd1, 0x84, 0x24, 0x1a, 0x6a, 0xed, 0x8b, 0x63,
				0xa6,                                           // 65-byte signature
				0xac,                                           // OP_CHECKSIG
				0x00, 0xe1, 0xf5, 0x05, 0x00, 0x00, 0x00, 0x00, // Transaction amount
				0x43, // Varint for length of pk script
				0x41, // OP_DATA_65
				0x04, 0xd6, 0x4b, 0xdf, 0xd0, 0x9e, 0xb1, 0xc5,
				0xfe, 0x29, 0x5a, 0xbd, 0xeb, 0x1d, 0xca, 0x42,
				0x81, 0xbe, 0x98, 0x8e, 0x2d, 0xa0, 0xb6, 0xc1,
				0xc6, 0xa5, 0x9d, 0xc2, 0x26, 0xc2, 0x86, 0x24,
				0xe1, 0x81, 0x75, 0xe8, 0x51, 0xc9, 0x6b, 0x97,
				0x3d, 0x81, 0xb0, 0x1c, 0xc3, 0x1f, 0x04, 0x78,
				0x34, 0xbc, 0x06, 0xd6, 0xd6, 0xed, 0xf6, 0x20,
				0xd1, 0x84, 0x24, 0x1a, 0x6a, 0xed, 0x8b, 0x63,
				0xa6,                   // 65-byte signature
				0xac,                   // OP_CHECKSIG
				0x00, 0x00, 0x00, 0x00, // Lock time
				0x00, 0x00, 0x00, 0x00, // Expiry
				0x01,                                                 // Varint for number of input signature
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, // Varint for sig script length (overflows)
			}, pver, txVer, &MessageError{},
		},

		// Transaction that has an output with a public key script [3]
		// that claims to have ~uint64(0) length.
		{
			[]byte{
				0x01, 0x00, 0x00, 0x00, // Version
				0x01, // Varint for number of input transactions
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Previous output hash
				0xff, 0xff, 0xff, 0xff, // Prevous output index
				0x00,                   // Previous output tree
				0x00,                   // Varint for length of signature script
				0xff, 0xff, 0xff, 0xff, // Sequence
				0x01,                                           // Varint for number of output transactions
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Transaction amount
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, // Varint for length of public key script
			}, pver, txVer, &MessageError{},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire format.
		var msg MsgTx
		r := bytes.NewReader(test.buf)
		err := msg.BtcDecode(r, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, reflect.TypeOf(test.err))
		}

		// Decode from wire format.
		r = bytes.NewReader(test.buf)
		err = msg.Deserialize(r)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Deserialize #%d wrong error got: %v, want: %v",
				i, err, reflect.TypeOf(test.err))
			continue
		}
	}
}

// TestTxSerializeSize performs tests to ensure the serialize size for various
// transactions is accurate.
func TestTxSerializeSize(t *testing.T) {
	// Empty tx message.
	noTx := NewMsgTx()
	noTx.Version = 1

	tests := []struct {
		in   *MsgTx // Tx to encode
		size int    // Expected serialized size
	}{
		// No inputs or outpus.
		{noTx, 15},

		// Transaction with an input and an output.
		{multiTx, 236},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		serializedSize := test.in.SerializeSize()
		if serializedSize != test.size {
			t.Errorf("MsgTx.SerializeSize: #%d got: %d, want: %d", i,
				serializedSize, test.size)
			continue
		}
	}
}

// multiTx is a MsgTx with an input and output and used in various tests.
var multiTx = &MsgTx{
	SerType: TxSerializeFull,
	Version: 1,
	TxIn: []*TxIn{
		{
			PreviousOutPoint: OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
			},
			Sequence:    0xffffffff,
			ValueIn:     0x1212121212121212,
			BlockHeight: 0x15151515,
			BlockIndex:  0x34343434,
			SignatureScript: []byte{
				0x04, 0x31, 0xdc, 0x00, 0x1b, 0x01, 0x62,
			},
		},
	},
	TxOut: []*TxOut{
		{
			Value:   0x12a05f200,
			Version: 0xabab,
			PkScript: []byte{
				0x41, // OP_DATA_65
				0x04, 0xd6, 0x4b, 0xdf, 0xd0, 0x9e, 0xb1, 0xc5,
				0xfe, 0x29, 0x5a, 0xbd, 0xeb, 0x1d, 0xca, 0x42,
				0x81, 0xbe, 0x98, 0x8e, 0x2d, 0xa0, 0xb6, 0xc1,
				0xc6, 0xa5, 0x9d, 0xc2, 0x26, 0xc2, 0x86, 0x24,
				0xe1, 0x81, 0x75, 0xe8, 0x51, 0xc9, 0x6b, 0x97,
				0x3d, 0x81, 0xb0, 0x1c, 0xc3, 0x1f, 0x04, 0x78,
				0x34, 0xbc, 0x06, 0xd6, 0xd6, 0xed, 0xf6, 0x20,
				0xd1, 0x84, 0x24, 0x1a, 0x6a, 0xed, 0x8b, 0x63,
				0xa6, // 65-byte signature
				0xac, // OP_CHECKSIG
			},
		},
		{
			Value:   0x5f5e100,
			Version: 0xbcbc,
			PkScript: []byte{
				0x41, // OP_DATA_65
				0x04, 0xd6, 0x4b, 0xdf, 0xd0, 0x9e, 0xb1, 0xc5,
				0xfe, 0x29, 0x5a, 0xbd, 0xeb, 0x1d, 0xca, 0x42,
				0x81, 0xbe, 0x98, 0x8e, 0x2d, 0xa0, 0xb6, 0xc1,
				0xc6, 0xa5, 0x9d, 0xc2, 0x26, 0xc2, 0x86, 0x24,
				0xe1, 0x81, 0x75, 0xe8, 0x51, 0xc9, 0x6b, 0x97,
				0x3d, 0x81, 0xb0, 0x1c, 0xc3, 0x1f, 0x04, 0x78,
				0x34, 0xbc, 0x06, 0xd6, 0xd6, 0xed, 0xf6, 0x20,
				0xd1, 0x84, 0x24, 0x1a, 0x6a, 0xed, 0x8b, 0x63,
				0xa6, // 65-byte signature
				0xac, // OP_CHECKSIG
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// multiTxPrefix is a MsgTx prefix with an input and output and used in various tests.
var multiTxPrefix = &MsgTx{
	SerType: TxSerializeNoWitness,
	Version: 1,
	TxIn: []*TxIn{
		{
			PreviousOutPoint: OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
			},
			Sequence: 0xffffffff,
		},
	},
	TxOut: []*TxOut{
		{
			Value:   0x12a05f200,
			Version: 0xabab,
			PkScript: []byte{
				0x41, // OP_DATA_65
				0x04, 0xd6, 0x4b, 0xdf, 0xd0, 0x9e, 0xb1, 0xc5,
				0xfe, 0x29, 0x5a, 0xbd, 0xeb, 0x1d, 0xca, 0x42,
				0x81, 0xbe, 0x98, 0x8e, 0x2d, 0xa0, 0xb6, 0xc1,
				0xc6, 0xa5, 0x9d, 0xc2, 0x26, 0xc2, 0x86, 0x24,
				0xe1, 0x81, 0x75, 0xe8, 0x51, 0xc9, 0x6b, 0x97,
				0x3d, 0x81, 0xb0, 0x1c, 0xc3, 0x1f, 0x04, 0x78,
				0x34, 0xbc, 0x06, 0xd6, 0xd6, 0xed, 0xf6, 0x20,
				0xd1, 0x84, 0x24, 0x1a, 0x6a, 0xed, 0x8b, 0x63,
				0xa6, // 65-byte signature
				0xac, // OP_CHECKSIG
			},
		},
		{
			Value:   0x5f5e100,
			Version: 0xbcbc,
			PkScript: []byte{
				0x41, // OP_DATA_65
				0x04, 0xd6, 0x4b, 0xdf, 0xd0, 0x9e, 0xb1, 0xc5,
				0xfe, 0x29, 0x5a, 0xbd, 0xeb, 0x1d, 0xca, 0x42,
				0x81, 0xbe, 0x98, 0x8e, 0x2d, 0xa0, 0xb6, 0xc1,
				0xc6, 0xa5, 0x9d, 0xc2, 0x26, 0xc2, 0x86, 0x24,
				0xe1, 0x81, 0x75, 0xe8, 0x51, 0xc9, 0x6b, 0x97,
				0x3d, 0x81, 0xb0, 0x1c, 0xc3, 0x1f, 0x04, 0x78,
				0x34, 0xbc, 0x06, 0xd6, 0xd6, 0xed, 0xf6, 0x20,
				0xd1, 0x84, 0x24, 0x1a, 0x6a, 0xed, 0x8b, 0x63,
				0xa6, // 65-byte signature
				0xac, // OP_CHECKSIG
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// multiTxWitness is a MsgTx witness with only input witness.
var multiTxWitness = &MsgTx{
	SerType: TxSerializeOnlyWitness,
	Version: 1,
	TxIn: []*TxIn{
		{
			ValueIn:     0x1212121212121212,
			BlockHeight: 0x15151515,
			BlockIndex:  0x34343434,
			SignatureScript: []byte{
				0x04, 0x31, 0xdc, 0x00, 0x1b, 0x01, 0x62,
			},
		},
	},
	TxOut: []*TxOut{},
}

// multiTxWitnessSigning is a MsgTx witness with only input witness sigscripts.
var multiTxWitnessSigning = &MsgTx{
	SerType: TxSerializeWitnessSigning,
	Version: 1,
	TxIn: []*TxIn{
		{
			SignatureScript: []byte{
				0x04, 0x31, 0xdc, 0x00, 0x1b, 0x01, 0x62,
			},
		},
	},
	TxOut: []*TxOut{},
}

// multiTxWitnessValueSigning is a MsgTx witness with only input witness
// sigscripts.
var multiTxWitnessValueSigning = &MsgTx{
	SerType: TxSerializeWitnessValueSigning,
	Version: 1,
	TxIn: []*TxIn{
		{
			ValueIn: 0x1212121212121212,
			SignatureScript: []byte{
				0x04, 0x31, 0xdc, 0x00, 0x1b, 0x01, 0x62,
			},
		},
	},
	TxOut: []*TxOut{},
}

// multiTxEncoded is the wire encoded bytes for multiTx using protocol version
// 0 and is used in the various tests.
var multiTxEncoded = []byte{
	0x01, 0x00, 0x00, 0x00, // Version [0]
	0x01,                                           // Varint for number of input transactions [4]
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // [5]
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Previous output hash
	0xff, 0xff, 0xff, 0xff, // Previous output index [37]
	0x00,                   // Previous output tree [41]
	0xff, 0xff, 0xff, 0xff, // Sequence [42]
	0x02,                                           // Varint for number of output transactions [46]
	0x00, 0xf2, 0x05, 0x2a, 0x01, 0x00, 0x00, 0x00, // Transaction amount [47]
	0xab, 0xab, // Script version [55]
	0x43, // Varint for length of pk script [57]
	0x41, // OP_DATA_65 [58]
	0x04, 0xd6, 0x4b, 0xdf, 0xd0, 0x9e, 0xb1, 0xc5,
	0xfe, 0x29, 0x5a, 0xbd, 0xeb, 0x1d, 0xca, 0x42,
	0x81, 0xbe, 0x98, 0x8e, 0x2d, 0xa0, 0xb6, 0xc1,
	0xc6, 0xa5, 0x9d, 0xc2, 0x26, 0xc2, 0x86, 0x24,
	0xe1, 0x81, 0x75, 0xe8, 0x51, 0xc9, 0x6b, 0x97,
	0x3d, 0x81, 0xb0, 0x1c, 0xc3, 0x1f, 0x04, 0x78,
	0x34, 0xbc, 0x06, 0xd6, 0xd6, 0xed, 0xf6, 0x20,
	0xd1, 0x84, 0x24, 0x1a, 0x6a, 0xed, 0x8b, 0x63,
	0xa6,                                           // 65-byte pubkey
	0xac,                                           // OP_CHECKSIG
	0x00, 0xe1, 0xf5, 0x05, 0x00, 0x00, 0x00, 0x00, // Transaction amount [123]
	0xbc, 0xbc, // Script version [134]
	0x43, // Varint for length of pk script [136]
	0x41, // OP_DATA_65
	0x04, 0xd6, 0x4b, 0xdf, 0xd0, 0x9e, 0xb1, 0xc5,
	0xfe, 0x29, 0x5a, 0xbd, 0xeb, 0x1d, 0xca, 0x42,
	0x81, 0xbe, 0x98, 0x8e, 0x2d, 0xa0, 0xb6, 0xc1,
	0xc6, 0xa5, 0x9d, 0xc2, 0x26, 0xc2, 0x86, 0x24,
	0xe1, 0x81, 0x75, 0xe8, 0x51, 0xc9, 0x6b, 0x97,
	0x3d, 0x81, 0xb0, 0x1c, 0xc3, 0x1f, 0x04, 0x78,
	0x34, 0xbc, 0x06, 0xd6, 0xd6, 0xed, 0xf6, 0x20,
	0xd1, 0x84, 0x24, 0x1a, 0x6a, 0xed, 0x8b, 0x63,
	0xa6,                   // 65-byte signature
	0xac,                   // OP_CHECKSIG
	0x00, 0x00, 0x00, 0x00, // Lock time [203]
	0x00, 0x00, 0x00, 0x00, // Expiry [207]
	0x01,                                           // Varint for number of input signature [211]
	0x12, 0x12, 0x12, 0x12, 0x12, 0x12, 0x12, 0x12, // ValueIn [212]
	0x15, 0x15, 0x15, 0x15, // BlockHeight [220]
	0x34, 0x34, 0x34, 0x34, // BlockIndex [224]
	0x07,                                     // Varint for length of signature script [228]
	0x04, 0x31, 0xdc, 0x00, 0x1b, 0x01, 0x62, // Signature script [229]
}

// multiTxPrefixEncoded is the wire encoded bytes for multiTx using protocol
// version 1 and is used in the various tests.
var multiTxPrefixEncoded = []byte{
	0x01, 0x00, 0x01, 0x00, // Version [0]
	0x01,                                           // Varint for number of input transactions [4]
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // [5]
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Previous output hash
	0xff, 0xff, 0xff, 0xff, // Previous output index [37]
	0x00,                   // Previous output tree [41]
	0xff, 0xff, 0xff, 0xff, // Sequence [43]
	0x02,                                           // Varint for number of output transactions [47]
	0x00, 0xf2, 0x05, 0x2a, 0x01, 0x00, 0x00, 0x00, // Transaction amount [48]
	0xab, 0xab, // Script version
	0x43, // Varint for length of pk script [56]
	0x41, // OP_DATA_65 [57]
	0x04, 0xd6, 0x4b, 0xdf, 0xd0, 0x9e, 0xb1, 0xc5,
	0xfe, 0x29, 0x5a, 0xbd, 0xeb, 0x1d, 0xca, 0x42,
	0x81, 0xbe, 0x98, 0x8e, 0x2d, 0xa0, 0xb6, 0xc1,
	0xc6, 0xa5, 0x9d, 0xc2, 0x26, 0xc2, 0x86, 0x24,
	0xe1, 0x81, 0x75, 0xe8, 0x51, 0xc9, 0x6b, 0x97,
	0x3d, 0x81, 0xb0, 0x1c, 0xc3, 0x1f, 0x04, 0x78,
	0x34, 0xbc, 0x06, 0xd6, 0xd6, 0xed, 0xf6, 0x20,
	0xd1, 0x84, 0x24, 0x1a, 0x6a, 0xed, 0x8b, 0x63,
	0xa6,                                           // 65-byte signature
	0xac,                                           // OP_CHECKSIG
	0x00, 0xe1, 0xf5, 0x05, 0x00, 0x00, 0x00, 0x00, // Transaction amount [124]
	0xbc, 0xbc, // Script version
	0x43, // Varint for length of pk script [132]
	0x41, // OP_DATA_65
	0x04, 0xd6, 0x4b, 0xdf, 0xd0, 0x9e, 0xb1, 0xc5,
	0xfe, 0x29, 0x5a, 0xbd, 0xeb, 0x1d, 0xca, 0x42,
	0x81, 0xbe, 0x98, 0x8e, 0x2d, 0xa0, 0xb6, 0xc1,
	0xc6, 0xa5, 0x9d, 0xc2, 0x26, 0xc2, 0x86, 0x24,
	0xe1, 0x81, 0x75, 0xe8, 0x51, 0xc9, 0x6b, 0x97,
	0x3d, 0x81, 0xb0, 0x1c, 0xc3, 0x1f, 0x04, 0x78,
	0x34, 0xbc, 0x06, 0xd6, 0xd6, 0xed, 0xf6, 0x20,
	0xd1, 0x84, 0x24, 0x1a, 0x6a, 0xed, 0x8b, 0x63,
	0xa6,                   // 65-byte signature
	0xac,                   // OP_CHECKSIG
	0x00, 0x00, 0x00, 0x00, // Lock time [198]
	0x00, 0x00, 0x00, 0x00, // Expiry [202]
}

// multiTxWitnessEncoded is the wire encoded bytes for multiTx using protocol version
// 1 and is used in the various tests.
var multiTxWitnessEncoded = []byte{
	0x01, 0x00, 0x02, 0x00, // Version
	0x01,                                           // Varint for number of input signature
	0x12, 0x12, 0x12, 0x12, 0x12, 0x12, 0x12, 0x12, // ValueIn
	0x15, 0x15, 0x15, 0x15, // BlockHeight
	0x34, 0x34, 0x34, 0x34, // BlockIndex
	0x07,                                     // Varint for length of signature script
	0x04, 0x31, 0xdc, 0x00, 0x1b, 0x01, 0x62, // Signature script
}

// multiTxWitnessSigningEncoded is the wire encoded bytes for multiTx using protocol version
// 1 and is used in the various tests.
var multiTxWitnessSigningEncoded = []byte{
	0x01, 0x00, 0x03, 0x00, // Version
	0x01,                                     // Varint for number of input signature
	0x07,                                     // Varint for length of signature script
	0x04, 0x31, 0xdc, 0x00, 0x1b, 0x01, 0x62, // Signature script
}

// multiTxWitnessValueSigningEncoded is the wire encoded bytes for multiTx using protocol version
// 1 and is used in the various tests.
var multiTxWitnessValueSigningEncoded = []byte{
	0x01, 0x00, 0x04, 0x00, // Version
	0x01,                                           // Varint for number of input signature
	0x12, 0x12, 0x12, 0x12, 0x12, 0x12, 0x12, 0x12, // ValueIn
	0x07,                                     // Varint for length of signature script
	0x04, 0x31, 0xdc, 0x00, 0x1b, 0x01, 0x62, // Signature script
}

// multiTxPkScriptLocs is the location information for the public key scripts
// located in multiTx.
var multiTxPkScriptLocs = []int{58, 136}
