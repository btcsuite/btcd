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

// Encrypt encrypts data for the target public key using AES-128-GCM
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

// Decrypt decrypts a passed message with a receiver private key, returns plaintext or decryption error
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
