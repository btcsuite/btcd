// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcec

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"math/big"
)

// GenerateSharedSecret generates a shared secret based on a private key and a
// public key using Diffie-Hellman key exchange (ECDH) (RFC 4753).
// RFC5903 Section 9 states we should only return x.
func GenerateSharedSecret(privkey *PrivateKey, pubkey *PublicKey) []byte {
	return secp.GenerateSharedSecret(privkey, pubkey)
}

// Encrypt encrypts a message using a public key, returning the encrypted message or an error.
// It generates an ephemeral key, derives a shared secret and an encryption key, then encrypts the message using AES-GCM.
// The ephemeral public key, nonce, tag and encrypted message are then combined and returned as a single byte slice.
func Encrypt(pubKey *PublicKey, msg []byte) ([]byte, error) {
	var pt bytes.Buffer

	ephemeral, err := NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %v", err)
	}

	pt.Write(ephemeral.PubKey().SerializeUncompressed())

	ecdhKey := GenerateSharedSecret(ephemeral, pubKey)
	hashedSecret := sha256.Sum256(ecdhKey)
	encryptionKey := hashedSecret[:16]

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	pt.Write(nonce)

	gcm, err := cipher.NewGCMWithNonceSize(block, 16)
	if err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, msg, nil)

	tag := ciphertext[len(ciphertext)-gcm.NonceSize():]
	pt.Write(tag)
	ciphertext = ciphertext[:len(ciphertext)-len(tag)]
	pt.Write(ciphertext)

	return pt.Bytes(), nil
}

// Decrypt decrypts data that was encrypted using the Encrypt function.
// The decrypted message is returned if the decryption is successful, or an error is returned if there are any issues.
func Decrypt(privkey *PrivateKey, msg []byte) ([]byte, error) {
	// Message cannot be less than length of public key (65) + nonce (16) + tag (16)
	if len(msg) <= (1 + 32 + 32 + 16 + 16) {
		return nil, fmt.Errorf("invalid length of message")
	}

	pb := new(big.Int).SetBytes(msg[:65]).Bytes()
	pubKey, err := ParsePubKey(pb)
	if err != nil {
		return nil, err
	}

	ecdhKey := GenerateSharedSecret(privkey, pubKey)
	hashedSecret := sha256.Sum256(ecdhKey)
	encryptionKey := hashedSecret[:16]

	msg = msg[65:]
	nonce := msg[:16]
	tag := msg[16:32]

	ciphertext := bytes.Join([][]byte{msg[32:], tag}, nil)

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("cannot create new aes block: %w", err)
	}

	gcm, err := cipher.NewGCMWithNonceSize(block, 16)
	if err != nil {
		return nil, fmt.Errorf("cannot create gcm cipher: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt ciphertext: %w", err)
	}

	return plaintext, nil
}
