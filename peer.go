// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"container/list"
	"fmt"
	"github.com/conformal/btcchain"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/conformal/go-socks"
	"github.com/davecgh/go-spew/spew"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// outputBufferSize is the number of elements the output channels use.
	outputBufferSize = 50

	// invTrickleSize is the maximum amount of inventory to send in a single
	// message when trickling inventory to remote peers.
	maxInvTrickleSize = 1000

	// maxKnownInventory is the maximum number of items to keep in the known
	// inventory cache.
	maxKnownInventory = 20000

	// idleTimeoutMinutes is the number of minutes of inactivity before
	// we time out a peer.
	idleTimeoutMinutes = 5

	// pingTimeoutMinutes is the number of minutes since we last sent a
	// message requiring a reply before we will ping a host.
	pingTimeoutMinutes = 2
)

// userAgent is the user agent string used to identify ourselves to other
// bitcoin peers.
var userAgent = fmt.Sprintf("/btcd:%d.%d.%d/", appMajor, appMinor, appPatch)

// zeroHash is the zero value hash (all zeros).  It is defined as a convenience.
var zeroHash btcwire.ShaHash

// minUint32 is a helper function to return the minimum of two uint32s.
// This avoids a math import and the need to cast to floats.
func minUint32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

// newNetAddress attempts to extract the IP address and port from the passed
// net.Addr interface and create a bitcoin NetAddress structure using that
// information.
func newNetAddress(addr net.Addr, services btcwire.ServiceFlag) (*btcwire.NetAddress, error) {
	// addr will be a net.TCPAddr when not using a proxy.
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		ip := tcpAddr.IP
		port := uint16(tcpAddr.Port)
		na := btcwire.NewNetAddressIPPort(ip, port, services)
		return na, nil
	}

	// addr will be a socks.ProxiedAddr when using a proxy.
	if proxiedAddr, ok := addr.(*socks.ProxiedAddr); ok {
		ip := net.ParseIP(proxiedAddr.Host)
		if ip == nil {
			ip = net.ParseIP("0.0.0.0")
		}
		port := uint16(proxiedAddr.Port)
		na := btcwire.NewNetAddressIPPort(ip, port, services)
		return na, nil
	}

	// For the most part, addr should be one of the two above cases, but
	// to be safe, fall back to trying to parse the information from the
	// address string as a last resort.
	host, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}
	na := btcwire.NewNetAddressIPPort(ip, uint16(port), services)
	return na, nil
}

type outMsg struct {
	msg      btcwire.Message
	doneChan chan bool
}

// peer provides a bitcoin peer for handling bitcoin communications.
type peer struct {
	server             *server
	protocolVersion    uint32
	btcnet             btcwire.BitcoinNet
	services           btcwire.ServiceFlag
	started            int32
	conn               net.Conn
	addr               string
	na                 *btcwire.NetAddress
	timeConnected      time.Time
	lastSend           time.Time
	lastRecv           time.Time
	inbound            bool
	connected          int32
	disconnect         int32 // only to be used atomically
	persistent         bool
	versionKnown       bool
	knownAddresses     map[string]bool
	knownInventory     *MruInventoryMap
	knownInvMutex      sync.Mutex
	requestedTxns      map[btcwire.ShaHash]bool // owned by blockmanager.
	requestedBlocks    map[btcwire.ShaHash]bool // owned by blockmanager.
	lastBlock          int32
	retrycount         int64
	prevGetBlocksBegin *btcwire.ShaHash // owned by blockmanager.
	prevGetBlocksStop  *btcwire.ShaHash // owned by blockmaanger.
	requestQueue       *list.List
	invSendQueue       *list.List
	continueHash       *btcwire.ShaHash
	outputQueue        chan outMsg
	outputInvChan      chan *btcwire.InvVect
	txProcessed        chan bool
	blockProcessed     chan bool
	quit               chan bool
	userAgent          string
}

// String returns the peer's address and directionality as a human-readable
// string.
func (p *peer) String() string {
	return fmt.Sprintf("%s (%s)", p.addr, directionString(p.inbound))
}

// isKnownInventory returns whether or not the peer is known to have the passed
// inventory.  It is safe for concurrent access.
func (p *peer) isKnownInventory(invVect *btcwire.InvVect) bool {
	p.knownInvMutex.Lock()
	defer p.knownInvMutex.Unlock()

	if p.knownInventory.Exists(invVect) {
		return true
	}
	return false
}

// addKnownInventory adds the passed inventory to the cache of known inventory
// for the peer.  It is safe for concurrent access.
func (p *peer) addKnownInventory(invVect *btcwire.InvVect) {
	p.knownInvMutex.Lock()
	defer p.knownInvMutex.Unlock()

	p.knownInventory.Add(invVect)
}

// pushVersionMsg sends a version message to the connected peer using the
// current state.
func (p *peer) pushVersionMsg() error {
	_, blockNum, err := p.server.db.NewestSha()
	if err != nil {
		return err
	}

	theirNa := p.na

	// If we are behind a proxy and the connection comes from the proxy then
	// we return an unroutable address as their address. This is to prevent
	// leaking the tor proxy address.
	if cfg.Proxy != "" {
		proxyaddress, _, err := net.SplitHostPort(cfg.Proxy)
		// invalid proxy means poorly configured, be on the safe side.
		if err != nil || p.na.IP.String() == proxyaddress {
			theirNa = &btcwire.NetAddress{
				Timestamp: time.Now(),
				IP:        net.IP([]byte{0, 0, 0, 0}),
			}
		}
	}

	// Version message.
	msg := btcwire.NewMsgVersion(
		p.server.addrManager.getBestLocalAddress(p.na), theirNa, p.server.nonce,
		userAgent, int32(blockNum))

	// XXX: bitcoind appears to always enable the full node services flag
	// of the remote peer netaddress field in the version message regardless
	// of whether it knows it supports it or not.  Also, bitcoind sets
	// the services field of the local peer to 0 regardless of support.
	//
	// Realistically, this should be set as follows:
	// - For outgoing connections:
	//    - Set the local netaddress services to what the local peer
	//      actually supports
	//    - Set the remote netaddress services to 0 to indicate no services
	//      as they are still unknown
	// - For incoming connections:
	//    - Set the local netaddress services to what the local peer
	//      actually supports
	//    - Set the remote netaddress services to the what was advertised by
	//      by the remote peer in its version message
	msg.AddrYou.Services = btcwire.SFNodeNetwork

	// Advertise that we're a full node.
	msg.Services = btcwire.SFNodeNetwork

	p.QueueMessage(msg, nil)
	return nil
}

// handleVersionMsg is invoked when a peer receives a version bitcoin message
// and is used to negotiate the protocol version details as well as kick start
// the communications.
func (p *peer) handleVersionMsg(msg *btcwire.MsgVersion) {
	// Detect self connections.
	if msg.Nonce == p.server.nonce {
		peerLog.Debugf("Disconnecting peer connected to self %s",
			p.addr)
		p.Disconnect()
		return
	}

	// Limit to one version message per peer.
	if p.versionKnown {
		p.logError("PEER: Only one version message per peer is allowed %s.",
			p.addr)
		p.Disconnect()
		return
	}

	// Negotiate the protocol version.
	p.protocolVersion = minUint32(p.protocolVersion, uint32(msg.ProtocolVersion))
	p.versionKnown = true
	peerLog.Debugf("Negotiated protocol version %d for peer %s",
		p.protocolVersion, p.addr)
	p.lastBlock = msg.LastBlock

	// Set the supported services for the peer to what the remote peer
	// advertised.
	p.services = msg.Services

	// Set the remote peer's user agent.
	p.userAgent = msg.UserAgent

	// Inbound connections.
	if p.inbound {
		// Set up a NetAddress for the peer to be used with AddrManager.
		// We only do this inbound because outbound set this up
		// at connection time and no point recomputing.
		na, err := newNetAddress(p.conn.RemoteAddr(), p.services)
		if err != nil {
			p.logError("Can't get remote address: %v", err)
			p.Disconnect()
			return
		}
		p.na = na

		// Send version.
		err = p.pushVersionMsg()
		if err != nil {
			p.logError("Can't send version message: %v", err)
			p.Disconnect()
			return
		}
	}

	// Send verack.
	p.QueueMessage(btcwire.NewMsgVerAck(), nil)

	// Outbound connections.
	if !p.inbound {
		// TODO(davec): Only do this if not doing the initial block
		// download and the local address is routable.
		if !cfg.DisableListen /* && isCurrent? */ {
			// get address that best matches. p.na
			lna := p.server.addrManager.getBestLocalAddress(p.na)
			if Routable(lna) {
				addresses := []*btcwire.NetAddress{lna}
				p.pushAddrMsg(addresses)
			}
		}

		// Request known addresses if the server address manager needs
		// more and the peer has a protocol version new enough to
		// include a timestamp with addresses.
		hasTimestamp := p.protocolVersion >= btcwire.NetAddressTimeVersion
		if p.server.addrManager.NeedMoreAddresses() && hasTimestamp {
			p.QueueMessage(btcwire.NewMsgGetAddr(), nil)
		}

		// Mark the address as a known good address.
		p.server.addrManager.Good(p.na)
	} else {
		// A peer might not be advertising the same address that it
		// actually connected from.  One example of why this can happen
		// is with NAT.  Only add the address to the address manager if
		// the addresses agree.
		if NetAddressKey(&msg.AddrMe) == NetAddressKey(p.na) {
			p.server.addrManager.AddAddress(p.na, p.na)
			p.server.addrManager.Good(p.na)
		}
	}

	// Signal the block manager this peer is a new sync candidate.
	p.server.blockManager.NewPeer(p)

	// TODO: Relay alerts.
}

// pushTxMsg sends a tx message for the provided transaction hash to the
// connected peer.  An error is returned if the transaction hash is not known.
func (p *peer) pushTxMsg(sha *btcwire.ShaHash, doneChan chan bool) error {
	// Attempt to fetch the requested transaction from the pool.  A
	// call could be made to check for existence first, but simply trying
	// to fetch a missing transaction results in the same behavior.
	tx, err := p.server.txMemPool.FetchTransaction(sha)
	if err != nil {
		peerLog.Tracef("Unable to fetch tx %v from transaction "+
			"pool: %v", sha, err)
		return err
	}
	p.QueueMessage(tx.MsgTx(), doneChan)

	return nil
}

// pushBlockMsg sends a block message for the provided block hash to the
// connected peer.  An error is returned if the block hash is not known.
func (p *peer) pushBlockMsg(sha *btcwire.ShaHash, doneChan chan bool) error {
	// What should this function do about the rate limiting the
	// number of blocks queued for this peer?
	// Current thought is have a counting mutex in the peer
	// such that if > N Tx/Block requests are currently in
	// the tx queue, wait until the mutex clears allowing more to be
	// sent. This prevents 500 1+MB blocks from being loaded into
	// memory and sit around until the output queue drains.
	// Actually the outputQueue has a limit of 50 in its queue
	// but still 50MB to 1.6GB(50 32MB blocks) just setting
	// in memory waiting to be sent is pointless.
	// I would recommend a getdata request limit of about 5
	// outstanding objects.
	// Should the tx complete api be a mutex or channel?

	blk, err := p.server.db.FetchBlockBySha(sha)
	if err != nil {
		peerLog.Tracef("Unable to fetch requested block sha %v: %v",
			sha, err)
		return err
	}

	// We only send the channel for this message if we aren't sending
	// an inv straight after.
	var dc chan bool
	sendInv := p.continueHash != nil && p.continueHash.IsEqual(sha)
	if !sendInv {
		dc = doneChan
	}
	p.QueueMessage(blk.MsgBlock(), dc)

	// When the peer requests the final block that was advertised in
	// response to a getblocks message which requested more blocks than
	// would fit into a single message, send it a new inventory message
	// to trigger it to issue another getblocks message for the next
	// batch of inventory.
	if p.continueHash != nil && p.continueHash.IsEqual(sha) {
		hash, _, err := p.server.db.NewestSha()
		if err == nil {
			invMsg := btcwire.NewMsgInv()
			iv := btcwire.NewInvVect(btcwire.InvTypeBlock, hash)
			invMsg.AddInvVect(iv)
			p.QueueMessage(invMsg, doneChan)
			p.continueHash = nil
		} else if doneChan != nil {
			// Avoid deadlock when caller waits on channel.
			go func() {
				doneChan <- false
			}()
		}
	}
	return nil
}

// PushGetBlocksMsg sends a getblocks message for the provided block locator
// and stop hash.  It will ignore back-to-back duplicate requests.
func (p *peer) PushGetBlocksMsg(locator btcchain.BlockLocator, stopHash *btcwire.ShaHash) error {
	// Extract the begin hash from the block locator, if one was specified,
	// to use for filtering duplicate getblocks requests.
	// request.
	var beginHash *btcwire.ShaHash
	if len(locator) > 0 {
		beginHash = locator[0]
	}

	// Filter duplicate getblocks requests.
	if p.prevGetBlocksStop != nil && p.prevGetBlocksBegin != nil &&
		beginHash != nil && stopHash.IsEqual(p.prevGetBlocksStop) &&
		beginHash.IsEqual(p.prevGetBlocksBegin) {

		peerLog.Tracef("Filtering duplicate [getblocks] with begin "+
			"hash %v, stop hash %v", beginHash, stopHash)
		return nil
	}

	// Construct the getblocks request and queue it to be sent.
	msg := btcwire.NewMsgGetBlocks(stopHash)
	for _, hash := range locator {
		err := msg.AddBlockLocatorHash(hash)
		if err != nil {
			return err
		}
	}
	p.QueueMessage(msg, nil)

	// Update the previous getblocks request information for filtering
	// duplicates.
	p.prevGetBlocksBegin = beginHash
	p.prevGetBlocksStop = stopHash
	return nil
}

// handleMemPoolMsg is invoked when a peer receives a mempool bitcoin message.
// It creates and sends an inventory message with the contents of the memory
// pool up to the maximum inventory allowed per message.
func (p *peer) handleMemPoolMsg(msg *btcwire.MsgMemPool) {
	// Generate inventory message with the available transactions in the
	// transaction memory pool.  Limit it to the max allowed inventory
	// per message.
	invMsg := btcwire.NewMsgInv()
	hashes := p.server.txMemPool.TxShas()
	for i, hash := range hashes {
		// Another thread might have removed the transaction from the
		// pool since the initial query.
		if !p.server.txMemPool.IsTransactionInPool(hash) {
			continue
		}

		iv := btcwire.NewInvVect(btcwire.InvTypeTx, hash)
		invMsg.AddInvVect(iv)
		if i+1 >= btcwire.MaxInvPerMsg {
			break
		}
	}

	// Send the inventory message if there is anything to send.
	if len(invMsg.InvList) > 0 {
		p.QueueMessage(invMsg, nil)
	}
}

// handleTxMsg is invoked when a peer receives a tx bitcoin message.  It blocks
// until the bitcoin transaction has been fully processed.  Unlock the block
// handler this does not serialize all transactions through a single thread
// transactions don't rely on the previous one in a linear fashion like blocks.
func (p *peer) handleTxMsg(msg *btcwire.MsgTx) {
	// Add the transaction to the known inventory for the peer.
	// Convert the raw MsgTx to a btcutil.Tx which provides some convenience
	// methods and things such as hash caching.
	tx := btcutil.NewTx(msg)
	iv := btcwire.NewInvVect(btcwire.InvTypeTx, tx.Sha())
	p.addKnownInventory(iv)

	// Queue the transaction up to be handled by the block manager and
	// intentionally block further receives until the transaction is fully
	// processed and known good or bad.  This helps prevent a malicious peer
	// from queueing up a bunch of bad transactions before disconnecting (or
	// being disconnected) and wasting memory.
	p.server.blockManager.QueueTx(tx, p)
	<-p.txProcessed
}

// handleBlockMsg is invoked when a peer receives a block bitcoin message.  It
// blocks until the bitcoin block has been fully processed.
func (p *peer) handleBlockMsg(msg *btcwire.MsgBlock, buf []byte) {
	// Convert the raw MsgBlock to a btcutil.Block which provides some
	// convenience methods and things such as hash caching.
	block := btcutil.NewBlockFromBlockAndBytes(msg, buf)

	// Add the block to the known inventory for the peer.
	hash, err := block.Sha()
	if err != nil {
		peerLog.Errorf("Unable to get block hash: %v", err)
		return
	}
	iv := btcwire.NewInvVect(btcwire.InvTypeBlock, hash)
	p.addKnownInventory(iv)

	// Queue the block up to be handled by the block
	// manager and intentionally block further receives
	// until the bitcoin block is fully processed and known
	// good or bad.  This helps prevent a malicious peer
	// from queueing up a bunch of bad blocks before
	// disconnecting (or being disconnected) and wasting
	// memory.  Additionally, this behavior is depended on
	// by at least the block acceptance test tool as the
	// reference implementation processes blocks in the same
	// thread and therefore blocks further messages until
	// the bitcoin block has been fully processed.
	p.server.blockManager.QueueBlock(block, p)
	<-p.blockProcessed
}

// handleInvMsg is invoked when a peer receives an inv bitcoin message and is
// used to examine the inventory being advertised by the remote peer and react
// accordingly. We pass the message down to blockmanager which will call
// QueueMessage with any appropriate responses.
func (p *peer) handleInvMsg(msg *btcwire.MsgInv) {
	p.server.blockManager.QueueInv(msg, p)
}

// handleGetData is invoked when a peer receives a getdata bitcoin message and
// is used to deliver block and transaction information.
func (p *peer) handleGetDataMsg(msg *btcwire.MsgGetData) {
	notFound := btcwire.NewMsgNotFound()

	doneChan := make(chan bool)
out:
	for i, iv := range msg.InvList {
		var c chan bool
		// If this will be the last message we send.
		if i == len(msg.InvList)-1 && len(notFound.InvList) == 0 {
			c = doneChan
		}
		var err error
		switch iv.Type {
		case btcwire.InvTypeTx:
			err = p.pushTxMsg(&iv.Hash, c)
		case btcwire.InvTypeBlock:
			err = p.pushBlockMsg(&iv.Hash, c)
		default:
			peerLog.Warnf("Unknown type in inventory request %d",
				iv.Type)
			break out
		}
		if err != nil {
			notFound.AddInvVect(iv)
		}
	}
	if len(notFound.InvList) != 0 {
		p.QueueMessage(notFound, doneChan)
	}

	// Wait for messages to be sent. We can send quite a lot of data at this
	// point and this will keep the peer busy for a decent amount of time.
	// We don't process anything else by them in this time so that we
	// have an idea of when we should hear back from them - else the idle
	// timeout could fire when we were only half done sending the blocks.
	<-doneChan
}

// handleGetBlocksMsg is invoked when a peer receives a getdata bitcoin message.
func (p *peer) handleGetBlocksMsg(msg *btcwire.MsgGetBlocks) {
	// Return all block hashes to the latest one (up to max per message) if
	// no stop hash was specified.
	// Attempt to find the ending index of the stop hash if specified.
	endIdx := btcdb.AllShas
	if !msg.HashStop.IsEqual(&zeroHash) {
		block, err := p.server.db.FetchBlockBySha(&msg.HashStop)
		if err == nil {
			endIdx = block.Height() + 1
		}
	}

	// Find the most recent known block based on the block locator.
	// Use the block after the genesis block if no other blocks in the
	// provided locator are known.  This does mean the client will start
	// over with the genesis block if unknown block locators are provided.
	// This mirrors the behavior in the reference implementation.
	startIdx := int64(1)
	for _, hash := range msg.BlockLocatorHashes {
		block, err := p.server.db.FetchBlockBySha(hash)
		if err == nil {
			// Start with the next hash since we know this one.
			startIdx = block.Height() + 1
			break
		}
	}

	// Don't attempt to fetch more than we can put into a single message.
	autoContinue := false
	if endIdx-startIdx > btcwire.MaxBlocksPerMsg {
		endIdx = startIdx + btcwire.MaxBlocksPerMsg
		autoContinue = true
	}

	// Generate inventory message.
	//
	// The FetchBlockBySha call is limited to a maximum number of hashes
	// per invocation.  Since the maximum number of inventory per message
	// might be larger, call it multiple times with the appropriate indices
	// as needed.
	invMsg := btcwire.NewMsgInv()
	for start := startIdx; start < endIdx; {
		// Fetch the inventory from the block database.
		hashList, err := p.server.db.FetchHeightRange(start, endIdx)
		if err != nil {
			peerLog.Warnf("Block lookup failed: %v", err)
			return
		}

		// The database did not return any further hashes.  Break out of
		// the loop now.
		if len(hashList) == 0 {
			break
		}

		// Add block inventory to the message.
		for _, hash := range hashList {
			hashCopy := hash
			iv := btcwire.NewInvVect(btcwire.InvTypeBlock, &hashCopy)
			invMsg.AddInvVect(iv)
		}
		start += int64(len(hashList))
	}

	// Send the inventory message if there is anything to send.
	if len(invMsg.InvList) > 0 {
		invListLen := len(invMsg.InvList)
		if autoContinue && invListLen == btcwire.MaxBlocksPerMsg {
			// Intentionally use a copy of the final hash so there
			// is not a reference into the inventory slice which
			// would prevent the entire slice from being eligible
			// for GC as soon as it's sent.
			continueHash := invMsg.InvList[invListLen-1].Hash
			p.continueHash = &continueHash
		}
		p.QueueMessage(invMsg, nil)
	}
}

// handleGetHeadersMsg is invoked when a peer receives a getheaders bitcoin
// message.
func (p *peer) handleGetHeadersMsg(msg *btcwire.MsgGetHeaders) {
	// Attempt to look up the height of the provided stop hash.
	endIdx := btcdb.AllShas
	block, err := p.server.db.FetchBlockBySha(&msg.HashStop)
	if err == nil {
		endIdx = block.Height() + 1
	}

	// There are no block locators so a specific header is being requested
	// as identified by the stop hash.
	if len(msg.BlockLocatorHashes) == 0 {
		// No blocks with the stop hash were found so there is nothing
		// to do.  Just return.  This behavior mirrors the reference
		// implementation.
		if endIdx == btcdb.AllShas {
			return
		}

		// Send the requested block header.
		headersMsg := btcwire.NewMsgHeaders()
		hdr := block.MsgBlock().Header // copy
		hdr.TxnCount = 0
		headersMsg.AddBlockHeader(&hdr)
		p.QueueMessage(headersMsg, nil)
		return
	}

	// Find the most recent known block based on the block locator.
	// Use the block after the genesis block if no other blocks in the
	// provided locator are known.  This does mean the client will start
	// over with the genesis block if unknown block locators are provided.
	// This mirrors the behavior in the reference implementation.
	startIdx := int64(1)
	for _, hash := range msg.BlockLocatorHashes {
		block, err := p.server.db.FetchBlockBySha(hash)
		if err == nil {
			// Start with the next hash since we know this one.
			startIdx = block.Height() + 1
			break
		}
	}

	// Don't attempt to fetch more than we can put into a single message.
	if endIdx-startIdx > btcwire.MaxBlockHeadersPerMsg {
		endIdx = startIdx + btcwire.MaxBlockHeadersPerMsg
	}

	// Generate headers message and send it.
	//
	// The FetchBlockBySha call is limited to a maximum number of hashes
	// per invocation.  Since the maximum number of headers per message
	// might be larger, call it multiple times with the appropriate indices
	// as needed.
	headersMsg := btcwire.NewMsgHeaders()
	for start := startIdx; start < endIdx; {
		// Fetch the inventory from the block database.
		hashList, err := p.server.db.FetchHeightRange(start, endIdx)
		if err != nil {
			peerLog.Warnf("Header lookup failed: %v", err)
			return
		}

		// The database did not return any further hashes.  Break out of
		// the loop now.
		if len(hashList) == 0 {
			break
		}

		// Add headers to the message.
		for _, hash := range hashList {
			block, err := p.server.db.FetchBlockBySha(&hash)
			if err != nil {
				peerLog.Warnf("Lookup of known block hash "+
					"failed: %v", err)
				continue
			}
			hdr := block.MsgBlock().Header // copy
			hdr.TxnCount = 0
			headersMsg.AddBlockHeader(&hdr)
		}

		// Start at the next block header after the latest one on the
		// next loop iteration.
		start += int64(len(hashList))
	}
	p.QueueMessage(headersMsg, nil)
}

// handleGetAddrMsg is invoked when a peer receives a getaddr bitcoin message
// and is used to provide the peer with known addresses from the address
// manager.
func (p *peer) handleGetAddrMsg(msg *btcwire.MsgGetAddr) {
	// Get the current known addresses from the address manager.
	addrCache := p.server.addrManager.AddressCache()

	// Push the addresses.
	err := p.pushAddrMsg(addrCache)
	if err != nil {
		p.logError("Can't push address message: %v", err)
		p.Disconnect()
		return
	}
}

// pushAddrMsg sends one, or more, addr message(s) to the connected peer using
// the provided addresses.
func (p *peer) pushAddrMsg(addresses []*btcwire.NetAddress) error {
	// Nothing to send.
	if len(addresses) == 0 {
		return nil
	}

	numAdded := 0
	msg := btcwire.NewMsgAddr()
	for _, na := range addresses {
		// Filter addresses the peer already knows about.
		if p.knownAddresses[NetAddressKey(na)] {
			continue
		}

		// Add the address to the message.
		err := msg.AddAddress(na)
		if err != nil {
			return err
		}
		numAdded++

		// Split into multiple messages as needed.
		if numAdded > 0 && numAdded%btcwire.MaxAddrPerMsg == 0 {
			p.QueueMessage(msg, nil)

			// NOTE: This needs to be a new address message and not
			// simply call ClearAddresses since the message is a
			// pointer and queueing it does not make a copy.
			msg = btcwire.NewMsgAddr()
		}
	}

	// Send message with remaining addresses if needed.
	if numAdded%btcwire.MaxAddrPerMsg != 0 {
		p.QueueMessage(msg, nil)
	}
	return nil
}

// handleAddrMsg is invoked when a peer receives an addr bitcoin message and
// is used to notify the server about advertised addresses.
func (p *peer) handleAddrMsg(msg *btcwire.MsgAddr) {
	// Ignore old style addresses which don't include a timestamp.
	if p.protocolVersion < btcwire.NetAddressTimeVersion {
		return
	}

	// A message that has no addresses is invalid.
	if len(msg.AddrList) == 0 {
		p.logError("Command [%s] from %s does not contain any addresses",
			msg.Command(), p.addr)
		p.Disconnect()
		return
	}

	for _, na := range msg.AddrList {
		// Don't add more address if we're disconnecting.
		if atomic.LoadInt32(&p.disconnect) != 0 {
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
		p.knownAddresses[NetAddressKey(na)] = true
	}

	// Add addresses to server address manager.  The address manager handles
	// the details of things such as preventing duplicate addresses, max
	// addresses, and last seen updates.
	// XXX bitcoind gives a 2 hour time penalty here, do we want to do the
	// same?
	p.server.addrManager.AddAddresses(msg.AddrList, p.na)
}

// handlePingMsg is invoked when a peer receives a ping bitcoin message.  For
// recent clients (protocol version > BIP0031Version), it replies with a pong
// message.  For older clients, it does nothing and anything other than failure
// is considered a successful ping.
func (p *peer) handlePingMsg(msg *btcwire.MsgPing) {
	// Only Reply with pong is message comes from a new enough client.
	if p.protocolVersion > btcwire.BIP0031Version {
		// Include nonce from ping so pong can be identified.
		p.QueueMessage(btcwire.NewMsgPong(msg.Nonce), nil)
	}
}

// readMessage reads the next bitcoin message from the peer with logging.
func (p *peer) readMessage() (msg btcwire.Message, buf []byte, err error) {
	msg, buf, err = btcwire.ReadMessage(p.conn, p.protocolVersion, p.btcnet)
	if err != nil {
		return
	}

	// Use closures to log expensive operations so they are only run when
	// the logging level requires it.
	peerLog.Debugf("%v", newLogClosure(func() string {
		// Debug summary of message.
		summary := messageSummary(msg)
		if len(summary) > 0 {
			summary = " (" + summary + ")"
		}
		return fmt.Sprintf("Received %v%s from %s",
			msg.Command(), summary, p.addr)
	}))
	peerLog.Tracef("%v", newLogClosure(func() string {
		return spew.Sdump(msg)
	}))
	peerLog.Tracef("%v", newLogClosure(func() string {
		return spew.Sdump(buf)
	}))

	return
}

// writeMessage sends a bitcoin Message to the peer with logging.
func (p *peer) writeMessage(msg btcwire.Message) {
	// Don't do anything if we're disconnecting.
	if atomic.LoadInt32(&p.disconnect) != 0 {
		return
	}
	if !p.versionKnown {
		switch msg.(type) {
		case *btcwire.MsgVersion:
			// This is OK.
		default:
			// We drop all messages other than version if we
			// haven't done the handshake already.
			return
		}
	}

	// Use closures to log expensive operations so they are only run when
	// the logging level requires it.
	peerLog.Debugf("%v", newLogClosure(func() string {
		// Debug summary of message.
		summary := messageSummary(msg)
		if len(summary) > 0 {
			summary = " (" + summary + ")"
		}
		return fmt.Sprintf("Sending %v%s to %s", msg.Command(),
			summary, p.addr)
	}))
	peerLog.Tracef("%v", newLogClosure(func() string {
		return spew.Sdump(msg)
	}))
	peerLog.Tracef("%v", newLogClosure(func() string {
		var buf bytes.Buffer
		err := btcwire.WriteMessage(&buf, msg, p.protocolVersion, p.btcnet)
		if err != nil {
			return err.Error()
		}
		return spew.Sdump(buf.Bytes())
	}))

	// Write the message to the peer.
	err := btcwire.WriteMessage(p.conn, msg, p.protocolVersion, p.btcnet)
	if err != nil {
		p.Disconnect()
		p.logError("Can't send message: %v", err)
		return
	}
}

// isAllowedByRegression returns whether or not the passed error is allowed by
// regression tests without disconnecting the peer.  In particular, regression
// tests need to be allowed to send malformed messages without the peer being
// disconnected.
func (p *peer) isAllowedByRegression(err error) bool {
	// Don't allow the error if it's not specifically a malformed message
	// error.
	if _, ok := err.(*btcwire.MessageError); !ok {
		return false
	}

	// Don't allow the error if it's not coming from localhost or the
	// hostname can't be determined for some reason.
	host, _, err := net.SplitHostPort(p.addr)
	if err != nil {
		return false
	}

	if host != "127.0.0.1" && host != "localhost" {
		return false
	}

	// Allowed if all checks passed.
	return true
}

// inHandler handles all incoming messages for the peer.  It must be run as a
// goroutine.
func (p *peer) inHandler() {
	idleTimer := time.AfterFunc(idleTimeoutMinutes*time.Minute, func() {
		// XXX technically very very very slightly racy, doesn't really
		// matter.
		if p.versionKnown {
			peerLog.Warnf("Peer %s no answer for %d minutes, "+
				"disconnecting", p, idleTimeoutMinutes)
		}
		p.Disconnect()
	})
out:
	for atomic.LoadInt32(&p.disconnect) == 0 {
		rmsg, buf, err := p.readMessage()
		// Stop the timer now, if we go around again we will reset it.
		idleTimer.Stop()
		if err != nil {
			// In order to allow regression tests with malformed
			// messages, don't disconnect the peer when we're in
			// regression test mode and the error is one of the
			// allowed errors.
			if cfg.RegressionTest && p.isAllowedByRegression(err) {
				peerLog.Errorf("Allowed regression test "+
					"error: %v", err)
				idleTimer.Reset(idleTimeoutMinutes * time.Minute)
				continue
			}

			// Only log the error if we're not forcibly disconnecting.
			if atomic.LoadInt32(&p.disconnect) == 0 {
				p.logError("PEER: Can't read message: %v", err)
			}
			break out
		}
		p.lastRecv = time.Now()

		// Ensure version message comes first.
		if _, ok := rmsg.(*btcwire.MsgVersion); !ok && !p.versionKnown {
			p.logError("PEER: A version message must precede all others")
			break out
		}

		// Handle each supported message type.
		markConnected := false
		switch msg := rmsg.(type) {
		case *btcwire.MsgVersion:
			p.handleVersionMsg(msg)
			markConnected = true

		case *btcwire.MsgVerAck:
			// Do nothing.

		case *btcwire.MsgGetAddr:
			p.handleGetAddrMsg(msg)

		case *btcwire.MsgAddr:
			p.handleAddrMsg(msg)
			markConnected = true

		case *btcwire.MsgPing:
			p.handlePingMsg(msg)
			markConnected = true

		case *btcwire.MsgPong:
			// Don't do anything, but could try to work out network
			// timing or similar.

		case *btcwire.MsgAlert:
			p.server.BroadcastMessage(msg, p)

		case *btcwire.MsgMemPool:
			p.handleMemPoolMsg(msg)

		case *btcwire.MsgTx:
			p.handleTxMsg(msg)

		case *btcwire.MsgBlock:
			p.handleBlockMsg(msg, buf)

		case *btcwire.MsgInv:
			p.handleInvMsg(msg)
			markConnected = true

		case *btcwire.MsgNotFound:
			// TODO(davec): Ignore this for now, but ultimately
			// it should probably be used to detect when something
			// we requested needs to be re-requested from another
			// peer.

		case *btcwire.MsgGetData:
			p.handleGetDataMsg(msg)
			markConnected = true

		case *btcwire.MsgGetBlocks:
			p.handleGetBlocksMsg(msg)

		case *btcwire.MsgGetHeaders:
			p.handleGetHeadersMsg(msg)

		default:
			peerLog.Debugf("Received unhandled message of type %v: Fix Me",
				rmsg.Command())
		}

		// Mark the address as currently connected and working as of
		// now if one of the messages that trigger it was processed.
		if markConnected && atomic.LoadInt32(&p.disconnect) == 0 {
			if p.na == nil {
				peerLog.Warnf("we're getting stuff before we " +
					"got a version message. that's bad")
				continue
			}
			p.server.addrManager.Connected(p.na)
		}
		// ok we got a message, reset the timer.
		// timer just calls p.Disconnect() after logging.
		idleTimer.Reset(idleTimeoutMinutes * time.Minute)
	}

	idleTimer.Stop()

	// Ensure connection is closed and notify server and block manager that
	// the peer is done.
	p.Disconnect()
	p.server.donePeers <- p
	// Only tell blockmanager we are gone if we ever told it we existed.
	if p.versionKnown {
		p.server.blockManager.DonePeer(p)
	}

	peerLog.Tracef("Peer input handler done for %s", p.addr)
}

// outHandler handles all outgoing messages for the peer.  It must be run as a
// goroutine.  It uses a buffered channel to serialize output messages while
// allowing the sender to continue running asynchronously.
func (p *peer) outHandler() {
	trickleTicker := time.NewTicker(time.Second * 10)
	pingTimer := time.AfterFunc(pingTimeoutMinutes*time.Minute, func() {
		nonce, err := btcwire.RandomUint64()
		if err != nil {
			peerLog.Errorf("Not sending ping on timeout to %s: %v",
				p, err)
			return
		}
		p.QueueMessage(btcwire.NewMsgPing(nonce), nil)
	})
out:
	for {
		select {
		case msg := <-p.outputQueue:
			// If the message is one we should get a reply for
			// then reset the timer, we only want to send pings
			// when otherwise we would not recieve a reply from
			// the peer. We specifically do not count block or inv
			// messages here since they are not sure of a reply if
			// the inv is of no interest explicitly solicited invs
			// should elicit a reply but we don't track them
			// specially.
			reset := true
			switch msg.msg.(type) {
			case *btcwire.MsgVersion:
				// should get an ack
			case *btcwire.MsgGetAddr:
				// should get addresses
			case *btcwire.MsgPing:
				// expects pong
			case *btcwire.MsgMemPool:
				// Should return an inv.
			case *btcwire.MsgGetData:
				// Should get us block, tx, or not found.
			case *btcwire.MsgGetHeaders:
				// Should get us headers back.

			default:
				// Not one of the above, no sure reply.
				// We want to ping if nothing else
				// interesting happens.
				reset = false
			}
			if reset {
				pingTimer.Reset(pingTimeoutMinutes * time.Minute)
			}
			p.writeMessage(msg.msg)
			p.lastSend = time.Now()
			if msg.doneChan != nil {
				msg.doneChan <- true
			}

		case iv := <-p.outputInvChan:
			// No handshake? They'll find out soon enough.
			if p.versionKnown {
				p.invSendQueue.PushBack(iv)
			}

		case <-trickleTicker.C:
			// Don't send anything if we're disconnecting or there
			// is no queued inventory.
			if atomic.LoadInt32(&p.disconnect) != 0 ||
				p.invSendQueue.Len() == 0 ||
				!p.versionKnown {
				continue
			}

			// Create and send as many inv messages as needed to
			// drain the inventory send queue.
			invMsg := btcwire.NewMsgInv()
			for e := p.invSendQueue.Front(); e != nil; e = p.invSendQueue.Front() {
				iv := p.invSendQueue.Remove(e).(*btcwire.InvVect)

				// Don't send inventory that became known after
				// the initial check.
				if p.isKnownInventory(iv) {
					continue
				}

				invMsg.AddInvVect(iv)
				if len(invMsg.InvList) >= maxInvTrickleSize {
					p.writeMessage(invMsg)
					invMsg = btcwire.NewMsgInv()
				}

				// Add the inventory that is being relayed to
				// the known inventory for the peer.
				p.addKnownInventory(iv)
			}
			if len(invMsg.InvList) > 0 {
				p.writeMessage(invMsg)
			}

		case <-p.quit:
			break out
		}
	}

	pingTimer.Stop()

	// Drain any wait channels before we go away so we don't leave something
	// waiting for us.
cleanup:
	for {
		select {
		case msg := <-p.outputQueue:
			if msg.doneChan != nil {
				msg.doneChan <- false
			}
		default:
			break cleanup
		}
	}
	peerLog.Tracef("Peer output handler done for %s", p.addr)
}

// QueueMessage adds the passed bitcoin message to the peer send queue.  It
// uses a buffered channel to communicate with the output handler goroutine so
// it is automatically rate limited and safe for concurrent access.
func (p *peer) QueueMessage(msg btcwire.Message, doneChan chan bool) {
	p.outputQueue <- outMsg{msg: msg, doneChan: doneChan}
}

// QueueInventory adds the passed inventory to the inventory send queue which
// might not be sent right away, rather it is trickled to the peer in batches.
// Inventory that the peer is already known to have is ignored.  It is safe for
// concurrent access.
func (p *peer) QueueInventory(invVect *btcwire.InvVect) {
	// Don't add the inventory to the send queue if the peer is
	// already known to have it.
	if p.isKnownInventory(invVect) {
		return
	}

	p.outputInvChan <- invVect
}

// Connected returns whether or not the peer is currently connected.
func (p *peer) Connected() bool {
	return atomic.LoadInt32(&p.connected) != 0 &&
		atomic.LoadInt32(&p.disconnect) == 0
}

// Disconnect disconnects the peer by closing the connection.  It also sets
// a flag so the impending shutdown can be detected.
func (p *peer) Disconnect() {
	// did we win the race?
	if atomic.AddInt32(&p.disconnect, 1) != 1 {
		return
	}
	close(p.quit)
	if atomic.LoadInt32(&p.connected) != 0 {
		p.conn.Close()
	}
}

// Start begins processing input and output messages.  It also sends the initial
// version message for outbound connections to start the negotiation process.
func (p *peer) Start() error {
	// Already started?
	if atomic.AddInt32(&p.started, 1) != 1 {
		return nil
	}

	peerLog.Tracef("Starting peer %s", p.addr)

	// Send an initial version message if this is an outbound connection.
	if !p.inbound {
		err := p.pushVersionMsg()
		if err != nil {
			p.logError("PEER: Can't send outbound version "+
				"message %v", err)
			p.Disconnect()
			return err
		}
	}

	// Start processing input and output.
	go p.inHandler()
	go p.outHandler()

	return nil
}

// Shutdown gracefully shuts down the peer by disconnecting it.
func (p *peer) Shutdown() {
	peerLog.Tracef("Shutdown peer %s", p.addr)
	p.Disconnect()
}

// newPeerBase returns a new base bitcoin peer for the provided server and
// inbound flag.  This is used by the newInboundPeer and newOutboundPeer
// functions to perform base setup needed by both types of peers.
func newPeerBase(s *server, inbound bool) *peer {
	p := peer{
		server:          s,
		protocolVersion: btcwire.ProtocolVersion,
		btcnet:          s.btcnet,
		services:        btcwire.SFNodeNetwork,
		inbound:         inbound,
		knownAddresses:  make(map[string]bool),
		knownInventory:  NewMruInventoryMap(maxKnownInventory),
		requestedTxns:   make(map[btcwire.ShaHash]bool),
		requestedBlocks: make(map[btcwire.ShaHash]bool),
		requestQueue:    list.New(),
		invSendQueue:    list.New(),
		outputQueue:     make(chan outMsg, outputBufferSize),
		outputInvChan:   make(chan *btcwire.InvVect, outputBufferSize),
		txProcessed:     make(chan bool, 1),
		blockProcessed:  make(chan bool, 1),
		quit:            make(chan bool),
	}
	return &p
}

// newPeer returns a new inbound bitcoin peer for the provided server and
// connection.  Use Start to begin processing incoming and outgoing messages.
func newInboundPeer(s *server, conn net.Conn) *peer {
	p := newPeerBase(s, true)
	p.conn = conn
	p.addr = conn.RemoteAddr().String()
	p.timeConnected = time.Now()
	atomic.AddInt32(&p.connected, 1)
	return p
}

// newOutbountPeer returns a new outbound bitcoin peer for the provided server and
// address and connects to it asynchronously. If the connection is successful
// then the peer will also be started.
func newOutboundPeer(s *server, addr string, persistent bool) *peer {
	p := newPeerBase(s, false)
	p.addr = addr
	p.persistent = persistent

	// Setup p.na with a temporary address that we are connecting to with
	// faked up service flags.  We will replace this with the real one after
	// version negotiation is successful.  The only failure case here would
	// be if the string was incomplete for connection so can't be split
	// into address and port, and thus this would be invalid anyway.  In
	// which case we return nil to be handled by the caller.  This must be
	// done before we fork off the goroutine because as soon as this
	// function returns the peer must have a valid netaddress.
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		p.logError("Tried to create a new outbound peer with invalid "+
			"address %s: %v", addr, err)
		return nil
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		p.logError("Tried to create a new outbound peer with invalid "+
			"port %s: %v", portStr, err)
		return nil
	}

	p.na, err = hostToNetAddress(host, uint16(port), 0)
	if err != nil {
		p.logError("Can not turn host %s into netaddress: %v",
			host, err)
		return nil
	}

	go func() {
		// Select which dial method to call depending on whether or
		// not a proxy is configured.  Also, add proxy information to
		// logged address if needed.
		dial := net.Dial
		faddr := addr
		if cfg.Proxy != "" {
			proxy := &socks.Proxy{
				Addr:     cfg.Proxy,
				Username: cfg.ProxyUser,
				Password: cfg.ProxyPass,
			}
			dial = proxy.Dial
			faddr = fmt.Sprintf("%s via proxy %s", addr, cfg.Proxy)
		}

		// Attempt to connect to the peer.  If the connection fails and
		// this is a persistent connection, retry after the retry
		// interval.
		for atomic.LoadInt32(&p.disconnect) == 0 {
			srvrLog.Debugf("Attempting to connect to %s", faddr)
			conn, err := dial("tcp", addr)
			if err != nil {
				p.retrycount += 1
				srvrLog.Debugf("Failed to connect to %s: %v",
					faddr, err)
				if !persistent {
					p.server.donePeers <- p
					return
				}
				scaledInterval := connectionRetryInterval.Nanoseconds() * p.retrycount / 2
				scaledDuration := time.Duration(scaledInterval)
				srvrLog.Debugf("Retrying connection to %s in "+
					"%s", faddr, scaledDuration)
				time.Sleep(scaledDuration)
				continue
			}

			// While we were sleeping trying to connect, the server
			// may have scheduled a shutdown.  In that case ditch
			// the peer immediately.
			if atomic.LoadInt32(&p.disconnect) == 0 {
				p.timeConnected = time.Now()
				p.server.addrManager.Attempt(p.na)

				// Connection was successful so log it and start peer.
				srvrLog.Debugf("Connected to %s",
					conn.RemoteAddr())
				p.conn = conn
				atomic.AddInt32(&p.connected, 1)
				p.retrycount = 0
				p.Start()
			}

			return
		}
	}()
	return p
}

// logError makes sure that we only log errors loudly on user peers.
func (p *peer) logError(fmt string, args ...interface{}) {
	if p.persistent {
		peerLog.Errorf(fmt, args...)
	} else {
		peerLog.Debugf(fmt, args...)
	}
}
