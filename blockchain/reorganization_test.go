// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"bytes"
	"compress/bzip2"
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
)

// reorgTestLong does a single, large reorganization.
func reorgTestLong(t *testing.T, params *chaincfg.Params) {
	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("reorgunittest", params)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	// The genesis block should fail to connect since it's already
	// inserted.
	err = chain.CheckConnectBlock(dcrutil.NewBlock(params.GenesisBlock),
		blockchain.BFNone)
	if err == nil {
		t.Errorf("CheckConnectBlock: Did not receive expected error")
	}

	// Load up the rest of the blocks up to HEAD.
	filename := filepath.Join("testdata/", "reorgto179.bz2")
	fi, err := os.Open(filename)
	if err != nil {
		t.Errorf("Unable to open %s: %v", filename, err)
	}
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder := gob.NewDecoder(bcBuf)
	blockChain := make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&blockChain); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	// Load up the short chain
	finalIdx1 := 179
	for i := 1; i < finalIdx1+1; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Fatalf("NewBlockFromBytes error: %v", err.Error())
		}

		_, _, err = chain.ProcessBlock(bl, blockchain.BFNone)
		if err != nil {
			t.Fatalf("ProcessBlock error at height %v: %v", i, err.Error())
		}
	}

	// Load the long chain and begin loading blocks from that too,
	// forcing a reorganization
	// Load up the rest of the blocks up to HEAD.
	filename = filepath.Join("testdata/", "reorgto180.bz2")
	fi, err = os.Open(filename)
	if err != nil {
		t.Errorf("Unable to open %s: %v", filename, err)
	}
	bcStream = bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf = new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder = gob.NewDecoder(bcBuf)
	blockChain = make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&blockChain); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	forkPoint := 131
	finalIdx2 := 180
	for i := forkPoint; i < finalIdx2+1; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Fatalf("NewBlockFromBytes error: %v", err.Error())
		}

		_, _, err = chain.ProcessBlock(bl, blockchain.BFNone)
		if err != nil {
			t.Fatalf("ProcessBlock error: %v", err.Error())
		}
	}

	// Ensure our blockchain is at the correct best tip
	topBlock, _ := chain.GetTopBlock()
	tipHash := topBlock.Hash()
	expected, _ := chainhash.NewHashFromStr("5ab969d0afd8295b6cd1506f2a310d" +
		"259322015c8bd5633f283a163ce0e50594")
	if *tipHash != *expected {
		t.Errorf("Failed to correctly reorg; expected tip %v, got tip %v",
			expected, tipHash)
	}
	have, err := chain.HaveBlock(expected)
	if !have {
		t.Errorf("missing tip block after reorganization test")
	}
	if err != nil {
		t.Errorf("unexpected error testing for presence of new tip block "+
			"after reorg test: %v", err)
	}
}

// reorgTestsShort does short reorganizations to test multiple, frequent
// reorganizations.
func reorgTestShort(t *testing.T, params *chaincfg.Params) {
	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("reorgunittestshort", params)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	// The genesis block should fail to connect since it's already
	// inserted.
	err = chain.CheckConnectBlock(dcrutil.NewBlock(params.GenesisBlock),
		blockchain.BFNone)
	if err == nil {
		t.Errorf("CheckConnectBlock: Did not receive expected error")
	}

	// Load up the rest of the blocks up to HEAD.
	filename := filepath.Join("testdata/", "reorgto179.bz2")
	fi, err := os.Open(filename)
	if err != nil {
		t.Errorf("Unable to open %s: %v", filename, err)
	}
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder := gob.NewDecoder(bcBuf)
	blockChain1 := make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&blockChain1); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	// Load the long chain and begin loading blocks from that too,
	// forcing a reorganization
	// Load up the rest of the blocks up to HEAD.
	filename = filepath.Join("testdata/", "reorgto180.bz2")
	fi, err = os.Open(filename)
	if err != nil {
		t.Errorf("Unable to open %s: %v", filename, err)
	}
	bcStream = bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf = new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder = gob.NewDecoder(bcBuf)
	blockChain2 := make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&blockChain2); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	forkPoint := 131
	finalIdx2 := 180

	for i := 1; i < forkPoint+1; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain1[int64(i)])
		if err != nil {
			t.Fatalf("NewBlockFromBytes error: %v", err.Error())
		}

		_, _, err = chain.ProcessBlock(bl, blockchain.BFNone)
		if err != nil {
			t.Fatalf("ProcessBlock error at height %v: %v", i, err.Error())
		}
	}

	// Reorg each block.
	dominant := blockChain2
	orphaned := blockChain1
	for i := forkPoint; i < finalIdx2; i++ {
		for j := 0; j < 2; j++ {
			bl, err := dcrutil.NewBlockFromBytes(dominant[int64(i+j)])
			if err != nil {
				t.Fatalf("NewBlockFromBytes error: %v", err.Error())
			}

			_, _, err = chain.ProcessBlock(bl, blockchain.BFNone)
			if err != nil {
				t.Fatalf("ProcessBlock error: %v", err.Error())
			}
		}

		dominant, orphaned = orphaned, dominant
	}

	// Ensure our blockchain is at the correct best tip
	topBlock, _ := chain.GetTopBlock()
	tipHash := topBlock.Hash()
	expected, _ := chainhash.NewHashFromStr("5ab969d0afd8295b6cd1506f2a310d" +
		"259322015c8bd5633f283a163ce0e50594")
	if *tipHash != *expected {
		t.Errorf("Failed to correctly reorg; expected tip %v, got tip %v",
			expected, tipHash)
	}
	have, err := chain.HaveBlock(expected)
	if !have {
		t.Errorf("missing tip block after reorganization test")
	}
	if err != nil {
		t.Errorf("unexpected error testing for presence of new tip block "+
			"after reorg test: %v", err)
	}
}

// reorgTestsForced tests a forced reorganization of a single block at HEAD.
func reorgTestForced(t *testing.T, params *chaincfg.Params) {
	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("reorgunittestforce", params)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	// The genesis block should fail to connect since it's already
	// inserted.
	err = chain.CheckConnectBlock(dcrutil.NewBlock(params.GenesisBlock),
		blockchain.BFNone)
	if err == nil {
		t.Errorf("CheckConnectBlock: Did not receive expected error")
	}

	// Load up the rest of the blocks up to HEAD.
	filename := filepath.Join("testdata/", "reorgto179.bz2")
	fi, err := os.Open(filename)
	if err != nil {
		t.Errorf("Failed to open %s: %v", filename, err)
	}
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder := gob.NewDecoder(bcBuf)
	blockChain := make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&blockChain); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	// Load up the short chain
	finalIdx1 := 131
	var oldBestHash *chainhash.Hash
	for i := 1; i < finalIdx1+1; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Fatalf("NewBlockFromBytes error: %v", err.Error())
		}
		if i == finalIdx1 {
			oldBestHash = bl.Hash()
		}

		_, _, err = chain.ProcessBlock(bl, blockchain.BFNone)
		if err != nil {
			t.Fatalf("ProcessBlock error at height %v: %v", i, err.Error())
		}
	}

	// Load the long chain and begin loading blocks from that too,
	// forcing a reorganization
	// Load up the rest of the blocks up to HEAD.
	filename = filepath.Join("testdata/", "reorgto180.bz2")
	fi, err = os.Open(filename)
	if err != nil {
		t.Errorf("Failed to open %s: %v", filename, err)
	}
	bcStream = bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf = new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder = gob.NewDecoder(bcBuf)
	blockChain = make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&blockChain); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	forkPoint := int64(131)
	forkBl, err := dcrutil.NewBlockFromBytes(blockChain[forkPoint])
	if err != nil {
		t.Fatalf("NewBlockFromBytes error: %v", err.Error())
	}

	_, _, err = chain.ProcessBlock(forkBl, blockchain.BFNone)
	if err != nil {
		t.Fatalf("ProcessBlock error: %v", err.Error())
	}
	newBestHash := forkBl.Hash()

	err = chain.ForceHeadReorganization(*oldBestHash, *newBestHash)
	if err != nil {
		t.Fatalf("failed forced reorganization: %v", err.Error())
	}

	// Ensure our blockchain is at the correct best tip for our forced
	// reorganization
	topBlock, _ := chain.GetTopBlock()
	tipHash := topBlock.Hash()
	expected, _ := chainhash.NewHashFromStr("0df603f434be1dca22d706c7c47be16a8" +
		"edcef2f151bcf08b51138aa1cda26e2")
	if *tipHash != *expected {
		t.Errorf("Failed to correctly reorg; expected tip %v, got tip %v",
			expected, tipHash)
	}
	have, err := chain.HaveBlock(expected)
	if !have {
		t.Errorf("missing tip block after reorganization test")
	}
	if err != nil {
		t.Errorf("unexpected error testing for presence of new tip block "+
			"after reorg test: %v", err)
	}
}

// TestReorganization loads a set of test blocks which force a chain
// reorganization to test the block chain handling code.
func TestReorganization(t *testing.T) {
	// Update simnet parameters to reflect what is expected by the legacy
	// data.
	params := cloneParams(&chaincfg.SimNetParams)
	params.GenesisBlock.Header.MerkleRoot = *mustParseHash("a216ea043f0d481a072424af646787794c32bcefd3ed181a090319bbf8a37105")
	genesisHash := params.GenesisBlock.BlockHash()
	params.GenesisHash = &genesisHash

	reorgTestLong(t, params)
	reorgTestShort(t, params)
	reorgTestForced(t, params)
}
