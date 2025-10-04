package mempool

import (
	"errors"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// txCounter ensures unique transaction hashes.
var txCounter = 0

// createTestOrphanTx creates a test transaction with the given inputs and outputs.
func createTestOrphanTx(inputs []wire.OutPoint, numOutputs int) *btcutil.Tx {
	mtx := wire.NewMsgTx(wire.TxVersion)

	// Add inputs. If no inputs provided, create a dummy input with unique hash.
	if len(inputs) == 0 {
		txCounter++
		mtx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Index: uint32(txCounter),
			},
		})
	} else {
		for _, input := range inputs {
			mtx.AddTxIn(&wire.TxIn{
				PreviousOutPoint: input,
			})
		}
	}

	// Add outputs.
	for i := 0; i < numOutputs; i++ {
		mtx.AddTxOut(&wire.TxOut{
			Value:    100000,
			PkScript: []byte{0x51}, // OP_TRUE
		})
	}

	return btcutil.NewTx(mtx)
}

// TestOrphanManagerAddOrphan verifies that AddOrphan correctly adds orphans
// and enforces size and count limits.
func TestOrphanManagerAddOrphan(t *testing.T) {
	cfg := DefaultOrphanConfig()
	cfg.MaxOrphans = 3
	cfg.MaxOrphanSize = 1000
	om := NewOrphanManager(cfg)

	// Add first orphan.
	tx1 := createTestOrphanTx(nil, 1)
	err := om.AddOrphan(tx1, Tag(1))
	require.NoError(t, err)
	require.Equal(t, 1, om.Count())
	require.True(t, om.IsOrphan(*tx1.Hash()))

	// Add second orphan from different peer.
	tx2 := createTestOrphanTx(nil, 1)
	err = om.AddOrphan(tx2, Tag(2))
	require.NoError(t, err)
	require.Equal(t, 2, om.Count())

	// Try to add duplicate.
	err = om.AddOrphan(tx1, Tag(1))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrOrphanAlreadyExists)
	require.Equal(t, 2, om.Count())

	// Add third orphan to reach limit.
	tx3 := createTestOrphanTx(nil, 1)
	err = om.AddOrphan(tx3, Tag(3))
	require.NoError(t, err)
	require.Equal(t, 3, om.Count())

	// Try to add fourth orphan, should fail due to limit.
	tx4 := createTestOrphanTx(nil, 1)
	err = om.AddOrphan(tx4, Tag(4))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrOrphanLimitReached)
	require.Equal(t, 3, om.Count())
}

// TestOrphanManagerSizeLimit verifies that AddOrphan rejects transactions
// that exceed MaxOrphanSize.
func TestOrphanManagerSizeLimit(t *testing.T) {
	cfg := DefaultOrphanConfig()
	cfg.MaxOrphanSize = 100 // Very small limit.
	om := NewOrphanManager(cfg)

	// Create a large transaction.
	inputs := make([]wire.OutPoint, 50)
	largeTx := createTestOrphanTx(inputs, 50)

	err := om.AddOrphan(largeTx, Tag(1))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrOrphanTooLarge)
	require.Equal(t, 0, om.Count())
}

// TestOrphanManagerRemoveOrphan verifies that RemoveOrphan correctly removes
// orphans with and without cascading to descendants.
func TestOrphanManagerRemoveOrphan(t *testing.T) {
	om := NewOrphanManager(DefaultOrphanConfig())

	// Build a chain: parent -> child -> grandchild.
	parent := createTestOrphanTx(nil, 1)
	require.NoError(t, om.AddOrphan(parent, Tag(1)))

	child := createTestOrphanTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, om.AddOrphan(child, Tag(1)))

	grandchild := createTestOrphanTx(
		[]wire.OutPoint{{Hash: *child.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, om.AddOrphan(grandchild, Tag(1)))

	require.Equal(t, 3, om.Count())

	// Remove child with cascade=false. This should remove just the child,
	// leaving parent and grandchild (grandchild becomes disconnected).
	err := om.RemoveOrphan(*child.Hash(), false)
	require.NoError(t, err)
	require.Equal(t, 2, om.Count())
	require.False(t, om.IsOrphan(*child.Hash()))
	require.True(t, om.IsOrphan(*parent.Hash()))
	require.True(t, om.IsOrphan(*grandchild.Hash()))

	// Remove parent with cascade=true. Since grandchild is disconnected from
	// parent, it should not be affected.
	err = om.RemoveOrphan(*parent.Hash(), true)
	require.NoError(t, err)
	require.Equal(t, 1, om.Count())
	require.False(t, om.IsOrphan(*parent.Hash()))
	require.True(t, om.IsOrphan(*grandchild.Hash()))

	// Build a new chain to test cascade=true.
	parent2 := createTestOrphanTx(nil, 1)
	require.NoError(t, om.AddOrphan(parent2, Tag(2)))

	child2 := createTestOrphanTx(
		[]wire.OutPoint{{Hash: *parent2.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, om.AddOrphan(child2, Tag(2)))

	grandchild2 := createTestOrphanTx(
		[]wire.OutPoint{{Hash: *child2.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, om.AddOrphan(grandchild2, Tag(2)))

	require.Equal(t, 4, om.Count())

	// Remove child2 with cascade=true. This should remove child2 and grandchild2.
	err = om.RemoveOrphan(*child2.Hash(), true)
	require.NoError(t, err)
	require.Equal(t, 2, om.Count())
	require.False(t, om.IsOrphan(*child2.Hash()))
	require.False(t, om.IsOrphan(*grandchild2.Hash()))
	require.True(t, om.IsOrphan(*parent2.Hash()))
}

// TestOrphanManagerRemoveOrphansByTag verifies that RemoveOrphansByTag
// correctly removes all orphans from a specific peer.
func TestOrphanManagerRemoveOrphansByTag(t *testing.T) {
	om := NewOrphanManager(DefaultOrphanConfig())

	// Add orphans from three different peers.
	tx1 := createTestOrphanTx(nil, 1)
	require.NoError(t, om.AddOrphan(tx1, Tag(1)))

	tx2 := createTestOrphanTx(nil, 1)
	require.NoError(t, om.AddOrphan(tx2, Tag(1)))

	tx3 := createTestOrphanTx(nil, 1)
	require.NoError(t, om.AddOrphan(tx3, Tag(2)))

	tx4 := createTestOrphanTx(nil, 1)
	require.NoError(t, om.AddOrphan(tx4, Tag(3)))

	require.Equal(t, 4, om.Count())

	// Remove all orphans from peer 1.
	removed := om.RemoveOrphansByTag(Tag(1))
	require.Equal(t, 2, removed)
	require.Equal(t, 2, om.Count())
	require.False(t, om.IsOrphan(*tx1.Hash()))
	require.False(t, om.IsOrphan(*tx2.Hash()))
	require.True(t, om.IsOrphan(*tx3.Hash()))
	require.True(t, om.IsOrphan(*tx4.Hash()))

	// Remove from non-existent peer.
	removed = om.RemoveOrphansByTag(Tag(999))
	require.Equal(t, 0, removed)
	require.Equal(t, 2, om.Count())

	// Remove remaining orphans.
	removed = om.RemoveOrphansByTag(Tag(2))
	require.Equal(t, 1, removed)
	removed = om.RemoveOrphansByTag(Tag(3))
	require.Equal(t, 1, removed)
	require.Equal(t, 0, om.Count())
}

// TestOrphanManagerExpireOrphans verifies that ExpireOrphans correctly
// removes orphans that have exceeded their TTL.
func TestOrphanManagerExpireOrphans(t *testing.T) {
	cfg := DefaultOrphanConfig()
	cfg.OrphanTTL = 100 * time.Millisecond
	cfg.ExpireScanInterval = 50 * time.Millisecond
	om := NewOrphanManager(cfg)

	// Add some orphans.
	tx1 := createTestOrphanTx(nil, 1)
	require.NoError(t, om.AddOrphan(tx1, Tag(1)))

	tx2 := createTestOrphanTx(nil, 1)
	require.NoError(t, om.AddOrphan(tx2, Tag(2)))

	require.Equal(t, 2, om.Count())

	// Immediate expiration should do nothing (scan interval not reached).
	removed := om.ExpireOrphans()
	require.Equal(t, 0, removed)
	require.Equal(t, 2, om.Count())

	// Wait for scan interval.
	time.Sleep(60 * time.Millisecond)

	// Now expiration should run but find nothing expired yet.
	removed = om.ExpireOrphans()
	require.Equal(t, 0, removed)
	require.Equal(t, 2, om.Count())

	// Wait for orphans to expire.
	time.Sleep(60 * time.Millisecond)

	// Next scan should remove expired orphans.
	removed = om.ExpireOrphans()
	require.Equal(t, 2, removed)
	require.Equal(t, 0, om.Count())
}

// TestOrphanManagerGetOrphan verifies that GetOrphan correctly retrieves
// orphan transactions.
func TestOrphanManagerGetOrphan(t *testing.T) {
	om := NewOrphanManager(DefaultOrphanConfig())

	tx := createTestOrphanTx(nil, 1)
	require.NoError(t, om.AddOrphan(tx, Tag(1)))

	// Get existing orphan.
	retrieved, exists := om.GetOrphan(*tx.Hash())
	require.True(t, exists)
	require.Equal(t, tx.Hash(), retrieved.Hash())

	// Get non-existent orphan.
	nonExistent := chainhash.Hash{}
	_, exists = om.GetOrphan(nonExistent)
	require.False(t, exists)
}

// TestOrphanManagerProcessOrphans verifies that ProcessOrphans correctly
// identifies and promotes orphans when a parent arrives.
func TestOrphanManagerProcessOrphans(t *testing.T) {
	om := NewOrphanManager(DefaultOrphanConfig())

	// Create a parent transaction (not in orphan pool).
	parent := createTestOrphanTx(nil, 2)

	// Add orphan children that spend from the parent.
	child1 := createTestOrphanTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, om.AddOrphan(child1, Tag(1)))

	child2 := createTestOrphanTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 1}}, 1,
	)
	require.NoError(t, om.AddOrphan(child2, Tag(2)))

	// Add a grandchild that spends from child1.
	grandchild := createTestOrphanTx(
		[]wire.OutPoint{{Hash: *child1.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, om.AddOrphan(grandchild, Tag(1)))

	// Add an unrelated orphan.
	unrelated := createTestOrphanTx(nil, 1)
	require.NoError(t, om.AddOrphan(unrelated, Tag(3)))

	require.Equal(t, 4, om.Count())

	// acceptFunc tracks which transactions were promoted.
	var promoted []chainhash.Hash
	acceptFunc := func(tx *btcutil.Tx) error {
		promoted = append(promoted, *tx.Hash())
		return nil
	}

	// Process orphans when parent arrives (parent is not in orphan pool).
	// This is the common case: parent is accepted to main mempool, and we
	// promote its orphan children.
	result, err := om.ProcessOrphans(parent, acceptFunc)
	require.NoError(t, err)

	// Should have promoted child1, child2, and grandchild (in some order).
	require.Len(t, result, 3)
	require.Len(t, promoted, 3)

	// Verify the promoted transactions.
	promotedMap := make(map[chainhash.Hash]struct{})
	for _, hash := range promoted {
		promotedMap[hash] = struct{}{}
	}
	require.Contains(t, promotedMap, *child1.Hash())
	require.Contains(t, promotedMap, *child2.Hash())
	require.Contains(t, promotedMap, *grandchild.Hash())

	// Orphan pool should now only contain unrelated.
	// (The 3 promoted orphans have been removed.)
	require.Equal(t, 1, om.Count())
	require.True(t, om.IsOrphan(*unrelated.Hash()))
	require.False(t, om.IsOrphan(*child1.Hash()))
	require.False(t, om.IsOrphan(*child2.Hash()))
	require.False(t, om.IsOrphan(*grandchild.Hash()))
}

// TestOrphanManagerProcessOrphansPartialFailure verifies that ProcessOrphans
// handles cases where some orphans cannot be promoted.
func TestOrphanManagerProcessOrphansPartialFailure(t *testing.T) {
	om := NewOrphanManager(DefaultOrphanConfig())

	// Create a parent and add it to orphan pool.
	parent := createTestOrphanTx(nil, 2)
	require.NoError(t, om.AddOrphan(parent, Tag(1)))

	// Add two children.
	child1 := createTestOrphanTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, om.AddOrphan(child1, Tag(1)))

	child2 := createTestOrphanTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 1}}, 1,
	)
	require.NoError(t, om.AddOrphan(child2, Tag(2)))

	// acceptFunc rejects child1 but accepts child2.
	acceptFunc := func(tx *btcutil.Tx) error {
		if tx.Hash().IsEqual(child1.Hash()) {
			return errors.New("validation failed")
		}
		return nil
	}

	// Process orphans.
	result, err := om.ProcessOrphans(parent, acceptFunc)
	require.NoError(t, err)

	// Should only have promoted child2.
	require.Len(t, result, 1)
	require.Equal(t, child2.Hash(), result[0].Hash())

	// child1 should remain in orphan pool.
	require.Equal(t, 2, om.Count())
	require.True(t, om.IsOrphan(*parent.Hash()))
	require.True(t, om.IsOrphan(*child1.Hash()))
	require.False(t, om.IsOrphan(*child2.Hash()))
}

// TestOrphanManagerPackageTracking verifies that the orphan manager's graph
// correctly tracks orphan package relationships for future promotion.
func TestOrphanManagerPackageTracking(t *testing.T) {
	om := NewOrphanManager(DefaultOrphanConfig())

	// Build a complex orphan package:
	//   root
	//   / \
	//  a   b
	//   \ /
	//    c

	root := createTestOrphanTx(nil, 2)
	require.NoError(t, om.AddOrphan(root, Tag(1)))

	txA := createTestOrphanTx(
		[]wire.OutPoint{{Hash: *root.Hash(), Index: 0}}, 1,
	)
	require.NoError(t, om.AddOrphan(txA, Tag(1)))

	txB := createTestOrphanTx(
		[]wire.OutPoint{{Hash: *root.Hash(), Index: 1}}, 1,
	)
	require.NoError(t, om.AddOrphan(txB, Tag(1)))

	txC := createTestOrphanTx(
		[]wire.OutPoint{
			{Hash: *txA.Hash(), Index: 0},
			{Hash: *txB.Hash(), Index: 0},
		}, 1,
	)
	require.NoError(t, om.AddOrphan(txC, Tag(1)))

	require.Equal(t, 4, om.Count())

	// Verify graph structure by checking nodes exist.
	node, exists := om.graph.GetNode(*root.Hash())
	require.True(t, exists)
	require.Len(t, node.Children, 2)

	// Verify that when we process root, all descendants are identified.
	var promoted []chainhash.Hash
	acceptFunc := func(tx *btcutil.Tx) error {
		promoted = append(promoted, *tx.Hash())
		return nil
	}

	result, err := om.ProcessOrphans(root, acceptFunc)
	require.NoError(t, err)

	// Should have promoted all descendants: A, B, C.
	require.Len(t, result, 3)
	require.Len(t, promoted, 3)

	promotedMap := make(map[chainhash.Hash]struct{})
	for _, hash := range promoted {
		promotedMap[hash] = struct{}{}
	}
	require.Contains(t, promotedMap, *txA.Hash())
	require.Contains(t, promotedMap, *txB.Hash())
	require.Contains(t, promotedMap, *txC.Hash())

	// Only root should remain in orphan pool.
	require.Equal(t, 1, om.Count())
	require.True(t, om.IsOrphan(*root.Hash()))
}
