// Adapted from https://github.com/BlockstreamResearch/bip-frost-dkg v0.2.0
package chilldkg

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	_ "github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/simplpedpop"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/util"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/vss"
	"github.com/stretchr/testify/require"
)

func TestChillDKG(ht *testing.T) {
	tests := []struct {
		t int
		n int
	}{
		{1, 1},
		{1, 2},
		{2, 2},
		{2, 3},
		{2, 5},
	}

	for _, p := range tests {
		ht.Run(
			fmt.Sprintf("%d of %d", p.t, p.n),
			func(ht *testing.T) {
				for _, c := range util.GetDKGTests() {
					name := c.Name
					if c.Investigation {
						name += "/investigate"
					}
					ht.Run(name, func(ht *testing.T) {
						testCorrectness(
							ht, p.t, p.n, c.SimFunc,
							c.Investigation,
						)
					})
				}
			},
		)
	}
}

func testCorrectness(ht *testing.T, t, n int, simFunc util.SimFunc,
	investigate bool) {

	seeds := make([][32]byte, n)
	for i := range seeds {
		l, err := rand.Read(seeds[i][:])
		require.NoError(ht, err)
		require.Equal(ht, 32, l)
	}

	dkgOutputs, eqsOrRecs := simFunc(ht, seeds, t, investigate)

	if investigate {
		require.Equal(ht, 0, len(dkgOutputs))
		require.Equal(ht, 0, len(eqsOrRecs))
		return
	}

	require.Equal(ht, n+1, len(dkgOutputs))
	require.Equal(ht, n+1, len(eqsOrRecs))
	testCorrectnessDKGOutput(ht, t, n, dkgOutputs)
}

func testCorrectnessDKGOutput(ht *testing.T, t, n int,
	dkgOutputs []*util.DKGOutput) {

	require.Equal(ht, n+1, len(dkgOutputs))

	for _, dkgOutput := range dkgOutputs {
		require.EqualValues(
			ht, dkgOutputs[0].ThresholdPubKey,
			dkgOutput.ThresholdPubKey,
		)

		require.Equal(ht, n, len(dkgOutput.PubShares))
		require.EqualValues(
			ht, dkgOutputs[0].PubShares,
			dkgOutput.PubShares,
		)
	}

	thresholdPubKey := dkgOutputs[0].ThresholdPubKey

	require.Nil(ht, dkgOutputs[0].SecShare)

	for shareIdxs := range signerCombinations(n, t) {
		shares := make([]*btcec.ModNScalar, 0, t)
		for _, idx := range shareIdxs {
			shares = append(shares, &dkgOutputs[idx+1].SecShare.Key)
		}

		recoveredSecret := btcec.PrivKeyFromScalar(
			recoverSecret(ht, shareIdxs, shares),
		)
		require.EqualValues(ht, thresholdPubKey, recoveredSecret.PubKey())
	}
}

func TestRecoverSecret(ht *testing.T) {
	f := vss.Polynomial([]*btcec.ModNScalar{
		new(btcec.ModNScalar), new(btcec.ModNScalar),
	})
	f[0].SetInt(23)
	f[1].SetInt(42)

	shares := make([]*btcec.ModNScalar, 0, 3)
	for i := 0; i < 3; i++ {
		idxScalar := new(btcec.ModNScalar)
		idxScalar.SetInt(uint32(i + 1))
		shares = append(shares, f.Evaluate(idxScalar))
	}

	require.EqualValues(ht, f[0], recoverSecret(
		ht, []int{0, 1}, []*btcec.ModNScalar{shares[0], shares[1]},
	))

	require.EqualValues(ht, f[0], recoverSecret(
		ht, []int{0, 2}, []*btcec.ModNScalar{shares[0], shares[2]},
	))

	require.EqualValues(ht, f[0], recoverSecret(
		ht, []int{1, 2}, []*btcec.ModNScalar{shares[1], shares[2]},
	))
}

func recoverSecret(ht *testing.T, indices []int,
	shares []*btcec.ModNScalar) *btcec.ModNScalar {

	t := len(shares)
	require.Equal(ht, t, len(indices))

	interpolatedShares := make([]*btcec.ModNScalar, 0, t)
	for i, idx := range indices {
		lam, err := util.DeriveInterpolatingValue(indices, idx)
		require.NoError(ht, err)
		lam.Mul(shares[i])
		interpolatedShares = append(interpolatedShares, lam)
	}

	return util.SumScalars(interpolatedShares)
}
