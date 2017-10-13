// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package treap

import (
	"encoding/binary"
	"math/rand"
	"reflect"
	"testing"
)

// serializeUint32 returns the big-endian encoding of the passed uint32.
func serializeUint32(ui uint32) []byte {
	var ret [4]byte
	binary.BigEndian.PutUint32(ret[:], ui)
	return ret[:]
}

// TestParentStack ensures the treapParentStack functionality works as intended.
func TestParentStack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		numNodes int
	}{
		{numNodes: 1},
		{numNodes: staticDepth},
		{numNodes: staticDepth + 1}, // Test dynamic code paths
	}

testLoop:
	for i, test := range tests {
		nodes := make([]*treapNode, 0, test.numNodes)
		for j := 0; j < test.numNodes; j++ {
			var key [4]byte
			binary.BigEndian.PutUint32(key[:], uint32(j))
			node := newTreapNode(key[:], key[:], 0)
			nodes = append(nodes, node)
		}

		// Push all of the nodes onto the parent stack while testing
		// various stack properties.
		stack := &parentStack{}
		for j, node := range nodes {
			stack.Push(node)

			// Ensure the stack length is the expected value.
			if stack.Len() != j+1 {
				t.Errorf("Len #%d (%d): unexpected stack "+
					"length - got %d, want %d", i, j,
					stack.Len(), j+1)
				continue testLoop
			}

			// Ensure the node at each index is the expected one.
			for k := 0; k <= j; k++ {
				atNode := stack.At(j - k)
				if !reflect.DeepEqual(atNode, nodes[k]) {
					t.Errorf("At #%d (%d): mismatched node "+
						"- got %v, want %v", i, j-k,
						atNode, nodes[k])
					continue testLoop
				}
			}
		}

		// Ensure each popped node is the expected one.
		for j := 0; j < len(nodes); j++ {
			node := stack.Pop()
			expected := nodes[len(nodes)-j-1]
			if !reflect.DeepEqual(node, expected) {
				t.Errorf("At #%d (%d): mismatched node - "+
					"got %v, want %v", i, j, node, expected)
				continue testLoop
			}
		}

		// Ensure the stack is now empty.
		if stack.Len() != 0 {
			t.Errorf("Len #%d: stack is not empty - got %d", i,
				stack.Len())
			continue testLoop
		}

		// Ensure attempting to retrieve a node at an index beyond the
		// stack's length returns nil.
		if node := stack.At(2); node != nil {
			t.Errorf("At #%d: did not give back nil - got %v", i,
				node)
			continue testLoop
		}

		// Ensure attempting to pop a node from an empty stack returns
		// nil.
		if node := stack.Pop(); node != nil {
			t.Errorf("Pop #%d: did not give back nil - got %v", i,
				node)
			continue testLoop
		}
	}
}

func init() {
	// Force the same pseudo random numbers for each test run.
	rand.Seed(0)
}
