// Adapted from https://github.com/BlockstreamResearch/bip-frost-dkg v0.2.0

package chilldkg

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/simplpedpop"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/util"
	"github.com/stretchr/testify/require"
)

func simulateChillDKG(ht *testing.T, seeds [][32]byte, t int,
	investigate bool) ([]*util.DKGOutput, [][]byte) {

	var (
		n          = len(seeds)
		decKeys    = make([]*btcec.PrivateKey, 0, n)
		encKeys    = make([]*btcec.PublicKey, 0, n)
		pStates1   = make([]*ParticipantState1, 0, n)
		pStates2   = make([]*ParticipantState2, 0, n)
		pMsgs1     = make([]*ParticipantMsg1, 0, n)
		pMsgs2     = make([]*ParticipantMsg2, 0, n)
		dkgOutputs = make([]*util.DKGOutput, 0, n+1)
		recs       = make([][]byte, 0, n+1)
	)

	for i := 0; i < n; i++ {
		decKey, encKey, err := hostPubKeyGen(seeds[i][:])
		require.NoError(ht, err)
		decKeys = append(decKeys, decKey)
		encKeys = append(encKeys, encKey)
	}

	params := &SessionParams{
		HostPubKeys: encKeys,
		T:           t,
	}

	for i := 0; i < n; i++ {
		var auxData [32]byte
		rn, err := rand.Read(auxData[:])
		require.NoError(ht, err)
		require.Equal(ht, 32, rn)

		pState, pMsg, err := ParticipantStep1(
			decKeys[i], params, auxData,
		)
		require.NoError(ht, err)
		pStates1 = append(pStates1, pState)
		pMsgs1 = append(pMsgs1, pMsg)
	}

	var faultyIdx []int

	if investigate {
		faultTweak := new(btcec.ModNScalar)
		faultTweak.SetInt(17)

		for i := 0; i < n; i++ {
			randInt, err := rand.Int(
				rand.Reader, big.NewInt(int64(n)),
			)
			require.NoError(ht, err)
			fault := int(randInt.Int64())
			faultyIdx = append(faultyIdx, fault)

			pMsgs1[fault].EncPmsg.EncShares[i].Add(faultTweak)
		}
	}

	cState, cMsg1, err := CoordinatorStep1(pMsgs1, params)
	require.NoError(ht, err)

	for i := 0; i < n; i++ {
		var auxData [32]byte
		rn, err := rand.Read(auxData[:])
		require.NoError(ht, err)
		require.Equal(ht, 32, rn)

		pState, pMsg, err := ParticipantStep2(
			decKeys[i], pStates1[i], cMsg1, auxData,
		)

		if investigate {
			require.Error(ht, err)

			pInv := err.(ErrUnknownFaultyParticipantOrCoordinator).
				InvData
			cInv := CoordinatorInvestigate(pMsgs1)
			require.Equal(ht, len(pMsgs1), len(cInv))

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
		pStates2 = append(pStates2, pState)
		pMsgs2 = append(pMsgs2, pMsg)
	}

	if investigate {
		return dkgOutputs, recs
	}

	cMsg2, cDkgOutput, cRec, err := CoordinatorFinalize(cState, pMsgs2)
	require.NoError(ht, err)

	dkgOutputs = append(dkgOutputs, cDkgOutput)
	recs = append(recs, []byte(*cRec))

	for i := 0; i < n; i++ {
		dkgOutput, rec, err := ParticipantFinalize(pStates2[i], cMsg2)
		require.NoError(ht, err)
		dkgOutputs = append(dkgOutputs, dkgOutput)
		recs = append(recs, []byte(*rec))
	}

	return dkgOutputs, recs
}

func init() {
	util.RegisterSimFunc("chilldkg", simulateChillDKG, true, true)
}
