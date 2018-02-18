// Copyright (c) 2013, 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/ripemd160"

	"github.com/decred/base58"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainec"
)

var (
	// ErrChecksumMismatch describes an error where decoding failed due
	// to a bad checksum.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrUnknownAddressType describes an error where an address can not
	// decoded as a specific address type due to the string encoding
	// beginning with an identifier byte unknown to any standard or
	// registered (via chaincfg.Register) network.
	ErrUnknownAddressType = errors.New("unknown address type")

	// ErrAddressCollision describes an error where an address can not
	// be uniquely determined as either a pay-to-pubkey-hash or
	// pay-to-script-hash address since the leading identifier is used for
	// describing both address kinds, but for different networks.  Rather
	// than assuming or defaulting to one or the other, this error is
	// returned and the caller must decide how to decode the address.
	ErrAddressCollision = errors.New("address collision")

	// ErrMissingDefaultNet describes an error in DecodeAddress that
	// attempts to decode an address without defining which network to decode
	// for.
	ErrMissingDefaultNet = errors.New("default net not defined")
)

// encodeAddress returns a human-readable payment address given a ripemd160 hash
// and netID which encodes the network and address type.  It is used in both
// pay-to-pubkey-hash (P2PKH) and pay-to-script-hash (P2SH) address encoding.
func encodeAddress(hash160 []byte, netID [2]byte) string {
	// Format is 2 bytes for a network and address class (i.e. P2PKH vs
	// P2SH), 20 bytes for a RIPEMD160 hash, and 4 bytes of checksum.
	return base58.CheckEncode(hash160[:ripemd160.Size], netID)
}

// encodePKAddress returns a human-readable payment address to a public key
// given a serialized public key, a netID, and a signature suite.
func encodePKAddress(serializedPK []byte, netID [2]byte, algo int) string {
	pubKeyBytes := []byte{0x00}

	switch algo {
	case chainec.ECTypeSecp256k1:
		pubKeyBytes[0] = byte(chainec.ECTypeSecp256k1)
	case chainec.ECTypeEdwards:
		pubKeyBytes[0] = byte(chainec.ECTypeEdwards)
	case chainec.ECTypeSecSchnorr:
		pubKeyBytes[0] = byte(chainec.ECTypeSecSchnorr)
	}

	// Pubkeys are encoded as [0] = type/ybit, [1:33] = serialized pubkey
	compressed := serializedPK
	if algo == chainec.ECTypeSecp256k1 || algo == chainec.ECTypeSecSchnorr {
		pub, err := chainec.Secp256k1.ParsePubKey(serializedPK)
		if err != nil {
			return ""
		}
		pubSerComp := pub.SerializeCompressed()

		// Set the y-bit if needed.
		if pubSerComp[0] == 0x03 {
			pubKeyBytes[0] |= (1 << 7)
		}

		compressed = pubSerComp[1:]
	}

	pubKeyBytes = append(pubKeyBytes, compressed...)
	return base58.CheckEncode(pubKeyBytes, netID)
}

// Address is an interface type for any type of destination a transaction
// output may spend to.  This includes pay-to-pubkey (P2PK), pay-to-pubkey-hash
// (P2PKH), and pay-to-script-hash (P2SH).  Address is designed to be generic
// enough that other kinds of addresses may be added in the future without
// changing the decoding and encoding API.
type Address interface {
	// String returns the string encoding of the transaction output
	// destination.
	//
	// Please note that String differs subtly from EncodeAddress: String
	// will return the value as a string without any conversion, while
	// EncodeAddress may convert destination types (for example,
	// converting pubkeys to P2PKH addresses) before encoding as a
	// payment address string.
	String() string

	// EncodeAddress returns the string encoding of the payment address
	// associated with the Address value.  See the comment on String
	// for how this method differs from String.
	EncodeAddress() string

	// ScriptAddress returns the raw bytes of the address to be used
	// when inserting the address into a txout's script.
	ScriptAddress() []byte

	// Hash160 returns the Hash160(data) where data is the data normally
	// hashed to 160 bits from the respective address type.
	Hash160() *[ripemd160.Size]byte

	// IsForNet returns whether or not the address is associated with the
	// passed network.
	IsForNet(*chaincfg.Params) bool

	// DSA returns the digital signature algorithm for the address.
	DSA(*chaincfg.Params) int

	// Net returns the network parameters of the address.
	Net() *chaincfg.Params
}

// NewAddressPubKey returns a new Address. decoded must
// be 33 bytes.
func NewAddressPubKey(decoded []byte, net *chaincfg.Params) (Address, error) {
	if len(decoded) == 33 {
		// First byte is the signature suite and ybit.
		suite := decoded[0]
		suite &= ^uint8(1 << 7)
		ybit := !(decoded[0]&(1<<7) == 0)
		toAppend := uint8(0x02)
		if ybit {
			toAppend = 0x03
		}

		switch int(suite) {
		case chainec.ECTypeSecp256k1:
			return NewAddressSecpPubKey(
				append([]byte{toAppend}, decoded[1:]...),
				net)
		case chainec.ECTypeEdwards:
			return NewAddressEdwardsPubKey(decoded, net)
		case chainec.ECTypeSecSchnorr:
			return NewAddressSecSchnorrPubKey(
				append([]byte{toAppend}, decoded[1:]...),
				net)
		}
		return nil, ErrUnknownAddressType
	}
	return nil, ErrUnknownAddressType
}

// DecodeAddress decodes the string encoding of an address and returns
// the Address if addr is a valid encoding for a known address type
func DecodeAddress(addr string) (Address, error) {
	// Switch on decoded length to determine the type.
	decoded, netID, err := base58.CheckDecode(addr)
	if err != nil {
		if err == base58.ErrChecksum {
			return nil, ErrChecksumMismatch
		}
		return nil, fmt.Errorf("decoded address is of unknown format: %v",
			err.Error())
	}

	net, err := detectNetworkForAddress(addr)
	if err != nil {
		return nil, ErrUnknownAddressType
	}

	switch netID {
	case net.PubKeyAddrID:
		return NewAddressPubKey(decoded, net)

	case net.PubKeyHashAddrID:
		return NewAddressPubKeyHash(decoded, net, chainec.ECTypeSecp256k1)

	case net.PKHEdwardsAddrID:
		return NewAddressPubKeyHash(decoded, net, chainec.ECTypeEdwards)

	case net.PKHSchnorrAddrID:
		return NewAddressPubKeyHash(decoded, net, chainec.ECTypeSecSchnorr)

	case net.ScriptHashAddrID:
		return NewAddressScriptHashFromHash(decoded, net)

	default:
		return nil, ErrUnknownAddressType
	}
}

// detectNetworkForAddress pops the first character from a string encoded
// address and detects what network type it is for.
func detectNetworkForAddress(addr string) (*chaincfg.Params, error) {
	if len(addr) < 1 {
		return nil, fmt.Errorf("empty string given for network detection")
	}

	networkChar := addr[0:1]
	switch networkChar {
	case chaincfg.MainNetParams.NetworkAddressPrefix:
		return &chaincfg.MainNetParams, nil
	case chaincfg.TestNet2Params.NetworkAddressPrefix:
		return &chaincfg.TestNet2Params, nil
	case chaincfg.SimNetParams.NetworkAddressPrefix:
		return &chaincfg.SimNetParams, nil
	}

	return nil, fmt.Errorf("unknown network type in string encoded address")
}

// AddressPubKeyHash is an Address for a pay-to-pubkey-hash (P2PKH)
// transaction.
type AddressPubKeyHash struct {
	net   *chaincfg.Params
	hash  [ripemd160.Size]byte
	netID [2]byte
}

// NewAddressPubKeyHash returns a new AddressPubKeyHash.  pkHash must
// be 20 bytes.
func NewAddressPubKeyHash(pkHash []byte, net *chaincfg.Params,
	algo int) (*AddressPubKeyHash, error) {
	var addrID [2]byte
	switch algo {
	case chainec.ECTypeSecp256k1:
		addrID = net.PubKeyHashAddrID
	case chainec.ECTypeEdwards:
		addrID = net.PKHEdwardsAddrID
	case chainec.ECTypeSecSchnorr:
		addrID = net.PKHSchnorrAddrID
	default:
		return nil, errors.New("unknown ECDSA algorithm")
	}
	apkh, err := newAddressPubKeyHash(pkHash, addrID)
	if err != nil {
		return nil, err
	}
	apkh.net = net
	return apkh, nil
}

// newAddressPubKeyHash is the internal API to create a pubkey hash address
// with a known leading identifier byte for a network, rather than looking
// it up through its parameters.  This is useful when creating a new address
// structure from a string encoding where the identifer byte is already
// known.
func newAddressPubKeyHash(pkHash []byte, netID [2]byte) (*AddressPubKeyHash,
	error) {
	// Check for a valid pubkey hash length.
	if len(pkHash) != ripemd160.Size {
		return nil, errors.New("pkHash must be 20 bytes")
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
// with the passed network.
func (a *AddressPubKeyHash) IsForNet(net *chaincfg.Params) bool {
	return a.netID == net.PubKeyHashAddrID ||
		a.netID == net.PKHEdwardsAddrID ||
		a.netID == net.PKHSchnorrAddrID
}

// String returns a human-readable string for the pay-to-pubkey-hash address.
// This is equivalent to calling EncodeAddress, but is provided so the type can
// be used as a fmt.Stringer.
func (a *AddressPubKeyHash) String() string {
	return a.EncodeAddress()
}

// Hash160 returns the underlying array of the pubkey hash.  This can be useful
// when an array is more appropriate than a slice (for example, when used as map
// keys).
func (a *AddressPubKeyHash) Hash160() *[ripemd160.Size]byte {
	return &a.hash
}

// DSA returns the digital signature algorithm for the associated public key
// hash.
func (a *AddressPubKeyHash) DSA(net *chaincfg.Params) int {
	switch a.netID {
	case net.PubKeyHashAddrID:
		return chainec.ECTypeSecp256k1
	case net.PKHEdwardsAddrID:
		return chainec.ECTypeEdwards
	case net.PKHSchnorrAddrID:
		return chainec.ECTypeSecSchnorr
	}
	return -1
}

// Net returns the network for the address.
func (a *AddressPubKeyHash) Net() *chaincfg.Params {
	return a.net
}

// AddressScriptHash is an Address for a pay-to-script-hash (P2SH)
// transaction.
type AddressScriptHash struct {
	net   *chaincfg.Params
	hash  [ripemd160.Size]byte
	netID [2]byte
}

// NewAddressScriptHash returns a new AddressScriptHash.
func NewAddressScriptHash(serializedScript []byte,
	net *chaincfg.Params) (*AddressScriptHash, error) {
	scriptHash := Hash160(serializedScript)
	ash, err := newAddressScriptHashFromHash(scriptHash, net.ScriptHashAddrID)
	if err != nil {
		return nil, err
	}
	ash.net = net

	return ash, nil
}

// NewAddressScriptHashFromHash returns a new AddressScriptHash.  scriptHash
// must be 20 bytes.
func NewAddressScriptHashFromHash(scriptHash []byte,
	net *chaincfg.Params) (*AddressScriptHash, error) {
	ash, err := newAddressScriptHashFromHash(scriptHash, net.ScriptHashAddrID)
	if err != nil {
		return nil, err
	}
	ash.net = net

	return ash, nil
}

// newAddressScriptHashFromHash is the internal API to create a script hash
// address with a known leading identifier byte for a network, rather than
// looking it up through its parameters.  This is useful when creating a new
// address structure from a string encoding where the identifer byte is already
// known.
func newAddressScriptHashFromHash(scriptHash []byte,
	netID [2]byte) (*AddressScriptHash, error) {
	// Check for a valid script hash length.
	if len(scriptHash) != ripemd160.Size {
		return nil, errors.New("scriptHash must be 20 bytes")
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
// with the passed network.
func (a *AddressScriptHash) IsForNet(net *chaincfg.Params) bool {
	return a.netID == net.ScriptHashAddrID
}

// String returns a human-readable string for the pay-to-script-hash address.
// This is equivalent to calling EncodeAddress, but is provided so the type can
// be used as a fmt.Stringer.
func (a *AddressScriptHash) String() string {
	return a.EncodeAddress()
}

// Hash160 returns the underlying array of the script hash.  This can be useful
// when an array is more appropriate than a slice (for example, when used as map
// keys).
func (a *AddressScriptHash) Hash160() *[ripemd160.Size]byte {
	return &a.hash
}

// DSA returns -1 (invalid) as the digital signature algorithm for scripts,
// as scripts may not involve digital signatures at all.
func (a *AddressScriptHash) DSA(net *chaincfg.Params) int {
	return -1
}

// Net returns the network for the address.
func (a *AddressScriptHash) Net() *chaincfg.Params {
	return a.net
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

// AddressSecpPubKey is an Address for a secp256k1 pay-to-pubkey transaction.
type AddressSecpPubKey struct {
	net          *chaincfg.Params
	pubKeyFormat PubKeyFormat
	pubKey       chainec.PublicKey
	pubKeyHashID [2]byte
}

// NewAddressSecpPubKey returns a new AddressSecpPubKey which represents a
// pay-to-pubkey address, using a secp256k1 pubkey.  The serializedPubKey
// parameter must be a valid pubkey and can be uncompressed, compressed, or
// hybrid.
func NewAddressSecpPubKey(serializedPubKey []byte,
	net *chaincfg.Params) (*AddressSecpPubKey, error) {
	pubKey, err := chainec.Secp256k1.ParsePubKey(serializedPubKey)
	if err != nil {
		return nil, err
	}

	// Set the format of the pubkey.  This probably should be returned
	// from dcrec, but do it here to avoid API churn.  We already know the
	// pubkey is valid since it parsed above, so it's safe to simply examine
	// the leading byte to get the format.
	pkFormat := PKFUncompressed
	switch serializedPubKey[0] {
	case 0x02, 0x03:
		pkFormat = PKFCompressed
	case 0x06, 0x07:
		pkFormat = PKFHybrid
	}

	return &AddressSecpPubKey{
		net:          net,
		pubKeyFormat: pkFormat,
		pubKey:       pubKey,
		pubKeyHashID: net.PubKeyHashAddrID,
	}, nil
}

// serialize returns the serialization of the public key according to the
// format associated with the address.
func (a *AddressSecpPubKey) serialize() []byte {
	switch a.pubKeyFormat {
	default:
		fallthrough
	case PKFUncompressed:
		return a.pubKey.SerializeUncompressed()

	case PKFCompressed:
		return a.pubKey.SerializeCompressed()

	case PKFHybrid:
		return a.pubKey.SerializeHybrid()
	}
}

// EncodeAddress returns the string encoding of the public key as a
// pay-to-pubkey-hash.  Note that the public key format (uncompressed,
// compressed, etc) will change the resulting address.  This is expected since
// pay-to-pubkey-hash is a hash of the serialized public key which obviously
// differs with the format.  At the time of this writing, most Decred addresses
// are pay-to-pubkey-hash constructed from the uncompressed public key.
//
// Part of the Address interface.
func (a *AddressSecpPubKey) EncodeAddress() string {
	return encodeAddress(Hash160(a.serialize()), a.pubKeyHashID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a public key.  Setting the public key format will affect the output of
// this function accordingly.  Part of the Address interface.
func (a *AddressSecpPubKey) ScriptAddress() []byte {
	return a.serialize()
}

// Hash160 returns the underlying array of the pubkey hash.  This can be useful
// when an array is more appropriate than a slice (for example, when used as map
// keys).
func (a *AddressSecpPubKey) Hash160() *[ripemd160.Size]byte {
	h160 := Hash160(a.pubKey.SerializeCompressed())
	array := new([ripemd160.Size]byte)
	copy(array[:], h160)

	return array
}

// IsForNet returns whether or not the pay-to-pubkey address is associated
// with the passed network.
func (a *AddressSecpPubKey) IsForNet(net *chaincfg.Params) bool {
	return a.pubKeyHashID == net.PubKeyHashAddrID
}

// String returns the hex-encoded human-readable string for the pay-to-pubkey
// address.  This is not the same as calling EncodeAddress.
func (a *AddressSecpPubKey) String() string {
	return encodePKAddress(a.serialize(), a.net.PubKeyAddrID,
		chainec.ECTypeSecp256k1)
}

// Format returns the format (uncompressed, compressed, etc) of the
// pay-to-pubkey address.
func (a *AddressSecpPubKey) Format() PubKeyFormat {
	return a.pubKeyFormat
}

// AddressPubKeyHash returns the pay-to-pubkey address converted to a
// pay-to-pubkey-hash address.  Note that the public key format (uncompressed,
// compressed, etc) will change the resulting address.  This is expected since
// pay-to-pubkey-hash is a hash of the serialized public key which obviously
// differs with the format.  At the time of this writing, most Decred addresses
// are pay-to-pubkey-hash constructed from the uncompressed public key.
func (a *AddressSecpPubKey) AddressPubKeyHash() *AddressPubKeyHash {
	addr := &AddressPubKeyHash{net: a.net, netID: a.pubKeyHashID}
	copy(addr.hash[:], Hash160(a.serialize()))
	return addr
}

// PubKey returns the underlying public key for the address.
func (a *AddressSecpPubKey) PubKey() chainec.PublicKey {
	return a.pubKey
}

// DSA returns the underlying digital signature algorithm for the
// address.
func (a *AddressSecpPubKey) DSA(net *chaincfg.Params) int {
	switch a.pubKeyHashID {
	case net.PubKeyHashAddrID:
		return chainec.ECTypeSecp256k1
	case net.PKHSchnorrAddrID:
		return chainec.ECTypeSecSchnorr
	}
	return -1
}

// Net returns the network for the address.
func (a *AddressSecpPubKey) Net() *chaincfg.Params {
	return a.net
}

// NewAddressSecpPubKeyCompressed creates a new address using a compressed public key
func NewAddressSecpPubKeyCompressed(pubkey chainec.PublicKey, params *chaincfg.Params) (*AddressSecpPubKey, error) {
	return NewAddressSecpPubKey(pubkey.SerializeCompressed(), params)
}

// AddressEdwardsPubKey is an Address for an Ed25519 pay-to-pubkey transaction.
type AddressEdwardsPubKey struct {
	net          *chaincfg.Params
	pubKey       chainec.PublicKey
	pubKeyHashID [2]byte
}

// NewAddressEdwardsPubKey returns a new AddressEdwardsPubKey which represents a
// pay-to-pubkey address, using an Ed25519 pubkey.  The serializedPubKey
// parameter must be a valid 32 byte serialized public key.
func NewAddressEdwardsPubKey(serializedPubKey []byte,
	net *chaincfg.Params) (*AddressEdwardsPubKey, error) {
	pubKey, err := chainec.Edwards.ParsePubKey(serializedPubKey)
	if err != nil {
		return nil, err
	}

	return &AddressEdwardsPubKey{
		net:          net,
		pubKey:       pubKey,
		pubKeyHashID: net.PKHEdwardsAddrID,
	}, nil
}

// serialize returns the serialization of the public key.
func (a *AddressEdwardsPubKey) serialize() []byte {
	return a.pubKey.Serialize()
}

// EncodeAddress returns the string encoding of the public key as a
// pay-to-pubkey-hash.
//
// Part of the Address interface.
func (a *AddressEdwardsPubKey) EncodeAddress() string {
	return encodeAddress(Hash160(a.serialize()), a.pubKeyHashID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a public key.  Setting the public key format will affect the output of
// this function accordingly.  Part of the Address interface.
func (a *AddressEdwardsPubKey) ScriptAddress() []byte {
	return a.serialize()
}

// Hash160 returns the underlying array of the pubkey hash.  This can be useful
// when an array is more appropriate than a slice (for example, when used as map
// keys).
func (a *AddressEdwardsPubKey) Hash160() *[ripemd160.Size]byte {
	h160 := Hash160(a.pubKey.Serialize())
	array := new([ripemd160.Size]byte)
	copy(array[:], h160)

	return array
}

// IsForNet returns whether or not the pay-to-pubkey address is associated
// with the passed network.
func (a *AddressEdwardsPubKey) IsForNet(net *chaincfg.Params) bool {
	return a.pubKeyHashID == net.PKHEdwardsAddrID
}

// String returns the hex-encoded human-readable string for the pay-to-pubkey
// address.  This is not the same as calling EncodeAddress.
func (a *AddressEdwardsPubKey) String() string {
	return encodePKAddress(a.serialize(), a.net.PubKeyAddrID,
		chainec.ECTypeEdwards)
}

// AddressPubKeyHash returns the pay-to-pubkey address converted to a
// pay-to-pubkey-hash address.
func (a *AddressEdwardsPubKey) AddressPubKeyHash() *AddressPubKeyHash {
	addr := &AddressPubKeyHash{net: a.net, netID: a.pubKeyHashID}
	copy(addr.hash[:], Hash160(a.serialize()))
	return addr
}

// PubKey returns the underlying public key for the address.
func (a *AddressEdwardsPubKey) PubKey() chainec.PublicKey {
	return a.pubKey
}

// DSA returns the underlying digital signature algorithm for the
// address.
func (a *AddressEdwardsPubKey) DSA(net *chaincfg.Params) int {
	return chainec.ECTypeEdwards
}

// Net returns the network for the address.
func (a *AddressEdwardsPubKey) Net() *chaincfg.Params {
	return a.net
}

// AddressSecSchnorrPubKey is an Address for a secp256k1 pay-to-pubkey
// transaction.
type AddressSecSchnorrPubKey struct {
	net          *chaincfg.Params
	pubKey       chainec.PublicKey
	pubKeyHashID [2]byte
}

// NewAddressSecSchnorrPubKey returns a new AddressSecpPubKey which represents a
// pay-to-pubkey address, using a secp256k1 pubkey.  The serializedPubKey
// parameter must be a valid pubkey and can be uncompressed, compressed, or
// hybrid.
func NewAddressSecSchnorrPubKey(serializedPubKey []byte,
	net *chaincfg.Params) (*AddressSecSchnorrPubKey, error) {
	pubKey, err := chainec.SecSchnorr.ParsePubKey(serializedPubKey)
	if err != nil {
		return nil, err
	}

	return &AddressSecSchnorrPubKey{
		net:          net,
		pubKey:       pubKey,
		pubKeyHashID: net.PKHSchnorrAddrID,
	}, nil
}

// serialize returns the serialization of the public key according to the
// format associated with the address.
func (a *AddressSecSchnorrPubKey) serialize() []byte {
	return a.pubKey.Serialize()
}

// EncodeAddress returns the string encoding of the public key as a
// pay-to-pubkey-hash.  Note that the public key format (uncompressed,
// compressed, etc) will change the resulting address.  This is expected since
// pay-to-pubkey-hash is a hash of the serialized public key which obviously
// differs with the format.  At the time of this writing, most Decred addresses
// are pay-to-pubkey-hash constructed from the uncompressed public key.
//
// Part of the Address interface.
func (a *AddressSecSchnorrPubKey) EncodeAddress() string {
	return encodeAddress(Hash160(a.serialize()), a.pubKeyHashID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a public key.  Setting the public key format will affect the output of
// this function accordingly.  Part of the Address interface.
func (a *AddressSecSchnorrPubKey) ScriptAddress() []byte {
	return a.serialize()
}

// Hash160 returns the underlying array of the pubkey hash.  This can be useful
// when an array is more appropriate than a slice (for example, when used as map
// keys).
func (a *AddressSecSchnorrPubKey) Hash160() *[ripemd160.Size]byte {
	h160 := Hash160(a.pubKey.Serialize())
	array := new([ripemd160.Size]byte)
	copy(array[:], h160)

	return array
}

// IsForNet returns whether or not the pay-to-pubkey address is associated
// with the passed network.
func (a *AddressSecSchnorrPubKey) IsForNet(net *chaincfg.Params) bool {
	return a.pubKeyHashID == net.PubKeyHashAddrID
}

// String returns the hex-encoded human-readable string for the pay-to-pubkey
// address.  This is not the same as calling EncodeAddress.
func (a *AddressSecSchnorrPubKey) String() string {
	return encodePKAddress(a.serialize(), a.net.PubKeyAddrID,
		chainec.ECTypeSecSchnorr)
}

// AddressPubKeyHash returns the pay-to-pubkey address converted to a
// pay-to-pubkey-hash address.
func (a *AddressSecSchnorrPubKey) AddressPubKeyHash() *AddressPubKeyHash {
	addr := &AddressPubKeyHash{net: a.net, netID: a.pubKeyHashID}
	copy(addr.hash[:], Hash160(a.serialize()))
	return addr
}

// DSA returns the underlying digital signature algorithm for the
// address.
func (a *AddressSecSchnorrPubKey) DSA(net *chaincfg.Params) int {
	return chainec.ECTypeSecSchnorr
}

// Net returns the network for the address.
func (a *AddressSecSchnorrPubKey) Net() *chaincfg.Params {
	return a.net
}
