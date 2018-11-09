// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"sync"
)

// LeakyAsset is a handler for disposable assets like external processes and
// temporary directories.
type LeakyAsset interface {
	Dispose()
}

// LeakyAssetsList keeps track of leaky assets
// to ensure their proper disposal before test framework exit.
//
// LeakyAssetsList implements a stack of leaky assets.
// Ideally assets suppose to be disposed in reverse order
// to avoid conflicts. Stack helps with that.
// Structure: [head>=(0)=(1)=.....=(n-1)=<tail]
// The nodesMap directs given asset to corresponding node for fast search
type LeakyAssetsList struct {
	size     int
	head     node
	tail     node
	nodesMap map[LeakyAsset]*node
}

// leaksList keeps track of all leaky assets and resources
// created by test setup execution.
//
var leaksList = setupList()
var registryAccessController sync.RWMutex

func setupList() *LeakyAssetsList {
	l := &LeakyAssetsList{}

	l.tail.previous = &l.head
	l.head.next = &l.tail

	l.size = 0
	l.nodesMap = make(map[LeakyAsset]*node)
	return l
}

// node is a double-linked list storing leaky asset
type node struct {
	next     *node
	previous *node
	asset    LeakyAsset
}

// String returns string representation of a node
// is used for debug purposes
func (n *node) String() string {
	return fmt.Sprintf("(%v)", n.asset)
}

// Size of the leaky assets list
func (list *LeakyAssetsList) Size() int {
	return list.size
}

// Remove element from the list
func (list *LeakyAssetsList) Remove(resource LeakyAsset) {
	if resource == nil {
		return
	}
	toRemove := list.nodesMap[resource]
	if toRemove == nil {
		return
	}

	delete(list.nodesMap, resource)

	toRemove.next.previous = toRemove.previous
	toRemove.previous.next = toRemove.next

	list.size--
}

// Contains returns true if element is present in the list
func (list *LeakyAssetsList) Contains(resource LeakyAsset) bool {
	if resource == nil {
		return false
	}
	return list.nodesMap[resource] != nil
}

// Add element to the list
func (list *LeakyAssetsList) Add(resource LeakyAsset) {
	if resource == nil {
		return
	}

	newNode := &node{asset: resource}

	list.nodesMap[resource] = newNode

	last := list.tail.previous

	last.next = newNode
	newNode.next = &list.tail

	newNode.previous = last
	list.tail.previous = newNode

	list.size++
}

// RegisterDisposableAsset registers disposable asset
// Does not tolerate multiple appends of the same element
func RegisterDisposableAsset(resource LeakyAsset) {
	var err error
	registryAccessController.Lock()
	if leaksList.Contains(resource) {
		err = fmt.Errorf("LeakyAsset is already registered: %v ",
			resource,
		)
	} else {
		leaksList.Add(resource)
	}
	registryAccessController.Unlock()

	CheckTestSetupMalfunction(err)
}

// DeRegisterDisposableAsset removes disposable asset from list
// Does not tolerate multiple removals of the same element
func DeRegisterDisposableAsset(resource LeakyAsset) {
	var err error
	registryAccessController.Lock()
	if !leaksList.Contains(resource) {
		err = fmt.Errorf("LeakyAsset is not registered: %v ",
			resource,
		)
	} else {
		leaksList.Remove(resource)
	}
	registryAccessController.Unlock()

	CheckTestSetupMalfunction(err)
}

// VerifyNoAssetsLeaked checks all leaky assets were properly disposed.
// Crashes if not.
// Should be called before test setup exit.
func VerifyNoAssetsLeaked() {
	var err error
	registryAccessController.Lock()
	if leaksList.Size() != 0 {
		err = fmt.Errorf(
			"incorrect state: resources leak detected: %v ",
			leaksList,
		)
	}
	registryAccessController.Unlock()

	CheckTestSetupMalfunction(err)
}

// forceDisposeLeakyAssets attempts to dispose all leaky assets in case of
// test setup malfunction
func forceDisposeLeakyAssets() {
	disposeList := make([]LeakyAsset, 0)

	registryAccessController.Lock()
	{
		l := leaksList
		// dispose assets in reverse order
		for current := l.tail.previous; current != &l.head; current = current.previous {
			disposeList = append(disposeList, current.asset)

		}
		l.tail.previous = &l.head
		l.head.next = &l.tail
		l.size = 0
		l.nodesMap = make(map[LeakyAsset]*node)
	}
	registryAccessController.Unlock()

	for _, asset := range disposeList {
		asset.Dispose()
	}
}
