// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package treap

import (
	"crypto/sha256"
	"testing"
)

// BenchmarkImmutablePutBatch benchmarks batched Put operations which exercise
// the pool-based recycling of intermediate treap nodes.
func BenchmarkImmutablePutBatch(b *testing.B) {
	batchSizes := []struct {
		name string
		size int
	}{
		{"batch=1", 1},
		{"batch=10", 10},
		{"batch=100", 100},
		{"batch=1000", 1000},
	}

	for _, bs := range batchSizes {
		b.Run(bs.name, func(b *testing.B) {
			// Pre-generate all keys to avoid measuring key
			// generation overhead.
			keys := make([]KVPair, bs.size)
			for i := range keys {
				hash := sha256.Sum256(serializeUint32(uint32(i)))
				keys[i] = KVPair{
					Key:   hash[:],
					Value: hash[:],
				}
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				t := NewImmutable()
				t = t.Put(keys...)
			}
		})
	}
}

// BenchmarkImmutableDeleteBatch benchmarks batched Delete operations which
// exercise the pool-based recycling of intermediate treap nodes.
func BenchmarkImmutableDeleteBatch(b *testing.B) {
	batchSizes := []struct {
		name string
		size int
	}{
		{"batch=1", 1},
		{"batch=10", 10},
		{"batch=100", 100},
		{"batch=1000", 1000},
	}

	for _, bs := range batchSizes {
		b.Run(bs.name, func(b *testing.B) {
			// Pre-generate keys and build the initial treap.
			kvPairs := make([]KVPair, bs.size)
			delKeys := make([][]byte, bs.size)
			for i := range kvPairs {
				hash := sha256.Sum256(serializeUint32(uint32(i)))
				kvPairs[i] = KVPair{
					Key:   hash[:],
					Value: hash[:],
				}
				delKeys[i] = hash[:]
			}
			baseTreap := NewImmutable().Put(kvPairs...)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				baseTreap.Delete(delKeys...)
			}
		})
	}
}

// BenchmarkImmutablePutSequential benchmarks sequential Put operations on a
// growing treap, which is the hot path during initial block download when
// populating the UTXO cache.
func BenchmarkImmutablePutSequential(b *testing.B) {
	const batchSize = 100

	// Pre-generate keys.
	keys := make([]KVPair, batchSize)
	for i := range keys {
		key := serializeUint32(uint32(i))
		keys[i] = KVPair{Key: key, Value: key}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		t := NewImmutable()
		// Simulate multiple rounds of batch inserts to exercise
		// recycling across iterations.
		for round := 0; round < 10; round++ {
			roundKeys := make([]KVPair, batchSize)
			for j := range roundKeys {
				key := serializeUint32(
					uint32(round*batchSize + j),
				)
				roundKeys[j] = KVPair{Key: key, Value: key}
			}
			t = t.Put(roundKeys...)
		}
	}
}

// BenchmarkImmutableMixedPutDelete benchmarks interleaved batch Put and Delete
// operations, simulating the commitTx pattern in dbcache.go.
func BenchmarkImmutableMixedPutDelete(b *testing.B) {
	const numKeys = 500

	// Pre-generate keys.
	putKVs := make([]KVPair, numKeys)
	delKeys := make([][]byte, numKeys/2)
	for i := 0; i < numKeys; i++ {
		hash := sha256.Sum256(serializeUint32(uint32(i)))
		putKVs[i] = KVPair{Key: hash[:], Value: hash[:]}
		if i < numKeys/2 {
			delKeys[i] = hash[:]
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		t := NewImmutable()
		t = t.Put(putKVs...)
		t = t.Delete(delKeys...)
	}
}
