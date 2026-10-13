// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package treap

import (
	"bytes"
	"math/rand"
)

// cloneTreapNode returns a shallow copy of the passed node.
func cloneTreapNode(node *treapNode) *treapNode {
	n := treapNodePool.Get().(*treapNode)
	n.key = node.key
	n.value = node.value
	n.priority = node.priority
	n.left = node.left
	n.right = node.right

	return n
}

// Immutable represents a treap data structure which is used to hold ordered
// key/value pairs using a combination of binary search tree and heap semantics.
// It is a self-organizing and randomized data structure that doesn't require
// complex operations to maintain balance.  Search, insert, and delete
// operations are all O(log n).  In addition, it provides O(1) snapshots for
// multi-version concurrency control (MVCC).
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
func (t *Immutable) get(key []byte) *treapNode {
	for node := t.root; node != nil; {
		// Traverse left or right depending on the result of the
		// comparison.
		compareResult := bytes.Compare(key, node.key)
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
func (t *Immutable) Has(key []byte) bool {
	if node := t.get(key); node != nil {
		return true
	}
	return false
}

// Get returns the value for the passed key.  The function will return nil when
// the key does not exist.
func (t *Immutable) Get(key []byte) []byte {
	if node := t.get(key); node != nil {
		return node.value
	}
	return nil
}

// KVPair is a helper struct for a key-value pair that's going to be inserted
// into the treap.
type KVPair struct {
	Key   []byte
	Value []byte
}

// nodeInSet returns true if the given node pointer exists in the set array.
// The set is terminated by the first nil entry.
func nodeInSet(node *treapNode, set *[staticDepth]*treapNode) bool {
	for _, n := range set {
		if n == nil {
			return false
		}
		if n == node {
			return true
		}
	}
	return false
}

// Put inserts the passed key/value pairs into the treap.  For operations
// requiring many insertions at once, Put is memory efficient as the
// intermediary treap nodes created between each put operation are recycled
// through an internal sync.Pool, reducing overall memory allocation.
//
// MVCC Snapshot Safety: The recycling of intermediate nodes is safe with
// respect to concurrent snapshot readers because:
//
//  1. Each internal put() call creates FRESH nodes via cloneTreapNode (pool
//     allocation) that are distinct from any node in a pre-existing treap
//     version or snapshot.
//  2. Snapshots hold references to the ORIGINAL treap's root, whose nodes
//     are never recycled here — only their clones are candidates.
//  3. A cloned node from a previous put() iteration is recycled only if it
//     was re-cloned by the subsequent put() (verified via pointer identity
//     in the replaced source set), confirming the new treap no longer
//     references it.
//  4. Cloned nodes NOT re-cloned survive in the new treap via structural
//     sharing and are never recycled.
func (t *Immutable) Put(kvPairs ...KVPair) *Immutable {
	if len(kvPairs) == 0 {
		return t
	}

	treap := t
	var prevCreated [staticDepth]*treapNode

	for _, kvPair := range kvPairs {
		newTreap, created, replaced := treap.put(
			kvPair.Key, kvPair.Value,
		)

		// Recycle nodes from the previous put() that were re-cloned
		// by the current put().  A node is safe to recycle when it
		// appears in the replaced source set, meaning the new treap
		// has a fresh clone in its place and no longer references the
		// previous version.  This is O(depth^2) pointer comparisons
		// which is far cheaper than the previous O(depth * log n)
		// approach using get() with key comparisons.
		for _, node := range prevCreated {
			if node == nil {
				break
			}

			if nodeInSet(node, &replaced) {
				node.recycle()
			}
		}

		treap = newTreap
		prevCreated = created
	}

	return treap
}

// put inserts the passed key/value pair and returns the new treap along with
// two node arrays: created contains all newly allocated nodes (clones of
// ancestors plus the new leaf), and replaced contains the original source
// nodes that were cloned.  The caller can compare these arrays across
// sequential put() calls to determine which intermediate nodes are no longer
// live and can be safely returned to the pool.
func (t *Immutable) put(key, value []byte) (
	*Immutable, [staticDepth]*treapNode, [staticDepth]*treapNode,
) {

	// Use an empty byte slice for the value when none was provided.  This
	// ultimately allows key existence to be determined from the value since
	// an empty byte slice is distinguishable from nil.
	if value == nil {
		value = emptySlice
	}

	var (
		created     [staticDepth]*treapNode
		replaced    [staticDepth]*treapNode
		createdIdx  int
		replacedIdx int
	)

	// The node is the root of the tree if there isn't already one.
	if t.root == nil {
		root := newTreapNode(key, value, rand.Int())
		created[0] = root
		return newImmutable(root, 1, nodeSize(root)),
			created, replaced
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

		// Track the new clone and the original source it replaced.
		if createdIdx < staticDepth {
			created[createdIdx] = nodeCopy
			createdIdx++
		}
		if replacedIdx < staticDepth {
			replaced[replacedIdx] = node
			replacedIdx++
		}

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
		nodeCopy.value = value

		// Return new immutable treap with the replaced node and
		// ancestors up to and including the root of the tree.
		newRoot := parents.At(parents.Len() - 1)
		newTotalSize := t.totalSize - uint64(len(node.value)) +
			uint64(len(value))
		return newImmutable(newRoot, t.count, newTotalSize),
			created, replaced
	}

	// Create the new leaf node.  There is no replaced source for a brand
	// new node — only clones of existing nodes have sources.
	node := newTreapNode(key, value, rand.Int())
	if createdIdx < staticDepth {
		created[createdIdx] = node
		createdIdx++
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
			node.right, parent.left = parent, node.right
		} else {
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

	return newImmutable(newRoot, t.count+1, t.totalSize+nodeSize(node)),
		created, replaced
}

// Delete removes the passed keys from the treap and returns the resulting
// treap.  Keys that do not exist are silently ignored.  For operations
// requiring many deletions at once, Delete is memory efficient as the
// intermediary treap nodes created between each delete operation are recycled
// through an internal sync.Pool, reducing overall memory allocation.
//
// The same MVCC snapshot safety guarantees as Put apply here — see Put for
// the full safety argument.
func (t *Immutable) Delete(keys ...[]byte) *Immutable {
	if len(keys) == 0 {
		return t
	}

	treap := t
	var prevCreated [staticDepth]*treapNode

	for _, key := range keys {
		newTreap, created, replacedSrc := treap.delete(key)

		// Only attempt recycling when the treap was actually modified.
		// When the key was not found, delete() returns the same treap
		// pointer and empty arrays, so prevCreated must be preserved
		// for the next iteration that does modify the treap.
		if newTreap != treap {
			for _, node := range prevCreated {
				if node == nil {
					break
				}

				if nodeInSet(node, &replacedSrc) {
					node.recycle()
				}
			}

			prevCreated = created
		}

		treap = newTreap
	}

	return treap
}

// delete removes the passed key from the treap and returns the new treap along
// with two node arrays following the same semantics as put(): created contains
// all newly allocated clones, replaced contains the original source nodes.  If
// the key does not exist, the original treap is returned with empty arrays.
func (t *Immutable) delete(key []byte) (
	*Immutable, [staticDepth]*treapNode, [staticDepth]*treapNode,
) {

	var (
		created     [staticDepth]*treapNode
		replaced    [staticDepth]*treapNode
		createdIdx  int
		replacedIdx int
	)

	// Find the node for the key while constructing a list of parents while
	// doing so.
	var parents parentStack
	var delNode *treapNode
	for node := t.root; node != nil; {
		parents.Push(node)

		// Traverse left or right depending on the result of the
		// comparison.
		compareResult := bytes.Compare(key, node.key)
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
		return t, created, replaced
	}

	// When the only node in the tree is the root node and it is the one
	// being deleted, there is nothing else to do besides removing it.
	parent := parents.At(1)
	if parent == nil && delNode.left == nil && delNode.right == nil {
		return newImmutable(nil, 0, 0), created, replaced
	}

	// Construct a replaced list of parents and the node to delete itself.
	// This is done because this is an immutable data structure and
	// therefore all ancestors of the node that will be deleted, up to and
	// including the root, need to be replaced.
	var newParents parentStack
	for i := parents.Len(); i > 0; i-- {
		node := parents.At(i - 1)
		nodeCopy := cloneTreapNode(node)

		// Track the clone and its source.
		if createdIdx < staticDepth {
			created[createdIdx] = nodeCopy
			createdIdx++
		}
		if replacedIdx < staticDepth {
			replaced[replacedIdx] = node
			replacedIdx++
		}

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
		} else if delNode.left.priority >= delNode.right.priority {
			child = delNode.left
			isLeft = true
		} else {
			child = delNode.right
		}

		// Track the source before cloning.
		if replacedIdx < staticDepth {
			replaced[replacedIdx] = child
			replacedIdx++
		}

		// Rotate left or right depending on which side the child node
		// is on.  This has the effect of moving the node to delete
		// towards the bottom of the tree while maintaining the
		// min-heap.
		child = cloneTreapNode(child)

		// Track the newly created clone.
		if createdIdx < staticDepth {
			created[createdIdx] = child
			createdIdx++
		}

		if isLeft {
			child.right, delNode.left = delNode, child.right
		} else {
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

	return newImmutable(newRoot, t.count-1, t.totalSize-nodeSize(delNode)),
		created, replaced
}

// ForEach invokes the passed function with every key/value pair in the treap
// in ascending order.
func (t *Immutable) ForEach(fn func(k, v []byte) bool) {
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

// NewImmutable returns a new empty immutable treap ready for use.  See the
// documentation for the Immutable structure for more details.
func NewImmutable() *Immutable {
	return &Immutable{}
}
