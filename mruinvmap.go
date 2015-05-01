// Copyright (c) 2013-2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/list"
	"fmt"

	"github.com/btcsuite/btcd/wire"
)

// MruInventoryMap provides a map that is limited to a maximum number of items
// with eviction for the oldest entry when the limit is exceeded.
type MruInventoryMap struct {
	invMap  map[wire.InvVect]*list.Element // nearly O(1) lookups
	invList *list.List                     // O(1) insert, update, delete
	limit   uint
}

// String returns the map as a human-readable string.
func (m MruInventoryMap) String() string {
	return fmt.Sprintf("<%d>%v", m.limit, m.invMap)
}

// Exists returns whether or not the passed inventory item is in the map.
func (m *MruInventoryMap) Exists(iv *wire.InvVect) bool {
	if _, exists := m.invMap[*iv]; exists {
		return true
	}
	return false
}

// Add adds the passed inventory to the map and handles eviction of the oldest
// item if adding the new item would exceed the max limit.
func (m *MruInventoryMap) Add(iv *wire.InvVect) {
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
		lru, ok := node.Value.(*wire.InvVect)
		if !ok {
			return
		}

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
func (m *MruInventoryMap) Delete(iv *wire.InvVect) {
	if node, exists := m.invMap[*iv]; exists {
		m.invList.Remove(node)
		delete(m.invMap, *iv)
	}
}

// NewMruInventoryMap returns a new inventory map that is limited to the number
// of entries specified by limit.  When the number of entries exceeds the limit,
// the oldest (least recently used) entry will be removed to make room for the
// new entry.
func NewMruInventoryMap(limit uint) *MruInventoryMap {
	m := MruInventoryMap{
		invMap:  make(map[wire.InvVect]*list.Element),
		invList: list.New(),
		limit:   limit,
	}
	return &m
}
