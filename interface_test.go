// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcdb_test

import (
	"github.com/conformal/btcdb"
	"github.com/davecgh/go-spew/spew"
	"reflect"
	"testing"
)

// testFetchBlockShaByHeightErrors ensures FetchBlockShaByHeight handles invalid
// heights correctly.
func testFetchBlockShaByHeightErrors(t *testing.T, dbType string, db btcdb.Db, numBlocks int64) bool {
	tests := []int64{-1, numBlocks, numBlocks + 1}
	for i, wantHeight := range tests {
		hashFromDb, err := db.FetchBlockShaByHeight(wantHeight)
		if err == nil {
			t.Errorf("FetchBlockShaByHeight #%d (%s): did not "+
				"return error on invalid index: %d - got: %v, "+
				"want: non-nil", i, dbType, wantHeight, err)
			return false
		}
		if hashFromDb != nil {
			t.Errorf("FetchBlockShaByHeight #%d (%s): returned "+
				"hash is not nil on invalid index: %d - got: "+
				"%v, want: nil", i, dbType, wantHeight, err)
			return false
		}
	}

	return false
}

// testInterface tests performs tests for the various interfaces of btcdb which
// require state in the database for the given database type.
func testInterface(t *testing.T, dbType string) {
	db, teardown, err := setupDB(dbType, "interface")
	if err != nil {
		t.Errorf("Failed to create test database (%s) %v", dbType, err)
		return
	}
	defer teardown()

	// Load up a bunch of test blocks.
	blocks, err := loadBlocks(t)
	if err != nil {
		t.Errorf("Unable to load blocks from test data %v: %v",
			blockDataFile, err)
		return
	}

	t.Logf("Loaded %d blocks", len(blocks))
	for height := int64(1); height < int64(len(blocks)); height++ {
		block := blocks[height]

		// Ensure there are no errors inserting each block into the
		// database.
		newHeight, err := db.InsertBlock(block)
		if err != nil {
			t.Errorf("InsertBlock (%s): failed to insert block %v "+
				"err %v", dbType, height, err)
			return
		}
		if newHeight != height {
			t.Errorf("InsertBlock (%s): height mismatch got: %v, "+
				"want: %v", dbType, newHeight, height)
			return
		}

		// Ensure the block now exists in the database.
		expectedHash, err := block.Sha()
		if err != nil {
			t.Errorf("block.Sha: %v", err)
			return
		}
		if exists := db.ExistsSha(expectedHash); !exists {
			t.Errorf("ExistsSha (%s): block %v does not exist",
				dbType, expectedHash)
			return
		}

		// Ensure loading the block back from the database gives back
		// the same MsgBlock and raw bytes.
		blockFromDb, err := db.FetchBlockBySha(expectedHash)
		if err != nil {
			t.Errorf("FetchBlockBySha (%s): %v", dbType, err)
			return
		}
		if !reflect.DeepEqual(block.MsgBlock(), blockFromDb.MsgBlock()) {
			t.Errorf("FetchBlockBySha (%s): block from database "+
				"does not match stored block\ngot: %v\n"+
				"want: %v", dbType,
				spew.Sdump(blockFromDb.MsgBlock()),
				spew.Sdump(block.MsgBlock()))
			return
		}
		blockBytes, err := block.Bytes()
		if err != nil {
			t.Errorf("block.Bytes: %v", err)
			return
		}
		blockFromDbBytes, err := blockFromDb.Bytes()
		if err != nil {
			t.Errorf("blockFromDb.Bytes: %v", err)
			return
		}
		if !reflect.DeepEqual(blockBytes, blockFromDbBytes) {
			t.Errorf("FetchBlockBySha (%s): block bytes from "+
				"database do not match stored block bytes\n"+
				"got: %v\nwant: %v", dbType,
				spew.Sdump(blockFromDbBytes),
				spew.Sdump(blockBytes))
			return
		}

		// Ensure the hash returned for the block by its height is the
		// expected value.
		hashFromDb, err := db.FetchBlockShaByHeight(height)
		if err != nil {
			t.Errorf("FetchBlockShaByHeight (%s): %v", dbType, err)
			return
		}
		if !hashFromDb.IsEqual(expectedHash) {
			t.Errorf("FetchBlockShaByHeight (%s): returned hash "+
				"does  not match expected value - got: %v, "+
				"want: %v", dbType, hashFromDb, expectedHash)
			return
		}
	}

	// Ensure FetchBlockShaByHeight handles invalid heights properly.
	if !testFetchBlockShaByHeightErrors(t, dbType, db, int64(len(blocks))) {
		return
	}

	// TODO(davec): Need to figure out how to handle the special checks
	// required for the duplicate transactions allowed by blocks 91842 and
	// 91880 on the main network due to the old miner + Satoshi client bug.

	// TODO(davec): Add tests for the following functions:
	/*
	   Close()
	   DropAfterBlockBySha(*btcwire.ShaHash) (err error)
	   - ExistsSha(sha *btcwire.ShaHash) (exists bool)
	   - FetchBlockBySha(sha *btcwire.ShaHash) (blk *btcutil.Block, err error)
	   - FetchBlockShaByHeight(height int64) (sha *btcwire.ShaHash, err error)
	   FetchHeightRange(startHeight, endHeight int64) (rshalist []btcwire.ShaHash, err error)
	   ExistsTxSha(sha *btcwire.ShaHash) (exists bool)
	   FetchTxBySha(txsha *btcwire.ShaHash) ([]*TxListReply, error)
	   FetchTxByShaList(txShaList []*btcwire.ShaHash) []*TxListReply
	   FetchUnSpentTxByShaList(txShaList []*btcwire.ShaHash) []*TxListReply
	   - InsertBlock(block *btcutil.Block) (height int64, err error)
	   InvalidateBlockCache()
	   InvalidateCache()
	   InvalidateTxCache()
	   NewIterateBlocks() (pbi BlockIterator, err error)
	   NewestSha() (sha *btcwire.ShaHash, height int64, err error)
	   RollbackClose()
	   SetDBInsertMode(InsertMode)
	   Sync()
	*/
}
