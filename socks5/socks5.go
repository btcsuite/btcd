// Copyright (c) 2015 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package socks5

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	protocolVersion = 5

	addressTypeIPv4   = 1
	addressTypeDomain = 3
	addressTypeIPv6   = 4

	authNone             = 0
	authGssAPI           = 1
	authUsernamePassword = 2
	authUnavailable      = 0xff

	commandTCPConnect   = 1
	commandTCPBind      = 2
	commandUDPAssociate = 3
)

var (
	// ErrAuthFailed is returned on authentication failures.
	ErrAuthFailed = errors.New("authentication failed")

	// ErrInvalidProxyResponse is returned when the proxy server
	// sends an invalid proxy response.
	ErrInvalidProxyResponse = errors.New("invalid proxy response")

	// ErrNoAcceptableAuthMethod is returned when the proxy server
	// does not contain any acceptable authentication methods.
	ErrNoAcceptableAuthMethod = errors.New("no acceptable authentication method")

	statusRequestGranted          = byte(0)
	statusGeneralFailure          = byte(1)
	statusConnectionNotAllowed    = byte(2)
	statusNetworkUnreachable      = byte(3)
	statusHostUnreachable         = byte(4)
	statusConnectionRefused       = byte(5)
	statusTTLExpired              = byte(6)
	statusCommandNotSupport       = byte(7)
	statusAddressTypeNotSupported = byte(8)

	statusErrors = map[byte]error{
		statusGeneralFailure:          errors.New("general failure"),
		statusConnectionNotAllowed:    errors.New("connection not allowed by ruleset"),
		statusNetworkUnreachable:      errors.New("network unreachable"),
		statusHostUnreachable:         errors.New("host unreachable"),
		statusConnectionRefused:       errors.New("connection refused by destination host"),
		statusTTLExpired:              errors.New("TTL expired"),
		statusCommandNotSupport:       errors.New("command not supported / protocol error"),
		statusAddressTypeNotSupported: errors.New("address type not supported"),
	}
)

// Socks5 houses information regarding a socks5 instance.
type Socks5 struct {
	mtx        sync.Mutex
	addr       string
	host       string
	port       string
	user       string
	pass       string
	randomized bool // random user credentials
}

// Dial attempts to connect to the passed address via a proxy
// server.
func (s *Socks5) Dial(network, addr string) (net.Conn, error) {
	remoteHost, strPort, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	remotePort, err := strconv.Atoi(strPort)
	if err != nil {
		return nil, err
	}

	s.mtx.Lock()
	var user, pass string
	if s.randomized {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		user = fmt.Sprintf("%08d", r.Int63())
		pass = fmt.Sprintf("%08d", r.Int63())
	} else {
		user = s.user
		pass = s.pass
	}
	paddr := s.addr
	s.mtx.Unlock()

	conn, err := net.Dial("tcp", paddr)
	if err != nil {
		return nil, err
	}

	if s.user != "" {
		buf := []byte{
			protocolVersion,
			2, // number of authentication methods
			authNone,
			authUsernamePassword,
		}
		_, err = conn.Write(buf)
	} else {
		buf := []byte{
			protocolVersion,
			1, // number of authencation methods
			authNone,
		}
		_, err = conn.Write(buf)
	}
	if err != nil {
		conn.Close()
		return nil, err
	}

	var res [2]byte
	if _, err := io.ReadFull(conn, res[:]); err != nil {
		conn.Close()
		return nil, err
	}
	if res[0] != protocolVersion {
		conn.Close()
		return nil, ErrInvalidProxyResponse
	}

	switch res[1] {
	case authGssAPI:
		err = ErrNoAcceptableAuthMethod
	case authNone:
		// Do nothing
	case authUnavailable:
		err = ErrNoAcceptableAuthMethod
	case authUsernamePassword:
		buf := make([]byte, 3+len(user)+len(pass))
		buf[0] = 1 // version
		buf[1] = byte(len(user))
		copy(buf[2:], user)
		buf[2+len(user)] = byte(len(pass))
		copy(buf[2+len(user)+1:], pass)
		if _, err = conn.Write(buf); err != nil {
			conn.Close()
			return nil, err
		}
		if _, err = io.ReadFull(conn, buf[:2]); err != nil {
			conn.Close()
			return nil, err
		}
		if buf[0] != 1 { // version
			err = ErrInvalidProxyResponse
		} else if buf[1] != 0 { // 0 = success
			err = ErrAuthFailed
		}
	default:
		err = ErrInvalidProxyResponse
	}

	if err != nil {
		conn.Close()
		return nil, err
	}

	// Command and connection request.
	buf := make([]byte, 5+len(remoteHost)+2)
	buf[0] = protocolVersion
	buf[1] = commandTCPConnect
	buf[2] = 0 // reserved
	buf[3] = addressTypeDomain
	buf[4] = byte(len(remoteHost))
	copy(buf[5:], remoteHost)
	buf[5+len(remoteHost)] = byte(remotePort >> 8)
	buf[6+len(remoteHost)] = byte(remotePort & 0xff)
	if _, err := conn.Write(buf); err != nil {
		conn.Close()
		return nil, err
	}

	// Server response
	var res2 [4]byte
	if _, err := io.ReadFull(conn, res2[:]); err != nil {
		conn.Close()
		return nil, err
	}

	if res2[0] != protocolVersion {
		conn.Close()
		return nil, ErrInvalidProxyResponse
	}
	if res2[1] != statusRequestGranted {
		conn.Close()
		err := statusErrors[res2[1]]
		if err == nil {
			err = ErrInvalidProxyResponse
		}
		return nil, err
	}

	switch res2[3] {
	case addressTypeDomain:
		var domainLen [1]byte
		if _, err := io.ReadFull(conn, domainLen[:]); err != nil {
			conn.Close()
			return nil, err
		}
		b := make([]byte, int(domainLen[0]))
		if _, err := io.ReadFull(conn, b); err != nil {
			conn.Close()
			return nil, err
		}
	case addressTypeIPv4:
		var b [4]byte
		if _, err := io.ReadFull(conn, b[:]); err != nil {
			conn.Close()
			return nil, err
		}
	case addressTypeIPv6:
		var b [16]byte
		if _, err := io.ReadFull(conn, b[:]); err != nil {
			conn.Close()
			return nil, err
		}
	default:
		conn.Close()
		return nil, ErrInvalidProxyResponse
	}

	if _, err := io.ReadFull(conn, res2[:2]); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

// IsRandomized returns whether or not proxy user credentials
// are randomized.
func (s *Socks5) IsRandomized() bool {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.randomized
}

// SetRandomized sets whether or not proxy user credentials should
// be randomized.
func (s *Socks5) SetRandomized(randomized bool) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.randomized = randomized
}

// New returns a new Socks5 instance.
func New(addr string, user, pass string) (*Socks5, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	return &Socks5{
		addr: addr,
		host: host,
		port: port,
		user: user,
		pass: pass,
	}, nil
}
