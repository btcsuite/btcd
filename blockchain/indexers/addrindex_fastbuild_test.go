// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/btcsuite/btclog"
	"github.com/stretchr/testify/require"
)

// TestAddrIndexConstructors ensures only the data directory constructor
// enables fast builds.
func TestAddrIndexConstructors(t *testing.T) {
	t.Parallel()

	incremental := NewAddrIndex(nil, &chaincfg.MainNetParams)
	require.Empty(t, incremental.dataDir, "incremental data directory")
	require.NoError(t, incremental.FastBuild(nil, nil),
		"incremental FastBuild")

	const dataDir = "test-data-dir"
	withDataDir := NewAddrIndexWithDataDir(
		nil, &chaincfg.MainNetParams, dataDir,
	)
	require.Equal(t, dataDir, withDataDir.dataDir, "data directory")
}

type failAddrViewDB struct {
	database.DB
	err error
}

func (db *failAddrViewDB) View(func(database.Tx) error) error {
	return db.err
}

// TestAddrIndexFastBuildFailureLog ensures a failed fast build directs the
// user to drop and retry the index while an expected interrupt remains quiet.
func TestAddrIndexFastBuildFailureLog(t *testing.T) {
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
			name: "interrupt",
			err:  fmt.Errorf("wrapped: %w", errInterruptRequested),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logBuf.Reset()
			db := &failAddrViewDB{err: test.err}
			idx := NewAddrIndexWithDataDir(
				db, &chaincfg.MainNetParams, "test-data-dir",
			)
			err := idx.FastBuild(nil, nil)
			require.ErrorIs(t, err, test.err, "FastBuild")

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

// TestAddrIndexFastBuildEndToEnd ensures a newly enabled address index uses
// the fast-build path, persists its tip, indexes an address, and removes its
// staging after the build completes.
func TestAddrIndexFastBuildEndToEnd(t *testing.T) {
	rootDir := t.TempDir()
	db, err := database.Create("ffldb", filepath.Join(rootDir, "db"),
		wire.MainNet)
	require.NoError(t, err, "database.Create")
	defer db.Close()

	params := &chaincfg.MainNetParams
	idx := NewAddrIndexWithDataDir(db, params, rootDir)
	manager := NewManager(db, []Indexer{NewTxIndex(db), idx})
	chain, err := blockchain.New(&blockchain.Config{
		DB:           db,
		ChainParams:  params,
		TimeSource:   blockchain.NewMedianTime(),
		SigCache:     txscript.NewSigCache(1000),
		IndexManager: manager,
	})
	require.NoError(t, err, "blockchain.New")

	best := chain.BestSnapshot()
	err = db.View(func(dbTx database.Tx) error {
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

	buildDir, exists := AddrIndexFastBuildDir(rootDir)
	require.Falsef(t, exists, "completed build left staging at %s",
		buildDir)

	pkScript := params.GenesisBlock.Transactions[0].TxOut[0].PkScript
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript, params)
	require.NoError(t, err, "extract genesis address")
	require.Len(t, addrs, 1, "genesis addresses")
	regions, skipped, err := idx.TxRegionsForAddress(
		nil, addrs[0], 0, 1, false,
	)
	require.NoError(t, err, "TxRegionsForAddress")
	require.Zero(t, skipped, "address lookup skipped")
	require.Len(t, regions, 1, "address lookup regions")
	require.Truef(t, regions[0].Hash.IsEqual(params.GenesisHash),
		"address region block = %s, want %s", regions[0].Hash,
		params.GenesisHash)
}
