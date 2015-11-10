// Copyright (c) 2013-2014 The btcsuite developers
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
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/database"
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

	// maxStandardTxSize is the maximum size allowed for transactions that
	// are considered standard and will therefore be relayed and considered
	// for mining.
	maxStandardTxSize = 100000

	// maxStandardSigScriptSize is the maximum size allowed for a
	// transaction input signature script to be considered standard.  This
	// value allows for a 15-of-15 CHECKMULTISIG pay-to-script-hash with
	// compressed keys.
	//
	// The form of the overall script is: OP_0 <15 signatures> OP_PUSHDATA2
	// <2 bytes len> [OP_15 <15 pubkeys> OP_15 OP_CHECKMULTISIG]
	//
	// For the p2sh script portion, each of the 15 compressed pubkeys are
	// 33 bytes (plus one for the OP_DATA_33 opcode), and the thus it totals
	// to (15*34)+3 = 513 bytes.  Next, each of the 15 signatures is a max
	// of 73 bytes (plus one for the OP_DATA_73 opcode).  Also, there is one
	// extra byte for the initial extra OP_0 push and 3 bytes for the
	// OP_PUSHDATA2 needed to specify the 513 bytes for the script push.
	// That brings the total to 1+(15*74)+3+513 = 1627.  This value also
	// adds a few extra bytes to provide a little buffer.
	// (1 + 15*74 + 3) + (15*34 + 3) + 23 = 1650
	maxStandardSigScriptSize = 1650

	// defaultMinRelayTxFee is the minimum fee in satoshi that is required
	// for a transaction to be treated as free for relay and mining
	// purposes.  It is also used to help determine if a transaction is
	// considered dust and as a base for calculating minimum required fees
	// for larger transactions.  This value is in Satoshi/1000 bytes.
	defaultMinRelayTxFee = btcutil.Amount(1000)
)

// TxDesc is a descriptor containing a transaction in the mempool and the
// metadata we store about it.
type TxDesc struct {
	Tx               *btcutil.Tx // Transaction.
	Added            time.Time   // Time when added to pool.
	Height           int32       // Blockheight when added to pool.
	Fee              int64       // Transaction fees.
	startingPriority float64     // Priority when added to the pool.
}

// txMemPool is used as a source of transactions that need to be mined into
// blocks and relayed to other peers.  It is safe for concurrent access from
// multiple peers.
type txMemPool struct {
	sync.RWMutex
	server        *server
	pool          map[wire.ShaHash]*TxDesc
	orphans       map[wire.ShaHash]*btcutil.Tx
	orphansByPrev map[wire.ShaHash]*list.List
	addrindex     map[string]map[wire.ShaHash]struct{} // maps address to txs
	outpoints     map[wire.OutPoint]*btcutil.Tx
	lastUpdated   time.Time // last time pool was updated
	pennyTotal    float64   // exponentially decaying total for penny spends.
	lastPennyUnix int64     // unix time of last ``penny spend''
}

// checkTransactionStandard performs a series of checks on a transaction to
// ensure it is a "standard" transaction.  A standard transaction is one that
// conforms to several additional limiting cases over what is considered a
// "sane" transaction such as having a version in the supported range, being
// finalized, conforming to more stringent size constraints, having scripts
// of recognized forms, and not containing "dust" outputs (those that are
// so small it costs more to process them than they are worth).
func (mp *txMemPool) checkTransactionStandard(tx *btcutil.Tx, height int32) error {
	msgTx := tx.MsgTx()

	// The transaction must be a currently supported version.
	if msgTx.Version > wire.TxVersion || msgTx.Version < 1 {
		str := fmt.Sprintf("transaction version %d is not in the "+
			"valid range of %d-%d", msgTx.Version, 1,
			wire.TxVersion)
		return txRuleError(wire.RejectNonstandard, str)
	}

	// The transaction must be finalized to be standard and therefore
	// considered for inclusion in a block.
	adjustedTime := mp.server.timeSource.AdjustedTime()
	if !blockchain.IsFinalizedTransaction(tx, height, adjustedTime) {
		return txRuleError(wire.RejectNonstandard,
			"transaction is not finalized")
	}

	// Since extremely large transactions with a lot of inputs can cost
	// almost as much to process as the sender fees, limit the maximum
	// size of a transaction.  This also helps mitigate CPU exhaustion
	// attacks.
	serializedLen := msgTx.SerializeSize()
	if serializedLen > maxStandardTxSize {
		str := fmt.Sprintf("transaction size of %v is larger than max "+
			"allowed size of %v", serializedLen, maxStandardTxSize)
		return txRuleError(wire.RejectNonstandard, str)
	}

	for i, txIn := range msgTx.TxIn {
		// Each transaction input signature script must not exceed the
		// maximum size allowed for a standard transaction.  See
		// the comment on maxStandardSigScriptSize for more details.
		sigScriptLen := len(txIn.SignatureScript)
		if sigScriptLen > maxStandardSigScriptSize {
			str := fmt.Sprintf("transaction input %d: signature "+
				"script size of %d bytes is large than max "+
				"allowed size of %d bytes", i, sigScriptLen,
				maxStandardSigScriptSize)
			return txRuleError(wire.RejectNonstandard, str)
		}

		// Each transaction input signature script must only contain
		// opcodes which push data onto the stack.
		if !txscript.IsPushOnlyScript(txIn.SignatureScript) {
			str := fmt.Sprintf("transaction input %d: signature "+
				"script is not push only", i)
			return txRuleError(wire.RejectNonstandard, str)
		}
	}

	// None of the output public key scripts can be a non-standard script or
	// be "dust" (except when the script is a null data script).
	numNullDataOutputs := 0
	for i, txOut := range msgTx.TxOut {
		scriptClass := txscript.GetScriptClass(txOut.PkScript)
		err := checkPkScriptStandard(txOut.PkScript, scriptClass)
		if err != nil {
			// Attempt to extract a reject code from the error so
			// it can be retained.  When not possible, fall back to
			// a non standard error.
			rejectCode, found := extractRejectCode(err)
			if !found {
				rejectCode = wire.RejectNonstandard
			}
			str := fmt.Sprintf("transaction output %d: %v", i, err)
			return txRuleError(rejectCode, str)
		}

		// Accumulate the number of outputs which only carry data.  For
		// all other script types, ensure the output value is not
		// "dust".
		if scriptClass == txscript.NullDataTy {
			numNullDataOutputs++
		} else if isDust(txOut, cfg.minRelayTxFee) {
			str := fmt.Sprintf("transaction output %d: payment "+
				"of %d is dust", i, txOut.Value)
			return txRuleError(wire.RejectDust, str)
		}
	}

	// A standard transaction must not have more than one output script that
	// only carries data.
	if numNullDataOutputs > 1 {
		str := "more than one transaction output in a nulldata script"
		return txRuleError(wire.RejectNonstandard, str)
	}

	return nil
}

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
			for e := orphans.Front(); e != nil; e = e.Next() {
				if e.Value.(*btcutil.Tx) == tx {
					orphans.Remove(e)
					break
				}
			}

			// Remove the map entry altogether if there are no
			// longer any orphans which depend on it.
			if orphans.Len() == 0 {
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
	if len(mp.orphans)+1 > cfg.MaxOrphanTxs && cfg.MaxOrphanTxs > 0 {
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
		if mp.orphansByPrev[originTxHash] == nil {
			mp.orphansByPrev[originTxHash] = list.New()
		}
		mp.orphansByPrev[originTxHash].PushBack(tx)
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
	// maxOrphanTxSize * cfg.MaxOrphanTxs (which is ~5MB using the default
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
		if cfg.AddrIndex {
			mp.removeTransactionFromAddrIndex(tx)
		}

		for _, txIn := range txDesc.Tx.MsgTx().TxIn {
			delete(mp.outpoints, txIn.PreviousOutPoint)
		}
		delete(mp.pool, *txHash)
		mp.lastUpdated = time.Now()
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
func (mp *txMemPool) addTransaction(tx *btcutil.Tx, height int32, fee int64) {
	// Add the transaction to the pool and mark the referenced outpoints
	// as spent by the pool.
	mp.pool[*tx.Sha()] = &TxDesc{
		Tx:     tx,
		Added:  time.Now(),
		Height: height,
		Fee:    fee,
	}
	for _, txIn := range tx.MsgTx().TxIn {
		mp.outpoints[txIn.PreviousOutPoint] = tx
	}
	mp.lastUpdated = time.Now()

	if cfg.AddrIndex {
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

// StartingPriority calculates the priority of this tx descriptor's underlying
// transaction relative to when it was first added to the mempool.  The result
// is lazily computed and then cached for subsequent function calls.
func (txD *TxDesc) StartingPriority(txStore blockchain.TxStore) float64 {
	// Return our cached result.
	if txD.startingPriority != float64(0) {
		return txD.startingPriority
	}

	// Compute our starting priority caching the result.
	inputAge := calcInputValueAge(txD, txStore, txD.Height)
	txD.startingPriority = calcPriority(txD.Tx, inputAge)

	return txD.startingPriority
}

// CurrentPriority calculates the current priority of this tx descriptor's
// underlying transaction relative to the next block height.
func (txD *TxDesc) CurrentPriority(txStore blockchain.TxStore, nextBlockHeight int32) float64 {
	inputAge := calcInputValueAge(txD, txStore, nextBlockHeight)
	return calcPriority(txD.Tx, inputAge)
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
	txStore, err := mp.server.blockManager.blockChain.FetchTransactionStore(tx, includeSpent)
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
// previously created ouput to the address.
func (mp *txMemPool) FilterTransactionsByAddress(addr btcutil.Address) ([]*btcutil.Tx, error) {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	if txs, exists := mp.addrindex[addr.EncodeAddress()]; exists {
		addressTxs := make([]*btcutil.Tx, 0, len(txs))
		for txHash := range txs {
			if tx, exists := mp.pool[txHash]; exists {
				addressTxs = append(addressTxs, tx.Tx)
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
	_, curHeight, err := mp.server.db.NewestSha()
	if err != nil {
		// This is an unexpected error so don't turn it into a rule
		// error.
		return nil, err
	}
	nextBlockHeight := curHeight + 1

	// Don't allow non-standard transactions if the network parameters
	// forbid their relaying.
	if !activeNetParams.RelayNonStdTxs {
		err := mp.checkTransactionStandard(tx, nextBlockHeight)
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
	minFee := calcMinRequiredTxRelayFee(serializedSize, cfg.minRelayTxFee)
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
	if isNew && !cfg.NoRelayPriority && txFee < minFee {
		txD := &TxDesc{
			Tx:     tx,
			Added:  time.Now(),
			Height: curHeight,
			Fee:    txFee,
		}
		currentPriority := txD.CurrentPriority(txStore, nextBlockHeight)
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
		if mp.pennyTotal >= cfg.FreeTxRelayLimit*10*1000 {
			str := fmt.Sprintf("transaction %v has been rejected "+
				"by the rate limiter due to low fees", txHash)
			return nil, txRuleError(wire.RejectInsufficientFee, str)
		}
		oldTotal := mp.pennyTotal

		mp.pennyTotal += float64(serializedSize)
		txmpLog.Tracef("rate limit: curTotal %v, nextTotal: %v, "+
			"limit %v", oldTotal, mp.pennyTotal,
			cfg.FreeTxRelayLimit*10*1000)
	}

	// Verify crypto signatures for each input and reject the transaction if
	// any don't verify.
	err = blockchain.ValidateTransactionScripts(tx, txStore,
		txscript.StandardVerifyFlags, mp.server.sigCache)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Add to transaction pool.
	mp.addTransaction(tx, curHeight, txFee)

	txmpLog.Debugf("Accepted transaction %v (pool size: %v)", txHash,
		len(mp.pool))

	if mp.server.rpcServer != nil {
		// Notify websocket clients about mempool transactions.
		mp.server.rpcServer.ntfnMgr.NotifyMempoolTx(tx, isNew)

		// Potentially notify any getblocktemplate long poll clients
		// about stale block templates due to the new transaction.
		mp.server.rpcServer.gbtWorkState.NotifyMempoolTx(mp.lastUpdated)
	}

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

		var enext *list.Element
		for e := orphans.Front(); e != nil; e = enext {
			enext = e.Next()
			tx := e.Value.(*btcutil.Tx)

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

			// Generate and relay the inventory vector for the
			// newly accepted transaction.
			iv := wire.NewInvVect(wire.InvTypeTx, tx.Sha())
			mp.server.RelayInventory(iv, tx)

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
		// Generate the inventory vector and relay it.
		iv := wire.NewInvVect(wire.InvTypeTx, tx.Sha())
		mp.server.RelayInventory(iv, tx)

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
func (mp *txMemPool) TxDescs() []*TxDesc {
	mp.RLock()
	defer mp.RUnlock()

	descs := make([]*TxDesc, len(mp.pool))
	i := 0
	for _, desc := range mp.pool {
		descs[i] = desc
		i++
	}

	return descs
}

// LastUpdated returns the last time a transaction was added to or removed from
// the main pool.  It does not include the orphan pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) LastUpdated() time.Time {
	mp.RLock()
	defer mp.RUnlock()

	return mp.lastUpdated
}

// newTxMemPool returns a new memory pool for validating and storing standalone
// transactions until they are mined into a block.
func newTxMemPool(server *server) *txMemPool {
	memPool := &txMemPool{
		server:        server,
		pool:          make(map[wire.ShaHash]*TxDesc),
		orphans:       make(map[wire.ShaHash]*btcutil.Tx),
		orphansByPrev: make(map[wire.ShaHash]*list.List),
		outpoints:     make(map[wire.OutPoint]*btcutil.Tx),
	}
	if cfg.AddrIndex {
		memPool.addrindex = make(map[string]map[wire.ShaHash]struct{})
	}
	return memPool
}
