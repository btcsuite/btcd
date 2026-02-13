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
	// ErrAggNoncesNotReady is returned when AggNonce, ProjectedNonce, or
	// NonceCoeff is called before all child nonces have been registered.
	ErrAggNoncesNotReady = fmt.Errorf(
		"not all child nonces have been registered",
	)

	// ErrAggSigsNotReady is returned when AggregatedSig or AsPartialSig
	// is called before all child sigs have been registered.
	ErrAggSigsNotReady = fmt.Errorf(
		"not all child signatures have been registered",
	)

	// ErrChildKeyNotFound is returned when a pubkey-based registration
	// cannot find the key among this node's children.
	ErrChildKeyNotFound = fmt.Errorf(
		"public key is not a child of this node",
	)

	// ErrDuplicateNonce is returned when a nonce is registered twice for
	// the same child.
	ErrDuplicateNonce = fmt.Errorf("nonce already registered for this child")

	// ErrDuplicateSig is returned when a signature is registered twice
	// for the same child.
	ErrDuplicateSig = fmt.Errorf("sig already registered for this child")

	// ErrNestedSessionReuse is returned when Sign is called more than
	// once on a NestedSession, which would reuse a nonce.
	ErrNestedSessionReuse = fmt.Errorf("nested session nonce already used")
)

// Aggregator manages nonce/sig collection for one internal node in the
// cosigner tree. It is single-use (one signing session) to prevent nonce
// reuse.
type Aggregator struct {
	// node is the internal tree node this aggregator manages.
	node *TreeNode

	// childKeyMap maps compressed child pubkey -> child index for O(1)
	// lookup during pubkey-based registration.
	childKeyMap map[[33]byte]int

	// Nonce state.
	childNonces   [][PubNonceSize]byte
	nonceReceived []bool
	noncesRecvd   int
	aggNonce      *[PubNonceSize]byte
	projNonce     *[PubNonceSize]byte
	nonceCoeff    *btcec.ModNScalar

	// Sig state.
	childSigs   []*PartialSignature
	sigReceived []bool
	sigsRecvd   int
	aggSig      *btcec.ModNScalar
}

// NewAggregator creates an Aggregator for the given internal tree node. The
// tree must have been aggregated first (AggregateTree called).
func NewAggregator(node *TreeNode) (*Aggregator, error) {
	if !node.isInternal() {
		return nil, ErrNodeNotInternal
	}
	if node.aggKey == nil {
		return nil, ErrNodeNotAggregated
	}

	numChildren := len(node.Children)

	childKeyMap := make(map[[33]byte]int, numChildren)
	for i, child := range node.Children {
		var keyBytes [33]byte
		copy(keyBytes[:], child.effectiveKey().SerializeCompressed())
		childKeyMap[keyBytes] = i
	}

	return &Aggregator{
		node:          node,
		childKeyMap:   childKeyMap,
		childNonces:   make([][PubNonceSize]byte, numChildren),
		nonceReceived: make([]bool, numChildren),
		childSigs:     make([]*PartialSignature, numChildren),
		sigReceived:   make([]bool, numChildren),
	}, nil
}

// childIndex looks up the child index for the given public key.
func (a *Aggregator) childIndex(
	childKey *btcec.PublicKey) (int, error) {

	var keyBytes [33]byte
	copy(keyBytes[:], childKey.SerializeCompressed())

	idx, ok := a.childKeyMap[keyBytes]
	if !ok {
		return 0, ErrChildKeyNotFound
	}

	return idx, nil
}

// RegisterNonce registers a child's public nonce, identified by the child's
// public key. Returns true when all children's nonces have been collected.
func (a *Aggregator) RegisterNonce(childKey *btcec.PublicKey,
	nonce [PubNonceSize]byte) (bool, error) {

	idx, err := a.childIndex(childKey)
	if err != nil {
		return false, err
	}

	if a.nonceReceived[idx] {
		return false, ErrDuplicateNonce
	}

	a.childNonces[idx] = nonce
	a.nonceReceived[idx] = true
	a.noncesRecvd++

	// If we have all nonces, perform aggregation and projection.
	if a.noncesRecvd == len(a.node.Children) {
		aggNonce, err := AggregateNonces(a.childNonces)
		if err != nil {
			return false, fmt.Errorf("aggregate nonces: %w", err)
		}
		a.aggNonce = &aggNonce

		// Project the nonces for the parent level.
		projNonce, bd, err := ProjectNonces(
			aggNonce, a.node.aggKey.FinalKey,
		)
		if err != nil {
			return false, fmt.Errorf("project nonces: %w", err)
		}
		a.projNonce = &projNonce
		a.nonceCoeff = bd

		return true, nil
	}

	return false, nil
}

// AggNonce returns the internally aggregated nonce (after SignAgg). Available
// after all child nonces are registered.
func (a *Aggregator) AggNonce() ([PubNonceSize]byte, error) {
	if a.aggNonce == nil {
		return [PubNonceSize]byte{}, ErrAggNoncesNotReady
	}

	return *a.aggNonce, nil
}

// ProjectedNonce returns the projected nonce for the parent level (after
// SignAggExt). Available after all child nonces are registered.
func (a *Aggregator) ProjectedNonce() ([PubNonceSize]byte, error) {
	if a.projNonce == nil {
		return [PubNonceSize]byte{}, ErrAggNoncesNotReady
	}

	return *a.projNonce, nil
}

// NonceCoeff returns the internal nonce coefficient b_d.
func (a *Aggregator) NonceCoeff() (*btcec.ModNScalar, error) {
	if a.nonceCoeff == nil {
		return nil, ErrAggNoncesNotReady
	}

	return a.nonceCoeff, nil
}

// RegisterSig registers a child's partial signature, identified by the
// child's public key. Returns true when all children's sigs have been
// collected.
func (a *Aggregator) RegisterSig(childKey *btcec.PublicKey,
	sig *PartialSignature) (bool, error) {

	idx, err := a.childIndex(childKey)
	if err != nil {
		return false, err
	}

	if a.sigReceived[idx] {
		return false, ErrDuplicateSig
	}

	a.childSigs[idx] = sig
	a.sigReceived[idx] = true
	a.sigsRecvd++

	// If we have all sigs, aggregate them.
	if a.sigsRecvd == len(a.node.Children) {
		a.aggSig = AggregatePartialSigs(a.childSigs)
		return true, nil
	}

	return false, nil
}

// AggregatedSig returns the aggregated partial sig scalar for this subtree.
// Available after all child sigs are registered.
func (a *Aggregator) AggregatedSig() (*btcec.ModNScalar, error) {
	if a.aggSig == nil {
		return nil, ErrAggSigsNotReady
	}

	return a.aggSig, nil
}

// AsPartialSig wraps the aggregated scalar as a PartialSignature for
// registration at the parent level.
func (a *Aggregator) AsPartialSig() (*PartialSignature, error) {
	if a.aggSig == nil {
		return nil, ErrAggSigsNotReady
	}

	// The R value for the partial sig is not meaningful at the subtree
	// level — only the scalar s matters for aggregation upward. We use
	// nil for R since it won't be used until the root combines everything.
	return &PartialSignature{
		S: a.aggSig,
	}, nil
}

// FinalSig produces the final Schnorr signature at the root level. It
// computes the effective nonce R from the root combined nonce and message,
// applies tweak corrections if the root has tweaks, and returns a standard
// BIP-340 Schnorr signature.
func (a *Aggregator) FinalSig(
	msg [32]byte) (*schnorr.Signature, error) {

	if a.aggSig == nil {
		return nil, ErrAggSigsNotReady
	}
	if a.aggNonce == nil {
		return nil, ErrAggNoncesNotReady
	}

	// Compute the root-level nonce blinder and effective nonce R.
	nonce, _, err := computeSigningNonce(
		*a.aggNonce, a.node.aggKey.FinalKey, msg,
	)
	if err != nil {
		return nil, fmt.Errorf("compute signing nonce: %w", err)
	}

	nonce.ToAffine()
	nonceKey := btcec.NewPublicKey(&nonce.X, &nonce.Y)

	// Start with the aggregated sig scalar.
	var finalS btcec.ModNScalar
	finalS.Set(a.aggSig)

	// If the root node has tweaks, apply the tweak correction:
	// s_final = s_agg + e * tweakAcc * parityFactor.
	if !a.node.tweakAcc.IsZero() {
		// Compute parity factor based on the root combined key.
		parityFactor := new(btcec.ModNScalar).SetInt(1)
		combinedKeyBytes := a.node.aggKey.FinalKey.SerializeCompressed()
		if combinedKeyBytes[0] == secp.PubKeyFormatCompressedOdd {
			parityFactor.Negate()
		}

		// Compute challenge e.
		var challengeMsg bytes.Buffer
		challengeMsg.Write(schnorr.SerializePubKey(nonceKey))
		challengeMsg.Write(
			schnorr.SerializePubKey(a.node.aggKey.FinalKey),
		)
		challengeMsg.Write(msg[:])
		challengeBytes := chainhash.TaggedHash(
			ChallengeHashTag, challengeMsg.Bytes(),
		)

		var e btcec.ModNScalar
		e.SetByteSlice(challengeBytes[:])

		// tweakProduct = e * tweakAcc * parityFactor.
		tweakProduct := new(btcec.ModNScalar).Set(&e)
		tweakProduct.Mul(&a.node.tweakAcc).Mul(parityFactor)

		finalS.Add(tweakProduct)
	}

	// Build the final Schnorr signature (R_x, s).
	var nonceJ btcec.JacobianPoint
	nonceKey.AsJacobian(&nonceJ)
	nonceJ.ToAffine()

	finalSig := schnorr.NewSignature(&nonceJ.X, &finalS)

	// Verify the signature before returning.
	if !finalSig.Verify(msg[:], a.node.aggKey.FinalKey) {
		return nil, ErrFinalSigInvalid
	}

	return finalSig, nil
}

// NestedSession manages a leaf signer's participation in one nested MuSig2
// signing round. For most use cases, prefer the Context/Session API with
// WithCosignerTree instead.
type NestedSession struct {
	signingKey *btcec.PrivateKey
	path       *SignerPath
	tree       *TreeNode
	rootKey    *AggregateKey

	localNonces *Nonces
	signed      bool
}

// NewNestedSession creates a standalone session for a leaf signer. The tree
// must have been aggregated via AggregateTree. Generates fresh nonces
// internally.
func NewNestedSession(signingKey *btcec.PrivateKey,
	tree *TreeNode) (*NestedSession, error) {

	if tree.aggKey == nil {
		return nil, ErrNodeNotAggregated
	}

	// Find the signer's path through the tree.
	path, err := tree.FindPath(signingKey.PubKey())
	if err != nil {
		return nil, fmt.Errorf("find signer path: %w", err)
	}

	// Generate fresh nonces.
	nonces, err := GenNonces(
		WithPublicKey(signingKey.PubKey()),
		WithNonceSecretKeyAux(signingKey),
		WithNonceCombinedKeyAux(tree.aggKey.FinalKey),
	)
	if err != nil {
		return nil, fmt.Errorf("generate nonces: %w", err)
	}

	return &NestedSession{
		signingKey:  signingKey,
		path:        path,
		tree:        tree,
		rootKey:     tree.aggKey,
		localNonces: nonces,
	}, nil
}

// PublicNonce returns this signer's public nonce.
func (ns *NestedSession) PublicNonce() [PubNonceSize]byte {
	return ns.localNonces.PubNonce
}

// RootKey returns the root aggregate public key.
func (ns *NestedSession) RootKey() *btcec.PublicKey {
	return ns.rootKey.FinalKey
}

// Sign produces a partial signature for the leaf signer. It requires:
//   - msg: the message to sign
//   - rootCombinedNonce: from the root-level aggregator's AggNonce()
//   - nestedCoeffs: maps TreeDepth -> b_d for each nesting level > 0 along
//     this signer's path (from each level's Aggregator.NonceCoeff())
//
// The signer computes b_path = b_0 * product(nestedCoeffs) and a_path from
// the SignerPath, then produces:
//
//	s = k1 + b_path * k2 + e * a_path * d_eff
func (ns *NestedSession) Sign(msg [32]byte,
	rootCombinedNonce [PubNonceSize]byte,
	nestedCoeffs map[TreeDepth]*btcec.ModNScalar,
) (*PartialSignature, error) {

	if ns.signed {
		return nil, ErrNestedSessionReuse
	}

	// Compute the root-level nonce blinder b_0.
	b0 := ComputeNonceBlinder(
		rootCombinedNonce, ns.rootKey.FinalKey, msg,
	)

	// Compute b_path = b_0 * product(nested b_d values).
	bPath := new(btcec.ModNScalar).Set(b0)
	for _, bd := range nestedCoeffs {
		bPath.Mul(bd)
	}

	// Get a_path and accumulated parity from the signer path.
	aPath := ns.path.APath()
	accParity := ns.path.AccumulatedParity()

	// Produce the partial signature.
	sig, err := NestedSign(
		ns.localNonces.SecNonce, ns.signingKey,
		aPath, bPath,
		rootCombinedNonce, ns.rootKey.FinalKey,
		msg, accParity,
	)
	if err != nil {
		return nil, err
	}

	// Blank out nonces to prevent reuse.
	ns.localNonces = nil
	ns.signed = true

	return sig, nil
}
