package silentpayments

import (
	"github.com/btcsuite/btcd/btcec/v2"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

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