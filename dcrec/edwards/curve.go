// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package edwards

import (
	"crypto/elliptic"
	"math/big"

	"github.com/agl/ed25519/edwards25519"
)

// TwistedEdwardsCurve extended an elliptical curve set of
// parameters to satisfy the interface of the elliptic package.
type TwistedEdwardsCurve struct {
	*elliptic.CurveParams
	H int // Cofactor of the curve

	A, D, I *big.Int // Edwards curve equation parameter constants

	// byteSize is simply the bit size / 8 and is provided for convenience
	// since it is calculated repeatedly.
	byteSize int
}

// Params returns the parameters for the curve.
func (curve TwistedEdwardsCurve) Params() *elliptic.CurveParams {
	return curve.CurveParams
}

// Marshal converts a point into the 32 byte encoded Ed25519 form.
func Marshal(curve TwistedEdwardsCurve, x, y *big.Int) []byte {
	return BigIntPointToEncodedBytes(x, y)[:]
}

// Unmarshal converts a point into the 32 byte encoded Ed25519 form.
func Unmarshal(curve *TwistedEdwardsCurve, data []byte) (x, y *big.Int) {
	var err error
	x, y, err = curve.EncodedBytesToBigIntPoint(copyBytes(data))
	if err != nil {
		x = nil
		y = nil
	}
	return
}

// RecoverXBigInt recovers the X value for some Y value, for a coordinate
// on the Ed25519 curve given as a big integer Y value.
func (curve *TwistedEdwardsCurve) RecoverXBigInt(xIsNeg bool,
	y *big.Int) *big.Int {
	// (y^2 - 1)
	l := new(big.Int).Mul(y, y)
	l.Sub(l, one)

	// inv(d*y^2+1)
	temp := new(big.Int).Mul(y, y)
	temp.Mul(temp, curve.D)
	temp.Add(temp, one)
	r := curve.invert(temp)

	// x2 = (y^2 - 1) * invert(d*y^2+1)
	x2 := new(big.Int).Mul(r, l)

	// x = exp(x^2,(P+3)/8, P)
	qp3 := new(big.Int).Add(curve.P, three)
	qp3.Div(qp3, eight) // /= curve.H
	x := new(big.Int).Exp(x2, qp3, curve.P)

	// check (x^2 - x2) % q != 0
	x22 := new(big.Int).Mul(x, x)
	xsub := new(big.Int).Sub(x22, x2)
	xsub.Mod(xsub, curve.P)
	if xsub.Cmp(zero) != 0 {
		ximod := new(big.Int)
		ximod.Mul(x, curve.I)
		ximod.Mod(ximod, curve.P)
		x.Set(ximod)
	}

	xmod2 := new(big.Int).Mod(x, two)
	if xmod2.Cmp(zero) != 0 {
		x.Sub(curve.P, x)
	}

	// We got the wrong x, negate it to get the right one.
	if xIsNeg != (x.Bit(0) == 1) {
		x.Sub(curve.P, x)
	}

	return x
}

// RecoverXFieldElement recovers the X value for some Y value, for a coordinate
// on the Ed25519 curve given as a field element. Y value. Probably the fastest
// way to get your respective X from Y.
func (curve *TwistedEdwardsCurve) RecoverXFieldElement(xIsNeg bool,
	y *edwards25519.FieldElement) *edwards25519.FieldElement {
	// (y^2 - 1)
	l := new(edwards25519.FieldElement)
	edwards25519.FeSquare(l, y)
	edwards25519.FeSub(l, l, &feOne)

	// inv(d*y^2+1)
	r := new(edwards25519.FieldElement)
	edwards25519.FeSquare(r, y)
	edwards25519.FeMul(r, r, &fed)
	edwards25519.FeAdd(r, r, &feOne)
	edwards25519.FeInvert(r, r)

	x2 := new(edwards25519.FieldElement)
	edwards25519.FeMul(x2, r, l)

	// Get a big int so we can do the exponentiation.
	x2Big := FieldElementToBigInt(x2)

	// x = exp(x^2,(P+3)/8, P)
	qp3 := new(big.Int).Add(curve.P, three)
	qp3.Div(qp3, eight) // /= curve.H
	xBig := new(big.Int).Exp(x2Big, qp3, curve.P)

	// Convert back to a field element and do
	// the rest.
	x := BigIntToFieldElement(xBig)

	// check (x^2 - x2) % q != 0
	x22 := new(edwards25519.FieldElement)
	edwards25519.FeSquare(x22, x)
	xsub := new(edwards25519.FieldElement)
	edwards25519.FeSub(xsub, x22, x2)
	xsubBig := FieldElementToBigInt(xsub)
	xsubBig.Mod(xsubBig, curve.P)

	if xsubBig.Cmp(zero) != 0 {
		xi := new(edwards25519.FieldElement)
		edwards25519.FeMul(xi, x, &feI)
		xiModBig := FieldElementToBigInt(xi)
		xiModBig.Mod(xiModBig, curve.P)
		xiMod := BigIntToFieldElement(xiModBig)

		x = xiMod
	}

	xBig = FieldElementToBigInt(x)
	xmod2 := new(big.Int).Mod(xBig, two)
	if xmod2.Cmp(zero) != 0 {
		// TODO replace this with FeSub
		xBig.Sub(curve.P, xBig)
		x = BigIntToFieldElement(xBig)
	}

	// We got the wrong x, negate it to get the right one.
	isNegative := edwards25519.FeIsNegative(x) == 1
	if xIsNeg != isNegative {
		edwards25519.FeNeg(x, x)
	}

	return x
}

// IsOnCurve returns bool to say if the point (x,y) is on the curve by
// checking (y^2 - x^2 - 1 - dx^2y^2) % P == 0.
func (curve *TwistedEdwardsCurve) IsOnCurve(x *big.Int, y *big.Int) bool {
	// Convert to field elements.
	xB := BigIntToEncodedBytes(x)
	yB := BigIntToEncodedBytes(y)

	yfe := new(edwards25519.FieldElement)
	xfe := new(edwards25519.FieldElement)
	edwards25519.FeFromBytes(yfe, yB)
	edwards25519.FeFromBytes(xfe, xB)

	x2 := new(edwards25519.FieldElement)
	edwards25519.FeSquare(x2, xfe)
	y2 := new(edwards25519.FieldElement)
	edwards25519.FeSquare(y2, yfe)

	dx2y2 := new(edwards25519.FieldElement)
	edwards25519.FeMul(dx2y2, &fed, x2)
	edwards25519.FeMul(dx2y2, dx2y2, y2)

	enum := new(edwards25519.FieldElement)
	edwards25519.FeSub(enum, y2, x2)
	edwards25519.FeSub(enum, enum, &feOne)
	edwards25519.FeSub(enum, enum, dx2y2)

	enumBig := FieldElementToBigInt(enum)
	enumBig.Mod(enumBig, curve.P)

	if enumBig.Cmp(zero) != 0 {
		return false
	}

	// Check if we're in the cofactor of the curve (8).
	modEight := new(big.Int)
	modEight.Mod(enumBig, eight)

	return modEight.Cmp(zero) == 0
}

// cachedGroupElement is a cached extended group element derived from
// another extended group element, for use in computation.
type cachedGroupElement struct {
	yPlusX, yMinusX, Z, T2d edwards25519.FieldElement
}

// toCached converts an extended group element to a useful intermediary
// containing precalculated values.
func toCached(r *cachedGroupElement, p *edwards25519.ExtendedGroupElement) {
	edwards25519.FeAdd(&r.yPlusX, &p.Y, &p.X)
	edwards25519.FeSub(&r.yMinusX, &p.Y, &p.X)
	edwards25519.FeCopy(&r.Z, &p.Z)
	edwards25519.FeMul(&r.T2d, &p.T, &fed2)
}

// Add adds two points represented by pairs of big integers on the elliptical
// curve.
func (curve *TwistedEdwardsCurve) Add(x1, y1, x2, y2 *big.Int) (x, y *big.Int) {
	// Convert to extended from affine.
	a := BigIntPointToEncodedBytes(x1, y1)
	aEGE := new(edwards25519.ExtendedGroupElement)
	aEGE.FromBytes(a)

	b := BigIntPointToEncodedBytes(x2, y2)
	bEGE := new(edwards25519.ExtendedGroupElement)
	bEGE.FromBytes(b)

	// Cache b for use in group element addition.
	bCached := new(cachedGroupElement)
	toCached(bCached, bEGE)

	p := aEGE
	q := bCached

	// geAdd(r*CompletedGroupElement, p*ExtendedGroupElement,
	//   q*CachedGroupElement)
	// r is the result.
	r := new(edwards25519.CompletedGroupElement)
	var t0 edwards25519.FieldElement

	edwards25519.FeAdd(&r.X, &p.Y, &p.X)
	edwards25519.FeSub(&r.Y, &p.Y, &p.X)
	edwards25519.FeMul(&r.Z, &r.X, &q.yPlusX)
	edwards25519.FeMul(&r.Y, &r.Y, &q.yMinusX)
	edwards25519.FeMul(&r.T, &q.T2d, &p.T)
	edwards25519.FeMul(&r.X, &p.Z, &q.Z)
	edwards25519.FeAdd(&t0, &r.X, &r.X)
	edwards25519.FeSub(&r.X, &r.Z, &r.Y)
	edwards25519.FeAdd(&r.Y, &r.Z, &r.Y)
	edwards25519.FeAdd(&r.Z, &t0, &r.T)
	edwards25519.FeSub(&r.T, &t0, &r.T)

	rEGE := new(edwards25519.ExtendedGroupElement)
	r.ToExtended(rEGE)

	s := new([32]byte)
	rEGE.ToBytes(s)

	x, y, _ = curve.EncodedBytesToBigIntPoint(s)

	return
}

// Double adds the same pair of big integer coordinates to itself on the
// elliptical curve.
func (curve *TwistedEdwardsCurve) Double(x1, y1 *big.Int) (x, y *big.Int) {
	// Convert to extended projective coordinates.
	a := BigIntPointToEncodedBytes(x1, y1)
	aEGE := new(edwards25519.ExtendedGroupElement)
	aEGE.FromBytes(a)

	r := new(edwards25519.CompletedGroupElement)
	aEGE.Double(r)
	rEGE := new(edwards25519.ExtendedGroupElement)
	r.ToExtended(rEGE)

	s := new([32]byte)
	rEGE.ToBytes(s)
	x, y, _ = curve.EncodedBytesToBigIntPoint(s)

	return
}

// ScalarMult returns k*(Bx,By) where k is a number in big-endian form. This
// uses the repeated doubling method, which is variable time.
// TODO use a constant time method to prevent side channel attacks.
func (curve *TwistedEdwardsCurve) ScalarMult(x1, y1 *big.Int,
	k []byte) (x, y *big.Int) {
	// Convert the scalar to a big int.
	s := new(big.Int).SetBytes(k)

	// Get a new group element to do cached doubling
	// calculations in.
	dEGE := new(edwards25519.ExtendedGroupElement)
	dEGE.Zero()

	// Use the doubling method for the multiplication.
	// p := given point
	// q := point(zero)
	// for each bit in the scalar, descending:
	//   double(q)
	//   if bit == 1:
	//     add(q, p)
	// return q
	//
	// Note that the addition is skipped for zero bits,
	// making this variable time and thus vulnerable to
	// side channel attack vectors.
	for i := s.BitLen() - 1; i >= 0; i-- {
		dCGE := new(edwards25519.CompletedGroupElement)
		dEGE.Double(dCGE)
		dCGE.ToExtended(dEGE)
		if s.Bit(i) == 1 {
			ss := new([32]byte)
			dEGE.ToBytes(ss)
			var err error
			xi, yi, err := curve.EncodedBytesToBigIntPoint(ss)
			if err != nil {
				return nil, nil
			}
			xAdd, yAdd := curve.Add(xi, yi, x1, y1)
			dTempBytes := BigIntPointToEncodedBytes(xAdd, yAdd)
			dEGE.FromBytes(dTempBytes)
		}
	}

	finalBytes := new([32]byte)
	dEGE.ToBytes(finalBytes)

	var err error
	x, y, err = curve.EncodedBytesToBigIntPoint(finalBytes)
	if err != nil {
		return nil, nil
	}

	return
}

// ScalarBaseMult returns k*G, where G is the base point of the group
// and k is an integer in big-endian form.
// TODO Optimize this with field elements
func (curve *TwistedEdwardsCurve) ScalarBaseMult(k []byte) (x, y *big.Int) {
	return curve.ScalarMult(curve.Gx, curve.Gy, k)
}

// ScalarAdd adds two scalars and returns the sum mod N.
func ScalarAdd(a, b *big.Int) *big.Int {
	feA := BigIntToFieldElement(a)
	feB := BigIntToFieldElement(b)
	sum := new(edwards25519.FieldElement)

	edwards25519.FeAdd(sum, feA, feB)
	sumArray := new([32]byte)
	edwards25519.FeToBytes(sumArray, sum)

	return EncodedBytesToBigInt(sumArray)
}

// InitParam25519 initializes an instance of the Ed25519 curve.
func (curve *TwistedEdwardsCurve) InitParam25519() {
	// The prime modulus of the field.
	// P = 2^255-19
	curve.CurveParams = new(elliptic.CurveParams)
	curve.P = new(big.Int)
	curve.P.SetBit(zero, 255, 1).Sub(curve.P, big.NewInt(19))

	// The prime order for the base point.
	// N = 2^252 + 27742317777372353535851937790883648493
	qs, _ := new(big.Int).SetString("27742317777372353535851937790883648493", 10)
	curve.N = new(big.Int)
	curve.N.SetBit(zero, 252, 1).Add(curve.N, qs) // AKA Q

	curve.A = new(big.Int)
	curve.A.SetInt64(-1).Add(curve.P, curve.A)

	// d = -121665 * inv(121666)
	da := new(big.Int).SetInt64(-121665)
	ds := new(big.Int).SetInt64(121666)
	di := curve.invert(ds)
	curve.D = new(big.Int).Mul(da, di)

	// I = expmod(2,(q-1)/4,q)
	psn := new(big.Int)
	psn.SetBit(zero, 255, 1).Sub(psn, big.NewInt(19))
	psn.Sub(psn, one)
	psn.Div(psn, four)
	curve.I = psn.Exp(two, psn, curve.P)

	// The base point.
	curve.Gx = new(big.Int)
	curve.Gx.SetString("151122213495354007725011514095885315"+
		"11454012693041857206046113283949847762202", 10)
	curve.Gy = new(big.Int)
	curve.Gy.SetString("463168356949264781694283940034751631"+
		"41307993866256225615783033603165251855960", 10)

	curve.BitSize = 256
	curve.H = 8

	// Provided for convenience since this gets computed repeatedly.
	curve.byteSize = curve.BitSize / 8
}

// Edwards returns a Curve which implements Ed25519.
func Edwards() *TwistedEdwardsCurve {
	c := new(TwistedEdwardsCurve)
	c.InitParam25519()
	return c
}
