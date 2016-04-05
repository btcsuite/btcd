// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"

	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcutil"
)

const (
	// MaxBlockCost defines the maximum block cost, where "block cost" is
	// interpreted as defined in BIP0141. A block's cost is calculated as
	// the sum of the cost of bytes in the existing transactions and
	// header, plus the cost of each byte in witness data. The cost of a
	// "base" byte is 4, while the cost of a witness byte is 1. As a result,
	// for a block to be valid, the BlockCost MUST be less then, or equal
	// to MaxBlockCost.
	MaxBlockCost = 4000000

	// MaxBlockBaseSize is the maximum number of bytes within a block
	// which can be allocated to non-witness data.
	MaxBlockBaseSize = 1000000

	// MaxBlockSigOpsCost is the maximum number of signature operations
	// allowed for a block. It is calculated via a weighted algorithm which
	// weights segragated witness sig ops lower than regular sig ops.
	MaxBlockSigOpsCost = 80000

	// WitnessScaleFactor determines the level of "discount" witness data
	// recieves compared to "base" data. A scale factor of 4, denotes that
	// witness data is 1/4 as cheap as regular non-witness data.
	WitnessScaleFactor = 4
)

// TODO(roasbeef): fin module contents

// TxVirtualSize...
func GetTxVirtualSize(tx *btcutil.Tx) int64 {
	// vSize := (cost(tx) + 3) / 4
	//       := (((baseSize * 3) + totalSize) + 4) / 4
	return (GetTransactionCost(tx) + (WitnessScaleFactor - 1)) /
		WitnessScaleFactor
}

// GetBlockCost...
func GetCost(blk *btcutil.Block) int64 {
	msgBlock := blk.MsgBlock()

	baseSize := msgBlock.SerializeSize()
	totalSize := msgBlock.SerializeSizeWitness()

	// (baseSize * 3) + totalSize
	return int64((baseSize * (WitnessScaleFactor - 1)) + totalSize)
}

// GetTransactionCost...
func GetTransactionCost(tx *btcutil.Tx) int64 {
	msgTx := tx.MsgTx()

	baseSize := msgTx.SerializeSize()
	totalSize := msgTx.SerializeSizeWitness()

	// (baseSize * 3) + totalSize
	return int64((baseSize * (WitnessScaleFactor - 1)) + totalSize)
}

// GetSigOpCost...
func GetSigOpCost(tx *btcutil.Tx, isCoinBaseTx bool, utxoView *UtxoViewpoint,
	bip16, segWit bool) (int, error) {

	numSigOps := CountSigOps(tx) * WitnessScaleFactor

	if bip16 {
		numP2SHSigOps, err := CountP2SHSigOps(tx, isCoinBaseTx, utxoView)
		if err != nil {
			return 0, nil
		}
		numSigOps += (numP2SHSigOps * WitnessScaleFactor)
	}

	if segWit {
		msgTx := tx.MsgTx()
		for txInIndex, txIn := range msgTx.TxIn {
			// Ensure the referenced input transaction is available.
			originTxHash := &txIn.PreviousOutPoint.Hash
			originTxIndex := txIn.PreviousOutPoint.Index
			txEntry := utxoView.LookupEntry(originTxHash)
			if txEntry == nil || txEntry.IsOutputSpent(originTxIndex) {
				str := fmt.Sprintf("unable to find unspent output "+
					"%v referenced from transaction %s:%d",
					txIn.PreviousOutPoint, tx.Sha(), txInIndex)
				return 0, ruleError(ErrMissingTx, str)
			}

			witness := txIn.Witness
			sigScript := txIn.SignatureScript
			pkScript := txEntry.PkScriptByIndex(originTxIndex)

			numSigOps += txscript.GetWitnessSigOpCount(sigScript, pkScript, witness)
		}

	}

	return numSigOps, nil
}
