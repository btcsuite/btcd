// Copyright (c) 2013-2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"context"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// TxValidator defines the interface for validating Bitcoin transactions at the
// consensus and policy level. This separates low-level validation (scripts,
// timelocks) from policy enforcement (fees, limits), enabling different
// validation strategies and easier testing.
//
// Unlike PolicyEnforcer which handles mempool-specific policies like RBF and
// ancestor limits, TxValidator focuses on fundamental transaction validity:
// cryptographic signatures and timelock consensus rules.
type TxValidator interface {
	// ValidateSanity performs preliminary sanity checks on a transaction
	// without requiring any blockchain context. This validates invariant
	// rules like non-negative outputs and valid script format.
	ValidateSanity(tx *btcutil.Tx) error

	// ValidateUtxoAvailability checks UTXO availability for a transaction:
	// 1. Verifies the transaction's outputs don't already exist in the
	//    blockchain (duplicate transaction detection).
	// 2. Identifies which inputs are missing (orphan detection).
	//
	// The utxoView is modified by removing the transaction's own outputs
	// so subsequent iterations only examine inputs.
	//
	// Returns:
	//   - missingParents: List of parent transaction hashes with missing
	//     outputs. Non-empty indicates this is an orphan transaction.
	//   - error: Returns error if transaction already exists in blockchain.
	ValidateUtxoAvailability(tx *btcutil.Tx,
		utxoView *blockchain.UtxoViewpoint,
	) (missingParents []*chainhash.Hash, err error)

	// ValidateInputs performs blockchain context-aware input validation,
	// checking that inputs exist, are unspent, and calculating transaction
	// fees. Returns the transaction fee on success.
	ValidateInputs(tx *btcutil.Tx, nextBlockHeight int32,
		utxoView *blockchain.UtxoViewpoint) (int64, error)

	// ValidateScripts verifies the cryptographic validity of all input
	// scripts in a transaction. This is the most expensive validation
	// operation and should be performed last after all policy checks pass.
	ValidateScripts(tx *btcutil.Tx,
		utxoView *blockchain.UtxoViewpoint) error

	// ValidateSequenceLocks checks that a transaction's relative timelocks
	// (BIP 68) are satisfied and it can be included in the next block.
	ValidateSequenceLocks(tx *btcutil.Tx, utxoView *blockchain.UtxoViewpoint,
		nextBlockHeight int32, medianTimePast time.Time) error
}

// TxValidatorConfig defines the configuration for transaction validation.
type TxValidatorConfig struct {
	// SigCache caches signature verification results to avoid redundant
	// expensive ECDSA operations.
	SigCache *txscript.SigCache

	// HashCache caches transaction sighash computations for performance.
	HashCache *txscript.HashCache

	// CalcSequenceLock is the function to use for calculating sequence
	// locks. This is typically provided by the blockchain package.
	CalcSequenceLock func(*btcutil.Tx,
		*blockchain.UtxoViewpoint) (*blockchain.SequenceLock, error)

	// ChainParams identifies the blockchain network for validation rules.
	ChainParams *chaincfg.Params
}

// StandardTxValidator implements TxValidator using Bitcoin Core's validation
// rules. This is the standard validator used by both TxPool and TxMempoolV2.
type StandardTxValidator struct {
	cfg TxValidatorConfig
}

// NewStandardTxValidator creates a new standard transaction validator with the
// given configuration.
func NewStandardTxValidator(cfg TxValidatorConfig) *StandardTxValidator {
	return &StandardTxValidator{
		cfg: cfg,
	}
}

// ValidateScripts verifies crypto signatures for each input and rejects the
// transaction if any don't verify.
func (v *StandardTxValidator) ValidateScripts(
	tx *btcutil.Tx,
	utxoView *blockchain.UtxoViewpoint,
) error {
	ctx := context.Background()
	log.TraceS(ctx, "Validating transaction scripts",
		"tx_hash", tx.Hash(),
		"input_count", len(tx.MsgTx().TxIn))

	// Use blockchain's script validation with standard verification flags.
	err := blockchain.ValidateTransactionScripts(
		tx, utxoView, txscript.StandardVerifyFlags, v.cfg.SigCache,
		v.cfg.HashCache,
	)
	if err != nil {
		log.DebugS(ctx, "Script validation failed",
			"tx_hash", tx.Hash(),
			"reason", err.Error())
		if cerr, ok := err.(blockchain.RuleError); ok {
			return chainRuleError(cerr)
		}
		return err
	}

	return nil
}

// ValidateSequenceLocks validates that a transaction's sequence locks are
// active, meaning the transaction can be included in the next block with
// respect to its defined relative lock times (BIP 68).
func (v *StandardTxValidator) ValidateSequenceLocks(
	tx *btcutil.Tx, utxoView *blockchain.UtxoViewpoint,
	nextBlockHeight int32, medianTimePast time.Time) error {

	// Use the CheckSequenceLocks helper which wraps the blockchain package
	// sequence lock calculation and validation.
	return CheckSequenceLocks(
		tx, utxoView, nextBlockHeight, medianTimePast,
		v.cfg.CalcSequenceLock,
	)
}

// ValidateSanity performs preliminary sanity checks on the transaction without
// requiring any blockchain context.
func (v *StandardTxValidator) ValidateSanity(tx *btcutil.Tx) error {
	err := blockchain.CheckTransactionSanity(tx)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return chainRuleError(cerr)
		}
		return err
	}
	return nil
}

// ValidateUtxoAvailability checks UTXO availability and identifies missing
// parents (orphan detection).
func (v *StandardTxValidator) ValidateUtxoAvailability(tx *btcutil.Tx,
	utxoView *blockchain.UtxoViewpoint) ([]*chainhash.Hash, error) {

	txHash := tx.Hash()

	// Don't allow the transaction if it exists in the main chain and is
	// not already fully spent.
	prevOut := wire.OutPoint{Hash: *txHash}
	for txOutIdx := range tx.MsgTx().TxOut {
		prevOut.Index = uint32(txOutIdx)

		entry := utxoView.LookupEntry(prevOut)
		if entry != nil && !entry.IsSpent() {
			return nil, txRuleError(wire.RejectDuplicate,
				"transaction already exists in blockchain")
		}

		utxoView.RemoveEntry(prevOut)
	}

	// Transaction is an orphan if any of the referenced transaction
	// outputs don't exist or are already spent.
	var missingParents []*chainhash.Hash
	for outpoint, entry := range utxoView.Entries() {
		if entry == nil || entry.IsSpent() {
			// Must make a copy of the hash here since the iterator
			// is replaced and taking its address directly would
			// result in all the entries pointing to the same
			// memory location and thus all be the final hash.
			hashCopy := outpoint.Hash
			missingParents = append(missingParents, &hashCopy)
		}
	}

	return missingParents, nil
}

// ValidateInputs performs blockchain context-aware input validation and returns
// the transaction fee.
func (v *StandardTxValidator) ValidateInputs(tx *btcutil.Tx,
	nextBlockHeight int32,
	utxoView *blockchain.UtxoViewpoint) (int64, error) {

	txFee, err := blockchain.CheckTransactionInputs(
		tx, nextBlockHeight, utxoView, v.cfg.ChainParams,
	)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return 0, chainRuleError(cerr)
		}
		return 0, err
	}

	return txFee, nil
}
