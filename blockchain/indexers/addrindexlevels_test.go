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
