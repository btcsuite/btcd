// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package sqlite3

import (
	"fmt"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcwire"
	"testing"
)

var t *testing.T

func SetTestingT(t_arg *testing.T) {
	t = t_arg
}

// FetchSha returns the datablock and pver for the given ShaHash.
// This is a testing only interface.
func FetchSha(db btcdb.Db, sha *btcwire.ShaHash) (buf []byte, pver uint32,
	blkid int64, err error) {
	sqldb, ok := db.(*SqliteDb)
	if !ok {
		err = fmt.Errorf("Invalid data type")
		return
	}
	buf, pver, blkid, err = sqldb.fetchSha(*sha)
	return
}

// SetBlockCacheSize configures the maximum number of blocks in the cache to
// be the given size should be made before any fetching.
// This is a testing only interface.
func SetBlockCacheSize(db btcdb.Db, newsize int) {
	sqldb, ok := db.(*SqliteDb)
	if !ok {
		return
	}
	bc := &sqldb.blockCache
	bc.maxcount = newsize
}

// SetTxCacheSize configures the maximum number of tx in the cache to
// be the given size should be made before any fetching.
// This is a testing only interface.
func SetTxCacheSize(db btcdb.Db, newsize int) {
	sqldb, ok := db.(*SqliteDb)
	if !ok {
		return
	}
	tc := &sqldb.txCache
	tc.maxcount = newsize
}

// KillTx is a function that deletes a transaction from the database
// this should only be used for testing purposes to valiate error paths
// in the database. This is _expected_ to leave the database in an
// inconsistant state.
func KillTx(dbarg btcdb.Db, txsha *btcwire.ShaHash) {
	db, ok := dbarg.(*SqliteDb)
	if !ok {
		return
	}
	db.endTx(false)
	db.startTx()
	tx := &db.txState
	key := txsha.String()
	_, err := tx.tx.Exec("DELETE FROM txtmp WHERE key == ?", key)
	if err != nil {
		log.Warnf("error deleting tx %v from txtmp", txsha)
	}
	_, err = tx.tx.Exec("DELETE FROM tx WHERE key == ?", key)
	if err != nil {
		log.Warnf("error deleting tx %v from tx (%v)", txsha, key)
	}
	err = db.endTx(true)
	if err != nil {
		// XXX
		db.endTx(false)
	}
	db.InvalidateCache()
}
