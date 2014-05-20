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

To verify a secp256k1 signature, the following may be done:

 package main

 import (
 	"encoding/hex"
 	"github.com/conformal/btcec"
 	"github.com/conformal/btcwire"
 	"log"
 )

 func main() {
 	// Decode hex-encoded serialized public key.
 	pubKeyBytes, err := hex.DecodeString("02a673638cb9587cb68ea08dbef685c"+
 		"6f2d2a751a8b3c6f2a7e9a4999e6e4bfaf5")
 	if err != nil {
 		log.Fatal(err)
 	}
 	pubKey, err := btcec.ParsePubKey(pubKeyBytes, btcec.S256())
 	if err != nil {
 		log.Fatal(err)
 	}

 	// Decode hex-encoded serialized signature.
 	sigBytes, err := hex.DecodeString("30450220090ebfb3690a0ff115bb1b38b"+
 		"8b323a667b7653454f1bccb06d4bbdca42c2079022100ec95778b51e707"+
 		"1cb1205f8bde9af6592fc978b0452dafe599481c46d6b2e479")
 	if err != nil {
 		log.Fatal(err)
 	}
 	decodedSig, err := btcec.ParseSignature(sigBytes, btcec.S256())
 	if err != nil {
 		log.Fatal(err)
 	}

 	// Verify the signature for the message using the public key.
 	message := "test message"
 	messageHash := btcwire.DoubleSha256([]byte(message))
 	if decodedSig.Verify(messageHash, pubKey) {
 		log.Println("Signature Verified")
 	}
 }

To sign a message using a secp256k1 private key, the following may be done:

 package main

 import (
 	"encoding/hex"
 	"github.com/conformal/btcec"
 	"github.com/conformal/btcwire"
 	"log"
 )

 func main() {
 	// Decode a hex-encoded private key.
 	pkBytes, err := hex.DecodeString("22a47fa09a223f2aa079edf85a7c2d4f87"+
 		"20ee63e502ee2869afab7de234b80c")
 	if err != nil {
 		log.Fatal(err)
 	}
 	priv, pub := btcec.PrivKeyFromBytes(btcec.S256(), pkBytes)

 	// Sign a message using the private key.
 	message := "test message"
 	messageHash := btcwire.DoubleSha256([]byte(message))
 	sig, err := priv.Sign(messageHash)
 	if err != nil {
 		log.Fatal(err)
 	}

 	log.Printf("Serialized Signature: %x\n", sig.Serialize())

 	// Verify the signature for the message using the public key.
 	if sig.Verify(messageHash, pub) {
 		log.Println("Signature Verified")
 	}
 }
*/
package btcec
