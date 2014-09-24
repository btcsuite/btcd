// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
// +build gensecp256k1

package btcec

import (
	"fmt"
)

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

// PrintBytePoints prints all the possible points per 8-bit window.
// normally, this is used to generate secp256k1.go
func (curve *KoblitzCurve) PrintBytePoints() {
	bitSize := curve.Params().BitSize
	byteSize := bitSize / 8
	doublingPoints := curve.getDoublingPoints()
	fmt.Println("// Copyright (c) 2014 Conformal Systems LLC.")
	fmt.Println("// Use of this source code is governed by an ISC")
	fmt.Println("// license that can be found in the LICENSE file.")
	fmt.Println()
	fmt.Println("package btcec")
	fmt.Println()
	fmt.Println("// Auto-generated file (see gensecp256k1.go)")
	fmt.Printf("var secp256k1BytePoints = [%d][256][3]fieldVal{\n", byteSize)
	// Segregate the bits into byte-sized windows
	for byteNum := 0; byteNum < byteSize; byteNum++ {
		fmt.Printf("\t{\n")
		// grab the 8 bits that make up this byte from doublingPoints
		startingBit := 8 * (byteSize - byteNum - 1)
		computingPoints := doublingPoints[startingBit : startingBit+8]
		// compute all points in this window
		for i := 0; i < 256; i++ {
			px, py, pz := new(fieldVal), new(fieldVal), new(fieldVal)
			for j := 0; j < 8; j++ {
				if i>>uint(j)&1 == 1 {
					curve.addJacobian(px, py, pz, &computingPoints[j][0],
						&computingPoints[j][1], &computingPoints[j][2], px, py, pz)
				}
			}
			fmt.Printf("\t\t{\n\t\t\tfieldVal{[10]uint32{%d, %d, %d, %d, %d, %d, %d, %d, %d, %d}},\n", px.n[0], px.n[1], px.n[2], px.n[3], px.n[4], px.n[5], px.n[6], px.n[7], px.n[8], px.n[9])
			fmt.Printf("\t\t\tfieldVal{[10]uint32{%d, %d, %d, %d, %d, %d, %d, %d, %d, %d}},\n", py.n[0], py.n[1], py.n[2], py.n[3], py.n[4], py.n[5], py.n[6], py.n[7], py.n[8], py.n[9])
			fmt.Printf("\t\t\tfieldVal{[10]uint32{%d, %d, %d, %d, %d, %d, %d, %d, %d, %d}},\n\t\t},\n", pz.n[0], pz.n[1], pz.n[2], pz.n[3], pz.n[4], pz.n[5], pz.n[6], pz.n[7], pz.n[8], pz.n[9])
		}
		fmt.Printf("\t},\n")
	}
	fmt.Printf("}\n")
}
