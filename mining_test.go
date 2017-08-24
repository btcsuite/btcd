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

// TestStakeTxFeePrioHeap tests the priority heaps including the stake types for
// both transaction fees per KB and transaction priority. It ensures that the
// primary sorting is first by stake type, and then by the latter chosen priority
// type.
func TestStakeTxFeePrioHeap(t *testing.T) {
	numElements := 1000
	numEdgeConditionElements := 12
	// Create some fake priority items that exercise the expected sort
	// edge conditions.
	testItems := []*txPrioItem{
		{feePerKB: 5678, txType: stake.TxTypeRegular, priority: 3},
		{feePerKB: 5678, txType: stake.TxTypeRegular, priority: 1},
		{feePerKB: 5678, txType: stake.TxTypeRegular, priority: 1}, // Duplicate fee and prio
		{feePerKB: 5678, txType: stake.TxTypeRegular, priority: 5},
		{feePerKB: 5678, txType: stake.TxTypeRegular, priority: 2},
		{feePerKB: 1234, txType: stake.TxTypeRegular, priority: 3},
		{feePerKB: 1234, txType: stake.TxTypeRegular, priority: 1},
		{feePerKB: 1234, txType: stake.TxTypeRegular, priority: 5},
		{feePerKB: 1234, txType: stake.TxTypeRegular, priority: 5}, // Duplicate fee and prio
		{feePerKB: 1234, txType: stake.TxTypeRegular, priority: 2},
		{feePerKB: 10000, txType: stake.TxTypeRegular, priority: 0}, // Higher fee, smaller prio
		{feePerKB: 0, txType: stake.TxTypeRegular, priority: 10000}, // Higher prio, lower fee
	}
	ph := newTxPriorityQueue((numElements + numEdgeConditionElements), txPQByStakeAndFee)

	// Add random data in addition to the edge conditions already manually
	// specified.
	for i := 0; i < (numElements + numEdgeConditionElements); i++ {
		if i >= numEdgeConditionElements {
			randType := stake.TxType(rand.Intn(4))
			randPrio := rand.Float64() * 100
			randFeePerKB := rand.Float64() * 10
			testItems = append(testItems, &txPrioItem{
				tx:       nil,
				txType:   randType,
				feePerKB: randFeePerKB,
				priority: randPrio,
			})
		}

		heap.Push(ph, testItems[i])
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

	ph = newTxPriorityQueue(len(testItems), txPQByStakeAndFeeAndThenPriority)
	for i := 0; i < numElements; i++ {
		randType := stake.TxType(rand.Intn(4))
		randPrio := rand.Float64() * 100
		randFeePerKB := rand.Float64() * 10
		prioItem := &txPrioItem{
			tx:       nil,
			txType:   randType,
			feePerKB: randFeePerKB,
			priority: randPrio,
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
