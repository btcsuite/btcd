// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"

	database "github.com/btcsuite/btcd/database2"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// Index represents a blockchain index, which is responsible for indexing
// a particular aspect of the blockchain for fast retrieval.
// When creating a BlockChain, it's possible to assign ChainIndexes to it.
// The BlockChain instance will call the callbacks to ensure the indexes
// are always up to date.
type Index interface {
	// Init initializes the Index for use with the given BlockChain.
	Init(b *BlockChain)

	// Name returns the index's name. It should be unique per index type.
	// It is used for the bucket name so it should be unique.
	Name() string

	// Version returns the index database version. The purpose of this is
	// allowing indexes to change their database format in an incompatible way
	// without having to check for leftover data from a previous version.
	// On startup, the database version is checked against the version reported
	// by the Index instance, and if they don't match the index bucket is
	// dropped and the index is caught up from scratch.
	Version() int32

	// Create initializes the necessary data structures for the index
	// in the database using an existing transaction. It creates buckets and
	// fills them with initial data as needed.
	Create(dbTx database.Tx, bucket database.Bucket) error

	// ConnectBlock indexes the given block using an existing database
	// transaction. The UtxoViewpoint is only present if the block is being
	// connected to the chain right now, otherwise (e.g during catchup) it
	// is nil.
	ConnectBlock(dbTx database.Tx, bucket database.Bucket, block *btcutil.Block, view *UtxoViewpoint) error

	// DisconnectBlock de-indexes the given block using an existing database
	// transaction. The UtxoViewpoint is only present if the block is being
	// disconnected from the chain right now, otherwise (e.g during catchup) it
	// is nil.
	DisconnectBlock(dbTx database.Tx, bucket database.Bucket, block *btcutil.Block, view *UtxoViewpoint) error
}

var (
	indexesBucketName  = []byte("idxs")
	indexBucketPrefix  = "b"
	indexTipPrefix     = "t"
	indexVersionPrefix = "v"
)

// errNoSuchIndex signifies that the requested index is not present
// in the database.
type errNoSuchIndex string

// Error implements the error interface.
func (e errNoSuchIndex) Error() string {
	return string(e)
}

// isNoSuchIndexErr returns whether or not the passed error is an
// errNoSuchIndex error.
func isNoSuchIndexErr(err error) bool {
	_, ok := err.(errNoSuchIndex)
	return ok
}

// indexFetchTip fetches the current index tip for the requested index.
//
// The database transaction parameter can be nil in which case a new one will
// be used.
func (b *BlockChain) indexFetchTip(dbTx database.Tx, indexName string) (*wire.ShaHash, int32, error) {
	if dbTx == nil {
		var hash *wire.ShaHash
		var height int32
		err := b.db.View(func(dbTx database.Tx) error {
			var err error
			hash, height, err = b.indexFetchTip(dbTx, indexName)
			return err
		})
		return hash, height, err
	}

	bucket := dbTx.Metadata().Bucket(indexesBucketName)
	data := bucket.Get([]byte(indexTipPrefix + indexName))
	if data == nil {
		str := fmt.Sprintf("index %s tip not in db", indexName)
		return nil, 0, errNoSuchIndex(str)
	}

	hash, err := wire.NewShaHash(data[:wire.HashSize])
	if err != nil {
		return nil, 0, err
	}
	height := int32(byteOrder.Uint32(data[wire.HashSize:]))

	return hash, height, nil
}

// indexUpdateTip updates the index tip for the requested index.
func (b *BlockChain) indexUpdateTip(dbTx database.Tx, indexName string, hash *wire.ShaHash, height int32) error {
	bucket := dbTx.Metadata().Bucket(indexesBucketName)

	data := make([]byte, wire.HashSize+4)
	copy(data, hash[:])
	byteOrder.PutUint32(data[wire.HashSize:], uint32(height))

	bucket.Put([]byte(indexTipPrefix+indexName), data)
	return nil
}

// indexFetchVersion fetches the current index version for the requested index.
//
// The database transaction parameter can be nil in which case a new one will
// be used.
func (b *BlockChain) indexFetchVersion(dbTx database.Tx, indexName string) (int32, error) {
	if dbTx == nil {
		var version int32
		err := b.db.View(func(dbTx database.Tx) error {
			var err error
			version, err = b.indexFetchVersion(dbTx, indexName)
			return err
		})
		return version, err
	}

	bucket := dbTx.Metadata().Bucket(indexesBucketName)
	data := bucket.Get([]byte(indexVersionPrefix + indexName))
	if data == nil {
		str := fmt.Sprintf("index %s version not in db", indexName)
		return 0, errNoSuchIndex(str)
	}
	version := int32(byteOrder.Uint32(data))

	return version, nil
}

// indexUpdateVersuib updates the index version for the requested index.
func (b *BlockChain) indexUpdateVersion(dbTx database.Tx, indexName string, version int32) error {
	bucket := dbTx.Metadata().Bucket(indexesBucketName)

	data := make([]byte, 4)
	byteOrder.PutUint32(data, uint32(version))

	bucket.Put([]byte(indexVersionPrefix+indexName), data)
	return nil
}

// indexConnectBlock indexes the given block with the given index, and updates
// the chain tip to reflect the block has been indexed.
func (b *BlockChain) indexConnectBlock(dbTx database.Tx, index Index, block *btcutil.Block, view *UtxoViewpoint) error {
	// Fetch the index bucket
	bucket := b.IndexBucket(dbTx, index.Name())

	// Call callback on the index
	err := index.ConnectBlock(dbTx, bucket, block, view)
	if err != nil {
		return err
	}

	// Update the tip to reflect this block was just indexed
	err = b.indexUpdateTip(dbTx, index.Name(), block.Sha(), block.Height())
	if err != nil {
		return err
	}
	return nil

}

// indexDisconnectBlock de-indexes the given block with the given index, and
// updates the chain tip to reflect the block has been de-indexed.
func (b *BlockChain) indexDisconnectBlock(dbTx database.Tx, index Index, block *btcutil.Block, view *UtxoViewpoint) error {
	// Fetch the index bucket
	bucket := b.IndexBucket(dbTx, index.Name())

	// Call callback on the index
	err := index.DisconnectBlock(dbTx, bucket, block, view)
	if err != nil {
		return err
	}

	// Update the tip to reflect this block was just indexed
	err = b.indexUpdateTip(dbTx, index.Name(), &block.MsgBlock().Header.PrevBlock, block.Height()-1)
	if err != nil {
		return err
	}
	return nil
}

// IndexBucket returns the index bucket for the given index name,
// or nil if it does not exist.
func (b *BlockChain) IndexBucket(dbTx database.Tx, indexName string) database.Bucket {
	return dbTx.Metadata().Bucket(indexesBucketName).Bucket([]byte(indexBucketPrefix + indexName))
}

// InitIndexes ensures that all indexes registered in the blockchain
// instance are created and are up to date. If needed, it will create and catch
// up the indexes to the current best state. It does not return until finished.
//
// NOTE: This function is not safe for concurrent access. In particular, blocks
// should not be connected or disconnected from the blockchain while it is
// running, so it must be run before starting blockManager or IBD.
func (b *BlockChain) InitIndexes() error {
	// Create new indexes.
	for _, index := range b.indexes {
		// Whether the index needs to be created or not.
		needsCreation := false

		// Check the index version.
		version, err := b.indexFetchVersion(nil, index.Name())
		if err != nil {
			if isNoSuchIndexErr(err) {
				// If it failed to fetch due to the index not existing, we have
				// to create it.
				needsCreation = true
			} else {
				// Otherwise it failed due to some other reason.
				return err
			}
		} else {
			// Index exists on db, check the version matches.
			if version != index.Version() {
				log.Infof("Version mismatch on index %s, dropping it: database %d, index %d",
					index.Name(), version, index.Version())

				// Drop the index.
				err := b.db.Update(func(dbTx database.Tx) error {
					indexesBucket := dbTx.Metadata().Bucket(indexesBucketName)
					return indexesBucket.DeleteBucket([]byte(indexBucketPrefix + index.Name()))
				})
				if err != nil {
					return err
				}

				needsCreation = true
			}
		}

		// If index does not exist, create it.
		if needsCreation {
			log.Infof("Index %s does not exist, creating it.", index.Name())

			err := b.db.Update(func(dbTx database.Tx) error {
				indexesBucket := dbTx.Metadata().Bucket(indexesBucketName)
				bucket, err := indexesBucket.CreateBucket([]byte(indexBucketPrefix + index.Name()))
				if err != nil {
					return err
				}
				// Set the tip to the genesis block
				err = b.indexUpdateTip(dbTx, index.Name(), b.chainParams.GenesisHash, 0)
				if err != nil {
					return err
				}
				// Set the version.
				err = b.indexUpdateVersion(dbTx, index.Name(), index.Version())
				if err != nil {
					return err
				}
				// Call the index callback for it to populate its
				// initial data if needed.
				err = index.Create(dbTx, bucket)
				if err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
	}

	// Bring indexes back to main chain if their tip is in an orphaned fork.
	// This is unlikely to happen, but can happen if btcd is launched with
	// an index enabled, then with it disabled, and a reorganize
	// happens that leaves the index tip orphaned.
	for _, index := range b.indexes {
		// Fetch the current index tip.
		hash, height, err := b.indexFetchTip(nil, index.Name())
		if err != nil {
			return err
		}
		initialHeight := height

		// Loop until the tip is a block that exists in the main chain.
		for {
			// Check the current tip exists.
			exists, err := b.MainChainHasBlock(nil, hash)
			if err != nil {
				return err
			}
			if exists {
				break
			}

			err = b.db.Update(func(dbTx database.Tx) error {
				// The current tip is orphaned. Remove the block from the addrindex.
				// We fetch the block bytes straight from db because using
				// BlockChain.BlockByHash would error since it's not in the main
				// chain.
				blockBytes, err := dbTx.FetchBlock(hash)
				if err != nil {
					return err
				}

				// Create the encapsulated block and set the height appropriately.
				block, err := btcutil.NewBlockFromBytes(blockBytes)
				if err != nil {
					return err
				}
				block.SetHeight(height)

				// Remove the block.
				err = b.indexDisconnectBlock(dbTx, index, block, nil)
				if err != nil {
					return err
				}

				// Update tip to the previous block.
				hash = &block.MsgBlock().Header.PrevBlock
				height--

				return nil
			})
			if err != nil {
				return err
			}
		}

		if initialHeight != height {
			log.Infof("Removed %d orphaned blocks from index (heights %d to %d)",
				initialHeight-height, initialHeight, height)
		}
	}

	// Fetch current index heights
	bestHeight := b.BestSnapshot().Height
	lowestHeight := bestHeight
	indexHeights := make([]int32, len(b.indexes))
	for i, index := range b.indexes {
		hash, height, err := b.indexFetchTip(nil, index.Name())
		log.Infof("Index tip: %s %s %d", index.Name(), hash, height)
		indexHeights[i] = height
		if err != nil {
			return err
		}
		if height < lowestHeight {
			lowestHeight = height
		}
	}

	if lowestHeight == bestHeight {
		log.Infof("All indexes are caught up.")
		return nil
	}

	log.Infof("Catching up indexes from height %d to %d",
		lowestHeight, bestHeight)

	// Loop from lowest tip height to best height, updating all indexes.
	for height := lowestHeight + 1; height <= bestHeight; height++ {
		block, err := b.BlockByHeight(height)
		if err != nil {
			return err
		}
		for i, index := range b.indexes {
			if indexHeights[i] < height {
				err := b.db.Update(func(dbTx database.Tx) error {
					return b.indexConnectBlock(dbTx, index, block, nil)
				})
				if err != nil {
					return err
				}
				indexHeights[i] = height
			}
		}
	}

	return nil
}
