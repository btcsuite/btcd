// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"fmt"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/mining"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

const (
	// maxStandardP2SHSigOps is the maximum number of signature operations
	// that are considered standard in a pay-to-script-hash script.
	maxStandardP2SHSigOps = 15

	// maxStandardTxCost is the max weight permitted by any transaction
	// according to the current default policy.
	maxStandardTxWeight = 400000

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

	// DefaultMinRelayTxFee is the minimum fee in satoshi that is required
	// for a transaction to be treated as free for relay and mining
	// purposes.  It is also used to help determine if a transaction is
	// considered dust and as a base for calculating minimum required fees
	// for larger transactions.  This value is in Satoshi/1000 bytes.
	DefaultMinRelayTxFee = btcutil.Amount(1000)

	// maxStandardMultiSigKeys is the maximum number of public keys allowed
	// in a multi-signature transaction output script for it to be
	// considered standard.
	maxStandardMultiSigKeys = 3

	// MaxStandardTxVersion is the maximum transaction version that is
	// considered standard and will be relayed and considered for inclusion
	// in a block. This includes v3 transaction support.
	MaxStandardTxVersion = 3
)

// calcMinRequiredTxRelayFee returns the minimum transaction fee required for a
// transaction with the passed serialized size to be accepted into the memory
// pool and relayed.
func calcMinRequiredTxRelayFee(serializedSize int64, minRelayTxFee btcutil.Amount) int64 {
	// Calculate the minimum fee for a transaction to be allowed into the
	// mempool and relayed by scaling the base fee (which is the minimum
	// free transaction relay fee).  minRelayTxFee is in Satoshi/kB so
	// multiply by serializedSize (which is in bytes) and divide by 1000 to
	// get minimum Satoshis.
	minFee := (serializedSize * int64(minRelayTxFee)) / 1000

	if minFee == 0 && minRelayTxFee > 0 {
		minFee = int64(minRelayTxFee)
	}

	// Set the minimum fee to the maximum possible value if the calculated
	// fee is not in the valid range for monetary amounts.
	if minFee < 0 || minFee > btcutil.MaxSatoshi {
		minFee = btcutil.MaxSatoshi
	}

	return minFee
}

// checkInputsStandard performs a series of checks on a transaction's inputs
// to ensure they are "standard".  A standard transaction input within the
// context of this function is one whose referenced public key script is of a
// standard form and, for pay-to-script-hash, does not have more than
// maxStandardP2SHSigOps signature operations.  However, it should also be noted
// that standard inputs also are those which have a clean stack after execution
// and only contain pushed data in their signature scripts.  This function does
// not perform those checks because the script engine already does this more
// accurately and concisely via the txscript.ScriptVerifyCleanStack and
// txscript.ScriptVerifySigPushOnly flags.
func checkInputsStandard(tx *btcutil.Tx, utxoView *blockchain.UtxoViewpoint) error {
	// NOTE: The reference implementation also does a coinbase check here,
	// but coinbases have already been rejected prior to calling this
	// function so no need to recheck.

	for i, txIn := range tx.MsgTx().TxIn {
		// It is safe to elide existence and index checks here since
		// they have already been checked prior to calling this
		// function.
		entry := utxoView.LookupEntry(txIn.PreviousOutPoint)
		originPkScript := entry.PkScript()
		switch txscript.GetScriptClass(originPkScript) {
		case txscript.ScriptHashTy:
			numSigOps := txscript.GetPreciseSigOpCount(
				txIn.SignatureScript, originPkScript, true)
			if numSigOps > maxStandardP2SHSigOps {
				str := fmt.Sprintf("transaction input #%d has "+
					"%d signature operations which is more "+
					"than the allowed max amount of %d",
					i, numSigOps, maxStandardP2SHSigOps)
				return txRuleError(wire.RejectNonstandard, str)
			}

		case txscript.NonStandardTy:
			str := fmt.Sprintf("transaction input #%d has a "+
				"non-standard script form", i)
			return txRuleError(wire.RejectNonstandard, str)
		}
	}

	return nil
}

// checkPkScriptStandard performs a series of checks on a transaction output
// script (public key script) to ensure it is a "standard" public key script.
// A standard public key script is one that is a recognized form, and for
// multi-signature scripts, only contains from 1 to maxStandardMultiSigKeys
// public keys.
func checkPkScriptStandard(pkScript []byte, scriptClass txscript.ScriptClass) error {
	switch scriptClass {
	case txscript.MultiSigTy:
		numPubKeys, numSigs, err := txscript.CalcMultiSigStats(pkScript)
		if err != nil {
			str := fmt.Sprintf("multi-signature script parse "+
				"failure: %v", err)
			return txRuleError(wire.RejectNonstandard, str)
		}

		// A standard multi-signature public key script must contain
		// from 1 to maxStandardMultiSigKeys public keys.
		if numPubKeys < 1 {
			str := "multi-signature script with no pubkeys"
			return txRuleError(wire.RejectNonstandard, str)
		}
		if numPubKeys > maxStandardMultiSigKeys {
			str := fmt.Sprintf("multi-signature script with %d "+
				"public keys which is more than the allowed "+
				"max of %d", numPubKeys, maxStandardMultiSigKeys)
			return txRuleError(wire.RejectNonstandard, str)
		}

		// A standard multi-signature public key script must have at
		// least 1 signature and no more signatures than available
		// public keys.
		if numSigs < 1 {
			return txRuleError(wire.RejectNonstandard,
				"multi-signature script with no signatures")
		}
		if numSigs > numPubKeys {
			str := fmt.Sprintf("multi-signature script with %d "+
				"signatures which is more than the available "+
				"%d public keys", numSigs, numPubKeys)
			return txRuleError(wire.RejectNonstandard, str)
		}

	case txscript.NonStandardTy:
		return txRuleError(wire.RejectNonstandard,
			"non-standard script form")
	}

	return nil
}

// GetDustThreshold calculates the dust limit for a *wire.TxOut by taking the
// size of a typical spending transaction and multiplying it by 3 to account
// for the minimum dust relay fee of 3000sat/kvb.
func GetDustThreshold(txOut *wire.TxOut) int64 {
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
	// Pay-to-witness-pubkey-hash bytes breakdown:
	//
	//  Output to witness key hash (31 bytes);
	//   8 value, 1 script len, 22 script [1 OP_0, 1 OP_DATA_20,
	//   20 bytes hash160]
	//
	//  Input (67 bytes as the 107 witness stack is discounted):
	//   36 prev outpoint, 1 script len, 0 script (not sigScript), 107
	//   witness stack bytes [1 element length, 33 compressed pubkey,
	//   element length 72 sig], 4 sequence
	//
	//
	// Theoretically this could examine the script type of the output script
	// and use a different size for the typical input script size for
	// pay-to-pubkey vs pay-to-pubkey-hash inputs per the above breakdowns,
	// but the only combination which is less than the value chosen is
	// a pay-to-pubkey script with a compressed pubkey, which is not very
	// common.
	//
	// The most common scripts are pay-to-pubkey-hash, and as per the above
	// breakdown, the minimum size of a p2pkh input script is 148 bytes.  So
	// that figure is used. If the output being spent is a witness program,
	// then we apply the witness discount to the size of the signature.
	//
	// The segwit analogue to p2pkh is a p2wkh output. This is the smallest
	// output possible using the new segwit features. The 107 bytes of
	// witness data is discounted by a factor of 4, leading to a computed
	// value of 67 bytes of witness data.
	//
	// Both cases share a 41 byte preamble required to reference the input
	// being spent and the sequence number of the input.
	totalSize := txOut.SerializeSize() + 41
	if txscript.IsWitnessProgram(txOut.PkScript) {
		totalSize += (107 / blockchain.WitnessScaleFactor)
	} else {
		totalSize += 107
	}

	return 3 * int64(totalSize)
}

// IsDust returns whether or not the passed transaction output amount is
// considered dust or not based on the passed minimum transaction relay fee.
// Dust is defined in terms of the minimum transaction relay fee.  In
// particular, if the cost to the network to spend coins is more than 1/3 of the
// minimum transaction relay fee, it is considered dust.
func IsDust(txOut *wire.TxOut, minRelayTxFee btcutil.Amount) bool {
	// Unspendable outputs are considered dust.
	if txscript.IsUnspendable(txOut.PkScript) {
		return true
	}

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
	return txOut.Value*1000/GetDustThreshold(txOut) < int64(minRelayTxFee)
}

// CheckTransactionStandard performs a series of checks on a transaction to
// ensure it is a "standard" transaction.  A standard transaction is one that
// conforms to several additional limiting cases over what is considered a
// "sane" transaction such as having a version in the supported range, being
// finalized, conforming to more stringent size constraints, having scripts
// of recognized forms, and not containing "dust" outputs (those that are
// so small it costs more to process them than they are worth).
func CheckTransactionStandard(tx *btcutil.Tx, height int32,
	medianTimePast time.Time, minRelayTxFee btcutil.Amount,
	maxTxVersion int32) error {

	// The transaction must be a currently supported version.
	msgTx := tx.MsgTx()
	if msgTx.Version > maxTxVersion || msgTx.Version < 1 {
		str := fmt.Sprintf("transaction version %d is not in the "+
			"valid range of %d-%d", msgTx.Version, 1,
			maxTxVersion)
		return txRuleError(wire.RejectNonstandard, str)
	}

	// The transaction must be finalized to be standard and therefore
	// considered for inclusion in a block.
	if !blockchain.IsFinalizedTransaction(tx, height, medianTimePast) {
		return txRuleError(wire.RejectNonstandard,
			"transaction is not finalized")
	}

	// Since extremely large transactions with a lot of inputs can cost
	// almost as much to process as the sender fees, limit the maximum
	// size of a transaction.  This also helps mitigate CPU exhaustion
	// attacks.
	txWeight := blockchain.GetTransactionWeight(tx)
	if txWeight > maxStandardTxWeight {
		str := fmt.Sprintf("weight of transaction is larger than max "+
			"allowed: %v > %v", txWeight, maxStandardTxWeight)
		return txRuleError(wire.RejectNonstandard, str)
	}

	for i, txIn := range msgTx.TxIn {
		// Each transaction input signature script must not exceed the
		// maximum size allowed for a standard transaction.  See
		// the comment on maxStandardSigScriptSize for more details.
		sigScriptLen := len(txIn.SignatureScript)
		if sigScriptLen > maxStandardSigScriptSize {
			str := fmt.Sprintf("transaction input %d: signature "+
				"script size is larger than max allowed: "+
				"%d > %d bytes", i, sigScriptLen,
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
			rejectCode := wire.RejectNonstandard
			if rejCode, found := extractRejectCode(err); found {
				rejectCode = rejCode
			}
			str := fmt.Sprintf("transaction output %d: %v", i, err)
			return txRuleError(rejectCode, str)
		}

		// Accumulate the number of outputs which only carry data.  For
		// all other script types, ensure the output value is not
		// "dust".
		if scriptClass == txscript.NullDataTy {
			numNullDataOutputs++
		} else if IsDust(txOut, minRelayTxFee) {
			str := fmt.Sprintf("transaction output %d: payment is "+
				"dust: %v", i, txOut.Value)
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

// GetTxVirtualSize computes the virtual size of a given transaction. A
// transaction's virtual size is based off its weight, creating a discount for
// any witness data it contains, proportional to the current
// blockchain.WitnessScaleFactor value.
func GetTxVirtualSize(tx *btcutil.Tx) int64 {
	// vSize := (weight(tx) + 3) / 4
	//       := (((baseSize * 3) + totalSize) + 3) / 4
	// We add 3 here as a way to compute the ceiling of the prior arithmetic
	// to 4. The division by 4 creates a discount for wit witness data.
	return (blockchain.GetTransactionWeight(tx) + (blockchain.WitnessScaleFactor - 1)) /
		blockchain.WitnessScaleFactor
}

// CheckTransactionSigCost validates that a transaction's signature operation
// cost does not exceed the specified maximum. This is a standalone function
// that can be called by both TxPool and TxMempoolV2.
func CheckTransactionSigCost(tx *btcutil.Tx, utxoView *blockchain.UtxoViewpoint,
	maxSigOpCostPerTx int) error {

	// Since the coinbase address itself can contain signature operations,
	// the maximum allowed signature operations per transaction is less
	// than the maximum allowed signature operations per block.
	sigOpCost, err := blockchain.GetSigOpCost(
		tx, false, utxoView, true, true,
	)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return chainRuleError(cerr)
		}
		return err
	}

	// Exit early if the sig cost is under limit.
	if sigOpCost <= maxSigOpCostPerTx {
		return nil
	}

	str := fmt.Sprintf("transaction %v sigop cost is too high: %d > %d",
		tx.Hash(), sigOpCost, maxSigOpCostPerTx)

	return txRuleError(wire.RejectNonstandard, str)
}

// CheckRelayFee validates that a transaction meets minimum relay fee
// requirements. This includes checking the absolute fee against the minimum
// relay fee policy, and optionally checking transaction priority for free or
// low-fee transactions.
//
// This is extracted from TxPool.validateRelayFeeMet to be reusable by both
// old TxPool and new TxMempoolV2. Rate limiting is handled separately by the
// callers.
func CheckRelayFee(tx *btcutil.Tx, txFee, txSize int64,
	utxoView *blockchain.UtxoViewpoint, nextBlockHeight int32,
	minRelayTxFee btcutil.Amount, disableRelayPriority bool,
	isNew bool) error {

	txHash := tx.Hash()

	// Most miners allow a free transaction area in blocks they mine to go
	// alongside the area used for high-priority transactions as well as
	// transactions with fees. A transaction size of up to 1000 bytes is
	// considered safe to go into this section. Further, the minimum fee
	// calculated below on its own would encourage several small
	// transactions to avoid fees rather than one single larger transaction
	// which is more desirable. Therefore, as long as the size of the
	// transaction does not exceed 1000 less than the reserved space for
	// high-priority transactions, don't require a fee for it.
	minFee := calcMinRequiredTxRelayFee(txSize, minRelayTxFee)

	if txSize >= (DefaultBlockPrioritySize-1000) && txFee < minFee {
		str := fmt.Sprintf("transaction %v has %d fees which is under "+
			"the required amount of %d", txHash, txFee, minFee)

		return txRuleError(wire.RejectInsufficientFee, str)
	}

	// Exit early if the min relay fee is met.
	if txFee >= minFee {
		return nil
	}

	// Require that free transactions have sufficient priority to be mined
	// in the next block. Transactions which are being added back to the
	// memory pool from blocks that have been disconnected during a reorg
	// are exempted (when isNew is false).
	//
	// Priority calculation requires a UTXO view. If utxoView is nil, we
	// skip the priority check (this can happen in testing scenarios).
	if isNew && !disableRelayPriority && utxoView != nil {
		currentPriority := mining.CalcPriority(
			tx.MsgTx(), utxoView, nextBlockHeight,
		)
		if currentPriority <= mining.MinHighPriority {
			str := fmt.Sprintf("transaction %v has insufficient "+
				"priority (%g <= %g)", txHash,
				currentPriority, mining.MinHighPriority)

			return txRuleError(wire.RejectInsufficientFee, str)
		}
	}

	return nil
}

// CheckSequenceLocks validates that a transaction's sequence locks are active,
// meaning the transaction can be included in the next block with respect to
// its defined relative lock times.
//
// This is extracted from TxPool.checkMempoolAcceptance to be reusable by both
// old TxPool and new TxMempoolV2.
func CheckSequenceLocks(
	tx *btcutil.Tx, utxoView *blockchain.UtxoViewpoint, nextBlockHeight int32,
	medianTimePast time.Time,
	calcSequenceLock func(*btcutil.Tx, *blockchain.UtxoViewpoint) (*blockchain.SequenceLock, error),
) error {

	// Calculate the sequence lock for this transaction.
	sequenceLock, err := calcSequenceLock(tx, utxoView)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return chainRuleError(cerr)
		}
		return err
	}

	// Check if the sequence lock is active (relative timelocks are met).
	if !blockchain.SequenceLockActive(
		sequenceLock, nextBlockHeight, medianTimePast,
	) {
		return txRuleError(wire.RejectNonstandard,
			"transaction's sequence locks on inputs not met")
	}

	return nil
}

// CheckSegWitDeployment validates that if a transaction contains witness data,
// the SegWit soft fork must be active. This prevents accepting witness
// transactions into the mempool before they can be mined.
//
// This is extracted from TxPool.validateSegWitDeployment to be reusable by
// both old TxPool and new TxMempoolV2.
func CheckSegWitDeployment(
	tx *btcutil.Tx, isDeploymentActive func(deploymentID uint32) (bool, error),
	chainParams *chaincfg.Params, bestHeight int32) error {

	// Exit early if this transaction doesn't have witness data.
	if !tx.MsgTx().HasWitness() {
		return nil
	}

	// If a transaction has witness data, and segwit isn't active yet, then
	// we won't accept it into the mempool as it can't be mined yet.
	segwitActive, err := isDeploymentActive(chaincfg.DeploymentSegwit)
	if err != nil {
		return err
	}

	// Exit early if segwit is active.
	if segwitActive {
		return nil
	}

	simnetHint := ""
	if chainParams.Net == wire.SimNet {
		simnetHint = fmt.Sprintf(" (The threshold for segwit "+
			"activation is %v and current best height is %v)",
			chainParams.MinerConfirmationWindow, bestHeight)
	}

	str := fmt.Sprintf("transaction %v has witness data, but segwit "+
		"isn't active yet%s", tx.Hash(), simnetHint)

	return txRuleError(wire.RejectNonstandard, str)
}
