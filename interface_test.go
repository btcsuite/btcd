// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcdb_test

import (
	"github.com/conformal/btcdb"
	"testing"
)

// testNewestShaEmpty ensures the NewestSha returns the values expected by
// the interface contract.
func testNewestShaEmpty(t *testing.T, db btcdb.Db) {
	sha, height, err := db.NewestSha()
	if err != nil {
		t.Errorf("NewestSha error %v", err)
	}
	if !sha.IsEqual(&zeroHash) {
		t.Errorf("NewestSha wrong hash got: %s, want %s", sha, &zeroHash)

	}
	if height != -1 {
		t.Errorf("NewestSha wrong height got: %s, want %s", height, -1)
	}
}

// TestEmptyDB tests that empty databases are handled properly.
func TestEmptyDB(t *testing.T) {
	for _, dbType := range btcdb.SupportedDBs() {
		// Ensure NewestSha returns expected values for a newly created
		// db.
		db, teardown, err := createDB(dbType, "emptydb", false)
		if err != nil {
			t.Errorf("Failed to create test database %v", err)
			return
		}
		testNewestShaEmpty(t, db)

		// Ensure NewestSha still returns expected values for an empty
		// database after reopen.
		db.Close()
		db, err = openDB(dbType, "emptydb")
		if err != nil {
			t.Errorf("Failed to open test database %v", err)
			return
		}
		testNewestShaEmpty(t, db)
		db.Close()

		// Clean up the old db.
		teardown()
	}
}
