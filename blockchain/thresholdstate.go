// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
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
	// state for the genesis block has by definition for all deployments.
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

	// ThresholdInvalid is a deployment that does not exist.
	ThresholdInvalid ThresholdState = 5
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

const (
	// invalidChoice indicates an invalid choice in the
	// ThresholdStateTuple.
	invalidChoice = uint32(0xffffffff)
)

// ThresholdStateTuple contains the current state and the activated choice,
// when valid.
type ThresholdStateTuple struct {
	// state contains the current ThresholdState.
	State ThresholdState

	// coice is set to invalidChoice unless state is: ThresholdLockedIn,
	// ThresholdFailed & ThresholdActive.  choice should always be
	// crosschecked with invalidChoice.
	Choice uint32
}

// thresholdStateTupleStrings is a map of ThresholdState values back to their
// constant names for pretty printing.
var thresholdStateTupleStrings = map[ThresholdState]string{
	ThresholdDefined:  "defined",
	ThresholdStarted:  "started",
	ThresholdLockedIn: "lockedin",
	ThresholdActive:   "active",
	ThresholdFailed:   "failed",
}

// String returns the ThresholdStateTuple as a human-readable tuple.
func (t ThresholdStateTuple) String() string {
	if s := thresholdStateTupleStrings[t.State]; s != "" {
		return fmt.Sprintf("%v", s)
	}
	return "invalid"
}

// newThresholdState returns an initialized ThresholdStateTuple.
func newThresholdState(state ThresholdState, choice uint32) ThresholdStateTuple {
	return ThresholdStateTuple{State: state, Choice: choice}
}

// thresholdConditionTally is returned by thresholdConditionChecker.Condition
// to indicate how many votes an option received.  The isAbstain and isNo flags
// are accordingly set.  Note isAbstain and isNo can NOT be both true at the
// same time.
type thresholdConditionTally struct {
	// Vote count
	count uint32

	// isAbstain is the abstain (or zero vote).
	isAbstain bool

	// isNo is the hard no vote.
	isNo bool
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

	// RuleChangeActivationQuorum is the minimum number of votes required
	// in a voting period for before we check
	// RuleChangeActivationThreshold.
	RuleChangeActivationQuorum() uint32

	// RuleChangeActivationThreshold is the number of votes required in
	// order to lock in a rule change.
	RuleChangeActivationThreshold(uint32) uint32

	// RuleChangeActivationInterval is the number of blocks in each threshold
	// state retarget window.
	RuleChangeActivationInterval() uint32

	// StakeValidationHeight is the minimum height required before votes start
	// counting.
	StakeValidationHeight() int64

	// Condition returns an array of thresholdConditionTally that contains
	// all votes.  By convention isAbstain and isNo can not be true at the
	// same time.  The array is always returned in the same order so that
	// the consumer can repeatedly call this function without having to
	// care about said order.  Only 1 isNo vote is allowed.  By convention
	// the zero value of the vote as determined by the mask is an isAbstain
	// vote.
	Condition(*blockNode, uint32) ([]thresholdConditionTally, error)
}

// thresholdStateCache provides a type to cache the threshold states of each
// threshold window for a set of IDs.  It also keeps track of which entries have
// been modified and therefore need to be written to the database.
type thresholdStateCache struct {
	dbUpdates map[chainhash.Hash]ThresholdStateTuple
	entries   map[chainhash.Hash]ThresholdStateTuple
}

// Lookup returns the threshold state associated with the given hash along with
// a boolean that indicates whether or not it is valid.
func (c *thresholdStateCache) Lookup(hash chainhash.Hash) (ThresholdStateTuple, bool) {
	state, ok := c.entries[hash]
	return state, ok
}

// Update updates the cache to contain the provided hash to threshold state
// mapping while properly tracking needed updates flush changes to the database.
func (c *thresholdStateCache) Update(hash chainhash.Hash, state ThresholdStateTuple) {
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
func newThresholdCaches(params *chaincfg.Params) map[uint32][]thresholdStateCache {
	caches := make(map[uint32][]thresholdStateCache)
	for version := range params.Deployments {
		caches[version] = make([]thresholdStateCache,
			len(params.Deployments[version]))
		for k := range caches[version] {
			caches[version][k].entries = make(map[chainhash.Hash]ThresholdStateTuple)
			caches[version][k].dbUpdates = make(map[chainhash.Hash]ThresholdStateTuple)
		}
	}
	return caches
}

// thresholdState returns the current rule change threshold state for the block
// AFTER the given node and deployment ID.  The cache is used to ensure the
// threshold states for previous windows are only calculated once.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) thresholdState(version uint32, prevNode *blockNode, checker thresholdConditionChecker, cache *thresholdStateCache) (ThresholdStateTuple, error) {
	// The threshold state for the window that contains the genesis block is
	// defined by definition.
	confirmationWindow := int64(checker.RuleChangeActivationInterval())
	svh := checker.StakeValidationHeight()
	if prevNode == nil || prevNode.height+1 < svh+confirmationWindow {
		return newThresholdState(ThresholdDefined, invalidChoice), nil
	}

	// Get the ancestor that is the last block of the previous confirmation
	// window in order to get its threshold state.  This can be done because
	// the state is the same for all blocks within a given window.
	wantHeight := calcWantHeight(svh,
		int64(checker.RuleChangeActivationInterval()),
		prevNode.height+1)
	var err error
	prevNode, err = b.index.AncestorNode(prevNode, wantHeight)
	if err != nil {
		return newThresholdState(ThresholdFailed, invalidChoice), err
	}

	// Iterate backwards through each of the previous confirmation windows
	// to find the most recently cached threshold state.
	var neededStates []*blockNode
	for prevNode != nil {
		// Nothing more to do if the state of the block is already
		// cached.
		if _, ok := cache.Lookup(prevNode.hash); ok {
			break
		}

		// The start and expiration times are based on the median block
		// time, so calculate it now.
		medianTime, err := b.index.CalcPastMedianTime(prevNode)
		if err != nil {
			return newThresholdState(ThresholdFailed,
				invalidChoice), err
		}

		// The state is simply defined if the start time hasn't been
		// been reached yet.
		if uint64(medianTime.Unix()) < checker.BeginTime() {
			cache.Update(prevNode.hash, ThresholdStateTuple{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			})
			break
		}

		// Add this node to the list of nodes that need the state
		// calculated and cached.
		neededStates = append(neededStates, prevNode)

		// Get the ancestor that is the last block of the previous
		// confirmation window.
		prevNode, err = b.index.AncestorNode(prevNode, prevNode.height-
			confirmationWindow)
		if err != nil {
			return newThresholdState(ThresholdFailed,
				invalidChoice), err
		}
	}

	// Start with the threshold state for the most recent confirmation
	// window that has a cached state.
	stateTuple := newThresholdState(ThresholdDefined, invalidChoice)
	if prevNode != nil {
		var ok bool
		stateTuple, ok = cache.Lookup(prevNode.hash)
		if !ok {
			return newThresholdState(ThresholdFailed,
					invalidChoice), AssertError(fmt.Sprintf(
					"thresholdState: cache lookup failed "+
						"for %v", prevNode.hash))
		}
	}

	// Since each threshold state depends on the state of the previous
	// window, iterate starting from the oldest unknown window.
	for neededNum := len(neededStates) - 1; neededNum >= 0; neededNum-- {
		prevNode := neededStates[neededNum]

		switch stateTuple.State {
		case ThresholdDefined:
			// Ensure we are at the minimal require height.
			if prevNode.height < svh {
				stateTuple.State = ThresholdDefined
				break
			}

			// The deployment of the rule change fails if it expires
			// before it is accepted and locked in.
			medianTime, err := b.index.CalcPastMedianTime(prevNode)
			if err != nil {
				return newThresholdState(ThresholdFailed,
					invalidChoice), err
			}
			medianTimeUnix := uint64(medianTime.Unix())
			if medianTimeUnix >= checker.EndTime() {
				stateTuple.State = ThresholdFailed
				break
			}

			// Make sure we are on the correct stake version.
			if b.calcStakeVersion(prevNode) < version {
				stateTuple.State = ThresholdDefined
				break
			}

			// The state must remain in the defined state so long as
			// a majority of the PoW miners have not upgraded.
			if !b.isMajorityVersion(int32(version), prevNode,
				b.chainParams.BlockRejectNumRequired) {

				stateTuple.State = ThresholdDefined
				break
			}

			// The state for the rule moves to the started state
			// once its start time has been reached (and it hasn't
			// already expired per the above).
			if medianTimeUnix >= checker.BeginTime() {
				stateTuple.State = ThresholdStarted
			}

		case ThresholdStarted:
			// The deployment of the rule change fails if it expires
			// before it is accepted and locked in.
			medianTime, err := b.index.CalcPastMedianTime(prevNode)
			if err != nil {
				return newThresholdState(ThresholdFailed,
					invalidChoice), err
			}
			if uint64(medianTime.Unix()) >= checker.EndTime() {
				stateTuple.State = ThresholdFailed
				break
			}

			// At this point, the rule change is still being voted
			// on by the miners, so iterate backwards through the
			// confirmation window to count all of the votes in it.
			var (
				counts       []thresholdConditionTally
				totalVotes   uint32
				abstainVotes uint32
			)
			countNode := prevNode
			for i := int64(0); i < confirmationWindow; i++ {
				c, err := checker.Condition(countNode, version)
				if err != nil {
					return newThresholdState(
						ThresholdFailed, invalidChoice), err
				}

				// Create array first time around.
				if len(counts) == 0 {
					counts = make([]thresholdConditionTally, len(c))
				}

				// Tally votes.
				for k := range c {
					counts[k].count += c[k].count
					counts[k].isAbstain = c[k].isAbstain
					counts[k].isNo = c[k].isNo
					if c[k].isAbstain {
						abstainVotes += c[k].count
					} else {
						totalVotes += c[k].count
					}
				}

				// Get the previous block node.  This function
				// is used over simply accessing countNode.parent
				// directly as it will dynamically create
				// previous block nodes as needed.  This helps
				// allow only the pieces of the chain that are
				// needed to remain in memory.
				countNode, err = b.index.PrevNodeFromNode(countNode)
				if err != nil {
					return newThresholdState(
						ThresholdFailed, invalidChoice), err
				}
			}

			// Determine if we have reached quorum.
			totalNonAbstainVotes := uint32(0)
			for _, v := range counts {
				if v.isAbstain && !v.isNo {
					continue
				}
				totalNonAbstainVotes += v.count
			}
			if totalNonAbstainVotes < checker.RuleChangeActivationQuorum() {
				break
			}

			// The state is locked in if the number of blocks in the
			// period that voted for the rule change meets the
			// activation threshold.
			for k, v := range counts {
				// We require at least 10% quorum on all votes.
				if v.count < checker.RuleChangeActivationThreshold(totalVotes) {
					continue
				}
				// Something went over the threshold
				switch {
				case !v.isAbstain && !v.isNo:
					// One of the choices has
					// reached majority.
					stateTuple.State = ThresholdLockedIn
					stateTuple.Choice = uint32(k)
				case !v.isAbstain && v.isNo:
					// No choice.  Only 1 No per
					// vote is allowed.  A No vote
					// is required though.
					stateTuple.State = ThresholdFailed
					stateTuple.Choice = uint32(k)
				case v.isAbstain && !v.isNo:
					// This is the abstain case.
					// The statemachine is not
					// supposed to change.
					continue
				case v.isAbstain && v.isNo:
					// Invalid choice.
					stateTuple.State = ThresholdFailed
					stateTuple.Choice = uint32(k)
				}
				break
			}

		case ThresholdLockedIn:
			// The new rule becomes active when its previous state
			// was locked in.
			stateTuple.State = ThresholdActive

		// Nothing to do if the previous state is active or failed since
		// they are both terminal states.
		case ThresholdActive:
		case ThresholdFailed:
		}

		// Update the cache to avoid recalculating the state in the
		// future.
		cache.Update(prevNode.hash, stateTuple)
	}

	return stateTuple, nil
}

// deploymentState returns the current rule change threshold for a given stake
// version and deploymentID.  The threshold is evaluated from the point of view
// of the block node passed in as the first argument to this method.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) deploymentState(prevNode *blockNode, version uint32, deploymentID string) (ThresholdStateTuple, error) {
	for k := range b.chainParams.Deployments[version] {
		if b.chainParams.Deployments[version][k].Vote.Id == deploymentID {
			checker := deploymentChecker{
				deployment: &b.chainParams.Deployments[version][k],
				chain:      b,
			}
			cache := &b.deploymentCaches[version][k]
			return b.thresholdState(version, prevNode, checker, cache)
		}
	}

	invalidState := ThresholdStateTuple{
		State:  ThresholdInvalid,
		Choice: invalidChoice,
	}
	return invalidState, DeploymentError(deploymentID)
}

// ThresholdState returns the current rule change threshold state of the given
// deployment ID for the block AFTER the provided block hash.
//
// This function is safe for concurrent access.
func (b *BlockChain) ThresholdState(hash *chainhash.Hash, version uint32, deploymentID string) (ThresholdStateTuple, error) {
	node := b.index.LookupNode(hash)
	if node == nil {
		invalidState := ThresholdStateTuple{
			State:  ThresholdInvalid,
			Choice: invalidChoice,
		}
		return invalidState, HashError(hash.String())
	}

	b.chainLock.Lock()
	state, err := b.deploymentState(node, version, deploymentID)
	b.chainLock.Unlock()
	return state, err
}

// isLNFeaturesAgendaActive returns whether or not the LN features agenda vote,
// as defined in DCP0002 and DCP0003 has passed and is now active from the point
// of view of the passed block node.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isLNFeaturesAgendaActive(prevNode *blockNode) (bool, error) {
	// Determine the version for the LN features agenda as defined in
	// DCP0002 and DCP0003 for the provided network.
	deploymentVer := uint32(5)
	if b.chainParams.Net != wire.MainNet {
		deploymentVer = 6
	}

	state, err := b.deploymentState(prevNode, deploymentVer,
		chaincfg.VoteIDLNFeatures)
	if err != nil {
		return false, err
	}

	// NOTE: The choice field of the return threshold state is not examined
	// here because there is only one possible choice that can be active for
	// the agenda, which is yes, so there is no need to check it.
	return state.State == ThresholdActive, nil

}

// IsLNFeaturesAgendaActive returns whether or not the LN features agenda vote,
// as defined in DCP0002 and DCP0003 has passed and is now active for the block
// AFTER the current best chain block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsLNFeaturesAgendaActive() (bool, error) {
	b.chainLock.Lock()
	isActive, err := b.isLNFeaturesAgendaActive(b.bestNode)
	b.chainLock.Unlock()
	return isActive, err
}

// VoteCounts is a compacted struct that is used to message vote counts.
type VoteCounts struct {
	Total        uint32
	TotalAbstain uint32
	VoteChoices  []uint32
}

// getVoteCounts returns the vote counts for the specified version for the
// current interval.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) getVoteCounts(node *blockNode, version uint32, d chaincfg.ConsensusDeployment) (VoteCounts, error) {
	height := calcWantHeight(b.chainParams.StakeValidationHeight,
		int64(b.chainParams.RuleChangeActivationInterval), node.height)

	var err error
	result := VoteCounts{
		VoteChoices: make([]uint32, len(d.Vote.Choices)),
	}
	countNode := node
	for countNode.height > height {
		for _, vote := range countNode.votes {
			// Wrong versions do not count.
			if vote.Version != version {
				continue
			}

			// Increase total votes.
			result.Total++

			index := d.Vote.VoteIndex(vote.Bits)
			if index == -1 {
				// Invalid votes are treated as abstain.
				result.TotalAbstain++
				continue
			} else if d.Vote.Choices[index].IsAbstain {
				result.TotalAbstain++
			}
			result.VoteChoices[index]++
		}

		// Get the previous block node.  This function
		// is used over simply accessing countNode.parent
		// directly as it will dynamically create
		// previous block nodes as needed.  This helps
		// allow only the pieces of the chain that are
		// needed to remain in memory.
		countNode, err = b.index.PrevNodeFromNode(countNode)
		if err != nil {
			return VoteCounts{}, err
		}
	}

	return result, nil
}

// GetVoteCounts returns the vote counts for the specified version and
// deployment identifier for the current interval.
//
// This function is safe for concurrent access.
func (b *BlockChain) GetVoteCounts(version uint32, deploymentID string) (VoteCounts, error) {
	for k := range b.chainParams.Deployments[version] {
		if b.chainParams.Deployments[version][k].Vote.Id == deploymentID {
			b.chainLock.Lock()
			defer b.chainLock.Unlock()
			return b.getVoteCounts(b.bestNode, version,
				b.chainParams.Deployments[version][k])
		}
	}
	return VoteCounts{}, DeploymentError(deploymentID)
}

// CountVoteVersion returns the total number of version votes for the current
// interval.
//
// This function is safe for concurrent access.
func (b *BlockChain) CountVoteVersion(version uint32) (uint32, error) {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()
	countNode := b.bestNode

	height := calcWantHeight(b.chainParams.StakeValidationHeight,
		int64(b.chainParams.RuleChangeActivationInterval),
		countNode.height)

	var err error
	total := uint32(0)
	for countNode.height > height {
		for _, vote := range countNode.votes {
			// Wrong versions do not count.
			if vote.Version != version {
				continue
			}

			// Increase total votes.
			total++
		}

		// Get the previous block node.  This function
		// is used over simply accessing countNode.parent
		// directly as it will dynamically create
		// previous block nodes as needed.  This helps
		// allow only the pieces of the chain that are
		// needed to remain in memory.
		countNode, err = b.index.PrevNodeFromNode(countNode)
		if err != nil {
			return 0, err
		}
	}

	return total, nil
}
