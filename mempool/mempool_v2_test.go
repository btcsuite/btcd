// Copyright (c) 2013-2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/stretchr/testify/require"
)

// newTestMempoolConfig creates a standard test configuration for TxMempoolV2
// with default dependencies suitable for most tests.
func newTestMempoolConfig() *MempoolConfig {
	return &MempoolConfig{
		FetchUtxoView: func(tx *btcutil.Tx) (*blockchain.UtxoViewpoint, error) {
			return blockchain.NewUtxoViewpoint(), nil
		},
		BestHeight: func() int32 {
			return 100
		},
		MedianTimePast: func() time.Time {
			return time.Now()
		},
		PolicyEnforcer: NewStandardPolicyEnforcer(DefaultPolicyConfig()),
		TxValidator:    NewStandardTxValidator(TxValidatorConfig{}),
		OrphanManager: NewOrphanManager(OrphanConfig{
			MaxOrphans:    DefaultMaxOrphanTxs,
			MaxOrphanSize: DefaultMaxOrphanTxSize,
		}),
	}
}

// newTestMempoolV2 creates a TxMempoolV2 instance with standard test
// configuration and fails the test if creation fails.
func newTestMempoolV2(t *testing.T) *TxMempoolV2 {
	t.Helper()
	mp, err := NewTxMempoolV2(newTestMempoolConfig())
	require.NoError(t, err, "failed to create test mempool")
	require.NotNil(t, mp, "mempool should not be nil")
	return mp
}

// TestNewTxMempoolV2 verifies that the constructor creates a properly
// initialized mempool with all components.
func TestNewTxMempoolV2(t *testing.T) {
	t.Parallel()

	mp, err := NewTxMempoolV2(newTestMempoolConfig())
	require.NoError(t, err, "should create mempool without error")
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

// TestMempoolConfigNilError verifies that nil config returns an error.
func TestMempoolConfigNilError(t *testing.T) {
	t.Parallel()

	// Create mempool with nil config.
	mp, err := NewTxMempoolV2(nil)
	require.Error(t, err, "should error on nil config")
	require.Nil(t, mp, "mempool should be nil on error")
	require.Contains(t, err.Error(), "config cannot be nil")
}

// TestMempoolMissingDependenciesError verifies that missing dependencies
// return errors.
func TestMempoolMissingDependenciesError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupConfig func() *MempoolConfig
		errContains string
	}{
		{
			name: "missing PolicyEnforcer",
			setupConfig: func() *MempoolConfig {
				cfg := newTestMempoolConfig()
				cfg.PolicyEnforcer = nil
				return cfg
			},
			errContains: "PolicyEnforcer is required",
		},
		{
			name: "missing TxValidator",
			setupConfig: func() *MempoolConfig {
				cfg := newTestMempoolConfig()
				cfg.TxValidator = nil
				return cfg
			},
			errContains: "TxValidator is required",
		},
		{
			name: "missing OrphanManager",
			setupConfig: func() *MempoolConfig {
				cfg := newTestMempoolConfig()
				cfg.OrphanManager = nil
				return cfg
			},
			errContains: "OrphanManager is required",
		},
		{
			name: "missing FetchUtxoView",
			setupConfig: func() *MempoolConfig {
				cfg := newTestMempoolConfig()
				cfg.FetchUtxoView = nil
				return cfg
			},
			errContains: "FetchUtxoView is required",
		},
		{
			name: "missing BestHeight",
			setupConfig: func() *MempoolConfig {
				cfg := newTestMempoolConfig()
				cfg.BestHeight = nil
				return cfg
			},
			errContains: "BestHeight is required",
		},
		{
			name: "missing MedianTimePast",
			setupConfig: func() *MempoolConfig {
				cfg := newTestMempoolConfig()
				cfg.MedianTimePast = nil
				return cfg
			},
			errContains: "MedianTimePast is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()
			mp, err := NewTxMempoolV2(cfg)
			require.Error(t, err, "should error on missing dependency")
			require.Nil(t, mp, "mempool should be nil on error")
			require.Contains(t, err.Error(), tt.errContains)
		})
	}
}

// TestMempoolCount verifies the Count method returns correct transaction count.
func TestMempoolCount(t *testing.T) {
	t.Parallel()

	h := createTestMempoolV2()

	// Initial count should be zero.
	require.Equal(t, 0, h.mempool.Count(), "initial count should be 0")

	// Add first transaction (parent1).
	parent1 := createTestTx()
	parent1View := createDefaultUtxoView(parent1)
	h.expectTxAccepted(parent1, parent1View)
	_, _, err := h.mempool.MaybeAcceptTransaction(parent1, true, false)
	require.NoError(t, err)
	require.Equal(t, 1, h.mempool.Count(), "count should be 1 after adding first tx")

	// Add second transaction (child of parent1) to ensure unique hash.
	child1 := createChildTx(parent1, 0)
	child1View := createDefaultUtxoView(child1)
	h.expectTxAccepted(child1, child1View)
	_, _, err = h.mempool.MaybeAcceptTransaction(child1, true, false)
	require.NoError(t, err)
	require.Equal(t, 2, h.mempool.Count(), "count should be 2 after adding second tx")

	// Add third transaction (child of child1) to avoid conflicts.
	child2 := createChildTx(child1, 0)
	child2View := createDefaultUtxoView(child2)
	h.expectTxAccepted(child2, child2View)
	_, _, err = h.mempool.MaybeAcceptTransaction(child2, true, false)
	require.NoError(t, err)
	require.Equal(t, 3, h.mempool.Count(), "count should be 3 after adding third tx")
}

// TestMempoolHaveTransaction verifies HaveTransaction checks both main pool
// and orphan pool.
func TestMempoolHaveTransaction(t *testing.T) {
	t.Parallel()

	h := createTestMempoolV2()

	// Random hash should not exist.
	randomHash := chainhash.Hash{0x01, 0x02, 0x03}
	require.False(t, h.mempool.HaveTransaction(&randomHash),
		"random hash should not exist")

	// Add transaction to main pool.
	tx := createTestTx()
	view := createDefaultUtxoView(tx)
	h.expectTxAccepted(tx, view)
	_, _, err := h.mempool.MaybeAcceptTransaction(tx, true, false)
	require.NoError(t, err)

	// Verify transaction is found.
	require.True(t, h.mempool.HaveTransaction(tx.Hash()),
		"should find transaction in mempool")

	// Add an orphan transaction (create with unique signature).
	orphan := createTestTx()
	// Make unique by modifying signature.
	orphan.MsgTx().TxIn[0].SignatureScript[0] = 0xFF
	orphanView := createOrphanUtxoView(orphan)
	h.expectTxOrphan(orphan, orphanView)

	// Use ProcessTransaction to actually add orphan to orphan pool.
	acceptedTxs, err := h.mempool.ProcessTransaction(orphan, true, false, 0)
	require.NoError(t, err)
	// Orphan returns nil (not accepted to main pool).
	require.Nil(t, acceptedTxs)

	// Verify orphan is also found by HaveTransaction (checks both pools).
	require.True(t, h.mempool.HaveTransaction(orphan.Hash()),
		"should find orphan transaction in orphan pool")
}

// TestMempoolIsTransactionInPool verifies IsTransactionInPool checks only
// the main pool.
func TestMempoolIsTransactionInPool(t *testing.T) {
	t.Parallel()

	h := createTestMempoolV2()

	randomHash := chainhash.Hash{0x01, 0x02, 0x03}
	require.False(t, h.mempool.IsTransactionInPool(&randomHash),
		"random hash should not exist in main pool")

	// Add transaction to main pool.
	tx := createTestTx()
	view := createDefaultUtxoView(tx)
	h.expectTxAccepted(tx, view)
	_, _, err := h.mempool.MaybeAcceptTransaction(tx, true, false)
	require.NoError(t, err)

	// Verify transaction is found in main pool.
	require.True(t, h.mempool.IsTransactionInPool(tx.Hash()),
		"should find transaction in main pool")

	// Add an orphan transaction (create with unique signature).
	orphan := createTestTx()
	// Make unique by modifying signature.
	orphan.MsgTx().TxIn[0].SignatureScript[0] = 0xFF
	orphanView := createOrphanUtxoView(orphan)
	h.expectTxOrphan(orphan, orphanView)

	// Use ProcessTransaction to actually add orphan to orphan pool.
	acceptedTxs, err := h.mempool.ProcessTransaction(orphan, true, false, 0)
	require.NoError(t, err)
	// Orphan returns nil (not accepted to main pool).
	require.Nil(t, acceptedTxs)

	// Verify orphan is NOT found by IsTransactionInPool (checks only main pool).
	require.False(t, h.mempool.IsTransactionInPool(orphan.Hash()),
		"should not find orphan in main pool")
	// But verify it's in the orphan pool.
	require.True(t, h.mempool.IsOrphanInPool(orphan.Hash()),
		"should find orphan in orphan pool")
}

// TestMempoolIsOrphanInPool verifies IsOrphanInPool checks only the orphan
// pool.
func TestMempoolIsOrphanInPool(t *testing.T) {
	t.Parallel()

	h := createTestMempoolV2()

	randomHash := chainhash.Hash{0x01, 0x02, 0x03}
	require.False(t, h.mempool.IsOrphanInPool(&randomHash),
		"random hash should not exist in orphan pool")

	// Add an orphan transaction (create with unique signature).
	orphan := createTestTx()
	// Make unique by modifying signature.
	orphan.MsgTx().TxIn[0].SignatureScript[0] = 0xFF
	orphanView := createOrphanUtxoView(orphan)
	h.expectTxOrphan(orphan, orphanView)

	// Use ProcessTransaction to actually add orphan to orphan pool.
	acceptedTxs, err := h.mempool.ProcessTransaction(orphan, true, false, 0)
	require.NoError(t, err)
	// Orphan returns nil (not accepted to main pool).
	require.Nil(t, acceptedTxs)

	// Verify orphan is found in orphan pool.
	require.True(t, h.mempool.IsOrphanInPool(orphan.Hash()),
		"should find orphan in orphan pool")
	// But NOT found in main pool.
	require.False(t, h.mempool.IsTransactionInPool(orphan.Hash()),
		"should not find orphan in main pool")

	// Add a regular transaction to main pool (create as child to make unique).
	parent := createTestTx()
	parentView := createDefaultUtxoView(parent)
	h.expectTxAccepted(parent, parentView)
	_, _, err = h.mempool.MaybeAcceptTransaction(parent, true, false)
	require.NoError(t, err)

	// Verify main pool transaction is NOT in orphan pool.
	require.False(t, h.mempool.IsOrphanInPool(parent.Hash()),
		"should not find main pool tx in orphan pool")
	// But IS found in main pool.
	require.True(t, h.mempool.IsTransactionInPool(parent.Hash()),
		"should find main pool tx in main pool")
}

// TestMempoolLastUpdated verifies that LastUpdated returns a valid timestamp.
func TestMempoolLastUpdated(t *testing.T) {
	t.Parallel()

	mp := newTestMempoolV2(t)

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

	mp := newTestMempoolV2(t)

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

// TestMempoolConfigCustomization verifies that custom dependencies can be
// injected via configuration.
func TestMempoolConfigCustomization(t *testing.T) {
	t.Parallel()

	// Create a custom policy enforcer.
	customPolicyCfg := DefaultPolicyConfig()
	customPolicyCfg.MaxAncestorCount = 10 // Custom value
	customPolicy := NewStandardPolicyEnforcer(customPolicyCfg)

	// Create a custom orphan manager.
	customOrphanMgr := NewOrphanManager(OrphanConfig{
		MaxOrphans:    50, // Custom value
		MaxOrphanSize: 50000,
	})

	cfg := &MempoolConfig{
		FetchUtxoView: func(tx *btcutil.Tx) (*blockchain.UtxoViewpoint, error) {
			return blockchain.NewUtxoViewpoint(), nil
		},
		BestHeight: func() int32 {
			return 100
		},
		MedianTimePast: func() time.Time {
			return time.Now()
		},
		PolicyEnforcer: customPolicy,
		TxValidator:    NewStandardTxValidator(TxValidatorConfig{}),
		OrphanManager:  customOrphanMgr,
	}

	mp, err := NewTxMempoolV2(cfg)
	require.NoError(t, err)

	require.NotNil(t, mp)
	require.NotNil(t, mp.policy)
	require.NotNil(t, mp.orphanMgr)
	require.NotNil(t, mp.txValidator)

	// Verify that the injected dependencies are used.
	require.Same(t, customPolicy, mp.policy,
		"should use injected policy enforcer")
	require.Same(t, customOrphanMgr, mp.orphanMgr,
		"should use injected orphan manager")
}
