package main

import (
	"bytes"
	"testing"

	"github.com/btcsuite/btcd/rpctest"
)

func TestGetBestBlock(t *testing.T) {
	nodeTest, err := rpctest.New(nil, nil)
	if err != nil {
		t.Fatalf("Unable to create rpctest: ", err)
	}

	if err := nodeTest.SetUp(true); err != nil {
		t.Fatalf("Unable to set up rpctest: ", err)
	}
	defer nodeTest.TearDown()

	_, prevbestHeight, err := nodeTest.Client.GetBestBlock()
	if err != nil {
		t.Fatalf("Call to `getbestblock` failed: ", err)
	}

	// Create a new block connecting to the current tip.
	newBlock, err := nodeTest.GenerateAndSubmitBlock(nil)
	if err != nil {
		t.Fatalf("Unable to generate block: ", err)
	}

	bestHash, bestHeight, err := nodeTest.Client.GetBestBlock()
	if err != nil {
		t.Fatalf("Call to `getbestblock` failed: ", err)
	}

	// Hash should be the same as the newly submitted block.
	expectedSha, _ := newBlock.Sha()
	if !bytes.Equal(bestHash[:], expectedSha[:]) {
		t.Fatalf("Block hashes do not match. Returned hash %v, wanted "+
			"hash %v", bestHash, expectedSha)
	}

	// Block height should now reflect newest height.
	if bestHeight != prevbestHeight+1 {
		t.Fatalf("Block heights do not match. Got %v, wanted %v",
			bestHeight, prevbestHeight+1)
	}
}

func TestGetBlockCount(t *testing.T) {
	nodeTest, err := rpctest.New(nil, nil)
	if err != nil {
		t.Fatalf("Unable to create rpctest: ", err)
	}

	if err := nodeTest.SetUp(true); err != nil {
		t.Errorf("Unable to set up rpctest: ", err)
	}
	defer nodeTest.TearDown()

	// Save the current count.
	currentCount, err := nodeTest.Client.GetBlockCount()
	if err != nil {
		t.Fatalf("Unable to get block count: ", err)
	}

	// Generate and submit a block.
	if _, err := nodeTest.GenerateAndSubmitBlock(nil); err != nil {
		t.Fatalf("Unable to generate block: ", err)
	}

	// Count should have increased by one.
	newCount, err := nodeTest.Client.GetBlockCount()
	if err != nil {
		t.Fatalf("Unable to get block count: ", err)
	}
	if newCount != currentCount+1 {
		t.Fatalf("Block count incorrect. Got %v should be %v",
			newCount, currentCount+1)
	}
}

func TestGetBlockHash(t *testing.T) {
	nodeTest, err := rpctest.New(nil, nil)
	if err != nil {
		t.Fatalf("Unable to create rpctest: ", err)
	}

	if err := nodeTest.SetUp(true); err != nil {
		t.Errorf("Unable to set up rpctest: ", err)
	}
	defer nodeTest.TearDown()

	// Create a new block connecting to the current tip.
	newBlock, err := nodeTest.GenerateAndSubmitBlock(nil)
	if err != nil {
		t.Fatalf("Unable to generate block: ", err)
	}

	blockHash, err := nodeTest.Client.GetBlockHash(newBlock.Height() + 1)
	if err != nil {
		t.Fatalf("Call to `getblockhash` failed: ", err)
	}

	// Block hashes should match newly created block.
	expectedSha, _ := newBlock.Sha()
	if !bytes.Equal(blockHash[:], expectedSha[:]) {
		t.Fatalf("Block hashes do not match. Returned hash %v, wanted "+
			"hash %v", blockHash, expectedSha)
	}
}
