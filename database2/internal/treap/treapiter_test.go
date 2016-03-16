// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package treap

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestMutableIterator ensures that the general behavior of mutable treap
// iterators is as expected including tests for first, last, ordered and reverse
// ordered iteration, limiting the range, seeking, and initially unpositioned.
func TestMutableIterator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		numKeys       int
		step          int
		startKey      []byte
		limitKey      []byte
		expectedFirst []byte
		expectedLast  []byte
		seekKey       []byte
		expectedSeek  []byte
	}{
		// No range limits.  Values are the set (0, 1, 2, ..., 49).
		// Seek existing value.
		{
			numKeys:       50,
			step:          1,
			expectedFirst: serializeUint32(0),
			expectedLast:  serializeUint32(49),
			seekKey:       serializeUint32(12),
			expectedSeek:  serializeUint32(12),
		},

		// Limited to range [24, end].  Values are the set
		// (0, 2, 4, ..., 48).  Seek value that doesn't exist and is
		// greater than largest existing key.
		{
			numKeys:       50,
			step:          2,
			startKey:      serializeUint32(24),
			expectedFirst: serializeUint32(24),
			expectedLast:  serializeUint32(48),
			seekKey:       serializeUint32(49),
			expectedSeek:  nil,
		},

		// Limited to range [start, 25).  Values are the set
		// (0, 3, 6, ..., 48).  Seek value that doesn't exist but is
		// before an existing value within the range.
		{
			numKeys:       50,
			step:          3,
			limitKey:      serializeUint32(25),
			expectedFirst: serializeUint32(0),
			expectedLast:  serializeUint32(24),
			seekKey:       serializeUint32(17),
			expectedSeek:  serializeUint32(18),
		},

		// Limited to range [10, 21).  Values are the set
		// (0, 4, ..., 48).  Seek value that exists, but is before the
		// minimum allowed range.
		{
			numKeys:       50,
			step:          4,
			startKey:      serializeUint32(10),
			limitKey:      serializeUint32(21),
			expectedFirst: serializeUint32(12),
			expectedLast:  serializeUint32(20),
			seekKey:       serializeUint32(4),
			expectedSeek:  nil,
		},

		// Limited by prefix {0,0,0}, range [{0,0,0}, {0,0,1}).
		// Since it's a bytewise compare,  {0,0,0,...} < {0,0,1}.
		// Seek existing value within the allowed range.
		{
			numKeys:       300,
			step:          1,
			startKey:      []byte{0x00, 0x00, 0x00},
			limitKey:      []byte{0x00, 0x00, 0x01},
			expectedFirst: serializeUint32(0),
			expectedLast:  serializeUint32(255),
			seekKey:       serializeUint32(100),
			expectedSeek:  serializeUint32(100),
		},
	}

testLoop:
	for i, test := range tests {
		// Insert a bunch of keys.
		testTreap := NewMutable()
		for i := 0; i < test.numKeys; i += test.step {
			key := serializeUint32(uint32(i))
			testTreap.Put(key, key)
		}

		// Create new iterator limited by the test params.
		iter := testTreap.Iterator(test.startKey, test.limitKey)

		// Ensure the first item is accurate.
		hasFirst := iter.First()
		if !hasFirst && test.expectedFirst != nil {
			t.Errorf("First #%d: unexpected exhausted iterator", i)
			continue
		}
		gotKey := iter.Key()
		if !bytes.Equal(gotKey, test.expectedFirst) {
			t.Errorf("First.Key #%d: unexpected key - got %x, "+
				"want %x", i, gotKey, test.expectedFirst)
			continue
		}
		gotVal := iter.Value()
		if !bytes.Equal(gotVal, test.expectedFirst) {
			t.Errorf("First.Value #%d: unexpected value - got %x, "+
				"want %x", i, gotVal, test.expectedFirst)
			continue
		}

		// Ensure the iterator gives the expected items in order.
		curNum := binary.BigEndian.Uint32(test.expectedFirst)
		for iter.Next() {
			curNum += uint32(test.step)

			// Ensure key is as expected.
			gotKey := iter.Key()
			expectedKey := serializeUint32(curNum)
			if !bytes.Equal(gotKey, expectedKey) {
				t.Errorf("iter.Key #%d (%d): unexpected key - "+
					"got %x, want %x", i, curNum, gotKey,
					expectedKey)
				continue testLoop
			}

			// Ensure value is as expected.
			gotVal := iter.Value()
			if !bytes.Equal(gotVal, expectedKey) {
				t.Errorf("iter.Value #%d (%d): unexpected "+
					"value - got %x, want %x", i, curNum,
					gotVal, expectedKey)
				continue testLoop
			}
		}

		// Ensure iterator is exhausted.
		if iter.Valid() {
			t.Errorf("Valid #%d: iterator should be exhausted", i)
			continue
		}

		// Ensure the last item is accurate.
		hasLast := iter.Last()
		if !hasLast && test.expectedLast != nil {
			t.Errorf("Last #%d: unexpected exhausted iterator", i)
			continue
		}
		gotKey = iter.Key()
		if !bytes.Equal(gotKey, test.expectedLast) {
			t.Errorf("Last.Key #%d: unexpected key - got %x, "+
				"want %x", i, gotKey, test.expectedLast)
			continue
		}
		gotVal = iter.Value()
		if !bytes.Equal(gotVal, test.expectedLast) {
			t.Errorf("Last.Value #%d: unexpected value - got %x, "+
				"want %x", i, gotVal, test.expectedLast)
			continue
		}

		// Ensure the iterator gives the expected items in reverse
		// order.
		curNum = binary.BigEndian.Uint32(test.expectedLast)
		for iter.Prev() {
			curNum -= uint32(test.step)

			// Ensure key is as expected.
			gotKey := iter.Key()
			expectedKey := serializeUint32(curNum)
			if !bytes.Equal(gotKey, expectedKey) {
				t.Errorf("iter.Key #%d (%d): unexpected key - "+
					"got %x, want %x", i, curNum, gotKey,
					expectedKey)
				continue testLoop
			}

			// Ensure value is as expected.
			gotVal := iter.Value()
			if !bytes.Equal(gotVal, expectedKey) {
				t.Errorf("iter.Value #%d (%d): unexpected "+
					"value - got %x, want %x", i, curNum,
					gotVal, expectedKey)
				continue testLoop
			}
		}

		// Ensure iterator is exhausted.
		if iter.Valid() {
			t.Errorf("Valid #%d: iterator should be exhausted", i)
			continue
		}

		// Seek to the provided key.
		seekValid := iter.Seek(test.seekKey)
		if !seekValid && test.expectedSeek != nil {
			t.Errorf("Seek #%d: unexpected exhausted iterator", i)
			continue
		}
		gotKey = iter.Key()
		if !bytes.Equal(gotKey, test.expectedSeek) {
			t.Errorf("Seek.Key #%d: unexpected key - got %x, "+
				"want %x", i, gotKey, test.expectedSeek)
			continue
		}
		gotVal = iter.Value()
		if !bytes.Equal(gotVal, test.expectedSeek) {
			t.Errorf("Seek.Value #%d: unexpected value - got %x, "+
				"want %x", i, gotVal, test.expectedSeek)
			continue
		}

		// Recreate the iterator and ensure calling Next on it before it
		// has been positioned gives the first element.
		iter = testTreap.Iterator(test.startKey, test.limitKey)
		hasNext := iter.Next()
		if !hasNext && test.expectedFirst != nil {
			t.Errorf("Next #%d: unexpected exhausted iterator", i)
			continue
		}
		gotKey = iter.Key()
		if !bytes.Equal(gotKey, test.expectedFirst) {
			t.Errorf("Next.Key #%d: unexpected key - got %x, "+
				"want %x", i, gotKey, test.expectedFirst)
			continue
		}
		gotVal = iter.Value()
		if !bytes.Equal(gotVal, test.expectedFirst) {
			t.Errorf("Next.Value #%d: unexpected value - got %x, "+
				"want %x", i, gotVal, test.expectedFirst)
			continue
		}

		// Recreate the iterator and ensure calling Prev on it before it
		// has been positioned gives the first element.
		iter = testTreap.Iterator(test.startKey, test.limitKey)
		hasPrev := iter.Prev()
		if !hasPrev && test.expectedLast != nil {
			t.Errorf("Prev #%d: unexpected exhausted iterator", i)
			continue
		}
		gotKey = iter.Key()
		if !bytes.Equal(gotKey, test.expectedLast) {
			t.Errorf("Prev.Key #%d: unexpected key - got %x, "+
				"want %x", i, gotKey, test.expectedLast)
			continue
		}
		gotVal = iter.Value()
		if !bytes.Equal(gotVal, test.expectedLast) {
			t.Errorf("Next.Value #%d: unexpected value - got %x, "+
				"want %x", i, gotVal, test.expectedLast)
			continue
		}
	}
}

// TestMutableEmptyIterator ensures that the various functions behave as
// expected when a mutable treap is empty.
func TestMutableEmptyIterator(t *testing.T) {
	t.Parallel()

	// Create iterator against empty treap.
	testTreap := NewMutable()
	iter := testTreap.Iterator(nil, nil)

	// Ensure Valid on empty iterator reports it as exhausted.
	if iter.Valid() {
		t.Fatal("Valid: iterator should be exhausted")
	}

	// Ensure First and Last on empty iterator report it as exhausted.
	if iter.First() {
		t.Fatal("First: iterator should be exhausted")
	}
	if iter.Last() {
		t.Fatal("Last: iterator should be exhausted")
	}

	// Ensure Next and Prev on empty iterator report it as exhausted.
	if iter.Next() {
		t.Fatal("Next: iterator should be exhausted")
	}
	if iter.Prev() {
		t.Fatal("Prev: iterator should be exhausted")
	}

	// Ensure Key and Value on empty iterator are nil.
	if gotKey := iter.Key(); gotKey != nil {
		t.Fatalf("Key: should be nil - got %q", gotKey)
	}
	if gotVal := iter.Value(); gotVal != nil {
		t.Fatalf("Value: should be nil - got %q", gotVal)
	}

	// Ensure Next and Prev report exhausted after forcing a reseek on an
	// empty iterator.
	iter.ForceReseek()
	if iter.Next() {
		t.Fatal("Next: iterator should be exhausted")
	}
	iter.ForceReseek()
	if iter.Prev() {
		t.Fatal("Prev: iterator should be exhausted")
	}
}

// TestIteratorUpdates ensures that issuing a call to ForceReseek on an iterator
// that had the underlying mutable treap updated works as expected.
func TestIteratorUpdates(t *testing.T) {
	t.Parallel()

	// Create a new treap with various values inserted in no particular
	// order.  The resulting keys are the set (2, 4, 7, 11, 18, 25).
	testTreap := NewMutable()
	testTreap.Put(serializeUint32(7), nil)
	testTreap.Put(serializeUint32(2), nil)
	testTreap.Put(serializeUint32(18), nil)
	testTreap.Put(serializeUint32(11), nil)
	testTreap.Put(serializeUint32(25), nil)
	testTreap.Put(serializeUint32(4), nil)

	// Create an iterator against the treap with a range that excludes the
	// lowest and highest entries.  The limited set is then (4, 7, 11, 18)
	iter := testTreap.Iterator(serializeUint32(3), serializeUint32(25))

	// Delete a key from the middle of the range and notify the iterator to
	// force a reseek.
	testTreap.Delete(serializeUint32(11))
	iter.ForceReseek()

	// Ensure that calling Next on the iterator after the forced reseek
	// gives the expected key.  The limited set of keys at this point is
	// (4, 7, 18) and the iterator has not yet been positioned.
	if !iter.Next() {
		t.Fatal("ForceReseek.Next: unexpected exhausted iterator")
	}
	wantKey := serializeUint32(4)
	gotKey := iter.Key()
	if !bytes.Equal(gotKey, wantKey) {
		t.Fatalf("ForceReseek.Key: unexpected key - got %x, want %x",
			gotKey, wantKey)
	}

	// Delete the key the iterator is currently position at and notify the
	// iterator to force a reseek.
	testTreap.Delete(serializeUint32(4))
	iter.ForceReseek()

	// Ensure that calling Next on the iterator after the forced reseek
	// gives the expected key.  The limited set of keys at this point is
	// (7, 18) and the iterator is positioned at a deleted entry before 7.
	if !iter.Next() {
		t.Fatal("ForceReseek.Next: unexpected exhausted iterator")
	}
	wantKey = serializeUint32(7)
	gotKey = iter.Key()
	if !bytes.Equal(gotKey, wantKey) {
		t.Fatalf("ForceReseek.Key: unexpected key - got %x, want %x",
			gotKey, wantKey)
	}

	// Add a key before the current key the iterator is position at and
	// notify the iterator to force a reseek.
	testTreap.Put(serializeUint32(4), nil)
	iter.ForceReseek()

	// Ensure that calling Prev on the iterator after the forced reseek
	// gives the expected key.  The limited set of keys at this point is
	// (4, 7, 18) and the iterator is positioned at 7.
	if !iter.Prev() {
		t.Fatal("ForceReseek.Prev: unexpected exhausted iterator")
	}
	wantKey = serializeUint32(4)
	gotKey = iter.Key()
	if !bytes.Equal(gotKey, wantKey) {
		t.Fatalf("ForceReseek.Key: unexpected key - got %x, want %x",
			gotKey, wantKey)
	}

	// Delete the next key the iterator would ordinarily move to then notify
	// the iterator to force a reseek.
	testTreap.Delete(serializeUint32(7))
	iter.ForceReseek()

	// Ensure that calling Next on the iterator after the forced reseek
	// gives the expected key.  The limited set of keys at this point is
	// (4, 18) and the iterator is positioned at 4.
	if !iter.Next() {
		t.Fatal("ForceReseek.Next: unexpected exhausted iterator")
	}
	wantKey = serializeUint32(18)
	gotKey = iter.Key()
	if !bytes.Equal(gotKey, wantKey) {
		t.Fatalf("ForceReseek.Key: unexpected key - got %x, want %x",
			gotKey, wantKey)
	}
}

// TestImmutableIterator ensures that the general behavior of immutable treap
// iterators is as expected including tests for first, last, ordered and reverse
// ordered iteration, limiting the range, seeking, and initially unpositioned.
func TestImmutableIterator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		numKeys       int
		step          int
		startKey      []byte
		limitKey      []byte
		expectedFirst []byte
		expectedLast  []byte
		seekKey       []byte
		expectedSeek  []byte
	}{
		// No range limits.  Values are the set (0, 1, 2, ..., 49).
		// Seek existing value.
		{
			numKeys:       50,
			step:          1,
			expectedFirst: serializeUint32(0),
			expectedLast:  serializeUint32(49),
			seekKey:       serializeUint32(12),
			expectedSeek:  serializeUint32(12),
		},

		// Limited to range [24, end].  Values are the set
		// (0, 2, 4, ..., 48).  Seek value that doesn't exist and is
		// greater than largest existing key.
		{
			numKeys:       50,
			step:          2,
			startKey:      serializeUint32(24),
			expectedFirst: serializeUint32(24),
			expectedLast:  serializeUint32(48),
			seekKey:       serializeUint32(49),
			expectedSeek:  nil,
		},

		// Limited to range [start, 25).  Values are the set
		// (0, 3, 6, ..., 48).  Seek value that doesn't exist but is
		// before an existing value within the range.
		{
			numKeys:       50,
			step:          3,
			limitKey:      serializeUint32(25),
			expectedFirst: serializeUint32(0),
			expectedLast:  serializeUint32(24),
			seekKey:       serializeUint32(17),
			expectedSeek:  serializeUint32(18),
		},

		// Limited to range [10, 21).  Values are the set
		// (0, 4, ..., 48).  Seek value that exists, but is before the
		// minimum allowed range.
		{
			numKeys:       50,
			step:          4,
			startKey:      serializeUint32(10),
			limitKey:      serializeUint32(21),
			expectedFirst: serializeUint32(12),
			expectedLast:  serializeUint32(20),
			seekKey:       serializeUint32(4),
			expectedSeek:  nil,
		},

		// Limited by prefix {0,0,0}, range [{0,0,0}, {0,0,1}).
		// Since it's a bytewise compare,  {0,0,0,...} < {0,0,1}.
		// Seek existing value within the allowed range.
		{
			numKeys:       300,
			step:          1,
			startKey:      []byte{0x00, 0x00, 0x00},
			limitKey:      []byte{0x00, 0x00, 0x01},
			expectedFirst: serializeUint32(0),
			expectedLast:  serializeUint32(255),
			seekKey:       serializeUint32(100),
			expectedSeek:  serializeUint32(100),
		},
	}

testLoop:
	for i, test := range tests {
		// Insert a bunch of keys.
		testTreap := NewImmutable()
		for i := 0; i < test.numKeys; i += test.step {
			key := serializeUint32(uint32(i))
			testTreap = testTreap.Put(key, key)
		}

		// Create new iterator limited by the test params.
		iter := testTreap.Iterator(test.startKey, test.limitKey)

		// Ensure the first item is accurate.
		hasFirst := iter.First()
		if !hasFirst && test.expectedFirst != nil {
			t.Errorf("First #%d: unexpected exhausted iterator", i)
			continue
		}
		gotKey := iter.Key()
		if !bytes.Equal(gotKey, test.expectedFirst) {
			t.Errorf("First.Key #%d: unexpected key - got %x, "+
				"want %x", i, gotKey, test.expectedFirst)
			continue
		}
		gotVal := iter.Value()
		if !bytes.Equal(gotVal, test.expectedFirst) {
			t.Errorf("First.Value #%d: unexpected value - got %x, "+
				"want %x", i, gotVal, test.expectedFirst)
			continue
		}

		// Ensure the iterator gives the expected items in order.
		curNum := binary.BigEndian.Uint32(test.expectedFirst)
		for iter.Next() {
			curNum += uint32(test.step)

			// Ensure key is as expected.
			gotKey := iter.Key()
			expectedKey := serializeUint32(curNum)
			if !bytes.Equal(gotKey, expectedKey) {
				t.Errorf("iter.Key #%d (%d): unexpected key - "+
					"got %x, want %x", i, curNum, gotKey,
					expectedKey)
				continue testLoop
			}

			// Ensure value is as expected.
			gotVal := iter.Value()
			if !bytes.Equal(gotVal, expectedKey) {
				t.Errorf("iter.Value #%d (%d): unexpected "+
					"value - got %x, want %x", i, curNum,
					gotVal, expectedKey)
				continue testLoop
			}
		}

		// Ensure iterator is exhausted.
		if iter.Valid() {
			t.Errorf("Valid #%d: iterator should be exhausted", i)
			continue
		}

		// Ensure the last item is accurate.
		hasLast := iter.Last()
		if !hasLast && test.expectedLast != nil {
			t.Errorf("Last #%d: unexpected exhausted iterator", i)
			continue
		}
		gotKey = iter.Key()
		if !bytes.Equal(gotKey, test.expectedLast) {
			t.Errorf("Last.Key #%d: unexpected key - got %x, "+
				"want %x", i, gotKey, test.expectedLast)
			continue
		}
		gotVal = iter.Value()
		if !bytes.Equal(gotVal, test.expectedLast) {
			t.Errorf("Last.Value #%d: unexpected value - got %x, "+
				"want %x", i, gotVal, test.expectedLast)
			continue
		}

		// Ensure the iterator gives the expected items in reverse
		// order.
		curNum = binary.BigEndian.Uint32(test.expectedLast)
		for iter.Prev() {
			curNum -= uint32(test.step)

			// Ensure key is as expected.
			gotKey := iter.Key()
			expectedKey := serializeUint32(curNum)
			if !bytes.Equal(gotKey, expectedKey) {
				t.Errorf("iter.Key #%d (%d): unexpected key - "+
					"got %x, want %x", i, curNum, gotKey,
					expectedKey)
				continue testLoop
			}

			// Ensure value is as expected.
			gotVal := iter.Value()
			if !bytes.Equal(gotVal, expectedKey) {
				t.Errorf("iter.Value #%d (%d): unexpected "+
					"value - got %x, want %x", i, curNum,
					gotVal, expectedKey)
				continue testLoop
			}
		}

		// Ensure iterator is exhausted.
		if iter.Valid() {
			t.Errorf("Valid #%d: iterator should be exhausted", i)
			continue
		}

		// Seek to the provided key.
		seekValid := iter.Seek(test.seekKey)
		if !seekValid && test.expectedSeek != nil {
			t.Errorf("Seek #%d: unexpected exhausted iterator", i)
			continue
		}
		gotKey = iter.Key()
		if !bytes.Equal(gotKey, test.expectedSeek) {
			t.Errorf("Seek.Key #%d: unexpected key - got %x, "+
				"want %x", i, gotKey, test.expectedSeek)
			continue
		}
		gotVal = iter.Value()
		if !bytes.Equal(gotVal, test.expectedSeek) {
			t.Errorf("Seek.Value #%d: unexpected value - got %x, "+
				"want %x", i, gotVal, test.expectedSeek)
			continue
		}

		// Recreate the iterator and ensure calling Next on it before it
		// has been positioned gives the first element.
		iter = testTreap.Iterator(test.startKey, test.limitKey)
		hasNext := iter.Next()
		if !hasNext && test.expectedFirst != nil {
			t.Errorf("Next #%d: unexpected exhausted iterator", i)
			continue
		}
		gotKey = iter.Key()
		if !bytes.Equal(gotKey, test.expectedFirst) {
			t.Errorf("Next.Key #%d: unexpected key - got %x, "+
				"want %x", i, gotKey, test.expectedFirst)
			continue
		}
		gotVal = iter.Value()
		if !bytes.Equal(gotVal, test.expectedFirst) {
			t.Errorf("Next.Value #%d: unexpected value - got %x, "+
				"want %x", i, gotVal, test.expectedFirst)
			continue
		}

		// Recreate the iterator and ensure calling Prev on it before it
		// has been positioned gives the first element.
		iter = testTreap.Iterator(test.startKey, test.limitKey)
		hasPrev := iter.Prev()
		if !hasPrev && test.expectedLast != nil {
			t.Errorf("Prev #%d: unexpected exhausted iterator", i)
			continue
		}
		gotKey = iter.Key()
		if !bytes.Equal(gotKey, test.expectedLast) {
			t.Errorf("Prev.Key #%d: unexpected key - got %x, "+
				"want %x", i, gotKey, test.expectedLast)
			continue
		}
		gotVal = iter.Value()
		if !bytes.Equal(gotVal, test.expectedLast) {
			t.Errorf("Next.Value #%d: unexpected value - got %x, "+
				"want %x", i, gotVal, test.expectedLast)
			continue
		}
	}
}

// TestImmutableEmptyIterator ensures that the various functions behave as
// expected when an immutable treap is empty.
func TestImmutableEmptyIterator(t *testing.T) {
	t.Parallel()

	// Create iterator against empty treap.
	testTreap := NewImmutable()
	iter := testTreap.Iterator(nil, nil)

	// Ensure Valid on empty iterator reports it as exhausted.
	if iter.Valid() {
		t.Fatal("Valid: iterator should be exhausted")
	}

	// Ensure First and Last on empty iterator report it as exhausted.
	if iter.First() {
		t.Fatal("First: iterator should be exhausted")
	}
	if iter.Last() {
		t.Fatal("Last: iterator should be exhausted")
	}

	// Ensure Next and Prev on empty iterator report it as exhausted.
	if iter.Next() {
		t.Fatal("Next: iterator should be exhausted")
	}
	if iter.Prev() {
		t.Fatal("Prev: iterator should be exhausted")
	}

	// Ensure Key and Value on empty iterator are nil.
	if gotKey := iter.Key(); gotKey != nil {
		t.Fatalf("Key: should be nil - got %q", gotKey)
	}
	if gotVal := iter.Value(); gotVal != nil {
		t.Fatalf("Value: should be nil - got %q", gotVal)
	}

	// Ensure calling ForceReseek on an immutable treap iterator does not
	// cause any issues since it only applies to mutable treap iterators.
	iter.ForceReseek()
	if iter.Next() {
		t.Fatal("Next: iterator should be exhausted")
	}
	iter.ForceReseek()
	if iter.Prev() {
		t.Fatal("Prev: iterator should be exhausted")
	}
}
