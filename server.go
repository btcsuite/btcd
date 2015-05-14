// Copyright (c) 2013-2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	mrand "math/rand"
	"net"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/addrmgr"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

const (
	// These constants are used by the DNS seed code to pick a random last seen
	// time.
	secondsIn3Days int32 = 24 * 60 * 60 * 3
	secondsIn4Days int32 = 24 * 60 * 60 * 4
)

const (
	// supportedServices describes which services are supported by the
	// server.
	supportedServices = wire.SFNodeNetwork

	// connectionRetryInterval is the amount of time to wait in between
	// retries when connecting to persistent peers.
	connectionRetryInterval = time.Second * 10

	// defaultMaxOutbound is the default number of max outbound peers.
	defaultMaxOutbound = 8
)

// broadcastMsg provides the ability to house a bitcoin message to be broadcast
// to all connected peers except specified excluded peers.
type broadcastMsg struct {
	message      wire.Message
	excludePeers []*peer
}

// broadcastInventoryAdd is a type used to declare that the InvVect it contains
// needs to be added to the rebroadcast map
type broadcastInventoryAdd relayMsg

// broadcastInventoryDel is a type used to declare that the InvVect it contains
// needs to be removed from the rebroadcast map
type broadcastInventoryDel *wire.InvVect

// relayMsg packages an inventory vector along with the newly discovered
// inventory so the relay has access to that information.
type relayMsg struct {
	invVect *wire.InvVect
	data    interface{}
}

// updatePeerHeightsMsg is a message sent from the blockmanager to the server
// after a new block has been accepted. The purpose of the message is to update
// the heights of peers that were known to announce the block before we
// connected it to the main chain or recognized it as an orphan. With these
// updates, peer heights will be kept up to date, allowing for fresh data when
// selecting sync peer candidacy.
type updatePeerHeightsMsg struct {
	newSha     *wire.ShaHash
	newHeight  int32
	originPeer *peer
}

// server provides a bitcoin server for handling communications to and from
// bitcoin peers.
type server struct {
	nonce                uint64
	listeners            []net.Listener
	chainParams          *chaincfg.Params
	started              int32      // atomic
	shutdown             int32      // atomic
	shutdownSched        int32      // atomic
	bytesMutex           sync.Mutex // For the following two fields.
	bytesReceived        uint64     // Total bytes received from all peers since start.
	bytesSent            uint64     // Total bytes sent by all peers since start.
	addrManager          *addrmgr.AddrManager
	rpcServer            *rpcServer
	blockManager         *blockManager
	addrIndexer          *addrIndexer
	txMemPool            *txMemPool
	cpuMiner             *CPUMiner
	modifyRebroadcastInv chan interface{}
	newPeers             chan *peer
	donePeers            chan *peer
	banPeers             chan *peer
	wakeup               chan struct{}
	query                chan interface{}
	relayInv             chan relayMsg
	broadcast            chan broadcastMsg
	peerHeightsUpdate    chan updatePeerHeightsMsg
	wg                   sync.WaitGroup
	quit                 chan struct{}
	nat                  NAT
	db                   database.Db
	timeSource           blockchain.MedianTimeSource
}

type peerState struct {
	peers            map[*peer]struct{}
	outboundPeers    map[*peer]struct{}
	persistentPeers  map[*peer]struct{}
	banned           map[string]time.Time
	outboundGroups   map[string]int
	maxOutboundPeers int
}

// randomUint16Number returns a random uint16 in a specified input range.  Note
// that the range is in zeroth ordering; if you pass it 1800, you will get
// values from 0 to 1800.
func randomUint16Number(max uint16) uint16 {
	// In order to avoid modulo bias and ensure every possible outcome in
	// [0, max) has equal probability, the random number must be sampled
	// from a random source that has a range limited to a multiple of the
	// modulus.
	var randomNumber uint16
	var limitRange = (math.MaxUint16 / max) * max
	for {
		binary.Read(rand.Reader, binary.LittleEndian, &randomNumber)
		if randomNumber < limitRange {
			return (randomNumber % max)
		}
	}
}

// AddRebroadcastInventory adds 'iv' to the list of inventories to be
// rebroadcasted at random intervals until they show up in a block.
func (s *server) AddRebroadcastInventory(iv *wire.InvVect, data interface{}) {
	// Ignore if shutting down.
	if atomic.LoadInt32(&s.shutdown) != 0 {
		return
	}

	s.modifyRebroadcastInv <- broadcastInventoryAdd{invVect: iv, data: data}
}

// RemoveRebroadcastInventory removes 'iv' from the list of items to be
// rebroadcasted if present.
func (s *server) RemoveRebroadcastInventory(iv *wire.InvVect) {
	// Ignore if shutting down.
	if atomic.LoadInt32(&s.shutdown) != 0 {
		return
	}

	s.modifyRebroadcastInv <- broadcastInventoryDel(iv)
}

func (p *peerState) Count() int {
	return len(p.peers) + len(p.outboundPeers) + len(p.persistentPeers)
}

func (p *peerState) OutboundCount() int {
	return len(p.outboundPeers) + len(p.persistentPeers)
}

func (p *peerState) NeedMoreOutbound() bool {
	return p.OutboundCount() < p.maxOutboundPeers &&
		p.Count() < cfg.MaxPeers
}

// forAllOutboundPeers is a helper function that runs closure on all outbound
// peers known to peerState.
func (p *peerState) forAllOutboundPeers(closure func(p *peer)) {
	for e := range p.outboundPeers {
		closure(e)
	}
	for e := range p.persistentPeers {
		closure(e)
	}
}

// forAllPeers is a helper function that runs closure on all peers known to
// peerState.
func (p *peerState) forAllPeers(closure func(p *peer)) {
	for e := range p.peers {
		closure(e)
	}
	p.forAllOutboundPeers(closure)
}

// handleUpdatePeerHeight updates the heights of all peers who were known to
// announce a block we recently accepted.
func (s *server) handleUpdatePeerHeights(state *peerState, umsg updatePeerHeightsMsg) {
	state.forAllPeers(func(p *peer) {
		// The origin peer should already have the updated height.
		if p == umsg.originPeer {
			return
		}

		// Skip this peer if it hasn't recently announced any new blocks.
		p.StatsMtx.Lock()
		if p.lastAnnouncedBlock == nil {
			p.StatsMtx.Unlock()
			return
		}

		// This is a pointer to the underlying memory which doesn't
		// change.
		latestBlkSha := p.lastAnnouncedBlock
		p.StatsMtx.Unlock()

		// If the peer has recently announced a block, and this block
		// matches our newly accepted block, then update their block
		// height.
		if *latestBlkSha == *umsg.newSha {
			p.UpdateLastBlockHeight(umsg.newHeight)
			p.UpdateLastAnnouncedBlock(nil)
		}
	})
}

// handleAddPeerMsg deals with adding new peers.  It is invoked from the
// peerHandler goroutine.
func (s *server) handleAddPeerMsg(state *peerState, p *peer) bool {
	if p == nil {
		return false
	}

	// Ignore new peers if we're shutting down.
	if atomic.LoadInt32(&s.shutdown) != 0 {
		srvrLog.Infof("New peer %s ignored - server is shutting "+
			"down", p)
		p.Shutdown()
		return false
	}

	// Disconnect banned peers.
	host, _, err := net.SplitHostPort(p.addr)
	if err != nil {
		srvrLog.Debugf("can't split hostport %v", err)
		p.Shutdown()
		return false
	}
	if banEnd, ok := state.banned[host]; ok {
		if time.Now().Before(banEnd) {
			srvrLog.Debugf("Peer %s is banned for another %v - "+
				"disconnecting", host, banEnd.Sub(time.Now()))
			p.Shutdown()
			return false
		}

		srvrLog.Infof("Peer %s is no longer banned", host)
		delete(state.banned, host)
	}

	// TODO: Check for max peers from a single IP.

	// Limit max number of total peers.
	if state.Count() >= cfg.MaxPeers {
		srvrLog.Infof("Max peers reached [%d] - disconnecting "+
			"peer %s", cfg.MaxPeers, p)
		p.Shutdown()
		// TODO(oga) how to handle permanent peers here?
		// they should be rescheduled.
		return false
	}

	// Add the new peer and start it.
	srvrLog.Debugf("New peer %s", p)
	if p.inbound {
		state.peers[p] = struct{}{}
		p.Start()
	} else {
		state.outboundGroups[addrmgr.GroupKey(p.na)]++
		if p.persistent {
			state.persistentPeers[p] = struct{}{}
		} else {
			state.outboundPeers[p] = struct{}{}
		}
	}

	return true
}

// handleDonePeerMsg deals with peers that have signalled they are done.  It is
// invoked from the peerHandler goroutine.
func (s *server) handleDonePeerMsg(state *peerState, p *peer) {
	var list map[*peer]struct{}
	if p.persistent {
		list = state.persistentPeers
	} else if p.inbound {
		list = state.peers
	} else {
		list = state.outboundPeers
	}
	for e := range list {
		if e == p {
			// Issue an asynchronous reconnect if the peer was a
			// persistent outbound connection.
			if !p.inbound && p.persistent && atomic.LoadInt32(&s.shutdown) == 0 {
				e = newOutboundPeer(s, p.addr, true, p.retryCount+1)
				return
			}
			if !p.inbound {
				state.outboundGroups[addrmgr.GroupKey(p.na)]--
			}
			delete(list, e)
			srvrLog.Debugf("Removed peer %s", p)
			return
		}
	}
	// If we get here it means that either we didn't know about the peer
	// or we purposefully deleted it.
}

// handleBanPeerMsg deals with banning peers.  It is invoked from the
// peerHandler goroutine.
func (s *server) handleBanPeerMsg(state *peerState, p *peer) {
	host, _, err := net.SplitHostPort(p.addr)
	if err != nil {
		srvrLog.Debugf("can't split ban peer %s %v", p.addr, err)
		return
	}
	direction := directionString(p.inbound)
	srvrLog.Infof("Banned peer %s (%s) for %v", host, direction,
		cfg.BanDuration)
	state.banned[host] = time.Now().Add(cfg.BanDuration)

}

// handleRelayInvMsg deals with relaying inventory to peers that are not already
// known to have it.  It is invoked from the peerHandler goroutine.
func (s *server) handleRelayInvMsg(state *peerState, msg relayMsg) {
	state.forAllPeers(func(p *peer) {
		if !p.Connected() {
			return
		}

		if msg.invVect.Type == wire.InvTypeTx {
			// Don't relay the transaction to the peer when it has
			// transaction relaying disabled.
			if p.RelayTxDisabled() {
				return
			}

			// Don't relay the transaction if there is a bloom
			// filter loaded and the transaction doesn't match it.
			if p.filter.IsLoaded() {
				tx, ok := msg.data.(*btcutil.Tx)
				if !ok {
					peerLog.Warnf("Underlying data for tx" +
						" inv relay is not a transaction")
					return
				}

				if !p.filter.MatchTxAndUpdate(tx) {
					return
				}
			}
		}

		// Queue the inventory to be relayed with the next batch.
		// It will be ignored if the peer is already known to
		// have the inventory.
		p.QueueInventory(msg.invVect)
	})
}

// handleBroadcastMsg deals with broadcasting messages to peers.  It is invoked
// from the peerHandler goroutine.
func (s *server) handleBroadcastMsg(state *peerState, bmsg *broadcastMsg) {
	state.forAllPeers(func(p *peer) {
		excluded := false
		for _, ep := range bmsg.excludePeers {
			if p == ep {
				excluded = true
			}
		}
		// Don't broadcast to still connecting outbound peers .
		if !p.Connected() {
			excluded = true
		}
		if !excluded {
			p.QueueMessage(bmsg.message, nil)
		}
	})
}

type getConnCountMsg struct {
	reply chan int32
}

type getPeerInfoMsg struct {
	reply chan []*btcjson.GetPeerInfoResult
}

type getAddedNodesMsg struct {
	reply chan []*peer
}

type disconnectNodeMsg struct {
	cmp   func(*peer) bool
	reply chan error
}

type connectNodeMsg struct {
	addr      string
	permanent bool
	reply     chan error
}

type removeNodeMsg struct {
	cmp   func(*peer) bool
	reply chan error
}

// handleQuery is the central handler for all queries and commands from other
// goroutines related to peer state.
func (s *server) handleQuery(querymsg interface{}, state *peerState) {
	switch msg := querymsg.(type) {
	case getConnCountMsg:
		nconnected := int32(0)
		state.forAllPeers(func(p *peer) {
			if p.Connected() {
				nconnected++
			}
		})
		msg.reply <- nconnected

	case getPeerInfoMsg:
		syncPeer := s.blockManager.SyncPeer()
		infos := make([]*btcjson.GetPeerInfoResult, 0, len(state.peers))
		state.forAllPeers(func(p *peer) {
			if !p.Connected() {
				return
			}

			// A lot of this will make the race detector go mad,
			// however it is statistics for purely informational purposes
			// and we don't really care if they are raced to get the new
			// version.
			p.StatsMtx.Lock()
			info := &btcjson.GetPeerInfoResult{
				ID:             p.id,
				Addr:           p.addr,
				Services:       fmt.Sprintf("%08d", p.services),
				LastSend:       p.lastSend.Unix(),
				LastRecv:       p.lastRecv.Unix(),
				BytesSent:      p.bytesSent,
				BytesRecv:      p.bytesReceived,
				ConnTime:       p.timeConnected.Unix(),
				TimeOffset:     p.timeOffset,
				Version:        p.protocolVersion,
				SubVer:         p.userAgent,
				Inbound:        p.inbound,
				StartingHeight: p.startingHeight,
				CurrentHeight:  p.lastBlock,
				BanScore:       0,
				SyncNode:       p == syncPeer,
			}
			info.PingTime = float64(p.lastPingMicros)
			if p.lastPingNonce != 0 {
				wait := float64(time.Now().Sub(p.lastPingTime).Nanoseconds())
				// We actually want microseconds.
				info.PingWait = wait / 1000
			}
			p.StatsMtx.Unlock()
			infos = append(infos, info)
		})
		msg.reply <- infos

	case connectNodeMsg:
		// XXX(oga) duplicate oneshots?
		for peer := range state.persistentPeers {
			if peer.addr == msg.addr {
				if msg.permanent {
					msg.reply <- errors.New("peer already connected")
				} else {
					msg.reply <- errors.New("peer exists as a permanent peer")
				}
				return
			}
		}

		// TODO(oga) if too many, nuke a non-perm peer.
		if s.handleAddPeerMsg(state,
			newOutboundPeer(s, msg.addr, msg.permanent, 0)) {
			msg.reply <- nil
		} else {
			msg.reply <- errors.New("failed to add peer")
		}
	case removeNodeMsg:
		found := disconnectPeer(state.persistentPeers, msg.cmp, func(p *peer) {
			// Keep group counts ok since we remove from
			// the list now.
			state.outboundGroups[addrmgr.GroupKey(p.na)]--
		})

		if found {
			msg.reply <- nil
		} else {
			msg.reply <- errors.New("peer not found")
		}
	// Request a list of the persistent (added) peers.
	case getAddedNodesMsg:
		// Respond with a slice of the relavent peers.
		peers := make([]*peer, 0, len(state.persistentPeers))
		for peer := range state.persistentPeers {
			peers = append(peers, peer)
		}
		msg.reply <- peers
	case disconnectNodeMsg:
		// Check inbound peers. We pass a nil callback since we don't
		// require any additional actions on disconnect for inbound peers.
		found := disconnectPeer(state.peers, msg.cmp, nil)
		if found {
			msg.reply <- nil
			return
		}

		// Check outbound peers.
		found = disconnectPeer(state.outboundPeers, msg.cmp, func(p *peer) {
			// Keep group counts ok since we remove from
			// the list now.
			state.outboundGroups[addrmgr.GroupKey(p.na)]--
		})
		if found {
			// If there are multiple outbound connections to the same
			// ip:port, continue disconnecting them all until no such
			// peers are found.
			for found {
				found = disconnectPeer(state.outboundPeers, msg.cmp, func(p *peer) {
					state.outboundGroups[addrmgr.GroupKey(p.na)]--
				})
			}
			msg.reply <- nil
			return
		}

		msg.reply <- errors.New("peer not found")
	}
}

// disconnectPeer attempts to drop the connection of a tageted peer in the
// passed peer list. Targets are identified via usage of the passed
// `compareFunc`, which should return `true` if the passed peer is the target
// peer. This function returns true on success and false if the peer is unable
// to be located. If the peer is found, and the passed callback: `whenFound'
// isn't nil, we call it with the peer as the argument before it is removed
// from the peerList, and is disconnected from the server.
func disconnectPeer(peerList map[*peer]struct{}, compareFunc func(*peer) bool, whenFound func(*peer)) bool {
	for peer := range peerList {
		if compareFunc(peer) {
			if whenFound != nil {
				whenFound(peer)
			}

			// This is ok because we are not continuing
			// to iterate so won't corrupt the loop.
			delete(peerList, peer)
			peer.Disconnect()
			return true
		}
	}
	return false
}

// listenHandler is the main listener which accepts incoming connections for the
// server.  It must be run as a goroutine.
func (s *server) listenHandler(listener net.Listener) {
	srvrLog.Infof("Server listening on %s", listener.Addr())
	for atomic.LoadInt32(&s.shutdown) == 0 {
		conn, err := listener.Accept()
		if err != nil {
			// Only log the error if we're not forcibly shutting down.
			if atomic.LoadInt32(&s.shutdown) == 0 {
				srvrLog.Errorf("can't accept connection: %v",
					err)
			}
			continue
		}
		s.AddPeer(newInboundPeer(s, conn))
	}
	s.wg.Done()
	srvrLog.Tracef("Listener handler done for %s", listener.Addr())
}

// seedFromDNS uses DNS seeding to populate the address manager with peers.
func (s *server) seedFromDNS() {
	// Nothing to do if DNS seeding is disabled.
	if cfg.DisableDNSSeed {
		return
	}

	for _, seeder := range activeNetParams.dnsSeeds {
		go func(seeder string) {
			randSource := mrand.New(mrand.NewSource(time.Now().UnixNano()))

			seedpeers, err := dnsDiscover(seeder)
			if err != nil {
				discLog.Infof("DNS discovery failed on seed %s: %v", seeder, err)
				return
			}
			numPeers := len(seedpeers)

			discLog.Infof("%d addresses found from DNS seed %s", numPeers, seeder)

			if numPeers == 0 {
				return
			}
			addresses := make([]*wire.NetAddress, len(seedpeers))
			// if this errors then we have *real* problems
			intPort, _ := strconv.Atoi(activeNetParams.DefaultPort)
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
			s.addrManager.AddAddresses(addresses, addresses[0])
		}(seeder)
	}
}

// peerHandler is used to handle peer operations such as adding and removing
// peers to and from the server, banning peers, and broadcasting messages to
// peers.  It must be run in a goroutine.
func (s *server) peerHandler() {
	// Start the address manager and block manager, both of which are needed
	// by peers.  This is done here since their lifecycle is closely tied
	// to this handler and rather than adding more channels to sychronize
	// things, it's easier and slightly faster to simply start and stop them
	// in this handler.
	s.addrManager.Start()
	s.blockManager.Start()

	srvrLog.Tracef("Starting peer handler")
	state := &peerState{
		peers:            make(map[*peer]struct{}),
		persistentPeers:  make(map[*peer]struct{}),
		outboundPeers:    make(map[*peer]struct{}),
		banned:           make(map[string]time.Time),
		maxOutboundPeers: defaultMaxOutbound,
		outboundGroups:   make(map[string]int),
	}
	if cfg.MaxPeers < state.maxOutboundPeers {
		state.maxOutboundPeers = cfg.MaxPeers
	}

	// Add peers discovered through DNS to the address manager.
	s.seedFromDNS()

	// Start up persistent peers.
	permanentPeers := cfg.ConnectPeers
	if len(permanentPeers) == 0 {
		permanentPeers = cfg.AddPeers
	}
	for _, addr := range permanentPeers {
		s.handleAddPeerMsg(state, newOutboundPeer(s, addr, true, 0))
	}

	// if nothing else happens, wake us up soon.
	time.AfterFunc(10*time.Second, func() { s.wakeup <- struct{}{} })

out:
	for {
		select {
		// New peers connected to the server.
		case p := <-s.newPeers:
			s.handleAddPeerMsg(state, p)

		// Disconnected peers.
		case p := <-s.donePeers:
			s.handleDonePeerMsg(state, p)

		// Block accepted in mainchain or orphan, update peer height.
		case umsg := <-s.peerHeightsUpdate:
			s.handleUpdatePeerHeights(state, umsg)

		// Peer to ban.
		case p := <-s.banPeers:
			s.handleBanPeerMsg(state, p)

		// New inventory to potentially be relayed to other peers.
		case invMsg := <-s.relayInv:
			s.handleRelayInvMsg(state, invMsg)

		// Message to broadcast to all connected peers except those
		// which are excluded by the message.
		case bmsg := <-s.broadcast:
			s.handleBroadcastMsg(state, &bmsg)

		// Used by timers below to wake us back up.
		case <-s.wakeup:
			// this page left intentionally blank

		case qmsg := <-s.query:
			s.handleQuery(qmsg, state)

		// Shutdown the peer handler.
		case <-s.quit:
			// Shutdown peers.
			state.forAllPeers(func(p *peer) {
				p.Shutdown()
			})
			break out
		}

		// Don't try to connect to more peers when running on the
		// simulation test network.  The simulation network is only
		// intended to connect to specified peers and actively avoid
		// advertising and connecting to discovered peers.
		if cfg.SimNet {
			continue
		}

		// Only try connect to more peers if we actually need more.
		if !state.NeedMoreOutbound() || len(cfg.ConnectPeers) > 0 ||
			atomic.LoadInt32(&s.shutdown) != 0 {
			continue
		}
		tries := 0
		for state.NeedMoreOutbound() &&
			atomic.LoadInt32(&s.shutdown) == 0 {
			nPeers := state.OutboundCount()
			if nPeers > 8 {
				nPeers = 8
			}
			addr := s.addrManager.GetAddress("any")
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
			if state.outboundGroups[key] != 0 {
				break
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
			if time.Now().After(addr.LastAttempt().Add(10*time.Minute)) &&
				tries < 30 {
				continue
			}

			// allow nondefault ports after 50 failed tries.
			if fmt.Sprintf("%d", addr.NetAddress().Port) !=
				activeNetParams.DefaultPort && tries < 50 {
				continue
			}

			addrStr := addrmgr.NetAddressKey(addr.NetAddress())

			tries = 0
			// any failure will be due to banned peers etc. we have
			// already checked that we have room for more peers.
			if s.handleAddPeerMsg(state,
				newOutboundPeer(s, addrStr, false, 0)) {
			}
		}

		// We need more peers, wake up in ten seconds and try again.
		if state.NeedMoreOutbound() {
			time.AfterFunc(10*time.Second, func() {
				s.wakeup <- struct{}{}
			})
		}
	}

	if cfg.AddrIndex {
		s.addrIndexer.Stop()
	}
	s.blockManager.Stop()
	s.addrManager.Stop()
	s.wg.Done()
	srvrLog.Tracef("Peer handler done")
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
func (s *server) RelayInventory(invVect *wire.InvVect, data interface{}) {
	s.relayInv <- relayMsg{invVect: invVect, data: data}
}

// BroadcastMessage sends msg to all peers currently connected to the server
// except those in the passed peers to exclude.
func (s *server) BroadcastMessage(msg wire.Message, exclPeers ...*peer) {
	// XXX: Need to determine if this is an alert that has already been
	// broadcast and refrain from broadcasting again.
	bmsg := broadcastMsg{message: msg, excludePeers: exclPeers}
	s.broadcast <- bmsg
}

// ConnectedCount returns the number of currently connected peers.
func (s *server) ConnectedCount() int32 {
	replyChan := make(chan int32)

	s.query <- getConnCountMsg{reply: replyChan}

	return <-replyChan
}

// AddedNodeInfo returns an array of btcjson.GetAddedNodeInfoResult structures
// describing the persistent (added) nodes.
func (s *server) AddedNodeInfo() []*peer {
	replyChan := make(chan []*peer)
	s.query <- getAddedNodesMsg{reply: replyChan}
	return <-replyChan
}

// PeerInfo returns an array of PeerInfo structures describing all connected
// peers.
func (s *server) PeerInfo() []*btcjson.GetPeerInfoResult {
	replyChan := make(chan []*btcjson.GetPeerInfoResult)

	s.query <- getPeerInfoMsg{reply: replyChan}

	return <-replyChan
}

// DisconnectNodeByAddr disconnects a peer by target address. Both outbound and
// inbound nodes will be searched for the target node. An error message will
// be returned if the peer was not found.
func (s *server) DisconnectNodeByAddr(addr string) error {
	replyChan := make(chan error)

	s.query <- disconnectNodeMsg{
		cmp:   func(p *peer) bool { return p.addr == addr },
		reply: replyChan,
	}

	return <-replyChan
}

// DisconnectNodeByID disconnects a peer by target node id. Both outbound and
// inbound nodes will be searched for the target node. An error message will be
// returned if the peer was not found.
func (s *server) DisconnectNodeById(id int32) error {
	replyChan := make(chan error)

	s.query <- disconnectNodeMsg{
		cmp:   func(p *peer) bool { return p.id == id },
		reply: replyChan,
	}

	return <-replyChan
}

// RemoveNodeByAddr removes a peer from the list of persistent peers if
// present. An error will be returned if the peer was not found.
func (s *server) RemoveNodeByAddr(addr string) error {
	replyChan := make(chan error)

	s.query <- removeNodeMsg{
		cmp:   func(p *peer) bool { return p.addr == addr },
		reply: replyChan,
	}

	return <-replyChan
}

// RemoveNodeById removes a peer by node ID from the list of persistent peers
// if present. An error will be returned if the peer was not found.
func (s *server) RemoveNodeById(id int32) error {
	replyChan := make(chan error)

	s.query <- removeNodeMsg{
		cmp:   func(p *peer) bool { return p.id == id },
		reply: replyChan,
	}

	return <-replyChan
}

// ConnectNode adds `addr' as a new outbound peer. If permanent is true then the
// peer will be persistent and reconnect if the connection is lost.
// It is an error to call this with an already existing peer.
func (s *server) ConnectNode(addr string, permanent bool) error {
	replyChan := make(chan error)

	s.query <- connectNodeMsg{addr: addr, permanent: permanent, reply: replyChan}

	return <-replyChan
}

// AddBytesSent adds the passed number of bytes to the total bytes sent counter
// for the server.  It is safe for concurrent access.
func (s *server) AddBytesSent(bytesSent uint64) {
	s.bytesMutex.Lock()
	defer s.bytesMutex.Unlock()

	s.bytesSent += bytesSent
}

// AddBytesReceived adds the passed number of bytes to the total bytes received
// counter for the server.  It is safe for concurrent access.
func (s *server) AddBytesReceived(bytesReceived uint64) {
	s.bytesMutex.Lock()
	defer s.bytesMutex.Unlock()

	s.bytesReceived += bytesReceived
}

// NetTotals returns the sum of all bytes received and sent across the network
// for all peers.  It is safe for concurrent access.
func (s *server) NetTotals() (uint64, uint64) {
	s.bytesMutex.Lock()
	defer s.bytesMutex.Unlock()

	return s.bytesReceived, s.bytesSent
}

// UpdatePeerHeights updates the heights of all peers who have have announced
// the latest connected main chain block, or a recognized orphan. These height
// updates allow us to dynamically refresh peer heights, ensuring sync peer
// selection has access to the latest block heights for each peer.
func (s *server) UpdatePeerHeights(latestBlkSha *wire.ShaHash, latestHeight int32, updateSource *peer) {
	s.peerHeightsUpdate <- updatePeerHeightsMsg{
		newSha:     latestBlkSha,
		newHeight:  latestHeight,
		originPeer: updateSource,
	}
}

// rebroadcastHandler keeps track of user submitted inventories that we have
// sent out but have not yet made it into a block. We periodically rebroadcast
// them in case our peers restarted or otherwise lost track of them.
func (s *server) rebroadcastHandler() {
	// Wait 5 min before first tx rebroadcast.
	timer := time.NewTimer(5 * time.Minute)
	pendingInvs := make(map[wire.InvVect]interface{})

out:
	for {
		select {
		case riv := <-s.modifyRebroadcastInv:
			switch msg := riv.(type) {
			// Incoming InvVects are added to our map of RPC txs.
			case broadcastInventoryAdd:
				pendingInvs[*msg.invVect] = msg.data

			// When an InvVect has been added to a block, we can
			// now remove it, if it was present.
			case broadcastInventoryDel:
				if _, ok := pendingInvs[*msg]; ok {
					delete(pendingInvs, *msg)
				}
			}

		case <-timer.C:
			// Any inventory we have has not made it into a block
			// yet. We periodically resubmit them until they have.
			for iv, data := range pendingInvs {
				ivCopy := iv
				s.RelayInventory(&ivCopy, data)
			}

			// Process at a random time up to 30mins (in seconds)
			// in the future.
			timer.Reset(time.Second *
				time.Duration(randomUint16Number(1800)))

		case <-s.quit:
			break out
		}
	}

	timer.Stop()

	// Drain channels before exiting so nothing is left waiting around
	// to send.
cleanup:
	for {
		select {
		case <-s.modifyRebroadcastInv:
		default:
			break cleanup
		}
	}
	s.wg.Done()
}

// Start begins accepting connections from peers.
func (s *server) Start() {
	// Already started?
	if atomic.AddInt32(&s.started, 1) != 1 {
		return
	}

	srvrLog.Trace("Starting server")

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

	if s.nat != nil {
		s.wg.Add(1)
		go s.upnpUpdateThread()
	}

	if !cfg.DisableRPC {
		s.wg.Add(1)

		// Start the rebroadcastHandler, which ensures user tx received by
		// the RPC server are rebroadcast until being included in a block.
		go s.rebroadcastHandler()

		s.rpcServer.Start()
	}

	// Start the CPU miner if generation is enabled.
	if cfg.Generate {
		s.cpuMiner.Start()
	}

	if cfg.AddrIndex {
		s.addrIndexer.Start()
	}
}

// Stop gracefully shuts down the server by stopping and disconnecting all
// peers and the main listener.
func (s *server) Stop() error {
	// Make sure this only happens once.
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		srvrLog.Infof("Server is already in the process of shutting down")
		return nil
	}

	srvrLog.Warnf("Server shutting down")

	// Stop all the listeners.  There will not be any listeners if
	// listening is disabled.
	for _, listener := range s.listeners {
		err := listener.Close()
		if err != nil {
			return err
		}
	}

	// Stop the CPU miner if needed
	s.cpuMiner.Stop()

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
}

// ScheduleShutdown schedules a server shutdown after the specified duration.
// It also dynamically adjusts how often to warn the server is going down based
// on remaining duration.
func (s *server) ScheduleShutdown(duration time.Duration) {
	// Don't schedule shutdown more than once.
	if atomic.AddInt32(&s.shutdownSched, 1) != 1 {
		return
	}
	srvrLog.Warnf("Server shutdown in %v", duration)
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
				srvrLog.Warnf("Server shutdown in %v", remaining)
			}
		}
	}()
}

// parseListeners splits the list of listen addresses passed in addrs into
// IPv4 and IPv6 slices and returns them.  This allows easy creation of the
// listeners on the correct interface "tcp4" and "tcp6".  It also properly
// detects addresses which apply to "all interfaces" and adds the address to
// both slices.
func parseListeners(addrs []string) ([]string, []string, bool, error) {
	ipv4ListenAddrs := make([]string, 0, len(addrs)*2)
	ipv6ListenAddrs := make([]string, 0, len(addrs)*2)
	haveWildcard := false

	for _, addr := range addrs {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			// Shouldn't happen due to already being normalized.
			return nil, nil, false, err
		}

		// Empty host or host of * on plan9 is both IPv4 and IPv6.
		if host == "" || (host == "*" && runtime.GOOS == "plan9") {
			ipv4ListenAddrs = append(ipv4ListenAddrs, addr)
			ipv6ListenAddrs = append(ipv6ListenAddrs, addr)
			haveWildcard = true
			continue
		}

		// Parse the IP.
		ip := net.ParseIP(host)
		if ip == nil {
			return nil, nil, false, fmt.Errorf("'%s' is not a "+
				"valid IP address", host)
		}

		// To4 returns nil when the IP is not an IPv4 address, so use
		// this determine the address type.
		if ip.To4() == nil {
			ipv6ListenAddrs = append(ipv6ListenAddrs, addr)
		} else {
			ipv4ListenAddrs = append(ipv4ListenAddrs, addr)
		}
	}
	return ipv4ListenAddrs, ipv6ListenAddrs, haveWildcard, nil
}

func (s *server) upnpUpdateThread() {
	// Go off immediately to prevent code duplication, thereafter we renew
	// lease every 15 minutes.
	timer := time.NewTimer(0 * time.Second)
	lport, _ := strconv.ParseInt(activeNetParams.DefaultPort, 10, 16)
	first := true
out:
	for {
		select {
		case <-timer.C:
			// TODO(oga) pick external port  more cleverly
			// TODO(oga) know which ports we are listening to on an external net.
			// TODO(oga) if specific listen port doesn't work then ask for wildcard
			// listen port?
			// XXX this assumes timeout is in seconds.
			listenPort, err := s.nat.AddPortMapping("tcp", int(lport), int(lport),
				"btcd listen port", 20*60)
			if err != nil {
				srvrLog.Warnf("can't add UPnP port mapping: %v", err)
			}
			if first && err == nil {
				// TODO(oga): look this up periodically to see if upnp domain changed
				// and so did ip.
				externalip, err := s.nat.GetExternalAddress()
				if err != nil {
					srvrLog.Warnf("UPnP can't get external address: %v", err)
					continue out
				}
				na := wire.NewNetAddressIPPort(externalip, uint16(listenPort),
					wire.SFNodeNetwork)
				err = s.addrManager.AddLocalAddress(na, addrmgr.UpnpPrio)
				if err != nil {
					// XXX DeletePortMapping?
				}
				srvrLog.Warnf("Successfully bound via UPnP to %s", addrmgr.NetAddressKey(na))
				first = false
			}
			timer.Reset(time.Minute * 15)
		case <-s.quit:
			break out
		}
	}

	timer.Stop()

	if err := s.nat.DeletePortMapping("tcp", int(lport), int(lport)); err != nil {
		srvrLog.Warnf("unable to remove UPnP port mapping: %v", err)
	} else {
		srvrLog.Debugf("succesfully disestablished UPnP port mapping")
	}

	s.wg.Done()
}

// newServer returns a new btcd server configured to listen on addr for the
// bitcoin network type specified by chainParams.  Use start to begin accepting
// connections from peers.
func newServer(listenAddrs []string, db database.Db, chainParams *chaincfg.Params) (*server, error) {
	nonce, err := wire.RandomUint64()
	if err != nil {
		return nil, err
	}

	amgr := addrmgr.New(cfg.DataDir, btcdLookup)

	var listeners []net.Listener
	var nat NAT
	if !cfg.DisableListen {
		ipv4Addrs, ipv6Addrs, wildcard, err :=
			parseListeners(listenAddrs)
		if err != nil {
			return nil, err
		}
		listeners = make([]net.Listener, 0, len(ipv4Addrs)+len(ipv6Addrs))
		discover := true
		if len(cfg.ExternalIPs) != 0 {
			discover = false
			// if this fails we have real issues.
			port, _ := strconv.ParseUint(
				activeNetParams.DefaultPort, 10, 16)

			for _, sip := range cfg.ExternalIPs {
				eport := uint16(port)
				host, portstr, err := net.SplitHostPort(sip)
				if err != nil {
					// no port, use default.
					host = sip
				} else {
					port, err := strconv.ParseUint(
						portstr, 10, 16)
					if err != nil {
						srvrLog.Warnf("Can not parse "+
							"port from %s for "+
							"externalip: %v", sip,
							err)
						continue
					}
					eport = uint16(port)
				}
				na, err := amgr.HostToNetAddress(host, eport,
					wire.SFNodeNetwork)
				if err != nil {
					srvrLog.Warnf("Not adding %s as "+
						"externalip: %v", sip, err)
					continue
				}

				err = amgr.AddLocalAddress(na, addrmgr.ManualPrio)
				if err != nil {
					amgrLog.Warnf("Skipping specified external IP: %v", err)
				}
			}
		} else if discover && cfg.Upnp {
			nat, err = Discover()
			if err != nil {
				srvrLog.Warnf("Can't discover upnp: %v", err)
			}
			// nil nat here is fine, just means no upnp on network.
		}

		// TODO(oga) nonstandard port...
		if wildcard {
			port, err :=
				strconv.ParseUint(activeNetParams.DefaultPort,
					10, 16)
			if err != nil {
				// I can't think of a cleaner way to do this...
				goto nowc
			}
			addrs, err := net.InterfaceAddrs()
			for _, a := range addrs {
				ip, _, err := net.ParseCIDR(a.String())
				if err != nil {
					continue
				}
				na := wire.NewNetAddressIPPort(ip,
					uint16(port), wire.SFNodeNetwork)
				if discover {
					err = amgr.AddLocalAddress(na, addrmgr.InterfacePrio)
					if err != nil {
						amgrLog.Debugf("Skipping local address: %v", err)
					}
				}
			}
		}
	nowc:

		for _, addr := range ipv4Addrs {
			listener, err := net.Listen("tcp4", addr)
			if err != nil {
				srvrLog.Warnf("Can't listen on %s: %v", addr,
					err)
				continue
			}
			listeners = append(listeners, listener)

			if discover {
				if na, err := amgr.DeserializeNetAddress(addr); err == nil {
					err = amgr.AddLocalAddress(na, addrmgr.BoundPrio)
					if err != nil {
						amgrLog.Warnf("Skipping bound address: %v", err)
					}
				}
			}
		}

		for _, addr := range ipv6Addrs {
			listener, err := net.Listen("tcp6", addr)
			if err != nil {
				srvrLog.Warnf("Can't listen on %s: %v", addr,
					err)
				continue
			}
			listeners = append(listeners, listener)
			if discover {
				if na, err := amgr.DeserializeNetAddress(addr); err == nil {
					err = amgr.AddLocalAddress(na, addrmgr.BoundPrio)
					if err != nil {
						amgrLog.Debugf("Skipping bound address: %v", err)
					}
				}
			}
		}

		if len(listeners) == 0 {
			return nil, errors.New("no valid listen address")
		}
	}

	s := server{
		nonce:                nonce,
		listeners:            listeners,
		chainParams:          chainParams,
		addrManager:          amgr,
		newPeers:             make(chan *peer, cfg.MaxPeers),
		donePeers:            make(chan *peer, cfg.MaxPeers),
		banPeers:             make(chan *peer, cfg.MaxPeers),
		wakeup:               make(chan struct{}),
		query:                make(chan interface{}),
		relayInv:             make(chan relayMsg, cfg.MaxPeers),
		broadcast:            make(chan broadcastMsg, cfg.MaxPeers),
		quit:                 make(chan struct{}),
		modifyRebroadcastInv: make(chan interface{}),
		peerHeightsUpdate:    make(chan updatePeerHeightsMsg),
		nat:                  nat,
		db:                   db,
		timeSource:           blockchain.NewMedianTime(),
	}
	bm, err := newBlockManager(&s)
	if err != nil {
		return nil, err
	}
	s.blockManager = bm
	s.txMemPool = newTxMemPool(&s)
	s.cpuMiner = newCPUMiner(&s)

	if cfg.AddrIndex {
		ai, err := newAddrIndexer(&s)
		if err != nil {
			return nil, err
		}
		s.addrIndexer = ai
	}

	if !cfg.DisableRPC {
		s.rpcServer, err = newRPCServer(cfg.RPCListeners, &s)
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
