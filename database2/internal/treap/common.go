// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package treap

import (
	"math/rand"
	"time"
)

const (
	// staticDepth is the size of the static array to use for keeping track
	// of the parent stack during treap iteration.  Since a treap has a very
	// high probability that the tree height is logarithmic, it is
	// exceedingly unlikely that the parent stack will ever exceed this size
	// even for extremely large numbers of items.
	staticDepth = 128

	// nodeFieldsSize is the size the fields of each node takes excluding
	// the contents of the key and value.  It assumes 64-bit pointers so
	// technically it is smaller on 32-bit platforms, but overestimating the
	// size in that case is acceptable since it avoids the need to import
	// unsafe.  It consists of 24-bytes for each key and value + 8 bytes for
	// each of the priority, left, and right fields (24*2 + 8*3).
	nodeFieldsSize = 72
)

var (
	// emptySlice is used for keys that have no value associated with them
	// so callers can distinguish between a key that does not exist and one
	// that has no value associated with it.
	emptySlice = make([]byte, 0)
)

// treapNode represents a node in the treap.
type treapNode struct {
	key      []byte
	value    []byte
	priority int
	left     *treapNode
	right    *treapNode
}

// nodeSize returns the number of bytes the specified node occupies including
// the struct fields and the contents of the key and value.
func nodeSize(node *treapNode) uint64 {
	return nodeFieldsSize + uint64(len(node.key)+len(node.value))
}

// newTreapNode returns a new node from the given key, value, and priority.  The
// node is not initially linked to any others.
func newTreapNode(key, value []byte, priority int) *treapNode {
	return &treapNode{key: key, value: value, priority: priority}
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

func init() {
	rand.Seed(time.Now().UnixNano())
}
