// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"github.com/conformal/btcwire"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	// maxAddresses identifies the maximum number of addresses that the
	// address manager will track.
	maxAddresses         = 2500
	newAddressBufferSize = 50
	dumpAddressInterval  = time.Minute * 2
)

// updateAddress is a helper function to either update an address already known
// to the address manager, or to add the address if not already known.
func updateAddress(a *AddrManager, netAddr *btcwire.NetAddress) {
	// Protect concurrent access.
	a.addrCacheLock.Lock()
	defer a.addrCacheLock.Unlock()

	// Update address if it already exists.
	addr := NetAddressKey(netAddr)
	if na, ok := a.addrCache[addr]; ok {
		// Update the last seen time.
		if netAddr.Timestamp.After(na.Timestamp) {
			na.Timestamp = netAddr.Timestamp
		}

		// Update services.
		na.AddService(na.Services)

		log.Tracef("[AMGR] Updated address manager address %s", addr)
		return
	}

	// Enforce max addresses.
	if len(a.addrCache)+1 > maxAddresses {
		log.Tracef("[AMGR] Max addresses of %d reached", maxAddresses)
		return
	}

	a.addrCache[addr] = netAddr
	log.Tracef("[AMGR] Added new address %s for a total of %d addresses",
		addr, len(a.addrCache))
}

// AddrManager provides a concurrency safe address manager for caching potential
// peers on the bitcoin network.
type AddrManager struct {
	addrCache       map[string]*btcwire.NetAddress
	addrCacheLock   sync.Mutex
	started         bool
	shutdown        bool
	newAddresses    chan []*btcwire.NetAddress
	removeAddresses chan []*btcwire.NetAddress
	wg              sync.WaitGroup
	quit            chan bool
}

// addressHandler is the main handler for the address manager.  It must be run
// as a goroutine.
func (a *AddrManager) addressHandler() {
	dumpAddressTicker := time.NewTicker(dumpAddressInterval)
out:
	for !a.shutdown {
		select {
		case addrs := <-a.newAddresses:
			for _, na := range addrs {
				updateAddress(a, na)
			}

		case <-dumpAddressTicker.C:
			if !a.shutdown {
				// TODO: Dump addresses to database.
			}

		case <-a.quit:
			// TODO: Dump addresses to database.
			break out
		}
	}
	dumpAddressTicker.Stop()
	a.wg.Done()
	log.Trace("[AMGR] Address handler done")
}

// Start begins the core address handler which manages a pool of known
// addresses, timeouts, and interval based writes.
func (a *AddrManager) Start() {
	// Already started?
	if a.started {
		return
	}

	log.Trace("[AMGR] Starting address manager")

	go a.addressHandler()
	a.wg.Add(1)
	a.started = true
}

// Stop gracefully shuts down the address manager by stopping the main handler.
func (a *AddrManager) Stop() error {
	if a.shutdown {
		log.Warnf("[AMGR] Address manager is already in the process of " +
			"shutting down")
		return nil
	}

	log.Infof("[AMGR] Address manager shutting down")
	a.shutdown = true
	a.quit <- true
	a.wg.Wait()
	return nil
}

// AddAddresses adds new addresses to the address manager.  It enforces a max
// number of addresses and silently ignores duplicate addresses.  It is
// safe for concurrent access.
func (a *AddrManager) AddAddresses(addrs []*btcwire.NetAddress) {
	a.newAddresses <- addrs
}

// AddAddress adds a new address to the address manager.  It enforces a max
// number of addresses and silently ignores duplicate addresses.  It is
// safe for concurrent access.
func (a *AddrManager) AddAddress(addr *btcwire.NetAddress) {
	addrs := []*btcwire.NetAddress{addr}
	a.newAddresses <- addrs
}

// NeedMoreAddresses returns whether or not the address manager needs more
// addresses.
func (a *AddrManager) NeedMoreAddresses() bool {
	// Protect concurrent access.
	a.addrCacheLock.Lock()
	defer a.addrCacheLock.Unlock()

	return len(a.addrCache)+1 <= maxAddresses
}

// NumAddresses returns the number of addresses known to the address manager.
func (a *AddrManager) NumAddresses() int {
	// Protect concurrent access.
	a.addrCacheLock.Lock()
	defer a.addrCacheLock.Unlock()

	return len(a.addrCache)
}

// AddressCache returns the current address cache.  It must be treated as
// read-only.
func (a *AddrManager) AddressCache() map[string]*btcwire.NetAddress {
	return a.addrCache
}

// New returns a new bitcoin address manager.
// Use Start to begin processing asynchronous address updates.
func NewAddrManager() *AddrManager {
	am := AddrManager{
		addrCache:    make(map[string]*btcwire.NetAddress),
		newAddresses: make(chan []*btcwire.NetAddress, newAddressBufferSize),
		quit:         make(chan bool),
	}
	return &am
}

// NetAddressKey returns a string key in the form of ip:port for IPv4 addresses
// or [ip]:port for IPv6 addresses.
func NetAddressKey(na *btcwire.NetAddress) string {
	port := strconv.FormatUint(uint64(na.Port), 10)
	addr := net.JoinHostPort(na.IP.String(), port)
	return addr
}
