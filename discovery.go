// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

const (
	torSucceeded         = 0x00
	torGeneralError      = 0x01
	torNotAllowed        = 0x02
	torNetUnreachable    = 0x03
	torHostUnreachable   = 0x04
	torConnectionRefused = 0x05
	torTtlExpired        = 0x06
	torCmdNotSupported   = 0x07
	torAddrNotSupported  = 0x08
)

var (
	ErrTorInvalidAddressResponse = errors.New("Invalid address response")
	ErrTorInvalidProxyResponse   = errors.New("Invalid proxy response")
	ErrTorUnrecognizedAuthMethod = errors.New("Invalid proxy authentication method")

	torStatusErrors = map[byte]error{
		torSucceeded:         errors.New("Tor succeeded"),
		torGeneralError:      errors.New("Tor general error"),
		torNotAllowed:        errors.New("Tor not allowed"),
		torNetUnreachable:    errors.New("Tor network is unreachable"),
		torHostUnreachable:   errors.New("Tor host is unreachable"),
		torConnectionRefused: errors.New("Tor connection refused"),
		torTtlExpired:        errors.New("Tor ttl expired"),
		torCmdNotSupported:   errors.New("Tor command not supported"),
		torAddrNotSupported:  errors.New("Tor address type not supported"),
	}
)

// try individual DNS server return list of strings for responses.
func doDNSLookup(host, proxy string) ([]net.IP, error) {
	var err error
	var addrs []net.IP

	if proxy != "" {
		addrs, err = torLookupIP(host, proxy)
	} else {
		addrs, err = net.LookupIP(host)
	}
	if err != nil {
		return nil, err
	}

	return addrs, nil
}

// Use Tor to resolve DNS.
/*
 TODO:
 * this function must be documented internally
 * this function does not handle IPv6
*/
func torLookupIP(host, proxy string) ([]net.IP, error) {
	conn, err := net.Dial("tcp", proxy)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	buf := []byte{'\x05', '\x01', '\x00'}
	_, err = conn.Write(buf)
	if err != nil {
		return nil, err
	}

	buf = make([]byte, 2)
	_, err = conn.Read(buf)
	if err != nil {
		return nil, err
	}
	if buf[0] != '\x05' {
		return nil, ErrTorInvalidProxyResponse
	}
	if buf[1] != '\x00' {
		return nil, ErrTorUnrecognizedAuthMethod
	}

	buf = make([]byte, 7+len(host))
	buf[0] = 5      // protocol version
	buf[1] = '\xF0' // Tor Resolve
	buf[2] = 0      // reserved
	buf[3] = 3      // Tor Resolve
	buf[4] = byte(len(host))
	copy(buf[5:], host)
	buf[5+len(host)] = 0 // Port 0

	_, err = conn.Write(buf)
	if err != nil {
		return nil, err
	}

	buf = make([]byte, 4)
	_, err = conn.Read(buf)
	if err != nil {
		return nil, err
	}
	if buf[0] != 5 {
		return nil, ErrTorInvalidProxyResponse
	}
	if buf[1] != 0 {
		if int(buf[1]) > len(torStatusErrors) {
			err = ErrTorInvalidProxyResponse
		} else {
			err := torStatusErrors[buf[1]]
			if err == nil {
				err = ErrTorInvalidProxyResponse
			}
		}
		return nil, err
	}
	if buf[3] != 1 {
		err := torStatusErrors[torGeneralError]
		return nil, err
	}

	buf = make([]byte, 4)
	bytes, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	if bytes != 4 {
		return nil, ErrTorInvalidAddressResponse
	}

	r := binary.BigEndian.Uint32(buf)

	addr := make([]net.IP, 1)
	addr[0] = net.IPv4(byte(r>>24), byte(r>>16), byte(r>>8), byte(r))

	return addr, nil
}

// dnsDiscover looks up the list of peers resolved by DNS for all hosts in
// seeders. If proxy is not "" then it is used as a tor proxy for the
// resolution. If any errors occur then the seeder that errored will not have
// any hosts in the list. Therefore if all hosts failed an empty slice of
// strings will be returned.
func dnsDiscover(seeder string, proxy string) []net.IP {
	log.Debugf("DISC: Fetching list of seeds from %v", seeder)
	peers, err := doDNSLookup(seeder, proxy)
	if err != nil {
		seederPlusProxy := seeder
		if proxy != "" {
			seederPlusProxy = fmt.Sprintf("%s (proxy %s)",
				seeder, proxy)
		}
		log.Debugf("DISC: Unable to fetch dns seeds "+
			"from %s: %v", seederPlusProxy, err)
		return []net.IP{}
	}

	return peers
}
