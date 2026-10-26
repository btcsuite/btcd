// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// addrIndexTestChain holds a chain of mainnet test blocks and its database.
type addrIndexTestChain struct {
	db      database.DB
	chain   *blockchain.BlockChain
	rootDir string
}

// setupAddrIndexTestChain creates a database and a chain holding the mainnet
// blocks from the genesis block through height.
func setupAddrIndexTestChain(t *testing.T, height int32) *addrIndexTestChain {
	t.Helper()
	rootDir := t.TempDir()
	db, err := database.Create(
		"ffldb", filepath.Join(rootDir, "db"), wire.MainNet,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	chain, err := blockchain.New(&blockchain.Config{
		DB: db, ChainParams: &chaincfg.MainNetParams,
		TimeSource: blockchain.NewMedianTime(),
	})
	require.NoError(t, err)

	c := &addrIndexTestChain{db: db, chain: chain, rootDir: rootDir}
	c.addTestBlocks(t, height)
	return c
}

// addTestBlocks adds the mainnet blocks after the chain tip through height.
func (c *addrIndexTestChain) addTestBlocks(t *testing.T, height int32) {
	t.Helper()
	f, err := os.Open("../testdata/blk_0_to_14131.dat")
	require.NoError(t, err)
	defer f.Close()
	tip := c.chain.BestSnapshot().Height
	for i := int32(0); i <= height; i++ {
		var header [2]uint32
		require.NoError(t, binary.Read(f, binary.LittleEndian, &header))
		require.Equal(t, uint32(wire.MainNet), header[0])
		data := make([]byte, header[1])
		_, err := io.ReadFull(f, data)
		require.NoError(t, err)
		if i <= tip {
			continue
		}
		block, err := btcutil.NewBlockFromBytes(data)
		require.NoError(t, err)
		_, _, err = c.chain.ProcessBlock(block, blockchain.BFNone)
		require.NoError(t, err)
	}
}

// connect adds the main chain blocks from first through last to the indexer
// with dbIndexConnectBlock, one database transaction per block.  It returns
// the number of spent outputs passed to the indexer.
func (c *addrIndexTestChain) connect(t *testing.T, indexer Indexer,
	first, last int32) int {

	t.Helper()
	spent := 0
	for height := first; height <= last; height++ {
		block, err := c.chain.BlockByHeight(height)
		require.NoError(t, err)
		var stxos []blockchain.SpentTxOut
		if indexNeedsInputs(indexer) {
			stxos, err = c.chain.FetchSpendJournal(block)
			require.NoError(t, err)
			spent += len(stxos)
		}
		require.NoError(t, c.db.Update(func(dbTx database.Tx) error {
			return dbIndexConnectBlock(dbTx, indexer, block, stxos)
		}))
	}
	return spent
}

// readAddrIndexTestBucket returns a copy of every level in the address index
// bucket, keyed by level key.
func readAddrIndexTestBucket(t *testing.T,
	db database.DB) map[[levelKeySize]byte][]byte {

	t.Helper()
	entries := make(map[[levelKeySize]byte][]byte)
	require.NoError(t, db.View(func(dbTx database.Tx) error {
		return dbTx.Metadata().Bucket(addrIndexKey).ForEach(
			func(key, value []byte) error {
				var levelKey [levelKeySize]byte
				copy(levelKey[:], key)
				valueCopy := append([]byte(nil), value...)
				entries[levelKey] = valueCopy
				return nil
			},
		)
	}))
	return entries
}
