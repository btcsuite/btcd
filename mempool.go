// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/list"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/mining"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

const (
	// mempoolHeight is the height used for the "block" height field of the
	// contextual transaction information provided in a transaction store.
	mempoolHeight = 0x7fffffff

	// maxOrphanTransactions is the maximum number of orphan transactions
	// that can be queued.
	maxOrphanTransactions = 1000

	// maxOrphanTxSize is the maximum size allowed for orphan transactions.
	// This helps prevent memory exhaustion attacks from sending a lot of
	// of big orphans.
	maxOrphanTxSize = 5000

	// maxSigOpsPerTx is the maximum number of signature operations
	// in a single transaction we will relay or mine.  It is a fraction
	// of the max signature operations for a block.
	maxSigOpsPerTx = blockchain.MaxSigOpsPerBlock / 5
)

// mempoolTxDesc is a descriptor containing a transaction in the mempool along
// with additional metadata.
type mempoolTxDesc struct {
	mining.TxDesc

	// StartingPriority is the priority of the transaction when it was added
	// to the pool.
	StartingPriority float64
}

// mempoolConfig is a descriptor containing the memory pool configuration.
type mempoolConfig struct {
	// DisableRelayPriority defines whether to relay free or low-fee
	// transactions that do not have enough priority to be relayed.
	DisableRelayPriority bool

	// EnableAddrIndex defines whether the address index should be enabled.
	EnableAddrIndex bool

	// FetchTransactionStore defines the function to use to fetch
	// transacation information.
	FetchTransactionStore func(*btcutil.Tx, bool) (blockchain.TxStore, error)

	// FreeTxRelayLimit defines the given amount in thousands of bytes
	// per minute that transactions with no fee are rate limited to.
	FreeTxRelayLimit float64

	// MaxOrphanTxs defines the maximum number of orphan transactions to
	// keep in memory.
	MaxOrphanTxs int

	// MinRelayTxFee defines the minimum transaction fee in BTC/kB to be
	// considered a non-zero fee.
	MinRelayTxFee btcutil.Amount

	// NewestSha defines the function to retrieve the newest sha
	NewestSha func() (*wire.ShaHash, int32, error)

	// RelayNtfnChan defines the channel to send newly accepted transactions
	// to.  If unset or set to nil, notifications will not be sent.
	RelayNtfnChan chan *btcutil.Tx

	// SigCache defines a signature cache to use.
	SigCache *txscript.SigCache

	// TimeSource defines the timesource to use.
	TimeSource blockchain.MedianTimeSource
}

// txMemPool is used as a source of transactions that need to be mined into
// blocks and relayed to other peers.  It is safe for concurrent access from
// multiple peers.
type txMemPool struct {
	// The following variables must only be used atomically.
	lastUpdated int64 // last time pool was updated

	sync.RWMutex
	cfg           mempoolConfig
	pool          map[wire.ShaHash]*mempoolTxDesc
	orphans       map[wire.ShaHash]*btcutil.Tx
	orphansByPrev map[wire.ShaHash]map[wire.ShaHash]*btcutil.Tx
	addrindex     map[string]map[wire.ShaHash]struct{} // maps address to txs
	outpoints     map[wire.OutPoint]*btcutil.Tx
	pennyTotal    float64 // exponentially decaying total for penny spends.
	lastPennyUnix int64   // unix time of last ``penny spend''
}

// Ensure the txMemPool type implements the mining.TxSource interface.
var _ mining.TxSource = (*txMemPool)(nil)

// removeOrphan is the internal function which implements the public
// RemoveOrphan.  See the comment for RemoveOrphan for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) removeOrphan(txHash *wire.ShaHash) {
	// Nothing to do if passed tx is not an orphan.
	tx, exists := mp.orphans[*txHash]
	if !exists {
		return
	}

	// Remove the reference from the previous orphan index.
	for _, txIn := range tx.MsgTx().TxIn {
		originTxHash := txIn.PreviousOutPoint.Hash
		if orphans, exists := mp.orphansByPrev[originTxHash]; exists {
			delete(orphans, *tx.Sha())

			// Remove the map entry altogether if there are no
			// longer any orphans which depend on it.
			if len(orphans) == 0 {
				delete(mp.orphansByPrev, originTxHash)
			}
		}
	}

	// Remove the transaction from the orphan pool.
	delete(mp.orphans, *txHash)
}

// RemoveOrphan removes the passed orphan transaction from the orphan pool and
// previous orphan index.
//
// This function is safe for concurrent access.
func (mp *txMemPool) RemoveOrphan(txHash *wire.ShaHash) {
	mp.Lock()
	mp.removeOrphan(txHash)
	mp.Unlock()
}

// limitNumOrphans limits the number of orphan transactions by evicting a random
// orphan if adding a new one would cause it to overflow the max allowed.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) limitNumOrphans() error {
	if len(mp.orphans)+1 > mp.cfg.MaxOrphanTxs && mp.cfg.MaxOrphanTxs > 0 {
		// Generate a cryptographically random hash.
		randHashBytes := make([]byte, wire.HashSize)
		_, err := rand.Read(randHashBytes)
		if err != nil {
			return err
		}
		randHashNum := new(big.Int).SetBytes(randHashBytes)

		// Try to find the first entry that is greater than the random
		// hash.  Use the first entry (which is already pseudorandom due
		// to Go's range statement over maps) as a fallback if none of
		// the hashes in the orphan pool are larger than the random
		// hash.
		var foundHash *wire.ShaHash
		for txHash := range mp.orphans {
			if foundHash == nil {
				foundHash = &txHash
			}
			txHashNum := blockchain.ShaHashToBig(&txHash)
			if txHashNum.Cmp(randHashNum) > 0 {
				foundHash = &txHash
				break
			}
		}

		mp.removeOrphan(foundHash)
	}

	return nil
}

// addOrphan adds an orphan transaction to the orphan pool.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) addOrphan(tx *btcutil.Tx) {
	// Limit the number orphan transactions to prevent memory exhaustion.  A
	// random orphan is evicted to make room if needed.
	mp.limitNumOrphans()

	mp.orphans[*tx.Sha()] = tx
	for _, txIn := range tx.MsgTx().TxIn {
		originTxHash := txIn.PreviousOutPoint.Hash
		if _, exists := mp.orphansByPrev[originTxHash]; !exists {
			mp.orphansByPrev[originTxHash] =
				make(map[wire.ShaHash]*btcutil.Tx)
		}
		mp.orphansByPrev[originTxHash][*tx.Sha()] = tx
	}

	txmpLog.Debugf("Stored orphan transaction %v (total: %d)", tx.Sha(),
		len(mp.orphans))
}

// maybeAddOrphan potentially adds an orphan to the orphan pool.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) maybeAddOrphan(tx *btcutil.Tx) error {
	// Ignore orphan transactions that are too large.  This helps avoid
	// a memory exhaustion attack based on sending a lot of really large
	// orphans.  In the case there is a valid transaction larger than this,
	// it will ultimtely be rebroadcast after the parent transactions
	// have been mined or otherwise received.
	//
	// Note that the number of orphan transactions in the orphan pool is
	// also limited, so this equates to a maximum memory used of
	// maxOrphanTxSize * mp.cfg.MaxOrphanTxs (which is ~5MB using the default
	// values at the time this comment was written).
	serializedLen := tx.MsgTx().SerializeSize()
	if serializedLen > maxOrphanTxSize {
		str := fmt.Sprintf("orphan transaction size of %d bytes is "+
			"larger than max allowed size of %d bytes",
			serializedLen, maxOrphanTxSize)
		return txRuleError(wire.RejectNonstandard, str)
	}

	// Add the orphan if the none of the above disqualified it.
	mp.addOrphan(tx)

	return nil
}

// isTransactionInPool returns whether or not the passed transaction already
// exists in the main pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) isTransactionInPool(hash *wire.ShaHash) bool {
	if _, exists := mp.pool[*hash]; exists {
		return true
	}

	return false
}

// IsTransactionInPool returns whether or not the passed transaction already
// exists in the main pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) IsTransactionInPool(hash *wire.ShaHash) bool {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	return mp.isTransactionInPool(hash)
}

// isOrphanInPool returns whether or not the passed transaction already exists
// in the orphan pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) isOrphanInPool(hash *wire.ShaHash) bool {
	if _, exists := mp.orphans[*hash]; exists {
		return true
	}

	return false
}

// IsOrphanInPool returns whether or not the passed transaction already exists
// in the orphan pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) IsOrphanInPool(hash *wire.ShaHash) bool {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	return mp.isOrphanInPool(hash)
}

// haveTransaction returns whether or not the passed transaction already exists
// in the main pool or in the orphan pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) haveTransaction(hash *wire.ShaHash) bool {
	return mp.isTransactionInPool(hash) || mp.isOrphanInPool(hash)
}

// HaveTransaction returns whether or not the passed transaction already exists
// in the main pool or in the orphan pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) HaveTransaction(hash *wire.ShaHash) bool {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	return mp.haveTransaction(hash)
}

// removeTransaction is the internal function which implements the public
// RemoveTransaction.  See the comment for RemoveTransaction for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) removeTransaction(tx *btcutil.Tx, removeRedeemers bool) {
	txHash := tx.Sha()
	if removeRedeemers {
		// Remove any transactions which rely on this one.
		for i := uint32(0); i < uint32(len(tx.MsgTx().TxOut)); i++ {
			outpoint := wire.NewOutPoint(txHash, i)
			if txRedeemer, exists := mp.outpoints[*outpoint]; exists {
				mp.removeTransaction(txRedeemer, true)
			}
		}
	}

	// Remove the transaction and mark the referenced outpoints as unspent
	// by the pool.
	if txDesc, exists := mp.pool[*txHash]; exists {
		if mp.cfg.EnableAddrIndex {
			mp.removeTransactionFromAddrIndex(tx)
		}

		for _, txIn := range txDesc.Tx.MsgTx().TxIn {
			delete(mp.outpoints, txIn.PreviousOutPoint)
		}
		delete(mp.pool, *txHash)
		atomic.StoreInt64(&mp.lastUpdated, time.Now().Unix())
	}

}

// removeTransactionFromAddrIndex removes the passed transaction from our
// address based index.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) removeTransactionFromAddrIndex(tx *btcutil.Tx) error {
	previousOutputScripts, err := mp.fetchReferencedOutputScripts(tx)
	if err != nil {
		txmpLog.Errorf("Unable to obtain referenced output scripts for "+
			"the passed tx (addrindex): %v", err)
		return err
	}

	for _, pkScript := range previousOutputScripts {
		mp.removeScriptFromAddrIndex(pkScript, tx)
	}

	for _, txOut := range tx.MsgTx().TxOut {
		mp.removeScriptFromAddrIndex(txOut.PkScript, tx)
	}

	return nil
}

// removeScriptFromAddrIndex dissociates the address encoded by the
// passed pkScript from the passed tx in our address based tx index.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) removeScriptFromAddrIndex(pkScript []byte, tx *btcutil.Tx) error {
	_, addresses, _, err := txscript.ExtractPkScriptAddrs(pkScript,
		activeNetParams.Params)
	if err != nil {
		txmpLog.Errorf("Unable to extract encoded addresses from script "+
			"for addrindex (addrindex): %v", err)
		return err
	}
	for _, addr := range addresses {
		delete(mp.addrindex[addr.EncodeAddress()], *tx.Sha())
	}

	return nil
}

// RemoveTransaction removes the passed transaction from the mempool. If
// removeRedeemers flag is set, any transactions that redeem outputs from the
// removed transaction will also be removed recursively from the mempool, as
// they would otherwise become orphan.
//
// This function is safe for concurrent access.
func (mp *txMemPool) RemoveTransaction(tx *btcutil.Tx, removeRedeemers bool) {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	mp.removeTransaction(tx, removeRedeemers)
}

// RemoveDoubleSpends removes all transactions which spend outputs spent by the
// passed transaction from the memory pool.  Removing those transactions then
// leads to removing all transactions which rely on them, recursively.  This is
// necessary when a block is connected to the main chain because the block may
// contain transactions which were previously unknown to the memory pool
//
// This function is safe for concurrent access.
func (mp *txMemPool) RemoveDoubleSpends(tx *btcutil.Tx) {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	for _, txIn := range tx.MsgTx().TxIn {
		if txRedeemer, ok := mp.outpoints[txIn.PreviousOutPoint]; ok {
			if !txRedeemer.Sha().IsEqual(tx.Sha()) {
				mp.removeTransaction(txRedeemer, true)
			}
		}
	}
}

// addTransaction adds the passed transaction to the memory pool.  It should
// not be called directly as it doesn't perform any validation.  This is a
// helper for maybeAcceptTransaction.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) addTransaction(txStore blockchain.TxStore, tx *btcutil.Tx, height int32, fee int64) {
	// Add the transaction to the pool and mark the referenced outpoints
	// as spent by the pool.
	mp.pool[*tx.Sha()] = &mempoolTxDesc{
		TxDesc: mining.TxDesc{
			Tx:     tx,
			Added:  time.Now(),
			Height: height,
			Fee:    fee,
		},
		StartingPriority: calcPriority(tx.MsgTx(), txStore, height),
	}
	for _, txIn := range tx.MsgTx().TxIn {
		mp.outpoints[txIn.PreviousOutPoint] = tx
	}
	atomic.StoreInt64(&mp.lastUpdated, time.Now().Unix())

	if mp.cfg.EnableAddrIndex {
		mp.addTransactionToAddrIndex(tx)
	}
}

// addTransactionToAddrIndex adds all addresses related to the transaction to
// our in-memory address index. Note that this address is only populated when
// we're running with the optional address index activated.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) addTransactionToAddrIndex(tx *btcutil.Tx) error {
	previousOutScripts, err := mp.fetchReferencedOutputScripts(tx)
	if err != nil {
		txmpLog.Errorf("Unable to obtain referenced output scripts for "+
			"the passed tx (addrindex): %v", err)
		return err
	}
	// Index addresses of all referenced previous output tx's.
	for _, pkScript := range previousOutScripts {
		mp.indexScriptAddressToTx(pkScript, tx)
	}

	// Index addresses of all created outputs.
	for _, txOut := range tx.MsgTx().TxOut {
		mp.indexScriptAddressToTx(txOut.PkScript, tx)
	}

	return nil
}

// fetchReferencedOutputScripts looks up and returns all the scriptPubKeys
// referenced by inputs of the passed transaction.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) fetchReferencedOutputScripts(tx *btcutil.Tx) ([][]byte, error) {
	txStore, err := mp.fetchInputTransactions(tx, false)
	if err != nil || len(txStore) == 0 {
		return nil, err
	}

	previousOutScripts := make([][]byte, 0, len(tx.MsgTx().TxIn))
	for _, txIn := range tx.MsgTx().TxIn {
		outPoint := txIn.PreviousOutPoint
		if txStore[outPoint.Hash].Err == nil {
			referencedOutPoint := txStore[outPoint.Hash].Tx.MsgTx().TxOut[outPoint.Index]
			previousOutScripts = append(previousOutScripts, referencedOutPoint.PkScript)
		}
	}
	return previousOutScripts, nil
}

// indexScriptByAddress alters our address index by indexing the payment address
// encoded by the passed scriptPubKey to the passed transaction.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) indexScriptAddressToTx(pkScript []byte, tx *btcutil.Tx) error {
	_, addresses, _, err := txscript.ExtractPkScriptAddrs(pkScript,
		activeNetParams.Params)
	if err != nil {
		txmpLog.Errorf("Unable to extract encoded addresses from script "+
			"for addrindex: %v", err)
		return err
	}

	for _, addr := range addresses {
		if mp.addrindex[addr.EncodeAddress()] == nil {
			mp.addrindex[addr.EncodeAddress()] = make(map[wire.ShaHash]struct{})
		}
		mp.addrindex[addr.EncodeAddress()][*tx.Sha()] = struct{}{}
	}

	return nil
}

// checkPoolDoubleSpend checks whether or not the passed transaction is
// attempting to spend coins already spent by other transactions in the pool.
// Note it does not check for double spends against transactions already in the
// main chain.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) checkPoolDoubleSpend(tx *btcutil.Tx) error {
	for _, txIn := range tx.MsgTx().TxIn {
		if txR, exists := mp.outpoints[txIn.PreviousOutPoint]; exists {
			str := fmt.Sprintf("output %v already spent by "+
				"transaction %v in the memory pool",
				txIn.PreviousOutPoint, txR.Sha())
			return txRuleError(wire.RejectDuplicate, str)
		}
	}

	return nil
}

// fetchInputTransactions fetches the input transactions referenced by the
// passed transaction.  First, it fetches from the main chain, then it tries to
// fetch any missing inputs from the transaction pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) fetchInputTransactions(tx *btcutil.Tx, includeSpent bool) (blockchain.TxStore, error) {
	txStore, err := mp.cfg.FetchTransactionStore(tx, includeSpent)
	if err != nil {
		return nil, err
	}

	// Attempt to populate any missing inputs from the transaction pool.
	for _, txD := range txStore {
		if txD.Err == database.ErrTxShaMissing || txD.Tx == nil {
			if poolTxDesc, exists := mp.pool[*txD.Hash]; exists {
				poolTx := poolTxDesc.Tx
				txD.Tx = poolTx
				txD.BlockHeight = mempoolHeight
				txD.Spent = make([]bool, len(poolTx.MsgTx().TxOut))
				txD.Err = nil
			}
		}
	}

	return txStore, nil
}

// FetchTransaction returns the requested transaction from the transaction pool.
// This only fetches from the main transaction pool and does not include
// orphans.
//
// This function is safe for concurrent access.
func (mp *txMemPool) FetchTransaction(txHash *wire.ShaHash) (*btcutil.Tx, error) {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	if txDesc, exists := mp.pool[*txHash]; exists {
		return txDesc.Tx, nil
	}

	return nil, fmt.Errorf("transaction is not in the pool")
}

// FilterTransactionsByAddress returns all transactions currently in the
// mempool that either create an output to the passed address or spend a
// previously created output to the address.
func (mp *txMemPool) FilterTransactionsByAddress(addr btcutil.Address) ([]*btcutil.Tx, error) {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	if txs, exists := mp.addrindex[addr.EncodeAddress()]; exists {
		addressTxs := make([]*btcutil.Tx, 0, len(txs))
		for txHash := range txs {
			if txD, exists := mp.pool[txHash]; exists {
				addressTxs = append(addressTxs, txD.Tx)
			}
		}
		return addressTxs, nil
	}

	return nil, fmt.Errorf("address does not have any transactions in the pool")
}

// maybeAcceptTransaction is the internal function which implements the public
// MaybeAcceptTransaction.  See the comment for MaybeAcceptTransaction for
// more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) maybeAcceptTransaction(tx *btcutil.Tx, isNew, rateLimit bool) ([]*wire.ShaHash, error) {
	txHash := tx.Sha()

	// Don't accept the transaction if it already exists in the pool.  This
	// applies to orphan transactions as well.  This check is intended to
	// be a quick check to weed out duplicates.
	if mp.haveTransaction(txHash) {
		str := fmt.Sprintf("already have transaction %v", txHash)
		return nil, txRuleError(wire.RejectDuplicate, str)
	}

	// Perform preliminary sanity checks on the transaction.  This makes
	// use of btcchain which contains the invariant rules for what
	// transactions are allowed into blocks.
	err := blockchain.CheckTransactionSanity(tx)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// A standalone transaction must not be a coinbase transaction.
	if blockchain.IsCoinBase(tx) {
		str := fmt.Sprintf("transaction %v is an individual coinbase",
			txHash)
		return nil, txRuleError(wire.RejectInvalid, str)
	}

	// Don't accept transactions with a lock time after the maximum int32
	// value for now.  This is an artifact of older bitcoind clients which
	// treated this field as an int32 and would treat anything larger
	// incorrectly (as negative).
	if tx.MsgTx().LockTime > math.MaxInt32 {
		str := fmt.Sprintf("transaction %v has a lock time after "+
			"2038 which is not accepted yet", txHash)
		return nil, txRuleError(wire.RejectNonstandard, str)
	}

	// Get the current height of the main chain.  A standalone transaction
	// will be mined into the next block at best, so it's height is at least
	// one more than the current height.
	_, curHeight, err := mp.cfg.NewestSha()
	if err != nil {
		// This is an unexpected error so don't turn it into a rule
		// error.
		return nil, err
	}
	nextBlockHeight := curHeight + 1

	// Don't allow non-standard transactions if the network parameters
	// forbid their relaying.
	if !activeNetParams.RelayNonStdTxs {
		err := checkTransactionStandard(tx, nextBlockHeight,
			mp.cfg.TimeSource, mp.cfg.MinRelayTxFee)
		if err != nil {
			// Attempt to extract a reject code from the error so
			// it can be retained.  When not possible, fall back to
			// a non standard error.
			rejectCode, found := extractRejectCode(err)
			if !found {
				rejectCode = wire.RejectNonstandard
			}
			str := fmt.Sprintf("transaction %v is not standard: %v",
				txHash, err)
			return nil, txRuleError(rejectCode, str)
		}
	}

	// The transaction may not use any of the same outputs as other
	// transactions already in the pool as that would ultimately result in a
	// double spend.  This check is intended to be quick and therefore only
	// detects double spends within the transaction pool itself.  The
	// transaction could still be double spending coins from the main chain
	// at this point.  There is a more in-depth check that happens later
	// after fetching the referenced transaction inputs from the main chain
	// which examines the actual spend data and prevents double spends.
	err = mp.checkPoolDoubleSpend(tx)
	if err != nil {
		return nil, err
	}

	// Fetch all of the transactions referenced by the inputs to this
	// transaction.  This function also attempts to fetch the transaction
	// itself to be used for detecting a duplicate transaction without
	// needing to do a separate lookup.
	txStore, err := mp.fetchInputTransactions(tx, false)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Don't allow the transaction if it exists in the main chain and is not
	// not already fully spent.
	if txD, exists := txStore[*txHash]; exists && txD.Err == nil {
		for _, isOutputSpent := range txD.Spent {
			if !isOutputSpent {
				return nil, txRuleError(wire.RejectDuplicate,
					"transaction already exists")
			}
		}
	}
	delete(txStore, *txHash)

	// Transaction is an orphan if any of the referenced input transactions
	// don't exist.  Adding orphans to the orphan pool is not handled by
	// this function, and the caller should use maybeAddOrphan if this
	// behavior is desired.
	var missingParents []*wire.ShaHash
	for _, txD := range txStore {
		if txD.Err == database.ErrTxShaMissing {
			missingParents = append(missingParents, txD.Hash)
		}
	}
	if len(missingParents) > 0 {
		return missingParents, nil
	}

	// Perform several checks on the transaction inputs using the invariant
	// rules in btcchain for what transactions are allowed into blocks.
	// Also returns the fees associated with the transaction which will be
	// used later.
	txFee, err := blockchain.CheckTransactionInputs(tx, nextBlockHeight, txStore)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Don't allow transactions with non-standard inputs if the network
	// parameters forbid their relaying.
	if !activeNetParams.RelayNonStdTxs {
		err := checkInputsStandard(tx, txStore)
		if err != nil {
			// Attempt to extract a reject code from the error so
			// it can be retained.  When not possible, fall back to
			// a non standard error.
			rejectCode, found := extractRejectCode(err)
			if !found {
				rejectCode = wire.RejectNonstandard
			}
			str := fmt.Sprintf("transaction %v has a non-standard "+
				"input: %v", txHash, err)
			return nil, txRuleError(rejectCode, str)
		}
	}

	// NOTE: if you modify this code to accept non-standard transactions,
	// you should add code here to check that the transaction does a
	// reasonable number of ECDSA signature verifications.

	// Don't allow transactions with an excessive number of signature
	// operations which would result in making it impossible to mine.  Since
	// the coinbase address itself can contain signature operations, the
	// maximum allowed signature operations per transaction is less than
	// the maximum allowed signature operations per block.
	numSigOps, err := blockchain.CountP2SHSigOps(tx, false, txStore)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}
	numSigOps += blockchain.CountSigOps(tx)
	if numSigOps > maxSigOpsPerTx {
		str := fmt.Sprintf("transaction %v has too many sigops: %d > %d",
			txHash, numSigOps, maxSigOpsPerTx)
		return nil, txRuleError(wire.RejectNonstandard, str)
	}

	// Don't allow transactions with fees too low to get into a mined block.
	//
	// Most miners allow a free transaction area in blocks they mine to go
	// alongside the area used for high-priority transactions as well as
	// transactions with fees.  A transaction size of up to 1000 bytes is
	// considered safe to go into this section.  Further, the minimum fee
	// calculated below on its own would encourage several small
	// transactions to avoid fees rather than one single larger transaction
	// which is more desirable.  Therefore, as long as the size of the
	// transaction does not exceeed 1000 less than the reserved space for
	// high-priority transactions, don't require a fee for it.
	serializedSize := int64(tx.MsgTx().SerializeSize())
	minFee := calcMinRequiredTxRelayFee(serializedSize, mp.cfg.MinRelayTxFee)
	if serializedSize >= (defaultBlockPrioritySize-1000) && txFee < minFee {
		str := fmt.Sprintf("transaction %v has %d fees which is under "+
			"the required amount of %d", txHash, txFee,
			minFee)
		return nil, txRuleError(wire.RejectInsufficientFee, str)
	}

	// Require that free transactions have sufficient priority to be mined
	// in the next block.  Transactions which are being added back to the
	// memory pool from blocks that have been disconnected during a reorg
	// are exempted.
	if isNew && !mp.cfg.DisableRelayPriority && txFee < minFee {
		currentPriority := calcPriority(tx.MsgTx(), txStore,
			nextBlockHeight)
		if currentPriority <= minHighPriority {
			str := fmt.Sprintf("transaction %v has insufficient "+
				"priority (%g <= %g)", txHash,
				currentPriority, minHighPriority)
			return nil, txRuleError(wire.RejectInsufficientFee, str)
		}
	}

	// Free-to-relay transactions are rate limited here to prevent
	// penny-flooding with tiny transactions as a form of attack.
	if rateLimit && txFee < minFee {
		nowUnix := time.Now().Unix()
		// we decay passed data with an exponentially decaying ~10
		// minutes window - matches bitcoind handling.
		mp.pennyTotal *= math.Pow(1.0-1.0/600.0,
			float64(nowUnix-mp.lastPennyUnix))
		mp.lastPennyUnix = nowUnix

		// Are we still over the limit?
		if mp.pennyTotal >= mp.cfg.FreeTxRelayLimit*10*1000 {
			str := fmt.Sprintf("transaction %v has been rejected "+
				"by the rate limiter due to low fees", txHash)
			return nil, txRuleError(wire.RejectInsufficientFee, str)
		}
		oldTotal := mp.pennyTotal

		mp.pennyTotal += float64(serializedSize)
		txmpLog.Tracef("rate limit: curTotal %v, nextTotal: %v, "+
			"limit %v", oldTotal, mp.pennyTotal,
			mp.cfg.FreeTxRelayLimit*10*1000)
	}

	// Verify crypto signatures for each input and reject the transaction if
	// any don't verify.
	err = blockchain.ValidateTransactionScripts(tx, txStore,
		txscript.StandardVerifyFlags, mp.cfg.SigCache)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Add to transaction pool.
	mp.addTransaction(txStore, tx, curHeight, txFee)

	txmpLog.Debugf("Accepted transaction %v (pool size: %v)", txHash,
		len(mp.pool))

	return nil, nil
}

// MaybeAcceptTransaction is the main workhorse for handling insertion of new
// free-standing transactions into a memory pool.  It includes functionality
// such as rejecting duplicate transactions, ensuring transactions follow all
// rules, detecting orphan transactions, and insertion into the memory pool.
//
// If the transaction is an orphan (missing parent transactions), the
// transaction is NOT added to the orphan pool, but each unknown referenced
// parent is returned.  Use ProcessTransaction instead if new orphans should
// be added to the orphan pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) MaybeAcceptTransaction(tx *btcutil.Tx, isNew, rateLimit bool) ([]*wire.ShaHash, error) {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	return mp.maybeAcceptTransaction(tx, isNew, rateLimit)
}

// processOrphans is the internal function which implements the public
// ProcessOrphans.  See the comment for ProcessOrphans for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) processOrphans(hash *wire.ShaHash) {
	// Start with processing at least the passed hash.
	processHashes := list.New()
	processHashes.PushBack(hash)
	for processHashes.Len() > 0 {
		// Pop the first hash to process.
		firstElement := processHashes.Remove(processHashes.Front())
		processHash := firstElement.(*wire.ShaHash)

		// Look up all orphans that are referenced by the transaction we
		// just accepted.  This will typically only be one, but it could
		// be multiple if the referenced transaction contains multiple
		// outputs.  Skip to the next item on the list of hashes to
		// process if there are none.
		orphans, exists := mp.orphansByPrev[*processHash]
		if !exists || orphans == nil {
			continue
		}

		for _, tx := range orphans {
			// Remove the orphan from the orphan pool.  Current
			// behavior requires that all saved orphans with
			// a newly accepted parent are removed from the orphan
			// pool and potentially added to the memory pool, but
			// transactions which cannot be added to memory pool
			// (including due to still being orphans) are expunged
			// from the orphan pool.
			//
			// TODO(jrick): The above described behavior sounds
			// like a bug, and I think we should investigate
			// potentially moving orphans to the memory pool, but
			// leaving them in the orphan pool if not all parent
			// transactions are known yet.
			orphanHash := tx.Sha()
			mp.removeOrphan(orphanHash)

			// Potentially accept the transaction into the
			// transaction pool.
			missingParents, err := mp.maybeAcceptTransaction(tx,
				true, true)
			if err != nil {
				// TODO: Remove orphans that depend on this
				// failed transaction.
				txmpLog.Debugf("Unable to move "+
					"orphan transaction %v to mempool: %v",
					tx.Sha(), err)
				continue
			}

			if len(missingParents) > 0 {
				// Transaction is still an orphan, so add it
				// back.
				mp.addOrphan(tx)
				continue
			}

			// Notify the caller of the new tx added to mempool.
			if mp.cfg.RelayNtfnChan != nil {
				mp.cfg.RelayNtfnChan <- tx
			}

			// Add this transaction to the list of transactions to
			// process so any orphans that depend on this one are
			// handled too.
			//
			// TODO(jrick): In the case that this is still an orphan,
			// we know that any other transactions in the orphan
			// pool with this orphan as their parent are still
			// orphans as well, and should be removed.  While
			// recursively calling removeOrphan and
			// maybeAcceptTransaction on these transactions is not
			// wrong per se, it is overkill if all we care about is
			// recursively removing child transactions of this
			// orphan.
			processHashes.PushBack(orphanHash)
		}
	}
}

// ProcessOrphans determines if there are any orphans which depend on the passed
// transaction hash (it is possible that they are no longer orphans) and
// potentially accepts them to the memory pool.  It repeats the process for the
// newly accepted transactions (to detect further orphans which may no longer be
// orphans) until there are no more.
//
// This function is safe for concurrent access.
func (mp *txMemPool) ProcessOrphans(hash *wire.ShaHash) {
	mp.Lock()
	mp.processOrphans(hash)
	mp.Unlock()
}

// ProcessTransaction is the main workhorse for handling insertion of new
// free-standing transactions into the memory pool.  It includes functionality
// such as rejecting duplicate transactions, ensuring transactions follow all
// rules, orphan transaction handling, and insertion into the memory pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) ProcessTransaction(tx *btcutil.Tx, allowOrphan, rateLimit bool) error {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	txmpLog.Tracef("Processing transaction %v", tx.Sha())

	// Potentially accept the transaction to the memory pool.
	missingParents, err := mp.maybeAcceptTransaction(tx, true, rateLimit)
	if err != nil {
		return err
	}

	if len(missingParents) == 0 {
		// Notify the caller that the tx was added to the mempool.
		if mp.cfg.RelayNtfnChan != nil {
			mp.cfg.RelayNtfnChan <- tx
		}

		// Accept any orphan transactions that depend on this
		// transaction (they may no longer be orphans if all inputs
		// are now available) and repeat for those accepted
		// transactions until there are no more.
		mp.processOrphans(tx.Sha())
	} else {
		// The transaction is an orphan (has inputs missing).  Reject
		// it if the flag to allow orphans is not set.
		if !allowOrphan {
			// Only use the first missing parent transaction in
			// the error message.
			//
			// NOTE: RejectDuplicate is really not an accurate
			// reject code here, but it matches the reference
			// implementation and there isn't a better choice due
			// to the limited number of reject codes.  Missing
			// inputs is assumed to mean they are already spent
			// which is not really always the case.
			str := fmt.Sprintf("orphan transaction %v references "+
				"outputs of unknown or fully-spent "+
				"transaction %v", tx.Sha(), missingParents[0])
			return txRuleError(wire.RejectDuplicate, str)
		}

		// Potentially add the orphan transaction to the orphan pool.
		err := mp.maybeAddOrphan(tx)
		if err != nil {
			return err
		}
	}

	return nil
}

// Count returns the number of transactions in the main pool.  It does not
// include the orphan pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) Count() int {
	mp.RLock()
	defer mp.RUnlock()

	return len(mp.pool)
}

// TxShas returns a slice of hashes for all of the transactions in the memory
// pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) TxShas() []*wire.ShaHash {
	mp.RLock()
	defer mp.RUnlock()

	hashes := make([]*wire.ShaHash, len(mp.pool))
	i := 0
	for hash := range mp.pool {
		hashCopy := hash
		hashes[i] = &hashCopy
		i++
	}

	return hashes
}

// TxDescs returns a slice of descriptors for all the transactions in the pool.
// The descriptors are to be treated as read only.
//
// This function is safe for concurrent access.
func (mp *txMemPool) TxDescs() []*mempoolTxDesc {
	mp.RLock()
	defer mp.RUnlock()

	descs := make([]*mempoolTxDesc, len(mp.pool))
	i := 0
	for _, desc := range mp.pool {
		descs[i] = desc
		i++
	}

	return descs
}

// MiningDescs returns a slice of mining descriptors for all the transactions
// in the pool.
//
// This is part of the mining.TxSource interface implementation and is safe for
// concurrent access as required by the interface contract.
func (mp *txMemPool) MiningDescs() []*mining.TxDesc {
	mp.RLock()
	defer mp.RUnlock()

	descs := make([]*mining.TxDesc, len(mp.pool))
	i := 0
	for _, desc := range mp.pool {
		descs[i] = &desc.TxDesc
		i++
	}

	return descs
}

// LastUpdated returns the last time a transaction was added to or removed from
// the main pool.  It does not include the orphan pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) LastUpdated() time.Time {
	return time.Unix(atomic.LoadInt64(&mp.lastUpdated), 0)
}

// newTxMemPool returns a new memory pool for validating and storing standalone
// transactions until they are mined into a block.
func newTxMemPool(cfg *mempoolConfig) *txMemPool {
	memPool := &txMemPool{
		cfg:           *cfg,
		pool:          make(map[wire.ShaHash]*mempoolTxDesc),
		orphans:       make(map[wire.ShaHash]*btcutil.Tx),
		orphansByPrev: make(map[wire.ShaHash]map[wire.ShaHash]*btcutil.Tx),
		outpoints:     make(map[wire.OutPoint]*btcutil.Tx),
	}
	if cfg.EnableAddrIndex {
		memPool.addrindex = make(map[string]map[wire.ShaHash]struct{})
	}
	return memPool
}
