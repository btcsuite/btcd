// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
This test file is part of the blockchain package rather than than the
blockchain_test package so it can bridge access to the internals to properly
test cases which are either not possible or can't reliably be tested via the
public interface.  The functions are only exported while the tests are being
run.
*/

package blockchain

import (
	"sort"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

// TstSetCoinbaseMaturity makes the ability to set the coinbase maturity
// available to the test package.
func (b *BlockChain) TstSetCoinbaseMaturity(maturity uint16) {
	b.chainParams.CoinbaseMaturity = maturity
}

// TstTimeSorter makes the internal timeSorter type available to the test
// package.
func TstTimeSorter(times []int64) sort.Interface {
	return timeSorter(times)
}

// TstCheckSerializedHeight makes the internal checkSerializedHeight function
// available to the test package.
var TstCheckSerializedHeight = checkSerializedHeight

// TstSetMaxMedianTimeEntries makes the ability to set the maximum number of
// median time entries available to the test package.
func TstSetMaxMedianTimeEntries(val int) {
	maxMedianTimeEntries = val
}

// TstCheckBlockScripts makes the internal checkBlockScripts function available
// to the test package.
var TstCheckBlockScripts = checkBlockScripts

// TstDeserializeUtxoEntry makes the internal deserializeUtxoEntry function
// available to the test package.
var TstDeserializeUtxoEntry = deserializeUtxoEntry

// TstNewFakeChain returns a chain that is usable for syntetic tests.  It is
// important to note that this chain has no database associated with it, so
// it is not usable with all functions and the tests must take care when making
// use of it.
func TstNewFakeChain(params *chaincfg.Params) (*BlockChain, *blockNode) {
	// Create a genesis block node and block index index populated with it
	// for use when creating the fake chain below.
	node := newBlockNode(&params.GenesisBlock.Header, 0)
	node.inMainChain = true
	index := newBlockIndex(nil, params)
	index.AddNode(node)

	targetTimespan := int64(params.TargetTimespan / time.Second)
	targetTimePerBlock := int64(params.TargetTimePerBlock / time.Second)
	adjustmentFactor := params.RetargetAdjustmentFactor
	return &BlockChain{
		chainParams:         params,
		timeSource:          NewMedianTime(),
		minRetargetTimespan: targetTimespan / adjustmentFactor,
		maxRetargetTimespan: targetTimespan * adjustmentFactor,
		blocksPerRetarget:   int32(targetTimespan / targetTimePerBlock),
		index:               index,
		warningCaches:       newThresholdCaches(vbNumBits),
		deploymentCaches:    newThresholdCaches(chaincfg.DefinedDeployments),
		bestNode:            node,
	}, node
}

// TstNewFakeNode creates a block node connected to the passed parent with the
// provided fields populated and fake values for the other fields and adds it
// to the blockchain's index as well as makes it the best node.
func (b *BlockChain) TstNewFakeNode(parent *blockNode, blockVersion int32, bits uint32, timestamp time.Time) *blockNode {
	// Make up a header and create a block node from it.
	header := &wire.BlockHeader{
		Version:   blockVersion,
		PrevBlock: parent.hash,
		Bits:      bits,
		Timestamp: timestamp,
	}
	node := newBlockNode(header, parent.height+1)
	node.parent = parent
	node.workSum.Add(parent.workSum, node.workSum)

	b.index.AddNode(node)
	b.bestNode = node
	return node
}
