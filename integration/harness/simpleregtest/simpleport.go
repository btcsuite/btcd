// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package simpleregtest

import "sync"

// LazyPortManager simply generates subsequent
// integers starting from the BasePort
type LazyPortManager struct {
	// Harnesses will subsequently reserve
	// network ports starting from the BasePort value
	BasePort     int
	offset       int
	registerLock sync.RWMutex
}

// ObtainPort returns prev returned value + 1
// starting from the BasePort
func (man *LazyPortManager) ObtainPort() (result int) {
	man.registerLock.Lock()
	defer man.registerLock.Unlock()
	result = man.BasePort + man.offset
	man.offset++
	return
}
