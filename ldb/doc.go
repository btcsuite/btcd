// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package ldb implements an instance of btcdb backed by leveldb.

Database version number is stored in a flat file <dbname>.ver
Currently a single (littlendian) integer in the file. If there is
additional data to save in the future, the presence of additional
data can be indicated by changing the version number, then parsing the
file differently.
*/
package ldb
