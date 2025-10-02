// Copyright (c) 2013-2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mempool/txgraph"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// mockGraph is a simple mock implementation of the Graph interface for testing
// policy enforcement without requiring a full graph implementation.
type mockGraph struct {
	nodes map[chainhash.Hash]*txgraph.TxGraphNode
}

func newMockGraph() *mockGraph {
	return &mockGraph{
		nodes: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
}

func (m *mockGraph) addNode(node *txgraph.TxGraphNode) {
	m.nodes[node.TxHash] = node
}

func (m *mockGraph) GetNode(hash chainhash.Hash) (*txgraph.TxGraphNode, bool) {
	node, exists := m.nodes[hash]
	return node, exists
}

func (m *mockGraph) GetAncestors(hash chainhash.Hash,
	maxDepth int) map[chainhash.Hash]*txgraph.TxGraphNode {

	ancestors := make(map[chainhash.Hash]*txgraph.TxGraphNode)
	m.getAncestorsRecursive(hash, ancestors, 0, maxDepth)

	return ancestors
}

func (m *mockGraph) getAncestorsRecursive(hash chainhash.Hash,
	result map[chainhash.Hash]*txgraph.TxGraphNode, depth, maxDepth int) {

	if maxDepth >= 0 && depth > maxDepth {
		return
	}

	node, exists := m.nodes[hash]
	if !exists {
		return
	}

	for parentHash, parent := range node.Parents {
		if _, seen := result[parentHash]; seen {
			continue
		}

		result[parentHash] = parent
		m.getAncestorsRecursive(parentHash, result, depth+1, maxDepth)
	}
}

func (m *mockGraph) GetDescendants(hash chainhash.Hash,
	maxDepth int) map[chainhash.Hash]*txgraph.TxGraphNode {

	descendants := make(map[chainhash.Hash]*txgraph.TxGraphNode)
	m.getDescendantsRecursive(hash, descendants, 0, maxDepth)
	return descendants
}

func (m *mockGraph) getDescendantsRecursive(hash chainhash.Hash,
	result map[chainhash.Hash]*txgraph.TxGraphNode, depth, maxDepth int) {

	if maxDepth >= 0 && depth > maxDepth {
		return
	}

	node, exists := m.nodes[hash]
	if !exists {
		return
	}

	for childHash, child := range node.Children {
		if _, seen := result[childHash]; seen {
			continue
		}

		result[childHash] = child

		m.getDescendantsRecursive(childHash, result, depth+1, maxDepth)
	}
}

// Ensure mockGraph implements the necessary Graph interface methods.
var _ interface {
	GetNode(chainhash.Hash) (*txgraph.TxGraphNode, bool)
	GetAncestors(chainhash.Hash, int) map[chainhash.Hash]*txgraph.TxGraphNode
	GetDescendants(chainhash.Hash, int) map[chainhash.Hash]*txgraph.TxGraphNode
} = (*mockGraph)(nil)

func createTxWithSequence(sequences []uint32) *btcutil.Tx {
	mtx := wire.NewMsgTx(wire.TxVersion)

	for _, seq := range sequences {
		mtx.AddTxIn(&wire.TxIn{
			Sequence: seq,
		})
	}

	mtx.AddTxOut(&wire.TxOut{
		Value:    100000,
		PkScript: []byte{0x51},
	})

	return btcutil.NewTx(mtx)
}

func createTxWithInputs(inputs []wire.OutPoint) *btcutil.Tx {
	mtx := wire.NewMsgTx(wire.TxVersion)

	for _, input := range inputs {
		mtx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: input,
			Sequence:         wire.MaxTxInSequenceNum,
		})
	}

	mtx.AddTxOut(&wire.TxOut{
		Value:    100000,
		PkScript: []byte{0x51},
	})

	return btcutil.NewTx(mtx)
}

// TestSignalsReplacementExplicit tests that transactions with low sequence
// numbers explicitly signal RBF.
func TestSignalsReplacementExplicit(t *testing.T) {
	t.Parallel()

	cfg := DefaultPolicyConfig()
	p := NewStandardPolicyEnforcer(cfg)
	graph := newMockGraph()

	tests := []struct {
		name      string
		sequences []uint32
		signals   bool
	}{
		{
			name:      "no signaling - all max sequence",
			sequences: []uint32{wire.MaxTxInSequenceNum},
			signals:   false,
		},
		{
			name:      "explicit signaling - one low sequence",
			sequences: []uint32{MaxRBFSequence},
			signals:   true,
		},
		{
			name:      "explicit signaling - below threshold",
			sequences: []uint32{MaxRBFSequence - 1},
			signals:   true,
		},
		{
			name:      "no signaling - above threshold",
			sequences: []uint32{MaxRBFSequence + 1},
			signals:   false,
		},
		{
			name: "explicit signaling - mixed sequences",
			sequences: []uint32{
				wire.MaxTxInSequenceNum, MaxRBFSequence,
			},
			signals: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := createTxWithSequence(tt.sequences)
			signals := p.SignalsReplacement(graph, tx)
			require.Equal(t, tt.signals, signals)
		})
	}
}

// TestSignalsReplacementInherited tests that transactions inherit RBF
// signaling from unconfirmed ancestors.
func TestSignalsReplacementInherited(t *testing.T) {
	t.Parallel()

	cfg := DefaultPolicyConfig()
	p := NewStandardPolicyEnforcer(cfg)
	graph := newMockGraph()

	// Create a parent that signals RBF.
	parent := createTxWithSequence([]uint32{MaxRBFSequence})
	parentNode := &txgraph.TxGraphNode{
		Tx:     parent,
		TxHash: *parent.Hash(),
		TxDesc: &txgraph.TxDesc{
			VirtualSize: 100,
			Fee:         1000,
		},
		Parents:  make(map[chainhash.Hash]*txgraph.TxGraphNode),
		Children: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
	graph.addNode(parentNode)

	// Create a child that doesn't explicitly signal but spends from parent.
	child := createTxWithInputs(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}},
	)
	child.MsgTx().TxIn[0].Sequence = wire.MaxTxInSequenceNum

	// Child should inherit RBF signaling from parent.
	signals := p.SignalsReplacement(graph, child)
	require.True(
		t, signals, "child should inherit RBF signaling from parent",
	)
}

// TestSignalsReplacementInheritedDeep tests RBF signaling inheritance through
// multiple generations of ancestors.
func TestSignalsReplacementInheritedDeep(t *testing.T) {
	t.Parallel()

	cfg := DefaultPolicyConfig()
	p := NewStandardPolicyEnforcer(cfg)
	graph := newMockGraph()

	// Create a grandparent that signals RBF.
	grandparent := createTxWithSequence([]uint32{MaxRBFSequence})
	grandparentNode := &txgraph.TxGraphNode{
		Tx:       grandparent,
		TxHash:   *grandparent.Hash(),
		TxDesc:   &txgraph.TxDesc{VirtualSize: 100, Fee: 1000},
		Parents:  make(map[chainhash.Hash]*txgraph.TxGraphNode),
		Children: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
	graph.addNode(grandparentNode)

	// Create a parent that doesn't signal.
	parent := createTxWithInputs(
		[]wire.OutPoint{{Hash: *grandparent.Hash(), Index: 0}},
	)
	parent.MsgTx().TxIn[0].Sequence = wire.MaxTxInSequenceNum
	parentNode := &txgraph.TxGraphNode{
		Tx:     parent,
		TxHash: *parent.Hash(),
		TxDesc: &txgraph.TxDesc{VirtualSize: 100, Fee: 1000},
		Parents: map[chainhash.Hash]*txgraph.TxGraphNode{
			*grandparent.Hash(): grandparentNode,
		},
		Children: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
	grandparentNode.Children[*parent.Hash()] = parentNode
	graph.addNode(parentNode)

	// Create a child that doesn't signal.
	child := createTxWithInputs(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}},
	)
	child.MsgTx().TxIn[0].Sequence = wire.MaxTxInSequenceNum

	// Child should inherit RBF signaling from grandparent.
	signals := p.SignalsReplacement(graph, child)
	require.True(
		t, signals, "child should inherit RBF signaling from "+
			"grandparent",
	)
}

// TestValidateReplacementBasic tests basic RBF replacement validation.
func TestValidateReplacementBasic(t *testing.T) {
	t.Parallel()

	cfg := DefaultPolicyConfig()
	p := NewStandardPolicyEnforcer(cfg)
	graph := newMockGraph()

	// Create a conflict transaction.
	conflict := createTxWithSequence([]uint32{MaxRBFSequence})
	conflictNode := &txgraph.TxGraphNode{
		Tx:     conflict,
		TxHash: *conflict.Hash(),
		TxDesc: &txgraph.TxDesc{
			VirtualSize: 100,
			Fee:         1000, // 10 sat/vbyte
		},
		Parents:  make(map[chainhash.Hash]*txgraph.TxGraphNode),
		Children: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
	graph.addNode(conflictNode)

	conflicts := &txgraph.ConflictSet{
		Transactions: map[chainhash.Hash]*txgraph.TxGraphNode{
			*conflict.Hash(): conflictNode,
		},
		Packages: make(map[txgraph.PackageID]*txgraph.TxPackage),
	}

	// Create a replacement with higher fee rate and absolute fee.
	replacement := createTxWithSequence([]uint32{MaxRBFSequence})
	replacementSize := GetTxVirtualSize(replacement)
	replacementFee := int64(1000 + calcMinRequiredTxRelayFee(
		replacementSize, 1000),
	)

	err := p.ValidateReplacement(
		graph, replacement, replacementFee, conflicts,
	)
	require.NoError(t, err, "valid replacement should be accepted")
}

// TestValidateReplacementInsufficientFeeRate tests that replacements with
// insufficient fee rates are rejected.
func TestValidateReplacementInsufficientFeeRate(t *testing.T) {
	t.Parallel()

	cfg := DefaultPolicyConfig()
	p := NewStandardPolicyEnforcer(cfg)
	graph := newMockGraph()

	conflict := createTxWithSequence([]uint32{MaxRBFSequence})
	conflictNode := &txgraph.TxGraphNode{
		Tx:     conflict,
		TxHash: *conflict.Hash(),
		TxDesc: &txgraph.TxDesc{
			VirtualSize: 100,
			Fee:         10000,
		},
		Parents:  make(map[chainhash.Hash]*txgraph.TxGraphNode),
		Children: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
	graph.addNode(conflictNode)

	conflicts := &txgraph.ConflictSet{
		Transactions: map[chainhash.Hash]*txgraph.TxGraphNode{
			*conflict.Hash(): conflictNode,
		},
		Packages: make(map[txgraph.PackageID]*txgraph.TxPackage),
	}

	// Replacement with higher absolute fee but same/lower fee rate (should
	// fail). To make the fee rate lower while absolute fee is higher, make
	// the tx larger.
	replacement := createTxWithSequence(
		[]uint32{MaxRBFSequence, MaxRBFSequence},
	)
	replacementSize := GetTxVirtualSize(replacement)
	minAbsoluteFee := int64(10000 + calcMinRequiredTxRelayFee(
		replacementSize, 1000),
	)
	replacementFee := minAbsoluteFee + 10

	err := p.ValidateReplacement(
		graph, replacement, replacementFee, conflicts,
	)
	require.ErrorIs(t, err, ErrInsufficientFeeRate)
}

// TestValidateReplacementTooManyEvictions tests that replacements evicting
// too many transactions are rejected.
func TestValidateReplacementTooManyEvictions(t *testing.T) {
	t.Parallel()

	cfg := DefaultPolicyConfig()
	cfg.MaxReplacementEvictions = 2
	p := NewStandardPolicyEnforcer(cfg)
	graph := newMockGraph()

	// Create 3 conflicts (exceeds limit of 2). Each conflict must have a
	// unique hash, so we create unique transactions.
	conflicts := &txgraph.ConflictSet{
		Transactions: make(map[chainhash.Hash]*txgraph.TxGraphNode),
		Packages:     make(map[txgraph.PackageID]*txgraph.TxPackage),
	}

	for i := 0; i < 3; i++ {
		tx := createTxWithSequence([]uint32{MaxRBFSequence})

		// Add unique output value to ensure different hashes.
		tx.MsgTx().TxOut[0].Value = int64(100000 + i)
		node := &txgraph.TxGraphNode{
			Tx:      tx,
			TxHash:  *tx.Hash(),
			TxDesc:  &txgraph.TxDesc{VirtualSize: 100, Fee: 1000},
			Parents: make(map[chainhash.Hash]*txgraph.TxGraphNode),
		}
		conflicts.Transactions[*tx.Hash()] = node
	}

	replacement := createTxWithSequence([]uint32{MaxRBFSequence})
	replacementFee := int64(10000)

	err := p.ValidateReplacement(
		graph, replacement, replacementFee, conflicts,
	)
	require.ErrorIs(t, err, ErrTooManyEvictions)
}

// TestValidateAncestorLimits tests ancestor count and size limit enforcement.
func TestValidateAncestorLimits(t *testing.T) {
	t.Parallel()

	cfg := DefaultPolicyConfig()
	cfg.MaxAncestorCount = 3
	cfg.MaxAncestorSize = 300
	p := NewStandardPolicyEnforcer(cfg)
	graph := newMockGraph()

	// Create a chain: A -> B -> C.
	txA := createTxWithSequence([]uint32{wire.MaxTxInSequenceNum})
	nodeA := &txgraph.TxGraphNode{
		Tx:       txA,
		TxHash:   *txA.Hash(),
		TxDesc:   &txgraph.TxDesc{VirtualSize: 100, Fee: 1000},
		Parents:  make(map[chainhash.Hash]*txgraph.TxGraphNode),
		Children: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
	graph.addNode(nodeA)

	txB := createTxWithInputs(
		[]wire.OutPoint{{Hash: *txA.Hash(), Index: 0}},
	)
	nodeB := &txgraph.TxGraphNode{
		Tx:     txB,
		TxHash: *txB.Hash(),
		TxDesc: &txgraph.TxDesc{VirtualSize: 100, Fee: 1000},
		Parents: map[chainhash.Hash]*txgraph.TxGraphNode{
			*txA.Hash(): nodeA,
		},
		Children: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
	nodeA.Children[*txB.Hash()] = nodeB
	graph.addNode(nodeB)

	txC := createTxWithInputs(
		[]wire.OutPoint{{Hash: *txB.Hash(), Index: 0}},
	)
	nodeC := &txgraph.TxGraphNode{
		Tx:     txC,
		TxHash: *txC.Hash(),
		TxDesc: &txgraph.TxDesc{VirtualSize: 100, Fee: 1000},
		Parents: map[chainhash.Hash]*txgraph.TxGraphNode{
			*txB.Hash(): nodeB,
		},
		Children: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
	nodeB.Children[*txC.Hash()] = nodeC
	graph.addNode(nodeC)

	// C has 2 ancestors (A, B) + itself = 3 total (at limit).
	err := p.ValidateAncestorLimits(graph, *txC.Hash())
	require.NoError(t, err, "should accept transaction at ancestor limit")

	// Total size: 100 + 100 + 100 = 300 bytes (at limit).
	require.NoError(t, err, "should accept transaction at size limit")
}

// TestValidateAncestorLimitsExceeded tests that exceeding ancestor limits is
// rejected.
func TestValidateAncestorLimitsExceeded(t *testing.T) {
	t.Parallel()

	cfg := DefaultPolicyConfig()
	cfg.MaxAncestorCount = 2
	p := NewStandardPolicyEnforcer(cfg)
	graph := newMockGraph()

	// Create a chain: A -> B -> C (C will have 3 total including itself).
	txA := createTxWithSequence([]uint32{wire.MaxTxInSequenceNum})
	nodeA := &txgraph.TxGraphNode{
		Tx:       txA,
		TxHash:   *txA.Hash(),
		TxDesc:   &txgraph.TxDesc{VirtualSize: 100, Fee: 1000},
		Parents:  make(map[chainhash.Hash]*txgraph.TxGraphNode),
		Children: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
	graph.addNode(nodeA)

	txB := createTxWithInputs(
		[]wire.OutPoint{{Hash: *txA.Hash(), Index: 0}},
	)
	nodeB := &txgraph.TxGraphNode{
		Tx:     txB,
		TxHash: *txB.Hash(),
		TxDesc: &txgraph.TxDesc{VirtualSize: 100, Fee: 1000},
		Parents: map[chainhash.Hash]*txgraph.TxGraphNode{
			*txA.Hash(): nodeA,
		},
		Children: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
	nodeA.Children[*txB.Hash()] = nodeB
	graph.addNode(nodeB)

	txC := createTxWithInputs(
		[]wire.OutPoint{{Hash: *txB.Hash(), Index: 0}},
	)
	nodeC := &txgraph.TxGraphNode{
		Tx:     txC,
		TxHash: *txC.Hash(),
		TxDesc: &txgraph.TxDesc{VirtualSize: 100, Fee: 1000},
		Parents: map[chainhash.Hash]*txgraph.TxGraphNode{
			*txB.Hash(): nodeB,
		},
		Children: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
	graph.addNode(nodeC)

	// C has 2 ancestors + itself = 3 total (exceeds limit of 2).
	err := p.ValidateAncestorLimits(graph, *txC.Hash())
	require.ErrorIs(t, err, ErrExceededAncestorLimit)
}

// TestValidateDescendantLimits tests descendant count and size limit
// enforcement.
func TestValidateDescendantLimits(t *testing.T) {
	t.Parallel()

	cfg := DefaultPolicyConfig()
	cfg.MaxDescendantCount = 2
	cfg.MaxDescendantSize = 200
	p := NewStandardPolicyEnforcer(cfg)
	graph := newMockGraph()

	// Create a chain: A -> B -> C
	txA := createTxWithSequence([]uint32{wire.MaxTxInSequenceNum})
	nodeA := &txgraph.TxGraphNode{
		Tx:       txA,
		TxHash:   *txA.Hash(),
		TxDesc:   &txgraph.TxDesc{VirtualSize: 100, Fee: 1000},
		Parents:  make(map[chainhash.Hash]*txgraph.TxGraphNode),
		Children: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
	graph.addNode(nodeA)

	txB := createTxWithInputs(
		[]wire.OutPoint{{Hash: *txA.Hash(), Index: 0}},
	)
	nodeB := &txgraph.TxGraphNode{
		Tx:     txB,
		TxHash: *txB.Hash(),
		TxDesc: &txgraph.TxDesc{VirtualSize: 100, Fee: 1000},
		Parents: map[chainhash.Hash]*txgraph.TxGraphNode{
			*txA.Hash(): nodeA,
		},
		Children: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
	nodeA.Children[*txB.Hash()] = nodeB
	graph.addNode(nodeB)

	txC := createTxWithInputs(
		[]wire.OutPoint{{Hash: *txB.Hash(), Index: 0}},
	)
	nodeC := &txgraph.TxGraphNode{
		Tx:     txC,
		TxHash: *txC.Hash(),
		TxDesc: &txgraph.TxDesc{VirtualSize: 100, Fee: 1000},
		Parents: map[chainhash.Hash]*txgraph.TxGraphNode{
			*txB.Hash(): nodeB,
		},
		Children: make(map[chainhash.Hash]*txgraph.TxGraphNode),
	}
	nodeB.Children[*txC.Hash()] = nodeC
	graph.addNode(nodeC)

	// A has 2 descendants (B, C) (at limit).
	err := p.ValidateDescendantLimits(graph, *txA.Hash())
	require.NoError(t, err, "should accept transaction at descendant limit")
}

// TestValidateRelayFee tests relay fee validation with and without rate limiting.
func TestValidateRelayFee(t *testing.T) {
	t.Parallel()

	cfg := DefaultPolicyConfig()
	p := NewStandardPolicyEnforcer(cfg)

	tx := createTxWithSequence([]uint32{wire.MaxTxInSequenceNum})
	size := GetTxVirtualSize(tx)
	minFee := calcMinRequiredTxRelayFee(size, cfg.MinRelayTxFee)

	// Test with sufficient fee.
	err := p.ValidateRelayFee(tx, minFee, size, true)
	require.NoError(t, err, "should accept transaction with sufficient fee")

	// Test with insufficient fee (should be rate limited for new tx).
	err = p.ValidateRelayFee(tx, minFee-1, size, true)
	require.NoError(t, err, "should accept new low-fee tx (rate limited)")

	// Test with insufficient fee for non-new tx.
	err = p.ValidateRelayFee(tx, minFee-1, size, false)
	require.Error(t, err, "should reject non-new low-fee tx")
}

// TestRateLimiting tests that the rate limiter correctly limits free
// transactions.
func TestRateLimiting(t *testing.T) {
	t.Parallel()

	cfg := DefaultPolicyConfig()
	cfg.FreeTxRelayLimit = 0.001
	p := NewStandardPolicyEnforcer(cfg)

	tx := createTxWithSequence([]uint32{wire.MaxTxInSequenceNum})
	size := GetTxVirtualSize(tx)

	// First free tx should be accepted.
	err := p.ValidateRelayFee(tx, 0, size, true)
	require.NoError(t, err)

	// Second free tx should be rejected (rate limited).
	err = p.ValidateRelayFee(tx, 0, size, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "rate limiter")
}

// TestRateLimiterDecay tests that the rate limiter's exponential decay works
// correctly.
func TestRateLimiterDecay(t *testing.T) {
	t.Parallel()

	cfg := DefaultPolicyConfig()
	cfg.FreeTxRelayLimit = 0.1
	p := NewStandardPolicyEnforcer(cfg)

	tx := createTxWithSequence([]uint32{wire.MaxTxInSequenceNum})
	size := int64(50)

	// Accept first transaction.
	err := p.ValidateRelayFee(tx, 0, size, true)
	require.NoError(t, err)

	// Manually advance time to simulate decay.
	p.mu.Lock()

	// 10 minutes ago.
	p.lastPennyUnix -= 600

	p.mu.Unlock()

	// Second transaction should be accepted after decay.
	err = p.ValidateRelayFee(tx, 0, size, true)
	require.NoError(t, err)
}

// TestPropertySignalsReplacementTransitive tests that RBF signaling is
// transitively inherited through a chain of transactions.
func TestPropertySignalsReplacementTransitive(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		cfg := DefaultPolicyConfig()
		p := NewStandardPolicyEnforcer(cfg)
		graph := newMockGraph()

		// Generate a random chain depth (1-10).
		depth := rapid.IntRange(1, 10).Draw(t, "chain_depth")

		// Build a chain where the root signals RBF.
		var (
			prevTx   *btcutil.Tx
			prevNode *txgraph.TxGraphNode
		)

		for i := 0; i < depth; i++ {
			var tx *btcutil.Tx
			if i == 0 {
				// Root signals RBF.
				tx = createTxWithSequence(
					[]uint32{MaxRBFSequence},
				)
			} else {
				// Descendants don't explicitly signal.
				tx = createTxWithInputs([]wire.OutPoint{
					{Hash: *prevTx.Hash(), Index: 0},
				})
				tx.MsgTx().TxIn[0].Sequence = wire.MaxTxInSequenceNum
			}

			node := &txgraph.TxGraphNode{
				Tx:     tx,
				TxHash: *tx.Hash(),
				TxDesc: &txgraph.TxDesc{
					VirtualSize: 100, Fee: 1000,
				},
				Parents: make(
					map[chainhash.Hash]*txgraph.TxGraphNode,
				),
				Children: make(
					map[chainhash.Hash]*txgraph.TxGraphNode,
				),
			}

			if prevNode != nil {
				node.Parents[*prevTx.Hash()] = prevNode
				prevNode.Children[*tx.Hash()] = node
			}

			graph.addNode(node)
			prevTx = tx
			prevNode = node
		}

		// All descendants should signal RBF due to inherited signaling.
		for hash := range graph.nodes {
			node, _ := graph.GetNode(hash)
			signals := p.SignalsReplacement(graph, node.Tx)
			if !signals {
				t.Fatalf("transaction %v should "+
					"signal RBF (inherited from root)",
					hash)
			}
		}
	})
}

// TestPropertyAncestorLimitEnforced generates chains of varying lengths and
// ensures that the ancestor limit is correctly enforced.
func TestPropertyAncestorLimitEnforced(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		// Random ancestor limit (5-30).
		limit := rapid.IntRange(5, 30).Draw(t, "ancestor_limit")

		cfg := DefaultPolicyConfig()
		cfg.MaxAncestorCount = limit
		cfg.MaxAncestorSize = 1000000
		p := NewStandardPolicyEnforcer(cfg)
		graph := newMockGraph()

		// Build a chain that exceeds the limit.
		chainLength := limit + rapid.IntRange(1, 10).Draw(t, "extra_depth")

		var (
			prevTx   *btcutil.Tx
			prevNode *txgraph.TxGraphNode
			lastHash chainhash.Hash
		)

		for i := 0; i < chainLength; i++ {
			var tx *btcutil.Tx
			if prevTx == nil {
				tx = createTxWithSequence(
					[]uint32{wire.MaxTxInSequenceNum},
				)
			} else {
				tx = createTxWithInputs([]wire.OutPoint{
					{Hash: *prevTx.Hash(), Index: 0},
				})
			}

			node := &txgraph.TxGraphNode{
				Tx:     tx,
				TxHash: *tx.Hash(),
				TxDesc: &txgraph.TxDesc{
					VirtualSize: 100, Fee: 1000,
				},
				Parents: make(
					map[chainhash.Hash]*txgraph.TxGraphNode,
				),
				Children: make(
					map[chainhash.Hash]*txgraph.TxGraphNode,
				),
			}

			if prevNode != nil {
				node.Parents[*prevTx.Hash()] = prevNode
				prevNode.Children[*tx.Hash()] = node
			}

			graph.addNode(node)
			prevTx = tx
			prevNode = node
			lastHash = *tx.Hash()
		}

		// The last transaction in the chain should exceed the ancestor
		// limit.
		err := p.ValidateAncestorLimits(graph, lastHash)
		if chainLength > limit {
			if err == nil {
				t.Fatalf("expected ancestor limit "+
					"error for chain length %d "+
					"(limit %d)", chainLength, limit)
			}
			require.ErrorIs(t, err, ErrExceededAncestorLimit)
		} else {
			require.NoError(t, err)
		}
	})
}

func TestPropertyFeeRateMonotonic(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		cfg := DefaultPolicyConfig()
		p := NewStandardPolicyEnforcer(cfg)

		// Generate random transaction size and fee.
		size := rapid.Int64Range(100, 100000).Draw(t, "size")
		fee1 := rapid.Int64Range(1000, 1000000).Draw(t, "fee1")
		fee2 := rapid.Int64Range(fee1+1, fee1+100000).Draw(t, "fee2")

		tx := createTxWithSequence([]uint32{wire.MaxTxInSequenceNum})

		// Higher fee should always be accepted if lower fee is accepted.
		err1 := p.ValidateRelayFee(tx, fee1, size, false)
		err2 := p.ValidateRelayFee(tx, fee2, size, false)

		if err1 == nil && err2 != nil {
			t.Fatalf("monotonicity violated: fee %d accepted but %d rejected",
				fee1, fee2)
		}
	})
}
