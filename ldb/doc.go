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
*/
package ldb
