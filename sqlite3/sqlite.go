// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package sqlite3

import (
	"database/sql"
	"fmt"
	"github.com/conformal/btcdb"
	"github.com/conformal/btclog"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"sync"
)

const (
	dbVersion     int = 2
	dbMaxTransCnt     = 20000
	dbMaxTransMem     = 64 * 1024 * 1024 // 64 MB
)

const (
	blkInsertSha = iota
	blkFetchSha
	blkExistsSha
	blkFetchIdx
	blkFetchIdxList
)

const (
	txInsertStmt = iota
	txFetchUsedByShaStmt
	txFetchLocationByShaStmt
	txFetchLocUsedByShaStmt
	txUpdateUsedByShaStmt

	txtmpInsertStmt
	txtmpFetchUsedByShaStmt
	txtmpFetchLocationByShaStmt
	txtmpFetchLocUsedByShaStmt
	txtmpUpdateUsedByShaStmt

	txMigrateCopy
	txMigrateClear
	txMigratePrep
	txMigrateFinish
	txMigrateCount
	txPragmaVacuumOn
	txPragmaVacuumOff
	txVacuum
	txExistsShaStmt
	txtmpExistsShaStmt
)

var blkqueries []string = []string{
	blkInsertSha:    "INSERT INTO block (key, pver, data) VALUES(?, ?, ?);",
	blkFetchSha:     "SELECT pver, data, blockid FROM block WHERE key = ?;",
	blkExistsSha:    "SELECT pver FROM block WHERE key = ?;",
	blkFetchIdx:     "SELECT key FROM block WHERE blockid = ?;",
	blkFetchIdxList: "SELECT key FROM block WHERE blockid >= ? AND blockid < ? ORDER BY blockid ASC LIMIT 500;",
}

var txqueries []string = []string{
	txInsertStmt:             "INSERT INTO tx (key, blockid, txoff, txlen, data) VALUES(?, ?, ?, ?, ?);",
	txFetchUsedByShaStmt:     "SELECT data FROM tx WHERE key = ?;",
	txFetchLocationByShaStmt: "SELECT blockid, txoff, txlen FROM tx WHERE key = ?;",
	txFetchLocUsedByShaStmt:  "SELECT blockid, txoff, txlen, data FROM tx WHERE key = ?;",
	txUpdateUsedByShaStmt:    "UPDATE tx SET data = ? WHERE key  = ?;",

	txtmpInsertStmt:             "INSERT INTO txtmp (key, blockid, txoff, txlen, data) VALUES(?, ?, ?, ?, ?);",
	txtmpFetchUsedByShaStmt:     "SELECT data FROM txtmp WHERE key = ?;",
	txtmpFetchLocationByShaStmt: "SELECT blockid, txoff, txlen FROM txtmp WHERE key = ?;",
	txtmpFetchLocUsedByShaStmt:  "SELECT blockid, txoff, txlen, data FROM txtmp WHERE key = ?;",
	txtmpUpdateUsedByShaStmt:    "UPDATE txtmp SET data = ? WHERE key = ?;",

	txMigrateCopy:      "INSERT INTO tx (key, blockid, txoff, txlen, data) SELECT key, blockid, txoff, txlen, data FROM txtmp;",
	txMigrateClear:     "DELETE from txtmp;",
	txMigratePrep:      "DROP index IF EXISTS uniquetx;",
	txMigrateFinish:    "CREATE UNIQUE INDEX IF NOT EXISTS uniquetx ON tx (key);",
	txMigrateCount:     "SELECT COUNT(*) FROM txtmp;",
	txPragmaVacuumOn:   "PRAGMA auto_vacuum = FULL;",
	txPragmaVacuumOff:  "PRAGMA auto_vacuum = NONE;",
	txVacuum:           "VACUUM;",
	txExistsShaStmt:    "SELECT blockid FROM tx WHERE key = ?;",
	txtmpExistsShaStmt: "SELECT blockid FROM txtmp WHERE key = ?;",
}

var log = btclog.Disabled

type tBlockInsertData struct {
	sha  btcwire.ShaHash
	pver uint32
	buf  []byte
}
type tTxInsertData struct {
	txsha   *btcwire.ShaHash
	blockid int64
	txoff   int
	txlen   int
	usedbuf []byte
}

type txState struct {
	tx           *sql.Tx
	writeCount   int
	txDataSz     int
	txInsertList []interface{}
}
type SqliteDb struct {
	sqldb        *sql.DB
	blkStmts     []*sql.Stmt
	blkBaseStmts []*sql.Stmt
	txStmts      []*sql.Stmt
	txBaseStmts  []*sql.Stmt
	txState      txState
	dbLock       sync.Mutex

	lastBlkShaCached bool
	lastBlkSha       btcwire.ShaHash
	lastBlkIdx       int64
	txCache          txCache
	blockCache       blockCache

	UseTempTX  bool
	TempTblSz  int
	TempTblMax int

	dbInsertMode btcdb.InsertMode
}

var self = btcdb.DriverDB{DbType: "sqlite", Create: CreateSqliteDB, Open: OpenSqliteDB}

func init() {
	btcdb.AddDBDriver(self)
}

// createDB configure the database, setting up all tables to initial state.
func createDB(db *sql.DB) error {
	log.Infof("Initializing new block database")

	// XXX check for old tables
	buildTables := []string{
		"CREATE TABLE dbversion (version integer);",
		"CREATE TABLE block ( blockid INTEGER PRIMARY KEY, key BLOB UNIQUE, " +
			"pver INTEGER NOT NULL, data BLOB NOT NULL);",
		"INSERT INTO dbversion (version) VALUES (" + fmt.Sprintf("%d", dbVersion) +
			");",
	}
	buildtxTables := []string{
		"CREATE TABLE tx (txidx INTEGER PRIMARY KEY, " +
			"key TEXT, " +
			"blockid INTEGER NOT NULL, " +
			"txoff INTEGER NOT NULL,  txlen INTEGER NOT NULL, " +
			"data BLOB NOT NULL, " +
			"FOREIGN KEY(blockid) REFERENCES block(blockid));",
		"CREATE TABLE txtmp (key TEXT PRIMARY KEY, " +
			"blockid INTEGER NOT NULL, " +
			"txoff INTEGER NOT NULL,  txlen INTEGER NOT NULL, " +
			"data BLOB NOT NULL, " +
			"FOREIGN KEY(blockid) REFERENCES block(blockid));",
		"CREATE UNIQUE INDEX uniquetx ON tx (key);",
	}
	for _, sql := range buildTables {
		_, err := db.Exec(sql)
		if err != nil {
			log.Warnf("sql table op failed %v [%v]", err, sql)
			return err
		}
	}
	for _, sql := range buildtxTables {
		_, err := db.Exec(sql)
		if err != nil {
			log.Warnf("sql table op failed %v [%v]", err, sql)
			return err
		}
	}

	return nil
}

// OpenSqliteDB opens an existing database for use.
func OpenSqliteDB(filepath string) (pbdb btcdb.Db, err error) {
	log = btcdb.GetLog()
	return newOrCreateSqliteDB(filepath, false)
}

// CreateSqliteDB creates, initializes and opens a database for use.
func CreateSqliteDB(filepath string) (pbdb btcdb.Db, err error) {
	log = btcdb.GetLog()
	return newOrCreateSqliteDB(filepath, true)
}

// newOrCreateSqliteDB opens a database, either creating it or opens
// existing database based on flag.
func newOrCreateSqliteDB(filepath string, create bool) (pbdb btcdb.Db, err error) {
	var bdb SqliteDb
	if create == false {
		_, err = os.Stat(filepath)
		if err != nil {
			return nil, btcdb.DbDoesNotExist
		}
	}

	db, err := sql.Open("sqlite3", filepath)
	if err != nil {
		log.Warnf("db open failed %v\n", err)
		return nil, err
	}

	db.Exec("PRAGMA page_size=4096;")
	db.Exec("PRAGMA foreign_keys=ON;")
	db.Exec("PRAGMA journal_mode=WAL;")

	dbverstmt, err := db.Prepare("SELECT version FROM dbversion;")
	if err != nil {
		// about the only reason this would fail is that the database
		// is not initialized
		if create == false {
			return nil, btcdb.DbDoesNotExist
		}
		err = createDB(db)
		if err != nil {
			// already warned in the called function
			return nil, err
		}
		dbverstmt, err = db.Prepare("SELECT version FROM dbversion;")
		if err != nil {
			// if it failed this a second time, fail.
			return nil, err
		}

	}
	row := dbverstmt.QueryRow()
	var version int
	err = row.Scan(&version)
	if err != nil {
		log.Warnf("unable to find db version: no row\n", err)
	}
	switch version {
	case dbVersion:
		// all good
	default:
		log.Warnf("mismatch db version: %v expected %v\n", version, dbVersion)
		return nil, fmt.Errorf("Invalid version in database")
	}
	bdb.sqldb = db

	bdb.blkStmts = make([]*sql.Stmt, len(blkqueries))
	bdb.blkBaseStmts = make([]*sql.Stmt, len(blkqueries))
	for i := range blkqueries {
		stmt, err := db.Prepare(blkqueries[i])
		if err != nil {
			// XXX log/
			return nil, err
		}
		bdb.blkBaseStmts[i] = stmt
	}
	for i := range bdb.blkBaseStmts {
		bdb.blkStmts[i] = bdb.blkBaseStmts[i]
	}

	bdb.txBaseStmts = make([]*sql.Stmt, len(txqueries))
	for i := range txqueries {
		stmt, err := db.Prepare(txqueries[i])
		if err != nil {
			// XXX log/
			return nil, err
		}
		bdb.txBaseStmts[i] = stmt
	}
	// NOTE: all array entries in txStmts remain nil'ed
	// tx statements are lazy bound
	bdb.txStmts = make([]*sql.Stmt, len(txqueries))

	bdb.blockCache.maxcount = 150
	bdb.blockCache.blockMap = map[btcwire.ShaHash]*blockCacheObj{}
	bdb.blockCache.blockMap = map[btcwire.ShaHash]*blockCacheObj{}
	bdb.blockCache.blockHeightMap = map[int64]*blockCacheObj{}
	bdb.txCache.maxcount = 2000
	bdb.txCache.txMap = map[btcwire.ShaHash]*txCacheObj{}

	bdb.UseTempTX = true
	bdb.TempTblMax = 1000000

	return &bdb, nil
}

// Sync verifies that the database is coherent on disk,
// and no outstanding transactions are in flight.
func (db *SqliteDb) Sync() {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	db.endTx(true)
}

// syncPoint notifies the db that this is a safe time to sync the database,
// if there are many outstanding transactions.
// Must be called with db lock held.
func (db *SqliteDb) syncPoint() {

	tx := &db.txState

	if db.TempTblSz > db.TempTblMax {
		err := db.migrateTmpTable()
		if err != nil {
			return
		}
	} else {
		if len(tx.txInsertList) > dbMaxTransCnt || tx.txDataSz > dbMaxTransMem {
			db.endTx(true)
		}
	}
}

// Close cleanly shuts down database, syncing all data.
func (db *SqliteDb) Close() {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	db.close()
}

// RollbackClose discards the recent database changes to the previously
// saved data at last Sync.
func (db *SqliteDb) RollbackClose() {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	tx := &db.txState
	if tx.tx != nil {
		err := tx.tx.Rollback()
		if err != nil {
			log.Debugf("Rollback failed: %v", err)
		} else {
			tx.tx = nil
		}
	}
	db.close()
}

// close performs the internal shutdown/close operation.
func (db *SqliteDb) close() {
	db.endTx(true)

	db.InvalidateCache()

	for i := range db.blkBaseStmts {
		db.blkBaseStmts[i].Close()
	}
	for i := range db.txBaseStmts {
		if db.txBaseStmts[i] != nil {
			db.txBaseStmts[i].Close()
			db.txBaseStmts[i] = nil
		}
	}
	db.sqldb.Close()
}

// txop returns the appropriately prepared statement, based on
// transaction state of the database.
func (db *SqliteDb) txop(op int) *sql.Stmt {
	if db.txStmts[op] != nil {
		return db.txStmts[op]
	}
	if db.txState.tx == nil {
		// we are not in a transaction, return the base statement
		return db.txBaseStmts[op]
	}

	if db.txStmts[op] == nil {
		db.txStmts[op] = db.txState.tx.Stmt(db.txBaseStmts[op])
	}

	return db.txStmts[op]
}

// startTx starts a transaction, preparing or scrubbing statements
// for proper operation inside a transaction.
func (db *SqliteDb) startTx() (err error) {
	tx := &db.txState
	if tx.tx != nil {
		// this shouldn't happen...
		log.Warnf("Db startTx called while in a transaction")
		return
	}
	tx.tx, err = db.sqldb.Begin()
	if err != nil {
		log.Warnf("Db startTx: begin failed %v", err)
		tx.tx = nil
		return
	}
	for i := range db.blkBaseStmts {
		db.blkStmts[i] = tx.tx.Stmt(db.blkBaseStmts[i])
	}
	for i := range db.txBaseStmts {
		db.txStmts[i] = nil // these are lazily prepared
	}
	return
}

// endTx commits the current active transaction, it zaps all of the prepared
// statements associated with the transaction.
func (db *SqliteDb) endTx(recover bool) (err error) {
	tx := &db.txState

	if tx.tx == nil {
		return
	}

	err = tx.tx.Commit()
	if err != nil && recover {
		// XXX - double check that the tx is dead after
		// commit failure (rollback?)

		log.Warnf("Db endTx: commit failed %v", err)
		err = db.rePlayTransaction()
		if err != nil {
			// We tried, return failure (after zeroing state)
			// so the upper level can notice and restart
		}
	}
	for i := range db.blkBaseStmts {
		db.blkStmts[i].Close()
		db.blkStmts[i] = db.blkBaseStmts[i]
	}
	for i := range db.txStmts {
		if db.txStmts[i] != nil {
			db.txStmts[i].Close()
			db.txStmts[i] = nil
		}
	}
	tx.tx = nil
	var emptyTxList []interface{}
	tx.txInsertList = emptyTxList
	tx.txDataSz = 0
	return
}

// rePlayTransaction will attempt to re-execute inserts performed
// sync the beginning of a transaction. This is to be used after
// a sql Commit operation fails to keep the database from losing data.
func (db *SqliteDb) rePlayTransaction() (err error) {
	err = db.startTx()
	if err != nil {
		return
	}
	tx := &db.txState
	for _, ins := range tx.txInsertList {
		switch v := ins.(type) {
		case tBlockInsertData:
			block := v
			_, err = db.blkStmts[blkInsertSha].Exec(block.sha.Bytes(),
				block.pver, block.buf)
			if err != nil {
				break
			}
		case tTxInsertData:
			txd := v
			txnamebytes := txd.txsha.Bytes()
			txop := db.txop(txInsertStmt)
			_, err = txop.Exec(txd.blockid, txnamebytes, txd.txoff,
				txd.txlen, txd.usedbuf)
			if err != nil {
				break
			}
		}
	}
	// This function is called even if we have failed.
	// We need to clean up so the database can be used again.
	// However we want the original error not any new error,
	// unless there was no original error but the commit fails.
	err2 := db.endTx(false)
	if err == nil && err2 != nil {
		err = err2
	}

	return
}

// DropAfterBlockBySha will remove any blocks from the database after the given block.
// It terminates any existing transaction and performs its operations in an
// atomic transaction, it is terminated (committed) before exit.
func (db *SqliteDb) DropAfterBlockBySha(sha *btcwire.ShaHash) (err error) {
	var row *sql.Row
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	// This is a destructive operation and involves multiple requests
	// so requires a transaction, terminate any transaction to date
	// and start a new transaction
	err = db.endTx(true)
	if err != nil {
		return err
	}
	err = db.startTx()
	if err != nil {
		return err
	}

	var startheight int64

	if db.lastBlkShaCached {
		startheight = db.lastBlkIdx
	} else {
		querystr := "SELECT blockid FROM block ORDER BY blockid DESC;"

		tx := &db.txState
		if tx.tx != nil {
			row = tx.tx.QueryRow(querystr)
		} else {
			row = db.sqldb.QueryRow(querystr)
		}
		var startblkidx int64
		err = row.Scan(&startblkidx)
		if err != nil {
			log.Warnf("DropAfterBlockBySha:unable to fetch blockheight %v", err)
			return err
		}
		startheight = startblkidx
	}
	// also drop any cached sha data
	db.lastBlkShaCached = false

	querystr := "SELECT blockid FROM block WHERE key = ?;"

	tx := &db.txState
	row = tx.tx.QueryRow(querystr, sha.Bytes())

	var keepidx int64
	err = row.Scan(&keepidx)
	if err != nil {
		// XXX
		db.endTx(false)
		return err
	}

	for height := startheight; height > keepidx; height = height - 1 {
		var blk *btcutil.Block
		blkc, ok := db.fetchBlockHeightCache(height)

		if ok {
			blk = blkc.blk
		} else {
			// must load the block from the db
			sha, err = db.fetchBlockShaByHeight(height - 1)
			if err != nil {
				return
			}

			var buf []byte
			buf, _, _, err = db.fetchSha(*sha)
			if err != nil {
				return
			}

			blk, err = btcutil.NewBlockFromBytes(buf)
			if err != nil {
				return
			}
		}

		for _, tx := range blk.MsgBlock().Transactions {
			err = db.unSpend(tx)
			if err != nil {
				return
			}
		}
	}

	// invalidate the cache after possibly using cached entries for block
	// lookup to unspend coins in them
	db.InvalidateCache()

	return db.delFromDB(keepidx)
}

func (db *SqliteDb) delFromDB(keepidx int64) error {
	tx := &db.txState
	_, err := tx.tx.Exec("DELETE FROM txtmp WHERE blockid > ?", keepidx)
	if err != nil {
		// XXX
		db.endTx(false)
		return err
	}

	_, err = tx.tx.Exec("DELETE FROM tx WHERE blockid > ?", keepidx)
	if err != nil {
		// XXX
		db.endTx(false)
		return err
	}

	// delete from block last in case of foreign keys
	_, err = tx.tx.Exec("DELETE FROM block WHERE blockid > ?", keepidx)
	if err != nil {
		// XXX
		db.endTx(false)
		return err
	}

	err = db.endTx(true)
	if err != nil {
		return err
	}
	return err
}

// InsertBlock inserts raw block and transaction data from a block into the
// database.  The first block inserted into the database will be treated as the
// genesis block.  Every subsequent block insert requires the referenced parent
// block to already exist.
func (db *SqliteDb) InsertBlock(block *btcutil.Block) (int64, error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	blocksha, err := block.Sha()
	if err != nil {
		log.Warnf("Failed to compute block sha %v", blocksha)
		return -1, err
	}

	mblock := block.MsgBlock()
	rawMsg, err := block.Bytes()
	if err != nil {
		log.Warnf("Failed to obtain raw block sha %v", blocksha)
		return -1, err
	}
	txloc, err := block.TxLoc()
	if err != nil {
		log.Warnf("Failed to obtain raw block sha %v", blocksha)
		return -1, err
	}

	// Insert block into database
	newheight, err := db.insertBlockData(blocksha, &mblock.Header.PrevBlock,
		0, rawMsg)
	if err != nil {
		log.Warnf("Failed to insert block %v %v %v", blocksha,
			&mblock.Header.PrevBlock, err)
		return -1, err
	}

	txinsertidx := -1
	success := false

	defer func() {
		if success {
			return
		}

		for txidx := 0; txidx <= txinsertidx; txidx++ {
			tx := mblock.Transactions[txidx]

			err = db.unSpend(tx)
			if err != nil {
				log.Warnf("unSpend error during block insert unwind %v %v %v", blocksha, txidx, err)
			}
		}

		err = db.delFromDB(newheight - 1)
		if err != nil {
			log.Warnf("Error during block insert unwind %v %v", blocksha, err)
		}
	}()

	// At least two blocks in the long past were generated by faulty
	// miners, the sha of the transaction exists in a previous block,
	// detect this condition and 'accept' the block.
	for txidx, tx := range mblock.Transactions {
		var txsha btcwire.ShaHash
		txsha, err = tx.TxSha()
		if err != nil {
			log.Warnf("failed to compute tx name block %v idx %v err %v", blocksha, txidx, err)
			return -1, err
		}

		// num tx inserted, thus would need unwind if failure occurs
		txinsertidx = txidx

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
				log.Tracef("skipping sha %v %v", dupsha, newheight)
				continue
			}
		}
		if newheight == 91880 {
			dupsha, err := btcwire.NewShaHashFromStr("e3bf3d07d4b0375638d5f1db5255fe07ba2c4cb067cd81b84ee974b6585fb468")
			if err != nil {
				panic("invalid sha string in source")
			}
			if txsha == *dupsha {
				log.Tracef("skipping sha %v %v", dupsha, newheight)
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
			var oBlkIdx int64
			oBlkIdx, _, _, err = db.fetchLocationBySha(&txsha)
			log.Warnf("oblkidx %v err %v", oBlkIdx, err)

			return -1, err
		}
		err = db.doSpend(tx)
		if err != nil {
			log.Warnf("block %v idx %v failed to spend tx %v %v err %v", blocksha, newheight, &txsha, txidx, err)

			return -1, err
		}
	}
	success = true
	db.syncPoint()
	return newheight, nil
}

// SetDBInsertMode provides hints to the database to how the application
// is running this allows the database to work in optimized modes when the
// database may be very busy.
func (db *SqliteDb) SetDBInsertMode(newmode btcdb.InsertMode) {

	oldMode := db.dbInsertMode
	switch newmode {
	case btcdb.InsertNormal:
		// Normal mode inserts tx directly into the tx table
		db.UseTempTX = false
		db.dbInsertMode = newmode
		switch oldMode {
		case btcdb.InsertFast:
			if db.TempTblSz != 0 {
				err := db.migrateTmpTable()
				if err != nil {
					return
				}
			}
		case btcdb.InsertValidatedInput:
			// generate tx indexes
			txop := db.txop(txMigrateFinish)
			_, err := txop.Exec()
			if err != nil {
				log.Warnf("Failed to create tx table index - %v", err)
			}
		}
	case btcdb.InsertFast:
		// Fast mode inserts tx into txtmp with validation,
		// then dumps to tx then rebuilds indexes at thresholds
		db.UseTempTX = true
		if oldMode != btcdb.InsertNormal {
			log.Warnf("switching between invalid DB modes")
			break
		}
		db.dbInsertMode = newmode
	case btcdb.InsertValidatedInput:
		// ValidatedInput mode inserts into tx table with
		// no duplicate checks, then builds index on exit from
		// ValidatedInput mode
		if oldMode != btcdb.InsertNormal {
			log.Warnf("switching between invalid DB modes")
			break
		}
		// remove tx table index
		txop := db.txop(txMigratePrep)
		_, err := txop.Exec()
		if err != nil {
			log.Warnf("Failed to clear tx table index - %v", err)
		}
		db.dbInsertMode = newmode

		// XXX
		db.UseTempTX = false
	}
}
func (db *SqliteDb) doSpend(tx *btcwire.MsgTx) error {
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

func (db *SqliteDb) unSpend(tx *btcwire.MsgTx) error {
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

func (db *SqliteDb) setSpentData(sha *btcwire.ShaHash, idx uint32) error {
	return db.setclearSpentData(sha, idx, true)
}

func (db *SqliteDb) clearSpentData(sha *btcwire.ShaHash, idx uint32) error {
	return db.setclearSpentData(sha, idx, false)
}

func (db *SqliteDb) setclearSpentData(txsha *btcwire.ShaHash, idx uint32, set bool) error {
	var spentdata []byte
	usingtmp := false
	txop := db.txop(txFetchUsedByShaStmt)
	row := txop.QueryRow(txsha.String())
	err := row.Scan(&spentdata)
	if err != nil {
		// if the error is simply didn't fine continue otherwise
		// retun failure

		usingtmp = true
		txop = db.txop(txtmpFetchUsedByShaStmt)
		row := txop.QueryRow(txsha.String())
		err := row.Scan(&spentdata)
		if err != nil {
			log.Warnf("Failed to locate spent data - %v %v", txsha, err)
			return err
		}
	}
	byteidx := idx / 8
	byteoff := idx % 8

	if set {
		spentdata[byteidx] |= (byte(1) << byteoff)
	} else {
		spentdata[byteidx] &= ^(byte(1) << byteoff)
	}
	txc, cached := db.fetchTxCache(txsha)
	if cached {
		txc.spent = spentdata
	}

	if usingtmp {
		txop = db.txop(txtmpUpdateUsedByShaStmt)
	} else {
		txop = db.txop(txUpdateUsedByShaStmt)
	}
	_, err = txop.Exec(spentdata, txsha.String())

	return err
}
