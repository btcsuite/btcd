// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"
)

// ErrInvalidNetAddr describes an error that indicates the caller didn't specify
// a TCP address as required.
var ErrInvalidNetAddr = errors.New("provided net.Addr is not a net.TCPAddr")

// maxNetAddressPayload returns the max payload size for a bitcoin NetAddress
// based on the protocol version.
func maxNetAddressPayload(pver uint32) uint32 {
	// Services 8 bytes + ip 16 bytes + port 2 bytes.
	plen := uint32(26)

	// NetAddressTimeVersion added a timestamp field.
	if pver >= NetAddressTimeVersion {
		// Timestamp 4 bytes.
		plen += 4
	}

	return plen
}

// NetAddress defines information about a peer on the network including the time
// it was last seen, the services it supports, its IP address, and port.
type NetAddress struct {
	// Last time the address was seen.  This is, unfortunately, encoded as a
	// uint32 on the wire and therefore is limited to 2106.  This field is
	// not present in the bitcoin version message (MsgVersion) nor was it
	// added until protocol version >= NetAddressTimeVersion.
	Timestamp time.Time

	// Bitfield which identifies the services supported by the address.
	Services ServiceFlag

	// IP address of the peer.
	IP net.IP

	// Port the peer is using.  This is encoded in big endian on the wire
	// which differs from most everything else.
	Port uint16
}

// HasService returns whether the specified service is supported by the address.
func (na *NetAddress) HasService(service ServiceFlag) bool {
	if na.Services&service == service {
		return true
	}
	return false
}

// AddService adds service as a supported service by the peer generating the
// message.
func (na *NetAddress) AddService(service ServiceFlag) {
	na.Services |= service
}

// SetAddress is a convenience function to set the IP address and port in one
// call.
func (na *NetAddress) SetAddress(ip net.IP, port uint16) {
	na.IP = ip
	na.Port = port
}

// NewNetAddressIPPort returns a new NetAddress using the provided IP, port, and
// supported services with defaults for the remaining fields.
func NewNetAddressIPPort(ip net.IP, port uint16, services ServiceFlag) *NetAddress {
	// Limit the timestamp to one second precision since the protocol
	// doesn't support better.
	na := NetAddress{
		Timestamp: time.Unix(time.Now().Unix(), 0),
		Services:  services,
		IP:        ip,
		Port:      port,
	}
	return &na
}

// NewNetAddress returns a new NetAddress using the provided TCP address and
// supported services with defaults for the remaining fields.
//
// Note that addr must be a net.TCPAddr.  An ErrInvalidNetAddr is returned
// if it is not.
func NewNetAddress(addr net.Addr, services ServiceFlag) (*NetAddress, error) {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return nil, ErrInvalidNetAddr
	}

	na := NewNetAddressIPPort(tcpAddr.IP, uint16(tcpAddr.Port), services)
	return na, nil
}

// readNetAddress reads an encoded NetAddress from r depending on the protocol
// version and whether or not the timestamp is included per ts.  Some messages
// like version do not include the timestamp.
func readNetAddress(r io.Reader, pver uint32, na *NetAddress, ts bool) error {
	var timestamp time.Time
	var services ServiceFlag
	var ip [16]byte
	var port uint16

	// NOTE: The bitcoin protocol uses a uint32 for the timestamp so it will
	// stop working somewhere around 2106.  Also timestamp wasn't added until
	// protocol version >= NetAddressTimeVersion
	if ts && pver >= NetAddressTimeVersion {
		var stamp uint32
		err := readElement(r, &stamp)
		if err != nil {
			return err
		}
		timestamp = time.Unix(int64(stamp), 0)
	}

	err := readElements(r, &services, &ip)
	if err != nil {
		return err
	}
	// Sigh.  Bitcoin protocol mixes little and big endian.
	err = binary.Read(r, binary.BigEndian, &port)
	if err != nil {
		return err
	}

	na.Timestamp = timestamp
	na.Services = services
	na.SetAddress(net.IP(ip[:]), port)
	return nil
}

// writeNetAddress serializes a NetAddress to w depending on the protocol
// version and whether or not the timestamp is included per ts.  Some messages
// like version do not include the timestamp.
func writeNetAddress(w io.Writer, pver uint32, na *NetAddress, ts bool) error {
	// NOTE: The bitcoin protocol uses a uint32 for the timestamp so it will
	// stop working somewhere around 2106.  Also timestamp wasn't added until
	// until protocol version >= NetAddressTimeVersion.
	if ts && pver >= NetAddressTimeVersion {
		err := writeElement(w, uint32(na.Timestamp.Unix()))
		if err != nil {
			return err
		}
	}

	// Ensure to always write 16 bytes even if the ip is nil.
	var ip [16]byte
	if na.IP != nil {
		copy(ip[:], na.IP.To16())
	}
	err := writeElements(w, na.Services, ip)
	if err != nil {
		return err
	}

	// Sigh.  Bitcoin protocol mixes little and big endian.
	err = binary.Write(w, binary.BigEndian, na.Port)
	if err != nil {
		return err
	}

	return nil
}
