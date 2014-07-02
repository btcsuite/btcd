// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcchain

import (
	"fmt"

	"github.com/conformal/btcdb"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// TxData contains contextual information about transactions such as which block
// they were found in and whether or not the outputs are spent.
type TxData struct {
	Tx          *btcutil.Tx
	Hash        *btcwire.ShaHash
	BlockHeight int64
	Spent       []bool
	Err         error
}

// TxStore is used to store transactions needed by other transactions for things
// such as script validation and double spend prevention.  This also allows the
// transaction data to be treated as a view since it can contain the information
// from the point-of-view of different points in the chain.
type TxStore map[btcwire.ShaHash]*TxData

// connectTransactions updates the passed map by applying transaction and
// spend information for all the transactions in the passed block.  Only
// transactions in the passed map are updated.
func connectTransactions(txStore TxStore, block *btcutil.Block) error {
	// Loop through all of the transactions in the block to see if any of
	// them are ones we need to update and spend based on the results map.
	for _, tx := range block.Transactions() {
		// Update the transaction store with the transaction information
		// if it's one of the requested transactions.
		msgTx := tx.MsgTx()
		if txD, exists := txStore[*tx.Sha()]; exists {
			txD.Tx = tx
			txD.BlockHeight = block.Height()
			txD.Spent = make([]bool, len(msgTx.TxOut))
			txD.Err = nil
		}

		// Spend the origin transaction output.
		for _, txIn := range msgTx.TxIn {
			originHash := &txIn.PreviousOutpoint.Hash
			originIndex := txIn.PreviousOutpoint.Index
			if originTx, exists := txStore[*originHash]; exists {
				if originIndex > uint32(len(originTx.Spent)) {
					continue
				}
				originTx.Spent[originIndex] = true
			}
		}
	}

	return nil
}

// disconnectTransactions updates the passed map by undoing transaction and
// spend information for all transactions in the passed block.  Only
// transactions in the passed map are updated.
func disconnectTransactions(txStore TxStore, block *btcutil.Block) error {
	// Loop through all of the transactions in the block to see if any of
	// them are ones that need to be undone based on the transaction store.
	for _, tx := range block.Transactions() {
		// Clear this transaction from the transaction store if needed.
		// Only clear it rather than deleting it because the transaction
		// connect code relies on its presence to decide whether or not
		// to update the store and any transactions which exist on both
		// sides of a fork would otherwise not be updated.
		if txD, exists := txStore[*tx.Sha()]; exists {
			txD.Tx = nil
			txD.BlockHeight = 0
			txD.Spent = nil
			txD.Err = btcdb.TxShaMissing
		}

		// Unspend the origin transaction output.
		for _, txIn := range tx.MsgTx().TxIn {
			originHash := &txIn.PreviousOutpoint.Hash
			originIndex := txIn.PreviousOutpoint.Index
			originTx, exists := txStore[*originHash]
			if exists && originTx.Tx != nil && originTx.Err == nil {
				if originIndex > uint32(len(originTx.Spent)) {
					continue
				}
				originTx.Spent[originIndex] = false
			}
		}
	}

	return nil
}

// fetchTxStoreMain fetches transaction data about the provided set of
// transactions from the point of view of the end of the main chain.  It takes
// a flag which specifies whether or not fully spent transaction should be
// included in the results.
func fetchTxStoreMain(db btcdb.Db, txSet map[btcwire.ShaHash]struct{}, includeSpent bool) TxStore {
	// Just return an empty store now if there are no requested hashes.
	txStore := make(TxStore)
	if len(txSet) == 0 {
		return txStore
	}

	// The transaction store map needs to have an entry for every requested
	// transaction.  By default, all the transactions are marked as missing.
	// Each entry will be filled in with the appropriate data below.
	txList := make([]*btcwire.ShaHash, 0, len(txSet))
	for hash := range txSet {
		hashCopy := hash
		txStore[hash] = &TxData{Hash: &hashCopy, Err: btcdb.TxShaMissing}
		txList = append(txList, &hashCopy)
	}

	// Ask the database (main chain) for the list of transactions.  This
	// will return the information from the point of view of the end of the
	// main chain.  Choose whether or not to include fully spent
	// transactions depending on the passed flag.
	fetchFunc := db.FetchUnSpentTxByShaList
	if includeSpent {
		fetchFunc = db.FetchTxByShaList
	}
	txReplyList := fetchFunc(txList)
	for _, txReply := range txReplyList {
		// Lookup the existing results entry to modify.  Skip
		// this reply if there is no corresponding entry in
		// the transaction store map which really should not happen, but
		// be safe.
		txD, ok := txStore[*txReply.Sha]
		if !ok {
			continue
		}

		// Fill in the transaction details.  A copy is used here since
		// there is no guarantee the returned data isn't cached and
		// this code modifies the data.  A bug caused by modifying the
		// cached data would likely be difficult to track down and could
		// cause subtle errors, so avoid the potential altogether.
		txD.Err = txReply.Err
		if txReply.Err == nil {
			txD.Tx = btcutil.NewTx(txReply.Tx)
			txD.BlockHeight = txReply.Height
			txD.Spent = make([]bool, len(txReply.TxSpent))
			copy(txD.Spent, txReply.TxSpent)
		}
	}

	return txStore
}

// fetchTxStore fetches transaction data about the provided set of transactions
// from the point of view of the given node.  For example, a given node might
// be down a side chain where a transaction hasn't been spent from its point of
// view even though it might have been spent in the main chain (or another side
// chain).  Another scenario is where a transaction exists from the point of
// view of the main chain, but doesn't exist in a side chain that branches
// before the block that contains the transaction on the main chain.
func (b *BlockChain) fetchTxStore(node *blockNode, txSet map[btcwire.ShaHash]struct{}) (TxStore, error) {
	// Get the previous block node.  This function is used over simply
	// accessing node.parent directly as it will dynamically create previous
	// block nodes as needed.  This helps allow only the pieces of the chain
	// that are needed to remain in memory.
	prevNode, err := b.getPrevNodeFromNode(node)
	if err != nil {
		return nil, err
	}

	// If we haven't selected a best chain yet or we are extending the main
	// (best) chain with a new block, fetch the requested set from the point
	// of view of the end of the main (best) chain without including fully
	// spent transactions in the results.  This is a little more efficient
	// since it means less transaction lookups are needed.
	if b.bestChain == nil || (prevNode != nil && prevNode.hash.IsEqual(b.bestChain.hash)) {
		txStore := fetchTxStoreMain(b.db, txSet, false)
		return txStore, nil
	}

	// Fetch the requested set from the point of view of the end of the
	// main (best) chain including fully spent transactions.  The fully
	// spent transactions are needed because the following code unspends
	// them to get the correct point of view.
	txStore := fetchTxStoreMain(b.db, txSet, true)

	// The requested node is either on a side chain or is a node on the main
	// chain before the end of it.  In either case, we need to undo the
	// transactions and spend information for the blocks which would be
	// disconnected during a reorganize to the point of view of the
	// node just before the requested node.
	detachNodes, attachNodes := b.getReorganizeNodes(prevNode)
	for e := detachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		block, err := b.db.FetchBlockBySha(n.hash)
		if err != nil {
			return nil, err
		}

		disconnectTransactions(txStore, block)
	}

	// The transaction store is now accurate to either the node where the
	// requested node forks off the main chain (in the case where the
	// requested node is on a side chain), or the requested node itself if
	// the requested node is an old node on the main chain.  Entries in the
	// attachNodes list indicate the requested node is on a side chain, so
	// if there are no nodes to attach, we're done.
	if attachNodes.Len() == 0 {
		return txStore, nil
	}

	// The requested node is on a side chain, so we need to apply the
	// transactions and spend information from each of the nodes to attach.
	for e := attachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		block, exists := b.blockCache[*n.hash]
		if !exists {
			return nil, fmt.Errorf("unable to find block %v in "+
				"side chain cache for transaction search",
				n.hash)
		}

		connectTransactions(txStore, block)
	}

	return txStore, nil
}

// fetchInputTransactions fetches the input transactions referenced by the
// transactions in the given block from its point of view.  See fetchTxList
// for more details on what the point of view entails.
func (b *BlockChain) fetchInputTransactions(node *blockNode, block *btcutil.Block) (TxStore, error) {
	// Build a map of in-flight transactions because some of the inputs in
	// this block could be referencing other transactions earlier in this
	// block which are not yet in the chain.
	txInFlight := map[btcwire.ShaHash]int{}
	transactions := block.Transactions()
	for i, tx := range transactions {
		txInFlight[*tx.Sha()] = i
	}

	// Loop through all of the transaction inputs (except for the coinbase
	// which has no inputs) collecting them into sets of what is needed and
	// what is already known (in-flight).
	txNeededSet := make(map[btcwire.ShaHash]struct{})
	txStore := make(TxStore)
	for i, tx := range transactions[1:] {
		for _, txIn := range tx.MsgTx().TxIn {
			// Add an entry to the transaction store for the needed
			// transaction with it set to missing by default.
			originHash := &txIn.PreviousOutpoint.Hash
			txD := &TxData{Hash: originHash, Err: btcdb.TxShaMissing}
			txStore[*originHash] = txD

			// It is acceptable for a transaction input to reference
			// the output of another transaction in this block only
			// if the referenced transaction comes before the
			// current one in this block.  Update the transaction
			// store acccordingly when this is the case.  Otherwise,
			// we still need the transaction.
			//
			// NOTE: The >= is correct here because i is one less
			// than the actual position of the transaction within
			// the block due to skipping the coinbase.
			if inFlightIndex, ok := txInFlight[*originHash]; ok &&
				i >= inFlightIndex {

				originTx := transactions[inFlightIndex]
				txD.Tx = originTx
				txD.BlockHeight = node.height
				txD.Spent = make([]bool, len(originTx.MsgTx().TxOut))
				txD.Err = nil
			} else {
				txNeededSet[*originHash] = struct{}{}
			}
		}
	}

	// Request the input transactions from the point of view of the node.
	txNeededStore, err := b.fetchTxStore(node, txNeededSet)
	if err != nil {
		return nil, err
	}

	// Merge the results of the requested transactions and the in-flight
	// transactions.
	for _, txD := range txNeededStore {
		txStore[*txD.Hash] = txD
	}

	return txStore, nil
}

// FetchTransactionStore fetches the input transactions referenced by the
// passed transaction from the point of view of the end of the main chain.  It
// also attempts to fetch the transaction itself so the returned TxStore can be
// examined for duplicate transactions.
func (b *BlockChain) FetchTransactionStore(tx *btcutil.Tx) (TxStore, error) {
	// Create a set of needed transactions from the transactions referenced
	// by the inputs of the passed transaction.  Also, add the passed
	// transaction itself as a way for the caller to detect duplicates.
	txNeededSet := make(map[btcwire.ShaHash]struct{})
	txNeededSet[*tx.Sha()] = struct{}{}
	for _, txIn := range tx.MsgTx().TxIn {
		txNeededSet[txIn.PreviousOutpoint.Hash] = struct{}{}
	}

	// Request the input transactions from the point of view of the end of
	// the main chain without including fully spent trasactions in the
	// results.  Fully spent transactions are only needed for chain
	// reorganization which does not apply here.
	txStore := fetchTxStoreMain(b.db, txNeededSet, false)
	return txStore, nil
}
