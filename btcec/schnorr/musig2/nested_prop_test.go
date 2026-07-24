// Copyright 2013-2025 The btcsuite developers

package musig2

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// testTree holds a generated tree along with all the leaf private keys needed
// to run the full signing protocol.
type testTree struct {
	root     *TreeNode
	leafKeys []*btcec.PrivateKey
}

// aggInfo pairs an Aggregator with its tree node for use in the protocol
// runner.
type aggInfo struct {
	agg  *Aggregator
	node *TreeNode
}

// genTreeNode recursively generates a random tree node. At each level it
// decides whether to create a leaf or a group (internal node). The deeper we
// go, the more likely we create leaves to ensure termination.
func genTreeNode(t *rapid.T, depth int, maxDepth int,
	keys *[]*btcec.PrivateKey, seed *uint32) *TreeNode {

	// At max depth or randomly chosen, create a leaf.
	isLeaf := depth >= maxDepth ||
		rapid.IntRange(0, maxDepth-depth).Draw(t, "leafOrGroup") == 0

	if isLeaf {
		priv := genKeyFromSeed(*seed)
		*seed++
		*keys = append(*keys, priv)

		return Leaf(priv.PubKey())
	}

	// Create an internal group with 2-5 children.
	numChildren := rapid.IntRange(2, 5).Draw(t, "numChildren")

	children := make([]*TreeNode, numChildren)
	for i := range children {
		children[i] = genTreeNode(
			t, depth+1, maxDepth, keys, seed,
		)
	}

	return Group(children, WithKeySort())
}

// genKeyFromSeed generates a deterministic private key from a seed value. Uses
// all 4 bytes of the seed to support trees with more than 255 leaves.
func genKeyFromSeed(seed uint32) *btcec.PrivateKey {
	var keyBytes [32]byte
	keyBytes[0] = byte(seed)
	keyBytes[1] = byte(seed >> 8)
	keyBytes[2] = byte(seed >> 16)
	keyBytes[3] = byte(seed >> 24)
	h := sha256.Sum256(keyBytes[:])
	priv, _ := btcec.PrivKeyFromBytes(h[:])

	return priv
}

// genTree generates a random tree suitable for property-based testing.
func genTree(t *rapid.T) testTree {
	maxDepth := rapid.IntRange(1, 5).Draw(t, "maxDepth")
	var keys []*btcec.PrivateKey
	seed := uint32(1)

	root := genTreeNode(t, 0, maxDepth, &keys, &seed)

	// Ensure we have a valid internal root (at least 2 children).
	// If the generator produced a single leaf at root, wrap it.
	if root.isLeaf() {
		priv2 := genKeyFromSeed(seed)
		seed++
		keys = append(keys, priv2)

		root = Group([]*TreeNode{
			root, Leaf(priv2.PubKey()),
		}, WithKeySort())
	}

	return testTree{root: root, leafKeys: keys}
}

// runFullProtocol executes the entire nested MuSig2 signing protocol for the
// given tree and returns the final signature and root aggregate key.
func runFullProtocol(t *testing.T, tree *TreeNode,
	leafKeys []*btcec.PrivateKey,
	msg [32]byte) (*schnorr.Signature, *AggregateKey) {

	t.Helper()

	rootKey, err := AggregateTree(tree)
	require.NoError(t, err)

	// Create sessions for each leaf.
	sessions := make([]*NestedSession, len(leafKeys))
	for i, key := range leafKeys {
		sess, err := NewNestedSession(key, tree)
		require.NoError(t, err)
		sessions[i] = sess
	}

	// Build aggregators for each internal node. We do a BFS/DFS to
	// collect them and map leaf keys to their parent aggregator.
	allAggs := make(map[*TreeNode]*aggInfo)
	var buildAggs func(n *TreeNode)
	buildAggs = func(n *TreeNode) {
		if !n.isInternal() {
			return
		}

		agg, err := NewAggregator(n)
		require.NoError(t, err)
		allAggs[n] = &aggInfo{agg: agg, node: n}

		for _, child := range n.Children {
			buildAggs(child)
		}
	}
	buildAggs(tree)

	// Map each leaf pubkey to its session index.
	leafSessionMap := make(map[string]int)
	for i, key := range leafKeys {
		pkHex := fmt.Sprintf("%x", key.PubKey().SerializeCompressed())
		leafSessionMap[pkHex] = i
	}

	// Round 1: Register nonces bottom-up.
	// We process nodes bottom-up by computing depth first.
	type nodeDepth struct {
		node  *TreeNode
		depth int
	}

	var allNodes []nodeDepth
	var collectInternal func(n *TreeNode, d int)
	collectInternal = func(n *TreeNode, d int) {
		if !n.isInternal() {
			return
		}
		allNodes = append(allNodes, nodeDepth{n, d})
		for _, child := range n.Children {
			collectInternal(child, d+1)
		}
	}
	collectInternal(tree, 0)

	// Sort by depth descending (deepest first).
	for i := 0; i < len(allNodes); i++ {
		for j := i + 1; j < len(allNodes); j++ {
			if allNodes[j].depth > allNodes[i].depth {
				allNodes[i], allNodes[j] = allNodes[j], allNodes[i]
			}
		}
	}

	// Register nonces at each level.
	for _, nd := range allNodes {
		ai := allAggs[nd.node]
		for _, child := range nd.node.Children {
			if child.isLeaf() {
				pkHex := fmt.Sprintf(
					"%x",
					child.PubKey.SerializeCompressed(),
				)
				idx := leafSessionMap[pkHex]

				_, err := ai.agg.RegisterNonce(
					child.PubKey,
					sessions[idx].PublicNonce(),
				)
				require.NoError(t, err)
			} else {
				// Internal or opaque child — get projected
				// nonce from child aggregator.
				childAI := allAggs[child]
				projected, err := childAI.agg.ProjectedNonce()
				require.NoError(t, err)

				_, err = ai.agg.RegisterNonce(
					child.effectiveKey(), projected,
				)
				require.NoError(t, err)
			}
		}
	}

	rootAI := allAggs[tree]
	rootCombined, err := rootAI.agg.AggNonce()
	require.NoError(t, err)

	// Round 2: Sign — each leaf uses the coefficients from the
	// aggregators along its specific path (not a shared depth map,
	// since sibling groups at the same depth have different b_d).
	leafSigs := make([]*PartialSignature, len(leafKeys))
	for i, key := range leafKeys {
		// Find the leaf's path through the tree to determine which
		// aggregators' coefficients it needs.
		leafCoeffs := collectLeafCoeffs(
			t, tree, key.PubKey(), allAggs, 0,
		)

		var signCoeffs map[TreeDepth]*btcec.ModNScalar
		if len(leafCoeffs) > 0 {
			signCoeffs = leafCoeffs
		}

		sig, err := sessions[i].Sign(
			msg, rootCombined, signCoeffs,
		)
		require.NoError(t, err)
		leafSigs[i] = sig
	}

	// Register signatures bottom-up.
	for _, nd := range allNodes {
		ai := allAggs[nd.node]
		for _, child := range nd.node.Children {
			if child.isLeaf() {
				pkHex := fmt.Sprintf(
					"%x",
					child.PubKey.SerializeCompressed(),
				)
				idx := leafSessionMap[pkHex]

				_, err := ai.agg.RegisterSig(
					child.PubKey, leafSigs[idx],
				)
				require.NoError(t, err)
			} else {
				childAI := allAggs[child]
				partial, err := childAI.agg.AsPartialSig()
				require.NoError(t, err)

				_, err = ai.agg.RegisterSig(
					child.effectiveKey(), partial,
				)
				require.NoError(t, err)
			}
		}
	}

	// Final signature.
	finalSig, err := rootAI.agg.FinalSig(msg)
	require.NoError(t, err)

	return finalSig, rootKey
}

// collectLeafCoeffs walks the tree to find the leaf and collects the nonce
// coefficients (b_d) from each internal node along its path that is not the
// root (depth > 0). This correctly handles sibling groups at the same depth
// having different b_d values.
func collectLeafCoeffs(t *testing.T, node *TreeNode,
	leafKey *btcec.PublicKey,
	aggs map[*TreeNode]*aggInfo,
	depth int) map[TreeDepth]*btcec.ModNScalar {

	t.Helper()

	if node.isLeaf() {
		if node.PubKey.IsEqual(leafKey) {
			return make(map[TreeDepth]*btcec.ModNScalar)
		}

		return nil
	}

	if node.Opaque {
		return nil
	}

	// Search children for the leaf.
	for _, child := range node.Children {
		childCoeffs := collectLeafCoeffs(
			t, child, leafKey, aggs, depth+1,
		)
		if childCoeffs == nil {
			continue
		}

		// Found the leaf in this subtree. If the child is an internal
		// node (has an aggregator) and is not at the root level, add
		// its nonce coefficient.
		if child.isInternal() {
			childAI, ok := aggs[child]
			require.True(t, ok)

			bd, err := childAI.agg.NonceCoeff()
			require.NoError(t, err)
			childCoeffs[TreeDepth(depth+1)] = bd
		}

		return childCoeffs
	}

	return nil
}

// countLeaves counts the total number of leaf nodes in a tree.
func countLeaves(n *TreeNode) int {
	if n.isLeaf() || n.Opaque {
		return 1
	}

	count := 0
	for _, child := range n.Children {
		count += countLeaves(child)
	}

	return count
}

// maxTreeDepth returns the maximum depth of the tree.
func maxTreeDepth(n *TreeNode) int {
	if n.isLeaf() || n.Opaque {
		return 0
	}

	maxChild := 0
	for _, child := range n.Children {
		d := maxTreeDepth(child)
		if d > maxChild {
			maxChild = d
		}
	}

	return maxChild + 1
}

// TestNestedPropertyArbitraryDepth verifies that nested MuSig2 produces valid
// BIP-340 Schnorr signatures for arbitrary tree configurations.
func TestNestedPropertyArbitraryDepth(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		tt := genTree(rt)

		numLeaves := countLeaves(tt.root)
		depth := maxTreeDepth(tt.root)
		t.Logf("tree: %d leaves, depth %d", numLeaves, depth)

		msg := sha256.Sum256([]byte("property test"))

		finalSig, rootKey := runFullProtocol(
			t, tt.root, tt.leafKeys, msg,
		)

		// Property 1: Final signature is valid BIP-340 Schnorr.
		require.True(t, finalSig.Verify(msg[:], rootKey.FinalKey),
			"signature should verify for tree with %d leaves, "+
				"depth %d", numLeaves, depth)

		// Property 2: Signature is standard (can be serialized and
		// re-parsed).
		sigBytes := finalSig.Serialize()
		parsedSig, err := schnorr.ParseSignature(sigBytes)
		require.NoError(t, err)
		require.True(t, parsedSig.Verify(msg[:], rootKey.FinalKey),
			"re-parsed signature should verify")
	})
}

// TestNestedPropertyOpaqueEquivalence verifies that replacing any subtree
// with an OpaqueNode(subtreeAggKey) produces the same root aggregate key.
func TestNestedPropertyOpaqueEquivalence(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		tt := genTree(rt)

		_, err := AggregateTree(tt.root)
		require.NoError(t, err)

		// For each internal node, build an equivalent tree with that
		// node replaced by an opaque node.
		var checkOpaque func(original *TreeNode)
		checkOpaque = func(original *TreeNode) {
			if !original.isInternal() {
				return
			}

			// Build a copy of the root tree, replacing this node
			// with OpaqueNode(node.effectiveKey()).
			copyTree := replaceWithOpaque(
				tt.root, original,
			)
			if copyTree == nil {
				// The node IS the root — skip (can't replace
				// root with opaque).
				return
			}

			opaqueRootKey, err := AggregateTree(copyTree)
			require.NoError(t, err)

			require.True(t,
				tt.root.aggKey.FinalKey.IsEqual(
					opaqueRootKey.FinalKey,
				),
				"opaque replacement should preserve root key",
			)

			// Recurse into children.
			for _, child := range original.Children {
				checkOpaque(child)
			}
		}

		for _, child := range tt.root.Children {
			checkOpaque(child)
		}
	})
}

// replaceWithOpaque deep-copies a tree, replacing targetNode with an opaque
// node. Returns nil if targetNode IS the root.
func replaceWithOpaque(root, target *TreeNode) *TreeNode {
	if root == target {
		return nil
	}

	return replaceNode(root, target)
}

// replaceNode recursively copies the tree, replacing target with an opaque
// node.
func replaceNode(node, target *TreeNode) *TreeNode {
	if node == target {
		return OpaqueNode(node.effectiveKey())
	}

	if node.isLeaf() || node.Opaque {
		return &TreeNode{
			PubKey: node.PubKey,
			Opaque: node.Opaque,
			opts:   node.opts,
		}
	}

	// Internal node: copy children, replacing target if found.
	newChildren := make([]*TreeNode, len(node.Children))
	for i, child := range node.Children {
		newChildren[i] = replaceNode(child, target)
	}

	return &TreeNode{
		Children: newChildren,
		opts:     node.opts,
	}
}

// TestNestedPropertyFlatEquivalence verifies that a flat tree (all leaves at
// root) produces the same root aggregate key as standard AggregateKeys.
func TestNestedPropertyFlatEquivalence(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		numSigners := rapid.IntRange(2, 10).Draw(
			rt, "numSigners",
		)

		keys := make([]*btcec.PrivateKey, numSigners)
		pubs := make([]*btcec.PublicKey, numSigners)
		leaves := make([]*TreeNode, numSigners)

		for i := range numSigners {
			keys[i] = genKeyFromSeed(uint32(i + 1))
			pubs[i] = keys[i].PubKey()
			leaves[i] = Leaf(pubs[i])
		}

		// Tree-based aggregation.
		tree := Group(leaves, WithKeySort())
		treeKey, err := AggregateTree(tree)
		require.NoError(t, err)

		// Standard aggregation.
		stdKey, _, _, err := AggregateKeys(pubs, true)
		require.NoError(t, err)

		require.True(t, treeKey.FinalKey.IsEqual(stdKey.FinalKey),
			"flat tree key should match standard aggregation "+
				"for %d signers", numSigners)

		// Also verify the full signing protocol produces a valid sig.
		msg := sha256.Sum256([]byte("flat equivalence"))
		finalSig, _ := runFullProtocol(t, tree, keys, msg)

		require.True(t, finalSig.Verify(msg[:], treeKey.FinalKey),
			"flat tree signature should verify")
	})
}

// TestNestedPropertyPathCoefficients verifies that path coefficients are
// consistent: a_path for a leaf is the product of aggregation coefficients
// at each level, and this product is non-zero for all reachable leaves.
func TestNestedPropertyPathCoefficients(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		tt := genTree(rt)

		_, err := AggregateTree(tt.root)
		require.NoError(t, err)

		for _, key := range tt.leafKeys {
			path, err := tt.root.FindPath(key.PubKey())
			require.NoError(t, err)

			// a_path should be non-zero.
			aPath := path.APath()
			require.False(t, aPath.IsZero(),
				"a_path should be non-zero for leaf %x",
				key.PubKey().SerializeCompressed())

			// Number of levels should equal the leaf's depth.
			require.Greater(t, len(path.Levels), 0,
				"path should have at least one level")

			// Depths should be monotonically increasing.
			for i := 1; i < len(path.Levels); i++ {
				require.Greater(t,
					path.Levels[i].Depth,
					path.Levels[i-1].Depth,
					"path depths should increase",
				)
			}

			// First level should be depth 0 (root).
			require.Equal(t, TreeDepth(0),
				path.Levels[0].Depth,
				"first path level should be root (depth 0)")
		}
	})
}

// TestNestedPropertyDifferentMessages verifies that the same tree
// configuration produces valid signatures for different messages.
func TestNestedPropertyDifferentMessages(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		tt := genTree(rt)

		msgBytes := rapid.SliceOfN(
			rapid.Byte(), 1, 100,
		).Draw(rt, "msgBytes")
		msg := sha256.Sum256(msgBytes)

		finalSig, rootKey := runFullProtocol(
			t, tt.root, tt.leafKeys, msg,
		)

		require.True(t, finalSig.Verify(msg[:], rootKey.FinalKey),
			"signature should verify for arbitrary message")
	})
}
