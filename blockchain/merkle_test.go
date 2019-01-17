// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcutil"
)

// TestMerkle tests the BuildMerkleTreeStore API.
func TestMerkle(t *testing.T) {
	block := btcutil.NewBlock(&Block100000)
	merkles := BuildMerkleTreeStore(block.Transactions(), false)
	calculatedMerkleRoot := merkles[len(merkles)-1]
	wantMerkle := &Block100000.Header.MerkleRoot
	if !wantMerkle.IsEqual(calculatedMerkleRoot) {
		t.Errorf("BuildMerkleTreeStore: merkle root mismatch - "+
			"got %v, want %v", calculatedMerkleRoot, wantMerkle)
	}
}

func makeHashes(size int) []*chainhash.Hash {
	var hashes = make([]*chainhash.Hash, size)
	for i := range hashes {
		hashes[i] = new(chainhash.Hash)
	}
	return hashes
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

	hashes := make([][]*chainhash.Hash, len(sizes))
	for i, size := range sizes {
		hashes[i] = makeHashes(size)
	}

	for i, size := range sizes {
		name := fmt.Sprintf("%d", size)
		b.Run(name, func(b *testing.B) {
			benchmarkRollingMerkleSlice(b, hashes[i])
		})
	}
}

var root chainhash.Hash

func benchmarkRollingMerkleSlice(b *testing.B, hashes []*chainhash.Hash) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		merkle := NewRollingMerkleTree(len(hashes))
		for _, hash := range hashes {
			merkle.Push(hash)
		}
		root = merkle.Root()
	}
}
