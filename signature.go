// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcec

import (
	"crypto/elliptic"
	"errors"
	"fmt"
	"math/big"
)

type Signature struct {
	R *big.Int
	S *big.Int
}

func ParseSignature(sigStr []byte, curve elliptic.Curve) (*Signature, error) {
	// Originally this code used encoding/asn1 in order to parse the
	// signature, but a number of problems were found with this approach.
	// Despite the fact that signatures are stored as DER, the difference
	// between go's idea of a bignum (and that they have sign) doesn't agree
	// with the openssl one (where they do not). The above is true as of
	// Go 1.1. In the end it was simpler to rewrite the code to explicitly
	// understand the format which is this:
	// 0x30 <length of whole message> <0x02> <length of R> <R> 0x2
	// <length of S> <S>.

	signature := &Signature{}

	// minimal message is when both numbers are 1 bytes. adding up to:
	// 0x30 + len + 0x02 + 0x01 + <byte> + 0x2 + 0x01 + <byte>
	if len(sigStr) < 8 {
		return nil, errors.New("malformed signature: too short")
	}
	// 0x30
	index := 0
	if sigStr[index] != 0x30 {
		return nil, errors.New("malformed signature: no header magic")
	}
	index++
	// length of remaining message
	siglen := sigStr[index]
	index++
	if int(siglen+2) > len(sigStr) {
		return nil, errors.New("malformed signature: bad length")
	}
	// trim the slice we're working on so we only look at what matters.
	sigStr = sigStr[:siglen+2]

	// 0x02
	if sigStr[index] != 0x02 {
		return nil,
			errors.New("malformed signature: no 1st int marker")
	}
	index++

	// Length of signature R.
	rLen := int(sigStr[index])
	// must be positive, must be able to fit in another 0x2, <len> <s>
	// hence the -3. We assume that the length must be at least one byte.
	index++
	if rLen <= 0 || rLen > len(sigStr)-index-3 {
		return nil, errors.New("malformed signature: bogus R length")
	}

	// Then R itself.
	signature.R = new(big.Int).SetBytes(sigStr[index : index+rLen])
	index += rLen
	// 0x02. length already checked in previous if.
	if sigStr[index] != 0x02 {
		return nil, errors.New("malformed signature: no 2nd int marker")
	}
	index++

	// Length of signature S.
	sLen := int(sigStr[index])
	index++
	// S should be the rest of the string.
	if sLen <= 0 || sLen > len(sigStr)-index {
		return nil, errors.New("malformed signature: bogus S length")
	}

	// Then S itself.
	signature.S = new(big.Int).SetBytes(sigStr[index : index+sLen])
	index += sLen

	// sanity check length parsing
	if index != len(sigStr) {
		return nil, fmt.Errorf("malformed signature: bad final length %v != %v",
			index, len(sigStr))
	}

	// Verify also checks this, but we can be more sure that we parsed
	// correctly if we verify here too.
	// FWIW the ecdsa spec states that R and S must be | 1, N - 1 |
	// but crypto/ecdsa only checks for Sign != 0. Mirror that.
	if signature.R.Sign() != 1 {
		return nil, errors.New("Signature R isn't 1 or more")
	}
	if signature.S.Sign() != 1 {
		return nil, errors.New("Signature S isn't 1 or more")
	}
	if signature.R.Cmp(curve.Params().N) >= 0 {
		return nil, errors.New("Signature R is >= curve.N")
	}
	if signature.S.Cmp(curve.Params().N) >= 0 {
		return nil, errors.New("Signature S is >= curve.N")
	}

	return signature, nil
}
