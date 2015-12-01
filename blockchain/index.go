// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
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
	// It is used to identify
	Name() string

	// Create initializes the necessary data structures for the index
	// in the database using an existing transaction. It creates buckets and
	// fills them with initial data as needed.
	Create(dbTx database.Tx, bucket database.Bucket) error

	// ConnectBlock indexes the given block using an existing database
	// transaction.
	ConnectBlock(dbTx database.Tx, bucket database.Bucket, block *btcutil.Block) error

	// DisconnectBlock de-indexes the given block using an existing database
	// transaction.
	DisconnectBlock(dbTx database.Tx, bucket database.Bucket, block *btcutil.Block) error
}

var (
	indexTipsBucketName = []byte("idxtips")
	indexesBucketName   = []byte("idxs")
)

// indexFetchTip fetches the current index tip for the requested index.
//
// The database transaction parameter can be nil in which case a a new one will
// be used.
func (b *BlockChain) indexFetchTip(dbTx database.Tx, indexNum int) (hash *wire.ShaHash, height int32, err error) {
	if dbTx == nil {
		err = b.db.View(func(dbTx database.Tx) error {
			var err error
			hash, height, err = b.indexFetchTip(dbTx, indexNum)
			return err
		})
		return
	}

	bucket := dbTx.Metadata().Bucket(indexTipsBucketName)
	data := bucket.Get([]byte(b.indexes[indexNum].Name()))

	hash, err = wire.NewShaHash(data[:wire.HashSize])
	if err != nil {
		return nil, 0, err
	}
	height = int32(byteOrder.Uint32(data[wire.HashSize:]))

	return
}

// indexUpdateTip updates the index tip for the requested index.
func (b *BlockChain) indexUpdateTip(dbTx database.Tx, indexNum int, hash *wire.ShaHash, height int32) error {
	bucket := dbTx.Metadata().Bucket(indexTipsBucketName)

	data := make([]byte, wire.HashSize+4)
	copy(data, hash[:])
	byteOrder.PutUint32(data[wire.HashSize:], uint32(height))

	bucket.Put([]byte(b.indexes[indexNum].Name()), data)
	return nil
}

// indexConnectBlock indexes the given block with the given index, and updates
// the chain tip to reflect the block has been indexed.
func (b *BlockChain) indexConnectBlock(dbTx database.Tx, indexNum int, block *btcutil.Block) error {
	// Fetch the index bucket
	bucket := b.GetIndexBucket(dbTx, b.indexes[indexNum].Name())

	// Call callback on the index
	err := b.indexes[indexNum].ConnectBlock(dbTx, bucket, block)
	if err != nil {
		return err
	}

	// Update the tip to reflect this block was just indexed
	err = b.indexUpdateTip(dbTx, indexNum, block.Sha(), block.Height())
	if err != nil {
		return err
	}
	return nil

}

// indexDisconnectBlock de-indexes the given block with the given index, and
// updates the chain tip to reflect the block has been de-indexed.
func (b *BlockChain) indexDisconnectBlock(dbTx database.Tx, indexNum int, block *btcutil.Block) error {
	// Fetch the index bucket
	bucket := b.GetIndexBucket(dbTx, b.indexes[indexNum].Name())

	// Call callback on the index
	err := b.indexes[indexNum].DisconnectBlock(dbTx, bucket, block)
	if err != nil {
		return err
	}

	// Update the tip to reflect this block was just indexed
	err = b.indexUpdateTip(dbTx, indexNum, &block.MsgBlock().Header.PrevBlock, block.Height()-1)
	if err != nil {
		return err
	}
	return nil
}

// GetIndexBucket returns the index bucket for the given index name,
// or nil if it does not exist.
func (b *BlockChain) GetIndexBucket(dbTx database.Tx, indexName string) database.Bucket {
	return dbTx.Metadata().Bucket(indexesBucketName).Bucket([]byte(indexName))
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
	err := b.db.Update(func(dbTx database.Tx) error {
		indexesBucket := dbTx.Metadata().Bucket(indexesBucketName)
		for i, index := range b.indexes {
			// If index does not exist, create it.
			if indexesBucket.Bucket([]byte(index.Name())) == nil {
				bucket, err := indexesBucket.CreateBucket([]byte(index.Name()))
				if err != nil {
					return err
				}
				// Call the index callback for it to populate its
				// initial data if needed.
				err = index.Create(dbTx, bucket)
				if err != nil {
					return err
				}
				// Set the tip to the genesis block
				err = b.indexUpdateTip(dbTx, i, b.chainParams.GenesisHash, 0)
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Bring indexes back to main chain if their tip is in an orphaned fork.
	// This is unlikely to happen, but can happen if btcd is launched with
	// an index enabled, then with it disabled, and a reorganize
	// happens that leaves the index tip orphaned.
	for i := range b.indexes {
		// Fetch the current index tip.
		hash, height, err := b.indexFetchTip(nil, i)
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
				err = b.indexDisconnectBlock(dbTx, i, block)
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
	for i := range b.indexes {
		_, height, err := b.indexFetchTip(nil, i)
		indexHeights[i] = height
		if err != nil {
			return err
		}
		if height < lowestHeight {
			lowestHeight = height
		}
	}

	log.Infof("Catching up indexes from height %d to %d",
		lowestHeight, bestHeight)

	// Loop from lowest tip height to best height, updating all indexes.
	for height := lowestHeight + 1; height <= bestHeight; height++ {
		block, err := b.BlockByHeight(height)
		if err != nil {
			return err
		}
		for i := range b.indexes {
			if indexHeights[i] < height {
				err := b.db.Update(func(dbTx database.Tx) error {
					return b.indexConnectBlock(dbTx, i, block)
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
