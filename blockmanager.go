// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/list"
	"github.com/conformal/btcchain"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	chanBufferSize = 50

	// blockDbNamePrefix is the prefix for the block database name.  The
	// database type is appended to this value to form the full block
	// database name.
	blockDbNamePrefix = "blocks"
)

// blockMsg packages a bitcoin block message and the peer it came from together
// so the block handler has access to that information.
type blockMsg struct {
	block *btcutil.Block
	peer  *peer
}

// txMsg packages a bitcoin tx message and the peer it came from together
// so the block handler has access to that information.
type txMsg struct {
	msg  *btcwire.MsgTx
	peer *peer
}

// blockManager provides a concurrency safe block manager for handling all
// incoming blocks.
type blockManager struct {
	server            *server
	started           bool
	shutdown          bool
	blockChain        *btcchain.BlockChain
	blockPeer         map[btcwire.ShaHash]*peer
	blockPeerMutex    sync.Mutex
	receivedLogBlocks int64
	receivedLogTx     int64
	lastBlockLogTime  time.Time
	processingReqs    bool
	syncPeer          *peer
	newBlocks         chan bool
	newCandidates     chan *peer
	donePeers         chan *peer
	blockQueue        chan *blockMsg
	chainNotify       chan *btcchain.Notification
	wg                sync.WaitGroup
	quit              chan bool
}

// startSync will choose the best peer among the available candidate peers to
// download/sync the blockchain from.  When syncing is already running, it
// simply returns.  It also examines the candidates for any which are no longer
// candidates and removes them as needed.
func (b *blockManager) startSync(peers *list.List) {
	// Return now if we're already syncing.
	if b.syncPeer != nil {
		return
	}

	// Find the height of the current known best block.
	_, height, err := b.server.db.NewestSha()
	if err != nil {
		log.Errorf("[BMGR] %v", err)
		return
	}

	var bestPeer *peer
	for e := peers.Front(); e != nil; e = e.Next() {
		p := e.Value.(*peer)

		// Remove sync candidate peers that are no longer candidates due
		// to passing their latest known block.
		if p.lastBlock <= int32(height) {
			peers.Remove(e)
			continue
		}

		// TODO(davec): Use a better algorithm to choose the best peer.
		// For now, just pick the first available candidate.
		bestPeer = p
	}

	// Start syncing from the best peer if one was selected.
	if bestPeer != nil {
		locator, err := b.blockChain.LatestBlockLocator()
		if err != nil {
			log.Errorf("[BMGR] Failed to get block locator for the "+
				"latest block: %v", err)
			return
		}

		log.Infof("[BMGR] Syncing to block height %d from peer %v",
			bestPeer.lastBlock, bestPeer.conn.RemoteAddr())
		bestPeer.pushGetBlocksMsg(locator, &zeroHash)
		b.syncPeer = bestPeer
	}

}

// handleNewCandidateMsg deals with new peers that have signalled they may
// be considered as a sync peer (they have already successfully negotiated).  It
// also start syncing if needed.  It is invoked from the syncHandler goroutine.
func (b *blockManager) handleNewCandidateMsg(peers *list.List, p *peer) {
	// Ignore if in the process of shutting down.
	if b.shutdown {
		return
	}

	// The peer is not a candidate for sync if it's not a full node.
	if p.services&btcwire.SFNodeNetwork != btcwire.SFNodeNetwork {
		return
	}

	// Add the peer as a candidate to sync from.
	peers.PushBack(p)

	// Start syncing by choosing the best candidate if needed.
	b.startSync(peers)
}

// handleDonePeerMsg deals with peers that have signalled they are done.  It
// removes the peer as a candidate for syncing and in the case where it was
// the current sync peer, attempts to select a new best peer to sync from.  It
// is invoked from the syncHandler goroutine.
func (b *blockManager) handleDonePeerMsg(peers *list.List, p *peer) {
	// Remove the peer from the list of candidate peers.
	for e := peers.Front(); e != nil; e = e.Next() {
		if e.Value == p {
			peers.Remove(e)
			break
		}
	}

	// Attempt to find a new peer to sync from if the quitting peer is the
	// sync peer.
	if b.syncPeer != nil && b.syncPeer == p {
		b.syncPeer = nil
		b.startSync(peers)
	}
}

// syncHandler deals with handling downloading (syncing) the block chain from
// other peers as they connect and disconnect.  It must be run as a goroutine.
func (b *blockManager) syncHandler() {
	log.Tracef("[BMGR] Starting sync handler")
	candidatePeers := list.New()
out:
	// Live while we're not shutting down.
	for !b.shutdown {
		select {
		case peer := <-b.newCandidates:
			b.handleNewCandidateMsg(candidatePeers, peer)

		case peer := <-b.donePeers:
			b.handleDonePeerMsg(candidatePeers, peer)

		case <-b.quit:
			break out
		}
	}
	b.wg.Done()
	log.Trace("[BMGR] Sync handler done")
}

// logBlockHeight logs a new block height as an information message to show
// progress to the user.  In order to prevent spam, it limits logging to one
// message every 10 seconds with duration and totals included.
func (b *blockManager) logBlockHeight(numTx, height int64) {
	b.receivedLogBlocks++
	b.receivedLogTx += numTx

	now := time.Now()
	duration := now.Sub(b.lastBlockLogTime)
	if duration < time.Second*10 {
		return
	}

	// Log information about new block height.
	blockStr := "blocks"
	if b.receivedLogBlocks == 1 {
		blockStr = "block"
	}
	txStr := "transactions"
	if b.receivedLogTx == 1 {
		txStr = "transaction"
	}
	log.Infof("[BMGR] Processed %d %s (%d %s) in the last %s - Block "+
		"height %d", b.receivedLogBlocks, blockStr, b.receivedLogTx,
		txStr, duration, height)

	b.receivedLogBlocks = 0
	b.receivedLogTx = 0
	b.lastBlockLogTime = now
}

// handleBlockMsg handles block messages from all peers.
func (b *blockManager) handleBlockMsg(bmsg *blockMsg) {
	// Keep track of which peer the block was sent from so the notification
	// handler can request the parent blocks from the appropriate peer.
	blockSha, _ := bmsg.block.Sha()
	b.blockPeerMutex.Lock()
	b.blockPeer[*blockSha] = bmsg.peer
	b.blockPeerMutex.Unlock()

	// Process the block to include validation, best chain selection, orphan
	// handling, etc.
	err := b.blockChain.ProcessBlock(bmsg.block)
	if err != nil {
		b.blockPeerMutex.Lock()
		delete(b.blockPeer, *blockSha)
		b.blockPeerMutex.Unlock()
		log.Warnf("[BMGR] Failed to process block %v: %v", blockSha, err)
		return
	}

	// Don't keep track of the peer that sent the block any longer if it's
	// not an orphan.
	if !b.blockChain.IsKnownOrphan(blockSha) {
		b.blockPeerMutex.Lock()
		delete(b.blockPeer, *blockSha)
		b.blockPeerMutex.Unlock()
	}

	// Log info about the new block height.
	_, height, err := b.server.db.NewestSha()
	if err != nil {
		log.Warnf("[BMGR] Failed to obtain latest sha - %v", err)
		return
	}
	b.logBlockHeight(int64(len(bmsg.block.MsgBlock().Transactions)), height)

	// Sync the db to disk.
	b.server.db.Sync()
}

// blockHandler is the main handler for the block manager.  It must be run
// as a goroutine.  It processes block and inv messages in a separate goroutine
// from the peer handlers so the block (MsgBlock) and tx (MsgTx) messages are
// handled by a single thread without needing to lock memory data structures.
// This is important because the block manager controls which blocks are needed
// and how the fetching should proceed.
//
// NOTE: Tx messages need to be handled here too.
// (either that or block and tx need to be handled in separate threads)
func (b *blockManager) blockHandler() {
out:
	for !b.shutdown {
		select {
		// Handle new block messages.
		case bmsg := <-b.blockQueue:
			b.handleBlockMsg(bmsg)
			bmsg.peer.blockProcessed <- true

		case <-b.quit:
			break out
		}
	}
	b.wg.Done()
	log.Trace("[BMGR] Block handler done")
}

// handleNotifyMsg handles notifications from btcchain.  Currently it doesn't
// respond to any notifications, but the idea is that it requests missing blocks
// in response to orphan notifications and updates the wallet for blocks
// connected to and disconnected from the main chain.
func (b *blockManager) handleNotifyMsg(notification *btcchain.Notification) {
	switch notification.Type {
	// An orphan block has been accepted by the block chain.
	case btcchain.NTOrphanBlock:
		b.blockPeerMutex.Lock()
		defer b.blockPeerMutex.Unlock()

		orphanHash := notification.Data.(*btcwire.ShaHash)
		if peer, exists := b.blockPeer[*orphanHash]; exists {
			orphanRoot := b.blockChain.GetOrphanRoot(orphanHash)
			locator, err := b.blockChain.LatestBlockLocator()
			if err != nil {
				log.Errorf("[BMGR] Failed to get block locator "+
					"for the latest block: %v", err)
				break
			}
			peer.pushGetBlocksMsg(locator, orphanRoot)
			delete(b.blockPeer, *orphanRoot)
			break
		} else {
			log.Warnf("Notification for orphan %v with no peer",
				orphanHash)
		}

	// A block has been accepted into the block chain.
	case btcchain.NTBlockAccepted:
		block, ok := notification.Data.(*btcutil.Block)
		if !ok {
			log.Warnf("[BMGR] Chain notification type not a block.")
			break
		}

		// It's ok to ignore the error here since the notification is
		// coming from the chain code which has already cached the hash.
		hash, _ := block.Sha()

		// Generate the inventory vector and relay it.
		iv := btcwire.NewInvVect(btcwire.InvVect_Block, hash)
		b.server.RelayInventory(iv)
	}
}

// chainNotificationHandler is the handler for asynchronous notifications from
// btcchain.  It must be run as a goroutine.
func (b *blockManager) chainNotificationHandler() {
out:
	for !b.shutdown {
		select {
		case notification := <-b.chainNotify:
			go b.handleNotifyMsg(notification)

		case <-b.quit:
			break out
		}
	}
	b.wg.Done()
	log.Trace("[BMGR] Chain notification handler done")
}

// QueueBlock adds the passed block message and peer to the block handling queue.
func (b *blockManager) QueueBlock(block *btcutil.Block, p *peer) {
	// Don't accept more blocks if we're shutting down.
	if b.shutdown {
		p.blockProcessed <- false
		return
	}

	bmsg := blockMsg{block: block, peer: p}
	b.blockQueue <- &bmsg
}

// Start begins the core block handler which processes block and inv messages.
func (b *blockManager) Start() {
	// Already started?
	if b.started {
		return
	}

	log.Trace("[BMGR] Starting block manager")
	b.wg.Add(3)
	go b.syncHandler()
	go b.blockHandler()
	go b.chainNotificationHandler()
	b.started = true
}

// Stop gracefully shuts down the block manager by stopping all asynchronous
// handlers and waiting for them to finish.
func (b *blockManager) Stop() error {
	if b.shutdown {
		log.Warnf("[BMGR] Block manager is already in the process of " +
			"shutting down")
		return nil
	}

	log.Infof("[BMGR] Block manager shutting down")
	b.shutdown = true
	close(b.quit)
	b.wg.Wait()
	return nil
}

// newBlockManager returns a new bitcoin block manager.
// Use Start to begin processing asynchronous block and inv updates.
func newBlockManager(s *server) *blockManager {
	chainNotify := make(chan *btcchain.Notification, chanBufferSize)
	bm := blockManager{
		server:           s,
		blockChain:       btcchain.New(s.db, s.btcnet, chainNotify),
		blockPeer:        make(map[btcwire.ShaHash]*peer),
		lastBlockLogTime: time.Now(),
		newBlocks:        make(chan bool, 1),
		newCandidates:    make(chan *peer, cfg.MaxPeers),
		donePeers:        make(chan *peer, cfg.MaxPeers),
		blockQueue:       make(chan *blockMsg, chanBufferSize),
		chainNotify:      chainNotify,
		quit:             make(chan bool),
	}
	bm.blockChain.DisableVerify(cfg.VerifyDisabled)
	return &bm
}

// removeRegressionDB removes the existing regression test database if running
// in regression test mode and it already exists.
func removeRegressionDB(dbPath string) error {
	// Dont do anything if not in regression test mode.
	if !cfg.RegressionTest {
		return nil
	}

	// Remove the old regression test database if it already exists.
	fi, err := os.Stat(dbPath)
	if err == nil {
		log.Infof("[BMGR] Removing regression test database from '%s'", dbPath)
		if fi.IsDir() {
			err := os.RemoveAll(dbPath)
			if err != nil {
				return err
			}
		} else {
			err := os.Remove(dbPath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// loadBlockDB opens the block database and returns a handle to it.
func loadBlockDB() (btcdb.Db, error) {
	// The database name is based on the database type.
	dbName := blockDbNamePrefix + "_" + cfg.DbType
	if cfg.DbType == "sqlite" {
		dbName = dbName + ".db"
	}
	dbPath := filepath.Join(cfg.DataDir, dbName)

	// The regression test is special in that it needs a clean database for
	// each run, so remove it now if it already exists.
	removeRegressionDB(dbPath)

	log.Infof("[BMGR] Loading block database from '%s'", dbPath)
	db, err := btcdb.OpenDB(cfg.DbType, dbPath)
	if err != nil {
		// Return the error if it's not because the database doesn't
		// exist.
		if err != btcdb.DbDoesNotExist {
			return nil, err
		}

		// Create the db if it does not exist.
		err = os.MkdirAll(cfg.DataDir, 0700)
		if err != nil {
			return nil, err
		}
		db, err = btcdb.CreateDB(cfg.DbType, dbPath)
		if err != nil {
			return nil, err
		}
	}

	// Get the latest block height from the database.
	_, height, err := db.NewestSha()
	if err != nil {
		db.Close()
		return nil, err
	}

	// Insert the appropriate genesis block for the bitcoin network being
	// connected to if needed.
	if height == -1 {
		genesis := btcutil.NewBlock(activeNetParams.genesisBlock)
		_, err := db.InsertBlock(genesis)
		if err != nil {
			db.Close()
			return nil, err
		}
		log.Infof("[BMGR] Inserted genesis block %v",
			activeNetParams.genesisHash)
		height = 0
	}

	log.Infof("[BMGR] Block database loaded with block height %d", height)
	return db, nil
}
