// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"net"
	"time"
)

// fakeConn implements the net.Conn interface and is used to test functions
// which work with a net.Conn without having to actually make any real
// connections.
type fakeConn struct {
	localAddr  net.Addr
	remoteAddr net.Addr
}

// Read doesn't do anything.  It just satisfies the net.Conn interface.
func (c *fakeConn) Read(b []byte) (n int, err error) {
	return 0, nil
}

// Write doesn't do anything.  It just satisfies the net.Conn interface.
func (c *fakeConn) Write(b []byte) (n int, err error) {
	return 0, nil
}

// Close doesn't do anything.  It just satisfies the net.Conn interface.
func (c *fakeConn) Close() error {
	return nil
}

// LocalAddr returns the localAddr field of the fake connection and satisfies
// the net.Conn interface.
func (c *fakeConn) LocalAddr() net.Addr {
	return c.localAddr
}

// RemoteAddr returns the remoteAddr field of the fake connection and satisfies
// the net.Conn interface.
func (c *fakeConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

// SetDeadline doesn't do anything.  It just satisfies the net.Conn interface.
func (c *fakeConn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline doesn't do anything.  It just satisfies the net.Conn
// interface.
func (c *fakeConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline doesn't do anything.  It just satisfies the net.Conn
// interface.
func (c *fakeConn) SetWriteDeadline(t time.Time) error {
	return nil
}
