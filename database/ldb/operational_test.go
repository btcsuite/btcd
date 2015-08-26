// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb_test

import (
	"bytes"
	"compress/bzip2"
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"

	"github.com/btcsuite/golangcrypto/ripemd160"
	//	"github.com/davecgh/go-spew/spew"
)

var network = wire.MainNet

// testDb is used to store db related context for a running test.
// the `cleanUpFunc` *must* be called after each test to maintain db
// consistency across tests.
type testDb struct {
	db          database.Db
	blocks      []*dcrutil.Block
	dbName      string
	dbNameVer   string
	cleanUpFunc func()
}

func setUpTestDb(t *testing.T, dbname string) (*testDb, error) {
	// Ignore db remove errors since it means we didn't have an old one.
	dbnamever := dbname + ".ver"
	_ = os.RemoveAll(dbname)
	_ = os.RemoveAll(dbnamever)
	db, err := database.CreateDB("leveldb", dbname)
	if err != nil {
		return nil, err
	}

	testdatafile := filepath.Join("..", "/../blockchain/testdata", "blocks0to168.bz2")
	blocks, err := loadBlocks(t, testdatafile)
	if err != nil {
		return nil, err
	}

	cleanUp := func() {
		db.Close()
		os.RemoveAll(dbname)
		os.RemoveAll(dbnamever)
	}

	return &testDb{
		db:          db,
		blocks:      blocks,
		dbName:      dbname,
		dbNameVer:   dbnamever,
		cleanUpFunc: cleanUp,
	}, nil
}

func TestOperational(t *testing.T) {
	testOperationalMode(t)
}

// testAddrIndexOperations ensures that all normal operations concerning
// the optional address index function correctly.
func testAddrIndexOperations(t *testing.T, db database.Db, newestBlock *dcrutil.Block, newestSha *chainhash.Hash, newestBlockIdx int64) {
	// Metadata about the current addr index state should be unset.
	sha, height, err := db.FetchAddrIndexTip()
	if err != database.ErrAddrIndexDoesNotExist {
		t.Fatalf("Address index metadata shouldn't be in db, hasn't been built up yet.")
	}

	var zeroHash chainhash.Hash
	if !sha.IsEqual(&zeroHash) {
		t.Fatalf("AddrIndexTip wrong hash got: %s, want %s", sha, &zeroHash)

	}

	if height != -1 {
		t.Fatalf("Addrindex not built up, yet a block index tip has been set to: %d.", height)
	}

	// Test enforcement of constraints for "limit" and "skip"
	var fakeAddr dcrutil.Address
	_, _, err = db.FetchTxsForAddr(fakeAddr, -1, 0, false)
	if err == nil {
		t.Fatalf("Negative value for skip passed, should return an error")
	}

	_, _, err = db.FetchTxsForAddr(fakeAddr, 0, -1, false)
	if err == nil {
		t.Fatalf("Negative value for limit passed, should return an error")
	}

	// Simple test to index outputs(s) of the first tx.
	testIndex := make(database.BlockAddrIndex, database.AddrIndexKeySize)
	testTx, err := newestBlock.Tx(0)
	if err != nil {
		t.Fatalf("Block has no transactions, unable to test addr "+
			"indexing, err %v", err)
	}

	// Extract the dest addr from the tx.
	_, testAddrs, _, err := txscript.ExtractPkScriptAddrs(testTx.MsgTx().TxOut[0].Version, testTx.MsgTx().TxOut[0].PkScript, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Unable to decode tx output, err %v", err)
	}

	// Extract the hash160 from the output script.
	var hash160Bytes [ripemd160.Size]byte
	testHash160 := testAddrs[0].(*dcrutil.AddressScriptHash).Hash160()
	copy(hash160Bytes[:], testHash160[:])

	// Create a fake index.
	blktxLoc, _, _ := newestBlock.TxLoc()
	testIndex = []*database.TxAddrIndex{
		{
			Hash160:  hash160Bytes,
			Height:   uint32(newestBlockIdx),
			TxOffset: uint32(blktxLoc[0].TxStart),
			TxLen:    uint32(blktxLoc[0].TxLen),
		},
	}

	// Insert our test addr index into the DB.
	err = db.UpdateAddrIndexForBlock(newestSha, newestBlockIdx, testIndex)
	if err != nil {
		t.Fatalf("UpdateAddrIndexForBlock: failed to index"+
			" addrs for block #%d (%s) "+
			"err %v", newestBlockIdx, newestSha, err)
	}

	// Chain Tip of address should've been updated.
	assertAddrIndexTipIsUpdated(db, t, newestSha, newestBlockIdx)

	// Check index retrieval.
	txReplies, _, err := db.FetchTxsForAddr(testAddrs[0], 0, 1000, false)
	if err != nil {
		t.Fatalf("FetchTxsForAddr failed to correctly fetch txs for an "+
			"address, err %v", err)
	}
	// Should have one reply.
	if len(txReplies) != 1 {
		t.Fatalf("Failed to properly index tx by address.")
	}

	// Our test tx and indexed tx should have the same sha.
	indexedTx := txReplies[0]
	if !bytes.Equal(indexedTx.Sha.Bytes(), testTx.Sha().Bytes()) {
		t.Fatalf("Failed to fetch proper indexed tx. Expected sha %v, "+
			"fetched %v", testTx.Sha(), indexedTx.Sha)
	}

	// Shut down DB.
	db.Sync()
	db.Close()

	// Re-Open, tip still should be updated to current height and sha.
	db, err = database.OpenDB("leveldb", "tstdbopmode")
	if err != nil {
		t.Fatalf("Unable to re-open created db, err %v", err)
	}
	assertAddrIndexTipIsUpdated(db, t, newestSha, newestBlockIdx)

	// Delete the entire index.
	err = db.PurgeAddrIndex()
	if err != nil {
		t.Fatalf("Couldn't delete address index, err %v", err)
	}

	// Former index should no longer exist.
	txReplies, _, err = db.FetchTxsForAddr(testAddrs[0], 0, 1000, false)
	if err != nil {
		t.Fatalf("Unable to fetch transactions for address: %v", err)
	}
	if len(txReplies) != 0 {
		t.Fatalf("Address index was not successfully deleted. "+
			"Should have 0 tx's indexed, %v were returned.",
			len(txReplies))
	}

	// Tip should be blanked out.
	if _, _, err := db.FetchAddrIndexTip(); err != database.ErrAddrIndexDoesNotExist {
		t.Fatalf("Address index was not fully deleted.")
	}

}

func assertAddrIndexTipIsUpdated(db database.Db, t *testing.T, newestSha *chainhash.Hash, newestBlockIdx int64) {
	// Safe to ignore error, since height will be < 0 in "error" case.
	sha, height, _ := db.FetchAddrIndexTip()
	if newestBlockIdx != height {
		t.Fatalf("Height of address index tip failed to update, "+
			"expected %v, got %v", newestBlockIdx, height)
	}
	if !bytes.Equal(newestSha.Bytes(), sha.Bytes()) {
		t.Fatalf("Sha of address index tip failed to update, "+
			"expected %v, got %v", newestSha, sha)
	}
}

func testOperationalMode(t *testing.T) {
	// simplified basic operation is:
	// 1) fetch block from remote server
	// 2) look up all txin (except coinbase in db)
	// 3) insert block
	// 4) exercise the optional addridex
	testDb, err := setUpTestDb(t, "tstdbopmode")
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer testDb.cleanUpFunc()
	err = nil
out:
	for height := int64(0); height < int64(len(testDb.blocks)); height++ {
		block := testDb.blocks[height]
		if height != 0 {
			// except for NoVerify which does not allow lookups check inputs
			mblock := block.MsgBlock()
			//t.Errorf("%v", blockchain.DebugBlockString(block))
			parentBlock := testDb.blocks[height-1]
			mParentBlock := parentBlock.MsgBlock()
			var txneededList []*chainhash.Hash
			opSpentInBlock := make(map[wire.OutPoint]struct{})
			if dcrutil.IsFlagSet16(dcrutil.BlockValid, mParentBlock.Header.VoteBits) {
				for _, tx := range mParentBlock.Transactions {
					for _, txin := range tx.TxIn {
						if txin.PreviousOutPoint.Index == uint32(4294967295) {
							continue
						}

						if existsInOwnBlockRegTree(mParentBlock, txin.PreviousOutPoint.Hash) {
							_, used := opSpentInBlock[txin.PreviousOutPoint]
							if !used {
								// Origin tx is in the block and so hasn't been
								// added yet, continue
								opSpentInBlock[txin.PreviousOutPoint] = struct{}{}
								continue
							} else {
								t.Errorf("output ref %v attempted double spend of previously spend output", txin.PreviousOutPoint)
							}
						}

						origintxsha := &txin.PreviousOutPoint.Hash
						txneededList = append(txneededList, origintxsha)
						exists, err := testDb.db.ExistsTxSha(origintxsha)
						if err != nil {
							t.Errorf("ExistsTxSha: unexpected error %v ", err)
						}
						if !exists {
							t.Errorf("referenced tx not found %v (height %v)", origintxsha, height)
						}

						_, err = testDb.db.FetchTxBySha(origintxsha)
						if err != nil {
							t.Errorf("referenced tx not found %v err %v ", origintxsha, err)
						}
					}
				}
			}
			for _, stx := range mblock.STransactions {
				for _, txin := range stx.TxIn {
					if txin.PreviousOutPoint.Index == uint32(4294967295) {
						continue
					}
					if existsInOwnBlockRegTree(mParentBlock, txin.PreviousOutPoint.Hash) {
						_, used := opSpentInBlock[txin.PreviousOutPoint]
						if !used {
							// Origin tx is in the block and so hasn't been
							// added yet, continue
							opSpentInBlock[txin.PreviousOutPoint] = struct{}{}
							continue
						} else {
							t.Errorf("output ref %v attempted double spend of previously spend output", txin.PreviousOutPoint)
						}
					}

					origintxsha := &txin.PreviousOutPoint.Hash
					txneededList = append(txneededList, origintxsha)

					exists, err := testDb.db.ExistsTxSha(origintxsha)
					if err != nil {
						t.Errorf("ExistsTxSha: unexpected error %v ", err)
					}
					if !exists {
						t.Errorf("referenced tx not found %v", origintxsha)
					}

					_, err = testDb.db.FetchTxBySha(origintxsha)
					if err != nil {
						t.Errorf("referenced tx not found %v err %v ", origintxsha, err)
					}
				}
			}
			txlist := testDb.db.FetchUnSpentTxByShaList(txneededList)
			for _, txe := range txlist {
				if txe.Err != nil {
					t.Errorf("tx list fetch failed %v err %v ", txe.Sha, txe.Err)
					break out
				}
			}
		}
		newheight, err := testDb.db.InsertBlock(block)
		if err != nil {
			t.Errorf("failed to insert block %v err %v", height, err)
			break out
		}
		if newheight != height {
			t.Errorf("height mismatch expect %v returned %v", height, newheight)
			break out
		}

		newSha, blkid, err := testDb.db.NewestSha()
		if err != nil {
			t.Errorf("failed to obtain latest sha %v %v", height, err)
		}

		if blkid != height {
			t.Errorf("height does not match latest block height %v %v %v", blkid, height, err)
		}

		blkSha := block.Sha()
		if *newSha != *blkSha {
			t.Errorf("Newest block sha does not match freshly inserted one %v %v %v ", newSha, blkSha, err)
		}
	}

	// now that the db is populated, do some additional tests
	testFetchHeightRange(t, testDb.db, testDb.blocks)

	// Ensure all operations dealing with the optional address index behave
	// correctly.
	newSha, blkid, err := testDb.db.NewestSha()
	testAddrIndexOperations(t, testDb.db, testDb.blocks[len(testDb.blocks)-1], newSha, blkid)
}

func TestBackout(t *testing.T) {
	testBackout(t)
}

func testBackout(t *testing.T) {
	// simplified basic operation is:
	// 1) fetch block from remote server
	// 2) look up all txin (except coinbase in db)
	// 3) insert block

	testDb, err := setUpTestDb(t, "tstdbbackout")
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer testDb.cleanUpFunc()

	if len(testDb.blocks) < 120 {
		t.Errorf("test data too small")
		return
	}

	err = nil
	for height := int64(0); height < int64(len(testDb.blocks)); height++ {
		if height == 100 {
			testDb.db.Sync()
		}
		if height == 120 {
			// Simulate unexpected application quit
			testDb.db.RollbackClose()
			break
		}

		block := testDb.blocks[height]

		newheight, err := testDb.db.InsertBlock(block)
		if err != nil {
			t.Errorf("failed to insert block %v err %v", height, err)
			return
		}
		if newheight != height {
			t.Errorf("height mismatch expect %v returned %v", height, newheight)
			return
		}
	}

	// db was closed at height 120, so no cleanup is possible.

	// reopen db
	testDb.db, err = database.OpenDB("leveldb", testDb.dbName)
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer func() {
		if err := testDb.db.Close(); err != nil {
			t.Errorf("Close: unexpected error: %v", err)
		}
	}()

	sha := testDb.blocks[99].Sha()
	if _, err := testDb.db.ExistsSha(sha); err != nil {
		t.Errorf("ExistsSha: unexpected error: %v", err)
	}
	_, err = testDb.db.FetchBlockBySha(sha)
	if err != nil {
		t.Errorf("failed to load block 99 from db %v", err)
		return
	}

	sha = testDb.blocks[119].Sha()
	if _, err := testDb.db.ExistsSha(sha); err != nil {
		t.Errorf("ExistsSha: unexpected error: %v", err)
	}
	_, err = testDb.db.FetchBlockBySha(sha)
	if err != nil {
		t.Errorf("loaded block 119 from db")
		return
	}

	// pick block 118 since tx for block 119 wont be inserted until block 120 is seen to be valid
	block := testDb.blocks[118]
	mblock := block.MsgBlock()
	txsha := mblock.Transactions[0].TxSha()
	exists, err := testDb.db.ExistsTxSha(&txsha)
	if err != nil {
		t.Errorf("ExistsTxSha: unexpected error %v ", err)
	}
	if !exists {
		t.Errorf("tx %v not located db\n", txsha)
	}

	_, err = testDb.db.FetchTxBySha(&txsha)
	if err != nil {
		t.Errorf("tx %v not located db\n", txsha)
		return
	}
}

func loadBlocks(t *testing.T, file string) (blocks []*dcrutil.Block, err error) {
	fi, err := os.Open(file)
	if err != nil {
		t.Errorf("failed to open file %v, err %v", file, err)
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
	blocks = make([]*dcrutil.Block, 0, len(blockchain))
	for height := int64(1); height < int64(len(blockchain)); height++ {
		block, err := dcrutil.NewBlockFromBytes(blockchain[height])
		if err != nil {
			t.Errorf("failed to parse block %v", height)
			return nil, err
		}
		block.SetHeight(height - 1)
		blocks = append(blocks, block)
	}

	return
}

func testFetchHeightRange(t *testing.T, db database.Db, blocks []*dcrutil.Block) {

	var testincrement int64 = 50
	var testcnt int64 = 100

	shanames := make([]*chainhash.Hash, len(blocks))

	nBlocks := int64(len(blocks))

	for i := range blocks {
		shanames[i] = blocks[i].Sha()
	}

	for startheight := int64(0); startheight < nBlocks; startheight += testincrement {
		endheight := startheight + testcnt

		if endheight > nBlocks {
			endheight = database.AllShas
		}

		shalist, err := db.FetchHeightRange(startheight, endheight)
		if err != nil {
			t.Errorf("FetchHeightRange: unexpected failure looking up shas %v", err)
		}

		if endheight == database.AllShas {
			if int64(len(shalist)) != nBlocks-startheight {
				t.Errorf("FetchHeightRange: expected A %v shas, got %v", nBlocks-startheight, len(shalist))
			}
		} else {
			if int64(len(shalist)) != testcnt {
				t.Errorf("FetchHeightRange: expected %v shas, got %v", testcnt, len(shalist))
			}
		}

		for i := range shalist {
			sha0 := *shanames[int64(i)+startheight]
			sha1 := shalist[i]
			if sha0 != sha1 {
				t.Errorf("FetchHeightRange: mismatch sha at %v requested range %v %v: %v %v ", int64(i)+startheight, startheight, endheight, sha0, sha1)
			}
		}
	}

}

func TestLimitAndSkipFetchTxsForAddr(t *testing.T) {
	testDb, err := setUpTestDb(t, "tstdbtxaddr")
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer testDb.cleanUpFunc()
	_, err = testDb.db.InsertBlock(testDb.blocks[0])
	if err != nil {
		t.Fatalf("failed to insert initial block")
	}
	// Insert a block with some fake test transactions. The block will have
	// 10 copies of a fake transaction involving same address.
	addrString := "DsZEAobx6qJ7K2qaHZBA2vBn66Nor8KYAKk"
	targetAddr, err := dcrutil.DecodeAddress(addrString, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Unable to decode test address: %v", err)
	}
	outputScript, err := txscript.PayToAddrScript(targetAddr)
	if err != nil {
		t.Fatalf("Unable make test pkScript %v", err)
	}
	fakeTxOut := wire.NewTxOut(10, outputScript)
	var emptyHash chainhash.Hash
	fakeHeader := wire.NewBlockHeader(0, &emptyHash, &emptyHash, &emptyHash, 1, [6]byte{}, 1, 1, 1, 1, 1, 1, 1, 1, 1, [36]byte{})
	msgBlock := wire.NewMsgBlock(fakeHeader)
	for i := 0; i < 10; i++ {
		mtx := wire.NewMsgTx()
		mtx.AddTxOut(fakeTxOut)
		msgBlock.AddTransaction(mtx)
	}
	lastBlock := testDb.blocks[0]
	msgBlock.Header.PrevBlock = *lastBlock.Sha()
	// Insert the test block into the DB.
	testBlock := dcrutil.NewBlock(msgBlock)
	newheight, err := testDb.db.InsertBlock(testBlock)
	if err != nil {
		t.Fatalf("Unable to insert block into db: %v", err)
	}

	// Create and insert an address index for out test addr.
	txLoc, _, _ := testBlock.TxLoc()
	index := make(database.BlockAddrIndex, len(txLoc))
	for i := range testBlock.Transactions() {
		var hash160 [ripemd160.Size]byte
		scriptAddr := targetAddr.ScriptAddress()
		copy(hash160[:], scriptAddr[:])
		txAddrIndex := &database.TxAddrIndex{
			Hash160:  hash160,
			Height:   uint32(newheight),
			TxOffset: uint32(txLoc[i].TxStart),
			TxLen:    uint32(txLoc[i].TxLen),
		}

		index[i] = txAddrIndex
	}
	blkSha := testBlock.Sha()
	err = testDb.db.UpdateAddrIndexForBlock(blkSha, newheight, index)
	if err != nil {
		t.Fatalf("UpdateAddrIndexForBlock: failed to index"+
			" addrs for block #%d (%s) "+
			"err %v", newheight, blkSha, err)
		return
	}

	// Try skipping the first 4 results, should get 6 in return.
	txReply, txSkipped, err := testDb.db.FetchTxsForAddr(targetAddr, 4, 100000, false)
	if err != nil {
		t.Fatalf("Unable to fetch transactions for address: %v", err)
	}
	if txSkipped != 4 {
		t.Fatalf("Did not correctly return skipped amount"+
			" got %v txs, expected %v", txSkipped, 4)
	}
	if len(txReply) != 6 {
		t.Fatalf("Did not correctly skip forward in txs for address reply"+
			" got %v txs, expected %v", len(txReply), 6)
	}

	// Limit the number of results to 3.
	txReply, txSkipped, err = testDb.db.FetchTxsForAddr(targetAddr, 0, 3, false)
	if err != nil {
		t.Fatalf("Unable to fetch transactions for address: %v", err)
	}
	if txSkipped != 0 {
		t.Fatalf("Did not correctly return skipped amount"+
			" got %v txs, expected %v", txSkipped, 0)
	}
	if len(txReply) != 3 {
		t.Fatalf("Did not correctly limit in txs for address reply"+
			" got %v txs, expected %v", len(txReply), 3)
	}

	// Skip 1, limit 5.
	txReply, txSkipped, err = testDb.db.FetchTxsForAddr(targetAddr, 1, 5, false)
	if err != nil {
		t.Fatalf("Unable to fetch transactions for address: %v", err)
	}
	if txSkipped != 1 {
		t.Fatalf("Did not correctly return skipped amount"+
			" got %v txs, expected %v", txSkipped, 1)
	}
	if len(txReply) != 5 {
		t.Fatalf("Did not correctly limit in txs for address reply"+
			" got %v txs, expected %v", len(txReply), 5)
	}
}
