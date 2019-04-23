// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestMerkle tests the BuildMerkleTreeStore API.
func TestMerkle(t *testing.T) {
	block := btcutil.NewBlock(&Block100000)
	calcMerkleRoot := CalcMerkleRoot(block.Transactions(), false)
	merkleStoreTree := BuildMerkleTreeStore(block.Transactions(), false)
	merkleStoreRoot := merkleStoreTree[len(merkleStoreTree)-1]

	require.Equal(t, *merkleStoreRoot, calcMerkleRoot)

	wantMerkle := &Block100000.Header.MerkleRoot
	if !wantMerkle.IsEqual(&calcMerkleRoot) {
		t.Errorf("BuildMerkleTreeStore: merkle root mismatch - "+
			"got %v, want %v", calcMerkleRoot, wantMerkle)
	}
}

func makeHashes(size int) []*chainhash.Hash {
	var hashes = make([]*chainhash.Hash, size)
	for i := range hashes {
		hashes[i] = new(chainhash.Hash)
	}
	return hashes
}

func makeTxs(size int) []*btcutil.Tx {
	var txs = make([]*btcutil.Tx, size)
	for i := range txs {
		tx := btcutil.NewTx(wire.NewMsgTx(2))
		tx.Hash()
		txs[i] = tx
	}
	return txs
}

// BenchmarkRollingMerkle benches the RollingMerkleTree while varying the number
// of leaves pushed to the tree.
func BenchmarkRollingMerkle(b *testing.B) {
	sizes := []int{
		1000,
		2000,
		4000,
		8000,
		16000,
		32000,
	}

	for _, size := range sizes {
		txs := makeTxs(size)
		name := fmt.Sprintf("%d", size)
		b.Run(name, func(b *testing.B) {
			benchmarkRollingMerkle(b, txs)
		})
	}
}

// BenchmarkMerkle benches the BuildMerkleTreeStore while varying the number
// of leaves pushed to the tree.
func BenchmarkMerkle(b *testing.B) {
	sizes := []int{
		1000,
		2000,
		4000,
		8000,
		16000,
		32000,
	}

	for _, size := range sizes {
		txs := makeTxs(size)
		name := fmt.Sprintf("%d", size)
		b.Run(name, func(b *testing.B) {
			benchmarkMerkle(b, txs)
		})
	}
}

func benchmarkRollingMerkle(b *testing.B, txs []*btcutil.Tx) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		CalcMerkleRoot(txs, false)
	}
}

func benchmarkMerkle(b *testing.B, txs []*btcutil.Tx) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		BuildMerkleTreeStore(txs, false)
	}
}
