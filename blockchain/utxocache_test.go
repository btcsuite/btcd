// Copyright (c) 2023 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
package blockchain

import (
	"crypto/sha256"
	"encoding/binary"
	"reflect"
	"sync"
	"testing"

	"github.com/btcsuite/btcd/wire"
)

func TestMapSlice(t *testing.T) {
	tests := []struct {
		keys []wire.OutPoint
	}{
		{
			keys: func() []wire.OutPoint {
				outPoints := make([]wire.OutPoint, 1000)
				for i := uint32(0); i < uint32(len(outPoints)); i++ {
					var buf [4]byte
					binary.BigEndian.PutUint32(buf[:], i)
					hash := sha256.Sum256(buf[:])

					op := wire.OutPoint{Hash: hash, Index: i}
					outPoints[i] = op
				}
				return outPoints
			}(),
		},
	}

	for _, test := range tests {
		m := make(map[wire.OutPoint]*UtxoEntry)

		maxSize := calculateRoughMapSize(1000, bucketSize)

		maxEntriesFirstMap := 500
		ms1 := make(map[wire.OutPoint]*UtxoEntry, maxEntriesFirstMap)
		ms := mapSlice{
			maps:                []map[wire.OutPoint]*UtxoEntry{ms1},
			maxEntries:          []int{maxEntriesFirstMap},
			maxTotalMemoryUsage: uint64(maxSize),
		}

		for _, key := range test.keys {
			m[key] = nil
			ms.put(key, nil, 0)
		}

		// Put in the same elements twice to test that the map slice won't hold duplicates.
		for _, key := range test.keys {
			m[key] = nil
			ms.put(key, nil, 0)
		}

		if len(m) != ms.length() {
			t.Fatalf("expected len of %d, got %d", len(m), ms.length())
		}

		for _, key := range test.keys {
			expected, found := m[key]
			if !found {
				t.Fatalf("expected key %s to exist in the go map", key.String())
			}

			got, found := ms.get(key)
			if !found {
				t.Fatalf("expected key %s to exist in the map slice", key.String())
			}

			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("expected value of %v, got %v", expected, got)
			}
		}
	}
}

// TestMapsliceConcurrency just tests that the mapslice won't result in a panic
// on concurrent access.
func TestMapsliceConcurrency(t *testing.T) {
	tests := []struct {
		keys []wire.OutPoint
	}{
		{
			keys: func() []wire.OutPoint {
				outPoints := make([]wire.OutPoint, 10000)
				for i := uint32(0); i < uint32(len(outPoints)); i++ {
					var buf [4]byte
					binary.BigEndian.PutUint32(buf[:], i)
					hash := sha256.Sum256(buf[:])

					op := wire.OutPoint{Hash: hash, Index: i}
					outPoints[i] = op
				}
				return outPoints
			}(),
		},
	}

	for _, test := range tests {
		maxSize := calculateRoughMapSize(1000, bucketSize)

		maxEntriesFirstMap := 500
		ms1 := make(map[wire.OutPoint]*UtxoEntry, maxEntriesFirstMap)
		ms := mapSlice{
			maps:                []map[wire.OutPoint]*UtxoEntry{ms1},
			maxEntries:          []int{maxEntriesFirstMap},
			maxTotalMemoryUsage: uint64(maxSize),
		}

		var wg sync.WaitGroup

		wg.Add(1)
		go func(m *mapSlice, keys []wire.OutPoint) {
			defer wg.Done()
			for i := 0; i < 5000; i++ {
				m.put(keys[i], nil, 0)
			}
		}(&ms, test.keys)

		wg.Add(1)
		go func(m *mapSlice, keys []wire.OutPoint) {
			defer wg.Done()
			for i := 5000; i < 10000; i++ {
				m.put(keys[i], nil, 0)
			}
		}(&ms, test.keys)

		wg.Add(1)
		go func(m *mapSlice) {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				m.size()
			}
		}(&ms)

		wg.Add(1)
		go func(m *mapSlice) {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				m.length()
			}
		}(&ms)

		wg.Add(1)
		go func(m *mapSlice, keys []wire.OutPoint) {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				m.get(keys[i])
			}
		}(&ms, test.keys)

		wg.Add(1)
		go func(m *mapSlice, keys []wire.OutPoint) {
			defer wg.Done()
			for i := 0; i < 5000; i++ {
				m.delete(keys[i])
			}
		}(&ms, test.keys)

		wg.Wait()
	}
}
