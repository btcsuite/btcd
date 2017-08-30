// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync_test

import (
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

/* This file contains mock structs and helper functions that are shared by tests
 * in the netsync_test package.
 */

// SimpleAddr implements the net.Addr using struct fields
type SimpleAddr struct {
	net, addr string
}

func (a SimpleAddr) Network() string { return a.net }
func (a SimpleAddr) String() string  { return a.addr }

// pipeConn mocks a network connection by implementing the net.Conn interface.
// It is used to test peer connection without actually opening a network
// connection.
type pipeConn struct {
	*io.PipeReader
	*io.PipeWriter

	localAddr, remoteAddr net.Addr
}

func (conn *pipeConn) Close() error {
	err := conn.PipeReader.Close()
	err1 := conn.PipeWriter.Close()
	if err == nil {
		err = err1
	}
	return err
}

func (conn *pipeConn) LocalAddr() net.Addr {
	return conn.localAddr
}

func (conn *pipeConn) RemoteAddr() net.Addr {
	return conn.remoteAddr
}

func (conn *pipeConn) SetDeadline(t time.Time) error {
	return errors.New("deadline not supported")
}

func (conn *pipeConn) SetReadDeadline(t time.Time) error {
	return errors.New("deadline not supported")
}

func (conn *pipeConn) SetWriteDeadline(t time.Time) error {
	return errors.New("deadline not supported")
}

// Pipe creates a synchronous, in-memory, full duplex network connection; both
// ends implement the net.Conn interface. Reads on one end are matched with
// writes on the other, copying data directly between the two; there is no
// internal buffering.
//
// The first address parameter is the local address of the first returned
// connection and the remote address of the second returned connection. The
// second address parameter is the opposite.
func Pipe(addr1, addr2 net.Addr) (net.Conn, net.Conn) {
	conn1 := pipeConn{localAddr: addr1, remoteAddr: addr2}
	conn2 := pipeConn{localAddr: addr2, remoteAddr: addr1}
	conn1.PipeReader, conn2.PipeWriter = io.Pipe()
	conn2.PipeReader, conn1.PipeWriter = io.Pipe()
	return &conn1, &conn2
}

// MakeConnectedPeers constructs a pair of peers that are connected to each
// other and returns them. They are constructed such that the second returned
// peer connects to the first returned peer so the first sees an inbound
// connection and the second sees an outbound connection. The index parameter
// is used in the network addresses of the peers to create peers with unique
// IPs.
func MakeConnectedPeers(inboundCfg peer.Config, outboundCfg peer.Config, index uint8) (*peer.Peer, *peer.Peer, error) {
	// Must enable this in tests to avoid automatic disconnection.
	inboundCfg.TstAllowSelfConnection = true
	outboundCfg.TstAllowSelfConnection = true

	conn1, conn2 := Pipe(
		&SimpleAddr{net: "tcp", addr: fmt.Sprintf("10.0.0.%d:8333", index)},
		&SimpleAddr{net: "tcp", addr: fmt.Sprintf("10.0.1.%d:8333", index)},
	)

	inboundPeer := peer.NewInboundPeer(&inboundCfg)
	inboundPeer.AssociateConnection(conn1)

	outboundPeer, err := peer.NewOutboundPeer(&outboundCfg,
		conn2.RemoteAddr().String())
	if err != nil {
		err = fmt.Errorf("failed to create new outbound peer: %v", err)
		return nil, nil, err
	}
	outboundPeer.AssociateConnection(conn2)

	// Now that both sides are connected, wait for handshake to complete
	complete := WaitUntil(func() bool {
		return inboundPeer.VerAckReceived() && outboundPeer.VerAckReceived()
	}, time.Second)

	if !complete {
		err = fmt.Errorf("timeout waiting for peer handshake to complete")
		return nil, nil, err
	}

	return inboundPeer, outboundPeer, nil
}

// WaitUntil is a convenience function that can be helpful in tests that run
// async goroutines. It takes a boolean returning function and polls until the
// function returns true and returns true when it does. If the function does not
// return true by a timeout, the WaitUntil returns false.
func WaitUntil(fn func() bool, timeout time.Duration) bool {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeoutChan := time.After(timeout)
	for {
		select {
		case <-ticker.C:
			if fn() {
				return true
			}
		case <-timeoutChan:
			return false
		}
	}
}

// MockPeerNotifier is a mock implemenation of netsync.PeerNotifierInterface.
// Whenever one of the interface methods is called, the MockPeerNotifier puts
// the call arguments in a channel for a receiver to make assertions about.
type MockPeerNotifier struct {
	announceNewTransactionsChan chan *announceNewTransactionsCall
	updatePeerHeightsChan       chan *updatePeerHeightsCall
	relayInventoryChan          chan *relayInventoryCall
	transactionConfirmedChan    chan *transactionConfirmedCall
}

type announceNewTransactionsCall struct {
	newTxs []*mempool.TxDesc
}

type updatePeerHeightsCall struct {
	latestBlkHash *chainhash.Hash
	latestHeight  int32
	updateSource  *peer.Peer
}

type relayInventoryCall struct {
	invVect *wire.InvVect
	data    interface{}
}

type transactionConfirmedCall struct {
	tx *btcutil.Tx
}

func (mock *MockPeerNotifier) AnnounceNewTransactions(newTxs []*mempool.TxDesc) {
	mock.announceNewTransactionsChan <- &announceNewTransactionsCall{
		newTxs: newTxs,
	}
}

func (mock *MockPeerNotifier) UpdatePeerHeights(latestBlkHash *chainhash.Hash, latestHeight int32, updateSource *peer.Peer) {
	mock.updatePeerHeightsChan <- &updatePeerHeightsCall{
		latestBlkHash: latestBlkHash,
		latestHeight:  latestHeight,
		updateSource:  updateSource,
	}
}

func (mock *MockPeerNotifier) RelayInventory(invVect *wire.InvVect, data interface{}) {
	mock.relayInventoryChan <- &relayInventoryCall{
		invVect: invVect,
		data:    data,
	}
}

func (mock *MockPeerNotifier) TransactionConfirmed(tx *btcutil.Tx) {
	mock.transactionConfirmedChan <- &transactionConfirmedCall{tx: tx}
}

// NewMockPeerNotifier creates a new MockPeerNotifier and initializes the
// channels.
func NewMockPeerNotifier() *MockPeerNotifier {
	return &MockPeerNotifier{
		announceNewTransactionsChan: make(chan *announceNewTransactionsCall, 10),
		updatePeerHeightsChan:       make(chan *updatePeerHeightsCall, 10),
		relayInventoryChan:          make(chan *relayInventoryCall, 10),
		transactionConfirmedChan:    make(chan *transactionConfirmedCall, 10),
	}
}

// GenerateAnyoneCanSpendAddress generates a P2SH address with an OP_TRUE in the
// redeem script so it can be trivially spent. Returns the scriptSig as well for
// convenience.
func GenerateAnyoneCanSpendAddress(chainParams *chaincfg.Params) (btcutil.Address, []byte, error) {
	redeemScript := []byte{txscript.OP_TRUE}
	scriptSig, err := txscript.NewScriptBuilder().AddData(redeemScript).Script()
	if err != nil {
		return nil, nil, err
	}
	address, err := btcutil.NewAddressScriptHash(redeemScript, chainParams)
	if err != nil {
		return nil, nil, err
	}
	return address, scriptSig, err
}
