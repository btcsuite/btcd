// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"math/big"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

var (
	// bigZero is 0 represented as a big.Int.  It is defined here to avoid
	// the overhead of creating it multiple times.
	bigZero = big.NewInt(0)

	// bigOne is 1 represented as a big.Int.  It is defined here to avoid
	// the overhead of creating it multiple times.
	bigOne = big.NewInt(1)

	// oneLsh256 is 1 shifted left 256 bits.  It is defined here to avoid
	// the overhead of creating it multiple times.
	oneLsh256 = new(big.Int).Lsh(bigOne, 256)

	// maxShift is the maximum shift for a difficulty that resets (e.g.
	// testnet difficulty).
	maxShift = uint(256)
)

// ShaHashToBig converts a wire.ShaHash into a big.Int that can be used to
// perform math comparisons.
func ShaHashToBig(hash *chainhash.Hash) *big.Int {
	// A ShaHash is in little-endian, but the big package wants the bytes
	// in big-endian, so reverse them.
	buf := *hash
	blen := len(buf)
	for i := 0; i < blen/2; i++ {
		buf[i], buf[blen-1-i] = buf[blen-1-i], buf[i]
	}

	return new(big.Int).SetBytes(buf[:])
}

// CompactToBig converts a compact representation of a whole number N to an
// unsigned 32-bit number.  The representation is similar to IEEE754 floating
// point numbers.
//
// Like IEEE754 floating point, there are three basic components: the sign,
// the exponent, and the mantissa.  They are broken out as follows:
//
//	* the most significant 8 bits represent the unsigned base 256 exponent
// 	* bit 23 (the 24th bit) represents the sign bit
//	* the least significant 23 bits represent the mantissa
//
//	-------------------------------------------------
//	|   Exponent     |    Sign    |    Mantissa     |
//	-------------------------------------------------
//	| 8 bits [31-24] | 1 bit [23] | 23 bits [22-00] |
//	-------------------------------------------------
//
// The formula to calculate N is:
// 	N = (-1^sign) * mantissa * 256^(exponent-3)
//
// This compact form is only used in decred to encode unsigned 256-bit numbers
// which represent difficulty targets, thus there really is not a need for a
// sign bit, but it is implemented here to stay consistent with bitcoind.
func CompactToBig(compact uint32) *big.Int {
	// Extract the mantissa, sign bit, and exponent.
	mantissa := compact & 0x007fffff
	isNegative := compact&0x00800000 != 0
	exponent := uint(compact >> 24)

	// Since the base for the exponent is 256, the exponent can be treated
	// as the number of bytes to represent the full 256-bit number.  So,
	// treat the exponent as the number of bytes and shift the mantissa
	// right or left accordingly.  This is equivalent to:
	// N = mantissa * 256^(exponent-3)
	var bn *big.Int
	if exponent <= 3 {
		mantissa >>= 8 * (3 - exponent)
		bn = big.NewInt(int64(mantissa))
	} else {
		bn = big.NewInt(int64(mantissa))
		bn.Lsh(bn, 8*(exponent-3))
	}

	// Make it negative if the sign bit is set.
	if isNegative {
		bn = bn.Neg(bn)
	}

	return bn
}

// BigToCompact converts a whole number N to a compact representation using
// an unsigned 32-bit number.  The compact representation only provides 23 bits
// of precision, so values larger than (2^23 - 1) only encode the most
// significant digits of the number.  See CompactToBig for details.
func BigToCompact(n *big.Int) uint32 {
	// No need to do any work if it's zero.
	if n.Sign() == 0 {
		return 0
	}

	// Since the base for the exponent is 256, the exponent can be treated
	// as the number of bytes.  So, shift the number right or left
	// accordingly.  This is equivalent to:
	// mantissa = mantissa / 256^(exponent-3)
	var mantissa uint32
	exponent := uint(len(n.Bytes()))
	if exponent <= 3 {
		mantissa = uint32(n.Bits()[0])
		mantissa <<= 8 * (3 - exponent)
	} else {
		// Use a copy to avoid modifying the caller's original number.
		tn := new(big.Int).Set(n)
		mantissa = uint32(tn.Rsh(tn, 8*(exponent-3)).Bits()[0])
	}

	// When the mantissa already has the sign bit set, the number is too
	// large to fit into the available 23-bits, so divide the number by 256
	// and increment the exponent accordingly.
	if mantissa&0x00800000 != 0 {
		mantissa >>= 8
		exponent++
	}

	// Pack the exponent, sign bit, and mantissa into an unsigned 32-bit
	// int and return it.
	compact := uint32(exponent<<24) | mantissa
	if n.Sign() < 0 {
		compact |= 0x00800000
	}
	return compact
}

// CalcWork calculates a work value from difficulty bits.  Decred increases
// the difficulty for generating a block by decreasing the value which the
// generated hash must be less than.  This difficulty target is stored in each
// block header using a compact representation as described in the documentation
// for CompactToBig.  The main chain is selected by choosing the chain that has
// the most proof of work (highest difficulty).  Since a lower target difficulty
// value equates to higher actual difficulty, the work value which will be
// accumulated must be the inverse of the difficulty.  Also, in order to avoid
// potential division by zero and really small floating point numbers, the
// result adds 1 to the denominator and multiplies the numerator by 2^256.
func CalcWork(bits uint32) *big.Int {
	// Return a work value of zero if the passed difficulty bits represent
	// a negative number. Note this should not happen in practice with valid
	// blocks, but an invalid block could trigger it.
	difficultyNum := CompactToBig(bits)
	if difficultyNum.Sign() <= 0 {
		return big.NewInt(0)
	}

	// (1 << 256) / (difficultyNum + 1)
	denominator := new(big.Int).Add(difficultyNum, bigOne)
	return new(big.Int).Div(oneLsh256, denominator)
}

// calcEasiestDifficulty calculates the easiest possible difficulty that a block
// can have given starting difficulty bits and a duration.  It is mainly used to
// verify that claimed proof of work by a block is sane as compared to a
// known good checkpoint.
func (b *BlockChain) calcEasiestDifficulty(bits uint32,
	duration time.Duration) uint32 {
	// Convert types used in the calculations below.
	durationVal := int64(duration)
	adjustmentFactor := big.NewInt(b.chainParams.RetargetAdjustmentFactor)
	maxRetargetTimespan := int64(b.chainParams.TargetTimespan) *
		b.chainParams.RetargetAdjustmentFactor

	// The test network rules allow minimum difficulty blocks after more
	// than twice the desired amount of time needed to generate a block has
	// elapsed.
	if b.chainParams.ResetMinDifficulty {
		if durationVal > int64(b.chainParams.TimePerBlock)*2 {
			return b.chainParams.PowLimitBits
		}
	}

	// Since easier difficulty equates to higher numbers, the easiest
	// difficulty for a given duration is the largest value possible given
	// the number of retargets for the duration and starting difficulty
	// multiplied by the max adjustment factor.
	newTarget := CompactToBig(bits)
	for durationVal > 0 && newTarget.Cmp(b.chainParams.PowLimit) < 0 {
		newTarget.Mul(newTarget, adjustmentFactor)
		durationVal -= maxRetargetTimespan
	}

	// Limit new value to the proof of work limit.
	if newTarget.Cmp(b.chainParams.PowLimit) > 0 {
		newTarget.Set(b.chainParams.PowLimit)
	}

	return BigToCompact(newTarget)
}

// findPrevTestNetDifficulty returns the difficulty of the previous block which
// did not have the special testnet minimum difficulty rule applied.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) findPrevTestNetDifficulty(startNode *blockNode) (uint32,
	error) {
	// Search backwards through the chain for the last block without
	// the special rule applied.
	blocksPerRetarget := b.chainParams.WorkDiffWindowSize *
		b.chainParams.WorkDiffWindows
	iterNode := startNode
	for iterNode != nil && iterNode.height%blocksPerRetarget != 0 &&
		iterNode.header.Bits == b.chainParams.PowLimitBits {

		// Get the previous block node.  This function is used over
		// simply accessing iterNode.parent directly as it will
		// dynamically create previous block nodes as needed.  This
		// helps allow only the pieces of the chain that are needed
		// to remain in memory.
		var err error
		iterNode, err = b.getPrevNodeFromNode(iterNode)
		if err != nil {
			log.Errorf("getPrevNodeFromNode: %v", err)
			return 0, err
		}
	}

	// Return the found difficulty or the minimum difficulty if no
	// appropriate block was found.
	lastBits := b.chainParams.PowLimitBits
	if iterNode != nil {
		lastBits = iterNode.header.Bits
	}
	return lastBits, nil
}

// calcNextRequiredDifficulty calculates the required difficulty for the block
// after the passed previous block node based on the difficulty retarget rules.
// This function differs from the exported CalcNextRequiredDifficulty in that
// the exported version uses the current best chain as the previous block node
// while this function accepts any block node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) calcNextRequiredDifficulty(curNode *blockNode,
	newBlockTime time.Time) (uint32, error) {
	// Genesis block.
	if curNode == nil {
		return b.chainParams.PowLimitBits, nil
	}

	// Get the old difficulty; if we aren't at a block height where it changes,
	// just return this.
	oldDiff := curNode.header.Bits
	oldDiffBig := CompactToBig(curNode.header.Bits)

	// We're not at a retarget point, return the oldDiff.
	if (curNode.height+1)%b.chainParams.WorkDiffWindowSize != 0 {
		// The test network rules allow minimum difficulty blocks after
		// more than twice the desired amount of time needed to generate
		// a block has elapsed.
		if b.chainParams.ResetMinDifficulty {
			// Return minimum difficulty when more than twice the
			// desired amount of time needed to generate a block has
			// elapsed.
			allowMinTime := curNode.header.Timestamp.Add(
				b.chainParams.TimePerBlock * b.chainParams.MinDiffResetTimeFactor)

			// For every extra target timespan that passes, we halve the
			// difficulty.
			if newBlockTime.After(allowMinTime) {
				timePassed := newBlockTime.Sub(curNode.header.Timestamp)
				timePassed -= (b.chainParams.TimePerBlock *
					b.chainParams.MinDiffResetTimeFactor)
				shifts := uint((timePassed / b.chainParams.TimePerBlock) + 1)

				// Scale the difficulty with time passed.
				oldTarget := CompactToBig(curNode.header.Bits)
				newTarget := new(big.Int)
				if shifts < maxShift {
					newTarget.Lsh(oldTarget, shifts)
				} else {
					newTarget.Set(oneLsh256)
				}

				// Limit new value to the proof of work limit.
				if newTarget.Cmp(b.chainParams.PowLimit) > 0 {
					newTarget.Set(b.chainParams.PowLimit)
				}

				return BigToCompact(newTarget), nil
			}

			// The block was mined within the desired timeframe, so
			// return the difficulty for the last block which did
			// not have the special minimum difficulty rule applied.
			prevBits, err := b.findPrevTestNetDifficulty(curNode)
			if err != nil {
				return 0, err
			}
			return prevBits, nil
		}

		return oldDiff, nil
	}

	// Declare some useful variables.
	RAFBig := big.NewInt(b.chainParams.RetargetAdjustmentFactor)
	nextDiffBigMin := CompactToBig(curNode.header.Bits)
	nextDiffBigMin.Div(nextDiffBigMin, RAFBig)
	nextDiffBigMax := CompactToBig(curNode.header.Bits)
	nextDiffBigMax.Mul(nextDiffBigMax, RAFBig)

	alpha := b.chainParams.WorkDiffAlpha

	// Number of nodes to traverse while calculating difficulty.
	nodesToTraverse := (b.chainParams.WorkDiffWindowSize *
		b.chainParams.WorkDiffWindows)

	// Initialize bigInt slice for the percentage changes for each window period
	// above or below the target.
	windowChanges := make([]*big.Int, b.chainParams.WorkDiffWindows)

	// Regress through all of the previous blocks and store the percent changes
	// per window period; use bigInts to emulate 64.32 bit fixed point.
	oldNode := curNode
	windowPeriod := int64(0)
	weights := uint64(0)
	recentTime := curNode.header.Timestamp.UnixNano()
	olderTime := int64(0)

	for i := int64(0); ; i++ {
		// Store and reset after reaching the end of every window period.
		if i%b.chainParams.WorkDiffWindowSize == 0 && i != 0 {
			olderTime = oldNode.header.Timestamp.UnixNano()
			timeDifference := recentTime - olderTime

			// Just assume we're at the target (no change) if we've
			// gone all the way back to the genesis block.
			if oldNode.height == 0 {
				timeDifference = int64(b.chainParams.TargetTimespan)
			}

			timeDifBig := big.NewInt(timeDifference)
			timeDifBig.Lsh(timeDifBig, 32) // Add padding
			targetTemp := big.NewInt(int64(b.chainParams.TargetTimespan))

			windowAdjusted := targetTemp.Div(timeDifBig, targetTemp)

			// Weight it exponentially. Be aware that this could at some point
			// overflow if alpha or the number of blocks used is really large.
			windowAdjusted = windowAdjusted.Lsh(windowAdjusted,
				uint((b.chainParams.WorkDiffWindows-windowPeriod)*alpha))

			// Sum up all the different weights incrementally.
			weights += 1 << uint64((b.chainParams.WorkDiffWindows-windowPeriod)*
				alpha)

			// Store it in the slice.
			windowChanges[windowPeriod] = windowAdjusted

			windowPeriod++

			recentTime = olderTime
		}

		if i == nodesToTraverse {
			break // Exit for loop when we hit the end.
		}

		// Get the previous block node.
		var err error
		tempNode := oldNode
		oldNode, err = b.getPrevNodeFromNode(oldNode)
		if err != nil {
			return 0, err
		}

		// If we're at the genesis block, reset the oldNode
		// so that it stays at the genesis block.
		if oldNode == nil {
			oldNode = tempNode
		}
	}

	// Sum up the weighted window periods.
	weightedSum := big.NewInt(0)
	for i := int64(0); i < b.chainParams.WorkDiffWindows; i++ {
		weightedSum.Add(weightedSum, windowChanges[i])
	}

	// Divide by the sum of all weights.
	weightsBig := big.NewInt(int64(weights))
	weightedSumDiv := weightedSum.Div(weightedSum, weightsBig)

	// Multiply by the old diff.
	nextDiffBig := weightedSumDiv.Mul(weightedSumDiv, oldDiffBig)

	// Right shift to restore the original padding (restore non-fixed point).
	nextDiffBig = nextDiffBig.Rsh(nextDiffBig, 32)

	// Check to see if we're over the limits for the maximum allowable retarget;
	// if we are, return the maximum or minimum except in the case that oldDiff
	// is zero.
	if oldDiffBig.Cmp(bigZero) == 0 { // This should never really happen,
		nextDiffBig.Set(nextDiffBig) // but in case it does...
	} else if nextDiffBig.Cmp(bigZero) == 0 {
		nextDiffBig.Set(b.chainParams.PowLimit)
	} else if nextDiffBig.Cmp(nextDiffBigMax) == 1 {
		nextDiffBig.Set(nextDiffBigMax)
	} else if nextDiffBig.Cmp(nextDiffBigMin) == -1 {
		nextDiffBig.Set(nextDiffBigMin)
	}

	// Limit new value to the proof of work limit.
	if nextDiffBig.Cmp(b.chainParams.PowLimit) > 0 {
		nextDiffBig.Set(b.chainParams.PowLimit)
	}

	// Log new target difficulty and return it.  The new target logging is
	// intentionally converting the bits back to a number instead of using
	// newTarget since conversion to the compact representation loses
	// precision.
	nextDiffBits := BigToCompact(nextDiffBig)
	log.Debugf("Difficulty retarget at block height %d", curNode.height+1)
	log.Debugf("Old target %08x (%064x)", curNode.header.Bits, oldDiffBig)
	log.Debugf("New target %08x (%064x)", nextDiffBits, CompactToBig(nextDiffBits))

	return nextDiffBits, nil
}

// CalcNextRequiredDiffFromNode calculates the required difficulty for the block
// given with the passed hash along with the given timestamp.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) CalcNextRequiredDiffFromNode(hash *chainhash.Hash,
	timestamp time.Time) (uint32, error) {
	// Fetch the block to get the difficulty for.
	node, err := b.findNode(hash)
	if err != nil {
		return 0, err
	}

	return b.calcNextRequiredDifficulty(node, timestamp)
}

// CalcNextRequiredDifficulty calculates the required difficulty for the block
// after the end of the current best chain based on the difficulty retarget
// rules.
//
// This function is safe for concurrent access.
func (b *BlockChain) CalcNextRequiredDifficulty(timestamp time.Time) (uint32,
	error) {
	b.chainLock.Lock()
	difficulty, err := b.calcNextRequiredDifficulty(b.bestNode, timestamp)
	b.chainLock.Unlock()
	return difficulty, err
}

// mergeDifficulty takes an original stake difficulty and two new, scaled
// stake difficulties, merges the new difficulties, and outputs a new
// merged stake difficulty.
func mergeDifficulty(oldDiff int64, newDiff1 int64, newDiff2 int64) int64 {
	newDiff1Big := big.NewInt(newDiff1)
	newDiff2Big := big.NewInt(newDiff2)
	newDiff2Big.Lsh(newDiff2Big, 32)

	oldDiffBig := big.NewInt(oldDiff)
	oldDiffBigLSH := big.NewInt(oldDiff)
	oldDiffBigLSH.Lsh(oldDiffBig, 32)

	newDiff1Big.Div(oldDiffBigLSH, newDiff1Big)
	newDiff2Big.Div(newDiff2Big, oldDiffBig)

	// Combine the two changes in difficulty.
	summedChange := big.NewInt(0)
	summedChange.Set(newDiff2Big)
	summedChange.Lsh(summedChange, 32)
	summedChange.Div(summedChange, newDiff1Big)
	summedChange.Mul(summedChange, oldDiffBig)
	summedChange.Rsh(summedChange, 32)

	return summedChange.Int64()
}

// calcNextRequiredStakeDifficulty calculates the exponentially weighted average
// and then uses it to determine the next stake difficulty.
// TODO: You can combine the first and second for loops below for a speed up
// if you'd like, I'm not sure how much it matters.
func (b *BlockChain) calcNextRequiredStakeDifficulty(curNode *blockNode) (int64,
	error) {
	alpha := b.chainParams.StakeDiffAlpha
	stakeDiffStartHeight := int64(b.chainParams.CoinbaseMaturity) +
		1
	maxRetarget := int64(b.chainParams.RetargetAdjustmentFactor)
	TicketPoolWeight := int64(b.chainParams.TicketPoolSizeWeight)

	// Number of nodes to traverse while calculating difficulty.
	nodesToTraverse := (b.chainParams.StakeDiffWindowSize *
		b.chainParams.StakeDiffWindows)

	// Genesis block. Block at height 1 has these parameters.
	// Additionally, if we're before the time when people generally begin
	// purchasing tickets, just use the MinimumStakeDiff.
	// This is sort of sloppy and coded with the hopes that generally by
	// stakeDiffStartHeight people will be submitting lots of SStx over the
	// past nodesToTraverse many nodes. It should be okay with the default
	// Decred parameters, but might do weird things if you use custom
	// parameters.
	if curNode == nil ||
		curNode.height < stakeDiffStartHeight {
		return b.chainParams.MinimumStakeDiff, nil
	}

	// Get the old difficulty; if we aren't at a block height where it changes,
	// just return this.
	oldDiff := curNode.header.SBits
	if (curNode.height+1)%b.chainParams.StakeDiffWindowSize != 0 {
		return oldDiff, nil
	}

	// The target size of the ticketPool in live tickets. Recast these as int64
	// to avoid possible overflows for large sizes of either variable in
	// params.
	targetForTicketPool := int64(b.chainParams.TicketsPerBlock) *
		int64(b.chainParams.TicketPoolSize)

	// Initialize bigInt slice for the percentage changes for each window period
	// above or below the target.
	windowChanges := make([]*big.Int, b.chainParams.StakeDiffWindows)

	// Regress through all of the previous blocks and store the percent changes
	// per window period; use bigInts to emulate 64.32 bit fixed point.
	oldNode := curNode
	windowPeriod := int64(0)
	weights := uint64(0)

	for i := int64(0); ; i++ {
		// Store and reset after reaching the end of every window period.
		if (i+1)%b.chainParams.StakeDiffWindowSize == 0 {
			// First adjust based on ticketPoolSize. Skew the difference
			// in ticketPoolSize by max adjustment factor to help
			// weight ticket pool size versus tickets per block.
			poolSizeSkew := (int64(oldNode.header.PoolSize)-
				targetForTicketPool)*TicketPoolWeight + targetForTicketPool

			// Don't let this be negative or zero.
			if poolSizeSkew <= 0 {
				poolSizeSkew = 1
			}

			curPoolSizeTemp := big.NewInt(poolSizeSkew)
			curPoolSizeTemp.Lsh(curPoolSizeTemp, 32) // Add padding
			targetTemp := big.NewInt(targetForTicketPool)

			windowAdjusted := curPoolSizeTemp.Div(curPoolSizeTemp, targetTemp)

			// Weight it exponentially. Be aware that this could at some point
			// overflow if alpha or the number of blocks used is really large.
			windowAdjusted = windowAdjusted.Lsh(windowAdjusted,
				uint((b.chainParams.StakeDiffWindows-windowPeriod)*alpha))

			// Sum up all the different weights incrementally.
			weights += 1 << uint64((b.chainParams.StakeDiffWindows-windowPeriod)*
				alpha)

			// Store it in the slice.
			windowChanges[windowPeriod] = windowAdjusted

			// windowFreshStake = 0
			windowPeriod++
		}

		if (i + 1) == nodesToTraverse {
			break // Exit for loop when we hit the end.
		}

		// Get the previous block node.
		var err error
		tempNode := oldNode
		oldNode, err = b.getPrevNodeFromNode(oldNode)
		if err != nil {
			return 0, err
		}

		// If we're at the genesis block, reset the oldNode
		// so that it stays at the genesis block.
		if oldNode == nil {
			oldNode = tempNode
		}
	}

	// Sum up the weighted window periods.
	weightedSum := big.NewInt(0)
	for i := int64(0); i < b.chainParams.StakeDiffWindows; i++ {
		weightedSum.Add(weightedSum, windowChanges[i])
	}

	// Divide by the sum of all weights.
	weightsBig := big.NewInt(int64(weights))
	weightedSumDiv := weightedSum.Div(weightedSum, weightsBig)

	// Multiply by the old stake diff.
	oldDiffBig := big.NewInt(oldDiff)
	nextDiffBig := weightedSumDiv.Mul(weightedSumDiv, oldDiffBig)

	// Right shift to restore the original padding (restore non-fixed point).
	nextDiffBig = nextDiffBig.Rsh(nextDiffBig, 32)
	nextDiffTicketPool := nextDiffBig.Int64()

	// Check to see if we're over the limits for the maximum allowable retarget;
	// if we are, return the maximum or minimum except in the case that oldDiff
	// is zero.
	if oldDiff == 0 { // This should never really happen, but in case it does...
		return nextDiffTicketPool, nil
	} else if nextDiffTicketPool == 0 {
		nextDiffTicketPool = oldDiff / maxRetarget
	} else if (nextDiffTicketPool / oldDiff) > (maxRetarget - 1) {
		nextDiffTicketPool = oldDiff * maxRetarget
	} else if (oldDiff / nextDiffTicketPool) > (maxRetarget - 1) {
		nextDiffTicketPool = oldDiff / maxRetarget
	}

	// The target number of new SStx per block for any given window period.
	targetForWindow := b.chainParams.StakeDiffWindowSize *
		int64(b.chainParams.TicketsPerBlock)

	// Regress through all of the previous blocks and store the percent changes
	// per window period; use bigInts to emulate 64.32 bit fixed point.
	oldNode = curNode
	windowFreshStake := int64(0)
	windowPeriod = int64(0)
	weights = uint64(0)

	for i := int64(0); ; i++ {
		// Add the fresh stake into the store for this window period.
		windowFreshStake += int64(oldNode.header.FreshStake)

		// Store and reset after reaching the end of every window period.
		if (i+1)%b.chainParams.StakeDiffWindowSize == 0 {
			// Don't let fresh stake be zero.
			if windowFreshStake <= 0 {
				windowFreshStake = 1
			}

			freshTemp := big.NewInt(windowFreshStake)
			freshTemp.Lsh(freshTemp, 32) // Add padding
			targetTemp := big.NewInt(targetForWindow)

			// Get the percentage change.
			windowAdjusted := freshTemp.Div(freshTemp, targetTemp)

			// Weight it exponentially. Be aware that this could at some point
			// overflow if alpha or the number of blocks used is really large.
			windowAdjusted = windowAdjusted.Lsh(windowAdjusted,
				uint((b.chainParams.StakeDiffWindows-windowPeriod)*alpha))

			// Sum up all the different weights incrementally.
			weights += 1 <<
				uint64((b.chainParams.StakeDiffWindows-windowPeriod)*alpha)

			// Store it in the slice.
			windowChanges[windowPeriod] = windowAdjusted

			windowFreshStake = 0
			windowPeriod++
		}

		if (i + 1) == nodesToTraverse {
			break // Exit for loop when we hit the end.
		}

		// Get the previous block node.
		var err error
		tempNode := oldNode
		oldNode, err = b.getPrevNodeFromNode(oldNode)
		if err != nil {
			return 0, err
		}

		// If we're at the genesis block, reset the oldNode
		// so that it stays at the genesis block.
		if oldNode == nil {
			oldNode = tempNode
		}
	}

	// Sum up the weighted window periods.
	weightedSum = big.NewInt(0)
	for i := int64(0); i < b.chainParams.StakeDiffWindows; i++ {
		weightedSum.Add(weightedSum, windowChanges[i])
	}

	// Divide by the sum of all weights.
	weightsBig = big.NewInt(int64(weights))
	weightedSumDiv = weightedSum.Div(weightedSum, weightsBig)

	// Multiply by the old stake diff.
	oldDiffBig = big.NewInt(oldDiff)
	nextDiffBig = weightedSumDiv.Mul(weightedSumDiv, oldDiffBig)

	// Right shift to restore the original padding (restore non-fixed point).
	nextDiffBig = nextDiffBig.Rsh(nextDiffBig, 32)
	nextDiffFreshStake := nextDiffBig.Int64()

	// Check to see if we're over the limits for the maximum allowable retarget;
	// if we are, return the maximum or minimum except in the case that oldDiff
	// is zero.
	if oldDiff == 0 { // This should never really happen, but in case it does...
		return nextDiffFreshStake, nil
	} else if nextDiffFreshStake == 0 {
		nextDiffFreshStake = oldDiff / maxRetarget
	} else if (nextDiffFreshStake / oldDiff) > (maxRetarget - 1) {
		nextDiffFreshStake = oldDiff * maxRetarget
	} else if (oldDiff / nextDiffFreshStake) > (maxRetarget - 1) {
		nextDiffFreshStake = oldDiff / maxRetarget
	}

	// Average the two differences using scaled multiplication.
	nextDiff := mergeDifficulty(oldDiff, nextDiffTicketPool, nextDiffFreshStake)

	// Check to see if we're over the limits for the maximum allowable retarget;
	// if we are, return the maximum or minimum except in the case that oldDiff
	// is zero.
	if oldDiff == 0 { // This should never really happen, but in case it does...
		return oldDiff, nil
	} else if nextDiff == 0 {
		nextDiff = oldDiff / maxRetarget
	} else if (nextDiff / oldDiff) > (maxRetarget - 1) {
		nextDiff = oldDiff * maxRetarget
	} else if (oldDiff / nextDiff) > (maxRetarget - 1) {
		nextDiff = oldDiff / maxRetarget
	}

	// If the next diff is below the network minimum, set the required stake
	// difficulty to the minimum.
	if nextDiff < b.chainParams.MinimumStakeDiff {
		return b.chainParams.MinimumStakeDiff, nil
	}

	return nextDiff, nil
}

// CalcNextRequiredStakeDifficulty is the exported version of the above function.
// This function is NOT safe for concurrent access.
func (b *BlockChain) CalcNextRequiredStakeDifficulty() (int64, error) {
	return b.calcNextRequiredStakeDifficulty(b.bestNode)
}

// estimateNextStakeDifficulty returns a user-specified estimate for the next
// stake difficulty, with the passed ticketsInWindow indicating the number of
// fresh stake to pretend exists within this window. Optionally the user can
// also override this variable with useMaxTickets, which simply plugs in the
// maximum number of tickets the user can try.
func (b *BlockChain) estimateNextStakeDifficulty(curNode *blockNode,
	ticketsInWindow int64, useMaxTickets bool) (int64, error) {
	alpha := b.chainParams.StakeDiffAlpha
	stakeDiffStartHeight := int64(b.chainParams.CoinbaseMaturity) +
		1
	maxRetarget := int64(b.chainParams.RetargetAdjustmentFactor)
	TicketPoolWeight := int64(b.chainParams.TicketPoolSizeWeight)

	// Number of nodes to traverse while calculating difficulty.
	nodesToTraverse := (b.chainParams.StakeDiffWindowSize *
		b.chainParams.StakeDiffWindows)

	// Genesis block. Block at height 1 has these parameters.
	if curNode == nil ||
		curNode.height < stakeDiffStartHeight {
		return b.chainParams.MinimumStakeDiff, nil
	}

	// Create a fake blockchain on top of the current best node with
	// the number of freshly purchased tickets as indicated by the
	// user.
	oldDiff := curNode.header.SBits
	topNode := curNode
	if (curNode.height+1)%b.chainParams.StakeDiffWindowSize != 0 {
		nextAdjHeight := ((curNode.height /
			b.chainParams.StakeDiffWindowSize) + 1) *
			b.chainParams.StakeDiffWindowSize
		maxTickets := (nextAdjHeight - curNode.height) *
			int64(b.chainParams.MaxFreshStakePerBlock)

		// If the user has indicated that the automatically
		// calculated maximum amount of tickets should be
		// used, plug that in here.
		if useMaxTickets {
			ticketsInWindow = maxTickets
		}

		// Double check to make sure there isn't too much.
		if ticketsInWindow > maxTickets {
			return 0, fmt.Errorf("too much fresh stake to be used "+
				"in evaluation requested; max %v, got %v", maxTickets,
				ticketsInWindow)
		}

		// Insert all the tickets into bogus nodes that will be
		// used to calculate the next difficulty below.
		ticketsToInsert := ticketsInWindow
		for i := curNode.height + 1; i < nextAdjHeight; i++ {
			emptyHeader := new(wire.BlockHeader)
			emptyHeader.Height = uint32(i)

			// User a constant pool size for estimate, since
			// this has much less fluctuation than freshStake.
			// TODO Use a better pool size estimate?
			emptyHeader.PoolSize = curNode.header.PoolSize

			// Insert the fake fresh stake into each block,
			// decrementing the amount we need to use each
			// time until we hit 0.
			freshStake := b.chainParams.MaxFreshStakePerBlock
			if int64(freshStake) > ticketsToInsert {
				freshStake = uint8(ticketsToInsert)
				ticketsToInsert -= ticketsToInsert
			} else {
				ticketsToInsert -= int64(b.chainParams.MaxFreshStakePerBlock)
			}
			emptyHeader.FreshStake = freshStake

			// Connect the header.
			emptyHeader.PrevBlock = *topNode.hash

			// Make up a node hash.
			hB, err := emptyHeader.Bytes()
			if err != nil {
				return 0, err
			}
			emptyHeaderHash := chainhash.HashFuncH(hB)

			thisNode := new(blockNode)
			thisNode.header = *emptyHeader
			thisNode.hash = &emptyHeaderHash
			thisNode.height = i
			thisNode.parent = topNode
			topNode = thisNode
		}
	}

	// The target size of the ticketPool in live tickets. Recast these as int64
	// to avoid possible overflows for large sizes of either variable in
	// params.
	targetForTicketPool := int64(b.chainParams.TicketsPerBlock) *
		int64(b.chainParams.TicketPoolSize)

	// Initialize bigInt slice for the percentage changes for each window period
	// above or below the target.
	windowChanges := make([]*big.Int, b.chainParams.StakeDiffWindows)

	// Regress through all of the previous blocks and store the percent changes
	// per window period; use bigInts to emulate 64.32 bit fixed point.
	oldNode := topNode
	windowPeriod := int64(0)
	weights := uint64(0)

	for i := int64(0); ; i++ {
		// Store and reset after reaching the end of every window period.
		if (i+1)%b.chainParams.StakeDiffWindowSize == 0 {
			// First adjust based on ticketPoolSize. Skew the difference
			// in ticketPoolSize by max adjustment factor to help
			// weight ticket pool size versus tickets per block.
			poolSizeSkew := (int64(oldNode.header.PoolSize)-
				targetForTicketPool)*TicketPoolWeight + targetForTicketPool

			// Don't let this be negative or zero.
			if poolSizeSkew <= 0 {
				poolSizeSkew = 1
			}

			curPoolSizeTemp := big.NewInt(poolSizeSkew)
			curPoolSizeTemp.Lsh(curPoolSizeTemp, 32) // Add padding
			targetTemp := big.NewInt(targetForTicketPool)

			windowAdjusted := curPoolSizeTemp.Div(curPoolSizeTemp, targetTemp)

			// Weight it exponentially. Be aware that this could at some point
			// overflow if alpha or the number of blocks used is really large.
			windowAdjusted = windowAdjusted.Lsh(windowAdjusted,
				uint((b.chainParams.StakeDiffWindows-windowPeriod)*alpha))

			// Sum up all the different weights incrementally.
			weights += 1 << uint64((b.chainParams.StakeDiffWindows-windowPeriod)*
				alpha)

			// Store it in the slice.
			windowChanges[windowPeriod] = windowAdjusted

			// windowFreshStake = 0
			windowPeriod++
		}

		if (i + 1) == nodesToTraverse {
			break // Exit for loop when we hit the end.
		}

		// Get the previous block node.
		var err error
		tempNode := oldNode
		oldNode, err = b.getPrevNodeFromNode(oldNode)
		if err != nil {
			return 0, err
		}

		// If we're at the genesis block, reset the oldNode
		// so that it stays at the genesis block.
		if oldNode == nil {
			oldNode = tempNode
		}
	}

	// Sum up the weighted window periods.
	weightedSum := big.NewInt(0)
	for i := int64(0); i < b.chainParams.StakeDiffWindows; i++ {
		weightedSum.Add(weightedSum, windowChanges[i])
	}

	// Divide by the sum of all weights.
	weightsBig := big.NewInt(int64(weights))
	weightedSumDiv := weightedSum.Div(weightedSum, weightsBig)

	// Multiply by the old stake diff.
	oldDiffBig := big.NewInt(oldDiff)
	nextDiffBig := weightedSumDiv.Mul(weightedSumDiv, oldDiffBig)

	// Right shift to restore the original padding (restore non-fixed point).
	nextDiffBig = nextDiffBig.Rsh(nextDiffBig, 32)
	nextDiffTicketPool := nextDiffBig.Int64()

	// Check to see if we're over the limits for the maximum allowable retarget;
	// if we are, return the maximum or minimum except in the case that oldDiff
	// is zero.
	if oldDiff == 0 { // This should never really happen, but in case it does...
		return nextDiffTicketPool, nil
	} else if nextDiffTicketPool == 0 {
		nextDiffTicketPool = oldDiff / maxRetarget
	} else if (nextDiffTicketPool / oldDiff) > (maxRetarget - 1) {
		nextDiffTicketPool = oldDiff * maxRetarget
	} else if (oldDiff / nextDiffTicketPool) > (maxRetarget - 1) {
		nextDiffTicketPool = oldDiff / maxRetarget
	}

	// The target number of new SStx per block for any given window period.
	targetForWindow := b.chainParams.StakeDiffWindowSize *
		int64(b.chainParams.TicketsPerBlock)

	// Regress through all of the previous blocks and store the percent changes
	// per window period; use bigInts to emulate 64.32 bit fixed point.
	oldNode = topNode
	windowFreshStake := int64(0)
	windowPeriod = int64(0)
	weights = uint64(0)

	for i := int64(0); ; i++ {
		// Add the fresh stake into the store for this window period.
		windowFreshStake += int64(oldNode.header.FreshStake)

		// Store and reset after reaching the end of every window period.
		if (i+1)%b.chainParams.StakeDiffWindowSize == 0 {
			// Don't let fresh stake be zero.
			if windowFreshStake <= 0 {
				windowFreshStake = 1
			}

			freshTemp := big.NewInt(windowFreshStake)
			freshTemp.Lsh(freshTemp, 32) // Add padding
			targetTemp := big.NewInt(targetForWindow)

			// Get the percentage change.
			windowAdjusted := freshTemp.Div(freshTemp, targetTemp)

			// Weight it exponentially. Be aware that this could at some point
			// overflow if alpha or the number of blocks used is really large.
			windowAdjusted = windowAdjusted.Lsh(windowAdjusted,
				uint((b.chainParams.StakeDiffWindows-windowPeriod)*alpha))

			// Sum up all the different weights incrementally.
			weights += 1 <<
				uint64((b.chainParams.StakeDiffWindows-windowPeriod)*alpha)

			// Store it in the slice.
			windowChanges[windowPeriod] = windowAdjusted

			windowFreshStake = 0
			windowPeriod++
		}

		if (i + 1) == nodesToTraverse {
			break // Exit for loop when we hit the end.
		}

		// Get the previous block node.
		var err error
		tempNode := oldNode
		oldNode, err = b.getPrevNodeFromNode(oldNode)
		if err != nil {
			return 0, err
		}

		// If we're at the genesis block, reset the oldNode
		// so that it stays at the genesis block.
		if oldNode == nil {
			oldNode = tempNode
		}
	}

	// Sum up the weighted window periods.
	weightedSum = big.NewInt(0)
	for i := int64(0); i < b.chainParams.StakeDiffWindows; i++ {
		weightedSum.Add(weightedSum, windowChanges[i])
	}

	// Divide by the sum of all weights.
	weightsBig = big.NewInt(int64(weights))
	weightedSumDiv = weightedSum.Div(weightedSum, weightsBig)

	// Multiply by the old stake diff.
	oldDiffBig = big.NewInt(oldDiff)
	nextDiffBig = weightedSumDiv.Mul(weightedSumDiv, oldDiffBig)

	// Right shift to restore the original padding (restore non-fixed point).
	nextDiffBig = nextDiffBig.Rsh(nextDiffBig, 32)
	nextDiffFreshStake := nextDiffBig.Int64()

	// Check to see if we're over the limits for the maximum allowable retarget;
	// if we are, return the maximum or minimum except in the case that oldDiff
	// is zero.
	if oldDiff == 0 { // This should never really happen, but in case it does...
		return nextDiffFreshStake, nil
	} else if nextDiffFreshStake == 0 {
		nextDiffFreshStake = oldDiff / maxRetarget
	} else if (nextDiffFreshStake / oldDiff) > (maxRetarget - 1) {
		nextDiffFreshStake = oldDiff * maxRetarget
	} else if (oldDiff / nextDiffFreshStake) > (maxRetarget - 1) {
		nextDiffFreshStake = oldDiff / maxRetarget
	}

	// Average the two differences using scaled multiplication.
	nextDiff := mergeDifficulty(oldDiff, nextDiffTicketPool, nextDiffFreshStake)

	// Check to see if we're over the limits for the maximum allowable retarget;
	// if we are, return the maximum or minimum except in the case that oldDiff
	// is zero.
	if oldDiff == 0 { // This should never really happen, but in case it does...
		return oldDiff, nil
	} else if nextDiff == 0 {
		nextDiff = oldDiff / maxRetarget
	} else if (nextDiff / oldDiff) > (maxRetarget - 1) {
		nextDiff = oldDiff * maxRetarget
	} else if (oldDiff / nextDiff) > (maxRetarget - 1) {
		nextDiff = oldDiff / maxRetarget
	}

	// If the next diff is below the network minimum, set the required stake
	// difficulty to the minimum.
	if nextDiff < b.chainParams.MinimumStakeDiff {
		return b.chainParams.MinimumStakeDiff, nil
	}

	return nextDiff, nil
}

// EstimateNextStakeDifficulty is the exported version of the above function.
// This function is NOT safe for concurrent access.
func (b *BlockChain) EstimateNextStakeDifficulty(ticketsInWindow int64,
	useMaxTickets bool) (int64, error) {
	return b.estimateNextStakeDifficulty(b.bestNode, ticketsInWindow,
		useMaxTickets)
}
