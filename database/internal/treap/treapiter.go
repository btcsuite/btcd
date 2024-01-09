// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package treap

import "bytes"

// Iterator represents an iterator for forwards and backwards iteration over
// the contents of a treap (mutable or immutable).
type Iterator struct {
	t        *Mutable    // Mutable treap iterator is associated with or nil
	root     *treapNode  // Root node of treap iterator is associated with
	node     *treapNode  // The node the iterator is positioned at
	parents  parentStack // The stack of parents needed to iterate
	isNew    bool        // Whether the iterator has been positioned
	seekKey  []byte      // Used to handle dynamic updates for mutable treap
	startKey []byte      // Used to limit the iterator to a range
	limitKey []byte      // Used to limit the iterator to a range
}

// limitIterator clears the current iterator node if it is outside of the range
// specified when the iterator was created.  It returns whether the iterator is
// valid.
func (iter *Iterator) limitIterator() bool {
	if iter.node == nil {
		return false
	}

	node := iter.node
	if iter.startKey != nil && bytes.Compare(node.key, iter.startKey) < 0 {
		iter.node = nil
		return false
	}

	if iter.limitKey != nil && bytes.Compare(node.key, iter.limitKey) >= 0 {
		iter.node = nil
		return false
	}

	return true
}

// seek moves the iterator based on the provided key and flags.
//
// When the exact match flag is set, the iterator will either be moved to first
// key in the treap that exactly matches the provided key, or the one
// before/after it depending on the greater flag.
//
// When the exact match flag is NOT set, the iterator will be moved to the first
// key in the treap before/after the provided key depending on the greater flag.
//
// In all cases, the limits specified when the iterator was created are
// respected.
func (iter *Iterator) seek(key []byte, exactMatch bool, greater bool) bool {
	iter.node = nil
	iter.parents = parentStack{}
	var selectedNodeDepth int
	for node := iter.root; node != nil; {
		iter.parents.Push(node)

		// Traverse left or right depending on the result of the
		// comparison.  Also, set the iterator to the node depending on
		// the flags so the iterator is positioned properly when an
		// exact match isn't found.
		compareResult := bytes.Compare(key, node.key)
		if compareResult < 0 {
			if greater {
				iter.node = node
				selectedNodeDepth = iter.parents.Len() - 1
			}
			node = node.left
			continue
		}
		if compareResult > 0 {
			if !greater {
				iter.node = node
				selectedNodeDepth = iter.parents.Len() - 1
			}
			node = node.right
			continue
		}

		// The key is an exact match.  Set the iterator and return now
		// when the exact match flag is set.
		if exactMatch {
			iter.node = node
			iter.parents.Pop()
			return iter.limitIterator()
		}

		// The key is an exact match, but the exact match is not set, so
		// choose which direction to go based on whether the larger or
		// smaller key was requested.
		if greater {
			node = node.right
		} else {
			node = node.left
		}
	}

	// There was either no exact match or there was an exact match but the
	// exact match flag was not set.  In any case, the parent stack might
	// need to be adjusted to only include the parents up to the selected
	// node.  Also, ensure the selected node's key does not exceed the
	// allowed range of the iterator.
	for i := iter.parents.Len(); i > selectedNodeDepth; i-- {
		iter.parents.Pop()
	}
	return iter.limitIterator()
}

// First moves the iterator to the first key/value pair.  When there is only a
// single key/value pair both First and Last will point to the same pair.
// Returns false if there are no key/value pairs.
func (iter *Iterator) First() bool {
	// Seek the start key if the iterator was created with one.  This will
	// result in either an exact match, the first greater key, or an
	// exhausted iterator if no such key exists.
	iter.isNew = false
	if iter.startKey != nil {
		return iter.seek(iter.startKey, true, true)
	}

	// The smallest key is in the left-most node.
	iter.parents = parentStack{}
	for node := iter.root; node != nil; node = node.left {
		if node.left == nil {
			iter.node = node
			return true
		}
		iter.parents.Push(node)
	}
	return false
}

// Last moves the iterator to the last key/value pair.  When there is only a
// single key/value pair both First and Last will point to the same pair.
// Returns false if there are no key/value pairs.
func (iter *Iterator) Last() bool {
	// Seek the limit key if the iterator was created with one.  This will
	// result in the first key smaller than the limit key, or an exhausted
	// iterator if no such key exists.
	iter.isNew = false
	if iter.limitKey != nil {
		return iter.seek(iter.limitKey, false, false)
	}

	// The highest key is in the right-most node.
	iter.parents = parentStack{}
	for node := iter.root; node != nil; node = node.right {
		if node.right == nil {
			iter.node = node
			return true
		}
		iter.parents.Push(node)
	}
	return false
}

// Next moves the iterator to the next key/value pair and returns false when the
// iterator is exhausted.  When invoked on a newly created iterator it will
// position the iterator at the first item.
func (iter *Iterator) Next() bool {
	if iter.isNew {
		return iter.First()
	}

	if iter.node == nil {
		return false
	}

	// Reseek the previous key without allowing for an exact match if a
	// force seek was requested.  This results in the key greater than the
	// previous one or an exhausted iterator if there is no such key.
	if seekKey := iter.seekKey; seekKey != nil {
		iter.seekKey = nil
		return iter.seek(seekKey, false, true)
	}

	// When there is no right node walk the parents until the parent's right
	// node is not equal to the previous child.  This will be the next node.
	if iter.node.right == nil {
		parent := iter.parents.Pop()
		for parent != nil && parent.right == iter.node {
			iter.node = parent
			parent = iter.parents.Pop()
		}
		iter.node = parent
		return iter.limitIterator()
	}

	// There is a right node, so the next node is the left-most node down
	// the right sub-tree.
	iter.parents.Push(iter.node)
	iter.node = iter.node.right
	for node := iter.node.left; node != nil; node = node.left {
		iter.parents.Push(iter.node)
		iter.node = node
	}
	return iter.limitIterator()
}

// Prev moves the iterator to the previous key/value pair and returns false when
// the iterator is exhausted.  When invoked on a newly created iterator it will
// position the iterator at the last item.
func (iter *Iterator) Prev() bool {
	if iter.isNew {
		return iter.Last()
	}

	if iter.node == nil {
		return false
	}

	// Reseek the previous key without allowing for an exact match if a
	// force seek was requested.  This results in the key smaller than the
	// previous one or an exhausted iterator if there is no such key.
	if seekKey := iter.seekKey; seekKey != nil {
		iter.seekKey = nil
		return iter.seek(seekKey, false, false)
	}

	// When there is no left node walk the parents until the parent's left
	// node is not equal to the previous child.  This will be the previous
	// node.
	for iter.node.left == nil {
		parent := iter.parents.Pop()
		for parent != nil && parent.left == iter.node {
			iter.node = parent
			parent = iter.parents.Pop()
		}
		iter.node = parent
		return iter.limitIterator()
	}

	// There is a left node, so the previous node is the right-most node
	// down the left sub-tree.
	iter.parents.Push(iter.node)
	iter.node = iter.node.left
	for node := iter.node.right; node != nil; node = node.right {
		iter.parents.Push(iter.node)
		iter.node = node
	}
	return iter.limitIterator()
}

// Seek moves the iterator to the first key/value pair with a key that is
// greater than or equal to the given key and returns true if successful.
func (iter *Iterator) Seek(key []byte) bool {
	iter.isNew = false
	return iter.seek(key, true, true)
}

// Key returns the key of the current key/value pair or nil when the iterator
// is exhausted.  The caller should not modify the contents of the returned
// slice.
func (iter *Iterator) Key() []byte {
	if iter.node == nil {
		return nil
	}
	return iter.node.key
}

// Value returns the value of the current key/value pair or nil when the
// iterator is exhausted.  The caller should not modify the contents of the
// returned slice.
func (iter *Iterator) Value() []byte {
	if iter.node == nil {
		return nil
	}
	return iter.node.value
}

// Valid indicates whether the iterator is positioned at a valid key/value pair.
// It will be considered invalid when the iterator is newly created or exhausted.
func (iter *Iterator) Valid() bool {
	return iter.node != nil
}

// ForceReseek notifies the iterator that the underlying mutable treap has been
// updated, so the next call to Prev or Next needs to reseek in order to allow
// the iterator to continue working properly.
//
// NOTE: Calling this function when the iterator is associated with an immutable
// treap has no effect as you would expect.
func (iter *Iterator) ForceReseek() {
	// Nothing to do when the iterator is associated with an immutable
	// treap.
	if iter.t == nil {
		return
	}

	// Update the iterator root to the mutable treap root in case it
	// changed.
	iter.root = iter.t.root

	// Set the seek key to the current node.  This will force the Next/Prev
	// functions to reseek, and thus properly reconstruct the iterator, on
	// their next call.
	if iter.node == nil {
		iter.seekKey = nil
		return
	}
	iter.seekKey = iter.node.key
}

// Iterator returns a new iterator for the mutable treap.  The newly returned
// iterator is not pointing to a valid item until a call to one of the methods
// to position it is made.
//
// The start key and limit key parameters cause the iterator to be limited to
// a range of keys.  The start key is inclusive and the limit key is exclusive.
// Either or both can be nil if the functionality is not desired.
//
// WARNING: The ForceSeek method must be called on the returned iterator if
// the treap is mutated.  Failure to do so will cause the iterator to return
// unexpected keys and/or values.
//
// For example:
//
//	iter := t.Iterator(nil, nil)
//	for iter.Next() {
//		if someCondition {
//			t.Delete(iter.Key())
//			iter.ForceReseek()
//		}
//	}
func (t *Mutable) Iterator(startKey, limitKey []byte) *Iterator {
	iter := &Iterator{
		t:        t,
		root:     t.root,
		isNew:    true,
		startKey: startKey,
		limitKey: limitKey,
	}
	return iter
}

// Iterator returns a new iterator for the immutable treap.  The newly returned
// iterator is not pointing to a valid item until a call to one of the methods
// to position it is made.
//
// The start key and limit key parameters cause the iterator to be limited to
// a range of keys.  The start key is inclusive and the limit key is exclusive.
// Either or both can be nil if the functionality is not desired.
func (t *Immutable) Iterator(startKey, limitKey []byte) *Iterator {
	iter := &Iterator{
		root:     t.root,
		isNew:    true,
		startKey: startKey,
		limitKey: limitKey,
	}
	return iter
}
