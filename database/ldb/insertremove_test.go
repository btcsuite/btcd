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

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ldb"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

var tstBlocks []*dcrutil.Block

func loadblocks(t *testing.T) []*dcrutil.Block {
	if len(tstBlocks) != 0 {
		return tstBlocks
	}

	testdatafile := filepath.Join("..", "/../blockchain/testdata", "blocks0to168.bz2")
	blocks, err := loadBlocks(t, testdatafile)
	if err != nil {
		t.Errorf("Unable to load blocks from test data: %v", err)
		return nil
	}
	tstBlocks = blocks
	return blocks
}

func TestUnspentInsert(t *testing.T) {
	testUnspentInsertStakeTree(t)
	testUnspentInsertRegTree(t)
}

// insert every block in the test chain
// after each insert, fetch all the tx affected by the latest
// block and verify that the the tx is spent/unspent
// new tx should be fully unspent, referenced tx should have
// the associated txout set to spent.
// checks tx tree stake only
func testUnspentInsertStakeTree(t *testing.T) {
	// Ignore db remove errors since it means we didn't have an old one.
	dbname := fmt.Sprintf("tstdbuspnt1")
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
	blocks := loadblocks(t)
endtest:
	for height := int64(0); height < int64(len(blocks))-1; height++ {
		block := blocks[height]

		var txneededList []*chainhash.Hash
		var txlookupList []*chainhash.Hash
		var txOutList []*chainhash.Hash
		var txInList []*wire.OutPoint
		spentFromParent := make(map[wire.OutPoint]struct{})
		for _, tx := range block.MsgBlock().STransactions {
			for _, txin := range tx.TxIn {
				if txin.PreviousOutPoint.Index == uint32(4294967295) {
					continue
				}
				origintxsha := &txin.PreviousOutPoint.Hash

				exists, err := db.ExistsTxSha(origintxsha)
				if err != nil {
					t.Errorf("ExistsTxSha: unexpected error %v ", err)
				}
				if !exists {
					// Check and see if the outpoint references txtreeregular of
					// the previous block. If it does, make sure nothing in tx
					// treeregular spends it in flight. Then check make sure it's
					// not currently spent for this block. If it isn't, mark it
					// spent and skip lookup in the db below, since the db won't
					// yet be able to add it as it's still to be inserted.
					spentFromParentReg := false
					parent := blocks[height-1]
					parentValid := dcrutil.IsFlagSet16(dcrutil.BlockValid, block.MsgBlock().Header.VoteBits)
					if parentValid {
						for _, prtx := range parent.Transactions() {
							// Check and make sure it's not being spent in this tx
							// tree first by an in flight tx. Mark it spent if it
							// is so it fails the check below.
							for _, prtxCheck := range parent.Transactions() {
								for _, prTxIn := range prtxCheck.MsgTx().TxIn {
									if prTxIn.PreviousOutPoint == txin.PreviousOutPoint {
										spentFromParent[txin.PreviousOutPoint] = struct{}{}
									}
								}
							}

							// If it is in the tree, make sure it's not already spent
							// somewhere else and mark it spent. Set the flag below
							// so we skip lookup.
							if prtx.Sha().IsEqual(origintxsha) {
								if _, spent := spentFromParent[txin.PreviousOutPoint]; !spent {
									spentFromParent[txin.PreviousOutPoint] = struct{}{}
									spentFromParentReg = true
								}
							}
						}
					}

					if !spentFromParentReg {
						t.Errorf("referenced tx not found %v %v", origintxsha, height)
					} else {
						continue
					}
				}

				txInList = append(txInList, &txin.PreviousOutPoint)
				txneededList = append(txneededList, origintxsha)
				txlookupList = append(txlookupList, origintxsha)
			}
			txshaname := tx.TxSha()
			txlookupList = append(txlookupList, &txshaname)
			txOutList = append(txOutList, &txshaname)
		}

		txneededmap := map[chainhash.Hash]*database.TxListReply{}
		txlist := db.FetchUnSpentTxByShaList(txneededList)
		for _, txe := range txlist {
			if txe.Err != nil {
				t.Errorf("tx list fetch failed %v err %v", txe.Sha, txe.Err)
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
		// only check transactions if current block is valid
		txlookupmap := map[chainhash.Hash]*database.TxListReply{}
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
				t.Errorf("txin %v:%v is unspent %v", spend.Hash, spend.Index, itxe.TxSpent)
			}
		}
		for _, txo := range txOutList {
			itxe := txlookupmap[*txo]
			for i, spent := range itxe.TxSpent {
				if spent == true {
					t.Errorf("height: %v freshly inserted tx %v already spent %v", height, txo, i)
				}
			}

		}
		if len(txInList) == 0 {
			continue
		}
		dropblock := blocks[height-1]

		err = db.DropAfterBlockBySha(dropblock.Sha())
		if err != nil {
			t.Errorf("failed to drop block %v err %v", height, err)
			break endtest
		}

		txlookupmap = map[chainhash.Hash]*database.TxListReply{}
		txlist = db.FetchUnSpentTxByShaList(txlookupList)
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
				t.Errorf("txin %v:%v is unspent %v", spend.Hash, spend.Index, itxe.TxSpent)
			}
		}
		newheight, err = db.InsertBlock(block)
		if err != nil {
			t.Errorf("failed to insert block %v err %v", height, err)
			break endtest
		}
		txlookupmap = map[chainhash.Hash]*database.TxListReply{}
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
				t.Errorf("txin %v:%v is unspent %v", spend.Hash, spend.Index, itxe.TxSpent)
			}
		}
	}
}

// getRegTreeOpsSpentBeforeThisOp returns all outpoints spent before this one
// in the block's tx tree regular. used for checking vs in flight tx.
func getRegTreeOpsSpentBeforeThisOp(block *dcrutil.Block, idx int, txinIdx int) map[wire.OutPoint]struct{} {
	spentOps := make(map[wire.OutPoint]struct{})

	thisTx := block.Transactions()[idx]
	for i, txIn := range thisTx.MsgTx().TxIn {
		if i < txinIdx {
			spentOps[txIn.PreviousOutPoint] = struct{}{}
		}
	}

	for i, tx := range block.Transactions() {
		if i < idx {
			for _, txIn := range tx.MsgTx().TxIn {
				spentOps[txIn.PreviousOutPoint] = struct{}{}
			}
		}
	}

	return spentOps
}

// unspendInflightTxTree returns all outpoints spent that reference internal
// transactions in a TxTreeRegular.
func unspendInflightTxTree(block *dcrutil.Block) map[wire.OutPoint]struct{} {
	unspentOps := make(map[wire.OutPoint]struct{})
	allTxHashes := make(map[chainhash.Hash]struct{})
	for _, tx := range block.Transactions() {
		h := tx.Sha()
		allTxHashes[*h] = struct{}{}
	}

	for _, tx := range block.Transactions() {
		for _, txIn := range tx.MsgTx().TxIn {
			if _, isLocal := allTxHashes[txIn.PreviousOutPoint.Hash]; isLocal {
				unspentOps[txIn.PreviousOutPoint] = struct{}{}
			}
		}
	}

	return unspentOps
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

// insert every block in the test chain
// after each insert, fetch all the tx affected by the latest
// block and verify that the the tx is spent/unspent
// new tx should be fully unspent, referenced tx should have
// the associated txout set to spent.
// checks tx tree regular only
func testUnspentInsertRegTree(t *testing.T) {
	// Ignore db remove errors since it means we didn't have an old one.
	dbname := fmt.Sprintf("tstdbuspnt1")
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
	blocks := loadblocks(t)
endtest:
	for height := int64(0); height < int64(len(blocks))-1; height++ {
		block := blocks[height]

		// jam in genesis block
		if height == 0 {
			_, err := db.InsertBlock(block)
			if err != nil {
				t.Errorf("failed to insert block %v err %v", height, err)
				break endtest
			}
			continue
		}

		var txneededList []*chainhash.Hash
		var txlookupList []*chainhash.Hash
		var txOutList []*chainhash.Hash
		var txInList []*wire.OutPoint
		parent := blocks[height-1]
		unspentStakeOps := unspendStakeTxTree(block)

		// Check regular tree of parent and make sure it's ok
		for txIdx, tx := range parent.MsgBlock().Transactions {
			for txinIdx, txin := range tx.TxIn {
				if txin.PreviousOutPoint.Index == uint32(4294967295) {
					continue
				}

				origintxsha := &txin.PreviousOutPoint.Hash

				exists, err := db.ExistsTxSha(origintxsha)
				if err != nil {
					t.Errorf("ExistsTxSha: unexpected error %v ", err)
				}
				if !exists {
					// Check and see if something in flight spends it from this
					// tx tree. We can skip looking for this transaction OP
					// if that's the case.
					spentFromParentReg := false
					alreadySpentOps := getRegTreeOpsSpentBeforeThisOp(parent,
						txIdx, txinIdx)
					_, alreadySpent := alreadySpentOps[txin.PreviousOutPoint]
					if !alreadySpent {
						spentFromParentReg = true
					}

					if !spentFromParentReg {
						t.Errorf("referenced tx not found %v %v", origintxsha,
							height)
					} else {
						continue
					}
				}

				txInList = append(txInList, &txin.PreviousOutPoint)
				txneededList = append(txneededList, origintxsha)
				txlookupList = append(txlookupList, origintxsha)
			}
			txshaname := tx.TxSha()
			txlookupList = append(txlookupList, &txshaname)
			txOutList = append(txOutList, &txshaname)
		}

		txneededmap := map[chainhash.Hash]*database.TxListReply{}
		txlist := db.FetchUnSpentTxByShaList(txneededList)
		for _, txe := range txlist {
			if txe.Err != nil {
				t.Errorf("tx list fetch failed %v err %v", txe.Sha, txe.Err)
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
		// only check transactions if current block validates parent block
		if !dcrutil.IsFlagSet16(block.MsgBlock().Header.VoteBits, dcrutil.BlockValid) {
			continue
		}

		txlookupmap := map[chainhash.Hash]*database.TxListReply{}
		txlist = db.FetchTxByShaList(txlookupList)
		for _, txe := range txlist {
			if txe.Err != nil {
				t.Errorf("tx list fetch failed %v err %v (height %v)", txe.Sha,
					txe.Err, height)
				break endtest
			}
			txlookupmap[*txe.Sha] = txe
		}
		for _, spend := range txInList {
			itxe := txlookupmap[spend.Hash]
			if itxe.TxSpent[spend.Index] == false {
				t.Errorf("txin %v:%v is unspent %v", spend.Hash, spend.Index, itxe.TxSpent)
			}
		}

		alreadySpentOps := unspendInflightTxTree(parent)
		for _, txo := range txOutList {
			itxe := txlookupmap[*txo]
			for i, spent := range itxe.TxSpent {
				if spent == true {
					// If this was spent in flight, skip
					thisOP := wire.OutPoint{
						Hash:  *txo,
						Index: uint32(i),
						Tree:  dcrutil.TxTreeRegular,
					}
					_, alreadySpent := alreadySpentOps[thisOP]
					if alreadySpent {
						continue
					}

					// If it was spent in the stake tree it's actually unspent too
					_, wasSpentInStakeTree := unspentStakeOps[thisOP]
					if wasSpentInStakeTree {
						continue
					}

					t.Errorf("height: %v freshly inserted tx %v already spent %v", height, txo, i)
				}
			}

		}
		if len(txInList) == 0 {
			continue
		}
		dropblock := blocks[height-1]

		err = db.DropAfterBlockBySha(dropblock.Sha())
		if err != nil {
			t.Errorf("failed to drop block %v err %v", height, err)
			break endtest
		}

		txlookupmap = map[chainhash.Hash]*database.TxListReply{}
		txlist = db.FetchUnSpentTxByShaList(txlookupList)
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
				t.Errorf("txin %v:%v is unspent %v", spend.Hash, spend.Index, itxe.TxSpent)
			}
		}
		newheight, err = db.InsertBlock(block)
		if err != nil {
			t.Errorf("failed to insert block %v err %v", height, err)
			break endtest
		}
		txlookupmap = map[chainhash.Hash]*database.TxListReply{}
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
				t.Errorf("txin %v:%v is unspent %v", spend.Hash, spend.Index, itxe.TxSpent)
			}
		}
	}
}
