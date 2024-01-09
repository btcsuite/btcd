// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
)

// importCmd defines the configuration options for the insecureimport command.
type importCmd struct {
	InFile   string `short:"i" long:"infile" description:"File containing the block(s)"`
	Progress int    `short:"p" long:"progress" description:"Show a progress message each time this number of seconds have passed -- Use 0 to disable progress announcements"`
}

var (
	// importCfg defines the configuration options for the command.
	importCfg = importCmd{
		InFile:   "bootstrap.dat",
		Progress: 10,
	}

	// zeroHash is a simply a hash with all zeros.  It is defined here to
	// avoid creating it multiple times.
	zeroHash = chainhash.Hash{}
)

// importResults houses the stats and result as an import operation.
type importResults struct {
	blocksProcessed int64
	blocksImported  int64
	err             error
}

// blockImporter houses information about an ongoing import from a block data
// file to the block database.
type blockImporter struct {
	db                database.DB
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
	if blockLen > wire.MaxBlockPayload {
		return nil, fmt.Errorf("block payload of %d bytes is larger "+
			"than the max allowed %d bytes", blockLen,
			wire.MaxBlockPayload)
	}

	serializedBlock := make([]byte, blockLen)
	if _, err := io.ReadFull(bi.r, serializedBlock); err != nil {
		return nil, err
	}

	return serializedBlock, nil
}

// processBlock potentially imports the block into the database.  It first
// deserializes the raw block while checking for errors.  Already known blocks
// are skipped and orphan blocks are considered errors.  Returns whether the
// block was imported along with any potential errors.
//
// NOTE: This is not a safe import as it does not verify chain rules.
func (bi *blockImporter) processBlock(serializedBlock []byte) (bool, error) {
	// Deserialize the block which includes checks for malformed blocks.
	block, err := btcutil.NewBlockFromBytes(serializedBlock)
	if err != nil {
		return false, err
	}

	// update progress statistics
	bi.lastBlockTime = block.MsgBlock().Header.Timestamp
	bi.receivedLogTx += int64(len(block.MsgBlock().Transactions))

	// Skip blocks that already exist.
	var exists bool
	err = bi.db.View(func(tx database.Tx) error {
		exists, err = tx.HasBlock(block.Hash())
		return err
	})
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}

	// Don't bother trying to process orphans.
	prevHash := &block.MsgBlock().Header.PrevBlock
	if !prevHash.IsEqual(&zeroHash) {
		var exists bool
		err := bi.db.View(func(tx database.Tx) error {
			exists, err = tx.HasBlock(prevHash)
			return err
		})
		if err != nil {
			return false, err
		}
		if !exists {
			return false, fmt.Errorf("import file contains block "+
				"%v which does not link to the available "+
				"block chain", prevHash)
		}
	}

	// Put the blocks into the database with no checking of chain rules.
	err = bi.db.Update(func(tx database.Tx) error {
		return tx.StoreBlock(block)
	})
	if err != nil {
		return false, err
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
// prevent spam, it limits logging to one message every importCfg.Progress
// seconds with duration and totals included.
func (bi *blockImporter) logProgress() {
	bi.receivedLogBlocks++

	now := time.Now()
	duration := now.Sub(bi.lastLogTime)
	if duration < time.Second*time.Duration(importCfg.Progress) {
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
func newBlockImporter(db database.DB, r io.ReadSeeker) *blockImporter {
	return &blockImporter{
		db:           db,
		r:            r,
		processQueue: make(chan []byte, 2),
		doneChan:     make(chan bool),
		errChan:      make(chan error),
		quit:         make(chan struct{}),
		lastLogTime:  time.Now(),
	}
}

// Execute is the main entry point for the command.  It's invoked by the parser.
func (cmd *importCmd) Execute(args []string) error {
	// Setup the global config options and ensure they are valid.
	if err := setupGlobalConfig(); err != nil {
		return err
	}

	// Ensure the specified block file exists.
	if !fileExists(cmd.InFile) {
		str := "The specified block file [%v] does not exist"
		return fmt.Errorf(str, cmd.InFile)
	}

	// Load the block database.
	db, err := loadBlockDB()
	if err != nil {
		return err
	}
	defer db.Close()

	// Ensure the database is sync'd and closed on Ctrl+C.
	addInterruptHandler(func() {
		log.Infof("Gracefully shutting down the database...")
		db.Close()
	})

	fi, err := os.Open(importCfg.InFile)
	if err != nil {
		return err
	}
	defer fi.Close()

	// Create a block importer for the database and input file and start it.
	// The results channel returned from start will contain an error if
	// anything went wrong.
	importer := newBlockImporter(db, fi)

	// Perform the import asynchronously and signal the main goroutine when
	// done.  This allows blocks to be processed and read in parallel.  The
	// results channel returned from Import contains the statistics about
	// the import including an error if something went wrong.  This is done
	// in a separate goroutine rather than waiting directly so the main
	// goroutine can be signaled for shutdown by either completion, error,
	// or from the main interrupt handler.  This is necessary since the main
	// goroutine must be kept running long enough for the interrupt handler
	// goroutine to finish.
	go func() {
		log.Info("Starting import")
		resultsChan := importer.Import()
		results := <-resultsChan
		if results.err != nil {
			dbErr, ok := results.err.(database.Error)
			if !ok || ok && dbErr.ErrorCode != database.ErrDbNotOpen {
				shutdownChannel <- results.err
				return
			}
		}

		log.Infof("Processed a total of %d blocks (%d imported, %d "+
			"already known)", results.blocksProcessed,
			results.blocksImported,
			results.blocksProcessed-results.blocksImported)
		shutdownChannel <- nil
	}()

	// Wait for shutdown signal from either a normal completion or from the
	// interrupt handler.
	err = <-shutdownChannel
	return err
}
