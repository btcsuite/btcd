// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// TestMerkle tests the BuildMerkleTreeStore API.
func TestMerkle(t *testing.T) {
	block := btcutil.NewBlock(&Block100000)
	calcMerkleRoot := BuildMerkleTreeStore(block.Transactions(), false)
	wantMerkle := &Block100000.Header.MerkleRoot
	if !wantMerkle.IsEqual(calcMerkleRoot) {
		t.Errorf("BuildMerkleTreeStore: merkle root mismatch - "+
			"got %v, want %v", calcMerkleRoot, wantMerkle)
	}
}

var root *chainhash.Hash

func BenchmarkMerkle(b *testing.B) {
	txs := make([]*btcutil.Tx, 0, b.N)
	for i := 0; i < b.N; i++ {
		tx := btcutil.NewTx(wire.NewMsgTx(2))
		tx.Hash()
		txs = append(txs, tx)
	}

	b.ResetTimer()
	b.ReportAllocs()

	root = BuildMerkleTreeStore(txs, false)
}

func BenchmarkRollingMerkle(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	merkle := NewRollingMerkleTree(b.N)
	for i := 0; i < b.N; i++ {
		var zeroHash chainhash.Hash
		merkle.Push(&zeroHash)
	}
	root = merkle.Root()
}
