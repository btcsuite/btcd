// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package tickettreap

import (
	"fmt"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

const (
	// ptrSize is the number of bytes in a native pointer.
	//
	// nolint: vet
	ptrSize = 4 << (^uintptr(0) >> 63)

	// staticDepth is the size of the static array to use for keeping track
	// of the parent stack during treap iteration.  Since a treap has a very
	// high probability that the tree height is logarithmic, it is
	// exceedingly unlikely that the parent stack will ever exceed this size
	// even for extremely large numbers of items.
	staticDepth = 128

	// nodeFieldsSize is the size the fields of each node takes excluding
	// the contents of the value.
	// Size = 32 (key) + 8 (value pointer) + 4 (priority) + 4 (size)
	//      + 8 (left pointer) + 8 (right pointer).
	nodeFieldsSize = 32 + ptrSize + 4 + 4 + 2*ptrSize

	// nodeValueSize is the size of the fixed-size fields of a Value,
	// Height (4 bytes) plus (4*1 byte).
	nodeValueSize = 8
)

// Key defines the key used to add an associated value to the treap.
type Key chainhash.Hash

// Value defines the information stored for a given key in the treap.
type Value struct {
	// Height is the block height of the associated ticket.
	Height uint32

	// Flags defining the ticket state.
	Missed  bool
	Revoked bool
	Spent   bool
	Expired bool
}

// treapNode represents a node in the treap.
type treapNode struct {
	key      Key
	value    *Value
	priority uint32
	size     uint32 // Count of items within this treap - the node itself counts as 1.
	left     *treapNode
	right    *treapNode
}

// nodeSize returns the number of bytes the specified node occupies including
// the struct fields and the contents of the key and value.
func nodeSize(node *treapNode) uint64 {
	return nodeFieldsSize + uint64(len(node.key)+nodeValueSize)
}

// newTreapNode returns a new node from the given key, value, and priority.  The
// node is not initially linked to any others.
func newTreapNode(key Key, value *Value, priority uint32) *treapNode {
	return &treapNode{key: key, value: value, priority: priority, size: 1}
}

// leftSize returns the size of the subtree on the left-hand side, and zero if
// there is no tree present there.
func (t *treapNode) leftSize() uint32 {
	if t.left != nil {
		return t.left.size
	}
	return 0
}

// rightSize returns the size of the subtree on the right-hand side, and zero if
// there is no tree present there.
func (t *treapNode) rightSize() uint32 {
	if t.right != nil {
		return t.right.size
	}
	return 0
}

// getByIndex returns the (Key, *Value) at the given position and panics if idx is
// out of bounds.
func (t *treapNode) getByIndex(idx int) (Key, *Value) {
	if idx < 0 || idx > int(t.size) {
		panic(fmt.Sprintf("getByIndex(%v) index out of bounds", idx))
	}
	node := t
	for {
		if node.left == nil {
			if idx == 0 {
				return node.key, node.value
			} else {
				node, idx = node.right, idx-1
			}
		} else {
			if idx < int(node.left.size) {
				node = node.left
			} else if idx == int(node.left.size) {
				return node.key, node.value
			} else {
				node, idx = node.right, idx-int(node.left.size)-1
			}
		}
	}
}

// parentStack represents a stack of parent treap nodes that are used during
// iteration.  It consists of a static array for holding the parents and a
// dynamic overflow slice.  It is extremely unlikely the overflow will ever be
// hit during normal operation, however, since a treap's height is
// probabilistic, the overflow case needs to be handled properly.  This approach
// is used because it is much more efficient for the majority case than
// dynamically allocating heap space every time the treap is iterated.
type parentStack struct {
	index    int
	items    [staticDepth]*treapNode
	overflow []*treapNode
}

// Len returns the current number of items in the stack.
func (s *parentStack) Len() int {
	return s.index
}

// At returns the item n number of items from the top of the stack, where 0 is
// the topmost item, without removing it.  It returns nil if n exceeds the
// number of items on the stack.
func (s *parentStack) At(n int) *treapNode {
	index := s.index - n - 1
	if index < 0 {
		return nil
	}

	if index < staticDepth {
		return s.items[index]
	}

	return s.overflow[index-staticDepth]
}

// Pop removes the top item from the stack.  It returns nil if the stack is
// empty.
func (s *parentStack) Pop() *treapNode {
	if s.index == 0 {
		return nil
	}

	s.index--
	if s.index < staticDepth {
		node := s.items[s.index]
		s.items[s.index] = nil
		return node
	}

	node := s.overflow[s.index-staticDepth]
	s.overflow[s.index-staticDepth] = nil
	return node
}

// Push pushes the passed item onto the top of the stack.
func (s *parentStack) Push(node *treapNode) {
	if s.index < staticDepth {
		s.items[s.index] = node
		s.index++
		return
	}

	// This approach is used over append because reslicing the slice to pop
	// the item causes the compiler to make unneeded allocations.  Also,
	// since the max number of items is related to the tree depth which
	// requires expontentially more items to increase, only increase the cap
	// one item at a time.  This is more intelligent than the generic append
	// expansion algorithm which often doubles the cap.
	index := s.index - staticDepth
	if index+1 > cap(s.overflow) {
		overflow := make([]*treapNode, index+1)
		copy(overflow, s.overflow)
		s.overflow = overflow
	}
	s.overflow[index] = node
	s.index++
}
