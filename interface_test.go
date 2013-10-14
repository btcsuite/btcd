// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcdb_test

import (
	"fmt"
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

// TestAddDuplicateDriver ensures that adding a duplicate driver does not
// overwrite an existing one.
func TestAddDuplicateDriver(t *testing.T) {
	supportedDBs := btcdb.SupportedDBs()
	if len(supportedDBs) == 0 {
		t.Errorf("TestAddDuplicateDriver: No backends to test")
		return
	}
	dbType := supportedDBs[0]

	// bogusCreateDB is a function which acts as a bogus create and open
	// driver function and intentionally returns a failure that can be
	// detected if the interface allows a duplicate driver to overwrite an
	// existing one.
	bogusCreateDB := func(string) (btcdb.Db, error) {
		return nil, fmt.Errorf("duplicate driver allowed for database "+
			"type [%v]", dbType)
	}

	// Create a driver that tries to replace an existing one.  Set its
	// create and open functions to a function that causes a test failure if
	// they are invoked.
	driver := btcdb.DriverDB{
		DbType: dbType,
		Create: bogusCreateDB,
		Open:   bogusCreateDB,
	}
	btcdb.AddDBDriver(driver)

	// Ensure creating a database of the type that we tried to replace
	// doesn't fail (if it does, it indicates the driver was erroneously
	// replaced).
	_, teardown, err := createDB(dbType, "dupdrivertest", true)
	if err != nil {
		t.Errorf("TestAddDuplicateDriver: %v", err)
		return
	}
	teardown()
}

// TestCreateOpenFail ensures that errors which occur while opening or closing
// a database are handled properly.
func TestCreateOpenFail(t *testing.T) {
	// bogusCreateDB is a function which acts as a bogus create and open
	// driver function that intentionally returns a failure which can be
	// detected.
	dbType := "createopenfail"
	openError := fmt.Errorf("failed to create or open database for "+
		"database type [%v]", dbType)
	bogusCreateDB := func(string) (btcdb.Db, error) {
		return nil, openError
	}

	// Create and add driver that intentionally fails when created or opened
	// to ensure errors on database open and create are handled properly.
	driver := btcdb.DriverDB{
		DbType: dbType,
		Create: bogusCreateDB,
		Open:   bogusCreateDB,
	}
	btcdb.AddDBDriver(driver)

	// Ensure creating a database with the new type fails with the expected
	// error.
	_, err := btcdb.CreateDB(dbType, "createfailtest")
	if err != openError {
		t.Errorf("TestCreateOpenFail: expected error not received - "+
			"got: %v, want %v", err, openError)
		return
	}

	// Ensure opening a database with the new type fails with the expected
	// error.
	_, err = btcdb.OpenDB(dbType, "openfailtest")
	if err != openError {
		t.Errorf("TestCreateOpenFail: expected error not received - "+
			"got: %v, want %v", err, openError)
		return
	}
}

// TestCreateOpenUnsupported ensures that attempting to create or open an
// unsupported database type is handled properly.
func TestCreateOpenUnsupported(t *testing.T) {
	// Ensure creating a database with an unsupported type fails with the
	// expected error.
	dbType := "unsupported"
	_, err := btcdb.CreateDB(dbType, "unsupportedcreatetest")
	if err != btcdb.DbUnknownType {
		t.Errorf("TestCreateOpenUnsupported: expected error not "+
			"received - got: %v, want %v", err, btcdb.DbUnknownType)
		return
	}

	// Ensure opening a database with the new type fails with the expected
	// error.
	_, err = btcdb.OpenDB(dbType, "unsupportedopentest")
	if err != btcdb.DbUnknownType {
		t.Errorf("TestCreateOpenUnsupported: expected error not "+
			"received - got: %v, want %v", err, btcdb.DbUnknownType)
		return
	}
}
