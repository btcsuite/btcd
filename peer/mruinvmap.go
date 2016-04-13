// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"bytes"
	"container/list"
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/wire"
)

// mruInventoryMap provides a concurrency safe map that is limited to a maximum
// number of items with eviction for the oldest entry when the limit is
// exceeded.
type mruInventoryMap struct {
	invMtx  sync.Mutex
	invMap  map[wire.InvVect]*list.Element // nearly O(1) lookups
	invList *list.List                     // O(1) insert, update, delete
	limit   uint
}

// String returns the map as a human-readable string.
//
// This function is safe for concurrent access.
func (m *mruInventoryMap) String() string {
	m.invMtx.Lock()
	defer m.invMtx.Unlock()

	lastEntryNum := len(m.invMap) - 1
	curEntry := 0
	buf := bytes.NewBufferString("[")
	for iv := range m.invMap {
		buf.WriteString(fmt.Sprintf("%v", iv))
		if curEntry < lastEntryNum {
			buf.WriteString(", ")
		}
		curEntry++
	}
	buf.WriteString("]")

	return fmt.Sprintf("<%d>%s", m.limit, buf.String())
}

// Exists returns whether or not the passed inventory item is in the map.
//
// This function is safe for concurrent access.
func (m *mruInventoryMap) Exists(iv *wire.InvVect) bool {
	m.invMtx.Lock()
	_, exists := m.invMap[*iv]
	m.invMtx.Unlock()

	return exists
}

// Add adds the passed inventory to the map and handles eviction of the oldest
// item if adding the new item would exceed the max limit.  Adding an existing
// item makes it the most recently used item.
//
// This function is safe for concurrent access.
func (m *mruInventoryMap) Add(iv *wire.InvVect) {
	m.invMtx.Lock()
	defer m.invMtx.Unlock()

	// When the limit is zero, nothing can be added to the map, so just
	// return.
	if m.limit == 0 {
		return
	}

	// When the entry already exists move it to the front of the list
	// thereby marking it most recently used.
	if node, exists := m.invMap[*iv]; exists {
		m.invList.MoveToFront(node)
		return
	}

	// Evict the least recently used entry (back of the list) if the the new
	// entry would exceed the size limit for the map.  Also reuse the list
	// node so a new one doesn't have to be allocated.
	if uint(len(m.invMap))+1 > m.limit {
		node := m.invList.Back()
		lru := node.Value.(*wire.InvVect)

		// Evict least recently used item.
		delete(m.invMap, *lru)

		// Reuse the list node of the item that was just evicted for the
		// new item.
		node.Value = iv
		m.invList.MoveToFront(node)
		m.invMap[*iv] = node
		return
	}

	// The limit hasn't been reached yet, so just add the new item.
	node := m.invList.PushFront(iv)
	m.invMap[*iv] = node
	return
}

// Delete deletes the passed inventory item from the map (if it exists).
//
// This function is safe for concurrent access.
func (m *mruInventoryMap) Delete(iv *wire.InvVect) {
	m.invMtx.Lock()
	if node, exists := m.invMap[*iv]; exists {
		m.invList.Remove(node)
		delete(m.invMap, *iv)
	}
	m.invMtx.Unlock()
}

// newMruInventoryMap returns a new inventory map that is limited to the number
// of entries specified by limit.  When the number of entries exceeds the limit,
// the oldest (least recently used) entry will be removed to make room for the
// new entry.
func newMruInventoryMap(limit uint) *mruInventoryMap {
	m := mruInventoryMap{
		invMap:  make(map[wire.InvVect]*list.Element),
		invList: list.New(),
		limit:   limit,
	}
	return &m
}
