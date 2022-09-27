btcd
====

[![Build Status](https://github.com/btcsuite/btcd/workflows/Build%20and%20Test/badge.svg)](https://github.com/btcsuite/btcd/actions)
[![Coverage Status](https://coveralls.io/repos/github/btcsuite/btcd/badge.svg?branch=master)](https://coveralls.io/github/btcsuite/btcd?branch=master)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/github.com/btcsuite/btcd)

btcd is an alternative full node bitcoin implementation written in Go (golang).

This project is currently under active development and is in a Beta state.  It
is extremely stable and has been in production use since October 2013.

It properly downloads, validates, and serves the block chain using the exact
rules (including consensus bugs) for block acceptance as Bitcoin Core.  We have
taken great care to avoid btcd causing a fork to the block chain.  It includes a
full block validation testing framework which contains all of the 'official'
block acceptance tests (and some additional ones) that is run on every pull
request to help ensure it properly follows consensus.  Also, it passes all of
the JSON test data in the Bitcoin Core code.

It also properly relays newly mined blocks, maintains a transaction pool, and
relays individual transactions that have not yet made it into a block.  It
ensures all individual transactions admitted to the pool follow the rules
required by the block chain and also includes more strict checks which filter
transactions based on miner requirements ("standard" transactions).

One key difference between btcd and Bitcoin Core is that btcd does *NOT* include
wallet functionality and this was a very intentional design decision.  See the
blog entry [here](https://web.archive.org/web/20171125143919/https://blog.conformal.com/btcd-not-your-moms-bitcoin-daemon)
for more details.  This means you can't actually make or receive payments
directly with btcd.  That functionality is provided by the
[btcwallet](https://github.com/btcsuite/btcwallet) and
[Paymetheus](https://github.com/btcsuite/Paymetheus) (Windows-only) projects
which are both under active development.

## Requirements

[Go](http://golang.org) 1.17 or newer.

## Installation

https://github.com/btcsuite/btcd/releases

#### Linux/BSD/MacOSX/POSIX - Build from Source

- Install Go according to the installation instructions here:
  http://golang.org/doc/install

- Ensure Go was installed properly and is a supported version:

```bash
$ go version
$ go env GOROOT GOPATH
```

NOTE: The `GOROOT` and `GOPATH` above must not be the same path.  It is
recommended that `GOPATH` is set to a directory in your home directory such as
`~/goprojects` to avoid write permission issues.  It is also recommended to add
`$GOPATH/bin` to your `PATH` at this point.

- Run the following commands to obtain btcd, all dependencies, and install it:

```bash
$ cd $GOPATH/src/github.com/btcsuite/btcd
$ go install -v . ./cmd/...
```

- btcd (and utilities) will now be installed in ```$GOPATH/bin```.  If you did
  not already add the bin directory to your system path during Go installation,
  we recommend you do so now.

## Updating

#### Linux/BSD/MacOSX/POSIX - Build from Source

- Run the following commands to update btcd, all dependencies, and install it:

```bash
$ cd $GOPATH/src/github.com/btcsuite/btcd
$ git pull
$ go install -v . ./cmd/...
```

## Getting Started

btcd has several configuration options available to tweak how it runs, but all
of the basic operations described in the intro section work with zero
configuration.

#### Linux/BSD/POSIX/Source

```bash
$ ./btcd
```

## IRC

- irc.libera.chat
- channel #btcd
- [webchat](https://web.libera.chat/gamja/?channels=btcd)

## Issue Tracker

The [integrated github issue tracker](https://github.com/btcsuite/btcd/issues)
is used for this project.

## Documentation

The documentation is a work-in-progress.  It is located in the [docs](https://github.com/btcsuite/btcd/tree/master/docs) folder.

## Release Verification

Please see our [documentation on the current build/verification
process](https://github.com/btcsuite/btcd/tree/master/release) for all our
releases for information on how to verify the integrity of published releases
using our reproducible build system.

## License

btcd is licensed under the [copyfree](http://copyfree.org) ISC License.
