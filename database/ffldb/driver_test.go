// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ffldb_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/database/ffldb"
)

// dbType is the database type name for this driver.
const dbType = "ffldb"

// TestCreateOpenFail ensures that errors related to creating and opening a
// database are handled properly.
func TestCreateOpenFail(t *testing.T) {
	t.Parallel()

	// Ensure that attempting to open a database that doesn't exist returns
	// the expected error.
	wantErrCode := database.ErrDbDoesNotExist
	_, err := database.Open(dbType, "noexist", blockDataNet)
	if !checkDbError(t, "Open", err, wantErrCode) {
		return
	}

	// Ensure that attempting to open a database with the wrong number of
	// parameters returns the expected error.
	wantErr := fmt.Errorf("invalid arguments to %s.Open -- expected "+
		"database path and block network", dbType)
	_, err = database.Open(dbType, 1, 2, 3)
	if err.Error() != wantErr.Error() {
		t.Errorf("Open: did not receive expected error - got %v, "+
			"want %v", err, wantErr)
		return
	}

	// Ensure that attempting to open a database with an invalid type for
	// the first parameter returns the expected error.
	wantErr = fmt.Errorf("first argument to %s.Open is invalid -- "+
		"expected database path string", dbType)
	_, err = database.Open(dbType, 1, blockDataNet)
	if err.Error() != wantErr.Error() {
		t.Errorf("Open: did not receive expected error - got %v, "+
			"want %v", err, wantErr)
		return
	}

	// Ensure that attempting to open a database with an invalid type for
	// the second parameter returns the expected error.
	wantErr = fmt.Errorf("second argument to %s.Open is invalid -- "+
		"expected block network", dbType)
	_, err = database.Open(dbType, "noexist", "invalid")
	if err.Error() != wantErr.Error() {
		t.Errorf("Open: did not receive expected error - got %v, "+
			"want %v", err, wantErr)
		return
	}

	// Ensure that attempting to create a database with the wrong number of
	// parameters returns the expected error.
	wantErr = fmt.Errorf("invalid arguments to %s.Create -- expected "+
		"database path and block network", dbType)
	_, err = database.Create(dbType, 1, 2, 3)
	if err.Error() != wantErr.Error() {
		t.Errorf("Create: did not receive expected error - got %v, "+
			"want %v", err, wantErr)
		return
	}

	// Ensure that attempting to create a database with an invalid type for
	// the first parameter returns the expected error.
	wantErr = fmt.Errorf("first argument to %s.Create is invalid -- "+
		"expected database path string", dbType)
	_, err = database.Create(dbType, 1, blockDataNet)
	if err.Error() != wantErr.Error() {
		t.Errorf("Create: did not receive expected error - got %v, "+
			"want %v", err, wantErr)
		return
	}

	// Ensure that attempting to create a database with an invalid type for
	// the second parameter returns the expected error.
	wantErr = fmt.Errorf("second argument to %s.Create is invalid -- "+
		"expected block network", dbType)
	_, err = database.Create(dbType, "noexist", "invalid")
	if err.Error() != wantErr.Error() {
		t.Errorf("Create: did not receive expected error - got %v, "+
			"want %v", err, wantErr)
		return
	}

	// Ensure operations against a closed database return the expected
	// error.
	dbPath := filepath.Join(os.TempDir(), "ffldb-createfail")
	_ = os.RemoveAll(dbPath)
	db, err := database.Create(dbType, dbPath, blockDataNet)
	if err != nil {
		t.Errorf("Create: unexpected error: %v", err)
		return
	}
	defer os.RemoveAll(dbPath)
	db.Close()

	wantErrCode = database.ErrDbNotOpen
	err = db.View(func(tx database.Tx) error {
		return nil
	})
	if !checkDbError(t, "View", err, wantErrCode) {
		return
	}

	wantErrCode = database.ErrDbNotOpen
	err = db.Update(func(tx database.Tx) error {
		return nil
	})
	if !checkDbError(t, "Update", err, wantErrCode) {
		return
	}

	wantErrCode = database.ErrDbNotOpen
	_, err = db.Begin(false)
	if !checkDbError(t, "Begin(false)", err, wantErrCode) {
		return
	}

	wantErrCode = database.ErrDbNotOpen
	_, err = db.Begin(true)
	if !checkDbError(t, "Begin(true)", err, wantErrCode) {
		return
	}

	wantErrCode = database.ErrDbNotOpen
	err = db.Close()
	if !checkDbError(t, "Close", err, wantErrCode) {
		return
	}
}

// TestPersistence ensures that values stored are still valid after closing and
// reopening the database.
func TestPersistence(t *testing.T) {
	t.Parallel()

	// Create a new database to run tests against.
	dbPath := filepath.Join(os.TempDir(), "ffldb-persistencetest")
	_ = os.RemoveAll(dbPath)
	db, err := database.Create(dbType, dbPath, blockDataNet)
	if err != nil {
		t.Errorf("Failed to create test database (%s) %v", dbType, err)
		return
	}
	defer os.RemoveAll(dbPath)
	defer db.Close()

	// Create a bucket, put some values into it, and store a block so they
	// can be tested for existence on re-open.
	bucket1Key := []byte("bucket1")
	storeValues := map[string]string{
		"b1key1": "foo1",
		"b1key2": "foo2",
		"b1key3": "foo3",
	}
	genesisBlock := btcutil.NewBlock(chaincfg.MainNetParams.GenesisBlock)
	genesisHash := chaincfg.MainNetParams.GenesisHash
	err = db.Update(func(tx database.Tx) error {
		metadataBucket := tx.Metadata()
		if metadataBucket == nil {
			return fmt.Errorf("Metadata: unexpected nil bucket")
		}

		bucket1, err := metadataBucket.CreateBucket(bucket1Key)
		if err != nil {
			return fmt.Errorf("CreateBucket: unexpected error: %v",
				err)
		}

		for k, v := range storeValues {
			err := bucket1.Put([]byte(k), []byte(v))
			if err != nil {
				return fmt.Errorf("Put: unexpected error: %v",
					err)
			}
		}

		if err := tx.StoreBlock(genesisBlock); err != nil {
			return fmt.Errorf("StoreBlock: unexpected error: %v",
				err)
		}

		return nil
	})
	if err != nil {
		t.Errorf("Update: unexpected error: %v", err)
		return
	}

	// Close and reopen the database to ensure the values persist.
	db.Close()
	db, err = database.Open(dbType, dbPath, blockDataNet)
	if err != nil {
		t.Errorf("Failed to open test database (%s) %v", dbType, err)
		return
	}
	defer db.Close()

	// Ensure the values previously stored in the 3rd namespace still exist
	// and are correct.
	err = db.View(func(tx database.Tx) error {
		metadataBucket := tx.Metadata()
		if metadataBucket == nil {
			return fmt.Errorf("Metadata: unexpected nil bucket")
		}

		bucket1 := metadataBucket.Bucket(bucket1Key)
		if bucket1 == nil {
			return fmt.Errorf("Bucket1: unexpected nil bucket")
		}

		for k, v := range storeValues {
			gotVal := bucket1.Get([]byte(k))
			if !reflect.DeepEqual(gotVal, []byte(v)) {
				return fmt.Errorf("Get: key '%s' does not "+
					"match expected value - got %s, want %s",
					k, gotVal, v)
			}
		}

		genesisBlockBytes, _ := genesisBlock.Bytes()
		gotBytes, err := tx.FetchBlock(genesisHash)
		if err != nil {
			return fmt.Errorf("FetchBlock: unexpected error: %v",
				err)
		}
		if !reflect.DeepEqual(gotBytes, genesisBlockBytes) {
			return fmt.Errorf("FetchBlock: stored block mismatch")
		}

		return nil
	})
	if err != nil {
		t.Errorf("View: unexpected error: %v", err)
		return
	}
}

// TestPrune tests that the older .fdb files are deleted with a call to prune.
func TestPrune(t *testing.T) {
	t.Parallel()

	// Create a new database to run tests against.
	dbPath := t.TempDir()
	db, err := database.Create(dbType, dbPath, blockDataNet)
	if err != nil {
		t.Errorf("Failed to create test database (%s) %v", dbType, err)
		return
	}
	defer db.Close()

	blockFileSize := uint64(2048)

	testfn := func(t *testing.T, db database.DB) {
		// Load the test blocks and save in the test context for use throughout
		// the tests.
		blocks, err := loadBlocks(t, blockDataFile, blockDataNet)
		if err != nil {
			t.Errorf("loadBlocks: Unexpected error: %v", err)
			return
		}
		err = db.Update(func(tx database.Tx) error {
			for i, block := range blocks {
				err := tx.StoreBlock(block)
				if err != nil {
					return fmt.Errorf("StoreBlock #%d: unexpected error: "+
						"%v", i, err)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}

		blockHashMap := make(map[chainhash.Hash][]byte, len(blocks))
		for _, block := range blocks {
			bytes, err := block.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			blockHashMap[*block.Hash()] = bytes
		}

		err = db.Update(func(tx database.Tx) error {
			_, err := tx.PruneBlocks(1024)
			if err == nil {
				return fmt.Errorf("Expected an error when attempting to prune" +
					"below the maxFileSize")
			}

			_, err = tx.PruneBlocks(0)
			if err == nil {
				return fmt.Errorf("Expected an error when attempting to prune" +
					"below the maxFileSize")
			}

			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		err = db.View(func(tx database.Tx) error {
			pruned, err := tx.BeenPruned()
			if err != nil {
				return err
			}

			if pruned {
				err = fmt.Errorf("The database hasn't been pruned but " +
					"BeenPruned returned true")
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}

		var deletedBlocks []chainhash.Hash

		// This should leave 3 files on disk.
		err = db.Update(func(tx database.Tx) error {
			deletedBlocks, err = tx.PruneBlocks(blockFileSize * 3)
			if err != nil {
				return err
			}

			pruned, err := tx.BeenPruned()
			if err != nil {
				return err
			}

			if pruned {
				err = fmt.Errorf("The database hasn't been committed yet " +
					"but files were already deleted")
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}

		// The only error we can get is a bad pattern error.  Since we're hardcoding
		// the pattern, we should not have an error at runtime.
		files, _ := filepath.Glob(filepath.Join(dbPath, "*.fdb"))
		if len(files) != 3 {
			t.Fatalf("Expected to find %d files but got %d",
				3, len(files))
		}

		err = db.View(func(tx database.Tx) error {
			pruned, err := tx.BeenPruned()
			if err != nil {
				return err
			}

			if !pruned {
				err = fmt.Errorf("The database has been pruned but " +
					"BeenPruned returned false")
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}

		// Check that all the blocks that say were deleted are deleted from the
		// block index bucket as well.
		err = db.View(func(tx database.Tx) error {
			for _, deletedBlock := range deletedBlocks {
				_, err := tx.FetchBlock(&deletedBlock)
				if dbErr, ok := err.(database.Error); !ok ||
					dbErr.ErrorCode != database.ErrBlockNotFound {

					return fmt.Errorf("Expected ErrBlockNotFound "+
						"but got %v", dbErr)
				}
			}

			return nil
		})
		if err != nil {
			t.Fatal(err)
		}

		// Check that the not deleted blocks are present.
		for _, deletedBlock := range deletedBlocks {
			delete(blockHashMap, deletedBlock)
		}
		err = db.View(func(tx database.Tx) error {
			for hash, wantBytes := range blockHashMap {
				gotBytes, err := tx.FetchBlock(&hash)
				if err != nil {
					return err
				}
				if !bytes.Equal(gotBytes, wantBytes) {
					return fmt.Errorf("got bytes %x, want bytes %x",
						gotBytes, wantBytes)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	ffldb.TstRunWithMaxBlockFileSize(db, uint32(blockFileSize), func() {
		testfn(t, db)
	})
}

// TestInterface performs all interfaces tests for this database driver.
func TestInterface(t *testing.T) {
	t.Parallel()

	// Create a new database to run tests against.
	dbPath := filepath.Join(os.TempDir(), "ffldb-interfacetest")
	_ = os.RemoveAll(dbPath)
	db, err := database.Create(dbType, dbPath, blockDataNet)
	if err != nil {
		t.Errorf("Failed to create test database (%s) %v", dbType, err)
		return
	}
	defer os.RemoveAll(dbPath)
	defer db.Close()

	// Ensure the driver type is the expected value.
	gotDbType := db.Type()
	if gotDbType != dbType {
		t.Errorf("Type: unepxected driver type - got %v, want %v",
			gotDbType, dbType)
		return
	}

	// Run all of the interface tests against the database.

	// Change the maximum file size to a small value to force multiple flat
	// files with the test data set.
	ffldb.TstRunWithMaxBlockFileSize(db, 2048, func() {
		testInterface(t, db)
	})
}
