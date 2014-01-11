// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/list"
	"fmt"
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

// blockMsg packages a bitcoin block message and the peer it came from together
// so the block handler has access to that information.
type headersMsg struct {
	headers *btcwire.MsgHeaders
	peer    *peer
}

// donePeerMsg signifies a newly disconnected peer to the block handler.
type donePeerMsg struct {
	peer *peer
}

// txMsg packages a bitcoin tx message and the peer it came from together
// so the block handler has access to that information.
type txMsg struct {
	tx   *btcutil.Tx
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
	requestedTxns     map[btcwire.ShaHash]bool
	requestedBlocks   map[btcwire.ShaHash]bool
	receivedLogBlocks int64
	receivedLogTx     int64
	lastBlockLogTime  time.Time
	processingReqs    bool
	syncPeer          *peer
	msgChan           chan interface{}
	wg                sync.WaitGroup
	quit              chan bool

	headerPool       map[btcwire.ShaHash]*headerstr
	headerOrphan     map[btcwire.ShaHash]*headerstr
	fetchingHeaders  bool
	startBlock       *btcwire.ShaHash
	fetchBlock       *btcwire.ShaHash
	lastBlock        *btcwire.ShaHash
	latestCheckpoint *btcchain.Checkpoint
}

type headerstr struct {
	header *btcwire.BlockHeader
	next   *headerstr
	height int
	sha    btcwire.ShaHash
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
		bmgrLog.Errorf("%v", err)
		return
	}

	var bestPeer *peer
	var enext *list.Element
	for e := peers.Front(); e != nil; e = enext {
		enext = e.Next()
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
			bmgrLog.Errorf("Failed to get block locator for the "+
				"latest block: %v", err)
			return
		}

		bmgrLog.Infof("Syncing to block height %d from peer %v",
			bestPeer.lastBlock, bestPeer.addr)

		// if starting from the beginning fetch headers and download
		// blocks based on that, otherwise compute the block download
		// via inv messages.  Regression test mode does not support the
		// headers-first approach so do normal block downloads when in
		// regression test mode.
		if height == 0 && !cfg.RegressionTest && !cfg.DisableCheckpoints {
			bestPeer.PushGetHeadersMsg(locator)
			b.fetchingHeaders = true
		} else {
			bestPeer.PushGetBlocksMsg(locator, &zeroHash)
		}
		b.syncPeer = bestPeer
	} else {
		bmgrLog.Warnf("No sync peer candidates available")
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

	bmgrLog.Infof("New valid peer %s (%s)", p, p.userAgent)

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

	bmgrLog.Infof("Lost peer %s", p)

	// Remove requested transactions from the global map so that they will
	// be fetched from elsewhere next time we get an inv.
	for k := range p.requestedTxns {
		delete(b.requestedTxns, k)
	}

	// Remove requested blocks from the global map so that they will be
	// fetched from elsewhere next time we get an inv.
	// TODO(oga) we could possibly here check which peers have these blocks
	// and request them now to speed things up a little.
	for k := range p.requestedBlocks {
		delete(b.requestedBlocks, k)
	}

	// Attempt to find a new peer to sync from if the quitting peer is the
	// sync peer.
	if b.syncPeer != nil && b.syncPeer == p {
		if b.fetchingHeaders {
			b.fetchingHeaders = false
			b.startBlock = nil
			b.fetchBlock = nil
			b.lastBlock = nil
			b.headerPool = make(map[btcwire.ShaHash]*headerstr)
			b.headerOrphan = make(map[btcwire.ShaHash]*headerstr)
		}
		b.syncPeer = nil
		b.startSync(peers)
	}
}

// logBlockHeight logs a new block height as an information message to show
// progress to the user.  In order to prevent spam, it limits logging to one
// message every 10 seconds with duration and totals included.
func (b *blockManager) logBlockHeight(numTx, height int64, latestHash *btcwire.ShaHash) {
	b.receivedLogBlocks++
	b.receivedLogTx += numTx

	now := time.Now()
	duration := now.Sub(b.lastBlockLogTime)
	if duration < time.Second*10 {
		return
	}

	// Truncated the duration to 10s of milliseconds.
	durationMillis := int64(duration / time.Millisecond)
	tDuration := 10 * time.Millisecond * time.Duration(durationMillis/10)

	// Attempt to get the timestamp of the latest block.
	blockTimeStr := ""
	block, err := b.server.db.FetchBlockBySha(latestHash)
	if err == nil {
		blockTimeStr = fmt.Sprintf(", %s", block.MsgBlock().Header.Timestamp)
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
	bmgrLog.Infof("Processed %d %s in the last %s (%d %s, height %d%s)",
		b.receivedLogBlocks, blockStr, tDuration, b.receivedLogTx,
		txStr, height, blockTimeStr)

	b.receivedLogBlocks = 0
	b.receivedLogTx = 0
	b.lastBlockLogTime = now
}

// handleTxMsg handles transaction messages from all peers.
func (b *blockManager) handleTxMsg(tmsg *txMsg) {
	// Keep track of which peer the tx was sent from.
	txHash := tmsg.tx.Sha()

	// If we didn't ask for this transaction then the peer is misbehaving.
	if _, ok := tmsg.peer.requestedTxns[*txHash]; !ok {
		bmgrLog.Warnf("Got unrequested transaction %v from %s -- "+
			"disconnecting", txHash, tmsg.peer.addr)
		tmsg.peer.Disconnect()
		return
	}

	// Process the transaction to include validation, insertion in the
	// memory pool, orphan handling, etc.
	err := tmsg.peer.server.txMemPool.ProcessTransaction(tmsg.tx)

	// Remove transaction from request maps. Either the mempool/chain
	// already knows about it and as such we shouldn't have any more
	// instances of trying to fetch it, or we failed to insert and thus
	// we'll retry next time we get an inv.
	delete(tmsg.peer.requestedTxns, *txHash)
	delete(b.requestedTxns, *txHash)

	if err != nil {
		// When the error is a rule error, it means the transaction was
		// simply rejected as opposed to something actually going wrong,
		// so log it as such.  Otherwise, something really did go wrong,
		// so log it as an actual error.
		if _, ok := err.(TxRuleError); ok {
			bmgrLog.Debugf("Rejected transaction %v: %v", txHash, err)
		} else {
			bmgrLog.Errorf("Failed to process transaction %v: %v", txHash, err)
		}
		return
	}
}

// current returns true if we believe we are synced with our peers, false if we
// still have blocks to check
func (b *blockManager) current() bool {
	if !b.blockChain.IsCurrent() {
		return false
	}

	// if blockChain thinks we are current and we have no syncPeer it
	// is probably right.
	if b.syncPeer == nil {
		return true
	}

	_, height, err := b.server.db.NewestSha()
	// No matter what chain thinks, if we are below the block we are
	// syncing to we are not current.
	// TODO(oga) we can get chain to return the height of each block when we
	// parse an orphan, which would allow us to update the height of peers
	// from what it was at initial handshake.
	if err != nil || height < int64(b.syncPeer.lastBlock) {
		return false
	}
	return true
}

// handleBlockMsg handles block messages from all peers.
func (b *blockManager) handleBlockMsg(bmsg *blockMsg) {

	defer func() {
		if b.startBlock != nil &&
			len(bmsg.peer.requestedBlocks) < 10 {

			// block queue getting short, ask for more.
			b.fetchHeaderBlocks()
		}

	}()
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
			bmgrLog.Warnf("Got unrequested block %v from %s -- "+
				"disconnecting", blockSha, bmsg.peer.addr)
			bmsg.peer.Disconnect()
			return
		}
	}
	b.blockPeer[*blockSha] = bmsg.peer

	fastAdd := false
	if b.fetchBlock != nil && blockSha.IsEqual(b.fetchBlock) {
		firstblock, ok := b.headerPool[*blockSha]
		if ok {
			if b.latestCheckpoint == nil {
				b.latestCheckpoint =
					b.blockChain.LatestCheckpoint()
			}
			if int64(firstblock.height) <=
				b.latestCheckpoint.Height {

				fastAdd = true
			}
			if firstblock.next != nil {
				b.fetchBlock = &firstblock.next.sha
			}
		}
	}
	// Process the block to include validation, best chain selection, orphan
	// handling, etc.
	err := b.blockChain.ProcessBlock(bmsg.block, fastAdd)

	if fastAdd && blockSha.IsEqual(b.lastBlock) {
		// have processed all blocks, switch to normal handling
		b.fetchingHeaders = false
		b.startBlock = nil
		b.fetchBlock = nil
		b.lastBlock = nil
		b.headerPool = make(map[btcwire.ShaHash]*headerstr)
		b.headerOrphan = make(map[btcwire.ShaHash]*headerstr)
	}

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
			bmgrLog.Infof("Rejected block %v: %v", blockSha, err)
		} else {
			bmgrLog.Errorf("Failed to process block %v: %v", blockSha, err)
		}
		return
	}

	// Don't keep track of the peer that sent the block any longer if it's
	// not an orphan.
	if !b.blockChain.IsKnownOrphan(blockSha) {
		delete(b.blockPeer, *blockSha)
	}

	// Log info about the new block height.
	latestHash, height, err := b.server.db.NewestSha()
	if err != nil {
		bmgrLog.Warnf("Failed to obtain latest sha - %v", err)
		return
	}
	b.logBlockHeight(int64(len(bmsg.block.MsgBlock().Transactions)), height,
		latestHash)

	// Sync the db to disk.
	b.server.db.Sync()
}

// haveInventory returns whether or not the inventory represented by the passed
// inventory vector is known.  This includes checking all of the various places
// inventory can be when it is in different states such as blocks that are part
// of the main chain, on a side chain, in the orphan pool, and transactions that
// are in the memory pool (either the main pool or orphan pool).
func (b *blockManager) haveInventory(invVect *btcwire.InvVect) bool {
	switch invVect.Type {
	case btcwire.InvVect_Block:
		// Ask chain if the block is known to it in any form (main
		// chain, side chain, or orphan).
		return b.blockChain.HaveBlock(&invVect.Hash)

	case btcwire.InvVect_Tx:
		// Ask the transaction memory pool if the transaction is known
		// to it in any form (main pool or orphan).
		if b.server.txMemPool.HaveTransaction(&invVect.Hash) {
			return true
		}

		// Check if the transaction exists from the point of view of the
		// end of the main chain.
		return b.server.db.ExistsTxSha(&invVect.Hash)
	}

	// The requested inventory is is an unsupported type, so just claim
	// it is known to avoid requesting it.
	return true
}

// handleInvMsg handles inv messages from all peers.
// We examine the inventory advertised by the remote peer and act accordingly.
func (b *blockManager) handleInvMsg(imsg *invMsg) {
	// Ignore invs from peers that aren't the sync if we are not current.
	// Helps prevent fetching a mass of orphans.
	if imsg.peer != b.syncPeer && !b.current() {
		return
	}

	// Attempt to find the final block in the inventory list.  There may
	// not be one.
	lastBlock := -1
	invVects := imsg.inv.InvList
	for i := len(invVects) - 1; i >= 0; i-- {
		if invVects[i].Type == btcwire.InvTypeBlock {
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
		if iv.Type != btcwire.InvTypeBlock && iv.Type != btcwire.InvTypeTx {
			continue
		}

		// Add the inventory to the cache of known inventory
		// for the peer.
		imsg.peer.AddKnownInventory(iv)

		if b.fetchingHeaders {
			// if we are fetching headers and already know
			// about a block, do not add process it.
			if _, ok := b.headerPool[iv.Hash]; ok {
				continue
			}
		}

		// Request the inventory if we don't already have it.
		if !b.haveInventory(iv) {
			// Add it to the request queue.
			imsg.peer.requestQueue.PushBack(iv)
			continue
		}

		if iv.Type == btcwire.InvTypeBlock {
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
					bmgrLog.Errorf("PEER: Failed to get block "+
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

		switch iv.Type {
		case btcwire.InvVect_Block:
			// Request the block if there is not already a pending
			// request.
			if _, exists := b.requestedBlocks[iv.Hash]; !exists {
				b.requestedBlocks[iv.Hash] = true
				imsg.peer.requestedBlocks[iv.Hash] = true
				gdmsg.AddInvVect(iv)
				numRequested++
			}

		case btcwire.InvVect_Tx:
			// Request the transaction if there is not already a
			// pending request.
			if _, exists := b.requestedTxns[iv.Hash]; !exists {
				b.requestedTxns[iv.Hash] = true
				imsg.peer.requestedTxns[iv.Hash] = true
				gdmsg.AddInvVect(iv)
				numRequested++
			}
		}

		if numRequested >= btcwire.MaxInvPerMsg {
			break
		}
	}
	if len(gdmsg.InvList) > 0 {
		imsg.peer.QueueMessage(gdmsg, nil)
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

			case *txMsg:
				b.handleTxMsg(msg)
				msg.peer.txProcessed <- true

			case *blockMsg:
				b.handleBlockMsg(msg)
				msg.peer.blockProcessed <- true

			case *invMsg:
				b.handleInvMsg(msg)

			case *headersMsg:
				b.handleHeadersMsg(msg)

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
	bmgrLog.Trace("Block handler done")
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
				bmgrLog.Errorf("Failed to get block locator "+
					"for the latest block: %v", err)
				break
			}
			peer.PushGetBlocksMsg(locator, orphanRoot)
			delete(b.blockPeer, *orphanRoot)
		} else {
			bmgrLog.Warnf("Notification for orphan %v with no peer",
				orphanHash)
		}

	// A block has been accepted into the block chain.  Relay it to other
	// peers.
	case btcchain.NTBlockAccepted:
		// Don't relay if we are not current. Other peers that are
		// current should already know about it.

		if !b.current() {
			return
		}

		block, ok := notification.Data.(*btcutil.Block)
		if !ok {
			bmgrLog.Warnf("Chain accepted notification is not a block.")
			break
		}

		// It's ok to ignore the error here since the notification is
		// coming from the chain code which has already cached the hash.
		hash, _ := block.Sha()

		// Generate the inventory vector and relay it.
		iv := btcwire.NewInvVect(btcwire.InvTypeBlock, hash)
		b.server.RelayInventory(iv)

	// A block has been connected to the main block chain.
	case btcchain.NTBlockConnected:
		block, ok := notification.Data.(*btcutil.Block)
		if !ok {
			bmgrLog.Warnf("Chain connected notification is not a block.")
			break
		}

		// Remove all of the transactions (except the coinbase) in the
		// connected block from the transaction pool.  Also, remove any
		// transactions which are now double spends as a result of these
		// new transactions.  Note that removing a transaction from
		// pool also removes any transactions which depend on it,
		// recursively.
		for _, tx := range block.Transactions()[1:] {
			b.server.txMemPool.RemoveTransaction(tx)
			b.server.txMemPool.RemoveDoubleSpends(tx)
		}

		// Notify frontends
		if r := b.server.rpcServer; r != nil {
			go func() {
				r.NotifyBlockTXs(b.server.db, block)
				r.NotifyBlockConnected(block)
			}()
		}

	// A block has been disconnected from the main block chain.
	case btcchain.NTBlockDisconnected:
		block, ok := notification.Data.(*btcutil.Block)
		if !ok {
			bmgrLog.Warnf("Chain disconnected notification is not a block.")
			break
		}

		// Reinsert all of the transactions (except the coinbase) into
		// the transaction pool.
		for _, tx := range block.Transactions()[1:] {
			err := b.server.txMemPool.MaybeAcceptTransaction(tx, nil)
			if err != nil {
				// Remove the transaction and all transactions
				// that depend on it if it wasn't accepted into
				// the transaction pool.
				b.server.txMemPool.RemoveTransaction(tx)
			}
		}

		// Notify frontends
		if r := b.server.rpcServer; r != nil {
			go r.NotifyBlockDisconnected(block)
		}
	}
}

// NewPeer informs the block manager of a newly active peer.
func (b *blockManager) NewPeer(p *peer) {
	// Ignore if we are shutting down.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		return
	}

	b.msgChan <- &newPeerMsg{peer: p}
}

// QueueTx adds the passed transaction message and peer to the block handling
// queue.
func (b *blockManager) QueueTx(tx *btcutil.Tx, p *peer) {
	// Don't accept more transactions if we're shutting down.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		p.txProcessed <- false
		return
	}

	b.msgChan <- &txMsg{tx: tx, peer: p}
}

// QueueBlock adds the passed block message and peer to the block handling queue.
func (b *blockManager) QueueBlock(block *btcutil.Block, p *peer) {
	// Don't accept more blocks if we're shutting down.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		p.blockProcessed <- false
		return
	}

	b.msgChan <- &blockMsg{block: block, peer: p}
}

// QueueInv adds the passed inv message and peer to the block handling queue.
func (b *blockManager) QueueInv(inv *btcwire.MsgInv, p *peer) {
	// No channel handling here because peers do not need to block on inv
	// messages.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		return
	}

	b.msgChan <- &invMsg{inv: inv, peer: p}
}

// QueueInv adds the passed headers message and peer to the block handling queue.
func (b *blockManager) QueueHeaders(headers *btcwire.MsgHeaders, p *peer) {
	// No channel handling here because peers do not need to block on inv
	// messages.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		return
	}

	b.msgChan <- &headersMsg{headers: headers, peer: p}
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

	bmgrLog.Trace("Starting block manager")
	b.wg.Add(1)
	go b.blockHandler()
}

// Stop gracefully shuts down the block manager by stopping all asynchronous
// handlers and waiting for them to finish.
func (b *blockManager) Stop() error {
	if atomic.AddInt32(&b.shutdown, 1) != 1 {
		bmgrLog.Warnf("Block manager is already in the process of " +
			"shutting down")
		return nil
	}

	bmgrLog.Infof("Block manager shutting down")
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
		requestedTxns:    make(map[btcwire.ShaHash]bool),
		requestedBlocks:  make(map[btcwire.ShaHash]bool),
		lastBlockLogTime: time.Now(),
		msgChan:          make(chan interface{}, cfg.MaxPeers*3),
		headerPool:       make(map[btcwire.ShaHash]*headerstr),
		headerOrphan:     make(map[btcwire.ShaHash]*headerstr),
		quit:             make(chan bool),
	}
	bm.blockChain = btcchain.New(s.db, s.btcnet, bm.handleNotifyMsg)
	bm.blockChain.DisableCheckpoints(cfg.DisableCheckpoints)
	if cfg.DisableCheckpoints {
		bmgrLog.Info("Checkpoints are disabled")
	}

	bmgrLog.Infof("Generating initial block node index.  This may " +
		"take a while...")
	err := bm.blockChain.GenerateInitialIndex()
	if err != nil {
		return nil, err
	}
	bmgrLog.Infof("Block index generation complete")

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
		btcdLog.Infof("Removing regression test database from '%s'", dbPath)
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

// dbPath returns the path to the block database given a database type.
func blockDbPath(dbType string) string {
	// The database name is based on the database type.
	dbName := blockDbNamePrefix + "_" + dbType
	if dbType == "sqlite" {
		dbName = dbName + ".db"
	}
	dbPath := filepath.Join(cfg.DataDir, dbName)
	return dbPath
}

// warnMultipeDBs shows a warning if multiple block database types are detected.
// This is not a situation most users want.  It is handy for development however
// to support multiple side-by-side databases.
func warnMultipeDBs() {
	// This is intentionally not using the known db types which depend
	// on the database types compiled into the binary since we want to
	// detect legacy db types as well.
	dbTypes := []string{"leveldb", "sqlite"}
	duplicateDbPaths := make([]string, 0, len(dbTypes)-1)
	for _, dbType := range dbTypes {
		if dbType == cfg.DbType {
			continue
		}

		// Store db path as a duplicate db if it exists.
		dbPath := blockDbPath(dbType)
		if fileExists(dbPath) {
			duplicateDbPaths = append(duplicateDbPaths, dbPath)
		}
	}

	// Warn if there are extra databases.
	if len(duplicateDbPaths) > 0 {
		selectedDbPath := blockDbPath(cfg.DbType)
		btcdLog.Warnf("WARNING: There are multiple block chain databases "+
			"using different database types.\nYou probably don't "+
			"want to waste disk space by having more than one.\n"+
			"Your current database is located at [%v].\nThe "+
			"additional database is located at %v", selectedDbPath,
			duplicateDbPaths)
	}
}

// loadBlockDB opens the block database and returns a handle to it.
func loadBlockDB() (btcdb.Db, error) {
	warnMultipeDBs()

	// The database name is based on the database type.
	dbPath := blockDbPath(cfg.DbType)

	// The regression test is special in that it needs a clean database for
	// each run, so remove it now if it already exists.
	removeRegressionDB(dbPath)

	btcdLog.Infof("Loading block database from '%s'", dbPath)
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
		btcdLog.Infof("Inserted genesis block %v",
			activeNetParams.genesisHash)
		height = 0
	}

	btcdLog.Infof("Block database loaded with block height %d", height)
	return db, nil
}

// handleHeadersMsg is invoked when a peer receives a headers bitcoin
// message.
func (b *blockManager) handleHeadersMsg(bmsg *headersMsg) {
	msg := bmsg.headers

	nheaders := len(msg.Headers)
	if nheaders == 0 {
		bmgrLog.Infof("Received %v block headers: Fetching blocks",
			len(b.headerPool))
		b.fetchHeaderBlocks()
		return
	}
	var blockhash btcwire.ShaHash

	if b.latestCheckpoint == nil {
		b.latestCheckpoint = b.blockChain.LatestCheckpoint()
	}

	for hdridx := range msg.Headers {
		blockhash, _ = msg.Headers[hdridx].BlockSha()
		var headerst headerstr
		headerst.header = msg.Headers[hdridx]
		headerst.sha = blockhash
		prev, ok := b.headerPool[headerst.header.PrevBlock]
		if ok {
			if prev.next == nil {
				prev.next = &headerst
			} else {
				bmgrLog.Infof("two children of the same block ??? %v %v %v", prev.sha, prev.next.sha, blockhash)
			}
			headerst.height = prev.height + 1
		} else if headerst.header.PrevBlock.IsEqual(activeNetParams.genesisHash) {
			ok = true
			headerst.height = 1
			b.startBlock = &headerst.sha
		}
		if int64(headerst.height) == b.latestCheckpoint.Height {
			if headerst.sha.IsEqual(b.latestCheckpoint.Hash) {
				// we can trust this header first download
				// TODO flag this?
			} else {
				// XXX marker does not match, must throw
				// away headers !?!?!
				// XXX dont trust peer?
			}
		}
		if ok {
			b.headerPool[blockhash] = &headerst
			b.lastBlock = &blockhash
		} else {
			bmgrLog.Infof("found orphan block %v", blockhash)
			b.headerOrphan[headerst.header.PrevBlock] = &headerst
		}
	}

	// Construct the getheaders request and queue it to be sent.
	ghmsg := btcwire.NewMsgGetHeaders()
	err := ghmsg.AddBlockLocatorHash(&blockhash)
	if err != nil {
		bmgrLog.Infof("msgheaders bad addheaders", blockhash)
		return
	}

	b.syncPeer.QueueMessage(ghmsg, nil)
}

// fetchHeaderBlocks is creates and sends a request to the syncPeer for
// the next list of blocks to downloaded.
func (b *blockManager) fetchHeaderBlocks() {
	gdmsg := btcwire.NewMsgGetDataSizeHint(btcwire.MaxInvPerMsg)
	numRequested := 0
	startBlock := b.startBlock
	for {
		if b.startBlock == nil {
			break
		}
		blockhash := b.startBlock
		firstblock, ok := b.headerPool[*blockhash]
		if !ok {
			bmgrLog.Warnf("current fetch block %v missing from headerPool", blockhash)
			break
		}
		var iv btcwire.InvVect
		iv.Hash = *blockhash
		iv.Type = btcwire.InvTypeBlock
		if !b.haveInventory(&iv) {
			b.requestedBlocks[*blockhash] = true
			b.syncPeer.requestedBlocks[*blockhash] = true
			gdmsg.AddInvVect(&iv)
			numRequested++
		}

		if b.fetchBlock == nil {
			b.fetchBlock = b.startBlock
		}
		if firstblock.next == nil {
			b.startBlock = nil
			break
		} else {
			b.startBlock = &firstblock.next.sha
		}

		if numRequested >= btcwire.MaxInvPerMsg {
			break
		}
	}
	if len(gdmsg.InvList) > 0 {
		bmgrLog.Debugf("requesting block %v len %v\n", startBlock, len(gdmsg.InvList))

		b.syncPeer.QueueMessage(gdmsg, nil)
	}
}
