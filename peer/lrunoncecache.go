// Copyright (c) 2015 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"bytes"
	"container/list"
	"fmt"
	"sync"
)

// lruNonceCache provides a concurrency safe cache that is limited to a maximum
// number of items with eviction for the oldest entry when the limit is
// exceeded.
type lruNonceCache struct {
	mtx        sync.Mutex
	nonceCache map[uint64]*list.Element // nearly O(1) lookups
	nonceList  *list.List               // O(1) insert, update, delete
	limit      uint
}

// String returns the cache as a human-readable string.
//
// This function is safe for concurrent access.
func (m *lruNonceCache) String() string {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	lastEntryNum := len(m.nonceCache) - 1
	curEntry := 0
	buf := bytes.NewBufferString("[")
	for nonce := range m.nonceCache {
		buf.WriteString(fmt.Sprintf("%d", nonce))
		if curEntry < lastEntryNum {
			buf.WriteString(", ")
		}
		curEntry++
	}
	buf.WriteString("]")

	return fmt.Sprintf("<%d>%s", m.limit, buf.String())
}

// Exists returns whether or not the passed nonce is in the cache.
//
// This function is safe for concurrent access.
func (m *lruNonceCache) Exists(nonce uint64) bool {
	m.mtx.Lock()
	_, exists := m.nonceCache[nonce]
	m.mtx.Unlock()

	return exists
}

// Add adds the passed nonce to the cache and handles eviction of the oldest item
// if adding the new item would exceed the max limit.  Adding an existing item
// makes it the most recently used item.
//
// This function is safe for concurrent access.
func (m *lruNonceCache) Add(nonce uint64) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	// When the limit is zero, nothing can be added to the cache, so just
	// return.
	if m.limit == 0 {
		return
	}

	// When the entry already exists move it to the front of the list
	// thereby marking it most recently used.
	if node, exists := m.nonceCache[nonce]; exists {
		m.nonceList.MoveToFront(node)
		return
	}

	// Evict the least recently used entry (back of the list) if the the new
	// entry would exceed the size limit for the cache.  Also reuse the list
	// node so a new one doesn't have to be allocated.
	if uint(len(m.nonceCache))+1 > m.limit {
		node := m.nonceList.Back()
		lru := node.Value.(uint64)

		// Evict least recently used item.
		delete(m.nonceCache, lru)

		// Reuse the list node of the item that was just evicted for the
		// new item.
		node.Value = nonce
		m.nonceList.MoveToFront(node)
		m.nonceCache[nonce] = node
		return
	}

	// The limit hasn't been reached yet, so just add the new item.
	node := m.nonceList.PushFront(nonce)
	m.nonceCache[nonce] = node
}

// Delete deletes the passed nonce from the cache (if it exists).
//
// This function is safe for concurrent access.
func (m *lruNonceCache) Delete(nonce uint64) {
	m.mtx.Lock()
	if node, exists := m.nonceCache[nonce]; exists {
		m.nonceList.Remove(node)
		delete(m.nonceCache, nonce)
	}
	m.mtx.Unlock()
}

// newLruNonceCache returns a new nonce cache that is limited to the number of
// entries specified by limit.  When the number of entries exceeds the limit,
// the oldest (least recently used) entry will be removed to make room for the
// new entry.
func newLruNonceCache(limit uint) *lruNonceCache {
	m := lruNonceCache{
		nonceCache: make(map[uint64]*list.Element),
		nonceList:  list.New(),
		limit:      limit,
	}
	return &m
}
