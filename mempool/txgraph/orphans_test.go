package txgraph

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestGetOrphansNoPredicate verifies that orphan detection correctly
// identifies transactions without in-graph parents when no external
// confirmation predicate is provided. This ensures the mempool can
// distinguish between transactions that form complete chains versus those
// awaiting unknown parents.
func TestGetOrphansNoPredicate(t *testing.T) {
	g := New(DefaultConfig())
	gen := newTxGenerator()

	// Create a rootless transaction to serve as a graph root, simulating
	// a transaction that spends confirmed outputs.
	tx1, desc1 := gen.createTx(nil, 2)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	// Create a child transaction to establish that having an in-graph
	// parent excludes a transaction from being considered an orphan.
	tx2, desc2 := gen.createTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	// Create a transaction with a parent that doesn't exist in the graph
	// to verify it's correctly identified as an orphan.
	unknownHash := chainhash.Hash{}
	for i := range unknownHash {
		unknownHash[i] = byte(i)
	}
	tx3, desc3 := gen.createTx(
		[]wire.OutPoint{{Hash: unknownHash, Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	// Without a predicate, only graph topology matters: transactions
	// without in-graph parents are orphans.
	orphans := g.GetOrphans(nil)
	require.Len(
		t, orphans, 2,
		"should have 2 parentless transactions (tx1 and tx3)",
	)

	// Build a hash set to enable efficient lookup verification of which
	// transactions were classified as orphans.
	orphanHashes := make(map[chainhash.Hash]bool)
	for _, orphan := range orphans {
		orphanHashes[orphan.TxHash] = true
	}
	require.True(t, orphanHashes[*tx1.Hash()])
	require.True(t, orphanHashes[*tx3.Hash()])
	require.False(t, orphanHashes[*tx2.Hash()])
}

// TestGetOrphansWithPredicate verifies that providing a confirmation
// predicate allows the mempool to distinguish between transactions spending
// confirmed versus unconfirmed outputs, which is essential for package
// acceptance logic where only truly orphaned transactions need special
// handling.
func TestGetOrphansWithPredicate(t *testing.T) {
	g := New(DefaultConfig())
	gen := newTxGenerator()

	confirmedHash := chainhash.Hash{9, 9, 9}

	// Create a transaction spending a confirmed output, which should not
	// be treated as an orphan despite having no in-graph parent.
	tx1, desc1 := gen.createTx(
		[]wire.OutPoint{{Hash: confirmedHash, Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	unconfirmedHash := chainhash.Hash{1, 2, 3}

	// Create a transaction spending an unconfirmed output not in the
	// graph, which should be identified as a true orphan.
	tx2, desc2 := gen.createTx(
		[]wire.OutPoint{{Hash: unconfirmedHash, Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	// Create a child transaction to verify that in-graph parents
	// continue to exclude transactions from orphan status.
	tx3, desc3 := gen.createTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	// This predicate simulates blockchain state by declaring which
	// outputs exist in the UTXO set.
	isConfirmed := func(outpoint wire.OutPoint) bool {
		return outpoint.Hash == confirmedHash
	}

	orphans := g.GetOrphans(isConfirmed)
	require.Len(
		t, orphans, 1, "only tx2 should be orphan (unconfirmed input)",
	)
	require.Equal(t, *tx2.Hash(), orphans[0].TxHash)
}

// TestGetOrphansAllConfirmed verifies the critical distinction that
// transactions spending only confirmed outputs are never orphans, even when
// they lack in-graph parents. This ensures the mempool correctly handles
// package relay where root transactions spending confirmed outputs don't
// need special orphan processing.
func TestGetOrphansAllConfirmed(t *testing.T) {
	g := New(DefaultConfig())
	gen := newTxGenerator()

	confirmedHash := chainhash.Hash{9, 9, 9}

	// Create a transaction that has no in-graph parent but spends a
	// confirmed output, testing the predicate overrides topology-based
	// orphan detection.
	tx1, desc1 := gen.createTx(
		[]wire.OutPoint{{Hash: confirmedHash, Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	// The predicate declares this output as confirmed, meaning it exists
	// in the UTXO set and doesn't need a parent transaction.
	isConfirmed := func(outpoint wire.OutPoint) bool {
		return outpoint.Hash == confirmedHash
	}

	// Despite lacking an in-graph parent, the confirmed input means this
	// transaction is not awaiting any missing dependencies.
	orphans := g.GetOrphans(isConfirmed)
	require.Len(
		t, orphans, 0, "tx1 should not be an orphan (confirmed input)",
	)
}

// TestGetOrphansMultipleInputs verifies that orphan detection uses AND
// semantics for multiple inputs: a transaction is an orphan if ANY input is
// unconfirmed and not in the graph. This matters for package relay where
// transactions with partially confirmed inputs still need their unconfirmed
// parent transactions included in the package.
func TestGetOrphansMultipleInputs(t *testing.T) {
	g := New(DefaultConfig())
	gen := newTxGenerator()

	confirmedHash := chainhash.Hash{9, 9, 9}
	unconfirmedHash := chainhash.Hash{1, 2, 3}

	// Create a transaction with mixed input types to verify that the
	// presence of even one unconfirmed input makes it an orphan.
	tx1, desc1 := gen.createTx([]wire.OutPoint{
		{Hash: confirmedHash, Index: 0},
		{Hash: unconfirmedHash, Index: 0},
	}, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	// The predicate only recognizes one of the two inputs as confirmed.
	isConfirmed := func(outpoint wire.OutPoint) bool {
		return outpoint.Hash == confirmedHash
	}

	// The transaction is an orphan because it's waiting on the
	// unconfirmed input, regardless of the confirmed input.
	orphans := g.GetOrphans(isConfirmed)
	require.Len(
		t, orphans, 1, "should be orphan due to unconfirmed input",
	)
	require.Equal(t, *tx1.Hash(), orphans[0].TxHash)
}

// TestIterateOrphansNoPredicate verifies that the iterator-based orphan
// detection provides the same semantics as batch retrieval but enables
// memory-efficient processing of large orphan sets without materializing
// the entire collection.
func TestIterateOrphansNoPredicate(t *testing.T) {
	g := New(DefaultConfig())
	gen := newTxGenerator()

	// Create a rootless transaction to establish one orphan candidate.
	tx1, desc1 := gen.createTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	// Create a child transaction to verify the iterator correctly
	// excludes transactions with in-graph parents.
	tx2, desc2 := gen.createTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	// Create another orphan with an unknown parent to test that the
	// iterator finds all parentless transactions.
	unknownHash := chainhash.Hash{1, 2, 3}
	tx3, desc3 := gen.createTx(
		[]wire.OutPoint{{Hash: unknownHash, Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	// Collect orphans via iteration, which provides the same results as
	// GetOrphans but allows early termination.
	var orphans []*TxGraphNode
	for orphan := range g.IterateOrphans(nil) {
		orphans = append(orphans, orphan)
	}
	require.Len(
		t, orphans, 2, "should iterate over 2 parentless transactions",
	)

	// Build a hash set to verify that iteration found the correct
	// orphans and excluded the child transaction.
	orphanHashes := make(map[chainhash.Hash]bool)
	for _, orphan := range orphans {
		orphanHashes[orphan.TxHash] = true
	}
	require.True(t, orphanHashes[*tx1.Hash()])
	require.True(t, orphanHashes[*tx3.Hash()])
	require.False(t, orphanHashes[*tx2.Hash()])
}

// TestIterateOrphansWithPredicate verifies that the iterator correctly
// applies confirmation predicates during traversal, enabling efficient
// streaming identification of transactions that need parent resolution
// during package relay without buffering all candidates.
func TestIterateOrphansWithPredicate(t *testing.T) {
	g := New(DefaultConfig())
	gen := newTxGenerator()

	confirmedHash := chainhash.Hash{9, 9, 9}
	unconfirmedHash := chainhash.Hash{1, 2, 3}

	// Create a transaction spending a confirmed output to verify the
	// predicate excludes it from iteration results.
	tx1, desc1 := gen.createTx(
		[]wire.OutPoint{{Hash: confirmedHash, Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	// Create a transaction spending an unconfirmed output to ensure the
	// iterator identifies it as requiring parent resolution.
	tx2, desc2 := gen.createTx(
		[]wire.OutPoint{{Hash: unconfirmedHash, Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	// The predicate simulates UTXO set lookup during iteration.
	isConfirmed := func(outpoint wire.OutPoint) bool {
		return outpoint.Hash == confirmedHash
	}

	var orphans []*TxGraphNode
	for orphan := range g.IterateOrphans(isConfirmed) {
		orphans = append(orphans, orphan)
	}
	require.Len(
		t, orphans, 1,
		"should iterate over 1 orphan with unconfirmed input",
	)
	require.Equal(t, *tx2.Hash(), orphans[0].TxHash)
}

// TestIterateOrphansEarlyStop verifies that the iterator properly supports
// early termination via break, which is critical for implementing efficient
// resource-limited operations like "find first N orphans" without
// traversing the entire graph.
func TestIterateOrphansEarlyStop(t *testing.T) {
	g := New(DefaultConfig())
	gen := newTxGenerator()

	// Create multiple orphan transactions to verify early termination
	// prevents processing all of them.
	for i := 0; i < 5; i++ {
		tx, desc := gen.createTx(nil, 1)
		require.NoError(t, g.AddTransaction(tx, desc))
	}

	// Break out of iteration early to demonstrate that the iterator
	// doesn't force full traversal of the orphan set.
	count := 0
	for range g.IterateOrphans(nil) {
		count++
		if count >= 2 {
			break
		}
	}

	require.Equal(t, 2, count, "should stop iteration after 2 orphans")
}

// TestGetOrphansEmptyGraph verifies the boundary condition that an empty
// graph correctly reports no orphans, ensuring the detection logic handles
// degenerate cases that may occur during mempool initialization or after
// block acceptance clears all transactions.
func TestGetOrphansEmptyGraph(t *testing.T) {
	g := New(DefaultConfig())

	orphans := g.GetOrphans(nil)
	require.Len(t, orphans, 0, "empty graph should have no orphans")
}

// TestIterateOrphansEmptyGraph verifies the iterator's boundary condition
// handling by ensuring iteration over an empty graph immediately completes
// without yielding any values, which is essential for correct behavior in
// newly initialized or fully cleared mempools.
func TestIterateOrphansEmptyGraph(t *testing.T) {
	g := New(DefaultConfig())

	var orphans []*TxGraphNode
	for orphan := range g.IterateOrphans(nil) {
		orphans = append(orphans, orphan)
	}
	require.Len(t, orphans, 0, "empty graph should yield no orphans")
}

// TestGetOrphansNoOrphans verifies the case where graph topology changes
// (transaction removal) would create orphans, but a confirmation predicate
// indicates the missing parent is now in the blockchain. This models the
// scenario where a block confirms a transaction that had children in the
// mempool.
func TestGetOrphansNoOrphans(t *testing.T) {
	g := New(DefaultConfig())
	gen := newTxGenerator()

	// Create a parent transaction that will be removed to simulate block
	// confirmation.
	tx1, desc1 := gen.createTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	// Create a child transaction that will lose its in-graph parent but
	// should not become an orphan if the parent is confirmed.
	tx2, desc2 := gen.createTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	// Simulate block confirmation by declaring the removed transaction's
	// outputs as now existing in the UTXO set.
	isConfirmed := func(outpoint wire.OutPoint) bool {
		return outpoint.Hash == *tx1.Hash()
	}

	require.NoError(t, g.RemoveTransaction(*tx1.Hash()))

	orphans := g.GetOrphans(isConfirmed)
	require.Len(
		t, orphans, 0, "tx2 should not be orphan (confirmed input)",
	)
}

// TestGetOrphansAllInputsConfirmed verifies that when a transaction has
// multiple inputs, the AND semantics work in reverse: a transaction is NOT
// an orphan if ALL inputs are confirmed. This ensures transactions
// combining multiple confirmed outputs don't incorrectly get flagged for
// special orphan handling.
func TestGetOrphansAllInputsConfirmed(t *testing.T) {
	g := New(DefaultConfig())
	gen := newTxGenerator()

	confirmedHash1 := chainhash.Hash{1, 1, 1}
	confirmedHash2 := chainhash.Hash{2, 2, 2}

	// Create a transaction spending multiple confirmed outputs to verify
	// that satisfying all inputs excludes it from orphan status.
	tx1, desc1 := gen.createTx([]wire.OutPoint{
		{Hash: confirmedHash1, Index: 0},
		{Hash: confirmedHash2, Index: 0},
	}, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	// The predicate confirms both inputs exist in the UTXO set.
	isConfirmed := func(outpoint wire.OutPoint) bool {
		return outpoint.Hash == confirmedHash1 ||
			outpoint.Hash == confirmedHash2
	}

	orphans := g.GetOrphans(isConfirmed)
	require.Len(
		t, orphans, 0, "should not be orphan (all inputs confirmed)",
	)
}