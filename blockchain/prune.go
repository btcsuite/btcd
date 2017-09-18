package blockchain

import (
	"time"
)

// pruningIntervalInMinutes is the interval in which to prune the blockchain's
// nodes and restore memory to the garbage collector.
const pruningIntervalInMinutes = 5

// chainPruner is used to occasionally prune the blockchain of old nodes that
// can be freed to the garbage collector.
type chainPruner struct {
	chain              *BlockChain
	lastNodeInsertTime time.Time
}

// newChainPruner returns a new chain pruner.
func newChainPruner(chain *BlockChain) *chainPruner {
	return &chainPruner{
		chain:              chain,
		lastNodeInsertTime: time.Now(),
	}
}

// pruneChainIfNeeded checks the current time versus the time of the last pruning.
// If the blockchain hasn't been pruned in this time, it initiates a new pruning.
//
// pruneChainIfNeeded must be called with the chainLock held for writes.
func (c *chainPruner) pruneChainIfNeeded() {
	now := time.Now()
	duration := now.Sub(c.lastNodeInsertTime)
	if duration < time.Minute*pruningIntervalInMinutes {
		return
	}

	c.lastNodeInsertTime = now
	c.chain.pruneStakeNodes()
}
