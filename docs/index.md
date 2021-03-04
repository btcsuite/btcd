# btcd

[![Build Status](https://github.com/btcsuite/btcd/workflows/Build%20and%20Test/badge.svg)](https://github.com/btcsuite/btcd/actions)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
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

## Documentation

Documentation is a work-in-progress. It is available at [btcd.readthedocs.io](https://btcd.readthedocs.io).

## Contents

* [Installation](installation.md)
* [Update](update.md)
* [Configuration](configuration.md)
* [Configuring TOR](configuring_tor.md)
* [Docker](using_docker.md)
* [Controlling](controlling.md)
* [Mining](mining.md)
* [Wallet](wallet.md)
* [Developer resources](developer_resources.md)
* [JSON RPC API](json_rpc_api.md)
* [Code contribution guidelines](code_contribution_guidelines.md)
* [Contact](contact.md)

## License

btcd is licensed under the [copyfree](http://copyfree.org) ISC License.

