# Bitcoin-Praxis

[![Build Status](https://github.com/bitcoin-praxis/bitcoin-praxis/workflows/Build%20and%20Test/badge.svg)](https://github.com/bitcoin-praxis/bitcoin-praxis/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

**Bitcoin-Praxis** is a Bitcoin full node forked from
[btcd](https://github.com/btcsuite/btcd). The daemon binary is **`praxisd`**.

It downloads, validates, and serves the blockchain using the same consensus
rules as Bitcoin Core (including consensus bugs), inherited from btcd's long
production track record since October 2013. Consensus safety remains the hard
gate: `blockchain/fullblocktests` must pass with identical outcomes.

## What is different

- **Witness-separated cold storage (M1, shipped):** past a rolling
  `--witness-buffer` (default 2016), witness is excised and the stripped block
  is zstd-compressed. Measured **52.5%** smaller on a 1005 GB mainnet chain
  (~477 GB). Distinct from `--prune`, which deletes whole blocks.
- **Roadmap:** parallel IBD validation (M2), in-process wallet +
  **Bitcoin-Praxis Wallet** GUI (M3), cross-platform async I/O (M4), DATUM /
  Stratum v2 mining (M5). See [docs/ROADMAP.md](docs/ROADMAP.md).

The Go module path is still `github.com/btcsuite/btcd` for now; user-facing
names are Bitcoin-Praxis / `praxisd`.

## Requirements

[Go](https://go.dev) 1.25 or newer.

## Build

```bash
$ git clone https://github.com/bitcoin-praxis/bitcoin-praxis.git
$ cd bitcoin-praxis
$ go build -o praxisd .
$ go build -o btcctl ./cmd/btcctl
```

Or `make build` (emits `praxisd`).

## Run

```bash
$ ./praxisd
```

Default data directory: `~/.praxisd/` (config: `~/.praxisd/praxisd.conf`).

If you previously used upstream btcd's `~/.btcd/`, data is **not** migrated
automatically — copy, symlink, or re-sync.

Useful flags:

```bash
$ ./praxisd --witness-buffer=2016   # default: hot window with full witness
$ ./praxisd --witness-buffer=0      # full archival (keep witness forever)
$ ./praxisd --prune=1536            # Core-style prune; forces witness-buffer off
```

## Documentation

| Doc | Contents |
|-----|----------|
| [docs/ROADMAP.md](docs/ROADMAP.md) | Milestones M1–M5 |
| [docs/M1_TEST_PLAN.md](docs/M1_TEST_PLAN.md) | M1 test matrix + **tests we ran** |
| [docs/configuration.md](docs/configuration.md) | Config / listen options |
| [docs/](docs/) | Full docs hub |

## Tests

```bash
$ go test ./... -test.timeout=20m
```

See [docs/M1_TEST_PLAN.md](docs/M1_TEST_PLAN.md) for the witness-storage matrix,
fuzz targets, mainnet measurement, IBD, and LN soak notes.

## Fund

- **Lightning:** `lively-reindeer-84@rizful.com`
- **On-chain:** `bc1psksl57x4nlpn08qse899jruk7t65jgj7n72gd835urgsqyu20acsq9408c`

The GitHub Sponsor button also lists these via
[`.github/FUNDING.yml`](.github/FUNDING.yml).

## Upstream

Bitcoin-Praxis inherits btcd's ISC license and consensus test suite. Upstream:
[btcsuite/btcd](https://github.com/btcsuite/btcd).

## License

Bitcoin-Praxis is licensed under the [copyfree](http://copyfree.org) ISC License
(inherited from btcd).
