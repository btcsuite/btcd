utcd
====

[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

utcd is a fork of btcd, an alternative full node bitcoin implementation written in Go (golang).
utcd implements the Utreexo accumulator into btcd.

This project is currently under active development, but is a work in progress.
The current release is only a demo release and should be treated like so.

## Requirements

[Go](http://golang.org) 1.14 or newer.

## Installation

Download from the releases tab on Github

## Getting Started

utcd only supports utreexocsn mode and will only connect to the designated
nodes that we have set up. To run a bridge node and connect to it, you
must modify/build from source.

#### Linux/BSD/POSIX/Source

```bash
$ ./btcd
```

## IRC

- irc.freenode.net
- channel #utreexo
- [webchat](https://webchat.freenode.net/?channels=btcd)

## License

utcd is licensed under the [copyfree](http://copyfree.org) ISC License.
