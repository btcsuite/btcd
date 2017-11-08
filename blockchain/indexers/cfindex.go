// Copyright (c) 2017 The btcsuite developers
// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"errors"
	"fmt"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/gcs"
	"github.com/decred/dcrd/gcs/blockcf"
	"github.com/decred/dcrd/wire"
)

const (
	// cfIndexName is the human-readable name for the index.
	cfIndexName = "committed filter index"
)

// Committed filters come in two flavors: basic and extended. They are
// generated and dropped in pairs, and both are indexed by a block's hash.
// Besides holding different content, they also live in different buckets.
var (
	// cfIndexParentBucketKey is the name of the parent bucket used to house
	// the index. The rest of the buckets live below this bucket.
	cfIndexParentBucketKey = []byte("cfindexparentbucket")

	// cfIndexKeys is an array of db bucket names used to house indexes of
	// block hashes to cfilters.
	cfIndexKeys = [][]byte{
		[]byte("cf0byhashidx"),
		[]byte("cf1byhashidx"),
	}

	// cfHeaderKeys is an array of db bucket names used to house indexes of
	// block hashes to cf headers.
	cfHeaderKeys = [][]byte{
		[]byte("cf0headerbyhashidx"),
		[]byte("cf1headerbyhashidx"),
	}

	maxFilterType = uint8(len(cfHeaderKeys) - 1)
)

// dbFetchFilter retrieves a block's basic or extended filter. A filter's
// absence is not considered an error.
func dbFetchFilter(dbTx database.Tx, key []byte, h *chainhash.Hash) []byte {
	idx := dbTx.Metadata().Bucket(cfIndexParentBucketKey).Bucket(key)
	return idx.Get(h[:])
}

// dbFetchFilterHeader retrieves a block's basic or extended filter header.
// A filter's absence is not considered an error.
func dbFetchFilterHeader(dbTx database.Tx, key []byte, h *chainhash.Hash) ([]byte, error) {
	idx := dbTx.Metadata().Bucket(cfIndexParentBucketKey).Bucket(key)

	fh := idx.Get(h[:])
	if fh == nil {
		return make([]byte, chainhash.HashSize), nil
	}
	if len(fh) != chainhash.HashSize {
		return nil, fmt.Errorf("invalid filter header length %v", len(fh))
	}

	return fh, nil
}

// dbStoreFilter stores a block's basic or extended filter.
func dbStoreFilter(dbTx database.Tx, key []byte, h *chainhash.Hash, f []byte) error {
	idx := dbTx.Metadata().Bucket(cfIndexParentBucketKey).Bucket(key)
	return idx.Put(h[:], f)
}

// dbStoreFilterHeader stores a block's basic or extended filter header.
func dbStoreFilterHeader(dbTx database.Tx, key []byte, h *chainhash.Hash, fh []byte) error {
	if len(fh) != chainhash.HashSize {
		return fmt.Errorf("invalid filter header length %v", len(fh))
	}
	idx := dbTx.Metadata().Bucket(cfIndexParentBucketKey).Bucket(key)
	return idx.Put(h[:], fh)
}

// dbDeleteFilter deletes a filter's basic or extended filter.
func dbDeleteFilter(dbTx database.Tx, key []byte, h *chainhash.Hash) error {
	idx := dbTx.Metadata().Bucket(cfIndexParentBucketKey).Bucket(key)
	return idx.Delete(h[:])
}

// dbDeleteFilterHeader deletes a filter's basic or extended filter header.
func dbDeleteFilterHeader(dbTx database.Tx, key []byte, h *chainhash.Hash) error {
	idx := dbTx.Metadata().Bucket(cfIndexParentBucketKey).Bucket(key)
	return idx.Delete(h[:])
}

// CFIndex implements a committed filter (cf) by hash index.
type CFIndex struct {
	db          database.DB
	chainParams *chaincfg.Params
}

// Ensure the CFIndex type implements the Indexer interface.
var _ Indexer = (*CFIndex)(nil)

// Init initializes the hash-based cf index. This is part of the Indexer
// interface.
func (idx *CFIndex) Init() error {
	return nil // Nothing to do.
}

// Key returns the database key to use for the index as a byte slice. This is
// part of the Indexer interface.
func (idx *CFIndex) Key() []byte {
	return cfIndexParentBucketKey
}

// Name returns the human-readable name of the index. This is part of the
// Indexer interface.
func (idx *CFIndex) Name() string {
	return cfIndexName
}

// Create is invoked when the indexer manager determines the index needs to
// be created for the first time. It creates buckets for the two hash-based cf
// indexes (simple, extended).
func (idx *CFIndex) Create(dbTx database.Tx) error {
	meta := dbTx.Metadata()

	cfIndexParentBucket, err := meta.CreateBucket(cfIndexParentBucketKey)
	if err != nil {
		return err
	}

	for _, bucketName := range cfIndexKeys {
		_, err = cfIndexParentBucket.CreateBucket(bucketName)
		if err != nil {
			return err
		}
	}

	for _, bucketName := range cfHeaderKeys {
		_, err = cfIndexParentBucket.CreateBucket(bucketName)
		if err != nil {
			return err
		}
	}

	firstHeader := make([]byte, chainhash.HashSize)
	err = dbStoreFilterHeader(dbTx, cfHeaderKeys[wire.GCSFilterRegular],
		&idx.chainParams.GenesisBlock.Header.PrevBlock, firstHeader)
	if err != nil {
		return err
	}

	return dbStoreFilterHeader(dbTx, cfHeaderKeys[wire.GCSFilterExtended],
		&idx.chainParams.GenesisBlock.Header.PrevBlock, firstHeader)
}

// storeFilter stores a given filter, and performs the steps needed to
// generate the filter's header.
func storeFilter(dbTx database.Tx, block *dcrutil.Block, f *gcs.Filter, filterType wire.FilterType) error {
	if uint8(filterType) > maxFilterType {
		return errors.New("unsupported filter type")
	}

	// Figure out which buckets to use.
	fkey := cfIndexKeys[filterType]
	hkey := cfHeaderKeys[filterType]

	// Start by storing the filter.
	h := block.Hash()
	var basicFilterBytes []byte
	if f != nil {
		basicFilterBytes = f.NBytes()
	}
	err := dbStoreFilter(dbTx, fkey, h, basicFilterBytes)
	if err != nil {
		return err
	}

	// Then fetch the previous block's filter header.
	ph := &block.MsgBlock().Header.PrevBlock
	pfh, err := dbFetchFilterHeader(dbTx, hkey, ph)
	if err != nil {
		return err
	}

	// Construct the new block's filter header, and store it.
	prevHeader, err := chainhash.NewHash(pfh)
	if err != nil {
		return err
	}
	fh := gcs.MakeHeaderForFilter(f, prevHeader)
	return dbStoreFilterHeader(dbTx, hkey, h, fh[:])
}

// ConnectBlock is invoked by the index manager when a new block has been
// connected to the main chain. This indexer adds a hash-to-cf mapping for
// every passed block. This is part of the Indexer interface.
func (idx *CFIndex) ConnectBlock(dbTx database.Tx, block, parent *dcrutil.Block, view *blockchain.UtxoViewpoint) error {
	f, err := blockcf.Regular(block.MsgBlock())
	if err != nil && err != gcs.ErrNoData {
		return err
	}

	err = storeFilter(dbTx, block, f, wire.GCSFilterRegular)
	if err != nil {
		return err
	}

	f, err = blockcf.Extended(block.MsgBlock())
	if err != nil && err != gcs.ErrNoData {
		return err
	}

	return storeFilter(dbTx, block, f, wire.GCSFilterExtended)
}

// DisconnectBlock is invoked by the index manager when a block has been
// disconnected from the main chain.  This indexer removes the hash-to-cf
// mapping for every passed block. This is part of the Indexer interface.
func (idx *CFIndex) DisconnectBlock(dbTx database.Tx, block, parent *dcrutil.Block, view *blockchain.UtxoViewpoint) error {
	for _, key := range cfIndexKeys {
		err := dbDeleteFilter(dbTx, key, block.Hash())
		if err != nil {
			return err
		}
	}

	for _, key := range cfHeaderKeys {
		err := dbDeleteFilterHeader(dbTx, key, block.Hash())
		if err != nil {
			return err
		}
	}

	return nil
}

// FilterByBlockHash returns the serialized contents of a block's basic or
// extended committed filter.
func (idx *CFIndex) FilterByBlockHash(h *chainhash.Hash, filterType wire.FilterType) ([]byte, error) {
	if uint8(filterType) > maxFilterType {
		return nil, errors.New("unsupported filter type")
	}

	var f []byte
	err := idx.db.View(func(dbTx database.Tx) error {
		f = dbFetchFilter(dbTx, cfIndexKeys[filterType], h)
		return nil
	})
	return f, err
}

// FilterHeaderByBlockHash returns the serialized contents of a block's basic
// or extended committed filter header.
func (idx *CFIndex) FilterHeaderByBlockHash(h *chainhash.Hash, filterType wire.FilterType) ([]byte, error) {
	if uint8(filterType) > maxFilterType {
		return nil, errors.New("unsupported filter type")
	}

	var fh []byte
	err := idx.db.View(func(dbTx database.Tx) error {
		var err error
		fh, err = dbFetchFilterHeader(dbTx,
			cfHeaderKeys[filterType], h)
		return err
	})
	return fh, err
}

// NewCfIndex returns a new instance of an indexer that is used to create a
// mapping of the hashes of all blocks in the blockchain to their respective
// committed filters.
//
// It implements the Indexer interface which plugs into the IndexManager that
// in turn is used by the blockchain package. This allows the index to be
// seamlessly maintained along with the chain.
func NewCfIndex(db database.DB, chainParams *chaincfg.Params) *CFIndex {
	return &CFIndex{db: db, chainParams: chainParams}
}

// DropCfIndex drops the CF index from the provided database if exists.
func DropCfIndex(db database.DB, interrupt <-chan struct{}) error {
	return dropIndexMetadata(db, cfIndexParentBucketKey, cfIndexName)
}
