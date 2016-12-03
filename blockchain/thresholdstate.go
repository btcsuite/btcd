// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// ThresholdState define the various threshold states used when voting on
// consensus changes.
type ThresholdState byte

// These constants are used to identify specific threshold states.
//
// NOTE: This section specifically does not use iota for the individual states
// since these values are serialized and must be stable for long-term storage.
const (
	// ThresholdDefined is the first state for each deployment and is the
	// state for the genesis block has by defintion for all deployments.
	ThresholdDefined ThresholdState = 0

	// ThresholdStarted is the state for a deployment once its start time
	// has been reached.
	ThresholdStarted ThresholdState = 1

	// ThresholdLockedIn is the state for a deployment during the retarget
	// period which is after the ThresholdStarted state period and the
	// number of blocks that have voted for the deployment equal or exceed
	// the required number of votes for the deployment.
	ThresholdLockedIn ThresholdState = 2

	// ThresholdActive is the state for a deployment for all blocks after a
	// retarget period in which the deployment was in the ThresholdLockedIn
	// state.
	ThresholdActive ThresholdState = 3

	// ThresholdFailed is the state for a deployment once its expiration
	// time has been reached and it did not reach the ThresholdLockedIn
	// state.
	ThresholdFailed ThresholdState = 4

	// numThresholdsStates is the maximum number of threshold states used in
	// tests.
	numThresholdsStates = iota
)

// thresholdStateStrings is a map of ThresholdState values back to their
// constant names for pretty printing.
var thresholdStateStrings = map[ThresholdState]string{
	ThresholdDefined:  "ThresholdDefined",
	ThresholdStarted:  "ThresholdStarted",
	ThresholdLockedIn: "ThresholdLockedIn",
	ThresholdActive:   "ThresholdActive",
	ThresholdFailed:   "ThresholdFailed",
}

// String returns the ThresholdState as a human-readable name.
func (t ThresholdState) String() string {
	if s := thresholdStateStrings[t]; s != "" {
		return s
	}
	return fmt.Sprintf("Unknown ThresholdState (%d)", int(t))
}

// thresholdConditionChecker provides a generic interface that is invoked to
// determine when a consensus rule change threshold should be changed.
type thresholdConditionChecker interface {
	// BeginTime returns the unix timestamp for the median block time after
	// which voting on a rule change starts (at the next window).
	BeginTime() uint64

	// EndTime returns the unix timestamp for the median block time after
	// which an attempted rule change fails if it has not already been
	// locked in or activated.
	EndTime() uint64

	// RuleChangeActivationThreshold is the number of blocks for which the
	// condition must be true in order to lock in a rule change.
	RuleChangeActivationThreshold() uint32

	// MinerConfirmationWindow is the number of blocks in each threshold
	// state retarget window.
	MinerConfirmationWindow() uint32

	// Condition returns whether or not the rule change activation condition
	// has been met.  This typically involves checking whether or not the
	// bit assocaited with the condition is set, but can be more complex as
	// needed.
	Condition(*blockNode) (bool, error)
}

// thresholdStateCache provides a type to cache the threshold states of each
// threshold window for a set of IDs.  It also keeps track of which entries have
// been modified and therefore need to be written to the database.
type thresholdStateCache struct {
	dbUpdates map[chainhash.Hash]ThresholdState
	entries   map[chainhash.Hash]ThresholdState
}

// Lookup returns the threshold state associated with the given hash along with
// a boolean that indicates whether or not it is valid.
func (c *thresholdStateCache) Lookup(hash chainhash.Hash) (ThresholdState, bool) {
	state, ok := c.entries[hash]
	return state, ok
}

// Update updates the cache to contain the provided hash to threshold state
// mapping while properly tracking needed updates flush changes to the database.
func (c *thresholdStateCache) Update(hash chainhash.Hash, state ThresholdState) {
	if existing, ok := c.entries[hash]; ok && existing == state {
		return
	}

	c.dbUpdates[hash] = state
	c.entries[hash] = state
}

// MarkFlushed marks all of the current udpates as flushed to the database.
// This is useful so the caller can ensure the needed database updates are not
// lost until they have successfully been written to the database.
func (c *thresholdStateCache) MarkFlushed() {
	for hash := range c.dbUpdates {
		delete(c.dbUpdates, hash)
	}
}

// newThresholdCaches returns a new array of caches to be used when calculating
// threshold states.
func newThresholdCaches(numCaches uint32) []thresholdStateCache {
	caches := make([]thresholdStateCache, numCaches)
	for i := 0; i < len(caches); i++ {
		caches[i] = thresholdStateCache{
			entries:   make(map[chainhash.Hash]ThresholdState),
			dbUpdates: make(map[chainhash.Hash]ThresholdState),
		}
	}
	return caches
}

// thresholdState returns the current rule change threshold state for the block
// AFTER the given node and deployment ID.  The cache is used to ensure the
// threshold states for previous windows are only calculated once.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) thresholdState(prevNode *blockNode, checker thresholdConditionChecker, cache *thresholdStateCache) (ThresholdState, error) {
	// The threshold state for the window that contains the genesis block is
	// defined by definition.
	confirmationWindow := int32(checker.MinerConfirmationWindow())
	if prevNode == nil || (prevNode.height+1) < confirmationWindow {
		return ThresholdDefined, nil
	}

	// Get the ancestor that is the last block of the previous confirmation
	// window in order to get its threshold state.  This can be done because
	// the state is the same for all blocks within a given window.
	var err error
	prevNode, err = b.ancestorNode(prevNode, prevNode.height-
		(prevNode.height+1)%confirmationWindow)
	if err != nil {
		return ThresholdFailed, err
	}

	// Iterate backwards through each of the previous confirmation windows
	// to find the most recently cached threshold state.
	var neededStates []*blockNode
	for prevNode != nil {
		// Nothing more to do if the state of the block is already
		// cached.
		if _, ok := cache.Lookup(*prevNode.hash); ok {
			break
		}

		// The start and expiration times are based on the median block
		// time, so calculate it now.
		medianTime, err := b.calcPastMedianTime(prevNode)
		if err != nil {
			return ThresholdFailed, err
		}

		// The state is simply defined if the start time hasn't been
		// been reached yet.
		if uint64(medianTime.Unix()) < checker.BeginTime() {
			cache.Update(*prevNode.hash, ThresholdDefined)
			break
		}

		// Add this node to the list of nodes that need the state
		// calculated and cached.
		neededStates = append(neededStates, prevNode)

		// Get the ancestor that is the last block of the previous
		// confirmation window.
		prevNode, err = b.ancestorNode(prevNode, prevNode.height-
			confirmationWindow)
		if err != nil {
			return ThresholdFailed, err
		}
	}

	// Start with the threshold state for the most recent confirmation
	// window that has a cached state.
	state := ThresholdDefined
	if prevNode != nil {
		var ok bool
		state, ok = cache.Lookup(*prevNode.hash)
		if !ok {
			return ThresholdFailed, AssertError(fmt.Sprintf(
				"thresholdState: cache lookup failed for %v",
				prevNode.hash))
		}
	}

	// Since each threshold state depends on the state of the previous
	// window, iterate starting from the oldest unknown window.
	for neededNum := len(neededStates) - 1; neededNum >= 0; neededNum-- {
		prevNode := neededStates[neededNum]

		switch state {
		case ThresholdDefined:
			// The deployment of the rule change fails if it expires
			// before it is accepted and locked in.
			medianTime, err := b.calcPastMedianTime(prevNode)
			if err != nil {
				return ThresholdFailed, err
			}
			medianTimeUnix := uint64(medianTime.Unix())
			if medianTimeUnix >= checker.EndTime() {
				state = ThresholdFailed
				break
			}

			// The state for the rule moves to the started state
			// once its start time has been reached (and it hasn't
			// already expired per the above).
			if medianTimeUnix >= checker.BeginTime() {
				state = ThresholdStarted
			}

		case ThresholdStarted:
			// The deployment of the rule change fails if it expires
			// before it is accepted and locked in.
			medianTime, err := b.calcPastMedianTime(prevNode)
			if err != nil {
				return ThresholdFailed, err
			}
			if uint64(medianTime.Unix()) >= checker.EndTime() {
				state = ThresholdFailed
				break
			}

			// At this point, the rule change is still being voted
			// on by the miners, so iterate backwards through the
			// confirmation window to count all of the votes in it.
			var count uint32
			countNode := prevNode
			for i := int32(0); i < confirmationWindow; i++ {
				condition, err := checker.Condition(countNode)
				if err != nil {
					return ThresholdFailed, err
				}
				if condition {
					count++
				}

				// Get the previous block node.  This function
				// is used over simply accessing countNode.parent
				// directly as it will dynamically create
				// previous block nodes as needed.  This helps
				// allow only the pieces of the chain that are
				// needed to remain in memory.
				countNode, err = b.getPrevNodeFromNode(countNode)
				if err != nil {
					return ThresholdFailed, err
				}
			}

			// The state is locked in if the number of blocks in the
			// period that voted for the rule change meets the
			// activation threshold.
			if count >= checker.RuleChangeActivationThreshold() {
				state = ThresholdLockedIn
			}

		case ThresholdLockedIn:
			// The new rule becomes active when its previous state
			// was locked in.
			state = ThresholdActive

		// Nothing to do if the previous state is active or failed since
		// they are both terminal states.
		case ThresholdActive:
		case ThresholdFailed:
		}

		// Update the cache to avoid recalculating the state in the
		// future.
		cache.Update(*prevNode.hash, state)
	}

	return state, nil
}

// ThresholdState returns the current rule change threshold state of the given
// deployment ID for the block AFTER then end of the current best chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) ThresholdState(deploymentID uint32) (ThresholdState, error) {
	if deploymentID > uint32(len(b.chainParams.Deployments)) {
		return ThresholdFailed, DeploymentError(deploymentID)
	}
	deployment := &b.chainParams.Deployments[deploymentID]
	checker := deploymentChecker{deployment: deployment, chain: b}
	cache := &b.deploymentCaches[deploymentID]
	b.chainLock.Lock()
	state, err := b.thresholdState(b.bestNode, checker, cache)
	b.chainLock.Unlock()
	return state, err
}
