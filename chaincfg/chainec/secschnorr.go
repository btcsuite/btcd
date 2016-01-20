// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chainec

import (
	"fmt"
	"io"
	"math/big"

	"github.com/decred/dcrd/dcrec/secp256k1"
	"github.com/decred/dcrd/dcrec/secp256k1/schnorr"
)

type secSchnorrDSA struct {
	// Constants
	getN func() *big.Int
	getP func() *big.Int

	// EC Math
	add            func(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int)
	isOnCurve      func(x *big.Int, y *big.Int) bool
	scalarMult     func(x, y *big.Int, k []byte) (*big.Int, *big.Int)
	scalarBaseMult func(k []byte) (*big.Int, *big.Int)

	// Private keys
	newPrivateKey     func(d *big.Int) PrivateKey
	privKeyFromBytes  func(pk []byte) (PrivateKey, PublicKey)
	privKeyFromScalar func(pk []byte) (PrivateKey, PublicKey)
	privKeyBytesLen   func() int

	// Public keys
	newPublicKey               func(x *big.Int, y *big.Int) PublicKey
	parsePubKey                func(pubKeyStr []byte) (PublicKey, error)
	pubKeyBytesLen             func() int
	pubKeyBytesLenUncompressed func() int
	pubKeyBytesLenCompressed   func() int
	pubKeyBytesLenHybrid       func() int

	// Signatures
	newSignature      func(r *big.Int, s *big.Int) Signature
	parseDERSignature func(sigStr []byte) (Signature, error)
	parseSignature    func(sigStr []byte) (Signature, error)
	recoverCompact    func(signature, hash []byte) (PublicKey, bool, error)

	// ECDSA
	generateKey func(rand io.Reader) ([]byte, *big.Int, *big.Int, error)
	sign        func(priv PrivateKey, hash []byte) (r, s *big.Int, err error)
	verify      func(pub PublicKey, hash []byte, r, s *big.Int) bool

	// Symmetric cipher encryption
	generateSharedSecret func(privkey []byte, x, y *big.Int) []byte
	encrypt              func(x, y *big.Int, in []byte) ([]byte, error)
	decrypt              func(privkey []byte, in []byte) ([]byte, error)
}

// Boilerplate exported functions to make the struct interact with the interface.
// Constants
func (s secSchnorrDSA) GetP() *big.Int {
	return s.getP()
}
func (s secSchnorrDSA) GetN() *big.Int {
	return s.getN()
}

// EC Math
func (s secSchnorrDSA) Add(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	return s.add(x1, y1, x2, y2)
}
func (s secSchnorrDSA) IsOnCurve(x, y *big.Int) bool {
	return s.isOnCurve(x, y)
}
func (s secSchnorrDSA) ScalarMult(x, y *big.Int, k []byte) (*big.Int, *big.Int) {
	return s.scalarMult(x, y, k)
}
func (s secSchnorrDSA) ScalarBaseMult(k []byte) (*big.Int, *big.Int) {
	return s.scalarBaseMult(k)
}

// Private keys
func (s secSchnorrDSA) NewPrivateKey(d *big.Int) PrivateKey {
	return s.newPrivateKey(d)
}
func (s secSchnorrDSA) PrivKeyFromBytes(pk []byte) (PrivateKey, PublicKey) {
	return s.privKeyFromBytes(pk)
}
func (s secSchnorrDSA) PrivKeyFromScalar(pk []byte) (PrivateKey, PublicKey) {
	return s.privKeyFromScalar(pk)
}
func (s secSchnorrDSA) PrivKeyBytesLen() int {
	return s.privKeyBytesLen()
}

// Public keys
func (s secSchnorrDSA) NewPublicKey(x *big.Int, y *big.Int) PublicKey {
	return s.newPublicKey(x, y)
}
func (s secSchnorrDSA) ParsePubKey(pubKeyStr []byte) (PublicKey, error) {
	return s.parsePubKey(pubKeyStr)
}
func (s secSchnorrDSA) PubKeyBytesLen() int {
	return s.pubKeyBytesLen()
}
func (s secSchnorrDSA) PubKeyBytesLenUncompressed() int {
	return s.pubKeyBytesLenUncompressed()
}
func (s secSchnorrDSA) PubKeyBytesLenCompressed() int {
	return s.pubKeyBytesLenCompressed()
}
func (s secSchnorrDSA) PubKeyBytesLenHybrid() int {
	return s.pubKeyBytesLenCompressed()
}

// Signatures
func (sp secSchnorrDSA) NewSignature(r, s *big.Int) Signature {
	return sp.newSignature(r, s)
}
func (s secSchnorrDSA) ParseDERSignature(sigStr []byte) (Signature, error) {
	return s.parseDERSignature(sigStr)
}
func (s secSchnorrDSA) ParseSignature(sigStr []byte) (Signature, error) {
	return s.parseSignature(sigStr)
}
func (s secSchnorrDSA) RecoverCompact(signature, hash []byte) (PublicKey, bool,
	error) {
	return s.recoverCompact(signature, hash)
}

// ECDSA
func (s secSchnorrDSA) GenerateKey(rand io.Reader) ([]byte, *big.Int, *big.Int,
	error) {
	return s.generateKey(rand)
}
func (sp secSchnorrDSA) Sign(priv PrivateKey, hash []byte) (r, s *big.Int,
	err error) {
	r, s, err = sp.sign(priv, hash)
	return
}
func (sp secSchnorrDSA) Verify(pub PublicKey, hash []byte, r, s *big.Int) bool {
	return sp.verify(pub, hash, r, s)
}

// Symmetric cipher encryption
func (s secSchnorrDSA) GenerateSharedSecret(privkey []byte, x, y *big.Int) []byte {
	return s.generateSharedSecret(privkey, x, y)
}
func (s secSchnorrDSA) Encrypt(x, y *big.Int, in []byte) ([]byte,
	error) {
	return s.encrypt(x, y, in)
}
func (s secSchnorrDSA) Decrypt(privkey []byte, in []byte) ([]byte,
	error) {
	return s.decrypt(privkey, in)
}

// newSecSchnorrDSA instatiates a function DSA subsystem over the secp256k1
// curve. A caveat for the functions below is that they're all routed through
// interfaces, and nil returns from the library itself for interfaces must
// ALWAYS be checked by checking the return value by attempted dereference
// (== nil).
func newSecSchnorrDSA() DSA {
	var secp DSA = &secSchnorrDSA{
		// Constants
		getP: func() *big.Int {
			return secp256k1Curve.P
		},
		getN: func() *big.Int {
			return secp256k1Curve.N
		},

		// EC Math
		add: func(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
			return secp256k1Curve.Add(x1, y1, x2, y2)
		},
		isOnCurve: func(x, y *big.Int) bool {
			return secp256k1Curve.IsOnCurve(x, y)
		},
		scalarMult: func(x, y *big.Int, k []byte) (*big.Int, *big.Int) {
			return secp256k1Curve.ScalarMult(x, y, k)
		},
		scalarBaseMult: func(k []byte) (*big.Int, *big.Int) {
			return secp256k1Curve.ScalarBaseMult(k)
		},

		// Private keys
		newPrivateKey: func(d *big.Int) PrivateKey {
			pk := secp256k1.NewPrivateKey(secp256k1Curve, d)
			if pk != nil {
				return PrivateKey(pk)
			}
			return nil
		},
		privKeyFromBytes: func(pk []byte) (PrivateKey, PublicKey) {
			priv, pub := secp256k1.PrivKeyFromBytes(secp256k1Curve, pk)
			if priv == nil {
				return nil, nil
			}
			if pub == nil {
				return nil, nil
			}
			tpriv := PrivateKey(priv)
			tpub := PublicKey(pub)
			return tpriv, tpub
		},
		privKeyFromScalar: func(pk []byte) (PrivateKey, PublicKey) {
			priv, pub := secp256k1.PrivKeyFromScalar(secp256k1Curve, pk)
			if priv == nil {
				return nil, nil
			}
			if pub == nil {
				return nil, nil
			}
			tpriv := PrivateKey(priv)
			tpub := PublicKey(pub)
			return tpriv, tpub
		},
		privKeyBytesLen: func() int {
			return secp256k1.PrivKeyBytesLen
		},

		// Public keys
		// Note that Schnorr only allows 33 byte public keys, however
		// as they are secp256k1 you still have access to the other
		// serialization types.
		newPublicKey: func(x *big.Int, y *big.Int) PublicKey {
			pk := secp256k1.NewPublicKey(secp256k1Curve, x, y)
			tpk := PublicKey(pk)
			return tpk
		},
		parsePubKey: func(pubKeyStr []byte) (PublicKey, error) {
			pk, err := schnorr.ParsePubKey(secp256k1Curve, pubKeyStr)
			if err != nil {
				return nil, err
			}
			tpk := PublicKey(pk)
			return tpk, err
		},
		pubKeyBytesLen: func() int {
			return schnorr.PubKeyBytesLen
		},
		pubKeyBytesLenUncompressed: func() int {
			return schnorr.PubKeyBytesLen
		},
		pubKeyBytesLenCompressed: func() int {
			return schnorr.PubKeyBytesLen
		},
		pubKeyBytesLenHybrid: func() int {
			return schnorr.PubKeyBytesLen
		},

		// Signatures
		newSignature: func(r *big.Int, s *big.Int) Signature {
			sig := schnorr.NewSignature(r, s)
			ts := Signature(sig)
			return ts
		},
		parseDERSignature: func(sigStr []byte) (Signature, error) {
			sig, err := schnorr.ParseSignature(sigStr)
			ts := Signature(sig)
			return ts, err
		},
		parseSignature: func(sigStr []byte) (Signature, error) {
			sig, err := schnorr.ParseSignature(sigStr)
			ts := Signature(sig)
			return ts, err
		},
		recoverCompact: func(signature, hash []byte) (PublicKey, bool, error) {
			pk, bl, err := schnorr.RecoverPubkey(secp256k1Curve, signature,
				hash)
			tpk := PublicKey(pk)
			return tpk, bl, err
		},

		// ECDSA
		generateKey: func(rand io.Reader) ([]byte, *big.Int, *big.Int, error) {
			return secp256k1.GenerateKey(secp256k1Curve, rand)
		},
		sign: func(priv PrivateKey, hash []byte) (r, s *big.Int, err error) {
			spriv := secp256k1.NewPrivateKey(secp256k1Curve, priv.GetD())
			return schnorr.Sign(secp256k1Curve, spriv, hash)
		},
		verify: func(pub PublicKey, hash []byte, r, s *big.Int) bool {
			spub := secp256k1.NewPublicKey(secp256k1Curve, pub.GetX(), pub.GetY())
			return schnorr.Verify(secp256k1Curve, spub, hash, r, s)
		},

		// Symmetric cipher encryption
		generateSharedSecret: func(privkey []byte, x, y *big.Int) []byte {
			sprivkey, _ := secp256k1.PrivKeyFromBytes(secp256k1Curve, privkey)
			if sprivkey == nil {
				return nil
			}
			spubkey := secp256k1.NewPublicKey(secp256k1Curve, x, y)

			return secp256k1.GenerateSharedSecret(sprivkey, spubkey)
		},
		encrypt: func(x, y *big.Int, in []byte) ([]byte, error) {
			spubkey := secp256k1.NewPublicKey(secp256k1Curve, x, y)

			return secp256k1.Encrypt(spubkey, in)
		},
		decrypt: func(privkey []byte, in []byte) ([]byte, error) {
			sprivkey, _ := secp256k1.PrivKeyFromBytes(secp256k1Curve, privkey)
			if sprivkey == nil {
				return nil, fmt.Errorf("failure deserializing privkey")
			}

			return secp256k1.Decrypt(sprivkey, in)
		},
	}

	return secp.(DSA)
}
