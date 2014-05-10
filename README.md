btcrpcclient
============

[![Build Status](https://travis-ci.org/conformal/btcrpcclient.png?branch=master)]
(https://travis-ci.org/conformal/btcrpcclient)
[![GoDoc](https://godoc.org/github.com/conformal/btcrpcclient?status.png)]
(http://godoc.org/github.com/conformal/btcrpcclient)

This package is currently under development.

You really probably don't want to use this yet!

## Documentation

* [API Reference](http://godoc.org/github.com/conformal/btcrpcclient)
* [btcd Websockets Example](https://github.com/conformal/btcrpcclient/blob/master/examples/btcdwebsockets)  
  Connects to a btcd RPC server using TLS-secured websockets,
  registers for block connected and block disconnected notifications, and gets
  the current block count
* [Bitcoin Core HTTP POST Example](https://github.com/conformal/btcrpcclient/blob/master/examples/bitcoincorehttp)  
  Connects to a bitcoin core RPC server using HTTP POST mode with TLS disabled
  and gets the current block count

## Installation

```bash
$ go get github.com/conformal/btcrpcclient
```

## License

Package btcrpcclient is licensed under the [copyfree](http://copyfree.org) ISC
License.
