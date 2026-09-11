package txgraph

import (
	"crypto/rand"
	"encoding/binary"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// txCounter is a global atomic counter for generating unique transaction
// hashes. Using atomic operations allows concurrent test execution without
// hash collisions, which would cause spurious test failures.
var txCounter uint64

// h dereferences a hash pointer to a value. This helper reduces visual noise
// in tests that need to pass hash values rather than pointers to assertion
// functions.
func h(hash *chainhash.Hash) chainhash.Hash {
	return *hash
}

// txGenerator generates unique test transactions using an atomic counter.
// This provides deterministic transaction creation for property-based tests
// where we need reproducible test cases but still require unique transaction
// IDs to avoid graph collisions.
type txGenerator struct {
	counter *uint64
}

// newTxGenerator creates a new transaction generator using the global
// txCounter to ensure uniqueness across all test cases.
func newTxGenerator() *txGenerator {
	return &txGenerator{counter: &txCounter}
}

// createTx creates a test transaction with a guaranteed unique hash by
// embedding an atomic counter in each output's pkScript. This ensures
// transaction uniqueness for tests that need deterministic behavior, unlike
// createTestTx which uses random data and may occasionally collide.
func (gen *txGenerator) createTx(inputs []wire.OutPoint,
	numOutputs int) (*btcutil.Tx, *TxDesc) {

	tx := wire.NewMsgTx(wire.TxVersion)

	for _, input := range inputs {
		tx.AddTxIn(wire.NewTxIn(&input, nil, nil))
	}

	// Embed atomic counter in pkScript to guarantee unique transaction
	// hash. The counter is atomically incremented for each output, which
	// means even transactions with the same inputs will have different
	// hashes due to different output scripts.
	for i := 0; i < numOutputs; i++ {
		counter := atomic.AddUint64(gen.counter, 1)
		pkScript := make([]byte, 8)
		binary.BigEndian.PutUint64(pkScript, counter)

		tx.AddTxOut(wire.NewTxOut(100000, pkScript))
	}

	btcTx := btcutil.NewTx(tx)

	// Create a minimal TxDesc with fixed fee values. Tests that need
	// specific fee rates should create their own descriptors.
	txDesc := &TxDesc{
		TxHash:      *btcTx.Hash(),
		VirtualSize: int64(btcTx.MsgTx().SerializeSize()),
		Fee:         1000,
		FeePerKB:    10000,
		Added:       time.Now(),
	}

	return btcTx, txDesc
}

// createTestTx creates a test transaction with random output addresses.
// This is used by legacy tests that don't require deterministic transaction
// IDs. The randomness provides high probability of uniqueness but with a
// small chance of collision, which is acceptable for simple unit tests but
// not for property-based testing.
func createTestTx(inputs []wire.OutPoint,
	numOutputs int) (*btcutil.Tx, *TxDesc) {

	tx := wire.NewMsgTx(wire.TxVersion)

	for _, input := range inputs {
		tx.AddTxIn(wire.NewTxIn(&input, nil, nil))
	}

	// Generate random P2PKH addresses for each output. Using valid
	// Bitcoin addresses (rather than raw random bytes) makes test
	// transactions more realistic and easier to debug.
	for i := 0; i < numOutputs; i++ {
		randBytes := make([]byte, 20)
		rand.Read(randBytes)

		addr, _ := btcutil.NewAddressPubKeyHash(
			randBytes, &chaincfg.MainNetParams,
		)
		pkScript, _ := txscript.PayToAddrScript(addr)

		tx.AddTxOut(wire.NewTxOut(100000, pkScript))
	}

	btcTx := btcutil.NewTx(tx)

	txDesc := &TxDesc{
		TxHash:      *btcTx.Hash(),
		VirtualSize: int64(btcTx.MsgTx().SerializeSize()),
		Fee:         1000,
		FeePerKB:    10000,
		Added:       time.Now(),
	}

	return btcTx, txDesc
}

// TestGraphAddRemove verifies that transactions can be added to and removed
// from the graph, including proper error handling for duplicate adds and
// removal of non-existent transactions.
func TestGraphAddRemove(t *testing.T) {
	g := New(DefaultConfig())

	tx1, desc1 := createTestTx(nil, 2)

	err := g.AddTransaction(tx1, desc1)
	require.NoError(t, err)

	node, exists := g.GetNode(*tx1.Hash())
	require.True(t, exists)
	require.NotNil(t, node)
	require.Equal(t, *tx1.Hash(), node.TxHash)

	// Duplicate addition should return ErrTransactionExists rather than
	// allowing graph corruption from multiple nodes with the same hash.
	err = g.AddTransaction(tx1, desc1)
	require.ErrorIs(t, err, ErrTransactionExists)

	err = g.RemoveTransaction(*tx1.Hash())
	require.NoError(t, err)

	_, exists = g.GetNode(*tx1.Hash())
	require.False(t, exists)

	// Removing a non-existent transaction should fail gracefully rather
	// than panicking or corrupting graph state.
	err = g.RemoveTransaction(*tx1.Hash())
	require.ErrorIs(t, err, ErrNodeNotFound)
}

// TestGraphEdges verifies that parent-child edges are automatically created
// when a transaction spends outputs from another transaction in the graph.
// This is critical for maintaining the dependency graph that drives ancestor/
// descendant queries and cluster formation.
func TestGraphEdges(t *testing.T) {
	g := New(DefaultConfig())

	parent, parentDesc := createTestTx(nil, 2)
	err := g.AddTransaction(parent, parentDesc)
	require.NoError(t, err)

	parentOut := wire.OutPoint{
		Hash:  *parent.Hash(),
		Index: 0,
	}
	child, childDesc := createTestTx([]wire.OutPoint{parentOut}, 1)

	// Edge creation should happen automatically during AddTransaction by
	// detecting that the child spends from the parent's outputs.
	err = g.AddTransaction(child, childDesc)
	require.NoError(t, err)

	parentNode, _ := g.GetNode(*parent.Hash())
	childNode, _ := g.GetNode(*child.Hash())

	require.Len(t, parentNode.Children, 1)
	require.Len(t, childNode.Parents, 1)
	require.NotNil(t, parentNode.Children[*child.Hash()])
	require.NotNil(t, childNode.Parents[*parent.Hash()])

	metrics := g.GetMetrics()
	require.Equal(t, 2, metrics.NodeCount)
	require.Equal(t, 1, metrics.EdgeCount)

	err = g.RemoveEdge(*parent.Hash(), *child.Hash())
	require.NoError(t, err)

	// Edge removal should update both nodes' relationship maps and
	// decrement the edge count metric.
	parentNode, _ = g.GetNode(*parent.Hash())
	childNode, _ = g.GetNode(*child.Hash())
	require.Len(t, parentNode.Children, 0)
	require.Len(t, childNode.Parents, 0)
}

// TestGraphAncestorsDescendants verifies that ancestor and descendant
// queries correctly traverse the transaction dependency graph. These queries
// are essential for enforcing BIP 125 ancestor/descendant limits and
// calculating package fee rates for mining.
func TestGraphAncestorsDescendants(t *testing.T) {
	g := New(DefaultConfig())

	// Build a linear chain to test traversal in both directions.
	var prevHash *chainhash.Hash
	var txs []*btcutil.Tx

	for i := 0; i < 4; i++ {
		var inputs []wire.OutPoint
		if prevHash != nil {
			inputs = append(inputs, wire.OutPoint{
				Hash:  *prevHash,
				Index: 0,
			})
		}

		tx, desc := createTestTx(inputs, 1)
		err := g.AddTransaction(tx, desc)
		require.NoError(t, err)

		txs = append(txs, tx)
		prevHash = tx.Hash()
	}

	// Ancestors of tx3 should include all transactions it depends on.
	ancestors := g.GetAncestors(*txs[2].Hash(), -1)
	require.Len(t, ancestors, 2)
	require.NotNil(t, ancestors[*txs[0].Hash()])
	require.NotNil(t, ancestors[*txs[1].Hash()])

	// Depth limit should stop traversal at the specified level.
	ancestors = g.GetAncestors(*txs[2].Hash(), 1)
	require.Len(t, ancestors, 1)
	require.NotNil(t, ancestors[*txs[1].Hash()])

	// Descendants of tx1 should include all transactions that depend on
	// it, directly or indirectly.
	descendants := g.GetDescendants(*txs[0].Hash(), -1)
	require.Len(t, descendants, 3)
	require.NotNil(t, descendants[*txs[1].Hash()])
	require.NotNil(t, descendants[*txs[2].Hash()])
	require.NotNil(t, descendants[*txs[3].Hash()])

	descendants = g.GetDescendants(*txs[0].Hash(), 2)
	require.Len(t, descendants, 2)
	require.NotNil(t, descendants[*txs[1].Hash()])
	require.NotNil(t, descendants[*txs[2].Hash()])
}

// TestCycleDetection verifies that the graph prevents cycles, which would
// violate the DAG property required for transaction dependencies. Cycles
// would make ancestor/descendant queries infinite loop and break topological
// ordering for block template construction.
func TestCycleDetection(t *testing.T) {
	g := New(DefaultConfig())

	tx1, desc1 := createTestTx(nil, 1)
	tx2, desc2 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)

	err := g.AddTransaction(tx1, desc1)
	require.NoError(t, err)
	err = g.AddTransaction(tx2, desc2)
	require.NoError(t, err)

	// Attempting to add an edge that would create a cycle (tx2 -> tx1
	// when tx1 -> tx2 already exists) must be rejected to maintain the
	// DAG invariant.
	err = g.AddEdge(*tx2.Hash(), *tx1.Hash())
	require.ErrorIs(t, err, ErrCycleDetected)
}

// TestClusterManagement verifies that transactions are correctly grouped
// into clusters (connected components) and that clusters merge when a
// transaction bridges two previously separate clusters. This is essential
// for RBF validation where replacement transactions must improve the fee
// rate of the entire cluster.
func TestClusterManagement(t *testing.T) {
	g := New(DefaultConfig())

	// Create two independent transaction chains. Each chain forms its own
	// cluster since there are no spending relationships between them.
	tx1, desc1 := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 2,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	tx4, desc4 := createTestTx(
		[]wire.OutPoint{{Hash: *tx3.Hash(), Index: 0}}, 2,
	)
	require.NoError(t, g.AddTransaction(tx4, desc4))

	require.Equal(t, 2, g.GetClusterCount())

	// Create a transaction that spends from both chains. This bridges
	// the two clusters, forcing them to merge into a single connected
	// component.
	tx5Inputs := []wire.OutPoint{
		{Hash: *tx2.Hash(), Index: 0},
		{Hash: *tx4.Hash(), Index: 0},
	}
	tx5, desc5 := createTestTx(tx5Inputs, 1)
	require.NoError(t, g.AddTransaction(tx5, desc5))

	require.Equal(t, 1, g.GetClusterCount())

	// All transactions should now belong to the same cluster.
	cluster1, err := g.GetCluster(*tx1.Hash())
	require.NoError(t, err)
	cluster5, err := g.GetCluster(*tx5.Hash())
	require.NoError(t, err)
	require.Equal(t, cluster1.ID, cluster5.ID)
	require.Len(t, cluster1.Nodes, 5)
}

// TestTRUCDetection tests version 3 transaction detection.
func TestTRUCDetection(t *testing.T) {
	g := New(DefaultConfig())

	// Create version 3 transaction.
	tx := wire.NewMsgTx(3)
	tx.AddTxOut(wire.NewTxOut(100000, nil))
	btcTx := btcutil.NewTx(tx)

	desc := &TxDesc{
		TxHash:      *btcTx.Hash(),
		VirtualSize: int64(tx.SerializeSize()),
		Fee:         1000,
		FeePerKB:    10000,
		Added:       time.Now(),
	}

	err := g.AddTransaction(btcTx, desc)
	require.NoError(t, err)

	node, exists := g.GetNode(*btcTx.Hash())
	require.True(t, exists)
	require.True(t, node.Metadata.IsTRUC)

	metrics := g.GetMetrics()
	require.Equal(t, 1, metrics.TRUCCount)
}

// TestHasTransaction tests the HasTransaction method.
func TestHasTransaction(t *testing.T) {
	g := New(DefaultConfig())

	// Add a transaction.
	tx, desc := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx, desc))

	// Test HasTransaction.
	require.True(t, g.HasTransaction(*tx.Hash()))

	// Test non-existent transaction.
	nonExistent := &wire.MsgTx{Version: 1}
	nonExistentHash := nonExistent.TxHash()
	require.False(t, g.HasTransaction(nonExistentHash))
}

// TestGetNodeCount tests the GetNodeCount method.
func TestGetNodeCount(t *testing.T) {
	g := New(DefaultConfig())

	// Initially should be 0.
	require.Equal(t, 0, g.GetNodeCount())

	// Add transactions.
	tx1, desc1 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))
	require.Equal(t, 1, g.GetNodeCount())

	tx2, desc2 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))
	require.Equal(t, 2, g.GetNodeCount())
}

// TestAddEdgeErrors tests error cases in AddEdge.
func TestAddEdgeErrors(t *testing.T) {
	g := New(DefaultConfig())

	// Try to add edge between non-existent nodes.
	tx1Msg := wire.NewMsgTx(1)
	tx2Msg := wire.NewMsgTx(1)
	hash1 := tx1Msg.TxHash()
	hash2 := tx2Msg.TxHash()

	err := g.AddEdge(hash1, hash2)
	require.Error(t, err)
	require.Equal(t, ErrNodeNotFound, err)

	// Add one node and try to add edge.
	tx1, desc1 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	err = g.AddEdge(*tx1.Hash(), hash2)
	require.Error(t, err)
	require.Equal(t, ErrNodeNotFound, err)

	// Add second node.
	tx2, desc2 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	// Add valid edge.
	err = g.AddEdge(*tx1.Hash(), *tx2.Hash())
	require.NoError(t, err)

	// Try to add duplicate edge.
	err = g.AddEdge(*tx1.Hash(), *tx2.Hash())
	require.NoError(t, err)

	// Try to create cycle.
	err = g.AddEdge(*tx2.Hash(), *tx1.Hash())
	require.Error(t, err)
	require.Equal(t, ErrCycleDetected, err)
}

// TestRemoveTransactionComplex tests complex removal scenarios.
func TestRemoveTransactionComplex(t *testing.T) {
	g := New(DefaultConfig())

	// Create a chain: tx1 -> tx2 -> tx3.
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

	// Remove middle transaction (should recursively remove tx3 too).
	err := g.RemoveTransaction(*tx2.Hash())
	require.NoError(t, err)

	// Verify tx3 was also removed.
	require.False(t, g.HasTransaction(*tx3.Hash()))
	require.False(t, g.HasTransaction(*tx2.Hash()))

	// tx1 should still exist.
	require.True(t, g.HasTransaction(*tx1.Hash()))

	// Verify tx1 has no children.
	node1, exists := g.GetNode(*tx1.Hash())
	require.True(t, exists)
	require.Len(t, node1.Children, 0)
}

// Property-based tests using rapid.

// TestPropertyNoSelfLoops verifies graph never has self-loops.
func TestPropertyNoSelfLoops(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := New(DefaultConfig())

		// Generate random transactions.
		numTxs := rapid.IntRange(1, 20).Draw(t, "numTxs")
		txs := make([]*btcutil.Tx, 0, numTxs)

		for i := 0; i < numTxs; i++ {
			// Randomly connect to previous transactions.
			var inputs []wire.OutPoint
			if len(txs) > 0 && rapid.Bool().Draw(t, "hasParent") {
				parentIdx := rapid.IntRange(0, len(txs)-1).Draw(
					t, "parentIdx",
				)
				inputs = append(inputs, wire.OutPoint{
					Hash:  *txs[parentIdx].Hash(),
					Index: 0,
				})
			}

			tx, desc := createTestTx(inputs, 1)

			err := g.AddTransaction(tx, desc)
			if err == nil {
				txs = append(txs, tx)
			} else {
				// It's OK if we get duplicate transaction
				// errors in random tests
				require.ErrorIs(t, err, ErrTransactionExists)
			}
		}

		// Property: No node should have itself as parent or child.
		for hash, node := range g.nodes {
			require.Nil(
				t, node.Parents[hash],
				"Node has itself as parent",
			)
			require.Nil(
				t, node.Children[hash],
				"Node has itself as child",
			)
		}
	})
}

// TestPropertyParentChildSymmetry verifies parent-child relationships are
// symmetric.
func TestPropertyParentChildSymmetry(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := New(DefaultConfig())

		// Generate random DAG.
		numTxs := rapid.IntRange(2, 15).Draw(t, "numTxs")
		txs := make([]*btcutil.Tx, numTxs)

		for i := 0; i < numTxs; i++ {
			var inputs []wire.OutPoint
			if i > 0 && rapid.Bool().Draw(t, "hasParent") {
				// Connect to random previous transaction.
				parentIdx := rapid.IntRange(0, i-1).Draw(
					t, "parentIdx",
				)
				inputs = append(inputs, wire.OutPoint{
					Hash:  *txs[parentIdx].Hash(),
					Index: 0,
				})
			}

			tx, desc := createTestTx(inputs, 1)
			txs[i] = tx
			g.AddTransaction(tx, desc)
		}

		// Property: If A is parent of B, then B is child of A.
		for _, node := range g.nodes {
			for _, parent := range node.Parents {
				require.NotNil(
					t, parent.Children[node.TxHash],
					"Parent-child relationship not "+
						"symmetric",
				)
			}
			for _, child := range node.Children {
				require.NotNil(
					t, child.Parents[node.TxHash],
					"Child-parent relationship not "+
						"symmetric",
				)
			}
		}
	})
}

// TestPropertyMetricsConsistency verifies metrics are consistent with actual
// state.
func TestPropertyMetricsConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := New(DefaultConfig())

		// Generate random operations.
		ops := rapid.IntRange(10, 50).Draw(t, "ops")
		addedTxs := make(map[chainhash.Hash]*btcutil.Tx)

		for i := 0; i < ops; i++ {
			op := rapid.IntRange(0, 2).Draw(t, "operation")

			switch op {
			case 0, 1:
				var inputs []wire.OutPoint
				if len(addedTxs) > 0 &&
					rapid.Bool().Draw(t, "hasParent") {

					// Pick random parent from added txs.
					for hash := range addedTxs {
						inputs = append(inputs, wire.OutPoint{
							Hash:  hash,
							Index: 0,
						})
						break
					}
				}

				tx, desc := createTestTx(inputs, 1)
				if err := g.AddTransaction(tx, desc); err == nil {
					addedTxs[*tx.Hash()] = tx
				}

			case 2:
				if len(addedTxs) > 0 {
					// Remove random transaction.
					for hash := range addedTxs {
						if err := g.RemoveTransaction(hash); err == nil {
							delete(addedTxs, hash)
						}
						break
					}
				}
			}
		}

		// Property: Metrics should match actual counts.
		metrics := g.GetMetrics()
		require.Equal(t, len(g.nodes), metrics.NodeCount,
			"Node count mismatch")

		// Count actual edges.
		actualEdges := 0
		for _, node := range g.nodes {
			actualEdges += len(node.Children)
		}
		require.Equal(t, actualEdges, metrics.EdgeCount,
			"Edge count mismatch")
	})
}

// TestPropertyPackageTopology verifies package topology calculations.
func TestPropertyPackageTopology(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := New(DefaultConfig())

		// Create a simple 1P1C package.
		parent, parentDesc := createTestTx(nil, 1)
		child, childDesc := createTestTx(
			[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1)

		g.AddTransaction(parent, parentDesc)
		g.AddTransaction(child, childDesc)

		// Try to identify packages.
		packages, err := g.IdentifyPackages()
		require.NoError(t, err)

		// Property: 1P1C package should be identified correctly.
		found1P1C := false
		for _, pkg := range packages {
			if pkg.Type == PackageType1P1C {
				found1P1C = true
				require.Len(t, pkg.Transactions, 2)
				require.Equal(t, 1, pkg.Topology.MaxDepth)
				require.Equal(t, 1, pkg.Topology.MaxWidth)
				require.True(t, pkg.Topology.IsLinear)
				require.True(t, pkg.Topology.IsTree)
			}
		}
		require.True(t, found1P1C, "1P1C package not identified")
	})
}

// TestPropertyIteratorCompleteness verifies iterators visit all expected nodes.
func TestPropertyIteratorCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := New(DefaultConfig())

		// Create random transactions.
		numTxs := rapid.IntRange(5, 20).Draw(t, "numTxs")
		expectedHashes := make(map[chainhash.Hash]bool)

		for i := 0; i < numTxs; i++ {
			tx, desc := createTestTx(nil, 1)
			g.AddTransaction(tx, desc)
			expectedHashes[*tx.Hash()] = true
		}

		// Property: BFS iterator should visit all nodes.
		visitedBFS := make(map[chainhash.Hash]bool)
		for node := range g.Iterate(
			WithOrder(TraversalBFS),
		) {
			visitedBFS[node.TxHash] = true
		}
		require.Equal(t, expectedHashes, visitedBFS,
			"BFS didn't visit all nodes")

		// Property: DFS iterator should visit all nodes.
		visitedDFS := make(map[chainhash.Hash]bool)
		for node := range g.Iterate(
			WithOrder(TraversalDFS),
		) {
			visitedDFS[node.TxHash] = true
		}
		require.Equal(t, expectedHashes, visitedDFS,
			"DFS didn't visit all nodes")
	})
}

// TestAddTransactionReverseOrder tests the "Find children that spend this
// transaction" code path by adding transactions in reverse topological order
// (children before parents). This tests that the spentBy index is maintained
// correctly even when children are added before their parents, which is
// critical for Bitcoin mempool behavior.
func TestAddTransactionReverseOrder(t *testing.T) {
	g := New(DefaultConfig())

	// Create parent transaction but don't add it yet.
	parent, parentDesc := createTestTx(nil, 2)
	parentHash := parent.Hash()

	// Create child that spends output 0 of parent.
	child1, child1Desc := createTestTx(
		[]wire.OutPoint{{Hash: *parentHash, Index: 0}}, 1,
	)

	// Create child that spends output 1 of parent.
	child2, child2Desc := createTestTx(
		[]wire.OutPoint{{Hash: *parentHash, Index: 1}}, 1,
	)

	// Add children FIRST (children arrive before parent in mempool). This
	// should populate spentBy index even though parent doesn't exist yet.
	require.NoError(t, g.AddTransaction(child1, child1Desc))
	require.NoError(t, g.AddTransaction(child2, child2Desc))

	// Verify children exist but have no parents yet (orphaned).
	child1Node, _ := g.GetNode(*child1.Hash())
	child2Node, _ := g.GetNode(*child2.Hash())
	require.Len(t, child1Node.Parents, 0, "child1 should have no parents yet")
	require.Len(t, child2Node.Parents, 0, "child2 should have no parents yet")

	// Now add parent - should trigger "Find children that spend this
	// transaction". This reconnects the orphaned children to their parent.
	require.NoError(t, g.AddTransaction(parent, parentDesc))

	// Verify parent is connected to both children.
	parentNode, exists := g.GetNode(h(parentHash))
	require.True(t, exists)
	require.Len(t, parentNode.Children, 2, "parent should have 2 children")
	require.NotNil(t, parentNode.Children[*child1.Hash()])
	require.NotNil(t, parentNode.Children[*child2.Hash()])

	// Verify children are now connected to parent.
	child1Node, _ = g.GetNode(*child1.Hash())
	child2Node, _ = g.GetNode(*child2.Hash())
	require.Len(t, child1Node.Parents, 1, "child1 should have 1 parent")
	require.Len(t, child2Node.Parents, 1, "child2 should have 1 parent")
	require.NotNil(t, child1Node.Parents[*parentHash])
	require.NotNil(t, child2Node.Parents[*parentHash])

	// Verify edge count is correct (2 edges).
	metrics := g.GetMetrics()
	require.Equal(t, 2, metrics.EdgeCount, "should have 2 edges")
}

// TestRemoveTransactionWithChildren tests edge removal from children
// in removeTransactionUnsafe.
func TestRemoveTransactionWithChildren(t *testing.T) {
	g := New(DefaultConfig())

	// Create a diamond pattern:
	//     tx1
	//    /   \
	//  tx2   tx3
	//    \   /
	//     tx4
	tx1, desc1 := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 2,
	)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	tx3, desc3 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 1}}, 2,
	)
	require.NoError(t, g.AddTransaction(tx3, desc3))

	tx4Inputs := []wire.OutPoint{
		{Hash: *tx2.Hash(), Index: 0},
		{Hash: *tx3.Hash(), Index: 0},
	}
	tx4, desc4 := createTestTx(tx4Inputs, 1)
	require.NoError(t, g.AddTransaction(tx4, desc4))

	// Verify initial state.
	require.Equal(t, 4, g.GetNodeCount())
	require.Equal(t, 4, g.GetMetrics().EdgeCount)

	// Remove tx2 (should also remove tx4 as descendant).
	require.NoError(t, g.RemoveTransaction(*tx2.Hash()))

	// Verify tx2 and tx4 are removed (tx4 loses both parents).
	require.False(t, g.HasTransaction(*tx2.Hash()))
	require.False(t, g.HasTransaction(*tx4.Hash()))

	// Verify tx1 and tx3 still exist.
	require.True(t, g.HasTransaction(*tx1.Hash()))
	require.True(t, g.HasTransaction(*tx3.Hash()))

	// Verify tx3 has its parent edge to tx1 still intact but child edge to
	// tx4 is gone.
	tx3Node, _ := g.GetNode(*tx3.Hash())
	require.Len(t, tx3Node.Parents, 1)
	require.NotNil(t, tx3Node.Parents[*tx1.Hash()])
	require.Len(
		t, tx3Node.Children, 0, "tx4 removed so tx3 should have "+
			"no children",
	)

	// Verify tx1 has only tx3 as child now.
	tx1Node, _ := g.GetNode(*tx1.Hash())
	require.Len(t, tx1Node.Children, 1)
	require.NotNil(t, tx1Node.Children[*tx3.Hash()])
}

// TestRemoveTransactionPackageCleanup tests package cleanup in
// removeTransactionUnsafe.
func TestRemoveTransactionPackageCleanup(t *testing.T) {
	g := New(DefaultConfig())

	// Create a 1P1C package.
	parent, parentDesc := createTestTx(nil, 1)
	child, childDesc := createTestTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1,
	)

	require.NoError(t, g.AddTransaction(parent, parentDesc))
	require.NoError(t, g.AddTransaction(child, childDesc))

	// Identify and assign packages.
	packages, err := g.IdentifyPackages()
	require.NoError(t, err)
	require.Len(t, packages, 1)

	// Assign package IDs.
	for i := range packages {
		pkg := packages[i]
		for hash := range pkg.Transactions {
			node, _ := g.GetNode(hash)
			node.Metadata.PackageID = &pkg.ID
		}
		g.indexes.packages[pkg.ID] = pkg
		for hash := range pkg.Transactions {
			g.indexes.nodeToPackage[hash] = pkg.ID
		}
	}

	// Verify package exists.
	require.Equal(t, 1, g.GetMetrics().PackageCount)

	// Remove parent (should remove child and clean up package).
	require.NoError(t, g.RemoveTransaction(*parent.Hash()))

	// Verify both transactions are gone.
	require.False(t, g.HasTransaction(*parent.Hash()))
	require.False(t, g.HasTransaction(*child.Hash()))

	// Verify package is cleaned up.
	require.Equal(t, 0, g.GetMetrics().PackageCount)
	require.Len(t, g.indexes.packages, 0)
	require.Len(t, g.indexes.nodeToPackage, 0)
}

// TestPropertyTransactionChains tests random transaction chains with
// add/remove operations.
func TestPropertyTransactionChains(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := New(DefaultConfig())

		// Generate random chain length.
		chainLength := rapid.IntRange(2, 10).Draw(t, "chainLength")

		// Build a chain of transactions in packages.
		chain := make([]*btcutil.Tx, chainLength)
		var prevHash *chainhash.Hash

		for i := 0; i < chainLength; i++ {
			var inputs []wire.OutPoint
			if prevHash != nil {
				inputs = append(inputs, wire.OutPoint{
					Hash:  *prevHash,
					Index: 0,
				})
			}

			tx, desc := createTestTx(inputs, 1)
			err := g.AddTransaction(tx, desc)
			require.NoError(t, err)

			chain[i] = tx
			prevHash = tx.Hash()
		}

		// Verify all transactions were added.
		require.Equal(t, chainLength, g.GetNodeCount())

		// Property: Edge count should be chainLength - 1.
		expectedEdges := chainLength - 1
		require.Equal(t, expectedEdges, g.GetMetrics().EdgeCount,
			"Edge count mismatch after adding chain")

		// Now remove transactions in random order and verify
		// invariants.
		indices := make([]int, chainLength)
		for i := range indices {
			indices[i] = i
		}
		removalOrder := rapid.Permutation(indices).Draw(
			t, "removalOrder",
		)

		for _, idx := range removalOrder {
			tx := chain[idx]
			if !g.HasTransaction(*tx.Hash()) {
				// Already removed as descendant.
				continue
			}

			// Remember state before removal.
			nodeCountBefore := g.GetNodeCount()
			edgeCountBefore := g.GetMetrics().EdgeCount

			// Remove transaction.
			err := g.RemoveTransaction(*tx.Hash())
			require.NoError(t, err)

			// Verify transaction is gone.
			require.False(t, g.HasTransaction(*tx.Hash()))

			// Verify node count decreased.
			require.Less(t, g.GetNodeCount(), nodeCountBefore)

			// Property: No dangling edges.
			for _, node := range g.nodes {
				for parentHash, parent := range node.Parents {
					require.NotNil(
						t, parent.Children[node.TxHash],
						"Dangling edge: parent %v "+
							"doesn't have child %v",
						parentHash, node.TxHash,
					)
				}
				for childHash, child := range node.Children {
					require.NotNil(
						t, child.Parents[node.TxHash],
						"Dangling edge: child %v "+
							"doesn't have parent %v",
						childHash, node.TxHash,
					)
				}
			}

			// Property: Edge count is consistent.
			actualEdges := 0
			for _, node := range g.nodes {
				actualEdges += len(node.Children)
			}
			require.Equal(
				t, actualEdges, g.GetMetrics().EdgeCount,
				"Edge count inconsistent after removal",
			)
			require.LessOrEqual(
				t, g.GetMetrics().EdgeCount, edgeCountBefore,
				"Edge count should not increase after removal",
			)
		}

		// Property: After removing all, graph should be empty.
		require.Equal(t, 0, g.GetNodeCount(), "Graph should be empty")
		require.Equal(
			t, 0, g.GetMetrics().EdgeCount, "No edges should remain",
		)
	})
}

// TestIterateClusterWithFilter tests cluster iteration with filters.
func TestIterateClusterWithFilter(t *testing.T) {
	g := New(DefaultConfig())

	// Create a cluster with multiple transactions.
	tx1, desc1 := createTestTx(nil, 2)
	tx2, desc2 := createTestTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 2,
	)
	tx3, desc3 := createTestTx(
		[]wire.OutPoint{{Hash: *tx2.Hash(), Index: 0}}, 1,
	)

	require.NoError(t, g.AddTransaction(tx1, desc1))
	require.NoError(t, g.AddTransaction(tx2, desc2))
	require.NoError(t, g.AddTransaction(tx3, desc3))

	// All should be in same cluster.
	require.Equal(t, 1, g.GetClusterCount())

	// Test iteration with filter (only nodes with 2 outputs).
	var filtered []*TxGraphNode
	filterFunc := func(node *TxGraphNode) bool {
		return len(node.Tx.MsgTx().TxOut) == 2
	}

	for node := range g.Iterate(
		WithOrder(TraversalCluster), WithStartNode(tx1.Hash()),
		WithFilter(filterFunc),
	) {
		filtered = append(filtered, node)
	}

	// Should only get tx1 and tx2 (both have 2 outputs), not tx3 (1
	// output).
	require.Len(t, filtered, 2)

	// Test iterating all clusters with nil start node.
	var allInClusters []*TxGraphNode
	for node := range g.Iterate(WithOrder(TraversalCluster)) {
		allInClusters = append(allInClusters, node)
	}
	require.Len(
		t, allInClusters, 3, "should iterate all nodes when start "+
			"is nil",
	)

	// Test early exit from yield function.
	count := 0
	for range g.Iterate(WithOrder(TraversalCluster)) {
		count++
		if count >= 2 {
			break // Exit early
		}
	}
	require.Equal(
		t, 2, count, "should stop iteration when yield returns false",
	)
}

// TestPropertyComplexPackageChains tests complex transaction graphs in
// packages.
func TestPropertyComplexPackageChains(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := New(DefaultConfig())
		gen := newTxGenerator()

		// Create a more complex graph structure.
		numRoots := rapid.IntRange(1, 3).Draw(t, "numRoots")
		roots := make([]*btcutil.Tx, numRoots)

		// Create root transactions.
		for i := 0; i < numRoots; i++ {
			tx, desc := gen.createTx(nil, 2)
			require.NoError(t, g.AddTransaction(tx, desc))
			roots[i] = tx
		}

		// Create children that may spend from multiple roots.
		numChildren := rapid.IntRange(1, 5).Draw(t, "numChildren")
		allTxs := make([]*btcutil.Tx, 0, numRoots+numChildren)
		allTxHashes := make([]chainhash.Hash, 0, numRoots+numChildren)
		allTxs = append(allTxs, roots...)
		for _, tx := range roots {
			allTxHashes = append(allTxHashes, *tx.Hash())
		}

		for i := 0; i < numChildren; i++ {
			// Randomly select parents from existing transactions.
			maxParents := 2
			if len(allTxs) < maxParents {
				maxParents = len(allTxs)
			}
			numParents := rapid.IntRange(1, maxParents).Draw(
				t, "numParents",
			)
			inputs := make([]wire.OutPoint, 0, numParents)

			txIndices := make([]int, len(allTxs))
			for j := range txIndices {
				txIndices[j] = j
			}
			selectedParents := rapid.Permutation(txIndices).Draw(
				t, "parentPerm",
			)
			for j := 0; j < numParents; j++ {
				parentTx := allTxs[selectedParents[j]]
				numOutputs := len(parentTx.MsgTx().TxOut)
				if numOutputs > 0 {
					// Use different output indexes to
					// avoid conflicts.
					outputIdx := uint32(j % numOutputs)
					inputs = append(inputs, wire.OutPoint{
						Hash:  *parentTx.Hash(),
						Index: outputIdx,
					})
				}
			}

			if len(inputs) > 0 {
				tx, desc := gen.createTx(inputs, 1)
				if err := g.AddTransaction(tx, desc); err == nil {
					allTxs = append(allTxs, tx)
					allTxHashes = append(
						allTxHashes, *tx.Hash(),
					)
				}
			}
		}

		initialNodeCount := g.GetNodeCount()
		initialEdgeCount := g.GetMetrics().EdgeCount

		// Property: Removing transactions maintains graph invariants.
		if len(allTxHashes) > 0 && g.GetNodeCount() > 0 {
			// Remove a random transaction using the stored hash
			// value.
			removeIdx := rapid.IntRange(0, len(allTxHashes)-1).Draw(
				t, "removeIdx",
			)
			hashToRemove := allTxHashes[removeIdx]

			// Debug: check if hash is in graph.
			hasIt := g.HasTransaction(hashToRemove)
			t.Logf("Hash to remove: %v, HasTransaction=%v",
				hashToRemove, hasIt)

			// Only proceed if transaction is actually in the graph.
			if !hasIt {
				// Transaction may not have been added due to
				// conflicts.
				t.Logf("Skipping because transaction not in " +
					"graph")
				return
			}

			t.Logf("About to call RemoveTransaction")
			err := g.RemoveTransaction(hashToRemove)
			t.Logf("RemoveTransaction returned: %v", err)

			require.NoError(
				t, err, "failed to remove transaction %v",
				hashToRemove,
			)

			// Property: Node count decreased.
			require.LessOrEqual(
				t, g.GetNodeCount(), initialNodeCount,
			)

			// Property: Edge count decreased or stayed same.
			require.LessOrEqual(
				t, g.GetMetrics().EdgeCount, initialEdgeCount,
			)

			// Property: All remaining edges are valid.
			for _, node := range g.nodes {
				for _, parent := range node.Parents {
					require.NotNil(
						t, parent.Children[node.TxHash],
						"Invalid parent-child relationship",
					)
				}
				for _, child := range node.Children {
					require.NotNil(
						t, child.Parents[node.TxHash],
						"Invalid child-parent relationship",
					)
				}
			}

			// Property: Metrics are consistent.
			actualEdges := 0
			for _, node := range g.nodes {
				actualEdges += len(node.Children)
			}
			require.Equal(t, actualEdges, g.GetMetrics().EdgeCount)
		}
	})
}

// TestWithIncludeStart tests the WithIncludeStart iterator option.
func TestWithIncludeStart(t *testing.T) {
	t.Parallel()

	g := New(DefaultConfig())
	gen := newTxGenerator()

	// Create a chain of transactions: tx1 -> tx2 -> tx3 -> tx4.
	tx1, desc1 := gen.createTx(nil, 1)
	tx2, desc2 := gen.createTx(
		[]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1,
	)
	tx3, desc3 := gen.createTx(
		[]wire.OutPoint{{Hash: *tx2.Hash(), Index: 0}}, 1,
	)
	tx4, desc4 := gen.createTx(
		[]wire.OutPoint{{Hash: *tx3.Hash(), Index: 0}}, 1,
	)

	require.NoError(t, g.AddTransaction(tx1, desc1))
	require.NoError(t, g.AddTransaction(tx2, desc2))
	require.NoError(t, g.AddTransaction(tx3, desc3))
	require.NoError(t, g.AddTransaction(tx4, desc4))

	// Test with IncludeStart = false (default behavior). Starting from
	// tx2, we should get descendants without tx2 itself.
	visitedExclude := slices.Collect(g.Iterate(
		WithStartNode(tx2.Hash()),
		WithOrder(TraversalDescendants),
		WithIncludeStart(false),
	))
	require.Len(
		t, visitedExclude, 2, "Should visit 2 descendants (tx3, tx4)",
	)
	excludeHashes := slicesMap(
		visitedExclude,
		func(n *TxGraphNode) chainhash.Hash { return n.TxHash },
	)

	require.Contains(t, excludeHashes, *tx3.Hash())
	require.Contains(t, excludeHashes, *tx4.Hash())
	require.NotContains(
		t, excludeHashes, *tx2.Hash(), "Should not include "+
			"starting node",
	)

	// Test with IncludeStart = true. Starting from tx2, we should get
	// descendants including tx2 itself.
	visitedInclude := slices.Collect(g.Iterate(
		WithStartNode(tx2.Hash()),
		WithOrder(TraversalDescendants),
		WithIncludeStart(true),
	))

	require.Len(
		t, visitedInclude, 3, "Should visit 3 nodes (tx2, tx3, tx4)",
	)

	includeHashes := slicesMap(
		visitedInclude,
		func(n *TxGraphNode) chainhash.Hash { return n.TxHash },
	)

	require.Contains(
		t, includeHashes, *tx2.Hash(), "Should include starting node",
	)
	require.Contains(t, includeHashes, *tx3.Hash())
	require.Contains(t, includeHashes, *tx4.Hash())

	// Test with ancestors direction and IncludeStart = true.
	visitedAncestors := slices.Collect(g.Iterate(
		WithStartNode(tx3.Hash()),
		WithOrder(TraversalAncestors),
		WithIncludeStart(true),
	))
	require.Len(
		t, visitedAncestors, 3, "Should visit 3 nodes (tx3, tx2, tx1)",
	)
	ancestorHashes := slicesMap(
		visitedAncestors,
		func(n *TxGraphNode) chainhash.Hash { return n.TxHash },
	)

	require.Contains(
		t, ancestorHashes, *tx3.Hash(), "Should include starting node",
	)
	require.Contains(t, ancestorHashes, *tx2.Hash())
	require.Contains(t, ancestorHashes, *tx1.Hash())
}

// slicesMap maps a slice using a transform function.
func slicesMap[T any, U any](slice []T, fn func(T) U) []U {
	result := make([]U, len(slice))
	for i, v := range slice {
		result[i] = fn(v)
	}
	return result
}

// TestValidatePackageErrors tests error cases in ValidatePackage.
func TestValidatePackageErrors(t *testing.T) {
	t.Parallel()

	g := New(DefaultConfig())

	// Test nil package.
	err := g.ValidatePackage(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil package")

	// Test empty package.
	emptyPkg := &TxPackage{
		ID:           PackageID{Type: PackageType1P1C},
		Transactions: make(map[chainhash.Hash]*TxGraphNode),
	}
	err = g.ValidatePackage(emptyPkg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty package")

	// Test package too large.
	cfg := DefaultConfig()
	largePkg := &TxPackage{
		ID:           PackageID{Type: PackageType1P1C},
		Transactions: make(map[chainhash.Hash]*TxGraphNode),
	}
	// Create more transactions than allowed by config.
	maxSize := cfg.MaxPackageSize
	gen := newTxGenerator()
	for i := 0; i < maxSize+1; i++ {
		tx, _ := gen.createTx(nil, 1)
		node := &TxGraphNode{
			TxHash: *tx.Hash(),
		}
		largePkg.Transactions[*tx.Hash()] = node
	}
	err = g.ValidatePackage(largePkg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "package too large")
}

// TestRemoveTransactionWithTRUC tests removal of TRUC transactions.
func TestRemoveTransactionWithTRUC(t *testing.T) {
	t.Parallel()

	g := New(DefaultConfig())
	gen := newTxGenerator()

	// Create a TRUC transaction.
	tx1, desc1 := gen.createTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	// Mark it as TRUC by directly accessing the internal structure.
	node, exists := g.GetNode(*tx1.Hash())
	require.True(t, exists)
	node.Metadata.IsTRUC = true
	g.indexes.trucTxs[*tx1.Hash()] = node

	// Update metrics.
	oldTrucCount := g.GetMetrics().TRUCCount
	atomic.AddInt32(&g.metrics.trucCount, 1)

	// Verify TRUC transaction is tracked.
	require.Equal(t, oldTrucCount+1, g.GetMetrics().TRUCCount)
	_, exists = g.indexes.trucTxs[*tx1.Hash()]
	require.True(t, exists, "TRUC transaction should be in index")

	// Remove the TRUC transaction.
	err := g.RemoveTransaction(*tx1.Hash())
	require.NoError(t, err)

	// Verify TRUC index is cleaned up.
	_, exists = g.indexes.trucTxs[*tx1.Hash()]
	require.False(
		t, exists, "TRUC transaction should be removed from index",
	)
	require.Equal(
		t, oldTrucCount, g.GetMetrics().TRUCCount,
		"TRUC count should be decremented",
	)
}

// TestRemoveTransactionWithEphemeral tests removal of ephemeral transactions.
func TestRemoveTransactionWithEphemeral(t *testing.T) {
	t.Parallel()

	g := New(DefaultConfig())
	gen := newTxGenerator()

	// Create an ephemeral transaction.
	tx1, desc1 := gen.createTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	// Mark it as ephemeral by directly accessing the internal structure.
	node, exists := g.GetNode(*tx1.Hash())
	require.True(t, exists)
	node.Metadata.IsEphemeral = true
	g.indexes.ephemeralTxs[*tx1.Hash()] = node

	// Update metrics.
	oldEphemeralCount := g.GetMetrics().EphemeralCount
	atomic.AddInt32(&g.metrics.ephemeralCount, 1)

	// Verify ephemeral transaction is tracked.
	require.Equal(t, oldEphemeralCount+1, g.GetMetrics().EphemeralCount)
	_, exists = g.indexes.ephemeralTxs[*tx1.Hash()]
	require.True(t, exists, "Ephemeral transaction should be in index")

	// Remove the ephemeral transaction.
	err := g.RemoveTransaction(*tx1.Hash())
	require.NoError(t, err)

	// Verify ephemeral index is cleaned up.
	_, exists = g.indexes.ephemeralTxs[*tx1.Hash()]
	require.False(
		t, exists, "Ephemeral transaction should be removed from index",
	)
	require.Equal(
		t, oldEphemeralCount, g.GetMetrics().EphemeralCount,
		"Ephemeral count should be decremented",
	)
}

// TestRemoveTransactionNoCascade tests that RemoveTransactionNoCascade
// properly removes a transaction without cascading to its children, and cleans
// up edges.
func TestRemoveTransactionNoCascade(t *testing.T) {
	t.Parallel()

	g := New(DefaultConfig())
	gen := newTxGenerator()

	// Create a structure where a child has TWO parents:
	//   parent1    parent2
	//       \       /
	//        child
	//
	// When we remove parent1 with NoCascade, the child should remain
	// (still has parent2), but the edge from parent1 to child must be
	// cleaned up.
	parent1, parent1Desc := gen.createTx(nil, 1)
	require.NoError(t, g.AddTransaction(parent1, parent1Desc))

	parent2, parent2Desc := gen.createTx(nil, 1)
	require.NoError(t, g.AddTransaction(parent2, parent2Desc))

	// Create a child that spends from BOTH parents.
	childInputs := []wire.OutPoint{
		{Hash: *parent1.Hash(), Index: 0},
		{Hash: *parent2.Hash(), Index: 0},
	}
	child, childDesc := gen.createTx(childInputs, 1)
	require.NoError(t, g.AddTransaction(child, childDesc))

	// Verify initial state: child has 2 parents.
	childNode, _ := g.GetNode(*child.Hash())
	require.Len(t, childNode.Parents, 2)
	require.NotNil(t, childNode.Parents[*parent1.Hash()])
	require.NotNil(t, childNode.Parents[*parent2.Hash()])

	parent1Node, _ := g.GetNode(*parent1.Hash())
	require.Len(t, parent1Node.Children, 1)
	require.NotNil(t, parent1Node.Children[*child.Hash()])

	initialEdgeCount := g.GetMetrics().EdgeCount
	require.Equal(t, 2, initialEdgeCount)

	// Remove parent1 without cascade (simulating confirmation). Child
	// should remain because it still has parent2.
	err := g.RemoveTransactionNoCascade(h(parent1.Hash()))
	require.NoError(t, err)

	// Verify parent1 is gone.
	require.False(t, g.HasTransaction(*parent1.Hash()))

	// Verify child still exists (still has parent2).
	require.True(t, g.HasTransaction(*child.Hash()))

	// Verify parent2 still exists.
	require.True(t, g.HasTransaction(*parent2.Hash()))

	// CRITICAL: Verify that the child's parent1 reference is cleaned
	// up.
	childNode, _ = g.GetNode(*child.Hash())
	require.Len(
		t, childNode.Parents, 1, "child should have 1 parent remaining",
	)
	require.Nil(
		t, childNode.Parents[*parent1.Hash()],
		"parent1 should not exist in child's parent map",
	)
	require.NotNil(
		t, childNode.Parents[*parent2.Hash()],
		"parent2 should still exist in child's parent map",
	)

	// Verify edge count is decremented correctly (1 edge removed).
	finalEdgeCount := g.GetMetrics().EdgeCount
	require.Equal(
		t, initialEdgeCount-1, finalEdgeCount,
		"should have removed 1 edge",
	)
}

// TestAddToClusterWhenClusterDoesNotExist tests the path where addToCluster
// is called with a cluster ID that doesn't exist.
func TestAddToClusterWhenClusterDoesNotExist(t *testing.T) {
	t.Parallel()

	g := New(DefaultConfig())
	gen := newTxGenerator()

	// Create two transactions that will form a cluster.
	tx1, desc1 := gen.createTx(nil, 1)
	tx2, desc2 := gen.createTx([]wire.OutPoint{{Hash: *tx1.Hash(), Index: 0}}, 1)

	require.NoError(t, g.AddTransaction(tx1, desc1))
	require.NoError(t, g.AddTransaction(tx2, desc2))

	// Get the cluster ID for tx1.
	clusterID := g.indexes.nodeToCluster[*tx1.Hash()]
	require.NotEqual(t, ClusterID(0), clusterID, "tx1 should be in a cluster")

	// Now manually delete the cluster from the index to simulate the
	// scenario where addToCluster is called with a non-existent cluster.
	delete(g.indexes.clusters, clusterID)

	// Create a new transaction.
	tx3, desc3 := gen.createTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx3, desc3))
	node3, exists := g.GetNode(*tx3.Hash())
	require.True(t, exists)

	// Manually call addToCluster with the deleted cluster ID. This
	// should trigger the !exists path and create a new cluster.
	oldClusterCount := g.GetMetrics().ClusterCount
	g.addToCluster(node3, clusterID)

	// Verify a new cluster was created.
	require.Equal(
		t, oldClusterCount+1, g.GetMetrics().ClusterCount,
		"New cluster should be created",
	)
	newClusterID := g.indexes.nodeToCluster[*tx3.Hash()]
	require.NotEqual(t, ClusterID(0), newClusterID, "tx3 should be in a cluster")

	// Verify the cluster exists and contains tx3.
	cluster, exists := g.indexes.clusters[newClusterID]
	require.True(t, exists, "New cluster should exist")
	require.Equal(t, 1, cluster.Size, "Cluster should have 1 node")
	require.Contains(t, cluster.Nodes, *tx3.Hash(), "Cluster should contain tx3")
}

