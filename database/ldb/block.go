// Copyright (c) 2013-2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb

import (
	"bytes"
	"encoding/binary"

	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/goleveldb/leveldb"
)

// FetchBlockBySha - return a btcutil Block
func (db *LevelDb) FetchBlockBySha(sha *wire.ShaHash) (blk *btcutil.Block, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()
	return db.fetchBlockBySha(sha)
}

// fetchBlockBySha - return a btcutil Block
// Must be called with db lock held.
func (db *LevelDb) fetchBlockBySha(sha *wire.ShaHash) (blk *btcutil.Block, err error) {

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

// FetchBlockHeightBySha returns the block height for the given hash.  This is
// part of the database.Db interface implementation.
func (db *LevelDb) FetchBlockHeightBySha(sha *wire.ShaHash) (int64, error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	return db.getBlkLoc(sha)
}

// FetchBlockHeaderBySha - return a ShaHash
func (db *LevelDb) FetchBlockHeaderBySha(sha *wire.ShaHash) (bh *wire.BlockHeader, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	// Read the raw block from the database.
	buf, _, err := db.fetchSha(sha)
	if err != nil {
		return nil, err
	}

	// Only deserialize the header portion and ensure the transaction count
	// is zero since this is a standalone header.
	var blockHeader wire.BlockHeader
	err = blockHeader.Deserialize(bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	bh = &blockHeader

	return bh, err
}

func (db *LevelDb) getBlkLoc(sha *wire.ShaHash) (int64, error) {
	key := shaBlkToKey(sha)

	data, err := db.lDb.Get(key, db.ro)
	if err != nil {
		if err == leveldb.ErrNotFound {
			err = database.ErrBlockShaMissing
		}
		return 0, err
	}

	// deserialize
	blkHeight := binary.LittleEndian.Uint64(data)

	return int64(blkHeight), nil
}

func (db *LevelDb) getBlkByHeight(blkHeight int64) (rsha *wire.ShaHash, rbuf []byte, err error) {
	var blkVal []byte

	key := int64ToKey(blkHeight)

	blkVal, err = db.lDb.Get(key, db.ro)
	if err != nil {
		log.Tracef("failed to find height %v", blkHeight)
		return // exists ???
	}

	var sha wire.ShaHash

	sha.SetBytes(blkVal[0:32])

	blockdata := make([]byte, len(blkVal[32:]))
	copy(blockdata[:], blkVal[32:])

	return &sha, blockdata, nil
}

func (db *LevelDb) getBlk(sha *wire.ShaHash) (rblkHeight int64, rbuf []byte, err error) {
	var blkHeight int64

	blkHeight, err = db.getBlkLoc(sha)
	if err != nil {
		return
	}

	var buf []byte

	_, buf, err = db.getBlkByHeight(blkHeight)
	if err != nil {
		return
	}
	return blkHeight, buf, nil
}

func (db *LevelDb) setBlk(sha *wire.ShaHash, blkHeight int64, buf []byte) {
	// serialize
	var lw [8]byte
	binary.LittleEndian.PutUint64(lw[0:8], uint64(blkHeight))

	shaKey := shaBlkToKey(sha)
	blkKey := int64ToKey(blkHeight)

	blkVal := make([]byte, len(sha)+len(buf))
	copy(blkVal[0:], sha[:])
	copy(blkVal[len(sha):], buf)

	db.lBatch().Put(shaKey, lw[:])
	db.lBatch().Put(blkKey, blkVal)
}

// insertSha stores a block hash and its associated data block with a
// previous sha of `prevSha'.
// insertSha shall be called with db lock held
func (db *LevelDb) insertBlockData(sha *wire.ShaHash, prevSha *wire.ShaHash, buf []byte) (int64, error) {
	oBlkHeight, err := db.getBlkLoc(prevSha)
	if err != nil {
		// check current block count
		// if count != 0  {
		//	err = database.PrevShaMissing
		//	return
		// }
		oBlkHeight = -1
		if db.nextBlock != 0 {
			return 0, err
		}
	}

	// TODO(drahn) check curfile filesize, increment curfile if this puts it over
	blkHeight := oBlkHeight + 1

	db.setBlk(sha, blkHeight, buf)

	// update the last block cache
	db.lastBlkShaCached = true
	db.lastBlkSha = *sha
	db.lastBlkIdx = blkHeight
	db.nextBlock = blkHeight + 1

	return blkHeight, nil
}

// fetchSha returns the datablock for the given ShaHash.
func (db *LevelDb) fetchSha(sha *wire.ShaHash) (rbuf []byte,
	rblkHeight int64, err error) {
	var blkHeight int64
	var buf []byte

	blkHeight, buf, err = db.getBlk(sha)
	if err != nil {
		return
	}

	return buf, blkHeight, nil
}

// ExistsSha looks up the given block hash
// returns true if it is present in the database.
func (db *LevelDb) ExistsSha(sha *wire.ShaHash) (bool, error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	// not in cache, try database
	return db.blkExistsSha(sha)
}

// blkExistsSha looks up the given block hash
// returns true if it is present in the database.
// CALLED WITH LOCK HELD
func (db *LevelDb) blkExistsSha(sha *wire.ShaHash) (bool, error) {
	key := shaBlkToKey(sha)

	return db.lDb.Has(key, db.ro)
}

// FetchBlockShaByHeight returns a block hash based on its height in the
// block chain.
func (db *LevelDb) FetchBlockShaByHeight(height int64) (sha *wire.ShaHash, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	return db.fetchBlockShaByHeight(height)
}

// fetchBlockShaByHeight returns a block hash based on its height in the
// block chain.
func (db *LevelDb) fetchBlockShaByHeight(height int64) (rsha *wire.ShaHash, err error) {
	key := int64ToKey(height)

	blkVal, err := db.lDb.Get(key, db.ro)
	if err != nil {
		log.Tracef("failed to find height %v", height)
		return // exists ???
	}

	var sha wire.ShaHash
	sha.SetBytes(blkVal[0:32])

	return &sha, nil
}

// FetchHeightRange looks up a range of blocks by the start and ending
// heights.  Fetch is inclusive of the start height and exclusive of the
// ending height. To fetch all hashes from the start height until no
// more are present, use the special id `AllShas'.
func (db *LevelDb) FetchHeightRange(startHeight, endHeight int64) (rshalist []wire.ShaHash, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	var endidx int64
	if endHeight == database.AllShas {
		endidx = startHeight + 500
	} else {
		endidx = endHeight
	}

	shalist := make([]wire.ShaHash, 0, endidx-startHeight)
	for height := startHeight; height < endidx; height++ {
		// TODO(drahn) fix blkFile from height

		key := int64ToKey(height)
		blkVal, lerr := db.lDb.Get(key, db.ro)
		if lerr != nil {
			break
		}

		var sha wire.ShaHash
		sha.SetBytes(blkVal[0:32])
		shalist = append(shalist, sha)
	}

	if err != nil {
		return
	}
	//log.Tracef("FetchIdxRange idx %v %v returned %v shas err %v", startHeight, endHeight, len(shalist), err)

	return shalist, nil
}

// NewestSha returns the hash and block height of the most recent (end) block of
// the block chain.  It will return the zero hash, -1 for the block height, and
// no error (nil) if there are not any blocks in the database yet.
func (db *LevelDb) NewestSha() (rsha *wire.ShaHash, rblkid int64, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	if db.lastBlkIdx == -1 {
		return &wire.ShaHash{}, -1, nil
	}
	sha := db.lastBlkSha

	return &sha, db.lastBlkIdx, nil
}

// checkAddrIndexVersion returns an error if the address index version stored
// in the database is less than the current version, or if it doesn't exist.
// This function is used on startup to signal OpenDB to drop the address index
// if it's in an old, incompatible format.
func (db *LevelDb) checkAddrIndexVersion() error {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	data, err := db.lDb.Get(addrIndexVersionKey, db.ro)
	if err != nil {
		return database.ErrAddrIndexDoesNotExist
	}

	indexVersion := binary.LittleEndian.Uint16(data)

	if indexVersion != uint16(addrIndexCurrentVersion) {
		return database.ErrAddrIndexDoesNotExist
	}

	return nil
}

// fetchAddrIndexTip returns the last block height and block sha to be indexed.
// Meta-data about the address tip is currently cached in memory, and will be
// updated accordingly by functions that modify the state. This function is
// used on start up to load the info into memory. Callers will use the public
// version of this function below, which returns our cached copy.
func (db *LevelDb) fetchAddrIndexTip() (*wire.ShaHash, int64, error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	data, err := db.lDb.Get(addrIndexMetaDataKey, db.ro)
	if err != nil {
		return &wire.ShaHash{}, -1, database.ErrAddrIndexDoesNotExist
	}

	var blkSha wire.ShaHash
	blkSha.SetBytes(data[0:32])

	blkHeight := binary.LittleEndian.Uint64(data[32:])

	return &blkSha, int64(blkHeight), nil
}

// FetchAddrIndexTip returns the hash and block height of the most recent
// block whose transactions have been indexed by address. It will return
// ErrAddrIndexDoesNotExist along with a zero hash, and -1 if the
// addrindex hasn't yet been built up.
func (db *LevelDb) FetchAddrIndexTip() (*wire.ShaHash, int64, error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	if db.lastAddrIndexBlkIdx == -1 {
		return &wire.ShaHash{}, -1, database.ErrAddrIndexDoesNotExist
	}
	sha := db.lastAddrIndexBlkSha

	return &sha, db.lastAddrIndexBlkIdx, nil
}
