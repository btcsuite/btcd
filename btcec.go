// Copyright 2010 The Go Authors. All rights reserved.
// Copyright 2011 ThePiachu. All rights reserved.
// Copyright 2013-2014 Conformal Systems LLC. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package btcec

// References:
//   [SECG]: Recommended Elliptic Curve Domain Parameters
//     http://www.secg.org/download/aid-784/sec2-v2.pdf

// This package operates, internally, on Jacobian coordinates. For a given
// (x, y) position on the curve, the Jacobian coordinates are (x1, y1, z1)
// where x = x1/z1² and y = y1/z1³. The greatest speedups come when the whole
// calculation can be performed within the transform (as in ScalarMult and
// ScalarBaseMult). But even for Add and Double, it's faster to apply and
// reverse the transform than to operate in affine coordinates.

import (
	"crypto/elliptic"
	"math/big"
	"sync"
)

//TODO: examine if we need to care about EC optimization as descibed here
//      https://bitcointalk.org/index.php?topic=155054.0;all

var (
	// fieldOne is simply the integer 1 in field representation.  It is
	// used to avoid needing to create it multiple times during the internal
	// arithmetic.
	fieldOne = new(fieldVal).SetInt(1)
)

// KoblitzCurve supports a koblitz curve implementation that fits the ECC Curve
// interface from crypto/elliptic.
type KoblitzCurve struct {
	*elliptic.CurveParams
	q *big.Int
	H int // cofactor of the curve.
}

// Params returns the parameters for the curve.
func (curve *KoblitzCurve) Params() *elliptic.CurveParams {
	return curve.CurveParams
}

// bigAffineToField takes an affine point (x, y) as big integers and converts
// it to an affine point as field values.
func (curve *KoblitzCurve) bigAffineToField(x, y *big.Int) (*fieldVal, *fieldVal) {
	x3, y3 := new(fieldVal), new(fieldVal)
	x3.SetByteSlice(x.Bytes())
	y3.SetByteSlice(y.Bytes())

	return x3, y3
}

// fieldJacobianToBigAffine takes a Jacobian point (x, y, z) as field values and
// converts it to an affine point as big integers.
func (curve *KoblitzCurve) fieldJacobianToBigAffine(x, y, z *fieldVal) (*big.Int, *big.Int) {
	// Inversions are expensive and both point addition and point doubling
	// are faster when working with points that have a z value of one.  So,
	// if the point needs to be converted to affine, go ahead and normalize
	// the point itself at the same time as the calculation is the same.
	var zInv, tempZ fieldVal
	zInv.Set(z).Inverse()   // zInv = Z^-1
	tempZ.SquareVal(&zInv)  // tempZ = Z^-2
	x.Mul(&tempZ)           // X = X/Z^2 (mag: 1)
	y.Mul(tempZ.Mul(&zInv)) // Y = Y/Z^3 (mag: 1)
	z.SetInt(1)             // Z = 1 (mag: 1)

	// Normalize the x and y values.
	x.Normalize()
	y.Normalize()

	// Convert the field values for the now affine point to big.Ints.
	x3, y3 := new(big.Int), new(big.Int)
	x3.SetBytes(x.Bytes()[:])
	y3.SetBytes(y.Bytes()[:])
	return x3, y3
}

// IsOnCurve returns boolean if the point (x,y) is on the curve.
// Part of the elliptic.Curve interface. This function differs from the
// crypto/elliptic algorithm since a = 0 not -3.
func (curve *KoblitzCurve) IsOnCurve(x, y *big.Int) bool {
	// Convert big ints to field values for faster arithmetic.
	fx, fy := curve.bigAffineToField(x, y)

	// Elliptic curve equation for secp256k1 is: y^2 = x^3 + 7
	y2 := new(fieldVal).SquareVal(fy).Normalize()
	result := new(fieldVal).SquareVal(fx).Mul(fx).AddInt(7).Normalize()
	return y2.Equals(result)
}

// addZ1AndZ2EqualsOne adds two Jacobian points that are already known to have
// z values of 1 and stores the result in (x3, y3, z3).  That is to say
// (x1, y1, 1) + (x2, y2, 1) = (x3, y3, z3).  It performs faster addition than
// the generic add routine since less arithmetic is needed due to the ability to
// avoid the z value multiplications.
func (curve *KoblitzCurve) addZ1AndZ2EqualsOne(x1, y1, z1, x2, y2, x3, y3, z3 *fieldVal) {
	// To compute the point addition efficiently, this implementation splits
	// the equation into intermediate elements which are used to minimize
	// the number of field multiplications using the method shown at:
	// http://hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-0.html#addition-mmadd-2007-bl
	//
	// In particular it performs the calculations using the following:
	// H = X2-X1, HH = H^2, I = 4*HH, J = H*I, r = 2*(Y2-Y1), V = X1*I
	// X3 = r^2-J-2*V, Y3 = r*(V-X3)-2*Y1*J, Z3 = 2*H
	//
	// This results in a cost of 4 field multiplications, 2 field squarings,
	// 6 field additions, and 5 integer multiplications.

	// When the x coordinates are the same for two points on the curve, the
	// y coordinates either must be the same, in which case it is point
	// doubling, or they are opposite and the result is the point at
	// infinity per the group law for elliptic curve cryptography.
	x1.Normalize()
	y1.Normalize()
	x2.Normalize()
	y2.Normalize()
	if x1.Equals(x2) {
		if y1.Equals(y2) {
			// Since x1 == x2 and y1 == y2, point doubling must be
			// done, otherwise the addition would end up dividing
			// by zero.
			curve.doubleJacobian(x1, y1, z1, x3, y3, z3)
			return
		}

		// Since x1 == x2 and y1 == -y2, the sum is the point at
		// infinity per the group law.
		x3.SetInt(0)
		y3.SetInt(0)
		z3.SetInt(0)
		return
	}

	// Calculate X3, Y3, and Z3 according to the intermediate elements
	// breakdown above.
	var h, i, j, r, v fieldVal
	var negJ, neg2V, negX3 fieldVal
	h.Set(x1).Negate(1).Add(x2)                // H = X2-X1 (mag: 3)
	i.SquareVal(&h).MulInt(4)                  // I = 4*H^2 (mag: 4)
	j.Mul2(&h, &i)                             // J = H*I (mag: 1)
	r.Set(y1).Negate(1).Add(y2).MulInt(2)      // r = 2*(Y2-Y1) (mag: 6)
	v.Mul2(x1, &i)                             // V = X1*I (mag: 1)
	negJ.Set(&j).Negate(1)                     // negJ = -J (mag: 2)
	neg2V.Set(&v).MulInt(2).Negate(2)          // neg2V = -(2*V) (mag: 3)
	x3.Set(&r).Square().Add(&negJ).Add(&neg2V) // X3 = r^2-J-2*V (mag: 6)
	negX3.Set(x3).Negate(6)                    // negX3 = -X3 (mag: 7)
	j.Mul(y1).MulInt(2).Negate(2)              // J = -(2*Y1*J) (mag: 3)
	y3.Set(&v).Add(&negX3).Mul(&r).Add(&j)     // Y3 = r*(V-X3)-2*Y1*J (mag: 4)
	z3.Set(&h).MulInt(2)                       // Z3 = 2*H (mag: 6)

	// Normalize the resulting field values to a magnitude of 1 as needed.
	x3.Normalize()
	y3.Normalize()
	z3.Normalize()
}

// addZ1EqualsZ2 adds two Jacobian points that are already known to have the
// same z value and stores the result in (x3, y3, z3).  That is to say
// (x1, y1, z1) + (x2, y2, z1) = (x3, y3, z3).  It performs faster addition than
// the generic add routine since less arithmetic is needed due to the known
// equivalence.
func (curve *KoblitzCurve) addZ1EqualsZ2(x1, y1, z1, x2, y2, x3, y3, z3 *fieldVal) {
	// To compute the point addition efficiently, this implementation splits
	// the equation into intermediate elements which are used to minimize
	// the number of field multiplications using a slightly modified version
	// of the method shown at:
	// http://hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-0.html#addition-mmadd-2007-bl
	//
	// In particular it performs the calculations using the following:
	// A = X2-X1, B = A^2, C=Y2-Y1, D = C^2, E = X1*B, F = X2*B
	// X3 = D-E-F, Y3 = C*(E-X3)-Y1*(F-E), Z3 = Z1*A
	//
	// This results in a cost of 5 field multiplications, 2 field squarings,
	// 9 field additions, and 0 integer multiplications.

	// When the x coordinates are the same for two points on the curve, the
	// y coordinates either must be the same, in which case it is point
	// doubling, or they are opposite and the result is the point at
	// infinity per the group law for elliptic curve cryptography.
	x1.Normalize()
	y1.Normalize()
	x2.Normalize()
	y2.Normalize()
	if x1.Equals(x2) {
		if y1.Equals(y2) {
			// Since x1 == x2 and y1 == y2, point doubling must be
			// done, otherwise the addition would end up dividing
			// by zero.
			curve.doubleJacobian(x1, y1, z1, x3, y3, z3)
			return
		}

		// Since x1 == x2 and y1 == -y2, the sum is the point at
		// infinity per the group law.
		x3.SetInt(0)
		y3.SetInt(0)
		z3.SetInt(0)
		return
	}

	// Calculate X3, Y3, and Z3 according to the intermediate elements
	// breakdown above.
	var a, b, c, d, e, f fieldVal
	var negX1, negY1, negE, negX3 fieldVal
	negX1.Set(x1).Negate(1)                // negX1 = -X1 (mag: 2)
	negY1.Set(y1).Negate(1)                // negY1 = -Y1 (mag: 2)
	a.Set(&negX1).Add(x2)                  // A = X2-X1 (mag: 3)
	b.SquareVal(&a)                        // B = A^2 (mag: 1)
	c.Set(&negY1).Add(y2)                  // C = Y2-Y1 (mag: 3)
	d.SquareVal(&c)                        // D = C^2 (mag: 1)
	e.Mul2(x1, &b)                         // E = X1*B (mag: 1)
	negE.Set(&e).Negate(1)                 // negE = -E (mag: 2)
	f.Mul2(x2, &b)                         // F = X2*B (mag: 1)
	x3.Add2(&e, &f).Negate(3).Add(&d)      // X3 = D-E-F (mag: 5)
	negX3.Set(x3).Negate(5).Normalize()    // negX3 = -X3 (mag: 1)
	y3.Set(y1).Mul(f.Add(&negE)).Negate(3) // Y3 = -(Y1*(F-E)) (mag: 4)
	y3.Add(e.Add(&negX3).Mul(&c))          // Y3 = C*(E-X3)+Y3 (mag: 5)
	z3.Mul2(z1, &a)                        // Z3 = Z1*A (mag: 1)

	// Normalize the resulting field values to a magnitude of 1 as needed.
	x3.Normalize()
	y3.Normalize()
}

// addZ2EqualsOne adds two Jacobian points when the second point is already
// known to have a z value of 1 (and the z value for the first point is not 1)
// and stores the result in (x3, y3, z3).  That is to say (x1, y1, z1) +
// (x2, y2, 1) = (x3, y3, z3).  It performs faster addition than the generic
// add routine since less arithmetic is needed due to the ability to avoid
// multiplications by the second point's z value.
func (curve *KoblitzCurve) addZ2EqualsOne(x1, y1, z1, x2, y2, x3, y3, z3 *fieldVal) {
	// To compute the point addition efficiently, this implementation splits
	// the equation into intermediate elements which are used to minimize
	// the number of field multiplications using the method shown at:
	// http://hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-0.html#addition-madd-2007-bl
	//
	// In particular it performs the calculations using the following:
	// Z1Z1 = Z1^2, U2 = X2*Z1Z1, S2 = Y2*Z1*Z1Z1, H = U2-X1, HH = H^2,
	// I = 4*HH, J = H*I, r = 2*(S2-Y1), V = X1*I
	// X3 = r^2-J-2*V, Y3 = r*(V-X3)-2*Y1*J, Z3 = (Z1+H)^2-Z1Z1-HH
	//
	// This results in a cost of 7 field multiplications, 4 field squarings,
	// 9 field additions, and 4 integer multiplications.

	// When the x coordinates are the same for two points on the curve, the
	// y coordinates either must be the same, in which case it is point
	// doubling, or they are opposite and the result is the point at
	// infinity per the group law for elliptic curve cryptography.  Since
	// any number of Jacobian coordinates can represent the same affine
	// point, the x and y values need to be converted to like terms.  Due to
	// the assumption made for this function that the second point has a z
	// value of 1 (z2=1), the first point is already "converted".
	var z1z1, u2, s2 fieldVal
	x1.Normalize()
	y1.Normalize()
	z1z1.SquareVal(z1)                        // Z1Z1 = Z1^2 (mag: 1)
	u2.Set(x2).Mul(&z1z1).Normalize()         // U2 = X2*Z1Z1 (mag: 1)
	s2.Set(y2).Mul(&z1z1).Mul(z1).Normalize() // S2 = Y2*Z1*Z1Z1 (mag: 1)
	if x1.Equals(&u2) {
		if y1.Equals(&s2) {
			// Since x1 == x2 and y1 == y2, point doubling must be
			// done, otherwise the addition would end up dividing
			// by zero.
			curve.doubleJacobian(x1, y1, z1, x3, y3, z3)
			return
		}

		// Since x1 == x2 and y1 == -y2, the sum is the point at
		// infinity per the group law.
		x3.SetInt(0)
		y3.SetInt(0)
		z3.SetInt(0)
		return
	}

	// Calculate X3, Y3, and Z3 according to the intermediate elements
	// breakdown above.
	var h, hh, i, j, r, rr, v fieldVal
	var negX1, negY1, negX3 fieldVal
	negX1.Set(x1).Negate(1)                // negX1 = -X1 (mag: 2)
	h.Add2(&u2, &negX1)                    // H = U2-X1 (mag: 3)
	hh.SquareVal(&h)                       // HH = H^2 (mag: 1)
	i.Set(&hh).MulInt(4)                   // I = 4 * HH (mag: 4)
	j.Mul2(&h, &i)                         // J = H*I (mag: 1)
	negY1.Set(y1).Negate(1)                // negY1 = -Y1 (mag: 2)
	r.Set(&s2).Add(&negY1).MulInt(2)       // r = 2*(S2-Y1) (mag: 6)
	rr.SquareVal(&r)                       // rr = r^2 (mag: 1)
	v.Mul2(x1, &i)                         // V = X1*I (mag: 1)
	x3.Set(&v).MulInt(2).Add(&j).Negate(3) // X3 = -(J+2*V) (mag: 4)
	x3.Add(&rr)                            // X3 = r^2+X3 (mag: 5)
	negX3.Set(x3).Negate(5)                // negX3 = -X3 (mag: 6)
	y3.Set(y1).Mul(&j).MulInt(2).Negate(2) // Y3 = -(2*Y1*J) (mag: 3)
	y3.Add(v.Add(&negX3).Mul(&r))          // Y3 = r*(V-X3)+Y3 (mag: 4)
	z3.Add2(z1, &h).Square()               // Z3 = (Z1+H)^2 (mag: 1)
	z3.Add(z1z1.Add(&hh).Negate(2))        // Z3 = Z3-(Z1Z1+HH) (mag: 4)

	// Normalize the resulting field values to a magnitude of 1 as needed.
	x3.Normalize()
	y3.Normalize()
	z3.Normalize()
}

// addGeneric adds two Jacobian points (x1, y1, z1) and (x2, y2, z2) without any
// assumptions about the z values of the two points and stores the result in
// (x3, y3, z3).  That is to say (x1, y1, z1) + (x2, y2, z2) = (x3, y3, z3).  It
// is the slowest of the add routines due to requiring the most arithmetic.
func (curve *KoblitzCurve) addGeneric(x1, y1, z1, x2, y2, z2, x3, y3, z3 *fieldVal) {
	// To compute the point addition efficiently, this implementation splits
	// the equation into intermediate elements which are used to minimize
	// the number of field multiplications using the method shown at:
	// http://hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-0.html#addition-add-2007-bl
	//
	// In particular it performs the calculations using the following:
	// Z1Z1 = Z1^2, Z2Z2 = Z2^2, U1 = X1*Z2Z2, U2 = X2*Z1Z1, S1 = Y1*Z2*Z2Z2
	// S2 = Y2*Z1*Z1Z1, H = U2-U1, I = (2*H)^2, J = H*I, r = 2*(S2-S1)
	// V = U1*I
	// X3 = r^2-J-2*V, Y3 = r*(V-X3)-2*S1*J, Z3 = ((Z1+Z2)^2-Z1Z1-Z2Z2)*H
	//
	// This results in a cost of 11 field multiplications, 5 field squarings,
	// 9 field additions, and 4 integer multiplications.

	// When the x coordinates are the same for two points on the curve, the
	// y coordinates either must be the same, in which case it is point
	// doubling, or they are opposite and the result is the point at
	// infinity.  Since any number of Jacobian coordinates can represent the
	// same affine point, the x and y values need to be converted to like
	// terms.
	var z1z1, z2z2, u1, u2, s1, s2 fieldVal
	z1z1.SquareVal(z1)                        // Z1Z1 = Z1^2 (mag: 1)
	z2z2.SquareVal(z2)                        // Z2Z2 = Z2^2 (mag: 1)
	u1.Set(x1).Mul(&z2z2).Normalize()         // U1 = X1*Z2Z2 (mag: 1)
	u2.Set(x2).Mul(&z1z1).Normalize()         // U2 = X2*Z1Z1 (mag: 1)
	s1.Set(y1).Mul(&z2z2).Mul(z2).Normalize() // S1 = Y1*Z2*Z2Z2 (mag: 1)
	s2.Set(y2).Mul(&z1z1).Mul(z1).Normalize() // S2 = Y2*Z1*Z1Z1 (mag: 1)
	if u1.Equals(&u2) {
		if s1.Equals(&s2) {
			// Since x1 == x2 and y1 == y2, point doubling must be
			// done, otherwise the addition would end up dividing
			// by zero.
			curve.doubleJacobian(x1, y1, z1, x3, y3, z3)
			return
		}

		// Since x1 == x2 and y1 == -y2, the sum is the point at
		// infinity per the group law.
		x3.SetInt(0)
		y3.SetInt(0)
		z3.SetInt(0)
		return
	}

	// Calculate X3, Y3, and Z3 according to the intermediate elements
	// breakdown above.
	var h, i, j, r, rr, v fieldVal
	var negU1, negS1, negX3 fieldVal
	negU1.Set(&u1).Negate(1)               // negU1 = -U1 (mag: 2)
	h.Add2(&u2, &negU1)                    // H = U2-U1 (mag: 3)
	i.Set(&h).MulInt(2).Square()           // I = (2*H)^2 (mag: 2)
	j.Mul2(&h, &i)                         // J = H*I (mag: 1)
	negS1.Set(&s1).Negate(1)               // negS1 = -S1 (mag: 2)
	r.Set(&s2).Add(&negS1).MulInt(2)       // r = 2*(S2-S1) (mag: 6)
	rr.SquareVal(&r)                       // rr = r^2 (mag: 1)
	v.Mul2(&u1, &i)                        // V = U1*I (mag: 1)
	x3.Set(&v).MulInt(2).Add(&j).Negate(3) // X3 = -(J+2*V) (mag: 4)
	x3.Add(&rr)                            // X3 = r^2+X3 (mag: 5)
	negX3.Set(x3).Negate(5)                // negX3 = -X3 (mag: 6)
	y3.Mul2(&s1, &j).MulInt(2).Negate(2)   // Y3 = -(2*S1*J) (mag: 3)
	y3.Add(v.Add(&negX3).Mul(&r))          // Y3 = r*(V-X3)+Y3 (mag: 4)
	z3.Add2(z1, z2).Square()               // Z3 = (Z1+Z2)^2 (mag: 1)
	z3.Add(z1z1.Add(&z2z2).Negate(2))      // Z3 = Z3-(Z1Z1+Z2Z2) (mag: 4)
	z3.Mul(&h)                             // Z3 = Z3*H (mag: 1)

	// Normalize the resulting field values to a magnitude of 1 as needed.
	x3.Normalize()
	y3.Normalize()
}

// addJacobian adds the passed Jacobian points (x1, y1, z1) and (x2, y2, z2)
// together and stores the result in (x3, y3, z3).
func (curve *KoblitzCurve) addJacobian(x1, y1, z1, x2, y2, z2, x3, y3, z3 *fieldVal) {
	// A point at infinity is the identity according to the group law for
	// elliptic curve cryptography.  Thus, ∞ + P = P and P + ∞ = P.
	if (x1.IsZero() && y1.IsZero()) || z1.IsZero() {
		x3.Set(x2)
		y3.Set(y2)
		z3.Set(z2)
		return
	}
	if (x2.IsZero() && y2.IsZero()) || z2.IsZero() {
		x3.Set(x1)
		y3.Set(y1)
		z3.Set(z1)
		return
	}

	// Faster point addition can be achieved when certain assumptions are
	// met.  For example, when both points have the same z value, arithmetic
	// on the z values can be avoided.  This section thus checks for these
	// conditions and calls an appropriate add function which is accelerated
	// by using those assumptions.
	z1.Normalize()
	z2.Normalize()
	isZ1One := z1.Equals(fieldOne)
	isZ2One := z2.Equals(fieldOne)
	switch {
	case isZ1One && isZ2One:
		curve.addZ1AndZ2EqualsOne(x1, y1, z1, x2, y2, x3, y3, z3)
		return
	case z1.Equals(z2):
		curve.addZ1EqualsZ2(x1, y1, z1, x2, y2, x3, y3, z3)
		return
	case isZ2One:
		curve.addZ2EqualsOne(x1, y1, z1, x2, y2, x3, y3, z3)
		return
	}

	// None of the above assumptions are true, so fall back to generic
	// point addition.
	curve.addGeneric(x1, y1, z1, x2, y2, z2, x3, y3, z3)
}

// Add returns the sum of (x1,y1) and (x2,y2). Part of the elliptic.Curve
// interface.
func (curve *KoblitzCurve) Add(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	// A point at infinity is the identity according to the group law for
	// elliptic curve cryptography.  Thus, ∞ + P = P and P + ∞ = P.
	if x1.Sign() == 0 && y1.Sign() == 0 {
		return x2, y2
	}
	if x2.Sign() == 0 && y2.Sign() == 0 {
		return x1, y1
	}

	// Convert the affine coordinates from big integers to field values
	// and do the point addition in Jacobian projective space.
	fx1, fy1 := curve.bigAffineToField(x1, y1)
	fx2, fy2 := curve.bigAffineToField(x2, y2)
	fx3, fy3, fz3 := new(fieldVal), new(fieldVal), new(fieldVal)
	fOne := new(fieldVal).SetInt(1)
	curve.addJacobian(fx1, fy1, fOne, fx2, fy2, fOne, fx3, fy3, fz3)

	// Convert the Jacobian coordinate field values back to affine big
	// integers.
	return curve.fieldJacobianToBigAffine(fx3, fy3, fz3)
}

// doubleZ1EqualsOne performs point doubling on the passed Jacobian point
// when the point is already known to have a z value of 1 and stores
// the result in (x3, y3, z3).  That is to say (x3, y3, z3) = 2*(x1, y1, 1).  It
// performs faster point doubling than the generic routine since less arithmetic
// is needed due to the ability to avoid multiplication by the z value.
func (curve *KoblitzCurve) doubleZ1EqualsOne(x1, y1, x3, y3, z3 *fieldVal) {
	// This function uses the assumptions that z1 is 1, thus the point
	// doubling formulas reduce to:
	//
	// X3 = (3*X1^2)^2 - 8*X1*Y1^2
	// Y3 = (3*X1^2)*(4*X1*Y1^2 - X3) - 8*Y1^4
	// Z3 = 2*Y1
	//
	// To compute the above efficiently, this implementation splits the
	// equation into intermediate elements which are used to minimize the
	// number of field multiplications in favor of field squarings which
	// are roughly 35% faster than field multiplications with the current
	// implementation at the time this was written.
	//
	// This uses a slightly modified version of the method shown at:
	// http://hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-0.html#doubling-mdbl-2007-bl
	//
	// In particular it performs the calculations using the following:
	// A = X1^2, B = Y1^2, C = B^2, D = 2*((X1+B)^2-A-C)
	// E = 3*A, F = E^2, X3 = F-2*D, Y3 = E*(D-X3)-8*C
	// Z3 = 2*Y1
	//
	// This results in a cost of 1 field multiplication, 5 field squarings,
	// 6 field additions, and 5 integer multiplications.
	var a, b, c, d, e, f fieldVal
	z3.Set(y1).MulInt(2)                     // Z3 = 2*Y1 (mag: 2)
	a.SquareVal(x1)                          // A = X1^2 (mag: 1)
	b.SquareVal(y1)                          // B = Y1^2 (mag: 1)
	c.SquareVal(&b)                          // C = B^2 (mag: 1)
	b.Add(x1).Square()                       // B = (X1+B)^2 (mag: 1)
	d.Set(&a).Add(&c).Negate(2)              // D = -(A+C) (mag: 3)
	d.Add(&b).MulInt(2)                      // D = 2*(B+D)(mag: 8)
	e.Set(&a).MulInt(3)                      // E = 3*A (mag: 3)
	f.SquareVal(&e)                          // F = E^2 (mag: 1)
	x3.Set(&d).MulInt(2).Negate(16)          // X3 = -(2*D) (mag: 17)
	x3.Add(&f)                               // X3 = F+X3 (mag: 18)
	f.Set(x3).Negate(18).Add(&d).Normalize() // F = D-X3 (mag: 1)
	y3.Set(&c).MulInt(8).Negate(8)           // Y3 = -(8*C) (mag: 9)
	y3.Add(f.Mul(&e))                        // Y3 = E*F+Y3 (mag: 10)

	// Normalize the field values back to a magnitude of 1.
	x3.Normalize()
	y3.Normalize()
	z3.Normalize()
}

// doubleGeneric performs point doubling on the passed Jacobian point without
// any assumptions about the z value and stores the result in (x3, y3, z3).
// That is to say (x3, y3, z3) = 2*(x1, y1, z1).  It is the slowest of the point
// doubling routines due to requiring the most arithmetic.
func (cuve *KoblitzCurve) doubleGeneric(x1, y1, z1, x3, y3, z3 *fieldVal) {
	// Point doubling formula for Jacobian coordinates for the secp256k1
	// curve:
	// X3 = (3*X1^2)^2 - 8*X1*Y1^2
	// Y3 = (3*X1^2)*(4*X1*Y1^2 - X3) - 8*Y1^4
	// Z3 = 2*Y1*Z1
	//
	// To compute the above efficiently, this implementation splits the
	// equation into intermediate elements which are used to minimize the
	// number of field multiplications in favor of field squarings which
	// are roughly 35% faster than field multiplications with the current
	// implementation at the time this was written.
	//
	// This uses a slightly modified version of the method shown at:
	// http://hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-0.html#doubling-dbl-2009-l
	//
	// In particular it performs the calculations using the following:
	// A = X1^2, B = Y1^2, C = B^2, D = 2*((X1+B)^2-A-C)
	// E = 3*A, F = E^2, X3 = F-2*D, Y3 = E*(D-X3)-8*C
	// Z3 = 2*Y1*Z1
	//
	// This results in a cost of 1 field multiplication, 5 field squarings,
	// 6 field additions, and 5 integer multiplications.
	var a, b, c, d, e, f fieldVal
	z3.Mul2(y1, z1).MulInt(2)                // Z3 = 2*Y1*Z1 (mag: 2)
	a.SquareVal(x1)                          // A = X1^2 (mag: 1)
	b.SquareVal(y1)                          // B = Y1^2 (mag: 1)
	c.SquareVal(&b)                          // C = B^2 (mag: 1)
	b.Add(x1).Square()                       // B = (X1+B)^2 (mag: 1)
	d.Set(&a).Add(&c).Negate(2)              // D = -(A+C) (mag: 3)
	d.Add(&b).MulInt(2)                      // D = 2*(B+D)(mag: 8)
	e.Set(&a).MulInt(3)                      // E = 3*A (mag: 3)
	f.SquareVal(&e)                          // F = E^2 (mag: 1)
	x3.Set(&d).MulInt(2).Negate(16)          // X3 = -(2*D) (mag: 17)
	x3.Add(&f)                               // X3 = F+X3 (mag: 18)
	f.Set(x3).Negate(18).Add(&d).Normalize() // F = D-X3 (mag: 1)
	y3.Set(&c).MulInt(8).Negate(8)           // Y3 = -(8*C) (mag: 9)
	y3.Add(f.Mul(&e))                        // Y3 = E*F+Y3 (mag: 10)

	// Normalize the field values back to a magnitude of 1.
	x3.Normalize()
	y3.Normalize()
	z3.Normalize()
}

// doubleJacobian doubles the passed Jacobian point (x1, y1, z1) and stores the
// result in (x3, y3, z3).
func (curve *KoblitzCurve) doubleJacobian(x1, y1, z1, x3, y3, z3 *fieldVal) {
	// Doubling a point at infinity is still infinity.
	if y1.IsZero() || z1.IsZero() {
		x3.SetInt(0)
		y3.SetInt(0)
		z3.SetInt(0)
		return
	}

	// Slightly faster point doubling can be achieved when the z value is 1
	// by avoiding the multiplication on the z value.  This section calls
	// a point doubling function which is accelerated by using that
	// assumption when possible.
	if z1.Normalize().Equals(fieldOne) {
		curve.doubleZ1EqualsOne(x1, y1, x3, y3, z3)
		return
	}

	// Fall back to generic point doubling which works with arbitrary z
	// values.
	curve.doubleGeneric(x1, y1, z1, x3, y3, z3)
}

// Double returns 2*(x1,y1). Part of the elliptic.Curve interface.
func (curve *KoblitzCurve) Double(x1, y1 *big.Int) (*big.Int, *big.Int) {
	if y1.Sign() == 0 {
		return new(big.Int), new(big.Int)
	}

	// Convert the affine coordinates from big integers to field values
	// and do the point doubling in Jacobian projective space.
	fx1, fy1 := curve.bigAffineToField(x1, y1)
	fx3, fy3, fz3 := new(fieldVal), new(fieldVal), new(fieldVal)
	fOne := new(fieldVal).SetInt(1)
	curve.doubleJacobian(fx1, fy1, fOne, fx3, fy3, fz3)

	// Convert the Jacobian coordinate field values back to affine big
	// integers.
	return curve.fieldJacobianToBigAffine(fx3, fy3, fz3)
}

// ScalarMult returns k*(Bx, By) where k is a big endian integer.
// Part of the elliptic.Curve interface.
func (curve *KoblitzCurve) ScalarMult(Bx, By *big.Int, k []byte) (*big.Int, *big.Int) {
	// This uses the left to right binary method for point multiplication:

	// Point Q = ∞ (point at infinity).
	qx, qy, qz := new(fieldVal), new(fieldVal), new(fieldVal)

	// Point P = the point to multiply the scalar with.
	px, py := curve.bigAffineToField(Bx, By)
	pz := new(fieldVal).SetInt(1)

	// Double and add as necessary depending on the bits set in the scalar.
	for _, byteVal := range k {
		for bitNum := 0; bitNum < 8; bitNum++ {
			// Q = 2*Q
			curve.doubleJacobian(qx, qy, qz, qx, qy, qz)
			if byteVal&0x80 == 0x80 {
				// Q = Q + P
				curve.addJacobian(qx, qy, qz, px, py, pz, qx,
					qy, qz)
			}
			byteVal <<= 1
		}
	}

	// Convert the Jacobian coordinate field values back to affine big.Ints.
	return curve.fieldJacobianToBigAffine(qx, qy, qz)
}

// ScalarBaseMult returns k*G where G is the base point of the group and k is a
// big endian integer.
// Part of the elliptic.Curve interface.
func (curve *KoblitzCurve) ScalarBaseMult(k []byte) (*big.Int, *big.Int) {
	return curve.ScalarMult(curve.Gx, curve.Gy, k)
}

// QPlus1Div4 returns the Q+1/4 constant for the curve for use in calculating
// square roots via exponention.
func (curve *KoblitzCurve) QPlus1Div4() *big.Int {
	return curve.q
}

// Curve parameters taken from: http://www.secg.org/collateral/sec2_final.pdf
var initonce sync.Once
var secp256k1 KoblitzCurve

func initAll() {
	initS256()
}

func initS256() {
	// See [SECG] section 2.7.1
	secp256k1.CurveParams = new(elliptic.CurveParams)
	secp256k1.P, _ = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC2F", 16)
	secp256k1.N, _ = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	secp256k1.B, _ = new(big.Int).SetString("0000000000000000000000000000000000000000000000000000000000000007", 16)
	secp256k1.Gx, _ = new(big.Int).SetString("79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798", 16)
	secp256k1.Gy, _ = new(big.Int).SetString("483ADA7726A3C4655DA4FBFC0E1108A8FD17B448A68554199C47D08FFB10D4B8", 16)
	secp256k1.BitSize = 256
	secp256k1.H = 1
	secp256k1.q = new(big.Int).Div(new(big.Int).Add(secp256k1.P,
		big.NewInt(1)), big.NewInt(4))
}

// S256 returns a Curve which implements secp256k1.
func S256() *KoblitzCurve {
	initonce.Do(initAll)
	return &secp256k1
}
