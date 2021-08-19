# lbcd

[![Build Status](https://github.com/lbryio/lbcd/workflows/Build%20and%20Test/badge.svg)](https://github.com/lbryio/lbcd/actions)
[![Coverage Status](https://coveralls.io/repos/github/lbryio/lbcd/badge.svg?branch=master)](https://coveralls.io/github/lbryio/lbcd?branch=master)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
<!--[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/github.com/lbryio/lbcd)-->

`lbcd` is a full node implementation of LBRY's blockchain written in Go (golang).

This project is currently under active development and is in a Beta state while
we ensure it matches LBRYcrd's functionality. The intention is that it properly
downloads, validates, and serves the block chain using the exact rules
(including consensus bugs) for block acceptance as LBRYcrd.
We have taken great care to avoid lbcd causing a fork to the blockchain.

Note: `lbcd` does *NOT* include wallet functionality.  That functionality is provided by the
[lbcwallet](https://github.com/lbryio/lbcwallet) and the [LBRY SDK](https://github.com/lbryio/lbry-sdk).

## Security

We take security seriously. Please contact [security](mailto:security@lbry.com) regarding any security issues.
Our PGP key is [here](https://lbry.com/faq/pgp-key) if you need it.

We maintain a mailing list for notifications of upgrades, security issues,
and soft/hard forks. To join, visit [fork list](https://lbry.com/forklist)

## Requirements

All common operating systems are supported. lbcd requires at least 8GB of RAM
and at least 100GB of disk storage. Both RAM and disk requirements increase slowly over time.
Using a fast NVMe disk is recommended.

`lbcd` is not immune to data loss. It expects a clean shutdown via SIGINT or
SIGTERM. SIGKILL, immediate VM kills, and sudden power loss can cause data
corruption, thus requiring chain resynchronization for recovery.

For compilation, [Go](http://golang.org) 1.16 or newer is required.

## Installation

Acquire binary files from [releases](https://github.com/lbryio/lbcd/releases)

### To build from Source on Linux/BSD/MacOSX/POSIX

Install Go according to its [installation instructions](http://golang.org/doc/install).

``` sh
git clone https://github.com/lbryio/lbcd
cd lbcd

# Build lbcd
go build .

# Build lbcctl
go build ./cmd/lbcctl
```

Both [GoLand](https://www.jetbrains.com/go/)
and [VS Code](https://code.visualstudio.com/docs/languages/go) IDEs are supported.

## Usage

By default, data and logs are stored in `<LBCDDIR>`:

- Linux: `~/.lbcd/`
- MacOS: `/Users/<username>/Library/Application Support/Lbcd/`

To enable RPC access a username and password is required. Example:

``` sh
./lbcd --rpcuser=rpcuser --rpcpass=rpcpass
```

Interact with lbcd via RPC using `lbcctl`

``` sh
./lbcctl --rpcuser=rpcuser --rpcpass=rpcpass getblockcount
./lbcctl --rpcuser=rpcuser --rpcpass=rpcpass getblocktemplate
```

By default, the RPCs are served over TLS. `lbcd` generates (if not exists) `rpc.cert` and
`rpc.key` under `<LBCDDIR>` where `lbcctl` would search and use them.

The RPCs can also be served without TLS *(on localhost only)* using (`--notls`)

``` sh
./lbcd --rpcuser=rpcuser --rpcpass=rpcpass --notls
./lbcctl --rpcuser=rpcuser --rpcpass=rpcpass --notls getblockcount
```

## Working with Different Networks

By default, `lbcd` and `lbcctl` use the following ports for different networks respectively:

| Network | RPC Port | Network Port |
| ------- | -------- | ------------ |
| mainnet | 9245     | 9246         |
| testnet | 19245    | 19246        |
| regtest | 29245    | 29246        |

Running `lbcd` and `lbcctl` with `--testnet` or `--regtest` would use different chain params as well as default RPC and Network ports.

``` sh
./lbcd --rpcuser=rpcuser --rpcpass=rpcpass --regtest
./lbcctl --rpcuser=rpcuser --rpcpass=rpcpass --regtest getblockcount
```

The default Network and RPC ports of `lbcd` can be overriden using `--listen` and `--rpclisten`
`lbcctl` can also connect to RPC server specified by `--rpcserver`

``` sh
./lbcd --rpcuser=rpcuser --rpcpass=rpcpass --regtest --listen=127.0.0.1:29248 --rpclisten=127.0.0.1:29247
./lbcctl --rpcuser=rpcuser --rpcpass=rpcpass --regtest --rpcserver=127.0.0.1:29247 getblockcount
```

Note: Wallet related RPCs are provided by [lbcwallet](https://github.com/lbryio/lbcwallet).

## Contributing

Contributions to this project are welcome, encouraged, and compensated.
The [integrated github issue tracker](https://github.com/lbryio/lbcd/issues)
is used for this project. All pull requests will be considered.

<!-- ## Release Verification
Please see our [documentation on the current build/verification
process](https://github.com/lbryio/lbcd/tree/master/release) for all our
releases for information on how to verify the integrity of published releases
using our reproducible build system.
-->

## License

lbcd is licensed under the [copyfree](http://copyfree.org) ISC License.
