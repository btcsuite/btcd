// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
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

func (c *addrIndexTestChain) resetAddrIndex(t *testing.T, idx *AddrIndex,
	height int32, entries map[[levelKeySize]byte][]byte) {

	t.Helper()
	hash := &chainhash.Hash{}
	if height >= 0 {
		var err error
		hash, err = c.chain.BlockHashByHeight(height)
		require.NoError(t, err)
	}
	require.NoError(t, c.db.Update(func(dbTx database.Tx) error {
		err := dbTx.Metadata().DeleteBucket(addrIndexKey)
		if err != nil {
			return err
		}
		if err := idx.Create(dbTx); err != nil {
			return err
		}
		bucket := dbTx.Metadata().Bucket(addrIndexKey)
		for key, value := range entries {
			if err := bucket.Put(key[:], value); err != nil {
				return err
			}
		}
		return dbPutIndexerTip(dbTx, addrIndexKey, hash, height)
	}))
}

type interruptAddrWriteDB struct {
	database.DB
	interrupt chan struct{}
	remaining int
}

func (db *interruptAddrWriteDB) Update(fn func(database.Tx) error) error {
	if err := db.DB.Update(fn); err != nil {
		return err
	}
	db.remaining--
	if db.remaining == 0 {
		close(db.interrupt)
	}
	return nil
}

func (db *interruptAddrWriteDB) Flush() error {
	return flushAddrIndexDB(db.DB)
}

func (c *addrIndexTestChain) interruptBuild(t *testing.T, updates int) {
	t.Helper()
	db := &interruptAddrWriteDB{
		DB: c.db, interrupt: make(chan struct{}), remaining: updates,
	}
	idx := NewAddrIndexWithDataDir(db, &chaincfg.MainNetParams, c.rootDir)
	require.ErrorIs(t, idx.FastBuild(c.chain, db.interrupt),
		errInterruptRequested)
	require.Zero(t, db.remaining)
	_, exists := AddrIndexFastBuildDir(c.rootDir)
	require.True(t, exists)
}

// TestAddrIndexFastBuildChainParity includes spent outputs so scan omissions
// cannot be hidden by tests that only supply prepared address records.
func TestAddrIndexFastBuildChainParity(t *testing.T) {
	c := setupAddrIndexTestChain(t, 14131)
	txManager := NewManager(c.db, []Indexer{NewTxIndex(c.db)})
	require.NoError(t, txManager.Init(c.chain, nil))
	idx := NewAddrIndexWithDataDir(c.db, &chaincfg.MainNetParams, c.rootDir)
	require.NoError(t, NewManager(c.db, []Indexer{idx}).createIndexes())

	spent := c.connect(t, idx, 0, 14131)
	require.Positive(t, spent)
	reference := readAddrIndexTestBucket(t, c.db)

	for _, writtenShards := range []int{0, 1, 39, 199} {
		name := fmt.Sprintf("shards_%d", writtenShards)
		t.Run(name, func(t *testing.T) {
			c.resetAddrIndex(t, idx, -1, nil)
			if writtenShards > 0 {
				// Each shard fits in one transaction.
				c.interruptBuild(t, writtenShards)
			}
			require.NoError(t, idx.FastBuild(c.chain, nil))
			got := readAddrIndexTestBucket(t, c.db)
			requireAddrBuildLevels(t, got, reference)
			_, exists := AddrIndexFastBuildDir(c.rootDir)
			require.False(t, exists)
		})
	}
}

// TestAddrIndexFastBuildResumeAfterChainGrowth scans in several chunks,
// interrupts the write pass, and resumes the build after the chain grows.
func TestAddrIndexFastBuildResumeAfterChainGrowth(t *testing.T) {
	const (
		firstTarget = 500
		target      = 1000
		chunkSize   = 100
	)
	c := setupAddrIndexTestChain(t, firstTarget)
	txIdx := NewTxIndex(c.db)
	idx := NewAddrIndexWithDataDir(c.db, &chaincfg.MainNetParams, c.rootDir)
	manager := NewManager(c.db, []Indexer{txIdx, idx})
	require.NoError(t, manager.createIndexes())
	c.connect(t, txIdx, 0, firstTarget)

	// Interrupt the write pass after its first database transaction.
	db := &interruptAddrWriteDB{
		DB: c.db, interrupt: make(chan struct{}), remaining: 1,
	}
	interrupted := NewAddrIndexWithDataDir(
		db, &chaincfg.MainNetParams, c.rootDir,
	)
	_, _, err := interrupted.buildAddrIndexFromChain(
		c.chain, chunkSize, db.interrupt,
	)
	require.ErrorIs(t, err, errInterruptRequested)

	// The resumed build scans the blocks after the first target and writes
	// the index through the new chain tip.
	c.addTestBlocks(t, target)
	c.connect(t, txIdx, firstTarget+1, target)
	_, _, err = idx.buildAddrIndexFromChain(c.chain, chunkSize, nil)
	require.NoError(t, err)
	got := readAddrIndexTestBucket(t, c.db)
	c.resetAddrIndex(t, idx, -1, nil)
	c.connect(t, idx, 0, target)
	requireAddrBuildLevels(t, got, readAddrIndexTestBucket(t, c.db))
}

// TestAddrIndexFastBuildBlockIDGap creates a gap by rolling back a database
// transaction after TxIndex.ConnectBlock advances its in-memory ID.
func TestAddrIndexFastBuildBlockIDGap(t *testing.T) {
	const target = 1005
	c := setupAddrIndexTestChain(t, target)
	txIdx := NewTxIndex(c.db)
	idx := NewAddrIndexWithDataDir(c.db, &chaincfg.MainNetParams, c.rootDir)
	manager := NewManager(c.db, []Indexer{txIdx, idx})
	require.NoError(t, manager.createIndexes())
	c.connect(t, txIdx, 0, 0)
	block, err := c.chain.BlockByHeight(1)
	require.NoError(t, err)
	rollbackErr := errors.New("transaction rollback")
	err = c.db.Update(func(dbTx database.Tx) error {
		if err := txIdx.ConnectBlock(dbTx, block, nil); err != nil {
			return err
		}
		return rollbackErr
	})
	require.ErrorIs(t, err, rollbackErr)
	c.connect(t, txIdx, 1, target)
	c.connect(t, idx, 0, target)
	reference := readAddrIndexTestBucket(t, c.db)

	for _, interrupted := range []bool{false, true} {
		name := fmt.Sprintf("interrupt_%t", interrupted)
		t.Run(name, func(t *testing.T) {
			c.resetAddrIndex(t, idx, -1, nil)
			if interrupted {
				c.interruptBuild(t, 2)
			}
			require.NoError(t, manager.Init(c.chain, nil))
			got := readAddrIndexTestBucket(t, c.db)
			requireAddrBuildLevels(t, got, reference)
		})
	}

	t.Run("transaction_index_drop", func(t *testing.T) {
		c.resetAddrIndex(t, idx, -1, nil)
		c.interruptBuild(t, 2)
		require.NoError(t, DropTxIndex(c.db, nil))
		manager := NewManager(c.db, []Indexer{NewTxIndex(c.db), idx})
		require.NoError(t, manager.Init(c.chain, nil))
		got := readAddrIndexTestBucket(t, c.db)

		// Rebuilding the transaction index assigns a fresh set of IDs.
		c.resetAddrIndex(t, idx, -1, nil)
		c.connect(t, idx, 0, target)
		requireAddrBuildLevels(t, got, readAddrIndexTestBucket(t, c.db))
	})
}

// TestAddrIndexFastBuildDecreasingBlockID gives a block an unused ID below the
// IDs of the blocks before it, as TxIndex does when it resumes below a gap
// left by a rolled back transaction.
func TestAddrIndexFastBuildDecreasingBlockID(t *testing.T) {
	const target = 1100
	c := setupAddrIndexTestChain(t, target)
	txIdx := NewTxIndex(c.db)
	idx := NewAddrIndexWithDataDir(c.db, &chaincfg.MainNetParams, c.rootDir)
	manager := NewManager(c.db, []Indexer{txIdx, idx})
	require.NoError(t, manager.createIndexes())

	// Heights 0 to 4 take IDs 1 to 5 and the rolled back connect of height
	// 5 leaves ID 6 unused.  Heights 5 to 10 take IDs 7 to 12, height 11
	// takes ID 6, and later heights take IDs from 13.
	c.connect(t, txIdx, 0, 4)
	block, err := c.chain.BlockByHeight(5)
	require.NoError(t, err)
	rollbackErr := errors.New("transaction rollback")
	err = c.db.Update(func(dbTx database.Tx) error {
		if err := txIdx.ConnectBlock(dbTx, block, nil); err != nil {
			return err
		}
		return rollbackErr
	})
	require.ErrorIs(t, err, rollbackErr)
	c.connect(t, txIdx, 5, 10)
	txIdx.curBlockID = 5
	c.connect(t, txIdx, 11, 11)
	require.NoError(t, txIdx.Init())
	c.connect(t, txIdx, 12, target)
	require.NoError(t, c.db.View(func(dbTx database.Tx) error {
		for height, want := range map[int32]uint32{
			4: 5, 5: 7, 10: 12, 11: 6, 12: 13, target: target + 1,
		} {
			hash, err := c.chain.BlockHashByHeight(height)
			if err != nil {
				return err
			}
			blockID, err := dbFetchBlockIDByHash(dbTx, hash)
			if err != nil {
				return err
			}
			if blockID != want {
				return fmt.Errorf("height %d has block ID %d, "+
					"want %d", height, blockID, want)
			}
		}
		return nil
	}))

	c.connect(t, idx, 0, target)
	reference := readAddrIndexTestBucket(t, c.db)

	for _, writtenShards := range []int{0, 100} {
		name := fmt.Sprintf("shards_%d", writtenShards)
		t.Run(name, func(t *testing.T) {
			c.resetAddrIndex(t, idx, -1, nil)
			if writtenShards > 0 {
				c.interruptBuild(t, writtenShards)
			}
			require.NoError(t, manager.Init(c.chain, nil))
			got := readAddrIndexTestBucket(t, c.db)
			requireAddrBuildLevels(t, got, reference)
		})
	}
}

func TestAddrIndexFastBuildTxIndexCatchUp(t *testing.T) {
	const target = 1005
	c := setupAddrIndexTestChain(t, target)
	for _, txHeight := range []int32{-1, 300} {
		name := fmt.Sprintf("transaction_index_height_%d", txHeight)
		t.Run(name, func(t *testing.T) {
			txIdx := NewTxIndex(c.db)
			idx := NewAddrIndexWithDataDir(
				c.db, &chaincfg.MainNetParams, c.rootDir,
			)
			manager := NewManager(c.db, []Indexer{txIdx, idx})
			require.NoError(t, manager.createIndexes())
			c.connect(t, txIdx, 0, txHeight)
			require.NoError(t, manager.Init(c.chain, nil))
			got := readAddrIndexTestBucket(t, c.db)
			c.resetAddrIndex(t, idx, -1, nil)
			c.connect(t, idx, 0, target)
			requireAddrBuildLevels(t, got,
				readAddrIndexTestBucket(t, c.db))
			require.NoError(t, DropTxIndex(c.db, nil))
		})
	}
}
