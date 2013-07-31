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

// InsertTx inserts a tx hash and its associated data into the database.
func (db *SqliteDb) InsertTx(txsha *btcwire.ShaHash, height int64, txoff int, txlen int, usedbuf []byte) (err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	return db.insertTx(txsha, height, txoff, txlen, usedbuf)
}

// insertTx inserts a tx hash and its associated data into the database.
// Must be called with db lock held.
func (db *SqliteDb) insertTx(txsha *btcwire.ShaHash, height int64, txoff int, txlen int, usedbuf []byte) (err error) {

	tx := &db.txState
	if tx.tx == nil {
		err = db.startTx()
		if err != nil {
			return
		}
	}
	blockid := height + 1
	txd := tTxInsertData{txsha: txsha, blockid: blockid, txoff: txoff, txlen: txlen, usedbuf: usedbuf}

	log.Tracef("inserting tx %v for block %v off %v len %v",
		txsha, blockid, txoff, txlen)

	rowBytes := txsha.String()

	var op int // which table to insert data into.
	if db.UseTempTX {
		var tblockid int64
		var ttxoff int
		var ttxlen int
		txop := db.txop(txFetchLocationByShaStmt)
		row := txop.QueryRow(rowBytes)
		err = row.Scan(&tblockid, &ttxoff, &ttxlen)
		if err != sql.ErrNoRows {
			// sha already present
			err = btcdb.DuplicateSha
			return
		}
		op = txtmpInsertStmt
	} else {
		op = txInsertStmt
	}

	txop := db.txop(op)
	_, err = txop.Exec(rowBytes, blockid, txoff, txlen, usedbuf)
	if err != nil {
		log.Warnf("failed to insert %v %v %v", txsha, blockid, err)
		return
	}
	if db.UseTempTX {
		db.TempTblSz++
	}

	// put in insert list for replay
	tx.txInsertList = append(tx.txInsertList, txd)

	return
}

// ExistsTxSha returns if the given tx sha exists in the database
func (db *SqliteDb) ExistsTxSha(txsha *btcwire.ShaHash) (exists bool) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	if _, ok := db.fetchTxCache(txsha); ok {
		return true
	}

	return db.existsTxSha(txsha)
}

// existsTxSha returns if the given tx sha exists in the database.o
// Must be called with the db lock held.
func (db *SqliteDb) existsTxSha(txsha *btcwire.ShaHash) (exists bool) {
	var blockid uint32

	txop := db.txop(txExistsShaStmt)
	row := txop.QueryRow(txsha.String())
	err := row.Scan(&blockid)

	if err == sql.ErrNoRows {
		txop = db.txop(txtmpExistsShaStmt)
		row = txop.QueryRow(txsha.String())
		err := row.Scan(&blockid)

		if err == sql.ErrNoRows {
			return false
		}
		if err != nil {
			log.Warnf("txTmpExistsTxSha: fail %v", err)
			return false
		}
		log.Warnf("txtmpExistsTxSha: success")
		return true
	}

	if err != nil {
		// ignore real errors?
		log.Warnf("existsTxSha: fail %v", err)
		return false
	}

	return true
}

// FetchLocationBySha looks up the Tx sha information by name.
func (db *SqliteDb) FetchLocationBySha(txsha *btcwire.ShaHash) (blockidx int64, txoff int, txlen int, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()
	return db.fetchLocationBySha(txsha)
}

// fetchLocationBySha look up the Tx sha information by name.
// Must be called with db lock held.
func (db *SqliteDb) fetchLocationBySha(txsha *btcwire.ShaHash) (height int64, txoff int, txlen int, err error) {
	var row *sql.Row
	var blockid int64
	var ttxoff int
	var ttxlen int

	rowBytes := txsha.String()
	txop := db.txop(txFetchLocationByShaStmt)
	row = txop.QueryRow(rowBytes)

	err = row.Scan(&blockid, &ttxoff, &ttxlen)
	if err == sql.ErrNoRows {
		txop = db.txop(txtmpFetchLocationByShaStmt)
		row = txop.QueryRow(rowBytes)

		err = row.Scan(&blockid, &ttxoff, &ttxlen)
		if err == sql.ErrNoRows {
			err = btcdb.TxShaMissing
			return
		}
		if err != nil {
			log.Warnf("txtmp FetchLocationBySha: fail %v",
				err)
			return
		}
	}
	if err != nil {
		log.Warnf("FetchLocationBySha: fail %v", err)
		return
	}
	height = blockid - 1
	txoff = ttxoff
	txlen = ttxlen
	return
}

// fetchLocationUsedBySha look up the Tx sha information by name.
// Must be called with db lock held.
func (db *SqliteDb) fetchLocationUsedBySha(txsha *btcwire.ShaHash) (rheight int64, rtxoff int, rtxlen int, rspentbuf []byte, err error) {
	var row *sql.Row
	var blockid int64
	var txoff int
	var txlen int
	var txspent []byte

	rowBytes := txsha.String()
	txop := db.txop(txFetchLocUsedByShaStmt)
	row = txop.QueryRow(rowBytes)

	err = row.Scan(&blockid, &txoff, &txlen, &txspent)
	if err == sql.ErrNoRows {
		txop = db.txop(txtmpFetchLocUsedByShaStmt)
		row = txop.QueryRow(rowBytes)

		err = row.Scan(&blockid, &txoff, &txlen, &txspent)
		if err == sql.ErrNoRows {
			err = btcdb.TxShaMissing
			return
		}
		if err != nil {
			log.Warnf("txtmp FetchLocationBySha: fail %v",
				err)
			return
		}
	}
	if err != nil {
		log.Warnf("FetchLocationBySha: fail %v", err)
		return
	}
	height := blockid - 1
	return height, txoff, txlen, txspent, nil
}

// FetchTxUsedBySha returns the used/spent buffer for a given transaction.
func (db *SqliteDb) FetchTxUsedBySha(txsha *btcwire.ShaHash) (spentbuf []byte, err error) {
	var row *sql.Row
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	rowBytes := txsha.String()
	txop := db.txop(txFetchUsedByShaStmt)
	row = txop.QueryRow(rowBytes)

	var databytes []byte
	err = row.Scan(&databytes)
	if err == sql.ErrNoRows {
		txop := db.txop(txtmpFetchUsedByShaStmt)
		row = txop.QueryRow(rowBytes)

		err = row.Scan(&databytes)
		if err == sql.ErrNoRows {
			err = btcdb.TxShaMissing
			return
		}
		if err != nil {
			log.Warnf("txtmp FetchLocationBySha: fail %v",
				err)
			return
		}
	}

	if err != nil {
		log.Warnf("FetchUsedBySha: fail %v", err)
		return
	}
	spentbuf = databytes
	return
}

var vaccumDbNextMigrate bool

// migrateTmpTable functions to perform internal db optimization when
// performing large numbers of database inserts. When in Fast operation
// mode, it inserts into txtmp, then when that table reaches a certain
// size limit it moves all tx in the txtmp table into the primary tx
// table and recomputes the index on the primary tx table.
func (db *SqliteDb) migrateTmpTable() error {
	db.endTx(true)
	db.startTx() // ???

	db.UseTempTX = false
	db.TempTblSz = 0

	var doVacuum bool
	var nsteps int
	if vaccumDbNextMigrate {
		nsteps = 6
		vaccumDbNextMigrate = false
		doVacuum = true
	} else {
		nsteps = 5
		vaccumDbNextMigrate = true
	}

	log.Infof("db compaction Stage 1/%v: Preparing", nsteps)
	txop := db.txop(txMigratePrep)
	_, err := txop.Exec()
	if err != nil {
		log.Warnf("Failed to prepare migrate - %v", err)
		return err
	}

	log.Infof("db compaction Stage 2/%v: Copying", nsteps)
	txop = db.txop(txMigrateCopy)
	_, err = txop.Exec()
	if err != nil {
		log.Warnf("Migrate read failed - %v", err)
		return err
	}

	log.Tracef("db compaction Stage 2a/%v: Enable db vacuum", nsteps)
	txop = db.txop(txPragmaVacuumOn)
	_, err = txop.Exec()
	if err != nil {
		log.Warnf("Migrate error trying to enable vacuum on "+
			"temporary transaction table - %v", err)
		return err
	}

	log.Infof("db compaction Stage 3/%v: Clearing old data", nsteps)
	txop = db.txop(txMigrateClear)
	_, err = txop.Exec()
	if err != nil {
		log.Warnf("Migrate error trying to clear temporary "+
			"transaction table - %v", err)
		return err
	}

	log.Tracef("db compaction Stage 3a/%v: Disable db vacuum", nsteps)
	txop = db.txop(txPragmaVacuumOff)
	_, err = txop.Exec()
	if err != nil {
		log.Warnf("Migrate error trying to disable vacuum on "+
			"temporary transaction table - %v", err)
		return err
	}

	log.Infof("db compaction Stage 4/%v: Rebuilding index", nsteps)
	txop = db.txop(txMigrateFinish)
	_, err = txop.Exec()
	if err != nil {
		log.Warnf("Migrate error trying to clear temporary "+
			"transaction table - %v", err)
		return err
	}

	log.Infof("db compaction Stage 5/%v: Finalizing transaction", nsteps)
	db.endTx(true) // ???

	if doVacuum {
		log.Infof("db compaction Stage 6/%v: Optimizing database", nsteps)
		txop = db.txop(txVacuum)
		_, err = txop.Exec()
		if err != nil {
			log.Warnf("migrate error trying to clear txtmp tbl %v", err)
			return err
		}
	}

	log.Infof("db compaction: Complete")

	// TODO(drahn) - determine if this should be turned back on or not
	db.UseTempTX = true

	return nil
}
