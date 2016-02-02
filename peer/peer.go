// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/go-socks/socks"
	"github.com/davecgh/go-spew/spew"
)

const (
	// MaxProtocolVersion is the max protocol version the peer supports.
	MaxProtocolVersion = 70011

	// outputBufferSize is the number of elements the output channels use.
	outputBufferSize = 50

	// invTrickleSize is the maximum amount of inventory to send in a single
	// message when trickling inventory to remote peers.
	maxInvTrickleSize = 1000

	// maxKnownInventory is the maximum number of items to keep in the known
	// inventory cache.
	maxKnownInventory = 1000

	// pingInterval is the interval of time to wait in between sending ping
	// messages.
	pingInterval = 2 * time.Minute

	// negotiateTimeout is the duration of inactivity before we timeout a
	// peer that hasn't completed the initial version negotiation.
	negotiateTimeout = 30 * time.Second

	// idleTimeout is the duration of inactivity before we time out a peer.
	idleTimeout = 5 * time.Minute

	// stallTickInterval is the interval of time between each check for
	// stalled peers.
	stallTickInterval = 15 * time.Second

	// stallResponseTimeout is the base maximum amount of time messages that
	// expect a response will wait before disconnecting the peer for
	// stalling.  The deadlines are adjusted for callback running times and
	// only checked on each stall tick interval.
	stallResponseTimeout = 30 * time.Second

	// trickleTimeout is the duration of the ticker which trickles down the
	// inventory to a peer.
	trickleTimeout = 10 * time.Second
)

var (
	// nodeCount is the total number of peer connections made since startup
	// and is used to assign an id to a peer.
	nodeCount int32

	// zeroHash is the zero value hash (all zeros).  It is defined as a
	// convenience.
	zeroHash wire.ShaHash

	// sentNonces houses the unique nonces that are generated when pushing
	// version messages that are used to detect self connections.
	sentNonces = newMruNonceMap(50)

	// allowSelfConns is only used to allow the tests to bypass the self
	// connection detecting and disconnect logic since they intentionally
	// do so for testing purposes.
	allowSelfConns bool
)

// MessageListeners defines callback function pointers to invoke with message
// listeners for a peer. Any listener which is not set to a concrete callback
// during peer initialization is ignored. Execution of multiple message
// listeners occurs serially, so one callback blocks the excution of the next.
//
// NOTE: Unless otherwise documented, these listeners must NOT directly call any
// blocking calls (such as WaitForDisconnect) on the peer instance since the input
// handler goroutine blocks until the callback has completed.  Doing so will
// result in a deadlock.
type MessageListeners struct {
	// OnGetAddr is invoked when a peer receives a getaddr bitcoin message.
	OnGetAddr func(p *Peer, msg *wire.MsgGetAddr)

	// OnAddr is invoked when a peer receives an addr bitcoin message.
	OnAddr func(p *Peer, msg *wire.MsgAddr)

	// OnPing is invoked when a peer receives a ping bitcoin message.
	OnPing func(p *Peer, msg *wire.MsgPing)

	// OnPong is invoked when a peer receives a pong bitcoin message.
	OnPong func(p *Peer, msg *wire.MsgPong)

	// OnAlert is invoked when a peer receives an alert bitcoin message.
	OnAlert func(p *Peer, msg *wire.MsgAlert)

	// OnMemPool is invoked when a peer receives a mempool bitcoin message.
	OnMemPool func(p *Peer, msg *wire.MsgMemPool)

	// OnTx is invoked when a peer receives a tx bitcoin message.
	OnTx func(p *Peer, msg *wire.MsgTx)

	// OnBlock is invoked when a peer receives a block bitcoin message.
	OnBlock func(p *Peer, msg *wire.MsgBlock, buf []byte)

	// OnInv is invoked when a peer receives an inv bitcoin message.
	OnInv func(p *Peer, msg *wire.MsgInv)

	// OnHeaders is invoked when a peer receives a headers bitcoin message.
	OnHeaders func(p *Peer, msg *wire.MsgHeaders)

	// OnNotFound is invoked when a peer receives a notfound bitcoin
	// message.
	OnNotFound func(p *Peer, msg *wire.MsgNotFound)

	// OnGetData is invoked when a peer receives a getdata bitcoin message.
	OnGetData func(p *Peer, msg *wire.MsgGetData)

	// OnGetBlocks is invoked when a peer receives a getblocks bitcoin
	// message.
	OnGetBlocks func(p *Peer, msg *wire.MsgGetBlocks)

	// OnGetHeaders is invoked when a peer receives a getheaders bitcoin
	// message.
	OnGetHeaders func(p *Peer, msg *wire.MsgGetHeaders)

	// OnFilterAdd is invoked when a peer receives a filteradd bitcoin message.
	// Peers that do not advertise support for bloom filters and negotiate to a
	// protocol version before BIP0111 will simply ignore the message while
	// those that negotiate to the BIP0111 protocol version or higher will be
	// immediately disconnected.
	OnFilterAdd func(p *Peer, msg *wire.MsgFilterAdd)

	// OnFilterClear is invoked when a peer receives a filterclear bitcoin
	// message.
	// Peers that do not advertise support for bloom filters and negotiate to a
	// protocol version before BIP0111 will simply ignore the message while
	// those that negotiate to the BIP0111 protocol version or higher will be
	// immediately disconnected.
	OnFilterClear func(p *Peer, msg *wire.MsgFilterClear)

	// OnFilterLoad is invoked when a peer receives a filterload bitcoin
	// message.
	// Peers that do not advertise support for bloom filters and negotiate to a
	// protocol version before BIP0111 will simply ignore the message while
	// those that negotiate to the BIP0111 protocol version or higher will be
	// immediately disconnected.
	OnFilterLoad func(p *Peer, msg *wire.MsgFilterLoad)

	// OnMerkleBlock  is invoked when a peer receives a merkleblock bitcoin
	// message.
	OnMerkleBlock func(p *Peer, msg *wire.MsgMerkleBlock)

	// OnVersion is invoked when a peer receives a version bitcoin message.
	OnVersion func(p *Peer, msg *wire.MsgVersion)

	// OnVerAck is invoked when a peer receives a verack bitcoin message.
	OnVerAck func(p *Peer, msg *wire.MsgVerAck)

	// OnReject is invoked when a peer receives a reject bitcoin message.
	OnReject func(p *Peer, msg *wire.MsgReject)

	// OnRead is invoked when a peer receives a bitcoin message.  It
	// consists of the number of bytes read, the message, and whether or not
	// an error in the read occurred.  Typically, callers will opt to use
	// the callbacks for the specific message types, however this can be
	// useful for circumstances such as keeping track of server-wide byte
	// counts or working with custom message types for which the peer does
	// not directly provide a callback.
	OnRead func(p *Peer, bytesRead int, msg wire.Message, err error)

	// OnWrite is invoked when a peer receives a bitcoin message.  It
	// consists of the number of bytes written, the message, and whether or
	// not an error in the write occurred.  This can be useful for
	// circumstances such as keeping track of server-wide byte counts.
	OnWrite func(p *Peer, bytesWritten int, msg wire.Message, err error)
}

// Config is the struct to hold configuration options useful to Peer.
type Config struct {
	// NewestBlock specifies a callback which provides the newest block
	// details to the peer as needed.  This can be nil in which case the
	// peer will report a block height of 0, however it is good practice for
	// peers to specify this so their currently best known is accurately
	// reported.
	NewestBlock ShaFunc

	// BestLocalAddress returns the best local address for a given address.
	BestLocalAddress AddrFunc

	// HostToNetAddress returns the netaddress for the given host. This can be
	// nil in  which case the host will be parsed as an IP address.
	HostToNetAddress HostToNetAddrFunc

	// Proxy indicates a proxy is being used for connections.  The only
	// effect this has is to prevent leaking the tor proxy address, so it
	// only needs to specified if using a tor proxy.
	Proxy string

	// UserAgentName specifies the user agent name to advertise.  It is
	// highly recommended to specify this value.
	UserAgentName string

	// UserAgentVersion specifies the user agent version to advertise.  It
	// is highly recommended to specify this value and that it follows the
	// form "major.minor.revision" e.g. "2.6.41".
	UserAgentVersion string

	// ChainParams identifies which chain parameters the peer is associated
	// with.  It is highly recommended to specify this field, however it can
	// be omitted in which case the test network will be used.
	ChainParams *chaincfg.Params

	// Services specifies which services to advertise as supported by the
	// local peer.  This field can be omitted in which case it will be 0
	// and therefore advertise no supported services.
	Services wire.ServiceFlag

	// ProtocolVersion specifies the maximum protocol version to use and
	// advertise.  This field can be omitted in which case
	// peer.MaxProtocolVersion will be used.
	ProtocolVersion uint32

	// DisableRelayTx specifies if the remote peer should be informed to
	// not send inv messages for transactions.
	DisableRelayTx bool

	// Listeners houses callback functions to be invoked on receiving peer
	// messages.
	Listeners MessageListeners
}

// newNetAddress attempts to extract the IP address and port from the passed
// net.Addr interface and create a bitcoin NetAddress structure using that
// information.
func newNetAddress(addr net.Addr, services wire.ServiceFlag) (*wire.NetAddress, error) {
	// addr will be a net.TCPAddr when not using a proxy.
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		ip := tcpAddr.IP
		port := uint16(tcpAddr.Port)
		na := wire.NewNetAddressIPPort(ip, port, services)
		return na, nil
	}

	// addr will be a socks.ProxiedAddr when using a proxy.
	if proxiedAddr, ok := addr.(*socks.ProxiedAddr); ok {
		ip := net.ParseIP(proxiedAddr.Host)
		if ip == nil {
			ip = net.ParseIP("0.0.0.0")
		}
		port := uint16(proxiedAddr.Port)
		na := wire.NewNetAddressIPPort(ip, port, services)
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
	na := wire.NewNetAddressIPPort(ip, uint16(port), services)
	return na, nil
}

type pausableTimer struct {
	t *time.Timer

	start time.Time
	d     time.Duration
}

func pausableTimerAfterFunc(d time.Duration, f func()) *pausableTimer {
	return &pausableTimer{
		t: time.AfterFunc(d, f),

		start: time.Now(),
		d:     d,
	}
}

func (pt *pausableTimer) Pause() bool {
	pt.d = pt.start.Add(pt.d).Sub(time.Now())
	return pt.t.Stop()
}

func (pt *pausableTimer) Unpause() bool {
	if pt.d >= 0 {
		pt.t.Reset(pt.d)
		pt.start = time.Now()
		return true
	}
	return false
}

func (pt *pausableTimer) Stop() bool {
	return pt.t.Stop()
}

type writeMsg struct {
	msg  wire.Message
	done chan<- struct{}
}

type readMsg struct {
	msg wire.Message
	buf []byte
	err error
}

// StatsSnap is a snapshot of peer stats at a point in time.
type StatsSnap struct {
	ID             int32
	Addr           string
	Services       wire.ServiceFlag
	LastSend       time.Time
	LastRecv       time.Time
	BytesSent      uint64
	BytesRecv      uint64
	ConnTime       time.Time
	TimeOffset     int64
	Version        uint32
	UserAgent      string
	Inbound        bool
	StartingHeight int32
	LastBlock      int32
	LastPingNonce  uint64
	LastPingTime   time.Time
	LastPingMicros int64
}

// ShaFunc is a function which returns a block sha, height and error
// It is used as a callback to get newest block details.
type ShaFunc func() (sha *wire.ShaHash, height int32, err error)

// AddrFunc is a func which takes an address and returns a related address.
type AddrFunc func(remoteAddr *wire.NetAddress) *wire.NetAddress

// HostToNetAddrFunc is a func which takes a host, port, services and returns
// the netaddress.
type HostToNetAddrFunc func(host string, port uint16,
	services wire.ServiceFlag) (*wire.NetAddress, error)

// NOTE: The overall data flow of a peer is split into 3 goroutines.  Inbound
// messages are read via the inHandler goroutine and generally dispatched to
// their own handler.  For inbound data-related messages such as blocks,
// transactions, and inventory, the data is handled by the corresponding
// message handlers.  The data flow for outbound messages is split into 2
// goroutines, queueHandler and outHandler.  The first, queueHandler, is used
// as a way for external entities to queue messages, by way of the QueueMessage
// function, quickly regardless of whether the peer is currently sending or not.
// It acts as the traffic cop between the external world and the actual
// goroutine which writes to the network socket.

// Peer provides a basic concurrent safe bitcoin peer for handling bitcoin
// communications via the peer-to-peer protocol.  It provides full duplex
// reading and writing, automatic handling of the initial handshake process,
// querying of usage statistics and other information about the remote peer such
// as its address, user agent, and protocol version, output message queueing,
// inventory trickling, and the ability to dynamically register and unregister
// callbacks for handling bitcoin protocol messages.
//
// Outbound messages are typically queued via QueueMessage or QueueInventory.
// QueueMessage is intended for all messages, including responses to data such
// as blocks and transactions.  QueueInventory, on the other hand, is only
// intended for relaying inventory as it employs a trickling mechanism to batch
// the inventory together.  However, some helper functions for pushing messages
// of specific types that typically require common special handling are
// provided as a convenience.
type Peer struct {
	conn net.Conn

	// These fields are set at creation time and never modified, so they are
	// safe to read from concurrently without a mutex.
	addr    string
	cfg     Config
	inbound bool

	id              int32
	na              *wire.NetAddress
	userAgent       string
	services        wire.ServiceFlag
	protocolVersion uint32
	version         *wire.MsgVersion

	knownInventory *mruInventoryMap

	prevGetBlocksMtx   sync.Mutex
	prevGetBlocksBegin *wire.ShaHash
	prevGetBlocksStop  *wire.ShaHash

	prevGetHdrsMtx   sync.Mutex
	prevGetHdrsBegin *wire.ShaHash
	prevGetHdrsStop  *wire.ShaHash

	// These fields keep track of statistics for the peer and are protected
	// by the statsMtx mutex.
	statsMtx           sync.RWMutex
	timeOffset         int64
	timeConnected      time.Time
	lastSend           time.Time
	lastRecv           time.Time
	bytesReceived      uint64
	bytesSent          uint64
	startingHeight     int32
	lastBlock          int32
	lastAnnouncedBlock *wire.ShaHash
	lastPingNonce      uint64    // Set to nonce if we have a pending ping.
	lastPingTime       time.Time // Time we sent last ping.
	lastPingMicros     int64     // Time for last ping to return.

	disconnectOnce      sync.Once
	disconnectWaitGroup sync.WaitGroup
	disconnect          chan struct{}

	write             chan writeMsg
	writeMsgQueue     chan writeMsg
	writeInvVectQueue chan *wire.InvVect

	responseDeadlinesMtx sync.Mutex
	responseDeadlines    map[string]*pausableTimer
}

// String returns the peer's address and directionality as a human-readable
// string.
//
// This function is safe for concurrent access.
func (p *Peer) String() string {
	return fmt.Sprintf("%s (%s)", p.addr, directionString(p.inbound))
}

// Version returns the peer's version that was ultimately negotiated.
//
// This function is safe for concurrent access.
func (p *Peer) Version() *wire.MsgVersion {
	return p.version
}

// UpdateLastBlockHeight updates the last known block for the peer.
//
// This function is safe for concurrent access.
func (p *Peer) UpdateLastBlockHeight(newHeight int32) {
	p.statsMtx.Lock()
	log.Tracef("Updating last block height of peer %v from %v to %v",
		p.addr, p.lastBlock, newHeight)
	p.lastBlock = int32(newHeight)
	p.statsMtx.Unlock()
}

// UpdateLastAnnouncedBlock updates meta-data about the last block sha this
// peer is known to have announced.
//
// This function is safe for concurrent access.
func (p *Peer) UpdateLastAnnouncedBlock(blkSha *wire.ShaHash) {
	log.Tracef("Updating last blk for peer %v, %v", p.addr, blkSha)

	p.statsMtx.Lock()
	p.lastAnnouncedBlock = blkSha
	p.statsMtx.Unlock()
}

// AddKnownInventory adds the passed inventory to the cache of known inventory
// for the peer.
//
// This function is safe for concurrent access.
func (p *Peer) AddKnownInventory(invVect *wire.InvVect) {
	p.knownInventory.Add(invVect)
}

// StatsSnapshot returns a snapshot of the current peer flags and statistics.
//
// This function is safe for concurrent access.
func (p *Peer) StatsSnapshot() *StatsSnap {
	p.statsMtx.RLock()
	defer p.statsMtx.RUnlock()

	id := p.id
	userAgent := p.userAgent
	services := p.services
	protocolVersion := p.protocolVersion

	// Get a copy of all relevant flags and stats.
	return &StatsSnap{
		ID:             id,
		Addr:           p.Addr(),
		UserAgent:      userAgent,
		Services:       services,
		LastSend:       p.lastSend,
		LastRecv:       p.lastRecv,
		BytesSent:      p.bytesSent,
		BytesRecv:      p.bytesReceived,
		ConnTime:       p.timeConnected,
		TimeOffset:     p.timeOffset,
		Version:        protocolVersion,
		Inbound:        p.inbound,
		StartingHeight: p.startingHeight,
		LastBlock:      p.lastBlock,
		LastPingNonce:  p.lastPingNonce,
		LastPingMicros: p.lastPingMicros,
		LastPingTime:   p.lastPingTime,
	}
}

// ID returns the peer id.
//
// This function is safe for concurrent access.
func (p *Peer) ID() int32 {
	return p.id
}

// NA returns the peer network address.
//
// This function is safe for concurrent access.
func (p *Peer) NA() *wire.NetAddress {
	return p.na
}

// Addr returns the peer address.
//
// This function is safe for concurrent access.
func (p *Peer) Addr() string {
	// The address doesn't change after initialization, therefore it is not
	// protected by a mutex.
	return p.addr
}

// Inbound returns whether the peer is inbound.
//
// This function is safe for concurrent access.
func (p *Peer) Inbound() bool {
	return p.inbound
}

// Services returns the services flag of the remote peer.
//
// This function is safe for concurrent access.
func (p *Peer) Services() wire.ServiceFlag {
	return p.services
}

// UserAgent returns the user agent of the remote peer.
//
// This function is safe for concurrent access.
func (p *Peer) UserAgent() string {
	return p.userAgent
}

// LastAnnouncedBlock returns the last announced block of the remote peer.
//
// This function is safe for concurrent access.
func (p *Peer) LastAnnouncedBlock() *wire.ShaHash {
	p.statsMtx.RLock()
	defer p.statsMtx.RUnlock()

	return p.lastAnnouncedBlock
}

// LastPingNonce returns the last ping nonce of the remote peer.
//
// This function is safe for concurrent access.
func (p *Peer) LastPingNonce() uint64 {
	p.statsMtx.RLock()
	defer p.statsMtx.RUnlock()

	return p.lastPingNonce
}

// LastPingTime returns the last ping time of the remote peer.
//
// This function is safe for concurrent access.
func (p *Peer) LastPingTime() time.Time {
	p.statsMtx.RLock()
	defer p.statsMtx.RUnlock()

	return p.lastPingTime
}

// LastPingMicros returns the last ping micros of the remote peer.
//
// This function is safe for concurrent access.
func (p *Peer) LastPingMicros() int64 {
	p.statsMtx.RLock()
	defer p.statsMtx.RUnlock()

	return p.lastPingMicros
}

// ProtocolVersion returns the peer protocol version.
//
// This function is safe for concurrent access.
func (p *Peer) ProtocolVersion() uint32 {
	return p.protocolVersion
}

// LastBlock returns the last block of the peer.
//
// This function is safe for concurrent access.
func (p *Peer) LastBlock() int32 {
	p.statsMtx.RLock()
	defer p.statsMtx.RUnlock()

	return p.lastBlock
}

// LastSend returns the last send time of the peer.
//
// This function is safe for concurrent access.
func (p *Peer) LastSend() time.Time {
	p.statsMtx.RLock()
	defer p.statsMtx.RUnlock()

	return p.lastSend
}

// LastRecv returns the last recv time of the peer.
//
// This function is safe for concurrent access.
func (p *Peer) LastRecv() time.Time {
	p.statsMtx.RLock()
	defer p.statsMtx.RUnlock()

	return p.lastRecv
}

// BytesSent returns the total number of bytes sent by the peer.
//
// This function is safe for concurrent access.
func (p *Peer) BytesSent() uint64 {
	p.statsMtx.RLock()
	defer p.statsMtx.RUnlock()

	return p.bytesSent
}

// BytesReceived returns the total number of bytes received by the peer.
//
// This function is safe for concurrent access.
func (p *Peer) BytesReceived() uint64 {
	p.statsMtx.RLock()
	defer p.statsMtx.RUnlock()

	return p.bytesReceived
}

// TimeConnected returns the time at which the peer connected.
//
// This function is safe for concurrent access.
func (p *Peer) TimeConnected() time.Time {
	p.statsMtx.RLock()
	defer p.statsMtx.RUnlock()

	return p.timeConnected
}

// TimeOffset returns the number of seconds the local time was offset from the
// time the peer reported during the initial negotiation phase.  Negative values
// indicate the remote peer's time is before the local time.
//
// This function is safe for concurrent access.
func (p *Peer) TimeOffset() int64 {
	p.statsMtx.RLock()
	defer p.statsMtx.RUnlock()

	return p.timeOffset
}

// StartingHeight returns the last known height the peer reported during the
// initial negotiation phase.
//
// This function is safe for concurrent access.
func (p *Peer) StartingHeight() int32 {
	p.statsMtx.RLock()
	defer p.statsMtx.RUnlock()

	return p.startingHeight
}

func (p *Peer) localMsgVersion() (*wire.MsgVersion, error) {
	var blockNum int32
	if p.cfg.NewestBlock != nil {
		var err error
		_, blockNum, err = p.cfg.NewestBlock()
		if err != nil {
			return nil, err
		}
	}

	theirNA := p.na

	// If we are behind a proxy and the connection comes from the proxy then
	// we return an unroutable address as their address. This is to prevent
	// leaking the tor proxy address.
	if p.cfg.Proxy != "" {
		proxyAddress, _, err := net.SplitHostPort(p.cfg.Proxy)
		// Invalid proxy means poorly configured, be on the safe side.
		if err != nil || p.na.IP.String() == proxyAddress {
			theirNA = &wire.NetAddress{
				Timestamp: time.Now(),
				IP:        net.IP([]byte{0, 0, 0, 0}),
			}
		}
	}

	// TODO(tuxcanfly): In case BestLocalAddress is nil, ourNA defaults to
	// remote NA, which is wrong. Need to fix this.
	ourNA := p.na
	if p.cfg.BestLocalAddress != nil {
		ourNA = p.cfg.BestLocalAddress(p.na)
	}

	// Generate a unique nonce for this peer so self connections can be
	// detected.  This is accomplished by adding it to a size-limited map of
	// recently seen nonces.
	nonce, err := wire.RandomUint64()
	if err != nil {
		return nil, err
	}
	sentNonces.Add(nonce)

	// Version message.
	msg := wire.NewMsgVersion(ourNA, theirNA, nonce, int32(blockNum))
	msg.AddUserAgent(p.cfg.UserAgentName, p.cfg.UserAgentVersion)

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
	msg.AddrYou.Services = wire.SFNodeNetwork

	// Advertise the services flag
	msg.Services = p.cfg.Services

	// Advertise our max supported protocol version.
	msg.ProtocolVersion = int32(p.ProtocolVersion())

	// Advertise if inv messages for transactions are desired.
	msg.DisableRelayTx = p.cfg.DisableRelayTx

	return msg, nil
}

// PushAddrMsg sends an addr message to the connected peer using the provided
// addresses.  This function is useful over manually sending the message via
// QueueMessage since it automatically limits the addresses to the maximum
// number allowed by the message and randomizes the chosen addresses when there
// are too many.  It returns the addresses that were actually sent and no
// message will be sent if there are no entries in the provided addresses slice.
//
// This function is safe for concurrent access.
func (p *Peer) PushAddrMsg(addresses []*wire.NetAddress) ([]*wire.NetAddress, error) {

	// Nothing to send.
	if len(addresses) == 0 {
		return nil, nil
	}

	msg := wire.NewMsgAddr()
	msg.AddrList = make([]*wire.NetAddress, len(addresses))
	copy(msg.AddrList, addresses)

	// Randomize the addresses sent if there are more than the maximum allowed.
	if len(msg.AddrList) > wire.MaxAddrPerMsg {
		// Shuffle the address list.
		for i := range msg.AddrList {
			j := rand.Intn(i + 1)
			msg.AddrList[i], msg.AddrList[j] = msg.AddrList[j], msg.AddrList[i]
		}

		// Truncate it to the maximum size.
		msg.AddrList = msg.AddrList[:wire.MaxAddrPerMsg]
	}

	p.QueueMessage(msg, nil)
	return msg.AddrList, nil
}

// PushGetBlocksMsg sends a getblocks message for the provided block locator
// and stop hash.  It will ignore back-to-back duplicate requests.
//
// This function is safe for concurrent access.
func (p *Peer) PushGetBlocksMsg(locator blockchain.BlockLocator, stopHash *wire.ShaHash) error {
	// Extract the begin hash from the block locator, if one was specified,
	// to use for filtering duplicate getblocks requests.
	var beginHash *wire.ShaHash
	if len(locator) > 0 {
		beginHash = locator[0]
	}

	// Filter duplicate getblocks requests.
	p.prevGetBlocksMtx.Lock()
	isDuplicate := p.prevGetBlocksStop != nil && p.prevGetBlocksBegin != nil &&
		beginHash != nil && stopHash.IsEqual(p.prevGetBlocksStop) &&
		beginHash.IsEqual(p.prevGetBlocksBegin)
	p.prevGetBlocksMtx.Unlock()

	if isDuplicate {
		log.Tracef(
			"Filtering duplicate [getblocks] with begin hash %v, stop hash %v",
			beginHash, stopHash)
		return nil
	}

	// Construct the getblocks request and queue it to be sent.
	msg := wire.NewMsgGetBlocks(stopHash)
	for _, hash := range locator {
		if err := msg.AddBlockLocatorHash(hash); err != nil {
			return err
		}
	}
	p.QueueMessage(msg, nil)

	// Update the previous getblocks request information for filtering
	// duplicates.
	p.prevGetBlocksMtx.Lock()
	p.prevGetBlocksBegin = beginHash
	p.prevGetBlocksStop = stopHash
	p.prevGetBlocksMtx.Unlock()
	return nil
}

// PushGetHeadersMsg sends a getblocks message for the provided block locator
// and stop hash.  It will ignore back-to-back duplicate requests.
//
// This function is safe for concurrent access.
func (p *Peer) PushGetHeadersMsg(locator blockchain.BlockLocator, stopHash *wire.ShaHash) error {
	// Extract the begin hash from the block locator, if one was specified,
	// to use for filtering duplicate getheaders requests.
	var beginHash *wire.ShaHash
	if len(locator) > 0 {
		beginHash = locator[0]
	}

	// Filter duplicate getheaders requests.
	p.prevGetHdrsMtx.Lock()
	isDuplicate := p.prevGetHdrsStop != nil && p.prevGetHdrsBegin != nil &&
		beginHash != nil && stopHash.IsEqual(p.prevGetHdrsStop) &&
		beginHash.IsEqual(p.prevGetHdrsBegin)
	p.prevGetHdrsMtx.Unlock()

	if isDuplicate {
		log.Tracef("Filtering duplicate [getheaders] with begin hash %v",
			beginHash)
		return nil
	}

	// Construct the getheaders request and queue it to be sent.
	msg := wire.NewMsgGetHeaders()
	msg.HashStop = *stopHash
	for _, hash := range locator {
		if err := msg.AddBlockLocatorHash(hash); err != nil {
			return err
		}
	}
	p.QueueMessage(msg, nil)

	// Update the previous getheaders request information for filtering
	// duplicates.
	p.prevGetHdrsMtx.Lock()
	p.prevGetHdrsBegin = beginHash
	p.prevGetHdrsStop = stopHash
	p.prevGetHdrsMtx.Unlock()
	return nil
}

// PushRejectMsg sends a reject message for the provided command, reject code,
// reject reason, and hash.  The hash will only be used when the command is a tx
// or block and should be nil in other cases.  The wait parameter will cause the
// function to block until the reject message has actually been sent.
//
// This function is safe for concurrent access.
func (p *Peer) PushRejectMsg(command string, code wire.RejectCode, reason string, hash *wire.ShaHash, wait bool) {
	// Don't bother sending the reject message if the protocol version
	// is too low.
	if p.ProtocolVersion() < wire.RejectVersion {
		return
	}

	msg := wire.NewMsgReject(command, code, reason)
	if command == wire.CmdTx || command == wire.CmdBlock {
		if hash == nil {
			log.Warnf("Sending a reject message for command "+
				"type %v which should have specified a hash "+
				"but does not", command)
			hash = &zeroHash
		}
		msg.Hash = *hash
	}

	// Send the message without waiting if the caller has not requested it.
	if !wait {
		p.QueueMessage(msg, nil)
		return
	}

	// Send the message and block until it has been sent before returning.
	doneChan := make(chan struct{}, 1)
	p.QueueMessage(msg, doneChan)
	<-doneChan
}

// handleVersionMsg is invoked when a peer receives a version bitcoin message
// and is used to negotiate the protocol version details as well as kick start
// the communications.
func (p *Peer) handleVersionMsg(msg *wire.MsgVersion) error {
	p.version = msg

	// Detect self connections.
	if !allowSelfConns && sentNonces.Exists(msg.Nonce) {
		return errors.New("disconnecting peer connected to self")
	}

	// Notify and disconnect clients that have a protocol version that is
	// too old.
	if msg.ProtocolVersion < int32(wire.MultipleAddressVersion) {
		// Send a reject message indicating the protocol version is
		// obsolete and wait for the message to be sent before
		// disconnecting.
		reason := fmt.Sprintf("protocol version must be %d or greater",
			wire.MultipleAddressVersion)

		// Ignore error.
		p.PushRejectMsg(msg.Command(), wire.RejectObsolete, reason, nil, true)
		return errors.New(reason)
	}

	// Updating a bunch of stats.
	p.lastBlock = msg.LastBlock
	p.startingHeight = msg.LastBlock
	// Set the peer's time offset.
	p.timeOffset = msg.Timestamp.Unix() - time.Now().Unix()

	// Negotiate the protocol version.
	if uint32(msg.ProtocolVersion) < p.protocolVersion {
		p.protocolVersion = uint32(msg.ProtocolVersion)
	}

	//p.versionKnown = true
	log.Debugf("Negotiated protocol version %d for peer %s",
		p.protocolVersion, p)
	// Set the peer's ID.
	p.id = atomic.AddInt32(&nodeCount, 1)
	// Set the supported services for the peer to what the remote peer
	// advertised.
	p.services = msg.Services
	// Set the remote peer's user agent.
	// Change this to remote useragent.
	p.userAgent = msg.UserAgent

	return nil
}

// isValidBIP0111 is a helper function for the bloom filter commands to check
// BIP0111 compliance.
func (p *Peer) isValidBIP0111(cmd string) bool {
	if p.Services()&wire.SFNodeBloom != wire.SFNodeBloom {
		if p.ProtocolVersion() >= wire.BIP0111Version {
			log.Debugf("%s sent an unsupported %s request -- disconnecting",
				p, cmd)
			p.Disconnect()
		} else {
			log.Debugf(
				"Ignoring %s request from %s -- bloom support is disabled",
				cmd, p)
		}
		return false
	}
	return true
}

// handlePingMsg is invoked when a peer receives a ping bitcoin message.  For
// recent clients (protocol version > BIP0031Version), it replies with a pong
// message.  For older clients, it does nothing and anything other than failure
// is considered a successful ping.
func (p *Peer) handlePingMsg(msg *wire.MsgPing) {
	// Only reply with pong if the message is from a new enough client.
	if p.ProtocolVersion() > wire.BIP0031Version {
		// Include nonce from ping so pong can be identified.
		// Ignore errors.
		p.QueueMessage(wire.NewMsgPong(msg.Nonce), nil)
	}
}

// handlePongMsg is invoked when a peer receives a pong bitcoin message.  It
// updates the ping statistics as required for recent clients (protocol
// version > BIP0031Version).  There is no effect for older clients or when a
// ping was not previously sent.
func (p *Peer) handlePongMsg(msg *wire.MsgPong) {
	p.statsMtx.Lock()
	defer p.statsMtx.Unlock()

	// Arguably we could use a buffered channel here sending data
	// in a fifo manner whenever we send a ping, or a list keeping track of
	// the times of each ping. For now we just make a best effort and
	// only record stats if it was for the last ping sent. Any preceding
	// and overlapping pings will be ignored. It is unlikely to occur
	// without large usage of the ping rpc call since we ping infrequently
	// enough that if they overlap we would have timed out the peer.
	if p.ProtocolVersion() > wire.BIP0031Version && p.lastPingNonce != 0 &&
		msg.Nonce == p.lastPingNonce {

		p.lastPingMicros = time.Now().Sub(p.lastPingTime).Nanoseconds()
		p.lastPingMicros /= 1000 // convert to usec.
		p.lastPingNonce = 0
	}
}

// readMessage reads the next bitcoin message from the peer with logging.
func (p *Peer) readMessage() (wire.Message, []byte, error) {
	n, msg, buf, err := wire.ReadMessageN(p.conn, p.ProtocolVersion(),
		p.cfg.ChainParams.Net)

	p.statsMtx.Lock()
	p.bytesReceived += uint64(n)
	p.statsMtx.Unlock()
	if p.cfg.Listeners.OnRead != nil {
		p.cfg.Listeners.OnRead(p, n, msg, err)
	}
	if err != nil {
		return nil, nil, err
	}

	// Use closures to log expensive operations so they are only run when
	// the logging level requires it.
	log.Debugf("%v", newLogClosure(func() string {
		// Debug summary of message.
		summary := messageSummary(msg)
		if len(summary) > 0 {
			summary = " (" + summary + ")"
		}
		return fmt.Sprintf("Received %v%s from %s",
			msg.Command(), summary, p)
	}))
	log.Tracef("%v", newLogClosure(func() string {
		return spew.Sdump(msg)
	}))
	log.Tracef("%v", newLogClosure(func() string {
		return spew.Sdump(buf)
	}))

	return msg, buf, nil
}

// writeMessage sends a bitcoin message to the peer with logging.
func (p *Peer) writeMessage(msg wire.Message) error {

	// Use closures to log expensive operations so they are only run when the
	// logging level requires it.
	log.Debugf("%v", newLogClosure(func() string {
		// Debug summary of message.
		summary := messageSummary(msg)
		if len(summary) > 0 {
			summary = " (" + summary + ")"
		}
		return fmt.Sprintf("Sending %v%s to %s", msg.Command(),
			summary, p)
	}))
	log.Tracef("%v", newLogClosure(func() string {
		return spew.Sdump(msg)
	}))
	log.Tracef("%v", newLogClosure(func() string {
		var buf bytes.Buffer
		err := wire.WriteMessage(&buf, msg, p.ProtocolVersion(),
			p.cfg.ChainParams.Net)
		if err != nil {
			return err.Error()
		}
		return spew.Sdump(buf.Bytes())
	}))

	// Write the message to the peer.
	n, err := wire.WriteMessageN(p.conn, msg, p.ProtocolVersion(),
		p.cfg.ChainParams.Net)
	p.statsMtx.Lock()
	p.bytesSent += uint64(n)
	p.statsMtx.Unlock()
	if p.cfg.Listeners.OnWrite != nil {
		p.cfg.Listeners.OnWrite(p, n, msg, err)
	}
	return err
}

// isAllowedByRegression returns whether or not the passed error is allowed by
// regression tests without disconnecting the peer.  In particular, regression
// tests need to be allowed to send malformed messages without the peer being
// disconnected.
func (p *Peer) isAllowedByRegression(err error) bool {
	// Don't allow the error if it's not specifically a malformed message
	// error.
	if _, ok := err.(*wire.MessageError); !ok {
		return false
	}

	// Don't allow the error if it's not coming from localhost or the
	// hostname can't be determined for some reason.
	if host, _, err := net.SplitHostPort(p.addr); err != nil {
		return false
	} else if host != "127.0.0.1" && host != "localhost" {
		return false
	}

	// Allowed if all checks passed.
	return true
}

// isRegTestNetwork returns whether or not the peer is running on the regression
// test network.
func (p *Peer) isRegTestNetwork() bool {
	return p.cfg.ChainParams.Net == wire.TestNet
}

// shouldHandleReadError returns whether or not the passed error, which is
// expected to have come from reading from the remote peer in the inHandler,
// should be logged and responded to with a reject message.
func (p *Peer) shouldHandleReadError(err error) bool {
	// No logging of reject message when the peer is being forcibly
	// disconnected.
	if !p.Connected() {
		return false
	}

	// No logging or reject message when the remote peer has been disconnected.
	if err == io.EOF {
		return false
	}
	if opErr, ok := err.(*net.OpError); ok && !opErr.Temporary() {
		return false
	}
	return true
}

// cmdData is a surrogate for getdata which can return a tx, block or notfound
// message.
const cmdData = "peer:data"

// maybeAddDeadline potentially adds a deadline for the appropriate expected
// response for the passed wire protocol command to the pending responses map.
func (p *Peer) maybeAddDeadline(msg wire.Message) {
	// Setup a deadline for each message being sent that expects a response.
	//
	// NOTE: Pings are intentionally ignored here since they are typically sent
	// asynchronously and as a result of a long backlock of messages, such as
	// is typical in the case of initial block download, the response won't be
	// received in time.

	timeout := stallResponseTimeout

	// The message commands that we expect a response by.
	responseCmd := ""
	switch msg.Command() {
	case wire.CmdVersion, wire.CmdMemPool, wire.CmdGetBlocks:
		responseCmd = wire.CmdInv
	case wire.CmdGetData:
		// Expects a block, tx, or notfound message message so use the cmdData
		// surrogate to encompass them all.
		responseCmd = cmdData
	case wire.CmdGetHeaders:
		// Expects a headers message.  Use a longer deadline since it
		// can take a while for the remote peer to load all of the
		// headers.
		timeout = timeout * 3
		responseCmd = wire.CmdHeaders
	}

	// Maybe the sent message is not expecting a response by a certain time.
	if responseCmd == "" {
		return
	}

	p.responseDeadlinesMtx.Lock()
	if _, ok := p.responseDeadlines[responseCmd]; !ok {

		// Add a pausable timer to disconnect the peer if required.
		t := pausableTimerAfterFunc(timeout, func() {
			log.Debugf("Timeout waiting for %v in response to %v.",
				responseCmd, msg.Command())
			p.Disconnect()
		})
		p.responseDeadlines[responseCmd] = t
	}
	p.responseDeadlinesMtx.Unlock()
}

// maybeRemoveDeadline returns false if a deadline was attempted to be stopped
// but had already been reached.
func (p *Peer) maybeRemoveDeadline(msg wire.Message) bool {
	responseCmd := msg.Command()
	switch msg.Command() {
	case wire.CmdBlock, wire.CmdTx, wire.CmdNotFound:
		// Use the surrogate cmdData to represent these message types.
		responseCmd = cmdData
	}

	success := true
	p.responseDeadlinesMtx.Lock()
	if timer, ok := p.responseDeadlines[responseCmd]; ok {
		success = timer.Stop()
		delete(p.responseDeadlines, responseCmd)
	}
	p.responseDeadlinesMtx.Unlock()
	return success
}

// pauseDeadlines will return true if all the timers were paused or false if a
// timer happened to expire while we called it.
func (p *Peer) pauseDeadlines() bool {
	success := true
	p.responseDeadlinesMtx.Lock()
	for _, timer := range p.responseDeadlines {
		if !timer.Pause() {
			success = false
		}
	}
	p.responseDeadlinesMtx.Unlock()
	return success
}

func (p *Peer) unpauseDeadlines() {
	p.responseDeadlinesMtx.Lock()
	for _, timer := range p.responseDeadlines {
		timer.Unpause()
	}
	p.responseDeadlinesMtx.Unlock()
}

func (p *Peer) readHandler() {
	defer p.disconnectWaitGroup.Done()

	for {
		read := make(chan readMsg)
		go func() {
			msg, buf, err := p.readMessage()
			read <- readMsg{msg, buf, err}
			close(read)
		}()

		select {
		case <-p.disconnect:
			return
		case rm := <-read:
			// Process message.
			if err := p.handleReadMsg(rm); err != nil {
				p.Disconnect()
			}
		case <-time.After(idleTimeout):
			// Deal with timeout.
			log.Warnf("Peer %s no answer for %s -- disconnecting",
				p, idleTimeout)
			p.Disconnect()
		}
	}
}

func (p *Peer) handleReadMsg(rm readMsg) error {
	if rm.err != nil {
		// In order to allow regression tests with malformed
		// messages, don't disconnect the peer when we're in
		// regression test mode and the error is one of the
		// allowed errors.
		if p.isRegTestNetwork() && p.isAllowedByRegression(rm.err) {
			log.Errorf("Allowed regression test error from %s: %v", p, rm.err)
			return nil
		}

		// Only log the error and send reject message if the
		// local peer is not forcibly disconnecting and the
		// remote peer has not disconnected.
		if p.shouldHandleReadError(rm.err) {
			errStr := fmt.Sprintf("Cannot read message from %s: %v", p, rm.err)
			log.Errorf(errStr)

			// Push a reject message for the malformed
			// message and wait for the message to be sent
			// before disconnecting.
			//
			// NOTE: Ideally this would include the command
			// in the header if at least that much of the
			// message was valid, but that is not currently
			// exposed by wire, so just used malformed for
			// the command.
			p.PushRejectMsg("malformed",
				wire.RejectMalformed, errStr, nil, true)
		}
		return rm.err
	}
	p.statsMtx.Lock()
	p.lastRecv = time.Now()
	p.statsMtx.Unlock()

	// On a slow running system by the time a message has been received and has
	// got to this point in processing a deadline might have expired.  This will
	// almost never be the case but still check to prevent a potential race
	// state.
	if !p.maybeRemoveDeadline(rm.msg) {
		return errors.New("deadline reached")
	}

	if !p.pauseDeadlines() {
		return errors.New("deadline reached")
	}
	defer p.unpauseDeadlines()

	// Handle each supported message type.
	switch msg := rm.msg.(type) {
	case *wire.MsgVersion:
		return errors.New("version already received")
	case *wire.MsgVerAck:
		return errors.New("verack already received")
	case *wire.MsgGetAddr:
		if p.cfg.Listeners.OnGetAddr != nil {
			p.cfg.Listeners.OnGetAddr(p, msg)
		}
	case *wire.MsgAddr:
		if p.cfg.Listeners.OnAddr != nil {
			p.cfg.Listeners.OnAddr(p, msg)
		}
	case *wire.MsgPing:
		p.handlePingMsg(msg)
		if p.cfg.Listeners.OnPing != nil {
			p.cfg.Listeners.OnPing(p, msg)
		}
	case *wire.MsgPong:
		p.handlePongMsg(msg)
		if p.cfg.Listeners.OnPong != nil {
			p.cfg.Listeners.OnPong(p, msg)
		}
	case *wire.MsgAlert:
		if p.cfg.Listeners.OnAlert != nil {
			p.cfg.Listeners.OnAlert(p, msg)
		}
	case *wire.MsgMemPool:
		if p.cfg.Listeners.OnMemPool != nil {
			p.cfg.Listeners.OnMemPool(p, msg)
		}
	case *wire.MsgTx:
		if p.cfg.Listeners.OnTx != nil {
			p.cfg.Listeners.OnTx(p, msg)
		}
	case *wire.MsgBlock:
		if p.cfg.Listeners.OnBlock != nil {
			p.cfg.Listeners.OnBlock(p, msg, rm.buf)
		}
	case *wire.MsgInv:
		if p.cfg.Listeners.OnInv != nil {
			p.cfg.Listeners.OnInv(p, msg)
		}
	case *wire.MsgHeaders:
		if p.cfg.Listeners.OnHeaders != nil {
			p.cfg.Listeners.OnHeaders(p, msg)
		}
	case *wire.MsgNotFound:
		if p.cfg.Listeners.OnNotFound != nil {
			p.cfg.Listeners.OnNotFound(p, msg)
		}
	case *wire.MsgGetData:
		if p.cfg.Listeners.OnGetData != nil {
			p.cfg.Listeners.OnGetData(p, msg)
		}
	case *wire.MsgGetBlocks:
		if p.cfg.Listeners.OnGetBlocks != nil {
			p.cfg.Listeners.OnGetBlocks(p, msg)
		}
	case *wire.MsgGetHeaders:
		if p.cfg.Listeners.OnGetHeaders != nil {
			p.cfg.Listeners.OnGetHeaders(p, msg)
		}
	case *wire.MsgFilterAdd:
		if p.isValidBIP0111(
			msg.Command()) && p.cfg.Listeners.OnFilterAdd != nil {
			p.cfg.Listeners.OnFilterAdd(p, msg)
		}
	case *wire.MsgFilterClear:
		if p.isValidBIP0111(
			msg.Command()) && p.cfg.Listeners.OnFilterClear != nil {
			p.cfg.Listeners.OnFilterClear(p, msg)
		}
	case *wire.MsgFilterLoad:
		if p.isValidBIP0111(
			msg.Command()) && p.cfg.Listeners.OnFilterLoad != nil {
			p.cfg.Listeners.OnFilterLoad(p, msg)
		}
	case *wire.MsgMerkleBlock:
		if p.cfg.Listeners.OnMerkleBlock != nil {
			p.cfg.Listeners.OnMerkleBlock(p, msg)
		}
	case *wire.MsgReject:
		if p.cfg.Listeners.OnReject != nil {
			p.cfg.Listeners.OnReject(p, msg)
		}
	default:
		return fmt.Errorf("unexpected message %v", msg.Command())
	}
	return nil
}

func (p *Peer) writeMsgQueueHandler() {
	defer p.disconnectWaitGroup.Done()

	pendingMsgs := list.New()
	for {
		for {
			elem := pendingMsgs.Front()
			if elem == nil {
				break
			}
			select {
			case <-p.disconnect:
				return
			case p.write <- elem.Value.(writeMsg):
				pendingMsgs.Remove(elem)
			default:
				break
			}
		}

		select {
		case <-p.disconnect:
			return
		case writeMsg := <-p.writeMsgQueue:
			pendingMsgs.PushBack(writeMsg)
		}
	}
}

func (p *Peer) writeInvVectQueueHandler() {
	defer p.disconnectWaitGroup.Done()

	trickleTicker := time.NewTicker(trickleTimeout)
	defer trickleTicker.Stop()

	invVects := []*wire.InvVect{}
	for {
		select {
		case <-p.disconnect:
			return
		case invVect := <-p.writeInvVectQueue:
			invVects = append(invVects, invVect)
		case <-trickleTicker.C:

			invMsg := wire.NewMsgInvSizeHint(uint(len(invVects)))
			for _, invVect := range invVects {
				if p.knownInventory.Exists(invVect) {
					continue
				}

				invMsg.AddInvVect(invVect)
				if len(invMsg.InvList) >= maxInvTrickleSize {
					p.QueueMessage(invMsg, nil)
					invMsg = wire.NewMsgInvSizeHint(uint(len(invVects)))
				}
				p.AddKnownInventory(invVect)
			}
			invVects = []*wire.InvVect{}

			if len(invMsg.InvList) > 0 {
				p.QueueMessage(invMsg, nil)
			}
		}
	}
}

func (p *Peer) writeHandler() {
	defer p.disconnectWaitGroup.Done()

	for {
		select {
		case <-p.disconnect:
			return
		case writeMsg := <-p.write:
			switch m := writeMsg.msg.(type) {
			case *wire.MsgPing:
				if p.ProtocolVersion() > wire.BIP0031Version {
					p.statsMtx.Lock()
					p.lastPingNonce = m.Nonce
					p.lastPingTime = time.Now()
					p.statsMtx.Unlock()
				}
			}

			err := p.writeMessage(writeMsg.msg)
			if writeMsg.done != nil {
				close(writeMsg.done)
			}
			if err != nil {
				if p.shouldLogWriteError(err) {
					log.Errorf("Failed to send message to %s: %v.", p, err)
				}
				p.Disconnect()
				return
			}

			p.maybeAddDeadline(writeMsg.msg)
		}
	}
}

// shouldLogWriteError returns whether or not the passed error, which is
// expected to have come from writing to the remote peer in the outHandler,
// should be logged.
func (p *Peer) shouldLogWriteError(err error) bool {
	// No logging when the peer is being forcibly disconnected.
	if !p.Connected() {
		return false
	}

	// No logging when the remote peer has been disconnected.
	if err == io.EOF {
		return false
	}
	if opErr, ok := err.(*net.OpError); ok && !opErr.Temporary() {
		return false
	}

	return true
}

func (p *Peer) pingTicker() {
	defer p.disconnectWaitGroup.Done()

	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

	for {
		select {
		case <-p.disconnect:
			return
		case <-pingTicker.C:
			nonce, err := wire.RandomUint64()
			if err != nil {
				log.Errorf("Not sending ping to %s: %v.", p, err)
				continue
			}
			p.QueueMessage(wire.NewMsgPing(nonce), nil)
		}
	}
}

// QueueMessage adds the passed bitcoin message to the peer send queue.
//
// This function is safe for concurrent access.
func (p *Peer) QueueMessage(msg wire.Message, done chan<- struct{}) {
	if !p.Connected() {
		if done != nil {
			go func() {
				done <- struct{}{}
			}()
		}
		return
	}
	p.writeMsgQueue <- writeMsg{msg, done}
	return
}

// QueueInventory adds the passed inventory to the inventory send queue which
// might not be sent right away, rather it is trickled to the peer in batches.
// Inventory that the peer is already known to have is ignored.
//
// This function is safe for concurrent access.
func (p *Peer) QueueInventory(invVect *wire.InvVect) {

	if p.knownInventory.Exists(invVect) {
		return
	}

	if !p.Connected() {
		return
	}

	p.writeInvVectQueue <- invVect
}

// Connected returns whether or not the peer is currently connected.
//
// This function is safe for concurrent access.
func (p *Peer) Connected() bool {
	select {
	case <-p.disconnect:
		return false
	default:
		return true
	}
}

// Disconnect gracefully shuts down the peer by disconnecting it.
func (p *Peer) Disconnect() error {

	// If the read and/or write goroutine are blocking when p.conn.Close() is
	// called then they will generate errors and attempt to also call Disconnect.
	// This prevents p.disconnect being closed multiple times.
	p.disconnectOnce.Do(func() {
		close(p.disconnect)
	})

	// Cancel all deadline timers.
	p.responseDeadlinesMtx.Lock()
	for cmd, timer := range p.responseDeadlines {
		timer.Stop()
		delete(p.responseDeadlines, cmd)
	}
	p.responseDeadlinesMtx.Unlock()
	return p.conn.Close()
}

// WaitForDisconnect waits until the peer has completely disconnected. This will
// happen if either the local or remote side has been disconnected or the peer
// is forcibly disconnectd via Disconnect.
func (p *Peer) WaitForDisconnect() {
	p.disconnectWaitGroup.Wait()
}

// newPeerBase returns a new base bitcoin peer based on the inbound flag.  This
// is used by the NewInboundPeer and NewOutboundPeer functions to perform base
// setup needed by both types of peers.
func newPeerBase(cfg *Config, inbound bool) *Peer {
	// Default to the max supported protocol version.  Override to the
	// version specified by the caller if configured.
	protocolVersion := uint32(MaxProtocolVersion)
	if cfg.ProtocolVersion != 0 {
		protocolVersion = cfg.ProtocolVersion
	}

	// Set the chain parameters to testnet if the caller did not specify any.
	if cfg.ChainParams == nil {
		cfg.ChainParams = &chaincfg.TestNet3Params
	}

	return &Peer{
		inbound:         inbound,
		knownInventory:  newMruInventoryMap(maxKnownInventory),
		cfg:             *cfg, // Copy so caller can't mutate.
		protocolVersion: protocolVersion,

		disconnect: make(chan struct{}),

		write:             make(chan writeMsg),
		writeMsgQueue:     make(chan writeMsg),
		writeInvVectQueue: make(chan *wire.InvVect),

		responseDeadlines: make(map[string]*pausableTimer),
	}
}

func (p *Peer) negotiateInboundVersion() error {

	// Wait for the remote's version.
	msg, _, err := p.readMessage()
	if err != nil {
		return err
	}

	verMsg, ok := msg.(*wire.MsgVersion)
	if !ok {
		return fmt.Errorf("unexpected message %T", msg)
	}

	// Handle remote version message.
	if err := p.handleVersionMsg(verMsg); err != nil {
		return err
	}

	if p.cfg.Listeners.OnVersion != nil {
		p.cfg.Listeners.OnVersion(p, verMsg)
	}

	// Send our version information.
	verMsg, err = p.localMsgVersion()
	if err != nil {
		return err
	}
	if err := p.writeMessage(verMsg); err != nil {
		return err
	}

	// Now wait for their acknowledgment.
	msg, _, err = p.readMessage()
	if err != nil {
		return err
	}
	verAckMsg, ok := msg.(*wire.MsgVerAck)
	if !ok {
		return fmt.Errorf("unexpected message %T", msg)
	}

	if p.cfg.Listeners.OnVerAck != nil {
		p.cfg.Listeners.OnVerAck(p, verAckMsg)
	}

	// Send our version acknowledgement.
	return p.writeMessage(wire.NewMsgVerAck())
}

// NewInboundPeer returns a new inbound bitcoin peer. Use Start to begin
// processing incoming and outgoing messages.
func NewInboundPeer(cfg *Config, conn net.Conn) (*Peer, error) {
	p := newPeerBase(cfg, true)
	p.addr = conn.RemoteAddr().String()

	na, err := newNetAddress(conn.RemoteAddr(), p.cfg.Services)
	if err != nil {
		return nil, err
	}
	p.na = na

	if err := startPeer(p, conn, p.negotiateInboundVersion); err != nil {
		return nil, conn.Close()
	}
	return p, nil
}

func (p *Peer) negotiateOutboundVersion() error {

	// Send our version information.
	verMsg, err := p.localMsgVersion()
	if err != nil {
		return err
	}
	if err := p.writeMessage(verMsg); err != nil {
		return err
	}

	// Wait for their version response.
	msg, _, err := p.readMessage()
	if err != nil {
		return err
	}

	verMsg, ok := msg.(*wire.MsgVersion)
	if !ok {
		return fmt.Errorf("unexpected message %T", msg)
	}

	// Check to see if we are compatible with their version.
	if err := p.handleVersionMsg(verMsg); err != nil {
		return err
	}

	if p.cfg.Listeners.OnVersion != nil {
		p.cfg.Listeners.OnVersion(p, verMsg)
	}

	// Send our version acknowledgement.
	if err := p.writeMessage(wire.NewMsgVerAck()); err != nil {
		return err
	}

	// Now wait for their acknowledgment.
	msg, _, err = p.readMessage()
	if err != nil {
		return err
	}
	verAckMsg, ok := msg.(*wire.MsgVerAck)
	if !ok {
		return fmt.Errorf("unexpected message %T", msg)
	}

	if p.cfg.Listeners.OnVerAck != nil {
		p.cfg.Listeners.OnVerAck(p, verAckMsg)
	}
	return nil
}

// NewOutboundPeer returns a new outbound bitcoin peer.
func NewOutboundPeer(cfg *Config, conn net.Conn, addr string) (*Peer, error) {
	p := newPeerBase(cfg, false)
	p.addr = addr

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}

	if cfg.HostToNetAddress != nil {
		na, err := cfg.HostToNetAddress(host, uint16(port), cfg.Services)
		if err != nil {
			return nil, err
		}
		p.na = na
	} else {
		p.na = wire.NewNetAddressIPPort(net.ParseIP(host), uint16(port),
			cfg.Services)
	}

	if err := startPeer(p, conn, p.negotiateOutboundVersion); err != nil {
		return nil, conn.Close()
	}
	return p, nil
}

func startPeer(p *Peer, conn net.Conn, negotiator func() error) error {
	p.conn = conn
	p.timeConnected = time.Now()

	negotiateErr := make(chan error)
	go func() {
		negotiateErr <- negotiator()
		close(negotiateErr)
	}()
	select {
	case err := <-negotiateErr:
		if err != nil {
			return err
		}
	case <-time.After(negotiateTimeout):
		return errors.New("protocol negotiation timeout")
	}

	// The protocol has been negotiated successfully so start fully duplexed
	// communication with the node.
	p.disconnectWaitGroup.Add(5)
	go p.writeHandler()
	go p.writeMsgQueueHandler()
	go p.writeInvVectQueueHandler()
	go p.readHandler()
	go p.pingTicker()

	return nil
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
