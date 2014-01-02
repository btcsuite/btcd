// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil

import (
	"bytes"
	"code.google.com/p/go.crypto/ripemd160"
	"errors"
	"github.com/conformal/btcwire"
)

// ErrUnknownNet describes an error where the Bitcoin network is
// not recognized.
var ErrUnknownNet = errors.New("unrecognized bitcoin network")

// ErrMalformedAddress describes an error where an address is improperly
// formatted, either due to an incorrect length of the hashed pubkey or
// a non-matching checksum.
var ErrMalformedAddress = errors.New("malformed address")

// ErrMalformedPrivateKey describes an error where an address is improperly
// formatted, either due to an incorrect length of the private key or
// a non-matching checksum.
var ErrMalformedPrivateKey = errors.New("malformed private key")

// Constants used to specify which network a payment address belongs
// to.  Mainnet address cannot be used on the Testnet, and vice versa.
const (
	// MainNetAddr is the address identifier for MainNet
	MainNetAddr = 0x00

	// TestNetAddr is the address identifier for TestNet
	TestNetAddr = 0x6f

	// MainNetKey is the key identifier for MainNet
	MainNetKey = 0x80

	// TestNetKey is the key identifier for TestNet
	TestNetKey = 0xef

	// MainNetScriptHash is the script hash identifier for MainNet
	MainNetScriptHash = 0x05

	// TestNetScriptHash is the script hash identifier for TestNet
	TestNetScriptHash = 0xc4
)

// EncodeAddress takes a 20-byte raw payment address (hash160 of a pubkey)
// and the Bitcoin network to create a human-readable payment address string.
func EncodeAddress(addrHash []byte, net btcwire.BitcoinNet) (encoded string, err error) {
	if len(addrHash) != ripemd160.Size {
		return "", ErrMalformedAddress
	}

	var netID byte
	switch net {
	case btcwire.MainNet:
		netID = MainNetAddr
	case btcwire.TestNet3:
		netID = TestNetAddr
	default:
		return "", ErrUnknownNet
	}

	return encodeHashWithNetId(netID, addrHash)
}

// EncodeScriptHash takes a 20-byte raw script hash (hash160 of the SHA256 of the redeeming script)
// and the Bitcoin network to create a human-readable payment address string.
func EncodeScriptHash(addrHash []byte, net btcwire.BitcoinNet) (encoded string, err error) {
	if len(addrHash) != ripemd160.Size {
		return "", ErrMalformedAddress
	}

	var netID byte
	switch net {
	case btcwire.MainNet:
		netID = MainNetScriptHash
	case btcwire.TestNet3:
		netID = TestNetScriptHash
	default:
		return "", ErrUnknownNet
	}

	return encodeHashWithNetId(netID, addrHash)
}

func encodeHashWithNetId(netID byte, addrHash []byte) (encoded string, err error) {
	tosum := append([]byte{netID}, addrHash...)
	cksum := btcwire.DoubleSha256(tosum)

	// Address before base58 encoding is 1 byte for netID, 20 bytes for
	// hash, plus 4 bytes of checksum.
	a := make([]byte, 25, 25)
	a[0] = netID
	copy(a[1:], addrHash)
	copy(a[21:], cksum[:4])

	return Base58Encode(a), nil
}

// DecodeAddress decodes a human-readable payment address string
// returning the 20-byte decoded address, along with the Bitcoin
// network for the address.
func DecodeAddress(addr string) (addrHash []byte, net btcwire.BitcoinNet, err error) {
	decoded := Base58Decode(addr)

	// Length of decoded address must be 20 bytes + 1 byte for a network
	// identifier byte + 4 bytes of checksum.
	if len(decoded) != ripemd160.Size+5 {
		return nil, 0x00, ErrMalformedAddress
	}

	switch decoded[0] {
	case MainNetAddr:
		net = btcwire.MainNet
	case TestNetAddr:
		net = btcwire.TestNet3
	default:
		return nil, 0, ErrUnknownNet
	}

	// Checksum is first four bytes of double SHA256 of the network byte
	// and addrHash.  Verify this matches the final 4 bytes of the decoded
	// address.
	tosum := decoded[:ripemd160.Size+1]
	cksum := btcwire.DoubleSha256(tosum)[:4]
	if !bytes.Equal(cksum, decoded[len(decoded)-4:]) {
		return nil, net, ErrMalformedAddress
	}

	addrHash = make([]byte, ripemd160.Size, ripemd160.Size)
	copy(addrHash, decoded[1:ripemd160.Size+1])

	return addrHash, net, nil
}

// EncodePrivateKey takes a 32-byte private key and encodes it into the
// Wallet Import Format (WIF).
func EncodePrivateKey(privKey []byte, net btcwire.BitcoinNet, compressed bool) (string, error) {
	if len(privKey) != 32 {
		return "", ErrMalformedPrivateKey
	}

	var netID byte
	switch net {
	case btcwire.MainNet:
		netID = MainNetKey
	case btcwire.TestNet3:
		netID = TestNetKey
	default:
		return "", ErrUnknownNet
	}

	tosum := append([]byte{netID}, privKey...)
	if compressed {
		tosum = append(tosum, 0x01)
	}
	cksum := btcwire.DoubleSha256(tosum)

	// Private key before base58 encoding is 1 byte for netID, 32 bytes for
	// privKey, plus an optional byte (0x01) if copressed, plus 4 bytes of checksum.
	encodeLen := 37
	if compressed {
		encodeLen += 1
	}
	a := make([]byte, encodeLen, encodeLen)
	a[0] = netID
	copy(a[1:], privKey)
	if compressed {
		copy(a[32+1:], []byte{0x01})
		copy(a[32+1+1:], cksum[:4])
	} else {
		copy(a[32+1:], cksum[:4])
	}
	return Base58Encode(a), nil
}

// DecodePrivateKey takes a Wallet Import Format (WIF) string and
// decodes into a 32-byte private key.
func DecodePrivateKey(wif string) ([]byte, btcwire.BitcoinNet, bool, error) {
	decoded := Base58Decode(wif)
	decodedLen := len(decoded)
	compressed := false

	// Length of decoded privkey must be 32 bytes + an optional 1 byte (0x01)
	// if compressed, plus 1 byte for netID + 4 bytes of checksum
	if decodedLen == 32+6 {
		compressed = true
		if decoded[33] != 0x01 {
			return nil, 0, compressed, ErrMalformedPrivateKey
		}
	} else if decodedLen != 32+5 {
		return nil, 0, compressed, ErrMalformedPrivateKey
	}

	var net btcwire.BitcoinNet
	switch decoded[0] {
	case MainNetKey:
		net = btcwire.MainNet
	case TestNetKey:
		net = btcwire.TestNet3
	default:
		return nil, 0, compressed, ErrUnknownNet
	}

	// Checksum is first four bytes of double SHA256 of the identifier byte
	// and privKey.  Verify this matches the final 4 bytes of the decoded
	// private key.
	var tosum []byte
	if compressed {
		tosum = decoded[:32+1+1]
	} else {
		tosum = decoded[:32+1]
	}
	cksum := btcwire.DoubleSha256(tosum)[:4]
	if !bytes.Equal(cksum, decoded[decodedLen-4:]) {
		return nil, 0, compressed, ErrMalformedPrivateKey
	}

	privKey := make([]byte, 32, 32)
	copy(privKey[:], decoded[1:32+1])

	return privKey, net, compressed, nil
}
