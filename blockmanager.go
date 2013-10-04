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
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	chanBufferSize = 50

	// blockDbNamePrefix is the prefix for the block database name.  The
	// database type is appended to this value to form the full block
	// database name.
	blockDbNamePrefix = "blocks"
)

// newPeerMsg signifies a newly connected peer to the block handler.
type newPeerMsg struct {
	peer *peer
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
	inv  *btcwire.MsgInv
	peer *peer
}

// donePeerMsg signifies a newly disconnected peer to the block handler.
type donePeerMsg struct {
	peer *peer
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
	started           int32
	shutdown          int32
	blockChain        *btcchain.BlockChain
	blockPeer         map[btcwire.ShaHash]*peer
	requestedBlocks   map[btcwire.ShaHash]bool
	receivedLogBlocks int64
	receivedLogTx     int64
	lastBlockLogTime  time.Time
	processingReqs    bool
	syncPeer          *peer
	msgChan           chan interface{}
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
		// to passing their latest known block.  NOTE: The < is
		// intentional as opposed to <=.  While techcnically the peer
		// doesn't have a later block when it's equal, it will likely
		// have one soon so it is a reasonable choice.  It also allows
		// the case where both are at 0 such as during regression test.
		if p.lastBlock < int32(height) {
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
			bestPeer.lastBlock, bestPeer.addr)
		bestPeer.PushGetBlocksMsg(locator, &zeroHash)
		b.syncPeer = bestPeer
	}
}

// isSyncCandidate returns whether or not the peer is a candidate to consider
// syncing from.
func (b *blockManager) isSyncCandidate(p *peer) bool {
	// Typically a peer is not a candidate for sync if it's not a full node,
	// however regression test is special in that the regression tool is
	// not a full node and still needs to be considered a sync candidate.
	if cfg.RegressionTest {
		// The peer is not a candidate if it's not coming from localhost
		// or the hostname can't be determined for some reason.
		host, _, err := net.SplitHostPort(p.addr)
		if err != nil {
			return false
		}

		if host != "127.0.0.1" && host != "localhost" {
			return false
		}
	} else {
		// The peer is not a candidate for sync if it's not a full node.
		if p.services&btcwire.SFNodeNetwork != btcwire.SFNodeNetwork {
			return false
		}
	}

	// Candidate if all checks passed.
	return true
}

// handleNewPeerMsg deals with new peers that have signalled they may
// be considered as a sync peer (they have already successfully negotiated).  It
// also starts syncing if needed.  It is invoked from the syncHandler goroutine.
func (b *blockManager) handleNewPeerMsg(peers *list.List, p *peer) {
	// Ignore if in the process of shutting down.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		return
	}

	log.Infof("[BMGR] New valid peer %s", p)

	// Ignore the peer if it's not a sync candidate.
	if !b.isSyncCandidate(p) {
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

	log.Infof("[BMGR] Lost peer %s", p)

	// remove requested blocks from the global map so that they will be
	// fetched from elsewhere next time we get an inv.
	// TODO(oga) we could possibly here check which peers have these blocks
	// and request them now to speed things up a little.
	for k := range p.requestedBlocks {
		delete(b.requestedBlocks, k)
	}

	// Attempt to find a new peer to sync from if the quitting peer is the
	// sync peer.
	if b.syncPeer != nil && b.syncPeer == p {
		b.syncPeer = nil
		b.startSync(peers)
	}
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

	// If we didn't ask for this block then the peer is misbehaving.
	if _, ok := bmsg.peer.requestedBlocks[*blockSha]; !ok {
		// The regression test intentionally sends some blocks twice
		// to test duplicate block insertion fails.  Don't disconnect
		// the peer or ignore the block when we're in regression test
		// mode in this case so the chain code is actually fed the
		// duplicate blocks.
		if !cfg.RegressionTest {
			log.Warnf("[BMGR] Got unrequested block %v from %s -- "+
				"disconnecting", blockSha, bmsg.peer.addr)
			bmsg.peer.Disconnect()
			return
		}
	}
	b.blockPeer[*blockSha] = bmsg.peer

	// Process the block to include validation, best chain selection, orphan
	// handling, etc.
	err := b.blockChain.ProcessBlock(bmsg.block)

	// Remove block from request maps. Either chain knows about it and such
	// we shouldn't have any more instances of trying to fetch it, or we
	// failed to insert and thus we'll retry next time we get an inv.
	delete(bmsg.peer.requestedBlocks, *blockSha)
	delete(b.requestedBlocks, *blockSha)

	if err != nil {
		delete(b.blockPeer, *blockSha)

		// When the error is a rule error, it means the block was simply
		// rejected as opposed to something actually going wrong, so log
		// it as such.  Otherwise, something really did go wrong, so log
		// it as an actual error.
		if _, ok := err.(btcchain.RuleError); ok {
			log.Infof("[BMGR] Rejected block %v: %v", blockSha, err)
		} else {
			log.Errorf("[BMGR] Failed to process block %v: %v", blockSha, err)
		}
		return
	}

	// Don't keep track of the peer that sent the block any longer if it's
	// not an orphan.
	if !b.blockChain.IsKnownOrphan(blockSha) {
		delete(b.blockPeer, *blockSha)
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

// handleInvMsg handles inv messages from all peers.
// We examine the inventory advertised by the remote peer and act accordingly.
//
// NOTE: This will need to have tx handling added as well when they are
// supported.
func (b *blockManager) handleInvMsg(imsg *invMsg) {
	// Ignore invs from peers that aren't the sync if we are not current.
	// Helps prevent fetching a mass of orphans.
	if imsg.peer != b.syncPeer && !b.blockChain.IsCurrent() {
		return
	}

	// Attempt to find the final block in the inventory list.  There may
	// not be one.
	lastBlock := -1
	invVects := imsg.inv.InvList
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
	chain := b.blockChain
	for i, iv := range invVects {
		// Ignore unsupported inventory types.
		if iv.Type != btcwire.InvVect_Block && iv.Type != btcwire.InvVect_Tx {
			continue
		}

		// Add the inventory to the cache of known inventory
		// for the peer.
		imsg.peer.addKnownInventory(iv)

		// Request the inventory if we don't already have it.
		if !chain.HaveInventory(iv) {
			// Add it to the request queue.
			imsg.peer.requestQueue.PushBack(iv)
			continue
		}

		if iv.Type == btcwire.InvVect_Block {
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
				imsg.peer.PushGetBlocksMsg(locator, orphanRoot)
				continue
			}

			// We already have the final block advertised by this
			// inventory message, so force a request for more.  This
			// should only happen if we're on a really long side
			// chain.
			if i == lastBlock {
				// Request blocks after this one up to the
				// final one the remote peer knows about (zero
				// stop hash).
				locator := chain.BlockLocatorFromHash(&iv.Hash)
				imsg.peer.PushGetBlocksMsg(locator, &zeroHash)
			}
		}
	}

	// Request as much as possible at once.  Anything that won't fit into
	// the request will be requested on the next inv message.
	numRequested := 0
	gdmsg := btcwire.NewMsgGetData()
	requestQueue := imsg.peer.requestQueue
	for e := requestQueue.Front(); e != nil; e = requestQueue.Front() {
		iv := e.Value.(*btcwire.InvVect)
		imsg.peer.requestQueue.Remove(e)
		// check that no one else has asked for this. if so we don't
		// need to ask.
		if _, exists := b.requestedBlocks[iv.Hash]; !exists {
			b.requestedBlocks[iv.Hash] = true
			imsg.peer.requestedBlocks[iv.Hash] = true
			gdmsg.AddInvVect(iv)
			numRequested++
		}

		if numRequested >= btcwire.MaxInvPerMsg {
			break
		}
	}
	if len(gdmsg.InvList) > 0 {
		imsg.peer.QueueMessage(gdmsg)
	}
}

// blockHandler is the main handler for the block manager.  It must be run
// as a goroutine.  It processes block and inv messages in a separate goroutine
// from the peer handlers so the block (MsgBlock) messages are handled by a
// single thread without needing to lock memory data structures.  This is
// important because the block manager controls which blocks are needed and how
// the fetching should proceed.
func (b *blockManager) blockHandler() {
	candidatePeers := list.New()
out:
	for {
		select {
		case m := <-b.msgChan:
			switch msg := m.(type) {
			case *newPeerMsg:
				b.handleNewPeerMsg(candidatePeers, msg.peer)

			case *blockMsg:
				b.handleBlockMsg(msg)
				msg.peer.blockProcessed <- true

			case *invMsg:
				b.handleInvMsg(msg)

			case *donePeerMsg:
				b.handleDonePeerMsg(candidatePeers, msg.peer)

			default:
				// bitch and whine.
			}

		case <-b.quit:
			break out
		}
	}
	b.wg.Done()
	log.Trace("[BMGR] Block handler done")
}

// handleNotifyMsg handles notifications from btcchain.  It does things such
// as request orphan block parents and relay accepted blocks to connected peers.
func (b *blockManager) handleNotifyMsg(notification *btcchain.Notification) {
	switch notification.Type {
	// An orphan block has been accepted by the block chain.  Request
	// its parents from the peer that sent it.
	case btcchain.NTOrphanBlock:
		orphanHash := notification.Data.(*btcwire.ShaHash)
		if peer, exists := b.blockPeer[*orphanHash]; exists {
			orphanRoot := b.blockChain.GetOrphanRoot(orphanHash)
			locator, err := b.blockChain.LatestBlockLocator()
			if err != nil {
				log.Errorf("[BMGR] Failed to get block locator "+
					"for the latest block: %v", err)
				break
			}
			peer.PushGetBlocksMsg(locator, orphanRoot)
			delete(b.blockPeer, *orphanRoot)
		} else {
			log.Warnf("Notification for orphan %v with no peer",
				orphanHash)
		}

	// A block has been accepted into the block chain.  Relay it to other
	// peers.
	case btcchain.NTBlockAccepted:
		// Don't relay if we are not current. Other peers that are
		// current should already know about it.
		// TODO(davec): This should really be over in RelayInventory
		// in server to stop all relays, but chain is not concurrent
		// safe at this time, so call it here to single thread access.
		if !b.blockChain.IsCurrent() {
			return
		}

		block, ok := notification.Data.(*btcutil.Block)
		if !ok {
			log.Warnf("[BMGR] Chain accepted notification is not a block.")
			break
		}

		// It's ok to ignore the error here since the notification is
		// coming from the chain code which has already cached the hash.
		hash, _ := block.Sha()

		// Generate the inventory vector and relay it.
		iv := btcwire.NewInvVect(btcwire.InvVect_Block, hash)
		b.server.RelayInventory(iv)

	// A block has been connected to the main block chain.
	case btcchain.NTBlockConnected:
		block, ok := notification.Data.(*btcutil.Block)
		if !ok {
			log.Warnf("[BMGR] Chain connected notification is not a block.")
			break
		}

		// Remove all of the transactions (except the coinbase) in the
		// connected block from the transaction pool.
		for _, tx := range block.MsgBlock().Transactions[1:] {
			b.server.txMemPool.removeTransaction(tx)
		}

	// A block has been disconnected from the main block chain.
	case btcchain.NTBlockDisconnected:
		block, ok := notification.Data.(*btcutil.Block)
		if !ok {
			log.Warnf("[BMGR] Chain disconnected notification is not a block.")
			break
		}

		// Reinsert all of the transactions (except the coinbase) into
		// the transaction pool.
		for _, tx := range block.MsgBlock().Transactions[1:] {
			err := b.server.txMemPool.ProcessTransaction(tx)
			if err != nil {
				// Remove the transaction and all transactions
				// that depend on it if it wasn't accepted into
				// the transaction pool.
				b.server.txMemPool.removeTransaction(tx)
			}
		}
	}
}

// NewPeer informs the blockmanager of a newly active peer.
func (b *blockManager) NewPeer(p *peer) {
	// Ignore if we are shutting down.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		return
	}
	b.msgChan <- &newPeerMsg{peer: p}
}

// QueueBlock adds the passed block message and peer to the block handling queue.
func (b *blockManager) QueueBlock(block *btcutil.Block, p *peer) {
	// Don't accept more blocks if we're shutting down.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		p.blockProcessed <- false
		return
	}

	bmsg := blockMsg{block: block, peer: p}
	b.msgChan <- &bmsg
}

// QueueInv adds the passed inv message and peer to the block handling queue.
func (b *blockManager) QueueInv(inv *btcwire.MsgInv, p *peer) {
	// No channel handling here because peers do not need to block on inv
	// messages.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		return
	}

	imsg := invMsg{inv: inv, peer: p}
	b.msgChan <- &imsg
}

// DonePeer informs the blockmanager that a peer has disconnected.
func (b *blockManager) DonePeer(p *peer) {
	// Ignore if we are shutting down.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		return
	}
	b.msgChan <- &donePeerMsg{peer: p}
}

// Start begins the core block handler which processes block and inv messages.
func (b *blockManager) Start() {
	// Already started?
	if atomic.AddInt32(&b.started, 1) != 1 {
		return
	}

	log.Trace("[BMGR] Starting block manager")
	b.wg.Add(1)
	go b.blockHandler()
}

// Stop gracefully shuts down the block manager by stopping all asynchronous
// handlers and waiting for them to finish.
func (b *blockManager) Stop() error {
	if atomic.AddInt32(&b.shutdown, 1) != 1 {
		log.Warnf("[BMGR] Block manager is already in the process of " +
			"shutting down")
		return nil
	}

	log.Infof("[BMGR] Block manager shutting down")
	close(b.quit)
	b.wg.Wait()
	return nil
}

// newBlockManager returns a new bitcoin block manager.
// Use Start to begin processing asynchronous block and inv updates.
func newBlockManager(s *server) (*blockManager, error) {
	bm := blockManager{
		server:           s,
		blockPeer:        make(map[btcwire.ShaHash]*peer),
		requestedBlocks:  make(map[btcwire.ShaHash]bool),
		lastBlockLogTime: time.Now(),
		msgChan:          make(chan interface{}, cfg.MaxPeers*3),
		quit:             make(chan bool),
	}
	bm.blockChain = btcchain.New(s.db, s.btcnet, bm.handleNotifyMsg)

	log.Infof("[BMGR] Generating initial block node index.  This may " +
		"take a while...")
	err := bm.blockChain.GenerateInitialIndex()
	if err != nil {
		return nil, err
	}
	log.Infof("[BMGR] Block index generation complete")

	return &bm, nil
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
