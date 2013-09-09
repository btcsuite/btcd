// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/conformal/btcwire"
	"time"
)

// MruInventoryMap provides a map that is limited to a maximum number of items
// with eviction for the oldest entry when the limit is exceeded.
type MruInventoryMap struct {
	invMap map[btcwire.InvVect]int64 // Use int64 for time for less mem.
	limit  uint
}

// String returns the map as a human-readable string.
func (m MruInventoryMap) String() string {
	return fmt.Sprintf("<%d>%v", m.limit, m.invMap)
}

// Exists returns whether or not the passed inventory item is in the map.
func (m *MruInventoryMap) Exists(iv *btcwire.InvVect) bool {
	if _, exists := m.invMap[*iv]; exists {
		return true
	}
	return false
}

// Add adds the passed inventory to the map and handles eviction of the oldest
// item if adding the new item would exceed the max limit.
func (m *MruInventoryMap) Add(iv *btcwire.InvVect) {
	// When the limit is zero, nothing can be added to the map, so just
	// return
	if m.limit == 0 {
		return
	}

	// When the entry already exists update its last seen time.
	if m.Exists(iv) {
		m.invMap[*iv] = time.Now().Unix()
		return
	}

	// Evict the oldest entry if the the new entry would exceed the size
	// limit for the map.
	if uint(len(m.invMap))+1 > m.limit {
		var oldestEntry btcwire.InvVect
		var oldestTime int64
		for iv, lastUpdated := range m.invMap {
			if oldestTime == 0 || lastUpdated < oldestTime {
				oldestEntry = iv
				oldestTime = lastUpdated
			}
		}

		m.Delete(&oldestEntry)
	}

	m.invMap[*iv] = time.Now().Unix()
	return
}

// Delete deletes the passed inventory item from the map (if it exists).
func (m *MruInventoryMap) Delete(iv *btcwire.InvVect) {
	delete(m.invMap, *iv)
}

// NewMruInventoryMap returns a new inventory map that is limited to the number
// of entries specified by limit.  When the number of entries exceeds the limit,
// the oldest (least recently used) entry will be removed to make room for the
// new entry..
func NewMruInventoryMap(limit uint) *MruInventoryMap {
	m := MruInventoryMap{
		invMap: make(map[btcwire.InvVect]int64),
		limit:  limit,
	}
	return &m
}
