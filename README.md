dcrutil
=======

[![Build Status](http://img.shields.io/travis/decred/dcrutil.svg)]
(https://travis-ci.org/decred/dcrutil) [![Coverage Status]
(http://img.shields.io/coveralls/decred/dcrutil.svg)]
(https://coveralls.io/r/decred/dcrutil?branch=master) [![ISC License]
(http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](http://img.shields.io/badge/godoc-reference-blue.svg)]
(http://godoc.org/github.com/decred/dcrutil)

Package dcrutil provides decred-specific convenience functions and types.
A comprehensive suite of tests is provided to ensure proper functionality.  See
`test_coverage.txt` for the gocov coverage report.  Alternatively, if you are
running a POSIX OS, you can run the `cov_report.sh` script for a real-time
report.

This package was developed for dcrd, a full-node implementation of Decred which
is under active development by Company 0.  Although it was primarily written for
dcrd, this package has intentionally been designed so it can be used as a
standalone package for any projects needing the functionality provided.

## Installation and Updating

```bash
$ go get -u github.com/decred/dcrutil
```

## License

Package dcrutil is licensed under the [copyfree](http://copyfree.org) ISC
License.
