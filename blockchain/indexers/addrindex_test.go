// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/wire"
)

// addrIndexBucket provides a mock address index database bucket by implementing
// the internalBucket interface.
type addrIndexBucket struct {
	levels map[[levelKeySize]byte][]byte
}

// Clone returns a deep copy of the mock address index bucket.
func (b *addrIndexBucket) Clone() *addrIndexBucket {
	levels := make(map[[levelKeySize]byte][]byte)
	for k, v := range b.levels {
		vCopy := make([]byte, len(v))
		copy(vCopy, v)
		levels[k] = vCopy
	}
	return &addrIndexBucket{levels: levels}
}

// Get returns the value associated with the key from the mock address index
// bucket.
//
// This is part of the internalBucket interface.
func (b *addrIndexBucket) Get(key []byte) []byte {
	var levelKey [levelKeySize]byte
	copy(levelKey[:], key)
	return b.levels[levelKey]
}

// Put stores the provided key/value pair to the mock address index bucket.
//
// This is part of the internalBucket interface.
func (b *addrIndexBucket) Put(key []byte, value []byte) error {
	var levelKey [levelKeySize]byte
	copy(levelKey[:], key)
	b.levels[levelKey] = value
	return nil
}

// Delete removes the provided key from the mock address index bucket.
//
// This is part of the internalBucket interface.
func (b *addrIndexBucket) Delete(key []byte) error {
	var levelKey [levelKeySize]byte
	copy(levelKey[:], key)
	delete(b.levels, levelKey)
	return nil
}

// printLevels returns a string with a visual representation of the provided
// address key taking into account the max size of each level.  It is useful
// when creating and debugging test cases.
func (b *addrIndexBucket) printLevels(addrKey [addrKeySize]byte) string {
	highestLevel := uint8(0)
	for k := range b.levels {
		if !bytes.Equal(k[:levelOffset], addrKey[:]) {
			continue
		}
		level := uint8(k[levelOffset])
		if level > highestLevel {
			highestLevel = level
		}
	}

	var levelBuf bytes.Buffer
	_, _ = levelBuf.WriteString("\n")
	maxEntries := level0MaxEntries
	for level := uint8(0); level <= highestLevel; level++ {
		data := b.levels[keyForLevel(addrKey, level)]
		numEntries := len(data) / txEntrySize
		for i := 0; i < numEntries; i++ {
			start := i * txEntrySize
			num := byteOrder.Uint32(data[start:])
			_, _ = levelBuf.WriteString(fmt.Sprintf("%02d ", num))
		}
		for i := numEntries; i < maxEntries; i++ {
			_, _ = levelBuf.WriteString("_  ")
		}
		_, _ = levelBuf.WriteString("\n")
		maxEntries *= 2
	}

	return levelBuf.String()
}

// sanityCheck ensures that all data stored in the bucket for the given address
// adheres to the level-based rules described by the address index
// documentation.
func (b *addrIndexBucket) sanityCheck(addrKey [addrKeySize]byte, expectedTotal int) error {
	// Find the highest level for the key.
	highestLevel := uint8(0)
	for k := range b.levels {
		if !bytes.Equal(k[:levelOffset], addrKey[:]) {
			continue
		}
		level := uint8(k[levelOffset])
		if level > highestLevel {
			highestLevel = level
		}
	}

	// Ensure the expected total number of entries are present and that
	// all levels adhere to the rules described in the address index
	// documentation.
	var totalEntries int
	maxEntries := level0MaxEntries
	for level := uint8(0); level <= highestLevel; level++ {
		// Level 0 can'have more entries than the max allowed if the
		// levels after it have data and it can't be empty.  All other
		// levels must either be half full or full.
		data := b.levels[keyForLevel(addrKey, level)]
		numEntries := len(data) / txEntrySize
		totalEntries += numEntries
		if level == 0 {
			if (highestLevel != 0 && numEntries == 0) ||
				numEntries > maxEntries {

				return fmt.Errorf("level %d has %d entries",
					level, numEntries)
			}
		} else if numEntries != maxEntries && numEntries != maxEntries/2 {
			return fmt.Errorf("level %d has %d entries", level,
				numEntries)
		}
		maxEntries *= 2
	}
	if totalEntries != expectedTotal {
		return fmt.Errorf("expected %d entries - got %d", expectedTotal,
			totalEntries)
	}

	// Ensure all of the numbers are in order starting from the highest
	// level moving to the lowest level.
	expectedNum := uint32(0)
	for level := highestLevel + 1; level > 0; level-- {
		data := b.levels[keyForLevel(addrKey, level)]
		numEntries := len(data) / txEntrySize
		for i := 0; i < numEntries; i++ {
			start := i * txEntrySize
			num := byteOrder.Uint32(data[start:])
			if num != expectedNum {
				return fmt.Errorf("level %d offset %d does "+
					"not contain the expected number of "+
					"%d - got %d", level, i, num,
					expectedNum)
			}
			expectedNum++
		}
	}

	return nil
}

// TestAddrIndexLevels ensures that adding and deleting entries to the address
// index creates multiple levels as described by the address index
// documentation.
func TestAddrIndexLevels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		key         [addrKeySize]byte
		numInsert   int
		printLevels bool // Set to help debug a specific test.
	}{
		{
			name:      "level 0 not full",
			numInsert: level0MaxEntries - 1,
		},
		{
			name:      "level 1 half",
			numInsert: level0MaxEntries + 1,
		},
		{
			name:      "level 1 full",
			numInsert: level0MaxEntries*2 + 1,
		},
		{
			name:      "level 2 half, level 1 half",
			numInsert: level0MaxEntries*3 + 1,
		},
		{
			name:      "level 2 half, level 1 full",
			numInsert: level0MaxEntries*4 + 1,
		},
		{
			name:      "level 2 full, level 1 half",
			numInsert: level0MaxEntries*5 + 1,
		},
		{
			name:      "level 2 full, level 1 full",
			numInsert: level0MaxEntries*6 + 1,
		},
		{
			name:      "level 3 half, level 2 half, level 1 half",
			numInsert: level0MaxEntries*7 + 1,
		},
		{
			name:      "level 3 full, level 2 half, level 1 full",
			numInsert: level0MaxEntries*12 + 1,
		},
	}

nextTest:
	for testNum, test := range tests {
		// Insert entries in order.
		populatedBucket := &addrIndexBucket{
			levels: make(map[[levelKeySize]byte][]byte),
		}
		for i := 0; i < test.numInsert; i++ {
			txLoc := wire.TxLoc{TxStart: i * 2}
			err := dbPutAddrIndexEntry(populatedBucket, test.key,
				uint32(i), txLoc)
			if err != nil {
				t.Errorf("dbPutAddrIndexEntry #%d (%s) - "+
					"unexpected error: %v", testNum,
					test.name, err)
				continue nextTest
			}
		}
		if test.printLevels {
			t.Log(populatedBucket.printLevels(test.key))
		}

		// Delete entries from the populated bucket until all entries
		// have been deleted.  The bucket is reset to the fully
		// populated bucket on each iteration so every combination is
		// tested.  Notice the upper limit purposes exceeds the number
		// of entries to ensure attempting to delete more entries than
		// there are works correctly.
		for numDelete := 0; numDelete <= test.numInsert+1; numDelete++ {
			// Clone populated bucket to run each delete against.
			bucket := populatedBucket.Clone()

			// Remove the number of entries for this iteration.
			err := dbRemoveAddrIndexEntries(bucket, test.key,
				numDelete)
			if err != nil {
				if numDelete <= test.numInsert {
					t.Errorf("dbRemoveAddrIndexEntries (%s) "+
						" delete %d - unexpected error: "+
						"%v", test.name, numDelete, err)
					continue nextTest
				}
			}
			if test.printLevels {
				t.Log(bucket.printLevels(test.key))
			}

			// Sanity check the levels to ensure the adhere to all
			// rules.
			numExpected := test.numInsert
			if numDelete <= test.numInsert {
				numExpected -= numDelete
			}
			err = bucket.sanityCheck(test.key, numExpected)
			if err != nil {
				t.Errorf("sanity check fail (%s) delete %d: %v",
					test.name, numDelete, err)
				continue nextTest
			}
		}
	}
}
