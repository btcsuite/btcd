txsort
======

[![Build Status](http://img.shields.io/travis/btcsuite/btcutil.svg)]
(https://travis-ci.org/btcsuite/btcutil) [![ISC License]
(http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

Package txsort provides the transaction sorting according to BIPLI01.

BIPLI01 defines a standard lexicographical sort order of transaction inputs and
outputs.  This is useful to standardize transactions for faster multi-party
agreement as well as preventing information leaks in a single-party use case.

The BIP goes into more detail, but for a quick and simplistic overview, the
order for inputs is defined as first sorting on the previous output hash and
then on the index as a tie breaker.  The order for outputs is defined as first
sorting on the amount and then on the raw public key script bytes as a tie
breaker.

A comprehensive suite of tests is provided to ensure proper functionality.

## Documentation

[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)]
(http://godoc.org/github.com/btcsuite/btcutil/txsort)

Full `go doc` style documentation for the project can be viewed online without
installing this package by using the GoDoc site here:
http://godoc.org/github.com/btcsuite/btcutil/txsort

You can also view the documentation locally once the package is installed with
the `godoc` tool by running `godoc -http=":6060"` and pointing your browser to
http://localhost:6060/pkg/github.com/btcsuite/btcutil/txsort

## Installation and Updating

```bash
$ go get -u github.com/btcsuite/btcutil/txsort
```

## License

Package txsort is licensed under the [copyfree](http://copyfree.org) ISC
License.
