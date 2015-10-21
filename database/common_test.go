// Copyright (c) 2013-2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database_test

import (
	"compress/bzip2"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ldb"
	_ "github.com/btcsuite/btcd/database/memdb"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

var (
	// network is the expected bitcoin network in the test block data.
	network = wire.MainNet

	// savedBlocks is used to store blocks loaded from the blockDataFile
	// so multiple invocations to loadBlocks from the various test functions
	// do not have to reload them from disk.
	savedBlocks []*btcutil.Block

	// blockDataFile is the path to a file containing the first 256 blocks
	// of the block chain.
	blockDataFile = filepath.Join("testdata", "blocks1-256.bz2")
)

var zeroHash = wire.ShaHash{}

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

	// Insert the main network genesis block.  This is part of the initial
	// database setup.
	genesisBlock := btcutil.NewBlock(chaincfg.MainNetParams.GenesisBlock)
	_, err = db.InsertBlock(genesisBlock)
	if err != nil {
		teardown()
		err := fmt.Errorf("failed to insert genesis block: %v", err)
		return nil, nil, err
	}

	return db, teardown, nil
}

// loadBlocks loads the blocks contained in the testdata directory and returns
// a slice of them.
func loadBlocks(t *testing.T) ([]*btcutil.Block, error) {
	if len(savedBlocks) != 0 {
		return savedBlocks, nil
	}

	var dr io.Reader
	fi, err := os.Open(blockDataFile)
	if err != nil {
		t.Errorf("failed to open file %v, err %v", blockDataFile, err)
		return nil, err
	}
	if strings.HasSuffix(blockDataFile, ".bz2") {
		z := bzip2.NewReader(fi)
		dr = z
	} else {
		dr = fi
	}

	defer func() {
		if err := fi.Close(); err != nil {
			t.Errorf("failed to close file %v %v", blockDataFile, err)
		}
	}()

	// Set the first block as the genesis block.
	blocks := make([]*btcutil.Block, 0, 256)
	genesis := btcutil.NewBlock(chaincfg.MainNetParams.GenesisBlock)
	blocks = append(blocks, genesis)

	for height := int64(1); err == nil; height++ {
		var rintbuf uint32
		err := binary.Read(dr, binary.LittleEndian, &rintbuf)
		if err == io.EOF {
			// hit end of file at expected offset: no warning
			height--
			err = nil
			break
		}
		if err != nil {
			t.Errorf("failed to load network type, err %v", err)
			break
		}
		if rintbuf != uint32(network) {
			t.Errorf("Block doesn't match network: %v expects %v",
				rintbuf, network)
			break
		}
		err = binary.Read(dr, binary.LittleEndian, &rintbuf)
		blocklen := rintbuf

		rbytes := make([]byte, blocklen)

		// read block
		dr.Read(rbytes)

		block, err := btcutil.NewBlockFromBytes(rbytes)
		if err != nil {
			t.Errorf("failed to parse block %v", height)
			return nil, err
		}
		blocks = append(blocks, block)
	}

	savedBlocks = blocks
	return blocks, nil
}
