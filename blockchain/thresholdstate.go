// Copyright (c) 2016-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// ThresholdState define the various threshold states used when voting on
// consensus changes.
type ThresholdState byte

// These constants are used to identify specific threshold states.
const (
	// ThresholdDefined is the first state for each deployment and is the
	// state for the genesis block has by definition for all deployments.
	ThresholdDefined ThresholdState = iota

	// ThresholdStarted is the state for a deployment once its start time
	// has been reached.
	ThresholdStarted

	// ThresholdLockedIn is the state for a deployment during the retarget
	// period which is after the ThresholdStarted state period and the
	// number of blocks that have voted for the deployment equal or exceed
	// the required number of votes for the deployment.
	ThresholdLockedIn

	// ThresholdActive is the state for a deployment for all blocks after a
	// retarget period in which the deployment was in the ThresholdLockedIn
	// state.
	ThresholdActive

	// ThresholdFailed is the state for a deployment once its expiration
	// time has been reached and it did not reach the ThresholdLockedIn
	// state.
	ThresholdFailed

	// numThresholdsStates is the maximum number of threshold states used in
	// tests.
	numThresholdsStates
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
	// HasStarted returns true if based on the passed block blockNode the
	// consensus is eligible for deployment.
	HasStarted(*blockNode) bool

	// HasEnded returns true if the target consensus rule change has
	// expired or timed out.
	HasEnded(*blockNode) bool

	// RuleChangeActivationThreshold is the number of blocks for which the
	// condition must be true in order to lock in a rule change.
	RuleChangeActivationThreshold() uint32

	// MinerConfirmationWindow is the number of blocks in each threshold
	// state retarget window.
	MinerConfirmationWindow() uint32

	// EligibleToActivate returns true if a custom deployment can
	// transition from the LockedIn to the Active state. For normal
	// deployments, this always returns true. However, some deployments add
	// extra rules like a minimum activation height, which can be
	// abstracted into a generic arbitrary check at the final state via
	// this method.
	EligibleToActivate(*blockNode) bool

	// IsSpeedy returns true if this is to be a "speedy" deployment. A
	// speedy deployment differs from a regular one in that only after a
	// miner block confirmation window can the deployment expire.
	IsSpeedy() bool

	// Condition returns whether or not the rule change activation
	// condition has been met.  This typically involves checking whether or
	// not the bit associated with the condition is set, but can be more
	// complex as needed.
	Condition(*blockNode) (bool, error)
}

// thresholdStateCache provides a type to cache the threshold states of each
// threshold window for a set of IDs.
type thresholdStateCache struct {
	entries map[chainhash.Hash]ThresholdState
}

// Lookup returns the threshold state associated with the given hash along with
// a boolean that indicates whether or not it is valid.
func (c *thresholdStateCache) Lookup(hash *chainhash.Hash) (ThresholdState, bool) {
	state, ok := c.entries[*hash]
	return state, ok
}

// Update updates the cache to contain the provided hash to threshold state
// mapping.
func (c *thresholdStateCache) Update(hash *chainhash.Hash, state ThresholdState) {
	c.entries[*hash] = state
}

// newThresholdCaches returns a new array of caches to be used when calculating
// threshold states.
func newThresholdCaches(numCaches uint32) []thresholdStateCache {
	caches := make([]thresholdStateCache, numCaches)
	for i := 0; i < len(caches); i++ {
		caches[i] = thresholdStateCache{
			entries: make(map[chainhash.Hash]ThresholdState),
		}
	}
	return caches
}

// PastMedianTime returns the past median time from the PoV of the passed block
// header. The past median time is the median time of the 11 blocks prior to
// the passed block header.
//
// NOTE: This is part of the chainfg.BlockClock interface
func (b *BlockChain) PastMedianTime(blockHeader *wire.BlockHeader) (time.Time, error) {
	prevHash := blockHeader.PrevBlock
	prevNode := b.index.LookupNode(&prevHash)

	// If we can't find the previous node, then we can't compute the block
	// time since it requires us to walk backwards from this node.
	if prevNode == nil {
		return time.Time{}, fmt.Errorf("blockHeader(%v) has no "+
			"previous node", blockHeader.BlockHash())
	}

	blockNode := newBlockNode(blockHeader, prevNode)

	return CalcPastMedianTime(blockNode), nil
}

// thresholdStateTransition given a state, a previous node, and a toeholds
// checker, this function transitions to the next state as defined by BIP 009.
// This state transition function is also aware of the "speedy trial"
// modifications made to BIP 0009 as part of the taproot softfork activation.
func thresholdStateTransition(state ThresholdState, prevNode *blockNode,
	checker thresholdConditionChecker,
	confirmationWindow int32) (ThresholdState, error) {

	switch state {
	case ThresholdDefined:
		// The deployment of the rule change fails if it
		// expires before it is accepted and locked in. However
		// speed deployments can only transition to failed
		// after a confirmation window.
		if !checker.IsSpeedy() && checker.HasEnded(prevNode) {
			log.Debugf("Moving from state=%v, to state=%v", state,
				ThresholdFailed)

			state = ThresholdFailed
			break
		}

		// The state for the rule moves to the started state
		// once its start time has been reached (and it hasn't
		// already expired per the above).
		if checker.HasStarted(prevNode) {
			log.Debugf("Moving from state=%v, to state=%v", state,
				ThresholdStarted)

			state = ThresholdStarted
		}

	case ThresholdStarted:
		// The deployment of the rule change fails if it
		// expires before it is accepted and locked in, but
		// only if this deployment isn't speedy.
		if !checker.IsSpeedy() && checker.HasEnded(prevNode) {
			log.Debugf("Moving from state=%v, to state=%v", state,
				ThresholdFailed)

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

			// Get the previous block node.
			countNode = countNode.parent
		}

		switch {
		// The state is locked in if the number of blocks in the
		// period that voted for the rule change meets the
		// activation threshold.
		case count >= checker.RuleChangeActivationThreshold():
			log.Debugf("Moving from state=%v, to state=%v", state,
				ThresholdLockedIn)

			state = ThresholdLockedIn

		// If this is a speedy deployment, we didn't meet the
		// threshold above, and the deployment has expired, then
		// we transition to failed.
		case checker.IsSpeedy() && checker.HasEnded(prevNode):
			log.Debugf("Moving from state=%v, to state=%v", state,
				ThresholdFailed)

			state = ThresholdFailed

		default:
			log.Tracef("Still at state=%v, threshold=%v", state,
				float64(count)/float64(checker.RuleChangeActivationThreshold()))
		}

	case ThresholdLockedIn:
		// At this point, we'll consult the deployment see if a
		// custom deployment has any other arbitrary conditions
		// that need to pass before execution. This might be a
		// minimum activation height or another policy.
		//
		// If we aren't eligible to active yet, then we'll just
		// stay in the locked in position.
		if !checker.EligibleToActivate(prevNode) {
			log.Debugf("Moving from state=%v, to state=%v", state,
				ThresholdLockedIn)

			state = ThresholdLockedIn
		} else {
			log.Debugf("Moving from state=%v, to state=%v", state,
				ThresholdActive)

			// The new rule becomes active when its
			// previous state was locked in assuming it's
			// now eligible to activate.
			state = ThresholdActive
		}

	// Nothing to do if the previous state is active or failed since
	// they are both terminal states.
	case ThresholdActive:
	case ThresholdFailed:
	}

	return state, nil
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
	prevNode = prevNode.Ancestor(prevNode.height -
		(prevNode.height+1)%confirmationWindow)

	// Iterate backwards through each of the previous confirmation windows
	// to find the most recently cached threshold state.
	var neededStates []*blockNode
	for prevNode != nil {
		// Nothing more to do if the state of the block is already
		// cached.
		if _, ok := cache.Lookup(&prevNode.hash); ok {
			break
		}

		// The state is simply defined if the start time hasn't been
		// been reached yet.
		if !checker.HasStarted(prevNode) {
			cache.Update(&prevNode.hash, ThresholdDefined)
			break
		}

		// Add this node to the list of nodes that need the state
		// calculated and cached.
		neededStates = append(neededStates, prevNode)

		// Get the ancestor that is the last block of the previous
		// confirmation window.
		prevNode = prevNode.RelativeAncestor(confirmationWindow)
	}

	// Start with the threshold state for the most recent confirmation
	// window that has a cached state.
	state := ThresholdDefined
	if prevNode != nil {
		var ok bool
		state, ok = cache.Lookup(&prevNode.hash)
		if !ok {
			return ThresholdFailed, AssertError(fmt.Sprintf(
				"thresholdState: cache lookup failed for %v",
				prevNode.hash))
		}
	}

	// Since each threshold state depends on the state of the previous
	// window, iterate starting from the oldest unknown window.
	var err error
	for neededNum := len(neededStates) - 1; neededNum >= 0; neededNum-- {
		prevNode := neededStates[neededNum]

		// Based on the current state, the previous node, and the
		// condition checker, transition to the next threshold state.
		state, err = thresholdStateTransition(
			state, prevNode, checker, confirmationWindow,
		)
		if err != nil {
			return state, err
		}

		// Update the cache to avoid recalculating the state in the
		// future.
		cache.Update(&prevNode.hash, state)
	}

	return state, nil
}

// ThresholdState returns the current rule change threshold state of the given
// deployment ID for the block AFTER the end of the current best chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) ThresholdState(deploymentID uint32) (ThresholdState, error) {
	b.chainLock.Lock()
	state, err := b.deploymentState(b.bestChain.Tip(), deploymentID)
	b.chainLock.Unlock()

	return state, err
}

// IsDeploymentActive returns true if the target deploymentID is active, and
// false otherwise.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsDeploymentActive(deploymentID uint32) (bool, error) {
	b.chainLock.Lock()
	state, err := b.deploymentState(b.bestChain.Tip(), deploymentID)
	b.chainLock.Unlock()
	if err != nil {
		return false, err
	}

	return state == ThresholdActive, nil
}

// deploymentState returns the current rule change threshold for a given
// deploymentID. The threshold is evaluated from the point of view of the block
// node passed in as the first argument to this method.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) deploymentState(prevNode *blockNode, deploymentID uint32) (ThresholdState, error) {
	if deploymentID > uint32(len(b.chainParams.Deployments)) {
		return ThresholdFailed, DeploymentError(deploymentID)
	}

	deployment := &b.chainParams.Deployments[deploymentID]
	checker := deploymentChecker{deployment: deployment, chain: b}
	cache := &b.deploymentCaches[deploymentID]

	return b.thresholdState(prevNode, checker, cache)
}

// initThresholdCaches initializes the threshold state caches for each warning
// bit and defined deployment and provides warnings if the chain is current per
// the warnUnknownRuleActivations function.
func (b *BlockChain) initThresholdCaches() error {
	// Initialize the warning and deployment caches by calculating the
	// threshold state for each of them.  This will ensure the caches are
	// populated and any states that needed to be recalculated due to
	// definition changes is done now.
	prevNode := b.bestChain.Tip().parent
	for bit := uint32(0); bit < vbNumBits; bit++ {
		checker := bitConditionChecker{bit: bit, chain: b}
		cache := &b.warningCaches[bit]
		_, err := b.thresholdState(prevNode, checker, cache)
		if err != nil {
			return err
		}
	}
	for id := 0; id < len(b.chainParams.Deployments); id++ {
		deployment := &b.chainParams.Deployments[id]
		cache := &b.deploymentCaches[id]
		checker := deploymentChecker{deployment: deployment, chain: b}
		_, err := b.thresholdState(prevNode, checker, cache)
		if err != nil {
			return err
		}
	}

	// No warnings about unknown rules until the chain is current.
	if b.isCurrent() {
		bestNode := b.bestChain.Tip()

		// Warn if any unknown new rules are either about to activate or
		// have already been activated.
		if err := b.warnUnknownRuleActivations(bestNode); err != nil {
			return err
		}
	}

	return nil
}
