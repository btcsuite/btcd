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

// TxVirtualSize computes the virtual size of a given transaction. A
// transaction's virtual is based off it's cost, creating a discount
// for any witness data it contains, proportional to the current
// WitnessScaleFactor value.
func GetTxVirtualSize(tx *btcutil.Tx) int64 {
	// vSize := (cost(tx) + 3) / 4
	//       := (((baseSize * 3) + totalSize) + 3) / 4
	// We add 3 here as a way to compute the ceiling of the prior arithmetic
	// to 4. The division by 4 creates a discount for wit witness data.
	return (GetTransactionCost(tx) + (WitnessScaleFactor - 1)) /
		WitnessScaleFactor
}

// GetBlockCost computes the value of the cost metric for a given block.
// Currently the cost metric is simply the sum of the block's serialized size
// without any witness data scaled proportionally by the WitnessScaleFactor,
// and the block's serialized size including any witness data.
func GetBlockCost(blk *btcutil.Block) int64 {
	msgBlock := blk.MsgBlock()

	baseSize := msgBlock.SerializeSizeStripped()
	totalSize := msgBlock.SerializeSize()

	// (baseSize * 3) + totalSize
	return int64((baseSize * (WitnessScaleFactor - 1)) + totalSize)
}

// GetTransactionCost computes the value of the cost metric for a given
// transaction. Currently the cost metric is simply the sum of the
// transactions's serialized size without any witness data scaled proportionally
// by the WitnessScaleFactor, and the transaction's serialized size including
// any witness data.
func GetTransactionCost(tx *btcutil.Tx) int64 {
	msgTx := tx.MsgTx()

	baseSize := msgTx.SerializeSizeStripped()
	totalSize := msgTx.SerializeSize()

	// (baseSize * 3) + totalSize
	return int64((baseSize * (WitnessScaleFactor - 1)) + totalSize)
}

// GetSigOpCost returns the unified sig op cost for the passed transaction
// respecting current active soft-forks which modified sig op cost counting.
// The unified sig op cost for a transaction is computed as the sum of: the
// legacy sig op count scaled according to the WitnessScaleFactor, the sig op
// count for all p2sh inputs scaled by the WitnessScaleFactor, and finally the
// unscaled sig op count for any inputs spending witness programs.
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

	if segWit && !isCoinBaseTx {
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
