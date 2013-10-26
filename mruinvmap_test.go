package main

import (
	"crypto/rand"
	"github.com/conformal/btcwire"
	"testing"
)

// BenchmarkMruInventoryList performs basic benchmarks on the most recently
// used inventory handling.
func BenchmarkMruInventoryList(b *testing.B) {
	// Create a bunch of fake inventory vectors to use in benchmarking
	// the mru inventory code.
	b.StopTimer()
	numInvVects := 100000
	invVects := make([]*btcwire.InvVect, 0, numInvVects)
	for i := 0; i < numInvVects; i++ {
		hashBytes := make([]byte, btcwire.HashSize)
		rand.Read(hashBytes)
		hash, _ := btcwire.NewShaHash(hashBytes)
		iv := btcwire.NewInvVect(btcwire.InvTypeBlock, hash)
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
