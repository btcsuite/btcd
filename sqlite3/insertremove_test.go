// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package sqlite3_test

import (
	//"compress/bzip2"
	//"encoding/binary"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcdb/sqlite3"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	//"io"
	"fmt"
	"os"
	"path/filepath"
	//"strings"
	"testing"
)

var tstBlocks []*btcutil.Block

func loadblocks(t *testing.T) []*btcutil.Block {
	if len(tstBlocks) != 0 {
		return tstBlocks
	}

	testdatafile := filepath.Join("testdata", "blocks1-256.bz2")
	blocks, err := loadBlocks(t, testdatafile)
	if err != nil {
		t.Errorf("Unable to load blocks from test data: %v", err)
		return nil
	}
	tstBlocks = blocks
	return blocks
}

func TestUnspentInsert(t *testing.T) {
	testUnspentInsert(t, dbTmDefault)
	testUnspentInsert(t, dbTmNormal)
	testUnspentInsert(t, dbTmFast)
}

// insert every block in the test chain
// after each insert, fetch all the tx affected by the latest
// block and verify that the the tx is spent/unspent
// new tx should be fully unspent, referenced tx should have
// the associated txout set to spent.
func testUnspentInsert(t *testing.T, mode int) {
	// Ignore db remove errors since it means we didn't have an old one.
	dbname := "tstdbuspnt1"
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
		t.Errorf("UnspentInsert test is not valid in NoVerify mode")
	}

	// Since we are dealing with small dataset, reduce cache size
	sqlite3.SetBlockCacheSize(db, 2)
	sqlite3.SetTxCacheSize(db, 3)

	blocks := loadblocks(t)
endtest:
	for height := int64(1); height < int64(len(blocks)); height++ {

		block := blocks[height]
		// look up inputs to this x
		mblock := block.MsgBlock()
		var txneededList []*btcwire.ShaHash
		var txlookupList []*btcwire.ShaHash
		var txOutList []*btcwire.ShaHash
		var txInList []*btcwire.OutPoint
		for _, tx := range mblock.Transactions {
			for _, txin := range tx.TxIn {
				if txin.PreviousOutpoint.Index == uint32(4294967295) {
					continue
				}
				origintxsha := &txin.PreviousOutpoint.Hash

				txInList = append(txInList, &txin.PreviousOutpoint)
				txneededList = append(txneededList, origintxsha)
				txlookupList = append(txlookupList, origintxsha)

				if !db.ExistsTxSha(origintxsha) {
					t.Errorf("referenced tx not found %v ", origintxsha)
				}

			}
			txshaname, _ := tx.TxSha(block.ProtocolVersion())
			txlookupList = append(txlookupList, &txshaname)
			txOutList = append(txOutList, &txshaname)
		}

		txneededmap := map[btcwire.ShaHash]*btcdb.TxListReply{}
		txlist := db.FetchTxByShaList(txneededList)
		for _, txe := range txlist {
			if txe.Err != nil {
				t.Errorf("tx list fetch failed %v err %v ", txe.Sha, txe.Err)
				break endtest
			}
			txneededmap[*txe.Sha] = txe
		}
		for _, spend := range txInList {
			itxe := txneededmap[spend.Hash]
			if itxe.TxSpent[spend.Index] == true {
				t.Errorf("txin %v:%v is already spent", spend.Hash, spend.Index)
			}
		}

		newheight, err := db.InsertBlock(block)
		if err != nil {
			t.Errorf("failed to insert block %v err %v", height, err)
			break endtest
		}
		if newheight != height {
			t.Errorf("height mismatch expect %v returned %v", height, newheight)
			break endtest
		}

		txlookupmap := map[btcwire.ShaHash]*btcdb.TxListReply{}
		txlist = db.FetchTxByShaList(txlookupList)
		for _, txe := range txlist {
			if txe.Err != nil {
				t.Errorf("tx list fetch failed %v err %v ", txe.Sha, txe.Err)
				break endtest
			}
			txlookupmap[*txe.Sha] = txe
		}
		for _, spend := range txInList {
			itxe := txlookupmap[spend.Hash]
			if itxe.TxSpent[spend.Index] == false {
				fmt.Printf("?? txin %v:%v is unspent %v\n", spend.Hash, spend.Index, itxe.TxSpent)
				t.Errorf("txin %v:%v is unspent %v", spend.Hash, spend.Index, itxe.TxSpent)
			}
		}
		for _, txo := range txOutList {
			itxe := txlookupmap[*txo]
			for i, spent := range itxe.TxSpent {
				if spent == true {
					t.Errorf("freshly inserted tx %v already spent %v", txo, i)
				}
			}

		}
		if len(txInList) == 0 {
			continue
		}
		dropblock := blocks[height-1]
		dropsha, _ := dropblock.Sha()

		err = db.DropAfterBlockBySha(dropsha)
		if err != nil {
			t.Errorf("failed to drop block %v err %v", height, err)
			break endtest
		}

		txlookupmap = map[btcwire.ShaHash]*btcdb.TxListReply{}
		txlist = db.FetchTxByShaList(txlookupList)
		for _, txe := range txlist {
			if txe.Err != nil {
				if _, ok := txneededmap[*txe.Sha]; ok {
					t.Errorf("tx list fetch failed %v err %v ", txe.Sha, txe.Err)
					break endtest
				}
			}
			txlookupmap[*txe.Sha] = txe
		}
		for _, spend := range txInList {
			itxe := txlookupmap[spend.Hash]
			if itxe.TxSpent[spend.Index] == true {
				fmt.Printf("?? txin %v:%v is spent %v\n", spend.Hash, spend.Index, itxe.TxSpent)
				t.Errorf("txin %v:%v is unspent %v", spend.Hash, spend.Index, itxe.TxSpent)
			}
		}
		newheight, err = db.InsertBlock(block)
		if err != nil {
			t.Errorf("failed to insert block %v err %v", height, err)
			break endtest
		}
		txlookupmap = map[btcwire.ShaHash]*btcdb.TxListReply{}
		txlist = db.FetchTxByShaList(txlookupList)
		for _, txe := range txlist {
			if txe.Err != nil {
				t.Errorf("tx list fetch failed %v err %v ", txe.Sha, txe.Err)
				break endtest
			}
			txlookupmap[*txe.Sha] = txe
		}
		for _, spend := range txInList {
			itxe := txlookupmap[spend.Hash]
			if itxe.TxSpent[spend.Index] == false {
				fmt.Printf("?? txin %v:%v is unspent %v\n", spend.Hash, spend.Index, itxe.TxSpent)
				t.Errorf("txin %v:%v is unspent %v", spend.Hash, spend.Index, itxe.TxSpent)
			}
		}
	}
}
