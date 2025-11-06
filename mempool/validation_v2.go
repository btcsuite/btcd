// Copyright (c) 2013-2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"context"
	"fmt"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mining"
	"github.com/btcsuite/btcd/wire"
)

// checkMempoolAcceptance performs a series of validations on the given
// transaction for TxMempoolV2. It returns an error when the transaction fails
// to meet mempool policy, otherwise a MempoolAcceptResult is returned.
//
// This is a standalone implementation for TxMempoolV2 that uses the graph for
// conflict detection and the policy enforcer for RBF validation. It does NOT
// modify the old TxPool implementation.
//
// The optional packageContext enables BIP 431 Rule 6 support: TRUC transactions
// may be below minimum relay fee when part of a valid package.
func (mp *TxMempoolV2) checkMempoolAcceptance(
	tx *btcutil.Tx,
	isNew, rateLimit, rejectDupOrphans bool,
	packageContext *PackageContext,
) (*MempoolAcceptResult, error) {

	txHash := tx.Hash()

	// Check for segwit activeness using policy enforcer.
	if err := mp.policy.ValidateSegWitDeployment(tx); err != nil {
		return nil, err
	}

	// Don't accept the transaction if it already exists in the pool. This
	// applies to orphan transactions as well when the reject duplicate
	// orphans flag is set. This check is intended to be a quick check to
	// weed out duplicates.
	if mp.graph.HasTransaction(*txHash) ||
		(rejectDupOrphans && mp.orphanMgr.IsOrphan(*txHash)) {

		str := fmt.Sprintf("already have transaction in mempool %v",
			txHash)
		return nil, txRuleError(wire.RejectDuplicate, str)
	}

	// Disallow transactions under the minimum standardness size.
	if tx.MsgTx().SerializeSizeStripped() < MinStandardTxNonWitnessSize {
		str := fmt.Sprintf("tx %v is too small", txHash)
		return nil, txRuleError(wire.RejectNonstandard, str)
	}

	// Perform preliminary sanity checks on the transaction using the
	// validator interface. This validates invariant rules that don't
	// require blockchain context.
	if err := mp.txValidator.ValidateSanity(tx); err != nil {
		return nil, err
	}

	// A standalone transaction must not be a coinbase transaction.
	if blockchain.IsCoinBase(tx) {
		str := fmt.Sprintf("transaction is an individual coinbase %v",
			txHash)
		return nil, txRuleError(wire.RejectInvalid, str)
	}

	// Get the current height of the main chain. A standalone transaction
	// will be mined into the next block at best, so its height is at least
	// one more than the current height.
	bestHeight := mp.cfg.BestHeight()
	nextBlockHeight := bestHeight + 1

	medianTimePast := mp.cfg.MedianTimePast()

	// The transaction may not use any of the same outputs as other
	// transactions already in the pool as that would ultimately result in
	// a double spend, unless those transactions signal for RBF.
	//
	// For TxMempoolV2, we use graph.GetConflicts() instead of the old
	// checkPoolDoubleSpend() method.
	conflicts := mp.graph.GetConflicts(tx)
	isReplacement := len(conflicts.Transactions) > 0

	// Fetch UTXO view including both confirmed and unconfirmed outputs.
	utxoView, err := mp.fetchInputUtxos(tx)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Check UTXO availability: verify the transaction doesn't already
	// exist in the blockchain and identify any missing parent transactions.
	missingParents, err := mp.txValidator.ValidateUtxoAvailability(tx, utxoView)
	if err != nil {
		return nil, err
	}

	// Exit early if this transaction is missing parents (it's an orphan).
	if len(missingParents) > 0 {
		ctx := context.Background()
		log.DebugS(ctx, "Transaction has missing parents",
			"tx_hash", txHash,
			"missing_count", len(missingParents))

		return &MempoolAcceptResult{
			MissingParents: missingParents,
		}, nil
	}

	// Perform several checks on the transaction inputs using the validator
	// interface. This validates that inputs exist, are unspent, and
	// calculates the transaction fee.
	//
	// NOTE: this check must be performed before `validateStandardness` to
	// make sure a nil entry is not returned from `utxoView.LookupEntry`.
	txFee, err := mp.txValidator.ValidateInputs(tx, nextBlockHeight, utxoView)
	if err != nil {
		return nil, err
	}

	// Don't allow non-standard transactions or non-standard inputs if the
	// network parameters forbid their acceptance.
	err = mp.policy.ValidateStandardness(
		tx, nextBlockHeight, medianTimePast, utxoView,
	)
	if err != nil {
		return nil, err
	}

	// Don't allow the transaction into the mempool unless its sequence
	// lock is active, meaning that it'll be allowed into the next block
	// with respect to its defined relative lock times.
	err = mp.txValidator.ValidateSequenceLocks(
		tx, utxoView, nextBlockHeight, medianTimePast,
	)
	if err != nil {
		return nil, err
	}

	// Don't allow transactions with an excessive number of signature
	// operations which would result in making it impossible to mine.
	if err := mp.policy.ValidateSigCost(tx, utxoView); err != nil {
		return nil, err
	}

	txSize := GetTxVirtualSize(tx)

	// For TxMempoolV2, use the policy enforcer's ValidateRelayFee method
	// instead of the old validateRelayFeeMet. Pass packageContext to enable
	// BIP 431 Rule 6 (zero-fee TRUC transactions in packages).
	err = mp.policy.ValidateRelayFee(
		tx, txFee, txSize, utxoView, nextBlockHeight, isNew, packageContext,
	)
	if err != nil {
		return nil, err
	}

	// If the transaction has any conflicts, and we've made it this far,
	// then we're processing a potential replacement. Use the policy
	// enforcer to validate RBF rules.
	if isReplacement {
		ctx := context.Background()
		log.DebugS(ctx, "Processing potential RBF replacement",
			"tx_hash", txHash,
			"conflicts_count", len(conflicts.Transactions))

		// Check if transaction signals replacement (explicit or inherited).
		if !mp.policy.SignalsReplacement(mp.graph, tx) {
			str := fmt.Sprintf("transaction %v spends outputs "+
				"already spent by mempool transaction without "+
				"signaling replacement (BIP 125)", txHash)
			return nil, txRuleError(wire.RejectDuplicate, str)
		}

		log.DebugS(ctx, "Calling ValidateReplacement",
			"tx_hash", txHash)

		// Validate replacement according to BIP 125 rules.
		err = mp.policy.ValidateReplacement(
			mp.graph, tx, txFee, conflicts,
		)
		if err != nil {
			log.DebugS(ctx, "ValidateReplacement failed",
				"tx_hash", txHash,
				"error", err.Error())
			return nil, err
		}

		log.DebugS(ctx, "ValidateReplacement succeeded",
			"tx_hash", txHash)
	}

	// Verify crypto signatures for each input and reject the transaction
	// if any don't verify.
	err = mp.txValidator.ValidateScripts(tx, utxoView)
	if err != nil {
		return nil, err
	}

	// Convert ConflictSet to old format for compatibility.
	conflictMap := make(map[chainhash.Hash]*btcutil.Tx)
	for hash, node := range conflicts.Transactions {
		conflictMap[hash] = node.Tx
	}

	result := &MempoolAcceptResult{
		TxFee:      btcutil.Amount(txFee),
		TxSize:     txSize,
		Conflicts:  conflictMap,
		utxoView:   utxoView,
		bestHeight: bestHeight,
	}

	return result, nil
}

// fetchInputUtxos loads UTXO details for transaction inputs from both the
// blockchain and the mempool. This matches TxPool behavior and enables
// parent-child transaction chains where children spend unconfirmed parent
// outputs.
//
// First it fetches from the blockchain, then augments with unconfirmed outputs
// from the mempool graph. This is essential for CPFP, package relay, and TRUC
// validation.
func (mp *TxMempoolV2) fetchInputUtxos(tx *btcutil.Tx) (*blockchain.UtxoViewpoint, error) {
	// Fetch blockchain UTXOs.
	utxoView, err := mp.cfg.FetchUtxoView(tx)
	if err != nil {
		return nil, err
	}

	// Augment with unconfirmed outputs from mempool graph.
	for _, txIn := range tx.MsgTx().TxIn {
		prevOut := &txIn.PreviousOutPoint
		entry := utxoView.LookupEntry(*prevOut)
		if entry != nil && !entry.IsSpent() {
			continue
		}

		// Check if parent exists in mempool.
		if parentNode, exists := mp.graph.GetNode(prevOut.Hash); exists {
			utxoView.AddTxOut(parentNode.Tx, prevOut.Index, mining.UnminedHeight)
		}
	}

	return utxoView, nil
}
