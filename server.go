// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/list"
	"errors"
	"fmt"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcwire"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// These constants are used by the DNS seed code to pick a random last seen
// time.
const (
	secondsIn3Days int32 = 24 * 60 * 60 * 3
	secondsIn4Days int32 = 24 * 60 * 60 * 4
)

const (
	// supportedServices describes which services are supported by the
	// server.
	supportedServices = btcwire.SFNodeNetwork

	// connectionRetryInterval is the amount of time to wait in between
	// retries when connecting to persistent peers.
	connectionRetryInterval = time.Second * 10

	// defaultMaxOutbound is the default number of max outbound peers.
	defaultMaxOutbound = 8
)

// broadcastMsg provides the ability to house a bitcoin message to be broadcast
// to all connected peers except specified excluded peers.
type broadcastMsg struct {
	message      btcwire.Message
	excludePeers []*peer
}

// server provides a bitcoin server for handling communications to and from
// bitcoin peers.
type server struct {
	nonce         uint64
	listeners     []net.Listener
	btcnet        btcwire.BitcoinNet
	started       int32 // atomic
	shutdown      int32 // atomic
	shutdownSched int32 // atomic
	addrManager   *AddrManager
	rpcServer     *rpcServer
	blockManager  *blockManager
	txMemPool     *txMemPool
	newPeers      chan *peer
	donePeers     chan *peer
	banPeers      chan *peer
	wakeup        chan bool
	query         chan interface{}
	relayInv      chan *btcwire.InvVect
	broadcast     chan broadcastMsg
	wg            sync.WaitGroup
	quit          chan bool
	db            btcdb.Db
}

// handleAddPeerMsg deals with adding new peers.  It is invoked from the
// peerHandler goroutine.
func (s *server) handleAddPeerMsg(peers *list.List, banned map[string]time.Time, p *peer) bool {
	if p == nil {
		return false
	}

	// Ignore new peers if we're shutting down.
	if atomic.LoadInt32(&s.shutdown) != 0 {
		log.Infof("SRVR: New peer %s ignored - server is shutting "+
			"down", p)
		p.Shutdown()
		return false
	}

	// Disconnect banned peers.
	host, _, err := net.SplitHostPort(p.addr)
	if err != nil {
		log.Debugf("SRVR: can't split hostport %v", err)
		p.Shutdown()
		return false
	}
	if banEnd, ok := banned[host]; ok {
		if time.Now().Before(banEnd) {
			log.Debugf("SRVR: Peer %s is banned for another %v - "+
				"disconnecting", host, banEnd.Sub(time.Now()))
			p.Shutdown()
			return false
		}

		log.Infof("SRVR: Peer %s is no longer banned", host)
		delete(banned, host)
	}

	// TODO: Check for max peers from a single IP.

	// Limit max number of total peers.
	if peers.Len() >= cfg.MaxPeers {
		log.Infof("SRVR: Max peers reached [%d] - disconnecting "+
			"peer %s", cfg.MaxPeers, p)
		p.Shutdown()
		// TODO(oga) how to handle permanent peers here?
		// they should be rescheduled.
		return false
	}

	// Add the new peer and start it.
	log.Debugf("SRVR: New peer %s", p)
	peers.PushBack(p)
	if p.inbound {
		p.Start()
	}

	return true
}

// handleDonePeerMsg deals with peers that have signalled they are done.  It is
// invoked from the peerHandler goroutine.
func (s *server) handleDonePeerMsg(peers *list.List, p *peer) bool {
	for e := peers.Front(); e != nil; e = e.Next() {
		if e.Value == p {
			// Issue an asynchronous reconnect if the peer was a
			// persistent outbound connection.
			if !p.inbound && p.persistent &&
				atomic.LoadInt32(&s.shutdown) == 0 {
				e.Value = newOutboundPeer(s, p.addr, true)
				return false
			}
			peers.Remove(e)
			log.Debugf("SRVR: Removed peer %s", p)
			return true
		}
	}
	log.Warnf("SRVR: Lost peer %v that we never had!", p)
	return false
}

// handleBanPeerMsg deals with banning peers.  It is invoked from the
// peerHandler goroutine.
func (s *server) handleBanPeerMsg(banned map[string]time.Time, p *peer) {
	host, _, err := net.SplitHostPort(p.addr)
	if err != nil {
		log.Debugf("SRVR: can't split ban peer %s %v", p.addr, err)
		return
	}
	direction := directionString(p.inbound)
	log.Infof("SRVR: Banned peer %s (%s) for %v", host, direction,
		cfg.BanDuration)
	banned[host] = time.Now().Add(cfg.BanDuration)

}

// handleRelayInvMsg deals with relaying inventory to peers that are not already
// known to have it.  It is invoked from the peerHandler goroutine.
func (s *server) handleRelayInvMsg(peers *list.List, iv *btcwire.InvVect) {
	// Loop through all connected peers and relay the inventory to those
	// which are not already known to have it.
	for e := peers.Front(); e != nil; e = e.Next() {
		p := e.Value.(*peer)
		if !p.Connected() {
			continue
		}

		// Queue the inventory to be relayed with the next batch.  It
		// will be ignored if the peer is already known to have the
		// inventory.
		p.QueueInventory(iv)
	}
}

// handleBroadcastMsg deals with broadcasting messages to peers.  It is invoked
// from the peerHandler goroutine.
func (s *server) handleBroadcastMsg(peers *list.List, bmsg *broadcastMsg) {
	for e := peers.Front(); e != nil; e = e.Next() {
		excluded := false
		for _, p := range bmsg.excludePeers {
			if e.Value == p {
				excluded = true
			}
		}
		p := e.Value.(*peer)
		// Don't broadcast to still connecting outbound peers .
		if !p.Connected() {
			excluded = true
		}
		if !excluded {
			p.QueueMessage(bmsg.message, nil)
		}
	}
}

// Peerinfo represents the information requested by the getpeerinfo rpc command.
type PeerInfo struct {
	Addr           string
	Services       btcwire.ServiceFlag
	LastSend       time.Time
	LastRecv       time.Time
	BytesSent      int
	BytesRecv      int
	ConnTime       time.Time
	Version        uint32
	SubVer         string
	Inbound        bool
	StartingHeight int32
	BanScore       int
	SyncNode       bool
}

type getConnCountMsg struct {
	reply chan int
}

type getPeerInfoMsg struct {
	reply chan []*PeerInfo
}

type addNodeMsg struct {
	addr      string
	permanent bool
	reply     chan error
}

type delNodeMsg struct {
	addr  string
	reply chan error
}

// handleQuery is the central handler for all queries and commands from other
// goroutines related to peer state.
func (s *server) handleQuery(querymsg interface{}, peers *list.List, bannedPeers map[string]time.Time) {
	switch msg := querymsg.(type) {
	case getConnCountMsg:
		nconnected := 0
		for e := peers.Front(); e != nil; e = e.Next() {
			peer := e.Value.(*peer)
			if peer.Connected() {
				nconnected++
			}
		}

		msg.reply <- nconnected
	case getPeerInfoMsg:
		infos := make([]*PeerInfo, 0, peers.Len())
		for e := peers.Front(); e != nil; e = e.Next() {
			peer := e.Value.(*peer)
			if !peer.Connected() {
				continue
			}
			// A lot of this will make the race detector go mad,
			// however it is statistics for purely informational purposes
			// and we don't really care if they are raced to get the new
			// version.
			info := &PeerInfo{
				Addr:           peer.addr,
				Services:       peer.services,
				LastSend:       peer.lastSend,
				LastRecv:       peer.lastRecv,
				BytesSent:      0, // TODO(oga) we need this from wire.
				BytesRecv:      0, // TODO(oga) we need this from wire.
				ConnTime:       peer.timeConnected,
				Version:        peer.protocolVersion,
				SubVer:         peer.userAgent,
				Inbound:        peer.inbound,
				StartingHeight: peer.lastBlock,
				BanScore:       0,
				SyncNode:       false, // TODO(oga) for now. bm knows this.
			}
			infos = append(infos, info)
		}
		msg.reply <- infos
	case addNodeMsg:
		// TODO(oga) really these checks only apply to permanent peers.
		for e := peers.Front(); e != nil; e = e.Next() {
			peer := e.Value.(*peer)
			if peer.addr == msg.addr {
				msg.reply <- errors.New("peer already connected")
				return
			}
		}
		// TODO(oga) if too many, nuke a non-perm peer.
		if s.handleAddPeerMsg(peers, bannedPeers,
			newOutboundPeer(s, msg.addr, msg.permanent)) {
			msg.reply <- nil
		} else {
			msg.reply <- errors.New("failed to add peer")
		}

	case delNodeMsg:
		found := false
		// TODO(oga) really these checks only apply to permanent peers.
		for e := peers.Front(); e != nil; e = e.Next() {
			peer := e.Value.(*peer)
			if peer.addr == msg.addr {
				peer.persistent = false // XXX hack!
				peer.Disconnect()
				found = true
				break
			}
		}

		if found {
			msg.reply <- nil
		} else {
			msg.reply <- errors.New("peer not found")
		}
	}
}

// listenHandler is the main listener which accepts incoming connections for the
// server.  It must be run as a goroutine.
func (s *server) listenHandler(listener net.Listener) {
	log.Infof("SRVR: Server listening on %s", listener.Addr())
	for atomic.LoadInt32(&s.shutdown) == 0 {
		conn, err := listener.Accept()
		if err != nil {
			// Only log the error if we're not forcibly shutting down.
			if atomic.LoadInt32(&s.shutdown) == 0 {
				log.Errorf("SRVR: can't accept connection: %v",
					err)
			}
			continue
		}
		s.AddPeer(newInboundPeer(s, conn))
	}
	s.wg.Done()
	log.Tracef("SRVR: Listener handler done for %s", listener.Addr())
}

// seedFromDNS uses DNS seeding to populate the address manager with peers.
func (s *server) seedFromDNS() {
	// Nothing to do if DNS seeding is disabled.
	if cfg.DisableDNSSeed {
		return
	}

	proxy := ""
	if cfg.Proxy != "" && cfg.UseTor {
		proxy = cfg.Proxy
	}
	for _, seeder := range activeNetParams.dnsSeeds {
		seedpeers := dnsDiscover(seeder, proxy)
		if len(seedpeers) == 0 {
			continue
		}
		addresses := make([]*btcwire.NetAddress, len(seedpeers))
		// if this errors then we have *real* problems
		intPort, _ := strconv.Atoi(activeNetParams.peerPort)
		for i, peer := range seedpeers {
			addresses[i] = new(btcwire.NetAddress)
			addresses[i].SetAddress(peer, uint16(intPort))
			// bitcoind seeds with addresses from
			// a time randomly selected between 3
			// and 7 days ago.
			addresses[i].Timestamp = time.Now().Add(-1 *
				time.Second * time.Duration(secondsIn3Days+
				s.addrManager.rand.Int31n(secondsIn4Days)))
		}

		// Bitcoind uses a lookup of the dns seeder here. This
		// is rather strange since the values looked up by the
		// DNS seed lookups will vary quite a lot.
		// to replicate this behaviour we put all addresses as
		// having come from the first one.
		s.addrManager.AddAddresses(addresses, addresses[0])
	}
	// XXX if this is empty do we want to use hardcoded
	// XXX peers like bitcoind does?
}

// peerHandler is used to handle peer operations such as adding and removing
// peers to and from the server, banning peers, and broadcasting messages to
// peers.  It must be run a a goroutine.
func (s *server) peerHandler() {
	// Start the address manager and block manager, both of which are needed
	// by peers.  This is done here since their lifecycle is closely tied
	// to this handler and rather than adding more channels to sychronize
	// things, it's easier and slightly faster to simply start and stop them
	// in this handler.
	s.addrManager.Start()
	s.blockManager.Start()

	log.Tracef("SRVR: Starting peer handler")
	peers := list.New()
	bannedPeers := make(map[string]time.Time)
	outboundPeers := 0
	maxOutbound := defaultMaxOutbound
	if cfg.MaxPeers < maxOutbound {
		maxOutbound = cfg.MaxPeers
	}

	// Add peers discovered through DNS to the address manager.
	s.seedFromDNS()

	// Start up persistent peers.
	permanentPeers := cfg.ConnectPeers
	if len(permanentPeers) == 0 {
		permanentPeers = cfg.AddPeers
	}
	for _, addr := range permanentPeers {
		if s.handleAddPeerMsg(peers, bannedPeers,
			newOutboundPeer(s, addr, true)) {
			outboundPeers++
		}
	}

	// if nothing else happens, wake us up soon.
	time.AfterFunc(10*time.Second, func() { s.wakeup <- true })

out:
	for {
		select {
		// New peers connected to the server.
		case p := <-s.newPeers:
			if s.handleAddPeerMsg(peers, bannedPeers, p) &&
				!p.inbound {
				outboundPeers++
			}

		// Disconnected peers.
		case p := <-s.donePeers:
			// handleDonePeerMsg return true if it removed a peer
			if s.handleDonePeerMsg(peers, p) {
				outboundPeers--
			}

		// Peer to ban.
		case p := <-s.banPeers:
			s.handleBanPeerMsg(bannedPeers, p)

		// New inventory to potentially be relayed to other peers.
		case invMsg := <-s.relayInv:
			s.handleRelayInvMsg(peers, invMsg)

		// Message to broadcast to all connected peers except those
		// which are excluded by the message.
		case bmsg := <-s.broadcast:
			s.handleBroadcastMsg(peers, &bmsg)

		// Used by timers below to wake us back up.
		case <-s.wakeup:
			// this page left intentionally blank

		case qmsg := <-s.query:
			s.handleQuery(qmsg, peers, bannedPeers)

		// Shutdown the peer handler.
		case <-s.quit:
			// Shutdown peers.
			for e := peers.Front(); e != nil; e = e.Next() {
				p := e.Value.(*peer)
				p.Shutdown()
			}
			break out
		}

		// Only try connect to more peers if we actually need more
		if outboundPeers >= maxOutbound || len(cfg.ConnectPeers) > 0 ||
			atomic.LoadInt32(&s.shutdown) != 0 {
			continue
		}
		groups := make(map[string]int)
		for e := peers.Front(); e != nil; e = e.Next() {
			peer := e.Value.(*peer)
			if !peer.inbound {
				groups[GroupKey(peer.na)]++
			}
		}

		tries := 0
		for outboundPeers < maxOutbound &&
			peers.Len() < cfg.MaxPeers &&
			atomic.LoadInt32(&s.shutdown) == 0 {
			// We bias like bitcoind does, 10 for no outgoing
			// up to 90 (8) for the selection of new vs tried
			//addresses.

			nPeers := outboundPeers
			if nPeers > 8 {
				nPeers = 8
			}
			addr := s.addrManager.GetAddress("any", 10+nPeers*10)
			if addr == nil {
				break
			}
			key := GroupKey(addr.na)
			// Address will not be invalid, local or unroutable
			// because addrmanager rejects those on addition.
			// Just check that we don't already have an address
			// in the same group so that we are not connecting
			// to the same network segment at the expense of
			// others. bitcoind breaks out of the loop here, but
			// we continue to try other addresses.
			if groups[key] != 0 {
				continue
			}

			tries++
			// After 100 bad tries exit the loop and we'll try again
			// later.
			if tries > 100 {
				break
			}

			// XXX if we have limited that address skip

			// only allow recent nodes (10mins) after we failed 30
			// times
			if time.Now().After(addr.lastattempt.Add(10*time.Minute)) &&
				tries < 30 {
				continue
			}

			// allow nondefault ports after 50 failed tries.
			if fmt.Sprintf("%d", addr.na.Port) !=
				activeNetParams.peerPort && tries < 50 {
				continue
			}

			addrStr := NetAddressKey(addr.na)

			tries = 0
			// any failure will be due to banned peers etc. we have
			// already checked that we have room for more peers.
			if s.handleAddPeerMsg(peers, bannedPeers,
				newOutboundPeer(s, addrStr, false)) {
				outboundPeers++
				groups[key]++
			}
		}

		// We we need more peers, wake up in ten seconds and try again.
		if outboundPeers < maxOutbound && peers.Len() < cfg.MaxPeers {
			time.AfterFunc(10*time.Second, func() {
				s.wakeup <- true
			})
		}
	}

	s.blockManager.Stop()
	s.addrManager.Stop()
	s.wg.Done()
	log.Tracef("SRVR: Peer handler done")
}

// AddPeer adds a new peer that has already been connected to the server.
func (s *server) AddPeer(p *peer) {
	s.newPeers <- p
}

// BanPeer bans a peer that has already been connected to the server by ip.
func (s *server) BanPeer(p *peer) {
	s.banPeers <- p
}

// RelayInventory relays the passed inventory to all connected peers that are
// not already known to have it.
func (s *server) RelayInventory(invVect *btcwire.InvVect) {
	s.relayInv <- invVect
}

// BroadcastMessage sends msg to all peers currently connected to the server
// except those in the passed peers to exclude.
func (s *server) BroadcastMessage(msg btcwire.Message, exclPeers ...*peer) {
	// XXX: Need to determine if this is an alert that has already been
	// broadcast and refrain from broadcasting again.
	bmsg := broadcastMsg{message: msg, excludePeers: exclPeers}
	s.broadcast <- bmsg
}

// ConnectedCount returns the number of currently connected peers.
func (s *server) ConnectedCount() int {
	replyChan := make(chan int)

	s.query <- getConnCountMsg{reply: replyChan}

	return <-replyChan
}

// PeerInfo returns an array of PeerInfo structures describing all connected
// peers.
func (s *server) PeerInfo() []*PeerInfo {
	replyChan := make(chan []*PeerInfo)

	s.query <- getPeerInfoMsg{reply: replyChan}

	return <-replyChan
}

// AddAddr adds `addr' as a new outbound peer. If permanent is true then the
// peer will be persistent and reconnect if the connection is lost.
// It is an error to call this with an already existing peer.
func (s *server) AddAddr(addr string, permanent bool) error {
	replyChan := make(chan error)

	s.query <- addNodeMsg{addr: addr, permanent: permanent, reply: replyChan}

	return <-replyChan
}

// RemoveAddr removes `addr' from the list of persistent peers if present.
// An error will be returned if the peer was not found.
func (s *server) RemoveAddr(addr string) error {
	replyChan := make(chan error)

	s.query <- delNodeMsg{addr: addr, reply: replyChan}

	return <-replyChan
}

// Start begins accepting connections from peers.
func (s *server) Start() {
	// Already started?
	if atomic.AddInt32(&s.started, 1) != 1 {
		return
	}

	log.Trace("SRVR: Starting server")

	// Start all the listeners.  There will not be any if listening is
	// disabled.
	for _, listener := range s.listeners {
		s.wg.Add(1)
		go s.listenHandler(listener)
	}

	// Start the peer handler which in turn starts the address and block
	// managers.
	s.wg.Add(1)
	go s.peerHandler()

	// Start the RPC server if it's not disabled.
	if !cfg.DisableRPC {
		s.rpcServer.Start()
	}
}

// Stop gracefully shuts down the server by stopping and disconnecting all
// peers and the main listener.
func (s *server) Stop() error {
	// Make sure this only happens once.
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		log.Infof("SRVR: Server is already in the process of shutting down")
		return nil
	}

	log.Warnf("SRVR: Server shutting down")

	// Stop all the listeners.  There will not be any listeners if
	// listening is disabled.
	for _, listener := range s.listeners {
		err := listener.Close()
		if err != nil {
			return err
		}
	}

	// Shutdown the RPC server if it's not disabled.
	if !cfg.DisableRPC {
		s.rpcServer.Stop()
	}

	// Signal the remaining goroutines to quit.
	close(s.quit)
	return nil
}

// WaitForShutdown blocks until the main listener and peer handlers are stopped.
func (s *server) WaitForShutdown() {
	s.wg.Wait()
	log.Infof("SRVR: Server shutdown complete")
}

// ScheduleShutdown schedules a server shutdown after the specified duration.
// It also dynamically adjusts how often to warn the server is going down based
// on remaining duration.
func (s *server) ScheduleShutdown(duration time.Duration) {
	// Don't schedule shutdown more than once.
	if atomic.AddInt32(&s.shutdownSched, 1) != 1 {
		return
	}
	log.Warnf("SRVR: Server shutdown in %v", duration)
	go func() {
		remaining := duration
		tickDuration := dynamicTickDuration(remaining)
		done := time.After(remaining)
		ticker := time.NewTicker(tickDuration)
	out:
		for {
			select {
			case <-done:
				ticker.Stop()
				s.Stop()
				break out
			case <-ticker.C:
				remaining = remaining - tickDuration
				if remaining < time.Second {
					continue
				}

				// Change tick duration dynamically based on remaining time.
				newDuration := dynamicTickDuration(remaining)
				if tickDuration != newDuration {
					tickDuration = newDuration
					ticker.Stop()
					ticker = time.NewTicker(tickDuration)
				}
				log.Warnf("SRVR: Server shutdown in %v", remaining)
			}
		}
	}()
}

// newServer returns a new btcd server configured to listen on addr for the
// bitcoin network type specified in btcnet.  Use start to begin accepting
// connections from peers.
func newServer(addr string, db btcdb.Db, btcnet btcwire.BitcoinNet) (*server, error) {
	nonce, err := btcwire.RandomUint64()
	if err != nil {
		return nil, err
	}

	var listeners []net.Listener
	if !cfg.DisableListen {
		// IPv4 listener.
		listener4, err := net.Listen("tcp4", addr)
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, listener4)

		// IPv6 listener.
		listener6, err := net.Listen("tcp6", addr)
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, listener6)
	}

	s := server{
		nonce:       nonce,
		listeners:   listeners,
		btcnet:      btcnet,
		addrManager: NewAddrManager(),
		newPeers:    make(chan *peer, cfg.MaxPeers),
		donePeers:   make(chan *peer, cfg.MaxPeers),
		banPeers:    make(chan *peer, cfg.MaxPeers),
		wakeup:      make(chan bool),
		query:       make(chan interface{}),
		relayInv:    make(chan *btcwire.InvVect, cfg.MaxPeers),
		broadcast:   make(chan broadcastMsg, cfg.MaxPeers),
		quit:        make(chan bool),
		db:          db,
	}
	bm, err := newBlockManager(&s)
	if err != nil {
		return nil, err
	}
	s.blockManager = bm
	s.txMemPool = newTxMemPool(&s)

	if !cfg.DisableRPC {
		s.rpcServer, err = newRPCServer(&s)
		if err != nil {
			return nil, err
		}
	}
	return &s, nil
}

// dynamicTickDuration is a convenience function used to dynamically choose a
// tick duration based on remaining time.  It is primarily used during
// server shutdown to make shutdown warnings more frequent as the shutdown time
// approaches.
func dynamicTickDuration(remaining time.Duration) time.Duration {
	switch {
	case remaining <= time.Second*5:
		return time.Second
	case remaining <= time.Second*15:
		return time.Second * 5
	case remaining <= time.Minute:
		return time.Second * 15
	case remaining <= time.Minute*5:
		return time.Minute
	case remaining <= time.Minute*15:
		return time.Minute * 5
	case remaining <= time.Hour:
		return time.Minute * 15
	}
	return time.Hour
}
