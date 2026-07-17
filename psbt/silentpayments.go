package psbt

import (
	"bytes"
	"encoding/binary"

	"github.com/btcsuite/btcd/txscript/v2"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
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

	// Share is either the sum of all ECDH shares of all eligible inputs or
	// the ECDH share of a single input (depending on the context of the
	// share, whether it's specified as a global field or as a field on a
	// single input).
	Share []byte
}

// EqualKey returns true if this silent payment share's key data is the same as
// the given silent payment share.
func (s *SilentPaymentShare) EqualKey(other *SilentPaymentShare) bool {
	return bytes.Equal(s.ScanKey, other.ScanKey)
}

// ReadSilentPaymentShare deserializes a silent payment share from the given key
// data and value.
func ReadSilentPaymentShare(keyData,
	value []byte) (*SilentPaymentShare, error) {

	// The key data must be the scan key, a valid compressed public key.
	if len(keyData) != secp.PubKeyBytesLenCompressed ||
		!validatePubkey(keyData) {

		return nil, ErrInvalidKeyData
	}

	// The share must be a valid compressed public key.
	if len(value) != secp.PubKeyBytesLenCompressed ||
		!validatePubkey(value) {

		return nil, ErrInvalidPsbtFormat
	}

	return &SilentPaymentShare{
		ScanKey: keyData[:secp.PubKeyBytesLenCompressed],
		Share:   value,
	}, nil
}

// SerializeSilentPaymentShare serializes a silent payment share to key data and
// value.
func SerializeSilentPaymentShare(share *SilentPaymentShare) ([]byte, []byte) {
	return share.ScanKey, share.Share
}

// SilentPaymentDLEQ is a DLEQ proof for a silent payment share.
type SilentPaymentDLEQ struct {
	// ScanKey is the silent payment recipient's scan key.
	ScanKey []byte

	// Proof is the DLEQ proof for the share with the same key.
	Proof []byte
}

// EqualKey returns true if this silent payment DLEQ's key data is the same as
// the given silent payment DLEQ.
func (d *SilentPaymentDLEQ) EqualKey(other *SilentPaymentDLEQ) bool {
	if !bytes.Equal(d.ScanKey, other.ScanKey) {
		return false
	}

	return true
}

// ReadSilentPaymentDLEQ deserializes a silent payment DLEQ proof from the given
// key data and value.
func ReadSilentPaymentDLEQ(keyData, value []byte) (*SilentPaymentDLEQ, error) {
	// The key data must be the scan key, a valid compressed public key.
	if len(keyData) != secp.PubKeyBytesLenCompressed ||
		!validatePubkey(keyData) {

		return nil, ErrInvalidKeyData
	}

	// The proof must be 64 bytes.
	if len(value) != 64 {
		return nil, ErrInvalidPsbtFormat
	}

	share := &SilentPaymentDLEQ{
		ScanKey: keyData[:secp.PubKeyBytesLenCompressed],
		Proof:   value,
	}

	return share, nil
}

// SerializeSilentPaymentDLEQ serializes a silent payment DLEQ proof to key data
// and value.
func SerializeSilentPaymentDLEQ(dleq *SilentPaymentDLEQ) ([]byte, []byte) {
	keyData := append([]byte{}, dleq.ScanKey...)

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

	scanKey := value[:secp.PubKeyBytesLenCompressed]
	spendKey := value[secp.PubKeyBytesLenCompressed:]

	// Both the scan and spend keys must be valid compressed public keys.
	if !validatePubkey(scanKey) || !validatePubkey(spendKey) {
		return nil, ErrInvalidPsbtFormat
	}

	return &SilentPaymentInfo{
		ScanKey:  scanKey,
		SpendKey: spendKey,
	}, nil
}

// SerializeSilentPaymentInfo serializes a silent payment info to value.
func SerializeSilentPaymentInfo(info *SilentPaymentInfo) []byte {
	value := make([]byte, 0, secp.PubKeyBytesLenCompressed*2)
	value = append(value, info.ScanKey...)
	value = append(value, info.SpendKey...)

	return value
}

// ReadSilentPaymentLabel deserializes a silent payment output label from the
// given key data and value. The key data must be empty and the value must be a
// 32-bit little-endian integer.
func ReadSilentPaymentLabel(value []byte) (uint32, error) {
	if len(value) != uint32Size {
		return 0, ErrInvalidPsbtFormat
	}

	return binary.LittleEndian.Uint32(value), nil
}
