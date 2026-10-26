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

	// The block file holds the mainnet blocks in height order starting at
	// the genesis block, each stored as:
	//  <network> <block length> <serialized block>
	// The blocks vary in length, so reaching the block at a height means
	// reading every block before it.
	tip := c.chain.BestSnapshot().Height
	for i := int32(0); i <= height; i++ {
		var header [2]uint32
		require.NoError(t, binary.Read(f, binary.LittleEndian, &header))
		require.Equal(t, uint32(wire.MainNet), header[0])
		data := make([]byte, header[1])
		_, err := io.ReadFull(f, data)
		require.NoError(t, err)

		// The chain already holds the blocks through its tip, so they are
		// not added.  Reading them above still moves the file to the
		// start of the next block.
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

// resetAddrIndex recreates the address index bucket holding only the levels in
// entries and sets the index tip to the main chain block at height.  A height
// of -1 sets the tip createIndexes writes for a new index, which marks the
// index uninitialized.
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

// interruptAddrWriteDB wraps a database and closes interrupt when remaining
// reaches zero.
type interruptAddrWriteDB struct {
	database.DB
	interrupt chan struct{}
	remaining int
}

// Update passes the call to the wrapped database and counts down remaining when
// the database transaction commits.
//
// This is part of the database.DB interface.
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

// Flush flushes the wrapped database, since FastBuild fails on a database that
// cannot flush pending writes.
func (db *interruptAddrWriteDB) Flush() error {
	return flushAddrIndexDB(db.DB)
}

// interruptBuild runs a fast build and interrupts its write pass after it
// commits the number of database transactions given by updates.  It requires
// the build to return errInterruptRequested after exactly that many database
// transactions and to leave its staging in place.
func (c *addrIndexTestChain) interruptBuild(t *testing.T, updates int) {
	t.Helper()
	db := &interruptAddrWriteDB{
		DB: c.db, interrupt: make(chan struct{}), remaining: updates,
	}
	idx := NewAddrIndexWithDataDir(db, &chaincfg.MainNetParams, c.rootDir)
	require.ErrorIs(t, idx.FastBuild(c.chain, db.interrupt),
		errInterruptRequested)
	require.Zero(t, db.remaining)
	stagingDir := filepath.Join(
		c.rootDir, addrIndexBuildDirName, "staging",
	)
	_, ok := readAddrBuildManifest(stagingDir)
	require.True(t, ok)
}

// addCheckpointAddrRecord adds a record for an address that is not in the
// chain to the checkpointed staging under rootDir and returns the key of level
// 0 of that address.  Only a build that writes the index from this staging
// writes the address to the index.
func addCheckpointAddrRecord(t *testing.T, rootDir string) [levelKeySize]byte {
	t.Helper()
	stagingDir := filepath.Join(rootDir, addrIndexBuildDirName, "staging")
	manifest, ok := readAddrBuildManifest(stagingDir)
	require.True(t, ok)

	var addrKey [addrKeySize]byte
	addrKey[1] = 0xff
	shard := int(addrKey[1])
	record := makeAddrRecord(&addrKey, 0, wire.TxLoc{})
	f, err := os.OpenFile(
		addrStagingShardPath(stagingDir, shard), os.O_WRONLY, 0,
	)
	require.NoError(t, err)
	_, err = f.WriteAt(record[:], manifest.ShardSizes[shard])
	require.NoError(t, err)
	require.NoError(t, f.Close())
	manifest.ShardSizes[shard] += addrRecordSize
	require.NoError(t, writeAddrBuildManifest(stagingDir, &manifest))
	return keyForLevel(addrKey, 0)
}

// requireBlockIDs requires each height in want to have the block ID it maps to
// in the transaction index.
func (c *addrIndexTestChain) requireBlockIDs(t *testing.T,
	want map[int32]uint32) {

	t.Helper()
	require.NoError(t, c.db.View(func(dbTx database.Tx) error {
		for height, wantID := range want {
			hash, err := c.chain.BlockHashByHeight(height)
			if err != nil {
				return err
			}
			blockID, err := dbFetchBlockIDByHash(dbTx, hash)
			if err != nil {
				return err
			}
			if blockID != wantID {
				return fmt.Errorf("height %d has block ID %d, "+
					"want %d", height, blockID, wantID)
			}
		}
		return nil
	}))
}

// TestAddrIndexFastBuildChainParity ensures a fast build writes the same levels
// as ConnectBlock for the mainnet blocks from the genesis block through height
// 14131, both in one run and after an interrupted write pass.  The blocks spend
// outputs, so the scan has to stage the entries ConnectBlock adds for the
// outputs they spend.
func TestAddrIndexFastBuildChainParity(t *testing.T) {
	// 1. Make a chain through block 14131.
	c := setupAddrIndexTestChain(t, 14131)

	// 2. Catch up the tx index, since the build reads block IDs from it.
	txManager := NewManager(c.db, []Indexer{NewTxIndex(c.db)})
	require.NoError(t, txManager.Init(c.chain, nil))

	// 3. Create the address index without fast building it.
	idx := NewAddrIndexWithDataDir(c.db, &chaincfg.MainNetParams, c.rootDir)
	require.NoError(t, NewManager(c.db, []Indexer{idx}).createIndexes())

	// 4. Index one block at a time, the slow way, for the reference.
	spent := c.connect(t, idx, 0, 14131)
	require.Positive(t, spent)
	reference := readAddrIndexTestBucket(t, c.db)

	for _, writtenShards := range []int{0, 1, 39, 199} {
		name := fmt.Sprintf("shards_%d", writtenShards)
		t.Run(name, func(t *testing.T) {
			// 1. Make the address index empty and uninitialized.
			c.resetAddrIndex(t, idx, -1, nil)
			var checkpointKey [levelKeySize]byte

			// 2. Cut a build off after writtenShards DB writes.
			if writtenShards > 0 {
				c.interruptBuild(t, writtenShards)

				// 3. Slip a fake address into the staging.
				// The levels match the reference even if
				// the build scans the chain again, but only
				// a build that writes the index from this
				// staging writes the fake address.
				checkpointKey = addCheckpointAddrRecord(
					t, c.rootDir,
				)
			}

			// 4. Fast build it, redoing any cut off write pass.
			require.NoError(t, idx.FastBuild(c.chain, nil))
			got := readAddrIndexTestBucket(t, c.db)

			// 5. Check that the index holds the fake address, which
			// is only in the checkpointed staging.
			if writtenShards > 0 {
				_, ok := got[checkpointKey]
				require.True(t, ok, "checkpoint entry")
				delete(got, checkpointKey)
			}

			// 6. Check that the levels match the reference.
			requireAddrBuildLevels(t, got, reference)

			// 7. Check that the build removed its staging.
			_, exists := AddrIndexFastBuildDir(c.rootDir)
			require.False(t, exists)
		})
	}
}

// TestAddrIndexFastBuildResumeAfterChainGrowth scans in several chunks,
// interrupts the write pass, and resumes the build after the chain grows.  It
// ensures the resumed build starts from the checkpoint and writes the same
// levels as ConnectBlock.
func TestAddrIndexFastBuildResumeAfterChainGrowth(t *testing.T) {
	const (
		firstTarget = 500
		target      = 1000
		chunkSize   = 100
	)

	// 1. Make a 500-block chain and empty indexes, then fill the tx index.
	c := setupAddrIndexTestChain(t, firstTarget)
	txIdx := NewTxIndex(c.db)
	idx := NewAddrIndexWithDataDir(c.db, &chaincfg.MainNetParams, c.rootDir)
	manager := NewManager(c.db, []Indexer{txIdx, idx})
	require.NoError(t, manager.createIndexes())
	c.connect(t, txIdx, 0, firstTarget)

	// 2. Build, but cut it off after the first DB write.
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

	// Check that staging says all 500 blocks were scanned.
	stagingDir := filepath.Join(
		c.rootDir, addrIndexBuildDirName, "staging",
	)
	manifest, ok := readAddrBuildManifest(stagingDir)
	require.True(t, ok)
	require.Equal(t, int32(firstTarget), manifest.Completed)

	// 3. Slip a fake address into the staging.  The levels match
	// ConnectBlock even if the resumed build scans from the genesis block
	// again, but only a build that keeps the records staged before the
	// chain grew writes the fake address.
	checkpointKey := addCheckpointAddrRecord(t, c.rootDir)

	// 4. Grow the chain to 1000.
	c.addTestBlocks(t, target)
	c.connect(t, txIdx, firstTarget+1, target)

	// 5. Resume the build.
	builtHash, builtHeight, err := idx.buildAddrIndexFromChain(
		c.chain, chunkSize, nil,
	)
	require.NoError(t, err)

	// 6. Check that it built through block 1000.
	tipHash, err := c.chain.BlockHashByHeight(target)
	require.NoError(t, err)
	require.Equal(t, int32(target), builtHeight)
	require.Equal(t, *tipHash, builtHash)

	// 7. Check that the index holds the fake address, which is only in
	// the checkpointed staging.
	got := readAddrIndexTestBucket(t, c.db)
	_, ok = got[checkpointKey]
	require.True(t, ok, "checkpoint entry")
	delete(got, checkpointKey)

	// 8. Rebuild the slow way and compare.
	c.resetAddrIndex(t, idx, -1, nil)
	c.connect(t, idx, 0, target)
	requireAddrBuildLevels(t, got, readAddrIndexTestBucket(t, c.db))
}

// TestAddrIndexFastBuildBlockIDGap ensures a fast build writes the same levels
// as ConnectBlock when the block IDs in the transaction index have a gap.  The
// gap comes from rolling back a database transaction after TxIndex.ConnectBlock
// advances curBlockID.  curBlockID is kept in memory and the rollback does not
// restore it, so the next block skips an ID.  This is a rare edge case.  It
// takes a database write error during a block connect that btcd logs and keeps
// running after, such as an I/O error from leveldb.  Still, a block ID is not
// always its height plus one, so the build has to look up each one in the
// transaction index.
func TestAddrIndexFastBuildBlockIDGap(t *testing.T) {
	const target = 1005

	// 1. Make a chain through block 1005 and empty indexes.
	c := setupAddrIndexTestChain(t, target)
	txIdx := NewTxIndex(c.db)
	idx := NewAddrIndexWithDataDir(c.db, &chaincfg.MainNetParams, c.rootDir)
	manager := NewManager(c.db, []Indexer{txIdx, idx})
	require.NoError(t, manager.createIndexes())

	// 2. Fill the tx index, but a rollback skips ID 2.
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

	// Check that block 0 got ID 1 and block 1 got ID 3.
	c.requireBlockIDs(t, map[int32]uint32{0: 1, 1: 3})

	// 3. Index the slow way for the reference.
	c.connect(t, idx, 0, target)
	reference := readAddrIndexTestBucket(t, c.db)

	for _, interrupted := range []bool{false, true} {
		name := fmt.Sprintf("interrupt_%t", interrupted)
		t.Run(name, func(t *testing.T) {
			// 1. Make the address index empty and uninitialized.
			c.resetAddrIndex(t, idx, -1, nil)

			// 2. Cut a build off after two DB writes.
			if interrupted {
				c.interruptBuild(t, 2)
			}

			// 3. Init fast builds, redoing any cut off write pass.
			require.NoError(t, manager.Init(c.chain, nil))
			got := readAddrIndexTestBucket(t, c.db)

			// 4. Check that the levels match the reference.
			requireAddrBuildLevels(t, got, reference)
		})
	}

	// Rebuilding the tx index without the gap changes the ID of every block
	// from height 1, so the write pass has to resolve the staged heights
	// through the new tx index.
	t.Run("transaction_index_drop", func(t *testing.T) {
		// 1. Reset the index and cut a build off after two DB writes.
		c.resetAddrIndex(t, idx, -1, nil)
		c.interruptBuild(t, 2)

		// 2. Slip a fake address into the staging.  The levels match
		// ConnectBlock even if the build scans the chain again, but
		// only a build that writes the index from this staging writes
		// the fake address.
		checkpointKey := addCheckpointAddrRecord(t, c.rootDir)

		// 3. Drop the tx and address indexes, but keep the staging.
		require.NoError(t, DropTxIndex(c.db, nil))

		// 4. Init rebuilds the tx index with no gap, then fast builds.
		manager := NewManager(c.db, []Indexer{NewTxIndex(c.db), idx})
		require.NoError(t, manager.Init(c.chain, nil))
		got := readAddrIndexTestBucket(t, c.db)

		// 5. Check that the index holds the fake address, which is only
		// in the checkpointed staging.
		_, ok := got[checkpointKey]
		require.True(t, ok, "checkpoint entry")
		delete(got, checkpointKey)

		// 6. Rebuild the slow way with the new IDs and compare.
		c.resetAddrIndex(t, idx, -1, nil)
		c.connect(t, idx, 0, target)
		requireAddrBuildLevels(t, got, readAddrIndexTestBucket(t, c.db))
	})
}

// TestAddrIndexFastBuildDecreasingBlockID gives a block an unused ID below the
// IDs of the blocks before it, as TxIndex does when it resumes below a gap left
// by a rolled back transaction.  TxIndex.Init finds the highest block ID with a
// search that assumes the IDs have no gap, so after a restart it can resume
// below one.  This is rarer than the gap itself, since the search has to check
// the skipped ID.  When it does, block IDs do not always increase with height,
// and the build has to order the entries of an address by height, not by block
// ID.  The test ensures a fast build writes the same levels as ConnectBlock,
// both in one run and after an interrupted write pass.
func TestAddrIndexFastBuildDecreasingBlockID(t *testing.T) {
	const target = 1100

	// 1. Make a chain through block 1100 and empty indexes.
	c := setupAddrIndexTestChain(t, target)
	txIdx := NewTxIndex(c.db)
	idx := NewAddrIndexWithDataDir(c.db, &chaincfg.MainNetParams, c.rootDir)
	manager := NewManager(c.db, []Indexer{txIdx, idx})
	require.NoError(t, manager.createIndexes())

	// 2. Fill the tx index to block 169, but a rollback skips ID 10.
	c.connect(t, txIdx, 0, 8)
	block, err := c.chain.BlockByHeight(9)
	require.NoError(t, err)
	rollbackErr := errors.New("transaction rollback")
	err = c.db.Update(func(dbTx database.Tx) error {
		if err := txIdx.ConnectBlock(dbTx, block, nil); err != nil {
			return err
		}
		return rollbackErr
	})
	require.ErrorIs(t, err, rollbackErr)
	c.connect(t, txIdx, 9, 169)

	// 3. Block 170 gets the unused ID 10, then Init resumes IDs at 172.
	txIdx.curBlockID = 9
	c.connect(t, txIdx, 170, 170)
	require.NoError(t, txIdx.Init())
	c.connect(t, txIdx, 171, target)

	// Block 170 (ID 10) spends the coinbase of block 9 (ID 11).
	c.requireBlockIDs(t, map[int32]uint32{
		8: 9, 9: 11, 169: 171, 170: 10, 171: 172, target: target + 1,
	})

	// 4. Index the slow way for the reference.
	c.connect(t, idx, 0, target)
	reference := readAddrIndexTestBucket(t, c.db)

	for _, writtenShards := range []int{0, 100} {
		name := fmt.Sprintf("shards_%d", writtenShards)
		t.Run(name, func(t *testing.T) {
			// 1. Make the address index empty and uninitialized.
			c.resetAddrIndex(t, idx, -1, nil)

			// 2. Cut a build off after writtenShards DB writes.
			if writtenShards > 0 {
				c.interruptBuild(t, writtenShards)
			}

			// 3. Init fast builds, redoing any cut off write pass.
			require.NoError(t, manager.Init(c.chain, nil))
			got := readAddrIndexTestBucket(t, c.db)

			// 4. Check that the levels match the reference.
			requireAddrBuildLevels(t, got, reference)
		})
	}
}

// TestAddrIndexFastBuildTxIndexCatchUp ensures Manager.Init catches up the
// transaction index before the fast build, from a new transaction index and
// from one that is behind the chain tip.
func TestAddrIndexFastBuildTxIndexCatchUp(t *testing.T) {
	const target = 1005
	c := setupAddrIndexTestChain(t, target)
	for _, txHeight := range []int32{-1, 300} {
		name := fmt.Sprintf("transaction_index_height_%d", txHeight)
		t.Run(name, func(t *testing.T) {
			// 1. Create empty tx and address indexes.
			txIdx := NewTxIndex(c.db)
			idx := NewAddrIndexWithDataDir(
				c.db, &chaincfg.MainNetParams, c.rootDir,
			)
			manager := NewManager(c.db, []Indexer{txIdx, idx})
			require.NoError(t, manager.createIndexes())

			// 2. Leave the tx index behind the tip, at txHeight.
			c.connect(t, txIdx, 0, txHeight)

			// 3. Init catches up the tx index, then fast builds.
			require.NoError(t, manager.Init(c.chain, nil))
			got := readAddrIndexTestBucket(t, c.db)

			// 4. Rebuild the slow way and compare.
			c.resetAddrIndex(t, idx, -1, nil)
			c.connect(t, idx, 0, target)
			requireAddrBuildLevels(t, got,
				readAddrIndexTestBucket(t, c.db))

			// 5. Drop both indexes before the next subtest.
			require.NoError(t, DropTxIndex(c.db, nil))
		})
	}
}
