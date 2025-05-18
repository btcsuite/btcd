package psbt

import (
	"bytes"
	"math"
	"math/bits"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

const (
	// schnorrSigMinLength is the minimum length of a Schnorr signature
	// which is 64 bytes.
	schnorrSigMinLength = schnorr.SignatureSize

	// schnorrSigMaxLength is the maximum length of a Schnorr signature
	// which is 64 bytes plus one byte for the appended sighash flag.
	schnorrSigMaxLength = schnorrSigMinLength + 1
)

// TaprootScriptSpendSig encapsulates an individual Schnorr signature for a
// given public key and leaf hash.
type TaprootScriptSpendSig struct {
	XOnlyPubKey []byte
	LeafHash    []byte
	Signature   []byte
	SigHash     txscript.SigHashType
}

// checkValid checks that both the pubkey and the signature are valid.
func (s *TaprootScriptSpendSig) checkValid() bool {
	return validateXOnlyPubkey(s.XOnlyPubKey) &&
		validateSchnorrSignature(s.Signature)
}

// EqualKey returns true if this script spend signature's key data is the same
// as the given script spend signature.
func (s *TaprootScriptSpendSig) EqualKey(other *TaprootScriptSpendSig) bool {
	return bytes.Equal(s.XOnlyPubKey, other.XOnlyPubKey) &&
		bytes.Equal(s.LeafHash, other.LeafHash)
}

// SortBefore returns true if this script spend signature's key is
// lexicographically smaller than the given other script spend signature's key
// and should come first when being sorted.
func (s *TaprootScriptSpendSig) SortBefore(other *TaprootScriptSpendSig) bool {
	return bytes.Compare(s.XOnlyPubKey, other.XOnlyPubKey) < 0 &&
		bytes.Compare(s.LeafHash, other.LeafHash) < 0
}

// TaprootTapLeafScript represents a single taproot leaf script that is
// identified by its control block.
type TaprootTapLeafScript struct {
	ControlBlock []byte
	Script       []byte
	LeafVersion  txscript.TapscriptLeafVersion
}

// checkValid checks that the control block is valid.
func (s *TaprootTapLeafScript) checkValid() bool {
	return validateControlBlock(s.ControlBlock)
}

// SortBefore returns true if this leaf script's key is lexicographically
// smaller than the given other leaf script's key and should come first when
// being sorted.
func (s *TaprootTapLeafScript) SortBefore(other *TaprootTapLeafScript) bool {
	return bytes.Compare(s.ControlBlock, other.ControlBlock) < 0
}

// TaprootBip32Derivation encapsulates the data for the input and output
// taproot specific BIP-32 derivation key-value fields.
type TaprootBip32Derivation struct {
	// XOnlyPubKey is the raw public key serialized in the x-only BIP-340
	// format.
	XOnlyPubKey []byte

	// LeafHashes is a list of leaf hashes that the given public key is
	// involved in.
	LeafHashes [][]byte

	// MasterKeyFingerprint is the fingerprint of the master pubkey.
	MasterKeyFingerprint uint32

	// Bip32Path is the BIP 32 path with child index as a distinct integer.
	Bip32Path []uint32
}

// SortBefore returns true if this derivation info's key is lexicographically
// smaller than the given other derivation info's key and should come first when
// being sorted.
func (s *TaprootBip32Derivation) SortBefore(other *TaprootBip32Derivation) bool {
	return bytes.Compare(s.XOnlyPubKey, other.XOnlyPubKey) < 0
}

// minTaprootBip32DerivationByteSize returns the minimum number of bytes
// required to encode a Taproot BIP32 derivation field, given the number of
// leaf hashes.
//
// NOTE: This function does not account for the size of the BIP32 child indexes,
// as we are only computing the minimum size (which occurs when the path is
// empty). The bits package is used to safely detect and handle overflows.
func minTaprootBip32DerivationByteSize(numHashes uint64) (uint64, error) {
	// The Taproot BIP32 derivation field is encoded as:
	//   [compact size uint: number of leaf hashes]
	//   [N × 32 bytes: leaf hashes]
	//   [4 bytes: master key fingerprint]
	//   [M × 4 bytes: BIP32 child indexes]
	//
	// To compute the minimum size given the number of hashes only, we assume:
	// - N = numHashes (provided)
	// - M = 0 (no child indexes)
	//
	// So the base byte size is:
	//   1 (leaf hash count) + (N × 32) + 4 (fingerprint)
	//
	// First, we calculate the total number of bytes for the leaf hashes.
	mulCarry, totalHashesBytes := bits.Mul64(numHashes, 32)
	if mulCarry != 0 {
		return 0, ErrInvalidPsbtFormat
	}

	// Since we're computing the minimum possible size, we add a constant that
	// accounts for the fixed size fields:
	// * 1 byte for the compact size leaf hash count (assumes numHashes < 0xfd)
	// * 4 bytes for the master key fingerprint
	// Total: 5 bytes.
	// All other fields (e.g., BIP32 path) are assumed absent for minimum size
	// calculation.
	result, addCarry := bits.Add64(5, totalHashesBytes, 0)
	if addCarry != 0 {
		return 0, ErrInvalidPsbtFormat
	}

	return result, nil
}

// ReadTaprootBip32Derivation deserializes a byte slice containing the Taproot
// BIP32 derivation info that consists of a list of leaf hashes as well as the
// normal BIP32 derivation info.
func ReadTaprootBip32Derivation(xOnlyPubKey,
	value []byte) (*TaprootBip32Derivation, error) {

	// The taproot key BIP 32 derivation path is defined as:
	//   <hashes len> <leaf hash>* <4 byte fingerprint> <32-bit uint>*
	// So we get at least 5 bytes for the length and the 4 byte fingerprint.
	if len(value) < 5 {
		return nil, ErrInvalidPsbtFormat
	}

	// The first element is the number of hashes that will follow.
	reader := bytes.NewReader(value)
	numHashes, err := wire.ReadVarInt(reader, 0)
	if err != nil {
		return nil, ErrInvalidPsbtFormat
	}

	// As a safety/sanity check, verify that the hash count fits in a `uint32`.
	// This isn’t mandated by BIP‑371, but it prevents overflow and limits
	// derivations to about 137 GiB of data.
	if numHashes > math.MaxUint32 {
		return nil, ErrInvalidPsbtFormat
	}

	// Given the number of hashes, we can calculate the minimum byte size
	// of the taproot BIP32 derivation.
	minByteSize, err := minTaprootBip32DerivationByteSize(numHashes)
	if err != nil {
		return nil, err
	}

	// Ensure that value is at least the minimum size.
	if uint64(len(value)) < minByteSize {
		return nil, ErrInvalidPsbtFormat
	}

	derivation := TaprootBip32Derivation{
		XOnlyPubKey: xOnlyPubKey,
		LeafHashes:  make([][]byte, int(numHashes)),
	}

	for i := 0; i < int(numHashes); i++ {
		derivation.LeafHashes[i] = make([]byte, 32)
		n, err := reader.Read(derivation.LeafHashes[i])
		if err != nil || n != 32 {
			return nil, ErrInvalidPsbtFormat
		}
	}

	// Extract the remaining bytes from the reader (we don't actually know
	// how many bytes we read due to the compact size integer at the
	// beginning).
	var leftoverBuf bytes.Buffer
	_, err = reader.WriteTo(&leftoverBuf)
	if err != nil {
		return nil, err
	}

	// Read the BIP32 derivation info.
	fingerprint, path, err := ReadBip32Derivation(leftoverBuf.Bytes())
	if err != nil {
		return nil, err
	}

	derivation.MasterKeyFingerprint = fingerprint
	derivation.Bip32Path = path

	return &derivation, nil
}

// SerializeTaprootBip32Derivation serializes a TaprootBip32Derivation to its
// raw byte representation.
func SerializeTaprootBip32Derivation(d *TaprootBip32Derivation) ([]byte,
	error) {

	var buf bytes.Buffer

	// The taproot key BIP 32 derivation path is defined as:
	//   <hashes len> <leaf hash>* <4 byte fingerprint> <32-bit uint>*
	err := wire.WriteVarInt(&buf, 0, uint64(len(d.LeafHashes)))
	if err != nil {
		return nil, ErrInvalidPsbtFormat
	}

	for _, hash := range d.LeafHashes {
		n, err := buf.Write(hash)
		if err != nil || n != 32 {
			return nil, ErrInvalidPsbtFormat
		}
	}

	_, err = buf.Write(SerializeBIP32Derivation(
		d.MasterKeyFingerprint, d.Bip32Path,
	))
	if err != nil {
		return nil, ErrInvalidPsbtFormat
	}

	return buf.Bytes(), nil
}

// validateXOnlyPubkey checks if pubKey is *any* valid pubKey serialization in a
// BIP-340 context (x-only serialization).
func validateXOnlyPubkey(pubKey []byte) bool {
	_, err := schnorr.ParsePubKey(pubKey)
	return err == nil
}

// validateSchnorrSignature checks that the passed byte slice is a valid Schnorr
// signature, _NOT_ including the sighash flag. It does *not* of course
// validate the signature against any message or public key.
func validateSchnorrSignature(sig []byte) bool {
	_, err := schnorr.ParseSignature(sig)
	return err == nil
}

// validateControlBlock checks that the passed byte slice is a valid control
// block as it would appear in a BIP-341 witness stack as the last element.
func validateControlBlock(controlBlock []byte) bool {
	_, err := txscript.ParseControlBlock(controlBlock)
	return err == nil
}
