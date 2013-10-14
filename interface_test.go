// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcdb_test

import (
	"github.com/davecgh/go-spew/spew"
	"reflect"
	"testing"
)

// testInterface tests performs tests for the various interfaces of btcdb which
// require state in the database for the given database type.
func testInterface(t *testing.T, dbType string) {
	db, teardown, err := setupDB(dbType, "interface")
	if err != nil {
		t.Errorf("Failed to create test database %v", err)
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
			t.Errorf("InsertBlock: failed to insert block %v err %v",
				height, err)
			return
		}
		if newHeight != height {
			t.Errorf("InsertBlock: height mismatch got: %v, want: %v",
				newHeight, height)
			return
		}

		// Ensure the block now exists in the database.
		expectedHash, err := block.Sha()
		if err != nil {
			t.Errorf("block.Sha: %v", err)
			return
		}
		if exists := db.ExistsSha(expectedHash); !exists {
			t.Errorf("ExistsSha: block %v does not exist",
				expectedHash)
			return
		}

		// Ensure loading the block back from the database gives back
		// the same MsgBlock and raw bytes.
		blockFromDb, err := db.FetchBlockBySha(expectedHash)
		if err != nil {
			t.Errorf("FetchBlockBySha: %v", err)
		}
		if !reflect.DeepEqual(block.MsgBlock(), blockFromDb.MsgBlock()) {
			t.Errorf("FetchBlockBySha: block from database does "+
				"not match stored block\ngot: %v\nwant: %v",
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
			t.Errorf("FetchBlockBySha: block bytes from database "+
				"do not match stored block bytes\ngot: %v\n"+
				"want: %v", spew.Sdump(blockFromDbBytes),
				spew.Sdump(blockBytes))
			return
		}

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
	   FetchBlockShaByHeight(height int64) (sha *btcwire.ShaHash, err error)
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
