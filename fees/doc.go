// Copyright (c) 2018-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package fees provides decred-specific methods for tracking and estimating fee
rates for new transactions to be mined into the network. Fee rate estimation has
two main goals:

- Ensuring transactions are mined within a target _confirmation range_
  (expressed in blocks);
- Attempting to minimize fees while maintaining be above restriction.

Preliminaries

There are two main regimes against which fee estimation needs to be evaluated
according to how full blocks being mined are (and consequently how important fee
rates are): _low contention_ and _high contention_:

In a low contention regime, the mempool sits mostly empty, transactions are
usually mined very soon after being published and transaction fees are mostly
sent using the minimum relay fee.

In a high contention regime, the mempool is usually filled with unmined
transactions, there is active dispute for space in a block (by transactions
using higher fees) and blocks are usually full.

The exact point of where these two regimes intersect is arbitrary, but it should
be clear in the examples and simulations which of these is being discussed.

Note: a very high contention scenario (> 90% of blocks being full and
transactions remaining in the mempool indefinitely) is one in which stakeholders
should be discussing alternative solutions (increase block size, provide other
second layer alternatives, etc). Also, the current fill rate of blocks in decred
is low, so while we try to account for this regime, I personally expect that the
implementation will need more tweaks as it approaches this.

The current approach to implement this estimation is based on bitcoin core's
algorithm. References [1] and [2] provide a high level description of how it
works there. Actual code is linked in references [3] and [4].

Outline of the Algorithm

The algorithm is currently based in fee estimation as used in v0.14 of bitcoin
core (which is also the basis for the v0.15+ method). A more comprehensive
overview is available in reference [1].

This particular version was chosen because it's simpler to implement and should
be sufficient for low contention regimes. It probably overestimates fees in
higher contention regimes and longer target confirmation windows, but as pointed
out earlier should be sufficient for current fill rate of decred's network.

The basic algorithm is as follows (as executed by a single full node):

Stats building stage:

- For each transaction observed entering mempool, record the block at which it
  was first seen
- For each mined transaction which was previously observed to enter the mempool,
  record how long (in blocks) it took to be mined and its fee rate
- Group mined transactions into fee rate _buckets_  and _confirmation ranges_,
  creating a table of how many transactions were mined at each confirmation
  range and fee rate bucket and their total committed fee
- Whenever a new block is mined, decay older transactions to account for a
  dynamic fee environment

Estimation stage:

- Input a target confirmation range (how many blocks to wait for the tx to be
  mined)
- Starting at the highest fee bucket, look for buckets where the chance of
  confirmation within the desired confirmation window is > 95%
- Average all such buckets to get the estimated fee rate

Simulation

Development of the estimator was originally performed and simulated using the
code in [5]. Simulation of the current code can be performed by using the
dcrfeesim tool available in [6].

Acknowledgements

Thanks to @davecgh for providing the initial review of the results and the
original developers of the bitcoin core code (the brunt of which seems to have
been made by @morcos).

## References

[1] Introduction to Bitcoin Core Estimation:
https://bitcointechtalk.com/an-introduction-to-bitcoin-core-fee-estimation-27920880ad0

[2] Proposed Changes to Fee Estimation in version 0.15:
https://gist.github.com/morcos/d3637f015bc4e607e1fd10d8351e9f41

[3] Source for fee estimation in v0.14:
https://github.com/bitcoin/bitcoin/blob/v0.14.2/src/policy/fees.cpp

[4] Source for fee estimation in version 0.16.2:
https://github.com/bitcoin/bitcoin/blob/v0.16.2/src/policy/fees.cpp

[5] Source for the original dcrfeesim and estimator work:
https://github.com/matheusd/dcrfeesim_dev

[6] Source for the current dcrfeesim, using this module:
https://github.com/matheusd/dcrfeesim
*/
package fees
