package txgraph

import (
	"iter"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// TxDesc contains transaction metadata for graph nodes.
// This is a simplified version to avoid circular mempool dependencies while
// still providing the information needed for ancestor/descendant calculations,
// fee rate analysis, and package validation.
type TxDesc struct {
	// TxHash is the transaction identifier used for graph lookups and
	// relationship tracking.
	TxHash chainhash.Hash

	// VirtualSize is used to calculate ancestor/descendant size limits and
	// to compute effective fee rates for package evaluation.
	VirtualSize int64

	// Fee is tracked to enable package fee calculations and to determine
	// whether fee-based policies are satisfied.
	Fee int64

	// FeePerKB enables sorting and filtering transactions by fee rate,
	// which is critical for block template construction and RBF logic.
	FeePerKB int64

	// Added tracks insertion time to enable time-based expiration and to
	// provide ordering for transactions with identical fee rates.
	Added time.Time
}

// PackageType represents the type of transaction package.
type PackageType uint8

const (
	// PackageTypeUnknown represents an unknown package type.
	PackageTypeUnknown PackageType = iota

	// PackageTypeStandard represents a standard package.
	PackageTypeStandard

	// PackageType1P1C represents a one-parent-one-child package.
	PackageType1P1C

	// PackageTypeTRUC represents a TRUC (v3) constrained package.
	PackageTypeTRUC

	// PackageTypeEphemeral represents a package with ephemeral dust.
	PackageTypeEphemeral
)

// ClusterID uniquely identifies a connected component of transactions.
type ClusterID uint64

// PackageID uniquely identifies a transaction package.
type PackageID struct {
	// Hash identifies the root transaction of the package, which serves
	// as the canonical identifier since all packages are rooted at a
	// specific transaction.
	Hash chainhash.Hash

	// Type distinguishes between package types (1P1C, TRUC, ephemeral)
	// since the same root transaction could theoretically belong to
	// multiple package interpretations.
	Type PackageType
}

// FeeratePoint represents a point on a feerate diagram.
type FeeratePoint struct {
	// CumulativeSize tracks the total size up to this point, enabling
	// efficient comparison of feerate diagrams for RBF validation.
	CumulativeSize int64

	// CumulativeFee tracks the total fees up to this point, used to
	// compute marginal fee rates and incentive compatibility.
	CumulativeFee int64

	// Feerate stores the marginal feerate at this point for quick
	// comparisons without recomputing from cumulative values.
	Feerate int64
}

// PackageTopology describes the shape of a package.
type PackageTopology struct {
	// MaxDepth tracks the longest ancestor chain in the package, enabling
	// enforcement of depth-based limits like TRUC's single-parent rule.
	MaxDepth int

	// MaxWidth tracks the maximum number of siblings at any level,
	// enabling detection of fan-out patterns.
	MaxWidth int

	// TotalNodes counts transactions in the package for quick size checks
	// without iterating the transaction map.
	TotalNodes int

	// IsLinear indicates a simple chain structure (A->B->C) which enables
	// optimizations for 1P1C packages and simpler validation logic.
	IsLinear bool

	// IsTree indicates no transaction has multiple parents (no diamond
	// patterns), which simplifies fee rate calculations and ensures
	// unambiguous ancestor relationships.
	IsTree bool
}

// GraphMetrics provides statistics about the transaction graph.
type GraphMetrics struct {
	// NodeCount tracks the total number of transactions in the graph for
	// capacity planning and monitoring.
	NodeCount int

	// EdgeCount tracks the total number of parent-child relationships,
	// indicating graph connectivity and complexity.
	EdgeCount int

	// PackageCount tracks identified packages for relay and mining
	// optimization monitoring.
	PackageCount int

	// TRUCCount tracks v3 transactions for monitoring adoption of the TRUC
	// policy and ensuring topology restrictions are enforced.
	TRUCCount int

	// EphemeralCount tracks transactions with ephemeral dust, which require
	// special handling to ensure dust is always spent.
	EphemeralCount int

	// MaxAncestors tracks the largest ancestor set size, helping identify
	// transactions approaching policy limits.
	MaxAncestors int

	// MaxDescendants tracks the largest descendant set size for policy
	// limit monitoring and potential eviction candidates.
	MaxDescendants int

	// AveragePackageSize provides insight into typical package complexity
	// for resource planning and optimization.
	AveragePackageSize float64

	// ClusterCount tracks the number of connected components, indicating
	// mempool fragmentation and potential for cluster-based eviction.
	ClusterCount int
}

// TxGraphNode represents a single transaction in the graph.
type TxGraphNode struct {
	// TxHash enables O(1) lookups in maps without dereferencing Tx.
	TxHash chainhash.Hash

	// Tx provides access to inputs and outputs for validation and edge
	// creation.
	Tx *btcutil.Tx

	// TxDesc stores fee and size information needed for policy decisions.
	TxDesc *TxDesc

	// Parents maps to transactions that this transaction spends outputs
	// from. Using a map enables O(1) parent existence checks during graph
	// traversal and cycle detection.
	Parents map[chainhash.Hash]*TxGraphNode

	// Children maps to transactions that spend this transaction's outputs.
	// Map structure allows efficient child removal during eviction without
	// scanning slices.
	Children map[chainhash.Hash]*TxGraphNode

	// Metadata holds feature-specific flags and relationships that don't
	// affect core graph structure but enable specialized processing.
	Metadata struct {
		// IsTRUC marks v3 transactions for topology validation.
		IsTRUC bool

		// IsEphemeral identifies transactions with dust outputs that
		// must be spent in the same package.
		IsEphemeral bool

		// PackageID associates this transaction with its package for
		// group validation and eviction.
		PackageID *PackageID

		// ClusterID groups connected transactions for RBF and CPFP
		// conflict detection.
		ClusterID ClusterID

		// AddedTime enables time-based eviction policies.
		AddedTime time.Time
	}
}

// TxCluster represents a connected component in the graph.
type TxCluster struct {
	// ID uniquely identifies this cluster for tracking relationships across
	// graph mutations.
	ID ClusterID

	// Nodes stores all transactions in this connected component, enabling
	// O(1) membership tests during cluster merges and splits.
	Nodes map[chainhash.Hash]*TxGraphNode

	// Size tracks the number of transactions for quick cluster size checks
	// without iterating the Nodes map.
	Size int

	// TotalFees aggregates fees across the cluster to compute effective
	// fee rates for mining and eviction decisions.
	TotalFees int64

	// TotalVSize aggregates virtual sizes to enforce cluster size limits
	// and to calculate cluster fee rates.
	TotalVSize int64

	// FeerateDiagram caches the feerate diagram used for RBF incentive
	// compatibility checks. This is expensive to compute so we cache it.
	FeerateDiagram []FeeratePoint

	// LastUpdated tracks when metrics were last computed, enabling
	// invalidation when the cluster changes.
	LastUpdated time.Time
}

// TxPackage represents a set of related transactions.
type TxPackage struct {
	// ID uniquely identifies this package for tracking and validation.
	ID PackageID

	// Transactions stores all members of the package. Using a map enables
	// efficient membership checks during package validation.
	Transactions map[chainhash.Hash]*TxGraphNode

	// Root identifies the root transaction that anchors this package.
	// All package types are rooted at a specific transaction.
	Root *TxGraphNode

	// TotalFees aggregates fees across the package to compute effective
	// package fee rates for relay and mining decisions.
	TotalFees int64

	// TotalSize aggregates sizes to enforce package size limits and to
	// calculate package fee rates.
	TotalSize int64

	// FeeRate stores the computed package feerate in sats per vbyte for
	// quick comparisons during relay and block template construction.
	FeeRate int64

	// Type identifies the package category (1P1C, TRUC, ephemeral) which
	// determines what validation rules apply.
	Type PackageType

	// Topology describes the shape of the package, enabling topology-based
	// validation rules like TRUC's single-child restriction.
	Topology PackageTopology

	// IsValid caches the validation result to avoid repeated expensive
	// validation checks during processing.
	IsValid bool

	// LastValidated tracks when validation occurred, enabling cache
	// invalidation if the package changes.
	LastValidated time.Time
}

// Graph defines the primary interface for transaction graph operations.
type Graph interface {
	// AddTransaction inserts a new transaction into the graph and
	// automatically creates edges to any parent transactions already in
	// the graph. This enables incremental graph construction as
	// transactions arrive from the network.
	AddTransaction(tx *btcutil.Tx, txDesc *TxDesc) error

	// RemoveTransaction removes a transaction and recursively removes all
	// descendants, maintaining graph consistency. This is used during
	// block confirmations and mempool evictions to prevent orphaned
	// children from remaining in the graph.
	RemoveTransaction(hash chainhash.Hash) error

	// RemoveTransactionNoCascade removes only the specified transaction
	// without touching descendants. This is useful when descendants will
	// be explicitly handled or when the caller needs fine-grained control
	// over eviction ordering.
	RemoveTransactionNoCascade(hash chainhash.Hash) error

	// GetNode retrieves a transaction from the graph by hash. The boolean
	// return indicates existence, enabling distinction between missing
	// transactions and nil nodes.
	GetNode(hash chainhash.Hash) (*TxGraphNode, bool)

	// HasTransaction checks if a transaction exists in the graph without
	// retrieving it, enabling efficient existence checks when the node
	// data isn't needed.
	HasTransaction(hash chainhash.Hash) bool

	// GetAncestors returns all ancestor transactions up to maxDepth.
	// This is used to enforce ancestor count/size limits for policy
	// validation and to compute ancestor fee rates for CPFP.
	GetAncestors(
		hash chainhash.Hash, maxDepth int,
	) map[chainhash.Hash]*TxGraphNode

	// GetDescendants returns all descendant transactions up to maxDepth.
	// This is used to enforce descendant limits and to identify all
	// transactions that must be removed when evicting a parent.
	GetDescendants(
		hash chainhash.Hash, maxDepth int,
	) map[chainhash.Hash]*TxGraphNode

	// GetCluster retrieves the connected component containing the given
	// transaction. This enables cluster-based fee rate calculations for
	// mining and RBF validation.
	GetCluster(hash chainhash.Hash) (*TxCluster, error)

	// GetOrphans returns transactions with unconfirmed inputs not in the
	// mempool. A transaction is an orphan if it has no parents in the
	// graph AND its inputs are not confirmed (as determined by the
	// predicate). If isConfirmed is nil, all transactions with no parents
	// are considered orphans.
	GetOrphans(isConfirmed InputConfirmedPredicate) []*TxGraphNode

	// IdentifyPackages scans the graph to detect transaction packages
	// (1P1C, TRUC, ephemeral). This enables package-aware relay and mining
	// optimizations by grouping related transactions.
	IdentifyPackages() ([]*TxPackage, error)

	// GetPackage retrieves a previously identified package by its root
	// transaction hash. This enables efficient package lookups during
	// relay validation and block template construction.
	GetPackage(hash chainhash.Hash) (*TxPackage, error)

	// ValidatePackage checks if a package satisfies all type-specific
	// rules (topology, size, TRUC constraints). This ensures only valid
	// packages are relayed and mined.
	ValidatePackage(pkg *TxPackage) error

	// Iterate returns an iterator over graph nodes using the specified
	// order and filters. This enables lazy evaluation of large result sets
	// without allocating memory for all matches upfront.
	Iterate(opts IteratorOption) iter.Seq[*TxGraphNode]

	// IteratePackages returns an iterator over all identified packages.
	// This enables package-by-package processing during block template
	// construction and relay decisions.
	IteratePackages() iter.Seq[*TxPackage]

	// IterateClusters returns an iterator over connected components in the
	// graph. This enables cluster-based fee rate analysis for mining and
	// cluster-aware eviction policies.
	IterateClusters() iter.Seq[*TxCluster]

	// IterateOrphans iterates over transactions with unconfirmed inputs
	// not in the mempool. See GetOrphans for the definition of an
	// orphan transaction.
	IterateOrphans(isConfirmed InputConfirmedPredicate) iter.Seq[*TxGraphNode]

	// GetMetrics returns comprehensive statistics about the graph. This
	// enables monitoring of mempool health, capacity planning, and
	// detection of unusual graph structures.
	GetMetrics() GraphMetrics

	// GetNodeCount returns the number of transactions in the graph. This
	// provides a quick way to check mempool size without computing full
	// metrics.
	GetNodeCount() int

	// GetClusterCount returns the number of connected components. This
	// indicates mempool fragmentation and is useful for understanding the
	// effectiveness of cluster-based optimizations.
	GetClusterCount() int
}

// TraversalOrder defines the traversal strategy for graph iteration.
type TraversalOrder uint8

const (
	// TraversalDefault iterates all nodes without specific order.
	TraversalDefault TraversalOrder = iota

	// TraversalDFS performs depth-first search.
	TraversalDFS

	// TraversalBFS performs breadth-first search.
	TraversalBFS

	// TraversalTopological visits in topological order.
	TraversalTopological

	// TraversalReverseTopo visits in reverse topological order.
	TraversalReverseTopo

	// TraversalAncestors visits ancestors only.
	TraversalAncestors

	// TraversalDescendants visits descendants only.
	TraversalDescendants

	// TraversalCluster visits all transactions in the same cluster.
	TraversalCluster

	// TraversalFeeRate visits in order by fee rate (high to low).
	TraversalFeeRate
)

// TraversalDirection specifies the direction of traversal.
type TraversalDirection uint8

const (
	// DirectionForward traverses from parents to children.
	DirectionForward TraversalDirection = iota

	// DirectionBackward traverses from children to parents.
	DirectionBackward

	// DirectionBoth traverses in both directions.
	DirectionBoth
)

// IteratorOption configures graph iteration behavior.
type IteratorOption struct {
	Order        TraversalOrder
	MaxDepth     int
	Filter       func(*TxGraphNode) bool
	StartNode    *chainhash.Hash
	Direction    TraversalDirection
	IncludeStart bool
}

// DefaultIteratorOption returns an IteratorOption with sensible defaults.
func DefaultIteratorOption() IteratorOption {
	return IteratorOption{
		Order:        TraversalDefault,
		MaxDepth:     -1,
		Direction:    DirectionForward,
		IncludeStart: false,
	}
}

// IterOption is a functional option for configuring iteration.
type IterOption func(*IteratorOption)

// WithOrder sets the traversal order.
func WithOrder(order TraversalOrder) IterOption {
	return func(o *IteratorOption) {
		o.Order = order
	}
}

// WithMaxDepth sets the maximum traversal depth (-1 for unlimited).
func WithMaxDepth(depth int) IterOption {
	return func(o *IteratorOption) {
		o.MaxDepth = depth
	}
}

// WithFilter sets a filter predicate.
func WithFilter(filter func(*TxGraphNode) bool) IterOption {
	return func(o *IteratorOption) {
		o.Filter = filter
	}
}

// WithStartNode sets the starting node for traversal.
func WithStartNode(hash *chainhash.Hash) IterOption {
	return func(o *IteratorOption) {
		o.StartNode = hash
	}
}

// WithDirection sets the traversal direction.
func WithDirection(direction TraversalDirection) IterOption {
	return func(o *IteratorOption) {
		o.Direction = direction
	}
}

// WithIncludeStart sets whether to include the starting node.
func WithIncludeStart(include bool) IterOption {
	return func(o *IteratorOption) {
		o.IncludeStart = include
	}
}

// InputConfirmedPredicate is a function that checks if a transaction input
// references a confirmed UTXO. This is used to distinguish between:
// - Orphans: transactions with unconfirmed inputs not in the mempool
// - Root transactions: transactions with confirmed inputs (not orphans)
//
// The predicate takes an outpoint and returns true if that output is confirmed
// on-chain, false if it's unconfirmed or doesn't exist.
type InputConfirmedPredicate func(outpoint wire.OutPoint) bool

