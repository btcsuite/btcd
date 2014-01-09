// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package btcec implements support for the elliptic curves needed for bitcoin.

Bitcoin uses elliptic curve cryptography using koblitz curves
(specifically secp256k1) for cryptographic functions.  See
http://www.secg.org/collateral/sec2_final.pdf for details on the
standard.

This package provides the data structures and functions implementing the
crypto/elliptic Curve interface in order to permit using these curves
with the standard crypto/ecdsa package provided with go. Helper
functionality is provided to parse signatures and public keys from
standard formats.  It was designed for use with btcd, but should be
general enough for other uses of elliptic curve crypto.  It was based on
some initial work by ThePiachu.

Usage

To verify a secp256k1 signature the following may be done:

 import crypto/ecdsa

 pubKey, err := btcec.ParsePubKey(pkStr, btcec.S256())

 signature, err := btcec.ParseSignature(sigStr, btcec.S256())

 ok := ecdsa.Verify(pubKey, message, signature.R, signature.S)

*/
package btcec
