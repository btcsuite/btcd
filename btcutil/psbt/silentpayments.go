package psbt

import (
	"bytes"

	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
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

	// There key data must be the scan key.
	if len(keyData) != secp.PubKeyBytesLenCompressed {
		return nil, ErrInvalidKeyData
	}

	// The share must be a public key.
	if len(value) != secp.PubKeyBytesLenCompressed {
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
	// The key data must be the scan key.
	if len(keyData) != secp.PubKeyBytesLenCompressed {
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
