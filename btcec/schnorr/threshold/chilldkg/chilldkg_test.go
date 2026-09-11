// Adapted from https://github.com/BlockstreamResearch/bip-frost-dkg v0.2.0
package chilldkg

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
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
					if c.Recovery {
						name += "/recover"
					}
					if c.Investigation {
						name += "/investigate"
					}
					ht.Run(name, func(ht *testing.T) {
						testCorrectness(
							ht, p.t, p.n, c.SimFunc,
							c.Investigation,
							c.Recovery,
						)
					})
				}
			},
		)
	}
}

func testCorrectness(ht *testing.T, t, n int, simFunc util.SimFunc,
	investigate, recovery bool) {

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

	if !recovery {
		return
	}

	rec := RecoveryData(eqsOrRecs[0])
	for i := 1; i <= n; i++ {
		hostSecKey, _, err := hostPubKeyGen(seeds[i-1][:])
		require.NoError(ht, err)
		recDkgOutput, _, err := Recover(hostSecKey, &rec)
		require.NoError(ht, err)
		require.EqualValues(ht, recDkgOutput, dkgOutputs[i])
	}
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

func TestRecoveryAcknowledgment(ht *testing.T) {
	var (
		err         error
		seeds       [][32]byte
		ackSigs     []*RecoveryAckMessage
		t           = 2
		n           = 3
		hostSecKeys = make([]*btcec.PrivateKey, n)
		hostPubKeys = make([]*btcec.PublicKey, n)
	)

	for i := range n {
		randBytes, err := util.Rand32Int()
		require.NoError(ht, err)

		seeds = append(seeds, *randBytes)

		hostSecKeys[i], hostPubKeys[i], err = hostPubKeyGen(
			randBytes[:],
		)
	}

	params := &SessionParams{
		HostPubKeys: hostPubKeys,
		T:           t,
	}

	_, recDatas := simulateChillDKG(ht, seeds, t, false)
	recData := RecoveryData(recDatas[1])

	for i := range n {
		ackSig, err := ParticipantRecoveryAckSign(
			hostSecKeys[i], &recData, params,
			util.Rand32IntForTest(ht),
		)
		require.NoError(ht, err)

		require.Equal(ht, 64, len(ackSig.Bytes()))

		ackSigs = append(ackSigs, ackSig)
	}

	require.NoError(ht, ParticipantRecoveryAcksVerify(
		&recData, params, ackSigs,
	))

	// SKIPPED: invalid HostPubKey in params (params contain parsed keys)

	// Duplicate HostPubKey in params
	invalidParams := &SessionParams{
		HostPubKeys: []*btcec.PublicKey{hostPubKeys[0], hostPubKeys[0]},
		T:           t,
	}

	sig, err := ParticipantRecoveryAckSign(
		hostSecKeys[0], &recData, invalidParams,
		util.Rand32IntForTest(ht),
	)
	require.Nil(ht, sig)
	require.ErrorIs(ht, err, ErrDuplicateHostPubKey{0, 1})

	// Invalid threshold in params
	invalidParams = &SessionParams{
		HostPubKeys: hostPubKeys,
		T:           n + 1,
	}
	sig, err = ParticipantRecoveryAckSign(
		hostSecKeys[0], &recData, invalidParams,
		util.Rand32IntForTest(ht),
	)
	require.Nil(ht, sig)
	require.ErrorIs(ht, err, ErrThresholdOrCount)

	// Wrong hostSecKey
	wrongSecKey, err := btcec.NewPrivateKey()
	require.NoError(ht, err)
	sig, err = ParticipantRecoveryAckSign(
		wrongSecKey, &recData, params, util.Rand32IntForTest(ht),
	)
	require.Nil(ht, sig)
	require.ErrorIs(ht, err, ErrHostSecKey("Host secret key does not "+
		"match any host public key"))

	// SKIPPED: invalid randomness length (we pass a [32]byte)

	// Mismatched params
	invalidParams = &SessionParams{
		HostPubKeys: hostPubKeys,
		T:           t + 1,
	}
	sig, err = ParticipantRecoveryAckSign(
		hostSecKeys[0], &recData, invalidParams,
		util.Rand32IntForTest(ht),
	)
	require.Nil(ht, sig)
	require.ErrorIs(ht, err, ErrRecoveryData("Recovery data does not "+
		"match the provided session parameters"))

	err = ParticipantRecoveryAcksVerify(
		&recData, invalidParams, ackSigs,
	)
	require.ErrorIs(ht, err, ErrRecoveryData("Recovery data does not "+
		"match the provided session parameters"))

	// Corrupted recovery data
	corruptedRecoveryData := RecoveryData(make([]byte, len(recData)))
	randLen, err := rand.Read(corruptedRecoveryData)
	require.NoError(ht, err)
	require.Equal(ht, len(recData), randLen)

	sig, err = ParticipantRecoveryAckSign(
		hostSecKeys[0], &corruptedRecoveryData, params,
		util.Rand32IntForTest(ht),
	)
	require.Nil(ht, sig)
	require.ErrorIs(ht, err, ErrRecoveryData("Failed to deserialize "+
		"recovery data"))

	err = ParticipantRecoveryAcksVerify(
		&corruptedRecoveryData, params, ackSigs,
	)
	require.ErrorIs(ht, err, ErrRecoveryData("Failed to deserialize "+
		"recovery data"))

	// Invalid signature
	invalidAckSigs := make([]*RecoveryAckMessage, len(ackSigs))
	copy(invalidAckSigs, ackSigs)

	invalidAckSig, err := schnorr.Sign(hostSecKeys[0], ackSigs[0].Bytes())
	require.NoError(ht, err)

	invalidAckSigs[1] = &RecoveryAckMessage{invalidAckSig}

	err = ParticipantRecoveryAcksVerify(
		&recData, params, invalidAckSigs,
	)
	require.ErrorIs(ht, err, ErrInvalidRecoveryAck{
		Participant: 1,
	})

	// SKIPPED: invalid signature length (we pass already-parsed sigs)

	// Wrong number of signatures
	invalidAckSigs = ackSigs[:len(ackSigs)-1]
	err = ParticipantRecoveryAcksVerify(
		&recData, params, invalidAckSigs,
	)
	require.Error(ht, err)
}
