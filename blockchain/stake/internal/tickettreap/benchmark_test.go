// Copyright (c) 2016-2017 The Decred developers
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
