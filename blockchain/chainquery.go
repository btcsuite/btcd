package blockchain

import (
	"sort"
	"strings"

	btcutil "github.com/lbryio/lbcutil"
)

type ChainTip struct { // duplicate of btcjson.GetChainTipsResult to avoid circular reference
	Height    int64
	Hash      string
	BranchLen int64
	Status    string
}

// nodeHeightSorter implements sort.Interface to allow a slice of nodes to
// be sorted by height in ascending order.
type nodeHeightSorter []ChainTip

// Len returns the number of nodes in the slice.  It is part of the
// sort.Interface implementation.
func (s nodeHeightSorter) Len() int {
	return len(s)
}

// Swap swaps the nodes at the passed indices.  It is part of the
// sort.Interface implementation.
func (s nodeHeightSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less returns whether the node with index i should sort before the node with
// index j.  It is part of the sort.Interface implementation.
func (s nodeHeightSorter) Less(i, j int) bool {
	// To ensure stable order when the heights are the same, fall back to
	// sorting based on hash.
	if s[i].Height == s[j].Height {
		return strings.Compare(s[i].Hash, s[j].Hash) < 0
	}
	return s[i].Height < s[j].Height
}

// ChainTips returns information, in JSON-RPC format, about all the currently
// known chain tips in the block index.
func (b *BlockChain) ChainTips() []ChainTip {
	// we need our current tip
	// we also need all of our orphans that aren't in the prevOrphans
	var results []ChainTip

	tip := b.bestChain.Tip()
	results = append(results, ChainTip{
		Height:    int64(tip.height),
		Hash:      tip.hash.String(),
		BranchLen: 0,
		Status:    "active",
	})

	b.orphanLock.RLock()
	defer b.orphanLock.RUnlock()

	notInBestChain := func(block *btcutil.Block) bool {
		node := b.bestChain.NodeByHeight(block.Height())
		if node == nil {
			return false
		}
		return node.hash.IsEqual(block.Hash())
	}

	for hash, orphan := range b.orphans {
		if len(b.prevOrphans[hash]) > 0 {
			continue
		}
		fork := orphan.block
		for fork != nil && notInBestChain(fork) {
			fork = b.orphans[*fork.Hash()].block
		}

		result := ChainTip{
			Height:    int64(orphan.block.Height()),
			Hash:      hash.String(),
			BranchLen: int64(orphan.block.Height() - fork.Height()),
		}

		// Determine the status of the chain tip.
		//
		// active:
		//   The current best chain tip.
		//
		// invalid:
		//   The block or one of its ancestors is invalid.
		//
		// headers-only:
		//   The block or one of its ancestors does not have the full block data
		//   available which also means the block can't be validated or
		//   connected.
		//
		// valid-fork:
		//   The block is fully validated which implies it was probably part of
		//   main chain at one point and was reorganized.
		//
		// valid-headers:
		//   The full block data is available and the header is valid, but the
		//   block was never validated which implies it was probably never part
		//   of the main chain.
		tipStatus := b.index.LookupNode(&hash).status
		if tipStatus.KnownInvalid() {
			result.Status = "invalid"
		} else if !tipStatus.HaveData() {
			result.Status = "headers-only"
		} else if tipStatus.KnownValid() {
			result.Status = "valid-fork"
		} else {
			result.Status = "valid-headers"
		}

		results = append(results, result)
	}

	// Generate the results sorted by descending height.
	sort.Sort(sort.Reverse(nodeHeightSorter(results)))
	return results
}
