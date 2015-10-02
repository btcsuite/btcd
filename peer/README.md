peer
====

[![Build Status](https://travis-ci.org/btcsuite/btcd.png?branch=master)]
(https://travis-ci.org/btcsuite/btcd)

Package peer provides a common base for creating and managing bitcoin network
peers.

## Overview

- Create peers for full nodes, Simplified Payment Verification (SPV) nodes,
  proxies etc
- Built-in handlers for common messages like initial message version
  negotiation, handling and responding to pings
- Register and manage multiple custom handlers for all messages

## Documentation

[![GoDoc](https://godoc.org/github.com/btcsuite/btcd/peer?status.png)]
(http://godoc.org/github.com/btcsuite/btcd/peer)

Full `go doc` style documentation for the project can be viewed online without
installing this package by using the GoDoc site here:
http://godoc.org/github.com/btcsuite/btcd/peer

You can also view the documentation locally once the package is installed with
the `godoc` tool by running `godoc -http=":6060"` and pointing your browser to
http://localhost:6060/pkg/github.com/btcsuite/btcd/peer

## Installation

```bash
$ go get github.com/btcsuite/btcd/peer
```

Package peer is licensed under the [copyfree](http://copyfree.org) ISC
License.
