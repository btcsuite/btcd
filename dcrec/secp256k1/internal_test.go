// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

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
func (f *FieldVal) TstRawInts() [10]uint32 {
	return f.n
}

// TstSetRawInts allows the test package to directly set the integers used by
// the internal field representation.  It is only available during the tests.
func (f *FieldVal) TstSetRawInts(raw [10]uint32) *FieldVal {
	for i := 0; i < len(raw); i++ {
		f.n[i] = raw[i]
	}
	return f
}

// TstFieldJacobianToBigAffine makes the internal FieldJacobianToBigAffine
// function available to the test package.
func (curve *KoblitzCurve) TstFieldJacobianToBigAffine(x, y, z *FieldVal) (*big.Int, *big.Int) {
	return curve.FieldJacobianToBigAffine(x, y, z)
}

// TstIsJacobianOnCurve returns boolean if the point (x,y,z) is on the curve.
func (curve *KoblitzCurve) TstIsJacobianOnCurve(x, y, z *FieldVal) bool {
	// Elliptic curve equation for secp256k1 is: y^2 = x^3 + 7
	// In Jacobian coordinates, Y = y/z^3 and X = x/z^2
	// Thus:
	// (y/z^3)^2 = (x/z^2)^3 + 7
	// y^2/z^6 = x^3/z^6 + 7
	// y^2 = x^3 + 7*z^6
	var y2, z2, x3, result FieldVal
	y2.SquareVal(y).Normalize()
	z2.SquareVal(z)
	x3.SquareVal(x).Mul(x)
	result.SquareVal(&z2).Mul(&z2).MulInt(7).Add(&x3).Normalize()
	return y2.Equals(&result)
}

// TstAddJacobian makes the internal addJacobian function available to the test
// package.
func (curve *KoblitzCurve) TstAddJacobian(x1, y1, z1, x2, y2, z2, x3, y3, z3 *FieldVal) {
	curve.AddJacobian(x1, y1, z1, x2, y2, z2, x3, y3, z3)
}

// TstDoubleJacobian makes the internal doubleJacobian function available to the test
// package.
func (curve *KoblitzCurve) TstDoubleJacobian(x1, y1, z1, x3, y3, z3 *FieldVal) {
	curve.doubleJacobian(x1, y1, z1, x3, y3, z3)
}

// NewFieldVal returns a new field value set to 0.  This is only available to
// the test package.
func NewFieldVal() *FieldVal {
	return new(FieldVal)
}

// TstNonceRFC6979 makes the nonceRFC6979 function available to the test package.
func TstNonceRFC6979(privkey *big.Int, hash []byte) *big.Int {
	return NonceRFC6979(privkey, hash, nil, nil)
}

// TstRemovePKCSPadding makes the internal removePKCSPadding function available
// to the test package.
func TstRemovePKCSPadding(src []byte) ([]byte, error) {
	return removePKCSPadding(src)
}
