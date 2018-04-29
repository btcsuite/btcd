// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript_test

import (
	"bytes"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

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
		txOut.PkScript = decodeHex("51")
		txOut.Value = 0x0000FF00FF00FF00
		tx.AddTxOut(txOut)
	}

	want := decodeHex("4ce2cd042d64e35b36fdbd16aff0d38a5abebff0e5e8f6b6b31" +
		"fcd4ac6957905")
	script := decodeHex("51")

	// Test prefix caching.
	msg1, err := txscript.CalcSignatureHash(script, txscript.SigHashAll, tx, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error %v", err.Error())
	}

	prefixHash := tx.TxHash()
	msg2, err := txscript.CalcSignatureHash(script, txscript.SigHashAll, tx, 0,
		&prefixHash)
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
	msg3, err := txscript.CalcSignatureHash(script, txscript.SigHashAll, tx, 1,
		&prefixHash)
	if err != nil {
		t.Fatalf("unexpected error %v", err.Error())
	}

	if bytes.Equal(msg1, msg3) {
		t.Errorf("for sighash all sig equivalent msgs %x and %x were "+
			"returned when using a cached prefix but different indices",
			msg1,
			msg3)
	}
}
