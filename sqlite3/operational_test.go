// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package sqlite3_test

import (
	"compress/bzip2"
	"encoding/binary"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcdb/sqlite3"
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
	testOperationalMode(t, dbTmNormal)
	testOperationalMode(t, dbTmFast)
	testOperationalMode(t, dbTmNoVerify)
}

func testOperationalMode(t *testing.T, mode int) {
	// simplified basic operation is:
	// 1) fetch block from remote server
	// 2) look up all txin (except coinbase in db)
	// 3) insert block

	// Ignore db remove errors since it means we didn't have an old one.
	dbname := "tstdbop1"
	_ = os.Remove(dbname)
	db, err := btcdb.CreateDB("sqlite", dbname)
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer os.Remove(dbname)
	defer db.Close()

	switch mode {
	case dbTmDefault: // default
		// no setup
	case dbTmNormal: // explicit normal
		db.SetDBInsertMode(btcdb.InsertNormal)
	case dbTmFast: // fast mode
		db.SetDBInsertMode(btcdb.InsertFast)
		if sqldb, ok := db.(*sqlite3.SqliteDb); ok {
			sqldb.TempTblMax = 100
		} else {
			t.Errorf("not right type")
		}
	case dbTmNoVerify: // validated block
		db.SetDBInsertMode(btcdb.InsertValidatedInput)
	}

	// Since we are dealing with small dataset, reduce cache size
	sqlite3.SetBlockCacheSize(db, 2)
	sqlite3.SetTxCacheSize(db, 3)

	testdatafile := filepath.Join("testdata", "blocks1-256.bz2")
	blocks, err := loadBlocks(t, testdatafile)
	if err != nil {
		t.Errorf("Unable to load blocks from test data for mode %v: %v",
			mode, err)
		return
	}

	err = nil
out:
	for height := int64(1); height < int64(len(blocks)); height++ {
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
	}

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
	testBackout(t, dbTmNormal)
	testBackout(t, dbTmFast)
}

func testBackout(t *testing.T, mode int) {
	// simplified basic operation is:
	// 1) fetch block from remote server
	// 2) look up all txin (except coinbase in db)
	// 3) insert block

	// Ignore db remove errors since it means we didn't have an old one.
	dbname := "tstdbop2"
	_ = os.Remove(dbname)
	db, err := btcdb.CreateDB("sqlite", dbname)
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer os.Remove(dbname)
	defer db.Close()

	switch mode {
	case dbTmDefault: // default
		// no setup
	case dbTmNormal: // explicit normal
		db.SetDBInsertMode(btcdb.InsertNormal)
	case dbTmFast: // fast mode
		db.SetDBInsertMode(btcdb.InsertFast)
		if sqldb, ok := db.(*sqlite3.SqliteDb); ok {
			sqldb.TempTblMax = 100
		} else {
			t.Errorf("not right type")
		}
	}

	// Since we are dealing with small dataset, reduce cache size
	sqlite3.SetBlockCacheSize(db, 2)
	sqlite3.SetTxCacheSize(db, 3)

	testdatafile := filepath.Join("testdata", "blocks1-256.bz2")
	blocks, err := loadBlocks(t, testdatafile)
	if len(blocks) < 120 {
		t.Errorf("test data too small")
		return
	}

	err = nil
	for height := int64(1); height < int64(len(blocks)); height++ {
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
			break
		}
		if newheight != height {
			t.Errorf("height mismatch expect %v returned %v", height, newheight)
			break
		}
	}

	// db was closed at height 120, so no cleanup is possible.

	// reopen db
	db, err = btcdb.OpenDB("sqlite", dbname)
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
		t.Errorf("failed to load block 99 from db", err)
	}

	sha, err = blocks[110].Sha()
	if err != nil {
		t.Errorf("failed to get block 110 sha err %v", err)
		return
	}
	_ = db.ExistsSha(sha)
	_, err = db.FetchBlockBySha(sha)
	if err == nil {
		t.Errorf("loaded block 110 from db, failure expected")
		return
	}

	block := blocks[110]
	mblock := block.MsgBlock()
	txsha, err := mblock.Transactions[0].TxSha(block.ProtocolVersion())
	exists := db.ExistsTxSha(&txsha)
	if exists {
		t.Errorf("tx %v exists in db, failure expected")
	}

	_, _, _, err = db.FetchTxBySha(&txsha)
	_, err = db.FetchTxUsedBySha(&txsha)

	block = blocks[99]
	mblock = block.MsgBlock()
	txsha, err = mblock.Transactions[0].TxSha(block.ProtocolVersion())
	oldused, err := db.FetchTxUsedBySha(&txsha)
	err = db.InsertTx(&txsha, 99, 1024, 1048, oldused)
	if err == nil {
		t.Errorf("dup insert of tx succeeded")
		return
	}
}

func loadBlocks(t *testing.T, file string) (blocks []*btcutil.Block, err error) {
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

	var block *btcutil.Block
	// block 0 isn't really there, put in nil
	blocks = append(blocks, block)

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

		var pver uint32
		switch {
		case height < 200000:
			pver = 1
		case height >= 200000:
			pver = 2
		}
		block, err = btcutil.NewBlockFromBytes(rbytes, pver)
		if err != nil {
			t.Errorf("failed to parse block %v", height)
			return
		}
		blocks = append(blocks, block)
	}
	return
}
