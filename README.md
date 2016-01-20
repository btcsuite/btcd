dcrd
====

dcrd is a Decred full node implementation written in Go (golang).

This project is currently under active development and is in a Beta state.  It
is extremely stable and has been in production use for over 6 months as of May
2014, however there are still a couple of major features we want to add before
we come out of beta.

It properly downloads, validates, and serves the block chain using the exact
rules (including bugs) for block acceptance as Bitcoin Core.  We have taken
great care to avoid dcrd causing a fork to the block chain.  It passes all of
the 'official' block acceptance tests
(https://github.com/TheBlueMatt/test-scripts) as well as all of the JSON test
data in the Bitcoin Core code.

It also relays newly mined blocks, maintains a transaction pool, and relays
individual transactions that have not yet made it into a block.  It ensures all
transactions admitted to the pool follow the rules required by the block chain
and also includes the same checks which filter transactions based on
miner requirements ("standard" transactions) as Bitcoin Core.

One key difference between dcrd and Bitcoin Core is that dcrd does *NOT* include
wallet functionality and this was a very intentional design decision.  See the
blog entry [here](https://blog.conformal.com/dcrd-not-your-moms-bitcoin-daemon)
for more details.  This means you can't actually make or receive payments
directly with dcrd.  That functionality is provided by the
[dcrwallet](https://github.com/decred/dcrwallet) and
[btcgui](https://github.com/decred/btcgui) projects which are both under
active development.

## Requirements

[Go](http://golang.org) 1.3 or newer.

## Installation

#### Windows - MSI Available

https://github.com/decred/dcrd/releases

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
`~/goprojects` to avoid write permission issues.

- Run the following command to obtain dcrd, all dependencies, and install it:

```bash
$ go get -u github.com/decred/dcrd/...
```

- dcrd (and utilities) will now be installed in either ```$GOROOT/bin``` or
  ```$GOPATH/bin``` depending on your configuration.  If you did not already
  add the bin directory to your system path during Go installation, we
  recommend you do so now.

## Updating

#### Windows

Install a newer MSI

#### Linux/BSD/MacOSX/POSIX - Build from Source

- Run the following command to update dcrd, all dependencies, and install it:

```bash
$ go get -u -v github.com/decred/dcrd/...
```

## Getting Started

dcrd has several configuration options avilable to tweak how it runs, but all
of the basic operations described in the intro section work with zero
configuration.

#### Windows (Installed from MSI)

Launch dcrd from your Start menu.

#### Linux/BSD/POSIX/Source

```bash
$ ./dcrd
````

## IRC

- irc.freenode.net
- channel #decred
- [webchat](https://webchat.freenode.net/?channels=decred)

## Issue Tracker

The [integrated github issue tracker](https://github.com/decred/dcrd/issues)
is used for this project.

## Documentation

The documentation is a work-in-progress.  It is located in the [docs](https://github.com/decred/dcrd/tree/master/docs) folder.

## License

dcrd is licensed under the [copyfree](http://copyfree.org) ISC License.
