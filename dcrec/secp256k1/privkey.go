// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"crypto/ecdsa"
	"crypto/rand"
	"io"
	"math/big"
)

// PrivateKey wraps an ecdsa.PrivateKey as a convenience mainly for signing
// things with the the private key without having to directly import the ecdsa
// package.
type PrivateKey ecdsa.PrivateKey

// NewPrivateKey instantiates a new private key from a scalar encoded as a
// big integer.
func NewPrivateKey(d *big.Int) *PrivateKey {
	b := make([]byte, 0, PrivKeyBytesLen)
	dB := paddedAppend(PrivKeyBytesLen, b, d.Bytes())
	priv, _ := PrivKeyFromBytes(dB)
	return priv
}

// PrivKeyFromBytes returns a private and public key for `curve' based on the
// private key passed as an argument as a byte slice.
func PrivKeyFromBytes(pk []byte) (*PrivateKey,
	*PublicKey) {
	curve := S256()
	x, y := curve.ScalarBaseMult(pk)

	priv := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: S256(),
			X:     x,
			Y:     y,
		},
		D: new(big.Int).SetBytes(pk),
	}

	return (*PrivateKey)(priv), (*PublicKey)(&priv.PublicKey)
}

// PrivKeyFromScalar is the same as PrivKeyFromBytes in secp256k1.
func PrivKeyFromScalar(s []byte) (*PrivateKey,
	*PublicKey) {
	return PrivKeyFromBytes(s)
}

// GeneratePrivateKey is a wrapper for ecdsa.GenerateKey that returns a PrivateKey
// instead of the normal ecdsa.PrivateKey.
func GeneratePrivateKey() (*PrivateKey, error) {
	key, err := ecdsa.GenerateKey(S256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	return (*PrivateKey)(key), nil
}

// GenerateKey generates a key using a random number generator, returning
// the private scalar and the corresponding public key points.
func GenerateKey(rand io.Reader) (priv []byte, x,
	y *big.Int, err error) {
	key, err := ecdsa.GenerateKey(S256(), rand)
	priv = key.D.Bytes()
	x = key.PublicKey.X
	y = key.PublicKey.Y

	return
}

// Public returns the PublicKey corresponding to this private key.
func (p PrivateKey) Public() (*big.Int, *big.Int) {
	return p.PublicKey.X, p.PublicKey.Y
}

// ToECDSA returns the private key as a *ecdsa.PrivateKey.
func (p *PrivateKey) ToECDSA() *ecdsa.PrivateKey {
	return (*ecdsa.PrivateKey)(p)
}

// Sign generates an ECDSA signature for the provided hash (which should be the
// result of hashing a larger message) using the private key. Produced signature
// is deterministic (same message and same key yield the same signature) and
// canonical in accordance with RFC6979 and BIP0062.
func (p *PrivateKey) Sign(hash []byte) (*Signature, error) {
	return signRFC6979(p, hash)
}

// PrivKeyBytesLen defines the length in bytes of a serialized private key.
const PrivKeyBytesLen = 32

// Serialize returns the private key number d as a big-endian binary-encoded
// number, padded to a length of 32 bytes.
func (p PrivateKey) Serialize() []byte {
	b := make([]byte, 0, PrivKeyBytesLen)
	return paddedAppend(PrivKeyBytesLen, b, p.ToECDSA().D.Bytes())
}

// SerializeSecret satisfies the chainec PrivateKey interface.
func (p PrivateKey) SerializeSecret() []byte {
	return p.Serialize()
}

// GetD satisfies the chainec PrivateKey interface.
func (p PrivateKey) GetD() *big.Int {
	return p.D
}

// GetType satisfies the chainec PrivateKey interface.
func (p PrivateKey) GetType() int {
	return ecTypeSecp256k1
}
