// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"math"
	"time"

	"github.com/btcsuite/btcd/wire"
)

const (
	// LockTimeThreshold is the number below which a lock time is
	// interpreted to be a block number.  Since an average of one block
	// is generated per 10 minutes, this allows blocks for about 9,512
	// years.  However, if the field is interpreted as a timestamp, given
	// the lock time is a uint32, the max is sometime around 2106.
	LockTimeThreshold uint32 = 5e8 // Tue Nov 5 00:53:20 1985 UTC
)

// IsFinalizedTransaction determines whether or not a transaction is finalized.
func IsFinalizedTransaction(msgTx *wire.MsgTx, blockHeight int64, blockTime time.Time) bool {
	// Lock time of zero means the transaction is finalized.
	lockTime := msgTx.LockTime
	if lockTime == 0 {
		return true
	}

	// The lock time field of a transaction is either a block height at
	// which the transaction is finalized or a timestamp depending on if the
	// value is before the LockTimeThreshold.  When it is under the
	// threshold it is a block height.
	blockTimeOrHeight := int64(0)
	if lockTime < LockTimeThreshold {
		blockTimeOrHeight = blockHeight
	} else {
		blockTimeOrHeight = blockTime.Unix()
	}
	if int64(lockTime) < blockTimeOrHeight {
		return true
	}

	// At this point, the transaction's lock time hasn't occured yet, but
	// the transaction might still be finalized if the sequence number
	// for all transaction inputs is maxed out.
	for _, txIn := range msgTx.TxIn {
		if txIn.Sequence != math.MaxUint32 {
			return false
		}
	}
	return true
}
