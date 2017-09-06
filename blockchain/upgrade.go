// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"container/list"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
)

const (
	// blockHdrOffset defines the offsets into a v1 block index row for the
	// block header.
	//
	// The serialized block index row format is:
	//   <blocklocation><blockheader>
	blockHdrOffset = 12
)

// migrateBlockIndex migrates all block entries from the v1 block index bucket
// to the v2 bucket. The v1 bucket stores all block entries keyed by block hash,
// whereas the v2 bucket stores the exact same values, but keyed instead by
// block height + hash.
func migrateBlockIndex(db database.DB) error {
	// Hardcoded bucket names so updates to the global values do not affect
	// old upgrades.
	v1BucketName := []byte("ffldb-blockidx")
	v2BucketName := []byte("blockheaderidx")

	err := db.Update(func(dbTx database.Tx) error {
		v1BlockIdxBucket := dbTx.Metadata().Bucket(v1BucketName)
		if v1BlockIdxBucket == nil {
			return fmt.Errorf("Bucket %s does not exist", v1BucketName)
		}

		log.Info("Re-indexing block information in the database. This might take a while...")

		v2BlockIdxBucket, err :=
			dbTx.Metadata().CreateBucketIfNotExists(v2BucketName)
		if err != nil {
			return err
		}

		// Scan the old block index bucket and construct a mapping of each block
		// to all child blocks.
		childBlocksMap, err := readBlockTree(v1BlockIdxBucket)
		if err != nil {
			return err
		}

		// Use the block graph to calculate the height of each block.
		blockHeights := determineBlockHeights(childBlocksMap)

		// Now that we have heights for all blocks, scan the old block index
		// bucket and insert all rows into the new one.
		return v1BlockIdxBucket.ForEach(func(hashBytes, blockRow []byte) error {
			endOffset := blockHdrOffset + blockHdrSize
			headerBytes := blockRow[blockHdrOffset:endOffset:endOffset]

			var hash chainhash.Hash
			copy(hash[:], hashBytes[0:chainhash.HashSize])

			height, exists := blockHeights[hash]
			if !exists {
				return fmt.Errorf("Unable to calculate chain height for "+
					"stored block %s", hash)
			}

			key := blockIndexKey(&hash, height)
			return v2BlockIdxBucket.Put(key, headerBytes)
		})
	})
	if err != nil {
		return err
	}

	log.Infof("Block database migration complete")
	return nil
}

// readBlockTree reads the old block index bucket and constructs a mapping of
// each block to all child blocks. This mapping represents the full tree of
// blocks.
func readBlockTree(v1BlockIdxBucket database.Bucket) (map[chainhash.Hash][]*chainhash.Hash, error) {
	childBlocksMap := make(map[chainhash.Hash][]*chainhash.Hash)
	err := v1BlockIdxBucket.ForEach(func(_, blockRow []byte) error {
		var header wire.BlockHeader
		endOffset := blockHdrOffset + blockHdrSize
		headerBytes := blockRow[blockHdrOffset:endOffset:endOffset]
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
