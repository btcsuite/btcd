// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/gcs/builder"
)

const (
	// cfIndexName is the human-readable name for the index.
	cfIndexName = "committed filter index"
)

// Committed filters come in two flavours: basic and extended. They are
// generated and dropped in pairs, and both are indexed by a block's hash.
// Besides holding different content, they also live in different buckets.
var (
	// cfBasicIndexKey is the name of the db bucket used to house the
	// block hash -> basic cf index (cf#0).
	cfBasicIndexKey = []byte("cf0byhashidx")
	// cfBasicHeaderKey is the name of the db bucket used to house the
	// block hash -> basic cf header index (cf#0).
	cfBasicHeaderKey = []byte("cf0headerbyhashidx")
	// cfExtendedIndexKey is the name of the db bucket used to house the
	// block hash -> extended cf index (cf#1).
	cfExtendedIndexKey = []byte("cf1byhashidx")
	// cfExtendedHeaderKey is the name of the db bucket used to house the
	// block hash -> extended cf header index (cf#1).
	cfExtendedHeaderKey = []byte("cf1headerbyhashidx")
)

// dbFetchBasicEntry() retrieves a block's basic filter. An entry's absence is
// not considered an error. The filter is returned serialized.
func dbFetchBasicEntry(dbTx database.Tx, h *chainhash.Hash) ([]byte, error) {
	idx := dbTx.Metadata().Bucket(cfBasicIndexKey)
	return idx.Get(h[:]), nil
}

// dbFetchExtendedEntry() retrieves a block's extended filter. An entry's
// absence is not considered an error. The filter is returned serialized.
func dbFetchExtendedEntry(dbTx database.Tx, h *chainhash.Hash) ([]byte, error) {
	idx := dbTx.Metadata().Bucket(cfExtendedIndexKey)
	return idx.Get(h[:]), nil
}

// dbFetchBasicHeader() retrieves a block's basic filter header. A filter's
// absence is not considered an error.
func dbFetchBasicHeader(dbTx database.Tx, h *chainhash.Hash) ([]byte, error) {
	idx := dbTx.Metadata().Bucket(cfBasicHeaderKey)
	return idx.Get(h[:]), nil
}

// dbFetchExtendedHeader() retrieves a block's extended filter header.
// A filter's absence is not considered an error.
func dbFetchExtendedHeader(dbTx database.Tx, h*chainhash.Hash) ([]byte, error) {
	idx := dbTx.Metadata().Bucket(cfExtendedHeaderKey)
	return idx.Get(h[:]), nil
}

// dbStoreBasicEntry() stores a block's basic filter.
func dbStoreBasicEntry(dbTx database.Tx, h *chainhash.Hash, f []byte) error {
	idx := dbTx.Metadata().Bucket(cfBasicIndexKey)
	return idx.Put(h[:], f)
}

// dbStoreBasicEntry() stores a block's extended filter.
func dbStoreExtendedEntry(dbTx database.Tx, h *chainhash.Hash, f []byte) error {
	idx := dbTx.Metadata().Bucket(cfExtendedIndexKey)
	return idx.Put(h[:], f)
}

// dbDeleteBasicEntry() deletes a block's basic filter.
func dbDeleteBasicEntry(dbTx database.Tx, h *chainhash.Hash) error {
	idx := dbTx.Metadata().Bucket(cfBasicIndexKey)
	return idx.Delete(h[:])
}

// dbDeleteExtendedEntry() deletes a block's extended filter.
func dbDeleteExtendedEntry(dbTx database.Tx, h *chainhash.Hash) error {
	idx := dbTx.Metadata().Bucket(cfExtendedIndexKey)
	return idx.Delete(h[:])
}

// CfIndex implements a committed filter (cf) by hash index.
type CfIndex struct {
	db database.DB
}

// Ensure the CfIndex type implements the Indexer interface.
var _ Indexer = (*CfIndex)(nil)

// Init initializes the hash-based cf index. This is part of the Indexer
// interface.
func (idx *CfIndex) Init() error {
	return nil // Nothing to do.
}

// Key returns the database key to use for the index as a byte slice. This is
// part of the Indexer interface.
func (idx *CfIndex) Key() []byte {
	return cfBasicIndexKey
}

// Name returns the human-readable name of the index. This is part of the
// Indexer interface.
func (idx *CfIndex) Name() string {
	return cfIndexName
}

// Create is invoked when the indexer manager determines the index needs to be
// created for the first time. It creates buckets for the two hash-based cf
// indexes (simple, extended).
func (idx *CfIndex) Create(dbTx database.Tx) error {
	meta := dbTx.Metadata()
	_, err := meta.CreateBucket(cfBasicIndexKey)
	if err != nil {
		return err
	}
	_, err = meta.CreateBucket(cfBasicHeaderKey)
	if err != nil {
		return err
	}
	_, err = meta.CreateBucket(cfExtendedIndexKey)
	if err != nil {
		return err
	}
	_, err = meta.CreateBucket(cfExtendedHeaderKey)
	return err
}

// makeBasicFilter() builds a block's basic filter, which consists of all
// outpoints and pkscript data pushes referenced by transactions within the
// block.
func makeBasicFilterForBlock(block *btcutil.Block) ([]byte, error) {
	b := builder.WithKeyHash(block.Hash())
	_, err := b.Key()
	if err != nil {
		return nil, err
	}
	for _, tx := range block.Transactions() {
		for _, txIn := range tx.MsgTx().TxIn {
			b.AddOutPoint(txIn.PreviousOutPoint)
		}
		for _, txOut := range tx.MsgTx().TxOut {
			b.AddScript(txOut.PkScript)
		}
	}
	f, err := b.Build()
	if err != nil {
		return nil, err
	}
	return f.Bytes(), nil
}

// makeExtendedFilter() builds a block's extended filter, which consists of
// all tx hashes and sigscript data pushes contained in the block.
func makeExtendedFilterForBlock(block *btcutil.Block) ([]byte, error) {
	b := builder.WithKeyHash(block.Hash())
	_, err := b.Key()
	if err != nil {
		return nil, err
	}
	for _, tx := range block.Transactions() {
		b.AddHash(tx.Hash())
		for _, txIn := range tx.MsgTx().TxIn {
			b.AddScript(txIn.SignatureScript)
		}
	}
	f, err := b.Build()
	if err != nil {
		return nil, err
	}
	return f.Bytes(), nil
}

// ConnectBlock is invoked by the index manager when a new block has been
// connected to the main chain. This indexer adds a hash-to-cf mapping for
// every passed block. This is part of the Indexer interface.
func (idx *CfIndex) ConnectBlock(dbTx database.Tx, block *btcutil.Block,
    view *blockchain.UtxoViewpoint) error {
	f, err := makeBasicFilterForBlock(block)
	if err != nil {
		return err
	}
	err = dbStoreBasicEntry(dbTx, block.Hash(), f)
	if err != nil {
		return err
	}
	f, err = makeExtendedFilterForBlock(block)
	if err != nil {
		return err
	}
	return dbStoreExtendedEntry(dbTx, block.Hash(), f)
}

// DisconnectBlock is invoked by the index manager when a block has been
// disconnected from the main chain.  This indexer removes the hash-to-cf
// mapping for every passed block. This is part of the Indexer interface.
func (idx *CfIndex) DisconnectBlock(dbTx database.Tx, block *btcutil.Block,
    view *blockchain.UtxoViewpoint) error {
	err := dbDeleteBasicEntry(dbTx, block.Hash())
	if err != nil {
		return err
	}
	return dbDeleteExtendedEntry(dbTx, block.Hash())
}

func (idx *CfIndex) FilterByBlockHash(hash *chainhash.Hash, extended bool) ([]byte, error) {
	var filterBytes []byte
	err := idx.db.View(func(dbTx database.Tx) error {
		var err error
		if extended {
			filterBytes, err = dbFetchExtendedEntry(dbTx, hash)
		} else {
			filterBytes, err = dbFetchBasicEntry(dbTx, hash)
		}
		return err
	})
	return filterBytes, err
}

// NewCfIndex returns a new instance of an indexer that is used to create a
// mapping of the hashes of all blocks in the blockchain to their respective
// committed filters.
//
// It implements the Indexer interface which plugs into the IndexManager that in
// turn is used by the blockchain package. This allows the index to be
// seamlessly maintained along with the chain.
func NewCfIndex(db database.DB) *CfIndex {
	return &CfIndex{db: db}
}

// DropCfIndex drops the CF index from the provided database if exists.
func DropCfIndex(db database.DB) error {
	return dropIndex(db, cfBasicIndexKey, cfIndexName)
}
