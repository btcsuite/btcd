// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	mrand "math/rand"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/addrmgr"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/blockchain/indexers"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/mining"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/bloom"
)

const (
	// These constants are used by the DNS seed code to pick a random last
	// seen time.
	secondsIn3Days int32 = 24 * 60 * 60 * 3
	secondsIn4Days int32 = 24 * 60 * 60 * 4
)

const (
	// defaultServices describes the default services that are supported by
	// the server.
	defaultServices = wire.SFNodeNetwork | wire.SFNodeBloom

	// defaultMaxOutbound is the default number of max outbound peers.
	defaultMaxOutbound = 8

	// connectionRetryInterval is the base amount of time to wait in between
	// retries when connecting to persistent peers.  It is adjusted by the
	// number of retries such that there is a retry backoff.
	connectionRetryInterval = time.Second * 5

	// maxConnectionRetryInterval is the max amount of time retrying of a
	// persistent peer is allowed to grow to.  This is necessary since the
	// retry logic uses a backoff mechanism which increases the interval
	// base done the number of retries that have been done.
	maxConnectionRetryInterval = time.Minute * 5
)

var (
	// userAgentName is the user agent name and is used to help identify
	// ourselves to other bitcoin peers.
	userAgentName = "btcd"

	// userAgentVersion is the user agent version and is used to help
	// identify ourselves to other bitcoin peers.
	userAgentVersion = fmt.Sprintf("%d.%d.%d", appMajor, appMinor, appPatch)
)

// broadcastMsg provides the ability to house a bitcoin message to be broadcast
// to all connected peers except specified excluded peers.
type broadcastMsg struct {
	message      wire.Message
	excludePeers []*serverPeer
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
	originPeer *serverPeer
}

// peerState maintains state of inbound, persistent, outbound peers as well
// as banned peers and outbound groups.
type peerState struct {
	pendingPeers     map[string]*serverPeer
	inboundPeers     map[int32]*serverPeer
	outboundPeers    map[int32]*serverPeer
	persistentPeers  map[int32]*serverPeer
	banned           map[string]time.Time
	outboundGroups   map[string]int
	maxOutboundPeers int
}

// Count returns the count of all known peers.
func (ps *peerState) Count() int {
	return len(ps.inboundPeers) + len(ps.outboundPeers) +
		len(ps.persistentPeers)
}

// OutboundCount returns the count of known outbound peers.
func (ps *peerState) OutboundCount() int {
	return len(ps.outboundPeers) + len(ps.persistentPeers)
}

// NeedMoreOutbound returns true if more outbound peers are required.
func (ps *peerState) NeedMoreOutbound() bool {
	return ps.OutboundCount() < ps.maxOutboundPeers &&
		ps.Count() < cfg.MaxPeers
}

// NeedMoreTries returns true if more outbound peer attempts can be tried.
func (ps *peerState) NeedMoreTries() bool {
	return len(ps.pendingPeers) < 2*(ps.maxOutboundPeers-ps.OutboundCount())
}

// forAllOutboundPeers is a helper function that runs closure on all outbound
// peers known to peerState.
func (ps *peerState) forAllOutboundPeers(closure func(sp *serverPeer)) {
	for _, e := range ps.outboundPeers {
		closure(e)
	}
	for _, e := range ps.persistentPeers {
		closure(e)
	}
}

// forPendingPeers is a helper function that runs closure on all pending peers
// known to peerState.
func (ps *peerState) forPendingPeers(closure func(sp *serverPeer)) {
	for _, e := range ps.pendingPeers {
		closure(e)
	}
}

// forAllPeers is a helper function that runs closure on all peers known to
// peerState.
func (ps *peerState) forAllPeers(closure func(sp *serverPeer)) {
	for _, e := range ps.inboundPeers {
		closure(e)
	}
	ps.forAllOutboundPeers(closure)
}

// server provides a bitcoin server for handling communications to and from
// bitcoin peers.
type server struct {
	// The following variables must only be used atomically.
	// Putting the uint64s first makes them 64-bit aligned for 32-bit systems.
	bytesReceived uint64 // Total bytes received from all peers since start.
	bytesSent     uint64 // Total bytes sent by all peers since start.
	started       int32
	shutdown      int32
	shutdownSched int32

	listeners            []net.Listener
	chainParams          *chaincfg.Params
	addrManager          *addrmgr.AddrManager
	sigCache             *txscript.SigCache
	rpcServer            *rpcServer
	blockManager         *blockManager
	txMemPool            *txMemPool
	cpuMiner             *CPUMiner
	modifyRebroadcastInv chan interface{}
	pendingPeers         chan *serverPeer
	newPeers             chan *serverPeer
	donePeers            chan *serverPeer
	banPeers             chan *serverPeer
	retryPeers           chan *serverPeer
	wakeup               chan struct{}
	query                chan interface{}
	relayInv             chan relayMsg
	broadcast            chan broadcastMsg
	peerHeightsUpdate    chan updatePeerHeightsMsg
	wg                   sync.WaitGroup
	quit                 chan struct{}
	nat                  NAT
	db                   database.DB
	timeSource           blockchain.MedianTimeSource
	services             wire.ServiceFlag

	// The following fields are used for optional indexes.  They will be nil
	// if the associated index is not enabled.  These fields are set during
	// initial creation of the server and never changed afterwards, so they
	// do not need to be protected for concurrent access.
	txIndex   *indexers.TxIndex
	addrIndex *indexers.AddrIndex
}

// serverPeer extends the peer to maintain state shared by the server and
// the blockmanager.
type serverPeer struct {
	*peer.Peer

	server          *server
	persistent      bool
	continueHash    *wire.ShaHash
	relayMtx        sync.Mutex
	disableRelayTx  bool
	requestQueue    []*wire.InvVect
	requestedTxns   map[wire.ShaHash]struct{}
	requestedBlocks map[wire.ShaHash]struct{}
	filter          *bloom.Filter
	knownAddresses  map[string]struct{}
	banScore        dynamicBanScore
	quit            chan struct{}
	// The following chans are used to sync blockmanager and server.
	txProcessed    chan struct{}
	blockProcessed chan struct{}
}

// newServerPeer returns a new serverPeer instance. The peer needs to be set by
// the caller.
func newServerPeer(s *server, isPersistent bool) *serverPeer {
	return &serverPeer{
		server:          s,
		persistent:      isPersistent,
		requestedTxns:   make(map[wire.ShaHash]struct{}),
		requestedBlocks: make(map[wire.ShaHash]struct{}),
		filter:          bloom.LoadFilter(nil),
		knownAddresses:  make(map[string]struct{}),
		quit:            make(chan struct{}),
		txProcessed:     make(chan struct{}, 1),
		blockProcessed:  make(chan struct{}, 1),
	}
}

// newestBlock returns the current best block hash and height using the format
// required by the configuration for the peer package.
func (sp *serverPeer) newestBlock() (*wire.ShaHash, int32, error) {
	best := sp.server.blockManager.chain.BestSnapshot()
	return best.Hash, best.Height, nil
}

// addKnownAddresses adds the given addresses to the set of known addreses to
// the peer to prevent sending duplicate addresses.
func (sp *serverPeer) addKnownAddresses(addresses []*wire.NetAddress) {
	for _, na := range addresses {
		sp.knownAddresses[addrmgr.NetAddressKey(na)] = struct{}{}
	}
}

// addressKnown true if the given address is already known to the peer.
func (sp *serverPeer) addressKnown(na *wire.NetAddress) bool {
	_, exists := sp.knownAddresses[addrmgr.NetAddressKey(na)]
	return exists
}

// setDisableRelayTx toggles relaying of transactions for the given peer.
// It is safe for concurrent access.
func (sp *serverPeer) setDisableRelayTx(disable bool) {
	sp.relayMtx.Lock()
	sp.disableRelayTx = disable
	sp.relayMtx.Unlock()
}

// relayTxDisabled returns whether or not relaying of transactions for the given
// peer is disabled.
// It is safe for concurrent access.
func (sp *serverPeer) relayTxDisabled() bool {
	sp.relayMtx.Lock()
	defer sp.relayMtx.Unlock()

	return sp.disableRelayTx
}

// pushAddrMsg sends an addr message to the connected peer using the provided
// addresses.
func (sp *serverPeer) pushAddrMsg(addresses []*wire.NetAddress) {
	// Filter addresses already known to the peer.
	addrs := make([]*wire.NetAddress, 0, len(addresses))
	for _, addr := range addresses {
		if !sp.addressKnown(addr) {
			addrs = append(addrs, addr)
		}
	}
	known, err := sp.PushAddrMsg(addrs)
	if err != nil {
		peerLog.Errorf("Can't push address message to %s: %v", sp.Peer, err)
		sp.Disconnect()
		return
	}
	sp.addKnownAddresses(known)
}

// addBanScore increases the persistent and decaying ban score fields by the
// values passed as parameters. If the resulting score exceeds half of the ban
// threshold, a warning is logged including the reason provided. Further, if
// the score is above the ban threshold, the peer will be banned and
// disconnected.
func (sp *serverPeer) addBanScore(persistent, transient uint32, reason string) {
	// No warning is logged and no score is calculated if banning is disabled.
	if cfg.DisableBanning {
		return
	}
	warnThreshold := cfg.BanThreshold >> 1
	if transient == 0 && persistent == 0 {
		// The score is not being increased, but a warning message is still
		// logged if the score is above the warn threshold.
		score := sp.banScore.Int()
		if score > warnThreshold {
			peerLog.Warnf("Misbehaving peer %s: %s -- ban score is %d, "+
				"it was not increased this time", sp, reason, score)
		}
		return
	}
	score := sp.banScore.Increase(persistent, transient)
	if score > warnThreshold {
		peerLog.Warnf("Misbehaving peer %s: %s -- ban score increased to %d",
			sp, reason, score)
		if score > cfg.BanThreshold {
			peerLog.Warnf("Misbehaving peer %s -- banning and disconnecting",
				sp)
			sp.server.BanPeer(sp)
			sp.Disconnect()
		}
	}
}

// OnVersion is invoked when a peer receives a version bitcoin message
// and is used to negotiate the protocol version details as well as kick start
// the communications.
func (sp *serverPeer) OnVersion(p *peer.Peer, msg *wire.MsgVersion) {
	// Add the remote peer time as a sample for creating an offset against
	// the local clock to keep the network time in sync.
	sp.server.timeSource.AddTimeSample(p.Addr(), msg.Timestamp)

	// Signal the block manager this peer is a new sync candidate.
	sp.server.blockManager.NewPeer(sp)

	// Choose whether or not to relay transactions before a filter command
	// is received.
	sp.setDisableRelayTx(msg.DisableRelayTx)

	// Update the address manager and request known addresses from the
	// remote peer for outbound connections.  This is skipped when running
	// on the simulation test network since it is only intended to connect
	// to specified peers and actively avoids advertising and connecting to
	// discovered peers.
	if !cfg.SimNet {
		addrManager := sp.server.addrManager
		// Outbound connections.
		if !p.Inbound() {
			// TODO(davec): Only do this if not doing the initial block
			// download and the local address is routable.
			if !cfg.DisableListen /* && isCurrent? */ {
				// Get address that best matches.
				lna := addrManager.GetBestLocalAddress(p.NA())
				if addrmgr.IsRoutable(lna) {
					// Filter addresses the peer already knows about.
					addresses := []*wire.NetAddress{lna}
					sp.pushAddrMsg(addresses)
				}
			}

			// Request known addresses if the server address manager needs
			// more and the peer has a protocol version new enough to
			// include a timestamp with addresses.
			hasTimestamp := p.ProtocolVersion() >=
				wire.NetAddressTimeVersion
			if addrManager.NeedMoreAddresses() && hasTimestamp {
				p.QueueMessage(wire.NewMsgGetAddr(), nil)
			}

			// Mark the address as a known good address.
			addrManager.Good(p.NA())
		} else {
			// A peer might not be advertising the same address that it
			// actually connected from.  One example of why this can happen
			// is with NAT.  Only add the address to the address manager if
			// the addresses agree.
			if addrmgr.NetAddressKey(&msg.AddrMe) == addrmgr.NetAddressKey(p.NA()) {
				addrManager.AddAddress(p.NA(), p.NA())
				addrManager.Good(p.NA())
			}
		}
	}

	// Add valid peer to the server.
	sp.server.AddPeer(sp)
}

// OnMemPool is invoked when a peer receives a mempool bitcoin message.
// It creates and sends an inventory message with the contents of the memory
// pool up to the maximum inventory allowed per message.  When the peer has a
// bloom filter loaded, the contents are filtered accordingly.
func (sp *serverPeer) OnMemPool(p *peer.Peer, msg *wire.MsgMemPool) {
	// A decaying ban score increase is applied to prevent flooding.
	// The ban score accumulates and passes the ban threshold if a burst of
	// mempool messages comes from a peer. The score decays each minute to
	// half of its value.
	sp.addBanScore(0, 33, "mempool")

	// Generate inventory message with the available transactions in the
	// transaction memory pool.  Limit it to the max allowed inventory
	// per message.  The NewMsgInvSizeHint function automatically limits
	// the passed hint to the maximum allowed, so it's safe to pass it
	// without double checking it here.
	txMemPool := sp.server.txMemPool
	txDescs := txMemPool.TxDescs()
	invMsg := wire.NewMsgInvSizeHint(uint(len(txDescs)))

	for i, txDesc := range txDescs {
		// Another thread might have removed the transaction from the
		// pool since the initial query.
		hash := txDesc.Tx.Sha()
		if !txMemPool.IsTransactionInPool(hash) {
			continue
		}

		// Either add all transactions when there is no bloom filter,
		// or only the transactions that match the filter when there is
		// one.
		if !sp.filter.IsLoaded() || sp.filter.MatchTxAndUpdate(txDesc.Tx) {
			iv := wire.NewInvVect(wire.InvTypeTx, hash)
			invMsg.AddInvVect(iv)
			if i+1 >= wire.MaxInvPerMsg {
				break
			}
		}
	}

	// Send the inventory message if there is anything to send.
	if len(invMsg.InvList) > 0 {
		p.QueueMessage(invMsg, nil)
	}
}

// OnTx is invoked when a peer receives a tx bitcoin message.  It blocks
// until the bitcoin transaction has been fully processed.  Unlock the block
// handler this does not serialize all transactions through a single thread
// transactions don't rely on the previous one in a linear fashion like blocks.
func (sp *serverPeer) OnTx(p *peer.Peer, msg *wire.MsgTx) {
	if cfg.BlocksOnly {
		peerLog.Tracef("Ignoring tx %v from %v - blocksonly enabled",
			msg.TxSha(), p)
		return
	}

	// Add the transaction to the known inventory for the peer.
	// Convert the raw MsgTx to a btcutil.Tx which provides some convenience
	// methods and things such as hash caching.
	tx := btcutil.NewTx(msg)
	iv := wire.NewInvVect(wire.InvTypeTx, tx.Sha())
	p.AddKnownInventory(iv)

	// Queue the transaction up to be handled by the block manager and
	// intentionally block further receives until the transaction is fully
	// processed and known good or bad.  This helps prevent a malicious peer
	// from queuing up a bunch of bad transactions before disconnecting (or
	// being disconnected) and wasting memory.
	sp.server.blockManager.QueueTx(tx, sp)
	<-sp.txProcessed
}

// OnBlock is invoked when a peer receives a block bitcoin message.  It
// blocks until the bitcoin block has been fully processed.
func (sp *serverPeer) OnBlock(p *peer.Peer, msg *wire.MsgBlock, buf []byte) {
	// Convert the raw MsgBlock to a btcutil.Block which provides some
	// convenience methods and things such as hash caching.
	block := btcutil.NewBlockFromBlockAndBytes(msg, buf)

	// Add the block to the known inventory for the peer.
	iv := wire.NewInvVect(wire.InvTypeBlock, block.Sha())
	p.AddKnownInventory(iv)

	// Queue the block up to be handled by the block
	// manager and intentionally block further receives
	// until the bitcoin block is fully processed and known
	// good or bad.  This helps prevent a malicious peer
	// from queuing up a bunch of bad blocks before
	// disconnecting (or being disconnected) and wasting
	// memory.  Additionally, this behavior is depended on
	// by at least the block acceptance test tool as the
	// reference implementation processes blocks in the same
	// thread and therefore blocks further messages until
	// the bitcoin block has been fully processed.
	sp.server.blockManager.QueueBlock(block, sp)
	<-sp.blockProcessed
}

// OnInv is invoked when a peer receives an inv bitcoin message and is
// used to examine the inventory being advertised by the remote peer and react
// accordingly.  We pass the message down to blockmanager which will call
// QueueMessage with any appropriate responses.
func (sp *serverPeer) OnInv(p *peer.Peer, msg *wire.MsgInv) {
	if !cfg.BlocksOnly {
		if len(msg.InvList) > 0 {
			sp.server.blockManager.QueueInv(msg, sp)
		}
		return
	}

	newInv := wire.NewMsgInvSizeHint(uint(len(msg.InvList)))
	for _, invVect := range msg.InvList {
		if invVect.Type == wire.InvTypeTx {
			peerLog.Tracef("Ignoring tx %v in inv from %v -- "+
				"blocksonly enabled", invVect.Hash, p)
			if p.ProtocolVersion() >= wire.BIP0037Version {
				peerLog.Infof("Peer %v is announcing "+
					"transactions -- disconnecting", p)
				p.Disconnect()
				return
			}
			continue
		}
		err := newInv.AddInvVect(invVect)
		if err != nil {
			peerLog.Errorf("Failed to add inventory vector: %v", err)
			break
		}
	}

	if len(newInv.InvList) > 0 {
		sp.server.blockManager.QueueInv(newInv, sp)
	}
}

// OnHeaders is invoked when a peer receives a headers bitcoin
// message.  The message is passed down to the block manager.
func (sp *serverPeer) OnHeaders(p *peer.Peer, msg *wire.MsgHeaders) {
	sp.server.blockManager.QueueHeaders(msg, sp)
}

// handleGetData is invoked when a peer receives a getdata bitcoin message and
// is used to deliver block and transaction information.
func (sp *serverPeer) OnGetData(p *peer.Peer, msg *wire.MsgGetData) {
	numAdded := 0
	notFound := wire.NewMsgNotFound()

	length := len(msg.InvList)
	// A decaying ban score increase is applied to prevent exhausting resources
	// with unusually large inventory queries.
	// Requesting more than the maximum inventory vector length within a short
	// period of time yields a score above the default ban threshold. Sustained
	// bursts of small requests are not penalized as that would potentially ban
	// peers performing IBD.
	// This incremental score decays each minute to half of its value.
	sp.addBanScore(0, uint32(length)*99/wire.MaxInvPerMsg, "getdata")

	// We wait on this wait channel periodically to prevent queuing
	// far more data than we can send in a reasonable time, wasting memory.
	// The waiting occurs after the database fetch for the next one to
	// provide a little pipelining.
	var waitChan chan struct{}
	doneChan := make(chan struct{}, 1)

	for i, iv := range msg.InvList {
		var c chan struct{}
		// If this will be the last message we send.
		if i == length-1 && len(notFound.InvList) == 0 {
			c = doneChan
		} else if (i+1)%3 == 0 {
			// Buffered so as to not make the send goroutine block.
			c = make(chan struct{}, 1)
		}
		var err error
		switch iv.Type {
		case wire.InvTypeTx:
			err = sp.server.pushTxMsg(sp, &iv.Hash, c, waitChan)
		case wire.InvTypeBlock:
			err = sp.server.pushBlockMsg(sp, &iv.Hash, c, waitChan)
		case wire.InvTypeFilteredBlock:
			err = sp.server.pushMerkleBlockMsg(sp, &iv.Hash, c, waitChan)
		default:
			peerLog.Warnf("Unknown type in inventory request %d",
				iv.Type)
			continue
		}
		if err != nil {
			notFound.AddInvVect(iv)

			// When there is a failure fetching the final entry
			// and the done channel was sent in due to there
			// being no outstanding not found inventory, consume
			// it here because there is now not found inventory
			// that will use the channel momentarily.
			if i == len(msg.InvList)-1 && c != nil {
				<-c
			}
		}
		numAdded++
		waitChan = c
	}
	if len(notFound.InvList) != 0 {
		p.QueueMessage(notFound, doneChan)
	}

	// Wait for messages to be sent. We can send quite a lot of data at this
	// point and this will keep the peer busy for a decent amount of time.
	// We don't process anything else by them in this time so that we
	// have an idea of when we should hear back from them - else the idle
	// timeout could fire when we were only half done sending the blocks.
	if numAdded > 0 {
		<-doneChan
	}
}

// OnGetBlocks is invoked when a peer receives a getblocks bitcoin
// message.
func (sp *serverPeer) OnGetBlocks(p *peer.Peer, msg *wire.MsgGetBlocks) {
	// Return all block hashes to the latest one (up to max per message) if
	// no stop hash was specified.
	// Attempt to find the ending index of the stop hash if specified.
	chain := sp.server.blockManager.chain
	endIdx := int32(math.MaxInt32)
	if !msg.HashStop.IsEqual(&zeroHash) {
		height, err := chain.BlockHeightByHash(&msg.HashStop)
		if err == nil {
			endIdx = height + 1
		}
	}

	// Find the most recent known block based on the block locator.
	// Use the block after the genesis block if no other blocks in the
	// provided locator are known.  This does mean the client will start
	// over with the genesis block if unknown block locators are provided.
	// This mirrors the behavior in the reference implementation.
	startIdx := int32(1)
	for _, hash := range msg.BlockLocatorHashes {
		height, err := chain.BlockHeightByHash(hash)
		if err == nil {
			// Start with the next hash since we know this one.
			startIdx = height + 1
			break
		}
	}

	// Don't attempt to fetch more than we can put into a single message.
	autoContinue := false
	if endIdx-startIdx > wire.MaxBlocksPerMsg {
		endIdx = startIdx + wire.MaxBlocksPerMsg
		autoContinue = true
	}

	// Fetch the inventory from the block database.
	hashList, err := chain.HeightRange(startIdx, endIdx)
	if err != nil {
		peerLog.Warnf("Block lookup failed: %v", err)
		return
	}

	// Generate inventory message.
	invMsg := wire.NewMsgInv()
	for i := range hashList {
		iv := wire.NewInvVect(wire.InvTypeBlock, &hashList[i])
		invMsg.AddInvVect(iv)
	}

	// Send the inventory message if there is anything to send.
	if len(invMsg.InvList) > 0 {
		invListLen := len(invMsg.InvList)
		if autoContinue && invListLen == wire.MaxBlocksPerMsg {
			// Intentionally use a copy of the final hash so there
			// is not a reference into the inventory slice which
			// would prevent the entire slice from being eligible
			// for GC as soon as it's sent.
			continueHash := invMsg.InvList[invListLen-1].Hash
			sp.continueHash = &continueHash
		}
		p.QueueMessage(invMsg, nil)
	}
}

// OnGetHeaders is invoked when a peer receives a getheaders bitcoin
// message.
func (sp *serverPeer) OnGetHeaders(p *peer.Peer, msg *wire.MsgGetHeaders) {
	// Ignore getheaders requests if not in sync.
	if !sp.server.blockManager.IsCurrent() {
		return
	}

	// Attempt to look up the height of the provided stop hash.
	chain := sp.server.blockManager.chain
	endIdx := int32(math.MaxInt32)
	height, err := chain.BlockHeightByHash(&msg.HashStop)
	if err == nil {
		endIdx = height + 1
	}

	// There are no block locators so a specific header is being requested
	// as identified by the stop hash.
	if len(msg.BlockLocatorHashes) == 0 {
		// No blocks with the stop hash were found so there is nothing
		// to do.  Just return.  This behavior mirrors the reference
		// implementation.
		if endIdx == math.MaxInt32 {
			return
		}

		// Fetch the raw block header bytes from the database.
		var headerBytes []byte
		err := sp.server.db.View(func(dbTx database.Tx) error {
			var err error
			headerBytes, err = dbTx.FetchBlockHeader(&msg.HashStop)
			return err
		})
		if err != nil {
			peerLog.Warnf("Lookup of known block hash failed: %v",
				err)
			return
		}

		// Deserialize the block header.
		var header wire.BlockHeader
		err = header.Deserialize(bytes.NewReader(headerBytes))
		if err != nil {
			peerLog.Warnf("Block header deserialize failed: %v",
				err)
			return
		}

		headersMsg := wire.NewMsgHeaders()
		headersMsg.AddBlockHeader(&header)
		p.QueueMessage(headersMsg, nil)
		return
	}

	// Find the most recent known block based on the block locator.
	// Use the block after the genesis block if no other blocks in the
	// provided locator are known.  This does mean the client will start
	// over with the genesis block if unknown block locators are provided.
	// This mirrors the behavior in the reference implementation.
	startIdx := int32(1)
	for _, hash := range msg.BlockLocatorHashes {
		height, err := chain.BlockHeightByHash(hash)
		if err == nil {
			// Start with the next hash since we know this one.
			startIdx = height + 1
			break
		}
	}

	// Don't attempt to fetch more than we can put into a single message.
	if endIdx-startIdx > wire.MaxBlockHeadersPerMsg {
		endIdx = startIdx + wire.MaxBlockHeadersPerMsg
	}

	// Fetch the inventory from the block database.
	hashList, err := chain.HeightRange(startIdx, endIdx)
	if err != nil {
		peerLog.Warnf("Header lookup failed: %v", err)
		return
	}

	// Generate headers message and send it.
	headersMsg := wire.NewMsgHeaders()
	err = sp.server.db.View(func(dbTx database.Tx) error {
		for i := range hashList {
			headerBytes, err := dbTx.FetchBlockHeader(&hashList[i])
			if err != nil {
				return err
			}

			var header wire.BlockHeader
			err = header.Deserialize(bytes.NewReader(headerBytes))
			if err != nil {
				return err
			}
			headersMsg.AddBlockHeader(&header)
		}

		return nil
	})
	if err != nil {
		peerLog.Warnf("Failed to build headers: %v", err)
		return
	}

	p.QueueMessage(headersMsg, nil)
}

// OnFilterAdd is invoked when a peer receives a filteradd bitcoin
// message and is used by remote peers to add data to an already loaded bloom
// filter.  The peer will be disconnected if a filter is not loaded when this
// message is received.
func (sp *serverPeer) OnFilterAdd(p *peer.Peer, msg *wire.MsgFilterAdd) {
	if sp.filter.IsLoaded() {
		peerLog.Debugf("%s sent a filteradd request with no filter "+
			"loaded -- disconnecting", p)
		p.Disconnect()
		return
	}

	sp.filter.Add(msg.Data)
}

// OnFilterClear is invoked when a peer receives a filterclear bitcoin
// message and is used by remote peers to clear an already loaded bloom filter.
// The peer will be disconnected if a filter is not loaded when this message is
// received.
func (sp *serverPeer) OnFilterClear(p *peer.Peer, msg *wire.MsgFilterClear) {
	if !sp.filter.IsLoaded() {
		peerLog.Debugf("%s sent a filterclear request with no "+
			"filter loaded -- disconnecting", p)
		p.Disconnect()
		return
	}

	sp.filter.Unload()
}

// OnFilterLoad is invoked when a peer receives a filterload bitcoin
// message and it used to load a bloom filter that should be used for
// delivering merkle blocks and associated transactions that match the filter.
func (sp *serverPeer) OnFilterLoad(p *peer.Peer, msg *wire.MsgFilterLoad) {
	sp.setDisableRelayTx(false)

	sp.filter.Reload(msg)
}

// OnGetAddr is invoked when a peer receives a getaddr bitcoin message
// and is used to provide the peer with known addresses from the address
// manager.
func (sp *serverPeer) OnGetAddr(p *peer.Peer, msg *wire.MsgGetAddr) {
	// Don't return any addresses when running on the simulation test
	// network.  This helps prevent the network from becoming another
	// public test network since it will not be able to learn about other
	// peers that have not specifically been provided.
	if cfg.SimNet {
		return
	}

	// Do not accept getaddr requests from outbound peers.  This reduces
	// fingerprinting attacks.
	if !p.Inbound() {
		return
	}

	// Get the current known addresses from the address manager.
	addrCache := sp.server.addrManager.AddressCache()

	// Push the addresses.
	sp.pushAddrMsg(addrCache)
}

// OnAddr is invoked when a peer receives an addr bitcoin message and is
// used to notify the server about advertised addresses.
func (sp *serverPeer) OnAddr(p *peer.Peer, msg *wire.MsgAddr) {
	// Ignore addresses when running on the simulation test network.  This
	// helps prevent the network from becoming another public test network
	// since it will not be able to learn about other peers that have not
	// specifically been provided.
	if cfg.SimNet {
		return
	}

	// Ignore old style addresses which don't include a timestamp.
	if p.ProtocolVersion() < wire.NetAddressTimeVersion {
		return
	}

	// A message that has no addresses is invalid.
	if len(msg.AddrList) == 0 {
		peerLog.Errorf("Command [%s] from %s does not contain any addresses",
			msg.Command(), p)
		p.Disconnect()
		return
	}

	for _, na := range msg.AddrList {
		// Don't add more address if we're disconnecting.
		if !p.Connected() {
			return
		}

		// Set the timestamp to 5 days ago if it's more than 24 hours
		// in the future so this address is one of the first to be
		// removed when space is needed.
		now := time.Now()
		if na.Timestamp.After(now.Add(time.Minute * 10)) {
			na.Timestamp = now.Add(-1 * time.Hour * 24 * 5)
		}

		// Add address to known addresses for this peer.
		sp.addKnownAddresses([]*wire.NetAddress{na})
	}

	// Add addresses to server address manager.  The address manager handles
	// the details of things such as preventing duplicate addresses, max
	// addresses, and last seen updates.
	// XXX bitcoind gives a 2 hour time penalty here, do we want to do the
	// same?
	sp.server.addrManager.AddAddresses(msg.AddrList, p.NA())
}

// OnRead is invoked when a peer receives a message and it is used to update
// the bytes received by the server.
func (sp *serverPeer) OnRead(p *peer.Peer, bytesRead int, msg wire.Message, err error) {
	sp.server.AddBytesReceived(uint64(bytesRead))
}

// OnWrite is invoked when a peer sends a message and it is used to update
// the bytes sent by the server.
func (sp *serverPeer) OnWrite(p *peer.Peer, bytesWritten int, msg wire.Message, err error) {
	sp.server.AddBytesSent(uint64(bytesWritten))
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

// AnnounceNewTransactions generates and relays inventory vectors and notifies
// both websocket and getblocktemplate long poll clients of the passed
// transactions.  This function should be called whenever new transactions
// are added to the mempool.
func (s *server) AnnounceNewTransactions(newTxs []*btcutil.Tx) {
	// Generate and relay inventory vectors for all newly accepted
	// transactions into the memory pool due to the original being
	// accepted.
	for _, tx := range newTxs {
		// Generate the inventory vector and relay it.
		iv := wire.NewInvVect(wire.InvTypeTx, tx.Sha())
		s.RelayInventory(iv, tx)

		if s.rpcServer != nil {
			// Notify websocket clients about mempool transactions.
			s.rpcServer.ntfnMgr.NotifyMempoolTx(tx, true)

			// Potentially notify any getblocktemplate long poll clients
			// about stale block templates due to the new transaction.
			s.rpcServer.gbtWorkState.NotifyMempoolTx(
				s.txMemPool.LastUpdated())
		}
	}
}

// pushTxMsg sends a tx message for the provided transaction hash to the
// connected peer.  An error is returned if the transaction hash is not known.
func (s *server) pushTxMsg(sp *serverPeer, sha *wire.ShaHash, doneChan chan<- struct{}, waitChan <-chan struct{}) error {
	// Attempt to fetch the requested transaction from the pool.  A
	// call could be made to check for existence first, but simply trying
	// to fetch a missing transaction results in the same behavior.
	tx, err := s.txMemPool.FetchTransaction(sha)
	if err != nil {
		peerLog.Tracef("Unable to fetch tx %v from transaction "+
			"pool: %v", sha, err)

		if doneChan != nil {
			doneChan <- struct{}{}
		}
		return err
	}

	// Once we have fetched data wait for any previous operation to finish.
	if waitChan != nil {
		<-waitChan
	}

	sp.QueueMessage(tx.MsgTx(), doneChan)

	return nil
}

// pushBlockMsg sends a block message for the provided block hash to the
// connected peer.  An error is returned if the block hash is not known.
func (s *server) pushBlockMsg(sp *serverPeer, hash *wire.ShaHash, doneChan chan<- struct{}, waitChan <-chan struct{}) error {
	// Fetch the raw block bytes from the database.
	var blockBytes []byte
	err := sp.server.db.View(func(dbTx database.Tx) error {
		var err error
		blockBytes, err = dbTx.FetchBlock(hash)
		return err
	})
	if err != nil {
		peerLog.Tracef("Unable to fetch requested block hash %v: %v",
			hash, err)

		if doneChan != nil {
			doneChan <- struct{}{}
		}
		return err
	}

	// Deserialize the block.
	var msgBlock wire.MsgBlock
	err = msgBlock.Deserialize(bytes.NewReader(blockBytes))
	if err != nil {
		peerLog.Tracef("Unable to deserialize requested block hash "+
			"%v: %v", hash, err)

		if doneChan != nil {
			doneChan <- struct{}{}
		}
		return err
	}

	// Once we have fetched data wait for any previous operation to finish.
	if waitChan != nil {
		<-waitChan
	}

	// We only send the channel for this message if we aren't sending
	// an inv straight after.
	var dc chan<- struct{}
	continueHash := sp.continueHash
	sendInv := continueHash != nil && continueHash.IsEqual(hash)
	if !sendInv {
		dc = doneChan
	}
	sp.QueueMessage(&msgBlock, dc)

	// When the peer requests the final block that was advertised in
	// response to a getblocks message which requested more blocks than
	// would fit into a single message, send it a new inventory message
	// to trigger it to issue another getblocks message for the next
	// batch of inventory.
	if sendInv {
		best := sp.server.blockManager.chain.BestSnapshot()
		invMsg := wire.NewMsgInvSizeHint(1)
		iv := wire.NewInvVect(wire.InvTypeBlock, best.Hash)
		invMsg.AddInvVect(iv)
		sp.QueueMessage(invMsg, doneChan)
		sp.continueHash = nil
	}
	return nil
}

// pushMerkleBlockMsg sends a merkleblock message for the provided block hash to
// the connected peer.  Since a merkle block requires the peer to have a filter
// loaded, this call will simply be ignored if there is no filter loaded.  An
// error is returned if the block hash is not known.
func (s *server) pushMerkleBlockMsg(sp *serverPeer, hash *wire.ShaHash, doneChan chan<- struct{}, waitChan <-chan struct{}) error {
	// Do not send a response if the peer doesn't have a filter loaded.
	if !sp.filter.IsLoaded() {
		if doneChan != nil {
			doneChan <- struct{}{}
		}
		return nil
	}

	// Fetch the raw block bytes from the database.
	blk, err := sp.server.blockManager.chain.BlockByHash(hash)
	if err != nil {
		peerLog.Tracef("Unable to fetch requested block hash %v: %v",
			hash, err)

		if doneChan != nil {
			doneChan <- struct{}{}
		}
		return err
	}

	// Generate a merkle block by filtering the requested block according
	// to the filter for the peer.
	merkle, matchedTxIndices := bloom.NewMerkleBlock(blk, sp.filter)

	// Once we have fetched data wait for any previous operation to finish.
	if waitChan != nil {
		<-waitChan
	}

	// Send the merkleblock.  Only send the done channel with this message
	// if no transactions will be sent afterwards.
	var dc chan<- struct{}
	if len(matchedTxIndices) == 0 {
		dc = doneChan
	}
	sp.QueueMessage(merkle, dc)

	// Finally, send any matched transactions.
	blkTransactions := blk.MsgBlock().Transactions
	for i, txIndex := range matchedTxIndices {
		// Only send the done channel on the final transaction.
		var dc chan<- struct{}
		if i == len(matchedTxIndices)-1 {
			dc = doneChan
		}
		if txIndex < uint32(len(blkTransactions)) {
			sp.QueueMessage(blkTransactions[txIndex], dc)
		}
	}

	return nil
}

// handleUpdatePeerHeight updates the heights of all peers who were known to
// announce a block we recently accepted.
func (s *server) handleUpdatePeerHeights(state *peerState, umsg updatePeerHeightsMsg) {
	state.forAllPeers(func(sp *serverPeer) {
		// The origin peer should already have the updated height.
		if sp == umsg.originPeer {
			return
		}

		// This is a pointer to the underlying memory which doesn't
		// change.
		latestBlkSha := sp.LastAnnouncedBlock()

		// Skip this peer if it hasn't recently announced any new blocks.
		if latestBlkSha == nil {
			return
		}

		// If the peer has recently announced a block, and this block
		// matches our newly accepted block, then update their block
		// height.
		if *latestBlkSha == *umsg.newSha {
			sp.UpdateLastBlockHeight(umsg.newHeight)
			sp.UpdateLastAnnouncedBlock(nil)
		}
	})
}

// handleAddPeerMsg deals with adding new peers.  It is invoked from the
// peerHandler goroutine.
func (s *server) handleAddPeerMsg(state *peerState, sp *serverPeer) bool {
	if sp == nil {
		return false
	}

	// Ignore new peers if we're shutting down.
	if atomic.LoadInt32(&s.shutdown) != 0 {
		srvrLog.Infof("New peer %s ignored - server is shutting down", sp)
		sp.Disconnect()
		return false
	}

	// Disconnect banned peers.
	host, _, err := net.SplitHostPort(sp.Addr())
	if err != nil {
		srvrLog.Debugf("can't split hostport %v", err)
		sp.Disconnect()
		return false
	}
	if banEnd, ok := state.banned[host]; ok {
		if time.Now().Before(banEnd) {
			srvrLog.Debugf("Peer %s is banned for another %v - disconnecting",
				host, banEnd.Sub(time.Now()))
			sp.Disconnect()
			return false
		}

		srvrLog.Infof("Peer %s is no longer banned", host)
		delete(state.banned, host)
	}

	// TODO: Check for max peers from a single IP.

	// Limit max outbound peers.
	if _, ok := state.pendingPeers[sp.Addr()]; ok {
		if state.OutboundCount() >= state.maxOutboundPeers {
			srvrLog.Infof("Max outbound peers reached [%d] - disconnecting "+
				"peer %s", state.maxOutboundPeers, sp)
			sp.Disconnect()
			return false
		}
	}

	// Limit max number of total peers.
	if state.Count() >= cfg.MaxPeers {
		srvrLog.Infof("Max peers reached [%d] - disconnecting peer %s",
			cfg.MaxPeers, sp)
		sp.Disconnect()
		// TODO(oga) how to handle permanent peers here?
		// they should be rescheduled.
		return false
	}

	// Add the new peer and start it.
	srvrLog.Debugf("New peer %s", sp)
	if sp.Inbound() {
		state.inboundPeers[sp.ID()] = sp
	} else {
		state.outboundGroups[addrmgr.GroupKey(sp.NA())]++
		if sp.persistent {
			state.persistentPeers[sp.ID()] = sp
		} else {
			state.outboundPeers[sp.ID()] = sp
		}
		// Remove from pending peers.
		delete(state.pendingPeers, sp.Addr())
	}

	return true
}

// handleDonePeerMsg deals with peers that have signalled they are done.  It is
// invoked from the peerHandler goroutine.
func (s *server) handleDonePeerMsg(state *peerState, sp *serverPeer) {
	if _, ok := state.pendingPeers[sp.Addr()]; ok {
		delete(state.pendingPeers, sp.Addr())
		srvrLog.Debugf("Removed pending peer %s", sp)
		return
	}

	var list map[int32]*serverPeer
	if sp.persistent {
		list = state.persistentPeers
	} else if sp.Inbound() {
		list = state.inboundPeers
	} else {
		list = state.outboundPeers
	}
	if _, ok := list[sp.ID()]; ok {
		// Issue an asynchronous reconnect if the peer was a
		// persistent outbound connection.
		if !sp.Inbound() && sp.persistent && atomic.LoadInt32(&s.shutdown) == 0 {
			// Retry peer
			sp2 := s.newOutboundPeer(sp.Addr(), sp.persistent)
			if sp2 != nil {
				go s.retryConn(sp2, false)
			}
		}
		if !sp.Inbound() && sp.VersionKnown() {
			state.outboundGroups[addrmgr.GroupKey(sp.NA())]--
		}
		delete(list, sp.ID())
		srvrLog.Debugf("Removed peer %s", sp)
		return
	}

	// Update the address' last seen time if the peer has acknowledged
	// our version and has sent us its version as well.
	if sp.VerAckReceived() && sp.VersionKnown() && sp.NA() != nil {
		s.addrManager.Connected(sp.NA())
	}

	// If we get here it means that either we didn't know about the peer
	// or we purposefully deleted it.
}

// handleBanPeerMsg deals with banning peers.  It is invoked from the
// peerHandler goroutine.
func (s *server) handleBanPeerMsg(state *peerState, sp *serverPeer) {
	host, _, err := net.SplitHostPort(sp.Addr())
	if err != nil {
		srvrLog.Debugf("can't split ban peer %s %v", sp.Addr(), err)
		return
	}
	direction := directionString(sp.Inbound())
	srvrLog.Infof("Banned peer %s (%s) for %v", host, direction,
		cfg.BanDuration)
	state.banned[host] = time.Now().Add(cfg.BanDuration)
}

// handleRelayInvMsg deals with relaying inventory to peers that are not already
// known to have it.  It is invoked from the peerHandler goroutine.
func (s *server) handleRelayInvMsg(state *peerState, msg relayMsg) {
	state.forAllPeers(func(sp *serverPeer) {
		if !sp.Connected() {
			return
		}

		if msg.invVect.Type == wire.InvTypeTx {
			// Don't relay the transaction to the peer when it has
			// transaction relaying disabled.
			if sp.relayTxDisabled() {
				return
			}
			// Don't relay the transaction if there is a bloom
			// filter loaded and the transaction doesn't match it.
			if sp.filter.IsLoaded() {
				tx, ok := msg.data.(*btcutil.Tx)
				if !ok {
					peerLog.Warnf("Underlying data for tx" +
						" inv relay is not a transaction")
					return
				}

				if !sp.filter.MatchTxAndUpdate(tx) {
					return
				}
			}
		}

		// Queue the inventory to be relayed with the next batch.
		// It will be ignored if the peer is already known to
		// have the inventory.
		sp.QueueInventory(msg.invVect)
	})
}

// handleBroadcastMsg deals with broadcasting messages to peers.  It is invoked
// from the peerHandler goroutine.
func (s *server) handleBroadcastMsg(state *peerState, bmsg *broadcastMsg) {
	state.forAllPeers(func(sp *serverPeer) {
		if !sp.Connected() {
			return
		}

		for _, ep := range bmsg.excludePeers {
			if sp == ep {
				return
			}
		}

		sp.QueueMessage(bmsg.message, nil)
	})
}

type getConnCountMsg struct {
	reply chan int32
}

type getPeersMsg struct {
	reply chan []*serverPeer
}

type getAddedNodesMsg struct {
	reply chan []*serverPeer
}

type disconnectNodeMsg struct {
	cmp   func(*serverPeer) bool
	reply chan error
}

type connectNodeMsg struct {
	addr      string
	permanent bool
	reply     chan error
}

type removeNodeMsg struct {
	cmp   func(*serverPeer) bool
	reply chan error
}

// handleQuery is the central handler for all queries and commands from other
// goroutines related to peer state.
func (s *server) handleQuery(state *peerState, querymsg interface{}) {
	switch msg := querymsg.(type) {
	case getConnCountMsg:
		nconnected := int32(0)
		state.forAllPeers(func(sp *serverPeer) {
			if sp.Connected() {
				nconnected++
			}
		})
		msg.reply <- nconnected

	case getPeersMsg:
		peers := make([]*serverPeer, 0, state.Count())
		state.forAllPeers(func(sp *serverPeer) {
			if !sp.Connected() {
				return
			}
			peers = append(peers, sp)
		})
		msg.reply <- peers

	case connectNodeMsg:
		// XXX(oga) duplicate oneshots?
		for _, peer := range state.persistentPeers {
			if peer.Addr() == msg.addr {
				if msg.permanent {
					msg.reply <- errors.New("peer already connected")
				} else {
					msg.reply <- errors.New("peer exists as a permanent peer")
				}
				return
			}
		}

		// TODO(oga) if too many, nuke a non-perm peer.
		sp := s.newOutboundPeer(msg.addr, msg.permanent)
		if sp != nil {
			go s.peerConnHandler(sp)
			msg.reply <- nil
		} else {
			msg.reply <- errors.New("failed to add peer")
		}
	case removeNodeMsg:
		found := disconnectPeer(state.persistentPeers, msg.cmp, func(sp *serverPeer) {
			// Keep group counts ok since we remove from
			// the list now.
			state.outboundGroups[addrmgr.GroupKey(sp.NA())]--
		})

		if found {
			msg.reply <- nil
		} else {
			msg.reply <- errors.New("peer not found")
		}
	// Request a list of the persistent (added) peers.
	case getAddedNodesMsg:
		// Respond with a slice of the relavent peers.
		peers := make([]*serverPeer, 0, len(state.persistentPeers))
		for _, sp := range state.persistentPeers {
			peers = append(peers, sp)
		}
		msg.reply <- peers
	case disconnectNodeMsg:
		// Check inbound peers. We pass a nil callback since we don't
		// require any additional actions on disconnect for inbound peers.
		found := disconnectPeer(state.inboundPeers, msg.cmp, nil)
		if found {
			msg.reply <- nil
			return
		}

		// Check outbound peers.
		found = disconnectPeer(state.outboundPeers, msg.cmp, func(sp *serverPeer) {
			// Keep group counts ok since we remove from
			// the list now.
			state.outboundGroups[addrmgr.GroupKey(sp.NA())]--
		})
		if found {
			// If there are multiple outbound connections to the same
			// ip:port, continue disconnecting them all until no such
			// peers are found.
			for found {
				found = disconnectPeer(state.outboundPeers, msg.cmp, func(sp *serverPeer) {
					state.outboundGroups[addrmgr.GroupKey(sp.NA())]--
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
func disconnectPeer(peerList map[int32]*serverPeer, compareFunc func(*serverPeer) bool, whenFound func(*serverPeer)) bool {
	for addr, peer := range peerList {
		if compareFunc(peer) {
			if whenFound != nil {
				whenFound(peer)
			}

			// This is ok because we are not continuing
			// to iterate so won't corrupt the loop.
			delete(peerList, addr)
			peer.Disconnect()
			return true
		}
	}
	return false
}

// newPeerConfig returns the configuration for the given serverPeer.
func newPeerConfig(sp *serverPeer) *peer.Config {
	return &peer.Config{
		Listeners: peer.MessageListeners{
			OnVersion:     sp.OnVersion,
			OnMemPool:     sp.OnMemPool,
			OnTx:          sp.OnTx,
			OnBlock:       sp.OnBlock,
			OnInv:         sp.OnInv,
			OnHeaders:     sp.OnHeaders,
			OnGetData:     sp.OnGetData,
			OnGetBlocks:   sp.OnGetBlocks,
			OnGetHeaders:  sp.OnGetHeaders,
			OnFilterAdd:   sp.OnFilterAdd,
			OnFilterClear: sp.OnFilterClear,
			OnFilterLoad:  sp.OnFilterLoad,
			OnGetAddr:     sp.OnGetAddr,
			OnAddr:        sp.OnAddr,
			OnRead:        sp.OnRead,
			OnWrite:       sp.OnWrite,

			// Note: The reference client currently bans peers that send alerts
			// not signed with its key.  We could verify against their key, but
			// since the reference client is currently unwilling to support
			// other implementations' alert messages, we will not relay theirs.
			OnAlert: nil,
		},
		NewestBlock:      sp.newestBlock,
		BestLocalAddress: sp.server.addrManager.GetBestLocalAddress,
		HostToNetAddress: sp.server.addrManager.HostToNetAddress,
		Proxy:            cfg.Proxy,
		UserAgentName:    userAgentName,
		UserAgentVersion: userAgentVersion,
		ChainParams:      sp.server.chainParams,
		Services:         sp.server.services,
		DisableRelayTx:   cfg.BlocksOnly,
		ProtocolVersion:  70011,
	}
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
				srvrLog.Errorf("Can't accept connection: %v", err)
			}
			continue
		}
		sp := newServerPeer(s, false)
		sp.Peer = peer.NewInboundPeer(newPeerConfig(sp))
		go s.peerDoneHandler(sp)
		if err := sp.Connect(conn); err != nil {
			if atomic.LoadInt32(&s.shutdown) == 0 {
				srvrLog.Errorf("Can't accept connection: %v", err)
			}
			continue
		}
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

	for _, seeder := range activeNetParams.DNSSeeds {
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

// newOutboundPeer initializes a new outbound peer and setups the message
// listeners.
func (s *server) newOutboundPeer(addr string, persistent bool) *serverPeer {
	sp := newServerPeer(s, persistent)
	p, err := peer.NewOutboundPeer(newPeerConfig(sp), addr)
	if err != nil {
		srvrLog.Errorf("Cannot create outbound peer %s: %v", addr, err)
		return nil
	}
	sp.Peer = p
	go s.peerDoneHandler(sp)
	return sp
}

// peerConnHandler handles peer connections. It must be run in a goroutine.
func (s *server) peerConnHandler(sp *serverPeer) {
	err := s.establishConn(sp)
	if err != nil {
		srvrLog.Debugf("Failed to connect to %s: %v", sp.Addr(), err)
		sp.Disconnect()
	}
}

// peerDoneHandler handles peer disconnects by notifiying the server that it's
// done.
func (s *server) peerDoneHandler(sp *serverPeer) {
	sp.WaitForDisconnect()
	s.donePeers <- sp

	// Only tell block manager we are gone if we ever told it we existed.
	if sp.VersionKnown() {
		s.blockManager.DonePeer(sp)
	}
	close(sp.quit)
}

// establishConn establishes a connection to the peer.
func (s *server) establishConn(sp *serverPeer) error {
	srvrLog.Debugf("Attempting to connect to %s", sp.Addr())
	conn, err := btcdDial("tcp", sp.Addr())
	if err != nil {
		return err
	}
	if err := sp.Connect(conn); err != nil {
		return err
	}
	srvrLog.Debugf("Connected to %s", sp.Addr())
	s.addrManager.Attempt(sp.NA())
	return nil
}

// retryConn retries connection to the peer after the given duration.  It must
// be run as a goroutine.
func (s *server) retryConn(sp *serverPeer, initialAttempt bool) {
	retryDuration := connectionRetryInterval
	for {
		if initialAttempt {
			retryDuration = 0
			initialAttempt = false
		} else {
			srvrLog.Debugf("Retrying connection to %s in %s", sp.Addr(),
				retryDuration)
		}
		select {
		case <-time.After(retryDuration):
			err := s.establishConn(sp)
			if err != nil {
				retryDuration += connectionRetryInterval
				if retryDuration > maxConnectionRetryInterval {
					retryDuration = maxConnectionRetryInterval
				}
				continue
			}
			return

		case <-sp.quit:
			return

		case <-s.quit:
			return
		}
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
		pendingPeers:     make(map[string]*serverPeer),
		inboundPeers:     make(map[int32]*serverPeer),
		persistentPeers:  make(map[int32]*serverPeer),
		outboundPeers:    make(map[int32]*serverPeer),
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
		sp := s.newOutboundPeer(addr, true)
		if sp != nil {
			go s.retryConn(sp, true)
		}
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
			s.handleQuery(state, qmsg)

		case <-s.quit:
			// Disconnect all peers on server shutdown.
			state.forAllPeers(func(sp *serverPeer) {
				srvrLog.Tracef("Shutdown peer %s", sp)
				sp.Disconnect()
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
			state.forPendingPeers(func(sp *serverPeer) {
				srvrLog.Tracef("Shutdown peer %s", sp)
				sp.Disconnect()
			})
			continue
		}
		tries := 0
		for state.NeedMoreOutbound() &&
			state.NeedMoreTries() &&
			atomic.LoadInt32(&s.shutdown) == 0 {
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

			// Check that we don't have a pending connection to this addr.
			addrStr := addrmgr.NetAddressKey(addr.NetAddress())
			if _, ok := state.pendingPeers[addrStr]; ok {
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
			if tries < 30 && time.Now().Sub(addr.LastAttempt()) < 10*time.Minute {
				continue
			}

			// allow nondefault ports after 50 failed tries.
			if fmt.Sprintf("%d", addr.NetAddress().Port) !=
				activeNetParams.DefaultPort && tries < 50 {
				continue
			}

			tries = 0
			sp := s.newOutboundPeer(addrStr, false)
			if sp != nil {
				go s.peerConnHandler(sp)
				state.pendingPeers[sp.Addr()] = sp
			}
		}

		// We need more peers, wake up in ten seconds and try again.
		if state.NeedMoreOutbound() {
			time.AfterFunc(10*time.Second, func() {
				s.wakeup <- struct{}{}
			})
		}
	}

	s.blockManager.Stop()
	s.addrManager.Stop()

	// Drain channels before exiting so nothing is left waiting around
	// to send.
cleanup:
	for {
		select {
		case <-s.newPeers:
		case <-s.donePeers:
		case <-s.peerHeightsUpdate:
		case <-s.relayInv:
		case <-s.broadcast:
		case <-s.wakeup:
		case <-s.query:
		default:
			break cleanup
		}
	}
	s.wg.Done()
	srvrLog.Tracef("Peer handler done")
}

// AddPeer adds a new peer that has already been connected to the server.
func (s *server) AddPeer(sp *serverPeer) {
	s.newPeers <- sp
}

// BanPeer bans a peer that has already been connected to the server by ip.
func (s *server) BanPeer(sp *serverPeer) {
	s.banPeers <- sp
}

// RelayInventory relays the passed inventory vector to all connected peers
// that are not already known to have it.
func (s *server) RelayInventory(invVect *wire.InvVect, data interface{}) {
	s.relayInv <- relayMsg{invVect: invVect, data: data}
}

// BroadcastMessage sends msg to all peers currently connected to the server
// except those in the passed peers to exclude.
func (s *server) BroadcastMessage(msg wire.Message, exclPeers ...*serverPeer) {
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
func (s *server) AddedNodeInfo() []*serverPeer {
	replyChan := make(chan []*serverPeer)
	s.query <- getAddedNodesMsg{reply: replyChan}
	return <-replyChan
}

// Peers returns an array of all connected peers.
func (s *server) Peers() []*serverPeer {
	replyChan := make(chan []*serverPeer)

	s.query <- getPeersMsg{reply: replyChan}

	return <-replyChan
}

// DisconnectNodeByAddr disconnects a peer by target address. Both outbound and
// inbound nodes will be searched for the target node. An error message will
// be returned if the peer was not found.
func (s *server) DisconnectNodeByAddr(addr string) error {
	replyChan := make(chan error)

	s.query <- disconnectNodeMsg{
		cmp:   func(sp *serverPeer) bool { return sp.Addr() == addr },
		reply: replyChan,
	}

	return <-replyChan
}

// DisconnectNodeByID disconnects a peer by target node id. Both outbound and
// inbound nodes will be searched for the target node. An error message will be
// returned if the peer was not found.
func (s *server) DisconnectNodeByID(id int32) error {
	replyChan := make(chan error)

	s.query <- disconnectNodeMsg{
		cmp:   func(sp *serverPeer) bool { return sp.ID() == id },
		reply: replyChan,
	}

	return <-replyChan
}

// RemoveNodeByAddr removes a peer from the list of persistent peers if
// present. An error will be returned if the peer was not found.
func (s *server) RemoveNodeByAddr(addr string) error {
	replyChan := make(chan error)

	s.query <- removeNodeMsg{
		cmp:   func(sp *serverPeer) bool { return sp.Addr() == addr },
		reply: replyChan,
	}

	return <-replyChan
}

// RemoveNodeByID removes a peer by node ID from the list of persistent peers
// if present. An error will be returned if the peer was not found.
func (s *server) RemoveNodeByID(id int32) error {
	replyChan := make(chan error)

	s.query <- removeNodeMsg{
		cmp:   func(sp *serverPeer) bool { return sp.ID() == id },
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
	atomic.AddUint64(&s.bytesSent, bytesSent)
}

// AddBytesReceived adds the passed number of bytes to the total bytes received
// counter for the server.  It is safe for concurrent access.
func (s *server) AddBytesReceived(bytesReceived uint64) {
	atomic.AddUint64(&s.bytesReceived, bytesReceived)
}

// NetTotals returns the sum of all bytes received and sent across the network
// for all peers.  It is safe for concurrent access.
func (s *server) NetTotals() (uint64, uint64) {
	return atomic.LoadUint64(&s.bytesReceived),
		atomic.LoadUint64(&s.bytesSent)
}

// UpdatePeerHeights updates the heights of all peers who have have announced
// the latest connected main chain block, or a recognized orphan. These height
// updates allow us to dynamically refresh peer heights, ensuring sync peer
// selection has access to the latest block heights for each peer.
func (s *server) UpdatePeerHeights(latestBlkSha *wire.ShaHash, latestHeight int32, updateSource *serverPeer) {
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

		// Strip IPv6 zone id if present since net.ParseIP does not
		// handle it.
		zoneIndex := strings.LastIndex(host, "%")
		if zoneIndex > 0 {
			host = host[:zoneIndex]
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
					s.services)
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
func newServer(listenAddrs []string, db database.DB, chainParams *chaincfg.Params) (*server, error) {
	services := defaultServices
	if cfg.NoPeerBloomFilters {
		services &^= wire.SFNodeBloom
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
					services)
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
					uint16(port), services)
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
		listeners:            listeners,
		chainParams:          chainParams,
		addrManager:          amgr,
		newPeers:             make(chan *serverPeer, cfg.MaxPeers),
		donePeers:            make(chan *serverPeer, cfg.MaxPeers),
		banPeers:             make(chan *serverPeer, cfg.MaxPeers),
		retryPeers:           make(chan *serverPeer, cfg.MaxPeers),
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
		services:             services,
		sigCache:             txscript.NewSigCache(cfg.SigCacheMaxSize),
	}

	// Create the transaction and address indexes if needed.
	//
	// CAUTION: the txindex needs to be first in the indexes array because
	// the addrindex uses data from the txindex during catchup.  If the
	// addrindex is run first, it may not have the transactions from the
	// current block indexed.
	var indexes []indexers.Indexer
	if cfg.TxIndex || cfg.AddrIndex {
		// Enable transaction index if address index is enabled since it
		// requires it.
		if !cfg.TxIndex {
			indxLog.Infof("Transaction index enabled because it " +
				"is required by the address index")
			cfg.TxIndex = true
		} else {
			indxLog.Info("Transaction index is enabled")
		}

		s.txIndex = indexers.NewTxIndex(db)
		indexes = append(indexes, s.txIndex)
	}
	if cfg.AddrIndex {
		indxLog.Info("Address index is enabled")
		s.addrIndex = indexers.NewAddrIndex(db, chainParams)
		indexes = append(indexes, s.addrIndex)
	}

	// Create an index manager if any of the optional indexes are enabled.
	var indexManager blockchain.IndexManager
	if len(indexes) > 0 {
		indexManager = indexers.NewManager(db, indexes)
	}
	bm, err := newBlockManager(&s, indexManager)
	if err != nil {
		return nil, err
	}
	s.blockManager = bm

	txC := mempoolConfig{
		Policy: mempoolPolicy{
			DisableRelayPriority: cfg.NoRelayPriority,
			FreeTxRelayLimit:     cfg.FreeTxRelayLimit,
			MaxOrphanTxs:         cfg.MaxOrphanTxs,
			MaxOrphanTxSize:      defaultMaxOrphanTxSize,
			MaxSigOpsPerTx:       blockchain.MaxSigOpsPerBlock / 5,
			MinRelayTxFee:        cfg.minRelayTxFee,
		},
		FetchUtxoView: s.blockManager.chain.FetchUtxoView,
		Chain:         s.blockManager.chain,
		SigCache:      s.sigCache,
		TimeSource:    s.timeSource,
		AddrIndex:     s.addrIndex,
	}
	s.txMemPool = newTxMemPool(&txC)

	// Create the mining policy based on the configuration options.
	// NOTE: The CPU miner relies on the mempool, so the mempool has to be
	// created before calling the function to create the CPU miner.
	policy := mining.Policy{
		BlockMinSize:      cfg.BlockMinSize,
		BlockMaxSize:      cfg.BlockMaxSize,
		BlockPrioritySize: cfg.BlockPrioritySize,
		TxMinFreeFee:      cfg.minRelayTxFee,
	}
	s.cpuMiner = newCPUMiner(&policy, &s)

	if !cfg.DisableRPC {
		s.rpcServer, err = newRPCServer(cfg.RPCListeners, &policy, &s)
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
