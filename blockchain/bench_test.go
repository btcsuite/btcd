// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcutil"
)

// BenchmarkIsCoinBase performs a simple benchmark against the IsCoinBase
// function.
func BenchmarkIsCoinBase(b *testing.B) {
	tx, _ := btcutil.NewBlock(&Block100000).Tx(1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blockchain.IsCoinBase(tx)
	}
}

// BenchmarkIsCoinBaseTx performs a simple benchmark against the IsCoinBaseTx
// function.
func BenchmarkIsCoinBaseTx(b *testing.B) {
	tx := Block100000.Transactions[1]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blockchain.IsCoinBaseTx(tx)
	}
}
