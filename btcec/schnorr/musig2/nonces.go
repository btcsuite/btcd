// Copyright 2013-2022 The btcsuite developers

package musig2

import (
	"crypto/rand"

	"github.com/btcsuite/btcd/btcec/v2"
)

const (
	// PubNonceSize is the size of the public nonces. Each public nonce is
	// serialized the full compressed encoding, which uses 32 bytes for each
	// nonce.
	PubNonceSize = 66

	// SecNonceSize is the size of the secret nonces for musig2. The secret
	// nonces are the corresponding private keys to the public nonce points.
	SecNonceSize = 64
)

// zeroSecNonce is a secret nonce that's all zeroes. This is used to check that
// we're not attempting to re-use a nonce, and also protect callers from it.
var zeroSecNonce [SecNonceSize]byte

// Nonces holds the public and secret nonces required for musig2.
//
// TODO(roasbeef): methods on this to help w/ parsing, etc?
type Nonces struct {
	// PubNonce holds the two 33-byte compressed encoded points that serve
	// as the public set of nonces.
	PubNonce [PubNonceSize]byte

	// SecNonce holds the two 32-byte scalar values that are the private
	// keys to the two public nonces.
	SecNonce [SecNonceSize]byte
}

// secNonceToPubNonce takes our two secrete nonces, and produces their two
// corresponding EC points, serialized in compressed format.
func secNonceToPubNonce(secNonce [SecNonceSize]byte) [PubNonceSize]byte {
	var k1Mod, k2Mod btcec.ModNScalar
	k1Mod.SetByteSlice(secNonce[:btcec.PrivKeyBytesLen])
	k2Mod.SetByteSlice(secNonce[btcec.PrivKeyBytesLen:])

	var r1, r2 btcec.JacobianPoint
	btcec.ScalarBaseMultNonConst(&k1Mod, &r1)
	btcec.ScalarBaseMultNonConst(&k2Mod, &r2)

	// Next, we'll convert the key in jacobian format to a normal public
	// key expressed in affine coordinates.
	r1.ToAffine()
	r2.ToAffine()
	r1Pub := btcec.NewPublicKey(&r1.X, &r1.Y)
	r2Pub := btcec.NewPublicKey(&r2.X, &r2.Y)

	var pubNonce [PubNonceSize]byte

	// The public nonces are serialized as: R1 || R2, where both keys are
	// serialized in compressed format.
	copy(pubNonce[:], r1Pub.SerializeCompressed())
	copy(
		pubNonce[btcec.PubKeyBytesLenCompressed:],
		r2Pub.SerializeCompressed(),
	)

	return pubNonce
}

// NonceGenOption is a function option that allows callers to modify how nonce
// generation happens.
type NonceGenOption func(*nonceGenOpts)

// nonceGenOpts is the set of options that control how nonce generation happens.
type nonceGenOpts struct {
	randReader func(b []byte) (int, error)
}

// defaultNonceGenOpts returns the default set of nonce generation options.
func defaultNonceGenOpts() *nonceGenOpts {
	return &nonceGenOpts{
		// By default, we always use the crypto/rand reader, but the
		// caller is able to specify their own generation, which can be
		// useful for deterministic tests.
		randReader: rand.Read,
	}
}

// GenNonces generates the secret nonces, as well as the public nonces which
// correspond to an EC point generated using the secret nonce as a private key.
func GenNonces(options ...NonceGenOption) (*Nonces, error) {
	opts := defaultNonceGenOpts()
	for _, opt := range options {
		opt(opts)
	}

	// Generate two 32-byte random values that'll be the private keys to
	// the public nonces.
	var k1, k2 [32]byte
	if _, err := opts.randReader(k1[:]); err != nil {
		return nil, err
	}
	if _, err := opts.randReader(k2[:]); err != nil {
		return nil, err
	}

	var nonces Nonces

	var k1Mod, k2Mod btcec.ModNScalar
	k1Mod.SetBytes(&k1)
	k2Mod.SetBytes(&k2)

	// The secret nonces are serialized as the concatenation of the two 32
	// byte secret nonce values.
	k1Mod.PutBytesUnchecked(nonces.SecNonce[:])
	k2Mod.PutBytesUnchecked(nonces.SecNonce[btcec.PrivKeyBytesLen:])

	// Next, we'll generate R_1 = k_1*G and R_2 = k_2*G. Along the way we
	// need to map our nonce values into mod n scalars so we can work with
	// the btcec API.
	nonces.PubNonce = secNonceToPubNonce(nonces.SecNonce)

	return &nonces, nil
}

// AggregateNonces aggregates the set of a pair of public nonces for each party
// into a single aggregated nonces to be used for multi-signing.
func AggregateNonces(pubNonces [][PubNonceSize]byte) ([PubNonceSize]byte, error) {
	// combineNonces is a helper function that aggregates (adds) up a
	// series of nonces encoded in compressed format. It uses a slicing
	// function to extra 33 bytes at a time from the packed 2x public
	// nonces.
	type nonceSlicer func([PubNonceSize]byte) []byte
	combineNonces := func(slicer nonceSlicer) (*btcec.PublicKey, error) {
		// Convert the set of nonces into jacobian coordinates we can
		// use to accumulate them all into each other.
		pubNonceJs := make([]*btcec.JacobianPoint, len(pubNonces))
		for i, pubNonceBytes := range pubNonces {
			// Using the slicer, extract just the bytes we need to
			// decode.
			var nonceJ btcec.JacobianPoint
			pubNonce, err := btcec.ParsePubKey(
				slicer(pubNonceBytes),
			)
			if err != nil {
				return nil, err
			}

			pubNonce.AsJacobian(&nonceJ)
			pubNonceJs[i] = &nonceJ
		}

		// Now that we have the set of complete nonces, we'll aggregate
		// them: R = R_i + R_i+1 + ... + R_i+n.
		var aggregateNonce btcec.JacobianPoint
		for _, pubNonceJ := range pubNonceJs {
			btcec.AddNonConst(
				&aggregateNonce, pubNonceJ, &aggregateNonce,
			)
		}

		// Now that we've aggregated all the points, we need to check
		// if this point is the point at infinity, if so, then we'll
		// just return the generator. At a later step, the malicious
		// party will be detected.
		if aggregateNonce == infinityPoint {
			// TODO(roasbeef): better way to get the generator w/
			// the new API? -- via old curve params instead?
			var generator btcec.JacobianPoint
			one := new(btcec.ModNScalar).SetInt(1)
			btcec.ScalarBaseMultNonConst(one, &generator)

			generator.ToAffine()
			return btcec.NewPublicKey(
				&generator.X, &generator.Y,
			), nil
		}

		aggregateNonce.ToAffine()
		return btcec.NewPublicKey(
			&aggregateNonce.X, &aggregateNonce.Y,
		), nil
	}

	// The final nonce public nonce is actually two nonces, one that
	// aggregate the first nonce of all the parties, and the other that
	// aggregates the second nonce of all the parties.
	var finalNonce [PubNonceSize]byte
	combinedNonce1, err := combineNonces(func(n [PubNonceSize]byte) []byte {
		return n[:btcec.PubKeyBytesLenCompressed]
	})
	if err != nil {
		return finalNonce, err
	}
	combinedNonce2, err := combineNonces(func(n [PubNonceSize]byte) []byte {
		return n[btcec.PubKeyBytesLenCompressed:]
	})
	if err != nil {
		return finalNonce, err
	}

	copy(finalNonce[:], combinedNonce1.SerializeCompressed())
	copy(
		finalNonce[btcec.PubKeyBytesLenCompressed:],
		combinedNonce2.SerializeCompressed(),
	)

	return finalNonce, nil
}
