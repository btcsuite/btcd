chaingen
========

[![Build Status](https://travis-ci.org/decred/dcrd.png?branch=master)]
(https://travis-ci.org/decred/dcrd) [![ISC License]
(http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)]
(http://godoc.org/github.com/decred/dcrd/blockchain/chaingen)

Package chaingen provides facilities for generating a full chain of blocks.

## Overview

Many consensus-related tests require a full chain of valid blocks with several
pieces of contextual information such as versions and votes.  Generating such a
chain is not a trivial task due to things such as the fact that tickets must be
purchased (at the correct ticket price), the appropriate winning votes must be
cast (which implies keeping track of all live tickets and implementing the
lottery selection algorithm), and all of the state-specific header fields such
as the pool size and the proof-of-work and proof-of-stake difficulties must be
set properly.

In order to simplify this complex process, this package provides a generator
that keeps track of all of the necessary state and generates and solves blocks
accordingly while allowing the caller to manipulate the blocks via munge
functions.

## Examples

* [Basic Usage Example]
  (http://godoc.org/github.com/decred/dcrd/blockchain/chaingen#example-package--BasicUsage)  
  Demonstrates creating a new generator instance and using it to generate the
  required premine block and enough blocks to have mature coinbase outputs to
  work with along with asserting the generator state along the way.

## Installation

```bash
$ go get -u github.com/decred/dcrd/blockchain/chaingen
```

## License

Package chaingen is licensed under the [copyfree](http://copyfree.org) ISC
License.
