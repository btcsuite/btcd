// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

type interruptAddrUpdateDB struct {
	database.DB
	interrupt chan struct{}
	done      bool
}

func (db *interruptAddrUpdateDB) Update(fn func(database.Tx) error) error {
	if err := db.DB.Update(fn); err != nil {
		return err
	}
	if !db.done {
		close(db.interrupt)
		db.done = true
	}
	return nil
}

type failAddrFlushDB struct {
	database.DB
	err error
}

func (db *failAddrFlushDB) Flush() error {
	return db.err
}

type noAddrFlushDB struct {
	database.DB
}

type interruptedAddrBuild struct {
	rootDir string
	db      database.DB
	chain   *blockchain.BlockChain
	addr    address.Address
}

func setupInterruptedAddrBuild(t *testing.T) *interruptedAddrBuild {
	t.Helper()

	rootDir := t.TempDir()
	db, err := database.Create(
		"ffldb", filepath.Join(rootDir, "db"), wire.MainNet,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	params := &chaincfg.MainNetParams
	chain, err := blockchain.New(&blockchain.Config{
		DB:          db,
		ChainParams: params,
		TimeSource:  blockchain.NewMedianTime(),
		SigCache:    txscript.NewSigCache(1000),
	})
	require.NoError(t, err)

	txManager := NewManager(db, []Indexer{NewTxIndex(db)})
	require.NoError(t, txManager.Init(chain, nil))

	idx := NewAddrIndexWithDataDir(db, params, rootDir)
	manager := NewManager(db, []Indexer{NewTxIndex(db), idx})
	require.NoError(t, manager.createIndexes())

	stager, _, err := idx.buildAddrIndexRecords(
		chain, addrBuildScanChunkSize, nil,
	)
	require.NoError(t, err)
	interrupt := make(chan struct{})
	wrappedDB := &interruptAddrUpdateDB{DB: db, interrupt: interrupt}
	writer := &AddrIndex{db: wrappedDB}
	err = writer.writeAddrIndexToDB(
		stager, []uint32{1}, addrBuildWriteBatchBytes, interrupt,
	)
	require.ErrorIs(t, err, errInterruptRequested)
	stager.close()

	pkScript := params.GenesisBlock.Transactions[0].TxOut[0].PkScript
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript, params)
	require.NoError(t, err)
	require.Len(t, addrs, 1)

	return &interruptedAddrBuild{
		rootDir: rootDir,
		db:      db,
		chain:   chain,
		addr:    addrs[0],
	}
}

func requireGenesisAddrIndex(t *testing.T, build *interruptedAddrBuild,
	idx *AddrIndex) {

	t.Helper()
	err := build.db.View(func(dbTx database.Tx) error {
		hash, height, err := dbFetchIndexerTip(dbTx, addrIndexKey)
		if err != nil {
			return err
		}
		genesisHash := chaincfg.MainNetParams.GenesisHash
		if height != 0 || !hash.IsEqual(genesisHash) {
			return fmt.Errorf("address index tip is (%d, %s)",
				height, hash)
		}
		return nil
	})
	require.NoError(t, err)

	regions, skipped, err := idx.TxRegionsForAddress(
		nil, build.addr, 0, 1, false,
	)
	require.NoError(t, err)
	require.Zero(t, skipped)
	require.Len(t, regions, 1)
	require.True(t, regions[0].Hash.IsEqual(
		chaincfg.MainNetParams.GenesisHash,
	))
	_, exists := AddrIndexFastBuildDir(build.rootDir)
	require.False(t, exists)
}

func TestInterruptedAddrBuildReplay(t *testing.T) {
	build := setupInterruptedAddrBuild(t)

	// The scan finished before the write pass was interrupted, so the
	// build resumes with the write pass.
	stagingDir := filepath.Join(
		build.rootDir, addrIndexBuildDirName, "staging",
	)
	manifest, ok := readAddrBuildManifest(stagingDir)
	require.True(t, ok)
	require.Equal(t, manifest.TargetHeight, manifest.Completed)

	idx := NewAddrIndexWithDataDir(
		build.db, &chaincfg.MainNetParams, build.rootDir,
	)
	manager := NewManager(build.db, []Indexer{NewTxIndex(build.db), idx})
	require.NoError(t, manager.Init(build.chain, nil))
	requireGenesisAddrIndex(t, build, idx)
}

func TestAddrIndexInitRemovesCompletedStaging(t *testing.T) {
	build := setupInterruptedAddrBuild(t)
	require.NoError(t, build.db.Update(func(dbTx database.Tx) error {
		return dbPutIndexerTip(dbTx, addrIndexKey,
			chaincfg.MainNetParams.GenesisHash, 0)
	}))
	require.NoError(t, flushAddrIndexDB(build.db))

	idx := NewAddrIndexWithDataDir(
		build.db, &chaincfg.MainNetParams, build.rootDir,
	)
	require.NoError(t, idx.Init())
	_, exists := AddrIndexFastBuildDir(build.rootDir)
	require.False(t, exists)
}

// TestAddrIndexInitKeepsUnfinishedStaging ensures Init keeps the staging of a
// build whose target is not the index tip.
func TestAddrIndexInitKeepsUnfinishedStaging(t *testing.T) {
	tests := []struct {
		name   string
		height int32
		hash   chainhash.Hash
	}{
		{"uninitialized", -1, chainhash.Hash{}},
		{"target_height_other_hash", 0, chainhash.Hash{3}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			build := setupInterruptedAddrBuild(t)
			err := build.db.Update(func(dbTx database.Tx) error {
				return dbPutIndexerTip(dbTx, addrIndexKey,
					&test.hash, test.height)
			})
			require.NoError(t, err)

			idx := NewAddrIndexWithDataDir(build.db,
				&chaincfg.MainNetParams, build.rootDir)
			require.NoError(t, idx.Init())
			_, exists := AddrIndexFastBuildDir(build.rootDir)
			require.True(t, exists)
		})
	}
}

func TestAddrIndexInitRemovesUnpublishedStaging(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(
		rootDir, addrIndexBuildDirName, "staging.tmp",
	), 0700))
	require.NoError(t, os.Mkdir(
		filepath.Join(rootDir, addrIndexBuildDirName)+".delete", 0700,
	))
	idx := NewAddrIndexWithDataDir(nil, nil, rootDir)
	require.NoError(t, idx.Init())
	_, exists := AddrIndexFastBuildDir(rootDir)
	require.False(t, exists)
	deleteDir := filepath.Join(rootDir, addrIndexBuildDirName) + ".delete"
	_, err := os.Stat(deleteDir)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestFastBuildFlushFailureRetainsStaging(t *testing.T) {
	build := setupInterruptedAddrBuild(t)
	flushErr := errors.New("flush failure")
	db := &failAddrFlushDB{DB: build.db, err: flushErr}
	idx := NewAddrIndexWithDataDir(
		db, &chaincfg.MainNetParams, build.rootDir,
	)
	require.ErrorIs(t, idx.FastBuild(build.chain, nil), flushErr)
	_, exists := AddrIndexFastBuildDir(build.rootDir)
	require.True(t, exists)

	// The matching tip is visible through the cache, but Init must retain
	// staging when that tip cannot be flushed.
	require.ErrorIs(t, idx.Init(), flushErr)
	_, exists = AddrIndexFastBuildDir(build.rootDir)
	require.True(t, exists)
}

// TestFastBuildChecksFlushBeforeStaging ensures a fast build fails before it
// creates staging when the database cannot flush.
func TestFastBuildChecksFlushBeforeStaging(t *testing.T) {
	c := setupAddrIndexTestChain(t, 10)
	db := &noAddrFlushDB{DB: c.db}
	idx := NewAddrIndexWithDataDir(db, &chaincfg.MainNetParams, c.rootDir)
	manager := NewManager(db, []Indexer{NewTxIndex(db), idx})
	require.ErrorContains(t, manager.Init(c.chain, nil), "cannot flush")
	_, exists := AddrIndexFastBuildDir(c.rootDir)
	require.False(t, exists)
}

// addrBuildDrops are the index drops that remove address index build staging.
var addrBuildDrops = []struct {
	name string
	drop func(database.DB, string, <-chan struct{}) error
}{
	{"address_index", DropAddrIndexWithDataDir},
	{"transaction_index", DropTxIndexWithDataDir},
}

func TestInterruptedIndexDropsRetainAddrBuild(t *testing.T) {
	for _, test := range addrBuildDrops {
		t.Run(test.name, func(t *testing.T) {
			build := setupInterruptedAddrBuild(t)
			interrupt := make(chan struct{})
			close(interrupt)
			require.ErrorIs(t, test.drop(
				build.db, build.rootDir, interrupt,
			), errInterruptRequested)
			_, exists := AddrIndexFastBuildDir(build.rootDir)
			require.True(t, exists)

			idx := NewAddrIndexWithDataDir(build.db,
				&chaincfg.MainNetParams, build.rootDir)
			manager := NewManager(build.db, []Indexer{
				NewTxIndex(build.db), idx,
			})
			require.NoError(t, manager.Init(build.chain, nil))
			requireGenesisAddrIndex(t, build, idx)
		})
	}
}

func TestIndexDropFlushFailureRetainsAddrBuild(t *testing.T) {
	for _, test := range addrBuildDrops {
		t.Run(test.name, func(t *testing.T) {
			build := setupInterruptedAddrBuild(t)
			flushErr := errors.New("flush failure")
			db := &failAddrFlushDB{DB: build.db, err: flushErr}
			err := test.drop(db, build.rootDir, nil)
			require.ErrorIs(t, err, flushErr)
			_, exists := AddrIndexFastBuildDir(build.rootDir)
			require.True(t, exists)

			err = test.drop(build.db, build.rootDir, nil)
			require.NoError(t, err)
			_, exists = AddrIndexFastBuildDir(build.rootDir)
			require.False(t, exists)
		})
	}
}

func TestIndexDropChecksFlushBeforeMutation(t *testing.T) {
	for _, test := range addrBuildDrops {
		t.Run(test.name, func(t *testing.T) {
			build := setupInterruptedAddrBuild(t)
			db := &noAddrFlushDB{DB: build.db}
			require.Error(t, test.drop(db, build.rootDir, nil))

			var dropping, bucketExists bool
			err := build.db.View(func(dbTx database.Tx) error {
				meta := dbTx.Metadata()
				tips := meta.Bucket(indexTipsBucketName)
				dropKey := indexDropKey(addrIndexKey)
				dropping = tips.Get(dropKey) != nil
				bucketExists = meta.Bucket(addrIndexKey) != nil
				return nil
			})
			require.NoError(t, err)
			require.False(t, dropping, "in-progress drop flag")
			require.True(t, bucketExists, "index bucket")
		})
	}
}
