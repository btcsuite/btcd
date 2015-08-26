// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
This test file is part of the ffldb package rather than than the ffldb_test
package so it can bridge access to the internals to properly test cases which
are either not possible or can't reliably be tested via the public interface.
The functions are only exported while the tests are being run.
*/

package ffldb

import "github.com/decred/dcrd/database"

// TstRunWithMaxBlockFileSize runs the passed function with the maximum allowed
// file size for the database set to the provided value.  The value will be set
// back to the original value upon completion.
func TstRunWithMaxBlockFileSize(idb database.DB, size uint32, fn func()) {
	ffldb := idb.(*db)
	origSize := ffldb.store.maxBlockFileSize

	ffldb.store.maxBlockFileSize = size
	fn()
	ffldb.store.maxBlockFileSize = origSize
}
