// Copyright (c) 2023 The utreexo developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math/rand"
	"testing"
)

func TestAncestor(t *testing.T) {
	height := 500_000
	blockNodes := chainedNodes(nil, height)

	for i, blockNode := range blockNodes {
		// Grab a random node that's a child of this node
		// and try to fetch the current blockNode with Ancestor.
		randNode := blockNodes[rand.Intn(height-i)+i]
		got := randNode.Ancestor(blockNode.height)

		// See if we got the right one.
		if got.hash != blockNode.hash {
			t.Fatalf("expected ancestor at height %d "+
				"but got a node at height %d",
				blockNode.height, got.height)
		}

		// Gensis doesn't have ancestors so skip the check below.
		if blockNode.height == 0 {
			continue
		}

		// The ancestors are deterministic so check that this node's
		// ancestor is the correct one.
		if blockNode.ancestor.height != getAncestorHeight(blockNode.height) {
			t.Fatalf("expected anestor at height %d, but it was at %d",
				getAncestorHeight(blockNode.height),
				blockNode.ancestor.height)
		}
	}
}
