package txgraph

import (
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// BenchmarkAddTransaction benchmarks adding transactions to the graph.
func BenchmarkAddTransaction(b *testing.B) {
	b.ReportAllocs()

	g := New(DefaultConfig())

	// Pre-create transactions to isolate graph operations from test data
	// generation overhead in timing measurements.
	txs := make([]*wire.MsgTx, b.N)
	descs := make([]*TxDesc, b.N)

	for i := 0; i < b.N; i++ {
		tx, desc := createTestTx(nil, 1)
		txs[i] = tx.MsgTx()
		descs[i] = desc
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		btcTx := btcutil.NewTx(txs[i])
		g.AddTransaction(btcTx, descs[i])
	}

	b.ReportMetric(float64(g.GetNodeCount()), "nodes")
}

// BenchmarkAddTransactionWithEdges benchmarks adding connected transactions.
// This measures the overhead of automatic edge detection when transactions
// form dependency chains, which is critical for understanding the cost of
// maintaining parent-child relationships in the mempool graph.
func BenchmarkAddTransactionWithEdges(b *testing.B) {
	b.ReportAllocs()

	g := New(DefaultConfig())

	// Create initial parent to enable edge creation for subsequent
	// transactions. This setup allows us to measure the overhead of
	// automatic edge detection as each new transaction references the
	// previous one.
	parent, parentDesc := createTestTx(nil, 1)
	g.AddTransaction(parent, parentDesc)

	b.ResetTimer()

	prevHash := parent.Hash()
	for i := 0; i < b.N; i++ {
		tx, desc := createTestTx([]wire.OutPoint{
			{Hash: *prevHash, Index: 0},
		}, 1)

		g.AddTransaction(tx, desc)
		prevHash = tx.Hash()
	}

	b.ReportMetric(float64(g.GetNodeCount()), "nodes")
	b.ReportMetric(float64(g.GetMetrics().EdgeCount), "edges")
}

// BenchmarkRemoveTransaction benchmarks removing transactions.
// This measures the cost of graph cleanup operations, which occur when
// transactions confirm in blocks or are evicted from the mempool.
func BenchmarkRemoveTransaction(b *testing.B) {
	b.ReportAllocs()

	g := New(DefaultConfig())

	// Pre-populate the graph with transactions to isolate removal
	// performance from insertion overhead.
	hashes := make([]*chainhash.Hash, b.N)
	for i := 0; i < b.N; i++ {
		tx, desc := createTestTx(nil, 1)
		g.AddTransaction(tx, desc)
		hashes[i] = tx.Hash()
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g.RemoveTransaction(*hashes[i])
	}
}

// BenchmarkGetAncestors benchmarks ancestor queries at various chain depths.
// This is critical for mempool policy enforcement, as ancestor limits are
// checked before accepting transactions (BIP 125).
func BenchmarkGetAncestors(b *testing.B) {
	benchmarkSizes := []int{10, 100, 1000}

	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("depth_%d", size), func(b *testing.B) {
			b.ReportAllocs()

			g := New(DefaultConfig())

			// Create a linear chain of transactions to test ancestor
			// traversal at different depths. This simulates worst-case
			// scenarios where transactions have deep dependency chains.
			var lastHash *chainhash.Hash
			for i := 0; i < size; i++ {
				var inputs []wire.OutPoint
				if lastHash != nil {
					inputs = append(inputs, wire.OutPoint{
						Hash:  *lastHash,
						Index: 0,
					})
				}

				tx, desc := createTestTx(inputs, 1)
				g.AddTransaction(tx, desc)
				lastHash = tx.Hash()
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				ancestors := g.GetAncestors(*lastHash, -1)
				_ = ancestors
			}
		})
	}
}

// BenchmarkGetDescendants benchmarks descendant queries at various depths.
// This is essential for RBF validation, where replacement transactions must
// account for all descendants of conflicting transactions.
func BenchmarkGetDescendants(b *testing.B) {
	benchmarkSizes := []int{10, 100, 1000}

	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("depth_%d", size), func(b *testing.B) {
			b.ReportAllocs()

			g := New(DefaultConfig())

			// Create a linear chain to test descendant traversal from the
			// root transaction. This measures the cost of walking forward
			// through dependency chains.
			var firstHash *chainhash.Hash
			var lastHash *chainhash.Hash

			for i := 0; i < size; i++ {
				var inputs []wire.OutPoint
				if lastHash != nil {
					inputs = append(inputs, wire.OutPoint{
						Hash:  *lastHash,
						Index: 0,
					})
				}

				tx, desc := createTestTx(inputs, 1)
				g.AddTransaction(tx, desc)

				if i == 0 {
					firstHash = tx.Hash()
				}
				lastHash = tx.Hash()
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				descendants := g.GetDescendants(*firstHash, -1)
				_ = descendants
			}
		})
	}
}

// BenchmarkIterateDFS benchmarks depth-first iteration over the graph.
// DFS is used for dependency-aware traversal when processing transaction
// chains in topological order matters.
func BenchmarkIterateDFS(b *testing.B) {
	graphSizes := []int{100, 1000, 5000}

	for _, size := range graphSizes {
		b.Run(fmt.Sprintf("nodes_%d", size), func(b *testing.B) {
			b.ReportAllocs()

			g := New(DefaultConfig())

			// Create a graph with independent transactions to measure the
			// cost of DFS traversal across disconnected components.
			for i := 0; i < size; i++ {
				tx, desc := createTestTx(nil, 1)
				g.AddTransaction(tx, desc)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				count := 0
				for range g.Iterate(
					WithOrder(TraversalDFS),
				) {
					count++
				}
			}
		})
	}
}

// BenchmarkIterateBFS benchmarks breadth-first iteration over the graph.
// BFS is useful for level-order traversal when analyzing transaction packages
// layer by layer, such as identifying ancestor sets at specific depths.
func BenchmarkIterateBFS(b *testing.B) {
	graphSizes := []int{100, 1000, 5000}

	for _, size := range graphSizes {
		b.Run(fmt.Sprintf("nodes_%d", size), func(b *testing.B) {
			b.ReportAllocs()

			g := New(DefaultConfig())

			// Create a graph with independent transactions to measure BFS
			// performance across multiple disconnected components.
			for i := 0; i < size; i++ {
				tx, desc := createTestTx(nil, 1)
				g.AddTransaction(tx, desc)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				count := 0
				for range g.Iterate(
					WithOrder(TraversalBFS),
				) {
					count++
				}
			}
		})
	}
}

// BenchmarkIterateTopological benchmarks topological iteration.
// Topological ordering is critical for block template construction and
// ensuring transactions are processed before their descendants.
func BenchmarkIterateTopological(b *testing.B) {
	graphSizes := []int{100, 1000, 5000}

	for _, size := range graphSizes {
		b.Run(fmt.Sprintf("nodes_%d", size), func(b *testing.B) {
			b.ReportAllocs()

			g := New(DefaultConfig())

			// Create a mix of chains and roots to test topological sort
			// performance on realistic graph structures. Breaking chains
			// every 10 transactions creates multiple entry points.
			var lastHash *chainhash.Hash
			for i := 0; i < size; i++ {
				var inputs []wire.OutPoint
				if lastHash != nil && i%10 != 0 {
					inputs = append(inputs, wire.OutPoint{
						Hash:  *lastHash,
						Index: 0,
					})
				}

				tx, desc := createTestTx(inputs, 1)
				g.AddTransaction(tx, desc)
				lastHash = tx.Hash()
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				count := 0
				for range g.Iterate(
					WithOrder(TraversalTopological),
				) {
					count++
				}
			}
		})
	}
}

// BenchmarkIdentifyPackages benchmarks package identification across the graph.
// This is used for package relay policies and determining which transaction
// groups should be evaluated together for mining and validation.
func BenchmarkIdentifyPackages(b *testing.B) {
	packageCounts := []int{10, 100, 500}

	for _, count := range packageCounts {
		b.Run(fmt.Sprintf("packages_%d", count), func(b *testing.B) {
			b.ReportAllocs()

			g := New(DefaultConfig())

			// Create 1P1C (one parent, one child) packages, which are the
			// most common pattern for CPFP. This tests the cost of scanning
			// for package structures across many independent clusters.
			for i := 0; i < count; i++ {
				parent, parentDesc := createTestTx(nil, 1)
				g.AddTransaction(parent, parentDesc)

				child, childDesc := createTestTx([]wire.OutPoint{
					{Hash: *parent.Hash(), Index: 0},
				}, 1)
				g.AddTransaction(child, childDesc)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				packages, _ := g.IdentifyPackages()
				_ = packages
			}
		})
	}
}

// BenchmarkPackageCreation benchmarks creating packages from graph nodes.
// This measures the cost of constructing TxPackage objects with computed
// aggregate metrics (total fees, sizes, topology analysis).
func BenchmarkPackageCreation(b *testing.B) {
	b.ReportAllocs()

	g := New(DefaultConfig())

	// Create a simple parent-child pair to benchmark package construction
	// overhead. The package creation involves metric aggregation and
	// topology analysis.
	parent, parentDesc := createTestTx(nil, 1)
	child, childDesc := createTestTx([]wire.OutPoint{
		{Hash: *parent.Hash(), Index: 0},
	}, 1)

	g.AddTransaction(parent, parentDesc)
	g.AddTransaction(child, childDesc)

	parentNode, _ := g.GetNode(*parent.Hash())
	childNode, _ := g.GetNode(*child.Hash())
	nodes := []*TxGraphNode{parentNode, childNode}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pkg, _ := g.CreatePackage(nodes)
		_ = pkg
	}
}

// BenchmarkClusterOperations benchmarks cluster management operations.
// Clusters are connected components in the graph, critical for RBF validation
// and mempool eviction policies.
func BenchmarkClusterOperations(b *testing.B) {
	b.Run("cluster_creation", func(b *testing.B) {
		b.ReportAllocs()

		g := New(DefaultConfig())

		b.ResetTimer()

		// Measure the cost of creating new independent clusters as
		// unrelated transactions are added to the mempool.
		for i := 0; i < b.N; i++ {
			tx, desc := createTestTx(nil, 1)
			g.AddTransaction(tx, desc)
		}

		b.ReportMetric(float64(g.GetClusterCount()), "clusters")
	})

	b.Run("cluster_merging", func(b *testing.B) {
		b.ReportAllocs()

		g := New(DefaultConfig())

		// Pre-create separate clusters that will be merged by transactions
		// spending from multiple parents. This simulates the common pattern
		// of consolidation transactions in the mempool.
		parents := make([]*chainhash.Hash, 100)
		for i := 0; i < 100; i++ {
			tx, desc := createTestTx(nil, 1)
			g.AddTransaction(tx, desc)
			parents[i] = tx.Hash()
		}

		b.ResetTimer()

		// Merge clusters by creating transactions that spend from multiple
		// parents, measuring the cost of cluster union operations.
		for i := 0; i < b.N; i++ {
			idx1 := i % len(parents)
			idx2 := (i + 1) % len(parents)

			inputs := []wire.OutPoint{
				{Hash: *parents[idx1], Index: 0},
				{Hash: *parents[idx2], Index: 0},
			}

			tx, desc := createTestTx(inputs, 1)
			g.AddTransaction(tx, desc)
		}

		b.ReportMetric(float64(g.GetClusterCount()), "final_clusters")
	})
}

// BenchmarkGetMetrics benchmarks metric collection from the graph.
// Metrics are used for monitoring mempool health and making eviction decisions
// when the mempool reaches capacity limits.
func BenchmarkGetMetrics(b *testing.B) {
	b.ReportAllocs()

	g := New(DefaultConfig())

	// Populate the graph with transactions to measure the cost of collecting
	// aggregate statistics across a realistically sized mempool graph.
	for i := 0; i < 1000; i++ {
		tx, desc := createTestTx(nil, 1)
		g.AddTransaction(tx, desc)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		metrics := g.GetMetrics()
		_ = metrics
	}
}

// BenchmarkConcurrentOperations benchmarks concurrent graph access patterns.
// The graph uses RWMutex for thread safety, allowing concurrent reads while
// serializing writes. This benchmark measures contention and throughput under
// realistic mixed workloads.
func BenchmarkConcurrentOperations(b *testing.B) {
	b.ReportAllocs()

	g := New(DefaultConfig())

	// Pre-populate the graph to provide transactions for read operations.
	// This establishes a baseline graph state before measuring concurrent
	// access patterns.
	hashes := make([]*chainhash.Hash, 1000)
	for i := 0; i < 1000; i++ {
		tx, desc := createTestTx(nil, 1)
		g.AddTransaction(tx, desc)
		hashes[i] = tx.Hash()
	}

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			idx := i % len(hashes)
			i++

			// Mix of read and write operations to simulate realistic
			// mempool access patterns: lookups, traversals, and insertions.
			switch i % 4 {
			case 0:
				g.GetNode(*hashes[idx])
			case 1:
				g.GetAncestors(*hashes[idx], 5)
			case 2:
				g.GetDescendants(*hashes[idx], 5)
			case 3:
				tx, desc := createTestTx(nil, 1)
				g.AddTransaction(tx, desc)
			}
		}
	})
}

