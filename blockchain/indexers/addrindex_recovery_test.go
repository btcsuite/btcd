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

// interruptAddrUpdateDB wraps a database and closes interrupt after the first
// database transaction that commits through its Update.
type interruptAddrUpdateDB struct {
	database.DB
	interrupt chan struct{}
	done      bool
}

// Update passes the call to the wrapped database and closes interrupt after the
// first database transaction that commits.
//
// This is part of the database.DB interface.
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

// failAddrFlushDB wraps a database and fails every call to Flush with err.
type failAddrFlushDB struct {
	database.DB
	err error
}

// Flush returns err without flushing the wrapped database.
//
// This is part of the addrBuildDBFlusher interface.
func (db *failAddrFlushDB) Flush() error {
	return db.err
}

// noAddrFlushDB wraps a database and leaves out its Flush method, so it is a
// database that cannot flush pending writes.
type noAddrFlushDB struct {
	database.DB
}

// interruptedAddrBuild holds the data directory, database and chain of the
// interrupted address index fast build that setupInterruptedAddrBuild leaves,
// and the genesis address.
type interruptedAddrBuild struct {
	rootDir string
	db      database.DB
	chain   *blockchain.BlockChain
	addr    address.Address
}

// setupInterruptedAddrBuild creates a database and a chain holding only the
// genesis block, catches up the transaction index, and leaves an address index
// fast build whose write pass is interrupted after its first database
// transaction.  The scan is complete, the write pass has written the entry for
// the genesis address, and the address index is still uninitialized with its
// staging in place.
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

// requireGenesisAddrIndex requires the address index tip to be the genesis
// block, the index to hold one entry for the genesis address, and the build
// directory to be removed.
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

	// Request two regions, so a second entry for the genesis address fails
	// the length check.
	regions, _, err := idx.TxRegionsForAddress(
		nil, build.addr, 0, 2, false,
	)
	require.NoError(t, err)
	require.Len(t, regions, 1)
	require.True(t, regions[0].Hash.IsEqual(
		chaincfg.MainNetParams.GenesisHash,
	))
	_, exists := AddrIndexFastBuildDir(build.rootDir)
	require.False(t, exists)
}

// requireAddrBuildStaging requires the staging under rootDir to hold a valid
// manifest.
func requireAddrBuildStaging(t *testing.T, rootDir string) {
	t.Helper()
	stagingDir := filepath.Join(rootDir, addrIndexBuildDirName, "staging")
	_, ok := readAddrBuildManifest(stagingDir)
	require.True(t, ok)
}

// TestInterruptedAddrBuildReplay ensures Manager.Init finishes a build whose
// write pass was interrupted after the scan completed.  The resumed build
// repeats the write pass from the checkpointed staging.  It rewrites the
// genesis address, whose entry the interrupted write pass already wrote, and
// the index holds one entry for that address.
func TestInterruptedAddrBuildReplay(t *testing.T) {
	// 1. Make a genesis-only chain with a build cut off after one DB write.
	build := setupInterruptedAddrBuild(t)

	// Check that staging says the scan finished.
	stagingDir := filepath.Join(
		build.rootDir, addrIndexBuildDirName, "staging",
	)
	manifest, ok := readAddrBuildManifest(stagingDir)
	require.True(t, ok)
	require.Equal(t, manifest.TargetHeight, manifest.Completed)

	// 2. Slip a fake address into the staging.  The other checks pass
	// even if the build scans the chain again, but only a build that
	// writes the index from this staging writes the fake address.
	checkpointKey := addCheckpointAddrRecord(t, build.rootDir)

	// 3. Change the transaction location in the entry the interrupted
	// write pass wrote for the genesis address.  Only a write pass that
	// rewrites the address restores it.
	addrKey, err := addrToKey(build.addr)
	require.NoError(t, err)
	genesisKey := keyForLevel(addrKey, 0)
	want := readAddrIndexTestBucket(t, build.db)[genesisKey]
	require.Len(t, want, txEntrySize)
	err = build.db.Update(func(dbTx database.Tx) error {
		bucket := dbTx.Metadata().Bucket(addrIndexKey)
		return bucket.Put(
			genesisKey[:], serializeAddrIndexEntry(1, wire.TxLoc{}),
		)
	})
	require.NoError(t, err)

	// 4. Init finishes the build, repeating the write pass.
	idx := NewAddrIndexWithDataDir(
		build.db, &chaincfg.MainNetParams, build.rootDir,
	)
	manager := NewManager(build.db, []Indexer{NewTxIndex(build.db), idx})
	require.NoError(t, manager.Init(build.chain, nil))

	// 5. Check that the tip is the genesis block, the genesis address has
	// one entry, and staging is gone.
	requireGenesisAddrIndex(t, build, idx)

	// 6. Check that the write pass rewrote the genesis address.
	got := readAddrIndexTestBucket(t, build.db)
	require.Equal(t, want, got[genesisKey])

	// 7. Check that the index holds the fake address, which is only in
	// the checkpointed staging.
	_, ok = got[checkpointKey]
	require.True(t, ok, "checkpoint entry")
}

// TestFastBuildReorgRebuildsAddressIndex ensures Manager.Init drops the address
// index and its staging when the target of an unfinished build is no longer in
// the main chain, which removes the entries the write pass left, and builds the
// address index again.
func TestFastBuildReorgRebuildsAddressIndex(t *testing.T) {
	build := setupInterruptedAddrBuild(t)
	stagingDir := filepath.Join(
		build.rootDir, addrIndexBuildDirName, "staging",
	)
	manifest, ok := readAddrBuildManifest(stagingDir)
	require.True(t, ok)
	manifest.TargetHash = chainhash.Hash{2}
	require.NoError(t, writeAddrBuildManifest(stagingDir, &manifest))

	// Put an entry for an address that is not in the chain, standing in for
	// one the write pass left for a block that is no longer in the main
	// chain.  It must not survive the rebuild.
	var orphanAddrKey [addrKeySize]byte
	orphanAddrKey[1] = 0xff
	orphanKey := keyForLevel(orphanAddrKey, 0)
	err := build.db.Update(func(dbTx database.Tx) error {
		bucket := dbTx.Metadata().Bucket(addrIndexKey)
		return bucket.Put(orphanKey[:], make([]byte, txEntrySize))
	})
	require.NoError(t, err)

	idx := NewAddrIndexWithDataDir(
		build.db, &chaincfg.MainNetParams, build.rootDir,
	)
	manager := NewManager(build.db, []Indexer{NewTxIndex(build.db), idx})
	require.NoError(t, manager.Init(build.chain, nil))
	requireGenesisAddrIndex(t, build, idx)
	_, ok = readAddrIndexTestBucket(t, build.db)[orphanKey]
	require.False(t, ok, "orphaned entry")
}

// TestDropStaleBuildKeepsMainChainBuild ensures an unfinished build whose
// target is in the main chain keeps its staging so that it can resume.
func TestDropStaleBuildKeepsMainChainBuild(t *testing.T) {
	build := setupInterruptedAddrBuild(t)
	idx := NewAddrIndexWithDataDir(
		build.db, &chaincfg.MainNetParams, build.rootDir,
	)
	dropped, err := idx.dropStaleBuild(build.chain, nil)
	require.NoError(t, err)
	require.False(t, dropped)
	requireAddrBuildStaging(t, build.rootDir)
}

// TestStagingForInitializedAddrIndex ensures Manager.Init returns an error for
// staging next to an address index whose tip is set below the build target.
func TestStagingForInitializedAddrIndex(t *testing.T) {
	const target = 10
	c := setupAddrIndexTestChain(t, target)
	txIdx := NewTxIndex(c.db)
	idx := NewAddrIndexWithDataDir(c.db, &chaincfg.MainNetParams, c.rootDir)
	manager := NewManager(c.db, []Indexer{txIdx, idx})
	require.NoError(t, manager.createIndexes())
	c.connect(t, txIdx, 0, target)
	c.interruptBuild(t, 1)
	c.resetAddrIndex(t, idx, 5, nil)
	require.ErrorContains(t, manager.Init(c.chain, nil), "staging exists")
}

// TestAddrIndexInitCompletedStaging ensures Init removes the staging of a
// build whose target is the index tip and keeps the index.  A build leaves
// such staging when it stops after it flushes the tip and before it removes
// the staging.  Init keeps the staging when the tip height or hash differs
// from the target.
func TestAddrIndexInitCompletedStaging(t *testing.T) {
	tests := []struct {
		name      string
		height    int32
		hash      chainhash.Hash
		completed bool
	}{
		{"completed", 0, *chaincfg.MainNetParams.GenesisHash, true},
		{"uninitialized", -1, chainhash.Hash{}, false},
		{"target_height_other_hash", 0, chainhash.Hash{3}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// 1. Cut a build off after its first DB write.
			build := setupInterruptedAddrBuild(t)

			// 2. Set the tip and flush it.
			err := build.db.Update(func(dbTx database.Tx) error {
				return dbPutIndexerTip(dbTx, addrIndexKey,
					&test.hash, test.height)
			})
			require.NoError(t, err)
			require.NoError(t, flushAddrIndexDB(build.db))

			// 3. Run Init.
			idx := NewAddrIndexWithDataDir(build.db,
				&chaincfg.MainNetParams, build.rootDir)
			require.NoError(t, idx.Init())

			// 4. For a completed build, check that staging is
			// gone and the index is kept.  Otherwise, check that
			// staging is kept.
			if test.completed {
				requireGenesisAddrIndex(t, build, idx)
			} else {
				requireAddrBuildStaging(t, build.rootDir)
			}
		})
	}
}

// TestAddrIndexInitRemovesUnpublishedStaging ensures Init removes a build
// directory that holds staging.tmp but no staging directory, which a build
// leaves when it stops before it renames staging.tmp to staging.  Init also
// finishes an interrupted removal of the build directory by removing
// addrindexbuild.delete, both next to such a build directory and on its own.
func TestAddrIndexInitRemovesUnpublishedStaging(t *testing.T) {
	// 1. Create addrindexbuild/staging.tmp and addrindexbuild.delete.
	rootDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(
		rootDir, addrIndexBuildDirName, "staging.tmp",
	), 0700))
	require.NoError(t, os.Mkdir(
		filepath.Join(rootDir, addrIndexBuildDirName)+".delete", 0700,
	))

	// 2. Check that Init removes both without using the database.
	idx := NewAddrIndexWithDataDir(nil, nil, rootDir)
	require.NoError(t, idx.Init())
	_, exists := AddrIndexFastBuildDir(rootDir)
	require.False(t, exists)
	deleteDir := filepath.Join(rootDir, addrIndexBuildDirName) + ".delete"
	_, err := os.Stat(deleteDir)
	require.ErrorIs(t, err, os.ErrNotExist)

	// 3. Check that Init removes addrindexbuild.delete on its own too.
	require.NoError(t, os.Mkdir(deleteDir, 0700))
	require.NoError(t, idx.Init())
	_, err = os.Stat(deleteDir)
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestFastBuildFlushFailureRetainsStaging ensures FastBuild and Init return the
// flush error and keep the staging when flushing the index tip fails, since
// staging shows the build is unfinished until the tip is durable.
func TestFastBuildFlushFailureRetainsStaging(t *testing.T) {
	// 1. Make a genesis-only chain with a build cut off after one DB write.
	build := setupInterruptedAddrBuild(t)

	// 2. Run FastBuild on a DB that fails every flush.
	flushErr := errors.New("flush failure")
	db := &failAddrFlushDB{DB: build.db, err: flushErr}
	idx := NewAddrIndexWithDataDir(
		db, &chaincfg.MainNetParams, build.rootDir,
	)
	require.ErrorIs(t, idx.FastBuild(build.chain, nil), flushErr)

	// Check that staging is kept, since the tip was not flushed.
	requireAddrBuildStaging(t, build.rootDir)

	// 3. Init sees the tip at the build target, but its flush fails too.
	require.ErrorIs(t, idx.Init(), flushErr)

	// 4. Check that staging is still kept.
	requireAddrBuildStaging(t, build.rootDir)
}

// TestFastBuildChecksFlushBeforeStaging ensures a fast build fails before it
// creates staging or writes to the address index when the database cannot
// flush.
func TestFastBuildChecksFlushBeforeStaging(t *testing.T) {
	// 1. Make a chain through block 10 and wrap its DB to leave out Flush.
	c := setupAddrIndexTestChain(t, 10)
	db := &noAddrFlushDB{DB: c.db}

	// 2. Check that Init fails as soon as the fast build starts.
	idx := NewAddrIndexWithDataDir(db, &chaincfg.MainNetParams, c.rootDir)
	manager := NewManager(db, []Indexer{NewTxIndex(db), idx})
	require.ErrorContains(t, manager.Init(c.chain, nil), "cannot flush")

	// 3. Check that there is no staging and the address index is empty.
	_, exists := AddrIndexFastBuildDir(c.rootDir)
	require.False(t, exists)
	require.Empty(t, readAddrIndexTestBucket(t, c.db))
}

// indexDropInProgress reports whether the index with key idxKey has a drop in
// progress.
func indexDropInProgress(t *testing.T, db database.DB, idxKey []byte) bool {
	t.Helper()
	var dropping bool
	require.NoError(t, db.View(func(dbTx database.Tx) error {
		tips := dbTx.Metadata().Bucket(indexTipsBucketName)
		dropping = tips.Get(indexDropKey(idxKey)) != nil
		return nil
	}))
	return dropping
}

// addrBuildDrops are the index drops that remove address index build staging.
var addrBuildDrops = []struct {
	name string
	drop func(database.DB, string, <-chan struct{}) error
}{
	{"address_index", DropAddrIndexWithDataDir},
	{"transaction_index", DropTxIndexWithDataDir},
}

// TestInterruptedIndexDropsRetainAddrBuild ensures an address index or
// transaction index drop that is interrupted keeps the address index build
// staging, since staging is only removed after the drop finishes and is
// flushed.  A transaction index drop drops the address index first, so the
// interrupt stops both drops during the address index drop.  Manager.Init then
// finishes the address index drop and fast builds the address index.
func TestInterruptedIndexDropsRetainAddrBuild(t *testing.T) {
	for _, test := range addrBuildDrops {
		t.Run(test.name, func(t *testing.T) {
			build := setupInterruptedAddrBuild(t)
			interrupt := make(chan struct{})
			close(interrupt)
			require.ErrorIs(t, test.drop(
				build.db, build.rootDir, interrupt,
			), errInterruptRequested)
			requireAddrBuildStaging(t, build.rootDir)
			require.True(t, indexDropInProgress(
				t, build.db, addrIndexKey,
			))
			require.False(t, indexDropInProgress(
				t, build.db, txIndexKey,
			))

			idx := NewAddrIndexWithDataDir(build.db,
				&chaincfg.MainNetParams, build.rootDir)
			manager := NewManager(build.db, []Indexer{
				NewTxIndex(build.db), idx,
			})
			require.NoError(t, manager.Init(build.chain, nil))
			require.False(t, indexDropInProgress(
				t, build.db, addrIndexKey,
			))
			requireGenesisAddrIndex(t, build, idx)
		})
	}
}

// TestIndexDropFlushFailureRetainsAddrBuild ensures an address index or
// transaction index drop keeps the address index build staging when flushing
// the drop fails, and that a retried drop removes it.
func TestIndexDropFlushFailureRetainsAddrBuild(t *testing.T) {
	for _, test := range addrBuildDrops {
		t.Run(test.name, func(t *testing.T) {
			build := setupInterruptedAddrBuild(t)
			flushErr := errors.New("flush failure")
			db := &failAddrFlushDB{DB: build.db, err: flushErr}
			err := test.drop(db, build.rootDir, nil)
			require.ErrorIs(t, err, flushErr)
			requireAddrBuildStaging(t, build.rootDir)

			err = test.drop(build.db, build.rootDir, nil)
			require.NoError(t, err)
			_, exists := AddrIndexFastBuildDir(build.rootDir)
			require.False(t, exists)
		})
	}
}

// TestIndexDropChecksFlushBeforeMutation ensures an address index or
// transaction index drop fails before it changes the database when address
// index build staging exists and the database cannot flush.  Neither the address
// index nor the transaction index has a drop in progress, and both buckets
// remain.
func TestIndexDropChecksFlushBeforeMutation(t *testing.T) {
	for _, test := range addrBuildDrops {
		t.Run(test.name, func(t *testing.T) {
			build := setupInterruptedAddrBuild(t)
			db := &noAddrFlushDB{DB: build.db}
			require.Error(t, test.drop(db, build.rootDir, nil))

			for _, idxKey := range [][]byte{addrIndexKey, txIndexKey} {
				require.False(t, indexDropInProgress(
					t, build.db, idxKey,
				), "%s in-progress drop flag", idxKey)

				var bucketExists bool
				err := build.db.View(func(dbTx database.Tx) error {
					meta := dbTx.Metadata()
					bucketExists = meta.Bucket(idxKey) != nil
					return nil
				})
				require.NoError(t, err)
				require.True(t, bucketExists, "%s bucket", idxKey)
			}
		})
	}
}
