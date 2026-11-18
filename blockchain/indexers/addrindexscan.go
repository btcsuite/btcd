// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/blockchain"
)

const (
	// addrScanProgressInterval is the minimum time between scan progress
	// messages.
	addrScanProgressInterval = 10 * time.Second

	// addrBuildDecodedScanMaxWorkers limits the decoded blocks held by scan
	// workers.  Each decoded block keeps both its serialized bytes and its
	// transaction structures, so the limit bounds memory use on hosts with
	// many CPUs.
	addrBuildDecodedScanMaxWorkers = 8
)

// addrScanProgress periodically logs how many blocks the scan workers have
// scanned.  It is safe for concurrent use.
type addrScanProgress struct {
	mu          sync.Mutex
	lastLogTime time.Time
	lastLogged  uint64
	scanned     uint64
	total       uint64
}

// newAddrScanProgress returns a scan progress logger for a scan of total
// blocks.
func newAddrScanProgress(total uint64) *addrScanProgress {
	return &addrScanProgress{
		lastLogTime: time.Now(),
		total:       total,
	}
}

// blockScanned records a scanned block and logs the blocks scanned since the
// last message once addrScanProgressInterval has passed.
func (p *addrScanProgress) blockScanned() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.scanned++
	now := time.Now()
	elapsed := now.Sub(p.lastLogTime)
	if elapsed < addrScanProgressInterval {
		return
	}

	recent := p.scanned - p.lastLogged
	blockLabel := "blocks"
	if recent == 1 {
		blockLabel = "block"
	}
	log.Infof("Scanned %d %s for address index entries in the last %s "+
		"(%d of %d blocks)", recent, blockLabel,
		elapsed.Truncate(10*time.Millisecond), p.scanned, p.total)
	p.lastLogged = p.scanned
	p.lastLogTime = now
}

// stageAddrBlock stages a record for each entry ConnectBlock adds for the main
// chain block at height.  data is cleared first, so a worker can reuse one map
// for all of its blocks.
func (idx *AddrIndex) stageAddrBlock(chain *blockchain.BlockChain,
	stager *addrStager, height int32, data writeIndexData) error {

	clear(data)
	block, err := chain.BlockByHeight(height)
	if err != nil {
		return err
	}
	txLocs, err := block.TxLoc()
	if err != nil {
		return err
	}
	stxos, err := chain.FetchSpendJournal(block)
	if err != nil {
		return err
	}
	idx.indexBlock(data, block, stxos)

	// Staging identifies blocks by height, which gives the chain order of
	// the records.  The write pass resolves the heights to block IDs
	// through the transaction index.
	for addrKey, txIdxs := range data {
		for _, txIdx := range txIdxs {
			err := stager.add(
				&addrKey, uint32(height), txLocs[txIdx],
			)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// scanAddrHeightRange scans the block heights in [start, end] with a pool of
// workers and stages the derived address index records.  Chain reads are safe
// to do concurrently.  The workers stage blocks in no particular order, and the
// write pass sorts the records.
func (idx *AddrIndex) scanAddrHeightRange(chain *blockchain.BlockChain,
	stager *addrStager, start, end int64, numWorkers int,
	progress *addrScanProgress, interrupt <-chan struct{}) error {

	var (
		workers    sync.WaitGroup
		errOnce    sync.Once
		firstErr   error
		stop       = make(chan struct{})
		nextHeight = start
	)
	// fail records the first error and stops the other workers.
	fail := func(e error) {
		errOnce.Do(func() {
			firstErr = e
			close(stop)
		})
	}

	for i := 0; i < numWorkers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			data := make(writeIndexData)

			for {
				if interruptRequested(interrupt) ||
					interruptRequested(stop) {

					fail(errInterruptRequested)
					return
				}
				// Each height in the range is claimed by
				// exactly one worker.
				height := atomic.AddInt64(&nextHeight, 1) - 1
				if height > end {
					return
				}
				if err := idx.stageAddrBlock(
					chain, stager, int32(height), data,
				); err != nil {

					fail(err)
					return
				}
				progress.blockScanned()
			}
		}()
	}
	workers.Wait()
	return firstErr
}
