// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database_test

import (
	"compress/bzip2"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
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

	blocks, err := loadReorgBlocks("reorgblocks.bz2")
	if err != nil {
		t.Fatalf("Error loading file: %v", err)
	}

	for i := int64(0); i <= 2; i++ {
		_, err = db.InsertBlock(blocks[i])
		if err != nil {
			t.Fatalf("Error inserting block %d (%v): %v", i,
				blocks[i].Sha(), err)
		}
		var txIDs []string
		for _, tx := range blocks[i].Transactions() {
			txIDs = append(txIDs, tx.Sha().String())
		}
	}

	for i := int64(1); i >= 0; i-- {
		blkHash := blocks[i].Sha()
		err = db.DropAfterBlockBySha(blkHash)
		if err != nil {
			t.Fatalf("Error removing block %d for reorganization: %v", i, err)
		}
		// Exercise NewestSha() to make sure DropAfterBlockBySha() updates the
		// info correctly
		maxHash, blkHeight, err := db.NewestSha()
		if err != nil {
			t.Fatalf("Error getting newest block info")
		}
		if !maxHash.IsEqual(blkHash) || blkHeight != i {
			t.Fatalf("NewestSha returned %v (%v), expected %v (%v)", blkHeight,
				maxHash, i, blkHash)
		}
	}

	for i := int64(3); i < int64(len(blocks)); i++ {
		blkHash := blocks[i].Sha()
		if err != nil {
			t.Fatalf("Error getting SHA for block %dA: %v", i-2, err)
		}
		_, err = db.InsertBlock(blocks[i])
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
		for _, tx := range block.Transactions() {
			_, err := db.FetchTxBySha(tx.Sha())
			if err != nil {
				t.Fatalf("Error fetching transaction %v: %v", tx.Sha(), err)
			}
		}
	}
}

// loadReorgBlocks reads files containing bitcoin block data (bzipped but
// otherwise in the format bitcoind writes) from disk and returns them as an
// array of btcutil.Block. This is copied from the blockchain package, which
// itself largely borrowed it from the test code in this package.
func loadReorgBlocks(filename string) ([]*btcutil.Block, error) {
	filename = filepath.Join("testdata/", filename)

	var blocks []*btcutil.Block
	var err error

	var network = wire.SimNet
	var dr io.Reader
	var fi io.ReadCloser

	fi, err = os.Open(filename)
	if err != nil {
		return blocks, err
	}

	if strings.HasSuffix(filename, ".bz2") {
		dr = bzip2.NewReader(fi)
	} else {
		dr = fi
	}
	defer fi.Close()

	var block *btcutil.Block

	err = nil
	for height := int64(1); err == nil; height++ {
		var rintbuf uint32
		err = binary.Read(dr, binary.LittleEndian, &rintbuf)
		if err == io.EOF {
			// hit end of file at expected offset: no warning
			height--
			err = nil
			break
		}
		if err != nil {
			break
		}
		if rintbuf != uint32(network) {
			break
		}
		err = binary.Read(dr, binary.LittleEndian, &rintbuf)
		if err != nil {
			return blocks, err
		}
		blocklen := rintbuf

		rbytes := make([]byte, blocklen)

		// read block
		numbytes, err := dr.Read(rbytes)
		if err != nil {
			return blocks, err
		}
		if uint32(numbytes) != blocklen {
			return blocks, io.ErrUnexpectedEOF
		}

		block, err = btcutil.NewBlockFromBytes(rbytes)
		if err != nil {
			return blocks, err
		}
		blocks = append(blocks, block)
	}

	return blocks, nil
}
