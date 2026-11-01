// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"fmt"
	"sort"

	"github.com/btcsuite/btcd/database"
)

// addrBuildWriteBatchBytes is the batch size of the write pass.  A database
// transaction stops at the next address boundary once it writes this many level
// key and value bytes.  A transaction keeps its writes in memory until it
// commits, so the batch size limits how much memory it holds.  The number of
// entries of an address varies widely, so the batch size is counted in bytes
// rather than in addresses.
const addrBuildWriteBatchBytes = 32 * 1024 * 1024

// addrLevelEntryCounts returns the number of entries in each address index
// level, from level 0 up, after numEntries entries are added one at a time
// with dbPutAddrIndexEntry.
//
// dbPutAddrIndexEntry keeps level 0 between 1 and level0MaxEntries entries and
// every higher level half full or full.  When level 0 is full, it finds the
// lowest level that is not full and appends the full level below it, so that
// level goes from empty to half full or from half full to full.  Each full
// level further down moves up one level, where it is half full, and the new
// entry goes in level 0.
//
// The counts follow from numEntries alone, because every level above a level
// k holds a multiple of maxEntriesForLevel(k).  Level 0 holds the count between
// 1 and level0MaxEntries that leaves a multiple of level0MaxEntries for the
// levels above it.  Each higher level is full when the entries left for it and
// the levels above it are a multiple of its maximum, and half full otherwise.
// For example, 100 entries put 4 in level 0, 16 in level 1, 16 in level 2 and
// 64 in level 3.
func addrLevelEntryCounts(numEntries int) []int {
	if numEntries == 0 {
		return nil
	}

	levelCounts := []int{(numEntries-1)%level0MaxEntries + 1}
	remaining := numEntries - levelCounts[0]
	for level := uint8(1); remaining > 0; level++ {
		maxEntries := maxEntriesForLevel(level)
		levelEntries := maxEntries / 2
		if remaining%maxEntries == 0 {
			levelEntries = maxEntries
		}
		levelCounts = append(levelCounts, levelEntries)
		remaining -= levelEntries
	}
	return levelCounts
}

// addrRecordLevelValues constructs the complete level layout for the records
// of one address, which must be ordered from oldest to newest.  The values
// share one buffer of the serialized entries, with older entries in higher
// levels.
func addrRecordLevelValues(records []addrRecord) [][]byte {
	entries := make([]byte, 0, len(records)*txEntrySize)
	for i := range records {
		entries = append(entries, records[i][addrKeySize:]...)
	}

	counts := addrLevelEntryCounts(len(records))
	levels := make([][]byte, len(counts))
	offset := 0
	for level := len(counts) - 1; level >= 0; level-- {
		end := offset + counts[level]*txEntrySize
		levels[level] = entries[offset:end]
		offset = end
	}
	return levels
}

// writeAddrLevelValues writes the level layout for the records of one address.
// Existing levels can only come from an interrupted write of this build, which
// wrote the layout of a prefix of these records.  Adding entries never removes
// a level, so putting each level replaces all of them.
func writeAddrLevelValues(bucket internalBucket,
	records []addrRecord) (int, error) {

	addrKey := records[0].key()
	levelBytes := 0
	for level, value := range addrRecordLevelValues(records) {
		key := keyForLevel(addrKey, uint8(level))
		if err := bucket.Put(key[:], value); err != nil {
			return 0, err
		}
		levelBytes += len(value) + levelKeySize
	}
	return levelBytes, nil
}

// addrRecordsEnd returns the end of the records for the address of
// records[start].
func addrRecordsEnd(records []addrRecord, start int,
	interrupt <-chan struct{}) (int, error) {

	addrKey := records[start].key()
	end := start + 1
	for end < len(records) && records[end].key() == addrKey {
		if (end-start)%addrStagingInterruptCheckRecords == 0 &&
			interruptRequested(interrupt) {

			return 0, errInterruptRequested
		}
		end++
	}
	return end, nil
}

// writeAddrIndexToDB sorts and writes one staging shard at a time.  blockIDs
// holds the block ID of each staged block, indexed by height.
// Database transactions stop at address boundaries once they write batchBytes.
func (idx *AddrIndex) writeAddrIndexToDB(stager *addrStager,
	blockIDs []uint32, batchBytes int, interrupt <-chan struct{}) error {

	stager.close()
	for shard := 0; shard < numAddrStagingShards; shard++ {
		if interruptRequested(interrupt) {
			return errInterruptRequested
		}

		records, err := readAddrStagingShard(
			addrStagingShardPath(stager.dir, shard), interrupt,
		)
		if err != nil {
			return err
		}

		// Staged records hold heights, so they are sorted into height
		// order before the heights are resolved to block IDs.
		sort.Sort(addrRecords(records))
		for i := range records {
			if i%addrStagingInterruptCheckRecords == 0 &&
				interruptRequested(interrupt) {

				return errInterruptRequested
			}
			height := int64(records[i].blockID())
			if height >= int64(len(blockIDs)) {
				return fmt.Errorf(
					"address staging block height %d "+
						"is outside the build range",
					height,
				)
			}
			byteOrder.PutUint32(
				records[i][addrKeySize:], blockIDs[height],
			)
		}

		for start := 0; start < len(records); {
			next := start
			err := idx.db.Update(func(dbTx database.Tx) error {
				bucket := dbTx.Metadata().Bucket(addrIndexKey)
				written := 0
				for next < len(records) {
					if interruptRequested(interrupt) {
						return errInterruptRequested
					}

					end, err := addrRecordsEnd(
						records, next, interrupt,
					)
					if err != nil {
						return err
					}
					levelBytes, err := writeAddrLevelValues(
						bucket, records[next:end],
					)
					if err != nil {
						return err
					}
					written += levelBytes
					next = end
					if written >= batchBytes {
						break
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			start = next
		}

		if (shard+1)%16 == 0 {
			log.Infof("Address index write: %d/%d shards", shard+1,
				numAddrStagingShards)
		}
	}
	return nil
}
