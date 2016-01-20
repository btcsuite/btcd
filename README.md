dcrutil
=======

Package dcrutil provides decred-specific convenience functions and types.
A comprehensive suite of tests is provided to ensure proper functionality.  See
`test_coverage.txt` for the gocov coverage report.  Alternatively, if you are
running a POSIX OS, you can run the `cov_report.sh` script for a real-time
report.  Package dcrutil is licensed under the liberal ISC license.

This package was developed for dcrd, an alternative full-node implementation of
Decred which is under active development by Company 0.  Although it was
primarily written for dcrd, this package has intentionally been designed so it
can be used as a standalone package for any projects needing the functionality
provided.

## Documentation

[![GoDoc](https://godoc.org/github.com/decred/dcrutil?status.png)]
(http://godoc.org/github.com/decred/dcrutil)

Full `go doc` style documentation for the project can be viewed online without
installing this package by using the GoDoc site here:
http://godoc.org/github.com/decred/dcrutil

You can also view the documentation locally once the package is installed with
the `godoc` tool by running `godoc -http=":6060"` and pointing your browser to
http://localhost:6060/pkg/github.com/decred/dcrutil

## Installation

```bash
$ go get github.com/decred/dcrutil
```

## License

Package dcrutil is licensed under the [copyfree](http://copyfree.org) ISC
License.
