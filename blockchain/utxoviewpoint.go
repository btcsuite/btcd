// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// UtxoViewpoint represents a view into the set of unspent transaction outputs
// from a specific point of view in the chain.  For example, it could be for
// the end of the main chain, some point in the history of the main chain, or
// down a side chain.
//
// The unspent outputs are needed by other transactions for things such as
// script validation and double spend prevention.
type UtxoViewpoint struct {
	entries  map[wire.OutPoint]*UtxoEntry
	bestHash chainhash.Hash

	// getEntryByHashSource is used to fulfill the method getEntryByHash, which
	// is only used for chains with a legacy database.
	getEntryByHashSource *utxoCache
}

// LookupEntry returns information about a given transaction output according to
// the current state of the view.  It will return nil if the passed output does
// not exist in the view or is otherwise not available such as when it has been
// disconnected during a reorg.
func (view *UtxoViewpoint) LookupEntry(outpoint wire.OutPoint) *UtxoEntry {
	return view.entries[outpoint]
}

//TODO(stevenroose) copy documentation.
// This method is part of the utxoView interface.
func (view *UtxoViewpoint) getEntry(outpoint wire.OutPoint) (*UtxoEntry, error) {
	return view.LookupEntry(outpoint), nil
}

//TODO(stevenroose) copy documentation.
// This method is part of the utxoView interface.
func (view *UtxoViewpoint) addEntry(outpoint wire.OutPoint, entry *UtxoEntry, overwrite bool) error {
	view.entries[outpoint] = entry
	return nil
}

func (view *UtxoViewpoint) spendEntry(outpoint wire.OutPoint, putIfNil *UtxoEntry) error {
	// If we don't have the entry yet, add it.
	entry, found := view.entries[outpoint]
	if !found {
		entry = putIfNil
		view.entries[outpoint] = entry
	}

	// Then mark it as spent.
	entry.Spend()
	return nil
}

// addTxOut adds the specified output to the view if it is not provably
// unspendable.  When the view already has an entry for the output, it will be
// marked unspent.  All fields will be updated for existing entries since it's
// possible it has changed during a reorg.
func (view *UtxoViewpoint) addTxOut(outpoint wire.OutPoint, txOut *wire.TxOut, isCoinBase bool, blockHeight int32) {
	// Don't add provably unspendable outputs.
	if txscript.IsUnspendable(txOut.PkScript) {
		return
	}

	// Update existing entries.  All fields are updated because it's
	// possible (although extremely unlikely) that the existing entry is
	// being replaced by a different transaction with the same hash.  This
	// is allowed so long as the previous transaction is fully spent.
	entry := view.LookupEntry(outpoint)
	if entry == nil {
		entry = new(UtxoEntry)
		view.entries[outpoint] = entry
	}

	entry.amount = txOut.Value
	entry.pkScript = txOut.PkScript
	entry.blockHeight = blockHeight
	entry.packedFlags = tfModified
	if isCoinBase {
		entry.packedFlags |= tfCoinBase
	}
}

// AddTxOut adds the specified output of the passed transaction to the view if
// it exists and is not provably unspendable.  When the view already has an
// entry for the output, it will be marked unspent.  All fields will be updated
// for existing entries since it's possible it has changed during a reorg.
func (view *UtxoViewpoint) AddTxOut(tx *btcutil.Tx, txOutIdx uint32, blockHeight int32) {
	// Can't add an output for an out of bounds index.
	if txOutIdx >= uint32(len(tx.MsgTx().TxOut)) {
		return
	}

	// Update existing entries.  All fields are updated because it's
	// possible (although extremely unlikely) that the existing entry is
	// being replaced by a different transaction with the same hash.  This
	// is allowed so long as the previous transaction is fully spent.
	prevOut := wire.OutPoint{Hash: *tx.Hash(), Index: txOutIdx}
	txOut := tx.MsgTx().TxOut[txOutIdx]
	view.addTxOut(prevOut, txOut, IsCoinBase(tx), blockHeight)
}

// AddTxOuts adds all outputs in the passed transaction which are not provably
// unspendable to the view.  When the view already has entries for any of the
// outputs, they are simply marked unspent.  All fields will be updated for
// existing entries since it's possible it has changed during a reorg.
func (view *UtxoViewpoint) AddTxOuts(tx *btcutil.Tx, blockHeight int32) {
	// Loop all of the transaction outputs and add those which are not
	// provably unspendable.
	isCoinBase := IsCoinBase(tx)
	prevOut := wire.OutPoint{Hash: *tx.Hash()}
	for txOutIdx, txOut := range tx.MsgTx().TxOut {
		// Update existing entries.  All fields are updated because it's
		// possible (although extremely unlikely) that the existing
		// entry is being replaced by a different transaction with the
		// same hash.  This is allowed so long as the previous
		// transaction is fully spent.
		prevOut.Index = uint32(txOutIdx)
		view.addTxOut(prevOut, txOut, isCoinBase, blockHeight)
	}
}

// addInputUtxos adds the unspent transaction outputs for the inputs referenced
// by the transactions in the given block to the view.  In particular, referenced
// entries that are earlier in the block are added to the view and entries that
// are already in the view are not modified.
func (view *UtxoViewpoint) addInputUtxos(source utxoView, block *btcutil.Block) error {
	// Build a map of in-flight transactions because some of the inputs in
	// this block could be referencing other transactions earlier in this
	// block which are not yet in the chain.
	txInFlight := map[chainhash.Hash]int{}
	transactions := block.Transactions()
	for i, tx := range transactions {
		txInFlight[*tx.Hash()] = i
	}

	// Loop through all of the transaction inputs (except for the coinbase
	// which has no inputs) collecting them into sets of what is needed and
	// what is already known (in-flight).
	for i, tx := range transactions[1:] {
		for _, txIn := range tx.MsgTx().TxIn {
			// Don't do anything for entries that are already in the view.
			if _, ok := view.entries[txIn.PreviousOutPoint]; ok {
				continue
			}

			// It is acceptable for a transaction input to reference
			// the output of another transaction in this block only
			// if the referenced transaction comes before the
			// current one in this block.  Add the outputs of the
			// referenced transaction as available utxos when this
			// is the case.  Otherwise, the utxo details are still
			// needed.
			//
			// NOTE: The >= is correct here because i is one less
			// than the actual position of the transaction within
			// the block due to skipping the coinbase.
			originHash := &txIn.PreviousOutPoint.Hash
			if inFlightIndex, ok := txInFlight[*originHash]; ok &&
				i >= inFlightIndex {

				originTx := transactions[inFlightIndex]
				view.AddTxOuts(originTx, block.Height())
				continue
			}

			// Add the entry from the source.
			entry, err := source.getEntry(txIn.PreviousOutPoint)
			if err == nil && entry != nil {
				view.entries[txIn.PreviousOutPoint] = entry.Clone()
			}
		}
	}

	return nil
}

// connectTransaction updates the view by adding all new utxos created by the
// passed transaction and marking all utxos that the transactions spend as
// spent.  In addition, when the 'stxos' argument is not nil, it will be updated
// to append an entry for each spent txout.  An error will be returned if the
// view does not contain the required utxos.  Set overwrite to true of new
// entries should be allowed to overwrite existing not-fully-spent entries.
func connectTransaction(view utxoView, tx *btcutil.Tx, blockHeight int32,
	stxos *[]SpentTxOut, overwrite bool) error {

	// Skip input processing when tx is coinbase.
	if !IsCoinBase(tx) {
		// Spend the referenced utxos by marking them spent in the view and,
		// if a slice was provided for the spent txout details, append an entry
		// to it.
		for _, txIn := range tx.MsgTx().TxIn {
			// Ensure the referenced utxo exists in the view.  This should
			// never happen unless there is a bug is introduced in the code.
			entry, err := view.getEntry(txIn.PreviousOutPoint)
			if err != nil {
				return err
			}
			if entry == nil {
				return AssertError(fmt.Sprintf("view missing input %v",
					txIn.PreviousOutPoint))
			}

			// Only create the stxo details if requested.
			if stxos != nil {
				// Populate the stxo details using the utxo entry.
				var stxo = SpentTxOut{
					Amount:     entry.Amount(),
					PkScript:   entry.PkScript(),
					Height:     entry.BlockHeight(),
					IsCoinBase: entry.IsCoinBase(),
				}
				*stxos = append(*stxos, stxo)
			}

			// Mark the entry as spent.
			if err := view.spendEntry(txIn.PreviousOutPoint, entry); err != nil {
				return err
			}
		}
	}

	// Add the transaction's outputs as available utxos.
	isCoinBase := IsCoinBase(tx)
	prevOut := wire.OutPoint{Hash: *tx.Hash()}
	for txOutIdx, txOut := range tx.MsgTx().TxOut {
		prevOut.Index = uint32(txOutIdx)

		// Don't add provably unspendable outputs.
		if txscript.IsUnspendable(txOut.PkScript) {
			continue
		}

		// Create a new entry from the output.
		entry := &UtxoEntry{
			amount:      txOut.Value,
			pkScript:    txOut.PkScript,
			blockHeight: blockHeight,
			packedFlags: tfModified,
		}
		if isCoinBase {
			entry.packedFlags |= tfCoinBase
		}
		if !overwrite {
			// If overwrite is false (i.e. we are not replaying blocks in
			// recovery mode), this entry is fresh, meaning it can be pruned when
			// it gets spent before the next flush.
			entry.packedFlags |= tfFresh
		}

		// Add entry to the view.
		if err := view.addEntry(prevOut, entry, overwrite); err != nil {
			return err
		}
	}
	return nil
}

// connectTransactions updates the view by adding all new utxos created by all
// of the transactions in the passed block, marking all utxos the transactions
// spend as spent, and setting the best hash for the view to the passed block.
// In addition, when the 'stxos' argument is not nil, it will be updated to
// append an entry for each spent txout.  Set overwrite to true of new
// entries should be allowed to overwrite existing not-fully-spent entries.
func connectTransactions(view utxoView, block *btcutil.Block,
	stxos *[]SpentTxOut, overwrite bool) error {

	for _, tx := range block.Transactions() {
		err := connectTransaction(view, tx, block.Height(), stxos, overwrite)
		if err != nil {
			return err
		}
	}

	return nil
}

// disconnectTransactions updates the view by removing all of the transactions
// created by the passed block, restoring all utxos the transactions spent by
// using the provided spent txo information, and setting the best hash for the
// view to the block before the passed block.
func disconnectTransactions(view utxoView, block *btcutil.Block,
	stxos []SpentTxOut, byHashSource utxoByHashSource) error {

	// Sanity check the correct number of stxos are provided.
	if len(stxos) != countSpentOutputs(block) {
		return AssertError("disconnectTransactions called with bad " +
			"spent transaction out information")
	}

	// Loop backwards through all transactions so everything is unspent in
	// reverse order.  This is necessary since transactions later in a block
	// can spend from previous ones.
	stxoIdx := len(stxos) - 1
	transactions := block.Transactions()
	for txIdx := len(transactions) - 1; txIdx > -1; txIdx-- {
		tx := transactions[txIdx]

		// All entries will need to potentially be marked as a coinbase.
		var packedFlags txoFlags
		isCoinBase := txIdx == 0
		if isCoinBase {
			packedFlags |= tfCoinBase
		}

		// Mark all of the spendable outputs originally created by the
		// transaction as spent.  It is instructive to note that while
		// the outputs aren't actually being spent here, rather they no
		// longer exist, since a pruned utxo set is used, there is no
		// practical difference between a utxo that does not exist and
		// one that has been spent.
		//
		// When the utxo does not already exist in the view, add an
		// entry for it and then mark it spent.  This is done because
		// the code relies on its existence in the view in order to
		// signal modifications have happened.
		txHash := tx.Hash()
		prevOut := wire.OutPoint{Hash: *txHash}
		for txOutIdx, txOut := range tx.MsgTx().TxOut {
			if txscript.IsUnspendable(txOut.PkScript) {
				continue
			}

			prevOut.Index = uint32(txOutIdx)

			// Mark the entry as spent.  To make sure the view has the entry,
			// create one to pass along.
			entry := &UtxoEntry{
				amount:      txOut.Value,
				pkScript:    txOut.PkScript,
				blockHeight: block.Height(),
				packedFlags: packedFlags,
			}
			if err := view.spendEntry(prevOut, entry); err != nil {
				return err
			}
		}

		// Loop backwards through all of the transaction inputs (except
		// for the coinbase which has no inputs) and unspend the
		// referenced txos.  This is necessary to match the order of the
		// spent txout entries.
		if isCoinBase {
			continue
		}
		for txInIdx := len(tx.MsgTx().TxIn) - 1; txInIdx > -1; txInIdx-- {
			originOut := tx.MsgTx().TxIn[txInIdx].PreviousOutPoint

			// Ensure the spent txout index is decremented to stay
			// in sync with the transaction input.
			stxo := &stxos[stxoIdx]
			stxoIdx--

			// The legacy v1 spend journal format only stored the
			// coinbase flag and height when the output was the last
			// unspent output of the transaction.  As a result, when
			// the information is missing, search for it by scanning
			// all possible outputs of the transaction since it must
			// be in one of them.
			//
			// It should be noted that this is quite inefficient,
			// but it realistically will almost never run since all
			// new entries include the information for all outputs
			// and thus the only way this will be hit is if a long
			// enough reorg happens such that a block with the old
			// spend data is being disconnected.  The probability of
			// that in practice is extremely low to begin with and
			// becomes vanishingly small the more new blocks are
			// connected.  In the case of a fresh database that has
			// only ever run with the new v2 format, this code path
			// will never run.
			if stxo.Height == 0 {
				utxo, err := byHashSource.getEntryByHash(txHash)
				if err != nil {
					return err
				}
				if utxo == nil {
					return AssertError(fmt.Sprintf("unable "+
						"to resurrect legacy stxo %v",
						originOut))
				}

				stxo.Height = utxo.BlockHeight()
				stxo.IsCoinBase = utxo.IsCoinBase()
			}

			// Restore the utxo using the stxo data from the spend
			// journal and mark it as modified.
			entry := &UtxoEntry{
				amount:      stxo.Amount,
				pkScript:    stxo.PkScript,
				blockHeight: stxo.Height,
				packedFlags: tfModified,
			}
			if stxo.IsCoinBase {
				entry.packedFlags |= tfCoinBase
			}

			// Then store the entry in the view.
			if err := view.addEntry(originOut, entry, true); err != nil {
				return err
			}
		}
	}

	return nil
}

// RemoveEntry removes the given transaction output from the current state of
// the view.  It will have no effect if the passed output does not exist in the
// view.
func (view *UtxoViewpoint) RemoveEntry(outpoint wire.OutPoint) {
	delete(view.entries, outpoint)
}

// Entries returns the underlying map that stores of all the utxo entries.
func (view *UtxoViewpoint) Entries() map[wire.OutPoint]*UtxoEntry {
	return view.entries
}

// prune prunes all entries marked modified that are now fully spent and marks
// all entries as unmodified.
func (view *UtxoViewpoint) prune() {
	for outpoint, entry := range view.entries {
		if entry == nil || (entry.isModified() && entry.IsSpent()) {
			delete(view.entries, outpoint)
			continue
		}

		entry.packedFlags ^= tfModified
	}
}

// NewUtxoViewpoint returns a new empty unspent transaction output view.
func NewUtxoViewpoint() *UtxoViewpoint {
	return &UtxoViewpoint{
		entries: make(map[wire.OutPoint]*UtxoEntry),
	}
}
