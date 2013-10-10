// Copyright (c) 2013 Conformal Systems LLC.
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
	Tx          *btcwire.MsgTx
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
	for i, tx := range block.MsgBlock().Transactions {
		txHash, err := block.TxSha(i)
		if err != nil {
			return err
		}

		// Update the transaction store with the transaction information
		// if it's one of the requested transactions.
		if txD, exists := txStore[*txHash]; exists {
			txD.Tx = tx
			txD.BlockHeight = block.Height()
			txD.Spent = make([]bool, len(tx.TxOut))
			txD.Err = nil
		}

		// Spend the origin transaction output.
		for _, txIn := range tx.TxIn {
			originHash := &txIn.PreviousOutpoint.Hash
			originIndex := txIn.PreviousOutpoint.Index
			if originTx, exists := txStore[*originHash]; exists {
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
	for i, tx := range block.MsgBlock().Transactions {
		txHash, err := block.TxSha(i)
		if err != nil {
			return err
		}

		// Clear this transaction from the transaction store if needed.
		// Only clear it rather than deleting it because the transaction
		// connect code relies on its presence to decide whether or not
		// to update the store and any transactions which exist on both
		// sides of a fork would otherwise not be updated.
		if txD, exists := txStore[*txHash]; exists {
			txD.Tx = nil
			txD.BlockHeight = 0
			txD.Spent = nil
			txD.Err = btcdb.TxShaMissing
		}

		// Unspend the origin transaction output.
		for _, txIn := range tx.TxIn {
			originHash := &txIn.PreviousOutpoint.Hash
			originIndex := txIn.PreviousOutpoint.Index
			originTx, exists := txStore[*originHash]
			if exists && originTx.Tx != nil && originTx.Err == nil {
				originTx.Spent[originIndex] = false
			}
		}
	}

	return nil
}

// fetchTxStoreMain fetches transaction data about the provided set of
// transactions from the point of view of the end of the main chain.
func fetchTxStoreMain(db btcdb.Db, txSet map[btcwire.ShaHash]bool) TxStore {
	// The transaction store map needs to have an entry for every requested
	// transaction.  By default, all the transactions are marked as missing.
	// Each entry will be filled in with the appropriate data below.
	txList := make([]*btcwire.ShaHash, 0, len(txSet))
	txStore := make(TxStore)
	for hash := range txSet {
		hashCopy := hash
		txStore[hash] = &TxData{Hash: &hashCopy, Err: btcdb.TxShaMissing}
		txList = append(txList, &hashCopy)
	}

	// Ask the database (main chain) for the list of transactions.  This
	// will return the information from the point of view of the end of the
	// main chain.
	txReplyList := db.FetchUnSpentTxByShaList(txList)
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
			txD.Tx = txReply.Tx
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
func (b *BlockChain) fetchTxStore(node *blockNode, txSet map[btcwire.ShaHash]bool) (TxStore, error) {
	// Get the previous block node.  This function is used over simply
	// accessing node.parent directly as it will dynamically create previous
	// block nodes as needed.  This helps allow only the pieces of the chain
	// that are needed to remain in memory.
	prevNode, err := b.getPrevNodeFromNode(node)
	if err != nil {
		return nil, err
	}

	// Fetch the requested set from the point of view of the end of the
	// main (best) chain.
	txStore := fetchTxStoreMain(b.db, txSet)

	// If we haven't selected a best chain yet or we are extending the main
	// (best) chain with a new block, everything is accurate, so return the
	// results now.
	if b.bestChain == nil || (prevNode != nil && prevNode.hash.IsEqual(b.bestChain.hash)) {
		return txStore, nil
	}

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
	transactions := block.MsgBlock().Transactions
	for i := range transactions {
		// Get transaction hash.  It's safe to ignore the error since
		// it's already cached in the nominal code path and the only
		// way it can fail is if the index is out of range which is
		// impossible here.
		txHash, _ := block.TxSha(i)
		txInFlight[*txHash] = i
	}

	// Loop through all of the transaction inputs (except for the coinbase
	// which has no inputs) collecting them into sets of what is needed and
	// what is already known (in-flight).
	txNeededSet := make(map[btcwire.ShaHash]bool)
	txStore := make(TxStore)
	for i, tx := range transactions[1:] {
		for _, txIn := range tx.TxIn {
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
				txD.Spent = make([]bool, len(originTx.TxOut))
				txD.Err = nil
			} else {
				txNeededSet[*originHash] = true
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
func (b *BlockChain) FetchTransactionStore(tx *btcwire.MsgTx) (TxStore, error) {
	txHash, err := tx.TxSha()
	if err != nil {
		return nil, err
	}

	// Create a set of needed transactions from the transactions referenced
	// by the inputs of the passed transaction.  Also, add the passed
	// transaction itself as a way for the caller to detect duplicates.
	txNeededSet := make(map[btcwire.ShaHash]bool)
	txNeededSet[txHash] = true
	for _, txIn := range tx.TxIn {
		txNeededSet[txIn.PreviousOutpoint.Hash] = true
	}

	// Request the input transactions from the point of view of the end of
	// the main chain.
	txStore := fetchTxStoreMain(b.db, txNeededSet)
	return txStore, nil
}
