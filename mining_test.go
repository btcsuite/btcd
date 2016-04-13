// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/heap"
	"math/rand"
	"testing"

	"github.com/btcsuite/btcutil"
)

// TestTxFeePrioHeap ensures the priority queue for transaction fees and
// priorities works as expected.
func TestTxFeePrioHeap(t *testing.T) {
	// Create priority items with random fees and priorites.
	const numItems = 1000
	prioItems := make([]*txPrioItem, numItems)
	highestFeePerKB := int64(0)
	highestPrio := float64(0)
	for i := 0; i < 1000; i++ {
		randPrio := rand.Float64() * 100
		if randPrio > highestPrio {
			highestPrio = randPrio
		}
		randFeePerKB := int64(rand.Float64() * btcutil.SatoshiPerBitcoin)
		if randFeePerKB > highestFeePerKB {
			highestFeePerKB = randFeePerKB
		}
		prioItems[i] = &txPrioItem{
			tx:       nil,
			priority: randPrio,
			feePerKB: randFeePerKB,
		}
	}

	// Test sorting by fee per KB.
	priorityQueue := newTxPriorityQueue(numItems, true)
	for i := 0; i < numItems; i++ {
		heap.Push(priorityQueue, prioItems[i])
	}
	for i := 0; i < numItems; i++ {
		prioItem := heap.Pop(priorityQueue).(*txPrioItem)
		if prioItem.feePerKB > highestFeePerKB {
			t.Fatalf("bad pop: %v fee per KB was more than last of %v",
				prioItem.feePerKB, highestFeePerKB)
		}
		highestFeePerKB = prioItem.feePerKB
	}

	// Test sorting by priority.
	priorityQueue = newTxPriorityQueue(numItems, false)
	for i := 0; i < numItems; i++ {
		heap.Push(priorityQueue, prioItems[i])
	}
	for i := 0; i < numItems; i++ {
		prioItem := heap.Pop(priorityQueue).(*txPrioItem)
		if prioItem.priority > highestPrio {
			t.Fatalf("bad pop: %v priority was more than last of %v",
				prioItem.priority, highestPrio)
		}
	}
}
