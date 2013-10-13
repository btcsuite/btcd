// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcdb_test

import (
	"fmt"
	"github.com/conformal/btcdb"
	_ "github.com/conformal/btcdb/ldb"
	_ "github.com/conformal/btcdb/sqlite3"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"os"
	"path/filepath"
)

var zeroHash = btcwire.ShaHash{}

// testDbRoot is the root directory used to create all test databases.
const testDbRoot = "testdbs"

// filesExists returns whether or not the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// openDB is used to open an existing database based on the database type and
// name.
func openDB(dbType, dbName string) (btcdb.Db, error) {
	// Handle memdb specially since it has no files on disk.
	if dbType == "memdb" {
		db, err := btcdb.OpenDB(dbType, "")
		if err != nil {
			return nil, fmt.Errorf("error opening db: %v", err)
		}
		return db, nil
	}

	dbPath := filepath.Join(testDbRoot, dbName)
	db, err := btcdb.OpenDB(dbType, dbPath)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %v", err)
	}

	return db, nil
}

// createDB creates a new db instance and returns a teardown function the caller
// should invoke when done testing to clean up.  The close flag indicates
// whether or not the teardown function should sync and close the database
// during teardown.
func createDB(dbType, dbName string, close bool) (btcdb.Db, func(), error) {
	// Handle memory database specially since it doesn't need the disk
	// specific handling.
	if dbType == "memdb" {
		db, err := btcdb.CreateDB(dbType, "")
		if err != nil {
			return nil, nil, fmt.Errorf("error creating db: %v", err)
		}

		// Setup a teardown function for cleaning up.  This function is
		// returned to the caller to be invoked when it is done testing.
		teardown := func() {
			if close {
				db.Close()
			}
		}

		return db, teardown, nil
	}

	// Create the root directory for test databases.
	if !fileExists(testDbRoot) {
		if err := os.MkdirAll(testDbRoot, 0700); err != nil {
			err := fmt.Errorf("unable to create test db "+
				"root: %v", err)
			return nil, nil, err
		}
	}

	// Create a new database to store the accepted blocks into.
	dbPath := filepath.Join(testDbRoot, dbName)
	_ = os.RemoveAll(dbPath)
	db, err := btcdb.CreateDB(dbType, dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating db: %v", err)
	}

	// Setup a teardown function for cleaning up.  This function is
	// returned to the caller to be invoked when it is done testing.
	teardown := func() {
		dbVersionPath := filepath.Join(testDbRoot, dbName+".ver")
		if close {
			db.Sync()
			db.Close()
		}
		os.RemoveAll(dbPath)
		os.Remove(dbVersionPath)
		os.RemoveAll(testDbRoot)
	}

	return db, teardown, nil
}

// setupDB is used to create a new db instance with the genesis block already
// inserted.  In addition to the new db instance, it returns a teardown function
// the caller should invoke when done testing to clean up.
func setupDB(dbType, dbName string) (btcdb.Db, func(), error) {
	db, teardown, err := createDB(dbType, dbName, true)
	if err != nil {
		return nil, nil, err
	}

	// Insert the main network genesis block.  This is part of the initial
	// database setup.
	genesisBlock := btcutil.NewBlock(&btcwire.GenesisBlock)
	_, err = db.InsertBlock(genesisBlock)
	if err != nil {
		teardown()
		err := fmt.Errorf("failed to insert genesis block: %v", err)
		return nil, nil, err
	}

	return db, teardown, nil
}
