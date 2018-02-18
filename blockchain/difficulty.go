// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"math/big"
	"time"

	"github.com/decred/dcrd/chaincfg"
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
)

// maxShift is the maximum shift for a difficulty that resets (e.g.
// testnet difficulty).
const maxShift = uint(256)

// HashToBig converts a chainhash.Hash into a big.Int that can be used to
// perform math comparisons.
func HashToBig(hash *chainhash.Hash) *big.Int {
	// A Hash is in little-endian, but the big package wants the bytes in
	// big-endian, so reverse them.
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
func (b *BlockChain) calcEasiestDifficulty(bits uint32, duration time.Duration) uint32 {
	// Convert types used in the calculations below.
	durationVal := int64(duration)
	adjustmentFactor := big.NewInt(b.chainParams.RetargetAdjustmentFactor)
	maxRetargetTimespan := int64(b.chainParams.TargetTimespan) *
		b.chainParams.RetargetAdjustmentFactor

	// The test network rules allow minimum difficulty blocks once too much
	// time has elapsed without mining a block.
	if b.chainParams.ReduceMinDifficulty {
		if durationVal > int64(b.chainParams.MinDiffReductionTime) {
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
func (b *BlockChain) findPrevTestNetDifficulty(startNode *blockNode) (uint32, error) {
	// Search backwards through the chain for the last block without
	// the special rule applied.
	blocksPerRetarget := b.chainParams.WorkDiffWindowSize *
		b.chainParams.WorkDiffWindows
	iterNode := startNode
	for iterNode != nil && iterNode.height%blocksPerRetarget != 0 &&
		iterNode.bits == b.chainParams.PowLimitBits {

		// Get the previous block node.  This function is used over
		// simply accessing iterNode.parent directly as it will
		// dynamically create previous block nodes as needed.  This
		// helps allow only the pieces of the chain that are needed
		// to remain in memory.
		var err error
		iterNode, err = b.index.PrevNodeFromNode(iterNode)
		if err != nil {
			log.Errorf("PrevNodeFromNode: %v", err)
			return 0, err
		}
	}

	// Return the found difficulty or the minimum difficulty if no
	// appropriate block was found.
	lastBits := b.chainParams.PowLimitBits
	if iterNode != nil {
		lastBits = iterNode.bits
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
func (b *BlockChain) calcNextRequiredDifficulty(curNode *blockNode, newBlockTime time.Time) (uint32, error) {
	// Genesis block.
	if curNode == nil {
		return b.chainParams.PowLimitBits, nil
	}

	// Get the old difficulty; if we aren't at a block height where it changes,
	// just return this.
	oldDiff := curNode.bits
	oldDiffBig := CompactToBig(curNode.bits)

	// We're not at a retarget point, return the oldDiff.
	if (curNode.height+1)%b.chainParams.WorkDiffWindowSize != 0 {
		// For networks that support it, allow special reduction of the
		// required difficulty once too much time has elapsed without
		// mining a block.
		if b.chainParams.ReduceMinDifficulty {
			// Return minimum difficulty when more than the desired
			// amount of time has elapsed without mining a block.
			reductionTime := int64(b.chainParams.MinDiffReductionTime /
				time.Second)
			allowMinTime := curNode.timestamp + reductionTime

			// For every extra target timespan that passes, we halve the
			// difficulty.
			if newBlockTime.Unix() > allowMinTime {
				timePassed := newBlockTime.Unix() - curNode.timestamp
				timePassed -= reductionTime
				shifts := uint((timePassed / int64(b.chainParams.TargetTimePerBlock/
					time.Second)) + 1)

				// Scale the difficulty with time passed.
				oldTarget := CompactToBig(curNode.bits)
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
	nextDiffBigMin := CompactToBig(curNode.bits)
	nextDiffBigMin.Div(nextDiffBigMin, RAFBig)
	nextDiffBigMax := CompactToBig(curNode.bits)
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
	var olderTime, windowPeriod int64
	var weights uint64
	oldNode := curNode
	recentTime := curNode.timestamp

	for i := int64(0); ; i++ {
		// Store and reset after reaching the end of every window period.
		if i%b.chainParams.WorkDiffWindowSize == 0 && i != 0 {
			olderTime = oldNode.timestamp
			timeDifference := recentTime - olderTime

			// Just assume we're at the target (no change) if we've
			// gone all the way back to the genesis block.
			if oldNode.height == 0 {
				timeDifference = int64(b.chainParams.TargetTimespan /
					time.Second)
			}

			timeDifBig := big.NewInt(timeDifference)
			timeDifBig.Lsh(timeDifBig, 32) // Add padding
			targetTemp := big.NewInt(int64(b.chainParams.TargetTimespan /
				time.Second))

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

		// Get the previous block node.  This function is used over
		// simply accessing firstNode.parent directly as it will
		// dynamically create previous block nodes as needed.  This
		// helps allow only the pieces of the chain that are needed
		// to remain in memory.
		var err error
		tempNode := oldNode
		oldNode, err = b.index.PrevNodeFromNode(oldNode)
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
	log.Debugf("Old target %08x (%064x)", curNode.bits, oldDiffBig)
	log.Debugf("New target %08x (%064x)", nextDiffBits, CompactToBig(nextDiffBits))

	return nextDiffBits, nil
}

// CalcNextRequiredDiffFromNode calculates the required difficulty for the block
// given with the passed hash along with the given timestamp.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) CalcNextRequiredDiffFromNode(hash *chainhash.Hash, timestamp time.Time) (uint32, error) {
	// Fetch the block to get the difficulty for.
	node, err := b.findNode(hash, maxSearchDepth)
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
func (b *BlockChain) CalcNextRequiredDifficulty(timestamp time.Time) (uint32, error) {
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

// calcNextRequiredStakeDifficultyV1 calculates the required stake difficulty
// for the block after the passed previous block node based on exponentially
// weighted averages.
//
// NOTE: This is the original stake difficulty algorithm that was used at Decred
// launch.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) calcNextRequiredStakeDifficultyV1(curNode *blockNode) (int64, error) {
	alpha := b.chainParams.StakeDiffAlpha
	stakeDiffStartHeight := int64(b.chainParams.CoinbaseMaturity) +
		1
	maxRetarget := b.chainParams.RetargetAdjustmentFactor
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
	oldDiff := curNode.sbits
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
			poolSizeSkew := (int64(oldNode.poolSize)-
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
		oldNode, err = b.index.PrevNodeFromNode(oldNode)
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
		windowFreshStake += int64(oldNode.freshStake)

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
		oldNode, err = b.index.PrevNodeFromNode(oldNode)
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

// estimateSupply returns an estimate of the coin supply for the provided block
// height.  This is primarily used in the stake difficulty algorithm and relies
// on an estimate to simplify the necessary calculations.  The actual total
// coin supply as of a given block height depends on many factors such as the
// number of votes included in every prior block (not including all votes
// reduces the subsidy) and whether or not any of the prior blocks have been
// invalidated by stakeholders thereby removing the PoW subsidy for them.
//
// This function is safe for concurrent access.
func estimateSupply(params *chaincfg.Params, height int64) int64 {
	if height <= 0 {
		return 0
	}

	// Estimate the supply by calculating the full block subsidy for each
	// reduction interval and multiplying it the number of blocks in the
	// interval then adding the subsidy produced by number of blocks in the
	// current interval.
	supply := params.BlockOneSubsidy()
	reductions := height / params.SubsidyReductionInterval
	subsidy := params.BaseSubsidy
	for i := int64(0); i < reductions; i++ {
		supply += params.SubsidyReductionInterval * subsidy

		subsidy *= params.MulSubsidy
		subsidy /= params.DivSubsidy
	}
	supply += (1 + height%params.SubsidyReductionInterval) * subsidy

	// Blocks 0 and 1 have special subsidy amounts that have already been
	// added above, so remove what their subsidies would have normally been
	// which were also added above.
	supply -= params.BaseSubsidy * 2

	return supply
}

// sumPurchasedTickets returns the sum of the number of tickets purchased in the
// most recent specified number of blocks from the point of view of the passed
// node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) sumPurchasedTickets(startNode *blockNode, numToSum int64) (int64, error) {
	var numPurchased int64
	for node, numTraversed := startNode, int64(0); node != nil &&
		numTraversed < numToSum; numTraversed++ {

		numPurchased += int64(node.freshStake)

		// Get the previous block node.  This function is used over
		// simply accessing iterNode.parent directly as it will
		// dynamically create previous block nodes as needed.  This
		// helps allow only the pieces of the chain that are needed
		// to remain in memory.
		var err error
		node, err = b.index.PrevNodeFromNode(node)
		if err != nil {
			return 0, err
		}
	}

	return numPurchased, nil
}

// calcNextStakeDiffV2 calculates the next stake difficulty for the given set
// of parameters using the algorithm defined in DCP0001.
//
// This function contains the heart of the algorithm and thus is separated for
// use in both the actual stake difficulty calculation as well as estimation.
//
// The caller must perform all of the necessary chain traversal in order to
// get the current difficulty, previous retarget interval's pool size plus
// its immature tickets, as well as the current pool size plus immature tickets.
//
// This function is safe for concurrent access.
func calcNextStakeDiffV2(params *chaincfg.Params, nextHeight, curDiff, prevPoolSizeAll, curPoolSizeAll int64) int64 {
	// Shorter version of various parameter for convenience.
	votesPerBlock := int64(params.TicketsPerBlock)
	ticketPoolSize := int64(params.TicketPoolSize)
	ticketMaturity := int64(params.TicketMaturity)

	// Calculate the difficulty by multiplying the old stake difficulty
	// with two ratios that represent a force to counteract the relative
	// change in the pool size (Fc) and a restorative force to push the pool
	// size  towards the target value (Fr).
	//
	// Per DCP0001, the generalized equation is:
	//
	//   nextDiff = min(max(curDiff * Fc * Fr, Slb), Sub)
	//
	// The detailed form expands to:
	//
	//                        curPoolSizeAll      curPoolSizeAll
	//   nextDiff = curDiff * ---------------  * -----------------
	//                        prevPoolSizeAll    targetPoolSizeAll
	//
	//   Slb = b.chainParams.MinimumStakeDiff
	//
	//               estimatedTotalSupply
	//   Sub = -------------------------------
	//          targetPoolSize / votesPerBlock
	//
	// In order to avoid the need to perform floating point math which could
	// be problematic across languages due to uncertainty in floating point
	// math libs, this is further simplified to integer math as follows:
	//
	//                   curDiff * curPoolSizeAll^2
	//   nextDiff = -----------------------------------
	//              prevPoolSizeAll * targetPoolSizeAll
	//
	// Further, the Sub parameter must calculate the denomitor first using
	// integer math.
	targetPoolSizeAll := votesPerBlock * (ticketPoolSize + ticketMaturity)
	curPoolSizeAllBig := big.NewInt(curPoolSizeAll)
	nextDiffBig := big.NewInt(curDiff)
	nextDiffBig.Mul(nextDiffBig, curPoolSizeAllBig)
	nextDiffBig.Mul(nextDiffBig, curPoolSizeAllBig)
	nextDiffBig.Div(nextDiffBig, big.NewInt(prevPoolSizeAll))
	nextDiffBig.Div(nextDiffBig, big.NewInt(targetPoolSizeAll))

	// Limit the new stake difficulty between the minimum allowed stake
	// difficulty and a maximum value that is relative to the total supply.
	//
	// NOTE: This is intentionally using integer math to prevent any
	// potential issues due to uncertainty in floating point math libs.  The
	// ticketPoolSize parameter already contains the result of
	// (targetPoolSize / votesPerBlock).
	nextDiff := nextDiffBig.Int64()
	estimatedSupply := estimateSupply(params, nextHeight)
	maximumStakeDiff := estimatedSupply / ticketPoolSize
	if nextDiff > maximumStakeDiff {
		nextDiff = maximumStakeDiff
	}
	if nextDiff < params.MinimumStakeDiff {
		nextDiff = params.MinimumStakeDiff
	}
	return nextDiff
}

// calcNextRequiredStakeDifficultyV2 calculates the required stake difficulty
// for the block after the passed previous block node based on the algorithm
// defined in DCP0001.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) calcNextRequiredStakeDifficultyV2(curNode *blockNode) (int64, error) {
	// Stake difficulty before any tickets could possibly be purchased is
	// the minimum value.
	nextHeight := int64(0)
	if curNode != nil {
		nextHeight = curNode.height + 1
	}
	stakeDiffStartHeight := int64(b.chainParams.CoinbaseMaturity) + 1
	if nextHeight < stakeDiffStartHeight {
		return b.chainParams.MinimumStakeDiff, nil
	}

	// Return the previous block's difficulty requirements if the next block
	// is not at a difficulty retarget interval.
	intervalSize := b.chainParams.StakeDiffWindowSize
	curDiff := curNode.sbits
	if nextHeight%intervalSize != 0 {
		return curDiff, nil
	}

	// Get the pool size and number of tickets that were immature at the
	// previous retarget interval.
	//
	// NOTE: Since the stake difficulty must be calculated based on existing
	// blocks, it is always calculated for the block after a given block, so
	// the information for the previous retarget interval must be retrieved
	// relative to the block just before it to coincide with how it was
	// originally calculated.
	var prevPoolSize int64
	prevRetargetHeight := nextHeight - intervalSize - 1
	prevRetargetNode, err := b.index.AncestorNode(curNode, prevRetargetHeight)
	if err != nil {
		return 0, err
	}
	if prevRetargetNode != nil {
		prevPoolSize = int64(prevRetargetNode.poolSize)
	}
	ticketMaturity := int64(b.chainParams.TicketMaturity)
	prevImmatureTickets, err := b.sumPurchasedTickets(prevRetargetNode,
		ticketMaturity)
	if err != nil {
		return 0, err
	}

	// Return the existing ticket price for the first few intervals to avoid
	// division by zero and encourage initial pool population.
	prevPoolSizeAll := prevPoolSize + prevImmatureTickets
	if prevPoolSizeAll == 0 {
		return curDiff, nil
	}

	// Count the number of currently immature tickets.
	immatureTickets, err := b.sumPurchasedTickets(curNode, ticketMaturity)
	if err != nil {
		return 0, err
	}

	// Calculate and return the final next required difficulty.
	curPoolSizeAll := int64(curNode.poolSize) + immatureTickets
	return calcNextStakeDiffV2(b.chainParams, nextHeight, curDiff,
		prevPoolSizeAll, curPoolSizeAll), nil
}

// sdiffAlgoDeploymentVersion returns the deployment vesion for the stake
// difficulty algorithm change as defined in DCP0001 for the provided network.
//
// This function is safe for concurrent access.
func sdiffAlgoDeploymentVersion(network wire.CurrencyNet) uint32 {
	if network != wire.MainNet {
		return 5
	}
	return 4
}

// calcNextRequiredStakeDifficulty calculates the required stake difficulty for
// the block after the passed previous block node based on the active stake
// difficulty retarget rules.
//
// This function differs from the exported CalcNextRequiredDifficulty in that
// the exported version uses the current best chain as the previous block node
// while this function accepts any block node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) calcNextRequiredStakeDifficulty(curNode *blockNode) (int64, error) {
	// Use the new stake difficulty algorithm if the stake vote for the new
	// algorithm agenda is active.
	//
	// NOTE: The choice field of the return threshold state is not examined
	// here because there is only one possible choice that can be active
	// for the agenda, which is yes, so there is no need to check it.
	deploymentVersion := sdiffAlgoDeploymentVersion(b.chainParams.Net)
	state, err := b.deploymentState(curNode, deploymentVersion,
		chaincfg.VoteIDSDiffAlgorithm)
	if err != nil {
		return 0, err
	}
	if state.State == ThresholdActive {
		return b.calcNextRequiredStakeDifficultyV2(curNode)
	}

	// Use the old stake difficulty algorithm in any other case.
	return b.calcNextRequiredStakeDifficultyV1(curNode)
}

// CalcNextRequiredStakeDifficulty calculates the required stake difficulty for
// the block after the end of the current best chain based on the active stake
// difficulty retarget rules.
//
// This function is safe for concurrent access.
func (b *BlockChain) CalcNextRequiredStakeDifficulty() (int64, error) {
	b.chainLock.Lock()
	nextDiff, err := b.calcNextRequiredStakeDifficulty(b.bestNode)
	b.chainLock.Unlock()
	return nextDiff, err
}

// estimateNextStakeDifficultyV1 estimates the next stake difficulty by
// pretending the provided number of tickets will be purchased in the remainder
// of the interval unless the flag to use max tickets is set in which case it
// will use the max possible number of tickets that can be purchased in the
// remainder of the interval.
//
// NOTE: This uses the original stake difficulty algorithm that was used at
// Decred launch.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) estimateNextStakeDifficultyV1(curNode *blockNode, ticketsInWindow int64, useMaxTickets bool) (int64, error) {
	alpha := b.chainParams.StakeDiffAlpha
	stakeDiffStartHeight := int64(b.chainParams.CoinbaseMaturity) +
		1
	maxRetarget := b.chainParams.RetargetAdjustmentFactor
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
	oldDiff := curNode.sbits
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
			var emptyHeader wire.BlockHeader
			emptyHeader.Height = uint32(i)

			// User a constant pool size for estimate, since
			// this has much less fluctuation than freshStake.
			// TODO Use a better pool size estimate?
			emptyHeader.PoolSize = curNode.poolSize

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
			emptyHeader.PrevBlock = topNode.hash

			thisNode := newBlockNode(&emptyHeader, topNode)
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
			poolSizeSkew := (int64(oldNode.poolSize)-
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
		oldNode, err = b.index.PrevNodeFromNode(oldNode)
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
		windowFreshStake += int64(oldNode.freshStake)

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
		oldNode, err = b.index.PrevNodeFromNode(oldNode)
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

// estimateNextStakeDifficultyV2 estimates the next stake difficulty using the
// algorithm defined in DCP0001 by pretending the provided number of tickets
// will be purchased in the remainder of the interval unless the flag to use max
// tickets is set in which case it will use the max possible number of tickets
// that can be purchased in the remainder of the interval.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) estimateNextStakeDifficultyV2(curNode *blockNode, newTickets int64, useMaxTickets bool) (int64, error) {
	// Calculate the next retarget interval height.
	curHeight := int64(0)
	if curNode != nil {
		curHeight = curNode.height
	}
	intervalSize := b.chainParams.StakeDiffWindowSize
	blocksUntilRetarget := intervalSize - curHeight%intervalSize
	nextRetargetHeight := curHeight + blocksUntilRetarget

	// This code really should be updated to work with retarget interval
	// size greater than the ticket maturity, such as is the case on
	// testnet, but since it does not currently work under that scenario,
	// return an error rather than incorrect results.
	ticketMaturity := int64(b.chainParams.TicketMaturity)
	if intervalSize > ticketMaturity {
		return 0, fmt.Errorf("stake difficulty estimation does not "+
			"currently work when the retarget interval is larger "+
			"than the ticket maturity (interval %d, ticket "+
			"maturity %d)", intervalSize, ticketMaturity)
	}

	// Calculate the maximum possible number of tickets that could be sold
	// in the remainder of the interval and potentially override the number
	// of new tickets to include in the estimate per the user-specified
	// flag.
	maxTicketsPerBlock := int64(b.chainParams.MaxFreshStakePerBlock)
	maxRemainingTickets := (blocksUntilRetarget - 1) * maxTicketsPerBlock
	if useMaxTickets {
		newTickets = maxRemainingTickets
	}

	// Ensure the specified number of tickets is not too high.
	if newTickets > maxRemainingTickets {
		return 0, fmt.Errorf("unable to create an estimated stake "+
			"difficulty with %d tickets since it is more than "+
			"the maximum remaining of %d", newTickets,
			maxRemainingTickets)
	}

	// Stake difficulty before any tickets could possibly be purchased is
	// the minimum value.
	stakeDiffStartHeight := int64(b.chainParams.CoinbaseMaturity) + 1
	if nextRetargetHeight < stakeDiffStartHeight {
		return b.chainParams.MinimumStakeDiff, nil
	}

	// Get the pool size and number of tickets that were immature at the
	// previous retarget interval
	//
	// NOTE: Since the stake difficulty must be calculated based on existing
	// blocks, it is always calculated for the block after a given block, so
	// the information for the previous retarget interval must be retrieved
	// relative to the block just before it to coincide with how it was
	// originally calculated.
	var prevPoolSize int64
	prevRetargetHeight := nextRetargetHeight - intervalSize - 1
	prevRetargetNode, err := b.index.AncestorNode(curNode, prevRetargetHeight)
	if err != nil {
		return 0, err
	}
	if prevRetargetNode != nil {
		prevPoolSize = int64(prevRetargetNode.poolSize)
	}
	prevImmatureTickets, err := b.sumPurchasedTickets(prevRetargetNode,
		ticketMaturity)
	if err != nil {
		return 0, err
	}

	// Return the existing ticket price for the first few intervals to avoid
	// division by zero and encourage initial pool population.
	curDiff := curNode.sbits
	prevPoolSizeAll := prevPoolSize + prevImmatureTickets
	if prevPoolSizeAll == 0 {
		return curDiff, nil
	}

	// Calculate the number of tickets that will still be immature at the
	// next retarget based on the known data.
	nextMaturityFloor := nextRetargetHeight - ticketMaturity - 1
	remainingImmatureTickets, err := b.sumPurchasedTickets(curNode,
		curHeight-nextMaturityFloor)
	if err != nil {
		return 0, err
	}

	// Calculate the number of tickets that will mature in the remainder of
	// the interval.
	//
	// NOTE: The pool size in the block headers does not include the tickets
	// maturing at the height in which they mature since they are not
	// eligible for selection until the next block, so exclude them by
	// starting one block before the next maturity floor.
	nextMaturityFloorNode, err := b.index.AncestorNode(curNode,
		nextMaturityFloor-1)
	if err != nil {
		return 0, err
	}
	curMaturityFloor := curHeight - ticketMaturity
	maturingTickets, err := b.sumPurchasedTickets(nextMaturityFloorNode,
		nextMaturityFloor-curMaturityFloor)
	if err != nil {
		return 0, err
	}

	// Calculate the number of votes that will occur during the remainder of
	// the interval.
	stakeValidationHeight := b.chainParams.StakeValidationHeight
	var pendingVotes int64
	if nextRetargetHeight > stakeValidationHeight {
		votingBlocks := blocksUntilRetarget - 1
		if curHeight < stakeValidationHeight {
			votingBlocks = nextRetargetHeight - stakeValidationHeight
		}
		votesPerBlock := int64(b.chainParams.TicketsPerBlock)
		pendingVotes = votingBlocks * votesPerBlock
	}

	// Calculate what the pool size would be as of the next interval.
	curPoolSize := int64(curNode.poolSize)
	estimatedPoolSize := curPoolSize + maturingTickets - pendingVotes
	estimatedImmatureTickets := remainingImmatureTickets + newTickets
	estimatedPoolSizeAll := estimatedPoolSize + estimatedImmatureTickets

	// Calculate and return the final estimated difficulty.
	return calcNextStakeDiffV2(b.chainParams, nextRetargetHeight, curDiff,
		prevPoolSizeAll, estimatedPoolSizeAll), nil
}

// estimateNextStakeDifficulty estimates the next stake difficulty by pretending
// the provided number of tickets will be purchased in the remainder of the
// interval unless the flag to use max tickets is set in which case it will use
// the max possible number of tickets that can be purchased in the remainder of
// the interval.
//
// The stake difficulty algorithm is selected based on the active rules.
//
// This function differs from the exported EstimateNextStakeDifficulty in that
// the exported version uses the current best chain as the block node while this
// function accepts any block node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) estimateNextStakeDifficulty(curNode *blockNode, newTickets int64, useMaxTickets bool) (int64, error) {
	// Use the new stake difficulty algorithm if the stake vote for the new
	// algorithm agenda is active.
	//
	// NOTE: The choice field of the return threshold state is not examined
	// here because there is only one possible choice that can be active
	// for the agenda, which is yes, so there is no need to check it.
	deploymentVersion := sdiffAlgoDeploymentVersion(b.chainParams.Net)
	state, err := b.deploymentState(curNode, deploymentVersion,
		chaincfg.VoteIDSDiffAlgorithm)
	if err != nil {
		return 0, err
	}
	if state.State == ThresholdActive {
		return b.estimateNextStakeDifficultyV2(curNode, newTickets,
			useMaxTickets)
	}

	// Use the old stake difficulty algorithm in any other case.
	return b.estimateNextStakeDifficultyV1(curNode, newTickets,
		useMaxTickets)
}

// EstimateNextStakeDifficulty estimates the next stake difficulty by pretending
// the provided number of tickets will be purchased in the remainder of the
// interval unless the flag to use max tickets is set in which case it will use
// the max possible number of tickets that can be purchased in the remainder of
// the interval.
//
// This function is safe for concurrent access.
func (b *BlockChain) EstimateNextStakeDifficulty(newTickets int64, useMaxTickets bool) (int64, error) {
	b.chainLock.Lock()
	estimate, err := b.estimateNextStakeDifficulty(b.bestNode, newTickets,
		useMaxTickets)
	b.chainLock.Unlock()
	return estimate, err
}
