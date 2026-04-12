package util

import (
	"fmt"
	"math"
	"slices"

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
func SumPoints(points []*btcec.JacobianPoint) *btcec.JacobianPoint {
	var point = new(btcec.JacobianPoint)

	for _, addPoint := range points {
		btcec.AddNonConst(point, addPoint, point)
	}

	point.ToAffine()

	return point
}

// TODO(aakselrod): Move to btcec package?
func SumScalars(scalars []*btcec.ModNScalar) *btcec.ModNScalar {
	var scalar = new(btcec.ModNScalar)

	for _, addScalar := range scalars {
		scalar.Add(addScalar)
	}

	return scalar
}

// TODO(aakselrod): Move to btcec package?
func IsPointAtInfinity(point *btcec.JacobianPoint) bool {
	return (point.X.IsZero() && point.Y.IsZero()) || point.Z.IsZero()
}

// TODO(aakselrod): Move to btcec package?
func IsPubKeyAtInfinity(key *btcec.PublicKey) bool {
	return key.IsEqual(pubKeyAtInfinity)
}

func DeriveInterpolatingValue(ids []int, id int) (*btcec.ModNScalar,
	error) {

	if !slices.Contains(ids, id) {
		return nil, fmt.Errorf("id not in list of ids")
	}
	if id < 0 || id > math.MaxUint32 {
		return nil, fmt.Errorf("id out of range")
	}
	mapIds := make(map[int]struct{})
	for i := range ids {
		mapIds[ids[i]] = struct{}{}
	}
	if len(mapIds) != len(ids) {
		return nil, fmt.Errorf("duplicate ids in slice")
	}

	num := new(btcec.ModNScalar)
	deno := new(btcec.ModNScalar)
	idNeg := new(btcec.ModNScalar)
	num.SetInt(uint32(1))
	deno.SetInt(uint32(1))
	idNeg.SetInt(uint32(id))
	idNeg.Negate()

	for i := range ids {
		if ids[i] == id {
			continue
		}

		numMul := new(btcec.ModNScalar)
		denoMul := new(btcec.ModNScalar)
		numMul.SetInt(uint32(ids[i] + 1))
		denoMul.SetInt(uint32(ids[i]))
		denoMul.Add(idNeg)

		num.Mul(numMul)
		deno.Mul(denoMul)
	}

	deno.InverseNonConst()
	num.Mul(deno)
	return num, nil
}

func ParsePrivKeyNonZeroChecked(b []byte) (*btcec.PrivateKey, error) {
	scalar := new(btcec.ModNScalar)

	if scalar.SetByteSlice(b) || scalar.IsZero() {
		return nil, fmt.Errorf("private key out of range")
	}

	defer scalar.Zero()

	return btcec.PrivKeyFromScalar(scalar), nil
}
