indexers
========

[![Build Status](https://travis-ci.org/decred/dcrd.png?branch=master)](https://travis-ci.org/decred/dcrd)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://godoc.org/github.com/decred/dcrd/blockchain/indexers?status.png)](http://godoc.org/github.com/decred/dcrd/blockchain/indexers)

Package indexers implements optional block chain indexes.

These indexes are typically used to enhance the amount of information available
via an RPC interface.

## Supported Indexers

- Transaction-by-hash (txbyhashidx) Index
  - Creates a mapping from the hash of each transaction to the block that
    contains it along with its offset and length within the serialized block
- Transaction-by-address (txbyaddridx) Index
  - Creates a mapping from every address to all transactions which either credit
    or debit the address
  - Requires the transaction-by-hash index
- Address-ever-seen (existsaddridx) Index
  - Stores a key with an empty value for every address that has ever existed 
    and was seen by the client
  - Requires the transaction-by-hash index

## Installation

```bash
$ go get -u github.com/decred/dcrd/blockchain/indexers
```

## License

Package indexers is licensed under the [copyfree](http://copyfree.org) ISC
License.
