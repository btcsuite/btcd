// Package socks dials connections through a SOCKS5 proxy such as Tor.
//
// It wraps golang.org/x/net/proxy and keeps the behavior btcd relied on from
// github.com/btcsuite/go-socks:
//
//   - A connection reports the requested destination as its remote address,
//     and the address the proxy bound for it as its local address, rather
//     than the addresses of the connection to the proxy.
//   - With Tor stream isolation, every connection authenticates with new
//     random credentials, so Tor puts it on a circuit of its own.
//   - Destinations are sent to the proxy unresolved, so the proxy resolves
//     hostnames, including onion addresses.
package socks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"strconv"
	"time"

	"golang.org/x/net/proxy"
)

// Proxy dials connections through a SOCKS5 proxy.
type Proxy struct {
	// Addr is the host:port address of the proxy.
	Addr string

	// Username and Password are the credentials for the proxy, if it
	// requires them. They are ignored when TorIsolation is set.
	Username string
	Password string

	// TorIsolation makes every connection authenticate with new random
	// credentials, which Tor uses to isolate it on a circuit of its own.
	TorIsolation bool
}

// ProxiedAddr is the address of a connection made through a proxy. Host may
// be a hostname, such as an onion address, since the proxy resolves it.
type ProxiedAddr struct {
	Net  string
	Host string
	Port int
}

// Network returns the network of the address, such as "tcp".
func (a *ProxiedAddr) Network() string {
	return a.Net
}

// String returns the address in host:port form.
func (a *ProxiedAddr) String() string {
	return net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
}

// Dial connects to addr through the proxy.
func (p *Proxy) Dial(network, addr string) (net.Conn, error) {
	return p.DialContext(context.Background(), network, addr)
}

// DialTimeout is like Dial, but gives up after timeout, which covers both
// connecting to the proxy and the SOCKS handshake. A zero timeout means no
// timeout.
func (p *Proxy) DialTimeout(network, addr string,
	timeout time.Duration) (net.Conn, error) {

	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	return p.DialContext(ctx, network, addr)
}

// DialContext connects to addr through the proxy. The network is "tcp",
// "tcp4", "tcp6" or "onion", which btcd uses for onion addresses. The context
// covers both connecting to the proxy and the SOCKS handshake.
func (p *Proxy) DialContext(ctx context.Context, network,
	addr string) (net.Conn, error) {

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}

	auth, err := p.auth()
	if err != nil {
		return nil, err
	}
	dialer, err := proxy.SOCKS5("tcp", p.Addr, auth, proxy.Direct)
	if err != nil {
		return nil, err
	}

	// x/net doesn't know the "onion" network. The proxy connects to onion
	// addresses over TCP like to any other host.
	proxyNetwork := network
	if network == "onion" {
		proxyNetwork = "tcp"
	}

	// The SOCKS5 dialer implements DialContext, which, unlike its Dial,
	// returns a connection that knows the address the proxy bound.
	conn, err := dialer.(proxy.ContextDialer).DialContext(
		ctx, proxyNetwork, addr,
	)
	if err != nil {
		return nil, err
	}

	var boundAddr net.Addr
	if c, ok := conn.(interface{ BoundAddr() net.Addr }); ok {
		boundAddr = c.BoundAddr()
	}

	return &proxiedConn{
		Conn:      conn,
		boundAddr: boundAddr,
		remoteAddr: &ProxiedAddr{
			Net:  network,
			Host: host,
			Port: port,
		},
	}, nil
}

// auth returns the credentials to authenticate with the proxy, or nil to not
// authenticate.
func (p *Proxy) auth() (*proxy.Auth, error) {
	if p.TorIsolation {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, err
		}

		return &proxy.Auth{
			User:     hex.EncodeToString(b[:8]),
			Password: hex.EncodeToString(b[8:]),
		}, nil
	}

	// SOCKS5 username/password authentication needs a username.
	if p.Username == "" {
		return nil, nil
	}

	return &proxy.Auth{User: p.Username, Password: p.Password}, nil
}

// proxiedConn is a connection made through a proxy.
type proxiedConn struct {
	net.Conn

	// boundAddr is the address the proxy bound for the connection, if it
	// reported one.
	boundAddr net.Addr

	// remoteAddr is the requested destination.
	remoteAddr *ProxiedAddr
}

// LocalAddr returns the address the proxy bound for the connection, or the
// local address of the connection to the proxy if it didn't report one.
func (c *proxiedConn) LocalAddr() net.Addr {
	if c.boundAddr != nil {
		return c.boundAddr
	}

	return c.Conn.LocalAddr()
}

// RemoteAddr returns the requested destination.
func (c *proxiedConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}
