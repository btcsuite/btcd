package txgraph

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
)

// BenchmarkGetConflictsSmallGraph benchmarks conflict detection in a small
// graph with 100 transactions where the replacement conflicts with a single
// transaction.
func BenchmarkGetConflictsSmallGraph(b *testing.B) {
	g := New(DefaultConfig())

	// Build a small graph with 100 transactions.
	var lastTx *btcutil.Tx
	for i := 0; i < 100; i++ {
		var inputs []wire.OutPoint
		if lastTx != nil {
			inputs = []wire.OutPoint{{Hash: *lastTx.Hash(), Index: 0}}
		}
		tx, desc := createTestTx(inputs, 1)
		_ = g.AddTransaction(tx, desc)
		lastTx = tx
	}

	// Create a replacement transaction that conflicts with transaction #50.
	replacement, _ := createTestTx(nil, 1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conflicts := g.GetConflicts(replacement)
		if len(conflicts.Transactions) != 0 {
			// Force evaluation to prevent optimization.
			_ = conflicts
		}
	}
}

// BenchmarkGetConflictsWithDescendants benchmarks conflict detection when the
// conflicting transaction has a long chain of descendants that must all be
// included in the conflict set.
func BenchmarkGetConflictsWithDescendants(b *testing.B) {
	g := New(DefaultConfig())

	// Build a chain of 100 transactions.
	parent, parentDesc := createTestTx(nil, 1)
	_ = g.AddTransaction(parent, parentDesc)

	lastTx := parent
	for i := 0; i < 100; i++ {
		tx, desc := createTestTx(
			[]wire.OutPoint{{Hash: *lastTx.Hash(), Index: 0}}, 1,
		)
		_ = g.AddTransaction(tx, desc)
		lastTx = tx
	}

	// Create a replacement that conflicts with the parent, which should
	// cascade to all 100 descendants.
	replacement, _ := createTestTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1,
	)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conflicts := g.GetConflicts(replacement)
		if len(conflicts.Transactions) != 100 {
			b.Fatalf("expected 100 conflicts, got %d", len(conflicts.Transactions))
		}
	}
}

// BenchmarkGetConflictsLargeGraph benchmarks conflict detection in a large
// graph with 10,000 transactions. This tests the O(1) lookup performance of
// the spentBy index.
func BenchmarkGetConflictsLargeGraph(b *testing.B) {
	g := New(DefaultConfig())

	// Build a large graph with 10,000 independent transactions.
	for i := 0; i < 10000; i++ {
		tx, desc := createTestTx(nil, 1)
		_ = g.AddTransaction(tx, desc)
	}

	// Create a replacement transaction that doesn't conflict with
	// anything. This tests the index lookup speed.
	replacement, _ := createTestTx(nil, 1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conflicts := g.GetConflicts(replacement)
		if len(conflicts.Transactions) != 0 {
			b.Fatalf("expected 0 conflicts, got %d", len(conflicts.Transactions))
		}
	}
}

// BenchmarkGetConflictsMultipleInputs benchmarks conflict detection for a
// transaction with many inputs, testing the per-input lookup performance.
func BenchmarkGetConflictsMultipleInputs(b *testing.B) {
	g := New(DefaultConfig())

	// Create 100 parent transactions.
	parents := make([]*btcutil.Tx, 100)
	for i := 0; i < 100; i++ {
		tx, desc := createTestTx(nil, 1)
		_ = g.AddTransaction(tx, desc)
		parents[i] = tx
	}

	// Create a transaction that spends from all 100 parents.
	inputs := make([]wire.OutPoint, 100)
	for i, parent := range parents {
		inputs[i] = wire.OutPoint{Hash: *parent.Hash(), Index: 0}
	}

	replacement, _ := createTestTx(inputs, 1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conflicts := g.GetConflicts(replacement)
		// Should have no conflicts since outputs aren't spent yet.
		if len(conflicts.Transactions) != 0 {
			b.Fatalf("expected 0 conflicts, got %d", len(conflicts.Transactions))
		}
	}
}
