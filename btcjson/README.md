btcjson
=======

[![Build Status](https://travis-ci.org/btcsuite/btcd.png?branch=master)]
(https://travis-ci.org/btcsuite/btcd) [![ISC License]
(http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

Package btcjson implements concrete types for marshalling to and from the
bitcoin JSON-RPC API.  A comprehensive suite of tests is provided to ensure
proper functionality.  Package btcjson is licensed under the copyfree ISC
license.

Although this package was primarily written for btcd, it has intentionally been
designed so it can be used as a standalone package for any projects needing to
marshal to and from bitcoin JSON-RPC requests and responses.

Note that although it's possible to use this package directly to implement an
RPC client, it is not recommended since it is only intended as an infrastructure
package.  Instead, RPC clients should use the
[btcrpcclient](https://github.com/btcsuite/btcrpcclient) package which provides
a full blown RPC client with many features such as automatic connection
management, websocket support, automatic notification re-registration on
reconnect, and conversion from the raw underlying RPC types (strings, floats,
ints, etc) to higher-level types with many nice and useful properties.

## JSON RPC

Bitcoin provides an extensive API call list to control the chain and wallet
servers through JSON-RPC.  These can be used to get information from the server
or to cause the server to perform some action.

The general form of the commands are:

```JSON
	{"jsonrpc": "1.0", "id":"test", "method": "getinfo", "params": []}
```

btcjson provides code to easily create these commands from go (as some of the
commands can be fairly complex), to send the commands to a running bitcoin RPC
server, and to handle the replies (putting them in useful Go data structures).

## Sample Use

```Go
	// Create a new command.
	cmd, err := btcjson.NewGetBlockCountCmd()
	if err != nil {
		// Handle error
	}

	// Marshal the command to a JSON-RPC formatted byte slice.
	marshalled, err := btcjson.MarshalCmd(id, cmd)
	if err != nil {
		// Handle error
	}

	// At this point marshalled contains the raw bytes that are ready to send
	// to the RPC server to issue the command.
	fmt.Printf("%s\n", marshalled)
```

## Documentation

[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)]
(http://godoc.org/github.com/btcsuite/btcd/btcjson)

Full `go doc` style documentation for the project can be viewed online without
installing this package by using the GoDoc site
[here](http://godoc.org/github.com/btcsuite/btcd/btcjson).

You can also view the documentation locally once the package is installed with
the `godoc` tool by running `godoc -http=":6060"` and pointing your browser to
http://localhost:6060/pkg/github.com/btcsuite/btcd/btcjson

## Installation

```bash
$ go get github.com/btcsuite/btcd/btcjson
```

## GPG Verification Key

All official release tags are signed by Conformal so users can ensure the code
has not been tampered with and is coming from Conformal.  To verify the
signature perform the following:

- Download the public key from the Conformal website at
  https://opensource.conformal.com/GIT-GPG-KEY-conformal.txt

- Import the public key into your GPG keyring:
  ```bash
  gpg --import GIT-GPG-KEY-conformal.txt
  ```

- Verify the release tag with the following command where `TAG_NAME` is a
  placeholder for the specific tag:
  ```bash
  git tag -v TAG_NAME
  ```

## License

Package btcjson is licensed under the [copyfree](http://copyfree.org) ISC
License.
