// Copyright (c) 2013, 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil

import (
	"bytes"
	"code.google.com/p/go.crypto/ripemd160"
	"errors"
	"github.com/conformal/btcwire"
)

var (
	// ErrChecksumMismatch describes an error where decoding failed due
	// to a bad checksum.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrUnknownIdentifier describes an error where decoding failed due
	// to an unknown magic byte identifier.
	ErrUnknownIdentifier = errors.New("unknown identifier byte")
)

// checkBitcoinNet returns an error is the bitcoin network is not supported.
func checkBitcoinNet(net btcwire.BitcoinNet) error {
	// Check for a valid bitcoin network.
	if !(net == btcwire.MainNet || net == btcwire.TestNet3) {
		return ErrUnknownNet
	}

	return nil
}

// encodeAddress returns a human-readable payment address given a ripemd160 hash
// and netid which encodes the bitcoin network and address type.  It is used
// in both pay-to-pubkey-hash (P2PKH) and pay-to-script-hash (P2SH) address
// encoding.
func encodeAddress(hash160 []byte, netID byte) string {
	tosum := make([]byte, ripemd160.Size+1)
	tosum[0] = netID
	copy(tosum[1:], hash160)
	cksum := btcwire.DoubleSha256(tosum)

	// Address before base58 encoding is 1 byte for netID, ripemd160 hash
	// size, plus 4 bytes of checksum (total 25).
	b := make([]byte, ripemd160.Size+5, ripemd160.Size+5)
	b[0] = netID
	copy(b[1:], hash160)
	copy(b[ripemd160.Size+1:], cksum[:4])

	return Base58Encode(b)

}

// Address is an interface type for any type of destination a transaction
// output may spend to.  This includes pay-to-pubkey (P2PK), pay-to-pubkey-hash
// (P2PKH), and pay-to-script-hash (P2SH).  Address is designed to be generic
// enough that other kinds of addresses may be added in the future without
// changing the decoding and encoding API.
type Address interface {
	// EncodeAddress returns the string encoding of the address.
	EncodeAddress() string

	// ScriptAddress returns the raw bytes of the address to be used
	// when inserting the address into a txout's script.
	ScriptAddress() []byte
}

// DecodeAddr decodes the string encoding of an address and returns
// the Address if addr is a valid encoding for a known address type.
//
// This is named DecodeAddr and not DecodeAddress due to DecodeAddress
// already being defined for an old api.  When the old api is eventually
// removed, a proper DecodeAddress function will be added, and DecodeAddr
// will become deprecated.
func DecodeAddr(addr string) (Address, error) {
	decoded := Base58Decode(addr)

	// Switch on decoded length to determine the type.
	switch len(decoded) {
	case 1 + ripemd160.Size + 4: // P2PKH or P2SH
		// Parse the network and hash type (pubkey hash vs script
		// hash) from the first byte.
		net := btcwire.MainNet
		isscript := false
		switch decoded[0] {
		case MainNetAddr:
			// Use defaults.

		case TestNetAddr:
			net = btcwire.TestNet3

		case MainNetScriptHash:
			isscript = true

		case TestNetScriptHash:
			isscript = true
			net = btcwire.TestNet3

		default:
			return nil, ErrUnknownIdentifier
		}

		// Verify hash checksum.  Checksum is calculated as the first
		// four bytes of double SHA256 of the network byte and hash.
		tosum := decoded[:ripemd160.Size+1]
		cksum := btcwire.DoubleSha256(tosum)[:4]
		if !bytes.Equal(cksum, decoded[len(decoded)-4:]) {
			return nil, ErrChecksumMismatch
		}

		// Return concrete type.
		if isscript {
			return NewAddressScriptHashFromHash(
				decoded[1:ripemd160.Size+1], net)
		}
		return NewAddressPubKeyHash(decoded[1:ripemd160.Size+1],
			net)

	case 33: // Compressed pubkey
		fallthrough

	case 65: // Uncompressed pubkey
		// TODO(jrick)
		return nil, errors.New("pay-to-pubkey unimplemented")

	default:
		return nil, errors.New("decoded address is of unknown size")
	}
}

// AddressPubKeyHash is an Address for a pay-to-pubkey-hash (P2PKH)
// transaction.
type AddressPubKeyHash struct {
	hash [ripemd160.Size]byte
	net  btcwire.BitcoinNet
}

// NewAddressPubKeyHash returns a new AddressPubKeyHash.  pkHash must
// be 20 bytes and net must be btcwire.MainNet or btcwire.TestNet3.
func NewAddressPubKeyHash(pkHash []byte, net btcwire.BitcoinNet) (*AddressPubKeyHash, error) {
	// Check for a valid pubkey hash length.
	if len(pkHash) != ripemd160.Size {
		return nil, errors.New("pkHash must be 20 bytes")
	}

	// Check for a valid bitcoin network.
	if err := checkBitcoinNet(net); err != nil {
		return nil, err
	}

	addr := &AddressPubKeyHash{net: net}
	copy(addr.hash[:], pkHash)
	return addr, nil
}

// EncodeAddress returns the string encoding of a pay-to-pubkey-hash
// address.  Part of the Address interface.
func (a *AddressPubKeyHash) EncodeAddress() string {
	var netID byte
	switch a.net {
	case btcwire.MainNet:
		netID = MainNetAddr
	case btcwire.TestNet3:
		netID = TestNetAddr
	}

	return encodeAddress(a.hash[:], netID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a pubkey hash.  Part of the Address interface.
func (a *AddressPubKeyHash) ScriptAddress() []byte {
	return a.hash[:]
}

// Net returns the bitcoin network associated with the pay-to-pubkey-hash
// address.
func (a *AddressPubKeyHash) Net() btcwire.BitcoinNet {
	return a.net
}

// AddressScriptHash is an Address for a pay-to-script-hash (P2SH)
// transaction.
type AddressScriptHash struct {
	hash [ripemd160.Size]byte
	net  btcwire.BitcoinNet
}

// NewAddressScriptHash returns a new AddressScriptHash.  net must be
// btcwire.MainNet or btcwire.TestNet3.
func NewAddressScriptHash(serializedScript []byte, net btcwire.BitcoinNet) (*AddressScriptHash, error) {
	// Create hash of serialized script.
	scriptHash := Hash160(serializedScript)

	return NewAddressScriptHashFromHash(scriptHash, net)
}

// NewAddressScriptHashFromHash returns a new AddressScriptHash.  scriptHash
// must be 20 bytes and net must be btcwire.MainNet or btcwire.TestNet3.
func NewAddressScriptHashFromHash(scriptHash []byte, net btcwire.BitcoinNet) (*AddressScriptHash, error) {
	// Check for a valid script hash length.
	if len(scriptHash) != ripemd160.Size {
		return nil, errors.New("scriptHash must be 20 bytes")
	}

	// Check for a valid bitcoin network.
	if err := checkBitcoinNet(net); err != nil {
		return nil, err
	}

	addr := &AddressScriptHash{net: net}
	copy(addr.hash[:], scriptHash)
	return addr, nil
}

// EncodeAddress returns the string encoding of a pay-to-script-hash
// address.  Part of the Address interface.
func (a *AddressScriptHash) EncodeAddress() string {
	var netID byte
	switch a.net {
	case btcwire.MainNet:
		netID = MainNetScriptHash
	case btcwire.TestNet3:
		netID = TestNetScriptHash
	}

	return encodeAddress(a.hash[:], netID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a script hash.  Part of the Address interface.
func (a *AddressScriptHash) ScriptAddress() []byte {
	return a.hash[:]
}

// Net returns the bitcoin network associated with the pay-to-script-hash
// address.
func (a *AddressScriptHash) Net() btcwire.BitcoinNet {
	return a.net
}
