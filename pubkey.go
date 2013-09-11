// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcec

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
)

func isOdd(a *big.Int) bool {
	return a.Bit(0) == 1
}

const (
	pubkeyCompressed   byte = 0x2 // y_bit + x coord
	pubkeyUncompressed byte = 0x4 // x coord + y coord
	pubkeyHybrid       byte = 0x6 // y_bit + x coord + y coord
)

// ParsePubKey parses a public key for a koblitz curve from a bytestring into a
// ecdsa.Publickey, verifying that it is valid. It supports compressed,
// uncompressed and hybrid signature formats.
func ParsePubKey(pubKeyStr []byte, curve *KoblitzCurve) (key *ecdsa.PublicKey, err error) {
	pubkey := ecdsa.PublicKey{}
	pubkey.Curve = curve

	format := pubKeyStr[0]
	ybit := (format & 0x1) == 0x1
	format &= ^byte(0x1)

	switch len(pubKeyStr) {
	case 65: // normal public key
		if format != pubkeyUncompressed && format != pubkeyHybrid {
			return nil, fmt.Errorf("invalid magic in pubkey str: "+
				"%d", pubKeyStr[0])
		}

		pubkey.X = new(big.Int).SetBytes(pubKeyStr[1:33])
		pubkey.Y = new(big.Int).SetBytes(pubKeyStr[33:])
		// hybrid keys have extra information, make use of it.
		if format == pubkeyHybrid && ybit != isOdd(pubkey.Y) {
			return nil, fmt.Errorf("ybit doesn't match oddness")
		}
	case 33: // compressed public key
		// format is 0x2 | solution, <X coordinate>
		// solution determines which solution of the curve we use.
		/// y^2 = x^3 + Curve.B
		if format != pubkeyCompressed {
			return nil, fmt.Errorf("invalid magic in compressed "+
				"pubkey string: %d", pubKeyStr[0])
		}
		pubkey.X = new(big.Int).SetBytes(pubKeyStr[1:33])
		// Y = +-sqrt(x^3 + B)
		x3 := new(big.Int).Mul(pubkey.X, pubkey.X)
		x3.Mul(x3, pubkey.X)
		x3.Add(x3, pubkey.Curve.Params().B)

		// now calculate sqrt mod p of x2 + B
		// This code used to do a full sqrt based on tonelli/shanks,
		// but this was replaced by the algorithms referenced in
		// https://bitcointalk.org/index.php?topic=162805.msg1712294#msg1712294
		y := new(big.Int).Exp(x3, curve.QPlus1Div4(), pubkey.Curve.Params().P)

		if ybit != isOdd(y) {
			y.Sub(pubkey.Curve.Params().P, y)
		}
		if ybit != isOdd(y) {
			return nil, fmt.Errorf("ybit doesn't match oddness")
		}

		pubkey.Y = y
	default: // wrong!
		return nil, fmt.Errorf("invalid pub key length %d",
			len(pubKeyStr))
	}

	if pubkey.X.Cmp(pubkey.Curve.Params().P) >= 0 {
		return nil, fmt.Errorf("pubkey X parameter is >= to P")
	}
	if pubkey.Y.Cmp(pubkey.Curve.Params().P) >= 0 {
		return nil, fmt.Errorf("pubkey Y parameter is >= to P")
	}
	if !pubkey.Curve.IsOnCurve(pubkey.X, pubkey.Y) {
		return nil, fmt.Errorf("pubkey isn't on secp265k1 curve")
	}
	return &pubkey, nil
}

// PublicKey is an ecdsa.PublicKey with additional functions to
// serialize in uncompressed, compressed, and hybrid formats.
type PublicKey ecdsa.PublicKey

// SerializeUncompressed serializes a public key in a 65-byte uncompressed
// format.
func (p *PublicKey) SerializeUncompressed() []byte {
	b := make([]byte, 65)
	b[0] = pubkeyUncompressed
	copy(b[1:33], p.X.Bytes())
	copy(b[33:], p.Y.Bytes())
	return b
}

// SerializeCompressed serializes a public key in a 33-byte compressed format.
func (p *PublicKey) SerializeCompressed() []byte {
	b := make([]byte, 33)
	format := pubkeyCompressed
	if isOdd(p.Y) {
		format |= 0x1
	}
	b[0] = format
	copy(b[1:33], p.X.Bytes())
	return b
}

// SerializeHybrid serializes a public key in a 65-byte hybrid format.
func (p *PublicKey) SerializeHybrid() []byte {
	b := make([]byte, 65)
	format := pubkeyHybrid
	if isOdd(p.Y) {
		format |= 0x1
	}
	b[0] = format
	copy(b[1:33], p.X.Bytes())
	copy(b[33:], p.Y.Bytes())
	return b
}
