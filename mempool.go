// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"container/list"
	"crypto/rand"
	"fmt"
	"github.com/conformal/btcchain"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcwire"
	"math"
	"math/big"
	"sync"
	"time"
)

// TxRuleError identifies a rule violation.  It is used to indicate that
// processing of a transaction failed due to one of the many validation
// rules.  The caller can use type assertions to determine if a failure was
// specifically due to a rule violation.
type TxRuleError string

// Error satisfies the error interface to print human-readable errors.
func (e TxRuleError) Error() string {
	return string(e)
}

const (
	// mempoolHeight is the height used for the "block" height field of the
	// contextual transaction information provided in a transaction store.
	mempoolHeight = 0x7fffffff

	// maxOrphanTransactions is the maximum number of orphan transactions
	// that can be queued.  At the time this comment was written, this
	// equates to 10,000 transactions, but will increase if the max allowed
	// block payload increases.
	maxOrphanTransactions = btcwire.MaxBlockPayload / 100

	// maxOrphanTxSize is the maximum size allowed for orphan transactions.
	// This helps prevent memory exhaustion attacks from sending a lot of
	// of big orphans.
	maxOrphanTxSize = 5000

	// maxStandardTxSize is the maximum size allowed for transactions that
	// are considered standard and will therefore be relayed and considered
	// for mining.
	maxStandardTxSize = btcwire.MaxBlockPayload / 10

	// maxStandardSigScriptSize is the maximum size allowed for a
	// transaction input signature script to be considered standard.  This
	// value allows for a CHECKMULTISIG pay-to-sript-hash with 3 signatures
	// since each signature is about 80-bytes, the 3 corresponding public
	// keys are 65-bytes each if uncompressed, and the script opcodes take
	// a few extra bytes.  This value also adds a few extra bytes for
	// prosperity.  3*80 + 3*65 + 65 = 500
	maxStandardSigScriptSize = 500

	// maxStandardMultiSigs is the maximum number of signatures
	// allowed in a multi-signature transaction output script for it to be
	// considered standard.
	maxStandardMultiSigs = 3

	// minTxRelayFee is the minimum fee in satoshi that is required for
	// a transaction to be treated as free for relay purposes.  It is also
	// used to help determine if a transation is considered dust.
	minTxRelayFee = 10000
)

// txMemPool is used as a source of transactions that need to be mined into
// blocks and relayed to other peers.  It is safe for concurrent access from
// multiple peers.
type txMemPool struct {
	sync.RWMutex
	server        *server
	pool          map[btcwire.ShaHash]*btcwire.MsgTx
	orphans       map[btcwire.ShaHash]*btcwire.MsgTx
	orphansByPrev map[btcwire.ShaHash]*list.List
	outpoints     map[btcwire.OutPoint]*btcwire.MsgTx
}

// isDust returns whether or not the passed transaction output amount is
// considered dust or not.  Dust is defined in terms of the minimum transaction
// relay fee.  In particular, if the cost to the network to spend coins is more
// than 1/3 of the minimum transaction relay fee, it is considered dust.
func isDust(txOut *btcwire.TxOut) bool {
	// Get the serialized size of the transaction output.
	//
	// TODO(davec): The serialized size should come from btcwire, but it
	// currently doesn't provide a way to do so for transaction outputs, so
	// calculate it here based on the current format.
	// 8 bytes for value + 1 byte for script length + script length
	txOutSize := 9 + len(txOut.PkScript)

	// The total serialized size consists of the output and the associated
	// input script to redeem it.  Since there is no input script
	// to redeem it yet, use the minimum size of a typical input script.
	//
	// Pay-to-pubkey-hash bytes breakdown:
	//
	//  Output to hash (34 bytes):
	//   8 value, 1 script len, 25 script [1 OP_DUP, 1 OP_HASH_160,
	//   1 OP_DATA_20, 20 hash, 1 OP_EQUALVERIFY, 1 OP_CHECKSIG]
	//
	//  Input with compressed pubkey (148 bytes):
	//   36 prev outpoint, 1 script len, 107 script [1 OP_DATA_72, 72 sig,
	//   1 OP_DATA_33, 33 compressed pubkey], 4 sequence
	//
	//  Input with uncompressed pubkey (180 bytes):
	//   36 prev outpoint, 1 script len, 139 script [1 OP_DATA_72, 72 sig,
	//   1 OP_DATA_65, 65 compressed pubkey], 4 sequence
	//
	// Pay-to-pubkey bytes breakdown:
	//
	//  Output to compressed pubkey (44 bytes):
	//   8 value, 1 script len, 35 script [1 OP_DATA_33,
	//   33 compressed pubkey, 1 OP_CHECKSIG]
	//
	//  Output to uncompressed pubkey (76 bytes):
	//   8 value, 1 script len, 67 script [1 OP_DATA_65, 65 pubkey,
	//   1 OP_CHECKSIG]
	//
	//  Input (114 bytes):
	//   36 prev outpoint, 1 script len, 73 script [1 OP_DATA_72,
	//   72 sig], 4 sequence
	//
	// Theoretically this could examine the script type of the output script
	// and use a different size for the typical input script size for
	// pay-to-pubkey vs pay-to-pubkey-hash inputs per the above breakdowns,
	// but the only combinination which is less than the value chosen is
	// a pay-to-pubkey script with a compressed pubkey, which is not very
	// common.
	//
	// The most common scripts are pay-to-pubkey-hash, and as per the above
	// breakdown, the minimum size of a p2pkh input script is 148 bytes.  So
	// that figure is used.
	totalSize := txOutSize + 148

	// The output is considered dust if the cost to the network to spend the
	// coins is more than 1/3 of the minimum transaction relay fee.
	// minTxRelayFee is in Satoshi/KB (kilobyte, not kibibyte), so
	// multiply by 1000 to convert bytes.
	//
	// Using the typical values for a pay-to-pubkey-hash transaction from
	// the breakdown above and the default minimum transaction relay fee of
	// 10000, this equates to values less than 5460 satoshi being considered
	// dust.
	//
	// The following is equivalent to (value/totalSize) * (1/3) * 1000
	// without needing to do floating point math.
	return txOut.Value*1000/(3*int64(totalSize)) < minTxRelayFee
}

// checkPkScriptStandard performs a series of checks on a transaction ouput
// script (public key script) to ensure it is a "standard" public key script.
// A standard public key script is one that is a recognized form, and for
// multi-signature scripts, only contains from 1 to 3 signatures.
func checkPkScriptStandard(pkScript []byte) error {
	scriptClass := btcscript.GetScriptClass(pkScript)
	switch scriptClass {
	case btcscript.MultiSigTy:
		// TODO(davec): Need to get the actual number of signatures.
		numSigs := 1
		if numSigs < 1 {
			str := fmt.Sprintf("multi-signature script with no " +
				"signatures")
			return TxRuleError(str)
		}
		if numSigs > maxStandardMultiSigs {
			str := fmt.Sprintf("multi-signature script with %d "+
				"signatures which is more than the allowed max "+
				"of %d", numSigs, maxStandardMultiSigs)
			return TxRuleError(str)
		}

	case btcscript.NonStandardTy:
		return TxRuleError(fmt.Sprintf("non-standard script form"))
	}

	return nil
}

// checkTransactionStandard performs a series of checks on a transaction to
// ensure it is a "standard" transaction.  A standard transaction is one that
// conforms to several additional limiting cases over what is considered a
// "sane" transaction such as having a version in the supported range, being
// finalized, conforming to more stringent size constraints, having scripts
// of recognized forms, and not containing "dust" outputs (those that are
// so small it costs more to process them than they are worth).
func checkTransactionStandard(tx *btcwire.MsgTx, height int64) error {
	// The transaction must be a currently supported version.
	if tx.Version > btcwire.TxVersion || tx.Version < 1 {
		str := fmt.Sprintf("transaction version %d is not in the "+
			"valid range of %d-%d", tx.Version, 1,
			btcwire.TxVersion)
		return TxRuleError(str)
	}

	// The transaction must be finalized to be standard and therefore
	// considered for inclusion in a block.
	if !btcchain.IsFinalizedTransaction(tx, height, time.Now()) {
		str := fmt.Sprintf("transaction is not finalized")
		return TxRuleError(str)
	}

	// Since extremely large transactions with a lot of inputs can cost
	// almost as much to process as the sender fees, limit the maximum
	// size of a transaction.  This also helps mitigate CPU exhaustion
	// attacks.
	var serializedTxBuf bytes.Buffer
	err := tx.Serialize(&serializedTxBuf)
	if err != nil {
		return err
	}
	serializedLen := serializedTxBuf.Len()
	if serializedLen > maxStandardTxSize {
		str := fmt.Sprintf("transaction size of %v is larger than max "+
			"allowed size of %v", serializedLen, maxStandardTxSize)
		return TxRuleError(str)
	}

	for i, txIn := range tx.TxIn {
		// Each transaction input signature script must not exceed the
		// maximum size allowed for a standard transaction.  See
		// the comment on maxStandardSigScriptSize for more details.
		sigScriptLen := len(txIn.SignatureScript)
		if sigScriptLen > maxStandardSigScriptSize {
			str := fmt.Sprintf("transaction input %d: signature "+
				"script size of %d bytes is large than max "+
				"allowed size of %d bytes", i, sigScriptLen,
				maxStandardSigScriptSize)
			return TxRuleError(str)
		}

		// Each transaction input signature script must only contain
		// opcodes which push data onto the stack.
		if !btcscript.IsPushOnlyScript(txIn.SignatureScript) {
			str := fmt.Sprintf("transaction input %d: signature "+
				"script is not push only", i)
			return TxRuleError(str)
		}
	}

	// None of the output public key scripts can be a non-standard script or
	// be "dust".
	for i, txOut := range tx.TxOut {
		err := checkPkScriptStandard(txOut.PkScript)
		if err != nil {
			str := fmt.Sprintf("transaction output %d: %v", i, err)
			return TxRuleError(str)
		}

		if isDust(txOut) {
			str := fmt.Sprintf("transaction output %d: payment "+
				"of %d is dust", i, txOut.Value)
			return TxRuleError(str)
		}
	}

	return nil
}

// checkInputsStandard performs a series of checks on a transaction's inputs
// to ensure they are "standard".  A standard transaction input is one that
// that consumes the same number of outputs from the stack as the output script
// pushes.  This help prevent resource exhaustion attacks by "creative" use of
// scripts that are super expensive to process like OP_DUP OP_CHECKSIG OP_DROP
// repeated a large number of times followed by a final OP_TRUE.
func checkInputsStandard(tx *btcwire.MsgTx) error {
	// TODO(davec): Implement
	return nil
}

// removeOrphan removes the passed orphan transaction from the orphan pool and
// previous orphan index.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) removeOrphan(txHash *btcwire.ShaHash) {
	// Nothing to do if passed tx is not an orphan.
	tx, exists := mp.orphans[*txHash]
	if !exists {
		return
	}

	// Remove the reference from the previous orphan index.
	for _, txIn := range tx.TxIn {
		originTxHash := txIn.PreviousOutpoint.Hash
		if orphans, exists := mp.orphansByPrev[originTxHash]; exists {
			for e := orphans.Front(); e != nil; e = e.Next() {
				if e.Value.(*btcwire.MsgTx) == tx {
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

// limitNumOrphans limits the number of orphan transactions by evicting a random
// orphan if adding a new one would cause it to overflow the max allowed.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) limitNumOrphans() error {
	if len(mp.orphans)+1 > maxOrphanTransactions {
		// Generate a cryptographically random hash.
		randHashBytes := make([]byte, btcwire.HashSize)
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
		var foundHash *btcwire.ShaHash
		for txHash := range mp.orphans {
			if foundHash == nil {
				foundHash = &txHash
			}
			txHashNum := btcchain.ShaHashToBig(&txHash)
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
func (mp *txMemPool) addOrphan(tx *btcwire.MsgTx, txHash *btcwire.ShaHash) {
	// Limit the number orphan transactions to prevent memory exhaustion.  A
	// random orphan is evicted to make room if needed.
	mp.limitNumOrphans()

	mp.orphans[*txHash] = tx
	for _, txIn := range tx.TxIn {
		originTxHash := txIn.PreviousOutpoint.Hash
		if mp.orphansByPrev[originTxHash] == nil {
			mp.orphansByPrev[originTxHash] = list.New()
		}
		mp.orphansByPrev[originTxHash].PushBack(tx)
	}

	log.Debugf("TXMP: Stored orphan transaction %v (total: %d)", txHash,
		len(mp.orphans))
}

// maybeAddOrphan potentially adds an orphan to the orphan pool.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) maybeAddOrphan(tx *btcwire.MsgTx, txHash *btcwire.ShaHash) error {
	// Ignore orphan transactions that are too large.  This helps avoid
	// a memory exhaustion attack based on sending a lot of really large
	// orphans.  In the case there is a valid transaction larger than this,
	// it will ultimtely be rebroadcast after the parent transactions
	// have been mined or otherwise received.
	//
	// Note that the number of orphan transactions in the orphan pool is
	// also limited, so this equates to a maximum memory used of
	// maxOrphanTxSize * maxOrphanTransactions (which is 500MB as of the
	// time this comment was written).
	var serializedTxBuf bytes.Buffer
	err := tx.Serialize(&serializedTxBuf)
	if err != nil {
		return err
	}
	serializedLen := serializedTxBuf.Len()
	if serializedLen > maxOrphanTxSize {
		str := fmt.Sprintf("orphan transaction size of %d bytes is "+
			"larger than max allowed size of %d bytes",
			serializedLen, maxOrphanTxSize)
		return TxRuleError(str)
	}

	// Add the orphan if the none of the above disqualified it.
	mp.addOrphan(tx, txHash)

	return nil
}

// isTransactionInPool returns whether or not the passed transaction already
// exists in the main pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) isTransactionInPool(hash *btcwire.ShaHash) bool {
	if _, exists := mp.pool[*hash]; exists {
		return true
	}

	return false
}

// IsTransactionInPool returns whether or not the passed transaction already
// exists in the main pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) IsTransactionInPool(hash *btcwire.ShaHash) bool {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	return mp.isTransactionInPool(hash)
}

// isOrphanInPool returns whether or not the passed transaction already exists
// in the orphan pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) isOrphanInPool(hash *btcwire.ShaHash) bool {
	if _, exists := mp.orphans[*hash]; exists {
		return true
	}

	return false
}

// IsOrphanInPool returns whether or not the passed transaction already exists
// in the orphan pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) IsOrphanInPool(hash *btcwire.ShaHash) bool {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	return mp.isOrphanInPool(hash)
}

// haveTransaction returns whether or not the passed transaction already exists
// in the main pool or in the orphan pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) haveTransaction(hash *btcwire.ShaHash) bool {
	return mp.isTransactionInPool(hash) || mp.isOrphanInPool(hash)
}

// HaveTransaction returns whether or not the passed transaction already exists
// in the main pool or in the orphan pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) HaveTransaction(hash *btcwire.ShaHash) bool {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	return mp.haveTransaction(hash)
}

// removeTransaction removes the passed transaction from the memory pool.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) removeTransaction(tx *btcwire.MsgTx) {
	// Remove any transactions which rely on this one.
	txHash, _ := tx.TxSha()
	for i := uint32(0); i < uint32(len(tx.TxOut)); i++ {
		outpoint := btcwire.NewOutPoint(&txHash, i)
		if txRedeemer, exists := mp.outpoints[*outpoint]; exists {
			mp.removeTransaction(txRedeemer)
		}
	}

	// Remove the transaction and mark the referenced outpoints as unspent
	// by the pool.
	if tx, exists := mp.pool[txHash]; exists {
		for _, txIn := range tx.TxIn {
			delete(mp.outpoints, txIn.PreviousOutpoint)
		}
		delete(mp.pool, txHash)
	}
}

// addTransaction adds the passed transaction to the memory pool.  It should
// not be called directly as it doesn't perform any validation.  This is a
// helper for maybeAcceptTransaction.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) addTransaction(tx *btcwire.MsgTx, txHash *btcwire.ShaHash) {
	// Add the transaction to the pool and mark the referenced outpoints
	// as spent by the pool.
	mp.pool[*txHash] = tx
	for _, txIn := range tx.TxIn {
		mp.outpoints[txIn.PreviousOutpoint] = tx
	}
}

// checkPoolDoubleSpend checks whether or not the passed transaction is
// attempting to spend coins already spent by other transactions in the pool.
// Note it does not check for double spends against transactions already in the
// main chain.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) checkPoolDoubleSpend(tx *btcwire.MsgTx) error {
	for _, txIn := range tx.TxIn {
		if txR, exists := mp.outpoints[txIn.PreviousOutpoint]; exists {
			hash, _ := txR.TxSha()
			str := fmt.Sprintf("transaction %v in the pool "+
				"already spends the same coins", hash)
			return TxRuleError(str)
		}
	}

	return nil
}

// fetchInputTransactions fetches the input transactions referenced by the
// passed transaction.  First, it fetches from the main chain, then it tries to
// fetch any missing inputs from the transaction pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) fetchInputTransactions(tx *btcwire.MsgTx) (btcchain.TxStore, error) {
	txStore, err := mp.server.blockManager.blockChain.FetchTransactionStore(tx)
	if err != nil {
		return nil, err
	}

	// Attempt to populate any missing inputs from the transaction pool.
	for _, txD := range txStore {
		if txD.Err == btcdb.TxShaMissing || txD.Tx == nil {
			if poolTx, exists := mp.pool[*txD.Hash]; exists {
				txD.Tx = poolTx
				txD.BlockHeight = mempoolHeight
				txD.Spent = make([]bool, len(poolTx.TxOut))
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
func (mp *txMemPool) FetchTransaction(txHash *btcwire.ShaHash) (*btcwire.MsgTx, error) {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	if tx, exists := mp.pool[*txHash]; exists {
		return tx, nil
	}

	return nil, fmt.Errorf("transaction is not in the pool")
}

// maybeAcceptTransaction is the main workhorse for handling insertion of new
// free-standing transactions into a memory pool.  It includes functionality
// such as rejecting duplicate transactions, ensuring transactions follow all
// rules, orphan transaction handling, and insertion into the memory pool.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) maybeAcceptTransaction(tx *btcwire.MsgTx, isOrphan *bool) error {
	*isOrphan = false
	txHash, err := tx.TxSha()
	if err != nil {
		return err
	}

	// Don't accept the transaction if it already exists in the pool.  This
	// applies to orphan transactions as well.  This check is intended to
	// be a quick check to weed out duplicates.
	if mp.haveTransaction(&txHash) {
		str := fmt.Sprintf("already have transaction %v", txHash)
		return TxRuleError(str)
	}

	// Perform preliminary sanity checks on the transaction.  This makes
	// use of btcchain which contains the invariant rules for what
	// transactions are allowed into blocks.
	err = btcchain.CheckTransactionSanity(tx)
	if err != nil {
		if _, ok := err.(btcchain.RuleError); ok {
			return TxRuleError(err.Error())
		}
		return err
	}

	// A standalone transaction must not be a coinbase transaction.
	if btcchain.IsCoinBase(tx) {
		str := fmt.Sprintf("transaction %v is an individual coinbase",
			txHash)
		return TxRuleError(str)
	}

	// Don't accept transactions with a lock time after the maximum int32
	// value for now.  This is an artifact of older bitcoind clients which
	// treated this field as an int32 and would treat anything larger
	// incorrectly (as negative).
	if tx.LockTime > math.MaxInt32 {
		str := fmt.Sprintf("transaction %v is has a lock time after "+
			"2038 which is not accepted yet", txHash)
		return TxRuleError(str)
	}

	// Get the current height of the main chain.  A standalone transaction
	// will be mined into the next block at best, so
	_, curHeight, err := mp.server.db.NewestSha()
	if err != nil {
		return err
	}
	nextBlockHeight := curHeight + 1

	// Don't allow non-standard transactions on the main network.
	if activeNetParams.btcnet == btcwire.MainNet {
		err := checkTransactionStandard(tx, nextBlockHeight)
		if err != nil {
			str := fmt.Sprintf("transaction %v is not a standard "+
				"transaction: %v", txHash, err)
			return TxRuleError(str)
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
		return err
	}

	// Fetch all of the transactions referenced by the inputs to this
	// transaction.  This function also attempts to fetch the transaction
	// itself to be used for detecting a duplicate transaction without
	// needing to do a separate lookup.
	txStore, err := mp.fetchInputTransactions(tx)
	if err != nil {
		return err
	}

	// Don't allow the transaction if it exists in the main chain and is not
	// not already fully spent.
	if txD, exists := txStore[txHash]; exists && txD.Err == nil {
		for _, isOutputSpent := range txD.Spent {
			if !isOutputSpent {
				str := fmt.Sprintf("transaction already exists")
				return TxRuleError(str)
			}
		}
	}
	delete(txStore, txHash)

	// Transaction is an orphan if any of the inputs don't exist.
	for _, txD := range txStore {
		if txD.Err == btcdb.TxShaMissing {
			*isOrphan = true
			return nil
		}
	}

	// Perform several checks on the transaction inputs using the invariant
	// rules in btcchain for what transactions are allowed into blocks.
	// Also returns the fees associated with the transaction which will be
	// used later.
	txFee, err := btcchain.CheckTransactionInputs(tx, nextBlockHeight, txStore)
	if err != nil {
		return err
	}

	// Don't allow transactions with non-standard inputs on the main
	// network.
	if activeNetParams.btcnet == btcwire.MainNet {
		err := checkInputsStandard(tx)
		if err != nil {
			str := fmt.Sprintf("transaction %v has a non-standard "+
				"input: %v", txHash, err)
			return TxRuleError(str)
		}
	}

	// Note: if you modify this code to accept non-standard transactions,
	// you should add code here to check that the transaction does a
	// reasonable number of ECDSA signature verifications.

	// TODO(davec): Don't allow the transaction if the transation fee
	// would be too low to get into an empty block.
	_ = txFee

	// Verify crypto signatures for each input and reject the transaction if
	// any don't verify.
	err = btcchain.ValidateTransactionScripts(tx, &txHash, time.Now(), txStore)
	if err != nil {
		return err
	}

	// TODO(davec): Rate-limit free transactions

	// Add to transaction pool.
	mp.addTransaction(tx, &txHash)

	log.Debugf("TXMP: Accepted transaction %v (pool size: %v)", txHash,
		len(mp.pool))

	// TODO(davec): Notifications

	// Generate the inventory vector and relay it.
	iv := btcwire.NewInvVect(btcwire.InvTypeTx, &txHash)
	mp.server.RelayInventory(iv)

	return nil
}

// processOrphans determines if there are any orphans which depend on the passed
// transaction hash (they are no longer orphans if true) and potentially accepts
// them.  It repeats the process for the newly accepted transactions (to detect
// further orphans which may no longer be orphans) until there are no more.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) processOrphans(hash *btcwire.ShaHash) error {
	// Start with processing at least the passed hash.
	processHashes := list.New()
	processHashes.PushBack(hash)
	for processHashes.Len() > 0 {
		// Pop the first hash to process.
		firstElement := processHashes.Remove(processHashes.Front())
		processHash := firstElement.(*btcwire.ShaHash)

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
			tx := e.Value.(*btcwire.MsgTx)

			// Remove the orphan from the orphan pool.
			orphanHash, err := tx.TxSha()
			if err != nil {
				return err
			}
			mp.removeOrphan(&orphanHash)

			// Potentially accept the transaction into the
			// transaction pool.
			var isOrphan bool
			err = mp.maybeAcceptTransaction(tx, &isOrphan)
			if err != nil {
				return err
			}

			if isOrphan {
				mp.removeOrphan(&orphanHash)
			}

			// Add this transaction to the list of transactions to
			// process so any orphans that depend on this one are
			// handled too.
			processHashes.PushBack(&orphanHash)
		}
	}

	return nil
}

// ProcessTransaction is the main workhorse for handling insertion of new
// free-standing transactions into a memory pool.  It includes functionality
// such as rejecting duplicate transactions, ensuring transactions follow all
// rules, orphan transaction handling, and insertion into the memory pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) ProcessTransaction(tx *btcwire.MsgTx) error {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	txHash, err := tx.TxSha()
	if err != nil {
		return err
	}
	log.Tracef("TXMP: Processing transaction %v", txHash)

	// Potentially accept the transaction to the memory pool.
	var isOrphan bool
	err = mp.maybeAcceptTransaction(tx, &isOrphan)
	if err != nil {
		return err
	}

	if !isOrphan {
		// Accept any orphan transactions that depend on this
		// transaction (they are no longer orphans) and repeat for those
		// accepted transactions until there are no more.
		err = mp.processOrphans(&txHash)
		if err != nil {
			return err
		}
	} else {
		// When the transaction is an orphan (has inputs missing),
		// potentially add it to the orphan pool.
		err := mp.maybeAddOrphan(tx, &txHash)
		if err != nil {
			return err
		}
	}

	return nil
}

// TxShas returns a slice of hashes for all of the transactions in the memory
// pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) TxShas() []*btcwire.ShaHash {
	mp.RLock()
	defer mp.RUnlock()

	hashes := make([]*btcwire.ShaHash, len(mp.pool))
	i := 0
	for hash := range mp.pool {
		hashCopy := hash
		hashes[i] = &hashCopy
		i++
	}

	return hashes
}

// newTxMemPool returns a new memory pool for validating and storing standalone
// transactions until they are mined into a block.
func newTxMemPool(server *server) *txMemPool {
	return &txMemPool{
		server:        server,
		pool:          make(map[btcwire.ShaHash]*btcwire.MsgTx),
		orphans:       make(map[btcwire.ShaHash]*btcwire.MsgTx),
		orphansByPrev: make(map[btcwire.ShaHash]*list.List),
		outpoints:     make(map[btcwire.OutPoint]*btcwire.MsgTx),
	}
}
