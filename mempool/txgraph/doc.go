// Package txgraph provides a transaction graph data structure for efficiently
// tracking relationships between transactions in the mempool. It supports
// package identification, ancestor/descendant queries, and various traversal
// strategies using Go's iter.Seq iterators.
//
// # Core Features
//
//   - Efficient O(1) lookups for transactions by hash
//   - Automatic edge creation based on transaction inputs/outputs
//   - Cluster management for connected components
//   - Package identification (1P1C, TRUC, ephemeral dust)
//   - Orphan transaction detection with configurable predicates
//   - Multiple traversal strategies (DFS, BFS, topological)
//   - Thread-safe operations with fine-grained locking
//
// # Graph Structure
//
// The graph maintains transactions as nodes with edges representing
// parent-child relationships. When a transaction spends outputs from
// another transaction, an edge is created from parent to child.
//
// # Packages
//
// The graph can identify various package types:
//   - 1P1C: One parent, one child packages for CPFP
//   - TRUC: Version 3 transactions with topology restrictions
//   - Ephemeral: Packages with ephemeral dust outputs
//   - Standard: General connected transaction packages
//
// # Iteration
//
// The graph supports multiple iteration strategies using iter.Seq:
//
//	// Iterate over all nodes in DFS order
//	for node := range graph.Iterate(IteratorOption{Order: TraversalDFS}) {
//	    // Process node
//	}
//
//	// Iterate over ancestors of a specific transaction
//	for node := range graph.Iterate(IteratorOption{
//	    Order:     TraversalAncestors,
//	    StartNode: txHash,
//	    MaxDepth:  10,
//	}) {
//	    // Process ancestor
//	}
//
// # Orphan Detection
//
// The graph can identify orphan transactions - transactions with unconfirmed
// inputs that are not present in the mempool. A configurable predicate function
// determines whether an input is confirmed on-chain:
//
//	// Define predicate to check if inputs are confirmed
//	isConfirmed := func(outpoint wire.OutPoint) bool {
//	    return utxoSet.IsConfirmed(outpoint)
//	}
//
//	// Get all orphan transactions
//	orphans := graph.GetOrphans(isConfirmed)
//
//	// Or iterate over orphans
//	for orphan := range graph.IterateOrphans(isConfirmed) {
//	    // Process orphan
//	}
//
// If the predicate is nil, all transactions with no parents in the graph
// are considered orphans.
//
// # Thread Safety
//
// All graph operations are thread-safe. The implementation uses a
// hierarchical locking strategy to minimize contention while maintaining
// consistency.
//
// # Example Usage
//
//	// Create a new graph
//	graph := txgraph.New(txgraph.DefaultConfig())
//
//	// Add a transaction
//	err := graph.AddTransaction(tx, txDesc)
//
//	// Get ancestors
//	ancestors := graph.GetAncestors(txHash, maxDepth)
//
//	// Identify packages
//	packages, err := graph.IdentifyPackages()
//
//	// Iterate with custom filter
//	for node := range graph.Iterate(IteratorOption{
//	    Order: TraversalFeeRate,
//	    Filter: func(n *TxGraphNode) bool {
//	        return n.TxDesc.FeePerKB > 10000
//	    },
//	}) {
//	    // Process high-fee transactions
//	}
package txgraph