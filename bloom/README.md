bloom
=====

[![Build Status](https://travis-ci.org/decred/dcrutil.png?branch=master)]
(https://travis-ci.org/decred/dcrutil)

Package bloom provides an API for dealing with decred-specific bloom filters.

A comprehensive suite of tests is provided to ensure proper functionality.  See
`test_coverage.txt` for the gocov coverage report.  Alternatively, if you are
running a POSIX OS, you can run the `cov_report.sh` script for a real-time
report.  Package coinset is licensed under the liberal ISC license.

## Documentation

[![GoDoc](https://godoc.org/github.com/decred/dcrutil/bloom?status.png)]
(http://godoc.org/github.com/decred/dcrutil/bloom)

Full `go doc` style documentation for the project can be viewed online without
installing this package by using the GoDoc site here:
http://godoc.org/github.com/decred/dcrutil/bloom

You can also view the documentation locally once the package is installed with
the `godoc` tool by running `godoc -http=":6060"` and pointing your browser to
http://localhost:6060/pkg/github.com/decred/dcrutil/bloom

## Installation

```bash
$ go get github.com/decred/dcrutil/bloom
```

## Examples

* [NewFilter Example]
  (http://godoc.org/github.com/decred/dcrutil/bloom#example-NewFilter)
  Demonstrates how to create a new bloom filter, add a transaction hash to it,
  and check if the filter matches the transaction.

## License

Package bloom is licensed under the [copyfree](http://copyfree.org) ISC
License.
