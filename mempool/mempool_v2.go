// Copyright (c) 2013-2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/blockchain/indexers"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mempool/txgraph"
	"github.com/btcsuite/btcd/mining"
	"github.com/btcsuite/btcd/txscript"
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
//  - Graph: Pure data structure, handles relationships
//  - OrphanManager: Lifecycle management for orphans
//  - PolicyEnforcer: Bitcoin Core-compatible policy decisions
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
	orphanMgr *OrphanManager

	// policy enforces mempool policies including RBF rules (BIP 125),
	// ancestor/descendant limits, and fee requirements. Separated from
	// graph to enable different policy configurations and easier testing.
	policy PolicyEnforcer

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
// This structure consolidates all configuration needed by TxMempoolV2
// and its components.
type MempoolConfig struct {
	// Policy defines mempool policy settings (relay rules, standardness).
	Policy Policy

	// ChainParams identifies the blockchain network (mainnet, testnet, etc).
	ChainParams *chaincfg.Params

	// FetchUtxoView provides UTXO data for transaction validation. This is
	// called during ProcessTransaction to validate inputs exist and aren't
	// double-spent.
	FetchUtxoView func(*btcutil.Tx) (*blockchain.UtxoViewpoint, error)

	// BestHeight returns the current blockchain tip height. Used for
	// timelock validation and policy decisions based on height.
	BestHeight func() int32

	// MedianTimePast returns median time past for the current chain tip.
	// Used for timelock validation (BIP 113).
	MedianTimePast func() time.Time

	// CalcSequenceLock calculates the sequence lock for a transaction
	// given its UTXO inputs. Used for relative timelock validation (BIP 68).
	CalcSequenceLock func(*btcutil.Tx, *blockchain.UtxoViewpoint) (*blockchain.SequenceLock, error)

	// IsDeploymentActive checks if a soft fork deployment is active at the
	// current tip. Used to enforce new consensus rules for transactions.
	IsDeploymentActive func(deploymentID uint32) (bool, error)

	// SigCache provides signature verification caching to avoid redundant
	// ECDSA operations when validating transactions.
	SigCache *txscript.SigCache

	// HashCache provides transaction hash mid-state caching for sighash
	// calculations, improving validation performance.
	HashCache *txscript.HashCache

	// AddrIndex optionally indexes mempool transactions by address. Can be
	// nil if address indexing is disabled.
	AddrIndex *indexers.AddrIndex

	// FeeEstimator optionally tracks transaction fee rates for fee
	// estimation. Can be nil if fee estimation is disabled.
	FeeEstimator *FeeEstimator

	// GraphConfig configures the underlying transaction graph. If nil,
	// defaults will be applied (100k node capacity, 101 tx max package size).
	GraphConfig *txgraph.Config

	// OrphanConfig configures orphan transaction management. If nil,
	// defaults will be applied (15 min TTL, 100 orphan limit).
	OrphanConfig OrphanConfig

	// PolicyConfig configures policy enforcement. If nil, defaults will be
	// applied (Bitcoin Core compatible: 25 ancestor limit, BIP 125 RBF).
	PolicyConfig PolicyConfig
}

// NewTxMempoolV2 creates a new graph-based mempool with the provided
// configuration. This constructor initializes all components (graph, orphan
// manager, policy enforcer) with appropriate defaults where configuration
// is not provided.
//
// The mempool is ready to use immediately after construction, but method
// implementations for transaction operations are not yet complete. This
// constructor establishes the structure for subsequent implementation tasks.
func NewTxMempoolV2(cfg *MempoolConfig) *TxMempoolV2 {
	if cfg == nil {
		cfg = &MempoolConfig{}
	}

	// Initialize graph with provided config or sensible defaults.
	// Default: 100k nodes, 101 tx max package (25 ancestors + 25
	// descendants + 1 root).
	graphCfg := cfg.GraphConfig
	if graphCfg == nil {
		graphCfg = txgraph.DefaultConfig()
	}
	graph := txgraph.New(graphCfg)

	// Initialize orphan manager with Bitcoin Core compatible defaults.
	// Default: 15 min TTL, 100 orphan limit, 5 min expire scan interval.
	orphanCfg := cfg.OrphanConfig
	if orphanCfg.MaxOrphans == 0 {
		orphanCfg.MaxOrphans = DefaultMaxOrphanTxs
		orphanCfg.MaxOrphanSize = DefaultMaxOrphanTxSize
		orphanCfg.OrphanTTL = DefaultOrphanTTL
		orphanCfg.ExpireScanInterval = DefaultOrphanExpireScanInterval
	}
	orphanMgr := NewOrphanManager(orphanCfg)

	// Initialize policy enforcer with Bitcoin Core defaults.
	// Default: 25 ancestor/descendant limit, 101 KB size limit,
	// BIP 125 RBF with 0xfffffffd sequence threshold.
	policyCfg := cfg.PolicyConfig
	if policyCfg.MaxAncestorCount == 0 {
		policyCfg = DefaultPolicyConfig()
	}
	policy := NewStandardPolicyEnforcer(policyCfg)

	mp := &TxMempoolV2{
		graph:     graph,
		orphanMgr: orphanMgr,
		policy:    policy,
		cfg:       *cfg,
	}

	// Initialize last updated timestamp to current time.
	mp.lastUpdated.Store(time.Now().Unix())

	return mp
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
func (mp *TxMempoolV2) MaybeAcceptTransaction(tx *btcutil.Tx, isNew, rateLimit bool) ([]*chainhash.Hash, *TxDesc, error) {
	panic("MaybeAcceptTransaction: not implemented")
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
//
// STUB: Implementation pending in implement-tx-operations task.
func (mp *TxMempoolV2) ProcessTransaction(tx *btcutil.Tx, allowOrphan, rateLimit bool, tag Tag) ([]*TxDesc, error) {
	panic("ProcessTransaction: not implemented")
}

// RemoveTransaction removes the passed transaction from the mempool. When the
// removeRedeemers flag is set, any transactions that redeem outputs from the
// removed transaction will also be removed recursively from the mempool, as
// they would otherwise become orphans.
//
// STUB: Implementation pending in implement-tx-operations task.
func (mp *TxMempoolV2) RemoveTransaction(tx *btcutil.Tx, removeRedeemers bool) {
	panic("RemoveTransaction: not implemented")
}

// RemoveDoubleSpends removes all transactions which spend outputs spent by the
// passed transaction from the memory pool. Removing those transactions then
// leads to removing all transactions which rely on them, recursively. This is
// necessary when a block is connected to the main chain because the block may
// contain transactions which were previously unknown to the memory pool.
//
// STUB: Implementation pending in implement-tx-operations task.
func (mp *TxMempoolV2) RemoveDoubleSpends(tx *btcutil.Tx) {
	panic("RemoveDoubleSpends: not implemented")
}

// RemoveOrphan removes the passed orphan transaction from the orphan pool.
//
// STUB: Implementation pending in implement-orphan-ops task.
func (mp *TxMempoolV2) RemoveOrphan(tx *btcutil.Tx) {
	panic("RemoveOrphan: not implemented")
}

// RemoveOrphansByTag removes all orphan transactions tagged with the provided
// identifier. This is useful when a peer disconnects to remove all orphans
// that were received from that peer.
//
// STUB: Implementation pending in implement-orphan-ops task.
func (mp *TxMempoolV2) RemoveOrphansByTag(tag Tag) uint64 {
	panic("RemoveOrphansByTag: not implemented")
}

// ProcessOrphans determines if there are any orphans which depend on the passed
// transaction (i.e., they spend one of its outputs). Each of those orphans are
// now eligible to be included in the mempool, so they are processed accordingly.
//
// It returns a slice of transactions added to the mempool as a result of
// processing the orphans.
//
// STUB: Implementation pending in implement-orphan-ops task.
func (mp *TxMempoolV2) ProcessOrphans(acceptedTx *btcutil.Tx) []*TxDesc {
	panic("ProcessOrphans: not implemented")
}

// FetchTransaction returns the requested transaction from the transaction pool.
// This only fetches from the main transaction pool and does not include orphans.
//
// STUB: Implementation pending in implement-query-ops task.
func (mp *TxMempoolV2) FetchTransaction(txHash *chainhash.Hash) (*btcutil.Tx, error) {
	panic("FetchTransaction: not implemented")
}

// TxHashes returns a slice of hashes for all of the transactions in the
// memory pool.
//
// STUB: Implementation pending in implement-query-ops task.
func (mp *TxMempoolV2) TxHashes() []*chainhash.Hash {
	panic("TxHashes: not implemented")
}

// TxDescs returns a slice of descriptors for all the transactions in the pool.
// The descriptors are to be treated as read only.
//
// STUB: Implementation pending in implement-query-ops task.
func (mp *TxMempoolV2) TxDescs() []*TxDesc {
	panic("TxDescs: not implemented")
}

// MiningDescs returns a slice of mining descriptors for all the transactions
// in the pool. The descriptors are specifically formatted for block template
// generation.
//
// STUB: Implementation pending in implement-query-ops task.
func (mp *TxMempoolV2) MiningDescs() []*mining.TxDesc {
	panic("MiningDescs: not implemented")
}

// RawMempoolVerbose returns all the entries in the mempool as a fully
// populated btcjson result for the getrawmempool RPC command.
//
// STUB: Implementation pending in implement-query-ops task.
func (mp *TxMempoolV2) RawMempoolVerbose() map[string]*btcjson.GetRawMempoolVerboseResult {
	panic("RawMempoolVerbose: not implemented")
}

// CheckSpend checks whether the passed outpoint is already spent by a
// transaction in the mempool. If that's the case, the spending transaction
// will be returned, otherwise nil will be returned.
//
// STUB: Implementation pending in implement-query-ops task.
func (mp *TxMempoolV2) CheckSpend(op wire.OutPoint) *btcutil.Tx {
	panic("CheckSpend: not implemented")
}

// CheckMempoolAcceptance behaves similarly to bitcoind's `testmempoolaccept`
// RPC method. It will perform a series of checks to decide whether this
// transaction can be accepted to the mempool. If not, the specific error is
// returned and the caller needs to take actions based on it.
//
// STUB: Implementation pending in implement-rbf-validation task.
func (mp *TxMempoolV2) CheckMempoolAcceptance(tx *btcutil.Tx) (*MempoolAcceptResult, error) {
	panic("CheckMempoolAcceptance: not implemented")
}

// Ensure TxMempoolV2 implements the TxMempool interface.
var _ TxMempool = (*TxMempoolV2)(nil)
