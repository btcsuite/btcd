// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package btcutil provides bitcoin-specific convenience functions and types.

Block Overview

A Block defines a bitcoin block that provides easier and more efficient
manipulation of raw wire protocol blocks.  It also memoizes hashes for the
block and its transactions on their first access so subsequent accesses don't
have to repeat the relatively expensive hashing operations.
*/
package btcutil
