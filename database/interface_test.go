// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database_test

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
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
	db          database.Db
	blockHeight int64
	blockHash   *chainhash.Hash
	block       *dcrutil.Block
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
	exists, err := tc.db.ExistsSha(tc.blockHash)
	if err != nil {
		tc.t.Errorf("ExistsSha (%s): block #%d (%s) unexpected error: "+
			"%v", tc.dbType, tc.blockHeight, tc.blockHash, err)
		return false
	}
	if !exists {
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

// testFetchBlockHeightBySha ensures FetchBlockHeightBySha conforms to the
// interface contract.
func testFetchBlockHeightBySha(tc *testContext) bool {
	// The block height must be fetchable by its hash without any errors.
	blockHeight, err := tc.db.FetchBlockHeightBySha(tc.blockHash)
	if err != nil {
		tc.t.Errorf("FetchBlockHeightBySha (%s): block #%d (%s) err: %v",
			tc.dbType, tc.blockHeight, tc.blockHash, err)
		return false
	}

	// The block height fetched from the database must match the expected
	// height.
	if blockHeight != tc.blockHeight {
		tc.t.Errorf("FetchBlockHeightBySha (%s): block #%d (%s) height "+
			"does not match expected value - got: %v", tc.dbType,
			tc.blockHeight, tc.blockHash, blockHeight)
		return false
	}

	return true
}

// testFetchBlockHeaderBySha ensures FetchBlockHeaderBySha conforms to the
// interface contract.
func testFetchBlockHeaderBySha(tc *testContext) bool {
	// The block header must be fetchable by its hash without any errors.
	blockHeader, err := tc.db.FetchBlockHeaderBySha(tc.blockHash)
	if err != nil {
		tc.t.Errorf("FetchBlockHeaderBySha (%s): block #%d (%s) err: %v",
			tc.dbType, tc.blockHeight, tc.blockHash, err)
		return false
	}

	// The block header fetched from the database must give back the same
	// BlockHeader that was stored.
	if !reflect.DeepEqual(&tc.block.MsgBlock().Header, blockHeader) {
		tc.t.Errorf("FetchBlockHeaderBySha (%s): block header #%d (%s) "+
			" does not match stored block\ngot: %v\nwant: %v",
			tc.dbType, tc.blockHeight, tc.blockHash,
			spew.Sdump(blockHeader),
			spew.Sdump(&tc.block.MsgBlock().Header))
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
	var blockPrev *dcrutil.Block = nil
	// Decred: WARNING. This function assumes that all block insertion calls have
	// dcrutil.blocks passed to them with block.blockHeight set correctly. However,
	// loading the genesis block in dcrd didn't do this (via block manager); pre-
	// production it should be established that all calls to this function pass
	// blocks with block.blockHeight set correctly.
	if tc.block.Height() != 0 {
		var errBlockPrev error
		blockPrev, errBlockPrev = tc.db.FetchBlockBySha(&tc.block.MsgBlock().Header.PrevBlock)
		if errBlockPrev != nil {
			blockSha := tc.block.Sha()
			tc.t.Errorf("Failed to fetch parent block of block %v", blockSha)
		}
	}

	votebits := tc.block.MsgBlock().Header.VoteBits
	if dcrutil.IsFlagSet16(votebits, dcrutil.BlockValid) && blockPrev != nil {
		for i, tx := range blockPrev.Transactions() {
			// The transaction must exist in the database.
			txHash := tx.Sha()
			exists, err := tc.db.ExistsTxSha(txHash)
			if err != nil {
				tc.t.Errorf("ExistsTxSha (%s): block #%d (%s) tx #%d "+
					"(%s) unexpected error: %v", tc.dbType,
					tc.blockHeight, tc.blockHash, i, txHash, err)
				return false
			}
			if !exists {
				_, err := tc.db.FetchTxBySha(txHash)
				if err != nil {
					tc.t.Errorf("ExistsTxSha (%s): block #%d (%s) "+
						"tx #%d (%s) does not exist", tc.dbType,
						tc.blockHeight, tc.blockHash, i, txHash)
				}
				return false
			}
		}
	}
	return true
}

// testFetchTxBySha ensures FetchTxBySha conforms to the interface contract.
func testFetchTxBySha(tc *testContext) bool {
	var blockPrev *dcrutil.Block = nil
	if tc.block.Height() != 0 {
		var errBlockPrev error
		blockPrev, errBlockPrev = tc.db.FetchBlockBySha(&tc.block.MsgBlock().Header.PrevBlock)
		if errBlockPrev != nil {
			blockSha := tc.block.Sha()
			tc.t.Errorf("Failed to fetch parent block of block %v", blockSha)
		}
	}

	votebits := tc.block.MsgBlock().Header.VoteBits
	if dcrutil.IsFlagSet16(votebits, dcrutil.BlockValid) && blockPrev != nil {
		for i, tx := range blockPrev.Transactions() {
			txHash := tx.Sha()
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
			if !reflect.DeepEqual(tx.MsgTx(), txFromDb) {
				tc.t.Errorf("FetchTxBySha (%s): block #%d (%s) "+
					"tx #%d (%s, %s) does not match stored tx\n"+
					"got: %v\nwant: %v", tc.dbType, tc.blockHeight,
					tc.blockHash, i, txHash, txFromDb.TxSha(), spew.Sdump(txFromDb),
					spew.Sdump(tx.MsgTx()))
				return false
			}
		}
	}
	for i, tx := range tc.block.MsgBlock().STransactions {
		txHash := tx.TxSha()
		txReplyList, err := tc.db.FetchTxBySha(&txHash)
		if err != nil {
			tc.t.Errorf("FetchTxBySha (%s): block #%d (%s) "+
				"sstx #%d (%s) err: %v", tc.dbType, tc.blockHeight,
				tc.blockHash, i, txHash, err)
			return false
		}
		if len(txReplyList) == 0 {
			tc.t.Errorf("FetchTxBySha (%s): block #%d (%s) "+
				"sstx #%d (%s) did not return reply data",
				tc.dbType, tc.blockHeight, tc.blockHash, i,
				txHash)
			return false
		}
		txFromDb := txReplyList[len(txReplyList)-1].Tx
		if !reflect.DeepEqual(tx, txFromDb) {
			tc.t.Errorf("FetchTxBySha (%s): block #%d (%s) "+
				"sstx #%d (%s) does not match stored sstx\n"+
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
	var blah = []bool{false}
	var blockPrev *dcrutil.Block = nil
	if tc.block.Height() != 0 {
		var errBlockPrev error
		blockPrev, errBlockPrev = tc.db.FetchBlockBySha(&tc.block.MsgBlock().Header.PrevBlock)
		if errBlockPrev != nil {
			blockSha := tc.block.Sha()
			tc.t.Errorf("Failed to fetch parent block of block %v", blockSha)
			return blah
		}
	}
	transactions := blockPrev.Transactions()
	numTxOut := len(transactions[txNum].MsgTx().TxOut)
	spentBuf := make([]bool, numTxOut)
	if tc.useSpends {
		if tc.blockHeight >= 2 && tc.blockHeight <= 43 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 45 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 46 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 48 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 49 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 54 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 55 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 57 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 59 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 63 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 67 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 68 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 69 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 70 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 73 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 74 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 76 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 77 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight >= 105 && tc.blockHeight <= 120 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 122 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 125 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 127 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 131 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 132 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 134 && txNum == 0 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
		}
		if tc.blockHeight == 44 && txNum == 1 {
			spentBuf[0] = true
			spentBuf[1] = true
			spentBuf[2] = true
			spentBuf[3] = true
			spentBuf[4] = false
			spentBuf[5] = true
		}
		if tc.blockHeight == 60 && txNum == 1 {
			spentBuf[0] = false
			spentBuf[1] = true
			spentBuf[2] = false
			spentBuf[3] = false
			spentBuf[4] = false
			spentBuf[5] = false
		}
		if tc.blockHeight == 75 && txNum == 1 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = false
			spentBuf[3] = false
			spentBuf[4] = false
			spentBuf[5] = true
		}
		if tc.blockHeight == 78 && txNum == 2 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = false
			spentBuf[3] = false
			spentBuf[4] = false
			spentBuf[5] = true
		}
		if tc.blockHeight == 79 && txNum == 1 {
			spentBuf[0] = false
			spentBuf[1] = true
			spentBuf[2] = false
			spentBuf[3] = false
			spentBuf[4] = false
			spentBuf[5] = false
		}
		if tc.blockHeight == 89 && txNum == 1 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
			spentBuf[3] = false
			spentBuf[4] = false
			spentBuf[5] = false
		}
		if tc.blockHeight == 90 && txNum == 1 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = false
			spentBuf[3] = false
			spentBuf[4] = false
			spentBuf[5] = true
		}
		if tc.blockHeight == 93 && txNum == 1 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = false
			spentBuf[3] = false
			spentBuf[4] = false
			spentBuf[5] = true
		}
		if tc.blockHeight == 95 && txNum == 1 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
			spentBuf[3] = false
			spentBuf[4] = false
			spentBuf[5] = false
		}
		if tc.blockHeight == 97 && txNum == 3 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = false
			spentBuf[3] = true
			spentBuf[4] = false
			spentBuf[5] = false
		}
		if tc.blockHeight == 99 && txNum == 1 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = false
			spentBuf[3] = true
			spentBuf[4] = false
			spentBuf[5] = false
		}
		if tc.blockHeight == 101 && txNum == 1 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = false
			spentBuf[3] = false
			spentBuf[4] = false
			spentBuf[5] = true
		}
		if tc.blockHeight == 103 && txNum == 1 {
			spentBuf[0] = true
			spentBuf[1] = false
			spentBuf[2] = false
			spentBuf[3] = false
			spentBuf[4] = false
			spentBuf[5] = false
		}
		if tc.blockHeight == 106 && txNum == 1 {
			spentBuf[0] = true
			spentBuf[1] = true
			spentBuf[2] = true
			spentBuf[3] = true
			spentBuf[4] = true
			spentBuf[5] = false
		}
		if tc.blockHeight == 111 && txNum == 1 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = false
			spentBuf[3] = false
			spentBuf[4] = false
			spentBuf[5] = true
		}
		if tc.blockHeight == 113 && txNum == 1 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = false
			spentBuf[3] = false
			spentBuf[4] = true
			spentBuf[5] = false
		}
		if tc.blockHeight == 113 && txNum == 2 {
			spentBuf[0] = true
			spentBuf[1] = false
			spentBuf[2] = false
			spentBuf[3] = false
			spentBuf[4] = false
			spentBuf[5] = true
		}
		if tc.blockHeight == 117 && txNum == 1 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
			spentBuf[3] = false
			spentBuf[4] = false
			spentBuf[5] = false
		}
		if tc.blockHeight == 122 && txNum == 1 {
			spentBuf[0] = true
			spentBuf[1] = true
			spentBuf[2] = true
			spentBuf[3] = false
			spentBuf[4] = true
			spentBuf[5] = true
		}
		if tc.blockHeight == 131 && txNum == 1 {
			spentBuf[0] = true
			spentBuf[1] = false
			spentBuf[2] = true
			spentBuf[3] = true
			spentBuf[4] = true
			spentBuf[5] = true
		}
		if tc.blockHeight == 135 && txNum == 1 {
			spentBuf[0] = true
			spentBuf[1] = true
			spentBuf[2] = true
			spentBuf[3] = true
			spentBuf[4] = true
			spentBuf[5] = false
		}
		if tc.blockHeight == 141 && txNum == 1 {
			spentBuf[0] = true
			spentBuf[1] = true
			spentBuf[2] = true
			spentBuf[3] = true
			spentBuf[4] = false
			spentBuf[5] = true
		}
		if tc.blockHeight == 142 && txNum == 1 {
			spentBuf[0] = true
			spentBuf[1] = true
			spentBuf[2] = false
			spentBuf[3] = true
			spentBuf[4] = true
			spentBuf[5] = true
		}
		if tc.blockHeight == 145 && txNum == 1 {
			spentBuf[0] = true
			spentBuf[1] = false
			spentBuf[2] = true
			spentBuf[3] = true
			spentBuf[4] = true
			spentBuf[5] = true
		}
		if tc.blockHeight == 146 && txNum == 1 {
			spentBuf[0] = true
			spentBuf[1] = false
			spentBuf[2] = false
			spentBuf[3] = true
			spentBuf[4] = false
			spentBuf[5] = true
		}
		if tc.blockHeight == 146 && txNum == 2 {
			spentBuf[0] = true
			spentBuf[1] = true
			spentBuf[2] = true
			spentBuf[3] = true
			spentBuf[4] = false
			spentBuf[5] = true
		}
		if tc.blockHeight == 147 && txNum == 1 {
			spentBuf[0] = false
			spentBuf[1] = false
			spentBuf[2] = true
			spentBuf[3] = false
			spentBuf[4] = true
			spentBuf[5] = false
		}
	}

	return spentBuf
}

// unspendStakeTxTree returns all outpoints spent before this one
// in the block's tx tree stake. used for unspending the stake tx
// tree to evaluate tx tree regular of prev block.
func unspendStakeTxTree(block *dcrutil.Block) map[wire.OutPoint]struct{} {
	unspentOps := make(map[wire.OutPoint]struct{})

	for _, tx := range block.STransactions() {
		for _, txIn := range tx.MsgTx().TxIn {
			unspentOps[txIn.PreviousOutPoint] = struct{}{}
		}
	}

	return unspentOps
}

func testFetchTxByShaListCommon(tc *testContext, includeSpent bool) bool {
	var blockPrev *dcrutil.Block = nil
	if tc.block.Height() != 0 {
		var errBlockPrev error
		blockPrev, errBlockPrev = tc.db.FetchBlockBySha(&tc.block.MsgBlock().Header.PrevBlock)
		if errBlockPrev != nil {
			blockSha := tc.block.Sha()
			tc.t.Errorf("Failed to fetch parent block of block %v", blockSha)
		}
	}

	unspentFromTxTreeStake := unspendStakeTxTree(tc.block)

	votebits := tc.block.MsgBlock().Header.VoteBits
	if dcrutil.IsFlagSet16(votebits, dcrutil.BlockValid) && blockPrev != nil {
		fetchFunc := tc.db.FetchUnSpentTxByShaList
		funcName := "FetchUnSpentTxByShaList"
		if includeSpent {
			fetchFunc = tc.db.FetchTxByShaList
			funcName = "FetchTxByShaList"
		}

		transactions := blockPrev.Transactions()
		txHashes := make([]*chainhash.Hash, len(transactions))
		for i, tx := range transactions {
			txHashes[i] = tx.Sha()
		}

		txReplyList := fetchFunc(txHashes)
		if len(txReplyList) != len(txHashes) {
			tc.t.Errorf("%s (%s): block #%d (%s) tx reply list does not "+
				" match expected length - got: %v, want: %v", funcName,
				tc.dbType, tc.blockHeight, tc.blockHash,
				len(txReplyList), len(txHashes))
			return false
		}
		for i, tx := range transactions {
			txHash := tx.Sha()
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
			if !reflect.DeepEqual(tx.MsgTx(), txD.Tx) {
				tc.t.Errorf("%s (%s): block #%d (%s) tx #%d (%s) does "+
					"not match stored tx\ngot: %v\nwant: %v",
					funcName, tc.dbType, tc.blockHeight,
					tc.blockHash, i, txHash, spew.Sdump(txD.Tx),
					spew.Sdump(tx.MsgTx()))
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
			if !txD.BlkSha.IsEqual(&tc.block.MsgBlock().Header.PrevBlock) {
				tc.t.Errorf("%s (%s): block #%d (%s) tx #%d (%s)"+
					"returned unexpected block hash - got %v",
					funcName, tc.dbType, tc.blockHeight,
					tc.blockHash, i, txHash, txD.BlkSha)
				return false
			}

			// The block height in the reply from the database must be the
			// expected value.
			if txD.Height != tc.blockHeight-1 {
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
				stakeInChecksDontPass := false
				for txoIdx := range spentBuf {
					if txD.TxSpent[txoIdx] != spentBuf[txoIdx] {
						op := wire.OutPoint{
							Hash:  *txHash,
							Index: uint32(txoIdx),
							Tree:  dcrutil.TxTreeRegular,
						}

						if _, unspent := unspentFromTxTreeStake[op]; !unspent {
							stakeInChecksDontPass = true
						}
					}
				}

				if stakeInChecksDontPass {
					tc.t.Errorf("%s (%s): block #%d (%s) tx #%d (%s) "+
						"returned unexpected spend data - got %v, "+
						"want %v", funcName, tc.dbType, tc.blockHeight,
						tc.blockHash, i, txHash, txD.TxSpent, spentBuf)
					return false
				}
			}
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

	// The height returned for the block given its hash must be the
	// expected value
	if !testFetchBlockHeightBySha(tc) {
		return false
	}

	// Loading the header back from the database must give back
	// the same BlockHeader that was stored.
	if !testFetchBlockHeaderBySha(tc) {
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

// testInterface tests performs tests for the various interfaces of the database
// package which require state in the database for the given database type.
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
	for height := int64(0); height < int64(len(blocks)); height++ {
		// Get the appropriate block and hash and update the test
		// context accordingly.
		block := blocks[height]
		context.blockHeight = height
		context.blockHash = block.Sha()
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

	// Run the data integrity tests again after all blocks have been
	// inserted to ensure the spend tracking  is working properly.
	context.useSpends = true
	for height := int64(0); height < int64(len(blocks)); height++ {
		// Get the appropriate block and hash and update the
		// test context accordingly.
		block := blocks[height]
		context.blockHeight = height
		context.blockHash = block.Sha()
		context.block = block

		testIntegrity(&context)
	}

	// TODO(davec): Need to figure out how to handle the special checks
	// required for the duplicate transactions allowed by blocks 91842 and
	// 91880 on the main network due to the old miner + Satoshi client bug.

	// TODO(davec): Add tests for error conditions:
	/*
	   - Don't allow duplicate blocks
	   - Don't allow insertion of block that contains a transaction that
	     already exists unless the previous one is fully spent
	   - Don't allow block that has a duplicate transaction in itself
	   - Don't allow block which contains a tx that references a missing tx
	   - Don't allow block which contains a tx that references another tx
	     that comes after it in the same block
	*/

	// TODO(davec): Add tests for the following functions:
	/*
	   - Close()
	   - DropAfterBlockBySha(*wire.ShaHash) (err error)
	   x ExistsSha(sha *wire.ShaHash) (exists bool)
	   x FetchBlockBySha(sha *wire.ShaHash) (blk *dcrutil.Block, err error)
	   x FetchBlockShaByHeight(height int64) (sha *wire.ShaHash, err error)
	   - FetchHeightRange(startHeight, endHeight int64) (rshalist []wire.ShaHash, err error)
	   x ExistsTxSha(sha *wire.ShaHash) (exists bool)
	   x FetchTxBySha(txsha *wire.ShaHash) ([]*TxListReply, error)
	   x FetchTxByShaList(txShaList []*wire.ShaHash) []*TxListReply
	   x FetchUnSpentTxByShaList(txShaList []*wire.ShaHash) []*TxListReply
	   x InsertBlock(block *dcrutil.Block) (height int64, err error)
	   x NewestSha() (sha *wire.ShaHash, height int64, err error)
	   - RollbackClose()
	   - Sync()
	*/
}
