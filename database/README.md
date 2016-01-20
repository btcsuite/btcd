database
========

[![ISC License]
(http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

Package database provides a database interface for the decred block chain and
transactions.

Please note that this package is intended to enable dcrd to support different
database backends and is not something that a client can directly access as only
one entity can have the database open at a time (for most database backends),
and that entity will be dcrd.

When a client wants programmatic access to the data provided by dcrd, they'll
likely want to use the [btcrpcclient](https://github.com/decred/btcrpcclient)
package which makes use of the [JSON-RPC API]
(https://github.com/decred/dcrd/tree/master/docs/json_rpc_api.md).

## Documentation

[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)]
(http://godoc.org/github.com/decred/dcrd/database)

Full `go doc` style documentation for the project can be viewed online without
installing this package by using the GoDoc site
[here](http://godoc.org/github.com/decred/dcrd/database).

You can also view the documentation locally once the package is installed with
the `godoc` tool by running `godoc -http=":6060"` and pointing your browser to
http://localhost:6060/pkg/github.com/decred/dcrd/database

## Installation

```bash
$ go get github.com/decred/dcrd/database
```

## Examples

* [CreateDB Example]
  (http://godoc.org/github.com/decred/dcrd/database#example-CreateDB)  
  Demonstrates creating a new database and inserting the genesis block into it.

* [NewestSha Example]
  (http://godoc.org/github.com/decred/dcrd/database#example-Db--NewestSha)  
  Demonstrates  querying the database for the most recent best block height and
  hash.

## TODO
- Increase test coverage to 100%

## License

Package database is licensed under the [copyfree](http://copyfree.org) ISC
License.
