// Copyright (c) 2013-2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mempool/txgraph"
	"github.com/btcsuite/btcd/wire"
)

// Policy violation errors.
var (
	// ErrTooManyEvictions indicates a replacement transaction would evict
	// too many transactions from the mempool.
	ErrTooManyEvictions = errors.New("replacement evicts too many transactions")

	// ErrReplacementSpendsParent indicates a replacement transaction
	// attempts to spend an output from a transaction it's replacing.
	ErrReplacementSpendsParent = errors.New("replacement spends parent transaction")

	// ErrInsufficientFeeRate indicates a replacement transaction has an
	// insufficient fee rate compared to the transactions it's replacing.
	ErrInsufficientFeeRate = errors.New("insufficient fee rate for replacement")

	// ErrInsufficientAbsoluteFee indicates a replacement transaction has an
	// insufficient absolute fee compared to the transactions it's replacing.
	ErrInsufficientAbsoluteFee = errors.New("insufficient absolute fee for replacement")

	// ErrNewUnconfirmedInput indicates a replacement transaction introduces
	// new unconfirmed inputs not present in the conflicts.
	ErrNewUnconfirmedInput = errors.New("replacement has new unconfirmed input")

	// ErrExceededAncestorLimit indicates a transaction would exceed the
	// maximum number or size of ancestors.
	ErrExceededAncestorLimit = errors.New("transaction exceeds ancestor limit")

	// ErrExceededDescendantLimit indicates a transaction would exceed the
	// maximum number or size of descendants.
	ErrExceededDescendantLimit = errors.New("transaction exceeds descendant limit")
)

// PolicyGraph defines the minimal graph interface needed by PolicyEnforcer.
// This is a subset of txgraph.Graph focused on relationship queries needed
// for policy decisions.
type PolicyGraph interface {
	// GetNode retrieves a transaction node from the graph.
	GetNode(hash chainhash.Hash) (*txgraph.TxGraphNode, bool)

	// GetAncestors returns all ancestor transactions up to maxDepth. A
	// negative maxDepth returns all ancestors.
	GetAncestors(hash chainhash.Hash,
		maxDepth int) map[chainhash.Hash]*txgraph.TxGraphNode

	// GetDescendants returns all descendant transactions up to maxDepth. A
	// negative maxDepth returns all descendants.
	GetDescendants(hash chainhash.Hash,
		maxDepth int) map[chainhash.Hash]*txgraph.TxGraphNode

	// IsValidPackageExtension validates that adding the transaction would
	// create a valid package with its parents and siblings. This checks
	// package-level constraints (TRUC topology, ephemeral dust, size limits)
	// without tracking the package. This composite method encapsulates the
	// details of package construction and validation.
	IsValidPackageExtension(tx *btcutil.Tx,
		desc *txgraph.TxDesc) error

	// BuildPackageNodes constructs the complete set of nodes needed to
	// validate adding a transaction to the mempool. This includes the new
	// transaction, all unconfirmed ancestors, and their descendants.
	// Required for package context building in BIP 431 Rule 6.
	BuildPackageNodes(tx *btcutil.Tx, desc *txgraph.TxDesc) []*txgraph.TxGraphNode

	// CreatePackage validates and classifies a package of transaction nodes.
	// Supports dry-run mode (WithDryRun option) for validation without
	// tracking. Required for package context building in BIP 431 Rule 6.
	CreatePackage(nodes []*txgraph.TxGraphNode, opts ...txgraph.PackageOption) (*txgraph.TxPackage, error)
}

// PolicyEnforcer defines the interface for mempool policy enforcement. This
// separates policy decisions from graph data structure operations, enabling
// easier testing and different policy configurations.
type PolicyEnforcer interface {
	// SignalsReplacement determines if a transaction signals that it can
	// be replaced using the Replace-By-Fee (RBF) policy. This includes
	// both explicit signaling (sequence number) and inherited signaling
	// (unconfirmed ancestors that signal RBF).
	SignalsReplacement(graph PolicyGraph, tx *btcutil.Tx) bool

	// ValidateReplacement determines whether a transaction is a valid
	// replacement of its conflicts according to BIP 125 RBF rules.
	ValidateReplacement(graph PolicyGraph, tx *btcutil.Tx, txFee int64,
		conflicts *txgraph.ConflictSet) error

	// ValidateAncestorLimits checks that a transaction doesn't exceed the
	// configured ancestor count and size limits.
	ValidateAncestorLimits(graph PolicyGraph, hash chainhash.Hash) error

	// ValidateDescendantLimits checks that a transaction doesn't exceed
	// the configured descendant count and size limits.
	ValidateDescendantLimits(graph PolicyGraph, hash chainhash.Hash) error

	// ValidateRelayFee checks that a transaction meets the minimum relay
	// fee requirements, including priority checks and rate limiting for
	// free/low-fee transactions.
	//
	// The optional packageContext parameter enables BIP 431 Rule 6 support:
	// TRUC (v3) transactions may be below the minimum relay fee when part
	// of a valid package, as long as the package fee rate meets requirements.
	ValidateRelayFee(tx *btcutil.Tx, fee int64, size int64,
		utxoView *blockchain.UtxoViewpoint, nextBlockHeight int32,
		isNew bool, packageContext *PackageContext) error

	// ValidateStandardness checks that a transaction meets standardness
	// requirements for relay (version, size, scripts, dust outputs).
	ValidateStandardness(tx *btcutil.Tx, height int32,
		medianTimePast time.Time, utxoView *blockchain.UtxoViewpoint,
	) error

	// ValidateSigCost checks that a transaction's signature operation cost
	// does not exceed the maximum allowed for relay.
	ValidateSigCost(tx *btcutil.Tx,
		utxoView *blockchain.UtxoViewpoint) error

	// ValidateSegWitDeployment checks that if a transaction contains
	// witness data, the SegWit soft fork must be active.
	ValidateSegWitDeployment(tx *btcutil.Tx) error

	// ValidatePackagePolicy validates that the transaction's package meets
	// all policy requirements (TRUC topology, ephemeral dust, size limits).
	//
	// This validates BIP 431 TRUC constraints and other package-level
	// policies. TRUC topology rules enforce ancestor/descendant limits
	// (max depth=1, 1-parent-1-child) to prevent pinning attacks. This is
	// conceptually similar to ValidateAncestorLimits but operates at the
	// package level.
	ValidatePackagePolicy(graph PolicyGraph, tx *btcutil.Tx,
		desc *txgraph.TxDesc) error

	// BuildPackageContext analyzes transactions and their relationship to
	// existing mempool transactions to build a package context for BIP 431
	// Rule 6 validation (allowing zero-fee TRUC transactions in packages).
	//
	// This method leverages existing package infrastructure:
	// - BuildPackageNodes: builds validation context (new txs + ancestors)
	// - CreatePackage (dry run): validates and classifies the package
	// - AnalyzePackageType: determines if it's TRUC/ephemeral/1P1C
	//
	// Returns a populated PackageContext along with the validated TxPackage.
	// The context is always returned when validation succeeds, even if the
	// package is not classified as TRUC. The returned error explains why a
	// package is invalid (topology, limits, etc.).
	BuildPackageContext(graph PolicyGraph, txs []*btcutil.Tx,
		descs []*txgraph.TxDesc) (*PackageContext, *txgraph.TxPackage, error)

	// ValidatePackageReplacement validates that a package can replace existing
	// mempool transactions according to BIP 125 rules, applied at package level.
	// This extends single-transaction RBF to entire packages.
	ValidatePackageReplacement(graph PolicyGraph, txs []*btcutil.Tx,
		packageFee int64, allConflicts map[chainhash.Hash]*txgraph.ConflictSet) error
}

// PolicyConfig defines mempool policy parameters. These settings control
// transaction acceptance, replacement, and relay behavior.
type PolicyConfig struct {
	// MaxRBFSequence is the maximum sequence number an input can use to
	// signal that the transaction can be replaced. Per BIP 125, this is
	// 0xfffffffd.
	MaxRBFSequence uint32

	// MaxReplacementEvictions is the maximum number of transactions that
	// can be evicted when accepting a replacement. Bitcoin Core uses 100.
	MaxReplacementEvictions int

	// RejectReplacement, if true, rejects all replacement transactions
	// regardless of RBF signaling.
	RejectReplacement bool

	// MaxAncestorCount is the maximum number of unconfirmed ancestors
	// (including the transaction itself) allowed. Bitcoin Core uses 25.
	MaxAncestorCount int

	// MaxAncestorSize is the maximum total virtual size in bytes of all
	// ancestors (including the transaction itself). Bitcoin Core uses
	// 101 KB.
	MaxAncestorSize int64

	// MaxDescendantCount is the maximum number of descendant transactions
	// allowed. Bitcoin Core uses 25.
	MaxDescendantCount int

	// MaxDescendantSize is the maximum total virtual size in bytes of all
	// descendants. Bitcoin Core uses 101 KB.
	MaxDescendantSize int64

	// MinRelayTxFee defines the minimum transaction fee in satoshi/kB
	// required for relay and mining.
	MinRelayTxFee btcutil.Amount

	// FreeTxRelayLimit defines the rate limit in KB/minute for
	// transactions with fees below MinRelayTxFee.
	FreeTxRelayLimit float64

	// DisableRelayPriority, if true, disables relaying of low-fee
	// transactions based on priority.
	DisableRelayPriority bool

	// MaxTxVersion is the maximum transaction version to accept.
	// Transactions with versions above this are rejected as non-standard.
	MaxTxVersion int32

	// MaxSigOpCostPerTx is the cumulative maximum cost of all signature
	// operations in a single transaction that will be relayed or mined.
	MaxSigOpCostPerTx int

	// IsDeploymentActive checks if a consensus deployment is active.
	// This is used for validating SegWit transactions.
	IsDeploymentActive func(deploymentID uint32) (bool, error)

	// ChainParams identifies the blockchain network (mainnet, testnet,
	// etc). Used for network-specific validation rules.
	ChainParams *chaincfg.Params

	// BestHeight returns the current best block height.
	BestHeight func() int32
}

// DefaultPolicyConfig returns a PolicyConfig with default values matching
// Bitcoin Core's mempool policy.
func DefaultPolicyConfig() PolicyConfig {
	return PolicyConfig{
		// RBF settings from BIP 125 and Bitcoin Core.
		MaxRBFSequence:          MaxRBFSequence,
		MaxReplacementEvictions: MaxReplacementEvictions,
		RejectReplacement:       false,

		// Ancestor/descendant limits from Bitcoin Core.
		MaxAncestorCount:   25,
		MaxAncestorSize:    101000,
		MaxDescendantCount: 25,
		MaxDescendantSize:  101000,

		// Fee settings from Bitcoin Core.
		MinRelayTxFee:        DefaultMinRelayTxFee,
		FreeTxRelayLimit:     15.0,
		DisableRelayPriority: false,

		// Transaction version and signature operation limits.
		MaxTxVersion:      2,     // Standard transaction version
		MaxSigOpCostPerTx: 80000, // 1/5 of max block sigop cost

		// Default deployment check (assume SegWit is active for testing).
		IsDeploymentActive: func(deploymentID uint32) (bool, error) {
			return true, nil
		},

		// Default to mainnet params.
		ChainParams: &chaincfg.MainNetParams,

		// Default height (reasonable for testing).
		BestHeight: func() int32 {
			return 700000
		},
	}
}

// StandardPolicyEnforcer implements the Policy interface with Bitcoin Core
// compatible policy enforcement.
type StandardPolicyEnforcer struct {
	cfg PolicyConfig

	mu            sync.Mutex
	pennyTotal    float64
	lastPennyUnix int64
}

// NewStandardPolicyEnforcer creates a new policy enforcer with the given
// configuration.
func NewStandardPolicyEnforcer(cfg PolicyConfig) *StandardPolicyEnforcer {
	return &StandardPolicyEnforcer{
		cfg:           cfg,
		lastPennyUnix: time.Now().Unix(),
	}
}

// SignalsReplacement determines if a transaction is signaling that it can be
// replaced using the Replace-By-Fee (RBF) policy.
//
// Per BIP 125, a transaction signals replaceability in two ways:
//
// 1. Explicit signaling: Any input has a sequence number <= MaxRBFSequence.
// 2. Inherited signaling: Any unconfirmed ancestor signals replaceability.
//
// Additionally, per BIP 431 Rule 1, v3 (TRUC) transactions always signal
// replaceability regardless of sequence numbers.
func (p *StandardPolicyEnforcer) SignalsReplacement(
	graph PolicyGraph, tx *btcutil.Tx) bool {

	// BIP 431 Rule 1: v3 transactions always signal replaceability.
	if tx.MsgTx().Version == 3 {
		return true
	}

	// Check for explicit signaling in this transaction's inputs.
	for _, txIn := range tx.MsgTx().TxIn {
		if txIn.Sequence <= p.cfg.MaxRBFSequence {
			return true
		}
	}

	// Check for inherited signaling from unconfirmed ancestors. Use a
	// cache to avoid redundant checks when traversing the graph.
	cache := make(map[chainhash.Hash]bool)
	for _, txIn := range tx.MsgTx().TxIn {
		parentHash := txIn.PreviousOutPoint.Hash
		if p.signalsReplacementRecursive(graph, parentHash, cache) {
			return true
		}
	}

	return false
}

// signalsReplacementRecursive recursively checks if a transaction or any of
// its unconfirmed ancestors signal RBF replacement.
func (p *StandardPolicyEnforcer) signalsReplacementRecursive(
	graph PolicyGraph, hash chainhash.Hash,
	cache map[chainhash.Hash]bool) bool {

	// Check cache first to avoid redundant traversal.
	if signals, ok := cache[hash]; ok {
		return signals
	}

	// Get the transaction from the graph. If it doesn't exist, it's either
	// confirmed or unknown, so it doesn't signal replacement.
	node, exists := graph.GetNode(hash)
	if !exists {
		cache[hash] = false
		return false
	}

	// Check for explicit signaling in this transaction's inputs.
	for _, txIn := range node.Tx.MsgTx().TxIn {
		if txIn.Sequence <= p.cfg.MaxRBFSequence {
			cache[hash] = true
			return true
		}
	}

	// Recursively check ancestors.
	for _, parent := range node.Parents {
		if p.signalsReplacementRecursive(graph, parent.TxHash, cache) {
			cache[hash] = true
			return true
		}
	}

	// This transaction and its ancestors don't signal replacement.
	cache[hash] = false
	return false
}

// ValidateReplacement determines whether a transaction is a valid replacement
// of its conflicts according to BIP 125 RBF rules.
//
// The BIP 125 rules enforced are:
//
//  1. The replacement evicts at most MaxReplacementEvictions transactions.
//  2. The replacement doesn't spend any outputs from the conflicts (no
//     spending parent).
//  3. The replacement has a higher fee rate than each conflict.
//  4. The replacement has a higher absolute fee than the sum of all conflicts
//     plus the minimum relay fee.
//  5. The replacement doesn't introduce new unconfirmed inputs beyond those
//     already present in the conflicts.
func (p *StandardPolicyEnforcer) ValidateReplacement(
	graph PolicyGraph,
	tx *btcutil.Tx,
	txFee int64,
	conflicts *txgraph.ConflictSet,
) error {
	ctx := context.Background()
	conflictCount := len(conflicts.Transactions)

	log.TraceS(ctx, "Validating RBF replacement",
		"tx_hash", tx.Hash(),
		"conflicts_count", conflictCount,
		"tx_fee", txFee)

	// Rule 1: Check eviction limit.
	if conflictCount > p.cfg.MaxReplacementEvictions {
		log.WarnS(ctx, "RBF eviction limit exceeded", nil,
			"tx_hash", tx.Hash(),
			"conflicts_count", conflictCount,
			"max_evictions", p.cfg.MaxReplacementEvictions)
		return fmt.Errorf("%w: %d conflicts (max %d)",
			ErrTooManyEvictions, conflictCount,
			p.cfg.MaxReplacementEvictions)
	}

	// Warn on large (but acceptable) replacements - potential DoS indicator.
	if conflictCount > 50 {
		log.InfoS(ctx, "Large RBF eviction count",
			"tx_hash", tx.Hash(),
			"conflicts_count", conflictCount,
			"threshold_pct", float64(conflictCount)/float64(p.cfg.MaxReplacementEvictions)*100)
	}

	// Rule 2: The replacement must not spend outputs from any of the conflicts.
	// Build ancestor set from the candidate transaction's inputs since the
	// candidate doesn't exist in the graph yet.
	for _, txIn := range tx.MsgTx().TxIn {
		parentHash := txIn.PreviousOutPoint.Hash

		// If the parent is one of the conflicts, that's OK (direct replacement).
		// But if any ancestor of the parent is a conflict, that violates BIP 125.
		if _, exists := graph.GetNode(parentHash); exists {
			// Check if this parent is a conflict.
			if _, isConflict := conflicts.Transactions[parentHash]; isConflict {
				continue // Direct parent conflict is allowed.
			}

			// Check if any of this parent's ancestors are conflicts.
			parentAncestors := graph.GetAncestors(parentHash, -1)
			for ancestorHash := range parentAncestors {
				if _, isConflict := conflicts.Transactions[ancestorHash]; isConflict {
					return fmt.Errorf("%w: %v", ErrReplacementSpendsParent,
						ancestorHash)
				}
			}
		}
	}

	// Rule 3: The replacement must have a higher fee rate than each conflict.
	txSize := GetTxVirtualSize(tx)
	txFeeRate := txFee * 1000 / txSize

	for conflictHash, conflictNode := range conflicts.Transactions {
		conflictFeeRate := conflictNode.TxDesc.Fee * 1000 /
			conflictNode.TxDesc.VirtualSize

		if txFeeRate <= conflictFeeRate {
			return fmt.Errorf("%w: replacement fee rate %d sat/kB <= "+
				"conflict %v fee rate %d sat/kB",
				ErrInsufficientFeeRate, txFeeRate, conflictHash,
				conflictFeeRate)
		}
	}

	// Rule 4: The replacement must have a higher absolute fee than the sum
	// of all conflicts plus the minimum relay fee.
	var conflictsFee int64
	for _, conflictNode := range conflicts.Transactions {
		conflictsFee += conflictNode.TxDesc.Fee
	}

	minFee := calcMinRequiredTxRelayFee(txSize, p.cfg.MinRelayTxFee)
	if txFee < conflictsFee+minFee {
		return fmt.Errorf("%w: replacement fee %d < conflicts fee %d + "+
			"relay fee %d", ErrInsufficientAbsoluteFee, txFee,
			conflictsFee, minFee)
	}

	// Rule 5: The replacement must not introduce new unconfirmed inputs
	// beyond those already present in the conflicts and their ancestors.
	conflictsInputs := make(map[chainhash.Hash]struct{})
	for _, conflictNode := range conflicts.Transactions {
		for _, txIn := range conflictNode.Tx.MsgTx().TxIn {
			conflictsInputs[txIn.PreviousOutPoint.Hash] = struct{}{}
		}
	}

	for _, txIn := range tx.MsgTx().TxIn {
		parentHash := txIn.PreviousOutPoint.Hash

		// OK if this input spends from a conflict.
		if _, inConflicts := conflictsInputs[parentHash]; inConflicts {
			continue
		}

		// Not OK if this introduces a new unconfirmed input.
		if _, exists := graph.GetNode(parentHash); exists {
			return fmt.Errorf("%w: %v", ErrNewUnconfirmedInput, parentHash)
		}
	}

	return nil
}

// ValidateAncestorLimits checks that a transaction doesn't exceed the
// configured ancestor count and size limits.
//
// Bitcoin Core enforces a limit of 25 ancestors and 101 KB total ancestor size
// to prevent unbounded chain growth in the mempool. This implementation
// matches that behavior.
func (p *StandardPolicyEnforcer) ValidateAncestorLimits(
	graph PolicyGraph, hash chainhash.Hash) error {
	ctx := context.Background()

	node, exists := graph.GetNode(hash)
	if !exists {
		return fmt.Errorf("transaction not found in graph")
	}

	// TRUC transactions have stricter topology limits (max 1 unconfirmed
	// ancestor) enforced by the PackageAnalyzer during package validation.
	// The standard 25-ancestor limit still applies to non-TRUC transactions.
	maxAncestors := p.cfg.MaxAncestorCount
	if node.Tx.MsgTx().Version == 3 {
		maxAncestors = 1
	}

	ancestors := graph.GetAncestors(hash, -1)

	ancestorCount := len(ancestors) + 1
	if ancestorCount > maxAncestors {
		log.WarnS(ctx, "Ancestor count limit exceeded", nil,
			"tx_hash", hash,
			"ancestor_count", ancestorCount,
			"max_ancestors", maxAncestors,
			"is_truc", node.Tx.MsgTx().Version == 3)
		return fmt.Errorf("%w: %d ancestors (max %d)",
			ErrExceededAncestorLimit, ancestorCount,
			maxAncestors)
	}

	// Log when approaching limit (potential chain spam).
	if ancestorCount > p.cfg.MaxAncestorCount*8/10 {
		log.DebugS(ctx, "High ancestor count",
			"tx_hash", hash,
			"ancestor_count", ancestorCount,
			"max_ancestors", p.cfg.MaxAncestorCount,
			"utilization_pct", ancestorCount*100/p.cfg.MaxAncestorCount)
	}

	// Check ancestor size limit (includes the transaction itself).
	var ancestorSize int64
	for _, ancestor := range ancestors {
		ancestorSize += ancestor.TxDesc.VirtualSize
	}

	// Add this transaction's size.
	ancestorSize += node.TxDesc.VirtualSize

	if ancestorSize > p.cfg.MaxAncestorSize {
		return fmt.Errorf("%w: %d bytes (max %d)",
			ErrExceededAncestorLimit, ancestorSize,
			p.cfg.MaxAncestorSize)
	}

	return nil
}

// ValidateDescendantLimits checks that a transaction doesn't exceed the
// configured descendant count and size limits.
//
// Bitcoin Core enforces a limit of 25 descendants and 101 KB total descendant
// size to prevent unbounded chain growth in the mempool. This implementation
// matches that behavior.
func (p *StandardPolicyEnforcer) ValidateDescendantLimits(
	graph PolicyGraph, hash chainhash.Hash) error {
	ctx := context.Background()

	node, exists := graph.GetNode(hash)
	if !exists {
		return fmt.Errorf("transaction not found in graph")
	}

	// TRUC transactions enforce max 1 unconfirmed descendant to prevent
	// pinning via descendant limits. Standard transactions use 25.
	maxDescendants := p.cfg.MaxDescendantCount
	if node.Tx.MsgTx().Version == 3 {
		maxDescendants = 1
	}

	descendants := graph.GetDescendants(hash, -1)

	descendantCount := len(descendants)
	if descendantCount > maxDescendants {
		log.WarnS(ctx, "Descendant count limit exceeded", nil,
			"tx_hash", hash,
			"descendant_count", descendantCount,
			"max_descendants", maxDescendants,
			"is_truc", node.Tx.MsgTx().Version == 3)
		return fmt.Errorf("%w: %d descendants (max %d)",
			ErrExceededDescendantLimit, descendantCount,
			maxDescendants)
	}

	// Check descendant size limit.
	var descendantSize int64
	for _, descendant := range descendants {
		descendantSize += descendant.TxDesc.VirtualSize
	}

	if descendantSize > p.cfg.MaxDescendantSize {
		return fmt.Errorf("%w: %d bytes (max %d)",
			ErrExceededDescendantLimit, descendantSize,
			p.cfg.MaxDescendantSize)
	}

	return nil
}

// ValidateRelayFee checks that a transaction meets the minimum relay fee
// requirements, including priority checks and rate limiting for free/low-fee
// transactions.
//
// Transactions with fees below the minimum are checked for priority (if
// enabled) and rate-limited using an exponentially decaying counter to prevent
// spam while allowing some free transactions.
//
// The optional packageContext enables BIP 431 Rule 6: TRUC (v3) transactions
// may be below the minimum relay fee when part of a valid package, as long as
// the package fee rate meets requirements.
func (p *StandardPolicyEnforcer) ValidateRelayFee(
	tx *btcutil.Tx, fee int64, size int64, utxoView *blockchain.UtxoViewpoint,
	nextBlockHeight int32, isNew bool, packageContext *PackageContext) error {
	ctx := context.Background()

	// BIP 431 Rule 6: TRUC (v3) transactions in packages may be below the
	// minimum relay fee. Individual transactions below the minimum are allowed
	// if they're part of a valid TRUC package where the package fee rate
	// meets requirements.
	//
	// This enables zero-fee commitment transactions in Lightning with fee
	// bumping via CPFP in a child transaction.
	if packageContext != nil && packageContext.IsTRUCPackage {
		// Check if this is a TRUC (v3) transaction.
		if tx.MsgTx().Version == 3 {
			// Skip individual min-relay-fee check for TRUC transactions
			// in packages. The package fee rate will be validated
			// separately to ensure the package as a whole meets fee
			// requirements.
			//
			// This allows zero-fee parent transactions as long as the
			// child pays enough to cover both (CPFP).
			log.DebugS(ctx, "TRUC transaction in package, skipping individual fee check",
				"tx_hash", tx.Hash(),
				"tx_fee", fee,
				"package_fee_rate", packageContext.PackageFeeRate)
			return nil
		}
	}

	// Check the minimum relay fee and priority requirements using
	// the standalone CheckRelayFee function.
	err := CheckRelayFee(
		tx, fee, size, utxoView, nextBlockHeight,
		p.cfg.MinRelayTxFee, p.cfg.DisableRelayPriority, isNew,
	)
	if err != nil {
		log.DebugS(ctx, "Transaction rejected due to low fee",
			"tx_hash", tx.Hash(),
			"fee", fee,
			"size", size,
			"fee_rate_sat_kb", fee*1000/size)
		return err
	}

	// Calculate minimum required fee to determine if rate limiting applies.
	minFee := calcMinRequiredTxRelayFee(size, p.cfg.MinRelayTxFee)

	// If the fee meets the minimum, no rate limiting needed.
	if fee >= minFee {
		return nil
	}

	// Apply rate limiting for free/low-fee transactions.
	p.mu.Lock()
	defer p.mu.Unlock()

	// Decay the penny total based on time elapsed since the last update.
	// This implements exponential decay with a 10-minute half-life.
	nowUnix := time.Now().Unix()
	p.pennyTotal *= math.Pow(1.0-1.0/600.0, float64(nowUnix-p.lastPennyUnix))
	p.lastPennyUnix = nowUnix

	// Check if we've exceeded the rate limit. Converting KB/min to
	// bytes/10min.
	limit := p.cfg.FreeTxRelayLimit * 10 * 1000
	if p.pennyTotal >= limit {
		return fmt.Errorf("transaction %v rejected by rate limiter: "+
			"%.0f bytes (limit %.0f bytes per 10 minutes)",
			tx.Hash(), p.pennyTotal, limit)
	}

	// Add this transaction's size to the penny total.
	p.pennyTotal += float64(size)

	return nil
}

// ValidateStandardness checks that a transaction meets standardness
// requirements for relay. This includes version checks, finalization, size
// limits, script checks, and dust checks.
func (p *StandardPolicyEnforcer) ValidateStandardness(
	tx *btcutil.Tx, height int32, medianTimePast time.Time,
	utxoView *blockchain.UtxoViewpoint) error {

	// Use the existing CheckTransactionStandard function which handles
	// version, finalization, size, and output script checks.
	err := CheckTransactionStandard(
		tx, height, medianTimePast,
		p.cfg.MinRelayTxFee, p.cfg.MaxTxVersion,
	)
	if err != nil {
		return err
	}

	// Also check input standardness (signature scripts, etc).
	return checkInputsStandard(tx, utxoView)
}

// ValidateSigCost checks that a transaction's signature operation cost does
// not exceed the maximum allowed for relay.
func (p *StandardPolicyEnforcer) ValidateSigCost(
	tx *btcutil.Tx, utxoView *blockchain.UtxoViewpoint) error {

	// Use the standalone CheckTransactionSigCost function to validate.
	return CheckTransactionSigCost(tx, utxoView, p.cfg.MaxSigOpCostPerTx)
}

// ValidateSegWitDeployment checks that if a transaction contains witness data,
// the SegWit soft fork must be active.
func (p *StandardPolicyEnforcer) ValidateSegWitDeployment(tx *btcutil.Tx) error {

	// Use the standalone CheckSegWitDeployment function to validate.
	return CheckSegWitDeployment(
		tx, p.cfg.IsDeploymentActive, p.cfg.ChainParams,
		p.cfg.BestHeight(),
	)
}

// ValidatePackagePolicy validates that the transaction's package meets all
// policy requirements (TRUC topology, ephemeral dust, size limits).
//
// This validates BIP 431 TRUC constraints and other package-level policies.
// TRUC topology rules enforce ancestor/descendant limits (max depth=1,
// 1-parent-1-child) to prevent pinning attacks. This is conceptually similar
// to ValidateAncestorLimits but operates at the package level.
func (p *StandardPolicyEnforcer) ValidatePackagePolicy(
	graph PolicyGraph,
	tx *btcutil.Tx,
	desc *txgraph.TxDesc,
) error {

	// Delegate to graph's composite validation method.
	return graph.IsValidPackageExtension(tx, desc)
}

// BuildPackageContext analyzes transactions and their mempool relationships
// to determine if they form a TRUC package that qualifies for BIP 431 Rule 6
// (allowing zero-fee transactions in packages).
//
// This leverages existing package validation infrastructure to properly
// classify and validate packages, reusing all the TRUC detection logic. A
// PackageContext and validated TxPackage are returned when validation succeeds,
// regardless of whether the package is classified as TRUC.
func (p *StandardPolicyEnforcer) BuildPackageContext(
	graph PolicyGraph,
	txs []*btcutil.Tx,
	descs []*txgraph.TxDesc,
) (*PackageContext, *txgraph.TxPackage, error) {

	if len(txs) == 0 || len(txs) != len(descs) {
		return nil, nil, fmt.Errorf("invalid package inputs: tx/descs length mismatch or zero")
	}

	// Enforce external submission limits (count, topology, weight).
	if err := validateSubmittedPackage(txs); err != nil {
		return nil, nil, err
	}

	// Build validation nodes for all transactions. This includes:
	// - Temporary nodes for new transactions
	// - Existing ancestors from mempool
	// - Siblings/descendants of those ancestors
	//
	// This gives us the complete package context for validation.
	allNodes := make([]*txgraph.TxGraphNode, 0)
	nodesSeen := make(map[chainhash.Hash]bool)

	for i, tx := range txs {
		// BuildPackageNodes returns all nodes needed to validate this tx,
		// including its mempool parents and their relationships.
		nodes := graph.BuildPackageNodes(tx, descs[i])

		// Deduplicate nodes across multiple transactions.
		for _, node := range nodes {
			if !nodesSeen[node.TxHash] {
				allNodes = append(allNodes, node)
				nodesSeen[node.TxHash] = true
			}
		}
	}

	if len(allNodes) == 0 {
		return nil, nil, fmt.Errorf("package context has no nodes")
	}

	// Use CreatePackage with dry run to validate and classify the package.
	// This reuses ALL the existing validation and classification logic:
	// - Topology validation
	// - Connectivity checks
	// - Package type classification (TRUC/ephemeral/1P1C)
	// - Fee rate calculation
	pkg, err := graph.CreatePackage(allNodes, txgraph.WithDryRun())
	if err != nil {
		return nil, nil, err
	}

	// Build package context for fee validation. The package fee rate and size
	// are provided even for non-TRUC packages so callers can enforce their own
	// limits or telemetry.
	ctx := &PackageContext{
		IsPartOfPackage: len(allNodes) > 1,
		PackageFeeRate:  pkg.FeeRate,
		PackageSize:     pkg.TotalSize,
		IsTRUCPackage:   pkg.Type == txgraph.PackageTypeTRUC,
	}

	return ctx, pkg, nil
}

// validateSubmittedPackage enforces external package submission limits:
//   - Package must be non-empty
//   - Transaction count must be <= MaxPackageCount (25)
//   - Package weight must be <= MaxPackageWeight (404000 wu)
//   - Package must be topologically sorted (parents before children)
func validateSubmittedPackage(txs []*btcutil.Tx) error {
	if len(txs) == 0 {
		return txRuleError(wire.RejectInvalid,
			"package must contain at least one transaction")
	}

	if len(txs) > MaxPackageCount {
		return txRuleError(wire.RejectInvalid,
			fmt.Sprintf("package contains %d transactions, max %d allowed",
				len(txs), MaxPackageCount))
	}

	// Build index for quick lookup of package membership/ordering.
	pkgTxs := make(map[chainhash.Hash]int, len(txs))
	for i, tx := range txs {
		pkgTxs[*tx.Hash()] = i
	}

	var totalVSize int64
	for i, tx := range txs {
		totalVSize += GetTxVirtualSize(tx)

		for _, txIn := range tx.MsgTx().TxIn {
			prevHash := txIn.PreviousOutPoint.Hash
			if parentIdx, inPackage := pkgTxs[prevHash]; inPackage {
				if parentIdx >= i {
					return txRuleError(wire.RejectInvalid,
						fmt.Sprintf("transaction %v at index %d references "+
							"transaction %v at index %d (not topologically sorted)",
							tx.Hash(), i, prevHash, parentIdx))
				}
			}
		}
	}

	totalWeight := totalVSize * 4
	if totalWeight > MaxPackageWeight {
		return txRuleError(wire.RejectInvalid,
			fmt.Sprintf("package weight %d exceeds max %d",
				totalWeight, MaxPackageWeight))
	}

	return nil
}

// ValidatePackageReplacement validates that a package of transactions can
// replace existing mempool transactions according to BIP 125 rules.
//
// Package RBF applies BIP 125 rules at the package level, comparing aggregate
// fees and fee rates rather than individual transaction metrics. This enables
// CPFP-based package replacements where the child pays for both transactions.
func (p *StandardPolicyEnforcer) ValidatePackageReplacement(
	graph PolicyGraph,
	txs []*btcutil.Tx,
	packageFee int64,
	allConflicts map[chainhash.Hash]*txgraph.ConflictSet,
) error {
	ctx := context.Background()

	// Aggregate all conflicts from all transactions in the package.
	allConflictTxs := make(map[chainhash.Hash]*txgraph.TxGraphNode)
	totalConflictCount := 0

	for _, conflicts := range allConflicts {
		for hash, node := range conflicts.Transactions {
			if _, exists := allConflictTxs[hash]; !exists {
				allConflictTxs[hash] = node
				totalConflictCount++
			}
		}
	}

	// If no conflicts, this isn't a replacement.
	if totalConflictCount == 0 {
		return nil
	}

	log.TraceS(ctx, "Validating package RBF replacement",
		"package_txs", len(txs),
		"total_conflicts", totalConflictCount,
		"package_fee", packageFee)

	// Rule 1: Check eviction limit at package level.
	if totalConflictCount > p.cfg.MaxReplacementEvictions {
		log.WarnS(ctx, "Package RBF eviction limit exceeded", nil,
			"conflicts_count", totalConflictCount,
			"max_evictions", p.cfg.MaxReplacementEvictions)
		return fmt.Errorf("%w: package replaces %d transactions (max %d)",
			ErrTooManyEvictions, totalConflictCount,
			p.cfg.MaxReplacementEvictions)
	}

	// Rule 2: Package must not spend outputs from any conflicts (except direct replacements).
	// Build set of all conflict hashes for quick lookup.
	conflictHashes := make(map[chainhash.Hash]bool)
	for hash := range allConflictTxs {
		conflictHashes[hash] = true
	}

	for _, tx := range txs {
		for _, txIn := range tx.MsgTx().TxIn {
			parentHash := txIn.PreviousOutPoint.Hash

			// Check if parent exists in graph.
			if _, exists := graph.GetNode(parentHash); exists {
				// Check if parent is a direct conflict (allowed).
				if conflictHashes[parentHash] {
					continue
				}

				// Check if any ancestors are conflicts (not allowed).
				ancestors := graph.GetAncestors(parentHash, -1)
				for ancestorHash := range ancestors {
					if conflictHashes[ancestorHash] {
						return fmt.Errorf("%w: package tx spends ancestor %v of conflict",
							ErrReplacementSpendsParent, ancestorHash)
					}
				}
			}
		}
	}

	// Rule 3: Package fee rate must be higher than each conflict's fee rate.
	var packageSize int64
	for _, tx := range txs {
		packageSize += GetTxVirtualSize(tx)
	}
	packageFeeRate := packageFee * 1000 / packageSize

	for conflictHash, conflictNode := range allConflictTxs {
		conflictFeeRate := conflictNode.TxDesc.Fee * 1000 /
			conflictNode.TxDesc.VirtualSize

		if packageFeeRate <= conflictFeeRate {
			return fmt.Errorf("%w: package fee rate %d sat/kB <= "+
				"conflict %v fee rate %d sat/kB",
				ErrInsufficientFeeRate, packageFeeRate, conflictHash,
				conflictFeeRate)
		}
	}

	// Rule 4: Package must pay for bandwidth (conflicts + relay fee).
	var conflictsFee int64
	for _, conflictNode := range allConflictTxs {
		conflictsFee += conflictNode.TxDesc.Fee
	}

	minRelayFee := calcMinRequiredTxRelayFee(packageSize, p.cfg.MinRelayTxFee)
	if packageFee < conflictsFee+minRelayFee {
		return fmt.Errorf("%w: package fee %d < conflicts fee %d + relay fee %d",
			ErrInsufficientAbsoluteFee, packageFee, conflictsFee, minRelayFee)
	}

	log.DebugS(ctx, "Package RBF validation passed",
		"package_fee", packageFee,
		"package_fee_rate", packageFeeRate,
		"conflicts_fee", conflictsFee,
		"evictions", totalConflictCount)

	return nil
}

// Ensure StandardPolicyEnforcer implements PolicyEnforcer interface.
var _ PolicyEnforcer = (*StandardPolicyEnforcer)(nil)
