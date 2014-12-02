// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/mably/btcchain"
	"github.com/mably/btcdb"
	_ "github.com/mably/btcdb/ldb"
	"github.com/mably/btcutil"
	"github.com/mably/btcwire"
)

var zeroHash = btcwire.ShaHash{}

// importResults houses the stats and result as an import operation.
type importResults struct {
	blocksProcessed int64
	blocksImported  int64
	err             error
}

// blockImporter houses information about an ongoing import from a block data
// file to the block database.
type blockImporter struct {
	db                btcdb.Db
	chain             *btcchain.BlockChain
	medianTime        btcchain.MedianTimeSource
	r                 io.ReadSeeker
	processQueue      chan []byte
	doneChan          chan bool
	errChan           chan error
	quit              chan struct{}
	wg                sync.WaitGroup
	blocksProcessed   int64
	blocksImported    int64
	receivedLogBlocks int64
	receivedLogTx     int64
	lastHeight        int64
	lastBlockTime     time.Time
	lastLogTime       time.Time
}

// readBlock reads the next block from the input file.
func (bi *blockImporter) readBlock() ([]byte, error) {
	// The block file format is:
	//  <network> <block length> <serialized block>
	var net uint32
	err := binary.Read(bi.r, binary.LittleEndian, &net)
	if err != nil {
		if err != io.EOF {
			return nil, err
		}

		// No block and no error means there are no more blocks to read.
		return nil, nil
	}
	if net != uint32(activeNetParams.Net) {
		return nil, fmt.Errorf("network mismatch -- got %x, want %x",
			net, uint32(activeNetParams.Net))
	}

	// Read the block length and ensure it is sane.
	var blockLen uint32
	if err := binary.Read(bi.r, binary.LittleEndian, &blockLen); err != nil {
		return nil, err
	}
	if blockLen > btcwire.MaxBlockPayload {
		return nil, fmt.Errorf("block payload of %d bytes is larger "+
			"than the max allowed %d bytes", blockLen,
			btcwire.MaxBlockPayload)
	}

	serializedBlock := make([]byte, blockLen)
	if _, err := io.ReadFull(bi.r, serializedBlock); err != nil {
		return nil, err
	}

	return serializedBlock, nil
}

// processBlock potentially imports the block into the database.  It first
// deserializes the raw block while checking for errors.  Already known blocks
// are skipped and orphan blocks are considered errors.  Finally, it runs the
// block through the chain rules to ensure it follows all rules and matches
// up to the known checkpoint.  Returns whether the block was imported along
// with any potential errors.
func (bi *blockImporter) processBlock(serializedBlock []byte) (bool, error) {
	// Deserialize the block which includes checks for malformed blocks.
	block, err := btcutil.NewBlockFromBytes(serializedBlock)
	if err != nil {
		return false, err
	}

	blockSha, err := block.Sha()
	if err != nil {
		return false, err
	}

	// update progress statistics
	bi.lastBlockTime = block.MsgBlock().Header.Timestamp
	bi.receivedLogTx += int64(len(block.MsgBlock().Transactions))

	// Skip blocks that already exist.
	exists, err := bi.db.ExistsSha(blockSha)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}

	// Don't bother trying to process orphans.
	prevHash := &block.MsgBlock().Header.PrevBlock
	if !prevHash.IsEqual(&zeroHash) {
		exists, err := bi.db.ExistsSha(prevHash)
		if err != nil {
			return false, err
		}
		if !exists {
			return false, fmt.Errorf("import file contains block "+
				"%v which does not link to the available "+
				"block chain", prevHash)
		}
	}

	// Ensure the blocks follows all of the chain rules and match up to the
	// known checkpoints.
	isOrphan, err := bi.chain.ProcessBlock(block, bi.medianTime,
		btcchain.BFFastAdd)
	if err != nil {
		return false, err
	}
	if isOrphan {
		return false, fmt.Errorf("import file contains an orphan "+
			"block: %v", blockSha)
	}

	return true, nil
}

// readHandler is the main handler for reading blocks from the import file.
// This allows block processing to take place in parallel with block reads.
// It must be run as a goroutine.
func (bi *blockImporter) readHandler() {
out:
	for {
		// Read the next block from the file and if anything goes wrong
		// notify the status handler with the error and bail.
		serializedBlock, err := bi.readBlock()
		if err != nil {
			bi.errChan <- fmt.Errorf("Error reading from input "+
				"file: %v", err.Error())
			break out
		}

		// A nil block with no error means we're done.
		if serializedBlock == nil {
			break out
		}

		// Send the block or quit if we've been signalled to exit by
		// the status handler due to an error elsewhere.
		select {
		case bi.processQueue <- serializedBlock:
		case <-bi.quit:
			break out
		}
	}

	// Close the processing channel to signal no more blocks are coming.
	close(bi.processQueue)
	bi.wg.Done()
}

// logProgress logs block progress as an information message.  In order to
// prevent spam, it limits logging to one message every cfg.Progress seconds
// with duration and totals included.
func (bi *blockImporter) logProgress() {
	bi.receivedLogBlocks++

	now := time.Now()
	duration := now.Sub(bi.lastLogTime)
	if duration < time.Second*time.Duration(cfg.Progress) {
		return
	}

	// Truncate the duration to 10s of milliseconds.
	durationMillis := int64(duration / time.Millisecond)
	tDuration := 10 * time.Millisecond * time.Duration(durationMillis/10)

	// Log information about new block height.
	blockStr := "blocks"
	if bi.receivedLogBlocks == 1 {
		blockStr = "block"
	}
	txStr := "transactions"
	if bi.receivedLogTx == 1 {
		txStr = "transaction"
	}
	log.Infof("Processed %d %s in the last %s (%d %s, height %d, %s)",
		bi.receivedLogBlocks, blockStr, tDuration, bi.receivedLogTx,
		txStr, bi.lastHeight, bi.lastBlockTime)

	bi.receivedLogBlocks = 0
	bi.receivedLogTx = 0
	bi.lastLogTime = now
}

// processHandler is the main handler for processing blocks.  This allows block
// processing to take place in parallel with block reads from the import file.
// It must be run as a goroutine.
func (bi *blockImporter) processHandler() {
out:
	for {
		select {
		case serializedBlock, ok := <-bi.processQueue:
			// We're done when the channel is closed.
			if !ok {
				break out
			}

			bi.blocksProcessed++
			bi.lastHeight++
			imported, err := bi.processBlock(serializedBlock)
			if err != nil {
				bi.errChan <- err
				break out
			}

			if imported {
				bi.blocksImported++
			}

			bi.logProgress()

		case <-bi.quit:
			break out
		}
	}
	bi.wg.Done()
}

// statusHandler waits for updates from the import operation and notifies
// the passed doneChan with the results of the import.  It also causes all
// goroutines to exit if an error is reported from any of them.
func (bi *blockImporter) statusHandler(resultsChan chan *importResults) {
	select {
	// An error from either of the goroutines means we're done so signal
	// caller with the error and signal all goroutines to quit.
	case err := <-bi.errChan:
		resultsChan <- &importResults{
			blocksProcessed: bi.blocksProcessed,
			blocksImported:  bi.blocksImported,
			err:             err,
		}
		close(bi.quit)

	// The import finished normally.
	case <-bi.doneChan:
		resultsChan <- &importResults{
			blocksProcessed: bi.blocksProcessed,
			blocksImported:  bi.blocksImported,
			err:             nil,
		}
	}
}

// Import is the core function which handles importing the blocks from the file
// associated with the block importer to the database.  It returns a channel
// on which the results will be returned when the operation has completed.
func (bi *blockImporter) Import() chan *importResults {
	// Start up the read and process handling goroutines.  This setup allows
	// blocks to be read from disk in parallel while being processed.
	bi.wg.Add(2)
	go bi.readHandler()
	go bi.processHandler()

	// Wait for the import to finish in a separate goroutine and signal
	// the status handler when done.
	go func() {
		bi.wg.Wait()
		bi.doneChan <- true
	}()

	// Start the status handler and return the result channel that it will
	// send the results on when the import is done.
	resultChan := make(chan *importResults)
	go bi.statusHandler(resultChan)
	return resultChan
}

// newBlockImporter returns a new importer for the provided file reader seeker
// and database.
func newBlockImporter(db btcdb.Db, r io.ReadSeeker) *blockImporter {
	return &blockImporter{
		db:           db,
		r:            r,
		processQueue: make(chan []byte, 2),
		doneChan:     make(chan bool),
		errChan:      make(chan error),
		quit:         make(chan struct{}),
		chain:        btcchain.New(db, activeNetParams, nil),
		medianTime:   btcchain.NewMedianTime(),
		lastLogTime:  time.Now(),
	}
}
