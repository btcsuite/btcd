// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// TestLruInventoryCache ensures the LruInventoryCache behaves as expected including
// limiting, eviction of least-recently used entries, specific entry removal,
// and existence tests.
func TestLruInventoryCache(t *testing.T) {
	// Create a bunch of fake inventory vectors to use in testing the lru
	// inventory code.
	numInvVects := 10
	invVects := make([]*wire.InvVect, 0, numInvVects)
	for i := 0; i < numInvVects; i++ {
		hash := &chainhash.Hash{byte(i)}
		iv := wire.NewInvVect(wire.InvTypeBlock, hash)
		invVects = append(invVects, iv)
	}

	tests := []struct {
		name  string
		limit int
	}{
		{name: "limit 0", limit: 0},
		{name: "limit 1", limit: 1},
		{name: "limit 5", limit: 5},
		{name: "limit 7", limit: 7},
		{name: "limit one less than available", limit: numInvVects - 1},
		{name: "limit all available", limit: numInvVects},
	}

testLoop:
	for i, test := range tests {
		// Create a new lru inventory cache limited by the specified test
		// limit and add all of the test inventory vectors.  This will
		// cause evicition since there are more test inventory vectors
		// than the limits.
		lruInvCache := newLruInventoryCache(uint(test.limit))
		for j := 0; j < numInvVects; j++ {
			lruInvCache.Add(invVects[j])
		}

		// Ensure the limited number of most recent entries in the
		// inventory vector list exist.
		for j := numInvVects - test.limit; j < numInvVects; j++ {
			if !lruInvCache.Exists(invVects[j]) {
				t.Errorf("Exists #%d (%s) entry %s does not "+
					"exist", i, test.name, *invVects[j])
				continue testLoop
			}
		}

		// Ensure the entries before the limited number of most recent
		// entries in the inventory vector list do not exist.
		for j := 0; j < numInvVects-test.limit; j++ {
			if lruInvCache.Exists(invVects[j]) {
				t.Errorf("Exists #%d (%s) entry %s exists", i,
					test.name, *invVects[j])
				continue testLoop
			}
		}

		// Readd the entry that should currently be the least-recently
		// used entry so it becomes the most-recently used entry, then
		// force an eviction by adding an entry that doesn't exist and
		// ensure the evicted entry is the new least-recently used
		// entry.
		//
		// This check needs at least 2 entries.
		if test.limit > 1 {
			origLruIndex := numInvVects - test.limit
			lruInvCache.Add(invVects[origLruIndex])

			iv := wire.NewInvVect(wire.InvTypeBlock,
				&chainhash.Hash{0x00, 0x01})
			lruInvCache.Add(iv)

			// Ensure the original lru entry still exists since it
			// was updated and should've have become the lru entry.
			if !lruInvCache.Exists(invVects[origLruIndex]) {
				t.Errorf("LRU #%d (%s) entry %s does not exist",
					i, test.name, *invVects[origLruIndex])
				continue testLoop
			}

			// Ensure the entry that should've become the new lru
			// entry was evicted.
			newLruIndex := origLruIndex + 1
			if lruInvCache.Exists(invVects[newLruIndex]) {
				t.Errorf("LRU #%d (%s) entry %s exists", i,
					test.name, *invVects[newLruIndex])
				continue testLoop
			}
		}

		// Delete all of the entries in the inventory vector list,
		// including those that don't exist in the cache, and ensure they
		// no longer exist.
		for j := 0; j < numInvVects; j++ {
			lruInvCache.Delete(invVects[j])
			if lruInvCache.Exists(invVects[j]) {
				t.Errorf("Delete #%d (%s) entry %s exists", i,
					test.name, *invVects[j])
				continue testLoop
			}
		}
	}
}

// TestLruInventoryCacheStringer tests the stringized output for the
// LruInventoryCache type.
func TestLruInventoryCacheStringer(t *testing.T) {
	// Create a couple of fake inventory vectors to use in testing the lru
	// inventory stringer code.
	hash1 := &chainhash.Hash{0x01}
	hash2 := &chainhash.Hash{0x02}
	iv1 := wire.NewInvVect(wire.InvTypeBlock, hash1)
	iv2 := wire.NewInvVect(wire.InvTypeBlock, hash2)

	// Create new lru inventory cache and add the inventory vectors.
	lruInvCache := newLruInventoryCache(uint(2))
	lruInvCache.Add(iv1)
	lruInvCache.Add(iv2)

	// Ensure the stringer gives the expected result.  Since cache iteration
	// is not ordered, either entry could be first, so account for both
	// cases.
	wantStr1 := fmt.Sprintf("<%d>[%s, %s]", 2, *iv1, *iv2)
	wantStr2 := fmt.Sprintf("<%d>[%s, %s]", 2, *iv2, *iv1)
	gotStr := lruInvCache.String()
	if gotStr != wantStr1 && gotStr != wantStr2 {
		t.Fatalf("unexpected string representation - got %q, want %q "+
			"or %q", gotStr, wantStr1, wantStr2)
	}
}

// BenchmarkLruInventoryList performs basic benchmarks on the most recently
// used inventory handling.
func BenchmarkLruInventoryList(b *testing.B) {
	// Create a bunch of fake inventory vectors to use in benchmarking
	// the lru inventory code.
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
	lruInvCache := newLruInventoryCache(uint(limit))
	for i := 0; i < b.N; i++ {
		lruInvCache.Add(invVects[i%numInvVects])
	}
}
