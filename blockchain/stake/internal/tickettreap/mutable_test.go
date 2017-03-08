// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package tickettreap

import (
	"bytes"
	"crypto/sha256"
	"reflect"
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
}

// TestMutableReset ensures that resetting an existing mutable treap works as
// expected.
func TestMutableReset(t *testing.T) {
	t.Parallel()

	// Insert a few keys.
	numItems := 10
	testTreap := NewMutable()
	for i := 0; i < numItems; i++ {
		key := uint32ToKey(uint32(i))
		value := &Value{Height: uint32(i)}
		testTreap.Put(key, value)
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
		// Ensure the treap no longer has the key.
		key := uint32ToKey(uint32(i))
		if testTreap.Has(key) {
			t.Fatalf("Has #%d: key %q is in treap", i, key)
		}

		// Get the key that no longer exists from the treap and ensure
		// it is nil.
		if gotVal := testTreap.Get(key); gotVal != nil {
			t.Fatalf("Get #%d: unexpected value - got %v, want nil",
				i, gotVal)
		}
	}

	// Ensure the number of keys iterated by ForEach is zero.
	var numIterated int
	testTreap.ForEach(func(k Key, v *Value) bool {
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
		key := uint32ToKey(uint32(i))
		value := &Value{Height: uint32(i)}
		testTreap.Put(key, value)

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
		expectedSize += (nodeFieldsSize + uint64(len(key)) + 4)
		if gotSize := testTreap.Size(); gotSize != expectedSize {
			t.Fatalf("Size #%d: unexpected byte size - got %d, "+
				"want %d", i, gotSize, expectedSize)
		}
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
		key := uint32ToKey(uint32(i))
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
			t.Fatalf("Get #%d: unexpected value - got %v, want nil",
				i, gotVal)
		}

		// Ensure the expected size is reported.
		expectedSize -= (nodeFieldsSize + uint64(len(key)) + 4)
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
		key := uint32ToKey(uint32(numItems - i - 1))
		value := &Value{Height: uint32(numItems - i - 1)}
		testTreap.Put(key, value)

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
		expectedSize += (nodeFieldsSize + uint64(len(key)) + 4)
		if gotSize := testTreap.Size(); gotSize != expectedSize {
			t.Fatalf("Size #%d: unexpected byte size - got %d, "+
				"want %d", i, gotSize, expectedSize)
		}
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
			t.Fatalf("Get #%d: unexpected value - got %v, want nil",
				i, gotVal)
		}

		// Ensure the expected size is reported.
		expectedSize -= (nodeFieldsSize + uint64(len(key)) + 4)
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
		key := Key(sha256.Sum256(serializeUint32(uint32(i))))
		value := &Value{Height: uint32(i)}
		testTreap.Put(key, value)

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
		expectedSize += nodeFieldsSize + uint64(len(key)) + 4
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
			t.Fatalf("Get #%d: unexpected value - got %v, want nil",
				i, gotVal)
		}

		// Ensure the expected size is reported.
		expectedSize -= (nodeFieldsSize + uint64(len(key)) + 4)
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

	expectedVal := &Value{Height: 10000}
	expectedSize := uint64(0)
	numItems := 1000
	testTreap := NewMutable()
	for i := 0; i < numItems; i++ {
		key := uint32ToKey(uint32(i))
		value := &Value{Height: uint32(i)}
		testTreap.Put(key, value)
		expectedSize += nodeFieldsSize + uint64(len(key)) + 4

		// Put a duplicate key with the the expected final value.
		testTreap.Put(key, expectedVal)

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

// TestMutableNilValue ensures that putting a nil value into a mutable treap
// results in a NOOP.
func TestMutableNilValue(t *testing.T) {
	t.Parallel()

	key := uint32ToKey(0)

	// Put the key with a nil value.
	testTreap := NewMutable()
	testTreap.Put(key, nil)

	// Ensure the key does NOT exist.
	if gotVal := testTreap.Has(key); gotVal {
		t.Fatalf("Has: unexpected result - got %v, want false", gotVal)
	}
	if gotVal := testTreap.Get(key); gotVal != nil {
		t.Fatalf("Get: unexpected result - got %v, want nil", gotVal)
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
		key := uint32ToKey(uint32(i))
		value := &Value{Height: uint32(i)}
		testTreap.Put(key, value)
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
