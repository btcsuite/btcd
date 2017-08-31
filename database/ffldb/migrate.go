// Copyright (c) 2015-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ffldb

import (
	"bytes"
	"container/list"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
)

const (
	// migrationSuccessVal is the single-byte value stored with a migration key
	// after the migration successfully completes.
	migrationSuccessVal = 0x01
)

var (
	// blockIdxMigrationKey is a special key stored in the blockIdxBucket after
	// all entries have been migrated to the new blockIdx2Bucket.
	blockIdxMigrationKey = []byte("migration-complete")
)

// zeroHash is the zero value hash (all zeros).  It is defined as a convenience.
var zeroHash chainhash.Hash

// migrateBlockIndex migrates all block entries from the v1 block index bucket
// to the v2 bucket. The v1 bucket stores all block entries keyed by block hash,
// whereas the v2 bucket stores the exact same values, but keyed instead by
// block height + hash.
func migrateBlockIndex(db *db) error {
	migrated := false
	err := db.Update(func(dbTx database.Tx) error {
		oldBlockIdxBucket := dbTx.Metadata().Bucket(blockIdxBucketName)
		if oldBlockIdxBucket == nil {
			return makeDbErr(database.ErrBucketNotFound, "Block index bucket does not exist", nil)
		}

		migrationStatus := oldBlockIdxBucket.Get(blockIdxMigrationKey)
		if migrationComplete(migrationStatus) {
			return nil
		}

		log.Info("Re-indexing block information in the database. This might take a while...")
		migrated = true

		newBlockIdxBucket, err := dbTx.Metadata().CreateBucketIfNotExists(
			blockIdx2BucketName)
		if err != nil {
			return err
		}

		blockHashBucket, err := dbTx.Metadata().CreateBucketIfNotExists(
			blockHashBucketName)
		if err != nil {
			return err
		}

		// Scan the old block index bucket and construct a mapping of each block
		// to all child blocks.
		childBlocksMap, err := readBlockTree(oldBlockIdxBucket)
		if err != nil {
			return err
		}

		// Use the block graph to calculate the height of each block.
		blockHeights := determineBlockHeights(childBlocksMap)

		// Now that we have heights for all blocks, scan the old block index
		// bucket and insert all rows into the new one.
		err = oldBlockIdxBucket.ForEach(func(hashBytes, blockRow []byte) error {
			var hash chainhash.Hash
			copy(hash[:], hashBytes[0:chainhash.HashSize])
			height, exists := blockHeights[hash]
			if !exists {
				message := fmt.Sprintf("Unable to calculate chain height for stored block %s", hash)
				return makeDbErr(database.ErrCorruption, message, nil)
			}

			key := blockIndexKey(&hash, height)
			err := newBlockIdxBucket.Put(key, blockRow)
			if err != nil {
				return err
			}

			// First 4 bytes of the blockIdxKey are the block height
			err = blockHashBucket.Put(hashBytes, key[0:4:4])
			return err
		})
		if err != nil {
			return err
		}

		// Set migration status to success
		err = oldBlockIdxBucket.Put(blockIdxMigrationKey,
			[]byte{migrationSuccessVal})
		return err
	})

	if err != nil {
		return err
	}

	if migrated {
		log.Infof("Block database upgrade complete")
	}
	return nil
}

// readBlockTree reads the old block index bucket and constructs a mapping of
// each block to all child blocks. This mapping represents the full tree of
// blocks.
func readBlockTree(oldBlockIdxBucket database.Bucket) (map[chainhash.Hash][]*chainhash.Hash, error) {
	childBlocksMap := make(map[chainhash.Hash][]*chainhash.Hash)
	err := oldBlockIdxBucket.ForEach(func(_, blockRow []byte) error {
		var header wire.BlockHeader
		endOffset := blockLocSize + blockHdrSize
		headerBytes := blockRow[blockLocSize:endOffset:endOffset]
		err := header.Deserialize(bytes.NewReader(headerBytes))
		if err != nil {
			return err
		}

		blockHash := header.BlockHash()
		childBlocksMap[header.PrevBlock] =
			append(childBlocksMap[header.PrevBlock], &blockHash)
		return nil
	})
	return childBlocksMap, err
}

// determineBlockHeights takes a map of block hashes to a slice of child hashes
// and uses it to compute the height for each block. The function assigns a
// height of 0 to the genesis hash and explores the tree of blocks
// breadth-first, assigning a height to every block with a path back to the
// genesis block.
func determineBlockHeights(childBlocksMap map[chainhash.Hash][]*chainhash.Hash) map[chainhash.Hash]uint32 {
	blockHeights := make(map[chainhash.Hash]uint32)
	queue := list.New()

	// The genesis block is included in childBlocksMap as a child of the zero
	// hash because that is the value of the PrevBlock field in the genesis
	// header.
	for _, genesisHash := range childBlocksMap[zeroHash] {
		blockHeights[*genesisHash] = 0
		queue.PushBack(genesisHash)
	}

	for e := queue.Front(); e != nil; e = queue.Front() {
		queue.Remove(e)
		hash := e.Value.(*chainhash.Hash)
		height := blockHeights[*hash]

		// For each block with this one as a parent, assign it a height and
		// push to queue for future processing.
		for _, childHash := range childBlocksMap[*hash] {
			blockHeights[*childHash] = height + 1
			queue.PushBack(childHash)
		}
	}
	return blockHeights
}

// migrationComplete takes a value from the database indicating the status of a
// migration and returns whether the migration has already completed.
func migrationComplete(status []byte) bool {
	return len(status) == 1 && status[0] == migrationSuccessVal
}
