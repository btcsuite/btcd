package mempool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mempool/txgraph"
	"github.com/btcsuite/btcd/wire"
)

// Orphan-specific errors.
var (
	// ErrOrphanAlreadyExists indicates the orphan transaction already
	// exists in the orphan pool.
	ErrOrphanAlreadyExists = errors.New("orphan already exists")

	// ErrOrphanTooLarge indicates the orphan transaction exceeds the
	// maximum allowed size.
	ErrOrphanTooLarge = errors.New("orphan too large")

	// ErrOrphanLimitReached indicates the orphan pool is at capacity.
	ErrOrphanLimitReached = errors.New("orphan limit reached")

	// ErrOrphanNotFound indicates the orphan transaction was not found in
	// the orphan pool.
	ErrOrphanNotFound = errors.New("orphan not found")
)

// OrphanManager manages orphan transactions using a separate transaction
// graph. An orphan is a transaction whose inputs reference outputs not yet
// confirmed or in the mempool.
//
// The key design decision is using a separate graph instance rather than
// storing orphans in the main mempool graph. This separation provides several
// benefits:
//
//  1. Isolation: Orphans are potentially invalid or spam transactions that
//     shouldn't pollute the main graph used for mining and fee estimation.
//
//  2. Different Lifecycle: Orphans have TTL-based expiration and peer-based
//     tagging, which don't apply to confirmed mempool transactions.
//
//  3. Package Tracking: Despite being separate, the graph structure enables
//     tracking of orphan packages. When a parent arrives, we can efficiently
//     find all descendant orphans that can be promoted together.
//
// The orphan graph is cleared when parents arrive and orphans are promoted to
// the main mempool, or when orphans expire or are removed due to peer actions.
type OrphanManager struct {
	// graph stores orphan transactions and their dependencies. This is a
	// completely separate graph from the main mempool to maintain
	// isolation between validated and potentially-invalid transactions.
	graph *txgraph.TxGraph

	// metadata stores orphan-specific information not tracked by the graph.
	// This includes the peer tag (for removal by peer), expiration time
	// (for TTL enforcement), and size (for limit enforcement).
	metadata map[chainhash.Hash]*orphanMetadata

	// byTag provides an index from peer tag to the set of orphans received
	// from that peer. This enables O(1) lookup when removing all orphans
	// from a disconnected or misbehaving peer.
	byTag map[Tag]map[chainhash.Hash]struct{}

	// config contains orphan-specific limits and policies that don't apply
	// to the main mempool.
	config OrphanConfig

	// nextExpireScan tracks when the next expiration scan should run. This
	// enables lazy expiration: we only scan for expired orphans
	// periodically rather than on every operation.
	nextExpireScan time.Time

	// mu protects all orphan manager state. RWMutex allows concurrent
	// reads (IsOrphan, GetOrphan) while serializing writes.
	mu sync.RWMutex
}

// orphanMetadata contains metadata about an orphan transaction that isn't
// part of the graph structure.
type orphanMetadata struct {
	// tag identifies the peer that sent this orphan, enabling efficient
	// removal of all orphans from a specific peer.
	tag Tag

	// expiration is the absolute time when this orphan should be removed.
	// Orphans have a limited lifetime to prevent memory exhaustion from
	// spam attacks.
	expiration time.Time

	// size is the transaction size in bytes, used to enforce total orphan
	// size limits across all orphans.
	size int
}

// OrphanConfig defines limits and policies for orphan transaction management.
type OrphanConfig struct {
	// MaxOrphans limits the total number of orphan transactions. Bitcoin
	// Core uses 100 as a reasonable limit to prevent memory exhaustion.
	MaxOrphans int

	// MaxOrphanSize limits the size of a single orphan transaction in
	// bytes. This prevents a single large transaction from consuming
	// excessive memory.
	MaxOrphanSize int

	// OrphanTTL defines how long an orphan remains in memory before
	// expiration. Bitcoin Core uses 20 minutes.
	OrphanTTL time.Duration

	// ExpireScanInterval defines how often to scan for expired orphans.
	// Less frequent scans reduce CPU usage but allow expired orphans to
	// linger longer in memory.
	ExpireScanInterval time.Duration
}

// DefaultOrphanConfig returns the default orphan configuration matching
// Bitcoin Core's behavior.
func DefaultOrphanConfig() OrphanConfig {
	return OrphanConfig{
		MaxOrphans:         100,
		MaxOrphanSize:      100000, // 100KB
		OrphanTTL:          20 * time.Minute,
		ExpireScanInterval: 5 * time.Minute,
	}
}

// NewOrphanManager creates a new orphan manager with the given configuration.
func NewOrphanManager(cfg OrphanConfig) *OrphanManager {
	// Create a dedicated graph for orphans. Use a small MaxNodes since
	// orphan count is limited, and no package analyzer since we don't
	// validate orphan packages until they're promoted.
	graphCfg := &txgraph.Config{
		MaxNodes:       cfg.MaxOrphans,
		MaxPackageSize: 101, // Same as main graph
	}

	return &OrphanManager{
		graph:          txgraph.New(graphCfg),
		metadata:       make(map[chainhash.Hash]*orphanMetadata),
		byTag:          make(map[Tag]map[chainhash.Hash]struct{}),
		config:         cfg,
		nextExpireScan: time.Now().Add(cfg.ExpireScanInterval),
	}
}

// AddOrphan adds an orphan transaction to the manager. The transaction is
// tagged with the peer that sent it and assigned an expiration time based on
// the configured TTL.
//
// Returns an error if:
//   - The transaction is too large (exceeds MaxOrphanSize)
//   - The orphan limit would be exceeded (MaxOrphans reached)
//   - The transaction already exists as an orphan
func (om *OrphanManager) AddOrphan(tx *btcutil.Tx, tag Tag) error {
	om.mu.Lock()
	defer om.mu.Unlock()

	ctx := context.Background()
	hash := *tx.Hash()

	// Check if already exists.
	if _, exists := om.metadata[hash]; exists {
		log.DebugS(ctx, "Orphan already exists",
			"tx_hash", hash,
			"tag", tag)
		return fmt.Errorf("%w: %v", ErrOrphanAlreadyExists, hash)
	}

	// Check size limit.
	size := tx.MsgTx().SerializeSize()
	if size > om.config.MaxOrphanSize {
		log.WarnS(ctx, "Orphan too large", nil,
			"tx_hash", hash,
			"size", size,
			"max_size", om.config.MaxOrphanSize)
		return fmt.Errorf("%w: %d bytes (max %d)",
			ErrOrphanTooLarge, size, om.config.MaxOrphanSize)
	}

	currentCount := len(om.metadata)
	// Check count limit.
	if currentCount >= om.config.MaxOrphans {
		log.WarnS(ctx, "Orphan pool limit reached", nil,
			"tx_hash", hash,
			"current_count", currentCount,
			"max_orphans", om.config.MaxOrphans,
			"tag", tag)
		return fmt.Errorf("%w: %d", ErrOrphanLimitReached, om.config.MaxOrphans)
	}

	// Warn when approaching capacity (potential memory DoS).
	if currentCount > om.config.MaxOrphans*8/10 {
		log.InfoS(ctx, "Orphan pool nearing capacity",
			"current_count", currentCount,
			"max_orphans", om.config.MaxOrphans,
			"utilization_pct", currentCount*100/om.config.MaxOrphans)
	}

	// Add to graph. Use a minimal TxDesc since we don't need fee
	// information for orphans (they're not mined until promoted).
	desc := &txgraph.TxDesc{
		TxHash:      hash,
		VirtualSize: int64(size),
		Fee:         0,
		FeePerKB:    0,
		Added:       time.Now(),
	}

	if err := om.graph.AddTransaction(tx, desc); err != nil {
		return fmt.Errorf("failed to add to graph: %w", err)
	}

	// Store metadata.
	om.metadata[hash] = &orphanMetadata{
		tag:        tag,
		expiration: time.Now().Add(om.config.OrphanTTL),
		size:       size,
	}

	// Update tag index.
	if om.byTag[tag] == nil {
		om.byTag[tag] = make(map[chainhash.Hash]struct{})
	}
	om.byTag[tag][hash] = struct{}{}

	log.DebugS(ctx, "Orphan added",
		"tx_hash", hash,
		"size", size,
		"tag", tag,
		"pool_size", len(om.metadata),
		"max_orphans", om.config.MaxOrphans)

	return nil
}

// RemoveOrphan removes an orphan transaction and optionally all its
// descendants. This is used when an orphan is promoted to the main mempool or
// when cleaning up invalid orphans.
func (om *OrphanManager) RemoveOrphan(hash chainhash.Hash, cascade bool) error {
	om.mu.Lock()
	defer om.mu.Unlock()

	return om.removeOrphanUnsafe(hash, cascade)
}

// removeOrphanUnsafe removes an orphan without locking. Must be called with
// lock held.
func (om *OrphanManager) removeOrphanUnsafe(hash chainhash.Hash, cascade bool) error {
	ctx := context.Background()
	_, exists := om.metadata[hash]
	if !exists {
		log.DebugS(ctx, "Orphan not found for removal",
			"tx_hash", hash)
		return fmt.Errorf("%w: %v", ErrOrphanNotFound, hash)
	}

	// If cascading, collect all descendants first so we can clean up their
	// metadata before removing from graph.
	var toRemove []chainhash.Hash
	toRemove = append(toRemove, hash)

	if cascade {
		// Get all descendants that will be removed.
		descendants := om.graph.GetDescendants(hash, -1)
		for descHash := range descendants {
			toRemove = append(toRemove, descHash)
		}

		if len(descendants) > 0 {
			log.DebugS(ctx, "Cascading orphan removal",
				"tx_hash", hash,
				"descendant_count", len(descendants))
		}
	}

	// Remove metadata and tag index entries for all transactions.
	for _, h := range toRemove {
		if m, exists := om.metadata[h]; exists {
			// Remove from tag index.
			if tagSet, exists := om.byTag[m.tag]; exists {
				delete(tagSet, h)
				if len(tagSet) == 0 {
					delete(om.byTag, m.tag)
				}
			}
			// Remove metadata.
			delete(om.metadata, h)
		}
	}

	// Remove from graph. Use cascade based on caller's preference.
	log.DebugS(ctx, "Orphan removed",
		"tx_hash", hash,
		"cascade", cascade,
		"removed_count", len(toRemove),
		"pool_size", len(om.metadata))

	if cascade {
		return om.graph.RemoveTransaction(hash)
	}
	return om.graph.RemoveTransactionNoCascade(hash)
}

// RemoveOrphansByTag removes all orphans received from a specific peer. This
// is used when a peer disconnects or is detected misbehaving.
//
// Returns the number of orphans removed.
func (om *OrphanManager) RemoveOrphansByTag(tag Tag) int {
	om.mu.Lock()
	defer om.mu.Unlock()

	ctx := context.Background()
	tagSet, exists := om.byTag[tag]
	if !exists {
		return 0
	}

	// Collect hashes to remove (can't modify map while iterating).
	toRemove := make([]chainhash.Hash, 0, len(tagSet))
	for hash := range tagSet {
		toRemove = append(toRemove, hash)
	}

	log.DebugS(ctx, "Removing orphans by tag",
		"tag", tag,
		"count", len(toRemove))

	// Remove each orphan.
	removed := 0
	for _, hash := range toRemove {
		if err := om.removeOrphanUnsafe(hash, true); err == nil {
			removed++
		}
	}

	if removed > 0 {
		log.DebugS(ctx, "Orphans removed by tag",
			"tag", tag,
			"removed_count", removed,
			"pool_size", len(om.metadata))
	}

	return removed
}

// ExpireOrphans removes all orphans that have exceeded their TTL. This should
// be called periodically to prevent accumulation of stale orphans.
//
// Returns the number of orphans expired.
func (om *OrphanManager) ExpireOrphans() int {
	om.mu.Lock()
	defer om.mu.Unlock()

	ctx := context.Background()
	// Only scan if it's time for the next expiration scan.
	now := time.Now()
	if now.Before(om.nextExpireScan) {
		return 0
	}

	// Update next scan time.
	om.nextExpireScan = now.Add(om.config.ExpireScanInterval)

	poolSize := len(om.metadata)
	log.TraceS(ctx, "Scanning for expired orphans",
		"pool_size", poolSize,
		"ttl", om.config.OrphanTTL)

	// Find expired orphans.
	var expired []chainhash.Hash
	for hash, meta := range om.metadata {
		if now.After(meta.expiration) {
			expired = append(expired, hash)
		}
	}

	// Remove expired orphans.
	removed := 0
	for _, hash := range expired {
		if err := om.removeOrphanUnsafe(hash, true); err == nil {
			removed++
		}
	}

	if removed > 0 {
		log.DebugS(ctx, "Expired orphans removed",
			"removed_count", removed,
			"pool_size", len(om.metadata),
			"ttl", om.config.OrphanTTL)
	}

	return removed
}

// IsOrphan checks if a transaction is currently tracked as an orphan.
func (om *OrphanManager) IsOrphan(hash chainhash.Hash) bool {
	om.mu.RLock()
	defer om.mu.RUnlock()

	_, exists := om.metadata[hash]
	return exists
}

// GetOrphan retrieves an orphan transaction if it exists.
func (om *OrphanManager) GetOrphan(hash chainhash.Hash) (*btcutil.Tx, bool) {
	om.mu.RLock()
	defer om.mu.RUnlock()

	node, exists := om.graph.GetNode(hash)
	if !exists {
		return nil, false
	}

	return node.Tx, true
}

// Count returns the current number of orphans.
func (om *OrphanManager) Count() int {
	om.mu.RLock()
	defer om.mu.RUnlock()

	return len(om.metadata)
}

// ProcessOrphans attempts to promote orphans when a parent transaction
// arrives. It finds all orphans that spend from the given parent and calls
// acceptFunc for each one. If acceptFunc succeeds, the orphan is removed from
// the orphan manager.
//
// This enables batch processing of orphan chains: when a transaction is
// accepted into the mempool, any orphans spending from it can be recursively
// processed.
//
// Returns the list of successfully promoted transactions.
func (om *OrphanManager) ProcessOrphans(
	parentTx *btcutil.Tx,
	acceptFunc func(*btcutil.Tx) error,
) ([]*btcutil.Tx, error) {

	om.mu.Lock()
	defer om.mu.Unlock()

	ctx := context.Background()
	parentHash := parentTx.Hash()
	log.TraceS(ctx, "Processing orphans for parent",
		"parent_tx", parentHash,
		"orphan_pool_size", len(om.metadata))

	// Find orphans that spend from this parent's outputs using the spentBy
	// index. The parent itself is not in the orphan graph (it's in the main
	// mempool), but orphans that spend its outputs are indexed by outpoint.
	var promoted []*btcutil.Tx
	toProcess := txgraph.NewQueue[*txgraph.TxGraphNode]()
	visited := make(map[chainhash.Hash]bool)

	// Check each output of the parent to see if it's spent by an orphan.
	// Iterate through actual outputs of the transaction.
	for txOutIdx := range parentTx.MsgTx().TxOut {
		outpoint := wire.OutPoint{
			Hash:  *parentHash,
			Index: uint32(txOutIdx),
		}

		child, exists := om.graph.GetSpendingTx(outpoint)
		if !exists {
			// No orphan spends this output.
			continue
		}

		// Found an orphan that spends this parent output.
		if !visited[child.TxHash] {
			toProcess.Enqueue(child)
			visited[child.TxHash] = true
		}
	}

	// Process each orphan and its descendants.
	for !toProcess.IsEmpty() {
		node, _ := toProcess.Dequeue()

		// Try to promote this orphan.
		if err := acceptFunc(node.Tx); err != nil {
			// If promotion fails, skip this orphan and its descendants.
			// They'll remain in the orphan pool for potential future
			// promotion or eventual expiration.
			log.DebugS(ctx, "Orphan promotion failed",
				"orphan_tx", node.Tx.Hash(),
				"parent_tx", parentHash,
				"reason", err.Error())
			continue
		}

		// Promotion succeeded! Remove from orphan manager.
		if err := om.removeOrphanUnsafe(*node.Tx.Hash(), false); err != nil {
			// Log error but continue processing other orphans.
			log.WarnS(ctx, "Failed to remove promoted orphan", err,
				"orphan_tx", node.Tx.Hash())
			continue
		}

		log.DebugS(ctx, "Orphan promoted",
			"orphan_tx", node.Tx.Hash(),
			"parent_tx", parentHash)

		promoted = append(promoted, node.Tx)

		// Add this orphan's children to the processing queue.
		for _, child := range node.Children {
			if !visited[child.TxHash] {
				toProcess.Enqueue(child)
				visited[child.TxHash] = true
			}
		}
	}

	if len(promoted) > 0 {
		log.DebugS(ctx, "Orphan processing complete",
			"parent_tx", parentHash,
			"promoted_count", len(promoted),
			"orphan_pool_size", len(om.metadata))
	}

	return promoted, nil
}

// Ensure OrphanManager implements the OrphanTxManager interface.
var _ OrphanTxManager = (*OrphanManager)(nil)
