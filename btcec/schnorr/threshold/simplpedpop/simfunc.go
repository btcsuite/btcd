// Adapted from https://github.com/BlockstreamResearch/bip-frost-dkg v0.2.0
package simplpedpop

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/util"
	"github.com/stretchr/testify/require"
)

func simulateSimplPedPop(ht *testing.T, seeds [][32]byte, t int,
	investigate bool) ([]*util.DKGOutput, [][]byte) {

	var (
		n          = len(seeds)
		pStates    = make([]*ParticipantState, 0, n)
		pMsgs      = make([]*ParticipantMessage, 0, n)
		pShares    = make([][]*btcec.ModNScalar, 0, n)
		dkgOutputs = make([]*util.DKGOutput, 0, n+1)
		eqs        = make([][]byte, 0, n+1)
	)

	for i := 0; i < n; i++ {

		var auxData [32]byte
		rn, err := rand.Read(auxData[:])
		require.NoError(ht, err)
		require.Equal(ht, 32, rn)

		pState, pMsg, pShare, err := ParticipantStep1(
			seeds[i], t, n, i, auxData,
		)
		require.NoError(ht, err)
		pStates = append(pStates, pState)
		pMsgs = append(pMsgs, pMsg)
		pShares = append(pShares, pShare)
	}

	cMsg, cOut, cEq, err := CoordinatorStep(pMsgs, t, n)
	require.NoError(ht, err)

	if !investigate {
		dkgOutputs = append(dkgOutputs, cOut)
		eqs = append(eqs, cEq)
	}

	for i := 0; i < n; i++ {
		partialShares := make([]*btcec.ModNScalar, 0, n)
		for _, share := range pShares {
			partialShares = append(partialShares, share[i])
		}

		var faultyIdx int

		if investigate {
			randInt, err := rand.Int(
				rand.Reader, big.NewInt(int64(n)),
			)
			require.NoError(ht, err)
			faultyIdx = int(randInt.Int64())
			faultTweak := new(btcec.ModNScalar)
			faultTweak.SetInt(17)
			partialShares[faultyIdx].Add(faultTweak)
		}

		secShare := ParticipantStep2PrepareSecShare(partialShares)
		pOut, pEq, err := ParticipantStep2(pStates[i], cMsg, secShare)

		if investigate {
			require.Error(ht, err)

			pInv := err.(*ParticipantInvestigationData)
			cInv := CoordinatorInvestigate(pMsgs)
			require.Equal(ht, len(pMsgs), len(cInv))
			err = ParticipantInvestigate(pInv, cInv[i], partialShares)
			if faultyIdx == i {
				require.ErrorIs(ht, err, ErrFaultyCoordinator)
			} else {
				require.ErrorIs(
					ht, err,
					ErrFaultyParticipantOrCoordinator(
						faultyIdx,
					),
				)
			}

			continue
		}

		require.NoError(ht, err)
		dkgOutputs = append(dkgOutputs, pOut)
		eqs = append(eqs, pEq)
	}

	return dkgOutputs, eqs
}

func init() {
	util.RegisterSimFunc("simplpedpop", simulateSimplPedPop, true, false)
}
