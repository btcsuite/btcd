// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcec_test

import (
	"encoding/hex"
	"fmt"

	"github.com/conformal/btcec"
	"github.com/conformal/btcwire"
)

// This example demonstrates signing a message with a secp256k1 private key that
// is first parsed form raw bytes and serializing the generated signature.
func Example_signMessage() {
	// Decode a hex-encoded private key.
	pkBytes, err := hex.DecodeString("22a47fa09a223f2aa079edf85a7c2d4f87" +
		"20ee63e502ee2869afab7de234b80c")
	if err != nil {
		fmt.Println(err)
		return
	}
	privKey, pubKey := btcec.PrivKeyFromBytes(btcec.S256(), pkBytes)

	// Sign a message using the private key.
	message := "test message"
	messageHash := btcwire.DoubleSha256([]byte(message))
	signature, err := privKey.Sign(messageHash)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Serialize and display the signature.
	//
	// NOTE: This is commented out for the example since the signature
	// produced uses random numbers and therefore will always be different.
	//fmt.Printf("Serialized Signature: %x\n", signature.Serialize())

	// Verify the signature for the message using the public key.
	verified := signature.Verify(messageHash, pubKey)
	fmt.Printf("Signature Verified? %v\n", verified)

	// Output:
	// Signature Verified? true
}

// This example demonstrates verifying a secp256k1 signature against a public
// key that is first parsed from raw bytes.  The signature is also parsed from
// raw bytes.
func Example_verifySignature() {
	// Decode hex-encoded serialized public key.
	pubKeyBytes, err := hex.DecodeString("02a673638cb9587cb68ea08dbef685c" +
		"6f2d2a751a8b3c6f2a7e9a4999e6e4bfaf5")
	if err != nil {
		fmt.Println(err)
		return
	}
	pubKey, err := btcec.ParsePubKey(pubKeyBytes, btcec.S256())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Decode hex-encoded serialized signature.
	sigBytes, err := hex.DecodeString("30450220090ebfb3690a0ff115bb1b38b" +
		"8b323a667b7653454f1bccb06d4bbdca42c2079022100ec95778b51e707" +
		"1cb1205f8bde9af6592fc978b0452dafe599481c46d6b2e479")
	if err != nil {
		fmt.Println(err)
		return
	}
	signature, err := btcec.ParseSignature(sigBytes, btcec.S256())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Verify the signature for the message using the public key.
	message := "test message"
	messageHash := btcwire.DoubleSha256([]byte(message))
	verified := signature.Verify(messageHash, pubKey)
	fmt.Println("Signature Verified?", verified)

	// Output:
	// Signature Verified? true
}
