// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb

import (
	"fmt"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcwire"
)

// FetchSha returns the datablock and pver for the given ShaHash.
// This is a testing only interface.
func FetchSha(db btcdb.Db, sha *btcwire.ShaHash) (buf []byte, pver uint32,
	blkid int64, err error) {
	sqldb, ok := db.(*LevelDb)
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
	sqldb, ok := db.(*LevelDb)
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
	sqldb, ok := db.(*LevelDb)
	if !ok {
		return
	}
	tc := &sqldb.txCache
	tc.maxcount = newsize
}
