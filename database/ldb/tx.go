// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/goleveldb/leveldb"
	"github.com/btcsuite/goleveldb/leveldb/util"

	"golang.org/x/crypto/ripemd160"
)

const (
	// Each address index is 34 bytes:
	// --------------------------------------------------------
	// | Prefix  | Hash160  | BlkHeight | Tx Offset | Tx Size |
	// --------------------------------------------------------
	// | 2 bytes | 20 bytes |  4 bytes  |  4 bytes  | 4 bytes |
	// --------------------------------------------------------
	addrIndexKeyLength = 2 + ripemd160.Size + 4 + 4 + 4

	batchDeleteThreshold = 10000
)

var addrIndexMetaDataKey = []byte("addrindex")

// All address index entries share this prefix to facilitate the use of
// iterators.
var addrIndexKeyPrefix = []byte("a-")

type txUpdateObj struct {
	txSha     *wire.ShaHash
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

type txAddrIndex struct {
	hash160   [ripemd160.Size]byte
	blkHeight int64
	txoffset  int
	txlen     int
}

// InsertTx inserts a tx hash and its associated data into the database.
func (db *LevelDb) InsertTx(txsha *wire.ShaHash, height int64, txoff int, txlen int, spentbuf []byte) (err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	return db.insertTx(txsha, height, txoff, txlen, spentbuf)
}

// insertTx inserts a tx hash and its associated data into the database.
// Must be called with db lock held.
func (db *LevelDb) insertTx(txSha *wire.ShaHash, height int64, txoff int, txlen int, spentbuf []byte) (err error) {
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
func (db *LevelDb) formatTx(txu *txUpdateObj) []byte {
	blkHeight := uint64(txu.blkHeight)
	txOff := uint32(txu.txoff)
	txLen := uint32(txu.txlen)
	spentbuf := txu.spentData

	txW := make([]byte, 16+len(spentbuf))
	binary.LittleEndian.PutUint64(txW[0:8], blkHeight)
	binary.LittleEndian.PutUint32(txW[8:12], txOff)
	binary.LittleEndian.PutUint32(txW[12:16], txLen)
	copy(txW[16:], spentbuf)

	return txW[:]
}

func (db *LevelDb) getTxData(txsha *wire.ShaHash) (int64, int, int, []byte, error) {
	key := shaTxToKey(txsha)
	buf, err := db.lDb.Get(key, db.ro)
	if err != nil {
		return 0, 0, 0, nil, err
	}

	blkHeight := binary.LittleEndian.Uint64(buf[0:8])
	txOff := binary.LittleEndian.Uint32(buf[8:12])
	txLen := binary.LittleEndian.Uint32(buf[12:16])

	spentBuf := make([]byte, len(buf)-16)
	copy(spentBuf, buf[16:])

	return int64(blkHeight), int(txOff), int(txLen), spentBuf, nil
}

func (db *LevelDb) getTxFullySpent(txsha *wire.ShaHash) ([]*spentTx, error) {

	var badTxList, spentTxList []*spentTx

	key := shaSpentTxToKey(txsha)
	buf, err := db.lDb.Get(key, db.ro)
	if err == leveldb.ErrNotFound {
		return badTxList, database.ErrTxShaMissing
	} else if err != nil {
		return badTxList, err
	}
	txListLen := len(buf) / 20

	spentTxList = make([]*spentTx, txListLen, txListLen)
	for i := range spentTxList {
		offset := i * 20

		blkHeight := binary.LittleEndian.Uint64(buf[offset : offset+8])
		txOff := binary.LittleEndian.Uint32(buf[offset+8 : offset+12])
		txLen := binary.LittleEndian.Uint32(buf[offset+12 : offset+16])
		numTxO := binary.LittleEndian.Uint32(buf[offset+16 : offset+20])

		sTx := spentTx{
			blkHeight: int64(blkHeight),
			txoff:     int(txOff),
			txlen:     int(txLen),
			numTxO:    int(numTxO),
		}

		spentTxList[i] = &sTx
	}

	return spentTxList, nil
}

func (db *LevelDb) formatTxFullySpent(sTxList []*spentTx) []byte {
	txW := make([]byte, 20*len(sTxList))

	for i, sTx := range sTxList {
		blkHeight := uint64(sTx.blkHeight)
		txOff := uint32(sTx.txoff)
		txLen := uint32(sTx.txlen)
		numTxO := uint32(sTx.numTxO)
		offset := i * 20

		binary.LittleEndian.PutUint64(txW[offset:offset+8], blkHeight)
		binary.LittleEndian.PutUint32(txW[offset+8:offset+12], txOff)
		binary.LittleEndian.PutUint32(txW[offset+12:offset+16], txLen)
		binary.LittleEndian.PutUint32(txW[offset+16:offset+20], numTxO)
	}

	return txW
}

// ExistsTxSha returns if the given tx sha exists in the database
func (db *LevelDb) ExistsTxSha(txsha *wire.ShaHash) (bool, error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	return db.existsTxSha(txsha)
}

// existsTxSha returns if the given tx sha exists in the database.o
// Must be called with the db lock held.
func (db *LevelDb) existsTxSha(txSha *wire.ShaHash) (bool, error) {
	key := shaTxToKey(txSha)

	return db.lDb.Has(key, db.ro)
}

// FetchTxByShaList returns the most recent tx of the name fully spent or not
func (db *LevelDb) FetchTxByShaList(txShaList []*wire.ShaHash) []*database.TxListReply {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	// until the fully spent separation of tx is complete this is identical
	// to FetchUnSpentTxByShaList
	replies := make([]*database.TxListReply, len(txShaList))
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
		if err == database.ErrTxShaMissing {
			// if the unspent pool did not have the tx,
			// look in the fully spent pool (only last instance)

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
		txlre := database.TxListReply{Sha: txsha, Tx: tx, BlkSha: blockSha, Height: height, TxSpent: btxspent, Err: err}
		replies[i] = &txlre
	}
	return replies
}

// FetchUnSpentTxByShaList given a array of ShaHash, look up the transactions
// and return them in a TxListReply array.
func (db *LevelDb) FetchUnSpentTxByShaList(txShaList []*wire.ShaHash) []*database.TxListReply {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	replies := make([]*database.TxListReply, len(txShaList))
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
		txlre := database.TxListReply{Sha: txsha, Tx: tx, BlkSha: blockSha, Height: height, TxSpent: btxspent, Err: err}
		replies[i] = &txlre
	}
	return replies
}

// fetchTxDataBySha returns several pieces of data regarding the given sha.
func (db *LevelDb) fetchTxDataBySha(txsha *wire.ShaHash) (rtx *wire.MsgTx, rblksha *wire.ShaHash, rheight int64, rtxspent []byte, err error) {
	var blkHeight int64
	var txspent []byte
	var txOff, txLen int

	blkHeight, txOff, txLen, txspent, err = db.getTxData(txsha)
	if err != nil {
		if err == leveldb.ErrNotFound {
			err = database.ErrTxShaMissing
		}
		return
	}
	return db.fetchTxDataByLoc(blkHeight, txOff, txLen, txspent)
}

// fetchTxDataByLoc returns several pieces of data regarding the given tx
// located by the block/offset/size location
func (db *LevelDb) fetchTxDataByLoc(blkHeight int64, txOff int, txLen int, txspent []byte) (rtx *wire.MsgTx, rblksha *wire.ShaHash, rheight int64, rtxspent []byte, err error) {
	var blksha *wire.ShaHash
	var blkbuf []byte

	blksha, blkbuf, err = db.getBlkByHeight(blkHeight)
	if err != nil {
		if err == leveldb.ErrNotFound {
			err = database.ErrTxShaMissing
		}
		return
	}

	//log.Trace("transaction %v is at block %v %v txoff %v, txlen %v\n",
	//	txsha, blksha, blkHeight, txOff, txLen)

	if len(blkbuf) < txOff+txLen {
		err = database.ErrTxShaMissing
		return
	}
	rbuf := bytes.NewReader(blkbuf[txOff : txOff+txLen])

	var tx wire.MsgTx
	err = tx.Deserialize(rbuf)
	if err != nil {
		log.Warnf("unable to decode tx block %v %v txoff %v txlen %v",
			blkHeight, blksha, txOff, txLen)
		return
	}

	return &tx, blksha, blkHeight, txspent, nil
}

// FetchTxBySha returns some data for the given Tx Sha.
func (db *LevelDb) FetchTxBySha(txsha *wire.ShaHash) ([]*database.TxListReply, error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	replylen := 0
	replycnt := 0

	tx, blksha, height, txspent, txerr := db.fetchTxDataBySha(txsha)
	if txerr == nil {
		replylen++
	} else {
		if txerr != database.ErrTxShaMissing {
			return []*database.TxListReply{}, txerr
		}
	}

	sTxList, fSerr := db.getTxFullySpent(txsha)

	if fSerr != nil {
		if fSerr != database.ErrTxShaMissing {
			return []*database.TxListReply{}, fSerr
		}
	} else {
		replylen += len(sTxList)
	}

	replies := make([]*database.TxListReply, replylen)

	if fSerr == nil {
		for _, stx := range sTxList {
			tx, blksha, _, _, err := db.fetchTxDataByLoc(
				stx.blkHeight, stx.txoff, stx.txlen, []byte{})
			if err != nil {
				if err != leveldb.ErrNotFound {
					return []*database.TxListReply{}, err
				}
				continue
			}
			btxspent := make([]bool, len(tx.TxOut), len(tx.TxOut))
			for i := range btxspent {
				btxspent[i] = true
			}
			txlre := database.TxListReply{Sha: txsha, Tx: tx, BlkSha: blksha, Height: stx.blkHeight, TxSpent: btxspent, Err: nil}
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
		txlre := database.TxListReply{Sha: txsha, Tx: tx, BlkSha: blksha, Height: height, TxSpent: btxspent, Err: nil}
		replies[replycnt] = &txlre
		replycnt++
	}
	return replies, nil
}

// addrIndexToKey serializes the passed txAddrIndex for storage within the DB.
func addrIndexToKey(index *txAddrIndex) []byte {
	record := make([]byte, addrIndexKeyLength, addrIndexKeyLength)
	copy(record[:2], addrIndexKeyPrefix)
	copy(record[2:22], index.hash160[:])

	// The index itself.
	binary.LittleEndian.PutUint32(record[22:26], uint32(index.blkHeight))
	binary.LittleEndian.PutUint32(record[26:30], uint32(index.txoffset))
	binary.LittleEndian.PutUint32(record[30:34], uint32(index.txlen))

	return record
}

// unpackTxIndex deserializes the raw bytes of a address tx index.
func unpackTxIndex(rawIndex []byte) *txAddrIndex {
	return &txAddrIndex{
		blkHeight: int64(binary.LittleEndian.Uint32(rawIndex[0:4])),
		txoffset:  int(binary.LittleEndian.Uint32(rawIndex[4:8])),
		txlen:     int(binary.LittleEndian.Uint32(rawIndex[8:12])),
	}
}

// bytesPrefix returns key range that satisfy the given prefix.
// This only applicable for the standard 'bytes comparer'.
func bytesPrefix(prefix []byte) *util.Range {
	var limit []byte
	for i := len(prefix) - 1; i >= 0; i-- {
		c := prefix[i]
		if c < 0xff {
			limit = make([]byte, i+1)
			copy(limit, prefix)
			limit[i] = c + 1
			break
		}
	}
	return &util.Range{Start: prefix, Limit: limit}
}

// FetchTxsForAddr looks up and returns all transactions which either
// spend from a previously created output of the passed address, or
// create a new output locked to the passed address. The, `limit` parameter
// should be the max number of transactions to be returned. Additionally, if the
// caller wishes to seek forward in the results some amount, the 'seek'
// represents how many results to skip.
func (db *LevelDb) FetchTxsForAddr(addr btcutil.Address, skip int,
	limit int) ([]*database.TxListReply, error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	// Enforce constraints for skip and limit.
	if skip < 0 {
		return nil, errors.New("offset for skip must be positive")
	}
	if limit < 0 {
		return nil, errors.New("value for limit must be positive")
	}

	// Parse address type, bailing on an unknown type.
	var addrKey []byte
	switch addr := addr.(type) {
	case *btcutil.AddressPubKeyHash:
		hash160 := addr.Hash160()
		addrKey = hash160[:]
	case *btcutil.AddressScriptHash:
		hash160 := addr.Hash160()
		addrKey = hash160[:]
	case *btcutil.AddressPubKey:
		hash160 := addr.AddressPubKeyHash().Hash160()
		addrKey = hash160[:]
	default:
		return nil, database.ErrUnsupportedAddressType
	}

	// Create the prefix for our search.
	addrPrefix := make([]byte, 22, 22)
	copy(addrPrefix[:2], addrIndexKeyPrefix)
	copy(addrPrefix[2:], addrKey)

	var replies []*database.TxListReply
	iter := db.lDb.NewIterator(bytesPrefix(addrPrefix), nil)

	for skip != 0 && iter.Next() {
		skip--
	}

	// Iterate through all address indexes that match the targeted prefix.
	for iter.Next() && limit != 0 {
		rawIndex := make([]byte, 22, 22)
		copy(rawIndex, iter.Key()[22:])
		addrIndex := unpackTxIndex(rawIndex)

		tx, blkSha, blkHeight, _, err := db.fetchTxDataByLoc(addrIndex.blkHeight,
			addrIndex.txoffset, addrIndex.txlen, []byte{})
		if err != nil {
			// Eat a possible error due to a potential re-org.
			continue
		}

		txSha, _ := tx.TxSha()
		txReply := &database.TxListReply{Sha: &txSha, Tx: tx,
			BlkSha: blkSha, Height: blkHeight, TxSpent: []bool{}, Err: err}

		replies = append(replies, txReply)
		limit--
	}
	iter.Release()

	return replies, nil
}

// UpdateAddrIndexForBlock updates the stored addrindex with passed
// index information for a particular block height. Additionally, it
// will update the stored meta-data related to the curent tip of the
// addr index. These two operations are performed in an atomic
// transaction which is commited before the function returns.
// Transactions indexed by address are stored with the following format:
//   * prefix || hash160 || blockHeight || txoffset || txlen
// Indexes are stored purely in the key, with blank data for the actual value
// in order to facilitate ease of iteration by their shared prefix and
// also to allow limiting the number of returned transactions (RPC).
// Alternatively, indexes for each address could be stored as an
// append-only list for the stored value. However, this add unnecessary
// overhead when storing and retrieving since the entire list must
// be fetched each time.
func (db *LevelDb) UpdateAddrIndexForBlock(blkSha *wire.ShaHash, blkHeight int64, addrIndex database.BlockAddrIndex) error {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	var blankData []byte
	batch := db.lBatch()
	defer db.lbatch.Reset()

	// Write all data for the new address indexes in a single batch
	// transaction.
	for addrKey, indexes := range addrIndex {
		for _, txLoc := range indexes {
			index := &txAddrIndex{
				hash160:   addrKey,
				blkHeight: blkHeight,
				txoffset:  txLoc.TxStart,
				txlen:     txLoc.TxLen,
			}
			// The index is stored purely in the key.
			packedIndex := addrIndexToKey(index)
			batch.Put(packedIndex, blankData)
		}
	}

	// Update tip of addrindex.
	newIndexTip := make([]byte, 40, 40)
	copy(newIndexTip[:32], blkSha.Bytes())
	binary.LittleEndian.PutUint64(newIndexTip[32:], uint64(blkHeight))
	batch.Put(addrIndexMetaDataKey, newIndexTip)

	if err := db.lDb.Write(batch, db.wo); err != nil {
		return err
	}

	db.lastAddrIndexBlkIdx = blkHeight
	db.lastAddrIndexBlkSha = *blkSha

	return nil
}

// DeleteAddrIndex deletes the entire addrindex stored within the DB.
// It also resets the cached in-memory metadata about the addr index.
func (db *LevelDb) DeleteAddrIndex() error {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	batch := db.lBatch()
	defer batch.Reset()

	// Delete the entire index along with any metadata about it.
	iter := db.lDb.NewIterator(bytesPrefix(addrIndexKeyPrefix), db.ro)
	numInBatch := 0
	for iter.Next() {
		key := iter.Key()
		batch.Delete(key)

		numInBatch++

		// Delete in chunks to potentially avoid very large batches.
		if numInBatch >= batchDeleteThreshold {
			if err := db.lDb.Write(batch, db.wo); err != nil {
				return err
			}
			batch.Reset()
			numInBatch = 0
		}
	}
	iter.Release()

	batch.Delete(addrIndexMetaDataKey)
	if err := db.lDb.Write(batch, db.wo); err != nil {
		return err
	}

	db.lastAddrIndexBlkIdx = -1
	db.lastAddrIndexBlkSha = wire.ShaHash{}

	return nil
}
