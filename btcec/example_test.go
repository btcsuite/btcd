// Copyright (c) 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcec_test

import (
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/wire"
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
	messageHash := wire.DoubleSha256([]byte(message))
	signature, err := privKey.Sign(messageHash)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Serialize and display the signature.
	fmt.Printf("Serialized Signature: %x\n", signature.Serialize())

	// Verify the signature for the message using the public key.
	verified := signature.Verify(messageHash, pubKey)
	fmt.Printf("Signature Verified? %v\n", verified)

	// Output:
	// Serialized Signature: 304402201008e236fa8cd0f25df4482dddbb622e8a8b26ef0ba731719458de3ccd93805b022032f8ebe514ba5f672466eba334639282616bb3c2f0ab09998037513d1f9e3d6d
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
	messageHash := wire.DoubleSha256([]byte(message))
	verified := signature.Verify(messageHash, pubKey)
	fmt.Println("Signature Verified?", verified)

	// Output:
	// Signature Verified? true
}

// This example demonstrates encrypting a message for a public key that is first
// parsed from raw bytes, then decrypting it using the corresponding private key.
func Example_encryptMessage() {
	// Decode the hex-encoded pubkey of the recipient.
	pubKeyBytes, err := hex.DecodeString("04115c42e757b2efb7671c578530ec191a1" +
		"359381e6a71127a9d37c486fd30dae57e76dc58f693bd7e7010358ce6b165e483a29" +
		"21010db67ac11b1b51b651953d2") // uncompressed pubkey
	if err != nil {
		fmt.Println(err)
		return
	}
	pubKey, err := btcec.ParsePubKey(pubKeyBytes, btcec.S256())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Encrypt a message decryptable by the private key corresponding to pubKey
	message := "test message"
	ciphertext, err := btcec.Encrypt(pubKey, []byte(message))
	if err != nil {
		fmt.Println(err)
		return
	}

	// Decode the hex-encoded private key.
	pkBytes, err := hex.DecodeString("a11b0a4e1a132305652ee7a8eb7848f6ad" +
		"5ea381e3ce20a2c086a2e388230811")
	if err != nil {
		fmt.Println(err)
		return
	}
	// note that we already have corresponding pubKey
	privKey, _ := btcec.PrivKeyFromBytes(btcec.S256(), pkBytes)

	// Try decrypting and verify if it's the same message.
	plaintext, err := btcec.Decrypt(privKey, ciphertext)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(string(plaintext))

	// Output:
	// test message
}

// This example demonstrates decrypting a message using a private key that is
// first parsed from raw bytes.
func Example_decryptMessage() {
	// Decode the hex-encoded private key.
	pkBytes, err := hex.DecodeString("a11b0a4e1a132305652ee7a8eb7848f6ad" +
		"5ea381e3ce20a2c086a2e388230811")
	if err != nil {
		fmt.Println(err)
		return
	}

	privKey, _ := btcec.PrivKeyFromBytes(btcec.S256(), pkBytes)

	ciphertext, err := hex.DecodeString("35f644fbfb208bc71e57684c3c8b437402ca" +
		"002047a2f1b38aa1a8f1d5121778378414f708fe13ebf7b4a7bb74407288c1958969" +
		"00207cf4ac6057406e40f79961c973309a892732ae7a74ee96cd89823913b8b8d650" +
		"a44166dc61ea1c419d47077b748a9c06b8d57af72deb2819d98a9d503efc59fc8307" +
		"d14174f8b83354fac3ff56075162")

	// Try decrypting the message.
	plaintext, err := btcec.Decrypt(privKey, ciphertext)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(string(plaintext))

	// Output:
	// test message
}
