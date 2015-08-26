// Copyright (c) 2015-2016 The Decred Developers
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

// TestTxFeePrioHeap tests the priority heaps including the stake types for both
// transaction fees per KB and transaction priority. It ensures that the primary
// sorting is first by stake type, and then by the latter chosen priority type.
func TestTxFeePrioHeap(t *testing.T) {
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
