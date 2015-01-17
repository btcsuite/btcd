btcdb
=====

[![Build Status](http://img.shields.io/travis/conformal/btcdb.svg)]
(https://travis-ci.org/conformal/btcdb) [![Coverage Status]
(https://img.shields.io/coveralls/conformal/btcdb.svg)]
(https://coveralls.io/r/conformal/btcdb) [![ISC License]
(http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

Package btcdb provides a database interface for the bitcoin block chain and
transactions.

## Documentation

[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)]
(http://godoc.org/github.com/btcsuite/btcdb)

Full `go doc` style documentation for the project can be viewed online without
installing this package by using the GoDoc site
[here](http://godoc.org/github.com/btcsuite/btcdb).

You can also view the documentation locally once the package is installed with
the `godoc` tool by running `godoc -http=":6060"` and pointing your browser to
http://localhost:6060/pkg/github.com/btcsuite/btcdb

## Installation

```bash
$ go get github.com/btcsuite/btcdb
```

## Examples

* [CreateDB Example]
  (http://godoc.org/github.com/btcsuite/btcdb#example-CreateDB)  
  Demonstrates creating a new database and inserting the genesis block into it.

* [NewestSha Example]
  (http://godoc.org/github.com/btcsuite/btcdb#example-Db--NewestSha)  
  Demonstrates  querying the database for the most recent best block height and
  hash.

## TODO
- Increase test coverage to 100%

## GPG Verification Key

All official release tags are signed by Conformal so users can ensure the code
has not been tampered with and is coming from Conformal.  To verify the
signature perform the following:

- Download the public key from the Conformal website at
  https://opensource.conformal.com/GIT-GPG-KEY-conformal.txt

- Import the public key into your GPG keyring:
  ```bash
  gpg --import GIT-GPG-KEY-conformal.txt
  ```

- Verify the release tag with the following command where `TAG_NAME` is a
  placeholder for the specific tag:
  ```bash
  git tag -v TAG_NAME
  ```

## License

Package btcdb is licensed under the [copyfree](http://copyfree.org) ISC License.
