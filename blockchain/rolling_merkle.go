package blockchain

import (
	"math"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// RollingMerkleTree computes the double SHA256 merkle root over a set of leaf
// hashes. Interior nodes are opportunistically computed as new leaves are
// added to the tree, consolidating any nodes that have both a left and right
// child before permitting another leaf to be added. As a result, a
// RollingMerkleTree only requires O(log n) additional storage to compute the
// final root.
type RollingMerkleTree struct {
	// nodes contains all nodes in the tree. The nodes may correspond to
	// interior nodes if their children have already been pruned.
	nodes map[uint32]chainhash.Hash

	// num is the number of leaf nodes ever pushed to the tree.
	num uint32

	// buf is a statically allocated array that will be used to concatenate
	// left and right nodes and serve as the digest to interior hashes.
	buf [2 * chainhash.HashSize]byte

	// both is slice that points to all of buf.
	both []byte

	// left is a slice pointing to the left half of buf.
	left []byte

	// right is a slice pointing to the right half of buf.
	right []byte
}

// MakeRollingMerkleTree creates a RollingMerkleTree that is preallocated for
// size leaves. More than size leaves may be pushed onto the tree, though may
// result in more wasteful allocations than is necessary.
func MakeRollingMerkleTree(size int) RollingMerkleTree {
	// Compute log2(size) in order to preallocate the amount of space
	// required by the rolling hash algorithm.
	logn := int(math.Log2(float64(size)) + 1)

	t := RollingMerkleTree{
		nodes: make(map[uint32]chainhash.Hash, logn),
	}

	// Construct long-lived slices to avoid reslicing the underlying arrays.
	t.both = t.buf[:]
	t.left = t.buf[:chainhash.HashSize]
	t.right = t.buf[chainhash.HashSize:]

	return t
}

// NewRollingMerkleTree initializes a RollingMerkleTree on the heap that is
// preallocated for size leaves. More than size leaves may be pushed onto the
// tree, though may result in more wasteful allocations than is necessary.
func NewRollingMerkleTree(size int) *RollingMerkleTree {
	merkle := MakeRollingMerkleTree(size)
	return &merkle
}

// Push adds the next leaf to the RollingMerkleTree. When adding a left leaf,
// the hash is copied and stored in the tree under its leaf index. When a right
// node is added, it is immediately consolidated with the preceding left node
// that was stored without actually adding it to the tree. If the resulting
// interior node then has a sibling, the interior nodes are hashed again and
// the resulting interior node is stored one level higher in the tree. This
// process is applied iteratively until all complete subtrees are represented by
// a single hash after a call to Push. Since there can be at most O(log n) such
// subtrees, this bounds the maximal state required at any point during the
// computation.
func (t *RollingMerkleTree) Push(hash *chainhash.Hash) {
	// Increment the number of elements contained in the tree. This is done
	// before calling prune, since we expect this count to be accurate when
	// calling prune in Push, as well as when calling Root.
	t.num++

	// Compute the index of this hash in the base of the tree. This index is
	// zero indexed, so even indexes correspond to left nodes in the base of
	// the tree, and odd indexes correspond to right nodes.
	idx := t.num - 1
	if idx%2 == 0 {
		// If we are pushing a left leaf, there will be no rolling
		// consolidation that can be performed. Hence we simply add the
		// node to our nodes index.
		t.nodes[idx] = *hash
	} else {
		// Otherwise this is a right leaf, and there should be at least
		// one interior hash we can opportunistically compute. The hash
		// of right leaves is passed down instead of being added to the
		// tree since it can be discarded immediately after computing
		// the resulting interior node.
		t.prune(hash)
	}
}

// Root returns the final merkle root of all elements added via Push.
func (t *RollingMerkleTree) Root() chainhash.Hash {
	switch len(t.nodes) {

	// If the tree is empty, the root is the empty hash.
	case 0:
		return chainhash.Hash{}

	// If there is only one element in the pruned tree, it is the root.
	case 1:
		return t.nodes[0]

	// Otherwise, we must perform the final round of pruning where any lone
	// left-children are hashed with themselves to compute its parent
	// interior node. This leaves a single element in the pruned tree
	// corresponding to the root.
	default:
		t.prune(nil)
		return t.nodes[0]
	}

}

// prune iteratively consolidates all complete subtrees into a single hash
// stored in nodes. When leaf is non-nil, it is assumed that the provided hash
// is a right leaf, and there exists at least one consolidation available, e.g.
// the left leaf added to nodes directly preceding. When no leaf is given, it
// is assumed that no more leaves will be added and that we must compute the
// final root of the tree. When this occurs, there may be lone left-nodes,
// which are consolidated by hashing the leaf with itself, as is required by
// consensus.
func (t *RollingMerkleTree) prune(leaf *chainhash.Hash) {
	// The final round of pruning is signaled by passing nil.
	final := leaf == nil

	// Attempt to consolidate the interior hashes by combining adjacent
	// nodes.
	for i := t.num - 1; i > 0; i /= 2 {
		if i%2 == 0 {
			// The next element to processes is a left node. If we
			// are not in final mode, there is nothing else we can
			// do until more elements are pushed and the right
			// subtree is complete or the final root is computed.
			if !final {
				return
			}

			// Otherwise we are computing the final root. If the
			// left item exists, we'll hash it with itself to mimic
			// the idiosyncrasies of the bitcoin merkle tree
			// structure. If the node doesn't exist, the interior
			// nodes have already been consolidated to a lower
			// index, so we simply continue to try and find them.
			if left, ok := t.nodes[i]; ok {
				delete(t.nodes, i)
				t.nodes[i/2] = t.hashMerkleBranches(&left, &left)
			}
			continue
		}

		// The next element to process is a right node. Locate the node
		// that should be directly to the left. If we are in final mode
		// and the node is not found, we'll continue iterating until
		// reaching the next interior node. Otherwise, we return and
		// wait for more items to be pushed on.
		left, ok := t.nodes[i-1]
		if !ok && final {
			continue
		} else if !ok && !final {
			return
		}

		// The left node was found and will be immediately used to
		// compute the next interior hash. Thus we remove the node from
		// the tree.
		delete(t.nodes, i-1)

		// Now, retrieve the right node from the tree. If this is the
		// first iteration and we are not in final mode, then the right
		// leaf has been threaded through. Otherwise, we lookup and
		// remove the interior node from the tree using the current
		// index. The right node should always exist in the latter case,
		// as it is the interior node computed in the prior iteration.
		var right chainhash.Hash
		if !final && i == t.num-1 {
			right = *leaf
		} else {
			right = t.nodes[i]
			delete(t.nodes, i)
		}

		// Finally, compute the resulting interior node by computing the
		// hash of left||right. The result is stored as the parent node
		// within the nodes index.
		t.nodes[i/2] = t.hashMerkleBranches(&left, &right)
	}
}

// hashMerkleBranches concatenates the left and write hashes and computes the
// SHA256 hash.
func (t *RollingMerkleTree) hashMerkleBranches(
	left, right *chainhash.Hash) chainhash.Hash {

	// Concatenate the left and right nodes.
	copy(t.left, left[:])
	copy(t.right, right[:])

	return chainhash.DoubleHashH(t.both)
}
