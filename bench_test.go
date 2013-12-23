// Copyright 2013 Conformal Systems LLC. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package btcec_test

import (
	"github.com/conformal/btcec"
	"testing"
)

// BechmarkScalarBaseMult benchmarks the secp256k1 curve ScalarBaseMult
// function.
func BechmarkScalarBaseMult(b *testing.B) {
	k := fromHex("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")
	curve := btcec.S256()
	for i := 0; i < b.N; i++ {
		curve.ScalarBaseMult(k.Bytes())
	}
}

// BenchmarkScalarMult benchmarks the secp256k1 curve ScalarMult function.
func BenchmarkScalarMult(b *testing.B) {
	x := fromHex("34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6")
	y := fromHex("0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232")
	k := fromHex("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")
	curve := btcec.S256()
	for i := 0; i < b.N; i++ {
		curve.ScalarMult(x, y, k.Bytes())
	}
}
