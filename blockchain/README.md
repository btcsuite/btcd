blockchain
==========

[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

### Bitcoin Chain Processing Overview

Before a block is allowed into the block chain, it must go through an intensive
series of validation rules.  The following list serves as a general outline of
those rules to provide some intuition into what is going on under the hood, but
is by no means exhaustive:

 - Reject duplicate blocks
 - Perform a series of sanity checks on the block and its transactions such as
   verifying proof of work, timestamps, number and character of transactions,
   transaction amounts, script complexity, and merkle root calculations
 - Compare the block against predetermined checkpoints for expected timestamps
   and difficulty based on elapsed time since the checkpoint
 - Save the most recent orphan blocks for a limited time in case their parent
   blocks become available
 - Stop processing if the block is an orphan as the rest of the processing
   depends on the block's position within the block chain
 - Perform a series of more thorough checks that depend on the block's position
   within the block chain such as verifying block difficulties adhere to
   difficulty retarget rules, timestamps are after the median of the last
   several blocks, all transactions are finalized, checkpoint blocks match, and
   block versions are in line with the previous blocks
 - Determine how the block fits into the chain and perform different actions
   accordingly in order to ensure any side chains which have higher difficulty
   than the main chain become the new main chain
 - When a block is being connected to the main chain (either through
   reorganization of a side chain to the main chain or just extending the
   main chain), perform further checks on the block's transactions such as
   verifying transaction duplicates, script complexity for the combination of
   connected scripts, coinbase maturity, double spends, and connected
   transaction values
 - Run the transaction scripts to verify the spender is allowed to spend the
   coins
 - Insert the block into the block database