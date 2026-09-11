// Adapted from https://github.com/BlockstreamResearch/bip-frost-dkg v0.2.0

package simplpedpop

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/util"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/vss"
)

const (
	// TODO(aakselrod): Move to chainhash package?
	POP_MSG_TAG = util.BIP_DKG_TAG + "pop message"
)

var (
	ErrSecretShareSum = fmt.Errorf("Sum of partial secret shares not equal to secret share")

	ErrPublicShareSum = util.ErrFaultyCoordinator("Sum of partial pubshares not equal to pubshare")

	ErrNotNPartialSecretShares = fmt.Errorf("Wrong number of partial secret shares")

	ErrNoErrorInvestigateCalled = fmt.Errorf("No error, all inputs are consistent")

	ErrFaultyCoordinator = util.ErrFaultyCoordinator("Coordinator fiddled with the share from me to myself")
)

// TODO(aakselrod): change to slice to enable blaming multiple participants?
func ErrFaultyParticipantOrCoordinator(participantId int) error {
	return util.ErrFaultyParticipantOrCoordinator{
		"Participant sent invalid partial secshare", participantId,
	}
}

type Pop []byte

func PopMsg(idx int) []byte {
	idxBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(idxBytes, uint32(idx))
	return idxBytes
}

func PopProve(privKey *btcec.PrivateKey, idx int, auxRand [32]byte) (*Pop,
	error) {

	// TODO(aakselrod): support correct tag prefix for tagged hash.
	sig, err := schnorr.Sign(
		privKey, PopMsg(idx), schnorr.CustomNonce(auxRand),
		schnorr.CustomNonConsensusBIPTag(POP_MSG_TAG),
	)
	if err != nil {
		return nil, err
	}

	pop := Pop(sig.Serialize())
	return &pop, nil
}

func PopVerify(pop *Pop, pubKey *btcec.PublicKey, idx int) bool {
	// TODO(aakselrod): support correct tag prefix for tagged hash.
	sig, err := schnorr.ParseSignature(*pop)
	if err != nil {
		return false
	}

	return sig.VerifyCustom(
		PopMsg(idx), pubKey,
		schnorr.CustomNonConsensusBIPTag(POP_MSG_TAG),
	)
}

type ParticipantMessage struct {
	Commitment *vss.VSSCommitment
	Pop        *Pop
}

func (p *ParticipantMessage) Bytes() []byte {
	buf := p.Commitment.Bytes()
	buf = append(buf, *p.Pop...)

	return buf
}

func ParseParticipantMessage(b []byte, t int) (*ParticipantMessage, error) {
	if len(b) < 33*t {
		return nil, util.ErrMsgParse("missing VSS commitment")
	}

	commitment, err := vss.ParseVSSCommitment(b[:33*t], t)
	if err != nil {
		return nil, util.ErrMsgParse("invalid VSS commitment")
	}

	b = b[33*t:]
	if len(b) < 64 {
		return nil, util.ErrMsgParse("missing proof of possession")
	}

	pop := Pop(b[:64])

	b = b[64:]
	if len(b) != 0 {
		return nil, util.ErrMsgParse("incorrect input bytes length")
	}

	return &ParticipantMessage{
		Commitment: commitment,
		Pop:        &pop,
	}, nil
}

type CoordinatorMessage struct {
	CommitmentsToSecrets          []*btcec.JacobianPoint
	SumCommitmentsToNonConstTerms []*btcec.JacobianPoint
	Pops                          []*Pop
}

func ParseCoordinatorMessage(b []byte, t, n int) (*CoordinatorMessage, error) {
	if len(b) < 33*n {
		return nil, util.ErrMsgParse(
			"missing commitments to secrets",
		)
	}

	cmsg := &CoordinatorMessage{
		CommitmentsToSecrets:          make([]*btcec.JacobianPoint, 0, n),
		SumCommitmentsToNonConstTerms: make([]*btcec.JacobianPoint, 0, t-1),
		Pops:                          make([]*Pop, 0, n),
	}

	for i := 0; i < n; i++ {
		point, err := btcec.ParseJacobian(b[i*33 : (i+1)*33])
		if err != nil {
			return nil, util.ErrMsgParse(
				"invalid commitment to secret",
			)
		}
		cmsg.CommitmentsToSecrets = append(cmsg.CommitmentsToSecrets, &point)
	}

	b = b[n*33:]
	if len(b) < 33*(t-1) {
		return nil, util.ErrMsgParse(
			"missing sum commitments to non-constant terms",
		)
	}

	for i := 0; i < t-1; i++ {
		point, err := btcec.ParseJacobian(b[i*33 : (i+1)*33])
		if err != nil {
			return nil, util.ErrMsgParse(
				"invalid sum commitment to non-constant term",
			)
		}
		cmsg.SumCommitmentsToNonConstTerms = append(
			cmsg.SumCommitmentsToNonConstTerms, &point,
		)
	}

	b = b[(t-1)*33:]

	for i := 0; i < n; i++ {
		pop := Pop(b[i*64 : (i+1)*64])
		cmsg.Pops = append(cmsg.Pops, &pop)
	}

	return cmsg, nil
}

func (m *CoordinatorMessage) Bytes() []byte {
	msg := make([]byte, 0,
		len(m.CommitmentsToSecrets)*33+
			len(m.SumCommitmentsToNonConstTerms)*33+
			len(m.Pops)*schnorr.SignatureSize,
	)

	for _, com := range append(
		m.CommitmentsToSecrets,
		m.SumCommitmentsToNonConstTerms...,
	) {
		msg = append(msg, btcec.JacobianToByteSlice(*com)...)
	}

	for _, pop := range m.Pops {
		msg = append(msg, *pop...)
	}

	return msg
}

type CoordinatorInvestigationMessage []*btcec.JacobianPoint

func (c *CoordinatorInvestigationMessage) Bytes() []byte {
	partialPubShares := []*btcec.JacobianPoint(*c)

	b := make([]byte, 0, len(partialPubShares)*33)

	for _, point := range partialPubShares {
		b = append(b, btcec.JacobianToByteSlice(*point)...)
	}

	return b
}

func ParseCoordinatorInvestigationMessage(msg []byte, n int) (
	*CoordinatorInvestigationMessage, error) {

	if len(msg) != n*33 {
		return nil, fmt.Errorf("Invalid coordinator " +
			"investigation message")
	}

	partialPubShares := make([]*btcec.JacobianPoint, 0, n)
	for i := range n {
		point, err := btcec.ParseJacobian(msg[i*33 : (i+1)*33])
		if err != nil {
			return nil, util.ErrMsgParse("invalid partial pubshare")
		}

		partialPubShares = append(partialPubShares, &point)
	}

	cMsg := CoordinatorInvestigationMessage(partialPubShares)

	return &cMsg, nil

}

func assembleSumCommitments(coms,
	sumComsToNonConstTerms []*btcec.JacobianPoint) *vss.VSSCommitment {

	return vss.NewVSSCommitment(append(
		[]*btcec.JacobianPoint{util.SumPoints(coms)},
		sumComsToNonConstTerms...,
	))
}

type ParticipantState struct {
	T                  int
	N                  int
	Idx                int
	CommitmentToSecret *btcec.JacobianPoint
}

type ParticipantInvestigationData struct {
	N        int
	Idx      int
	SecShare *btcec.ModNScalar
	PubShare *btcec.JacobianPoint
}

func (*ParticipantInvestigationData) Error() string {
	return "Received invalid secshare; consider using " +
		"participant_investigate() to determine a faulty party"
}

func ParticipantStep1(seed [32]byte, threshold, numParticipants, idx int,
	auxRand [32]byte) (*ParticipantState, *ParticipantMessage,
	[]*btcec.ModNScalar, error) {

	switch {
	case threshold > numParticipants:
		return nil, nil, nil, fmt.Errorf("threshold greater " +
			"than number of participants")

	case idx >= numParticipants:
		return nil, nil, nil, fmt.Errorf("index greater than or " +
			"equal to number of participants")
	}

	vss := vss.GenerateVSS(seed[:], threshold)
	partialSecSharesFromMe, err := vss.SecShares(numParticipants)
	if err != nil {
		return nil, nil, nil, err
	}

	pop, err := PopProve(vss.Secret(), idx, auxRand)
	if err != nil {
		return nil, nil, nil, err
	}

	commitment := vss.Commit()
	commitmentToSecret := commitment.CommitmentToSecret()

	msg := &ParticipantMessage{
		Commitment: commitment,
		Pop:        pop,
	}

	state := &ParticipantState{
		T:                  threshold,
		N:                  numParticipants,
		Idx:                idx,
		CommitmentToSecret: commitmentToSecret,
	}

	return state, msg, partialSecSharesFromMe, nil
}

func ParticipantStep2PrepareSecShare(
	partialSecShares []*btcec.ModNScalar) *btcec.ModNScalar {

	return util.SumScalars(partialSecShares)
}

func ParticipantStep2(state *ParticipantState, cMsg *CoordinatorMessage,
	secShare *btcec.ModNScalar) (*util.DKGOutput, []byte, error) {

	if state.N != len(cMsg.CommitmentsToSecrets) || state.N != len(cMsg.Pops) ||
		state.T-1 != len(cMsg.SumCommitmentsToNonConstTerms) {

		return nil, nil, fmt.Errorf("incorrect number of commitments")
	}

	if !bytes.Equal(
		btcec.JacobianToByteSlice(*state.CommitmentToSecret),
		btcec.JacobianToByteSlice(*cMsg.CommitmentsToSecrets[state.Idx]),
	) {
		return nil, nil, util.ErrFaultyCoordinator("Coordinator " +
			"sent unexpected first group element for local " +
			"participant id")
	}

	for i, com := range cMsg.CommitmentsToSecrets {
		if i == state.Idx {
			continue
		}

		if util.IsPointAtInfinity(com) {
			return nil, nil, util.ErrFaultyParticipantOrCoordinator{
				"Participant sent invalid commitment", i,
			}
		}

		if !PopVerify(
			cMsg.Pops[i], btcec.NewPublicKey(&com.X, &com.Y), i,
		) {
			return nil, nil, util.ErrFaultyParticipantOrCoordinator{
				"Participant sent invalid proof-of-knowledge",
				i,
			}
		}
	}

	sumCommitments := assembleSumCommitments(
		cMsg.CommitmentsToSecrets,
		cMsg.SumCommitmentsToNonConstTerms,
	)

	sumCommitmentsTweaked, tweak, pubTweak, err := sumCommitments.
		InvalidTaprootCommit()
	if err != nil {
		return nil, nil, err
	}

	var (
		pubShareTweaked = sumCommitmentsTweaked.PubShare(state.Idx)
		secShareTweaked = &btcec.ModNScalar{}
	)

	secShareTweaked.Set(secShare)
	secShareTweaked.Add(tweak)

	if !vss.VerifySecShare(secShareTweaked, pubShareTweaked) {
		var (
			pubShare = new(btcec.JacobianPoint)
			neg1     = new(btcec.ModNScalar)
		)

		neg1.SetInt(1)
		neg1.Negate()
		btcec.ScalarMultNonConst(neg1, pubTweak, pubShare)
		btcec.AddNonConst(pubShareTweaked, pubShare, pubShare)
		pubShare.ToAffine()

		return nil, nil, &ParticipantInvestigationData{
			N:        state.N,
			Idx:      state.Idx,
			SecShare: secShare,
			PubShare: pubShare,
		}
	}

	thresholdPubKey := sumCommitmentsTweaked.CommitmentToSecret()

	pubShares := make([]*btcec.PublicKey, 0, state.N)
	for i := 0; i < state.N; i++ {
		// TODO(aakselrod): optimize case where i == idx?
		curPubShare := sumCommitmentsTweaked.PubShare(i)
		pubShares = append(pubShares, btcec.NewPublicKey(
			&curPubShare.X, &curPubShare.Y,
		))
	}

	secShareKey := btcec.PrivKeyFromScalar(secShareTweaked)

	eqInput := make([]byte, 4)
	binary.BigEndian.PutUint32(eqInput, uint32(state.T))
	eqInput = append(eqInput, sumCommitments.Bytes()...)

	return &util.DKGOutput{
		SecShare:  secShareKey,
		PubShares: pubShares,
		ThresholdPubKey: btcec.NewPublicKey(
			&thresholdPubKey.X, &thresholdPubKey.Y,
		),
	}, eqInput, nil
}

func ParticipantInvestigate(pInv *ParticipantInvestigationData,
	cInv *CoordinatorInvestigationMessage,
	partialSecShares []*btcec.ModNScalar) error {

	if len(partialSecShares) != pInv.N {
		return ErrNotNPartialSecretShares
	}

	sumShares := util.SumPoints(*cInv)
	if !(sumShares.X.Equals(&pInv.PubShare.X) &&
		sumShares.Y.Equals(&pInv.PubShare.Y) &&
		sumShares.Z.Equals(&pInv.PubShare.Z)) {

		return ErrPublicShareSum
	}

	secShare := util.SumScalars(partialSecShares)
	if !(secShare.Equals(pInv.SecShare)) {
		return ErrSecretShareSum
	}

	for i := 0; i < pInv.N; i++ {
		if vss.VerifySecShare(partialSecShares[i], (*cInv)[i]) {
			continue
		}

		if pInv.Idx == i {
			return ErrFaultyCoordinator
		}

		return ErrFaultyParticipantOrCoordinator(i)
	}

	if !vss.VerifySecShare(pInv.SecShare, pInv.PubShare) {
		return fmt.Errorf("My own secret share doesn't match my public share")
	}

	return ErrNoErrorInvestigateCalled
}

func CoordinatorStep(pMsgs []*ParticipantMessage, t, n int) (
	*CoordinatorMessage, *util.DKGOutput, []byte, error) {

	if len(pMsgs) != n {
		return nil, nil, nil, fmt.Errorf("must have exactly n participant messages")
	}

	cMsg := &CoordinatorMessage{
		CommitmentsToSecrets:          make([]*btcec.JacobianPoint, 0, n),
		SumCommitmentsToNonConstTerms: make([]*btcec.JacobianPoint, 0, t-1),
		Pops:                          make([]*Pop, 0, n),
	}

	for _, pMsg := range pMsgs {
		cMsg.CommitmentsToSecrets = append(
			cMsg.CommitmentsToSecrets,
			pMsg.Commitment.CommitmentToSecret(),
		)

		cMsg.Pops = append(cMsg.Pops, pMsg.Pop)
	}

	for j := 0; j < t-1; j++ {
		commitmentsToNonConstTerms := make([]*btcec.JacobianPoint, 0, n)

		for _, pMsg := range pMsgs {
			commitmentsToNonConstTerms = append(
				commitmentsToNonConstTerms,
				pMsg.Commitment.CommitmentToNonConstTerms()[j],
			)
		}

		cMsg.SumCommitmentsToNonConstTerms = append(
			cMsg.SumCommitmentsToNonConstTerms,
			util.SumPoints(commitmentsToNonConstTerms),
		)
	}

	sumCommitments := assembleSumCommitments(
		cMsg.CommitmentsToSecrets,
		cMsg.SumCommitmentsToNonConstTerms,
	)

	sumCommitmentsTweaked, _, _, err := sumCommitments.
		InvalidTaprootCommit()
	if err != nil {
		return nil, nil, nil, err
	}

	thresholdPubKey := sumCommitmentsTweaked.CommitmentToSecret()

	pubShares := make([]*btcec.PublicKey, 0, n)
	for i := 0; i < n; i++ {
		curPubShare := sumCommitmentsTweaked.PubShare(i)
		pubShares = append(pubShares, btcec.NewPublicKey(
			&curPubShare.X, &curPubShare.Y,
		))
	}

	eqInput := make([]byte, 4)
	binary.BigEndian.PutUint32(eqInput, uint32(t))
	eqInput = append(eqInput, sumCommitments.Bytes()...)

	return cMsg, &util.DKGOutput{
		PubShares: pubShares,
		ThresholdPubKey: btcec.NewPublicKey(
			&thresholdPubKey.X, &thresholdPubKey.Y,
		),
	}, eqInput, nil
}

func CoordinatorInvestigate(
	pMsgs []*ParticipantMessage) []*CoordinatorInvestigationMessage {

	n := len(pMsgs)

	cMsgs := make([]*CoordinatorInvestigationMessage, 0, n)

	for i := 0; i < n; i++ {
		pubShares := make([]*btcec.JacobianPoint, 0, n)

		for _, msg := range pMsgs {
			pubShares = append(
				pubShares, msg.Commitment.PubShare(i),
			)
		}

		cMsg := CoordinatorInvestigationMessage(pubShares)
		cMsgs = append(cMsgs, &cMsg)
	}

	return cMsgs
}
