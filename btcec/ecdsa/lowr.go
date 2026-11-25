// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ecdsa

import (
	"encoding/binary"

	"github.com/btcsuite/btcd/btcec/v2"
)

// SignLowR is like Sign, but grinds for a low R: one whose first byte, in its
// 32-byte big-endian encoding, is below 0x80, so DER never pads it with a
// 0x00 byte.
//
// The search is Bitcoin Core's CKey::Sign.  The first attempt is the
// signature Sign returns.  Attempt i > 0 passes i, as 32 bytes in
// little-endian order, to RFC6979 as extra data.  The first attempt with a
// low R is returned.
//
// For a hash below the group order, the result is the signature Core makes.
// For a larger hash it can differ, as Sign's does: dcrd does not reduce the
// hash before deriving the nonce, and libsecp256k1 does.
func SignLowR(key *btcec.PrivateKey, hash []byte) *Signature {
	var keyBytes [32]byte
	key.Key.PutBytes(&keyBytes)
	defer zeroArray32(&keyBytes)

	var extra [32]byte
	for counter := uint32(0); ; counter++ {
		// The first attempt has no extra data, which is not the same
		// as 32 zero bytes.
		var extraData []byte
		if counter > 0 {
			binary.LittleEndian.PutUint32(extra[:], counter)
			extraData = extra[:]
		}

		sig := signWithExtra(&key.Key, keyBytes[:], hash, extraData)
		if hasLowR(sig) {
			return sig
		}
	}
}

// signWithExtra returns the RFC6979 signature of hash that uses extra as
// additional nonce data.  It tries the next nonce in the RFC6979 stream in the
// negligible case that R or S is zero.
func signWithExtra(key *btcec.ModNScalar, keyBytes, hash,
	extra []byte) *Signature {

	for iteration := uint32(0); ; iteration++ {
		k := btcec.NonceRFC6979(keyBytes, hash, extra, nil, iteration)
		sig, ok := signWithNonce(key, k, hash)
		k.Zero()
		if ok {
			return sig
		}
	}
}

// signWithNonce signs hash with the given key and nonce, as the dcrd secp256k1
// package does for Sign, and returns a signature with a low S.  It fails if R
// or S is zero.
func signWithNonce(key, k *btcec.ModNScalar, hash []byte) (*Signature, bool) {
	// r = (kG).x mod N
	var kG btcec.JacobianPoint
	btcec.ScalarBaseMultNonConst(k, &kG)
	kG.ToAffine()

	var xBytes [32]byte
	kG.X.PutBytes(&xBytes)
	var r btcec.ModNScalar
	r.SetBytes(&xBytes)
	if r.IsZero() {
		return nil, false
	}

	// s = k^-1 (e + d*r) mod N, negated if above N/2.
	var e btcec.ModNScalar
	e.SetByteSlice(hash)
	kInv := new(btcec.ModNScalar).InverseValNonConst(k)
	s := new(btcec.ModNScalar).Mul2(key, &r).Add(&e).Mul(kInv)
	if s.IsZero() {
		return nil, false
	}
	if s.IsOverHalfOrder() {
		s.Negate()
	}

	return NewSignature(&r, s), true
}

// hasLowR returns whether the first byte of the signature's R is below 0x80.
func hasLowR(sig *Signature) bool {
	r := sig.R()
	return r.Bytes()[0] < 0x80
}

// zeroArray32 zeroes the 32-byte array.
func zeroArray32(b *[32]byte) {
	*b = [32]byte{}
}
