// Copyright (c) 2013, 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil

import (
	"bytes"
	"code.google.com/p/go.crypto/ripemd160"
	"encoding/hex"
	"errors"
	"github.com/conformal/btcec"
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

// checkBitcoinNet returns an error if the bitcoin network is not supported.
func checkBitcoinNet(net btcwire.BitcoinNet) error {
	// Check for a valid bitcoin network.
	switch net {
	case btcwire.MainNet:
		fallthrough
	case btcwire.TestNet:
		fallthrough
	case btcwire.TestNet3:
		return nil

	default:
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

	// IsForNet returns whether or not the address is associated with the
	// passed bitcoin network.
	IsForNet(btcwire.BitcoinNet) bool
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

	default:
		return nil, errors.New("decoded address is of unknown size")
	}
}

// AddressPubKeyHash is an Address for a pay-to-pubkey-hash (P2PKH)
// transaction.
type AddressPubKeyHash struct {
	hash  [ripemd160.Size]byte
	netID byte
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

	// Choose the appropriate network ID for the address based on the
	// network.
	var netID byte
	switch net {
	case btcwire.MainNet:
		netID = MainNetAddr

	case btcwire.TestNet:
		fallthrough
	case btcwire.TestNet3:
		netID = TestNetAddr
	}

	addr := &AddressPubKeyHash{netID: netID}
	copy(addr.hash[:], pkHash)
	return addr, nil
}

// EncodeAddress returns the string encoding of a pay-to-pubkey-hash
// address.  Part of the Address interface.
func (a *AddressPubKeyHash) EncodeAddress() string {
	return encodeAddress(a.hash[:], a.netID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a pubkey hash.  Part of the Address interface.
func (a *AddressPubKeyHash) ScriptAddress() []byte {
	return a.hash[:]
}

// IsForNet returns whether or not the pay-to-pubkey-hash address is associated
// with the passed bitcoin network.
func (a *AddressPubKeyHash) IsForNet(net btcwire.BitcoinNet) bool {
	switch net {
	case btcwire.MainNet:
		return a.netID == MainNetAddr

	case btcwire.TestNet:
		fallthrough
	case btcwire.TestNet3:
		return a.netID == TestNetAddr
	}

	return false
}

// String returns a human-readable string for the pay-to-pubkey-hash address.
// This is equivalent to calling EncodeAddress, but is provided so the type can
// be used as a fmt.Stringer.
func (a *AddressPubKeyHash) String() string {
	return a.EncodeAddress()
}

// AddressScriptHash is an Address for a pay-to-script-hash (P2SH)
// transaction.
type AddressScriptHash struct {
	hash  [ripemd160.Size]byte
	netID byte
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

	// Choose the appropriate network ID for the address based on the
	// network.
	var netID byte
	switch net {
	case btcwire.MainNet:
		netID = MainNetScriptHash

	case btcwire.TestNet:
		fallthrough
	case btcwire.TestNet3:
		netID = TestNetScriptHash
	}

	addr := &AddressScriptHash{netID: netID}
	copy(addr.hash[:], scriptHash)
	return addr, nil
}

// EncodeAddress returns the string encoding of a pay-to-script-hash
// address.  Part of the Address interface.
func (a *AddressScriptHash) EncodeAddress() string {
	return encodeAddress(a.hash[:], a.netID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a script hash.  Part of the Address interface.
func (a *AddressScriptHash) ScriptAddress() []byte {
	return a.hash[:]
}

// IsForNet returns whether or not the pay-to-script-hash address is associated
// with the passed bitcoin network.
func (a *AddressScriptHash) IsForNet(net btcwire.BitcoinNet) bool {
	switch net {
	case btcwire.MainNet:
		return a.netID == MainNetScriptHash
	case btcwire.TestNet:
		fallthrough
	case btcwire.TestNet3:
		return a.netID == TestNetScriptHash
	}

	return false
}

// String returns a human-readable string for the pay-to-script-hash address.
// This is equivalent to calling EncodeAddress, but is provided so the type can
// be used as a fmt.Stringer.
func (a *AddressScriptHash) String() string {
	return a.EncodeAddress()
}

// PubKeyFormat describes what format to use for a pay-to-pubkey address.
type PubKeyFormat int

const (
	// PKFUncompressed indicates the pay-to-pubkey address format is an
	// uncompressed public key.
	PKFUncompressed PubKeyFormat = iota

	// PKFCompressed indicates the pay-to-pubkey address format is a
	// compressed public key.
	PKFCompressed

	// PKFHybrid indicates the pay-to-pubkey address format is a hybrid
	// public key.
	PKFHybrid
)

// AddressPubKey is an Address for a pay-to-pubkey transaction.
type AddressPubKey struct {
	pubKeyFormat PubKeyFormat
	pubKey       *btcec.PublicKey
	netID        byte
}

// NewAddressPubKey returns a new AddressPubKey which represents a pay-to-pubkey
// address.  The serializedPubKey parameter must be a valid pubkey and can be
// uncompressed, compressed, or hybrid.  The net parameter must be
// btcwire.MainNet or btcwire.TestNet3.
func NewAddressPubKey(serializedPubKey []byte, net btcwire.BitcoinNet) (*AddressPubKey, error) {
	pubKey, err := btcec.ParsePubKey(serializedPubKey, btcec.S256())
	if err != nil {
		return nil, err
	}

	// Set the format of the pubkey.  This probably should be returned
	// from btcec, but do it here to avoid API churn.  We already know the
	// pubkey is valid since it parsed above, so it's safe to simply examine
	// the leading byte to get the format.
	pkFormat := PKFUncompressed
	switch serializedPubKey[0] {
	case 0x02:
		fallthrough
	case 0x03:
		pkFormat = PKFCompressed

	case 0x06:
		fallthrough
	case 0x07:
		pkFormat = PKFHybrid
	}

	// Check for a valid bitcoin network.
	if err := checkBitcoinNet(net); err != nil {
		return nil, err
	}

	// Choose the appropriate network ID for the address based on the
	// network.
	var netID byte
	switch net {
	case btcwire.MainNet:
		netID = MainNetAddr

	case btcwire.TestNet:
		fallthrough
	case btcwire.TestNet3:
		netID = TestNetAddr
	}

	ecPubKey := (*btcec.PublicKey)(pubKey)
	return &AddressPubKey{
		pubKeyFormat: pkFormat,
		pubKey:       ecPubKey,
		netID:        netID,
	}, nil
}

// serialize returns the serialization of the public key according to the
// format associated with the address.
func (a *AddressPubKey) serialize() []byte {
	var serializedPubKey []byte
	switch a.pubKeyFormat {
	default:
		fallthrough
	case PKFUncompressed:
		serializedPubKey = a.pubKey.SerializeUncompressed()

	case PKFCompressed:
		serializedPubKey = a.pubKey.SerializeCompressed()

	case PKFHybrid:
		serializedPubKey = a.pubKey.SerializeHybrid()
	}

	return serializedPubKey
}

// EncodeAddress returns the string encoding of the public key as a
// pay-to-pubkey-hash.  Note that the public key format (uncompressed,
// compressed, etc) will change the resulting address.  This is expected since
// pay-to-pubkey-hash is a hash of the serialized public key which obviously
// differs with the format.  At the time of this writing, most Bitcoin addresses
// are pay-to-pubkey-hash constructed from the uncompressed public key.
//
// Part of the Address interface.
func (a *AddressPubKey) EncodeAddress() string {
	return encodeAddress(Hash160(a.serialize()), a.netID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a public key.  Setting the public key format will affect the output of
// this function accordingly.  Part of the Address interface.
func (a *AddressPubKey) ScriptAddress() []byte {
	return a.serialize()
}

// IsForNet returns whether or not the pay-to-pubkey address is associated
// with the passed bitcoin network.
func (a *AddressPubKey) IsForNet(net btcwire.BitcoinNet) bool {
	switch net {
	case btcwire.MainNet:
		return a.netID == MainNetAddr

	case btcwire.TestNet:
		fallthrough
	case btcwire.TestNet3:
		return a.netID == TestNetAddr
	}

	return false
}

// String returns the hex-encoded human-readable string for the pay-to-pubkey
// address.  This is not the same as calling EncodeAddress.
func (a *AddressPubKey) String() string {
	return hex.EncodeToString(a.serialize())
}

// PubKeyFormat returns the format (uncompressed, compressed, etc) of the
// pay-to-pubkey address.
func (a *AddressPubKey) Format() PubKeyFormat {
	return a.pubKeyFormat
}

// SetFormat sets the format (uncompressed, compressed, etc) of the
// pay-to-pubkey address.
func (a *AddressPubKey) SetFormat(pkFormat PubKeyFormat) {
	a.pubKeyFormat = pkFormat
}

// AddressPubKeyHash returns the pay-to-pubkey address converted to a
// pay-to-pubkey-hash address.  Note that the public key format (uncompressed,
// compressed, etc) will change the resulting address.  This is expected since
// pay-to-pubkey-hash is a hash of the serialized public key which obviously
// differs with the format.  At the time of this writing, most Bitcoin addresses
// are pay-to-pubkey-hash constructed from the uncompressed public key.
func (a *AddressPubKey) AddressPubKeyHash() *AddressPubKeyHash {
	addr := &AddressPubKeyHash{netID: a.netID}
	copy(addr.hash[:], Hash160(a.serialize()))
	return addr

}
