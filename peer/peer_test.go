// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer_test

import (
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/go-socks/socks"
)

// conn mocks a network connection by implementing the net.Conn interface.  It
// is used to test peer connection without actually opening a network
// connection.
type conn struct {
	io.Reader
	io.Writer
	io.Closer

	// local network, address for the connection.
	laddr net.Addr
	// remote network, address for the connection.
	raddr net.Addr

	// mocks socks proxy if true
	proxy bool
}

// LocalAddr returns the local address for the connection.
func (c conn) LocalAddr() net.Addr {
	return c.laddr
}

// Remote returns the remote address for the connection.
func (c conn) RemoteAddr() net.Addr {
	if !c.proxy {
		return c.raddr
	}

	host, strPort, _ := net.SplitHostPort(c.raddr.String())
	port, _ := strconv.Atoi(strPort)
	return &socks.ProxiedAddr{
		Net:  c.raddr.Network(),
		Host: host,
		Port: port,
	}
}

// Close handles closing the connection.
func (c conn) Close() error {
	return nil
}

func (c conn) SetDeadline(t time.Time) error      { return nil }
func (c conn) SetReadDeadline(t time.Time) error  { return nil }
func (c conn) SetWriteDeadline(t time.Time) error { return nil }

// addr mocks a network address
type addr struct {
	net, address string
}

func (m addr) Network() string { return m.net }
func (m addr) String() string  { return m.address }

// pipe turns two mock connections into a full-duplex connection similar to
// net.Pipe to allow pipe's with (fake) addresses.
func pipe(c1, c2 *conn) (*conn, *conn) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	c1.Writer = w1
	c2.Reader = r1
	c1.Reader = r2
	c2.Writer = w2

	return c1, c2
}

// peerStats holds the expected peer stats used for testing peer.
type peerStats struct {
	wantUserAgent       string
	wantServices        wire.ServiceFlag
	wantProtocolVersion uint32
	wantLastBlock       int32
	wantStartingHeight  int32
	wantLastPingTime    time.Time
	wantLastPingNonce   uint64
	wantLastPingMicros  int64
	wantTimeOffset      int64
	wantBytesSent       uint64
	wantBytesReceived   uint64
}

// testPeer tests the given peer's flags and stats
func testPeer(t *testing.T, p *peer.Peer, s peerStats) {
	if p.UserAgent() != s.wantUserAgent {
		t.Fatalf("testPeer: wrong UserAgent - got %v, want %v",
			p.UserAgent(), s.wantUserAgent)
	}

	if p.Services() != s.wantServices {
		t.Fatalf("testPeer: wrong Services - got %v, want %v",
			p.Services(), s.wantServices)
	}

	if !p.LastPingTime().Equal(s.wantLastPingTime) {
		t.Fatalf("testPeer: wrong LastPingTime - got %v, want %v",
			p.LastPingTime(), s.wantLastPingTime)
	}

	if p.LastPingNonce() != s.wantLastPingNonce {
		t.Fatalf("testPeer: wrong LastPingNonce - got %v, want %v",
			p.LastPingNonce(), s.wantLastPingNonce)
	}

	if p.LastPingMicros() != s.wantLastPingMicros {
		t.Fatalf("testPeer: wrong LastPingMicros - got %v, want %v",
			p.LastPingMicros(), s.wantLastPingMicros)
	}

	if p.ProtocolVersion() != s.wantProtocolVersion {
		t.Fatalf("testPeer: wrong ProtocolVersion - got %v, want %v",
			p.ProtocolVersion(), s.wantProtocolVersion)
	}

	if p.LastBlock() != s.wantLastBlock {
		t.Fatalf("testPeer: wrong LastBlock - got %v, want %v",
			p.LastBlock(), s.wantLastBlock)
	}

	// Allow for a deviation of 1s, as the second may tick when the message is
	// in transit and the protocol doesn't support any further precision.
	if p.TimeOffset() != s.wantTimeOffset && p.TimeOffset() != s.wantTimeOffset-1 {
		t.Fatalf("testPeer: wrong TimeOffset - got %v, want %v or %v",
			p.TimeOffset(), s.wantTimeOffset, s.wantTimeOffset-1)
	}

	if p.BytesSent() != s.wantBytesSent {
		t.Fatalf("testPeer: wrong BytesSent - got %v, want %v",
			p.BytesSent(), s.wantBytesSent)
	}

	if p.BytesReceived() != s.wantBytesReceived {
		t.Fatalf("testPeer: wrong BytesReceived - got %v, want %v",
			p.BytesReceived(), s.wantBytesReceived)
	}

	if p.StartingHeight() != s.wantStartingHeight {
		t.Fatalf("testPeer: wrong StartingHeight - got %v, want %v",
			p.StartingHeight(), s.wantStartingHeight)
	}

	stats := p.StatsSnapshot()

	if p.ID() != stats.ID {
		t.Fatalf("testPeer: wrong ID - got %v, want %v", p.ID(), stats.ID)
	}

	if p.Addr() != stats.Addr {
		t.Fatalf("testPeer: wrong Addr - got %v, want %v", p.Addr(), stats.Addr)
	}

	if p.LastSend() != stats.LastSend {
		t.Fatalf("testPeer: wrong LastSend - got %v, want %v",
			p.LastSend(), stats.LastSend)
	}

	if p.LastRecv() != stats.LastRecv {
		t.Fatalf("testPeer: wrong LastRecv - got %v, want %v",
			p.LastRecv(), stats.LastRecv)
	}
}

// TestPeerConnection tests connection between inbound and outbound peers.
func TestPeerConnection(t *testing.T) {
	verack := make(chan struct{}, 2)
	peerCfg := &peer.Config{
		Listeners: peer.MessageListeners{
			OnWrite: func(p *peer.Peer, bytesWritten int,
				msg wire.Message, err error) {
				switch msg.(type) {
				case *wire.MsgVerAck:
					verack <- struct{}{}
				}
			},
		},
		UserAgentName:    "peer",
		UserAgentVersion: "1.0",
		ChainParams:      &chaincfg.MainNetParams,
		Services:         0,
	}
	localAddr, err := net.ResolveTCPAddr("tcp", "10.0.0.1:8333")
	if err != nil {
		t.Fatal(err)
	}
	remoteAddr, err := net.ResolveTCPAddr("tcp", "10.0.0.2:8333")
	if err != nil {
		t.Fatal(err)
	}
	wantStats := peerStats{
		wantUserAgent:       wire.DefaultUserAgent + "peer:1.0/",
		wantServices:        0,
		wantProtocolVersion: peer.MaxProtocolVersion,
		wantLastPingTime:    time.Time{},
		wantLastPingNonce:   uint64(0),
		wantLastPingMicros:  int64(0),
		wantTimeOffset:      int64(0),
		wantBytesSent:       158, // 134 version + 24 verack
		wantBytesReceived:   158,
	}
	tests := []struct {
		name  string
		setup func() (*peer.Peer, *peer.Peer, error)
	}{
		{
			"basic handshake",
			func() (*peer.Peer, *peer.Peer, error) {
				inConn, outConn := pipe(
					&conn{raddr: localAddr},
					&conn{raddr: remoteAddr},
				)

				var inPeer, outPeer *peer.Peer
				var inPeerErr, outPeerErr error
				var wg sync.WaitGroup
				wg.Add(2)
				go func() {
					inPeer, inPeerErr = peer.NewInboundPeer(peerCfg, inConn)
					wg.Done()
				}()
				go func() {
					outPeer, outPeerErr = peer.NewOutboundPeer(
						peerCfg, outConn, outConn.RemoteAddr().String())
					wg.Done()
				}()
				wg.Wait()

				if inPeerErr != nil || outPeerErr != nil {
					t.Fatalf("In err: %v, out err: %v", inPeerErr, outPeerErr)
				}
				for i := 0; i < 2; i++ {
					select {
					case <-verack:
					case <-time.After(time.Second * 1):
						return nil, nil, errors.New("verack timeout")
					}
				}
				return inPeer, outPeer, nil
			},
		},
		{
			"socks proxy",
			func() (*peer.Peer, *peer.Peer, error) {
				inConn, outConn := pipe(
					&conn{raddr: localAddr, proxy: true},
					&conn{raddr: remoteAddr},
				)

				var inPeer, outPeer *peer.Peer
				var inPeerErr, outPeerErr error
				var wg sync.WaitGroup
				wg.Add(2)
				go func() {
					inPeer, inPeerErr = peer.NewInboundPeer(peerCfg, inConn)
					wg.Done()
				}()
				go func() {
					outPeer, outPeerErr = peer.NewOutboundPeer(peerCfg,
						outConn, outConn.RemoteAddr().String())
					wg.Done()
				}()
				wg.Wait()

				if inPeerErr != nil || outPeerErr != nil {
					t.Fatalf("In err: %v, out err: %v", inPeerErr, outPeerErr)
				}
				for i := 0; i < 2; i++ {
					select {
					case <-verack:
					case <-time.After(time.Second * 1):
						return nil, nil, errors.New("verack timeout")
					}
				}
				return inPeer, outPeer, nil
			},
		},
	}
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		inPeer, outPeer, err := test.setup()
		if err != nil {
			t.Fatalf("TestPeerConnection setup #%d: unexpected err %v\n",
				i, err)
		}
		testPeer(t, inPeer, wantStats)
		testPeer(t, outPeer, wantStats)

		inPeer.Disconnect()
		outPeer.Disconnect()
	}
}

// TestPeerListeners tests that the peer listeners are called as expected.
func TestPeerListeners(t *testing.T) {
	ok := make(chan wire.Message, 20)
	inPeerCfg := peer.Config{
		Listeners: peer.MessageListeners{
			OnGetAddr: func(p *peer.Peer, msg *wire.MsgGetAddr) {
				ok <- msg
			},
			OnAddr: func(p *peer.Peer, msg *wire.MsgAddr) {
				ok <- msg
			},
			OnPing: func(p *peer.Peer, msg *wire.MsgPing) {
				ok <- msg
			},
			OnPong: func(p *peer.Peer, msg *wire.MsgPong) {
				ok <- msg
			},
			OnAlert: func(p *peer.Peer, msg *wire.MsgAlert) {
				ok <- msg
			},
			OnMemPool: func(p *peer.Peer, msg *wire.MsgMemPool) {
				ok <- msg
			},
			OnTx: func(p *peer.Peer, msg *wire.MsgTx) {
				ok <- msg
			},
			OnBlock: func(p *peer.Peer, msg *wire.MsgBlock, buf []byte) {
				ok <- msg
			},
			OnInv: func(p *peer.Peer, msg *wire.MsgInv) {
				ok <- msg
			},
			OnHeaders: func(p *peer.Peer, msg *wire.MsgHeaders) {
				ok <- msg
			},
			OnNotFound: func(p *peer.Peer, msg *wire.MsgNotFound) {
				ok <- msg
			},
			OnGetData: func(p *peer.Peer, msg *wire.MsgGetData) {
				ok <- msg
			},
			OnGetBlocks: func(p *peer.Peer, msg *wire.MsgGetBlocks) {
				ok <- msg
			},
			OnGetHeaders: func(p *peer.Peer, msg *wire.MsgGetHeaders) {
				ok <- msg
			},
			OnFilterAdd: func(p *peer.Peer, msg *wire.MsgFilterAdd) {
				ok <- msg
			},
			OnFilterClear: func(p *peer.Peer, msg *wire.MsgFilterClear) {
				ok <- msg
			},
			OnFilterLoad: func(p *peer.Peer, msg *wire.MsgFilterLoad) {
				ok <- msg
			},
			OnMerkleBlock: func(p *peer.Peer, msg *wire.MsgMerkleBlock) {
				ok <- msg
			},
			OnReject: func(p *peer.Peer, msg *wire.MsgReject) {
				ok <- msg
			},
		},
		UserAgentName:    "peer",
		UserAgentVersion: "1.0",
		ChainParams:      &chaincfg.MainNetParams,
		Services:         wire.SFNodeBloom,
	}
	localAddr, err := net.ResolveTCPAddr("tcp", "10.0.0.1:8333")
	if err != nil {
		t.Fatal(err)
	}
	remoteAddr, err := net.ResolveTCPAddr("tcp", "10.0.0.2:8333")
	if err != nil {
		t.Fatal(err)
	}
	inConn, outConn := pipe(
		&conn{raddr: localAddr},
		&conn{raddr: remoteAddr},
	)

	var inPeer, outPeer *peer.Peer
	var inPeerErr, outPeerErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		inPeer, inPeerErr = peer.NewInboundPeer(&inPeerCfg, inConn)
		wg.Done()
	}()

	outPeerCfg := inPeerCfg
	go func() {
		outPeer, outPeerErr = peer.NewOutboundPeer(&outPeerCfg, outConn,
			outConn.RemoteAddr().String())
		wg.Done()
	}()
	wg.Wait()

	if inPeerErr != nil || outPeerErr != nil {
		t.Fatalf("In err: %v, out err: %v", inPeerErr, outPeerErr)
	}

	tests := []struct {
		listener string
		msg      wire.Message
	}{
		{
			"OnGetAddr",
			wire.NewMsgGetAddr(),
		},
		{
			"OnAddr",
			wire.NewMsgAddr(),
		},
		{
			"OnPing",
			wire.NewMsgPing(42),
		},
		{
			"OnPong",
			wire.NewMsgPong(42),
		},
		{
			"OnAlert",
			wire.NewMsgAlert([]byte("payload"), []byte("signature")),
		},
		{
			"OnMemPool",
			wire.NewMsgMemPool(),
		},
		{
			"OnTx",
			wire.NewMsgTx(),
		},
		{
			"OnBlock",
			wire.NewMsgBlock(
				wire.NewBlockHeader(&wire.ShaHash{}, &wire.ShaHash{}, 1, 1)),
		},
		{
			"OnInv",
			wire.NewMsgInv(),
		},
		{
			"OnHeaders",
			wire.NewMsgHeaders(),
		},
		{
			"OnNotFound",
			wire.NewMsgNotFound(),
		},
		{
			"OnGetData",
			wire.NewMsgGetData(),
		},
		{
			"OnGetBlocks",
			wire.NewMsgGetBlocks(&wire.ShaHash{}),
		},
		{
			"OnGetHeaders",
			wire.NewMsgGetHeaders(),
		},
		{
			"OnFilterAdd",
			wire.NewMsgFilterAdd([]byte{0x01}),
		},
		{
			"OnFilterClear",
			wire.NewMsgFilterClear(),
		},
		{
			"OnFilterLoad",
			wire.NewMsgFilterLoad([]byte{0x01}, 10, 0, wire.BloomUpdateNone),
		},
		{
			"OnMerkleBlock",
			wire.NewMsgMerkleBlock(
				wire.NewBlockHeader(&wire.ShaHash{}, &wire.ShaHash{}, 1, 1)),
		},
		// only one version message is allowed
		// only one verack message is allowed
		{
			"OnMsgReject",
			wire.NewMsgReject("block", wire.RejectDuplicate, "dupe block"),
		},
	}
	t.Logf("Running %d tests", len(tests))
	for _, test := range tests {
		// Queue the test message
		done := make(chan struct{})
		outPeer.QueueMessage(test.msg, done)
		<-done

		select {
		case <-ok:
		case <-time.After(time.Second * 1):
			t.Fatalf("TestPeerListeners: %s timeout", test.listener)
		}
	}
	inPeer.Disconnect()
	outPeer.Disconnect()
}

func init() {
	// Allow self connection when running the tests.
	peer.TstAllowSelfConns()
}
