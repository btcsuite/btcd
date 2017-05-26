ffldb
=====

[![Build Status](http://img.shields.io/travis/decred/dcrd.svg)](https://travis-ci.org/decred/dcrd)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](http://godoc.org/github.com/decred/dcrd/database/ffldb)

Package ffldb implements a driver for the database package that uses leveldb for
the backing metadata and flat files for block storage.

This driver is the recommended driver for use with dcrd.  It makes use leveldb
for the metadata, flat files for block storage, and checksums in key areas to
ensure data integrity.

Package ffldb is licensed under the copyfree ISC license.

## Usage

This package is a driver to the database package and provides the database type
of "ffldb".  The parameters the Open and Create functions take are the
database path as a string and the block network.

```Go
db, err := database.Open("ffldb", "path/to/database", wire.MainNet)
if err != nil {
	// Handle error
}
```

```Go
db, err := database.Create("ffldb", "path/to/database", wire.MainNet)
if err != nil {
	// Handle error
}
```

## License

Package ffldb is licensed under the [copyfree](http://copyfree.org) ISC
License.
