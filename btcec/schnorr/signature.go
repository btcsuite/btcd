// Copyright (c) 2013-2022 The btcsuite developers

package schnorr

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	ecdsa_schnorr "github.com/decred/dcrd/dcrec/secp256k1/v4/schnorr"
)

const (
	// SignatureSize is the size of an encoded Schnorr signature.
	SignatureSize = 64

	// scalarSize is the size of an encoded big endian scalar.
	scalarSize = 32
)

// Signature is a type representing a Schnorr signature.
type Signature struct {
	r btcec.FieldVal
	s btcec.ModNScalar
}

// NewSignature instantiates a new signature given some r and s values.
func NewSignature(r *btcec.FieldVal, s *btcec.ModNScalar) *Signature {
	var sig Signature
	sig.r.Set(r).Normalize()
	sig.s.Set(s)
	return &sig
}

// Serialize returns the Schnorr signature in the more strict format.
//
// The signatures are encoded as
//
//	sig[0:32]  x coordinate of the point R, encoded as a big-endian uint256
//	sig[32:64] s, encoded also as big-endian uint256
func (sig Signature) Serialize() []byte {
	// Total length of returned signature is the length of r and s.
	var b [SignatureSize]byte
	sig.r.PutBytesUnchecked(b[0:32])
	sig.s.PutBytesUnchecked(b[32:64])
	return b[:]
}

// ParseSignature parses a signature according to the BIP-340 specification and
// enforces the following additional restrictions specific to secp256k1:
//
// - The r component must be in the valid range for secp256k1 field elements
// - The s component must be in the valid range for secp256k1 scalars
func ParseSignature(sig []byte) (*Signature, error) {
	// The signature must be the correct length.
	sigLen := len(sig)
	if sigLen < SignatureSize {
		str := fmt.Sprintf("malformed signature: too short: %d < %d",
			sigLen, SignatureSize)
		return nil, signatureError(ecdsa_schnorr.ErrSigTooShort, str)
	}
	if sigLen > SignatureSize {
		str := fmt.Sprintf("malformed signature: too long: %d > %d",
			sigLen, SignatureSize)
		return nil, signatureError(ecdsa_schnorr.ErrSigTooLong, str)
	}

	// The signature is validly encoded at this point, however, enforce
	// additional restrictions to ensure r is in the range [0, p-1], and s
	// is in the range [0, n-1] since valid Schnorr signatures are required
	// to be in that range per spec.
	var r btcec.FieldVal
	if overflow := r.SetByteSlice(sig[0:32]); overflow {
		str := "invalid signature: r >= field prime"
		return nil, signatureError(ecdsa_schnorr.ErrSigRTooBig, str)
	}
	var s btcec.ModNScalar
	s.SetByteSlice(sig[32:64])

	// Return the signature.
	return NewSignature(&r, &s), nil
}

// IsEqual compares this Signature instance to the one passed, returning true
// if both Signatures are equivalent. A signature is equivalent to another, if
// they both have the same scalar value for R and S.
func (sig Signature) IsEqual(otherSig *Signature) bool {
	return sig.r.Equals(&otherSig.r) && sig.s.Equals(&otherSig.s)
}

// schnorrVerify attempt to verify the signature for the provided message and
// secp256k1 public key and either returns nil if successful or a specific error
// indicating why it failed if not successful.
//
// This differs from the exported Verify method in that it returns a specific
// error to support better testing while the exported method simply returns a
// bool indicating success or failure.
func schnorrVerify(sig *Signature, message []byte, pubKeyBytes []byte) error {
	// The algorithm for verifying a BIP-340 signature:
	//
	// 1. P = lift_x(int(pk)); fail if if that fails.
	// 2. r = int(sig[0:32]); fail is r >= p.
	// 3. s = int(sig[32:64]); fail if s >= n.
	// 4. e = int(tagged_hash("BIP0340/challenge", bytes(r) || bytes(P) || m)) mod n.
	// 5. R = s*G - e*P
	// 6. Fail if is_infinite(R)
	// 7. Fail if not hash_even_y(R)
	// 8. Fail is x(R) != r.
	// 9. Return success iff failure did not occur before reaching this point.

	// Step 1.
	//
	// P = lift_x(int(pk))
	//
	// Fail if P is not a point on the curve
	pubKey, err := ParsePubKey(pubKeyBytes)
	if err != nil {
		return err
	}
	if !pubKey.IsOnCurve() {
		str := "pubkey point is not on curve"
		return signatureError(ecdsa_schnorr.ErrPubKeyNotOnCurve, str)
	}

	// Step 2.
	//
	// Fail if r >= p
	//
	// Note this is already handled by the fact r is a field element.

	// Step 3.
	//
	// Fail if s >= n
	//
	// Note this is already handled by the fact s is a mod n scalar.

	// Step 4.
	//
	// e = int(tagged_hash("BIP0340/challenge", bytes(r) || bytes(P) || M)) mod n.
	var rBytes [32]byte
	sig.r.PutBytesUnchecked(rBytes[:])
	pBytes := SerializePubKey(pubKey)

	commitment := chainhash.TaggedHash(
		chainhash.TagBIP0340Challenge, rBytes[:], pBytes, message,
	)

	var e btcec.ModNScalar
	e.SetBytes((*[32]byte)(commitment))

	// Negate e here so we can use AddNonConst below to subtract the s*G
	// point from e*P.
	e.Negate()

	// Step 5.
	//
	// R = s*G - e*P
	var P, R, sG, eP btcec.JacobianPoint
	pubKey.AsJacobian(&P)
	btcec.ScalarBaseMultNonConst(&sig.s, &sG)
	btcec.ScalarMultNonConst(&e, &P, &eP)
	btcec.AddNonConst(&sG, &eP, &R)

	// Step 6.
	//
	// Fail if R is the point at infinity
	if (R.X.IsZero() && R.Y.IsZero()) || R.Z.IsZero() {
		str := "calculated R point is the point at infinity"
		return signatureError(ecdsa_schnorr.ErrSigRNotOnCurve, str)
	}

	// Step 7.
	//
	// Fail if R.y is odd
	//
	// Note that R must be in affine coordinates for this check.
	R.ToAffine()
	if R.Y.IsOdd() {
		str := "calculated R y-value is odd"
		return signatureError(ecdsa_schnorr.ErrSigRYIsOdd, str)
	}

	// Step 8.
	//
	// Verified if R.x == r
	//
	// Note that R must be in affine coordinates for this check.
	if !sig.r.Equals(&R.X) {
		str := "calculated R point was not given R"
		return signatureError(ecdsa_schnorr.ErrUnequalRValues, str)
	}

	// Step 9.
	//
	// Return success iff failure did not occur before reaching this point.
	return nil
}

// Verify returns whether or not the signature is valid for the provided message
// and secp256k1 public key.
func (sig *Signature) Verify(message []byte, pubKey *btcec.PublicKey) bool {
	pubkeyBytes := SerializePubKey(pubKey)
	return schnorrVerify(sig, message, pubkeyBytes) == nil
}

// schnorrSign generates a BIP-340 signature over the secp256k1 curve for the
// provided message using the given nonce and private key.  The produced
// signature is deterministic (same message, nonce, and key yield the same
// signature) and canonical.
//
// WARNING: Both the nonce and private keys must NOT be 0. Since this is an
// internal use function, these preconditions MUST be satisfied by the caller.
func schnorrSign(privKey, nonce *btcec.ModNScalar, pubKey *btcec.PublicKey,
	message []byte, opts *signOptions) (*Signature, error) {

	// The algorithm for producing a BIP-340 signature:
	//
	// G = curve generator
	// n = curve order
	// d = private key
	// m = message
	// a = auxiliary random data
	// r, s = signature
	//
	// 1. d' = int(d)
	// 2. Fail if d = 0 or d >= n
	// 3. P = d'*G
	// 4. Negate d if P.y is odd
	// 5. t = bytes(d) xor tagged_hash("BIP0340/aux", a)
	// 6. rand = tagged_hash("BIP0340/nonce", t || bytes(P) || m)
	// 7. k' = int(rand) mod n
	// 8. Fail if k' = 0
	// 9. R = 'k*G
	// 10. Negate k if R.y id odd
	// 11. e = tagged_hash("BIP0340/challenge", bytes(R) || bytes(P) || m) mod n
	// 12. sig = bytes(R) || bytes((k + e*d)) mod n
	// 13. If Verify(bytes(P), m, sig) fails, abort.
	// 14. return sig.
	//
	// Note that the set of functional options passed in may modify the
	// above algorithm. If AuxRand is used, steps 6-8 follow BIP-340's
	// nonce derivation using the provided auxiliary randomness. If
	// AuxRand is NOT used (the default), we match libsecp256k1 behavior
	// by using the same algorithm but with auxRand being a zeroed 32 byte
	// array, which is allowed by BIP-340. If FastSign is passed, we skip
	// step 14 (signature verification).

	// NOTE: Steps 1-8 are performed by the caller.

	//
	// Step 9.
	//
	// R = kG
	var R btcec.JacobianPoint
	k := *nonce
	btcec.ScalarBaseMultNonConst(&k, &R)

	// Step 10.
	//
	// Negate nonce k if R.y is odd (R.y is the y coordinate of the point R)
	//
	// Note that R must be in affine coordinates for this check.
	R.ToAffine()
	if R.Y.IsOdd() {
		k.Negate()
	}

	// Step 11.
	//
	// e = tagged_hash("BIP0340/challenge", bytes(R) || bytes(P) || m) mod n
	pBytes := SerializePubKey(pubKey)
	commitment := chainhash.TaggedHash(
		chainhash.TagBIP0340Challenge, R.X.Bytes()[:], pBytes, message,
	)

	var e btcec.ModNScalar
	if overflow := e.SetBytes((*[32]byte)(commitment)); overflow != 0 {
		k.Zero()
		str := "hash of (r || P || m) too big"
		return nil, signatureError(ecdsa_schnorr.ErrSchnorrHashValue,
			str)
	}

	// Step 12.
	//
	// s = k + e*d mod n
	s := new(btcec.ModNScalar).Mul2(&e, privKey).Add(&k)
	k.Zero()

	sig := NewSignature(&R.X, s)

	// Step 13.
	//
	// If Verify(bytes(P), m, sig) fails, abort.
	if !opts.fastSign {
		if err := schnorrVerify(sig, message, pBytes); err != nil {
			return nil, err
		}
	}

	// Step 14.
	//
	// Return (r, s)
	return sig, nil
}

// SignOption is a functional option argument that allows callers to modify the
// way we generate BIP-340 schnorr signatures.
type SignOption func(*signOptions)

// signOptions houses the set of functional options that can be used to modify
// the method used to generate the BIP-340 signature.
type signOptions struct {
	// fastSign determines if we'll skip the check at the end of the routine
	// where we attempt to verify the produced signature.
	fastSign bool

	// auxRand allows the user to pass in their own auxiliary random data,
	// which is useful for schemes like mu-sig.
	auxRand *[32]byte
}

// defaultSignOptions returns the default set of signing operations.
func defaultSignOptions() *signOptions {
	return &signOptions{}
}

// FastSign forces signing to skip the extra verification step at the end.
// Performance sensitive applications may opt to use this option to speed up the
// signing operation.
func FastSign() SignOption {
	return func(o *signOptions) {
		o.fastSign = true
	}
}

// Deprecated: CustomNonce was replaced by AuxRand, because the naming is,
// misleading, making one think that signing multiple messages with the same
// private key, same auxData and different message could lead to private key
// extraction, which is not the case, since data derived from the message is
// used to generate the nonce.
func CustomNonce(auxData [32]byte) SignOption {
	return func(o *signOptions) {
		o.auxRand = &auxData
	}
}

// AuxRand allows users to pass in a custom set of auxiliary data that's used as
// additional input randomness to generate the nonce used during signing. Users
// may want to specify this custom value when using multi-signatures schemes
// such as Mu-Sig2.
func AuxRand(auxRand [32]byte) SignOption {
	return func(o *signOptions) {
		o.auxRand = &auxRand
	}
}

// Sign generates an BIP-340 signature over the secp256k1 curve for the
// provided message using the given private key.  The produced signature is
// deterministic (same message and same key yield the same signature) and
// canonical.
//
// Note that the current signing implementation has a few remaining variable
// time aspects which make use of the private key and the generated nonce,
// which can expose the signer to constant time attacks.  As a result, this
// function should not be used in situations where there is the possibility of
// someone having EM field/cache/etc access.
func Sign(privKey *btcec.PrivateKey, message []byte,
	signOpts ...SignOption) (*Signature, error) {

	// First, parse the set of optional signing options.
	opts := defaultSignOptions()
	for _, option := range signOpts {
		option(opts)
	}

	// The algorithm for producing a BIP-340 signature:
	//
	// G = curve generator
	// n = curve order
	// d = private key
	// m = message
	// a = auxiliary random data
	// r, s = signature
	//
	// 1. d' = int(d)
	// 2. Fail if d = 0 or d >= n
	// 3. P = d'*G
	// 4. Negate d if P.y is odd
	// 5. t = bytes(d) xor tagged_hash("BIP0340/aux", a)
	// 6. rand = tagged_hash("BIP0340/nonce", t || bytes(P) || m)
	// 7. k' = int(rand) mod n
	// 8. Fail if k' = 0
	// 9. R = 'k*G
	// 10. Negate k if R.y id odd
	// 11. e = tagged_hash("BIP0340/challenge", bytes(R) || bytes(P) || m) mod n
	// 12. sig = bytes(R) || bytes((k + e*d)) mod n
	// 13. If Verify(bytes(P), m, sig) fails, abort.
	// 14. return sig.

	// Step 1.
	//
	// d' = int(d)
	var privKeyScalar btcec.ModNScalar
	privKeyScalar.Set(&privKey.Key)

	// Step 2.
	//
	// Fail if d = 0 or d >= n
	if privKeyScalar.IsZero() {
		str := "private key is zero"
		return nil, signatureError(ecdsa_schnorr.ErrPrivateKeyIsZero,
			str)
	}

	// Step 3.
	//
	// P = 'd*G
	pub := privKey.PubKey()

	// Step 4.
	//
	// Negate d if P.y is odd.
	pubKeyBytes := pub.SerializeCompressed()
	if pubKeyBytes[0] == secp.PubKeyFormatCompressedOdd {
		privKeyScalar.Negate()
	}

	// Step 5.
	//
	// t = bytes(d) xor tagged_hash("BIP0340/aux", a)
	//
	// When AuxRand is not passed in, to generate the same signature as
	// libsecp256k1 does, use a zeroed slice as the auxRand, otherwise, use
	// the auxRand provided by the passed AuxRand.
	//
	// There's no risk of private key leakage using the zeroed auxRand,
	// because the hash `t`, will be hashed with `pubKeyBytes` and `message`
	// to generate `rand` so, given the same private key and different
	// message, rand will be always the different, which prevents attacks
	// from nonce reuse.
	var auxRand [32]byte
	if opts.auxRand != nil {
		copy(auxRand[:], opts.auxRand[:])
	}
	privBytes := privKeyScalar.Bytes()
	defer clear(privBytes[:])
	t := chainhash.TaggedHash(chainhash.TagBIP0340Aux, auxRand[:])
	defer clear(t[:])
	for i := range t {
		t[i] ^= privBytes[i]
	}

	// Step 6.
	//
	// rand = tagged_hash("BIP0340/nonce", t || bytes(P) || m)
	//
	// We snip off the first byte of the serialized pubkey, as we
	// only need the x coordinate and not the marker byte.
	rand := chainhash.TaggedHash(chainhash.TagBIP0340Nonce, t[:],
		pubKeyBytes[1:], message)

	// Step 7.
	//
	// k'= int(rand) mod n
	var kPrime btcec.ModNScalar
	kPrime.SetBytes((*[32]byte)(rand))

	// Step 8.
	//
	// Fail if k' = 0
	if kPrime.IsZero() {
		str := "generated nonce is zero"
		return nil, signatureError(ecdsa_schnorr.ErrSchnorrHashValue,
			str)
	}

	// Steps 9-14.
	sig, err := schnorrSign(&privKeyScalar, &kPrime, pub, message, opts)
	kPrime.Zero()
	if err != nil {
		return nil, err
	}

	return sig, nil
}
