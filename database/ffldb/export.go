// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ffldb

import (
	"github.com/btcsuite/btcd/database"
)

// TstRunWithMaxBlockFileSize runs the passed function with the maximum allowed
// file size for the database set to the provided value.  The value will be set
// back to the original value upon completion.
//
// Callers should only use this for testing.
func TstRunWithMaxBlockFileSize(idb database.DB, size uint32, fn func()) {
	ffldb := idb.(*db)
	origSize := ffldb.store.maxBlockFileSize

	ffldb.store.maxBlockFileSize = size
	fn()
	ffldb.store.maxBlockFileSize = origSize
}
