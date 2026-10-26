// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btclog"
	"github.com/stretchr/testify/require"
)

// TestAddrIndexConstructors ensures only the data directory constructor
// enables fast builds.
func TestAddrIndexConstructors(t *testing.T) {
	t.Parallel()

	// 1. Check that FastBuild does nothing without a data directory.
	incremental := NewAddrIndex(nil, &chaincfg.MainNetParams)
	require.Empty(t, incremental.dataDir, "incremental data directory")
	require.NoError(t, incremental.FastBuild(nil, nil),
		"incremental FastBuild")

	// 2. Check that NewAddrIndexWithDataDir keeps the data directory.
	const dataDir = "test-data-dir"
	withDataDir := NewAddrIndexWithDataDir(
		nil, &chaincfg.MainNetParams, dataDir,
	)
	require.Equal(t, dataDir, withDataDir.dataDir, "data directory")
}

// failAddrViewDB wraps a database and fails every call to View with err.
type failAddrViewDB struct {
	database.DB
	err error
}

// View returns err without running the view function.
//
// This is part of the database.DB interface.
func (db *failAddrViewDB) View(func(database.Tx) error) error {
	return db.err
}

// TestAddrIndexFastBuildFailureLog ensures a failed fast build directs the
// user to drop and retry the index while an expected interrupt remains quiet.
func TestAddrIndexFastBuildFailureLog(t *testing.T) {
	// Capture the package log so subtests can check it.
	var logBuf bytes.Buffer
	oldLog := log
	UseLogger(btclog.NewBackend(&logBuf).Logger("TEST"))
	t.Cleanup(func() {
		log = oldLog
	})

	tests := []struct {
		name       string
		err        error
		wantAdvice bool
	}{
		{
			name:       "failure",
			err:        errors.New("test failure"),
			wantAdvice: true,
		},
		{
			// It is wrapped, so FastBuild must use errors.Is.
			name: "interrupt",
			err:  fmt.Errorf("wrapped: %w", errInterruptRequested),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logBuf.Reset()

			// 1. Fail the index tip read, before the chain is used.
			db := &failAddrViewDB{err: test.err}
			idx := NewAddrIndexWithDataDir(
				db, &chaincfg.MainNetParams, "test-data-dir",
			)
			err := idx.FastBuild(nil, nil)
			require.ErrorIs(t, err, test.err, "FastBuild")

			// 2. Check that only failures log an error and advice.
			logged := logBuf.String()
			gotFailure := strings.Contains(logged,
				"Address index fast build failed")
			gotAdvice := strings.Contains(logged,
				"--dropaddrindex") &&
				strings.Contains(logged, "--addrindex")
			require.Equal(t, test.wantAdvice, gotFailure,
				"failure logged: %q", logged)
			require.Equal(t, test.wantAdvice, gotAdvice,
				"advice logged: %q", logged)
		})
	}
}

// countConnectedAddrIndex is an address index that counts the blocks connected
// to it.
type countConnectedAddrIndex struct {
	*AddrIndex
	connected int
}

// ConnectBlock counts the block and passes the call to the wrapped address
// index.
//
// This is part of the Indexer interface.
func (idx *countConnectedAddrIndex) ConnectBlock(dbTx database.Tx,
	block *btcutil.Block, stxos []blockchain.SpentTxOut) error {

	idx.connected++
	return idx.AddrIndex.ConnectBlock(dbTx, block, stxos)
}

// TestAddrIndexFastBuildEndToEnd ensures an address index newly enabled on an
// existing chain uses the fast-build path, persists its tip, indexes an
// address, and removes its staging after the build completes.
func TestAddrIndexFastBuildEndToEnd(t *testing.T) {
	const target = 170

	// 1. Make a chain through block 170, the first to spend an output.
	c := setupAddrIndexTestChain(t, target)

	// 2. Make a tx index and a block-counting address index.
	params := &chaincfg.MainNetParams
	idx := NewAddrIndexWithDataDir(c.db, params, c.rootDir)
	counted := &countConnectedAddrIndex{AddrIndex: idx}
	manager := NewManager(c.db, []Indexer{NewTxIndex(c.db), counted})

	// 3. Reopen the chain, so blockchain.New fast builds the address index.
	chain, err := blockchain.New(&blockchain.Config{
		DB:           c.db,
		ChainParams:  params,
		TimeSource:   blockchain.NewMedianTime(),
		SigCache:     txscript.NewSigCache(1000),
		IndexManager: manager,
	})
	require.NoError(t, err, "blockchain.New")

	// 4. Check that it was built without connecting blocks.
	require.Zero(t, counted.connected,
		"blocks connected to the address index")

	// 5. Check that the address index tip is the chain tip at block 170.
	best := chain.BestSnapshot()
	require.Equal(t, int32(target), best.Height, "chain tip height")
	err = c.db.View(func(dbTx database.Tx) error {
		tipHash, tipHeight, err := dbFetchIndexerTip(dbTx, addrIndexKey)
		if err != nil {
			return err
		}
		if tipHeight != best.Height || !tipHash.IsEqual(&best.Hash) {
			return fmt.Errorf("index tip = (%d, %s), want (%d, %s)",
				tipHeight, tipHash, best.Height, best.Hash)
		}
		return nil
	})
	require.NoError(t, err, "address index tip")

	// 6. Check that the build removed its staging.
	buildDir, exists := AddrIndexFastBuildDir(c.rootDir)
	require.Falsef(t, exists, "completed build left staging at %s",
		buildDir)

	// 7. Check that block 9's coinbase address is in blocks 9 and 170.
	block9, err := chain.BlockByHeight(9)
	require.NoError(t, err, "block 9")
	pkScript := block9.MsgBlock().Transactions[0].TxOut[0].PkScript
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript, params)
	require.NoError(t, err, "extract block 9 coinbase address")
	require.Len(t, addrs, 1, "block 9 coinbase addresses")

	// Ask for three, so a third entry would show up.
	regions, skipped, err := idx.TxRegionsForAddress(
		nil, addrs[0], 0, 3, false,
	)
	require.NoError(t, err, "TxRegionsForAddress")
	require.Zero(t, skipped, "address lookup skipped")
	require.Len(t, regions, 2, "address lookup regions")
	for i, height := range []int32{9, target} {
		hash, err := chain.BlockHashByHeight(height)
		require.NoError(t, err, "block hash")
		require.Truef(t, regions[i].Hash.IsEqual(hash),
			"address region %d block = %s, want block %d (%s)",
			i, regions[i].Hash, height, hash)
	}

	// 8. Rebuild the slow way and compare.
	got := readAddrIndexTestBucket(t, c.db)
	c.resetAddrIndex(t, idx, -1, nil)
	c.connect(t, idx, 0, target)
	requireAddrBuildLevels(t, got, readAddrIndexTestBucket(t, c.db))
}
