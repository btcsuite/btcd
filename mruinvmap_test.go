// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"crypto/rand"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// BenchmarkMruInventoryList performs basic benchmarks on the most recently
// used inventory handling.
func BenchmarkMruInventoryList(b *testing.B) {
	// Create a bunch of fake inventory vectors to use in benchmarking
	// the mru inventory code.
	b.StopTimer()
	numInvVects := 100000
	invVects := make([]*wire.InvVect, 0, numInvVects)
	for i := 0; i < numInvVects; i++ {
		hashBytes := make([]byte, chainhash.HashSize)
		rand.Read(hashBytes)
		hash, _ := chainhash.NewHash(hashBytes)
		iv := wire.NewInvVect(wire.InvTypeBlock, hash)
		invVects = append(invVects, iv)
	}
	b.StartTimer()

	// Benchmark the add plus evicition code.
	limit := 20000
	mruInvMap := NewMruInventoryMap(uint(limit))
	for i := 0; i < b.N; i++ {
		mruInvMap.Add(invVects[i%numInvVects])
	}
}
