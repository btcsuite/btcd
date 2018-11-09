// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"sync"
)

// Pool offers management for reusable resources.
type Pool struct {
	registryAccessController sync.RWMutex

	// poolSpawner creates and disposes new instances upon request
	poolSpawner Spawner

	// poolCache stores reusable instances for the following requests
	poolCache map[string]Spawnable
}

// NewPool produces a new Pool instance
func NewPool(spawner Spawner) *Pool {
	return &Pool{
		poolCache:   make(map[string]Spawnable),
		poolSpawner: spawner,
	}
}

// Spawnable wraps reusable asset
type Spawnable interface {
}

// Spawner manages a new Spawnable instance creation and disposal
type Spawner interface {
	// NewInstance returns a new freshly created Spawnable instance
	NewInstance(spawnableName string) Spawnable

	// NameForTag defines a policy for mapping input tags to Spawnable names
	// for the poolCache
	NameForTag(tag string) string

	// Dispose should take care of Spawnable instance disposal
	Dispose(spawnableToDispose Spawnable) error
}

// ObtainSpawnable returns reusable Spawnable instance upon request,
// creates a new instance when required and stores it in the poolCache
// for the following calls
func (pool *Pool) ObtainSpawnable(tag string) Spawnable {
	// Resolve Spawnable name for the tag requested
	// Pool uses SpawnableName as a key to poolCache a Spawnable instance
	spawnableName := pool.poolSpawner.NameForTag(tag)

	spawnable := pool.poolCache[spawnableName]

	// Create and poolCache a new instance when not present in poolCache
	if spawnable == nil {
		spawnable = pool.poolSpawner.NewInstance(spawnableName)
		pool.poolCache[spawnableName] = spawnable
	}

	return spawnable
}

// ObtainSpawnableConcurrentSafe is the ObtainSpawnable but
// safe for concurrent access.
func (pool *Pool) ObtainSpawnableConcurrentSafe(tag string) Spawnable {
	pool.registryAccessController.Lock()
	defer pool.registryAccessController.Unlock()
	return pool.ObtainSpawnable(tag)
}

// DisposeAll disposes all instances in the poolCache
func (pool *Pool) DisposeAll() {
	for key, spawnable := range pool.poolCache {
		err := pool.poolSpawner.Dispose(spawnable)
		delete(pool.poolCache, key)
		if err != nil {
			fmt.Printf("Failed to dispose Spawnable <%v>: %v", key, err)
		}
	}
}

// InitTags ensures the poolCache will
// immediately resolve tags from the given list
// for the future calls.
func (pool *Pool) InitTags(tags []string) {
	for _, tag := range tags {
		pool.ObtainSpawnable(tag)
	}
}

// Size returns current number if instances stored in the pool
func (pool *Pool) Size() int {
	return len(pool.poolCache)
}
