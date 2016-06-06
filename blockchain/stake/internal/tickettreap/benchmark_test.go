// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package tickettreap

import (
	"crypto/sha256"
	"sync"
	"testing"
)

// numTicketKeys is the number of keys to generate for use in the benchmarks.
const numTicketKeys = 42500

var (
	// generatedTicketKeys is used to store ticket keys generated for use
	// in the benchmarks so that they only need to be generatd once for all
	// benchmarks that use them.
	genTicketKeysLock   sync.Mutex
	generatedTicketKeys []Key
)

// genTicketKeys generates and returns 'numTicketKeys' along with memoizing them
// so that future calls return the cached data.
func genTicketKeys() []Key {
	// Return generated keys if already done.
	genTicketKeysLock.Lock()
	defer genTicketKeysLock.Unlock()
	if generatedTicketKeys != nil {
		return generatedTicketKeys
	}

	// Generate the keys and cache them for future invocations.
	ticketKeys := make([]Key, 0, numTicketKeys)
	for i := 0; i < numTicketKeys; i++ {
		key := Key(sha256.Sum256(serializeUint32(uint32(i))))
		ticketKeys = append(ticketKeys, key)
	}
	generatedTicketKeys = ticketKeys
	return ticketKeys
}

// BenchmarkMutableCopy benchmarks how long it takes to copy a mutable treap
// to another one when it contains 'numTicketKeys' entries.
func BenchmarkMutableCopy(b *testing.B) {
	// Populate mutable treap with a bunch of key/value pairs.
	testTreap := NewMutable()
	ticketKeys := genTicketKeys()
	for j := 0; j < len(ticketKeys); j++ {
		hashBytes := ticketKeys[j]
		value := &Value{Height: uint32(j)}
		testTreap.Put(hashBytes, value)
	}

	b.ReportAllocs()
	b.ResetTimer()

	// Copying a mutable treap requires iterating all of the entries and
	// populating them into a new treap all with a lock held for concurrency
	// safety.
	var mtx sync.RWMutex
	for i := 0; i < b.N; i++ {
		benchTreap := NewMutable()
		mtx.Lock()
		testTreap.ForEach(func(k Key, v *Value) bool {
			benchTreap.Put(k, v)
			return true
		})
		mtx.Unlock()
	}
}

// BenchmarkImmutableCopy benchmarks how long it takes to copy an immutable
// treap to another one when it contains 'numTicketKeys' entries.
func BenchmarkImmutableCopy(b *testing.B) {
	// Populate immutable treap with a bunch of key/value pairs.
	testTreap := NewImmutable()
	ticketKeys := genTicketKeys()
	for j := 0; j < len(ticketKeys); j++ {
		hashBytes := ticketKeys[j]
		value := &Value{Height: uint32(j)}
		testTreap = testTreap.Put(hashBytes, value)
	}

	b.ReportAllocs()
	b.ResetTimer()

	// Copying an immutable treap is just protecting access to the treap
	// root and getting another reference to it.
	var mtx sync.RWMutex
	for i := 0; i < b.N; i++ {
		mtx.RLock()
		benchTreap := testTreap
		mtx.RUnlock()
		_ = benchTreap
	}
}

// BenchmarkMutableCopy benchmarks how long it takes to iterate a mutable treap
// when it contains 'numTicketKeys' entries.
func BenchmarkMutableIterate(b *testing.B) {
	// Populate mutable treap with a bunch of key/value pairs.
	testTreap := NewMutable()
	ticketKeys := genTicketKeys()
	for j := 0; j < len(ticketKeys); j++ {
		hashBytes := ticketKeys[j]
		value := &Value{Height: uint32(j)}
		testTreap.Put(hashBytes, value)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		testTreap.ForEach(func(k Key, v *Value) bool {
			return true
		})
	}

}

// BenchmarkImmutableIterate benchmarks how long it takes to iterate an
// immutable treap when it contains 'numTicketKeys' entries.
func BenchmarkImmutableIterate(b *testing.B) {
	// Populate immutable treap with a bunch of key/value pairs.
	testTreap := NewImmutable()
	ticketKeys := genTicketKeys()
	for j := 0; j < len(ticketKeys); j++ {
		hashBytes := ticketKeys[j]
		value := &Value{Height: uint32(j)}
		testTreap = testTreap.Put(hashBytes, value)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		testTreap.ForEach(func(k Key, v *Value) bool {
			return true
		})
	}

}
