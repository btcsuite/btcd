// Copyright (c) 2015 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"fmt"
	"testing"
)

// TestLruNonceCache ensures the lruNonceCache behaves as expected including
// limiting, eviction of least-recently used entries, specific entry removal,
// and existence tests.
func TestLruNonceCache(t *testing.T) {
	// Create a bunch of fake nonces to use in testing the lru nonce code.
	numNonces := 10
	nonces := make([]uint64, 0, numNonces)
	for i := 0; i < numNonces; i++ {
		nonces = append(nonces, uint64(i))
	}

	tests := []struct {
		name  string
		limit int
	}{
		{name: "limit 0", limit: 0},
		{name: "limit 1", limit: 1},
		{name: "limit 5", limit: 5},
		{name: "limit 7", limit: 7},
		{name: "limit one less than available", limit: numNonces - 1},
		{name: "limit all available", limit: numNonces},
	}

testLoop:
	for i, test := range tests {
		// Create a new lru nonce cache limited by the specified test
		// limit and add all of the test nonces.  This will cause
		// evicition since there are more test nonces than the limits.
		lruNonceCache := newLruNonceCache(uint(test.limit))
		for j := 0; j < numNonces; j++ {
			lruNonceCache.Add(nonces[j])
		}

		// Ensure the limited number of most recent entries in the list
		// exist.
		for j := numNonces - test.limit; j < numNonces; j++ {
			if !lruNonceCache.Exists(nonces[j]) {
				t.Errorf("Exists #%d (%s) entry %d does not "+
					"exist", i, test.name, nonces[j])
				continue testLoop
			}
		}

		// Ensure the entries before the limited number of most recent
		// entries in the list do not exist.
		for j := 0; j < numNonces-test.limit; j++ {
			if lruNonceCache.Exists(nonces[j]) {
				t.Errorf("Exists #%d (%s) entry %d exists", i,
					test.name, nonces[j])
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
			origLruIndex := numNonces - test.limit
			lruNonceCache.Add(nonces[origLruIndex])

			lruNonceCache.Add(uint64(numNonces) + 1)

			// Ensure the original lru entry still exists since it
			// was updated and should've have become the lru entry.
			if !lruNonceCache.Exists(nonces[origLruIndex]) {
				t.Errorf("LRU #%d (%s) entry %d does not exist",
					i, test.name, nonces[origLruIndex])
				continue testLoop
			}

			// Ensure the entry that should've become the new lru
			// entry was evicted.
			newLruIndex := origLruIndex + 1
			if lruNonceCache.Exists(nonces[newLruIndex]) {
				t.Errorf("LRU #%d (%s) entry %d exists", i,
					test.name, nonces[newLruIndex])
				continue testLoop
			}
		}

		// Delete all of the entries in the list, including those that
		// don't exist in the cache, and ensure they no longer exist.
		for j := 0; j < numNonces; j++ {
			lruNonceCache.Delete(nonces[j])
			if lruNonceCache.Exists(nonces[j]) {
				t.Errorf("Delete #%d (%s) entry %d exists", i,
					test.name, nonces[j])
				continue testLoop
			}
		}
	}
}

// TestLruNonceCacheStringer tests the stringized output for the lruNonceCache type.
func TestLruNonceCacheStringer(t *testing.T) {
	// Create a couple of fake nonces to use in testing the lru nonce
	// stringer code.
	nonce1 := uint64(10)
	nonce2 := uint64(20)

	// Create new lru nonce cache and add the nonces.
	lruNonceCache := newLruNonceCache(uint(2))
	lruNonceCache.Add(nonce1)
	lruNonceCache.Add(nonce2)

	// Ensure the stringer gives the expected result.  Since cache iteration
	// is not ordered, either entry could be first, so account for both
	// cases.
	wantStr1 := fmt.Sprintf("<%d>[%d, %d]", 2, nonce1, nonce2)
	wantStr2 := fmt.Sprintf("<%d>[%d, %d]", 2, nonce2, nonce1)
	gotStr := lruNonceCache.String()
	if gotStr != wantStr1 && gotStr != wantStr2 {
		t.Fatalf("unexpected string representation - got %q, want %q "+
			"or %q", gotStr, wantStr1, wantStr2)
	}
}

// BenchmarkLruNonceList performs basic benchmarks on the most recently used
// nonce handling.
func BenchmarkLruNonceList(b *testing.B) {
	// Create a bunch of fake nonces to use in benchmarking the lru nonce
	// code.
	b.StopTimer()
	numNonces := 100000
	nonces := make([]uint64, 0, numNonces)
	for i := 0; i < numNonces; i++ {
		nonces = append(nonces, uint64(i))
	}
	b.StartTimer()

	// Benchmark the add plus evicition code.
	limit := 20000
	lruNonceCache := newLruNonceCache(uint(limit))
	for i := 0; i < b.N; i++ {
		lruNonceCache.Add(nonces[i%numNonces])
	}
}
