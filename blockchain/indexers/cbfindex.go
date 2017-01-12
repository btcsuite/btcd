// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/gcs"

	"os"
)

const (
	// cbfIndexName is the human-readable name for the index.
	cbfIndexName = "committed bloom filter index"
)

var (
	// cbfIndexKey is the name of the db bucket used to house the
	// block hash -> CBF index.
	cbfIndexKey = []byte("cbfbyhashidx")

	// errNoCBFEntry is an error that indicates a requested entry does
	// not exist in the CBF index.
	errCBFEntry = errors.New("no entry in the block ID index")
)

// The serialized format for keys and values in the block hash to CBF bucket is:
//   <hash> = <CBF>
//
//   Field           Type              Size
//   hash            chainhash.Hash    32 bytes
//   CBF             []byte            variable
//   -----
//   Total: > 32 bytes

// CBFIndex implements a CBF by hash index.
type CBFIndex struct {
	db database.DB
}

// Ensure the CBFIndex type implements the Indexer interface.
var _ Indexer = (*CBFIndex)(nil)

// Init initializes the hash-based CBF index.
//
// This is part of the Indexer interface.
func (idx *CBFIndex) Init() error {
	return nil
}

// Key returns the database key to use for the index as a byte slice.
//
// This is part of the Indexer interface.
func (idx *CBFIndex) Key() []byte {
	return cbfIndexKey
}

// Name returns the human-readable name of the index.
//
// This is part of the Indexer interface.
func (idx *CBFIndex) Name() string {
	return cbfIndexName
}

// Create is invoked when the indexer manager determines the index needs
// to be created for the first time.  It creates the buckets for the hash-based
// CBF index.
//
// This is part of the Indexer interface.
func (idx *CBFIndex) Create(dbTx database.Tx) error {
	meta := dbTx.Metadata()
	_, err := meta.CreateBucket(cbfIndexKey)
	return err
}

func generateFilterForBlock(block *btcutil.Block) ([]byte, error) {
	txSlice := block.Transactions() // XXX can this fail?
	txHashes := make([][]byte, len(txSlice))

	for i := 0; i < len(txSlice); i++ {
		txHash, err := block.TxHash(i)
		if err != nil {
			return nil, err
		}
		txHashes = append(txHashes, txHash.CloneBytes())
	}

	var key [gcs.KeySize]byte
	P := uint8(20) // collision probability

	for i := 0; i < gcs.KeySize; i += 4 {
		binary.BigEndian.PutUint32(key[i:], uint32(0xcafebabe))
	}

	filter, err := gcs.BuildGCSFilter(P, key, txHashes)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "Generated CBF for block %v", block.Hash())

	return filter.Bytes(), nil
}

// ConnectBlock is invoked by the index manager when a new block has been
// connected to the main chain.  This indexer adds a hash-to-CBF mapping for
// every passed block.
//
// This is part of the Indexer interface.
func (idx *CBFIndex) ConnectBlock(dbTx database.Tx, block *btcutil.Block,
    view *blockchain.UtxoViewpoint) error {
	filterBytes, err := generateFilterForBlock(block)
	if err != nil {
		return err
	}

	meta := dbTx.Metadata()
	index := meta.Bucket(cbfIndexKey)
	err = index.Put(block.Hash().CloneBytes(), filterBytes)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Stored CBF for block %v", block.Hash())

	return nil
}

// DisconnectBlock is invoked by the index manager when a block has been
// disconnected from the main chain.  This indexer removes the hash-to-CBF
// mapping for every passed block.
//
// This is part of the Indexer interface.
func (idx *CBFIndex) DisconnectBlock(dbTx database.Tx, block *btcutil.Block, view *blockchain.UtxoViewpoint) error {
	return nil
}

// NewCBFIndex returns a new instance of an indexer that is used to create a
// mapping of the hashes of all blocks in the blockchain to their respective
// committed bloom filters.
//
// It implements the Indexer interface which plugs into the IndexManager that in
// turn is used by the blockchain package.  This allows the index to be
// seamlessly maintained along with the chain.
func NewCBFIndex(db database.DB) *CBFIndex {
	return &CBFIndex{db: db}
}

// DropCBFIndex drops the CBF index from the provided database if exists.
func DropCBFIndex(db database.DB) error {
	return dropIndex(db, cbfIndexKey, cbfIndexName)
}
