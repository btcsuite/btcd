// Copyright (c) 2013-2022 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"fmt"

	secp "github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
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
