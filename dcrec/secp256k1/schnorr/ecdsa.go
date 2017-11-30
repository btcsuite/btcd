// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package schnorr

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec/secp256k1"
)

// scalarSize is the size of an encoded big endian scalar.
const scalarSize = 32

var (
	// bigZero is the big representation of zero.
	bigZero = new(big.Int).SetInt64(0)

	// ecTypeSecSchnorr is the ECDSA type for the chainec interface.
	ecTypeSecSchnorr = 2
)

// zeroArray zeroes the memory of a scalar array.
func zeroArray(a *[scalarSize]byte) {
	for i := 0; i < scalarSize; i++ {
		a[i] = 0x00
	}
}

// zeroSlice zeroes the memory of a scalar byte slice.
func zeroSlice(s []byte) {
	for i := 0; i < scalarSize; i++ {
		s[i] = 0x00
	}
}

// schnorrSign signs a Schnorr signature using a specified hash function
// and the given nonce, private key, message, and optional public nonce.
// CAVEAT: Lots of variable time algorithms using both the private key and
// k, which can expose the signer to constant time attacks. You have been
// warned! DO NOT use this algorithm where you might have the possibility
// of someone having EM field/cache/etc access.
// Memory management is also kind of sloppy and whether or not your keys
// or nonces can be found in memory later is likely a product of when the
// garbage collector runs.
// TODO Use field elements with constant time algorithms to prevent said
// attacks.
// This is identical to the Schnorr signature function found in libsecp256k1:
// https://github.com/bitcoin/secp256k1/tree/master/src/modules/schnorr
func schnorrSign(msg []byte, ps []byte, k []byte,
	pubNonceX *big.Int, pubNonceY *big.Int,
	hashFunc func([]byte) []byte) (*Signature, error) {
	curve := secp256k1.S256()
	if len(msg) != scalarSize {
		str := fmt.Sprintf("wrong size for message (got %v, want %v)",
			len(msg), scalarSize)
		return nil, schnorrError(ErrBadInputSize, str)
	}
	if len(ps) != scalarSize {
		str := fmt.Sprintf("wrong size for privkey (got %v, want %v)",
			len(ps), scalarSize)
		return nil, schnorrError(ErrBadInputSize, str)
	}
	if len(k) != scalarSize {
		str := fmt.Sprintf("wrong size for nonce k (got %v, want %v)",
			len(k), scalarSize)
		return nil, schnorrError(ErrBadInputSize, str)
	}

	psBig := new(big.Int).SetBytes(ps)
	bigK := new(big.Int).SetBytes(k)

	if psBig.Cmp(bigZero) == 0 {
		str := fmt.Sprintf("secret scalar is zero")
		return nil, schnorrError(ErrInputValue, str)
	}
	if psBig.Cmp(curve.N) >= 0 {
		str := fmt.Sprintf("secret scalar is out of bounds")
		return nil, schnorrError(ErrInputValue, str)
	}
	if bigK.Cmp(bigZero) == 0 {
		str := fmt.Sprintf("k scalar is zero")
		return nil, schnorrError(ErrInputValue, str)
	}
	if bigK.Cmp(curve.N) >= 0 {
		str := fmt.Sprintf("k scalar is out of bounds")
		return nil, schnorrError(ErrInputValue, str)
	}

	// R = kG
	var Rpx, Rpy *big.Int
	Rpx, Rpy = curve.ScalarBaseMult(k)
	if pubNonceX != nil && pubNonceY != nil {
		// Optional: if k' exists then R = R+k'
		Rpx, Rpy = curve.Add(Rpx, Rpy, pubNonceX, pubNonceY)
	}

	// Check if the field element that would be represented by Y is odd.
	// If it is, just keep k in the group order.
	if Rpy.Bit(0) == 1 {
		bigK.Mod(bigK, curve.N)
		bigK.Sub(curve.N, bigK)
	}

	// h = Hash(r || m)
	Rpxb := BigIntToEncodedBytes(Rpx)
	hashInput := make([]byte, 0, scalarSize*2)
	hashInput = append(hashInput, Rpxb[:]...)
	hashInput = append(hashInput, msg...)
	h := hashFunc(hashInput)
	hBig := new(big.Int).SetBytes(h)

	// If the hash ends up larger than the order of the curve, abort.
	if hBig.Cmp(curve.N) >= 0 {
		str := fmt.Sprintf("hash of (R || m) too big")
		return nil, schnorrError(ErrSchnorrHashValue, str)
	}

	// s = k - hx
	// TODO Speed this up a bunch by using field elements, not
	// big ints. That we multiply the private scalar using big
	// ints is also probably bad because we can only assume the
	// math isn't in constant time, thus opening us up to side
	// channel attacks. Using a constant time field element
	// implementation will fix this.
	sBig := new(big.Int)
	sBig.Mul(hBig, psBig)
	sBig.Sub(bigK, sBig)
	sBig.Mod(sBig, curve.N)

	if sBig.Cmp(bigZero) == 0 {
		str := fmt.Sprintf("sig s %v is zero", sBig)
		return nil, schnorrError(ErrZeroSigS, str)
	}

	// Zero out the private key and nonce when we're done with it.
	bigK.SetInt64(0)
	zeroSlice(k)
	psBig.SetInt64(0)
	zeroSlice(ps)

	return &Signature{Rpx, sBig}, nil
}

// Sign is the exported version of sign. It uses RFC6979 and Blake256 to
// produce a Schnorr signature.
func Sign(priv *secp256k1.PrivateKey,
	hash []byte) (r, s *big.Int, err error) {
	// Convert the private scalar to a 32 byte big endian number.
	pA := BigIntToEncodedBytes(priv.GetD())
	defer zeroArray(pA)

	// Generate a 32-byte scalar to use as a nonce. Try RFC6979
	// first.
	kB := nonceRFC6979(priv.Serialize(), hash, nil, nil)

	for {
		sig, err := schnorrSign(hash, pA[:], kB, nil, nil,
			chainhash.HashB)
		if err == nil {
			r = sig.GetR()
			s = sig.GetS()
			break
		}

		errTyped, ok := err.(Error)
		if !ok {
			return nil, nil, fmt.Errorf("unknown error type")
		}
		if errTyped.GetCode() != ErrSchnorrHashValue {
			return nil, nil, err
		}

		// We need to compute a new nonce, because the one we used
		// didn't work. Compute a random nonce.
		_, err = rand.Read(kB)
		if err != nil {
			return nil, nil, err
		}
	}

	return r, s, nil
}

// schnorrVerify is the internal function for verification of a secp256k1
// Schnorr signature. A secure hash function may be passed for the calculation
// of r.
// This is identical to the Schnorr verification function found in libsecp256k1:
// https://github.com/bitcoin/secp256k1/tree/master/src/modules/schnorr
func schnorrVerify(sig []byte,
	pubkey *secp256k1.PublicKey, msg []byte, hashFunc func([]byte) []byte) (bool,
	error) {
	curve := secp256k1.S256()
	if len(msg) != scalarSize {
		str := fmt.Sprintf("wrong size for message (got %v, want %v)",
			len(msg), scalarSize)
		return false, schnorrError(ErrBadInputSize, str)
	}

	if len(sig) != SignatureSize {
		str := fmt.Sprintf("wrong size for signature (got %v, want %v)",
			len(sig), SignatureSize)
		return false, schnorrError(ErrBadInputSize, str)
	}
	if pubkey == nil {
		str := fmt.Sprintf("nil pubkey")
		return false, schnorrError(ErrInputValue, str)
	}

	if !curve.IsOnCurve(pubkey.GetX(), pubkey.GetY()) {
		str := fmt.Sprintf("pubkey point is not on curve")
		return false, schnorrError(ErrPointNotOnCurve, str)
	}

	sigR := sig[:32]
	sigS := sig[32:]
	sigRCopy := make([]byte, scalarSize)
	copy(sigRCopy, sigR)
	toHash := append(sigRCopy, msg...)
	h := hashFunc(toHash)
	hBig := new(big.Int).SetBytes(h)

	// If the hash ends up larger than the order of the curve, abort.
	// Same thing for hash == 0 (as unlikely as that is...).
	if hBig.Cmp(curve.N) >= 0 {
		str := fmt.Sprintf("hash of (R || m) too big")
		return false, schnorrError(ErrSchnorrHashValue, str)
	}
	if hBig.Cmp(bigZero) == 0 {
		str := fmt.Sprintf("hash of (R || m) is zero value")
		return false, schnorrError(ErrSchnorrHashValue, str)
	}

	// Convert s to big int.
	sBig := EncodedBytesToBigInt(copyBytes(sigS))

	// We also can't have s greater than the order of the curve.
	if sBig.Cmp(curve.N) >= 0 {
		str := fmt.Sprintf("s value is too big")
		return false, schnorrError(ErrInputValue, str)
	}

	// r can't be larger than the curve prime.
	rBig := EncodedBytesToBigInt(copyBytes(sigR))
	if rBig.Cmp(curve.P) == 1 {
		str := fmt.Sprintf("given R was greater than curve prime")
		return false, schnorrError(ErrBadSigRNotOnCurve, str)
	}

	// r' = hQ + sG
	lx, ly := curve.ScalarMult(pubkey.GetX(), pubkey.GetY(), h)
	rx, ry := curve.ScalarBaseMult(sigS)
	rlx, rly := curve.Add(lx, ly, rx, ry)

	if rly.Bit(0) == 1 {
		str := fmt.Sprintf("calculated R y-value was odd")
		return false, schnorrError(ErrBadSigRYValue, str)
	}
	if !curve.IsOnCurve(rlx, rly) {
		str := fmt.Sprintf("calculated R point was not on curve")
		return false, schnorrError(ErrBadSigRNotOnCurve, str)
	}
	rlxB := BigIntToEncodedBytes(rlx)

	// r == r' --> valid signature
	if !bytes.Equal(sigR, rlxB[:]) {
		str := fmt.Sprintf("calculated R point was not given R")
		return false, schnorrError(ErrUnequalRValues, str)
	}

	return true, nil
}

// Verify is the generalized and exported function for the verification of a
// secp256k1 Schnorr signature. BLAKE256 is used as the hashing function.
func Verify(pubkey *secp256k1.PublicKey,
	msg []byte, r *big.Int, s *big.Int) bool {
	sig := NewSignature(r, s)
	ok, _ := schnorrVerify(sig.Serialize(), pubkey, msg,
		chainhash.HashB)

	return ok
}

// schnorrRecover recovers a public key using a signature, hash function,
// and message. It also attempts to verify the signature against the
// regenerated public key.
func schnorrRecover(sig, msg []byte,
	hashFunc func([]byte) []byte) (*secp256k1.PublicKey, bool, error) {
	curve := secp256k1.S256()
	if len(msg) != scalarSize {
		str := fmt.Sprintf("wrong size for message (got %v, want %v)",
			len(msg), scalarSize)
		return nil, false, schnorrError(ErrBadInputSize, str)
	}

	if len(sig) != SignatureSize {
		str := fmt.Sprintf("wrong size for signature (got %v, want %v)",
			len(sig), SignatureSize)
		return nil, false, schnorrError(ErrBadInputSize, str)
	}

	sigR := sig[:32]
	sigS := sig[32:]
	sigRCopy := make([]byte, scalarSize)
	copy(sigRCopy, sigR)
	toHash := append(sigRCopy, msg...)
	h := hashFunc(toHash)
	hBig := new(big.Int).SetBytes(h)

	// If the hash ends up larger than the order of the curve, abort.
	// Same thing for hash == 0 (as unlikely as that is...).
	if hBig.Cmp(curve.N) >= 0 {
		str := fmt.Sprintf("hash of (R || m) too big")
		return nil, false, schnorrError(ErrSchnorrHashValue, str)
	}
	if hBig.Cmp(bigZero) == 0 {
		str := fmt.Sprintf("hash of (R || m) is zero value")
		return nil, false, schnorrError(ErrSchnorrHashValue, str)
	}

	// Convert s to big int.
	sBig := EncodedBytesToBigInt(copyBytes(sigS))

	// We also can't have s greater than the order of the curve.
	if sBig.Cmp(curve.N) >= 0 {
		str := fmt.Sprintf("s value is too big")
		return nil, false, schnorrError(ErrInputValue, str)
	}

	// r can't be larger than the curve prime.
	rBig := EncodedBytesToBigInt(copyBytes(sigR))
	if rBig.Cmp(curve.P) == 1 {
		str := fmt.Sprintf("given R was greater than curve prime")
		return nil, false, schnorrError(ErrBadSigRNotOnCurve, str)
	}

	// Decompress the Y value. We know that the first bit must
	// be even. Use the PublicKey struct to make it easier.
	compressedPoint := make([]byte, PubKeyBytesLen)
	compressedPoint[0] = pubkeyCompressed
	copy(compressedPoint[1:], sigR)
	rPoint, err := secp256k1.ParsePubKey(compressedPoint)
	if err != nil {
		str := fmt.Sprintf("bad r point")
		return nil, false, schnorrError(ErrRegenerateRPoint, str)
	}

	// Get the inverse of the hash.
	hInv := new(big.Int).ModInverse(hBig, curve.N)
	hInv.Mod(hInv, curve.N)

	// Negate s.
	sBig.Sub(curve.N, sBig)
	sBig.Mod(sBig, curve.N)

	// s' = -s * inverse(h).
	sBig.Mul(sBig, hInv)
	sBig.Mod(sBig, curve.N)

	// Q = h^(-1)R + s'G
	lx, ly := curve.ScalarMult(rPoint.GetX(), rPoint.GetY(), hInv.Bytes())
	rx, ry := curve.ScalarBaseMult(sBig.Bytes())
	pkx, pky := curve.Add(lx, ly, rx, ry)

	// Check if the public key is on the curve.
	if !curve.IsOnCurve(pkx, pky) {
		str := fmt.Sprintf("pubkey not on curve")
		return nil, false, schnorrError(ErrPubKeyOffCurve, str)
	}
	pubkey := secp256k1.NewPublicKey(pkx, pky)

	// Verify this signature. Slow, lots of double checks, could be more
	// cheaply implemented as
	// hQ + sG - R == 0
	// which this function checks.
	// This will sometimes pass even for corrupted signatures, but
	// this shouldn't be a concern because whoever is using the
	// results should be checking the returned public key against
	// some known one anyway. In the case of these Schnorr signatures,
	// relatively high numbers of corrupted signatures (50-70%)
	// seem to produce valid pubkeys and valid signatures.
	_, err = schnorrVerify(sig, pubkey, msg, hashFunc)
	if err != nil {
		str := fmt.Sprintf("pubkey/sig pair could not be validated")
		return nil, false, schnorrError(ErrRegenSig, str)
	}

	return pubkey, true, nil
}

// RecoverPubkey is the exported and generalized version of schnorrRecover.
// It recovers a public key given a signature and a message, using BLAKE256
// as the hashing function.
func RecoverPubkey(sig,
	msg []byte) (*secp256k1.PublicKey, bool, error) {

	return schnorrRecover(sig, msg, chainhash.HashB)
}
