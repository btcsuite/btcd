// Copyright 2013-2025 The btcsuite developers

package musig2

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/stretchr/testify/require"
)

// genTestKey generates a deterministic private key for testing based on a
// seed index.
func genTestKey(t *testing.T, seed byte) *btcec.PrivateKey {
	t.Helper()

	var keyBytes [32]byte
	keyBytes[0] = seed
	keyBytes[31] = seed
	h := sha256.Sum256(keyBytes[:])

	privKey, _ := btcec.PrivKeyFromBytes(h[:])

	return privKey
}

// TestAggregateTree verifies that recursive tree key aggregation matches
// manual step-by-step aggregation using AggregateKeys directly.
func TestAggregateTree(t *testing.T) {
	t.Parallel()

	// Generate three keys: Alice, Bob, Carol.
	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)
	carolPriv := genTestKey(t, 3)

	alicePk := alicePriv.PubKey()
	bobPk := bobPriv.PubKey()
	carolPk := carolPriv.PubKey()

	// Build tree: Group(Group(Alice, Bob), Carol) with sorting.
	tree := Group([]*TreeNode{
		Group([]*TreeNode{Leaf(alicePk), Leaf(bobPk)}, WithKeySort()),
		Leaf(carolPk),
	}, WithKeySort())

	// Aggregate the tree.
	rootKey, err := AggregateTree(tree)
	require.NoError(t, err)
	require.NotNil(t, rootKey)

	// Manually compute the same: first aggregate Alice+Bob.
	abAggKey, _, _, err := AggregateKeys(
		[]*btcec.PublicKey{alicePk, bobPk}, true,
	)
	require.NoError(t, err)

	// Then aggregate (AB_agg, Carol).
	rootManual, _, _, err := AggregateKeys(
		[]*btcec.PublicKey{abAggKey.FinalKey, carolPk}, true,
	)
	require.NoError(t, err)

	// The root keys should match.
	require.True(t, rootKey.FinalKey.IsEqual(rootManual.FinalKey),
		"tree-aggregated root key should match manual aggregation")

	// Verify internal node state is populated.
	abNode := tree.Children[0]
	require.NotNil(t, abNode.aggKey,
		"AB node should have aggKey populated")
	require.True(t, abNode.aggKey.FinalKey.IsEqual(abAggKey.FinalKey),
		"AB node aggKey should match manual AB aggregation")
	require.NotNil(t, abNode.keysHash,
		"AB node should have keysHash populated")
}

// TestAggregateTreeWithOpaque verifies that replacing a subtree with an
// OpaqueNode produces the same root aggregate key.
func TestAggregateTreeWithOpaque(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)
	carolPriv := genTestKey(t, 3)

	alicePk := alicePriv.PubKey()
	bobPk := bobPriv.PubKey()
	carolPk := carolPriv.PubKey()

	// Full tree: Group(Group(Alice, Bob), Carol).
	fullTree := Group([]*TreeNode{
		Group([]*TreeNode{Leaf(alicePk), Leaf(bobPk)}, WithKeySort()),
		Leaf(carolPk),
	}, WithKeySort())

	fullRootKey, err := AggregateTree(fullTree)
	require.NoError(t, err)

	// Compute AB aggregate key separately.
	abAggKey, _, _, err := AggregateKeys(
		[]*btcec.PublicKey{alicePk, bobPk}, true,
	)
	require.NoError(t, err)

	// Partial tree with opaque AB node.
	opaqueTree := Group([]*TreeNode{
		OpaqueNode(abAggKey.FinalKey),
		Leaf(carolPk),
	}, WithKeySort())

	opaqueRootKey, err := AggregateTree(opaqueTree)
	require.NoError(t, err)

	require.True(t, fullRootKey.FinalKey.IsEqual(opaqueRootKey.FinalKey),
		"opaque tree root key should match full tree root key")
}

// TestProjectNonces verifies that ProjectNonces correctly computes
// (R'_1, b_d * R'_2) and that b_d matches the expected hash.
func TestProjectNonces(t *testing.T) {
	t.Parallel()

	// Generate two signers' nonces and aggregate them.
	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)

	aliceNonces, err := GenNonces(
		WithPublicKey(alicePriv.PubKey()),
		WithNonceSecretKeyAux(alicePriv),
	)
	require.NoError(t, err)

	bobNonces, err := GenNonces(
		WithPublicKey(bobPriv.PubKey()),
		WithNonceSecretKeyAux(bobPriv),
	)
	require.NoError(t, err)

	// Aggregate their nonces (standard SignAgg).
	aggNonce, err := AggregateNonces(
		[][PubNonceSize]byte{aliceNonces.PubNonce, bobNonces.PubNonce},
	)
	require.NoError(t, err)

	// Get the group's aggregate key.
	groupKey, _, _, err := AggregateKeys(
		[]*btcec.PublicKey{alicePriv.PubKey(), bobPriv.PubKey()}, true,
	)
	require.NoError(t, err)

	// Project the nonces.
	projNonce, bd, err := ProjectNonces(aggNonce, groupKey.FinalKey)
	require.NoError(t, err)
	require.NotNil(t, bd)

	// Verify R'_1 is unchanged.
	require.True(t,
		bytes.Equal(
			projNonce[:btcec.PubKeyBytesLenCompressed],
			aggNonce[:btcec.PubKeyBytesLenCompressed],
		),
		"R'_1 should be unchanged in projection",
	)

	// Verify b_d matches manual computation: H("MuSig/noncecoef", X || R'_agg).
	var hashBuf bytes.Buffer
	hashBuf.Write(schnorr.SerializePubKey(groupKey.FinalKey))
	hashBuf.Write(aggNonce[:])
	expectedHash := chainhash.TaggedHash(
		NonceBlindTag, hashBuf.Bytes(),
	)
	var expectedBd btcec.ModNScalar
	expectedBd.SetByteSlice(expectedHash[:])
	require.True(t, bd.Equals(&expectedBd),
		"b_d should match manual hash computation")

	// Verify R'_2 was actually multiplied by b_d.
	r2, err := btcec.ParsePubKey(
		aggNonce[btcec.PubKeyBytesLenCompressed:],
	)
	require.NoError(t, err)

	var r2J btcec.JacobianPoint
	r2.AsJacobian(&r2J)
	btcec.ScalarMultNonConst(bd, &r2J, &r2J)
	r2J.ToAffine()
	expectedR2 := btcec.NewPublicKey(&r2J.X, &r2J.Y)

	projR2, err := btcec.ParsePubKey(
		projNonce[btcec.PubKeyBytesLenCompressed:],
	)
	require.NoError(t, err)

	require.True(t, expectedR2.IsEqual(projR2),
		"projected R'_2 should equal b_d * R'_2")
}

// TestFindPath verifies that FindPath correctly locates a leaf and returns
// the right path coefficients.
func TestFindPath(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)
	carolPriv := genTestKey(t, 3)
	davePriv := genTestKey(t, 4)

	alicePk := alicePriv.PubKey()
	bobPk := bobPriv.PubKey()
	carolPk := carolPriv.PubKey()
	davePk := davePriv.PubKey()

	// Depth-2 tree: Group(Group(Group(Alice, Bob), Carol), Dave).
	tree := Group([]*TreeNode{
		Group([]*TreeNode{
			Group([]*TreeNode{
				Leaf(alicePk), Leaf(bobPk),
			}, WithKeySort()),
			Leaf(carolPk),
		}, WithKeySort()),
		Leaf(davePk),
	}, WithKeySort())

	_, err := AggregateTree(tree)
	require.NoError(t, err)

	// Find Alice's path (depth 2).
	alicePath, err := tree.FindPath(alicePk)
	require.NoError(t, err)
	require.Equal(t, 3, len(alicePath.Levels),
		"Alice should have 3 path levels (root, mid, AB)")
	require.True(t, alicePath.LeafPubKey.IsEqual(alicePk))

	// Verify depths.
	require.Equal(t, TreeDepth(0), alicePath.Levels[0].Depth)
	require.Equal(t, TreeDepth(1), alicePath.Levels[1].Depth)
	require.Equal(t, TreeDepth(2), alicePath.Levels[2].Depth)

	// Verify APath is non-zero.
	aPath := alicePath.APath()
	require.False(t, aPath.IsZero(), "a_path should be non-zero")

	// Find Dave's path (depth 0 child).
	davePath, err := tree.FindPath(davePk)
	require.NoError(t, err)
	require.Equal(t, 1, len(davePath.Levels),
		"Dave should have 1 path level (root only)")
	require.Equal(t, TreeDepth(0), davePath.Levels[0].Depth)

	// Try to find a key that's not in the tree.
	unknownPriv := genTestKey(t, 99)
	_, err = tree.FindPath(unknownPriv.PubKey())
	require.ErrorIs(t, err, ErrLeafNotFound)
}

// TestFindPathWithOpaque verifies that FindPath returns an error when
// the target key would be inside an opaque subtree.
func TestFindPathWithOpaque(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)
	carolPriv := genTestKey(t, 3)

	alicePk := alicePriv.PubKey()
	bobPk := bobPriv.PubKey()
	carolPk := carolPriv.PubKey()

	// Compute AB aggregate key.
	abAggKey, _, _, err := AggregateKeys(
		[]*btcec.PublicKey{alicePk, bobPk}, true,
	)
	require.NoError(t, err)

	// Tree with opaque AB node.
	tree := Group([]*TreeNode{
		OpaqueNode(abAggKey.FinalKey),
		Leaf(carolPk),
	}, WithKeySort())

	_, err = AggregateTree(tree)
	require.NoError(t, err)

	// Carol should be findable.
	carolPath, err := tree.FindPath(carolPk)
	require.NoError(t, err)
	require.Equal(t, 1, len(carolPath.Levels))

	// Alice should NOT be findable (she's inside the opaque node).
	_, err = tree.FindPath(alicePk)
	require.ErrorIs(t, err, ErrLeafNotFound)
}

// TestAggregateTreeDepth2 tests a three-level tree.
func TestAggregateTreeDepth2(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)
	carolPriv := genTestKey(t, 3)
	davePriv := genTestKey(t, 4)

	alicePk := alicePriv.PubKey()
	bobPk := bobPriv.PubKey()
	carolPk := carolPriv.PubKey()
	davePk := davePriv.PubKey()

	// Tree: Group(Group(Group(Alice, Bob), Carol), Dave).
	tree := Group([]*TreeNode{
		Group([]*TreeNode{
			Group([]*TreeNode{
				Leaf(alicePk), Leaf(bobPk),
			}, WithKeySort()),
			Leaf(carolPk),
		}, WithKeySort()),
		Leaf(davePk),
	}, WithKeySort())

	rootKey, err := AggregateTree(tree)
	require.NoError(t, err)

	// Manual step-by-step:
	abKey, _, _, err := AggregateKeys(
		[]*btcec.PublicKey{alicePk, bobPk}, true,
	)
	require.NoError(t, err)

	abcKey, _, _, err := AggregateKeys(
		[]*btcec.PublicKey{abKey.FinalKey, carolPk}, true,
	)
	require.NoError(t, err)

	rootManual, _, _, err := AggregateKeys(
		[]*btcec.PublicKey{abcKey.FinalKey, davePk}, true,
	)
	require.NoError(t, err)

	require.True(t, rootKey.FinalKey.IsEqual(rootManual.FinalKey),
		"depth-2 tree root should match manual aggregation")
}

// TestNestedDepth1 is the core integration test: Alice+Bob group signing with
// Carol, verifying the final signature is a valid BIP-340 Schnorr signature.
func TestNestedDepth1(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)
	carolPriv := genTestKey(t, 3)

	alicePk := alicePriv.PubKey()
	bobPk := bobPriv.PubKey()
	carolPk := carolPriv.PubKey()

	// Declare and aggregate the tree.
	tree := Group([]*TreeNode{
		Group([]*TreeNode{Leaf(alicePk), Leaf(bobPk)}, WithKeySort()),
		Leaf(carolPk),
	}, WithKeySort())

	rootKey, err := AggregateTree(tree)
	require.NoError(t, err)

	// Create sessions for each leaf signer.
	aliceSess, err := NewNestedSession(alicePriv, tree)
	require.NoError(t, err)

	bobSess, err := NewNestedSession(bobPriv, tree)
	require.NoError(t, err)

	carolSess, err := NewNestedSession(carolPriv, tree)
	require.NoError(t, err)

	// Round 1 — Nonce exchange. AB coordinator creates aggregator for
	// the AB subtree.
	abNode := tree.Children[0]
	abAgg, err := NewAggregator(abNode)
	require.NoError(t, err)

	_, err = abAgg.RegisterNonce(alicePk, aliceSess.PublicNonce())
	require.NoError(t, err)

	allNonces, err := abAgg.RegisterNonce(bobPk, bobSess.PublicNonce())
	require.NoError(t, err)
	require.True(t, allNonces, "should have all AB nonces")

	abProjected, err := abAgg.ProjectedNonce()
	require.NoError(t, err)

	// Root coordinator.
	rootAgg, err := NewAggregator(tree)
	require.NoError(t, err)

	_, err = rootAgg.RegisterNonce(
		abNode.effectiveKey(), abProjected,
	)
	require.NoError(t, err)

	allNonces, err = rootAgg.RegisterNonce(
		carolPk, carolSess.PublicNonce(),
	)
	require.NoError(t, err)
	require.True(t, allNonces, "should have all root nonces")

	rootCombined, err := rootAgg.AggNonce()
	require.NoError(t, err)

	// Round 2 — Signing (message known).
	msg := sha256.Sum256([]byte("payment"))

	abBd, err := abAgg.NonceCoeff()
	require.NoError(t, err)

	aliceSig, err := aliceSess.Sign(msg, rootCombined,
		map[TreeDepth]*btcec.ModNScalar{1: abBd})
	require.NoError(t, err)

	bobSig, err := bobSess.Sign(msg, rootCombined,
		map[TreeDepth]*btcec.ModNScalar{1: abBd})
	require.NoError(t, err)

	carolSig, err := carolSess.Sign(msg, rootCombined, nil)
	require.NoError(t, err)

	// Aggregate partial signatures up the tree.
	_, err = abAgg.RegisterSig(alicePk, aliceSig)
	require.NoError(t, err)

	allSigs, err := abAgg.RegisterSig(bobPk, bobSig)
	require.NoError(t, err)
	require.True(t, allSigs, "should have all AB sigs")

	abPartial, err := abAgg.AsPartialSig()
	require.NoError(t, err)

	_, err = rootAgg.RegisterSig(abNode.effectiveKey(), abPartial)
	require.NoError(t, err)

	allSigs, err = rootAgg.RegisterSig(carolPk, carolSig)
	require.NoError(t, err)
	require.True(t, allSigs, "should have all root sigs")

	// Produce the final Schnorr signature.
	finalSig, err := rootAgg.FinalSig(msg)
	require.NoError(t, err)
	require.NotNil(t, finalSig)

	// Verify against the root key using standard BIP-340.
	require.True(t, finalSig.Verify(msg[:], rootKey.FinalKey),
		"final nested signature should verify against root key")
}

// TestNestedNonceReuse verifies that signing twice with the same session
// returns an error.
func TestNestedNonceReuse(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)

	tree := Group([]*TreeNode{
		Leaf(alicePriv.PubKey()),
		Leaf(bobPriv.PubKey()),
	}, WithKeySort())

	_, err := AggregateTree(tree)
	require.NoError(t, err)

	sess, err := NewNestedSession(alicePriv, tree)
	require.NoError(t, err)

	msg := sha256.Sum256([]byte("test"))
	var rootNonce [PubNonceSize]byte // Dummy for this test.

	// First sign should work (may fail on nonce computation, but won't be
	// a reuse error).
	_, _ = sess.Sign(msg, rootNonce, nil)

	// Second sign should return reuse error.
	_, err = sess.Sign(msg, rootNonce, nil)
	require.ErrorIs(t, err, ErrNestedSessionReuse)
}

// TestAggregatorPubKeyRegistration verifies error cases for the Aggregator's
// pubkey-based registration.
func TestAggregatorPubKeyRegistration(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)
	carolPriv := genTestKey(t, 3)

	alicePk := alicePriv.PubKey()
	bobPk := bobPriv.PubKey()
	carolPk := carolPriv.PubKey()

	tree := Group([]*TreeNode{
		Leaf(alicePk),
		Leaf(bobPk),
	}, WithKeySort())

	_, err := AggregateTree(tree)
	require.NoError(t, err)

	agg, err := NewAggregator(tree)
	require.NoError(t, err)

	var dummyNonce [PubNonceSize]byte

	// Unknown key should fail.
	_, err = agg.RegisterNonce(carolPk, dummyNonce)
	require.ErrorIs(t, err, ErrChildKeyNotFound)

	// Register Alice.
	_, err = agg.RegisterNonce(alicePk, dummyNonce)
	require.NoError(t, err)

	// Duplicate registration should fail.
	_, err = agg.RegisterNonce(alicePk, dummyNonce)
	require.ErrorIs(t, err, ErrDuplicateNonce)

	// Trying to create aggregator from a leaf should fail.
	_, err = NewAggregator(Leaf(alicePk))
	require.ErrorIs(t, err, ErrNodeNotInternal)
}

// TestNestedDepth1ContextAPI repeats the core integration test (Alice+Bob
// group with Carol) but using the managed Context/Session API with
// WithCosignerTree and WithNestedCoeffs.
func TestNestedDepth1ContextAPI(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)
	carolPriv := genTestKey(t, 3)

	alicePk := alicePriv.PubKey()
	bobPk := bobPriv.PubKey()
	carolPk := carolPriv.PubKey()

	// Declare and aggregate the tree.
	tree := Group([]*TreeNode{
		Group([]*TreeNode{Leaf(alicePk), Leaf(bobPk)}, WithKeySort()),
		Leaf(carolPk),
	}, WithKeySort())

	rootKey, err := AggregateTree(tree)
	require.NoError(t, err)

	// Create contexts and sessions via managed API.
	aliceCtx, err := NewContext(alicePriv, false,
		WithCosignerTree(tree),
	)
	require.NoError(t, err)

	aliceSess, err := aliceCtx.NewSession()
	require.NoError(t, err)

	bobCtx, err := NewContext(bobPriv, false,
		WithCosignerTree(tree),
	)
	require.NoError(t, err)

	bobSess, err := bobCtx.NewSession()
	require.NoError(t, err)

	carolCtx, err := NewContext(carolPriv, false,
		WithCosignerTree(tree),
	)
	require.NoError(t, err)

	carolSess, err := carolCtx.NewSession()
	require.NoError(t, err)

	// Round 1 — Nonce exchange.
	abNode := tree.Children[0]
	abAgg, err := NewAggregator(abNode)
	require.NoError(t, err)

	_, err = abAgg.RegisterNonce(alicePk, aliceSess.PublicNonce())
	require.NoError(t, err)

	allNonces, err := abAgg.RegisterNonce(bobPk, bobSess.PublicNonce())
	require.NoError(t, err)
	require.True(t, allNonces)

	abProjected, err := abAgg.ProjectedNonce()
	require.NoError(t, err)

	rootAgg, err := NewAggregator(tree)
	require.NoError(t, err)

	_, err = rootAgg.RegisterNonce(abNode.effectiveKey(), abProjected)
	require.NoError(t, err)

	allNonces, err = rootAgg.RegisterNonce(carolPk, carolSess.PublicNonce())
	require.NoError(t, err)
	require.True(t, allNonces)

	rootCombined, err := rootAgg.AggNonce()
	require.NoError(t, err)

	// Round 2 — Signing. Distribute root combined nonce to all signers.
	msg := sha256.Sum256([]byte("context api test"))

	require.NoError(t, aliceSess.RegisterCombinedNonce(rootCombined))
	require.NoError(t, bobSess.RegisterCombinedNonce(rootCombined))
	require.NoError(t, carolSess.RegisterCombinedNonce(rootCombined))

	abBd, err := abAgg.NonceCoeff()
	require.NoError(t, err)

	// Alice and Bob sign with nested coefficients.
	aliceSig, err := aliceSess.Sign(msg,
		WithNestedCoeffs(map[TreeDepth]*btcec.ModNScalar{1: abBd}),
	)
	require.NoError(t, err)

	bobSig, err := bobSess.Sign(msg,
		WithNestedCoeffs(map[TreeDepth]*btcec.ModNScalar{1: abBd}),
	)
	require.NoError(t, err)

	// Carol signs at root level (no nested coefficients).
	carolSig, err := carolSess.Sign(msg)
	require.NoError(t, err)

	// Aggregate partial signatures up the tree.
	_, err = abAgg.RegisterSig(alicePk, aliceSig)
	require.NoError(t, err)

	allSigs, err := abAgg.RegisterSig(bobPk, bobSig)
	require.NoError(t, err)
	require.True(t, allSigs)

	abPartial, err := abAgg.AsPartialSig()
	require.NoError(t, err)

	_, err = rootAgg.RegisterSig(abNode.effectiveKey(), abPartial)
	require.NoError(t, err)

	allSigs, err = rootAgg.RegisterSig(carolPk, carolSig)
	require.NoError(t, err)
	require.True(t, allSigs)

	// Produce the final Schnorr signature.
	finalSig, err := rootAgg.FinalSig(msg)
	require.NoError(t, err)
	require.NotNil(t, finalSig)

	// Verify against the root key using standard BIP-340.
	require.True(t, finalSig.Verify(msg[:], rootKey.FinalKey),
		"Context/Session API nested signature should verify")
}

// TestNestedDepth2 tests a three-level nesting: ((Alice+Bob)+Carol)+Dave.
func TestNestedDepth2(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)
	carolPriv := genTestKey(t, 3)
	davePriv := genTestKey(t, 4)

	alicePk := alicePriv.PubKey()
	bobPk := bobPriv.PubKey()
	carolPk := carolPriv.PubKey()
	davePk := davePriv.PubKey()

	// Tree: Group(Group(Group(Alice, Bob), Carol), Dave).
	abNode := Group(
		[]*TreeNode{Leaf(alicePk), Leaf(bobPk)}, WithKeySort(),
	)
	abcNode := Group(
		[]*TreeNode{abNode, Leaf(carolPk)}, WithKeySort(),
	)
	tree := Group(
		[]*TreeNode{abcNode, Leaf(davePk)}, WithKeySort(),
	)

	rootKey, err := AggregateTree(tree)
	require.NoError(t, err)

	// Create sessions.
	aliceSess, err := NewNestedSession(alicePriv, tree)
	require.NoError(t, err)

	bobSess, err := NewNestedSession(bobPriv, tree)
	require.NoError(t, err)

	carolSess, err := NewNestedSession(carolPriv, tree)
	require.NoError(t, err)

	daveSess, err := NewNestedSession(davePriv, tree)
	require.NoError(t, err)

	// Round 1: Nonce exchange — bottom-up.

	// AB aggregator (depth 2).
	abAgg, err := NewAggregator(abNode)
	require.NoError(t, err)

	_, err = abAgg.RegisterNonce(alicePk, aliceSess.PublicNonce())
	require.NoError(t, err)
	allNonces, err := abAgg.RegisterNonce(
		bobPk, bobSess.PublicNonce(),
	)
	require.NoError(t, err)
	require.True(t, allNonces)

	abProjected, err := abAgg.ProjectedNonce()
	require.NoError(t, err)

	// ABC aggregator (depth 1).
	abcAgg, err := NewAggregator(abcNode)
	require.NoError(t, err)

	_, err = abcAgg.RegisterNonce(
		abNode.effectiveKey(), abProjected,
	)
	require.NoError(t, err)
	allNonces, err = abcAgg.RegisterNonce(
		carolPk, carolSess.PublicNonce(),
	)
	require.NoError(t, err)
	require.True(t, allNonces)

	abcProjected, err := abcAgg.ProjectedNonce()
	require.NoError(t, err)

	// Root aggregator (depth 0).
	rootAgg, err := NewAggregator(tree)
	require.NoError(t, err)

	_, err = rootAgg.RegisterNonce(
		abcNode.effectiveKey(), abcProjected,
	)
	require.NoError(t, err)
	allNonces, err = rootAgg.RegisterNonce(
		davePk, daveSess.PublicNonce(),
	)
	require.NoError(t, err)
	require.True(t, allNonces)

	rootCombined, err := rootAgg.AggNonce()
	require.NoError(t, err)

	// Round 2: Signing.
	msg := sha256.Sum256([]byte("depth-2 test"))

	abBd, err := abAgg.NonceCoeff()
	require.NoError(t, err)

	abcBd, err := abcAgg.NonceCoeff()
	require.NoError(t, err)

	// Alice and Bob need both nested coefficients (depth 1 and 2).
	nestedCoeffsAB := map[TreeDepth]*btcec.ModNScalar{
		1: abcBd,
		2: abBd,
	}

	aliceSig, err := aliceSess.Sign(msg, rootCombined, nestedCoeffsAB)
	require.NoError(t, err)

	bobSig, err := bobSess.Sign(msg, rootCombined, nestedCoeffsAB)
	require.NoError(t, err)

	// Carol needs only depth 1 coefficient.
	carolSig, err := carolSess.Sign(msg, rootCombined,
		map[TreeDepth]*btcec.ModNScalar{1: abcBd},
	)
	require.NoError(t, err)

	// Dave at root level.
	daveSig, err := daveSess.Sign(msg, rootCombined, nil)
	require.NoError(t, err)

	// Signature aggregation — bottom-up.

	// AB aggregation.
	_, err = abAgg.RegisterSig(alicePk, aliceSig)
	require.NoError(t, err)
	allSigs, err := abAgg.RegisterSig(bobPk, bobSig)
	require.NoError(t, err)
	require.True(t, allSigs)

	abPartial, err := abAgg.AsPartialSig()
	require.NoError(t, err)

	// ABC aggregation.
	_, err = abcAgg.RegisterSig(abNode.effectiveKey(), abPartial)
	require.NoError(t, err)
	allSigs, err = abcAgg.RegisterSig(carolPk, carolSig)
	require.NoError(t, err)
	require.True(t, allSigs)

	abcPartial, err := abcAgg.AsPartialSig()
	require.NoError(t, err)

	// Root aggregation.
	_, err = rootAgg.RegisterSig(abcNode.effectiveKey(), abcPartial)
	require.NoError(t, err)
	allSigs, err = rootAgg.RegisterSig(davePk, daveSig)
	require.NoError(t, err)
	require.True(t, allSigs)

	// Final signature.
	finalSig, err := rootAgg.FinalSig(msg)
	require.NoError(t, err)

	// Verify with standard BIP-340.
	require.True(t, finalSig.Verify(msg[:], rootKey.FinalKey),
		"depth-2 nested signature should verify")
}

// TestNestedFlat verifies that a single-level tree (all signers at root)
// produces a valid signature, equivalent to standard MuSig2.
func TestNestedFlat(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)
	carolPriv := genTestKey(t, 3)

	alicePk := alicePriv.PubKey()
	bobPk := bobPriv.PubKey()
	carolPk := carolPriv.PubKey()

	// Flat tree: all leaves at root.
	tree := Group([]*TreeNode{
		Leaf(alicePk), Leaf(bobPk), Leaf(carolPk),
	}, WithKeySort())

	rootKey, err := AggregateTree(tree)
	require.NoError(t, err)

	// Also compute the standard key aggregation for comparison.
	stdKey, _, _, err := AggregateKeys(
		[]*btcec.PublicKey{alicePk, bobPk, carolPk}, true,
	)
	require.NoError(t, err)

	require.True(t, rootKey.FinalKey.IsEqual(stdKey.FinalKey),
		"flat tree root key should match standard key aggregation")

	// Create sessions.
	aliceSess, err := NewNestedSession(alicePriv, tree)
	require.NoError(t, err)

	bobSess, err := NewNestedSession(bobPriv, tree)
	require.NoError(t, err)

	carolSess, err := NewNestedSession(carolPriv, tree)
	require.NoError(t, err)

	// Round 1: Nonce exchange at root level only.
	rootAgg, err := NewAggregator(tree)
	require.NoError(t, err)

	_, err = rootAgg.RegisterNonce(alicePk, aliceSess.PublicNonce())
	require.NoError(t, err)
	_, err = rootAgg.RegisterNonce(bobPk, bobSess.PublicNonce())
	require.NoError(t, err)
	allNonces, err := rootAgg.RegisterNonce(
		carolPk, carolSess.PublicNonce(),
	)
	require.NoError(t, err)
	require.True(t, allNonces)

	rootCombined, err := rootAgg.AggNonce()
	require.NoError(t, err)

	// Round 2: Signing — no nested coefficients needed.
	msg := sha256.Sum256([]byte("flat tree test"))

	aliceSig, err := aliceSess.Sign(msg, rootCombined, nil)
	require.NoError(t, err)

	bobSig, err := bobSess.Sign(msg, rootCombined, nil)
	require.NoError(t, err)

	carolSig, err := carolSess.Sign(msg, rootCombined, nil)
	require.NoError(t, err)

	// Aggregation.
	_, err = rootAgg.RegisterSig(alicePk, aliceSig)
	require.NoError(t, err)
	_, err = rootAgg.RegisterSig(bobPk, bobSig)
	require.NoError(t, err)
	allSigs, err := rootAgg.RegisterSig(carolPk, carolSig)
	require.NoError(t, err)
	require.True(t, allSigs)

	// Final signature.
	finalSig, err := rootAgg.FinalSig(msg)
	require.NoError(t, err)

	require.True(t, finalSig.Verify(msg[:], rootKey.FinalKey),
		"flat tree nested signature should verify")
}

// TestNestedTaproot verifies nested signing with a taproot tweak applied at
// the root node.
func TestNestedTaproot(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)
	carolPriv := genTestKey(t, 3)

	alicePk := alicePriv.PubKey()
	bobPk := bobPriv.PubKey()
	carolPk := carolPriv.PubKey()

	// Use a dummy script root for the taproot tweak.
	scriptRoot := sha256.Sum256([]byte("taproot script root"))

	// Tree with taproot tweak at root.
	tree := Group([]*TreeNode{
		Group([]*TreeNode{Leaf(alicePk), Leaf(bobPk)}, WithKeySort()),
		Leaf(carolPk),
	}, WithKeySort(), WithTaprootNodeTweak(scriptRoot[:]))

	rootKey, err := AggregateTree(tree)
	require.NoError(t, err)

	// Verify that we can access the pre-tweaked key.
	require.NotNil(t, rootKey.PreTweakedKey,
		"taproot tree should have pre-tweaked key")
	require.False(t, rootKey.FinalKey.IsEqual(rootKey.PreTweakedKey),
		"tweaked output key should differ from internal key")

	// Create sessions using standalone API.
	aliceSess, err := NewNestedSession(alicePriv, tree)
	require.NoError(t, err)

	bobSess, err := NewNestedSession(bobPriv, tree)
	require.NoError(t, err)

	carolSess, err := NewNestedSession(carolPriv, tree)
	require.NoError(t, err)

	// Round 1: Nonce exchange.
	abNode := tree.Children[0]
	abAgg, err := NewAggregator(abNode)
	require.NoError(t, err)

	_, err = abAgg.RegisterNonce(alicePk, aliceSess.PublicNonce())
	require.NoError(t, err)
	allNonces, err := abAgg.RegisterNonce(
		bobPk, bobSess.PublicNonce(),
	)
	require.NoError(t, err)
	require.True(t, allNonces)

	abProjected, err := abAgg.ProjectedNonce()
	require.NoError(t, err)

	rootAgg, err := NewAggregator(tree)
	require.NoError(t, err)

	_, err = rootAgg.RegisterNonce(abNode.effectiveKey(), abProjected)
	require.NoError(t, err)
	allNonces, err = rootAgg.RegisterNonce(
		carolPk, carolSess.PublicNonce(),
	)
	require.NoError(t, err)
	require.True(t, allNonces)

	rootCombined, err := rootAgg.AggNonce()
	require.NoError(t, err)

	// Round 2: Signing.
	msg := sha256.Sum256([]byte("taproot nested test"))

	abBd, err := abAgg.NonceCoeff()
	require.NoError(t, err)

	aliceSig, err := aliceSess.Sign(msg, rootCombined,
		map[TreeDepth]*btcec.ModNScalar{1: abBd},
	)
	require.NoError(t, err)

	bobSig, err := bobSess.Sign(msg, rootCombined,
		map[TreeDepth]*btcec.ModNScalar{1: abBd},
	)
	require.NoError(t, err)

	carolSig, err := carolSess.Sign(msg, rootCombined, nil)
	require.NoError(t, err)

	// Signature aggregation.
	_, err = abAgg.RegisterSig(alicePk, aliceSig)
	require.NoError(t, err)
	allSigs, err := abAgg.RegisterSig(bobPk, bobSig)
	require.NoError(t, err)
	require.True(t, allSigs)

	abPartial, err := abAgg.AsPartialSig()
	require.NoError(t, err)

	_, err = rootAgg.RegisterSig(abNode.effectiveKey(), abPartial)
	require.NoError(t, err)
	allSigs, err = rootAgg.RegisterSig(carolPk, carolSig)
	require.NoError(t, err)
	require.True(t, allSigs)

	// Final signature.
	finalSig, err := rootAgg.FinalSig(msg)
	require.NoError(t, err)

	// Verify against the tweaked output key.
	require.True(t, finalSig.Verify(msg[:], rootKey.FinalKey),
		"taproot nested signature should verify against output key")
}

// TestNestedBIP86 verifies nested signing with a BIP-86 tweak at the root
// node (key-only spend, no script tree).
func TestNestedBIP86(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)
	carolPriv := genTestKey(t, 3)

	alicePk := alicePriv.PubKey()
	bobPk := bobPriv.PubKey()
	carolPk := carolPriv.PubKey()

	// Tree with BIP-86 tweak at root.
	tree := Group([]*TreeNode{
		Group([]*TreeNode{Leaf(alicePk), Leaf(bobPk)}, WithKeySort()),
		Leaf(carolPk),
	}, WithKeySort(), WithBIP86NodeTweak())

	rootKey, err := AggregateTree(tree)
	require.NoError(t, err)

	// Should have pre-tweaked key.
	require.NotNil(t, rootKey.PreTweakedKey)

	// Create sessions.
	aliceSess, err := NewNestedSession(alicePriv, tree)
	require.NoError(t, err)

	bobSess, err := NewNestedSession(bobPriv, tree)
	require.NoError(t, err)

	carolSess, err := NewNestedSession(carolPriv, tree)
	require.NoError(t, err)

	// Nonce exchange.
	abNode := tree.Children[0]
	abAgg, err := NewAggregator(abNode)
	require.NoError(t, err)

	_, err = abAgg.RegisterNonce(alicePk, aliceSess.PublicNonce())
	require.NoError(t, err)
	_, err = abAgg.RegisterNonce(bobPk, bobSess.PublicNonce())
	require.NoError(t, err)

	abProjected, err := abAgg.ProjectedNonce()
	require.NoError(t, err)

	rootAgg, err := NewAggregator(tree)
	require.NoError(t, err)

	_, err = rootAgg.RegisterNonce(abNode.effectiveKey(), abProjected)
	require.NoError(t, err)
	_, err = rootAgg.RegisterNonce(carolPk, carolSess.PublicNonce())
	require.NoError(t, err)

	rootCombined, err := rootAgg.AggNonce()
	require.NoError(t, err)

	// Signing.
	msg := sha256.Sum256([]byte("bip86 nested test"))

	abBd, err := abAgg.NonceCoeff()
	require.NoError(t, err)

	aliceSig, err := aliceSess.Sign(msg, rootCombined,
		map[TreeDepth]*btcec.ModNScalar{1: abBd},
	)
	require.NoError(t, err)

	bobSig, err := bobSess.Sign(msg, rootCombined,
		map[TreeDepth]*btcec.ModNScalar{1: abBd},
	)
	require.NoError(t, err)

	carolSig, err := carolSess.Sign(msg, rootCombined, nil)
	require.NoError(t, err)

	// Aggregation.
	_, err = abAgg.RegisterSig(alicePk, aliceSig)
	require.NoError(t, err)
	_, err = abAgg.RegisterSig(bobPk, bobSig)
	require.NoError(t, err)

	abPartial, err := abAgg.AsPartialSig()
	require.NoError(t, err)

	_, err = rootAgg.RegisterSig(abNode.effectiveKey(), abPartial)
	require.NoError(t, err)
	_, err = rootAgg.RegisterSig(carolPk, carolSig)
	require.NoError(t, err)

	// Final signature.
	finalSig, err := rootAgg.FinalSig(msg)
	require.NoError(t, err)

	// Verify against the BIP-86 tweaked output key.
	require.True(t, finalSig.Verify(msg[:], rootKey.FinalKey),
		"BIP-86 nested signature should verify")
}

// TestNestedLargeTree tests a large tree with 10 groups of 10 signers each
// (100 total leaf signers).
func TestNestedLargeTree(t *testing.T) {
	t.Parallel()

	const numGroups = 10
	const signersPerGroup = 10

	// Generate all keys.
	type signerInfo struct {
		priv *btcec.PrivateKey
		pub  *btcec.PublicKey
	}

	signers := make([][]signerInfo, numGroups)
	groupNodes := make([]*TreeNode, numGroups)

	seed := byte(1)
	for g := 0; g < numGroups; g++ {
		signers[g] = make([]signerInfo, signersPerGroup)
		leaves := make([]*TreeNode, signersPerGroup)

		for s := 0; s < signersPerGroup; s++ {
			priv := genTestKey(t, seed)
			signers[g][s] = signerInfo{
				priv: priv,
				pub:  priv.PubKey(),
			}
			leaves[s] = Leaf(priv.PubKey())
			seed++
		}

		groupNodes[g] = Group(leaves, WithKeySort())
	}

	tree := Group(groupNodes, WithKeySort())

	rootKey, err := AggregateTree(tree)
	require.NoError(t, err)

	// Create sessions for all signers.
	sessions := make([][]*NestedSession, numGroups)
	for g := 0; g < numGroups; g++ {
		sessions[g] = make([]*NestedSession, signersPerGroup)
		for s := 0; s < signersPerGroup; s++ {
			sess, err := NewNestedSession(signers[g][s].priv, tree)
			require.NoError(t, err)
			sessions[g][s] = sess
		}
	}

	// Round 1: Nonce exchange.

	// Per-group aggregators.
	groupAggs := make([]*Aggregator, numGroups)
	for g := 0; g < numGroups; g++ {
		agg, err := NewAggregator(groupNodes[g])
		require.NoError(t, err)

		for s := 0; s < signersPerGroup; s++ {
			_, err = agg.RegisterNonce(
				signers[g][s].pub,
				sessions[g][s].PublicNonce(),
			)
			require.NoError(t, err)
		}

		groupAggs[g] = agg
	}

	// Root aggregator.
	rootAgg, err := NewAggregator(tree)
	require.NoError(t, err)

	for g := 0; g < numGroups; g++ {
		projected, err := groupAggs[g].ProjectedNonce()
		require.NoError(t, err)

		_, err = rootAgg.RegisterNonce(
			groupNodes[g].effectiveKey(), projected,
		)
		require.NoError(t, err)
	}

	rootCombined, err := rootAgg.AggNonce()
	require.NoError(t, err)

	// Round 2: Signing.
	msg := sha256.Sum256([]byte("large tree test"))

	for g := 0; g < numGroups; g++ {
		bd, err := groupAggs[g].NonceCoeff()
		require.NoError(t, err)

		coeffs := map[TreeDepth]*btcec.ModNScalar{1: bd}

		for s := 0; s < signersPerGroup; s++ {
			sig, err := sessions[g][s].Sign(
				msg, rootCombined, coeffs,
			)
			require.NoError(t, err)

			_, err = groupAggs[g].RegisterSig(
				signers[g][s].pub, sig,
			)
			require.NoError(t, err)
		}

		partial, err := groupAggs[g].AsPartialSig()
		require.NoError(t, err)

		_, err = rootAgg.RegisterSig(
			groupNodes[g].effectiveKey(), partial,
		)
		require.NoError(t, err)
	}

	// Final signature.
	finalSig, err := rootAgg.FinalSig(msg)
	require.NoError(t, err)

	require.True(t, finalSig.Verify(msg[:], rootKey.FinalKey),
		"100-signer nested signature should verify")
}

// TestNestedAsymmetric tests an unbalanced tree: one leaf at root alongside a
// deeply nested subtree (5 levels deep).
func TestNestedAsymmetric(t *testing.T) {
	t.Parallel()

	// Generate 6 keys: one for each level of depth, plus the root-level
	// leaf.
	keys := make([]*btcec.PrivateKey, 6)
	pubs := make([]*btcec.PublicKey, 6)
	for i := 0; i < 6; i++ {
		keys[i] = genTestKey(t, byte(i+1))
		pubs[i] = keys[i].PubKey()
	}

	// Build a right-leaning chain: key0 is always at the deepest leaf,
	// paired with key1, then that group is paired with key2, etc.
	// Key5 is the root-level sibling leaf.
	//
	// Tree: Group(Group(Group(Group(Group(key0, key1), key2), key3),
	//                    key4), key5)
	subtree := Group([]*TreeNode{
		Leaf(pubs[0]), Leaf(pubs[1]),
	}, WithKeySort())

	for i := 2; i < 5; i++ {
		subtree = Group([]*TreeNode{
			subtree, Leaf(pubs[i]),
		}, WithKeySort())
	}

	tree := Group([]*TreeNode{
		subtree, Leaf(pubs[5]),
	}, WithKeySort())

	rootKey, err := AggregateTree(tree)
	require.NoError(t, err)

	// Verify the deepest signer has the correct path depth.
	path0, err := tree.FindPath(pubs[0])
	require.NoError(t, err)
	require.Equal(t, 5, len(path0.Levels),
		"deepest signer should have 5 path levels")

	// Verify the root-level leaf has depth 1.
	path5, err := tree.FindPath(pubs[5])
	require.NoError(t, err)
	require.Equal(t, 1, len(path5.Levels),
		"root-level leaf should have 1 path level")

	// Create sessions for all signers.
	sessions := make([]*NestedSession, 6)
	for i := 0; i < 6; i++ {
		sess, err := NewNestedSession(keys[i], tree)
		require.NoError(t, err)
		sessions[i] = sess
	}

	// Collect all internal nodes for aggregators. We traverse the tree
	// to find them.
	type nodeLevel struct {
		node  *TreeNode
		depth TreeDepth
	}

	var internalNodes []nodeLevel
	var collectNodes func(n *TreeNode, d TreeDepth)
	collectNodes = func(n *TreeNode, d TreeDepth) {
		if n.isInternal() {
			internalNodes = append(
				internalNodes, nodeLevel{n, d},
			)
			for _, child := range n.Children {
				collectNodes(child, d+1)
			}
		}
	}
	collectNodes(tree, 0)

	// Create aggregators for each internal node.
	aggs := make(map[TreeDepth]*Aggregator)
	nodesByDepth := make(map[TreeDepth]*TreeNode)
	for _, nl := range internalNodes {
		agg, err := NewAggregator(nl.node)
		require.NoError(t, err)
		aggs[nl.depth] = agg
		nodesByDepth[nl.depth] = nl.node
	}

	// Round 1: Nonce exchange — bottom-up.

	// Deepest level (depth 4): key0 + key1.
	_, err = aggs[4].RegisterNonce(pubs[0], sessions[0].PublicNonce())
	require.NoError(t, err)
	_, err = aggs[4].RegisterNonce(pubs[1], sessions[1].PublicNonce())
	require.NoError(t, err)

	// Propagate upward.
	for d := TreeDepth(3); d > 0; d-- {
		childNode := nodesByDepth[d+1]
		projected, err := aggs[d+1].ProjectedNonce()
		require.NoError(t, err)

		_, err = aggs[d].RegisterNonce(
			childNode.effectiveKey(), projected,
		)
		require.NoError(t, err)

		// Register the leaf signer at this depth.
		leafIdx := int(5 - d) // key2 at depth 3, key3 at 2, key4 at 1.
		_, err = aggs[d].RegisterNonce(
			pubs[leafIdx], sessions[leafIdx].PublicNonce(),
		)
		require.NoError(t, err)
	}

	// Root level (depth 0).
	childNode := nodesByDepth[1]
	projected, err := aggs[1].ProjectedNonce()
	require.NoError(t, err)

	_, err = aggs[0].RegisterNonce(
		childNode.effectiveKey(), projected,
	)
	require.NoError(t, err)
	_, err = aggs[0].RegisterNonce(pubs[5], sessions[5].PublicNonce())
	require.NoError(t, err)

	rootCombined, err := aggs[0].AggNonce()
	require.NoError(t, err)

	// Round 2: Signing.
	msg := sha256.Sum256([]byte("asymmetric tree test"))

	// Collect all nested coefficients for the deepest signers.
	allCoeffs := make(map[TreeDepth]*btcec.ModNScalar)
	for d := TreeDepth(1); d <= 4; d++ {
		bd, err := aggs[d].NonceCoeff()
		require.NoError(t, err)
		allCoeffs[d] = bd
	}

	// Key0 and Key1 need all 4 nested coefficients.
	sig0, err := sessions[0].Sign(msg, rootCombined, allCoeffs)
	require.NoError(t, err)
	sig1, err := sessions[1].Sign(msg, rootCombined, allCoeffs)
	require.NoError(t, err)

	// Key2 needs coefficients for depths 1-3.
	coeffs2 := map[TreeDepth]*btcec.ModNScalar{
		1: allCoeffs[1], 2: allCoeffs[2], 3: allCoeffs[3],
	}
	sig2, err := sessions[2].Sign(msg, rootCombined, coeffs2)
	require.NoError(t, err)

	// Key3 needs coefficients for depths 1-2.
	coeffs3 := map[TreeDepth]*btcec.ModNScalar{
		1: allCoeffs[1], 2: allCoeffs[2],
	}
	sig3, err := sessions[3].Sign(msg, rootCombined, coeffs3)
	require.NoError(t, err)

	// Key4 needs coefficient for depth 1.
	coeffs4 := map[TreeDepth]*btcec.ModNScalar{1: allCoeffs[1]}
	sig4, err := sessions[4].Sign(msg, rootCombined, coeffs4)
	require.NoError(t, err)

	// Key5 at root level needs no nested coefficients.
	sig5, err := sessions[5].Sign(msg, rootCombined, nil)
	require.NoError(t, err)

	// Signature aggregation — bottom-up.
	_, err = aggs[4].RegisterSig(pubs[0], sig0)
	require.NoError(t, err)
	_, err = aggs[4].RegisterSig(pubs[1], sig1)
	require.NoError(t, err)

	for d := TreeDepth(3); d > 0; d-- {
		childNode := nodesByDepth[d+1]
		childPartial, err := aggs[d+1].AsPartialSig()
		require.NoError(t, err)

		_, err = aggs[d].RegisterSig(
			childNode.effectiveKey(), childPartial,
		)
		require.NoError(t, err)

		leafIdx := int(5 - d)
		leafSigs := []*PartialSignature{sig2, sig3, sig4}
		_, err = aggs[d].RegisterSig(
			pubs[leafIdx], leafSigs[int(3-d)],
		)
		require.NoError(t, err)
	}

	// Root aggregation.
	childPartial, err := aggs[1].AsPartialSig()
	require.NoError(t, err)

	_, err = aggs[0].RegisterSig(
		nodesByDepth[1].effectiveKey(), childPartial,
	)
	require.NoError(t, err)
	_, err = aggs[0].RegisterSig(pubs[5], sig5)
	require.NoError(t, err)

	// Final signature.
	finalSig, err := aggs[0].FinalSig(msg)
	require.NoError(t, err)

	require.True(t, finalSig.Verify(msg[:], rootKey.FinalKey),
		"asymmetric nested signature should verify")
}

// TestNestedContextNonceReuse verifies that signing twice via the
// Context/Session API returns an error.
func TestNestedContextNonceReuse(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)

	tree := Group([]*TreeNode{
		Leaf(alicePriv.PubKey()),
		Leaf(bobPriv.PubKey()),
	}, WithKeySort())

	_, err := AggregateTree(tree)
	require.NoError(t, err)

	ctx, err := NewContext(alicePriv, false, WithCosignerTree(tree))
	require.NoError(t, err)

	sess, err := ctx.NewSession()
	require.NoError(t, err)

	msg := sha256.Sum256([]byte("reuse test"))

	// Register a dummy combined nonce so we can attempt signing.
	bobNonces, err := GenNonces(
		WithPublicKey(bobPriv.PubKey()),
		WithNonceSecretKeyAux(bobPriv),
	)
	require.NoError(t, err)

	combined, err := AggregateNonces(
		[][PubNonceSize]byte{
			sess.PublicNonce(), bobNonces.PubNonce,
		},
	)
	require.NoError(t, err)
	require.NoError(t, sess.RegisterCombinedNonce(combined))

	// First sign.
	_, _ = sess.Sign(msg)

	// Second sign should fail with reuse error.
	_, err = sess.Sign(msg)
	require.ErrorIs(t, err, ErrSigningContextReuse)
}

// TestNestedCompatibility verifies that nested MuSig2 signatures are
// indistinguishable from standard BIP-340 Schnorr signatures — they verify
// with the standard schnorr.Verify function.
func TestNestedCompatibility(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 10)
	bobPriv := genTestKey(t, 20)
	carolPriv := genTestKey(t, 30)

	alicePk := alicePriv.PubKey()
	bobPk := bobPriv.PubKey()
	carolPk := carolPriv.PubKey()

	tree := Group([]*TreeNode{
		Group([]*TreeNode{Leaf(alicePk), Leaf(bobPk)}, WithKeySort()),
		Leaf(carolPk),
	}, WithKeySort())

	rootKey, err := AggregateTree(tree)
	require.NoError(t, err)

	// Run full signing protocol.
	aliceSess, err := NewNestedSession(alicePriv, tree)
	require.NoError(t, err)
	bobSess, err := NewNestedSession(bobPriv, tree)
	require.NoError(t, err)
	carolSess, err := NewNestedSession(carolPriv, tree)
	require.NoError(t, err)

	abNode := tree.Children[0]
	abAgg, err := NewAggregator(abNode)
	require.NoError(t, err)

	_, err = abAgg.RegisterNonce(alicePk, aliceSess.PublicNonce())
	require.NoError(t, err)
	_, err = abAgg.RegisterNonce(bobPk, bobSess.PublicNonce())
	require.NoError(t, err)

	abProjected, err := abAgg.ProjectedNonce()
	require.NoError(t, err)

	rootAgg, err := NewAggregator(tree)
	require.NoError(t, err)

	_, err = rootAgg.RegisterNonce(abNode.effectiveKey(), abProjected)
	require.NoError(t, err)
	_, err = rootAgg.RegisterNonce(carolPk, carolSess.PublicNonce())
	require.NoError(t, err)

	rootCombined, err := rootAgg.AggNonce()
	require.NoError(t, err)

	msg := sha256.Sum256([]byte("compatibility test"))

	abBd, err := abAgg.NonceCoeff()
	require.NoError(t, err)

	aliceSig, err := aliceSess.Sign(msg, rootCombined,
		map[TreeDepth]*btcec.ModNScalar{1: abBd},
	)
	require.NoError(t, err)

	bobSig, err := bobSess.Sign(msg, rootCombined,
		map[TreeDepth]*btcec.ModNScalar{1: abBd},
	)
	require.NoError(t, err)

	carolSig, err := carolSess.Sign(msg, rootCombined, nil)
	require.NoError(t, err)

	_, err = abAgg.RegisterSig(alicePk, aliceSig)
	require.NoError(t, err)
	_, err = abAgg.RegisterSig(bobPk, bobSig)
	require.NoError(t, err)

	abPartial, err := abAgg.AsPartialSig()
	require.NoError(t, err)

	_, err = rootAgg.RegisterSig(abNode.effectiveKey(), abPartial)
	require.NoError(t, err)
	_, err = rootAgg.RegisterSig(carolPk, carolSig)
	require.NoError(t, err)

	finalSig, err := rootAgg.FinalSig(msg)
	require.NoError(t, err)

	// Verify using the standard schnorr.Verify function directly, not
	// the Signature.Verify method. This proves the signature is a
	// standard BIP-340 Schnorr signature.
	sigBytes := finalSig.Serialize()
	parsedSig, err := schnorr.ParseSignature(sigBytes)
	require.NoError(t, err)

	require.True(t, parsedSig.Verify(msg[:], rootKey.FinalKey),
		"re-parsed nested sig should verify as standard BIP-340")
}

// TestProjectNoncesInvalidR2 verifies that ProjectNonces returns an error
// when the second nonce component (R'_2) in the aggregated nonce contains
// invalid point data.
func TestProjectNoncesInvalidR2(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)

	// Build a valid aggregated nonce from two signers.
	aliceNonces, err := GenNonces(
		WithPublicKey(alicePriv.PubKey()),
		WithNonceSecretKeyAux(alicePriv),
	)
	require.NoError(t, err)

	bobNonces, err := GenNonces(
		WithPublicKey(bobPriv.PubKey()),
		WithNonceSecretKeyAux(bobPriv),
	)
	require.NoError(t, err)

	aggNonce, err := AggregateNonces(
		[][PubNonceSize]byte{
			aliceNonces.PubNonce, bobNonces.PubNonce,
		},
	)
	require.NoError(t, err)

	groupKey, _, _, err := AggregateKeys(
		[]*btcec.PublicKey{alicePriv.PubKey(), bobPriv.PubKey()}, true,
	)
	require.NoError(t, err)

	// Corrupt the R'_2 bytes (second 33-byte component) so ParsePubKey
	// fails.
	var badNonce [PubNonceSize]byte
	copy(badNonce[:], aggNonce[:])
	for i := btcec.PubKeyBytesLenCompressed; i < PubNonceSize; i++ {
		badNonce[i] = 0xff
	}

	_, _, err = ProjectNonces(badNonce, groupKey.FinalKey)
	require.Error(t, err, "ProjectNonces should fail with invalid R'_2")
	require.Contains(t, err.Error(), "parse R'_2")
}

// TestProjectNoncesDeterminism verifies that calling ProjectNonces twice
// with the same inputs produces identical results.
func TestProjectNoncesDeterminism(t *testing.T) {
	t.Parallel()

	alicePriv := genTestKey(t, 1)
	bobPriv := genTestKey(t, 2)

	aliceNonces, err := GenNonces(
		WithPublicKey(alicePriv.PubKey()),
		WithNonceSecretKeyAux(alicePriv),
	)
	require.NoError(t, err)

	bobNonces, err := GenNonces(
		WithPublicKey(bobPriv.PubKey()),
		WithNonceSecretKeyAux(bobPriv),
	)
	require.NoError(t, err)

	aggNonce, err := AggregateNonces(
		[][PubNonceSize]byte{
			aliceNonces.PubNonce, bobNonces.PubNonce,
		},
	)
	require.NoError(t, err)

	groupKey, _, _, err := AggregateKeys(
		[]*btcec.PublicKey{alicePriv.PubKey(), bobPriv.PubKey()}, true,
	)
	require.NoError(t, err)

	// Call ProjectNonces twice and verify identical output.
	proj1, bd1, err := ProjectNonces(aggNonce, groupKey.FinalKey)
	require.NoError(t, err)

	proj2, bd2, err := ProjectNonces(aggNonce, groupKey.FinalKey)
	require.NoError(t, err)

	require.Equal(t, proj1, proj2,
		"projected nonces should be deterministic")
	require.True(t, bd1.Equals(bd2),
		"nonce coefficients should be deterministic")
}
