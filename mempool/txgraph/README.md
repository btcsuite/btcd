# txgraph: Transaction Graph for Bitcoin Mempool

The `txgraph` package provides a high-performance transaction graph data
structure for tracking relationships between unconfirmed Bitcoin transactions.
It enables efficient ancestor/descendant queries, transaction package
identification, and cluster analysis—all critical for modern mempool policies
including TRUC (v3 transactions), CPFP (Child-Pays-For-Parent), and ephemeral
dust handling.

## Why txgraph?

Bitcoin's mempool needs to understand transaction dependencies to make
intelligent relay and mining decisions. When a child transaction spends outputs
from an unconfirmed parent, the mempool must:

- **Enforce policy limits**: Ancestor/descendant count and size restrictions
(BIP 125)

- **Enable package relay**: Validate and relay transaction groups atomically

- **Calculate effective fee rates**: Consider CPFP when prioritizing transactions

- **Detect conflicts**: Identify transactions that spend the same outputs

- **Support RBF**: Compute incentive compatibility for replacements

The `txgraph` package provides the foundational graph structure that makes
these operations efficient, replacing O(n) linear scans with O(1) lookups and
cached computations.

## Core Concepts

### Transaction Graph

A **transaction graph** is a directed acyclic graph (DAG) where:

- **Nodes** represent unconfirmed transactions in the mempool

- **Edges** represent spend relationships (parent → child)

- An edge from tx A to tx B means tx B spends an output from tx A

The graph structure enables efficient traversal for ancestor/descendant queries
without repeatedly scanning the entire mempool.

### Clusters

A **cluster** is a connected component in the transaction graph—a set of
transactions that are related through spend relationships. Clusters are
important for:

- **RBF validation**: Replacement transactions must improve the fee of the
entire cluster

- **Mining optimization**: Miners can evaluate clusters as atomic units

- **Eviction policy**: When the mempool is full, low-fee clusters are
candidates for removal

### Transaction Packages

A **package** is a specific type of transaction group identified by structure
and validation rules:

- **1P1C (One Parent, One Child)**: Simple CPFP pattern with exactly one parent
and one child

- **TRUC (v3)**: BIP 431 packages with topology restrictions to prevent pinning

- **Ephemeral**: Packages containing transactions with dust outputs that must
be spent

- **Standard**: General connected transaction groups

Packages enable package-aware relay policies and optimized block template
construction.

## Installation

```bash
go get github.com/btcsuite/btcd/mempool/txgraph
```

## Quick Start

### Example 1: Building a Transaction Graph

```go
package main

import (
	"fmt"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/mempool/txgraph"
	"github.com/btcsuite/btcd/wire"
)

func main() {
	// Create a new transaction graph with default configuration.
	graph := txgraph.New(txgraph.DefaultConfig())

	// Create a parent transaction (typically from P2P relay).
	parentTx := createTransaction() 
	parentDesc := &txgraph.TxDesc{
		TxHash:      *parentTx.Hash(),
		VirtualSize: 200,
		Fee:         1000,
		FeePerKB:    5000,
	}

	// Add parent to the graph.
	if err := graph.AddTransaction(parentTx, parentDesc); err != nil {
		panic(err)
	}

	// Create a child that spends from the parent.
	childTx := createChildTransaction(parentTx) // Spends parent's output
	childDesc := &txgraph.TxDesc{
		TxHash:      *childTx.Hash(),
		VirtualSize: 150,
		Fee:         2000,
		FeePerKB:    13333,
	}

	// Add child to the graph - edges are created automatically.
	if err := graph.AddTransaction(childTx, childDesc); err != nil {
		panic(err)
	}

	// Query ancestors (returns parent).
	ancestors := graph.GetAncestors(*childTx.Hash(), -1) // -1 = unlimited depth
	fmt.Printf("Child has %d ancestor(s)\n", len(ancestors))

	// Query descendants (returns child).
	descendants := graph.GetDescendants(*parentTx.Hash(), -1)
	fmt.Printf("Parent has %d descendant(s)\n", len(descendants))
}
```

### Example 2: Iterating Over Clusters

```go
package main

import (
	"fmt"
	"github.com/btcsuite/btcd/mempool/txgraph"
)

func analyzeMempool(graph *txgraph.TxGraph) {
	// Iterate over all connected components (clusters) in the mempool.
	for cluster := range graph.IterateClusters() {
		fmt.Printf("Cluster %d:\n", cluster.ID)
		fmt.Printf("  Transactions: %d\n", cluster.Size)
		fmt.Printf("  Total fees: %d sats\n", cluster.TotalFees)
		fmt.Printf("  Total size: %d vbytes\n", cluster.TotalVSize)

		// Calculate cluster fee rate.
		if cluster.TotalVSize > 0 {
			feeRate := (cluster.TotalFees * 1000) / cluster.TotalVSize
			fmt.Printf("  Fee rate: %d sat/vB\n", feeRate)
		}

		// Identify entry points (root transactions with no parents).
		fmt.Printf("  Roots: %d\n", len(cluster.Roots))

		// Identify leaf transactions (no children, candidates for eviction).
		fmt.Printf("  Leaves: %d\n", len(cluster.Leaves))
	}
}
```

### Example 3: Package Identification and Validation

```go
package main

import (
	"fmt"
	"github.com/btcsuite/btcd/mempool/txgraph"
)

func validatePackages(graph *txgraph.TxGraph, analyzer txgraph.PackageAnalyzer) {
	// Identify all packages in the graph.
	packages, err := graph.IdentifyPackages()
	if err != nil {
		panic(err)
	}

	for _, pkg := range packages {
		fmt.Printf("Package %s (type: %v):\n",
			pkg.ID.Hash.String()[:8], pkg.Type)

		// Check package properties.
		fmt.Printf("  Transactions: %d\n", len(pkg.Transactions))
		fmt.Printf("  Total fees: %d sats\n", pkg.TotalFees)
		fmt.Printf("  Package fee rate: %d sat/vB\n", pkg.FeeRate)

		// Validate package according to its type-specific rules.
		if err := graph.ValidatePackage(pkg); err != nil {
			fmt.Printf("  ❌ Validation failed: %v\n", err)
			continue
		}
		fmt.Printf("  ✅ Valid package\n")

		// Check topology properties.
		topo := pkg.Topology
		if topo.IsLinear {
			fmt.Printf("  Structure: Linear chain (depth %d)\n", topo.MaxDepth)
		} else if topo.IsTree {
			fmt.Printf("  Structure: Tree (max width %d)\n", topo.MaxWidth)
		} else {
			fmt.Printf("  Structure: Complex DAG\n")
		}
	}
}
```

### Example 4: Advanced Iteration with Options

```go
package main

import (
	"fmt"
	"github.com/btcsuite/btcd/mempool/txgraph"
)

func findHighFeeTransactions(graph *txgraph.TxGraph, minFeeRate int64) {
	// Use functional options to configure iteration.
	highFeeFilter := func(node *txgraph.TxGraphNode) bool {
		return node.TxDesc.FeePerKB >= minFeeRate
	}

	// Iterate in fee rate order (high to low), filtered by minimum fee rate.
	for node := range graph.Iterate(
		txgraph.WithOrder(txgraph.TraversalFeeRate),
		txgraph.WithFilter(highFeeFilter),
	) {
		fmt.Printf("High-fee tx: %s (%d sat/kB)\n",
			node.TxHash.String()[:8],
			node.TxDesc.FeePerKB,
		)
	}
}

func getAncestorsWithLimit(graph *txgraph.TxGraph, txHash chainhash.Hash) {
	// Iterate ancestors up to depth 3 in BFS order.
	for ancestor := range graph.Iterate(
		txgraph.WithOrder(txgraph.TraversalBFS),
		txgraph.WithStartNode(&txHash),
		txgraph.WithDirection(txgraph.DirectionBackward),
		txgraph.WithMaxDepth(3),
		txgraph.WithIncludeStart(false), // Exclude starting transaction
	) {
		fmt.Printf("Ancestor: %s\n", ancestor.TxHash.String()[:8])
	}
}
```

## Common Patterns

### Building Graphs Incrementally

Add transactions as they arrive from P2P relay. The graph automatically creates
edges when it detects spend relationships:

```go
// As each transaction arrives...
if err := graph.AddTransaction(tx, txDesc); err != nil {
    // Handle error (duplicate, invalid, etc.)
}
// Edges to existing parents are created automatically.
```

### Package-Aware Relay

Use package identification to enable package relay policies:

```go
packages, _ := graph.IdentifyPackages()
for _, pkg := range packages {
    if pkg.Type == txgraph.PackageTypeTRUC {
        // Apply TRUC-specific relay rules
    }
}
```

### Ancestor/Descendant Limits

Enforce BIP 125 policy limits before accepting transactions:

```go
const maxAncestorCount = 25
const maxDescendantCount = 25

ancestors := graph.GetAncestors(txHash, -1)
if len(ancestors) > maxAncestorCount {
    return errors.New("exceeds ancestor limit")
}

descendants := graph.GetDescendants(txHash, -1)
if len(descendants) > maxDescendantCount {
    return errors.New("exceeds descendant limit")
}
```

### Efficient Graph Cleanup

Remove confirmed transactions efficiently when blocks arrive:

```go
for _, tx := range block.Transactions() {
    // Cascade removal: removes tx and all descendants
    graph.RemoveTransaction(*tx.Hash())
}
```

## Package Analyzer Interface

The `PackageAnalyzer` interface abstracts protocol-specific validation logic,
enabling testing and future upgrades without modifying the core graph:

```go
type PackageAnalyzer interface {
    IsTRUCTransaction(tx *wire.MsgTx) bool
    HasEphemeralDust(tx *wire.MsgTx) bool
    IsZeroFee(desc *TxDesc) bool
    ValidateTRUCPackage(nodes []*TxGraphNode) bool
    ValidateEphemeralPackage(nodes []*TxGraphNode) bool
    AnalyzePackageType(nodes []*TxGraphNode) PackageType
}
```

Implement this interface to customize package validation for your use case or
to add new package types.

## Performance Characteristics

- **Transaction lookup**: O(1) via hash map
- **Add transaction**: O(1) for graph insertion + O(k) for k parent edges
- **Remove transaction**: O(1) for node + O(d) for d descendants (cascade)
- **Ancestor/descendant query**: O(a) or O(d) where a/d is count
- **Package identification**: O(n) where n is number of root nodes

## Thread Safety

All graph operations are thread-safe and protected by a read-write mutex. Read
operations (queries, iteration) can proceed concurrently, while write
operations (add, remove) acquire exclusive access.

## Further Reading

- **API Documentation**: Run `go doc github.com/btcsuite/btcd/mempool/txgraph`
- **BIP 431 (TRUC)**: https://github.com/bitcoin/bips/blob/master/bip-0431.mediawiki
- **BIP 125 (RBF)**: https://github.com/bitcoin/bips/blob/master/bip-0125.mediawiki
