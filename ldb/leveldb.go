// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb

import (
	"encoding/binary"
	"fmt"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/conformal/seelog"
	"github.com/conformal/goleveldb/leveldb"
	"github.com/conformal/goleveldb/leveldb/cache"
	"github.com/conformal/goleveldb/leveldb/opt"
	"os"
	"runtime"
	"sync"
)

const (
	dbVersion     int = 2
	dbMaxTransCnt     = 20000
	dbMaxTransMem     = 64 * 1024 * 1024 // 64 MB
)

var log seelog.LoggerInterface = seelog.Disabled

type tTxInsertData struct {
	txsha   *btcwire.ShaHash
	blockid int64
	txoff   int
	txlen   int
	usedbuf []byte
}

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
	lastBlkSha       btcwire.ShaHash
	lastBlkIdx       int64

	txUpdateMap map[btcwire.ShaHash]*txUpdateObj
}

var self = btcdb.DriverDB{DbType: "leveldb", Create: CreateDB, Open: OpenDB}

func init() {
	btcdb.AddDBDriver(self)
}

// OpenDB opens an existing database for use.
func OpenDB(dbpath string) (btcdb.Db, error) {
	log = btcdb.GetLog()

	db, err := openDB(dbpath, 0)
	if err != nil {
		return nil, err
	}

	// Need to find last block and tx

	var lastknownblock, nextunknownblock, testblock int64

	increment := int64(100000)
	ldb := db.(*LevelDb)

	var lastSha *btcwire.ShaHash
	// forward scan
blockforward:
	for {

		sha, _, err := ldb.getBlkByHeight(testblock)
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
				var emptysha btcwire.ShaHash
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
		sha, _, err := ldb.getBlkByHeight(testblock)
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

	ldb.lastBlkSha = *lastSha
	ldb.lastBlkIdx = lastknownblock
	ldb.nextBlock = lastknownblock + 1

	return db, nil
}

var CurrentDBVersion int32 = 1

func openDB(dbpath string, flag opt.OptionsFlag) (pbdb btcdb.Db, err error) {
	var db LevelDb
	var tlDb *leveldb.DB
	var dbversion int32

	defer func() {
		if err == nil {
			db.lDb = tlDb

			db.txUpdateMap = map[btcwire.ShaHash]*txUpdateObj{}

			pbdb = &db
		}
	}()

	if flag&opt.OFCreateIfMissing == opt.OFCreateIfMissing {
		err = os.Mkdir(dbpath, 0750)
		if err != nil {
			log.Errorf("mkdir failed %v %v", dbpath, err)
			return
		}
	} else {
		_, err = os.Stat(dbpath)
		if err != nil {
			err = btcdb.DbDoesNotExist
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
		if flag&opt.OFCreateIfMissing != 0 {
			needVersionFile = true
			dbversion = CurrentDBVersion
		}
	}

	myCache := cache.NewEmptyCache()
	opts := &opt.Options{Flag: flag,
		BlockCache:      myCache,
		MaxOpenFiles:    256,
		CompressionType: opt.NoCompression,
	}

	switch dbversion {
	case 0:
		opts = &opt.Options{Flag: flag}
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
func CreateDB(dbpath string) (btcdb.Db, error) {
	log = btcdb.GetLog()

	// No special setup needed, just OpenBB
	db, err := openDB(dbpath, opt.OFCreateIfMissing)
	if err == nil {
		ldb := db.(*LevelDb)
		ldb.lastBlkIdx = -1
		ldb.nextBlock = 0
	}
	return db, err
}

func (db *LevelDb) close() {
	db.lDb.Close()
}

// Sync verifies that the database is coherent on disk,
// and no outstanding transactions are in flight.
func (db *LevelDb) Sync() {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	// while specified by the API, does nothing
	// however does grab lock to verify it does not return until other operations are complete.
}

// Close cleanly shuts down database, syncing all data.
func (db *LevelDb) Close() {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	db.close()
}

// DropAfterBlockBySha will remove any blocks from the database after
// the given block.
func (db *LevelDb) DropAfterBlockBySha(sha *btcwire.ShaHash) (rerr error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()
	defer func() {
		if rerr == nil {
			rerr = db.processBatches()

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
		var blk *btcutil.Block
		blksha, buf, err := db.getBlkByHeight(height)
		if err != nil {
			return err
		}
		blk, err = btcutil.NewBlockFromBytes(buf)
		if err != nil {
			return err
		}

		for _, tx := range blk.MsgBlock().Transactions {
			err = db.unSpend(tx)
			if err != nil {
				return err
			}
		}
		// rather than iterate the list of tx backward, do it twice.
		for _, tx := range blk.MsgBlock().Transactions {
			txSha, _ := tx.TxSha()
			var txUo txUpdateObj
			txUo.delete = true
			db.txUpdateMap[txSha] = &txUo
		}
		db.lBatch().Delete(shaBlkToKey(blksha))
		db.lBatch().Delete(int64ToKey(height))
	}

	db.nextBlock = keepidx + 1

	return nil
}

// InsertBlock inserts raw block and transaction data from a block into the
// database.  The first block inserted into the database will be treated as the
// genesis block.  Every subsequent block insert requires the referenced parent
// block to already exist.
func (db *LevelDb) InsertBlock(block *btcutil.Block) (height int64, rerr error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()
	defer func() {
		if rerr == nil {
			rerr = db.processBatches()
		}
	}()

	blocksha, err := block.Sha()
	if err != nil {
		log.Warnf("Failed to compute block sha %v", blocksha)
		return 0, err
	}
	mblock := block.MsgBlock()
	rawMsg, err := block.Bytes()
	if err != nil {
		log.Warnf("Failed to obtain raw block sha %v", blocksha)
		return 0, err
	}
	txloc, err := block.TxLoc()
	if err != nil {
		log.Warnf("Failed to obtain raw block sha %v", blocksha)
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

	// At least two blocks in the long past were generated by faulty
	// miners, the sha of the transaction exists in a previous block,
	// detect this condition and 'accept' the block.
	for txidx, tx := range mblock.Transactions {
		var txsha btcwire.ShaHash
		txsha, err = tx.TxSha()
		if err != nil {
			log.Warnf("failed to compute tx name block %v idx %v err %v", blocksha, txidx, err)
			return 0, err
		}
		// Some old blocks contain duplicate transactions
		// Attempt to cleanly bypass this problem
		// http://blockexplorer.com/b/91842
		// http://blockexplorer.com/b/91880
		if newheight == 91842 {
			dupsha, err := btcwire.NewShaHashFromStr("d5d27987d2a3dfc724e359870c6644b40e497bdc0589a033220fe15429d88599")
			if err != nil {
				panic("invalid sha string in source")
			}
			if txsha == *dupsha {
				//log.Tracef("skipping sha %v %v", dupsha, newheight)
				continue
			}
		}
		if newheight == 91880 {
			dupsha, err := btcwire.NewShaHashFromStr("e3bf3d07d4b0375638d5f1db5255fe07ba2c4cb067cd81b84ee974b6585fb468")
			if err != nil {
				panic("invalid sha string in source")
			}
			if txsha == *dupsha {
				//log.Tracef("skipping sha %v %v", dupsha, newheight)
				continue
			}
		}
		spentbuflen := (len(tx.TxOut) + 7) / 8
		spentbuf := make([]byte, spentbuflen, spentbuflen)
		if len(tx.TxOut)%8 != 0 {
			for i := uint(len(tx.TxOut) % 8); i < 8; i++ {
				spentbuf[spentbuflen-1] |= (byte(1) << i)
			}
		}

		err = db.insertTx(&txsha, newheight, txloc[txidx].TxStart, txloc[txidx].TxLen, spentbuf)
		if err != nil {
			log.Warnf("block %v idx %v failed to insert tx %v %v err %v", blocksha, newheight, &txsha, txidx, err)

			return
		}
		err = db.doSpend(tx)
		if err != nil {
			log.Warnf("block %v idx %v failed to spend tx %v %v err %v", blocksha, newheight, &txsha, txidx, err)

			return
		}
	}
	return newheight, nil
}

// SetDBInsertMode provides hints to the database to how the application
// is running this allows the database to work in optimized modes when the
// database may be very busy.
func (db *LevelDb) SetDBInsertMode(newmode btcdb.InsertMode) {

	// special modes are not supported
}

// doSpend iterates all TxIn in a bitcoin transaction marking each associated
// TxOut as spent.
func (db *LevelDb) doSpend(tx *btcwire.MsgTx) error {
	for txinidx := range tx.TxIn {
		txin := tx.TxIn[txinidx]

		inTxSha := txin.PreviousOutpoint.Hash
		inTxidx := txin.PreviousOutpoint.Index

		if inTxidx == ^uint32(0) {
			continue
		}

		//log.Infof("spending %v %v",  &inTxSha, inTxidx)

		err := db.setSpentData(&inTxSha, inTxidx)
		if err != nil {
			return err
		}
	}
	return nil
}

// unSpend iterates all TxIn in a bitcoin transaction marking each associated
// TxOut as unspent.
func (db *LevelDb) unSpend(tx *btcwire.MsgTx) error {
	for txinidx := range tx.TxIn {
		txin := tx.TxIn[txinidx]

		inTxSha := txin.PreviousOutpoint.Hash
		inTxidx := txin.PreviousOutpoint.Index

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

func (db *LevelDb) setSpentData(sha *btcwire.ShaHash, idx uint32) error {
	return db.setclearSpentData(sha, idx, true)
}

func (db *LevelDb) clearSpentData(sha *btcwire.ShaHash, idx uint32) error {
	return db.setclearSpentData(sha, idx, false)
}

func (db *LevelDb) setclearSpentData(txsha *btcwire.ShaHash, idx uint32, set bool) error {
	var txUo *txUpdateObj
	var ok bool

	if txUo, ok = db.txUpdateMap[*txsha]; !ok {
		// not cached, load from db
		var txU txUpdateObj
		blkHeight, txOff, txLen, spentData, err := db.getTxData(txsha)
		if err != nil {
			return err
		}

		txU.txSha = txsha
		txU.blkHeight = blkHeight
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

	db.txUpdateMap[*txsha] = txUo

	return nil
}

func int64ToKey(keyint int64) []byte {
	key := fmt.Sprintf("%d", keyint)
	return []byte(key)
}

func shaBlkToKey(sha *btcwire.ShaHash) []byte {
	shaB := sha.Bytes()
	return shaB
}

func shaTxToKey(sha *btcwire.ShaHash) []byte {
	shaB := sha.Bytes()
	shaB = append(shaB, "tx"...)
	return shaB
}

func (db *LevelDb) lBatch() *leveldb.Batch {
	if db.lbatch == nil {
		db.lbatch = new(leveldb.Batch)
	}
	return db.lbatch
}

func (db *LevelDb) processBatches() error {
	var err error

	if len(db.txUpdateMap) != 0 || db.lbatch != nil {
		if db.lbatch == nil {
			db.lbatch = new(leveldb.Batch)
		}

		for txSha, txU := range db.txUpdateMap {
			key := shaTxToKey(&txSha)
			if txU.delete {
				//log.Infof("deleting tx %v", txSha)
				db.lbatch.Delete(key)
			} else {
				//log.Infof("inserting tx %v", txSha)
				txdat, err := db.formatTx(txU)
				if err != nil {
					return err
				}
				db.lbatch.Put(key, txdat)
			}
		}

		err = db.lDb.Write(db.lbatch, db.wo)
		if err != nil {
			log.Tracef("batch failed %v\n", err)
			return err
		}
		db.lbatch.Reset()
		db.txUpdateMap = map[btcwire.ShaHash]*txUpdateObj{}
		runtime.GC()
	}

	return nil
}

func (db *LevelDb) RollbackClose() {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	db.close()
}
