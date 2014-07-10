btcutil
=======

[![Build Status](https://travis-ci.org/conformal/btcutil.png?branch=master)]
(https://travis-ci.org/conformal/btcutil)

Package btcutil provides bitcoin-specific convenience functions and types.
A comprehensive suite of tests is provided to ensure proper functionality.  See
`test_coverage.txt` for the gocov coverage report.  Alternatively, if you are
running a POSIX OS, you can run the `cov_report.sh` script for a real-time
report.  Package btcutil is licensed under the liberal ISC license.

This package was developed for btcd, an alternative full-node implementation of
bitcoin which is under active development by Conformal.  Although it was
primarily written for btcd, this package has intentionally been designed so it
can be used as a standalone package for any projects needing the functionality
provided.

## Documentation

[![GoDoc](https://godoc.org/github.com/conformal/btcutil?status.png)]
(http://godoc.org/github.com/conformal/btcutil)

Full `go doc` style documentation for the project can be viewed online without
installing this package by using the GoDoc site here:
http://godoc.org/github.com/conformal/btcutil

You can also view the documentation locally once the package is installed with
the `godoc` tool by running `godoc -http=":6060"` and pointing your browser to
http://localhost:6060/pkg/github.com/conformal/btcutil

## Installation

```bash
$ go get github.com/conformal/btcutil
```

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

Package btcutil is licensed under the [copyfree](http://copyfree.org) ISC
License.
