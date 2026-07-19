// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/wire/v2"
)

var errTxNotFound = errors.New("tx not found in cold block")

// TestCreateTxRawResultWitnessPruned verifies that createTxRawResult marks cold
// (witness-stripped) transactions with witness_pruned and does not invent a
// distinct wtxid (hash == txid when pruned).
func TestCreateTxRawResultWitnessPruned(t *testing.T) {
	raw := loadWitnessBlockFixture(t)
	block, err := btcutil.NewBlockFromBytes(raw)
	if err != nil {
		t.Fatalf("NewBlockFromBytes: %v", err)
	}

	// Pick a non-coinbase tx that had witness before compaction.
	var fullTx *btcutil.Tx
	for i, tx := range block.Transactions() {
		if i == 0 {
			continue
		}
		if tx.MsgTx().HasWitness() {
			fullTx = tx
			break
		}
	}
	if fullTx == nil {
		t.Skip("fixture has no non-coinbase witness tx")
	}

	dbPath := t.TempDir()
	db, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer db.Close()

	hash := block.Hash()
	if err := db.Update(func(tx database.Tx) error {
		return tx.StoreBlock(block)
	}); err != nil {
		t.Fatalf("StoreBlock: %v", err)
	}
	if err := db.Update(func(tx database.Tx) error {
		return tx.(database.ColdCompactor).CompactBlockToCold(hash)
	}); err != nil {
		t.Fatalf("CompactBlockToCold: %v", err)
	}

	if !dbBlockIsCold(db, hash) {
		t.Fatal("expected block to be cold after compaction")
	}

	// Load the stripped tx bytes the way getrawtransaction does.
	var stripped *wire.MsgTx
	err = db.View(func(dbTx database.Tx) error {
		blkBytes, err := dbTx.FetchBlock(hash)
		if err != nil {
			return err
		}
		coldBlock, err := btcutil.NewBlockFromBytes(blkBytes)
		if err != nil {
			return err
		}
		for _, tx := range coldBlock.Transactions() {
			if tx.Hash().IsEqual(fullTx.Hash()) {
				stripped = tx.MsgTx()
				return nil
			}
		}
		return errTxNotFound
	})
	if err != nil {
		t.Fatalf("load stripped tx: %v", err)
	}

	txid := fullTx.Hash().String()
	wantWtxid := fullTx.MsgTx().WitnessHash().String()
	if wantWtxid == txid {
		t.Fatal("fixture tx wtxid == txid before prune; need a real witness tx")
	}

	hotResult, err := createTxRawResult(&chaincfg.MainNetParams, fullTx.MsgTx(),
		txid, &block.MsgBlock().Header, hash.String(), 1, 100, false)
	if err != nil {
		t.Fatalf("createTxRawResult hot: %v", err)
	}
	if hotResult.WitnessPruned {
		t.Error("hot result unexpectedly witness_pruned")
	}
	if hotResult.Hash != wantWtxid {
		t.Errorf("hot hash=%s, want wtxid %s", hotResult.Hash, wantWtxid)
	}

	coldResult, err := createTxRawResult(&chaincfg.MainNetParams, stripped,
		txid, &block.MsgBlock().Header, hash.String(), 1, 100, true)
	if err != nil {
		t.Fatalf("createTxRawResult cold: %v", err)
	}
	if !coldResult.WitnessPruned {
		t.Error("cold result missing witness_pruned")
	}
	if coldResult.Hash != txid {
		t.Errorf("cold hash=%s, want txid %s (must not invent wtxid)",
			coldResult.Hash, txid)
	}
	if coldResult.Hash == wantWtxid {
		t.Error("cold result still reports historical wtxid")
	}
}

func loadWitnessBlockFixture(t *testing.T) []byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("wire", "testdata", "block-*.blk"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Skip("no wire/testdata block fixtures")
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		block, err := btcutil.NewBlockFromBytes(raw)
		if err != nil {
			continue
		}
		stripped, err := block.BytesNoWitness()
		if err != nil {
			continue
		}
		if len(stripped) != len(raw) {
			return raw
		}
	}
	t.Skip("no witness-containing fixture")
	return nil
}
