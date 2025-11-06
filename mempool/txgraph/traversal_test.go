package txgraph

import (
	"slices"
	"testing"

	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestIterateBFSWithIncludeStart validates that the IncludeStart option
// correctly controls whether the starting transaction is included in BFS
// traversal results. This is critical for mempool operations where some
// algorithms need to process the anchor transaction itself (e.g., fee
// calculation), while others only need its descendants (e.g., eviction
// checking).
func TestIterateBFSWithIncludeStart(t *testing.T) {
	g := New(DefaultConfig())

	// Create chain: tx1 -> tx2 -> tx3.
	tx1, desc1 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx(
		[]wire.OutPoint{{Hash: *tx2.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	// When IncludeStart is true, the iterator must emit the start node
	// first, followed by its descendants in breadth-first order. This
	// allows callers to process the entire subgraph atomically.
	visited := slices.Collect(g.Iterate(
		WithOrder(TraversalBFS),
		WithStartNode(tx2.Hash()),
		WithIncludeStart(true),
	))

	require.Len(t, visited, 2, "should include start node")
	require.Equal(
		t, *tx2.Hash(), visited[0].TxHash,
		"start node should be first",
	)
	require.Equal(t, *tx3.Hash(), visited[1].TxHash)

	// When IncludeStart is false (default behavior), the iterator skips
	// the start node and only yields descendants. This is useful when
	// checking if removing a transaction would affect other transactions.
	visited = slices.Collect(g.Iterate(
		WithOrder(TraversalBFS),
		WithStartNode(tx2.Hash()),
		WithIncludeStart(false),
	))

	require.Len(t, visited, 1, "should not include start node")
	require.Equal(t, *tx3.Hash(), visited[0].TxHash)
}

// TestIterateDFSWithIncludeStart verifies that depth-first traversal
// respects the IncludeStart option when exploring transaction trees. DFS
// is used in mempool for dependency chain validation where we need to
// explore each branch fully before backtracking, making it essential for
// detecting circular dependencies and validating TRUC topology rules.
func TestIterateDFSWithIncludeStart(t *testing.T) {
	g := New(DefaultConfig())

	// Build a simple tree where tx1 has two children (tx2, tx3).
	// This tests DFS behavior on branching structures rather than
	// linear chains.
	tx1, desc1 := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 1}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	// With IncludeStart=true, the root must appear first, allowing
	// algorithms to validate parent properties before processing
	// children.
	visited := slices.Collect(g.Iterate(
		WithOrder(TraversalDFS),
		WithStartNode(tx1.Hash()),
		WithIncludeStart(true),
		WithDirection(DirectionForward),
	))

	require.Len(t, visited, 3, "should include start node")
	require.Equal(
		t, *tx1.Hash(), visited[0].TxHash,
		"start node should be first",
	)

	// With IncludeStart=false, we get only descendants. This mode is
	// used when validating that child transactions remain valid after
	// hypothetically removing the parent.
	visited = slices.Collect(g.Iterate(
		WithOrder(TraversalDFS),
		WithStartNode(tx1.Hash()),
		WithIncludeStart(false),
		WithDirection(DirectionForward),
	))

	require.Len(t, visited, 2, "should not include start node")

	// Verify we got both children but not the parent.
	seenHashes := make(map[string]bool)
	for _, n := range visited {
		seenHashes[n.TxHash.String()] = true
	}
	require.False(t, seenHashes[tx1.Hash().String()])
	require.True(t, seenHashes[tx2.Hash().String()])
	require.True(t, seenHashes[tx3.Hash().String()])
}

// TestIterateReverseTopoWithFilter validates that reverse topological
// ordering correctly processes transactions from leaves to roots while
// applying filters. This traversal order is critical for block template
// construction where transactions must be evaluated in an order that
// ensures all children are processed before their parents, allowing
// accurate ancestor fee rate calculations.
func TestIterateReverseTopoWithFilter(t *testing.T) {
	g := New(DefaultConfig())

	// Build a dependency chain with varying fee rates to test
	// filtering during topological iteration.
	tx1, desc1 := createTestTx(nil, 1)
	desc1.FeePerKB = 1000
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	desc2.FeePerKB = 5000
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx(
		[]wire.OutPoint{{Hash: *tx2.Hash(), Index: 0}}, 1,
	)
	desc3.FeePerKB = 10000
	require.NoError(t, g.AddTransaction(tx3, desc3))

	// The filter allows selective processing of only high-value
	// transactions, which is used during mempool eviction to prioritize
	// keeping profitable transaction chains.
	highFeeFilter := func(n *TxGraphNode) bool {
		return n.TxDesc.FeePerKB >= 5000
	}

	visited := slices.Collect(g.Iterate(
		WithOrder(TraversalReverseTopo),
		WithFilter(highFeeFilter),
	))

	// Verify only high-fee transactions appear, in reverse topological
	// order (children before parents).
	require.Len(t, visited, 2, "should filter out low-fee transaction")
	require.Equal(
		t, *tx3.Hash(), visited[0].TxHash,
		"tx3 should be first (reverse topo)",
	)
	require.Equal(t, *tx2.Hash(), visited[1].TxHash)
}

// TestIterateWithDirectionBackward verifies that backward traversal walks
// from children to ancestors. This direction is essential for calculating
// the total ancestor set of a transaction, which determines whether adding
// a new transaction would exceed ancestor count/size limits defined in
// mempool policy.
func TestIterateWithDirectionBackward(t *testing.T) {
	g := New(DefaultConfig())

	// Create chain: tx1 -> tx2 -> tx3.
	tx1, desc1 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx(
		[]wire.OutPoint{{Hash: *tx2.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	// Starting from tx3 and moving backward visits all ancestors in
	// breadth-first order. This is how we compute ancestor fees and
	// validate ancestor limits.
	visited := slices.Collect(g.Iterate(
		WithOrder(TraversalBFS),
		WithStartNode(tx3.Hash()),
		WithDirection(DirectionBackward),
		WithIncludeStart(false),
	))

	require.Len(t, visited, 2, "should visit tx2 and tx1")
	require.Equal(
		t, *tx2.Hash(), visited[0].TxHash,
		"tx2 is immediate parent",
	)
	require.Equal(t, *tx1.Hash(), visited[1].TxHash)
}

// TestIterateWithDirectionBoth validates bidirectional traversal, which
// explores both ancestors and descendants from a starting transaction.
// This is used when evicting a transaction from the mempool to identify
// the complete cluster that would be affected, as we must remove all
// descendants while considering ancestor fee implications.
func TestIterateWithDirectionBoth(t *testing.T) {
	g := New(DefaultConfig())

	// Create chain: tx1 -> tx2 -> tx3.
	tx1, desc1 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx(
		[]wire.OutPoint{{Hash: *tx2.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	// From tx2, bidirectional traversal finds both its ancestor (tx1)
	// and descendant (tx3). This gives the complete cluster affected by
	// any change to tx2.
	visited := slices.Collect(g.Iterate(
		WithOrder(TraversalBFS),
		WithStartNode(tx2.Hash()),
		WithDirection(DirectionBoth),
		WithIncludeStart(false),
	))

	require.Len(
		t, visited, 2,
		"should visit both tx1 (parent) and tx3 (child)",
	)
	seenHashes := make(map[string]bool)
	for _, n := range visited {
		seenHashes[n.TxHash.String()] = true
	}
	require.True(t, seenHashes[tx1.Hash().String()])
	require.True(t, seenHashes[tx3.Hash().String()])
	require.False(
		t, seenHashes[tx2.Hash().String()],
		"should not include start",
	)
}

// TestIteratePairsWithOptions validates that IteratePairs correctly emits
// parent-child relationships as pairs, which is essential for CPFP (Child
// Pays For Parent) analysis. By iterating edges rather than nodes, we can
// efficiently compute fee deltas and determine which children are boosting
// low-fee ancestors.
func TestIteratePairsWithOptions(t *testing.T) {
	g := New(DefaultConfig())

	// Build a tree with one parent and two children to test edge
	// enumeration.
	tx1, desc1 := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 1}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	// IteratePairs emits one pair per edge, allowing us to analyze
	// each parent-child relationship independently for fee rate
	// calculations.
	pairs := slices.Collect(g.IteratePairs(
		WithOrder(TraversalDefault),
		WithStartNode(tx1.Hash()),
		WithDirection(DirectionForward),
	))

	require.Len(t, pairs, 2, "should have 2 edges from tx1")

	// Verify each pair represents a valid edge from tx1 to one of its
	// children.
	for _, pair := range pairs {
		require.Equal(t, *tx1.Hash(), pair.Parent.TxHash)
		require.True(t,
			pair.Child.TxHash == *tx2.Hash() ||
				pair.Child.TxHash == *tx3.Hash(),
			"child should be tx2 or tx3",
		)
	}
}

// TestIteratePairsWithFilter validates that filters are applied to
// parent-child pairs, enabling selective analysis of specific
// relationships. This is used in RBF (Replace-By-Fee) scenarios where we
// need to identify which high-value dependencies would be broken by
// replacing a transaction.
func TestIteratePairsWithFilter(t *testing.T) {
	g := New(DefaultConfig())

	// Create two independent parent-child chains with different fee
	// rates to test filtering at the edge level.
	tx1, desc1 := createTestTx(nil, 1)
	desc1.FeePerKB = 10000
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx(nil, 1)
	desc2.FeePerKB = 1000
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	tx4, desc4 := createTestTx(
		[]wire.OutPoint{{Hash: *tx2.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx4, desc4))

	// The filter applies to parent nodes in the pairs, allowing us to
	// focus analysis on edges originating from high-fee transactions.
	highFeeFilter := func(n *TxGraphNode) bool {
		return n.TxDesc.FeePerKB >= 5000
	}

	pairs := slices.Collect(g.IteratePairs(
		WithOrder(TraversalDefault),
		WithFilter(highFeeFilter),
	))

	// Only the edge from high-fee tx1 should appear.
	require.Len(t, pairs, 1, "should filter out low-fee parent edges")
	require.Equal(t, *tx1.Hash(), pairs[0].Parent.TxHash)
	require.Equal(t, *tx3.Hash(), pairs[0].Child.TxHash)
}

// TestIterateBackwardWithMaxDepth ensures that depth limits correctly
// bound backward traversal. This prevents unbounded ancestor walks in
// large transaction chains and enables efficient "bounded ancestor
// search" needed for quick policy checks without traversing the entire
// mempool history.
func TestIterateBackwardWithMaxDepth(t *testing.T) {
	g := New(DefaultConfig())

	// Build a longer chain to test depth limiting.
	tx1, desc1 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx(
		[]wire.OutPoint{{Hash: *tx2.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	tx4, desc4 := createTestTx(
		[]wire.OutPoint{{Hash: *tx3.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx4, desc4))

	// MaxDepth limits how far back we search. This is critical for
	// performance when checking if a transaction has any recent
	// unconfirmed ancestors without walking the entire chain.
	visited := slices.Collect(g.Iterate(
		WithOrder(TraversalBFS),
		WithStartNode(tx4.Hash()),
		WithDirection(DirectionBackward),
		WithMaxDepth(2),
		WithIncludeStart(false),
	))

	// Depth 1 yields tx3, depth 2 yields tx2. tx1 at depth 3 is
	// excluded.
	require.Len(t, visited, 2, "should respect maxDepth")
	require.Equal(t, *tx3.Hash(), visited[0].TxHash)
	require.Equal(t, *tx2.Hash(), visited[1].TxHash)
}

// TestIterateDFSBackwardWithFilter validates depth-first backward
// traversal with filtering on diamond-shaped DAGs. This pattern is
// crucial for analyzing complex dependency structures where a transaction
// has multiple parents, as seen in coinjoin and batched payment scenarios
// where we need to identify which specific ancestor chains meet certain
// criteria.
func TestIterateDFSBackwardWithFilter(t *testing.T) {
	g := New(DefaultConfig())

	// Build a diamond: tx1 branches to tx2 and tx3, which both feed into
	// tx4. This tests filtering when multiple paths exist to ancestors.
	tx1, desc1 := createTestTx(nil, 2)
	desc1.FeePerKB = 1000
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	desc2.FeePerKB = 10000
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 1}}, 1,
	)
	desc3.FeePerKB = 500
	require.NoError(t, g.AddTransaction(tx3, desc3))

	tx4, desc4 := createTestTx([]wire.OutPoint{
		{Hash: *tx2.Hash(), Index: 0},
		{Hash: *tx3.Hash(), Index: 0},
	}, 1)
	require.NoError(t, g.AddTransaction(tx4, desc4))

	// When traversing backward with a filter, we only follow paths
	// through ancestors that pass the predicate. This allows
	// identifying specific "valuable" dependency chains while ignoring
	// low-value branches.
	highFeeFilter := func(n *TxGraphNode) bool {
		return n.TxDesc.FeePerKB >= 5000
	}

	visited := slices.Collect(g.Iterate(
		WithOrder(TraversalDFS),
		WithStartNode(tx4.Hash()),
		WithDirection(DirectionBackward),
		WithFilter(highFeeFilter),
		WithIncludeStart(false),
	))

	// Only tx2 passes the filter. tx3 and tx1 are both too low-fee.
	require.Len(t, visited, 1, "should filter out low-fee nodes")
	require.Equal(t, *tx2.Hash(), visited[0].TxHash)
}