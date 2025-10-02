package txgraph

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// assertConflicts verifies that the ConflictSet contains exactly the expected
// transaction hashes and no others.
func assertConflicts(t *testing.T, conflicts *ConflictSet, expectedHashes ...*chainhash.Hash) {
	t.Helper()
	require.Len(t, conflicts.Transactions, len(expectedHashes))
	for _, hash := range expectedHashes {
		require.Contains(t, conflicts.Transactions, *hash)
	}
}

// TestGetConflictsNoConflicts verifies that GetConflicts returns an empty
// conflict set when the transaction has no conflicts with existing mempool
// transactions.
func TestGetConflictsNoConflicts(t *testing.T) {
	g := New(DefaultConfig())

	// Add a transaction to the mempool.
	tx1, desc1 := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	// Create a new transaction that spends a different output. This should
	// not conflict with tx1.
	tx2, _ := createTestTx(nil, 1)

	conflicts := g.GetConflicts(tx2)
	require.Empty(t, conflicts.Transactions, "should have no conflicts")
	require.Empty(t, conflicts.Packages, "should have no package conflicts")
}

// TestGetConflictsSingleConflict verifies that GetConflicts correctly
// identifies a single conflicting transaction when the input transaction
// attempts to spend an output already spent in the mempool.
func TestGetConflictsSingleConflict(t *testing.T) {
	g := New(DefaultConfig())

	// Add a transaction that spends from a parent.
	parent, parentDesc := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(parent, parentDesc))

	child, childDesc := createTestTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(child, childDesc))

	// Create a replacement transaction that spends the same parent output.
	// This conflicts with the existing child transaction.
	replacement, _ := createTestTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1,
	)

	conflicts := g.GetConflicts(replacement)
	assertConflicts(t, conflicts, child.Hash())
}

// TestGetConflicts_MultipleConflicts verifies that GetConflicts returns all
// transactions that conflict when multiple inputs spend already-spent outputs.
func TestGetConflictsMultipleConflicts(t *testing.T) {
	g := New(DefaultConfig())

	// Create two parent transactions.
	parent1, desc1 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(parent1, desc1))

	parent2, desc2 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(parent2, desc2))

	// Create two children, each spending from a different parent.
	child1, childDesc1 := createTestTx(
		[]wire.OutPoint{{Hash: *parent1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(child1, childDesc1))

	child2, childDesc2 := createTestTx(
		[]wire.OutPoint{{Hash: *parent2.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(child2, childDesc2))

	// Create a transaction that spends from both parents. This conflicts
	// with both children.
	conflicting, _ := createTestTx(
		[]wire.OutPoint{
			{Hash: *parent1.Hash(), Index: 0},
			{Hash: *parent2.Hash(), Index: 0},
		}, 1,
	)

	conflicts := g.GetConflicts(conflicting)
	assertConflicts(t, conflicts, child1.Hash(), child2.Hash())
}

// TestGetConflicts_WithDescendants verifies that when a transaction conflicts
// with an existing transaction, all descendants of the conflicting transaction
// are also included in the result. This is essential for RBF because
// replacing a transaction invalidates all its descendants.
func TestGetConflictsWithDescendants(t *testing.T) {
	g := New(DefaultConfig())

	// Build a chain: parent -> child -> grandchild.
	parent, parentDesc := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(parent, parentDesc))

	child, childDesc := createTestTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(child, childDesc))

	grandchild, grandchildDesc := createTestTx(
		[]wire.OutPoint{{Hash: *child.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(grandchild, grandchildDesc))

	// Create a replacement that conflicts with the child. This should also
	// include the grandchild in the conflict set since it depends on the
	// child.
	replacement, _ := createTestTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1,
	)

	conflicts := g.GetConflicts(replacement)
	assertConflicts(t, conflicts, child.Hash(), grandchild.Hash())
	require.NotContains(t, conflicts.Transactions, *parent.Hash(), "should not include parent")
}

// TestGetConflicts_ComplexDescendantTree verifies that GetConflicts correctly
// handles complex descendant trees where a conflicting transaction has
// multiple descendants in different branches.
func TestGetConflictsComplexDescendantTree(t *testing.T) {
	g := New(DefaultConfig())

	// Build a tree:
	//   parent
	//     |
	//   child (conflicting)
	//   /  \
	//  gc1 gc2

	parent, parentDesc := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(parent, parentDesc))

	child, childDesc := createTestTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 2,
	)
	require.NoError(t, g.AddTransaction(child, childDesc))

	// Two grandchildren spending from different outputs of child.
	grandchild1, gc1Desc := createTestTx(
		[]wire.OutPoint{{Hash: *child.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(grandchild1, gc1Desc))

	grandchild2, gc2Desc := createTestTx(
		[]wire.OutPoint{{Hash: *child.Hash(), Index: 1}}, 1,
	)
	require.NoError(t, g.AddTransaction(grandchild2, gc2Desc))

	// Create a replacement that conflicts with child.
	replacement, _ := createTestTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1,
	)

	conflicts := g.GetConflicts(replacement)
	assertConflicts(t, conflicts, child.Hash(), grandchild1.Hash(), grandchild2.Hash())
}

// TestGetConflicts_PartialConflicts verifies that when a transaction has
// multiple inputs and only some cause conflicts, GetConflicts returns only
// the conflicting transactions and their descendants.
func TestGetConflictsPartialConflicts(t *testing.T) {
	g := New(DefaultConfig())

	// Create two independent parent-child pairs.
	parent1, desc1 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(parent1, desc1))

	child1, childDesc1 := createTestTx(
		[]wire.OutPoint{{Hash: *parent1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(child1, childDesc1))

	parent2, desc2 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(parent2, desc2))

	// parent2 is not spent yet - this won't conflict.

	// Create a transaction that spends from parent1 (conflicts with
	// child1) and parent2 (no conflict).
	partialConflict, _ := createTestTx(
		[]wire.OutPoint{
			{Hash: *parent1.Hash(), Index: 0},
			{Hash: *parent2.Hash(), Index: 0},
		}, 1,
	)

	conflicts := g.GetConflicts(partialConflict)
	assertConflicts(t, conflicts, child1.Hash())
	require.NotContains(t, conflicts.Transactions, *parent2.Hash())
}
