// Copyright 2013-2014 Conformal Systems LLC. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package btcec_test

import (
	"crypto/ecdsa"
	"github.com/conformal/btcec"
	"testing"
)

// BenchmarkAddJacobian benchmarks the secp256k1 curve addJacobian function.
func BenchmarkAddJacobian(b *testing.B) {
	b.StopTimer()
	x1 := btcec.NewFieldVal().SetHex("34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6")
	y1 := btcec.NewFieldVal().SetHex("0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232")
	z1 := btcec.NewFieldVal().SetHex("1")
	x2 := btcec.NewFieldVal().SetHex("34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6")
	y2 := btcec.NewFieldVal().SetHex("0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232")
	z2 := btcec.NewFieldVal().SetHex("1")
	x3, y3, z3 := btcec.NewFieldVal(), btcec.NewFieldVal(), btcec.NewFieldVal()
	curve := btcec.S256()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		curve.TstAddJacobian(x1, y1, z1, x2, y2, z2, x3, y3, z3)
	}
}

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

// BenchmarkSigVerify benchmarks how long it takes the secp256k1 curve to
// verify signatures.
func BenchmarkSigVerify(b *testing.B) {
	b.StopTimer()
	// Randomly generated keypair.
	// Private key: 9e0699c91ca1e3b7e3c9ba71eb71c89890872be97576010fe593fbf3fd57e66d
	pubKey := ecdsa.PublicKey{
		Curve: btcec.S256(),
		X:     fromHex("d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab"),
		Y:     fromHex("ab65528eefbb8057aa85d597258a3fbd481a24633bc9b47a9aa045c91371de52"),
	}

	// Double sha256 of []byte{0x01, 0x02, 0x03, 0x04}
	msgHash := fromHex("8de472e2399610baaa7f84840547cd409434e31f5d3bd71e4d947f283874f9c0")
	sigR := fromHex("fef45d2892953aa5bbcdb057b5e98b208f1617a7498af7eb765574e29b5d9c2c")
	sigS := fromHex("d47563f52aac6b04b55de236b7c515eb9311757db01e02cff079c3ca6efb063f")

	if !ecdsa.Verify(&pubKey, msgHash.Bytes(), sigR, sigS) {
		b.Errorf("Signature failed to verify")
		return
	}
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		ecdsa.Verify(&pubKey, msgHash.Bytes(), sigR, sigS)
	}
}
