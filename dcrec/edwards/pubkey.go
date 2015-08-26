// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package edwards

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
)

// These constants define the lengths of serialized public keys.
const (
	PubKeyBytesLen = 32
)

// PublicKey is an ecdsa.PublicKey with an additional function to
// serialize.
type PublicKey ecdsa.PublicKey

// NewPublicKey instantiates a new public key.
func NewPublicKey(curve *TwistedEdwardsCurve, x *big.Int, y *big.Int) *PublicKey {
	return &PublicKey{curve, x, y}
}

// ParsePubKey parses a public key for an edwards curve from a bytestring into a
// ecdsa.Publickey, verifying that it is valid.
func ParsePubKey(curve *TwistedEdwardsCurve, pubKeyStr []byte) (key *PublicKey,
	err error) {
	pubkey := PublicKey{}
	pubkey.Curve = curve
	x, y, err := curve.EncodedBytesToBigIntPoint(copyBytes(pubKeyStr))
	if err != nil {
		return nil, err
	}
	pubkey.X = x
	pubkey.Y = y

	if len(pubKeyStr) == 0 {
		return nil, errors.New("pubkey string is empty")
	}
	if pubkey.X.Cmp(pubkey.Curve.Params().P) >= 0 {
		return nil, fmt.Errorf("pubkey X parameter is >= to P")
	}
	if pubkey.Y.Cmp(pubkey.Curve.Params().P) >= 0 {
		return nil, fmt.Errorf("pubkey Y parameter is >= to P")
	}

	return &pubkey, nil
}

// ToECDSA returns the public key as a *ecdsa.PublicKey.
func (p PublicKey) ToECDSA() *ecdsa.PublicKey {
	pkecdsa := ecdsa.PublicKey(p)
	return &pkecdsa
}

// Serialize serializes a public key in a 32-byte compressed little endian format.
func (p PublicKey) Serialize() []byte {
	if p.X == nil || p.Y == nil {
		return nil
	}
	return BigIntPointToEncodedBytes(p.X, p.Y)[:]
}

// SerializeUncompressed satisfies the chainec PublicKey interface.
func (p PublicKey) SerializeUncompressed() []byte {
	return p.Serialize()
}

// SerializeCompressed satisfies the chainec PublicKey interface.
func (p PublicKey) SerializeCompressed() []byte {
	return p.Serialize()
}

// SerializeHybrid satisfies the chainec PublicKey interface.
func (p PublicKey) SerializeHybrid() []byte {
	return p.Serialize()
}

// GetCurve satisfies the chainec PublicKey interface.
func (p PublicKey) GetCurve() interface{} {
	return p.Curve
}

// GetX satisfies the chainec PublicKey interface.
func (p PublicKey) GetX() *big.Int {
	return p.X
}

// GetY satisfies the chainec PublicKey interface.
func (p PublicKey) GetY() *big.Int {
	return p.Y
}

// GetType satisfies the chainec PublicKey interface.
func (p PublicKey) GetType() int {
	return ecTypeEdwards
}
