// Copyright (c) 2013-2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/heap"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/golangcrypto/ripemd160"
)

type indexState int

const (
	// Our two operating modes.

	// We go into "CatchUp" mode when, on boot, the current best
	// chain height is greater than the last block we've indexed.
	// "CatchUp" mode is characterized by several concurrent worker
	// goroutines indexing blocks organized by a manager goroutine.
	// When in "CatchUp" mode, incoming requests to index newly solved
	// blocks are backed up for later processing. Once we've finished
	// catching up, we process these queued jobs, and then enter into
	// "maintainence" mode.
	indexCatchUp indexState = iota
	// When in "maintainence" mode, we have a single worker serially
	// processing incoming jobs to index newly solved blocks.
	indexMaintain
)

// Limit the number of goroutines that concurrently
// build the index to catch up based on the number
// of processor cores.  This help ensure the system
// stays reasonably responsive under heavy load.
var numCatchUpWorkers = runtime.NumCPU() * 3

// indexBlockMsg packages a request to have the addresses of a block indexed.
type indexBlockMsg struct {
	blk  *btcutil.Block
	done chan struct{}
}

// writeIndexReq represents a request to have a completed address index
// committed to the database.
type writeIndexReq struct {
	blk       *btcutil.Block
	addrIndex database.BlockAddrIndex
}

// addrIndexer provides a concurrent service for indexing the transactions of
// target blocks based on the addresses involved in the transaction.
type addrIndexer struct {
	server          *server
	started         int32
	shutdown        int32
	state           indexState
	quit            chan struct{}
	wg              sync.WaitGroup
	addrIndexJobs   chan *indexBlockMsg
	writeRequests   chan *writeIndexReq
	progressLogger  *blockProgressLogger
	currentIndexTip int64
	chainTip        int64
	sync.Mutex
}

// newAddrIndexer creates a new block address indexer.
// Use Start to begin processing incoming index jobs.
func newAddrIndexer(s *server) (*addrIndexer, error) {
	_, chainHeight, err := s.db.NewestSha()
	if err != nil {
		return nil, err
	}

	_, lastIndexedHeight, err := s.db.FetchAddrIndexTip()
	if err != nil && err != database.ErrAddrIndexDoesNotExist {
		return nil, err
	}

	var state indexState
	if chainHeight == lastIndexedHeight {
		state = indexMaintain
	} else {
		state = indexCatchUp
	}

	ai := &addrIndexer{
		server:          s,
		quit:            make(chan struct{}),
		state:           state,
		addrIndexJobs:   make(chan *indexBlockMsg),
		writeRequests:   make(chan *writeIndexReq, numCatchUpWorkers),
		currentIndexTip: lastIndexedHeight,
		chainTip:        chainHeight,
		progressLogger: newBlockProgressLogger("Indexed addresses of",
			adxrLog),
	}
	return ai, nil
}

// Start begins processing of incoming indexing jobs.
func (a *addrIndexer) Start() {
	// Already started?
	if atomic.AddInt32(&a.started, 1) != 1 {
		return
	}
	adxrLog.Trace("Starting address indexer")
	a.wg.Add(2)
	go a.indexManager()
	go a.indexWriter()
}

// Stop gracefully shuts down the address indexer by stopping all ongoing
// worker goroutines, waiting for them to finish their current task.
func (a *addrIndexer) Stop() error {
	if atomic.AddInt32(&a.shutdown, 1) != 1 {
		adxrLog.Warnf("Address indexer is already in the process of " +
			"shutting down")
		return nil
	}
	adxrLog.Infof("Address indexer shutting down")
	close(a.quit)
	a.wg.Wait()
	return nil
}

// IsCaughtUp returns a bool representing if the address indexer has
// caught up with the best height on the main chain.
func (a *addrIndexer) IsCaughtUp() bool {
	a.Lock()
	defer a.Unlock()
	return a.state == indexMaintain
}

// indexManager creates, and oversees worker index goroutines.
// indexManager is the main goroutine for the addresses indexer.
// It creates, and oversees worker goroutines to index incoming blocks, with
// the exact behavior depending on the current index state
// (catch up, vs maintain). Completion of catch-up mode is always proceeded by
// a gracefull transition into "maintain" mode.
// NOTE: Must be run as a goroutine.
func (a *addrIndexer) indexManager() {
	if a.state == indexCatchUp {
		adxrLog.Infof("Building up address index from height %v to %v.",
			a.currentIndexTip+1, a.chainTip)
		// Quit semaphores to gracefully shut down our worker tasks.
		runningWorkers := make([]chan struct{}, 0, numCatchUpWorkers)
		shutdownWorkers := func() {
			for _, quit := range runningWorkers {
				close(quit)
			}
		}
		criticalShutdown := func() {
			shutdownWorkers()
			a.server.Stop()
		}

		// Spin up all of our "catch up" worker goroutines, giving them
		// a quit channel and WaitGroup so we can gracefully exit if
		// needed.
		var workerWg sync.WaitGroup
		catchUpChan := make(chan *indexBlockMsg)
		for i := 0; i < numCatchUpWorkers; i++ {
			quit := make(chan struct{})
			runningWorkers = append(runningWorkers, quit)
			workerWg.Add(1)
			go a.indexCatchUpWorker(catchUpChan, &workerWg, quit)
		}

		// Starting from the next block after our current index tip,
		// feed our workers each successive block to index until we've
		// caught up to the current highest block height.
		lastBlockIdxHeight := a.currentIndexTip + 1
		for lastBlockIdxHeight <= a.chainTip {
			targetSha, err := a.server.db.FetchBlockShaByHeight(lastBlockIdxHeight)
			if err != nil {
				adxrLog.Errorf("Unable to look up the sha of the "+
					"next target block (height %v): %v",
					lastBlockIdxHeight, err)
				criticalShutdown()
				goto fin
			}
			targetBlock, err := a.server.db.FetchBlockBySha(targetSha)
			if err != nil {
				// Unable to locate a target block by sha, this
				// is a critical error, we may have an
				// inconsistency in the DB.
				adxrLog.Errorf("Unable to look up the next "+
					"target block (sha %v): %v", targetSha, err)
				criticalShutdown()
				goto fin
			}

			// Send off the next job, ready to exit if a shutdown is
			// signalled.
			indexJob := &indexBlockMsg{blk: targetBlock}
			select {
			case catchUpChan <- indexJob:
				lastBlockIdxHeight++
			case <-a.quit:
				shutdownWorkers()
				goto fin
			}
			_, a.chainTip, err = a.server.db.NewestSha()
			if err != nil {
				adxrLog.Errorf("Unable to get latest block height: %v", err)
				criticalShutdown()
				goto fin
			}
		}

		a.Lock()
		a.state = indexMaintain
		a.Unlock()

		// We've finished catching up. Signal our workers to quit, and
		// wait until they've all finished.
		shutdownWorkers()
		workerWg.Wait()
	}

	adxrLog.Infof("Address indexer has caught up to best height, entering " +
		"maintainence mode")

	// We're all caught up at this point. We now serially process new jobs
	// coming in.
	for {
		select {
		case indexJob := <-a.addrIndexJobs:
			addrIndex, err := a.indexBlockAddrs(indexJob.blk)
			if err != nil {
				adxrLog.Errorf("Unable to index transactions of"+
					" block: %v", err)
				a.server.Stop()
				goto fin
			}
			a.writeRequests <- &writeIndexReq{blk: indexJob.blk,
				addrIndex: addrIndex}
		case <-a.quit:
			goto fin
		}
	}
fin:
	a.wg.Done()
}

// UpdateAddressIndex asynchronously queues a newly solved block to have its
// transactions indexed by address.
func (a *addrIndexer) UpdateAddressIndex(block *btcutil.Block) {
	go func() {
		job := &indexBlockMsg{blk: block}
		a.addrIndexJobs <- job
	}()
}

// pendingIndexWrites writes is a priority queue which is used to ensure the
// address index of the block height N+1 is written when our address tip is at
// height N. This ordering is necessary to maintain index consistency in face
// of our concurrent workers, which may not necessarily finish in the order the
// jobs are handed out.
type pendingWriteQueue []*writeIndexReq

// Len returns the number of items in the priority queue. It is part of the
// heap.Interface implementation.
func (pq pendingWriteQueue) Len() int { return len(pq) }

// Less returns whether the item in the priority queue with index i should sort
// before the item with index j. It is part of the heap.Interface implementation.
func (pq pendingWriteQueue) Less(i, j int) bool {
	return pq[i].blk.Height() < pq[j].blk.Height()
}

// Swap swaps the items at the passed indices in the priority queue. It is
// part of the heap.Interface implementation.
func (pq pendingWriteQueue) Swap(i, j int) { pq[i], pq[j] = pq[j], pq[i] }

// Push pushes the passed item onto the priority queue. It is part of the
// heap.Interface implementation.
func (pq *pendingWriteQueue) Push(x interface{}) {
	*pq = append(*pq, x.(*writeIndexReq))
}

// Pop removes the highest priority item (according to Less) from the priority
// queue and returns it.  It is part of the heap.Interface implementation.
func (pq *pendingWriteQueue) Pop() interface{} {
	n := len(*pq)
	item := (*pq)[n-1]
	(*pq)[n-1] = nil
	*pq = (*pq)[0 : n-1]
	return item
}

// indexWriter commits the populated address indexes created by the
// catch up workers to the database. Since we have concurrent workers, the writer
// ensures indexes are written in ascending order to avoid a possible gap in the
// address index triggered by an unexpected shutdown.
// NOTE: Must be run as a goroutine
func (a *addrIndexer) indexWriter() {
	var pendingWrites pendingWriteQueue
	minHeightWrite := make(chan *writeIndexReq)
	workerQuit := make(chan struct{})
	writeFinished := make(chan struct{}, 1)

	// Spawn a goroutine to feed our writer address indexes such
	// that, if our address tip is at N, the index for block N+1 is always
	// written first. We use a priority queue to enforce this condition
	// while accepting new write requests.
	go func() {
		for {
		top:
			select {
			case incomingWrite := <-a.writeRequests:
				heap.Push(&pendingWrites, incomingWrite)

				// Check if we've found a write request that
				// satisfies our condition. If we have, then
				// chances are we have some backed up requests
				// which wouldn't be written until a previous
				// request showed up. If this is the case we'll
				// quickly flush our heap of now available in
				// order writes. We also accept write requests
				// with a block height *before* the current
				// index tip, in order to re-index new prior
				// blocks added to the main chain during a
				// re-org.
				writeReq := heap.Pop(&pendingWrites).(*writeIndexReq)
				_, addrTip, _ := a.server.db.FetchAddrIndexTip()
				for writeReq.blk.Height() == (addrTip+1) ||
					writeReq.blk.Height() <= addrTip {
					minHeightWrite <- writeReq

					// Wait for write to finish so we get a
					// fresh view of the addrtip.
					<-writeFinished

					// Break to grab a new write request
					if pendingWrites.Len() == 0 {
						break top
					}

					writeReq = heap.Pop(&pendingWrites).(*writeIndexReq)
					_, addrTip, _ = a.server.db.FetchAddrIndexTip()
				}

				// We haven't found the proper write request yet,
				// push back onto our heap and wait for the next
				// request which may be our target write.
				heap.Push(&pendingWrites, writeReq)
			case <-workerQuit:
				return
			}
		}
	}()

out:
	// Our main writer loop. Here we actually commit the populated address
	// indexes to the database.
	for {
		select {
		case nextWrite := <-minHeightWrite:
			sha := nextWrite.blk.Sha()
			height := nextWrite.blk.Height()
			err := a.server.db.UpdateAddrIndexForBlock(sha, height,
				nextWrite.addrIndex)
			if err != nil {
				adxrLog.Errorf("Unable to write index for block, "+
					"sha %v, height %v", sha, height)
				a.server.Stop()
				break out
			}
			writeFinished <- struct{}{}
			a.progressLogger.LogBlockHeight(nextWrite.blk)
		case <-a.quit:
			break out
		}

	}
	close(workerQuit)
	a.wg.Done()
}

// indexCatchUpWorker indexes the transactions of previously validated and
// stored blocks.
// NOTE: Must be run as a goroutine
func (a *addrIndexer) indexCatchUpWorker(workChan chan *indexBlockMsg,
	wg *sync.WaitGroup, quit chan struct{}) {
out:
	for {
		select {
		case indexJob := <-workChan:
			addrIndex, err := a.indexBlockAddrs(indexJob.blk)
			if err != nil {
				adxrLog.Errorf("Unable to index transactions of"+
					" block: %v", err)
				a.server.Stop()
				break out
			}
			a.writeRequests <- &writeIndexReq{blk: indexJob.blk,
				addrIndex: addrIndex}
		case <-quit:
			break out
		}
	}
	wg.Done()
}

// indexScriptPubKey indexes all data pushes greater than 8 bytes within the
// passed SPK. Our "address" index is actually a hash160 index, where in the
// ideal case the data push is either the hash160 of a publicKey (P2PKH) or
// a Script (P2SH).
func indexScriptPubKey(addrIndex database.BlockAddrIndex, scriptPubKey []byte,
	locInBlock *wire.TxLoc) error {
	dataPushes, err := txscript.PushedData(scriptPubKey)
	if err != nil {
		adxrLog.Tracef("Couldn't get pushes: %v", err)
		return err
	}

	for _, data := range dataPushes {
		// Only index pushes greater than 8 bytes.
		if len(data) < 8 {
			continue
		}

		var indexKey [ripemd160.Size]byte
		// A perfect little hash160.
		if len(data) <= 20 {
			copy(indexKey[:], data)
			// Otherwise, could be a payToPubKey or an OP_RETURN, so we'll
			// make a hash160 out of it.
		} else {
			copy(indexKey[:], btcutil.Hash160(data))
		}

		addrIndex[indexKey] = append(addrIndex[indexKey], locInBlock)
	}
	return nil
}

// indexBlockAddrs returns a populated index of the all the transactions in the
// passed block based on the addresses involved in each transaction.
func (a *addrIndexer) indexBlockAddrs(blk *btcutil.Block) (database.BlockAddrIndex, error) {
	addrIndex := make(database.BlockAddrIndex)
	txLocs, err := blk.TxLoc()
	if err != nil {
		return nil, err
	}

	for txIdx, tx := range blk.Transactions() {
		// Tx's offset and length in the block.
		locInBlock := &txLocs[txIdx]

		// Coinbases don't have any inputs.
		if !blockchain.IsCoinBase(tx) {
			// Index the SPK's of each input's previous outpoint
			// transaction.
			for _, txIn := range tx.MsgTx().TxIn {
				// Lookup and fetch the referenced output's tx.
				prevOut := txIn.PreviousOutPoint
				txList, err := a.server.db.FetchTxBySha(&prevOut.Hash)
				if len(txList) == 0 {
					return nil, fmt.Errorf("transaction %v not found",
						prevOut.Hash)
				}
				if err != nil {
					adxrLog.Errorf("Error fetching tx %v: %v",
						prevOut.Hash, err)
					return nil, err
				}
				prevOutTx := txList[len(txList)-1]
				inputOutPoint := prevOutTx.Tx.TxOut[prevOut.Index]

				indexScriptPubKey(addrIndex, inputOutPoint.PkScript, locInBlock)
			}
		}

		for _, txOut := range tx.MsgTx().TxOut {
			indexScriptPubKey(addrIndex, txOut.PkScript, locInBlock)
		}
	}
	return addrIndex, nil
}
