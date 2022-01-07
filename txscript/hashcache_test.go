// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"math/rand"
	"testing"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/davecgh/go-spew/spew"
)

func init() {
	rand.Seed(time.Now().Unix())
}

// genTestTx creates a random transaction for uses within test cases.
func genTestTx() (*wire.MsgTx, *MultiPrevOutFetcher, error) {
	tx := wire.NewMsgTx(2)
	tx.Version = rand.Int31()

	prevOuts := NewMultiPrevOutFetcher(nil)

	numTxins := 1 + rand.Intn(11)
	for i := 0; i < numTxins; i++ {
		randTxIn := wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Index: uint32(rand.Int31()),
			},
			Sequence: uint32(rand.Int31()),
		}
		_, err := rand.Read(randTxIn.PreviousOutPoint.Hash[:])
		if err != nil {
			return nil, nil, err
		}

		tx.TxIn = append(tx.TxIn, &randTxIn)

		prevOuts.AddPrevOut(
			randTxIn.PreviousOutPoint, &wire.TxOut{},
		)
	}

	numTxouts := 1 + rand.Intn(11)
	for i := 0; i < numTxouts; i++ {
		randTxOut := wire.TxOut{
			Value:    rand.Int63(),
			PkScript: make([]byte, rand.Intn(30)),
		}
		if _, err := rand.Read(randTxOut.PkScript); err != nil {
			return nil, nil, err
		}
		tx.TxOut = append(tx.TxOut, &randTxOut)
	}

	return tx, prevOuts, nil
}

// TestHashCacheAddContainsHashes tests that after items have been added to the
// hash cache, the ContainsHashes method returns true for all the items
// inserted.  Conversely, ContainsHashes should return false for any items
// _not_ in the hash cache.
func TestHashCacheAddContainsHashes(t *testing.T) {
	t.Parallel()

	cache := NewHashCache(10)

	var (
		err          error
		randPrevOuts *MultiPrevOutFetcher
	)
	prevOuts := NewMultiPrevOutFetcher(nil)

	// First, we'll generate 10 random transactions for use within our
	// tests.
	const numTxns = 10
	txns := make([]*wire.MsgTx, numTxns)
	for i := 0; i < numTxns; i++ {
		txns[i], randPrevOuts, err = genTestTx()
		if err != nil {
			t.Fatalf("unable to generate test tx: %v", err)
		}

		prevOuts.Merge(randPrevOuts)
	}

	// With the transactions generated, we'll add each of them to the hash
	// cache.
	for _, tx := range txns {
		cache.AddSigHashes(tx, prevOuts)
	}

	// Next, we'll ensure that each of the transactions inserted into the
	// cache are properly located by the ContainsHashes method.
	for _, tx := range txns {
		txid := tx.TxHash()
		if ok := cache.ContainsHashes(&txid); !ok {
			t.Fatalf("txid %v not found in cache but should be: ",
				txid)
		}
	}

	randTx, _, err := genTestTx()
	if err != nil {
		t.Fatalf("unable to generate tx: %v", err)
	}

	// Finally, we'll assert that a transaction that wasn't added to the
	// cache won't be reported as being present by the ContainsHashes
	// method.
	randTxid := randTx.TxHash()
	if ok := cache.ContainsHashes(&randTxid); ok {
		t.Fatalf("txid %v wasn't inserted into cache but was found",
			randTxid)
	}
}

// TestHashCacheAddGet tests that the sighashes for a particular transaction
// are properly retrieved by the GetSigHashes function.
func TestHashCacheAddGet(t *testing.T) {
	t.Parallel()

	cache := NewHashCache(10)

	// To start, we'll generate a random transaction and compute the set of
	// sighashes for the transaction.
	randTx, prevOuts, err := genTestTx()
	if err != nil {
		t.Fatalf("unable to generate tx: %v", err)
	}
	sigHashes := NewTxSigHashes(randTx, prevOuts)

	// Next, add the transaction to the hash cache.
	cache.AddSigHashes(randTx, prevOuts)

	// The transaction inserted into the cache above should be found.
	txid := randTx.TxHash()
	cacheHashes, ok := cache.GetSigHashes(&txid)
	if !ok {
		t.Fatalf("tx %v wasn't found in cache", txid)
	}

	// Finally, the sighashes retrieved should exactly match the sighash
	// originally inserted into the cache.
	if *sigHashes != *cacheHashes {
		t.Fatalf("sighashes don't match: expected %v, got %v",
			spew.Sdump(sigHashes), spew.Sdump(cacheHashes))
	}
}

// TestHashCachePurge tests that items are able to be properly removed from the
// hash cache.
func TestHashCachePurge(t *testing.T) {
	t.Parallel()

	cache := NewHashCache(10)

	var (
		err          error
		randPrevOuts *MultiPrevOutFetcher
	)
	prevOuts := NewMultiPrevOutFetcher(nil)

	// First we'll start by inserting numTxns transactions into the hash cache.
	const numTxns = 10
	txns := make([]*wire.MsgTx, numTxns)
	for i := 0; i < numTxns; i++ {
		txns[i], randPrevOuts, err = genTestTx()
		if err != nil {
			t.Fatalf("unable to generate test tx: %v", err)
		}

		prevOuts.Merge(randPrevOuts)
	}
	for _, tx := range txns {
		cache.AddSigHashes(tx, prevOuts)
	}

	// Once all the transactions have been inserted, we'll purge them from
	// the hash cache.
	for _, tx := range txns {
		txid := tx.TxHash()
		cache.PurgeSigHashes(&txid)
	}

	// At this point, none of the transactions inserted into the hash cache
	// should be found within the cache.
	for _, tx := range txns {
		txid := tx.TxHash()
		if ok := cache.ContainsHashes(&txid); ok {
			t.Fatalf("tx %v found in cache but should have "+
				"been purged: ", txid)
		}
	}
}
