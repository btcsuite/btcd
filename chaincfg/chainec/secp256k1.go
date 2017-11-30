// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chainec

import (
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/decred/dcrd/dcrec/secp256k1"
)

type secp256k1DSA struct {
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

var (
	secp256k1Curve = secp256k1.S256()
)

// Boilerplate exported functions to make the struct interact with the interface.
// Constants
func (sp secp256k1DSA) GetP() *big.Int {
	return sp.getP()
}
func (sp secp256k1DSA) GetN() *big.Int {
	return sp.getN()
}

// EC Math
func (sp secp256k1DSA) Add(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	return sp.add(x1, y1, x2, y2)
}
func (sp secp256k1DSA) IsOnCurve(x, y *big.Int) bool {
	return sp.isOnCurve(x, y)
}
func (sp secp256k1DSA) ScalarMult(x, y *big.Int, k []byte) (*big.Int, *big.Int) {
	return sp.scalarMult(x, y, k)
}
func (sp secp256k1DSA) ScalarBaseMult(k []byte) (*big.Int, *big.Int) {
	return sp.scalarBaseMult(k)
}

// Private keys
func (sp secp256k1DSA) NewPrivateKey(d *big.Int) PrivateKey {
	return sp.newPrivateKey(d)
}
func (sp secp256k1DSA) PrivKeyFromBytes(pk []byte) (PrivateKey, PublicKey) {
	return sp.privKeyFromBytes(pk)
}
func (sp secp256k1DSA) PrivKeyFromScalar(pk []byte) (PrivateKey, PublicKey) {
	return sp.privKeyFromScalar(pk)
}
func (sp secp256k1DSA) PrivKeyBytesLen() int {
	return sp.privKeyBytesLen()
}

// Public keys
func (sp secp256k1DSA) NewPublicKey(x *big.Int, y *big.Int) PublicKey {
	return sp.newPublicKey(x, y)
}
func (sp secp256k1DSA) ParsePubKey(pubKeyStr []byte) (PublicKey, error) {
	return sp.parsePubKey(pubKeyStr)
}
func (sp secp256k1DSA) PubKeyBytesLen() int {
	return sp.pubKeyBytesLen()
}
func (sp secp256k1DSA) PubKeyBytesLenUncompressed() int {
	return sp.pubKeyBytesLenUncompressed()
}
func (sp secp256k1DSA) PubKeyBytesLenCompressed() int {
	return sp.pubKeyBytesLenCompressed()
}
func (sp secp256k1DSA) PubKeyBytesLenHybrid() int {
	return sp.pubKeyBytesLenCompressed()
}

// Signatures
func (sp secp256k1DSA) NewSignature(r, s *big.Int) Signature {
	return sp.newSignature(r, s)
}
func (sp secp256k1DSA) ParseDERSignature(sigStr []byte) (Signature, error) {
	return sp.parseDERSignature(sigStr)
}
func (sp secp256k1DSA) ParseSignature(sigStr []byte) (Signature, error) {
	return sp.parseSignature(sigStr)
}
func (sp secp256k1DSA) RecoverCompact(signature, hash []byte) (PublicKey, bool,
	error) {
	return sp.recoverCompact(signature, hash)
}

// ECDSA
func (sp secp256k1DSA) GenerateKey(rand io.Reader) ([]byte, *big.Int, *big.Int,
	error) {
	return sp.generateKey(rand)
}
func (sp secp256k1DSA) Sign(priv PrivateKey, hash []byte) (r, s *big.Int,
	err error) {
	r, s, err = sp.sign(priv, hash)
	return
}
func (sp secp256k1DSA) Verify(pub PublicKey, hash []byte, r, s *big.Int) bool {
	return sp.verify(pub, hash, r, s)
}

// Symmetric cipher encryption
func (sp secp256k1DSA) GenerateSharedSecret(privkey []byte, x, y *big.Int) []byte {
	return sp.generateSharedSecret(privkey, x, y)
}
func (sp secp256k1DSA) Encrypt(x, y *big.Int, in []byte) ([]byte,
	error) {
	return sp.encrypt(x, y, in)
}
func (sp secp256k1DSA) Decrypt(privkey []byte, in []byte) ([]byte,
	error) {
	return sp.decrypt(privkey, in)
}

// newSecp256k1DSA instatiates a function DSA subsystem over the secp256k1
// curve. A caveat for the functions below is that they're all routed through
// interfaces, and nil returns from the library itself for interfaces must
// ALWAYS be checked by checking the return value by attempted dereference
// (== nil).
func newSecp256k1DSA() DSA {
	var secp DSA = &secp256k1DSA{
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
			if d == nil {
				return nil
			}
			pk := secp256k1.NewPrivateKey(d)
			if pk != nil {
				return PrivateKey(pk)
			}
			return nil
		},
		privKeyFromBytes: func(pk []byte) (PrivateKey, PublicKey) {
			priv, pub := secp256k1.PrivKeyFromBytes(pk)
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
			priv, pub := secp256k1.PrivKeyFromScalar(pk)
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
		newPublicKey: func(x *big.Int, y *big.Int) PublicKey {
			pk := secp256k1.NewPublicKey(x, y)
			tpk := PublicKey(pk)
			return tpk
		},
		parsePubKey: func(pubKeyStr []byte) (PublicKey, error) {
			pk, err := secp256k1.ParsePubKey(pubKeyStr)
			if err != nil {
				return nil, err
			}
			tpk := PublicKey(pk)
			return tpk, err
		},
		pubKeyBytesLen: func() int {
			return secp256k1.PubKeyBytesLenCompressed
		},
		pubKeyBytesLenUncompressed: func() int {
			return secp256k1.PubKeyBytesLenUncompressed
		},
		pubKeyBytesLenCompressed: func() int {
			return secp256k1.PubKeyBytesLenCompressed
		},
		pubKeyBytesLenHybrid: func() int {
			return secp256k1.PubKeyBytesLenHybrid
		},

		// Signatures
		newSignature: func(r *big.Int, s *big.Int) Signature {
			sig := secp256k1.NewSignature(r, s)
			ts := Signature(sig)
			return ts
		},
		parseDERSignature: func(sigStr []byte) (Signature, error) {
			sig, err := secp256k1.ParseDERSignature(sigStr, secp256k1Curve)
			if err != nil {
				return nil, err
			}
			ts := Signature(sig)
			return ts, err
		},
		parseSignature: func(sigStr []byte) (Signature, error) {
			sig, err := secp256k1.ParseSignature(sigStr, secp256k1Curve)
			if err != nil {
				return nil, err
			}
			ts := Signature(sig)
			return ts, err
		},
		recoverCompact: func(signature, hash []byte) (PublicKey, bool, error) {
			pk, bl, err := secp256k1.RecoverCompact(signature,
				hash)
			tpk := PublicKey(pk)
			return tpk, bl, err
		},

		// ECDSA
		generateKey: func(rand io.Reader) ([]byte, *big.Int, *big.Int, error) {
			return secp256k1.GenerateKey(rand)
		},
		sign: func(priv PrivateKey, hash []byte) (r, s *big.Int, err error) {
			if priv.GetType() != ECTypeSecp256k1 {
				return nil, nil, errors.New("wrong type")
			}
			spriv, ok := priv.(*secp256k1.PrivateKey)
			if !ok {
				return nil, nil, errors.New("wrong type")
			}
			sig, err := spriv.Sign(hash)
			if sig != nil {
				r = sig.GetR()
				s = sig.GetS()
			}
			return
		},
		verify: func(pub PublicKey, hash []byte, r, s *big.Int) bool {
			spub := secp256k1.NewPublicKey(pub.GetX(), pub.GetY())
			ssig := secp256k1.NewSignature(r, s)
			return ssig.Verify(hash, spub)
		},

		// Symmetric cipher encryption
		generateSharedSecret: func(privkey []byte, x, y *big.Int) []byte {
			sprivkey, _ := secp256k1.PrivKeyFromBytes(privkey)
			if sprivkey == nil {
				return nil
			}
			spubkey := secp256k1.NewPublicKey(x, y)

			return secp256k1.GenerateSharedSecret(sprivkey, spubkey)
		},
		encrypt: func(x, y *big.Int, in []byte) ([]byte, error) {
			spubkey := secp256k1.NewPublicKey(x, y)

			return secp256k1.Encrypt(spubkey, in)
		},
		decrypt: func(privkey []byte, in []byte) ([]byte, error) {
			sprivkey, _ := secp256k1.PrivKeyFromBytes(privkey)
			if sprivkey == nil {
				return nil, fmt.Errorf("failure deserializing privkey")
			}

			return secp256k1.Decrypt(sprivkey, in)
		},
	}

	return secp.(DSA)
}
