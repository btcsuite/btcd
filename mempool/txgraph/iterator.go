package txgraph

import (
	"iter"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// Iterate returns an iterator over graph nodes.
func (g *TxGraph) Iterate(options ...IterOption) iter.Seq[*TxGraphNode] {
	// Build options with defaults.
	opts := DefaultIteratorOption()
	for _, option := range options {
		option(&opts)
	}

	return func(yield func(*TxGraphNode) bool) {
		g.mu.RLock()
		defer g.mu.RUnlock()

		// Get starting node.
		var startNode *TxGraphNode
		if opts.StartNode != nil {
			startNode = g.nodes[*opts.StartNode]
			if startNode == nil {
				return
			}
		}

		// Select traversal implementation.
		switch opts.Order {
		case TraversalDefault:
			// Default to iterating all nodes.
			for _, node := range g.nodes {
				// Apply filter.
				if opts.Filter != nil && !opts.Filter(node) {
					continue
				}
				if !yield(node) {
					return
				}
			}
		case TraversalDFS:
			g.iterateDFS(startNode, opts, yield)
		case TraversalBFS:
			g.iterateBFS(startNode, opts, yield)
		case TraversalTopological:
			g.iterateTopological(opts, yield)
		case TraversalReverseTopo:
			g.iterateReverseTopological(opts, yield)
		case TraversalAncestors:
			g.iterateAncestors(startNode, opts, yield)
		case TraversalDescendants:
			g.iterateDescendants(startNode, opts, yield)
		case TraversalCluster:
			g.iterateCluster(startNode, opts, yield)
		case TraversalFeeRate:
			g.iterateFeeRate(opts, yield)
		}
	}
}

// iterateDFS performs depth-first traversal.
func (g *TxGraph) iterateDFS(start *TxGraphNode, opts IteratorOption, yield func(*TxGraphNode) bool) {
	visited := make(map[chainhash.Hash]bool)
	stack := NewStack[*TxGraphNode]()
	depth := make(map[chainhash.Hash]int)

	// Initialize with start node or all roots.
	if start != nil {
		if opts.IncludeStart {
			stack.Push(start)
			depth[start.TxHash] = 0
		} else {
			// Mark start node as visited to prevent revisiting.
			visited[start.TxHash] = true
			depth[start.TxHash] = 0
			// Start with children/parents based on direction.
			g.addNeighborsToStack(start, stack, depth, opts.Direction, 1)
		}
	} else {
		// Find all root nodes (no parents).
		for _, node := range g.nodes {
			if len(node.Parents) == 0 {
				stack.Push(node)
				depth[node.TxHash] = 0
			}
		}
	}

	for !stack.IsEmpty() {
		// Pop from stack.
		node, _ := stack.Pop()

		// Skip if already visited.
		if visited[node.TxHash] {
			continue
		}
		visited[node.TxHash] = true

		// Check depth limit.
		if opts.MaxDepth >= 0 && depth[node.TxHash] > opts.MaxDepth {
			continue
		}

		// Yield node to consumer if filter passes.
		if opts.Filter == nil || opts.Filter(node) {
			if !yield(node) {
				return // Consumer wants to stop
			}
		}

		// Always add neighbors to continue traversal.
		nextDepth := depth[node.TxHash] + 1
		g.addNeighborsToStack(node, stack, depth, opts.Direction, nextDepth)
	}
}

// iterateBFS performs breadth-first traversal.
func (g *TxGraph) iterateBFS(start *TxGraphNode, opts IteratorOption, yield func(*TxGraphNode) bool) {
	visited := make(map[chainhash.Hash]bool)
	queue := NewQueue[*TxGraphNode]()
	depth := make(map[chainhash.Hash]int)

	// Initialize with start node or all roots.
	if start != nil {
		if opts.IncludeStart {
			queue.Enqueue(start)
			depth[start.TxHash] = 0
		} else {
			// Mark start node as visited to prevent revisiting.
			visited[start.TxHash] = true
			depth[start.TxHash] = 0
			// Start with children/parents based on direction.
			g.addNeighborsToQueue(start, queue, depth, opts.Direction, 1)
		}
	} else {
		for _, node := range g.nodes {
			if len(node.Parents) == 0 {
				queue.Enqueue(node)
				depth[node.TxHash] = 0
			}
		}
	}

	for !queue.IsEmpty() {
		// Dequeue from front.
		node, _ := queue.Dequeue()

		// Skip if already visited.
		if visited[node.TxHash] {
			continue
		}
		visited[node.TxHash] = true

		// Check depth limit.
		if opts.MaxDepth >= 0 && depth[node.TxHash] > opts.MaxDepth {
			continue
		}

		// Yield to consumer if filter passes.
		if opts.Filter == nil || opts.Filter(node) {
			if !yield(node) {
				return
			}
		}

		// Always add neighbors to continue traversal.
		nextDepth := depth[node.TxHash] + 1
		g.addNeighborsToQueue(node, queue, depth, opts.Direction, nextDepth)
	}
}

// iterateTopological performs topological traversal.
func (g *TxGraph) iterateTopological(opts IteratorOption, yield func(*TxGraphNode) bool) {
	// Calculate in-degrees.
	inDegree := make(map[chainhash.Hash]int)
	for hash, node := range g.nodes {
		inDegree[hash] = len(node.Parents)
	}

	// Find all nodes with no parents.
	queue := NewQueue[*TxGraphNode]()
	for hash, degree := range inDegree {
		if degree == 0 {
			queue.Enqueue(g.nodes[hash])
		}
	}

	// Process in topological order.
	for !queue.IsEmpty() {
		node, _ := queue.Dequeue()

		// Apply filter.
		if opts.Filter != nil && !opts.Filter(node) {
			continue
		}

		// Yield to consumer.
		if !yield(node) {
			return
		}

		// Update in-degrees and queue children.
		for _, child := range node.Children {
			inDegree[child.TxHash]--
			if inDegree[child.TxHash] == 0 {
				queue.Enqueue(child)
			}
		}
	}
}

// iterateReverseTopological performs reverse topological traversal.
func (g *TxGraph) iterateReverseTopological(opts IteratorOption, yield func(*TxGraphNode) bool) {
	// Calculate out-degrees.
	outDegree := make(map[chainhash.Hash]int)
	for hash, node := range g.nodes {
		outDegree[hash] = len(node.Children)
	}

	// Find all nodes with no children.
	queue := NewQueue[*TxGraphNode]()
	for hash, degree := range outDegree {
		if degree == 0 {
			queue.Enqueue(g.nodes[hash])
		}
	}

	// Process in reverse topological order.
	for !queue.IsEmpty() {
		node, _ := queue.Dequeue()

		// Apply filter.
		if opts.Filter != nil && !opts.Filter(node) {
			continue
		}

		// Yield to consumer.
		if !yield(node) {
			return
		}

		// Update out-degrees and queue parents.
		for _, parent := range node.Parents {
			outDegree[parent.TxHash]--
			if outDegree[parent.TxHash] == 0 {
				queue.Enqueue(parent)
			}
		}
	}
}

// iterateAncestors iterates over all ancestors of a node.
func (g *TxGraph) iterateAncestors(start *TxGraphNode, opts IteratorOption, yield func(*TxGraphNode) bool) {
	if start == nil {
		return
	}

	visited := make(map[chainhash.Hash]bool)
	queue := NewQueue[*TxGraphNode]()
	depth := make(map[chainhash.Hash]int)

	// Include start node if requested.
	if opts.IncludeStart {
		queue.Enqueue(start)
		depth[start.TxHash] = 0
	} else {
		// Start with parents only.
		for _, parent := range start.Parents {
			if parent != nil {
				queue.Enqueue(parent)
				depth[parent.TxHash] = 1
			}
		}
	}

	for !queue.IsEmpty() {
		node, _ := queue.Dequeue()

		// Use the hash from the node itself.
		nodeHash := node.TxHash
		if visited[nodeHash] {
			continue
		}
		visited[nodeHash] = true

		// Check depth limit.
		nodeDepth := depth[node.TxHash]
		if opts.MaxDepth >= 0 && nodeDepth > opts.MaxDepth {
			continue
		}

		// Yield to consumer if filter passes.
		if opts.Filter == nil || opts.Filter(node) {
			if !yield(node) {
				return
			}
		}

		// Add parents to continue traversal (depth starts at 1 for parents).
		nextDepth := depth[node.TxHash] + 1
		for _, parent := range node.Parents {
			if !visited[parent.TxHash] {
				queue.Enqueue(parent)
				depth[parent.TxHash] = nextDepth
			}
		}
	}
}

// iterateDescendants iterates over all descendants of a node.
func (g *TxGraph) iterateDescendants(start *TxGraphNode, opts IteratorOption, yield func(*TxGraphNode) bool) {
	if start == nil {
		return
	}

	visited := make(map[chainhash.Hash]bool)
	queue := NewQueue[*TxGraphNode]()
	depth := make(map[chainhash.Hash]int)

	// Include start node if requested.
	if opts.IncludeStart {
		queue.Enqueue(start)
		depth[start.TxHash] = 0
	} else {
		// Start with children only.
		for _, child := range start.Children {
			queue.Enqueue(child)
			depth[child.TxHash] = 1
		}
	}

	for !queue.IsEmpty() {
		node, _ := queue.Dequeue()

		if visited[node.TxHash] {
			continue
		}
		visited[node.TxHash] = true

		// Check depth limit.
		if opts.MaxDepth >= 0 && depth[node.TxHash] > opts.MaxDepth {
			continue
		}

		// Yield to consumer if filter passes.
		if opts.Filter == nil || opts.Filter(node) {
			if !yield(node) {
				return
			}
		}

		// Add children to continue traversal (depth starts at 1 for children).
		nextDepth := depth[node.TxHash] + 1
		for _, child := range node.Children {
			if !visited[child.TxHash] {
				queue.Enqueue(child)
				depth[child.TxHash] = nextDepth
			}
		}
	}
}

// iterateCluster iterates over all nodes in the same cluster.
func (g *TxGraph) iterateCluster(start *TxGraphNode, opts IteratorOption, yield func(*TxGraphNode) bool) {
	if start == nil {
		// Iterate all clusters.
		for _, cluster := range g.indexes.clusters {
			for _, node := range cluster.Nodes {
				if opts.Filter != nil && !opts.Filter(node) {
					continue
				}
				if !yield(node) {
					return
				}
			}
		}
		return
	}

	// Find cluster for start node.
	clusterID, exists := g.indexes.nodeToCluster[start.TxHash]
	if !exists {
		return
	}

	cluster, exists := g.indexes.clusters[clusterID]
	if !exists {
		return
	}

	// Iterate nodes in cluster.
	for _, node := range cluster.Nodes {
		if opts.Filter != nil && !opts.Filter(node) {
			continue
		}
		if !yield(node) {
			return
		}
	}
}

// iterateFeeRate iterates nodes ordered by fee rate using a max-heap to
// efficiently yield transactions in descending order of fee rate.
func (g *TxGraph) iterateFeeRate(
	opts IteratorOption,
	yield func(*TxGraphNode) bool,
) {

	pq := NewPriorityQueue(func(a, b *TxGraphNode) bool {
		return a.TxDesc.FeePerKB > b.TxDesc.FeePerKB
	}, len(g.nodes))

	for _, node := range g.nodes {
		if opts.Filter != nil && !opts.Filter(node) {
			continue
		}
		pq.Push(node)
	}

	for !pq.IsEmpty() {
		node, _ := pq.Pop()
		if !yield(node) {
			return
		}
	}
}

// IteratePackages returns an iterator over packages.
func (g *TxGraph) IteratePackages() iter.Seq[*TxPackage] {
	return func(yield func(*TxPackage) bool) {
		g.mu.RLock()
		defer g.mu.RUnlock()

		for _, pkg := range g.indexes.packages {
			if !yield(pkg) {
				return
			}
		}
	}
}

// IterateClusters returns an iterator over clusters.
func (g *TxGraph) IterateClusters() iter.Seq[*TxCluster] {
	return func(yield func(*TxCluster) bool) {
		g.mu.RLock()
		defer g.mu.RUnlock()

		for _, cluster := range g.indexes.clusters {
			if !yield(cluster) {
				return
			}
		}
	}
}

// IterateOrphans returns an iterator over orphan transactions.
// A transaction is considered an orphan if:
// 1. It has no parents in the graph (len(Parents) == 0)
// 2. AND at least one of its inputs is unconfirmed (as determined by the
//    isConfirmed predicate)
//
// If isConfirmed is nil, all transactions with no parents are yielded. This
// is useful when the caller cannot determine chain state and wants to
// identify all potentially orphaned transactions.
func (g *TxGraph) IterateOrphans(
	isConfirmed InputConfirmedPredicate,
) iter.Seq[*TxGraphNode] {
	return func(yield func(*TxGraphNode) bool) {
		g.mu.RLock()
		defer g.mu.RUnlock()

		for _, node := range g.nodes {
			// Orphans by definition have no parents in the mempool, since
			// they're waiting for unconfirmed parent transactions that
			// haven't arrived yet.
			if len(node.Parents) > 0 {
				continue
			}

			// Without chain state access, we conservatively treat all
			// parentless transactions as potential orphans.
			if isConfirmed == nil {
				if !yield(node) {
					return
				}
				continue
			}

			// Distinguish between true orphans (waiting for unconfirmed
			// parents) and root transactions (spending confirmed UTXOs).
			// A transaction is only an orphan if at least one input
			// references an unconfirmed output not in the mempool.
			hasUnconfirmedInput := false
			for _, txIn := range node.Tx.MsgTx().TxIn {
				if !isConfirmed(txIn.PreviousOutPoint) {
					hasUnconfirmedInput = true
					break
				}
			}

			// Root transactions with all confirmed inputs are not orphans,
			// as they're not waiting for any parent transactions.
			if hasUnconfirmedInput {
				if !yield(node) {
					return
				}
			}
		}
	}
}

// addNeighborsToStack adds neighbors to DFS stack based on direction.
func (g *TxGraph) addNeighborsToStack(
	node *TxGraphNode,
	stack *Stack[*TxGraphNode],
	depth map[chainhash.Hash]int,
	direction TraversalDirection,
	nextDepth int,
) {
	switch direction {
	case DirectionForward:
		for _, child := range node.Children {
			if _, exists := depth[child.TxHash]; !exists {
				stack.Push(child)
				depth[child.TxHash] = nextDepth
			}
		}
	case DirectionBackward:
		for _, parent := range node.Parents {
			if _, exists := depth[parent.TxHash]; !exists {
				stack.Push(parent)
				depth[parent.TxHash] = nextDepth
			}
		}
	case DirectionBoth:
		for _, child := range node.Children {
			if _, exists := depth[child.TxHash]; !exists {
				stack.Push(child)
				depth[child.TxHash] = nextDepth
			}
		}
		for _, parent := range node.Parents {
			if _, exists := depth[parent.TxHash]; !exists {
				stack.Push(parent)
				depth[parent.TxHash] = nextDepth
			}
		}
	}
}

// addNeighborsToQueue adds neighbors to BFS queue based on direction.
func (g *TxGraph) addNeighborsToQueue(
	node *TxGraphNode,
	queue *Queue[*TxGraphNode],
	depth map[chainhash.Hash]int,
	direction TraversalDirection,
	nextDepth int,
) {
	switch direction {
	case DirectionForward:
		for _, child := range node.Children {
			if _, exists := depth[child.TxHash]; !exists {
				queue.Enqueue(child)
				depth[child.TxHash] = nextDepth
			}
		}
	case DirectionBackward:
		for _, parent := range node.Parents {
			if _, exists := depth[parent.TxHash]; !exists {
				queue.Enqueue(parent)
				depth[parent.TxHash] = nextDepth
			}
		}
	case DirectionBoth:
		for _, child := range node.Children {
			if _, exists := depth[child.TxHash]; !exists {
				queue.Enqueue(child)
				depth[child.TxHash] = nextDepth
			}
		}
		for _, parent := range node.Parents {
			if _, exists := depth[parent.TxHash]; !exists {
				queue.Enqueue(parent)
				depth[parent.TxHash] = nextDepth
			}
		}
	}
}