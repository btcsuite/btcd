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
	// ErrUnknownNet describes an error where the Bitcoin network is
	// not recognized.
	ErrUnknownNet = errors.New("unrecognized bitcoin network")

	// ErrMalformedAddress describes an error where an address is improperly
	// formatted, either due to an incorrect length of the hashed pubkey or
	// a non-matching checksum.
	ErrMalformedAddress = errors.New("malformed address")

	// ErrMalformedPrivateKey describes an error where an address is
	// improperly formatted, either due to an incorrect length of the
	// private key or a non-matching checksum.
	ErrMalformedPrivateKey = errors.New("malformed private key")

	// ErrChecksumMismatch describes an error where decoding failed due
	// to a bad checksum.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrUnknownIdentifier describes an error where decoding failed due
	// to an unknown magic byte identifier.
	ErrUnknownIdentifier = errors.New("unknown identifier byte")
)

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

// DecodeAddress decodes the string encoding of an address and returns
// the Address if addr is a valid encoding for a known address type.
//
// The bitcoin network the address is associated with is extracted if possible.
// When the address does not encode the network, such as in the case of a raw
// public key, the address will be associated with the passed defaultNet.
func DecodeAddress(addr string, defaultNet btcwire.BitcoinNet) (Address, error) {
	// Serialized public keys are either 65 bytes (130 hex chars) if
	// uncompressed/hybrid or 33 bytes (66 hex chars) if compressed.
	if len(addr) == 130 || len(addr) == 66 {
		serializedPubKey, err := hex.DecodeString(addr)
		if err != nil {
			return nil, err
		}
		return NewAddressPubKey(serializedPubKey, defaultNet)
	}

	// Switch on decoded length to determine the type.
	decoded := Base58Decode(addr)
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
