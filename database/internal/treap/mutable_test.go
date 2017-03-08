// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package treap

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

// TestMutableEmpty ensures calling functions on an empty mutable treap works as
// expected.
func TestMutableEmpty(t *testing.T) {
	t.Parallel()

	// Ensure the treap length is the expected value.
	testTreap := NewMutable()
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

// TestMutableReset ensures that resetting an existing mutable treap works as
// expected.
func TestMutableReset(t *testing.T) {
	t.Parallel()

	// Insert a few keys.
	numItems := 10
	testTreap := NewMutable()
	for i := 0; i < numItems; i++ {
		key := serializeUint32(uint32(i))
		testTreap.Put(key, key)
	}

	// Reset it.
	testTreap.Reset()

	// Ensure the treap length is now 0.
	if gotLen := testTreap.Len(); gotLen != 0 {
		t.Fatalf("Len: unexpected length - got %d, want %d", gotLen, 0)
	}

	// Ensure the reported size is now 0.
	if gotSize := testTreap.Size(); gotSize != 0 {
		t.Fatalf("Size: unexpected byte size - got %d, want 0",
			gotSize)
	}

	// Ensure the treap no longer has any of the keys.
	for i := 0; i < numItems; i++ {
		key := serializeUint32(uint32(i))

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
	}

	// Ensure the number of keys iterated by ForEach is zero.
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

// TestMutableSequential ensures that putting keys into a mutable treap in
// sequential order works as expected.
func TestMutableSequential(t *testing.T) {
	t.Parallel()

	// Insert a bunch of sequential keys while checking several of the treap
	// functions work as expected.
	expectedSize := uint64(0)
	numItems := 1000
	testTreap := NewMutable()
	for i := 0; i < numItems; i++ {
		key := serializeUint32(uint32(i))
		testTreap.Put(key, key)

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
		if gotVal := testTreap.Get(key); !bytes.Equal(gotVal, key) {
			t.Fatalf("Get #%d: unexpected value - got %x, want %x",
				i, gotVal, key)
		}

		// Ensure the expected size is reported.
		expectedSize += (nodeFieldsSize + 8)
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
		testTreap.Delete(key)

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

// TestMutableReverseSequential ensures that putting keys into a mutable treap
// in reverse sequential order works as expected.
func TestMutableReverseSequential(t *testing.T) {
	t.Parallel()

	// Insert a bunch of sequential keys while checking several of the treap
	// functions work as expected.
	expectedSize := uint64(0)
	numItems := 1000
	testTreap := NewMutable()
	for i := 0; i < numItems; i++ {
		key := serializeUint32(uint32(numItems - i - 1))
		testTreap.Put(key, key)

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
		if gotVal := testTreap.Get(key); !bytes.Equal(gotVal, key) {
			t.Fatalf("Get #%d: unexpected value - got %x, want %x",
				i, gotVal, key)
		}

		// Ensure the expected size is reported.
		expectedSize += (nodeFieldsSize + 8)
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
		testTreap.Delete(key)

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

// TestMutableUnordered ensures that putting keys into a mutable treap in no
// paritcular order works as expected.
func TestMutableUnordered(t *testing.T) {
	t.Parallel()

	// Insert a bunch of out-of-order keys while checking several of the
	// treap functions work as expected.
	expectedSize := uint64(0)
	numItems := 1000
	testTreap := NewMutable()
	for i := 0; i < numItems; i++ {
		// Hash the serialized int to generate out-of-order keys.
		hash := sha256.Sum256(serializeUint32(uint32(i)))
		key := hash[:]
		testTreap.Put(key, key)

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
		if gotVal := testTreap.Get(key); !bytes.Equal(gotVal, key) {
			t.Fatalf("Get #%d: unexpected value - got %x, want %x",
				i, gotVal, key)
		}

		// Ensure the expected size is reported.
		expectedSize += nodeFieldsSize + uint64(len(key)+len(key))
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
		testTreap.Delete(key)

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

// TestMutableDuplicatePut ensures that putting a duplicate key into a mutable
// treap updates the existing value.
func TestMutableDuplicatePut(t *testing.T) {
	t.Parallel()

	key := serializeUint32(0)
	val := []byte("testval")

	// Put the key twice with the second put being the expected final value.
	testTreap := NewMutable()
	testTreap.Put(key, key)
	testTreap.Put(key, val)

	// Ensure the key still exists and is the new value.
	if gotVal := testTreap.Has(key); !gotVal {
		t.Fatalf("Has: unexpected result - got %v, want false", gotVal)
	}
	if gotVal := testTreap.Get(key); !bytes.Equal(gotVal, val) {
		t.Fatalf("Get: unexpected result - got %x, want %x", gotVal, val)
	}

	// Ensure the expected size is reported.
	expectedSize := uint64(nodeFieldsSize + len(key) + len(val))
	if gotSize := testTreap.Size(); gotSize != expectedSize {
		t.Fatalf("Size: unexpected byte size - got %d, want %d",
			gotSize, expectedSize)
	}
}

// TestMutableNilValue ensures that putting a nil value into a mutable treap
// results in a key being added with an empty byte slice.
func TestMutableNilValue(t *testing.T) {
	t.Parallel()

	key := serializeUint32(0)

	// Put the key with a nil value.
	testTreap := NewMutable()
	testTreap.Put(key, nil)

	// Ensure the key exists and is an empty byte slice.
	if gotVal := testTreap.Has(key); !gotVal {
		t.Fatalf("Has: unexpected result - got %v, want false", gotVal)
	}
	if gotVal := testTreap.Get(key); gotVal == nil {
		t.Fatalf("Get: unexpected result - got nil, want empty slice")
	}
	if gotVal := testTreap.Get(key); len(gotVal) != 0 {
		t.Fatalf("Get: unexpected result - got %x, want empty slice",
			gotVal)
	}
}

// TestMutableForEachStopIterator ensures that returning false from the ForEach
// callback of a mutable treap stops iteration early.
func TestMutableForEachStopIterator(t *testing.T) {
	t.Parallel()

	// Insert a few keys.
	numItems := 10
	testTreap := NewMutable()
	for i := 0; i < numItems; i++ {
		key := serializeUint32(uint32(i))
		testTreap.Put(key, key)
	}

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
