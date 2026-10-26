// Adapted from https://github.com/BlockstreamResearch/bip-frost-dkg v0.2.0

package encpedpop

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/internal"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/simplpedpop"
	"github.com/stretchr/testify/require"
)

func simulateEncPedPop(ht *testing.T, seeds []*[32]byte, t int,
	investigate bool) ([]*threshold.DKGOutput, [][]byte) {

	var (
		n          = len(seeds)
		decKeys    = make([]*btcec.PrivateKey, 0, n)
		encKeys    = make([]*btcec.PublicKey, 0, n)
		pStates    = make([]*ParticipantState, 0, n)
		pMsgs      = make([]*ParticipantMessage, 0, n)
		dkgOutputs = make([]*threshold.DKGOutput, 0, n+1)
		eqs        = make([][]byte, 0, n+1)
	)

	for i := 0; i < n; i++ {
		decKey, encKey := internal.EncPedPopTestKeys(seeds[i])
		decKeys = append(decKeys, decKey)
		encKeys = append(encKeys, encKey)
	}

	for i := 0; i < n; i++ {
		auxData := internal.Rand32IntForTest(ht)
		pState, pMsg, err := ParticipantStep1(
			seeds[i], decKeys[i], encKeys, t, i, auxData,
		)
		require.NoError(ht, err)
		pStates = append(pStates, pState)
		pMsgs = append(pMsgs, pMsg)
	}

	var faultyIdx []int

	if investigate {
		faultTweak := new(btcec.ModNScalar)
		faultTweak.SetInt(17)

		for i := 0; i < n; i++ {
			fault := internal.RandIntForTest(ht, n)
			faultyIdx = append(faultyIdx, fault)

			pMsgs[fault].EncShares[i].Add(faultTweak)
		}
	}

	cMsg, cOut, cEq, encSecShares, err := CoordinatorStep(
		pMsgs, t, encKeys,
	)
	require.NoError(ht, err)

	if !investigate {
		dkgOutputs = append(dkgOutputs, cOut)
		eqs = append(eqs, cEq)
	}

	for i := 0; i < n; i++ {
		pOut, pEq, err := ParticipantStep2(
			pStates[i], decKeys[i], cMsg, encSecShares[i],
		)

		if investigate {
			require.Error(ht, err)

			pInv := err.(*ParticipantInvestigationData)
			cInv := CoordinatorInvestigate(pMsgs)
			require.Equal(ht, len(pMsgs), len(cInv))

			err = ParticipantInvestigate(pInv, cInv[i])
			if faultyIdx[i] == i {
				require.ErrorIs(
					ht, err,
					simplpedpop.ErrFaultyCoordinator,
				)
			} else {
				require.ErrorIs(
					ht, err,
					simplpedpop.ErrFaultyParticipantOrCoordinator(
						faultyIdx[i],
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
	internal.RegisterSimFunc("encpedpop", simulateEncPedPop, true, false)
}
