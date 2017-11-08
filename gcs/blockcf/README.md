blockcf
==========

[![GoDoc](https://godoc.org/github.com/decred/dcrd/gcs/blockcf?status.png)](http://godoc.org/github.com/decred/dcrd/gcs/blockcf)

Package blockcf provides functions to build committed filters from blocks.
Unlike the gcs package, which is a general implementation of golomb coded sets,
this package is tailored for specific filter creation for Decred blocks.