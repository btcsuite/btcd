// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb_test

import (
	"github.com/conformal/btcdb"
	"github.com/conformal/btcwire"
	"os"
	"testing"
)

// we need to test for empty databas and make certain it returns proper value

func TestEmptyDB(t *testing.T) {

	dbname := "tstdbempty"
	_ = os.RemoveAll(dbname)
	db, err := btcdb.CreateDB("leveldb", dbname)
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer os.RemoveAll(dbname)

	// This is a reopen test
	db.Close()

	db, err = btcdb.OpenDB("leveldb", dbname)
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer db.Close()

	sha, height, err := db.NewestSha()
	if !sha.IsEqual(&btcwire.ShaHash{}) {
		t.Errorf("sha not nil")
	}
	if height != -1 {
		t.Errorf("height not -1 %v", height)
	}
}
