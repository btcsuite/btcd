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
	calcMerkleRoot := CalcMerkleRoot(block.Transactions(), false)
	merkleStoreTree := BuildMerkleTreeStore(block.Transactions(), false)
	merkleStoreRoot := merkleStoreTree[len(merkleStoreTree)-1]

	if calcMerkleRoot != *merkleStoreRoot {
		t.Fatalf("BuildMerkleTreeStore root does not match "+
			"CalcMerkleRoot, store: %v calc: %v", merkleStoreRoot,
			calcMerkleRoot)
	}

	wantMerkle := &Block100000.Header.MerkleRoot
	if !wantMerkle.IsEqual(&calcMerkleRoot) {
		t.Errorf("BuildMerkleTreeStore: merkle root mismatch - "+
			"got %v, want %v", calcMerkleRoot, wantMerkle)
	}
}

// TestMerkleBlock46559 computes the merkle root of block 46559. The number of
// transactions in this blocks forces the case where a left leaf must be hashed
// with itself, since the block only has three transactions.
func TestRollingMerkleBlock46559(t *testing.T) {
	merkle := NewRollingMerkleTree(3)

	coinbase, _ := chainhash.NewHashFromStr(
		"70e208b60ef5156b5f48d64999115195412b3fec607065099effc17a44af8902",
	)
	tx1, _ := chainhash.NewHashFromStr(
		"5a9e2d301cd7d08b1a920d262d4b0ea4e3fd0a4b4e28c8e104470f031d79f787",
	)
	tx2, _ := chainhash.NewHashFromStr(
		"c945e8e02aaccb74f0a12fac98ffeb57d9e3f235d1819df28705690e1d01312f",
	)

	merkle.Push(coinbase)
	merkle.Push(tx1)
	merkle.Push(tx2)

	root := merkle.Root()

	expRoot, _ := chainhash.NewHashFromStr(
		"c6f00b209da9234b18f8995aad9aaddd8bc5bcd3d5455d8c87cb942265ab5464",
	)
	if root != *expRoot {
		t.Fatalf("root mismatch, want: %v, got %v", *expRoot, root)
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
