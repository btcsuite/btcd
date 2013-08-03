// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	//"github.com/conformal/btcdb"
	"github.com/conformal/btcwire"
)

type txUpdateObj struct {
	txSha     *btcwire.ShaHash
	blkHeight int64
	txoff     int
	txlen     int
	spentData []byte
	delete    bool
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
		fmt.Printf("fail encoding blkHeight %v\n", err)
		err = fmt.Errorf("Write fail")
		return nil, err
	}

	err = binary.Write(&txW, binary.LittleEndian, txOff)
	if err != nil {
		fmt.Printf("fail encoding txoff %v\n", err)
		err = fmt.Errorf("Write fail")
		return nil, err
	}

	err = binary.Write(&txW, binary.LittleEndian, txLen)
	if err != nil {
		fmt.Printf("fail encoding txlen %v\n", err)
		err = fmt.Errorf("Write fail")
		return nil, err
	}

	err = binary.Write(&txW, binary.LittleEndian, spentbuf)
	if err != nil {
		fmt.Printf("fail encoding spentbuf %v\n", err)
		err = fmt.Errorf("Write fail")
		return nil, err
	}

	return txW.Bytes(), nil
}

func (db *LevelDb) getTxData(txsha *btcwire.ShaHash) (rblkHeight int64,
	rtxOff int, rtxLen int, rspentBuf []byte, err error) {
	var buf []byte

	key := shaToKey(txsha)
	buf, err = db.tShaDb.Get(key, db.ro)
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
		fmt.Printf("fail encoding spentbuf %v\n", err)
		err = fmt.Errorf("Db Corrupt 4")
		return
	}
	return blkHeight, int(txOff), int(txLen), spentBuf, nil
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

// FetchLocationBySha looks up the Tx sha information by name.
func (db *LevelDb) FetchLocationBySha(txsha *btcwire.ShaHash) (blockidx int64, txoff int, txlen int, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()
	err = fmt.Errorf("obsolete function")
	return
}

// FetchTxUsedBySha returns the used/spent buffer for a given transaction.
func (db *LevelDb) FetchTxUsedBySha(txSha *btcwire.ShaHash) (spentbuf []byte, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	_, _, _, spentbuf, err = db.getTxData(txSha)
	if err != nil {
		return
	}
	return // spentbuf has the value already
}

func (db *LevelDb) fetchLocationUsedBySha(txsha *btcwire.ShaHash) error {
	// delete me
	return fmt.Errorf("Deleted function")
}
