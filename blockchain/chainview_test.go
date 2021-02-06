// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/wire"
)

// testNoncePrng provides a deterministic prng for the nonce in generated fake
// nodes.  The ensures that the node have unique hashes.
var testNoncePrng = rand.New(rand.NewSource(0))

// chainedNodes returns the specified number of nodes constructed such that each
// subsequent node points to the previous one to create a chain.  The first node
// will point to the passed parent which can be nil if desired.
func chainedNodes(parent *blockNode, numNodes int) []*blockNode {
	nodes := make([]*blockNode, numNodes)
	tip := parent
	for i := 0; i < numNodes; i++ {
		// This is invalid, but all that is needed is enough to get the
		// synthetic tests to work.
		header := wire.BlockHeader{Nonce: testNoncePrng.Uint32()}
		if tip != nil {
			header.PrevBlock = tip.hash
		}
		nodes[i] = newBlockNode(&header, tip)
		tip = nodes[i]
	}
	return nodes
}

// String returns the block node as a human-readable name.
func (node blockNode) String() string {
	return fmt.Sprintf("%s(%d)", node.hash, node.height)
}

// tstTip is a convenience function to grab the tip of a chain of block nodes
// created via chainedNodes.
func tstTip(nodes []*blockNode) *blockNode {
	return nodes[len(nodes)-1]
}

// locatorHashes is a convenience function that returns the hashes for all of
// the passed indexes of the provided nodes.  It is used to construct expected
// block locators in the tests.
func locatorHashes(nodes []*blockNode, indexes ...int) BlockLocator {
	hashes := make(BlockLocator, 0, len(indexes))
	for _, idx := range indexes {
		hashes = append(hashes, &nodes[idx].hash)
	}
	return hashes
}

// zipLocators is a convenience function that returns a single block locator
// given a variable number of them and is used in the tests.
func zipLocators(locators ...BlockLocator) BlockLocator {
	var hashes BlockLocator
	for _, locator := range locators {
		hashes = append(hashes, locator...)
	}
	return hashes
}

// TestChainView ensures all of the exported functionality of chain views works
// as intended with the exception of some special cases which are handled in
// other tests.
func TestChainView(t *testing.T) {
	// Construct a synthetic block index consisting of the following
	// structure.
	// 0 -> 1 -> 2  -> 3  -> 4
	//       \-> 2a -> 3a -> 4a  -> 5a -> 6a -> 7a -> ... -> 26a
	//             \-> 3a'-> 4a' -> 5a'
	branch0Nodes := chainedNodes(nil, 5)
	branch1Nodes := chainedNodes(branch0Nodes[1], 25)
	branch2Nodes := chainedNodes(branch1Nodes[0], 3)

	tip := tstTip
	tests := []struct {
		name       string
		view       *chainView   // active view
		genesis    *blockNode   // expected genesis block of active view
		tip        *blockNode   // expected tip of active view
		side       *chainView   // side chain view
		sideTip    *blockNode   // expected tip of side chain view
		fork       *blockNode   // expected fork node
		contains   []*blockNode // expected nodes in active view
		noContains []*blockNode // expected nodes NOT in active view
		equal      *chainView   // view expected equal to active view
		unequal    *chainView   // view expected NOT equal to active
		locator    BlockLocator // expected locator for active view tip
	}{
		{
			// Create a view for branch 0 as the active chain and
			// another view for branch 1 as the side chain.
			name:       "chain0-chain1",
			view:       newChainView(tip(branch0Nodes)),
			genesis:    branch0Nodes[0],
			tip:        tip(branch0Nodes),
			side:       newChainView(tip(branch1Nodes)),
			sideTip:    tip(branch1Nodes),
			fork:       branch0Nodes[1],
			contains:   branch0Nodes,
			noContains: branch1Nodes,
			equal:      newChainView(tip(branch0Nodes)),
			unequal:    newChainView(tip(branch1Nodes)),
			locator:    locatorHashes(branch0Nodes, 4, 3, 2, 1, 0),
		},
		{
			// Create a view for branch 1 as the active chain and
			// another view for branch 2 as the side chain.
			name:       "chain1-chain2",
			view:       newChainView(tip(branch1Nodes)),
			genesis:    branch0Nodes[0],
			tip:        tip(branch1Nodes),
			side:       newChainView(tip(branch2Nodes)),
			sideTip:    tip(branch2Nodes),
			fork:       branch1Nodes[0],
			contains:   branch1Nodes,
			noContains: branch2Nodes,
			equal:      newChainView(tip(branch1Nodes)),
			unequal:    newChainView(tip(branch2Nodes)),
			locator: zipLocators(
				locatorHashes(branch1Nodes, 24, 23, 22, 21, 20,
					19, 18, 17, 16, 15, 14, 13, 11, 7),
				locatorHashes(branch0Nodes, 1, 0)),
		},
		{
			// Create a view for branch 2 as the active chain and
			// another view for branch 0 as the side chain.
			name:       "chain2-chain0",
			view:       newChainView(tip(branch2Nodes)),
			genesis:    branch0Nodes[0],
			tip:        tip(branch2Nodes),
			side:       newChainView(tip(branch0Nodes)),
			sideTip:    tip(branch0Nodes),
			fork:       branch0Nodes[1],
			contains:   branch2Nodes,
			noContains: branch0Nodes[2:],
			equal:      newChainView(tip(branch2Nodes)),
			unequal:    newChainView(tip(branch0Nodes)),
			locator: zipLocators(
				locatorHashes(branch2Nodes, 2, 1, 0),
				locatorHashes(branch1Nodes, 0),
				locatorHashes(branch0Nodes, 1, 0)),
		},
	}
testLoop:
	for _, test := range tests {
		// Ensure the active and side chain heights are the expected
		// values.
		if test.view.Height() != test.tip.height {
			t.Errorf("%s: unexpected active view height -- got "+
				"%d, want %d", test.name, test.view.Height(),
				test.tip.height)
			continue
		}
		if test.side.Height() != test.sideTip.height {
			t.Errorf("%s: unexpected side view height -- got %d, "+
				"want %d", test.name, test.side.Height(),
				test.sideTip.height)
			continue
		}

		// Ensure the active and side chain genesis block is the
		// expected value.
		if test.view.Genesis() != test.genesis {
			t.Errorf("%s: unexpected active view genesis -- got "+
				"%v, want %v", test.name, test.view.Genesis(),
				test.genesis)
			continue
		}
		if test.side.Genesis() != test.genesis {
			t.Errorf("%s: unexpected side view genesis -- got %v, "+
				"want %v", test.name, test.view.Genesis(),
				test.genesis)
			continue
		}

		// Ensure the active and side chain tips are the expected nodes.
		if test.view.Tip() != test.tip {
			t.Errorf("%s: unexpected active view tip -- got %v, "+
				"want %v", test.name, test.view.Tip(), test.tip)
			continue
		}
		if test.side.Tip() != test.sideTip {
			t.Errorf("%s: unexpected active view tip -- got %v, "+
				"want %v", test.name, test.side.Tip(),
				test.sideTip)
			continue
		}

		// Ensure that regardless of the order the two chains are
		// compared they both return the expected fork point.
		forkNode := test.view.FindFork(test.side.Tip())
		if forkNode != test.fork {
			t.Errorf("%s: unexpected fork node (view, side) -- "+
				"got %v, want %v", test.name, forkNode,
				test.fork)
			continue
		}
		forkNode = test.side.FindFork(test.view.Tip())
		if forkNode != test.fork {
			t.Errorf("%s: unexpected fork node (side, view) -- "+
				"got %v, want %v", test.name, forkNode,
				test.fork)
			continue
		}

		// Ensure that the fork point for a node that is already part
		// of the chain view is the node itself.
		forkNode = test.view.FindFork(test.view.Tip())
		if forkNode != test.view.Tip() {
			t.Errorf("%s: unexpected fork node (view, tip) -- "+
				"got %v, want %v", test.name, forkNode,
				test.view.Tip())
			continue
		}

		// Ensure all expected nodes are contained in the active view.
		for _, node := range test.contains {
			if !test.view.Contains(node) {
				t.Errorf("%s: expected %v in active view",
					test.name, node)
				continue testLoop
			}
		}

		// Ensure all nodes from side chain view are NOT contained in
		// the active view.
		for _, node := range test.noContains {
			if test.view.Contains(node) {
				t.Errorf("%s: unexpected %v in active view",
					test.name, node)
				continue testLoop
			}
		}

		// Ensure equality of different views into the same chain works
		// as intended.
		if !test.view.Equals(test.equal) {
			t.Errorf("%s: unexpected unequal views", test.name)
			continue
		}
		if test.view.Equals(test.unequal) {
			t.Errorf("%s: unexpected equal views", test.name)
			continue
		}

		// Ensure all nodes contained in the view return the expected
		// next node.
		for i, node := range test.contains {
			// Final node expects nil for the next node.
			var expected *blockNode
			if i < len(test.contains)-1 {
				expected = test.contains[i+1]
			}
			if next := test.view.Next(node); next != expected {
				t.Errorf("%s: unexpected next node -- got %v, "+
					"want %v", test.name, next, expected)
				continue testLoop
			}
		}

		// Ensure nodes that are not contained in the view do not
		// produce a successor node.
		for _, node := range test.noContains {
			if next := test.view.Next(node); next != nil {
				t.Errorf("%s: unexpected next node -- got %v, "+
					"want nil", test.name, next)
				continue testLoop
			}
		}

		// Ensure all nodes contained in the view can be retrieved by
		// height.
		for _, wantNode := range test.contains {
			node := test.view.NodeByHeight(wantNode.height)
			if node != wantNode {
				t.Errorf("%s: unexpected node for height %d -- "+
					"got %v, want %v", test.name,
					wantNode.height, node, wantNode)
				continue testLoop
			}
		}

		// Ensure the block locator for the tip of the active view
		// consists of the expected hashes.
		locator := test.view.BlockLocator(test.view.tip())
		if !reflect.DeepEqual(locator, test.locator) {
			t.Errorf("%s: unexpected locator -- got %v, want %v",
				test.name, locator, test.locator)
			continue
		}
	}
}

// TestChainViewForkCorners ensures that finding the fork between two chains
// works in some corner cases such as when the two chains have completely
// unrelated histories.
func TestChainViewForkCorners(t *testing.T) {
	// Construct two unrelated single branch synthetic block indexes.
	branchNodes := chainedNodes(nil, 5)
	unrelatedBranchNodes := chainedNodes(nil, 7)

	// Create chain views for the two unrelated histories.
	view1 := newChainView(tstTip(branchNodes))
	view2 := newChainView(tstTip(unrelatedBranchNodes))

	// Ensure attempting to find a fork point with a node that doesn't exist
	// doesn't produce a node.
	if fork := view1.FindFork(nil); fork != nil {
		t.Fatalf("FindFork: unexpected fork -- got %v, want nil", fork)
	}

	// Ensure attempting to find a fork point in two chain views with
	// totally unrelated histories doesn't produce a node.
	for _, node := range branchNodes {
		if fork := view2.FindFork(node); fork != nil {
			t.Fatalf("FindFork: unexpected fork -- got %v, want nil",
				fork)
		}
	}
	for _, node := range unrelatedBranchNodes {
		if fork := view1.FindFork(node); fork != nil {
			t.Fatalf("FindFork: unexpected fork -- got %v, want nil",
				fork)
		}
	}
}

// TestChainViewSetTip ensures changing the tip works as intended including
// capacity changes.
func TestChainViewSetTip(t *testing.T) {
	// Construct a synthetic block index consisting of the following
	// structure.
	// 0 -> 1 -> 2  -> 3  -> 4
	//       \-> 2a -> 3a -> 4a  -> 5a -> 6a -> 7a -> ... -> 26a
	branch0Nodes := chainedNodes(nil, 5)
	branch1Nodes := chainedNodes(branch0Nodes[1], 25)

	tip := tstTip
	tests := []struct {
		name     string
		view     *chainView     // active view
		tips     []*blockNode   // tips to set
		contains [][]*blockNode // expected nodes in view for each tip
	}{
		{
			// Create an empty view and set the tip to increasingly
			// longer chains.
			name:     "increasing",
			view:     newChainView(nil),
			tips:     []*blockNode{tip(branch0Nodes), tip(branch1Nodes)},
			contains: [][]*blockNode{branch0Nodes, branch1Nodes},
		},
		{
			// Create a view with a longer chain and set the tip to
			// increasingly shorter chains.
			name:     "decreasing",
			view:     newChainView(tip(branch1Nodes)),
			tips:     []*blockNode{tip(branch0Nodes), nil},
			contains: [][]*blockNode{branch0Nodes, nil},
		},
		{
			// Create a view with a shorter chain and set the tip to
			// a longer chain followed by setting it back to the
			// shorter chain.
			name:     "small-large-small",
			view:     newChainView(tip(branch0Nodes)),
			tips:     []*blockNode{tip(branch1Nodes), tip(branch0Nodes)},
			contains: [][]*blockNode{branch1Nodes, branch0Nodes},
		},
		{
			// Create a view with a longer chain and set the tip to
			// a smaller chain followed by setting it back to the
			// longer chain.
			name:     "large-small-large",
			view:     newChainView(tip(branch1Nodes)),
			tips:     []*blockNode{tip(branch0Nodes), tip(branch1Nodes)},
			contains: [][]*blockNode{branch0Nodes, branch1Nodes},
		},
	}

testLoop:
	for _, test := range tests {
		for i, tip := range test.tips {
			// Ensure the view tip is the expected node.
			test.view.SetTip(tip)
			if test.view.Tip() != tip {
				t.Errorf("%s: unexpected view tip -- got %v, "+
					"want %v", test.name, test.view.Tip(),
					tip)
				continue testLoop
			}

			// Ensure all expected nodes are contained in the view.
			for _, node := range test.contains[i] {
				if !test.view.Contains(node) {
					t.Errorf("%s: expected %v in active view",
						test.name, node)
					continue testLoop
				}
			}

		}
	}
}

// TestChainViewNil ensures that creating and accessing a nil chain view behaves
// as expected.
func TestChainViewNil(t *testing.T) {
	// Ensure two unininitialized views are considered equal.
	view := newChainView(nil)
	if !view.Equals(newChainView(nil)) {
		t.Fatal("uninitialized nil views unequal")
	}

	// Ensure the genesis of an uninitialized view does not produce a node.
	if genesis := view.Genesis(); genesis != nil {
		t.Fatalf("Genesis: unexpected genesis -- got %v, want nil",
			genesis)
	}

	// Ensure the tip of an uninitialized view does not produce a node.
	if tip := view.Tip(); tip != nil {
		t.Fatalf("Tip: unexpected tip -- got %v, want nil", tip)
	}

	// Ensure the height of an uninitialized view is the expected value.
	if height := view.Height(); height != -1 {
		t.Fatalf("Height: unexpected height -- got %d, want -1", height)
	}

	// Ensure attempting to get a node for a height that does not exist does
	// not produce a node.
	if node := view.NodeByHeight(10); node != nil {
		t.Fatalf("NodeByHeight: unexpected node -- got %v, want nil", node)
	}

	// Ensure an uninitialized view does not report it contains nodes.
	fakeNode := chainedNodes(nil, 1)[0]
	if view.Contains(fakeNode) {
		t.Fatalf("Contains: view claims it contains node %v", fakeNode)
	}

	// Ensure the next node for a node that does not exist does not produce
	// a node.
	if next := view.Next(nil); next != nil {
		t.Fatalf("Next: unexpected next node -- got %v, want nil", next)
	}

	// Ensure the next node for a node that exists does not produce a node.
	if next := view.Next(fakeNode); next != nil {
		t.Fatalf("Next: unexpected next node -- got %v, want nil", next)
	}

	// Ensure attempting to find a fork point with a node that doesn't exist
	// doesn't produce a node.
	if fork := view.FindFork(nil); fork != nil {
		t.Fatalf("FindFork: unexpected fork -- got %v, want nil", fork)
	}

	// Ensure attempting to get a block locator for the tip doesn't produce
	// one since the tip is nil.
	if locator := view.BlockLocator(nil); locator != nil {
		t.Fatalf("BlockLocator: unexpected locator -- got %v, want nil",
			locator)
	}

	// Ensure attempting to get a block locator for a node that exists still
	// works as intended.
	branchNodes := chainedNodes(nil, 50)
	wantLocator := locatorHashes(branchNodes, 49, 48, 47, 46, 45, 44, 43,
		42, 41, 40, 39, 38, 36, 32, 24, 8, 0)
	locator := view.BlockLocator(tstTip(branchNodes))
	if !reflect.DeepEqual(locator, wantLocator) {
		t.Fatalf("BlockLocator: unexpected locator -- got %v, want %v",
			locator, wantLocator)
	}
}
