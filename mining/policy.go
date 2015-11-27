// Copyright (c) 2014-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import "github.com/btcsuite/btcutil"

// Policy houses the policy (configuration parameters) which is used to control
// the generation of block templates.  See the documentation for
// NewBlockTemplate for more details on each of these parameters are used.
type Policy struct {
	// BlockMinSize is the minimum block size in bytes to be used when
	// generating a block template.
	BlockMinSize uint32

	// BlockMaxSize is the maximum block size in bytes to be used when
	// generating a block template.
	BlockMaxSize uint32

	// BlockPrioritySize is the size in bytes for high-priority / low-fee
	// transactions to be used when generating a block template.
	BlockPrioritySize uint32

	// TxMinFreeFee is the minimum fee in Satoshi/1000 bytes that is
	// required for a transaction to be treated as free for mining purposes
	// (block template generation).
	TxMinFreeFee btcutil.Amount
}
