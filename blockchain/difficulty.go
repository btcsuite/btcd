// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math/big"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

var (
	// bigOne is 1 represented as a big.Int.  It is defined here to avoid
	// the overhead of creating it multiple times.
	bigOne = big.NewInt(1)

	// oneLsh256 is 1 shifted left 256 bits.  It is defined here to avoid
	// the overhead of creating it multiple times.
	oneLsh256 = new(big.Int).Lsh(bigOne, 256)
)

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
// This compact form is only used in bitcoin to encode unsigned 256-bit numbers
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

// CalcWork calculates a work value from difficulty bits.  Bitcoin increases
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

func (b *BlockChain) GetNextWorkRequired(header *wire.BlockHeader) (uint32, error) {
	prevBlock := b.bestChain.Tip()
	// genesis block
	if prevBlock == nil {
		return BigToCompact(b.chainParams.PowLimit), nil
	}

	// Special rule for regTest: we never retarget.
	if b.chainParams == &chaincfg.RegressionNetParams ||
		b.chainParams == &chaincfg.SimNetParams {
		return prevBlock.bits, nil
	}

	if IsDAAEnabled(prevBlock, b.chainParams) {
		return b.getNextCashWorkRequired(prevBlock, header)
	}

	return b.getNextEDAWorkRequired(prevBlock, header)
}

// GetNextCashWorkRequired Compute the next required proof of work using a weighted
// average of the estimated hashRate per block.
//
// Using a weighted average ensure that the timestamp parameter cancels out in
// most of the calculation - except for the timestamp of the first and last
// block. Because timestamps are the least trustworthy information we have as
// input, this ensures the algorithm is more resistant to malicious inputs.
func (b *BlockChain) getNextCashWorkRequired(prevBlock *blockNode,
	header *wire.BlockHeader) (uint32, error) {

	// Special difficulty rule for testnet:
	// If the new block's timestamp is more than 2* 10 minutes then allow
	// mining of a min-difficulty block.
	if b.chainParams.ReduceMinDifficulty &&
		(int64(header.Timestamp.Unix()) >
			(prevBlock.timestamp + int64(2*b.chainParams.TargetTimePerBlock.Seconds()))) {
		return b.chainParams.PowLimitBits, nil
	}

	// Compute the difficulty based on the full adjustment interval.
	// Get the last suitable block of the difficulty interval.
	lastNode := b.getSuitableBlock(prevBlock)

	// Get the first suitable block of the difficulty interval.
	firstHeight := prevBlock.height - 144
	firstNode := b.getSuitableBlock(b.bestChain.NodeByHeight(firstHeight))
	if firstNode == nil {
		panic("the firstNode should not equal nil")
	}

	// Compute the target based on time and work done during the interval.
	nextTarget := b.computeTarget(firstNode, lastNode)
	if nextTarget.Cmp(b.chainParams.PowLimit) > 0 {
		return b.chainParams.PowLimitBits, nil
	}

	return BigToCompact(nextTarget), nil
}

// getNextEDAWorkRequired Compute the next required proof of work using the
// legacy Bitcoin difficulty adjustment + Emergency Difficulty Adjustment (EDA).
func (b *BlockChain) getNextEDAWorkRequired(prevBlock *blockNode,
	header *wire.BlockHeader) (uint32, error) {

	// Only change once per difficulty adjustment interval
	curHeight := prevBlock.height + 1
	if int64(curHeight)%b.chainParams.DifficultyAdjustmentInterval() == 0 &&
		int64(curHeight) >= b.chainParams.DifficultyAdjustmentInterval() {
		// Go back by what we want to be 14 days worth of blocks
		firstHeight := curHeight - int32(b.chainParams.DifficultyAdjustmentInterval())
		firstNode := b.bestChain.NodeByHeight(firstHeight)

		return b.calculateNextWorkRequired(prevBlock, firstNode.timestamp)
	}

	proofOfWorkLimit := b.chainParams.PowLimitBits
	if b.chainParams.ReduceMinDifficulty {
		// Special difficulty rule for testnet:
		// If the new block's timestamp is more than 2* 10 minutes then allow
		// mining of a min-difficulty block.
		if int64(header.Timestamp.Second()) >
			prevBlock.timestamp+2*int64(b.chainParams.TargetTimePerBlock.Seconds()) {
			return proofOfWorkLimit, nil
		}

		// Return the last non-special-min-difficulty-rules-block
		node := prevBlock
		for node.parent != nil &&
			int64(node.height)%b.chainParams.DifficultyAdjustmentInterval() != 0 &&
			node.bits == proofOfWorkLimit {
			node = node.parent
		}

		return node.bits, nil
	}

	// We can't go bellow the minimum, so early bail.
	bits := prevBlock.bits
	if bits == proofOfWorkLimit {
		return proofOfWorkLimit, nil
	}

	// If producing the last 6 block took less than 12h, we keep the same
	// difficulty
	node6 := b.bestChain.NodeByHeight(curHeight - 7)
	if node6 == nil {
		panic("the block Index should not equal nil")
	}
	mtp6Blocks := prevBlock.CalcPastMedianTime().Unix() - node6.CalcPastMedianTime().Unix()
	if mtp6Blocks < 12*3600 {
		return bits, nil
	}

	// If producing the last 6 blocks took more than 12h, increase the
	// difficulty target by 1/4 (which reduces the difficulty by 20%).
	// This ensures that the chain does not get stuck in case we lose
	// hashrate abruptly.
	pow := CompactToBig(bits)
	pow.Add(pow, new(big.Int).Div(pow, big.NewInt(4)))

	// Make sure we do not go bellow allowed values.
	powLimit := CompactToBig(b.chainParams.PowLimitBits)
	if pow.Cmp(powLimit) > 0 {
		pow = powLimit
	}

	return BigToCompact(pow), nil
}

func (b *BlockChain) calculateNextWorkRequired(prevNode *blockNode,
	firstBlockTime int64) (uint32, error) {

	if b.chainParams == &chaincfg.RegressionNetParams ||
		b.chainParams == &chaincfg.SimNetParams {
		return prevNode.bits, nil
	}

	// Limit adjustment step
	actualTimeSpan := prevNode.timestamp - firstBlockTime
	targetTimeSpan := int64(b.chainParams.TargetTimespan.Seconds())
	if actualTimeSpan < targetTimeSpan/4 {
		actualTimeSpan = targetTimeSpan / 4
	}

	if actualTimeSpan > targetTimeSpan*4 {
		actualTimeSpan = targetTimeSpan * 4
	}

	// Retarget
	bnNew := CompactToBig(prevNode.bits)
	bnNew.Mul(bnNew, big.NewInt(int64(actualTimeSpan)))
	bnNew.Div(bnNew, big.NewInt(targetTimeSpan))
	if bnNew.Cmp(b.chainParams.PowLimit) > 0 {
		bnNew = b.chainParams.PowLimit
	}
	return BigToCompact(bnNew), nil
}

// computeTarget Compute the a target based on the work done between 2 blocks and the time
// required to produce that work.
func (b *BlockChain) computeTarget(indexFirst, indexLast *blockNode) *big.Int {
	if indexLast.height <= indexFirst.height {
		panic("indexLast height should be greater than indexFirst height ")
	}

	/**
	* From the total work done and the time it took to produce that much work,
	* we can deduce how much work we expect to be produced in the targeted time
	* between blocks.
	 */
	work := new(big.Int).Sub(indexLast.workSum, indexFirst.workSum)
	work.Mul(work, big.NewInt(int64(b.chainParams.TargetTimePerBlock.Seconds())))

	// In order to avoid difficulty cliffs, we bound the amplitude of the
	// adjustment we are going to do.
	if indexLast.timestamp <= indexFirst.timestamp {
		panic("indexLast time should greater than indexFirst time ")
	}
	actualTimeSpan := indexLast.timestamp - indexFirst.timestamp
	interval := int64(b.chainParams.TargetTimePerBlock.Seconds())
	if actualTimeSpan > 288*interval {
		actualTimeSpan = 288 * interval
	} else if actualTimeSpan < 72*interval {
		actualTimeSpan = 72 * interval
	}

	work.Div(work, big.NewInt(int64(actualTimeSpan)))
	/**
	 * We need to compute T = (2^256 / W) - 1 but 2^256 doesn't fit in 256 bits.
	 * By expressing 1 as W / W, we get (2^256 - W) / W, and we can compute
	 * 2^256 - W as the complement of W.
	 */
	return new(big.Int).Sub(new(big.Int).Div(oneLsh256, work), big.NewInt(1))
}

// To reduce the impact of timestamp manipulation, we select the block we are
// basing our computation on via a median of 3.
func (b *BlockChain) getSuitableBlock(lastNode *blockNode) *blockNode {
	// In order to avoid a block is a very skewed timestamp to have too much
	// influence, we select the median of the 3 top most nodes as a starting
	// point.
	nodes := make([]*blockNode, 3)
	nodes[2] = lastNode
	nodes[1] = lastNode.parent

	nodes[0] = nodes[1].parent

	// Sorting network.
	if nodes[0].timestamp > nodes[2].timestamp {
		nodes[0], nodes[2] = nodes[2], nodes[0]
	}

	if nodes[0].timestamp > nodes[1].timestamp {
		nodes[0], nodes[1] = nodes[1], nodes[0]
	}

	if nodes[1].timestamp > nodes[2].timestamp {
		nodes[1], nodes[2] = nodes[2], nodes[1]
	}

	// We should have our candidate in the middle now.
	return nodes[1]
}

func (b *BlockChain) CheckProofOfWork(hash *chainhash.Hash, bits uint32) bool {
	target := CompactToBig(bits)
	if target.Sign() <= 0 ||
		target.Cmp(b.chainParams.PowLimit) > 0 ||
		HashToBig(hash).Cmp(target) > 0 {

		return false
	}

	return true
}
