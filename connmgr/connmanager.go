// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"github.com/btcsuite/btcd/addrmgr"
	"net"
	"strings"
)

type Config struct {
	DataDir     string
	onionlookup func(string) ([]net.IP, error)
	lookup      func(string) ([]net.IP, error)
	oniondial   func(string, string) (net.Conn, error)
	dial        func(string, string) (net.Conn, error)
}

// ConnManager provides a generic connection manager for the bitcoin network.
type ConnManager struct {
	cfg         *Config
	addrManager *addrmgr.AddrManager
}

// Dial connects to the address on the named network using the appropriate dial
// function depending on the address and configuration options.  For example,
// .onion addresses will be dialed using the onion specific proxy if one was
// specified, but will otherwise use the normal dial function (which could
// itself use a proxy or not).
func (cm *ConnManager) Dial(network, address string) (net.Conn, error) {
	if strings.Contains(address, ".onion:") {
		return cm.cfg.oniondial(network, address)
	}
	return cm.cfg.dial(network, address)
}

// Lookup returns the correct DNS lookup function to use depending on the
// passed host and configuration options.  For example, .onion addresses will
// be resolved using the onion specific proxy if one was specified, but will
// otherwise treat the normal proxy as tor unless --noonion was specified in
// which case the lookup will fail.  Meanwhile, normal IP addresses will be
// resolved using tor if a proxy was specified unless --noonion was also
// specified in which case the normal system DNS resolver will be used.
func (cm *ConnManager) Lookup(host string) ([]net.IP, error) {
	if strings.HasSuffix(host, ".onion") {
		return cm.cfg.onionlookup(host)
	}
	return cm.cfg.lookup(host)
}

// New returns a new bitcoin connection manager.
// Use Start to begin processing asynchronous connection management.
func New(cfg *Config) *ConnManager {
	cm := ConnManager{}
	amgr := addrmgr.New(cfg.DataDir, cm.Lookup)
	cm.addrManager = amgr
	return &cm
}
