package v2transport

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"

	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// rekeyInterval is the number of messages that can be encrypted or
	// decrypted with a single key before we rotate keys.
	rekeyInterval = 224

	// keySize is the size of the keys used.
	keySize = 32
)

// FSChaCha20Poly1305 is a wrapper around ChaCha20Poly1305 that changes its
// nonce after every message and is rekeyed after every rekeying interval.
type FSChaCha20Poly1305 struct {
	key       []byte
	packetCtr uint64
	cipher    cipher.AEAD
}

// NewFSChaCha20Poly1305 creates a new instance of FSChaCha20Poly1305.
func NewFSChaCha20Poly1305(initialKey []byte) (*FSChaCha20Poly1305, error) {
	cipher, err := chacha20poly1305.New(initialKey)
	if err != nil {
		return nil, err
	}

	f := &FSChaCha20Poly1305{
		key:       initialKey,
		packetCtr: 0,
		cipher:    cipher,
	}

	return f, nil
}

// Encrypt encrypts the plaintext using the associated data, returning the
// ciphertext or an error.
func (f *FSChaCha20Poly1305) Encrypt(aad, plaintext []byte) ([]byte, error) {
	return f.crypt(aad, plaintext, false)
}

// Decrypt decrypts the ciphertext using the assosicated data, returning the
// plaintext or an error.
func (f *FSChaCha20Poly1305) Decrypt(aad, ciphertext []byte) ([]byte, error) {
	return f.crypt(aad, ciphertext, true)
}

// crypt takes the aad and plaintext/ciphertext and either encrypts or decrypts
// `text` and returns the result. If a failure was encountered, an error will
// be returned.
func (f *FSChaCha20Poly1305) crypt(aad, text []byte,
	decrypt bool) ([]byte, error) {

	// The nonce is constructed as the 4-byte little-endian encoding of the
	// number of messages crypted with the current key followed by the
	// 8-byte little-endian encoding of the number of re-keying performed.
	var nonce [12]byte
	numMsgs := uint32(f.packetCtr % rekeyInterval)
	numRekeys := uint64(f.packetCtr / rekeyInterval)
	binary.LittleEndian.PutUint32(nonce[0:4], numMsgs)
	binary.LittleEndian.PutUint64(nonce[4:12], numRekeys)

	var result []byte
	if decrypt {
		// Decrypt using the nonce, ciphertext, and aad.
		var err error
		result, err = f.cipher.Open(nil, nonce[:], text, aad)
		if err != nil {
			// It is ok to error here without incrementing
			// packetCtr because we will no longer be decrypting
			// any more messages.
			return nil, err
		}
	} else {
		// Encrypt using the nonce, plaintext, and aad.
		result = f.cipher.Seal(nil, nonce[:], text, aad)
	}

	f.packetCtr++

	// Rekey if we are at the rekeying interval.
	if f.packetCtr%rekeyInterval == 0 {
		var rekeyNonce [12]byte
		rekeyNonce[0] = 0xff
		rekeyNonce[1] = 0xff
		rekeyNonce[2] = 0xff
		rekeyNonce[3] = 0xff

		copy(rekeyNonce[4:], nonce[4:])

		var dummyPlaintext [32]byte
		f.key = f.cipher.Seal(
			nil, rekeyNonce[:], dummyPlaintext[:], nil,
		)[:keySize]
		cipher, err := chacha20poly1305.New(f.key)
		if err != nil {
			fmt.Println(err)
			return nil, err
		}
		f.cipher = cipher
	}

	return result, nil
}

// FSChaCha20 is a stream cipher that is used to encrypt the length of the
// packets. This cipher is rekeyed when chunkCtr reaches a multiple of
// rekeyInterval.
type FSChaCha20 struct {
	key      []byte
	chunkCtr uint64
	cipher   *chacha20.Cipher
}

// NewFSChaCha20 initializes a new FSChaCha20 cipher instance.
func NewFSChaCha20(initialKey []byte) (*FSChaCha20, error) {
	var initialNonce [12]byte
	binary.LittleEndian.PutUint64(initialNonce[4:12], 0)

	cipher, err := chacha20.NewUnauthenticatedCipher(
		initialKey, initialNonce[:],
	)
	if err != nil {
		return nil, err
	}

	return &FSChaCha20{
		key:      initialKey,
		chunkCtr: 0,
		cipher:   cipher,
	}, nil
}

// Crypt is used to either encrypt or decrypt text. This function is used for
// both encryption and decryption as the two operations are identical.
func (f *FSChaCha20) Crypt(text []byte) ([]byte, error) {
	// XOR the text with the keystream to get either the cipher or
	// plaintext.
	textLen := len(text)
	dst := make([]byte, textLen)
	f.cipher.XORKeyStream(dst, text)

	// Increment the chunkCtr every time this function is called.
	f.chunkCtr++

	// Check if we need to rekey.
	if f.chunkCtr%rekeyInterval == 0 {
		// Get the new key by getting 32 bytes from the keystream. Use
		// all 0's so that we can get the actual bytes from
		// XORKeyStream since the chacha20 library doesn't supply us
		// with the keystream's bytes directly.
		var (
			dummyXor [32]byte
			newKey   [32]byte
		)
		f.cipher.XORKeyStream(newKey[:], dummyXor[:])
		f.key = newKey[:]

		var nonce [12]byte
		numRekeys := f.chunkCtr / rekeyInterval
		binary.LittleEndian.PutUint64(nonce[4:12], numRekeys)

		cipher, err := chacha20.NewUnauthenticatedCipher(
			f.key, nonce[:],
		)
		if err != nil {
			return nil, err
		}

		f.cipher = cipher
	}

	return dst, nil
}
