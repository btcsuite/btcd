// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package index

import (
	"fmt"

	"github.com/btcsuite/btcd/blockchain"
	database "github.com/btcsuite/btcd/database2"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

var (
	// txIndexName is the name of the db bucket used to house the index.
	txIndexName = "txbyhash"
)

// -----------------------------------------------------------------------------
// The transaction index consists of an entry for every transaction in the main
// chain.  Since it possible for multiple transactions to have the same hash as
// long as the previous transaction with the same hash is fully spent, each
// entry consists of one or more records.
//
// The serialized format is:
//
//   <num entries>[<block hash><start offset><tx length>,...]
//
//   Field           Type           Size
//   num entries     uint8          1 byte
//   block hash      wire.ShaHash   wire.HashSize
//   start offset    uint32         4 bytes
//   tx length       uint32         4 bytes
// -----------------------------------------------------------------------------

// dbHasTxIndexEntry uses an existing database transaction to return whether or
// not the transaction index contains the provided hash.
func dbHasTxIndexEntry(bucket database.Bucket, txHash *wire.ShaHash) bool {
	return bucket.Get(txHash[:]) != nil
}

// dbPutTxIndexEntry uses an existing database transaction to update the
// transaction index given the provided values.
func dbPutTxIndexEntry(bucket database.Bucket, txHash, blockHash *wire.ShaHash, txLoc wire.TxLoc) error {
	existing := bucket.Get(txHash[:])

	// Since a new entry is being added, there will be one more than is
	// already there (if any).
	numEntries := uint8(0)
	if len(existing) > 0 {
		numEntries = existing[0]
	}
	numEntries++

	// Serialize the entry.
	serializedData := make([]byte, (uint32(numEntries)*(wire.HashSize+8))+1)
	serializedData[0] = numEntries
	offset := uint32(1)
	if len(existing) > 0 {
		copy(serializedData[1:], existing[1:])
		offset += uint32(len(existing) - 1)
	}
	copy(serializedData[offset:], blockHash[:])
	offset += wire.HashSize
	byteOrder.PutUint32(serializedData[offset:], uint32(txLoc.TxStart))
	offset += 4
	byteOrder.PutUint32(serializedData[offset:], uint32(txLoc.TxLen))

	return bucket.Put(txHash[:], serializedData)
}

// dbFetchTxIndexEntry uses an existing database transaction to fetch the block
// region for the provided transaction hash from the transaction index.  When
// there is no entry for the provided hash, nil will be returned for the both
// the region and the error.  When there are multiple entries, the block region
// for the latest one will be returned.
func dbFetchTxIndexEntry(bucket database.Bucket, txHash *wire.ShaHash) (*database.BlockRegion, error) {
	// Load the record from the database and return now if it doesn't exist.
	serializedData := bucket.Get(txHash[:])
	if len(serializedData) == 0 {
		return nil, nil
	}

	// Ensure the serialized data has enough bytes to properly deserialize.
	numEntries := uint8(serializedData[0])
	if len(serializedData) < (int(numEntries)*(wire.HashSize+8) + 1) {
		return nil, database.Error{
			ErrorCode: database.ErrCorruption,
			Description: fmt.Sprintf("corrupt transaction index "+
				"entry for %s", txHash),
		}
	}

	// Deserialize the final entry.
	region := database.BlockRegion{Hash: &wire.ShaHash{}}
	offset := (uint32(numEntries)-1)*(wire.HashSize+8) + 1
	copy(region.Hash[:], serializedData[offset:offset+wire.HashSize])
	offset += wire.HashSize
	region.Offset = byteOrder.Uint32(serializedData[offset : offset+4])
	offset += 4
	region.Len = byteOrder.Uint32(serializedData[offset : offset+4])

	return &region, nil
}

// dbAddTxIndexEntries uses an existing database transaction to add a
// transaction index entry for every transaction in the passed block.
func dbAddTxIndexEntries(bucket database.Bucket, block *btcutil.Block) error {
	txLocs, err := block.TxLoc()
	if err != nil {
		return err
	}

	for i, tx := range block.Transactions() {
		err := dbPutTxIndexEntry(bucket, tx.Sha(), block.Sha(), txLocs[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// dbRemoveTxIndexEntry uses an existing database transaction to remove the most
// recent transaction index entry for the given hash.
func dbRemoveTxIndexEntry(bucket database.Bucket, txHash *wire.ShaHash) error {
	serializedData := bucket.Get(txHash[:])
	if len(serializedData) == 0 {
		return fmt.Errorf("can't remove non-existent transaction %s "+
			"from the transaction index", txHash)
	}

	// Remove the key altogether if there was only one entry.
	numEntries := uint8(serializedData[0])
	if numEntries == 1 {
		return bucket.Delete(txHash[:])
	}

	// Ensure the serialized data has enough bytes to properly deserialize.
	if len(serializedData) < (int(numEntries)*(wire.HashSize+8) + 1) {
		return database.Error{
			ErrorCode: database.ErrCorruption,
			Description: fmt.Sprintf("corrupt transaction index "+
				"entry for %s", txHash),
		}
	}

	// Remove the latest entry since there is more than one.
	numEntries--
	serializedData[0] = numEntries
	offset := (numEntries-1)*(wire.HashSize+8) + 1
	serializedData = serializedData[:offset]
	return bucket.Put(txHash[:], serializedData)
}

// dbRemoveTxIndexEntries uses an existing database transaction to remove the
// latest transaction entry for every transaction in the passed block.
func dbRemoveTxIndexEntries(bucket database.Bucket, block *btcutil.Block) error {
	for _, tx := range block.Transactions() {
		err := dbRemoveTxIndexEntry(bucket, tx.Sha())
		if err != nil {
			return err
		}
	}

	return nil
}

// TxIndex indexes transaction locations by hash.
type TxIndex struct {
	chain *blockchain.BlockChain
}

// Init initializes the Index for use with the given BlockChain.
func (i *TxIndex) Init(b *blockchain.BlockChain) {
	i.chain = b
}

// Name returns the index's name. It should be unique per index type.
// It is used to identify
func (i *TxIndex) Name() string {
	return txIndexName
}

// Version returns the index's version.
func (i *TxIndex) Version() int32 {
	return 1
}

// Create initializes the necessary data structures for the index
// in the database using an existing transaction. It creates buckets and
// fills them with initial data as needed.
func (i *TxIndex) Create(dbTx database.Tx, bucket database.Bucket) error {
	// Nothing needs to be inserted.
	return nil
}

// ConnectBlock indexes the given block using an existing database
// transaction.
func (i *TxIndex) ConnectBlock(dbTx database.Tx, bucket database.Bucket, block *btcutil.Block, view *blockchain.UtxoViewpoint) error {
	// Add transaction index entries for all of the transactions in
	// the block being connected.
	return dbAddTxIndexEntries(bucket, block)
}

// DisconnectBlock de-indexes the given block using an existing database
// transaction.
func (i *TxIndex) DisconnectBlock(dbTx database.Tx, bucket database.Bucket, block *btcutil.Block, view *blockchain.UtxoViewpoint) error {
	// Remove transaction index entries for all of the transactions
	// in the block being disconnected.
	return dbRemoveTxIndexEntries(bucket, block)
}

// TxBlockRegion returns the block region for the provided transaction hash
// from the transaction index.  The block region can in turn be used to load the
// raw transaction bytes.  When there is no entry for the provided hash, nil
// will be returned for the both the entry and the error.
//
// The database transaction parameter can be nil in which case a a new one will
// be used.
//
// This function is safe for concurrent access.
func (i *TxIndex) TxBlockRegion(dbTx database.Tx, hash *wire.ShaHash) (*database.BlockRegion, error) {
	if dbTx == nil {
		var region *database.BlockRegion
		err := i.chain.DB().View(func(dbTx database.Tx) error {
			var err error
			region, err = i.TxBlockRegion(dbTx, hash)
			return err
		})
		return region, err
	}

	bucket := i.chain.IndexBucket(dbTx, txIndexName)
	return dbFetchTxIndexEntry(bucket, hash)
}

// ExistsTx returns whether or not the provided transaction hash exists from the
// viewpoint of the end of the main chain.
//
// This function is safe for concurrent access.
func (i *TxIndex) ExistsTx(dbTx database.Tx, hash *wire.ShaHash) (bool, error) {
	if dbTx == nil {
		var exists bool
		err := i.chain.DB().View(func(dbTx database.Tx) error {
			var err error
			exists, err = i.ExistsTx(dbTx, hash)
			return err
		})
		return exists, err
	}

	bucket := i.chain.IndexBucket(dbTx, txIndexName)
	return dbHasTxIndexEntry(bucket, hash), nil
}

// AddMempoolTx is called when a tx is added to the mempool, it gets added
// to the index data structures if needed.
func (i *TxIndex) AddMempoolTx(tx *btcutil.Tx) error {
	return nil
}

// RemoveMempoolTx is called when a tx is removed from the mempool, it gets
// removed from the index data structures if needed.
func (i *TxIndex) RemoveMempoolTx(tx *btcutil.Tx) error {
	return nil
}

// Ensure TxIndex implements blockchain.Index
var _ blockchain.Index = (*TxIndex)(nil)

// NewTxIndex creates a new TxIndex.
func NewTxIndex() *TxIndex {
	return &TxIndex{}
}
