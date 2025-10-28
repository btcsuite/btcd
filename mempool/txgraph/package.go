package txgraph

import (
	"bytes"
	"crypto/sha256"
	"sort"
	"sync/atomic"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// PackageOption is a functional option for configuring CreatePackage behavior.
type PackageOption func(*packageOptions)

// packageOptions holds configuration for package creation.
type packageOptions struct {
	dryRun bool
}

// WithDryRun configures CreatePackage to skip package tracking. This is used
// during validation-only scenarios where we want to check package policy
// without adding the package to persistent indexes. The dry run mode is
// essential during pre-acceptance validation to avoid tracking packages for
// transactions that haven't been added to the graph yet.
func WithDryRun() PackageOption {
	return func(opts *packageOptions) {
		opts.dryRun = true
	}
}

// IdentifyPackages scans the graph to detect and classify transaction
// packages according to their type (1P1C, TRUC, ephemeral, standard).
// Package classification enables the relay system to apply appropriate
// validation rules based on BIP 431 (TRUC) and ephemeral dust policies.
// We prioritize more specific package types (TRUC, ephemeral) before
// checking general types (1P1C, standard) because stricter validation
// rules must be enforced when applicable to ensure consensus
// compatibility.
func (g *TxGraph) IdentifyPackages() ([]*TxPackage, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	packages := make([]*TxPackage, 0)
	processed := make(map[chainhash.Hash]bool)

	for hash, node := range g.nodes {
		if processed[hash] {
			continue
		}

		// Package roots have no unconfirmed parents, making them the
		// starting point for package formation. Non-roots are
		// processed as children of their parent packages.
		if !g.isPackageRoot(node) {
			continue
		}

		// Try to form different package types in priority order.
		// Specific types like TRUC and ephemeral have stricter
		// topology and fee requirements that override the more
		// permissive standard package rules.
		//
		// TODO(rosabeef): revisit
		if pkg := g.tryTRUCPackage(node); pkg != nil {
			packages = append(packages, pkg)
			g.markPackageProcessed(pkg, processed)

		} else if pkg := g.tryEphemeralPackage(node); pkg != nil {
			packages = append(packages, pkg)
			g.markPackageProcessed(pkg, processed)

		} else if pkg := g.try1P1CPackage(node); pkg != nil {
			packages = append(packages, pkg)
			g.markPackageProcessed(pkg, processed)

		} else if pkg := g.tryStandardPackage(node); pkg != nil {

			packages = append(packages, pkg)
			g.markPackageProcessed(pkg, processed)
		}
	}

	return packages, nil
}

// isPackageRoot determines whether a node serves as the starting point
// for package formation. Roots are chosen as transactions with no
// unconfirmed parents because this ensures we process packages from their
// topological origin, allowing proper parent-child relationship
// validation and preventing double-counting of descendants.
func (g *TxGraph) isPackageRoot(node *TxGraphNode) bool {
	// Only nodes without unconfirmed parents qualify as package roots.
	// Nodes with parents will be included when their parent package is
	// formed.
	return len(node.Parents) == 0
}

// try1P1CPackage attempts to form a 1-parent-1-child package for CPFP
// fee bumping. This package type is preferred in relay policy because it
// allows efficient validation and ensures predictable topology. The
// strict 1P1C relationship guarantees that fee calculations are
// unambiguous and that the package cannot be fragmented during relay.
func (g *TxGraph) try1P1CPackage(node *TxGraphNode) *TxPackage {
	// Require exactly one child to maintain the 1P1C topology invariant.
	if len(node.Children) != 1 {
		return nil
	}

	// Extract the single child from the map.
	var child *TxGraphNode
	for _, c := range node.Children {
		child = c
		break
	}

	// Verify the child has only this parent in the mempool. Multiple
	// unconfirmed parents would violate 1P1C topology and complicate
	// fee calculation for package relay.
	unconfirmedParents := 0
	for _, parent := range child.Parents {
		if _, exists := g.nodes[parent.TxHash]; exists {
			unconfirmedParents++
		}
	}

	if unconfirmedParents != 1 {
		return nil
	}

	// Construct the package with the parent as root to establish proper
	// topological ordering for relay and mining.
	pkg := &TxPackage{
		Type:         PackageType1P1C,
		Transactions: make(map[chainhash.Hash]*TxGraphNode),
		Root:         node,
	}

	pkg.Transactions[node.TxHash] = node
	pkg.Transactions[child.TxHash] = child

	// Compute aggregate package metrics for fee rate evaluation. Package
	// fee rate determines mining priority and relay acceptance.
	pkg.TotalFees = node.TxDesc.Fee + child.TxDesc.Fee
	pkg.TotalSize = node.TxDesc.VirtualSize + child.TxDesc.VirtualSize
	if pkg.TotalSize > 0 {
		pkg.FeeRate = pkg.TotalFees * 1000 / pkg.TotalSize
	}

	// Generate deterministic package ID for deduplication and indexing.
	pkg.ID = g.generatePackageID(pkg)

	// Record topology characteristics for validation and optimization.
	// 1P1C packages always form a simple linear chain.
	pkg.Topology = PackageTopology{
		MaxDepth:   1,
		MaxWidth:   1,
		TotalNodes: 2,
		IsLinear:   true,
		IsTree:     true,
	}

	return pkg
}

// tryTRUCPackage attempts to form a TRUC (v3 transaction) package
// according to BIP 431. TRUC transactions use version 3 to signal opt-in
// topology restrictions that enable more efficient package relay and RBF.
// The restricted topology (max 1 parent, 1 child) prevents pinning
// attacks while maintaining CPFP fee bumping capability.
func (g *TxGraph) tryTRUCPackage(node *TxGraphNode) *TxPackage {
	// Require analyzer to be configured for TRUC validation. Without
	// the analyzer, we cannot enforce BIP 431 rules.
	if g.analyzer == nil {
		return nil
	}

	// Verify this is a version 3 transaction signaling TRUC opt-in.
	if !g.analyzer.IsTRUCTransaction(node.Tx.MsgTx()) {
		return nil
	}

	// Enforce BIP 431 topology restriction: TRUC transactions cannot
	// have multiple children as this would enable pinning vectors.
	if len(node.Children) > 1 {
		return nil
	}

	pkg := &TxPackage{
		Type:         PackageTypeTRUC,
		Transactions: make(map[chainhash.Hash]*TxGraphNode),
		Root:         node,
	}

	// Add the root TRUC transaction to the package.
	pkg.Transactions[node.TxHash] = node
	totalFees := node.TxDesc.Fee
	totalSize := node.TxDesc.VirtualSize
	nodeCount := 1
	maxDepth := 0

	// Include the TRUC child if present, maintaining the restricted
	// topology required by BIP 431.
	if len(node.Children) == 1 {
		var child *TxGraphNode
		for _, c := range node.Children {
			child = c
			break
		}

		// Both parent and child must be v3 to form a valid TRUC
		// package. Mixing v3 and non-v3 transactions would violate
		// BIP 431 and create relay policy ambiguity.
		if !g.analyzer.IsTRUCTransaction(child.Tx.MsgTx()) {
			return nil
		}

		pkg.Transactions[child.TxHash] = child
		totalFees += child.TxDesc.Fee
		totalSize += child.TxDesc.VirtualSize
		nodeCount++
		maxDepth = 1
	}

	pkg.TotalFees = totalFees
	pkg.TotalSize = totalSize
	if pkg.TotalSize > 0 {
		pkg.FeeRate = pkg.TotalFees * 1000 / pkg.TotalSize
	}

	// Generate deterministic package ID for indexing and deduplication.
	pkg.ID = g.generatePackageID(pkg)

	// Record topology metadata. TRUC packages are always linear
	// chains due to the 1-parent-1-child restriction enforced above.
	pkg.Topology = PackageTopology{
		MaxDepth:   maxDepth,
		MaxWidth:   1,
		TotalNodes: nodeCount,
		IsLinear:   true,
		IsTree:     true,
	}

	return pkg
}

// tryEphemeralPackage attempts to form an ephemeral dust package.
// These packages allow zero-fee parent transactions with dust outputs
// (typically P2A anchors) that must be spent by a child transaction
// within the same package. This pattern enables efficient fee bumping
// for commitment transactions while preventing dust accumulation on the
// UTXO set.
func (g *TxGraph) tryEphemeralPackage(node *TxGraphNode) *TxPackage {
	// Require analyzer for ephemeral dust detection and validation.
	if g.analyzer == nil {
		return nil
	}

	// Verify the parent contains ephemeral dust outputs that must be
	// spent within the package to prevent UTXO set pollution.
	if !g.analyzer.HasEphemeralDust(node.Tx.MsgTx()) {
		return nil
	}

	// Ephemeral parents typically have zero fee since they rely on
	// CPFP for mining incentive. Non-zero fee would be economically
	// wasteful.
	if !g.analyzer.IsZeroFee(node.TxDesc) {
		return nil
	}

	// Require at least one child to spend the dust output. Unspent
	// dust violates the ephemeral package contract and would persist
	// in the UTXO set.
	if len(node.Children) == 0 {
		return nil
	}

	pkg := &TxPackage{
		Type:         PackageTypeEphemeral,
		Transactions: make(map[chainhash.Hash]*TxGraphNode),
		Root:         node,
	}

	// Add the parent transaction containing ephemeral dust outputs.
	pkg.Transactions[node.TxHash] = node
	totalFees := node.TxDesc.Fee
	totalSize := node.TxDesc.VirtualSize

	// Include all children that spend the ephemeral outputs. Multiple
	// children may be present if the dust spending is batched with
	// other operations.
	for _, child := range node.Children {
		pkg.Transactions[child.TxHash] = child
		totalFees += child.TxDesc.Fee
		totalSize += child.TxDesc.VirtualSize
	}

	pkg.TotalFees = totalFees
	pkg.TotalSize = totalSize
	if pkg.TotalSize > 0 {
		pkg.FeeRate = pkg.TotalFees * 1000 / pkg.TotalSize
	}

	// Generate deterministic package ID for tracking and deduplication.
	pkg.ID = g.generatePackageID(pkg)

	// Compute topology metrics for validation. Ephemeral packages
	// may have multiple children unlike TRUC packages.
	pkg.Topology = g.calculateTopology(pkg)

	return pkg
}

// tryStandardPackage attempts to form a standard package from connected
// transactions. Standard packages serve as the fallback for transactions
// that don't qualify for specialized types. We use BFS traversal to
// discover the full connected component up to the maximum package size,
// enabling package relay for arbitrary transaction topologies.
func (g *TxGraph) tryStandardPackage(node *TxGraphNode) *TxPackage {
	// Use breadth-first search to explore the connected component.
	// BFS ensures we discover transactions in topological order and
	// can efficiently limit package size.
	visited := make(map[chainhash.Hash]bool)
	queue := []*TxGraphNode{node}

	pkg := &TxPackage{
		Type:         PackageTypeStandard,
		Transactions: make(map[chainhash.Hash]*TxGraphNode),
		Root:         node,
	}

	totalFees := int64(0)
	totalSize := int64(0)

	for len(queue) > 0 &&
		len(pkg.Transactions) < g.config.MaxPackageSize {

		current := queue[0]
		queue = queue[1:]

		if visited[current.TxHash] {
			continue
		}
		visited[current.TxHash] = true

		// Include this transaction in the package and update
		// metrics.
		pkg.Transactions[current.TxHash] = current
		totalFees += current.TxDesc.Fee
		totalSize += current.TxDesc.VirtualSize

		// Traverse descendants to include child transactions in the
		// package for CPFP consideration.
		for _, child := range current.Children {
			if !visited[child.TxHash] {
				queue = append(queue, child)
			}
		}

		// Also traverse ancestors to capture the full connected
		// component. This ensures we don't fragment packages that
		// have complex dependency relationships.
		for _, parent := range current.Parents {
			if !visited[parent.TxHash] {
				queue = append(queue, parent)
			}
		}
	}

	// Single-transaction packages provide no relay benefit. Package
	// formation is only useful when grouping related transactions for
	// aggregate fee calculation.
	if len(pkg.Transactions) <= 1 {
		return nil
	}

	pkg.TotalFees = totalFees
	pkg.TotalSize = totalSize
	if pkg.TotalSize > 0 {
		pkg.FeeRate = pkg.TotalFees * 1000 / pkg.TotalSize
	}

	// Generate deterministic package ID based on constituent
	// transactions.
	pkg.ID = g.generatePackageID(pkg)

	// Compute topology metrics for validation and optimization
	// decisions.
	pkg.Topology = g.calculateTopology(pkg)

	return pkg
}

// generatePackageID generates a deterministic unique identifier for a
// package by hashing its constituent transaction IDs. Determinism is
// essential to enable package deduplication across peers and to provide
// stable references for package tracking during relay. Lexicographic
// sorting ensures the same set of transactions always produces the same
// ID regardless of discovery order.
func (g *TxGraph) generatePackageID(pkg *TxPackage) PackageID {
	// Collect all transaction hashes from the package.
	hashes := make([]chainhash.Hash, 0, len(pkg.Transactions))
	for hash := range pkg.Transactions {
		hashes = append(hashes, hash)
	}

	// Sort lexicographically to ensure deterministic ID generation.
	// Without sorting, the iteration order of the map would produce
	// non-deterministic results.
	sort.Slice(hashes, func(i, j int) bool {
		return bytes.Compare(hashes[i][:], hashes[j][:]) < 0
	})

	// Concatenate sorted hashes and compute SHA256 to create a
	// compact unique identifier.
	var buf bytes.Buffer
	for _, hash := range hashes {
		buf.Write(hash[:])
	}

	packageHash := sha256.Sum256(buf.Bytes())

	return PackageID{
		Hash: chainhash.Hash(packageHash),
		Type: pkg.Type,
	}
}

// calculateTopology analyzes the structure of a package to compute its
// topological properties. These metrics inform validation decisions
// (depth limits), mining optimization (linear chains are simpler), and
// relay policy (tree structures avoid diamond dependencies). We use BFS
// from the root to measure depth and width at each level.
func (g *TxGraph) calculateTopology(pkg *TxPackage) PackageTopology {
	topology := PackageTopology{
		TotalNodes: len(pkg.Transactions),
	}

	// Without a root, we cannot traverse the package to compute depth
	// and width metrics. Return early with minimal topology info.
	if pkg.Root == nil {
		return topology
	}

	// Build package-local Children map by examining Parents of all
	// nodes in the package. This ensures temporary nodes (not yet
	// added to the graph) are included in topology calculation.
	// This is critical for validating TRUC depth constraints when
	// the package includes a new transaction not yet in the graph.
	pkgChildren := make(map[chainhash.Hash]map[chainhash.Hash]*TxGraphNode)
	for _, node := range pkg.Transactions {
		for parentHash := range node.Parents {
			if pkgChildren[parentHash] == nil {
				pkgChildren[parentHash] = make(map[chainhash.Hash]*TxGraphNode)
			}
			pkgChildren[parentHash][node.TxHash] = node
		}
	}

	visited := make(map[chainhash.Hash]int)

	// Use proper Queue data structure for BFS traversal.
	type queueItem struct {
		node  *TxGraphNode
		depth int
	}
	queue := NewQueue[queueItem]()
	queue.Enqueue(queueItem{pkg.Root, 0})

	maxDepth := 0
	widthByDepth := make(map[int]int)

	for !queue.IsEmpty() {
		current, _ := queue.Dequeue()

		if _, exists := visited[current.node.TxHash]; exists {
			continue
		}
		visited[current.node.TxHash] = current.depth

		widthByDepth[current.depth]++
		if current.depth > maxDepth {
			maxDepth = current.depth
		}

		// Explore children using package-local Children map. This
		// includes temporary nodes not yet added to the graph,
		// ensuring correct depth calculation during validation.
		for _, child := range pkgChildren[current.node.TxHash] {
			_, exists := pkg.Transactions[child.TxHash]
			if exists {
				if _, visited := visited[child.TxHash]; !visited {
					queue.Enqueue(queueItem{child, current.depth + 1})
				}
			}
		}
	}

	topology.MaxDepth = maxDepth

	// Determine maximum width (most siblings at any depth
	// level). High width indicates parallel transaction
	// structure.
	for _, width := range widthByDepth {
		if width > topology.MaxWidth {
			topology.MaxWidth = width
		}
	}

	// Check if package forms a linear chain. Linear chains are
	// simpler to validate and optimize for block template
	// construction.
	topology.IsLinear = true
	for _, width := range widthByDepth {
		if width > 1 {
			topology.IsLinear = false
			break
		}
	}

	// Verify tree structure (no diamond dependencies). Tree
	// topology simplifies relay and prevents pinning attacks.
	topology.IsTree = g.isPackageTree(pkg)

	return topology
}

// isPackageTree verifies that a package forms a tree structure without
// diamond patterns (nodes with multiple parents). Tree topology is
// desirable because it prevents dependency cycles and simplifies relay
// logic. Diamond patterns create ambiguity in fee assignment and can
// enable pinning attacks where an attacker creates complex dependencies
// to prevent package relay.
func (g *TxGraph) isPackageTree(pkg *TxPackage) bool {
	// Scan all nodes to detect any with multiple parents within the
	// package. Multiple parents indicate a diamond pattern where
	// dependencies merge.
	for _, node := range pkg.Transactions {
		parentsInPackage := 0
		for parentHash := range node.Parents {
			if _, exists := pkg.Transactions[parentHash]; exists {
				parentsInPackage++
				if parentsInPackage > 1 {
					return false
				}
			}
		}
	}
	return true
}

// markPackageProcessed records that a package has been formed and
// indexes all its transactions. This prevents duplicate package formation
// when a transaction could belong to multiple overlapping packages,
// ensuring each transaction is counted in exactly one package. The
// indexes enable efficient lookup of packages by transaction ID during
// relay and validation.
func (g *TxGraph) markPackageProcessed(
	pkg *TxPackage, processed map[chainhash.Hash]bool) {

	for hash := range pkg.Transactions {
		processed[hash] = true

		// Map each transaction to its package for reverse lookup
		// during validation and relay operations.
		g.indexes.nodeToPackage[hash] = pkg.ID
	}

	// Store package in the global index to enable iteration over
	// all packages for mempool queries and eviction.
	g.indexes.packages[pkg.ID] = pkg

	// Increment package count for monitoring and metrics collection.
	atomic.AddInt32(&g.metrics.packageCount, 1)
}

// isPackageConnected verifies that all nodes in a package form a
// single connected component. Package relay requires connectivity because
// disconnected transactions cannot provide CPFP benefits to each other
// and should be treated as separate packages. We use BFS to test
// reachability from an arbitrary starting node to all other nodes.
func (g *TxGraph) isPackageConnected(nodes []*TxGraphNode) bool {
	if len(nodes) <= 1 {
		return true
	}

	// Build lookup table for O(1) membership testing during
	// traversal.
	nodeSet := make(map[chainhash.Hash]bool)
	for _, node := range nodes {
		nodeSet[node.TxHash] = true
	}

	// Use BFS from an arbitrary start node to test reachability. If
	// all nodes are reachable, the graph is connected.
	visited := make(map[chainhash.Hash]bool)
	queue := []*TxGraphNode{nodes[0]}
	visited[nodes[0].TxHash] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Traverse parent edges to find ancestors in the package.
		for _, parent := range current.Parents {
			if nodeSet[parent.TxHash] && !visited[parent.TxHash] {
				visited[parent.TxHash] = true
				queue = append(queue, parent)
			}
		}

		// Traverse child edges to find descendants in the package.
		for _, child := range current.Children {
			if nodeSet[child.TxHash] && !visited[child.TxHash] {
				visited[child.TxHash] = true
				queue = append(queue, child)
			}
		}
	}

	// If we visited all nodes, the graph is connected. Unvisited
	// nodes indicate disconnected components.
	return len(visited) == len(nodes)
}

// CreatePackage constructs a TxPackage from a specified set of graph
// nodes, providing explicit control over package formation. This is used
// during relay validation when incoming packages specify their
// membership, or during RBF evaluation when comparing conflicting package
// groups. The function enforces connectivity requirements, computes
// topology metrics, and classifies package type based on BIP 431 and
// ephemeral dust rules to ensure appropriate relay policies are applied.
//
// By default, successfully validated packages are tracked in the graph's
// package indexes. Use WithDryRun() to skip tracking during validation-only
// scenarios (e.g., pre-acceptance checks before AddTransaction).
func (g *TxGraph) CreatePackage(nodes []*TxGraphNode, opts ...PackageOption) (*TxPackage, error) {
	// Apply functional options.
	options := &packageOptions{}
	for _, opt := range opts {
		opt(options)
	}
	if len(nodes) == 0 {
		return nil, ErrInvalidTopology
	}

	if len(nodes) > g.config.MaxPackageSize {
		return nil, ErrInvalidTopology
	}

	pkg := &TxPackage{
		Type:         PackageTypeStandard,
		Transactions: make(map[chainhash.Hash]*TxGraphNode),
	}

	totalFees := int64(0)
	totalSize := int64(0)

	// Populate the package with all provided nodes and compute
	// aggregate metrics.
	for _, node := range nodes {
		pkg.Transactions[node.TxHash] = node
		totalFees += node.TxDesc.Fee
		totalSize += node.TxDesc.VirtualSize
	}

	// Enforce connectivity requirement for multi-transaction
	// packages. Disconnected transactions cannot provide CPFP
	// benefits and should be separate packages.
	if len(nodes) > 1 {
		if !g.isPackageConnected(nodes) {
			return nil, ErrDisconnectedPackage
		}
	}

	// Identify the root node (no parents within package) to
	// establish topological ordering for validation and relay. The
	// root serves as the starting point for depth-first traversal.
	for _, node := range nodes {
		hasParentInPackage := false
		for parentHash := range node.Parents {
			if _, exists := pkg.Transactions[parentHash]; exists {
				hasParentInPackage = true
				break
			}
		}
		if !hasParentInPackage {
			pkg.Root = node
			break
		}
	}

	// Classify package type (TRUC, ephemeral, 1P1C, standard) to
	// determine which relay policies apply. Use the analyzer if
	// available; otherwise default to standard (already set).
	if g.analyzer != nil {
		pkg.Type = g.analyzer.AnalyzePackageType(nodes)
	}

	// Finalize package metrics for fee rate evaluation and relay
	// decisions.
	pkg.TotalFees = totalFees
	pkg.TotalSize = totalSize
	if pkg.TotalSize > 0 {
		pkg.FeeRate = pkg.TotalFees * 1000 / pkg.TotalSize
	}

	// Generate deterministic package ID for tracking and deduplication.
	pkg.ID = g.generatePackageID(pkg)

	// Compute topology characteristics for policy enforcement.
	pkg.Topology = g.calculateTopology(pkg)

	// Run validation rules appropriate to the package type.
	// Validation may fail if topology constraints are violated or
	// required conditions (e.g., dust spending) are not met.
	if err := g.ValidatePackage(pkg); err != nil {
		return nil, err
	}

	pkg.IsValid = true
	pkg.LastValidated = nodes[0].Metadata.AddedTime

	// Track the package if all nodes are persistent (exist in graph) and
	// this is not a dry run. This ensures we only track packages after
	// AddTransaction succeeds, preventing index inconsistency if validation
	// passes but addition fails. Dry run mode is used during pre-acceptance
	// validation to check policy without modifying indexes.
	if !options.dryRun {
		g.maybeTrackPackage(pkg)
	}

	return pkg, nil
}

// maybeTrackPackage tracks the package only if all nodes exist in the
// graph (no temporary nodes from validation). This prevents tracking
// packages during validation before AddTransaction succeeds.
//
// The package is indexed in two ways:
//   - packages map: PackageID → *TxPackage
//   - nodeToPackage map: TxHash → PackageID (for reverse lookup)
func (g *TxGraph) maybeTrackPackage(pkg *TxPackage) {
	// Check if any node is temporary (not in graph yet).
	for hash := range pkg.Transactions {
		if _, exists := g.nodes[hash]; !exists {
			// Contains temporary node - don't track yet.
			// This happens during validation before AddTransaction.
			return
		}
	}

	// All nodes are persistent - track the package.
	g.indexes.packages[pkg.ID] = pkg
	for hash := range pkg.Transactions {
		g.indexes.nodeToPackage[hash] = pkg.ID
	}
	atomic.AddInt32(&g.metrics.packageCount, 1)
}

