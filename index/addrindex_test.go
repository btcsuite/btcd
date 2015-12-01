// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package index

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	database "github.com/btcsuite/btcd/database2"
	_ "github.com/btcsuite/btcd/database2/ffldb"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

func decodeHex(s string) []byte {
	r, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source")
	}
	return r
}
func decodeHash(s string) *wire.ShaHash {
	r, err := wire.NewShaHashFromStr(s)
	if err != nil {
		panic("invalid hash in source")
	}
	return r
}

type addrIndexTestCase struct {
	key           *addrKey
	numToSkip     uint32
	numRequested  uint32
	reverse       bool
	resultRegions []addrIndexResult
	resultSkipped uint32
}

var errRunTestCaseFail = errors.New("runAddrIndexTestCases failure")
var testBucketName = []byte("testaddridx")

func runAddrIndexTestCases(t *testing.T, db database.DB, tests []addrIndexTestCase) error {
	fail := true
	err := db.View(func(dbTx database.Tx) error {
		bucket := dbTx.Metadata().Bucket(testBucketName)
		for i, test := range tests {
			regions, skipped, err := dbFetchAddrIndexEntries(bucket, test.key, test.numToSkip, test.numRequested, test.reverse)
			if err != nil {
				t.Errorf("addrIndexTestCase #%d: call failed: %v\n", i, err)
				fail = true
				continue
			}
			if skipped != test.resultSkipped {
				t.Errorf("addrIndexTestCase #%d: mismatched skipped: got %d, want %d", i, skipped, test.resultSkipped)
				fail = true
			}
			if !reflect.DeepEqual(regions, test.resultRegions) {
				t.Errorf("addrIndexTestCase #%d: mismatched regions", i)
				fail = true
			}
		}
		return nil
	})
	if err != nil {
		t.Errorf("Failed to run test cases: %v\n", err)
		return err
	}
	if fail {
		return errRunTestCaseFail
	}
	return nil
}

// TestAddrIndexStorage ensures adding and removing blocks, and querying the
// addrindex works as intended.
func TestAddrIndexStorage(t *testing.T) {
	t.Parallel()

	// Create all the test data

	// Intentionally create two different types of address with
	// the same hash to check the addr index properly distinguishes them.
	addrPub, err := btcutil.NewAddressPubKeyHash(decodeHex("1cd28a85a3a2ba2a4eb07b9e654e34bf158d0073"), &chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Failed to create P2PKH address: %v\n", err)
		return
	}
	addrScript, err := btcutil.NewAddressScriptHashFromHash(decodeHex("1cd28a85a3a2ba2a4eb07b9e654e34bf158d0073"), &chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Failed to create P2SH address: %v\n", err)
		return
	}
	addrPubKey, err := addrToKey(addrPub)
	if err != nil {
		t.Errorf("Failed to create P2PKH address key: %v\n", err)
		return
	}
	addrScriptKey, err := addrToKey(addrScript)
	if err != nil {
		t.Errorf("Failed to create P2SH address key: %v\n", err)
		return
	}

	if *addrPubKey == *addrScriptKey {
		t.Errorf("Got same addr key for P2PKH and P2SH addresses with the same hash.")
		return
	}

	testBlock1 := btcutil.NewBlock(&wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:    3,
			PrevBlock:  *chaincfg.MainNetParams.GenesisHash,
			MerkleRoot: *decodeHash("df8588094308ddc4601a3abc8a5baede453e9ef1c25f12302591eeeedaf90554"),
			Timestamp:  time.Unix(1293623863, 0),
			Bits:       0x1b04864c,
			Nonce:      0x10572b0f,
		},
	})
	testBlock1.SetHeight(1)

	testData1 := writeIndexData{
		*addrPubKey: []wire.TxLoc{
			wire.TxLoc{TxStart: 10, TxLen: 1},
			wire.TxLoc{TxStart: 20, TxLen: 2},
			wire.TxLoc{TxStart: 30, TxLen: 3},
			wire.TxLoc{TxStart: 40, TxLen: 4},
			wire.TxLoc{TxStart: 50, TxLen: 5},
		},
		*addrScriptKey: []wire.TxLoc{
			wire.TxLoc{TxStart: 60, TxLen: 6},
			wire.TxLoc{TxStart: 70, TxLen: 7},
			wire.TxLoc{TxStart: 80, TxLen: 8},
		},
	}

	// Another test block. This one adds enough transactions so that the
	// total amount of transactions doesn't fit in just the first level.
	testBlock2 := btcutil.NewBlock(&wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:    3,
			PrevBlock:  *testBlock1.Sha(),
			MerkleRoot: *decodeHash("d3a8ae4a2baf65969c77e11d4b23c2a8e86c88296fad49579432b3ea219c64de"),
			Timestamp:  time.Unix(1293623863, 0),
			Bits:       0x1b04864c,
			Nonce:      0x10572b0f,
		},
	})
	testBlock2.SetHeight(2)

	testData2 := writeIndexData{
		*addrPubKey: []wire.TxLoc{
			wire.TxLoc{TxStart: 60, TxLen: 6},
			wire.TxLoc{TxStart: 70, TxLen: 7},
			wire.TxLoc{TxStart: 80, TxLen: 8},
			wire.TxLoc{TxStart: 90, TxLen: 9},
			wire.TxLoc{TxStart: 100, TxLen: 10},
		},
	}

	testCases0 := []addrIndexTestCase{
		{
			key:           addrPubKey,
			numToSkip:     0,
			numRequested:  10,
			reverse:       false,
			resultRegions: nil,
			resultSkipped: 0,
		},
		{
			key:           addrPubKey,
			numToSkip:     20,
			numRequested:  10,
			reverse:       false,
			resultRegions: nil,
			resultSkipped: 0,
		},
		{
			key:           addrPubKey,
			numToSkip:     0,
			numRequested:  10,
			reverse:       true,
			resultRegions: nil,
			resultSkipped: 0,
		},
		{
			key:           addrPubKey,
			numToSkip:     20,
			numRequested:  10,
			reverse:       true,
			resultRegions: nil,
			resultSkipped: 0,
		},
	}

	testCases1 := []addrIndexTestCase{
		{
			key:          addrPubKey,
			numToSkip:    0,
			numRequested: 10,
			reverse:      false,
			resultRegions: []addrIndexResult{
				{1, 10, 1},
				{1, 20, 2},
				{1, 30, 3},
				{1, 40, 4},
				{1, 50, 5},
			},
			resultSkipped: 0,
		},
		{
			key:          addrPubKey,
			numToSkip:    3,
			numRequested: 10,
			reverse:      false,
			resultRegions: []addrIndexResult{
				{1, 40, 4},
				{1, 50, 5},
			},
			resultSkipped: 3,
		},
		{
			key:           addrPubKey,
			numToSkip:     10,
			numRequested:  10,
			reverse:       false,
			resultRegions: nil,
			resultSkipped: 5,
		},
		{
			key:          addrPubKey,
			numToSkip:    0,
			numRequested: 10,
			reverse:      true,
			resultRegions: []addrIndexResult{
				{1, 50, 5},
				{1, 40, 4},
				{1, 30, 3},
				{1, 20, 2},
				{1, 10, 1},
			},
			resultSkipped: 0,
		},
		{
			key:          addrPubKey,
			numToSkip:    4,
			numRequested: 10,
			reverse:      true,
			resultRegions: []addrIndexResult{
				{1, 10, 1},
			},
			resultSkipped: 4,
		},
		{
			key:           addrPubKey,
			numToSkip:     10,
			numRequested:  10,
			reverse:       true,
			resultRegions: nil,
			resultSkipped: 5,
		},
	}

	testCases2 := []addrIndexTestCase{
		{
			key:          addrScriptKey,
			numToSkip:    0,
			numRequested: 50,
			reverse:      false,
			resultRegions: []addrIndexResult{
				{1, 10, 1},
				{1, 20, 2},
				{1, 30, 3},
				{1, 40, 4},
				{1, 50, 5},
				{2, 60, 6},
				{2, 70, 7},
				{2, 80, 8},
				{2, 90, 9},
				{2, 100, 10},
			},
			resultSkipped: 0,
		},
		{
			key:          addrScriptKey,
			numToSkip:    0,
			numRequested: 6,
			reverse:      false,
			resultRegions: []addrIndexResult{
				{1, 10, 1},
				{1, 20, 2},
				{1, 30, 3},
				{1, 40, 4},
				{1, 50, 5},
				{2, 60, 6},
			},
			resultSkipped: 0,
		},
		{
			key:          addrScriptKey,
			numToSkip:    3,
			numRequested: 3,
			reverse:      true,
			resultRegions: []addrIndexResult{
				{2, 70, 7},
				{2, 60, 6},
				{1, 50, 5},
			},
			resultSkipped: 3,
		},
		{
			key:          addrScriptKey,
			numToSkip:    3,
			numRequested: 3,
			reverse:      false,
			resultRegions: []addrIndexResult{
				{1, 40, 4},
				{1, 50, 5},
				{2, 60, 6},
			},
			resultSkipped: 3,
		},
		{
			key:          addrScriptKey,
			numToSkip:    0,
			numRequested: 6,
			reverse:      true,
			resultRegions: []addrIndexResult{
				{2, 100, 10},
				{2, 90, 9},
				{2, 80, 8},
				{2, 70, 7},
				{2, 60, 6},
				{1, 50, 5},
			},
			resultSkipped: 0,
		},
		{
			key:           addrPubKey,
			numToSkip:     20,
			numRequested:  20,
			reverse:       false,
			resultRegions: nil,
			resultSkipped: 10,
		},
		{
			key:           addrPubKey,
			numToSkip:     20,
			numRequested:  20,
			reverse:       true,
			resultRegions: nil,
			resultSkipped: 10,
		},
	}

	dbPath := filepath.Join(os.TempDir(), "addrindexstoragetest")
	_ = os.RemoveAll(dbPath)
	db, err := database.Create("ffldb", dbPath, chaincfg.MainNetParams.Net)
	if err != nil {
		t.Errorf("Failed to create database: %v\n", err)
		return
	}
	defer os.RemoveAll(dbPath)
	defer db.Close()

	// Check creating the addr index works fine.
	err = db.Update(func(dbTx database.Tx) error {
		dbTx.Metadata().CreateBucket(testBucketName)
		return nil
	})
	if err != nil {
		t.Errorf("Failed to create address index: %v\n", err)
		return
	}

	err = runAddrIndexTestCases(t, db, testCases0)
	if err != nil {
		return
	}

	// Add block 1
	err = db.Update(func(dbTx database.Tx) error {
		bucket := dbTx.Metadata().Bucket(testBucketName)
		return dbAppendAddrIndexDataForBlock(bucket, testBlock1, testData1)
	})
	if err != nil {
		t.Errorf("Failed to append block to index: %v\n", err)
		return
	}

	// Test block 1
	err = runAddrIndexTestCases(t, db, testCases1)
	if err != nil {
		return
	}

	// Add the 2nd block
	err = db.Update(func(dbTx database.Tx) error {
		bucket := dbTx.Metadata().Bucket(testBucketName)
		return dbAppendAddrIndexDataForBlock(bucket, testBlock2, testData2)
	})
	if err != nil {
		t.Errorf("Failed to append block to index: %v\n", err)
		return
	}

	// Test block 2
	err = runAddrIndexTestCases(t, db, testCases2)
	if err != nil {
		return
	}

	// Remove the 2nd block
	err = db.Update(func(dbTx database.Tx) error {
		bucket := dbTx.Metadata().Bucket(testBucketName)
		return dbRemoveAddrIndexDataForBlock(bucket, testBlock2, testData2)
	})
	if err != nil {
		t.Errorf("Failed to append block to index: %v\n", err)
		return
	}

	// Test block 1 again
	err = runAddrIndexTestCases(t, db, testCases1)
	if err != nil {
		return
	}

	// Remove the 1st block
	err = db.Update(func(dbTx database.Tx) error {
		bucket := dbTx.Metadata().Bucket(testBucketName)
		return dbRemoveAddrIndexDataForBlock(bucket, testBlock1, testData1)
	})
	if err != nil {
		t.Errorf("Failed to append block to index: %v\n", err)
		return
	}

	// Test empy index again
	err = runAddrIndexTestCases(t, db, testCases0)
	if err != nil {
		return
	}
}
