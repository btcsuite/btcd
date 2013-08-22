// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb_test

import (
	"compress/bzip2"
	"encoding/binary"
	"fmt"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var network = btcwire.MainNet

const (
	dbTmDefault = iota
	dbTmNormal
	dbTmFast
	dbTmNoVerify
)

func TestOperational(t *testing.T) {
	testOperationalMode(t, dbTmDefault)
	//testOperationalMode(t, dbTmNormal)
	//testOperationalMode(t, dbTmFast)
	//testOperationalMode(t, dbTmNoVerify)
}

func testOperationalMode(t *testing.T, mode int) {
	// simplified basic operation is:
	// 1) fetch block from remote server
	// 2) look up all txin (except coinbase in db)
	// 3) insert block

	// Ignore db remove errors since it means we didn't have an old one.
	dbname := fmt.Sprintf("tstdbop1.%d", mode)
	_ = os.RemoveAll(dbname)
	db, err := btcdb.CreateDB("leveldb", dbname)
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer os.RemoveAll(dbname)
	defer db.Close()

	switch mode {
	case dbTmDefault: // default
		// no setup
	case dbTmNormal: // explicit normal
		db.SetDBInsertMode(btcdb.InsertNormal)
	case dbTmFast: // fast mode

	case dbTmNoVerify: // validated block
		db.SetDBInsertMode(btcdb.InsertValidatedInput)
	}

	testdatafile := filepath.Join("testdata", "blocks1-256.bz2")
	blocks, err := loadBlocks(t, testdatafile)
	if err != nil {
		t.Errorf("Unable to load blocks from test data for mode %v: %v",
			mode, err)
		return
	}

	err = nil
out:
	for height := int64(0); height < int64(len(blocks)); height++ {
		block := blocks[height]
		if mode != dbTmNoVerify {
			// except for NoVerify which does not allow lookups check inputs
			mblock := block.MsgBlock()
			var txneededList []*btcwire.ShaHash
			for _, tx := range mblock.Transactions {
				for _, txin := range tx.TxIn {
					if txin.PreviousOutpoint.Index == uint32(4294967295) {
						continue
					}
					origintxsha := &txin.PreviousOutpoint.Hash
					txneededList = append(txneededList, origintxsha)

					if !db.ExistsTxSha(origintxsha) {
						t.Errorf("referenced tx not found %v ", origintxsha)
					}

					_, _, _, _, err := db.FetchTxAllBySha(origintxsha)
					if err != nil {
						t.Errorf("referenced tx not found %v err %v ", origintxsha, err)
					}
					_, _, _, _, err = db.FetchTxAllBySha(origintxsha)
					if err != nil {
						t.Errorf("referenced tx not found %v err %v ", origintxsha, err)
					}
					_, _, _, err = db.FetchTxBySha(origintxsha)
					if err != nil {
						t.Errorf("referenced tx not found %v err %v ", origintxsha, err)
					}
					_, _, err = db.FetchTxBufBySha(origintxsha)
					if err != nil {
						t.Errorf("referenced tx not found %v err %v ", origintxsha, err)
					}
					_, err = db.FetchTxUsedBySha(origintxsha)
					if err != nil {
						t.Errorf("tx used fetch fail %v err %v ", origintxsha, err)
					}
				}
			}
			txlist := db.FetchTxByShaList(txneededList)
			for _, txe := range txlist {
				if txe.Err != nil {
					t.Errorf("tx list fetch failed %v err %v ", txe.Sha, txe.Err)
					break out
				}
			}

		}

		newheight, err := db.InsertBlock(block)
		if err != nil {
			t.Errorf("failed to insert block %v err %v", height, err)
			break out
		}
		if newheight != height {
			t.Errorf("height mismatch expect %v returned %v", height, newheight)
			break out
		}

		newSha, blkid, err := db.NewestSha()
		if err != nil {
			t.Errorf("failed to obtain latest sha %v %v", height, err)
		}

		if blkid != height {
			t.Errorf("height doe not match latest block height %v %v", blkid, height, err)
		}

		blkSha, _ := block.Sha()
		if *newSha != *blkSha {
			t.Errorf("Newest block sha does not match freshly inserted one %v %v ", newSha, blkSha, err)
		}
	}

	// now that db is populated, do some additional test
	testFetchRangeHeight(t, db, blocks)

	switch mode {
	case dbTmDefault: // default
		// no cleanup
	case dbTmNormal: // explicit normal
		// no cleanup
	case dbTmFast: // fast mode
		db.SetDBInsertMode(btcdb.InsertNormal)
	case dbTmNoVerify: // validated block
		db.SetDBInsertMode(btcdb.InsertNormal)
	}
}

func TestBackout(t *testing.T) {
	testBackout(t, dbTmDefault)
	//testBackout(t, dbTmNormal)
	//testBackout(t, dbTmFast)
}

func testBackout(t *testing.T, mode int) {
	// simplified basic operation is:
	// 1) fetch block from remote server
	// 2) look up all txin (except coinbase in db)
	// 3) insert block

	t.Logf("mode %v", mode)
	// Ignore db remove errors since it means we didn't have an old one.
	dbname := fmt.Sprintf("tstdbop2.%d", mode)
	_ = os.RemoveAll(dbname)
	db, err := btcdb.CreateDB("leveldb", dbname)
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer os.RemoveAll(dbname)
	defer db.Close()

	switch mode {
	case dbTmDefault: // default
		// no setup
	case dbTmNormal: // explicit normal
		db.SetDBInsertMode(btcdb.InsertNormal)
	case dbTmFast: // fast mode

	}

	testdatafile := filepath.Join("testdata", "blocks1-256.bz2")
	blocks, err := loadBlocks(t, testdatafile)
	if len(blocks) < 120 {
		t.Errorf("test data too small")
		return
	}

	err = nil
	for height := int64(0); height < int64(len(blocks)); height++ {
		if height == 100 {
			t.Logf("Syncing at block height 100")
			db.Sync()
		}
		if height == 120 {
			t.Logf("Simulating unexpected application quit")
			// Simulate unexpected application quit
			db.RollbackClose()
			break
		}

		block := blocks[height]

		newheight, err := db.InsertBlock(block)
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
	db, err = btcdb.OpenDB("leveldb", dbname)
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer db.Close()

	sha, err := blocks[99].Sha()
	if err != nil {
		t.Errorf("failed to get block 99 sha err %v", err)
		return
	}
	_ = db.ExistsSha(sha)
	_, err = db.FetchBlockBySha(sha)
	if err != nil {
		t.Errorf("failed to load block 99 from db %v", err)
		return
	}

	sha, err = blocks[119].Sha()
	if err != nil {
		t.Errorf("failed to get block 110 sha err %v", err)
		return
	}
	_ = db.ExistsSha(sha)
	_, err = db.FetchBlockBySha(sha)
	if err != nil {
		t.Errorf("loaded block 119 from db")
		return
	}

	block := blocks[119]
	mblock := block.MsgBlock()
	txsha, err := mblock.Transactions[0].TxSha()
	exists := db.ExistsTxSha(&txsha)
	if !exists {
		t.Errorf("tx %v not located db\n", txsha)
	}

	_, _, _, err = db.FetchTxBySha(&txsha)
	if err != nil {
		t.Errorf("tx %v not located db\n", txsha)
		return
	}
	_, err = db.FetchTxUsedBySha(&txsha)
	if err != nil {
		t.Errorf("tx %v not located db\n", txsha)
		return
	}
}


var savedblocks []*btcutil.Block

func loadBlocks(t *testing.T, file string) (blocks []*btcutil.Block, err error) {
	if len(savedblocks) != 0 {
		blocks = savedblocks
		return
	}
	testdatafile := filepath.Join("testdata", "blocks1-256.bz2")
	var dr io.Reader
	var fi io.ReadCloser
	fi, err = os.Open(testdatafile)
	if err != nil {
		t.Errorf("failed to open file %v, err %v", testdatafile, err)
		return
	}
	if strings.HasSuffix(testdatafile, ".bz2") {
		z := bzip2.NewReader(fi)
		dr = z
	} else {
		dr = fi
	}

	defer func() {
		if err := fi.Close(); err != nil {
			t.Errorf("failed to close file %v %v", testdatafile, err)
		}
	}()

	// Set the first block as the genesis block.
	genesis := btcutil.NewBlock(&btcwire.GenesisBlock)
	blocks = append(blocks, genesis)

	var block *btcutil.Block
	err = nil
	for height := int64(1); err == nil; height++ {
		var rintbuf uint32
		err = binary.Read(dr, binary.LittleEndian, &rintbuf)
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

		block, err = btcutil.NewBlockFromBytes(rbytes)
		if err != nil {
			t.Errorf("failed to parse block %v", height)
			return
		}
		blocks = append(blocks, block)
	}
	savedblocks = blocks
	return
}

func testFetchRangeHeight(t *testing.T, db btcdb.Db, blocks []*btcutil.Block) {

	var testincrement int64 = 50
	var testcnt int64 = 100

	shanames := make([]*btcwire.ShaHash, len(blocks))

	nBlocks := int64(len(blocks))

	for i := range blocks {
		blockSha, err := blocks[i].Sha()
		if err != nil {
			t.Errorf("FetchRangeHeight: unexpected failure computing block sah %v", err)
		}
		shanames[i] = blockSha
	}

	for startheight := int64(0); startheight < nBlocks; startheight += testincrement {
		endheight := startheight + testcnt

		if endheight > nBlocks {
			endheight = btcdb.AllShas
		}

		shalist, err := db.FetchHeightRange(startheight, endheight)
		if err != nil {
			t.Errorf("FetchRangeHeight: unexpected failure looking up shas %v", err)
		}

		if endheight == btcdb.AllShas {
			if int64(len(shalist)) != nBlocks-startheight {
				t.Errorf("FetchRangeHeight: expected A %v shas, got %v", nBlocks-startheight, len(shalist))
			}
		} else {
			if int64(len(shalist)) != testcnt {
				t.Errorf("FetchRangeHeight: expected %v shas, got %v", testcnt, len(shalist))
			}
		}

		for i := range shalist {
			sha0 := *shanames[int64(i)+startheight]
			sha1 := shalist[i]
			if sha0 != sha1 {
				t.Errorf("FetchRangeHeight: mismatch sha at %v requested range %v %v: %v %v ", int64(i)+startheight, startheight, endheight, sha0, sha1)
			}
		}
	}

}
