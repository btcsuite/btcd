// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/heap"
	"math/rand"
	"testing"

	"github.com/decred/dcrd/blockchain/stake"
)

// TestTxFeePrioHeap ensures the priority queue for transaction fees and
// priorities works as expected.
func TestTxFeePrioHeap(t *testing.T) {
	// Create priority items with random fees and priorites.
	const numItems = 1000
	prioItems := make([]*txPrioItem, numItems)
	highestFeePerKB := float64(0)
	highestPrio := float64(0)
	for i := 0; i < 1000; i++ {
		randPrio := rand.Float64() * 100
		if randPrio > highestPrio {
			highestPrio = randPrio
		}
		randFeePerKB := rand.Float64() * 1e8
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
	priorityQueue := newTxPriorityQueue(numItems, txPQByFee)
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
	priorityQueue = newTxPriorityQueue(numItems, txPQByPriority)
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

// TestStakeTxFeePrioHeap tests the priority heaps including the stake types for
// both transaction fees per KB and transaction priority. It ensures that the
// primary sorting is first by stake type, and then by the latter chosen priority
// type.
func TestStakeTxFeePrioHeap(t *testing.T) {
	numElements := 1000
	ph := newTxPriorityQueue(numElements, txPQByStakeAndFee)

	for i := 0; i < numElements; i++ {
		randType := stake.TxType(rand.Intn(4))
		randPrio := rand.Float64() * 100
		randFeePerKB := rand.Float64() * 10
		prioItem := &txPrioItem{
			tx:       nil,
			txType:   randType,
			priority: randPrio,
			feePerKB: randFeePerKB,
		}
		heap.Push(ph, prioItem)
	}

	// Test sorting by stake and fee per KB.
	last := &txPrioItem{
		tx:       nil,
		txType:   stake.TxTypeSSGen,
		priority: 10000.0,
		feePerKB: 10000.0,
	}
	for i := 0; i < numElements; i++ {
		prioItem := heap.Pop(ph)
		txpi, ok := prioItem.(*txPrioItem)
		if ok {
			if txpi.feePerKB > last.feePerKB &&
				compareStakePriority(txpi, last) >= 0 {
				t.Errorf("bad pop: %v fee per KB was more than last of %v "+
					"while the txtype was %v but last was %v",
					txpi.feePerKB, last.feePerKB, txpi.txType, last.txType)
			}
			last = txpi
		} else {
			t.Fatalf("casting failure")
		}
	}

	ph = newTxPriorityQueue(numElements, txPQByStakeAndPriority)
	for i := 0; i < numElements; i++ {
		randType := stake.TxType(rand.Intn(4))
		randPrio := rand.Float64() * 100
		randFeePerKB := rand.Float64() * 10
		prioItem := &txPrioItem{
			tx:       nil,
			txType:   randType,
			priority: randPrio,
			feePerKB: randFeePerKB,
		}
		heap.Push(ph, prioItem)
	}

	// Test sorting by stake and priority.
	last = &txPrioItem{
		tx:       nil,
		txType:   stake.TxTypeSSGen,
		priority: 10000.0,
		feePerKB: 10000.0,
	}
	for i := 0; i < numElements; i++ {
		prioItem := heap.Pop(ph)
		txpi, ok := prioItem.(*txPrioItem)
		if ok {
			if txpi.priority > last.priority &&
				compareStakePriority(txpi, last) >= 0 {
				t.Errorf("bad pop: %v fee per KB was more than last of %v "+
					"while the txtype was %v but last was %v",
					txpi.feePerKB, last.feePerKB, txpi.txType, last.txType)
			}
			last = txpi
		} else {
			t.Fatalf("casting failure")
		}
	}

	ph = newTxPriorityQueue(numElements, txPQByStakeAndFeeAndThenPriority)
	for i := 0; i < numElements; i++ {
		randType := stake.TxType(rand.Intn(4))
		randPrio := rand.Float64() * 100
		randFeePerKB := rand.Float64() * 10
		prioItem := &txPrioItem{
			tx:       nil,
			txType:   randType,
			priority: randPrio,
			feePerKB: randFeePerKB,
		}
		heap.Push(ph, prioItem)
	}

	// Test sorting with fees per KB for high stake priority, then
	// priority for low stake priority.
	last = &txPrioItem{
		tx:       nil,
		txType:   stake.TxTypeSSGen,
		priority: 10000.0,
		feePerKB: 10000.0,
	}
	for i := 0; i < numElements; i++ {
		prioItem := heap.Pop(ph)
		txpi, ok := prioItem.(*txPrioItem)
		if ok {
			bothAreLowStakePriority :=
				txStakePriority(txpi.txType) == regOrRevocPriority &&
					txStakePriority(last.txType) == regOrRevocPriority
			if !bothAreLowStakePriority {
				if txpi.feePerKB > last.feePerKB &&
					compareStakePriority(txpi, last) >= 0 {
					t.Errorf("bad pop: %v fee per KB was more than last of %v "+
						"while the txtype was %v but last was %v",
						txpi.feePerKB, last.feePerKB, txpi.txType, last.txType)
				}
			}
			if bothAreLowStakePriority {
				if txpi.priority > last.priority &&
					compareStakePriority(txpi, last) >= 0 {
					t.Errorf("bad pop: %v priority was more than last of %v "+
						"while the txtype was %v but last was %v",
						txpi.feePerKB, last.feePerKB, txpi.txType, last.txType)
				}
			}
			last = txpi
		} else {
			t.Fatalf("casting failure")
		}
	}
}
