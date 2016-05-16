// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

// There are five potential viewpoints we need to worry about.

// ViewpointPrevValidInitial is the viewpoint from the perspective of the
// everything up the the previous block's TxTreeRegular, used to validate
// that tx tree regular.
const ViewpointPrevValidInitial = int8(0)

// ViewpointPrevValidStake is the viewpoint from the perspective of the
// everything up the the previous block's TxTreeRegular plus the
// contents of the TxTreeRegular, to validate TxTreeStake.
const ViewpointPrevValidStake = int8(1)

// ViewpointPrevInvalidStake is the viewpoint from the perspective of the
// everything up the the previous block's TxTreeRegular but without the
// contents of the TxTreeRegular, to validate TxTreeStake.
const ViewpointPrevInvalidStake = int8(2)

// ViewpointPrevValidRegular is the viewpoint from the perspective of the
// everything up the the previous block's TxTreeRegular plus the
// contents of the TxTreeRegular and TxTreeStake of current block,
// to validate TxTreeRegular of the current block.
const ViewpointPrevValidRegular = int8(3)

// ViewpointPrevInvalidRegular is the viewpoint from the perspective of the
// everything up the the previous block's TxTreeRegular minus the
// contents of the TxTreeRegular and TxTreeStake of current block,
// to validate TxTreeRegular of the current block.
const ViewpointPrevInvalidRegular = int8(4)

// TxData contains contextual information about transactions such as which block
// they were found in and whether or not the outputs are spent.
type TxData struct {
	Tx          *dcrutil.Tx
	Hash        *chainhash.Hash
	BlockHeight int64
	BlockIndex  uint32
	Spent       []bool
	Err         error
}

// TxStore is used to store transactions needed by other transactions for things
// such as script validation and double spend prevention.  This also allows the
// transaction data to be treated as a view since it can contain the information
// from the point-of-view of different points in the chain.
type TxStore map[chainhash.Hash]*TxData

// connectTxTree lets you connect an arbitrary TxTree to a txStore to push it
// forward in history.
// TxTree true == TxTreeRegular
// TxTree false == TxTreeStake
func connectTxTree(txStore TxStore,
	block *dcrutil.Block,
	txTree bool) {
	var transactions []*dcrutil.Tx
	if txTree {
		transactions = block.Transactions()
	} else {
		transactions = block.STransactions()
	}

	// Loop through all of the transactions in the block to see if any of
	// them are ones we need to update and spend based on the results map.
	for i, tx := range transactions {
		// Update the transaction store with the transaction information
		// if it's one of the requested transactions.
		msgTx := tx.MsgTx()
		if txD, exists := txStore[*tx.Sha()]; exists {
			txD.Tx = tx
			txD.BlockHeight = block.Height()
			txD.BlockIndex = uint32(i)
			txD.Spent = make([]bool, len(msgTx.TxOut))
			txD.Err = nil
		}

		// Spend the origin transaction output.
		for _, txIn := range msgTx.TxIn {
			originHash := &txIn.PreviousOutPoint.Hash
			originIndex := txIn.PreviousOutPoint.Index
			if originTx, exists := txStore[*originHash]; exists {
				if originTx.Spent == nil {
					continue
				}
				if originIndex >= uint32(len(originTx.Spent)) {
					continue
				}
				originTx.Spent[originIndex] = true
			}
		}
	}

	return
}

func connectTransactions(txStore TxStore, block *dcrutil.Block, parent *dcrutil.Block) error {
	// There is no regular tx from before the genesis block, so ignore the genesis
	// block for the next step.
	if parent != nil && block.Height() != 0 {
		mBlock := block.MsgBlock()
		votebits := mBlock.Header.VoteBits
		regularTxTreeValid := dcrutil.IsFlagSet16(votebits, dcrutil.BlockValid)

		// Only add the transactions in the event that the parent block's regular
		// tx were validated.
		if regularTxTreeValid {
			// Loop through all of the regular transactions in the block to see if
			// any of them are ones we need to update and spend based on the
			// results map.
			for i, tx := range parent.Transactions() {
				// Update the transaction store with the transaction information
				// if it's one of the requested transactions.
				msgTx := tx.MsgTx()
				if txD, exists := txStore[*tx.Sha()]; exists {
					txD.Tx = tx
					txD.BlockHeight = block.Height() - 1
					txD.BlockIndex = uint32(i)
					txD.Spent = make([]bool, len(msgTx.TxOut))
					txD.Err = nil
				}

				// Spend the origin transaction output.
				for _, txIn := range msgTx.TxIn {
					originHash := &txIn.PreviousOutPoint.Hash
					originIndex := txIn.PreviousOutPoint.Index
					if originTx, exists := txStore[*originHash]; exists {
						if originTx.Spent == nil {
							continue
						}
						if originIndex >= uint32(len(originTx.Spent)) {
							continue
						}
						originTx.Spent[originIndex] = true
					}
				}
			}
		}
	}

	// Loop through all of the stake transactions in the block to see if any of
	// them are ones we need to update and spend based on the results map.
	for i, tx := range block.STransactions() {
		// Update the transaction store with the transaction information
		// if it's one of the requested transactions.
		msgTx := tx.MsgTx()
		if txD, exists := txStore[*tx.Sha()]; exists {
			txD.Tx = tx
			txD.BlockHeight = block.Height()
			txD.BlockIndex = uint32(i)
			txD.Spent = make([]bool, len(msgTx.TxOut))
			txD.Err = nil
		}

		// Spend the origin transaction output.
		for _, txIn := range msgTx.TxIn {
			originHash := &txIn.PreviousOutPoint.Hash
			originIndex := txIn.PreviousOutPoint.Index
			if originTx, exists := txStore[*originHash]; exists {
				if originTx.Spent == nil {
					continue
				}
				if originIndex >= uint32(len(originTx.Spent)) {
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
func disconnectTransactions(txStore TxStore, block *dcrutil.Block, parent *dcrutil.Block) error {
	// Loop through all of the stake transactions in the block to see if any of
	// them are ones that need to be undone based on the transaction store.
	for _, tx := range block.STransactions() {
		// Clear this transaction from the transaction store if needed.
		// Only clear it rather than deleting it because the transaction
		// connect code relies on its presence to decide whether or not
		// to update the store and any transactions which exist on both
		// sides of a fork would otherwise not be updated.
		if txD, exists := txStore[*tx.Sha()]; exists {
			txD.Tx = nil
			txD.BlockHeight = int64(wire.NullBlockHeight)
			txD.BlockIndex = wire.NullBlockIndex
			txD.Spent = nil
			txD.Err = database.ErrTxShaMissing
		}

		// Unspend the origin transaction output.
		for _, txIn := range tx.MsgTx().TxIn {
			originHash := &txIn.PreviousOutPoint.Hash
			originIndex := txIn.PreviousOutPoint.Index
			originTx, exists := txStore[*originHash]
			if exists && originTx.Tx != nil && originTx.Err == nil {
				if originTx.Spent == nil {
					continue
				}
				if originIndex >= uint32(len(originTx.Spent)) {
					continue
				}
				originTx.Spent[originIndex] = false
			}
		}
	}

	// There is no regular tx from before the genesis block, so ignore the genesis
	// block for the next step.
	if parent != nil && block.Height() != 0 {
		mBlock := block.MsgBlock()
		votebits := mBlock.Header.VoteBits
		regularTxTreeValid := dcrutil.IsFlagSet16(votebits, dcrutil.BlockValid)

		// Only bother to unspend transactions if the parent's tx tree was
		// validated. Otherwise, these transactions were never in the blockchain's
		// history in the first place.
		if regularTxTreeValid {
			// Loop through all of the regular transactions in the block to see if
			// any of them are ones that need to be undone based on the
			// transaction store.
			for _, tx := range parent.Transactions() {
				// Clear this transaction from the transaction store if needed.
				// Only clear it rather than deleting it because the transaction
				// connect code relies on its presence to decide whether or not
				// to update the store and any transactions which exist on both
				// sides of a fork would otherwise not be updated.
				if txD, exists := txStore[*tx.Sha()]; exists {
					txD.Tx = nil
					txD.BlockHeight = int64(wire.NullBlockHeight)
					txD.BlockIndex = wire.NullBlockIndex
					txD.Spent = nil
					txD.Err = database.ErrTxShaMissing
				}

				// Unspend the origin transaction output.
				for _, txIn := range tx.MsgTx().TxIn {
					originHash := &txIn.PreviousOutPoint.Hash
					originIndex := txIn.PreviousOutPoint.Index
					originTx, exists := txStore[*originHash]
					if exists && originTx.Tx != nil && originTx.Err == nil {
						if originTx.Spent == nil {
							continue
						}
						if originIndex >= uint32(len(originTx.Spent)) {
							continue
						}
						originTx.Spent[originIndex] = false
					}
				}
			}
		}
	}

	return nil
}

// fetchTxStoreMain fetches transaction data about the provided set of
// transactions from the point of view of the end of the main chain.  It takes
// a flag which specifies whether or not fully spent transaction should be
// included in the results.
func fetchTxStoreMain(db database.Db, txSet map[chainhash.Hash]struct{}, includeSpent bool) TxStore {
	// Just return an empty store now if there are no requested hashes.
	txStore := make(TxStore)
	if len(txSet) == 0 {
		return txStore
	}

	// The transaction store map needs to have an entry for every requested
	// transaction.  By default, all the transactions are marked as missing.
	// Each entry will be filled in with the appropriate data below.
	txList := make([]*chainhash.Hash, 0, len(txSet))
	for hash := range txSet {
		hashCopy := hash
		txStore[hash] = &TxData{Hash: &hashCopy, Err: database.ErrTxShaMissing}
		txList = append(txList, &hashCopy)
	}

	// Ask the database (main chain) for the list of transactions.  This
	// will return the information from the point of view of the end of the
	// main chain.  Choose whether or not to include fully spent
	// transactions depending on the passed flag.
	var txReplyList []*database.TxListReply
	if includeSpent {
		txReplyList = db.FetchTxByShaList(txList)
	} else {
		txReplyList = db.FetchUnSpentTxByShaList(txList)
	}
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
			txD.Tx = dcrutil.NewTx(txReply.Tx)
			txD.BlockHeight = txReply.Height
			txD.BlockIndex = txReply.Index
			txD.Spent = make([]bool, len(txReply.TxSpent))
			copy(txD.Spent, txReply.TxSpent)
		}
	}

	return txStore
}

// handleTxStoreViewpoint appends extra Tx Trees to update to a different
// viewpoint.
func handleTxStoreViewpoint(block *dcrutil.Block, parentBlock *dcrutil.Block,
	txStore TxStore, viewpoint int8) error {
	// We don't need to do anything for the current top block viewpoint.
	if viewpoint == ViewpointPrevValidInitial {
		return nil
	}

	// ViewpointPrevValidStake: Append the prev block TxTreeRegular
	// txs to fill out TxIns.
	if viewpoint == ViewpointPrevValidStake {
		connectTxTree(txStore, parentBlock, true)
		return nil
	}

	// ViewpointPrevInvalidStake: Do not append the prev block
	// TxTreeRegular txs, since they don't exist.
	if viewpoint == ViewpointPrevInvalidStake {
		return nil
	}

	// ViewpointPrevValidRegular: Append the prev block TxTreeRegular
	// txs to fill in TxIns, then append the cur block TxTreeStake
	// txs to fill in TxInss. TxTreeRegular from current block will
	// never be allowed to spend from the stake tree of the current
	// block anyway because of the consensus rules regarding output
	// maturity, but do it anyway.
	if viewpoint == ViewpointPrevValidRegular {
		connectTxTree(txStore, parentBlock, true)
		connectTxTree(txStore, block, false)
		return nil
	}

	// ViewpointPrevInvalidRegular: Append the cur block TxTreeStake
	// txs to fill in TxIns. TxTreeRegular from current block will
	// never be allowed to spend from the stake tree of the current
	// block anyway because of the consensus rules regarding output
	// maturity, but do it anyway.
	if viewpoint == ViewpointPrevInvalidRegular {
		connectTxTree(txStore, block, false)
		return nil
	}

	return fmt.Errorf("Error: invalid viewpoint '0x%x' given to "+
		"handleTxStoreViewpoint", viewpoint)
}

// fetchTxStore fetches transaction data about the provided set of transactions
// from the point of view of the given node.  For example, a given node might
// be down a side chain where a transaction hasn't been spent from its point of
// view even though it might have been spent in the main chain (or another side
// chain).  Another scenario is where a transaction exists from the point of
// view of the main chain, but doesn't exist in a side chain that branches
// before the block that contains the transaction on the main chain.
func (b *BlockChain) fetchTxStore(node *blockNode, block *dcrutil.Block,
	txSet map[chainhash.Hash]struct{}, viewpoint int8) (TxStore, error) {
	// Get the previous block node.  This function is used over simply
	// accessing node.parent directly as it will dynamically create previous
	// block nodes as needed.  This helps allow only the pieces of the chain
	// that are needed to remain in memory.
	prevNode, err := b.getPrevNodeFromNode(node)
	if err != nil {
		return nil, err
	}
	// We don't care if the previous node doesn't exist because this
	// block is the genesis block.
	if prevNode == nil {
		return nil, nil
	}

	// Get the previous block, too.
	prevBlock, err := b.getBlockFromHash(prevNode.hash)
	if err != nil {
		return nil, err
	}

	// If we haven't selected a best chain yet or we are extending the main
	// (best) chain with a new block, fetch the requested set from the point
	// of view of the end of the main (best) chain without including fully
	// spent transactions in the results.  This is a little more efficient
	// since it means less transaction lookups are needed.
	if b.bestChain == nil || (prevNode != nil &&
		prevNode.hash.IsEqual(b.bestChain.hash)) {
		txStore := fetchTxStoreMain(b.db, txSet, false)
		err := handleTxStoreViewpoint(block, prevBlock, txStore, viewpoint)
		if err != nil {
			return nil, err
		}
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
	detachNodes, attachNodes, err := b.getReorganizeNodes(prevNode)
	if err != nil {
		return nil, err
	}

	for e := detachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		blockDisconnect, err := b.db.FetchBlockBySha(n.hash)
		if err != nil {
			return nil, err
		}

		// Load the parent block from either the database or the sidechain.
		parentHash := &blockDisconnect.MsgBlock().Header.PrevBlock

		parentBlock, errFetchBlock := b.getBlockFromHash(parentHash)
		if errFetchBlock != nil {
			return nil, errFetchBlock
		}

		err = disconnectTransactions(txStore, blockDisconnect, parentBlock)
		if err != nil {
			return nil, err
		}
	}

	// The transaction store is now accurate to either the node where the
	// requested node forks off the main chain (in the case where the
	// requested node is on a side chain), or the requested node itself if
	// the requested node is an old node on the main chain.  Entries in the
	// attachNodes list indicate the requested node is on a side chain, so
	// if there are no nodes to attach, we're done.
	if attachNodes.Len() == 0 {
		err = handleTxStoreViewpoint(block, prevBlock, txStore, viewpoint)
		if err != nil {
			return nil, err
		}

		return txStore, nil
	}

	// The requested node is on a side chain, so we need to apply the
	// transactions and spend information from each of the nodes to attach.
	for e := attachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		blockConnect, exists := b.blockCache[*n.hash]
		if !exists {
			return nil, fmt.Errorf("unable to find block %v in "+
				"side chain cache for transaction search",
				n.hash)
		}

		// Load the parent block from either the database or the sidechain.
		parentHash := &blockConnect.MsgBlock().Header.PrevBlock

		parentBlock, errFetchBlock := b.getBlockFromHash(parentHash)
		if errFetchBlock != nil {
			return nil, errFetchBlock
		}

		err = connectTransactions(txStore, blockConnect, parentBlock)
		if err != nil {
			return nil, err
		}
	}

	err = handleTxStoreViewpoint(block, prevBlock, txStore, viewpoint)
	if err != nil {
		return nil, err
	}

	return txStore, nil
}

// fetchInputTransactions fetches the input transactions referenced by the
// transactions in the given block from its point of view.  See fetchTxList
// for more details on what the point of view entails.
// Decred: This function is for verifying the validity of the regular tx tree in
// 		this block for the case that it does get accepted in the next block.
func (b *BlockChain) fetchInputTransactions(node *blockNode, block *dcrutil.Block, viewpoint int8) (TxStore, error) {
	// Verify we have the same node as we do block.
	blockHash := block.Sha()
	if !node.hash.IsEqual(blockHash) {
		return nil, fmt.Errorf("node and block hash are different")
	}

	// If we need the previous block, grab it.
	var parentBlock *dcrutil.Block
	if viewpoint == ViewpointPrevValidInitial ||
		viewpoint == ViewpointPrevValidStake ||
		viewpoint == ViewpointPrevValidRegular {
		var errFetchBlock error
		parentBlock, errFetchBlock = b.getBlockFromHash(node.parentHash)
		if errFetchBlock != nil {
			return nil, errFetchBlock
		}
	}

	txInFlight := map[chainhash.Hash]int{}
	txNeededSet := make(map[chainhash.Hash]struct{})
	txStore := make(TxStore)

	// Case 1: ViewpointPrevValidInitial. We need the viewpoint of the
	// current chain without the TxTreeRegular of the previous block
	// added so we can validate that.
	if viewpoint == ViewpointPrevValidInitial {
		// Build a map of in-flight transactions because some of the inputs in
		// this block could be referencing other transactions earlier in this
		// block which are not yet in the chain.
		transactions := parentBlock.Transactions()
		for i, tx := range transactions {
			txInFlight[*tx.Sha()] = i
		}

		// Loop through all of the transaction inputs (except for the coinbase
		// which has no inputs) collecting them into sets of what is needed and
		// what is already known (in-flight).
		for i, tx := range transactions[1:] {
			for _, txIn := range tx.MsgTx().TxIn {
				// Add an entry to the transaction store for the needed
				// transaction with it set to missing by default.
				originHash := &txIn.PreviousOutPoint.Hash
				txD := &TxData{Hash: originHash, Err: database.ErrTxShaMissing}
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
					txD.BlockHeight = node.height - 1
					txD.BlockIndex = uint32(inFlightIndex)
					txD.Spent = make([]bool, len(originTx.MsgTx().TxOut))
					txD.Err = nil
				} else {
					txNeededSet[*originHash] = struct{}{}
				}
			}
		}

		// Request the input transactions from the point of view of the node.
		txNeededStore, err := b.fetchTxStore(node, block, txNeededSet, viewpoint)
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

	// Case 2+3: ViewpointPrevValidStake and ViewpointPrevValidStake.
	// For ViewpointPrevValidStake, we need the viewpoint of the
	// current chain with the TxTreeRegular of the previous block
	// added so we can validate the TxTreeStake of the current block.
	// For ViewpointPrevInvalidStake, we need the viewpoint of the
	// current chain with the TxTreeRegular of the previous block
	// missing so we can validate the TxTreeStake of the current block.
	if viewpoint == ViewpointPrevValidStake ||
		viewpoint == ViewpointPrevInvalidStake {
		// We need all of the stake tx txins. None of these are considered
		// in-flight in relation to the regular tx tree or to other tx in
		// the stake tx tree, so don't do any of those expensive checks and
		// just append it to the tx slice.
		stransactions := block.STransactions()
		for _, tx := range stransactions {
			isSSGen, _ := stake.IsSSGen(tx)

			for i, txIn := range tx.MsgTx().TxIn {
				// Ignore stakebases.
				if isSSGen && i == 0 {
					continue
				}

				// Add an entry to the transaction store for the needed
				// transaction with it set to missing by default.
				originHash := &txIn.PreviousOutPoint.Hash
				txD := &TxData{Hash: originHash, Err: database.ErrTxShaMissing}
				txStore[*originHash] = txD

				txNeededSet[*originHash] = struct{}{}
			}
		}

		// Request the input transactions from the point of view of the node.
		txNeededStore, err := b.fetchTxStore(node, block, txNeededSet, viewpoint)
		if err != nil {
			return nil, err
		}

		return txNeededStore, nil
	}

	// Case 4+5: ViewpointPrevValidRegular and
	// ViewpointPrevInvalidRegular.
	// For ViewpointPrevValidRegular, we need the viewpoint of the
	// current chain with the TxTreeRegular of the previous block
	// and the TxTreeStake of the current block added so we can
	// validate the TxTreeRegular of the current block.
	// For ViewpointPrevInvalidRegular, we need the viewpoint of the
	// current chain with the TxTreeRegular of the previous block
	// missing and the TxTreeStake of the current block added so we
	// can validate the TxTreeRegular of the current block.
	if viewpoint == ViewpointPrevValidRegular ||
		viewpoint == ViewpointPrevInvalidRegular {
		transactions := block.Transactions()
		for i, tx := range transactions {
			txInFlight[*tx.Sha()] = i
		}

		// Loop through all of the transaction inputs (except for the coinbase
		// which has no inputs) collecting them into sets of what is needed and
		// what is already known (in-flight).
		txNeededSet := make(map[chainhash.Hash]struct{})
		txStore = make(TxStore)
		for i, tx := range transactions[1:] {
			for _, txIn := range tx.MsgTx().TxIn {
				// Add an entry to the transaction store for the needed
				// transaction with it set to missing by default.
				originHash := &txIn.PreviousOutPoint.Hash
				txD := &TxData{Hash: originHash, Err: database.ErrTxShaMissing}
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
					txD.BlockIndex = uint32(inFlightIndex)
					txD.Spent = make([]bool, len(originTx.MsgTx().TxOut))
					txD.Err = nil
				} else {
					txNeededSet[*originHash] = struct{}{}
				}
			}
		}

		// Request the input transactions from the point of view of the node.
		txNeededStore, err := b.fetchTxStore(node, block, txNeededSet, viewpoint)
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

	return nil, fmt.Errorf("Invalid viewpoint passed to fetchInputTransactions")
}

// FetchTransactionStore fetches the input transactions referenced by the
// passed transaction from the point of view of the end of the main chain.  It
// also attempts to fetch the transaction itself so the returned TxStore can be
// examined for duplicate transactions.
// IsValid indicates if the current block on head has had its TxTreeRegular
// validated by the stake voters.
func (b *BlockChain) FetchTransactionStore(tx *dcrutil.Tx,
	isValid bool, includeSpent bool) (TxStore, error) {
	isSSGen, _ := stake.IsSSGen(tx)

	// Create a set of needed transactions from the transactions referenced
	// by the inputs of the passed transaction.  Also, add the passed
	// transaction itself as a way for the caller to detect duplicates.
	txNeededSet := make(map[chainhash.Hash]struct{})
	txNeededSet[*tx.Sha()] = struct{}{}
	for i, txIn := range tx.MsgTx().TxIn {
		// Skip all stakebase inputs.
		if isSSGen && (i == 0) {
			continue
		}

		txNeededSet[txIn.PreviousOutPoint.Hash] = struct{}{}
	}

	// Request the input transactions from the point of view of the end of
	// the main chain with or without without including fully spent transactions
	// in the results.
	txStore := fetchTxStoreMain(b.db, txNeededSet, includeSpent)

	topBlock, err := b.getBlockFromHash(b.bestChain.hash)
	if err != nil {
		return nil, err
	}

	if isValid {
		connectTxTree(txStore, topBlock, true)
	}

	return txStore, nil
}
