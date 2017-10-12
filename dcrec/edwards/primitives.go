// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package edwards

import (
	"fmt"
	"math/big"

	"github.com/agl/ed25519/edwards25519"
)

// Some notes on primitives in Ed25519:
//   1) The integers themselves are stored as 32-byte little endian
//        representations. If the store value is a point, the bit in
//        the 31st byte, seventh position (b[31]>>7) represents whether
//        or not the X value retrieved from the Y value should be
//        negative or not. Remember, in affine EC space, the negative
//        is P - positiveX. The rest of the 255 bits then represent
//        the Y-value in little endian.
//   2) For high effiency, 40 byte field elements (10x int32s) are
//        often used to represent integers.
//   3) For further increases in efficiency, the affine (cartesian)
//        coordinates are converted into projective (extended or non-
//        extended) formats, which include a Z and T or Z value
//        respectively.
//   4) Almost *everything* is encoded in little endian, with the
//        exception of ECDSA X and Y values of points in affine space.

// reverse reverses a byte string.
func reverse(s *[32]byte) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// copyBytes copies a byte slice to a 32 byte array.
func copyBytes(aB []byte) *[32]byte {
	if aB == nil {
		return nil
	}
	s := new([32]byte)

	// If we have a short byte string, expand
	// it so that it's long enough.
	aBLen := len(aB)
	if aBLen < fieldIntSize {
		diff := fieldIntSize - aBLen
		for i := 0; i < diff; i++ {
			aB = append([]byte{0x00}, aB...)
		}
	}

	for i := 0; i < fieldIntSize; i++ {
		s[i] = aB[i]
	}

	return s
}

// copyBytes64 copies a byte slice to a 64 byte array.
func copyBytes64(aB []byte) *[64]byte {
	if aB == nil {
		return nil
	}

	s := new([64]byte)

	// If we have a short byte string, expand
	// it so that it's long enough.
	aBLen := len(aB)
	if aBLen < 64 {
		diff := 64 - aBLen
		for i := 0; i < diff; i++ {
			aB = append([]byte{0x00}, aB...)
		}
	}

	for i := 0; i < 64; i++ {
		s[i] = aB[i]
	}

	return s
}

// zeroSlice zeroes the memory of a scalar byte slice.
func zeroSlice(s []byte) {
	for i := 0; i < PrivScalarSize; i++ {
		s[i] = 0x00
	}
}

// BigIntToEncodedBytes converts a big integer into its corresponding
// 32 byte little endian representation.
func BigIntToEncodedBytes(a *big.Int) *[32]byte {
	s := new([32]byte)
	if a == nil {
		return s
	}
	// Caveat: a can be longer than 32 bytes.
	aB := a.Bytes()

	// If we have a short byte string, expand
	// it so that it's long enough.
	aBLen := len(aB)
	if aBLen < fieldIntSize {
		diff := fieldIntSize - aBLen
		for i := 0; i < diff; i++ {
			aB = append([]byte{0x00}, aB...)
		}
	}

	for i := 0; i < fieldIntSize; i++ {
		s[i] = aB[i]
	}

	// Reverse the byte string --> little endian after
	// encoding.
	reverse(s)

	return s
}

// BigIntToEncodedBytesNoReverse converts a big integer into its corresponding
// 32 byte big endian representation.
func BigIntToEncodedBytesNoReverse(a *big.Int) *[32]byte {
	s := new([32]byte)
	if a == nil {
		return s
	}
	// Caveat: a can be longer than 32 bytes.
	aB := a.Bytes()

	// If we have a short byte string, expand
	// it so that it's long enough.
	aBLen := len(aB)
	if aBLen < fieldIntSize {
		diff := fieldIntSize - aBLen
		for i := 0; i < diff; i++ {
			aB = append([]byte{0x00}, aB...)
		}
	}

	for i := 0; i < fieldIntSize; i++ {
		s[i] = aB[i]
	}

	return s
}

// BigIntToFieldElement converts a big little endian integer into its corresponding
// 40 byte field representation.
func BigIntToFieldElement(a *big.Int) *edwards25519.FieldElement {
	aB := BigIntToEncodedBytes(a)
	fe := new(edwards25519.FieldElement)
	edwards25519.FeFromBytes(fe, aB)
	return fe
}

// BigIntPointToEncodedBytes converts an affine point to a compressed
// 32 byte integer representation.
func BigIntPointToEncodedBytes(x *big.Int, y *big.Int) *[32]byte {
	s := BigIntToEncodedBytes(y)
	xB := BigIntToEncodedBytes(x)
	xFE := new(edwards25519.FieldElement)
	edwards25519.FeFromBytes(xFE, xB)
	isNegative := edwards25519.FeIsNegative(xFE) == 1

	if isNegative {
		s[31] |= (1 << 7)
	} else {
		s[31] &^= (1 << 7)
	}

	return s
}

// EncodedBytesToBigInt converts a 32 byte little endian representation of
// an integer into a big, big endian integer.
func EncodedBytesToBigInt(s *[32]byte) *big.Int {
	// Use a copy so we don't screw up our original
	// memory.
	sCopy := new([32]byte)
	for i := 0; i < fieldIntSize; i++ {
		sCopy[i] = s[i]
	}
	reverse(sCopy)

	bi := new(big.Int).SetBytes(sCopy[:])

	return bi
}

// EncodedBytesToBigIntNoReverse converts a 32 byte big endian representation of
// an integer into a big little endian integer.
func EncodedBytesToBigIntNoReverse(s *[32]byte) *big.Int {
	// Use a copy so we don't screw up our original
	// memory.
	sCopy := new([32]byte)
	for i := 0; i < fieldIntSize; i++ {
		sCopy[i] = s[i]
	}

	bi := new(big.Int).SetBytes(sCopy[:])

	return bi
}

// extendedToBigAffine converts projective x, y, and z field elements into
// affine x and y coordinates, and returns whether or not the x value
// returned is negative.
func (curve *TwistedEdwardsCurve) extendedToBigAffine(xi, yi,
	zi *edwards25519.FieldElement) (*big.Int, *big.Int, bool) {
	var recip, x, y edwards25519.FieldElement

	// Normalize to Z=1.
	edwards25519.FeInvert(&recip, zi)
	edwards25519.FeMul(&x, xi, &recip)
	edwards25519.FeMul(&y, yi, &recip)

	isNegative := edwards25519.FeIsNegative(&x) == 1

	return FieldElementToBigInt(&x), FieldElementToBigInt(&y), isNegative
}

// EncodedBytesToBigIntPoint converts a 32 byte representation of a point
// on the elliptical curve into a big integer point. It returns an error
// if the point does not fall on the curve.
func (curve *TwistedEdwardsCurve) EncodedBytesToBigIntPoint(s *[32]byte) (*big.Int,
	*big.Int, error) {
	sCopy := new([32]byte)
	for i := 0; i < fieldIntSize; i++ {
		sCopy[i] = s[i]
	}

	xIsNegBytes := sCopy[31]>>7 == 1
	p := new(edwards25519.ExtendedGroupElement)
	if !p.FromBytes(sCopy) {
		return nil, nil, fmt.Errorf("point not on curve")
	}

	// Normalize the X and Y coordinates in affine space.
	x, y, isNegative := curve.extendedToBigAffine(&p.X, &p.Y, &p.Z)

	// We got the wrong sign; flip the bit and recalculate.
	if xIsNegBytes != isNegative {
		x.Sub(curve.P, x)
	}

	// This should hopefully never happen, since the
	// library itself should never let us create a bad
	// point.
	if !curve.IsOnCurve(x, y) {
		return nil, nil, fmt.Errorf("point not on curve")
	}

	return x, y, nil
}

// EncodedBytesToFieldElement converts a 32 byte little endian integer into
// a field element.
func EncodedBytesToFieldElement(s *[32]byte) *edwards25519.FieldElement {
	fe := new(edwards25519.FieldElement)
	edwards25519.FeFromBytes(fe, s)
	return fe
}

// FieldElementToBigInt converts a 40 byte field element into a big int.
func FieldElementToBigInt(fe *edwards25519.FieldElement) *big.Int {
	s := new([32]byte)
	edwards25519.FeToBytes(s, fe)
	reverse(s)

	aBI := new(big.Int).SetBytes(s[:])

	return aBI
}

// FieldElementToEncodedBytes converts a 40 byte field element into a 32 byte
// little endian integer.
func FieldElementToEncodedBytes(fe *edwards25519.FieldElement) *[32]byte {
	s := new([32]byte)
	edwards25519.FeToBytes(s, fe)
	return s
}

// invert inverts a big integer over the Ed25519 curve.
func (curve *TwistedEdwardsCurve) invert(a *big.Int) *big.Int {
	sub2 := new(big.Int).Sub(curve.P, two)
	inv := new(big.Int).Exp(a, sub2, curve.P)
	return inv
}
