// Copyright 2013-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import "testing"

// BenchmarkAddJacobian benchmarks the secp256k1 curve addJacobian function with
// Z values of 1 so that the associated optimizations are used.
func BenchmarkAddJacobian(b *testing.B) {
	b.StopTimer()
	x1 := new(fieldVal).SetHex("34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6")
	y1 := new(fieldVal).SetHex("0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232")
	z1 := new(fieldVal).SetHex("1")
	x2 := new(fieldVal).SetHex("34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6")
	y2 := new(fieldVal).SetHex("0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232")
	z2 := new(fieldVal).SetHex("1")
	x3, y3, z3 := new(fieldVal), new(fieldVal), new(fieldVal)
	curve := S256()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		curve.addJacobian(x1, y1, z1, x2, y2, z2, x3, y3, z3)
	}
}

// BenchmarkAddJacobianNotZOne benchmarks the secp256k1 curve addJacobian
// function with Z values other than one so the optimizations associated with
// Z=1 aren't used.
func BenchmarkAddJacobianNotZOne(b *testing.B) {
	b.StopTimer()
	x1 := new(fieldVal).SetHex("d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718")
	y1 := new(fieldVal).SetHex("5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190")
	z1 := new(fieldVal).SetHex("2")
	x2 := new(fieldVal).SetHex("91abba6a34b7481d922a4bd6a04899d5a686f6cf6da4e66a0cb427fb25c04bd4")
	y2 := new(fieldVal).SetHex("03fede65e30b4e7576a2abefc963ddbf9fdccbf791b77c29beadefe49951f7d1")
	z2 := new(fieldVal).SetHex("3")
	x3, y3, z3 := new(fieldVal), new(fieldVal), new(fieldVal)
	curve := S256()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		curve.addJacobian(x1, y1, z1, x2, y2, z2, x3, y3, z3)
	}
}

// BenchmarkScalarBaseMult benchmarks the secp256k1 curve ScalarBaseMult
// function.
func BenchmarkScalarBaseMult(b *testing.B) {
	k := fromHex("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")
	curve := S256()
	for i := 0; i < b.N; i++ {
		curve.ScalarBaseMult(k.Bytes())
	}
}

// BenchmarkScalarBaseMultLarge benchmarks the secp256k1 curve ScalarBaseMult
// function with abnormally large k values.
func BenchmarkScalarBaseMultLarge(b *testing.B) {
	k := fromHex("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c005751111111011111110")
	curve := S256()
	for i := 0; i < b.N; i++ {
		curve.ScalarBaseMult(k.Bytes())
	}
}

// BenchmarkScalarMult benchmarks the secp256k1 curve ScalarMult function.
func BenchmarkScalarMult(b *testing.B) {
	x := fromHex("34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6")
	y := fromHex("0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232")
	k := fromHex("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")
	curve := S256()
	for i := 0; i < b.N; i++ {
		curve.ScalarMult(x, y, k.Bytes())
	}
}

// BenchmarkNAF benchmarks the NAF function.
func BenchmarkNAF(b *testing.B) {
	k := fromHex("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")
	for i := 0; i < b.N; i++ {
		NAF(k.Bytes())
	}
}

// BenchmarkSigVerify benchmarks how long it takes the secp256k1 curve to
// verify signatures.
func BenchmarkSigVerify(b *testing.B) {
	b.StopTimer()
	// Randomly generated keypair.
	// Private key: 9e0699c91ca1e3b7e3c9ba71eb71c89890872be97576010fe593fbf3fd57e66d
	pubKey := PublicKey{
		Curve: S256(),
		X:     fromHex("d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab"),
		Y:     fromHex("ab65528eefbb8057aa85d597258a3fbd481a24633bc9b47a9aa045c91371de52"),
	}

	// Double sha256 of []byte{0x01, 0x02, 0x03, 0x04}
	msgHash := fromHex("8de472e2399610baaa7f84840547cd409434e31f5d3bd71e4d947f283874f9c0")
	sig := Signature{
		R: fromHex("fef45d2892953aa5bbcdb057b5e98b208f1617a7498af7eb765574e29b5d9c2c"),
		S: fromHex("d47563f52aac6b04b55de236b7c515eb9311757db01e02cff079c3ca6efb063f"),
	}

	if !sig.Verify(msgHash.Bytes(), &pubKey) {
		b.Errorf("Signature failed to verify")
		return
	}
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		sig.Verify(msgHash.Bytes(), &pubKey)
	}
}

// BenchmarkFieldNormalize benchmarks how long it takes the internal field
// to perform normalization (which includes modular reduction).
func BenchmarkFieldNormalize(b *testing.B) {
	// The normalize function is constant time so default value is fine.
	f := new(fieldVal)
	for i := 0; i < b.N; i++ {
		f.Normalize()
	}
}
