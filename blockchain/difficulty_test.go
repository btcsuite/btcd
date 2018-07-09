// Copyright (c) 2014-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math/big"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// TestBigToCompact ensures BigToCompact converts big integers to the expected
// compact representation.
func TestBigToCompact(t *testing.T) {
	tests := []struct {
		in  int64
		out uint32
	}{
		{0, 0},
		{-1, 25231360},
	}

	for x, test := range tests {
		n := big.NewInt(test.in)
		r := BigToCompact(n)
		if r != test.out {
			t.Errorf("TestBigToCompact test #%d failed: got %d want %d\n",
				x, r, test.out)
			return
		}
	}
}

// TestCompactToBig ensures CompactToBig converts numbers using the compact
// representation to the expected big intergers.
func TestCompactToBig(t *testing.T) {
	tests := []struct {
		in  uint32
		out int64
	}{
		{10000000, 0},
	}

	for x, test := range tests {
		n := CompactToBig(test.in)
		want := big.NewInt(test.out)
		if n.Cmp(want) != 0 {
			t.Errorf("TestCompactToBig test #%d failed: got %d want %d\n",
				x, n.Int64(), want.Int64())
			return
		}
	}
}

// TestCalcWork ensures CalcWork calculates the expected work value from values
// in compact representation.
func TestCalcWork(t *testing.T) {
	tests := []struct {
		in  uint32
		out int64
	}{
		{10000000, 0},
	}

	for x, test := range tests {
		bits := uint32(test.in)

		r := CalcWork(bits)
		if r.Int64() != test.out {
			t.Errorf("TestCalcWork test #%d failed: got %v want %d\n",
				x, r.Int64(), test.out)
			return
		}
	}
}

func TestGetNextWork(t *testing.T) {
	chain := BlockChain{
		chainParams: &chaincfg.MainNetParams,
	}

	// Test calculation of next difficulty target with no constraints applying
	lastRetargetTime := 1261130161 // Block #30240
	node := blockNode{
		height:    32255,
		timestamp: 1262152739, // Block #32255
		bits:      0x1d00ffff,
	}

	targetBits, err := chain.calculateNextWorkRequired(&node, int64(lastRetargetTime))
	if err != nil {
		panic(err)
	}
	if targetBits != 0x1d00d86a {
		t.Errorf("calculateNextWorkRequired() calculates target bits error, want: %d, but got: %d", 0x1d00d86a, targetBits)
	}

	// Test the constraint on the upper bound for next work
	lastRetargetTime = 1231006505 // Block #0
	node = blockNode{
		height:    2015,
		timestamp: 1233061996, // Block #2015
		bits:      0x1d00ffff,
	}

	targetBits, err = chain.calculateNextWorkRequired(&node, int64(lastRetargetTime))
	if err != nil {
		panic(err)
	}

	if targetBits != 0x1d00ffff {
		t.Errorf("calculateNextWorkRequired() calculates target bits error, want: %d, but got: %d", 0x1d00ffff, targetBits)
	}

	// Test the constraint on the lower bound for actual time taken
	lastRetargetTime = 1279008237 // Block #66528
	node = blockNode{
		height:    68543,
		timestamp: 1279297671, // Block #68543
		bits:      0x1c05a3f4,
	}

	targetBits, err = chain.calculateNextWorkRequired(&node, int64(lastRetargetTime))
	if err != nil {
		panic(err)
	}

	if targetBits != 0x1c0168fd {
		t.Errorf("calculateNextWorkRequired() calculates target bits error, want: %d, but got: %d", 0x1c0168fd, targetBits)
	}

	// Test the constraint on the upper bound for actual time taken
	lastRetargetTime = 1263163443 // NOTE: Not an actual block time
	node = blockNode{
		height:    46367,
		timestamp: 1269211443, // Block #46367
		bits:      0x1c387f6f,
	}

	targetBits, err = chain.calculateNextWorkRequired(&node, int64(lastRetargetTime))
	if err != nil {
		panic(err)
	}

	if targetBits != 0x1d00e1fd {
		t.Errorf("calculateNextWorkRequired() calculates target bits error, want: %d, but got: %d", 0x1d00e1fd, targetBits)
	}
}

func GetBlockNode(prevNode *blockNode, timeInterval int64, bits uint32) *blockNode {
	node := blockNode{
		parent:    prevNode,
		height:    prevNode.height + 1,
		timestamp: prevNode.timestamp + timeInterval,
		bits:      bits,
	}

	node.workSum = new(big.Int).Add(prevNode.workSum, CalcWork(node.bits))

	return &node
}

func NewBlockHeader(timestamp int64, bits uint32) *wire.BlockHeader {
	return &wire.BlockHeader{
		Timestamp: time.Unix(timestamp, 0),
		Bits:      bits,
	}
}

func AddBlockToChain(blocks []*blockNode, idx int, header *wire.BlockHeader, chain *BlockChain) {
	chain.index.index[header.BlockHash()] = blocks[idx]
	blocks[idx].hash = header.BlockHash()
	chain.bestChain.nodes = append(chain.bestChain.nodes, blocks[idx])
}

func TestRetarget(t *testing.T) {
	chain := &BlockChain{
		chainParams: &chaincfg.MainNetParams,
		index: &blockIndex{
			index: make(map[chainhash.Hash]*blockNode),
		},
		bestChain: &chainView{
			nodes: make([]*blockNode, 0),
		},
	}

	blocks := make([]*blockNode, 115)
	powLimit := chain.chainParams.PowLimit
	currentPow := new(big.Int).Rsh(powLimit, 1)
	initialBits := BigToCompact(currentPow)

	// Genesis block.
	blocks[0] = &blockNode{
		height:    0,
		timestamp: 1269211443,
		bits:      initialBits,
	}
	blocks[0].workSum = CalcWork(blocks[0].bits)
	blocks[0] = newBlockNode(NewBlockHeader(1269211443, initialBits), nil)
	header := blocks[0].Header()
	AddBlockToChain(blocks, 0, &header, chain)

	// Pile up some blocks.
	for i := 1; i < 100; i++ {
		blocks[i] = GetBlockNode(blocks[i-1], int64(chain.chainParams.TargetTimePerBlock.Seconds()), initialBits)
		header := blocks[i].Header()
		AddBlockToChain(blocks, i, &header, chain)
	}

	// We start getting 2h blocks time. For the first 5 blocks, it doesn't
	// matter as the MTP is not affected. For the next 5 block, MTP difference
	// increases but stays below 12h.
	for i := 100; i < 110; i++ {
		blocks[i] = GetBlockNode(blocks[i-1], 2*3600, initialBits)
		header = blocks[i].Header()
		AddBlockToChain(blocks, i, &header, chain)

		powTarget, err := chain.GetNextWorkRequired(&header)
		if err != nil {
			panic(err)
		}
		if powTarget != initialBits {
			t.Errorf("GetNextWorkRequired() calculates error, want: %d, but got: %d", initialBits, powTarget)
		}
	}

	// Now we expect the difficulty to decrease.
	blocks[110] = GetBlockNode(blocks[109], 2*3600, initialBits)
	currentPow = CompactToBig(BigToCompact(currentPow))
	currentPow.Add(currentPow, new(big.Int).Rsh(currentPow, 2))
	header = blocks[110].Header()
	AddBlockToChain(blocks, 110, &header, chain)

	powTarget, err := chain.GetNextWorkRequired(&header)
	if err != nil {
		panic(err)
	}
	if powTarget != BigToCompact(currentPow) {
		t.Errorf("GetNextWorkRequired() calculates error, want: %d, but got: %d", BigToCompact(currentPow), powTarget)
	}
}

func TestCashDifficulty(t *testing.T) {
	chain := &BlockChain{
		chainParams: &chaincfg.MainNetParams,
		index: &blockIndex{
			index: make(map[chainhash.Hash]*blockNode),
		},
		bestChain: &chainView{
			nodes: make([]*blockNode, 0),
		},
	}

	powLimit := chain.chainParams.PowLimit
	currentPow := new(big.Int).Rsh(powLimit, 4)
	initialBits := BigToCompact(currentPow)

	blocks := make([]*blockNode, 3000)
	// Genesis block.
	blocks[0] = &blockNode{
		height:    0,
		timestamp: 1269211443,
		bits:      initialBits,
	}
	blocks[0].workSum = CalcWork(blocks[0].bits)
	blocks[0] = newBlockNode(NewBlockHeader(1269211443, initialBits), nil)
	header := blocks[0].Header()
	AddBlockToChain(blocks, 0, &header, chain)

	// Block counter
	var i int
	// Pile up some blocks every 10 mins to establish some history.
	for i = 1; i < 2050; i++ {
		blocks[i] = GetBlockNode(blocks[i-1], 600, initialBits)
		header := blocks[i].Header()
		AddBlockToChain(blocks, i, &header, chain)
	}

	bits, err := chain.getNextCashWorkRequired(blocks[2049], nil)
	if err != nil {
		panic(err)
	}

	// Difficulty stays the same as long as we produce a block every 10 mins.
	for j := 0; j < 10; j++ {
		blocks[i] = GetBlockNode(blocks[i-1], 600, bits)
		header = blocks[i].Header()
		AddBlockToChain(blocks, i, &header, chain)

		powTarget, err := chain.getNextCashWorkRequired(blocks[i], nil)
		if err != nil {
			panic(err)
		}
		if powTarget != bits {
			t.Errorf("GetNextWorkRequired() calculates error, want: %d, but got: %d", bits, powTarget)
		}

		i++
	}

	// Make sure we skip over blocks that are out of wack. To do so, we produce
	// a block that is far in the future, and then produce a block with the
	// expected timestamp.
	blocks[i] = GetBlockNode(blocks[i-1], 6000, bits)
	header = blocks[i].Header()
	AddBlockToChain(blocks, i, &header, chain)
	powTarget, err := chain.getNextCashWorkRequired(blocks[i], nil)
	i++
	if err != nil {
		panic(err)
	}
	if powTarget != bits {
		t.Errorf("GetNextWorkRequired() calculates error, want: %d, but got: %d", bits, powTarget)
	}

	blocks[i] = GetBlockNode(blocks[i-1], 2*600-6000, bits)
	header = blocks[i].Header()
	AddBlockToChain(blocks, i, &header, chain)
	powTarget, err = chain.getNextCashWorkRequired(blocks[i], nil)
	i++
	if err != nil {
		panic(err)
	}
	if powTarget != bits {
		t.Errorf("GetNextWorkRequired() calculates error, want: %d, but got: %d", bits, powTarget)
	}

	// The system should continue unaffected by the block with a bogous
	// timestamps.
	for j := 0; j < 20; j++ {
		blocks[i] = GetBlockNode(blocks[i-1], 600, bits)
		header = blocks[i].Header()
		AddBlockToChain(blocks, i, &header, chain)
		powTarget, err = chain.getNextCashWorkRequired(blocks[i], nil)
		if err != nil {
			panic(err)
		}
		if powTarget != bits {
			t.Errorf("GetNextWorkRequired() calculates error, want: %d, but got: %d", bits, powTarget)
		}
		i++
	}

	// We start emitting blocks slightly faster. The first block has no impact.
	blocks[i] = GetBlockNode(blocks[i-1], 550, bits)
	header = blocks[i].Header()
	AddBlockToChain(blocks, i, &header, chain)
	powTarget, err = chain.getNextCashWorkRequired(blocks[i], nil)
	if err != nil {
		panic(err)
	}
	if powTarget != bits {
		t.Errorf("GetNextWorkRequired() calculates error, want: %d, but got: %d", bits, powTarget)
	}
	i++

	// Now we should see difficulty increase slowly.
	for j := 0; j < 10; j++ {
		blocks[i] = GetBlockNode(blocks[i-1], 550, bits)
		header = blocks[i].Header()
		AddBlockToChain(blocks, i, &header, chain)
		nextBits, err := chain.getNextCashWorkRequired(blocks[i], nil)
		if err != nil {
			panic(err)
		}

		currentTarget := CompactToBig(bits)
		nextTarget := CompactToBig(nextBits)

		// Make sure that difficulty increases very slowly.
		if nextTarget.Cmp(currentTarget) >= 0 {
			t.Error("difficulty should increase")
		}

		if new(big.Int).Sub(currentTarget, nextTarget).Cmp(new(big.Int).Rsh(currentTarget, 10)) >= 0 {
			t.Error("difficulty changes too large")
		}

		bits = nextBits
		i++
	}

	// Check the actual value.
	if bits != 0x1c0fe7b1 {
		t.Error("Calculated bits error")
	}

	// If we dramatically shorten block production, difficulty increases faster.
	for j := 0; j < 20; j++ {
		blocks[i] = GetBlockNode(blocks[i-1], 10, bits)
		header = blocks[i].Header()
		AddBlockToChain(blocks, i, &header, chain)
		nextBits, err := chain.getNextCashWorkRequired(blocks[i], nil)
		if err != nil {
			panic(err)
		}

		currentTarget := CompactToBig(bits)
		nextTarget := CompactToBig(nextBits)

		// Make sure that difficulty increases faster.
		if nextTarget.Cmp(currentTarget) >= 0 {
			t.Error("difficulty should increase")
		}

		if new(big.Int).Sub(currentTarget, nextTarget).Cmp(new(big.Int).Rsh(currentTarget, 4)) >= 0 {
			t.Error("difficulty changes too large")
		}

		bits = nextBits
		i++
	}

	// Check the actual value.
	if bits != 0x1c0db19f {
		t.Error("Calculated bits error")
	}

	// We start to emit blocks significantly slower. The first block has no
	// impact.
	blocks[i] = GetBlockNode(blocks[i-1], 6000, bits)
	header = blocks[i].Header()
	AddBlockToChain(blocks, i, &header, chain)
	bits, err = chain.getNextCashWorkRequired(blocks[i], nil)
	i++
	if err != nil {
		panic(err)
	}

	// Check the actual value.
	if bits != 0x1c0d9222 {
		t.Error("Calculated bits error")
	}

	// If we dramatically slow down block production, difficulty decreases.
	for j := 0; j < 93; j++ {
		blocks[i] = GetBlockNode(blocks[i-1], 6000, bits)
		header = blocks[i].Header()
		AddBlockToChain(blocks, i, &header, chain)
		nextBits, err := chain.getNextCashWorkRequired(blocks[i], nil)
		if err != nil {
			panic(err)
		}

		currentTarget := CompactToBig(bits)
		nextTarget := CompactToBig(nextBits)

		// Check the difficulty decreases.
		if nextTarget.Cmp(chain.chainParams.PowLimit) > 0 {
			t.Error("Calculated difficulty should be equal or larger than powLimit")
		}

		if nextTarget.Cmp(currentTarget) <= 0 {
			t.Error("difficulty should increase")
		}

		if new(big.Int).Sub(currentTarget, nextTarget).Cmp(new(big.Int).Rsh(currentTarget, 3)) >= 0 {
			t.Error("difficulty changes too large")
		}

		bits = nextBits
		i++
	}

	// Check the actual value.
	if bits != 0x1c2f13b9 {
		t.Error("Calculated bits error")
	}

	// Due to the window of time being bounded, next block's difficulty actually
	// gets harder.
	blocks[i] = GetBlockNode(blocks[i-1], 6000, bits)
	header = blocks[i].Header()
	AddBlockToChain(blocks, i, &header, chain)
	bits, err = chain.getNextCashWorkRequired(blocks[i], nil)
	i++
	if err != nil {
		panic(err)
	}
	if bits != 0x1c2ee9bf {
		t.Error("Calculated bits error")
	}

	// And goes down again. It takes a while due to the window being bounded and
	// the skewed block causes 2 blocks to get out of the window.
	for j := 0; j < 192; j++ {
		blocks[i] = GetBlockNode(blocks[i-1], 6000, bits)
		header = blocks[i].Header()
		AddBlockToChain(blocks, i, &header, chain)
		nextBits, err := chain.getNextCashWorkRequired(blocks[i], nil)
		if err != nil {
			panic(err)
		}

		currentTarget := CompactToBig(bits)
		nextTarget := CompactToBig(nextBits)

		// Check the difficulty decreases.
		if nextTarget.Cmp(chain.chainParams.PowLimit) > 0 {
			t.Error("Calculated difficulty should be equal or larger than powLimit")
		}

		if nextTarget.Cmp(currentTarget) <= 0 {
			t.Error("difficulty should increase")
		}

		if new(big.Int).Sub(currentTarget, nextTarget).Cmp(new(big.Int).Rsh(currentTarget, 3)) >= 0 {
			t.Error("difficulty changes too large")
		}

		bits = nextBits
		i++
	}

	// Check the actual value.
	if bits != 0x1d00ffff {
		t.Error("Calculated bits error")
	}

	// Once the difficulty reached the minimum allowed level, it doesn't get any
	// easier.
	for j := 0; j < 5; j++ {
		blocks[i] = GetBlockNode(blocks[i-1], 6000, bits)
		header = blocks[i].Header()
		AddBlockToChain(blocks, i, &header, chain)
		nextBits, err := chain.getNextCashWorkRequired(blocks[i], nil)
		if err != nil {
			panic(err)
		}

		// Check the difficulty stays constant.
		if nextBits != BigToCompact(chain.chainParams.PowLimit) {
			t.Error("The current difficulty should be the lowest difficulty[powLimit]")
		}

		bits = nextBits
		i++
	}
}
