// Adapted from https://github.com/BlockstreamResearch/bip-frost-dkg v0.2.0

package chilldkg

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/encpedpop"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/util"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/vss"
	"github.com/btcsuite/btcd/chainhash/v2"
)

const (
	// TODO(aakselrod): Move to chainhash package?
	BIP_DKG_PARAMS_HASH_TAG = util.BIP_DKG_TAG + "params_hash"
	sigLen                  = schnorr.SignatureSize
)

var (
	BIP_DKG_TAG_CERTEQ_PREFIX = append(
		[]byte(util.BIP_DKG_TAG+"certeq message"),
		make([]byte, 33-22)...,
	)

	BIP_DKG_TAG_RECOVERY_ACK_PREFIX = append(
		[]byte(util.BIP_DKG_TAG+"recovery acknowledgment"),
		make([]byte, 33-31)...,
	)
)

func certEqMessage(x []byte, idx int) []byte {
	return append(binary.BigEndian.AppendUint32(
		BIP_DKG_TAG_CERTEQ_PREFIX, uint32(idx),
	), x...)
}

func CertEqParticipantStep(hostSecKey *btcec.PrivateKey, idx int,
	x []byte, auxRand [32]byte) ([]byte, error) {

	msg := certEqMessage(x, idx)
	sig, err := schnorr.Sign(hostSecKey, msg, schnorr.CustomNonce(auxRand))
	if err != nil {
		return nil, err
	}

	return sig.Serialize(), nil
}

func CertEqCertLen(n int) int {
	return n * sigLen
}

type ErrInvalidSignatureInCertificate int

func (e ErrInvalidSignatureInCertificate) Error() string {
	return "Participant has provided an invalid signature for the " +
		"certificate"
}

func CertEqVerify(hostPubKeys []*btcec.PublicKey, x, cert []byte) error {
	n := len(hostPubKeys)

	if len(cert) != CertEqCertLen(n) {
		return util.ErrMsgParse("invalid certificate length")
	}

	for i, hostPubKey := range hostPubKeys {
		msg := certEqMessage(x, i)
		sig, err := schnorr.ParseSignature(cert[i*sigLen : (i+1)*sigLen])
		if err != nil {
			return ErrInvalidSignatureInCertificate(i)
		}

		if !sig.Verify(msg, hostPubKey) {
			return ErrInvalidSignatureInCertificate(i)
		}
	}

	return nil
}

func CertEqCoordinatorStep(sigs [][]byte) []byte {
	return bytes.Join(sigs, nil)
}

type SessionParams struct {
	HostPubKeys []*btcec.PublicKey
	T           int
}

func ParamsValidate(params *SessionParams) error {
	n := len(params.HostPubKeys)
	if !(params.T >= 1 && n >= params.T && math.MaxUint32 >= n) {
		return ErrThresholdOrCount
	}

	// TODO(aakselrod): Check host pubkeys for validity when creating
	// SessionParams

	hostPubKeyToIdx := make(map[btcec.PublicKey]int)
	for i, key := range params.HostPubKeys {
		if _, ok := hostPubKeyToIdx[*key]; ok {
			return ErrDuplicateHostPubKey{hostPubKeyToIdx[*key], i}
		}

		hostPubKeyToIdx[*key] = i
	}

	return nil
}

func ParamsHash(params *SessionParams) (*chainhash.Hash, error) {
	//TODO(aakselrod): optimize and cache
	err := ParamsValidate(params)
	if err != nil {
		return nil, err
	}

	data := make([]byte, 4, 4+33*len(params.HostPubKeys))
	binary.BigEndian.PutUint32(data, uint32(params.T))
	for _, hostPubKey := range params.HostPubKeys {
		data = append(data, hostPubKey.SerializeCompressed()...)
	}

	return chainhash.TaggedHash([]byte(BIP_DKG_PARAMS_HASH_TAG), data), nil
}

type RecoveryData []byte

type ParticipantMsg1 struct {
	EncPmsg *encpedpop.ParticipantMessage
}

func (p *ParticipantMsg1) Bytes() []byte {
	return p.EncPmsg.Bytes()
}

func ParseParticipantMsg1(b []byte, t, n int) (*ParticipantMsg1, error) {
	encPmsg, err := encpedpop.ParseParticipantMessage(b, t, n)
	if err != nil {
		return nil, err
	}

	return &ParticipantMsg1{
		EncPmsg: encPmsg,
	}, nil
}

type ParticipantMsg2 struct {
	Sig []byte
}

func ParseParticipantMsg2(b []byte) (*ParticipantMsg2, error) {
	if len(b) != 64 {
		return nil, fmt.Errorf("MsgParseError")
	}

	return &ParticipantMsg2{
		Sig: b,
	}, nil
}

func (p *ParticipantMsg2) Bytes() []byte {
	return p.Sig
}

type CoordinatorMsg1 struct {
	EncCmsg      *encpedpop.CoordinatorMessage
	EncSecShares []*btcec.ModNScalar
}

func (c *CoordinatorMsg1) Bytes() []byte {
	b := c.EncCmsg.Bytes()

	for _, share := range c.EncSecShares {
		shareBytes := share.Bytes()
		b = append(b, shareBytes[:]...)
	}

	return b
}

func ParseCoordinatorMsg1(b []byte, t, n int) (*CoordinatorMsg1, error) {
	encCmsgLen := 33*n + 33*(t-1) + 64*n + 33*n
	if len(b) < encCmsgLen {
		return nil, util.ErrMsgParse(
			"message too short for encpedpop coordinator message",
		)
	}

	var (
		err  error
		cmsg = &CoordinatorMsg1{
			EncSecShares: make([]*btcec.ModNScalar, 0, n),
		}
	)

	cmsg.EncCmsg, err = encpedpop.ParseCoordinatorMessage(
		b[:encCmsgLen], t, n,
	)
	if err != nil {
		return nil, err
	}

	b = b[encCmsgLen:]
	if len(b) < 32*n {
		return nil, util.ErrMsgParse("missing encrypted secret shares")
	}

	for i := 0; i < n; i++ {
		share := new(btcec.ModNScalar)
		o := share.SetByteSlice(b[i*32 : (i+1)*32])
		if o {
			return nil, util.ErrMsgParse("invalid encrypted secret shares")
		}
		cmsg.EncSecShares = append(cmsg.EncSecShares, share)
	}

	b = b[32*n:]
	if len(b) != 0 {
		return nil, util.ErrMsgParse("incorrect input bytes length")
	}

	return cmsg, nil
}

type CoordinatorMsg2 struct {
	Cert []byte
}

func (c *CoordinatorMsg2) Bytes() []byte {
	return c.Cert
}

func ParseCoordinatorMsg2(b []byte) (*CoordinatorMsg2, error) {
	return &CoordinatorMsg2{
		Cert: b,
	}, nil
}

type CoordinatorInvestigationMsg struct {
	EncCinv *encpedpop.CoordinatorInvestigationMessage
}

func (c *CoordinatorInvestigationMsg) Bytes() []byte {
	return c.EncCinv.Bytes()
}

func ParseCoordinatorInvestigationMsg(b []byte, n int) (
	*CoordinatorInvestigationMsg, error) {

	encCInvMsg, err := encpedpop.ParseCoordinatorInvestigationMessage(b, n)
	if err != nil {
		return nil, err
	}

	return &CoordinatorInvestigationMsg{
		EncCinv: encCInvMsg,
	}, nil
}

type ParticipantState1 struct {
	Params   *SessionParams
	Idx      int
	EncState *encpedpop.ParticipantState
}

type ParticipantState2 struct {
	Params    *SessionParams
	EqInput   []byte
	DKGOutput *util.DKGOutput
}

func ParticipantStep1(hostSecKey *btcec.PrivateKey, params *SessionParams,
	random [32]byte) (*ParticipantState1, *ParticipantMsg1, error) {

	err := ParamsValidate(params)
	if err != nil {
		return nil, nil, err
	}

	var (
		seed       [32]byte
		hostPubKey = hostSecKey.PubKey()
	)

	idx, found := util.PubKeyIdx(hostPubKey, params.HostPubKeys)
	if !found {
		return nil, nil, ErrHostSecKey("Host secret key does not " +
			"match any host public key")
	}

	// TODO(aakselrod): comment on BIP? zero
	copy(seed[:], hostSecKey.Serialize())

	encState, encPmsg, err := encpedpop.ParticipantStep1(
		seed, hostSecKey, params.HostPubKeys, params.T, idx, random,
	)
	if err != nil {
		return nil, nil, err
	}

	return &ParticipantState1{
			Params:   params,
			Idx:      idx,
			EncState: encState,
		}, &ParticipantMsg1{
			EncPmsg: encPmsg,
		}, nil
}

type ErrUnknownFaultyParticipantOrCoordinator struct {
	InvData *encpedpop.ParticipantInvestigationData
}

func (e ErrUnknownFaultyParticipantOrCoordinator) Error() string {
	if e.InvData == nil {
		return "InvData must be set."
	}

	return e.InvData.Error()
}

func ParticipantStep2(hostSecKey *btcec.PrivateKey, state1 *ParticipantState1,
	cmsg1 *CoordinatorMsg1, auxRand [32]byte) (*ParticipantState2,
	*ParticipantMsg2, error) {

	if !state1.Params.HostPubKeys[state1.Idx].IsEqual(hostSecKey.PubKey()) {
		return nil, nil, ErrHostSecKey("Host secret key does not " +
			"match the one used in participant_step1")
	}

	dkgOutput, eqInput, err := encpedpop.ParticipantStep2(
		state1.EncState, hostSecKey, cmsg1.EncCmsg,
		cmsg1.EncSecShares[state1.Idx],
	)
	if err != nil {
		invData, ok := err.(*encpedpop.ParticipantInvestigationData)
		if !ok {
			return nil, nil, err
		}

		return nil, nil,
			ErrUnknownFaultyParticipantOrCoordinator{invData}
	}

	for _, share := range cmsg1.EncSecShares {
		shareBytes := share.Bytes()
		eqInput = append(eqInput, shareBytes[:]...)
	}

	sig, err := CertEqParticipantStep(
		hostSecKey, state1.Idx, eqInput, auxRand,
	)
	if err != nil {
		return nil, nil, err
	}

	return &ParticipantState2{
			Params:    state1.Params,
			EqInput:   eqInput,
			DKGOutput: dkgOutput,
		}, &ParticipantMsg2{
			Sig: sig,
		}, nil
}

func ParticipantFinalize(state2 *ParticipantState2, cmsg2 *CoordinatorMsg2) (
	*util.DKGOutput, *RecoveryData, error) {

	err := CertEqVerify(
		state2.Params.HostPubKeys, state2.EqInput, cmsg2.Cert,
	)
	if err != nil {
		_, ok := err.(ErrInvalidSignatureInCertificate)
		if ok {
			return nil, nil,
				util.ErrFaultyCoordinator("Coordinator has " +
					"provided a certificate with an " +
					"invalid signature")

		}

		if _, ok := err.(util.ErrMsgParse); ok {
			return nil, nil, util.ErrFaultyCoordinator(
				err.Error())
		}

		return nil, nil, err
	}

	recoveryData := RecoveryData(append(state2.EqInput, cmsg2.Cert...))

	return state2.DKGOutput, &recoveryData, nil
}

func ParticipantInvestigate(pInv *encpedpop.ParticipantInvestigationData,
	cInv *CoordinatorInvestigationMsg) error {

	return encpedpop.ParticipantInvestigate(pInv, cInv.EncCinv)
}

type CoordinatorState struct {
	Params    *SessionParams
	EqInput   []byte
	DKGOutput *util.DKGOutput
}

func CoordinatorStep1(pmsgs1 []*ParticipantMsg1, params *SessionParams) (
	*CoordinatorState, *CoordinatorMsg1, error) {

	if len(pmsgs1) != len(params.HostPubKeys) {
		return nil, nil, fmt.Errorf("num pmsgs not equal to num hostpubkeys")
	}

	encPmsgs := make(
		[]*encpedpop.ParticipantMessage, 0, len(pmsgs1),
	)

	for _, pmsg := range pmsgs1 {
		encPmsgs = append(encPmsgs, pmsg.EncPmsg)
	}

	encCmsg, dkgOutput, eqInput, encSecShares, err := encpedpop.
		CoordinatorStep(
			encPmsgs, params.T, params.HostPubKeys,
		)
	if err != nil {
		return nil, nil, err
	}

	for _, share := range encSecShares {
		shareBytes := share.Bytes()
		eqInput = append(eqInput, shareBytes[:]...)
	}

	return &CoordinatorState{
			Params:    params,
			EqInput:   eqInput,
			DKGOutput: dkgOutput,
		}, &CoordinatorMsg1{
			EncCmsg:      encCmsg,
			EncSecShares: encSecShares,
		}, nil
}

func CoordinatorFinalize(state *CoordinatorState, pmsgs2 []*ParticipantMsg2) (
	*CoordinatorMsg2, *util.DKGOutput, *RecoveryData, error) {

	sigs := make([][]byte, 0, len(pmsgs2))
	for _, pmsg := range pmsgs2 {
		sigs = append(sigs, pmsg.Sig)
	}

	cert := CertEqCoordinatorStep(sigs)
	err := CertEqVerify(state.Params.HostPubKeys, state.EqInput, cert)
	if err != nil {
		keyErr, ok := err.(ErrInvalidSignatureInCertificate)
		if ok {
			return nil, nil, nil, util.ErrFaultyParticipant{
				keyErr.Error(), int(keyErr),
			}
		}

		return nil, nil, nil, err
	}

	recoveryData := RecoveryData(append(state.EqInput, cert...))

	return &CoordinatorMsg2{
		Cert: cert,
	}, state.DKGOutput, &recoveryData, nil
}

func CoordinatorInvestigate(
	pMsgs []*ParticipantMsg1) []*CoordinatorInvestigationMsg {

	var (
		encPmsgs = make(
			[]*encpedpop.ParticipantMessage, 0,
			len(pMsgs),
		)
		cInvs = make([]*CoordinatorInvestigationMsg, 0, len(pMsgs))
	)

	for _, pMsg := range pMsgs {
		encPmsgs = append(encPmsgs, pMsg.EncPmsg)
	}

	encCinvs := encpedpop.CoordinatorInvestigate(encPmsgs)

	for _, cInv := range encCinvs {
		cInvs = append(cInvs, &CoordinatorInvestigationMsg{
			EncCinv: cInv,
		})
	}

	return cInvs
}

type ErrRecoveryData string

func (e ErrRecoveryData) Error() string {
	if e == "" {
		return "Recovery data error"
	}

	return string(e)
}

var errInvalidSessionParams = ErrRecoveryData("Invalid session parameters in " +
	"recovery data")

func deserializeRecoveryData(rec []byte) (int, *vss.VSSCommitment,
	[]*btcec.PublicKey, []*btcec.PublicKey, []*btcec.ModNScalar, []byte,
	error) {

	if len(rec) < 4 {
		return 0, nil, nil, nil, nil, nil, fmt.Errorf("invalid " +
			"recovery data length")
	}

	t := int(binary.BigEndian.Uint32(rec[:4]))

	rec = rec[4:]
	if len(rec) < t*33 {
		return 0, nil, nil, nil, nil, nil, fmt.Errorf("invalid " +
			"recovery data length")
	}

	sumComs, err := vss.ParseVSSCommitment(rec[:33*t], t)
	if err != nil {
		return 0, nil, nil, nil, nil, nil, err
	}

	rec = rec[33*t:]
	if len(rec)%(33+33+32+64) != 0 {
		return 0, nil, nil, nil, nil, nil, fmt.Errorf("invalid " +
			"recovery data length")
	}

	n := len(rec) / (33 + 33 + 32 + 64)
	hostPubKeys := make([]*btcec.PublicKey, 0, n)
	for i := 0; i < n; i++ {
		pubkey, err := btcec.ParsePubKey(rec[:33])
		if err != nil {
			return 0, nil, nil, nil, nil, nil,
				errInvalidSessionParams
		}
		hostPubKeys = append(hostPubKeys, pubkey)
		rec = rec[33:]
	}

	pubNonces := make([]*btcec.PublicKey, 0, n)
	for i := 0; i < n; i++ {
		pubnonce, err := btcec.ParsePubKey(rec[:33])
		if err != nil {
			return 0, nil, nil, nil, nil, nil, err
		}
		pubNonces = append(pubNonces, pubnonce)
		rec = rec[33:]
	}

	encSecShares := make([]*btcec.ModNScalar, 0, n)
	for i := 0; i < n; i++ {
		encSecShare := new(btcec.ModNScalar)
		overflowed := encSecShare.SetByteSlice(rec[:32])
		if overflowed {
			return 0, nil, nil, nil, nil, nil, fmt.Errorf(
				"Invalid encrypted secret share at %d", i)
		}
		encSecShares = append(encSecShares, encSecShare)
		rec = rec[32:]
	}

	return t, sumComs, hostPubKeys, pubNonces, encSecShares, rec, nil
}

func Recover(hostSecKey *btcec.PrivateKey, recoveryData *RecoveryData) (
	*util.DKGOutput, *SessionParams, error) {

	t, sumComs, hostPubKeys, pubNonces, encSecShares, cert, err :=
		deserializeRecoveryData(*recoveryData)
	if err != nil {
		if _, ok := err.(ErrRecoveryData); ok {
			return nil, nil, err
		}

		return nil, nil, ErrRecoveryData("Failed to deserialize " +
			"recovery data")
	}

	if hostSecKey != nil {
		_, found := util.PubKeyIdx(hostSecKey.PubKey(), hostPubKeys)
		if !found {
			return nil, nil, ErrHostSecKey("Host secret key does " +
				"not match any host public key in the " +
				"recovery data")
		}
	}

	n := len(hostPubKeys)
	params := &SessionParams{
		HostPubKeys: hostPubKeys,
		T:           t,
	}

	err = ParamsValidate(params)
	if err != nil {
		return nil, nil, errInvalidSessionParams
	}

	eqInput := (*recoveryData)[:len(*recoveryData)-len(cert)]
	err = CertEqVerify(hostPubKeys, eqInput, cert)
	if err != nil {
		return nil, nil, ErrRecoveryData("Invalid certificate in " +
			"recovery data")
	}

	sumComs, tweak, _, err := sumComs.InvalidTaprootCommit()
	if err != nil {
		return nil, nil, err
	}

	thresholdPubPoint := sumComs.CommitmentToSecret()
	thresholdPubKey := btcec.NewPublicKey(
		&thresholdPubPoint.X, &thresholdPubPoint.Y,
	)

	pubShares := make([]*btcec.PublicKey, 0, n)
	for i := 0; i < n; i++ {
		pubPoint := sumComs.PubShare(i)
		pubShare := btcec.NewPublicKey(&pubPoint.X, &pubPoint.Y)
		pubShares = append(pubShares, pubShare)
	}

	var secShareTweaked *btcec.PrivateKey
	if hostSecKey != nil {
		hostPubKey := hostSecKey.PubKey()
		idx, found := util.PubKeyIdx(hostPubKey, hostPubKeys)
		if !found {
			return nil, nil, ErrHostSecKey("Host secret key does " +
				"not match any host public key")
		}

		encContext := encpedpop.SerializeEncContext(t, hostPubKeys)
		secShare, err := encpedpop.DecryptSum(
			hostSecKey, pubNonces, encContext, idx,
			encSecShares[idx],
		)
		if err != nil {
			return nil, nil, err
		}

		secShare.Add(tweak)

		ourPubShare := new(btcec.JacobianPoint)
		pubShares[idx].AsJacobian(ourPubShare)

		if !vss.VerifySecShare(secShare, ourPubShare) {
			return nil, nil, fmt.Errorf("secshare doesn't match pubshare")
		}

		secShareTweaked = btcec.PrivKeyFromScalar(secShare)
	}

	return &util.DKGOutput{
		SecShare:        secShareTweaked,
		ThresholdPubKey: thresholdPubKey,
		PubShares:       pubShares,
	}, params, nil
}

// TODO(aakselrod): should this be ACK instead of Ack?
func recoveryAckMessage(x []byte, idx int) []byte {
	return append(binary.BigEndian.AppendUint32(
		BIP_DKG_TAG_RECOVERY_ACK_PREFIX, uint32(idx),
	), x...)
}

func recoveryAckSign(hostSecKey *btcec.PrivateKey, idx int, x []byte,
	auxRand [32]byte) (*schnorr.Signature, error) {

	return schnorr.Sign(
		hostSecKey, recoveryAckMessage(x, idx),
		schnorr.CustomNonce(auxRand),
	)
}

type RecoveryAckMessage struct {
	Sig *schnorr.Signature
}

func ParseRecoveryAckMessage(b []byte) (*RecoveryAckMessage, error) {
	sig, err := schnorr.ParseSignature(b)
	if err != nil {
		return nil, err
	}

	return &RecoveryAckMessage{sig}, nil
}

func (r *RecoveryAckMessage) Bytes() []byte {
	return r.Sig.Serialize()
}

func checkParamsAgainstRecoveryData(recoveryData *RecoveryData,
	params *SessionParams) error {

	err := ParamsValidate(params)
	if err != nil {
		return err
	}

	t, _, hostPubKeys, _, _, _, err := deserializeRecoveryData(
		*recoveryData,
	)
	if err != nil {
		return ErrRecoveryData("Failed to deserialize recovery " +
			"data")
	}
	if t != params.T || len(hostPubKeys) != len(params.HostPubKeys) {
		return ErrRecoveryData("Recovery data does not match " +
			"the provided session parameters")
	}

	for i, pubKey := range hostPubKeys {
		if !pubKey.IsEqual(params.HostPubKeys[i]) {
			return ErrRecoveryData("Recovery data does not " +
				"match the provided session parameters")
		}
	}

	return nil
}

func ParticipantRecoveryAckSign(hostSecKey *btcec.PrivateKey,
	recoveryData *RecoveryData, params *SessionParams, auxRand [32]byte) (
	*RecoveryAckMessage, error) {

	idx, found := util.PubKeyIdx(hostSecKey.PubKey(), params.HostPubKeys)
	if !found {
		return nil, ErrHostSecKey("Host secret key does not match " +
			"any host public key")
	}

	err := checkParamsAgainstRecoveryData(recoveryData, params)
	if err != nil {
		return nil, err
	}

	sig, err := recoveryAckSign(hostSecKey, idx, *recoveryData, auxRand)
	if err != nil {
		return nil, err
	}

	return &RecoveryAckMessage{sig}, nil
}

type ErrInvalidRecoveryAck util.ErrFaultyParticipant

func (e ErrInvalidRecoveryAck) Error() string {
	if e.Msg == "" {
		return "Invalid recovery ack error"
	}

	return e.Msg
}

func ParticipantRecoveryAcksVerify(recoveryData *RecoveryData,
	params *SessionParams, ackSigs []*RecoveryAckMessage) error {

	err := checkParamsAgainstRecoveryData(recoveryData, params)
	if err != nil {
		return err
	}

	if len(ackSigs) != len(params.HostPubKeys) {
		return fmt.Errorf("Number of recovery acknowledgment " +
			"signatures must match number of host public keys")
	}

	for i, pubKey := range params.HostPubKeys {
		msg := recoveryAckMessage(*recoveryData, i)
		if !ackSigs[i].Sig.Verify(msg, pubKey) {
			return ErrInvalidRecoveryAck{
				Participant: i,
			}
		}
	}

	return nil
}
