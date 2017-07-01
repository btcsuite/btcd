// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcutil"
)

// utxoOutput houses details about an individual unspent transaction output such
// as whether or not it is spent, its public key script, and how much it pays.
//
// Standard public key scripts are stored in the database using a compressed
// format. Since the vast majority of scripts are of the standard form, a fairly
// significant savings is achieved by discarding the portions of the standard
// scripts that can be reconstructed.
//
// Also, since it is common for only a specific output in a given utxo entry to
// be referenced from a redeeming transaction, the script and amount for a given
// output is not uncompressed until the first time it is accessed.  This
// provides a mechanism to avoid the overhead of needlessly uncompressing all
// outputs for a given utxo entry at the time of load.
type utxoOutput struct {
	spent      bool   // Output is spent.
	compressed bool   // The amount and public key script are compressed.
	amount     int64  // The amount of the output.
	pkScript   []byte // The public key script for the output.
}

// maybeDecompress decompresses the amount and public key script fields of the
// utxo and marks it decompressed if needed.
func (o *utxoOutput) maybeDecompress(version int32) {
	// Nothing to do if it's not compressed.
	if !o.compressed {
		return
	}

	o.amount = int64(decompressTxOutAmount(uint64(o.amount)))
	o.pkScript = decompressScript(o.pkScript, version)
	o.compressed = false
}

// UtxoEntry contains contextual information about an unspent transaction such
// as whether or not it is a coinbase transaction, which block it was found in,
// and the spent status of its outputs.
type UtxoEntry struct {
	modified      bool                   // Entry changed since load.
	version       int32                  // The version of this tx.
	isCoinBase    bool                   // Whether entry is a coinbase tx.
	blockHeight   int32                  // Height of block containing tx.
	sparseOutputs map[uint32]*utxoOutput // Sparse map of unspent outputs.
}

// Version returns the version of the transaction the utxo represents.
func (entry *UtxoEntry) Version() int32 {
	return entry.version
}

// IsCoinBase returns whether or not the transaction the utxo entry represents
// is a coinbase.
func (entry *UtxoEntry) IsCoinBase() bool {
	return entry.isCoinBase
}

// BlockHeight returns the height of the block containing the transaction the
// utxo entry represents.
func (entry *UtxoEntry) BlockHeight() int32 {
	return entry.blockHeight
}

// IsOutputSpent returns whether or not the provided output index has been
// spent based upon the current state of the unspent transaction output view
// the entry was obtained from.
//
// Returns true if the output index references an output that does not exist
// either due to it being invalid or because the output is not part of the view
// due to previously being spent/pruned.
func (entry *UtxoEntry) IsOutputSpent(outputIndex uint32) bool {
	output, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return true
	}

	return output.spent
}

// SpendOutput marks the output at the provided index as spent.  Specifying an
// output index that does not exist will not have any effect.
func (entry *UtxoEntry) SpendOutput(outputIndex uint32) {
	output, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return
	}

	// Nothing to do if the output is already spent.
	if output.spent {
		return
	}

	entry.modified = true
	output.spent = true
}

// IsFullySpent returns whether or not the transaction the utxo entry represents
// is fully spent.
func (entry *UtxoEntry) IsFullySpent() bool {
	// The entry is not fully spent if any of the outputs are unspent.
	for _, output := range entry.sparseOutputs {
		if !output.spent {
			return false
		}
	}

	return true
}

// AmountByIndex returns the amount of the provided output index.
//
// Returns 0 if the output index references an output that does not exist
// either due to it being invalid or because the output is not part of the view
// due to previously being spent/pruned.
func (entry *UtxoEntry) AmountByIndex(outputIndex uint32) int64 {
	output, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return 0
	}

	// Ensure the output is decompressed before returning the amount.
	output.maybeDecompress(entry.version)
	return output.amount
}

// PkScriptByIndex returns the public key script for the provided output index.
//
// Returns nil if the output index references an output that does not exist
// either due to it being invalid or because the output is not part of the view
// due to previously being spent/pruned.
func (entry *UtxoEntry) PkScriptByIndex(outputIndex uint32) []byte {
	output, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return nil
	}

	// Ensure the output is decompressed before returning the script.
	output.maybeDecompress(entry.version)
	return output.pkScript
}

// Clone returns a deep copy of the utxo entry.
func (entry *UtxoEntry) Clone() *UtxoEntry {
	if entry == nil {
		return nil
	}

	newEntry := &UtxoEntry{
		version:       entry.version,
		isCoinBase:    entry.isCoinBase,
		blockHeight:   entry.blockHeight,
		sparseOutputs: make(map[uint32]*utxoOutput),
	}
	for outputIndex, output := range entry.sparseOutputs {
		newEntry.sparseOutputs[outputIndex] = &utxoOutput{
			spent:      output.spent,
			compressed: output.compressed,
			amount:     output.amount,
			pkScript:   output.pkScript,
		}
	}
	return newEntry
}

// newUtxoEntry returns a new unspent transaction output entry with the provided
// coinbase flag and block height ready to have unspent outputs added.
func newUtxoEntry(version int32, isCoinBase bool, blockHeight int32) *UtxoEntry {
	return &UtxoEntry{
		version:       version,
		isCoinBase:    isCoinBase,
		blockHeight:   blockHeight,
		sparseOutputs: make(map[uint32]*utxoOutput),
	}
}

// UtxoViewpoint represents a view into the set of unspent transaction outputs
// from a specific point of view in the chain.  For example, it could be for
// the end of the main chain, some point in the history of the main chain, or
// down a side chain.
//
// The unspent outputs are needed by other transactions for things such as
// script validation and double spend prevention.
type UtxoViewpoint struct {
	entries  map[chainhash.Hash]*UtxoEntry
	bestHash chainhash.Hash
}

// BestHash returns the hash of the best block in the chain the view currently
// respresents.
func (view *UtxoViewpoint) BestHash() *chainhash.Hash {
	return &view.bestHash
}

// SetBestHash sets the hash of the best block in the chain the view currently
// respresents.
func (view *UtxoViewpoint) SetBestHash(hash *chainhash.Hash) {
	view.bestHash = *hash
}

// LookupEntry returns information about a given transaction according to the
// current state of the view.  It will return nil if the passed transaction
// hash does not exist in the view or is otherwise not available such as when
// it has been disconnected during a reorg.
func (view *UtxoViewpoint) LookupEntry(txHash *chainhash.Hash) *UtxoEntry {
	entry, ok := view.entries[*txHash]
	if !ok {
		return nil
	}

	return entry
}

// AddTxOuts adds all outputs in the passed transaction which are not provably
// unspendable to the view.  When the view already has entries for any of the
// outputs, they are simply marked unspent.  All fields will be updated for
// existing entries since it's possible it has changed during a reorg.
func (view *UtxoViewpoint) AddTxOuts(tx *btcutil.Tx, blockHeight int32) {
	// When there are not already any utxos associated with the transaction,
	// add a new entry for it to the view.
	entry := view.LookupEntry(tx.Hash())
	if entry == nil {
		entry = newUtxoEntry(tx.MsgTx().Version, IsCoinBase(tx),
			blockHeight)
		view.entries[*tx.Hash()] = entry
	} else {
		entry.blockHeight = blockHeight
	}
	entry.modified = true

	// Loop all of the transaction outputs and add those which are not
	// provably unspendable.
	for txOutIdx, txOut := range tx.MsgTx().TxOut {
		if txscript.IsUnspendable(txOut.PkScript) {
			continue
		}

		// Update existing entries.  All fields are updated because it's
		// possible (although extremely unlikely) that the existing
		// entry is being replaced by a different transaction with the
		// same hash.  This is allowed so long as the previous
		// transaction is fully spent.
		if output, ok := entry.sparseOutputs[uint32(txOutIdx)]; ok {
			output.spent = false
			output.compressed = false
			output.amount = txOut.Value
			output.pkScript = txOut.PkScript
			continue
		}

		// Add the unspent transaction output.
		entry.sparseOutputs[uint32(txOutIdx)] = &utxoOutput{
			spent:      false,
			compressed: false,
			amount:     txOut.Value,
			pkScript:   txOut.PkScript,
		}
	}
}

// connectTransaction updates the view by adding all new utxos created by the
// passed transaction and marking all utxos that the transactions spend as
// spent.  In addition, when the 'stxos' argument is not nil, it will be updated
// to append an entry for each spent txout.  An error will be returned if the
// view does not contain the required utxos.
func (view *UtxoViewpoint) connectTransaction(tx *btcutil.Tx, blockHeight int32, stxos *[]spentTxOut) error {
	// Coinbase transactions don't have any inputs to spend.
	if IsCoinBase(tx) {
		// Add the transaction's outputs as available utxos.
		view.AddTxOuts(tx, blockHeight)
		return nil
	}

	// Spend the referenced utxos by marking them spent in the view and,
	// if a slice was provided for the spent txout details, append an entry
	// to it.
	for _, txIn := range tx.MsgTx().TxIn {
		originIndex := txIn.PreviousOutPoint.Index
		entry := view.entries[txIn.PreviousOutPoint.Hash]

		// Ensure the referenced utxo exists in the view.  This should
		// never happen unless there is a bug is introduced in the code.
		if entry == nil {
			return AssertError(fmt.Sprintf("view missing input %v",
				txIn.PreviousOutPoint))
		}
		entry.SpendOutput(originIndex)

		// Don't create the stxo details if not requested.
		if stxos == nil {
			continue
		}

		// Populate the stxo details using the utxo entry.  When the
		// transaction is fully spent, set the additional stxo fields
		// accordingly since those details will no longer be available
		// in the utxo set.
		var stxo = spentTxOut{
			compressed: false,
			version:    entry.Version(),
			amount:     entry.AmountByIndex(originIndex),
			pkScript:   entry.PkScriptByIndex(originIndex),
		}
		if entry.IsFullySpent() {
			stxo.height = entry.BlockHeight()
			stxo.isCoinBase = entry.IsCoinBase()
		}

		// Append the entry to the provided spent txouts slice.
		*stxos = append(*stxos, stxo)
	}

	// Add the transaction's outputs as available utxos.
	view.AddTxOuts(tx, blockHeight)
	return nil
}

// connectTransactions updates the view by adding all new utxos created by all
// of the transactions in the passed block, marking all utxos the transactions
// spend as spent, and setting the best hash for the view to the passed block.
// In addition, when the 'stxos' argument is not nil, it will be updated to
// append an entry for each spent txout.
func (view *UtxoViewpoint) connectTransactions(block *btcutil.Block, stxos *[]spentTxOut) error {
	for _, tx := range block.Transactions() {
		err := view.connectTransaction(tx, block.Height(), stxos)
		if err != nil {
			return err
		}
	}

	// Update the best hash for view to include this block since all of its
	// transactions have been connected.
	view.SetBestHash(block.Hash())
	return nil
}

// disconnectTransactions updates the view by removing all of the transactions
// created by the passed block, restoring all utxos the transactions spent by
// using the provided spent txo information, and setting the best hash for the
// view to the block before the passed block.
func (view *UtxoViewpoint) disconnectTransactions(block *btcutil.Block, stxos []spentTxOut) error {
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

		// Clear this transaction from the view if it already exists or
		// create a new empty entry for when it does not.  This is done
		// because the code relies on its existence in the view in order
		// to signal modifications have happened.
		isCoinbase := txIdx == 0
		entry := view.entries[*tx.Hash()]
		if entry == nil {
			entry = newUtxoEntry(tx.MsgTx().Version, isCoinbase,
				block.Height())
			view.entries[*tx.Hash()] = entry
		}
		entry.modified = true
		entry.sparseOutputs = make(map[uint32]*utxoOutput)

		// Loop backwards through all of the transaction inputs (except
		// for the coinbase which has no inputs) and unspend the
		// referenced txos.  This is necessary to match the order of the
		// spent txout entries.
		if isCoinbase {
			continue
		}
		for txInIdx := len(tx.MsgTx().TxIn) - 1; txInIdx > -1; txInIdx-- {
			// Ensure the spent txout index is decremented to stay
			// in sync with the transaction input.
			stxo := &stxos[stxoIdx]
			stxoIdx--

			// When there is not already an entry for the referenced
			// transaction in the view, it means it was fully spent,
			// so create a new utxo entry in order to resurrect it.
			txIn := tx.MsgTx().TxIn[txInIdx]
			originHash := &txIn.PreviousOutPoint.Hash
			originIndex := txIn.PreviousOutPoint.Index
			entry := view.entries[*originHash]
			if entry == nil {
				entry = newUtxoEntry(stxo.version,
					stxo.isCoinBase, stxo.height)
				view.entries[*originHash] = entry
			}

			// Mark the entry as modified since it is either new
			// or will be changed below.
			entry.modified = true

			// Restore the specific utxo using the stxo data from
			// the spend journal if it doesn't already exist in the
			// view.
			output, ok := entry.sparseOutputs[originIndex]
			if !ok {
				// Add the unspent transaction output.
				entry.sparseOutputs[originIndex] = &utxoOutput{
					spent:      false,
					compressed: stxo.compressed,
					amount:     stxo.amount,
					pkScript:   stxo.pkScript,
				}
				continue
			}

			// Mark the existing referenced transaction output as
			// unspent.
			output.spent = false
		}
	}

	// Update the best hash for view to the previous block since all of the
	// transactions for the current block have been disconnected.
	view.SetBestHash(&block.MsgBlock().Header.PrevBlock)
	return nil
}

// Entries returns the underlying map that stores of all the utxo entries.
func (view *UtxoViewpoint) Entries() map[chainhash.Hash]*UtxoEntry {
	return view.entries
}

// commit prunes all entries marked modified that are now fully spent and marks
// all entries as unmodified.
func (view *UtxoViewpoint) commit() {
	for txHash, entry := range view.entries {
		if entry == nil || (entry.modified && entry.IsFullySpent()) {
			delete(view.entries, txHash)
			continue
		}

		entry.modified = false
	}
}

// fetchUtxosMain fetches unspent transaction output data about the provided
// set of transactions from the point of view of the end of the main chain at
// the time of the call.
//
// Upon completion of this function, the view will contain an entry for each
// requested transaction.  Fully spent transactions, or those which otherwise
// don't exist, will result in a nil entry in the view.
func (view *UtxoViewpoint) fetchUtxosMain(db database.DB, txSet map[chainhash.Hash]struct{}) error {
	// Nothing to do if there are no requested hashes.
	if len(txSet) == 0 {
		return nil
	}

	// Load the unspent transaction output information for the requested set
	// of transactions from the point of view of the end of the main chain.
	//
	// NOTE: Missing entries are not considered an error here and instead
	// will result in nil entries in the view.  This is intentionally done
	// since other code uses the presence of an entry in the store as a way
	// to optimize spend and unspend updates to apply only to the specific
	// utxos that the caller needs access to.
	return db.View(func(dbTx database.Tx) error {
		for hash := range txSet {
			hashCopy := hash
			entry, err := dbFetchUtxoEntry(dbTx, &hashCopy)
			if err != nil {
				return err
			}

			view.entries[hash] = entry
		}

		return nil
	})
}

// fetchUtxos loads utxo details about provided set of transaction hashes into
// the view from the database as needed unless they already exist in the view in
// which case they are ignored.
func (view *UtxoViewpoint) fetchUtxos(db database.DB, txSet map[chainhash.Hash]struct{}) error {
	// Nothing to do if there are no requested hashes.
	if len(txSet) == 0 {
		return nil
	}

	// Filter entries that are already in the view.
	txNeededSet := make(map[chainhash.Hash]struct{})
	for hash := range txSet {
		// Already loaded into the current view.
		if _, ok := view.entries[hash]; ok {
			continue
		}

		txNeededSet[hash] = struct{}{}
	}

	// Request the input utxos from the database.
	return view.fetchUtxosMain(db, txNeededSet)
}

// fetchInputUtxos loads utxo details about the input transactions referenced
// by the transactions in the given block into the view from the database as
// needed.  In particular, referenced entries that are earlier in the block are
// added to the view and entries that are already in the view are not modified.
func (view *UtxoViewpoint) fetchInputUtxos(db database.DB, block *btcutil.Block) error {
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
	txNeededSet := make(map[chainhash.Hash]struct{})
	for i, tx := range transactions[1:] {
		for _, txIn := range tx.MsgTx().TxIn {
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

			// Don't request entries that are already in the view
			// from the database.
			if _, ok := view.entries[*originHash]; ok {
				continue
			}

			txNeededSet[*originHash] = struct{}{}
		}
	}

	// Request the input utxos from the database.
	return view.fetchUtxosMain(db, txNeededSet)
}

// NewUtxoViewpoint returns a new empty unspent transaction output view.
func NewUtxoViewpoint() *UtxoViewpoint {
	return &UtxoViewpoint{
		entries: make(map[chainhash.Hash]*UtxoEntry),
	}
}

// FetchUtxoView loads utxo details about the input transactions referenced by
// the passed transaction from the point of view of the end of the main chain.
// It also attempts to fetch the utxo details for the transaction itself so the
// returned view can be examined for duplicate unspent transaction outputs.
//
// This function is safe for concurrent access however the returned view is NOT.
func (b *BlockChain) FetchUtxoView(tx *btcutil.Tx) (*UtxoViewpoint, error) {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	// Create a set of needed transactions based on those referenced by the
	// inputs of the passed transaction.  Also, add the passed transaction
	// itself as a way for the caller to detect duplicates that are not
	// fully spent.
	txNeededSet := make(map[chainhash.Hash]struct{})
	txNeededSet[*tx.Hash()] = struct{}{}
	if !IsCoinBase(tx) {
		for _, txIn := range tx.MsgTx().TxIn {
			txNeededSet[txIn.PreviousOutPoint.Hash] = struct{}{}
		}
	}

	// Request the utxos from the point of view of the end of the main
	// chain.
	view := NewUtxoViewpoint()
	err := view.fetchUtxosMain(b.db, txNeededSet)
	return view, err
}

// FetchUtxoEntry loads and returns the unspent transaction output entry for the
// passed hash from the point of view of the end of the main chain.
//
// NOTE: Requesting a hash for which there is no data will NOT return an error.
// Instead both the entry and the error will be nil.  This is done to allow
// pruning of fully spent transactions.  In practice this means the caller must
// check if the returned entry is nil before invoking methods on it.
//
// This function is safe for concurrent access however the returned entry (if
// any) is NOT.
func (b *BlockChain) FetchUtxoEntry(txHash *chainhash.Hash) (*UtxoEntry, error) {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	var entry *UtxoEntry
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		entry, err = dbFetchUtxoEntry(dbTx, txHash)
		return err
	})
	if err != nil {
		return nil, err
	}

	return entry, nil
}
