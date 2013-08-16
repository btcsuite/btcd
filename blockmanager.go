// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/list"
	"github.com/conformal/btcchain"
	"github.com/conformal/btcdb"
	_ "github.com/conformal/btcdb/sqlite3"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	chanBufferSize = 50
)

// inventoryItem is used to track known and requested inventory items.
type inventoryItem struct {
	invVect *btcwire.InvVect
	peers   []*peer
}

// blockMsg packages a bitcoin block message and the peer it came from together
// so the block handler has access to that information.
type blockMsg struct {
	block *btcutil.Block
	peer  *peer
}

// invMsg packages a bitcoin inv message and the peer it came from together
// so the block handler has access to that information.
type invMsg struct {
	msg  *btcwire.MsgInv
	peer *peer
}

// txMsg packages a bitcoin tx message and the peer it came from together
// so the block handler has access to that information.
type txMsg struct {
	msg  *btcwire.MsgTx
	peer *peer
}

// blockManager provides a concurrency safe block manager for handling all
// incoming block inventory advertisement as well as issuing requests to
// download needed blocks of the block chain from other peers.  It works by
// forcing all incoming block inventory advertisements through a single
// goroutine which then determines whether the block is needed and how the
// requests should be made amongst multiple peers.
type blockManager struct {
	server            *server
	started           bool
	shutdown          bool
	blockChain        *btcchain.BlockChain
	requestQueue      *list.List
	requestMap        map[string]*inventoryItem
	outstandingBlocks int
	receivedLogBlocks int64
	receivedLogTx     int64
	lastBlockLogTime  time.Time
	processingReqs    bool
	newBlocks         chan bool
	blockQueue        chan *blockMsg
	invQueue          chan *invMsg
	chainNotify       chan *btcchain.Notification
	wg                sync.WaitGroup
	quit              chan bool
}

// logBlockHeight logs a new block height as an information message to show
// progress to the user.  In order to prevent spam, it limits logging to one
// message every 10 seconds with duration and totals included.
func (b *blockManager) logBlockHeight(numTx, height int64) {
	b.receivedLogBlocks++
	b.receivedLogTx += numTx

	now := time.Now()
	duration := now.Sub(b.lastBlockLogTime)
	if b.outstandingBlocks != 0 && duration < time.Second*10 {
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

// handleInvMsg handles inventory messages for all peers.  It adds blocks that
// we need along with which peers know about each block to a request queue
// based upon the advertised inventory.  It also attempts to strike a balance
// between the number of in-flight blocks and keeping the request queue full
// by issuing more getblocks (MsgGetBlocks) requests as needed.
func (b *blockManager) handleInvMsg(msg *btcwire.MsgInv, p *peer) {
	// Find the last block in the inventory list.
	invVects := msg.InvList
	var lastHash *btcwire.ShaHash
	for i := len(invVects) - 1; i >= 0; i-- {
		if invVects[i].Type == btcwire.InvVect_Block {
			lastHash = &invVects[i].Hash
			break
		}
	}

	for _, iv := range invVects {
		switch iv.Type {
		case btcwire.InvVect_Block:
			// Ignore this block if we already have it.
			// TODO(davec): Need to check orphans too.
			if b.server.db.ExistsSha(&iv.Hash) {
				log.Tracef("[BMGR] Ignoring known block %v.", &iv.Hash)
				continue
			}

			// Add the peer to the list of peers which can serve the block if
			// it's already queued to be fetched.
			if item, ok := b.requestMap[iv.Hash.String()]; ok {
				item.peers = append(item.peers, p)
				continue
			}

			// Add the item to the end of the request queue.
			item := &inventoryItem{
				invVect: iv,
				peers:   []*peer{p},
			}
			b.requestMap[item.invVect.Hash.String()] = item
			b.requestQueue.PushBack(item)
			b.outstandingBlocks++

		case btcwire.InvVect_Tx:
			// XXX: Handle transactions here.
		}
	}

	// Request more blocks if there aren't enough in-flight blocks.
	if lastHash != nil && b.outstandingBlocks < btcwire.MaxBlocksPerMsg*5 {
		stopHash := btcwire.ShaHash{}
		gbmsg := btcwire.NewMsgGetBlocks(&stopHash)
		gbmsg.AddBlockLocatorHash(lastHash)
		p.QueueMessage(gbmsg)
	}
}

// handleBlockMsg handles block messages from all peers.  It is currently
// very simple.  It doesn't validate the block or handle orphans and side
// chains.  It simply inserts the block into the database after ensuring the
// previous block is already inserted.
func (b *blockManager) handleBlockMsg(block *btcutil.Block) {
	b.outstandingBlocks--
	msg := block.MsgBlock()

	// Process the block to include validation, best chain selection, orphan
	// handling, etc.
	err := b.blockChain.ProcessBlock(block)
	if err != nil {
		blockSha, err2 := block.Sha()
		if err2 != nil {
			log.Errorf("[BMGR] %v", err2)
		}
		log.Warnf("[BMGR] Failed to process block %v: %v", blockSha, err)
		return
	}

	// Log info about the new block height.
	_, height, err := b.server.db.NewestSha()
	if err != nil {
		log.Warnf("[BMGR] Failed to obtain latest sha - %v", err)
		return
	}
	b.logBlockHeight(int64(len(msg.Transactions)), height)

	// Sync the db to disk when there are no more outstanding blocks.
	// NOTE: Periodic syncs happen as new data is requested as well.
	if b.outstandingBlocks <= 0 {
		b.server.db.Sync()
	}
}

// blockHandler is the main handler for the block manager.  It must be run as a
// goroutine.  It processes block and inv messages in a separate goroutine from
// the peer handlers so the block (MsgBlock) and tx (MsgTx) messages are handled
// by a single thread without needing to lock memory data structures.  This is
// important because the block manager controls which blocks are needed and how
// the fetching should proceed.
//
// NOTE: Tx messages need to be handled here too.
// (either that or block and tx need to be handled in separate threads)
func (b *blockManager) blockHandler() {
out:
	for !b.shutdown {
		select {
		// Handle new block messages.
		case bmsg := <-b.blockQueue:
			b.handleBlockMsg(bmsg.block)
			bmsg.peer.blockProcessed <- true

		// Handle new inventory messages.
		case msg := <-b.invQueue:
			b.handleInvMsg(msg.msg, msg.peer)
			// Request the blocks.
			if b.requestQueue.Len() > 0 && !b.processingReqs {
				b.processingReqs = true
				b.newBlocks <- true
			}

		case <-b.newBlocks:
			numRequested := 0
			gdmsg := btcwire.NewMsgGetData()
			var p *peer
			for e := b.requestQueue.Front(); e != nil; e = b.requestQueue.Front() {
				item := e.Value.(*inventoryItem)
				p = item.peers[0]
				gdmsg.AddInvVect(item.invVect)
				delete(b.requestMap, item.invVect.Hash.String())
				b.requestQueue.Remove(e)

				numRequested++
				if numRequested >= btcwire.MaxInvPerMsg {
					break
				}
			}
			b.server.db.Sync()
			if len(gdmsg.InvList) > 0 && p != nil {
				p.QueueMessage(gdmsg)
			}
			b.processingReqs = false

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
// connected and disconnected to the main chain.
func (b *blockManager) handleNotifyMsg(notification *btcchain.Notification) {
	switch notification.Type {
	case btcchain.NTOrphanBlock:
		// TODO(davec): Ask the peer to fill in the missing blocks for the
		// orphan root if it's not nil.
		orphanRoot := notification.Data.(*btcwire.ShaHash)
		_ = orphanRoot

	case btcchain.NTBlockAccepted:
		// TODO(davec): Relay inventory, but don't relay old inventory
		// during initial block download.
	}
}

// chainNotificationHandler is the handler for asynchronous notifications from
// btcchain.  It must be run as a goroutine.
func (b *blockManager) chainNotificationHandler() {
out:
	for !b.shutdown {
		select {
		case notification := <-b.chainNotify:
			b.handleNotifyMsg(notification)

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

// QueueInv adds the passed inventory message and peer to the inventory handling
// queue.
func (b *blockManager) QueueInv(msg *btcwire.MsgInv, p *peer) {
	// Don't accept more inventory if we're shutting down.
	if b.shutdown {
		return
	}

	imsg := invMsg{msg: msg, peer: p}
	b.invQueue <- &imsg
}

// Start begins the core block handler which processes block and inv messages.
func (b *blockManager) Start() {
	// Already started?
	if b.started {
		return
	}

	log.Trace("[BMGR] Starting block manager")
	go b.blockHandler()
	go b.chainNotificationHandler()
	b.wg.Add(2)
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

// AddBlockLocators adds block locators to a getblocks message starting with
// the passed hash back to the genesis block hash.  In order to keep the list
// of locator hashes to a reasonable number of entries, first it adds the
// most recent 10 block hashes (starting with the passed hash), then doubles the
// step each loop iteration to exponentially decrease the number of hashes the
// further away from head and closer to the genesis block it gets.
func (b *blockManager) AddBlockLocators(hash *btcwire.ShaHash, msg *btcwire.MsgGetBlocks) error {
	// XXX(davec): This is fetching the block data too.
	block, err := b.server.db.FetchBlockBySha(hash)
	if err != nil {
		log.Warnf("[BMGR] Lookup of known valid index failed %v", hash)
		return err
	}
	blockIndex := block.Height()

	// We want inventory after the passed hash.
	msg.AddBlockLocatorHash(hash)

	// Generate the block locators according to the algorithm described in
	// in the function comment and make sure to leave room for the already
	// added hash and final genesis hash.
	increment := int64(1)
	for i := 1; i < btcwire.MaxBlockLocatorsPerMsg-2; i++ {
		if i > 10 {
			increment *= 2
		}
		blockIndex -= increment
		if blockIndex <= 1 {
			break
		}

		h, err := b.server.db.FetchBlockShaByHeight(blockIndex)
		if err != nil {
			// This shouldn't happen and it's ok to ignore, so just
			// continue to the next.
			log.Warnf("[BMGR] Lookup of known valid index failed %v",
				blockIndex)
			continue
		}
		msg.AddBlockLocatorHash(h)
	}
	msg.AddBlockLocatorHash(&btcwire.GenesisHash)
	return nil
}

// newBlockManager returns a new bitcoin block manager.
// Use Start to begin processing asynchronous block and inv updates.
func newBlockManager(s *server) *blockManager {
	chainNotify := make(chan *btcchain.Notification, chanBufferSize)
	bm := blockManager{
		server:           s,
		blockChain:       btcchain.New(s.db, s.btcnet, chainNotify),
		requestQueue:     list.New(),
		requestMap:       make(map[string]*inventoryItem),
		lastBlockLogTime: time.Now(),
		newBlocks:        make(chan bool, 1),
		blockQueue:       make(chan *blockMsg, chanBufferSize),
		invQueue:         make(chan *invMsg, chanBufferSize),
		chainNotify:      chainNotify,
		quit:             make(chan bool),
	}
	bm.blockChain.DisableVerify(cfg.VerifyDisabled)
	return &bm
}

// loadBlockDB opens the block database and returns a handle to it.
func loadBlockDB() (btcdb.Db, error) {
	dbPath := filepath.Join(cfg.DbDir, activeNetParams.dbName)
	log.Infof("[BMGR] Loading block database from '%s'", dbPath)
	db, err := btcdb.OpenDB("sqlite", dbPath)
	if err != nil {
		// Return the error if it's not because the database doesn't
		// exist.
		if err != btcdb.DbDoesNotExist {
			return nil, err
		}

		// Create the db if it does not exist.
		err = os.MkdirAll(cfg.DbDir, 0700)
		if err != nil {
			return nil, err
		}
		db, err = btcdb.CreateDB("sqlite", dbPath)
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
