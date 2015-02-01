// Copyright (c) 2014-2015 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is ignored during the regular build due to the following build tag.
// This build tag is set during go generate.
// +build gensecp256k1

package btcec

import "encoding/binary"

// secp256k1BytePoints are dummy points used so the code which generates the
// real values can compile.
var secp256k1BytePoints = []byte{}

// getDoublingPoints returns all the possible G^(2^i) for i in
// 0..n-1 where n is the curve's bit size (256 in the case of secp256k1)
// the coordinates are recorded as Jacobian coordinates.
func (curve *KoblitzCurve) getDoublingPoints() [][3]fieldVal {
	bitSize := curve.Params().BitSize
	doublingPoints := make([][3]fieldVal, bitSize)

	// initialize px, py, pz to the Jacobian coordinates for the base point
	px, py := curve.bigAffineToField(curve.Gx, curve.Gy)
	pz := new(fieldVal).SetInt(1)
	for i := 0; i < bitSize; i++ {
		doublingPoints[i] = [3]fieldVal{*px, *py, *pz}
		// P = 2*P
		curve.doubleJacobian(px, py, pz, px, py, pz)
	}
	return doublingPoints
}

// SerializedBytePoints returns a serialized byte slice which contains all of
// the possible points per 8-bit window.  This is used to when generating
// secp256k1.go.
func (curve *KoblitzCurve) SerializedBytePoints() []byte {
	bitSize := curve.Params().BitSize
	byteSize := bitSize / 8
	doublingPoints := curve.getDoublingPoints()

	// Segregate the bits into byte-sized windows
	serialized := make([]byte, byteSize*256*3*10*4)
	offset := 0
	for byteNum := 0; byteNum < byteSize; byteNum++ {
		// Grab the 8 bits that make up this byte from doublingPoints.
		startingBit := 8 * (byteSize - byteNum - 1)
		computingPoints := doublingPoints[startingBit : startingBit+8]

		// Compute all points in this window and serialize them.
		for i := 0; i < 256; i++ {
			px, py, pz := new(fieldVal), new(fieldVal), new(fieldVal)
			for j := 0; j < 8; j++ {
				if i>>uint(j)&1 == 1 {
					curve.addJacobian(px, py, pz, &computingPoints[j][0],
						&computingPoints[j][1], &computingPoints[j][2], px, py, pz)
				}
			}
			for i := 0; i < 10; i++ {
				binary.LittleEndian.PutUint32(serialized[offset:], px.n[i])
				offset += 4
			}
			for i := 0; i < 10; i++ {
				binary.LittleEndian.PutUint32(serialized[offset:], py.n[i])
				offset += 4
			}
			for i := 0; i < 10; i++ {
				binary.LittleEndian.PutUint32(serialized[offset:], pz.n[i])
				offset += 4
			}
		}
	}

	return serialized
}
