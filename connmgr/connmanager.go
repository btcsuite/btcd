// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"github.com/btcsuite/btcd/addrmgr"
	"net"
)

var (
	// Lookup looks up the host using the local resolver.
	Lookup func(string) ([]net.IP, error) = net.LookupIP

	// Dial connects to the address on the named network.
	Dial func(string, string) (net.Conn, error) = net.Dial
)

// ConnManager provides a generic connection manager for the bitcoin network.
type ConnManager struct {
	addrManager *addrmgr.AddrManager
}

func (cm *ConnManager) Start() {
	cm.addrManager.Start()
}

// New returns a new bitcoin connection manager.
// Use Start to begin processing asynchronous connection management.
func New(DataDir string) (*ConnManager, error) {
	amgr := addrmgr.New(DataDir, Lookup)
	cm := ConnManager{
		addrManager: amgr,
	}
	return &cm, nil
}
