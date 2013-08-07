// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"errors"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/davecgh/go-spew/spew"
	"net"
	"sync"
	"time"
)

const outputBufferSize = 50

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
	lastBlock       int32
	wg              sync.WaitGroup
	outputQueue     chan btcwire.Message
	quit            chan bool
}

// pushVersionMsg sends a version message to the connected peer using the
// current state.
func (p *peer) pushVersionMsg() error {
	_, blockNum, err := p.server.db.NewestSha()
	if err != nil {
		return err
	}

	msg, err := btcwire.NewMsgVersionFromConn(p.conn, p.server.nonce,
		userAgent, int32(blockNum))
	if err != nil {
		return err
	}

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
		p.disconnect = true
		p.conn.Close()
		return
	}

	// Limit to one version message per peer.
	if p.versionKnown {
		log.Errorf("[PEER] Only one version message per peer is allowed %s.",
			p.conn.RemoteAddr())
		p.disconnect = true
		p.conn.Close()
		return
	}

	// Negotiate the protocol version.
	p.protocolVersion = minUint32(p.protocolVersion, uint32(msg.ProtocolVersion))
	p.versionKnown = true
	log.Debugf("[PEER] Negotiated protocol version %d for peer %s",
		p.protocolVersion, p.conn.RemoteAddr())
	p.lastBlock = msg.LastBlock

	// Inbound connections.
	if p.inbound {
		// Set the supported services for the peer to what the remote
		// peer advertised.
		p.services = msg.Services

		// Send version.
		err := p.pushVersionMsg()
		if err != nil {
			log.Errorf("[PEER] %v", err)
			p.disconnect = true
			p.conn.Close()
			return
		}

		// Add inbound peer address to the server address manager.
		na, err := btcwire.NewNetAddress(p.conn.RemoteAddr(), p.services)
		if err != nil {
			log.Errorf("[PEER] %v", err)
			p.disconnect = true
			p.conn.Close()
			return
		}
		p.server.addrManager.AddAddress(na)
	}

	// Send verack.
	p.outputQueue <- btcwire.NewMsgVerAck()

	// Outbound connections.
	if !p.inbound {
		// TODO: Only do this if we're listening, not doing the initial
		// block download, and are routable.
		// Advertise the local address.
		na, err := btcwire.NewNetAddress(p.conn.LocalAddr(), p.services)
		if err != nil {
			log.Errorf("[PEER] %v", err)
			p.disconnect = true
			p.conn.Close()
			return
		}
		na.Services = p.services
		addresses := map[string]*btcwire.NetAddress{
			NetAddressKey(na): na,
		}
		p.pushAddrMsg(addresses)

		// Request known addresses if the server address manager needs
		// more and the peer has a protocol version new enough to
		// include a timestamp with addresses.
		hasTimestamp := p.protocolVersion >= btcwire.NetAddressTimeVersion
		if p.server.addrManager.NeedMoreAddresses() && hasTimestamp {
			p.outputQueue <- btcwire.NewMsgGetAddr()
		}
	}

	// Request latest blocks if the peer has blocks we're interested in.
	// XXX: Ask block manager for latest so we get in-flight too...
	sha, lastBlock, err := p.server.db.NewestSha()
	if err != nil {
		log.Errorf("[PEER] %v", err)
		p.disconnect = true
		p.conn.Close()
	}
	// If the peer has blocks we're interested in.
	if p.lastBlock > int32(lastBlock) {
		stopHash := btcwire.ShaHash{}
		gbmsg := btcwire.NewMsgGetBlocks(&stopHash)
		p.server.blockManager.AddBlockLocators(sha, gbmsg)
		p.outputQueue <- gbmsg
	}

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
	return nil
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
	var err error
	startIdx := int64(0)
	endIdx := btcdb.AllShas

	// Return all block hashes to the latest one (up to max per message) if
	// no stop hash was specified.
	// Attempt to find the ending index of the stop hash if specified.
	if !msg.HashStop.IsEqual(&zeroHash) {
		block, err := p.server.db.FetchBlockBySha(&msg.HashStop)
		if err != nil {
			// Fetch all if we dont recognize the stop hash.
			endIdx = btcdb.AllShas
		}
		endIdx = block.Height()
	}

	// TODO(davec): This should have some logic to utilize the additional
	// locator hashes to ensure the proper chain.
	for _, hash := range msg.BlockLocatorHashes {
		// TODO(drahn) does using the caching interface make sense
		// on index lookups ?
		block, err := p.server.db.FetchBlockBySha(hash)
		if err == nil {
			// Start with the next hash since we know this one.
			startIdx = block.Height() + 1
			break
		}
	}

	// Don't attempt to fetch more than we can put into a single message.
	if endIdx-startIdx > btcwire.MaxInvPerMsg {
		endIdx = startIdx + btcwire.MaxInvPerMsg
	}

	// Fetch the inventory from the block database.
	hashList, err := p.server.db.FetchHeightRange(startIdx, endIdx)
	if err != nil {
		log.Warnf(" lookup returned %v ", err)
		return
	}

	// Nothing to send.
	if len(hashList) == 0 {
		return
	}

	// Generate inventory vectors and push the inventory message.
	inv := btcwire.NewMsgInv()
	for _, hash := range hashList {
		iv := btcwire.InvVect{Type: btcwire.InvVect_Block, Hash: hash}
		inv.AddInvVect(&iv)
	}
	p.QueueMessage(inv)
}

// handleGetBlocksMsg is invoked when a peer receives a getheaders bitcoin
// message.
func (p *peer) handleGetHeadersMsg(msg *btcwire.MsgGetHeaders) {
	var err error
	startIdx := int64(0)
	endIdx := btcdb.AllShas

	// Return all block hashes to the latest one (up to max per message) if
	// no stop hash was specified.
	// Attempt to find the ending index of the stop hash if specified.
	if !msg.HashStop.IsEqual(&zeroHash) {
		block, err := p.server.db.FetchBlockBySha(&msg.HashStop)
		if err != nil {
			// Fetch all if we dont recognize the stop hash.
			endIdx = btcdb.AllShas
		}
		endIdx = block.Height()
	}

	// TODO(davec): This should have some logic to utilize the additional
	// locator hashes to ensure the proper chain.
	for _, hash := range msg.BlockLocatorHashes {
		// TODO(drahn) does using the caching interface make sense
		// on index lookups ?
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

	// Fetch the inventory from the block database.
	hashList, err := p.server.db.FetchHeightRange(startIdx, endIdx)
	if err != nil {
		log.Warnf("lookup returned %v ", err)
		return
	}

	// Nothing to send.
	if len(hashList) == 0 {
		return
	}

	// Generate inventory vectors and push the inventory message.
	headersMsg := btcwire.NewMsgHeaders()
	for _, hash := range hashList {
		block, err := p.server.db.FetchBlockBySha(&hash)
		if err != nil {
			log.Warnf("[PEER] badness %v", err)
		}
		hdr := block.MsgBlock().Header // copy
		hdr.TxnCount = 0
		headersMsg.AddBlockHeader(&hdr)
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
		p.disconnect = true
		p.conn.Close()
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
		p.disconnect = true
		p.conn.Close()
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
func (p *peer) writeMessage(msg btcwire.Message) error {
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
		return err
	}
	return nil
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

		// Some messages are handled directly, while other messages
		// are sent to a queue to be processed.  Directly handling
		// getdata and getblocks messages makes it impossible for a peer
		// to spam with requests.  However, it means that our getdata
		// requests to it may not get prompt replies.
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
			block := btcutil.NewBlockFromBlockAndBytes(msg, buf)
			p.server.blockManager.QueueBlock(block)

		case *btcwire.MsgInv:
			p.server.blockManager.QueueInv(msg, p)

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

	// Ensure connection is closed and notify server that the peer is done.
	p.disconnect = true
	p.conn.Close()
	p.server.donePeers <- p
	p.quit <- true

	p.wg.Done()
	log.Tracef("[PEER] Peer input handler done for %s", p.conn.RemoteAddr())
}

// outHandler handles all outgoing messages for the peer.  It must be run as a
// goroutine.  It uses a buffered channel to serialize output messages while
// allowing the sender to continue running asynchronously.
func (p *peer) outHandler() {
out:
	for {
		select {
		case msg := <-p.outputQueue:
			// Don't send anything if we're disconnected.
			if p.disconnect {
				continue
			}
			err := p.writeMessage(msg)
			if err != nil {
				p.disconnect = true
				log.Errorf("[PEER] %v", err)
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

	// If server is shutting down, don't even start watchdog
	if p.server.shutdown {
		log.Debug("[PEER] server is shutting down")
		return nil
	}

	return nil
}

// Shutdown gracefully shuts down the peer by signalling the async input and
// output handler and waiting for them to finish.
func (p *peer) Shutdown() {
	log.Tracef("[PEER] Shutdown peer %s", p.conn.RemoteAddr())
	p.disconnect = true
	p.conn.Close()
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
		outputQueue:     make(chan btcwire.Message, outputBufferSize),
		quit:            make(chan bool),
	}
	return &p
}

type logClosure func() string

func (c logClosure) String() string {
	return c()
}

func newLogClosure(c func() string) logClosure {
	return logClosure(c)
}
