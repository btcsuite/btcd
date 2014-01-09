// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb

import (
//"fmt"
)

// InvalidateTxCache clear/release all cached transactions.
func (db *LevelDb) InvalidateTxCache() {
}

// InvalidateTxCache clear/release all cached blocks.
func (db *LevelDb) InvalidateBlockCache() {
}

// InvalidateCache clear/release all cached blocks and transactions.
func (db *LevelDb) InvalidateCache() {
}
