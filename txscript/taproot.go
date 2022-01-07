// Copyright (c) 2013-2022 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	secp "github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// TapscriptLeafVersion represents the various possible versions of a tapscript
// leaf version. Leaf versions are used to define, or introduce new script
// semantics, under the base taproot execution model.
//
// TODO(roasbeef): add validation here as well re proper prefix, etc?
type TapscriptLeafVersion uint8

const (
	// BaseLeafVersion is the base tapscript leaf version. The semantics of
	// this version are defined in BIP 342.
	BaseLeafVersion TapscriptLeafVersion = 0xc0
)

const (
	// ControlBlockBaseSize is the base size of a control block. This
	// includes the initial byte for the leaf version, and then serialized
	// schnorr public key.
	ControlBlockBaseSize = 33

	// ControlBlockNodeSize is the size of a given merkle branch hash in
	// the control block.
	ControlBlockNodeSize = 32

	// ControlBlockMaxNodeCount is the max number of nodes that can be
	// included in a control block. This value represents a merkle tree of
	// depth 2^128.
	ControlBlockMaxNodeCount = 128

	// ControlBlockMaxSize is the max possible size of a control block.
	// This simulates revealing a leaf from the largest possible tapscript
	// tree.
	ControlBlockMaxSize = ControlBlockBaseSize + (ControlBlockNodeSize *
		ControlBlockMaxNodeCount)
)

// VerifyTaprootKeySpend attempts to verify a top-level taproot key spend,
// returning a non-nil error if the passed signature is invalid.  If a sigCache
// is passed in, then the sig cache will be consulted to skip full verification
// of a signature that has already been seen. Witness program here should be
// the 32-byte x-only schnorr output public key.
//
// NOTE: The TxSigHashes MUST be passed in and fully populated.
func VerifyTaprootKeySpend(witnessProgram []byte, rawSig []byte, tx *wire.MsgTx,
	inputIndex int, prevOuts PrevOutputFetcher, hashCache *TxSigHashes,
	sigCache *SigCache) error {

	// First, we'll need to extract the public key from the witness
	// program.
	rawKey := witnessProgram

	// Extract the annex if it exists, so we can compute the proper proper
	// sighash below.
	var annex []byte
	witness := tx.TxIn[inputIndex].Witness
	if isAnnexedWitness(witness) {
		annex, _ = extractAnnex(witness)
	}

	// Now that we have the public key, we can create a new top-level
	// keyspend verifier that'll handle all the sighash and schnorr
	// specifics for us.
	keySpendVerifier, err := newTaprootSigVerifier(
		rawKey, rawSig, tx, inputIndex, prevOuts, sigCache,
		hashCache, annex,
	)
	if err != nil {
		return err
	}

	valid := keySpendVerifier.Verify()
	if valid {
		return nil
	}

	// TODO(roasbeef): add proper error
	return fmt.Errorf("invalid sig")
}

// ControlBlock houses the structured witness input for a taproot spend. This
// includes the internal taproot key, the leaf version, and finally a nearly
// complete merkle inclusion proof for the main taproot commitment.
//
// TODO(roasbeef): method to serialize control block that commits to even
// y-bit, which pops up everywhere even tho 32 byte keys
type ControlBlock struct {
	// InternalKey is the internal public key in the taproot commitment.
	InternalKey *secp.PublicKey

	// OutputKeyYIsOdd denotes if the y coordinate of the output key (the
	// key placed in the actual taproot output is odd.
	OutputKeyYIsOdd bool

	// LeafVersion is the specified leaf version of the tapscript leaf that
	// the InclusionProof below is based off of.
	LeafVersion TapscriptLeafVersion

	// InclusionProof is a series of merkle branches that when hashed
	// pairwise, starting with the revealed script, will yield the taproot
	// commitment root.
	InclusionProof []byte
}

// ToBytes returns the control block in a format suitable for using as part of
// a witness spending a tapscript output.
func (c *ControlBlock) ToBytes() ([]byte, error) {
	var b bytes.Buffer

	// The first byte of the control block is the leaf version byte XOR'd with
	// the parity of the y coordinate of the public key.
	yParity := byte(0)
	if c.OutputKeyYIsOdd {
		yParity = 1
	}

	// The first byte is a combination of the leaf version, using the lowest
	// bit to encode the single bit that denotes if the yo coordinate if odd or
	// even.
	leafVersionAndParity := byte(c.LeafVersion) | yParity
	if err := b.WriteByte(leafVersionAndParity); err != nil {
		return nil, err
	}

	// Next, we encode the raw 32 byte schnorr public key
	if _, err := b.Write(schnorr.SerializePubKey(c.InternalKey)); err != nil {
		return nil, err
	}

	// Finally, we'll write out the inclusion proof as is, without any length
	// prefix.
	if _, err := b.Write(c.InclusionProof); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

// RootHash calculates the root hash of a tapscript given the revealed script.
func (c *ControlBlock) RootHash(revealedScript []byte) []byte {
	// We'll start by creating a new tapleaf from the revealed script,
	// this'll serve as the initial hash we'll use to incrementally
	// reconstruct the merkle root using the control block elements.
	merkleAccumulator := NewTapLeaf(c.LeafVersion, revealedScript).TapHash()

	// Now that we have our initial hash, we'll parse the control block one
	// node at a time to build up our merkle accumulator into the taproot
	// commitment.
	//
	// The control block is a series of nodes that serve as an inclusion
	// proof as we can start hashing with our leaf, with each internal
	// branch, until we reach the root.
	numNodes := len(c.InclusionProof) / ControlBlockNodeSize
	for nodeOffset := 0; nodeOffset < numNodes; nodeOffset++ {
		// Extract the new node using our index to serve as a 32-byte
		// offset.
		leafOffset := 32 * nodeOffset
		nextNode := c.InclusionProof[leafOffset : leafOffset+32]

		merkleAccumulator = tapBranchHash(merkleAccumulator[:], nextNode)
	}

	return merkleAccumulator[:]
}

// ParseControlBlock attempts to parse the raw bytes of a control block. An
// error is returned if the control block isn't well formed, or can't be
// parsed.
func ParseControlBlock(ctrlBlock []byte) (*ControlBlock, error) {
	// The control block minimally must contain 33 bytes (for the leaf
	// version and internal key) along with at least a single value
	// comprising the merkle proof. If not, then it's invalid.
	switch {
	// The control block must minimally have 33 bytes for the internal
	// public key and script leaf version.
	case len(ctrlBlock) < ControlBlockBaseSize:
		return nil, fmt.Errorf("invalid control block size")

	// The control block can't be larger than a proof for the largest
	// possible tapscript merkle tree with 2^128 leaves.
	case len(ctrlBlock) > ControlBlockMaxSize:
		return nil, fmt.Errorf("invalid max block size")

	// Ignoring the fixed sized portion, we expect the total number of
	// remaining bytes to be a multiple of the node size, which is 32
	// bytes.
	case (len(ctrlBlock)-ControlBlockBaseSize)%ControlBlockNodeSize != 0:
		return nil, fmt.Errorf("invalid max block size")
	}

	// With the basic sanity checking complete, we can now parse the
	// control block.
	leafVersion := TapscriptLeafVersion(ctrlBlock[0] & TaprootLeafMask)

	// Extract the parity of the y coordinate of the internal key.
	var yIsOdd bool
	if ctrlBlock[0]&0x01 == 0x01 {
		yIsOdd = true
	}

	// Next, we'll parse the public key, which is the 32 bytes following
	// the leaf version.
	rawKey := ctrlBlock[1:33]
	pubKey, err := schnorr.ParsePubKey(rawKey)
	if err != nil {
		return nil, err
	}

	// The rest of the bytes are the control block itself, which encodes a
	// merkle proof of inclusion.
	proofBytes := ctrlBlock[33:]

	return &ControlBlock{
		InternalKey:     pubKey,
		OutputKeyYIsOdd: yIsOdd,
		LeafVersion:     leafVersion,
		InclusionProof:  proofBytes,
	}, nil
}

// ComputeTaprootOutputKey calculates a top-level taproot output key given an
// internal key, and tapscript merkle root. The final key is derived as:
// taprootKey = internalKey + (h_tapTweak(internalKey || merkleRoot)*G).
func ComputeTaprootOutputKey(pubKey *btcec.PublicKey,
	scriptRoot []byte) *btcec.PublicKey {

	// This routine only operates on x-only public keys where the public
	// key always has an even y coordinate, so we'll re-parse it as such.
	internalKey, _ := schnorr.ParsePubKey(schnorr.SerializePubKey(pubKey))

	// First, we'll compute the tap tweak hash that commits to the internal
	// key and the merkle script root.
	tapTweakHash := chainhash.TaggedHash(
		chainhash.TagTapTweak, schnorr.SerializePubKey(internalKey),
		scriptRoot,
	)

	// With the tap tweek computed,  we'll need to convert the merkle root
	// into something in the domain we can manipulate: a scalar value mod
	// N.
	var tweakScalar btcec.ModNScalar
	tweakScalar.SetBytes((*[32]byte)(tapTweakHash))

	// Next, we'll need to convert the internal key to jacobian coordinates
	// as the routines we need only operate on this type.
	var internalPoint btcec.JacobianPoint
	internalKey.AsJacobian(&internalPoint)

	// With our intermediate data obtained, we'll now compute:
	//
	// taprootKey = internalPoint + (tapTweak*G).
	var tPoint, taprootKey btcec.JacobianPoint
	btcec.ScalarBaseMultNonConst(&tweakScalar, &tPoint)
	btcec.AddNonConst(&internalPoint, &tPoint, &taprootKey)

	// Finally, we'll convert the key back to affine coordinates so we can
	// return the format of public key we usually use.
	taprootKey.ToAffine()

	return btcec.NewPublicKey(&taprootKey.X, &taprootKey.Y)
}

// ComputeTaprootKeyNoScript calculates the top-level taproot output key given
// an internal key, and a desire that the only way an output can be spent is
// with the keyspend path. This is useful for normal wallet operations that
// don't need any other additional spending conditions.
func ComputeTaprootKeyNoScript(internalKey *btcec.PublicKey) *btcec.PublicKey {
	// We'll compute a custom tap tweak hash that just commits to the key,
	// rather than an actual root hash.
	fakeScriptroot := []byte{}

	return ComputeTaprootOutputKey(internalKey, fakeScriptroot)
}

// TweakTaprootPrivKey applies the same operation as ComputeTaprootOutputKey,
// but on the private key instead. The final key is derived as: privKey +
// h_tapTweak(internalKey || merkleRoot) % N, where N is the order of the
// secp256k1 curve, and merkleRoot is the root hash of the tapscript tree.
func TweakTaprootPrivKey(privKey *btcec.PrivateKey,
	scriptRoot []byte) *btcec.PrivateKey {

	// If the corresponding public key has an odd y coordinate, then we'll
	// negate the private key as specified in BIP 341.
	privKeyScalar := &privKey.Key
	pubKeyBytes := privKey.PubKey().SerializeCompressed()
	if pubKeyBytes[0] == btcec.PubKeyFormatCompressedOdd {
		privKeyScalar.Negate()
	}

	// Next, we'll compute the tap tweak hash that commits to the internal
	// key and the merkle script root. We'll snip off the extra parity byte
	// from the compressed serialization and use that directly.
	schnorrKeyBytes := pubKeyBytes[1:]
	tapTweakHash := chainhash.TaggedHash(
		chainhash.TagTapTweak, schnorrKeyBytes, scriptRoot,
	)

	// Map the private key to a ModNScalar which is needed to perform
	// operation mod the curve order.
	var tweakScalar btcec.ModNScalar
	tweakScalar.SetBytes((*[32]byte)(tapTweakHash))

	// Now that we have the private key in its may negated form, we'll add
	// the script root as a tweak. As we're using a ModNScalar all
	// operations are already normalized mod the curve order.
	privTweak := privKeyScalar.Add(&tweakScalar)

	return btcec.PrivKeyFromScalar(privTweak)
}

// VerifyTaprootLeafCommitment attempts to verify a taproot commitment of the
// revealed script within the taprootWitnessProgram (a schnorr public key)
// given the required information included in the control block. An error is
// returned if the reconstructed taproot commitment (a function of the merkle
// root and the internal key) doesn't match the passed witness program.
func VerifyTaprootLeafCommitment(controlBlock *ControlBlock,
	taprootWitnessProgram []byte, revealedScript []byte) error {

	// First, we'll calculate the root hash from the given proof and
	// revealed script.
	rootHash := controlBlock.RootHash(revealedScript)

	// Next, we'll construct the final commitment (creating the external or
	// taproot output key) as a function of this commitment and the
	// included internal key: taprootKey = internalKey + (tPoint*G).
	taprootKey := ComputeTaprootOutputKey(
		controlBlock.InternalKey, rootHash,
	)

	// If we convert the taproot key to a witness program (we just need to
	// serialize the public key), then it should exactly match the witness
	// program passed in.
	expectedWitnessProgram := schnorr.SerializePubKey(taprootKey)
	if !bytes.Equal(expectedWitnessProgram, taprootWitnessProgram) {
		return fmt.Errorf("invalid witness commitment")
	}

	// Otherwise, if we reach here, the commitment opening is valid and
	// execution can continue.
	return nil
}

// TapNode represents an abstract node in a tapscript merkle tree. A node is
// either a branch or a leaf.
type TapNode interface {
	// TapHash returns the hash of the node. This will either be a tagged
	// hash derived from a branch, or a leaf.
	TapHash() chainhash.Hash

	// Left returns the left node. If this is a leaf node, this may be nil.
	Left() TapNode

	// Right returns the right node. If this is a leaf node, this may be
	// nil.
	Right() TapNode
}

// TapLeaf represents a leaf in a tapscript tree. A leaf has two components:
// the leaf version, and the script associated with that leaf version.
type TapLeaf struct {
	// LeafVersion is the leaf version of this leaf.
	LeafVersion TapscriptLeafVersion

	// Script is the script to be validated based on the specified leaf
	// version.
	Script []byte
}

// Left rights the left node for this leaf. As this is a leaf the left node is
// nil.
func (t TapLeaf) Left() TapNode {
	return nil
}

// Right rights the right node for this leaf. As this is a leaf the right node
// is nil.
func (t TapLeaf) Right() TapNode {
	return nil
}

// NewBaseTapLeaf returns a new TapLeaf for the specified script, using the
// current base leaf version (BIP 342).
func NewBaseTapLeaf(script []byte) TapLeaf {
	return TapLeaf{
		Script:      script,
		LeafVersion: BaseLeafVersion,
	}
}

// NewTapLeaf returns a new TapLeaf with the given leaf version and script to
// be committed to.
func NewTapLeaf(leafVersion TapscriptLeafVersion, script []byte) TapLeaf {
	return TapLeaf{
		LeafVersion: leafVersion,
		Script:      script,
	}
}

// TapHash returns the hash digest of the target taproot script leaf. The
// digest is computed as: h_tapleaf(leafVersion || compactSizeof(script) ||
// script).
func (t TapLeaf) TapHash() chainhash.Hash {
	// The leaf encoding is: leafVersion || compactSizeof(script) ||
	// script, where compactSizeof returns the compact size needed to
	// encode the value.
	var leafEncoding bytes.Buffer

	_ = leafEncoding.WriteByte(byte(t.LeafVersion))
	_ = wire.WriteVarBytes(&leafEncoding, 0, t.Script)

	return *chainhash.TaggedHash(chainhash.TagTapLeaf, leafEncoding.Bytes())
}

// TapBranch represents an internal branch in the tapscript tree. The left or
// right nodes may either be another branch, leaves, or a combination of both.
type TapBranch struct {
	// leftNode is the left node, this cannot be nil.
	leftNode TapNode

	// rightNode is the right node, this cannot be nil.
	rightNode TapNode
}

// NewTapBranch creates a new internal branch from a left and right node.
func NewTapBranch(l, r TapNode) TapBranch {

	return TapBranch{
		leftNode:  l,
		rightNode: r,
	}
}

// Left is the left node of the branch, this might be a leaf or another
// branch.
func (t TapBranch) Left() TapNode {
	return t.leftNode
}

// Right is the right node of a branch, this might be a leaf or another branch.
func (t TapBranch) Right() TapNode {
	return t.rightNode
}

// TapHash returns the hash digest of the taproot internal branch given a left
// and right node. The final hash digest is: h_tapbranch(leftNode ||
// rightNode), where leftNode is the lexicographically smaller of the two nodes.
func (t TapBranch) TapHash() chainhash.Hash {
	leftHash := t.leftNode.TapHash()
	rightHash := t.rightNode.TapHash()
	return tapBranchHash(leftHash[:], rightHash[:])
}

// tapBranchHash takes the raw tap hashes of the right and left nodes and
// hashes them into a branch. See The TapBranch method for the specifics.
func tapBranchHash(l, r []byte) chainhash.Hash {
	if bytes.Compare(l[:], r[:]) > 0 {
		l = r
		r = l
	}

	return *chainhash.TaggedHash(
		chainhash.TagTapBranch, l[:], r[:],
	)
}

// TapscriptProof is a proof of inclusion that a given leaf (a script and leaf
// version) is included within a top-level taproot output commitment.
type TapscriptProof struct {
	// TapLeaf is the leaf that we want to prove inclusion for.
	TapLeaf

	// InclusionProof is the tail end of the control block that contains
	// the series of hashes (the sibling hashes up the tree), that when
	// hashed together allow us to re-derive the top level taproot output.
	InclusionProof []byte
}

// ToControlBlock maps the tapscript proof into a fully valid control block
// that can be used as a witness item for a tapscript spend.
func (t *TapscriptProof) ToControlBlock(internalKey *btcec.PublicKey) ControlBlock {
	return ControlBlock{
		InternalKey:    internalKey,
		LeafVersion:    t.TapLeaf.LeafVersion,
		InclusionProof: t.InclusionProof,
	}
}
