package txgraph

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

var (
	// ErrTransactionExists is returned when attempting to add a duplicate
	// transaction.
	ErrTransactionExists = errors.New("transaction already exists in graph")

	// ErrNodeNotFound is returned when a node is not found in the graph.
	ErrNodeNotFound = errors.New("node not found in graph")

	// ErrInvalidTopology is returned when a package has invalid topology.
	ErrInvalidTopology = errors.New("invalid package topology")

	// ErrDisconnectedPackage is returned when package nodes are not
	// connected.
	ErrDisconnectedPackage = errors.New("package contains disconnected " +
		"nodes")

	// ErrCycleDetected is returned when a cycle is detected in the graph.
	ErrCycleDetected = errors.New("cycle detected in graph")

	// ErrMaxDepthExceeded is returned when max traversal depth is exceeded.
	ErrMaxDepthExceeded = errors.New("maximum depth exceeded")

	// ErrInvalidEdge is returned when attempting to create an invalid edge.
	ErrInvalidEdge = errors.New("invalid edge")
)

// Config defines configuration for the transaction graph.
type Config struct {
	// MaxNodes limits graph capacity to prevent unbounded memory growth.
	// When reached, new transaction additions will be rejected, triggering
	// mempool eviction policies in the caller.
	MaxNodes int

	// MaxPackageSize limits the number of transactions in a package.
	// Bitcoin Core uses 101 (25 ancestors + 25 descendants + 1 root),
	// enforced here to prevent package relay DoS attacks.
	MaxPackageSize int

	// PackageAnalyzer provides protocol-specific validation logic. If nil,
	// package identification will use heuristics only without protocol
	// enforcement (useful for testing or non-standard configurations).
	PackageAnalyzer PackageAnalyzer
}

// DefaultConfig returns the default graph configuration.
func DefaultConfig() *Config {
	return &Config{
		MaxNodes:       100000,
		MaxPackageSize: 101,
	}
}

// TxGraph implements the Graph interface.
type TxGraph struct {
	config *Config

	// analyzer provides protocol-specific validation logic for transaction
	// packages. Abstracted as an interface to enable testing with mocks and
	// to support future protocol upgrades without modifying the core graph.
	analyzer PackageAnalyzer

	// nodes stores all transactions currently in the mempool. Hash map
	// provides O(1) lookups by transaction ID, which is critical for
	// performance as the mempool can contain thousands of transactions.
	nodes map[chainhash.Hash]*TxGraphNode

	// indexes contains auxiliary data structures for O(1) lookups that
	// would otherwise require O(n) graph traversal.
	indexes struct {
		// spentBy maps an outpoint to the transaction that spends it.
		// This enables orphan transaction handling: when a transaction
		// arrives before its parent, we can quickly connect them once
		// the parent arrives without rescanning all transactions.
		spentBy map[wire.OutPoint]*TxGraphNode

		// clusters maps cluster IDs to connected components. Clusters
		// are used for RBF validation (replacement must improve entire
		// cluster fee) and mempool eviction policies.
		clusters map[ClusterID]*TxCluster

		// nodeToCluster provides O(1) cluster lookup for any
		// transaction, avoiding repeated graph traversal to find
		// connected components.
		nodeToCluster map[chainhash.Hash]ClusterID

		// packages maps package IDs to identified transaction packages.
		// Packages are used for package relay policies and block
		// template construction.
		packages map[PackageID]*TxPackage

		// nodeToPackage enables quick package membership checks without
		// recomputing package structures.
		nodeToPackage map[chainhash.Hash]PackageID

		// trucTxs indexes v3 (TRUC) transactions for efficient TRUC
		// policy enforcement without scanning all nodes.
		trucTxs map[chainhash.Hash]*TxGraphNode

		// ephemeralTxs indexes transactions with ephemeral dust outputs
		// for package validation and relay policy checks.
		ephemeralTxs map[chainhash.Hash]*TxGraphNode
	}

	// metrics tracks aggregate statistics using atomic operations to enable
	// lock-free reads. This is essential because metrics are frequently
	// queried for monitoring and eviction decisions.
	metrics struct {
		nodeCount      int32
		edgeCount      int32
		packageCount   int32
		clusterCount   int32
		trucCount      int32
		ephemeralCount int32
	}

	// nextClusterID generates monotonically increasing cluster identifiers.
	// Uses atomic.Uint64 to avoid contention on the main graph mutex during
	// high-frequency transaction additions.
	nextClusterID atomic.Uint64

	// mu protects the graph structure. RWMutex allows concurrent reads
	// (queries, iteration) while serializing writes (add/remove operations).
	mu sync.RWMutex
}

// New creates a new transaction graph.
func New(config *Config) *TxGraph {
	if config == nil {
		config = DefaultConfig()
	}

	g := &TxGraph{
		config:   config,
		analyzer: config.PackageAnalyzer,
		nodes:    make(map[chainhash.Hash]*TxGraphNode),
	}

	g.indexes.spentBy = make(map[wire.OutPoint]*TxGraphNode)
	g.indexes.clusters = make(map[ClusterID]*TxCluster)
	g.indexes.nodeToCluster = make(map[chainhash.Hash]ClusterID)
	g.indexes.packages = make(map[PackageID]*TxPackage)
	g.indexes.nodeToPackage = make(map[chainhash.Hash]PackageID)
	g.indexes.trucTxs = make(map[chainhash.Hash]*TxGraphNode)
	g.indexes.ephemeralTxs = make(map[chainhash.Hash]*TxGraphNode)

	return g
}

// AddTransaction adds a transaction to the graph.
func (g *TxGraph) AddTransaction(tx *btcutil.Tx, txDesc *TxDesc) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	hash := tx.Hash()

	// Check if already exists.
	if _, exists := g.nodes[*hash]; exists {
		return ErrTransactionExists
	}

	// Check capacity.
	if int(atomic.LoadInt32(&g.metrics.nodeCount)) >= g.config.MaxNodes {
		return fmt.Errorf("graph at capacity: %d nodes",
			g.config.MaxNodes)
	}

	// Create new node.
	node := &TxGraphNode{
		TxHash:   *hash,
		Tx:       tx,
		TxDesc:   txDesc,
		Parents:  make(map[chainhash.Hash]*TxGraphNode),
		Children: make(map[chainhash.Hash]*TxGraphNode),
	}

	// Set metadata.
	node.Metadata.AddedTime = time.Now()
	node.Metadata.IsTRUC = (tx.MsgTx().Version == 3)
	node.Metadata.ClusterID = ClusterID(g.nextClusterID.Add(1))

	// Add to graph.
	g.nodes[*hash] = node

	// Connect edges based on inputs.
	for _, txIn := range tx.MsgTx().TxIn {
		parentHash := txIn.PreviousOutPoint.Hash

		// Always update spentBy index, even if parent doesn't exist yet.
		// This handles the orphan case where a child arrives before its
		// parent: when the parent eventually arrives, we use this index
		// to quickly find and connect all waiting children without
		// rescanning the entire mempool. This is a time-space tradeoff:
		// we maintain extra index entries for orphans to avoid O(n)
		// scans on every parent arrival.
		g.indexes.spentBy[txIn.PreviousOutPoint] = node

		if parent, exists := g.nodes[parentHash]; exists {
			// Create bidirectional edge between parent and child.
			node.Parents[parentHash] = parent
			parent.Children[*hash] = node

			atomic.AddInt32(&g.metrics.edgeCount, 1)
		}
	}

	// Find children that spend this transaction.
	for i := range tx.MsgTx().TxOut {
		outpoint := wire.OutPoint{
			Hash:  *hash,
			Index: uint32(i),
		}

		if child, exists := g.indexes.spentBy[outpoint]; exists {
			node.Children[child.TxHash] = child
			child.Parents[*hash] = node
			atomic.AddInt32(&g.metrics.edgeCount, 1)
		}
	}

	// Update cluster assignment after all edges are established. Cluster
	// identification requires knowing the node's complete set of parents
	// and children to determine which existing clusters need to be merged.
	// Doing this before edge creation would result in incorrect cluster
	// assignments when the node bridges multiple clusters.
	g.updateClusterAssignment(node)

	// Update feature-specific indexes.
	if node.Metadata.IsTRUC {
		g.indexes.trucTxs[*hash] = node
		atomic.AddInt32(&g.metrics.trucCount, 1)
	}

	// Update metrics.
	atomic.AddInt32(&g.metrics.nodeCount, 1)

	return nil
}

// RemoveTransaction removes a transaction from the graph.
func (g *TxGraph) RemoveTransaction(hash chainhash.Hash) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	_, exists := g.nodes[hash]
	if !exists {
		return ErrNodeNotFound
	}

	// Collect all descendants to remove.
	toRemove := g.collectDescendantsToRemove(hash)

	// Remove in reverse order (children before parents). This ordering is
	// required because removeTransactionUnsafe updates parent nodes to
	// remove child references. If we removed parents first, we'd be
	// modifying the Children map of deleted nodes, which could cause
	// panics or leave dangling pointers. Removing children first ensures
	// all parent.Children map updates refer to live nodes.
	for i := len(toRemove) - 1; i >= 0; i-- {
		// Skip if node was already removed. This can occur when a node
		// has multiple parents that were both in the removal set.
		if _, exists := g.nodes[toRemove[i]]; !exists {
			continue
		}
		if err := g.removeTransactionUnsafe(toRemove[i]); err != nil {
			return err
		}
	}

	return nil
}

// RemoveTransactionNoCascade removes a transaction without removing its
// descendants. This is used when a transaction is confirmed in a block - the
// transaction leaves the mempool but its children remain valid (they now
// reference a confirmed input).
//
// The children will have their parent reference updated to remove the
// confirmed transaction, but they remain in the graph.
//
// This is in contrast to RemoveTransaction which cascades and removes all
// descendants, which is appropriate when a transaction is evicted/invalidated
// (e.g., replaced by RBF).
func (g *TxGraph) RemoveTransactionNoCascade(hash chainhash.Hash) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	_, exists := g.nodes[hash]
	if !exists {
		return ErrNodeNotFound
	}

	// Remove just this transaction, without cascading to descendants.
	// The removeTransactionUnsafe function will clean up edges from
	// children.
	return g.removeTransactionUnsafe(hash)
}

// collectDescendantsToRemove collects all descendants including the node
// itself. Must be called with lock held. Returns a slice of hashes in
// topological order (parents before children).
func (g *TxGraph) collectDescendantsToRemove(
	hash chainhash.Hash,
) []chainhash.Hash {
	node, exists := g.nodes[hash]
	if !exists {
		// Node doesn't exist, return empty slice.
		return nil
	}

	result := []chainhash.Hash{hash}

	for childHash := range node.Children {
		childDescendants := g.collectDescendantsToRemove(childHash)
		result = append(result, childDescendants...)
	}

	return result
}

// removeTransactionUnsafe removes a single transaction without recursion. Must
// be called with lock held.
func (g *TxGraph) removeTransactionUnsafe(hash chainhash.Hash) error {
	node, exists := g.nodes[hash]
	if !exists {
		return ErrNodeNotFound
	}

	// Remove edges from parents.
	for parentHash, parent := range node.Parents {
		delete(parent.Children, hash)

		atomic.AddInt32(&g.metrics.edgeCount, -1)

		// Remove from spent index.
		for _, txIn := range node.Tx.MsgTx().TxIn {
			if txIn.PreviousOutPoint.Hash == parentHash {
				delete(g.indexes.spentBy, txIn.PreviousOutPoint)
			}
		}
	}

	// Remove edges from children.
	for _, child := range node.Children {
		delete(child.Parents, hash)

		atomic.AddInt32(&g.metrics.edgeCount, -1)
	}

	// Remove from feature indexes.
	if node.Metadata.IsTRUC {
		delete(g.indexes.trucTxs, hash)

		atomic.AddInt32(&g.metrics.trucCount, -1)
	}
	if node.Metadata.IsEphemeral {
		delete(g.indexes.ephemeralTxs, hash)

		atomic.AddInt32(&g.metrics.ephemeralCount, -1)
	}

	// Remove from cluster.
	if clusterID, exists := g.indexes.nodeToCluster[hash]; exists {
		if cluster, exists := g.indexes.clusters[clusterID]; exists {
			delete(cluster.Nodes, hash)
			if len(cluster.Nodes) == 0 {
				delete(g.indexes.clusters, clusterID)
				atomic.AddInt32(&g.metrics.clusterCount, -1)
			}
		}

		delete(g.indexes.nodeToCluster, hash)
	}

	// Remove from packages.
	if pkgID := node.Metadata.PackageID; pkgID != nil {
		if pkg, exists := g.indexes.packages[*pkgID]; exists {
			delete(pkg.Transactions, hash)

			if len(pkg.Transactions) == 0 {
				delete(g.indexes.packages, *pkgID)

				atomic.AddInt32(&g.metrics.packageCount, -1)
			}
		}

		delete(g.indexes.nodeToPackage, hash)
	}

	// Remove node.
	delete(g.nodes, hash)

	atomic.AddInt32(&g.metrics.nodeCount, -1)

	return nil
}

// GetNode retrieves a node from the graph.
func (g *TxGraph) GetNode(hash chainhash.Hash) (*TxGraphNode, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	node, exists := g.nodes[hash]
	return node, exists
}

// HasTransaction checks if a transaction exists in the graph.
func (g *TxGraph) HasTransaction(hash chainhash.Hash) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	_, exists := g.nodes[hash]
	return exists
}

// GetAncestors returns all ancestors of a transaction up to maxDepth.
func (g *TxGraph) GetAncestors(hash chainhash.Hash,
	maxDepth int) map[chainhash.Hash]*TxGraphNode {

	g.mu.RLock()
	defer g.mu.RUnlock()

	node, exists := g.nodes[hash]
	if !exists {
		return nil
	}

	ancestors := make(map[chainhash.Hash]*TxGraphNode)
	visited := make(map[chainhash.Hash]bool)
	g.collectAncestorsRecursive(node, ancestors, visited, 0, maxDepth)

	return ancestors
}

// collectAncestorsRecursive recursively collects ancestors.
func (g *TxGraph) collectAncestorsRecursive(
	node *TxGraphNode, ancestors map[chainhash.Hash]*TxGraphNode,
	visited map[chainhash.Hash]bool, currentDepth, maxDepth int) {
	if maxDepth >= 0 && currentDepth >= maxDepth {
		return
	}

	for hash, parent := range node.Parents {
		if visited[hash] {
			continue
		}

		visited[hash] = true
		ancestors[hash] = parent

		g.collectAncestorsRecursive(
			parent, ancestors, visited, currentDepth+1, maxDepth,
		)
	}
}

// GetDescendants returns all descendants of a transaction up to maxDepth.
func (g *TxGraph) GetDescendants(hash chainhash.Hash,
	maxDepth int) map[chainhash.Hash]*TxGraphNode {

	g.mu.RLock()
	defer g.mu.RUnlock()

	node, exists := g.nodes[hash]
	if !exists {
		return nil
	}

	descendants := make(map[chainhash.Hash]*TxGraphNode)
	visited := make(map[chainhash.Hash]bool)

	g.collectDescendantsRecursive(node, descendants, visited, 0, maxDepth)

	return descendants
}

// collectDescendantsRecursive recursively collects descendants.
func (g *TxGraph) collectDescendantsRecursive(
	node *TxGraphNode,
	descendants map[chainhash.Hash]*TxGraphNode,
	visited map[chainhash.Hash]bool,
	currentDepth, maxDepth int,
) {
	if maxDepth >= 0 && currentDepth >= maxDepth {
		return
	}

	for hash, child := range node.Children {
		if visited[hash] {
			continue
		}
		visited[hash] = true
		descendants[hash] = child

		g.collectDescendantsRecursive(
			child, descendants, visited, currentDepth+1, maxDepth,
		)
	}
}

// GetCluster returns the cluster containing the specified transaction.
func (g *TxGraph) GetCluster(hash chainhash.Hash) (*TxCluster, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	clusterID, exists := g.indexes.nodeToCluster[hash]
	if !exists {
		return nil, ErrNodeNotFound
	}

	cluster, exists := g.indexes.clusters[clusterID]
	if !exists {
		return nil, fmt.Errorf("cluster %d not found", clusterID)
	}

	return cluster, nil
}

// GetPackage returns the package containing the specified transaction.
func (g *TxGraph) GetPackage(hash chainhash.Hash) (*TxPackage, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	pkgID, exists := g.indexes.nodeToPackage[hash]
	if !exists {
		return nil, ErrNodeNotFound
	}

	pkg, exists := g.indexes.packages[pkgID]
	if !exists {
		return nil, fmt.Errorf("package not found")
	}

	return pkg, nil
}

// GetOrphans returns all orphan transactions as a slice.
// See IterateOrphans for the definition of an orphan transaction.
func (g *TxGraph) GetOrphans(isConfirmed InputConfirmedPredicate,
) []*TxGraphNode {

	return slices.Collect(g.IterateOrphans(isConfirmed))
}

// GetConflicts returns all transactions and packages that conflict with the
// given transaction. A conflict occurs when the input transaction attempts to
// spend an output that is already spent by a transaction in the mempool.
//
// The method uses the spentBy index for O(1) conflict detection per input.
// For each conflicting transaction found, it includes all descendants since
// they would become invalid if their ancestor is replaced. It also identifies
// any packages that contain conflicting transactions.
//
// The returned ConflictSet provides both individual transactions (for
// fine-grained analysis) and packages (for package-based eviction policies).
//
// Returns an empty ConflictSet if there are no conflicts.
func (g *TxGraph) GetConflicts(tx *btcutil.Tx) *ConflictSet {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := &ConflictSet{
		Transactions: make(map[chainhash.Hash]*TxGraphNode),
		Packages:     make(map[PackageID]*TxPackage),
	}

	// Check each input of the candidate transaction for conflicts with
	// existing mempool transactions. The spentBy index maps each spent
	// output to the transaction that spends it, enabling O(1) lookups.
	for _, txIn := range tx.MsgTx().TxIn {
		// If this outpoint is already spent by a mempool transaction,
		// that transaction conflicts with our candidate.
		conflictNode, exists := g.indexes.spentBy[txIn.PreviousOutPoint]
		if !exists {
			continue
		}

		// Add the directly conflicting transaction.
		result.Transactions[conflictNode.TxHash] = conflictNode

		// Add all descendants of the conflict. When we replace a
		// transaction via RBF, all its descendants must also be
		// removed because they spend outputs that will no longer
		// exist.
		descendants := g.GetDescendants(conflictNode.TxHash, -1)
		for hash, node := range descendants {
			result.Transactions[hash] = node
		}
	}

	// Identify packages that contain any conflicting transactions. This
	// enables package-based eviction where entire packages are treated as
	// atomic units.
	for hash := range result.Transactions {
		pkgID, exists := g.indexes.nodeToPackage[hash]
		if !exists {
			continue
		}

		// Only add each package once even if multiple transactions
		// from the same package are in the conflict set.
		if _, alreadyAdded := result.Packages[pkgID]; alreadyAdded {
			continue
		}

		pkg, exists := g.indexes.packages[pkgID]
		if exists {
			result.Packages[pkgID] = pkg
		}
	}

	return result
}

// ValidatePackage validates a transaction package.
func (g *TxGraph) ValidatePackage(pkg *TxPackage) error {
	if pkg == nil {
		return fmt.Errorf("nil package")
	}

	// Check empty package.
	if len(pkg.Transactions) == 0 {
		return fmt.Errorf("empty package")
	}

	// Check size limits.
	if len(pkg.Transactions) > g.config.MaxPackageSize {
		return fmt.Errorf("package too large: %d transactions",
			len(pkg.Transactions))
	}

	// Validate topology consistency.
	if pkg.Topology.MaxDepth > len(pkg.Transactions)-1 {
		return fmt.Errorf("invalid topology: max depth %d "+
			"exceeds possible depth for %d transactions",
			pkg.Topology.MaxDepth, len(pkg.Transactions))
	}

	// Validate topology based on type.
	switch pkg.Type {
	case PackageType1P1C:
		if len(pkg.Transactions) != 2 {
			return ErrInvalidTopology
		}
		// TODO: Validate 1 parent, 1 child relationship.

	case PackageTypeTRUC:
		// All transactions must be version 3.
		for _, node := range pkg.Transactions {
			if !node.Metadata.IsTRUC {
				return fmt.Errorf("non-TRUC " +
					"transaction in TRUC package")
			}
		}
	}

	return nil
}

// GetMetrics returns current graph metrics.
func (g *TxGraph) GetMetrics() GraphMetrics {
	return GraphMetrics{
		NodeCount:    int(atomic.LoadInt32(&g.metrics.nodeCount)),
		EdgeCount:    int(atomic.LoadInt32(&g.metrics.edgeCount)),
		PackageCount: int(atomic.LoadInt32(&g.metrics.packageCount)),
		TRUCCount:    int(atomic.LoadInt32(&g.metrics.trucCount)),
		EphemeralCount: int(atomic.LoadInt32(
			&g.metrics.ephemeralCount,
		)),
		ClusterCount: int(atomic.LoadInt32(&g.metrics.clusterCount)),
	}
}

// GetNodeCount returns the number of nodes in the graph.
func (g *TxGraph) GetNodeCount() int {
	return int(atomic.LoadInt32(&g.metrics.nodeCount))
}

// GetClusterCount returns the number of clusters in the graph.
func (g *TxGraph) GetClusterCount() int {
	return int(atomic.LoadInt32(&g.metrics.clusterCount))
}

// wouldCreateCycle checks if adding an edge would create a cycle in the DAG.
func (g *TxGraph) wouldCreateCycle(parent, child *TxGraphNode) bool {
	// Cycle detection uses the fundamental DAG property: adding edge
	// parent→child creates a cycle if and only if there's already a path
	// child→parent. We check this by attempting to reach parent from child
	// through existing edges. The graph is a directed acyclic graph (DAG)
	// where edges point from parent to child (spending relationship), so
	// any path child→parent would violate the acyclic property when
	// combined with the new parent→child edge.
	visited := make(map[chainhash.Hash]bool)
	return g.isReachable(child, parent.TxHash, visited)
}

// isReachable performs depth-first search to check if target is reachable from
// source via children edges.
func (g *TxGraph) isReachable(source *TxGraphNode, target chainhash.Hash,
	visited map[chainhash.Hash]bool) bool {

	if source.TxHash == target {
		return true
	}

	// Visited map prevents infinite loops and improves performance by
	// avoiding redundant traversal of already-explored subgraphs.
	if visited[source.TxHash] {
		return false
	}
	visited[source.TxHash] = true

	// Recursively traverse children edges. In transaction graph terms,
	// this follows spending relationships forward (parent→child direction).
	for _, child := range source.Children {
		if g.isReachable(child, target, visited) {
			return true
		}
	}

	return false
}

// updateClusterAssignment updates the cluster assignment for a node.
func (g *TxGraph) updateClusterAssignment(node *TxGraphNode) {
	// Find clusters of connected nodes.
	parentClusters := make(map[ClusterID]bool)
	childClusters := make(map[ClusterID]bool)

	for _, parent := range node.Parents {
		if cid, exists := g.indexes.nodeToCluster[parent.TxHash]; exists {
			parentClusters[cid] = true
		}
	}

	for _, child := range node.Children {
		if cid, exists := g.indexes.nodeToCluster[child.TxHash]; exists {
			childClusters[cid] = true
		}
	}

	// Merge all related clusters.
	allClusters := make(map[ClusterID]bool)
	for cid := range parentClusters {
		allClusters[cid] = true
	}
	for cid := range childClusters {
		allClusters[cid] = true
	}

	if len(allClusters) == 0 {
		// Create new cluster.
		g.createNewCluster(node)
	} else if len(allClusters) == 1 {

		// Add to existing cluster.
		for cid := range allClusters {
			g.addToCluster(node, cid)
		}

	} else {
		// Merge multiple clusters.
		g.mergeClusters(node, allClusters)
	}
}

// createNewCluster creates a new cluster for a node.
func (g *TxGraph) createNewCluster(node *TxGraphNode) {
	clusterID := ClusterID(g.nextClusterID.Add(1))

	cluster := &TxCluster{
		ID:    clusterID,
		Nodes: make(map[chainhash.Hash]*TxGraphNode),
		Size:  1,
	}

	cluster.Nodes[node.TxHash] = node

	g.indexes.clusters[clusterID] = cluster
	g.indexes.nodeToCluster[node.TxHash] = clusterID
	node.Metadata.ClusterID = clusterID

	atomic.AddInt32(&g.metrics.clusterCount, 1)
}

// addToCluster adds a node to an existing cluster.
func (g *TxGraph) addToCluster(node *TxGraphNode, clusterID ClusterID) {
	cluster, exists := g.indexes.clusters[clusterID]
	if !exists {
		g.createNewCluster(node)
		return
	}

	cluster.Nodes[node.TxHash] = node
	cluster.Size++

	g.indexes.nodeToCluster[node.TxHash] = clusterID

	node.Metadata.ClusterID = clusterID
}

// mergeClusters merges multiple clusters into one.
func (g *TxGraph) mergeClusters(
	node *TxGraphNode, clusterIDs map[ClusterID]bool,
) {
	// Use the lowest cluster ID as the target to maintain deterministic
	// behavior and stable cluster identification across runs. This choice
	// is arbitrary but consistent: we could choose any cluster as the
	// merge target, but always picking the lowest ID ensures that cluster
	// IDs don't change unnecessarily during graph operations, which
	// simplifies debugging and testing.
	var targetID ClusterID
	first := true
	for cid := range clusterIDs {
		if first || cid < targetID {
			targetID = cid
			first = false
		}
	}

	targetCluster := g.indexes.clusters[targetID]
	if targetCluster == nil {
		g.createNewCluster(node)
		return
	}

	// Add the new node.
	targetCluster.Nodes[node.TxHash] = node

	g.indexes.nodeToCluster[node.TxHash] = targetID
	node.Metadata.ClusterID = targetID

	// Merge other clusters into target.
	for cid := range clusterIDs {
		if cid == targetID {
			continue
		}

		if cluster, exists := g.indexes.clusters[cid]; exists {
			for hash, n := range cluster.Nodes {
				targetCluster.Nodes[hash] = n
				g.indexes.nodeToCluster[hash] = targetID
				n.Metadata.ClusterID = targetID
			}

			delete(g.indexes.clusters, cid)

			atomic.AddInt32(&g.metrics.clusterCount, -1)
		}
	}

	targetCluster.Size = len(targetCluster.Nodes)
}

// mergeNodeClusters merges clusters when adding an edge.
func (g *TxGraph) mergeNodeClusters(parent, child *TxGraphNode) {
	parentCluster := g.indexes.nodeToCluster[parent.TxHash]
	childCluster := g.indexes.nodeToCluster[child.TxHash]

	if parentCluster == childCluster {
		return
	}

	clusters := make(map[ClusterID]bool)
	clusters[parentCluster] = true
	clusters[childCluster] = true

	g.mergeClusters(parent, clusters)
}

