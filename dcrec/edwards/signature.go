// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package edwards

import (
	"fmt"
	"math/big"
)

// Signature is a type representing an ecdsa signature.
type Signature struct {
	R *big.Int
	S *big.Int
}

// SignatureSize is the size of an encoded ECDSA signature.
const SignatureSize = 64

// NewSignature instantiates a new signature given some R,S values.
func NewSignature(r, s *big.Int) *Signature {
	return &Signature{r, s}
}

// Serialize returns the ECDSA signature in the more strict format.
//
// The signatures are encoded as
//   sig[0:32]  R, a point encoded as little endian
//   sig[32:64] S, scalar multiplication/addition results = (ab+c) mod l
//     encoded also as little endian
func (sig Signature) Serialize() []byte {
	rBytes := BigIntToEncodedBytes(sig.R)
	sBytes := BigIntToEncodedBytes(sig.S)

	all := append(rBytes[:], sBytes[:]...)

	return all
}

// parseSig is the default method of parsing a serialized Ed25519 signature.
func parseSig(curve *TwistedEdwardsCurve, sigStr []byte, der bool) (*Signature,
	error) {
	if der {
		return nil, fmt.Errorf("DER signatures not allowed in ed25519")
	}

	if len(sigStr) != SignatureSize {
		return nil, fmt.Errorf("bad signature size; have %v, want %v",
			len(sigStr), SignatureSize)
	}

	rBytes := copyBytes(sigStr[0:32])
	r := EncodedBytesToBigInt(rBytes)
	// r is a point on the curve as well. Evaluate it and make sure it's
	// a valid point.
	_, _, err := curve.EncodedBytesToBigIntPoint(rBytes)
	if err != nil {
		return nil, err
	}

	sBytes := copyBytes(sigStr[32:64])
	s := EncodedBytesToBigInt(sBytes)
	// s may not be zero or >= curve.N.
	if s.Cmp(curve.N) >= 0 || s.Cmp(zero) == 0 {
		return nil, fmt.Errorf("s scalar is empty or larger than the order of " +
			"the curve")
	}

	return &Signature{r, s}, nil
}

// ParseSignature parses a signature in BER format for the curve type `curve'
// into a Signature type, perfoming some basic sanity checks.
func ParseSignature(curve *TwistedEdwardsCurve, sigStr []byte) (*Signature,
	error) {
	return parseSig(curve, sigStr, false)
}

// ParseDERSignature offers a legacy function for plugging into Decred, which
// is based off btcec.
func ParseDERSignature(curve *TwistedEdwardsCurve, sigStr []byte) (*Signature,
	error) {
	return parseSig(curve, sigStr, false)
}

// RecoverCompact uses a signature and a hash to recover is private
// key, is not yet implemented.
// TODO: Implement.
func RecoverCompact(signature, hash []byte) (*PublicKey, bool, error) {
	// TODO One day reimplement this? cj
	return nil, false, nil
}

// GetR satisfies the chainec Signature interface.
func (sig Signature) GetR() *big.Int {
	return sig.R
}

// GetS satisfies the chainec Signature interface.
func (sig Signature) GetS() *big.Int {
	return sig.S
}

// GetType satisfies the chainec Signature interface.
func (sig Signature) GetType() int {
	return ecTypeEdwards
}
