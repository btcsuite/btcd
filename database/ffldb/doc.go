// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package ffldb implements a driver for the database package that uses leveldb
for the backing metadata and flat files for block storage.

This driver is the recommended driver for use with btcd.  It makes use leveldb
for the metadata, flat files for block storage, and checksums in key areas to
ensure data integrity.

# Usage

This package is a driver to the database package and provides the database type
of "ffldb". Open and Create take a database path, block network, and optional
ffldb.Options to configure the LevelDB read cache:

	db, err := database.Open("ffldb", "path/to/database", wire.MainNet)
	if err != nil {
		// Handle error
	}

	opts := ffldb.Options{BlockCacheSize: 32 << 20}
	db, err := database.Create("ffldb", "path/to/database", wire.MainNet, opts)
	if err != nil {
		// Handle error
	}
*/
package ffldb
