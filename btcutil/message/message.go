package message

import (
	"bytes"
	"encoding/base64"
	"errors"

	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

const (
	header         = "Bitcoin Signed Message:\n"
	compactSigSize = 65
)

var (
	// ErrUnsupportedAddress is returned when the address is not P2PKH.
	ErrUnsupportedAddress = errors.New("message: only P2PKH addresses are supported")

	// ErrNotCompactSignature is returned when the decoded signature is not
	// 65 bytes (the size of a compact ECDSA signature).
	ErrNotCompactSignature = errors.New("message: not a compact signature")
)

// Hash computes the Bitcoin message hash: DoubleHashB of the
// length-prefixed header concatenated with the length-prefixed message.
func Hash(message string) []byte {
	var buf bytes.Buffer
	wire.WriteVarString(&buf, 0, header)
	wire.WriteVarString(&buf, 0, message)
	return chainhash.DoubleHashB(buf.Bytes())
}

// Verify verifies a BIP-137 compact ECDSA signature against a P2PKH address.
func Verify(addr btcutil.Address, message string, signature string) (bool, error) {
	p2pkhAddr, ok := addr.(*btcutil.AddressPubKeyHash)
	if !ok {
		return false, ErrUnsupportedAddress
	}

	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false, err
	}
	if len(sigBytes) != compactSigSize {
		return false, ErrNotCompactSignature
	}

	// Mirror Bitcoin Core behavior, which treats error in
	// RecoverCompact as invalid signature.
	pk, wasCompressed, err := ecdsa.RecoverCompact(sigBytes, Hash(message))
	if err != nil {
		return false, nil
	}

	var serializedPK []byte
	if wasCompressed {
		serializedPK = pk.SerializeCompressed()
	} else {
		serializedPK = pk.SerializeUncompressed()
	}

	recoveredHash := btcutil.Hash160(serializedPK)
	return bytes.Equal(recoveredHash, p2pkhAddr.Hash160()[:]), nil
}
