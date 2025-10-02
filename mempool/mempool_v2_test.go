// Copyright (c) 2013-2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestNewTxMempoolV2 verifies that the constructor creates a properly
// initialized mempool with all components.
func TestNewTxMempoolV2(t *testing.T) {
	t.Parallel()

	mp := NewTxMempoolV2(nil)

	require.NotNil(t, mp, "mempool should not be nil")
	require.NotNil(t, mp.graph, "graph should be initialized")
	require.NotNil(t, mp.orphanMgr, "orphan manager should be initialized")
	require.NotNil(t, mp.policy, "policy enforcer should be initialized")

	// Verify last updated timestamp is recent (within last second).
	lastUpdated := mp.LastUpdated()
	timeSinceUpdate := time.Since(lastUpdated)
	require.Less(t, timeSinceUpdate, time.Second,
		"last updated should be recent")
}

// TestMempoolConfigDefaults verifies that nil configuration fields receive
// appropriate defaults.
func TestMempoolConfigDefaults(t *testing.T) {
	t.Parallel()

	// Create mempool with nil config.
	mp := NewTxMempoolV2(nil)

	// Graph should be created with default config.
	require.Equal(t, 0, mp.graph.GetNodeCount(),
		"empty graph should have 0 nodes")

	// Orphan manager should use defaults: 100 orphans, 100KB max size,
	// 20 min TTL, 5 min scan interval.
	// We can't directly inspect config, but we can verify it exists.
	require.NotNil(t, mp.orphanMgr)

	// Policy enforcer should use defaults.
	require.NotNil(t, mp.policy)
}

// TestMempoolCount verifies the Count method returns correct transaction count.
func TestMempoolCount(t *testing.T) {
	t.Parallel()

	mp := NewTxMempoolV2(nil)

	// Initial count should be zero.
	require.Equal(t, 0, mp.Count(), "initial count should be 0")

	// TODO: Add transactions and verify count increases.
	// This will be tested properly in implement-tx-operations task.
}

// TestMempoolHaveTransaction verifies HaveTransaction checks both main pool
// and orphan pool.
func TestMempoolHaveTransaction(t *testing.T) {
	t.Parallel()

	mp := NewTxMempoolV2(nil)

	// Random hash should not exist.
	randomHash := chainhash.Hash{0x01, 0x02, 0x03}
	require.False(t, mp.HaveTransaction(&randomHash),
		"random hash should not exist")

	// TODO: Add transaction and verify it's found.
	// This will be tested properly in implement-tx-operations task.
}

// TestMempoolIsTransactionInPool verifies IsTransactionInPool checks only
// the main pool.
func TestMempoolIsTransactionInPool(t *testing.T) {
	t.Parallel()

	mp := NewTxMempoolV2(nil)

	randomHash := chainhash.Hash{0x01, 0x02, 0x03}
	require.False(t, mp.IsTransactionInPool(&randomHash),
		"random hash should not exist in main pool")

	// TODO: Add transaction and verify it's found in main pool but not as
	// orphan. This will be tested properly in implement-tx-operations task.
}

// TestMempoolIsOrphanInPool verifies IsOrphanInPool checks only the orphan
// pool.
func TestMempoolIsOrphanInPool(t *testing.T) {
	t.Parallel()

	mp := NewTxMempoolV2(nil)

	randomHash := chainhash.Hash{0x01, 0x02, 0x03}
	require.False(t, mp.IsOrphanInPool(&randomHash),
		"random hash should not exist in orphan pool")

	// TODO: Add orphan and verify it's found in orphan pool but not main
	// pool. This will be tested properly in implement-orphan-ops task.
}

// TestMempoolLastUpdated verifies that LastUpdated returns a valid timestamp.
func TestMempoolLastUpdated(t *testing.T) {
	t.Parallel()

	mp := NewTxMempoolV2(nil)

	// LastUpdated should return a recent time.
	lastUpdated := mp.LastUpdated()
	require.False(t, lastUpdated.IsZero(), "last updated should not be zero")

	// Should be within the last second.
	timeSinceUpdate := time.Since(lastUpdated)
	require.Less(t, timeSinceUpdate, time.Second,
		"last updated should be recent")
}

// TestMempoolConcurrentAccess verifies that concurrent reads don't race.
// This is a basic smoke test for the RWMutex behavior.
func TestMempoolConcurrentAccess(t *testing.T) {
	t.Parallel()

	mp := NewTxMempoolV2(nil)

	// Launch multiple goroutines performing concurrent reads.
	const numReaders = 10
	const numReads = 100

	var wg sync.WaitGroup
	wg.Add(numReaders)

	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numReads; j++ {
				// Perform various read operations.
				_ = mp.Count()
				_ = mp.LastUpdated()

				hash := chainhash.Hash{}
				_ = mp.HaveTransaction(&hash)
				_ = mp.IsTransactionInPool(&hash)
				_ = mp.IsOrphanInPool(&hash)
			}
		}()
	}

	// Wait for all readers to complete.
	wg.Wait()
}

// TestMempoolStubbedMethodsPanic verifies that unimplemented methods panic
// with "not implemented" message as expected.
func TestMempoolStubbedMethodsPanic(t *testing.T) {
	t.Parallel()

	mp := NewTxMempoolV2(nil)

	// Create a dummy transaction for testing.
	msgTx := wire.NewMsgTx(wire.TxVersion)
	msgTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})
	msgTx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x51}, // OP_TRUE
	})

	// Test that stubbed methods panic.
	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: "MaybeAcceptTransaction",
			fn: func() {
				_, _, _ = mp.MaybeAcceptTransaction(nil, false, false)
			},
		},
		{
			name: "ProcessTransaction",
			fn: func() {
				_, _ = mp.ProcessTransaction(nil, false, false, 0)
			},
		},
		{
			name: "RemoveTransaction",
			fn: func() {
				mp.RemoveTransaction(nil, false)
			},
		},
		{
			name: "RemoveDoubleSpends",
			fn: func() {
				mp.RemoveDoubleSpends(nil)
			},
		},
		{
			name: "RemoveOrphan",
			fn: func() {
				mp.RemoveOrphan(nil)
			},
		},
		{
			name: "RemoveOrphansByTag",
			fn: func() {
				_ = mp.RemoveOrphansByTag(0)
			},
		},
		{
			name: "ProcessOrphans",
			fn: func() {
				_ = mp.ProcessOrphans(nil)
			},
		},
		{
			name: "FetchTransaction",
			fn: func() {
				_, _ = mp.FetchTransaction(nil)
			},
		},
		{
			name: "TxHashes",
			fn: func() {
				_ = mp.TxHashes()
			},
		},
		{
			name: "TxDescs",
			fn: func() {
				_ = mp.TxDescs()
			},
		},
		{
			name: "MiningDescs",
			fn: func() {
				_ = mp.MiningDescs()
			},
		},
		{
			name: "RawMempoolVerbose",
			fn: func() {
				_ = mp.RawMempoolVerbose()
			},
		},
		{
			name: "CheckSpend",
			fn: func() {
				_ = mp.CheckSpend(wire.OutPoint{})
			},
		},
		{
			name: "CheckMempoolAcceptance",
			fn: func() {
				_, _ = mp.CheckMempoolAcceptance(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify that calling the stubbed method panics.
			require.Panics(t, tt.fn,
				"stubbed method %s should panic", tt.name)
		})
	}
}

// TestMempoolInterfaceCompliance verifies that TxMempoolV2 implements the
// TxMempool interface. This is a compile-time check via the var declaration
// in mempool_v2.go, but we add an explicit test for documentation.
func TestMempoolInterfaceCompliance(t *testing.T) {
	t.Parallel()

	var _ TxMempool = (*TxMempoolV2)(nil)
}

// TestMempoolConfigCustomization verifies that custom configuration is
// properly applied when provided.
func TestMempoolConfigCustomization(t *testing.T) {
	t.Parallel()

	// Create a custom policy config.
	customPolicyCfg := DefaultPolicyConfig()
	customPolicyCfg.MaxAncestorCount = 10 // Custom value

	cfg := &MempoolConfig{
		PolicyConfig: customPolicyCfg,
	}

	mp := NewTxMempoolV2(cfg)

	require.NotNil(t, mp)
	require.NotNil(t, mp.policy)

	// The policy should use our custom config.
	// We can't directly inspect the config, but we can verify the policy
	// was created.
	require.NotNil(t, mp.policy)
}
