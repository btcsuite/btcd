// Copyright 2013-2022 The btcsuite developers

package musig2

import (
	"crypto/sha256"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

// nestedSigner holds all the state for a single leaf signer in a nested
// MuSig2 tree.
type nestedSigner struct {
	privKey *btcec.PrivateKey
	pubKey  *btcec.PublicKey
	nonces  *Nonces
}

// newNestedSigner creates a new signer with a fresh key pair and nonces.
func newNestedSigner(t *testing.T) *nestedSigner {
	t.Helper()

	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("unable to gen priv key: %v", err)
	}

	nonces, err := GenNonces(WithPublicKey(privKey.PubKey()))
	if err != nil {
		t.Fatalf("unable to gen nonces: %v", err)
	}

	return &nestedSigner{
		privKey: privKey,
		pubKey:  privKey.PubKey(),
		nonces:  nonces,
	}
}

// TestExternalizeNonces tests that ExternalizeNonces produces valid nonce
// points that can be parsed and used in further operations.
func TestExternalizeNonces(t *testing.T) {
	t.Parallel()

	// Create two signers and aggregate their nonces.
	s1 := newNestedSigner(t)
	s2 := newNestedSigner(t)

	pubNonces := [][PubNonceSize]byte{
		s1.nonces.PubNonce, s2.nonces.PubNonce,
	}
	aggNonce, err := AggregateNonces(pubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate nonces: %v", err)
	}

	// Aggregate the keys.
	keys := []*btcec.PublicKey{s1.pubKey, s2.pubKey}
	aggKey, _, _, err := AggregateKeys(keys, false)
	if err != nil {
		t.Fatalf("unable to aggregate keys: %v", err)
	}

	// Externalize the nonces.
	extNonce, err := ExternalizeNonces(aggNonce, aggKey.FinalKey)
	if err != nil {
		t.Fatalf("unable to externalize nonces: %v", err)
	}

	// Verify both nonce points in the externalized nonce are valid.
	_, err = btcec.ParsePubKey(
		extNonce[:btcec.PubKeyBytesLenCompressed],
	)
	if err != nil {
		t.Fatalf("externalized R_1 is not a valid point: %v", err)
	}
	_, err = btcec.ParsePubKey(
		extNonce[btcec.PubKeyBytesLenCompressed:],
	)
	if err != nil {
		t.Fatalf("externalized R_2 is not a valid point: %v", err)
	}

	// The first nonce should be unchanged (R_1 = R'_1).
	for i := 0; i < btcec.PubKeyBytesLenCompressed; i++ {
		if extNonce[i] != aggNonce[i] {
			t.Fatalf("R_1 should be unchanged after " +
				"externalization")
		}
	}

	// The second nonce should be different (R_2 = b * R'_2).
	r2Ext := extNonce[btcec.PubKeyBytesLenCompressed:]
	r2Agg := aggNonce[btcec.PubKeyBytesLenCompressed:]
	same := true
	for i := range r2Ext {
		if r2Ext[i] != r2Agg[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatalf("R_2 should be different after externalization")
	}
}

// TestNestedMuSig2SingleLevel tests a nested MuSig2 signing session with one
// level of nesting. A group of nested signers (Alice, Bob) produces a combined
// partial signature that, together with a regular signer (Carol), forms a
// valid Schnorr signature.
func TestNestedMuSig2SingleLevel(t *testing.T) {
	t.Parallel()

	msg := sha256.Sum256([]byte("nested musig2 single level"))

	// Create the nested group: Alice and Bob.
	alice := newNestedSigner(t)
	bob := newNestedSigner(t)
	nestedKeys := []*btcec.PublicKey{alice.pubKey, bob.pubKey}

	// Create the regular signer: Carol.
	carol := newNestedSigner(t)

	// Step 1: Aggregate the nested group's keys to get the group key.
	nestedAggKey, _, _, err := AggregateKeys(nestedKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate nested keys: %v", err)
	}

	// Step 2: The top-level key set is [nestedGroupKey, carol].
	topKeys := []*btcec.PublicKey{nestedAggKey.FinalKey, carol.pubKey}

	// Step 3: Aggregate nonces within the nested group.
	nestedPubNonces := [][PubNonceSize]byte{
		alice.nonces.PubNonce, bob.nonces.PubNonce,
	}
	nestedAggNonce, err := AggregateNonces(nestedPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate nested nonces: %v", err)
	}

	// Step 4: Externalize the nested group's nonces.
	extNonce, err := ExternalizeNonces(
		nestedAggNonce, nestedAggKey.FinalKey,
	)
	if err != nil {
		t.Fatalf("unable to externalize nonces: %v", err)
	}

	// Step 5: Aggregate top-level nonces (externalized + Carol's).
	topPubNonces := [][PubNonceSize]byte{extNonce, carol.nonces.PubNonce}
	topAggNonce, err := AggregateNonces(topPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate top-level nonces: %v", err)
	}

	// Step 6: Each nested signer produces a partial signature using
	// NestedSign.
	nestingLevels := []NestingLevel{
		{
			PubKeys:  nestedKeys,
			AggNonce: nestedAggNonce,
		},
	}

	aliceSig, err := NestedSign(
		alice.nonces.SecNonce, alice.privKey, topAggNonce,
		topKeys, msg, nestingLevels,
	)
	if err != nil {
		t.Fatalf("alice unable to sign: %v", err)
	}

	bobSig, err := NestedSign(
		bob.nonces.SecNonce, bob.privKey, topAggNonce,
		topKeys, msg, nestingLevels,
	)
	if err != nil {
		t.Fatalf("bob unable to sign: %v", err)
	}

	// Step 7: Aggregate the nested group's partial signatures.
	nestedGroupSig, err := AggregateNestedPartialSigs(
		[]*PartialSignature{aliceSig, bobSig},
	)
	if err != nil {
		t.Fatalf("unable to aggregate nested sigs: %v", err)
	}

	// Step 8: Carol signs normally using the standard Sign function.
	carolSig, err := Sign(
		carol.nonces.SecNonce, carol.privKey, topAggNonce,
		topKeys, msg,
	)
	if err != nil {
		t.Fatalf("carol unable to sign: %v", err)
	}

	// Step 9: Combine all top-level partial signatures.
	finalSig := CombineSigs(
		nestedGroupSig.R,
		[]*PartialSignature{nestedGroupSig, carolSig},
	)

	// Step 10: Aggregate the top-level keys and verify.
	topAggKey, _, _, err := AggregateKeys(topKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate top keys: %v", err)
	}

	if !finalSig.Verify(msg[:], topAggKey.FinalKey) {
		t.Fatalf("final signature is invalid!")
	}
}

// TestNestedMuSig2TwoLevels tests a nested MuSig2 signing session with two
// levels of nesting (3 total depths). This verifies that the recursive
// nesting works correctly for deeper trees.
func TestNestedMuSig2TwoLevels(t *testing.T) {
	t.Parallel()

	msg := sha256.Sum256([]byte("nested musig2 two levels"))

	// Create leaf signers at depth 2 (deepest).
	leaf1 := newNestedSigner(t)
	leaf2 := newNestedSigner(t)
	depth2Keys := []*btcec.PublicKey{leaf1.pubKey, leaf2.pubKey}

	// Aggregate depth 2 keys to get the depth 1 group key.
	depth2AggKey, _, _, err := AggregateKeys(depth2Keys, false)
	if err != nil {
		t.Fatalf("unable to aggregate depth 2 keys: %v", err)
	}

	// Create a regular signer at depth 1.
	mid := newNestedSigner(t)
	depth1Keys := []*btcec.PublicKey{depth2AggKey.FinalKey, mid.pubKey}

	// Aggregate depth 1 keys to get the depth 0 group key.
	depth1AggKey, _, _, err := AggregateKeys(depth1Keys, false)
	if err != nil {
		t.Fatalf("unable to aggregate depth 1 keys: %v", err)
	}

	// Create a regular signer at depth 0 (top level).
	top := newNestedSigner(t)
	topKeys := []*btcec.PublicKey{depth1AggKey.FinalKey, top.pubKey}

	// --- Nonce aggregation from the bottom up ---

	// Depth 2: aggregate leaf nonces.
	depth2PubNonces := [][PubNonceSize]byte{
		leaf1.nonces.PubNonce, leaf2.nonces.PubNonce,
	}
	depth2AggNonce, err := AggregateNonces(depth2PubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate depth 2 nonces: %v", err)
	}

	// Externalize depth 2 nonces.
	depth2ExtNonce, err := ExternalizeNonces(
		depth2AggNonce, depth2AggKey.FinalKey,
	)
	if err != nil {
		t.Fatalf("unable to externalize depth 2 nonces: %v", err)
	}

	// Depth 1: aggregate nonces (externalized depth 2 + mid).
	depth1PubNonces := [][PubNonceSize]byte{
		depth2ExtNonce, mid.nonces.PubNonce,
	}
	depth1AggNonce, err := AggregateNonces(depth1PubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate depth 1 nonces: %v", err)
	}

	// Externalize depth 1 nonces.
	depth1ExtNonce, err := ExternalizeNonces(
		depth1AggNonce, depth1AggKey.FinalKey,
	)
	if err != nil {
		t.Fatalf("unable to externalize depth 1 nonces: %v", err)
	}

	// Top level: aggregate nonces.
	topPubNonces := [][PubNonceSize]byte{
		depth1ExtNonce, top.nonces.PubNonce,
	}
	topAggNonce, err := AggregateNonces(topPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate top nonces: %v", err)
	}

	// --- Signing ---

	// Leaf signers (depth 2) use NestedSign with 2 nesting levels.
	nestingLevels2Deep := []NestingLevel{
		{
			PubKeys:  depth1Keys,
			AggNonce: depth1AggNonce,
		},
		{
			PubKeys:  depth2Keys,
			AggNonce: depth2AggNonce,
		},
	}

	leaf1Sig, err := NestedSign(
		leaf1.nonces.SecNonce, leaf1.privKey, topAggNonce,
		topKeys, msg, nestingLevels2Deep,
	)
	if err != nil {
		t.Fatalf("leaf1 unable to sign: %v", err)
	}
	leaf2Sig, err := NestedSign(
		leaf2.nonces.SecNonce, leaf2.privKey, topAggNonce,
		topKeys, msg, nestingLevels2Deep,
	)
	if err != nil {
		t.Fatalf("leaf2 unable to sign: %v", err)
	}

	// Aggregate depth 2 partial sigs.
	depth2GroupSig, err := AggregateNestedPartialSigs(
		[]*PartialSignature{leaf1Sig, leaf2Sig},
	)
	if err != nil {
		t.Fatalf("unable to aggregate depth 2 sigs: %v", err)
	}

	// Mid signer (depth 1) uses NestedSign with 1 nesting level.
	nestingLevels1Deep := []NestingLevel{
		{
			PubKeys:  depth1Keys,
			AggNonce: depth1AggNonce,
		},
	}

	midSig, err := NestedSign(
		mid.nonces.SecNonce, mid.privKey, topAggNonce,
		topKeys, msg, nestingLevels1Deep,
	)
	if err != nil {
		t.Fatalf("mid unable to sign: %v", err)
	}

	// Aggregate depth 1 partial sigs (depth2 group + mid).
	depth1GroupSig, err := AggregateNestedPartialSigs(
		[]*PartialSignature{depth2GroupSig, midSig},
	)
	if err != nil {
		t.Fatalf("unable to aggregate depth 1 sigs: %v", err)
	}

	// Top signer signs normally.
	topSig, err := Sign(
		top.nonces.SecNonce, top.privKey, topAggNonce,
		topKeys, msg,
	)
	if err != nil {
		t.Fatalf("top signer unable to sign: %v", err)
	}

	// Combine all top-level partial sigs.
	finalSig := CombineSigs(
		depth1GroupSig.R,
		[]*PartialSignature{depth1GroupSig, topSig},
	)

	// Verify the final signature.
	topAggKey, _, _, err := AggregateKeys(topKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate top keys: %v", err)
	}

	if !finalSig.Verify(msg[:], topAggKey.FinalKey) {
		t.Fatalf("final signature is invalid!")
	}
}

// TestAggregateNestedPartialSigs tests that partial signature aggregation
// correctly sums the scalar values.
func TestAggregateNestedPartialSigs(t *testing.T) {
	t.Parallel()

	// Create some dummy partial signatures with known scalar values.
	var s1, s2, s3 btcec.ModNScalar
	s1.SetInt(42)
	s2.SetInt(58)
	s3.SetInt(100)

	// Use a dummy R point.
	dummyKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("unable to gen key: %v", err)
	}
	dummyR := dummyKey.PubKey()

	sig1 := NewPartialSignature(&s1, dummyR)
	sig2 := NewPartialSignature(&s2, dummyR)
	sig3 := NewPartialSignature(&s3, dummyR)

	combined, err := AggregateNestedPartialSigs(
		[]*PartialSignature{&sig1, &sig2, &sig3},
	)
	if err != nil {
		t.Fatalf("unable to aggregate: %v", err)
	}

	// The combined scalar should be 42 + 58 + 100 = 200.
	var expected btcec.ModNScalar
	expected.SetInt(200)
	if !combined.S.Equals(&expected) {
		t.Fatalf("combined scalar mismatch: got %v, want 200",
			combined.S)
	}

	// The R point should match the first sig's R.
	if !combined.R.IsEqual(dummyR) {
		t.Fatalf("combined R point should match input R")
	}
}

// TestAggregateNestedPartialSigsEmpty tests that aggregating an empty slice
// returns an error.
func TestAggregateNestedPartialSigsEmpty(t *testing.T) {
	t.Parallel()

	_, err := AggregateNestedPartialSigs(nil)
	if err != ErrNoPartialSigs {
		t.Fatalf("expected ErrNoPartialSigs, got: %v", err)
	}
}

// TestNestedMuSig2ManySigners tests nested MuSig2 with larger groups of
// signers at various nesting levels.
func TestNestedMuSig2ManySigners(t *testing.T) {
	t.Parallel()

	msg := sha256.Sum256([]byte("nested musig2 many signers"))

	const (
		numNestedSigners = 10
		numTopRegular    = 3
	)

	// Create the nested group signers.
	nestedSigners := make([]*nestedSigner, numNestedSigners)
	nestedKeys := make([]*btcec.PublicKey, numNestedSigners)
	for i := 0; i < numNestedSigners; i++ {
		nestedSigners[i] = newNestedSigner(t)
		nestedKeys[i] = nestedSigners[i].pubKey
	}

	// Aggregate nested group keys.
	nestedAggKey, _, _, err := AggregateKeys(nestedKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate nested keys: %v", err)
	}

	// Create top-level regular signers.
	topRegularSigners := make([]*nestedSigner, numTopRegular)
	topKeys := make([]*btcec.PublicKey, numTopRegular+1)
	topKeys[0] = nestedAggKey.FinalKey
	for i := 0; i < numTopRegular; i++ {
		topRegularSigners[i] = newNestedSigner(t)
		topKeys[i+1] = topRegularSigners[i].pubKey
	}

	// Aggregate nested nonces.
	nestedPubNonces := make([][PubNonceSize]byte, numNestedSigners)
	for i, s := range nestedSigners {
		nestedPubNonces[i] = s.nonces.PubNonce
	}
	nestedAggNonce, err := AggregateNonces(nestedPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate nested nonces: %v", err)
	}

	// Externalize nested nonces.
	extNonce, err := ExternalizeNonces(
		nestedAggNonce, nestedAggKey.FinalKey,
	)
	if err != nil {
		t.Fatalf("unable to externalize nonces: %v", err)
	}

	// Aggregate top-level nonces.
	topPubNonces := make([][PubNonceSize]byte, numTopRegular+1)
	topPubNonces[0] = extNonce
	for i, s := range topRegularSigners {
		topPubNonces[i+1] = s.nonces.PubNonce
	}
	topAggNonce, err := AggregateNonces(topPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate top nonces: %v", err)
	}

	// Each nested signer produces a partial signature.
	nestingLevels := []NestingLevel{
		{
			PubKeys:  nestedKeys,
			AggNonce: nestedAggNonce,
		},
	}

	nestedPartialSigs := make([]*PartialSignature, numNestedSigners)
	for i, s := range nestedSigners {
		sig, err := NestedSign(
			s.nonces.SecNonce, s.privKey, topAggNonce,
			topKeys, msg, nestingLevels,
		)
		if err != nil {
			t.Fatalf("nested signer %d unable to sign: %v", i, err)
		}
		nestedPartialSigs[i] = sig
	}

	// Aggregate nested partial sigs.
	nestedGroupSig, err := AggregateNestedPartialSigs(nestedPartialSigs)
	if err != nil {
		t.Fatalf("unable to aggregate nested sigs: %v", err)
	}

	// Top-level regular signers sign normally.
	allTopSigs := make([]*PartialSignature, numTopRegular+1)
	allTopSigs[0] = nestedGroupSig
	for i, s := range topRegularSigners {
		sig, err := Sign(
			s.nonces.SecNonce, s.privKey, topAggNonce,
			topKeys, msg,
		)
		if err != nil {
			t.Fatalf("top signer %d unable to sign: %v", i, err)
		}
		allTopSigs[i+1] = sig
	}

	// Combine all top-level sigs.
	finalSig := CombineSigs(nestedGroupSig.R, allTopSigs)

	// Verify.
	topAggKey, _, _, err := AggregateKeys(topKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate top keys: %v", err)
	}

	if !finalSig.Verify(msg[:], topAggKey.FinalKey) {
		t.Fatalf("final signature is invalid!")
	}
}

// TestVerifyNestedPartialSig tests that VerifyNestedPartialSig correctly
// validates partial signatures from nested signers, and rejects invalid ones.
func TestVerifyNestedPartialSig(t *testing.T) {
	t.Parallel()

	msg := sha256.Sum256([]byte("nested partial sig verify"))

	// Create the nested group: Alice and Bob.
	alice := newNestedSigner(t)
	bob := newNestedSigner(t)
	nestedKeys := []*btcec.PublicKey{alice.pubKey, bob.pubKey}

	// Create the regular signer: Carol.
	carol := newNestedSigner(t)

	nestedAggKey, _, _, err := AggregateKeys(nestedKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate nested keys: %v", err)
	}
	topKeys := []*btcec.PublicKey{nestedAggKey.FinalKey, carol.pubKey}

	// Nonce aggregation.
	nestedPubNonces := [][PubNonceSize]byte{
		alice.nonces.PubNonce, bob.nonces.PubNonce,
	}
	nestedAggNonce, err := AggregateNonces(nestedPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate nested nonces: %v", err)
	}
	extNonce, err := ExternalizeNonces(
		nestedAggNonce, nestedAggKey.FinalKey,
	)
	if err != nil {
		t.Fatalf("unable to externalize nonces: %v", err)
	}
	topPubNonces := [][PubNonceSize]byte{extNonce, carol.nonces.PubNonce}
	topAggNonce, err := AggregateNonces(topPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate top nonces: %v", err)
	}

	nestingLevels := []NestingLevel{
		{
			PubKeys:  nestedKeys,
			AggNonce: nestedAggNonce,
		},
	}

	// Generate partial signatures.
	aliceSig, err := NestedSign(
		alice.nonces.SecNonce, alice.privKey, topAggNonce,
		topKeys, msg, nestingLevels,
	)
	if err != nil {
		t.Fatalf("alice unable to sign: %v", err)
	}

	bobSig, err := NestedSign(
		bob.nonces.SecNonce, bob.privKey, topAggNonce,
		topKeys, msg, nestingLevels,
	)
	if err != nil {
		t.Fatalf("bob unable to sign: %v", err)
	}

	// Verify Alice's partial sig should pass.
	if !VerifyNestedPartialSig(aliceSig, alice.nonces.PubNonce,
		topAggNonce, topKeys, alice.pubKey, msg, nestingLevels) {

		t.Fatalf("alice's partial sig should be valid")
	}

	// Verify Bob's partial sig should pass.
	if !VerifyNestedPartialSig(bobSig, bob.nonces.PubNonce,
		topAggNonce, topKeys, bob.pubKey, msg, nestingLevels) {

		t.Fatalf("bob's partial sig should be valid")
	}

	// Verify Alice's sig with Bob's nonce should fail.
	if VerifyNestedPartialSig(aliceSig, bob.nonces.PubNonce,
		topAggNonce, topKeys, alice.pubKey, msg, nestingLevels) {

		t.Fatalf("alice's sig with bob's nonce should be invalid")
	}

	// Verify Alice's sig with Bob's key should fail.
	if VerifyNestedPartialSig(aliceSig, alice.nonces.PubNonce,
		topAggNonce, topKeys, bob.pubKey, msg, nestingLevels) {

		t.Fatalf("alice's sig verified with bob's key should be invalid")
	}

	// Verify with a different message should fail.
	wrongMsg := sha256.Sum256([]byte("wrong message"))
	if VerifyNestedPartialSig(aliceSig, alice.nonces.PubNonce,
		topAggNonce, topKeys, alice.pubKey, wrongMsg, nestingLevels) {

		t.Fatalf("sig verified with wrong message should be invalid")
	}
}

// TestVerifyNestedPartialSigTwoLevels tests partial signature verification
// with two levels of nesting.
func TestVerifyNestedPartialSigTwoLevels(t *testing.T) {
	t.Parallel()

	msg := sha256.Sum256([]byte("nested verify two levels"))

	// Create the same tree structure as TestNestedMuSig2TwoLevels.
	leaf1 := newNestedSigner(t)
	leaf2 := newNestedSigner(t)
	depth2Keys := []*btcec.PublicKey{leaf1.pubKey, leaf2.pubKey}

	depth2AggKey, _, _, err := AggregateKeys(depth2Keys, false)
	if err != nil {
		t.Fatalf("unable to aggregate depth 2 keys: %v", err)
	}

	mid := newNestedSigner(t)
	depth1Keys := []*btcec.PublicKey{depth2AggKey.FinalKey, mid.pubKey}

	depth1AggKey, _, _, err := AggregateKeys(depth1Keys, false)
	if err != nil {
		t.Fatalf("unable to aggregate depth 1 keys: %v", err)
	}

	top := newNestedSigner(t)
	topKeys := []*btcec.PublicKey{depth1AggKey.FinalKey, top.pubKey}

	// Nonce aggregation.
	depth2PubNonces := [][PubNonceSize]byte{
		leaf1.nonces.PubNonce, leaf2.nonces.PubNonce,
	}
	depth2AggNonce, err := AggregateNonces(depth2PubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate depth 2 nonces: %v", err)
	}
	depth2ExtNonce, err := ExternalizeNonces(
		depth2AggNonce, depth2AggKey.FinalKey,
	)
	if err != nil {
		t.Fatalf("unable to externalize depth 2 nonces: %v", err)
	}

	depth1PubNonces := [][PubNonceSize]byte{
		depth2ExtNonce, mid.nonces.PubNonce,
	}
	depth1AggNonce, err := AggregateNonces(depth1PubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate depth 1 nonces: %v", err)
	}
	depth1ExtNonce, err := ExternalizeNonces(
		depth1AggNonce, depth1AggKey.FinalKey,
	)
	if err != nil {
		t.Fatalf("unable to externalize depth 1 nonces: %v", err)
	}

	topPubNonces := [][PubNonceSize]byte{
		depth1ExtNonce, top.nonces.PubNonce,
	}
	topAggNonce, err := AggregateNonces(topPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate top nonces: %v", err)
	}

	// Sign and verify leaf signers at depth 2.
	nestingLevels2Deep := []NestingLevel{
		{PubKeys: depth1Keys, AggNonce: depth1AggNonce},
		{PubKeys: depth2Keys, AggNonce: depth2AggNonce},
	}

	leaf1Sig, err := NestedSign(
		leaf1.nonces.SecNonce, leaf1.privKey, topAggNonce,
		topKeys, msg, nestingLevels2Deep,
	)
	if err != nil {
		t.Fatalf("leaf1 unable to sign: %v", err)
	}

	if !VerifyNestedPartialSig(leaf1Sig, leaf1.nonces.PubNonce,
		topAggNonce, topKeys, leaf1.pubKey, msg,
		nestingLevels2Deep) {

		t.Fatalf("leaf1's partial sig should be valid")
	}

	// Sign and verify mid signer at depth 1.
	nestingLevels1Deep := []NestingLevel{
		{PubKeys: depth1Keys, AggNonce: depth1AggNonce},
	}

	midSig, err := NestedSign(
		mid.nonces.SecNonce, mid.privKey, topAggNonce,
		topKeys, msg, nestingLevels1Deep,
	)
	if err != nil {
		t.Fatalf("mid unable to sign: %v", err)
	}

	if !VerifyNestedPartialSig(midSig, mid.nonces.PubNonce,
		topAggNonce, topKeys, mid.pubKey, msg,
		nestingLevels1Deep) {

		t.Fatalf("mid's partial sig should be valid")
	}
}

// TestNestedMuSig2CompatWithStandard verifies that a single-person nested
// group correctly produces a valid Schnorr signature when combined with a
// regular top-level signer. This is the simplest nesting case and confirms
// the nested path works end-to-end even at the degenerate 1-of-1 boundary.
func TestNestedMuSig2CompatWithStandard(t *testing.T) {
	t.Parallel()

	msg := sha256.Sum256([]byte("nested musig2 compat"))

	// Create two signers. Signer1 will be wrapped in a 1-person nested
	// group. Signer2 is a regular top-level participant.
	signer1 := newNestedSigner(t)
	signer2 := newNestedSigner(t)

	// Signer1 as a 1-person group.
	nestedKeys := []*btcec.PublicKey{signer1.pubKey}
	nestedAggKey, _, _, err := AggregateKeys(nestedKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate nested keys: %v", err)
	}

	// Top-level key set uses the nested aggregate key.
	topKeys := []*btcec.PublicKey{
		nestedAggKey.FinalKey, signer2.pubKey,
	}

	// Aggregate signer1's nonces (1-person group).
	nestedPubNonces := [][PubNonceSize]byte{signer1.nonces.PubNonce}
	nestedAggNonce, err := AggregateNonces(nestedPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate nested nonces: %v", err)
	}

	// Externalize.
	extNonce, err := ExternalizeNonces(
		nestedAggNonce, nestedAggKey.FinalKey,
	)
	if err != nil {
		t.Fatalf("unable to externalize nonces: %v", err)
	}

	// Top-level nonce aggregation.
	topPubNonces := [][PubNonceSize]byte{
		extNonce, signer2.nonces.PubNonce,
	}
	topAggNonce, err := AggregateNonces(topPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate top nonces: %v", err)
	}

	// Signer1 signs via NestedSign.
	nestingLevels := []NestingLevel{
		{PubKeys: nestedKeys, AggNonce: nestedAggNonce},
	}
	sig1Nested, err := NestedSign(
		signer1.nonces.SecNonce, signer1.privKey, topAggNonce,
		topKeys, msg, nestingLevels,
	)
	if err != nil {
		t.Fatalf("signer1 nested sign failed: %v", err)
	}

	// Verify the nested partial sig.
	if !VerifyNestedPartialSig(sig1Nested, signer1.nonces.PubNonce,
		topAggNonce, topKeys, signer1.pubKey, msg, nestingLevels) {

		t.Fatalf("signer1 nested partial sig should be valid")
	}

	// Aggregate (1-person group).
	groupSig, err := AggregateNestedPartialSigs(
		[]*PartialSignature{sig1Nested},
	)
	if err != nil {
		t.Fatalf("unable to aggregate nested sigs: %v", err)
	}

	// Signer2 signs normally.
	sig2, err := Sign(
		signer2.nonces.SecNonce, signer2.privKey, topAggNonce,
		topKeys, msg,
	)
	if err != nil {
		t.Fatalf("signer2 sign failed: %v", err)
	}

	// Combine and verify the final Schnorr signature.
	finalSig := CombineSigs(
		groupSig.R,
		[]*PartialSignature{groupSig, sig2},
	)

	topAggKey, _, _, err := AggregateKeys(topKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate top keys: %v", err)
	}

	if !finalSig.Verify(msg[:], topAggKey.FinalKey) {
		t.Fatalf("nested path final signature is invalid!")
	}
}

// TestNestedMuSig2WithTaprootTweak tests that nested MuSig2 works correctly
// when the top-level key has a taproot tweak applied.
func TestNestedMuSig2WithTaprootTweak(t *testing.T) {
	t.Parallel()

	msg := sha256.Sum256([]byte("nested musig2 taproot tweak"))

	// Create the nested group.
	alice := newNestedSigner(t)
	bob := newNestedSigner(t)
	nestedKeys := []*btcec.PublicKey{alice.pubKey, bob.pubKey}

	nestedAggKey, _, _, err := AggregateKeys(nestedKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate nested keys: %v", err)
	}

	// Create a regular top-level signer.
	carol := newNestedSigner(t)
	topKeys := []*btcec.PublicKey{nestedAggKey.FinalKey, carol.pubKey}

	// Nonce aggregation.
	nestedPubNonces := [][PubNonceSize]byte{
		alice.nonces.PubNonce, bob.nonces.PubNonce,
	}
	nestedAggNonce, err := AggregateNonces(nestedPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate nested nonces: %v", err)
	}
	extNonce, err := ExternalizeNonces(
		nestedAggNonce, nestedAggKey.FinalKey,
	)
	if err != nil {
		t.Fatalf("unable to externalize nonces: %v", err)
	}
	topPubNonces := [][PubNonceSize]byte{extNonce, carol.nonces.PubNonce}
	topAggNonce, err := AggregateNonces(topPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate top nonces: %v", err)
	}

	// Apply a BIP 86 taproot tweak (key-only spend).
	nestingLevels := []NestingLevel{
		{
			PubKeys:  nestedKeys,
			AggNonce: nestedAggNonce,
		},
	}

	aliceSig, err := NestedSign(
		alice.nonces.SecNonce, alice.privKey, topAggNonce,
		topKeys, msg, nestingLevels, WithBip86SignTweak(),
	)
	if err != nil {
		t.Fatalf("alice unable to sign: %v", err)
	}
	bobSig, err := NestedSign(
		bob.nonces.SecNonce, bob.privKey, topAggNonce,
		topKeys, msg, nestingLevels, WithBip86SignTweak(),
	)
	if err != nil {
		t.Fatalf("bob unable to sign: %v", err)
	}

	nestedGroupSig, err := AggregateNestedPartialSigs(
		[]*PartialSignature{aliceSig, bobSig},
	)
	if err != nil {
		t.Fatalf("unable to aggregate nested sigs: %v", err)
	}

	carolSig, err := Sign(
		carol.nonces.SecNonce, carol.privKey, topAggNonce,
		topKeys, msg, WithBip86SignTweak(),
	)
	if err != nil {
		t.Fatalf("carol unable to sign: %v", err)
	}

	// Combine with taproot tweak.
	finalSig := CombineSigs(
		nestedGroupSig.R,
		[]*PartialSignature{nestedGroupSig, carolSig},
		WithBip86TweakedCombine(msg, topKeys, false),
	)

	// Verify against the tweaked key.
	topAggKey, _, _, err := AggregateKeys(
		topKeys, false, WithBIP86KeyTweak(),
	)
	if err != nil {
		t.Fatalf("unable to aggregate top keys: %v", err)
	}

	// For BIP 340 verification, we need to use the x-only key.
	xOnlyKey, err := schnorr.ParsePubKey(
		schnorr.SerializePubKey(topAggKey.FinalKey),
	)
	if err != nil {
		t.Fatalf("unable to parse x-only key: %v", err)
	}

	if !finalSig.Verify(msg[:], xOnlyKey) {
		t.Fatalf("final signature is invalid!")
	}
}

// TestNestedSessionSingleLevel tests the managed NestedSession API with a
// single level of nesting. Alice and Bob form a nested group that, together
// with a standard signer Carol, produces a valid Schnorr signature.
func TestNestedSessionSingleLevel(t *testing.T) {
	t.Parallel()

	msg := sha256.Sum256([]byte("nested session single level"))

	// Create the nested group: Alice and Bob.
	alice := newNestedSigner(t)
	bob := newNestedSigner(t)
	nestedKeys := []*btcec.PublicKey{alice.pubKey, bob.pubKey}

	// Create the regular signer: Carol.
	carol := newNestedSigner(t)

	// Aggregate the nested group's keys.
	nestedAggKey, _, _, err := AggregateKeys(nestedKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate nested keys: %v", err)
	}

	topKeys := []*btcec.PublicKey{nestedAggKey.FinalKey, carol.pubKey}

	// Aggregate nonces within the nested group.
	nestedPubNonces := [][PubNonceSize]byte{
		alice.nonces.PubNonce, bob.nonces.PubNonce,
	}
	nestedAggNonce, err := AggregateNonces(nestedPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate nested nonces: %v", err)
	}

	nestingLevels := []NestingLevel{
		{PubKeys: nestedKeys, AggNonce: nestedAggNonce},
	}

	// Create Alice's NestedSession via Context.
	aliceCtx, err := NewContext(
		alice.privKey, false,
		WithNestedSigners(nestedKeys, topKeys, nestingLevels),
	)
	if err != nil {
		t.Fatalf("unable to create alice context: %v", err)
	}

	aliceSession, err := aliceCtx.NewNestedSession(
		2, WithPreGeneratedNonce(alice.nonces),
	)
	if err != nil {
		t.Fatalf("unable to create alice session: %v", err)
	}

	// Create Bob's NestedSession via Context.
	bobCtx, err := NewContext(
		bob.privKey, false,
		WithNestedSigners(nestedKeys, topKeys, nestingLevels),
	)
	if err != nil {
		t.Fatalf("unable to create bob context: %v", err)
	}

	bobSession, err := bobCtx.NewNestedSession(
		2, WithPreGeneratedNonce(bob.nonces),
	)
	if err != nil {
		t.Fatalf("unable to create bob session: %v", err)
	}

	// Externalize the nested group's nonces and build the top-level
	// aggregate nonce.
	extNonce, err := ExternalizeNonces(
		nestedAggNonce, nestedAggKey.FinalKey,
	)
	if err != nil {
		t.Fatalf("unable to externalize nonces: %v", err)
	}
	topPubNonces := [][PubNonceSize]byte{
		extNonce, carol.nonces.PubNonce,
	}
	topAggNonce, err := AggregateNonces(topPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate top nonces: %v", err)
	}

	// Register the top-level combined nonce with both sessions.
	if err := aliceSession.RegisterCombinedNonce(topAggNonce); err != nil {
		t.Fatalf("alice register nonce: %v", err)
	}
	if err := bobSession.RegisterCombinedNonce(topAggNonce); err != nil {
		t.Fatalf("bob register nonce: %v", err)
	}

	// Both signers produce their nested partial signatures. The session
	// stores our own sig internally for aggregation.
	_, err = aliceSession.Sign(msg)
	if err != nil {
		t.Fatalf("alice sign: %v", err)
	}
	bobSig, err := bobSession.Sign(msg)
	if err != nil {
		t.Fatalf("bob sign: %v", err)
	}

	// Alice verifies Bob's sig and accumulates it.
	if !aliceSession.VerifyPeerSig(
		bobSig, bob.nonces.PubNonce, bob.pubKey,
	) {
		t.Fatalf("alice could not verify bob's sig")
	}
	done, err := aliceSession.CombinePeerSig(bobSig)
	if err != nil {
		t.Fatalf("alice combine bob sig: %v", err)
	}
	if !done {
		t.Fatalf("expected aggregation to be complete")
	}

	groupSig := aliceSession.AggregatedSig()
	if groupSig == nil {
		t.Fatalf("aggregated sig should not be nil")
	}

	// Carol signs normally using the standard Sign function.
	carolSig, err := Sign(
		carol.nonces.SecNonce, carol.privKey, topAggNonce,
		topKeys, msg,
	)
	if err != nil {
		t.Fatalf("carol unable to sign: %v", err)
	}

	// Combine and verify the final Schnorr signature.
	finalSig := CombineSigs(
		groupSig.R,
		[]*PartialSignature{groupSig, carolSig},
	)

	topAggKey, _, _, err := AggregateKeys(topKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate top keys: %v", err)
	}

	if !finalSig.Verify(msg[:], topAggKey.FinalKey) {
		t.Fatalf("final signature is invalid!")
	}
}

// TestNestedSessionTwoLevels tests the managed NestedSession API with two
// levels of nesting (3 total depths).
func TestNestedSessionTwoLevels(t *testing.T) {
	t.Parallel()

	msg := sha256.Sum256([]byte("nested session two levels"))

	// Create leaf signers at depth 2.
	leaf1 := newNestedSigner(t)
	leaf2 := newNestedSigner(t)
	depth2Keys := []*btcec.PublicKey{leaf1.pubKey, leaf2.pubKey}

	depth2AggKey, _, _, err := AggregateKeys(depth2Keys, false)
	if err != nil {
		t.Fatalf("unable to aggregate depth 2 keys: %v", err)
	}

	// Mid signer at depth 1.
	mid := newNestedSigner(t)
	depth1Keys := []*btcec.PublicKey{depth2AggKey.FinalKey, mid.pubKey}

	depth1AggKey, _, _, err := AggregateKeys(depth1Keys, false)
	if err != nil {
		t.Fatalf("unable to aggregate depth 1 keys: %v", err)
	}

	// Top-level signer at depth 0.
	top := newNestedSigner(t)
	topKeys := []*btcec.PublicKey{depth1AggKey.FinalKey, top.pubKey}

	// --- Nonce aggregation (bottom-up) ---
	depth2PubNonces := [][PubNonceSize]byte{
		leaf1.nonces.PubNonce, leaf2.nonces.PubNonce,
	}
	depth2AggNonce, err := AggregateNonces(depth2PubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate depth 2 nonces: %v", err)
	}
	depth2ExtNonce, err := ExternalizeNonces(
		depth2AggNonce, depth2AggKey.FinalKey,
	)
	if err != nil {
		t.Fatalf("unable to externalize depth 2 nonces: %v", err)
	}

	depth1PubNonces := [][PubNonceSize]byte{
		depth2ExtNonce, mid.nonces.PubNonce,
	}
	depth1AggNonce, err := AggregateNonces(depth1PubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate depth 1 nonces: %v", err)
	}
	depth1ExtNonce, err := ExternalizeNonces(
		depth1AggNonce, depth1AggKey.FinalKey,
	)
	if err != nil {
		t.Fatalf("unable to externalize depth 1 nonces: %v", err)
	}

	topPubNonces := [][PubNonceSize]byte{
		depth1ExtNonce, top.nonces.PubNonce,
	}
	topAggNonce, err := AggregateNonces(topPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate top nonces: %v", err)
	}

	// --- Leaf signers (depth 2) use NestedSession with 2 nesting levels ---
	nestingLevels2Deep := []NestingLevel{
		{PubKeys: depth1Keys, AggNonce: depth1AggNonce},
		{PubKeys: depth2Keys, AggNonce: depth2AggNonce},
	}

	leaf1Ctx, err := NewContext(
		leaf1.privKey, false,
		WithNestedSigners(depth2Keys, topKeys, nestingLevels2Deep),
	)
	if err != nil {
		t.Fatalf("unable to create leaf1 context: %v", err)
	}
	leaf1Session, err := leaf1Ctx.NewNestedSession(
		2, WithPreGeneratedNonce(leaf1.nonces),
	)
	if err != nil {
		t.Fatalf("unable to create leaf1 session: %v", err)
	}

	leaf2Ctx, err := NewContext(
		leaf2.privKey, false,
		WithNestedSigners(depth2Keys, topKeys, nestingLevels2Deep),
	)
	if err != nil {
		t.Fatalf("unable to create leaf2 context: %v", err)
	}
	leaf2Session, err := leaf2Ctx.NewNestedSession(
		2, WithPreGeneratedNonce(leaf2.nonces),
	)
	if err != nil {
		t.Fatalf("unable to create leaf2 session: %v", err)
	}

	// Register top-level combined nonce.
	if err := leaf1Session.RegisterCombinedNonce(topAggNonce); err != nil {
		t.Fatalf("leaf1 register nonce: %v", err)
	}
	if err := leaf2Session.RegisterCombinedNonce(topAggNonce); err != nil {
		t.Fatalf("leaf2 register nonce: %v", err)
	}

	// Sign. The session stores our own sig internally for aggregation.
	_, err = leaf1Session.Sign(msg)
	if err != nil {
		t.Fatalf("leaf1 sign: %v", err)
	}
	leaf2Sig, err := leaf2Session.Sign(msg)
	if err != nil {
		t.Fatalf("leaf2 sign: %v", err)
	}

	// Leaf1 accumulates leaf2's sig.
	if !leaf1Session.VerifyPeerSig(
		leaf2Sig, leaf2.nonces.PubNonce, leaf2.pubKey,
	) {
		t.Fatalf("leaf1 could not verify leaf2's sig")
	}
	done, err := leaf1Session.CombinePeerSig(leaf2Sig)
	if err != nil {
		t.Fatalf("leaf1 combine: %v", err)
	}
	if !done {
		t.Fatalf("expected depth 2 aggregation to be complete")
	}
	depth2GroupSig := leaf1Session.AggregatedSig()

	// --- Mid signer (depth 1) uses NestedSession with 1 nesting level ---
	nestingLevels1Deep := []NestingLevel{
		{PubKeys: depth1Keys, AggNonce: depth1AggNonce},
	}
	midCtx, err := NewContext(
		mid.privKey, false,
		WithNestedSigners(depth1Keys, topKeys, nestingLevels1Deep),
	)
	if err != nil {
		t.Fatalf("unable to create mid context: %v", err)
	}
	midSession, err := midCtx.NewNestedSession(
		2, WithPreGeneratedNonce(mid.nonces),
	)
	if err != nil {
		t.Fatalf("unable to create mid session: %v", err)
	}
	if err := midSession.RegisterCombinedNonce(topAggNonce); err != nil {
		t.Fatalf("mid register nonce: %v", err)
	}

	midSig, err := midSession.Sign(msg)
	if err != nil {
		t.Fatalf("mid sign: %v", err)
	}

	// Aggregate depth 1 sigs (depth2 group + mid).
	depth1GroupSig, err := AggregateNestedPartialSigs(
		[]*PartialSignature{depth2GroupSig, midSig},
	)
	if err != nil {
		t.Fatalf("unable to aggregate depth 1 sigs: %v", err)
	}

	// Top signer signs normally.
	topSig, err := Sign(
		top.nonces.SecNonce, top.privKey, topAggNonce,
		topKeys, msg,
	)
	if err != nil {
		t.Fatalf("top signer unable to sign: %v", err)
	}

	// Combine and verify.
	finalSig := CombineSigs(
		depth1GroupSig.R,
		[]*PartialSignature{depth1GroupSig, topSig},
	)

	topAggKey, _, _, err := AggregateKeys(topKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate top keys: %v", err)
	}

	if !finalSig.Verify(msg[:], topAggKey.FinalKey) {
		t.Fatalf("final signature is invalid!")
	}
}

// TestNestedSessionWithTaprootTweak tests the NestedSession API with a BIP 86
// taproot tweak applied at the top level.
func TestNestedSessionWithTaprootTweak(t *testing.T) {
	t.Parallel()

	msg := sha256.Sum256([]byte("nested session taproot"))

	// Create the nested group: Alice and Bob.
	alice := newNestedSigner(t)
	bob := newNestedSigner(t)
	nestedKeys := []*btcec.PublicKey{alice.pubKey, bob.pubKey}

	nestedAggKey, _, _, err := AggregateKeys(nestedKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate nested keys: %v", err)
	}

	// Regular top-level signer: Carol.
	carol := newNestedSigner(t)
	topKeys := []*btcec.PublicKey{nestedAggKey.FinalKey, carol.pubKey}

	// Nonce aggregation.
	nestedPubNonces := [][PubNonceSize]byte{
		alice.nonces.PubNonce, bob.nonces.PubNonce,
	}
	nestedAggNonce, err := AggregateNonces(nestedPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate nested nonces: %v", err)
	}

	nestingLevels := []NestingLevel{
		{PubKeys: nestedKeys, AggNonce: nestedAggNonce},
	}

	extNonce, err := ExternalizeNonces(
		nestedAggNonce, nestedAggKey.FinalKey,
	)
	if err != nil {
		t.Fatalf("unable to externalize nonces: %v", err)
	}
	topPubNonces := [][PubNonceSize]byte{
		extNonce, carol.nonces.PubNonce,
	}
	topAggNonce, err := AggregateNonces(topPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate top nonces: %v", err)
	}

	// Create sessions with BIP 86 tweak.
	aliceCtx, err := NewContext(
		alice.privKey, false,
		WithNestedSigners(nestedKeys, topKeys, nestingLevels),
		WithBip86TweakCtx(),
	)
	if err != nil {
		t.Fatalf("unable to create alice context: %v", err)
	}
	aliceSession, err := aliceCtx.NewNestedSession(
		2, WithPreGeneratedNonce(alice.nonces),
	)
	if err != nil {
		t.Fatalf("unable to create alice session: %v", err)
	}

	bobCtx, err := NewContext(
		bob.privKey, false,
		WithNestedSigners(nestedKeys, topKeys, nestingLevels),
		WithBip86TweakCtx(),
	)
	if err != nil {
		t.Fatalf("unable to create bob context: %v", err)
	}
	bobSession, err := bobCtx.NewNestedSession(
		2, WithPreGeneratedNonce(bob.nonces),
	)
	if err != nil {
		t.Fatalf("unable to create bob session: %v", err)
	}

	// Register combined nonces and sign.
	if err := aliceSession.RegisterCombinedNonce(topAggNonce); err != nil {
		t.Fatalf("alice register nonce: %v", err)
	}
	if err := bobSession.RegisterCombinedNonce(topAggNonce); err != nil {
		t.Fatalf("bob register nonce: %v", err)
	}

	_, err = aliceSession.Sign(msg)
	if err != nil {
		t.Fatalf("alice sign: %v", err)
	}
	bobSig, err := bobSession.Sign(msg)
	if err != nil {
		t.Fatalf("bob sign: %v", err)
	}

	// Accumulate and aggregate.
	if !aliceSession.VerifyPeerSig(
		bobSig, bob.nonces.PubNonce, bob.pubKey,
	) {
		t.Fatalf("alice could not verify bob's sig")
	}
	done, err := aliceSession.CombinePeerSig(bobSig)
	if err != nil {
		t.Fatalf("combine: %v", err)
	}
	if !done {
		t.Fatalf("expected aggregation to be complete")
	}

	groupSig := aliceSession.AggregatedSig()

	// Carol signs with BIP 86 tweak.
	carolSig, err := Sign(
		carol.nonces.SecNonce, carol.privKey, topAggNonce,
		topKeys, msg, WithBip86SignTweak(),
	)
	if err != nil {
		t.Fatalf("carol unable to sign: %v", err)
	}

	// Combine with taproot tweak.
	finalSig := CombineSigs(
		groupSig.R,
		[]*PartialSignature{groupSig, carolSig},
		WithBip86TweakedCombine(msg, topKeys, false),
	)

	// Verify against the tweaked key.
	topAggKey, _, _, err := AggregateKeys(
		topKeys, false, WithBIP86KeyTweak(),
	)
	if err != nil {
		t.Fatalf("unable to aggregate top keys: %v", err)
	}

	xOnlyKey, err := schnorr.ParsePubKey(
		schnorr.SerializePubKey(topAggKey.FinalKey),
	)
	if err != nil {
		t.Fatalf("unable to parse x-only key: %v", err)
	}

	if !finalSig.Verify(msg[:], xOnlyKey) {
		t.Fatalf("final signature is invalid!")
	}
}

// TestNestedSessionGuard tests that NewSession() returns an error when called
// on a context configured with WithNestedSigners.
func TestNestedSessionGuard(t *testing.T) {
	t.Parallel()

	alice := newNestedSigner(t)
	bob := newNestedSigner(t)
	nestedKeys := []*btcec.PublicKey{alice.pubKey, bob.pubKey}

	nestedAggKey, _, _, err := AggregateKeys(nestedKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate nested keys: %v", err)
	}

	carol := newNestedSigner(t)
	topKeys := []*btcec.PublicKey{nestedAggKey.FinalKey, carol.pubKey}

	nestedAggNonce, err := AggregateNonces(
		[][PubNonceSize]byte{
			alice.nonces.PubNonce, bob.nonces.PubNonce,
		},
	)
	if err != nil {
		t.Fatalf("unable to aggregate nonces: %v", err)
	}

	nestingLevels := []NestingLevel{
		{PubKeys: nestedKeys, AggNonce: nestedAggNonce},
	}

	ctx, err := NewContext(
		alice.privKey, false,
		WithNestedSigners(nestedKeys, topKeys, nestingLevels),
	)
	if err != nil {
		t.Fatalf("unable to create context: %v", err)
	}

	// NewSession should fail on a nested-configured context.
	_, err = ctx.NewSession()
	if err != ErrNestedContextRequiresNestedSession {
		t.Fatalf("expected ErrNestedContextRequiresNestedSession, "+
			"got: %v", err)
	}
}

// TestNestedSessionNonceReuse tests that calling Sign() twice on the same
// NestedSession returns ErrSigningContextReuse.
func TestNestedSessionNonceReuse(t *testing.T) {
	t.Parallel()

	alice := newNestedSigner(t)
	bob := newNestedSigner(t)
	nestedKeys := []*btcec.PublicKey{alice.pubKey, bob.pubKey}

	nestedAggKey, _, _, err := AggregateKeys(nestedKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate nested keys: %v", err)
	}

	carol := newNestedSigner(t)
	topKeys := []*btcec.PublicKey{nestedAggKey.FinalKey, carol.pubKey}

	nestedPubNonces := [][PubNonceSize]byte{
		alice.nonces.PubNonce, bob.nonces.PubNonce,
	}
	nestedAggNonce, err := AggregateNonces(nestedPubNonces)
	if err != nil {
		t.Fatalf("unable to aggregate nonces: %v", err)
	}

	nestingLevels := []NestingLevel{
		{PubKeys: nestedKeys, AggNonce: nestedAggNonce},
	}

	extNonce, err := ExternalizeNonces(
		nestedAggNonce, nestedAggKey.FinalKey,
	)
	if err != nil {
		t.Fatalf("unable to externalize nonces: %v", err)
	}
	topAggNonce, err := AggregateNonces(
		[][PubNonceSize]byte{extNonce, carol.nonces.PubNonce},
	)
	if err != nil {
		t.Fatalf("unable to aggregate top nonces: %v", err)
	}

	ctx, err := NewContext(
		alice.privKey, false,
		WithNestedSigners(nestedKeys, topKeys, nestingLevels),
	)
	if err != nil {
		t.Fatalf("unable to create context: %v", err)
	}

	session, err := ctx.NewNestedSession(
		2, WithPreGeneratedNonce(alice.nonces),
	)
	if err != nil {
		t.Fatalf("unable to create session: %v", err)
	}

	if err := session.RegisterCombinedNonce(topAggNonce); err != nil {
		t.Fatalf("register nonce: %v", err)
	}

	msg := sha256.Sum256([]byte("nonce reuse test"))

	// First sign should succeed.
	_, err = session.Sign(msg)
	if err != nil {
		t.Fatalf("first sign: %v", err)
	}

	// Second sign should fail with nonce reuse error.
	_, err = session.Sign(msg)
	if err != ErrSigningContextReuse {
		t.Fatalf("expected ErrSigningContextReuse, got: %v", err)
	}
}

// TestNestedSessionNotNestedContext tests that NewNestedSession returns an
// error when called on a context not configured with WithNestedSigners.
func TestNestedSessionNotNestedContext(t *testing.T) {
	t.Parallel()

	alice := newNestedSigner(t)
	bob := newNestedSigner(t)

	ctx, err := NewContext(
		alice.privKey, false,
		WithKnownSigners(
			[]*btcec.PublicKey{alice.pubKey, bob.pubKey},
		),
	)
	if err != nil {
		t.Fatalf("unable to create context: %v", err)
	}

	// NewNestedSession should fail on a non-nested context.
	_, err = ctx.NewNestedSession(2)
	if err != ErrNotNestedContext {
		t.Fatalf("expected ErrNotNestedContext, got: %v", err)
	}
}

// TestNestedSessionTopLevelCombinedKey tests the TopLevelCombinedKey method.
func TestNestedSessionTopLevelCombinedKey(t *testing.T) {
	t.Parallel()

	alice := newNestedSigner(t)
	bob := newNestedSigner(t)
	nestedKeys := []*btcec.PublicKey{alice.pubKey, bob.pubKey}

	nestedAggKey, _, _, err := AggregateKeys(nestedKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate nested keys: %v", err)
	}

	carol := newNestedSigner(t)
	topKeys := []*btcec.PublicKey{nestedAggKey.FinalKey, carol.pubKey}

	nestedAggNonce, err := AggregateNonces(
		[][PubNonceSize]byte{
			alice.nonces.PubNonce, bob.nonces.PubNonce,
		},
	)
	if err != nil {
		t.Fatalf("unable to aggregate nonces: %v", err)
	}

	nestingLevels := []NestingLevel{
		{PubKeys: nestedKeys, AggNonce: nestedAggNonce},
	}

	ctx, err := NewContext(
		alice.privKey, false,
		WithNestedSigners(nestedKeys, topKeys, nestingLevels),
	)
	if err != nil {
		t.Fatalf("unable to create context: %v", err)
	}

	// TopLevelCombinedKey should return the aggregated top-level key.
	topCombined, err := ctx.TopLevelCombinedKey()
	if err != nil {
		t.Fatalf("unable to get top level combined key: %v", err)
	}

	// Compare against directly aggregating the top-level keys.
	expected, _, _, err := AggregateKeys(topKeys, false)
	if err != nil {
		t.Fatalf("unable to aggregate top keys: %v", err)
	}

	if !topCombined.IsEqual(expected.FinalKey) {
		t.Fatalf("top level combined key mismatch")
	}

	// TopLevelCombinedKey on a non-nested context should fail.
	regularCtx, err := NewContext(
		alice.privKey, false,
		WithKnownSigners(
			[]*btcec.PublicKey{alice.pubKey, bob.pubKey},
		),
	)
	if err != nil {
		t.Fatalf("unable to create regular context: %v", err)
	}

	_, err = regularCtx.TopLevelCombinedKey()
	if err != ErrNotNestedContext {
		t.Fatalf("expected ErrNotNestedContext, got: %v", err)
	}
}
