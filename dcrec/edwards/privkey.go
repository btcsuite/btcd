// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package edwards

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha512"
	"fmt"
	"math/big"

	"github.com/agl/ed25519"
)

// These constants define the lengths of serialized private keys.
const (
	PrivScalarSize  = 32
	PrivKeyBytesLen = 64
)

// PrivateKey wraps an ecdsa.PrivateKey as a convenience mainly for signing
// things with the the private key without having to directly import the ecdsa
// package.
type PrivateKey struct {
	ecPk   *ecdsa.PrivateKey
	secret *[32]byte
}

// NewPrivateKey instantiates a new private key from a scalar encoded as a
// big integer.
func NewPrivateKey(curve *TwistedEdwardsCurve, d *big.Int) *PrivateKey {
	dArray := BigIntToEncodedBytes(d)
	priv, _ := PrivKeyFromSecret(curve, dArray[:])
	return priv
}

// GeneratePrivateKey is a wrapper for ecdsa.GenerateKey that returns a
// PrivateKey instead of the normal ecdsa.PrivateKey.
func GeneratePrivateKey(curve *TwistedEdwardsCurve) (*PrivateKey, error) {
	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, err
	}

	pk := new(PrivateKey)
	pk.ecPk = key

	return pk, nil
}

// computeScalar obtains a private scalar from a private key.
func computeScalar(privateKey *[PrivKeyBytesLen]byte) *[PrivScalarSize]byte {
	h := sha512.New()
	h.Write(privateKey[:32])
	digest := h.Sum(nil)

	digest[0] &= 248  // Make a multiple of 8
	digest[31] &= 127 // Zero the first bit of this byte
	digest[31] |= 64  // Enable the seventh bit

	var hBytes [PrivScalarSize]byte
	copy(hBytes[:], digest)

	return &hBytes
}

// PrivKeyFromBytes returns a private and public key for `curve' based on the
// private key passed as an argument as a byte slice.
func PrivKeyFromBytes(curve *TwistedEdwardsCurve,
	pkBytes []byte) (*PrivateKey, *PublicKey) {
	if len(pkBytes) != PrivKeyBytesLen {
		return nil, nil
	}
	pk := copyBytes64(pkBytes)

	// The ed25519 library does a weird thing where it generates a
	// private key 64 bytes long, and stores the private scalar in
	// the first 32 bytes and the public key in the last 32 bytes.
	// So, make sure we only grab the actual scalar we care about.
	privKeyBytes := pk[0:32]
	pubKeyBytes := pk[32:64]
	pubKey, err := ParsePubKey(curve, pubKeyBytes)
	if err != nil {
		return nil, nil
	}

	priv := &ecdsa.PrivateKey{
		PublicKey: *pubKey.ToECDSA(),
		D:         new(big.Int).SetBytes(computeScalar(pk)[:]),
	}
	privEd := new(PrivateKey)
	privEd.ecPk = priv
	privEd.secret = copyBytes(privKeyBytes)

	return privEd, (*PublicKey)(&priv.PublicKey)
}

// PrivKeyFromSecret returns a private and public key for `curve' based on the
// 32-byte private key secret passed as an argument as a byte slice.
func PrivKeyFromSecret(curve *TwistedEdwardsCurve, s []byte) (*PrivateKey,
	*PublicKey) {
	if len(s) != PrivKeyBytesLen/2 {
		return nil, nil
	}

	// Instead of using rand to generate our scalar, use the scalar
	// itself as a reader.
	sReader := bytes.NewReader(s)
	_, pk, err := ed25519.GenerateKey(sReader)
	if err != nil {
		return nil, nil
	}

	return PrivKeyFromBytes(curve, pk[:])
}

// PrivKeyFromScalar returns a private and public key for `curve' based on the
// 32-byte private scalar passed as an argument as a byte slice (encoded big
// endian int).
func PrivKeyFromScalar(curve *TwistedEdwardsCurve, p []byte) (*PrivateKey,
	*PublicKey, error) {
	if len(p) != PrivScalarSize {
		return nil, nil, fmt.Errorf("bad private scalar size")
	}

	pk := new(PrivateKey)
	pk.ecPk = new(ecdsa.PrivateKey)
	pk.ecPk.D = new(big.Int).SetBytes(p)

	// The scalar must be in the subgroup.
	if pk.ecPk.D.Cmp(curve.N) > 0 {
		return nil, nil, fmt.Errorf("not on subgroup (>N)")
	}

	// The scalar must not be zero or negative.
	if pk.ecPk.D.Cmp(zero) <= 0 {
		return nil, nil, fmt.Errorf("zero or negative scalar")
	}

	pk.ecPk.Curve = curve
	pk.ecPk.PublicKey.X, pk.ecPk.PublicKey.Y =
		curve.ScalarBaseMult(pk.GetD().Bytes())

	if pk.ecPk.PublicKey.X == nil || pk.ecPk.PublicKey.Y == nil {
		return nil, nil, fmt.Errorf("scalarbase mult failure to get pubkey")
	}
	pub := PublicKey(pk.ecPk.PublicKey)

	return pk, &pub, nil
}

// Public returns the PublicKey corresponding to this private key.
func (p PrivateKey) Public() (*big.Int, *big.Int) {
	return p.ecPk.PublicKey.X, p.ecPk.PublicKey.Y
}

// ToECDSA returns the private key as a *ecdsa.PrivateKey.
func (p PrivateKey) ToECDSA() *ecdsa.PrivateKey {
	return p.ecPk
}

// Serialize returns the private key as a 32 byte big endian number.
func (p PrivateKey) Serialize() []byte {
	if p.ecPk.D == nil ||
		p.ecPk.PublicKey.X == nil ||
		p.ecPk.PublicKey.Y == nil {
		return nil
	}
	privateScalar := copyBytes(p.ecPk.D.Bytes())

	return privateScalar[:]
}

// SerializeSecret returns the 32 byte secret along with its public key as 64
// bytes.
func (p PrivateKey) SerializeSecret() []byte {
	if p.secret == nil {
		return nil
	}

	// This is little endian.
	pubX, pubY := p.Public()
	spk := BigIntPointToEncodedBytes(pubX, pubY)

	all := append(p.secret[:], spk[:]...)

	return all
}

// GetD satisfies the chainec PrivateKey interface.
func (p PrivateKey) GetD() *big.Int {
	return p.ecPk.D
}

// GetType satisfies the chainec PrivateKey interface.
func (p PrivateKey) GetType() int {
	return ecTypeEdwards
}
