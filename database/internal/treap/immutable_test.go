// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package treap

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

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
	key := serializeUint32(0)
	if gotVal := testTreap.Has(key); gotVal {
		t.Fatalf("Has: unexpected result - got %v, want false", gotVal)
	}
	if gotVal := testTreap.Get(key); gotVal != nil {
		t.Fatalf("Get: unexpected result - got %x, want nil", gotVal)
	}

	// Ensure there are no panics when deleting keys from an empty treap.
	testTreap.Delete(key)

	// Ensure the number of keys iterated by ForEach on an empty treap is
	// zero.
	var numIterated int
	testTreap.ForEach(func(k, v []byte) bool {
		numIterated++
		return true
	})
	if numIterated != 0 {
		t.Fatalf("ForEach: unexpected iterate count - got %d, want 0",
			numIterated)
	}
}

// TestImmutableSequential ensures that putting keys into an immutable treap in
// sequential order works as expected.
func TestImmutableSequential(t *testing.T) {
	t.Parallel()

	// Insert a bunch of sequential keys while checking several of the treap
	// functions work as expected.
	expectedSize := uint64(0)
	numItems := 1000
	keyCount := 100
	testTreap := NewImmutable()
	for i := 0; i < numItems/keyCount; i++ {
		keys := make([][]byte, 0, keyCount)
		kvPairs := make([]KVPair, 0, keyCount)
		for j := 0; j < keyCount; j++ {
			n := i*keyCount + j
			key := serializeUint32(uint32(n))
			keys = append(keys, key)
			kvPairs = append(kvPairs, KVPair{key, key})
		}

		testTreap = testTreap.Put(kvPairs...)

		// Ensure the treap length is the expected value.
		if gotLen := testTreap.Len(); gotLen != (i+1)*keyCount {
			t.Fatalf("Len #%d: unexpected length - got %d, want %d",
				i, gotLen, i+1)
		}

		for j, key := range keys {
			// Ensure the treap has the key.
			if !testTreap.Has(key) {
				t.Fatalf("Has #%d#%d: key %q is not in treap", i, j, key)
			}

			// Get the key from the treap and ensure it is the expected
			// value.
			if gotVal := testTreap.Get(key); !bytes.Equal(gotVal, key) {
				t.Fatalf("Get #%d#%d: unexpected value - got %x, want %x",
					i, j, gotVal, key)
			}

			expectedSize += (nodeFieldsSize + 8)
		}

		// Ensure the expected size is reported.
		if gotSize := testTreap.Size(); gotSize != expectedSize {
			t.Fatalf("Size #%d: unexpected byte size - got %d, "+
				"want %d", i, gotSize, expectedSize)
		}
	}

	// Ensure the all keys are iterated by ForEach in order.
	var numIterated int
	testTreap.ForEach(func(k, v []byte) bool {
		wantKey := serializeUint32(uint32(numIterated))

		// Ensure the key is as expected.
		if !bytes.Equal(k, wantKey) {
			t.Fatalf("ForEach #%d: unexpected key - got %x, want %x",
				numIterated, k, wantKey)
		}

		// Ensure the value is as expected.
		if !bytes.Equal(v, wantKey) {
			t.Fatalf("ForEach #%d: unexpected value - got %x, want %x",
				numIterated, v, wantKey)
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
		key := serializeUint32(uint32(i))
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
			t.Fatalf("Get #%d: unexpected value - got %x, want nil",
				i, gotVal)
		}

		// Ensure the expected size is reported.
		expectedSize -= (nodeFieldsSize + 8)
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
	keyCount := 100
	testTreap := NewImmutable()
	for i := 0; i < numItems/keyCount; i++ {
		keys := make([][]byte, 0, keyCount)
		kvPairs := make([]KVPair, 0, keyCount)
		for j := 0; j < keyCount; j++ {
			n := numItems - (i * keyCount) - j - 1
			key := serializeUint32(uint32(n))
			keys = append(keys, key)
			kvPairs = append(kvPairs, KVPair{key, key})
		}

		testTreap = testTreap.Put(kvPairs...)

		// Ensure the treap length is the expected value.
		if gotLen := testTreap.Len(); gotLen != (i+1)*keyCount {
			t.Fatalf("Len #%d: unexpected length - got %d, want %d",
				i, gotLen, i+1)
		}

		for j, key := range keys {
			// Ensure the treap has the key.
			if !testTreap.Has(key) {
				t.Fatalf("Has #%d#%d: key %q is not in treap", i, j, key)
			}

			// Get the key from the treap and ensure it is the expected
			// value.
			if gotVal := testTreap.Get(key); !bytes.Equal(gotVal, key) {
				t.Fatalf("Get #%d#%d: unexpected value - got %x, want %x",
					i, j, gotVal, key)
			}

			expectedSize += (nodeFieldsSize + 8)
		}

		// Ensure the expected size is reported.
		if gotSize := testTreap.Size(); gotSize != expectedSize {
			t.Fatalf("Size #%d: unexpected byte size - got %d, "+
				"want %d", i, gotSize, expectedSize)
		}
	}

	// Ensure the all keys are iterated by ForEach in order.
	var numIterated int
	testTreap.ForEach(func(k, v []byte) bool {
		wantKey := serializeUint32(uint32(numIterated))

		// Ensure the key is as expected.
		if !bytes.Equal(k, wantKey) {
			t.Fatalf("ForEach #%d: unexpected key - got %x, want %x",
				numIterated, k, wantKey)
		}

		// Ensure the value is as expected.
		if !bytes.Equal(v, wantKey) {
			t.Fatalf("ForEach #%d: unexpected value - got %x, want %x",
				numIterated, v, wantKey)
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
		key := serializeUint32(uint32(i))
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
			t.Fatalf("Get #%d: unexpected value - got %x, want nil",
				i, gotVal)
		}

		// Ensure the expected size is reported.
		expectedSize -= (nodeFieldsSize + 8)
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
	keyCount := 100
	testTreap := NewImmutable()
	for i := 0; i < numItems/keyCount; i++ {
		// Hash the serialized int to generate out-of-order keys.
		keys := make([][]byte, 0, keyCount)
		kvPairs := make([]KVPair, 0, keyCount)
		for j := 0; j < keyCount; j++ {
			n := i*keyCount + j
			hash := sha256.Sum256(serializeUint32(uint32(n)))
			key := hash[:]
			keys = append(keys, key)
			kvPairs = append(kvPairs, KVPair{key, key})
		}

		testTreap = testTreap.Put(kvPairs...)

		// Ensure the treap length is the expected value.
		if gotLen := testTreap.Len(); gotLen != (i+1)*keyCount {
			t.Fatalf("Len #%d: unexpected length - got %d, want %d",
				i, gotLen, i+1)
		}

		for j, key := range keys {
			// Ensure the treap has the key.
			if !testTreap.Has(key) {
				t.Fatalf("Has #%d#%d: key %q is not in treap", i, j, key)
			}

			// Get the key from the treap and ensure it is the expected
			// value.
			if gotVal := testTreap.Get(key); !bytes.Equal(gotVal, key) {
				t.Fatalf("Get #%d#%d: unexpected value - got %x, want %x",
					i, j, gotVal, key)
			}

			expectedSize += nodeFieldsSize + uint64(len(key)+len(key))
		}

		// Ensure the expected size is reported.
		if gotSize := testTreap.Size(); gotSize != expectedSize {
			t.Fatalf("Size #%d: unexpected byte size - got %d, "+
				"want %d", i, gotSize, expectedSize)
		}
	}

	// Delete the keys one-by-one while checking several of the treap
	// functions work as expected.
	for i := 0; i < numItems; i++ {
		// Hash the serialized int to generate out-of-order keys.
		hash := sha256.Sum256(serializeUint32(uint32(i)))
		key := hash[:]
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
			t.Fatalf("Get #%d: unexpected value - got %x, want nil",
				i, gotVal)
		}

		// Ensure the expected size is reported.
		expectedSize -= (nodeFieldsSize + 64)
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

	keyCount := 100
	expectedVal := []byte("testval")

	expectedSize := uint64(0)
	numItems := 1000
	testTreap := NewImmutable()
	for i := 0; i < numItems/keyCount; i++ {
		keys := make([][]byte, 0, keyCount)
		kvPairs := make([]KVPair, 0, keyCount)
		for j := 0; j < keyCount; j++ {
			n := i*keyCount + j
			key := serializeUint32(uint32(n))
			keys = append(keys, key)
			kvPairs = append(kvPairs, KVPair{key, key})
		}

		testTreap = testTreap.Put(kvPairs...)

		// Get expectedSize.
		for _, key := range keys {
			expectedSize += nodeFieldsSize + uint64(len(key)+len(key))
		}

		// Put duplicate keys with the expected final values.
		expectedPairs := make([]KVPair, keyCount)
		for i := range expectedPairs {
			expectedPairs[i] = KVPair{keys[i], expectedVal}
		}

		testTreap = testTreap.Put(expectedPairs...)

		// Ensure the keys still exist and is the new value.
		for _, key := range keys {
			if gotVal := testTreap.Has(key); !gotVal {
				t.Fatalf("Has: unexpected result - got %v, want true",
					gotVal)
			}
			if gotVal := testTreap.Get(key); !bytes.Equal(gotVal, expectedVal) {
				t.Fatalf("Get: unexpected result - got %x, want %x",
					gotVal, expectedVal)
			}

			expectedSize -= uint64(len(key))
			expectedSize += uint64(len(expectedVal))
		}

		// Ensure the expected size is reported.
		if gotSize := testTreap.Size(); gotSize != expectedSize {
			t.Fatalf("Size: unexpected byte size - got %d, want %d",
				gotSize, expectedSize)
		}
	}
}

// TestImmutableNilValue ensures that putting a nil value into an immutable
// treap results in a key being added with an empty byte slice.
func TestImmutableNilValue(t *testing.T) {
	t.Parallel()

	key := serializeUint32(0)

	// Put the key with a nil value.
	testTreap := NewImmutable()
	testTreap = testTreap.Put(KVPair{key, nil})

	// Ensure the key exists and is an empty byte slice.
	if gotVal := testTreap.Has(key); !gotVal {
		t.Fatalf("Has: unexpected result - got %v, want true", gotVal)
	}
	if gotVal := testTreap.Get(key); gotVal == nil {
		t.Fatalf("Get: unexpected result - got nil, want empty slice")
	}
	if gotVal := testTreap.Get(key); len(gotVal) != 0 {
		t.Fatalf("Get: unexpected result - got %x, want empty slice",
			gotVal)
	}
}

// TestImmutableForEachStopIterator ensures that returning false from the ForEach
// callback on an immutable treap stops iteration early.
func TestImmutableForEachStopIterator(t *testing.T) {
	t.Parallel()

	// Insert a few keys.
	numItems := 10
	testTreap := NewImmutable()
	kvPairs := make([]KVPair, 0, numItems)
	for i := 0; i < numItems; i++ {
		key := serializeUint32(uint32(i))
		kvPairs = append(kvPairs, KVPair{key, key})
	}
	testTreap = testTreap.Put(kvPairs...)

	// Ensure ForEach exits early on false return by caller.
	var numIterated int
	testTreap.ForEach(func(k, v []byte) bool {
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
	keyCount := 100
	testTreap := NewImmutable()
	for i := 0; i < numItems/keyCount; i++ {
		treapSnap := testTreap

		keys := make([][]byte, 0, keyCount)
		kvPairs := make([]KVPair, 0, keyCount)
		for j := 0; j < keyCount; j++ {
			n := i*keyCount + j
			key := serializeUint32(uint32(n))
			keys = append(keys, key)
			kvPairs = append(kvPairs, KVPair{key, key})
		}

		testTreap = testTreap.Put(kvPairs...)

		// Ensure the length of the treap snapshot is the expected
		// value.
		if gotLen := treapSnap.Len(); gotLen != i*keyCount {
			t.Fatalf("Len #%d: unexpected length - got %d, want %d",
				i, gotLen, i)
		}

		for j, key := range keys {
			// Ensure the treap snapshot does not have the key.
			if treapSnap.Has(key) {
				t.Fatalf("Has #%d#%d: key %q is in treap", i, j, key)
			}

			// Get the key that doesn't exist in the treap snapshot and
			// ensure it is nil.
			if gotVal := treapSnap.Get(key); gotVal != nil {
				t.Fatalf("Get #%d#%d: unexpected value - got %x, want nil",
					i, j, gotVal)
			}

			// Ensure the expected size is reported.
			if gotSize := treapSnap.Size(); gotSize != expectedSize {
				t.Fatalf("Size #%d#%d: unexpected byte size - got %d, "+
					"want %d", i, j, gotSize, expectedSize)
			}
		}

		expectedSize += (nodeFieldsSize + 8) * uint64(keyCount)
	}

	// Delete the keys one-by-one while checking several of the treap
	// functions work as expected.
	for i := 0; i < numItems; i++ {
		treapSnap := testTreap

		key := serializeUint32(uint32(i))
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
		if gotVal := treapSnap.Get(key); !bytes.Equal(gotVal, key) {
			t.Fatalf("Get #%d: unexpected value - got %x, want %x",
				i, gotVal, key)
		}

		// Ensure the expected size is reported.
		if gotSize := treapSnap.Size(); gotSize != expectedSize {
			t.Fatalf("Size #%d: unexpected byte size - got %d, "+
				"want %d", i, gotSize, expectedSize)
		}
		expectedSize -= (nodeFieldsSize + 8)
	}
}
