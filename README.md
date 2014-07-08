btcscript
=========

[![Build Status](https://travis-ci.org/conformal/btcscript.png?branch=master)]
(https://travis-ci.org/conformal/btcscript)

Package btcscript implements the bitcoin transaction scripts.  There is
a comprehensive test suite. `test_coverage.txt` contains the current
coverage statistics (generated using gocov).  On a UNIX-like OS, the
script `cov_report.sh` can be used to generate the report.  Package
btcscript is licensed under the liberal ISC license.

This package is one of the core packages from btcd, an alternative full-node
implementation of bitcoin which is under active development by Conformal.
Although it was primarily written for btcd, this package has intentionally been
designed so it can be used as a standalone package for any projects needing to
use or validate bitcoin transaction scripts.

## Bitcoin Scripts

Bitcoin provides a stack-based, FORTH-like langauge for the scripts in
the bitcoin transactions.  This language is not turing complete
although it is still fairly powerful.  A description of the language
can be found at https://en.bitcoin.it/wiki/Script

## Documentation

[![GoDoc](https://godoc.org/github.com/conformal/btcscript?status.png)]
(http://godoc.org/github.com/conformal/btcscript)

Full `go doc` style documentation for the project can be viewed online without
installing this package by using the GoDoc site
[here](http://godoc.org/github.com/conformal/btcscript).

You can also view the documentation locally once the package is installed with
the `godoc` tool by running `godoc -http=":6060"` and pointing your browser to
http://localhost:6060/pkg/github.com/conformal/btcscript

## Installation

```bash
$ go get github.com/conformal/btcscript
```

## Examples

* [Standard Pay-to-pubkey-hash Script]
  (http://godoc.org/github.com/conformal/btcscript#example-PayToAddrScript)  
  Demonstrates creating a script which pays to a bitcoin address.  It also
  prints the created script hex and uses the DisasmString function to display
  the disassembled script.

* [Extracting Details from Standard Scripts]
  (http://godoc.org/github.com/conformal/btcscript#example-ExtractPkScriptAddrs)  
  Demonstrates extracting information from a standard public key script.

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

Package btcscript is licensed under the liberal ISC License.
