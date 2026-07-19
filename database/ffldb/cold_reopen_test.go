// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ffldb

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire/v2"
)

// TestColdCompactionSurvivesReopen simulates a clean shutdown after compaction
// (the recoverable outcome of a crash that occurs only after the DB transaction
// commits): close the database, reopen the same path, and assert the block is
// still cold and byte-identical to the stripped serialization.
//
// Crash *during* an in-flight compaction transaction is covered by
// TestCompactBlockToColdRollback (forced rollback leaves no orphan cold data).
func TestColdCompactionSurvivesReopen(t *testing.T) {
	dbPath := t.TempDir()

	raw := findWitnessFixture(t)
	block, err := btcutil.NewBlockFromBytes(raw)
	if err != nil {
		t.Fatalf("NewBlockFromBytes: %v", err)
	}
	hash := block.Hash()
	wantStripped := strippedSerialization(t, raw)

	pdb, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Store hot, then compact in a separate committed tx (same pattern as
	// TestCompactBlockToCold — pending-store blocks cannot be compacted).
	if err := pdb.Update(func(tx database.Tx) error {
		return tx.StoreBlock(block)
	}); err != nil {
		pdb.Close()
		t.Fatalf("StoreBlock: %v", err)
	}
	if err := pdb.Update(func(tx database.Tx) error {
		return tx.(database.ColdCompactor).CompactBlockToCold(hash)
	}); err != nil {
		pdb.Close()
		t.Fatalf("CompactBlockToCold: %v", err)
	}
	if err := pdb.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := database.Open("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer reopened.Close()

	if err := reopened.View(func(tx database.Tx) error {
		cold, err := tx.(database.ColdCompactor).IsColdBlock(hash)
		if err != nil {
			return err
		}
		if !cold {
			t.Fatal("block not cold after reopen")
		}
		got, err := tx.FetchBlock(hash)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, wantStripped) {
			t.Errorf("FetchBlock after reopen: got len %d, want stripped %d",
				len(got), len(wantStripped))
		}
		return nil
	}); err != nil {
		t.Fatalf("View after reopen: %v", err)
	}

	// Sanity: cold files still on disk under the datadir.
	matches, _ := filepath.Glob(filepath.Join(dbPath, coldFileSubdir, "*"))
	if len(matches) == 0 {
		t.Error("no cold files after reopen")
	}
}

// findWitnessFixture returns a mainnet fixture that actually contains witness
// data so cold compaction is observably different from a no-op strip.
func findWitnessFixture(t *testing.T) []byte {
	t.Helper()
	for _, raw := range loadColdFixtures(t) {
		stripped := strippedSerialization(t, raw)
		if len(stripped) != len(raw) {
			return raw
		}
	}
	t.Skip("no witness-containing fixture")
	return nil
}
