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
	"github.com/decred/dcrutil"
)

// TestTxFeePrioHeap ensures the priority queue for transaction fees and
// priorities works as expected. It doesn't set anything for the stake
// priority, so this test only tests sorting by fee and then priority.
func TestTxFeePrioHeap(t *testing.T) {
	// Create some fake priority items that exercise the expected sort
	// edge conditions.
	testItems := []*txPrioItem{
		{feePerKB: 5678, priority: 1},
		{feePerKB: 5678, priority: 1}, // Duplicate fee and prio
		{feePerKB: 5678, priority: 5},
		{feePerKB: 5678, priority: 2},
		{feePerKB: 1234, priority: 3},
		{feePerKB: 1234, priority: 1},
		{feePerKB: 1234, priority: 5},
		{feePerKB: 1234, priority: 5}, // Duplicate fee and prio
		{feePerKB: 1234, priority: 2},
		{feePerKB: 10000, priority: 0}, // Higher fee, smaller prio
		{feePerKB: 0, priority: 10000}, // Higher prio, lower fee
	}
	numItems := len(testItems)

	// Add random data in addition to the edge conditions already manually
	// specified.
	randSeed := rand.Int63()
	defer func() {
		if t.Failed() {
			t.Logf("Random numbers using seed: %v", randSeed)
		}
	}()
	prng := rand.New(rand.NewSource(randSeed))
	for i := 0; i < 1000; i++ {
		testItems = append(testItems, &txPrioItem{
			feePerKB: prng.Float64() * dcrutil.AtomsPerCoin,
			priority: prng.Float64() * 100,
		})
	}

	// Test sorting by fee per KB then priority.
	var highest *txPrioItem
	priorityQueue := newTxPriorityQueue(len(testItems), txPQByFee)
	for i := 0; i < len(testItems); i++ {
		prioItem := testItems[i]
		if highest == nil {
			highest = prioItem
		}
		if prioItem.feePerKB >= highest.feePerKB {
			highest = prioItem

			if prioItem.feePerKB == highest.feePerKB {
				if prioItem.priority >= highest.priority {
					highest = prioItem
				}
			}
		}
		heap.Push(priorityQueue, prioItem)
	}

	for i := 0; i < len(testItems); i++ {
		prioItem := heap.Pop(priorityQueue).(*txPrioItem)
		feesEqual := false
		switch {
		case prioItem.feePerKB > highest.feePerKB:
			t.Fatalf("priority sort: item (fee per KB: %v, "+
				"priority: %v) higher than than prev "+
				"(fee per KB: %v, priority %v)",
				prioItem.feePerKB, prioItem.priority,
				highest.feePerKB, highest.priority)
		case prioItem.feePerKB == highest.feePerKB:
			feesEqual = true
		default:
		}
		if feesEqual {
			switch {
			case prioItem.priority > highest.priority:
				t.Fatalf("priority sort: item (fee per KB: %v, "+
					"priority: %v) higher than than prev "+
					"(fee per KB: %v, priority %v)",
					prioItem.feePerKB, prioItem.priority,
					highest.feePerKB, highest.priority)
			case prioItem.priority == highest.priority:
			default:
			}
		}

		highest = prioItem
	}

	// Test sorting by priority then fee per KB.
	highest = nil
	priorityQueue = newTxPriorityQueue(numItems, txPQByPriority)
	for i := 0; i < len(testItems); i++ {
		prioItem := testItems[i]
		if highest == nil {
			highest = prioItem
		}
		if prioItem.priority >= highest.priority {
			highest = prioItem

			if prioItem.priority == highest.priority {
				if prioItem.feePerKB >= highest.feePerKB {
					highest = prioItem
				}
			}
		}
		heap.Push(priorityQueue, prioItem)
	}

	for i := 0; i < len(testItems); i++ {
		prioItem := heap.Pop(priorityQueue).(*txPrioItem)
		prioEqual := false
		switch {
		case prioItem.priority > highest.priority:
			t.Fatalf("priority sort: item (fee per KB: %v, "+
				"priority: %v) higher than than prev "+
				"(fee per KB: %v, priority %v)",
				prioItem.feePerKB, prioItem.priority,
				highest.feePerKB, highest.priority)
		case prioItem.priority == highest.priority:
			prioEqual = true
		default:
		}
		if prioEqual {
			switch {
			case prioItem.feePerKB > highest.feePerKB:
				t.Fatalf("priority sort: item (fee per KB: %v, "+
					"priority: %v) higher than than prev "+
					"(fee per KB: %v, priority %v)",
					prioItem.feePerKB, prioItem.priority,
					highest.feePerKB, highest.priority)
			case prioItem.priority == highest.priority:
			default:
			}
		}

		highest = prioItem
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
		}
	}
}
