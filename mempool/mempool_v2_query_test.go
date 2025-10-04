// Copyright (c) 2013-2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestFetchTransaction verifies that FetchTransaction correctly retrieves
// transactions from the mempool.
func TestFetchTransaction(t *testing.T) {
	t.Parallel()

	h := createTestMempoolV2()

	// Test fetching non-existent transaction.
	randomHash := chainhash.Hash{0x01}
	tx, err := h.mempool.FetchTransaction(&randomHash)
	require.Error(t, err, "should error for non-existent transaction")
	require.Nil(t, tx, "should return nil for non-existent transaction")
	require.Contains(t, err.Error(), "not in the pool")

	// Add a transaction to the mempool.
	testTx := createTestTx()
	view := createDefaultUtxoView(testTx)
	h.expectTxAccepted(testTx, view)

	_, _, err = h.mempool.MaybeAcceptTransaction(testTx, true, false)
	require.NoError(t, err)

	// Fetch the transaction and verify it matches.
	fetchedTx, err := h.mempool.FetchTransaction(testTx.Hash())
	require.NoError(t, err)
	require.NotNil(t, fetchedTx)
	require.Equal(t, testTx.Hash(), fetchedTx.Hash())
}

// TestTxHashes verifies that TxHashes returns correct hash list.
func TestTxHashes(t *testing.T) {
	t.Parallel()

	h := createTestMempoolV2()

	// Empty mempool should return empty slice.
	hashes := h.mempool.TxHashes()
	require.NotNil(t, hashes, "should return non-nil slice")
	require.Empty(t, hashes, "empty mempool should have no hashes")

	// Add several unique transactions.
	// Create a parent and two children to ensure unique hashes.
	parent := createTestTx()
	child1 := createChildTx(parent, 0)
	child2 := createChildTx(parent, 1)

	// Add parent first.
	parentView := createDefaultUtxoView(parent)
	h.expectTxAccepted(parent, parentView)
	_, _, err := h.mempool.MaybeAcceptTransaction(parent, true, false)
	require.NoError(t, err)

	// Add child1.
	child1View := createDefaultUtxoView(child1)
	h.expectTxAccepted(child1, child1View)
	_, _, err = h.mempool.MaybeAcceptTransaction(child1, true, false)
	require.NoError(t, err)

	// Add child2.
	child2View := createDefaultUtxoView(child2)
	h.expectTxAccepted(child2, child2View)
	_, _, err = h.mempool.MaybeAcceptTransaction(child2, true, false)
	require.NoError(t, err)

	// Get hashes and verify.
	hashes = h.mempool.TxHashes()
	require.Len(t, hashes, 3)

	// Verify all three transaction hashes are present.
	hashMap := make(map[chainhash.Hash]bool)
	for _, hash := range hashes {
		hashMap[*hash] = true
	}
	require.True(t, hashMap[*parent.Hash()])
	require.True(t, hashMap[*child1.Hash()])
	require.True(t, hashMap[*child2.Hash()])
}

// TestTxDescs verifies that TxDescs returns correct descriptors.
func TestTxDescs(t *testing.T) {
	t.Parallel()

	h := createTestMempoolV2()

	// Empty mempool should return empty slice.
	descs := h.mempool.TxDescs()
	require.NotNil(t, descs, "should return non-nil slice")
	require.Empty(t, descs, "empty mempool should have no descriptors")

	// Add a transaction.
	tx := createTestTx()
	view := createDefaultUtxoView(tx)
	h.expectTxAccepted(tx, view)

	_, txDesc, err := h.mempool.MaybeAcceptTransaction(tx, true, false)
	require.NoError(t, err)
	require.NotNil(t, txDesc)

	// Get descriptors and verify.
	descs = h.mempool.TxDescs()
	require.Len(t, descs, 1)

	// Verify descriptor fields.
	desc := descs[0]
	require.Equal(t, tx.Hash(), desc.Tx.Hash())
	require.Equal(t, txDesc.Height, desc.Height)
	require.Equal(t, txDesc.Fee, desc.Fee)
	require.Equal(t, txDesc.FeePerKB, desc.FeePerKB)
}

// TestMiningDescs verifies that MiningDescs returns correct mining descriptors.
func TestMiningDescs(t *testing.T) {
	t.Parallel()

	h := createTestMempoolV2()

	// Empty mempool should return empty slice.
	descs := h.mempool.MiningDescs()
	require.NotNil(t, descs, "should return non-nil slice")
	require.Empty(t, descs, "empty mempool should have no mining descriptors")

	// Add parent and child transactions to test topological ordering.
	parent := createTestTx()
	child := createChildTx(parent, 0)

	// Add parent first.
	parentView := createDefaultUtxoView(parent)
	h.expectTxAccepted(parent, parentView)
	_, _, err := h.mempool.MaybeAcceptTransaction(parent, true, false)
	require.NoError(t, err)

	// Add child.
	childView := createDefaultUtxoView(child)
	h.expectTxAccepted(child, childView)
	_, _, err = h.mempool.MaybeAcceptTransaction(child, true, false)
	require.NoError(t, err)

	// Get mining descriptors.
	descs = h.mempool.MiningDescs()
	require.Len(t, descs, 2)

	// Verify both parent and child are present.
	hashes := make(map[chainhash.Hash]bool)
	for _, desc := range descs {
		hashes[*desc.Tx.Hash()] = true
	}
	require.True(t, hashes[*parent.Hash()], "parent should be in results")
	require.True(t, hashes[*child.Hash()], "child should be in results")

	// TODO: Add more sophisticated topological ordering verification
	// once we have a more complex transaction graph test case.
}

// TestRawMempoolVerbose verifies that RawMempoolVerbose returns correct verbose info.
func TestRawMempoolVerbose(t *testing.T) {
	t.Parallel()

	h := createTestMempoolV2()

	// Empty mempool should return empty map.
	verbose := h.mempool.RawMempoolVerbose()
	require.NotNil(t, verbose, "should return non-nil map")
	require.Empty(t, verbose, "empty mempool should have empty verbose map")

	// Add parent and child to test dependency tracking.
	parent := createTestTx()
	child := createChildTx(parent, 0)

	parentView := createDefaultUtxoView(parent)
	h.expectTxAccepted(parent, parentView)
	_, _, err := h.mempool.MaybeAcceptTransaction(parent, true, false)
	require.NoError(t, err)

	childView := createDefaultUtxoView(child)
	h.expectTxAccepted(child, childView)
	_, _, err = h.mempool.MaybeAcceptTransaction(child, true, false)
	require.NoError(t, err)

	// Get verbose info.
	verbose = h.mempool.RawMempoolVerbose()
	require.Len(t, verbose, 2)

	// Verify child has parent in depends list.
	childVerbose, ok := verbose[child.Hash().String()]
	require.True(t, ok)
	require.Contains(t, childVerbose.Depends, parent.Hash().String())

	// Verify parent has no dependencies.
	parentVerbose, ok := verbose[parent.Hash().String()]
	require.True(t, ok)
	require.Empty(t, parentVerbose.Depends)
}

// TestMempoolV2CheckSpend verifies that CheckSpend correctly detects spending transactions.
func TestMempoolV2CheckSpend(t *testing.T) {
	t.Parallel()

	h := createTestMempoolV2()

	// Test checking spend for non-existent outpoint.
	op := wire.OutPoint{
		Hash:  chainhash.Hash{0x01},
		Index: 0,
	}
	spender := h.mempool.CheckSpend(op)
	require.Nil(t, spender, "should return nil for unspent outpoint")

	// Add parent and child transactions.
	parent := createTestTx()
	child := createChildTx(parent, 0)

	parentView := createDefaultUtxoView(parent)
	h.expectTxAccepted(parent, parentView)
	_, _, err := h.mempool.MaybeAcceptTransaction(parent, true, false)
	require.NoError(t, err)

	childView := createDefaultUtxoView(child)
	h.expectTxAccepted(child, childView)
	_, _, err = h.mempool.MaybeAcceptTransaction(child, true, false)
	require.NoError(t, err)

	// Check if child spends parent's output.
	parentOutpoint := wire.OutPoint{
		Hash:  *parent.Hash(),
		Index: 0,
	}
	spender = h.mempool.CheckSpend(parentOutpoint)
	require.NotNil(t, spender)
	require.Equal(t, child.Hash(), spender.Hash())

	// Check an unspent output from parent.
	unspentOutpoint := wire.OutPoint{
		Hash:  *parent.Hash(),
		Index: 1,
	}
	spender = h.mempool.CheckSpend(unspentOutpoint)
	require.Nil(t, spender)
}

// TestQueryMethodsConcurrentAccess verifies that query methods are safe for
// concurrent access.
func TestQueryMethodsConcurrentAccess(t *testing.T) {
	t.Parallel()

	mp := newTestMempoolV2(t)

	// Launch multiple goroutines performing concurrent query operations.
	const numReaders = 10
	const numReads = 100

	done := make(chan bool)

	for i := 0; i < numReaders; i++ {
		go func() {
			for j := 0; j < numReads; j++ {
				// Perform various query operations concurrently.
				_ = mp.TxHashes()
				_ = mp.TxDescs()
				_ = mp.MiningDescs()
				_ = mp.RawMempoolVerbose()

				randomHash := chainhash.Hash{byte(j)}
				_, _ = mp.FetchTransaction(&randomHash)
				_ = mp.CheckSpend(wire.OutPoint{Hash: randomHash})
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete.
	for i := 0; i < numReaders; i++ {
		<-done
	}

	// No panics or races should occur (verified by race detector).
}
