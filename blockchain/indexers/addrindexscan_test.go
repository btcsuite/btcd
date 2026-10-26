// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"math"
	"sort"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/database"
	"github.com/stretchr/testify/require"
)

// TestScanAddrHeightRangeCompleted ensures a scan whose start is past its end
// returns without reading any block, including at the largest int32 height.
func TestScanAddrHeightRangeCompleted(t *testing.T) {
	t.Parallel()

	idx := &AddrIndex{}
	for _, end := range []int64{0, math.MaxInt32} {
		require.NoError(t, idx.scanAddrHeightRange(
			nil, nil, end+1, end, addrBuildDecodedScanMaxWorkers,
			nil, nil,
		))
	}
}

// applyAddrBuildRecords sorts records and writes the level layout for the
// records of each address to bucket with writeAddrLevelValues.
func applyAddrBuildRecords(t *testing.T, bucket *addrIndexBucket,
	records []addrRecord) {

	t.Helper()
	sort.Sort(addrRecords(records))
	for start := 0; start < len(records); {
		// The sort puts the records of each address next to each other,
		// so end advances to the end of the records for the address of
		// records[start].
		addrKey := records[start].key()
		end := start + 1
		for end < len(records) && records[end].key() == addrKey {
			end++
		}

		_, err := writeAddrLevelValues(bucket, records[start:end])
		require.NoError(t, err)
		start = end
	}
}

// TestScanAddrHeightRange ensures the scan stages the entries that
// ConnectBlock adds and reports a block it cannot load.
func TestScanAddrHeightRange(t *testing.T) {
	const target = 300
	c := setupAddrIndexTestChain(t, target)
	txIdx := NewTxIndex(c.db)
	idx := NewAddrIndex(c.db, &chaincfg.MainNetParams)
	manager := NewManager(c.db, []Indexer{txIdx, idx})
	require.NoError(t, manager.createIndexes())

	// The staged heights are resolved to block IDs through the transaction
	// index.
	c.connect(t, txIdx, 0, target)

	// Index each block with ConnectBlock for the reference levels.
	c.connect(t, idx, 0, target)
	reference := readAddrIndexTestBucket(t, c.db)

	// Stage the same blocks with the scan.  The checkpoint flushes the
	// staged records to the staging files so they can be read back.
	stager, err := newAddrStager(t.TempDir())
	require.NoError(t, err)
	defer stager.close()
	progress := newAddrScanProgress(target + 1)
	require.NoError(t, idx.scanAddrHeightRange(c.chain, stager, 0, target,
		addrBuildDecodedScanMaxWorkers, progress, nil))
	require.NoError(t, stager.checkpoint(&addrBuildManifest{}, nil))
	var records []addrRecord
	for shard := 0; shard < numAddrStagingShards; shard++ {
		staged, err := readAddrStagingShard(
			addrStagingShardPath(stager.dir, shard), nil,
		)
		require.NoError(t, err)
		records = append(records, staged...)
	}

	// Resolve each staged height to the block ID that the transaction
	// index gave the block at that height.
	require.NoError(t, c.db.View(func(dbTx database.Tx) error {
		for i := range records {
			height := int32(records[i].blockID())
			hash, err := c.chain.BlockHashByHeight(height)
			if err != nil {
				return err
			}
			blockID, err := dbFetchBlockIDByHash(dbTx, hash)
			if err != nil {
				return err
			}
			byteOrder.PutUint32(records[i][addrKeySize:], blockID)
		}
		return nil
	}))

	// Write the levels from the staged records and require them to match
	// the reference.
	got := &addrIndexBucket{levels: make(map[[levelKeySize]byte][]byte)}
	applyAddrBuildRecords(t, got, records)
	requireAddrBuildLevels(t, got.levels, reference)

	// The chain ends at target, so loading the block after it fails.
	err = idx.scanAddrHeightRange(c.chain, stager, target-1, target+1,
		addrBuildDecodedScanMaxWorkers, newAddrScanProgress(3), nil)
	require.Error(t, err)
}
