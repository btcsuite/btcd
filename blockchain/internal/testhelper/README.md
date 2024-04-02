testhelper
==========

[![Build Status](https://github.com/btcsuite/btcd/workflows/Build%20and%20Test/badge.svg)](https://github.com/btcsuite/btcd/actions)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/github.com/btcsuite/btcd/blockchain/testhelper)

Package testhelper provides functions that are used internally in the
btcd/blockchain and btcd/blockchain/fullblocktests package to test consensus
validation rules.  Mainly provided to avoid dependency cycles internally among
the different packages in btcd.

## License

Package testhelper is licensed under the [copyfree](http://copyfree.org) ISC
License.
