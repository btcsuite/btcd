package txgraph

import (
	"slices"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestIteratorTraversalMethods verifies that different traversal orders
// (ancestors, descendants, fee rate, topological) produce correct results.
// The iterator patterns are critical for mempool operations: ancestor/
// descendant traversal for policy limits, fee rate sorting for mining, and
// topological ordering for block template construction.
func TestIteratorTraversalMethods(t *testing.T) {
	g := New(DefaultConfig())

	// Create a diamond-shaped DAG to test multiple traversal paths. This
	// structure ensures that traversal algorithms handle nodes with
	// multiple parents and children correctly.
	//     tx1
	//    /   \
	//  tx2   tx3
	//    \  /
	//    tx4
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

	tx4, desc4 := createTestTx([]wire.OutPoint{
		{Hash: *tx2.Hash(), Index: 0},
		{Hash: *tx3.Hash(), Index: 0},
	}, 1)
	require.NoError(t, g.AddTransaction(tx4, desc4))

	t.Run("Ancestors", func(t *testing.T) {
		tx4Node, exists := g.GetNode(*tx4.Hash())
		require.True(t, exists, "tx4 node should exist")
		require.Len(t, tx4Node.Parents, 2, "tx4 should have 2 parents")

		// Ancestor traversal should find all transactions that tx4
		// depends on, including both direct parents (tx2, tx3) and the
		// grandparent (tx1). This traversal is used for enforcing BIP
		// 125 ancestor count and size limits.
		count := 0
		ancestors := make(map[chainhash.Hash]bool)
		for node := range g.Iterate(
			WithOrder(TraversalAncestors),
			WithStartNode(tx4.Hash()),
		) {
			t.Logf("Visited ancestor: %v", node.TxHash)
			ancestors[node.TxHash] = true
			count++
		}
		require.Equal(t, 3, count, "Should have 3 ancestors")
		require.True(t, ancestors[*tx1.Hash()])
		require.True(t, ancestors[*tx2.Hash()])
		require.True(t, ancestors[*tx3.Hash()])
	})

	t.Run("Descendants", func(t *testing.T) {
		// Descendant traversal should find all transactions that depend
		// on tx1, either directly or transitively. This is used for RBF
		// validation where we need to compute the total fees of all
		// transactions that would be evicted by a replacement.
		count := 0
		descendants := make(map[chainhash.Hash]bool)
		for node := range g.Iterate(
			WithOrder(TraversalDescendants),
			WithStartNode(tx1.Hash()),
		) {
			descendants[node.TxHash] = true
			count++
		}
		require.Equal(t, 3, count, "Should have 3 descendants")
		require.True(t, descendants[*tx2.Hash()])
		require.True(t, descendants[*tx3.Hash()])
		require.True(t, descendants[*tx4.Hash()])
	})

	t.Run("FeeRate", func(t *testing.T) {
		// Fee rate traversal should iterate transactions in descending
		// fee rate order. This is used for block template construction
		// where miners want to include the highest fee transactions
		// first to maximize revenue.
		var lastFeeRate int64 = -1
		for node := range g.Iterate(
			WithOrder(TraversalFeeRate),
		) {
			if lastFeeRate == -1 {
				lastFeeRate = node.TxDesc.FeePerKB
			} else {
				require.LessOrEqual(
					t, node.TxDesc.FeePerKB, lastFeeRate,
				)
				lastFeeRate = node.TxDesc.FeePerKB
			}
		}
	})

	t.Run("ReverseTopological", func(t *testing.T) {
		// Reverse topological order visits children before parents. This
		// is useful for transaction removal where we must remove
		// descendants before ancestors to avoid dangling references.
		var order []chainhash.Hash
		for node := range g.Iterate(
			WithOrder(TraversalReverseTopo),
		) {
			order = append(order, node.TxHash)
		}
		require.Equal(t, 4, len(order))

		// Verify that children appear before their parents in the
		// traversal order, which is required for safe removal.
		tx4Idx := -1
		tx2Idx := -1
		tx3Idx := -1
		tx1Idx := -1
		for i, hash := range order {
			switch hash {
			case *tx4.Hash():
				tx4Idx = i
			case *tx2.Hash():
				tx2Idx = i
			case *tx3.Hash():
				tx3Idx = i
			case *tx1.Hash():
				tx1Idx = i
			}
		}
		require.Less(t, tx4Idx, tx2Idx)
		require.Less(t, tx4Idx, tx3Idx)
		require.Less(t, tx2Idx, tx1Idx)
		require.Less(t, tx3Idx, tx1Idx)
	})

	t.Run("Cluster", func(t *testing.T) {
		// Cluster traversal should visit all transactions in the same
		// connected component. Since all transactions in this test are
		// connected via spending relationships, they form one cluster.
		count := 0
		for range g.Iterate(
			WithOrder(TraversalCluster),
			WithStartNode(tx1.Hash()),
		) {
			count++
		}
		require.Equal(t, 4, count, "All txs should be in same cluster")
	})

	t.Run("MaxDepth", func(t *testing.T) {
		// Depth limiting should stop traversal after a specified number
		// of levels. This prevents unbounded traversal in deep
		// dependency chains.
		count := 0
		for range g.Iterate(
			WithOrder(TraversalDescendants),
			WithStartNode(tx1.Hash()),
			WithMaxDepth(1),
		) {
			count++
		}
		require.Equal(
			t, 2, count, "Should only get direct children with "+
				"depth 1",
		)
	})

	t.Run("Filter", func(t *testing.T) {
		// Filter predicates allow selective iteration based on node
		// properties. This is useful for finding specific transaction
		// patterns without traversing the entire graph.
		count := 0
		nodes := make(map[chainhash.Hash]bool)
		for node := range g.Iterate(
			WithOrder(TraversalBFS),
			WithFilter(func(n *TxGraphNode) bool {
				// Select only transactions with exactly one
				// parent. This pattern identifies simple chains
				// without merge points.
				return len(n.Parents) == 1
			}),
		) {
			nodes[node.TxHash] = true
			count++
		}
		require.Equal(t, 2, count, "Should only get tx2 and tx3")
		require.True(t, nodes[*tx2.Hash()], "Should include tx2")
		require.True(t, nodes[*tx3.Hash()], "Should include tx3")
	})

	t.Run("DirectionBackward", func(t *testing.T) {
		// Backward direction traverses toward ancestors (parents). This
		// is useful for finding all transactions that must confirm
		// before a given transaction can be included in a block.
		count := 0
		for range g.Iterate(
			WithOrder(TraversalDFS),
			WithStartNode(tx4.Hash()),
			WithDirection(DirectionBackward),
		) {
			count++
		}
		require.Equal(t, 3, count, "Should traverse backward to parents")
	})

	t.Run("DirectionBoth", func(t *testing.T) {
		// Bidirectional traversal visits both parents and children.
		// This is useful for analyzing the local neighborhood of a
		// transaction without full graph traversal.
		count := 0
		visited := make(map[chainhash.Hash]bool)
		for node := range g.Iterate(
			WithOrder(TraversalBFS),
			WithStartNode(tx2.Hash()),
			WithDirection(DirectionBoth),
			WithMaxDepth(1),
		) {
			visited[node.TxHash] = true
			count++
		}
		require.Equal(
			t, 2, count, "Should traverse to both parent and child",
		)
		require.True(
			t, visited[*tx1.Hash()], "Should visit tx1 (parent)",
		)
		require.True(
			t, visited[*tx4.Hash()], "Should visit tx4 (child)",
		)
	})
}

// TestIteratePackages verifies that package iteration produces all
// identified transaction packages in the graph. Package iteration enables
// efficient processing of transaction groups for package relay policies and
// mining optimization.
func TestIteratePackages(t *testing.T) {
	// Basic 1P1C (one parent, one child) detection works without an
	// analyzer, making this test independent of protocol-specific logic.
	g := New(DefaultConfig())

	// Create multiple independent 1P1C packages, which is the most common
	// pattern for CPFP (child pays for parent) transactions.
	for i := 0; i < 3; i++ {
		parent, parentDesc := createTestTx(nil, 1)
		require.NoError(t, g.AddTransaction(parent, parentDesc))

		child, childDesc := createTestTx(
			[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1)
		require.NoError(t, g.AddTransaction(child, childDesc))
	}

	identifiedPkgs, err := g.IdentifyPackages()
	require.NoError(t, err)
	require.Len(t, identifiedPkgs, 3, "Should identify 3 packages")

	packageCount := 0
	for pkg := range g.IteratePackages() {
		packageCount++
		require.NotEmpty(t, pkg.ID)
		require.NotEmpty(t, pkg.Transactions)
		require.NotNil(t, pkg.Topology)
	}

	require.Equal(t, 3, packageCount, "Should have 3 packages")
}

// TestIterateClusters tests cluster iteration.
func TestIterateClusters(t *testing.T) {
	g := New(DefaultConfig())

	// Create 2 separate chains.
	// Chain 1.
	tx1, desc1 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))
	tx2, desc2 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	// Chain 2.
	tx3, desc3 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx3, desc3))
	tx4, desc4 := createTestTx(
		[]wire.OutPoint{{Hash: *tx3.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx4, desc4))

	// Count clusters.
	clusterCount := 0
	totalNodes := 0
	for cluster := range g.IterateClusters() {
		clusterCount++
		totalNodes += len(cluster.Nodes)
		require.NotEmpty(t, cluster.ID)
	}

	require.Equal(t, 2, clusterCount, "Should have 2 clusters")
	require.Equal(t, 4, totalNodes, "Should have 4 total nodes")
}

// TestIterateClusterFromNode tests cluster iteration from specific node.
func TestIterateClusterFromNode(t *testing.T) {
	g := New(DefaultConfig())

	// Create a cluster of 3 connected transactions.
	tx1, desc1 := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx([]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx([]wire.OutPoint{{Hash: *tx1.Hash(), Index: 1}}, 1)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	// Create separate transaction.
	tx4, desc4 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx4, desc4))

	// Iterate cluster containing tx2.
	count := 0
	seenHashes := make(map[chainhash.Hash]bool)
	for node := range g.Iterate(
		WithOrder(TraversalCluster),
		WithStartNode(tx2.Hash()),
	) {
		count++
		seenHashes[node.TxHash] = true
	}

	require.Equal(t, 3, count, "Should see 3 nodes in cluster")
	require.True(t, seenHashes[*tx1.Hash()])
	require.True(t, seenHashes[*tx2.Hash()])
	require.True(t, seenHashes[*tx3.Hash()])
	require.False(t, seenHashes[*tx4.Hash()], "Should not see tx4")
}

// TestTraversalDefault tests the default traversal order.
func TestTraversalDefault(t *testing.T) {
	g := New(DefaultConfig())

	// Add some transactions.
	tx1, desc1 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx([]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	// Test default traversal (should iterate all nodes).
	count := 0
	nodes := make(map[string]bool)
	for node := range g.Iterate(
		WithOrder(TraversalDefault),
	) {
		count++
		nodes[node.TxHash.String()] = true
	}

	require.Equal(t, 3, count, "Should iterate over all 3 nodes")
	require.True(t, nodes[tx1.Hash().String()])
	require.True(t, nodes[tx2.Hash().String()])
	require.True(t, nodes[tx3.Hash().String()])
}

// TestIterateTopological tests topological iteration.
func TestIterateTopological(t *testing.T) {
	g := New(DefaultConfig())

	// Create a DAG:
	//   tx1 -> tx2 -> tx3
	//      \-> tx4
	tx1, desc1 := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx([]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx([]wire.OutPoint{{Hash: *tx2.Hash(), Index: 0}}, 1)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	tx4, desc4 := createTestTx([]wire.OutPoint{{Hash: *tx1.Hash(), Index: 1}}, 1)
	require.NoError(t, g.AddTransaction(tx4, desc4))

	// Test topological order.
	var order []string
	for node := range g.Iterate(
		WithOrder(TraversalTopological),
	) {
		order = append(order, node.TxHash.String())
	}

	require.Equal(t, 4, len(order))

	// tx1 must come before tx2 and tx4.
	tx1Idx := slices.Index(order, tx1.Hash().String())
	tx2Idx := slices.Index(order, tx2.Hash().String())
	tx3Idx := slices.Index(order, tx3.Hash().String())
	tx4Idx := slices.Index(order, tx4.Hash().String())

	require.Less(t, tx1Idx, tx2Idx)
	require.Less(t, tx1Idx, tx4Idx)
	require.Less(t, tx2Idx, tx3Idx)
}

// TestIterateClusterComplete tests complete cluster iteration.
func TestIterateClusterComplete(t *testing.T) {
	g := New(DefaultConfig())

	// Create a cluster with multiple nodes.
	tx1, desc1 := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx([]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx([]wire.OutPoint{{Hash: *tx1.Hash(), Index: 1}}, 1)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	// Create separate cluster.
	tx4, desc4 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx4, desc4))

	// Test cluster iteration from tx2.
	count := 0
	nodes := make(map[string]bool)
	for node := range g.Iterate(
		WithOrder(TraversalCluster),
		WithStartNode(tx2.Hash()),
	) {
		count++
		nodes[node.TxHash.String()] = true
	}

	require.Equal(t, 3, count, "Should find 3 nodes in cluster")
	require.True(t, nodes[tx1.Hash().String()])
	require.True(t, nodes[tx2.Hash().String()])
	require.True(t, nodes[tx3.Hash().String()])
	require.False(t, nodes[tx4.Hash().String()], "tx4 should not be in cluster")
}

// TestAddNeighborsToStack verifies that stack neighbor addition correctly
// adds parents, children, or both depending on traversal direction. This
// internal helper is critical for DFS/BFS traversal correctness.
func TestAddNeighborsToStack(t *testing.T) {
	g := New(DefaultConfig())

	// Create chain: tx1 -> tx2 -> tx3.
	tx1, desc1 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx([]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx([]wire.OutPoint{{Hash: *tx2.Hash(), Index: 0}}, 1)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	// Test DFS with DirectionBoth.
	visited := make(map[string]bool)
	for node := range g.Iterate(
		WithOrder(TraversalDFS),
		WithStartNode(tx2.Hash()),
		WithDirection(DirectionBoth),
	) {
		visited[node.TxHash.String()] = true
	}

	// Should visit both parent (tx1) and child (tx3).
	require.True(t, visited[tx1.Hash().String()])
	require.True(t, visited[tx3.Hash().String()])
	require.False(t, visited[tx2.Hash().String()]) // Start node excluded
}
