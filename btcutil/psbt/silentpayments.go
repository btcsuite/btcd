package psbt

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

const (
	// outPointSize is the serialized size of an outpoint in bytes.
	outPointSize = sha256.Size + uint32Size
)

var (
	// SilentPaymentDummyP2TROutput is a dummy P2TR output that can be used
	// to mark a transaction output as a silent payment recipient output to
	// which the final Taproot output key hasn't been calculated yet.
	SilentPaymentDummyP2TROutput = append(
		[]byte{txscript.OP_1, txscript.OP_DATA_32},
		bytes.Repeat([]byte{0x00}, 32)...,
	)
)

// SilentPaymentShare is a single ECDH share for a silent payment.
type SilentPaymentShare struct {
	// ScanKey is the silent payment recipient's scan key.
	ScanKey []byte

	// OutPoints is the list of outpoints the share is for.
	OutPoints []wire.OutPoint

	// Share is the sum of all ECDH shares for the outpoints.
	Share []byte
}

// EqualKey returns true if this silent payment share's key data is the same as
// the given silent payment share.
func (s *SilentPaymentShare) EqualKey(other *SilentPaymentShare) bool {
	if !bytes.Equal(s.ScanKey, other.ScanKey) {
		return false
	}

	return equalOutPoints(s.OutPoints, other.OutPoints)
}

// equalOutPoints returns true if the given outpoints are equal.
func equalOutPoints(a, b []wire.OutPoint) bool {
	if len(a) != len(b) {
		return false
	}

	sort.Slice(a, func(i, j int) bool {
		opI := a[i]
		opJ := a[j]
		if opI.Hash == opJ.Hash {
			return opI.Index < opJ.Index
		}

		return bytes.Compare(opI.Hash[:], opJ.Hash[:]) < 0
	})
	sort.Slice(b, func(i, j int) bool {
		opI := b[i]
		opJ := b[j]
		if opI.Hash == opJ.Hash {
			return opI.Index < opJ.Index
		}

		return bytes.Compare(opI.Hash[:], opJ.Hash[:]) < 0
	})

	for i, outPoint := range a {
		if outPoint != b[i] {
			return false
		}
	}

	return true
}

// ReadSilentPaymentShare deserializes a silent payment share from the given key
// data and value.
func ReadSilentPaymentShare(keyData,
	value []byte) (*SilentPaymentShare, error) {

	// There must be at least the scan key in the key data.
	if len(keyData) < secp.PubKeyBytesLenCompressed {
		return nil, ErrInvalidKeyData
	}

	// The remaining bytes of the key data are outpoints.
	outPointsLen := len(keyData) - secp.PubKeyBytesLenCompressed
	if outPointsLen%outPointSize != 0 {
		return nil, ErrInvalidKeyData
	}

	// The share must be a public key.
	if len(value) != secp.PubKeyBytesLenCompressed {
		return nil, ErrInvalidPsbtFormat
	}

	share := &SilentPaymentShare{
		ScanKey:   keyData[:secp.PubKeyBytesLenCompressed],
		OutPoints: make([]wire.OutPoint, outPointsLen/outPointSize),
		Share:     value,
	}

	for i := 0; i < outPointsLen; i += outPointSize {
		idx := i / outPointSize
		copy(share.OutPoints[idx].Hash[:], keyData[i:i+sha256.Size])
		share.OutPoints[idx].Index = binary.LittleEndian.Uint32(
			keyData[i+sha256.Size : i+sha256.Size+uint32Size],
		)
	}

	return share, nil
}

// SerializeSilentPaymentShare serializes a silent payment share to key data and
// value.
func SerializeSilentPaymentShare(share *SilentPaymentShare) ([]byte, []byte) {
	keyData := make(
		[]byte, 0, len(share.ScanKey)+len(share.OutPoints)*outPointSize,
	)
	keyData = append(keyData, share.ScanKey...)
	for _, outPoint := range share.OutPoints {
		keyData = append(keyData, outPoint.Hash[:]...)
		var idx [4]byte
		binary.LittleEndian.PutUint32(idx[:], outPoint.Index)
		keyData = append(keyData, idx[:]...)
	}

	return keyData, share.Share
}

// SilentPaymentDLEQ is a DLEQ proof for a silent payment share.
type SilentPaymentDLEQ struct {
	// ScanKey is the silent payment recipient's scan key.
	ScanKey []byte

	// OutPoints is the list of outpoints the share proof is for.
	OutPoints []wire.OutPoint

	// Proof is the DLEQ proof for the share with the same key.
	Proof []byte
}

// EqualKey returns true if this silent payment DLEQ's key data is the same as
// the given silent payment DLEQ.
func (d *SilentPaymentDLEQ) EqualKey(other *SilentPaymentDLEQ) bool {
	if !bytes.Equal(d.ScanKey, other.ScanKey) {
		return false
	}

	return equalOutPoints(d.OutPoints, other.OutPoints)
}

// ReadSilentPaymentDLEQ deserializes a silent payment DLEQ proof from the given
// key data and value.
func ReadSilentPaymentDLEQ(keyData, value []byte) (*SilentPaymentDLEQ, error) {
	// There must be at least the scan key in the key data.
	if len(keyData) < secp.PubKeyBytesLenCompressed {
		return nil, ErrInvalidKeyData
	}

	// The remaining bytes of the key data are outpoints.
	outPointsLen := len(keyData) - secp.PubKeyBytesLenCompressed
	if outPointsLen%outPointSize != 0 {
		return nil, ErrInvalidKeyData
	}

	// The proof must be 64 bytes.
	if len(value) != 64 {
		return nil, ErrInvalidPsbtFormat
	}

	share := &SilentPaymentDLEQ{
		ScanKey:   keyData[:secp.PubKeyBytesLenCompressed],
		OutPoints: make([]wire.OutPoint, outPointsLen/outPointSize),
		Proof:     value,
	}

	for i := 0; i < outPointsLen; i += outPointSize {
		idx := i / outPointSize
		copy(share.OutPoints[idx].Hash[:], keyData[i:i+sha256.Size])
		share.OutPoints[idx].Index = binary.LittleEndian.Uint32(
			keyData[i+sha256.Size : i+sha256.Size+uint32Size],
		)
	}

	return share, nil
}

// SerializeSilentPaymentDLEQ serializes a silent payment DLEQ proof to key data
// and value.
func SerializeSilentPaymentDLEQ(dleq *SilentPaymentDLEQ) ([]byte, []byte) {
	keyData := make(
		[]byte, 0, len(dleq.ScanKey)+len(dleq.OutPoints)*outPointSize,
	)
	keyData = append(keyData, dleq.ScanKey...)
	for _, outPoint := range dleq.OutPoints {
		keyData = append(keyData, outPoint.Hash[:]...)
		var idx [4]byte
		binary.LittleEndian.PutUint32(idx[:], outPoint.Index)
		keyData = append(keyData, idx[:]...)
	}

	return keyData, dleq.Proof
}

// SilentPaymentInfo is the information needed to create a silent payment
// recipient output.
type SilentPaymentInfo struct {
	// ScanKey is the silent payment recipient's scan key.
	ScanKey []byte

	// SpendKey is the silent payment recipient's spend key.
	SpendKey []byte
}

// ReadSilentPaymentInfo deserializes a silent payment info from the given
// value.
func ReadSilentPaymentInfo(value []byte) (*SilentPaymentInfo, error) {
	if len(value) != secp.PubKeyBytesLenCompressed*2 {
		return nil, ErrInvalidPsbtFormat
	}

	return &SilentPaymentInfo{
		ScanKey:  value[:secp.PubKeyBytesLenCompressed],
		SpendKey: value[secp.PubKeyBytesLenCompressed:],
	}, nil
}

// SerializeSilentPaymentInfo serializes a silent payment info to value.
func SerializeSilentPaymentInfo(info *SilentPaymentInfo) []byte {
	value := make([]byte, 0, secp.PubKeyBytesLenCompressed*2)
	value = append(value, info.ScanKey...)
	value = append(value, info.SpendKey...)

	return value
}
