package btcutil

import (
	"bytes"
	"code.google.com/p/go.crypto/ripemd160"
	"errors"
	"github.com/conformal/btcwire"
)

// ErrAddrUnknownNet describes an error where the address identifier
// byte is not recognized as belonging to neither the Bitcoin MainNet nor
// TestNet.
var ErrAddrUnknownNet = errors.New("unrecognized network identifier byte")

// ErrMalformedAddress describes an error where an address is improperly
// formatted, either due to an incorrect length of the hashed pubkey or
// a non-matching checksum.
var ErrMalformedAddress  = errors.New("malformed address")

// Constants used to specify which network a payment address belongs
// to.  Mainnet address cannot be used on the Testnet, and vice versa.
const (
	// MainNetAddr is the address identifier for MainNet
	MainNetAddr = 0x00

	// TestNetAddr is the address identifier for TestNet
	TestNetAddr = 0x6f
)

// EncodeAddress takes a 20-byte raw payment address (hash160 of the
// uncompressed pubkey) and a network identifying byte and encodes the
// payment address in a human readable string.
func EncodeAddress(addrHash []byte, netID byte) (encoded string, err error) {
	if len(addrHash) != ripemd160.Size {
		return "", ErrMalformedAddress
	}
	if netID != MainNetAddr && netID != TestNetAddr {
		return "", ErrAddrUnknownNet
	}

	tosum := append([]byte{netID}, addrHash...)
	cksum := btcwire.DoubleSha256(tosum)

	a := append([]byte{netID}, addrHash...)
	a = append(a, cksum[:4]...)

	return Base58Encode(a), nil
}

// DecodeAddress decodes a human readable payment address string
// returning the 20-byte decoded address, along with the network
// identifying byte.
func DecodeAddress(addr string) (addrHash []byte, netID byte, err error) {
	decoded := Base58Decode(addr)

	// Length of decoded address must be 20 bytes + 1 byte for a network
	// identifier byte + 4 bytes of checksum.
	if len(decoded) != ripemd160.Size+5 {
		return nil, 0x00, ErrMalformedAddress
	}

	netID = decoded[0]
	if netID != MainNetAddr && netID != TestNetAddr {
		return nil, 0x00, ErrAddrUnknownNet
	}
	addrHash = decoded[1:21]

	// Checksum is first four bytes of double SHA256 of the network byte
	// and addrHash.  Verify this matches the final 4 bytes of the decoded
	// address.
	tosum := append([]byte{netID}, addrHash...)
	cksum := btcwire.DoubleSha256(tosum)[:4]
	if !bytes.Equal(cksum, decoded[len(decoded)-4:]) {
		return nil, 0x00, ErrMalformedAddress
	}

	return addrHash, netID, nil
}
