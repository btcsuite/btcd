// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/heap"
	"container/list"
	"fmt"
	"time"

	"github.com/conformal/btcchain"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

const (
	// generatedBlockVersion is the version of the block being generated.
	// It is defined as a constant here rather than using the
	// btcwire.BlockVersion constant since a change in the block version
	// will require changes to the generated block.  Using the btcwire
	// constant for generated block version could allow creation of invalid
	// blocks for the updated version.
	generatedBlockVersion = 2

	// minHighPriority is the minimum priority value that allows a
	// transaction to be considered high priority.
	minHighPriority = btcutil.SatoshiPerBitcoin * 144.0 / 250

	// blockHeaderOverhead is the max number of bytes it takes to serialize
	// a block header and max possible transaction count.
	blockHeaderOverhead = btcwire.MaxBlockHeaderPayload + btcwire.MaxVarIntPayload

	// coinbaseFlags is added to the coinbase script of a generated block
	// and is used to monitor BIP16 support as well as blocks that are
	// generated via btcd.
	coinbaseFlags = "/P2SH/btcd/"

	// standardScriptVerifyFlags are the script flags which are used when
	// executing transaction scripts to enforce additional checks which
	// are required for the script to be considered standard.  These checks
	// help reduce issues related to transaction malleability as well as
	// allow pay-to-script hash transactions.  Note these flags are
	// different than what is required for the consensus rules in that they
	// are more strict.
	standardScriptVerifyFlags = btcscript.ScriptBip16 |
		btcscript.ScriptCanonicalSignatures |
		btcscript.ScriptStrictMultiSig
)

// txPrioItem houses a transaction along with extra information that allows the
// transaction to be prioritized and track dependencies on other transactions
// which have not been mined into a block yet.
type txPrioItem struct {
	tx       *btcutil.Tx
	fee      int64
	priority float64
	feePerKB float64

	// dependsOn holds a map of transaction hashes which this one depends
	// on.  It will only be set when the transaction references other
	// transactions in the memory pool and hence must come after them in
	// a block.
	dependsOn map[btcwire.ShaHash]struct{}
}

// txPriorityQueueLessFunc describes a function that can be used as a compare
// function for a transaction priority queue (txPriorityQueue).
type txPriorityQueueLessFunc func(*txPriorityQueue, int, int) bool

// txPriorityQueue implements a priority queue of txPrioItem elements that
// supports an arbitrary compare function as defined by txPriorityQueueLessFunc.
type txPriorityQueue struct {
	lessFunc txPriorityQueueLessFunc
	items    []*txPrioItem
}

// Len returns the number of items in the priority queue.  It is part of the
// heap.Interface implementation.
func (pq *txPriorityQueue) Len() int {
	return len(pq.items)
}

// Less returns whether the item in the priority queue with index i should sort
// before the item with index j by deferring to the assigned less function.  It
// is part of the heap.Interface implementation.
func (pq *txPriorityQueue) Less(i, j int) bool {
	return pq.lessFunc(pq, i, j)
}

// Swap swaps the items at the passed indices in the priority queue.  It is
// part of the heap.Interface implementation.
func (pq *txPriorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
}

// Push pushes the passed item onto the priority queue.  It is part of the
// heap.Interface implementation.
func (pq *txPriorityQueue) Push(x interface{}) {
	pq.items = append(pq.items, x.(*txPrioItem))
}

// Pop removes the highest priority item (according to Less) from the priority
// queue and returns it.  It is part of the heap.Interface implementation.
func (pq *txPriorityQueue) Pop() interface{} {
	n := len(pq.items)
	item := pq.items[n-1]
	pq.items[n-1] = nil
	pq.items = pq.items[0 : n-1]
	return item
}

// SetLessFunc sets the compare function for the priority queue to the provided
// function.  It also invokes heap.Init on the priority queue using the new
// function so it can immediately be used with heap.Push/Pop.
func (pq *txPriorityQueue) SetLessFunc(lessFunc txPriorityQueueLessFunc) {
	pq.lessFunc = lessFunc
	heap.Init(pq)
}

// txPQByPriority sorts a txPriorityQueue by transaction priority and then fees
// per kilobyte.
func txPQByPriority(pq *txPriorityQueue, i, j int) bool {
	// Using > here so that pop gives the highest priority item as opposed
	// to the lowest.  Sort by priority first, then fee.
	if pq.items[i].priority == pq.items[j].priority {
		return pq.items[i].feePerKB > pq.items[j].feePerKB
	}
	return pq.items[i].priority > pq.items[j].priority

}

// txPQByFee sorts a txPriorityQueue by fees per kilobyte and then transaction
// priority.
func txPQByFee(pq *txPriorityQueue, i, j int) bool {
	// Using > here so that pop gives the highest fee item as opposed
	// to the lowest.  Sort by fee first, then priority.
	if pq.items[i].feePerKB == pq.items[j].feePerKB {
		return pq.items[i].priority > pq.items[j].priority
	}
	return pq.items[i].feePerKB > pq.items[j].feePerKB
}

// newTxPriorityQueue returns a new transaction priority queue that reserves the
// passed amount of space for the elements.  The new priority queue uses either
// the txPQByPriority or the txPQByFee compare function depending on the
// sortByFee parameter and is already initialized for use with heap.Push/Pop.
// The priority queue can grow larger than the reserved space, but extra copies
// of the underlying array can be avoided by reserving a sane value.
func newTxPriorityQueue(reserve int, sortByFee bool) *txPriorityQueue {
	pq := &txPriorityQueue{
		items: make([]*txPrioItem, 0, reserve),
	}
	if sortByFee {
		pq.SetLessFunc(txPQByFee)
	} else {
		pq.SetLessFunc(txPQByPriority)
	}
	return pq
}

// BlockTemplate houses a block that has yet to be solved along with additional
// details about the fees and the number of signature operations for each
// transaction in the block.
type BlockTemplate struct {
	block           *btcwire.MsgBlock
	fees            []int64
	sigOpCounts     []int64
	height          int64
	validPayAddress bool
}

// minInt is a helper function to return the minimum of two ints.  This avoids
// a math import and the need to cast to floats.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// mergeTxStore adds all of the transactions in txStoreB to txStoreA.  The
// result is that txStoreA will contain all of its original transactions plus
// all of the transactions in txStoreB.
func mergeTxStore(txStoreA btcchain.TxStore, txStoreB btcchain.TxStore) {
	for hash, txDataB := range txStoreB {
		if txDataA, exists := txStoreA[hash]; !exists ||
			(txDataA.Err == btcdb.ErrTxShaMissing && txDataB.Err !=
				btcdb.ErrTxShaMissing) {

			txStoreA[hash] = txDataB
		}
	}
}

// standardCoinbaseScript returns a standard script suitable for use as the
// signature script of the coinbase transaction of a new block.  In particular,
// it starts with the block height that is required by version 2 blocks and adds
// the extra nonce as well as additional coinbase flags.
func standardCoinbaseScript(nextBlockHeight int64, extraNonce uint64) []byte {
	return btcscript.NewScriptBuilder().AddInt64(nextBlockHeight).
		AddUint64(extraNonce).AddData([]byte(coinbaseFlags)).Script()
}

// createCoinbaseTx returns a coinbase transaction paying an appropriate subsidy
// based on the passed block height to the provided address.  When the address
// is nil, the coinbase transaction will instead be redeemable by anyone.
//
// See the comment for NewBlockTemplate for more information about why the nil
// address handling is useful.
func createCoinbaseTx(coinbaseScript []byte, nextBlockHeight int64, addr btcutil.Address) (*btcutil.Tx, error) {
	// Create the script to pay to the provided payment address if one was
	// specified.  Otherwise create a script that allows the coinbase to be
	// redeemable by anyone.
	var pkScript []byte
	if addr != nil {
		var err error
		pkScript, err = btcscript.PayToAddrScript(addr)
		if err != nil {
			return nil, err
		}
	} else {
		scriptBuilder := btcscript.NewScriptBuilder()
		pkScript = scriptBuilder.AddOp(btcscript.OP_TRUE).Script()
	}

	tx := btcwire.NewMsgTx()
	tx.AddTxIn(&btcwire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *btcwire.NewOutPoint(&btcwire.ShaHash{},
			btcwire.MaxPrevOutIndex),
		SignatureScript: coinbaseScript,
		Sequence:        btcwire.MaxTxInSequenceNum,
	})
	tx.AddTxOut(&btcwire.TxOut{
		Value: btcchain.CalcBlockSubsidy(nextBlockHeight,
			activeNetParams.Params),
		PkScript: pkScript,
	})
	return btcutil.NewTx(tx), nil
}

// calcPriority returns a transaction priority given a transaction and the sum
// of each of its input values multiplied by their age (# of confirmations).
// Thus, the final formula for the priority is:
// sum(inputValue * inputAge) / adjustedTxSize
func calcPriority(tx *btcutil.Tx, serializedTxSize int, inputValueAge float64) float64 {
	// In order to encourage spending multiple old unspent transaction
	// outputs thereby reducing the total set, don't count the constant
	// overhead for each input as well as enough bytes of the signature
	// script to cover a pay-to-script-hash redemption with a compressed
	// pubkey.  This makes additional inputs free by boosting the priority
	// of the transaction accordingly.  No more incentive is given to avoid
	// encouraging gaming future transactions through the use of junk
	// outputs.  This is the same logic used in the reference
	// implementation.
	//
	// The constant overhead for a txin is 41 bytes since the previous
	// outpoint is 36 bytes + 4 bytes for the sequence + 1 byte the
	// signature script length.
	//
	// A compressed pubkey pay-to-script-hash redemption with a maximum len
	// signature is of the form:
	// [OP_DATA_73 <73-byte sig> + OP_DATA_35 + {OP_DATA_33
	// <33 byte compresed pubkey> + OP_CHECKSIG}]
	//
	// Thus 1 + 73 + 1 + 1 + 33 + 1 = 110
	overhead := 0
	for _, txIn := range tx.MsgTx().TxIn {
		// Max inputs + size can't possibly overflow here.
		overhead += 41 + minInt(110, len(txIn.SignatureScript))
	}

	if overhead >= serializedTxSize {
		return 0.0
	}

	return inputValueAge / float64(serializedTxSize-overhead)
}

// spendTransaction updates the passed transaction store by marking the inputs
// to the passed transaction as spent.  It also adds the passed transaction to
// the store at the provided height.
func spendTransaction(txStore btcchain.TxStore, tx *btcutil.Tx, height int64) error {
	for _, txIn := range tx.MsgTx().TxIn {
		originHash := &txIn.PreviousOutPoint.Hash
		originIndex := txIn.PreviousOutPoint.Index
		if originTx, exists := txStore[*originHash]; exists {
			originTx.Spent[originIndex] = true
		}
	}

	txStore[*tx.Sha()] = &btcchain.TxData{
		Tx:          tx,
		Hash:        tx.Sha(),
		BlockHeight: height,
		Spent:       make([]bool, len(tx.MsgTx().TxOut)),
		Err:         nil,
	}

	return nil
}

// logSkippedDeps logs any dependencies which are also skipped as a result of
// skipping a transaction while generating a block template at the trace level.
func logSkippedDeps(tx *btcutil.Tx, deps *list.List) {
	if deps == nil {
		return
	}

	for e := deps.Front(); e != nil; e = e.Next() {
		item := e.Value.(*txPrioItem)
		minrLog.Tracef("Skipping tx %s since it depends on %s\n",
			item.tx.Sha(), tx.Sha())
	}
}

// minimumMedianTime returns the minimum allowed timestamp for a block building
// on the end of the current best chain.  In particular, it is one second after
// the median timestamp of the last several blocks per the chain consensus
// rules.
func minimumMedianTime(chainState *chainState) (time.Time, error) {
	chainState.Lock()
	defer chainState.Unlock()
	if chainState.pastMedianTimeErr != nil {
		return time.Time{}, chainState.pastMedianTimeErr
	}

	return chainState.pastMedianTime.Add(time.Second), nil
}

// medianAdjustedTime returns the current time adjusted to ensure it is at least
// one second after the median timestamp of the last several blocks per the
// chain consensus rules.
func medianAdjustedTime(chainState *chainState) (time.Time, error) {
	chainState.Lock()
	defer chainState.Unlock()
	if chainState.pastMedianTimeErr != nil {
		return time.Time{}, chainState.pastMedianTimeErr
	}

	// The timestamp for the block must not be before the median timestamp
	// of the last several blocks.  Thus, choose the maximum between the
	// current time and one second after the past median time.  The current
	// timestamp is truncated to a second boundary before comparison since a
	// block timestamp does not supported a precision greater than one
	// second.
	newTimestamp := time.Unix(time.Now().Unix(), 0)
	minTimestamp := chainState.pastMedianTime.Add(time.Second)
	if newTimestamp.Before(minTimestamp) {
		newTimestamp = minTimestamp
	}

	return newTimestamp, nil
}

// NewBlockTemplate returns a new block template that is ready to be solved
// using the transactions from the passed transaction memory pool and a coinbase
// that either pays to the passed address if it is not nil, or a coinbase that
// is redeemable by anyone if the passed address is nil.  The nil address
// functionality is useful since there are cases such as the getblocktemplate
// RPC where external mining software is responsible for creating their own
// coinbase which will replace the one generated for the block template.  Thus
// the need to have configured address can be avoided.
//
// The transactions selected and included are prioritized according to several
// factors.  First, each transaction has a priority calculated based on its
// value, age of inputs, and size.  Transactions which consist of larger
// amounts, older inputs, and small sizes have the highest priority.  Second, a
// fee per kilobyte is calculated for each transaction.  Transactions with a
// higher fee per kilobyte are preferred.  Finally, the block generation related
// configuration options are all taken into account.
//
// Transactions which only spend outputs from other transactions already in the
// block chain are immediately added to a priority queue which either
// prioritizes based on the priority (then fee per kilobyte) or the fee per
// kilobyte (then priority) depending on whether or not the BlockPrioritySize
// configuration option allots space for high-priority transactions.
// Transactions which spend outputs from other transactions in the memory pool
// are added to a dependency map so they can be added to the priority queue once
// the transactions they depend on have been included.
//
// Once the high-priority area (if configured) has been filled with transactions,
// or the priority falls below what is considered high-priority, the priority
// queue is updated to prioritize by fees per kilobyte (then priority).
//
// When the fees per kilobyte drop below the TxMinFreeFee configuration option,
// the transaction will be skipped unless there is a BlockMinSize set, in which
// case the block will be filled with the low-fee/free transactions until the
// block size reaches that minimum size.
//
// Any transactions which would cause the block to exceed the BlockMaxSize
// configuration option, exceed the maximum allowed signature operations per
// block, or otherwise cause the block to be invalid are skipped.
//
// Given the above, a block generated by this function is of the following form:
//
//   -----------------------------------  --  --
//  |      Coinbase Transaction         |   |   |
//  |-----------------------------------|   |   |
//  |                                   |   |   | ----- cfg.BlockPrioritySize
//  |   High-priority Transactions      |   |   |
//  |                                   |   |   |
//  |-----------------------------------|   | --
//  |                                   |   |
//  |                                   |   |
//  |                                   |   |--- cfg.BlockMaxSize
//  |  Transactions prioritized by fee  |   |
//  |  until <= cfg.TxMinFreeFee        |   |
//  |                                   |   |
//  |                                   |   |
//  |                                   |   |
//  |-----------------------------------|   |
//  |  Low-fee/Non high-priority (free) |   |
//  |  transactions (while block size   |   |
//  |  <= cfg.BlockMinSize)             |   |
//   -----------------------------------  --
func NewBlockTemplate(mempool *txMemPool, payToAddress btcutil.Address) (*BlockTemplate, error) {
	blockManager := mempool.server.blockManager
	chainState := &blockManager.chainState
	chain := blockManager.blockChain

	// Extend the most recently known best block.
	chainState.Lock()
	prevHash := chainState.newestHash
	nextBlockHeight := chainState.newestHeight + 1
	chainState.Unlock()

	// Create a standard coinbase transaction paying to the provided
	// address.  NOTE: The coinbase value will be updated to include the
	// fees from the selected transactions later after they have actually
	// been selected.  It is created here to detect any errors early
	// before potentially doing a lot of work below.  The extra nonce helps
	// ensure the transaction is not a duplicate transaction (paying the
	// same value to the same public key address would otherwise be an
	// identical transaction for block version 1).
	extraNonce := uint64(0)
	coinbaseScript := standardCoinbaseScript(nextBlockHeight, extraNonce)
	coinbaseTx, err := createCoinbaseTx(coinbaseScript, nextBlockHeight,
		payToAddress)
	if err != nil {
		return nil, err
	}
	numCoinbaseSigOps := int64(btcchain.CountSigOps(coinbaseTx))

	// Get the current memory pool transactions and create a priority queue
	// to hold the transactions which are ready for inclusion into a block
	// along with some priority related and fee metadata.  Reserve the same
	// number of items that are in the memory pool for the priority queue.
	// Also, choose the initial sort order for the priority queue based on
	// whether or not there is an area allocated for high-priority
	// transactions.
	mempoolTxns := mempool.TxDescs()
	sortedByFee := cfg.BlockPrioritySize == 0
	priorityQueue := newTxPriorityQueue(len(mempoolTxns), sortedByFee)

	// Create a slice to hold the transactions to be included in the
	// generated block with reserved space.  Also create a transaction
	// store to house all of the input transactions so multiple lookups
	// can be avoided.
	blockTxns := make([]*btcutil.Tx, 0, len(mempoolTxns))
	blockTxns = append(blockTxns, coinbaseTx)
	blockTxStore := make(btcchain.TxStore)

	// dependers is used to track transactions which depend on another
	// transaction in the memory pool.  This, in conjunction with the
	// dependsOn map kept with each dependent transaction helps quickly
	// determine which dependent transactions are now eligible for inclusion
	// in the block once each transaction has been included.
	dependers := make(map[btcwire.ShaHash]*list.List)

	// Create slices to hold the fees and number of signature operations
	// for each of the selected transactions and add an entry for the
	// coinbase.  This allows the code below to simply append details about
	// a transaction as it is selected for inclusion in the final block.
	// However, since the total fees aren't known yet, use a dummy value for
	// the coinbase fee which will be updated later.
	txFees := make([]int64, 0, len(mempoolTxns))
	txSigOpCounts := make([]int64, 0, len(mempoolTxns))
	txFees = append(txFees, -1) // Updated once known
	txSigOpCounts = append(txSigOpCounts, numCoinbaseSigOps)

	minrLog.Debugf("Considering %d mempool transactions for inclusion to "+
		"new block", len(mempoolTxns))

mempoolLoop:
	for _, txDesc := range mempoolTxns {
		// A block can't have more than one coinbase or contain
		// non-finalized transactions.
		tx := txDesc.Tx
		if btcchain.IsCoinBase(tx) {
			minrLog.Tracef("Skipping coinbase tx %s", tx.Sha())
			continue
		}
		if !btcchain.IsFinalizedTransaction(tx, nextBlockHeight, time.Now()) {
			minrLog.Tracef("Skipping non-finalized tx %s", tx.Sha())
			continue
		}

		// Fetch all of the transactions referenced by the inputs to
		// this transaction.  NOTE: This intentionally does not fetch
		// inputs from the mempool since a transaction which depends on
		// other transactions in the mempool must come after those
		// dependencies in the final generated block.
		txStore, err := chain.FetchTransactionStore(tx)
		if err != nil {
			minrLog.Warnf("Unable to fetch transaction store for "+
				"tx %s: %v", tx.Sha(), err)
			continue
		}

		// Calculate the input value age sum for the transaction.  This
		// is comprised of the sum all of input amounts multiplied by
		// their respective age (number of confirmations since the
		// referenced input transaction).  While doing the above, also
		// setup dependencies for any transactions which reference other
		// transactions in the mempool so they can be properly ordered
		// below.
		prioItem := &txPrioItem{tx: txDesc.Tx}
		inputValueAge := float64(0.0)
		for _, txIn := range tx.MsgTx().TxIn {
			originHash := &txIn.PreviousOutPoint.Hash
			originIndex := txIn.PreviousOutPoint.Index
			txData, exists := txStore[*originHash]
			if !exists || txData.Err != nil || txData.Tx == nil {
				if !mempool.HaveTransaction(originHash) {
					minrLog.Tracef("Skipping tx %s because "+
						"it references tx %s which is "+
						"not available", tx.Sha,
						originHash)
					continue mempoolLoop
				}

				// The transaction is referencing another
				// transaction in the memory pool, so setup an
				// ordering dependency.
				depList, exists := dependers[*originHash]
				if !exists {
					depList = list.New()
					dependers[*originHash] = depList
				}
				depList.PushBack(prioItem)
				if prioItem.dependsOn == nil {
					prioItem.dependsOn = make(
						map[btcwire.ShaHash]struct{})
				}
				prioItem.dependsOn[*originHash] = struct{}{}

				// No need to calculate or sum input value age
				// for this input since it's zero due to
				// the input age multiplier of 0.
				continue
			}

			// Ensure the output index in the referenced transaction
			// is available.
			msgTx := txData.Tx.MsgTx()
			if originIndex > uint32(len(msgTx.TxOut)) {
				minrLog.Tracef("Skipping tx %s because "+
					"it references output %d of tx %s "+
					"which is out of bounds", tx.Sha,
					originIndex, originHash)
				continue mempoolLoop
			}

			// Sum the input value times age.
			originTxOut := txData.Tx.MsgTx().TxOut[originIndex]
			inputValue := originTxOut.Value
			inputAge := nextBlockHeight - txData.BlockHeight
			inputValueAge += float64(inputValue * inputAge)
		}

		// Calculate the final transaction priority using the input
		// value age sum as well as the adjusted transaction size.  The
		// formula is: sum(inputValue * inputAge) / adjustedTxSize
		txSize := tx.MsgTx().SerializeSize()
		prioItem.priority = calcPriority(tx, txSize, inputValueAge)

		// Calculate the fee in Satoshi/KB.
		// NOTE: This is a more precise value than the one calculated
		// during calcMinRelayFee which rounds up to the nearest full
		// kilobyte boundary.  This is beneficial since it provides an
		// incentive to create smaller transactions.
		prioItem.feePerKB = float64(txDesc.Fee) / (float64(txSize) / 1000)
		prioItem.fee = txDesc.Fee

		// Add the transaction to the priority queue to mark it ready
		// for inclusion in the block unless it has dependencies.
		if prioItem.dependsOn == nil {
			heap.Push(priorityQueue, prioItem)
		}

		// Merge the store which contains all of the input transactions
		// for this transaction into the input transaction store.  This
		// allows the code below to avoid a second lookup.
		mergeTxStore(blockTxStore, txStore)
	}

	minrLog.Tracef("Priority queue len %d, dependers len %d",
		priorityQueue.Len(), len(dependers))

	// The starting block size is the size of the block header plus the max
	// possible transaction count size, plus the size of the coinbase
	// transaction.
	blockSize := blockHeaderOverhead + uint32(coinbaseTx.MsgTx().SerializeSize())
	blockSigOps := numCoinbaseSigOps
	totalFees := int64(0)

	// Choose which transactions make it into the block.
	for priorityQueue.Len() > 0 {
		// Grab the highest priority (or highest fee per kilobyte
		// depending on the sort order) transaction.
		prioItem := heap.Pop(priorityQueue).(*txPrioItem)
		tx := prioItem.tx

		// Grab the list of transactions which depend on this one (if
		// any) and remove the entry for this transaction as it will
		// either be included or skipped, but in either case the deps
		// are no longer needed.
		deps := dependers[*tx.Sha()]
		delete(dependers, *tx.Sha())

		// Enforce maximum block size.  Also check for overflow.
		txSize := uint32(tx.MsgTx().SerializeSize())
		blockPlusTxSize := blockSize + txSize
		if blockPlusTxSize < blockSize || blockPlusTxSize >= cfg.BlockMaxSize {
			minrLog.Tracef("Skipping tx %s because it would exceed "+
				"the max block size", tx.Sha())
			logSkippedDeps(tx, deps)
			continue
		}

		// Enforce maximum signature operations per block.  Also check
		// for overflow.
		numSigOps := int64(btcchain.CountSigOps(tx))
		if blockSigOps+numSigOps < blockSigOps ||
			blockSigOps+numSigOps > btcchain.MaxSigOpsPerBlock {
			minrLog.Tracef("Skipping tx %s because it would "+
				"exceed the maximum sigops per block", tx.Sha())
			logSkippedDeps(tx, deps)
			continue
		}
		numP2SHSigOps, err := btcchain.CountP2SHSigOps(tx, false,
			blockTxStore)
		if err != nil {
			minrLog.Tracef("Skipping tx %s due to error in "+
				"CountP2SHSigOps: %v", tx.Sha(), err)
			logSkippedDeps(tx, deps)
			continue
		}
		numSigOps += int64(numP2SHSigOps)
		if blockSigOps+numSigOps < blockSigOps ||
			blockSigOps+numSigOps > btcchain.MaxSigOpsPerBlock {
			minrLog.Tracef("Skipping tx %s because it would "+
				"exceed the maximum sigops per block (p2sh)",
				tx.Sha())
			logSkippedDeps(tx, deps)
			continue
		}

		// Skip free transactions once the block is larger than the
		// minimum block size.
		if sortedByFee && prioItem.feePerKB < minTxRelayFee &&
			blockPlusTxSize >= cfg.BlockMinSize {

			minrLog.Tracef("Skipping tx %s with feePerKB %.2f "+
				"< minTxRelayFee %d and block size %d >= "+
				"minBlockSize %d", tx.Sha(), prioItem.feePerKB,
				minTxRelayFee, blockPlusTxSize,
				cfg.BlockMinSize)
			logSkippedDeps(tx, deps)
			continue
		}

		// Prioritize by fee per kilobyte once the block is larger than
		// the priority size or there are no more high-priority
		// transactions.
		if !sortedByFee && (blockPlusTxSize >= cfg.BlockPrioritySize ||
			prioItem.priority <= minHighPriority) {

			minrLog.Tracef("Switching to sort by fees per "+
				"kilobyte blockSize %d >= BlockPrioritySize "+
				"%d || priority %.2f <= minHighPriority %.2f",
				blockPlusTxSize, cfg.BlockPrioritySize,
				prioItem.priority, minHighPriority)

			sortedByFee = true
			priorityQueue.SetLessFunc(txPQByFee)

			// Put the transaction back into the priority queue and
			// skip it so it is re-priortized by fees if it won't
			// fit into the high-priority section or the priority is
			// too low.  Otherwise this transaction will be the
			// final one in the high-priority section, so just fall
			// though to the code below so it is added now.
			if blockPlusTxSize > cfg.BlockPrioritySize ||
				prioItem.priority < minHighPriority {

				heap.Push(priorityQueue, prioItem)
				continue
			}
		}

		// Ensure the transaction inputs pass all of the necessary
		// preconditions before allowing it to be added to the block.
		_, err = btcchain.CheckTransactionInputs(tx, nextBlockHeight,
			blockTxStore)
		if err != nil {
			minrLog.Tracef("Skipping tx %s due to error in "+
				"CheckTransactionInputs: %v", tx.Sha(), err)
			logSkippedDeps(tx, deps)
			continue
		}
		err = btcchain.ValidateTransactionScripts(tx, blockTxStore,
			standardScriptVerifyFlags)
		if err != nil {
			minrLog.Tracef("Skipping tx %s due to error in "+
				"ValidateTransactionScripts: %v", tx.Sha(), err)
			logSkippedDeps(tx, deps)
			continue
		}

		// Spend the transaction inputs in the block transaction store
		// and add an entry for it to ensure any transactions which
		// reference this one have it available as an input and can
		// ensure they aren't double spending.
		spendTransaction(blockTxStore, tx, nextBlockHeight)

		// Add the transaction to the block, increment counters, and
		// save the fees and signature operation counts to the block
		// template.
		blockTxns = append(blockTxns, tx)
		blockSize += txSize
		blockSigOps += numSigOps
		totalFees += prioItem.fee
		txFees = append(txFees, prioItem.fee)
		txSigOpCounts = append(txSigOpCounts, numSigOps)

		minrLog.Tracef("Adding tx %s (priority %.2f, feePerKB %.2f)",
			prioItem.tx.Sha(), prioItem.priority, prioItem.feePerKB)

		// Add transactions which depend on this one (and also do not
		// have any other unsatisified dependencies) to the priority
		// queue.
		if deps != nil {
			for e := deps.Front(); e != nil; e = e.Next() {
				// Add the transaction to the priority queue if
				// there are no more dependencies after this
				// one.
				item := e.Value.(*txPrioItem)
				delete(item.dependsOn, *tx.Sha())
				if len(item.dependsOn) == 0 {
					heap.Push(priorityQueue, item)
				}
			}
		}
	}

	// Now that the actual transactions have been selected, update the
	// block size for the real transaction count and coinbase value with
	// the total fees accordingly.
	blockSize -= btcwire.MaxVarIntPayload -
		uint32(btcwire.VarIntSerializeSize(uint64(len(blockTxns))))
	coinbaseTx.MsgTx().TxOut[0].Value += totalFees
	txFees[0] = -totalFees

	// Calculate the required difficulty for the block.  The timestamp
	// is potentially adjusted to ensure it comes after the median time of
	// the last several blocks per the chain consensus rules.
	ts, err := medianAdjustedTime(chainState)
	if err != nil {
		return nil, err
	}
	requiredDifficulty, err := blockManager.CalcNextRequiredDifficulty(ts)
	if err != nil {
		return nil, err
	}

	// Create a new block ready to be solved.
	merkles := btcchain.BuildMerkleTreeStore(blockTxns)
	var msgBlock btcwire.MsgBlock
	msgBlock.Header = btcwire.BlockHeader{
		Version:    generatedBlockVersion,
		PrevBlock:  *prevHash,
		MerkleRoot: *merkles[len(merkles)-1],
		Timestamp:  ts,
		Bits:       requiredDifficulty,
	}
	for _, tx := range blockTxns {
		if err := msgBlock.AddTransaction(tx.MsgTx()); err != nil {
			return nil, err
		}
	}

	// Finally, perform a full check on the created block against the chain
	// consensus rules to ensure it properly connects to the current best
	// chain with no issues.
	block := btcutil.NewBlock(&msgBlock)
	block.SetHeight(nextBlockHeight)
	if err := blockManager.CheckConnectBlock(block); err != nil {
		return nil, err
	}

	minrLog.Debugf("Created new block template (%d transactions, %d in "+
		"fees, %d signature operations, %d bytes, target difficulty "+
		"%064x)", len(msgBlock.Transactions), totalFees, blockSigOps,
		blockSize, btcchain.CompactToBig(msgBlock.Header.Bits))

	return &BlockTemplate{
		block:           &msgBlock,
		fees:            txFees,
		sigOpCounts:     txSigOpCounts,
		height:          nextBlockHeight,
		validPayAddress: payToAddress != nil,
	}, nil
}

// UpdateBlockTime updates the timestamp in the header of the passed block to
// the current time while taking into account the median time of the last
// several blocks to ensure the new time is after that time per the chain
// consensus rules.  Finally, it will update the target difficulty if needed
// based on the new time for the test networks since their target difficulty can
// change based upon time.
func UpdateBlockTime(msgBlock *btcwire.MsgBlock, bManager *blockManager) error {
	// The new timestamp is potentially adjusted to ensure it comes after
	// the median time of the last several blocks per the chain consensus
	// rules.
	newTimestamp, err := medianAdjustedTime(&bManager.chainState)
	if err != nil {
		return err
	}
	msgBlock.Header.Timestamp = newTimestamp

	// If running on a network that requires recalculating the difficulty,
	// do so now.
	if activeNetParams.ResetMinDifficulty {
		difficulty, err := bManager.CalcNextRequiredDifficulty(newTimestamp)
		if err != nil {
			return err
		}
		msgBlock.Header.Bits = difficulty
	}

	return nil
}

// UpdateExtraNonce updates the extra nonce in the coinbase script of the passed
// block by regenerating the coinbase script with the passed value and block
// height.  It also recalculates and updates the new merkle root that results
// from changing the coinbase script.
func UpdateExtraNonce(msgBlock *btcwire.MsgBlock, blockHeight int64, extraNonce uint64) error {
	coinbaseScript := standardCoinbaseScript(blockHeight, extraNonce)
	if len(coinbaseScript) > btcchain.MaxCoinbaseScriptLen {
		return fmt.Errorf("coinbase transaction script length "+
			"of %d is out of range (min: %d, max: %d)",
			len(coinbaseScript), btcchain.MinCoinbaseScriptLen,
			btcchain.MaxCoinbaseScriptLen)
	}
	msgBlock.Transactions[0].TxIn[0].SignatureScript = coinbaseScript

	// TODO(davec): A btcutil.Block should use saved in the state to avoid
	// recalculating all of the other transaction hashes.
	// block.Transactions[0].InvalidateCache()

	// Recalculate the merkle root with the updated extra nonce.
	block := btcutil.NewBlock(msgBlock)
	merkles := btcchain.BuildMerkleTreeStore(block.Transactions())
	msgBlock.Header.MerkleRoot = *merkles[len(merkles)-1]
	return nil
}
