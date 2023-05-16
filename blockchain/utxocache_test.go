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

// getValidP2PKHScript returns a valid P2PKH script.  Useful as unspendables cannot be
// added to the cache.
func getValidP2PKHScript() []byte {
	validP2PKHScript := []byte{
		// OP_DUP
		0x76,
		// OP_HASH160
		0xa9,
		// OP_DATA_20
		0x14,
		// <20-byte pubkey hash>
		0xf0, 0x7a, 0xb8, 0xce, 0x72, 0xda, 0x4e, 0x76,
		0x0b, 0x74, 0x7d, 0x48, 0xd6, 0x65, 0xec, 0x96,
		0xad, 0xf0, 0x24, 0xf5,
		// OP_EQUALVERIFY
		0x88,
		// OP_CHECKSIG
		0xac,
	}
	return validP2PKHScript
}

// outpointFromInt generates an outpoint from an int by hashing the int and making
// the given int the index.
func outpointFromInt(i int) wire.OutPoint {
	// Boilerplate to create an outpoint.
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], uint32(i))
	hash := sha256.Sum256(buf[:])
	return wire.OutPoint{Hash: hash, Index: uint32(i)}
}

func TestUtxoCacheEntrySize(t *testing.T) {
	type block struct {
		txOuts []*wire.TxOut
		outOps []wire.OutPoint
		txIns  []*wire.TxIn
	}
	tests := []struct {
		name         string
		blocks       []block
		expectedSize uint64
	}{
		{
			name: "one entry",
			blocks: func() []block {
				return []block{
					{
						txOuts: []*wire.TxOut{
							{Value: 10000, PkScript: getValidP2PKHScript()},
						},
						outOps: []wire.OutPoint{
							outpointFromInt(0),
						},
					},
				}
			}(),
			expectedSize: pubKeyHashLen + baseEntrySize,
		},
		{
			name: "10 entries, 4 spend",
			blocks: func() []block {
				blocks := make([]block, 0, 10)
				for i := 0; i < 10; i++ {
					op := outpointFromInt(i)

					block := block{
						txOuts: []*wire.TxOut{
							{Value: 10000, PkScript: getValidP2PKHScript()},
						},
						outOps: []wire.OutPoint{
							op,
						},
					}

					// Spend all outs in blocks less than 4.
					if i < 4 {
						block.txIns = []*wire.TxIn{
							{PreviousOutPoint: op},
						}
					}

					blocks = append(blocks, block)
				}
				return blocks
			}(),
			// Multipled by 6 since we'll have 6 entries left.
			expectedSize: (pubKeyHashLen + baseEntrySize) * 6,
		},
		{
			name: "spend everything",
			blocks: func() []block {
				blocks := make([]block, 0, 500)
				for i := 0; i < 500; i++ {
					op := outpointFromInt(i)

					block := block{
						txOuts: []*wire.TxOut{
							{Value: 1000, PkScript: getValidP2PKHScript()},
						},
						outOps: []wire.OutPoint{
							op,
						},
					}

					// Spend all outs in blocks less than 4.
					block.txIns = []*wire.TxIn{
						{PreviousOutPoint: op},
					}

					blocks = append(blocks, block)
				}
				return blocks
			}(),
			expectedSize: 0,
		},
	}

	for _, test := range tests {
		// Size is just something big enough so that the mapslice doesn't
		// run out of memory.
		s := newUtxoCache(nil, 1*1024*1024)

		for height, block := range test.blocks {
			for i, out := range block.txOuts {
				s.addTxOut(block.outOps[i], out, true, int32(height))
			}

			for _, in := range block.txIns {
				s.addTxIn(in, nil)
			}
		}

		if s.totalEntryMemory != test.expectedSize {
			t.Errorf("Failed test %s. Expected size of %d, got %d",
				test.name, test.expectedSize, s.totalEntryMemory)
		}
	}
}
