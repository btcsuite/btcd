// Copyright 2013-2022 The btcsuite developers

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
	// NestedNonceBlindTag is the tagged hash tag used to compute the
	// nested nonce blinding factor (H̄_non in the paper). Unlike the
	// top-level NonceBlindTag, this hash does NOT include the message.
	NestedNonceBlindTag = []byte("MuSig/nested_noncecoef")

	// ErrNoNestingLevels is returned when NestedSign is called with an
	// empty nesting levels slice.
	ErrNoNestingLevels = fmt.Errorf("at least one nesting level is " +
		"required")

	// ErrSignerKeyNotInLevel is returned when the signer's public key is
	// not found in the deepest nesting level's key set.
	ErrSignerKeyNotInLevel = fmt.Errorf("signer's key not found in " +
		"deepest nesting level")

	// ErrNoPartialSigs is returned when AggregateNestedPartialSigs is
	// called with an empty slice of partial signatures.
	ErrNoPartialSigs = fmt.Errorf("at least one partial signature is " +
		"required")
)

// NestingLevel represents one level in a nested MuSig2 cosigner tree. Each
// level contains the set of public keys at that depth and the internally
// aggregated nonce produced by AggregateNonces at that depth.
type NestingLevel struct {
	// PubKeys is the set of all cosigner public keys at this depth.
	PubKeys []*btcec.PublicKey

	// AggNonce is the internally aggregated nonce (output of
	// AggregateNonces) for the signers at this depth.
	AggNonce [PubNonceSize]byte
}

// computeNestedNonceBlind computes the nested nonce blinding factor b for a
// given aggregated key and aggregate nonce pair. This implements H̄_non from
// the paper: b = H(NestedNonceBlindTag, aggKey_xonly || R'_1 || R'_2).
func computeNestedNonceBlind(aggKey *btcec.PublicKey,
	aggNonce [PubNonceSize]byte) *btcec.ModNScalar {

	var buf bytes.Buffer
	buf.Write(schnorr.SerializePubKey(aggKey))
	buf.Write(aggNonce[:])

	blindHash := chainhash.TaggedHash(
		NestedNonceBlindTag, buf.Bytes(),
	)

	var blind btcec.ModNScalar
	blind.SetByteSlice(blindHash[:])

	return &blind
}

// ExternalizeNonces implements the SignAggExt algorithm from the nested MuSig2
// paper. It takes the internally aggregated nonce and the aggregated public
// key for a group of nested signers, and produces an externalized public nonce
// that is indistinguishable from a regular participant's nonce.
//
// For ν=2: R_1 = R'_1 (unchanged), R_2 = b * R'_2.
func ExternalizeNonces(aggNonce [PubNonceSize]byte,
	aggKey *btcec.PublicKey) ([PubNonceSize]byte, error) {

	var extNonce [PubNonceSize]byte

	// Parse the two internal aggregate nonce points.
	r1J, err := btcec.ParseJacobian(
		aggNonce[:btcec.PubKeyBytesLenCompressed],
	)
	if err != nil {
		return extNonce, fmt.Errorf("unable to parse R'_1: %w", err)
	}
	r2J, err := btcec.ParseJacobian(
		aggNonce[btcec.PubKeyBytesLenCompressed:],
	)
	if err != nil {
		return extNonce, fmt.Errorf("unable to parse R'_2: %w", err)
	}

	// Compute the nested nonce blinding factor.
	b := computeNestedNonceBlind(aggKey, aggNonce)

	// R_1 = R'_1 (b^0 = 1, so unchanged).
	// R_2 = b * R'_2.
	btcec.ScalarMultNonConst(b, &r2J, &r2J)

	// Serialize the externalized nonce.
	r1J.ToAffine()
	r2J.ToAffine()

	copy(extNonce[:], btcec.JacobianToByteSlice(r1J))
	copy(
		extNonce[btcec.PubKeyBytesLenCompressed:],
		btcec.JacobianToByteSlice(r2J),
	)

	return extNonce, nil
}

// NestedSign implements the Sign' algorithm from the nested MuSig2 paper. It
// generates a partial signature for a leaf signer in a nested MuSig2 tree.
//
// Parameters:
//   - secNonce: the signer's secret nonce (from GenNonces).
//   - privKey: the signer's private key.
//   - combinedNonce: the top-level aggregate nonce (from AggregateNonces at
//     depth 0, or from ExternalizeNonces if this group is itself nested).
//   - pubKeys: the top-level set of public keys (depth 0).
//   - msg: the 32-byte message to sign.
//   - nestingLevels: the nesting levels from depth 1 (one level below the top)
//     down to the signer's own level (deepest). For a signer that is one level
//     deep, this contains exactly one NestingLevel.
//   - signOpts: optional signing options (tweaks, etc.) applied to the
//     top-level key aggregation.
func NestedSign(secNonce [SecNonceSize]byte, privKey *btcec.PrivateKey,
	combinedNonce [PubNonceSize]byte, pubKeys []*btcec.PublicKey,
	msg [32]byte, nestingLevels []NestingLevel,
	signOpts ...SignOption) (*PartialSignature, error) {

	if len(nestingLevels) == 0 {
		return nil, ErrNoNestingLevels
	}

	// Parse the set of optional signing options.
	opts := defaultSignOptions()
	for _, option := range signOpts {
		option(opts)
	}

	// Check that our signing key belongs to the secNonce.
	if !bytes.Equal(secNonce[btcec.PrivKeyBytesLen*2:],
		privKey.PubKey().SerializeCompressed()) {

		return nil, ErrSecNoncePubkey
	}

	pubKey := privKey.PubKey()

	// Check that the signer's key is in the deepest nesting level.
	deepest := nestingLevels[len(nestingLevels)-1]
	var containsKey bool
	for _, pk := range deepest.PubKeys {
		if pubKey.IsEqual(pk) {
			containsKey = true
			break
		}
	}
	if !containsKey {
		return nil, ErrSignerKeyNotInLevel
	}

	// Walk from the deepest level upward, computing the product of
	// aggregation coefficients and nonce blinding factors.
	aProduct := new(btcec.ModNScalar).SetInt(1)
	bProduct := new(btcec.ModNScalar).SetInt(1)

	signerKey := pubKey
	for i := len(nestingLevels) - 1; i >= 0; i-- {
		level := nestingLevels[i]

		// Compute aggregation coefficient for signerKey at this level.
		keysHash := keyHashFingerprint(level.PubKeys, false)
		uniqueKeyIdx := secondUniqueKeyIndex(level.PubKeys, false)
		a := aggregationCoefficient(
			level.PubKeys, signerKey, keysHash, uniqueKeyIdx,
		)
		aProduct.Mul(a)

		// Aggregate the keys at this level to get the group key that
		// represents this level one step up in the tree.
		aggKey, _, _, err := AggregateKeys(level.PubKeys, false)
		if err != nil {
			return nil, fmt.Errorf("unable to aggregate keys at "+
				"nesting level %d: %w", i, err)
		}

		// Compute the nested nonce blinding factor for this level.
		b := computeNestedNonceBlind(aggKey.FinalKey, level.AggNonce)
		bProduct.Mul(b)

		// The aggregated key at this level is the signer's key at the
		// next level up.
		signerKey = aggKey.FinalKey
	}

	// Now handle the top level (depth 0).
	keysHash := keyHashFingerprint(pubKeys, opts.sortKeys)
	uniqueKeyIdx := secondUniqueKeyIndex(pubKeys, opts.sortKeys)

	// Compute aggregation coefficient for signerKey (our group's
	// aggregate key) at the top level.
	a := aggregationCoefficient(
		pubKeys, signerKey, keysHash, uniqueKeyIdx,
	)
	aProduct.Mul(a)

	// Aggregate the top-level keys with any tweaks.
	keyAggOpts := []KeyAggOption{
		WithKeysHash(keysHash), WithUniqueKeyIndex(uniqueKeyIdx),
	}
	switch {
	case opts.bip86Tweak:
		keyAggOpts = append(keyAggOpts, WithBIP86KeyTweak())
	case opts.taprootTweak != nil:
		keyAggOpts = append(
			keyAggOpts, WithTaprootKeyTweak(opts.taprootTweak),
		)
	case len(opts.tweaks) != 0:
		keyAggOpts = append(keyAggOpts, WithKeyTweaks(opts.tweaks...))
	}

	combinedKey, parityAcc, _, err := AggregateKeys(
		pubKeys, opts.sortKeys, keyAggOpts...,
	)
	if err != nil {
		return nil, err
	}

	// Compute the top-level signing nonce R and blinding factor b_0.
	nonce, nonceBlinder, err := computeSigningNonce(
		combinedNonce, combinedKey.FinalKey, msg,
	)
	if err != nil {
		return nil, err
	}
	bProduct.Mul(nonceBlinder)

	// Parse the secret nonces.
	var k1, k2 btcec.ModNScalar
	k1.SetByteSlice(secNonce[:btcec.PrivKeyBytesLen])
	k2.SetByteSlice(secNonce[btcec.PrivKeyBytesLen:])

	if k1.IsZero() || k2.IsZero() {
		return nil, ErrSecretNonceZero
	}

	nonce.ToAffine()
	nonceKey := btcec.NewPublicKey(&nonce.X, &nonce.Y)

	// If the nonce R has an odd y coordinate, negate both secret nonces.
	if nonce.Y.IsOdd() {
		k1.Negate()
		k2.Negate()
	}

	privKeyScalar := privKey.Key
	if privKeyScalar.IsZero() {
		return nil, ErrPrivKeyZero
	}

	// Compute the parity factor for the combined key.
	combinedKeyYIsOdd := func() bool {
		combinedKeyBytes := combinedKey.FinalKey.SerializeCompressed()
		return combinedKeyBytes[0] == secp.PubKeyFormatCompressedOdd
	}()

	parityCombinedKey := new(btcec.ModNScalar).SetInt(1)
	if combinedKeyYIsOdd {
		parityCombinedKey.Negate()
	}

	// Apply parity factors: d = g * gacc * x.
	privKeyScalar.Mul(parityCombinedKey).Mul(parityAcc)

	// Compute the challenge: e = H(ChallengeHashTag, R || Q || m).
	var challengeMsg bytes.Buffer
	challengeMsg.Write(schnorr.SerializePubKey(nonceKey))
	challengeMsg.Write(schnorr.SerializePubKey(combinedKey.FinalKey))
	challengeMsg.Write(msg[:])
	challengeBytes := chainhash.TaggedHash(
		ChallengeHashTag, challengeMsg.Bytes(),
	)
	var e btcec.ModNScalar
	e.SetByteSlice(challengeBytes[:])

	// Compute č = e * ∏ a_{1,d} (challenge times product of aggregation
	// coefficients across all levels).
	e.Mul(aProduct)

	// Compute the partial signature:
	//   s = k1 + b̌ * k2 + č * d
	// where b̌ = ∏ b_d (product of all nonce blinds).
	s := new(btcec.ModNScalar)
	s.Add(&k1).Add(k2.Mul(bProduct)).Add(e.Mul(&privKeyScalar))

	sig := NewPartialSignature(s, nonceKey)

	return &sig, nil
}

// VerifyNestedPartialSig verifies a partial signature produced by a nested
// MuSig2 signer. It checks that s*G == R̂ + č*g*P, where R̂ is the signer's
// effective nonce (blinded by the product of all nonce blinding factors), č is
// the challenge multiplied by the product of aggregation coefficients across
// all nesting levels, g is the parity factor, and P is the signer's public key.
//
// Parameters:
//   - partialSig: the partial signature to verify.
//   - pubNonce: the signer's individual public nonce (from GenNonces).
//   - combinedNonce: the top-level aggregate nonce.
//   - pubKeys: the top-level set of public keys (depth 0).
//   - signingKey: the signer's public key (must be in the deepest nesting
//     level).
//   - msg: the 32-byte message that was signed.
//   - nestingLevels: the nesting levels (same as passed to NestedSign).
//   - signOpts: optional signing options (tweaks, etc.) applied to the
//     top-level key aggregation.
func VerifyNestedPartialSig(partialSig *PartialSignature,
	pubNonce [PubNonceSize]byte, combinedNonce [PubNonceSize]byte,
	pubKeys []*btcec.PublicKey, signingKey *btcec.PublicKey,
	msg [32]byte, nestingLevels []NestingLevel,
	signOpts ...SignOption) bool {

	return verifyNestedPartialSig(
		partialSig, pubNonce, combinedNonce, pubKeys, signingKey,
		msg, nestingLevels, signOpts...,
	) == nil
}

// verifyNestedPartialSig is the internal implementation of
// VerifyNestedPartialSig that returns detailed errors.
func verifyNestedPartialSig(partialSig *PartialSignature,
	pubNonce [PubNonceSize]byte, combinedNonce [PubNonceSize]byte,
	pubKeys []*btcec.PublicKey, signingKey *btcec.PublicKey,
	msg [32]byte, nestingLevels []NestingLevel,
	signOpts ...SignOption) error {

	if len(nestingLevels) == 0 {
		return ErrNoNestingLevels
	}

	opts := defaultSignOptions()
	for _, option := range signOpts {
		option(opts)
	}

	s := partialSig.S

	// Walk from the deepest level upward, computing the product of
	// aggregation coefficients and nonce blinding factors (same as in
	// NestedSign).
	aProduct := new(btcec.ModNScalar).SetInt(1)
	bProduct := new(btcec.ModNScalar).SetInt(1)

	currentKey := signingKey
	for i := len(nestingLevels) - 1; i >= 0; i-- {
		level := nestingLevels[i]

		keysHash := keyHashFingerprint(level.PubKeys, false)
		uniqueKeyIdx := secondUniqueKeyIndex(level.PubKeys, false)
		a := aggregationCoefficient(
			level.PubKeys, currentKey, keysHash, uniqueKeyIdx,
		)
		aProduct.Mul(a)

		aggKey, _, _, err := AggregateKeys(level.PubKeys, false)
		if err != nil {
			return err
		}

		b := computeNestedNonceBlind(aggKey.FinalKey, level.AggNonce)
		bProduct.Mul(b)

		currentKey = aggKey.FinalKey
	}

	// Top-level aggregation coefficient.
	keysHash := keyHashFingerprint(pubKeys, opts.sortKeys)
	uniqueKeyIdx := secondUniqueKeyIndex(pubKeys, opts.sortKeys)
	a := aggregationCoefficient(
		pubKeys, currentKey, keysHash, uniqueKeyIdx,
	)
	aProduct.Mul(a)

	// Top-level key aggregation with tweaks.
	keyAggOpts := []KeyAggOption{
		WithKeysHash(keysHash), WithUniqueKeyIndex(uniqueKeyIdx),
	}
	switch {
	case opts.bip86Tweak:
		keyAggOpts = append(keyAggOpts, WithBIP86KeyTweak())
	case opts.taprootTweak != nil:
		keyAggOpts = append(
			keyAggOpts, WithTaprootKeyTweak(opts.taprootTweak),
		)
	case len(opts.tweaks) != 0:
		keyAggOpts = append(keyAggOpts, WithKeyTweaks(opts.tweaks...))
	}

	combinedKey, parityAcc, _, err := AggregateKeys(
		pubKeys, opts.sortKeys, keyAggOpts...,
	)
	if err != nil {
		return err
	}

	// Compute the top-level nonce blinding factor and combined nonce R.
	var (
		nonceMsgBuf  bytes.Buffer
		nonceBlinder btcec.ModNScalar
	)
	nonceMsgBuf.Write(combinedNonce[:])
	nonceMsgBuf.Write(schnorr.SerializePubKey(combinedKey.FinalKey))
	nonceMsgBuf.Write(msg[:])
	nonceBlindHash := chainhash.TaggedHash(
		NonceBlindTag, nonceMsgBuf.Bytes(),
	)
	nonceBlinder.SetByteSlice(nonceBlindHash[:])
	bProduct.Mul(&nonceBlinder)

	// Parse the top-level combined nonce points to compute R.
	r1J, err := btcec.ParseJacobian(
		combinedNonce[:btcec.PubKeyBytesLenCompressed],
	)
	if err != nil {
		return err
	}
	r2J, err := btcec.ParseJacobian(
		combinedNonce[btcec.PubKeyBytesLenCompressed:],
	)
	if err != nil {
		return err
	}

	var nonce btcec.JacobianPoint
	btcec.ScalarMultNonConst(&nonceBlinder, &r2J, &r2J)
	btcec.AddNonConst(&r1J, &r2J, &nonce)

	if nonce == infinityPoint {
		btcec.GeneratorJacobian(&nonce)
	} else {
		nonce.ToAffine()
	}

	// Parse the signer's individual public nonces.
	pubNonce1J, err := btcec.ParseJacobian(
		pubNonce[:btcec.PubKeyBytesLenCompressed],
	)
	if err != nil {
		return err
	}
	pubNonce2J, err := btcec.ParseJacobian(
		pubNonce[btcec.PubKeyBytesLenCompressed:],
	)
	if err != nil {
		return err
	}

	// Compute the signer's effective nonce: R̂ = R'_1 + b̌ * R'_2,
	// where b̌ is the product of all nonce blinding factors.
	var pubNonceJ btcec.JacobianPoint
	btcec.ScalarMultNonConst(bProduct, &pubNonce2J, &pubNonce2J)
	btcec.AddNonConst(&pubNonce1J, &pubNonce2J, &pubNonceJ)
	pubNonceJ.ToAffine()

	// If the combined nonce R has odd y, negate the effective nonce.
	if nonce.Y.IsOdd() {
		pubNonceJ.Y.Negate(1)
		pubNonceJ.Y.Normalize()
	}

	// Compute the challenge: e = H(ChallengeHashTag, R || Q || m).
	var challengeMsg bytes.Buffer
	challengeMsg.Write(schnorr.SerializePubKey(btcec.NewPublicKey(
		&nonce.X, &nonce.Y,
	)))
	challengeMsg.Write(schnorr.SerializePubKey(combinedKey.FinalKey))
	challengeMsg.Write(msg[:])
	challengeBytes := chainhash.TaggedHash(
		ChallengeHashTag, challengeMsg.Bytes(),
	)
	var e btcec.ModNScalar
	e.SetByteSlice(challengeBytes[:])

	// Compute č = e * ∏ a_{1,d}.
	e.Mul(aProduct)

	// Compute parity factor: g * gacc.
	parityCombinedKey := new(btcec.ModNScalar).SetInt(1)
	combinedKeyBytes := combinedKey.FinalKey.SerializeCompressed()
	if combinedKeyBytes[0] == secp.PubKeyFormatCompressedOdd {
		parityCombinedKey.Negate()
	}
	finalParityFactor := parityCombinedKey.Mul(parityAcc)

	var signKeyJ btcec.JacobianPoint
	signingKey.AsJacobian(&signKeyJ)

	// Verify: s*G == R̂ + č*g*P.
	var sG, rP btcec.JacobianPoint
	btcec.ScalarBaseMultNonConst(s, &sG)
	btcec.ScalarMultNonConst(
		e.Mul(finalParityFactor), &signKeyJ, &rP,
	)
	btcec.AddNonConst(&rP, &pubNonceJ, &rP)

	sG.ToAffine()
	rP.ToAffine()

	if sG != rP {
		return ErrPartialSigInvalid
	}

	return nil
}

// AggregateNestedPartialSigs implements SignAgg' from the nested MuSig2 paper.
// It sums partial signature scalars from signers at the same nesting level to
// produce a single aggregate partial signature for the parent level.
func AggregateNestedPartialSigs(
	partialSigs []*PartialSignature) (*PartialSignature, error) {

	if len(partialSigs) == 0 {
		return nil, ErrNoPartialSigs
	}

	var combinedS btcec.ModNScalar
	for _, sig := range partialSigs {
		combinedS.Add(sig.S)
	}

	// All partial sigs at the same level share the same R point.
	result := NewPartialSignature(&combinedS, partialSigs[0].R)

	return &result, nil
}
