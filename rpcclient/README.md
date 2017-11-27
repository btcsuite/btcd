rpcclient
=========

[![Build Status](http://img.shields.io/travis/decred/dcrd.svg)](https://travis-ci.org/decred/dcrd)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](http://godoc.org/github.com/decred/dcrd/rpcclient)

rpcclient implements a Websocket-enabled Decred JSON-RPC client package written
in [Go](http://golang.org/).  It provides a robust and easy to use client for
interfacing with a Decred RPC server that uses a dcrd compatible Decred
JSON-RPC API.

## Status

This package is currently under active development.  It is already stable and
the infrastructure is complete.  However, there are still several RPCs left to
implement and the API is not stable yet.

## Documentation

* [API Reference](http://godoc.org/github.com/decred/dcrd/rpcclient)
* [dcrd Websockets Example](https://github.com/decred/dcrd/tree/master/rpcclient/examples/dcrdwebsockets)
  Connects to a dcrd RPC server using TLS-secured websockets, registers for
  block connected and block disconnected notifications, and gets the current
  block count
* [dcrwallet Websockets Example](https://github.com/decred/dcrd/tree/master/rpcclient/examples/dcrwalletwebsockets)  
  Connects to a dcrwallet RPC server using TLS-secured websockets, registers for
  notifications about changes to account balances, and gets a list of unspent
  transaction outputs (utxos) the wallet can sign

## Major Features

* Supports Websockets (dcrd/dcrwallet) and HTTP POST mode (bitcoin core-like)
* Provides callback and registration functions for dcrd/dcrwallet notifications
* Supports dcrd extensions
* Translates to and from higher-level and easier to use Go types
* Offers a synchronous (blocking) and asynchronous API
* When running in Websockets mode (the default):
  * Automatic reconnect handling (can be disabled)
  * Outstanding commands are automatically reissued
  * Registered notifications are automatically reregistered
  * Back-off support on reconnect attempts

## Installation

```bash
$ go get -u github.com/decred/dcrd/rpcclient
```

## License

Package rpcclient is licensed under the [copyfree](http://copyfree.org) ISC
License.
