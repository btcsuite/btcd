// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
The netsync package implements a concurrency safe block syncing protocol. The
provided implementation of SyncManager communicates with connected peers to
perform an initial block download, keep the chain and mempool in sync, and
announce new blocks connected to the chain. Currently the block manager selects
a single sync peer that it downloads all blocks from until it is up to date with
the longest chain the sync peer is aware of.
*/
package netsync
