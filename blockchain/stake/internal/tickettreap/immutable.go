// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package tickettreap

import (
	"bytes"
	"sort"
)

// cloneTreapNode returns a shallow copy of the passed node.
func cloneTreapNode(node *treapNode) *treapNode {
	return &treapNode{
		key:      node.key,
		value:    node.value,
		priority: node.priority,
		size:     node.size,
		left:     node.left,
		right:    node.right,
	}
}

// Immutable represents a treap data structure which is used to hold ordered
// key/value pairs using a combination of binary search tree and heap semantics.
//
// In other parts of the code base, we see a traditional implementation of the
// treap with a randomised priority.
//
// Note that this treap has been modified from the original in that the
// priority, rather than being a random value, is deterministically assigned the
// monotonically increasing block height.
//
// However what is interesting is that we see similar behaviour to the treap
// structure, because the keys themselves are totally randomised, degenerate
// cases of trees with a bias for leftness or rightness cannot occur.
//
// Do make note however, if the keys were not to be randomised, this would not
// be a structure which can be relied upon to self-balance as treaps do.
//
// What motivated this alteration of the treap priority was that it can be used
// as a priority queue to discover the elements at the smallest height, which
// substantially improves application performance in one critical spot, via
// use of ForEachByHeight.
//
// All operations which result in modifying the treap return a new version of
// the treap with only the modified nodes updated.  All unmodified nodes are
// shared with the previous version.  This is extremely useful in concurrent
// applications since the caller only has to atomically replace the treap
// pointer with the newly returned version after performing any mutations.  All
// readers can simply use their existing pointer as a snapshot since the treap
// it points to is immutable.  This effectively provides O(1) snapshot
// capability with efficient memory usage characteristics since the old nodes
// only remain allocated until there are no longer any references to them.
type Immutable struct {
	root  *treapNode
	count int

	// totalSize is the best estimate of the total size of of all data in
	// the treap including the keys, values, and node sizes.
	totalSize uint64
}

// newImmutable returns a new immutable treap given the passed parameters.
func newImmutable(root *treapNode, count int, totalSize uint64) *Immutable {
	return &Immutable{root: root, count: count, totalSize: totalSize}
}

// Len returns the number of items stored in the treap.
func (t *Immutable) Len() int {
	return t.count
}

// Size returns a best estimate of the total number of bytes the treap is
// consuming including all of the fields used to represent the nodes as well as
// the size of the keys and values.  Shared values are not detected, so the
// returned size assumes each value is pointing to different memory.
func (t *Immutable) Size() uint64 {
	return t.totalSize
}

// get returns the treap node that contains the passed key.  It will return nil
// when the key does not exist.
func (t *Immutable) get(key Key) *treapNode {
	for node := t.root; node != nil; {
		// Traverse left or right depending on the result of the
		// comparison.
		compareResult := bytes.Compare(key[:], node.key[:])
		if compareResult < 0 {
			node = node.left
			continue
		}
		if compareResult > 0 {
			node = node.right
			continue
		}

		// The key exists.
		return node
	}

	// A nil node was reached which means the key does not exist.
	return nil
}

// Has returns whether or not the passed key exists.
func (t *Immutable) Has(key Key) bool {
	if node := t.get(key); node != nil {
		return true
	}
	return false
}

// Get returns the value for the passed key.  The function will return nil when
// the key does not exist.
func (t *Immutable) Get(key Key) *Value {
	if node := t.get(key); node != nil {
		return node.value
	}
	return nil
}

// GetByIndex returns the (Key, *Value) at the given position and panics if idx
// is out of bounds.
func (t *Immutable) GetByIndex(idx int) (Key, *Value) {
	return t.root.getByIndex(idx)
}

// Put inserts the passed key/value pair.  Passing a nil value will result in a
// NOOP.
func (t *Immutable) Put(key Key, value *Value) *Immutable {
	// Nothing to do if a nil value is passed.
	if value == nil {
		return t
	}

	// The node is the root of the tree if there isn't already one.
	if t.root == nil {
		root := newTreapNode(key, value, value.Height)
		return newImmutable(root, 1, nodeSize(root))
	}

	// Find the binary tree insertion point and construct a replaced list of
	// parents while doing so.  This is done because this is an immutable
	// data structure so regardless of where in the treap the new key/value
	// pair ends up, all ancestors up to and including the root need to be
	// replaced.
	//
	// When the key matches an entry already in the treap, replace the node
	// with a new one that has the new value set and return.
	var parents parentStack
	var compareResult int
	for node := t.root; node != nil; {
		// Clone the node and link its parent to it if needed.
		nodeCopy := cloneTreapNode(node)
		if oldParent := parents.At(0); oldParent != nil {
			if oldParent.left == node {
				oldParent.left = nodeCopy
			} else {
				oldParent.right = nodeCopy
			}
		}
		parents.Push(nodeCopy)

		// Traverse left or right depending on the result of comparing
		// the keys.
		compareResult = bytes.Compare(key[:], node.key[:])
		if compareResult < 0 {
			node = node.left
			continue
		}
		if compareResult > 0 {
			node = node.right
			continue
		}

		// The key already exists, so update its value.
		nodeCopy.value = value

		// Return new immutable treap with the replaced node and
		// ancestors up to and including the root of the tree.
		newRoot := parents.At(parents.Len() - 1)
		return newImmutable(newRoot, t.count, t.totalSize)
	}

	// Recompute the size member of all parents, to account for inserted item.
	node := newTreapNode(key, value, value.Height)
	for i := 0; i < parents.Len(); i++ {
		parents.At(i).size++
	}

	// Link the new node into the binary tree in the correct position.
	parent := parents.At(0)
	if compareResult < 0 {
		parent.left = node
	} else {
		parent.right = node
	}

	// Perform any rotations needed to maintain the min-heap and replace
	// the ancestors up to and including the tree root.
	newRoot := parents.At(parents.Len() - 1)
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
			// just to help visualise right-rotation...
			//        p               n
			//       / \       ->    / \
			//      n  p.r         n.l  p
			//     / \                 / \
			//   n.l n.r             n.r p.r
			node.size += 1 + parent.rightSize()
			parent.size -= 1 + node.leftSize()
			node.right, parent.left = parent, node.right
		} else {
			node.size += 1 + parent.leftSize()
			parent.size -= 1 + node.rightSize()
			node.left, parent.right = parent, node.left
		}

		// Either set the new root of the tree when there is no
		// grandparent or relink the grandparent to the node based on
		// which side the old parent the node is replacing was on.
		grandparent := parents.At(0)
		if grandparent == nil {
			newRoot = node
		} else if grandparent.left == parent {
			grandparent.left = node
		} else {
			grandparent.right = node
		}
	}

	return newImmutable(newRoot, t.count+1, t.totalSize+nodeSize(node))
}

// Delete removes the passed key from the treap and returns the resulting treap
// if it exists.  The original immutable treap is returned if the key does not
// exist.
func (t *Immutable) Delete(key Key) *Immutable {
	// Find the node for the key while constructing a list of parents while
	// doing so.
	var parents parentStack
	var delNode *treapNode
	for node := t.root; node != nil; {
		parents.Push(node)

		// Traverse left or right depending on the result of the
		// comparison.
		compareResult := bytes.Compare(key[:], node.key[:])
		if compareResult < 0 {
			node = node.left
			continue
		}
		if compareResult > 0 {
			node = node.right
			continue
		}

		// The key exists.
		delNode = node
		break
	}

	// There is nothing to do if the key does not exist.
	if delNode == nil {
		return t
	}

	// When the only node in the tree is the root node and it is the one
	// being deleted, there is nothing else to do besides removing it.
	parent := parents.At(1)
	if parent == nil && delNode.left == nil && delNode.right == nil {
		return newImmutable(nil, 0, 0)
	}

	// Construct a replaced list of parents and the node to delete itself.
	// This is done because this is an immutable data structure and
	// therefore all ancestors of the node that will be deleted, up to and
	// including the root, need to be replaced.
	var newParents parentStack
	for i := parents.Len(); i > 0; i-- {
		node := parents.At(i - 1)
		nodeCopy := cloneTreapNode(node)
		nodeCopy.size--
		if oldParent := newParents.At(0); oldParent != nil {
			if oldParent.left == node {
				oldParent.left = nodeCopy
			} else {
				oldParent.right = nodeCopy
			}
		}
		newParents.Push(nodeCopy)
	}
	delNode = newParents.Pop()
	parent = newParents.At(0)

	// Perform rotations to move the node to delete to a leaf position while
	// maintaining the min-heap while replacing the modified children.
	var child *treapNode
	newRoot := newParents.At(newParents.Len() - 1)
	for delNode.left != nil || delNode.right != nil {
		// Choose the child with the higher priority.
		var isLeft bool
		if delNode.left == nil {
			child = delNode.right
		} else if delNode.right == nil {
			child = delNode.left
			isLeft = true
		} else if delNode.left.priority <= delNode.right.priority {
			child = delNode.left
			isLeft = true
		} else {
			child = delNode.right
		}

		// Rotate left or right depending on which side the child node
		// is on.  This has the effect of moving the node to delete
		// towards the bottom of the tree while maintaining the
		// min-heap.
		child = cloneTreapNode(child)
		if isLeft {
			child.size += delNode.rightSize()
			child.right, delNode.left = delNode, child.right
		} else {
			child.size += delNode.leftSize()
			child.left, delNode.right = delNode, child.left
		}

		// Either set the new root of the tree when there is no
		// grandparent or relink the grandparent to the node based on
		// which side the old parent the node is replacing was on.
		//
		// Since the node to be deleted was just moved down a level, the
		// new grandparent is now the current parent and the new parent
		// is the current child.
		if parent == nil {
			newRoot = child
		} else if parent.left == delNode {
			parent.left = child
		} else {
			parent.right = child
		}

		// The parent for the node to delete is now what was previously
		// its child.
		parent = child
	}

	// Delete the node, which is now a leaf node, by disconnecting it from
	// its parent.
	if parent.right == delNode {
		parent.right = nil
	} else {
		parent.left = nil
	}

	return newImmutable(newRoot, t.count-1, t.totalSize-nodeSize(delNode))
}

// ForEach invokes the passed function with every key/value pair in the treap
// in ascending order.
func (t *Immutable) ForEach(fn func(k Key, v *Value) bool) {
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

// ForEachByHeight iterates all elements in the tree less than a given height in
// the blockchain.
func (t *Immutable) ForEachByHeight(heightLessThan uint32, fn func(k Key, v *Value) bool) {
	// Add the root node and all children to the left of it to the list of
	// nodes to traverse and loop until they, and all of their child nodes,
	// have been traversed.
	var parents parentStack
	for node := t.root; node != nil && node.priority < heightLessThan; node = node.left {
		parents.Push(node)
	}
	for parents.Len() > 0 {
		node := parents.Pop()
		if !fn(node.key, node.value) {
			return
		}

		// Extend the nodes to traverse by all children to the left of
		// the current node's right child.
		for node := node.right; node != nil && node.priority < heightLessThan; node = node.left {
			parents.Push(node)
		}
	}
}

// FetchWinnersAndExpired is a ticket database specific function which iterates
// over the entire treap and finds winners at selected indexes and all tickets
// whose height is less than or equal to the passed height. These are returned
// as slices of pointers to keys, which can be recast as []*chainhash.Hash.
// This is only used for benchmarking and is not consensus compatible.
func (t *Immutable) FetchWinnersAndExpired(idxs []int, height uint32) ([]*Key, []*Key) {
	if idxs == nil {
		return nil, nil
	}

	sortedIdxs := sort.IntSlice(idxs)
	sort.Sort(sortedIdxs)

	// TODO buffer winners according to the TicketsPerBlock value from
	// chaincfg?
	idx := 0
	var winners []*Key
	var expired []*Key
	winnerIdx := 0
	t.ForEach(func(k Key, v *Value) bool {
		if v.Height <= height {
			expired = append(expired, &k)
		}
		if idx == sortedIdxs[winnerIdx] {
			winners = append(winners, &k)
			if winnerIdx+1 < len(sortedIdxs) {
				winnerIdx++
			}
		}

		idx++
		return true
	})

	return winners, expired
}

// NewImmutable returns a new empty immutable treap ready for use.  See the
// documentation for the Immutable structure for more details.
func NewImmutable() *Immutable {
	return &Immutable{}
}
