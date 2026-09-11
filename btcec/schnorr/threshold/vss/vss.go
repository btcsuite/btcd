// Adapted from https://github.com/BlockstreamResearch/bip-frost-dkg v0.2.0

package vss

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/util"
	"github.com/btcsuite/btcd/chainhash/v2"
)

type Polynomial []*btcec.ModNScalar

func NewPolynomial(coefficients []*btcec.ModNScalar) *Polynomial {
	// TODO(aakselrod): do we really need to copy here?
	coeffs := make([]*btcec.ModNScalar, 0, len(coefficients))
	for _, c := range coefficients {
		scalar := new(btcec.ModNScalar).Set(c)
		coeffs = append(coeffs, scalar)
	}

	p := Polynomial(coeffs)
	return &p
}

func (p *Polynomial) Evaluate(x *btcec.ModNScalar) *btcec.ModNScalar {
	var value btcec.ModNScalar

	for i := len(*p) - 1; i >= 0; i-- {
		value.Mul(x).Add((*p)[i])
	}

	return &value
}

type VSSCommitment []*btcec.JacobianPoint

func NewVSSCommitment(points []*btcec.JacobianPoint) *VSSCommitment {
	// TODO(aakselrod): do we really need to copy here?
	ges := make([]*btcec.JacobianPoint, 0, len(points))
	for _, point := range points {
		ge := new(btcec.JacobianPoint)
		ge.Set(point)
		ges = append(ges, ge)
	}

	c := VSSCommitment(ges)
	return &c
}

func (c *VSSCommitment) T() int {
	return len(*c)
}

func (c *VSSCommitment) PubShare(i int) *btcec.JacobianPoint {
	var (
		p, r btcec.JacobianPoint
		ai   = new(big.Int)
		as   = new(btcec.ModNScalar)
		n    = btcec.Params().N
	)

	for idx, point := range *c {
		// TODO(aakselrod): should this be constant-time?
		ai.Exp(
			big.NewInt(int64(i+1)),
			big.NewInt(int64(idx)),
			n,
		)
		aiBytes := ai.Bytes()
		as.SetByteSlice(aiBytes)
		btcec.ScalarMultNonConst(as, point, &p)
		btcec.AddNonConst(&r, &p, &r)
	}

	(&r).ToAffine()
	return &r
}

func VerifySecShare(secShare *btcec.ModNScalar,
	pubShare *btcec.JacobianPoint) bool {

	var result = new(btcec.JacobianPoint)
	btcec.ScalarBaseMultNonConst(secShare, result)
	result.ToAffine()

	return result.X.Equals(&pubShare.X) && result.Y.Equals(&pubShare.Y) &&
		result.Z.Equals(&pubShare.Z)
}

func (c *VSSCommitment) Bytes() []byte {
	var result = make([]byte, 0, len(*c)*33)

	for _, point := range *c {
		result = append(result, btcec.JacobianToByteSlice(*point)...)
	}

	return result
}

func (c *VSSCommitment) Add(commitment *VSSCommitment) (*VSSCommitment, error) {
	if len(*c) != len(*commitment) {
		return nil, errors.New("commitments must be equal length")
	}

	var result VSSCommitment

	for i := range *c {
		sum := new(btcec.JacobianPoint)
		btcec.AddNonConst((*c)[i], (*commitment)[i], sum)
		sum.ToAffine()
		result = append(result, sum)
	}

	return &result, nil
}

func ParseVSSCommitment(b []byte, t int) (*VSSCommitment, error) {
	if len(b) != t*33 {
		return nil, fmt.Errorf("invalid length of bytes")
	}

	var result VSSCommitment

	for i := range t {
		point, err := btcec.ParseJacobian(b[i*33 : i*33+33])
		if err != nil {
			return nil, err
		}

		result = append(result, &point)
	}

	return &result, nil
}

func (c *VSSCommitment) CommitmentToSecret() *btcec.JacobianPoint {
	return (*c)[0]
}

func (c *VSSCommitment) CommitmentToNonConstTerms() []*btcec.JacobianPoint {
	return (*c)[1:]
}

// TODO(aakselrod): maybe return tweak as privkey/pubkey?
func (c *VSSCommitment) InvalidTaprootCommit() (*VSSCommitment,
	*btcec.ModNScalar, *btcec.JacobianPoint, error) {

	var (
		pk                = c.CommitmentToSecret()
		secshareTweak     = new(btcec.ModNScalar)
		pubshareTweak     = new(btcec.JacobianPoint)
		vssTweak          = make([]*btcec.JacobianPoint, 0, c.T())
		tweakedCommitment *VSSCommitment
	)

	secshareTweak.SetByteSlice(chainhash.TaggedHash(
		chainhash.TagTapTweak,
		schnorr.SerializePubKey(util.GetXOnlyPubKey(pk)),
	)[:])
	btcec.ScalarBaseMultNonConst(secshareTweak, pubshareTweak)

	vssTweak = append(vssTweak, pubshareTweak)
	for i := 0; i < c.T()-1; i++ {
		vssTweak = append(vssTweak, &btcec.JacobianPoint{})
	}

	tweakedCommitment = NewVSSCommitment(vssTweak)
	tweakedCommitment, err := tweakedCommitment.Add(c)
	if err != nil {
		return nil, nil, nil, err
	}

	return tweakedCommitment, secshareTweak, pubshareTweak, nil
}

type VSS Polynomial

func GenerateVSS(seed []byte, t int) *VSS {
	var result VSS

	tBytes := make([]byte, 4)
	for i := 0; i < t; i++ {
		binary.BigEndian.PutUint32(tBytes, uint32(i))
		s := chainhash.TaggedHash(
			[]byte(util.BIP_DKG_TAG+"vss coeffs"),
			append(seed, tBytes...),
		)
		coefficient := new(btcec.ModNScalar)
		coefficient.SetByteSlice(s[:])
		result = append(result, coefficient)
	}

	return &result
}

func (v *VSS) SecShareFor(i int) (*btcec.ModNScalar, error) {
	if i < 0 {
		return nil, fmt.Errorf("Invalid participant index: %d", i)
	}

	x := new(btcec.ModNScalar)
	x.SetInt(uint32(i + 1))

	if x.Equals(new(btcec.ModNScalar)) {
		return nil, fmt.Errorf("Scalar can't be 0")
	}

	p := Polynomial(*v)

	return (&p).Evaluate(x), nil
}

func (v *VSS) SecShares(n int) ([]*btcec.ModNScalar, error) {
	shares := make([]*btcec.ModNScalar, 0, n)

	for i := 0; i < n; i++ {
		share, err := v.SecShareFor(i)
		if err != nil {
			return nil, err
		}

		shares = append(shares, share)
	}

	return shares, nil
}

func (v *VSS) Commit() *VSSCommitment {
	points := make([]*btcec.JacobianPoint, 0, len(*v))

	for _, c := range *v {
		point := new(btcec.JacobianPoint)
		btcec.ScalarBaseMultNonConst(c, point)
		point.ToAffine()
		points = append(points, point)
	}

	return NewVSSCommitment(points)
}

func (v *VSS) Secret() *btcec.PrivateKey {
	return &btcec.PrivateKey{Key: *(*v)[0]}
}
