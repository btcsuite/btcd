// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package memdb

import (
	"github.com/conformal/btcdb"
	"github.com/conformal/seelog"
)

var log = seelog.Disabled

func init() {
	driver := btcdb.DriverDB{DbType: "memdb", Create: CreateDB, Open: OpenDB}
	btcdb.AddDBDriver(driver)
}

// OpenDB opens an existing database for use.
func OpenDB(dbpath string) (btcdb.Db, error) {
	// A memory database is not persistent, so let CreateDB handle it.
	return CreateDB(dbpath)
}

// CreateDB creates, initializes, and opens a database for use.
func CreateDB(dbpath string) (btcdb.Db, error) {
	log = btcdb.GetLog()
	return newMemDb(), nil
}
