package psbt

import (
	"bytes"
	"encoding/binary"

	"github.com/btcsuite/btcd/btcutil/base58"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

const (
	// uint32Size is the size of a uint32 in bytes.
	uint32Size = 4
)

// Bip32Derivation encapsulates the data for the input and output
// Bip32Derivation key-value fields.
//
// TODO(roasbeef): use hdkeychain here instead?
type Bip32Derivation struct {
	// PubKey is the raw pubkey serialized in compressed format.
	PubKey []byte

	// MasterKeyFingerprint is the fingerprint of the master pubkey.
	MasterKeyFingerprint uint32

	// Bip32Path is the BIP 32 path with child index as a distinct integer.
	Bip32Path []uint32
}

// checkValid ensures that the PubKey in the Bip32Derivation struct is valid.
func (pb *Bip32Derivation) checkValid() bool {
	return validatePubkey(pb.PubKey)
}

// Bip32Sorter implements sort.Interface for the Bip32Derivation struct.
type Bip32Sorter []*Bip32Derivation

func (s Bip32Sorter) Len() int { return len(s) }

func (s Bip32Sorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s Bip32Sorter) Less(i, j int) bool {
	return bytes.Compare(s[i].PubKey, s[j].PubKey) < 0
}

// ReadBip32Derivation deserializes a byte slice containing chunks of 4 byte
// little endian encodings of uint32 values, the first of which is the
// MasterKeyFingerprint and the remainder of which are the derivation path.
func ReadBip32Derivation(path []byte) (uint32, []uint32, error) {
	// BIP-0174 defines the derivation path being encoded as
	//   "<32-bit uint> <32-bit uint>*"
	// with the asterisk meaning 0 to n times. Which in turn means that an
	// empty path is valid, only the key fingerprint is mandatory.
	if len(path) < uint32Size || len(path)%uint32Size != 0 {
		return 0, nil, ErrInvalidPsbtFormat
	}

	masterKeyInt := binary.LittleEndian.Uint32(path[:uint32Size])

	var paths []uint32
	for i := uint32Size; i < len(path); i += uint32Size {
		paths = append(paths, binary.LittleEndian.Uint32(
			path[i:i+uint32Size],
		))
	}

	return masterKeyInt, paths, nil
}

// SerializeBIP32Derivation takes a master key fingerprint as defined in BIP32,
// along with a path specified as a list of uint32 values, and returns a
// bytestring specifying the derivation in the format required by BIP174: //
// master key fingerprint (4) || child index (4) || child index (4) || ....
func SerializeBIP32Derivation(masterKeyFingerprint uint32,
	bip32Path []uint32) []byte {

	var masterKeyBytes [uint32Size]byte
	binary.LittleEndian.PutUint32(masterKeyBytes[:], masterKeyFingerprint)

	derivationPath := make([]byte, 0, uint32Size+uint32Size*len(bip32Path))
	derivationPath = append(derivationPath, masterKeyBytes[:]...)
	for _, path := range bip32Path {
		var pathBytes [uint32Size]byte
		binary.LittleEndian.PutUint32(pathBytes[:], path)
		derivationPath = append(derivationPath, pathBytes[:]...)
	}

	return derivationPath
}

// XPub is a struct that encapsulates an extended public key, as defined in
// BIP-0032.
type XPub struct {
	// ExtendedKey is the serialized extended public key as defined in
	// BIP-0032.
	ExtendedKey []byte

	// MasterFingerprint is the fingerprint of the master pubkey.
	MasterKeyFingerprint uint32

	// Bip32Path is the derivation path of the key, with hardened elements
	// having the 0x80000000 offset added, as defined in BIP-0032. The
	// number of path elements must match the depth provided in the extended
	// public key.
	Bip32Path []uint32
}

// ReadXPub deserializes a byte slice containing an extended public key and a
// BIP-0032 derivation path.
func ReadXPub(keyData []byte, path []byte) (*XPub, error) {
	xPub, err := DecodeExtendedKey(keyData)
	if err != nil {
		return nil, ErrInvalidPsbtFormat
	}
	numPathElements := xPub.Depth()

	// The path also contains the master key fingerprint,
	expectedSize := int(uint32Size * (numPathElements + 1))
	if len(path) != expectedSize {
		return nil, ErrInvalidPsbtFormat
	}

	masterKeyFingerprint, bip32Path, err := ReadBip32Derivation(path)
	if err != nil {
		return nil, err
	}

	return &XPub{
		ExtendedKey:          keyData,
		MasterKeyFingerprint: masterKeyFingerprint,
		Bip32Path:            bip32Path,
	}, nil
}

// EncodeExtendedKey serializes an extended key to a byte slice, without the
// checksum.
func EncodeExtendedKey(key *hdkeychain.ExtendedKey) []byte {
	serializedKey := key.String()
	decodedKey := base58.Decode(serializedKey)
	return decodedKey[:len(decodedKey)-uint32Size]
}

// DecodeExtendedKey deserializes an extended key from a byte slice that does
// not contain the checksum.
func DecodeExtendedKey(encodedKey []byte) (*hdkeychain.ExtendedKey, error) {
	checkSum := chainhash.DoubleHashB(encodedKey)[:uint32Size]
	serializedBytes := append(encodedKey, checkSum...)
	xPub, err := hdkeychain.NewKeyFromString(base58.Encode(serializedBytes))
	if err != nil {
		return nil, err
	}

	return xPub, nil
}
