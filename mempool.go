// Copyright (c) 2013-2014 Conformal Systems LLC.
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

	"github.com/mably/btcchain"
	"github.com/mably/btcdb"
	"github.com/mably/btcscript"
	"github.com/mably/btcutil"
	"github.com/mably/btcwire"
)

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

	// maxSigOpsPerTx is the maximum number of signature operations
	// in a single transaction we will relay or mine.  It is a fraction
	// of the max signature operations for a block.
	maxSigOpsPerTx = btcchain.MaxSigOpsPerBlock / 5

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

	// maxStandardMultiSigKeys is the maximum number of public keys allowed
	// in a multi-signature transaction output script for it to be
	// considered standard.
	maxStandardMultiSigKeys = 3

	// minTxRelayFee is the minimum fee in satoshi that is required for a
	// transaction to be treated as free for relay and mining purposes.  It
	// is also used to help determine if a transaction is considered dust
	// and as a base for calculating minimum required fees for larger
	// transactions.  This value is in Satoshi/1000 bytes.
	minTxRelayFee = 1000
)

// TxDesc is a descriptor containing a transaction in the mempool and the
// metadata we store about it.
type TxDesc struct {
	Tx               *btcutil.Tx // Transaction.
	Added            time.Time   // Time when added to pool.
	Height           int64       // Blockheight when added to pool.
	Fee              int64       // Transaction fees.
	startingPriority float64     // Priority when added to the pool.
}

// txMemPool is used as a source of transactions that need to be mined into
// blocks and relayed to other peers.  It is safe for concurrent access from
// multiple peers.
type txMemPool struct {
	sync.RWMutex
	server        *server
	pool          map[btcwire.ShaHash]*TxDesc
	orphans       map[btcwire.ShaHash]*btcutil.Tx
	orphansByPrev map[btcwire.ShaHash]*list.List
	outpoints     map[btcwire.OutPoint]*btcutil.Tx
	lastUpdated   time.Time // last time pool was updated
	pennyTotal    float64   // exponentially decaying total for penny spends.
	lastPennyUnix int64     // unix time of last ``penny spend''
}

// isDust returns whether or not the passed transaction output amount is
// considered dust or not.  Dust is defined in terms of the minimum transaction
// relay fee.  In particular, if the cost to the network to spend coins is more
// than 1/3 of the minimum transaction relay fee, it is considered dust.
func isDust(txOut *btcwire.TxOut) bool {
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
	totalSize := txOut.SerializeSize() + 148

	// The output is considered dust if the cost to the network to spend the
	// coins is more than 1/3 of the minimum free transaction relay fee.
	// minFreeTxRelayFee is in Satoshi/KB, so multiply by 1000 to
	// convert to bytes.
	//
	// Using the typical values for a pay-to-pubkey-hash transaction from
	// the breakdown above and the default minimum free transaction relay
	// fee of 1000, this equates to values less than 546 satoshi being
	// considered dust.
	//
	// The following is equivalent to (value/totalSize) * (1/3) * 1000
	// without needing to do floating point math.
	return txOut.Value*1000/(3*int64(totalSize)) < minTxRelayFee
}

// checkPkScriptStandard performs a series of checks on a transaction ouput
// script (public key script) to ensure it is a "standard" public key script.
// A standard public key script is one that is a recognized form, and for
// multi-signature scripts, only contains from 1 to maxStandardMultiSigKeys
// public keys.
func checkPkScriptStandard(pkScript []byte, scriptClass btcscript.ScriptClass) error {
	switch scriptClass {
	case btcscript.MultiSigTy:
		numPubKeys, numSigs, err := btcscript.CalcMultiSigStats(pkScript)
		if err != nil {
			str := fmt.Sprintf("multi-signature script parse "+
				"failure: %v", err)
			return txRuleError(btcwire.RejectNonstandard, str)
		}

		// A standard multi-signature public key script must contain
		// from 1 to maxStandardMultiSigKeys public keys.
		if numPubKeys < 1 {
			str := "multi-signature script with no pubkeys"
			return txRuleError(btcwire.RejectNonstandard, str)
		}
		if numPubKeys > maxStandardMultiSigKeys {
			str := fmt.Sprintf("multi-signature script with %d "+
				"public keys which is more than the allowed "+
				"max of %d", numPubKeys, maxStandardMultiSigKeys)
			return txRuleError(btcwire.RejectNonstandard, str)
		}

		// A standard multi-signature public key script must have at
		// least 1 signature and no more signatures than available
		// public keys.
		if numSigs < 1 {
			return txRuleError(btcwire.RejectNonstandard,
				"multi-signature script with no signatures")
		}
		if numSigs > numPubKeys {
			str := fmt.Sprintf("multi-signature script with %d "+
				"signatures which is more than the available "+
				"%d public keys", numSigs, numPubKeys)
			return txRuleError(btcwire.RejectNonstandard, str)
		}

	case btcscript.NonStandardTy:
		return txRuleError(btcwire.RejectNonstandard,
			"non-standard script form")
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
func checkTransactionStandard(tx *btcutil.Tx, height int64) error {
	msgTx := tx.MsgTx()

	// The transaction must be a currently supported version.
	if msgTx.Version > btcwire.TxVersion || msgTx.Version < 1 {
		str := fmt.Sprintf("transaction version %d is not in the "+
			"valid range of %d-%d", msgTx.Version, 1,
			btcwire.TxVersion)
		return txRuleError(btcwire.RejectNonstandard, str)
	}

	// The transaction must be finalized to be standard and therefore
	// considered for inclusion in a block.
	if !btcchain.IsFinalizedTransaction(tx, height, time.Now()) {
		return txRuleError(btcwire.RejectNonstandard,
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
		return txRuleError(btcwire.RejectNonstandard, str)
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
			return txRuleError(btcwire.RejectNonstandard, str)
		}

		// Each transaction input signature script must only contain
		// opcodes which push data onto the stack.
		if !btcscript.IsPushOnlyScript(txIn.SignatureScript) {
			str := fmt.Sprintf("transaction input %d: signature "+
				"script is not push only", i)
			return txRuleError(btcwire.RejectNonstandard, str)
		}

		// Each transaction input signature script must only contain
		// canonical data pushes.  A canonical data push is one where
		// the minimum possible number of bytes is used to represent
		// the data push as possible.
		if !btcscript.HasCanonicalPushes(txIn.SignatureScript) {
			str := fmt.Sprintf("transaction input %d: signature "+
				"script has a non-canonical data push", i)
			return txRuleError(btcwire.RejectNonstandard, str)
		}
	}

	// None of the output public key scripts can be a non-standard script or
	// be "dust".
	numNullDataOutputs := 0
	for i, txOut := range msgTx.TxOut {
		scriptClass := btcscript.GetScriptClass(txOut.PkScript)
		err := checkPkScriptStandard(txOut.PkScript, scriptClass)
		if err != nil {
			// Attempt to extract a reject code from the error so
			// it can be retained.  When not possible, fall back to
			// a non standard error.
			rejectCode, found := extractRejectCode(err)
			if !found {
				rejectCode = btcwire.RejectNonstandard
			}
			str := fmt.Sprintf("transaction output %d: %v", i, err)
			return txRuleError(rejectCode, str)
		}

		// Accumulate the number of outputs which only carry data.
		if scriptClass == btcscript.NullDataTy {
			numNullDataOutputs++
		}

		if isDust(txOut) {
			str := fmt.Sprintf("transaction output %d: payment "+
				"of %d is dust", i, txOut.Value)
			return txRuleError(btcwire.RejectDust, str)
		}
	}

	// A standard transaction must not have more than one output script that
	// only carries data.
	if numNullDataOutputs > 1 {
		str := "more than one transaction output in a nulldata script"
		return txRuleError(btcwire.RejectNonstandard, str)
	}

	return nil
}

// checkInputsStandard performs a series of checks on a transaction's inputs
// to ensure they are "standard".  A standard transaction input is one that
// that consumes the expected number of elements from the stack and that number
// is the same as the output script pushes.  This help prevent resource
// exhaustion attacks by "creative" use of scripts that are super expensive to
// process like OP_DUP OP_CHECKSIG OP_DROP repeated a large number of times
// followed by a final OP_TRUE.
func checkInputsStandard(tx *btcutil.Tx, txStore btcchain.TxStore) error {
	// NOTE: The reference implementation also does a coinbase check here,
	// but coinbases have already been rejected prior to calling this
	// function so no need to recheck.

	for i, txIn := range tx.MsgTx().TxIn {
		// It is safe to elide existence and index checks here since
		// they have already been checked prior to calling this
		// function.
		prevOut := txIn.PreviousOutPoint
		originTx := txStore[prevOut.Hash].Tx.MsgTx()
		originPkScript := originTx.TxOut[prevOut.Index].PkScript

		// Calculate stats for the script pair.
		scriptInfo, err := btcscript.CalcScriptInfo(txIn.SignatureScript,
			originPkScript, true)
		if err != nil {
			str := fmt.Sprintf("transaction input #%d script parse "+
				"failure: %v", i, err)
			return txRuleError(btcwire.RejectNonstandard, str)
		}

		// A negative value for expected inputs indicates the script is
		// non-standard in some way.
		if scriptInfo.ExpectedInputs < 0 {
			str := fmt.Sprintf("transaction input #%d expects %d "+
				"inputs", i, scriptInfo.ExpectedInputs)
			return txRuleError(btcwire.RejectNonstandard, str)
		}

		// The script pair is non-standard if the number of available
		// inputs does not match the number of expected inputs.
		if scriptInfo.NumInputs != scriptInfo.ExpectedInputs {
			str := fmt.Sprintf("transaction input #%d expects %d "+
				"inputs, but referenced output script provides "+
				"%d", i, scriptInfo.ExpectedInputs,
				scriptInfo.NumInputs)
			return txRuleError(btcwire.RejectNonstandard, str)
		}
	}

	return nil
}

// calcMinRequiredTxRelayFee returns the minimum transaction fee required for a
// transaction with the passed serialized size to be accepted into the memory
// pool and relayed.
func calcMinRequiredTxRelayFee(serializedSize int64) int64 {
	// Calculate the minimum fee for a transaction to be allowed into the
	// mempool and relayed by scaling the base fee (which is the minimum
	// free transaction relay fee).  minTxRelayFee is in Satoshi/KB, so
	// divide the transaction size by 1000 to convert to kilobytes.  Also,
	// integer division is used so fees only increase on full kilobyte
	// boundaries.
	minFee := (1 + serializedSize/1000) * minTxRelayFee

	// Set the minimum fee to the maximum possible value if the calculated
	// fee is not in the valid range for monetary amounts.
	if minFee < 0 || minFee > btcutil.MaxSatoshi {
		minFee = btcutil.MaxSatoshi
	}

	return minFee
}

// removeOrphan is the internal function which implements the public
// RemoveOrphan.  See the comment for RemoveOrphan for more details.
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) removeOrphan(txHash *btcwire.ShaHash) {
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
// This function is safe for concurrent access.
func (mp *txMemPool) RemoveOrphan(txHash *btcwire.ShaHash) {
	mp.Lock()
	mp.removeOrphan(txHash)
	mp.Unlock()
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
	// maxOrphanTxSize * maxOrphanTransactions (which is 500MB as of the
	// time this comment was written).
	serializedLen := tx.MsgTx().SerializeSize()
	if serializedLen > maxOrphanTxSize {
		str := fmt.Sprintf("orphan transaction size of %d bytes is "+
			"larger than max allowed size of %d bytes",
			serializedLen, maxOrphanTxSize)
		return txRuleError(btcwire.RejectNonstandard, str)
	}

	// Add the orphan if the none of the above disqualified it.
	mp.addOrphan(tx)

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

// removeTransaction is the internal function which implements the public
// RemoveTransaction.  See the comment for RemoveTransaction for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) removeTransaction(tx *btcutil.Tx) {
	// Remove any transactions which rely on this one.
	txHash := tx.Sha()
	for i := uint32(0); i < uint32(len(tx.MsgTx().TxOut)); i++ {
		outpoint := btcwire.NewOutPoint(txHash, i)
		if txRedeemer, exists := mp.outpoints[*outpoint]; exists {
			mp.removeTransaction(txRedeemer)
		}
	}

	// Remove the transaction and mark the referenced outpoints as unspent
	// by the pool.
	if txDesc, exists := mp.pool[*txHash]; exists {
		for _, txIn := range txDesc.Tx.MsgTx().TxIn {
			delete(mp.outpoints, txIn.PreviousOutPoint)
		}
		delete(mp.pool, *txHash)
		mp.lastUpdated = time.Now()
	}
}

// RemoveTransaction removes the passed transaction and any transactions which
// depend on it from the memory pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) RemoveTransaction(tx *btcutil.Tx) {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	mp.removeTransaction(tx)
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
				mp.removeTransaction(txRedeemer)
			}
		}
	}
}

// addTransaction adds the passed transaction to the memory pool.  It should
// not be called directly as it doesn't perform any validation.  This is a
// helper for maybeAcceptTransaction.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) addTransaction(tx *btcutil.Tx, height, fee int64) {
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
}

// calcInputValueAge is a helper function used to calculate the input age of
// a transaction.  The input age for a txin is the number of confirmations
// since the referenced txout multiplied by its output value.  The total input
// age is the sum of this value for each txin.  Any inputs to the transaction
// which are currently in the mempool and hence not mined into a block yet,
// contribute no additional input age to the transaction.
func calcInputValueAge(txDesc *TxDesc, txStore btcchain.TxStore, nextBlockHeight int64) float64 {
	var totalInputAge float64
	for _, txIn := range txDesc.Tx.MsgTx().TxIn {
		originHash := &txIn.PreviousOutPoint.Hash
		originIndex := txIn.PreviousOutPoint.Index

		// Don't attempt to accumulate the total input age if the txIn
		// in question doesn't exist.
		if txData, exists := txStore[*originHash]; exists && txData.Tx != nil {
			// Inputs with dependencies currently in the mempool
			// have their block height set to a special constant.
			// Their input age should computed as zero since their
			// parent hasn't made it into a block yet.
			var inputAge int64
			if txData.BlockHeight == mempoolHeight {
				inputAge = 0
			} else {
				inputAge = nextBlockHeight - txData.BlockHeight
			}

			// Sum the input value times age.
			originTxOut := txData.Tx.MsgTx().TxOut[originIndex]
			inputValue := originTxOut.Value
			totalInputAge += float64(inputValue * inputAge)
		}
	}

	return totalInputAge
}

// StartingPriority calculates the priority of this tx descriptor's underlying
// transaction relative to when it was first added to the mempool.  The result
// is lazily computed and then cached for subsequent function calls.
func (txD *TxDesc) StartingPriority(txStore btcchain.TxStore) float64 {
	// Return our cached result.
	if txD.startingPriority != float64(0) {
		return txD.startingPriority
	}

	// Compute our starting priority caching the result.
	inputAge := calcInputValueAge(txD, txStore, txD.Height)
	txSize := txD.Tx.MsgTx().SerializeSize()
	txD.startingPriority = calcPriority(txD.Tx, txSize, inputAge)

	return txD.startingPriority
}

// CurrentPriority calculates the current priority of this tx descriptor's
// underlying transaction relative to the next block height.
func (txD *TxDesc) CurrentPriority(txStore btcchain.TxStore, nextBlockHeight int64) float64 {
	inputAge := calcInputValueAge(txD, txStore, nextBlockHeight)
	txSize := txD.Tx.MsgTx().SerializeSize()
	return calcPriority(txD.Tx, txSize, inputAge)
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
			str := fmt.Sprintf("transaction %v in the pool "+
				"already spends the same coins", txR.Sha())
			return txRuleError(btcwire.RejectDuplicate, str)
		}
	}

	return nil
}

// fetchInputTransactions fetches the input transactions referenced by the
// passed transaction.  First, it fetches from the main chain, then it tries to
// fetch any missing inputs from the transaction pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) fetchInputTransactions(tx *btcutil.Tx) (btcchain.TxStore, error) {
	txStore, err := mp.server.blockManager.blockChain.FetchTransactionStore(tx)
	if err != nil {
		return nil, err
	}

	// Attempt to populate any missing inputs from the transaction pool.
	for _, txD := range txStore {
		if txD.Err == btcdb.ErrTxShaMissing || txD.Tx == nil {
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
func (mp *txMemPool) FetchTransaction(txHash *btcwire.ShaHash) (*btcutil.Tx, error) {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	if txDesc, exists := mp.pool[*txHash]; exists {
		return txDesc.Tx, nil
	}

	return nil, fmt.Errorf("transaction is not in the pool")
}

// maybeAcceptTransaction is the internal function which implements the public
// MaybeAcceptTransaction.  See the comment for MaybeAcceptTransaction for
// more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) maybeAcceptTransaction(tx *btcutil.Tx, isOrphan *bool, isNew, rateLimit bool) error {
	if isOrphan != nil {
		*isOrphan = false
	}
	txHash := tx.Sha()

	// Don't accept the transaction if it already exists in the pool.  This
	// applies to orphan transactions as well.  This check is intended to
	// be a quick check to weed out duplicates.
	if mp.haveTransaction(txHash) {
		str := fmt.Sprintf("already have transaction %v", txHash)
		return txRuleError(btcwire.RejectDuplicate, str)
	}

	// Perform preliminary sanity checks on the transaction.  This makes
	// use of btcchain which contains the invariant rules for what
	// transactions are allowed into blocks.
	err := btcchain.CheckTransactionSanity(tx)
	if err != nil {
		if cerr, ok := err.(btcchain.RuleError); ok {
			return chainRuleError(cerr)
		}
		return err
	}

	// A standalone transaction must not be a coinbase transaction.
	if btcchain.IsCoinBase(tx) {
		str := fmt.Sprintf("transaction %v is an individual coinbase",
			txHash)
		return txRuleError(btcwire.RejectInvalid, str)
	}

	// Don't accept transactions with a lock time after the maximum int32
	// value for now.  This is an artifact of older bitcoind clients which
	// treated this field as an int32 and would treat anything larger
	// incorrectly (as negative).
	if tx.MsgTx().LockTime > math.MaxInt32 {
		str := fmt.Sprintf("transaction %v has a lock time after "+
			"2038 which is not accepted yet", txHash)
		return txRuleError(btcwire.RejectNonstandard, str)
	}

	// Get the current height of the main chain.  A standalone transaction
	// will be mined into the next block at best, so it's height is at least
	// one more than the current height.
	_, curHeight, err := mp.server.db.NewestSha()
	if err != nil {
		// This is an unexpected error so don't turn it into a rule
		// error.
		return err
	}
	nextBlockHeight := curHeight + 1

	// Don't allow non-standard transactions if the network parameters
	// forbid their relaying.
	if !activeNetParams.RelayNonStdTxs {
		err := checkTransactionStandard(tx, nextBlockHeight)
		if err != nil {
			// Attempt to extract a reject code from the error so
			// it can be retained.  When not possible, fall back to
			// a non standard error.
			rejectCode, found := extractRejectCode(err)
			if !found {
				rejectCode = btcwire.RejectNonstandard
			}
			str := fmt.Sprintf("transaction %v is not standard: %v",
				txHash, err)
			return txRuleError(rejectCode, str)
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
		if cerr, ok := err.(btcchain.RuleError); ok {
			return chainRuleError(cerr)
		}
		return err
	}

	// Don't allow the transaction if it exists in the main chain and is not
	// not already fully spent.
	if txD, exists := txStore[*txHash]; exists && txD.Err == nil {
		for _, isOutputSpent := range txD.Spent {
			if !isOutputSpent {
				return txRuleError(btcwire.RejectDuplicate,
					"transaction already exists")
			}
		}
	}
	delete(txStore, *txHash)

	// Transaction is an orphan if any of the inputs don't exist.
	for _, txD := range txStore {
		if txD.Err == btcdb.ErrTxShaMissing {
			if isOrphan != nil {
				*isOrphan = true
			}
			return nil
		}
	}

	// Perform several checks on the transaction inputs using the invariant
	// rules in btcchain for what transactions are allowed into blocks.
	// Also returns the fees associated with the transaction which will be
	// used later.
	txFee, err := btcchain.CheckTransactionInputs(tx, nextBlockHeight, txStore,
		mp.server.blockManager.blockChain)
	if err != nil {
		if cerr, ok := err.(btcchain.RuleError); ok {
			return chainRuleError(cerr)
		}
		return err
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
				rejectCode = btcwire.RejectNonstandard
			}
			str := fmt.Sprintf("transaction %v has a non-standard "+
				"input: %v", txHash, err)
			return txRuleError(rejectCode, str)
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
	numSigOps, err := btcchain.CountP2SHSigOps(tx, false, txStore)
	if err != nil {
		if cerr, ok := err.(btcchain.RuleError); ok {
			return chainRuleError(cerr)
		}
		return err
	}
	numSigOps += btcchain.CountSigOps(tx)
	if numSigOps > maxSigOpsPerTx {
		str := fmt.Sprintf("transaction %v has too many sigops: %d > %d",
			txHash, numSigOps, maxSigOpsPerTx)
		return txRuleError(btcwire.RejectNonstandard, str)
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
	minFee := calcMinRequiredTxRelayFee(serializedSize)
	if serializedSize >= (defaultBlockPrioritySize-1000) && txFee < minFee {
		str := fmt.Sprintf("transaction %v has %d fees which is under "+
			"the required amount of %d", txHash, txFee,
			minFee)
		return txRuleError(btcwire.RejectInsufficientFee, str)
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
			return txRuleError(btcwire.RejectInsufficientFee, str)
		}
		oldTotal := mp.pennyTotal

		mp.pennyTotal += float64(serializedSize)
		txmpLog.Tracef("rate limit: curTotal %v, nextTotal: %v, "+
			"limit %v", oldTotal, mp.pennyTotal,
			cfg.FreeTxRelayLimit*10*1000)
	}

	// Verify crypto signatures for each input and reject the transaction if
	// any don't verify.
	err = btcchain.ValidateTransactionScripts(tx, txStore,
		standardScriptVerifyFlags)
	if err != nil {
		if cerr, ok := err.(btcchain.RuleError); ok {
			return chainRuleError(cerr)
		}
		return err
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

	return nil
}

// MaybeAcceptTransaction is the main workhorse for handling insertion of new
// free-standing transactions into a memory pool.  It includes functionality
// such as rejecting duplicate transactions, ensuring transactions follow all
// rules, orphan transaction handling, and insertion into the memory pool.  The
// isOrphan parameter can be nil if the caller does not need to know whether
// or not the transaction is an orphan.
//
// This function is safe for concurrent access.
func (mp *txMemPool) MaybeAcceptTransaction(tx *btcutil.Tx, isOrphan *bool, isNew, rateLimit bool) error {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	return mp.maybeAcceptTransaction(tx, isOrphan, isNew, rateLimit)
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
			tx := e.Value.(*btcutil.Tx)

			// Remove the orphan from the orphan pool.
			orphanHash := tx.Sha()
			mp.removeOrphan(orphanHash)

			// Potentially accept the transaction into the
			// transaction pool.
			var isOrphan bool
			err := mp.maybeAcceptTransaction(tx, &isOrphan, true, true)
			if err != nil {
				return err
			}

			if !isOrphan {
				// Generate the inventory vector and relay it.
				iv := btcwire.NewInvVect(btcwire.InvTypeTx, tx.Sha())
				mp.server.RelayInventory(iv)
			} else {
				mp.removeOrphan(orphanHash)
			}

			// Add this transaction to the list of transactions to
			// process so any orphans that depend on this one are
			// handled too.
			processHashes.PushBack(orphanHash)
		}
	}

	return nil
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
	var isOrphan bool
	err := mp.maybeAcceptTransaction(tx, &isOrphan, true, rateLimit)
	if err != nil {
		return err
	}

	if !isOrphan {
		// Generate the inventory vector and relay it.
		iv := btcwire.NewInvVect(btcwire.InvTypeTx, tx.Sha())
		mp.server.RelayInventory(iv)

		// Accept any orphan transactions that depend on this
		// transaction (they are no longer orphans) and repeat for those
		// accepted transactions until there are no more.
		err := mp.processOrphans(tx.Sha())
		if err != nil {
			return err
		}
	} else {
		// The transaction is an orphan (has inputs missing).  Reject
		// it if the flag to allow orphans is not set.
		if !allowOrphan {
			// NOTE: RejectDuplicate is really not an accurate
			// reject code here, but it matches the reference
			// implementation and there isn't a better choice due
			// to the limited number of reject codes.  Missing
			// inputs is assumed to mean they are already spent
			// which is not really always the case.
			return txRuleError(btcwire.RejectDuplicate,
				"transaction spends unknown inputs")
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
	return &txMemPool{
		server:        server,
		pool:          make(map[btcwire.ShaHash]*TxDesc),
		orphans:       make(map[btcwire.ShaHash]*btcutil.Tx),
		orphansByPrev: make(map[btcwire.ShaHash]*list.List),
		outpoints:     make(map[btcwire.OutPoint]*btcutil.Tx),
	}
}
