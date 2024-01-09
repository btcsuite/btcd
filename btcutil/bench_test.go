package btcutil_test

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

var (
	bencHash *chainhash.Hash
)

// BenchmarkTxHash benchmarks the performance of calculating the hash of a
// transaction.
func BenchmarkTxHash(b *testing.B) {
	// Make a new block from the test block, we'll then call the Bytes
	// function to cache the serialized block. Afterwards we all
	// Transactions to populate the serialization cache.
	testBlock := btcutil.NewBlock(&Block100000)
	_, _ = testBlock.Bytes()

	// The second transaction in the block has no witness data. The first
	// does however.
	testTx := testBlock.Transactions()[1]
	testTx2 := testBlock.Transactions()[0]

	// Run a benchmark for the portion that needs to strip the non-witness
	// data from the transaction.
	b.Run("tx_hash_has_witness", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		var txHash *chainhash.Hash
		for i := 0; i < b.N; i++ {
			txHash = testTx2.Hash()
		}

		bencHash = txHash
	})

	// Next, run it for the portion that can just hash the bytes directly.
	b.Run("tx_hash_no_witness", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		var txHash *chainhash.Hash
		for i := 0; i < b.N; i++ {
			txHash = testTx.Hash()
		}

		bencHash = txHash
	})

}

// BenchmarkTxWitnessHash benchmarks the performance of calculating the hash of
// a transaction.
func BenchmarkTxWitnessHash(b *testing.B) {
	// Make a new block from the test block, we'll then call the Bytes
	// function to cache the serialized block. Afterwards we all
	// Transactions to populate the serialization cache.
	testBlock := btcutil.NewBlock(&Block100000)
	_, _ = testBlock.Bytes()

	// The first transaction in the block has been modified to have witness
	// data.
	testTx := testBlock.Transactions()[0]

	b.ResetTimer()
	b.ReportAllocs()

	var txHash *chainhash.Hash
	for i := 0; i < b.N; i++ {
		txHash = testTx.WitnessHash()
	}

	bencHash = txHash

}
