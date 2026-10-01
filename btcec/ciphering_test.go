// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcec

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGenerateSharedSecret(t *testing.T) {
	privKey1, err := NewPrivateKey()
	if err != nil {
		t.Errorf("private key generation error: %s", err)
		return
	}
	privKey2, err := NewPrivateKey()
	if err != nil {
		t.Errorf("private key generation error: %s", err)
		return
	}

	secret1 := GenerateSharedSecret(privKey1, privKey2.PubKey())
	secret2 := GenerateSharedSecret(privKey2, privKey1.PubKey())

	if !bytes.Equal(secret1, secret2) {
		t.Errorf("ECDH failed, secrets mismatch - first: %x, second: %x",
			secret1, secret2)
	}
}

func TestEncryptAndDecrypt(t *testing.T) {
	privateKey, err := NewPrivateKey()
	if err != nil {
		t.Errorf("private key generation error: %s", err)
		return
	}
	publicKey := privateKey.PubKey()
	message := []byte("Hello, this is a test message.")

	encryptedMessage, err := Encrypt(publicKey, message)
	if !assert.NoError(t, err) {
		return
	}

	fmt.Println("Encrypted Message:", hex.EncodeToString(encryptedMessage))

	decryptedMessage, err := Decrypt(privateKey, encryptedMessage)
	if !assert.NoError(t, err) {
		return
	}

	fmt.Println("Decrypted Message:", string(decryptedMessage))

	assert.Equal(t, string(message), string(decryptedMessage))

}
