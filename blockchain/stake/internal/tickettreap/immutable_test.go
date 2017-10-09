// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package tickettreap

import (
	"bytes"
	"crypto/sha256"
	"math/rand"
	"reflect"
	"runtime"
	"testing"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
)

// assertPanic tests that code correctly panics, and will raise a testing error
// if the function parameter does not panic as expected.
func assertPanic(t *testing.T, f func()) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	f()
}

// isHeap tests whether the treap meets the min-heap invariant.
func (t *treapNode) isHeap() bool {
	if t == nil {
		return true
	}

	left := t.left == nil || t.left.priority >= t.priority && t.left.isHeap()
	right := t.right == nil || t.right.priority >= t.priority && t.right.isHeap()

	return left && right
}

// TestImmutableEmpty ensures calling functions on an empty immutable treap
// works as expected.
func TestImmutableEmpty(t *testing.T) {
	t.Parallel()

	// Ensure the treap length is the expected value.
	testTreap := NewImmutable()
	if gotLen := testTreap.Len(); gotLen != 0 {
		t.Fatalf("Len: unexpected length - got %d, want %d", gotLen, 0)
	}

	// Ensure the reported size is 0.
	if gotSize := testTreap.Size(); gotSize != 0 {
		t.Fatalf("Size: unexpected byte size - got %d, want 0",
			gotSize)
	}

	// Ensure there are no errors with requesting keys from an empty treap.
	key := uint32ToKey(0)
	if gotVal := testTreap.Has(key); gotVal {
		t.Fatalf("Has: unexpected result - got %v, want false", gotVal)
	}
	if gotVal := testTreap.Get(key); gotVal != nil {
		t.Fatalf("Get: unexpected result - got %v, want nil", gotVal)
	}

	// Ensure there are no panics when deleting keys from an empty treap.
	testTreap.Delete(key)

	// Ensure the number of keys iterated by ForEach on an empty treap is
	// zero.
	var numIterated int
	testTreap.ForEach(func(k Key, v *Value) bool {
		numIterated++
		return true
	})
	if numIterated != 0 {
		t.Fatalf("ForEach: unexpected iterate count - got %d, want 0",
			numIterated)
	}

	// assert panic for GetByIndex
	assertPanic(t, func() {
		testTreap.GetByIndex(-1)
	})

	assertPanic(t, func() {
		testTreap.GetByIndex(0)
	})
}

// TestImmutableSequential ensures that putting keys into an immutable treap in
// sequential order works as expected.
func TestImmutableSequential(t *testing.T) {
	t.Parallel()

	// Insert a bunch of sequential keys while checking several of the treap
	// functions work as expected.
	expectedSize := uint64(0)
	numItems := 1000
	testTreap := NewImmutable()
	for i := 0; i < numItems; i++ {
		key := uint32ToKey(uint32(i))
		value := &Value{Height: uint32(i)}
		testTreap = testTreap.Put(key, value)

		// Ensure the treap length is the expected value.
		if gotLen := testTreap.Len(); gotLen != i+1 {
			t.Fatalf("Len #%d: unexpected length - got %d, want %d",
				i, gotLen, i+1)
		}

		// Ensure the treap has the key.
		if !testTreap.Has(key) {
			t.Fatalf("Has #%d: key %q is not in treap", i, key)
		}

		// Get the key from the treap and ensure it is the expected
		// value.
		if gotVal := testTreap.Get(key); !reflect.DeepEqual(gotVal, value) {
			t.Fatalf("Get #%d: unexpected value - got %v, want %v",
				i, gotVal, value)
		}

		// Ensure the expected size is reported.
		expectedSize += (nodeFieldsSize + uint64(len(key)) + nodeValueSize)
		if gotSize := testTreap.Size(); gotSize != expectedSize {
			t.Fatalf("Size #%d: unexpected byte size - got %d, "+
				"want %d", i, gotSize, expectedSize)
		}

		// test GetByIndex
		k, v := testTreap.GetByIndex(i)
		if key != k {
			t.Fatalf("Get #%d: unexpected key - got %v, want %v",
				i, k, key)
		}

		if value != v {
			t.Fatalf("Get #%d: unexpected value - got %v, want %v",
				i, v, value)
		}
	}

	// assert panic for GetByIndex out of bounds
	assertPanic(t, func() {
		testTreap.GetByIndex(numItems)
	})

	if !testTreap.root.isHeap() {
		t.Fatalf("Heap invariant violated")
	}

	// Ensure the all keys are iterated by ForEach in order.
	var numIterated int
	testTreap.ForEach(func(k Key, v *Value) bool {
		// Ensure the key is as expected.
		wantKey := uint32ToKey(uint32(numIterated))
		if !bytes.Equal(k[:], wantKey[:]) {
			t.Fatalf("ForEach #%d: unexpected key - got %x, want %x",
				numIterated, k, wantKey)
		}

		// Ensure the value is as expected.
		wantValue := &Value{Height: uint32(numIterated)}
		if !reflect.DeepEqual(v, wantValue) {
			t.Fatalf("ForEach #%d: unexpected value - got %v, want %v",
				numIterated, v, wantValue)
		}

		numIterated++
		return true
	})

	// Ensure all items were iterated.
	if numIterated != numItems {
		t.Fatalf("ForEach: unexpected iterate count - got %d, want %d",
			numIterated, numItems)
	}

	numIterated = 0
	// query top 5% of the tree, check height less than the less-than-height
	// requested
	queryHeight := uint32(50) / 20
	testTreap.ForEachByHeight(queryHeight, func(k Key, v *Value) bool {
		// Ensure the height is as expected.
		if !(v.Height < queryHeight) {
			t.Fatalf("ForEach #%d: unexpected value - got %v, want under %v",
				numIterated, v, queryHeight)
		}

		numIterated++
		return true
	})

	// Ensure all items were iterated.
	if numIterated != int(queryHeight) {
		t.Fatalf("ForEachByHeight: unexpected iterate count - got %d, want %d",
			numIterated, int(queryHeight))
	}

	// Delete the keys one-by-one while checking several of the treap
	// functions work as expected.
	for i := 0; i < numItems; i++ {
		key := uint32ToKey(uint32(i))
		testTreap = testTreap.Delete(key)

		expectedLen := numItems - i - 1
		expectedHeadValue := i + 1

		// Ensure the treap length is the expected value.
		if gotLen := testTreap.Len(); gotLen != expectedLen {
			t.Fatalf("Len #%d: unexpected length - got %d, want %d",
				i, gotLen, numItems-i-1)
		}

		// Ensure the treap no longer has the key.
		if testTreap.Has(key) {
			t.Fatalf("Has #%d: key %q is in treap", i, key)
		}

		// test GetByIndex is correct at the head of the treap.
		if expectedLen > 0 {
			if k, _ := testTreap.GetByIndex(0); k != uint32ToKey(uint32(expectedHeadValue)) {
				t.Fatalf("Get #%d: unexpected key - got %v, want %v",
					i, k, key)
			}
		}

		// test GetByIndex is correct at the mid of the treap.
		if expectedLen > 0 {
			halfIdx := expectedLen / 2
			if k, _ := testTreap.GetByIndex(halfIdx); k != uint32ToKey(uint32(expectedHeadValue+halfIdx)) {
				t.Fatalf("Get #%d: unexpected key - got %v, want %v",
					i, k, key)
			}
		}

		// test GetByIndex is correct at the tail of the treap.
		if expectedLen > 0 {
			if k, _ := testTreap.GetByIndex(expectedLen - 1); k != uint32ToKey(uint32(expectedHeadValue+expectedLen-1)) {
				t.Fatalf("Get #%d: unexpected key - got %v, want %v",
					i, k, key)
			}
		}

		// Get the key that no longer exists from the treap and ensure
		// it is nil.
		if gotVal := testTreap.Get(key); gotVal != nil {
			t.Fatalf("Get #%d: unexpected value - got %v, want nil",
				i, gotVal)
		}

		// Ensure the expected size is reported.
		expectedSize -= (nodeFieldsSize + uint64(len(key)) + nodeValueSize)
		if gotSize := testTreap.Size(); gotSize != expectedSize {
			t.Fatalf("Size #%d: unexpected byte size - got %d, "+
				"want %d", i, gotSize, expectedSize)
		}
	}
}

// TestImmutableReverseSequential ensures that putting keys into an immutable
// treap in reverse sequential order works as expected.
func TestImmutableReverseSequential(t *testing.T) {
	t.Parallel()

	// Insert a bunch of sequential keys while checking several of the treap
	// functions work as expected.
	expectedSize := uint64(0)
	numItems := 1000
	testTreap := NewImmutable()
	for i := 0; i < numItems; i++ {
		key := uint32ToKey(uint32(numItems - i - 1))
		value := &Value{Height: uint32(numItems - i - 1)}
		testTreap = testTreap.Put(key, value)

		// Ensure the treap length is the expected value.
		if gotLen := testTreap.Len(); gotLen != i+1 {
			t.Fatalf("Len #%d: unexpected length - got %d, want %d",
				i, gotLen, i+1)
		}

		// Ensure the treap has the key.
		if !testTreap.Has(key) {
			t.Fatalf("Has #%d: key %q is not in treap", i, key)
		}

		// Get the key from the treap and ensure it is the expected
		// value.
		if gotVal := testTreap.Get(key); !reflect.DeepEqual(gotVal, value) {
			t.Fatalf("Get #%d: unexpected value - got %v, want %v",
				i, gotVal, value)
		}

		// Ensure the expected size is reported.
		expectedSize += (nodeFieldsSize + uint64(len(key)) + nodeValueSize)
		if gotSize := testTreap.Size(); gotSize != expectedSize {
			t.Fatalf("Size #%d: unexpected byte size - got %d, "+
				"want %d", i, gotSize, expectedSize)
		}
	}

	if !testTreap.root.isHeap() {
		t.Fatalf("Heap invariant violated")
	}

	// Ensure the all keys are iterated by ForEach in order.
	var numIterated int
	testTreap.ForEach(func(k Key, v *Value) bool {
		// Ensure the key is as expected.
		wantKey := uint32ToKey(uint32(numIterated))
		if !bytes.Equal(k[:], wantKey[:]) {
			t.Fatalf("ForEach #%d: unexpected key - got %x, want %x",
				numIterated, k, wantKey)
		}

		// Ensure the value is as expected.
		wantValue := &Value{Height: uint32(numIterated)}
		if !reflect.DeepEqual(v, wantValue) {
			t.Fatalf("ForEach #%d: unexpected value - got %v, want %v",
				numIterated, v, wantValue)
		}

		numIterated++
		return true
	})

	// Ensure all items were iterated.
	if numIterated != numItems {
		t.Fatalf("ForEach: unexpected iterate count - got %d, want %d",
			numIterated, numItems)
	}

	// Delete the keys one-by-one while checking several of the treap
	// functions work as expected.
	for i := 0; i < numItems; i++ {
		// Intentionally use the reverse order they were inserted here.
		key := uint32ToKey(uint32(i))
		testTreap = testTreap.Delete(key)

		// Ensure the treap length is the expected value.
		if gotLen := testTreap.Len(); gotLen != numItems-i-1 {
			t.Fatalf("Len #%d: unexpected length - got %d, want %d",
				i, gotLen, numItems-i-1)
		}

		// Ensure the treap no longer has the key.
		if testTreap.Has(key) {
			t.Fatalf("Has #%d: key %q is in treap", i, key)
		}

		// Get the key that no longer exists from the treap and ensure
		// it is nil.
		if gotVal := testTreap.Get(key); gotVal != nil {
			t.Fatalf("Get #%d: unexpected value - got %v, want nil",
				i, gotVal)
		}

		if !testTreap.root.isHeap() {
			t.Fatalf("Heap invariant violated")
		}

		// Ensure the expected size is reported.
		expectedSize -= (nodeFieldsSize + uint64(len(key)) + nodeValueSize)
		if gotSize := testTreap.Size(); gotSize != expectedSize {
			t.Fatalf("Size #%d: unexpected byte size - got %d, "+
				"want %d", i, gotSize, expectedSize)
		}
	}
}

// TestImmutableUnordered ensures that putting keys into an immutable treap in
// no paritcular order works as expected.
func TestImmutableUnordered(t *testing.T) {
	t.Parallel()

	// Insert a bunch of out-of-order keys while checking several of the
	// treap functions work as expected.
	expectedSize := uint64(0)
	numItems := 1000
	testTreap := NewImmutable()
	for i := 0; i < numItems; i++ {
		// Hash the serialized int to generate out-of-order keys.
		key := Key(sha256.Sum256(serializeUint32(uint32(i))))
		value := &Value{Height: uint32(i)}
		testTreap = testTreap.Put(key, value)

		// Ensure the treap length is the expected value.
		if gotLen := testTreap.Len(); gotLen != i+1 {
			t.Fatalf("Len #%d: unexpected length - got %d, want %d",
				i, gotLen, i+1)
		}

		// Ensure the treap has the key.
		if !testTreap.Has(key) {
			t.Fatalf("Has #%d: key %q is not in treap", i, key)
		}

		// Get the key from the treap and ensure it is the expected
		// value.
		if gotVal := testTreap.Get(key); !reflect.DeepEqual(gotVal, value) {
			t.Fatalf("Get #%d: unexpected value - got %v, want %v",
				i, gotVal, value)
		}

		// Ensure the expected size is reported.
		expectedSize += nodeFieldsSize + uint64(len(key)) + nodeValueSize
		if gotSize := testTreap.Size(); gotSize != expectedSize {
			t.Fatalf("Size #%d: unexpected byte size - got %d, "+
				"want %d", i, gotSize, expectedSize)
		}
	}

	// Delete the keys one-by-one while checking several of the treap
	// functions work as expected.
	for i := 0; i < numItems; i++ {
		// Hash the serialized int to generate out-of-order keys.
		key := Key(sha256.Sum256(serializeUint32(uint32(i))))
		testTreap = testTreap.Delete(key)

		// Ensure the treap length is the expected value.
		if gotLen := testTreap.Len(); gotLen != numItems-i-1 {
			t.Fatalf("Len #%d: unexpected length - got %d, want %d",
				i, gotLen, numItems-i-1)
		}

		// Ensure the treap no longer has the key.
		if testTreap.Has(key) {
			t.Fatalf("Has #%d: key %q is in treap", i, key)
		}

		// Get the key that no longer exists from the treap and ensure
		// it is nil.
		if gotVal := testTreap.Get(key); gotVal != nil {
			t.Fatalf("Get #%d: unexpected value - got %v, want nil",
				i, gotVal)
		}

		// Ensure the expected size is reported.
		expectedSize -= (nodeFieldsSize + uint64(len(key)) + nodeValueSize)
		if gotSize := testTreap.Size(); gotSize != expectedSize {
			t.Fatalf("Size #%d: unexpected byte size - got %d, "+
				"want %d", i, gotSize, expectedSize)
		}
	}
}

// TestImmutableDuplicatePut ensures that putting a duplicate key into an
// immutable treap works as expected.
func TestImmutableDuplicatePut(t *testing.T) {
	t.Parallel()

	expectedVal := &Value{Height: 10000}
	expectedSize := uint64(0)
	numItems := 1000
	testTreap := NewImmutable()
	for i := 0; i < numItems; i++ {
		key := uint32ToKey(uint32(i))
		value := &Value{Height: uint32(i)}
		testTreap = testTreap.Put(key, value)
		expectedSize += nodeFieldsSize + uint64(len(key)) + nodeValueSize

		// Put a duplicate key with the the expected final value.
		testTreap = testTreap.Put(key, expectedVal)

		// Ensure the key still exists and is the new value.
		if gotVal := testTreap.Has(key); !gotVal {
			t.Fatalf("Has: unexpected result - got %v, want false",
				gotVal)
		}
		if gotVal := testTreap.Get(key); !reflect.DeepEqual(gotVal, expectedVal) {
			t.Fatalf("Get: unexpected result - got %v, want %v",
				gotVal, expectedVal)
		}

		// Ensure the expected size is reported.
		if gotSize := testTreap.Size(); gotSize != expectedSize {
			t.Fatalf("Size: unexpected byte size - got %d, want %d",
				gotSize, expectedSize)
		}
	}
}

// TestImmutableNilValue ensures that putting a nil value into an immutable
// treap results in a NOOP.
func TestImmutableNilValue(t *testing.T) {
	t.Parallel()

	key := uint32ToKey(0)

	// Put the key with a nil value.
	testTreap := NewImmutable()
	testTreap = testTreap.Put(key, nil)

	// Ensure the key does NOT exist.
	if gotVal := testTreap.Has(key); gotVal {
		t.Fatalf("Has: unexpected result - got %v, want false", gotVal)
	}
	if gotVal := testTreap.Get(key); gotVal != nil {
		t.Fatalf("Get: unexpected result - got %v, want nil", gotVal)
	}
}

// TestImmutableForEachStopIterator ensures that returning false from the ForEach
// callback on an immutable treap stops iteration early.
func TestImmutableForEachStopIterator(t *testing.T) {
	t.Parallel()

	// Insert a few keys.
	numItems := 10
	testTreap := NewImmutable()
	for i := 0; i < numItems; i++ {
		key := uint32ToKey(uint32(i))
		value := &Value{Height: uint32(i)}
		testTreap = testTreap.Put(key, value)
	}

	// Ensure ForEach exits early on false return by caller.
	var numIterated int
	testTreap.ForEach(func(k Key, v *Value) bool {
		numIterated++
		return numIterated != numItems/2
	})
	if numIterated != numItems/2 {
		t.Fatalf("ForEach: unexpected iterate count - got %d, want %d",
			numIterated, numItems/2)
	}
}

// TestImmutableSnapshot ensures that immutable treaps are actually immutable by
// keeping a reference to the previous treap, performing a mutation, and then
// ensuring the referenced treap does not have the mutation applied.
func TestImmutableSnapshot(t *testing.T) {
	t.Parallel()

	// Insert a bunch of sequential keys while checking several of the treap
	// functions work as expected.
	expectedSize := uint64(0)
	numItems := 1000
	testTreap := NewImmutable()
	for i := 0; i < numItems; i++ {
		treapSnap := testTreap

		key := uint32ToKey(uint32(i))
		value := &Value{Height: uint32(i)}
		testTreap = testTreap.Put(key, value)

		// Ensure the length of the treap snapshot is the expected
		// value.
		if gotLen := treapSnap.Len(); gotLen != i {
			t.Fatalf("Len #%d: unexpected length - got %d, want %d",
				i, gotLen, i)
		}

		// Ensure the treap snapshot does not have the key.
		if treapSnap.Has(key) {
			t.Fatalf("Has #%d: key %q is in treap", i, key)
		}

		// Get the key that doesn't exist in the treap snapshot and
		// ensure it is nil.
		if gotVal := treapSnap.Get(key); gotVal != nil {
			t.Fatalf("Get #%d: unexpected value - got %v, want nil",
				i, gotVal)
		}

		// Ensure the expected size is reported.
		if gotSize := treapSnap.Size(); gotSize != expectedSize {
			t.Fatalf("Size #%d: unexpected byte size - got %d, "+
				"want %d", i, gotSize, expectedSize)
		}
		expectedSize += (nodeFieldsSize + uint64(len(key)) + nodeValueSize)
	}

	// Delete the keys one-by-one while checking several of the treap
	// functions work as expected.
	for i := 0; i < numItems; i++ {
		treapSnap := testTreap

		key := uint32ToKey(uint32(i))
		value := &Value{Height: uint32(i)}
		testTreap = testTreap.Delete(key)

		// Ensure the length of the treap snapshot is the expected
		// value.
		if gotLen := treapSnap.Len(); gotLen != numItems-i {
			t.Fatalf("Len #%d: unexpected length - got %d, want %d",
				i, gotLen, numItems-i)
		}

		// Ensure the treap snapshot still has the key.
		if !treapSnap.Has(key) {
			t.Fatalf("Has #%d: key %q is not in treap", i, key)
		}

		// Get the key from the treap snapshot and ensure it is still
		// the expected value.
		if gotVal := treapSnap.Get(key); !reflect.DeepEqual(gotVal, value) {
			t.Fatalf("Get #%d: unexpected value - got %v, want %v",
				i, gotVal, value)
		}

		// Ensure the expected size is reported.
		if gotSize := treapSnap.Size(); gotSize != expectedSize {
			t.Fatalf("Size #%d: unexpected byte size - got %d, "+
				"want %d", i, gotSize, expectedSize)
		}
		expectedSize -= (nodeFieldsSize + uint64(len(key)) + nodeValueSize)
	}
}

// randHash generates a "random" hash using a deterministic source.
func randHash(r rand.Source) *chainhash.Hash {
	hash := new(chainhash.Hash)
	for i := 0; i < chainhash.HashSize/2; i++ {
		random := uint64(r.Int63())
		randByte1 := random % 255
		randByte2 := (random >> 8) % 255
		hash[i] = uint8(randByte1)
		hash[i+1] = uint8(randByte2)
	}

	return hash
}

// pickRandWinners picks tickets per block many random "winners" and returns
// their indexes.
func pickRandWinners(sz int, r rand.Source) []int {
	if sz == 0 {
		panic("bad sz!")
	}

	perBlock := int(chaincfg.MainNetParams.TicketsPerBlock)
	winners := make([]int, perBlock)
	for i := 0; i < perBlock; i++ {
		winners[i] = int(r.Int63() % int64(sz))
	}

	return winners
}

// TestImmutableMemory tests the memory for creating n many nodes cloned and
// modified in the memory analogous to what is actually seen in the Decred
// mainnet, then analyzes the relative memory usage with runtime stats.
func TestImmutableMemory(t *testing.T) {
	// Collect information about memory at the start.
	runtime.GC()
	memStats := new(runtime.MemStats)
	runtime.ReadMemStats(memStats)
	initAlloc := memStats.Alloc
	initTotal := memStats.TotalAlloc

	// Insert a bunch of sequential keys while checking several of the treap
	// functions work as expected.
	randSource := rand.NewSource(12345)
	numItems := 40960
	numNodes := 128
	nodeTreaps := make([]*Immutable, numNodes)
	testTreap := NewImmutable()

	// Populate.
	for i := 0; i < numItems; i++ {
		randomHash := randHash(randSource)
		testTreap = testTreap.Put(Key(*randomHash),
			&Value{uint32(randSource.Int63()), false, true, false, true})
	}
	nodeTreaps[0] = testTreap

	// Start populating the "nodes". Ignore expiring tickets for the
	// sake of testing. For each node, remove 5 "random" tickets and
	// insert 5 "random" tickets.
	maxHeight := uint32(0xFFFFFFFF)
	lastTreap := nodeTreaps[0]
	lastTotal := initTotal
	allocsPerNode := make([]uint64, numNodes)
	for i := 1; i < numNodes; i++ {
		treapCopy := lastTreap
		sz := treapCopy.Len()
		winnerIdxs := pickRandWinners(sz, randSource)
		winners, _ := treapCopy.FetchWinnersAndExpired(winnerIdxs, maxHeight)
		for _, k := range winners {
			treapCopy = treapCopy.Delete(*k)
		}

		perBlock := int(chaincfg.MainNetParams.TicketsPerBlock)
		for i := 0; i < perBlock; i++ {
			randomHash := randHash(randSource)
			treapCopy = treapCopy.Put(Key(*randomHash),
				&Value{uint32(randSource.Int63()), false, true, false, true})
		}

		runtime.ReadMemStats(memStats)
		finalTotal := memStats.TotalAlloc
		allocsPerNode[i] = finalTotal - lastTotal
		lastTotal = finalTotal

		nodeTreaps[i] = treapCopy
		lastTreap = treapCopy
	}

	avgUint64 := func(uis []uint64) uint64 {
		var sum uint64
		for i := range uis {
			sum += uis[i]
		}
		return sum / uint64(len(uis))
	}

	runtime.GC()
	runtime.ReadMemStats(memStats)
	finalAlloc := memStats.Alloc
	t.Logf("Ticket treaps for %v nodes allocated %v many bytes total after GC",
		numNodes, finalAlloc-initAlloc)
	t.Logf("Ticket treaps allocated an average of %v many bytes per node",
		avgUint64(allocsPerNode))

	// Keep all the treaps alive in memory so GC doesn't rm them in
	// the previous step.
	lenTest := nodeTreaps[0].count == nodeTreaps[0].Len()
	if !lenTest {
		t.Errorf("bad len test")
	}
}
