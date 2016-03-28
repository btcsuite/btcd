// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	mrand "math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

var (
	// MaxConnections is the number of max connections.
	MaxConnections = 125

	// Lookup looks up the host using the local resolver.
	Lookup func(string) ([]net.IP, error) = net.LookupIP

	// Dial connects to the address on the named network.
	Dial func(string, string) (net.Conn, error) = net.Dial

	// ChainParams identifies the chain params to use.
	ChainParams *chaincfg.Params = &chaincfg.MainNetParams
)

// ConnResult handles the result of an Connect request.
type ConnResult struct {
	Addr      string
	Permanent bool
	Conn      net.Conn
	Err       error
}

// ConnManager provides a generic connection manager for the bitcoin network.
type ConnManager struct {
	wg               sync.WaitGroup
	newConnections   chan *ConnResult
	closeConnections chan string

	connections map[string]net.Conn
}

// seedFromDNS uses DNS seeding to populate the address manager with peers.
func (cm *ConnManager) seedFromDNS(DNSSeeds []string) {
	for _, seeder := range DNSSeeds {
		go func(seeder string) {
			randSource := mrand.New(mrand.NewSource(time.Now().UnixNano()))

			seedpeers, err := DnsDiscover(seeder)
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
			intPort, _ := strconv.Atoi(ChainParams.DefaultPort)
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
			// cm.AddrManager.AddAddresses(addresses, addresses[0])
		}(seeder)
	}
}

// connectionHandler is a service that monitors requests for new
// connections or closed connections and maps addresses to their
// respective connections.
func (cm *ConnManager) connectionHandler() {
	for {
		select {
		case cr := <-cm.newConnections:
			cm.connections[cr.Addr] = cr.Conn

		case addr := <-cm.closeConnections:
			c := cm.connections[addr]
			err := c.Close()
			if err != nil {
				log.Infof("Error closing connection %s: %v", addr, err)
			}
			delete(cm.connections, addr)
		}
	}
	cm.wg.Done()
}

// Start launches the connection manager.
func (cm *ConnManager) Start() {
	cm.seedFromDNS(ChainParams.DNSSeeds)

	cm.wg.Add(1)
	go cm.connectionHandler()
}

// WaitForShutdown blocks until the connection handlers are stopped.
func (cm *ConnManager) WaitForShutdown() {
	cm.wg.Wait()
}

// Connect handles a connection request to the given addr and
// returns a chan with the connection result.
func (cm *ConnManager) Connect(addr string, permanent bool) <-chan *ConnResult {
	c := make(chan *ConnResult)
	go func() {
		conn, err := Dial("tcp", addr)
		result := &ConnResult{
			Addr:      addr,
			Permanent: permanent,
			Conn:      conn,
			Err:       err,
		}
		cm.newConnections <- result
		c <- result
	}()
	return c
}

// Disconnect disconnects the given connection.
func (cm *ConnManager) Disconnect(addr string) {
	cm.closeConnections <- addr
}

// New returns a new bitcoin connection manager.
// Use Start to begin processing asynchronous connection management.
func New() (*ConnManager, error) {
	cm := ConnManager{
		connections: make(map[string]net.Conn, MaxConnections),
	}
	return &cm, nil
}
