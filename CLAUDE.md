# btcd

## Project Overview

btcd is an alternative full-node Bitcoin implementation written in Go. It properly downloads, validates, and serves the blockchain using the exact rules (including consensus bugs) as Bitcoin Core. It does **not** include wallet functionality (that's handled by [btcwallet](https://github.com/btcsuite/btcwallet)). Licensed under ISC.

## Architecture

The project is a multi-module Go workspace with the main `btcd` daemon at the root and several submodules:

### Main Daemon (root package `main`)

Entry points and core server logic:
- `btcd.go` — Main entry point, initialization, and shutdown
- `server.go` — Core full-node server (`server` struct), peer management, block/tx handling
- `config.go` — CLI flag parsing and configuration loading
- `rpcserver.go` — JSON-RPC server implementation
- `rpcwebsocket.go` — WebSocket notifications for the RPC server
- `netsync/` — Block chain synchronization logic
- `peer/` — Bitcoin p2p peer protocol implementation
- `connmgr/` — Connection manager for outbound peer connections

### Core Packages

| Package | Purpose |
|---------|---------|
| `blockchain/` | Block chain validation, chain state management |
| `blockchain/indexers/` | Optional indexes (txindex, addrindex, cfindex) |
| `mempool/` | Transaction memory pool and policy |
| `mining/` | Block template generation and mining |
| `txscript/` | Bitcoin transaction script parsing, evaluation, and creation |
| `btcec/` | Elliptic curve cryptography (secp256k1), separately versioned as `btcec/v2` |
| `btcutil/` | Bitcoin utility types (addresses, amounts, blocks) |
| `btcutil/psbt/` | Partially Signed Bitcoin Transactions (PSBT) |
| `chaincfg/` | Network parameters for mainnet, testnet, signet, regtest, simnet |
| `chaincfg/chainhash/` | Block/tx hash type |
| `wire/` | Bitcoin wire protocol message serialization |
| `database/` | Database backend interfaces and drivers |
| `btcjson/` | JSON-RPC command/result types |
| `rpcclient/` | Client for the JSON-RPC API |
| `addrmgr/` | Address manager (peer address book) |
| `v2transport/` | Transport protocol v2 (BIP324) support |

### CLI Tools (`cmd/`)

- `btcctl` — CLI client for the JSON-RPC API
- `addblock` — Utility to manually add blocks
- `findcheckpoint` — Checkpoint lookup utility
- `gencerts` — TLS certificate generation for RPC

### Integration Tests (`integration/`)

End-to-end tests using `rpctest` framework. These require a running btcd instance.

## Build & Development

```bash
# Build all binaries (outputs to project directory)
make build

# Install all binaries to $GOBIN
make install

# Install release binaries (statically linked, stripped)
make release-install

# Run unit tests (all modules, uses rpctest tag)
make unit

# Run unit tests with coverage
make unit-cover

# Run unit tests with race detector (requires CGO)
make unit-race

# Format and fix imports
make fmt

# Lint with golangci-lint
make lint

# Run go mod tidy for all modules
make tidy-module

# Clean generated files
make clean
```

### Testing Individual Packages

```bash
# Run tests for a specific package
go test -v -tags=rpctest ./blockchain/...

# Run tests for submodules
cd btcec && go test -v -tags=rpctest ./...
cd btcutil && go test -v -tags=rpctest ./...
```

CI runs `make build`, `make unit-cover`, and `make unit-race` on Go 1.22.

## Configuration

- Config file: `~/.btcd/btcd.conf` (POSIX) or `%LOCALAPPDATA%\btcd\btcd.conf` (Windows)
- Sample config: `sample-btcd.conf`
- Networks: `--testnet`, `--regtest`, `--simnet`, or `--signet` flags
- RPC: disabled by default; enable with `--rpcuser` and `--rpcpass`

## Key Conventions

### Code Style
- **80 character line limit** — tabs count as 8 spaces. This is strictly enforced.
- Follow [Effective Go](https://go.dev/doc/effective_go) guidelines.
- Format with `goimports` and `gofmt -s`.
- Lint with `golangci-lint v2` (config in `.golangci.yml`).
- Segment code into logical stanzas separated by blank lines.
- Use spacing between `case`/`select` stanzas.

### Documentation
- Every function must have a comment starting with the function name.
- Exported functions need detailed caller-facing documentation.
- In-body comments explain the *why* and *intention*, not the *what*.
- Full style guide: `.gemini/styleguide.md`, `docs/code_formatting_rules.md`.

### Testing
- Use `require` library from `testify` in unit tests.
- Prefer table-driven tests or `rapid` property-based tests.
- New features require tests for positive and negative paths.
- Bug fixes must include regression tests.

### Git Commits
- Format: `subsystem: short description` (under 50 chars)
- Use the package name as subsystem (e.g., `peer:`, `rpcclient:`, `multi:`)
- Present tense ("Fix bug", not "Fixed bug")
- Body wrapped at 72 characters

### Error Handling
- Errors are values — return them, don't panic (except for truly unrecoverable conditions).
- Use `fmt.Errorf("context: %w", err)` for error wrapping.

## Dependencies

- Go 1.22+ (CI uses 1.22.11, `go.mod` specifies 1.23.2)
- Key deps: `decred/dcrd/dcrec/secp256k1/v4`, `gorilla/websocket`, `syndtr/goleveldb`, `pgregory.net/rapid`
