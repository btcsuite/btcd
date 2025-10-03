// Copyright (c) 2013-2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
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
	ValidateRelayFee(tx *btcutil.Tx, fee int64, size int64,
		utxoView *blockchain.UtxoViewpoint, nextBlockHeight int32,
		isNew bool) error

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
func (p *StandardPolicyEnforcer) SignalsReplacement(
	graph PolicyGraph, tx *btcutil.Tx) bool {

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

	// Rule 1: Check eviction limit.
	if len(conflicts.Transactions) > p.cfg.MaxReplacementEvictions {
		return fmt.Errorf("%w: %d conflicts (max %d)",
			ErrTooManyEvictions, len(conflicts.Transactions),
			p.cfg.MaxReplacementEvictions)
	}

	// Rule 2: The replacement must not spend outputs from any of the
	// conflicts. Get all ancestors to check for overlap.
	ancestors := graph.GetAncestors(*tx.Hash(), -1)
	for conflictHash := range conflicts.Transactions {
		if _, exists := ancestors[conflictHash]; exists {
			return fmt.Errorf("%w: %v", ErrReplacementSpendsParent,
				conflictHash)
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

	// Get all ancestors for this transaction.
	ancestors := graph.GetAncestors(hash, -1)

	// Check ancestor count limit (includes the transaction itself).
	ancestorCount := len(ancestors) + 1
	if ancestorCount > p.cfg.MaxAncestorCount {
		return fmt.Errorf("%w: %d ancestors (max %d)",
			ErrExceededAncestorLimit, ancestorCount,
			p.cfg.MaxAncestorCount)
	}

	// Check ancestor size limit (includes the transaction itself).
	var ancestorSize int64
	for _, ancestor := range ancestors {
		ancestorSize += ancestor.TxDesc.VirtualSize
	}

	// Add this transaction's size.
	node, exists := graph.GetNode(hash)
	if exists {
		ancestorSize += node.TxDesc.VirtualSize
	}

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

	// Get all descendants for this transaction.
	descendants := graph.GetDescendants(hash, -1)

	// Check descendant count limit.
	descendantCount := len(descendants)
	if descendantCount > p.cfg.MaxDescendantCount {
		return fmt.Errorf("%w: %d descendants (max %d)",
			ErrExceededDescendantLimit, descendantCount,
			p.cfg.MaxDescendantCount)
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
func (p *StandardPolicyEnforcer) ValidateRelayFee(
	tx *btcutil.Tx, fee int64, size int64, utxoView *blockchain.UtxoViewpoint,
	nextBlockHeight int32, isNew bool) error {

	// First check the minimum relay fee and priority requirements using
	// the standalone CheckRelayFee function.
	err := CheckRelayFee(
		tx, fee, size, utxoView, nextBlockHeight,
		p.cfg.MinRelayTxFee, p.cfg.DisableRelayPriority, isNew,
	)
	if err != nil {
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

// Ensure StandardPolicyEnforcer implements PolicyEnforcer interface.
var _ PolicyEnforcer = (*StandardPolicyEnforcer)(nil)
