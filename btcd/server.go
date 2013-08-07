// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/list"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcwire"
	"net"
	"sync"
	"time"
)

// supportedServices describes which services are supported by the server.
const supportedServices = btcwire.SFNodeNetwork

// connectionRetryInterval is the amount of time to wait in between retries
// when connecting to persistent peers.
const connectionRetryInterval = time.Second * 10

// directionString is a helper function that returns a string that represents
// the direction of a connection (inbound or outbound).
func directionString(inbound bool) string {
	if inbound {
		return "inbound"
	}
	return "outbound"
}

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
	listener      net.Listener
	btcnet        btcwire.BitcoinNet
	started       bool
	shutdown      bool
	shutdownSched bool
	addrManager   *AddrManager
	rpcServer     *rpcServer
	blockManager  *blockManager
	newPeers      chan *peer
	donePeers     chan *peer
	banPeers      chan *peer
	broadcast     chan broadcastMsg
	wg            sync.WaitGroup
	quit          chan bool
	db            btcdb.Db
}

// handleAddPeerMsg deals with adding new peers.  It is invoked from the
// peerHandler goroutine.
func (s *server) handleAddPeerMsg(peers *list.List, banned map[string]time.Time, p *peer) {

	// Ignore new peers if we're shutting down.
	direction := directionString(p.inbound)
	if s.shutdown {
		log.Infof("[SRVR] New peer %s (%s) ignored - server is "+
			"shutting down", p.conn.RemoteAddr(), direction)
		p.Shutdown()
		return
	}

	// Disconnect banned peers.
	host, _, err := net.SplitHostPort(p.conn.RemoteAddr().String())
	if err != nil {
		log.Errorf("[SRVR] %v", err)
		p.Shutdown()
		return
	}
	if banEnd, ok := banned[host]; ok {
		if time.Now().Before(banEnd) {
			log.Debugf("[SRVR] Peer %s is banned for another %v - "+
				"disconnecting", host, banEnd.Sub(time.Now()))
			p.Shutdown()
			return
		}

		log.Infof("[SRVR] Peer %s is no longer banned", host)
		delete(banned, host)
	}

	// TODO: Check for max peers from a single IP.

	// Limit max number of total peers.
	if peers.Len() >= cfg.MaxPeers {
		log.Infof("[SRVR] Max peers reached [%d] - disconnecting "+
			"peer %s (%s)", cfg.MaxPeers, p.conn.RemoteAddr(),
			direction)
		p.Shutdown()
		return
	}

	// Add the new peer and start it.
	log.Infof("[SRVR] New peer %s (%s)", p.conn.RemoteAddr(), direction)
	peers.PushBack(p)
	p.Start()
}

// handleDonePeerMsg deals with peers that have signalled they are done.  It is
// invoked from the peerHandler goroutine.
func (s *server) handleDonePeerMsg(peers *list.List, p *peer) {
	direction := directionString(p.inbound)
	for e := peers.Front(); e != nil; e = e.Next() {
		if e.Value == p {
			peers.Remove(e)
			log.Infof("[SRVR] Removed peer %s (%s)",
				p.conn.RemoteAddr(), direction)

			// Issue an asynchronous reconnect if the peer was a
			// persistent outbound connection.
			if !p.inbound && p.persistent {
				addr := p.conn.RemoteAddr().String()
				s.ConnectPeerAsync(addr, true)
			}
			return
		}
	}
}

// handleBanPeerMsg deals with banning peers.  It is invoked from the
// peerHandler goroutine.
func (s *server) handleBanPeerMsg(banned map[string]time.Time, p *peer) {
	host, _, err := net.SplitHostPort(p.conn.RemoteAddr().String())
	if err != nil {
		log.Errorf("[SRVR] %v", err)
		return
	}
	direction := directionString(p.inbound)
	log.Infof("[SRVR] Banned peer %s (%s) for %v", host, direction,
		cfg.BanDuration)
	banned[host] = time.Now().Add(cfg.BanDuration)

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
		if !excluded {
			p := e.Value.(*peer)
			p.QueueMessage(bmsg.message)
		}
	}

}

// listenHandler is the main listener which accepts incoming connections for the
// server.  It must be run as a goroutine.
func (s *server) listenHandler() {
	log.Infof("[SRVR] Server listening on %s", s.listener.Addr())
	for !s.shutdown {
		conn, err := s.listener.Accept()
		if err != nil {
			// Only log the error if we're not forcibly shutting down.
			if !s.shutdown {
				log.Errorf("[SRVR] %v", err)
			}
			continue
		}
		s.AddPeer(newPeer(s, conn, true, false))
	}
	s.wg.Done()
	log.Tracef("[SRVR] Listener handler done for %s", s.listener.Addr())
}

// peerHandler is used to handle peer operations such as adding and removing
// peers to and from the server, banning peers, and broadcasting messages to
// peers.  It must be run a a goroutine.
func (s *server) peerHandler() {
	log.Tracef("[SRVR] Starting peer handler for %s", s.listener.Addr())
	peers := list.New()
	bannedPeers := make(map[string]time.Time)

	// Live while we're not shutting down or there are still connected peers.
	for !s.shutdown || peers.Len() != 0 {
		select {

		// New peers connected to the server.
		case p := <-s.newPeers:
			s.handleAddPeerMsg(peers, bannedPeers, p)

		// Disconnected peers.
		case p := <-s.donePeers:
			s.handleDonePeerMsg(peers, p)

		// Peer to ban.
		case p := <-s.banPeers:
			s.handleBanPeerMsg(bannedPeers, p)

		// Message to broadcast to all connected peers except those
		// which are excluded by the message.
		case bmsg := <-s.broadcast:
			s.handleBroadcastMsg(peers, &bmsg)

		// Shutdown the peer handler.
		case <-s.quit:
			// Shutdown peers.
			for e := peers.Front(); e != nil; e = e.Next() {
				p := e.Value.(*peer)
				p.Shutdown()
			}
		}
	}
	s.wg.Done()
	log.Tracef("[SRVR] Peer handler done on %s", s.listener.Addr())
}

// AddPeer adds a new peer that has already been connected to the server.
func (s *server) AddPeer(p *peer) {
	s.newPeers <- p
}

// BanPeer bans a peer that has already been connected to the server by ip.
func (s *server) BanPeer(p *peer) {
	s.banPeers <- p
}

// BroadcastMessage sends msg to all peers currently connected to the server
// except those in the passed peers to exclude.
func (s *server) BroadcastMessage(msg btcwire.Message, exclPeers ...*peer) {
	// XXX: Need to determine if this is an alert that has already been
	// broadcast and refrain from broadcasting again.
	bmsg := broadcastMsg{message: msg, excludePeers: exclPeers}
	s.broadcast <- bmsg
}

// ConnectPeerAsync attempts to asynchronously connect to addr.  If successful,
// a new peer is created and added to the server's peer list.
func (s *server) ConnectPeerAsync(addr string, persistent bool) {
	// Don't try to connect to a peer if the server is shutting down.
	if s.shutdown {
		return
	}

	go func() {
		// Attempt to connect to the peer.  If the connection fails and
		// this is a persistent connection, retry after the retry
		// interval.
		for !s.shutdown {
			log.Debugf("[SRVR] Attempting to connect to %s", addr)
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				log.Errorf("[SRVR] %v", err)
				if !persistent {
					return
				}
				log.Infof("[SRVR] Retrying connection to %s "+
					"in %s", addr, connectionRetryInterval)
				time.Sleep(connectionRetryInterval)
				continue
			}

			// Connection was successful so log it and create a new
			// peer.
			log.Infof("[SRVR] Connected to %s", conn.RemoteAddr())
			s.AddPeer(newPeer(s, conn, false, persistent))
			return
		}
	}()
}

// Start begins accepting connections from peers.
func (s *server) Start() {
	// Already started?
	if s.started {
		return
	}

	log.Trace("[SRVR] Starting server")
	go s.listenHandler()
	go s.peerHandler()
	s.wg.Add(2)
	s.addrManager.Start()
	s.blockManager.Start()
	if !cfg.DisableRpc {
		s.rpcServer.Start()
	}

	s.started = true
}

// Stop gracefully shuts down the server by stopping and disconnecting all
// peers and the main listener.
func (s *server) Stop() error {
	if s.shutdown {
		log.Infof("[SRVR] Server is already in the process of shutting down")
		return nil
	}

	log.Warnf("[SRVR] Server shutting down")
	s.shutdown = true
	s.quit <- true
	if !cfg.DisableRpc {
		s.rpcServer.Stop()
	}
	s.addrManager.Stop()
	s.blockManager.Stop()
	err := s.listener.Close()
	if err != nil {
		return err
	}
	return nil
}

// WaitForShutdown blocks until the main listener and peer handlers are stopped.
func (s *server) WaitForShutdown() {
	s.wg.Wait()
	log.Infof("[SRVR] Server shutdown complete")
}

// ScheduleShutdown schedules a server shutdown after the specified duration.
// It also dynamically adjusts how often to warn the server is going down based
// on remaining duration.
func (s *server) ScheduleShutdown(duration time.Duration) {
	// Don't schedule shutdown more than once.
	if s.shutdownSched {
		return
	}
	log.Warnf("[SRVR] Server shutdown in %v", duration)
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
				log.Warnf("[SRVR] Server shutdown in %v", remaining)
			}
		}
	}()
	s.shutdownSched = true
}

// newServer returns a new btcd server configured to listen on addr for the
// bitcoin network type specified in btcnet.  Use start to begin accepting
// connections from peers.
func newServer(addr string, db btcdb.Db, btcnet btcwire.BitcoinNet) (*server, error) {
	nonce, err := btcwire.RandomUint64()
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	s := server{
		nonce:       nonce,
		listener:    listener,
		btcnet:      btcnet,
		addrManager: NewAddrManager(),
		newPeers:    make(chan *peer, cfg.MaxPeers),
		donePeers:   make(chan *peer, cfg.MaxPeers),
		banPeers:    make(chan *peer, cfg.MaxPeers),
		broadcast:   make(chan broadcastMsg, cfg.MaxPeers),
		quit:        make(chan bool),
		db:          db,
	}
	s.blockManager = newBlockManager(&s)

	log.Infof("[BMGR] Generating initial block node index.  This may " +
		"take a while...")
	err = s.blockManager.blockChain.GenerateInitialIndex()
	if err != nil {
		return nil, err
	}
	log.Infof("[BMGR] Block index generation complete")

	if !cfg.DisableRpc {
		s.rpcServer, err = newRpcServer(&s)
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
