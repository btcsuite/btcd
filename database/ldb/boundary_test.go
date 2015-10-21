// Copyright (c) 2013-2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb_test

import (
	"os"
	"testing"

	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
)

// we need to test for an empty database and make certain it returns the proper
// values

func TestEmptyDB(t *testing.T) {

	dbname := "tstdbempty"
	dbnamever := dbname + ".ver"
	_ = os.RemoveAll(dbname)
	_ = os.RemoveAll(dbnamever)
	db, err := database.CreateDB("leveldb", dbname)
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer os.RemoveAll(dbname)
	defer os.RemoveAll(dbnamever)

	sha, height, err := db.NewestSha()
	if !sha.IsEqual(&wire.ShaHash{}) {
		t.Errorf("sha not zero hash")
	}
	if height != -1 {
		t.Errorf("height not -1 %v", height)
	}

	// This is a reopen test
	if err := db.Close(); err != nil {
		t.Errorf("Close: unexpected error: %v", err)
	}

	db, err = database.OpenDB("leveldb", dbname)
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
	if !sha.IsEqual(&wire.ShaHash{}) {
		t.Errorf("sha not zero hash")
	}
	if height != -1 {
		t.Errorf("height not -1 %v", height)
	}
}
