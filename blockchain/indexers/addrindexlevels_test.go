// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"testing"

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
