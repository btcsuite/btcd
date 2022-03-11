// Copyright 2013-2016 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package musig2

import (
	"bytes"
	"fmt"

	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

var (
	// NonceBlindTag is that tag used to construct the value b, which
	// blinds the second public nonce of each party.
	NonceBlindTag = []byte("MuSig/noncecoef")

	// ChallengeHashTag is the tag used to construct the challenge hash
	ChallengeHashTag = []byte("BIP0340/challenge")

	// ErrNoncePointAtInfinity is returned if during signing, the fully
	// combined public nonce is the point at infinity.
	ErrNoncePointAtInfinity = fmt.Errorf("signing nonce is the infinity " +
		"point")

	// ErrPrivKeyZero is returned when the private key for signing is
	// actually zero.
	ErrPrivKeyZero = fmt.Errorf("priv key is zero")

	// ErrPartialSigInvalid is returned when a partial is found to be
	// invalid.
	ErrPartialSigInvalid = fmt.Errorf("partial signature is invalid")

	// ErrSecretNonceZero is returned when a secret nonce is passed in a
	// zero.
	ErrSecretNonceZero = fmt.Errorf("secret nonce is blank")
)

// infinityPoint is the jacobian representation of the point at infinity.
var infinityPoint btcec.JacobianPoint

// PartialSignature reprints a partial (s-only) musig2 multi-signature. This
// isn't a valid schnorr signature by itself, as it needs to be aggregated
// along with the other partial signatures to be completed.
type PartialSignature struct {
	S *btcec.ModNScalar

	R *btcec.PublicKey
}

// NewPartialSignature returns a new instances of the partial sig struct.
func NewPartialSignature(s *btcec.ModNScalar,
	r *btcec.PublicKey) PartialSignature {

	return PartialSignature{
		S: s,
		R: r,
	}
}

// SignOption is a functional option argument that allows callers to modify the
// way we generate musig2 schnorr signatures.
type SignOption func(*signOptions)

// signOptions houses the set of functional options that can be used to modify
// the method used to generate the musig2 partial signature.
type signOptions struct {
	// fastSign determines if we'll skip the check at the end of the
	// routine where we attempt to verify the produced signature.
	fastSign bool

	// sortKeys determines if the set of keys should be sorted before doing
	// key aggregation.
	sortKeys bool
}

// defaultSignOptions returns the default set of signing operations.
func defaultSignOptions() *signOptions {
	return &signOptions{}
}

// WithFastSign forces signing to skip the extra verification step at the end.
// Performance sensitive applications may opt to use this option to speed up
// the signing operation.
func WithFastSign() SignOption {
	return func(o *signOptions) {
		o.fastSign = true
	}
}

// WithSortedKeys determines if the set of signing public keys are to be sorted
// or not before doing key aggregation.
func WithSortedKeys() SignOption {
	return func(o *signOptions) {
		o.sortKeys = true
	}
}

// Sign generates a musig2 partial signature given the passed key set, secret
// nonce, public nonce, and private keys. This method returns an error if the
// generated nonces are either too large, or end up mapping to the point at
// infinity.
func Sign(secNonce [SecNonceSize]byte, privKey *btcec.PrivateKey,
	combinedNonce [PubNonceSize]byte, pubKeys []*btcec.PublicKey,
	msg [32]byte, signOpts ...SignOption) (*PartialSignature, error) {

	// First, parse the set of optional signing options.
	opts := defaultSignOptions()
	for _, option := range signOpts {
		option(opts)
	}

	// Next, we'll parse the public nonces into R1 and R2.
	r1, err := btcec.ParsePubKey(
		combinedNonce[:btcec.PubKeyBytesLenCompressed],
	)
	if err != nil {
		return nil, err
	}
	r2, err := btcec.ParsePubKey(
		combinedNonce[btcec.PubKeyBytesLenCompressed:],
	)
	if err != nil {
		return nil, err
	}

	// Compute the hash of all the keys here as we'll need it do aggregrate
	// the keys and also at the final step of signing.
	keysHash := keyHashFingerprint(pubKeys, opts.sortKeys)

	// Next we'll construct the aggregated public key based on the set of
	// signers.
	uniqueKeyIndex := secondUniqueKeyIndex(pubKeys)
	combinedKey := AggregateKeys(
		pubKeys, opts.sortKeys, WithKeysHash(keysHash),
		WithUniqueKeyIndex(uniqueKeyIndex),
	)

	// Next we'll compute the value b, that blinds our second public
	// nonce:
	//  * b = h(tag=NonceBlindTag, combinedNonce || combinedKey || m).
	var (
		nonceMsgBuf  bytes.Buffer
		nonceBlinder btcec.ModNScalar
	)
	nonceMsgBuf.Write(combinedNonce[:])
	nonceMsgBuf.Write(schnorr.SerializePubKey(combinedKey))
	nonceMsgBuf.Write(msg[:])
	nonceBlindHash := chainhash.TaggedHash(
		NonceBlindTag, nonceMsgBuf.Bytes(),
	)
	nonceBlinder.SetByteSlice(nonceBlindHash[:])

	var nonce, r1J, r2J btcec.JacobianPoint
	r1.AsJacobian(&r1J)
	r2.AsJacobian(&r2J)

	// With our nonce blinding value, we'll now combine both the public
	// nonces, using the blinding factor to tweak the second nonce:
	//  * R = R_1 + b*R_2
	btcec.ScalarMultNonConst(&nonceBlinder, &r2J, &r2J)
	btcec.AddNonConst(&r1J, &r2J, &nonce)

	// If the combined nonce it eh point at infinity, then we'll bail out.
	if nonce == infinityPoint {
		return nil, ErrNoncePointAtInfinity
	}

	// Next we'll parse out our two secret nonces, which we'll be using in
	// the core signing process below.
	var k1, k2 btcec.ModNScalar
	k1.SetByteSlice(secNonce[:btcec.PrivKeyBytesLen])
	k2.SetByteSlice(secNonce[btcec.PrivKeyBytesLen:])

	if k1.IsZero() || k2.IsZero() {
		return nil, ErrSecretNonceZero
	}

	nonce.ToAffine()

	nonceKey := btcec.NewPublicKey(&nonce.X, &nonce.Y)

	// If the nonce R has an odd y coordinate, then we'll negate both our
	// secret nonces.
	if nonce.Y.IsOdd() {
		k1.Negate()
		k2.Negate()
	}

	privKeyScalar := privKey.Key
	if privKeyScalar.IsZero() {
		return nil, ErrPrivKeyZero
	}

	// If the y coordinate of the public key is odd xor the y coordinate of
	// the combined public key is odd, then we'll negate the private key.
	pubKey := privKey.PubKey()
	pubKeyYIsOdd := func() bool {
		pubKeyBytes := pubKey.SerializeCompressed()
		return pubKeyBytes[0] == secp.PubKeyFormatCompressedOdd
	}()
	combinedKeyYIsOdd := func() bool {
		combinedKeyBytes := combinedKey.SerializeCompressed()
		return combinedKeyBytes[0] == secp.PubKeyFormatCompressedOdd
	}()
	if pubKeyYIsOdd != combinedKeyYIsOdd {
		privKeyScalar.Negate()
	}

	// Next we'll create the challenge hash that commits to the combined
	// nonce, combined public key and also the message: * e =
	// H(tag=ChallengeHashTag, R || Q || m) mod n
	var challengeMsg bytes.Buffer
	challengeMsg.Write(schnorr.SerializePubKey(nonceKey))
	challengeMsg.Write(schnorr.SerializePubKey(combinedKey))
	challengeMsg.Write(msg[:])
	challengeBytes := chainhash.TaggedHash(
		ChallengeHashTag, challengeMsg.Bytes(),
	)
	var e btcec.ModNScalar
	e.SetByteSlice(challengeBytes[:])

	// Next, we'll compute mu, our aggregation coefficient for the key that
	// we're signing with.
	mu := aggregationCoefficient(pubKeys, pubKey, keysHash, uniqueKeyIndex)

	// With mu constructed, we can finally generate our partial signature
	// as: s = (k1_1 + b*k_2 + e*mu*d) mod n.
	s := new(btcec.ModNScalar)
	s.Add(&k1).Add(k2.Mul(&nonceBlinder)).Add(e.Mul(mu).Mul(&privKeyScalar))

	sig := NewPartialSignature(s, nonceKey)

	// If we're not in fast sign mode, then we'll also validate our partial
	// signature.
	if !opts.fastSign {
		pubNonce := secNonceToPubNonce(&secNonce)
		sigValid := sig.Verify(
			pubNonce, combinedNonce, pubKeys, pubKey, msg,
			signOpts...,
		)
		if !sigValid {
			return nil, fmt.Errorf("sig is invalid!")
		}
	}

	return &sig, nil
}

// Verify implements partial signature verification given the public nonce for
// the signer, aggregate nonce, signer set and finally the message being
// signed.
func (p *PartialSignature) Verify(pubNonce [PubNonceSize]byte,
	combinedNonce [PubNonceSize]byte, keySet []*btcec.PublicKey,
	signingKey *btcec.PublicKey, msg [32]byte, signOpts ...SignOption) bool {

	pubKey := schnorr.SerializePubKey(signingKey)
	return verifyPartialSig(
		p, pubNonce, combinedNonce, keySet, pubKey, msg, signOpts...,
	) == nil
}

// verifyPartialSig attempts to verify a partial schnorr signature given the
// necessary parameters. This is the internal version of Verify that returns
// detailed errors.  signed.
func verifyPartialSig(partialSig *PartialSignature, pubNonce [PubNonceSize]byte,
	combinedNonce [PubNonceSize]byte, keySet []*btcec.PublicKey,
	pubKey []byte, msg [32]byte, signOpts ...SignOption) error {

	opts := defaultSignOptions()
	for _, option := range signOpts {
		option(opts)
	}

	// First we'll map the internal partial signature back into something
	// we can manipulate.
	s := partialSig.S

	// Next we'll parse out the two public nonces into something we can
	// use.
	//
	// TODO(roasbeef): consolidate, new method
	r1, err := btcec.ParsePubKey(
		combinedNonce[:btcec.PubKeyBytesLenCompressed],
	)
	if err != nil {
		return err
	}
	r2, err := btcec.ParsePubKey(
		combinedNonce[btcec.PubKeyBytesLenCompressed:],
	)
	if err != nil {
		return err
	}

	// Compute the hash of all the keys here as we'll need it do aggregrate
	// the keys and also at the final step of verification.
	keysHash := keyHashFingerprint(keySet, opts.sortKeys)

	uniqueKeyIndex := secondUniqueKeyIndex(keySet)

	// Next we'll construct the aggregated public key based on the set of
	// signers.
	combinedKey := AggregateKeys(
		keySet, opts.sortKeys,
		WithKeysHash(keysHash), WithUniqueKeyIndex(uniqueKeyIndex),
	)

	// Next we'll compute the value b, that blinds our second public
	// nonce:
	//  * b = h(tag=NonceBlindTag, combinedNonce || combinedKey || m).
	var (
		nonceMsgBuf  bytes.Buffer
		nonceBlinder btcec.ModNScalar
	)
	nonceMsgBuf.Write(combinedNonce[:])
	nonceMsgBuf.Write(schnorr.SerializePubKey(combinedKey))
	nonceMsgBuf.Write(msg[:])
	nonceBlindHash := chainhash.TaggedHash(NonceBlindTag, nonceMsgBuf.Bytes())
	nonceBlinder.SetByteSlice(nonceBlindHash[:])

	var nonce, r1J, r2J btcec.JacobianPoint
	r1.AsJacobian(&r1J)
	r2.AsJacobian(&r2J)

	// With our nonce blinding value, we'll now combine both the public
	// nonces, using the blinding factor to tweak the second nonce:
	//  * R = R_1 + b*R_2
	btcec.ScalarMultNonConst(&nonceBlinder, &r2J, &r2J)
	btcec.AddNonConst(&r1J, &r2J, &nonce)

	// Next, we'll parse out the set of public nonces this signer used to
	// generate the signature.
	pubNonce1, err := btcec.ParsePubKey(
		pubNonce[:btcec.PubKeyBytesLenCompressed],
	)
	if err != nil {
		return err
	}
	pubNonce2, err := btcec.ParsePubKey(
		pubNonce[btcec.PubKeyBytesLenCompressed:],
	)
	if err != nil {
		return err
	}

	// We'll perform a similar aggregation and blinding operator as we did
	// above for the combined nonces: R' = R_1' + b*R_2'.
	var pubNonceJ, pubNonce1J, pubNonce2J btcec.JacobianPoint
	pubNonce1.AsJacobian(&pubNonce1J)
	pubNonce2.AsJacobian(&pubNonce2J)
	btcec.ScalarMultNonConst(&nonceBlinder, &pubNonce2J, &pubNonce2J)
	btcec.AddNonConst(&pubNonce1J, &pubNonce2J, &pubNonceJ)

	nonce.ToAffine()

	// If the combined nonce used in the challenge hash has an odd y
	// coordinate, then we'll negate our final public nonce.
	if nonce.Y.IsOdd() {
		pubNonceJ.ToAffine()
		pubNonceJ.Y.Negate(1)
		pubNonceJ.Y.Normalize()
	}

	// Next we'll create the challenge hash that commits to the combined
	// nonce, combined public key and also the message:
	//  * e = H(tag=ChallengeHashTag, R || Q || m) mod n
	var challengeMsg bytes.Buffer
	challengeMsg.Write(schnorr.SerializePubKey(btcec.NewPublicKey(
		&nonce.X, &nonce.Y,
	)))
	challengeMsg.Write(schnorr.SerializePubKey(combinedKey))
	challengeMsg.Write(msg[:])
	challengeBytes := chainhash.TaggedHash(
		ChallengeHashTag, challengeMsg.Bytes(),
	)
	var e btcec.ModNScalar
	e.SetByteSlice(challengeBytes[:])

	signingKey, err := schnorr.ParsePubKey(pubKey)
	if err != nil {
		return err
	}

	// Next, we'll compute mu, our aggregation coefficient for the key that
	// we're signing with.
	mu := aggregationCoefficient(keySet, signingKey, keysHash, uniqueKeyIndex)

	// If the combined key has an odd y coordinate, then we'll negate the
	// signer key.
	var signKeyJ btcec.JacobianPoint
	signingKey.AsJacobian(&signKeyJ)
	combinedKeyBytes := combinedKey.SerializeCompressed()
	if combinedKeyBytes[0] == secp.PubKeyFormatCompressedOdd {
		signKeyJ.ToAffine()
		signKeyJ.Y.Negate(1)
		signKeyJ.Y.Normalize()
	}

	// In the final set, we'll check that: s*G == R' + e*mu*P.
	var sG, rP btcec.JacobianPoint
	btcec.ScalarBaseMultNonConst(s, &sG)
	btcec.ScalarMultNonConst(e.Mul(mu), &signKeyJ, &rP)
	btcec.AddNonConst(&rP, &pubNonceJ, &rP)

	sG.ToAffine()
	rP.ToAffine()

	if sG != rP {
		return ErrPartialSigInvalid
	}

	return nil
}

// CombineSigs combines the set of public keys given the final aggregated
// nonce, and the series of partial signatures for each nonce.
func CombineSigs(combinedNonce *btcec.PublicKey,
	partialSigs []*PartialSignature) *schnorr.Signature {

	var combinedSig btcec.ModNScalar
	for _, partialSig := range partialSigs {
		combinedSig.Add(partialSig.S)
	}

	// TODO(roasbeef): less verbose way to get the x coord...
	var nonceJ btcec.JacobianPoint
	combinedNonce.AsJacobian(&nonceJ)
	nonceJ.ToAffine()

	return schnorr.NewSignature(&nonceJ.X, &combinedSig)
}
