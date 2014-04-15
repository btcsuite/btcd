// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcwire"
	"github.com/conformal/goleveldb/leveldb"
)

type txUpdateObj struct {
	txSha     *btcwire.ShaHash
	blkHeight int64
	txoff     int
	txlen     int
	ntxout    int
	spentData []byte
	delete    bool
}

type spentTx struct {
	blkHeight int64
	txoff     int
	txlen     int
	numTxO    int
	delete    bool
}
type spentTxUpdate struct {
	txl    []*spentTx
	delete bool
}

// InsertTx inserts a tx hash and its associated data into the database.
func (db *LevelDb) InsertTx(txsha *btcwire.ShaHash, height int64, txoff int, txlen int, spentbuf []byte) (err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	return db.insertTx(txsha, height, txoff, txlen, spentbuf)
}

// insertTx inserts a tx hash and its associated data into the database.
// Must be called with db lock held.
func (db *LevelDb) insertTx(txSha *btcwire.ShaHash, height int64, txoff int, txlen int, spentbuf []byte) (err error) {
	var txU txUpdateObj

	txU.txSha = txSha
	txU.blkHeight = height
	txU.txoff = txoff
	txU.txlen = txlen
	txU.spentData = spentbuf

	db.txUpdateMap[*txSha] = &txU

	return nil
}

// formatTx generates the value buffer for the Tx db.
func (db *LevelDb) formatTx(txu *txUpdateObj) ([]byte, error) {

	blkHeight := txu.blkHeight
	txoff := txu.txoff
	txlen := txu.txlen
	spentbuf := txu.spentData

	txOff := int32(txoff)
	txLen := int32(txlen)

	var txW bytes.Buffer

	err := binary.Write(&txW, binary.LittleEndian, blkHeight)
	if err != nil {
		err = fmt.Errorf("Write fail")
		return nil, err
	}

	err = binary.Write(&txW, binary.LittleEndian, txOff)
	if err != nil {
		err = fmt.Errorf("Write fail")
		return nil, err
	}

	err = binary.Write(&txW, binary.LittleEndian, txLen)
	if err != nil {
		err = fmt.Errorf("Write fail")
		return nil, err
	}

	err = binary.Write(&txW, binary.LittleEndian, spentbuf)
	if err != nil {
		err = fmt.Errorf("Write fail")
		return nil, err
	}

	return txW.Bytes(), nil
}

func (db *LevelDb) getTxData(txsha *btcwire.ShaHash) (rblkHeight int64,
	rtxOff int, rtxLen int, rspentBuf []byte, err error) {
	var buf []byte

	key := shaTxToKey(txsha)
	buf, err = db.lDb.Get(key, db.ro)
	if err != nil {
		return
	}

	var blkHeight int64
	var txOff, txLen int32
	dr := bytes.NewBuffer(buf)
	err = binary.Read(dr, binary.LittleEndian, &blkHeight)
	if err != nil {
		err = fmt.Errorf("Db Corrupt 1")
		return
	}
	err = binary.Read(dr, binary.LittleEndian, &txOff)
	if err != nil {
		err = fmt.Errorf("Db Corrupt 2")
		return
	}
	err = binary.Read(dr, binary.LittleEndian, &txLen)
	if err != nil {
		err = fmt.Errorf("Db Corrupt 3")
		return
	}
	// remainder of buffer is spentbuf
	spentBuf := make([]byte, dr.Len())
	err = binary.Read(dr, binary.LittleEndian, spentBuf)
	if err != nil {
		err = fmt.Errorf("Db Corrupt 4")
		return
	}
	return blkHeight, int(txOff), int(txLen), spentBuf, nil
}

func (db *LevelDb) getTxFullySpent(txsha *btcwire.ShaHash) ([]*spentTx, error) {

	var badTxList, spentTxList []*spentTx

	key := shaSpentTxToKey(txsha)
	buf, err := db.lDb.Get(key, db.ro)
	if err == leveldb.ErrNotFound {
		return badTxList, btcdb.TxShaMissing
	} else if err != nil {
		return badTxList, err
	}
	txListLen := len(buf) / 20
	txR := bytes.NewBuffer(buf)
	spentTxList = make([]*spentTx, txListLen, txListLen)

	for i := range spentTxList {
		var sTx spentTx
		var blkHeight int64
		var txOff, txLen, numTxO int32

		err := binary.Read(txR, binary.LittleEndian, &blkHeight)
		if err != nil {
			err = fmt.Errorf("sTx Read fail 0")
			return nil, err
		}
		sTx.blkHeight = blkHeight

		err = binary.Read(txR, binary.LittleEndian, &txOff)
		if err != nil {
			err = fmt.Errorf("sTx Read fail 1")
			return nil, err
		}
		sTx.txoff = int(txOff)

		err = binary.Read(txR, binary.LittleEndian, &txLen)
		if err != nil {
			err = fmt.Errorf("sTx Read fail 2")
			return nil, err
		}
		sTx.txlen = int(txLen)

		err = binary.Read(txR, binary.LittleEndian, &numTxO)
		if err != nil {
			err = fmt.Errorf("sTx Read fail 3")
			return nil, err
		}
		sTx.numTxO = int(numTxO)

		spentTxList[i] = &sTx
	}

	return spentTxList, nil
}

func (db *LevelDb) formatTxFullySpent(sTxList []*spentTx) ([]byte, error) {
	var txW bytes.Buffer

	for _, sTx := range sTxList {
		blkHeight := sTx.blkHeight
		txOff := int32(sTx.txoff)
		txLen := int32(sTx.txlen)
		numTxO := int32(sTx.numTxO)

		err := binary.Write(&txW, binary.LittleEndian, blkHeight)
		if err != nil {
			err = fmt.Errorf("Write fail")
			return nil, err
		}

		err = binary.Write(&txW, binary.LittleEndian, txOff)
		if err != nil {
			err = fmt.Errorf("Write fail")
			return nil, err
		}

		err = binary.Write(&txW, binary.LittleEndian, txLen)
		if err != nil {
			err = fmt.Errorf("Write fail")
			return nil, err
		}

		err = binary.Write(&txW, binary.LittleEndian, numTxO)
		if err != nil {
			err = fmt.Errorf("Write fail")
			return nil, err
		}
	}

	return txW.Bytes(), nil
}

// ExistsTxSha returns if the given tx sha exists in the database
func (db *LevelDb) ExistsTxSha(txsha *btcwire.ShaHash) (exists bool) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	return db.existsTxSha(txsha)
}

// existsTxSha returns if the given tx sha exists in the database.o
// Must be called with the db lock held.
func (db *LevelDb) existsTxSha(txSha *btcwire.ShaHash) (exists bool) {
	_, _, _, _, err := db.getTxData(txSha)
	if err == nil {
		return true
	}

	// BUG(drahn) If there was an error beside non-existant deal with it.

	return false
}

// FetchTxByShaList returns the most recent tx of the name fully spent or not
func (db *LevelDb) FetchTxByShaList(txShaList []*btcwire.ShaHash) []*btcdb.TxListReply {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	// until the fully spent separation of tx is complete this is identical
	// to FetchUnSpentTxByShaList
	replies := make([]*btcdb.TxListReply, len(txShaList))
	for i, txsha := range txShaList {
		tx, blockSha, height, txspent, err := db.fetchTxDataBySha(txsha)
		btxspent := []bool{}
		if err == nil {
			btxspent = make([]bool, len(tx.TxOut), len(tx.TxOut))
			for idx := range tx.TxOut {
				byteidx := idx / 8
				byteoff := uint(idx % 8)
				btxspent[idx] = (txspent[byteidx] & (byte(1) << byteoff)) != 0
			}
		}
		if err == btcdb.TxShaMissing {
			// if the unspent pool did not have the tx,
			// look in the fully spent pool (only last instance

			sTxList, fSerr := db.getTxFullySpent(txsha)
			if fSerr == nil && len(sTxList) != 0 {
				idx := len(sTxList) - 1
				stx := sTxList[idx]

				tx, blockSha, _, _, err = db.fetchTxDataByLoc(
					stx.blkHeight, stx.txoff, stx.txlen, []byte{})
				if err == nil {
					btxspent = make([]bool, len(tx.TxOut))
					for i := range btxspent {
						btxspent[i] = true
					}
				}
			}
		}
		txlre := btcdb.TxListReply{Sha: txsha, Tx: tx, BlkSha: blockSha, Height: height, TxSpent: btxspent, Err: err}
		replies[i] = &txlre
	}
	return replies
}

// FetchUnSpentTxByShaList given a array of ShaHash, look up the transactions
// and return them in a TxListReply array.
func (db *LevelDb) FetchUnSpentTxByShaList(txShaList []*btcwire.ShaHash) []*btcdb.TxListReply {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	replies := make([]*btcdb.TxListReply, len(txShaList))
	for i, txsha := range txShaList {
		tx, blockSha, height, txspent, err := db.fetchTxDataBySha(txsha)
		btxspent := []bool{}
		if err == nil {
			btxspent = make([]bool, len(tx.TxOut), len(tx.TxOut))
			for idx := range tx.TxOut {
				byteidx := idx / 8
				byteoff := uint(idx % 8)
				btxspent[idx] = (txspent[byteidx] & (byte(1) << byteoff)) != 0
			}
		}
		txlre := btcdb.TxListReply{Sha: txsha, Tx: tx, BlkSha: blockSha, Height: height, TxSpent: btxspent, Err: err}
		replies[i] = &txlre
	}
	return replies
}

// fetchTxDataBySha returns several pieces of data regarding the given sha.
func (db *LevelDb) fetchTxDataBySha(txsha *btcwire.ShaHash) (rtx *btcwire.MsgTx, rblksha *btcwire.ShaHash, rheight int64, rtxspent []byte, err error) {
	var blkHeight int64
	var txspent []byte
	var txOff, txLen int

	blkHeight, txOff, txLen, txspent, err = db.getTxData(txsha)
	if err != nil {
		if err == leveldb.ErrNotFound {
			err = btcdb.TxShaMissing
		}
		return
	}
	return db.fetchTxDataByLoc(blkHeight, txOff, txLen, txspent)
}

// fetchTxDataByLoc returns several pieces of data regarding the given tx
// located by the block/offset/size location
func (db *LevelDb) fetchTxDataByLoc(blkHeight int64, txOff int, txLen int, txspent []byte) (rtx *btcwire.MsgTx, rblksha *btcwire.ShaHash, rheight int64, rtxspent []byte, err error) {
	var blksha *btcwire.ShaHash
	var blkbuf []byte

	blksha, blkbuf, err = db.getBlkByHeight(blkHeight)
	if err != nil {
		if err == leveldb.ErrNotFound {
			err = btcdb.TxShaMissing
		}
		return
	}

	//log.Trace("transaction %v is at block %v %v txoff %v, txlen %v\n",
	//	txsha, blksha, blkHeight, txOff, txLen)

	if len(blkbuf) < txOff+txLen {
		err = btcdb.TxShaMissing
		return
	}
	rbuf := bytes.NewBuffer(blkbuf[txOff : txOff+txLen])

	var tx btcwire.MsgTx
	err = tx.Deserialize(rbuf)
	if err != nil {
		log.Warnf("unable to decode tx block %v %v txoff %v txlen %v",
			blkHeight, blksha, txOff, txLen)
		return
	}

	return &tx, blksha, blkHeight, txspent, nil
}

// FetchTxBySha returns some data for the given Tx Sha.
func (db *LevelDb) FetchTxBySha(txsha *btcwire.ShaHash) ([]*btcdb.TxListReply, error) {
	replylen := 0
	replycnt := 0

	tx, blksha, height, txspent, txerr := db.fetchTxDataBySha(txsha)
	if txerr == nil {
		replylen++
	} else {
		if txerr != btcdb.TxShaMissing {
			return []*btcdb.TxListReply{}, txerr
		}
	}

	sTxList, fSerr := db.getTxFullySpent(txsha)

	if fSerr != nil {
		if fSerr != btcdb.TxShaMissing {
			return []*btcdb.TxListReply{}, fSerr
		}
	} else {
		replylen += len(sTxList)
	}

	replies := make([]*btcdb.TxListReply, replylen)

	if fSerr == nil {
		for _, stx := range sTxList {
			tx, blksha, _, _, err := db.fetchTxDataByLoc(
				stx.blkHeight, stx.txoff, stx.txlen, []byte{})
			if err != nil {
				if err != leveldb.ErrNotFound {
					return []*btcdb.TxListReply{}, err
				}
				continue
			}
			btxspent := make([]bool, len(tx.TxOut), len(tx.TxOut))
			for i := range btxspent {
				btxspent[i] = true
			}
			txlre := btcdb.TxListReply{Sha: txsha, Tx: tx, BlkSha: blksha, Height: stx.blkHeight, TxSpent: btxspent, Err: nil}
			replies[replycnt] = &txlre
			replycnt++
		}
	}
	if txerr == nil {
		btxspent := make([]bool, len(tx.TxOut), len(tx.TxOut))
		for idx := range tx.TxOut {
			byteidx := idx / 8
			byteoff := uint(idx % 8)
			btxspent[idx] = (txspent[byteidx] & (byte(1) << byteoff)) != 0
		}
		txlre := btcdb.TxListReply{Sha: txsha, Tx: tx, BlkSha: blksha, Height: height, TxSpent: btxspent, Err: nil}
		replies[replycnt] = &txlre
		replycnt++
	}
	return replies, nil
}
