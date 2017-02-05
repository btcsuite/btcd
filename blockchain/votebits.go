// Copyright (c) 2017 The decred developers
// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import "github.com/decred/dcrd/chaincfg"

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

// RuleChangeActivationThreshold is the number of blocks for which the condition
// must be true in order to lock in a rule change.
//
// This implementation returns the value defined by the chain params the checker
// is associated with.
//
// This is part of the thresholdConditionChecker interface implementation.
func (c deploymentChecker) RuleChangeActivationThreshold() uint32 {
	return c.chain.chainParams.RuleChangeActivationThreshold
}

// MinerConfirmationWindow is the number of blocks in each threshold state
// retarget window.
//
// This implementation returns the value defined by the chain params the checker
// is associated with.
//
// This is part of the thresholdConditionChecker interface implementation.
func (c deploymentChecker) MinerConfirmationWindow() uint32 {
	return c.chain.chainParams.MinerConfirmationWindow
}

// Condition returns true when the specific bit defined by the deployment
// associated with the checker is set.
//
// This is part of the thresholdConditionChecker interface implementation.
func (c deploymentChecker) Condition(node *blockNode) ([]thresholdConditionTally, error) {
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
	for _, v := range node.voteBits {
		// Make sure only valid bits are set.
		x := c.deployment.Vote.Mask & v
		if x != v {
			// Ignore invalid vote; others may be ok.
			continue
		}
		isIgnore, err := c.deployment.Vote.IsIgnore(v)
		if err != nil {
			// Ignore invalid vote; others may be ok.
			continue
		}
		isNo, err := c.deployment.Vote.IsNo(v)
		if err != nil {
			// Ignore invalid vote; others may be ok.
			continue
		}

		idx := int(x >> shift)
		tally[idx].count += 1
		tally[idx].isIgnore = isIgnore
		tally[idx].isNo = isNo
	}

	return tally, nil
}
