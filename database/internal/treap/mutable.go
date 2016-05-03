// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package treap

import (
	"bytes"
	"math/rand"
)

// Mutable represents a treap data structure which is used to hold ordered
// key/value pairs using a combination of binary search tree and heap semantics.
// It is a self-organizing and randomized data structure that doesn't require
// complex operations to maintain balance.  Search, insert, and delete
// operations are all O(log n).
type Mutable struct {
	root  *treapNode
	count int

	// totalSize is the best estimate of the total size of of all data in
	// the treap including the keys, values, and node sizes.
	totalSize uint64
}

// Len returns the number of items stored in the treap.
func (t *Mutable) Len() int {
	return t.count
}

// Size returns a best estimate of the total number of bytes the treap is
// consuming including all of the fields used to represent the nodes as well as
// the size of the keys and values.  Shared values are not detected, so the
// returned size assumes each value is pointing to different memory.
func (t *Mutable) Size() uint64 {
	return t.totalSize
}

// get returns the treap node that contains the passed key and its parent.  When
// the found node is the root of the tree, the parent will be nil.  When the key
// does not exist, both the node and the parent will be nil.
func (t *Mutable) get(key []byte) (*treapNode, *treapNode) {
	var parent *treapNode
	for node := t.root; node != nil; {
		// Traverse left or right depending on the result of the
		// comparison.
		compareResult := bytes.Compare(key, node.key)
		if compareResult < 0 {
			parent = node
			node = node.left
			continue
		}
		if compareResult > 0 {
			parent = node
			node = node.right
			continue
		}

		// The key exists.
		return node, parent
	}

	// A nil node was reached which means the key does not exist.
	return nil, nil
}

// Has returns whether or not the passed key exists.
func (t *Mutable) Has(key []byte) bool {
	if node, _ := t.get(key); node != nil {
		return true
	}
	return false
}

// Get returns the value for the passed key.  The function will return nil when
// the key does not exist.
func (t *Mutable) Get(key []byte) []byte {
	if node, _ := t.get(key); node != nil {
		return node.value
	}
	return nil
}

// relinkGrandparent relinks the node into the treap after it has been rotated
// by changing the passed grandparent's left or right pointer, depending on
// where the old parent was, to point at the passed node.  Otherwise, when there
// is no grandparent, it means the node is now the root of the tree, so update
// it accordingly.
func (t *Mutable) relinkGrandparent(node, parent, grandparent *treapNode) {
	// The node is now the root of the tree when there is no grandparent.
	if grandparent == nil {
		t.root = node
		return
	}

	// Relink the grandparent's left or right pointer based on which side
	// the old parent was.
	if grandparent.left == parent {
		grandparent.left = node
	} else {
		grandparent.right = node
	}
}

// Put inserts the passed key/value pair.
func (t *Mutable) Put(key, value []byte) {
	// Use an empty byte slice for the value when none was provided.  This
	// ultimately allows key existence to be determined from the value since
	// an empty byte slice is distinguishable from nil.
	if value == nil {
		value = emptySlice
	}

	// The node is the root of the tree if there isn't already one.
	if t.root == nil {
		node := newTreapNode(key, value, rand.Int())
		t.count = 1
		t.totalSize = nodeSize(node)
		t.root = node
		return
	}

	// Find the binary tree insertion point and construct a list of parents
	// while doing so.  When the key matches an entry already in the treap,
	// just update its value and return.
	var parents parentStack
	var compareResult int
	for node := t.root; node != nil; {
		parents.Push(node)
		compareResult = bytes.Compare(key, node.key)
		if compareResult < 0 {
			node = node.left
			continue
		}
		if compareResult > 0 {
			node = node.right
			continue
		}

		// The key already exists, so update its value.
		t.totalSize -= uint64(len(node.value))
		t.totalSize += uint64(len(value))
		node.value = value
		return
	}

	// Link the new node into the binary tree in the correct position.
	node := newTreapNode(key, value, rand.Int())
	t.count++
	t.totalSize += nodeSize(node)
	parent := parents.At(0)
	if compareResult < 0 {
		parent.left = node
	} else {
		parent.right = node
	}

	// Perform any rotations needed to maintain the min-heap.
	for parents.Len() > 0 {
		// There is nothing left to do when the node's priority is
		// greater than or equal to its parent's priority.
		parent = parents.Pop()
		if node.priority >= parent.priority {
			break
		}

		// Perform a right rotation if the node is on the left side or
		// a left rotation if the node is on the right side.
		if parent.left == node {
			node.right, parent.left = parent, node.right
		} else {
			node.left, parent.right = parent, node.left
		}
		t.relinkGrandparent(node, parent, parents.At(0))
	}
}

// Delete removes the passed key if it exists.
func (t *Mutable) Delete(key []byte) {
	// Find the node for the key along with its parent.  There is nothing to
	// do if the key does not exist.
	node, parent := t.get(key)
	if node == nil {
		return
	}

	// When the only node in the tree is the root node and it is the one
	// being deleted, there is nothing else to do besides removing it.
	if parent == nil && node.left == nil && node.right == nil {
		t.root = nil
		t.count = 0
		t.totalSize = 0
		return
	}

	// Perform rotations to move the node to delete to a leaf position while
	// maintaining the min-heap.
	var isLeft bool
	var child *treapNode
	for node.left != nil || node.right != nil {
		// Choose the child with the higher priority.
		if node.left == nil {
			child = node.right
			isLeft = false
		} else if node.right == nil {
			child = node.left
			isLeft = true
		} else if node.left.priority >= node.right.priority {
			child = node.left
			isLeft = true
		} else {
			child = node.right
			isLeft = false
		}

		// Rotate left or right depending on which side the child node
		// is on.  This has the effect of moving the node to delete
		// towards the bottom of the tree while maintaining the
		// min-heap.
		if isLeft {
			child.right, node.left = node, child.right
		} else {
			child.left, node.right = node, child.left
		}
		t.relinkGrandparent(child, node, parent)

		// The parent for the node to delete is now what was previously
		// its child.
		parent = child
	}

	// Delete the node, which is now a leaf node, by disconnecting it from
	// its parent.
	if parent.right == node {
		parent.right = nil
	} else {
		parent.left = nil
	}
	t.count--
	t.totalSize -= nodeSize(node)
}

// ForEach invokes the passed function with every key/value pair in the treap
// in ascending order.
func (t *Mutable) ForEach(fn func(k, v []byte) bool) {
	// Add the root node and all children to the left of it to the list of
	// nodes to traverse and loop until they, and all of their child nodes,
	// have been traversed.
	var parents parentStack
	for node := t.root; node != nil; node = node.left {
		parents.Push(node)
	}
	for parents.Len() > 0 {
		node := parents.Pop()
		if !fn(node.key, node.value) {
			return
		}

		// Extend the nodes to traverse by all children to the left of
		// the current node's right child.
		for node := node.right; node != nil; node = node.left {
			parents.Push(node)
		}
	}
}

// Reset efficiently removes all items in the treap.
func (t *Mutable) Reset() {
	t.count = 0
	t.totalSize = 0
	t.root = nil
}

// NewMutable returns a new empty mutable treap ready for use.  See the
// documentation for the Mutable structure for more details.
func NewMutable() *Mutable {
	return &Mutable{}
}
