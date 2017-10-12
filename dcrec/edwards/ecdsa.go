// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package edwards

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"math/big"

	"github.com/agl/ed25519"
	"github.com/agl/ed25519/edwards25519"
)

// BIG CAVEAT
// Memory management is kind of sloppy and whether or not your keys or
// nonces can be found in memory later is likely a product of when the
// garbage collector runs.
// Signing/EC mult is also not constant side, so don't use this in any
// application where you think you might be vulnerable to side channel
// attacks.

var (
	// oneInitializer is used to fill a byte slice with byte 0x01.  It is provided
	// here to avoid the need to create it multiple times.
	oneInitializer = []byte{0x01}

	// ecTypeEdwards is the ECDSA type for the chainec interface.
	ecTypeEdwards = 1
)

// GenerateKey generates a key using a random number generator, returning
// the private scalar and the corresponding public key points from a
// random secret.
func GenerateKey(curve *TwistedEdwardsCurve, rand io.Reader) (priv []byte, x,
	y *big.Int, err error) {
	var pub *[PubKeyBytesLen]byte
	var privArray *[PrivKeyBytesLen]byte
	pub, privArray, err = ed25519.GenerateKey(rand)
	if err != nil {
		return nil, nil, nil, err
	}
	priv = privArray[:]

	x, y, err = curve.EncodedBytesToBigIntPoint(pub)
	if err != nil {
		return nil, nil, nil, err
	}

	return
}

// SignFromSecret signs a message 'hash' using the given private key priv. It doesn't
// actually user the random reader (the lib is maybe deterministic???).
func SignFromSecret(rand io.Reader, priv *PrivateKey, hash []byte) (r, s *big.Int,
	err error) {
	r, s, err = SignFromSecretNoReader(priv, hash)

	return
}

// SignFromSecretNoReader signs a message 'hash' using the given private key
// priv. It doesn't actually user the random reader.
func SignFromSecretNoReader(priv *PrivateKey, hash []byte) (r, s *big.Int,
	err error) {
	privBytes := priv.SerializeSecret()
	privArray := copyBytes64(privBytes)
	sig := ed25519.Sign(privArray, hash)

	// The signatures are encoded as
	//   sig[0:32]  R, a point encoded as little endian
	//   sig[32:64] S, scalar multiplication/addition results = (ab+c) mod l
	//     encoded also as little endian
	rBytes := copyBytes(sig[0:32])
	r = EncodedBytesToBigInt(rBytes)
	sBytes := copyBytes(sig[32:64])
	s = EncodedBytesToBigInt(sBytes)

	return
}

// nonceRFC6979 is a local instatiation of deterministic nonce generation
// by the standards of RFC6979.
func nonceRFC6979(curve *TwistedEdwardsCurve, privkey []byte, hash []byte,
	extra []byte, version []byte) []byte {
	pkD := new(big.Int).SetBytes(privkey)
	defer pkD.SetInt64(0)
	bigK := NonceRFC6979(curve, pkD, hash, extra, version)
	defer bigK.SetInt64(0)
	k := BigIntToEncodedBytesNoReverse(bigK)
	return k[:]
}

// NonceRFC6979 generates an ECDSA nonce (`k`) deterministically according to
// RFC 6979. It takes a 32-byte hash as an input and returns 32-byte nonce to
// be used in ECDSA algorithm.
func NonceRFC6979(curve *TwistedEdwardsCurve, privkey *big.Int, hash []byte,
	extra []byte, version []byte) *big.Int {
	q := curve.Params().N
	x := privkey
	alg := sha256.New

	qlen := q.BitLen()
	holen := alg().Size()
	rolen := (qlen + 7) >> 3
	bx := append(int2octets(x, rolen), bits2octets(hash, curve, rolen)...)
	if len(extra) == 32 {
		bx = append(bx, extra...)
	}
	if len(version) == 16 && len(extra) == 32 {
		bx = append(bx, extra...)
	}
	if len(version) == 16 && len(extra) != 32 {
		bx = append(bx, bytes.Repeat([]byte{0x00}, 32)...)
		bx = append(bx, version...)
	}

	// Step B
	v := bytes.Repeat(oneInitializer, holen)

	// Step C (Go zeroes the all allocated memory)
	k := make([]byte, holen)

	// Step D
	k = mac(alg, k, append(append(v, 0x00), bx...))

	// Step E
	v = mac(alg, k, v)

	// Step F
	k = mac(alg, k, append(append(v, 0x01), bx...))

	// Step G
	v = mac(alg, k, v)

	// Step H
	for {
		// Step H1
		var t []byte

		// Step H2
		for len(t)*8 < qlen {
			v = mac(alg, k, v)
			t = append(t, v...)
		}

		// Step H3
		secret := hashToInt(t, curve)
		if secret.Cmp(one) >= 0 && secret.Cmp(q) < 0 {
			return secret
		}
		k = mac(alg, k, append(v, 0x00))
		v = mac(alg, k, v)
	}
}

// hashToInt converts a hash value to an integer. There is some disagreement
// about how this is done. [NSA] suggests that this is done in the obvious
// manner, but [SECG] truncates the hash to the bit-length of the curve order
// first. We follow [SECG] because that's what OpenSSL does. Additionally,
// OpenSSL right shifts excess bits from the number if the hash is too large
// and we mirror that too.
// This is borrowed from crypto/ecdsa.
func hashToInt(hash []byte, c *TwistedEdwardsCurve) *big.Int {
	orderBits := c.Params().N.BitLen()
	orderBytes := (orderBits + 7) / 8
	if len(hash) > orderBytes {
		hash = hash[:orderBytes]
	}

	ret := new(big.Int).SetBytes(hash)
	excess := len(hash)*8 - orderBits
	if excess > 0 {
		ret.Rsh(ret, uint(excess))
	}
	return ret
}

// mac returns an HMAC of the given key and message.
func mac(alg func() hash.Hash, k, m []byte) []byte {
	h := hmac.New(alg, k)
	h.Write(m)
	return h.Sum(nil)
}

// https://tools.ietf.org/html/rfc6979#section-2.3.3
func int2octets(v *big.Int, rolen int) []byte {
	out := v.Bytes()

	// left pad with zeros if it's too short
	if len(out) < rolen {
		out2 := make([]byte, rolen)
		copy(out2[rolen-len(out):], out)
		return out2
	}

	// drop most significant bytes if it's too long
	if len(out) > rolen {
		out2 := make([]byte, rolen)
		copy(out2, out[len(out)-rolen:])
		return out2
	}

	return out
}

// https://tools.ietf.org/html/rfc6979#section-2.3.4
func bits2octets(in []byte, curve *TwistedEdwardsCurve, rolen int) []byte {
	z1 := hashToInt(in, curve)
	z2 := new(big.Int).Sub(z1, curve.Params().N)
	if z2.Sign() < 0 {
		return int2octets(z1, rolen)
	}
	return int2octets(z2, rolen)
}

// SignFromScalar signs a message 'hash' using the given private scalar priv.
// It uses RFC6979 to generate a deterministic nonce. Considered experimental.
// r = kG, where k is the RFC6979 nonce
// s = r + hash512(k || A || M) * a
func SignFromScalar(curve *TwistedEdwardsCurve, priv *PrivateKey,
	nonce []byte, hash []byte) (r, s *big.Int, err error) {
	publicKey := new([PubKeyBytesLen]byte)
	var A edwards25519.ExtendedGroupElement
	privateScalar := copyBytes(priv.Serialize())
	reverse(privateScalar) // BE --> LE
	edwards25519.GeScalarMultBase(&A, privateScalar)
	A.ToBytes(publicKey)

	// For signing from a scalar, r = nonce.
	nonceLE := copyBytes(nonce)
	reverse(nonceLE)
	var R edwards25519.ExtendedGroupElement
	edwards25519.GeScalarMultBase(&R, nonceLE)

	var encodedR [32]byte
	R.ToBytes(&encodedR)

	// h = hash512(k || A || M)
	h := sha512.New()
	h.Reset()
	h.Write(encodedR[:])
	h.Write(publicKey[:])
	h.Write(hash)

	// s = r + h * a
	var hramDigest [64]byte
	h.Sum(hramDigest[:0])
	var hramDigestReduced [32]byte
	edwards25519.ScReduce(&hramDigestReduced, &hramDigest)

	var localS [32]byte
	edwards25519.ScMulAdd(&localS, &hramDigestReduced, privateScalar,
		nonceLE)

	signature := new([64]byte)
	copy(signature[:], encodedR[:])
	copy(signature[32:], localS[:])
	sigEd, err := ParseSignature(curve, signature[:])
	if err != nil {
		return nil, nil, err
	}

	return sigEd.GetR(), sigEd.GetS(), nil
}

// SignThreshold signs a message 'hash' using the given private scalar priv in
// a threshold group signature. It uses RFC6979 to generate a deterministic nonce.
// Considered experimental.
// As opposed to the threshold signing function for secp256k1, this function
// takes the entirety of the public nonce point (all points added) instead of
// the public nonce point with n-1 keys added.
// r = K_Sum
// s = r + hash512(k || A || M) * a
func SignThreshold(curve *TwistedEdwardsCurve, priv *PrivateKey,
	groupPub *PublicKey, hash []byte, privNonce *PrivateKey,
	pubNonceSum *PublicKey) (r, s *big.Int, err error) {
	if priv == nil || hash == nil || privNonce == nil || pubNonceSum == nil {
		return nil, nil, fmt.Errorf("nil input")
	}

	privateScalar := copyBytes(priv.Serialize())
	reverse(privateScalar) // BE --> LE

	// Threshold variant scheme:
	// R = K_Sum
	// Where K_Sum is the sum of the public keys corresponding to
	// the private nonce scalars of each group signature member.
	// That is, R = k1G + ... + knG.
	encodedGroupR := BigIntPointToEncodedBytes(pubNonceSum.GetX(),
		pubNonceSum.GetY())

	// h = hash512(k || A || M)
	var hramDigest [64]byte
	h := sha512.New()
	h.Reset()
	h.Write(encodedGroupR[:])
	h.Write(groupPub.Serialize()[:])
	h.Write(hash)
	h.Sum(hramDigest[:0])
	var hramDigestReduced [32]byte
	edwards25519.ScReduce(&hramDigestReduced, &hramDigest)

	// s = r + h * a
	var localS [32]byte
	privNonceLE := copyBytes(privNonce.Serialize())
	reverse(privNonceLE) // BE --> LE
	edwards25519.ScMulAdd(&localS, &hramDigestReduced, privateScalar,
		privNonceLE)

	signature := new([64]byte)
	copy(signature[:], encodedGroupR[:])
	copy(signature[32:], localS[:])
	sigEd, err := ParseSignature(curve, signature[:])
	if err != nil {
		return nil, nil, err
	}

	return sigEd.GetR(), sigEd.GetS(), nil
}

// Sign is the generalized and exported version of Ed25519 signing, that
// handles both standard private secrets and non-standard scalars.
func Sign(curve *TwistedEdwardsCurve, priv *PrivateKey, hash []byte) (r,
	s *big.Int, err error) {
	if priv == nil {
		return nil, nil, fmt.Errorf("private key is nil")
	}
	if hash == nil {
		return nil, nil, fmt.Errorf("message key is nil")
	}

	if priv.secret == nil {
		privLE := copyBytes(priv.Serialize())
		reverse(privLE)
		nonce := nonceRFC6979(curve, privLE[:], hash, nil, nil)
		return SignFromScalar(curve, priv, nonce, hash)
	}

	return SignFromSecretNoReader(priv, hash)
}

// Verify verifies a message 'hash' using the given public keys and signature.
func Verify(pub *PublicKey, hash []byte, r, s *big.Int) bool {
	if pub == nil || hash == nil || r == nil || s == nil {
		return false
	}

	pubBytes := pub.Serialize()
	sig := &Signature{r, s}
	sigBytes := sig.Serialize()
	pubArray := copyBytes(pubBytes)
	sigArray := copyBytes64(sigBytes)
	return ed25519.Verify(pubArray, hash, sigArray)
}
