// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcec

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// GenerateSharedSecret generates a shared secret based on a private key and a
// public key using Diffie-Hellman key exchange (ECDH) (RFC 4753).
// RFC5903 Section 9 states we should only return x.
func GenerateSharedSecret(privkey *PrivateKey, pubkey *PublicKey) []byte {
	return secp.GenerateSharedSecret(privkey, pubkey)
}

// Encrypt encrypts data for the target public key using AES-128-GCM
func Encrypt(pubKey *PublicKey, msg []byte) ([]byte, error) {
	ephemeral, err := NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %v", err)
	}

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

	gcm, err := cipher.NewGCMWithNonceSize(block, 16)
	if err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, msg, nil)
	ciphertext = append(nonce, ciphertext...)
	return ciphertext, nil
}
