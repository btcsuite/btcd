// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestAddrStagingRecord ensures a staging record holds the address key, block
// ID and serialized entry it was made from, and that sorting orders records by
// address key.
func TestAddrStagingRecord(t *testing.T) {
	t.Parallel()

	// The keys differ in the first byte of the address hash, and first has
	// the larger key, so it sorts after second.
	var firstKey, secondKey [addrKeySize]byte
	firstKey[1] = 2
	secondKey[1] = 1
	first := makeAddrRecord(
		&firstKey, 10, wire.TxLoc{TxStart: 20, TxLen: 30},
	)
	second := makeAddrRecord(
		&secondKey, 11, wire.TxLoc{TxStart: 21, TxLen: 31},
	)

	require.Equal(t, firstKey, first.key())
	require.Equal(t, uint32(10), first.blockID())
	require.Equal(t, serializeAddrIndexEntry(
		10, wire.TxLoc{TxStart: 20, TxLen: 30},
	), first[addrKeySize:])

	records := addrRecords{first, second}
	sort.Sort(records)
	require.Equal(t, secondKey, records[0].key())
}

// TestAddrStagingCheckpointResume ensures that resuming from a checkpoint
// truncates a longer staging file to its recorded size, which drops the
// records written after the checkpoint, and that new records are appended
// after the recorded size.
func TestAddrStagingCheckpointResume(t *testing.T) {
	dir := t.TempDir()
	stager, err := newAddrStager(dir)
	require.NoError(t, err)

	// The first byte of the address hash selects the shard, so the records
	// of the two keys are staged in different staging files.
	var firstKey, secondKey [addrKeySize]byte
	firstKey[1] = 3
	secondKey[1] = 200
	require.NoError(t, stager.add(
		&firstKey, 1, wire.TxLoc{TxStart: 2, TxLen: 3},
	))
	require.NoError(t, stager.add(
		&secondKey, 4, wire.TxLoc{TxStart: 5, TxLen: 6},
	))

	// The checkpoint makes the two records durable, and ShardSizes holds
	// the size of each staging file.
	manifest := addrBuildManifest{
		Completed:    10,
		TargetHeight: 20,
		TargetHash:   chainhash.Hash{1},
	}
	require.NoError(t, stager.checkpoint(&manifest, nil))
	checkpointSizes := manifest.ShardSizes

	// This record is durable but is not part of the checkpoint.
	require.NoError(t, stager.add(
		&firstKey, 7, wire.TxLoc{TxStart: 8, TxLen: 9},
	))
	shard := &stager.shards[firstKey[1]]
	require.NoError(t, shard.buf.Flush())
	require.NoError(t, shard.f.Sync())
	stager.close()

	// A resumed scan reads the manifest from the staging directory, so
	// resume from the manifest the checkpoint wrote.
	saved, ok := readAddrBuildManifest(dir)
	require.True(t, ok)
	require.Equal(t, manifest, saved)
	resumed, err := openAddrStager(dir, &saved, nil)
	require.NoError(t, err)
	defer resumed.close()

	// Resuming truncates the staging file of the first key to its recorded
	// size, which drops the record added after the checkpoint.  Every
	// staging file is at its recorded size, and each key has only the
	// record staged before the checkpoint.
	for i, size := range checkpointSizes {
		info, err := os.Stat(addrStagingShardPath(dir, i))
		require.NoError(t, err)
		require.Equalf(t, size, info.Size(), "shard %d", i)
	}
	firstRecords, err := readAddrStagingShard(
		addrStagingShardPath(dir, int(firstKey[1])), nil,
	)
	require.NoError(t, err)
	require.Equal(t, []addrRecord{
		makeAddrRecord(&firstKey, 1, wire.TxLoc{TxStart: 2, TxLen: 3}),
	}, firstRecords)
	secondRecords, err := readAddrStagingShard(
		addrStagingShardPath(dir, int(secondKey[1])), nil,
	)
	require.NoError(t, err)
	require.Equal(t, []addrRecord{
		makeAddrRecord(&secondKey, 4, wire.TxLoc{TxStart: 5, TxLen: 6}),
	}, secondRecords)

	// Records staged after resuming follow the checkpointed records.
	require.NoError(t, resumed.add(
		&firstKey, 10, wire.TxLoc{TxStart: 11, TxLen: 12},
	))
	require.NoError(t, resumed.checkpoint(&saved, nil))
	firstRecords, err = readAddrStagingShard(
		addrStagingShardPath(dir, int(firstKey[1])), nil,
	)
	require.NoError(t, err)
	require.Equal(t, []addrRecord{
		makeAddrRecord(&firstKey, 1, wire.TxLoc{TxStart: 2, TxLen: 3}),
		makeAddrRecord(
			&firstKey, 10, wire.TxLoc{TxStart: 11, TxLen: 12},
		),
	}, firstRecords)
}

// TestAddrStagingManifestValidation ensures readAddrBuildManifest returns a
// valid manifest as it was written and reports a manifest that cannot be
// decoded or that breaks one of its rules as not valid, and that
// openAddrStager returns errInterruptRequested when an interrupt is already
// requested.
func TestAddrStagingManifestValidation(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, addrBuildManifestName)

	// validManifest returns a manifest that readAddrBuildManifest accepts,
	// with one staging file that holds two records.
	validManifest := func() addrBuildManifest {
		shardSizes := make([]int64, numAddrStagingShards)
		shardSizes[1] = 2 * addrRecordSize
		return addrBuildManifest{
			Version:      addrBuildManifestVersion,
			Completed:    10,
			TargetHeight: 20,
			TargetHash:   chainhash.Hash{1},
			ShardSizes:   shardSizes,
		}
	}

	// A valid manifest reads back as writeAddrBuildManifest wrote it.
	manifest := validManifest()
	err := writeAddrBuildManifest(dir, &manifest)
	require.NoError(t, err)
	saved, ok := readAddrBuildManifest(dir)
	require.True(t, ok)
	require.Equal(t, manifest, saved)

	require.NoError(t, os.WriteFile(manifestPath, []byte("invalid"), 0600))
	_, ok = readAddrBuildManifest(dir)
	require.False(t, ok)

	// Each test changes a valid manifest so that it breaks exactly one
	// rule.  The manifest is written with json.Marshal, since
	// writeAddrBuildManifest would set the version.
	tests := []struct {
		name   string
		modify func(manifest *addrBuildManifest)
	}{{
		name: "other version",
		modify: func(manifest *addrBuildManifest) {
			manifest.Version++
		},
	}, {
		name: "negative target height",
		modify: func(manifest *addrBuildManifest) {
			manifest.Completed = -1
			manifest.TargetHeight = -1
		},
	}, {
		name: "completed height below -1",
		modify: func(manifest *addrBuildManifest) {
			manifest.Completed = -2
		},
	}, {
		name: "completed height above the target height",
		modify: func(manifest *addrBuildManifest) {
			manifest.Completed = manifest.TargetHeight + 1
		},
	}, {
		name: "too few shard sizes",
		modify: func(manifest *addrBuildManifest) {
			manifest.ShardSizes = manifest.ShardSizes[:1]
		},
	}, {
		name: "too many shard sizes",
		modify: func(manifest *addrBuildManifest) {
			manifest.ShardSizes = append(manifest.ShardSizes, 0)
		},
	}, {
		name: "negative shard size",
		modify: func(manifest *addrBuildManifest) {
			manifest.ShardSizes[1] = -addrRecordSize
		},
	}, {
		name: "shard size with a partial record",
		modify: func(manifest *addrBuildManifest) {
			manifest.ShardSizes[1] = addrRecordSize + 1
		},
	}}
	for _, test := range tests {
		invalid := validManifest()
		test.modify(&invalid)
		data, err := json.Marshal(invalid)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(manifestPath, data, 0600))
		_, ok := readAddrBuildManifest(dir)
		require.False(t, ok, test.name)
	}

	interrupt := make(chan struct{})
	close(interrupt)
	_, err = openAddrStager(dir, &manifest, interrupt)
	require.ErrorIs(t, err, errInterruptRequested)
}
