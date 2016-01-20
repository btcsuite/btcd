txscript
========

Package txscript implements the decred transaction script language.  There is
a comprehensive test suite.  Package txscript is licensed under the liberal ISC
license.

This package has intentionally been designed so it can be used as a standalone
package for any projects needing to use or validate decred transaction scripts.

## Decred Scripts

Decred provides a stack-based, FORTH-like langauge for the scripts in
the decred transactions.  This language is not turing complete
although it is still fairly powerful.

## Documentation

[![GoDoc](https://godoc.org/github.com/decred/dcrd/txscript?status.png)]
(http://godoc.org/github.com/decred/dcrd/txscript)

Full `go doc` style documentation for the project can be viewed online without
installing this package by using the GoDoc site
[here](http://godoc.org/github.com/decred/dcrd/txscript).

You can also view the documentation locally once the package is installed with
the `godoc` tool by running `godoc -http=":6060"` and pointing your browser to
http://localhost:6060/pkg/github.com/decred/dcrd/txscript

## Installation

```bash
$ go get github.com/decred/dcrd/txscript
```

## Examples

* [Standard Pay-to-pubkey-hash Script]
  (http://godoc.org/github.com/decred/dcrd/txscript#example-PayToAddrScript)  
  Demonstrates creating a script which pays to a decred address.  It also
  prints the created script hex and uses the DisasmString function to display
  the disassembled script.

* [Extracting Details from Standard Scripts]
  (http://godoc.org/github.com/decred/dcrd/txscript#example-ExtractPkScriptAddrs)  
  Demonstrates extracting information from a standard public key script.

* [Manually Signing a Transaction Output]
  (http://godoc.org/github.com/decred/dcrd/txscript#example-SignTxOutput)  
  Demonstrates manually creating and signing a redeem transaction.

## License

Package txscript is licensed under the liberal ISC License.
