btcdb
=====

[![Build Status](https://travis-ci.org/conformal/btcdb.png?branch=master)]
(https://travis-ci.org/conformal/btcdb)

Package btcdb provides a database interface for the bitcoin block chain and
transactions.  There is a test suite with a high percentage of code coverage.
See `test_coverage.txt` for the current coverage (using gocov).  Alternatively,
if you are running a POSIX OS, you can run the cov_report.sh script for a
real-time report.  Package btcdb is licensed under the liberal ISC license.

## Documentation

[![GoDoc](https://godoc.org/github.com/conformal/btcdb?status.png)]
(http://godoc.org/github.com/conformal/btcdb)

Full `go doc` style documentation for the project can be viewed online without
installing this package by using the GoDoc site
[here](http://godoc.org/github.com/conformal/btcdb).

You can also view the documentation locally once the package is installed with
the `godoc` tool by running `godoc -http=":6060"` and pointing your browser to
http://localhost:6060/pkg/github.com/conformal/btcdb

## Installation

```bash
$ go get github.com/conformal/btcdb
```

## Examples

* [CreateDB Example]
  (http://godoc.org/github.com/conformal/btcdb#example-CreateDB)  
  Demonstrates creating a new database and inserting the genesis block into it.

* [NewestSha Example]
  (http://godoc.org/github.com/conformal/btcdb#example-Db--NewestSha)  
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

Package btcdb is licensed under the liberal ISC License.
