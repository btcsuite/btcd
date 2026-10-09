// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
)

var (
	// ErrMissingKey is returned if one of the structs is missing a public
	// key (the error is always wrapped with the name of the public key that
	// is missing).
	ErrMissingKey = errors.New("missing public key")

	// ErrInvalidTapLeafHash is returned if the length of the tap leaf hash
	// is neither zero (not set) or 32 bytes long.
	ErrInvalidTapLeafHash = errors.New("invalid value for tap leaf hash")

	// ErrMissingPartialSignature is returned if a partial signature is
	// missing.
	ErrMissingPartialSignature = errors.New("missing partial signature")
)

// MuSig2Participants represents a set of participants in a MuSig2 signing
// session.
type MuSig2Participants struct {
	// AggregateKey is the plain (non-tweaked) aggregate public key of all
	// participants, from the `KeyAgg` algorithm as described in the MuSig2
	// BIP. This key may or may not be in the script directly (x-only). It
	// may instead be a parent public key from which the public key in the
	// script were derived.
	AggregateKey *btcec.PublicKey

	// Keys is a list of the public keys of the participants in the MuSig2
	// aggregate key in the order required for aggregation. If sorting was
	// done, then the keys must be in the sorted order.
	Keys []*btcec.PublicKey
}

// Validate asserts the data contained within the MuSig2Participants is sane for
// being serialized into a PSBT packet.
func (m *MuSig2Participants) Validate() error {
	if m == nil {
		return ErrInvalidKeyData
	}

	if m.AggregateKey == nil {
		return fmt.Errorf("aggregate key: %w", ErrMissingKey)
	}

	// An empty list of participants would be serialized as a zero-length
	// value, which the read path rejects as invalid.
	if len(m.Keys) == 0 {
		return fmt.Errorf("participant keys: %w", ErrMissingKey)
	}

	for idx, key := range m.Keys {
		if key == nil {
			return fmt.Errorf("participant key %d: %w", idx,
				ErrMissingKey)
		}
	}

	return nil
}

// KeyData returns the key data for the MuSig2Participants struct.
func (m *MuSig2Participants) KeyData() []byte {
	return m.AggregateKey.SerializeCompressed()
}

// ReadMuSig2Participants reads a set of MuSig2 participants from a key-value
// pair in a PSBT.
func ReadMuSig2Participants(keyData,
	value []byte) (*MuSig2Participants, error) {

	if len(keyData) != btcec.PubKeyBytesLenCompressed {
		return nil, ErrInvalidKeyData
	}

	if len(value) == 0 || len(value)%btcec.PubKeyBytesLenCompressed != 0 {
		return nil, ErrInvalidPsbtFormat
	}

	numKeys := len(value) / btcec.PubKeyBytesLenCompressed
	participants := &MuSig2Participants{
		Keys: make([]*btcec.PublicKey, numKeys),
	}

	var err error
	participants.AggregateKey, err = btcec.ParsePubKey(keyData)
	if err != nil {
		return nil, err
	}

	for idx := range numKeys {
		start := idx * btcec.PubKeyBytesLenCompressed
		participants.Keys[idx], err = btcec.ParsePubKey(
			value[start : start+btcec.PubKeyBytesLenCompressed],
		)
		if err != nil {
			return nil, err
		}
	}

	return participants, nil
}

// SerializeMuSig2Participants serializes a set of MuSig2 participants to a
// key-value pair in a PSBT.
func SerializeMuSig2Participants(w io.Writer, typ uint8,
	participants *MuSig2Participants) error {

	// Make sure what we write would also be accepted again when reading.
	if err := participants.Validate(); err != nil {
		return err
	}

	value := make(
		[]byte, len(participants.Keys)*btcec.PubKeyBytesLenCompressed,
	)

	for idx, key := range participants.Keys {
		copy(
			value[idx*btcec.PubKeyBytesLenCompressed:],
			key.SerializeCompressed(),
		)
	}

	return serializeKVPairWithType(w, typ, participants.KeyData(), value)
}

// MuSig2PubNonce represents a public nonce provided by a participant in a
// MuSig2 signing session.
type MuSig2PubNonce struct {
	// PubKey is the public key of the participant providing this nonce.
	PubKey *btcec.PublicKey

	// AggregateKey is the plain (non-tweaked) aggregate public key the
	// participant is providing the nonce for. This must be the key found in
	// the script and not the aggregate public key that it was derived from,
	// if it was derived from an aggregate key.
	AggregateKey *btcec.PublicKey

	// TapLeafHash is the optional hash of the BIP-0341 tap leaf hash of the
	// Taproot leaf script that will be signed. If the aggregate key is the
	// taproot internal key or the taproot output key, then the tap leaf
	// hash must be omitted.
	TapLeafHash []byte

	// PubNonce is the public nonce provided by the participant, produced
	// by the `NonceGen` algorithm as described in the MuSig2 BIP.
	PubNonce [musig2.PubNonceSize]byte
}

// Validate asserts the data contained within the MuSig2PubNonce is sane for
// being serialized into a PSBT packet.
func (m *MuSig2PubNonce) Validate() error {
	if m == nil {
		return ErrInvalidKeyData
	}

	if m.PubKey == nil {
		return fmt.Errorf("public key: %w", ErrMissingKey)
	}

	if m.AggregateKey == nil {
		return fmt.Errorf("aggregate key: %w", ErrMissingKey)
	}

	if len(m.TapLeafHash) != 0 && len(m.TapLeafHash) != sha256.Size {
		return ErrInvalidTapLeafHash
	}

	return nil
}

// KeyData returns the key data for the MuSig2PubNonce struct.
func (m *MuSig2PubNonce) KeyData() []byte {
	// The tap leaf hash is optional.
	keyLen := 2*btcec.PubKeyBytesLenCompressed + len(m.TapLeafHash)
	keyData := make([]byte, keyLen)

	copy(keyData, m.PubKey.SerializeCompressed())
	copy(
		keyData[btcec.PubKeyBytesLenCompressed:],
		m.AggregateKey.SerializeCompressed(),
	)

	if len(m.TapLeafHash) != 0 {
		copy(keyData[2*btcec.PubKeyBytesLenCompressed:], m.TapLeafHash)
	}

	return keyData
}

// ReadMuSig2PubNonce reads a MuSig2 public nonce from a key-value pair in a
// PSBT.
func ReadMuSig2PubNonce(keyData, value []byte) (*MuSig2PubNonce, error) {
	const pubKeyLen = btcec.PubKeyBytesLenCompressed
	const minLength = 2 * pubKeyLen
	const maxLength = minLength + sha256.Size

	if len(keyData) != minLength && len(keyData) != maxLength {
		return nil, ErrInvalidKeyData
	}

	if len(value) != musig2.PubNonceSize {
		return nil, ErrInvalidPsbtFormat
	}

	var (
		nonce MuSig2PubNonce
		err   error
	)
	nonce.PubKey, err = btcec.ParsePubKey(keyData[0:pubKeyLen])
	if err != nil {
		return nil, err
	}

	nonce.AggregateKey, err = btcec.ParsePubKey(
		keyData[pubKeyLen : 2*pubKeyLen],
	)
	if err != nil {
		return nil, err
	}

	if len(keyData) == maxLength {
		nonce.TapLeafHash = make([]byte, sha256.Size)
		copy(nonce.TapLeafHash, keyData[2*pubKeyLen:])
	}

	copy(nonce.PubNonce[:], value)

	return &nonce, nil
}

// SerializeMuSig2PubNonce serializes a MuSig2 public nonce to a key-value pair
// in a PSBT.
func SerializeMuSig2PubNonce(w io.Writer, typ uint8,
	nonce *MuSig2PubNonce) error {

	// Make sure what we write would also be accepted again when reading.
	if err := nonce.Validate(); err != nil {
		return err
	}

	return serializeKVPairWithType(
		w, typ, nonce.KeyData(), nonce.PubNonce[:],
	)
}

// MuSig2PartialSig represents a partial signature provided by a participant in
// a MuSig2 signing session.
type MuSig2PartialSig struct {
	// PubKey is the public key of the participant providing this partial
	// signature.
	PubKey *btcec.PublicKey

	// AggregateKey is the plain (non-tweaked) aggregate public key the
	// participant is providing the partial signature for. This must be the
	// key found in the script and not the aggregate public key that it was
	// derived from, if it was derived from an aggregate key.
	AggregateKey *btcec.PublicKey

	// TapLeafHash is the optional hash of the BIP-0341 tap leaf hash of the
	// Taproot leaf script that will be signed. If the aggregate key is the
	// taproot internal key or the taproot output key, then the tap leaf
	// hash must be omitted.
	TapLeafHash []byte

	// PartialSig is the partial signature provided by the participant,
	// produced by the `Sign` algorithm as described in the MuSig2 BIP.
	PartialSig musig2.PartialSignature
}

// Validate asserts the data contained within the MuSig2PartialSig is sane for
// being serialized into a PSBT packet.
func (m *MuSig2PartialSig) Validate() error {
	if m == nil {
		return ErrInvalidKeyData
	}

	if m.PubKey == nil {
		return fmt.Errorf("public key: %w", ErrMissingKey)
	}

	if m.AggregateKey == nil {
		return fmt.Errorf("aggregate key: %w", ErrMissingKey)
	}

	if len(m.TapLeafHash) != 0 && len(m.TapLeafHash) != sha256.Size {
		return ErrInvalidTapLeafHash
	}

	// Only the S value of a partial signature is serialized (and therefore
	// only S is restored when parsing), so we must not require R here.
	if m.PartialSig.S == nil {
		return ErrMissingPartialSignature
	}

	return nil
}

// KeyData returns the key data for the MuSig2PartialSig struct.
func (m *MuSig2PartialSig) KeyData() []byte {
	// The tap leaf hash is optional.
	keyLen := 2*btcec.PubKeyBytesLenCompressed + len(m.TapLeafHash)
	keyData := make([]byte, keyLen)

	copy(keyData, m.PubKey.SerializeCompressed())
	copy(
		keyData[btcec.PubKeyBytesLenCompressed:],
		m.AggregateKey.SerializeCompressed(),
	)

	if len(m.TapLeafHash) != 0 {
		copy(keyData[2*btcec.PubKeyBytesLenCompressed:], m.TapLeafHash)
	}

	return keyData
}

// ReadMuSig2PartialSig reads a MuSig2 partial signature from a key-value pair
// in a PSBT.
func ReadMuSig2PartialSig(keyData, value []byte) (*MuSig2PartialSig, error) {
	const pubKeyLen = btcec.PubKeyBytesLenCompressed
	const minLength = 2 * pubKeyLen
	const maxLength = minLength + sha256.Size

	if len(keyData) != minLength && len(keyData) != maxLength {
		return nil, ErrInvalidKeyData
	}

	if len(value) != schnorr.ScalarSize {
		return nil, ErrInvalidPsbtFormat
	}

	var (
		partialSig MuSig2PartialSig
		err        error
	)
	partialSig.PubKey, err = btcec.ParsePubKey(keyData[0:pubKeyLen])
	if err != nil {
		return nil, err
	}

	partialSig.AggregateKey, err = btcec.ParsePubKey(
		keyData[pubKeyLen : 2*pubKeyLen],
	)
	if err != nil {
		return nil, err
	}

	if len(keyData) == maxLength {
		partialSig.TapLeafHash = make([]byte, sha256.Size)
		copy(partialSig.TapLeafHash, keyData[2*pubKeyLen:])
	}

	err = partialSig.PartialSig.Decode(bytes.NewReader(value))
	if err != nil {
		return nil, err
	}

	return &partialSig, nil
}

// SerializeMuSig2PartialSig serializes a MuSig2 partial signature to a
// key-value pair in a PSBT.
func SerializeMuSig2PartialSig(w io.Writer, typ uint8,
	partialSig *MuSig2PartialSig) error {

	// Make sure what we write would also be accepted again when reading.
	if err := partialSig.Validate(); err != nil {
		return err
	}

	var buf bytes.Buffer
	err := partialSig.PartialSig.Encode(&buf)
	if err != nil {
		return err
	}

	return serializeKVPairWithType(
		w, typ, partialSig.KeyData(), buf.Bytes(),
	)
}
