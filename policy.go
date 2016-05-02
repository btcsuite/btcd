// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

// calcMinRequiredTxRelayFee returns the minimum transaction fee required for a
// transaction with the passed serialized size to be accepted into the memory
// pool and relayed.
func calcMinRequiredTxRelayFee(serializedSize, minRelayTxFee int64) int64 {
	// Calculate the minimum fee for a transaction to be allowed into the
	// mempool and relayed by scaling the base fee (which is the minimum
	// free transaction relay fee).  minTxRelayFee is in Atom/KB, so
	// divide the transaction size by 1000 to convert to kilobytes.  Also,
	// integer division is used so fees only increase on full kilobyte
	// boundaries.

	minFee := (serializedSize * minRelayTxFee) / 1000

	if minFee == 0 && minRelayTxFee > 0 {
		minFee = minRelayTxFee
	}

	// Set the minimum fee to the maximum possible value if the calculated
	// fee is not in the valid range for monetary amounts.
	if minFee < 0 || minFee > dcrutil.MaxAmount {
		minFee = dcrutil.MaxAmount
	}

	return minFee
}

// calcPriority returns a transaction priority given a transaction and the sum
// of each of its input values multiplied by their age (# of confirmations).
// Thus, the final formula for the priority is:
// sum(inputValue * inputAge) / adjustedTxSize
func calcPriority(tx *dcrutil.Tx, inputValueAge float64) float64 {
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

	serializedTxSize := tx.MsgTx().SerializeSize()
	if overhead >= serializedTxSize {
		return 0.0
	}

	return inputValueAge / float64(serializedTxSize-overhead)
}

// calcInputValueAge is a helper function used to calculate the input age of
// a transaction.  The input age for a txin is the number of confirmations
// since the referenced txout multiplied by its output value.  The total input
// age is the sum of this value for each txin.  Any inputs to the transaction
// which are currently in the mempool and hence not mined into a block yet,
// contribute no additional input age to the transaction.
func calcInputValueAge(txDesc *TxDesc, txStore blockchain.TxStore,
	nextBlockHeight int64) float64 {
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

// checkInputsStandard performs a series of checks on a transaction's inputs
// to ensure they are "standard".  A standard transaction input is one that
// that consumes the expected number of elements from the stack and that number
// is the same as the output script pushes.  This help prevent resource
// exhaustion attacks by "creative" use of scripts that are super expensive to
// process like OP_DUP OP_CHECKSIG OP_DROP repeated a large number of times
// followed by a final OP_TRUE.
// Decred TODO: I think this is okay, but we'll see with simnet.
func checkInputsStandard(tx *dcrutil.Tx,
	txType stake.TxType,
	txStore blockchain.TxStore) error {
	// NOTE: The reference implementation also does a coinbase check here,
	// but coinbases have already been rejected prior to calling this
	// function so no need to recheck.

	for i, txIn := range tx.MsgTx().TxIn {
		if i == 0 && txType == stake.TxTypeSSGen {
			continue
		}

		// It is safe to elide existence and index checks here since
		// they have already been checked prior to calling this
		// function.
		prevOut := txIn.PreviousOutPoint
		originTx := txStore[prevOut.Hash].Tx.MsgTx()
		originPkScript := originTx.TxOut[prevOut.Index].PkScript

		// Calculate stats for the script pair.
		scriptInfo, err := txscript.CalcScriptInfo(txIn.SignatureScript,
			originPkScript, true)
		if err != nil {
			str := fmt.Sprintf("transaction input #%d script parse "+
				"failure: %v", i, err)
			return txRuleError(wire.RejectNonstandard, str)
		}

		// A negative value for expected inputs indicates the script is
		// non-standard in some way.
		if scriptInfo.ExpectedInputs < 0 {
			str := fmt.Sprintf("transaction input #%d expects %d "+
				"inputs", i, scriptInfo.ExpectedInputs)
			return txRuleError(wire.RejectNonstandard, str)
		}

		// The script pair is non-standard if the number of available
		// inputs does not match the number of expected inputs.
		if scriptInfo.NumInputs != scriptInfo.ExpectedInputs {
			str := fmt.Sprintf("transaction input #%d expects %d "+
				"inputs, but referenced output script provides "+
				"%d", i, scriptInfo.ExpectedInputs,
				scriptInfo.NumInputs)
			return txRuleError(wire.RejectNonstandard, str)
		}
	}

	return nil
}

// checkPkScriptStandard performs a series of checks on a transaction ouput
// script (public key script) to ensure it is a "standard" public key script.
// A standard public key script is one that is a recognized form, and for
// multi-signature scripts, only contains from 1 to maxStandardMultiSigKeys
// public keys.
func checkPkScriptStandard(version uint16, pkScript []byte,
	scriptClass txscript.ScriptClass) error {
	// Only default Bitcoin-style script is standard except for
	// null data outputs.
	if version != wire.DefaultPkScriptVersion {
		str := fmt.Sprintf("versions other than default pkscript version " +
			"are currently non-standard except for provably unspendable " +
			"outputs")
		return txRuleError(wire.RejectNonstandard, str)
	}

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

// isDust returns whether or not the passed transaction output amount is
// considered dust or not.  Dust is defined in terms of the minimum transaction
// relay fee.  In particular, if the cost to the network to spend coins is more
// than 1/3 of the minimum transaction relay fee, it is considered dust.
func isDust(txOut *wire.TxOut, minTxRelayFee dcrutil.Amount) bool {
	// Unspendable outputs are considered dust.
	if txscript.IsUnspendable(txOut.PkScript) {
		return true
	}

	// The total serialized size consists of the output and the associated
	// input script to redeem it.  Since there is no input script
	// to redeem it yet, use the minimum size of a typical input script.
	//
	// Pay-to-pubkey-hash bytes breakdown:
	//
	//  Output to hash (38 bytes):
	//   2 script version, 8 value, 1 script len, 25 script
	//   [1 OP_DUP, 1 OP_HASH_160, 1 OP_DATA_20, 20 hash,
	//   1 OP_EQUALVERIFY, 1 OP_CHECKSIG]
	//
	//  Input with compressed pubkey (165 bytes):
	//   37 prev outpoint, 16 fraud proof, 1 script len,
	//   107 script [1 OP_DATA_72, 72 sig, 1 OP_DATA_33,
	//   33 compressed pubkey], 4 sequence
	//
	//  Input with uncompressed pubkey (198 bytes):
	//   37 prev outpoint,  16 fraud proof, 1 script len,
	//   139 script [1 OP_DATA_72, 72 sig, 1 OP_DATA_65,
	//   65 compressed pubkey], 4 sequence, 1 witness
	//   append
	//
	// Pay-to-pubkey bytes breakdown:
	//
	//  Output to compressed pubkey (46 bytes):
	//   2 script version, 8 value, 1 script len, 35 script
	//   [1 OP_DATA_33, 33 compressed pubkey, 1 OP_CHECKSIG]
	//
	//  Output to uncompressed pubkey (76 bytes):
	//   2 script version, 8 value, 1 script len, 67 script
	//   [1 OP_DATA_65, 65 pubkey, 1 OP_CHECKSIG]
	//
	//  Input (133 bytes):
	//   37 prev outpoint, 16 fraud proof, 1 script len, 73
	//   script [1 OP_DATA_72, 72 sig], 4 sequence, 1 witness
	//   append
	//
	// Theoretically this could examine the script type of the output script
	// and use a different size for the typical input script size for
	// pay-to-pubkey vs pay-to-pubkey-hash inputs per the above breakdowns,
	// but the only combinination which is less than the value chosen is
	// a pay-to-pubkey script with a compressed pubkey, which is not very
	// common.
	//
	// The most common scripts are pay-to-pubkey-hash, and as per the above
	// breakdown, the minimum size of a p2pkh input script is 165 bytes.  So
	// that figure is used.
	totalSize := txOut.SerializeSize() + 165

	// The output is considered dust if the cost to the network to spend the
	// coins is more than 1/3 of the minimum free transaction relay fee.
	// minFreeTxRelayFee is in Atom/KB, so multiply by 1000 to
	// convert to bytes.
	//
	// Using the typical values for a pay-to-pubkey-hash transaction from
	// the breakdown above and the default minimum free transaction relay
	// fee of 5000000, this equates to values less than 546 atoms being
	// considered dust.
	//
	// The following is equivalent to (value/totalSize) * (1/3) * 1000
	// without needing to do floating point math.

	return txOut.Value*1000/(3*int64(totalSize)) < int64(minTxRelayFee)
}

// minInt is a helper function to return the minimum of two ints.  This avoids
// a math import and the need to cast to floats.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
