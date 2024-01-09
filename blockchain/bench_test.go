// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
)

// BenchmarkIsCoinBase performs a simple benchmark against the IsCoinBase
// function.
func BenchmarkIsCoinBase(b *testing.B) {
	tx, _ := btcutil.NewBlock(&Block100000).Tx(1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsCoinBase(tx)
	}
}

// BenchmarkIsCoinBaseTx performs a simple benchmark against the IsCoinBaseTx
// function.
func BenchmarkIsCoinBaseTx(b *testing.B) {
	tx := Block100000.Transactions[1]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsCoinBaseTx(tx)
	}
}

func BenchmarkUtxoFetchMap(b *testing.B) {
	block := Block100000
	transactions := block.Transactions
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		needed := make(map[wire.OutPoint]struct{}, len(transactions))
		for _, tx := range transactions[1:] {
			for _, txIn := range tx.TxIn {
				needed[txIn.PreviousOutPoint] = struct{}{}
			}
		}
	}
}

func BenchmarkUtxoFetchSlices(b *testing.B) {
	block := Block100000
	transactions := block.Transactions
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		needed := make([]wire.OutPoint, 0, len(transactions))
		for _, tx := range transactions[1:] {
			for _, txIn := range tx.TxIn {
				needed = append(needed, txIn.PreviousOutPoint)
			}
		}
	}
}

func BenchmarkAncestor(b *testing.B) {
	height := 1 << 19
	blockNodes := chainedNodes(nil, height)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blockNodes[len(blockNodes)-1].Ancestor(0)
		for j := 0; j <= 19; j++ {
			blockNodes[len(blockNodes)-1].Ancestor(1 << j)
		}
	}
}
