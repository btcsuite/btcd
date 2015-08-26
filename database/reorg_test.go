// Copyright (c) 2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database_test

import (
	"bytes"
	"compress/bzip2"
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrutil"
)

// testReorganization performs reorganization tests for the passed DB type.
// Much of the setup is copied from the blockchain package, but the test looks
// to see if each TX in each block in the best chain can be fetched using
// FetchTxBySha. If not, then there's a bug.
func testReorganization(t *testing.T, dbType string) {
	db, teardown, err := createDB(dbType, "reorganization", true)
	if err != nil {
		t.Fatalf("Failed to create test database (%s) %v", dbType, err)
	}
	defer teardown()

	blocks, err := loadReorgBlocks("reorgto179.bz2")
	if err != nil {
		t.Fatalf("Error loading file: %v", err)
	}
	blocksReorg, err := loadReorgBlocks("reorgto180.bz2")
	if err != nil {
		t.Fatalf("Error loading file: %v", err)
	}

	// Find where chain forks
	var forkHash chainhash.Hash
	var forkHeight int64
	for i := range blocks {
		if blocks[i].Sha().IsEqual(blocksReorg[i].Sha()) {
			blkHash := blocks[i].Sha()
			forkHash = *blkHash
			forkHeight = int64(i)
		}
	}

	// Insert all blocks from chain 1
	for i := int64(0); i < int64(len(blocks)); i++ {
		blkHash := blocks[i].Sha()
		if err != nil {
			t.Fatalf("Error getting SHA for block %dA: %v", i-2, err)
		}

		_, err = db.InsertBlock(blocks[i])
		if err != nil {
			t.Fatalf("Error inserting block %dA (%v): %v", i-2, blkHash, err)
		}
	}

	// Remove blocks to fork point
	db.DropAfterBlockBySha(&forkHash)
	if err != nil {
		t.Errorf("couldn't DropAfterBlockBySha: %v", err.Error())
	}

	// Insert blocks from the other chain to simulate a reorg
	for i := forkHeight + 1; i < int64(len(blocksReorg)); i++ {
		blkHash := blocksReorg[i].Sha()
		if err != nil {
			t.Fatalf("Error getting SHA for block %dA: %v", i-2, err)
		}
		_, err = db.InsertBlock(blocksReorg[i])
		if err != nil {
			t.Fatalf("Error inserting block %dA (%v): %v", i-2, blkHash, err)
		}
	}

	_, maxHeight, err := db.NewestSha()
	if err != nil {
		t.Fatalf("Error getting newest block info")
	}

	for i := int64(0); i <= maxHeight; i++ {
		blkHash, err := db.FetchBlockShaByHeight(i)
		if err != nil {
			t.Fatalf("Error fetching SHA for block %d: %v", i, err)
		}
		block, err := db.FetchBlockBySha(blkHash)
		if err != nil {
			t.Fatalf("Error fetching block %d (%v): %v", i, blkHash, err)
		}
		prevBlockSha := block.MsgBlock().Header.PrevBlock
		prevBlock, _ := db.FetchBlockBySha(&prevBlockSha)
		votebits := blocksReorg[i].MsgBlock().Header.VoteBits
		if dcrutil.IsFlagSet16(votebits, dcrutil.BlockValid) && prevBlock != nil {
			for _, tx := range prevBlock.Transactions() {
				_, err := db.FetchTxBySha(tx.Sha())
				if err != nil {
					t.Fatalf("Error fetching transaction %v: %v", tx.Sha(), err)
				}
			}
		}
		for _, tx := range block.STransactions() {
			_, err := db.FetchTxBySha(tx.Sha())
			if err != nil {
				t.Fatalf("Error fetching transaction %v: %v", tx.Sha(), err)
			}
		}
	}
}

// loadReorgBlocks reads files containing decred block data (bzipped but
// otherwise in the format bitcoind writes) from disk and returns them as an
// array of dcrutil.Block. This is copied from the blockchain package, which
// itself largely borrowed it from the test code in this package.
func loadReorgBlocks(filename string) ([]*dcrutil.Block, error) {
	filename = filepath.Join("../blockchain/testdata/", filename)
	fi, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder := gob.NewDecoder(bcBuf)
	blockchain := make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&blockchain); err != nil {
		return nil, err
	}

	var block *dcrutil.Block

	blocks := make([]*dcrutil.Block, 0, len(blockchain))
	for height := int64(0); height < int64(len(blockchain)); height++ {
		block, err = dcrutil.NewBlockFromBytes(blockchain[height])
		if err != nil {
			return blocks, err
		}
		block.SetHeight(height)
		blocks = append(blocks, block)
	}

	return blocks, nil
}
