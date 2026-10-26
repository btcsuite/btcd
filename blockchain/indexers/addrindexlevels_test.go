// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// addrBuildTestEntry returns the block ID and transaction location of test
// entry i.  Every three entries share a block ID and the start offset grows
// with i, so entries in order of i are in chain order.
func addrBuildTestEntry(i int) (uint32, wire.TxLoc) {
	return uint32(i / 3), wire.TxLoc{TxStart: i * 4, TxLen: i + 1}
}

// addrBuildTestRecords returns the staging records of the test entries in
// [start, end) for the provided address key.
func addrBuildTestRecords(addrKey [addrKeySize]byte, start,
	end int) []addrRecord {

	records := make([]addrRecord, 0, end-start)
	for i := start; i < end; i++ {
		blockID, txLoc := addrBuildTestEntry(i)
		record := makeAddrRecord(&addrKey, blockID, txLoc)
		records = append(records, record)
	}
	return records
}

// putAddrBuildTestEntries adds the test entries in [start, end) to bucket under
// the provided address key, one at a time with dbPutAddrIndexEntry, the way
// ConnectBlock adds them.
func putAddrBuildTestEntries(t *testing.T, bucket internalBucket,
	addrKey [addrKeySize]byte, start, end int) {

	t.Helper()
	for i := start; i < end; i++ {
		blockID, txLoc := addrBuildTestEntry(i)
		require.NoError(t, dbPutAddrIndexEntry(
			bucket, addrKey, blockID, txLoc,
		))
	}
}

// countAddrUpdatesDB wraps a database and counts the calls to Update.
type countAddrUpdatesDB struct {
	database.DB
	updates int
}

// Update counts the call and passes it to the wrapped database.
//
// This is part of the database.DB interface.
func (db *countAddrUpdatesDB) Update(fn func(database.Tx) error) error {
	db.updates++
	return db.DB.Update(fn)
}

// requireAddrBuildLevels requires got and want to hold the same level keys and
// values.
func requireAddrBuildLevels(t *testing.T, got,
	want map[[levelKeySize]byte][]byte) {

	t.Helper()
	require.Len(t, got, len(want))
	for key, value := range want {
		require.Equalf(t, value, got[key], "level %x", key)
	}
}

// TestAddrLevelEntryCounts ensures addrLevelEntryCounts returns the number of
// entries in each level that dbPutAddrIndexEntry leaves, for 0 through 10000
// entries added one at a time.
func TestAddrLevelEntryCounts(t *testing.T) {
	t.Parallel()

	var addrKey [addrKeySize]byte
	bucket := &addrIndexBucket{levels: make(map[[levelKeySize]byte][]byte)}
	for numEntries := 0; numEntries <= 10000; numEntries++ {
		if numEntries > 0 {
			putAddrBuildTestEntries(
				t, bucket, addrKey, numEntries-1, numEntries,
			)
		}

		counts := addrLevelEntryCounts(numEntries)
		require.Len(t, counts, len(bucket.levels))
		for level, count := range counts {
			key := keyForLevel(addrKey, uint8(level))
			got := len(bucket.levels[key]) / txEntrySize
			require.Equal(t, got, count)
		}
	}
}

// TestWriteAddrLevelValuesReplacesPrefix writes the layout of a prefix of an
// address's records, then the full records, and requires the layout that
// dbPutAddrIndexEntry leaves for the full records.
func TestWriteAddrLevelValuesReplacesPrefix(t *testing.T) {
	t.Parallel()

	var addrKey [addrKeySize]byte
	for total := 1; total <= 300; total++ {
		records := addrBuildTestRecords(addrKey, 0, total)
		want := &addrIndexBucket{
			levels: make(map[[levelKeySize]byte][]byte),
		}
		putAddrBuildTestEntries(t, want, addrKey, 0, total)
		for prefix := 0; prefix <= total; prefix++ {
			got := &addrIndexBucket{
				levels: make(map[[levelKeySize]byte][]byte),
			}
			if prefix > 0 {
				_, err := writeAddrLevelValues(
					got, records[:prefix],
				)
				require.NoError(t, err)
			}
			_, err := writeAddrLevelValues(got, records)
			require.NoError(t, err)
			requireAddrBuildLevels(t, got.levels, want.levels)
		}
	}
}

// TestWriteAddrIndexToDB ensures writeAddrIndexToDB replaces the staged heights
// with block IDs and writes the same levels dbPutAddrIndexEntry leaves.  It
// also ensures a database transaction stops at the next address boundary once
// it writes batchBytes, and otherwise writes every address of its shard.
func TestWriteAddrIndexToDB(t *testing.T) {
	// Block IDs decrease with height here, so the written order must come
	// from the staged heights.
	lastHeight, _ := addrBuildTestEntry(level0MaxEntries * 5)
	blockIDs := make([]uint32, lastHeight+1)
	for i := range blockIDs {
		blockIDs[i] = uint32(len(blockIDs) - i)
	}

	// Shard 200 holds two addresses so a database transaction can end
	// between them.
	addrs := []struct {
		shard, id byte
		count     int
	}{
		{0, 0, 1},
		{31, 0, level0MaxEntries + 1},
		{200, 0, level0MaxEntries*5 + 1},
		{200, 1, 3},
	}
	stager, err := newAddrStager(t.TempDir())
	require.NoError(t, err)
	reference := &addrIndexBucket{
		levels: make(map[[levelKeySize]byte][]byte),
	}
	for _, addr := range addrs {
		var addrKey [addrKeySize]byte
		addrKey[1] = addr.shard
		addrKey[2] = addr.id
		for i := addr.count - 1; i >= 0; i-- {
			height, txLoc := addrBuildTestEntry(i)
			err := stager.add(&addrKey, height, txLoc)
			require.NoError(t, err)
		}
		for i := 0; i < addr.count; i++ {
			height, txLoc := addrBuildTestEntry(i)
			require.NoError(t, dbPutAddrIndexEntry(
				reference, addrKey, blockIDs[height], txLoc,
			))
		}
	}
	manifest := addrBuildManifest{Completed: -1, TargetHeight: 0}
	require.NoError(t, stager.checkpoint(&manifest, nil))

	// requireWrite writes the staged records into a new database with the
	// provided batch size, and requires the reference levels and the
	// provided number of database transactions.
	requireWrite := func(batchBytes, updates int) {
		t.Helper()
		db, err := database.Create(
			"ffldb", filepath.Join(t.TempDir(), "db"), wire.MainNet,
		)
		require.NoError(t, err)
		defer db.Close()
		require.NoError(t, db.Update(func(dbTx database.Tx) error {
			_, err := dbTx.Metadata().CreateBucket(addrIndexKey)
			return err
		}))

		countingDB := &countAddrUpdatesDB{DB: db}
		idx := &AddrIndex{db: countingDB}
		err = idx.writeAddrIndexToDB(stager, blockIDs, batchBytes, nil)
		require.NoError(t, err)
		require.Equal(t, updates, countingDB.updates)
		levels := make(map[[levelKeySize]byte][]byte)
		require.NoError(t, db.View(func(dbTx database.Tx) error {
			bucket := dbTx.Metadata().Bucket(addrIndexKey)
			return bucket.ForEach(func(key, value []byte) error {
				var levelKey [levelKeySize]byte
				copy(levelKey[:], key)
				levels[levelKey] = append([]byte(nil), value...)
				return nil
			})
		}))
		requireAddrBuildLevels(t, levels, reference.levels)
	}

	// Every address reaches a one byte batch, so each one is written in
	// its own database transaction.
	requireWrite(1, len(addrs))

	// No shard reaches a batch of math.MaxInt bytes, so each of the three
	// shards is written in one database transaction, which holds both
	// addresses of shard 200.
	requireWrite(math.MaxInt, 3)
}
