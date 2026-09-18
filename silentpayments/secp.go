package silentpayments

import (
	"github.com/btcsuite/btcd/btcec/v2"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// Generator returns the generator point of the secp256k1 curve.
func Generator() *btcec.PublicKey {
	one := new(secp.ModNScalar).SetInt(1)
	return ScalarBaseMult(*one)
}

// ScalarBaseMult returns the public key resulting from the scalar
// multiplication of the generator point with the given private key.
func ScalarBaseMult(a secp.ModNScalar) *btcec.PublicKey {
	var resultJ btcec.JacobianPoint
	btcec.ScalarBaseMultNonConst(&a, &resultJ)
	resultJ.ToAffine()

	return btcec.NewPublicKey(&resultJ.X, &resultJ.Y)
}

// ScalarBaseMultAdd returns the public key resulting from the scalar
// multiplication of the base point with the given private key, then adding
// the given public key.
func ScalarBaseMultAdd(a secp.ModNScalar, b *btcec.PublicKey) *btcec.PublicKey {
	var resultJ, bJ btcec.JacobianPoint
	b.AsJacobian(&bJ)

	btcec.ScalarBaseMultNonConst(&a, &resultJ)
	btcec.AddNonConst(&bJ, &resultJ, &resultJ)

	resultJ.ToAffine()

	return btcec.NewPublicKey(&resultJ.X, &resultJ.Y)
}

// ScalarMult returns the public key resulting from the scalar multiplication
// of the given scalar with the given public key.
func ScalarMult(a secp.ModNScalar, pub *btcec.PublicKey) *btcec.PublicKey {
	var resultJ btcec.JacobianPoint
	pub.AsJacobian(&resultJ)

	btcec.ScalarMultNonConst(&a, &resultJ, &resultJ)
	resultJ.ToAffine()

	return btcec.NewPublicKey(&resultJ.X, &resultJ.Y)
}

// Negate returns the public key resulting from the negation of the given public
// key.
func Negate(pub *btcec.PublicKey) *btcec.PublicKey {
	var resultJ btcec.JacobianPoint
	pub.AsJacobian(&resultJ)

	resultJ.Y.Negate(1)
	resultJ.Y.Normalize()
	resultJ.ToAffine()

	return btcec.NewPublicKey(&resultJ.X, &resultJ.Y)
}

// Add returns the public key resulting from the addition of the two given
// public keys.
func Add(a, b *btcec.PublicKey) *btcec.PublicKey {
	var aJ, bJ btcec.JacobianPoint
	a.AsJacobian(&aJ)
	b.AsJacobian(&bJ)

	btcec.AddNonConst(&aJ, &bJ, &aJ)
	aJ.ToAffine()

	return btcec.NewPublicKey(&aJ.X, &aJ.Y)
}
