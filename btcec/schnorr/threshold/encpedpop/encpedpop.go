// Adapted from https://github.com/BlockstreamResearch/bip-frost-dkg v0.2.0

package encpedpop

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/simplpedpop"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/util"
	"github.com/btcsuite/btcd/chainhash/v2"
)

const (
	// TODO(aakselrod): Move to chainhash package?
	ECDH_MSG_TAG         = util.BIP_DKG_TAG + "encpedpop ecdh"
	SELF_PAD_MSG_TAG     = util.BIP_DKG_TAG + "encaps_multi self_pad"
	ENC_SEED_MSG_TAG     = util.BIP_DKG_TAG + "encpedpop seed"
	AUX_RAND_MSG_TAG     = util.BIP_DKG_TAG + "simplpedpop aux"
	ENC_SECNONCE_MSG_TAG = util.BIP_DKG_TAG + "encpedpop secnonce"
)

// Adapted from secp to avoid losing oddness, maybe refactor?
func GenerateSharedSecret(privkey *btcec.PrivateKey,
	pubkey *btcec.PublicKey) *btcec.PublicKey {

	var point, result btcec.JacobianPoint
	pubkey.AsJacobian(&point)
	btcec.ScalarMultNonConst(&privkey.Key, &point, &result)
	result.ToAffine()
	return btcec.NewPublicKey(&result.X, &result.Y)
}

func ECDH(secKey *btcec.PrivateKey, theirKey *btcec.PublicKey, context []byte,
	sending bool) (*btcec.ModNScalar, error) {

	if util.IsPubKeyAtInfinity(theirKey) {
		return nil, fmt.Errorf("peer pubkey must not be point at infinity")
	}

	sharedKey := GenerateSharedSecret(secKey, theirKey)

	if util.IsPubKeyAtInfinity(sharedKey) {
		return nil, fmt.Errorf("ecdh shared secret must not be point at infinity")
	}

	data := chainhash.HashB(sharedKey.SerializeCompressed())

	if sending {
		data = append(
			data,
			secKey.PubKey().SerializeCompressed()...,
		)
		data = append(
			data,
			theirKey.SerializeCompressed()...,
		)
	} else {
		data = append(
			data,
			theirKey.SerializeCompressed()...,
		)
		data = append(
			data,
			secKey.PubKey().SerializeCompressed()...,
		)
	}

	if len(data) != 32+2*33 {
		return nil, fmt.Errorf("key sizes don't match")
	}

	data = append(data, context...)

	hash := chainhash.TaggedHash([]byte(ECDH_MSG_TAG), data)

	scalar := new(btcec.ModNScalar)
	scalar.SetBytes((*[32]byte)(hash))

	return scalar, nil
}

func SelfPad(symKey *btcec.PrivateKey, pubNonce *btcec.PublicKey,
	context []byte) *btcec.ModNScalar {

	var data []byte

	data = append(data, symKey.Serialize()...)
	data = append(data, pubNonce.SerializeCompressed()...)
	data = append(data, context...)

	hash := chainhash.TaggedHash([]byte(SELF_PAD_MSG_TAG), data)

	scalar := new(btcec.ModNScalar)
	scalar.SetBytes((*[32]byte)(hash))

	return scalar
}

func EncapsMulti(secNonce, decKey *btcec.PrivateKey, encKeys []*btcec.PublicKey,
	context []byte, idx int) ([]*btcec.ModNScalar, error) {

	pads := make([]*btcec.ModNScalar, 0, len(encKeys))

	for i := range encKeys {
		ctxBytes := make([]byte, 4, 4+len(context))
		binary.BigEndian.PutUint32(ctxBytes, uint32(i))
		ctxBytes = append(ctxBytes, context...)

		if i == idx {
			pad := SelfPad(decKey, secNonce.PubKey(), ctxBytes)
			pads = append(pads, pad)
		} else {
			pad, err := ECDH(secNonce, encKeys[i], ctxBytes, true)
			if err != nil {
				return nil, err
			}

			pads = append(pads, pad)
		}
	}

	return pads, nil
}

func EncryptMulti(secNonce, decKey *btcec.PrivateKey,
	encKeys []*btcec.PublicKey, context []byte, idx int,
	plaintexts []*btcec.ModNScalar) ([]*btcec.ModNScalar, error) {

	pads, err := EncapsMulti(secNonce, decKey, encKeys, context, idx)
	if err != nil {
		return nil, err
	}

	if len(pads) != len(plaintexts) {
		return nil, fmt.Errorf("number of plaintexts not equal to " +
			"number of participant pubkeys")
	}

	for i := range pads {
		pads[i].Add(plaintexts[i])
	}

	return pads, nil
}

func DecapsMulti(decKey *btcec.PrivateKey, pubNonces []*btcec.PublicKey,
	context []byte, idx int) ([]*btcec.ModNScalar, error) {

	pads := make([]*btcec.ModNScalar, 0, len(pubNonces))

	ctxBytes := make([]byte, 4, 4+len(context))
	binary.BigEndian.PutUint32(ctxBytes, uint32(idx))
	ctxBytes = append(ctxBytes, context...)
	for i := range pubNonces {
		if i == idx {
			pad := SelfPad(decKey, pubNonces[idx], ctxBytes)
			pads = append(pads, pad)
		} else {
			pad, err := ECDH(decKey, pubNonces[i], ctxBytes, false)
			if err != nil {
				return nil, err
			}

			pads = append(pads, pad)
		}
	}

	return pads, nil
}

func DecryptSum(decKey *btcec.PrivateKey, pubNonces []*btcec.PublicKey,
	context []byte, idx int, sumCiphertexts *btcec.ModNScalar) (
	*btcec.ModNScalar, error) {

	if idx >= len(pubNonces) {
		return nil, fmt.Errorf("idx out of range")
	}

	pads, err := DecapsMulti(decKey, pubNonces, context, idx)
	if err != nil {
		return nil, err
	}

	sumPlaintexts := util.SumScalars(pads).Negate().Add(sumCiphertexts)

	return sumPlaintexts, nil
}

type ParticipantMessage struct {
	Simpl     *simplpedpop.ParticipantMessage
	PubNonce  *btcec.PublicKey
	EncShares []*btcec.ModNScalar
}

func (p *ParticipantMessage) Bytes() []byte {
	buf := p.Simpl.Bytes()
	buf = append(buf, p.PubNonce.SerializeCompressed()...)

	for _, share := range p.EncShares {
		shareBytes := share.Bytes()
		buf = append(buf, shareBytes[:]...)
	}

	return buf
}

func ParseParticipantMessage(b []byte, t, n int) (*ParticipantMessage, error) {
	simplPmsgLen := 33*t + 64
	if len(b) < simplPmsgLen {
		return nil, util.ErrMsgParse(
			"missing simplpedpop participant message",
		)
	}

	simplPmsg, err := simplpedpop.ParseParticipantMessage(b[:64+33*t], t)
	if err != nil {
		return nil, err
	}

	b = b[simplPmsgLen:]
	if len(b) < 33 {
		return nil, util.ErrMsgParse("missing public nonce")
	}

	pubNonce, err := btcec.ParsePubKey(b[:33])
	if err != nil {
		return nil, err
	}

	b = b[33:]
	if len(b) < 32*n {
		return nil, util.ErrMsgParse("missing encrypted secret shares")
	}

	encShares := make([]*btcec.ModNScalar, 0, n)
	for i := range n {
		share := new(btcec.ModNScalar)
		overflow := share.SetByteSlice(b[32*i : 32*(i+1)])
		if overflow {
			return nil, fmt.Errorf("invalid encrypted secret share")
		}

		encShares = append(encShares, share)
	}

	b = b[32*n:]
	if len(b) != 0 {
		return nil, util.ErrMsgParse("incorrect input bytes length")
	}

	return &ParticipantMessage{
		Simpl:     simplPmsg,
		PubNonce:  pubNonce,
		EncShares: encShares,
	}, nil
}

type CoordinatorMessage struct {
	Simpl     *simplpedpop.CoordinatorMessage
	PubNonces []*btcec.PublicKey
}

func (c *CoordinatorMessage) Bytes() []byte {
	b := c.Simpl.Bytes()

	for _, nonce := range c.PubNonces {
		b = append(b, nonce.SerializeCompressed()...)
	}

	return b
}

func ParseCoordinatorMessage(b []byte, t, n int) (*CoordinatorMessage, error) {
	simplCmsgLen := 33*n + 33*(t-1) + 64*n
	if len(b) < simplCmsgLen {
		return nil, util.ErrMsgParse(
			"missing simplpedpop coordinator message",
		)
	}

	var (
		err  error
		cmsg = &CoordinatorMessage{
			PubNonces: make([]*btcec.PublicKey, 0, n),
		}
	)

	cmsg.Simpl, err = simplpedpop.ParseCoordinatorMessage(
		b[:simplCmsgLen], t, n,
	)
	if err != nil {
		return nil, err
	}

	b = b[simplCmsgLen:]
	if len(b) < 33*n {
		return nil, util.ErrMsgParse("missing public nonces")
	}

	for i := 0; i < n; i++ {
		key, err := btcec.ParsePubKey(b[i*33 : (i+1)*33])
		if err != nil {
			return nil, util.ErrFaultyParticipantOrCoordinator{
				"invalid public nonce", i,
			}
		}
		cmsg.PubNonces = append(cmsg.PubNonces, key)
	}

	b = b[33*n:]
	if len(b) != 0 {
		return nil, util.ErrMsgParse(
			"incorrect input bytes length",
		)
	}

	return cmsg, nil
}

type CoordinatorInvestigationMessage struct {
	EncPartialSecShares []*btcec.ModNScalar
	PartialPubShares    []*btcec.JacobianPoint
}

func (c *CoordinatorInvestigationMessage) Bytes() []byte {
	b := make([]byte, 0, len(c.EncPartialSecShares)*(32+33))

	for _, share := range c.EncPartialSecShares {
		shareBytes := share.Bytes()
		b = append(b, shareBytes[:]...)
	}

	simplCInvMsg := simplpedpop.CoordinatorInvestigationMessage(
		c.PartialPubShares,
	)
	b = append(b, simplCInvMsg.Bytes()...)

	return b
}

func ParseCoordinatorInvestigationMessage(b []byte, n int) (
	*CoordinatorInvestigationMessage, error) {

	if len(b) != n*(32+33) {
		return nil, fmt.Errorf("Invalid coordinator " +
			"investigation message")
	}

	var (
		c = new(CoordinatorInvestigationMessage)
	)
	for i := range n {
		share := new(btcec.ModNScalar)
		overflow := share.SetByteSlice(b[32*i : 32*(i+1)])
		if overflow {
			return nil, fmt.Errorf("invalid encrypted secret share")
		}

		c.EncPartialSecShares = append(c.EncPartialSecShares, share)
	}

	simplCInvMsg, err := simplpedpop.ParseCoordinatorInvestigationMessage(
		b[n*32:], n,
	)
	if err != nil {
		return nil, err
	}

	c.PartialPubShares = []*btcec.JacobianPoint(*simplCInvMsg)

	return c, nil
}

type ParticipantState struct {
	Simpl    *simplpedpop.ParticipantState
	PubNonce *btcec.PublicKey
	EncKeys  []*btcec.PublicKey
	Idx      int
}

type ParticipantInvestigationData struct {
	Simpl       *simplpedpop.ParticipantInvestigationData
	EncSecShare *btcec.ModNScalar
	Pads        []*btcec.ModNScalar
}

func (*ParticipantInvestigationData) Error() string {
	return "Received invalid secshare; consider using " +
		"participant_investigate() to determine a faulty party"
}

func SerializeEncContext(t int, encKeys []*btcec.PublicKey) []byte {
	context := make([]byte, 4, 4+33*len(encKeys))
	binary.BigEndian.PutUint32(context, uint32(t))

	for i := range encKeys {
		context = append(context, encKeys[i].SerializeCompressed()...)
	}

	return context
}

func ParticipantStep1(seed [32]byte, decKey *btcec.PrivateKey,
	encKeys []*btcec.PublicKey, t, idx int, random [32]byte) (
	*ParticipantState, *ParticipantMessage, error) {

	if t < 0 || t > math.MaxUint32 {
		return nil, nil, fmt.Errorf("t must be in range of uint32")
	}
	n := len(encKeys)

	encContext := SerializeEncContext(t, encKeys)
	seedPreimage := append(seed[:], random[:]...)
	seedPreimage = append(seedPreimage, encContext...)
	simplSeed := chainhash.TaggedHash(
		[]byte(ENC_SEED_MSG_TAG), seedPreimage,
	)

	simplAuxHash := chainhash.TaggedHash(
		[]byte(AUX_RAND_MSG_TAG), simplSeed[:],
	)
	simplAuxRand := [32]byte(*simplAuxHash)

	secNonceHash := chainhash.TaggedHash(
		[]byte(ENC_SECNONCE_MSG_TAG), simplSeed[:],
	)
	secNonce, pubNonce := btcec.PrivKeyFromBytes(secNonceHash[:])

	simplPstate, simplPmsg, shares, err := simplpedpop.ParticipantStep1(
		[32]byte(*simplSeed), t, n, idx, simplAuxRand,
	)
	if err != nil {
		return nil, nil, err
	}
	if len(shares) != n {
		return nil, nil, fmt.Errorf("simplpedpop participant step 1 " +
			"returned invalid number of shares")
	}

	encShares, err := EncryptMulti(
		secNonce, decKey, encKeys, encContext, idx, shares,
	)
	if err != nil {
		return nil, nil, err
	}

	pMsg := &ParticipantMessage{
		Simpl:     simplPmsg,
		PubNonce:  pubNonce,
		EncShares: encShares,
	}

	pState := &ParticipantState{
		Simpl:    simplPstate,
		PubNonce: pubNonce,
		EncKeys:  encKeys,
		Idx:      idx,
	}

	return pState, pMsg, nil
}

func appendEqInput(eqInput []byte, encKeys,
	pubNonces []*btcec.PublicKey) []byte {

	for i := range encKeys {
		eqInput = append(eqInput, encKeys[i].SerializeCompressed()...)
	}

	for i := range pubNonces {
		eqInput = append(eqInput, pubNonces[i].SerializeCompressed()...)
	}

	return eqInput
}

func ParticipantStep2(state *ParticipantState, decKey *btcec.PrivateKey,
	cmsg *CoordinatorMessage, encSecShare *btcec.ModNScalar) (
	*util.DKGOutput, []byte, error) {

	if !state.PubNonce.IsEqual(cmsg.PubNonces[state.Idx]) {
		return nil, nil, util.ErrFaultyCoordinator("Coordinator " +
			"replied with wrong pubnonce")
	}

	encContext := SerializeEncContext(state.Simpl.T, state.EncKeys)
	pads, err := DecapsMulti(decKey, cmsg.PubNonces, encContext, state.Idx)
	if err != nil {
		return nil, nil, err
	}

	secShare := util.SumScalars(pads).Negate().Add(encSecShare)

	dkgOutput, eqInput, err := simplpedpop.ParticipantStep2(
		state.Simpl, cmsg.Simpl, secShare,
	)
	if err != nil {
		invData, ok := err.(*simplpedpop.ParticipantInvestigationData)
		if !ok {
			return nil, nil, err
		}

		return nil, nil, &ParticipantInvestigationData{
			Simpl:       invData,
			EncSecShare: encSecShare,
			Pads:        pads,
		}
	}

	eqInput = appendEqInput(eqInput, state.EncKeys, cmsg.PubNonces)

	return dkgOutput, eqInput, nil
}

func ParticipantInvestigate(pInv *ParticipantInvestigationData,
	cInv *CoordinatorInvestigationMessage) error {

	if len(cInv.EncPartialSecShares) != len(pInv.Pads) {
		return fmt.Errorf("length of pads must be equal to length of encrypted secret shares")
	}

	partialSecShares := make([]*btcec.ModNScalar, 0, len(pInv.Pads))

	for i := range pInv.Pads {
		partialSecShare := new(btcec.ModNScalar)
		partialSecShare.Set(pInv.Pads[i]).Negate()
		partialSecShare.Add(cInv.EncPartialSecShares[i])
		partialSecShares = append(partialSecShares, partialSecShare)
	}

	simplCinv := simplpedpop.CoordinatorInvestigationMessage(
		cInv.PartialPubShares,
	)

	err := simplpedpop.ParticipantInvestigate(
		pInv.Simpl, &simplCinv, partialSecShares,
	)
	switch err {
	case nil:
		return simplpedpop.ErrNoErrorInvestigateCalled

	case simplpedpop.ErrSecretShareSum:
		if util.SumScalars(cInv.EncPartialSecShares).Equals(
			pInv.EncSecShare) {

			return fmt.Errorf("simplpedpop investigation " +
				"returned secret share sum error but " +
				"encrypted partial secshares add up to " +
				"encrypted secshare")
		} else {
			return util.ErrFaultyCoordinator("Sum of encrypted " +
				"partial secshares not equal to encrypted " +
				"secshare")
		}

	default:
		return err
	}
}

func CoordinatorStep(pMsgs []*ParticipantMessage, t int,
	encKeys []*btcec.PublicKey) (*CoordinatorMessage, *util.DKGOutput,
	[]byte, []*btcec.ModNScalar, error) {

	n := len(encKeys)
	if len(pMsgs) != n {
		return nil, nil, nil, nil, fmt.Errorf("pMsgs and encKeys " +
			"must be same length")
	}

	simplPmsgs := make([]*simplpedpop.ParticipantMessage, 0, n)
	pubNonces := make([]*btcec.PublicKey, 0, n)
	for _, pMsg := range pMsgs {
		simplPmsgs = append(simplPmsgs, pMsg.Simpl)
		pubNonces = append(pubNonces, pMsg.PubNonce)
	}

	simplCmsg, dkgOutput, eqInput, err := simplpedpop.CoordinatorStep(
		simplPmsgs, t, n,
	)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	encSecShares := make([]*btcec.ModNScalar, 0, n)

	for i := 0; i < n; i++ {
		encSecShare := new(btcec.ModNScalar)

		for _, pMsg := range pMsgs {
			encSecShare.Add(pMsg.EncShares[i])
		}

		encSecShares = append(encSecShares, encSecShare)
	}

	eqInput = appendEqInput(eqInput, encKeys, pubNonces)

	return &CoordinatorMessage{
		Simpl:     simplCmsg,
		PubNonces: pubNonces,
	}, dkgOutput, eqInput, encSecShares, nil
}

func CoordinatorInvestigate(
	pMsgs []*ParticipantMessage) []*CoordinatorInvestigationMessage {

	simplPmsgs := make([]*simplpedpop.ParticipantMessage, 0, len(pMsgs))
	for _, pMsg := range pMsgs {
		simplPmsgs = append(simplPmsgs, pMsg.Simpl)
	}

	simplCinvs := simplpedpop.CoordinatorInvestigate(simplPmsgs)
	cInvs := make([]*CoordinatorInvestigationMessage, 0, len(simplCinvs))
	for i := range simplCinvs {
		partialSecShares := make([]*btcec.ModNScalar, 0, len(pMsgs))
		for _, pMsg := range pMsgs {
			partialSecShares = append(
				partialSecShares, pMsg.EncShares[i],
			)
		}

		cInvs = append(cInvs, &CoordinatorInvestigationMessage{
			EncPartialSecShares: partialSecShares,
			PartialPubShares:    *simplCinvs[i],
		})
	}

	return cInvs
}
