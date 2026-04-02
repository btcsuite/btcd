package bip322

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"

	"github.com/btcsuite/btcd/wire"
)

// FormatType identifies the format of a BIP-322 signature.
type FormatType int

const (
	// FormatSimple is a BIP-322 Simple format — base64-encoded witness stack.
	FormatSimple FormatType = iota

	// FormatFull is a BIP-322 Full format — base64-encoded complete to_sign transaction.
	FormatFull
)

var (
	// ErrEmptyWitness is returned when encoding an empty witness stack.
	ErrEmptyWitness = errors.New("bip322: witness stack is empty")
)

// EncodeSimple serializes a witness stack in Bitcoin consensus encoding and
// returns it as a base64-encoded string (BIP-322 Simple format).
//
// The encoding format is:
//   - varint: number of witness items
//   - for each item: varint(len) + item bytes
func EncodeSimple(witness wire.TxWitness) (string, error) {
	if len(witness) == 0 {
		return "", ErrEmptyWitness
	}
	var buf bytes.Buffer
	if err := wire.WriteVarInt(&buf, 0, uint64(len(witness))); err != nil {
		return "", err
	}
	for _, item := range witness {
		if err := wire.WriteVarInt(&buf, 0, uint64(len(item))); err != nil {
			return "", err
		}
		if _, err := buf.Write(item); err != nil {
			return "", err
		}
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// DecodeSimple decodes a BIP-322 Simple format signature from a base64-encoded
// string into a witness stack.
func DecodeSimple(encoded string) (wire.TxWitness, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	r := bytes.NewReader(data)
	count, err := wire.ReadVarInt(r, 0)
	if err != nil {
		return nil, err
	}
	witness := make(wire.TxWitness, count)
	for i := range witness {
		itemLen, err := wire.ReadVarInt(r, 0)
		if err != nil {
			return nil, err
		}
		item := make([]byte, itemLen)
		if _, err := io.ReadFull(r, item); err != nil {
			return nil, err
		}
		witness[i] = item
	}
	return witness, nil
}

// EncodeFull serializes a complete transaction in Bitcoin network format and
// returns it as a base64-encoded string (BIP-322 Full format).
func EncodeFull(tx *wire.MsgTx) (string, error) {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// DecodeFull decodes a BIP-322 Full format signature from a base64-encoded
// string into a complete transaction.
func DecodeFull(encoded string) (*wire.MsgTx, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	tx := wire.NewMsgTx(wire.TxVersion)
	if err := tx.Deserialize(bytes.NewReader(data)); err != nil {
		return nil, err
	}
	return tx, nil
}

// DetectFormat determines the BIP-322 signature format of a base64-encoded
// signature string.
//
// Detection order:
//  1. Valid consensus-encoded witness stack -> FormatSimple
//  2. Valid serialized transaction -> FormatFull
func DetectFormat(signature string) (FormatType, error) {
	data, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return 0, err
	}

	if isValidWitnessEncoding(data) {
		return FormatSimple, nil
	}

	tx := wire.NewMsgTx(wire.TxVersion)
	if err := tx.Deserialize(bytes.NewReader(data)); err == nil {
		return FormatFull, nil
	}

	return 0, errors.New("bip322: unrecognized signature format")
}

// isValidWitnessEncoding checks whether data is a valid consensus-encoded
// witness stack (varint count followed by length-prefixed items) with no
// trailing bytes.
func isValidWitnessEncoding(data []byte) bool {
	r := bytes.NewReader(data)
	count, err := wire.ReadVarInt(r, 0)
	if err != nil {
		return false
	}
	for i := uint64(0); i < count; i++ {
		itemLen, err := wire.ReadVarInt(r, 0)
		if err != nil {
			return false
		}
		if _, err := io.ReadFull(r, make([]byte, itemLen)); err != nil {
			return false
		}
	}
	// All bytes must be consumed — no trailing garbage.
	return r.Len() == 0
}
