// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"container/list"
	"errors"
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
	"time"
)

const (
	outputBufferSize = 50

	// invTrickleSize is the maximum amount of inventory to send in a single
	// message when trickling inventory to remote peers.
	maxInvTrickleSize = 1000

	// maxKnownInventory is the maximum number of items to keep in the known
	// inventory cache.
	maxKnownInventory = 20000
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

// peer provides a bitcoin peer for handling bitcoin communications.
type peer struct {
	server          *server
	protocolVersion uint32
	btcnet          btcwire.BitcoinNet
	services        btcwire.ServiceFlag
	started         bool
	conn            net.Conn
	timeConnected   time.Time
	inbound         bool
	disconnect      bool
	persistent      bool
	versionKnown    bool
	knownAddresses  map[string]bool
	knownInventory  *MruInventoryMap
	knownInvMutex   sync.Mutex
	lastBlock       int32
	requestQueue    *list.List
	invSendQueue    *list.List
	continueHash    *btcwire.ShaHash
	wg              sync.WaitGroup
	outputQueue     chan btcwire.Message
	outputInvChan   chan *btcwire.InvVect
	blockProcessed  chan bool
	quit            chan bool
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

	// Create a NetAddress for the local IP.  Don't assume any services
	// until we know otherwise.
	naMe, err := newNetAddress(p.conn.LocalAddr(), 0)
	if err != nil {
		return err
	}

	// Create a NetAddress for the remote IP.  Don't assume any services
	// until we know otherwise.
	naYou, err := newNetAddress(p.conn.RemoteAddr(), 0)
	if err != nil {
		return err
	}

	// Version message.
	msg := btcwire.NewMsgVersion(naMe, naYou, p.server.nonce, userAgent,
		int32(blockNum))

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

	p.outputQueue <- msg
	return nil
}

// handleVersionMsg is invoked when a peer receives a version bitcoin message
// and is used to negotiate the protocol version details as well as kick start
// the communications.
func (p *peer) handleVersionMsg(msg *btcwire.MsgVersion) {
	// Detect self connections.
	if msg.Nonce == p.server.nonce {
		log.Debugf("[PEER] Disconnecting peer connected to self %s",
			p.conn.RemoteAddr())
		p.Disconnect()
		return
	}

	// Limit to one version message per peer.
	if p.versionKnown {
		log.Errorf("[PEER] Only one version message per peer is allowed %s.",
			p.conn.RemoteAddr())
		p.Disconnect()
		return
	}

	// Negotiate the protocol version.
	p.protocolVersion = minUint32(p.protocolVersion, uint32(msg.ProtocolVersion))
	p.versionKnown = true
	log.Debugf("[PEER] Negotiated protocol version %d for peer %s",
		p.protocolVersion, p.conn.RemoteAddr())
	p.lastBlock = msg.LastBlock

	// Set the supported services for the peer to what the remote peer
	// advertised.
	p.services = msg.Services

	// Inbound connections.
	if p.inbound {
		// Send version.
		err := p.pushVersionMsg()
		if err != nil {
			log.Errorf("[PEER] %v", err)
			p.Disconnect()
			return
		}

		// Add inbound peer address to the server address manager.
		na, err := btcwire.NewNetAddress(p.conn.RemoteAddr(), p.services)
		if err != nil {
			log.Errorf("[PEER] %v", err)
			p.Disconnect()
			return
		}
		p.server.addrManager.AddAddress(na)
	}

	// Send verack.
	p.outputQueue <- btcwire.NewMsgVerAck()

	// Outbound connections.
	if !p.inbound {
		// TODO(davec): Only do this if not doing the initial block
		// download and the local address is routable.
		if !cfg.DisableListen {
			// Advertise the local address.
			na, err := newNetAddress(p.conn.LocalAddr(), p.services)
			if err != nil {
				log.Errorf("[PEER] %v", err)
				p.Disconnect()
				return
			}
			addresses := map[string]*btcwire.NetAddress{
				NetAddressKey(na): na,
			}
			p.pushAddrMsg(addresses)
		}

		// Request known addresses if the server address manager needs
		// more and the peer has a protocol version new enough to
		// include a timestamp with addresses.
		hasTimestamp := p.protocolVersion >= btcwire.NetAddressTimeVersion
		if p.server.addrManager.NeedMoreAddresses() && hasTimestamp {
			p.outputQueue <- btcwire.NewMsgGetAddr()
		}
	}

	// Signal the block manager this peer is a new sync candidate.
	p.server.blockManager.newCandidates <- p

	// TODO: Relay alerts.
}

// pushTxMsg sends a tx message for the provided transaction hash to the
// connected peer.  An error is returned if the transaction sha is not known.
func (p *peer) pushTxMsg(sha btcwire.ShaHash) error {
	// We dont deal with these for now.
	return errors.New("Tx fetching not implemented")
}

// pushBlockMsg sends a block message for the provided block hash to the
// connected peer.  An error is returned if the block hash is not known.
func (p *peer) pushBlockMsg(sha btcwire.ShaHash) error {
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

	blk, err := p.server.db.FetchBlockBySha(&sha)
	if err != nil {
		log.Tracef("[PEER] Unable to fetch requested block sha %v: %v",
			&sha, err)
		return err
	}
	p.QueueMessage(blk.MsgBlock())

	// When the peer requests the final block that was advertised in
	// response to a getblocks message which requested more blocks than
	// would fit into a single message, send it a new inventory message
	// to trigger it to issue another getblocks message for the next
	// batch of inventory.
	if p.continueHash != nil && p.continueHash.IsEqual(&sha) {
		hash, _, err := p.server.db.NewestSha()
		if err == nil {
			invMsg := btcwire.NewMsgInv()
			iv := btcwire.NewInvVect(btcwire.InvVect_Block, hash)
			invMsg.AddInvVect(iv)
			p.QueueMessage(invMsg)
			p.continueHash = nil
		}
	}
	return nil
}

// pushGetBlocksMsg sends a getblocks message for the provided block locator
// and stop hash.
func (p *peer) pushGetBlocksMsg(locator btcchain.BlockLocator, stopHash *btcwire.ShaHash) error {
	msg := btcwire.NewMsgGetBlocks(stopHash)
	for _, hash := range locator {
		err := msg.AddBlockLocatorHash(hash)
		if err != nil {
			return err
		}
	}
	p.QueueMessage(msg)
	return nil
}

// handleBlockMsg is invoked when a peer receives a block bitcoin message.  It
// blocks until the bitcoin block has been fully processed.
func (p *peer) handleBlockMsg(msg *btcwire.MsgBlock, buf []byte) {
	// Convert the raw MsgBlock to a btcutil.Block which
	// provides some convience methods and things such as
	// hash caching.
	block := btcutil.NewBlockFromBlockAndBytes(msg, buf)

	// Add the block to the known inventory for the peer.
	hash, err := block.Sha()
	if err != nil {
		log.Errorf("Unable to get block hash: %v", err)
		return
	}
	iv := btcwire.NewInvVect(btcwire.InvVect_Block, hash)
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
// accordingly.
//
// NOTE: This will need to have tx handling added as well when they are
// supported.
func (p *peer) handleInvMsg(msg *btcwire.MsgInv) {
	// Attempt to find the final block in the inventory list.  There may
	// not be one.
	lastBlock := -1
	invVects := msg.InvList
	for i := len(invVects) - 1; i >= 0; i-- {
		if invVects[i].Type == btcwire.InvVect_Block {
			lastBlock = i
			break
		}
	}

	// Request the advertised inventory if we don't already have it.  Also,
	// request parent blocks of orphans if we receive one we already have.
	// Finally, attempt to detect potential stalls due to long side chains
	// we already have and request more blocks to prevent them.
	chain := p.server.blockManager.blockChain
	for i, iv := range invVects {
		switch iv.Type {
		case btcwire.InvVect_Block:
			// Add the inventory to the cache of known inventory
			// for the peer.
			p.addKnownInventory(iv)

			// Request the inventory if we don't already have it.
			if !chain.HaveInventory(iv) {
				// Add it to the request queue.
				p.requestQueue.PushBack(iv)
				continue
			}

			// The block is an orphan block that we already have.
			// When the existing orphan was processed, it requested
			// the missing parent blocks.  When this scenario
			// happens, it means there were more blocks missing
			// than are allowed into a single inventory message.  As
			// a result, once this peer requested the final
			// advertised block, the remote peer noticed and is now
			// resending the orphan block as an available block
			// to signal there are more missing blocks that need to
			// be requested.
			if chain.IsKnownOrphan(&iv.Hash) {
				// Request blocks starting at the latest known
				// up to the root of the orphan that just came
				// in.
				orphanRoot := chain.GetOrphanRoot(&iv.Hash)
				locator, err := chain.LatestBlockLocator()
				if err != nil {
					log.Errorf("[PEER] Failed to get block "+
						"locator for the latest block: "+
						"%v", err)
					continue
				}
				p.pushGetBlocksMsg(locator, orphanRoot)
				continue
			}

			// We already have the final block advertised by this
			// inventory message, so force a request for more.  This
			// should only really happen if we're on a really long
			// side chain.
			if i == lastBlock {
				// Request blocks after this one up to the
				// final one the remote peer knows about (zero
				// stop hash).
				locator := chain.BlockLocatorFromHash(&iv.Hash)
				p.pushGetBlocksMsg(locator, &zeroHash)
			}

		// Ignore unsupported inventory types.
		default:
			continue
		}
	}

	// Request as much as possible at once.  Anything that won't fit into
	// the request will be requested on the next inv message.
	numRequested := 0
	gdmsg := btcwire.NewMsgGetData()
	for e := p.requestQueue.Front(); e != nil; e = p.requestQueue.Front() {
		iv := e.Value.(*btcwire.InvVect)
		gdmsg.AddInvVect(iv)
		p.requestQueue.Remove(e)

		numRequested++
		if numRequested >= btcwire.MaxInvPerMsg {
			break
		}
	}
	if len(gdmsg.InvList) > 0 {
		p.QueueMessage(gdmsg)
	}
}

// handleGetData is invoked when a peer receives a getdata bitcoin message and
// is used to deliver block and transaction information.
func (p *peer) handleGetDataMsg(msg *btcwire.MsgGetData) {
	notFound := btcwire.NewMsgNotFound()

out:
	for _, iv := range msg.InvList {
		var err error
		switch iv.Type {
		case btcwire.InvVect_Tx:
			err = p.pushTxMsg(iv.Hash)
		case btcwire.InvVect_Block:
			err = p.pushBlockMsg(iv.Hash)
		default:
			log.Warnf("[PEER] Unknown type in inventory request %d",
				iv.Type)
			break out
		}
		if err != nil {
			notFound.AddInvVect(iv)
		}
	}
	if len(notFound.InvList) != 0 {
		p.QueueMessage(notFound)
	}
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
			log.Warnf("[PEER] Block lookup failed: %v", err)
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
			iv := btcwire.NewInvVect(btcwire.InvVect_Block, &hashCopy)
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
		p.QueueMessage(invMsg)
	}
}

// handleGetBlocksMsg is invoked when a peer receives a getheaders bitcoin
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
		p.QueueMessage(headersMsg)
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
			log.Warnf("[PEER] Header lookup failed: %v", err)
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
				log.Warnf("[PEER] Lookup of known block hash "+
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
	p.QueueMessage(headersMsg)
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
		log.Errorf("[PEER] %v", err)
		p.Disconnect()
		return
	}
}

// pushAddrMsg sends one, or more, addr message(s) to the connected peer using
// the provided addresses.
func (p *peer) pushAddrMsg(addresses map[string]*btcwire.NetAddress) error {
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
			p.outputQueue <- msg
			msg.ClearAddresses()
		}
	}

	// Send message with remaining addresses if needed.
	if numAdded%btcwire.MaxAddrPerMsg != 0 {
		p.outputQueue <- msg
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
		log.Errorf("[PEER] Command [%s] from %s does not contain any addresses",
			msg.Command(), p.conn.RemoteAddr())
		p.Disconnect()
		return
	}

	for _, na := range msg.AddrList {
		// Don't add more address if we're disconnecting.
		if p.disconnect {
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
	p.server.addrManager.AddAddresses(msg.AddrList)
}

// handlePingMsg is invoked when a peer receives a ping bitcoin message.  For
// recent clients (protocol version > BIP0031Version), it replies with a pong
// message.  For older clients, it does nothing and anything other than failure
// is considered a successful ping.
func (p *peer) handlePingMsg(msg *btcwire.MsgPing) {
	// Only Reply with pong is message comes from a new enough client.
	if p.protocolVersion > btcwire.BIP0031Version {
		// Include nonce from ping so pong can be identified.
		p.outputQueue <- btcwire.NewMsgPong(msg.Nonce)
	}
}

// readMessage reads the next bitcoin message from the peer with logging.
func (p *peer) readMessage() (msg btcwire.Message, buf []byte, err error) {
	msg, buf, err = btcwire.ReadMessage(p.conn, p.protocolVersion, p.btcnet)
	if err != nil {
		return
	}
	log.Debugf("[PEER] Received command [%v] from %s", msg.Command(),
		p.conn.RemoteAddr())

	// Use closures to log expensive operations so they are only run when
	// the logging level requires it.
	log.Tracef("%v", newLogClosure(func() string {
		return "[PEER] " + spew.Sdump(msg)
	}))
	log.Tracef("%v", newLogClosure(func() string {
		return "[PEER] " + spew.Sdump(buf)
	}))

	return
}

// writeMessage sends a bitcoin Message to the peer with logging.
func (p *peer) writeMessage(msg btcwire.Message) {
	// Don't do anything if we're disconnecting.
	if p.disconnect == true {
		return
	}

	log.Debugf("[PEER] Sending command [%v] to %s", msg.Command(),
		p.conn.RemoteAddr())

	// Use closures to log expensive operations so they are only run when the
	// logging level requires it.
	log.Tracef("%v", newLogClosure(func() string {
		return "[PEER] msg" + spew.Sdump(msg)
	}))
	log.Tracef("%v", newLogClosure(func() string {
		var buf bytes.Buffer
		err := btcwire.WriteMessage(&buf, msg, p.protocolVersion, p.btcnet)
		if err != nil {
			return err.Error()
		}
		return "[PEER] " + spew.Sdump(buf.Bytes())
	}))

	// Write the message to the peer.
	err := btcwire.WriteMessage(p.conn, msg, p.protocolVersion, p.btcnet)
	if err != nil {
		p.Disconnect()
		log.Errorf("[PEER] %v", err)
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
	host, _, err := net.SplitHostPort(p.conn.RemoteAddr().String())
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
out:
	for !p.disconnect {
		rmsg, buf, err := p.readMessage()
		if err != nil {
			// In order to allow regression tests with malformed
			// messages, don't disconnect the peer when we're in
			// regression test mode and the error is one of the
			// allowed errors.
			if cfg.RegressionTest && p.isAllowedByRegression(err) {
				log.Errorf("[PEER] %v", err)
				continue
			}

			// Only log the error if we're not forcibly disconnecting.
			if !p.disconnect {
				log.Errorf("[PEER] %v", err)
			}
			break out
		}

		// Ensure version message comes first.
		if _, ok := rmsg.(*btcwire.MsgVersion); !ok && !p.versionKnown {
			log.Errorf("[PEER] A version message must precede all others")
			break out
		}

		// Handle each supported message type.
		switch msg := rmsg.(type) {
		case *btcwire.MsgVersion:
			p.handleVersionMsg(msg)

		case *btcwire.MsgVerAck:
			// Do nothing.

		case *btcwire.MsgGetAddr:
			p.handleGetAddrMsg(msg)

		case *btcwire.MsgAddr:
			p.handleAddrMsg(msg)

		case *btcwire.MsgPing:
			p.handlePingMsg(msg)

		case *btcwire.MsgPong:
			// Don't do anything, but could try to work out network
			// timing or similar.

		case *btcwire.MsgAlert:
			p.server.BroadcastMessage(msg, p)

		case *btcwire.MsgBlock:
			p.handleBlockMsg(msg, buf)

		case *btcwire.MsgInv:
			p.handleInvMsg(msg)

		case *btcwire.MsgGetData:
			p.handleGetDataMsg(msg)

		case *btcwire.MsgGetBlocks:
			p.handleGetBlocksMsg(msg)

		case *btcwire.MsgGetHeaders:
			p.handleGetHeadersMsg(msg)

		default:
			log.Debugf("[PEER] Received unhandled message of type %v: Fix Me",
				rmsg.Command())
		}
	}

	// Ensure connection is closed and notify server and block manager that
	// the peer is done.
	p.Disconnect()
	p.server.donePeers <- p
	p.server.blockManager.donePeers <- p
	p.quit <- true

	p.wg.Done()
	log.Tracef("[PEER] Peer input handler done for %s", p.conn.RemoteAddr())
}

// outHandler handles all outgoing messages for the peer.  It must be run as a
// goroutine.  It uses a buffered channel to serialize output messages while
// allowing the sender to continue running asynchronously.
func (p *peer) outHandler() {
	trickleTicker := time.NewTicker(time.Second * 10)
out:
	for {
		select {
		case msg := <-p.outputQueue:
			p.writeMessage(msg)

		case iv := <-p.outputInvChan:
			p.invSendQueue.PushBack(iv)

		case <-trickleTicker.C:
			// Don't send anything if we're disconnecting or there
			// is no queued inventory.
			if p.disconnect || p.invSendQueue.Len() == 0 {
				continue
			}

			// Create a new inventory message, populate it with as
			// much per
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
				log.Infof("Relaying %d inv to %v", len(invMsg.InvList), p.conn.RemoteAddr())
				p.writeMessage(invMsg)
			}

		case <-p.quit:
			break out
		}
	}
	p.wg.Done()
	log.Tracef("[PEER] Peer output handler done for %s", p.conn.RemoteAddr())
}

// QueueMessage adds the passed bitcoin message to the peer send queue.  It
// uses a buffered channel to communicate with the output handler goroutine so
// it is automatically rate limited and safe for concurrent access.
func (p *peer) QueueMessage(msg btcwire.Message) {
	p.outputQueue <- msg
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

// Start begins processing input and output messages.  It also sends the initial
// version message for outbound connections to start the negotiation process.
func (p *peer) Start() error {
	// Already started?
	if p.started {
		return nil
	}

	log.Tracef("[PEER] Starting peer %s", p.conn.RemoteAddr())

	// Send an initial version message if this is an outbound connection.
	if !p.inbound {
		err := p.pushVersionMsg()
		if err != nil {
			log.Errorf("[PEER] %v", err)
			p.conn.Close()
			return err
		}
	}

	// Start processing input and output.
	go p.inHandler()
	go p.outHandler()
	p.wg.Add(2)
	p.started = true

	return nil
}

// Disconnect disconnects the peer by closing the connection.  It also sets
// a flag so the impending shutdown can be detected.
func (p *peer) Disconnect() {
	p.disconnect = true
	p.conn.Close()
}

// Shutdown gracefully shuts down the peer by disconnecting it and waiting for
// all goroutines to finish.
func (p *peer) Shutdown() {
	log.Tracef("[PEER] Shutdown peer %s", p.conn.RemoteAddr())
	p.Disconnect()
	p.wg.Wait()
}

// newPeer returns a new bitcoin peer for the provided server and connection.
// Use start to begin processing incoming and outgoing messages.
func newPeer(s *server, conn net.Conn, inbound bool, persistent bool) *peer {
	p := peer{
		server:          s,
		protocolVersion: btcwire.ProtocolVersion,
		btcnet:          s.btcnet,
		services:        btcwire.SFNodeNetwork,
		conn:            conn,
		timeConnected:   time.Now(),
		inbound:         inbound,
		persistent:      persistent,
		knownAddresses:  make(map[string]bool),
		knownInventory:  NewMruInventoryMap(maxKnownInventory),
		requestQueue:    list.New(),
		invSendQueue:    list.New(),
		outputQueue:     make(chan btcwire.Message, outputBufferSize),
		outputInvChan:   make(chan *btcwire.InvVect, outputBufferSize),
		blockProcessed:  make(chan bool, 1),
		quit:            make(chan bool),
	}
	return &p
}
