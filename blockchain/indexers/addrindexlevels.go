// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

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
