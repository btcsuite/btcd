// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb_test

import (
	"os"
	"testing"

	"github.com/conformal/btcdb"
	"github.com/conformal/btcwire"
)

// we need to test for empty databas and make certain it returns proper value

func TestEmptyDB(t *testing.T) {

	dbname := "tstdbempty"
	dbnamever := dbname + ".ver"
	_ = os.RemoveAll(dbname)
	_ = os.RemoveAll(dbnamever)
	db, err := btcdb.CreateDB("leveldb", dbname)
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer os.RemoveAll(dbname)
	defer os.RemoveAll(dbnamever)

	sha, height, err := db.NewestSha()
	if !sha.IsEqual(&btcwire.ShaHash{}) {
		t.Errorf("sha not zero hash")
	}
	if height != -1 {
		t.Errorf("height not -1 %v", height)
	}

	// This is a reopen test
	if err := db.Close(); err != nil {
		t.Errorf("Close: unexpected error: %v", err)
	}

	db, err = btcdb.OpenDB("leveldb", dbname)
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close: unexpected error: %v", err)
		}
	}()

	sha, height, err = db.NewestSha()
	if !sha.IsEqual(&btcwire.ShaHash{}) {
		t.Errorf("sha not zero hash")
	}
	if height != -1 {
		t.Errorf("height not -1 %v", height)
	}
}
