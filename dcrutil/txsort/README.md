txsort
======

[![Build Status](http://img.shields.io/travis/decred/dcrd.svg)](https://travis-ci.org/decred/dcrd)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](http://img.shields.io/badge/godoc-reference-blue.svg)](http://godoc.org/github.com/decred/dcrd/dcrutil/txsort)

Package txsort provides stable transaction sorting.

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

A comprehensive suite of tests is provided to ensure proper functionality.

## Installation and Updating

```bash
$ go get -u github.com/decred/dcrd/dcrutil/txsort
```

## License

Package txsort is licensed under the [copyfree](http://copyfree.org) ISC
License.
