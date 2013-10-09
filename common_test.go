// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcchain_test

import (
	"fmt"
	"github.com/conformal/btcchain"
	"github.com/conformal/btcdb"
	_ "github.com/conformal/btcdb/ldb"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"os"
	"path/filepath"
)

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

// chainSetup is used to create a new db and chain instance with the genesis
// block already inserted.  In addition to the new chain instnce, it returns
// a teardown function the caller should invoke when done testing to clean up.
func chainSetup(dbName string) (*btcchain.BlockChain, func(), error) {
	// Create the root directory for test databases.
	if !fileExists(testDbRoot) {
		if err := os.MkdirAll(testDbRoot, 0700); err != nil {
			err := fmt.Errorf("unable to create test db root: %v", err)
			return nil, nil, err
		}
	}

	// Create a new database to store the accepted blocks into.
	dbPath := filepath.Join(testDbRoot, dbName)
	_ = os.RemoveAll(dbPath)
	db, err := btcdb.CreateDB("leveldb", dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating db: %v", err)
	}

	// Setup a teardown function for cleaning up.  This function is returned
	// to the caller to be invoked when it is done testing.
	teardown := func() {
		dbVersionPath := filepath.Join(testDbRoot, dbName+".ver")
		db.Sync()
		db.Close()
		os.RemoveAll(dbPath)
		os.Remove(dbVersionPath)
		os.RemoveAll(testDbRoot)
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

	chain := btcchain.New(db, btcwire.MainNet, nil)
	return chain, teardown, nil
}
