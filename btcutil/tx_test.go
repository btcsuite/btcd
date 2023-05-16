// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil_test

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/davecgh/go-spew/spew"
)

// TestTx tests the API for Tx.
func TestTx(t *testing.T) {
	testTx := Block100000.Transactions[0]
	tx := btcutil.NewTx(testTx)

	// Ensure we get the same data back out.
	if msgTx := tx.MsgTx(); !reflect.DeepEqual(msgTx, testTx) {
		t.Errorf("MsgTx: mismatched MsgTx - got %v, want %v",
			spew.Sdump(msgTx), spew.Sdump(testTx))
	}

	// Ensure transaction index set and get work properly.
	wantIndex := 0
	tx.SetIndex(0)
	if gotIndex := tx.Index(); gotIndex != wantIndex {
		t.Errorf("Index: mismatched index - got %v, want %v",
			gotIndex, wantIndex)
	}

	// Hash for block 100,000 transaction 0.
	wantHashStr := "8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87"
	wantHash, err := chainhash.NewHashFromStr(wantHashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
	}

	// Request the hash multiple times to test generation and caching.
	for i := 0; i < 2; i++ {
		hash := tx.Hash()
		if !hash.IsEqual(wantHash) {
			t.Errorf("Hash #%d mismatched hash - got %v, want %v", i,
				hash, wantHash)
		}
	}
}

// TestNewTxFromBytes tests creation of a Tx from serialized bytes.
func TestNewTxFromBytes(t *testing.T) {
	// Serialize the test transaction.
	testTx := Block100000.Transactions[0]
	var testTxBuf bytes.Buffer
	err := testTx.Serialize(&testTxBuf)
	if err != nil {
		t.Errorf("Serialize: %v", err)
	}
	testTxBytes := testTxBuf.Bytes()

	// Create a new transaction from the serialized bytes.
	tx, err := btcutil.NewTxFromBytes(testTxBytes)
	if err != nil {
		t.Errorf("NewTxFromBytes: %v", err)
		return
	}

	// Ensure the generated MsgTx is correct.
	if msgTx := tx.MsgTx(); !reflect.DeepEqual(msgTx, testTx) {
		t.Errorf("MsgTx: mismatched MsgTx - got %v, want %v",
			spew.Sdump(msgTx), spew.Sdump(testTx))
	}
}

// TestTxErrors tests the error paths for the Tx API.
func TestTxErrors(t *testing.T) {
	// Serialize the test transaction.
	testTx := Block100000.Transactions[0]
	var testTxBuf bytes.Buffer
	err := testTx.Serialize(&testTxBuf)
	if err != nil {
		t.Errorf("Serialize: %v", err)
	}
	testTxBytes := testTxBuf.Bytes()

	// Truncate the transaction byte buffer to force errors.
	shortBytes := testTxBytes[:4]
	_, err = btcutil.NewTxFromBytes(shortBytes)
	if err != io.EOF {
		t.Errorf("NewTxFromBytes: did not get expected error - "+
			"got %v, want %v", err, io.EOF)
	}
}

// TestTxHasWitness tests the HasWitness() method.
func TestTxHasWitness(t *testing.T) {
	msgTx := Block100000.Transactions[0] // contains witness data
	tx := btcutil.NewTx(msgTx)

	tx.WitnessHash() // Populate the witness hash cache
	tx.HasWitness()  // Should not fail (see btcsuite/btcd#1543)

	if !tx.HasWitness() {
		t.Errorf("HasWitness: got false, want true")
	}

	for _, msgTxWithoutWitness := range Block100000.Transactions[1:] {
		txWithoutWitness := btcutil.NewTx(msgTxWithoutWitness)
		if txWithoutWitness.HasWitness() {
			t.Errorf("HasWitness: got false, want true")
		}
	}
}

// TestTxWitnessHash tests the WitnessHash() method.
func TestTxWitnessHash(t *testing.T) {
	msgTx := Block100000.Transactions[0] // contains witness data
	tx := btcutil.NewTx(msgTx)

	if tx.WitnessHash().IsEqual(tx.Hash()) {
		t.Errorf("WitnessHash: witness hash and tx id must NOT be same - "+
			"got %v, want %v", tx.WitnessHash(), tx.Hash())
	}

	for _, msgTxWithoutWitness := range Block100000.Transactions[1:] {
		txWithoutWitness := btcutil.NewTx(msgTxWithoutWitness)
		if !txWithoutWitness.WitnessHash().IsEqual(txWithoutWitness.Hash()) {
			t.Errorf("WitnessHash: witness hash and tx id must be same - "+
				"got %v, want %v", txWithoutWitness.WitnessHash(), txWithoutWitness.Hash())
		}
	}
}
