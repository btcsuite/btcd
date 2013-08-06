// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package sqlite3_test

import (
	"github.com/conformal/btcdb"
	"github.com/conformal/btcdb/sqlite3"
	"os"
	"path/filepath"
	"testing"
)

func TestFailOperational(t *testing.T) {
	sqlite3.SetTestingT(t)
	failtestOperationalMode(t, dbTmDefault)
	failtestOperationalMode(t, dbTmNormal)
	failtestOperationalMode(t, dbTmFast)
	failtestOperationalMode(t, dbTmNoVerify)
}

func failtestOperationalMode(t *testing.T, mode int) {
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
		// no point in testing this 
		return
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
	for height := int64(0); height < int64(len(blocks)); height++ {
		block := blocks[height]

		mblock := block.MsgBlock()
		blockname, _ :=  block.Sha()

		if height == 248 {
			// time to corrupt the datbase, to see if it leaves the block or tx in the db
			if len(mblock.Transactions) != 2 {
				t.Errorf("transaction #248 should have two transactions txid %v ?= 828ef3b079f9c23829c56fe86e85b4a69d9e06e5b54ea597eef5fb3ffef509fe", blockname)
				return
			}
			tx := mblock.Transactions[1]
			txin := tx.TxIn[0]
			origintxsha := &txin.PreviousOutpoint.Hash
			sqlite3.KillTx(db, origintxsha)
			_, _, _, _, err = db.FetchTxAllBySha(origintxsha)
			if err == nil {
				t.Errorf("deleted tx found %v", origintxsha)
			}
		}


		if height == 248 {
		}
		newheight, err := db.InsertBlock(block)
		if err != nil {
			if height != 248 {
				t.Errorf("failed to insert block %v err %v", height, err)
				break out
			}
		} else {
			if height == 248 {
				t.Errorf("block insert with missing input tx succeeded block %v err %v", height, err)
				break out
			}
		}
		if height == 248 {
			for _, tx := range mblock.Transactions {
				txsha, err := tx.TxSha()
				_, _, _, _, err = db.FetchTxAllBySha(&txsha)
				if err == nil {
					t.Errorf("referenced tx found, should not have been %v, ", txsha)
				}
			}
		}
		if height == 248 {
			exists := db.ExistsSha(blockname)
			if exists == true {
				t.Errorf("block still present after failed insert")
			}
			// if we got here with no error, testing was successful
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
