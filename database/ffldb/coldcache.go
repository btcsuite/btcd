// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Cold-tier decompressed-block LRU cache.
//
// Cold blocks are stored on disk as witness-stripped + zstd-compressed records.
// Reading a region of a cold block (e.g. the 80-byte header for
// FetchBlockHeader, or a transaction for an indexer) requires decompressing the
// entire stripped block first. Without a cache, each region read re-decompresses
// the same block — a header fetch followed by a tx fetch decompresses twice.
//
// This cache holds the most recently decompressed stripped blocks, keyed by
// their on-disk blockLocation (fileNum|offset|len), which uniquely identifies a
// cold record. Cold blocks are immutable (witness is stripped at compaction
// time and the record is never modified), so cache entries never need
// invalidation.
//
// The cache is bounded by entry count (maxColdCacheBlocks). A byte bound would
// be tighter on memory but adds bookkeeping per hit/miss; count-bounded is
// simpler and sufficient given blocks are at most ~4MB stripped and the default
// of 20 entries is ~80MB worst case.

package ffldb

import (
	"container/list"
	"sync"
)

// maxColdCacheBlocks is the maximum number of decompressed cold blocks to keep
// in memory. Chosen to match maxOpenFiles so a sequential scan touching each
// cold file keeps its hottest block cached without thrashing.
const maxColdCacheBlocks = 20

// coldBlockCache is a bounded LRU of decompressed cold-tier stripped blocks,
// keyed by blockLocation. It is goroutine-safe.
type coldBlockCache struct {
	mu sync.Mutex

	ll    *list.List                   // Front = most recently used.
	elems map[blockLocation]*list.Element // Map from location to list element.
}

// coldCacheEntry is the value stored in the LRU list.
type coldCacheEntry struct {
	loc   blockLocation
	bytes []byte // decompressed stripped block
}

// newColdBlockCache returns an empty cold block cache.
func newColdBlockCache() *coldBlockCache {
	return &coldBlockCache{
		ll:    list.New(),
		elems: make(map[blockLocation]*list.Element),
	}
}

// get returns the cached decompressed bytes for loc, or nil if not present.
// On a hit, loc is moved to the front of the LRU.
func (c *coldBlockCache) get(loc blockLocation) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.elems[loc]; ok {
		c.ll.MoveToFront(elem)
		return elem.Value.(*coldCacheEntry).bytes
	}
	return nil
}

// put inserts a decompressed cold block into the cache, evicting the least
// recently used entry if the cache is at capacity. The bytes slice is stored
// as-is (callers must not mutate it after inserting).
func (c *coldBlockCache) put(loc blockLocation, bytes []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If already present (concurrent decompression of the same block), just
	// update and move to front.
	if elem, ok := c.elems[loc]; ok {
		elem.Value.(*coldCacheEntry).bytes = bytes
		c.ll.MoveToFront(elem)
		return
	}

	entry := &coldCacheEntry{loc: loc, bytes: bytes}
	elem := c.ll.PushFront(entry)
	c.elems[loc] = elem

	if c.ll.Len() > maxColdCacheBlocks {
		oldest := c.ll.Back()
		if oldest != nil {
			c.ll.Remove(oldest)
			delete(c.elems, oldest.Value.(*coldCacheEntry).loc)
		}
	}
}

// len returns the number of entries currently in the cache (for testing).
func (c *coldBlockCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
