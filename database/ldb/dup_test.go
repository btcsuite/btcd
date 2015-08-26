// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	//"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

func existsInOwnBlockRegTree(block *wire.MsgBlock, hash chainhash.Hash) bool {
	for _, tx := range block.Transactions {
		txH := tx.TxSha()
		if txH == hash {
			return true
		}
	}

	return false
}

func Test_dupTx(t *testing.T) {

	// Ignore db remove errors since it means we didn't have an old one.
	dbname := fmt.Sprintf("tstdbdup0")
	dbnamever := dbname + ".ver"
	_ = os.RemoveAll(dbname)
	_ = os.RemoveAll(dbnamever)
	db, err := database.CreateDB("leveldb", dbname)
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

	testdatafile := filepath.Join("../", "../blockchain/testdata", "blocks0to168.bz2")
	blocks, err := loadBlocks(t, testdatafile)
	if err != nil {
		t.Errorf("Unable to load blocks from test data for: %v",
			err)
		return
	}

	var lastSha *chainhash.Hash

	// Populate with the fisrt 256 blocks, so we have blocks to 'mess with'
	err = nil
out:
	for height := int64(0); height < int64(len(blocks)); height++ {
		block := blocks[height]
		if height != 0 {
			// except for NoVerify which does not allow lookups check inputs
			mblock := block.MsgBlock()
			//t.Errorf("%v", blockchain.DebugBlockString(block))
			parentBlock := blocks[height-1]
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
						exists, err := db.ExistsTxSha(origintxsha)
						if err != nil {
							t.Errorf("ExistsTxSha: unexpected error %v ", err)
						}
						if !exists {
							t.Errorf("referenced tx not found %v (height %v)", origintxsha, height)
						}

						_, err = db.FetchTxBySha(origintxsha)
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

					exists, err := db.ExistsTxSha(origintxsha)
					if err != nil {
						t.Errorf("ExistsTxSha: unexpected error %v ", err)
					}
					if !exists {
						t.Errorf("referenced tx not found %v", origintxsha)
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

		blkSha := block.Sha()
		if *newSha != *blkSha {
			t.Errorf("Newest block sha does not match freshly inserted one %v %v %v ", newSha, blkSha, err)
		}
		lastSha = blkSha
	}

	// generate a new block based on the last sha
	// these block are not verified, so there are a bunch of garbage fields
	// in the 'generated' block.

	var bh wire.BlockHeader

	bh.Version = 0
	bh.PrevBlock = *lastSha
	// Bits, Nonce are not filled in

	mblk := wire.NewMsgBlock(&bh)

	hash, _ := chainhash.NewHashFromStr("c23953c56cb2ef8e4698e3ed3b0fc4c837754d3cd16485192d893e35f32626b4")

	po := wire.NewOutPoint(hash, 0, dcrutil.TxTreeRegular)
	txI := wire.NewTxIn(po, []byte("garbage"))
	txO := wire.NewTxOut(50000000, []byte("garbageout"))

	var tx wire.MsgTx
	tx.AddTxIn(txI)
	tx.AddTxOut(txO)

	mblk.AddTransaction(&tx)

	blk := dcrutil.NewBlock(mblk)

	fetchList := []*chainhash.Hash{hash}
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
		if lr.Err != nil && lr.Err != database.ErrTxShaMissing {
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
					t.Errorf("stx %v spent %v err %v\n", lr.Sha,
						lr.TxSpent, lr.Err)
				}
			}
		}
	}

	err = db.DropAfterBlockBySha(lastSha)
	if err != nil {
		t.Errorf("failed to drop spending block %v", err)
	}
}
