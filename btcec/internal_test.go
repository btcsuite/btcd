// Copyright (c) 2013-2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcec

import (
	"math/big"
)

const (
	TstPubkeyUncompressed = pubkeyUncompressed
	TstPubkeyCompressed   = pubkeyCompressed
	TstPubkeyHybrid       = pubkeyHybrid
)

// TstRawInts allows the test package to get the integers from the internal
// field representation for ensuring correctness.  It is only available during
// the tests.
func (f *fieldVal) TstRawInts() [10]uint32 {
	return f.n
}

// TstSetRawInts allows the test package to directly set the integers used by
// the internal field representation.  It is only available during the tests.
func (f *fieldVal) TstSetRawInts(raw [10]uint32) *fieldVal {
	for i := 0; i < len(raw); i++ {
		f.n[i] = raw[i]
	}
	return f
}

// TstFieldJacobianToBigAffine makes the internal fieldJacobianToBigAffine
// function available to the test package.
func (curve *KoblitzCurve) TstFieldJacobianToBigAffine(x, y, z *fieldVal) (*big.Int, *big.Int) {
	return curve.fieldJacobianToBigAffine(x, y, z)
}

// TstIsJacobianOnCurve returns boolean if the point (x,y,z) is on the curve.
func (curve *KoblitzCurve) TstIsJacobianOnCurve(x, y, z *fieldVal) bool {
	// Elliptic curve equation for secp256k1 is: y^2 = x^3 + 7
	// In Jacobian coordinates, Y = y/z^3 and X = x/z^2
	// Thus:
	// (y/z^3)^2 = (x/z^2)^3 + 7
	// y^2/z^6 = x^3/z^6 + 7
	// y^2 = x^3 + 7*z^6
	var y2, z2, x3, result fieldVal
	y2.SquareVal(y).Normalize()
	z2.SquareVal(z)
	x3.SquareVal(x).Mul(x)
	result.SquareVal(&z2).Mul(&z2).MulInt(7).Add(&x3).Normalize()
	return y2.Equals(&result)
}

// TstAddJacobian makes the internal addJacobian function available to the test
// package.
func (curve *KoblitzCurve) TstAddJacobian(x1, y1, z1, x2, y2, z2, x3, y3, z3 *fieldVal) {
	curve.addJacobian(x1, y1, z1, x2, y2, z2, x3, y3, z3)
}

// TstDoubleJacobian makes the internal doubleJacobian function available to the test
// package.
func (curve *KoblitzCurve) TstDoubleJacobian(x1, y1, z1, x3, y3, z3 *fieldVal) {
	curve.doubleJacobian(x1, y1, z1, x3, y3, z3)
}

// NewFieldVal returns a new field value set to 0.  This is only available to
// the test package.
func NewFieldVal() *fieldVal {
	return new(fieldVal)
}

// TstNonceRFC6979 makes the nonceRFC6979 function available to the test package.
func TstNonceRFC6979(privkey *big.Int, hash []byte) *big.Int {
	return nonceRFC6979(privkey, hash)
}

// TstRemovePKCSPadding makes the internal removePKCSPadding function available
// to the test package.
func TstRemovePKCSPadding(src []byte) ([]byte, error) {
	return removePKCSPadding(src)
}
