btcd
====

[![Build Status](https://travis-ci.org/conformal/btcd.png?branch=master)]
(https://travis-ci.org/conformal/btcd)

btcd is an alternative full node bitcoin implementation written in Go (golang).

This project is currently under active development and is in an Alpha state.

It currently properly downloads, validates, and serves the block chain using the
exact rules (including bugs) for block acceptance as the reference
implementation (bitcoind).  We have taken great care to avoid btcd causing a
fork to the block chain.  It passes all of the 'official' block acceptance tests
(https://github.com/TheBlueMatt/test-scripts).

It also properly relays newly mined blocks, maintains a transaction pool,
and relays individual transactions that have not yet made it into a block.  It
ensures all individual transactions admitted to the pool follow the rules
required into the block chain and also includes the vast majority of the more
strict checks which filter transactions based on miner requirements ("standard"
transactions).

One key difference between btcd and bitcoind is that btcd does *NOT* include
wallet functionality and this was a very intentional design decision. See the
blog entry [here](https://blog.conformal.com/btcd-not-your-moms-bitcoin-daemon)
for more details.  This means you can't actually make or receive payments
directly with btcd.  That functionality will be provided by the forthcoming
btcwallet and btcgui.

## Installation

#### Windows - MSI Available

https://github.com/conformal/btcd/releases

#### Linux/BSD/POSIX - Build from Source

- Install Go according to the installation instructions here:
  http://golang.org/doc/install
  btcd requires features only available in go1.2 or later.

- Run the following command to obtain btcd, all dependencies, and install it:
  ```$ go get github.com/conformal/btcd```

- btcd will now be installed in either ```$GOROOT/bin``` or ```$GOPATH/bin```
  depending on your configuration.  If you did not already add to your system
  path during the installation, we recommend you do so now.

## Updating

#### Windows

Install a newer MSI

#### Linux/BSD/POSIX - Build from Source

- Run the following command to update btcd, all dependencies, and install it:
  ```$ go get -u -v github.com/conformal/btcd/...```

## Getting Started

btcd has several configuration options avilable to tweak how it runs, but all
of the basic operations described in the intro section work with zero
configuration.

#### Windows (Installed from MSI)

Launch btcd from your Start menu.

#### Linux/BSD/POSIX/Source

```bash
$ ./btcd
````
## Mailing lists

- btcd: discussion of btcd and its packages.
- btcd-commits: readonly mail-out of source code changes.

To subscribe to a given list, send email to list+subscribe@opensource.conformal.com

## TODO

The following is a brief overview of the next things we have planned to work on
for btcd.  Note this does not include the separate btcwallet and btcgui which
are currently under heavy development:

- Documentation
- Code cleanup
- Add remaining missing RPC calls
- Complete several TODO items in the code
- Offer cross-compiled binaries for popular OSes (Fedora, Ubuntu, FreeBSD, OpenBSD)

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

btcd is licensed under the liberal ISC License.
