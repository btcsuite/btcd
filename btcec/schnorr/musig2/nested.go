// Copyright 2013-2025 The btcsuite developers

package musig2

import (
	"bytes"
	"fmt"

	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

var (
	// ErrNodeNotInternal is returned when an operation that requires an
	// internal node (one with children) is called on a leaf or opaque node.
	ErrNodeNotInternal = fmt.Errorf("node is not an internal group node")

	// ErrNodeNotAggregated is returned when an operation requires the tree
	// to have been aggregated first via AggregateTree.
	ErrNodeNotAggregated = fmt.Errorf("tree has not been aggregated")

	// ErrLeafNotFound is returned when FindPath cannot locate the
	// specified leaf public key in the tree.
	ErrLeafNotFound = fmt.Errorf("leaf public key not found in tree")

	// ErrLeafInOpaqueSubtree is returned when FindPath encounters the
	// target key inside an opaque subtree whose internal structure is
	// unknown.
	ErrLeafInOpaqueSubtree = fmt.Errorf(
		"cannot find path into opaque subtree",
	)
)

// TreeDepth represents the depth of a node in the cosigner tree. Depth 0 is
// the root level.
type TreeDepth uint32

// TreeNodeOption is a functional option for tree node construction.
type TreeNodeOption func(*treeNodeOptions)

// treeNodeOptions houses the set of functional options that can be used to
// modify tree node behavior.
type treeNodeOptions struct {
	sortKeys       bool
	tweaks         []KeyTweakDesc
	taprootTweak   []byte
	bip86Tweak     bool
	hasTaprootOpts bool
}

// defaultTreeNodeOptions returns the default tree node options.
func defaultTreeNodeOptions() *treeNodeOptions {
	return &treeNodeOptions{}
}

// WithKeySort specifies that children should be sorted by key during
// aggregation at this node.
func WithKeySort() TreeNodeOption {
	return func(o *treeNodeOptions) {
		o.sortKeys = true
	}
}

// WithNodeTweaks applies tweaks to the aggregate key at this node.
func WithNodeTweaks(tweaks ...KeyTweakDesc) TreeNodeOption {
	return func(o *treeNodeOptions) {
		o.tweaks = tweaks
	}
}

// WithTaprootNodeTweak applies a taproot tweak at this node. The scriptRoot
// is the merkle root of the taproot script tree.
func WithTaprootNodeTweak(scriptRoot []byte) TreeNodeOption {
	return func(o *treeNodeOptions) {
		o.taprootTweak = scriptRoot
		o.hasTaprootOpts = true
	}
}

// WithBIP86NodeTweak applies a BIP-86 tweak at this node. This commits to
// just the internal key with no script tree.
func WithBIP86NodeTweak() TreeNodeOption {
	return func(o *treeNodeOptions) {
		o.bip86Tweak = true
		o.hasTaprootOpts = true
	}
}

// TreeNode describes a node in the cosigner tree. Leaf nodes hold a single
// public key. Internal nodes hold children and compute an aggregate key.
// Opaque nodes represent subtrees whose internal structure is unknown — only
// their aggregate key is known (for partial tree views).
type TreeNode struct {
	// PubKey is the public key for this node:
	//   - Leaf: the signer's public key
	//   - Internal: populated by AggregateTree (aggregate of children)
	//   - Opaque: the known aggregate key of the hidden subtree
	PubKey *btcec.PublicKey

	// Children holds child nodes for internal nodes. nil for leaves and
	// opaque nodes.
	Children []*TreeNode

	// Opaque marks this as a node whose subtree structure is unknown. Only
	// the PubKey (aggregate key) is known.
	Opaque bool

	// opts holds the functional options applied to this node.
	opts *treeNodeOptions

	// Populated by AggregateTree:
	aggKey       *AggregateKey
	parityAcc    btcec.ModNScalar
	tweakAcc     btcec.ModNScalar
	keysHash     []byte
	uniqueKeyIdx int
}

// isLeaf returns true if this node is a leaf (no children, not opaque).
func (tn *TreeNode) isLeaf() bool {
	return len(tn.Children) == 0 && !tn.Opaque
}

// isInternal returns true if this node is an internal group node.
func (tn *TreeNode) isInternal() bool {
	return len(tn.Children) > 0
}

// effectiveKey returns the public key that should be used when this node
// participates as a child in a parent aggregation.
func (tn *TreeNode) effectiveKey() *btcec.PublicKey {
	// For internal nodes that have been aggregated, use the final
	// aggregate key.
	if tn.aggKey != nil {
		return tn.aggKey.FinalKey
	}

	// For leaves and opaque nodes, use the stored public key.
	return tn.PubKey
}

// Leaf creates a leaf node for a single signer.
func Leaf(pubKey *btcec.PublicKey) *TreeNode {
	return &TreeNode{
		PubKey: pubKey,
		opts:   defaultTreeNodeOptions(),
	}
}

// Group creates an internal node. Each child is either a Leaf, another Group,
// or an OpaqueNode. Functional options control sorting and tweaks.
func Group(children []*TreeNode, opts ...TreeNodeOption) *TreeNode {
	nodeOpts := defaultTreeNodeOptions()
	for _, opt := range opts {
		opt(nodeOpts)
	}

	return &TreeNode{
		Children: children,
		opts:     nodeOpts,
	}
}

// OpaqueNode creates a node representing a subtree whose internal structure is
// unknown. Only the aggregate public key is provided. This enables partial
// tree views where a participant doesn't know the full tree — they know their
// own subtree and see sibling subtrees as opaque aggregate keys.
func OpaqueNode(aggKey *btcec.PublicKey) *TreeNode {
	return &TreeNode{
		PubKey: aggKey,
		Opaque: true,
		opts:   defaultTreeNodeOptions(),
	}
}

// AggregateTree recursively aggregates keys from leaves to root. It populates
// each internal node's aggKey, parityAcc, tweakAcc, keysHash, and
// uniqueKeyIdx. Opaque nodes are treated as pre-aggregated (their PubKey is
// used directly as a child key for the parent's aggregation). Returns the root
// aggregate key.
func AggregateTree(root *TreeNode) (*AggregateKey, error) {
	return aggregateTreeNode(root)
}

// aggregateTreeNode recursively aggregates a single tree node.
func aggregateTreeNode(node *TreeNode) (*AggregateKey, error) {
	// Leaves and opaque nodes don't need aggregation.
	if node.isLeaf() || node.Opaque {
		return nil, nil
	}

	// Recursively aggregate all children first (bottom-up).
	for _, child := range node.Children {
		if _, err := aggregateTreeNode(child); err != nil {
			return nil, err
		}
	}

	// Collect the effective public keys of all children.
	childKeys := make([]*btcec.PublicKey, len(node.Children))
	for i, child := range node.Children {
		childKeys[i] = child.effectiveKey()
	}

	// Build key aggregation options from the node's functional options.
	var keyAggOpts []KeyAggOption
	switch {
	case node.opts.bip86Tweak:
		keyAggOpts = append(keyAggOpts, WithBIP86KeyTweak())

	case node.opts.hasTaprootOpts:
		keyAggOpts = append(
			keyAggOpts,
			WithTaprootKeyTweak(node.opts.taprootTweak),
		)

	case len(node.opts.tweaks) > 0:
		keyAggOpts = append(
			keyAggOpts, WithKeyTweaks(node.opts.tweaks...),
		)
	}

	// Perform key aggregation for this node.
	aggKey, parityAcc, tweakAcc, err := AggregateKeys(
		childKeys, node.opts.sortKeys, keyAggOpts...,
	)
	if err != nil {
		return nil, fmt.Errorf("aggregate keys at node: %w", err)
	}

	// Populate the node's aggregated state.
	node.aggKey = aggKey
	node.PubKey = aggKey.FinalKey
	node.parityAcc = *parityAcc
	node.tweakAcc = *tweakAcc
	node.keysHash = keyHashFingerprint(childKeys, node.opts.sortKeys)
	node.uniqueKeyIdx = secondUniqueKeyIndex(
		childKeys, node.opts.sortKeys,
	)

	return aggKey, nil
}

// ProjectNonces performs SignAggExt: projects an internally aggregated nonce
// pair for use by the parent level.
//
// Given aggNonce (R'_1 || R'_2) from standard AggregateNonces and the group's
// aggregate key X, it returns:
//   - projectedNonce: (R'_1, b_d * R'_2) serialized for the parent
//   - nonceCoeff: the internal coefficient b_d
//
// The nonce coefficient is computed as:
//
//	b_d = H("MuSig/noncecoef", X || R'_1 || R'_2)
//
// This follows the paper's H_non(X̃, (R'_1, ..., R'_ν)) definition with
// key-first ordering, distinct from BIP-327's root-level nonces-first
// ordering.
func ProjectNonces(aggNonce [PubNonceSize]byte,
	groupKey *btcec.PublicKey) (
	[PubNonceSize]byte, *btcec.ModNScalar, error) {

	// Compute b_d = H("MuSig/noncecoef", X || R'_agg). Key first per the
	// paper's specification.
	var hashBuf bytes.Buffer
	hashBuf.Write(schnorr.SerializePubKey(groupKey))
	hashBuf.Write(aggNonce[:])

	bHash := chainhash.TaggedHash(NonceBlindTag, hashBuf.Bytes())
	var bd btcec.ModNScalar
	bd.SetByteSlice(bHash[:])

	// Parse R'_2 (second 33 bytes of the combined nonce).
	r2, err := btcec.ParsePubKey(
		aggNonce[btcec.PubKeyBytesLenCompressed:],
	)
	if err != nil {
		return [PubNonceSize]byte{}, nil, fmt.Errorf(
			"parse R'_2: %w", err,
		)
	}

	// Compute b_d * R'_2.
	var r2J btcec.JacobianPoint
	r2.AsJacobian(&r2J)
	btcec.ScalarMultNonConst(&bd, &r2J, &r2J)
	r2J.ToAffine()

	// Serialize the projected nonce: (R'_1, b_d * R'_2).
	var projectedNonce [PubNonceSize]byte

	// R'_1 is unchanged (first 33 bytes).
	copy(projectedNonce[:btcec.PubKeyBytesLenCompressed],
		aggNonce[:btcec.PubKeyBytesLenCompressed])

	// b_d * R'_2 replaces the second 33 bytes.
	projR2 := btcec.NewPublicKey(&r2J.X, &r2J.Y)
	copy(projectedNonce[btcec.PubKeyBytesLenCompressed:],
		projR2.SerializeCompressed())

	return projectedNonce, &bd, nil
}

// PathLevel holds per-level info along a signer's path through the cosigner
// tree.
type PathLevel struct {
	// AggKey is the aggregate key at this level.
	AggKey *btcec.PublicKey

	// KeyCoeff is the key aggregation coefficient a_i for this
	// signer/subgroup at this level.
	KeyCoeff btcec.ModNScalar

	// ParityAcc is the parity accumulator from tweaks at this level.
	ParityAcc btcec.ModNScalar

	// TweakAcc is the tweak accumulator at this level.
	TweakAcc btcec.ModNScalar

	// Depth is the depth of this level in the tree (0 = root).
	Depth TreeDepth
}

// SignerPath captures a leaf signer's path from root to leaf with all
// coefficients needed for signing.
type SignerPath struct {
	// Levels holds path info from root (index 0) to the leaf's immediate
	// parent (last index).
	Levels []PathLevel

	// LeafPubKey is the public key of the leaf signer.
	LeafPubKey *btcec.PublicKey
}

// APath returns the product of all key aggregation coefficients along the
// path. This is ∏ a_i from root to leaf.
func (sp *SignerPath) APath() *btcec.ModNScalar {
	result := new(btcec.ModNScalar).SetInt(1)
	for i := range sp.Levels {
		result.Mul(&sp.Levels[i].KeyCoeff)
	}

	return result
}

// AccumulatedParity returns the product of all parity accumulators along the
// path (from tweaks at each level).
func (sp *SignerPath) AccumulatedParity() *btcec.ModNScalar {
	result := new(btcec.ModNScalar).SetInt(1)
	for i := range sp.Levels {
		result.Mul(&sp.Levels[i].ParityAcc)
	}

	return result
}

// AccumulatedTweak returns the tweak accumulator from the root level. Only
// the root-level tweak accumulator is needed for final signature correction.
func (sp *SignerPath) AccumulatedTweak() *btcec.ModNScalar {
	if len(sp.Levels) == 0 {
		return new(btcec.ModNScalar)
	}

	result := new(btcec.ModNScalar).Set(&sp.Levels[0].TweakAcc)
	return result
}

// FindPath locates a leaf by public key and returns its SignerPath. Returns
// an error if the key is not found or is in an opaque subtree.
func (tn *TreeNode) FindPath(
	leafKey *btcec.PublicKey) (*SignerPath, error) {

	path, found := findPathRecursive(tn, leafKey, 0)
	if !found {
		return nil, ErrLeafNotFound
	}

	// The recursive helper builds the path in leaf-to-root order
	// (appending at each level). Reverse it to get root-to-leaf order.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return &SignerPath{
		Levels:     path,
		LeafPubKey: leafKey,
	}, nil
}

// findPathRecursive does a DFS to find the leaf key and collect path levels.
// Returns the path levels (root to leaf's parent) and whether the key was
// found.
func findPathRecursive(node *TreeNode, leafKey *btcec.PublicKey,
	depth TreeDepth) ([]PathLevel, bool) {

	// If this is a leaf node, check if it matches.
	if node.isLeaf() {
		if node.PubKey.IsEqual(leafKey) {
			return nil, true
		}

		return nil, false
	}

	// If this is an opaque node, we can't search inside it.
	if node.Opaque {
		return nil, false
	}

	// This is an internal node. Search each child.
	if node.aggKey == nil {
		return nil, false
	}

	// Collect child keys for coefficient computation. We need the
	// potentially-sorted key list.
	childKeys := make([]*btcec.PublicKey, len(node.Children))
	for i, child := range node.Children {
		childKeys[i] = child.effectiveKey()
	}

	// If keys are sorted, sort them to match the aggregation order.
	if node.opts.sortKeys {
		childKeys = sortKeys(childKeys)
	}

	for _, child := range node.Children {
		subPath, found := findPathRecursive(
			child, leafKey, depth+1,
		)
		if !found {
			continue
		}

		// Compute the key aggregation coefficient for this child at
		// this level.
		childEffKey := child.effectiveKey()
		a := aggregationCoefficient(
			childKeys, childEffKey, node.keysHash,
			node.uniqueKeyIdx,
		)

		// Build the path level for this node.
		level := PathLevel{
			AggKey:    node.PubKey,
			ParityAcc: node.parityAcc,
			TweakAcc:  node.tweakAcc,
			Depth:     depth,
		}
		level.KeyCoeff.Set(a)

		// Append this level after the sub-path (we build leaf-first,
		// then reverse in FindPath to get root-first order). This
		// avoids O(depth²) from repeated prepend allocations.
		return append(subPath, level), true
	}

	return nil, false
}

// NestedSign generates a partial signature for a leaf signer in a nested
// MuSig2 tree.
//
// The partial signature is computed as:
//
//	s = k1 + b_path * k2 + e * d_eff
//
// where d_eff = g_root * accParity * a_path * privKey, and b_path is the
// product of all nonce coefficients from root to leaf.
func NestedSign(secNonce [SecNonceSize]byte, privKey *btcec.PrivateKey,
	aPath, bPath *btcec.ModNScalar, combinedNonce [PubNonceSize]byte,
	rootKey *btcec.PublicKey, msg [32]byte,
	accParity *btcec.ModNScalar) (*PartialSignature, error) {

	// Check that our signing key belongs to the secNonce.
	if !bytes.Equal(
		secNonce[btcec.PrivKeyBytesLen*2:],
		privKey.PubKey().SerializeCompressed(),
	) {
		return nil, ErrSecNoncePubkey
	}

	// Compute the root-level nonce blinder b_0 and the effective nonce
	// R.
	nonce, _, err := computeSigningNonce(
		combinedNonce, rootKey, msg,
	)
	if err != nil {
		return nil, err
	}

	// Parse secret nonces.
	var k1, k2 btcec.ModNScalar
	k1.SetByteSlice(secNonce[:btcec.PrivKeyBytesLen])
	k2.SetByteSlice(secNonce[btcec.PrivKeyBytesLen : btcec.PrivKeyBytesLen*2])

	if k1.IsZero() || k2.IsZero() {
		return nil, ErrSecretNonceZero
	}

	nonce.ToAffine()
	nonceKey := btcec.NewPublicKey(&nonce.X, &nonce.Y)

	// If the nonce R has an odd y coordinate, negate both secret nonces.
	if nonce.Y.IsOdd() {
		k1.Negate()
		k2.Negate()
	}

	privKeyScalar := privKey.Key
	if privKeyScalar.IsZero() {
		return nil, ErrPrivKeyZero
	}

	// Determine if the root combined key has odd y. If so, negate the
	// key contribution.
	combinedKeyBytes := rootKey.SerializeCompressed()
	combinedKeyYIsOdd := combinedKeyBytes[0] == secp.PubKeyFormatCompressedOdd

	parityCombinedKey := new(btcec.ModNScalar).SetInt(1)
	if combinedKeyYIsOdd {
		parityCombinedKey.Negate()
	}

	// Compute d_eff = g_root * accParity * a_path * sk.
	privKeyScalar.Mul(parityCombinedKey).Mul(accParity).Mul(aPath)

	// Compute the challenge: e = H("BIP0340/challenge", R || X || m).
	var challengeMsg bytes.Buffer
	challengeMsg.Write(schnorr.SerializePubKey(nonceKey))
	challengeMsg.Write(schnorr.SerializePubKey(rootKey))
	challengeMsg.Write(msg[:])
	challengeBytes := chainhash.TaggedHash(
		ChallengeHashTag, challengeMsg.Bytes(),
	)
	var e btcec.ModNScalar
	e.SetByteSlice(challengeBytes[:])

	// s = k1 + b_path * k2 + e * d_eff.
	s := new(btcec.ModNScalar)
	s.Add(&k1).Add(k2.Mul(bPath)).Add(e.Mul(&privKeyScalar))

	sig := NewPartialSignature(s, nonceKey)

	return &sig, nil
}

// AggregatePartialSigs sums partial signature scalars from children at one
// level: s_agg = sum(s_i) mod n.
func AggregatePartialSigs(
	sigs []*PartialSignature) *btcec.ModNScalar {

	result := new(btcec.ModNScalar)
	for _, sig := range sigs {
		result.Add(sig.S)
	}

	return result
}
