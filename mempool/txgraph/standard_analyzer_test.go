// Copyright (c) 2013-2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txgraph

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// wrapNodesInPackage creates a minimal TxPackage for testing purposes.
// It computes the MaxDepth by traversing the parent relationships.
func wrapNodesInPackage(nodes []*TxGraphNode) *TxPackage {
	pkg := &TxPackage{
		Transactions: make(map[chainhash.Hash]*TxGraphNode),
		Topology:     PackageTopology{},
	}

	// Add all nodes to the package.
	for _, node := range nodes {
		pkg.Transactions[node.TxHash] = node
	}

	// Compute MaxDepth by finding the deepest path.
	maxDepth := 0
	for _, node := range nodes {
		depth := computeNodeDepth(node, pkg.Transactions)
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	pkg.Topology.MaxDepth = maxDepth

	return pkg
}

// computeNodeDepth calculates the depth of a node (number of ancestors).
func computeNodeDepth(node *TxGraphNode, pkgNodes map[chainhash.Hash]*TxGraphNode) int {
	depth := 0
	visited := make(map[chainhash.Hash]bool)
	queue := []*TxGraphNode{node}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current.TxHash] {
			continue
		}
		visited[current.TxHash] = true

		hasParentInPkg := false
		for parentHash, parentNode := range current.Parents {
			if _, inPkg := pkgNodes[parentHash]; inPkg {
				hasParentInPkg = true
				queue = append(queue, parentNode)
			}
		}

		if hasParentInPkg {
			depth++
		}
	}

	return depth
}

// TestIsTRUCTransaction verifies that the analyzer correctly identifies v3
// transactions as TRUC transactions per BIP 431.
func TestIsTRUCTransaction(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	tests := []struct {
		name     string
		version  int32
		expected bool
	}{
		{
			name:     "version 1 is not TRUC",
			version:  1,
			expected: false,
		},
		{
			name:     "version 2 is not TRUC",
			version:  2,
			expected: false,
		},
		{
			name:     "version 3 is TRUC",
			version:  3,
			expected: true,
		},
		{
			name:     "version 4 is not TRUC",
			version:  4,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx := wire.NewMsgTx(tc.version)
			result := analyzer.IsTRUCTransaction(tx)
			require.Equal(t, tc.expected, result,
				"version %d: expected TRUC=%v, got %v",
				tc.version, tc.expected, result)
		})
	}
}

// TestIsZeroFee verifies that the analyzer correctly identifies zero-fee
// transactions.
func TestIsZeroFee(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	tests := []struct {
		name     string
		fee      int64
		expected bool
	}{
		{
			name:     "zero fee transaction",
			fee:      0,
			expected: true,
		},
		{
			name:     "positive fee transaction",
			fee:      1000,
			expected: false,
		},
		{
			name:     "large fee transaction",
			fee:      100000,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			desc := &TxDesc{
				Fee: tc.fee,
			}
			result := analyzer.IsZeroFee(desc)
			require.Equal(t, tc.expected, result,
				"fee %d: expected zero=%v, got %v",
				tc.fee, tc.expected, result)
		})
	}
}

// testingT is an interface that both *testing.T and *rapid.T satisfy.
// This allows helper functions to work with both regular tests and property tests.
type testingT interface {
	Helper()
	Fatalf(format string, args ...interface{})
}

// createV3Tx generates a TRUC (version 3) transaction with specified virtual
// size for testing BIP 431 validation. The txGenerator ensures each transaction
// has unique content preventing hash collisions in graph map structures.
func createV3Tx(t testingT, vsize int64) (*btcutil.Tx, *TxDesc) {
	t.Helper()

	gen := newTxGenerator()
	btcTx, txDesc := gen.createTx([]wire.OutPoint{{Index: 0}}, 1)
	btcTx.MsgTx().Version = 3
	txDesc.VirtualSize = vsize

	btcTx = btcutil.NewTx(btcTx.MsgTx())
	txDesc.TxHash = *btcTx.Hash()

	return btcTx, txDesc
}

// createV2Tx generates a standard version 2 transaction for testing non-TRUC
// behavior and verifying that TRUC rules don't apply to v2 transactions.
func createV2Tx(t testingT, vsize int64) (*btcutil.Tx, *TxDesc) {
	t.Helper()

	gen := newTxGenerator()
	btcTx, txDesc := gen.createTx([]wire.OutPoint{{Index: 0}}, 1)
	btcTx.MsgTx().Version = 2
	txDesc.VirtualSize = vsize

	btcTx = btcutil.NewTx(btcTx.MsgTx())
	txDesc.TxHash = *btcTx.Hash()

	return btcTx, txDesc
}

// createNode constructs a TxGraphNode with initialized parent and child maps.
// The map initialization prevents nil pointer errors during graph traversal in
// TRUC validation where checking len(node.Parents) or len(node.Children) is
// required.
func createNode(tx *btcutil.Tx, desc *TxDesc) *TxGraphNode {
	return &TxGraphNode{
		TxHash:   *tx.Hash(),
		Tx:       tx,
		TxDesc:   desc,
		Parents:  make(map[chainhash.Hash]*TxGraphNode),
		Children: make(map[chainhash.Hash]*TxGraphNode),
	}
}

// TestValidateTRUCPackage_ValidSingleTx verifies that a single v3 transaction
// with no ancestors or descendants passes validation.
func TestValidateTRUCPackageValidSingleTx(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	// Create a standalone v3 transaction (no parents/children).
	tx, desc := createV3Tx(t, 5000)
	node := createNode(tx, desc)

	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{node}))
	require.True(t, result, "Single v3 tx should be valid")
}

// TestValidateTRUCPackage_Valid1P1C verifies that a valid 1-parent-1-child v3
// package passes validation (BIP 431 compliant topology).
func TestValidateTRUCPackageValid1P1C(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	// Create parent v3 transaction (9,000 vB - under 10k limit).
	parentTx, parentDesc := createV3Tx(t, 9000)
	parentNode := createNode(parentTx, parentDesc)

	// Create child v3 transaction (500 vB - under 1k limit).
	childTx, childDesc := createV3Tx(t, 500)
	childNode := createNode(childTx, childDesc)

	// Link parent and child using map syntax.
	parentNode.Children[*childTx.Hash()] = childNode
	childNode.Parents[*parentTx.Hash()] = parentNode

	// Package with both transactions.
	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode, childNode}))
	require.True(t, result, "Valid 1P1C v3 package should pass")
}

// TestValidateTRUCPackage_RejectMultipleAncestors verifies that a v3
// transaction with more than 1 unconfirmed ancestor is rejected per BIP 431
// Rule 3.
func TestValidateTRUCPackageRejectMultipleAncestors(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	// Create two parent v3 transactions.
	parent1Tx, parent1Desc := createV3Tx(t, 5000)
	parent1Node := createNode(parent1Tx, parent1Desc)

	parent2Tx, parent2Desc := createV3Tx(t, 5000)
	parent2Node := createNode(parent2Tx, parent2Desc)

	// Create child v3 transaction spending from both parents.
	childTx, childDesc := createV3Tx(t, 500)
	childNode := createNode(childTx, childDesc)

	// Link child to both parents using map syntax.
	childNode.Parents[*parent1Tx.Hash()] = parent1Node
	childNode.Parents[*parent2Tx.Hash()] = parent2Node
	parent1Node.Children[*childTx.Hash()] = childNode
	parent2Node.Children[*childTx.Hash()] = childNode

	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parent1Node, parent2Node, childNode}))
	require.False(t, result, "v3 tx with 2 unconfirmed ancestors should be rejected")
}

// TestValidateTRUCPackage_RejectMultipleDescendants verifies that a v3
// transaction with more than 1 unconfirmed descendant is rejected per BIP 431
// Rule 3.
func TestValidateTRUCPackageRejectMultipleDescendants(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	// Create parent v3 transaction.
	parentTx, parentDesc := createV3Tx(t, 5000)
	parentNode := createNode(parentTx, parentDesc)

	// Create two child v3 transactions.
	child1Tx, child1Desc := createV3Tx(t, 500)
	child1Node := createNode(child1Tx, child1Desc)

	child2Tx, child2Desc := createV3Tx(t, 500)
	child2Node := createNode(child2Tx, child2Desc)

	// Link parent to both children using map syntax.
	parentNode.Children[*child1Tx.Hash()] = child1Node
	parentNode.Children[*child2Tx.Hash()] = child2Node
	child1Node.Parents[*parentTx.Hash()] = parentNode
	child2Node.Parents[*parentTx.Hash()] = parentNode

	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode, child1Node, child2Node}))
	require.False(t, result, "v3 tx with 2 unconfirmed descendants should be rejected")
}

// TestValidateTRUCPackage_RejectParentTooLarge verifies that a v3 parent
// transaction exceeding 10,000 vB is rejected per BIP 431 Rule 4.
func TestValidateTRUCPackageRejectParentTooLarge(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	// Create parent v3 transaction that's too large (10,001 vB).
	parentTx, parentDesc := createV3Tx(t, MaxTRUCParentSize+1)
	parentNode := createNode(parentTx, parentDesc)

	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode}))
	require.False(t, result, "v3 parent >10k vB should be rejected")
}

// TestValidateTRUCPackage_AcceptParentAtLimit verifies that a v3 parent
// transaction exactly at 10,000 vB is accepted.
func TestValidateTRUCPackageAcceptParentAtLimit(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	// Create parent v3 transaction exactly at limit.
	parentTx, parentDesc := createV3Tx(t, MaxTRUCParentSize)
	parentNode := createNode(parentTx, parentDesc)

	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode}))
	require.True(t, result, "v3 parent at 10k vB should be accepted")
}

// TestValidateTRUCPackage_RejectChildTooLarge verifies that a v3 child
// transaction exceeding 1,000 vB is rejected per BIP 431 Rule 5.
func TestValidateTRUCPackageRejectChildTooLarge(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	// Create parent v3 transaction.
	parentTx, parentDesc := createV3Tx(t, 5000)
	parentNode := createNode(parentTx, parentDesc)

	// Create child v3 transaction that's too large (1,001 vB).
	childTx, childDesc := createV3Tx(t, MaxTRUCChildSize+1)
	childNode := createNode(childTx, childDesc)

	// Link parent and child using map syntax.
	parentNode.Children[*childTx.Hash()] = childNode
	childNode.Parents[*parentTx.Hash()] = parentNode

	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode, childNode}))
	require.False(t, result, "v3 child >1k vB should be rejected")
}

// TestValidateTRUCPackage_AcceptChildAtLimit verifies that a v3 child
// transaction exactly at 1,000 vB is accepted.
func TestValidateTRUCPackageAcceptChildAtLimit(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	// Create parent v3 transaction.
	parentTx, parentDesc := createV3Tx(t, 5000)
	parentNode := createNode(parentTx, parentDesc)

	// Create child v3 transaction exactly at limit.
	childTx, childDesc := createV3Tx(t, MaxTRUCChildSize)
	childNode := createNode(childTx, childDesc)

	// Link parent and child using map syntax.
	parentNode.Children[*childTx.Hash()] = childNode
	childNode.Parents[*parentTx.Hash()] = parentNode

	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode, childNode}))
	require.True(t, result, "v3 child at 1k vB should be accepted")
}

// TestValidateTRUCPackage_RejectNonV3Ancestor verifies that a v3 transaction
// with a non-v3 unconfirmed ancestor is rejected per BIP 431 Rule 2.
func TestValidateTRUCPackageRejectNonV3Ancestor(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	// Create parent v2 (non-TRUC) transaction.
	parentTx, parentDesc := createV2Tx(t, 5000)
	parentNode := createNode(parentTx, parentDesc)

	// Create child v3 transaction.
	childTx, childDesc := createV3Tx(t, 500)
	childNode := createNode(childTx, childDesc)

	// Link parent and child using map syntax.
	parentNode.Children[*childTx.Hash()] = childNode
	childNode.Parents[*parentTx.Hash()] = parentNode

	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode, childNode}))
	require.False(t, result, "v3 tx with non-v3 unconfirmed ancestor should be rejected")
}

// TestValidateTRUCPackage_RejectNonV3Descendant verifies that a v3 transaction
// with a non-v3 descendant is rejected per BIP 431 Rule 2.
func TestValidateTRUCPackageRejectNonV3Descendant(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	// Create parent v3 transaction.
	parentTx, parentDesc := createV3Tx(t, 5000)
	parentNode := createNode(parentTx, parentDesc)

	// Create child v2 (non-TRUC) transaction.
	childTx, childDesc := createV2Tx(t, 500)
	childNode := createNode(childTx, childDesc)

	// Link parent and child using map syntax.
	parentNode.Children[*childTx.Hash()] = childNode
	childNode.Parents[*parentTx.Hash()] = parentNode

	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode, childNode}))
	require.False(t, result, "v3 tx with non-v3 descendant should be rejected")
}

// TestValidateTRUCPackage_NonV3Ignored verifies that non-v3 transactions in
// the package are not validated against TRUC rules.
func TestValidateTRUCPackageNonV3Ignored(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	// Create a standalone v2 transaction with invalid TRUC topology
	// (but should be ignored since it's not v3).
	parent1Tx, parent1Desc := createV2Tx(t, 5000)
	parent1Node := createNode(parent1Tx, parent1Desc)

	parent2Tx, parent2Desc := createV2Tx(t, 5000)
	parent2Node := createNode(parent2Tx, parent2Desc)

	childTx, childDesc := createV2Tx(t, 500)
	childNode := createNode(childTx, childDesc)

	// Link child to both parents using map syntax (2 parents - invalid for v3).
	childNode.Parents[*parent1Tx.Hash()] = parent1Node
	childNode.Parents[*parent2Tx.Hash()] = parent2Node
	parent1Node.Children[*childTx.Hash()] = childNode
	parent2Node.Children[*childTx.Hash()] = childNode

	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parent1Node, parent2Node, childNode}))
	require.True(t, result, "Non-v3 transactions should not be validated by TRUC rules")
}

// TestValidateTRUCPackage_EmptyPackage verifies that an empty package is
// considered valid.
func TestValidateTRUCPackageEmptyPackage(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()
	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{}))
	require.True(t, result, "Empty package should be valid")
}

// TestValidateTRUCPackage_NilMaps verifies that nodes with nil Parents/Children
// maps are handled correctly.
func TestValidateTRUCPackageNilMaps(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	// Create a standalone v3 transaction with nil maps.
	tx, desc := createV3Tx(t, 5000)
	node := &TxGraphNode{
		TxHash:   *tx.Hash(),
		Tx:       tx,
		TxDesc:   desc,
		Parents:  nil, // Nil map instead of empty map.
		Children: nil, // Nil map instead of empty map.
	}

	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{node}))
	require.True(t, result, "v3 tx with nil Parents/Children maps should be valid")
}

// TestValidateTRUCPackage_ComplexValidTopology verifies that a more complex
// but still valid TRUC package passes validation.
func TestValidateTRUCPackageComplexValidTopology(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	// Create a chain: grandparent -> parent -> child.
	// All v3, all within size limits.
	grandparentTx, grandparentDesc := createV3Tx(t, 8000)
	grandparentNode := createNode(grandparentTx, grandparentDesc)

	parentTx, parentDesc := createV3Tx(t, 900)
	parentNode := createNode(parentTx, parentDesc)

	childTx, childDesc := createV3Tx(t, 800)
	childNode := createNode(childTx, childDesc)

	// Link grandparent -> parent.
	grandparentNode.Children[*parentTx.Hash()] = parentNode
	parentNode.Parents[*grandparentTx.Hash()] = grandparentNode

	// Link parent -> child.
	parentNode.Children[*childTx.Hash()] = childNode
	childNode.Parents[*parentTx.Hash()] = parentNode

	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{grandparentNode, parentNode, childNode}))
	require.False(t, result, "3-generation v3 chain violates BIP 431 Rule 3 (max 1 ancestor)")
}

// TestValidateTRUCPackage_BoundaryParentSize verifies boundary conditions for
// parent size limits.
func TestValidateTRUCPackageBoundaryParentSize(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	tests := []struct {
		name     string
		size     int64
		expected bool
	}{
		{
			name:     "parent at 9999 vB is valid",
			size:     9999,
			expected: true,
		},
		{
			name:     "parent at 10000 vB is valid",
			size:     10000,
			expected: true,
		},
		{
			name:     "parent at 10001 vB is invalid",
			size:     10001,
			expected: false,
		},
		{
			name:     "parent at 10002 vB is invalid",
			size:     10002,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parentTx, parentDesc := createV3Tx(t, tc.size)
			parentNode := createNode(parentTx, parentDesc)

			result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode}))
			require.Equal(t, tc.expected, result)
		})
	}
}

// TestValidateTRUCPackage_BoundaryChildSize verifies boundary conditions for
// child size limits.
func TestValidateTRUCPackageBoundaryChildSize(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	tests := []struct {
		name     string
		size     int64
		expected bool
	}{
		{
			name:     "child at 999 vB is valid",
			size:     999,
			expected: true,
		},
		{
			name:     "child at 1000 vB is valid",
			size:     1000,
			expected: true,
		},
		{
			name:     "child at 1001 vB is invalid",
			size:     1001,
			expected: false,
		},
		{
			name:     "child at 1002 vB is invalid",
			size:     1002,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parentTx, parentDesc := createV3Tx(t, 5000)
			parentNode := createNode(parentTx, parentDesc)

			childTx, childDesc := createV3Tx(t, tc.size)
			childNode := createNode(childTx, childDesc)

			// Link parent and child.
			parentNode.Children[*childTx.Hash()] = childNode
			childNode.Parents[*parentTx.Hash()] = parentNode

			result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode, childNode}))
			require.Equal(t, tc.expected, result)
		})
	}
}

// TestValidateTRUCPackage_ZeroSizeTransaction verifies handling of zero-size
// transactions.
func TestValidateTRUCPackageZeroSizeTransaction(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	// Create a v3 transaction with zero vsize (edge case).
	tx, desc := createV3Tx(t, 0)
	node := createNode(tx, desc)

	result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{node}))
	require.True(t, result, "v3 tx with 0 vB should be valid (within parent limit)")
}

// Property-based tests using rapid.

// TestValidateTRUCPackage_Property_ParentSizeInvariant uses property-based
// testing to verify that TRUC parent size limits are consistently enforced.
func TestValidateTRUCPackagePropertyParentSizeInvariant(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate random parent size (0 to 20,000 vB).
		parentSize := rapid.Int64Range(0, 20000).Draw(rt, "parent_size")

		// Create v3 parent transaction with generated size.
		parentTx, parentDesc := createV3Tx(rt, parentSize)
		parentNode := createNode(parentTx, parentDesc)

		result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode}))

		// Invariant: parent should be valid iff size <= 10,000 vB.
		expectedValid := parentSize <= MaxTRUCParentSize
		require.Equal(rt, expectedValid, result,
			"Parent size %d vB: expected valid=%v, got %v",
			parentSize, expectedValid, result)
	})
}

// TestValidateTRUCPackage_Property_ChildSizeInvariant uses property-based
// testing to verify that TRUC child size limits are consistently enforced.
func TestValidateTRUCPackagePropertyChildSizeInvariant(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate random child size (0 to 5,000 vB).
		childSize := rapid.Int64Range(0, 5000).Draw(rt, "child_size")

		// Create parent v3 transaction.
		parentTx, parentDesc := createV3Tx(rt, 5000)
		parentNode := createNode(parentTx, parentDesc)

		// Create child v3 transaction with generated size.
		childTx, childDesc := createV3Tx(rt, childSize)
		childNode := createNode(childTx, childDesc)

		// Link parent and child.
		parentNode.Children[*childTx.Hash()] = childNode
		childNode.Parents[*parentTx.Hash()] = parentNode

		result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode, childNode}))

		// Invariant: child with unconfirmed parent should be valid iff size <= 1,000 vB.
		expectedValid := childSize <= MaxTRUCChildSize
		require.Equal(rt, expectedValid, result,
			"Child size %d vB: expected valid=%v, got %v",
			childSize, expectedValid, result)
	})
}

// TestValidateTRUCPackage_Property_TopologyInvariant uses property-based
// testing to verify that TRUC topology restrictions are consistently enforced.
func TestValidateTRUCPackagePropertyTopologyInvariant(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate random topology: number of parents and children.
		numParents := rapid.IntRange(0, 5).Draw(rt, "num_parents")
		numChildren := rapid.IntRange(0, 5).Draw(rt, "num_children")

		// Create the transaction under test.
		tx, desc := createV3Tx(rt, 500)
		node := createNode(tx, desc)

		// Create parent nodes.
		for i := 0; i < numParents; i++ {
			parentTx, parentDesc := createV3Tx(rt, 500)
			parentNode := createNode(parentTx, parentDesc)
			node.Parents[*parentTx.Hash()] = parentNode
		}

		// Create child nodes.
		for i := 0; i < numChildren; i++ {
			childTx, childDesc := createV3Tx(rt, 500)
			childNode := createNode(childTx, childDesc)
			node.Children[*childTx.Hash()] = childNode
		}

		result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{node}))

		// Invariant: v3 tx should be valid iff it has ≤1 parent and ≤1 child.
		expectedValid := numParents <= 1 && numChildren <= 1
		require.Equal(rt, expectedValid, result,
			"Topology (%d parents, %d children): expected valid=%v, got %v",
			numParents, numChildren, expectedValid, result)
	})
}

// TestValidateTRUCPackage_Property_VersionInvariant verifies that only v3
// transactions are validated by TRUC rules.
func TestValidateTRUCPackagePropertyVersionInvariant(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate random transaction version.
		version := rapid.Int32Range(1, 10).Draw(rt, "version")

		// Create transaction with random version and excessive descendants
		// (would violate TRUC rules if checked).
		tx := wire.NewMsgTx(version)
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0,
			},
		})
		tx.AddTxOut(&wire.TxOut{Value: 100000000})

		btcTx := btcutil.NewTx(tx)
		desc := &TxDesc{
			TxHash:      *btcTx.Hash(),
			VirtualSize: 500,
			Fee:         1000,
		}
		node := createNode(btcTx, desc)

		// Add multiple children (would violate TRUC Rule 3 if v3).
		for i := 0; i < 3; i++ {
			childTx := wire.NewMsgTx(version)
			childTx.AddTxIn(&wire.TxIn{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: uint32(i),
				},
			})
			childTx.AddTxOut(&wire.TxOut{Value: 100000000})
			btcChildTx := btcutil.NewTx(childTx)
			childDesc := &TxDesc{
				TxHash:      *btcChildTx.Hash(),
				VirtualSize: 500,
				Fee:         1000,
			}
			childNode := createNode(btcChildTx, childDesc)
			node.Children[*btcChildTx.Hash()] = childNode
		}

		result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{node}))

		// Invariant: Only v3 transactions are validated by TRUC rules.
		// Non-v3 transactions should always pass.
		if version == TRUCVersion {
			// v3 with 3 children should fail.
			require.False(rt, result)
		} else {
			// Non-v3 with 3 children should pass (TRUC rules don't apply).
			require.True(rt, result)
		}
	})
}

// TestValidateTRUCPackage_Property_AllOrNoneTRUC verifies BIP 431 Rule 2:
// all unconfirmed ancestors and descendants must be v3 if the transaction is v3.
func TestValidateTRUCPackagePropertyAllOrNoneTRUC(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate random parent version (v2 or v3).
		parentVersion := rapid.Int32Range(2, 3).Draw(rt, "parent_version")
		// Generate random child version (v2 or v3).
		childVersion := rapid.Int32Range(2, 3).Draw(rt, "child_version")

		// Create parent transaction.
		parentTx := wire.NewMsgTx(parentVersion)
		parentTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0,
			},
		})
		parentTx.AddTxOut(&wire.TxOut{Value: 100000000})
		btcParentTx := btcutil.NewTx(parentTx)
		parentDesc := &TxDesc{
			TxHash:      *btcParentTx.Hash(),
			VirtualSize: 500,
			Fee:         1000,
		}
		parentNode := createNode(btcParentTx, parentDesc)

		// Create child transaction.
		childTx := wire.NewMsgTx(childVersion)
		childTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0,
			},
		})
		childTx.AddTxOut(&wire.TxOut{Value: 100000000})
		btcChildTx := btcutil.NewTx(childTx)
		childDesc := &TxDesc{
			TxHash:      *btcChildTx.Hash(),
			VirtualSize: 500,
			Fee:         1000,
		}
		childNode := createNode(btcChildTx, childDesc)

		// Link parent and child.
		parentNode.Children[*btcChildTx.Hash()] = childNode
		childNode.Parents[*btcParentTx.Hash()] = parentNode

		result := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode, childNode}))

		// Invariant: If either transaction is v3, both must be v3.
		if parentVersion == TRUCVersion || childVersion == TRUCVersion {
			expectedValid := parentVersion == TRUCVersion && childVersion == TRUCVersion
			require.Equal(rt, expectedValid, result,
				"Parent v%d with child v%d: expected valid=%v, got %v",
				parentVersion, childVersion, expectedValid, result)
		} else {
			// Neither is v3, should always pass.
			require.True(rt, result)
		}
	})
}

// TestValidateTRUCPackage_Property_SizeConsistency verifies that size
// validation is consistent regardless of package composition.
func TestValidateTRUCPackagePropertySizeConsistency(t *testing.T) {
	t.Parallel()

	analyzer := NewStandardPackageAnalyzer()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate random parent size.
		parentSize := rapid.Int64Range(0, 15000).Draw(rt, "parent_size")
		// Generate random child size.
		childSize := rapid.Int64Range(0, 2000).Draw(rt, "child_size")

		// Create parent v3 transaction.
		parentTx, parentDesc := createV3Tx(rt, parentSize)
		parentNode := createNode(parentTx, parentDesc)

		// Test parent alone.
		parentAloneValid := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode}))

		// Create child v3 transaction.
		childTx, childDesc := createV3Tx(rt, childSize)
		childNode := createNode(childTx, childDesc)

		// Link parent and child.
		parentNode.Children[*childTx.Hash()] = childNode
		childNode.Parents[*parentTx.Hash()] = parentNode

		// Test package together.
		packageValid := analyzer.ValidateTRUCPackage(wrapNodesInPackage([]*TxGraphNode{parentNode, childNode}))

		// Invariant: Parent size validation should be consistent.
		if parentSize > MaxTRUCParentSize {
			require.False(rt, parentAloneValid, "Oversized parent should fail alone")
			require.False(rt, packageValid, "Oversized parent should fail in package")
		}

		// Invariant: Child size validation should apply when it has a parent.
		if parentSize <= MaxTRUCParentSize && childSize > MaxTRUCChildSize {
			require.False(rt, packageValid, "Oversized child should fail in package")
		}

		// Invariant: Valid parent and valid child should pass together.
		if parentSize <= MaxTRUCParentSize && childSize <= MaxTRUCChildSize {
			require.True(rt, packageValid, "Valid sizes should pass together")
		}
	})
}
