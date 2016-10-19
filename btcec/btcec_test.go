// Copyright 2011 The Go Authors. All rights reserved.
// Copyright 2011 ThePiachu. All rights reserved.
// Copyright 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcec

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
)

// isJacobianOnS256Curve returns boolean if the point (x,y,z) is on the
// secp256k1 curve.
func isJacobianOnS256Curve(x, y, z *fieldVal) bool {
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

// TestAddJacobian tests addition of points projected in Jacobian coordinates.
func TestAddJacobian(t *testing.T) {
	tests := []struct {
		x1, y1, z1 string // Coordinates (in hex) of first point to add
		x2, y2, z2 string // Coordinates (in hex) of second point to add
		x3, y3, z3 string // Coordinates (in hex) of expected point
	}{
		// Addition with a point at infinity (left hand side).
		// ∞ + P = P
		{
			"0",
			"0",
			"0",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"1",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"1",
		},
		// Addition with a point at infinity (right hand side).
		// P + ∞ = P
		{
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"1",
			"0",
			"0",
			"0",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"1",
		},

		// Addition with z1=z2=1 different x values.
		{
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"1",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"1",
			"0cfbc7da1e569b334460788faae0286e68b3af7379d5504efc25e4dba16e46a6",
			"e205f79361bbe0346b037b4010985dbf4f9e1e955e7d0d14aca876bfa79aad87",
			"44a5646b446e3877a648d6d381370d9ef55a83b666ebce9df1b1d7d65b817b2f",
		},
		// Addition with z1=z2=1 same x opposite y.
		// P(x, y, z) + P(x, -y, z) = infinity
		{
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"1",
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"f48e156428cf0276dc092da5856e182288d7569f97934a56fe44be60f0d359fd",
			"1",
			"0",
			"0",
			"0",
		},
		// Addition with z1=z2=1 same point.
		// P(x, y, z) + P(x, y, z) = 2P
		{
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"1",
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"1",
			"ec9f153b13ee7bd915882859635ea9730bf0dc7611b2c7b0e37ee64f87c50c27",
			"b082b53702c466dcf6e984a35671756c506c67c2fcb8adb408c44dd0755c8f2a",
			"16e3d537ae61fb1247eda4b4f523cfbaee5152c0d0d96b520376833c1e594464",
		},

		// Addition with z1=z2 (!=1) different x values.
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"5d2fe112c21891d440f65a98473cb626111f8a234d2cd82f22172e369f002147",
			"98e3386a0a622a35c4561ffb32308d8e1c6758e10ebb1b4ebd3d04b4eb0ecbe8",
			"2",
			"cfbc7da1e569b334460788faae0286e68b3af7379d5504efc25e4dba16e46a60",
			"817de4d86ef80d1ac0ded00426176fd3e787a5579f43452b2a1db021e6ac3778",
			"129591ad11b8e1de99235b4e04dc367bd56a0ed99baf3a77c6c75f5a6e05f08d",
		},
		// Addition with z1=z2 (!=1) same x opposite y.
		// P(x, y, z) + P(x, -y, z) = infinity
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"a470ab21467813b6e0496d2c2b70c11446bab4fcbc9a52b7f225f30e869aea9f",
			"2",
			"0",
			"0",
			"0",
		},
		// Addition with z1=z2 (!=1) same point.
		// P(x, y, z) + P(x, y, z) = 2P
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"9f153b13ee7bd915882859635ea9730bf0dc7611b2c7b0e37ee65073c50fabac",
			"2b53702c466dcf6e984a35671756c506c67c2fcb8adb408c44dd125dc91cb988",
			"6e3d537ae61fb1247eda4b4f523cfbaee5152c0d0d96b520376833c2e5944a11",
		},

		// Addition with z1!=z2 and z2=1 different x values.
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"1",
			"3ef1f68795a6ccd1181e23eab80a1b9a2cebdcde755413bf097936eb5b91b4f3",
			"0bef26c377c068d606f6802130bb7e9f3c3d2abcfa1a295950ed81133561cb04",
			"252b235a2371c3bd3246b69c09b86cf7aad41db3375e74ef8d8ebeb4dc0be11a",
		},
		// Addition with z1!=z2 and z2=1 same x opposite y.
		// P(x, y, z) + P(x, -y, z) = infinity
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"f48e156428cf0276dc092da5856e182288d7569f97934a56fe44be60f0d359fd",
			"1",
			"0",
			"0",
			"0",
		},
		// Addition with z1!=z2 and z2=1 same point.
		// P(x, y, z) + P(x, y, z) = 2P
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"1",
			"9f153b13ee7bd915882859635ea9730bf0dc7611b2c7b0e37ee65073c50fabac",
			"2b53702c466dcf6e984a35671756c506c67c2fcb8adb408c44dd125dc91cb988",
			"6e3d537ae61fb1247eda4b4f523cfbaee5152c0d0d96b520376833c2e5944a11",
		},

		// Addition with z1!=z2 and z2!=1 different x values.
		// P(x, y, z) + P(x, y, z) = 2P
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"91abba6a34b7481d922a4bd6a04899d5a686f6cf6da4e66a0cb427fb25c04bd4",
			"03fede65e30b4e7576a2abefc963ddbf9fdccbf791b77c29beadefe49951f7d1",
			"3",
			"3f07081927fd3f6dadd4476614c89a09eba7f57c1c6c3b01fa2d64eac1eef31e",
			"949166e04ebc7fd95a9d77e5dfd88d1492ecffd189792e3944eb2b765e09e031",
			"eb8cba81bcffa4f44d75427506737e1f045f21e6d6f65543ee0e1d163540c931",
		}, // Addition with z1!=z2 and z2!=1 same x opposite y.
		// P(x, y, z) + P(x, -y, z) = infinity
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"dcc3768780c74a0325e2851edad0dc8a566fa61a9e7fc4a34d13dcb509f99bc7",
			"cafc41904dd5428934f7d075129c8ba46eb622d4fc88d72cd1401452664add18",
			"3",
			"0",
			"0",
			"0",
		},
		// Addition with z1!=z2 and z2!=1 same point.
		// P(x, y, z) + P(x, y, z) = 2P
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"dcc3768780c74a0325e2851edad0dc8a566fa61a9e7fc4a34d13dcb509f99bc7",
			"3503be6fb22abd76cb082f8aed63745b9149dd2b037728d32ebfebac99b51f17",
			"3",
			"9f153b13ee7bd915882859635ea9730bf0dc7611b2c7b0e37ee65073c50fabac",
			"2b53702c466dcf6e984a35671756c506c67c2fcb8adb408c44dd125dc91cb988",
			"6e3d537ae61fb1247eda4b4f523cfbaee5152c0d0d96b520376833c2e5944a11",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Convert hex to field values.
		x1 := new(fieldVal).SetHex(test.x1)
		y1 := new(fieldVal).SetHex(test.y1)
		z1 := new(fieldVal).SetHex(test.z1)
		x2 := new(fieldVal).SetHex(test.x2)
		y2 := new(fieldVal).SetHex(test.y2)
		z2 := new(fieldVal).SetHex(test.z2)
		x3 := new(fieldVal).SetHex(test.x3)
		y3 := new(fieldVal).SetHex(test.y3)
		z3 := new(fieldVal).SetHex(test.z3)

		// Ensure the test data is using points that are actually on
		// the curve (or the point at infinity).
		if !z1.IsZero() && !isJacobianOnS256Curve(x1, y1, z1) {
			t.Errorf("#%d first point is not on the curve -- "+
				"invalid test data", i)
			continue
		}
		if !z2.IsZero() && !isJacobianOnS256Curve(x2, y2, z2) {
			t.Errorf("#%d second point is not on the curve -- "+
				"invalid test data", i)
			continue
		}
		if !z3.IsZero() && !isJacobianOnS256Curve(x3, y3, z3) {
			t.Errorf("#%d expected point is not on the curve -- "+
				"invalid test data", i)
			continue
		}

		// Add the two points.
		rx, ry, rz := new(fieldVal), new(fieldVal), new(fieldVal)
		S256().addJacobian(x1, y1, z1, x2, y2, z2, rx, ry, rz)

		// Ensure result matches expected.
		if !rx.Equals(x3) || !ry.Equals(y3) || !rz.Equals(z3) {
			t.Errorf("#%d wrong result\ngot: (%v, %v, %v)\n"+
				"want: (%v, %v, %v)", i, rx, ry, rz, x3, y3, z3)
			continue
		}
	}
}

// TestAddAffine tests addition of points in affine coordinates.
func TestAddAffine(t *testing.T) {
	tests := []struct {
		x1, y1 string // Coordinates (in hex) of first point to add
		x2, y2 string // Coordinates (in hex) of second point to add
		x3, y3 string // Coordinates (in hex) of expected point
	}{
		// Addition with a point at infinity (left hand side).
		// ∞ + P = P
		{
			"0",
			"0",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
		},
		// Addition with a point at infinity (right hand side).
		// P + ∞ = P
		{
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"0",
			"0",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
		},

		// Addition with different x values.
		{
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"fd5b88c21d3143518d522cd2796f3d726793c88b3e05636bc829448e053fed69",
			"21cf4f6a5be5ff6380234c50424a970b1f7e718f5eb58f68198c108d642a137f",
		},
		// Addition with same x opposite y.
		// P(x, y) + P(x, -y) = infinity
		{
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"f48e156428cf0276dc092da5856e182288d7569f97934a56fe44be60f0d359fd",
			"0",
			"0",
		},
		// Addition with same point.
		// P(x, y) + P(x, y) = 2P
		{
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"59477d88ae64a104dbb8d31ec4ce2d91b2fe50fa628fb6a064e22582196b365b",
			"938dc8c0f13d1e75c987cb1a220501bd614b0d3dd9eb5c639847e1240216e3b6",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Convert hex to field values.
		x1, y1 := fromHex(test.x1), fromHex(test.y1)
		x2, y2 := fromHex(test.x2), fromHex(test.y2)
		x3, y3 := fromHex(test.x3), fromHex(test.y3)

		// Ensure the test data is using points that are actually on
		// the curve (or the point at infinity).
		if !(x1.Sign() == 0 && y1.Sign() == 0) && !S256().IsOnCurve(x1, y1) {
			t.Errorf("#%d first point is not on the curve -- "+
				"invalid test data", i)
			continue
		}
		if !(x2.Sign() == 0 && y2.Sign() == 0) && !S256().IsOnCurve(x2, y2) {
			t.Errorf("#%d second point is not on the curve -- "+
				"invalid test data", i)
			continue
		}
		if !(x3.Sign() == 0 && y3.Sign() == 0) && !S256().IsOnCurve(x3, y3) {
			t.Errorf("#%d expected point is not on the curve -- "+
				"invalid test data", i)
			continue
		}

		// Add the two points.
		rx, ry := S256().Add(x1, y1, x2, y2)

		// Ensure result matches expected.
		if rx.Cmp(x3) != 00 || ry.Cmp(y3) != 0 {
			t.Errorf("#%d wrong result\ngot: (%x, %x)\n"+
				"want: (%x, %x)", i, rx, ry, x3, y3)
			continue
		}
	}
}

// TestDoubleJacobian tests doubling of points projected in Jacobian
// coordinates.
func TestDoubleJacobian(t *testing.T) {
	tests := []struct {
		x1, y1, z1 string // Coordinates (in hex) of point to double
		x3, y3, z3 string // Coordinates (in hex) of expected point
	}{
		// Doubling a point at infinity is still infinity.
		{
			"0",
			"0",
			"0",
			"0",
			"0",
			"0",
		},
		// Doubling with z1=1.
		{
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"1",
			"ec9f153b13ee7bd915882859635ea9730bf0dc7611b2c7b0e37ee64f87c50c27",
			"b082b53702c466dcf6e984a35671756c506c67c2fcb8adb408c44dd0755c8f2a",
			"16e3d537ae61fb1247eda4b4f523cfbaee5152c0d0d96b520376833c1e594464",
		},
		// Doubling with z1!=1.
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"9f153b13ee7bd915882859635ea9730bf0dc7611b2c7b0e37ee65073c50fabac",
			"2b53702c466dcf6e984a35671756c506c67c2fcb8adb408c44dd125dc91cb988",
			"6e3d537ae61fb1247eda4b4f523cfbaee5152c0d0d96b520376833c2e5944a11",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Convert hex to field values.
		x1 := new(fieldVal).SetHex(test.x1)
		y1 := new(fieldVal).SetHex(test.y1)
		z1 := new(fieldVal).SetHex(test.z1)
		x3 := new(fieldVal).SetHex(test.x3)
		y3 := new(fieldVal).SetHex(test.y3)
		z3 := new(fieldVal).SetHex(test.z3)

		// Ensure the test data is using points that are actually on
		// the curve (or the point at infinity).
		if !z1.IsZero() && !isJacobianOnS256Curve(x1, y1, z1) {
			t.Errorf("#%d first point is not on the curve -- "+
				"invalid test data", i)
			continue
		}
		if !z3.IsZero() && !isJacobianOnS256Curve(x3, y3, z3) {
			t.Errorf("#%d expected point is not on the curve -- "+
				"invalid test data", i)
			continue
		}

		// Double the point.
		rx, ry, rz := new(fieldVal), new(fieldVal), new(fieldVal)
		S256().doubleJacobian(x1, y1, z1, rx, ry, rz)

		// Ensure result matches expected.
		if !rx.Equals(x3) || !ry.Equals(y3) || !rz.Equals(z3) {
			t.Errorf("#%d wrong result\ngot: (%v, %v, %v)\n"+
				"want: (%v, %v, %v)", i, rx, ry, rz, x3, y3, z3)
			continue
		}
	}
}

// TestDoubleAffine tests doubling of points in affine coordinates.
func TestDoubleAffine(t *testing.T) {
	tests := []struct {
		x1, y1 string // Coordinates (in hex) of point to double
		x3, y3 string // Coordinates (in hex) of expected point
	}{
		// Doubling a point at infinity is still infinity.
		// 2*∞ = ∞ (point at infinity)

		{
			"0",
			"0",
			"0",
			"0",
		},

		// Random points.
		{
			"e41387ffd8baaeeb43c2faa44e141b19790e8ac1f7ff43d480dc132230536f86",
			"1b88191d430f559896149c86cbcb703193105e3cf3213c0c3556399836a2b899",
			"88da47a089d333371bd798c548ef7caae76e737c1980b452d367b3cfe3082c19",
			"3b6f659b09a362821dfcfefdbfbc2e59b935ba081b6c249eb147b3c2100b1bc1",
		},
		{
			"b3589b5d984f03ef7c80aeae444f919374799edf18d375cab10489a3009cff0c",
			"c26cf343875b3630e15bccc61202815b5d8f1fd11308934a584a5babe69db36a",
			"e193860172998751e527bb12563855602a227fc1f612523394da53b746bb2fb1",
			"2bfcf13d2f5ab8bb5c611fab5ebbed3dc2f057062b39a335224c22f090c04789",
		},
		{
			"2b31a40fbebe3440d43ac28dba23eee71c62762c3fe3dbd88b4ab82dc6a82340",
			"9ba7deb02f5c010e217607fd49d58db78ec273371ea828b49891ce2fd74959a1",
			"2c8d5ef0d343b1a1a48aa336078eadda8481cb048d9305dc4fdf7ee5f65973a2",
			"bb4914ac729e26d3cd8f8dc8f702f3f4bb7e0e9c5ae43335f6e94c2de6c3dc95",
		},
		{
			"61c64b760b51981fab54716d5078ab7dffc93730b1d1823477e27c51f6904c7a",
			"ef6eb16ea1a36af69d7f66524c75a3a5e84c13be8fbc2e811e0563c5405e49bd",
			"5f0dcdd2595f5ad83318a0f9da481039e36f135005420393e72dfca985b482f4",
			"a01c849b0837065c1cb481b0932c441f49d1cab1b4b9f355c35173d93f110ae0",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Convert hex to field values.
		x1, y1 := fromHex(test.x1), fromHex(test.y1)
		x3, y3 := fromHex(test.x3), fromHex(test.y3)

		// Ensure the test data is using points that are actually on
		// the curve (or the point at infinity).
		if !(x1.Sign() == 0 && y1.Sign() == 0) && !S256().IsOnCurve(x1, y1) {
			t.Errorf("#%d first point is not on the curve -- "+
				"invalid test data", i)
			continue
		}
		if !(x3.Sign() == 0 && y3.Sign() == 0) && !S256().IsOnCurve(x3, y3) {
			t.Errorf("#%d expected point is not on the curve -- "+
				"invalid test data", i)
			continue
		}

		// Double the point.
		rx, ry := S256().Double(x1, y1)

		// Ensure result matches expected.
		if rx.Cmp(x3) != 00 || ry.Cmp(y3) != 0 {
			t.Errorf("#%d wrong result\ngot: (%x, %x)\n"+
				"want: (%x, %x)", i, rx, ry, x3, y3)
			continue
		}
	}
}

func TestOnCurve(t *testing.T) {
	s256 := S256()
	if !s256.IsOnCurve(s256.Params().Gx, s256.Params().Gy) {
		t.Errorf("FAIL S256")
	}
}

type baseMultTest struct {
	k    string
	x, y string
}

//TODO: add more test vectors
var s256BaseMultTests = []baseMultTest{
	{
		"AA5E28D6A97A2479A65527F7290311A3624D4CC0FA1578598EE3C2613BF99522",
		"34F9460F0E4F08393D192B3C5133A6BA099AA0AD9FD54EBCCFACDFA239FF49C6",
		"B71EA9BD730FD8923F6D25A7A91E7DD7728A960686CB5A901BB419E0F2CA232",
	},
	{
		"7E2B897B8CEBC6361663AD410835639826D590F393D90A9538881735256DFAE3",
		"D74BF844B0862475103D96A611CF2D898447E288D34B360BC885CB8CE7C00575",
		"131C670D414C4546B88AC3FF664611B1C38CEB1C21D76369D7A7A0969D61D97D",
	},
	{
		"6461E6DF0FE7DFD05329F41BF771B86578143D4DD1F7866FB4CA7E97C5FA945D",
		"E8AECC370AEDD953483719A116711963CE201AC3EB21D3F3257BB48668C6A72F",
		"C25CAF2F0EBA1DDB2F0F3F47866299EF907867B7D27E95B3873BF98397B24EE1",
	},
	{
		"376A3A2CDCD12581EFFF13EE4AD44C4044B8A0524C42422A7E1E181E4DEECCEC",
		"14890E61FCD4B0BD92E5B36C81372CA6FED471EF3AA60A3E415EE4FE987DABA1",
		"297B858D9F752AB42D3BCA67EE0EB6DCD1C2B7B0DBE23397E66ADC272263F982",
	},
	{
		"1B22644A7BE026548810C378D0B2994EEFA6D2B9881803CB02CEFF865287D1B9",
		"F73C65EAD01C5126F28F442D087689BFA08E12763E0CEC1D35B01751FD735ED3",
		"F449A8376906482A84ED01479BD18882B919C140D638307F0C0934BA12590BDE",
	},
}

//TODO: test different curves as well?
func TestBaseMult(t *testing.T) {
	s256 := S256()
	for i, e := range s256BaseMultTests {
		k, ok := new(big.Int).SetString(e.k, 16)
		if !ok {
			t.Errorf("%d: bad value for k: %s", i, e.k)
		}
		x, y := s256.ScalarBaseMult(k.Bytes())
		if fmt.Sprintf("%X", x) != e.x || fmt.Sprintf("%X", y) != e.y {
			t.Errorf("%d: bad output for k=%s: got (%X, %X), want (%s, %s)", i, e.k, x, y, e.x, e.y)
		}
		if testing.Short() && i > 5 {
			break
		}
	}
}

func TestBaseMultVerify(t *testing.T) {
	s256 := S256()
	for bytes := 1; bytes < 40; bytes++ {
		for i := 0; i < 30; i++ {
			data := make([]byte, bytes)
			_, err := rand.Read(data)
			if err != nil {
				t.Errorf("failed to read random data for %d", i)
				continue
			}
			x, y := s256.ScalarBaseMult(data)
			xWant, yWant := s256.ScalarMult(s256.Gx, s256.Gy, data)
			if x.Cmp(xWant) != 0 || y.Cmp(yWant) != 0 {
				t.Errorf("%d: bad output for %X: got (%X, %X), want (%X, %X)", i, data, x, y, xWant, yWant)
			}
			if testing.Short() && i > 2 {
				break
			}
		}
	}
}

func TestScalarMult(t *testing.T) {
	// Strategy for this test:
	// Get a random exponent from the generator point at first
	// This creates a new point which is used in the next iteration
	// Use another random exponent on the new point.
	// We use BaseMult to verify by multiplying the previous exponent
	// and the new random exponent together (mod N)
	s256 := S256()
	x, y := s256.Gx, s256.Gy
	exponent := big.NewInt(1)
	for i := 0; i < 1024; i++ {
		data := make([]byte, 32)
		_, err := rand.Read(data)
		if err != nil {
			t.Fatalf("failed to read random data at %d", i)
			break
		}
		x, y = s256.ScalarMult(x, y, data)
		exponent.Mul(exponent, new(big.Int).SetBytes(data))
		xWant, yWant := s256.ScalarBaseMult(exponent.Bytes())
		if x.Cmp(xWant) != 0 || y.Cmp(yWant) != 0 {
			t.Fatalf("%d: bad output for %X: got (%X, %X), want (%X, %X)", i, data, x, y, xWant, yWant)
			break
		}
	}
}

// Test this curve's usage with the ecdsa package.

func testKeyGeneration(t *testing.T, c *KoblitzCurve, tag string) {
	priv, err := NewPrivateKey(c)
	if err != nil {
		t.Errorf("%s: error: %s", tag, err)
		return
	}
	if !c.IsOnCurve(priv.PublicKey.X, priv.PublicKey.Y) {
		t.Errorf("%s: public key invalid: %s", tag, err)
	}
}

func TestKeyGeneration(t *testing.T) {
	testKeyGeneration(t, S256(), "S256")
}

func testSignAndVerify(t *testing.T, c *KoblitzCurve, tag string) {
	priv, _ := NewPrivateKey(c)
	pub := priv.PubKey()

	hashed := []byte("testing")
	sig, err := priv.Sign(hashed)
	if err != nil {
		t.Errorf("%s: error signing: %s", tag, err)
		return
	}

	if !sig.Verify(hashed, pub) {
		t.Errorf("%s: Verify failed", tag)
	}

	hashed[0] ^= 0xff
	if sig.Verify(hashed, pub) {
		t.Errorf("%s: Verify always works!", tag)
	}
}

func TestSignAndVerify(t *testing.T) {
	testSignAndVerify(t, S256(), "S256")
}

func TestNAF(t *testing.T) {
	negOne := big.NewInt(-1)
	one := big.NewInt(1)
	two := big.NewInt(2)
	for i := 0; i < 1024; i++ {
		data := make([]byte, 32)
		_, err := rand.Read(data)
		if err != nil {
			t.Fatalf("failed to read random data at %d", i)
			break
		}
		nafPos, nafNeg := NAF(data)
		want := new(big.Int).SetBytes(data)
		got := big.NewInt(0)
		// Check that the NAF representation comes up with the right number
		for i := 0; i < len(nafPos); i++ {
			bytePos := nafPos[i]
			byteNeg := nafNeg[i]
			for j := 7; j >= 0; j-- {
				got.Mul(got, two)
				if bytePos&0x80 == 0x80 {
					got.Add(got, one)
				} else if byteNeg&0x80 == 0x80 {
					got.Add(got, negOne)
				}
				bytePos <<= 1
				byteNeg <<= 1
			}
		}
		if got.Cmp(want) != 0 {
			t.Errorf("%d: Failed NAF got %X want %X", i, got, want)
		}
	}
}

// These test vectors were taken from
//   http://csrc.nist.gov/groups/STM/cavp/documents/dss/ecdsatestvectors.zip
var testVectors = []struct {
	msg    string
	Qx, Qy string
	r, s   string
	ok     bool
}{
/*
 * All of these tests are disabled since they are for P224, not sec256k1.
 * they are left here as an example of test vectors for when some *real*
 * vectors may be found.
 * - oga@conformal.com
	{
		"09626b45493672e48f3d1226a3aff3201960e577d33a7f72c7eb055302db8fe8ed61685dd036b554942a5737cd1512cdf811ee0c00e6dd2f08c69f08643be396e85dafda664801e772cdb7396868ac47b172245b41986aa2648cb77fbbfa562581be06651355a0c4b090f9d17d8f0ab6cced4e0c9d386cf465a516630f0231bd",
		"9504b5b82d97a264d8b3735e0568decabc4b6ca275bc53cbadfc1c40",
		"03426f80e477603b10dee670939623e3da91a94267fc4e51726009ed",
		"81d3ac609f9575d742028dd496450a58a60eea2dcf8b9842994916e1",
		"96a8c5f382c992e8f30ccce9af120b067ec1d74678fa8445232f75a5",
		false,
	},
	{
		"96b2b6536f6df29be8567a72528aceeaccbaa66c66c534f3868ca9778b02faadb182e4ed34662e73b9d52ecbe9dc8e875fc05033c493108b380689ebf47e5b062e6a0cdb3dd34ce5fe347d92768d72f7b9b377c20aea927043b509c078ed2467d7113405d2ddd458811e6faf41c403a2a239240180f1430a6f4330df5d77de37",
		"851e3100368a22478a0029353045ae40d1d8202ef4d6533cfdddafd8",
		"205302ac69457dd345e86465afa72ee8c74ca97e2b0b999aec1f10c2",
		"4450c2d38b697e990721aa2dbb56578d32b4f5aeb3b9072baa955ee0",
		"e26d4b589166f7b4ba4b1c8fce823fa47aad22f8c9c396b8c6526e12",
		false,
	},
	{
		"86778dbb4a068a01047a8d245d632f636c11d2ad350740b36fad90428b454ad0f120cb558d12ea5c8a23db595d87543d06d1ef489263d01ee529871eb68737efdb8ff85bc7787b61514bed85b7e01d6be209e0a4eb0db5c8df58a5c5bf706d76cb2bdf7800208639e05b89517155d11688236e6a47ed37d8e5a2b1e0adea338e",
		"ad5bda09d319a717c1721acd6688d17020b31b47eef1edea57ceeffc",
		"c8ce98e181770a7c9418c73c63d01494b8b80a41098c5ea50692c984",
		"de5558c257ab4134e52c19d8db3b224a1899cbd08cc508ce8721d5e9",
		"745db7af5a477e5046705c0a5eff1f52cb94a79d481f0c5a5e108ecd",
		true,
	},
	{
		"4bc6ef1958556686dab1e39c3700054a304cbd8f5928603dcd97fafd1f29e69394679b638f71c9344ce6a535d104803d22119f57b5f9477e253817a52afa9bfbc9811d6cc8c8be6b6566c6ef48b439bbb532abe30627548c598867f3861ba0b154dc1c3deca06eb28df8efd28258554b5179883a36fbb1eecf4f93ee19d41e3d",
		"cc5eea2edf964018bdc0504a3793e4d2145142caa09a72ac5fb8d3e8",
		"a48d78ae5d08aa725342773975a00d4219cf7a8029bb8cf3c17c374a",
		"67b861344b4e416d4094472faf4272f6d54a497177fbc5f9ef292836",
		"1d54f3fcdad795bf3b23408ecbac3e1321d1d66f2e4e3d05f41f7020",
		false,
	},
	{
		"bb658732acbf3147729959eb7318a2058308b2739ec58907dd5b11cfa3ecf69a1752b7b7d806fe00ec402d18f96039f0b78dbb90a59c4414fb33f1f4e02e4089de4122cd93df5263a95be4d7084e2126493892816e6a5b4ed123cb705bf930c8f67af0fb4514d5769232a9b008a803af225160ce63f675bd4872c4c97b146e5e",
		"6234c936e27bf141fc7534bfc0a7eedc657f91308203f1dcbd642855",
		"27983d87ca785ef4892c3591ef4a944b1deb125dd58bd351034a6f84",
		"e94e05b42d01d0b965ffdd6c3a97a36a771e8ea71003de76c4ecb13f",
		"1dc6464ffeefbd7872a081a5926e9fc3e66d123f1784340ba17737e9",
		false,
	},
	{
		"7c00be9123bfa2c4290be1d8bc2942c7f897d9a5b7917e3aabd97ef1aab890f148400a89abd554d19bec9d8ed911ce57b22fbcf6d30ca2115f13ce0a3f569a23bad39ee645f624c49c60dcfc11e7d2be24de9c905596d8f23624d63dc46591d1f740e46f982bfae453f107e80db23545782be23ce43708245896fc54e1ee5c43",
		"9f3f037282aaf14d4772edffff331bbdda845c3f65780498cde334f1",
		"8308ee5a16e3bcb721b6bc30000a0419bc1aaedd761be7f658334066",
		"6381d7804a8808e3c17901e4d283b89449096a8fba993388fa11dc54",
		"8e858f6b5b253686a86b757bad23658cda53115ac565abca4e3d9f57",
		false,
	},
	{
		"cffc122a44840dc705bb37130069921be313d8bde0b66201aebc48add028ca131914ef2e705d6bedd19dc6cf9459bbb0f27cdfe3c50483808ffcdaffbeaa5f062e097180f07a40ef4ab6ed03fe07ed6bcfb8afeb42c97eafa2e8a8df469de07317c5e1494c41547478eff4d8c7d9f0f484ad90fedf6e1c35ee68fa73f1691601",
		"a03b88a10d930002c7b17ca6af2fd3e88fa000edf787dc594f8d4fd4",
		"e0cf7acd6ddc758e64847fe4df9915ebda2f67cdd5ec979aa57421f5",
		"387b84dcf37dc343c7d2c5beb82f0bf8bd894b395a7b894565d296c1",
		"4adc12ce7d20a89ce3925e10491c731b15ddb3f339610857a21b53b4",
		false,
	},
	{
		"26e0e0cafd85b43d16255908ccfd1f061c680df75aba3081246b337495783052ba06c60f4a486c1591a4048bae11b4d7fec4f161d80bdc9a7b79d23e44433ed625eab280521a37f23dd3e1bdc5c6a6cfaa026f3c45cf703e76dab57add93fe844dd4cda67dc3bddd01f9152579e49df60969b10f09ce9372fdd806b0c7301866",
		"9a8983c42f2b5a87c37a00458b5970320d247f0c8a88536440173f7d",
		"15e489ec6355351361900299088cfe8359f04fe0cab78dde952be80c",
		"929a21baa173d438ec9f28d6a585a2f9abcfc0a4300898668e476dc0",
		"59a853f046da8318de77ff43f26fe95a92ee296fa3f7e56ce086c872",
		true,
	},
	{
		"1078eac124f48ae4f807e946971d0de3db3748dd349b14cca5c942560fb25401b2252744f18ad5e455d2d97ed5ae745f55ff509c6c8e64606afe17809affa855c4c4cdcaf6b69ab4846aa5624ed0687541aee6f2224d929685736c6a23906d974d3c257abce1a3fb8db5951b89ecb0cda92b5207d93f6618fd0f893c32cf6a6e",
		"d6e55820bb62c2be97650302d59d667a411956138306bd566e5c3c2b",
		"631ab0d64eaf28a71b9cbd27a7a88682a2167cee6251c44e3810894f",
		"65af72bc7721eb71c2298a0eb4eed3cec96a737cc49125706308b129",
		"bd5a987c78e2d51598dbd9c34a9035b0069c580edefdacee17ad892a",
		false,
	},
	{
		"919deb1fdd831c23481dfdb2475dcbe325b04c34f82561ced3d2df0b3d749b36e255c4928973769d46de8b95f162b53cd666cad9ae145e7fcfba97919f703d864efc11eac5f260a5d920d780c52899e5d76f8fe66936ff82130761231f536e6a3d59792f784902c469aa897aabf9a0678f93446610d56d5e0981e4c8a563556b",
		"269b455b1024eb92d860a420f143ac1286b8cce43031562ae7664574",
		"baeb6ca274a77c44a0247e5eb12ca72bdd9a698b3f3ae69c9f1aaa57",
		"cb4ec2160f04613eb0dfe4608486091a25eb12aa4dec1afe91cfb008",
		"40b01d8cd06589481574f958b98ca08ade9d2a8fe31024375c01bb40",
		false,
	},
	{
		"6e012361250dacf6166d2dd1aa7be544c3206a9d43464b3fcd90f3f8cf48d08ec099b59ba6fe7d9bdcfaf244120aed1695d8be32d1b1cd6f143982ab945d635fb48a7c76831c0460851a3d62b7209c30cd9c2abdbe3d2a5282a9fcde1a6f418dd23c409bc351896b9b34d7d3a1a63bbaf3d677e612d4a80fa14829386a64b33f",
		"6d2d695efc6b43b13c14111f2109608f1020e3e03b5e21cfdbc82fcd",
		"26a4859296b7e360b69cf40be7bd97ceaffa3d07743c8489fc47ca1b",
		"9a8cb5f2fdc288b7183c5b32d8e546fc2ed1ca4285eeae00c8b572ad",
		"8c623f357b5d0057b10cdb1a1593dab57cda7bdec9cf868157a79b97",
		true,
	},
	{
		"bf6bd7356a52b234fe24d25557200971fc803836f6fec3cade9642b13a8e7af10ab48b749de76aada9d8927f9b12f75a2c383ca7358e2566c4bb4f156fce1fd4e87ef8c8d2b6b1bdd351460feb22cdca0437ac10ca5e0abbbce9834483af20e4835386f8b1c96daaa41554ceee56730aac04f23a5c765812efa746051f396566",
		"14250131b2599939cf2d6bc491be80ddfe7ad9de644387ee67de2d40",
		"b5dc473b5d014cd504022043c475d3f93c319a8bdcb7262d9e741803",
		"4f21642f2201278a95339a80f75cc91f8321fcb3c9462562f6cbf145",
		"452a5f816ea1f75dee4fd514fa91a0d6a43622981966c59a1b371ff8",
		false,
	},
	{
		"0eb7f4032f90f0bd3cf9473d6d9525d264d14c031a10acd31a053443ed5fe919d5ac35e0be77813071b4062f0b5fdf58ad5f637b76b0b305aec18f82441b6e607b44cdf6e0e3c7c57f24e6fd565e39430af4a6b1d979821ed0175fa03e3125506847654d7e1ae904ce1190ae38dc5919e257bdac2db142a6e7cd4da6c2e83770",
		"d1f342b7790a1667370a1840255ac5bbbdc66f0bc00ae977d99260ac",
		"76416cabae2de9a1000b4646338b774baabfa3db4673790771220cdb",
		"bc85e3fc143d19a7271b2f9e1c04b86146073f3fab4dda1c3b1f35ca",
		"9a5c70ede3c48d5f43307a0c2a4871934424a3303b815df4bb0f128e",
		false,
	},
	{
		"5cc25348a05d85e56d4b03cec450128727bc537c66ec3a9fb613c151033b5e86878632249cba83adcefc6c1e35dcd31702929c3b57871cda5c18d1cf8f9650a25b917efaed56032e43b6fc398509f0d2997306d8f26675f3a8683b79ce17128e006aa0903b39eeb2f1001be65de0520115e6f919de902b32c38d691a69c58c92",
		"7e49a7abf16a792e4c7bbc4d251820a2abd22d9f2fc252a7bf59c9a6",
		"44236a8fb4791c228c26637c28ae59503a2f450d4cfb0dc42aa843b9",
		"084461b4050285a1a85b2113be76a17878d849e6bc489f4d84f15cd8",
		"079b5bddcc4d45de8dbdfd39f69817c7e5afa454a894d03ee1eaaac3",
		false,
	},
	{
		"1951533ce33afb58935e39e363d8497a8dd0442018fd96dff167b3b23d7206a3ee182a3194765df4768a3284e23b8696c199b4686e670d60c9d782f08794a4bccc05cffffbd1a12acd9eb1cfa01f7ebe124da66ecff4599ea7720c3be4bb7285daa1a86ebf53b042bd23208d468c1b3aa87381f8e1ad63e2b4c2ba5efcf05845",
		"31945d12ebaf4d81f02be2b1768ed80784bf35cf5e2ff53438c11493",
		"a62bebffac987e3b9d3ec451eb64c462cdf7b4aa0b1bbb131ceaa0a4",
		"bc3c32b19e42b710bca5c6aaa128564da3ddb2726b25f33603d2af3c",
		"ed1a719cc0c507edc5239d76fe50e2306c145ad252bd481da04180c0",
		false,
	},
*/
}

func TestVectors(t *testing.T) {
	sha := sha1.New()

	for i, test := range testVectors {
		pub := PublicKey{
			Curve: S256(),
			X:     fromHex(test.Qx),
			Y:     fromHex(test.Qy),
		}
		msg, _ := hex.DecodeString(test.msg)
		sha.Reset()
		sha.Write(msg)
		hashed := sha.Sum(nil)
		sig := Signature{R: fromHex(test.r), S: fromHex(test.s)}
		if verified := sig.Verify(hashed, &pub); verified != test.ok {
			t.Errorf("%d: bad result %v instead of %v", i, verified,
				test.ok)
		}
		if testing.Short() {
			break
		}
	}
}
