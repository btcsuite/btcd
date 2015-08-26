// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb

import (
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/btcsuite/btclog"
	"github.com/btcsuite/goleveldb/leveldb"
	"github.com/btcsuite/goleveldb/leveldb/opt"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

const (
	dbVersion     int = 2
	dbMaxTransCnt     = 20000
	dbMaxTransMem     = 64 * 1024 * 1024 // 64 MB
)

var log = btclog.Disabled

type tTxInsertData struct {
	txsha   *chainhash.Hash
	blockid int64
	txoff   int
	txlen   int
	usedbuf []byte
}

// LevelDb holds internal state for databse.
type LevelDb struct {
	// lock preventing multiple entry
	dbLock sync.Mutex

	// leveldb pieces
	lDb *leveldb.DB
	ro  *opt.ReadOptions
	wo  *opt.WriteOptions

	lbatch *leveldb.Batch

	nextBlock int64

	lastBlkShaCached bool
	lastBlkSha       chainhash.Hash
	lastBlkIdx       int64

	lastAddrIndexBlkSha chainhash.Hash
	lastAddrIndexBlkIdx int64

	txUpdateMap      map[chainhash.Hash]*txUpdateObj
	txSpentUpdateMap map[chainhash.Hash]*spentTxUpdate
}

var self = database.DriverDB{DbType: "leveldb", CreateDB: CreateDB, OpenDB: OpenDB}

func init() {
	database.AddDBDriver(self)
}

// parseArgs parses the arguments from the database package Open/Create methods.
func parseArgs(funcName string, args ...interface{}) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("Invalid arguments to ldb.%s -- "+
			"expected database path string", funcName)
	}
	dbPath, ok := args[0].(string)
	if !ok {
		return "", fmt.Errorf("First argument to ldb.%s is invalid -- "+
			"expected database path string", funcName)
	}
	return dbPath, nil
}

// CurrentDBVersion is the database version.
var CurrentDBVersion int32 = 1

// OpenDB opens an existing database for use.
func OpenDB(args ...interface{}) (database.Db, error) {
	dbpath, err := parseArgs("OpenDB", args...)
	if err != nil {
		return nil, err
	}

	log = database.GetLog()

	db, err := openDB(dbpath, false)
	if err != nil {
		return nil, err
	}

	// Need to find last block and tx
	var lastknownblock, nextunknownblock, testblock int64

	increment := int64(100000)
	ldb := db.(*LevelDb)

	var lastSha *chainhash.Hash
	// forward scan
blockforward:
	for {

		sha, err := ldb.fetchBlockShaByHeight(testblock)
		if err == nil {
			// block is found
			lastSha = sha
			lastknownblock = testblock
			testblock += increment
		} else {
			if testblock == 0 {
				//no blocks in db, odd but ok.
				lastknownblock = -1
				nextunknownblock = 0
				var emptysha chainhash.Hash
				lastSha = &emptysha
			} else {
				nextunknownblock = testblock
			}
			break blockforward
		}
	}

	// narrow search
blocknarrow:
	for lastknownblock != -1 {
		testblock = (lastknownblock + nextunknownblock) / 2
		sha, err := ldb.fetchBlockShaByHeight(testblock)
		if err == nil {
			lastknownblock = testblock
			lastSha = sha
		} else {
			nextunknownblock = testblock
		}
		if lastknownblock+1 == nextunknownblock {
			break blocknarrow
		}
	}

	// Load the last block whose transactions have been indexed by address.
	if sha, idx, err := ldb.fetchAddrIndexTip(); err == nil {
		if err = ldb.checkAddrIndexVersion(); err == nil {
			ldb.lastAddrIndexBlkSha = *sha
			ldb.lastAddrIndexBlkIdx = idx
			log.Infof("Address index synced and loaded to height %v", idx)
		} else {
			log.Infof("Address index in old, incompatible format, dropping...")
			ldb.deleteOldAddrIndex()
			ldb.PurgeAddrIndex()
			log.Infof("Old, incompatible address index dropped and can now be rebuilt")
		}
	} else {
		ldb.lastAddrIndexBlkIdx = -1
	}

	ldb.lastBlkSha = *lastSha
	ldb.lastBlkIdx = lastknownblock
	ldb.nextBlock = lastknownblock + 1

	return db, nil
}

func openDB(dbpath string, create bool) (pbdb database.Db, err error) {
	var db LevelDb
	var tlDb *leveldb.DB
	var dbversion int32

	defer func() {
		if err == nil {
			db.lDb = tlDb

			db.txUpdateMap = map[chainhash.Hash]*txUpdateObj{}
			db.txSpentUpdateMap = make(map[chainhash.Hash]*spentTxUpdate)

			pbdb = &db
		}
	}()

	if create == true {
		err = os.Mkdir(dbpath, 0750)
		if err != nil {
			log.Errorf("mkdir failed %v %v", dbpath, err)
			return
		}
	} else {
		_, err = os.Stat(dbpath)
		if err != nil {
			err = database.ErrDbDoesNotExist
			return
		}
	}

	needVersionFile := false
	verfile := dbpath + ".ver"
	fi, ferr := os.Open(verfile)
	if ferr == nil {
		defer fi.Close()

		ferr = binary.Read(fi, binary.LittleEndian, &dbversion)
		if ferr != nil {
			dbversion = ^0
		}
	} else {
		if create == true {
			needVersionFile = true
			dbversion = CurrentDBVersion
		}
	}

	opts := &opt.Options{
		BlockCacher:     opt.DefaultBlockCacher,
		Compression:     opt.NoCompression,
		OpenFilesCacher: opt.DefaultOpenFilesCacher,
	}

	switch dbversion {
	case 0:
		opts = &opt.Options{}
	case 1:
		// uses defaults from above
	default:
		err = fmt.Errorf("unsupported db version %v", dbversion)
		return
	}

	tlDb, err = leveldb.OpenFile(dbpath, opts)
	if err != nil {
		return
	}

	// If we opened the database successfully on 'create'
	// update the
	if needVersionFile {
		fo, ferr := os.Create(verfile)
		if ferr != nil {
			// TODO(design) close and delete database?
			err = ferr
			return
		}
		defer fo.Close()
		err = binary.Write(fo, binary.LittleEndian, dbversion)
		if err != nil {
			return
		}
	}

	return
}

// CreateDB creates, initializes and opens a database for use.
func CreateDB(args ...interface{}) (database.Db, error) {
	dbpath, err := parseArgs("Create", args...)
	if err != nil {
		return nil, err
	}

	log = database.GetLog()

	// No special setup needed, just OpenBB
	db, err := openDB(dbpath, true)
	if err == nil {
		ldb := db.(*LevelDb)
		ldb.lastBlkIdx = -1
		ldb.lastAddrIndexBlkIdx = -1
		ldb.nextBlock = 0
	}
	return db, err
}

func (db *LevelDb) close() error {
	return db.lDb.Close()
}

// Sync verifies that the database is coherent on disk,
// and no outstanding transactions are in flight.
func (db *LevelDb) Sync() error {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	// while specified by the API, does nothing
	// however does grab lock to verify it does not return until other operations are complete.
	return nil
}

// Close cleanly shuts down database, syncing all data.
func (db *LevelDb) Close() error {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	return db.close()
}

// DropAfterBlockBySha will remove any blocks from the database after
// the given block.
func (db *LevelDb) DropAfterBlockBySha(sha *chainhash.Hash) (rerr error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()
	defer func() {
		if rerr == nil {
			rerr = db.processBatches()
		} else {
			db.lBatch().Reset()
		}
	}()

	startheight := db.nextBlock - 1

	keepidx, err := db.getBlkLoc(sha)
	if err != nil {
		// should the error here be normalized ?
		log.Tracef("block loc failed %v ", sha)
		return err
	}

	for height := startheight; height > keepidx; height = height - 1 {
		var blk *dcrutil.Block
		blksha, buf, err := db.getBlkByHeight(height)
		if err != nil {
			return err
		}

		blk, err = dcrutil.NewBlockFromBytes(buf)
		if err != nil {
			return err
		}

		// Obtain previous block sha and buffer
		var blkprev *dcrutil.Block
		_, bufprev, errprev := db.getBlkByHeight(height - 1) // discard blkshaprev
		if errprev != nil {
			return errprev
		}

		// Do the same thing for the parent block
		blkprev, errprev = dcrutil.NewBlockFromBytes(bufprev)
		if errprev != nil {
			return errprev
		}

		// Unspend the stake tx in the current block
		for _, tx := range blk.MsgBlock().STransactions {
			err = db.unSpend(tx)
			if err != nil {
				return err
			}
		}
		// rather than iterate the list of tx backward, do it twice.
		for _, tx := range blk.STransactions() {
			var txUo txUpdateObj
			txUo.delete = true
			db.txUpdateMap[*tx.Sha()] = &txUo
		}

		// Check to see if the regular txs of the parent were even included; if
		// they are, unspend all of these regular tx too
		votebits := blk.MsgBlock().Header.VoteBits
		if dcrutil.IsFlagSet16(votebits, dcrutil.BlockValid) && height != 0 {
			// Unspend the regular tx in the current block
			for _, tx := range blkprev.MsgBlock().Transactions {
				err = db.unSpend(tx)
				if err != nil {
					return err
				}
			}
			// rather than iterate the list of tx backward, do it twice.
			for _, tx := range blkprev.Transactions() {
				var txUo txUpdateObj
				txUo.delete = true
				db.txUpdateMap[*tx.Sha()] = &txUo
			}
		}

		db.lBatch().Delete(shaBlkToKey(blksha))
		db.lBatch().Delete(int64ToKey(height))
	}

	// update the last block cache
	db.lastBlkShaCached = true
	db.lastBlkSha = *sha
	db.lastBlkIdx = keepidx
	db.nextBlock = keepidx + 1

	return nil
}

// InsertBlock inserts raw block and transaction data from a block into the
// database.  The first block inserted into the database will be treated as the
// genesis block.  Every subsequent block insert requires the referenced parent
// block to already exist.
func (db *LevelDb) InsertBlock(block *dcrutil.Block) (height int64, rerr error) {
	// Be careful with this function on syncs.  It contains decred changes.

	// Obtain the previous block first so long as it's not the genesis block
	var blockPrev *dcrutil.Block

	// Decred: WARNING. This function assumes that all block insertion calls have
	// dcrutil.blocks passed to them with block.blockHeight set correctly. However,
	// loading the genesis block in btcd didn't do this (via block manager); pre-
	// production it should be established that all calls to this function pass
	// blocks with block.blockHeight set correctly.
	if block.Height() != 0 {
		var errBlockPrev error
		blockPrev, errBlockPrev = db.FetchBlockBySha(&block.MsgBlock().Header.PrevBlock)
		if errBlockPrev != nil {
			blockSha := block.Sha()
			log.Warnf("Failed to fetch parent block of block %v", blockSha)
			return 0, errBlockPrev
		}
	}

	db.dbLock.Lock()
	defer db.dbLock.Unlock()
	defer func() {
		if rerr == nil {
			rerr = db.processBatches()
		} else {
			db.lBatch().Reset()
		}
	}()

	blocksha := block.Sha()
	mblock := block.MsgBlock()
	rawMsg, err := block.Bytes()
	if err != nil {
		log.Warnf("Failed to obtain raw block sha %v", blocksha)
		return 0, err
	}
	_, sTxLoc, err := block.TxLoc()
	if err != nil {
		log.Warnf("Failed to obtain raw block sha %v, stxloc %v", blocksha, sTxLoc)
		return 0, err
	}

	// Insert block into database
	newheight, err := db.insertBlockData(blocksha, &mblock.Header.PrevBlock,
		rawMsg)
	if err != nil {
		log.Warnf("Failed to insert block %v %v %v", blocksha,
			&mblock.Header.PrevBlock, err)
		return 0, err
	}

	// Get data necessary to process regular tx tree of parent block if it's not
	// the genesis block.
	var mBlockPrev *wire.MsgBlock
	var txLoc []wire.TxLoc

	if blockPrev != nil {
		blockShaPrev := blockPrev.Sha()

		mBlockPrev = blockPrev.MsgBlock()

		txLoc, _, err = blockPrev.TxLoc()
		if err != nil {
			log.Warnf("Failed to obtain raw block sha %v, txloc %v", blockShaPrev, txLoc)
			return 0, err
		}
	}

	// Insert the regular tx of the parent block into the tx database if the vote
	// bits enable it, and if it's not the genesis block.
	votebits := mblock.Header.VoteBits
	if dcrutil.IsFlagSet16(votebits, dcrutil.BlockValid) && blockPrev != nil {
		for txidx, tx := range mBlockPrev.Transactions {
			txsha, err := blockPrev.TxSha(txidx)

			if err != nil {
				log.Warnf("failed to compute tx name block %v idx %v err %v", blocksha, txidx, err)
				return 0, err
			}
			spentbuflen := (len(tx.TxOut) + 7) / 8
			spentbuf := make([]byte, spentbuflen, spentbuflen)
			if len(tx.TxOut)%8 != 0 {
				for i := uint(len(tx.TxOut) % 8); i < 8; i++ {
					spentbuf[spentbuflen-1] |= (byte(1) << i)
				}
			}

			// newheight-1 instead of newheight below, as the tx is actually found
			// in the parent.
			//fmt.Printf("insert tx %v into db at height %v\n", txsha, newheight)
			err = db.insertTx(txsha, newheight-1, uint32(txidx), txLoc[txidx].TxStart, txLoc[txidx].TxLen, spentbuf)
			if err != nil {
				log.Warnf("block %v idx %v failed to insert tx %v %v err %v", blocksha, newheight-1, &txsha, txidx, err)
				return 0, err
			}

			err = db.doSpend(tx)
			if err != nil {
				log.Warnf("block %v idx %v failed to spend tx %v %v err %v", blocksha, newheight, txsha, txidx, err)
				return 0, err
			}
		}
	}

	// Insert the stake tx of the current block into the tx database.
	if len(mblock.STransactions) != 0 {
		for txidx, tx := range mblock.STransactions {
			txsha, err := block.STxSha(txidx)

			if err != nil {
				log.Warnf("failed to compute stake tx name block %v idx %v err %v", blocksha, txidx, err)
				return 0, err
			}
			spentbuflen := (len(tx.TxOut) + 7) / 8
			spentbuf := make([]byte, spentbuflen, spentbuflen)
			if len(tx.TxOut)%8 != 0 {
				for i := uint(len(tx.TxOut) % 8); i < 8; i++ {
					spentbuf[spentbuflen-1] |= (byte(1) << i)
				}
			}

			err = db.insertTx(txsha, newheight, uint32(txidx), sTxLoc[txidx].TxStart, sTxLoc[txidx].TxLen, spentbuf)
			if err != nil {
				log.Warnf("block %v idx %v failed to insert stake tx %v %v err %v", blocksha, newheight, &txsha, txidx, err)
				return 0, err
			}

			err = db.doSpend(tx)
			if err != nil {
				log.Warnf("block %v idx %v failed to spend stx %v %v err %v", blocksha, newheight, txsha, txidx, err)
				return 0, err
			}
		}
	}

	return newheight, nil
}

// doSpend iterates all TxIn in a decred transaction marking each associated
// TxOut as spent.
func (db *LevelDb) doSpend(tx *wire.MsgTx) error {
	for txinidx := range tx.TxIn {
		txin := tx.TxIn[txinidx]

		inTxSha := txin.PreviousOutPoint.Hash
		inTxidx := txin.PreviousOutPoint.Index

		if inTxidx == ^uint32(0) {
			continue
		}

		err := db.setSpentData(&inTxSha, inTxidx)
		if err != nil {
			return err
		}
	}
	return nil
}

// unSpend iterates all TxIn in a decred transaction marking each associated
// TxOut as unspent.
func (db *LevelDb) unSpend(tx *wire.MsgTx) error {
	for txinidx := range tx.TxIn {
		txin := tx.TxIn[txinidx]

		inTxSha := txin.PreviousOutPoint.Hash
		inTxidx := txin.PreviousOutPoint.Index

		if inTxidx == ^uint32(0) {
			continue
		}

		err := db.clearSpentData(&inTxSha, inTxidx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (db *LevelDb) setSpentData(sha *chainhash.Hash, idx uint32) error {
	return db.setclearSpentData(sha, idx, true)
}

func (db *LevelDb) clearSpentData(sha *chainhash.Hash, idx uint32) error {
	return db.setclearSpentData(sha, idx, false)
}

func (db *LevelDb) setclearSpentData(txsha *chainhash.Hash, idx uint32, set bool) error {
	var txUo *txUpdateObj
	var ok bool

	if txUo, ok = db.txUpdateMap[*txsha]; !ok {
		// not cached, load from db
		var txU txUpdateObj
		blkHeight, blkIndex, txOff, txLen, spentData, err := db.getTxData(txsha)
		if err != nil {
			// setting a fully spent tx is an error.
			if set == true {
				log.Warnf("setclearSpentData attempted to set fully spent tx "+
					"%v %v %v",
					txsha,
					idx,
					set)

				// if we are clearing a tx and it wasn't found
				// in the tx table, it could be in the fully spent
				// (duplicates) table.
				_, err := db.getTxFullySpent(txsha)
				if err != nil {
					log.Warnf("getTxFullySpent couldn't find the tx either: %v",
						err.Error())
				}
				return err
			}
			// if we are clearing a tx and it wasn't found
			// in the tx table, it could be in the fully spent
			// (duplicates) table.
			spentTxList, err := db.getTxFullySpent(txsha)
			if err != nil {
				log.Warnf("encountered setclearSpentData error for tx hash %v "+
					"idx %v set %v: getTxFullySpent returned %v",
					txsha,
					idx,
					set,
					err.Error())
				return err
			}

			// need to reslice the list to exclude the most recent.
			sTx := spentTxList[len(spentTxList)-1]
			spentTxList[len(spentTxList)-1] = nil
			if len(spentTxList) == 1 {
				// write entry to delete tx from spent pool
				db.txSpentUpdateMap[*txsha] = &spentTxUpdate{delete: true}
			} else {
				// This code should never be hit - aakselrod
				return fmt.Errorf("fully-spent tx %v does not have 1 record: "+
					"%v", txsha, len(spentTxList))
			}

			// Create 'new' Tx update data.
			blkHeight = sTx.blkHeight
			blkIndex = sTx.blkIndex
			txOff = sTx.txoff
			txLen = sTx.txlen
			spentbuflen := (sTx.numTxO + 7) / 8
			spentData = make([]byte, spentbuflen, spentbuflen)
			for i := range spentData {
				spentData[i] = ^byte(0)
			}
		}

		txU.txSha = txsha
		txU.blkHeight = blkHeight
		txU.blkIndex = blkIndex
		txU.txoff = txOff
		txU.txlen = txLen
		txU.spentData = spentData

		txUo = &txU
	}

	byteidx := idx / 8
	byteoff := idx % 8

	if set {
		txUo.spentData[byteidx] |= (byte(1) << byteoff)
	} else {
		txUo.spentData[byteidx] &= ^(byte(1) << byteoff)
	}

	// check for fully spent Tx
	fullySpent := true
	for _, val := range txUo.spentData {
		if val != ^byte(0) {
			fullySpent = false
			break
		}
	}
	if fullySpent {
		var txSu *spentTxUpdate
		// Look up Tx in fully spent table
		if txSuOld, ok := db.txSpentUpdateMap[*txsha]; ok {
			txSu = txSuOld
		} else {
			var txSuStore spentTxUpdate
			txSu = &txSuStore

			txSuOld, err := db.getTxFullySpent(txsha)
			if err == nil {
				txSu.txl = txSuOld
			}
		}

		// Fill in spentTx
		var sTx spentTx
		sTx.blkHeight = txUo.blkHeight
		sTx.blkIndex = txUo.blkIndex
		sTx.txoff = txUo.txoff
		sTx.txlen = txUo.txlen
		// XXX -- there is no way to comput the real TxOut
		// from the spent array.
		sTx.numTxO = 8 * len(txUo.spentData)

		// append this txdata to fully spent txlist
		txSu.txl = append(txSu.txl, &sTx)

		// mark txsha as deleted in the txUpdateMap
		log.Tracef("***tx %v is fully spent\n", txsha)

		db.txSpentUpdateMap[*txsha] = txSu

		txUo.delete = true
		db.txUpdateMap[*txsha] = txUo
	} else {
		db.txUpdateMap[*txsha] = txUo
	}

	return nil
}

func int64ToKey(keyint int64) []byte {
	key := strconv.FormatInt(keyint, 10)
	return []byte(key)
}

func shaBlkToKey(sha *chainhash.Hash) []byte {
	return sha[:]
}

// These are used here and in tx.go's deleteOldAddrIndex() to prevent deletion
// of indexes other than the addrindex now.
var recordSuffixTx = []byte{'t', 'x'}
var recordSuffixSpentTx = []byte{'s', 'x'}

func shaTxToKey(sha *chainhash.Hash) []byte {
	key := make([]byte, len(sha)+len(recordSuffixTx))
	copy(key, sha[:])
	copy(key[len(sha):], recordSuffixTx)
	return key
}

func shaSpentTxToKey(sha *chainhash.Hash) []byte {
	key := make([]byte, len(sha)+len(recordSuffixSpentTx))
	copy(key, sha[:])
	copy(key[len(sha):], recordSuffixSpentTx)
	return key
}

func (db *LevelDb) lBatch() *leveldb.Batch {
	if db.lbatch == nil {
		db.lbatch = new(leveldb.Batch)
	}
	return db.lbatch
}

func (db *LevelDb) processBatches() error {
	var err error

	if len(db.txUpdateMap) != 0 || len(db.txSpentUpdateMap) != 0 || db.lbatch != nil {
		if db.lbatch == nil {
			db.lbatch = new(leveldb.Batch)
		}

		defer db.lbatch.Reset()

		for txSha, txU := range db.txUpdateMap {
			key := shaTxToKey(&txSha)
			if txU.delete {
				log.Tracef("deleting tx %v", txSha)
				db.lbatch.Delete(key)
			} else {
				log.Tracef("inserting tx %v", txSha)
				txdat := db.formatTx(txU)
				db.lbatch.Put(key, txdat)
			}
		}
		for txSha, txSu := range db.txSpentUpdateMap {
			key := shaSpentTxToKey(&txSha)
			if txSu.delete {
				log.Tracef("deleting tx %v", txSha)
				db.lbatch.Delete(key)
			} else {
				log.Tracef("inserting tx %v", txSha)
				txdat := db.formatTxFullySpent(txSu.txl)
				db.lbatch.Put(key, txdat)
			}
		}

		err = db.lDb.Write(db.lbatch, db.wo)
		if err != nil {
			log.Tracef("batch failed %v\n", err)
			return err
		}
		db.txUpdateMap = map[chainhash.Hash]*txUpdateObj{}
		db.txSpentUpdateMap = make(map[chainhash.Hash]*spentTxUpdate)
	}

	return nil
}

// RollbackClose this is part of the database.Db interface and should discard
// recent changes to the db and the close the db.  This currently just does
// a clean shutdown.
func (db *LevelDb) RollbackClose() error {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	return db.close()
}
