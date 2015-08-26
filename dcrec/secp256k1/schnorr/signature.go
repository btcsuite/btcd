// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package schnorr

import (
	"fmt"
	"math/big"
)

// Signature is a type representing a Schnorr signature.
type Signature struct {
	R *big.Int
	S *big.Int
}

// SignatureSize is the size of an encoded Schnorr signature.
const SignatureSize = 64

// NewSignature instantiates a new signature given some R,S values.
func NewSignature(r, s *big.Int) *Signature {
	return &Signature{r, s}
}

// Serialize returns the Schnorr signature in the more strict format.
//
// The signatures are encoded as
//   sig[0:32]  R, a point encoded as big endian
//   sig[32:64] S, scalar multiplication/addition results = (ab+c) mod l
//     encoded also as big endian
func (sig Signature) Serialize() []byte {
	rBytes := BigIntToEncodedBytes(sig.R)
	sBytes := BigIntToEncodedBytes(sig.S)

	all := append(rBytes[:], sBytes[:]...)

	return all
}

func parseSig(sigStr []byte) (*Signature, error) {
	if len(sigStr) != SignatureSize {
		return nil, fmt.Errorf("bad signature size; have %v, want %v",
			len(sigStr), SignatureSize)
	}

	rBytes := copyBytes(sigStr[0:32])
	r := EncodedBytesToBigInt(rBytes)
	sBytes := copyBytes(sigStr[32:64])
	s := EncodedBytesToBigInt(sBytes)

	return &Signature{r, s}, nil
}

// ParseSignature parses a signature in BER format for the curve type `curve'
// into a Signature type, perfoming some basic sanity checks.
func ParseSignature(sigStr []byte) (*Signature, error) {
	return parseSig(sigStr)
}

// GetR satisfies the chainec PublicKey interface.
func (sig Signature) GetR() *big.Int {
	return sig.R
}

// GetS satisfies the chainec PublicKey interface.
func (sig Signature) GetS() *big.Int {
	return sig.S
}

// GetType satisfies the chainec Signature interface.
func (sig Signature) GetType() int {
	return ecTypeSecSchnorr
}
