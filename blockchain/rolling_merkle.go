package blockchain

import (
	"math"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

type RollingMerkleTree struct {
	// leaves contains all leaves in the tree. The leaves may correspond to
	// interior nodes if their children have already been pruned.
	leaves map[uint32]*chainhash.Hash

	// num is the number of leaf nodes ever pushed to the tree.
	num uint32

	// pool is a small cache of unused chainhash.Hash's that is used to
	// avoid allocating new hashes when copying hashes pushed into the tree.
	pool []*chainhash.Hash

	// buf is a statically allocated array that will be used to concatenate
	// left and right nodes and serve as the digest to interior hashes.
	buf [2 * chainhash.HashSize]byte

	// hash is slice that points to all of buf.
	hash []byte

	// left is a slice pointing to the left half of buf.
	left []byte

	// right is a slice pointing to the right half of buf.
	right []byte
}

func MakeRollingMerkleTree(size int) RollingMerkleTree {
	// Compute log2(size) in order to preallocate the amount of space
	// required by the rolling hash algorithm.
	logn := int(math.Log2(float64(size)) + 1)

	t := RollingMerkleTree{
		leaves: make(map[uint32]*chainhash.Hash, logn),
		pool:   make([]*chainhash.Hash, 0, logn),
	}

	// Construct long-lived slices to avoid allocations from reslicing the
	// arrays.
	t.hash = t.buf[:]
	t.left = t.buf[:chainhash.HashSize]
	t.right = t.buf[chainhash.HashSize:]

	return t
}

func NewRollingMerkleTree(size int) *RollingMerkleTree {
	merkle := MakeRollingMerkleTree(size)
	return &merkle
}

func (t *RollingMerkleTree) Push(hash *chainhash.Hash) {
	// Compute the index of this hash in the base of the tree. This index is
	// zero indexed, so odd indexes correspond to left nodes in the base of
	// the tree, and even indexes correspond to right nodes.
	idx := t.num

	// Increment the number of elements contained in the tree.
	t.num++

	// Since hashMerkleBranches will always write the result into the left
	// buffer, we must take extra precautions to ensure we don't mutate the
	// hashes pushed into the tree. Thus, for all leafs that would be left
	// nodes at the base of the tree we will copy the hash into a different
	// buffer. We don't need to do this for right leaves, since their
	// pointer is threaded all the way down and hashed without being
	// inserted into the tree.
	var h *chainhash.Hash
	if idx%2 == 0 {
		// If our pool of unused hashes is not empty, we'll reuse one
		// of the chainhashes, otherwise we will allocate a new
		// chainhash.
		if len(t.pool) > 0 {
			// Take the last hash from the pool and truncate the
			// slice. The taken value is nilled to prevent gc leaks.
			h = t.pool[len(t.pool)-1]
			t.pool[len(t.pool)-1] = nil
			t.pool = t.pool[:len(t.pool)-1]
		} else {
			h = new(chainhash.Hash)
		}
		copy(h[:], hash[:])

		// Finally, since we are pushing a left leaf, there will be no
		// rolling consolidation that can be performed. Hence we simply
		// add the node to our leaves index.
		t.leaves[idx] = h
	} else {
		// Otherwise this is an even index, and there should be at least
		// one interior hash we can opportunistically compute.  The hash
		// of right leaves is passed down instead of being added to the
		// tree since it can be discarded immediately after computing
		// the resulting interior node.
		t.prune(hash)
	}
}

func (t *RollingMerkleTree) Root() *chainhash.Hash {
	switch len(t.leaves) {
	case 0:
		return new(chainhash.Hash)
	case 1:
		return t.leaves[0]
	default:
		// Perform the final round of pruning by passing nil. This
		// leaves a single element in leaves corresponding to the root.
		t.prune(nil)
		return t.leaves[0]
	}

}

func (t *RollingMerkleTree) prune(leaf *chainhash.Hash) {
	// The final round of pruning is signaled by passing nil.
	final := leaf == nil

	// Attempt to consolidate the interior hashes by combining adjacent
	// nodes.
	for i := t.num - 1; i > 0; i /= 2 {
		if i%2 == 0 {
			// The next element to processes is a left node. If we
			// are not in final mode, there is nothing else we can
			// do until more elements are pushed on or the final
			// root is computed.
			if !final {
				return
			}

			// Otherwise we are computing the final root. If the
			// left item exists, we'll hash it with itself to mimic
			// the idiosyncrasies of the bitcoin merkle tree
			// structure. If the node doesn't exist, the interior
			// nodes have already been consolidated to a lower
			// index, so we simply continue to try and find them.
			if left, ok := t.leaves[i]; ok {
				t.leaves[i/2] = t.hashMerkleBranches(left, left)
				delete(t.leaves, i)
			}
			continue
		}

		// The next element to process is a right node. Locate the node
		// that should be directly to the left. If we are in final mode
		// and the node is not found, we'll continue iterating until
		// reaching the next interior node. Otherwise, we return and
		// wait for more items to be pushed on.
		left, ok := t.leaves[i-1]
		if !ok && final {
			continue
		} else if !ok && !final {
			return
		}

		// The left node was found and will be immediately used to
		// compute the next interior hash. Thus we remove the node from
		// the tree.
		delete(t.leaves, i-1)

		// Now, retrieve the right node from the tree. If this is the
		// first iteration and we are not in final mode, then the right
		// leaf has been threaded through. Otherwise, we lookup and
		// remove the interior node from the tree using the current
		// index.
		var right *chainhash.Hash
		if !final && i == t.num-1 {
			right = leaf
		} else {
			right = t.leaves[i]
			delete(t.leaves, i)

			// Return any interior right nodes to the pool if we
			// have excess capacity. Leaf right nodes are never
			// added here, so they will never be over written.
			if len(t.pool) < cap(t.pool) {
				t.pool = append(t.pool, right)
			}
		}

		// Finally, compute the resulting interior node by computing the
		// hash of left||right. The result value is stored back in the
		// left pointer.
		t.leaves[i/2] = t.hashMerkleBranches(left, right)
	}
}

// hashMerkleBranches concatenates the left and write hashes and computes the
// SHA256 hash. The result is copied into the left buffer.
func (t *RollingMerkleTree) hashMerkleBranches(
	left, right *chainhash.Hash) *chainhash.Hash {

	// Concatenate the left and right nodes.
	copy(t.left, left[:])
	copy(t.right, right[:])

	*left = chainhash.DoubleHashH(t.hash)
	return left
}
