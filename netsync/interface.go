// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// PeerNotifier exposes methods to notify peers of status changes to
// transactions, blocks, etc. Currently server (in the main package) implements
// this interface.
type PeerNotifier interface {
	AnnounceNewTransactions(newTxs []*mempool.TxDesc)

	UpdatePeerHeights(latestBlkHash *chainhash.Hash, latestHeight int32, updateSource *peer.Peer)

	RelayInventory(invVect *wire.InvVect, data interface{})

	TransactionConfirmed(tx *btcutil.Tx)
}

// Config is a configuration struct used to initialize a new SyncManager.
type Config struct {
	PeerNotifier PeerNotifier
	Chain        *blockchain.BlockChain
	TxMemPool    *mempool.TxPool
	ChainParams  *chaincfg.Params

	DisableCheckpoints bool
	MaxPeers           int
}

// SyncManager is the interface used to communicate block related messages with
// peers. The SyncManager is started as by executing Start() in a goroutine.
// Once started, it selects peers to sync from and starts the initial block
// download. Once the chain is in sync, the SyncManager handles incoming block
// and header notifications and relays announcements of new blocks to peers.
type SyncManager interface {
	// NewPeer informs the SyncManager of a newly active peer.
	NewPeer(p *peer.Peer)

	// QueueTx adds the passed transaction message and peer to the block
	// handling queue.
	QueueTx(tx *btcutil.Tx, p *peer.Peer, done chan struct{})

	// QueueBlock adds the passed block message and peer to the block handling
	// queue.
	QueueBlock(block *btcutil.Block, p *peer.Peer, done chan struct{})

	// QueueInv adds the passed inv message and peer to the block handling
	// queue.
	QueueInv(inv *wire.MsgInv, p *peer.Peer)

	// QueueHeaders adds the passed headers message and peer to the block
	// handling queue.
	QueueHeaders(headers *wire.MsgHeaders, p *peer.Peer)

	// DonePeer informs the SyncManager that a peer has disconnected.
	DonePeer(p *peer.Peer)

	// Start begins the core block handler which processes block and inv
	// messages.
	Start()

	// Stop gracefully shuts down the SyncManager by stopping all asynchronous
	// handlers and waiting for them to finish.
	Stop() error

	// SyncPeerID returns the ID of the current sync peer, or 0 if there is
	// none.
	SyncPeerID() int32

	// ProcessBlock makes use of ProcessBlock on an internal instance of a block
	// chain.
	ProcessBlock(block *btcutil.Block, flags blockchain.BehaviorFlags) (bool, error)

	// IsCurrent returns whether or not the SyncManager believes it is synced
	// with the connected peers.
	IsCurrent() bool

	// Pause pauses the SyncManager until the returned channel is closed.
	//
	// Note that while paused, all peer and block processing is halted.  The
	// message sender should avoid pausing the SyncManager for long durations.
	Pause() chan<- struct{}
}
