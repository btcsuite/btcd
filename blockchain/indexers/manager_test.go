// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

type bucketBase struct {
	database.Bucket
}

// errorBucket wraps a database bucket and injects errors into operations used
// while cataloging and deleting index buckets.
type errorBucket struct {
	bucketBase
	forEachBucketErr error
	deleteBucketErr  error
}

// Bucket returns a wrapped nested bucket that preserves error injection.
func (b *errorBucket) Bucket(key []byte) database.Bucket {
	bucket := b.bucketBase.Bucket.Bucket(key)
	if bucket == nil {
		return nil
	}

	return &errorBucket{
		bucketBase:       bucketBase{Bucket: bucket},
		forEachBucketErr: b.forEachBucketErr,
		deleteBucketErr:  b.deleteBucketErr,
	}
}

// ForEachBucket returns the configured cataloging error when present.
func (b *errorBucket) ForEachBucket(fn func(k []byte) error) error {
	if b.forEachBucketErr != nil {
		return b.forEachBucketErr
	}

	return b.bucketBase.Bucket.ForEachBucket(fn)
}

// DeleteBucket returns the configured bucket deletion error when present.
func (b *errorBucket) DeleteBucket(key []byte) error {
	if b.deleteBucketErr != nil {
		return b.deleteBucketErr
	}

	return b.bucketBase.Bucket.DeleteBucket(key)
}

// errorTx wraps the metadata bucket of a database transaction so errors can
// be injected into bucket operations.
type errorTx struct {
	database.Tx
	forEachBucketErr error
	deleteBucketErr  error
}

// Metadata returns the wrapped top-level metadata bucket.
func (tx *errorTx) Metadata() database.Bucket {
	return &errorBucket{
		bucketBase:       bucketBase{Bucket: tx.Tx.Metadata()},
		forEachBucketErr: tx.forEachBucketErr,
		deleteBucketErr:  tx.deleteBucketErr,
	}
}

// errorDB wraps all managed transactions with the configured error injector.
type errorDB struct {
	database.DB
	forEachBucketErr error
	deleteBucketErr  error
}

// View invokes the view function with an error-injecting transaction.
func (db *errorDB) View(fn func(database.Tx) error) error {
	return db.DB.View(func(tx database.Tx) error {
		return fn(&errorTx{
			Tx:               tx,
			forEachBucketErr: db.forEachBucketErr,
			deleteBucketErr:  db.deleteBucketErr,
		})
	})
}

// Update invokes the update function with an error-injecting transaction.
func (db *errorDB) Update(fn func(database.Tx) error) error {
	return db.DB.Update(func(tx database.Tx) error {
		return fn(&errorTx{
			Tx:               tx,
			forEachBucketErr: db.forEachBucketErr,
			deleteBucketErr:  db.deleteBucketErr,
		})
	})
}

// createDropIndexTestDB creates a database with the metadata required to drop
// a test index.
func createDropIndexTestDB(t *testing.T, idxKey []byte) database.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "blocks_ffldb")
	db, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("unable to create test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("unable to close test database: %v", err)
		}
	})

	err = db.Update(func(tx database.Tx) error {
		meta := tx.Metadata()
		indexesBucket, err := meta.CreateBucket(indexTipsBucketName)
		if err != nil {
			return err
		}
		if err := indexesBucket.Put(idxKey, []byte{0x01}); err != nil {
			return err
		}

		_, err = meta.CreateBucket(idxKey)
		return err
	})
	if err != nil {
		t.Fatalf("unable to initialize test index: %v", err)
	}

	return db
}

// TestDropIndexCatalogError ensures errors encountered while cataloging index
// buckets are returned to the caller.
func TestDropIndexCatalogError(t *testing.T) {
	idxKey := []byte("testidx")
	testErr := errors.New("test catalog error")
	db := &errorDB{
		DB:               createDropIndexTestDB(t, idxKey),
		forEachBucketErr: testErr,
	}

	err := dropIndex(db, idxKey, "test index", nil)
	if !errors.Is(err, testErr) {
		t.Fatalf("unexpected error: got %v, want %v", err, testErr)
	}
}

// TestDropIndexDeleteBucketError ensures errors encountered while deleting an
// index bucket are returned to the caller.
func TestDropIndexDeleteBucketError(t *testing.T) {
	idxKey := []byte("testidx")
	testErr := errors.New("test bucket deletion error")
	db := &errorDB{
		DB:              createDropIndexTestDB(t, idxKey),
		deleteBucketErr: testErr,
	}

	err := dropIndex(db, idxKey, "test index", nil)
	if !errors.Is(err, testErr) {
		t.Fatalf("unexpected error: got %v, want %v", err, testErr)
	}
}

type countBlockReadsDB struct {
	database.DB
	blockReads int
}

type countBlockReadsTx struct {
	database.Tx
	db *countBlockReadsDB
}

func (db *countBlockReadsDB) View(fn func(database.Tx) error) error {
	return db.DB.View(func(dbTx database.Tx) error {
		return fn(&countBlockReadsTx{Tx: dbTx, db: db})
	})
}

func (tx *countBlockReadsTx) FetchBlock(hash *chainhash.Hash) ([]byte, error) {
	tx.db.blockReads++
	return tx.Tx.FetchBlock(hash)
}

func TestManagerCatchUpBlockReads(t *testing.T) {
	const target = 10
	tests := []struct {
		name      string
		tipHeight int32
		fastBuild bool
	}{
		{name: "fast_build_disabled", tipHeight: -1},
		{name: "index_not_empty", tipHeight: 0, fastBuild: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := setupAddrIndexTestChain(t, target)
			db := &countBlockReadsDB{DB: c.db}
			txIdx := NewTxIndex(db)
			idx := NewAddrIndex(db, &chaincfg.MainNetParams)
			if test.fastBuild {
				idx = NewAddrIndexWithDataDir(
					db, &chaincfg.MainNetParams, c.rootDir,
				)
			}
			manager := NewManager(db, []Indexer{txIdx, idx})
			require.NoError(t, manager.createIndexes())
			c.connect(t, txIdx, 0, test.tipHeight)
			c.connect(t, idx, 0, test.tipHeight)
			chain, err := blockchain.New(&blockchain.Config{
				DB: db, ChainParams: &chaincfg.MainNetParams,
				TimeSource: blockchain.NewMedianTime(),
			})
			require.NoError(t, err)

			// Chain initialization also reads blocks.
			db.blockReads = 0
			require.NoError(t, manager.Init(chain, nil))
			best := chain.BestSnapshot()
			for _, key := range [][]byte{txIndexKey, addrIndexKey} {
				var (
					hash   *chainhash.Hash
					height int32
				)
				err := db.View(func(dbTx database.Tx) error {
					var err error
					hash, height, err = dbFetchIndexerTip(
						dbTx, key,
					)
					return err
				})
				require.NoError(t, err)
				require.Equal(t, int32(target), height)
				require.Equal(t, best.Hash, *hash)
			}
			wantReads := int(target - test.tipHeight)
			require.Equal(t, wantReads, db.blockReads,
				"ordinary catchup should read each block once")
		})
	}
}

// catchUpTestIndex is an index that counts the blocks connected to it.
type catchUpTestIndex struct {
	key       []byte
	connected int
}

func (idx *catchUpTestIndex) Key() []byte {
	return idx.key
}

func (idx *catchUpTestIndex) Name() string {
	return string(idx.key)
}

func (idx *catchUpTestIndex) Create(database.Tx) error {
	return nil
}

func (idx *catchUpTestIndex) Init() error {
	return nil
}

func (idx *catchUpTestIndex) ConnectBlock(database.Tx, *btcutil.Block,
	[]blockchain.SpentTxOut) error {

	idx.connected++
	return nil
}

func (idx *catchUpTestIndex) DisconnectBlock(database.Tx, *btcutil.Block,
	[]blockchain.SpentTxOut) error {

	return nil
}

// newManagerTestChain returns a database with a chain that holds only the
// genesis block.
func newManagerTestChain(t *testing.T) (database.DB, *blockchain.BlockChain) {
	t.Helper()
	db, err := database.Create(
		"ffldb", filepath.Join(t.TempDir(), "db"), wire.MainNet,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	chain, err := blockchain.New(&blockchain.Config{
		DB: db, ChainParams: &chaincfg.MainNetParams,
		TimeSource: blockchain.NewMedianTime(),
	})
	require.NoError(t, err)
	return db, chain
}

// fetchTestIndexTips returns the tip heights of the indexes with the passed
// keys.
func fetchTestIndexTips(db database.DB, keys ...[]byte) ([]int32, error) {
	heights := make([]int32, len(keys))
	err := db.View(func(dbTx database.Tx) error {
		for i, key := range keys {
			_, height, err := dbFetchIndexerTip(dbTx, key)
			if err != nil {
				return err
			}
			heights[i] = height
		}
		return nil
	})
	return heights, err
}

// TestManagerCatchUp ensures Manager.Init creates a missing index and connects
// each block only to the indexes whose tips are behind it.
func TestManagerCatchUp(t *testing.T) {
	db, chain := newManagerTestChain(t)
	caughtUp := &catchUpTestIndex{key: []byte("caughtup")}
	behind := &catchUpTestIndex{key: []byte("behind")}
	best := chain.BestSnapshot()
	err := db.Update(func(dbTx database.Tx) error {
		meta := dbTx.Metadata()
		_, err := meta.CreateBucket(indexTipsBucketName)
		if err != nil {
			return err
		}
		return dbPutIndexerTip(
			dbTx, caughtUp.key, &best.Hash, best.Height,
		)
	})
	require.NoError(t, err)

	manager := NewManager(db, []Indexer{caughtUp, behind})
	require.NoError(t, manager.Init(chain, nil))
	require.Zero(t, caughtUp.connected)
	require.Equal(t, 1, behind.connected)
	tips, err := fetchTestIndexTips(db, caughtUp.key, behind.key)
	require.NoError(t, err)
	require.Equal(t, []int32{best.Height, best.Height}, tips)
}

// fastBuildTestIndex is a FastBuilder that records the tips of other indexes
// when FastBuild runs and sets its own tip to the chain tip.
type fastBuildTestIndex struct {
	catchUpTestIndex
	db       database.DB
	needed   bool
	observed [][]byte
	tips     []int32
}

func (idx *fastBuildTestIndex) NeedsFastBuild(
	*blockchain.BlockChain) (bool, error) {

	return idx.needed, nil
}

func (idx *fastBuildTestIndex) FastBuild(chain *blockchain.BlockChain,
	interrupt <-chan struct{}) error {

	tips, err := fetchTestIndexTips(idx.db, idx.observed...)
	if err != nil {
		return err
	}
	idx.tips = tips

	best := chain.BestSnapshot()
	return idx.db.Update(func(dbTx database.Tx) error {
		return dbPutIndexerTip(dbTx, idx.key, &best.Hash, best.Height)
	})
}

// TestManagerFastBuild ensures Manager.Init catches up the indexes before a
// FastBuilder that needs a build, calls FastBuild before it catches up later
// indexes, and connects blocks only to the indexes the build left behind.
func TestManagerFastBuild(t *testing.T) {
	tests := []struct {
		name          string
		needed        bool
		wantTips      []int32
		wantConnected int
	}{
		{name: "build_not_needed", wantConnected: 1},
		{name: "build_needed", needed: true, wantTips: []int32{0, -1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, chain := newManagerTestChain(t)
			earlier := &catchUpTestIndex{key: []byte("earlier")}
			later := &catchUpTestIndex{key: []byte("later")}
			builder := &fastBuildTestIndex{
				catchUpTestIndex: catchUpTestIndex{
					key: []byte("builder"),
				},
				db:       db,
				needed:   test.needed,
				observed: [][]byte{earlier.key, later.key},
			}
			indexes := []Indexer{earlier, builder, later}
			manager := NewManager(db, indexes)
			require.NoError(t, manager.Init(chain, nil))

			require.Equal(t, test.wantTips, builder.tips,
				"earlier and later tips during FastBuild")
			require.Equal(t, test.wantConnected, builder.connected)
			require.Equal(t, 1, earlier.connected)
			require.Equal(t, 1, later.connected)
		})
	}
}
