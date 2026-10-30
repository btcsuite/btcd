// Copyright (c) 2023 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
package blockchain

import (
	"os"
	"runtime"
	"testing"
	"unsafe"

	"github.com/btcsuite/btcd/wire/v2"
)

// sampledAlloc uses sampling to perform allocations and return the first
// allocation size that reaches minSamples. We have to use sampling because the
// runtime introduces non determinism in respect with total bytes allocated
// between two points in code.
func sampledAlloc(count int, allocf func(count int) uint64) uint64 {
	const sampleLimit = 100000
	const minSamples = 100

	// We have to use sampling because the runtime introduces non
	// determinism in respect with total bytes allocated between
	// two points in code.
	samples := make(map[uint64]int)
	var lastSample uint64
	var reachedMinSamples bool
	for range sampleLimit {
		lastSample = allocf(count)
		samples[lastSample]++
		if samples[lastSample] >= minSamples {
			reachedMinSamples = true
			break
		}
	}

	if !reachedMinSamples {
		panic("not reached min samples")
	}

	return lastSample
}

// sliceSink is used to avoid compiler optimizations.
var sliceSink [][]byte

// TestRoundupsize asserts that the rounded up size maches the expectations for
// all size classes by comparing against runtime allocations. In case of
// breakages in the allocator behavior, this test should fail.
func TestRoundupsize(t *testing.T) {
	// For some reason the allocation sampling fails when make unit-race is
	// executed. But we don't use concurrency here, so is safe to not run
	// this test with race detection.
	if os.Getenv("GORACE") != "" {
		return
	}

	const n = 8

	// allocf allocates n byte slices of count capacity and returns the
	// total bytes allocated.
	allocF := func(count int) uint64 {
		var ms0, ms1 runtime.MemStats

		s := make([][]byte, n)
		runtime.ReadMemStats(&ms0)
		for i := range n {
			s[i] = make([]byte, count)
		}
		runtime.ReadMemStats(&ms1)
		sliceSink = s
		sliceSink = nil

		return ms1.TotalAlloc - ms0.TotalAlloc
	}

	// Allocations smaller than max size class size should align by the size
	// of the next class.
	for _, i := range class_to_size[1:] {
		want := sampledAlloc(int(i-1), allocF) / n
		got := uint64(roundupsize(uintptr(i - 1)))

		if want != got {
			t.Errorf("i %d; want %d, got %d", i, want, got)
		}
	}

	// Allocations greater than the last class should align by the page
	// size.
	lastClassSize := class_to_size[len(class_to_size)-1]

	want := alignUp(uintptr(lastClassSize+1), _PageSize)
	got := roundupsize(uintptr(lastClassSize + 1))
	real := sampledAlloc(int(lastClassSize+1), allocF) / n

	if want != got {
		t.Errorf("size %d; want %d got %d", lastClassSize+1, want, got)
	}

	if want != uintptr(real) {
		t.Errorf("size %d; want %d real %d", lastClassSize+1, want,
			real)
	}
}

// TestAvgEntrySize asserts that the avgEntrySize is being calculated correctly,
// accounting for the inneficiencies of the current implementation caused by
// allocating each entry with an allocation for the UtxoEntry and another for
// the scriptPubKey slice.
//
// This test remains true while we don't change the utxocache implementation to
// allocate in batch instead of at least two allocations for each added entry.
func TestAvgEntrySize(t *testing.T) {
	utxoEntryAlloc := roundupsize(unsafe.Sizeof(UtxoEntry{}))
	if baseEntrySize != utxoEntryAlloc {
		t.Errorf("bad base entry size; want %d got %d", baseEntrySize,
			utxoEntryAlloc)
	}

	const p2pkhPubScriptLen = 25
	pkhAlloc := roundupsize(p2pkhPubScriptLen)
	if pubKeyHashLen != pkhAlloc {
		t.Errorf("bad pubkeyHashLen; want %d got %d", pubKeyHashLen,
			pkhAlloc)
	}

	entryAlloc := roundupsize(utxoEntryAlloc + pkhAlloc)
	if avgEntrySize != entryAlloc {
		t.Errorf("bad avgEntrySize; want %d got %d", avgEntrySize,
			entryAlloc)
	}
}

// TestAlignUpPow2 asserts that the AlignUpPow2 function is working as expected,
// which is, given input N, we expect the next power of two to be returned or
// the same value if it is already a power of 2.
func TestAlignUpPow2(t *testing.T) {
	tests := []struct {
		name string
		in   uint64
		exp  uint64
	}{
		{"01", 0, 0},
		{"02", 1, 1},
		{"03", 2, 2},
		{"04", 3, 4},
		{"05", 4, 4},
		{"06", 65, 128},
		{"07", 64, 64},
		{"08", 63, 64},
		{"09", 1 << 63, 1 << 63},
		{"10", 1<<63 - 1, 1 << 63},
		{"11", 1<<62 + 1, 1 << 63},
		{"12", ^uint64(0), 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := alignUpPow2(test.in)
			if test.exp != r {
				t.Errorf("test name: %s; want: %d, "+
					"got: %d", test.name, test.exp, r)
			}
		})
	}
}

// mapSink is used to avoid compiler optimizations changing the test results.
var mapSink map[wire.OutPoint]*UtxoEntry

// TestMapSize asserts that given map hint and group size, we calculate the same
// size as the Go allocator allocates. To do this, we compare the calculated
// size against the bytes allocated by the runtime.
//
// This test is important because otherwise we may allocate more or less memory
// than the maximum utxo cache size. This way, if our calculation becomes wrong
// for changes in the Go runtime, we at least will know that there's something
// wrong that needs to be fixed.
func TestMapSize(t *testing.T) {
	// Helper function that allocates a map with capacity to store hint
	// slots and return how many bytes were allocated during the make map
	// call.
	allocF := func(hint int) uint64 {
		var ms0, ms1 runtime.MemStats

		runtime.ReadMemStats(&ms0)
		mapSink = make(map[wire.OutPoint]*UtxoEntry, hint)
		runtime.ReadMemStats(&ms1)
		mapSink = nil

		return ms1.TotalAlloc - ms0.TotalAlloc
	}

	// Make sure that the groupSize constant is correct.
	realGroupSize := unsafe.Sizeof(struct {
		slots [8]struct {
			key   wire.OutPoint
			value *UtxoEntry
		}
		metadata uint64
	}{})
	if groupSize != realGroupSize {
		t.Errorf("unexpected group size; want %d, got %d", groupSize,
			realGroupSize)
	}

	// The test calculate a hint for each base and compares the calculated
	// allocation against the real allocation of a new map using the hint.
	bases := []int{
		0, 1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096,
		8192,
	}
	for _, base := range bases {
		// We are adding one to the maximum load factor on purpose,
		// because this way we don't pass a hint that will be exactly
		// the maximum load factor for a map, which may increase the
		// probability of failure if the map load factor changes.
		hint := (base*mapLoadFactorNum)/mapLoadFactorDen + 1

		want := mapSize(hint, groupSize)
		got := sampledAlloc(hint, allocF)
		if want != got {
			t.Errorf("Failed base %d; allocWant %d, got %d", base,
				want, got)
		}
	}
}

// TestHintForSize asserts that given maxSize, hintForSize returns a hint that
// allocates a map of up to maxSize. Also, hint >> 1 should allocate a map of
// nearly half the previous size.
func TestHintForSize(t *testing.T) {
	tests := []struct {
		name         string
		maxSize      uint64
		wantHint     int
		wantSize     uint64
		wantPrevSize uint64
	}{
		{"01", 0, 8, 48, 48},
		{"02", 1, 8, 48, 48},
		{"03", 2, 8, 48, 48},
		{"04", 4, 8, 48, 48},
		{"05", 8, 8, 48, 48},
		{"06", 1024, 12, 984, 48},
		{"07", 2048, 24, 1880, 984},
		{"08", 1 << 20, 12743, 918192, 459120},
		{"09", 2 << 20, 25486, 1836336, 918192},
		{"10", 256 << 20, 3262236, 235044912, 117522480},
		{"11", 512 << 20, 6524472, 470089776, 235044912},
		{"12", 1 << 30, 13048945, 940179504, 470089776},
	}

	for _, test := range tests {
		hint := hintForSize(test.maxSize, groupSize)
		if test.wantHint != hint {
			t.Errorf("test %s; bad hint; want %d got %d",
				test.name, test.wantHint, hint)
		}

		size := mapSize(hint, groupSize)
		if test.wantSize != size {
			t.Errorf("test %s; bad size; want %d got %d",
				test.name, test.wantSize, size)
		}

		prevSize := mapSize(hint>>1, groupSize)
		if test.wantPrevSize != prevSize {
			t.Errorf("test %s; bad prevSize; want %d got %d",
				test.name, test.wantPrevSize, prevSize)
		}
	}
}
