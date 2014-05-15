// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcec

import (
	"crypto/ecdsa"
	"crypto/rand"
	"math/big"
)

// PrivateKey is an ecdsa.PrivateKey
// It provides a method Sign
type PrivateKey ecdsa.PrivateKey

// PrivKeyFromBytes returns a private and public key for `curve' based on the
// private key passed as an argument as a byte slice.
func PrivKeyFromBytes(curve *KoblitzCurve, pk []byte) (*PrivateKey,
	*PublicKey) {
	x, y := curve.ScalarBaseMult(pk)

	priv := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: curve,
			X:     x,
			Y:     y,
		},
		D: new(big.Int).SetBytes(pk),
	}

	return (*PrivateKey)(priv), (*PublicKey)(&priv.PublicKey)
}

// ToECDSA returns the private key as a *ecdsa.PrivateKey.
func (p *PrivateKey) ToECDSA() *ecdsa.PrivateKey {
	return (*ecdsa.PrivateKey)(p)
}

// Sign wraps ecdsa.Sign to sign an arbitrary length hash (which should be the result of hashing a larger message) using the private key.
// It returns the signature as a *Signature. The security of the private key depends on the entropy of rand (crypto/rand.Reader).
func (p *PrivateKey) Sign(hash []byte) (*Signature, error) {
	r, s, err := ecdsa.Sign(rand.Reader, p.ToECDSA(), hash)
	if err != nil {
		return nil, err
	}
	sig := &Signature{
		R: r,
		S: s,
	}
	return sig, nil
}
