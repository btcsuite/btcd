package socks

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testTimeout bounds how long a test waits for the test proxy.
const testTimeout = 5 * time.Second

// proxyRequest is what a client sent to the test proxy.
type proxyRequest struct {
	// user and pass are the credentials the client authenticated with, if
	// any.
	user, pass string

	// addrType is the SOCKS address type of the destination.
	addrType byte

	// host and port are the requested destination.
	host string
	port int
}

// successReply is the reply of a SOCKS5 proxy that connected to the
// destination and reports 0.0.0.0:0 as the bound address, as Tor does.
var successReply = []byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}

// startProxy runs a minimal SOCKS5 proxy that sends reply to every connect
// request, then keeps the connection open. It returns the proxy address and a
// channel that receives each request.
func startProxy(t *testing.T, reply []byte) (string, chan proxyRequest) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ln.Close()
	})

	requests := make(chan proxyRequest, 10)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()

				req, err := readRequest(conn)
				if err != nil {
					return
				}
				requests <- req
				if _, err := conn.Write(reply); err != nil {
					return
				}
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()

	return ln.Addr().String(), requests
}

// readRequest reads the method negotiation, the optional username/password
// authentication and the connect request of a SOCKS5 client.
func readRequest(conn net.Conn) (proxyRequest, error) {
	var req proxyRequest

	// Greeting: version, number of methods, methods.
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return req, err
	}
	methods := make([]byte, header[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return req, err
	}

	// Pick username/password authentication when offered.
	if !bytes.Contains(methods, []byte{2}) {
		if _, err := conn.Write([]byte{5, 0}); err != nil {
			return req, err
		}
	} else {
		if _, err := conn.Write([]byte{5, 2}); err != nil {
			return req, err
		}
		user, pass, err := readCredentials(conn)
		if err != nil {
			return req, err
		}
		req.user, req.pass = user, pass
		if _, err := conn.Write([]byte{1, 0}); err != nil {
			return req, err
		}
	}

	// Request: version, command, reserved, address type, address, port.
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return req, err
	}
	req.addrType = head[3]

	var addr []byte
	switch req.addrType {
	case 1:
		addr = make([]byte, net.IPv4len)
	case 4:
		addr = make([]byte, net.IPv6len)
	case 3:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return req, err
		}
		addr = make([]byte, length[0])
	}
	if _, err := io.ReadFull(conn, addr); err != nil {
		return req, err
	}
	if req.addrType == 3 {
		req.host = string(addr)
	} else {
		req.host = net.IP(addr).String()
	}

	port := make([]byte, 2)
	if _, err := io.ReadFull(conn, port); err != nil {
		return req, err
	}
	req.port = int(binary.BigEndian.Uint16(port))

	return req, nil
}

// readCredentials reads a username/password authentication request.
func readCredentials(conn net.Conn) (string, string, error) {
	var b [1]byte
	field := func() (string, error) {
		if _, err := io.ReadFull(conn, b[:]); err != nil {
			return "", err
		}
		value := make([]byte, b[0])
		_, err := io.ReadFull(conn, value)

		return string(value), err
	}

	// Version of the subnegotiation.
	if _, err := io.ReadFull(conn, b[:]); err != nil {
		return "", "", err
	}
	user, err := field()
	if err != nil {
		return "", "", err
	}
	pass, err := field()

	return user, pass, err
}

// receive returns the next request the test proxy received.
func receive(t *testing.T, requests chan proxyRequest) proxyRequest {
	t.Helper()

	select {
	case req := <-requests:
		return req

	case <-time.After(testTimeout):
		t.Fatal("the proxy received no request")
		return proxyRequest{}
	}
}

// dial dials addr through p and closes the connection when the test ends.
func dial(t *testing.T, p *Proxy, addr string) net.Conn {
	t.Helper()

	conn, err := p.DialTimeout("tcp", addr, testTimeout)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	return conn
}

// TestDialAddresses checks that a connection reports the destination as its
// remote address and the address bound by the proxy as its local address.
func TestDialAddresses(t *testing.T) {
	t.Parallel()

	proxyAddr, requests := startProxy(t, successReply)
	conn := dial(t, &Proxy{Addr: proxyAddr}, "1.2.3.4:8333")

	req := receive(t, requests)
	require.Equal(t, byte(1), req.addrType)
	require.Equal(t, "1.2.3.4", req.host)
	require.Equal(t, 8333, req.port)
	require.Empty(t, req.user)

	require.Equal(t, &ProxiedAddr{Net: "tcp", Host: "1.2.3.4", Port: 8333},
		conn.RemoteAddr())
	require.Equal(t, "0.0.0.0:0", conn.LocalAddr().String())
}

// TestDialHostname checks that a hostname is sent to the proxy unresolved and
// reported as the remote address, also with the "onion" network, which btcd
// uses to dial onion addresses.
func TestDialHostname(t *testing.T) {
	t.Parallel()

	host := "vww6ybal4bd7szmgncyruucpgfkqahzddi37ktceo3ah7ngmcopnpyyd.onion"
	addr := net.JoinHostPort(host, "8333")

	for _, network := range []string{"tcp", "onion"} {
		proxyAddr, requests := startProxy(t, successReply)
		p := &Proxy{Addr: proxyAddr}
		conn, err := p.DialTimeout(network, addr, testTimeout)
		require.NoError(t, err, network)
		t.Cleanup(func() {
			_ = conn.Close()
		})

		req := receive(t, requests)
		require.Equal(t, byte(3), req.addrType, network)
		require.Equal(t, host, req.host, network)
		want := &ProxiedAddr{Net: network, Host: host, Port: 8333}
		require.Equal(t, want, conn.RemoteAddr())
	}
}

// TestDialCredentials checks that the configured credentials are sent to the
// proxy.
func TestDialCredentials(t *testing.T) {
	t.Parallel()

	proxyAddr, requests := startProxy(t, successReply)
	p := &Proxy{Addr: proxyAddr, Username: "user", Password: "pass"}
	dial(t, p, "1.2.3.4:8333")

	req := receive(t, requests)
	require.Equal(t, "user", req.user)
	require.Equal(t, "pass", req.pass)
}

// TestDialTorIsolation checks that with Tor stream isolation, every
// connection uses new random credentials instead of the configured ones.
func TestDialTorIsolation(t *testing.T) {
	t.Parallel()

	proxyAddr, requests := startProxy(t, successReply)
	p := &Proxy{
		Addr:         proxyAddr,
		Username:     "user",
		Password:     "pass",
		TorIsolation: true,
	}

	dial(t, p, "1.2.3.4:8333")
	first := receive(t, requests)
	dial(t, p, "1.2.3.4:8333")
	second := receive(t, requests)

	for _, req := range []proxyRequest{first, second} {
		require.Len(t, req.user, 16)
		require.Len(t, req.pass, 16)
		require.NotEqual(t, "user", req.user)
	}
	require.NotEqual(t, first.user, second.user)
	require.NotEqual(t, first.pass, second.pass)
}

// TestDialLongBoundHostname checks that a long hostname as the bound address
// in the proxy's reply is handled. btcsuite/go-socks panicked on it.
func TestDialLongBoundHostname(t *testing.T) {
	t.Parallel()

	name := strings.Repeat("a", 255)
	reply := append([]byte{5, 0, 0, 3, byte(len(name))}, name...)
	reply = append(reply, 0x20, 0x8d)

	proxyAddr, _ := startProxy(t, reply)
	conn := dial(t, &Proxy{Addr: proxyAddr}, "1.2.3.4:8333")

	require.Equal(t, net.JoinHostPort(name, strconv.Itoa(8333)),
		conn.LocalAddr().String())
}

// TestDialTimeout checks that the timeout covers the SOCKS handshake, so a
// proxy that never answers can't hang the dial.
func TestDialTimeout(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ln.Close()
	})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(io.Discard, conn)
	}()

	p := &Proxy{Addr: ln.Addr().String()}
	start := time.Now()
	_, err = p.DialTimeout("tcp", "1.2.3.4:8333", 100*time.Millisecond)
	require.Error(t, err)
	require.Less(t, time.Since(start), testTimeout)
}
