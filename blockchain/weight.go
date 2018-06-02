// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"github.com/btcsuite/btcutil"
)

const (
	// DefaultMaxBlockSize is the maximum number of bytes within a block
	DefaultMaxBlockSize = 32000000

	// MaxBlockSigOpsCost is the maximum number of signature operations
	// allowed for a block. It is calculated via a weighted algorithm which
	// weights segregated witness sig ops lower than regular sig ops.
	MaxBlockSigOpsCost = 80000

	// MaxTxSigOpsCount allowed number of signature check operations per transaction. */
	MaxTxSigOpsCount = 20000

	// OneMegaByte is the convenient bytes value representing of 1M
	OneMegaByte = 1000000

	// MaxBlockSigOpsPerMB The maximum allowed number of signature check operations per MB in a
	// block (network rule)
	MaxBlockSigOpsPerMB = 2000
)

// GetTransactionWeight computes the value of the weight metric for a given
// transaction. Currently the weight metric is simply the sum of the
// transactions's serialized size without any witness data scaled
// proportionally by the WitnessScaleFactor, and the transaction's serialized
// size including any witness data.
func GetTransactionWeight(tx *btcutil.Tx) int64 {
	msgTx := tx.MsgTx()

	return int64(msgTx.SerializeSize())
}

func GetMaxBlockSigOpsCount(blocksize int) int {
	mbRoundedUp := 1 + ((blocksize - 1) / OneMegaByte)
	return mbRoundedUp * MaxBlockSigOpsPerMB
}
