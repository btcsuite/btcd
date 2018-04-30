// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// TestVarIntSerializeSize ensures the serialize size for variable length
// integers works as intended.
func TestVarIntSerializeSize(t *testing.T) {
	tests := []struct {
		val  uint64 // Value to get the serialized size for
		size int    // Expected serialized size
	}{

		{0, 1},                  // Single byte encoded
		{0xfc, 1},               // Max single byte encoded
		{0xfd, 3},               // Min 3-byte encoded
		{0xffff, 3},             // Max 3-byte encoded
		{0x10000, 5},            // Min 5-byte encoded
		{0xffffffff, 5},         // Max 5-byte encoded
		{0x100000000, 9},        // Min 9-byte encoded
		{0xffffffffffffffff, 9}, // Max 9-byte encoded
	}

	for i, test := range tests {
		serializedSize := varIntSerializeSize(test.val)
		if serializedSize != test.size {
			t.Errorf("varIntSerializeSize #%d got: %d, want: %d", i,
				serializedSize, test.size)
			continue
		}
	}
}

// TestPutVarInt ensures encoding variable length integers works as intended.
func TestPutVarInt(t *testing.T) {
	tests := []struct {
		val     uint64 // Value to encode
		encoded []byte // expected encoding
	}{

		{0, hexToBytes("00")},                                  // Single byte
		{0xfc, hexToBytes("fc")},                               // Max single
		{0xfd, hexToBytes("fdfd00")},                           // Min 3-byte
		{0xffff, hexToBytes("fdffff")},                         // Max 3-byte
		{0x10000, hexToBytes("fe00000100")},                    // Min 5-byte
		{0xffffffff, hexToBytes("feffffffff")},                 // Max 5-byte
		{0x100000000, hexToBytes("ff0000000001000000")},        // Min 9-byte
		{0xffffffffffffffff, hexToBytes("ffffffffffffffffff")}, // Max 9-byte
	}

	for i, test := range tests {
		encoded := make([]byte, varIntSerializeSize(test.val))
		gotBytesWritten := putVarInt(encoded, test.val)
		if !bytes.Equal(encoded, test.encoded) {
			t.Errorf("putVarInt #%d\n got: %x want: %x", i, encoded,
				test.encoded)
			continue
		}
		if gotBytesWritten != len(test.encoded) {
			t.Errorf("putVarInt: did not get expected number of bytes written "+
				"for %d - got %d, want %d", test.val, gotBytesWritten,
				len(test.encoded))
			continue
		}
	}
}

// TestCalcSignatureHash does some rudimentary testing of msg hash calculation.
func TestCalcSignatureHash(t *testing.T) {
	tx := new(wire.MsgTx)
	tx.SerType = wire.TxSerializeFull
	tx.Version = 1
	for i := 0; i < 3; i++ {
		txIn := new(wire.TxIn)
		txIn.Sequence = 0xFFFFFFFF
		txIn.PreviousOutPoint.Hash = chainhash.HashH([]byte{byte(i)})
		txIn.PreviousOutPoint.Index = uint32(i)
		txIn.PreviousOutPoint.Tree = int8(0)
		tx.AddTxIn(txIn)
	}
	for i := 0; i < 2; i++ {
		txOut := new(wire.TxOut)
		txOut.PkScript = hexToBytes("51")
		txOut.Value = 0x0000FF00FF00FF00
		tx.AddTxOut(txOut)
	}

	want := hexToBytes("4ce2cd042d64e35b36fdbd16aff0d38a5abebff0e5e8f6b6b" +
		"31fcd4ac6957905")
	script := hexToBytes("51")

	// Test prefix caching.
	msg1, err := CalcSignatureHash(script, SigHashAll, tx, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error %v", err.Error())
	}

	prefixHash := tx.TxHash()
	msg2, err := CalcSignatureHash(script, SigHashAll, tx, 0, &prefixHash)
	if err != nil {
		t.Fatalf("unexpected error %v", err.Error())
	}

	if !bytes.Equal(msg1, want) {
		t.Errorf("for sighash all sig noncached wrong msg -- got %x, want %x",
			msg1,
			want)
	}
	if !bytes.Equal(msg2, want) {
		t.Errorf("for sighash all sig cached wrong msg -- got %x, want %x",
			msg1,
			want)
	}
	if !bytes.Equal(msg1, msg2) {
		t.Errorf("for sighash all sig non-equivalent msgs %x and %x were "+
			"returned when using a cached prefix",
			msg1,
			msg2)
	}

	// Move the index and make sure that we get a whole new hash, despite
	// using the same TxOuts.
	msg3, err := CalcSignatureHash(script, SigHashAll, tx, 1, &prefixHash)
	if err != nil {
		t.Fatalf("unexpected error %v", err.Error())
	}

	if bytes.Equal(msg1, msg3) {
		t.Errorf("for sighash all sig equivalent msgs %x and %x were "+
			"returned when using a cached prefix but different indices",
			msg1, msg3)
	}
}
