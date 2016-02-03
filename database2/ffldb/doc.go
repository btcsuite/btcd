// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package ffldb implements a driver for the database package that uses leveldb
for the backing metadata and flat files for block storage.

This driver is the recommended driver for use with btcd.  It makes use leveldb
for the metadata, flat files for block storage, and checksums in key areas to
ensure data integrity.

Usage

This package is a driver to the database package and provides the database type
of "ffldb".  The parameters the Open and Create functions take are the
database path as a string and the block network:

	db, err := database.Open("ffldb", "path/to/database", wire.MainNet)
	if err != nil {
		// Handle error
	}

	db, err := database.Create("ffldb", "path/to/database", wire.MainNet)
	if err != nil {
		// Handle error
	}
*/
package ffldb
