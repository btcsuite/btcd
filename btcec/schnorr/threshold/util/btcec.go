package util

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
)

var (
	pubKeyAtInfinity = new(btcec.PublicKey)
)

func GetXOnlyPubKey(Q *btcec.JacobianPoint) *btcec.PublicKey {
	q := new(btcec.JacobianPoint)
	q.Set(Q)

	g := new(btcec.ModNScalar)
	g.SetInt(1)

	if q.Y.IsOdd() {
		g.Negate()
	}

	btcec.ScalarMultNonConst(g, q, q)
	q.ToAffine()

	return btcec.NewPublicKey(&q.X, &q.Y)
}

// TODO(aakselrod): Move to btcec package?
func SerializeCompressedWithInfinity(pubKey *btcec.PublicKey) []byte {
	b := pubKey.SerializeCompressed()
	if IsPubKeyAtInfinity(pubKey) {
		b[0] = 0
	}

	return b
}

func ParsePubKeyWithInfinity(b []byte) (*btcec.PublicKey, error) {
	if b[0] == 0 {
		for i := range b {
			if b[i] != 0 {
				return nil, fmt.Errorf("Identity public key must be all zeroes")
			}
		}

		return new(btcec.PublicKey), nil
	}

	return btcec.ParsePubKey(b)
}

// TODO(aakselrod): Move to btcec package?
func IsPointAtInfinity(point *btcec.JacobianPoint) bool {
	return (point.X.IsZero() && point.Y.IsZero()) || point.Z.IsZero()
}

// TODO(aakselrod): Move to btcec package?
func IsPubKeyAtInfinity(key *btcec.PublicKey) bool {
	return key.IsEqual(pubKeyAtInfinity)
}
