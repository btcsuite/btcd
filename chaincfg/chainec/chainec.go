// Copyright (c) 2015-2016 The Decred Developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chainec

import (
	"crypto/ecdsa"
	"io"
	"math/big"
)

// PublicKey is an interface representing a public key and its associated
// functions.
type PublicKey interface {
	// Serialize is the default serialization method.
	Serialize() []byte

	// SerializeUncompressed serializes to the uncompressed format (if
	// available).
	SerializeUncompressed() []byte

	// SerializeCompressed serializes to the compressed format (if
	// available).
	SerializeCompressed() []byte

	// SerializeHybrid serializes to the hybrid format (if
	// available).
	SerializeHybrid() []byte

	// ToECDSA converts the public key to an ECDSA public key.
	ToECDSA() *ecdsa.PublicKey

	// GetCurve returns the current curve as an interface.
	GetCurve() interface{}

	// GetX returns the point's X value.
	GetX() *big.Int

	// GetY returns the point's Y value.
	GetY() *big.Int

	// GetType returns the ECDSA type of this key.
	GetType() int
}

// PrivateKey is an interface representing a private key and its associated
// functions.
type PrivateKey interface {
	// Serialize serializes the 32-byte private key scalar to a
	// byte slice.
	Serialize() []byte

	// SerializeSecret serializes the secret to the default serialization
	// format. Used for Ed25519.
	SerializeSecret() []byte

	// Public returns the (X,Y) coordinates of the point produced
	// by scalar multiplication of the scalar by the base point,
	// AKA the public key.
	Public() (*big.Int, *big.Int)

	// GetD returns the value of the private scalar.
	GetD() *big.Int

	// GetType returns the ECDSA type of this key.
	GetType() int
}

// Signature is an interface representing a signature and its associated
// functions.
type Signature interface {
	// Serialize serializes the signature to the default serialization
	// format.
	Serialize() []byte

	// GetR gets the R value of the signature.
	GetR() *big.Int

	// GetS gets the S value of the signature.
	GetS() *big.Int

	// GetType returns the ECDSA type of this key.
	GetType() int
}

// DSA is an encapsulating interface for all the functions of a digital
// signature algorithm.
type DSA interface {
	// ----------------------------------------------------------------------------
	// Constants
	//
	// GetP gets the prime modulus of the curve.
	GetP() *big.Int

	// GetN gets the prime order of the curve.
	GetN() *big.Int

	// ----------------------------------------------------------------------------
	// EC Math
	//
	// Add adds two points on the curve.
	Add(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int)

	// IsOnCurve checks if a given point is on the curve.
	IsOnCurve(x *big.Int, y *big.Int) bool

	// ScalarMult gives the product of scalar multiplication of scalar k
	// by point (x,y) on the curve.
	ScalarMult(x, y *big.Int, k []byte) (*big.Int, *big.Int)

	// ScalarBaseMult gives the product of scalar multiplication of
	// scalar k by the base point (generator) of the curve.
	ScalarBaseMult(k []byte) (*big.Int, *big.Int)

	// ----------------------------------------------------------------------------
	// Private keys
	//
	// NewPrivateKey instantiates a new private key for the given
	// curve.
	NewPrivateKey(*big.Int) PrivateKey

	// PrivKeyFromBytes calculates the public key from serialized bytes,
	// and returns both it and the private key.
	PrivKeyFromBytes(pk []byte) (PrivateKey, PublicKey)

	// PrivKeyFromScalar calculates the public key from serialized scalar
	// bytes, and returns both it and the private key. Useful for curves
	// like Ed25519, where serialized private keys are different from
	// serialized private scalars.
	PrivKeyFromScalar(pk []byte) (PrivateKey, PublicKey)

	// PrivKeyBytesLen returns the length of a serialized private key.
	PrivKeyBytesLen() int

	// ----------------------------------------------------------------------------
	// Public keys
	//
	// NewPublicKey instantiates a new public key (point) for the
	// given curve.
	NewPublicKey(x *big.Int, y *big.Int) PublicKey

	// ParsePubKey parses a serialized public key for the given
	// curve and returns a public key.
	ParsePubKey(pubKeyStr []byte) (PublicKey, error)

	// PubKeyBytesLen returns the length of the default serialization
	// method for a public key.
	PubKeyBytesLen() int

	// PubKeyBytesLenUncompressed returns the length of the uncompressed
	// serialization method for a public key.
	PubKeyBytesLenUncompressed() int

	// PubKeyBytesLenCompressed returns the length of the compressed
	// serialization method for a public key.
	PubKeyBytesLenCompressed() int

	// PubKeyBytesLenHybrid returns the length of the hybrid
	// serialization method for a public key.
	PubKeyBytesLenHybrid() int

	// ----------------------------------------------------------------------------
	// Signatures
	//
	// NewSignature instantiates a new signature for the given ECDSA
	// method.
	NewSignature(r *big.Int, s *big.Int) Signature

	// ParseDERSignature parses a DER encoded signature for the given
	// ECDSA method. If the method doesn't support DER signatures, it
	// just parses with the default method.
	ParseDERSignature(sigStr []byte) (Signature, error)

	// ParseSignature a default encoded signature for the given ECDSA
	// method.
	ParseSignature(sigStr []byte) (Signature, error)

	// RecoverCompact recovers a public key from an encoded signature
	// and message, then verifies the signature against the public
	// key.
	RecoverCompact(signature, hash []byte) (PublicKey, bool, error)

	// ----------------------------------------------------------------------------
	// ECDSA
	//
	// GenerateKey generates a new private and public keypair from the
	// given reader.
	GenerateKey(rand io.Reader) ([]byte, *big.Int, *big.Int, error)

	// Sign produces an ECDSA signature in the form of (R,S) using a
	// private key and a message.
	Sign(priv PrivateKey, hash []byte) (r, s *big.Int, err error)

	// Verify verifies an ECDSA signature against a given message and
	// public key.
	Verify(pub PublicKey, hash []byte, r, s *big.Int) bool

	// ----------------------------------------------------------------------------
	// Symmetric cipher encryption
	//
	// GenerateSharedSecret generates a shared secret using a private scalar
	// and a public key using ECDH.
	GenerateSharedSecret(privkey []byte, x, y *big.Int) []byte

	// Encrypt encrypts data to a recipient public key.
	Encrypt(x, y *big.Int, in []byte) ([]byte, error)

	// Decrypt decrypts data encoded to the public key that originates
	// from the passed private scalar.
	Decrypt(privkey []byte, in []byte) ([]byte, error)
}

// --------------------------------------------------------------------------------
// Accessible DSA suites for export.
//
const (
	ECTypeSecp256k1  int = iota // 0
	ECTypeEdwards               // 1
	ECTypeSecSchnorr            // 2
)

// Secp256k1 is the secp256k1 curve and ECDSA system used in Bitcoin.
var Secp256k1 = newSecp256k1DSA()

// Edwards is the Ed25519 ECDSA signature system.
var Edwards = newEdwardsDSA()

// SecSchnorr is a Schnorr signature scheme about the secp256k1 curve
// implemented in libsecp256k1.
var SecSchnorr = newSecSchnorrDSA()
