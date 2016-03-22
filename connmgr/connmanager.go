// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	mrand "math/rand"
	"net"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/addrmgr"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

var (
	// Lookup looks up the host using the local resolver.
	Lookup func(string) ([]net.IP, error) = net.LookupIP

	// Dial connects to the address on the named network.
	Dial func(string, string) (net.Conn, error) = net.Dial

	// ChainParams identifies the chain params to use.
	ChainParams *chaincfg.Params = &chaincfg.MainNetParams

	// PermanentPeers is a list of peers to maintain permanent connections.
	PermanentPeers []string
)

// ConnResult handles the result of an Connect request.
type ConnResult struct {
	Conn net.Conn
	Err  error
}

// ConnManager provides a generic connection manager for the bitcoin network.
type ConnManager struct {
	AddrManager *addrmgr.AddrManager
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
			cm.AddrManager.AddAddresses(addresses, addresses[0])
		}(seeder)
	}
}

func (cm *ConnManager) Start() {
	cm.AddrManager.Start()
	cm.seedFromDNS(ChainParams.DNSSeeds)
	// TODO: Connect PermanentPeers
}

// Connect handles a connection request to the given addr and
// returns a chan with the connection result.
func (cm *ConnManager) Connect(addr string) <-chan *ConnResult {
	c := make(chan *ConnResult)
	go func() {
		conn, err := Dial("tcp", addr)
		result := &ConnResult{
			Conn: conn,
			Err:  err,
		}
		c <- result
	}()
	return c
}

// New returns a new bitcoin connection manager.
// Use Start to begin processing asynchronous connection management.
func New(DataDir string) (*ConnManager, error) {
	amgr := addrmgr.New(DataDir, Lookup)
	cm := ConnManager{
		AddrManager: amgr,
	}
	return &cm, nil
}
