netsync
=======

[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

## Overview

This package implements a concurrency safe block syncing protocol. The
SyncManager communicates with connected peers to perform an initial block
download, keep the chain and unconfirmed transaction pool in sync, and announce
new blocks connected to the chain. Currently the sync manager selects a single
sync peer that it downloads all blocks from until it is up to date with the
longest chain the sync peer is aware of.
