// Copyright (c) 2017-2018 The Decred developers
// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"github.com/decred/dcrd/chaincfg"
)

// deploymentChecker provides a thresholdConditionChecker which can be used to
// test a specific deployment rule.  This is required for properly detecting
// and activating consensus rule changes.
type deploymentChecker struct {
	deployment *chaincfg.ConsensusDeployment
	chain      *BlockChain
}

// Ensure the deploymentChecker type implements the thresholdConditionChecker
// interface.
var _ thresholdConditionChecker = deploymentChecker{}

// BeginTime returns the unix timestamp for the median block time after which
// voting on a rule change starts (at the next window).
//
// This implementation returns the value defined by the specific deployment the
// checker is associated with.
//
// This is part of the thresholdConditionChecker interface implementation.
func (c deploymentChecker) BeginTime() uint64 {
	return c.deployment.StartTime
}

// EndTime returns the unix timestamp for the median block time after which an

// attempted rule change fails if it has not already been locked in or
// activated.
//
// This implementation returns the value defined by the specific deployment the
// checker is associated with.
//
// This is part of the thresholdConditionChecker interface implementation.
func (c deploymentChecker) EndTime() uint64 {
	return c.deployment.ExpireTime
}

// RuleChangeActivationQuorum is the minimum votes required to reach quorum.
//
// This implementation returns the value defined by the chain params the checker
// is associated with.
//
// This is part of the thresholdConditionChecker interface implementation.
func (c deploymentChecker) RuleChangeActivationQuorum() uint32 {
	return c.chain.chainParams.RuleChangeActivationQuorum
}

// RuleChangeActivationThreshold is the number of votes required to reach the
// threshold as defined by chain params.
//
// This implementation returns the value defined by the chain params the checker
// is associated with.
//
// This is part of the thresholdConditionChecker interface implementation.
func (c deploymentChecker) RuleChangeActivationThreshold(totalVotes uint32) uint32 {
	return totalVotes * c.chain.chainParams.RuleChangeActivationMultiplier /
		c.chain.chainParams.RuleChangeActivationDivisor
}

// RuleChangeActivationInterval is the number of blocks in each threshold state
// retarget window.
//
// This implementation returns the value defined by the chain params the checker
// is associated with.
//
// This is part of the thresholdConditionChecker interface implementation.
func (c deploymentChecker) RuleChangeActivationInterval() uint32 {
	return c.chain.chainParams.RuleChangeActivationInterval
}

// StakeValidationHeight is the minimum height required before votes start
// counting.
//
// This implementation returns the value defined by the chain params the checker
// is associated with.
//
// This is part of the thresholdConditionChecker interface implementation.
func (c deploymentChecker) StakeValidationHeight() int64 {
	return c.chain.chainParams.StakeValidationHeight
}

// Condition returns true when the specific bit defined by the deployment
// associated with the checker is set.
//
// This is part of the thresholdConditionChecker interface implementation.
func (c deploymentChecker) Condition(node *blockNode, version uint32) ([]thresholdConditionTally, error) {
	if c.deployment.Vote.Mask == 0 {
		return []thresholdConditionTally{}, AssertError("invalid mask")
	}

	// Calculate shift in order to make a zero based index later.
	var shift uint16
	mask := c.deployment.Vote.Mask
	for {
		if mask&0x0001 == 0x0001 {
			break
		}
		shift++
		mask >>= 1
	}

	// Setup tally array and iterate over Choices to assemble the vote
	// information into the thresholdConditionTally array.
	tally := make([]thresholdConditionTally, len(c.deployment.Vote.Choices))
	for t, choice := range c.deployment.Vote.Choices {
		tally[t].isAbstain = choice.IsAbstain
		tally[t].isNo = choice.IsNo
	}

	for _, vote := range node.votes {
		if version != vote.Version {
			// Wrong version, ignore.
			continue
		}
		idx := c.deployment.Vote.Mask & vote.Bits >> shift
		if int(idx) > len(c.deployment.Vote.Choices)-1 {
			// Invalid choice.
			continue
		}
		tally[idx].count += 1
	}

	return tally, nil
}
