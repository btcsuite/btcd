# Installation

The first step is to install Bitcoin-Praxis (`praxisd`). See one of the following
sections for details on how to install on the supported operating systems.

## Requirements

[Go](https://go.dev) 1.25 or newer.

## Linux/BSD/MacOSX/POSIX — Build from Source

* Install Go according to the [installation instructions](https://go.dev/doc/install)
* Ensure Go was installed properly and is a supported version:

```bash
go version
go env GOROOT GOPATH
```

NOTE: The `GOROOT` and `GOPATH` above must not be the same path. It is
recommended that `GOPATH` is set to a directory in your home directory such as
`~/goprojects` to avoid write permission issues. It is also recommended to add
`$GOPATH/bin` to your `PATH` at this point.

* Clone and build:

```bash
git clone https://github.com/bitcoin-praxis/bitcoin-praxis.git
cd bitcoin-praxis
go build -o praxisd .
go build -o praxctl ./cmd/praxctl
# or: make build
```

* Optionally install into `$GOPATH/bin` / `$GOBIN`:

```bash
make install
```

The Go module path is still `github.com/btcsuite/btcd` for now; the user-facing
binaries are `praxisd` and `praxctl`.

## Windows

Build from source with Go as above (`go build -o praxisd.exe .`). MSI installers
from upstream btcd do not apply to this fork.

## Upstream release verification (optional)

Upstream btcd release tags are signed by Conformal. Bitcoin-Praxis releases (when
published) will document their own verification process. For verifying upstream
btcd tags, see the [Conformal GPG key](https://raw.githubusercontent.com/btcsuite/btcd/master/release/GIT-GPG-KEY-conformal.txt)
and `git tag -v TAG_NAME` in a btcsuite/btcd checkout.

## Startup

Typically `praxisd` will run and start downloading the blockchain with no extra
configuration. Default data directory: `~/.praxisd/`.

There is an optional method to use a `bootstrap.dat` file that may speed up the
initial blockchain download:

* [Using bootstrap.dat](configuration.md#using-bootstrapdat)
