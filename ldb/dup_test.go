// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/conformal/btcdb"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

func Test_dupTx(t *testing.T) {

	// Ignore db remove errors since it means we didn't have an old one.
	dbname := fmt.Sprintf("tstdbdup0")
	dbnamever := dbname + ".ver"
	_ = os.RemoveAll(dbname)
	_ = os.RemoveAll(dbnamever)
	db, err := btcdb.CreateDB("leveldb", dbname)
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer os.RemoveAll(dbname)
	defer os.RemoveAll(dbnamever)
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close: unexpected error: %v", err)
		}
	}()

	testdatafile := filepath.Join("testdata", "blocks1-256.bz2")
	blocks, err := loadBlocks(t, testdatafile)
	if err != nil {
		t.Errorf("Unable to load blocks from test data for: %v",
			err)
		return
	}

	var lastSha *btcwire.ShaHash

	// Populate with the fisrt 256 blocks, so we have blocks to 'mess with'
	err = nil
out:
	for height := int64(0); height < int64(len(blocks)); height++ {
		block := blocks[height]

		// except for NoVerify which does not allow lookups check inputs
		mblock := block.MsgBlock()
		var txneededList []*btcwire.ShaHash
		for _, tx := range mblock.Transactions {
			for _, txin := range tx.TxIn {
				if txin.PreviousOutPoint.Index == uint32(4294967295) {
					continue
				}
				origintxsha := &txin.PreviousOutPoint.Hash
				txneededList = append(txneededList, origintxsha)

				exists, err := db.ExistsTxSha(origintxsha)
				if err != nil {
					t.Errorf("ExistsTxSha: unexpected error %v ", err)
				}
				if !exists {
					t.Errorf("referenced tx not found %v ", origintxsha)
				}

				_, err = db.FetchTxBySha(origintxsha)
				if err != nil {
					t.Errorf("referenced tx not found %v err %v ", origintxsha, err)
				}
			}
		}
		txlist := db.FetchUnSpentTxByShaList(txneededList)
		for _, txe := range txlist {
			if txe.Err != nil {
				t.Errorf("tx list fetch failed %v err %v ", txe.Sha, txe.Err)
				break out
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
			t.Errorf("height doe not match latest block height %v %v %v", blkid, height, err)
		}

		blkSha, _ := block.Sha()
		if *newSha != *blkSha {
			t.Errorf("Newest block sha does not match freshly inserted one %v %v %v ", newSha, blkSha, err)
		}
		lastSha = blkSha
	}

	// genrate a new block based on the last sha
	// these block are not verified, so there are a bunch of garbage fields
	// in the 'generated' block.

	var bh btcwire.BlockHeader

	bh.Version = 2
	bh.PrevBlock = *lastSha
	// Bits, Nonce are not filled in

	mblk := btcwire.NewMsgBlock(&bh)

	hash, _ := btcwire.NewShaHashFromStr("df2b060fa2e5e9c8ed5eaf6a45c13753ec8c63282b2688322eba40cd98ea067a")

	po := btcwire.NewOutPoint(hash, 0)
	txI := btcwire.NewTxIn(po, []byte("garbage"))
	txO := btcwire.NewTxOut(50000000, []byte("garbageout"))

	var tx btcwire.MsgTx
	tx.AddTxIn(txI)
	tx.AddTxOut(txO)

	mblk.AddTransaction(&tx)

	blk := btcutil.NewBlock(mblk)

	fetchList := []*btcwire.ShaHash{hash}
	listReply := db.FetchUnSpentTxByShaList(fetchList)
	for _, lr := range listReply {
		if lr.Err != nil {
			t.Errorf("sha %v spent %v err %v\n", lr.Sha,
				lr.TxSpent, lr.Err)
		}
	}

	_, err = db.InsertBlock(blk)
	if err != nil {
		t.Errorf("failed to insert phony block %v", err)
	}

	// ok, did it 'spend' the tx ?

	listReply = db.FetchUnSpentTxByShaList(fetchList)
	for _, lr := range listReply {
		if lr.Err != btcdb.ErrTxShaMissing {
			t.Errorf("sha %v spent %v err %v\n", lr.Sha,
				lr.TxSpent, lr.Err)
		}
	}

	txlist := blk.Transactions()
	for _, tx := range txlist {
		txsha := tx.Sha()
		txReply, err := db.FetchTxBySha(txsha)
		if err != nil {
			t.Errorf("fully spent lookup %v err %v\n", hash, err)
		} else {
			for _, lr := range txReply {
				if lr.Err != nil {
					fmt.Errorf("stx %v spent %v err %v\n", lr.Sha,
						lr.TxSpent, lr.Err)
				}
			}
		}
	}

	t.Logf("Dropping block")

	err = db.DropAfterBlockBySha(lastSha)
	if err != nil {
		t.Errorf("failed to drop spending block %v", err)
	}
}
