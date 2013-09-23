// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb

import (
	"bytes"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// FetchBlockBySha - return a btcutil Block, object may be a cached.
func (db *LevelDb) FetchBlockBySha(sha *btcwire.ShaHash) (blk *btcutil.Block, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()
	return db.fetchBlockBySha(sha)
}

// fetchBlockBySha - return a btcutil Block, object may be a cached.
// Must be called with db lock held.
func (db *LevelDb) fetchBlockBySha(sha *btcwire.ShaHash) (blk *btcutil.Block, err error) {

	buf, height, err := db.fetchSha(sha)
	if err != nil {
		return
	}

	blk, err = btcutil.NewBlockFromBytes(buf)
	if err != nil {
		return
	}
	blk.SetHeight(height)

	return
}

// FetchTxByShaList given a array of ShaHash, look up the transactions
// and return them in a TxListReply array.
func (db *LevelDb) FetchTxByShaList(txShaList []*btcwire.ShaHash) []*btcdb.TxListReply {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	replies := make([]*btcdb.TxListReply, len(txShaList))
	for i, txsha := range txShaList {
		tx, _, _, _, height, txspent, err := db.fetchTxDataBySha(txsha)
		btxspent := []bool{}
		if err == nil {
			btxspent = make([]bool, len(tx.TxOut), len(tx.TxOut))
			for idx := range tx.TxOut {
				byteidx := idx / 8
				byteoff := uint(idx % 8)
				btxspent[idx] = (txspent[byteidx] & (byte(1) << byteoff)) != 0
			}
		}
		txlre := btcdb.TxListReply{Sha: txsha, Tx: tx, Height: height, TxSpent: btxspent, Err: err}
		replies[i] = &txlre
	}
	return replies
}

// fetchTxDataBySha returns several pieces of data regarding the given sha.
func (db *LevelDb) fetchTxDataBySha(txsha *btcwire.ShaHash) (rtx *btcwire.MsgTx, rtxbuf []byte, rpver uint32, rblksha *btcwire.ShaHash, rheight int64, rtxspent []byte, err error) {
	var pver uint32
	var blksha *btcwire.ShaHash
	var blkHeight int64
	var txspent []byte
	var txOff, txLen int
	var blkbuf []byte

	blkHeight, txOff, txLen, txspent, err = db.getTxData(txsha)
	if err != nil {
		err = btcdb.TxShaMissing
		return
	}

	blksha, blkbuf, err = db.getBlkByHeight(blkHeight)
	if err != nil {
		return
	}

	//log.Trace("transaction %v is at block %v %v txoff %v, txlen %v\n",
	//	txsha, blksha, blkHeight, txOff, txLen)

	txbuf := make([]byte, txLen)
	copy(txbuf[:], blkbuf[txOff:txOff+txLen])
	rbuf := bytes.NewBuffer(txbuf)

	var tx btcwire.MsgTx
	err = tx.Deserialize(rbuf)
	if err != nil {
		log.Warnf("unable to decode tx block %v %v txoff %v txlen %v",
			blkHeight, blksha, txOff, txLen)
		return
	}

	return &tx, txbuf, pver, blksha, blkHeight, txspent, nil
}

// FetchTxAllBySha returns several pieces of data regarding the given sha.
func (db *LevelDb) FetchTxAllBySha(txsha *btcwire.ShaHash) (rtx *btcwire.MsgTx, rtxbuf []byte, rpver uint32, rblksha *btcwire.ShaHash, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	tx, txbuf, pver, blksha, _, _, err := db.fetchTxDataBySha(txsha)

	return tx, txbuf, pver, blksha, err
}

// FetchTxBySha returns some data for the given Tx Sha.
func (db *LevelDb) FetchTxBySha(txsha *btcwire.ShaHash) (rtx *btcwire.MsgTx, rpver uint32, blksha *btcwire.ShaHash, err error) {
	rtx, _, rpver, blksha, err = db.FetchTxAllBySha(txsha)
	return
}

// FetchTxBufBySha return the bytestream data and associated protocol version.
// for the given Tx Sha
func (db *LevelDb) FetchTxBufBySha(txsha *btcwire.ShaHash) (txbuf []byte, rpver uint32, err error) {
	_, txbuf, rpver, _, err = db.FetchTxAllBySha(txsha)
	return
}

// InvalidateTxCache clear/release all cached transactions.
func (db *LevelDb) InvalidateTxCache() {
}

// InvalidateTxCache clear/release all cached blocks.
func (db *LevelDb) InvalidateBlockCache() {
}

// InvalidateCache clear/release all cached blocks and transactions.
func (db *LevelDb) InvalidateCache() {
}
