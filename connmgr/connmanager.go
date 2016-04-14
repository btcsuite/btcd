// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"fmt"
	mrand "math/rand"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/addrmgr"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

// LookupFunc defines a function that looks up the host.
type LookupFunc func(string) ([]net.IP, error)

// DialFunc defines a function that dials a connection.
type DialFunc func(string, string) (net.Conn, error)

// Config holds the configuration options related to the connection manager.
type Config struct {
	// MaxOutboundPeers is the number of max outbound connections.
	MaxOutboundPeers uint32

	// MaxConnections is the number of max connections.
	MaxConnections uint32

	// Lookup looks up the host using the local resolver.
	Lookup LookupFunc

	// Dial connects to the address on the named network.
	Dial DialFunc

	// ChainParams identifies the chain params to use.
	ChainParams *chaincfg.Params
}

// ConnResult handles the result of an Connect request.
type ConnResult struct {
	Addr      string
	Permanent bool
	Conn      net.Conn
	Err       error
}

// ConnRequest handles a new Connect request
type ConnRequest struct {
	Addr      string
	Permanent bool
	Result    chan *ConnResult
}

// ConnManager provides a generic connection manager for the bitcoin network.
type ConnManager struct {
	cfg  *Config
	amgr *addrmgr.AddrManager

	wg     sync.WaitGroup
	wakeup chan struct{}
	quit   chan struct{}

	connectionCount      uint32
	newConnections       chan *ConnRequest
	closeConnections     chan uint32
	connections          map[uint32]net.Conn
	outboundGroups       map[string]int
	NewConnectionHandler func(<-chan *ConnResult)
}

// seedFromDNS uses DNS seeding to populate the address manager with peers.
func (cm *ConnManager) seedFromDNS() {
	for _, seeder := range cm.cfg.ChainParams.DNSSeeds {
		go func(seeder string) {
			randSource := mrand.New(mrand.NewSource(time.Now().UnixNano()))

			seedpeers, err := DnsDiscover(seeder, cm.cfg.Lookup)
			if err != nil {
				log.Infof("DNS discovery failed on seed %s: %v", seeder, err)
				return
			}
			numPeers := len(seedpeers)

			log.Infof("%d addresses found from DNS seed %s", numPeers, seeder)

			if numPeers == 0 {
				return
			}
			addresses := make([]*wire.NetAddress, len(seedpeers))
			// if this errors then we have *real* problems
			intPort, _ := strconv.Atoi(cm.cfg.ChainParams.DefaultPort)
			for i, peer := range seedpeers {
				addresses[i] = new(wire.NetAddress)
				addresses[i].SetAddress(peer, uint16(intPort))
				// bitcoind seeds with addresses from
				// a time randomly selected between 3
				// and 7 days ago.
				addresses[i].Timestamp = time.Now().Add(-1 *
					time.Second * time.Duration(secondsIn3Days+
					randSource.Int31n(secondsIn4Days)))
			}

			// Bitcoind uses a lookup of the dns seeder here. This
			// is rather strange since the values looked up by the
			// DNS seed lookups will vary quite a lot.
			// to replicate this behaviour we put all addresses as
			// having come from the first one.
			cm.amgr.AddAddresses(addresses, addresses[0])
		}(seeder)
	}
}

// connectionManager manages a fixed number of outbound connections.
func (cm *ConnManager) connectionManager() {
	time.AfterFunc(10*time.Second, func() { cm.wakeup <- struct{}{} })
out:
	for {
		select {
		case <-cm.wakeup:
			if cm.connectionCount < cm.cfg.MaxOutboundPeers {
				addr := cm.amgr.GetAddress("any")
				if addr == nil {
					break
				}
				key := addrmgr.GroupKey(addr.NetAddress())
				// Address will not be invalid, local or unroutable
				// because addrmanager rejects those on addition.
				// Just check that we don't already have an address
				// in the same group so that we are not connecting
				// to the same network segment at the expense of
				// others.
				if cm.outboundGroups[key] != 0 {
					break
				}

				addrStr := addrmgr.NetAddressKey(addr.NetAddress())

				// allow nondefault ports after 50 failed tries.
				if fmt.Sprintf("%d", addr.NetAddress().Port) !=
					cm.cfg.ChainParams.DefaultPort {
					continue
				}

				cr := cm.Connect(addrStr, false)
				go cm.NewConnectionHandler(cr)
			}
			time.AfterFunc(10*time.Second, func() { cm.wakeup <- struct{}{} })

		case <-cm.quit:
			break out
		}
	}
	cm.wg.Done()
}

// connectionHandler is a service that monitors requests for new
// connections or closed connections and maps addresses to their
// respective connections.
func (cm *ConnManager) connectionHandler() {
out:
	for {
		select {
		case req := <-cm.newConnections:
			connectionID := atomic.AddUint32(&cm.connectionCount, 1)
			conn, err := cm.cfg.Dial("tcp", req.Addr)
			res := &ConnResult{
				Addr:      req.Addr,
				Permanent: req.Permanent,
				Conn:      conn,
				Err:       err,
			}
			req.Result <- res
			cm.connections[connectionID] = conn

		case connectionID := <-cm.closeConnections:
			c := cm.connections[connectionID]
			err := c.Close()
			if err != nil {
				log.Infof("Error closing connection: %v", err)
			}
			delete(cm.connections, connectionID)

		case <-cm.quit:
			break out
		}
	}

	// Close connections
	for _, c := range cm.connections {
		if c != nil {
			c.Close()
		}
	}

	cm.wg.Done()
}

// Start launches the connection manager.
func (cm *ConnManager) Start() {
	cm.seedFromDNS()

	cm.wg.Add(2)
	go cm.connectionHandler()
	go cm.connectionManager()
}

// WaitForShutdown blocks until the connection handlers are stopped.
func (cm *ConnManager) WaitForShutdown() {
	cm.wg.Wait()
}

// Stop gracefully shuts down the connection manager.
func (cm *ConnManager) Stop() {
	log.Warnf("Connection Manager shutting down")
	close(cm.quit)
}

// Connect handles a connection request to the given addr and
// returns a chan with the connection result.
func (cm *ConnManager) Connect(addr string, permanent bool) <-chan *ConnResult {
	c := make(chan *ConnResult)
	req := &ConnRequest{
		Addr:      addr,
		Permanent: permanent,
		Result:    c,
	}
	cm.newConnections <- req
	return c
}

// Disconnect disconnects the given address.
func (cm *ConnManager) Disconnect(connectionID uint32) {
	cm.closeConnections <- connectionID
}

// New returns a new bitcoin connection manager.
// Use Start to begin processing asynchronous connection management.
func New(cfg *Config, amgr *addrmgr.AddrManager, connHandler func(<-chan *ConnResult)) (*ConnManager, error) {
	cm := ConnManager{
		cfg:              cfg,
		newConnections:   make(chan *ConnRequest, cfg.MaxConnections),
		closeConnections: make(chan uint32, cfg.MaxConnections),

		quit:                 make(chan struct{}),
		wakeup:               make(chan struct{}),
		amgr:                 amgr,
		connections:          make(map[uint32]net.Conn, cfg.MaxConnections),
		outboundGroups:       make(map[string]int),
		NewConnectionHandler: connHandler,
	}
	return &cm, nil
}
