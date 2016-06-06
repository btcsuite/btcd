tickettreap
===========

[![Build Status](https://img.shields.io/travis/decred/dcrd.svg)]
(https://travis-ci.org/decred/dcrd) [![ISC License]
(http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)]
(http://godoc.org/github.com/decred/dcrd/blockchain/stake/internal/tickettreap)

Package tickettreap implements a treap data structure that is used to hold
live tickets ordered by their key along with some associated data using a
combination of binary search tree and heap semantics.  It is a self-organizing
and randomized data structure that doesn't require complex operations to
maintain balance.  Search, insert, and delete operations are all O(log n).
Both mutable and immutable variants are provided.

The mutable variant is typically faster since it is able to simply update the
treap when modifications are made.  However, a mutable treap is not safe for
concurrent access without careful use of locking by the caller and care must be
taken when iterating since it can change out from under the iterator.

The immutable variant works by creating a new version of the treap for all
mutations by replacing modified nodes with new nodes that have updated values
while sharing all unmodified nodes with the previous version.  This is extremely
useful in concurrent applications since the caller only has to atomically
replace the treap pointer with the newly returned version after performing any
mutations.  All readers can simply use their existing pointer as a snapshot
since the treap it points to is immutable.  This effectively provides O(1)
snapshot capability with efficient memory usage characteristics since the old
nodes only remain allocated until there are no longer any references to them.

## Usage

This package is only used internally in the stake code and as such is not
available for use outside of it.

## License

Package tickettreap is licensed under the [copyfree](http://copyfree.org) ISC
License.
