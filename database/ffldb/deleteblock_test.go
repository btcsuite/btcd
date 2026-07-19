// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ffldb

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire/v2"
)

// TestDeleteBlock verifies DeleteBlock removes the body from the index so
// FetchBlock fails, is idempotent, and leaves other blocks untouched.
func TestDeleteBlock(t *testing.T) {
	dbPath := t.TempDir()
	pdb, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer pdb.Close()

	fixtures := loadColdFixtures(t)
	if len(fixtures) < 2 {
		t.Skip("need at least two block fixtures")
	}

	block0, err := btcutil.NewBlockFromBytes(fixtures[0])
	if err != nil {
		t.Fatalf("block0: %v", err)
	}
	block1, err := btcutil.NewBlockFromBytes(fixtures[1])
	if err != nil {
		t.Fatalf("block1: %v", err)
	}
	hash0 := block0.Hash()
	hash1 := block1.Hash()

	if err := pdb.Update(func(tx database.Tx) error {
		if err := tx.StoreBlock(block0); err != nil {
			return err
		}
		return tx.StoreBlock(block1)
	}); err != nil {
		t.Fatalf("StoreBlock: %v", err)
	}

	if err := pdb.Update(func(tx database.Tx) error {
		return tx.DeleteBlock(hash0)
	}); err != nil {
		t.Fatalf("DeleteBlock: %v", err)
	}

	if err := pdb.View(func(tx database.Tx) error {
		has, err := tx.HasBlock(hash0)
		if err != nil {
			return err
		}
		if has {
			t.Fatal("HasBlock true after DeleteBlock")
		}
		if _, err := tx.FetchBlock(hash0); err == nil {
			t.Fatal("FetchBlock succeeded after DeleteBlock")
		}
		got, err := tx.FetchBlock(hash1)
		if err != nil {
			return err
		}
		if len(got) == 0 {
			t.Fatal("sibling block body missing after DeleteBlock")
		}
		return nil
	}); err != nil {
		t.Fatalf("View: %v", err)
	}

	// Idempotent.
	if err := pdb.Update(func(tx database.Tx) error {
		return tx.DeleteBlock(hash0)
	}); err != nil {
		t.Fatalf("DeleteBlock again: %v", err)
	}
}
