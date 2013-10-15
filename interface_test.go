// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcdb_test

import (
	"github.com/conformal/btcdb"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/davecgh/go-spew/spew"
	"reflect"
	"testing"
)

// testContext is used to store context information about a running test which
// is passed into helper functions.  The useSpends field indicates whether or
// not the spend data should be empty or figure it out based on the specific
// test blocks provided.  This is needed because the first loop where the blocks
// are inserted, the tests are running against the latest block and therefore
// none of the outputs can be spent yet.  However, on subsequent runs, all
// blocks have been inserted and therefore some of the transaction outputs are
// spent.
type testContext struct {
	t           *testing.T
	dbType      string
	db          btcdb.Db
	blockHeight int64
	blockHash   *btcwire.ShaHash
	block       *btcutil.Block
	useSpends   bool
}

// testInsertBlock ensures InsertBlock conforms to the interface contract.
func testInsertBlock(tc *testContext) bool {
	// The block must insert without any errors.
	newHeight, err := tc.db.InsertBlock(tc.block)
	if err != nil {
		tc.t.Errorf("InsertBlock (%s): failed to insert block #%d (%s) "+
			"err %v", tc.dbType, tc.blockHeight, tc.blockHash, err)
		return false
	}

	// The returned height must be the expected value.
	if newHeight != tc.blockHeight {
		tc.t.Errorf("InsertBlock (%s): height mismatch got: %v, "+
			"want: %v", tc.dbType, newHeight, tc.blockHeight)
		return false
	}

	return true
}

// testNewestSha ensures the NewestSha returns the values expected by the
// interface contract.
func testNewestSha(tc *testContext) bool {
	// The news block hash and height must be returned without any errors.
	sha, height, err := tc.db.NewestSha()
	if err != nil {
		tc.t.Errorf("NewestSha (%s): block #%d (%s) error %v",
			tc.dbType, tc.blockHeight, tc.blockHash, err)
		return false
	}

	// The returned hash must be the expected value.
	if !sha.IsEqual(tc.blockHash) {
		tc.t.Errorf("NewestSha (%s): block #%d (%s) wrong hash got: %s",
			tc.dbType, tc.blockHeight, tc.blockHash, sha)
		return false

	}

	// The returned height must be the expected value.
	if height != tc.blockHeight {
		tc.t.Errorf("NewestSha (%s): block #%d (%s) wrong height "+
			"got: %d", tc.dbType, tc.blockHeight, tc.blockHash,
			height)
		return false
	}

	return true
}

// testExistsSha ensures ExistsSha conforms to the interface contract.
func testExistsSha(tc *testContext) bool {
	// The block must exist in the database.
	if exists := tc.db.ExistsSha(tc.blockHash); !exists {
		tc.t.Errorf("ExistsSha (%s): block #%d (%s) does not exist",
			tc.dbType, tc.blockHeight, tc.blockHash)
		return false
	}

	return true
}

// testFetchBlockBySha ensures FetchBlockBySha conforms to the interface
// contract.
func testFetchBlockBySha(tc *testContext) bool {
	// The block must be fetchable by its hash without any errors.
	blockFromDb, err := tc.db.FetchBlockBySha(tc.blockHash)
	if err != nil {
		tc.t.Errorf("FetchBlockBySha (%s): block #%d (%s) err: %v",
			tc.dbType, tc.blockHeight, tc.blockHash, err)
		return false
	}

	// The block fetched from the database must give back the same MsgBlock
	// and raw bytes that were stored.
	if !reflect.DeepEqual(tc.block.MsgBlock(), blockFromDb.MsgBlock()) {
		tc.t.Errorf("FetchBlockBySha (%s): block #%d (%s) does not "+
			"match stored block\ngot: %v\nwant: %v", tc.dbType,
			tc.blockHeight, tc.blockHash,
			spew.Sdump(blockFromDb.MsgBlock()),
			spew.Sdump(tc.block.MsgBlock()))
		return false
	}
	blockBytes, err := tc.block.Bytes()
	if err != nil {
		tc.t.Errorf("block.Bytes: %v", err)
		return false
	}
	blockFromDbBytes, err := blockFromDb.Bytes()
	if err != nil {
		tc.t.Errorf("blockFromDb.Bytes: %v", err)
		return false
	}
	if !reflect.DeepEqual(blockBytes, blockFromDbBytes) {
		tc.t.Errorf("FetchBlockBySha (%s): block #%d (%s) bytes do "+
			"not match stored bytes\ngot: %v\nwant: %v", tc.dbType,
			tc.blockHeight, tc.blockHash,
			spew.Sdump(blockFromDbBytes), spew.Sdump(blockBytes))
		return false
	}

	return true
}

// testFetchBlockShaByHeight ensures FetchBlockShaByHeight conforms to the
// interface contract.
func testFetchBlockShaByHeight(tc *testContext) bool {
	// The hash returned for the block by its height must be the expected
	// value.
	hashFromDb, err := tc.db.FetchBlockShaByHeight(tc.blockHeight)
	if err != nil {
		tc.t.Errorf("FetchBlockShaByHeight (%s): block #%d (%s) err: %v",
			tc.dbType, tc.blockHeight, tc.blockHash, err)
		return false
	}
	if !hashFromDb.IsEqual(tc.blockHash) {
		tc.t.Errorf("FetchBlockShaByHeight (%s): block #%d (%s) hash "+
			"does not match expected value - got: %v", tc.dbType,
			tc.blockHeight, tc.blockHash, hashFromDb)
		return false
	}

	return true
}

func testFetchBlockShaByHeightErrors(tc *testContext) bool {
	// Invalid heights must error and return a nil hash.
	tests := []int64{-1, tc.blockHeight + 1, tc.blockHeight + 2}
	for i, wantHeight := range tests {
		hashFromDb, err := tc.db.FetchBlockShaByHeight(wantHeight)
		if err == nil {
			tc.t.Errorf("FetchBlockShaByHeight #%d (%s): did not "+
				"return error on invalid index: %d - got: %v, "+
				"want: non-nil", i, tc.dbType, wantHeight, err)
			return false
		}
		if hashFromDb != nil {
			tc.t.Errorf("FetchBlockShaByHeight #%d (%s): returned "+
				"hash is not nil on invalid index: %d - got: "+
				"%v, want: nil", i, tc.dbType, wantHeight, err)
			return false
		}
	}

	return true
}

// testExistsTxSha ensures ExistsTxSha conforms to the interface contract.
func testExistsTxSha(tc *testContext) bool {
	txHashes, err := tc.block.TxShas()
	if err != nil {
		tc.t.Errorf("block.TxShas: %v", err)
		return false
	}

	for i := range txHashes {
		// The transaction must exist in the database.
		txHash := txHashes[i]
		if exists := tc.db.ExistsTxSha(txHash); !exists {
			tc.t.Errorf("ExistsTxSha (%s): block #%d (%s) "+
				"tx #%d (%s) does not exist", tc.dbType,
				tc.blockHeight, tc.blockHash, i, txHash)
			return false
		}
	}

	return true
}

// testFetchTxBySha ensures FetchTxBySha conforms to the interface contract.
func testFetchTxBySha(tc *testContext) bool {
	txHashes, err := tc.block.TxShas()
	if err != nil {
		tc.t.Errorf("block.TxShas: %v", err)
		return false
	}

	for i, tx := range tc.block.MsgBlock().Transactions {
		txHash := txHashes[i]
		txReplyList, err := tc.db.FetchTxBySha(txHash)
		if err != nil {
			tc.t.Errorf("FetchTxBySha (%s): block #%d (%s) "+
				"tx #%d (%s) err: %v", tc.dbType, tc.blockHeight,
				tc.blockHash, i, txHash, err)
			return false
		}
		if len(txReplyList) == 0 {
			tc.t.Errorf("FetchTxBySha (%s): block #%d (%s) "+
				"tx #%d (%s) did not return reply data",
				tc.dbType, tc.blockHeight, tc.blockHash, i,
				txHash)
			return false
		}
		txFromDb := txReplyList[len(txReplyList)-1].Tx
		if !reflect.DeepEqual(tx, txFromDb) {
			tc.t.Errorf("FetchTxBySha (%s): block #%d (%s) "+
				"tx #%d (%s) does not match stored tx\n"+
				"got: %v\nwant: %v", tc.dbType, tc.blockHeight,
				tc.blockHash, i, txHash, spew.Sdump(txFromDb),
				spew.Sdump(tx))
			return false
		}
	}

	return true
}

// expectedSpentBuf returns the expected transaction spend information depending
// on the block height and and transaction number.  NOTE: These figures are
// only valid for the specific set of test data provided at the time these tests
// were written.  In particular, this means the first 256 blocks of the mainnet
// block chain.
//
// The first run through while the blocks are still being inserted, the tests
// are running against the latest block and therefore none of the outputs can
// be spent yet.  However, on subsequent runs, all blocks have been inserted and
// therefore some of the transaction outputs are spent.
func expectedSpentBuf(tc *testContext, txNum int) []bool {
	numTxOut := len(tc.block.MsgBlock().Transactions[txNum].TxOut)
	spentBuf := make([]bool, numTxOut)
	if tc.useSpends {
		if tc.blockHeight == 9 && txNum == 0 {
			spentBuf[0] = true
		}

		if tc.blockHeight == 170 && txNum == 1 {
			spentBuf[1] = true
		}

		if tc.blockHeight == 181 && txNum == 1 {
			spentBuf[1] = true
		}

		if tc.blockHeight == 182 && txNum == 1 {
			spentBuf[0] = true
			spentBuf[1] = true
		}

		if tc.blockHeight == 183 && txNum == 1 {
			spentBuf[0] = true
			spentBuf[1] = true
		}
	}

	return spentBuf
}

func testFetchTxByShaListCommon(tc *testContext, includeSpent bool) bool {
	fetchFunc := tc.db.FetchUnSpentTxByShaList
	funcName := "FetchUnSpentTxByShaList"
	if includeSpent {
		fetchFunc = tc.db.FetchTxByShaList
		funcName = "FetchTxByShaList"
	}

	txHashes, err := tc.block.TxShas()
	if err != nil {
		tc.t.Errorf("block.TxShas: %v", err)
		return false
	}

	txReplyList := fetchFunc(txHashes)
	if len(txReplyList) != len(txHashes) {
		tc.t.Errorf("%s (%s): block #%d (%s) tx reply list does not "+
			" match expected length - got: %v, want: %v", funcName,
			tc.dbType, tc.blockHeight, tc.blockHash,
			len(txReplyList), len(txHashes))
		return false
	}
	for i, tx := range tc.block.MsgBlock().Transactions {
		txHash := txHashes[i]
		txD := txReplyList[i]

		// The transaction hash in the reply must be the expected value.
		if !txD.Sha.IsEqual(txHash) {
			tc.t.Errorf("%s (%s): block #%d (%s) tx #%d (%s) "+
				"hash does not match expected value - got %v",
				funcName, tc.dbType, tc.blockHeight,
				tc.blockHash, i, txHash, txD.Sha)
			return false
		}

		// The reply must not indicate any errors.
		if txD.Err != nil {
			tc.t.Errorf("%s (%s): block #%d (%s) tx #%d (%s) "+
				"returned unexpected error - got %v, want nil",
				funcName, tc.dbType, tc.blockHeight,
				tc.blockHash, i, txHash, txD.Err)
			return false
		}

		// The transaction in the reply fetched from the database must
		// be the same MsgTx that was stored.
		if !reflect.DeepEqual(tx, txD.Tx) {
			tc.t.Errorf("%s (%s): block #%d (%s) tx #%d (%s) does "+
				"not match stored tx\ngot: %v\nwant: %v",
				funcName, tc.dbType, tc.blockHeight,
				tc.blockHash, i, txHash, spew.Sdump(txD.Tx),
				spew.Sdump(tx))
			return false
		}

		// The block hash in the reply from the database must be the
		// expected value.
		if txD.BlkSha == nil {
			tc.t.Errorf("%s (%s): block #%d (%s) tx #%d (%s) "+
				"returned nil block hash", funcName, tc.dbType,
				tc.blockHeight, tc.blockHash, i, txHash)
			return false
		}
		if !txD.BlkSha.IsEqual(tc.blockHash) {
			tc.t.Errorf("%s (%s): block #%d (%s) tx #%d (%s)"+
				"returned unexpected block hash - got %v",
				funcName, tc.dbType, tc.blockHeight,
				tc.blockHash, i, txHash, txD.BlkSha)
			return false
		}

		// The block height in the reply from the database must be the
		// expected value.
		if txD.Height != tc.blockHeight {
			tc.t.Errorf("%s (%s): block #%d (%s) tx #%d (%s) "+
				"returned unexpected block height - got %v",
				funcName, tc.dbType, tc.blockHeight,
				tc.blockHash, i, txHash, txD.Height)
			return false
		}

		// The spend data in the reply from the database must not
		// indicate any of the transactions that were just inserted are
		// spent.
		if txD.TxSpent == nil {
			tc.t.Errorf("%s (%s): block #%d (%s) tx #%d (%s) "+
				"returned nil spend data", funcName, tc.dbType,
				tc.blockHeight, tc.blockHash, i, txHash)
			return false
		}
		spentBuf := expectedSpentBuf(tc, i)
		if !reflect.DeepEqual(txD.TxSpent, spentBuf) {
			tc.t.Errorf("%s (%s): block #%d (%s) tx #%d (%s) "+
				"returned unexpected spend data - got %v, "+
				"want %v", funcName, tc.dbType, tc.blockHeight,
				tc.blockHash, i, txHash, txD.TxSpent, spentBuf)
			return false
		}
	}

	return true
}

// testFetchTxByShaList ensures FetchTxByShaList conforms to the interface
// contract.
func testFetchTxByShaList(tc *testContext) bool {
	return testFetchTxByShaListCommon(tc, true)
}

// testFetchUnSpentTxByShaList ensures FetchUnSpentTxByShaList conforms to the
// interface contract.
func testFetchUnSpentTxByShaList(tc *testContext) bool {
	return testFetchTxByShaListCommon(tc, false)
}

// testIntegrity performs a series of tests against the interface functions
// which fetch and check for data existence.
func testIntegrity(tc *testContext) bool {
	// The block must now exist in the database.
	if !testExistsSha(tc) {
		return false
	}

	// Loading the block back from the database must give back
	// the same MsgBlock and raw bytes that were stored.
	if !testFetchBlockBySha(tc) {
		return false
	}

	// The hash returned for the block by its height must be the
	// expected value.
	if !testFetchBlockShaByHeight(tc) {
		return false
	}

	// All of the transactions in the block must now exist in the
	// database.
	if !testExistsTxSha(tc) {
		return false
	}

	// Loading all of the transactions in the block back from the
	// database must give back the same MsgTx that was stored.
	if !testFetchTxBySha(tc) {
		return false
	}

	// All of the transactions in the block must be fetchable via
	// FetchTxByShaList and all of the list replies must have the
	// expected values.
	if !testFetchTxByShaList(tc) {
		return false
	}

	// All of the transactions in the block must be fetchable via
	// FetchUnSpentTxByShaList and all of the list replies must have
	// the expected values.
	if !testFetchUnSpentTxByShaList(tc) {
		return false
	}

	return true
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

	// Create a test context to pass around.
	context := testContext{t: t, dbType: dbType, db: db}

	t.Logf("Loaded %d blocks for testing %s", len(blocks), dbType)
	for height := int64(1); height < int64(len(blocks)); height++ {
		// Get the appropriate block and hash and update the test
		// context accordingly.
		block := blocks[height]
		blockHash, err := block.Sha()
		if err != nil {
			t.Errorf("block.Sha: %v", err)
			return
		}
		context.blockHeight = height
		context.blockHash = blockHash
		context.block = block

		// The block must insert without any errors and return the
		// expected height.
		if !testInsertBlock(&context) {
			return
		}

		// The NewestSha function must return the correct information
		// about the block that was just inserted.
		if !testNewestSha(&context) {
			return
		}

		// The block must pass all data integrity tests which involve
		// invoking all and testing the result of all interface
		// functions which deal with fetch and checking for data
		// existence.
		if !testIntegrity(&context) {
			return
		}

		if !testFetchBlockShaByHeightErrors(&context) {
			return
		}
	}

	// The data integrity tests must still pass after calling each of the
	// invalidate cache functions.  This intentionally uses a map since
	// map iteration is not the same order every run.  This helps catch
	// issues that could be caused by calling one version before another.
	context.useSpends = true
	invalidateCacheFuncs := map[string]func(){
		"InvalidateBlockCache": db.InvalidateBlockCache,
		"InvalidateTxCache":    db.InvalidateTxCache,
		"InvalidateCache":      db.InvalidateCache,
	}
	for funcName, invalidateCacheFunc := range invalidateCacheFuncs {
		t.Logf("Running integrity tests after calling %s", funcName)
		invalidateCacheFunc()

		for height := int64(0); height < int64(len(blocks)); height++ {
			// Get the appropriate block and hash and update the
			// test context accordingly.
			block := blocks[height]
			blockHash, err := block.Sha()
			if err != nil {
				t.Errorf("block.Sha: %v", err)
				return
			}
			context.blockHeight = height
			context.blockHash = blockHash
			context.block = block

			testIntegrity(&context)
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
	   - FetchBlockShaByHeight(height int64) (sha *btcwire.ShaHash, err error)
	   FetchHeightRange(startHeight, endHeight int64) (rshalist []btcwire.ShaHash, err error)
	   - ExistsTxSha(sha *btcwire.ShaHash) (exists bool)
	   - FetchTxBySha(txsha *btcwire.ShaHash) ([]*TxListReply, error)
	   - FetchTxByShaList(txShaList []*btcwire.ShaHash) []*TxListReply
	   - FetchUnSpentTxByShaList(txShaList []*btcwire.ShaHash) []*TxListReply
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
