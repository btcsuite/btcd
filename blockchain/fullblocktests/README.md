fullblocktests
==============

[![Build Status](http://img.shields.io/travis/decred/dcrd.svg)](https://travis-ci.org/decred/dcrd)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](http://godoc.org/github.com/decred/dcrd/blockchain/fullblocktests)

Package fullblocktests provides a set of full block tests to be used for testing
the consensus validation rules.  The tests are intended to be flexible enough to
allow both unit-style tests directly against the blockchain code as well as
integration style tests over the peer-to-peer network.  To achieve that goal,
each test contains additional information about the expected result, however
that information can be ignored when doing comparison tests between two
independent versions over the peer-to-peer network.

This package has intentionally been designed so it can be used as a standalone
package for any projects needing to test their implementation against a full set
of blocks that excerise the consensus validation rules.

## Installation and Updating

```bash
$ go get -u github.com/decred/dcrd/blockchain/fullblocktests
```

## License

Package fullblocktests is licensed under the [copyfree](http://copyfree.org) ISC
License.
