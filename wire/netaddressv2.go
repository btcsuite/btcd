package wire

import (
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/sha3"
)

const (
	// maxAddrV2Size is the maximum size an address may be in the addrv2
	// message.
	maxAddrV2Size = 512
)

var (
	// ErrInvalidAddressSize is an error that means an incorrect address
	// size was decoded for a networkID or that the address exceeded the
	// maximum size for an unknown networkID.
	ErrInvalidAddressSize = fmt.Errorf("invalid address size")

	// ErrSkippedNetworkID is returned when the cjdns, i2p, or unknown
	// networks are encountered during decoding. btcd does not support i2p
	// or cjdns addresses. In the case of an unknown networkID, this is so
	// that a future BIP reserving a new networkID does not cause older
	// addrv2-supporting btcd software to disconnect upon receiving the new
	// addresses. This error can also be returned when an OnionCat-encoded
	// torv2 address is received with the ipv6 networkID. This error
	// signals to the caller to continue reading.
	ErrSkippedNetworkID = fmt.Errorf("skipped networkID")
)

// maxNetAddressV2Payload returns the max payload size for an address used in
// the addrv2 message.
func maxNetAddressV2Payload() uint32 {
	// The timestamp takes up four bytes.
	plen := uint32(4)

	// The ServiceFlag is a varint and its maximum size is 9 bytes.
	plen += 9

	// The netID is a single byte.
	plen += 1

	// The largest address is 512 bytes. Even though it will not be a valid
	// address, we should read and ignore it. The preceding varint to
	// store 512 bytes is 3 bytes long. This gives us a total of 515 bytes.
	plen += 515

	// The port is 2 bytes.
	plen += 2

	return plen
}

// isOnionCatTor returns whether a given ip address is actually an encoded tor
// v2 address. The wire package is unable to use the addrmgr's IsOnionCatTor as
// doing so would give an import cycle.
func isOnionCatTor(ip net.IP) bool {
	onionCatNet := net.IPNet{
		IP:   net.ParseIP("fd87:d87e:eb43::"),
		Mask: net.CIDRMask(48, 128),
	}
	return onionCatNet.Contains(ip)
}

// NetAddressV2 defines information about a peer on the network including the
// last time it was seen, the services it supports, its address, and port. This
// struct is used in the addrv2 message (MsgAddrV2) and can contain larger
// addresses, like Tor. Additionally, it can contain any NetAddress address.
type NetAddressV2 struct {
	// Last time the address was seen. This is, unfortunately, encoded as a
	// uint32 on the wire and therefore is limited to 2106. This field is
	// not present in the bitcoin version message (MsgVersion) nor was it
	// added until protocol version >= NetAddressTimeVersion.
	Timestamp time.Time

	// Services is a bitfield which identifies the services supported by
	// the address. This is encoded in CompactSize.
	Services ServiceFlag

	// Addr is the network address of the peer. This is a variable-length
	// address. Network() returns the BIP-155 networkID which is a uint8
	// encoded as a string. String() returns the address as a string.
	Addr net.Addr

	// Port is the port of the address. This is 0 if the network doesn't
	// use ports.
	Port uint16
}

// HasService returns whether the specified service is supported by the
// address.
func (na *NetAddressV2) HasService(service ServiceFlag) bool {
	return na.Services&service == service
}

// AddService adds a service to the Services bitfield.
func (na *NetAddressV2) AddService(service ServiceFlag) {
	na.Services |= service
}

// ToLegacy attempts to convert a NetAddressV2 to a legacy NetAddress. This
// only works for ipv4, ipv6, or torv2 addresses as they can be encoded with
// the OnionCat encoding. If this method is called on a torv3 address, nil will
// be returned.
func (na *NetAddressV2) ToLegacy() *NetAddress {
	legacyNa := &NetAddress{
		Timestamp: na.Timestamp,
		Services:  na.Services,
		Port:      na.Port,
	}

	switch a := na.Addr.(type) {
	case *ipv4Addr:
		legacyNa.IP = a.addr[:]
	case *ipv6Addr:
		legacyNa.IP = a.addr[:]
	case *torv2Addr:
		legacyNa.IP = a.onionCatEncoding()
	case *torv3Addr:
		return nil
	}

	return legacyNa
}

// IsTorV3 returns a bool that signals to the caller whether or not this is a
// torv3 address.
func (na *NetAddressV2) IsTorV3() bool {
	_, ok := na.Addr.(*torv3Addr)
	return ok
}

// TorV3Key returns the first byte of the v3 public key. This is used in the
// addrmgr to calculate a key from a network group.
func (na *NetAddressV2) TorV3Key() byte {
	// This should never be called on a non-torv3 address.
	addr, ok := na.Addr.(*torv3Addr)
	if !ok {
		panic("unexpected TorV3Key call on non-torv3 address")
	}

	return addr.addr[0]
}

// NetAddressV2FromBytes creates a NetAddressV2 from a byte slice. It will
// also handle a torv2 address using the OnionCat encoding.
func NetAddressV2FromBytes(timestamp time.Time, services ServiceFlag,
	addrBytes []byte, port uint16) *NetAddressV2 {

	var netAddr net.Addr
	switch len(addrBytes) {
	case ipv4Size:
		addr := &ipv4Addr{}
		addr.netID = ipv4
		copy(addr.addr[:], addrBytes)
		netAddr = addr
	case ipv6Size:
		if isOnionCatTor(addrBytes) {
			addr := &torv2Addr{}
			addr.netID = torv2
			copy(addr.addr[:], addrBytes[6:])
			netAddr = addr
			break
		}

		addr := &ipv6Addr{}
		addr.netID = ipv6
		copy(addr.addr[:], addrBytes)
		netAddr = addr
	case torv2Size:
		addr := &torv2Addr{}
		addr.netID = torv2
		copy(addr.addr[:], addrBytes)
		netAddr = addr
	case TorV3Size:
		addr := &torv3Addr{}
		addr.netID = torv3
		copy(addr.addr[:], addrBytes)
		netAddr = addr
	}

	return &NetAddressV2{
		Timestamp: timestamp,
		Services:  services,
		Addr:      netAddr,
		Port:      port,
	}
}

// writeNetAddressV2 writes a NetAddressV2 to a writer.
func writeNetAddressV2(w io.Writer, pver uint32, na *NetAddressV2) error {
	err := writeElement(w, uint32(na.Timestamp.Unix()))
	if err != nil {
		return err
	}

	if err := WriteVarInt(w, pver, uint64(na.Services)); err != nil {
		return err
	}

	var (
		netID   networkID
		address []byte
	)

	switch a := na.Addr.(type) {
	case *ipv4Addr:
		netID = a.netID
		address = a.addr[:]
	case *ipv6Addr:
		netID = a.netID
		address = a.addr[:]
	case *torv2Addr:
		netID = a.netID
		address = a.addr[:]
	case *torv3Addr:
		netID = a.netID
		address = a.addr[:]
	default:
		// This should not occur.
		return fmt.Errorf("unexpected address type")
	}

	if err := writeElement(w, netID); err != nil {
		return err
	}

	addressSize := uint64(len(address))
	if err := WriteVarInt(w, pver, addressSize); err != nil {
		return err
	}

	if err := writeElement(w, address); err != nil {
		return err
	}

	return binary.Write(w, bigEndian, na.Port)
}

// readNetAddressV2 reads a NetAddressV2 from a reader. This function has
// checks that the corresponding write function doesn't. This is because
// reading from the peer is untrusted whereas writing assumes we have already
// validated the NetAddressV2.
func readNetAddressV2(r io.Reader, pver uint32, na *NetAddressV2) error {
	err := readElement(r, (*uint32Time)(&na.Timestamp))
	if err != nil {
		return err
	}

	// Services is encoded as a variable length integer in addrv2.
	services, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	na.Services = ServiceFlag(services)

	var netID uint8
	if err := readElement(r, &netID); err != nil {
		return err
	}

	decodedSize, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	if !isKnownNetworkID(netID) {
		// In the case of an unknown networkID, we'll read the address
		// size and error with ErrInvalidAddressSize if it's greater
		// than maxAddrV2Size. If the address size is within the valid
		// range, we'll just read and discard the address. In this
		// case, ErrSkippedNetworkID will be returned to signal to the
		// caller to continue reading.
		if decodedSize > maxAddrV2Size {
			return ErrInvalidAddressSize
		}

		// The +2 is the port field.
		discardedAddrPort := make([]byte, decodedSize+2)
		if err := readElement(r, &discardedAddrPort); err != nil {
			return err
		}

		return ErrSkippedNetworkID
	}

	// If the netID is an i2p or cjdns address, we'll advance the reader
	// and return a special error to signal to the caller to not use the
	// passed NetAddressV2 struct. Otherwise, we'll just read the address
	// and port without returning an error.
	switch networkID(netID) {
	case ipv4:
		addr := &ipv4Addr{}
		addr.netID = ipv4
		if decodedSize != uint64(ipv4Size) {
			return ErrInvalidAddressSize
		}

		if err := readElement(r, &addr.addr); err != nil {
			return err
		}

		na.Port, err = binarySerializer.Uint16(r, bigEndian)
		if err != nil {
			return err
		}

		na.Addr = addr
	case ipv6:
		addr := &ipv6Addr{}
		addr.netID = ipv6
		if decodedSize != uint64(ipv6Size) {
			return ErrInvalidAddressSize
		}

		if err := readElement(r, &addr.addr); err != nil {
			return err
		}

		na.Port, err = binarySerializer.Uint16(r, bigEndian)
		if err != nil {
			return err
		}

		na.Addr = addr

		// BIP-155 says to ignore OnionCat addresses in addrv2
		// messages.
		if isOnionCatTor(addr.addr[:]) {
			return ErrSkippedNetworkID
		}
	case torv2:
		addr := &torv2Addr{}
		addr.netID = torv2
		if decodedSize != uint64(torv2Size) {
			return ErrInvalidAddressSize
		}

		if err := readElement(r, &addr.addr); err != nil {
			return err
		}

		na.Port, err = binarySerializer.Uint16(r, bigEndian)
		if err != nil {
			return err
		}

		na.Addr = addr
	case torv3:
		addr := &torv3Addr{}
		addr.netID = torv3
		if decodedSize != uint64(TorV3Size) {
			return ErrInvalidAddressSize
		}

		if err := readElement(r, &addr.addr); err != nil {
			return err
		}

		na.Port, err = binarySerializer.Uint16(r, bigEndian)
		if err != nil {
			return err
		}

		// BIP-155 does not specify to validate the public key here.
		// bitcoind does not validate the ed25519 pubkey.
		na.Addr = addr
	case i2p:
		addr := &i2pAddr{}
		addr.netID = i2p
		if decodedSize != uint64(i2pSize) {
			return ErrInvalidAddressSize
		}

		if err := readElement(r, &addr.addr); err != nil {
			return err
		}

		na.Port, err = binarySerializer.Uint16(r, bigEndian)
		if err != nil {
			return err
		}

		return ErrSkippedNetworkID
	case cjdns:
		addr := &cjdnsAddr{}
		addr.netID = cjdns
		if decodedSize != uint64(cjdnsSize) {
			return ErrInvalidAddressSize
		}

		if err := readElement(r, &addr.addr); err != nil {
			return err
		}

		na.Port, err = binarySerializer.Uint16(r, bigEndian)
		if err != nil {
			return err
		}

		return ErrSkippedNetworkID
	}

	return nil
}

// networkID represents the network that a given address is in. CJDNS and I2P
// addresses are not included.
type networkID uint8

const (
	// ipv4 means the following address is ipv4.
	ipv4 networkID = iota + 1

	// ipv6 means the following address is ipv6.
	ipv6

	// torv2 means the following address is a torv2 hidden service address.
	torv2

	// torv3 means the following address is a torv3 hidden service address.
	torv3

	// i2p means the following address is an i2p address.
	i2p

	// cjdns means the following address is a cjdns address.
	cjdns
)

const (
	// ipv4Size is the size of an ipv4 address.
	ipv4Size = 4

	// ipv6Size is the size of an ipv6 address.
	ipv6Size = 16

	// torv2Size is the size of a torv2 address.
	torv2Size = 10

	// TorV3Size is the size of a torv3 address in bytes.
	TorV3Size = 32

	// i2pSize is the size of an i2p address.
	i2pSize = 32

	// cjdnsSize is the size of a cjdns address.
	cjdnsSize = 16
)

const (
	// TorV2EncodedSize is the size of a torv2 address encoded in base32
	// with the ".onion" suffix.
	TorV2EncodedSize = 22

	// TorV3EncodedSize is the size of a torv3 address encoded in base32
	// with the ".onion" suffix.
	TorV3EncodedSize = 62
)

// isKnownNetworkID returns true if the networkID is one listed above and false
// otherwise.
func isKnownNetworkID(netID uint8) bool {
	return uint8(ipv4) <= netID && netID <= uint8(cjdns)
}

type ipv4Addr struct {
	addr  [ipv4Size]byte
	netID networkID
}

// Part of the net.Addr interface.
func (a *ipv4Addr) String() string {
	return net.IP(a.addr[:]).String()
}

// Part of the net.Addr interface.
func (a *ipv4Addr) Network() string {
	return string(a.netID)
}

// Compile-time constraints to check that ipv4Addr meets the net.Addr
// interface.
var _ net.Addr = (*ipv4Addr)(nil)

type ipv6Addr struct {
	addr  [ipv6Size]byte
	netID networkID
}

// Part of the net.Addr interface.
func (a *ipv6Addr) String() string {
	return net.IP(a.addr[:]).String()
}

// Part of the net.Addr interface.
func (a *ipv6Addr) Network() string {
	return string(a.netID)
}

// Compile-time constraints to check that ipv6Addr meets the net.Addr
// interface.
var _ net.Addr = (*ipv4Addr)(nil)

type torv2Addr struct {
	addr  [torv2Size]byte
	netID networkID
}

// Part of the net.Addr interface.
func (a *torv2Addr) String() string {
	base32Hash := base32.StdEncoding.EncodeToString(a.addr[:])
	return strings.ToLower(base32Hash) + ".onion"
}

// Part of the net.Addr interface.
func (a *torv2Addr) Network() string {
	return string(a.netID)
}

// onionCatEncoding returns a torv2 address as an ipv6 address.
func (a *torv2Addr) onionCatEncoding() net.IP {
	prefix := []byte{0xfd, 0x87, 0xd8, 0x7e, 0xeb, 0x43}
	return net.IP(append(prefix, a.addr[:]...))
}

// Compile-time constraints to check that torv2Addr meets the net.Addr
// interface.
var _ net.Addr = (*torv2Addr)(nil)

type torv3Addr struct {
	addr  [TorV3Size]byte
	netID networkID
}

// Part of the net.Addr interface.
func (a *torv3Addr) String() string {
	// BIP-155 describes the torv3 address format:
	// onion_address = base32(PUBKEY | CHECKSUM | VERSION) + ".onion"
	// CHECKSUM = H(".onion checksum" | PUBKEY | VERSION)[:2]
	// PUBKEY = addr, which is the ed25519 pubkey of the hidden service.
	// VERSION = '\x03'
	// H() is the SHA3-256 cryptographic hash function.

	torV3Version := []byte("\x03")
	checksumConst := []byte(".onion checksum")

	// Write never returns an error so there is no need to handle it.
	h := sha3.New256()
	h.Write(checksumConst)
	h.Write(a.addr[:])
	h.Write(torV3Version)
	truncatedChecksum := h.Sum(nil)[:2]

	var base32Input [35]byte
	copy(base32Input[:32], a.addr[:])
	copy(base32Input[32:34], truncatedChecksum)
	copy(base32Input[34:], torV3Version)

	base32Hash := base32.StdEncoding.EncodeToString(base32Input[:])
	return strings.ToLower(base32Hash) + ".onion"
}

// Part of the net.Addr interface.
func (a *torv3Addr) Network() string {
	return string(a.netID)
}

// Compile-time constraints to check that torv3Addr meets the net.Addr
// interface.
var _ net.Addr = (*torv3Addr)(nil)

type i2pAddr struct {
	addr  [i2pSize]byte
	netID networkID
}

type cjdnsAddr struct {
	addr  [cjdnsSize]byte
	netID networkID
}
