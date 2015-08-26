// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chainec

import (
	"errors"
	"io"
	"math/big"

	"github.com/decred/dcrd/dcrec/edwards"
)

type edwardsDSA struct {
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
	edwardsCurve = edwards.Edwards()
)

// Boilerplate exported functions to make the struct interact with the interface.
// Constants
func (e edwardsDSA) GetP() *big.Int {
	return e.getP()
}
func (e edwardsDSA) GetN() *big.Int {
	return e.getN()
}

// EC Math
func (e edwardsDSA) Add(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	return e.add(x1, y1, x2, y2)
}
func (e edwardsDSA) IsOnCurve(x, y *big.Int) bool {
	return e.isOnCurve(x, y)
}
func (e edwardsDSA) ScalarMult(x, y *big.Int, k []byte) (*big.Int, *big.Int) {
	return e.scalarMult(x, y, k)
}
func (e edwardsDSA) ScalarBaseMult(k []byte) (*big.Int, *big.Int) {
	return e.scalarBaseMult(k)
}

// Private keys
func (e edwardsDSA) NewPrivateKey(d *big.Int) PrivateKey {
	return e.newPrivateKey(d)
}
func (e edwardsDSA) PrivKeyFromBytes(pk []byte) (PrivateKey, PublicKey) {
	return e.privKeyFromBytes(pk)
}
func (e edwardsDSA) PrivKeyFromScalar(pk []byte) (PrivateKey, PublicKey) {
	return e.privKeyFromScalar(pk)
}
func (e edwardsDSA) PrivKeyBytesLen() int {
	return e.privKeyBytesLen()
}

// Public keys
func (e edwardsDSA) NewPublicKey(x *big.Int, y *big.Int) PublicKey {
	return e.newPublicKey(x, y)
}
func (e edwardsDSA) ParsePubKey(pubKeyStr []byte) (PublicKey, error) {
	return e.parsePubKey(pubKeyStr)
}
func (e edwardsDSA) PubKeyBytesLen() int {
	return e.pubKeyBytesLen()
}
func (e edwardsDSA) PubKeyBytesLenUncompressed() int {
	return e.pubKeyBytesLenUncompressed()
}
func (e edwardsDSA) PubKeyBytesLenCompressed() int {
	return e.pubKeyBytesLenCompressed()
}
func (e edwardsDSA) PubKeyBytesLenHybrid() int {
	return e.pubKeyBytesLenCompressed()
}

// Signatures
func (e edwardsDSA) NewSignature(r, s *big.Int) Signature {
	return e.newSignature(r, s)
}
func (e edwardsDSA) ParseDERSignature(sigStr []byte) (Signature, error) {
	return e.parseDERSignature(sigStr)
}
func (e edwardsDSA) ParseSignature(sigStr []byte) (Signature, error) {
	return e.parseSignature(sigStr)
}
func (e edwardsDSA) RecoverCompact(signature, hash []byte) (PublicKey, bool,
	error) {
	return e.recoverCompact(signature, hash)
}

// ECDSA
func (e edwardsDSA) GenerateKey(rand io.Reader) ([]byte, *big.Int, *big.Int,
	error) {
	return e.generateKey(rand)
}
func (e edwardsDSA) Sign(priv PrivateKey, hash []byte) (r, s *big.Int,
	err error) {
	r, s, err = e.sign(priv, hash)
	return
}
func (e edwardsDSA) Verify(pub PublicKey, hash []byte, r, s *big.Int) bool {
	return e.verify(pub, hash, r, s)
}

// Symmetric cipher encryption
func (e edwardsDSA) GenerateSharedSecret(privkey []byte, x, y *big.Int) []byte {
	return e.generateSharedSecret(privkey, x, y)
}
func (e edwardsDSA) Encrypt(x, y *big.Int, in []byte) ([]byte,
	error) {
	return e.encrypt(x, y, in)
}
func (e edwardsDSA) Decrypt(privkey []byte, in []byte) ([]byte,
	error) {
	return e.decrypt(privkey, in)
}

// newEdwardsDSA instatiates a function DSA subsystem over the edwards 25519
// curve. A caveat for the functions below is that they're all routed through
// interfaces, and nil returns from the library itself for interfaces must
// ALWAYS be checked by checking the return value by attempted dereference
// (== nil).
func newEdwardsDSA() DSA {
	var ed DSA = &edwardsDSA{
		// Constants
		getP: func() *big.Int {
			return edwardsCurve.P
		},
		getN: func() *big.Int {
			return edwardsCurve.N
		},

		// EC Math
		add: func(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
			return edwardsCurve.Add(x1, y1, x2, y2)
		},
		isOnCurve: func(x, y *big.Int) bool {
			return edwardsCurve.IsOnCurve(x, y)
		},
		scalarMult: func(x, y *big.Int, k []byte) (*big.Int, *big.Int) {
			return edwardsCurve.ScalarMult(x, y, k)
		},
		scalarBaseMult: func(k []byte) (*big.Int, *big.Int) {
			return edwardsCurve.ScalarBaseMult(k)
		},

		// Private keys
		newPrivateKey: func(d *big.Int) PrivateKey {
			pk := edwards.NewPrivateKey(edwardsCurve, d)
			if pk != nil {
				return PrivateKey(*pk)
			}
			return nil
		},
		privKeyFromBytes: func(pk []byte) (PrivateKey, PublicKey) {
			priv, pub := edwards.PrivKeyFromBytes(edwardsCurve, pk)
			if priv == nil {
				return nil, nil
			}
			if pub == nil {
				return nil, nil
			}
			tpriv := PrivateKey(*priv)
			tpub := PublicKey(*pub)
			return tpriv, tpub
		},
		privKeyFromScalar: func(pk []byte) (PrivateKey, PublicKey) {
			priv, pub, err := edwards.PrivKeyFromScalar(edwardsCurve, pk)
			if err != nil {
				return nil, nil
			}
			if priv == nil {
				return nil, nil
			}
			if pub == nil {
				return nil, nil
			}
			tpriv := PrivateKey(*priv)
			tpub := PublicKey(*pub)
			return tpriv, tpub
		},
		privKeyBytesLen: func() int {
			return edwards.PrivKeyBytesLen
		},

		// Public keys
		newPublicKey: func(x *big.Int, y *big.Int) PublicKey {
			pk := edwards.NewPublicKey(edwardsCurve, x, y)
			tpk := PublicKey(*pk)
			return tpk
		},
		parsePubKey: func(pubKeyStr []byte) (PublicKey, error) {
			pk, err := edwards.ParsePubKey(edwardsCurve, pubKeyStr)
			if err != nil {
				return nil, err
			}
			tpk := PublicKey(*pk)
			return tpk, err
		},
		pubKeyBytesLen: func() int {
			return edwards.PubKeyBytesLen
		},
		pubKeyBytesLenUncompressed: func() int {
			return edwards.PubKeyBytesLen
		},
		pubKeyBytesLenCompressed: func() int {
			return edwards.PubKeyBytesLen
		},
		pubKeyBytesLenHybrid: func() int {
			return edwards.PubKeyBytesLen
		},

		// Signatures
		newSignature: func(r *big.Int, s *big.Int) Signature {
			sig := edwards.NewSignature(r, s)
			ts := Signature(*sig)
			return ts
		},
		parseDERSignature: func(sigStr []byte) (Signature, error) {
			sig, err := edwards.ParseDERSignature(edwardsCurve, sigStr)
			if err != nil {
				return nil, err
			}
			ts := Signature(*sig)
			return ts, err
		},
		parseSignature: func(sigStr []byte) (Signature, error) {
			sig, err := edwards.ParseSignature(edwardsCurve, sigStr)
			if err != nil {
				return nil, err
			}
			ts := Signature(*sig)
			return ts, err
		},
		recoverCompact: func(signature, hash []byte) (PublicKey, bool, error) {
			pk, bl, err := edwards.RecoverCompact(signature, hash)
			tpk := PublicKey(*pk)
			return tpk, bl, err
		},

		// ECDSA
		generateKey: func(rand io.Reader) ([]byte, *big.Int, *big.Int, error) {
			return edwards.GenerateKey(edwardsCurve, rand)
		},
		sign: func(priv PrivateKey, hash []byte) (r, s *big.Int, err error) {
			if priv.GetType() != ECTypeEdwards {
				return nil, nil, errors.New("wrong type")
			}
			epriv, ok := priv.(edwards.PrivateKey)
			if !ok {
				return nil, nil, errors.New("wrong type")
			}
			r, s, err = edwards.Sign(edwardsCurve, &epriv, hash)
			return
		},
		verify: func(pub PublicKey, hash []byte, r, s *big.Int) bool {
			if pub.GetType() != ECTypeEdwards {
				return false
			}
			epub, ok := pub.(edwards.PublicKey)
			if !ok {
				return false
			}
			return edwards.Verify(&epub, hash, r, s)
		},

		// Symmetric cipher encryption
		generateSharedSecret: func(privkey []byte, x, y *big.Int) []byte {
			privKeyLocal, _, err := edwards.PrivKeyFromScalar(edwardsCurve,
				privkey)
			if err != nil {
				return nil
			}
			pubkey := edwards.NewPublicKey(edwardsCurve, x, y)
			return edwards.GenerateSharedSecret(privKeyLocal, pubkey)
		},
		encrypt: func(x, y *big.Int, in []byte) ([]byte, error) {
			pubkey := edwards.NewPublicKey(edwardsCurve, x, y)
			return edwards.Encrypt(edwardsCurve, pubkey, in)
		},
		decrypt: func(privkey []byte, in []byte) ([]byte, error) {
			privKeyLocal, _, err := edwards.PrivKeyFromScalar(edwardsCurve,
				privkey)
			if err != nil {
				return nil, err
			}
			return edwards.Decrypt(edwardsCurve, privKeyLocal, in)
		},
	}

	return ed.(DSA)
}
