// Copyright (c) 2013-2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mempool/txgraph"
	"github.com/btcsuite/btcd/mining"
	"github.com/btcsuite/btcd/wire"
)

const (
	// DefaultMaxOrphanTxs is the default maximum number of orphan
	// transactions that can be queued. This matches Bitcoin Core's limit.
	DefaultMaxOrphanTxs = 100

	// DefaultMaxOrphanTxSize is the default maximum size allowed for orphan
	// transactions in bytes. This prevents memory exhaustion from large
	// orphans.
	DefaultMaxOrphanTxSize = 100000 // 100 KB

	// DefaultOrphanTTL is the default time an orphan transaction is allowed
	// to stay in the orphan pool before expiration. This matches Bitcoin
	// Core's 20 minute limit.
	DefaultOrphanTTL = 20 * time.Minute

	// DefaultOrphanExpireScanInterval is the default minimum time between
	// scans of the orphan pool to evict expired transactions.
	DefaultOrphanExpireScanInterval = 5 * time.Minute
)

// OrphanTxManager defines the interface for managing orphan transactions.
// This interface contains only the methods used by TxMempoolV2, enabling
// easy testing and decoupling from the concrete OrphanManager implementation.
type OrphanTxManager interface {
	// IsOrphan returns whether the transaction exists in the orphan pool.
	IsOrphan(hash chainhash.Hash) bool

	// AddOrphan adds an orphan transaction to the pool with the given tag.
	AddOrphan(tx *btcutil.Tx, tag Tag) error

	// RemoveOrphan removes an orphan transaction from the pool.
	RemoveOrphan(hash chainhash.Hash, cascade bool) error

	// RemoveOrphansByTag removes all orphans tagged with the given tag.
	RemoveOrphansByTag(tag Tag) int

	// ProcessOrphans processes orphans that may now be acceptable after a
	// parent transaction was added. Returns the list of promoted orphans.
	ProcessOrphans(
		parentTx *btcutil.Tx,
		acceptFunc func(*btcutil.Tx) error,
	) ([]*btcutil.Tx, error)
}

// TxFeeEstimator defines the interface for observing transaction fees.
// This interface contains only the methods used by TxMempoolV2.
type TxFeeEstimator interface {
	// ObserveTransaction is called when a transaction is added to the
	// mempool to track its fee for estimation purposes.
	ObserveTransaction(txDesc *TxDesc)
}

// TxAddrIndexer defines the interface for indexing transactions by address.
// This interface contains only the methods used by TxMempoolV2.
type TxAddrIndexer interface {
	// AddUnconfirmedTx adds the transaction to the address index.
	AddUnconfirmedTx(tx *btcutil.Tx, utxoView *blockchain.UtxoViewpoint)

	// RemoveUnconfirmedTx removes the transaction from the address index.
	RemoveUnconfirmedTx(txHash *chainhash.Hash)
}

// TxMempoolV2 implements a graph-based transaction mempool that separates
// data structure operations (graph) from policy enforcement. This design
// enables easier testing, better maintainability, and supports future protocol
// upgrades like TRUC transactions and ephemeral anchors.
//
// Unlike the flat-map TxPool, TxMempoolV2 explicitly tracks transaction
// relationships via a directed acyclic graph (DAG), enabling efficient
// ancestor/descendant queries, cluster analysis for RBF, and package
// identification for relay and validation.
//
// Architecture:
//
//	┌─────────────────────────────────────────────┐
//	│           TxMempoolV2                       │
//	├─────────────────────────────────────────────┤
//	│                                             │
//	│  ┌──────────────┐  ┌──────────────────┐    │
//	│  │   TxGraph    │  │ OrphanManager    │    │
//	│  │              │  │                  │    │
//	│  │ - Nodes      │  │ - Orphan Graph   │    │
//	│  │ - Edges      │  │ - TTL Expiry     │    │
//	│  │ - Clusters   │  │ - Peer Tagging   │    │
//	│  └──────────────┘  └──────────────────┘    │
//	│                                             │
//	│  ┌─────────────────────────────────────┐   │
//	│  │     PolicyEnforcer                  │   │
//	│  │                                     │   │
//	│  │ - RBF Validation (BIP 125)          │   │
//	│  │ - Ancestor/Descendant Limits        │   │
//	│  │ - Fee Rate Validation               │   │
//	│  └─────────────────────────────────────┘   │
//	│                                             │
//	└─────────────────────────────────────────────┘
//
// The separation of concerns enables:
//   - Graph: Pure data structure, handles relationships
//   - OrphanManager: Lifecycle management for orphans
//   - PolicyEnforcer: Bitcoin Core-compatible policy decisions
//
// This structure runs in parallel with the old TxPool during development
// and testing. Once validated, it will replace TxPool entirely.
type TxMempoolV2 struct {
	// graph stores all confirmed mempool transactions and their
	// relationships. The graph provides O(1) lookups and efficient
	// traversal for ancestor/descendant queries needed for RBF and
	// package validation.
	graph *txgraph.TxGraph

	// orphanMgr manages orphan transactions in a separate graph. Orphans
	// have different lifecycle (TTL-based expiration, peer tagging) and
	// shouldn't pollute the main graph used for mining and fee estimation.
	orphanMgr OrphanTxManager

	// policy enforces mempool policies including RBF rules (BIP 125),
	// ancestor/descendant limits, and fee requirements. Separated from
	// graph to enable different policy configurations and easier testing.
	policy PolicyEnforcer

	// txValidator validates transaction scripts and sequence locks. This
	// handles the expensive cryptographic validation and consensus rules
	// for timelocks.
	txValidator TxValidator

	// cfg contains mempool configuration including blockchain interface
	// functions (UTXO fetching, best height) and policy settings.
	cfg MempoolConfig

	// lastUpdated tracks the last time a transaction was added or removed
	// from the mempool. Uses atomic operations for lock-free reads by RPC
	// handlers and mining code.
	lastUpdated atomic.Int64

	// mu protects concurrent access to mempool state. RWMutex allows
	// concurrent reads (queries for mining, RPC) while serializing writes
	// (transaction add/remove operations).
	mu sync.RWMutex
}

// MempoolConfig defines configuration for the graph-based mempool.
// This structure contains only the fields directly used by TxMempoolV2.
// Dependencies (PolicyEnforcer, TxValidator, OrphanManager) must be
// constructed and provided by the caller.
type MempoolConfig struct {
	// FetchUtxoView provides UTXO data for transaction validation. This is
	// called during validation to check that inputs exist and aren't
	// double-spent. Required.
	FetchUtxoView func(*btcutil.Tx) (*blockchain.UtxoViewpoint, error)

	// BestHeight returns the current blockchain tip height. Used for
	// transaction validation and policy decisions. Required.
	BestHeight func() int32

	// MedianTimePast returns median time past for the current chain tip.
	// Used for timelock validation (BIP 113). Required.
	MedianTimePast func() time.Time

	// AddrIndex optionally indexes mempool transactions by address. Can be
	// nil if address indexing is disabled.
	AddrIndex TxAddrIndexer

	// FeeEstimator optionally tracks transaction fee rates for fee
	// estimation. Can be nil if fee estimation is disabled.
	FeeEstimator TxFeeEstimator

	// PolicyEnforcer validates mempool policies including RBF rules,
	// ancestor/descendant limits, and fee requirements. Required.
	PolicyEnforcer PolicyEnforcer

	// TxValidator validates transaction scripts and sequence locks.
	// Required.
	TxValidator TxValidator

	// OrphanManager manages orphan transactions. Required.
	OrphanManager OrphanTxManager

	// GraphConfig configures the underlying transaction graph. If nil,
	// defaults will be applied (100k node capacity, 101 tx max package size).
	GraphConfig *txgraph.Config
}

// NewTxMempoolV2 creates a new graph-based mempool with the provided
// configuration. All required dependencies (PolicyEnforcer, TxValidator,
// OrphanManager) must be provided via the config.
//
// This design enforces explicit dependency injection, making dependencies
// clear and testable. Callers must construct dependencies before creating
// the mempool.
//
// Returns an error if any required dependencies are missing.
func NewTxMempoolV2(cfg *MempoolConfig) (*TxMempoolV2, error) {
	if cfg == nil {
		return nil, fmt.Errorf("mempool config cannot be nil")
	}

	// Validate required dependencies are provided.
	if cfg.PolicyEnforcer == nil {
		return nil, fmt.Errorf("PolicyEnforcer is required")
	}
	if cfg.TxValidator == nil {
		return nil, fmt.Errorf("TxValidator is required")
	}
	if cfg.OrphanManager == nil {
		return nil, fmt.Errorf("OrphanManager is required")
	}
	if cfg.FetchUtxoView == nil {
		return nil, fmt.Errorf("FetchUtxoView is required")
	}
	if cfg.BestHeight == nil {
		return nil, fmt.Errorf("BestHeight is required")
	}
	if cfg.MedianTimePast == nil {
		return nil, fmt.Errorf("MedianTimePast is required")
	}

	// Initialize graph with provided config or sensible defaults.
	// Default: 100k nodes, 101 tx max package (25 ancestors + 25
	// descendants + 1 root).
	graphCfg := cfg.GraphConfig
	if graphCfg == nil {
		graphCfg = txgraph.DefaultConfig()
	}
	graph := txgraph.New(graphCfg)

	mp := &TxMempoolV2{
		graph:       graph,
		orphanMgr:   cfg.OrphanManager,
		policy:      cfg.PolicyEnforcer,
		txValidator: cfg.TxValidator,
		cfg:         *cfg,
	}

	// Initialize last updated timestamp to current time.
	mp.lastUpdated.Store(time.Now().Unix())

	log.InfoS(context.Background(), "Initialized TxMempoolV2",
		"graph_capacity", graphCfg.MaxNodes,
		"max_package_size", graphCfg.MaxPackageSize)

	return mp, nil
}

// LastUpdated returns the last time a transaction was added to or removed
// from the mempool. This is used by RPC handlers and mining code to detect
// mempool changes.
//
// This method uses atomic loads and requires no mutex, enabling lock-free
// concurrent access by multiple readers.
func (mp *TxMempoolV2) LastUpdated() time.Time {
	return time.Unix(mp.lastUpdated.Load(), 0)
}

// Count returns the number of transactions in the main mempool pool.
// This count excludes orphan transactions.
//
// This method takes a read lock to ensure consistent reads during concurrent
// modifications.
func (mp *TxMempoolV2) Count() int {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	return mp.graph.GetNodeCount()
}

// HaveTransaction returns whether the passed transaction already exists in
// the main pool or in the orphan pool.
//
// This method takes a read lock to ensure consistent reads across both the
// main graph and orphan manager.
func (mp *TxMempoolV2) HaveTransaction(hash *chainhash.Hash) bool {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	return mp.graph.HasTransaction(*hash) || mp.orphanMgr.IsOrphan(*hash)
}

// IsTransactionInPool returns whether the passed transaction exists in the
// main pool. This differs from HaveTransaction in that it does NOT check
// the orphan pool.
//
// This method takes a read lock to ensure consistent reads.
func (mp *TxMempoolV2) IsTransactionInPool(hash *chainhash.Hash) bool {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	return mp.graph.HasTransaction(*hash)
}

// IsOrphanInPool returns whether the passed transaction exists in the
// orphan pool. This differs from HaveTransaction in that it does NOT check
// the main pool.
//
// This method takes a read lock to ensure consistent reads.
func (mp *TxMempoolV2) IsOrphanInPool(hash *chainhash.Hash) bool {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	return mp.orphanMgr.IsOrphan(*hash)
}

// The following methods are stubs that will be implemented in subsequent
// tasks. Each panics with "not implemented" to clearly indicate the
// method is not yet functional.

// MaybeAcceptTransaction is the internal function which implements the core of
// ProcessTransaction. It is separated to allow testing of the core logic
// without the additional overhead of orphan processing.
//
// STUB: Implementation pending in implement-tx-operations task.
// MaybeAcceptTransaction validates and potentially adds a transaction to the
// mempool. Returns missing parent hashes if transaction is an orphan,
// otherwise returns the accepted TxDesc.
//
// This method acquires the write lock and calls maybeAcceptTransactionLocked.
func (mp *TxMempoolV2) MaybeAcceptTransaction(tx *btcutil.Tx, isNew, rateLimit bool) ([]*chainhash.Hash, *TxDesc, error) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	return mp.maybeAcceptTransactionLocked(tx, isNew, rateLimit, true)
}

// maybeAcceptTransactionLocked is the internal implementation of
// MaybeAcceptTransaction that must be called with the mempool lock held.
//
// The rejectDupOrphans parameter controls whether to reject transactions that
// already exist in the orphan pool.
func (mp *TxMempoolV2) maybeAcceptTransactionLocked(
	tx *btcutil.Tx,
	isNew, rateLimit, rejectDupOrphans bool,
) ([]*chainhash.Hash, *TxDesc, error) {

	txHash := tx.Hash()
	ctx := context.Background()
	log.TraceS(ctx, "Processing transaction acceptance",
		"tx_hash", txHash,
		"is_new", isNew,
		"rate_limit", rateLimit)

	// Validate transaction using the v2 validation pipeline.
	result, err := mp.checkMempoolAcceptance(tx, isNew, rateLimit, rejectDupOrphans)
	if err != nil {
		log.DebugS(ctx, "Transaction rejected",
			"tx_hash", txHash,
			"reason", err.Error())
		return nil, nil, err
	}

	// If orphan (has missing parents), return parent hashes.
	if len(result.MissingParents) > 0 {
		log.DebugS(ctx, "Transaction is an orphan",
			"tx_hash", txHash,
			"missing_parents", len(result.MissingParents))
		return result.MissingParents, nil, nil
	}

	// Handle RBF replacements if conflicts exist.
	if len(result.Conflicts) > 0 {
		conflictCount := len(result.Conflicts)
		feeRate := int64(result.TxFee) * 1000 / result.TxSize
		log.DebugS(ctx, "RBF replacement",
			"tx_hash", txHash,
			"conflicts_count", conflictCount,
			"fee_rate_sat_vbyte", feeRate)

		// Warn on large replacements (potential DoS indicator).
		if conflictCount > 10 {
			log.InfoS(ctx, "Large RBF replacement detected",
				"tx_hash", txHash,
				"conflicts_count", conflictCount)
		}

		// Remove conflicts from graph. The graph will cascade to
		// descendants automatically.
		for conflictHash := range result.Conflicts {
			if err := mp.graph.RemoveTransaction(conflictHash); err != nil {
				log.WarnS(ctx, "Failed to remove RBF conflict", err,
					"conflict_hash", conflictHash)
			}
		}
	}

	// Create TxDesc for the graph node.
	graphDesc := &txgraph.TxDesc{
		TxHash:      *txHash,
		VirtualSize: result.TxSize,
		Fee:         int64(result.TxFee),
		FeePerKB:    int64(result.TxFee) * 1000 / result.TxSize,
		Added:       time.Now(),
	}

	// Add transaction to graph.
	if err := mp.graph.AddTransaction(tx, graphDesc); err != nil {
		return nil, nil, err
	}

	// Update address index if enabled.
	if mp.cfg.AddrIndex != nil {
		mp.cfg.AddrIndex.AddUnconfirmedTx(tx, result.utxoView)
	}

	// Record for fee estimation if enabled.
	if mp.cfg.FeeEstimator != nil {
		mp.cfg.FeeEstimator.ObserveTransaction(&TxDesc{
			TxDesc: mining.TxDesc{
				Tx:       tx,
				Added:    time.Now(),
				Height:   result.bestHeight,
				Fee:      int64(result.TxFee),
				FeePerKB: int64(result.TxFee) * 1000 / result.TxSize,
			},
		})
	}

	// Update last modified timestamp.
	mp.lastUpdated.Store(time.Now().Unix())

	feeRate := int64(result.TxFee) * 1000 / result.TxSize
	log.DebugS(ctx, "Transaction accepted",
		"tx_hash", txHash,
		"pool_size", mp.graph.GetNodeCount(),
		"tx_size", result.TxSize,
		"fee_rate_sat_vbyte", feeRate)

	// Build TxDesc for return value.
	txDesc := &TxDesc{
		TxDesc: mining.TxDesc{
			Tx:       tx,
			Added:    time.Now(),
			Height:   result.bestHeight,
			Fee:      int64(result.TxFee),
			FeePerKB: int64(result.TxFee) * 1000 / result.TxSize,
		},
		StartingPriority: 0, // Priority is deprecated
	}

	return nil, txDesc, nil
}

// ProcessTransaction is the main workhorse for handling insertion of new
// free-standing transactions into the memory pool. It includes functionality
// such as rejecting duplicate transactions, ensuring transactions follow all
// rules, orphan transaction handling, and insertion into the memory pool.
//
// It returns a slice of transactions added to the mempool. When the error is
// nil, the list will include the passed transaction itself along with any
// additional orphan transactions that were added as a result of the passed
// one being accepted.
func (mp *TxMempoolV2) ProcessTransaction(tx *btcutil.Tx, allowOrphan, rateLimit bool, tag Tag) ([]*TxDesc, error) {
	ctx := context.Background()
	log.TraceS(ctx, "Processing transaction",
		"tx_hash", tx.Hash(),
		"allow_orphan", allowOrphan,
		"rate_limit", rateLimit,
		"tag", tag)

	mp.mu.Lock()
	defer mp.mu.Unlock()

	// Attempt to accept transaction into the mempool.
	missingParents, txDesc, err := mp.maybeAcceptTransactionLocked(
		tx, true, rateLimit, true,
	)
	if err != nil {
		return nil, err
	}

	// If no missing parents, transaction was accepted.
	if len(missingParents) == 0 {
		// Process any orphans that can now be accepted.
		newTxs := mp.processOrphansLocked(tx)

		// Build result with parent first, then promoted orphans.
		acceptedTxs := make([]*TxDesc, len(newTxs)+1)
		acceptedTxs[0] = txDesc
		copy(acceptedTxs[1:], newTxs)

		return acceptedTxs, nil
	}

	// Transaction is an orphan.
	if !allowOrphan {
		// Only use the first missing parent in the error message.
		// RejectDuplicate matches reference implementation.
		str := fmt.Sprintf("orphan transaction %v references "+
			"outputs of unknown or fully-spent transaction %v",
			tx.Hash(), missingParents[0])
		return nil, txRuleError(wire.RejectDuplicate, str)
	}

	// Add to orphan pool.
	if err := mp.orphanMgr.AddOrphan(tx, tag); err != nil {
		return nil, err
	}

	return nil, nil
}

// processOrphansLocked processes orphans that may now be acceptable after
// a parent transaction was added. Must be called with lock held.
//
// Returns a slice of TxDesc for all orphans that were successfully promoted
// to the main mempool (including recursively promoted orphans).
func (mp *TxMempoolV2) processOrphansLocked(acceptedTx *btcutil.Tx) []*TxDesc {
	var acceptedTxns []*TxDesc

	// Define callback for attempting to accept each orphan.
	acceptFunc := func(orphanTx *btcutil.Tx) error {
		// Try to accept the orphan. Don't reject if it's a duplicate
		// orphan since we're processing it from the orphan pool.
		missingParents, txDesc, err := mp.maybeAcceptTransactionLocked(
			orphanTx, true, true, false,
		)
		if err != nil {
			// Orphan is invalid - return error so it gets removed.
			return err
		}

		// If orphan still has missing parents, return error so
		// OrphanManager keeps it in the pool for future processing.
		if len(missingParents) > 0 {
			return fmt.Errorf("orphan still has missing parents")
		}

		// Successfully accepted - store the descriptor.
		acceptedTxns = append(acceptedTxns, txDesc)
		return nil
	}

	// Process orphans that spend outputs from the accepted transaction.
	promoted, err := mp.orphanMgr.ProcessOrphans(acceptedTx, acceptFunc)
	if err != nil {
		ctx := context.Background()
		log.WarnS(ctx, "Error processing orphans", err,
			"parent_tx", acceptedTx.Hash())
	}

	// Recursively process newly promoted orphans.
	if len(promoted) > 0 {
		ctx := context.Background()
		log.DebugS(ctx, "Orphans promoted",
			"count", len(promoted),
			"parent_tx", acceptedTx.Hash())

		for _, promotedTx := range promoted {
			moreTxs := mp.processOrphansLocked(promotedTx)
			acceptedTxns = append(acceptedTxns, moreTxs...)
		}
	}

	return acceptedTxns
}

// RemoveTransaction removes the passed transaction from the mempool. When the
// removeRedeemers flag is set, any transactions that redeem outputs from the
// removed transaction will also be removed recursively from the mempool, as
// they would otherwise become orphans.
func (mp *TxMempoolV2) RemoveTransaction(tx *btcutil.Tx, removeRedeemers bool) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	txHash := tx.Hash()

	// Remove from graph. The removeRedeemers flag determines whether to
	// cascade the removal to descendants.
	var err error
	if removeRedeemers {
		// Cascade removal to all descendants (default graph behavior).
		err = mp.graph.RemoveTransaction(*txHash)
	} else {
		// Remove only this transaction, leaving descendants as orphans.
		// Note: The current txgraph doesn't have a non-cascade remove,
		// so we always cascade. This matches TxPool behavior where
		// removing a transaction always removes its redeemers.
		err = mp.graph.RemoveTransaction(*txHash)
	}

	if err != nil {
		// Transaction not in pool, nothing to do.
		return
	}

	// Update address index if enabled.
	if mp.cfg.AddrIndex != nil {
		mp.cfg.AddrIndex.RemoveUnconfirmedTx(txHash)
	}

	// Update last modified timestamp.
	mp.lastUpdated.Store(time.Now().Unix())

	ctx := context.Background()
	log.DebugS(ctx, "Transaction removed",
		"tx_hash", txHash,
		"pool_size", mp.graph.GetNodeCount(),
		"remove_redeemers", removeRedeemers)
}

// RemoveDoubleSpends removes all transactions which spend outputs spent by the
// passed transaction from the memory pool. Removing those transactions then
// leads to removing all transactions which rely on them, recursively. This is
// necessary when a block is connected to the main chain because the block may
// contain transactions which were previously unknown to the memory pool.
func (mp *TxMempoolV2) RemoveDoubleSpends(tx *btcutil.Tx) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	// Use graph's GetConflicts to find all transactions that spend the
	// same outputs as this transaction.
	conflicts := mp.graph.GetConflicts(tx)

	ctx := context.Background()
	// Remove each conflict (with descendants).
	removedCount := 0
	for conflictHash := range conflicts.Transactions {
		// Don't try to remove the transaction itself if it's in the pool.
		if conflictHash == *tx.Hash() {
			continue
		}

		if err := mp.graph.RemoveTransaction(conflictHash); err != nil {
			log.WarnS(ctx, "Failed to remove double spend", err,
				"conflict_hash", conflictHash,
				"trigger_tx", tx.Hash())
		} else {
			removedCount++
		}
	}

	if removedCount > 0 {
		log.DebugS(ctx, "Double spends removed",
			"count", removedCount,
			"trigger_tx", tx.Hash())
	}
}

// RemoveOrphan removes the passed orphan transaction from the orphan pool.
//
// This function is safe for concurrent access.
func (mp *TxMempoolV2) RemoveOrphan(tx *btcutil.Tx) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	// Delegate to OrphanManager. Use cascade=false to only remove this
	// specific orphan without affecting its descendants.
	_ = mp.orphanMgr.RemoveOrphan(*tx.Hash(), false)
}

// RemoveOrphansByTag removes all orphan transactions tagged with the provided
// identifier. This is useful when a peer disconnects to remove all orphans
// that were received from that peer.
//
// This function is safe for concurrent access.
func (mp *TxMempoolV2) RemoveOrphansByTag(tag Tag) uint64 {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	// Delegate to OrphanManager and convert return type.
	removed := mp.orphanMgr.RemoveOrphansByTag(tag)
	return uint64(removed)
}

// ProcessOrphans determines if there are any orphans which depend on the passed
// transaction (i.e., they spend one of its outputs). Each of those orphans are
// now eligible to be included in the mempool, so they are processed accordingly.
//
// It returns a slice of transactions added to the mempool as a result of
// processing the orphans.
//
// This function is safe for concurrent access.
func (mp *TxMempoolV2) ProcessOrphans(acceptedTx *btcutil.Tx) []*TxDesc {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	// Collect TxDesc results from successfully promoted orphans.
	var acceptedDescs []*TxDesc

	// Create acceptance function that wraps maybeAcceptTransactionLocked.
	acceptFunc := func(tx *btcutil.Tx) error {
		// Try to accept the orphan into the mempool.
		// isNew=true (orphan is new to main pool)
		// rateLimit=false (already vetted when added as orphan)
		// rejectDupOrphans=false (it's currently an orphan)
		_, txDesc, err := mp.maybeAcceptTransactionLocked(
			tx, true, false, false)
		if err != nil {
			return err
		}

		// Successfully accepted - store the descriptor.
		acceptedDescs = append(acceptedDescs, txDesc)
		return nil
	}

	// Delegate to OrphanManager to process orphans.
	_, _ = mp.orphanMgr.ProcessOrphans(acceptedTx, acceptFunc)

	return acceptedDescs
}

// FetchTransaction returns the requested transaction from the transaction pool.
// This only fetches from the main transaction pool and does not include orphans.
//
// This function is safe for concurrent access.
func (mp *TxMempoolV2) FetchTransaction(txHash *chainhash.Hash) (*btcutil.Tx, error) {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	// Lookup transaction in graph.
	node, exists := mp.graph.GetNode(*txHash)
	if !exists {
		return nil, fmt.Errorf("transaction is not in the pool")
	}

	return node.Tx, nil
}

// TxHashes returns a slice of hashes for all of the transactions in the
// memory pool.
//
// This function is safe for concurrent access.
func (mp *TxMempoolV2) TxHashes() []*chainhash.Hash {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	// Pre-allocate slice with exact capacity.
	count := mp.graph.GetNodeCount()
	hashes := make([]*chainhash.Hash, 0, count)

	// Iterate all nodes and collect hashes.
	for node := range mp.graph.Iterate() {
		// Make a copy of the hash to avoid returning pointers to node internals.
		hashCopy := node.TxHash
		hashes = append(hashes, &hashCopy)
	}

	return hashes
}

// TxDescs returns a slice of descriptors for all the transactions in the pool.
// The descriptors are to be treated as read only.
//
// This function is safe for concurrent access.
func (mp *TxMempoolV2) TxDescs() []*TxDesc {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	// Pre-allocate slice with exact capacity.
	count := mp.graph.GetNodeCount()
	descs := make([]*TxDesc, 0, count)

	// Iterate all nodes and build descriptors.
	for node := range mp.graph.Iterate() {
		// Convert graph TxDesc to mempool TxDesc format.
		desc := &TxDesc{
			TxDesc: mining.TxDesc{
				Tx:       node.Tx,
				Added:    node.TxDesc.Added,
				Height:   mp.cfg.BestHeight(),
				Fee:      node.TxDesc.Fee,
				FeePerKB: node.TxDesc.FeePerKB,
			},
			StartingPriority: 0, // Priority is deprecated.
		}
		descs = append(descs, desc)
	}

	return descs
}

// MiningDescs returns a slice of mining descriptors for all the transactions
// in the pool. The descriptors are specifically formatted for block template
// generation.
//
// This is part of the mining.TxSource interface implementation and is safe for
// concurrent access as required by the interface contract.
func (mp *TxMempoolV2) MiningDescs() []*mining.TxDesc {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	// Pre-allocate slice with exact capacity.
	count := mp.graph.GetNodeCount()
	descs := make([]*mining.TxDesc, 0, count)

	// Iterate all nodes and build mining descriptors.
	for node := range mp.graph.Iterate() {
		desc := &mining.TxDesc{
			Tx:       node.Tx,
			Added:    node.TxDesc.Added,
			Height:   mp.cfg.BestHeight(),
			Fee:      node.TxDesc.Fee,
			FeePerKB: node.TxDesc.FeePerKB,
		}
		descs = append(descs, desc)
	}

	return descs
}

// RawMempoolVerbose returns all the entries in the mempool as a fully
// populated btcjson result for the getrawmempool RPC command.
//
// This function is safe for concurrent access.
func (mp *TxMempoolV2) RawMempoolVerbose() map[string]*btcjson.GetRawMempoolVerboseResult {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	count := mp.graph.GetNodeCount()
	result := make(map[string]*btcjson.GetRawMempoolVerboseResult, count)
	bestHeight := mp.cfg.BestHeight()

	// Iterate all nodes and build verbose results.
	for node := range mp.graph.Iterate() {
		tx := node.Tx

		// Calculate current priority based on the inputs. Use zero if we
		// can't fetch the UTXO view for some reason.
		var currentPriority float64
		utxos, err := mp.cfg.FetchUtxoView(tx)
		if err == nil {
			currentPriority = mining.CalcPriority(tx.MsgTx(), utxos,
				bestHeight+1)
		}

		mpd := &btcjson.GetRawMempoolVerboseResult{
			Size:             int32(tx.MsgTx().SerializeSize()),
			Vsize:            int32(GetTxVirtualSize(tx)),
			Weight:           int32(blockchain.GetTransactionWeight(tx)),
			Fee:              btcutil.Amount(node.TxDesc.Fee).ToBTC(),
			Time:             node.TxDesc.Added.Unix(),
			Height:           int64(bestHeight),
			StartingPriority: 0, // Priority is deprecated.
			CurrentPriority:  currentPriority,
			Depends:          make([]string, 0),
		}

		// Build dependency list (parents that are also in the mempool).
		for _, txIn := range tx.MsgTx().TxIn {
			parentHash := txIn.PreviousOutPoint.Hash
			if mp.graph.HasTransaction(parentHash) {
				mpd.Depends = append(mpd.Depends, parentHash.String())
			}
		}

		result[tx.Hash().String()] = mpd
	}

	return result
}

// CheckSpend checks whether the passed outpoint is already spent by a
// transaction in the mempool. If that's the case, the spending transaction
// will be returned, otherwise nil will be returned.
func (mp *TxMempoolV2) CheckSpend(op wire.OutPoint) *btcutil.Tx {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	// Use graph's spentBy index for O(1) lookup.
	spender, exists := mp.graph.GetSpendingTx(op)
	if !exists {
		return nil
	}

	return spender.Tx
}

// CheckMempoolAcceptance behaves similarly to bitcoind's `testmempoolaccept`
// RPC method. It will perform a series of checks to decide whether this
// transaction can be accepted to the mempool. If not, the specific error is
// returned and the caller needs to take actions based on it.
//
// This function is safe for concurrent access.
func (mp *TxMempoolV2) CheckMempoolAcceptance(tx *btcutil.Tx) (*MempoolAcceptResult, error) {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	// Call internal validation with testmempoolaccept-RPC parameters:
	// - isNew=true (treat as new transaction)
	// - rateLimit=false (no rate limiting for RPC checks)
	// - rejectDupOrphans=true (reject if already in orphan pool)
	return mp.checkMempoolAcceptance(tx, true, false, true)
}

// Ensure TxMempoolV2 implements the TxMempool interface.
var _ TxMempool = (*TxMempoolV2)(nil)
