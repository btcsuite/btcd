// Adapted from https://github.com/BlockstreamResearch/bip-frost-dkg v0.2.0

package vss

import (
	"crypto/rand"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

func TestVSS(ht *testing.T) {
	randPolynomial := func(t int) *Polynomial {
		coeffs := make([]*btcec.ModNScalar, 0, t)
		for i := 0; i < t; i++ {
			var data [32]byte
			n, err := rand.Read(data[:])
			require.NoError(ht, err)
			require.Equal(ht, 32, n)
			coeff := new(btcec.ModNScalar)
			require.Zero(ht, coeff.SetBytes(&data))
			coeffs = append(coeffs, coeff)
		}
		return NewPolynomial(coeffs)
	}

	for t := 1; t < 3; t++ {
		for n := t; n < 2*t+1; n++ {
			f := randPolynomial(t)
			vss := VSS(*f)
			secShares, err := vss.SecShares(n)
			require.NoError(ht, err)
			require.Equal(ht, n, len(secShares))

			commitment := vss.Commit()
			for i := 0; i < len(secShares); i++ {
				require.True(ht, VerifySecShare(
					secShares[i], commitment.PubShare(i),
				))
			}

			tweakedCommitment, privTweak, pubTweak, err := commitment.InvalidTaprootCommit()
			require.NoError(ht, err)
			var (
				tweakedPrivShare btcec.ModNScalar
				tweakedPubShare  btcec.JacobianPoint
			)
			tweakedPrivShare.Set(&(vss.Secret().Key)).Add(privTweak)
			btcec.AddNonConst(
				commitment.CommitmentToSecret(), pubTweak, &tweakedPubShare,
			)
			tweakedPubShare.ToAffine()
			require.True(ht, VerifySecShare(
				&tweakedPrivShare, &tweakedPubShare,
			))
			for i := 0; i < len(secShares); i++ {
				tweakedPrivShare.Set(secShares[i]).Add(privTweak)
				require.True(ht, VerifySecShare(
					&tweakedPrivShare,
					tweakedCommitment.PubShare(i),
				))
			}

			ht.Logf("Verified %d of %d", t, n)
		}
	}
}
