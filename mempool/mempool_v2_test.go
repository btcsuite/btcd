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
	"github.com/btcsuite/btcd/wire"
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

	mp := newTestMempoolV2(t)

	// Initial count should be zero.
	require.Equal(t, 0, mp.Count(), "initial count should be 0")

	// TODO: Add transactions and verify count increases.
	// This will be tested properly in implement-tx-operations task.
}

// TestMempoolHaveTransaction verifies HaveTransaction checks both main pool
// and orphan pool.
func TestMempoolHaveTransaction(t *testing.T) {
	t.Parallel()

	mp := newTestMempoolV2(t)

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

	mp := newTestMempoolV2(t)

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

	mp := newTestMempoolV2(t)

	randomHash := chainhash.Hash{0x01, 0x02, 0x03}
	require.False(t, mp.IsOrphanInPool(&randomHash),
		"random hash should not exist in orphan pool")

	// TODO: Add orphan and verify it's found in orphan pool but not main
	// pool. This will be tested properly in implement-orphan-ops task.
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

// TestMempoolStubbedMethodsPanic verifies that unimplemented methods panic
// with "not implemented" message as expected.
func TestMempoolStubbedMethodsPanic(t *testing.T) {
	t.Parallel()

	mp := newTestMempoolV2(t)

	// Test that stubbed methods panic (only for methods not yet implemented).
	tests := []struct {
		name string
		fn   func()
	}{
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
