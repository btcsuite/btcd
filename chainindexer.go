// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

/*
import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"

	"github.com/btcsuite/golangcrypto/ripemd160"
)

TODO Replace this with a new addrindexer

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
	// "maintenance" mode.
	indexCatchUp indexState = iota
	// When in "maintenance" mode, we have a single worker serially
	// processing incoming jobs to index newly solved blocks.
	indexMaintain
)

// addrIndexer provides a concurrent service for indexing the transactions of
// target blocks based on the addresses involved in the transaction.
type addrIndexer struct {
	server          *server
	started         int32
	shutdown        int32
	state           indexState
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
		state:           state,
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
	err := a.initialize()
	if err != nil {
		adxrLog.Errorf("Couldn't start address indexer: %v", err.Error())
		return
	}
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
	return nil
}

// IsCaughtUp returns a bool representing if the address indexer has
// caught up with the best height on the main chain.
func (a *addrIndexer) IsCaughtUp() bool {
	a.Lock()
	defer a.Unlock()
	return a.state == indexMaintain
}

// initialize starts the address indexer and fills the database up to the
// top height of the current database.
func (a *addrIndexer) initialize() error {
	if a.state == indexCatchUp {
		adxrLog.Infof("Building up address index from height %v to %v.",
			a.currentIndexTip+1, a.chainTip)

		// Starting from the next block after our current index tip,
		// feed our workers each successive block to index until we've
		// caught up to the current highest block height.
		lastBlockIdxHeight := a.currentIndexTip + 1
		for lastBlockIdxHeight <= a.chainTip {
			// Skip the genesis block.
			if !(lastBlockIdxHeight == 0) {
				targetSha, err := a.server.db.FetchBlockShaByHeight(
					lastBlockIdxHeight)
				if err != nil {
					return fmt.Errorf("Unable to look up the sha of the "+
						"next target block (height %v): %v",
						lastBlockIdxHeight, err)
				}
				targetBlock, err := a.server.db.FetchBlockBySha(targetSha)
				if err != nil {
					// Unable to locate a target block by sha, this
					// is a critical error, we may have an
					// inconsistency in the DB.
					return fmt.Errorf("Unable to look up the next "+
						"target block (sha %v): %v", targetSha, err)
				}
				targetParent, err := a.server.db.FetchBlockBySha(
					&targetBlock.MsgBlock().Header.PrevBlock)
				if err != nil {
					// Unable to locate a target block by sha, this
					// is a critical error, we may have an
					// inconsistency in the DB.
					return fmt.Errorf("Unable to look up the next "+
						"target block parent (sha %v): %v",
						targetBlock.MsgBlock().Header.PrevBlock, err)
				}

				addrIndex, err := a.indexBlockAddrs(targetBlock, targetParent)
				if err != nil {
					return fmt.Errorf("Unable to index transactions of"+
						" block: %v", err)
				}
				err = a.server.db.UpdateAddrIndexForBlock(targetSha,
					lastBlockIdxHeight,
					addrIndex)
				if err != nil {
					return fmt.Errorf("Unable to insert block: %v", err.Error())
				}
			}
			lastBlockIdxHeight++
		}

		a.Lock()
		a.state = indexMaintain
		a.Unlock()
	}

	adxrLog.Debugf("Address indexer has queued up to best height, safe " +
		"to begin maintainence mode")

	return nil
}

// convertToAddrIndex indexes all data pushes greater than 8 bytes within the
// passed SPK and returns a TxAddrIndex with the given data. Our "address"
// index is actually a hash160 index, where in the ideal case the data push
// is either the hash160 of a publicKey (P2PKH) or a Script (P2SH).
func convertToAddrIndex(scrVersion uint16, scr []byte, height int64,
	locInBlock *wire.TxLoc, txType stake.TxType) ([]*database.TxAddrIndex, error) {
	var tais []*database.TxAddrIndex

	if scr == nil || locInBlock == nil {
		return nil, fmt.Errorf("passed nil pointer")
	}

	var indexKey [ripemd160.Size]byte

	// Get the script classes and extract the PKH if applicable.
	// If it's multisig, unknown, etc, just hash the script itself.
	class, addrs, _, err := txscript.ExtractPkScriptAddrs(scrVersion, scr,
		activeNetParams.Params)
	if err != nil {
		return nil, fmt.Errorf("script conversion error: %v", err.Error())
	}
	knownType := false
	for _, addr := range addrs {
		switch {
		case class == txscript.PubKeyTy:
			copy(indexKey[:], addr.Hash160()[:])
		case class == txscript.PubkeyAltTy:
			copy(indexKey[:], addr.Hash160()[:])
		case class == txscript.PubKeyHashTy:
			copy(indexKey[:], addr.ScriptAddress()[:])
		case class == txscript.PubkeyHashAltTy:
			copy(indexKey[:], addr.ScriptAddress()[:])
		case class == txscript.StakeSubmissionTy:
			copy(indexKey[:], addr.ScriptAddress()[:])
		case class == txscript.StakeGenTy:
			copy(indexKey[:], addr.ScriptAddress()[:])
		case class == txscript.StakeRevocationTy:
			copy(indexKey[:], addr.ScriptAddress()[:])
		case class == txscript.StakeSubChangeTy:
			copy(indexKey[:], addr.ScriptAddress()[:])
		case class == txscript.MultiSigTy:
			copy(indexKey[:], addr.ScriptAddress()[:])
		case class == txscript.ScriptHashTy:
			copy(indexKey[:], addr.ScriptAddress()[:])
		}

		tai := &database.TxAddrIndex{
			Hash160:  indexKey,
			Height:   uint32(height),
			TxOffset: uint32(locInBlock.TxStart),
			TxLen:    uint32(locInBlock.TxLen),
		}

		tais = append(tais, tai)
		knownType = true
	}

	// This is a commitment for a future vote or
	// revocation. Extract the address data from
	// it and store it in the addrIndex.
	if txType == stake.TxTypeSStx && class == txscript.NullDataTy {
		addr, err := stake.AddrFromSStxPkScrCommitment(scr,
			activeNetParams.Params)
		if err != nil {
			return nil, fmt.Errorf("ticket commit pkscr conversion error: %v",
				err.Error())
		}

		copy(indexKey[:], addr.ScriptAddress()[:])
		tai := &database.TxAddrIndex{
			Hash160:  indexKey,
			Height:   uint32(height),
			TxOffset: uint32(locInBlock.TxStart),
			TxLen:    uint32(locInBlock.TxLen),
		}

		tais = append(tais, tai)
	} else if !knownType {
		copy(indexKey[:], dcrutil.Hash160(scr))
		tai := &database.TxAddrIndex{
			Hash160:  indexKey,
			Height:   uint32(height),
			TxOffset: uint32(locInBlock.TxStart),
			TxLen:    uint32(locInBlock.TxLen),
		}

		tais = append(tais, tai)
	}

	return tais, nil
}

// lookupTransaction is a special transaction lookup function that searches
// the database, the block, and its parent for a transaction. This is needed
// because indexBlockAddrs is called AFTER a block is added/removed in the
// blockchain in blockManager, necessitating that the blocks internally be
// searched for inputs for any given transaction too. Additionally, it's faster
// to get the tx from the blocks here since they're already
func (a *addrIndexer) lookupTransaction(txHash chainhash.Hash, blk *dcrutil.Block,
	parent *dcrutil.Block) (*wire.MsgTx, error) {
	// Search the previous block and parent first.
	txTreeRegularValid := dcrutil.IsFlagSet16(blk.MsgBlock().Header.VoteBits,
		dcrutil.BlockValid)

	// Search the regular tx tree of this and the last block if the
	// tx tree regular was validated.
	if txTreeRegularValid {
		for _, stx := range parent.STransactions() {
			if stx.Sha().IsEqual(&txHash) {
				return stx.MsgTx(), nil
			}
		}
		for _, tx := range parent.Transactions() {
			if tx.Sha().IsEqual(&txHash) {
				return tx.MsgTx(), nil
			}
		}
		for _, tx := range blk.Transactions() {
			if tx.Sha().IsEqual(&txHash) {
				return tx.MsgTx(), nil
			}
		}
	} else {
		// Just search this block's regular tx tree and the previous
		// block's stake tx tree.
		for _, stx := range parent.STransactions() {
			if stx.Sha().IsEqual(&txHash) {
				return stx.MsgTx(), nil
			}
		}
		for _, tx := range blk.Transactions() {
			if tx.Sha().IsEqual(&txHash) {
				return tx.MsgTx(), nil
			}
		}
	}

	// Lookup and fetch the referenced output's tx in the database.
	txList, err := a.server.db.FetchTxBySha(&txHash)
	if err != nil {
		adxrLog.Errorf("Error fetching tx %v: %v",
			txHash, err)
		return nil, err
	}

	if len(txList) == 0 {
		return nil, fmt.Errorf("transaction %v not found",
			txHash)
	}

	return txList[len(txList)-1].Tx, nil
}

// indexBlockAddrs returns a populated index of the all the transactions in the
// passed block based on the addresses involved in each transaction.
func (a *addrIndexer) indexBlockAddrs(blk *dcrutil.Block,
	parent *dcrutil.Block) (database.BlockAddrIndex, error) {
	var addrIndex database.BlockAddrIndex
	_, stxLocs, err := blk.TxLoc()
	if err != nil {
		return nil, err
	}

	txTreeRegularValid := dcrutil.IsFlagSet16(blk.MsgBlock().Header.VoteBits,
		dcrutil.BlockValid)

	// Add regular transactions iff the block was validated.
	if txTreeRegularValid {
		txLocs, _, err := parent.TxLoc()
		if err != nil {
			return nil, err
		}
		for txIdx, tx := range parent.Transactions() {
			// Tx's offset and length in the block.
			locInBlock := &txLocs[txIdx]

			// Coinbases don't have any inputs.
			if !blockchain.IsCoinBase(tx) {
				// Index the SPK's of each input's previous outpoint
				// transaction.
				for _, txIn := range tx.MsgTx().TxIn {
					prevOutTx, err := a.lookupTransaction(
						txIn.PreviousOutPoint.Hash,
						blk,
						parent)
					inputOutPoint := prevOutTx.TxOut[txIn.PreviousOutPoint.Index]

					toAppend, err := convertToAddrIndex(inputOutPoint.Version,
						inputOutPoint.PkScript, parent.Height(), locInBlock,
						stake.TxTypeRegular)
					if err != nil {
						adxrLog.Tracef("Error converting tx txin %v: %v",
							txIn.PreviousOutPoint.Hash, err)
						continue
					}
					addrIndex = append(addrIndex, toAppend...)
				}
			}

			for _, txOut := range tx.MsgTx().TxOut {
				toAppend, err := convertToAddrIndex(txOut.Version, txOut.PkScript,
					parent.Height(), locInBlock, stake.TxTypeRegular)
				if err != nil {
					adxrLog.Tracef("Error converting tx txout %v: %v",
						tx.MsgTx().TxSha(), err)
					continue
				}
				addrIndex = append(addrIndex, toAppend...)
			}
		}
	}

	// Add stake transactions.
	for stxIdx, stx := range blk.STransactions() {
		// Tx's offset and length in the block.
		locInBlock := &stxLocs[stxIdx]

		txType := stake.DetermineTxType(stx)

		// Index the SPK's of each input's previous outpoint
		// transaction.
		for i, txIn := range stx.MsgTx().TxIn {
			// Stakebases don't have any inputs.
			if txType == stake.TxTypeSSGen && i == 0 {
				continue
			}

			// Lookup and fetch the referenced output's tx.
			prevOutTx, err := a.lookupTransaction(
				txIn.PreviousOutPoint.Hash,
				blk,
				parent)
			inputOutPoint := prevOutTx.TxOut[txIn.PreviousOutPoint.Index]

			toAppend, err := convertToAddrIndex(inputOutPoint.Version,
				inputOutPoint.PkScript, blk.Height(), locInBlock,
				txType)
			if err != nil {
				adxrLog.Tracef("Error converting stx txin %v: %v",
					txIn.PreviousOutPoint.Hash, err)
				continue
			}
			addrIndex = append(addrIndex, toAppend...)
		}

		for _, txOut := range stx.MsgTx().TxOut {
			toAppend, err := convertToAddrIndex(txOut.Version, txOut.PkScript,
				blk.Height(), locInBlock, txType)
			if err != nil {
				adxrLog.Tracef("Error converting stx txout %v: %v",
					stx.MsgTx().TxSha(), err)
				continue
			}
			addrIndex = append(addrIndex, toAppend...)
		}
	}

	return addrIndex, nil
}

// InsertBlock synchronously queues a newly solved block to have its
// transactions indexed by address.
func (a *addrIndexer) InsertBlock(block *dcrutil.Block, parent *dcrutil.Block) error {
	addrIndex, err := a.indexBlockAddrs(block, parent)
	if err != nil {
		return fmt.Errorf("Unable to index transactions of"+
			" block: %v", err)
	}
	err = a.server.db.UpdateAddrIndexForBlock(block.Sha(),
		block.Height(),
		addrIndex)
	if err != nil {
		return fmt.Errorf("Unable to insert block: %v", err.Error())
	}

	return nil
}

// RemoveBlock removes all transactions from a block on the tip from the
// address index database.
func (a *addrIndexer) RemoveBlock(block *dcrutil.Block,
	parent *dcrutil.Block) error {
	addrIndex, err := a.indexBlockAddrs(block, parent)
	if err != nil {
		return fmt.Errorf("Unable to index transactions of"+
			" block: %v", err)
	}
	err = a.server.db.DropAddrIndexForBlock(block.Sha(),
		block.Height(),
		addrIndex)
	if err != nil {
		return fmt.Errorf("Unable to remove block: %v", err.Error())
	}

	return nil
}
*/
