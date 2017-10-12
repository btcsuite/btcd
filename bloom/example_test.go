// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bloom_test

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/decred/dcrd/bloom"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// This example demonstrates how to create a new bloom filter, add a transaction
// hash to it, and check if the filter matches the transaction.
func Example_newFilter() {
	rand.Seed(time.Now().UnixNano())
	tweak := rand.Uint32()

	// Create a new bloom filter intended to hold 10 elements with a 0.01%
	// false positive rate and does not include any automatic update
	// functionality when transactions are matched.
	filter := bloom.NewFilter(10, tweak, 0.0001, wire.BloomUpdateNone)

	// Create a transaction hash and add it to the filter.  This particular
	// trasaction is the first transaction in block 310,000 of the main
	// bitcoin block chain.
	txHashStr := "fd611c56ca0d378cdcd16244b45c2ba9588da3adac367c4ef43e808b280b8a45"
	txHash, err := chainhash.NewHashFromStr(txHashStr)
	if err != nil {
		fmt.Println(err)
		return
	}
	filter.AddHash(txHash)

	// Show that the filter matches.
	matches := filter.Matches(txHash[:])
	fmt.Println("Filter Matches?:", matches)

	// Output:
	// Filter Matches?: true
}
