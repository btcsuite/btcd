package psbt

import (
	"bytes"

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

	// A hash is 32 bytes in size, so we need at least numHashes*32 + 5
	// bytes to be present.
	if len(value) < (int(numHashes)*32)+5 {
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
