// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcec

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"math/big"
)

// PrivateKey wraps an ecdsa.PrivateKey as a convenience mainly for signing
// things with the the private key without having to directly import the ecdsa
// package.
type PrivateKey ecdsa.PrivateKey

// PrivKeyFromBytes returns a private and public key for `curve' based on the
// private key passed as an argument as a byte slice.
func PrivKeyFromBytes(curve elliptic.Curve, pk []byte) (*PrivateKey,
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

// NewPrivateKey is a wrapper for ecdsa.GenerateKey that returns a PrivateKey
// instead of the normal ecdsa.PrivateKey.
func NewPrivateKey(curve elliptic.Curve) (*PrivateKey, error) {
	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, err
	}
	return (*PrivateKey)(key), nil
}

// PubKey returns the PublicKey corresponding to this private key.
func (p *PrivateKey) PubKey() *PublicKey {
	return (*PublicKey)(&p.PublicKey)
}

// ToECDSA returns the private key as a *ecdsa.PrivateKey.
func (p *PrivateKey) ToECDSA() *ecdsa.PrivateKey {
	return (*ecdsa.PrivateKey)(p)
}

// Sign generates an ECDSA signature for the provided hash (which should be the result
// of hashing a larger message) using the private key. Produced signature
// is deterministic (same message and same key yield the same signature) and canonical
// in accordance with RFC6979 and BIP0062.
func (p *PrivateKey) Sign(hash []byte) (*Signature, error) {
	return signRFC6979(p, hash, 0)
}

// SignCanonical goes through signatures and returns only a canonical
// representations.  This matches the EOS blockchain expectations.
func (p *PrivateKey) SignCanonical(curve *KoblitzCurve, hash []byte) ([]byte, error) {
	for i := 0; i < 25; i++ {
		sig, err := signRFC6979(p, hash, i)
		if err != nil {
			return nil, err
		}

		compactSig, err := makeCompact(curve, sig, p, hash, true)
		if err != nil {
			continue
		}

		if isCanonical(compactSig) {
			return compactSig, nil
		}
	}
	return nil, errors.New("couldn't find a canonical signature")
}

// PrivKeyBytesLen defines the length in bytes of a serialized private key.
const PrivKeyBytesLen = 32

// Serialize returns the private key number d as a big-endian binary-encoded
// number, padded to a length of 32 bytes.
func (p *PrivateKey) Serialize() []byte {
	b := make([]byte, 0, PrivKeyBytesLen)
	return paddedAppend(PrivKeyBytesLen, b, p.ToECDSA().D.Bytes())
}

func isCanonical(compactSig []byte) bool {
	// From EOS's codebase, our way of doing Canonical sigs.
	// https://steemit.com/steem/@dantheman/steem-and-bitshares-cryptographic-security-update
	//
	// !(c.data[1] & 0x80)
	// && !(c.data[1] == 0 && !(c.data[2] & 0x80))
	// && !(c.data[33] & 0x80)
	// && !(c.data[33] == 0 && !(c.data[34] & 0x80));

	d := compactSig
	t1 := (d[1] & 0x80) == 0
	t2 := !(d[1] == 0 && ((d[2] & 0x80) == 0))
	t3 := (d[33] & 0x80) == 0
	t4 := !(d[33] == 0 && ((d[34] & 0x80) == 0))
	return t1 && t2 && t3 && t4
}
