// Copyright (c) 2023 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
package blockchain

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/wire"
)

func TestMapSlice(t *testing.T) {
	tests := []struct {
		keys []wire.OutPoint
	}{
		{
			keys: func() []wire.OutPoint {
				outPoints := make([]wire.OutPoint, 1000)
				for i := uint32(0); i < uint32(len(outPoints)); i++ {
					var buf [4]byte
					binary.BigEndian.PutUint32(buf[:], i)
					hash := sha256.Sum256(buf[:])

					op := wire.OutPoint{Hash: hash, Index: i}
					outPoints[i] = op
				}
				return outPoints
			}(),
		},
	}

	for _, test := range tests {
		m := make(map[wire.OutPoint]*UtxoEntry)

		maxSize := calculateRoughMapSize(1000, bucketSize)

		maxEntriesFirstMap := 500
		ms1 := make(map[wire.OutPoint]*UtxoEntry, maxEntriesFirstMap)
		ms := mapSlice{
			maps:                []map[wire.OutPoint]*UtxoEntry{ms1},
			maxEntries:          []int{maxEntriesFirstMap},
			maxTotalMemoryUsage: uint64(maxSize),
		}

		for _, key := range test.keys {
			m[key] = nil
			ms.put(key, nil, 0)
		}

		// Put in the same elements twice to test that the map slice won't hold duplicates.
		for _, key := range test.keys {
			m[key] = nil
			ms.put(key, nil, 0)
		}

		if len(m) != ms.length() {
			t.Fatalf("expected len of %d, got %d", len(m), ms.length())
		}

		for _, key := range test.keys {
			expected, found := m[key]
			if !found {
				t.Fatalf("expected key %s to exist in the go map", key.String())
			}

			got, found := ms.get(key)
			if !found {
				t.Fatalf("expected key %s to exist in the map slice", key.String())
			}

			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("expected value of %v, got %v", expected, got)
			}
		}
	}
}

// TestMapsliceConcurrency just tests that the mapslice won't result in a panic
// on concurrent access.
func TestMapsliceConcurrency(t *testing.T) {
	tests := []struct {
		keys []wire.OutPoint
	}{
		{
			keys: func() []wire.OutPoint {
				outPoints := make([]wire.OutPoint, 10000)
				for i := uint32(0); i < uint32(len(outPoints)); i++ {
					var buf [4]byte
					binary.BigEndian.PutUint32(buf[:], i)
					hash := sha256.Sum256(buf[:])

					op := wire.OutPoint{Hash: hash, Index: i}
					outPoints[i] = op
				}
				return outPoints
			}(),
		},
	}

	for _, test := range tests {
		maxSize := calculateRoughMapSize(1000, bucketSize)

		maxEntriesFirstMap := 500
		ms1 := make(map[wire.OutPoint]*UtxoEntry, maxEntriesFirstMap)
		ms := mapSlice{
			maps:                []map[wire.OutPoint]*UtxoEntry{ms1},
			maxEntries:          []int{maxEntriesFirstMap},
			maxTotalMemoryUsage: uint64(maxSize),
		}

		var wg sync.WaitGroup

		wg.Add(1)
		go func(m *mapSlice, keys []wire.OutPoint) {
			defer wg.Done()
			for i := 0; i < 5000; i++ {
				m.put(keys[i], nil, 0)
			}
		}(&ms, test.keys)

		wg.Add(1)
		go func(m *mapSlice, keys []wire.OutPoint) {
			defer wg.Done()
			for i := 5000; i < 10000; i++ {
				m.put(keys[i], nil, 0)
			}
		}(&ms, test.keys)

		wg.Add(1)
		go func(m *mapSlice) {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				m.size()
			}
		}(&ms)

		wg.Add(1)
		go func(m *mapSlice) {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				m.length()
			}
		}(&ms)

		wg.Add(1)
		go func(m *mapSlice, keys []wire.OutPoint) {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				m.get(keys[i])
			}
		}(&ms, test.keys)

		wg.Add(1)
		go func(m *mapSlice, keys []wire.OutPoint) {
			defer wg.Done()
			for i := 0; i < 5000; i++ {
				m.delete(keys[i])
			}
		}(&ms, test.keys)

		wg.Wait()
	}
}

// getValidP2PKHScript returns a valid P2PKH script.  Useful as unspendables cannot be
// added to the cache.
func getValidP2PKHScript() []byte {
	validP2PKHScript := []byte{
		// OP_DUP
		0x76,
		// OP_HASH160
		0xa9,
		// OP_DATA_20
		0x14,
		// <20-byte pubkey hash>
		0xf0, 0x7a, 0xb8, 0xce, 0x72, 0xda, 0x4e, 0x76,
		0x0b, 0x74, 0x7d, 0x48, 0xd6, 0x65, 0xec, 0x96,
		0xad, 0xf0, 0x24, 0xf5,
		// OP_EQUALVERIFY
		0x88,
		// OP_CHECKSIG
		0xac,
	}
	return validP2PKHScript
}

// outpointFromInt generates an outpoint from an int by hashing the int and making
// the given int the index.
func outpointFromInt(i int) wire.OutPoint {
	// Boilerplate to create an outpoint.
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], uint32(i))
	hash := sha256.Sum256(buf[:])
	return wire.OutPoint{Hash: hash, Index: uint32(i)}
}

func TestUtxoCacheEntrySize(t *testing.T) {
	type block struct {
		txOuts []*wire.TxOut
		outOps []wire.OutPoint
		txIns  []*wire.TxIn
	}
	tests := []struct {
		name         string
		blocks       []block
		expectedSize uint64
	}{
		{
			name: "one entry",
			blocks: func() []block {
				return []block{
					{
						txOuts: []*wire.TxOut{
							{Value: 10000, PkScript: getValidP2PKHScript()},
						},
						outOps: []wire.OutPoint{
							outpointFromInt(0),
						},
					},
				}
			}(),
			expectedSize: pubKeyHashLen + baseEntrySize,
		},
		{
			name: "10 entries, 4 spend",
			blocks: func() []block {
				blocks := make([]block, 0, 10)
				for i := 0; i < 10; i++ {
					op := outpointFromInt(i)

					block := block{
						txOuts: []*wire.TxOut{
							{Value: 10000, PkScript: getValidP2PKHScript()},
						},
						outOps: []wire.OutPoint{
							op,
						},
					}

					// Spend all outs in blocks less than 4.
					if i < 4 {
						block.txIns = []*wire.TxIn{
							{PreviousOutPoint: op},
						}
					}

					blocks = append(blocks, block)
				}
				return blocks
			}(),
			// Multiplied by 6 since we'll have 6 entries left.
			expectedSize: (pubKeyHashLen + baseEntrySize) * 6,
		},
		{
			name: "spend everything",
			blocks: func() []block {
				blocks := make([]block, 0, 500)
				for i := 0; i < 500; i++ {
					op := outpointFromInt(i)

					block := block{
						txOuts: []*wire.TxOut{
							{Value: 1000, PkScript: getValidP2PKHScript()},
						},
						outOps: []wire.OutPoint{
							op,
						},
					}

					// Spend all outs in blocks less than 4.
					block.txIns = []*wire.TxIn{
						{PreviousOutPoint: op},
					}

					blocks = append(blocks, block)
				}
				return blocks
			}(),
			expectedSize: 0,
		},
	}

	for _, test := range tests {
		// Size is just something big enough so that the mapslice doesn't
		// run out of memory.
		s := newUtxoCache(nil, 1*1024*1024)

		for height, block := range test.blocks {
			for i, out := range block.txOuts {
				s.addTxOut(block.outOps[i], out, true, int32(height))
			}

			for _, in := range block.txIns {
				s.addTxIn(in, nil)
			}
		}

		if s.totalEntryMemory != test.expectedSize {
			t.Errorf("Failed test %s. Expected size of %d, got %d",
				test.name, test.expectedSize, s.totalEntryMemory)
		}
	}
}

// assertConsistencyState asserts the utxo consistency states of the blockchain.
func assertConsistencyState(chain *BlockChain, hash *chainhash.Hash) error {
	var bytes []byte
	err := chain.db.View(func(dbTx database.Tx) (err error) {
		bytes = dbFetchUtxoStateConsistency(dbTx)
		return
	})
	if err != nil {
		return fmt.Errorf("Error fetching utxo state consistency: %v", err)
	}
	actualHash, err := chainhash.NewHash(bytes)
	if err != nil {
		return err
	}
	if !actualHash.IsEqual(hash) {
		return fmt.Errorf("Unexpected consistency hash: %v instead of %v",
			actualHash, hash)
	}

	return nil
}

// assertNbEntriesOnDisk asserts that the total number of utxo entries on the
// disk is equal to the given expected number.
func assertNbEntriesOnDisk(chain *BlockChain, expectedNumber int) error {
	var nb int
	err := chain.db.View(func(dbTx database.Tx) error {
		cursor := dbTx.Metadata().Bucket(utxoSetBucketName).Cursor()
		nb = 0
		for b := cursor.First(); b; b = cursor.Next() {
			nb++
			_, err := deserializeUtxoEntry(cursor.Value())
			if err != nil {
				return fmt.Errorf("Failed to deserialize entry: %v", err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("Error fetching utxo entries: %v", err)
	}
	if nb != expectedNumber {
		return fmt.Errorf("Expected %d elements in the UTXO set, but found %d",
			expectedNumber, nb)
	}

	return nil
}

// utxoCacheTestChain creates a test BlockChain to be used for utxo cache tests.
// It uses the regression test parameters, a coin matutiry of 1 block and sets
// the cache size limit to 10 MiB.
func utxoCacheTestChain(testName string) (*BlockChain, *chaincfg.Params, func()) {
	params := chaincfg.RegressionNetParams
	chain, tearDown, err := chainSetup(testName, &params)
	if err != nil {
		panic(fmt.Sprintf("error loading blockchain with database: %v", err))
	}

	chain.TstSetCoinbaseMaturity(1)
	chain.utxoCache.maxTotalMemoryUsage = 10 * 1024 * 1024
	chain.utxoCache.cachedEntries.maxTotalMemoryUsage = chain.utxoCache.maxTotalMemoryUsage

	return chain, &params, tearDown
}

func TestUtxoCacheFlush(t *testing.T) {
	chain, params, tearDown := utxoCacheTestChain("TestUtxoCacheFlush")
	defer tearDown()
	cache := chain.utxoCache
	tip := btcutil.NewBlock(params.GenesisBlock)

	// The chainSetup init triggers the consistency status write.
	err := assertConsistencyState(chain, params.GenesisHash)
	if err != nil {
		t.Fatal(err)
	}

	err = assertNbEntriesOnDisk(chain, 0)
	if err != nil {
		t.Fatal(err)
	}

	// LastFlushHash starts with genesis.
	if cache.lastFlushHash != *params.GenesisHash {
		t.Fatalf("lastFlushHash before first flush expected to be "+
			"genesis block hash, instead was %v", cache.lastFlushHash)
	}

	// First, add 10 utxos without flushing.
	outPoints := make([]wire.OutPoint, 10)
	for i := range outPoints {
		op := outpointFromInt(i)
		outPoints[i] = op

		// Add the txout.
		txOut := wire.TxOut{Value: 10000, PkScript: getValidP2PKHScript()}
		cache.addTxOut(op, &txOut, true, int32(i))
	}

	if cache.cachedEntries.length() != len(outPoints) {
		t.Fatalf("Expected 10 entries, has %d instead", cache.cachedEntries.length())
	}

	// All entries should be fresh and modified.
	for _, m := range cache.cachedEntries.maps {
		for outpoint, entry := range m {
			if entry == nil {
				t.Fatalf("Unexpected nil entry found for %v", outpoint)
			}
			if !entry.isModified() {
				t.Fatal("Entry should be marked mofified")
			}
			if !entry.isFresh() {
				t.Fatal("Entry should be marked fresh")
			}
		}
	}

	// Spend the last outpoint and pop it off from the outpoints slice.
	var spendOp wire.OutPoint
	spendOp, outPoints = outPoints[len(outPoints)-1], outPoints[:len(outPoints)-1]
	cache.addTxIn(&wire.TxIn{PreviousOutPoint: spendOp}, nil)

	if cache.cachedEntries.length() != len(outPoints) {
		t.Fatalf("Expected %d entries, has %d instead",
			len(outPoints), cache.cachedEntries.length())
	}

	// Not flushed yet.
	err = assertConsistencyState(chain, params.GenesisHash)
	if err != nil {
		t.Fatal(err)
	}

	err = assertNbEntriesOnDisk(chain, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Flush.
	err = chain.db.Update(func(dbTx database.Tx) error {
		return cache.flush(dbTx, FlushRequired, chain.stateSnapshot)
	})
	if err != nil {
		t.Fatalf("unexpected error while flushing cache: %v", err)
	}
	if cache.cachedEntries.length() != 0 {
		t.Fatalf("Expected 0 entries, has %d instead", cache.cachedEntries.length())
	}

	err = assertConsistencyState(chain, tip.Hash())
	if err != nil {
		t.Fatal(err)
	}
	err = assertNbEntriesOnDisk(chain, len(outPoints))
	if err != nil {
		t.Fatal(err)
	}

	// Fetch the flushed utxos.
	entries, err := cache.fetchEntries(outPoints)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the returned entries are not marked fresh and modified.
	for _, entry := range entries {
		if entry.isFresh() {
			t.Fatal("Entry should not be marked fresh")
		}
		if entry.isModified() {
			t.Fatal("Entry should not be marked modified")
		}
	}

	// Check that the fetched entries in the cache are not marked fresh and modified.
	for _, m := range cache.cachedEntries.maps {
		for outpoint, elem := range m {
			if elem == nil {
				t.Fatalf("Unexpected nil entry found for %v", outpoint)
			}
			if elem.isFresh() {
				t.Fatal("Entry should not be marked fresh")
			}
			if elem.isModified() {
				t.Fatal("Entry should not be marked modified")
			}
		}
	}

	// Spend 5 utxos.
	prevLen := len(outPoints)
	for i := 0; i < 5; i++ {
		spendOp, outPoints = outPoints[len(outPoints)-1], outPoints[:len(outPoints)-1]
		cache.addTxIn(&wire.TxIn{PreviousOutPoint: spendOp}, nil)
	}

	// Should still have the entries in cache so they can be flushed to disk.
	if cache.cachedEntries.length() != prevLen {
		t.Fatalf("Expected 10 entries, has %d instead", cache.cachedEntries.length())
	}

	// Flush.
	err = chain.db.Update(func(dbTx database.Tx) error {
		return cache.flush(dbTx, FlushRequired, chain.stateSnapshot)
	})
	if err != nil {
		t.Fatalf("unexpected error while flushing cache: %v", err)
	}
	if cache.cachedEntries.length() != 0 {
		t.Fatalf("Expected 0 entries, has %d instead", cache.cachedEntries.length())
	}

	err = assertConsistencyState(chain, tip.Hash())
	if err != nil {
		t.Fatal(err)
	}
	err = assertNbEntriesOnDisk(chain, len(outPoints))
	if err != nil {
		t.Fatal(err)
	}

	// Add 5 utxos without flushing and test for periodic flushes.
	outPoints1 := make([]wire.OutPoint, 5)
	for i := range outPoints1 {
		// i + prevLen here to avoid collision since we're just hashing
		// the int.
		op := outpointFromInt(i + prevLen)
		outPoints1[i] = op

		// Add the txout.
		txOut := wire.TxOut{Value: 10000, PkScript: getValidP2PKHScript()}
		cache.addTxOut(op, &txOut, true, int32(i+prevLen))
	}
	if cache.cachedEntries.length() != len(outPoints1) {
		t.Fatalf("Expected %d entries, has %d instead",
			len(outPoints1), cache.cachedEntries.length())
	}

	// Attempt to flush with flush periodic.  Shouldn't flush.
	err = chain.db.Update(func(dbTx database.Tx) error {
		return cache.flush(dbTx, FlushPeriodic, chain.stateSnapshot)
	})
	if err != nil {
		t.Fatalf("unexpected error while flushing cache: %v", err)
	}
	if cache.cachedEntries.length() == 0 {
		t.Fatalf("Expected %d entries, has %d instead",
			len(outPoints1), cache.cachedEntries.length())
	}

	// Arbitrarily set the last flush time to 6 minutes ago.
	cache.lastFlushTime = time.Now().Add(-time.Minute * 6)

	// Attempt to flush with flush periodic.  Should flush now.
	err = chain.db.Update(func(dbTx database.Tx) error {
		return cache.flush(dbTx, FlushPeriodic, chain.stateSnapshot)
	})
	if err != nil {
		t.Fatalf("unexpected error while flushing cache: %v", err)
	}
	if cache.cachedEntries.length() != 0 {
		t.Fatalf("Expected 0 entries, has %d instead", cache.cachedEntries.length())
	}

	err = assertConsistencyState(chain, tip.Hash())
	if err != nil {
		t.Fatal(err)
	}
	err = assertNbEntriesOnDisk(chain, len(outPoints)+len(outPoints1))
	if err != nil {
		t.Fatal(err)
	}
}

func TestFlushNeededAfterPrune(t *testing.T) {
	// Construct a synthetic block chain with a block index consisting of
	// the following structure.
	// 	genesis -> 1 -> 2 -> ... -> 15 -> 16  -> 17  -> 18
	tip := tstTip
	chain := newFakeChain(&chaincfg.MainNetParams)
	chain.utxoCache = newUtxoCache(nil, 0)
	branchNodes := chainedNodes(chain.bestChain.Genesis(), 18)
	for _, node := range branchNodes {
		chain.index.SetStatusFlags(node, statusValid)
		chain.index.AddNode(node)
	}
	chain.bestChain.SetTip(tip(branchNodes))

	tests := []struct {
		name          string
		lastFlushHash chainhash.Hash
		delHashes     []chainhash.Hash
		expected      bool
	}{
		{
			name: "deleted block up to height 9, last flush hash at block 10",
			delHashes: func() []chainhash.Hash {
				delBlockHashes := make([]chainhash.Hash, 0, 9)
				for i := range branchNodes {
					if branchNodes[i].height < 10 {
						delBlockHashes = append(delBlockHashes, branchNodes[i].hash)
					}
				}

				return delBlockHashes
			}(),
			lastFlushHash: func() chainhash.Hash {
				// Just some sanity checking to make sure the height is 10.
				if branchNodes[9].height != 10 {
					panic("was looking for height 10")
				}
				return branchNodes[9].hash
			}(),
			expected: false,
		},
		{
			name: "deleted blocks up to height 10, last flush hash at block 10",
			delHashes: func() []chainhash.Hash {
				delBlockHashes := make([]chainhash.Hash, 0, 10)
				for i := range branchNodes {
					if branchNodes[i].height < 11 {
						delBlockHashes = append(delBlockHashes, branchNodes[i].hash)
					}
				}
				return delBlockHashes
			}(),
			lastFlushHash: func() chainhash.Hash {
				// Just some sanity checking to make sure the height is 10.
				if branchNodes[9].height != 10 {
					panic("was looking for height 10")
				}
				return branchNodes[9].hash
			}(),
			expected: true,
		},
		{
			name: "deleted block height 17, last flush hash at block 5",
			delHashes: func() []chainhash.Hash {
				delBlockHashes := make([]chainhash.Hash, 1)
				delBlockHashes[0] = branchNodes[16].hash
				// Just some sanity checking to make sure the height is 10.
				if branchNodes[16].height != 17 {
					panic("was looking for height 17")
				}
				return delBlockHashes
			}(),
			lastFlushHash: func() chainhash.Hash {
				// Just some sanity checking to make sure the height is 10.
				if branchNodes[4].height != 5 {
					panic("was looking for height 5")
				}
				return branchNodes[4].hash
			}(),
			expected: true,
		},
		{
			name: "deleted block height 3, last flush hash at block 4",
			delHashes: func() []chainhash.Hash {
				delBlockHashes := make([]chainhash.Hash, 1)
				delBlockHashes[0] = branchNodes[2].hash
				// Just some sanity checking to make sure the height is 10.
				if branchNodes[2].height != 3 {
					panic("was looking for height 3")
				}
				return delBlockHashes
			}(),
			lastFlushHash: func() chainhash.Hash {
				// Just some sanity checking to make sure the height is 10.
				if branchNodes[3].height != 4 {
					panic("was looking for height 4")
				}
				return branchNodes[3].hash
			}(),
			expected: false,
		},
	}

	for _, test := range tests {
		chain.utxoCache.lastFlushHash = test.lastFlushHash
		got, err := chain.flushNeededAfterPrune(test.delHashes)
		if err != nil {
			t.Fatal(err)
		}

		if got != test.expected {
			t.Fatalf("for test %s, expected need flush to return %v but got %v",
				test.name, test.expected, got)
		}
	}
}

func TestFlushOnPrune(t *testing.T) {
	chain, tearDown, err := chainSetup("TestFlushOnPrune", &chaincfg.MainNetParams)
	if err != nil {
		panic(fmt.Sprintf("error loading blockchain with database: %v", err))
	}
	defer tearDown()

	chain.utxoCache.maxTotalMemoryUsage = 10 * 1024 * 1024
	chain.utxoCache.cachedEntries.maxTotalMemoryUsage = chain.utxoCache.maxTotalMemoryUsage

	// Set the maxBlockFileSize and the prune target small so that we can trigger a
	// prune to happen.
	maxBlockFileSize := uint32(8192)
	chain.pruneTarget = uint64(maxBlockFileSize) * 2

	// Read blocks from the file.
	blocks, err := loadBlocks("blk_0_to_14131.dat")
	if err != nil {
		t.Fatalf("failed to read block from file. %v", err)
	}

	syncBlocks := func() {
		for i, block := range blocks {
			if i == 0 {
				// Skip the genesis block.
				continue
			}
			isMainChain, _, err := chain.ProcessBlock(block, BFNone)
			if err != nil {
				t.Fatal(err)
			}

			if !isMainChain {
				t.Fatalf("expected block %s to be on the main chain", block.Hash())
			}
		}
	}

	// Sync the chain.
	ffldb.TstRunWithMaxBlockFileSize(chain.db, maxBlockFileSize, syncBlocks)

	// Function that errors out if the block that should exist doesn't exist.
	shouldExist := func(dbTx database.Tx, blockHash *chainhash.Hash) {
		bytes, err := dbTx.FetchBlock(blockHash)
		if err != nil {
			t.Fatal(err)
		}
		block, err := btcutil.NewBlockFromBytes(bytes)
		if err != nil {
			t.Fatalf("didn't find block %v. %v", blockHash, err)
		}

		if !block.Hash().IsEqual(blockHash) {
			t.Fatalf("expected to find block %v but got %v",
				blockHash, block.Hash())
		}
	}

	// Function that errors out if the block that shouldn't exist exists.
	shouldNotExist := func(dbTx database.Tx, blockHash *chainhash.Hash) {
		bytes, err := dbTx.FetchBlock(chaincfg.MainNetParams.GenesisHash)
		if err == nil {
			t.Fatalf("expected block %s to be pruned", blockHash)
		}
		if len(bytes) != 0 {
			t.Fatalf("expected block %s to be pruned but got %v",
				blockHash, bytes)
		}
	}

	// The below code checks that the correct blocks were pruned.
	chain.db.View(func(dbTx database.Tx) error {
		exist := false
		for _, block := range blocks {
			// Blocks up to the last flush hash should not exist.
			// The utxocache is big enough so that it shouldn't flush
			// on it being full.  It should only flush on prunes.
			if block.Hash().IsEqual(&chain.utxoCache.lastFlushHash) {
				exist = true
			}

			if exist {
				shouldExist(dbTx, block.Hash())
			} else {
				shouldNotExist(dbTx, block.Hash())
			}

		}

		return nil
	})
}

func TestInitConsistentState(t *testing.T) {
	//  Boilerplate for creating a chain.
	dbName := "TestFlushOnPrune"
	chain, tearDown, err := chainSetup(dbName, &chaincfg.MainNetParams)
	if err != nil {
		panic(fmt.Sprintf("error loading blockchain with database: %v", err))
	}
	defer tearDown()
	chain.utxoCache.maxTotalMemoryUsage = 10 * 1024 * 1024
	chain.utxoCache.cachedEntries.maxTotalMemoryUsage = chain.utxoCache.maxTotalMemoryUsage

	// Read blocks from the file.
	blocks, err := loadBlocks("blk_0_to_14131.dat")
	if err != nil {
		t.Fatalf("failed to read block from file. %v", err)
	}

	// Sync up to height 13,000.  Flush the utxocache at height 11_000.
	cacheFlushHeight := 9000
	initialSyncHeight := 12_000
	for i, block := range blocks {
		if i == 0 {
			// Skip the genesis block.
			continue
		}

		isMainChain, _, err := chain.ProcessBlock(block, BFNone)
		if err != nil {
			t.Fatal(err)
		}

		if !isMainChain {
			t.Fatalf("expected block %s to be on the main chain", block.Hash())
		}

		if i == cacheFlushHeight {
			err = chain.FlushUtxoCache(FlushRequired)
			if err != nil {
				t.Fatal(err)
			}
		}
		if i == initialSyncHeight {
			break
		}
	}

	// Sanity check.
	if chain.BestSnapshot().Height != int32(initialSyncHeight) {
		t.Fatalf("expected the chain to sync up to height %d", initialSyncHeight)
	}

	// Close the database without flushing the utxocache.  This leaves the
	// chaintip at height 13,000 but the utxocache consistent state at 11,000.
	err = chain.db.Close()
	if err != nil {
		t.Fatal(err)
	}
	chain.db = nil

	// Re-open the database and pass the re-opened db to internal structs.
	dbPath := filepath.Join(testDbRoot, dbName)
	ndb, err := database.Open(testDbType, dbPath, blockDataNet)
	if err != nil {
		t.Fatal(err)
	}
	chain.db = ndb
	chain.utxoCache.db = ndb
	chain.index.db = ndb

	// Sanity check to see that the utxo cache was flushed before the
	// current chain tip.
	var statusBytes []byte
	ndb.View(func(dbTx database.Tx) error {
		statusBytes = dbFetchUtxoStateConsistency(dbTx)
		return nil
	})
	statusHash, err := chainhash.NewHash(statusBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !statusHash.IsEqual(blocks[cacheFlushHeight].Hash()) {
		t.Fatalf("expected the utxocache to be flushed at "+
			"block hash %s but got %s",
			blocks[cacheFlushHeight].Hash(), statusHash)
	}

	// Call InitConsistentState.  This will make the utxocache catch back
	// up to the tip.
	err = chain.InitConsistentState(chain.bestChain.tip(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Sync the reset of the blocks.
	for i, block := range blocks {
		if i <= initialSyncHeight {
			continue
		}
		isMainChain, _, err := chain.ProcessBlock(block, BFNone)
		if err != nil {
			t.Fatal(err)
		}

		if !isMainChain {
			t.Fatalf("expected block %s to be on the main chain", block.Hash())
		}
	}

	if chain.BestSnapshot().Height != blocks[len(blocks)-1].Height() {
		t.Fatalf("expected the chain to sync up to height %d",
			blocks[len(blocks)-1].Height())
	}
}
