// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"

	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// utxoOutput houses details about an individual unspent transaction output such
// as whether or not it is spent, its public key script, and how much it pays.
type utxoOutput struct {
	spent    bool   // Output is spent.
	pkScript []byte // The public key script for the output.
	amount   int64  // The amount of the output.
}

// UtxoEntry contains contextual information about an unspent transaction such
// as whether or not it is a coinbase transaction, which block it was found in,
// and the spent status of its outputs.
type UtxoEntry struct {
	modified      bool              // Entry has been changed since load.
	version       int32             // The version of this tx.
	isCoinBase    bool              // Whether this entry is a coinbase tx.
	blockHeight   int32             // Height the block containing the tx.
	sparseOutputs map[uint32]uint32 // Sparse map of unspent output indices.
	outputs       []utxoOutput      // Packed slice of unspent outputs.
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
// spent based upon the current state of the unspent transaction output store
// the entry was obtained from.
//
// Returns true if the output index references an output that does not exist
// either due to it being invalid or because the output is not part of the store
// due to previously being spent/pruned.
func (entry *UtxoEntry) IsOutputSpent(outputIndex uint32) bool {
	packedIndex, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return true
	}

	return entry.outputs[packedIndex].spent
}

// SpendOutput marks the output at the provided index as spent.  Specifying an
// output index that does not exist will not have any effect.
func (entry *UtxoEntry) SpendOutput(outputIndex uint32) {
	packedIndex, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return
	}

	// Nothing to do if the output is already spent.
	output := &entry.outputs[packedIndex]
	if output.spent {
		return
	}

	entry.modified = true
	output.spent = true
	return
}

// IsFullySpent returns whether or not the transaction the utxo entry represents
// is fully spent.
func (entry *UtxoEntry) IsFullySpent() bool {
	// The entry is not fully spent if any of the outputs are unspent.
	for _, output := range entry.outputs {
		if !output.spent {
			return false
		}
	}

	return true
}

// Amount returns the amount of the provided output index.
//
// Returns 0 if the output index references an output that does not exist
// either due to it being invalid or because the output is not part of the store
// due to previously being spent/pruned.
func (entry *UtxoEntry) Amount(outputIndex uint32) int64 {
	packedIndex, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return 0
	}

	return entry.outputs[packedIndex].amount
}

// PkScriptByIndex returns the public key script for the provided output index.
//
// Returns nil if the output index references an output that does not exist
// either due to it being invalid or because the output is not part of the store
// due to previously being spent/pruned.
func (entry *UtxoEntry) PkScriptByIndex(outputIndex uint32) []byte {
	packedIndex, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return nil
	}

	return entry.outputs[packedIndex].pkScript
}

// newUtxoEntry returns a new unspent transaction output entry with the provided
// coinbase flag and block height ready to have unspent outputs added.
func newUtxoEntry(version int32, isCoinBase bool, blockHeight int32) *UtxoEntry {
	return &UtxoEntry{
		version:       version,
		isCoinBase:    isCoinBase,
		blockHeight:   blockHeight,
		sparseOutputs: make(map[uint32]uint32),
	}
}

// UtxoStore provides a subset of the set of unspent transaction outputs from a
// specific point of view in the chain.  For example, it could be from the end
// of the main chain, some point in the history of the main chain, or down a
// side chain.
//
// The unspent outputs are needed by other transactions for things such as
// script validation and double spend prevention.
type UtxoStore map[wire.ShaHash]*UtxoEntry

// LookupEntry returns information about a given transaction according to the
// current state of the store.  It will return nil if the passed transaction
// hash does not exist in the store or is otherwise not available such as when
// it has been disconnected during a reorg.
func (store UtxoStore) LookupEntry(txHash *wire.ShaHash) *UtxoEntry {
	entry, ok := store[*txHash]
	if !ok {
		return nil
	}

	return entry
}

// addTxOuts adds all outputs in the passed transaction which are not provably
// unspendable to the store.  This function only differs from AddTxOuts in that
// it accepts an entry that has already been looked up in the store.  This allow
// an extra extra map lookup to be avoided when connecting transactions.
func (store UtxoStore) addTxOuts(entry *UtxoEntry, tx *btcutil.Tx, blockHeight int32) {
	// When there are not already any utxos associated with the transaction,
	// add a new entry for it to the store.
	if entry == nil {
		entry = newUtxoEntry(tx.MsgTx().Version, IsCoinBase(tx),
			blockHeight)
		store[*tx.Sha()] = entry
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
		if packedIndex, ok := entry.sparseOutputs[uint32(txOutIdx)]; ok {
			output := &entry.outputs[packedIndex]
			output.spent = false
			output.amount = txOut.Value
			output.pkScript = txOut.PkScript
			continue
		}

		// Add the unspent transaction output.
		packedIndex := uint32(len(entry.outputs))
		entry.outputs = append(entry.outputs, utxoOutput{
			spent:    false,
			pkScript: txOut.PkScript,
			amount:   txOut.Value,
		})
		entry.sparseOutputs[uint32(txOutIdx)] = packedIndex
	}
	return
}

// AddTxOuts adds all outputs in the passed transaction which are not provably
// unspendable to the store.  When the store already has entries for any of the
// outputs, they are simply marked unspent.  All fields will be updated for
// existing entries since it's possible it has changed during a reorg.
func (store UtxoStore) AddTxOuts(tx *btcutil.Tx, blockHeight int32) {
	entry := store.LookupEntry(tx.Sha())
	store.addTxOuts(entry, tx, blockHeight)
}

// connectTransactions updates the utxo store by applying transaction and spend
// information for all the transactions in the passed block.  Only transactions
// already in the store are updated.
func (store UtxoStore) connectTransactions(block *btcutil.Block) {
	// Loop through all of the transactions in the block to see if any of
	// them are ones that need to be updated and spent based on the store.
	for _, tx := range block.Transactions() {
		// Add the transaction's outputs as available utxos if it's one
		// of the requested transactions.
		if entry, ok := store[*tx.Sha()]; ok {
			store.addTxOuts(entry, tx, block.Height())
		}

		// Spend the origin transaction outputs.
		for _, txIn := range tx.MsgTx().TxIn {
			originHash := &txIn.PreviousOutPoint.Hash
			originIndex := txIn.PreviousOutPoint.Index
			if entry := store.LookupEntry(originHash); entry != nil {
				entry.SpendOutput(originIndex)
			}
		}
	}
}

// disconnectTransactions updates the utxo store by undoing transaction and
// spend information for all transactions in the passed block.  Only
// transactions already in the store are updated.
func (store UtxoStore) disconnectTransactions(block *btcutil.Block) error {
	// Loop backwards through all transactions so everything is unspent in
	// reverse order.  This is necessary since transactions later in a block
	// can spend from previous ones.
	transactions := block.Transactions()
	for txIdx := len(transactions) - 1; txIdx > -1; txIdx-- {
		tx := transactions[txIdx]

		// Clear this transaction from the transaction store if needed.
		// Only clear it rather than deleting it because the transaction
		// connect code relies on its presence to decide whether or not
		// to update the store and any transactions which exist on both
		// sides of a fork would otherwise not be updated.
		if entry := store.LookupEntry(tx.Sha()); entry != nil {
			entry.modified = true
			entry.sparseOutputs = nil
			entry.outputs = nil
		}

		// Loop through all of the transaction inputs (except for the
		// coinbase which has no inputs) and unspend the origin
		// transaction outputs.
		if txIdx == 0 {
			continue
		}
		for _, txIn := range tx.MsgTx().TxIn {
			originHash := &txIn.PreviousOutPoint.Hash
			originIndex := txIn.PreviousOutPoint.Index
			if entry, ok := store[*originHash]; ok {
				// NOTE: Once the real pruned utxo set is
				// implemented this will need to use a reorg
				// journal to resurrect the entry if it doesn't
				// exist (is nil).
				//
				// However, since the code currently loads all
				// outputs regardless of them being spent, the
				// entry will always be there.
				entry.modified = true

				// Mark the referenced transaction output as
				// unspent.
				packedIndex, ok := entry.sparseOutputs[originIndex]
				if ok {
					output := &entry.outputs[packedIndex]
					output.spent = false
					continue
				}
			}
		}
	}

	return nil
}

// fetchUtxosMain fetches unspent transaction output data about the provided
// set of transactions from the point of view of the end of the main chain at
// the time of the call.
//
// There will be an entry in the returned store for each requested transaction.
// Typically fully spent transactions, or those which otherwise don't exist,
// will result in a nil entry in the store.  This is useful since it allows
// spending and unspending to only have to deal with the specifically requested
// transactions.
//
// This function MUST be called with the chain state lock held (for reads).
func (b *BlockChain) fetchUtxosMain(txSet map[wire.ShaHash]struct{}) (UtxoStore, error) {
	// Just return an empty store now if there are no requested hashes.
	store := make(UtxoStore)
	if len(txSet) == 0 {
		return store, nil
	}

	// Load the unspent transaction output information for the requested set
	// of transactions from the point of view of the end of the main chain.
	//
	// NOTE: Missing entries are not considered an error here and instead
	// will result in nil entries in the store.  This is intentionally done
	// since other code uses the presence of an entry in the store as a way
	// to optimize spend and unspend updates to apply only to the specific
	// utxos that the caller needs access to.
	err := b.db.View(func(dbTx database.Tx) error {
		for hash := range txSet {
			hashCopy := hash
			entry, err := dbFetchUtxoEntry(dbTx, &hashCopy)
			if err != nil {
				return err
			}

			store[hash] = entry
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return store, nil
}

// fetchUtxoStore fetches utxo details about the provided set of transactions
// from the point of view of the given node.  For example, a given node might be
// down a side chain where a transaction hasn't been spent from its point of
// view even though it might have been spent in the main chain (or another side
// chain).  Another scenario is where a transaction exists from the point of
// view of the main chain, but doesn't exist in a side chain that branches
// before the block that contains the transaction on the main chain.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) fetchUtxoStore(node *blockNode, txSet map[wire.ShaHash]struct{}) (UtxoStore, error) {
	// Get the previous block node.  This function is used over simply
	// accessing node.parent directly as it will dynamically create previous
	// block nodes as needed.  This helps allow only the pieces of the chain
	// that are needed to remain in memory.
	prevNode, err := b.getPrevNodeFromNode(node)
	if err != nil {
		return nil, err
	}

	// Fetch the requested set from the point of view of the end of the main
	// (best) chain.
	utxoStore, err := b.fetchUtxosMain(txSet)
	if err != nil {
		return nil, err
	}

	// When the given node is extending the main (best) chain, there is
	// nothing more to do since the point of view is already accurate.
	if prevNode != nil && prevNode.hash.IsEqual(b.bestNode.hash) {
		return utxoStore, nil
	}

	// The requested node is either on a side chain or is a node on the main
	// chain before the end of it.  In either case, the transactions in the
	// blocks which would be disconnected during a reorganize to the point
	// of view of the node just before the requested node must be unspent.
	detachNodes, attachNodes := b.getReorganizeNodes(prevNode)
	for e := detachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		var block *btcutil.Block
		err = b.db.View(func(dbTx database.Tx) error {
			var err error
			block, err = dbFetchBlockByHash(dbTx, n.hash)
			return err
		})
		if err != nil {
			return nil, err
		}

		utxoStore.disconnectTransactions(block)
	}

	// The store is now accurate to either the node where the requested node
	// forks off the main chain (in the case where the requested node is on
	// a side chain), or the requested node itself if the requested node is
	// an old node on the main chain.  Entries in the attachNodes list
	// indicate the requested node is on a side chain, so if there are no
	// nodes to attach, there is nothing left to do.
	if attachNodes.Len() == 0 {
		return utxoStore, nil
	}

	// The requested node is on a side chain, so connect all of the
	// transactions in the side chain blocks in order.
	for e := attachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		block, exists := b.blockCache[*n.hash]
		if !exists {
			return nil, fmt.Errorf("unable to find block %v in "+
				"side chain cache for transaction search",
				n.hash)
		}

		utxoStore.connectTransactions(block)
	}

	return utxoStore, nil
}

// fetchInputUtxos loads utxo details about the input transactions referenced
// by the transactions in the given block from its point of view.  See fetchUxos
// for more details on what the point of view entails.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) fetchInputUtxos(node *blockNode, block *btcutil.Block) (UtxoStore, error) {
	// Build a map of in-flight transactions because some of the inputs in
	// this block could be referencing other transactions earlier in this
	// block which are not yet in the chain.
	txInFlight := map[wire.ShaHash]int{}
	transactions := block.Transactions()
	for i, tx := range transactions {
		txInFlight[*tx.Sha()] = i
	}

	// Loop through all of the transaction inputs (except for the coinbase
	// which has no inputs) collecting them into sets of what is needed and
	// what is already known (in-flight).
	txNeededSet := make(map[wire.ShaHash]struct{})
	store := make(UtxoStore)
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
				store.AddTxOuts(originTx, node.height)
			} else {
				txNeededSet[*originHash] = struct{}{}
			}
		}
	}

	// Request the input utxos from the point of view of the node.
	neededStore, err := b.fetchUtxoStore(node, txNeededSet)
	if err != nil {
		return nil, err
	}

	// Merge the results of the requested utxos and the in-flight utxos.
	for hash, entry := range neededStore {
		store[hash] = entry
	}

	return store, nil
}

// FetchUtxoStore loads utxo details about the input transactions referenced by
// the passed transaction from the point of view of the end of the main chain.
// It also attempts to fetch the utxo details for the transaction itself so the
// returned store can be examined for duplicate unspent transaction outputs.
//
// This function is safe for concurrent access however the returned UtxoStore is
// NOT.
func (b *BlockChain) FetchUtxoStore(tx *btcutil.Tx) (UtxoStore, error) {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	// Create a set of needed transactions based on those referenced by the
	// inputs of the passed transaction.  Also, add the passed transaction
	// itself as a way for the caller to detect duplicates that are not
	// fully spent.
	txNeededSet := make(map[wire.ShaHash]struct{})
	txNeededSet[*tx.Sha()] = struct{}{}
	if !IsCoinBase(tx) {
		for _, txIn := range tx.MsgTx().TxIn {
			txNeededSet[txIn.PreviousOutPoint.Hash] = struct{}{}
		}
	}

	// Request the utxos from the point of view of the end of the main
	// chain.
	return b.fetchUtxosMain(txNeededSet)
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
func (b *BlockChain) FetchUtxoEntry(txHash *wire.ShaHash) (*UtxoEntry, error) {
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

	// NOTE: Once the real pruned utxo set is implemented this check will no
	// longer be needed since the entry will always be nil for fully spent
	// transactions.  However, the current code loads all of the outputs and
	// sets the spent flag accordingly in order to enable testing while the
	// pruned utxo set is developed.
	if entry != nil && entry.IsFullySpent() {
		return nil, nil
	}

	return entry, nil
}
