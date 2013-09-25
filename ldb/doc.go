// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package sqlite3 implements a sqlite3 instance of btcdb.

sqlite provides a zero setup, single file database.  It requires cgo
and the presence of the sqlite library and headers, but nothing else.
The performance is generally high although it goes down with database
size.

Many of the block or tx specific functions for btcdb are in this subpackage.

Database version number is stored in a flat file <dbname>.ver
Currently a single (littlendian) integer in the file. If there is
additional data to save in the future, the presense of additional
data can be indicated by changing the version number, then parsing the
file differently.

*/
package ldb
