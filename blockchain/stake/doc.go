// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package stake contains code for all of dcrd's stake
transaction chain handling and other portions related to the
Proof-of-Stake (PoS) system.

At the heart of the PoS system are tickets, votes and revocations.
These 3 pieces work together to determine if previous blocks are
valid and their txs should be confirmed.

Important Parts included in stake package:

- Processing SSTx (tickets), SSGen (votes), SSRtx (revocations)
- TicketDB
- Stake Reward calculation
- Stake transaction identification (IsSStx, IsSSGen, IsSSRtx)


*/
package stake
