// Copyright (c) 2015 The btcsuite developers
// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package txsort provides stable transaction sorting.

Overview

This package implements a standard lexicographical sort order of transaction
inputs and outputs.  This is useful to standardize transactions for faster
multi-party agreement as well as preventing information leaks in a single-party
use case.  It is a modified form of BIP69 which has been updated to account for
differences with Decred-specific transactions.

The sort order for transaction inputs is defined as follows:
- Previous transaction tree in ascending order
- Previous transaction hash (treated as a big-endian uint256) lexicographically
  in ascending order
- Previous output index in ascending order

The sort order for transaction outputs is defined as follows:
- Amount in ascending order
- Public key script version in ascending order
- Raw public key script bytes lexicographically in ascending order
*/
package txsort
