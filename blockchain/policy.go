// Copyright (c) 2018 The bcext developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import "github.com/btcsuite/btcd/wire"

const (
	// DefaultMaxBlockSize is the maximum number of bytes within a block
	DefaultMaxBlockSize = 32000000

	// MaxTransactionSize is the maximum size for transactions we're willing to relay/mine
	MaxTransactionSize = OneMegaByte

	// MaxTxSigOpsCount allowed number of signature check operations per transaction. */
	MaxTxSigOpsCount = 20000

	// OneMegaByte is the convenient bytes value representing of 1M
	OneMegaByte = 1000000

	// MaxBlockSigOpsPerMB The maximum allowed number of signature check operations per MB in a
	// block (network rule)
	MaxBlockSigOpsPerMB = 20000

	// MaxOutputsPerBlock is the maximum number of transaction outputs there
	// can be in a block of max weight size.
	MaxOutputsPerBlock = DefaultMaxBlockSize / wire.MinTxOutPayload
)

func GetMaxBlockSigOpsCount(blocksize int) int {
	mbRoundedUp := 1 + ((blocksize - 1) / OneMegaByte)
	return mbRoundedUp * MaxBlockSigOpsPerMB
}
