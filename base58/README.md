base58
==========

[![Build Status](https://travis-ci.org/btcsuite/btcutil.png?branch=master)]
(https://travis-ci.org/btcsuite/btcutil)

Package base58 provides an API for encoding and decoding to and from the
modified base58 encoding.  It also provides an API to do Base58Check encoding,
as described [here](https://en.bitcoin.it/wiki/Base58Check_encoding).

A comprehensive suite of tests is provided to ensure proper functionality.
Package base58 is licensed under the copyfree ISC license.

## Documentation

[![GoDoc](https://godoc.org/github.com/btcsuite/btcutil/base58?status.png)]
(http://godoc.org/github.com/btcsuite/btcutil/base58)

Full `go doc` style documentation for the project can be viewed online without
installing this package by using the GoDoc site here:
http://godoc.org/github.com/btcsuite/btcutil/base58

You can also view the documentation locally once the package is installed with
the `godoc` tool by running `godoc -http=":6060"` and pointing your browser to
http://localhost:6060/pkg/github.com/btcsuite/btcutil/base58

## Installation

```bash
$ go get github.com/btcsuite/btcutil/base58
```

## Examples

* [Decode Example]
  (http://godoc.org/github.com/btcsuite/btcutil/base58#example-Decode)  
  Demonstrates how to decode modified base58 encoded data.
* [Encode Example]
  (http://godoc.org/github.com/btcsuite/btcutil/base58#example-Encode)  
  Demonstrates how to encode data using the modified base58 encoding scheme.
* [CheckDecode Example]
  (http://godoc.org/github.com/btcsuite/btcutil/base58#example-CheckDecode)  
  Demonstrates how to decode Base58Check encoded data.
* [CheckEncode Example]
  (http://godoc.org/github.com/btcsuite/btcutil/base58#example-CheckEncode)  
  Demonstrates how to encode data using the Base58Check encoding scheme.

## License

Package base58 is licensed under the [copyfree](http://copyfree.org) ISC
License.
