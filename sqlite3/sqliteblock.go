// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package sqlite3

import (
	"database/sql"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcwire"
	_ "github.com/mattn/go-sqlite3"
)

// InsertBlockData stores a block hash and its associated data block with a
// previous sha of `prevSha' and a version of `pver'.
func (db *SqliteDb) InsertBlockData(sha *btcwire.ShaHash, prevSha *btcwire.ShaHash, pver uint32, buf []byte) (blockid int64, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	return db.insertBlockData(sha, prevSha, pver, buf)
}

// insertSha stores a block hash and its associated data block with a
// previous sha of `prevSha' and a version of `pver'.
// insertSha shall be called with db lock held
func (db *SqliteDb) insertBlockData(sha *btcwire.ShaHash, prevSha *btcwire.ShaHash, pver uint32, buf []byte) (blockid int64, err error) {
	tx := &db.txState
	if tx.tx == nil {
		err = db.startTx()
		if err != nil {
			return
		}
	}

	// It is an error if the previous block does not already exist in the
	// database, unless there are no blocks at all.
	if prevOk := db.blkExistsSha(prevSha); !prevOk {
		var numBlocks uint64
		querystr := "SELECT COUNT(blockid) FROM block;"
		err := db.sqldb.QueryRow(querystr).Scan(&numBlocks)
		if err != nil {
			return 0, err
		}
		if numBlocks != 0 {
			return 0, btcdb.PrevShaMissing
		}
	}

	result, err := db.blkStmts[blkInsertSha].Exec(sha.Bytes(), pver, buf)
	if err != nil {
		return
	}

	blkid, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	blkid -= 1 // skew between btc blockid and sql

	// Because we don't know know what the last idx is, we don't
	// cache unless already cached
	if db.lastBlkShaCached == true {
		db.lastBlkSha = *sha
		db.lastBlkIdx++
	}

	bid := tBlockInsertData{*sha, pver, buf}
	tx.txInsertList = append(tx.txInsertList, bid)
	tx.txDataSz += len(buf)

	blockid = blkid
	return
}

// fetchSha returns the datablock and pver for the given ShaHash.
func (db *SqliteDb) fetchSha(sha btcwire.ShaHash) (buf []byte, pver uint32,
	blkid int64, err error) {

	row := db.blkStmts[blkFetchSha].QueryRow(sha.Bytes())

	var blockidx int64
	var databytes []byte
	err = row.Scan(&pver, &databytes, &blockidx)
	if err == sql.ErrNoRows {
		return // no warning
	}
	if err != nil {
		log.Warnf("fail 2 %v", err)
		return
	}
	buf = databytes
	blkid = blockidx - 1 // skew between btc blockid and sql
	return
}

// ExistsSha looks up the given block hash
// returns true if it is present in the database.
func (db *SqliteDb) ExistsSha(sha *btcwire.ShaHash) (exists bool) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	_, exists = db.fetchBlockCache(sha)
	if exists {
		return
	}

	// not in cache, try database
	exists = db.blkExistsSha(sha)
	return
}

// blkExistsSha looks up the given block hash
// returns true if it is present in the database.
// CALLED WITH LOCK HELD
func (db *SqliteDb) blkExistsSha(sha *btcwire.ShaHash) bool {
	var pver uint32

	row := db.blkStmts[blkExistsSha].QueryRow(sha.Bytes())
	err := row.Scan(&pver)

	if err == sql.ErrNoRows {
		return false
	}

	if err != nil {
		// ignore real errors?
		log.Warnf("blkExistsSha: fail %v", err)
		return false
	}
	return true
}

// FetchBlockShaByHeight returns a block hash based on its height in the
// block chain.
func (db *SqliteDb) FetchBlockShaByHeight(height int64) (sha *btcwire.ShaHash, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	return db.fetchBlockShaByHeight(height)
}

// fetchBlockShaByHeight returns a block hash based on its height in the
// block chain.
func (db *SqliteDb) fetchBlockShaByHeight(height int64) (sha *btcwire.ShaHash, err error) {
	var row *sql.Row

	blockidx := height + 1 // skew between btc blockid and sql

	row = db.blkStmts[blkFetchIdx].QueryRow(blockidx)

	var shabytes []byte
	err = row.Scan(&shabytes)
	if err != nil {
		return
	}
	var shaval btcwire.ShaHash
	shaval.SetBytes(shabytes)
	return &shaval, nil
}

// FetchHeightRange looks up a range of blocks by the start and ending
// heights.  Fetch is inclusive of the start height and exclusive of the
// ending height. To fetch all hashes from the start height until no
// more are present, use the special id `AllShas'.
func (db *SqliteDb) FetchHeightRange(startHeight, endHeight int64) (rshalist []btcwire.ShaHash, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	startidx := startHeight + 1 // skew between btc block height and sql

	var endidx int64
	if endHeight == btcdb.AllShas {
		endidx = btcdb.AllShas // no skew if asking for all
	} else {
		endidx = endHeight + 1 // skew between btc block height and sql
	}
	rows, err := db.blkStmts[blkFetchIdxList].Query(startidx, endidx)
	if err != nil {
		log.Warnf("query failed %v", err)
		return
	}

	var shalist []btcwire.ShaHash
	for rows.Next() {
		var sha btcwire.ShaHash
		var shabytes []byte
		err = rows.Scan(&shabytes)
		if err != nil {
			log.Warnf("wtf? %v", err)
			break
		}
		sha.SetBytes(shabytes)
		shalist = append(shalist, sha)
	}
	rows.Close()
	if err == nil {
		rshalist = shalist
	}
	log.Tracef("FetchIdxRange idx %v %v returned %v shas err %v", startHeight, endHeight, len(shalist), err)
	return
}

// NewestSha returns the hash and block height of the most recent (end) block of
// the block chain.  It will return the zero hash, -1 for the block height, and
// no error (nil) if there are not any blocks in the database yet.
func (db *SqliteDb) NewestSha() (sha *btcwire.ShaHash, blkid int64, err error) {
	var row *sql.Row
	var blockidx int64
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	// answer may be cached
	if db.lastBlkShaCached == true {
		shacopy := db.lastBlkSha
		sha = &shacopy
		blkid = db.lastBlkIdx - 1 // skew between btc blockid and sql
		return
	}

	querystr := "SELECT key, blockid FROM block ORDER BY blockid DESC;"

	tx := &db.txState
	if tx.tx != nil {
		row = tx.tx.QueryRow(querystr)
	} else {
		row = db.sqldb.QueryRow(querystr)
	}

	var shabytes []byte
	err = row.Scan(&shabytes, &blockidx)
	if err == sql.ErrNoRows {
		return &btcwire.ShaHash{}, -1, nil
	}
	if err == nil {
		var retsha btcwire.ShaHash
		retsha.SetBytes(shabytes)
		sha = &retsha
		blkid = blockidx - 1 // skew between btc blockid and sql

		db.lastBlkSha = retsha
		db.lastBlkIdx = blockidx
		db.lastBlkShaCached = true
	}
	return
}

type SqliteBlockIterator struct {
	rows *sql.Rows
	stmt *sql.Stmt
	db   *SqliteDb
}

// NextRow iterates thru all blocks in database.
func (bi *SqliteBlockIterator) NextRow() bool {
	return bi.rows.Next()
}

// Row returns row data for block iterator.
func (bi *SqliteBlockIterator) Row() (key *btcwire.ShaHash, pver uint32,
	buf []byte, err error) {
	var keybytes []byte

	err = bi.rows.Scan(&keybytes, &pver, &buf)
	if err == nil {
		var retkey btcwire.ShaHash
		retkey.SetBytes(keybytes)
		key = &retkey
	}
	return
}

// Close shuts down the iterator when done walking blocks in the database.
func (bi *SqliteBlockIterator) Close() {
	bi.rows.Close()
	bi.stmt.Close()
}

// NewIterateBlocks prepares iterator for all blocks in database.
func (db *SqliteDb) NewIterateBlocks() (btcdb.BlockIterator, error) {
	var bi SqliteBlockIterator
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	stmt, err := db.sqldb.Prepare("SELECT key, pver, data FROM block ORDER BY blockid;")
	if err != nil {
		return nil, err
	}
	tx := &db.txState
	if tx.tx != nil {
		txstmt := tx.tx.Stmt(stmt)
		stmt.Close()
		stmt = txstmt
	}
	bi.stmt = stmt

	bi.rows, err = bi.stmt.Query()
	if err != nil {
		return nil, err
	}
	bi.db = db

	return &bi, nil
}
