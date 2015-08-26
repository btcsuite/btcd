treap
=====

[![Build Status](https://travis-ci.org/btcsuite/btcd.png?branch=master)]
(https://travis-ci.org/btcsuite/btcd)

Package treap implements a treap data structure that is used to hold ordered
key/value pairs using a combination of binary search tree and heap semantics.
It is a self-organizing and randomized data structure that doesn't require
complex operations to to maintain balance.  Search, insert, and delete
operations are all O(log n).  Both mutable and immutable variants are provided.

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

Package treap is licensed under the copyfree ISC license.

## Usage

This package is only used internally in the database code and as such is not
available for use outside of it.

## Documentation

[![GoDoc](https://godoc.org/github.com/btcsuite/btcd/database/internal/treap?status.png)]
(http://godoc.org/github.com/btcsuite/btcd/database/internal/treap)

Full `go doc` style documentation for the project can be viewed online without
installing this package by using the GoDoc site here:
http://godoc.org/github.com/btcsuite/btcd/database/internal/treap

You can also view the documentation locally once the package is installed with
the `godoc` tool by running `godoc -http=":6060"` and pointing your browser to
http://localhost:6060/pkg/github.com/btcsuite/btcd/database/internal/treap

## License

Package treap is licensed under the [copyfree](http://copyfree.org) ISC
License.
