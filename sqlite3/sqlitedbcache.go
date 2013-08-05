// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package sqlite3

import (
	"bytes"
	"container/list"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"sync"
)

type txCache struct {
	maxcount int
	fifo     list.List
	// NOTE:  the key is specifically ShaHash, not *ShaHash
	txMap     map[btcwire.ShaHash]*txCacheObj
	cacheLock sync.RWMutex
}

type txCacheObj struct {
	next   *txCacheObj
	sha    btcwire.ShaHash
	blksha btcwire.ShaHash
	pver   uint32
	tx     *btcwire.MsgTx
	height int64
	spent  []byte
	txbuf  []byte
}

type blockCache struct {
	maxcount       int
	fifo           list.List
	blockMap       map[btcwire.ShaHash]*blockCacheObj
	blockHeightMap map[int64]*blockCacheObj
	cacheLock      sync.RWMutex
}

type blockCacheObj struct {
	next *blockCacheObj
	sha  btcwire.ShaHash
	blk  *btcutil.Block
}

// FetchBlockBySha - return a btcutil Block, object may be a cached.
func (db *SqliteDb) FetchBlockBySha(sha *btcwire.ShaHash) (blk *btcutil.Block, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()
	return db.fetchBlockBySha(sha)
}

// fetchBlockBySha - return a btcutil Block, object may be a cached.
// Must be called with db lock held.
func (db *SqliteDb) fetchBlockBySha(sha *btcwire.ShaHash) (blk *btcutil.Block, err error) {

	blkcache, ok := db.fetchBlockCache(sha)
	if ok {
		return blkcache.blk, nil
	}

	buf, _, height, err := db.fetchSha(*sha)
	if err != nil {
		return nil, err
	}

	blk, err = btcutil.NewBlockFromBytes(buf)
	if err != nil {
		return
	}
	blk.SetHeight(height)
	db.insertBlockCache(sha, blk)

	return
}

// fetchBlockCache check if a block is in the block cache, if so return it.
func (db *SqliteDb) fetchBlockCache(sha *btcwire.ShaHash) (*blockCacheObj, bool) {

	db.blockCache.cacheLock.RLock()
	defer db.blockCache.cacheLock.RUnlock()

	blkobj, ok := db.blockCache.blockMap[*sha]
	if !ok { // could this just return the map deref?
		return nil, false
	}
	return blkobj, true
}

// fetchBlockHeightCache check if a block is in the block cache, if so return it.
func (db *SqliteDb) fetchBlockHeightCache(height int64) (*blockCacheObj, bool) {

	db.blockCache.cacheLock.RLock()
	defer db.blockCache.cacheLock.RUnlock()

	blkobj, ok := db.blockCache.blockHeightMap[height]
	if !ok { // could this just return the map deref?
		return nil, false
	}
	return blkobj, true
}

// insertBlockCache insert the given sha/block into the cache map.
// If the block cache is determined to be full, it will release
// an old entry in FIFO order.
func (db *SqliteDb) insertBlockCache(sha *btcwire.ShaHash, blk *btcutil.Block) {
	bc := &db.blockCache

	bc.cacheLock.Lock()
	defer bc.cacheLock.Unlock()

	blkObj := blockCacheObj{sha: *sha, blk: blk}
	bc.fifo.PushBack(&blkObj)

	if bc.fifo.Len() > bc.maxcount {
		listobj := bc.fifo.Front()
		bc.fifo.Remove(listobj)
		tailObj, ok := listobj.Value.(*blockCacheObj)
		if ok {
			delete(bc.blockMap, tailObj.sha)
			delete(bc.blockHeightMap, tailObj.blk.Height())
		} else {
			panic("invalid type pushed on blockCache list")
		}
	}

	bc.blockHeightMap[blk.Height()] = &blkObj
	bc.blockMap[blkObj.sha] = &blkObj
}

// FetchTxByShaList given a array of ShaHash, look up the transactions
// and return them in a TxListReply array.
func (db *SqliteDb) FetchTxByShaList(txShaList []*btcwire.ShaHash) []*btcdb.TxListReply {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	var replies []*btcdb.TxListReply
	for _, txsha := range txShaList {
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
		replies = append(replies, &txlre)
	}
	return replies
}

// fetchTxDataBySha returns several pieces of data regarding the given sha.
func (db *SqliteDb) fetchTxDataBySha(txsha *btcwire.ShaHash) (rtx *btcwire.MsgTx, rtxbuf []byte, rpver uint32, rblksha *btcwire.ShaHash, rheight int64, rtxspent []byte, err error) {

	var pver uint32
	var blksha *btcwire.ShaHash
	var height int64
	var txspent []byte
	var toff int
	var tlen int
	var blk *btcutil.Block
	var blkbuf []byte

	// Check Tx cache
	if txc, ok := db.fetchTxCache(txsha); ok {
		if txc.spent != nil {
			return txc.tx, txc.txbuf, txc.pver, &txc.blksha, txc.height, txc.spent, nil
		}
	}

	// If not cached load it
	height, toff, tlen, txspent, err = db.fetchLocationUsedBySha(txsha)
	if err != nil {
		return
	}

	blksha, err = db.fetchBlockShaByHeight(height)
	if err != nil {
		log.Warnf("block idx lookup %v to %v", height, err)
		return
	}
	log.Tracef("transaction %v is at block %v %v tx %v",
		txsha, blksha, height, toff)

	blk, err = db.fetchBlockBySha(blksha)
	if err != nil {
		log.Warnf("unable to fetch block %v %v ",
			height, &blksha)
		return
	}

	blkbuf, err = blk.Bytes()
	if err != nil {
		log.Warnf("unable to decode block %v %v", height, &blksha)
		return
	}

	txbuf := make([]byte, tlen)
	copy(txbuf[:], blkbuf[toff:toff+tlen])
	rbuf := bytes.NewBuffer(txbuf)

	var tx btcwire.MsgTx
	err = tx.BtcDecode(rbuf, pver)
	if err != nil {
		log.Warnf("unable to decode tx block %v %v txoff %v txlen %v",
			height, &blksha, toff, tlen)
		return
	}

	// Shove data into TxCache
	// XXX -
	var txc txCacheObj
	txc.sha = *txsha
	txc.tx = &tx
	txc.txbuf = txbuf
	txc.pver = pver
	txc.height = height
	txc.spent = txspent
	txc.blksha = *blksha
	db.insertTxCache(&txc)

	return &tx, txbuf, pver, blksha, height, txspent, nil
}

// FetchTxAllBySha returns several pieces of data regarding the given sha.
func (db *SqliteDb) FetchTxAllBySha(txsha *btcwire.ShaHash) (rtx *btcwire.MsgTx, rtxbuf []byte, rpver uint32, rblksha *btcwire.ShaHash, err error) {
	var pver uint32
	var blksha *btcwire.ShaHash
	var height int64
	var toff int
	var tlen int
	var blk *btcutil.Block
	var blkbuf []byte

	// Check Tx cache
	if txc, ok := db.fetchTxCache(txsha); ok {
		return txc.tx, txc.txbuf, txc.pver, &txc.blksha, nil
	}

	// If not cached load it
	height, toff, tlen, err = db.FetchLocationBySha(txsha)
	if err != nil {
		return
	}

	blksha, err = db.FetchBlockShaByHeight(height)
	if err != nil {
		log.Warnf("block idx lookup %v to %v", height, err)
		return
	}
	log.Tracef("transaction %v is at block %v %v tx %v",
		txsha, blksha, height, toff)

	blk, err = db.FetchBlockBySha(blksha)
	if err != nil {
		log.Warnf("unable to fetch block %v %v ",
			height, &blksha)
		return
	}

	blkbuf, err = blk.Bytes()
	if err != nil {
		log.Warnf("unable to decode block %v %v", height, &blksha)
		return
	}

	txbuf := make([]byte, tlen)
	copy(txbuf[:], blkbuf[toff:toff+tlen])
	rbuf := bytes.NewBuffer(txbuf)

	var tx btcwire.MsgTx
	err = tx.BtcDecode(rbuf, pver)
	if err != nil {
		log.Warnf("unable to decode tx block %v %v txoff %v txlen %v",
			height, &blksha, toff, tlen)
		return
	}

	// Shove data into TxCache
	// XXX -
	var txc txCacheObj
	txc.sha = *txsha
	txc.tx = &tx
	txc.txbuf = txbuf
	txc.pver = pver
	txc.height = height
	txc.blksha = *blksha
	db.insertTxCache(&txc)

	return &tx, txbuf, pver, blksha, nil
}

// FetchTxBySha returns some data for the given Tx Sha.
func (db *SqliteDb) FetchTxBySha(txsha *btcwire.ShaHash) (rtx *btcwire.MsgTx, rpver uint32, blksha *btcwire.ShaHash, err error) {
	rtx, _, rpver, blksha, err = db.FetchTxAllBySha(txsha)
	return
}

// FetchTxBufBySha return the bytestream data and associated protocol version.
// for the given Tx Sha
func (db *SqliteDb) FetchTxBufBySha(txsha *btcwire.ShaHash) (txbuf []byte, rpver uint32, err error) {
	_, txbuf, rpver, _, err = db.FetchTxAllBySha(txsha)
	return
}

// fetchTxCache look up the given transaction in the Tx cache.
func (db *SqliteDb) fetchTxCache(sha *btcwire.ShaHash) (*txCacheObj, bool) {
	tc := &db.txCache

	tc.cacheLock.RLock()
	defer tc.cacheLock.RUnlock()

	txObj, ok := tc.txMap[*sha]
	if !ok { // could this just return the map deref?
		return nil, false
	}
	return txObj, true
}

// insertTxCache, insert the given txobj into the cache.
// if the tx cache is determined to be full, it will release
// an old entry in FIFO order.
func (db *SqliteDb) insertTxCache(txObj *txCacheObj) {
	tc := &db.txCache

	tc.cacheLock.Lock()
	defer tc.cacheLock.Unlock()

	tc.fifo.PushBack(txObj)

	if tc.fifo.Len() >= tc.maxcount {
		listobj := tc.fifo.Front()
		tc.fifo.Remove(listobj)
		tailObj, ok := listobj.Value.(*txCacheObj)
		if ok {
			delete(tc.txMap, tailObj.sha)
		} else {
			panic("invalid type pushed on tx list")
		}

	}

	tc.txMap[txObj.sha] = txObj
}

// InvalidateTxCache clear/release all cached transactions.
func (db *SqliteDb) InvalidateTxCache() {
	tc := &db.txCache
	tc.cacheLock.Lock()
	defer tc.cacheLock.Unlock()
	tc.txMap = map[btcwire.ShaHash]*txCacheObj{}
	tc.fifo = list.List{}
}

// InvalidateTxCache clear/release all cached blocks.
func (db *SqliteDb) InvalidateBlockCache() {
	bc := &db.blockCache
	bc.cacheLock.Lock()
	defer bc.cacheLock.Unlock()
	bc.blockMap = map[btcwire.ShaHash]*blockCacheObj{}
	bc.blockHeightMap = map[int64]*blockCacheObj{}
	bc.fifo = list.List{}
}

// InvalidateCache clear/release all cached blocks and transactions.
func (db *SqliteDb) InvalidateCache() {
	db.InvalidateTxCache()
	db.InvalidateBlockCache()
}
