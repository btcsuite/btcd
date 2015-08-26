// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database_test

import (
	"bytes"
	"compress/bzip2"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ldb"
	_ "github.com/decred/dcrd/database/memdb"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

var (
	// network is the expected decred network in the test block data.
	network = wire.SimNet

	// savedBlocks is used to store blocks loaded from the blockDataFile
	// so multiple invocations to loadBlocks from the various test functions
	// do not have to reload them from disk.
	savedBlocks []*dcrutil.Block

	// blockDataFile is the path to a file containing the first 168 blocks
	// of a simulated blockchain designed to abuse network rules.
	blockDataFile = filepath.Join("../blockchain/testdata", "blocks0to168.bz2")
)

var zeroHash = chainhash.Hash{}

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
func openDB(dbType, dbName string) (database.Db, error) {
	// Handle memdb specially since it has no files on disk.
	if dbType == "memdb" {
		db, err := database.OpenDB(dbType)
		if err != nil {
			return nil, fmt.Errorf("error opening db: %v", err)
		}
		return db, nil
	}

	dbPath := filepath.Join(testDbRoot, dbName)
	db, err := database.OpenDB(dbType, dbPath)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %v", err)
	}

	return db, nil
}

// createDB creates a new db instance and returns a teardown function the caller
// should invoke when done testing to clean up.  The close flag indicates
// whether or not the teardown function should sync and close the database
// during teardown.
func createDB(dbType, dbName string, close bool) (database.Db, func(), error) {
	// Handle memory database specially since it doesn't need the disk
	// specific handling.
	if dbType == "memdb" {
		db, err := database.CreateDB(dbType)
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
	db, err := database.CreateDB(dbType, dbPath)
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
func setupDB(dbType, dbName string) (database.Db, func(), error) {
	db, teardown, err := createDB(dbType, dbName, true)
	if err != nil {
		return nil, nil, err
	}
	return db, teardown, nil
}

// loadBlocks loads the blocks contained in the testdata directory and returns
// a slice of them.
func loadBlocks(t *testing.T) ([]*dcrutil.Block, error) {
	if len(savedBlocks) != 0 {
		return savedBlocks, nil
	}

	fi, err := os.Open(blockDataFile)
	if err != nil {
		t.Errorf("failed to open file %v, err %v", blockDataFile, err)
		return nil, err
	}
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder := gob.NewDecoder(bcBuf)
	blockchain := make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&blockchain); err != nil {
		t.Errorf("error decoding test blockchain")
	}

	blocks := make([]*dcrutil.Block, 0, len(blockchain))
	for height := int64(1); height < int64(len(blockchain)); height++ {
		block, err := dcrutil.NewBlockFromBytes(blockchain[height])
		if err != nil {
			t.Errorf("failed to parse block %v", height)
			return nil, err
		}
		block.SetHeight(height - 1)
		blocks = append(blocks, block)
	}

	savedBlocks = blocks
	return blocks, nil
}
